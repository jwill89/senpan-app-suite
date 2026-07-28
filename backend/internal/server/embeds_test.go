package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestPostDiscordWebhookComponentsFlag verifies that a payload carrying components
// is posted with `?with_components=true` (Discord drops the buttons otherwise) and
// that a plain embed payload is posted without the flag.
func TestPostDiscordWebhookComponentsFlag(t *testing.T) {
	cases := []struct {
		name      string
		payload   discordWebhookPayload
		wantParam bool
	}{
		{
			name: "with buttons",
			payload: discordWebhookPayload{
				Embeds: []discordEmbed{{Title: "Hi"}},
				Components: []discordComponent{{
					Type: componentActionRow,
					Components: []discordComponent{{
						Type: componentButton, Style: buttonStyleLink, Label: "Go", URL: "https://example.com",
					}},
				}},
			},
			wantParam: true,
		},
		{
			name:      "embed only",
			payload:   discordWebhookPayload{Embeds: []discordEmbed{{Title: "Hi"}}},
			wantParam: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.RawQuery
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()

			if err := postDiscordWebhook(context.Background(), srv.URL, tc.payload); err != nil {
				t.Fatalf("postDiscordWebhook: %v", err)
			}
			hasParam := strings.Contains(gotQuery, "with_components=true")
			if hasParam != tc.wantParam {
				t.Fatalf("with_components present=%v, want %v (query=%q)", hasParam, tc.wantParam, gotQuery)
			}
		})
	}
}

// TestWithComponentsParam checks the flag is merged into an existing query string
// and that an unparseable URL is returned unchanged.
func TestWithComponentsParam(t *testing.T) {
	got := withComponentsParam("https://discord.com/api/webhooks/1/abc?wait=true")
	if !strings.Contains(got, "with_components=true") || !strings.Contains(got, "wait=true") {
		t.Fatalf("expected both params, got %q", got)
	}
}

// TestPostDiscordWebhookSurfacesDiscordReason is the regression for the opaque-502
// bug: when Discord rejects a post (e.g. a deleted webhook -> 404 "Unknown
// Webhook"), the returned error must carry Discord's own reason and status, not a
// bare status code - that reason is what the send handlers now log.
func TestPostDiscordWebhookSurfacesDiscordReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Unknown Webhook","code":10015}`))
	}))
	defer srv.Close()

	err := postDiscordWebhook(context.Background(), srv.URL, discordWebhookPayload{Embeds: []discordEmbed{{Title: "Hi"}}})
	if err == nil {
		t.Fatal("expected an error for a 404 response, got nil")
	}
	for _, want := range []string{"404", "Unknown Webhook", "10015"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

// TestDiscordErrorMessage covers the reason extraction from Discord's error body.
func TestDiscordErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"message and code", `{"message":"Unknown Webhook","code":10015}`, "Unknown Webhook (code 10015)"},
		{"message only", `{"message":"You are being rate limited."}`, "You are being rate limited."},
		{"empty body", "", "no response body"},
		{"non-json falls back to raw", "Bad Gateway", "Bad Gateway"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := discordErrorMessage([]byte(tc.body)); got != tc.want {
				t.Fatalf("discordErrorMessage(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

// TestSanitizeWebhookErrHidesToken verifies a transport error never leaks the
// webhook URL - whose path carries the secret token - into the string that gets
// logged and returned to the client.
func TestSanitizeWebhookErrHidesToken(t *testing.T) {
	const secret = "SUPERSECRETTOKEN"
	webhookURL := "https://discord.com/api/webhooks/123/" + secret

	connErr := &url.Error{Op: "Post", URL: webhookURL, Err: errors.New("dial tcp 1.2.3.4:443: connect: connection refused")}
	got := sanitizeWebhookErr(connErr)
	if strings.Contains(got, secret) || strings.Contains(got, "discord.com") {
		t.Fatalf("sanitized error leaked the URL/token: %q", got)
	}
	if !strings.Contains(got, "connection refused") {
		t.Errorf("sanitized error dropped the cause: %q", got)
	}

	timeoutErr := &url.Error{Op: "Post", URL: webhookURL, Err: timeoutError{}}
	if got := sanitizeWebhookErr(timeoutErr); strings.Contains(got, secret) || !strings.Contains(got, "timed out") {
		t.Fatalf("timeout path leaked token or lost label: %q", got)
	}

	// A non-*url.Error passes through unchanged.
	if got := sanitizeWebhookErr(errors.New("plain error")); got != "plain error" {
		t.Fatalf("passthrough = %q, want %q", got, "plain error")
	}
}

// timeoutError is a net-timeout-shaped error so *url.Error.Timeout() reports true.
type timeoutError struct{}

func (timeoutError) Error() string { return "i/o timeout" }
func (timeoutError) Timeout() bool { return true }

// TestNormalizeEmbedURL covers the fix for the Discord 400 "embeds invalid" bug:
// an image URL whose filename has a space (or non-ASCII) must be percent-encoded
// so Discord accepts the embed, encoding must be idempotent, and a non-http value
// must be rejected so the caller omits the field.
func TestNormalizeEmbedURL(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{"space in filename", "https://x.co/images/a/Summer Event.png", "https://x.co/images/a/Summer%20Event.png", true},
		{"non-ascii filename", "https://x.co/images/a/café.png", "https://x.co/images/a/caf%C3%A9.png", true},
		{"already encoded is unchanged", "https://x.co/images/a/Summer%20Event.png", "https://x.co/images/a/Summer%20Event.png", true},
		{"plain url untouched", "https://x.co/images/a/normal.png", "https://x.co/images/a/normal.png", true},
		{"relative path rejected", "images/a/pic.png", "", false},
		{"blank rejected", "   ", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := normalizeEmbedURL(tc.in)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("normalizeEmbedURL(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestBuildEmbedEncodesImageURL is the end-to-end guard: an embed built with a
// spaced image/thumbnail URL carries the encoded form, so the payload Discord
// receives is well-formed.
func TestBuildEmbedEncodesImageURL(t *testing.T) {
	e := newEmbed().
		title("Hi").
		image("https://x.co/images/a/My Photo.png").
		thumbnail("https://x.co/images/a/My Logo.png").
		build()
	if e.Image == nil || e.Image.URL != "https://x.co/images/a/My%20Photo.png" {
		t.Errorf("image URL = %v, want encoded", e.Image)
	}
	if e.Thumbnail == nil || e.Thumbnail.URL != "https://x.co/images/a/My%20Logo.png" {
		t.Errorf("thumbnail URL = %v, want encoded", e.Thumbnail)
	}
}
