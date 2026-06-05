package server

import (
	"net/http"
	"strings"
)

// handleBoard serves a player's bingo board along with the current game state.
// With ?preview=1, returns only the card data without game state (admin card previews).
//
//	Endpoint:  GET /api/board?id=XXXXXX[&preview=1]
//	Auth:      public
//	Params:    id (required) — 6-char card ID; preview (optional) — skip game state
//	Response:  {"card": Card, "game": GameState|null, "game_details": "..."}
//	           or with preview=1: {"card": Card}
func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "Board ID is required")
		return
	}

	card, err := s.store.GetCard(strings.ToUpper(id))
	if err != nil {
		writeInternalError(w, "get card", err)
		return
	}
	if card == nil {
		writeError(w, http.StatusNotFound, "Board not found")
		return
	}

	// Lightweight preview mode — return only the card, skip game state.
	if r.URL.Query().Get("preview") != "" {
		writeJSON(w, http.StatusOK, map[string]any{"card": card})
		return
	}

	state, _, err := s.game.CurrentState()
	if err != nil {
		writeInternalError(w, "get game state", err)
		return
	}

	details, _ := s.game.GameDetails()

	writeJSON(w, http.StatusOK, map[string]any{
		"card":         card,
		"game":         state,
		"game_details": details,
	})
}
