package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/game"
	"app-suite/internal/permissions"
)

// A player's board: their card, by its id, with the game running as players see it.
// The id is all a player has, so it is public; a personal card still waiting for
// staff to approve it is refused to anyone but the staff who review it.

var boardTags = []string{"Board"}

// Board is a card and the game it plays in.
type Board struct {
	Card        Card       `json:"card" doc:"The card, with its numbers."`
	Game        *game.Game `json:"game" doc:"The game running, as players see it: its numbers once their draw delay has passed (staff with the Game page see them at once), and whether players were told about a half-time mini-game. Null when no game is running, and in a preview."`
	GameDetails string     `json:"gameDetails" doc:"The game's details (Markdown); empty for none, and in a preview."`
}

type boardInput struct {
	ID      string `path:"id" pattern:"^[A-Za-z0-9]{6}$" patternDescription:"six letters or digits" doc:"The card's id, in any case."`
	Preview bool   `query:"preview" doc:"The card alone, without the game: for a card preview."`
}

type boardOutput struct{ Body Board }

func (h handlers) registerBoard(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-board", Method: http.MethodGet, Path: "/board/{id}",
		Summary: "A player's board",
		Description: "Anyone with the card's id may read it. A personal card waiting for approval is refused (403) " +
			"to anyone but staff with the Manage Cards page. Since the game differs by who asks (see get-game), " +
			"it is never cached.",
		Tags:   boardTags,
		Errors: []int{http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity},
	}, h.getBoard)
}

func (h handlers) getBoard(ctx context.Context, in *boardInput) (*boardOutput, error) {
	c, err := h.cardAnswer(h.read().GetCard(ctx, cardKey(in.ID)))
	if err != nil {
		return nil, err
	}
	// A personal card plays once staff approve it; until then only they may see it,
	// to review it.
	if c.CustomStatus == "pending" && !Permitted(Who(ctx), permissions.BingoCards) {
		return nil, huma.Error403Forbidden("This personal card is waiting for staff to approve it, and can't be played yet.")
	}
	out := Board{Card: c}
	if in.Preview {
		return &boardOutput{Body: out}, nil
	}
	state, err := h.Game.State(ctx, gameStaff(ctx))
	if err != nil {
		return nil, h.fail("read the game", err)
	}
	out.Game, out.GameDetails = state.Game, state.GameDetails
	return &boardOutput{Body: out}, nil
}
