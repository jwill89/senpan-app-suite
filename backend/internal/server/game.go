package server

import (
	"app-suite/internal/bingo"
	"app-suite/internal/model"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
}

// handleGameStart starts a new game with the selected win patterns.
//
//	Endpoint:    POST /api/game/start
//	Auth:        permission:bingo-game
//	Request:     {"pattern_ids": [...]}
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
	game, err := s.game.Start(req.PatternIDs)
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
//	Broadcasts:  game_draw (to admins immediately; to players delayed/immediate)
func (s *Server) handleGameDraw(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoGame) {
		return
	}
	req, err := readJSON[gameDrawRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	result, err := s.game.Draw()
	if err != nil {
		writeInternalError(w, "draw number", err)
		return
	}
	if result == nil {
		writeError(w, http.StatusBadRequest, "No active game or all 75 numbers have been drawn")
		return
	}
	writeJSON(w, http.StatusOK, result)

	// Broadcast to admins immediately (keeps other admin tabs in sync)
	s.broadcastDrawToAdmins(result.Drawn, result.Winners)

	// Broadcast to players: delayed or immediate
	delay := req.Delay
	if delay < 0 {
		delay = 0
	}
	if delay > 60 {
		delay = 60
	}
	if delay > 0 {
		drawn := result.Drawn
		time.AfterFunc(time.Duration(delay)*time.Second, func() {
			s.broadcastDrawToPlayers(drawn)
		})
	} else {
		s.broadcastDrawToPlayers(result.Drawn)
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
}

// handleGameHalftime alerts players about a half-time mini-game.
//
//	Endpoint:    POST /api/game/halftime
//	Auth:        permission:bingo-game
//	Response:    200 {"ok": true}
//	Broadcasts:  halftime_minigame (to players)
func (s *Server) handleGameHalftime(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoGame) {
		return
	}
	s.hub.BroadcastToPlayers(struct {
		Type string `json:"type"`
	}{Type: "halftime_minigame"})
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
type gamePatchRequest struct {
	Delay         *int    `json:"delay"`
	Details       *string `json:"details"`
	YoeverEnabled *bool   `json:"yoever_enabled"`
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

	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}
