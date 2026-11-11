package game

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"time"

	"app-suite/internal/bingo"
	"app-suite/internal/live"
)

// After a draw, and the automatic draws.
//
// One goroutine (Run) makes the automatic draws, so there is never more than one;
// it reads the interval afresh each time round, so a change takes effect at once.
// The draw delay holds back only when players get each number (afterDraw), never the
// staff's cadence.

// afterDraw tells everyone about a draw and does what follows from it: staff hear the
// number at once and players after delay seconds; a new winner stops the automatic
// draws; reaching the half-time mark asks staff about a mini-game. Shared by the
// manual draw and the automatic loop.
//
// Not logged per draw: a game is some seventy of them, which would bury the rest of
// the log. The start and end lines carry the game's shape, and a failed draw logs.
func (g *Service) afterDraw(ctx context.Context, res *DrawResult, newWinner bool, delay int) {
	gameID, number := res.Game.ID, res.Drawn
	g.hub.Publish(&DrawEvent{Envelope: live.Game(gameID), Payload: DrawPayload{Drawn: number, Winners: res.Winners}}, toStaff)
	tellPlayers := func() {
		g.hub.Publish(&PlayerDrawEvent{Envelope: live.Game(gameID), Payload: PlayerDrawPayload{Drawn: number}}, toPlayers)
	}
	if delay > 0 {
		// Up to a minute later: dropped if the game has ended (or another started)
		// in the meantime, so one game's number never reaches the next.
		time.AfterFunc(time.Duration(delay)*time.Second, func() {
			defer recoverPanic("delayed player draw event")
			if running, err := g.Running(ctx); err == nil && running == gameID {
				tellPlayers()
			}
		})
	} else {
		tellPlayers()
	}
	// A winner stops the automatic draws first, so a draw that both wins and reaches
	// half-time leaves them off with nothing to resume.
	if newWinner && g.DisableAuto() {
		g.autoChanged()
	}
	// Half-time: once, on the draw that reaches the mark.
	if len(res.Game.CalledNumbers) == bingo.HalftimeThreshold(grids(res.Game.Patterns)) {
		g.onHalftime(gameID, g.now().Add(time.Duration(delay)*time.Second))
	}
}

// AutoState is whether the automatic draws are on, and the seconds between them.
func (g *Service) AutoState() (enabled bool, interval int) {
	g.autoMu.Lock()
	defer g.autoMu.Unlock()
	return g.autoEnabled, g.autoInterval
}

// resetAuto sets a new game's automatic draws: on if it was started with them, at
// interval (held to range), and nothing for half-time to resume.
func (g *Service) resetAuto(enabled bool, interval int) {
	g.autoMu.Lock()
	g.autoEnabled, g.autoInterval, g.resumeAuto = enabled, bingo.ClampAutoInterval(interval), false
	g.autoMu.Unlock()
}

// setAuto switches the automatic draws on or off. Either way half-time has nothing to
// resume after: staff have chosen.
func (g *Service) setAuto(on bool) {
	g.autoMu.Lock()
	g.autoEnabled, g.resumeAuto = on, false
	g.autoMu.Unlock()
}

// enableAuto switches the automatic draws on, reporting whether this switched them
// from off: checked and set under one lock.
func (g *Service) enableAuto() (switched bool) {
	g.autoMu.Lock()
	defer g.autoMu.Unlock()
	switched = !g.autoEnabled
	g.autoEnabled, g.resumeAuto = true, false
	return switched
}

// DisableAuto switches the automatic draws off for good (a manual draw, a winner,
// the numbers running out, the game ending): half-time won't resume them. It reports
// whether they were on.
func (g *Service) DisableAuto() (wasOn bool) {
	g.autoMu.Lock()
	defer g.autoMu.Unlock()
	wasOn = g.autoEnabled
	g.autoEnabled, g.resumeAuto = false, false
	return wasOn
}

// pauseForHalftime switches running automatic draws off for the half-time decision,
// remembering to resume them if staff decline the mini-game. It reports whether it
// paused them.
func (g *Service) pauseForHalftime() (paused bool) {
	g.autoMu.Lock()
	defer g.autoMu.Unlock()
	if !g.autoEnabled {
		return false
	}
	g.autoEnabled, g.resumeAuto = false, true
	return true
}

// resume switches the automatic draws back on if half-time paused them, reporting
// whether it did.
func (g *Service) resume() bool {
	g.autoMu.Lock()
	defer g.autoMu.Unlock()
	if !g.resumeAuto {
		return false
	}
	g.autoEnabled, g.resumeAuto = true, false
	return true
}

// clearResume forgets that half-time paused the automatic draws: staff chose the
// mini-game, and switch them on again themselves when it is over.
func (g *Service) clearResume() {
	g.autoMu.Lock()
	g.resumeAuto = false
	g.autoMu.Unlock()
}

// autoChanged tells every console the automatic draws' state, so each one's switch
// and interval follow, and has the loop look again.
func (g *Service) autoChanged() {
	enabled, interval := g.AutoState()
	g.hub.Publish(&AutoDrawEvent{Payload: AutoDrawPayload{Enabled: enabled, Interval: interval}}, toStaff)
	g.wake()
}

// wake has the loop look again; it never blocks.
func (g *Service) wake() {
	select {
	case g.autoWake <- struct{}{}:
	default:
	}
}

// autoDrawOnce makes one automatic draw and what follows from it. When there is
// nothing to draw (the automatic draws were switched off a moment ago, the numbers
// ran out, the game ended) it makes sure they are off, so the loop parks.
func (g *Service) autoDrawOnce(ctx context.Context) {
	defer recoverPanic("auto draw")
	delay := g.DrawDelay(ctx)
	res, newWinner, err := g.drawAuto(ctx, delay)
	switch {
	case err != nil && ctx.Err() != nil:
		return // the server is stopping
	case err != nil && !errors.Is(err, ErrNoGame) && !errors.Is(err, ErrAllDrawn):
		slog.Error("auto draw failed", "error", err)
		return
	case res == nil:
		if g.DisableAuto() {
			g.autoChanged()
		}
		return
	}
	// The number is drawn: it reaches players, and what follows from it happens,
	// even if the loop is being stopped.
	g.afterDraw(context.WithoutCancel(ctx), res, newWinner, delay)
}

// Run makes the automatic draws until ctx ends: the first the moment they are
// switched on, then one every interval. It parks while they are off. The server
// starts it once, with a context cancelled on shutdown, so it stops before the
// database closes.
func (g *Service) Run(ctx context.Context) {
	defer recoverPanic("auto draw scheduler")
	slog.Info("scheduler started", "scheduler", "auto-draw")
	defer slog.Info("scheduler stopped", "scheduler", "auto-draw")
	timer := time.NewTimer(time.Hour)
	stopTimer(timer)
	defer timer.Stop()
	for {
		enabled, interval := g.AutoState()
		switch {
		case enabled && g.autoDrawNow.Swap(false):
			// Just switched on: the first number now, then the interval.
			g.autoDrawOnce(ctx)
			continue
		case enabled:
			stopTimer(timer)
			timer.Reset(time.Duration(bingo.ClampAutoInterval(interval)) * time.Second)
		default:
			// autoDrawNow is left alone: switching them off clears it (Change), and
			// clearing it here would race a switch back on.
			stopTimer(timer)
		}
		select {
		case <-ctx.Done():
			return
		case <-g.autoWake:
		case <-timer.C:
			g.autoDrawOnce(ctx)
		}
	}
}

// stopTimer stops t and drains a tick that fired unread, so a stale one can't draw.
func stopTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}

// recoverPanic keeps a panic in a timer or the loop from taking the server down: it
// is logged, with where it happened.
func recoverPanic(where string) {
	if r := recover(); r != nil {
		slog.Error("recovered from panic", "where", where, "panic", r, "stack", string(debug.Stack()))
	}
}
