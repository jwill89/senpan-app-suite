package server_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"app-suite/internal/server"
	"app-suite/internal/store"
	"app-suite/internal/ws"
)

// A sign-in outlives a restart, since sessions live in the database, and the
// database holds only a hash of the session's token, never the cookie's value.
func TestSessionsOutliveARestartAndAreStoredHashed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	start := func() *testEnv {
		st, err := store.New(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}
		ts := httptest.NewTLSServer(server.New(t.Context(), st, ws.NewHub(), t.TempDir(), nil))
		jar, _ := cookiejar.New(nil)
		client := ts.Client()
		client.Jar = jar
		return &testEnv{ts: ts, client: client, store: st}
	}

	first := start()
	first.loginAdmin(t)
	at, _ := url.Parse(first.ts.URL)
	cookies := first.client.Jar.Cookies(at)
	if len(cookies) == 0 {
		t.Fatal("signing in set no cookie")
	}
	var token string
	for _, c := range cookies {
		if c.Name == "session" {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatalf("no session cookie among %v", cookies)
	}

	var stored []string
	rows, err := first.store.DB().QueryContext(t.Context(), `SELECT token FROM sessions`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, s)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(token))
	if len(stored) != 1 || stored[0] != hex.EncodeToString(sum[:]) {
		t.Fatalf("sessions table holds %v; want only the SHA-256 of the cookie's token", stored)
	}

	first.ts.Close()
	if err := first.store.Close(); err != nil {
		t.Fatal(err)
	}

	second := start()
	t.Cleanup(func() { second.ts.Close(); second.store.Close() })
	again, _ := url.Parse(second.ts.URL)
	second.client.Jar.SetCookies(again, cookies)
	resp := second.get(t, "/api/auth")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/auth after the restart: %d", resp.StatusCode)
	}
	if body := decodeBody(t, resp); body["authenticated"] != true {
		t.Fatalf("the sign-in did not survive the restart: %v", body)
	}
}
