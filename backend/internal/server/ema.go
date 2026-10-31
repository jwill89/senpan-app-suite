package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// -- Ema Plaques --------------------------------------------------------------
//
// A festival's plaque board (see model.EmaBoard). Staff with festival-ema set the
// board up, review the plaques designed on the site, and approve them - which is
// when the app posts them to the window's Discord webhook and deletes the stored
// image, Discord holding the posted copy. The artist works the request queue with
// festival-ema-requests. Visitors submit from the festival dashboard through the
// singular /api/ema/... paths, which sit outside the plural admin prefixes so
// adminMutationResource leaves them alone; they broadcast their own invalidation.

// emaImageRelDir is where site plaques wait for review, under the web root. Not a
// managed image-library category: these are visitors' uploads, kept only until
// they are posted or rejected.
const emaImageRelDir = "images/ema_plaques"

// maxEmaImageBytes bounds a plaque upload. The designer exports at most 2048 px
// wide, which as a PNG is well under this.
const maxEmaImageBytes = 12 << 20

// maxEmaImageSide bounds a plaque's pixel size, checked before the image is kept.
const maxEmaImageSide = 4096

// maxEmaDescriptionLen bounds an artist request, counted in runes.
const maxEmaDescriptionLen = 2000

// maxEmaArtistImageBytes bounds the artist's finished plaque: Discord's default
// upload limit, so nothing is accepted that the post would then be refused for.
const maxEmaArtistImageBytes = 10 << 20

// maxEmaArtistLimit bounds a window's artist_limit - well past any one artist's
// day, so a typo of an extra zero is caught rather than stored.
const maxEmaArtistLimit = 1000

// emaWindowRequest is one window in a board save.
type emaWindowRequest struct {
	ID          int64  `json:"id"`
	Label       string `json:"label"`
	StartAt     string `json:"start_at"`
	EndAt       string `json:"end_at"`
	WebhookURL  string `json:"webhook_url"`
	ArtistLimit int    `json:"artist_limit"`
}

// emaBoardWriteRequest is the JSON body for creating or replacing a board.
type emaBoardWriteRequest struct {
	FestivalMapID int64              `json:"festival_map_id"`
	Title         string             `json:"title"`
	TemplateImage string             `json:"template_image"`
	Instructions  string             `json:"instructions"`
	DiscordLink   string             `json:"discord_link"`
	AllowSelf     bool               `json:"allow_self"`
	AllowSite     bool               `json:"allow_site"`
	AllowArtist   bool               `json:"allow_artist"`
	ArtistName    string             `json:"artist_name"`
	Windows       []emaWindowRequest `json:"windows"`
}

// emaBoardFromRequest validates a save and builds the board, or returns the
// message to refuse it with.
func emaBoardFromRequest(req emaBoardWriteRequest) (*model.EmaBoard, string) {
	b := &model.EmaBoard{
		FestivalMapID: req.FestivalMapID,
		Title:         strings.TrimSpace(req.Title),
		TemplateImage: strings.TrimSpace(req.TemplateImage),
		Instructions:  req.Instructions,
		DiscordLink:   strings.TrimSpace(req.DiscordLink),
		AllowSelf:     req.AllowSelf,
		AllowSite:     req.AllowSite,
		AllowArtist:   req.AllowArtist,
		ArtistName:    strings.TrimSpace(req.ArtistName),
		Status:        "open",
	}
	if b.FestivalMapID <= 0 {
		return nil, "Pick the festival this board belongs to"
	}
	if b.ArtistName == "" {
		b.ArtistName = "Tara"
	}
	if !b.AllowSelf && !b.AllowSite && !b.AllowArtist {
		return nil, "Offer at least one way to write a plaque"
	}
	// Both of the template's uses - downloading it, and designing on it - need it.
	if (b.AllowSelf || b.AllowSite) && b.TemplateImage == "" {
		return nil, "Pick the plaque template image"
	}
	if b.DiscordLink != "" && !isHTTPURL(b.DiscordLink) {
		return nil, "The Discord link must be a web address"
	}
	if len(req.Windows) == 0 {
		return nil, "Add at least one day to take plaques on"
	}
	type span struct {
		label      string
		start, end time.Time
	}
	spans := make([]span, 0, len(req.Windows))
	for i, wr := range req.Windows {
		w := model.EmaWindow{
			ID:          wr.ID,
			Label:       strings.TrimSpace(wr.Label),
			WebhookURL:  strings.TrimSpace(wr.WebhookURL),
			ArtistLimit: wr.ArtistLimit,
		}
		if w.Label == "" {
			w.Label = fmt.Sprintf("Day %d", i+1)
		}
		start, err1 := time.Parse(time.RFC3339, strings.TrimSpace(wr.StartAt))
		end, err2 := time.Parse(time.RFC3339, strings.TrimSpace(wr.EndAt))
		if err1 != nil || err2 != nil {
			return nil, w.Label + " needs a start and an end"
		}
		if !end.After(start) {
			return nil, w.Label + " ends before it starts"
		}
		if w.WebhookURL != "" && !isDiscordWebhookURL(w.WebhookURL) {
			return nil, w.Label + "'s webhook isn't a Discord webhook URL"
		}
		if w.ArtistLimit < 0 || w.ArtistLimit > maxEmaArtistLimit {
			return nil, fmt.Sprintf("%s's artist request limit must be 0 (no limit) to %d", w.Label, maxEmaArtistLimit)
		}
		w.StartAt, w.EndAt = start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)
		b.Windows = append(b.Windows, w)
		spans = append(spans, span{w.Label, start, end})
	}
	// Overlapping windows would leave "which day is this plaque for?" with two
	// answers, and the one-per-window rule with a loophole.
	sort.Slice(spans, func(i, j int) bool { return spans[i].start.Before(spans[j].start) })
	for i := 1; i < len(spans); i++ {
		if spans[i].start.Before(spans[i-1].end) {
			return nil, spans[i-1].label + " and " + spans[i].label + " overlap"
		}
	}
	return b, ""
}

// handleEmaBoardsList returns every board.
//
//	Endpoint:  GET /api/ema-boards
//	Auth:      festival-ema
func (s *Server) handleEmaBoardsList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalEma) {
		return
	}
	boards, err := s.store.ListEmaBoards(r.Context())
	if err != nil {
		writeInternalError(w, "list ema boards", err)
		return
	}
	writeJSON(w, http.StatusOK, model.EmaBoardsResponse{Boards: boards})
}

// handleEmaBoardDetail returns one board with its windows (webhooks included).
//
//	Endpoint:  GET /api/ema-boards/{id}
//	Auth:      festival-ema
func (s *Server) handleEmaBoardDetail(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalEma) {
		return
	}
	id, ok := pathInt64(w, r, "id", "board")
	if !ok {
		return
	}
	b, err := s.store.GetEmaBoard(r.Context(), id)
	if err != nil {
		writeInternalError(w, "get ema board", err)
		return
	}
	if b == nil {
		writeError(w, http.StatusNotFound, "Plaque board not found")
		return
	}
	writeJSON(w, http.StatusOK, model.EmaBoardResponse{Board: *b})
}

// checkEmaFestival confirms the festival a board is filed under exists, and that
// no other board already claims it. exceptID is the board being saved (0 on create).
func (s *Server) checkEmaFestival(w http.ResponseWriter, r *http.Request, mapID, exceptID int64) bool {
	m, err := s.store.GetFestivalMap(r.Context(), mapID)
	if err != nil {
		writeInternalError(w, "get festival for ema board", err)
		return false
	}
	if m == nil {
		writeError(w, http.StatusBadRequest, "That festival doesn't exist")
		return false
	}
	other, err := s.store.GetEmaBoardForMap(r.Context(), mapID)
	if err != nil {
		writeInternalError(w, "check ema board for festival", err)
		return false
	}
	if other != nil && other.ID != exceptID {
		writeError(w, http.StatusConflict, "That festival already has a plaque board")
		return false
	}
	return true
}

// handleEmaBoardCreate creates a board (windows inline). A new board is open.
//
//	Endpoint:  POST /api/ema-boards
//	Auth:      festival-ema
//	Response:  201 EmaBoardResponse
func (s *Server) handleEmaBoardCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalEma) {
		return
	}
	req, err := readJSON[emaBoardWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	b, msg := emaBoardFromRequest(req)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if !s.checkEmaFestival(w, r, b.FestivalMapID, 0) {
		return
	}
	id, err := s.store.CreateEmaBoard(r.Context(), b)
	if err != nil {
		writeInternalError(w, "create ema board", err)
		return
	}
	created, err := s.store.GetEmaBoard(r.Context(), id)
	if err != nil || created == nil {
		writeInternalError(w, "reload ema board", err)
		return
	}
	writeJSON(w, http.StatusCreated, model.EmaBoardResponse{Board: *created})
}

// handleEmaBoardUpdate replaces a board's editable fields and reconciles its
// windows by id (status is set through PATCH).
//
//	Endpoint:  PUT /api/ema-boards/{id}
//	Auth:      festival-ema
func (s *Server) handleEmaBoardUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalEma) {
		return
	}
	id, ok := pathInt64(w, r, "id", "board")
	if !ok {
		return
	}
	req, err := readJSON[emaBoardWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	b, msg := emaBoardFromRequest(req)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	existing, err := s.store.GetEmaBoard(r.Context(), id)
	if err != nil {
		writeInternalError(w, "get ema board", err)
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "Plaque board not found")
		return
	}
	if !s.checkEmaFestival(w, r, b.FestivalMapID, id) {
		return
	}
	b.ID = id
	if err := s.store.UpdateEmaBoard(r.Context(), b); err != nil {
		if errors.Is(err, store.ErrEmaWindowNotOnBoard) {
			writeError(w, http.StatusBadRequest, "That day doesn't belong to this board - reload and try again")
			return
		}
		writeInternalError(w, "update ema board", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// emaStatusRequest is the JSON body for PATCH /api/ema-boards/{id}.
type emaStatusRequest struct {
	Status string `json:"status"`
}

// handleEmaBoardPatch opens or closes a board. A closed board takes no plaques and
// leaves the festival dashboard; its review queue stays workable.
//
//	Endpoint:  PATCH /api/ema-boards/{id}
//	Auth:      festival-ema
func (s *Server) handleEmaBoardPatch(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalEma) {
		return
	}
	id, ok := pathInt64(w, r, "id", "board")
	if !ok {
		return
	}
	req, err := readJSON[emaStatusRequest](w, r)
	if err != nil || (req.Status != "open" && req.Status != "closed") {
		writeError(w, http.StatusBadRequest, `Status must be "open" or "closed"`)
		return
	}
	if err := s.store.SetEmaBoardStatus(r.Context(), id, req.Status); err != nil {
		writeInternalError(w, "set ema board status", err)
		return
	}
	writeJSON(w, http.StatusOK, model.StatusResponse{Status: req.Status})
}

// handleEmaBoardDelete deletes a board, its windows and plaques, and the images
// still waiting for review.
//
//	Endpoint:  DELETE /api/ema-boards/{id}
//	Auth:      festival-ema
func (s *Server) handleEmaBoardDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalEma) {
		return
	}
	id, ok := pathInt64(w, r, "id", "board")
	if !ok {
		return
	}
	// Detached: the rows and the files go together, so a client leaving between the
	// two must not strand the files.
	ctx := context.WithoutCancel(r.Context())
	deleted, images, err := s.store.DeleteEmaBoard(ctx, id)
	if err != nil {
		writeInternalError(w, "delete ema board", err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "Plaque board not found")
		return
	}
	for _, img := range images {
		s.removeEmaImage(img)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleEmaPlaquesList returns a board's plaques - the review queue and the record.
//
//	Endpoint:  GET /api/ema-boards/{id}/plaques
//	Auth:      festival-ema
func (s *Server) handleEmaPlaquesList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalEma) {
		return
	}
	id, ok := pathInt64(w, r, "id", "board")
	if !ok {
		return
	}
	plaques, err := s.store.ListEmaPlaques(r.Context(), id)
	if err != nil {
		writeInternalError(w, "list ema plaques", err)
		return
	}
	writeJSON(w, http.StatusOK, model.EmaPlaquesResponse{Plaques: plaques})
}

// loadReviewablePlaque resolves the {plaqueId} of a review route to a site plaque
// on the {id} board, writing the error itself when it isn't one.
func (s *Server) loadReviewablePlaque(w http.ResponseWriter, r *http.Request) (*model.EmaPlaque, bool) {
	boardID, ok := pathInt64(w, r, "id", "board")
	if !ok {
		return nil, false
	}
	plaqueID, ok := pathInt64(w, r, "plaqueId", "plaque")
	if !ok {
		return nil, false
	}
	p, err := s.store.GetEmaPlaque(r.Context(), plaqueID)
	if err != nil {
		writeInternalError(w, "get ema plaque", err)
		return nil, false
	}
	if p == nil || p.BoardID != boardID || p.Kind != model.EmaKindSite {
		writeError(w, http.StatusNotFound, "Plaque not found")
		return nil, false
	}
	if p.Status != model.EmaStatusPending {
		writeError(w, http.StatusConflict, "This plaque has already been reviewed")
		return nil, false
	}
	return p, true
}

// handleEmaPlaqueApprove posts a site plaque to its window's Discord webhook, then
// records it as posted and deletes its image.
//
//	Endpoint:  POST /api/ema-boards/{id}/plaques/{plaqueId}/approve
//	Auth:      festival-ema
//	Response:  200 EmaPlaqueResponse; 409 already reviewed or being posted; 502 Discord refused
//
// The plaque is claimed (pending -> posting) before the post, so a second staff
// member approving it at the same moment is refused rather than posting it twice.
// A failed post hands it back to pending to try again.
func (s *Server) handleEmaPlaqueApprove(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalEma) {
		return
	}
	p, ok := s.loadReviewablePlaque(w, r)
	if !ok {
		return
	}
	board, err := s.store.GetEmaBoard(r.Context(), p.BoardID)
	if err != nil || board == nil {
		writeInternalError(w, "get ema board for approval", err)
		return
	}
	webhook := emaWindowWebhook(board, p.WindowID)
	if webhook == "" {
		writeError(w, http.StatusBadRequest, "Set a webhook for "+p.WindowLabel+" before approving its plaques")
		return
	}
	data, err := s.readEmaImage(p.Image)
	if err != nil {
		writeInternalError(w, "read ema plaque image", err)
		return
	}

	// From the claim on, the steps belong together: a client that leaves must not
	// strand the plaque in "posting", nor cut the Discord post off half-sent.
	ctx := context.WithoutCancel(r.Context())
	claimed, err := s.store.SetEmaPlaqueStatus(ctx, p.ID, model.EmaStatusPending, model.EmaStatusPosting)
	if err != nil {
		writeInternalError(w, "claim ema plaque", err)
		return
	}
	if !claimed {
		writeError(w, http.StatusConflict, "Someone else is reviewing this plaque")
		return
	}

	who := model.ParticipantLabel(p.CharacterName, p.World)
	postErr := s.postEmaToDiscord(ctx, webhookTarget{Kind: "ema_plaque", Name: who}, webhook,
		"Hung by **"+who+"** - "+p.WindowLabel,
		discordFile{Name: "ema.png", ContentType: "image/png", Data: data})
	if postErr != nil {
		if _, err := s.store.SetEmaPlaqueStatus(ctx, p.ID, model.EmaStatusPosting, model.EmaStatusPending); err != nil {
			slog.Error("release ema plaque after a failed post", "plaque_id", p.ID, "error", err)
		}
		if errors.Is(postErr, errWebhookAmbiguous) {
			writeError(w, http.StatusBadGateway,
				"Discord didn't answer, so the plaque may or may not have posted. Check the channel before approving it again.")
			slog.Error("ema plaque post ambiguous", "plaque_id", p.ID, "error", postErr)
			return
		}
		writeUpstreamError(w, "post ema plaque", postErr)
		return
	}

	if _, err := s.store.FinishEmaPlaque(ctx, p.ID, model.EmaStatusPosting, model.EmaStatusPosted, s.actorName(r)); err != nil {
		// Posted, but not recorded as such: say so rather than invite a second post.
		slog.Error("record ema plaque as posted", "plaque_id", p.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "The plaque was posted, but recording it failed. Don't approve it again.")
		return
	}
	s.removeEmaImage(p.Image)
	slog.Info("ema plaque posted", "plaque_id", p.ID, "board_id", p.BoardID, "by", s.actorName(r))
	s.writeEmaPlaque(ctx, w, p.ID)
}

// handleEmaPlaqueReject refuses a site plaque and deletes its image. Its author
// may then submit another for the same window.
//
//	Endpoint:  POST /api/ema-boards/{id}/plaques/{plaqueId}/reject
//	Auth:      festival-ema
func (s *Server) handleEmaPlaqueReject(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalEma) {
		return
	}
	p, ok := s.loadReviewablePlaque(w, r)
	if !ok {
		return
	}
	ctx := context.WithoutCancel(r.Context())
	done, err := s.store.FinishEmaPlaque(ctx, p.ID, model.EmaStatusPending, model.EmaStatusRejected, s.actorName(r))
	if err != nil {
		writeInternalError(w, "reject ema plaque", err)
		return
	}
	if !done {
		writeError(w, http.StatusConflict, "This plaque has already been reviewed")
		return
	}
	s.removeEmaImage(p.Image)
	s.writeEmaPlaque(ctx, w, p.ID)
}

// emaWindowWebhook returns the webhook of the window a plaque was made for, or ""
// when the window has none or has since been deleted.
func emaWindowWebhook(board *model.EmaBoard, windowID *int64) string {
	for _, win := range board.Windows {
		if windowID != nil && win.ID == *windowID {
			return win.WebhookURL
		}
	}
	return ""
}

// postEmaToDiscord posts a plaque to a window's webhook: an embed carrying the
// image as an attachment, sent through the shared funnel so the dry-run and
// override valves apply.
func (s *Server) postEmaToDiscord(ctx context.Context, target webhookTarget, webhook, description string, file discordFile) error {
	embed := newEmbed().title("An Ema plaque").description(description).build()
	// Set directly: the builder takes web URLs only, and this names the upload.
	embed.Image = &discordEmbedImage{URL: "attachment://" + file.Name}
	return s.postDiscordWebhookFile(ctx, target, webhook, discordWebhookPayload{Embeds: []discordEmbed{embed}}, file)
}

// writeEmaPlaque answers with a plaque's current row.
func (s *Server) writeEmaPlaque(ctx context.Context, w http.ResponseWriter, id int64) {
	p, err := s.store.GetEmaPlaque(ctx, id)
	if err != nil || p == nil {
		writeInternalError(w, "reload ema plaque", err)
		return
	}
	writeJSON(w, http.StatusOK, model.EmaPlaqueResponse{Plaque: *p})
}

// handleEmaRequestsList returns the artist's queue across every board.
//
//	Endpoint:  GET /api/ema-requests
//	Auth:      festival-ema-requests
func (s *Server) handleEmaRequestsList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalEmaRequests) {
		return
	}
	plaques, err := s.store.ListEmaArtistRequests(r.Context())
	if err != nil {
		writeInternalError(w, "list ema requests", err)
		return
	}
	writeJSON(w, http.StatusOK, model.EmaPlaquesResponse{Plaques: plaques})
}

// emaRequestPatch is the JSON body for PATCH /api/ema-requests/{id}.
type emaRequestPatch struct {
	Complete bool `json:"complete"`
}

// handleEmaRequestPatch marks an artist request complete, or reopens it.
//
//	Endpoint:  PATCH /api/ema-requests/{id}
//	Auth:      festival-ema-requests
func (s *Server) handleEmaRequestPatch(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalEmaRequests) {
		return
	}
	id, ok := pathInt64(w, r, "id", "request")
	if !ok {
		return
	}
	req, err := readJSON[emaRequestPatch](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	applied, err := s.store.SetEmaRequestComplete(r.Context(), id, req.Complete, s.actorName(r))
	if err != nil {
		writeInternalError(w, "set ema request complete", err)
		return
	}
	if !applied {
		writeError(w, http.StatusConflict, "That request has already changed - reload the list")
		return
	}
	s.writeEmaPlaque(r.Context(), w, id)
}

// emaArtistImageName names the artist's finished plaque in the Discord post by its
// sniffed type, or returns "" for anything that isn't an image Discord shows inline.
func emaArtistImageName(contentType string) string {
	switch contentType {
	case "image/png":
		return "ema.png"
	case "image/jpeg":
		return "ema.jpg"
	case "image/gif":
		return "ema.gif"
	case "image/webp":
		return "ema.webp"
	}
	return ""
}

// handleEmaRequestPost posts the artist's finished plaque for an open request
// straight to its window's Discord webhook, crediting the visitor who asked and the
// artist who made it, and marks the request complete. There is no review step - the
// artist is staff - and nothing is kept: the upload is read into memory, posted,
// and dropped. Marking a request complete without posting (PATCH) still works, for
// a plaque the artist posts by hand.
//
//	Endpoint:  POST /api/ema-requests/{id}/post
//	Auth:      festival-ema-requests
//	Request:   multipart: image (PNG, JPEG, GIF or WebP, max 10 MB)
//	Response:  200 EmaPlaqueResponse (status "complete"); 400 no webhook / not an
//	           image; 404 not a request; 409 not open; 502 Discord refused
//
// The request is claimed (requested -> posting) before the post, like an approval,
// so a double click can't post it twice; a failed post hands it back.
func (s *Server) handleEmaRequestPost(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalEmaRequests) {
		return
	}
	id, ok := pathInt64(w, r, "id", "request")
	if !ok {
		return
	}
	// Under the memory cap, so the upload never touches the disk.
	r.Body = http.MaxBytesReader(w, r.Body, maxEmaArtistImageBytes+(1<<20))
	if err := r.ParseMultipartForm(maxEmaArtistImageBytes + (1 << 20)); err != nil { //nolint:gosec // the body is capped by the MaxBytesReader just above
		writeError(w, http.StatusBadRequest, "That image is too large - Discord takes up to 10 MB")
		return
	}
	p, err := s.store.GetEmaPlaque(r.Context(), id)
	if err != nil {
		writeInternalError(w, "get ema request", err)
		return
	}
	if p == nil || p.Kind != model.EmaKindArtist {
		writeError(w, http.StatusNotFound, "Request not found")
		return
	}
	if p.Status != model.EmaStatusRequested {
		writeError(w, http.StatusConflict, "This request is already finished - reopen it to post a plaque for it")
		return
	}
	board, err := s.store.GetEmaBoard(r.Context(), p.BoardID)
	if err != nil || board == nil {
		writeInternalError(w, "get ema board for a request", err)
		return
	}
	webhook := emaWindowWebhook(board, p.WindowID)
	if webhook == "" {
		writeError(w, http.StatusBadRequest, p.WindowLabel+" has no webhook to post to - ask staff to set one")
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Attach the finished plaque")
		return
	}
	defer func() { _ = file.Close() }() // read in full below; nothing to report on close
	data, err := io.ReadAll(io.LimitReader(file, maxEmaArtistImageBytes+1))
	if err != nil || len(data) > maxEmaArtistImageBytes {
		writeError(w, http.StatusBadRequest, "That image is too large - Discord takes up to 10 MB")
		return
	}
	// Sniffed from the bytes, not trusted from the browser's label.
	contentType := http.DetectContentType(data)
	name := emaArtistImageName(contentType)
	if name == "" {
		writeError(w, http.StatusBadRequest, "The plaque must be a PNG, JPEG, GIF or WebP image")
		return
	}

	// From the claim on, the steps belong together, as in an approval.
	ctx := context.WithoutCancel(r.Context())
	claimed, err := s.store.SetEmaPlaqueStatus(ctx, p.ID, model.EmaStatusRequested, model.EmaStatusPosting)
	if err != nil {
		writeInternalError(w, "claim ema request", err)
		return
	}
	if !claimed {
		writeError(w, http.StatusConflict, "This request is already being posted")
		return
	}

	who := model.ParticipantLabel(p.CharacterName, p.World)
	postErr := s.postEmaToDiscord(ctx, webhookTarget{Kind: "ema_request", Name: who}, webhook,
		"Requested by **"+who+"**, made by **"+board.ArtistName+"** - "+p.WindowLabel,
		discordFile{Name: name, ContentType: contentType, Data: data})
	if postErr != nil {
		if _, err := s.store.SetEmaPlaqueStatus(ctx, p.ID, model.EmaStatusPosting, model.EmaStatusRequested); err != nil {
			slog.Error("release ema request after a failed post", "plaque_id", p.ID, "error", err)
		}
		if errors.Is(postErr, errWebhookAmbiguous) {
			writeError(w, http.StatusBadGateway,
				"Discord didn't answer, so the plaque may or may not have posted. Check the channel before posting it again.")
			slog.Error("ema request post ambiguous", "plaque_id", p.ID, "error", postErr)
			return
		}
		writeUpstreamError(w, "post ema request", postErr)
		return
	}

	if _, err := s.store.FinishEmaPlaque(ctx, p.ID, model.EmaStatusPosting, model.EmaStatusComplete, s.actorName(r)); err != nil {
		// Posted, but not recorded as such: say so rather than invite a second post.
		slog.Error("record ema request as complete", "plaque_id", p.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "The plaque was posted, but recording it failed. Don't post it again.")
		return
	}
	slog.Info("ema request posted", "plaque_id", p.ID, "board_id", p.BoardID, "by", s.actorName(r))
	s.writeEmaPlaque(ctx, w, p.ID)
}

// -- Public: submitting a plaque ------------------------------------------------

// openEmaWindow resolves a board for a public submission: it must be open, belong
// to a published festival, offer `allow`, and be inside one of its windows now.
// Writes the refusal itself.
func (s *Server) openEmaWindow(w http.ResponseWriter, r *http.Request, allow func(*model.EmaBoard) bool) (*model.EmaBoard, *model.EmaWindow, bool) {
	id, ok := pathInt64(w, r, "id", "board")
	if !ok {
		return nil, nil, false
	}
	b, err := s.store.GetEmaBoard(r.Context(), id)
	if err != nil {
		writeInternalError(w, "get ema board", err)
		return nil, nil, false
	}
	if b == nil || b.Status != "open" || !allow(b) {
		writeError(w, http.StatusNotFound, "This plaque board isn't taking plaques")
		return nil, nil, false
	}
	m, err := s.store.GetFestivalMap(r.Context(), b.FestivalMapID)
	if err != nil {
		writeInternalError(w, "get festival for ema board", err)
		return nil, nil, false
	}
	if m == nil || m.Status != model.MapStatusPublished {
		writeError(w, http.StatusNotFound, "This plaque board isn't taking plaques")
		return nil, nil, false
	}
	win := currentEmaWindow(b.Windows, time.Now().UTC())
	if win == nil {
		writeError(w, http.StatusBadRequest, "Plaques aren't being taken right now - check the festival's days")
		return nil, nil, false
	}
	return b, win, true
}

// currentEmaWindow returns the window `now` falls inside (start inclusive, end
// exclusive), or nil. Windows never overlap (checked on save).
func currentEmaWindow(windows []model.EmaWindow, now time.Time) *model.EmaWindow {
	for i := range windows {
		start, err1 := time.Parse(time.RFC3339, windows[i].StartAt)
		end, err2 := time.Parse(time.RFC3339, windows[i].EndAt)
		if err1 == nil && err2 == nil && !now.Before(start) && now.Before(end) {
			return &windows[i]
		}
	}
	return nil
}

// emaVisitor validates the name and world a submission is made under.
func emaVisitor(name, world string) (string, string, string) {
	name, world = strings.TrimSpace(name), strings.TrimSpace(world)
	switch {
	case name == "":
		return "", "", "Enter your character name"
	case world == "":
		return "", "", "Pick your home world"
	case utf8.RuneCountInString(name) > maxParticipantNameLen || utf8.RuneCountInString(world) > maxParticipantNameLen:
		return "", "", "That name is too long"
	}
	return name, world, ""
}

// guardEmaSubmit applies the public submit paths' rate limit and bot check.
func (s *Server) guardEmaSubmit(w http.ResponseWriter, r *http.Request, turnstileToken string) bool {
	ip := clientIP(r)
	if s.turnstileEnabled() && !s.verifyTurnstile(r.Context(), turnstileToken, ip) {
		slog.Warn("turnstile verification failed (ema plaque)", "ip", ip)
		writeError(w, http.StatusForbidden, "Bot check failed. Please try again.")
		return false
	}
	return true
}

// emaLimited applies the shared submit budget, counting every attempt.
func (s *Server) emaLimited(w http.ResponseWriter, r *http.Request) bool {
	ip := clientIP(r)
	if s.emaLimiter.isLimited(ip) {
		slog.Warn("ema plaque submission rate limited", "ip", ip)
		writeError(w, http.StatusTooManyRequests, "Too many plaques sent. Please try again later.")
		return true
	}
	s.emaLimiter.recordFailure(ip)
	return false
}

// handleEmaPlaqueSubmit takes a plaque designed on the site and holds it for staff
// review.
//
//	Endpoint:  POST /api/ema/{id}/plaques
//	Auth:      public (rate limited, bot-checked)
//	Request:   multipart: image (PNG), name, world, turnstile_token
//	Response:  201 EmaPlaqueResponse (status "pending"); 409 already has one this window
func (s *Server) handleEmaPlaqueSubmit(w http.ResponseWriter, r *http.Request) {
	if s.emaLimited(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxEmaImageBytes+(1<<20))
	if err := r.ParseMultipartForm(maxEmaImageBytes + (1 << 20)); err != nil { //nolint:gosec // the body is capped by the MaxBytesReader just above
		writeError(w, http.StatusBadRequest, "That plaque is too large to send")
		return
	}
	if !s.guardEmaSubmit(w, r, r.FormValue("turnstile_token")) {
		return
	}
	name, world, msg := emaVisitor(r.FormValue("name"), r.FormValue("world"))
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	board, win, ok := s.openEmaWindow(w, r, func(b *model.EmaBoard) bool { return b.AllowSite })
	if !ok {
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Attach the plaque image")
		return
	}
	defer func() { _ = file.Close() }() // read in full below; nothing to report on close
	data, err := io.ReadAll(io.LimitReader(file, maxEmaImageBytes+1))
	if err != nil || len(data) > maxEmaImageBytes {
		writeError(w, http.StatusBadRequest, "That plaque is too large to send")
		return
	}
	// A PNG of a sensible size, checked by decoding its header - not by trusting
	// the content type the browser sent.
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		writeError(w, http.StatusBadRequest, "The plaque must be a PNG image")
		return
	}
	if cfg.Width > maxEmaImageSide || cfg.Height > maxEmaImageSide {
		writeError(w, http.StatusBadRequest, "That plaque image is too large")
		return
	}

	rel, err := s.saveEmaImage(board.ID, data)
	if err != nil {
		writeInternalError(w, "save ema plaque image", err)
		return
	}
	// Detached: the file is on disk now, and its row must follow or the file go.
	ctx := context.WithoutCancel(r.Context())
	winID := win.ID
	p, err := s.store.CreateEmaPlaque(ctx, &model.EmaPlaque{
		BoardID: board.ID, WindowID: &winID, WindowLabel: win.Label, Kind: model.EmaKindSite,
		CharacterName: name, World: world, Image: rel, Status: model.EmaStatusPending,
	})
	if err != nil {
		s.removeEmaImage(rel)
		if errors.Is(err, store.ErrEmaPlaqueTaken) {
			writeError(w, http.StatusConflict, "You already have a plaque for "+win.Label+" - one per day")
			return
		}
		writeInternalError(w, "create ema plaque", err)
		return
	}
	writeJSON(w, http.StatusCreated, model.EmaPlaqueResponse{Plaque: *p})
	slog.Info("ema plaque submitted", "plaque_id", p.ID, "board_id", board.ID, "window", win.Label)
	s.broadcastResourceChanged("ema-boards")
}

// emaRequestSubmit is the JSON body for POST /api/ema/{id}/requests.
type emaRequestSubmit struct {
	Name           string `json:"name"`
	World          string `json:"world"`
	Description    string `json:"description"`
	TurnstileToken string `json:"turnstile_token"`
}

// handleEmaRequestSubmit records a request for the artist to make a plaque.
//
//	Endpoint:  POST /api/ema/{id}/requests
//	Auth:      public (rate limited, bot-checked)
//	Response:  201 EmaPlaqueResponse (status "requested"); 409 already has one this
//	           window, or the window's artist requests are full
func (s *Server) handleEmaRequestSubmit(w http.ResponseWriter, r *http.Request) {
	if s.emaLimited(w, r) {
		return
	}
	req, err := readJSON[emaRequestSubmit](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if !s.guardEmaSubmit(w, r, req.TurnstileToken) {
		return
	}
	name, world, msg := emaVisitor(req.Name, req.World)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		writeError(w, http.StatusBadRequest, "Describe the plaque you'd like")
		return
	}
	if utf8.RuneCountInString(desc) > maxEmaDescriptionLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Keep the description under %d characters", maxEmaDescriptionLen))
		return
	}
	board, win, ok := s.openEmaWindow(w, r, func(b *model.EmaBoard) bool { return b.AllowArtist })
	if !ok {
		return
	}
	winID := win.ID
	p, err := s.store.CreateEmaPlaque(r.Context(), &model.EmaPlaque{
		BoardID: board.ID, WindowID: &winID, WindowLabel: win.Label, Kind: model.EmaKindArtist,
		CharacterName: name, World: world, Description: desc, Status: model.EmaStatusRequested,
	})
	if errors.Is(err, store.ErrEmaPlaqueTaken) {
		writeError(w, http.StatusConflict, "You already have a plaque for "+win.Label+" - one per day")
		return
	}
	if errors.Is(err, store.ErrEmaArtistFull) {
		writeError(w, http.StatusConflict, board.ArtistName+"'s requests for "+win.Label+" are full")
		return
	}
	if err != nil {
		writeInternalError(w, "create ema request", err)
		return
	}
	writeJSON(w, http.StatusCreated, model.EmaPlaqueResponse{Plaque: *p})
	s.broadcastResourceChanged("ema-requests")
}

// -- Files ----------------------------------------------------------------------

// saveEmaImage writes a plaque's PNG under emaImageRelDir with an unguessable
// name, returning its web-root-relative path.
func (s *Server) saveEmaImage(boardID int64, data []byte) (string, error) {
	token, err := randHex(16)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(s.webRoot, filepath.FromSlash(emaImageRelDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%d-%s.png", boardID, token)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return "", err
	}
	return path.Join(emaImageRelDir, name), nil
}

// randHex returns n random bytes as hex, for an unguessable file name.
func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// emaImagePath resolves a stored plaque image to its file, or "" when the value
// isn't one of ours (so nothing outside the plaque folder can be read or removed).
func (s *Server) emaImagePath(rel string) string {
	base := path.Base(rel)
	if rel != path.Join(emaImageRelDir, base) || base == "." || strings.HasPrefix(base, ".") {
		return ""
	}
	return filepath.Join(s.webRoot, filepath.FromSlash(emaImageRelDir), base)
}

func (s *Server) readEmaImage(rel string) ([]byte, error) {
	p := s.emaImagePath(rel)
	if p == "" {
		return nil, fmt.Errorf("not a plaque image: %q", rel)
	}
	return os.ReadFile(p) //nolint:gosec // emaImagePath only resolves inside the plaque folder
}

// removeEmaImage deletes a plaque's file. Best effort: a file already gone is fine,
// and any other failure is logged - the row is what records the plaque.
func (s *Server) removeEmaImage(rel string) {
	p := s.emaImagePath(rel)
	if p == "" {
		return
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("remove ema plaque image", "image", rel, "error", err)
	}
}
