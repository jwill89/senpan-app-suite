package server_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"app-suite/internal/server"
)

// sampleLog is a few slog-style JSON lines plus a blank and a malformed line,
// mirroring what the rotating log file actually contains.
const sampleLog = `{"time":"2026-07-05T19:14:23Z","level":"INFO","msg":"http request","method":"GET","path":"/api/version","status":200}
not json at all

{"time":"2026-07-05T19:14:30Z","level":"ERROR","msg":"http request","path":"/api/bookclub/lookup","status":424}
{"time":"2026-07-05T19:15:00Z","level":"WARN","msg":"auth failed","ip":"[::1]:5000"}`

func TestParseLogLines_ParsesSkipsAndPromotes(t *testing.T) {
	entries := server.ParseLogLinesForTest([]byte(sampleLog), 0, false, "")
	// 3 valid JSON objects; the blank and "not json" lines are skipped.
	if len(entries) != 3 {
		t.Fatalf("got %d entries; want 3 (blank + malformed skipped)", len(entries))
	}
	first := entries[0]
	if first.Level != "INFO" || first.Message != "http request" || first.Time == "" {
		t.Errorf("promotion wrong: %+v", first)
	}
	// time/level/msg must NOT leak into Fields; other attrs must.
	if _, ok := first.Fields["msg"]; ok {
		t.Error("msg leaked into Fields")
	}
	if first.Fields["path"] != "/api/version" {
		t.Errorf("path field = %v; want /api/version", first.Fields["path"])
	}
}

func TestParseLogLines_LevelFilter(t *testing.T) {
	// Minimum level WARN(4) keeps the ERROR and WARN lines, drops INFO.
	warnLvl, ok := server.SlogLevelValueForTest("warn")
	if !ok || warnLvl != 4 {
		t.Fatalf("SlogLevelValue(warn) = (%d,%v); want (4,true)", warnLvl, ok)
	}
	entries := server.ParseLogLinesForTest([]byte(sampleLog), warnLvl, true, "")
	if len(entries) != 2 {
		t.Fatalf("got %d entries; want 2 (>= WARN)", len(entries))
	}
	for _, e := range entries {
		if e.Level == "INFO" {
			t.Errorf("INFO entry survived the WARN filter: %+v", e)
		}
	}
}

func TestParseLogLines_TextQuery(t *testing.T) {
	// Case-insensitive substring match against the whole line.
	entries := server.ParseLogLinesForTest([]byte(sampleLog), 0, false, "lookup")
	if len(entries) != 1 || entries[0].Fields["path"] != "/api/bookclub/lookup" {
		t.Fatalf("query 'lookup' = %+v; want the single lookup line", entries)
	}
}

func TestLogLevelName_RoundTrip(t *testing.T) {
	for _, name := range []string{"debug", "info", "warn", "error"} {
		lvl, ok := server.ParseLogLevelNameForTest(name)
		if !ok {
			t.Fatalf("ParseLogLevelName(%q) not ok", name)
		}
		if got := server.LogLevelNameForTest(lvl); got != name {
			t.Errorf("round-trip %q → %v → %q", name, lvl, got)
		}
	}
	if _, ok := server.ParseLogLevelNameForTest("nonsense"); ok {
		t.Error("ParseLogLevelName(nonsense) should be !ok")
	}
	// slog.LevelWarn is the numeric boundary the name mapping keys off.
	if server.LogLevelNameForTest(slog.LevelWarn) != "warn" {
		t.Error("LevelWarn should name to warn")
	}
}

func TestLogClientIP_PrefersCloudflareHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	r.RemoteAddr = "[::1]:44686"
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 172.71.1.1")
	r.Header.Set("CF-Connecting-IP", "198.51.100.7")
	// CF-Connecting-IP wins over XFF and RemoteAddr.
	if got := server.LogClientIPForTest(r); got != "198.51.100.7" {
		t.Errorf("with CF-Connecting-IP: got %q; want 198.51.100.7", got)
	}
	// Without it, the leftmost XFF (original client) is used.
	r.Header.Del("CF-Connecting-IP")
	if got := server.LogClientIPForTest(r); got != "203.0.113.9" {
		t.Errorf("XFF leftmost: got %q; want 203.0.113.9", got)
	}
	// With neither, the RemoteAddr host (port stripped).
	r.Header.Del("X-Forwarded-For")
	if got := server.LogClientIPForTest(r); got != "::1" {
		t.Errorf("RemoteAddr host: got %q; want ::1", got)
	}
}
