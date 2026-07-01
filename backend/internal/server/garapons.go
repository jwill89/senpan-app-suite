package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// ── Garapon admin (list + detail + CRUD + drawing links) ────────────────────
//
// A garapon is a festival lottery drum (see model.Garapon). Admins manage it like
// a raffle — create/edit/close — but instead of public sign-up, each player gets a
// private tokenized link (a GaraponPlayer) with a draw allowance. The public draw
// endpoints below need no auth; the token is the capability.

// handleGaraponsList returns every garapon (admin only).
//
//	Endpoint:  GET /api/garapons
//	Auth:      admin, or a user granted festival-garapon
//	Response:  {"garapons": [...]}
func (s *Server) handleGaraponsList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalGarapon) {
		return
	}
	garapons, err := s.store.ListGarapons()
	if err != nil {
		writeInternalError(w, "list garapons", err)
		return
	}
	writeJSON(w, http.StatusOK, model.GaraponsResponse{Garapons: garapons})
}

// handleGaraponDetail returns a single garapon with its prizes, drawing links,
// and full draw log (admin only).
//
//	Endpoint:  GET /api/garapons/{id}
//	Auth:      admin, or a user granted festival-garapon
//	Response:  {"garapon": Garapon, "players": [...], "draws": [...]}
func (s *Server) handleGaraponDetail(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalGarapon) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid garapon ID")
		return
	}
	garapon, err := s.store.GetGarapon(id)
	if err != nil {
		writeInternalError(w, "get garapon", err)
		return
	}
	if garapon == nil {
		writeError(w, http.StatusNotFound, "Garapon not found")
		return
	}
	players, err := s.store.ListGaraponPlayers(id)
	if err != nil {
		writeInternalError(w, "list garapon players", err)
		return
	}
	draws, err := s.store.ListGaraponDraws(id)
	if err != nil {
		writeInternalError(w, "list garapon draws", err)
		return
	}
	writeJSON(w, http.StatusOK, model.GaraponDetailResponse{Garapon: *garapon, Players: players, Draws: draws})
}

// garaponWriteRequest is the JSON body for creating (POST /api/garapons) or
// replacing (PUT /api/garapons/{id}) a garapon. The id comes from the path on PUT.
type garaponWriteRequest struct {
	Title           string               `json:"title"`
	Details         string               `json:"details"`
	GrandPrizeImage string               `json:"grand_prize_image"`
	StampRallyID    *int64               `json:"stamp_rally_id"` // optional link to an open rally
	Prizes          []model.GaraponPrize `json:"prizes"`
}

// resolveStampRallyLink validates an optional garapon→rally link: nil/0 means
// unlinked; a supplied id must be a real, OPEN rally (closed/unknown rallies are
// rejected). Writes the error and returns ok=false on any problem.
func (s *Server) resolveStampRallyLink(w http.ResponseWriter, id *int64) (*int64, bool) {
	if id == nil || *id == 0 {
		return nil, true
	}
	rally, err := s.store.GetStampRally(*id)
	if err != nil {
		writeInternalError(w, "get rally for link", err)
		return nil, false
	}
	if rally == nil {
		writeError(w, http.StatusBadRequest, "Linked stamp rally not found")
		return nil, false
	}
	if rally.Status != "open" {
		writeError(w, http.StatusBadRequest, "The linked stamp rally is closed")
		return nil, false
	}
	return id, true
}

// sanitizeGaraponPrizes trims/normalizes incoming prize rows: it drops blank rows,
// defaults an empty ball color, floors negative rates at 0, and enforces exactly
// one grand prize (defaulting the first when none is flagged). Returns a non-empty
// user-facing message when no usable prizes remain or more than one grand prize is
// flagged (so the handler can pass it straight to writeError).
func sanitizeGaraponPrizes(in []model.GaraponPrize) (prizes []model.GaraponPrize, errMsg string) {
	out := make([]model.GaraponPrize, 0, len(in))
	grandCount := 0
	for _, p := range in {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		color := strings.TrimSpace(p.BallColor)
		if color == "" {
			color = "#cccccc"
		}
		rate := p.Rate
		if rate < 0 {
			rate = 0
		}
		if p.IsGrand {
			grandCount++
		}
		out = append(out, model.GaraponPrize{Name: name, BallColor: color, Rate: rate, IsGrand: p.IsGrand})
	}
	if len(out) == 0 {
		return nil, "At least one prize is required"
	}
	if grandCount == 0 {
		out[0].IsGrand = true
	} else if grandCount > 1 {
		return nil, "Only one prize can be the grand prize"
	}
	return out, ""
}

// handleGaraponCreate creates a garapon.
//
//	Endpoint:  POST /api/garapons
//	Auth:      admin, or a user granted festival-garapon
//	Response:  201 {"garapon": Garapon}
func (s *Server) handleGaraponCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalGarapon) {
		return
	}
	req, err := readJSON[garaponWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Title is required")
		return
	}
	prizes, msg := sanitizeGaraponPrizes(req.Prizes)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	link, ok := s.resolveStampRallyLink(w, req.StampRallyID)
	if !ok {
		return
	}
	garapon := &model.Garapon{
		Title:           title,
		Details:         req.Details,
		GrandPrizeImage: req.GrandPrizeImage,
		StampRallyID:    link,
		Prizes:          prizes,
	}
	id, err := s.store.CreateGarapon(garapon)
	if err != nil {
		writeInternalError(w, "create garapon", err)
		return
	}
	garapon.ID = id
	garapon.Status = "open"
	writeJSON(w, http.StatusCreated, model.GaraponResponse{Garapon: *garapon})
}

// handleGaraponUpdate replaces a garapon's editable fields (status is not editable
// here and is preserved — use the close/reopen verbs).
//
//	Endpoint:  PUT /api/garapons/{id}
//	Auth:      admin, or a user granted festival-garapon
//	Response:  200 {"ok": true}
func (s *Server) handleGaraponUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalGarapon) {
		return
	}
	id, ok := pathInt64(w, r, "id", "garapon")
	if !ok {
		return
	}
	req, err := readJSON[garaponWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Title is required")
		return
	}
	prizes, msg := sanitizeGaraponPrizes(req.Prizes)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	link, ok := s.resolveStampRallyLink(w, req.StampRallyID)
	if !ok {
		return
	}
	garapon := &model.Garapon{
		ID:              id,
		Title:           title,
		Details:         req.Details,
		GrandPrizeImage: req.GrandPrizeImage,
		StampRallyID:    link,
		Prizes:          prizes,
	}
	if err := s.store.UpdateGarapon(garapon); err != nil {
		writeInternalError(w, "update garapon", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// handleGaraponDelete deletes a garapon. The grand-prize image is managed
// centrally on System → Images (the "Garapon" category), so the file is left
// intact — it may be reused.
//
//	Endpoint:  DELETE /api/garapons/{id}
//	Auth:      admin, or a user granted festival-garapon
//	Response:  204 No Content
func (s *Server) handleGaraponDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalGarapon) {
		return
	}
	id, ok := pathInt64(w, r, "id", "garapon")
	if !ok {
		return
	}
	if _, err := s.store.DeleteGarapon(id); err != nil {
		writeInternalError(w, "delete garapon", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setGaraponStatus applies a status change and responds with {ok, status}. Shared
// by the close and reopen verb handlers.
func (s *Server) setGaraponStatus(w http.ResponseWriter, r *http.Request, status string) {
	if !s.requirePermission(w, r, permFestivalGarapon) {
		return
	}
	id, ok := pathInt64(w, r, "id", "garapon")
	if !ok {
		return
	}
	if err := s.store.SetGaraponStatus(id, status); err != nil {
		writeInternalError(w, "set garapon status", err)
		return
	}
	writeJSON(w, http.StatusOK, model.StatusResponse{OK: true, Status: status})
}

// handleGaraponClose closes a garapon (no further draws).
//
//	Endpoint:  POST /api/garapons/{id}/close
//	Auth:      admin, or a user granted festival-garapon
//	Response:  200 {"ok": true, "status": "closed"}
func (s *Server) handleGaraponClose(w http.ResponseWriter, r *http.Request) {
	s.setGaraponStatus(w, r, "closed")
}

// handleGaraponReopen reopens a closed garapon.
//
//	Endpoint:  POST /api/garapons/{id}/reopen
//	Auth:      admin, or a user granted festival-garapon
//	Response:  200 {"ok": true, "status": "open"}
func (s *Server) handleGaraponReopen(w http.ResponseWriter, r *http.Request) {
	s.setGaraponStatus(w, r, "open")
}

// garaponPlayerCreateRequest is the JSON body for POST /api/garapons/{id}/players.
type garaponPlayerCreateRequest struct {
	PlayerName string `json:"player_name"`
	MaxDraws   int    `json:"max_draws"`
}

// handleGaraponPlayerCreate issues a new per-player drawing link.
//
//	Endpoint:  POST /api/garapons/{id}/players
//	Auth:      admin, or a user granted festival-garapon
//	Response:  201 {"player": GaraponPlayer}
func (s *Server) handleGaraponPlayerCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalGarapon) {
		return
	}
	garaponID, ok := pathInt64(w, r, "id", "garapon")
	if !ok {
		return
	}
	req, err := readJSON[garaponPlayerCreateRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name := strings.TrimSpace(req.PlayerName)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Player name is required")
		return
	}
	maxDraws := req.MaxDraws
	if maxDraws < 1 {
		maxDraws = 1
	}
	garapon, err := s.store.GetGarapon(garaponID)
	if err != nil {
		writeInternalError(w, "get garapon for player", err)
		return
	}
	if garapon == nil {
		writeError(w, http.StatusNotFound, "Garapon not found")
		return
	}
	player, err := s.store.CreateGaraponPlayer(garaponID, name, maxDraws)
	if err != nil {
		writeInternalError(w, "create garapon player", err)
		return
	}
	// If the garapon is linked to an open stamp rally, also issue this participant
	// a stamp card USING THE SAME TOKEN, so one hash serves both /garapon/<token>
	// and /stamp-card/<token>. Best-effort: a rally that's since closed/vanished
	// just yields no card (the drawing link is still valid on its own).
	if garapon.StampRallyID != nil {
		if rally, _ := s.store.GetStampRally(*garapon.StampRallyID); rally != nil && rally.Status == "open" {
			if card, err := s.store.IssueRallyCardWithToken(*garapon.StampRallyID, name, player.Token); err == nil && card != nil {
				if err := s.store.SetPlayerStampCard(player.ID, card.ID); err == nil {
					player.StampCardToken = card.Token
				}
				// A stamp card was issued — let admins viewing the rally see it live.
				s.broadcastResourceChanged("stamp-rallies")
			}
		}
	}
	writeJSON(w, http.StatusCreated, model.GaraponPlayerResponse{Player: *player})
}

// handleGaraponPlayerDelete removes a garapon's drawing link.
//
//	Endpoint:  DELETE /api/garapons/{id}/players/{playerId}
//	Auth:      admin, or a user granted festival-garapon
//	Response:  204 No Content
func (s *Server) handleGaraponPlayerDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalGarapon) {
		return
	}
	garaponID, ok := pathInt64(w, r, "id", "garapon")
	if !ok {
		return
	}
	playerID, ok := pathInt64(w, r, "playerId", "player")
	if !ok {
		return
	}
	existing, err := s.store.GetGaraponPlayerByID(playerID)
	if err != nil {
		writeInternalError(w, "get garapon player", err)
		return
	}
	// The link must belong to the garapon in the path — otherwise the
	// open/closed check below would read the wrong garapon's status and could
	// force-delete a drawn link from a different, still-open garapon.
	if existing == nil || existing.GaraponID != garaponID {
		writeError(w, http.StatusNotFound, "Drawing link not found")
		return
	}
	// A closed garapon can be cleaned up: any link may be deleted, and its
	// draws stay in the log (garapon_draws.player_id is ON DELETE SET NULL).
	// While the garapon is open, a link that has already drawn can't be deleted.
	garapon, err := s.store.GetGarapon(garaponID)
	if err != nil {
		writeInternalError(w, "get garapon for delete player", err)
		return
	}
	closed := garapon != nil && garapon.Status == "closed"
	if existing.DrawsUsed > 0 && !closed {
		writeError(w, http.StatusConflict,
			"This player has already drawn and can't be deleted while the garapon is open")
		return
	}
	if _, err := s.store.DeleteGaraponPlayer(playerID, closed); err != nil {
		writeInternalError(w, "delete garapon player", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Garapon public (tokenized player view + draw) ───────────────────────────

// toPublicGarapon copies a garapon for the player view, zeroing each prize's Rate
// so the configured odds aren't exposed. The trimmed wire shape (no odds, no
// admin-only fields) is model.PublicGarapon.
func toPublicGarapon(g *model.Garapon) model.PublicGarapon {
	prizes := make([]model.GaraponPrize, len(g.Prizes))
	for i, p := range g.Prizes {
		p.Rate = 0
		prizes[i] = p
	}
	return model.PublicGarapon{
		ID:              g.ID,
		Title:           g.Title,
		Details:         g.Details,
		GrandPrizeImage: g.GrandPrizeImage,
		Status:          g.Status,
		Prizes:          prizes,
	}
}

// loadGaraponByToken resolves a player token to its player + garapon, writing the
// 404 itself when the token is unknown. Returns (player, garapon, ok).
func (s *Server) loadGaraponByToken(w http.ResponseWriter, token string) (*model.GaraponPlayer, *model.Garapon, bool) {
	player, err := s.store.GetGaraponPlayerByToken(token)
	if err != nil {
		writeInternalError(w, "get garapon player by token", err)
		return nil, nil, false
	}
	if player == nil {
		writeError(w, http.StatusNotFound, "Drawing link not found")
		return nil, nil, false
	}
	garapon, err := s.store.GetGarapon(player.GaraponID)
	if err != nil {
		writeInternalError(w, "get garapon for token", err)
		return nil, nil, false
	}
	if garapon == nil {
		writeError(w, http.StatusNotFound, "Garapon not found")
		return nil, nil, false
	}
	return player, garapon, true
}

// handleGaraponPublic returns the player-facing garapon view for a drawing token:
// the garapon (no odds), the player's name + draw allowance/usage, and their own
// draw history.
//
//	Endpoint:  GET /api/garapon/{token}
//	Auth:      public (the token is the capability)
//	Response:  {"garapon": {...}, "player": {...}, "draws": [...]}
func (s *Server) handleGaraponPublic(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	player, garapon, ok := s.loadGaraponByToken(w, token)
	if !ok {
		return
	}
	draws, err := s.store.ListPlayerDraws(player.ID)
	if err != nil {
		writeInternalError(w, "list player draws", err)
		return
	}
	writeJSON(w, http.StatusOK, model.GaraponPublicResponse{
		Garapon: toPublicGarapon(garapon),
		Player:  model.GaraponPublicPlayer{PlayerName: player.PlayerName, MaxDraws: player.MaxDraws, DrawsUsed: player.DrawsUsed},
		Draws:   draws,
	})
}

// handleGaraponDraw performs one authoritative draw for a token. The store
// re-checks the open status + remaining-draw cap atomically and weighted-picks the
// prize, so the client can't bias the odds or exceed the allowance.
//
//	Endpoint:  POST /api/garapon/{token}/draw
//	Auth:      public (the token is the capability)
//	Response:  {"draw": GaraponDraw, "draws_used": int, "max_draws": int}
func (s *Server) handleGaraponDraw(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	player, _, ok := s.loadGaraponByToken(w, token)
	if !ok {
		return
	}

	draw, err := s.store.RecordGaraponDraw(player.ID)
	switch {
	case errors.Is(err, store.ErrGaraponClosed):
		writeError(w, http.StatusBadRequest, "This garapon is closed")
		return
	case errors.Is(err, store.ErrGaraponNoDraws):
		writeError(w, http.StatusConflict, "No draws remaining")
		return
	case errors.Is(err, store.ErrGaraponNoPrizes):
		writeError(w, http.StatusBadRequest, "This garapon has no prizes configured")
		return
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "Drawing link not found")
		return
	case err != nil:
		writeInternalError(w, "record garapon draw", err)
		return
	}

	// A draw mutates the admin-visible draw log + the player's used count, but this
	// is the *public* draw path (the token is the capability), so it's excluded from
	// the adminMutationResource middleware that invalidates admin views. Broadcast
	// the "garapons" signal explicitly so an admin viewing the garapon detail sees
	// the new draw appear live (the refetch re-applies the per-feature guard).
	s.broadcastResourceChanged("garapons")

	// Exactly one draw was just recorded, so the fresh usage is player.DrawsUsed+1
	// (its allowance is unchanged) — no need to reload the player.
	writeJSON(w, http.StatusOK, model.GaraponDrawResponse{
		Draw:      *draw,
		DrawsUsed: player.DrawsUsed + 1,
		MaxDraws:  player.MaxDraws,
	})
}
