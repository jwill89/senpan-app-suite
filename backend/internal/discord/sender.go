package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Timeout bounds one post to Discord.
const Timeout = 15 * time.Second

var defaultClient = &http.Client{Timeout: Timeout}

// Sender is the one place a post leaves the process.
//
// A development server usually runs on a copy of the live database, whose webhook
// URLs are real, so two valves sit here, where every posting feature goes through:
// DryRun logs a post instead of sending it, and Override sends every post to one
// test webhook whatever the record says. DryRun wins when both are set; both are
// off in production.
type Sender struct {
	// HTTP is the client posts go through; nil uses a client with Timeout.
	HTTP *http.Client
	// DryRun suppresses every post, logging it instead.
	DryRun bool
	// Override, when set, is the webhook every post goes to.
	Override string
}

func (s *Sender) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return defaultClient
}

// PostEmbed sends a single embed to the webhook.
func (s *Sender) PostEmbed(ctx context.Context, target Target, webhookURL string, embed Embed) error {
	return s.Post(ctx, target, webhookURL, Payload{Embeds: []Embed{embed}})
}

// Post sends a webhook message. A payload with components is posted with
// `?with_components=true`: without it Discord posts the message and silently drops
// the buttons. Link buttons don't interact, so a channel webhook may send them once
// the flag is set.
func (s *Sender) Post(ctx context.Context, target Target, webhookURL string, payload Payload) error {
	return s.send(ctx, target, webhookURL, payload, nil)
}

// PostFile sends a message with one file attached, as Discord's multipart form
// (payload_json and files[0]). An embed in the payload shows the file as
// attachment://<file.Name>.
func (s *Sender) PostFile(ctx context.Context, target Target, webhookURL string, payload Payload, file File) error {
	return s.send(ctx, target, webhookURL, payload, &file)
}

func (s *Sender) send(ctx context.Context, target Target, webhookURL string, payload Payload, file *File) error {
	if s.DryRun {
		slog.Warn("discord post SUPPRESSED (dry run)", "kind", target.Kind, "name", target.Name,
			"embeds", len(payload.Embeds), "attachment", file != nil)
		return nil
	}
	if s.Override != "" {
		slog.Warn("discord post REDIRECTED to the override webhook", "kind", target.Kind, "name", target.Name)
		webhookURL = s.Override
	}

	contentType := "application/json"
	if file != nil {
		payload.Attachments = []attachment{{ID: 0, Filename: file.Name}}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode embed")
	}
	if file != nil {
		if body, contentType, err = multipartBody(body, *file); err != nil {
			return fmt.Errorf("encode attachment: %w", err)
		}
	}
	if len(payload.Components) > 0 {
		webhookURL = withComponentsParam(webhookURL)
	}
	slog.Debug("discord webhook post", "kind", target.Kind, "name", target.Name,
		"embeds", len(payload.Embeds), "components", len(payload.Components), "bytes", len(body))
	start := time.Now()
	// Every stored webhook passed IsWebhookURL when it was saved, and the override is
	// the operator's own flag or environment value.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body)) //nolint:gosec // a checked Discord webhook URL (see above)
	if err != nil {
		return &Error{Kind: Unreachable, Message: sanitize(err, s.client()), Err: err}
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := s.client().Do(req) //nolint:gosec // a checked Discord webhook URL (see above)
	if err != nil {
		// The error is not wrapped: http.Client bakes the webhook URL, whose path
		// is its token, into the *url.Error it returns.
		return &Error{Kind: transportKind(err), Message: sanitize(err, s.client())}
	}
	defer func() { _ = resp.Body.Close() }() // read below: closing it has nothing to report
	// The body is read, not discarded, so a refusal can say why ("Unknown Webhook"
	// for a deleted webhook) and the keep-alive connection is reusable.
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	slog.Debug("discord webhook response", "status", resp.StatusCode)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError(resp, respBody)
	}
	// The one record that a post reached Discord. A failure is logged by its caller,
	// which knows whether it will retry or answer an error, so this covers success
	// only.
	slog.Info("discord post sent", "kind", target.Kind, "name", target.Name,
		"status", resp.StatusCode, "embeds", len(payload.Embeds),
		"duration_ms", time.Since(start).Milliseconds())
	return nil
}

// transportKind says whether a request that failed outright may have reached
// Discord: a failure to connect never left, anything else (a timeout, a reset
// connection) may have.
func transportKind(err error) Kind {
	if op, ok := errors.AsType[*net.OpError](err); ok && op.Op == "dial" && !op.Timeout() {
		return Unreachable
	}
	return Ambiguous
}

// statusError is the error for a non-2xx answer.
func statusError(resp *http.Response, body []byte) *Error {
	e := &Error{Status: resp.StatusCode, Message: errorMessage(body)}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		e.Kind = RateLimited
		e.RetryAfter = retryAfter(resp.Header.Get("Retry-After"), body)
	case resp.StatusCode >= 500:
		e.Kind = Unreachable
	default:
		e.Kind = Refused
	}
	return e
}

// retryAfter reads how long Discord asked to wait: the Retry-After header in
// seconds, else the body's retry_after (seconds, possibly fractional). Zero when
// neither says.
func retryAfter(header string, body []byte) time.Duration {
	if secs, err := strconv.ParseFloat(strings.TrimSpace(header), 64); err == nil && secs > 0 {
		return time.Duration(secs * float64(time.Second))
	}
	var b struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if err := json.Unmarshal(body, &b); err == nil && b.RetryAfter > 0 {
		return time.Duration(b.RetryAfter * float64(time.Second))
	}
	return 0
}

// multipartBody wraps a JSON payload and one file in the multipart form a webhook
// takes for an upload, returning the body and its Content-Type.
func multipartBody(payloadJSON []byte, file File) ([]byte, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("payload_json", string(payloadJSON)); err != nil {
		return nil, "", err
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files[0]"; filename=%q`, file.Name))
	h.Set("Content-Type", file.ContentType)
	part, err := mw.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(file.Data); err != nil {
		return nil, "", err
	}
	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), mw.FormDataContentType(), nil
}

// errorMessage extracts a readable reason from Discord's error body, a JSON
// envelope like {"message":"Unknown Webhook","code":10015}: "Unknown Webhook (code
// 10015)". Without one it is a trimmed snippet of the body, or "no response body".
// The body is Discord's, so it never carries the webhook token.
func errorMessage(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return "no response body"
	}
	var e struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		if e.Code != 0 {
			return fmt.Sprintf("%s (code %d)", e.Message, e.Code)
		}
		return e.Message
	}
	return TruncateRunes(string(body), 300)
}

// sanitize renders a transport error without the webhook URL, whose path carries the
// secret token. http.Client.Do wraps a failure in a *url.Error that prints the full
// URL, so only the underlying cause is kept.
func sanitize(err error, c *http.Client) string {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		if urlErr.Timeout() {
			return fmt.Sprintf("timed out after %s", c.Timeout)
		}
		return fmt.Sprintf("could not connect (%v)", urlErr.Err)
	}
	return err.Error()
}

// withComponentsParam adds `with_components=true` to the webhook URL's query. A URL
// that can't be parsed comes back unchanged.
func withComponentsParam(webhookURL string) string {
	u, err := url.Parse(webhookURL)
	if err != nil {
		return webhookURL
	}
	q := u.Query()
	q.Set("with_components", "true")
	u.RawQuery = q.Encode()
	return u.String()
}
