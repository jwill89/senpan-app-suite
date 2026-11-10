package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/db"
	"app-suite/internal/festival"
	"app-suite/internal/model"
	"app-suite/internal/permissions"
	"app-suite/internal/random"
	"app-suite/internal/store"
)

// Stamp rallies: events whose stamps participants collect on a card of their own by
// entering a stall's password. Staff author the rally and issue cards; anyone may
// sign up for a rally opened to it, and a card's token is the only thing gating the
// card.

var stampRallyTags = []string{"Stamp Rallies"}

// Placement positions an item on a stamp card.
type Placement struct {
	X        float64 `json:"x" doc:"The distance from the card's left edge, in percent of its width. A save clamps it to 0..100."`
	Y        float64 `json:"y" doc:"The distance from the card's top edge, in percent of its height. A save clamps it to 0..100."`
	Width    float64 `json:"width" doc:"The item's width in percent of the card's. A save clamps it to 0..100 and gives 10 to one with none."`
	Height   float64 `json:"height" doc:"The item's height in percent of the card's. A save clamps it to 0..100 and gives 10 to one with none."`
	Rotation float64 `json:"rotation" doc:"The item's turn in degrees."`
}

func (p Placement) sanitized() Placement {
	s := festival.SanitizePlacement(festival.Placement{X: p.X, Y: p.Y, Width: p.Width, Height: p.Height, Rotation: p.Rotation})
	return Placement{X: s.X, Y: s.Y, Width: s.Width, Height: s.Height, Rotation: s.Rotation}
}

// StampRally is a stamp rally as the staff see it.
type StampRally struct {
	ID                 int64             `json:"id" doc:"The rally's id."`
	Title              string            `json:"title" doc:"The rally's title."`
	CardImage          string            `json:"cardImage" doc:"The card's background picture: a path from the image library."`
	NotStampedImage    string            `json:"notStampedImage" doc:"The picture shown where a stamp isn't collected and where a prize isn't won yet."`
	RedeemImage        string            `json:"redeemImage" doc:"A screenshot of where to redeem a finished card, shown once it is complete."`
	AvailableFrom      *string           `json:"availableFrom" nullable:"true" doc:"When it opens, RFC 3339 in UTC; null for no start."`
	AvailableTo        *string           `json:"availableTo" nullable:"true" doc:"When it ends, RFC 3339 in UTC; null for no end."`
	Details            string            `json:"details" doc:"What it is about (Markdown)."`
	RedeemInstructions string            `json:"redeemInstructions" doc:"How to redeem a finished card (Markdown), shown once it is complete."`
	Status             string            `json:"status" enum:"open,closed" doc:"A closed rally takes no more stamps and no sign-ups."`
	PublicSignup       bool              `json:"publicSignup" doc:"Whether anyone may sign up for a card; off, only staff issue cards."`
	CompletionMode     string            `json:"completionMode" enum:"all,counts" doc:"all: a card is complete when every stamp is collected or can't be any more. counts: when it holds requiredFood food stamps and requiredGame game stamps."`
	RequiredFood       int64             `json:"requiredFood" doc:"The food stamps a card needs in counts mode."`
	RequiredGame       int64             `json:"requiredGame" doc:"The game stamps a card needs in counts mode."`
	FestivalMapID      *int64            `json:"festivalMapId" nullable:"true" doc:"The festival it is filed under; null for none."`
	FestivalMapName    string            `json:"festivalMapName" doc:"The festival's title; empty for none."`
	CreatedAt          *string           `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
	Stamps             []StampRallyStamp `json:"stamps" nullable:"false" doc:"Its stamps in order, with their passwords. Empty in a list."`
	Prizes             []StampRallyPrize `json:"prizes" nullable:"false" doc:"Its prizes in order. Empty in a list."`
	CardCount          int64             `json:"cardCount" doc:"The cards issued."`
	CompletedCount     int64             `json:"completedCount" doc:"The cards complete."`
	StampCount         int64             `json:"stampCount" doc:"The stamps it has."`
	ActiveStampCount   int64             `json:"activeStampCount" doc:"The stamps not paused."`
}

// StampRallyStamp is one collectable stamp.
type StampRallyStamp struct {
	ID            int64     `json:"id" doc:"The stamp's id."`
	OccupantID    *int64    `json:"occupantId" nullable:"true" doc:"The stall of the rally's festival that holds it; null when it isn't tied to one."`
	StallName     string    `json:"stallName" doc:"The title of that stall; empty when none."`
	AffiliateID   *int64    `json:"affiliateId" nullable:"true" doc:"The affiliate that runs the stall; null for the Tea House itself. A stamp on a stall takes it from the stall."`
	AffiliateName string    `json:"affiliateName" doc:"The affiliate's name; empty for none."`
	Image         string    `json:"image" doc:"The stamp's picture: a path from the image library."`
	Password      string    `json:"password" doc:"The word a participant enters to collect it, read out at the stall. Staff only."`
	StampType     string    `json:"stampType" enum:"food,game" doc:"The kind of stall: what a counts rally counts."`
	Placement     Placement `json:"placement" doc:"Where it sits on the card."`
	ActiveFrom    *string   `json:"activeFrom" nullable:"true" doc:"When it can first be collected, RFC 3339 in UTC; null for the whole rally."`
	ActiveTo      *string   `json:"activeTo" nullable:"true" doc:"When it can last be collected, RFC 3339 in UTC; null for the whole rally."`
	Paused        bool      `json:"paused" doc:"Whether staff have paused it: it can't be collected."`
	SortOrder     int64     `json:"sortOrder" doc:"Its place in the rally's stamps."`
}

// StampRallyPrize is a prize a finished card reveals.
type StampRallyPrize struct {
	ID        int64     `json:"id" doc:"The prize's id."`
	Name      string    `json:"name" doc:"The prize."`
	Image     string    `json:"image" doc:"Its picture: a path from the image library."`
	Placement Placement `json:"placement" doc:"Where it sits on the card."`
	SortOrder int64     `json:"sortOrder" doc:"Its place in the rally's prizes."`
}

// StampRallyInput is a rally as a save sends it: all of its editable fields, since a
// save replaces them. Its status changes through close and reopen.
type StampRallyInput struct {
	Title              string                 `json:"title" minLength:"1" maxLength:"200" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The rally's title."`
	CardImage          string                 `json:"cardImage" maxLength:"500" doc:"The card's background picture: a path from the image library."`
	NotStampedImage    string                 `json:"notStampedImage" maxLength:"500" doc:"The picture shown where a stamp isn't collected and where a prize isn't won yet."`
	RedeemImage        string                 `json:"redeemImage" maxLength:"500" doc:"A screenshot of where to redeem a finished card."`
	AvailableFrom      *string                `json:"availableFrom,omitempty" nullable:"true" format:"date-time" doc:"When it opens, RFC 3339; null or left out for no start."`
	AvailableTo        *string                `json:"availableTo,omitempty" nullable:"true" format:"date-time" doc:"When it ends, RFC 3339; null or left out for no end."`
	Details            string                 `json:"details" maxLength:"20000" doc:"What it is about (Markdown)."`
	RedeemInstructions string                 `json:"redeemInstructions" maxLength:"20000" doc:"How to redeem a finished card (Markdown)."`
	PublicSignup       bool                   `json:"publicSignup" doc:"Whether anyone may sign up for a card."`
	CompletionMode     string                 `json:"completionMode" enum:"all,counts" doc:"all: every stamp collected or out of reach. counts: requiredFood food stamps and requiredGame game stamps."`
	RequiredFood       int64                  `json:"requiredFood" minimum:"0" doc:"The food stamps a card needs in counts mode, kept to the number of food stamps."`
	RequiredGame       int64                  `json:"requiredGame" minimum:"0" doc:"The game stamps a card needs in counts mode, kept to the number of game stamps."`
	FestivalMapID      *int64                 `json:"festivalMapId,omitempty" nullable:"true" minimum:"1" doc:"The festival to file it under; null or left out for none. One that doesn't exist is refused."`
	Stamps             []StampRallyStampInput `json:"stamps" maxItems:"200" nullable:"false" doc:"The rally's whole set of stamps: a stamp with an id keeps that stamp and what was collected on it, id 0 adds one, and one left out is deleted. An empty list clears them."`
	Prizes             []StampRallyPrizeInput `json:"prizes" maxItems:"100" nullable:"false" doc:"The rally's whole set of prizes, replacing the old. An empty list clears them."`
}

// StampRallyStampInput is a stamp as a save sends it.
type StampRallyStampInput struct {
	ID          int64     `json:"id" minimum:"0" doc:"The stamp's id; 0 adds a new one."`
	OccupantID  *int64    `json:"occupantId,omitempty" nullable:"true" minimum:"1" doc:"The stall of the rally's festival that holds it. With no festival it is cleared; one that isn't on the festival is dropped, and the stamp falls back to its affiliate."`
	AffiliateID *int64    `json:"affiliateId,omitempty" nullable:"true" minimum:"1" doc:"The affiliate that runs the stall; null for the Tea House itself. A stamp on a festival stall takes it from the stall."`
	Image       string    `json:"image" maxLength:"500" doc:"The stamp's picture: a path from the image library."`
	Password    string    `json:"password" maxLength:"200" doc:"The word a participant enters to collect it. An empty one can't be matched."`
	StampType   string    `json:"stampType" enum:"food,game" doc:"The kind of stall."`
	Placement   Placement `json:"placement" doc:"Where it sits on the card; clamped into the card."`
	ActiveFrom  *string   `json:"activeFrom,omitempty" nullable:"true" format:"date-time" doc:"When it can first be collected, RFC 3339; null or left out for the whole rally."`
	ActiveTo    *string   `json:"activeTo,omitempty" nullable:"true" format:"date-time" doc:"When it can last be collected, RFC 3339; null or left out for the whole rally."`
	Paused      bool      `json:"paused" doc:"Whether it is paused."`
}

// StampRallyPrizeInput is a prize as a save sends it.
type StampRallyPrizeInput struct {
	Name      string    `json:"name" maxLength:"200" doc:"The prize."`
	Image     string    `json:"image" maxLength:"500" doc:"Its picture: a path from the image library."`
	Placement Placement `json:"placement" doc:"Where it sits on the card; clamped into the card."`
}

// StampRallyList is every rally.
type StampRallyList struct {
	StampRallies []StampRally `json:"stampRallies" nullable:"false" doc:"Newest first, with their counts and without stamps or prizes."`
}

// StampRallyDetail is a rally with its cards.
type StampRallyDetail struct {
	StampRally StampRally       `json:"stampRally" doc:"The rally, with its stamps and prizes."`
	Cards      []StampRallyCard `json:"cards" nullable:"false" doc:"The cards issued, newest first."`
}

// StampRallyCard is a participant's card as the staff see it.
type StampRallyCard struct {
	ID              int64   `json:"id" doc:"The card's id."`
	Token           string  `json:"token" doc:"The token of the card's link. Staff only."`
	ParticipantName string  `json:"participantName" doc:"The character."`
	World           string  `json:"world" doc:"Their home world; empty when it isn't known."`
	Completed       bool    `json:"completed" doc:"Whether the card is complete."`
	CompletedAt     *string `json:"completedAt" nullable:"true" doc:"When it was found complete, RFC 3339 in UTC."`
	CreatedAt       *string `json:"createdAt" nullable:"true" doc:"When it was issued, RFC 3339 in UTC."`
	CollectedCount  int64   `json:"collectedCount" doc:"The stamps collected on it."`
}

// StampRallyCardInput is a card staff issue.
type StampRallyCardInput struct {
	ParticipantName string `json:"participantName" minLength:"1" maxLength:"60" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The character. A name written as Name @ World with no world sent is split."`
	World           string `json:"world,omitempty" maxLength:"60" doc:"Their home world; empty when staff don't know it."`
}

// StampRallyStampChanges is what a stamp's patch sends.
type StampRallyStampChanges struct {
	Paused bool `json:"paused" doc:"Whether the stamp is paused: a paused stamp can't be collected."`
}

// StampRallyLogEntry is one stamp collected.
type StampRallyLogEntry struct {
	CardID          *int64  `json:"cardId" nullable:"true" doc:"The card that collected it; null once the card is deleted."`
	ParticipantName string  `json:"participantName" doc:"The participant as Character @ World, as they were."`
	StampID         *int64  `json:"stampId" nullable:"true" doc:"The stamp; null once it is deleted."`
	StallName       string  `json:"stallName" doc:"The stall, as it was named."`
	StampType       string  `json:"stampType" enum:"food,game" doc:"The kind of stall."`
	StampedAt       *string `json:"stampedAt" nullable:"true" doc:"When it was collected, RFC 3339 in UTC."`
}

// StampRallyLogList is a rally's log.
type StampRallyLogList struct {
	Logs []StampRallyLogEntry `json:"logs" nullable:"false" doc:"Every collection, a participant's together."`
}

// SignupRally is a rally anyone may sign up for.
type SignupRally struct {
	ID            int64   `json:"id" doc:"The rally's id."`
	Title         string  `json:"title" doc:"The rally's title."`
	CardImage     string  `json:"cardImage" doc:"The card's background picture."`
	Details       string  `json:"details" doc:"What it is about (Markdown)."`
	AvailableFrom *string `json:"availableFrom" nullable:"true" doc:"When it opens, RFC 3339 in UTC."`
	AvailableTo   *string `json:"availableTo" nullable:"true" doc:"When it ends, RFC 3339 in UTC."`
	GaraponTitle  string  `json:"garaponTitle" doc:"The title of the open garapon linked to it, whose drawing link a sign-up issues too; empty for none."`
}

// SignupRallyList is the rallies open to sign-up.
type SignupRallyList struct {
	Rallies []SignupRally `json:"rallies" nullable:"false" doc:"The open rallies that take sign-ups, inside their window, newest first."`
}

// StampSignupInput is a sign-up for a rally.
type StampSignupInput struct {
	CharacterName  string `json:"characterName" minLength:"1" maxLength:"60" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The character signing up."`
	World          string `json:"world" minLength:"1" maxLength:"60" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"Their home world."`
	TurnstileToken string `json:"turnstileToken,omitempty" maxLength:"2048" doc:"The Cloudflare Turnstile response, when the check is on (GET /config gives its site key)."`
}

// StampSignup is what a sign-up issues.
type StampSignup struct {
	ParticipantName string  `json:"participantName" doc:"The character."`
	World           string  `json:"world" doc:"Their home world."`
	RallyTitle      string  `json:"rallyTitle" doc:"The rally."`
	CardToken       string  `json:"cardToken" doc:"The token of the stamp card's link. It can be shared: it opens the card and nothing else."`
	GaraponToken    *string `json:"garaponToken" nullable:"true" doc:"The token of the drawing link, only when the rally has an open garapon. A secret apart from the card's: whoever holds it can spend the draws. This answer is the one time it is given, so it is kept on the participant's device."`
	GaraponTitle    *string `json:"garaponTitle" nullable:"true" doc:"The garapon's title, when a drawing link was issued."`
}

// StampLookupInput asks for the cards held under a name.
type StampLookupInput struct {
	CharacterName string `json:"characterName" minLength:"1" maxLength:"120" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The character, as they signed up. A name written as Name @ World with no world sent is split."`
	World         string `json:"world,omitempty" maxLength:"60" doc:"Their home world."`
}

// StampLookupEntry is a card held under the name searched.
type StampLookupEntry struct {
	RallyID          int64  `json:"rallyId" doc:"The rally."`
	RallyTitle       string `json:"rallyTitle" doc:"The rally's title."`
	CardToken        string `json:"cardToken" doc:"The token of the card's link."`
	ParticipantName  string `json:"participantName" doc:"The name the card is held under, which may be spelled differently from the search."`
	World            string `json:"world" doc:"The world it is held under; empty when none was known."`
	Completed        bool   `json:"completed" doc:"Whether the card is complete."`
	GaraponTitle     string `json:"garaponTitle" doc:"The open garapon whose drawing link was issued with the card; empty for none."`
	GaraponDrawsLeft *int64 `json:"garaponDrawsLeft" nullable:"true" doc:"The draws left on that link; null when there is none. The link's token is never given: a name is public and a draw can't be undone."`
}

// StampLookup is the answer to a lookup.
type StampLookup struct {
	Entries []StampLookupEntry `json:"entries" nullable:"false" doc:"The cards held under the whole name and world, ignoring case, in open rallies. Empty when none, so a lookup can't be used to find out who signed up."`
}

// PublicStampRally is a rally as a participant sees it.
type PublicStampRally struct {
	ID                 int64   `json:"id" doc:"The rally's id."`
	Title              string  `json:"title" doc:"The rally's title."`
	CardImage          string  `json:"cardImage" doc:"The card's background picture."`
	NotStampedImage    string  `json:"notStampedImage" doc:"The picture shown where a stamp isn't collected and where a prize isn't won yet."`
	Details            string  `json:"details" doc:"What it is about (Markdown)."`
	RedeemInstructions string  `json:"redeemInstructions" doc:"How to redeem a finished card (Markdown)."`
	RedeemImage        string  `json:"redeemImage" doc:"A screenshot of where to redeem."`
	AvailableFrom      *string `json:"availableFrom" nullable:"true" doc:"When it opens, RFC 3339 in UTC."`
	AvailableTo        *string `json:"availableTo" nullable:"true" doc:"When it ends, RFC 3339 in UTC."`
	IsActive           bool    `json:"isActive" doc:"Whether it takes stamps now: open and inside its window."`
	CompletionMode     string  `json:"completionMode" enum:"all,counts" doc:"What finishes a card."`
	RequiredFood       int64   `json:"requiredFood" doc:"The food stamps a counts card needs."`
	RequiredGame       int64   `json:"requiredGame" doc:"The game stamps a counts card needs."`
	FestivalLink       string  `json:"festivalLink" doc:"The shortcode or id of the published festival it is filed under, which opens that festival's page; empty for none."`
}

// PublicStamp is a stamp on a participant's card.
type PublicStamp struct {
	ID            int64     `json:"id" doc:"The stamp's id."`
	AffiliateName string    `json:"affiliateName" doc:"The stall: its title on a festival plan, else its affiliate, else the Tea House."`
	OccupantID    *int64    `json:"occupantId" nullable:"true" doc:"The festival stall that holds it, so the plan can mark the ones collected."`
	StampType     string    `json:"stampType" enum:"food,game" doc:"The kind of stall."`
	Image         string    `json:"image" doc:"The stamp's picture."`
	Placement     Placement `json:"placement" doc:"Where it sits on the card."`
	ActiveFrom    *string   `json:"activeFrom" nullable:"true" doc:"When it can first be collected, RFC 3339 in UTC."`
	ActiveTo      *string   `json:"activeTo" nullable:"true" doc:"When it can last be collected, RFC 3339 in UTC."`
	Available     bool      `json:"available" doc:"Whether it can be collected now."`
	Expired       bool      `json:"expired" doc:"Whether it can never be collected again, as opposed to closed for now."`
	Collected     bool      `json:"collected" doc:"Whether this card holds it."`
	CollectedAt   *string   `json:"collectedAt" nullable:"true" doc:"When it was collected, RFC 3339 in UTC."`
}

// PublicPrize is a prize slot on a participant's card.
type PublicPrize struct {
	ID        int64     `json:"id" doc:"The prize's id."`
	Name      string    `json:"name" doc:"The prize; empty until the card is complete."`
	Image     string    `json:"image" doc:"Its picture; empty until the card is complete."`
	Placement Placement `json:"placement" doc:"Where it sits on the card."`
}

// PublicStampCard is a participant's card.
type PublicStampCard struct {
	Rally           PublicStampRally `json:"rally" doc:"The rally."`
	ParticipantName string           `json:"participantName" doc:"The participant as Character @ World."`
	Completed       bool             `json:"completed" doc:"Whether the card is complete."`
	CompletedAt     *string          `json:"completedAt" nullable:"true" doc:"When it was found complete, RFC 3339 in UTC."`
	Stamps          []PublicStamp    `json:"stamps" nullable:"false" doc:"The rally's stamps, without passwords."`
	Prizes          []PublicPrize    `json:"prizes" nullable:"false" doc:"The prize slots; each shows its prize only once the card is complete."`
	PrizesRevealed  bool             `json:"prizesRevealed" doc:"Whether the prizes are shown: the card is complete."`
}

// StampSubmitInput is a password entered at a stall.
type StampSubmitInput struct {
	Password string `json:"password" minLength:"1" maxLength:"200" pattern:"\\S" patternDescription:"more than spaces" doc:"The word read out at the stall."`
}

// StampResult is the card after a stamp was collected.
type StampResult struct {
	Card             PublicStampCard `json:"card" doc:"The refreshed card."`
	CollectedStampID int64           `json:"collectedStampId" doc:"The stamp just collected."`
}

type rallyIDInput struct {
	ID int64 `path:"id" doc:"The rally's id."`
}

type rallySaveInput struct {
	ID   int64 `path:"id" doc:"The rally's id."`
	Body StampRallyInput
}

type rallyCreateInput struct{ Body StampRallyInput }

type rallyStampInput struct {
	ID      int64 `path:"id" doc:"The rally's id."`
	StampID int64 `path:"stampId" doc:"The stamp's id."`
	Body    StampRallyStampChanges
}

type rallyCardInput struct {
	ID   int64 `path:"id" doc:"The rally's id."`
	Body StampRallyCardInput
}

type rallyCardIDInput struct {
	ID     int64 `path:"id" doc:"The rally's id."`
	CardID int64 `path:"cardId" doc:"The card's id."`
}

type rallySignupInput struct {
	ID   int64 `path:"id" doc:"The rally's id."`
	Body StampSignupInput
}

type rallyLookupInput struct{ Body StampLookupInput }

type stampCardStampInput struct {
	Token string `path:"token" maxLength:"128" doc:"The card's token."`
	Body  StampSubmitInput
}

type rallyOutput struct{ Body StampRally }

type rallyCreatedOutput struct {
	Status int
	Body   StampRally
}

type rallyListOutput struct{ Body StampRallyList }

type rallyDetailOutput struct{ Body StampRallyDetail }

type rallyStampOutput struct{ Body StampRallyStamp }

type rallyCardCreatedOutput struct {
	Status int
	Body   StampRallyCard
}

type rallyLogOutput struct{ Body StampRallyLogList }

type signupListOutput struct{ Body SignupRallyList }

type signupOutput struct {
	Status int
	Body   StampSignup
}

type lookupOutput struct{ Body StampLookup }

type stampCardOutput struct{ Body PublicStampCard }

type stampResultOutput struct{ Body StampResult }

func (h handlers) registerStampRallies(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.FestivalStampRally)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}
	notFound := slices.Concat(guarded, []int{http.StatusNotFound})

	huma.Register(api, huma.Operation{
		OperationID: "list-stamp-rallies", Method: http.MethodGet, Path: "/stamp-rallies",
		Summary: "Every stamp rally", Tags: stampRallyTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.listStampRallies)
	huma.Register(api, huma.Operation{
		OperationID: "list-open-stamp-rallies", Method: http.MethodGet, Path: "/stamp-rallies/public",
		Summary: "The rallies anyone may sign up for",
		Description: "Open rallies that took up public sign-up, inside their window. Nothing about their cards or " +
			"participants.",
		Tags: stampRallyTags, Middlewares: huma.Middlewares{public},
	}, h.listOpenStampRallies)
	huma.Register(api, huma.Operation{
		OperationID: "create-stamp-rally", Method: http.MethodPost, Path: "/stamp-rallies",
		Summary: "Make a stamp rally", Description: "It starts open, with the stamps and prizes sent.", Tags: stampRallyTags,
		DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: slices.Concat(guarded, []int{http.StatusUnprocessableEntity}),
	}, h.createStampRally)
	huma.Register(api, huma.Operation{
		OperationID: "get-stamp-rally", Method: http.MethodGet, Path: "/stamp-rallies/{id}",
		Summary: "One rally with its stamps, prizes and cards", Tags: stampRallyTags,
		Security: secured, Middlewares: page, Errors: notFound,
	}, h.getStampRally)
	huma.Register(api, huma.Operation{
		OperationID: "update-stamp-rally", Method: http.MethodPut, Path: "/stamp-rallies/{id}",
		Summary: "Replace a rally's editable fields, stamps and prizes",
		Description: "Its status is kept. A stamp that keeps its id keeps what was collected on it; one left out is " +
			"deleted, its log rows staying. A stamp tied to a stall of the rally's festival takes its affiliate from " +
			"that stall.",
		Tags: stampRallyTags, Security: secured, Middlewares: page,
		Errors: slices.Concat(notFound, []int{http.StatusUnprocessableEntity}),
	}, h.updateStampRally)
	huma.Register(api, huma.Operation{
		OperationID: "delete-stamp-rally", Method: http.MethodDelete, Path: "/stamp-rallies/{id}",
		Summary: "Delete a rally", Description: "Its stamps, prizes, cards and log go with it; a garapon that served it is unlinked.",
		Tags: stampRallyTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page, Errors: notFound,
	}, h.deleteStampRally)
	huma.Register(api, huma.Operation{
		OperationID: "close-stamp-rally", Method: http.MethodPost, Path: "/stamp-rallies/{id}/close",
		Summary: "Close a rally", Description: "It takes no more stamps or sign-ups, and any card can be deleted.",
		Tags: stampRallyTags, Security: secured, Middlewares: page, Errors: notFound,
	}, h.closeStampRally)
	huma.Register(api, huma.Operation{
		OperationID: "reopen-stamp-rally", Method: http.MethodPost, Path: "/stamp-rallies/{id}/reopen",
		Summary: "Open a closed rally again", Tags: stampRallyTags, Security: secured, Middlewares: page, Errors: notFound,
	}, h.reopenStampRally)
	huma.Register(api, huma.Operation{
		OperationID: "list-stamp-rally-logs", Method: http.MethodGet, Path: "/stamp-rallies/{id}/logs",
		Summary: "Every stamp collected in a rally", Tags: stampRallyTags, Security: secured, Middlewares: page, Errors: notFound,
	}, h.listStampRallyLogs)
	huma.Register(api, huma.Operation{
		OperationID: "update-stamp-rally-stamp", Method: http.MethodPatch, Path: "/stamp-rallies/{id}/stamps/{stampId}",
		Summary: "Pause or resume a stamp", Description: "Without saving the whole rally.", Tags: stampRallyTags,
		Security: secured, Middlewares: page, Errors: notFound,
	}, h.updateStampRallyStamp)
	huma.Register(api, huma.Operation{
		OperationID: "create-stamp-rally-card", Method: http.MethodPost, Path: "/stamp-rallies/{id}/cards",
		Summary: "Issue a card", Description: "Staff may issue a card to a name already holding one.", Tags: stampRallyTags,
		DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: slices.Concat(notFound, []int{http.StatusUnprocessableEntity}),
	}, h.createStampRallyCard)
	huma.Register(api, huma.Operation{
		OperationID: "delete-stamp-rally-card", Method: http.MethodDelete, Path: "/stamp-rallies/{id}/cards/{cardId}",
		Summary: "Delete a card",
		Description: "A card that has collected stamps can't be deleted while the rally is open (409); once it is closed any " +
			"card can, and the stamp log is kept.",
		Tags: stampRallyTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page,
		Errors: slices.Concat(notFound, []int{http.StatusConflict}),
	}, h.deleteStampRallyCard)
	huma.Register(api, huma.Operation{
		OperationID: "sign-up-for-stamp-rally", Method: http.MethodPost, Path: "/stamp-rallies/{id}/sign-up",
		Summary: "Sign up for a card",
		Description: "Anyone, for a rally that took up public sign-up. A rally that doesn't exist and one that didn't are " +
			"both 404, so it can't be used to find rallies staff hand cards out for privately. A rally with an open " +
			"garapon also issues a drawing link, its token given only in this answer. One card per character and " +
			"world (409). Fifteen tries from one address in ten minutes, sign-ups and lookups together, made or not " +
			"(429, with Retry-After).",
		Tags: stampRallyTags, DefaultStatus: http.StatusCreated,
		Errors: []int{http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity,
			http.StatusTooManyRequests},
	}, h.signUpForStampRally)
	huma.Register(api, huma.Operation{
		OperationID: "look-up-stamp-cards", Method: http.MethodPost, Path: "/stamp-cards/lookup",
		Summary: "Find my cards",
		Description: "For someone who lost their link: the cards held under the whole name and world, ignoring case, in " +
			"open rallies. A miss is an empty list, never a 404. It gives the card's token and the draws left on its " +
			"drawing link, never the drawing token. Fifteen tries from one address in ten minutes, with the sign-ups " +
			"(429, with Retry-After).",
		Tags: stampRallyTags, Errors: []int{http.StatusUnprocessableEntity, http.StatusTooManyRequests},
	}, h.lookUpStampCards)
	huma.Register(api, huma.Operation{
		OperationID: "get-stamp-card", Method: http.MethodGet, Path: "/stamp-cards/{token}",
		Summary: "A participant's card",
		Description: "Anyone holding the token. Without the stamps' passwords, and with each prize shown only once the card " +
			"is complete. A card found complete here is recorded as complete.",
		Tags: stampRallyTags, Errors: []int{http.StatusNotFound},
	}, h.getStampCard)
	huma.Register(api, huma.Operation{
		OperationID: "stamp-stamp-card", Method: http.MethodPost, Path: "/stamp-cards/{token}/stamp",
		Summary: "Collect a stamp with its password",
		Description: "The stamp whose password matches. A stall that is closed, paused, outside its hours or in a closed rally, " +
			"or a stamp already collected, is a 409. A password matching no stamp is a 422 and counts against ten " +
			"wrong passwords from one address in ten minutes (429, with Retry-After), checked first.",
		Tags: stampRallyTags, Errors: []int{http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity,
			http.StatusTooManyRequests},
	}, h.stampStampCard)
}

func noSuchRally() error { return huma.Error404NotFound("There is no such stamp rally.") }

func noSuchStampCard() error { return huma.Error404NotFound("There is no such stamp card.") }

func toStampRally(r *db.GetStampRallyRow) StampRally {
	return StampRally{
		ID: r.ID, Title: r.Title, CardImage: r.CardImage, NotStampedImage: r.NotStampedImage, RedeemImage: r.RedeemImage,
		AvailableFrom: r.AvailableFrom, AvailableTo: r.AvailableTo, Details: r.Details,
		RedeemInstructions: r.RedeemInstructions, Status: r.Status, PublicSignup: r.PublicSignup != 0,
		CompletionMode: model.NormalizeRallyCompletion(r.CompletionMode), RequiredFood: r.RequiredFood,
		RequiredGame: r.RequiredGame, FestivalMapID: intPtr(r.FestivalMapID), FestivalMapName: r.FestivalMapName,
		CreatedAt: r.CreatedAt, Stamps: []StampRallyStamp{}, Prizes: []StampRallyPrize{}, CardCount: r.CardCount,
		CompletedCount: r.CompletedCount, StampCount: r.StampCount, ActiveStampCount: r.ActiveStampCount,
	}
}

func toRallyStamp(s *db.GetRallyStampRow) StampRallyStamp {
	return StampRallyStamp{
		ID: s.ID, OccupantID: intPtr(s.OccupantID), StallName: s.StallName, AffiliateID: intPtr(s.AffiliateID),
		AffiliateName: s.AffiliateName, Image: s.Image, Password: s.Password, StampType: model.NormalizeStampType(s.StampType),
		Placement:  Placement{X: s.PosX, Y: s.PosY, Width: s.Width, Height: s.Height, Rotation: s.Rotation},
		ActiveFrom: s.ActiveFrom, ActiveTo: s.ActiveTo, Paused: s.Paused != 0, SortOrder: s.SortOrder,
	}
}

func toRallyPrize(p *db.StampRallyPrize) StampRallyPrize {
	return StampRallyPrize{
		ID: p.ID, Name: p.Name, Image: p.Image, SortOrder: p.SortOrder,
		Placement: Placement{X: p.PosX, Y: p.PosY, Width: p.Width, Height: p.Height, Rotation: p.Rotation},
	}
}

func toRallyCard(c *db.GetRallyCardRow) StampRallyCard {
	return StampRallyCard{
		ID: c.ID, Token: c.Token, ParticipantName: c.ParticipantName, World: c.World, Completed: c.Completed != 0,
		CompletedAt: c.CompletedAt, CreatedAt: c.CreatedAt, CollectedCount: c.CollectedCount,
	}
}

// rule is the rally as the pure rules read it.
func (r StampRally) rule() festival.Rally {
	return festival.Rally{
		Status: r.Status, AvailableFrom: deref(r.AvailableFrom), AvailableTo: deref(r.AvailableTo),
		CompletionMode: r.CompletionMode, RequiredFood: int(r.RequiredFood), RequiredGame: int(r.RequiredGame),
	}
}

// stampRules is the stamps as the pure rules read them.
func (r StampRally) stampRules() []festival.Stamp {
	out := make([]festival.Stamp, 0, len(r.Stamps))
	for _, s := range r.Stamps {
		out = append(out, s.rule())
	}
	return out
}

func (s StampRallyStamp) rule() festival.Stamp {
	return festival.Stamp{ID: s.ID, Type: s.StampType, Paused: s.Paused, ActiveFrom: deref(s.ActiveFrom), ActiveTo: deref(s.ActiveTo)}
}

// stall is the stall's name to show and to log.
func (s StampRallyStamp) stall() string {
	for _, name := range []string{s.StallName, s.AffiliateName} {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			return trimmed
		}
	}
	return model.DefaultStallName
}

// loadRally reads a rally with its stamps and prizes, or the 404 when there is none.
func (h handlers) loadRally(ctx context.Context, q *db.Queries, id int64) (StampRally, error) {
	row, err := q.GetStampRally(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return StampRally{}, noSuchRally()
	}
	if err != nil {
		return StampRally{}, h.fail("read a stamp rally", err)
	}
	r := toStampRally(row)
	stamps, err := q.ListStampRallyStamps(ctx, id)
	if err != nil {
		return StampRally{}, h.fail("read a rally's stamps", err)
	}
	for _, s := range stamps {
		r.Stamps = append(r.Stamps, toRallyStamp((*db.GetRallyStampRow)(s)))
	}
	prizes, err := q.ListStampRallyPrizes(ctx, id)
	if err != nil {
		return StampRally{}, h.fail("read a rally's prizes", err)
	}
	for _, p := range prizes {
		r.Prizes = append(r.Prizes, toRallyPrize(p))
	}
	return r, nil
}

func (h handlers) listStampRallies(ctx context.Context, _ *struct{}) (*rallyListOutput, error) {
	rows, err := h.read().ListStampRallies(ctx)
	if err != nil {
		return nil, h.fail("list stamp rallies", err)
	}
	out := StampRallyList{StampRallies: make([]StampRally, 0, len(rows))}
	for _, r := range rows {
		out.StampRallies = append(out.StampRallies, toStampRally((*db.GetStampRallyRow)(r)))
	}
	return &rallyListOutput{Body: out}, nil
}

func (h handlers) getStampRally(ctx context.Context, in *rallyIDInput) (*rallyDetailOutput, error) {
	q := h.read()
	r, err := h.loadRally(ctx, q, in.ID)
	if err != nil {
		return nil, err
	}
	cards, err := q.ListRallyCards(ctx, in.ID)
	if err != nil {
		return nil, h.fail("list a rally's cards", err)
	}
	out := StampRallyDetail{StampRally: r, Cards: make([]StampRallyCard, 0, len(cards))}
	for _, c := range cards {
		out.Cards = append(out.Cards, toRallyCard((*db.GetRallyCardRow)(c)))
	}
	return &rallyDetailOutput{Body: out}, nil
}

// rallySave is a rally's fields as they are stored.
type rallySave struct {
	rally  db.UpdateStampRallyParams
	stamps []savedStamp
	prizes []db.InsertStampRallyPrizeParams
}

// savedStamp is a stamp's columns beside the id it was saved with.
type savedStamp struct {
	id int64
	db.InsertStampRallyStampParams
}

// normalizeInput puts a time a client sent in the stored form: nil for none, or a
// detail naming the field when it isn't a time.
func normalizeInput(location string, in *string) (*string, *huma.ErrorDetail) {
	if in == nil {
		return nil, nil
	}
	n, err := store.NormalizeTime(*in)
	if err != nil {
		return nil, &huma.ErrorDetail{Location: location, Value: *in, Message: err.Error()}
	}
	if n == "" {
		return nil, nil
	}
	return &n, nil
}

// rallyColumns checks a rally's fields the schema can't and puts them as they are
// stored: the times, the completion rule against the stamps there are, the placements
// clamped, and the stamps settled against the festival they are filed under.
func (h handlers) rallyColumns(ctx context.Context, in StampRallyInput) (rallySave, error) {
	var problems []error
	add := func(d *huma.ErrorDetail) {
		if d != nil {
			problems = append(problems, d)
		}
	}
	from, d := normalizeInput("body.availableFrom", in.AvailableFrom)
	add(d)
	to, d := normalizeInput("body.availableTo", in.AvailableTo)
	add(d)

	out := rallySave{stamps: make([]savedStamp, 0, len(in.Stamps)), prizes: make([]db.InsertStampRallyPrizeParams, 0, len(in.Prizes))}
	food, game := 0, 0
	for i, s := range in.Stamps {
		loc := "body.stamps[" + strconv.Itoa(i) + "]"
		af, d := normalizeInput(loc+".activeFrom", s.ActiveFrom)
		add(d)
		at, d := normalizeInput(loc+".activeTo", s.ActiveTo)
		add(d)
		kind := model.NormalizeStampType(s.StampType)
		if kind == model.StampTypeGame {
			game++
		} else {
			food++
		}
		p := s.Placement.sanitized()
		out.stamps = append(out.stamps, savedStamp{id: s.ID, InsertStampRallyStampParams: db.InsertStampRallyStampParams{
			OccupantID: nullInt(s.OccupantID), AffiliateID: nullInt(s.AffiliateID), Image: strings.TrimSpace(s.Image),
			Password: strings.TrimSpace(s.Password), StampType: kind,
			PosX: p.X, PosY: p.Y, Width: p.Width, Height: p.Height, Rotation: p.Rotation,
			ActiveFrom: af, ActiveTo: at, Paused: boolInt(s.Paused), SortOrder: int64(i),
		}})
	}
	for i, pr := range in.Prizes {
		p := pr.Placement.sanitized()
		out.prizes = append(out.prizes, db.InsertStampRallyPrizeParams{
			Name: strings.TrimSpace(pr.Name), Image: strings.TrimSpace(pr.Image),
			PosX: p.X, PosY: p.Y, Width: p.Width, Height: p.Height, Rotation: p.Rotation, SortOrder: int64(i),
		})
	}

	// Kept (clamped) whatever the mode, so switching back to counts doesn't lose the
	// numbers already set.
	requiredFood := int64(festival.ClampRequired(int(in.RequiredFood), food))
	requiredGame := int64(festival.ClampRequired(int(in.RequiredGame), game))
	if msg := festival.CompletionRuleError(in.CompletionMode, int(requiredFood), int(requiredGame)); msg != "" {
		problems = append(problems,
			&huma.ErrorDetail{Location: "body.requiredFood", Value: in.RequiredFood, Message: msg},
			&huma.ErrorDetail{Location: "body.requiredGame", Value: in.RequiredGame, Message: msg})
	}
	if len(problems) > 0 {
		return rallySave{}, huma.Error422UnprocessableEntity("The stamp rally can't be saved as it is.", problems...)
	}

	if err := h.settleStalls(ctx, in.FestivalMapID, out.stamps); err != nil {
		return rallySave{}, err
	}
	out.rally = db.UpdateStampRallyParams{
		Title: strings.TrimSpace(in.Title), CardImage: strings.TrimSpace(in.CardImage),
		NotStampedImage: strings.TrimSpace(in.NotStampedImage), AvailableFrom: from, AvailableTo: to, Details: in.Details,
		RedeemInstructions: in.RedeemInstructions, RedeemImage: strings.TrimSpace(in.RedeemImage),
		PublicSignup: boolInt(in.PublicSignup), CompletionMode: model.NormalizeRallyCompletion(in.CompletionMode),
		RequiredFood: requiredFood, RequiredGame: requiredGame, FestivalMapID: nullInt(in.FestivalMapID),
	}
	return out, nil
}

// settleStalls settles a rally's festival link and every stamp's stall against what
// exists.
//
// A rally on a festival names the stalls of its plan (a stamp's occupantId) rather
// than a bare affiliate: a booth that changes hands between days is two businesses
// with two activities. A linked stamp takes its affiliate from its stall, one source
// of truth instead of two that can drift. A stamp naming a stall that isn't on the
// festival falls back to its affiliate rather than failing the save, and a rally on no
// festival has every stall cleared, since naming one would claim a link it lacks.
func (h handlers) settleStalls(ctx context.Context, festivalID *int64, stamps []savedStamp) error {
	if festivalID == nil {
		for i := range stamps {
			stamps[i].OccupantID = sql.NullInt64{}
		}
		return nil
	}
	m, err := h.findFestivalMap(ctx, h.read(), *festivalID)
	if err != nil {
		return h.fail("read a festival for a rally", err)
	}
	if m == nil {
		return unknown("body.festivalMapId", *festivalID, "festival map")
	}
	affiliates := make(map[int64]sql.NullInt64)
	for _, stall := range m.Stalls {
		for _, o := range stall.Occupants {
			a := sql.NullInt64{}
			if o.AffiliateID != nil {
				a = sql.NullInt64{Int64: *o.AffiliateID, Valid: true}
			}
			affiliates[o.ID] = a
		}
	}
	for i := range stamps {
		if !stamps[i].OccupantID.Valid {
			continue
		}
		a, found := affiliates[stamps[i].OccupantID.Int64]
		if !found {
			stamps[i].OccupantID = sql.NullInt64{}
			continue
		}
		stamps[i].AffiliateID = a
	}
	return nil
}

// replaceRallyPrizes writes a rally's prizes in the order sent, replacing the old.
func replaceRallyPrizes(ctx context.Context, q *db.Queries, rallyID int64, prizes []db.InsertStampRallyPrizeParams) error {
	if err := q.DeleteStampRallyPrizes(ctx, rallyID); err != nil {
		return err
	}
	for _, p := range prizes {
		p.RallyID = rallyID
		if err := q.InsertStampRallyPrize(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// reconcileStamps makes a rally's stamps match those saved: a stamp with an id is
// updated in place, so what was collected on it stays, id 0 is added, and one left out
// is deleted.
func reconcileStamps(ctx context.Context, q *db.Queries, rallyID int64, stamps []savedStamp) error {
	keep := make(map[int64]bool, len(stamps))
	for i, s := range stamps {
		s.RallyID = rallyID
		if s.id == 0 {
			id, err := q.InsertStampRallyStamp(ctx, s.InsertStampRallyStampParams)
			if err != nil {
				return err
			}
			keep[id] = true
			continue
		}
		n, err := q.UpdateStampRallyStamp(ctx, db.UpdateStampRallyStampParams{
			OccupantID: s.OccupantID, AffiliateID: s.AffiliateID, Image: s.Image, Password: s.Password, StampType: s.StampType,
			PosX: s.PosX, PosY: s.PosY, Width: s.Width, Height: s.Height, Rotation: s.Rotation,
			ActiveFrom: s.ActiveFrom, ActiveTo: s.ActiveTo, Paused: s.Paused, SortOrder: s.SortOrder, ID: s.id, RallyID: rallyID,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return unknown("body.stamps["+strconv.Itoa(i)+"].id", s.id, "stamp in this rally")
		}
		keep[s.id] = true
	}
	existing, err := q.ListStampRallyStampIDs(ctx, rallyID)
	if err != nil {
		return err
	}
	for _, id := range existing {
		if !keep[id] {
			if err := q.DeleteStampRallyStamp(ctx, db.DeleteStampRallyStampParams{ID: id, RallyID: rallyID}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h handlers) createStampRally(ctx context.Context, in *rallyCreateInput) (*rallyCreatedOutput, error) {
	save, err := h.rallyColumns(ctx, in.Body)
	if err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	var id int64
	err = h.transact(ctx, func(q *db.Queries) error {
		r := save.rally
		var err error
		id, err = q.CreateStampRally(ctx, db.CreateStampRallyParams{
			Title: r.Title, CardImage: r.CardImage, NotStampedImage: r.NotStampedImage, AvailableFrom: r.AvailableFrom,
			AvailableTo: r.AvailableTo, Details: r.Details, RedeemInstructions: r.RedeemInstructions, RedeemImage: r.RedeemImage,
			PublicSignup: r.PublicSignup, CompletionMode: r.CompletionMode, RequiredFood: r.RequiredFood,
			RequiredGame: r.RequiredGame, FestivalMapID: r.FestivalMapID,
		})
		if err != nil {
			return err
		}
		for i := range save.stamps {
			save.stamps[i].id = 0
		}
		if err := reconcileStamps(ctx, q, id, save.stamps); err != nil {
			return err
		}
		return replaceRallyPrizes(ctx, q, id, save.prizes)
	})
	var problem huma.StatusError
	switch {
	case errors.As(err, &problem):
		return nil, problem
	case err != nil:
		return nil, h.fail("create a stamp rally", err)
	}
	r, err := h.loadRally(ctx, h.read(), id)
	if err != nil {
		return nil, err
	}
	return &rallyCreatedOutput{Status: http.StatusCreated, Body: r}, nil
}

func (h handlers) updateStampRally(ctx context.Context, in *rallySaveInput) (*rallyOutput, error) {
	save, err := h.rallyColumns(ctx, in.Body)
	if err != nil {
		return nil, err
	}
	save.rally.ID = in.ID
	ctx = context.WithoutCancel(ctx)
	err = h.transact(ctx, func(q *db.Queries) error {
		n, err := q.UpdateStampRally(ctx, save.rally)
		if err != nil {
			return err
		}
		if n == 0 {
			return noSuchRally()
		}
		if err := reconcileStamps(ctx, q, in.ID, save.stamps); err != nil {
			return err
		}
		return replaceRallyPrizes(ctx, q, in.ID, save.prizes)
	})
	var problem huma.StatusError
	switch {
	case errors.As(err, &problem):
		return nil, problem
	case err != nil:
		return nil, h.fail("update a stamp rally", err)
	}
	r, err := h.loadRally(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	return &rallyOutput{Body: r}, nil
}

func (h handlers) deleteStampRally(ctx context.Context, in *rallyIDInput) (*struct{}, error) {
	ctx = context.WithoutCancel(ctx)
	var deleted int64
	err := h.transact(ctx, func(q *db.Queries) error {
		// The garapon's link is a plain column, so it is cleared here.
		if err := q.ClearGaraponRallyLink(ctx, sql.NullInt64{Int64: in.ID, Valid: true}); err != nil {
			return err
		}
		var err error
		deleted, err = q.DeleteStampRally(ctx, in.ID)
		return err
	})
	if err != nil {
		return nil, h.fail("delete a stamp rally", err)
	}
	if deleted == 0 {
		return nil, noSuchRally()
	}
	ResourceChanged(h.Live, "garapons")
	return nil, nil
}

// setRallyStatus closes or reopens a rally and answers it.
func (h handlers) setRallyStatus(ctx context.Context, id int64, status string) (*rallyOutput, error) {
	ctx = context.WithoutCancel(ctx)
	n, err := h.write().SetStampRallyStatus(ctx, db.SetStampRallyStatusParams{Status: status, ID: id})
	if err != nil {
		return nil, h.fail("set a stamp rally's status", err)
	}
	if n == 0 {
		return nil, noSuchRally()
	}
	r, err := h.loadRally(ctx, h.read(), id)
	if err != nil {
		return nil, err
	}
	return &rallyOutput{Body: r}, nil
}

func (h handlers) closeStampRally(ctx context.Context, in *rallyIDInput) (*rallyOutput, error) {
	return h.setRallyStatus(ctx, in.ID, "closed")
}

func (h handlers) reopenStampRally(ctx context.Context, in *rallyIDInput) (*rallyOutput, error) {
	return h.setRallyStatus(ctx, in.ID, "open")
}

func (h handlers) listStampRallyLogs(ctx context.Context, in *rallyIDInput) (*rallyLogOutput, error) {
	q := h.read()
	if _, err := q.GetStampRallyStatus(ctx, in.ID); errors.Is(err, sql.ErrNoRows) {
		return nil, noSuchRally()
	} else if err != nil {
		return nil, h.fail("read a stamp rally", err)
	}
	rows, err := q.ListRallyCollections(ctx, in.ID)
	if err != nil {
		return nil, h.fail("list a rally's log", err)
	}
	out := StampRallyLogList{Logs: make([]StampRallyLogEntry, 0, len(rows))}
	for _, e := range rows {
		out.Logs = append(out.Logs, StampRallyLogEntry{
			CardID: intPtr(e.CardID), ParticipantName: e.ParticipantName, StampID: intPtr(e.StampID),
			StallName: e.StallName, StampType: e.StampType, StampedAt: e.StampedAt,
		})
	}
	return &rallyLogOutput{Body: out}, nil
}

func (h handlers) updateStampRallyStamp(ctx context.Context, in *rallyStampInput) (*rallyStampOutput, error) {
	ctx = context.WithoutCancel(ctx)
	n, err := h.write().SetStampPaused(ctx, db.SetStampPausedParams{Paused: boolInt(in.Body.Paused), ID: in.StampID, RallyID: in.ID})
	if err != nil {
		return nil, h.fail("pause a stamp", err)
	}
	if n == 0 {
		return nil, huma.Error404NotFound("There is no such stamp in this rally.")
	}
	s, err := h.read().GetRallyStamp(ctx, db.GetRallyStampParams{ID: in.StampID, RallyID: in.ID})
	if err != nil {
		return nil, h.fail("read a stamp", err)
	}
	return &rallyStampOutput{Body: toRallyStamp(s)}, nil
}

func (h handlers) createStampRallyCard(ctx context.Context, in *rallyCardInput) (*rallyCardCreatedOutput, error) {
	q := h.read()
	if _, err := q.GetStampRallyStatus(ctx, in.ID); errors.Is(err, sql.ErrNoRows) {
		return nil, noSuchRally()
	} else if err != nil {
		return nil, h.fail("read a stamp rally", err)
	}
	name, world := strings.TrimSpace(in.Body.ParticipantName), strings.TrimSpace(in.Body.World)
	if world == "" {
		name, world = model.SplitParticipantLabel(name)
	}
	ctx = context.WithoutCancel(ctx)
	id, err := h.write().CreateRallyCard(ctx, db.CreateRallyCardParams{
		RallyID: in.ID, Token: random.Token(), ParticipantName: name, World: world,
	})
	if err != nil {
		return nil, h.fail("issue a stamp card", err)
	}
	c, err := h.read().GetRallyCard(ctx, id)
	if err != nil {
		return nil, h.fail("read a stamp card", err)
	}
	return &rallyCardCreatedOutput{Status: http.StatusCreated, Body: toRallyCard(c)}, nil
}

func (h handlers) deleteStampRallyCard(ctx context.Context, in *rallyCardIDInput) (*struct{}, error) {
	ctx = context.WithoutCancel(ctx)
	status, err := h.read().GetStampRallyStatus(ctx, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, noSuchRally()
	}
	if err != nil {
		return nil, h.fail("read a stamp rally", err)
	}
	// A card that has collected stamps waits for its rally to close; its log rows stay
	// either way.
	n, err := h.write().DeleteRallyCard(ctx, db.DeleteRallyCardParams{CardID: in.CardID, RallyID: in.ID, Force: boolInt(status == "closed")})
	if err != nil {
		return nil, h.fail("delete a stamp card", err)
	}
	if n > 0 {
		return nil, nil
	}
	c, err := h.read().GetRallyCard(ctx, in.CardID)
	switch {
	case errors.Is(err, sql.ErrNoRows) || (err == nil && c.RallyID != in.ID):
		return nil, huma.Error404NotFound("There is no such card in this rally.")
	case err != nil:
		return nil, h.fail("read a stamp card", err)
	}
	return nil, huma.Error409Conflict("This card has collected stamps and can't be deleted while the rally is open. " +
		"Close the rally first; the stamp log is kept.")
}

func (h handlers) listOpenStampRallies(ctx context.Context, _ *struct{}) (*signupListOutput, error) {
	rows, err := h.read().ListSignupRallies(ctx)
	if err != nil {
		return nil, h.fail("list rallies open to sign-up", err)
	}
	// The window is relative to now, so it is applied here rather than in the query.
	now := time.Now().UTC()
	out := SignupRallyList{Rallies: make([]SignupRally, 0, len(rows))}
	for _, r := range rows {
		if !festival.WithinWindow(deref(r.AvailableFrom), deref(r.AvailableTo), now) {
			continue
		}
		out.Rallies = append(out.Rallies, SignupRally{
			ID: r.ID, Title: r.Title, CardImage: r.CardImage, Details: r.Details,
			AvailableFrom: r.AvailableFrom, AvailableTo: r.AvailableTo, GaraponTitle: r.GaraponTitle,
		})
	}
	return &signupListOutput{Body: out}, nil
}

func (h handlers) signUpForStampRally(ctx context.Context, in *rallySignupInput) (*signupOutput, error) {
	// Every try counts, so a bot can't fill a rally's cards with junk names.
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.RallyLimit, ip); wait > 0 {
		h.Logger.Warn("stamp rally sign-up held back", "ip", ip)
		return nil, tooMany(wait, "Too many sign-ups. Please try again later.")
	}
	if h.RallyLimit != nil {
		h.RallyLimit.Fail(ip)
	}
	if !h.turnstile(ctx, in.Body.TurnstileToken, ip) {
		h.Logger.Warn("turnstile check failed", "ip", ip, "where", "stamp rally sign-up")
		return nil, huma.Error403Forbidden("Bot check failed. Please try again.")
	}

	name, world := strings.TrimSpace(in.Body.CharacterName), strings.TrimSpace(in.Body.World)
	q := h.read()
	row, err := q.GetStampRally(ctx, in.ID)
	// A rally that exists but didn't opt in is reported as missing, so this can't be used
	// to find rallies whose cards staff hand out privately.
	if errors.Is(err, sql.ErrNoRows) || (err == nil && row.PublicSignup == 0) {
		return nil, huma.Error404NotFound("That stamp rally isn't open for sign-ups.")
	}
	if err != nil {
		return nil, h.fail("read a stamp rally", err)
	}
	rally := toStampRally(row)
	if !festival.RallyOpen(rally.rule(), time.Now().UTC()) {
		return nil, huma.Error409Conflict("This stamp rally is no longer accepting sign-ups.")
	}

	// An open garapon linked to the rally means this sign-up issues a drawing link too.
	// If looking it up fails the participant still gets their card. Nobody is present to
	// choose a number of draws, so the garapon's own default is used.
	var garaponID int64
	var garaponTitle string
	draws := int64(1)
	switch g, err := q.OpenGaraponForRally(ctx, sql.NullInt64{Int64: in.ID, Valid: true}); {
	case err == nil:
		garaponID, garaponTitle, draws = g.ID, g.Title, festival.DefaultDraws(g.DefaultDraws)
	case !errors.Is(err, sql.ErrNoRows):
		h.Logger.Error("look up a rally's garapon", "rallyId", in.ID, "error", err)
	}

	// The name check and both inserts are one step: checking apart would let two sign-ups
	// racing on a name both see it free.
	ctx = context.WithoutCancel(ctx)
	cardToken, garaponToken := random.Token(), ""
	err = h.transact(ctx, func(q *db.Queries) error {
		taken, err := q.CountRallyCardsNamed(ctx, db.CountRallyCardsNamedParams{RallyID: in.ID, Name: name, World: world})
		if err != nil {
			return err
		}
		if taken > 0 {
			return huma.Error409Conflict(`Someone has already signed up for this rally under that name. ` +
				`If that was you, use "Find my links" to get your link back.`)
		}
		cardID, err := q.CreateRallyCard(ctx, db.CreateRallyCardParams{RallyID: in.ID, Token: cardToken, ParticipantName: name, World: world})
		if err != nil {
			return err
		}
		if garaponID == 0 {
			return nil
		}
		// A token of its own: the card's can be shared, the drawing link spends draws.
		garaponToken = random.Token()
		return q.CreateGaraponPlayerForCard(ctx, db.CreateGaraponPlayerForCardParams{
			GaraponID: garaponID, Token: garaponToken, PlayerName: name, World: world, MaxDraws: draws,
			StampCardID: sql.NullInt64{Int64: cardID, Valid: true},
		})
	})
	var problem huma.StatusError
	switch {
	case errors.As(err, &problem):
		return nil, problem
	case err != nil:
		return nil, h.fail("sign up for a stamp rally", err)
	}

	out := StampSignup{ParticipantName: name, World: world, RallyTitle: rally.Title, CardToken: cardToken}
	if garaponID != 0 {
		out.GaraponToken, out.GaraponTitle = &garaponToken, &garaponTitle
	}
	h.Logger.Info("stamp rally sign-up", "rallyId", in.ID, "rally", rally.Title,
		"participant", model.ParticipantLabel(name, world), "garapon", garaponTitle, "garaponDraws", draws)
	// The write middleware leaves public paths out, so tell the pages that show the new card
	// and drawing link.
	ResourceChanged(h.Live, "stamp-rallies")
	if garaponID != 0 {
		ResourceChanged(h.Live, "garapons")
	}
	return &signupOutput{Status: http.StatusCreated, Body: out}, nil
}

func (h handlers) lookUpStampCards(ctx context.Context, in *rallyLookupInput) (*lookupOutput, error) {
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.RallyLimit, ip); wait > 0 {
		h.Logger.Warn("stamp card lookup held back", "ip", ip)
		return nil, tooMany(wait, "Too many lookups. Please try again later.")
	}
	if h.RallyLimit != nil {
		h.RallyLimit.Fail(ip)
	}
	name, world := strings.TrimSpace(in.Body.CharacterName), strings.TrimSpace(in.Body.World)
	if world == "" {
		name, world = model.SplitParticipantLabel(name)
	}
	if utf8.RuneCountInString(name) == 0 {
		return nil, huma.Error422UnprocessableEntity("Enter the name you signed up with.", &huma.ErrorDetail{
			Location: "body.characterName", Value: in.Body.CharacterName, Message: "Enter the name you signed up with.",
		})
	}
	rows, err := h.read().LookUpParticipantCards(ctx, db.LookUpParticipantCardsParams{
		Name: name, World: world, Label: model.ParticipantLabel(name, world),
	})
	if err != nil {
		return nil, h.fail("look up stamp cards", err)
	}
	out := StampLookup{Entries: make([]StampLookupEntry, 0, len(rows))}
	for _, r := range rows {
		e := StampLookupEntry{
			RallyID: r.RallyID, RallyTitle: r.RallyTitle, CardToken: r.CardToken, ParticipantName: r.ParticipantName,
			World: r.World, Completed: r.Completed != 0, GaraponTitle: r.GaraponTitle,
		}
		// No garapon joined means the subtraction is meaningless.
		if r.GaraponTitle != "" {
			left := max(r.DrawsLeft, 0)
			e.GaraponDrawsLeft = &left
		}
		out.Entries = append(out.Entries, e)
	}
	return &lookupOutput{Body: out}, nil
}

// cardView is a card's data with the rally it belongs to and what it has collected.
type cardView struct {
	card      *db.GetRallyCardByTokenRow
	rally     StampRally
	collected map[int64]string
}

func (h handlers) loadCardView(ctx context.Context, q *db.Queries, token string) (cardView, error) {
	card, err := q.GetRallyCardByToken(ctx, strings.TrimSpace(token))
	if errors.Is(err, sql.ErrNoRows) {
		return cardView{}, noSuchStampCard()
	}
	if err != nil {
		return cardView{}, h.fail("read a stamp card", err)
	}
	rally, err := h.loadRally(ctx, q, card.RallyID)
	if err != nil {
		return cardView{}, err
	}
	collected, err := collectedStamps(ctx, q, card.ID)
	if err != nil {
		return cardView{}, h.fail("read a card's stamps", err)
	}
	return cardView{card: card, rally: rally, collected: collected}, nil
}

// collectedStamps is the stamps a card holds, each with when it was collected.
func collectedStamps(ctx context.Context, q *db.Queries, cardID int64) (map[int64]string, error) {
	rows, err := q.ListCollectedStamps(ctx, sql.NullInt64{Int64: cardID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(rows))
	for _, r := range rows {
		out[r.StampID.Int64] = deref(r.StampedAt)
	}
	return out, nil
}

// completeCard records a card as complete when it has just become so, which catches a
// completion that came of a stamp's window ending, not only of a stamp collected. It
// reports whether it did.
func completeCard(ctx context.Context, q *db.Queries, v *cardView, now time.Time) (bool, error) {
	if v.card.Completed != 0 || !festival.CardComplete(v.rally.rule(), v.rally.stampRules(), v.collected, now) {
		return false, nil
	}
	at := now.UTC().Format(time.RFC3339)
	if err := q.SetRallyCardCompleted(ctx, db.SetRallyCardCompletedParams{CompletedAt: &at, ID: v.card.ID}); err != nil {
		return false, err
	}
	v.card.Completed, v.card.CompletedAt = 1, &at
	return true, nil
}

// publicCard is the card a participant sees: no passwords, availability and collection
// worked out against now, and each prize named only once the card is complete.
func (h handlers) publicCard(ctx context.Context, v cardView, now time.Time) PublicStampCard {
	r := v.rally
	rule := r.rule()
	card := v.card
	complete := card.Completed != 0
	out := PublicStampCard{
		Rally: PublicStampRally{
			ID: r.ID, Title: r.Title, CardImage: r.CardImage, NotStampedImage: r.NotStampedImage, Details: r.Details,
			RedeemInstructions: r.RedeemInstructions, RedeemImage: r.RedeemImage, AvailableFrom: r.AvailableFrom,
			AvailableTo: r.AvailableTo, IsActive: festival.RallyOpen(rule, now), CompletionMode: r.CompletionMode,
			RequiredFood: r.RequiredFood, RequiredGame: r.RequiredGame, FestivalLink: h.festivalLink(ctx, r.ID),
		},
		ParticipantName: model.ParticipantLabel(card.ParticipantName, card.World),
		Completed:       complete,
		CompletedAt:     card.CompletedAt,
		PrizesRevealed:  complete,
		Stamps:          make([]PublicStamp, 0, len(r.Stamps)),
		Prizes:          make([]PublicPrize, 0, len(r.Prizes)),
	}
	for _, s := range r.Stamps {
		at, got := v.collected[s.ID]
		ps := PublicStamp{
			ID: s.ID, AffiliateName: s.stall(), OccupantID: s.OccupantID, StampType: s.StampType, Image: s.Image,
			Placement: s.Placement, ActiveFrom: s.ActiveFrom, ActiveTo: s.ActiveTo,
			Available: festival.StampAvailable(rule, s.rule(), now), Expired: festival.StampExpired(rule, s.rule(), now),
			Collected: got,
		}
		if got && at != "" {
			ps.CollectedAt = &at
		}
		out.Stamps = append(out.Stamps, ps)
	}
	for _, p := range r.Prizes {
		pp := PublicPrize{ID: p.ID, Placement: p.Placement}
		if complete {
			pp.Name, pp.Image = p.Name, p.Image
		}
		out.Prizes = append(out.Prizes, pp)
	}
	return out
}

func (h handlers) getStampCard(ctx context.Context, in *tokenInput) (*stampCardOutput, error) {
	v, err := h.loadCardView(ctx, h.read(), in.Token)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	changed, err := completeCard(context.WithoutCancel(ctx), h.write(), &v, now)
	if err != nil {
		// The collected stamps are already kept and the card completes on the next view, so
		// this doesn't fail the read; but it is said, or a card would report itself
		// incomplete with nothing anywhere to say why.
		h.Logger.Error("mark a stamp card complete", "cardId", v.card.ID, "rallyId", v.rally.ID, "error", err)
	}
	if changed {
		ResourceChanged(h.Live, "stamp-rallies")
	}
	return &stampCardOutput{Body: h.publicCard(ctx, v, now)}, nil
}

func (h handlers) stampStampCard(ctx context.Context, in *stampCardStampInput) (*stampResultOutput, error) {
	// Stamp passwords are short words read out at a stall, so an unthrottled endpoint is a
	// password oracle: a script could collect a card's stamps without visiting anything.
	// Only a miss costs budget, so a participant collecting normally never notices.
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.StampGuessLimit, ip); wait > 0 {
		h.Logger.Warn("stamp password guessing held back", "ip", ip)
		return nil, tooMany(wait, "Too many incorrect passwords. Please try again later.")
	}
	v, err := h.loadCardView(ctx, h.read(), in.Token)
	if err != nil {
		return nil, err
	}
	password := strings.TrimSpace(in.Body.Password)
	var match *StampRallyStamp
	for i := range v.rally.Stamps {
		if s := &v.rally.Stamps[i]; s.Password != "" && s.Password == password {
			match = s
			break
		}
	}
	if match == nil {
		if h.StampGuessLimit != nil {
			h.StampGuessLimit.Fail(ip)
		}
		return nil, huma.Error422UnprocessableEntity("That password doesn't match any stamp on this card.", &huma.ErrorDetail{
			Location: "body.password", Message: "That password doesn't match any stamp on this card.",
		})
	}
	if h.StampGuessLimit != nil {
		h.StampGuessLimit.Reset(ip)
	}

	now := time.Now().UTC()
	if !festival.StampAvailable(v.rally.rule(), match.rule(), now) {
		return nil, huma.Error409Conflict("This stall is currently closed and cannot be stamped.")
	}

	// The participant and the stall are logged as they are now, whole, so the log outlives
	// the card and the stamp and can still tell apart two players who share a name.
	ctx = context.WithoutCancel(ctx)
	err = h.transact(ctx, func(q *db.Queries) error {
		n, err := q.CollectStamp(ctx, db.CollectStampParams{
			RallyID: v.rally.ID, CardID: sql.NullInt64{Int64: v.card.ID, Valid: true}, StampID: sql.NullInt64{Int64: match.ID, Valid: true},
			ParticipantName: model.ParticipantLabel(v.card.ParticipantName, v.card.World), StallName: match.stall(),
			StampType: model.NormalizeStampType(match.StampType),
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return huma.Error409Conflict("You've already collected this stamp.")
		}
		if v.collected, err = collectedStamps(ctx, q, v.card.ID); err != nil {
			return err
		}
		_, err = completeCard(ctx, q, &v, now)
		return err
	})
	var problem huma.StatusError
	switch {
	case errors.As(err, &problem):
		return nil, problem
	case err != nil:
		return nil, h.fail("collect a stamp", err)
	}
	// The write middleware leaves public paths out, so tell the pages that show the log.
	ResourceChanged(h.Live, "stamp-rallies")
	return &stampResultOutput{Body: StampResult{Card: h.publicCard(ctx, v, now), CollectedStampID: match.ID}}, nil
}
