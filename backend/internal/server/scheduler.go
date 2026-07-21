package server

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"
)

// runScheduler is the shared engine behind the background Discord-post
// schedulers (announcements, book-club events). It calls sweep once immediately
// — to catch up on anything that came due while the process was down — and then
// on every tick of interval, until ctx is cancelled. The schedulers differ only
// in their interval and sweep function, so each is a one-line wrapper around
// this. Safe to call in a goroutine; it returns when ctx is done.
func runScheduler(ctx context.Context, interval time.Duration, sweep func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// A panic in a single sweep (e.g. one malformed announcement) must not kill the
	// long-lived scheduler goroutine — recover, log, and keep ticking.
	safeSweep := func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("scheduler sweep panicked", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		sweep()
	}
	safeSweep() // sweep immediately on startup (catch up after downtime)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			safeSweep()
		}
	}
}
