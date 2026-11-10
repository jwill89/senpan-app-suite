package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"modernc.org/sqlite"
)

// cancel_query(id) cancels the context stored under id. The driver's functions are
// process-wide, so it is registered once and finds each query's cancel by the id
// the query passes it.
var (
	registerCancelQuery sync.Once
	cancelQueries       sync.Map // int64 -> context.CancelFunc
	cancelQueryIDs      atomic.Int64
)

// interruptedQueryErr returns the error a real query gives when its context is
// canceled while it runs - as opposed to before it starts, which database/sql
// catches itself and reports as context.Canceled. The query cancels its own
// context from a SQL function on its first row, so the interrupt lands mid-flight
// every time rather than depending on a race with a timer.
func interruptedQueryErr(t *testing.T) error {
	t.Helper()
	registerCancelQuery.Do(func() {
		sqlite.MustRegisterScalarFunction("cancel_query", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if cancel, ok := cancelQueries.Load(args[0]); ok {
				cancel.(context.CancelFunc)()
			}
			return nil, nil
		})
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	id := cancelQueryIDs.Add(1)
	cancelQueries.Store(id, cancel)
	defer cancelQueries.Delete(id)

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "interrupt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	err = db.QueryRowContext(ctx, `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c WHERE x < 1000000)
		SELECT count(cancel_query(?)) FROM c`, id).Scan(&n)
	if err == nil {
		t.Fatal("the query ran to the end; it was meant to be interrupted")
	}
	return err
}

// TestErrorLevelCanceledIsDebug pins how a failure is logged. A query refused or
// stopped because the work's context was canceled means the client went away (or the
// process is shutting down): that is Debug, since there is nobody to tell and an
// ERROR line per closed tab would bury real failures. Everything else is Error.
func TestErrorLevelCanceledIsDebug(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want slog.Level
	}{
		{"refused before it ran", context.Canceled, slog.LevelDebug},
		{"wrapped on the way up", fmt.Errorf("list cards: %w", context.Canceled), slog.LevelDebug},
		// What a client disconnecting mid-query usually produces: the driver
		// interrupting the query part-way, whatever error it reports that with.
		{"interrupted mid-query", interruptedQueryErr(t), slog.LevelDebug},
		{"a real failure", errors.New("disk I/O error"), slog.LevelError},
		{"a deadline is not a departed client", context.DeadlineExceeded, slog.LevelError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := errorLevel(c.err); got != c.want {
				t.Errorf("errorLevel = %v, want %v", got, c.want)
			}
		})
	}
}
