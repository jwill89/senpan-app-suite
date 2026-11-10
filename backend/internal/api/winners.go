package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/db"
	"app-suite/internal/permissions"
)

// The winners log: every winning card a game's end confirmed (the game writes it),
// which staff page through, prune, and watch for players who win often.

var winnersTags = []string{"Winners log"}

// WinnersLogEntry is one confirmed win.
type WinnersLogEntry struct {
	ID              int64   `json:"id" doc:"The entry's id."`
	LoggedAt        *string `json:"loggedAt" nullable:"true" doc:"When the game it was won in ended, RFC 3339 in UTC."`
	CardID          string  `json:"cardId" doc:"The winning card's id."`
	PlayerName      string  `json:"playerName" doc:"The player the card was issued to then; empty for none."`
	GameDetails     string  `json:"gameDetails" doc:"The game's details (Markdown) then."`
	WinningPatterns string  `json:"winningPatterns" doc:"The names of the game's win patterns, as a JSON array of strings."`
}

// WinnersLogPage is a page of the log.
type WinnersLogPage struct {
	Entries []WinnersLogEntry `json:"entries" nullable:"false" doc:"The page's entries, in the order asked for."`
	Total   int64             `json:"total" doc:"How many entries the whole log holds."`
	Page    int64             `json:"page" doc:"The page's number, from 1."`
	PerPage int64             `json:"perPage" doc:"The most entries a page holds."`
}

// FrequentWinner is a player who has won often lately.
type FrequentWinner struct {
	PlayerName string `json:"playerName" doc:"The player's name, as their cards gave it."`
	WinCount   int64  `json:"winCount" doc:"How many wins the log holds for them in the window."`
}

// FrequentWinnerList is the players who have won often lately.
type FrequentWinnerList struct {
	Winners []FrequentWinner `json:"winners" nullable:"false" doc:"The players with at least the settings' frequentWinnerThreshold wins in the last frequentWinnerHours hours, most wins first."`
}

// DeletedCount is what a delete of everything took away.
type DeletedCount struct {
	Deleted int64 `json:"deleted" doc:"How many were deleted."`
}

type winnersInput struct {
	Page    int64  `query:"page" default:"1" minimum:"1" maximum:"1000000" doc:"Which page, from 1. A page past the end is empty, with the log's total."`
	PerPage int64  `query:"perPage" default:"25" minimum:"1" maximum:"200" doc:"The most entries a page holds."`
	Sort    string `query:"sort" default:"loggedAt" enum:"loggedAt,cardId,playerName,gameDetails" doc:"The field the log is sorted by."`
	Dir     string `query:"dir" default:"desc" enum:"asc,desc" doc:"The sort's direction."`
}

type winnerIDInput struct {
	ID int64 `path:"id" doc:"The entry's id."`
}

type winnersOutput struct{ Body WinnersLogPage }

type frequentWinnersOutput struct{ Body FrequentWinnerList }

type deletedCountOutput struct{ Body DeletedCount }

func (h handlers) registerWinners(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.BingoWinnersLog)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}

	huma.Register(api, huma.Operation{
		OperationID: "list-winners", Method: http.MethodGet, Path: "/winners-log",
		Summary: "A page of the winners log", Tags: winnersTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.listWinners)
	huma.Register(api, huma.Operation{
		OperationID: "list-frequent-winners", Method: http.MethodGet, Path: "/winners-log/frequent",
		Summary: "The players who have won often lately",
		Description: "The window and the number of wins are the settings' frequentWinnerHours and " +
			"frequentWinnerThreshold.",
		Tags: winnersTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.listFrequentWinners)
	huma.Register(api, huma.Operation{
		OperationID: "delete-all-winners", Method: http.MethodDelete, Path: "/winners-log/all",
		Summary: "Clear the winners log", Tags: winnersTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.deleteAllWinners)
	huma.Register(api, huma.Operation{
		OperationID: "delete-winner", Method: http.MethodDelete, Path: "/winners-log/{id}",
		Summary: "Delete a winners-log entry", Tags: winnersTags, DefaultStatus: http.StatusNoContent,
		Security: secured, Middlewares: page, Errors: append(guarded, http.StatusNotFound),
	}, h.deleteWinner)
}

func (h handlers) listWinners(ctx context.Context, in *winnersInput) (*winnersOutput, error) {
	q := h.read()
	total, err := q.CountWinners(ctx)
	if err != nil {
		return nil, h.fail("count the winners log", err)
	}
	rows, err := q.ListWinners(ctx, db.ListWinnersParams{
		Sort: in.Sort, Dir: in.Dir, PageSize: in.PerPage, Skip: (in.Page - 1) * in.PerPage,
	})
	if err != nil {
		return nil, h.fail("list the winners log", err)
	}
	out := WinnersLogPage{Entries: make([]WinnersLogEntry, 0, len(rows)), Total: total, Page: in.Page, PerPage: in.PerPage}
	for _, r := range rows {
		out.Entries = append(out.Entries, WinnersLogEntry{
			ID: r.ID, LoggedAt: r.LoggedAt, CardID: r.CardID, PlayerName: r.PlayerName,
			GameDetails: r.GameDetails, WinningPatterns: r.WinningPatterns,
		})
	}
	return &winnersOutput{Body: out}, nil
}

func (h handlers) listFrequentWinners(ctx context.Context, _ *struct{}) (*frequentWinnersOutput, error) {
	q := h.read()
	s, err := loadSettings(ctx, q)
	if err != nil {
		return nil, h.fail("load settings", err)
	}
	since := time.Now().UTC().Add(-time.Duration(s.FrequentWinnerHours) * time.Hour).Format(time.RFC3339)
	rows, err := q.ListFrequentWinners(ctx, db.ListFrequentWinnersParams{Since: &since, MinWins: int64(s.FrequentWinnerThreshold)})
	if err != nil {
		return nil, h.fail("list frequent winners", err)
	}
	out := FrequentWinnerList{Winners: make([]FrequentWinner, 0, len(rows))}
	for _, r := range rows {
		out.Winners = append(out.Winners, FrequentWinner{PlayerName: r.PlayerName, WinCount: r.WinCount})
	}
	return &frequentWinnersOutput{Body: out}, nil
}

func (h handlers) deleteAllWinners(ctx context.Context, _ *struct{}) (*deletedCountOutput, error) {
	n, err := h.write().DeleteAllWinners(ctx)
	if err != nil {
		return nil, h.fail("clear the winners log", err)
	}
	return &deletedCountOutput{Body: DeletedCount{Deleted: n}}, nil
}

func (h handlers) deleteWinner(ctx context.Context, in *winnerIDInput) (*struct{}, error) {
	n, err := h.write().DeleteWinner(ctx, in.ID)
	if err != nil {
		return nil, h.fail("delete a winners-log entry", err)
	}
	if n == 0 {
		return nil, huma.Error404NotFound("There is no such winners-log entry.")
	}
	return nil, nil
}
