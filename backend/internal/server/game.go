package server

import (
	"app-suite/internal/bingo"
	"app-suite/internal/model"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// recoverPanic is a deferred guard for background goroutines and timer callbacks
// (auto-draw loop, delayed player broadcasts): it turns a panic into a logged
// error instead of letting it crash the whole process. `where` names the site.
func recoverPanic(where string) {
	if r := recover(); r != nil {
		slog.Error("recovered from panic", "where", where, "panic", r, "stack", string(debug.Stack()))
	}
}

// currentGameID returns the id of the live game, or 0 when there is none. Used to
// discard a deferred broadcast whose game has since ended (and possibly been
// replaced by a new game) so game A's number/alert can't leak into game B.
func (s *Server) currentGameID() int64 {
	state, _, err := s.game.CurrentState()
	if err != nil || state == nil {
		return 0
	}
	return state.ID
}

// handleGameState returns the current game state and cached winners.
//
//	Endpoint:  GET /api/game
//	Auth:      public
//	Response:  {"game": GameState|null, "winners": [...], "game_details": "..."}
func (s *Server) handleGameState(w http.ResponseWriter, r *http.Request) {
	state, winners, err := s.game.CurrentState()
	if err != nil {
		writeInternalError(w, "get game state", err)
		return
	}
	details, _ := s.game.GameDetails()
	writeJSON(w, http.StatusOK, model.GameStateResponse{
		Game:        state,
		Winners:     winners,
		GameDetails: details,
	})
}

// gameStartRequest is the JSON body for POST /api/game/start.
type gameStartRequest struct {
	PatternIDs []int `json:"pattern_ids"` // pattern IDs to use when starting a game
	// Auto starts the game with the automatic-draw loop running; AutoInterval is
	// the seconds between draws (the "Time Between Calls" setting). Both usually
	// come from the New Game form or a preset the admin applied.
	Auto         bool `json:"auto"`
	AutoInterval int  `json:"auto_interval"`
}

// handleGameStart starts a new game with the selected win patterns.
//
//	Endpoint:    POST /api/game/start
//	Auth:        permission:bingo-game
//	Request:     {"pattern_ids": [...], "auto": false, "auto_interval": 30}
//	Response:    200 GameStateResponse
//	Broadcasts:  game_update (start)
func (s *Server) handleGameStart(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoGame) {
		return
	}
	req, err := readJSON[gameStartRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if len(req.PatternIDs) == 0 {
		writeError(w, http.StatusBadRequest, "Select at least one pattern")
		return
	}
	game, err := s.game.Start(req.PatternIDs, req.Auto, req.AutoInterval)
	if err != nil {
		writeInternalError(w, "start game", err)
		return
	}
	details, _ := s.game.GameDetails()
	writeJSON(w, http.StatusOK, model.GameStateResponse{
		Game:        game,
		Winners:     []string{},
		GameDetails: details,
	})
	s.broadcastGameStart(game, details)
	// Wake the auto-draw scheduler so it (re)arms for the new game — or stays idle
	// when the game started manual. Starting with auto on draws the first number
	// immediately (then the interval cadence).
	if req.Auto {
		s.requestImmediateAutoDraw()
	} else {
		s.signalAutoWake()
	}
}

// gameDrawRequest is the JSON body for POST /api/game/draw.
type gameDrawRequest struct {
	Delay int `json:"delay"` // seconds to delay player broadcast (0 = instant)
}

// handleGameDraw draws the next number.
//
//	Endpoint:    POST /api/game/draw
//	Auth:        permission:bingo-game
//	Request:     {"delay": 0}
//	Response:    200 DrawResult
//	Broadcasts:  game_draw (to admins immediately; to players delayed/immediate),
//	             plus auto_config / halftime_prompt as post-draw effects fire
func (s *Server) handleGameDraw(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoGame) {
		return
	}
	req, err := readJSON[gameDrawRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// A manual draw takes over from the auto loop: switch auto off first (and wake
	// the scheduler so it cancels its pending draw) so the two can never both fire.
	// Done before the draw so the timer is cancelled the instant the admin takes
	// control; DrawAuto's under-lock guard covers the microscopic overlap.
	if s.game.DisableAuto() {
		s.broadcastAutoConfig()
		s.signalAutoWake()
	}

	result, newWinner, err := s.game.Draw()
	if err != nil {
		writeInternalError(w, "draw number", err)
		return
	}
	if result == nil {
		writeError(w, http.StatusBadRequest, "No active game or all callable numbers have been drawn")
		return
	}
	writeJSON(w, http.StatusOK, result)

	// A manual draw uses the delay carried on the request (a per-click choice); the
	// shared post-draw logic broadcasts it and applies the winner/half-time effects.
	s.postDraw(result, newWinner, clampDrawDelay(req.Delay))
}

// clampDrawDelay coerces a requested player draw delay into the supported 0–60s.
func clampDrawDelay(delay int) int {
	return min(max(delay, 0), 60)
}

// postDraw broadcasts a completed draw and applies its side-effects, shared by the
// manual draw handler and the automatic-draw loop. Admins see the number
// immediately; players see it after `delay` seconds. A new winner switches the
// auto loop off, and crossing the half-time mark pauses auto (if running) and
// prompts admins for a mini-game.
func (s *Server) postDraw(result *bingo.DrawResult, newWinner bool, delay int) {
	// Capture the game this draw belongs to. The delayed player broadcast below can
	// fire up to 60s later; if the game is ended and a new one started within that
	// window, the deferred closure must NOT leak game A's number into game B.
	gameID := result.Game.ID

	// Admins immediately (keeps every admin surface in sync).
	s.broadcastDrawToAdmins(result.Drawn, result.Winners)

	// Players: delayed or immediate. The moment the number reaches players is used
	// below to hold a half-time alert until they've seen the triggering number.
	drawn := result.Drawn
	if delay > 0 {
		time.AfterFunc(time.Duration(delay)*time.Second, func() {
			defer recoverPanic("delayed player draw broadcast")
			// Drop the broadcast if the game it belongs to is no longer the live
			// game (ended, or ended+restarted within the delay window).
			if s.currentGameID() != gameID {
				return
			}
			s.broadcastDrawToPlayers(drawn)
		})
	} else {
		s.broadcastDrawToPlayers(drawn)
	}

	// A winner ends the auto loop straight away (before the half-time check, so a
	// draw that both wins and crosses the midpoint leaves auto off with no resume).
	if newWinner {
		if s.game.DisableAuto() {
			s.broadcastAutoConfig()
			s.signalAutoWake()
		}
	}

	// Half-time: fire once, exactly on the crossing draw.
	if len(result.Game.CalledNumbers) == bingo.HalftimeThreshold(result.Game.Patterns) {
		s.onHalftime(time.Now().Add(time.Duration(delay) * time.Second))
	}
}

// gameEndRequest is the JSON body for POST /api/game/end.
type gameEndRequest struct {
	ValidWinnerIDs []string `json:"valid_winner_ids"` // card IDs confirmed as valid winners on end
}

// handleGameEnd ends the active game, logging the confirmed valid winners.
//
//	Endpoint:    POST /api/game/end
//	Auth:        permission:bingo-game
//	Request:     {"valid_winner_ids": [...]}
//	Response:    200 EndGameResponse
//	Broadcasts:  game_update (end)
func (s *Server) handleGameEnd(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoGame) {
		return
	}
	req, err := readJSON[gameEndRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Before ending, gather game info for winners log
	var patternNames []string
	if state, _, err := s.game.CurrentState(); err == nil && state != nil {
		for _, p := range state.Patterns {
			patternNames = append(patternNames, p.Name)
		}
	}
	gameDetails, _ := s.game.GameDetails()

	ok, err := s.game.End()
	if err != nil {
		writeInternalError(w, "end game", err)
		return
	}

	// Log valid winners
	if ok && len(req.ValidWinnerIDs) > 0 {
		patternsJSON, _ := json.Marshal(patternNames)
		playerNames, _ := s.store.GetCardPlayerNames(req.ValidWinnerIDs)
		entries := make([]model.WinnersLogEntry, 0, len(req.ValidWinnerIDs))
		for _, cardID := range req.ValidWinnerIDs {
			entries = append(entries, model.WinnersLogEntry{
				CardID:          cardID,
				PlayerName:      playerNames[cardID],
				GameDetails:     gameDetails,
				WinningPatterns: string(patternsJSON),
			})
		}
		_ = s.store.InsertWinnersLog(entries)
	}

	writeJSON(w, http.StatusOK, model.EndGameResponse{Ended: ok})
	s.broadcastGameEnd()
	// End() switched auto off; wake the scheduler so it parks its timer.
	s.signalAutoWake()
}

// gameHalftimeRequest is the JSON body for POST /api/game/halftime. `minigame`
// records the admin's answer to the half-time prompt: true alerts players about a
// mini-game (and leaves auto paused); false declines it and resumes auto if it was
// paused for the prompt. An empty body defaults to true, preserving the original
// "trigger the alert" behaviour for older clients.
type gameHalftimeRequest struct {
	Minigame *bool `json:"minigame"`
}

// handleGameHalftime records the admin's half-time decision.
//
//	Endpoint:    POST /api/game/halftime
//	Auth:        permission:bingo-game
//	Request:     {"minigame": true|false}
//	Response:    200 {"ok": true}
//	Broadcasts:  halftime_minigame (players; when minigame=true, held until the
//	             triggering number has reached them), auto_config (on resume)
func (s *Server) handleGameHalftime(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoGame) {
		return
	}
	// Tolerate an absent/empty body (older clients POST nothing) → default to
	// alerting players about a mini-game.
	req, _ := readJSON[gameHalftimeRequest](w, r)
	minigame := req.Minigame == nil || *req.Minigame

	if minigame {
		// Alert players, but not before the triggering number has reached them
		// (auto stays paused; the admin re-enables it manually when ready).
		s.game.ClearHalftimeResume() // the choice is "mini-game" — don't auto-resume
		s.broadcastHalftimeMinigameWhenReady()
	} else if s.game.ResumeAutoAfterHalftime() {
		// No mini-game: switch auto back on if it was paused for the prompt.
		s.broadcastAutoConfig()
		s.signalAutoWake()
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// yoeverRequest is the JSON body for POST /api/game/yoever.
type yoeverRequest struct {
	CardID string `json:"card_id"` // the triggering player's board id
}

// defaultYoeverCooldownSeconds is the anti-spam window between a card's
// "It's Yoever" triggers when the admin hasn't configured one.
const defaultYoeverCooldownSeconds = 180

// yoeverCooldown returns the configured per-card cooldown between reaction
// triggers (yoever_cooldown_seconds), clamped to a non-negative duration.
func (s *Server) yoeverCooldown() time.Duration {
	secs := s.getSettingInt("yoever_cooldown_seconds", defaultYoeverCooldownSeconds)
	if secs < 0 {
		secs = 0
	}
	return time.Duration(secs) * time.Second
}

// handleGameYoever lets a player trigger the shared "It's Yoever" reaction — a
// sound + a bouncing image with the player's name, broadcast to every connected
// client. It is public (any player holding a valid board id), but each card is
// throttled to one trigger per yoever_cooldown_seconds and the whole feature can
// be switched off by an admin.
//
//	Endpoint:    POST /api/game/yoever
//	Auth:        public
//	Request:     {"card_id": "ABC123"}
//	Response:    200 YoeverResponse (ok, count, cooldown_seconds)
//	             400 bad id, 403 disabled, 404 unknown card, 409 no active game,
//	             429 cooldown ({"error","retry_after"} + Retry-After header)
//	Broadcasts:  yoever (to all clients)
func (s *Server) handleGameYoever(w http.ResponseWriter, r *http.Request) {
	req, err := readJSON[yoeverRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	cardID := strings.ToUpper(strings.TrimSpace(req.CardID))
	if cardID == "" {
		writeError(w, http.StatusBadRequest, "Board ID is required")
		return
	}

	// Resolve the triggering player's name for the broadcast label; this also
	// validates the board id is real (players connect with it but never prove it).
	card, err := s.store.GetCard(cardID)
	if err != nil {
		writeInternalError(w, "get card", err)
		return
	}
	if card == nil {
		writeError(w, http.StatusNotFound, "Board not found")
		return
	}

	cooldown := s.yoeverCooldown()
	count, retryAfter, err := s.game.TriggerYoever(cardID, time.Now(), cooldown)
	switch {
	case errors.Is(err, bingo.ErrYoeverNoGame):
		writeError(w, http.StatusConflict, "No game is currently active")
		return
	case errors.Is(err, bingo.ErrYoeverDisabled):
		writeError(w, http.StatusForbidden, "It's Yoever is currently switched off")
		return
	case err != nil:
		writeInternalError(w, "trigger yoever", err)
		return
	}
	if retryAfter > 0 {
		// Round the wait up to whole seconds so a sub-second remainder still reads
		// as "1s", never "0s".
		secs := int(retryAfter.Seconds())
		if time.Duration(secs)*time.Second < retryAfter {
			secs++
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeJSON(w, http.StatusTooManyRequests, struct {
			Error      string `json:"error"`
			RetryAfter int    `json:"retry_after"`
		}{Error: "You just did that — give it a moment.", RetryAfter: secs})
		return
	}

	// Announce to everyone (players + admins): play the sound + bounce the image
	// labelled with this player's name, and update the admin "Yoevers: N" counter.
	s.hub.Broadcast(struct {
		Type       string `json:"type"`
		PlayerName string `json:"player_name"`
		Count      int    `json:"count"`
	}{Type: "yoever", PlayerName: card.PlayerName, Count: count})

	writeJSON(w, http.StatusOK, model.YoeverResponse{
		OK:              true,
		Count:           count,
		CooldownSeconds: int(cooldown.Seconds()),
	})
}

// gamePatchRequest is the JSON body for PATCH /api/game. Every field is a pointer
// so an absent field ("not being changed") is distinguishable from a zero value:
//   - delay present          → validate 0–60, persist default_draw_delay, broadcast draw_delay_update
//   - details present        → set game details, broadcast details_update
//   - yoever_enabled present → toggle the "It's Yoever" reaction, broadcast yoever_config
//   - auto_enabled present   → switch the automatic-draw loop on/off, broadcast auto_config
//   - auto_interval present  → adjust the seconds between auto draws, broadcast auto_config
type gamePatchRequest struct {
	Delay         *int    `json:"delay"`
	Details       *string `json:"details"`
	YoeverEnabled *bool   `json:"yoever_enabled"`
	AutoEnabled   *bool   `json:"auto_enabled"`
	AutoInterval  *int    `json:"auto_interval"`
}

// handleGamePatch partially updates the shared game controls (draw delay and/or
// game details), broadcasting each change so every admin's controls update live.
//
//	Endpoint:    PATCH /api/game
//	Auth:        permission:bingo-game
//	Request:     {"delay"?: 0, "details"?: "..."}
//	Response:    200 {"ok": true}
//	Broadcasts:  draw_delay_update (delay), details_update (details)
func (s *Server) handleGamePatch(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoGame) {
		return
	}
	req, err := readJSON[gamePatchRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// The draw delay is a shared game control: persist the caller's choice (so it
	// survives page loads — admins read it as default_draw_delay) and broadcast it
	// so every other admin's selector updates live.
	if req.Delay != nil {
		delay := *req.Delay
		if delay < 0 || delay > 60 {
			writeError(w, http.StatusBadRequest, "Draw delay must be 0–60")
			return
		}
		if err := s.store.SetSetting("default_draw_delay", strconv.Itoa(delay)); err != nil {
			writeInternalError(w, "save draw delay", err)
			return
		}
		s.hub.Broadcast(struct {
			Type  string `json:"type"`
			Delay int    `json:"delay"`
		}{Type: "draw_delay_update", Delay: delay})
	}

	if req.Details != nil {
		if err := s.game.SetGameDetails(*req.Details); err != nil {
			writeInternalError(w, "update game details", err)
			return
		}
		// Broadcast the updated details to all connected clients
		s.hub.Broadcast(struct {
			Type    string `json:"type"`
			Details string `json:"game_details"`
		}{Type: "details_update", Details: *req.Details})
	}

	// The "It's Yoever" on/off switch is a shared, per-game admin control: flip it
	// and tell every client so players' trigger button shows/hides live and other
	// admins' toggle stays in step.
	if req.YoeverEnabled != nil {
		s.game.SetYoeverEnabled(*req.YoeverEnabled)
		s.hub.Broadcast(struct {
			Type    string `json:"type"`
			Enabled bool   `json:"enabled"`
		}{Type: "yoever_config", Enabled: *req.YoeverEnabled})
	}

	// Automatic-draw controls are shared, per-game admin controls too. Apply the
	// interval first so an enable in the same PATCH arms with the new value, then
	// broadcast the resulting state and wake the scheduler so it re-arms or stops.
	if req.AutoInterval != nil {
		s.game.SetAutoInterval(*req.AutoInterval)
	}
	if req.AutoEnabled != nil {
		if *req.AutoEnabled {
			// Turning auto on: EnableAutoOnce does the check-and-set under one lock
			// and reports a genuine off→on flip, so concurrent enables can't each
			// arm a redundant immediate first draw (the earlier get-then-set TOCTOU).
			if transitioned, _ := s.game.EnableAutoOnce(); transitioned {
				s.autoDrawNow.Store(true)
			}
		} else {
			s.game.SetAutoEnabled(false)
		}
	}
	if req.AutoInterval != nil || req.AutoEnabled != nil {
		s.broadcastAutoConfig()
		s.signalAutoWake()
	}

	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// ── Automatic-draw loop ──────────────────────────────────────────────────────
//
// The auto loop lives here (not in the bingo service) because each draw is a
// broadcast side-effect: it reuses the exact same post-draw path as a manual
// draw (admins immediately, players after the delay, winner/half-time effects).
// A single long-lived goroutine (RunAutoDrawScheduler) owns the timing so there
// is never more than one auto-drawer, and it re-reads the live interval on every
// cycle so admin changes take effect without a restart. The player draw delay is
// applied only to the player broadcast (postDraw), never to the cadence.

// signalAutoWake nudges the auto-draw scheduler to re-evaluate its timer after an
// auto-relevant change (game start/end, enable/disable, interval or delay change).
// Non-blocking: the wake channel is a one-slot mailbox, so a burst collapses to a
// single re-evaluation.
func (s *Server) signalAutoWake() {
	select {
	case s.autoWake <- struct{}{}:
	default:
	}
}

// requestImmediateAutoDraw asks the scheduler to draw the first number the moment
// auto turns on (rather than after one interval), then wakes it to act on the
// request. Used when a game starts with auto on and when auto is toggled on.
func (s *Server) requestImmediateAutoDraw() {
	s.autoDrawNow.Store(true)
	s.signalAutoWake()
}

// broadcastAutoConfig tells every client the current auto-draw state (enabled +
// interval) so all admin surfaces — web and plugin — keep their toggle and "Time
// Between Calls" selector in step, exactly like yoever_config.
func (s *Server) broadcastAutoConfig() {
	enabled, interval := s.game.AutoState()
	s.hub.Broadcast(struct {
		Type     string `json:"type"`
		Enabled  bool   `json:"enabled"`
		Interval int    `json:"interval"`
	}{Type: "auto_config", Enabled: enabled, Interval: interval})
}

// currentDrawDelay reads the shared player draw delay (seconds), clamped to 0–60.
// It governs only how long each drawn number is held before it reaches players;
// admins always see the number immediately, and it never affects the auto cadence.
func (s *Server) currentDrawDelay() int {
	return clampDrawDelay(s.getSettingInt("default_draw_delay", 0))
}

// onHalftime handles a draw crossing the half-time mark: pause the auto loop (if
// running) so the admin can decide on a mini-game, remember when the triggering
// number reaches players (so a confirmed alert isn't shown before it), and prompt
// every admin surface. `readyAt` is when the crossing number reaches players.
func (s *Server) onHalftime(readyAt time.Time) {
	paused := s.game.PauseAutoForHalftime()
	if paused {
		s.broadcastAutoConfig()
		s.signalAutoWake()
	}
	s.halftimeMu.Lock()
	s.halftimeReadyAt = readyAt
	s.halftimeMu.Unlock()
	// Prompt admins to choose whether to run a mini-game. `auto_paused` lets the
	// modal explain that declining will resume the auto draws.
	s.hub.BroadcastToAdmins(struct {
		Type       string `json:"type"`
		AutoPaused bool   `json:"auto_paused"`
	}{Type: "halftime_prompt", AutoPaused: paused})
}

// broadcastHalftimeMinigameWhenReady alerts players about a mini-game, holding the
// message until the half-time-triggering number has actually reached them (so they
// never see the mini-game prompt before the number that caused it). The wait is
// the remaining draw-delay recorded when half-time fired; usually zero by the time
// an admin has answered the prompt.
func (s *Server) broadcastHalftimeMinigameWhenReady() {
	s.halftimeMu.Lock()
	readyAt := s.halftimeReadyAt
	s.halftimeMu.Unlock()

	// Capture the game this alert belongs to (the admin confirmed the mini-game
	// while it was live) so a hold of up to the draw delay can't deliver game A's
	// alert into a game B that started in the meantime. Also carried on the payload
	// as game_id for the client-side guard.
	gameID := s.currentGameID()

	send := func() {
		defer recoverPanic("halftime minigame broadcast")
		if s.currentGameID() != gameID {
			return
		}
		s.hub.BroadcastToPlayers(struct {
			Type   string `json:"type"`
			GameID int64  `json:"game_id"`
		}{Type: "halftime_minigame", GameID: gameID})
	}
	if remaining := time.Until(readyAt); remaining > 0 {
		time.AfterFunc(remaining, send)
	} else {
		send()
	}
}

// autoDrawOnce performs one automatic draw and its side-effects, mirroring a manual
// draw. It self-guards: it only draws while auto is enabled, and switches auto off
// (broadcasting the change) when the callable pool is exhausted or the game has
// gone, so the scheduler then parks.
func (s *Server) autoDrawOnce() {
	// A panic here (e.g. in a broadcast side-effect) must not kill the scheduler
	// goroutine — recover, log, and let the loop continue.
	defer recoverPanic("auto draw")
	// DrawAuto draws only while auto is still enabled, checking the flag under the
	// draw lock — so a disable racing with this fire (a manual draw taking over, a
	// winner, an admin toggle) can't leak a stray number.
	result, newWinner, err := s.game.DrawAuto()
	if err != nil {
		slog.Error("auto draw failed", "error", err)
		return
	}
	if result == nil {
		// Auto was switched off (raced), the callable pool is exhausted, or the game
		// is gone — make sure auto is off so the loop parks.
		if s.game.DisableAuto() {
			s.broadcastAutoConfig()
			s.signalAutoWake()
		}
		return
	}
	s.postDraw(result, newWinner, s.currentDrawDelay())
}

// RunAutoDrawScheduler is the single goroutine that drives automatic draws. It
// draws the first number the instant auto is switched on, then spaces subsequent
// draws by the interval — the player draw delay only shifts when each number
// reaches players (see postDraw), it never stretches the admin's cadence. A config
// change (signalAutoWake) makes it recompute; it parks (no timer) whenever auto is
// off. Launched once from main with a context cancelled on shutdown, so it drains
// cleanly and never draws into a closing database.
func (s *Server) RunAutoDrawScheduler(ctx context.Context) {
	// Backstop: each draw is already guarded in autoDrawOnce, but a panic anywhere
	// else in the loop must not crash the process. (autoDrawOnce's own recover keeps
	// the loop alive across draw panics; this only catches the rare rest.)
	defer recoverPanic("auto draw scheduler")
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	for {
		enabled, interval := s.game.AutoState()
		switch {
		case enabled && s.autoDrawNow.Swap(false):
			// Auto was just switched on — draw the first number immediately, then
			// loop to arm the interval cadence.
			s.autoDrawOnce()
			continue
		case enabled:
			// Space draws by the interval alone. The player draw delay is applied
			// per-draw in postDraw (players lag the admin by the delay); folding it
			// in here would wrongly stretch the gap the admin waits between numbers.
			resetTimer(timer, time.Duration(bingo.ClampAutoInterval(interval))*time.Second)
		default:
			s.autoDrawNow.Store(false) // drop a stale immediate-draw request while off
			stopTimer(timer)
		}

		select {
		case <-ctx.Done():
			return
		case <-s.autoWake:
			// Config changed — loop to recompute the timer from the new state.
			continue
		case <-timer.C:
			s.autoDrawOnce()
			// Loop to schedule the next draw (or park if auto just switched off).
		}
	}
}

// resetTimer safely resets a timer to fire after d, draining a pending fire first
// so a stale tick can't trigger an extra draw.
func resetTimer(t *time.Timer, d time.Duration) {
	stopTimer(t)
	t.Reset(d)
}

// stopTimer stops a timer and drains any already-fired-but-unread tick.
func stopTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}
