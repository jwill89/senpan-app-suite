package server

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"
)

// runScheduler is the shared engine behind the background Discord-post
// schedulers (announcements, book-club events). It calls sweep once immediately
// - to catch up on anything that came due while the process was down - and then
// on every tick of interval, until ctx is cancelled. The schedulers differ only
// in their name, interval and sweep function, so each is a one-line wrapper
// around this. Safe to call in a goroutine; it returns when ctx is done.
//
// name identifies the scheduler in the log. Start and stop are logged at Info, so
// the log answers "was this thing even running?" - a scheduler that never started
// and one whose queue was empty are otherwise indistinguishable from the outside.
// Individual sweeps are Debug on purpose: at a 30s interval, Info would add ~2,880
// "nothing was due" lines a day and bury the ones that did something. The sweep
// functions log at Info when they actually act.
func runScheduler(ctx context.Context, name string, interval time.Duration, sweep func()) {
	slog.Info("scheduler started", "scheduler", name, "interval", interval.String())
	defer func() { slog.Info("scheduler stopped", "scheduler", name) }()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// A panic in a single sweep (e.g. one malformed announcement) must not kill the
	// long-lived scheduler goroutine - recover, log, and keep ticking.
	safeSweep := func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("scheduler sweep panicked", "scheduler", name, "panic", r, "stack", string(debug.Stack()))
			}
		}()
		start := time.Now()
		sweep()
		slog.Debug("scheduler swept", "scheduler", name, "duration_ms", time.Since(start).Milliseconds())
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
