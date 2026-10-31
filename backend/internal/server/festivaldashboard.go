package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// The festival dashboard: one page where a participant follows everything filed
// under a festival - the plan, their stamp card, their garapon draws and their
// raffle entries - and acts on each of them.
//
// This endpoint is only the READ side, and only the part that depends on who is
// looking; the plan comes from GET /api/festival-maps/public/{id}, and every
// action (stamping, drawing, entering, signing up) goes through the endpoint that
// already owns it. The path sits outside the plural "festival-maps" prefix
// because adminMutationResource treats every POST under that prefix as an admin
// edit, and this read would broadcast one on every page load.

// festivalDashboardRequest is the JSON body for POST /api/festival-dashboard/{id}:
// whatever the participant's own device holds. Every field is optional.
type festivalDashboardRequest struct {
	CardToken    string `json:"card_token"`
	GaraponToken string `json:"garapon_token"`
	Name         string `json:"name"`
	World        string `json:"world"`
}

// handleFestivalDashboard returns one participant's standing at a festival.
//
//	Endpoint:  POST /api/festival-dashboard/{id}
//	Auth:      public (the tokens are the capability; the name only reads raffle entries)
//	Request:   {"card_token", "garapon_token", "name", "world"} - all optional
//	Response:  200 FestivalDashboard
//
// POST rather than GET so neither the tokens nor the name reach a URL, a query
// string or the access log. A token that belongs to a different festival - or to
// nothing - is ignored rather than refused: a phone that has been to two
// festivals holds both festivals' links, and either way the answer is "not here".
func (s *Server) handleFestivalDashboard(w http.ResponseWriter, r *http.Request) {
	// Generous, because the page reads this on every visit and after each entry;
	// it is here so the exact-name raffle match can't be walked at speed.
	ip := clientIP(r)
	if s.dashboardLimiter.isLimited(ip) {
		slog.Warn("festival dashboard rate limited", "ip", ip)
		writeError(w, http.StatusTooManyRequests, "Too many requests. Please try again in a few minutes.")
		return
	}
	s.dashboardLimiter.recordFailure(ip)

	m, err := s.resolvePublicMap(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeInternalError(w, "get festival for dashboard", err)
		return
	}
	if m == nil || m.Status != model.MapStatusPublished {
		writeError(w, http.StatusNotFound, "Festival not found")
		return
	}
	req, err := readJSON[festivalDashboardRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name, world := strings.TrimSpace(req.Name), strings.TrimSpace(req.World)
	if utf8.RuneCountInString(name) > maxParticipantNameLen || utf8.RuneCountInString(world) > maxParticipantNameLen {
		writeError(w, http.StatusBadRequest, "That name is too long")
		return
	}

	now := time.Now().UTC()
	out := model.FestivalDashboard{Raffles: []model.FestivalDashboardRaffle{}}

	// The drawing link first: it is the stronger secret, and it carries its paired
	// card, so a device holding only the drawing link still gets the card.
	player, garapon, err := s.dashboardGaraponPlayer(r.Context(), strings.TrimSpace(req.GaraponToken))
	if err != nil {
		writeInternalError(w, "load garapon for dashboard", err)
		return
	}
	cardToken := strings.TrimSpace(req.CardToken)
	if cardToken == "" && player != nil {
		cardToken = player.StampCardToken
	}

	card, rally, err := s.dashboardCard(r.Context(), cardToken, m.ID)
	if err != nil {
		writeInternalError(w, "load stamp card for dashboard", err)
		return
	}
	if rally == nil {
		if rally, err = s.dashboardOpenRally(r.Context(), m.ID); err != nil {
			writeInternalError(w, "load rally for dashboard", err)
			return
		}
	}

	if rally != nil {
		out.Rally = &model.FestivalDashboardRally{
			ID: rally.ID, Title: rally.Title, CardImage: rally.CardImage, Details: rally.Details,
			SignupOpen: rally.PublicSignup && rallyOpen(rally, now),
		}
		if card != nil {
			collected, err := s.store.ListCollectedStampIDs(r.Context(), card.ID)
			if err != nil {
				writeInternalError(w, "list collected stamps for dashboard", err)
				return
			}
			s.maybeComplete(r.Context(), card, rally, collected, now)
			pc := buildPublicCard(rally, card, rally.Stamps, rally.Prizes, collected, now)
			out.Card = &pc
			out.CardToken = card.Token
			out.ParticipantName, out.World = card.ParticipantName, card.World
		}

		// A drawing link only counts when its drum serves THIS rally, and - when a
		// card came too - when it is that card's own link. A card and a drawing link
		// from two different people would otherwise show one person's card beside
		// another's draws. The card wins: it is what names the participant here.
		if garapon != nil && (garapon.StampRallyID == nil || *garapon.StampRallyID != rally.ID) {
			player, garapon = nil, nil
		}
		if player != nil && card != nil && player.StampCardToken != "" && player.StampCardToken != card.Token {
			player, garapon = nil, nil
		}
		if garapon == nil {
			if garapon, err = s.dashboardOpenGarapon(r.Context(), rally.ID); err != nil {
				writeInternalError(w, "load garapon for dashboard", err)
				return
			}
		}
		if garapon != nil {
			dg, err := s.dashboardGarapon(r.Context(), garapon, player, card)
			if err != nil {
				writeInternalError(w, "build garapon for dashboard", err)
				return
			}
			out.Garapon = dg
			if out.ParticipantName == "" && player != nil {
				out.ParticipantName, out.World = player.PlayerName, player.World
			}
		}
	}
	if out.ParticipantName == "" {
		out.ParticipantName, out.World = name, world
	}

	raffles, err := s.dashboardRaffles(r.Context(), m.ID, out.ParticipantName, out.World, now)
	if err != nil {
		writeInternalError(w, "load raffles for dashboard", err)
		return
	}
	out.Raffles = raffles

	if out.Ema, err = s.dashboardEma(r.Context(), m.ID, out.ParticipantName, out.World, now); err != nil {
		writeInternalError(w, "load ema board for dashboard", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// dashboardEma builds the festival's open plaque board for a visitor: what it
// offers, its windows (never their webhooks), the one taking plaques now, and the
// visitor's own plaques when a name is known. nil when the festival has no open
// board.
func (s *Server) dashboardEma(ctx context.Context, mapID int64, name, world string, now time.Time) (*model.FestivalDashboardEma, error) {
	b, err := s.store.GetEmaBoardForMap(ctx, mapID)
	if err != nil || b == nil || b.Status != "open" {
		return nil, err
	}
	out := &model.FestivalDashboardEma{
		ID: b.ID, Title: b.Title, TemplateImage: b.TemplateImage, Instructions: b.Instructions,
		DiscordLink: b.DiscordLink, AllowSelf: b.AllowSelf, AllowSite: b.AllowSite,
		AllowArtist: b.AllowArtist, ArtistName: b.ArtistName,
		Windows: make([]model.PublicEmaWindow, 0, len(b.Windows)),
		Plaques: []model.PublicEmaPlaque{},
	}
	current := currentEmaWindow(b.Windows, now)
	if current != nil {
		id := current.ID
		out.CurrentWindowID = &id
	}
	for _, w := range b.Windows {
		pw := model.PublicEmaWindow{
			ID: w.ID, Label: w.Label, StartAt: w.StartAt, EndAt: w.EndAt,
			IsOpen: current != nil && current.ID == w.ID, ArtistLimit: w.ArtistLimit,
		}
		if w.ArtistLimit > 0 {
			pw.ArtistLeft = max(0, w.ArtistLimit-w.ArtistCount)
		}
		out.Windows = append(out.Windows, pw)
	}
	if name == "" {
		return out, nil
	}
	mine, err := s.store.ListEmaPlaquesFor(ctx, b.ID, name, world)
	if err != nil {
		return nil, err
	}
	for i := range mine {
		p := &mine[i]
		pp := model.PublicEmaPlaque{
			ID: p.ID, WindowID: p.WindowID, WindowLabel: p.WindowLabel, Kind: p.Kind,
			Status: p.Status, Description: p.Description, CreatedAt: p.CreatedAt,
		}
		// The visitor's own upload, shown back to them while it waits; the file is
		// gone once it is posted or rejected.
		if p.Status == model.EmaStatusPending || p.Status == model.EmaStatusPosting {
			pp.Image = p.Image
		}
		out.Plaques = append(out.Plaques, pp)
		if current != nil && p.WindowID != nil && *p.WindowID == current.ID && p.Status != model.EmaStatusRejected {
			out.HasPlaqueThisWindow = true
		}
	}
	return out, nil
}

// dashboardGaraponPlayer resolves a drawing token to its link and drum. An empty
// or unknown token is (nil, nil, nil), not an error.
func (s *Server) dashboardGaraponPlayer(ctx context.Context, token string) (*model.GaraponPlayer, *model.Garapon, error) {
	if token == "" {
		return nil, nil, nil
	}
	player, err := s.store.GetGaraponPlayerByToken(ctx, token)
	if err != nil || player == nil {
		return nil, nil, err
	}
	garapon, err := s.store.GetGarapon(ctx, player.GaraponID)
	if err != nil || garapon == nil {
		return nil, nil, err
	}
	return player, garapon, nil
}

// dashboardCard resolves a card token to its card and rally, keeping them only
// when the rally is filed under this festival. The rally may since have closed:
// a finished card, with its prizes and where to redeem them, is still worth
// showing for as long as the festival is.
func (s *Server) dashboardCard(ctx context.Context, token string, mapID int64) (*model.StampRallyCard, *model.StampRally, error) {
	if token == "" {
		return nil, nil, nil
	}
	card, err := s.store.GetRallyCardByToken(ctx, token)
	if err != nil || card == nil {
		return nil, nil, err
	}
	rally, err := s.store.GetStampRally(ctx, card.RallyID)
	if err != nil || rally == nil {
		return nil, nil, err
	}
	if rally.FestivalMapID == nil || *rally.FestivalMapID != mapID {
		return nil, nil, nil
	}
	return card, rally, nil
}

// dashboardOpenRally returns the festival's not-closed rally (with its stamps and
// prizes), or nil. Unlike the public map, which badges stalls only while the rally
// is inside its window, the dashboard keeps one that hasn't opened yet: its card
// says when it starts, and that beats saying nothing.
func (s *Server) dashboardOpenRally(ctx context.Context, mapID int64) (*model.StampRally, error) {
	row, err := s.store.GetOpenRallyForMap(ctx, mapID)
	if err != nil || row == nil {
		return nil, err
	}
	return s.store.GetStampRally(ctx, row.ID)
}

// dashboardOpenGarapon returns the open drum linked to a rally, with its prizes.
func (s *Server) dashboardOpenGarapon(ctx context.Context, rallyID int64) (*model.Garapon, error) {
	row, err := s.store.OpenGaraponForRally(ctx, rallyID)
	if err != nil || row == nil {
		return nil, err
	}
	return s.store.GetGarapon(ctx, row.ID)
}

// dashboardGarapon builds the drum's panel. With the drawing link (player) it
// carries the participant's allowance and history; without it, the draws waiting
// on the link paired with their card - the count, never the token.
func (s *Server) dashboardGarapon(ctx context.Context, g *model.Garapon, player *model.GaraponPlayer,
	card *model.StampRallyCard) (*model.FestivalDashboardGarapon, error) {
	out := &model.FestivalDashboardGarapon{
		Garapon:      toPublicGarapon(g),
		DefaultDraws: clampDefaultDraws(g.DefaultDraws),
		Draws:        []model.GaraponDraw{},
	}
	if player != nil {
		draws, err := s.store.ListPlayerDraws(ctx, player.ID)
		if err != nil {
			return nil, err
		}
		out.Player = &model.GaraponPublicPlayer{
			PlayerName: player.PlayerName, MaxDraws: player.MaxDraws,
			DrawsUsed: player.DrawsUsed, StampCardToken: player.StampCardToken,
		}
		out.Draws = draws
		return out, nil
	}
	if card != nil {
		paired, err := s.store.GetGaraponPlayerByCard(ctx, card.ID)
		if err != nil {
			return nil, err
		}
		if paired != nil && paired.GaraponID == g.ID {
			out.HasPairedLink = true
			out.PairedDrawsLeft = max(0, paired.MaxDraws-paired.DrawsUsed)
		}
	}
	return out, nil
}

// dashboardRaffles lists the festival's raffles a visitor may see - the same rule
// as GET /api/raffles/{id} - each with the participant's own entry when a name is
// known.
func (s *Server) dashboardRaffles(ctx context.Context, mapID int64, name, world string, now time.Time) ([]model.FestivalDashboardRaffle, error) {
	rows, err := s.store.ListFestivalRaffles(ctx, mapID)
	if err != nil {
		return nil, err
	}
	var mine map[int64]model.RaffleLookupEntry
	if name != "" {
		if mine, err = s.store.FestivalRaffleEntriesFor(ctx, mapID, name, world); err != nil {
			return nil, err
		}
	}
	out := make([]model.FestivalDashboardRaffle, 0, len(rows))
	for i := range rows {
		fr := &rows[i]
		if !raffleIsPubliclyViewable(&fr.Raffle) {
			continue
		}
		dr := model.FestivalDashboardRaffle{
			Raffle:       fr.Raffle,
			TotalEntries: fr.TotalEntries,
			Running:      raffleRunning(stallRaffleOf(&fr.Raffle), now),
		}
		if e, found := mine[fr.ID]; found {
			dr.Entry = &e
		}
		if fr.Status == "closed" && fr.WinnerName != "" {
			dr.WinnerName = model.ParticipantLabel(fr.WinnerName, fr.WinnerWorld)
			dr.IsWinner = name != "" && strings.EqualFold(fr.WinnerName, name) && strings.EqualFold(fr.WinnerWorld, world)
		}
		out = append(out, dr)
	}
	return out, nil
}

// stallRaffleOf adapts a raffle to the shape raffleRunning reads, so the map's
// stall panel and the dashboard agree on what "running" means.
func stallRaffleOf(r *model.Raffle) store.StallRaffle {
	return store.StallRaffle{
		ID: r.ID, Title: r.Title, PrizeImage: r.PrizeImage, Status: r.Status,
		AvailableFrom: r.AvailableFrom, AvailableTo: r.AvailableTo,
	}
}

// festivalLinkForRally is FestivalLinkForRally with a failure logged rather than
// returned: the link is a convenience on the card and drawing pages, and must never
// cost a participant the page itself.
func (s *Server) festivalLinkForRally(ctx context.Context, rallyID int64) string {
	link, err := s.store.FestivalLinkForRally(ctx, rallyID)
	if err != nil {
		slog.Log(ctx, errorLevel(err), "look up festival link for rally", "rally_id", rallyID, "error", err)
		return ""
	}
	return link
}
