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
	writeJSON(w, http.StatusOK, map[string]any{"garapons": garapons})
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
	writeJSON(w, http.StatusOK, map[string]any{"garapon": garapon, "players": players, "draws": draws})
}

// garaponRequest is the JSON body for POST /api/garapons.
// Action: "create", "update", "delete", or "set_status".
type garaponRequest struct {
	Action          string               `json:"action"`
	ID              int64                `json:"id"`
	Title           string               `json:"title"`
	Details         string               `json:"details"`
	GrandPrizeImage string               `json:"grand_prize_image"`
	Status          string               `json:"status"`
	Prizes          []model.GaraponPrize `json:"prizes"`
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

// handleGaraponsAction processes garapon CRUD + status changes.
//
//	Endpoint:  POST /api/garapons
//	Auth:      admin, or a user granted festival-garapon
//	Request:   {"action": "create"|"update"|"delete"|"set_status", ...}
func (s *Server) handleGaraponsAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalGarapon) {
		return
	}
	req, err := readJSON[garaponRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "create":
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
		garapon := &model.Garapon{
			Title:           title,
			Details:         req.Details,
			GrandPrizeImage: req.GrandPrizeImage,
			Prizes:          prizes,
		}
		id, err := s.store.CreateGarapon(garapon)
		if err != nil {
			writeInternalError(w, "create garapon", err)
			return
		}
		garapon.ID = id
		garapon.Status = "open"
		writeJSON(w, http.StatusCreated, map[string]any{"garapon": garapon})

	case "update":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Garapon id is required")
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
		garapon := &model.Garapon{
			ID:              req.ID,
			Title:           title,
			Details:         req.Details,
			GrandPrizeImage: req.GrandPrizeImage,
			Prizes:          prizes,
		}
		if err := s.store.UpdateGarapon(garapon); err != nil {
			writeInternalError(w, "update garapon", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "delete":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Garapon id is required")
			return
		}
		deleted, err := s.store.DeleteGarapon(req.ID)
		if err != nil {
			writeInternalError(w, "delete garapon", err)
			return
		}
		// The grand-prize image is managed centrally on System → Images (the
		// "Garapon" category), so the file is left intact — it may be reused.
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	case "set_status":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Garapon id is required")
			return
		}
		if req.Status != "open" && req.Status != "closed" {
			writeError(w, http.StatusBadRequest, "Status must be \"open\" or \"closed\"")
			return
		}
		if err := s.store.SetGaraponStatus(req.ID, req.Status); err != nil {
			writeInternalError(w, "set garapon status", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": req.Status})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, update, delete, set_status")
	}
}

// garaponPlayersRequest is the JSON body for POST /api/garapons/{id}/players.
// Action: "create_player" or "delete_player".
type garaponPlayersRequest struct {
	Action     string `json:"action"`
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
	MaxDraws   int    `json:"max_draws"`
}

// handleGaraponPlayers creates or deletes a garapon's drawing links.
//
//	Endpoint:  POST /api/garapons/{id}/players
//	Auth:      admin, or a user granted festival-garapon
//	Request:   {"action":"create_player","player_name":"...","max_draws":5}
//	           {"action":"delete_player","player_id":1}
func (s *Server) handleGaraponPlayers(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalGarapon) {
		return
	}
	garaponID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid garapon ID")
		return
	}
	req, err := readJSON[garaponPlayersRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "create_player":
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
		writeJSON(w, http.StatusCreated, map[string]any{"player": player})

	case "delete_player":
		if req.PlayerID <= 0 {
			writeError(w, http.StatusBadRequest, "Player id is required")
			return
		}
		existing, err := s.store.GetGaraponPlayerByID(req.PlayerID)
		if err != nil {
			writeInternalError(w, "get garapon player", err)
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
		if existing != nil && existing.DrawsUsed > 0 && !closed {
			writeError(w, http.StatusConflict,
				"This player has already drawn and can't be deleted while the garapon is open")
			return
		}
		deleted, err := s.store.DeleteGaraponPlayer(req.PlayerID, closed)
		if err != nil {
			writeInternalError(w, "delete garapon player", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create_player, delete_player")
	}
}

// ── Garapon public (tokenized player view + draw) ───────────────────────────

// publicGarapon is the trimmed garapon shape sent to players: it carries the
// prizes (names + ball colors + which is grand) but never the appearance rates —
// the odds stay admin-only.
type publicGarapon struct {
	ID              int64                `json:"id"`
	Title           string               `json:"title"`
	Details         string               `json:"details"`
	GrandPrizeImage string               `json:"grand_prize_image"`
	Status          string               `json:"status"`
	Prizes          []model.GaraponPrize `json:"prizes"`
}

// toPublicGarapon copies a garapon for the player view, zeroing each prize's Rate
// so the configured odds aren't exposed.
func toPublicGarapon(g *model.Garapon) publicGarapon {
	prizes := make([]model.GaraponPrize, len(g.Prizes))
	for i, p := range g.Prizes {
		p.Rate = 0
		prizes[i] = p
	}
	return publicGarapon{
		ID:              g.ID,
		Title:           g.Title,
		Details:         g.Details,
		GrandPrizeImage: g.GrandPrizeImage,
		Status:          g.Status,
		Prizes:          prizes,
	}
}

// publicPlayer is the trimmed player shape sent to the public view (no token —
// the caller already holds it in the URL).
type publicPlayer struct {
	PlayerName string `json:"player_name"`
	MaxDraws   int    `json:"max_draws"`
	DrawsUsed  int    `json:"draws_used"`
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
	writeJSON(w, http.StatusOK, map[string]any{
		"garapon": toPublicGarapon(garapon),
		"player":  publicPlayer{PlayerName: player.PlayerName, MaxDraws: player.MaxDraws, DrawsUsed: player.DrawsUsed},
		"draws":   draws,
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

	// Reload the player for the fresh used-count so the client can update the
	// remaining-draws display without a second request.
	updated, err := s.store.GetGaraponPlayerByID(player.ID)
	usedCount := player.DrawsUsed + 1
	maxDraws := player.MaxDraws
	if err == nil && updated != nil {
		usedCount = updated.DrawsUsed
		maxDraws = updated.MaxDraws
	}
	writeJSON(w, http.StatusOK, map[string]any{"draw": draw, "draws_used": usedCount, "max_draws": maxDraws})
}
