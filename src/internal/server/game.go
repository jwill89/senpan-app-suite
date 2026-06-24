package server

import (
	"app-suite/internal/model"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// gameRequest is the JSON body for POST /api/game.
// Action determines the operation: "start", "draw", "end", "trigger_halftime", or "update_details".
type gameRequest struct {
	Action         string   `json:"action"`
	PatternIDs     []int    `json:"pattern_ids"`      // pattern IDs to use when starting a game
	Details        string   `json:"details"`          // game details text (for update_details)
	Delay          int      `json:"delay"`            // seconds to delay player broadcast (0 = instant)
	ValidWinnerIDs []string `json:"valid_winner_ids"` // card IDs confirmed as valid winners on end
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
	writeJSON(w, http.StatusOK, map[string]any{
		"game":         state,
		"winners":      winners,
		"game_details": details,
	})
}

// handleGameAction processes game lifecycle operations.
//
//	Endpoint:    POST /api/game
//	Auth:        admin, or a user granted this page's permission
//	Request:     {"action": "start"|"draw"|"end"|"trigger_halftime"|"set_delay"|"update_details", ...}
//	Response:    varies by action
//	Broadcasts:  game_update (start/end), game_draw (draw), halftime_minigame,
//	             draw_delay_update (set_delay), details_update
func (s *Server) handleGameAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoGame) {
		return
	}

	req, err := readJSON[gameRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "start":
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
		writeJSON(w, http.StatusOK, map[string]any{
			"game":         game,
			"winners":      []string{},
			"game_details": details,
		})
		s.broadcastGameStart(game, details)

	case "draw":
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

	case "end":
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

		writeJSON(w, http.StatusOK, map[string]any{"ended": ok})
		s.broadcastGameEnd()

	case "trigger_halftime":
		s.hub.BroadcastToPlayers(struct {
			Type string `json:"type"`
		}{Type: "halftime_minigame"})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "set_delay":
		// The draw delay is a shared game control: persist the caller's choice (so
		// it survives page loads — admins read it as default_draw_delay) and
		// broadcast it so every other admin's selector updates live.
		delay := req.Delay
		if delay < 0 || delay > 60 {
			writeError(w, http.StatusBadRequest, "Draw delay must be 0–60")
			return
		}
		if err := s.store.SetSetting("default_draw_delay", strconv.Itoa(delay)); err != nil {
			writeInternalError(w, "save draw delay", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		s.hub.Broadcast(struct {
			Type  string `json:"type"`
			Delay int    `json:"delay"`
		}{Type: "draw_delay_update", Delay: delay})

	case "update_details":
		if err := s.game.SetGameDetails(req.Details); err != nil {
			writeInternalError(w, "update game details", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		// Broadcast the updated details to all connected clients
		s.hub.Broadcast(struct {
			Type    string `json:"type"`
			Details string `json:"game_details"`
		}{Type: "details_update", Details: req.Details})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: start, draw, end, trigger_halftime, update_details")
	}
}
