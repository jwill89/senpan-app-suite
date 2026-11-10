package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/db"
	"app-suite/internal/discord"
	"app-suite/internal/festival"
	"app-suite/internal/model"
	"app-suite/internal/permissions"
	"app-suite/internal/uploads"
)

// Ema plaques: a festival's board takes wishing plaques on its days. Staff set the
// board up and review the plaques visitors design on the site, which are posted to
// the day's Discord webhook when approved. The artist works the queue of plaques
// visitors ask for. Visitors submit through the festival's address.

var emaTags = []string{"Ema"}

const (
	// emaImageRelDir is where site plaques wait for review, under the web root. These
	// are visitors' uploads, kept only until they are posted or rejected.
	emaImageRelDir = "images/ema_plaques"
	// maxEmaImageBytes bounds a plaque upload. The designer exports at most 2048 px
	// wide, which as a PNG is well under this.
	maxEmaImageBytes = 12 << 20
	// maxEmaImageSide bounds a plaque's pixel size, checked before the image is kept.
	maxEmaImageSide = 4096
	// maxEmaArtistImageBytes bounds the artist's finished plaque: Discord's default
	// upload limit, so nothing is accepted that the post would then be refused for.
	maxEmaArtistImageBytes = 10 << 20
	// multipartOverhead is what a form takes beyond its file.
	multipartOverhead = 1 << 20
)

// EmaWindow is one day a board takes plaques on, as the staff see it.
type EmaWindow struct {
	ID          int64  `json:"id" doc:"The day's id."`
	Label       string `json:"label" doc:"What it is called, such as Day 1."`
	StartAt     string `json:"startAt" format:"date-time" doc:"When it starts, RFC 3339 in UTC."`
	EndAt       string `json:"endAt" format:"date-time" doc:"When it ends, RFC 3339 in UTC; the day takes plaques until then."`
	WebhookURL  string `json:"webhookUrl" doc:"The Discord webhook the day's plaques are posted to; empty for none. Only staff see it."`
	ArtistLimit int64  `json:"artistLimit" doc:"The most artist requests the day takes; 0 for no limit."`
	ArtistCount int64  `json:"artistCount" doc:"The artist requests made for the day."`
	SortOrder   int64  `json:"sortOrder" doc:"Its place in the board's list of days."`
}

// EmaBoard is a plaque board as the staff see it.
type EmaBoard struct {
	ID              int64       `json:"id" doc:"The board's id."`
	FestivalMapID   int64       `json:"festivalMapId" doc:"The festival it belongs to; a festival has one board."`
	FestivalMapName string      `json:"festivalMapName" doc:"That festival's title."`
	Title           string      `json:"title" doc:"The board's title."`
	TemplateImage   string      `json:"templateImage" doc:"The blank plaque to download or design on: a path from the image library."`
	Instructions    string      `json:"instructions" doc:"How to make and post a plaque yourself (Markdown)."`
	DiscordLink     string      `json:"discordLink" doc:"Where visitors post their own plaques; empty for none."`
	AllowSelf       bool        `json:"allowSelf" doc:"Whether visitors may make a plaque anywhere and post it themselves."`
	AllowSite       bool        `json:"allowSite" doc:"Whether visitors may design a plaque on the site, for staff to review."`
	AllowArtist     bool        `json:"allowArtist" doc:"Whether visitors may ask the artist for a plaque."`
	ArtistName      string      `json:"artistName" doc:"The artist's name."`
	Status          string      `json:"status" enum:"open,closed" doc:"A closed board takes no plaques and leaves the festival page; its queue stays workable."`
	CreatedAt       string      `json:"createdAt" format:"date-time" doc:"When it was made, RFC 3339 in UTC."`
	Windows         []EmaWindow `json:"windows" nullable:"false" doc:"Its days in order. Empty in a list."`
	PendingCount    int64       `json:"pendingCount" doc:"Site plaques waiting for review."`
	RequestedCount  int64       `json:"requestedCount" doc:"Artist requests not yet done."`
}

// EmaWindowInput is a day as a save sends it.
type EmaWindowInput struct {
	ID          int64  `json:"id,omitempty" minimum:"0" doc:"The day's id to keep it (and its plaques); 0 or left out adds one. A day left out of the save is deleted, and its plaques keep their label. An id that isn't on this board is a 409."`
	Label       string `json:"label" maxLength:"100" doc:"What it is called; empty for Day N."`
	StartAt     string `json:"startAt" format:"date-time" doc:"When it starts, RFC 3339."`
	EndAt       string `json:"endAt" format:"date-time" doc:"When it ends, RFC 3339; after the start. Days can't overlap."`
	WebhookURL  string `json:"webhookUrl" maxLength:"500" doc:"The Discord webhook its plaques are posted to: https, on Discord, under /api/webhooks/; empty for none."`
	ArtistLimit int64  `json:"artistLimit" minimum:"0" maximum:"1000" doc:"The most artist requests it takes; 0 for no limit."`
}

// EmaBoardInput is a board as a save sends it: all of its editable fields, since a save
// replaces them. Its status changes through close and reopen.
type EmaBoardInput struct {
	FestivalMapID int64            `json:"festivalMapId" minimum:"1" doc:"The festival it belongs to. One that doesn't exist is refused, and so is one that already has a board."`
	Title         string           `json:"title" maxLength:"200" doc:"The board's title."`
	TemplateImage string           `json:"templateImage" maxLength:"500" doc:"The blank plaque: a path from the image library. Needed when visitors may make a plaque themselves or on the site."`
	Instructions  string           `json:"instructions" maxLength:"20000" doc:"How to make and post a plaque yourself (Markdown)."`
	DiscordLink   string           `json:"discordLink" maxLength:"500" doc:"Where visitors post their own plaques: a web address; empty for none."`
	AllowSelf     bool             `json:"allowSelf" doc:"Whether visitors may make a plaque anywhere and post it themselves."`
	AllowSite     bool             `json:"allowSite" doc:"Whether visitors may design a plaque on the site."`
	AllowArtist   bool             `json:"allowArtist" doc:"Whether visitors may ask the artist for a plaque. At least one of the three ways is needed."`
	ArtistName    string           `json:"artistName" maxLength:"100" doc:"The artist's name; empty for Tara."`
	Windows       []EmaWindowInput `json:"windows" minItems:"1" maxItems:"60" nullable:"false" doc:"The days it takes plaques on, at least one."`
}

// EmaBoardList is every board.
type EmaBoardList struct {
	Boards []EmaBoard `json:"boards" nullable:"false" doc:"Newest first, with the counts of what waits for staff and the artist, and without their days."`
}

// EmaPlaque is a plaque or a request for one, as the staff see it.
type EmaPlaque struct {
	ID              int64   `json:"id" doc:"The plaque's id."`
	BoardID         int64   `json:"boardId" doc:"The board it was made on."`
	WindowID        *int64  `json:"windowId" nullable:"true" doc:"The day it was made for; null once that day is deleted."`
	WindowLabel     string  `json:"windowLabel" doc:"That day's label, as it was when the plaque was made."`
	Kind            string  `json:"kind" enum:"site,artist" doc:"site: designed on the site for review. artist: a request for the artist."`
	CharacterName   string  `json:"characterName" doc:"The visitor's character."`
	World           string  `json:"world" doc:"Their home world."`
	Description     string  `json:"description" doc:"What an artist request asks for; empty for a site plaque."`
	Image           string  `json:"image" doc:"A site plaque's picture while it waits; empty once it is posted or rejected."`
	Status          string  `json:"status" enum:"pending,posting,posted,rejected,requested,complete" doc:"pending: awaiting review. posting: being posted. posted and rejected end a site plaque. requested and complete are an artist request's."`
	ReviewedBy      string  `json:"reviewedBy" doc:"Who reviewed or finished it."`
	ReviewedAt      *string `json:"reviewedAt" nullable:"true" doc:"When, RFC 3339 in UTC; null while it waits."`
	CreatedAt       string  `json:"createdAt" format:"date-time" doc:"When it was made, RFC 3339 in UTC."`
	FestivalMapName string  `json:"festivalMapName" doc:"The festival it belongs to."`
}

// EmaPlaqueList is a list of plaques.
type EmaPlaqueList struct {
	Plaques []EmaPlaque `json:"plaques" nullable:"false" doc:"Newest first."`
}

// PublicEmaWindow is a day as a visitor sees it: never its webhook.
type PublicEmaWindow struct {
	ID          int64  `json:"id" doc:"The day's id."`
	Label       string `json:"label" doc:"What it is called."`
	StartAt     string `json:"startAt" format:"date-time" doc:"When it starts, RFC 3339 in UTC."`
	EndAt       string `json:"endAt" format:"date-time" doc:"When it ends, RFC 3339 in UTC."`
	IsOpen      bool   `json:"isOpen" doc:"Whether it is taking plaques now."`
	ArtistLimit int64  `json:"artistLimit" doc:"The most artist requests it takes; 0 for no limit."`
	ArtistLeft  int64  `json:"artistLeft" doc:"The artist requests it can still take; 0 when it has no limit or is full."`
}

// PublicEmaPlaque is a plaque as the visitor who made it sees it.
type PublicEmaPlaque struct {
	ID          int64  `json:"id" doc:"The plaque's id."`
	WindowID    *int64 `json:"windowId" nullable:"true" doc:"The day it was made for; null once that day is deleted."`
	WindowLabel string `json:"windowLabel" doc:"That day's label."`
	Kind        string `json:"kind" enum:"site,artist" doc:"site: designed on the site. artist: a request for the artist."`
	Status      string `json:"status" enum:"pending,posting,posted,rejected,requested,complete" doc:"Where it stands."`
	Description string `json:"description" doc:"What an artist request asks for."`
	Image       string `json:"image" doc:"The visitor's own upload while it waits for review; empty once it is posted or rejected."`
	CreatedAt   string `json:"createdAt" format:"date-time" doc:"When it was made, RFC 3339 in UTC."`
}

// FestivalEma is a festival's open plaque board as a visitor sees it.
type FestivalEma struct {
	ID                  int64             `json:"id" doc:"The board's id."`
	Title               string            `json:"title" doc:"The board's title."`
	TemplateImage       string            `json:"templateImage" doc:"The blank plaque."`
	Instructions        string            `json:"instructions" doc:"How to make and post a plaque yourself (Markdown)."`
	DiscordLink         string            `json:"discordLink" doc:"Where visitors post their own plaques; empty for none."`
	AllowSelf           bool              `json:"allowSelf" doc:"Whether visitors may make a plaque anywhere and post it themselves."`
	AllowSite           bool              `json:"allowSite" doc:"Whether visitors may design a plaque on the site."`
	AllowArtist         bool              `json:"allowArtist" doc:"Whether visitors may ask the artist for a plaque."`
	ArtistName          string            `json:"artistName" doc:"The artist's name."`
	Windows             []PublicEmaWindow `json:"windows" nullable:"false" doc:"Its days in order."`
	CurrentWindowID     *int64            `json:"currentWindowId" nullable:"true" doc:"The day taking plaques now; null when none."`
	HasPlaqueThisWindow bool              `json:"hasPlaqueThisWindow" doc:"Whether the visitor already has a plaque for the day open now: one a day."`
	Plaques             []PublicEmaPlaque `json:"plaques" nullable:"false" doc:"The visitor's own plaques; empty when no name was sent."`
}

// EmaRequestInput is a visitor's request for the artist to make a plaque.
type EmaRequestInput struct {
	CharacterName  string `json:"characterName" maxLength:"60" doc:"Their character."`
	World          string `json:"world" maxLength:"60" doc:"Their home world."`
	Description    string `json:"description" maxLength:"2000" doc:"What they would like the plaque to be."`
	TurnstileToken string `json:"turnstileToken,omitempty" maxLength:"4096" doc:"The Cloudflare Turnstile response; needed only when the bot check is on."`
}

// EmaPlaqueForm is a site plaque a visitor sends.
type EmaPlaqueForm struct {
	Image          huma.FormFile `form:"image" contentType:"image/*" required:"true" doc:"The plaque: a PNG of at most 12 MB and 4096 pixels a side."`
	CharacterName  string        `form:"characterName" required:"false" maxLength:"60" doc:"Their character."`
	World          string        `form:"world" required:"false" maxLength:"60" doc:"Their home world."`
	TurnstileToken string        `form:"turnstileToken" required:"false" maxLength:"4096" doc:"The Cloudflare Turnstile response; needed only when the bot check is on."`
}

// EmaRequestPostForm is the artist's finished plaque.
type EmaRequestPostForm struct {
	Image huma.FormFile `form:"image" contentType:"image/*" required:"true" doc:"The finished plaque: a PNG, JPEG, GIF or WebP of at most 10 MB, judged from its bytes and never kept."`
}

// GoogleFontList is the Google Fonts families.
type GoogleFontList struct {
	Families []string `json:"families" nullable:"false" doc:"The family names, most popular first. Empty when no API key is set or Google can't be reached."`
}

type emaBoardIDInput struct {
	ID int64 `path:"id" doc:"The board's id."`
}

type emaBoardSaveInput struct {
	ID   int64 `path:"id" doc:"The board's id."`
	Body EmaBoardInput
}

type emaBoardCreateInput struct{ Body EmaBoardInput }

type emaPlaqueIDInput struct {
	ID       int64 `path:"id" doc:"The board's id."`
	PlaqueID int64 `path:"plaqueId" doc:"The plaque's id."`
}

type emaRequestIDInput struct {
	ID int64 `path:"id" doc:"The request's id."`
}

type emaRequestPostInput struct {
	ID      int64 `path:"id" doc:"The request's id."`
	RawBody huma.MultipartFormFiles[EmaRequestPostForm]
}

type emaSubmitInput struct {
	IDOrSlug string `path:"idOrSlug" maxLength:"100" doc:"The festival's shortcode or numeric id."`
	RawBody  huma.MultipartFormFiles[EmaPlaqueForm]
}

type emaRequestSubmitInput struct {
	IDOrSlug string `path:"idOrSlug" maxLength:"100" doc:"The festival's shortcode or numeric id."`
	Body     EmaRequestInput
}

type emaBoardOutput struct{ Body EmaBoard }

type emaBoardCreatedOutput struct {
	Status int
	Body   EmaBoard
}

type emaBoardListOutput struct{ Body EmaBoardList }

type emaPlaqueOutput struct{ Body EmaPlaque }

type emaPlaqueListOutput struct{ Body EmaPlaqueList }

type publicEmaPlaqueOutput struct {
	Status int
	Body   PublicEmaPlaque
}

type googleFontsOutput struct{ Body GoogleFontList }

func (h handlers) registerEma(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.FestivalEma)}
	queue := huma.Middlewares{h.permitted(api, permissions.FestivalEmaRequests)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}
	notFound := slices.Concat(guarded, []int{http.StatusNotFound})
	invalid := slices.Concat(notFound, []int{http.StatusConflict, http.StatusUnprocessableEntity})
	posts := slices.Concat(discordErrors, []int{http.StatusConflict})

	huma.Register(api, huma.Operation{
		OperationID: "list-ema-boards", Method: http.MethodGet, Path: "/ema-boards",
		Summary: "Every plaque board", Tags: emaTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.listEmaBoards)
	huma.Register(api, huma.Operation{
		OperationID: "create-ema-board", Method: http.MethodPost, Path: "/ema-boards",
		Summary: "Make a plaque board", Description: "It starts open.", Tags: emaTags,
		DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: slices.Concat(guarded, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.createEmaBoard)
	huma.Register(api, huma.Operation{
		OperationID: "get-ema-board", Method: http.MethodGet, Path: "/ema-boards/{id}",
		Summary: "One plaque board with its days", Tags: emaTags, Security: secured, Middlewares: page, Errors: notFound,
	}, h.getEmaBoard)
	huma.Register(api, huma.Operation{
		OperationID: "update-ema-board", Method: http.MethodPut, Path: "/ema-boards/{id}",
		Summary: "Replace a plaque board's editable fields",
		Description: "Its status is kept. Its days are reconciled by id, so a plaque stays attached to its day across " +
			"an edit.",
		Tags: emaTags, Security: secured, Middlewares: page, Errors: invalid,
	}, h.updateEmaBoard)
	huma.Register(api, huma.Operation{
		OperationID: "close-ema-board", Method: http.MethodPost, Path: "/ema-boards/{id}/close",
		Summary:     "Close a plaque board",
		Description: "It takes no plaques and leaves the festival page; its review queue stays workable.",
		Tags:        emaTags, Security: secured, Middlewares: page, Errors: notFound,
	}, h.closeEmaBoard)
	huma.Register(api, huma.Operation{
		OperationID: "reopen-ema-board", Method: http.MethodPost, Path: "/ema-boards/{id}/reopen",
		Summary: "Open a closed plaque board again", Tags: emaTags, Security: secured, Middlewares: page, Errors: notFound,
	}, h.reopenEmaBoard)
	huma.Register(api, huma.Operation{
		OperationID: "delete-ema-board", Method: http.MethodDelete, Path: "/ema-boards/{id}",
		Summary:     "Delete a plaque board",
		Description: "Its days and plaques go with it, and so do the pictures still waiting for review.",
		Tags:        emaTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page, Errors: notFound,
	}, h.deleteEmaBoard)
	huma.Register(api, huma.Operation{
		OperationID: "list-ema-plaques", Method: http.MethodGet, Path: "/ema-boards/{id}/plaques",
		Summary: "A board's plaques", Description: "The review queue and the record.", Tags: emaTags,
		Security: secured, Middlewares: page, Errors: notFound,
	}, h.listEmaPlaques)
	huma.Register(api, huma.Operation{
		OperationID: "approve-ema-plaque", Method: http.MethodPost, Path: "/ema-boards/{id}/plaques/{plaqueId}/approve",
		Summary: "Approve a site plaque",
		Description: "Claims the plaque so a second approval is refused, posts it to its day's Discord webhook, records it " +
			"as posted and deletes its picture. A failed post hands it back for another try; when Discord didn't " +
			"answer, the plaque may or may not have posted and the channel should be checked first.",
		Tags: emaTags, Security: secured, Middlewares: page,
		Errors: slices.Concat(notFound, posts),
	}, h.approveEmaPlaque)
	huma.Register(api, huma.Operation{
		OperationID: "reject-ema-plaque", Method: http.MethodPost, Path: "/ema-boards/{id}/plaques/{plaqueId}/reject",
		Summary:     "Reject a site plaque",
		Description: "Deletes its picture and frees its author to send another for the same day.",
		Tags:        emaTags, Security: secured, Middlewares: page, Errors: slices.Concat(notFound, []int{http.StatusConflict}),
	}, h.rejectEmaPlaque)
	huma.Register(api, huma.Operation{
		OperationID: "list-ema-requests", Method: http.MethodGet, Path: "/ema-requests",
		Summary: "The artist's queue", Description: "The requests for the artist across every board.", Tags: emaTags,
		Security: secured, Middlewares: queue, Errors: guarded,
	}, h.listEmaRequests)
	huma.Register(api, huma.Operation{
		OperationID: "complete-ema-request", Method: http.MethodPost, Path: "/ema-requests/{id}/complete",
		Summary:     "Mark an artist request done",
		Description: "For a plaque the artist posts by hand.",
		Tags:        emaTags, Security: secured, Middlewares: queue, Errors: slices.Concat(notFound, []int{http.StatusConflict}),
	}, h.completeEmaRequest)
	huma.Register(api, huma.Operation{
		OperationID: "reopen-ema-request", Method: http.MethodPost, Path: "/ema-requests/{id}/reopen",
		Summary: "Reopen a finished artist request", Tags: emaTags, Security: secured, Middlewares: queue,
		Errors: slices.Concat(notFound, []int{http.StatusConflict}),
	}, h.reopenEmaRequest)
	huma.Register(api, huma.Operation{
		OperationID: "post-ema-request", Method: http.MethodPost, Path: "/ema-requests/{id}/post",
		Summary: "Post the artist's finished plaque",
		Description: "Sends the picture to the request's day webhook, crediting the visitor and the artist, and marks the " +
			"request complete. There is no review, and nothing is kept: the picture is read into memory, posted and " +
			"dropped. Claimed like an approval, so a double click can't post twice.",
		Tags: emaTags, Security: secured, Middlewares: queue, MaxBodyBytes: maxEmaArtistImageBytes + multipartOverhead,
		Errors: slices.Concat(notFound, posts, []int{http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity}),
	}, h.postEmaRequest)
	huma.Register(api, huma.Operation{
		OperationID: "submit-ema-plaque", Method: http.MethodPost, Path: "/festivals/{idOrSlug}/ema-plaques",
		Summary: "Send a plaque designed on the site",
		Description: "Held for staff review. One per person per day. Ten tries from one address in ten minutes, made or " +
			"not, counted before anything else (429, with Retry-After).",
		Tags: emaTags, Middlewares: huma.Middlewares{h.emaBudget(api)}, MaxBodyBytes: maxEmaImageBytes + multipartOverhead,
		DefaultStatus: http.StatusCreated,
		Errors: []int{
			http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusRequestEntityTooLarge,
			http.StatusUnprocessableEntity, http.StatusTooManyRequests,
		},
	}, h.submitEmaPlaque)
	huma.Register(api, huma.Operation{
		OperationID: "request-ema-plaque", Method: http.MethodPost, Path: "/festivals/{idOrSlug}/ema-requests",
		Summary: "Ask the artist for a plaque",
		Description: "One per person per day, within the day's limit. Ten tries from one address in ten minutes, made or " +
			"not, counted before anything else (429, with Retry-After).",
		Tags: emaTags, Middlewares: huma.Middlewares{h.emaBudget(api)}, DefaultStatus: http.StatusCreated,
		Errors: []int{
			http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity,
			http.StatusTooManyRequests,
		},
	}, h.requestEmaPlaque)
	huma.Register(api, huma.Operation{
		OperationID: "list-google-fonts", Method: http.MethodGet, Path: "/google-fonts",
		Summary: "The Google Fonts families",
		Description: "For the plaque designer's font picker. The server reads the list with its own key, which stays " +
			"secret, and keeps it for twelve hours.",
		Tags: emaTags, Middlewares: huma.Middlewares{public},
	}, h.listGoogleFonts)
}

// emaBudget counts every public plaque submission against the address before the
// request's body is read, so a flood is turned away without being parsed.
func (h handlers) emaBudget(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		ip := requestOf(ctx.Context()).IP
		if wait := retryAfter(h.EmaLimit, ip); wait > 0 {
			h.Logger.Warn("ema plaque submission held back", "ip", ip)
			ctx.SetHeader("Retry-After", fmt.Sprint(max(int((wait+time.Second-1)/time.Second), 1)))
			_ = huma.WriteErr(api, ctx, http.StatusTooManyRequests, "Too many plaques sent. Please try again later.")
			return
		}
		if h.EmaLimit != nil {
			h.EmaLimit.Fail(ip)
		}
		next(ctx)
	}
}

func noSuchEmaBoard() error { return huma.Error404NotFound("There is no such plaque board.") }

func noSuchPlaque() error { return huma.Error404NotFound("There is no such plaque.") }

func noSuchEmaRequest() error { return huma.Error404NotFound("There is no such request.") }

// fieldProblem is a 422 naming one field and saying what is wrong with it.
func fieldProblem(location string, value any, msg string) error {
	return huma.Error422UnprocessableEntity(msg, &huma.ErrorDetail{Location: location, Value: value, Message: msg})
}

func toEmaBoard(b *db.GetEmaBoardRow) EmaBoard {
	return EmaBoard{
		ID: b.ID, FestivalMapID: b.FestivalMapID, FestivalMapName: b.FestivalMapName, Title: b.Title,
		TemplateImage: b.TemplateImage, Instructions: b.Instructions, DiscordLink: b.DiscordLink,
		AllowSelf: b.AllowSelf != 0, AllowSite: b.AllowSite != 0, AllowArtist: b.AllowArtist != 0, ArtistName: b.ArtistName,
		Status: b.Status, CreatedAt: b.CreatedAt, Windows: []EmaWindow{}, PendingCount: b.PendingCount,
		RequestedCount: b.RequestedCount,
	}
}

func toEmaPlaque(p *db.GetEmaPlaqueRow) EmaPlaque {
	return EmaPlaque{
		ID: p.ID, BoardID: p.BoardID, WindowID: intPtr(p.WindowID), WindowLabel: p.WindowLabel, Kind: p.Kind,
		CharacterName: p.CharacterName, World: p.World, Description: p.Description, Image: p.Image, Status: p.Status,
		ReviewedBy: p.ReviewedBy, ReviewedAt: p.ReviewedAt, CreatedAt: p.CreatedAt, FestivalMapName: p.FestivalMapName,
	}
}

func toPublicEmaPlaque(p *db.GetEmaPlaqueRow) PublicEmaPlaque {
	return PublicEmaPlaque{
		ID: p.ID, WindowID: intPtr(p.WindowID), WindowLabel: p.WindowLabel, Kind: p.Kind, Status: p.Status,
		Description: p.Description, Image: p.Image, CreatedAt: p.CreatedAt,
	}
}

// findEmaBoard reads a board with its days, or nil when there is none.
func (h handlers) findEmaBoard(ctx context.Context, q *db.Queries, board func() (*db.GetEmaBoardRow, error)) (*EmaBoard, error) {
	row, err := board()
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, h.fail("read a plaque board", err)
	}
	b := toEmaBoard(row)
	windows, err := q.ListEmaWindows(ctx, row.ID)
	if err != nil {
		return nil, h.fail("read a board's days", err)
	}
	for _, w := range windows {
		b.Windows = append(b.Windows, EmaWindow{
			ID: w.ID, Label: w.Label, StartAt: w.StartAt, EndAt: w.EndAt, WebhookURL: w.WebhookUrl,
			ArtistLimit: w.ArtistLimit, ArtistCount: w.ArtistCount, SortOrder: w.SortOrder,
		})
	}
	return &b, nil
}

func (h handlers) loadEmaBoard(ctx context.Context, q *db.Queries, id int64) (EmaBoard, error) {
	b, err := h.findEmaBoard(ctx, q, func() (*db.GetEmaBoardRow, error) { return q.GetEmaBoard(ctx, id) })
	if err != nil {
		return EmaBoard{}, err
	}
	if b == nil {
		return EmaBoard{}, noSuchEmaBoard()
	}
	return *b, nil
}

func (h handlers) listEmaBoards(ctx context.Context, _ *struct{}) (*emaBoardListOutput, error) {
	rows, err := h.read().ListEmaBoards(ctx)
	if err != nil {
		return nil, h.fail("list plaque boards", err)
	}
	out := EmaBoardList{Boards: make([]EmaBoard, 0, len(rows))}
	for _, r := range rows {
		out.Boards = append(out.Boards, toEmaBoard((*db.GetEmaBoardRow)(r)))
	}
	return &emaBoardListOutput{Body: out}, nil
}

func (h handlers) getEmaBoard(ctx context.Context, in *emaBoardIDInput) (*emaBoardOutput, error) {
	b, err := h.loadEmaBoard(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	return &emaBoardOutput{Body: b}, nil
}

// emaBoardColumns checks a board the schema can't and puts it as it is stored.
func emaBoardColumns(in EmaBoardInput) (festival.EmaBoard, error) {
	b := festival.EmaBoard{
		FestivalMapID: in.FestivalMapID, TemplateImage: in.TemplateImage, DiscordLink: in.DiscordLink,
		ArtistName: in.ArtistName, AllowSelf: in.AllowSelf, AllowSite: in.AllowSite, AllowArtist: in.AllowArtist,
		Windows: make([]festival.EmaWindow, 0, len(in.Windows)),
	}
	for _, w := range in.Windows {
		b.Windows = append(b.Windows, festival.EmaWindow{
			ID: w.ID, Label: w.Label, StartAt: w.StartAt, EndAt: w.EndAt, WebhookURL: w.WebhookURL, ArtistLimit: w.ArtistLimit,
		})
	}
	if bad := festival.CheckBoard(&b); bad != nil {
		return b, fieldProblem("body."+bad.Field, nil, bad.Message)
	}
	return b, nil
}

// checkEmaFestival confirms the festival a board is filed under exists and that no
// other board already holds it. exceptID is the board being saved, 0 on create.
func (h handlers) checkEmaFestival(ctx context.Context, mapID, exceptID int64) error {
	q := h.read()
	if _, err := q.GetFestivalMap(ctx, mapID); errors.Is(err, sql.ErrNoRows) {
		return unknown("body.festivalMapId", mapID, "festival")
	} else if err != nil {
		return h.fail("read a festival for a plaque board", err)
	}
	other, err := q.GetEmaBoardForMap(ctx, mapID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return h.fail("read a festival's plaque board", err)
	}
	if other.ID != exceptID {
		return huma.Error409Conflict("That festival already has a plaque board.")
	}
	return nil
}

func (h handlers) createEmaBoard(ctx context.Context, in *emaBoardCreateInput) (*emaBoardCreatedOutput, error) {
	b, err := emaBoardColumns(in.Body)
	if err != nil {
		return nil, err
	}
	if err := h.checkEmaFestival(ctx, b.FestivalMapID, 0); err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	var id int64
	err = h.transact(ctx, func(q *db.Queries) error {
		var err error
		id, err = q.CreateEmaBoard(ctx, db.CreateEmaBoardParams{
			FestivalMapID: b.FestivalMapID, Title: strings.TrimSpace(in.Body.Title), TemplateImage: b.TemplateImage,
			Instructions: in.Body.Instructions, DiscordLink: b.DiscordLink, AllowSelf: boolInt(b.AllowSelf),
			AllowSite: boolInt(b.AllowSite), AllowArtist: boolInt(b.AllowArtist), ArtistName: b.ArtistName,
		})
		if err != nil {
			return err
		}
		for i, w := range b.Windows {
			if err := q.InsertEmaWindow(ctx, db.InsertEmaWindowParams{
				BoardID: id, Label: w.Label, StartAt: w.StartAt, EndAt: w.EndAt, WebhookUrl: w.WebhookURL,
				ArtistLimit: w.ArtistLimit, SortOrder: int64(i),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if uniqueViolation(err) {
		return nil, huma.Error409Conflict("That festival already has a plaque board.")
	}
	if err != nil {
		return nil, h.fail("create a plaque board", err)
	}
	created, err := h.loadEmaBoard(ctx, h.read(), id)
	if err != nil {
		return nil, err
	}
	return &emaBoardCreatedOutput{Status: http.StatusCreated, Body: created}, nil
}

func (h handlers) updateEmaBoard(ctx context.Context, in *emaBoardSaveInput) (*emaBoardOutput, error) {
	b, err := emaBoardColumns(in.Body)
	if err != nil {
		return nil, err
	}
	if _, err := h.loadEmaBoard(ctx, h.read(), in.ID); err != nil {
		return nil, err
	}
	if err := h.checkEmaFestival(ctx, b.FestivalMapID, in.ID); err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	err = h.transact(ctx, func(q *db.Queries) error {
		n, err := q.UpdateEmaBoard(ctx, db.UpdateEmaBoardParams{
			FestivalMapID: b.FestivalMapID, Title: strings.TrimSpace(in.Body.Title), TemplateImage: b.TemplateImage,
			Instructions: in.Body.Instructions, DiscordLink: b.DiscordLink, AllowSelf: boolInt(b.AllowSelf),
			AllowSite: boolInt(b.AllowSite), AllowArtist: boolInt(b.AllowArtist), ArtistName: b.ArtistName, ID: in.ID,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return noSuchEmaBoard()
		}
		existing, err := q.ListEmaWindowIDs(ctx, in.ID)
		if err != nil {
			return err
		}
		kept := make(map[int64]bool, len(b.Windows))
		for i, w := range b.Windows {
			if w.ID == 0 {
				if err := q.InsertEmaWindow(ctx, db.InsertEmaWindowParams{
					BoardID: in.ID, Label: w.Label, StartAt: w.StartAt, EndAt: w.EndAt, WebhookUrl: w.WebhookURL,
					ArtistLimit: w.ArtistLimit, SortOrder: int64(i),
				}); err != nil {
					return err
				}
				continue
			}
			if !slices.Contains(existing, w.ID) {
				return huma.Error409Conflict("That day doesn't belong to this board - reload and try again.")
			}
			kept[w.ID] = true
			if _, err := q.UpdateEmaWindow(ctx, db.UpdateEmaWindowParams{
				Label: w.Label, StartAt: w.StartAt, EndAt: w.EndAt, WebhookUrl: w.WebhookURL, ArtistLimit: w.ArtistLimit,
				SortOrder: int64(i), ID: w.ID, BoardID: in.ID,
			}); err != nil {
				return err
			}
		}
		for _, id := range existing {
			if !kept[id] {
				if err := q.DeleteEmaWindow(ctx, id); err != nil {
					return err
				}
			}
		}
		return nil
	})
	var problem huma.StatusError
	switch {
	case errors.As(err, &problem):
		return nil, problem
	case uniqueViolation(err):
		return nil, huma.Error409Conflict("That festival already has a plaque board.")
	case err != nil:
		return nil, h.fail("update a plaque board", err)
	}
	updated, err := h.loadEmaBoard(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	return &emaBoardOutput{Body: updated}, nil
}

func (h handlers) setEmaBoardStatus(ctx context.Context, id int64, status string) (*emaBoardOutput, error) {
	ctx = context.WithoutCancel(ctx)
	n, err := h.write().SetEmaBoardStatus(ctx, db.SetEmaBoardStatusParams{Status: status, ID: id})
	if err != nil {
		return nil, h.fail("set a plaque board's status", err)
	}
	if n == 0 {
		return nil, noSuchEmaBoard()
	}
	b, err := h.loadEmaBoard(ctx, h.read(), id)
	if err != nil {
		return nil, err
	}
	return &emaBoardOutput{Body: b}, nil
}

func (h handlers) closeEmaBoard(ctx context.Context, in *emaBoardIDInput) (*emaBoardOutput, error) {
	return h.setEmaBoardStatus(ctx, in.ID, "closed")
}

func (h handlers) reopenEmaBoard(ctx context.Context, in *emaBoardIDInput) (*emaBoardOutput, error) {
	return h.setEmaBoardStatus(ctx, in.ID, "open")
}

func (h handlers) deleteEmaBoard(ctx context.Context, in *emaBoardIDInput) (*struct{}, error) {
	// Detached: the rows and the files go together, so a client leaving between the two
	// must not strand the files.
	ctx = context.WithoutCancel(ctx)
	images, err := h.read().ListEmaBoardImages(ctx, in.ID)
	if err != nil {
		return nil, h.fail("list a board's plaque images", err)
	}
	n, err := h.write().DeleteEmaBoard(ctx, in.ID)
	if err != nil {
		return nil, h.fail("delete a plaque board", err)
	}
	if n == 0 {
		return nil, noSuchEmaBoard()
	}
	for _, img := range images {
		h.removePlaqueImage(img)
	}
	// The middleware tells the boards; the artist's queue lost this board's requests.
	ResourceChanged(h.Live, "ema-requests")
	return nil, nil
}

func (h handlers) listEmaPlaques(ctx context.Context, in *emaBoardIDInput) (*emaPlaqueListOutput, error) {
	q := h.read()
	if _, err := q.GetEmaBoard(ctx, in.ID); errors.Is(err, sql.ErrNoRows) {
		return nil, noSuchEmaBoard()
	} else if err != nil {
		return nil, h.fail("read a plaque board", err)
	}
	rows, err := q.ListEmaPlaques(ctx, in.ID)
	if err != nil {
		return nil, h.fail("list plaques", err)
	}
	out := EmaPlaqueList{Plaques: make([]EmaPlaque, 0, len(rows))}
	for _, r := range rows {
		out.Plaques = append(out.Plaques, toEmaPlaque((*db.GetEmaPlaqueRow)(r)))
	}
	return &emaPlaqueListOutput{Body: out}, nil
}

func (h handlers) listEmaRequests(ctx context.Context, _ *struct{}) (*emaPlaqueListOutput, error) {
	rows, err := h.read().ListEmaArtistRequests(ctx)
	if err != nil {
		return nil, h.fail("list artist requests", err)
	}
	out := EmaPlaqueList{Plaques: make([]EmaPlaque, 0, len(rows))}
	for _, r := range rows {
		out.Plaques = append(out.Plaques, toEmaPlaque((*db.GetEmaPlaqueRow)(r)))
	}
	return &emaPlaqueListOutput{Body: out}, nil
}

// reviewer names the caller for the record of who reviewed a plaque.
func reviewer(ctx context.Context) string {
	if u := Who(ctx); u != nil {
		return u.Username
	}
	return "unknown"
}

// reloadEmaPlaque answers a plaque's current row.
func (h handlers) reloadEmaPlaque(ctx context.Context, id int64) (EmaPlaque, error) {
	p, err := h.read().GetEmaPlaque(ctx, id)
	if err != nil {
		return EmaPlaque{}, h.fail("reload a plaque", err)
	}
	return toEmaPlaque(p), nil
}

// reviewablePlaque reads the site plaque of the board in the path that is waiting for
// review.
func (h handlers) reviewablePlaque(ctx context.Context, in *emaPlaqueIDInput) (*db.GetEmaPlaqueRow, error) {
	p, err := h.read().GetEmaPlaque(ctx, in.PlaqueID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (p.BoardID != in.ID || p.Kind != festival.EmaKindSite)) {
		return nil, noSuchPlaque()
	}
	if err != nil {
		return nil, h.fail("read a plaque", err)
	}
	if p.Status != festival.EmaPending {
		return nil, huma.Error409Conflict("This plaque has already been reviewed.")
	}
	return p, nil
}

// windowWebhook is the webhook of the day a plaque was made for, or "" when the day
// has none or has been deleted.
func windowWebhook(board EmaBoard, windowID *int64) string {
	for _, w := range board.Windows {
		if windowID != nil && w.ID == *windowID {
			return w.WebhookURL
		}
	}
	return ""
}

// postEmaToDiscord posts a plaque to a day's webhook: an embed carrying the picture as
// an attachment, sent through the shared sender so the dry-run and override valves
// apply.
func (h handlers) postEmaToDiscord(ctx context.Context, target discord.Target, webhook, description string, file discord.File) error {
	embed := discord.NewEmbed().Title("An Ema plaque").Description(description).Build()
	// Set directly: the builder takes web URLs only, and this names the upload.
	embed.Image = &discord.EmbedImage{URL: "attachment://" + file.Name}
	return h.Discord.PostFile(ctx, target, webhook, discord.Payload{Embeds: []discord.Embed{embed}}, file)
}

// emaPostProblem answers a failed post. Discord not answering is its own wording, since
// the plaque may have gone out; again says what not to do until the channel is checked.
func (h handlers) emaPostProblem(err error, what, again string) error {
	if discord.IsAmbiguous(err) {
		h.Logger.Error("ema post ambiguous", "what", what, "error", err)
		return huma.Error503ServiceUnavailable("Discord didn't answer, so the plaque may or may not have posted. Check the channel before " + again + ".")
	}
	return h.discordProblem(err, what)
}

func (h handlers) approveEmaPlaque(ctx context.Context, in *emaPlaqueIDInput) (*emaPlaqueOutput, error) {
	p, err := h.reviewablePlaque(ctx, in)
	if err != nil {
		return nil, err
	}
	board, err := h.loadEmaBoard(ctx, h.read(), p.BoardID)
	if err != nil {
		return nil, h.fail("read a board for an approval", err)
	}
	webhook := windowWebhook(board, intPtr(p.WindowID))
	if webhook == "" {
		return nil, huma.Error500InternalServerError("Set a webhook for " + p.WindowLabel + " before approving its plaques.")
	}
	data, err := h.readPlaqueImage(p.Image)
	if err != nil {
		return nil, h.fail("read a plaque's picture", err)
	}

	// From the claim on, the steps belong together: a client that leaves must not strand
	// the plaque in "posting", nor cut the Discord post off half-sent.
	ctx = context.WithoutCancel(ctx)
	w := h.write()
	claimed, err := w.MoveEmaPlaque(ctx, db.MoveEmaPlaqueParams{ToStatus: festival.EmaPosting, ID: p.ID, FromStatus: festival.EmaPending})
	if err != nil {
		return nil, h.fail("claim a plaque", err)
	}
	if claimed == 0 {
		return nil, huma.Error409Conflict("Someone else is reviewing this plaque.")
	}

	who := model.ParticipantLabel(p.CharacterName, p.World)
	postErr := h.postEmaToDiscord(ctx, discord.Target{Kind: "ema_plaque", Name: who}, webhook,
		"Hung by **"+who+"** - "+p.WindowLabel, discord.File{Name: "ema.png", ContentType: "image/png", Data: data})
	if postErr != nil {
		if _, err := w.MoveEmaPlaque(ctx, db.MoveEmaPlaqueParams{ToStatus: festival.EmaPending, ID: p.ID, FromStatus: festival.EmaPosting}); err != nil {
			h.Logger.Error("release a plaque after a failed post", "plaqueId", p.ID, "error", err)
		}
		return nil, h.emaPostProblem(postErr, "post an ema plaque", "approving it again")
	}

	by := reviewer(ctx)
	if _, err := w.FinishEmaPlaque(ctx, db.FinishEmaPlaqueParams{
		ToStatus: festival.EmaPosted, ReviewedBy: by, ID: p.ID, FromStatus: festival.EmaPosting,
	}); err != nil {
		// Posted, but not recorded as such: say so rather than invite a second post.
		h.Logger.Error("record a plaque as posted", "plaqueId", p.ID, "error", err)
		return nil, huma.Error500InternalServerError("The plaque was posted, but recording it failed. Don't approve it again.")
	}
	h.removePlaqueImage(p.Image)
	h.Logger.Info("ema plaque posted", "plaqueId", p.ID, "boardId", p.BoardID, "by", by)
	out, err := h.reloadEmaPlaque(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return &emaPlaqueOutput{Body: out}, nil
}

func (h handlers) rejectEmaPlaque(ctx context.Context, in *emaPlaqueIDInput) (*emaPlaqueOutput, error) {
	p, err := h.reviewablePlaque(ctx, in)
	if err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	done, err := h.write().FinishEmaPlaque(ctx, db.FinishEmaPlaqueParams{
		ToStatus: festival.EmaRejected, ReviewedBy: reviewer(ctx), ID: p.ID, FromStatus: festival.EmaPending,
	})
	if err != nil {
		return nil, h.fail("reject a plaque", err)
	}
	if done == 0 {
		return nil, huma.Error409Conflict("This plaque has already been reviewed.")
	}
	h.removePlaqueImage(p.Image)
	out, err := h.reloadEmaPlaque(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return &emaPlaqueOutput{Body: out}, nil
}

// artistRequest reads an artist request, or the 404 when there is none.
func (h handlers) artistRequest(ctx context.Context, id int64) (*db.GetEmaPlaqueRow, error) {
	p, err := h.read().GetEmaPlaque(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && p.Kind != festival.EmaKindArtist) {
		return nil, noSuchEmaRequest()
	}
	if err != nil {
		return nil, h.fail("read an artist request", err)
	}
	return p, nil
}

var errRequestChanged = huma.Error409Conflict("That request has already changed - reload the list.")

func (h handlers) completeEmaRequest(ctx context.Context, in *emaRequestIDInput) (*emaPlaqueOutput, error) {
	if _, err := h.artistRequest(ctx, in.ID); err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	n, err := h.write().CompleteEmaRequest(ctx, db.CompleteEmaRequestParams{ReviewedBy: reviewer(ctx), ID: in.ID})
	if err != nil {
		return nil, h.fail("complete an artist request", err)
	}
	if n == 0 {
		return nil, errRequestChanged
	}
	out, err := h.reloadEmaPlaque(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &emaPlaqueOutput{Body: out}, nil
}

func (h handlers) reopenEmaRequest(ctx context.Context, in *emaRequestIDInput) (*emaPlaqueOutput, error) {
	if _, err := h.artistRequest(ctx, in.ID); err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	n, err := h.write().ReopenEmaRequest(ctx, in.ID)
	if err != nil {
		return nil, h.fail("reopen an artist request", err)
	}
	if n == 0 {
		return nil, errRequestChanged
	}
	out, err := h.reloadEmaPlaque(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &emaPlaqueOutput{Body: out}, nil
}

// artistImageName names the artist's finished plaque in the Discord post by its
// sniffed type, or "" for anything that isn't an image Discord shows inline.
func artistImageName(contentType string) string {
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

func (h handlers) postEmaRequest(ctx context.Context, in *emaRequestPostInput) (*emaPlaqueOutput, error) {
	form := in.RawBody.Data()
	defer func() { _ = form.Image.Close() }() // read in full below; nothing to report on close

	p, err := h.artistRequest(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if p.Status != festival.EmaRequested {
		return nil, huma.Error409Conflict("This request is already finished - reopen it to post a plaque for it.")
	}
	board, err := h.loadEmaBoard(ctx, h.read(), p.BoardID)
	if err != nil {
		return nil, h.fail("read a board for a request", err)
	}
	webhook := windowWebhook(board, intPtr(p.WindowID))
	if webhook == "" {
		return nil, huma.Error500InternalServerError(p.WindowLabel + " has no webhook to post to - ask staff to set one.")
	}

	data, err := io.ReadAll(io.LimitReader(form.Image, maxEmaArtistImageBytes+1))
	if err != nil {
		return nil, h.fail("read the artist's plaque", err)
	}
	if len(data) > maxEmaArtistImageBytes {
		return nil, huma.Error413RequestEntityTooLarge("That image is too large - Discord takes up to 10 MB.")
	}
	// Judged from the bytes, not trusted from the browser's label, and saved again
	// without its metadata before it goes to Discord.
	checked, err := uploads.CheckImage(data, uploads.ImageRule{})
	switch {
	case errors.Is(err, uploads.ErrTooLarge):
		return nil, fieldProblem("body.image", nil, "That plaque image is too large.")
	case errors.Is(err, uploads.ErrNotImage):
		return nil, fieldProblem("body.image", nil, "The plaque must be a PNG, JPEG, GIF or WebP image.")
	case err != nil:
		return nil, h.fail("check the artist's plaque", err)
	}
	contentType, name := checked.Type, artistImageName(checked.Type)
	data = checked.Data

	// From the claim on, the steps belong together, as in an approval.
	ctx = context.WithoutCancel(ctx)
	w := h.write()
	claimed, err := w.MoveEmaPlaque(ctx, db.MoveEmaPlaqueParams{ToStatus: festival.EmaPosting, ID: p.ID, FromStatus: festival.EmaRequested})
	if err != nil {
		return nil, h.fail("claim an artist request", err)
	}
	if claimed == 0 {
		return nil, huma.Error409Conflict("This request is already being posted.")
	}

	who := model.ParticipantLabel(p.CharacterName, p.World)
	postErr := h.postEmaToDiscord(ctx, discord.Target{Kind: "ema_request", Name: who}, webhook,
		"Requested by **"+who+"**, made by **"+board.ArtistName+"** - "+p.WindowLabel,
		discord.File{Name: name, ContentType: contentType, Data: data})
	if postErr != nil {
		if _, err := w.MoveEmaPlaque(ctx, db.MoveEmaPlaqueParams{ToStatus: festival.EmaRequested, ID: p.ID, FromStatus: festival.EmaPosting}); err != nil {
			h.Logger.Error("release an artist request after a failed post", "plaqueId", p.ID, "error", err)
		}
		return nil, h.emaPostProblem(postErr, "post an ema request", "posting it again")
	}

	by := reviewer(ctx)
	if _, err := w.FinishEmaPlaque(ctx, db.FinishEmaPlaqueParams{
		ToStatus: festival.EmaComplete, ReviewedBy: by, ID: p.ID, FromStatus: festival.EmaPosting,
	}); err != nil {
		// Posted, but not recorded as such: say so rather than invite a second post.
		h.Logger.Error("record an artist request as complete", "plaqueId", p.ID, "error", err)
		return nil, huma.Error500InternalServerError("The plaque was posted, but recording it failed. Don't post it again.")
	}
	h.Logger.Info("ema request posted", "plaqueId", p.ID, "boardId", p.BoardID, "by", by)
	out, err := h.reloadEmaPlaque(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return &emaPlaqueOutput{Body: out}, nil
}

// -- Visitors ----------------------------------------------------------------------

// openEmaDay finds the open board of a festival for a public submission: the festival
// must be published and its board open and offering the way asked for, and a day must
// be taking plaques now.
func (h handlers) openEmaDay(ctx context.Context, idOrSlug string, allow func(EmaBoard) bool) (EmaBoard, EmaWindow, error) {
	q := h.read()
	m, err := h.publishedMap(ctx, q, idOrSlug)
	if err != nil {
		var problem huma.StatusError
		if errors.As(err, &problem) && problem.GetStatus() == http.StatusNotFound {
			return EmaBoard{}, EmaWindow{}, errNoPlaqueBoard
		}
		return EmaBoard{}, EmaWindow{}, err
	}
	b, err := h.findEmaBoard(ctx, q, func() (*db.GetEmaBoardRow, error) {
		r, err := q.GetEmaBoardForMap(ctx, m.ID)
		return (*db.GetEmaBoardRow)(r), err
	})
	if err != nil {
		return EmaBoard{}, EmaWindow{}, err
	}
	if b == nil || b.Status != "open" || !allow(*b) {
		return EmaBoard{}, EmaWindow{}, errNoPlaqueBoard
	}
	i := festival.CurrentDay(emaDays(b.Windows), time.Now().UTC())
	if i < 0 {
		return EmaBoard{}, EmaWindow{}, huma.Error409Conflict("Plaques aren't being taken right now - check the festival's days.")
	}
	return *b, b.Windows[i], nil
}

var errNoPlaqueBoard = huma.Error404NotFound("This festival isn't taking plaques.")

// emaDays are a board's days for finding the open one; one whose times don't read is
// never open.
func emaDays(windows []EmaWindow) []festival.Day {
	days := make([]festival.Day, 0, len(windows))
	for _, w := range windows {
		start, err1 := time.Parse(time.RFC3339, w.StartAt)
		end, err2 := time.Parse(time.RFC3339, w.EndAt)
		if err1 != nil || err2 != nil {
			start, end = time.Time{}, time.Time{}
		}
		days = append(days, festival.Day{ID: w.ID, Start: start, End: end})
	}
	return days
}

// emaVisitor validates the name and world a submission is made under.
func emaVisitor(name, world string) (string, string, error) {
	name, world = strings.TrimSpace(name), strings.TrimSpace(world)
	switch {
	case name == "":
		return "", "", fieldProblem("body.characterName", name, "Enter your character name.")
	case world == "":
		return "", "", fieldProblem("body.world", world, "Pick your home world.")
	case utf8.RuneCountInString(name) > maxParticipantNameLen:
		return "", "", fieldProblem("body.characterName", name, "That name is too long.")
	case utf8.RuneCountInString(world) > maxParticipantNameLen:
		return "", "", fieldProblem("body.world", world, "That world name is too long.")
	}
	return name, world, nil
}

// maxParticipantNameLen bounds a submitted name or world, counted in runes so a
// multi-byte name isn't cut short by a byte count.
const maxParticipantNameLen = 60

// botCheck is the 403 for a failed Turnstile check, or nil when it passes.
func (h handlers) botCheck(ctx context.Context, token, where string) error {
	ip := requestOf(ctx).IP
	if !h.turnstile(ctx, token, ip) {
		h.Logger.Warn("turnstile check failed", "ip", ip, "where", where)
		return huma.Error403Forbidden("Bot check failed. Please try again.")
	}
	return nil
}

// insertPlaque records a plaque, refusing a second one for the same person on the
// same day (409) and an artist request past its day's limit (409).
//
// The limit is checked after the insert, inside the same write-locked transaction:
// the lock keeps two requests for the last place from both counting the day as not
// full, and inserting first leaves the one-per-day refusal ahead of the full one for
// someone who already has a plaque.
func (h handlers) insertPlaque(ctx context.Context, p db.InsertEmaPlaqueParams, artistName string) (int64, error) {
	var id int64
	err := h.transact(ctx, func(q *db.Queries) error {
		var err error
		if id, err = q.InsertEmaPlaque(ctx, p); err != nil {
			return err
		}
		if p.Kind != festival.EmaKindArtist || !p.WindowID.Valid {
			return nil
		}
		load, err := q.GetEmaWindowLoad(ctx, p.WindowID.Int64)
		if err != nil {
			return err
		}
		if load.ArtistLimit > 0 && load.ArtistCount > load.ArtistLimit {
			return huma.Error409Conflict(artistName + "'s requests for " + p.WindowLabel + " are full.")
		}
		return nil
	})
	var problem huma.StatusError
	switch {
	case errors.As(err, &problem):
		return 0, problem
	case uniqueViolation(err):
		return 0, huma.Error409Conflict("You already have a plaque for " + p.WindowLabel + " - one per day.")
	case err != nil:
		return 0, h.fail("record a plaque", err)
	}
	return id, nil
}

func (h handlers) submitEmaPlaque(ctx context.Context, in *emaSubmitInput) (*publicEmaPlaqueOutput, error) {
	form := in.RawBody.Data()
	defer func() { _ = form.Image.Close() }() // read in full below; nothing to report on close

	if err := h.botCheck(ctx, form.TurnstileToken, "ema plaque"); err != nil {
		return nil, err
	}
	name, world, err := emaVisitor(form.CharacterName, form.World)
	if err != nil {
		return nil, err
	}
	board, win, err := h.openEmaDay(ctx, in.IDOrSlug, func(b EmaBoard) bool { return b.AllowSite })
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(io.LimitReader(form.Image, maxEmaImageBytes+1))
	if err != nil {
		return nil, h.fail("read a plaque", err)
	}
	if len(data) > maxEmaImageBytes {
		return nil, huma.Error413RequestEntityTooLarge("That plaque is too large to send.")
	}
	// A PNG of a sensible size, judged from its bytes and saved again without its
	// metadata, not by trusting the content type the browser sent.
	checked, err := uploads.CheckImage(data, uploads.ImageRule{Types: []string{"image/png"}, MaxSide: maxEmaImageSide})
	switch {
	case errors.Is(err, uploads.ErrTooLarge):
		return nil, fieldProblem("body.image", nil, "That plaque image is too large.")
	case errors.Is(err, uploads.ErrNotImage):
		return nil, fieldProblem("body.image", nil, "The plaque must be a PNG image.")
	case err != nil:
		return nil, h.fail("check a plaque", err)
	}
	data = checked.Data

	rel, err := h.saveEmaImage(board.ID, data)
	if err != nil {
		return nil, h.fail("save a plaque's picture", err)
	}
	// Detached: the file is on disk now, and its row must follow or the file go.
	ctx = context.WithoutCancel(ctx)
	id, err := h.insertPlaque(ctx, db.InsertEmaPlaqueParams{
		BoardID: board.ID, WindowID: sql.NullInt64{Int64: win.ID, Valid: true}, WindowLabel: win.Label,
		Kind: festival.EmaKindSite, CharacterName: name, World: world, Image: rel, Status: festival.EmaPending,
	}, board.ArtistName)
	if err != nil {
		h.removePlaqueImage(rel)
		return nil, err
	}
	p, err := h.read().GetEmaPlaque(ctx, id)
	if err != nil {
		return nil, h.fail("reload a plaque", err)
	}
	h.Logger.Info("ema plaque submitted", "plaqueId", id, "boardId", board.ID, "window", win.Label)
	// The write middleware leaves public paths out, so tell the review queue.
	ResourceChanged(h.Live, "ema-boards")
	return &publicEmaPlaqueOutput{Status: http.StatusCreated, Body: toPublicEmaPlaque(p)}, nil
}

func (h handlers) requestEmaPlaque(ctx context.Context, in *emaRequestSubmitInput) (*publicEmaPlaqueOutput, error) {
	if err := h.botCheck(ctx, in.Body.TurnstileToken, "ema request"); err != nil {
		return nil, err
	}
	name, world, err := emaVisitor(in.Body.CharacterName, in.Body.World)
	if err != nil {
		return nil, err
	}
	desc := strings.TrimSpace(in.Body.Description)
	if desc == "" {
		return nil, fieldProblem("body.description", desc, "Describe the plaque you'd like.")
	}
	board, win, err := h.openEmaDay(ctx, in.IDOrSlug, func(b EmaBoard) bool { return b.AllowArtist })
	if err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	id, err := h.insertPlaque(ctx, db.InsertEmaPlaqueParams{
		BoardID: board.ID, WindowID: sql.NullInt64{Int64: win.ID, Valid: true}, WindowLabel: win.Label,
		Kind: festival.EmaKindArtist, CharacterName: name, World: world, Description: desc, Status: festival.EmaRequested,
	}, board.ArtistName)
	if err != nil {
		return nil, err
	}
	p, err := h.read().GetEmaPlaque(ctx, id)
	if err != nil {
		return nil, h.fail("reload a request", err)
	}
	ResourceChanged(h.Live, "ema-requests")
	return &publicEmaPlaqueOutput{Status: http.StatusCreated, Body: toPublicEmaPlaque(p)}, nil
}

// buildFestivalEma builds a festival's open plaque board for a visitor: what it offers,
// its days (never their webhooks), the one taking plaques now, and the visitor's own
// plaques when a name is known. It is nil when the festival has no open board.
func (h handlers) buildFestivalEma(ctx context.Context, q *db.Queries, mapID int64, name, world string, now time.Time) (*FestivalEma, error) {
	b, err := h.findEmaBoard(ctx, q, func() (*db.GetEmaBoardRow, error) {
		r, err := q.GetEmaBoardForMap(ctx, mapID)
		return (*db.GetEmaBoardRow)(r), err
	})
	if err != nil || b == nil || b.Status != "open" {
		return nil, err
	}
	out := &FestivalEma{
		ID: b.ID, Title: b.Title, TemplateImage: b.TemplateImage, Instructions: b.Instructions, DiscordLink: b.DiscordLink,
		AllowSelf: b.AllowSelf, AllowSite: b.AllowSite, AllowArtist: b.AllowArtist, ArtistName: b.ArtistName,
		Windows: make([]PublicEmaWindow, 0, len(b.Windows)), Plaques: []PublicEmaPlaque{},
	}
	var current int64
	if i := festival.CurrentDay(emaDays(b.Windows), now); i >= 0 {
		current = b.Windows[i].ID
		out.CurrentWindowID = &current
	}
	for _, w := range b.Windows {
		pw := PublicEmaWindow{
			ID: w.ID, Label: w.Label, StartAt: w.StartAt, EndAt: w.EndAt, IsOpen: current != 0 && current == w.ID,
			ArtistLimit: w.ArtistLimit,
		}
		if w.ArtistLimit > 0 {
			pw.ArtistLeft = max(0, w.ArtistLimit-w.ArtistCount)
		}
		out.Windows = append(out.Windows, pw)
	}
	if name == "" {
		return out, nil
	}
	mine, err := q.ListEmaPlaquesFor(ctx, db.ListEmaPlaquesForParams{BoardID: b.ID, Name: name, World: world})
	if err != nil {
		return nil, err
	}
	for _, p := range mine {
		pp := toPublicEmaPlaque((*db.GetEmaPlaqueRow)(p))
		// The visitor's own upload is shown back to them while it waits; the file is gone
		// once it is posted or rejected.
		if p.Status != festival.EmaPending && p.Status != festival.EmaPosting {
			pp.Image = ""
		}
		out.Plaques = append(out.Plaques, pp)
		if current != 0 && p.WindowID.Valid && p.WindowID.Int64 == current && p.Status != festival.EmaRejected {
			out.HasPlaqueThisWindow = true
		}
	}
	return out, nil
}

// -- Files ---------------------------------------------------------------------------

// saveEmaImage writes a plaque's PNG under emaImageRelDir with an unguessable name,
// returning its web-root-relative path.
func (h handlers) saveEmaImage(boardID int64, data []byte) (string, error) {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	dir := filepath.Join(h.WebRoot, filepath.FromSlash(emaImageRelDir))
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // the web server reads the pictures back from here
		return "", err
	}
	name := fmt.Sprintf("%d-%s.png", boardID, hex.EncodeToString(token))
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil { //nolint:gosec // served-adjacent upload, read back by the server
		return "", err
	}
	return path.Join(emaImageRelDir, name), nil
}

// emaImagePath resolves a stored plaque image to its file, or "" when the value isn't
// one of ours, so nothing outside the plaque folder can be read or removed.
func (h handlers) emaImagePath(rel string) string {
	base := path.Base(rel)
	if rel != path.Join(emaImageRelDir, base) || base == "." || strings.HasPrefix(base, ".") || strings.ContainsRune(base, '\\') {
		return ""
	}
	return filepath.Join(h.WebRoot, filepath.FromSlash(emaImageRelDir), base)
}

func (h handlers) readPlaqueImage(rel string) ([]byte, error) {
	p := h.emaImagePath(rel)
	if p == "" {
		return nil, fmt.Errorf("not a plaque image: %q", rel)
	}
	return os.ReadFile(p) //nolint:gosec // emaImagePath only resolves inside the plaque folder
}

// removePlaqueImage deletes a plaque's file. Best effort: a file already gone is fine,
// and any other failure is logged, the row being what records the plaque.
func (h handlers) removePlaqueImage(rel string) {
	p := h.emaImagePath(rel)
	if p == "" {
		return
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		h.Logger.Warn("remove a plaque picture", "image", rel, "error", err)
	}
}
