// Package discord talks to Discord as the app's bot: a REST client for posting, editing
// and deleting messages and for reading the server's channels and roles, and a gateway
// session that keeps the bot online.
//
// Ported from the Kindred Nights app (internal/discord), which built and hardened the same
// pieces without a third-party Discord library. Every lesson that port carried over is
// noted where it applies.
//
// Nothing here knows about Senpan's features or database: it takes a token, a base URL
// and a context, and returns typed results or one of the errors below.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"app-suite/internal/version"
)

// DefaultAPI is Discord's REST base. Tests (and the local fake) point the client elsewhere.
const DefaultAPI = "https://discord.com/api/v10"

// Errors a caller can act on. ErrNotFound is a 404: a channel, message or role that does
// not exist (a message someone deleted in Discord, most often). ErrRejected is a 400:
// Discord refused the request as written - Said gives its reason. ErrForbidden is a 403:
// something the bot may not do. ErrUnauthorized is a 401: the token is wrong or was reset.
// ErrRateLimited is a 429 with longer to wait than this call was willing to.
var (
	ErrNotFound     = errors.New("discord: not found")
	ErrRejected     = errors.New("discord: rejected")
	ErrForbidden    = errors.New("discord: forbidden")
	ErrUnauthorized = errors.New("discord: unauthorized")
	ErrRateLimited  = errors.New("discord: rate limited")
)

// Client calls the REST API as the bot. Calls time out on their own and carry a context,
// so a shutdown or a client disconnect cancels them.
type Client struct {
	token string
	base  string
	http  *http.Client

	// limits remembers, per route, when Discord said the route's bucket refills after a
	// call used it up (X-RateLimit-Remaining: 0). The next call on that route waits until
	// then instead of spending a request on a 429.
	mu     sync.Mutex
	limits map[string]time.Time
}

// New returns nil for an empty token: every caller treats a nil client as "Discord is off".
func New(token, base string) *Client {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	if base == "" {
		base = DefaultAPI
	}
	return &Client{
		token:  token,
		base:   strings.TrimRight(base, "/"),
		http:   &http.Client{Timeout: 15 * time.Second},
		limits: map[string]time.Time{},
	}
}

// request is one call to Discord. A patient call waits out longer rate limits, and more of
// them: a batch (a reading list, a run of deletes) would rather be late than half done.
type request struct {
	method, path string
	body         []byte
	patient      bool
}

// do sends one JSON request and decodes the answer into out (when non-nil).
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	return c.doPatient(ctx, method, path, body, out, false)
}

func (c *Client) doPatient(ctx context.Context, method, path string, body, out any, patient bool) error {
	r := request{method: method, path: path, patient: patient}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r.body = b
	}
	return c.send(ctx, r, out)
}

type rateLimited struct {
	RetryAfter float64 `json:"retry_after"`
}

// send makes the call. Errors name the method, never the path, so no Discord id reaches a
// log line (and a transport error's URL is unwrapped for the same reason).
func (c *Client) send(ctx context.Context, r request, out any) error {
	longest, tries := 5*time.Second, 2
	if r.patient {
		longest, tries = 60*time.Second, 4
	}
	route := routeKey(r.method, r.path)
	for attempt := 1; ; attempt++ {
		if err := c.waitForRoute(ctx, route, longest); err != nil {
			return err
		}
		var reader io.Reader
		if r.body != nil {
			reader = bytes.NewReader(r.body)
		}
		req, err := http.NewRequestWithContext(ctx, r.method, c.base+r.path, reader)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bot "+c.token)
		req.Header.Set("User-Agent", "DiscordBot (https://apps.senpan.cafe, "+version.Version+")")
		if r.body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if ue, ok := errors.AsType[*url.Error](err); ok {
				err = ue.Err
			}
			return fmt.Errorf("discord %s: %w", r.method, err)
		}
		// Read the whole answer: a truncated body that parsed as empty once hid a failure
		// in Kindred, so a read error fails the call.
		data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("discord %s: read the answer: %w", r.method, err)
		}
		c.noteLimit(route, resp.Header)
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			var rl rateLimited
			_ = json.Unmarshal(data, &rl)
			wait := time.Duration(rl.RetryAfter * float64(time.Second))
			if attempt >= tries || rl.RetryAfter <= 0 || wait > longest {
				return fmt.Errorf("%w: %s, for %.0fs", ErrRateLimited, r.method, rl.RetryAfter)
			}
			if err := sleep(ctx, wait); err != nil {
				return err
			}
			continue
		case resp.StatusCode == http.StatusNotFound:
			return ErrNotFound
		case resp.StatusCode == http.StatusBadRequest:
			return rejection{said: refusal(data)}
		case resp.StatusCode == http.StatusUnauthorized:
			return ErrUnauthorized
		case resp.StatusCode == http.StatusForbidden:
			return ErrForbidden
		case resp.StatusCode < 200 || resp.StatusCode > 299:
			return fmt.Errorf("discord %s: status %d", r.method, resp.StatusCode)
		}
		if out != nil && len(data) > 0 {
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("discord %s: decode: %w", r.method, err)
			}
		}
		return nil
	}
}

// sleep waits d, or less if the context ends first.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// waitForRoute holds a call until the route's bucket has refilled, when an earlier call
// used it up. A wait longer than this call accepts is reported as a rate limit instead.
func (c *Client) waitForRoute(ctx context.Context, route string, longest time.Duration) error {
	c.mu.Lock()
	until, ok := c.limits[route]
	if ok && !time.Now().Before(until) {
		delete(c.limits, route)
		ok = false
	}
	c.mu.Unlock()
	if !ok {
		return nil
	}
	wait := time.Until(until)
	if wait > longest {
		return fmt.Errorf("%w: waiting %.0fs", ErrRateLimited, wait.Seconds())
	}
	return sleep(ctx, wait)
}

// noteLimit records when a route may be called again, from the headers Discord sends with
// every answer. Only an exhausted bucket is remembered.
func (c *Client) noteLimit(route string, h http.Header) {
	if h.Get("X-RateLimit-Remaining") != "0" {
		return
	}
	after, err := strconv.ParseFloat(h.Get("X-RateLimit-Reset-After"), 64)
	if err != nil || after <= 0 {
		return
	}
	c.mu.Lock()
	c.limits[route] = time.Now().Add(time.Duration(after * float64(time.Second)))
	c.mu.Unlock()
}

// snowflake matches a Discord id in a path segment.
var snowflake = regexp.MustCompile(`^\d{5,}$`)

// routeKey groups calls the way Discord's buckets do, closely enough: a route's "major"
// id (the channel or the server it acts on) is kept, and any further id - a message id -
// is folded, since every message in a channel shares that channel's limits.
func routeKey(method, path string) string {
	path, _, _ = strings.Cut(path, "?")
	parts := strings.Split(path, "/")
	kept := 0
	for i, p := range parts {
		if !snowflake.MatchString(p) {
			continue
		}
		kept++
		if kept > 1 {
			parts[i] = "{id}"
		}
	}
	return method + " " + strings.Join(parts, "/")
}

// rejection is a 400 with what Discord said was wrong. It matches ErrRejected; Said gives
// the rest.
type rejection struct {
	said string
}

func (r rejection) Error() string        { return ErrRejected.Error() }
func (r rejection) Is(target error) bool { return target == ErrRejected }

// Said is Discord's own reason for refusing a request (which part of a message, and what is
// wrong with it), or "" when it gave none.
func Said(err error) string {
	if r, ok := errors.AsType[rejection](err); ok {
		return r.said
	}
	return ""
}

// refusal flattens the body of a 400 into lines like "embeds.0.description: Must be 4096 or
// fewer in length." Discord nests its reasons under the path of what they are about.
func refusal(body []byte) string {
	var b struct {
		Message string         `json:"message"`
		Errors  map[string]any `json:"errors"`
	}
	if json.Unmarshal(body, &b) != nil {
		return ""
	}
	var lines []string
	var walk func(path string, node any)
	walk = func(path string, node any) {
		m, ok := node.(map[string]any)
		if !ok {
			return
		}
		if list, ok := m["_errors"].([]any); ok {
			for _, e := range list {
				if said, ok := e.(map[string]any)["message"].(string); ok {
					lines = append(lines, strings.TrimPrefix(path+": ", ": ")+said)
				}
			}
		}
		for _, key := range slices.Sorted(maps.Keys(m)) {
			if key != "_errors" {
				walk(strings.TrimPrefix(path+"."+key, "."), m[key])
			}
		}
	}
	walk("", b.Errors)
	if len(lines) == 0 {
		return b.Message
	}
	return clip(strings.Join(lines, " "), 500)
}

// clip caps s at limit runes.
func clip(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}
