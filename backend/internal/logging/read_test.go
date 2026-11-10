package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// sampleLog is a few slog-style JSON lines with a blank and a malformed one, as the
// rotating log file holds them.
const sampleLog = `{"time":"2026-07-05T19:14:23Z","level":"INFO","msg":"http request","method":"GET","path":"/api/v1/version","status":200}
not json at all

{"time":"2026-07-05T19:14:30Z","level":"ERROR","msg":"http request","path":"/api/bookclub/lookup","status":424}
{"time":"2026-07-05T19:15:00Z","level":"WARN","msg":"auth failed","ip":"[::1]:5000"}
`

func sampleFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "senpan.log")
	if err := os.WriteFile(path, []byte(sampleLog), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Read parses each JSON line, skips the rest, lifts time, level and msg out of the
// fields, and returns the newest first.
func TestReadParsesSkipsAndPromotes(t *testing.T) {
	entries, truncated, err := Read(sampleFile(t), Filter{})
	if err != nil || truncated {
		t.Fatalf("Read: %v, truncated %v", err, truncated)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries; want 3 (the blank and malformed lines skipped)", len(entries))
	}
	newest, oldest := entries[0], entries[2]
	if newest.Level != "WARN" || oldest.Level != "INFO" || oldest.Message != "http request" || oldest.Time == "" {
		t.Errorf("entries out of order or not promoted: newest %+v, oldest %+v", newest, oldest)
	}
	if _, ok := oldest.Fields["msg"]; ok {
		t.Error("msg leaked into Fields")
	}
	if oldest.Fields["path"] != "/api/v1/version" {
		t.Errorf("path field = %v; want /api/v1/version", oldest.Fields["path"])
	}
}

// A level keeps it and those above it; text matches the whole line in any case; a
// limit keeps the newest.
func TestReadFilters(t *testing.T) {
	path := sampleFile(t)
	if entries, _, _ := Read(path, Filter{Level: "warn"}); len(entries) != 2 {
		t.Errorf("warn and above: %d entries; want 2", len(entries))
	}
	if entries, _, _ := Read(path, Filter{Query: "LOOKUP"}); len(entries) != 1 || entries[0].Fields["path"] != "/api/bookclub/lookup" {
		t.Errorf("query LOOKUP: %+v; want the lookup line", entries)
	}
	if entries, _, _ := Read(path, Filter{Limit: 1}); len(entries) != 1 || entries[0].Level != "WARN" {
		t.Errorf("limit 1: %+v; want the newest", entries)
	}
}

// A log file that doesn't exist yet reads as empty, not as an error.
func TestReadMissingFileIsEmpty(t *testing.T) {
	entries, _, err := Read(filepath.Join(t.TempDir(), "none.log"), Filter{})
	if err != nil || len(entries) != 0 {
		t.Fatalf("missing file: %v, %d entries", err, len(entries))
	}
}

func TestLevelNamesRoundTrip(t *testing.T) {
	for _, name := range []string{"debug", "info", "warn", "error"} {
		lvl, ok := LevelValue(name)
		if !ok || LevelName(lvl) != name {
			t.Errorf("round trip %q -> %v (%v) -> %q", name, lvl, ok, LevelName(lvl))
		}
	}
	if _, ok := LevelValue("nonsense"); ok {
		t.Error(`LevelValue("nonsense") should not be a level`)
	}
	if LevelName(slog.LevelWarn) != "warn" {
		t.Error("slog.LevelWarn should be named warn")
	}
}
