package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// ── Stamp Rally admin (events + stamps + prizes + cards + logs) ──────────────
//
// A Stamp Rally is an event (see model.StampRally) whose stamps participants
// collect by entering per-stall passwords on a tokenized card. The admin authors
// the event and issues card links; the public endpoints below need no auth (the
// token is the capability). Availability + completion are computed here against
// time.Now (the store stays pure data access), reusing parseRaffleTime for the
// UTC window parsing shared with raffles.

// clampPct constrains a placement percentage to the card's box (0–100). Width/height
// are also floored just above zero so an item can't collapse to nothing.
func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// sanitizePlacement clamps x/y/width/height into the card box; rotation is left as-is.
func sanitizePlacement(p model.Placement) model.Placement {
	p.X = clampPct(p.X)
	p.Y = clampPct(p.Y)
	p.Width = clampPct(p.Width)
	p.Height = clampPct(p.Height)
	if p.Width <= 0 {
		p.Width = 10
	}
	if p.Height <= 0 {
		p.Height = 10
	}
	return p
}

// ── Availability + completion (time logic) ───────────────────────────────────

// rallyOpen reports whether the event accepts stamping now: it must be manually open
// (status "open", not "closed") AND within its availability window.
func rallyOpen(r *model.StampRally, now time.Time) bool {
	if r.Status == "closed" {
		return false
	}
	if from, ok := parseRaffleTime(r.AvailableFrom); ok && now.Before(from) {
		return false
	}
	if to, ok := parseRaffleTime(r.AvailableTo); ok && now.After(to) {
		return false
	}
	return true
}

// stampAvailable reports whether a stamp can be collected right now: the event is
// open, the stamp is within its own active window, and it isn't paused.
func stampAvailable(r *model.StampRally, st *model.StampRallyStamp, now time.Time) bool {
	if st.Paused || !rallyOpen(r, now) {
		return false
	}
	if from, ok := parseRaffleTime(st.ActiveFrom); ok && now.Before(from) {
		return false
	}
	if to, ok := parseRaffleTime(st.ActiveTo); ok && now.After(to) {
		return false
	}
	return true
}

// stampExpired reports whether a stamp can NEVER be collected again — its own active
// window ended, or the whole event ended. Such a stamp no longer blocks completion.
func stampExpired(r *model.StampRally, st *model.StampRallyStamp, now time.Time) bool {
	if to, ok := parseRaffleTime(st.ActiveTo); ok && now.After(to) {
		return true
	}
	if to, ok := parseRaffleTime(r.AvailableTo); ok && now.After(to) {
		return true
	}
	return false
}

// rallyCardComplete reports whether a card is complete: every stamp is either
// collected or permanently expired, and the participant collected at least one.
// A merely-paused stamp still within its window keeps the card incomplete.
func rallyCardComplete(r *model.StampRally, stamps []model.StampRallyStamp, collected map[int64]string, now time.Time) bool {
	if len(collected) == 0 {
		return false
	}
	for i := range stamps {
		st := &stamps[i]
		if _, ok := collected[st.ID]; ok {
			continue
		}
		if !stampExpired(r, st, now) {
			return false // still collectable and not collected → not complete
		}
	}
	return true
}

// ── Public payload shapes (no passwords; prizes hidden until complete) ────────
//
// The exported wire structs (model.PublicStampCard and its nested
// model.PublicStampRally/PublicStamp/PublicPrize) live in the model package; the
// building logic stays here.

// buildPublicCard assembles the participant-facing view from loaded rows, stripping
// passwords, computing each stamp's availability/collection, and revealing prize
// name/image only when the card is complete.
func buildPublicCard(r *model.StampRally, card *model.StampRallyCard, stamps []model.StampRallyStamp,
	prizes []model.StampRallyPrize, collected map[int64]string, now time.Time) model.PublicStampCard {
	pc := model.PublicStampCard{
		Rally: model.PublicStampRally{
			ID: r.ID, Title: r.Title, CardImage: r.CardImage, NotStampedImage: r.NotStampedImage,
			Details: r.Details, RedeemInstructions: r.RedeemInstructions, RedeemImage: r.RedeemImage,
			AvailableFrom: r.AvailableFrom, AvailableTo: r.AvailableTo, IsActive: rallyOpen(r, now),
		},
		ParticipantName: card.ParticipantName,
		Completed:       card.Completed,
		CompletedAt:     card.CompletedAt,
		PrizesRevealed:  card.Completed,
		Stamps:          make([]model.PublicStamp, 0, len(stamps)),
		Prizes:          make([]model.PublicPrize, 0, len(prizes)),
	}
	for i := range stamps {
		st := &stamps[i]
		at, got := collected[st.ID]
		pc.Stamps = append(pc.Stamps, model.PublicStamp{
			ID: st.ID, AffiliateName: st.AffiliateName, Image: st.Image, Placement: st.Placement,
			ActiveFrom: st.ActiveFrom, ActiveTo: st.ActiveTo,
			Available: stampAvailable(r, st, now), Collected: got, CollectedAt: at,
		})
	}
	for i := range prizes {
		p := &prizes[i]
		pp := model.PublicPrize{ID: p.ID, Placement: p.Placement}
		if card.Completed {
			pp.Name = p.Name
			pp.Image = p.Image
		}
		pc.Prizes = append(pc.Prizes, pp)
	}
	return pc
}

// ── Admin: list / detail ─────────────────────────────────────────────────────

// handleStampRalliesList returns every rally (admin only).
//
//	Endpoint:  GET /api/stamp-rallies
//	Auth:      admin, or a user granted festival-stamp-rally
func (s *Server) handleStampRalliesList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalStampRally) {
		return
	}
	rallies, err := s.store.ListStampRallies()
	if err != nil {
		writeInternalError(w, "list stamp rallies", err)
		return
	}
	writeJSON(w, http.StatusOK, model.StampRalliesResponse{StampRallies: rallies})
}

// handleStampRallyDetail returns a rally with its stamps, prizes, and issued cards.
//
//	Endpoint:  GET /api/stamp-rallies/{id}
//	Auth:      admin, or a user granted festival-stamp-rally
func (s *Server) handleStampRallyDetail(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalStampRally) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid stamp rally ID")
		return
	}
	rally, err := s.store.GetStampRally(id)
	if err != nil {
		writeInternalError(w, "get stamp rally", err)
		return
	}
	if rally == nil {
		writeError(w, http.StatusNotFound, "Stamp rally not found")
		return
	}
	cards, err := s.store.ListRallyCards(id)
	if err != nil {
		writeInternalError(w, "list rally cards", err)
		return
	}
	writeJSON(w, http.StatusOK, model.StampRallyDetailResponse{StampRally: *rally, Cards: cards})
}

// handleStampRallyLogs returns the event-wide stamp log (every collection across all
// cards), ordered so a participant's rows group together.
//
//	Endpoint:  GET /api/stamp-rallies/{id}/logs
//	Auth:      admin, or a user granted festival-stamp-rally
func (s *Server) handleStampRallyLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalStampRally) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid stamp rally ID")
		return
	}
	logs, err := s.store.ListRallyCollections(id)
	if err != nil {
		writeInternalError(w, "list rally collections", err)
		return
	}
	writeJSON(w, http.StatusOK, model.StampRallyLogsResponse{Logs: logs})
}

// ── Admin: CRUD ──────────────────────────────────────────────────────────────

// stampRallyWriteRequest is the JSON body for creating (POST /api/stamp-rallies)
// or replacing (PUT /api/stamp-rallies/{id}) a rally. The id comes from the path
// on PUT.
type stampRallyWriteRequest struct {
	Title              string                  `json:"title"`
	CardImage          string                  `json:"card_image"`
	NotStampedImage    string                  `json:"not_stamped_image"`
	AvailableFrom      string                  `json:"available_from"`
	AvailableTo        string                  `json:"available_to"`
	Details            string                  `json:"details"`
	RedeemInstructions string                  `json:"redeem_instructions"`
	RedeemImage        string                  `json:"redeem_image"`
	Stamps             []model.StampRallyStamp `json:"stamps"`
	Prizes             []model.StampRallyPrize `json:"prizes"`
}

// rallyFromRequest builds a sanitized model.StampRally (sans ID) from a request.
func rallyFromRequest(req stampRallyWriteRequest, title string) *model.StampRally {
	stamps := make([]model.StampRallyStamp, 0, len(req.Stamps))
	for _, st := range req.Stamps {
		st.Image = strings.TrimSpace(st.Image)
		st.Password = strings.TrimSpace(st.Password)
		st.AffiliateName = ""
		st.Placement = sanitizePlacement(st.Placement)
		stamps = append(stamps, st)
	}
	prizes := make([]model.StampRallyPrize, 0, len(req.Prizes))
	for _, p := range req.Prizes {
		p.Name = strings.TrimSpace(p.Name)
		p.Image = strings.TrimSpace(p.Image)
		p.Placement = sanitizePlacement(p.Placement)
		prizes = append(prizes, p)
	}
	return &model.StampRally{
		Title:              title,
		CardImage:          strings.TrimSpace(req.CardImage),
		NotStampedImage:    strings.TrimSpace(req.NotStampedImage),
		AvailableFrom:      strings.TrimSpace(req.AvailableFrom),
		AvailableTo:        strings.TrimSpace(req.AvailableTo),
		Details:            req.Details,
		RedeemInstructions: req.RedeemInstructions,
		RedeemImage:        strings.TrimSpace(req.RedeemImage),
		Stamps:             stamps,
		Prizes:             prizes,
	}
}

// handleStampRallyCreate creates a rally (stamps + prizes inline).
//
//	Endpoint:  POST /api/stamp-rallies
//	Auth:      admin, or a user granted festival-stamp-rally
//	Response:  201 {"stamp_rally": StampRally}
func (s *Server) handleStampRallyCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalStampRally) {
		return
	}
	req, err := readJSON[stampRallyWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Title is required")
		return
	}
	rally := rallyFromRequest(req, title)
	id, err := s.store.CreateStampRally(rally)
	if err != nil {
		writeInternalError(w, "create stamp rally", err)
		return
	}
	rally.ID = id
	rally.Status = "open"
	writeJSON(w, http.StatusCreated, model.StampRallyResponse{StampRally: *rally})
}

// handleStampRallyUpdate replaces a rally's editable fields (stamps + prizes
// inline). Status is not editable here and is preserved — use close/reopen.
//
//	Endpoint:  PUT /api/stamp-rallies/{id}
//	Auth:      admin, or a user granted festival-stamp-rally
//	Response:  200 {"ok": true}
func (s *Server) handleStampRallyUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalStampRally) {
		return
	}
	id, ok := pathInt64(w, r, "id", "stamp rally")
	if !ok {
		return
	}
	req, err := readJSON[stampRallyWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Title is required")
		return
	}
	rally := rallyFromRequest(req, title)
	rally.ID = id
	if err := s.store.UpdateStampRally(rally); err != nil {
		writeInternalError(w, "update stamp rally", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// handleStampRallyDelete deletes a rally and all its stamps/prizes/cards.
//
//	Endpoint:  DELETE /api/stamp-rallies/{id}
//	Auth:      admin, or a user granted festival-stamp-rally
//	Response:  204 No Content
func (s *Server) handleStampRallyDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalStampRally) {
		return
	}
	id, ok := pathInt64(w, r, "id", "stamp rally")
	if !ok {
		return
	}
	if _, err := s.store.DeleteStampRally(id); err != nil {
		writeInternalError(w, "delete stamp rally", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setStampRallyStatus applies a status change and responds with {ok, status}.
// Shared by the close and reopen verb handlers.
func (s *Server) setStampRallyStatus(w http.ResponseWriter, r *http.Request, status string) {
	if !s.requirePermission(w, r, permFestivalStampRally) {
		return
	}
	id, ok := pathInt64(w, r, "id", "stamp rally")
	if !ok {
		return
	}
	if err := s.store.SetStampRallyStatus(id, status); err != nil {
		writeInternalError(w, "set stamp rally status", err)
		return
	}
	writeJSON(w, http.StatusOK, model.StatusResponse{OK: true, Status: status})
}

// handleStampRallyClose closes a rally (read-only, moves to the closed table).
//
//	Endpoint:  POST /api/stamp-rallies/{id}/close
//	Auth:      admin, or a user granted festival-stamp-rally
//	Response:  200 {"ok": true, "status": "closed"}
func (s *Server) handleStampRallyClose(w http.ResponseWriter, r *http.Request) {
	s.setStampRallyStatus(w, r, "closed")
}

// handleStampRallyReopen reopens a closed rally.
//
//	Endpoint:  POST /api/stamp-rallies/{id}/reopen
//	Auth:      admin, or a user granted festival-stamp-rally
//	Response:  200 {"ok": true, "status": "open"}
func (s *Server) handleStampRallyReopen(w http.ResponseWriter, r *http.Request) {
	s.setStampRallyStatus(w, r, "open")
}

// stampPausedRequest is the JSON body for PATCH /api/stamp-rallies/{id}/stamps/{stampId}.
type stampPausedRequest struct {
	Paused bool `json:"paused"`
}

// handleStampRallyStampPatch pauses/resumes a single stamp without a full event
// re-save (flipping a boolean → PATCH).
//
//	Endpoint:  PATCH /api/stamp-rallies/{id}/stamps/{stampId}
//	Auth:      admin, or a user granted festival-stamp-rally
//	Response:  200 {"ok": true, "paused": bool}
func (s *Server) handleStampRallyStampPatch(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalStampRally) {
		return
	}
	rallyID, ok := pathInt64(w, r, "id", "stamp rally")
	if !ok {
		return
	}
	stampID, ok := pathInt64(w, r, "stampId", "stamp")
	if !ok {
		return
	}
	req, err := readJSON[stampPausedRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	paused, err := s.store.SetStampPaused(rallyID, stampID, req.Paused)
	if err != nil {
		writeInternalError(w, "set stamp paused", err)
		return
	}
	if !paused {
		writeError(w, http.StatusNotFound, "Stamp not found")
		return
	}
	writeJSON(w, http.StatusOK, model.PausedResponse{OK: true, Paused: req.Paused})
}

// stampRallyCardCreateRequest is the JSON body for POST /api/stamp-rallies/{id}/cards.
type stampRallyCardCreateRequest struct {
	ParticipantName string `json:"participant_name"`
}

// handleStampRallyCardCreate issues a tokenized participant card link.
//
//	Endpoint:  POST /api/stamp-rallies/{id}/cards
//	Auth:      admin, or a user granted festival-stamp-rally
//	Response:  201 {"card": StampRallyCard}
func (s *Server) handleStampRallyCardCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalStampRally) {
		return
	}
	rallyID, ok := pathInt64(w, r, "id", "stamp rally")
	if !ok {
		return
	}
	req, err := readJSON[stampRallyCardCreateRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name := strings.TrimSpace(req.ParticipantName)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Participant name is required")
		return
	}
	rally, err := s.store.GetStampRally(rallyID)
	if err != nil {
		writeInternalError(w, "get rally for card", err)
		return
	}
	if rally == nil {
		writeError(w, http.StatusNotFound, "Stamp rally not found")
		return
	}
	card, err := s.store.IssueRallyCard(rallyID, name)
	if err != nil {
		writeInternalError(w, "issue rally card", err)
		return
	}
	writeJSON(w, http.StatusCreated, model.StampRallyCardResponse{Card: *card})
}

// handleStampRallyCardDelete removes a participant card link.
//
//	Endpoint:  DELETE /api/stamp-rallies/{id}/cards/{cardId}
//	Auth:      admin, or a user granted festival-stamp-rally
//	Response:  204 No Content
func (s *Server) handleStampRallyCardDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalStampRally) {
		return
	}
	rallyID, ok := pathInt64(w, r, "id", "stamp rally")
	if !ok {
		return
	}
	cardID, ok := pathInt64(w, r, "cardId", "card")
	if !ok {
		return
	}
	// A card with collected stamps can only be deleted once the rally is closed
	// (its log is preserved either way — collected rows snapshot participant/stall
	// and detach via ON DELETE SET NULL). Mirrors garapon drawing-link deletion.
	rally, err := s.store.GetStampRally(rallyID)
	if err != nil {
		writeInternalError(w, "get rally for card delete", err)
		return
	}
	if rally == nil {
		writeError(w, http.StatusNotFound, "Stamp rally not found")
		return
	}
	closed := rally.Status == "closed"
	if !closed {
		collected, err := s.store.ListCollectedStampIDs(cardID)
		if err != nil {
			writeInternalError(w, "list collected for card delete", err)
			return
		}
		if len(collected) > 0 {
			writeError(w, http.StatusConflict,
				"This card has collected stamps and can't be deleted while the rally is open — close the rally first (the stamp log is kept).")
			return
		}
	}
	if _, err := s.store.DeleteRallyCard(rallyID, cardID, closed); err != nil {
		writeInternalError(w, "delete rally card", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Public (tokenized card view + stamp) ─────────────────────────────────────

// loadCardByToken resolves a token to its card + rally, writing the 404 itself when
// the token is unknown. Returns (card, rally, ok).
func (s *Server) loadCardByToken(w http.ResponseWriter, token string) (*model.StampRallyCard, *model.StampRally, bool) {
	card, err := s.store.GetRallyCardByToken(token)
	if err != nil {
		writeInternalError(w, "get card by token", err)
		return nil, nil, false
	}
	if card == nil {
		writeError(w, http.StatusNotFound, "Stamp card not found")
		return nil, nil, false
	}
	rally, err := s.store.GetStampRally(card.RallyID)
	if err != nil {
		writeInternalError(w, "get rally for card", err)
		return nil, nil, false
	}
	if rally == nil {
		writeError(w, http.StatusNotFound, "Stamp rally not found")
		return nil, nil, false
	}
	return card, rally, true
}

// maybeComplete recomputes completion and persists it when the card has just become
// complete (so completion driven by stamp expiry is caught lazily on read). Mutates
// the passed card so the response reflects the new state.
func (s *Server) maybeComplete(card *model.StampRallyCard, rally *model.StampRally,
	collected map[int64]string, now time.Time) {
	if card.Completed {
		return
	}
	if rallyCardComplete(rally, rally.Stamps, collected, now) {
		ts := now.UTC().Format(time.RFC3339)
		if err := s.store.SetRallyCardCompleted(card.ID, ts); err == nil {
			card.Completed = true
			card.CompletedAt = ts
		}
	}
}

// handleStampCardPublic returns a participant's card view for a token (no passwords).
//
//	Endpoint:  GET /api/stamp-card/{token}
//	Auth:      public (the token is the capability)
func (s *Server) handleStampCardPublic(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	card, rally, ok := s.loadCardByToken(w, token)
	if !ok {
		return
	}
	collected, err := s.store.ListCollectedStampIDs(card.ID)
	if err != nil {
		writeInternalError(w, "list collected", err)
		return
	}
	now := time.Now().UTC()
	s.maybeComplete(card, rally, collected, now)
	writeJSON(w, http.StatusOK, buildPublicCard(rally, card, rally.Stamps, rally.Prizes, collected, now))
}

// stampSubmitRequest is the JSON body for POST /api/stamp-card/{token}/stamp.
type stampSubmitRequest struct {
	Password string `json:"password"`
}

// handleStampCardStamp collects a stamp by password for a tokenized card.
//
//	Endpoint:  POST /api/stamp-card/{token}/stamp
//	Auth:      public (the token is the capability)
//	Request:   {"password":"..."}
//	Response:  the refreshed public card + "collected_stamp_id"
func (s *Server) handleStampCardStamp(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	card, rally, ok := s.loadCardByToken(w, token)
	if !ok {
		return
	}
	req, err := readJSON[stampSubmitRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	password := strings.TrimSpace(req.Password)
	if password == "" {
		writeError(w, http.StatusBadRequest, "Enter a password")
		return
	}

	// Find the stamp whose password matches (empty-password stamps never match).
	var match *model.StampRallyStamp
	for i := range rally.Stamps {
		if rally.Stamps[i].Password != "" && rally.Stamps[i].Password == password {
			match = &rally.Stamps[i]
			break
		}
	}
	if match == nil {
		writeError(w, http.StatusBadRequest, "That password doesn't match any stamp on this card")
		return
	}

	now := time.Now().UTC()
	if !stampAvailable(rally, match, now) {
		writeError(w, http.StatusBadRequest, "This stall is currently closed and cannot be stamped")
		return
	}

	// Snapshot the participant + stall onto the log so it survives card/stamp deletion.
	stall := strings.TrimSpace(match.AffiliateName)
	if stall == "" {
		stall = "Senpan Tea House"
	}
	if _, err := s.store.CollectStamp(card.RallyID, card.ID, match.ID, card.ParticipantName, stall); err != nil {
		if errors.Is(err, store.ErrStampAlreadyCollected) {
			writeError(w, http.StatusConflict, "You've already collected this stamp")
			return
		}
		writeInternalError(w, "collect stamp", err)
		return
	}

	// Reload collected set, recompute completion, and broadcast so an admin viewing
	// the manager/logs sees the new collection live (this public path is excluded
	// from the adminMutationResource middleware).
	collected, err := s.store.ListCollectedStampIDs(card.ID)
	if err != nil {
		writeInternalError(w, "list collected", err)
		return
	}
	s.maybeComplete(card, rally, collected, now)
	s.broadcastResourceChanged("stamp-rallies")

	resp := buildPublicCard(rally, card, rally.Stamps, rally.Prizes, collected, now)
	writeJSON(w, http.StatusOK, model.StampSubmitResponse{
		Card:             resp,
		CollectedStampID: match.ID,
	})
}
