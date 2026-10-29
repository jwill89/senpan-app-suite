package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Gateway op codes, as Discord numbers them.
const (
	opDispatch       = 0
	opHeartbeat      = 1
	opIdentify       = 2
	opPresence       = 3
	opResume         = 6
	opReconnect      = 7
	opInvalidSession = 9
	opHello          = 10
	opHeartbeatACK   = 11
)

// IntentGuilds is the one intent the app asks for: channel and role changes, which drop the
// pickers' cache early. It is not privileged, so no Developer Portal switch is needed.
const IntentGuilds = 1 << 0

type gatewayPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	S  *int64          `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

// GatewayStatus is what the Discord page shows about the live connection.
type GatewayStatus struct {
	Online      bool   `json:"online"`
	ConnectedAt string `json:"connected_at,omitempty"`
	LastError   string `json:"last_error,omitempty"`
}

// Gateway keeps one session open so the bot shows as online, heartbeating and resuming as
// Discord asks. Posting never depends on it: every post goes over REST.
type Gateway struct {
	client *Client
	// URL replaces the gateway address Discord hands out; tests point it at a fake.
	URL string
	// Intents is what the session asks to be sent, at identify.
	Intents int
	// Handle receives each dispatched event by name, when set. It runs on the session's
	// goroutine, so it must return quickly - slow work belongs on a goroutine of its own, or
	// heartbeats stall and Discord drops the session.
	Handle func(event string, data json.RawMessage)
	// Presence says how the bot should show, asked each time it is about to be told to
	// Discord. Unset, the bot is plainly online.
	Presence func() Presence
	// refresh holds at most one request to tell Discord the presence again.
	refresh chan struct{}

	mu        sync.Mutex
	status    GatewayStatus
	sessionID string
	resumeURL string
	seq       int64
	userID    string
	appID     string
}

// NewGateway returns a gateway for the client. A nil client gives a gateway that does
// nothing, so callers need no special case for "Discord is off".
func NewGateway(client *Client) *Gateway {
	return &Gateway{client: client, refresh: make(chan struct{}, 1)}
}

// Self is the bot's own account and application ids, known once a session has been ready.
func (g *Gateway) Self() (userID, applicationID string) {
	if g == nil {
		return "", ""
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.userID, g.appID
}

// RefreshPresence has the open session tell Discord how the bot should show now. Asked while
// no session is up, it is kept for the next one, which is harmless: that session says the
// same thing as it identifies.
func (g *Gateway) RefreshPresence() {
	if g == nil || g.client == nil {
		return
	}
	select {
	case g.refresh <- struct{}{}:
	default:
	}
}

func (g *Gateway) presence() map[string]any {
	if g.Presence == nil {
		return Presence{}.payload()
	}
	return g.Presence().payload()
}

// Status reports whether a session is up, since when, and the last error otherwise.
func (g *Gateway) Status() GatewayStatus {
	if g == nil || g.client == nil {
		return GatewayStatus{}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.status
}

// Run connects, and reconnects with backoff, until the context ends.
func (g *Gateway) Run(ctx context.Context) {
	if g == nil || g.client == nil {
		return
	}
	backoff := time.Second
	for {
		started := time.Now()
		err := g.session(ctx)
		if ctx.Err() != nil {
			g.setOffline("")
			return
		}
		// After a close code Discord will not resume from, the session is let go of so the
		// next attempt identifies afresh instead of asking for a resume Discord would refuse.
		if code := websocket.CloseStatus(err); code != -1 && !resumable(code) {
			g.forget()
		}
		g.setOffline(err.Error())
		slog.Warn("discord gateway disconnected", "error", err)
		// A session that held for a while earns a fresh start on the backoff.
		if time.Since(started) > time.Minute {
			backoff = time.Second
		}
		wait := backoff + time.Duration(rand.Int64N(int64(time.Second))) //nolint:gosec // jitter, not a secret
		if sleep(ctx, wait) != nil {
			return
		}
		if backoff < 5*time.Minute {
			backoff *= 2
		}
	}
}

const (
	handshakeTimeout = 15 * time.Second
	// The most one frame may hold. READY and GUILD_CREATE run far past the websocket
	// library's 32 KB default, which in Kindred closed every session at startup.
	frameLimit = 16 << 20
)

var (
	errReconnect = errors.New("discord asked for a reconnect")
	errZombie    = errors.New("no heartbeat ack")
)

// session runs one connection: hello, identify or resume, then heartbeats and events, until
// Discord or the context ends it.
func (g *Gateway) session(ctx context.Context) error {
	url, err := g.address(ctx)
	if err != nil {
		return err
	}
	// A handshake that stalls must fail and be retried, not park the gateway for good. The
	// deadline is the handshake's alone: once the protocol switches, the context no longer
	// touches the connection.
	dialing, cancel := context.WithTimeout(ctx, handshakeTimeout)
	ws, _, err := websocket.Dial(dialing, url+"/?v=10&encoding=json", nil) //nolint:bodyclose // the handshake response belongs to the library, whose docs say never to close it
	cancel()
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	// Torn down, never closed with a code: Discord resumes a session only when its socket
	// went away without a goodbye, and a normal close would throw the session away.
	defer func() { _ = ws.CloseNow() }()
	ws.SetReadLimit(frameLimit)

	done := make(chan struct{})
	defer close(done)
	frames := make(chan gatewayPayload)
	readErr := make(chan error, 1)
	go func() {
		for {
			var p gatewayPayload
			if err := wsjson.Read(ctx, ws, &p); err != nil {
				readErr <- err
				return
			}
			select {
			case frames <- p:
			case <-done:
				return
			}
		}
	}()

	var hello struct {
		HeartbeatInterval float64 `json:"heartbeat_interval"`
	}
	select {
	case p := <-frames:
		if p.Op != opHello {
			return fmt.Errorf("expected hello, got op %d", p.Op)
		}
		if err := json.Unmarshal(p.D, &hello); err != nil {
			return fmt.Errorf("hello: %w", err)
		}
	case err := <-readErr:
		return fmt.Errorf("hello: %w", err)
	case <-time.After(handshakeTimeout):
		return errors.New("hello: discord sent nothing")
	case <-ctx.Done():
		return ctx.Err()
	}
	interval := time.Duration(hello.HeartbeatInterval) * time.Millisecond

	g.mu.Lock()
	resuming := g.sessionID != ""
	sessionID, seq := g.sessionID, g.seq
	g.mu.Unlock()
	if resuming {
		err = wsjson.Write(ctx, ws, map[string]any{"op": opResume, "d": map[string]any{
			"token": g.client.token, "session_id": sessionID, "seq": seq,
		}})
	} else {
		err = wsjson.Write(ctx, ws, map[string]any{"op": opIdentify, "d": map[string]any{
			"token":      g.client.token,
			"intents":    g.Intents,
			"properties": map[string]string{"os": "linux", "browser": "senpan-app-suite", "device": "senpan-app-suite"},
			"presence":   g.presence(),
		}})
	}
	if err != nil {
		return fmt.Errorf("identify: %w", err)
	}

	// The first beat waits a random part of the interval, as the protocol asks.
	beat := time.NewTimer(time.Duration(rand.Float64() * float64(interval))) //nolint:gosec // jitter, not a secret
	defer beat.Stop()
	acked := true
	// A presence is told only to a session Discord has accepted; one asked for sooner waits.
	ready, owed := false, false
	tell := func() error {
		if err := wsjson.Write(ctx, ws, map[string]any{"op": opPresence, "d": g.presence()}); err != nil {
			return fmt.Errorf("presence: %w", err)
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return fmt.Errorf("read: %w", err)
		case <-g.refresh:
			if !ready {
				owed = true
				continue
			}
			if err := tell(); err != nil {
				return err
			}
		case <-beat.C:
			if !acked {
				return errZombie
			}
			acked = false
			if err := g.heartbeat(ctx, ws); err != nil {
				return err
			}
			beat.Reset(interval)
		case p := <-frames:
			if p.S != nil {
				g.mu.Lock()
				g.seq = *p.S
				g.mu.Unlock()
			}
			switch p.Op {
			case opDispatch:
				g.dispatch(p.T, p.D)
				if p.T == "READY" || p.T == "RESUMED" {
					ready = true
					if owed {
						owed = false
						if err := tell(); err != nil {
							return err
						}
					}
				}
			case opHeartbeat:
				if err := g.heartbeat(ctx, ws); err != nil {
					return err
				}
			case opHeartbeatACK:
				acked = true
			case opReconnect:
				return errReconnect
			case opInvalidSession:
				var canResume bool
				_ = json.Unmarshal(p.D, &canResume)
				if !canResume {
					g.forget()
				}
				return errors.New("session invalidated")
			}
		}
	}
}

func (g *Gateway) heartbeat(ctx context.Context, ws *websocket.Conn) error {
	g.mu.Lock()
	seq := g.seq
	g.mu.Unlock()
	var d any
	if seq > 0 {
		d = seq
	}
	if err := wsjson.Write(ctx, ws, map[string]any{"op": opHeartbeat, "d": d}); err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	return nil
}

// dispatch notes the session on READY and RESUMED and passes every event on.
func (g *Gateway) dispatch(event string, data json.RawMessage) {
	switch event {
	case "READY":
		var ready struct {
			SessionID        string `json:"session_id"`
			ResumeGatewayURL string `json:"resume_gateway_url"`
			User             struct {
				ID string `json:"id"`
			} `json:"user"`
			Application struct {
				ID string `json:"id"`
			} `json:"application"`
		}
		_ = json.Unmarshal(data, &ready)
		g.mu.Lock()
		g.sessionID, g.resumeURL = ready.SessionID, ready.ResumeGatewayURL
		g.userID, g.appID = ready.User.ID, ready.Application.ID
		g.status = GatewayStatus{Online: true, ConnectedAt: time.Now().UTC().Format(time.RFC3339)}
		g.mu.Unlock()
		slog.Info("discord gateway ready")
	case "RESUMED":
		g.mu.Lock()
		g.status = GatewayStatus{Online: true, ConnectedAt: time.Now().UTC().Format(time.RFC3339)}
		g.mu.Unlock()
		slog.Info("discord gateway resumed")
	}
	if g.Handle != nil {
		g.Handle(event, data)
	}
}

// address is where to connect: the resume address while a session can be resumed, otherwise
// the one Discord hands out.
func (g *Gateway) address(ctx context.Context) (string, error) {
	g.mu.Lock()
	resume := g.resumeURL
	resuming := g.sessionID != ""
	g.mu.Unlock()
	if resuming && resume != "" {
		return resume, nil
	}
	if g.URL != "" {
		return g.URL, nil
	}
	return g.client.GatewayURL(ctx)
}

// resumable says whether Discord will take a resume after closing with this code. The rest
// end the session for good: a bad token, an intent the bot may not ask for, a sequence or a
// session Discord no longer has.
func resumable(code websocket.StatusCode) bool {
	return !slices.Contains([]websocket.StatusCode{4004, 4007, 4009, 4010, 4011, 4012, 4013, 4014}, code)
}

func (g *Gateway) forget() {
	g.mu.Lock()
	g.sessionID, g.resumeURL, g.seq = "", "", 0
	g.mu.Unlock()
}

func (g *Gateway) setOffline(reason string) {
	g.mu.Lock()
	g.status = GatewayStatus{Online: false, LastError: reason}
	g.mu.Unlock()
}
