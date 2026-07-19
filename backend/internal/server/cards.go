package server

import (
	"encoding/json"
	"log/slog"
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
		entries[i] = model.CardListEntry{
			ID:           c.ID,
			PlayerName:   c.PlayerName,
			Details:      c.Details,
			CreatedAt:    c.CreatedAt,
			Protected:    c.Protected,
			CustomStatus: c.CustomStatus,
			World:        c.World,
		}
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

// handleCardsDeleteAll deletes every non-Protected card, reporting how many rows
// were removed (per the project's bulk-delete convention). Protected cards (approved
// custom cards, or any card an admin marked Protected) are spared.
//
//	Endpoint:    DELETE /api/cards/all
//	Auth:        permission:bingo-cards
//	Response:    200 {"deleted": N}
//	Broadcasts:  cards_update; card_deleted (only to players whose card was deleted)
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
	writeJSON(w, http.StatusOK, model.DeletedCountResponse{Deleted: int64(len(deleted))})
	s.broadcastCards()
	// Disconnect only the players whose card was actually deleted — a player on a
	// surviving Protected card keeps their live board.
	if msg, err := json.Marshal(map[string]string{"type": "card_deleted"}); err == nil {
		for _, id := range deleted {
			s.hub.DisconnectCardClients(id, msg)
		}
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

// handleCardApprove approves a pending custom-card request: it becomes a live,
// playable card and is automatically marked Protected. Invalidates the card cache so
// the newly approved card is immediately eligible to win.
//
//	Endpoint:    POST /api/cards/{id}/approve
//	Auth:        permission:bingo-cards
//	Response:    200 {"ok": true} (404 when no pending custom card has that id)
//	Broadcasts:  cards_update
func (s *Server) handleCardApprove(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoCards) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Card id is required")
		return
	}
	ok, err := s.store.ApproveCustomCard(id)
	if err != nil {
		writeInternalError(w, "approve card", err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "No pending custom card with that ID")
		return
	}
	s.game.InvalidateCardCache()
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
	s.broadcastCards()
}

// cardProtectRequest is the JSON body for POST /api/cards/{id}/protect.
type cardProtectRequest struct {
	Protected bool `json:"protected"`
}

// handleCardProtect marks or unmarks a card as Protected. Protected cards are spared
// by "Delete All" (they can still be deleted individually).
//
//	Endpoint:    POST /api/cards/{id}/protect
//	Auth:        permission:bingo-cards
//	Request:     {"protected": true|false}
//	Response:    200 {"ok": true} (404 when the card doesn't exist)
//	Broadcasts:  cards_update
func (s *Server) handleCardProtect(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoCards) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Card id is required")
		return
	}
	req, err := readJSON[cardProtectRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	ok, err := s.store.SetCardProtected(id, req.Protected)
	if err != nil {
		writeInternalError(w, "set card protected", err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "Card not found")
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
	s.broadcastCards()
}

// cardRequestRequest is the JSON body for POST /api/cards/request (a public custom
// card request built on the Personal Card Requests page).
type cardRequestRequest struct {
	CharacterName  string  `json:"character_name"`
	World          string  `json:"world"`
	CardID         string  `json:"card_id"`
	BoardData      [][]int `json:"board_data"`
	TurnstileToken string  `json:"turnstile_token"` // Cloudflare Turnstile token (when enabled)
}

// handleCardRequest accepts a public Personal Card Request: a hand-built bingo card
// with a user-chosen 6-character ID, character name, and home world. It validates
// the card structurally, rejects a taken ID or a board identical to an existing
// card, and on success stores the card as pending — awaiting staff approval and not
// yet playable (see handleBoard / cachedCards).
//
//	Endpoint:    POST /api/cards/request
//	Auth:        public (rate-limited; Cloudflare Turnstile when configured)
//	Request:     {"character_name","world","card_id","board_data":[[...]],"turnstile_token"}
//	Response:    201 {"id": "ABC123", "status": "pending"}
//	Broadcasts:  cards_update
func (s *Server) handleCardRequest(w http.ResponseWriter, r *http.Request) {
	// Public endpoint: throttle per IP so a bot can't flood the cards table with
	// pending requests. Every attempt counts against the limit.
	ip := clientIP(r)
	if s.cardReqLimiter.isLimited(ip) {
		slog.Warn("card request rate limited", "ip", ip)
		writeError(w, http.StatusTooManyRequests, "Too many card requests. Please try again later.")
		return
	}
	s.cardReqLimiter.recordFailure(ip)

	req, err := readJSON[cardRequestRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if s.turnstileEnabled() && !s.verifyTurnstile(r.Context(), req.TurnstileToken, ip) {
		slog.Warn("turnstile verification failed (card request)", "ip", ip)
		writeError(w, http.StatusForbidden, "Bot check failed. Please try again.")
		return
	}

	charName := strings.TrimSpace(req.CharacterName)
	world := strings.TrimSpace(req.World)
	if charName == "" || world == "" {
		writeError(w, http.StatusBadRequest, "Character name and world are required")
		return
	}
	if len(charName) > 60 || len(world) > 60 {
		writeError(w, http.StatusBadRequest, "Character name and world must be 60 characters or fewer")
		return
	}

	// Validate + normalise the chosen ID (exactly 6 alphanumeric, upper-cased).
	id, err := bingo.ValidateCustomID(req.CardID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the board is a structurally valid bingo card.
	if err := bingo.ValidateBoard(req.BoardData); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// The ID must not already be in use.
	exists, err := s.store.CardExists(id)
	if err != nil {
		writeInternalError(w, "check card id", err)
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "That card ID is already taken — please choose another.")
		return
	}

	// The exact card (same numbers in the same cells) must not already exist.
	dupID, dup, err := s.store.FindDuplicateBoard(req.BoardData)
	if err != nil {
		writeInternalError(w, "check duplicate board", err)
		return
	}
	if dup {
		writeError(w, http.StatusConflict, "A card with these exact numbers already exists (card "+dupID+"). Please change some numbers.")
		return
	}

	if err := s.store.CreateCustomCard(id, req.BoardData, charName, world); err != nil {
		writeInternalError(w, "create custom card", err)
		return
	}
	s.game.InvalidateCardCache()
	writeJSON(w, http.StatusCreated, model.CardRequestResponse{ID: id, Status: "pending"})
	s.broadcastCards()
}
