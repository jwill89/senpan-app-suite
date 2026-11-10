package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/db"
	"app-suite/internal/model"
	"app-suite/internal/permissions"
	"app-suite/internal/raffles"
	"app-suite/internal/random"
	"app-suite/internal/store"
)

// Raffles: a prize drawn from the entries people buy tickets with. The public sees the
// raffles open now and signs up; the Raffles page's staff see every raffle, manage its
// entries and payments, and draw and confirm a winner.

var raffleTags = []string{"Raffles"}

// lookupLimit is the most entries one public search answers; past it the answer says
// it is truncated.
const (
	lookupLimit    = 50
	lookupMinQuery = 2
)

// Raffle is a raffle as the API answers it.
type Raffle struct {
	ID                 int64   `json:"id" doc:"The raffle's id."`
	Title              string  `json:"title" doc:"The raffle's title."`
	Description        string  `json:"description" doc:"What it is about (Markdown)."`
	Rules              string  `json:"rules" doc:"Its rules (Markdown)."`
	MaxEntries         int64   `json:"maxEntries" doc:"The most tickets one person may hold. In custom mode it is the length of the price ladder."`
	SignupInstructions string  `json:"signupInstructions" doc:"How to pay and sign up (Markdown). A details-only raffle shows them in place of a sign-up form; the others after signing up."`
	EntryMode          string  `json:"entryMode" enum:"details,single,custom" doc:"How it prices tickets and takes sign-ups: details publishes it for reference and is entered elsewhere, single charges every ticket costPerEntry, custom prices each ticket from tierCosts in order."`
	CostPerEntry       int64   `json:"costPerEntry" doc:"The gil every ticket costs in single mode; 0 in the others."`
	TierCosts          []int64 `json:"tierCosts" nullable:"false" doc:"The gil the first, second, third... ticket costs in custom mode; empty in the others."`
	AvailableFrom      *string `json:"availableFrom" nullable:"true" doc:"When it opens, RFC 3339 in UTC; null for always."`
	AvailableTo        *string `json:"availableTo" nullable:"true" doc:"When it stops taking entries, RFC 3339 in UTC; null for no end."`
	PrizeImage         string  `json:"prizeImage" doc:"The prize's picture: a path from the image library."`
	PayImage           string  `json:"payImage" doc:"A screenshot showing where to pay, shown under the sign-up instructions: a path from the image library."`
	Status             string  `json:"status" enum:"open,closed" doc:"open is on the public list while inside its window; closed shows its winner."`
	WinnerEntryID      *int64  `json:"winnerEntryId" nullable:"true" doc:"The entry picked as the winner; null until one is picked. Only a closed raffle's winner is public."`
	FestivalMapID      *int64  `json:"festivalMapId" nullable:"true" doc:"The festival it belongs to; null for none."`
	FestivalMapName    string  `json:"festivalMapName" doc:"The festival's title; empty for none."`
	OccupantID         *int64  `json:"occupantId" nullable:"true" doc:"The stall on that festival's plan it is assigned to; null for none."`
	StallName          string  `json:"stallName" doc:"The stall's title; empty for none."`
	CreatedAt          *string `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
	WinnerName         *string `json:"winnerName" nullable:"true" doc:"The winner as Character @ World. Filled by list-raffles for an account holding the Raffles page; null for anyone else."`
	PaidTotal          *int64  `json:"paidTotal" nullable:"true" doc:"The gil collected: every settled ticket priced by the entry mode, less what was waived. Filled by list-raffles for an account holding the Raffles page; null for anyone else."`
}

// RaffleInput is a raffle as a save sends it: all of its editable fields, since a save
// replaces them. Its status and winner change through their own operations.
type RaffleInput struct {
	Title              string  `json:"title" minLength:"1" maxLength:"200" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The raffle's title."`
	Description        string  `json:"description" maxLength:"20000" doc:"What it is about (Markdown); empty for none."`
	Rules              string  `json:"rules" maxLength:"20000" doc:"Its rules (Markdown); empty for none."`
	MaxEntries         int64   `json:"maxEntries" minimum:"0" maximum:"100" doc:"The most tickets one person may hold, 100 at most; below 1 holds 1. In custom mode it is set to the length of tierCosts."`
	SignupInstructions string  `json:"signupInstructions" maxLength:"20000" doc:"How to pay and sign up (Markdown); empty for none."`
	EntryMode          string  `json:"entryMode" enum:"details,single,custom" doc:"How it prices tickets and takes sign-ups: details publishes it for reference and is entered elsewhere, single charges every ticket costPerEntry, custom prices each ticket from tierCosts in order."`
	CostPerEntry       int64   `json:"costPerEntry" minimum:"0" doc:"The gil every ticket costs in single mode; ignored in the others."`
	TierCosts          []int64 `json:"tierCosts" maxItems:"100" nullable:"false" doc:"The gil the first, second, third... ticket costs in custom mode, each 0 or more and at least one; ignored in the others."`
	AvailableFrom      *string `json:"availableFrom,omitempty" nullable:"true" format:"date-time" doc:"When it opens, RFC 3339; null or left out for always."`
	AvailableTo        *string `json:"availableTo,omitempty" nullable:"true" format:"date-time" doc:"When it stops taking entries, RFC 3339; null or left out for no end."`
	PrizeImage         string  `json:"prizeImage" maxLength:"500" doc:"The prize's picture: a path from the image library; empty for none."`
	PayImage           string  `json:"payImage" maxLength:"500" doc:"A screenshot showing where to pay: a path from the image library; empty for none."`
	FestivalMapID      *int64  `json:"festivalMapId,omitempty" nullable:"true" minimum:"1" doc:"The festival to file it under; null or left out for none. One that no longer exists is refused."`
	OccupantID         *int64  `json:"occupantId,omitempty" nullable:"true" minimum:"1" doc:"The stall on that festival's plan to assign it to; null or left out for none. A stall that isn't on the festival is dropped, and the festival's absence clears it."`
}

// RaffleList is the raffles a caller may see.
type RaffleList struct {
	Raffles []Raffle `json:"raffles" nullable:"false" doc:"For the public, the raffles open now inside their window. For an account holding the Raffles page, every raffle with its winnerName and paidTotal. Newest first."`
}

// RaffleWinner is the winner of a closed raffle as the public sees them.
type RaffleWinner struct {
	CharacterName string `json:"characterName" doc:"The winning character."`
	World         string `json:"world" doc:"Their home world."`
	NumEntries    int64  `json:"numEntries" doc:"How many tickets they held."`
}

// RaffleDetail is one raffle and its totals.
type RaffleDetail struct {
	Raffle       Raffle        `json:"raffle" doc:"The raffle."`
	TotalEntries int64         `json:"totalEntries" doc:"The tickets held across all its entries."`
	WinnerEntry  *RaffleWinner `json:"winnerEntry" doc:"The winner: set only for a closed raffle that has one, and without any payment detail."`
}

// RaffleEntry is a person's entry in a raffle: one or more tickets, merged per
// character and world.
type RaffleEntry struct {
	ID            int64   `json:"id" doc:"The entry's id."`
	RaffleID      int64   `json:"raffleId" doc:"The raffle it is in."`
	CharacterName string  `json:"characterName" doc:"The character."`
	World         string  `json:"world" doc:"Their home world."`
	NumEntries    int64   `json:"numEntries" doc:"The tickets held."`
	PaidEntries   int64   `json:"paidEntries" doc:"How many of them are settled."`
	AmountWaived  int64   `json:"amountWaived" doc:"The gil forgiven across all its settlements."`
	Paid          bool    `json:"paid" doc:"Whether nothing is outstanding."`
	PaymentState  string  `json:"paymentState" enum:"unpaid,partial,paid" doc:"unpaid when none is settled, partial when some are (more were bought since), paid when all are."`
	CreatedAt     *string `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
}

// RaffleEntryList is a raffle's entries.
type RaffleEntryList struct {
	Entries []RaffleEntry `json:"entries" nullable:"false" doc:"Every entry, oldest first."`
}

// RaffleEntryInput is an entry staff add.
type RaffleEntryInput struct {
	CharacterName string `json:"characterName" minLength:"1" maxLength:"60" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The character."`
	World         string `json:"world" minLength:"1" maxLength:"60" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"Their home world."`
	NumEntries    int64  `json:"numEntries" minimum:"1" maximum:"100" doc:"The tickets to add."`
	Paid          bool   `json:"paid,omitempty" doc:"Whether to settle the tickets just added at once."`
	AmountWaived  int64  `json:"amountWaived,omitempty" minimum:"0" doc:"The gil to forgive on that settlement. Ignored unless paid."`
}

// RaffleSettlement is a payment staff record on an entry.
type RaffleSettlement struct {
	PaidEntries *int64 `json:"paidEntries,omitempty" minimum:"1" maximum:"100" doc:"How many of the entry's tickets this settles, for a part payment. Left out, it settles every ticket; more than it holds settles all."`
	Waived      int64  `json:"waived,omitempty" minimum:"0" doc:"The gil forgiven now. It is added to what the entry has had waived, never replacing it."`
}

// RaffleEnterInput is a public sign-up.
type RaffleEnterInput struct {
	CharacterName  string `json:"characterName" minLength:"1" maxLength:"60" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The character signing up."`
	World          string `json:"world" minLength:"1" maxLength:"60" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"Their home world."`
	NumEntries     int64  `json:"numEntries" minimum:"1" maximum:"100" doc:"The tickets to add to the character's entry."`
	TurnstileToken string `json:"turnstileToken,omitempty" maxLength:"2048" doc:"The Cloudflare Turnstile response, when the check is on (GET /config gives its site key)."`
}

// RaffleEnterResult confirms a sign-up.
type RaffleEnterResult struct {
	Message            string `json:"message" doc:"Signed up, or entries added to an existing entry."`
	TotalEntries       int64  `json:"totalEntries" doc:"The tickets the character now holds."`
	TotalCost          int64  `json:"totalCost" doc:"The sticker price of every ticket they hold."`
	AmountDue          int64  `json:"amountDue" doc:"What they still have to pay: the price of the tickets not yet settled. It says nothing of why it is lower than the total."`
	SignupInstructions string `json:"signupInstructions" doc:"How to pay (Markdown)."`
}

// RaffleLookupInput is a search of a raffle's entrants by name.
type RaffleLookupInput struct {
	Name string `json:"name" minLength:"2" maxLength:"60" doc:"Part of the character name they signed up with: at least 2 characters, not counting spaces around it."`
}

// RaffleLookupEntry is one hit of a search.
type RaffleLookupEntry struct {
	CharacterName string `json:"characterName" doc:"The character, spelled as they signed up."`
	World         string `json:"world" doc:"Their home world."`
	NumEntries    int64  `json:"numEntries" doc:"The tickets held."`
	PaidEntries   int64  `json:"paidEntries" doc:"How many of them are settled."`
	PaymentState  string `json:"paymentState" enum:"unpaid,partial,paid" doc:"How far it is settled."`
}

// RaffleLookup is the answer to a search.
type RaffleLookup struct {
	Entries   []RaffleLookupEntry `json:"entries" nullable:"false" doc:"Entries whose character name contains the search, by name, without an id or any gil figure. At most 50."`
	Truncated bool                `json:"truncated" doc:"Whether more matched than are listed: narrow the search."`
}

// RaffleWinnerChoice asks to draw another winner.
type RaffleWinnerChoice struct {
	Replaces int64 `json:"replaces" minimum:"1" doc:"The entry that is the pending winner now. If it isn't, another console changed it and nothing happens."`
}

// RaffleWinnerConfirm confirms the pending winner.
type RaffleWinnerConfirm struct {
	WinnerEntryID int64 `json:"winnerEntryId" minimum:"1" doc:"The entry the staff confirm as the winner. If it isn't the pending winner, another console changed it and nothing happens."`
}

type raffleIDInput struct {
	ID int64 `path:"id" doc:"The raffle's id."`
}

type raffleEntryIDInput struct {
	ID      int64 `path:"id" doc:"The raffle's id."`
	EntryID int64 `path:"entryId" doc:"The entry's id."`
}

type raffleCreateInput struct{ Body RaffleInput }

type raffleSaveInput struct {
	ID   int64 `path:"id" doc:"The raffle's id."`
	Body RaffleInput
}

type raffleEntryAddInput struct {
	ID   int64 `path:"id" doc:"The raffle's id."`
	Body RaffleEntryInput
}

type raffleSettleInput struct {
	ID      int64 `path:"id" doc:"The raffle's id."`
	EntryID int64 `path:"entryId" doc:"The entry's id."`
	Body    RaffleSettlement
}

type raffleEnterRequest struct {
	ID   int64 `path:"id" doc:"The raffle's id."`
	Body RaffleEnterInput
}

type raffleLookupRequest struct {
	ID   int64 `path:"id" doc:"The raffle's id."`
	Body RaffleLookupInput
}

type raffleChoiceInput struct {
	ID   int64 `path:"id" doc:"The raffle's id."`
	Body RaffleWinnerChoice
}

type raffleConfirmInput struct {
	ID   int64 `path:"id" doc:"The raffle's id."`
	Body RaffleWinnerConfirm
}

type raffleOutput struct{ Body Raffle }

type raffleCreatedOutput struct {
	Status int
	Body   Raffle
}

type raffleListOutput struct{ Body RaffleList }

type raffleDetailOutput struct{ Body RaffleDetail }

type raffleEntryOutput struct{ Body RaffleEntry }

type raffleEntryAddedOutput struct {
	Status int
	Body   RaffleEntry
}

type raffleEntryListOutput struct{ Body RaffleEntryList }

type raffleEnterOutput struct {
	Status int
	Body   RaffleEnterResult
}

type raffleLookupOutput struct{ Body RaffleLookup }

// alsoOK declares a 200 answer carrying T beside an operation's 201, for an operation
// that answers 201 when it makes something and 200 when it changes what was there.
func alsoOK[T any](api huma.API, description string) map[string]*huma.Response {
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[T](), true, "")
	return map[string]*huma.Response{"200": {
		Description: description,
		Content:     map[string]*huma.MediaType{"application/json": {Schema: schema}},
	}}
}

func (h handlers) registerRaffles(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.TeahouseRaffles)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}
	notFound := slices.Concat(guarded, []int{http.StatusNotFound})

	huma.Register(api, huma.Operation{
		OperationID: "list-raffles", Method: http.MethodGet, Path: "/raffles",
		Summary: "The raffles a caller may see",
		Description: "Anyone gets the raffles open now inside their window. An account holding the Raffles page " +
			"gets every raffle, with the winner and the gil collected.",
		Tags: raffleTags,
	}, h.listRaffles)
	huma.Register(api, huma.Operation{
		OperationID: "get-raffle", Method: http.MethodGet, Path: "/raffles/{id}",
		Summary: "One raffle",
		Description: "Anyone gets a closed raffle or one open inside its window; a raffle not open yet is a 404, " +
			"so guessing its id reveals nothing. An account holding the Raffles page gets any.",
		Tags: raffleTags, Errors: []int{http.StatusNotFound},
	}, h.getRaffle)
	huma.Register(api, huma.Operation{
		OperationID: "create-raffle", Method: http.MethodPost, Path: "/raffles",
		Summary: "Make a raffle", Description: "It starts open.", Tags: raffleTags,
		DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.createRaffle)
	huma.Register(api, huma.Operation{
		OperationID: "update-raffle", Method: http.MethodPut, Path: "/raffles/{id}",
		Summary: "Replace a raffle's editable fields", Description: "Its status and winner are kept.",
		Tags: raffleTags, Security: secured, Middlewares: page,
		Errors: slices.Concat(notFound, []int{http.StatusUnprocessableEntity}),
	}, h.updateRaffle)
	huma.Register(api, huma.Operation{
		OperationID: "delete-raffle", Method: http.MethodDelete, Path: "/raffles/{id}",
		Summary: "Delete a raffle", Description: "Its entries go with it. Its pictures stay in the image library.",
		Tags: raffleTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page, Errors: notFound,
	}, h.deleteRaffle)
	huma.Register(api, huma.Operation{
		OperationID: "close-raffle", Method: http.MethodPost, Path: "/raffles/{id}/close",
		Summary: "Close a raffle",
		Description: "Without confirming a winner: for one that drew no entries, or a details-only raffle drawn " +
			"elsewhere. A winner already recorded is kept.",
		Tags: raffleTags, Security: secured, Middlewares: page, Errors: notFound,
	}, h.closeRaffle)
	huma.Register(api, huma.Operation{
		OperationID: "reopen-raffle", Method: http.MethodPost, Path: "/raffles/{id}/reopen",
		Summary: "Open a closed raffle again", Description: "The undo of a close. A winner recorded is kept.",
		Tags: raffleTags, Security: secured, Middlewares: page, Errors: notFound,
	}, h.reopenRaffle)
	huma.Register(api, huma.Operation{
		OperationID: "list-raffle-entries", Method: http.MethodGet, Path: "/raffles/{id}/entries",
		Summary: "A raffle's entries", Tags: raffleTags, Security: secured, Middlewares: page, Errors: notFound,
	}, h.listRaffleEntries)
	huma.Register(api, huma.Operation{
		OperationID: "add-raffle-entry", Method: http.MethodPost, Path: "/raffles/{id}/entries",
		Summary: "Add an entry",
		Description: "Staff add tickets for a character at any time while the raffle is open, inside its window or " +
			"not. 201 when it makes a new entry, 200 when the tickets join the character's existing entry (matched " +
			"on the whole name and world, ignoring case). A closed raffle, or tickets past the allowance, is a 409.",
		Tags: raffleTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Responses: alsoOK[RaffleEntry](api, "The tickets joined the character's existing entry."),
		Errors:    slices.Concat(notFound, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.addRaffleEntry)
	huma.Register(api, huma.Operation{
		OperationID: "settle-raffle-entry", Method: http.MethodPost, Path: "/raffles/{id}/entries/{entryId}/settle",
		Summary: "Record a payment on an entry",
		Description: "Settles every ticket, or paidEntries of them for a part payment, and adds waived to the gil " +
			"forgiven. A payment only moves forward: settling an entry with nothing outstanding changes nothing.",
		Tags: raffleTags, Security: secured, Middlewares: page, Errors: slices.Concat(notFound, []int{http.StatusUnprocessableEntity}),
	}, h.settleRaffleEntry)
	huma.Register(api, huma.Operation{
		OperationID: "unsettle-raffle-entry", Method: http.MethodPost, Path: "/raffles/{id}/entries/{entryId}/unsettle",
		Summary: "Clear an entry's payment",
		Description: "Nothing settled and nothing waived, so a mistake leaves no waiver credited against an " +
			"unpaid entry.",
		Tags: raffleTags, Security: secured, Middlewares: page, Errors: notFound,
	}, h.unsettleRaffleEntry)
	huma.Register(api, huma.Operation{
		OperationID: "delete-raffle-entry", Method: http.MethodDelete, Path: "/raffles/{id}/entries/{entryId}",
		Summary: "Delete an entry", Description: "If it was the pending winner, the raffle has none again.",
		Tags: raffleTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page, Errors: notFound,
	}, h.deleteRaffleEntry)
	huma.Register(api, huma.Operation{
		OperationID: "pick-raffle-winner", Method: http.MethodPost, Path: "/raffles/{id}/pick-winner",
		Summary: "Draw the pending winner",
		Description: "An entry chosen at random, one chance per ticket settled (per ticket held in a details-only " +
			"raffle). A raffle that already has a pending winner is a 409: confirm them or pick another.",
		Tags: raffleTags, Security: secured, Middlewares: page, Errors: slices.Concat(notFound, []int{http.StatusConflict}),
	}, h.pickRaffleWinner)
	huma.Register(api, huma.Operation{
		OperationID: "pick-another-raffle-winner", Method: http.MethodPost, Path: "/raffles/{id}/pick-another",
		Summary: "Draw another winner",
		Description: "Replaces the pending winner with another entry. If the pending winner isn't the one named " +
			"(another console changed it) nothing changes (409). With no other entry to draw, the raffle is left " +
			"with no pending winner (409).",
		Tags: raffleTags, Security: secured, Middlewares: page, Errors: slices.Concat(notFound, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.pickAnotherRaffleWinner)
	huma.Register(api, huma.Operation{
		OperationID: "verify-raffle-winner", Method: http.MethodPost, Path: "/raffles/{id}/verify-winner",
		Summary: "Confirm the winner and close the raffle",
		Description: "The pending winner becomes the raffle's winner and the raffle closes. With no pending winner, " +
			"or one that isn't the entry named, nothing happens (409).",
		Tags: raffleTags, Security: secured, Middlewares: page, Errors: slices.Concat(notFound, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.verifyRaffleWinner)
	huma.Register(api, huma.Operation{
		OperationID: "enter-raffle", Method: http.MethodPost, Path: "/raffles/{id}/enter",
		Summary: "Sign up for a raffle",
		Description: "Anyone may: 201 for a new entry, 200 when the tickets join the character's existing one. " +
			"Twenty tries from one address in ten minutes, made or not (429, with Retry-After). A raffle that is " +
			"closed, outside its window or details-only, or tickets past the allowance, is a 409.",
		Tags: raffleTags, DefaultStatus: http.StatusCreated,
		Responses: alsoOK[RaffleEnterResult](api, "The tickets joined the character's existing entry."),
		Errors: []int{http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity,
			http.StatusTooManyRequests},
	}, h.enterRaffle)
	huma.Register(api, huma.Operation{
		OperationID: "look-up-raffle-entries", Method: http.MethodPost, Path: "/raffles/{id}/lookup",
		Summary: "Find an entry by name",
		Description: "Have I already entered? Entries merge on the whole name and world, so a returning entrant " +
			"needs the exact spelling they used. Matches part of the name, ignoring case. Thirty searches from " +
			"one address in ten minutes (429, with Retry-After). Only for a raffle the caller can see.",
		Tags: raffleTags, Errors: []int{http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusTooManyRequests},
	}, h.lookUpRaffleEntries)
}

func noSuchRaffle() error { return huma.Error404NotFound("There is no such raffle.") }

func noSuchRaffleEntry() error {
	return huma.Error404NotFound("There is no such entry in this raffle.")
}

func nullInt(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func intPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

// toRaffle is a stored raffle as the API answers it, without the staff aggregates.
func toRaffle(r *db.GetRaffleRow) Raffle {
	var ladder []float64
	// A ladder that doesn't read prices nothing, rather than failing the page.
	if err := json.Unmarshal([]byte(r.TierCosts), &ladder); err != nil {
		ladder = nil
	}
	tiers := make([]int64, 0, len(ladder))
	for _, c := range ladder {
		tiers = append(tiers, raffles.Gil(c))
	}
	return Raffle{
		ID: r.ID, Title: r.Title, Description: r.Description, Rules: r.Rules, MaxEntries: r.MaxEntries,
		SignupInstructions: r.SignupInstructions, EntryMode: raffles.NormalizeMode(r.EntryMode),
		CostPerEntry: raffles.Gil(r.CostPerEntry), TierCosts: tiers,
		AvailableFrom: r.AvailableFrom, AvailableTo: r.AvailableTo, PrizeImage: r.PrizeImage, PayImage: r.PayImage,
		Status: r.Status, WinnerEntryID: intPtr(r.WinnerEntryID),
		FestivalMapID: intPtr(r.FestivalMapID), FestivalMapName: r.FestivalMapName,
		OccupantID: intPtr(r.OccupantID), StallName: r.StallName, CreatedAt: r.CreatedAt,
	}
}

// pricing is what the raffle charges for its tickets.
func (r Raffle) pricing() raffles.Pricing {
	return raffles.Pricing{Mode: r.EntryMode, CostPerEntry: r.CostPerEntry, TierCosts: r.TierCosts}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// open reports whether the raffle is taking entries now.
func (r Raffle) open(now time.Time) bool {
	return raffles.IsOpen(r.Status, deref(r.AvailableFrom), deref(r.AvailableTo), now)
}

// viewable reports whether anyone may see the raffle now.
func (r Raffle) viewable(now time.Time) bool {
	return raffles.PubliclyViewable(r.Status, deref(r.AvailableFrom), deref(r.AvailableTo), now)
}

func toRaffleEntry(e *db.RaffleEntry) RaffleEntry {
	return RaffleEntry{
		ID: e.ID, RaffleID: e.RaffleID, CharacterName: e.CharacterName, World: e.World,
		NumEntries: e.NumEntries, PaidEntries: e.PaidEntries, AmountWaived: raffles.Gil(e.AmountWaived),
		Paid: e.Paid != 0, PaymentState: raffles.PaymentState(int(e.NumEntries), int(e.PaidEntries)), CreatedAt: e.CreatedAt,
	}
}

func (e RaffleEntry) settlement() raffles.Entry {
	return raffles.Entry{NumEntries: int(e.NumEntries), PaidEntries: int(e.PaidEntries), AmountWaived: e.AmountWaived}
}

// raffleStaff reports whether the caller holds the Raffles page.
func raffleStaff(ctx context.Context) bool {
	return Permitted(Who(ctx), permissions.TeahouseRaffles)
}

// loadRaffle reads a raffle, or the 404 when there is none.
func (h handlers) loadRaffle(ctx context.Context, q *db.Queries, id int64) (Raffle, error) {
	row, err := q.GetRaffle(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Raffle{}, noSuchRaffle()
	}
	if err != nil {
		return Raffle{}, h.fail("read a raffle", err)
	}
	return toRaffle(row), nil
}

// loadVisibleRaffle reads a raffle the caller may see: any for the Raffles page's
// staff, otherwise one a public visitor could find. Anything else is a 404.
func (h handlers) loadVisibleRaffle(ctx context.Context, id int64) (Raffle, error) {
	r, err := h.loadRaffle(ctx, h.read(), id)
	if err != nil {
		return Raffle{}, err
	}
	if !raffleStaff(ctx) && !r.viewable(time.Now().UTC()) {
		return Raffle{}, noSuchRaffle()
	}
	return r, nil
}

func (h handlers) listRaffles(ctx context.Context, _ *struct{}) (*raffleListOutput, error) {
	q := h.read()
	rows, err := q.ListRaffles(ctx)
	if err != nil {
		return nil, h.fail("list raffles", err)
	}
	staff := raffleStaff(ctx)
	now := time.Now().UTC()
	out := RaffleList{Raffles: make([]Raffle, 0, len(rows))}
	for _, row := range rows {
		r := toRaffle((*db.GetRaffleRow)(row))
		if staff || r.open(now) {
			out.Raffles = append(out.Raffles, r)
		}
	}
	if staff {
		if err := h.addStaffTotals(ctx, q, out.Raffles); err != nil {
			return nil, h.fail("total the raffles", err)
		}
	}
	return &raffleListOutput{Body: out}, nil
}

// addStaffTotals fills in each raffle's winner and the gil it collected. Each entry's
// settlement is priced on its own by the raffle's entry mode, since a tiered raffle
// prices one entry of three tickets differently from three of one.
func (h handlers) addStaffTotals(ctx context.Context, q *db.Queries, list []Raffle) error {
	winners, err := q.ListRaffleWinnerNames(ctx)
	if err != nil {
		return err
	}
	names := make(map[int64]string, len(winners))
	for _, w := range winners {
		if w.CharacterName != "" {
			names[w.ID] = model.ParticipantLabel(w.CharacterName, w.World)
		}
	}
	settled, err := q.ListSettledRaffleEntries(ctx)
	if err != nil {
		return err
	}
	byRaffle := make(map[int][]raffles.Entry)
	index := make(map[int64]int, len(list))
	for i := range list {
		index[list[i].ID] = i
	}
	for _, e := range settled {
		if i, ok := index[e.RaffleID]; ok {
			byRaffle[i] = append(byRaffle[i], raffles.Entry{
				NumEntries: int(e.NumEntries), PaidEntries: int(e.PaidEntries), AmountWaived: raffles.Gil(e.AmountWaived),
			})
		}
	}
	for i := range list {
		r := &list[i]
		if r.WinnerEntryID != nil {
			if name, ok := names[*r.WinnerEntryID]; ok {
				r.WinnerName = &name
			}
		}
		var total int64
		for _, e := range byRaffle[i] {
			total += r.pricing().AmountCollected(e)
		}
		r.PaidTotal = &total
	}
	return nil
}

func (h handlers) getRaffle(ctx context.Context, in *raffleIDInput) (*raffleDetailOutput, error) {
	r, err := h.loadVisibleRaffle(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	q := h.read()
	tickets, err := q.CountRaffleTickets(ctx, in.ID)
	if err != nil {
		return nil, h.fail("count a raffle's tickets", err)
	}
	out := RaffleDetail{Raffle: r, TotalEntries: tickets}
	if r.Status == raffles.StatusClosed && r.WinnerEntryID != nil {
		// Fetched by id and cut down to the name, world and tickets: what the entry
		// paid and was let off is private.
		switch e, err := q.GetRaffleEntry(ctx, *r.WinnerEntryID); {
		case err == nil:
			out.WinnerEntry = &RaffleWinner{CharacterName: e.CharacterName, World: e.World, NumEntries: e.NumEntries}
		case !errors.Is(err, sql.ErrNoRows):
			return nil, h.fail("read a raffle's winner", err)
		}
	}
	return &raffleDetailOutput{Body: out}, nil
}

// columns checks a raffle's fields that the schema can't, and puts them as they are
// stored: the cost fields only its entry mode uses (so switching a raffle from custom
// back to single leaves no stale ladder behind as a shadow price), and in custom mode
// the allowance pinned to the ladder's length, because a ticket past the ladder would
// have no price.
func (h handlers) columns(in RaffleInput) (db.CreateRaffleParams, error) {
	mode := raffles.NormalizeMode(in.EntryMode)
	var problems []error
	if mode == raffles.ModeCustom {
		if len(in.TierCosts) == 0 {
			problems = append(problems, &huma.ErrorDetail{Location: "body.tierCosts", Value: in.TierCosts,
				Message: "Add at least one entry cost."})
		}
		for i, c := range in.TierCosts {
			if c < 0 {
				problems = append(problems, &huma.ErrorDetail{Location: "body.tierCosts[" + strconv.Itoa(i) + "]",
					Value: c, Message: "Cost for entry " + strconv.Itoa(i+1) + " must be 0 or more."})
			}
		}
	}
	var from, to *string
	for _, t := range []struct {
		field string
		in    *string
		out   **string
	}{{"availableFrom", in.AvailableFrom, &from}, {"availableTo", in.AvailableTo, &to}} {
		if t.in == nil {
			continue
		}
		n, err := store.NormalizeTime(*t.in)
		if err != nil {
			problems = append(problems, &huma.ErrorDetail{Location: "body." + t.field, Value: *t.in, Message: err.Error()})
			continue
		}
		if n != "" {
			*t.out = &n
		}
	}
	if len(problems) > 0 {
		return db.CreateRaffleParams{}, huma.Error422UnprocessableEntity("The raffle can't be saved as it is.", problems...)
	}

	p := db.CreateRaffleParams{
		Title: strings.TrimSpace(in.Title), Description: in.Description, Rules: in.Rules,
		MaxEntries: max(in.MaxEntries, 1), SignupInstructions: in.SignupInstructions, EntryMode: mode,
		TierCosts: "[]", AvailableFrom: from, AvailableTo: to, PrizeImage: in.PrizeImage,
		PayImage: strings.TrimSpace(in.PayImage), FestivalMapID: nullInt(in.FestivalMapID), OccupantID: nullInt(in.OccupantID),
	}
	switch mode {
	case raffles.ModeCustom:
		ladder, err := json.Marshal(in.TierCosts)
		if err != nil {
			return db.CreateRaffleParams{}, h.fail("encode a raffle's prices", err)
		}
		p.TierCosts = string(ladder)
		p.MaxEntries = int64(len(in.TierCosts))
	case raffles.ModeSingle:
		p.CostPerEntry = float64(in.CostPerEntry)
	}
	return p, nil
}

// resolveStall settles a raffle's festival and its assigned stall against what
// exists. A raffle naming no festival has its stall cleared, since a stall would
// claim a festival link it doesn't have; a festival that is gone is a 422; and a stall
// that isn't on the festival is dropped rather than failing the save, since the
// raffle is still a good raffle, only not pinned to a pitch.
func (h handlers) resolveStall(ctx context.Context, p *db.CreateRaffleParams) error {
	if !p.FestivalMapID.Valid {
		p.OccupantID = sql.NullInt64{}
		return nil
	}
	m, err := h.findFestivalMap(ctx, h.read(), p.FestivalMapID.Int64)
	if err != nil {
		return h.fail("read a festival for a raffle", err)
	}
	if m == nil {
		return unknown("body.festivalMapId", p.FestivalMapID.Int64, "festival")
	}
	if !p.OccupantID.Valid {
		return nil
	}
	for _, stall := range m.Stalls {
		for _, o := range stall.Occupants {
			if o.ID == p.OccupantID.Int64 {
				return nil
			}
		}
	}
	p.OccupantID = sql.NullInt64{}
	return nil
}

func (h handlers) createRaffle(ctx context.Context, in *raffleCreateInput) (*raffleCreatedOutput, error) {
	p, err := h.columns(in.Body)
	if err != nil {
		return nil, err
	}
	if err := h.resolveStall(ctx, &p); err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	id, err := h.write().CreateRaffle(ctx, p)
	if err != nil {
		return nil, h.fail("create a raffle", err)
	}
	r, err := h.loadRaffle(ctx, h.read(), id)
	if err != nil {
		return nil, err
	}
	return &raffleCreatedOutput{Status: http.StatusCreated, Body: r}, nil
}

func (h handlers) updateRaffle(ctx context.Context, in *raffleSaveInput) (*raffleOutput, error) {
	p, err := h.columns(in.Body)
	if err != nil {
		return nil, err
	}
	if err := h.resolveStall(ctx, &p); err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	n, err := h.write().UpdateRaffle(ctx, db.UpdateRaffleParams{
		Title: p.Title, Description: p.Description, Rules: p.Rules, MaxEntries: p.MaxEntries,
		SignupInstructions: p.SignupInstructions, EntryMode: p.EntryMode, CostPerEntry: p.CostPerEntry,
		TierCosts: p.TierCosts, AvailableFrom: p.AvailableFrom, AvailableTo: p.AvailableTo,
		PrizeImage: p.PrizeImage, PayImage: p.PayImage, FestivalMapID: p.FestivalMapID, OccupantID: p.OccupantID, ID: in.ID,
	})
	if err != nil {
		return nil, h.fail("update a raffle", err)
	}
	if n == 0 {
		return nil, noSuchRaffle()
	}
	r, err := h.loadRaffle(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	return &raffleOutput{Body: r}, nil
}

func (h handlers) deleteRaffle(ctx context.Context, in *raffleIDInput) (*struct{}, error) {
	n, err := h.write().DeleteRaffle(context.WithoutCancel(ctx), in.ID)
	if err != nil {
		return nil, h.fail("delete a raffle", err)
	}
	if n == 0 {
		return nil, noSuchRaffle()
	}
	return nil, nil
}

// setStatus closes or reopens a raffle and answers it.
func (h handlers) setStatus(ctx context.Context, id int64, status string) (*raffleOutput, error) {
	ctx = context.WithoutCancel(ctx)
	n, err := h.write().SetRaffleStatus(ctx, db.SetRaffleStatusParams{Status: status, ID: id})
	if err != nil {
		return nil, h.fail("set a raffle's status", err)
	}
	if n == 0 {
		return nil, noSuchRaffle()
	}
	r, err := h.loadRaffle(ctx, h.read(), id)
	if err != nil {
		return nil, err
	}
	return &raffleOutput{Body: r}, nil
}

func (h handlers) closeRaffle(ctx context.Context, in *raffleIDInput) (*raffleOutput, error) {
	return h.setStatus(ctx, in.ID, raffles.StatusClosed)
}

func (h handlers) reopenRaffle(ctx context.Context, in *raffleIDInput) (*raffleOutput, error) {
	return h.setStatus(ctx, in.ID, raffles.StatusOpen)
}

func (h handlers) listRaffleEntries(ctx context.Context, in *raffleIDInput) (*raffleEntryListOutput, error) {
	q := h.read()
	if _, err := h.loadRaffle(ctx, q, in.ID); err != nil {
		return nil, err
	}
	rows, err := q.ListRaffleEntries(ctx, in.ID)
	if err != nil {
		return nil, h.fail("list a raffle's entries", err)
	}
	out := RaffleEntryList{Entries: make([]RaffleEntry, 0, len(rows))}
	for _, e := range rows {
		out.Entries = append(out.Entries, toRaffleEntry(e))
	}
	return &raffleEntryListOutput{Body: out}, nil
}

// errEntryLimit ends an entry transaction: the tickets would pass the allowance.
var errEntryLimit = errors.New("over the allowance")

// addTickets adds tickets to a character's entry in a raffle, making the entry if
// it has none, inside the transaction q is bound to. The allowance is tested by
// subtraction, never by summing first: the count comes from a request, and summing a
// huge one with the tickets held would wrap negative, pass the test, and write a count
// no read can scan back. It answers the entry, the tickets held before, and whether
// the entry is new; past the allowance it answers errEntryLimit.
func addTickets(ctx context.Context, q *db.Queries, raffleID int64, name, world string, num, allowance int64) (entry *db.RaffleEntry, before int64, created bool, err error) {
	found, err := q.FindRaffleEntry(ctx, db.FindRaffleEntryParams{RaffleID: raffleID, CharacterName: name, World: world})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		created = true
	case err != nil:
		return nil, 0, false, err
	default:
		before = found.NumEntries
	}
	if num > allowance-before {
		return nil, before, created, errEntryLimit
	}
	if created {
		entry, err = q.CreateRaffleEntry(ctx, db.CreateRaffleEntryParams{
			RaffleID: raffleID, CharacterName: name, World: world, NumEntries: num,
		})
		return entry, 0, true, err
	}
	if err := q.AddRaffleEntryTickets(ctx, db.AddRaffleEntryTicketsParams{Tickets: num, ID: found.ID}); err != nil {
		return nil, 0, false, err
	}
	entry, err = q.GetRaffleEntry(ctx, found.ID)
	return entry, before, false, err
}

// overAllowance is the 409 for tickets past the allowance; who is "You" or "They".
func overAllowance(who string, num, before, allowance int64) error {
	if before > 0 {
		return huma.Error409Conflict(fmt.Sprintf("Cannot add %d entries. %s already have %d of %d max entries.",
			num, who, before, allowance))
	}
	return huma.Error409Conflict(fmt.Sprintf("Number of entries cannot exceed %d.", allowance))
}

func (h handlers) addRaffleEntry(ctx context.Context, in *raffleEntryAddInput) (*raffleEntryAddedOutput, error) {
	r, err := h.loadRaffle(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	if r.Status != raffles.StatusOpen {
		return nil, huma.Error409Conflict("This raffle is no longer accepting entries.")
	}
	body := in.Body
	name, world := strings.TrimSpace(body.CharacterName), strings.TrimSpace(body.World)

	// The tickets and their settlement are one write: a client that goes away between
	// them can't leave a paid-for entry recorded unpaid.
	ctx = context.WithoutCancel(ctx)
	var entry *db.RaffleEntry
	var before int64
	var created bool
	err = h.transact(ctx, func(q *db.Queries) error {
		var err error
		if entry, before, created, err = addTickets(ctx, q, in.ID, name, world, body.NumEntries, r.MaxEntries); err != nil {
			return err
		}
		if !body.Paid {
			return nil
		}
		// The tickets just added are outstanding by definition, so this always applies.
		if _, err := q.SettleAllRaffleEntryTickets(ctx, db.SettleAllRaffleEntryTicketsParams{
			Waived: float64(body.AmountWaived), ID: entry.ID,
		}); err != nil {
			return err
		}
		entry, err = q.GetRaffleEntry(ctx, entry.ID)
		return err
	})
	if errors.Is(err, errEntryLimit) {
		return nil, overAllowance("They", body.NumEntries, before, r.MaxEntries)
	}
	if err != nil {
		return nil, h.fail("add a raffle entry", err)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return &raffleEntryAddedOutput{Status: status, Body: toRaffleEntry(entry)}, nil
}

// loadEntry reads an entry that belongs to the raffle, or the 404: an entry of
// another raffle can't be reached by pairing its id with any raffle id.
func (h handlers) loadEntry(ctx context.Context, q *db.Queries, raffleID, entryID int64) (*db.RaffleEntry, error) {
	e, err := q.GetRaffleEntry(ctx, entryID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && e.RaffleID != raffleID) {
		return nil, noSuchRaffleEntry()
	}
	if err != nil {
		return nil, h.fail("read a raffle entry", err)
	}
	return e, nil
}

func (h handlers) settleRaffleEntry(ctx context.Context, in *raffleSettleInput) (*raffleEntryOutput, error) {
	if _, err := h.loadEntry(ctx, h.read(), in.ID, in.EntryID); err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	q := h.write()
	// A statement that settles nothing (already settled) is no error: the entry read
	// back below is the settlement that stuck.
	var err error
	if in.Body.PaidEntries == nil {
		_, err = q.SettleAllRaffleEntryTickets(ctx, db.SettleAllRaffleEntryTicketsParams{
			Waived: float64(in.Body.Waived), ID: in.EntryID,
		})
	} else {
		_, err = q.SettleSomeRaffleEntryTickets(ctx, db.SettleSomeRaffleEntryTicketsParams{
			Tickets: *in.Body.PaidEntries, Waived: float64(in.Body.Waived), ID: in.EntryID,
		})
	}
	if err != nil {
		return nil, h.fail("settle a raffle entry", err)
	}
	e, err := h.loadEntry(ctx, h.read(), in.ID, in.EntryID)
	if err != nil {
		return nil, err
	}
	return &raffleEntryOutput{Body: toRaffleEntry(e)}, nil
}

func (h handlers) unsettleRaffleEntry(ctx context.Context, in *raffleEntryIDInput) (*raffleEntryOutput, error) {
	if _, err := h.loadEntry(ctx, h.read(), in.ID, in.EntryID); err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	if _, err := h.write().ClearRaffleEntrySettlement(ctx, in.EntryID); err != nil {
		return nil, h.fail("clear a raffle entry's payment", err)
	}
	e, err := h.loadEntry(ctx, h.read(), in.ID, in.EntryID)
	if err != nil {
		return nil, err
	}
	return &raffleEntryOutput{Body: toRaffleEntry(e)}, nil
}

func (h handlers) deleteRaffleEntry(ctx context.Context, in *raffleEntryIDInput) (*struct{}, error) {
	if _, err := h.loadEntry(ctx, h.read(), in.ID, in.EntryID); err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	// The winner column is not an enforced foreign key, so the raffle is cleared of the
	// entry in the same transaction; staff can then pick again.
	err := h.transact(ctx, func(q *db.Queries) error {
		if err := q.ClearRaffleWinnerIfEntry(ctx, sql.NullInt64{Int64: in.EntryID, Valid: true}); err != nil {
			return err
		}
		_, err := q.DeleteRaffleEntry(ctx, in.EntryID)
		return err
	})
	if err != nil {
		return nil, h.fail("delete a raffle entry", err)
	}
	return nil, nil
}

// drawWinner draws an entry of a raffle, leaving out the entry skip (0 for none): one
// chance per ticket settled for a raffle that charges, per ticket held for a
// details-only one. It answers nil when there is nothing to draw from.
func drawWinner(ctx context.Context, q *db.Queries, r Raffle, skip int64) (*db.RaffleEntry, error) {
	entries, err := q.ListRaffleEntries(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	paidOnly := raffles.AcceptsSignups(r.EntryMode)
	weights := make([]int64, len(entries))
	for i, e := range entries {
		if e.ID != skip {
			weights[i] = int64(raffles.Tickets(int(e.NumEntries), int(e.PaidEntries), paidOnly))
		}
	}
	if i := raffles.Pick(weights, random.IntN); i >= 0 {
		return entries[i], nil
	}
	return nil, nil
}

func noEntriesToDraw(r Raffle) error {
	if raffles.AcceptsSignups(r.EntryMode) {
		return huma.Error409Conflict("There are no paid entries to pick from.")
	}
	return huma.Error409Conflict("There are no entries to pick from.")
}

var errPendingChanged = errors.New("the pending winner changed")

func pendingChanged() error {
	return huma.Error409Conflict("The pending winner isn't that entry any more; another console changed it. Reload and try again.")
}

func (h handlers) pickRaffleWinner(ctx context.Context, in *raffleIDInput) (*raffleEntryOutput, error) {
	r, err := h.loadRaffle(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	if r.WinnerEntryID != nil {
		return nil, huma.Error409Conflict("A winner is already picked. Confirm them, or pick another.")
	}
	winner, err := drawWinner(ctx, h.read(), r, 0)
	if err != nil {
		return nil, h.fail("draw a raffle winner", err)
	}
	if winner == nil {
		return nil, noEntriesToDraw(r)
	}
	n, err := h.write().PickRaffleWinner(context.WithoutCancel(ctx), db.PickRaffleWinnerParams{
		WinnerEntryID: sql.NullInt64{Int64: winner.ID, Valid: true}, ID: in.ID,
	})
	if err != nil {
		return nil, h.fail("record a raffle winner", err)
	}
	if n == 0 {
		return nil, huma.Error409Conflict("A winner is already picked. Confirm them, or pick another.")
	}
	return &raffleEntryOutput{Body: toRaffleEntry(winner)}, nil
}

func (h handlers) pickAnotherRaffleWinner(ctx context.Context, in *raffleChoiceInput) (*raffleEntryOutput, error) {
	// Clearing the pick and making the next go together, detached: stopping between
	// them would leave the raffle with its winner cleared and none drawn.
	ctx = context.WithoutCancel(ctx)
	var winner *db.RaffleEntry
	var r Raffle
	err := h.transact(ctx, func(q *db.Queries) error {
		var err error
		if r, err = h.loadRaffle(ctx, q, in.ID); err != nil {
			return err
		}
		if r.WinnerEntryID == nil || *r.WinnerEntryID != in.Body.Replaces {
			return errPendingChanged
		}
		if err := q.ClearRaffleWinner(ctx, in.ID); err != nil {
			return err
		}
		if winner, err = drawWinner(ctx, q, r, in.Body.Replaces); err != nil || winner == nil {
			// Nothing else to draw leaves the winner cleared, which commits.
			return err
		}
		_, err = q.PickRaffleWinner(ctx, db.PickRaffleWinnerParams{
			WinnerEntryID: sql.NullInt64{Int64: winner.ID, Valid: true}, ID: in.ID,
		})
		return err
	})
	var problem huma.StatusError
	switch {
	case errors.Is(err, errPendingChanged):
		return nil, pendingChanged()
	case errors.As(err, &problem):
		return nil, err
	case err != nil:
		return nil, h.fail("draw another raffle winner", err)
	case winner == nil:
		return nil, huma.Error409Conflict("There is no other entry to pick from; the raffle has no pending winner.")
	}
	return &raffleEntryOutput{Body: toRaffleEntry(winner)}, nil
}

func (h handlers) verifyRaffleWinner(ctx context.Context, in *raffleConfirmInput) (*raffleOutput, error) {
	r, err := h.loadRaffle(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	switch {
	case r.WinnerEntryID == nil:
		return nil, huma.Error409Conflict("No winner is picked to confirm.")
	case *r.WinnerEntryID != in.Body.WinnerEntryID:
		return nil, pendingChanged()
	}
	return h.setStatus(ctx, in.ID, raffles.StatusClosed)
}

func (h handlers) enterRaffle(ctx context.Context, in *raffleEnterRequest) (*raffleEnterOutput, error) {
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.RaffleEntryLimit, ip); wait > 0 {
		h.Logger.Warn("raffle entry held back", "ip", ip)
		return nil, tooMany(wait, "Too many entries from here. Try again later.")
	}
	// Every try counts, taken in or not, so nobody can flood a raffle's entries.
	if h.RaffleEntryLimit != nil {
		h.RaffleEntryLimit.Fail(ip)
	}
	// Entries weight the winner pick, so bot-flooded ones would skew the odds.
	if !h.turnstile(ctx, in.Body.TurnstileToken, ip) {
		h.Logger.Warn("turnstile check failed", "ip", ip, "where", "raffle entry")
		return nil, huma.Error403Forbidden("The bot check didn't pass. Complete it and try again.")
	}
	r, err := h.loadRaffle(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	switch {
	case r.Status != raffles.StatusOpen:
		return nil, huma.Error409Conflict("This raffle is no longer accepting entries.")
	// A details-only raffle publishes its instructions and takes sign-ups elsewhere, so
	// a request here is a client out of step with it.
	case !raffles.AcceptsSignups(r.EntryMode):
		return nil, huma.Error409Conflict("This raffle does not take sign-ups here. Follow its sign-up instructions.")
	case !raffles.InWindow(deref(r.AvailableFrom), deref(r.AvailableTo), now):
		if from, ok := raffles.ParseTime(deref(r.AvailableFrom)); ok && now.Before(from) {
			return nil, huma.Error409Conflict("This raffle is not yet open for entries.")
		}
		return nil, huma.Error409Conflict("This raffle is no longer accepting entries.")
	}

	name, world := strings.TrimSpace(in.Body.CharacterName), strings.TrimSpace(in.Body.World)
	// The tickets are added atomically against the allowance, so two sign-ups for one
	// character can't both pass a stale count and exceed it.
	ctx = context.WithoutCancel(ctx)
	var entry *db.RaffleEntry
	var before int64
	var created bool
	err = h.transact(ctx, func(q *db.Queries) error {
		var err error
		entry, before, created, err = addTickets(ctx, q, in.ID, name, world, in.Body.NumEntries, r.MaxEntries)
		return err
	})
	if errors.Is(err, errEntryLimit) {
		return nil, overAllowance("You", in.Body.NumEntries, before, r.MaxEntries)
	}
	if err != nil {
		return nil, h.fail("record a raffle entry", err)
	}

	// What they owe is the sticker price less what this character has settled here, so a
	// returning entrant isn't quoted for tickets already paid for. It folds settled
	// tickets and waived gil together, so it can't be used to work out that a
	// character was comped.
	pricing := r.pricing()
	held := toRaffleEntry(entry)
	out := RaffleEnterResult{
		Message: "Entries added successfully", TotalEntries: held.NumEntries,
		TotalCost: pricing.EntryCost(int(held.NumEntries)), AmountDue: pricing.AmountOutstanding(held.settlement()),
		SignupInstructions: r.SignupInstructions,
	}
	status := http.StatusOK
	if created {
		out.Message, status = "Signed up successfully", http.StatusCreated
	}
	// The server's write middleware leaves the public sign-up out, so tell the page.
	ResourceChanged(h.Live, "raffles")
	return &raffleEnterOutput{Status: status, Body: out}, nil
}

func (h handlers) lookUpRaffleEntries(ctx context.Context, in *raffleLookupRequest) (*raffleLookupOutput, error) {
	// Its own limit, not the sign-up's: a search is what somebody does right before
	// signing up, and sharing a budget would lock them out of the entry they searched to
	// get right.
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.RaffleLookupLimit, ip); wait > 0 {
		h.Logger.Warn("raffle lookup held back", "ip", ip)
		return nil, tooMany(wait, "Too many searches from here. Try again later.")
	}
	if h.RaffleLookupLimit != nil {
		h.RaffleLookupLimit.Fail(ip)
	}
	name := strings.TrimSpace(in.Body.Name)
	// Entrants search for their own name, which they know; one character is a request
	// for the entrant list.
	if utf8.RuneCountInString(name) < lookupMinQuery {
		return nil, huma.Error422UnprocessableEntity("Enter more of the name you signed up with.", &huma.ErrorDetail{
			Location: "body.name", Value: in.Body.Name,
			Message: fmt.Sprintf("Enter at least %d characters of the name you signed up with.", lookupMinQuery),
		})
	}
	// Only a raffle the caller could already see: a hidden one must not become
	// searchable because its id was guessed.
	if _, err := h.loadVisibleRaffle(ctx, in.ID); err != nil {
		return nil, err
	}
	rows, err := h.read().SearchRaffleEntries(ctx, db.SearchRaffleEntriesParams{
		RaffleID: in.ID, Search: name, RowLimit: lookupLimit + 1,
	})
	if err != nil {
		return nil, h.fail("search a raffle's entries", err)
	}
	out := RaffleLookup{Entries: make([]RaffleLookupEntry, 0, min(len(rows), lookupLimit))}
	for i, e := range rows {
		if i == lookupLimit {
			out.Truncated = true
			break
		}
		out.Entries = append(out.Entries, RaffleLookupEntry{
			CharacterName: e.CharacterName, World: e.World, NumEntries: e.NumEntries, PaidEntries: e.PaidEntries,
			PaymentState: raffles.PaymentState(int(e.NumEntries), int(e.PaidEntries)),
		})
	}
	return &raffleLookupOutput{Body: out}, nil
}
