package server

import (
	"io"
	"net/http"
	"net/http/httptest"
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

			if err := postDiscordWebhook(srv.URL, tc.payload); err != nil {
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
