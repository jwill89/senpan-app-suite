package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"app-suite/internal/discord"
)

// TestWriteUpstreamErrorMapsDiscordFailures holds the status a failed Discord post is
// answered with, against a webhook server that fails in each way: never a 502.
func TestWriteUpstreamErrorMapsDiscordFailures(t *testing.T) {
	cases := []struct {
		name       string
		hook       http.HandlerFunc
		wantStatus int
		wantRetry  string
	}{
		{"refused", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }, http.StatusInternalServerError, ""},
		{"rate limited", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "4")
			w.WriteHeader(http.StatusTooManyRequests)
		}, http.StatusTooManyRequests, "4"},
		{"Discord down", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }, http.StatusServiceUnavailable, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hook := httptest.NewServer(tc.hook)
			defer hook.Close()
			err := (&discord.Sender{}).PostEmbed(context.Background(), discord.Target{Kind: "test"}, hook.URL, discord.Embed{Title: "Hi"})

			rec := httptest.NewRecorder()
			writeUpstreamError(rec, "test post", err)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d; want %d", rec.Code, tc.wantStatus)
			}
			if got := rec.Header().Get("Retry-After"); got != tc.wantRetry {
				t.Errorf("Retry-After = %q; want %q", got, tc.wantRetry)
			}
		})
	}
}
