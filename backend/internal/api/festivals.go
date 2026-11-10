package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/db"
	"app-suite/internal/festival"
	"app-suite/internal/raffles"
)

// The festival dashboard: one page where a participant follows everything filed under
// a festival (the plan, their stamp card, their garapon draws, their raffle entries
// and the plaque board) and acts on each. It is only the read that depends on who is
// looking; the plan is the public map, and every action goes through the operation
// that already owns it.

var festivalTags = []string{"Festivals"}

// FestivalDashboardInput is whatever the participant's own device holds. Every field is
// optional.
type FestivalDashboardInput struct {
	CardToken     string `json:"cardToken,omitempty" maxLength:"100" doc:"The token of their stamp card. One that belongs to another festival, or to nothing, is ignored."`
	GaraponToken  string `json:"garaponToken,omitempty" maxLength:"100" doc:"The token of their drawing link. One whose garapon doesn't serve this festival's rally is ignored."`
	CharacterName string `json:"characterName,omitempty" maxLength:"60" doc:"Their character, to find their raffle entries and plaques when no card names them."`
	World         string `json:"world,omitempty" maxLength:"60" doc:"Their home world."`
}

// FestivalDashboardRally is the festival's stamp rally as a visitor sees it.
type FestivalDashboardRally struct {
	ID         int64  `json:"id" doc:"The rally's id."`
	Title      string `json:"title" doc:"The rally's title."`
	CardImage  string `json:"cardImage" doc:"The card's background picture."`
	Details    string `json:"details" doc:"What it is about (Markdown)."`
	SignupOpen bool   `json:"signupOpen" doc:"Whether anyone may sign up for a card now."`
}

// FestivalDashboardGarapon is the festival's garapon as a visitor sees it.
type FestivalDashboardGarapon struct {
	Garapon         PublicGarapon         `json:"garapon" doc:"The garapon."`
	DefaultDraws    int64                 `json:"defaultDraws" doc:"The draws a new drawing link carries."`
	Player          *GaraponPlayerSummary `json:"player" doc:"The visitor's own drawing link; null when they hold none."`
	Draws           []PublicGaraponDraw   `json:"draws" nullable:"false" doc:"The draws made with their link, oldest first."`
	HasPairedLink   bool                  `json:"hasPairedLink" doc:"Whether a drawing link was issued with their card, which they don't hold."`
	PairedDrawsLeft int64                 `json:"pairedDrawsLeft" doc:"The draws left on that link; never the link itself."`
}

// FestivalRaffle is a raffle filed under the festival, with the visitor's standing in it.
type FestivalRaffle struct {
	Raffle       Raffle             `json:"raffle" doc:"The raffle."`
	TotalEntries int64              `json:"totalEntries" doc:"The tickets entered in all."`
	Running      bool               `json:"running" doc:"Whether it can be entered now."`
	Winner       *RaffleWinner      `json:"winner" doc:"The winner once the raffle is closed; null before."`
	IsWinner     bool               `json:"isWinner" doc:"Whether the visitor is the winner."`
	Entry        *RaffleLookupEntry `json:"entry" doc:"The visitor's own entry, on their whole name and world; null when they have none."`
}

// FestivalDashboard is one participant's standing at a festival.
type FestivalDashboard struct {
	ParticipantName string                    `json:"participantName" doc:"Who the visitor is: the card's participant, else the drawing link's, else the name they sent."`
	World           string                    `json:"world" doc:"Their home world."`
	Rally           *FestivalDashboardRally   `json:"rally" doc:"The festival's stamp rally; null when it has none that isn't closed."`
	Card            *PublicStampCard          `json:"card" doc:"Their stamp card; null when they hold none for this festival."`
	CardToken       *string                   `json:"cardToken" nullable:"true" doc:"The card's token, which the visitor may not have sent when only the drawing link was."`
	Garapon         *FestivalDashboardGarapon `json:"garapon" doc:"The garapon serving the rally; null when there is none."`
	Raffles         []FestivalRaffle          `json:"raffles" nullable:"false" doc:"The festival's raffles a visitor may see, newest first."`
	Ema             *FestivalEma              `json:"ema" doc:"The festival's open plaque board; null when it has none."`
}

type festivalDashboardInput struct {
	IDOrSlug string `path:"idOrSlug" maxLength:"100" doc:"The festival's shortcode or numeric id."`
	Body     FestivalDashboardInput
}

type festivalDashboardOutput struct{ Body FestivalDashboard }

func (h handlers) registerFestivals(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "read-festival-dashboard", Method: http.MethodPost, Path: "/festivals/{idOrSlug}/dashboard",
		Summary: "A participant's standing at a festival",
		Description: "A POST so neither the tokens nor the name reach a URL or the access log. A token that belongs to a " +
			"different festival, or to nothing, is ignored: a phone that has been to two festivals holds both. " +
			"A card's completion that has just come due is recorded. 120 reads from one address in ten minutes " +
			"(429, with Retry-After).",
		Tags:   festivalTags,
		Errors: []int{http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusTooManyRequests},
	}, h.readFestivalDashboard)
}

// publicGarapon is a garapon as a player sees it: no odds, no link to the rally.
func publicGarapon(g Garapon) PublicGarapon {
	pub := PublicGarapon{
		ID: g.ID, Title: g.Title, Details: g.Details, GrandPrizeImage: g.GrandPrizeImage, Status: g.Status,
		Prizes: make([]PublicGaraponPrize, 0, len(g.Prizes)),
	}
	for _, p := range g.Prizes {
		pub.Prizes = append(pub.Prizes, PublicGaraponPrize{ID: p.ID, Name: p.Name, BallColor: p.BallColor, IsGrand: p.IsGrand})
	}
	return pub
}

func (h handlers) readFestivalDashboard(ctx context.Context, in *festivalDashboardInput) (*festivalDashboardOutput, error) {
	// Generous, because the page reads this on every visit and after each entry; it is
	// here so the whole-name raffle match can't be walked at speed.
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.DashboardLimit, ip); wait > 0 {
		h.Logger.Warn("festival dashboard held back", "ip", ip)
		return nil, tooMany(wait, "Too many requests. Please try again in a few minutes.")
	}
	if h.DashboardLimit != nil {
		h.DashboardLimit.Fail(ip)
	}

	q := h.read()
	m, err := h.publishedMap(ctx, q, in.IDOrSlug)
	if err != nil {
		return nil, err
	}
	name, world := strings.TrimSpace(in.Body.CharacterName), strings.TrimSpace(in.Body.World)
	now := time.Now().UTC()
	out := FestivalDashboard{Raffles: []FestivalRaffle{}}

	// The drawing link first: it is the stronger secret, and it carries its paired card,
	// so a device holding only the drawing link still gets the card.
	player, garapon, err := h.dashboardPlayer(ctx, q, strings.TrimSpace(in.Body.GaraponToken))
	if err != nil {
		return nil, err
	}
	cardToken := strings.TrimSpace(in.Body.CardToken)
	if cardToken == "" && player != nil {
		cardToken = player.StampCardToken
	}

	view, err := h.dashboardCard(ctx, q, cardToken, m.ID)
	if err != nil {
		return nil, err
	}
	var rally *StampRally
	if view != nil {
		rally = &view.rally
	} else if rally, err = h.dashboardOpenRally(ctx, q, m.ID); err != nil {
		return nil, err
	}

	if rally != nil {
		out.Rally = &FestivalDashboardRally{
			ID: rally.ID, Title: rally.Title, CardImage: rally.CardImage, Details: rally.Details,
			SignupOpen: rally.PublicSignup && festival.RallyOpen(rally.rule(), now),
		}
		if view != nil {
			changed, err := completeCard(context.WithoutCancel(ctx), h.write(), view, now)
			switch {
			case err != nil:
				// The collected stamps are already kept and the card completes on the next view.
				h.Logger.Error("mark a stamp card complete", "cardId", view.card.ID, "rallyId", rally.ID, "error", err)
			case changed:
				ResourceChanged(h.Live, "stamp-rallies")
			}
			card := h.publicCard(ctx, *view, now)
			out.Card = &card
			token := view.card.Token
			out.CardToken = &token
			out.ParticipantName, out.World = view.card.ParticipantName, view.card.World
		}

		// A drawing link only counts when its drum serves THIS rally, and, when a card came
		// too, when it is that card's own link. A card and a drawing link from two people
		// would otherwise show one person's card beside another's draws. The card wins:
		// it is what names the participant here.
		if garapon != nil && (garapon.StampRallyID == nil || *garapon.StampRallyID != rally.ID) {
			player, garapon = nil, nil
		}
		if player != nil && view != nil && player.StampCardToken != "" && player.StampCardToken != view.card.Token {
			player, garapon = nil, nil
		}
		if garapon == nil {
			if garapon, err = h.dashboardOpenGarapon(ctx, q, rally.ID); err != nil {
				return nil, err
			}
		}
		if garapon != nil {
			dg, err := h.dashboardGarapon(ctx, q, *garapon, player, view)
			if err != nil {
				return nil, err
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

	if out.Raffles, err = h.dashboardRaffles(ctx, q, m.ID, out.ParticipantName, out.World, now); err != nil {
		return nil, err
	}
	if out.Ema, err = h.buildFestivalEma(ctx, q, m.ID, out.ParticipantName, out.World, now); err != nil {
		return nil, h.fail("read a festival's plaque board", err)
	}
	return &festivalDashboardOutput{Body: out}, nil
}

// dashboardPlayer resolves a drawing token to its link and drum. An empty or unknown
// token is neither, not an error.
func (h handlers) dashboardPlayer(ctx context.Context, q *db.Queries, token string) (*db.GetGaraponPlayerByTokenRow, *Garapon, error) {
	if token == "" {
		return nil, nil, nil
	}
	p, err := q.GetGaraponPlayerByToken(ctx, token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, h.fail("read a drawing link", err)
	}
	g, err := h.loadGarapon(ctx, q, p.GaraponID)
	if err != nil {
		return nil, nil, err
	}
	return p, &g, nil
}

// dashboardCard resolves a card token to its card and rally, keeping them only when the
// rally is filed under this festival. The rally may since have closed: a finished card,
// with its prizes and where to redeem them, is still worth showing for as long as the
// festival is.
func (h handlers) dashboardCard(ctx context.Context, q *db.Queries, token string, mapID int64) (*cardView, error) {
	if token == "" {
		return nil, nil
	}
	card, err := q.GetRallyCardByToken(ctx, token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, h.fail("read a stamp card", err)
	}
	rally, err := h.loadRally(ctx, q, card.RallyID)
	if err != nil {
		return nil, err
	}
	if rally.FestivalMapID == nil || *rally.FestivalMapID != mapID {
		return nil, nil
	}
	collected, err := collectedStamps(ctx, q, card.ID)
	if err != nil {
		return nil, h.fail("read a card's stamps", err)
	}
	return &cardView{card: card, rally: rally, collected: collected}, nil
}

// dashboardOpenRally returns the festival's rally that isn't closed, with its stamps
// and prizes. Unlike the public map, which badges stalls only while the rally is
// inside its window, the dashboard keeps one that hasn't opened yet: its card says
// when it starts, and that beats saying nothing.
func (h handlers) dashboardOpenRally(ctx context.Context, q *db.Queries, mapID int64) (*StampRally, error) {
	row, err := q.GetOpenRallyForMap(ctx, sql.NullInt64{Int64: mapID, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, h.fail("read a festival's rally", err)
	}
	r, err := h.loadRally(ctx, q, row.ID)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// dashboardOpenGarapon returns the open drum linked to a rally, with its prizes.
func (h handlers) dashboardOpenGarapon(ctx context.Context, q *db.Queries, rallyID int64) (*Garapon, error) {
	row, err := q.OpenGaraponForRally(ctx, sql.NullInt64{Int64: rallyID, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, h.fail("read a rally's garapon", err)
	}
	g, err := h.loadGarapon(ctx, q, row.ID)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// dashboardGarapon builds the drum's panel. With the drawing link it carries the
// participant's allowance and history; without it, the draws waiting on the link
// paired with their card: the count, never the token.
func (h handlers) dashboardGarapon(ctx context.Context, q *db.Queries, g Garapon, player *db.GetGaraponPlayerByTokenRow, view *cardView) (*FestivalDashboardGarapon, error) {
	out := &FestivalDashboardGarapon{
		Garapon: publicGarapon(g), DefaultDraws: festival.DefaultDraws(g.DefaultDraws), Draws: []PublicGaraponDraw{},
	}
	if player != nil {
		draws, err := q.ListPlayerGaraponDraws(ctx, sql.NullInt64{Int64: player.ID, Valid: true})
		if err != nil {
			return nil, h.fail("list a player's draws", err)
		}
		grand := grandPrizes(g)
		for _, d := range draws {
			out.Draws = append(out.Draws, publicGaraponDraw(d, grand))
		}
		out.Player = &GaraponPlayerSummary{
			PlayerName: player.PlayerName, World: player.World, MaxDraws: player.MaxDraws, DrawsUsed: player.DrawsUsed,
			StampCardToken: player.StampCardToken,
		}
		return out, nil
	}
	if view != nil {
		paired, err := q.GetGaraponPlayerByCard(ctx, sql.NullInt64{Int64: view.card.ID, Valid: true})
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return nil, h.fail("read a card's drawing link", err)
		case paired.GaraponID == g.ID:
			out.HasPairedLink = true
			out.PairedDrawsLeft = max(0, paired.MaxDraws-paired.DrawsUsed)
		}
	}
	return out, nil
}

// dashboardRaffles lists the festival's raffles a visitor may see, by the rule of the
// public raffle pages, each with the participant's own entry when a name is known.
func (h handlers) dashboardRaffles(ctx context.Context, q *db.Queries, mapID int64, name, world string, now time.Time) ([]FestivalRaffle, error) {
	rows, err := q.ListFestivalRaffles(ctx, sql.NullInt64{Int64: mapID, Valid: true})
	if err != nil {
		return nil, h.fail("list a festival's raffles", err)
	}
	tallies, err := q.ListFestivalRaffleTallies(ctx, sql.NullInt64{Int64: mapID, Valid: true})
	if err != nil {
		return nil, h.fail("tally a festival's raffles", err)
	}
	byRaffle := make(map[int64]*db.ListFestivalRaffleTalliesRow, len(tallies))
	for _, t := range tallies {
		byRaffle[t.ID] = t
	}
	mine := map[int64]RaffleLookupEntry{}
	if name != "" {
		entries, err := q.ListFestivalRaffleEntriesFor(ctx, db.ListFestivalRaffleEntriesForParams{
			MapID: sql.NullInt64{Int64: mapID, Valid: true}, Name: name, World: world,
		})
		if err != nil {
			return nil, h.fail("read a visitor's raffle entries", err)
		}
		for _, e := range entries {
			mine[e.RaffleID] = RaffleLookupEntry{
				CharacterName: e.CharacterName, World: e.World, NumEntries: e.NumEntries, PaidEntries: e.PaidEntries,
				PaymentState: raffles.PaymentState(int(e.NumEntries), int(e.PaidEntries)),
			}
		}
	}
	out := make([]FestivalRaffle, 0, len(rows))
	for _, row := range rows {
		r := toRaffle((*db.GetRaffleRow)(row))
		if !r.viewable(now) {
			continue
		}
		fr := FestivalRaffle{
			Raffle: r, Running: raffles.Running(r.Status, deref(r.AvailableFrom), deref(r.AvailableTo), now),
		}
		if t := byRaffle[r.ID]; t != nil {
			fr.TotalEntries = t.TotalEntries
			if r.Status == raffles.StatusClosed && t.WinnerName != "" {
				fr.Winner = &RaffleWinner{CharacterName: t.WinnerName, World: t.WinnerWorld, NumEntries: t.WinnerTickets}
				fr.IsWinner = name != "" && strings.EqualFold(t.WinnerName, name) && strings.EqualFold(t.WinnerWorld, world)
			}
		}
		if e, found := mine[r.ID]; found {
			fr.Entry = &e
		}
		out = append(out, fr)
	}
	return out, nil
}
