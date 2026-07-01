package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"app-suite/internal/bingo"
	"app-suite/internal/model"
	"app-suite/internal/store"
)

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
	entries := make([]model.CardListEntry, len(cards))
	for i, c := range cards {
		entries[i] = model.CardListEntry{ID: c.ID, PlayerName: c.PlayerName, Details: c.Details, CreatedAt: c.CreatedAt}
	}
	writeJSON(w, http.StatusOK, model.CardsListResponse{Cards: entries})
}

// cardCreateRequest is the JSON body for POST /api/cards (create one named card).
type cardCreateRequest struct {
	PlayerName string `json:"player_name"`
}

// handleCardCreate generates one card and assigns it to a player name in a single
// step, so an admin can hand a named card to a walk-in without a separate edit.
//
//	Endpoint:    POST /api/cards
//	Auth:        permission:bingo-cards
//	Request:     {"player_name": "..."}
//	Response:    201 GenerateSingleCardResponse
//	Broadcasts:  cards_update
func (s *Server) handleCardCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoCards) {
		return
	}
	req, err := readJSON[cardCreateRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
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
	writeJSON(w, http.StatusCreated, model.GenerateSingleCardResponse{
		Card:  model.GeneratedNamedCard{ID: id, PlayerName: name, BoardData: board},
		Count: 1,
	})
	s.broadcastCards()
}

// cardGenerateRequest is the JSON body for POST /api/cards/generate (bulk generate).
type cardGenerateRequest struct {
	Count int `json:"count"` // number of cards to generate (clamped 1–500)
}

// handleCardsGenerate bulk-generates N random cards.
//
//	Endpoint:    POST /api/cards/generate
//	Auth:        permission:bingo-cards
//	Request:     {"count": N}
//	Response:    201 GenerateCardsResponse
//	Broadcasts:  cards_update
func (s *Server) handleCardsGenerate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoCards) {
		return
	}
	req, err := readJSON[cardGenerateRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	count := max(1, min(500, req.Count))

	cards := make([]model.GeneratedCard, 0, count)
	batch := make([]store.CardBatchEntry, 0, count)
	for range count {
		id, err := bingo.GenerateID(s.store.CardExists)
		if err != nil {
			writeInternalError(w, "generate card ID", err)
			return
		}
		board := bingo.GenerateBoard()
		cards = append(cards, model.GeneratedCard{ID: id, BoardData: board})
		batch = append(batch, store.CardBatchEntry{ID: id, Board: board})
	}
	if err := s.store.SaveCardsBatch(batch); err != nil {
		writeInternalError(w, "save cards batch", err)
		return
	}
	s.game.InvalidateCardCache()
	writeJSON(w, http.StatusCreated, model.GenerateCardsResponse{Cards: cards, Count: len(cards)})
	s.broadcastCards()
}

// handleCardsDeleteAll deletes every card, reporting how many rows were removed
// (per the project's bulk-delete convention).
//
//	Endpoint:    DELETE /api/cards/all
//	Auth:        permission:bingo-cards
//	Response:    200 {"deleted": N}
//	Broadcasts:  cards_update; card_deleted (to all player connections)
func (s *Server) handleCardsDeleteAll(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoCards) {
		return
	}
	deleted, err := s.store.DeleteAllCards()
	if err != nil {
		writeInternalError(w, "delete all cards", err)
		return
	}
	s.game.InvalidateCardCache()
	writeJSON(w, http.StatusOK, model.DeletedCountResponse{Deleted: deleted})
	s.broadcastCards()
	// Disconnect all player WebSocket connections
	if msg, err := json.Marshal(map[string]string{"type": "card_deleted"}); err == nil {
		s.hub.DisconnectAllPlayerClients(msg)
	}
}

// handleCardDelete deletes one card by id.
//
//	Endpoint:    DELETE /api/cards/{id}
//	Auth:        permission:bingo-cards
//	Response:    204 No Content
//	Broadcasts:  cards_update; card_deleted (to the affected player connection)
func (s *Server) handleCardDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoCards) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Card id is required")
		return
	}
	deleted, err := s.store.DeleteCard(id)
	if err != nil {
		writeInternalError(w, "delete card", err)
		return
	}
	s.game.InvalidateCardCache()
	w.WriteHeader(http.StatusNoContent)
	s.broadcastCards()
	// Disconnect any player whose card was just deleted
	if deleted {
		if msg, err := json.Marshal(map[string]string{"type": "card_deleted"}); err == nil {
			s.hub.DisconnectCardClients(id, msg)
		}
	}
}

// cardUpdateRequest is the JSON body for PATCH /api/cards/{id}.
type cardUpdateRequest struct {
	PlayerName string `json:"player_name"`
	Details    string `json:"details"`
}

// handleCardUpdate updates a card's assigned player name and details.
//
//	Endpoint:    PATCH /api/cards/{id}
//	Auth:        permission:bingo-cards
//	Request:     {"player_name": "...", "details": "..."}
//	Response:    200 {"ok": true}
//	Broadcasts:  cards_update
func (s *Server) handleCardUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoCards) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Card id is required")
		return
	}
	req, err := readJSON[cardUpdateRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if err := s.store.UpdateCardPlayer(id, req.PlayerName, req.Details); err != nil {
		writeInternalError(w, "update card player", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
	// Broadcast so other admins viewing Manage Cards see the assignment change
	// live (and keep their indicators, since the broadcast carries the names).
	s.broadcastCards()
}
