package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"app-suite/internal/bingo"
	"app-suite/internal/store"
)

// cardsRequest is the JSON body for POST /api/cards.
// Action determines the operation: "generate", "delete", "delete_all", or "update_player".
type cardsRequest struct {
	Action     string `json:"action"`
	ID         string `json:"id"`          // card ID (for delete, update_player)
	Count      int    `json:"count"`       // number of cards to generate (1–500)
	PlayerName string `json:"player_name"` // player name (for update_player)
	Details    string `json:"details"`     // card details (for update_player)
}

// handleCardsList returns all card IDs with their player names and details.
//
//	Endpoint:  GET /api/cards
//	Auth:      admin, or a user granted this page's permission
//	Response:  {"cards": [{id, player_name, details}, ...]}
func (s *Server) handleCardsList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoCards) {
		return
	}

	cards, err := s.store.ListCardIDsWithNames()
	if err != nil {
		writeInternalError(w, "list cards", err)
		return
	}
	type cardListEntry struct {
		ID         string `json:"id"`
		PlayerName string `json:"player_name"`
		Details    string `json:"details"`
		CreatedAt  string `json:"created_at"`
	}
	entries := make([]cardListEntry, len(cards))
	for i, c := range cards {
		entries[i] = cardListEntry{ID: c.ID, PlayerName: c.PlayerName, Details: c.Details, CreatedAt: c.CreatedAt}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cards": entries})
}

// handleCardsAction processes card management operations.
//
//	Endpoint:    POST /api/cards
//	Auth:        admin, or a user granted this page's permission
//	Request:     {"action": "generate"|"generate_single"|"delete"|"delete_all"|"update_player", ...}
//	Response:    varies by action
//	Broadcasts:  cards_update (on generate/generate_single/delete/delete_all/update_player)
//	             card_deleted (to affected player connections)
func (s *Server) handleCardsAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoCards) {
		return
	}

	req, err := readJSON[cardsRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "generate":
		count := max(1, min(500, req.Count))

		type generatedCard struct {
			ID        string  `json:"id"`
			BoardData [][]int `json:"board_data"`
		}
		cards := make([]generatedCard, 0, count)
		batch := make([]store.CardBatchEntry, 0, count)
		for range count {
			id, err := bingo.GenerateID(s.store.CardExists)
			if err != nil {
				writeInternalError(w, "generate card ID", err)
				return
			}
			board := bingo.GenerateBoard()
			cards = append(cards, generatedCard{ID: id, BoardData: board})
			batch = append(batch, store.CardBatchEntry{ID: id, Board: board})
		}
		if err := s.store.SaveCardsBatch(batch); err != nil {
			writeInternalError(w, "save cards batch", err)
			return
		}
		s.game.InvalidateCardCache()
		writeJSON(w, http.StatusOK, map[string]any{"cards": cards, "count": len(cards)})
		s.broadcastCards()

	case "generate_single":
		// Generate one card and assign it to a player name in a single step, so an
		// admin can hand a named card to a walk-in without a separate edit.
		name := strings.TrimSpace(req.PlayerName)
		id, err := bingo.GenerateID(s.store.CardExists)
		if err != nil {
			writeInternalError(w, "generate card ID", err)
			return
		}
		board := bingo.GenerateBoard()
		if err := s.store.SaveCard(id, board); err != nil {
			writeInternalError(w, "save card", err)
			return
		}
		if name != "" {
			if err := s.store.UpdateCardPlayer(id, name, ""); err != nil {
				writeInternalError(w, "assign card player", err)
				return
			}
		}
		s.game.InvalidateCardCache()
		writeJSON(w, http.StatusOK, map[string]any{
			"card":  map[string]any{"id": id, "player_name": name, "board_data": board},
			"count": 1,
		})
		s.broadcastCards()

	case "delete":
		if req.ID == "" {
			writeError(w, http.StatusBadRequest, "Card id is required")
			return
		}
		deleted, err := s.store.DeleteCard(req.ID)
		if err != nil {
			writeInternalError(w, "delete card", err)
			return
		}
		s.game.InvalidateCardCache()
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
		s.broadcastCards()
		// Disconnect any player whose card was just deleted
		if deleted {
			if msg, err := json.Marshal(map[string]string{"type": "card_deleted"}); err == nil {
				s.hub.DisconnectCardClients(req.ID, msg)
			}
		}

	case "delete_all":
		deleted, err := s.store.DeleteAllCards()
		if err != nil {
			writeInternalError(w, "delete all cards", err)
			return
		}
		s.game.InvalidateCardCache()
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
		s.broadcastCards()
		// Disconnect all player WebSocket connections
		if msg, err := json.Marshal(map[string]string{"type": "card_deleted"}); err == nil {
			s.hub.DisconnectAllPlayerClients(msg)
		}

	case "update_player":
		if req.ID == "" {
			writeError(w, http.StatusBadRequest, "Card id is required")
			return
		}
		if err := s.store.UpdateCardPlayer(req.ID, req.PlayerName, req.Details); err != nil {
			writeInternalError(w, "update card player", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		// Broadcast so other admins viewing Manage Cards see the assignment change
		// live (and keep their indicators, since the broadcast carries the names).
		s.broadcastCards()

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: generate, generate_single, delete, delete_all, update_player")
	}
}
