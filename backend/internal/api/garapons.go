package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/db"
	"app-suite/internal/festival"
	"app-suite/internal/model"
	"app-suite/internal/permissions"
	"app-suite/internal/random"
)

// Garapons: festival lottery drums. Staff make a drum with its prize tiers and issue
// each player a drawing link, a private token with a number of draws. The player
// opens the link and draws; the server picks every prize.

var garaponTags = []string{"Garapons"}

// Garapon is a garapon as the staff see it.
type Garapon struct {
	ID              int64          `json:"id" doc:"The garapon's id."`
	Title           string         `json:"title" doc:"The garapon's title."`
	Details         string         `json:"details" doc:"What it is about (Markdown)."`
	GrandPrizeImage string         `json:"grandPrizeImage" doc:"The grand prize's picture: a path from the image library; empty for none."`
	Status          string         `json:"status" enum:"open,closed" doc:"open takes draws; closed takes none and may have its players deleted."`
	StampRallyID    *int64         `json:"stampRallyId" nullable:"true" doc:"The stamp rally whose sign-ups it serves; null for none. A drawing link issued for the garapon also issues a card in that rally."`
	StampRallyTitle string         `json:"stampRallyTitle" doc:"The linked rally's title; empty for none."`
	DefaultDraws    int64          `json:"defaultDraws" doc:"The draws a drawing link carries when nobody picks a number."`
	CreatedAt       *string        `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
	Prizes          []GaraponPrize `json:"prizes" nullable:"false" doc:"Its prize tiers in order. Empty in a list."`
	PlayerCount     int64          `json:"playerCount" doc:"How many drawing links it has."`
	DrawCount       int64          `json:"drawCount" doc:"How many draws have been made across them."`
}

// GaraponPrize is one prize tier of a garapon.
type GaraponPrize struct {
	ID        int64   `json:"id" doc:"The prize's id."`
	Name      string  `json:"name" doc:"The prize."`
	BallColor string  `json:"ballColor" doc:"The color of the ball that wins it: a CSS color."`
	Rate      float64 `json:"rate" minimum:"0" doc:"How often it comes up, relative to the other prizes' rates; they need not total 100."`
	IsGrand   bool    `json:"isGrand" doc:"Whether it is the grand prize, the one the garapon's picture shows. Exactly one prize is."`
	SortOrder int64   `json:"sortOrder" doc:"Its place in the garapon's prize list."`
}

// GaraponInput is a garapon as a save sends it: all of its editable fields, since a
// save replaces them. Its status changes through close and reopen.
type GaraponInput struct {
	Title           string              `json:"title" minLength:"1" maxLength:"200" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The garapon's title."`
	Details         string              `json:"details" maxLength:"20000" doc:"What it is about (Markdown); empty for none."`
	GrandPrizeImage string              `json:"grandPrizeImage" maxLength:"500" doc:"The grand prize's picture: a path from the image library; empty for none."`
	StampRallyID    *int64              `json:"stampRallyId,omitempty" nullable:"true" minimum:"1" doc:"An open stamp rally to serve; null or left out for none. One that doesn't exist is refused, and so is a closed one."`
	DefaultDraws    int64               `json:"defaultDraws,omitempty" minimum:"1" doc:"The draws a drawing link carries when nobody picks a number; 1 when left out."`
	Prizes          []GaraponPrizeInput `json:"prizes" minItems:"1" maxItems:"50" nullable:"false" doc:"The prize tiers in order. A prize with no name is dropped; at least one must be left, and exactly one is grand (the first when none is marked)."`
}

// GaraponPrizeInput is a prize tier as a save sends it.
type GaraponPrizeInput struct {
	Name      string  `json:"name" maxLength:"200" doc:"The prize."`
	BallColor string  `json:"ballColor,omitempty" maxLength:"50" doc:"The color of the ball that wins it: a CSS color; empty or left out for gray."`
	Rate      float64 `json:"rate" minimum:"0" doc:"How often it comes up, relative to the other prizes' rates."`
	IsGrand   bool    `json:"isGrand,omitempty" doc:"Whether it is the grand prize."`
}

// GaraponList is every garapon.
type GaraponList struct {
	Garapons []Garapon `json:"garapons" nullable:"false" doc:"Newest first, each with its player and draw counts and without prizes."`
}

// GaraponDetail is a garapon with its drawing links and its draw log.
type GaraponDetail struct {
	Garapon Garapon         `json:"garapon" doc:"The garapon, with its prizes."`
	Players []GaraponPlayer `json:"players" nullable:"false" doc:"Its drawing links, oldest first."`
	Draws   []GaraponDraw   `json:"draws" nullable:"false" doc:"Every draw made, newest first."`
}

// GaraponPlayer is a drawing link issued to a player.
type GaraponPlayer struct {
	ID             int64   `json:"id" doc:"The link's id."`
	GaraponID      int64   `json:"garaponId" doc:"The garapon it draws from."`
	Token          string  `json:"token" doc:"The link's token: whoever holds it can spend the draws, so it is for staff to hand to the player."`
	PlayerName     string  `json:"playerName" doc:"The character."`
	World          string  `json:"world" doc:"Their home world; empty when it isn't known."`
	MaxDraws       int64   `json:"maxDraws" doc:"The draws the link carries."`
	DrawsUsed      int64   `json:"drawsUsed" doc:"The draws made with it."`
	StampCardToken string  `json:"stampCardToken" doc:"The token of the stamp rally card issued with the link, a separate secret that can be shared; empty when the garapon serves no rally."`
	CreatedAt      *string `json:"createdAt" nullable:"true" doc:"When it was issued, RFC 3339 in UTC."`
}

// GaraponPlayerInput is a drawing link staff issue.
type GaraponPlayerInput struct {
	PlayerName string `json:"playerName" minLength:"1" maxLength:"60" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The character. A name written as Name @ World with no world sent is split."`
	World      string `json:"world,omitempty" maxLength:"60" doc:"Their home world; empty when staff don't know it."`
	MaxDraws   int64  `json:"maxDraws,omitempty" doc:"The draws the link carries; below 1 or left out, the garapon's defaultDraws."`
}

// GaraponDraw is one draw in the log.
type GaraponDraw struct {
	ID         int64   `json:"id" doc:"The draw's id."`
	GaraponID  int64   `json:"garaponId" doc:"The garapon it was made on."`
	PlayerID   *int64  `json:"playerId" nullable:"true" doc:"The link that made it; null once the link is deleted."`
	PrizeID    *int64  `json:"prizeId" nullable:"true" doc:"The prize it won, which a later save of the garapon may have replaced."`
	PlayerName string  `json:"playerName" doc:"The player as Character @ World, as they were when it was made."`
	PrizeName  string  `json:"prizeName" doc:"The prize, as it was when it was won."`
	BallColor  string  `json:"ballColor" doc:"The color of the ball, as it was."`
	DrawnAt    *string `json:"drawnAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
}

// PublicGaraponPrize is a prize tier as a player sees it: no rate.
type PublicGaraponPrize struct {
	ID        int64  `json:"id" doc:"The prize's id."`
	Name      string `json:"name" doc:"The prize."`
	BallColor string `json:"ballColor" doc:"The color of the ball that wins it: a CSS color."`
	IsGrand   bool   `json:"isGrand" doc:"Whether it is the grand prize."`
}

// PublicGarapon is a garapon as a player sees it: no odds, no link to the rally.
type PublicGarapon struct {
	ID              int64                `json:"id" doc:"The garapon's id."`
	Title           string               `json:"title" doc:"The garapon's title."`
	Details         string               `json:"details" doc:"What it is about (Markdown)."`
	GrandPrizeImage string               `json:"grandPrizeImage" doc:"The grand prize's picture; empty for none."`
	Status          string               `json:"status" enum:"open,closed" doc:"A closed garapon takes no more draws."`
	Prizes          []PublicGaraponPrize `json:"prizes" nullable:"false" doc:"The prizes in order."`
}

// GaraponPlayerSummary is a drawing link as its holder sees it: the token is the one
// they hold.
type GaraponPlayerSummary struct {
	PlayerName     string `json:"playerName" doc:"The character."`
	World          string `json:"world" doc:"Their home world; empty when it isn't known."`
	MaxDraws       int64  `json:"maxDraws" doc:"The draws the link carries."`
	DrawsUsed      int64  `json:"drawsUsed" doc:"The draws made with it."`
	StampCardToken string `json:"stampCardToken" doc:"The token of the stamp card issued with the link, which opens that card; empty when there is none."`
}

// PublicGaraponDraw is a draw as its player sees it.
type PublicGaraponDraw struct {
	PrizeName string  `json:"prizeName" doc:"The prize won."`
	BallColor string  `json:"ballColor" doc:"The color of the ball."`
	IsGrand   bool    `json:"isGrand" doc:"Whether it was the grand prize."`
	DrawnAt   *string `json:"drawnAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
}

// PublicGaraponPlayer is what a drawing link opens.
type PublicGaraponPlayer struct {
	Garapon      PublicGarapon        `json:"garapon" doc:"The garapon."`
	Player       GaraponPlayerSummary `json:"player" doc:"The link's holder and their draws."`
	Draws        []PublicGaraponDraw  `json:"draws" nullable:"false" doc:"The draws made with the link, oldest first."`
	FestivalLink string               `json:"festivalLink" doc:"The shortcode or id of the published festival the linked rally is filed under; empty for none."`
}

// GaraponDrawResult is the draw just made.
type GaraponDrawResult struct {
	Draw      PublicGaraponDraw `json:"draw" doc:"What was won."`
	DrawsUsed int64             `json:"drawsUsed" doc:"The draws made with the link, this one included."`
	MaxDraws  int64             `json:"maxDraws" doc:"The draws the link carries."`
}

type garaponIDInput struct {
	ID int64 `path:"id" doc:"The garapon's id."`
}

type garaponSaveInput struct {
	ID   int64 `path:"id" doc:"The garapon's id."`
	Body GaraponInput
}

type garaponCreateInput struct{ Body GaraponInput }

type garaponPlayerInput struct {
	ID   int64 `path:"id" doc:"The garapon's id."`
	Body GaraponPlayerInput
}

type garaponPlayerIDInput struct {
	ID       int64 `path:"id" doc:"The garapon's id."`
	PlayerID int64 `path:"playerId" doc:"The drawing link's id."`
}

type tokenInput struct {
	Token string `path:"token" maxLength:"128" doc:"The link's token."`
}

type garaponOutput struct{ Body Garapon }

type garaponCreatedOutput struct {
	Status int
	Body   Garapon
}

type garaponListOutput struct{ Body GaraponList }

type garaponDetailOutput struct{ Body GaraponDetail }

type garaponPlayerCreatedOutput struct {
	Status int
	Body   GaraponPlayer
}

type publicGaraponOutput struct{ Body PublicGaraponPlayer }

type garaponDrawOutput struct{ Body GaraponDrawResult }

func (h handlers) registerGarapons(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.FestivalGarapon)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}
	notFound := slices.Concat(guarded, []int{http.StatusNotFound})

	huma.Register(api, huma.Operation{
		OperationID: "list-garapons", Method: http.MethodGet, Path: "/garapons",
		Summary: "Every garapon", Tags: garaponTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.listGarapons)
	huma.Register(api, huma.Operation{
		OperationID: "create-garapon", Method: http.MethodPost, Path: "/garapons",
		Summary: "Make a garapon", Description: "It starts open.", Tags: garaponTags,
		DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: slices.Concat(guarded, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.createGarapon)
	huma.Register(api, huma.Operation{
		OperationID: "get-garapon", Method: http.MethodGet, Path: "/garapons/{id}",
		Summary: "One garapon with its drawing links and draw log", Tags: garaponTags,
		Security: secured, Middlewares: page, Errors: notFound,
	}, h.getGarapon)
	huma.Register(api, huma.Operation{
		OperationID: "update-garapon", Method: http.MethodPut, Path: "/garapons/{id}",
		Summary: "Replace a garapon's editable fields",
		Description: "Its status is kept. Its prizes are replaced; the draws already made keep the names they were won " +
			"under.",
		Tags: garaponTags, Security: secured, Middlewares: page,
		Errors: slices.Concat(notFound, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.updateGarapon)
	huma.Register(api, huma.Operation{
		OperationID: "delete-garapon", Method: http.MethodDelete, Path: "/garapons/{id}",
		Summary: "Delete a garapon", Description: "Its prizes, drawing links and draws go with it. Its picture stays in the image library.",
		Tags: garaponTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page, Errors: notFound,
	}, h.deleteGarapon)
	huma.Register(api, huma.Operation{
		OperationID: "close-garapon", Method: http.MethodPost, Path: "/garapons/{id}/close",
		Summary: "Close a garapon", Description: "It takes no more draws.", Tags: garaponTags,
		Security: secured, Middlewares: page, Errors: notFound,
	}, h.closeGarapon)
	huma.Register(api, huma.Operation{
		OperationID: "reopen-garapon", Method: http.MethodPost, Path: "/garapons/{id}/reopen",
		Summary: "Open a closed garapon again", Tags: garaponTags, Security: secured, Middlewares: page, Errors: notFound,
	}, h.reopenGarapon)
	huma.Register(api, huma.Operation{
		OperationID: "create-garapon-player", Method: http.MethodPost, Path: "/garapons/{id}/players",
		Summary: "Issue a drawing link",
		Description: "Works on a closed garapon too. When the garapon serves an open stamp rally, the player is issued a " +
			"card in it as well, with a token of its own.",
		Tags: garaponTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: slices.Concat(notFound, []int{http.StatusUnprocessableEntity}),
	}, h.createGaraponPlayer)
	huma.Register(api, huma.Operation{
		OperationID: "delete-garapon-player", Method: http.MethodDelete, Path: "/garapons/{id}/players/{playerId}",
		Summary: "Delete a drawing link",
		Description: "While the garapon is open a link that has drawn can't be deleted (409); on a closed one any link can, " +
			"and its draws stay in the log. The stamp card issued with the link goes with it, unless it has collected " +
			"stamps in a rally still open.",
		Tags: garaponTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page,
		Errors: slices.Concat(notFound, []int{http.StatusConflict}),
	}, h.deleteGaraponPlayer)
	huma.Register(api, huma.Operation{
		OperationID: "get-garapon-player", Method: http.MethodGet, Path: "/garapon-players/{token}",
		Summary: "What a drawing link opens",
		Description: "Anyone holding the token: the garapon without its odds, the link's draws left, and its own draw " +
			"history. A closed garapon still shows.",
		Tags: garaponTags, Errors: []int{http.StatusNotFound},
	}, h.getGaraponPlayer)
	huma.Register(api, huma.Operation{
		OperationID: "draw-garapon", Method: http.MethodPost, Path: "/garapon-players/{token}/draw",
		Summary: "Draw once",
		Description: "The server checks that the garapon is open and the link has draws left, picks the prize by its rate " +
			"and records the draw in one step, so neither the odds nor the allowance can be bent. Twenty tries from one " +
			"address in ten minutes, made or not, counted before the token is looked up (429, with Retry-After).",
		Tags: garaponTags, Errors: []int{http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests},
	}, h.drawGarapon)
}

func noSuchGarapon() error { return huma.Error404NotFound("There is no such garapon.") }

func noSuchDrawingLink() error { return huma.Error404NotFound("There is no such drawing link.") }

// toGarapon is a stored garapon as the API answers it, without its prizes.
func toGarapon(g *db.GetGaraponRow) Garapon {
	return Garapon{
		ID: g.ID, Title: g.Title, Details: g.Details, GrandPrizeImage: g.GrandPrizeImage, Status: g.Status,
		StampRallyID: intPtr(g.StampRallyID), StampRallyTitle: g.StampRallyTitle, DefaultDraws: g.DefaultDraws,
		CreatedAt: g.CreatedAt, Prizes: []GaraponPrize{}, PlayerCount: g.PlayerCount, DrawCount: g.DrawCount,
	}
}

func toGaraponPrize(p *db.GaraponPrize) GaraponPrize {
	return GaraponPrize{
		ID: p.ID, Name: p.Name, BallColor: p.BallColor, Rate: p.Rate, IsGrand: p.IsGrand != 0, SortOrder: p.SortOrder,
	}
}

func toGaraponPlayer(p *db.GetGaraponPlayerRow) GaraponPlayer {
	return GaraponPlayer{
		ID: p.ID, GaraponID: p.GaraponID, Token: p.Token, PlayerName: p.PlayerName, World: p.World,
		MaxDraws: p.MaxDraws, DrawsUsed: p.DrawsUsed, StampCardToken: p.StampCardToken, CreatedAt: p.CreatedAt,
	}
}

func toGaraponDraw(d *db.GaraponDraw) GaraponDraw {
	out := GaraponDraw{
		ID: d.ID, GaraponID: d.GaraponID, PlayerID: intPtr(d.PlayerID), PlayerName: d.PlayerName,
		PrizeName: d.PrizeName, BallColor: d.BallColor, DrawnAt: d.DrawnAt,
	}
	if d.PrizeID != 0 {
		out.PrizeID = &d.PrizeID
	}
	return out
}

// loadGarapon reads a garapon with its prizes, or the 404 when there is none.
func (h handlers) loadGarapon(ctx context.Context, q *db.Queries, id int64) (Garapon, error) {
	row, err := q.GetGarapon(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Garapon{}, noSuchGarapon()
	}
	if err != nil {
		return Garapon{}, h.fail("read a garapon", err)
	}
	g := toGarapon(row)
	prizes, err := q.ListGaraponPrizes(ctx, id)
	if err != nil {
		return Garapon{}, h.fail("read a garapon's prizes", err)
	}
	for _, p := range prizes {
		g.Prizes = append(g.Prizes, toGaraponPrize(p))
	}
	return g, nil
}

func (h handlers) listGarapons(ctx context.Context, _ *struct{}) (*garaponListOutput, error) {
	rows, err := h.read().ListGarapons(ctx)
	if err != nil {
		return nil, h.fail("list garapons", err)
	}
	out := GaraponList{Garapons: make([]Garapon, 0, len(rows))}
	for _, r := range rows {
		out.Garapons = append(out.Garapons, toGarapon((*db.GetGaraponRow)(r)))
	}
	return &garaponListOutput{Body: out}, nil
}

func (h handlers) getGarapon(ctx context.Context, in *garaponIDInput) (*garaponDetailOutput, error) {
	q := h.read()
	g, err := h.loadGarapon(ctx, q, in.ID)
	if err != nil {
		return nil, err
	}
	players, err := q.ListGaraponPlayers(ctx, in.ID)
	if err != nil {
		return nil, h.fail("list a garapon's players", err)
	}
	draws, err := q.ListGaraponDraws(ctx, in.ID)
	if err != nil {
		return nil, h.fail("list a garapon's draws", err)
	}
	out := GaraponDetail{Garapon: g, Players: make([]GaraponPlayer, 0, len(players)), Draws: make([]GaraponDraw, 0, len(draws))}
	for _, p := range players {
		out.Players = append(out.Players, toGaraponPlayer((*db.GetGaraponPlayerRow)(p)))
	}
	for _, d := range draws {
		out.Draws = append(out.Draws, toGaraponDraw(d))
	}
	return &garaponDetailOutput{Body: out}, nil
}

// garaponColumns checks a garapon's fields the schema can't and puts them as they
// are stored: the prizes sanitized, and a linked rally that must exist and be open.
func (h handlers) garaponColumns(ctx context.Context, in GaraponInput) (db.UpdateGaraponParams, []festival.Prize, error) {
	prizes := make([]festival.Prize, 0, len(in.Prizes))
	for _, p := range in.Prizes {
		prizes = append(prizes, festival.Prize{Name: p.Name, BallColor: p.BallColor, Rate: p.Rate, IsGrand: p.IsGrand})
	}
	prizes, err := festival.SanitizePrizes(prizes)
	switch {
	case errors.Is(err, festival.ErrNoPrizes):
		return db.UpdateGaraponParams{}, nil, huma.Error422UnprocessableEntity("The garapon can't be saved as it is.",
			&huma.ErrorDetail{Location: "body.prizes", Message: "At least one prize with a name is required."})
	case errors.Is(err, festival.ErrManyGrand):
		return db.UpdateGaraponParams{}, nil, huma.Error422UnprocessableEntity("The garapon can't be saved as it is.",
			&huma.ErrorDetail{Location: "body.prizes", Message: "Only one prize can be the grand prize."})
	}
	if in.StampRallyID != nil {
		status, err := h.read().GetStampRallyStatus(ctx, *in.StampRallyID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return db.UpdateGaraponParams{}, nil, unknown("body.stampRallyId", *in.StampRallyID, "stamp rally")
		case err != nil:
			return db.UpdateGaraponParams{}, nil, h.fail("read a garapon's rally", err)
		case status != "open":
			return db.UpdateGaraponParams{}, nil, huma.Error409Conflict("The linked stamp rally is closed.")
		}
	}
	return db.UpdateGaraponParams{
		Title: strings.TrimSpace(in.Title), Details: in.Details, GrandPrizeImage: in.GrandPrizeImage,
		StampRallyID: nullInt(in.StampRallyID), DefaultDraws: festival.DefaultDraws(in.DefaultDraws),
	}, prizes, nil
}

// savePrizes replaces a garapon's prizes in the transaction q is bound to; the order
// given is the order kept.
func savePrizes(ctx context.Context, q *db.Queries, garaponID int64, prizes []festival.Prize) error {
	if err := q.DeleteGaraponPrizes(ctx, garaponID); err != nil {
		return err
	}
	for i, p := range prizes {
		if err := q.InsertGaraponPrize(ctx, db.InsertGaraponPrizeParams{
			GaraponID: garaponID, Name: p.Name, BallColor: p.BallColor, Rate: p.Rate, IsGrand: boolInt(p.IsGrand), SortOrder: int64(i),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (h handlers) createGarapon(ctx context.Context, in *garaponCreateInput) (*garaponCreatedOutput, error) {
	p, prizes, err := h.garaponColumns(ctx, in.Body)
	if err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	var id int64
	err = h.transact(ctx, func(q *db.Queries) error {
		var err error
		id, err = q.CreateGarapon(ctx, db.CreateGaraponParams{
			Title: p.Title, Details: p.Details, GrandPrizeImage: p.GrandPrizeImage, StampRallyID: p.StampRallyID, DefaultDraws: p.DefaultDraws,
		})
		if err != nil {
			return err
		}
		return savePrizes(ctx, q, id, prizes)
	})
	if err != nil {
		return nil, h.fail("create a garapon", err)
	}
	g, err := h.loadGarapon(ctx, h.read(), id)
	if err != nil {
		return nil, err
	}
	return &garaponCreatedOutput{Status: http.StatusCreated, Body: g}, nil
}

func (h handlers) updateGarapon(ctx context.Context, in *garaponSaveInput) (*garaponOutput, error) {
	p, prizes, err := h.garaponColumns(ctx, in.Body)
	if err != nil {
		return nil, err
	}
	p.ID = in.ID
	ctx = context.WithoutCancel(ctx)
	err = h.transact(ctx, func(q *db.Queries) error {
		n, err := q.UpdateGarapon(ctx, p)
		if err != nil {
			return err
		}
		if n == 0 {
			return noSuchGarapon()
		}
		return savePrizes(ctx, q, in.ID, prizes)
	})
	var problem huma.StatusError
	switch {
	case errors.As(err, &problem):
		return nil, problem
	case err != nil:
		return nil, h.fail("update a garapon", err)
	}
	g, err := h.loadGarapon(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	return &garaponOutput{Body: g}, nil
}

func (h handlers) deleteGarapon(ctx context.Context, in *garaponIDInput) (*struct{}, error) {
	n, err := h.write().DeleteGarapon(context.WithoutCancel(ctx), in.ID)
	if err != nil {
		return nil, h.fail("delete a garapon", err)
	}
	if n == 0 {
		return nil, noSuchGarapon()
	}
	return nil, nil
}

// setGaraponStatus closes or reopens a garapon and answers it.
func (h handlers) setGaraponStatus(ctx context.Context, id int64, status string) (*garaponOutput, error) {
	ctx = context.WithoutCancel(ctx)
	n, err := h.write().SetGaraponStatus(ctx, db.SetGaraponStatusParams{Status: status, ID: id})
	if err != nil {
		return nil, h.fail("set a garapon's status", err)
	}
	if n == 0 {
		return nil, noSuchGarapon()
	}
	g, err := h.loadGarapon(ctx, h.read(), id)
	if err != nil {
		return nil, err
	}
	return &garaponOutput{Body: g}, nil
}

func (h handlers) closeGarapon(ctx context.Context, in *garaponIDInput) (*garaponOutput, error) {
	return h.setGaraponStatus(ctx, in.ID, "closed")
}

func (h handlers) reopenGarapon(ctx context.Context, in *garaponIDInput) (*garaponOutput, error) {
	return h.setGaraponStatus(ctx, in.ID, "open")
}

func (h handlers) createGaraponPlayer(ctx context.Context, in *garaponPlayerInput) (*garaponPlayerCreatedOutput, error) {
	g, err := h.loadGarapon(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	name, world := strings.TrimSpace(in.Body.PlayerName), strings.TrimSpace(in.Body.World)
	if world == "" {
		name, world = model.SplitParticipantLabel(name)
	}
	// An unspecified allowance is the garapon's own default, so "two draws each for this
	// event" is set once rather than retyped per link.
	maxDraws := in.Body.MaxDraws
	if maxDraws < 1 {
		maxDraws = festival.DefaultDraws(g.DefaultDraws)
	}

	// The link and its card are made together; a card is the stamp rally's, with a token
	// of its own, so sharing it can't hand over the draws.
	ctx = context.WithoutCancel(ctx)
	var playerID int64
	var issuedCard bool
	err = h.transact(ctx, func(q *db.Queries) error {
		var err error
		playerID, err = q.CreateGaraponPlayer(ctx, db.CreateGaraponPlayerParams{
			GaraponID: in.ID, Token: random.Token(), PlayerName: name, World: world, MaxDraws: maxDraws,
		})
		if err != nil || g.StampRallyID == nil {
			return err
		}
		status, err := q.GetStampRallyStatus(ctx, *g.StampRallyID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && status != "open") {
			// A rally closed or gone since the link was made to it leaves a link with no card.
			return nil
		}
		if err != nil {
			return err
		}
		cardID, err := q.CreateRallyCard(ctx, db.CreateRallyCardParams{
			RallyID: *g.StampRallyID, Token: random.Token(), ParticipantName: name, World: world,
		})
		if err != nil {
			return err
		}
		issuedCard = true
		return q.SetPlayerStampCard(ctx, db.SetPlayerStampCardParams{StampCardID: sql.NullInt64{Int64: cardID, Valid: true}, ID: playerID})
	})
	if err != nil {
		return nil, h.fail("issue a drawing link", err)
	}
	p, err := h.read().GetGaraponPlayer(ctx, playerID)
	if err != nil {
		return nil, h.fail("read a drawing link", err)
	}
	if issuedCard {
		ResourceChanged(h.Live, "stamp-rallies")
	}
	return &garaponPlayerCreatedOutput{Status: http.StatusCreated, Body: toGaraponPlayer(p)}, nil
}

func (h handlers) deleteGaraponPlayer(ctx context.Context, in *garaponPlayerIDInput) (*struct{}, error) {
	ctx = context.WithoutCancel(ctx)
	g, err := h.loadGarapon(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	// The link must belong to the garapon in the path, or the open check below would
	// read the wrong garapon's status and could delete a link that has drawn from a
	// drum still open.
	p, err := h.read().GetGaraponPlayer(ctx, in.PlayerID)
	switch {
	case errors.Is(err, sql.ErrNoRows) || (err == nil && p.GaraponID != in.ID):
		return nil, noSuchDrawingLink()
	case err != nil:
		return nil, h.fail("read a drawing link", err)
	}
	closed := g.Status == "closed"
	if p.DrawsUsed > 0 && !closed {
		return nil, huma.Error409Conflict("This player has already drawn and can't be deleted while the garapon is open.")
	}
	var deleted int64
	err = h.transact(ctx, func(q *db.Queries) error {
		cardID, err := q.GetPlayerStampCardID(ctx, in.PlayerID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		deleted, err = q.DeleteGaraponPlayer(ctx, db.DeleteGaraponPlayerParams{PlayerID: in.PlayerID, Force: boolInt(closed)})
		if err != nil || deleted == 0 || !cardID.Valid {
			return err
		}
		// The card goes by the rule the rally's own card delete keeps: one that has
		// collected stamps stays until its rally is closed.
		return q.SweepPlayerStampCard(ctx, cardID.Int64)
	})
	if err != nil {
		return nil, h.fail("delete a drawing link", err)
	}
	if deleted == 0 {
		// It drew between the check and the delete.
		return nil, huma.Error409Conflict("This player has already drawn and can't be deleted while the garapon is open.")
	}
	ResourceChanged(h.Live, "stamp-rallies")
	return nil, nil
}

// loadDrawingLink reads the drawing link a token names and its garapon, or the 404.
func (h handlers) loadDrawingLink(ctx context.Context, q *db.Queries, token string) (*db.GetGaraponPlayerByTokenRow, Garapon, error) {
	p, err := q.GetGaraponPlayerByToken(ctx, strings.TrimSpace(token))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Garapon{}, noSuchDrawingLink()
	}
	if err != nil {
		return nil, Garapon{}, h.fail("read a drawing link", err)
	}
	g, err := h.loadGarapon(ctx, q, p.GaraponID)
	if err != nil {
		return nil, Garapon{}, err
	}
	return p, g, nil
}

func publicGaraponDraw(d *db.GaraponDraw, grand map[int64]bool) PublicGaraponDraw {
	return PublicGaraponDraw{PrizeName: d.PrizeName, BallColor: d.BallColor, IsGrand: grand[d.PrizeID], DrawnAt: d.DrawnAt}
}

func grandPrizes(g Garapon) map[int64]bool {
	grand := make(map[int64]bool, len(g.Prizes))
	for _, p := range g.Prizes {
		grand[p.ID] = p.IsGrand
	}
	return grand
}

func (h handlers) getGaraponPlayer(ctx context.Context, in *tokenInput) (*publicGaraponOutput, error) {
	q := h.read()
	p, g, err := h.loadDrawingLink(ctx, q, in.Token)
	if err != nil {
		return nil, err
	}
	draws, err := q.ListPlayerGaraponDraws(ctx, sql.NullInt64{Int64: p.ID, Valid: true})
	if err != nil {
		return nil, h.fail("list a player's draws", err)
	}
	pub := publicGarapon(g)
	grand := grandPrizes(g)
	out := PublicGaraponPlayer{
		Garapon: pub,
		Player: GaraponPlayerSummary{
			PlayerName: p.PlayerName, World: p.World, MaxDraws: p.MaxDraws, DrawsUsed: p.DrawsUsed, StampCardToken: p.StampCardToken,
		},
		Draws: make([]PublicGaraponDraw, 0, len(draws)),
	}
	for _, d := range draws {
		out.Draws = append(out.Draws, publicGaraponDraw(d, grand))
	}
	if g.StampRallyID != nil {
		out.FestivalLink = h.festivalLink(ctx, *g.StampRallyID)
	}
	return &publicGaraponOutput{Body: out}, nil
}

func (h handlers) drawGarapon(ctx context.Context, in *tokenInput) (*garaponDrawOutput, error) {
	// Before the token is looked up: a draw can't be undone, so the budget has to cover
	// tries at tokens that turn out wrong as well as right.
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.GaraponDrawLimit, ip); wait > 0 {
		h.Logger.Warn("garapon draw held back", "ip", ip)
		return nil, tooMany(wait, "Too many draws. Please try again later.")
	}
	if h.GaraponDrawLimit != nil {
		h.GaraponDrawLimit.Fail(ip)
	}
	p, err := h.read().GetGaraponPlayerByToken(ctx, strings.TrimSpace(in.Token))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, noSuchDrawingLink()
	}
	if err != nil {
		return nil, h.fail("read a drawing link", err)
	}

	// Checking the garapon and the allowance, picking and recording go in one step, so
	// two draws at once can't both take the last one.
	ctx = context.WithoutCancel(ctx)
	var out GaraponDrawResult
	err = h.transact(ctx, func(q *db.Queries) error {
		dc, err := q.GetDrawContext(ctx, p.ID)
		if err != nil {
			return err
		}
		if dc.Status != "open" {
			return huma.Error409Conflict("This garapon is closed.")
		}
		used, err := q.CountPlayerDraws(ctx, sql.NullInt64{Int64: p.ID, Valid: true})
		if err != nil {
			return err
		}
		if used >= dc.MaxDraws {
			return huma.Error409Conflict("No draws remaining.")
		}
		prizes, err := q.ListGaraponPrizes(ctx, dc.GaraponID)
		if err != nil {
			return err
		}
		if len(prizes) == 0 {
			return huma.Error409Conflict("This garapon has no prizes configured.")
		}
		rates := make([]float64, len(prizes))
		for i, pr := range prizes {
			rates[i] = pr.Rate
		}
		won := prizes[festival.PickPrize(rates, random.IntN)]
		// The log keeps the whole identity, not just the name: staff read it when handing
		// out a prize, and it outlives the link.
		id, err := q.InsertGaraponDraw(ctx, db.InsertGaraponDrawParams{
			GaraponID: dc.GaraponID, PlayerID: sql.NullInt64{Int64: p.ID, Valid: true}, PrizeID: won.ID,
			PlayerName: model.ParticipantLabel(dc.PlayerName, dc.World), PrizeName: won.Name, BallColor: won.BallColor,
		})
		if err != nil {
			return err
		}
		d, err := q.GetGaraponDraw(ctx, id)
		if err != nil {
			return err
		}
		after, err := q.CountPlayerDraws(ctx, sql.NullInt64{Int64: p.ID, Valid: true})
		if err != nil {
			return err
		}
		out = GaraponDrawResult{Draw: publicGaraponDraw(d, map[int64]bool{won.ID: won.IsGrand != 0}), DrawsUsed: after, MaxDraws: dc.MaxDraws}
		return nil
	})
	var problem huma.StatusError
	switch {
	case errors.As(err, &problem):
		return nil, problem
	case errors.Is(err, sql.ErrNoRows):
		return nil, noSuchDrawingLink()
	case err != nil:
		return nil, h.fail("record a garapon draw", err)
	}
	// The write middleware leaves public paths out, so tell the Garapon page.
	ResourceChanged(h.Live, "garapons")
	return &garaponDrawOutput{Body: out}, nil
}
