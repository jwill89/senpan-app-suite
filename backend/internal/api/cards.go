package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/bingo"
	"app-suite/internal/db"
	"app-suite/internal/live"
	"app-suite/internal/permissions"
)

// Bingo cards: the Manage Cards page's list, making cards one at a time or in bulk,
// staff's edits, approving a personal card someone asked for, protecting a card from
// Delete all, and deleting. Anyone may ask for a personal card (request-card); it
// waits for staff to approve it before it plays. Every change reaches the staff with
// the page as a cards_changed event, and the boards of a deleted card are sent
// card_deleted, which ends their stream.

var cardTags = []string{"Cards"}

// CardBoard is a card's numbers, row by row from the top: column B holds 1-15, I
// 16-30, N 31-45, G 46-60 and O 61-75, and the centre is 0, the free space.
type CardBoard [][]int

// Schema is exactly five rows of exactly five integers from 0 to 75.
func (CardBoard) Schema(huma.Registry) *huma.Schema {
	five, lowest, highest := 5, 0.0, 75.0
	return &huma.Schema{
		Type: huma.TypeArray, MinItems: &five, MaxItems: &five,
		Items: &huma.Schema{
			Type: huma.TypeArray, MinItems: &five, MaxItems: &five,
			Items: &huma.Schema{Type: huma.TypeInteger, Minimum: &lowest, Maximum: &highest},
		},
	}
}

// CardSummary is a card without its numbers, as the Manage Cards list shows it.
type CardSummary struct {
	ID           string  `json:"id" doc:"The card's id: six letters and digits, upper-case."`
	PlayerName   string  `json:"playerName" doc:"The player it is issued to, or the character who asked for a personal card; empty for none."`
	Details      string  `json:"details" doc:"Staff's note on the cardholder; empty for none."`
	CreatedAt    *string `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC; null when no time was recorded for it."`
	Protected    bool    `json:"protected" doc:"Whether Delete all spares it. Approving a personal card protects it."`
	CustomStatus string  `json:"customStatus" enum:",pending,approved" doc:"A personal card's state: pending until staff approve it (it can't be played until then), then approved; empty for a generated card."`
	World        string  `json:"world" doc:"The home world of the character who asked for a personal card; empty otherwise."`
}

// Card is a card with its numbers.
type Card struct {
	CardSummary
	BoardData CardBoard `json:"boardData" doc:"Its numbers, row by row from the top: column B holds 1-15, I 16-30, N 31-45, G 46-60 and O 61-75, and the centre is 0, the free space."`
}

// CardList is every card.
type CardList struct {
	Cards []CardSummary `json:"cards" nullable:"false" doc:"Every card, by id."`
}

// CardBatch is the cards a bulk generate made.
type CardBatch struct {
	Cards []Card `json:"cards" nullable:"false" doc:"The new cards, unassigned."`
	Count int    `json:"count" doc:"How many were made."`
}

// CardInput is a new card.
type CardInput struct {
	PlayerName string `json:"playerName,omitempty" maxLength:"100" doc:"The player to issue it to; empty or left out for none."`
}

// CardBatchInput is how many cards to make.
type CardBatchInput struct {
	Count int `json:"count" minimum:"1" maximum:"500" doc:"How many cards to make, from 1 to 500."`
}

// CardChanges is what staff change on a card: only what is sent.
type CardChanges struct {
	PlayerName *string `json:"playerName,omitempty" maxLength:"100" doc:"The player it is issued to; empty for none."`
	Details    *string `json:"details,omitempty" maxLength:"1000" doc:"Staff's note on the cardholder; empty for none."`
}

// CardProtection is whether Delete all spares a card.
type CardProtection struct {
	Protected bool `json:"protected" doc:"Whether Delete all spares it."`
}

// CardRequestInput is a personal card someone asks for: a board they built, with an
// id they chose.
type CardRequestInput struct {
	CharacterName  string    `json:"characterName" minLength:"1" maxLength:"60" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The name of the character asking for it."`
	World          string    `json:"world" minLength:"1" maxLength:"60" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The character's home world."`
	CardID         string    `json:"cardId" pattern:"^[A-Za-z0-9]{6}$" patternDescription:"six letters or digits" doc:"The id they chose: six letters or digits, stored upper-case. One another card has is refused (409)."`
	BoardData      CardBoard `json:"boardData" doc:"The numbers they chose, row by row from the top: column B holds 1-15, I 16-30, N 31-45, G 46-60 and O 61-75, none twice in a column, and the centre is 0, the free space. Numbers another card has, in the same cells, are refused (409)."`
	TurnstileToken string    `json:"turnstileToken,omitempty" maxLength:"2048" doc:"The Cloudflare Turnstile response, when the check is on (GET /config gives its site key)."`
}

// CardRequestReceipt says a personal card was taken in.
type CardRequestReceipt struct {
	ID     string `json:"id" doc:"The card's id, upper-case."`
	Status string `json:"status" enum:"pending" doc:"Always pending: the card plays once staff approve it."`
}

type cardIDInput struct {
	ID string `path:"id" pattern:"^[A-Za-z0-9]{6}$" patternDescription:"six letters or digits" doc:"The card's id, in any case."`
}

type cardCreateInput struct {
	Body CardInput
}

type cardGenerateInput struct {
	Body CardBatchInput
}

type cardUpdateInput struct {
	ID   string `path:"id" pattern:"^[A-Za-z0-9]{6}$" patternDescription:"six letters or digits" doc:"The card's id, in any case."`
	Body CardChanges
}

type cardProtectInput struct {
	ID   string `path:"id" pattern:"^[A-Za-z0-9]{6}$" patternDescription:"six letters or digits" doc:"The card's id, in any case."`
	Body CardProtection
}

type cardRequestInput struct {
	Body CardRequestInput
}

type cardOutput struct{ Body Card }

type cardCreatedOutput struct {
	Status int
	Body   Card
}

type cardBatchOutput struct {
	Status int
	Body   CardBatch
}

type cardListOutput struct{ Body CardList }

type cardRequestOutput struct {
	Status int
	Body   CardRequestReceipt
}

func (h handlers) registerCards(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.BingoCards)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}
	writes := "Staff with the page hear of it on their live stream (cards_changed)."

	huma.Register(api, huma.Operation{
		OperationID: "list-cards", Method: http.MethodGet, Path: "/cards",
		Summary: "Every card", Description: "Without their numbers: get-card has them.",
		Tags: cardTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.listCards)
	huma.Register(api, huma.Operation{
		OperationID: "get-card", Method: http.MethodGet, Path: "/cards/{id}",
		Summary: "A card with its numbers", Description: "A pending personal card too, for staff to review.",
		Tags: cardTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusUnprocessableEntity),
	}, h.getCard)
	huma.Register(api, huma.Operation{
		OperationID: "create-card", Method: http.MethodPost, Path: "/cards",
		Summary: "Make a card", Description: "A card with random numbers and a new id, issued to a player if one is named. " + writes,
		Tags: cardTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.createCard)
	huma.Register(api, huma.Operation{
		OperationID: "generate-cards", Method: http.MethodPost, Path: "/cards/generate",
		Summary: "Make cards in bulk",
		Description: "Unassigned cards with random numbers and new ids. Always makes the number asked for, " +
			"so it always answers 201. " + writes,
		Tags: cardTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.generateCards)
	huma.Register(api, huma.Operation{
		OperationID: "request-card", Method: http.MethodPost, Path: "/cards/request",
		Summary: "Ask for a personal card",
		Description: "Anyone may ask: the card is kept, pending, and plays once staff approve it (approve-card). " +
			"An id or numbers another card has are refused (409). Ten tries from one address in ten minutes, " +
			"made or not (429, with Retry-After). " + writes,
		Tags: cardTags, DefaultStatus: http.StatusAccepted,
		Errors: []int{http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusTooManyRequests},
	}, h.requestCard)
	huma.Register(api, huma.Operation{
		OperationID: "update-card", Method: http.MethodPatch, Path: "/cards/{id}",
		Summary: "Change a card's player or note", Description: writes,
		Tags: cardTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusUnprocessableEntity),
	}, h.updateCard)
	huma.Register(api, huma.Operation{
		OperationID: "approve-card", Method: http.MethodPost, Path: "/cards/{id}/approve",
		Summary: "Approve a personal card",
		Description: "A pending personal card becomes playable, and protected. A card that isn't pending is " +
			"refused (409). " + writes,
		Tags: cardTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity),
	}, h.approveCard)
	huma.Register(api, huma.Operation{
		OperationID: "protect-card", Method: http.MethodPost, Path: "/cards/{id}/protect",
		Summary: "Protect a card from Delete all, or stop protecting it", Description: writes,
		Tags: cardTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusUnprocessableEntity),
	}, h.protectCard)
	huma.Register(api, huma.Operation{
		OperationID: "delete-all-cards", Method: http.MethodDelete, Path: "/cards/all",
		Summary: "Delete every unprotected card",
		Description: "Protected cards are kept. The boards of a deleted card are sent card_deleted, which " +
			"ends their stream. " + writes,
		Tags: cardTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.deleteAllCards)
	huma.Register(api, huma.Operation{
		OperationID: "delete-card", Method: http.MethodDelete, Path: "/cards/{id}",
		Summary: "Delete a card", Description: "Protected or not. Its boards are sent card_deleted, which ends " +
			"their stream. " + writes,
		Tags: cardTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusUnprocessableEntity),
	}, h.deleteCard)
}

// errNoCard, errNotPending, errCardIDTaken and errSameBoard end a card's transaction
// and say why.
var (
	errNoCard      = errors.New("no such card")
	errNotPending  = errors.New("not a pending card")
	errCardIDTaken = errors.New("card id taken")
	errSameBoard   = errors.New("another card has these numbers")
)

// cardKey is a card's id as it is stored: upper-case.
func cardKey(id string) string { return strings.ToUpper(id) }

func noSuchCard() error { return huma.Error404NotFound("There is no such card.") }

func toCardSummary(id, playerName, details string, createdAt *string, protected int64, customStatus, world string) CardSummary {
	return CardSummary{
		ID: id, PlayerName: playerName, Details: details, CreatedAt: createdAt,
		Protected: protected != 0, CustomStatus: customStatus, World: world,
	}
}

func toCard(r *db.Card) (Card, error) {
	c := Card{CardSummary: toCardSummary(r.ID, r.PlayerName, r.Details, r.CreatedAt, r.Protected, r.CustomStatus, r.World)}
	if err := json.Unmarshal([]byte(r.BoardData), &c.BoardData); err != nil {
		return Card{}, fmt.Errorf("card %s's numbers: %w", r.ID, err)
	}
	return c, nil
}

// cardList reads every card.
func cardList(ctx context.Context, q *db.Queries) (CardList, error) {
	rows, err := q.ListCardSummaries(ctx)
	if err != nil {
		return CardList{}, err
	}
	out := CardList{Cards: make([]CardSummary, 0, len(rows))}
	for _, r := range rows {
		out.Cards = append(out.Cards, toCardSummary(r.ID, r.PlayerName, r.Details, r.CreatedAt, r.Protected, r.CustomStatus, r.World))
	}
	return out, nil
}

// cardAnswer turns a card a query read or wrote into the answer, or the 404 or 500
// its error calls for.
func (h handlers) cardAnswer(r *db.Card, err error) (Card, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, noSuchCard()
	}
	if err != nil {
		return Card{}, h.fail("read or write a card", err)
	}
	c, err := toCard(r)
	if err != nil {
		return Card{}, h.fail("read a card", err)
	}
	return c, nil
}

// makeCard stores a card with random numbers and an id no card has, issued to
// player (empty for none).
func makeCard(ctx context.Context, q *db.Queries, player string) (Card, error) {
	id, err := bingo.GenerateID(func(id string) (bool, error) { return q.CardExists(ctx, id) })
	if err != nil {
		return Card{}, err
	}
	board, err := json.Marshal(bingo.GenerateBoard())
	if err != nil {
		return Card{}, err
	}
	r, err := q.CreateCard(ctx, db.CreateCardParams{ID: id, BoardData: string(board), PlayerName: player})
	if err != nil {
		return Card{}, err
	}
	return toCard(r)
}

// cardsChanged tells the game the cards changed, and the staff with the Manage Cards
// page, who read them again. The list names who holds which card, so nobody else
// hears of it.
func (h handlers) cardsChanged() {
	if h.CardsChanged != nil {
		h.CardsChanged()
	}
	h.publish(&CardsChangedEvent{}, toPage(permissions.BingoCards))
}

// cardsGone tells the boards of these deleted cards, whose streams then end.
func (h handlers) cardsGone(ids []string) {
	for _, id := range ids {
		h.publish(&CardDeletedEvent{}, live.Audience{Card: id})
	}
}

func (h handlers) listCards(ctx context.Context, _ *struct{}) (*cardListOutput, error) {
	list, err := cardList(ctx, h.read())
	if err != nil {
		return nil, h.fail("list cards", err)
	}
	return &cardListOutput{Body: list}, nil
}

func (h handlers) getCard(ctx context.Context, in *cardIDInput) (*cardOutput, error) {
	c, err := h.cardAnswer(h.read().GetCard(ctx, cardKey(in.ID)))
	if err != nil {
		return nil, err
	}
	return &cardOutput{Body: c}, nil
}

func (h handlers) createCard(ctx context.Context, in *cardCreateInput) (*cardCreatedOutput, error) {
	ctx = context.WithoutCancel(ctx) // a card made is told of, whether or not the caller waits
	var c Card
	err := h.transact(ctx, func(q *db.Queries) (err error) {
		c, err = makeCard(ctx, q, strings.TrimSpace(in.Body.PlayerName))
		return err
	})
	if err != nil {
		return nil, h.fail("make a card", err)
	}
	h.cardsChanged()
	return &cardCreatedOutput{Status: http.StatusCreated, Body: c}, nil
}

func (h handlers) generateCards(ctx context.Context, in *cardGenerateInput) (*cardBatchOutput, error) {
	ctx = context.WithoutCancel(ctx)
	var made []Card
	// One transaction: each new id is checked against the cards made before it in the
	// same batch as well as the stored ones, and the batch lands whole or not at all.
	err := h.transact(ctx, func(q *db.Queries) error {
		made = make([]Card, 0, in.Body.Count)
		for range in.Body.Count {
			c, err := makeCard(ctx, q, "")
			if err != nil {
				return err
			}
			made = append(made, c)
		}
		return nil
	})
	if err != nil {
		return nil, h.fail("make cards", err)
	}
	h.cardsChanged()
	return &cardBatchOutput{Status: http.StatusCreated, Body: CardBatch{Cards: made, Count: len(made)}}, nil
}

// boardProblem is the 422 for a board that isn't a bingo card, naming its cell.
func boardProblem(board CardBoard, p *bingo.BoardError) error {
	location, value := "body.boardData", any(board)
	if p.Row >= 0 {
		location, value = fmt.Sprintf("%s[%d]", location, p.Row), board[p.Row]
		if p.Col >= 0 {
			location, value = fmt.Sprintf("%s[%d]", location, p.Col), board[p.Row][p.Col]
		}
	}
	return huma.Error422UnprocessableEntity(p.Reason, &huma.ErrorDetail{Location: location, Value: value, Message: p.Reason})
}

func (h handlers) requestCard(ctx context.Context, in *cardRequestInput) (*cardRequestOutput, error) {
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.CardRequestLimit, ip); wait > 0 {
		h.Logger.Warn("card request held back", "ip", ip)
		return nil, tooMany(wait, "Too many card requests from here. Try again later.")
	}
	// Every try counts, taken in or not, so nobody can fill the cards with requests.
	if h.CardRequestLimit != nil {
		h.CardRequestLimit.Fail(ip)
	}
	if !h.turnstile(ctx, in.Body.TurnstileToken, ip) {
		h.Logger.Warn("turnstile check failed", "ip", ip, "where", "card request")
		return nil, huma.Error403Forbidden("The bot check didn't pass. Complete it and try again.")
	}
	req := in.Body
	if p := bingo.ValidateBoard(req.BoardData); p != nil {
		return nil, boardProblem(req.BoardData, p)
	}
	board, err := json.Marshal(req.BoardData)
	if err != nil {
		return nil, h.fail("encode a card's numbers", err)
	}
	id := cardKey(req.CardID)
	ctx = context.WithoutCancel(ctx)
	// One transaction, so two requests for the same id or numbers can't both pass.
	err = h.transact(ctx, func(q *db.Queries) error {
		taken, err := q.CardExists(ctx, id)
		if err != nil {
			return err
		}
		if taken {
			return errCardIDTaken
		}
		if _, err := q.FindCardByBoard(ctx, string(board)); err == nil {
			return errSameBoard
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return q.CreateCardRequest(ctx, db.CreateCardRequestParams{
			ID: id, BoardData: string(board), PlayerName: strings.TrimSpace(req.CharacterName), World: strings.TrimSpace(req.World),
		})
	})
	switch {
	case errors.Is(err, errCardIDTaken):
		return nil, huma.Error409Conflict("That card ID is taken. Choose another.")
	case errors.Is(err, errSameBoard):
		return nil, huma.Error409Conflict("A card with exactly these numbers already exists. Change some of them.")
	case err != nil:
		return nil, h.fail("take in a card request", err)
	}
	h.Logger.Info("personal card requested", "card_id", id)
	h.cardsChanged()
	return &cardRequestOutput{Status: http.StatusAccepted, Body: CardRequestReceipt{ID: id, Status: "pending"}}, nil
}

// optional is a change to a text column: null (leave it) when it isn't sent.
func optional(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.TrimSpace(*s), Valid: true}
}

func (h handlers) updateCard(ctx context.Context, in *cardUpdateInput) (*cardOutput, error) {
	ctx = context.WithoutCancel(ctx)
	c, err := h.cardAnswer(h.write().UpdateCard(ctx, db.UpdateCardParams{
		PlayerName: optional(in.Body.PlayerName), Details: optional(in.Body.Details), ID: cardKey(in.ID),
	}))
	if err != nil {
		return nil, err
	}
	h.cardsChanged()
	return &cardOutput{Body: c}, nil
}

func (h handlers) approveCard(ctx context.Context, in *cardIDInput) (*cardOutput, error) {
	id := cardKey(in.ID)
	ctx = context.WithoutCancel(ctx)
	var row *db.Card
	err := h.transact(ctx, func(q *db.Queries) error {
		current, err := q.GetCard(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return errNoCard
		}
		if err != nil {
			return err
		}
		if current.CustomStatus != "pending" {
			return errNotPending
		}
		row, err = q.ApproveCard(ctx, id)
		return err
	})
	switch {
	case errors.Is(err, errNoCard):
		return nil, noSuchCard()
	case errors.Is(err, errNotPending):
		return nil, huma.Error409Conflict("Card " + id + " isn't waiting for approval.")
	}
	c, err := h.cardAnswer(row, err)
	if err != nil {
		return nil, err
	}
	h.cardsChanged()
	return &cardOutput{Body: c}, nil
}

func (h handlers) protectCard(ctx context.Context, in *cardProtectInput) (*cardOutput, error) {
	ctx = context.WithoutCancel(ctx)
	c, err := h.cardAnswer(h.write().SetCardProtected(ctx, db.SetCardProtectedParams{
		Protected: boolInt(in.Body.Protected), ID: cardKey(in.ID),
	}))
	if err != nil {
		return nil, err
	}
	h.cardsChanged()
	return &cardOutput{Body: c}, nil
}

func (h handlers) deleteCard(ctx context.Context, in *cardIDInput) (*struct{}, error) {
	id := cardKey(in.ID)
	ctx = context.WithoutCancel(ctx)
	n, err := h.write().DeleteCard(ctx, id)
	if err != nil {
		return nil, h.fail("delete a card", err)
	}
	if n == 0 {
		return nil, noSuchCard()
	}
	h.cardsChanged()
	h.cardsGone([]string{id})
	return nil, nil
}

func (h handlers) deleteAllCards(ctx context.Context, _ *struct{}) (*deletedCountOutput, error) {
	ctx = context.WithoutCancel(ctx)
	ids, err := h.write().DeleteUnprotectedCards(ctx)
	if err != nil {
		return nil, h.fail("delete every unprotected card", err)
	}
	if len(ids) > 0 {
		h.cardsChanged()
		h.cardsGone(ids)
	}
	return &deletedCountOutput{Body: DeletedCount{Deleted: int64(len(ids))}}, nil
}
