package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
)

// interruptedQueryErr returns the error a real query gives when its context is
// canceled while it runs - as opposed to before it starts, which database/sql
// catches itself and reports as context.Canceled. The query cancels its own
// context from a SQL function on its first row, so the interrupt lands mid-flight
// every time rather than depending on a race with a timer.
func interruptedQueryErr(t *testing.T) error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	db, err := driver.Open(filepath.Join(t.TempDir(), "interrupt.db"), func(c *sqlite3.Conn) error {
		return c.CreateFunction("cancel_query", 0, 0, func(sqlite3.Context, ...sqlite3.Value) { cancel() })
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	err = db.QueryRowContext(ctx, `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c WHERE x < 1000000)
		SELECT count(cancel_query()) FROM c`).Scan(&n)
	if err == nil {
		t.Fatal("the query ran to the end; it was meant to be interrupted")
	}
	return err
}

// TestWriteInternalErrorCanceledLogsAtDebug pins how a failed request is logged.
// A query refused or stopped because the request's context was canceled means the
// client went away: that is logged at Debug, since there is nobody to tell and an
// ERROR line per closed tab would bury real failures. Everything else still logs at
// Error. The response is the same 500 either way.
func TestWriteInternalErrorCanceledLogsAtDebug(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	cases := []struct {
		name  string
		err   error
		level string
	}{
		{"refused before it ran", context.Canceled, "DEBUG"},
		{"wrapped on the way up", fmt.Errorf("list cards: %w", context.Canceled), "DEBUG"},
		// What a client disconnecting mid-query usually produces: the driver's
		// interrupt, which errors.Is(err, context.Canceled) alone does not see.
		{"interrupted mid-query", interruptedQueryErr(t), "DEBUG"},
		{"a real failure", errors.New("disk I/O error"), "ERROR"},
		{"a deadline is not a departed client", context.DeadlineExceeded, "ERROR"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf.Reset()
			rec := httptest.NewRecorder()
			writeInternalError(rec, "list cards", c.err)

			if rec.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500", rec.Code)
			}
			if body := rec.Body.String(); !strings.Contains(body, `"error":"Internal server error"`) {
				t.Errorf("body = %s, want the generic internal error", body)
			}
			var line map[string]any
			if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
				t.Fatalf("want exactly one JSON log line, got %q: %v", buf.String(), err)
			}
			if line["level"] != c.level {
				t.Errorf("level = %v, want %s (line: %s)", line["level"], c.level, buf.String())
			}
			if line["context"] != "list cards" {
				t.Errorf("context = %v, want the call site's label", line["context"])
			}
		})
	}
}
