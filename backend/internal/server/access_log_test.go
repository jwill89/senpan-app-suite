package server_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

// TestAccessLog_IncludesActor verifies the request log names who made each
// request — an authenticated account (cookie session), a Cloudflare-verified
// bot, or anonymous — exercising the actor-through-context wiring end to end.
func TestAccessLog_IncludesActor(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	env := newTestEnv(t)
	env.loginAdmin(t)

	// Authenticated admin (cookie session): auth=session + the username.
	buf.Reset()
	resp := env.get(t, "/api/settings")
	resp.Body.Close()
	if out := buf.String(); !strings.Contains(out, `"auth":"session"`) || !strings.Contains(out, `"user":"admin"`) {
		t.Errorf("authenticated request: want auth=session + user=admin in log; got:\n%s", out)
	}

	// A client that trusts the test cert but carries NO cookie jar → anonymous.
	// (env.ts.Client() returns the shared client that already holds the admin
	// session cookie, so reuse only its transport.)
	anon := &http.Client{Transport: env.ts.Client().Transport}

	buf.Reset()
	if r, err := anon.Get(env.ts.URL + "/api/version"); err == nil {
		r.Body.Close()
	}
	if out := buf.String(); !strings.Contains(out, `"auth":"anon"`) {
		t.Errorf("anonymous request: want auth=anon in log; got:\n%s", out)
	}

	// Cloudflare-verified bot (custom transform-rule header) → auth=bot + name.
	buf.Reset()
	req, _ := http.NewRequest(http.MethodGet, env.ts.URL+"/api/version", nil)
	req.Header.Set("X-Verified-Bot", "true")
	req.Header.Set("User-Agent", "Googlebot/2.1")
	if r, err := anon.Do(req); err == nil {
		r.Body.Close()
	}
	if out := buf.String(); !strings.Contains(out, `"auth":"bot"`) || !strings.Contains(out, "Googlebot") {
		t.Errorf("verified-bot request: want auth=bot + Googlebot in log; got:\n%s", out)
	}
}
