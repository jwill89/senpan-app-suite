package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/game"
	"app-suite/internal/permissions"
)

// The bingo game: the game running, which anyone may read (players get its numbers
// once the draw delay has passed, staff with the Game page at once), and the staff's
// commands to start, draw, end, answer the half-time prompt and change its controls.
// Players send the It's Yoever reaction from their board. Every change reaches the
// consoles, the plugin and the boards as a live event (the internal/game package
// says which event goes to whom).

var gameTags = []string{"Game"}

// GameStartInput is a game to start.
type GameStartInput struct {
	PatternIDs   []int64 `json:"patternIds" minItems:"1" maxItems:"1000" nullable:"false" doc:"The win patterns to play for: at least one. Those deleted since are left out, and a start none of whose patterns exists is refused (422)."`
	Auto         bool    `json:"auto,omitempty" doc:"Whether numbers are drawn on their own, the first at once. Left out, they aren't."`
	AutoInterval int     `json:"autoInterval,omitempty" doc:"Seconds between automatic draws, held between 5 and 600; 0 or left out for the default, 30."`
	Replace      bool    `json:"replace,omitempty" doc:"End the game running, if one is, and start this one in its place. That game's winners are not logged, since nobody confirmed them (end-game does). Left out, a start while a game is running is refused (409)."`
}

// DrawInput is how a number is drawn.
type DrawInput struct {
	Delay int `json:"delay,omitempty" minimum:"0" maximum:"60" doc:"Seconds the number waits before players get it, so the caller can announce it first; staff get it at once. Left out, 0."`
}

// GameEndInput ends a game, naming it and the winners staff confirmed.
type GameEndInput struct {
	GameID         int64    `json:"gameId" doc:"The game to end, as the console shows it. One that has already ended answers ended false; one that has ended while another has started since is refused (409), so a confirm left open can't end the new game."`
	ValidWinnerIDs []string `json:"validWinnerIds,omitempty" maxItems:"1000" nullable:"false" doc:"The winning cards staff confirmed: each is written to the winners log. Left out or empty, none."`
}

// HalftimeAnswer is staff's answer to a game's half-time prompt.
type HalftimeAnswer struct {
	GameID   int64 `json:"gameId" doc:"The game whose prompt this answers (the prompt's gameId). An answer for a game that isn't running, or a second answer, is refused (409)."`
	Minigame bool  `json:"minigame" doc:"True to run a mini-game: players are told once the number that reached half-time has got to them, and the automatic draws stay paused. False for none: they start again if the prompt paused them."`
}

// GameChanges is what staff change on the game's controls: only what is sent.
type GameChanges struct {
	Delay         *int    `json:"delay,omitempty" minimum:"0" maximum:"60" doc:"Seconds a drawn number waits before players get it, kept as the settings' defaultDrawDelay: the automatic draws use it, and every console's selector follows."`
	Details       *string `json:"details,omitempty" maxLength:"20000" doc:"The game's details (Markdown), shown to players; empty for none."`
	YoeverEnabled *bool   `json:"yoeverEnabled,omitempty" doc:"Whether players may send the It's Yoever reaction this game."`
	AutoEnabled   *bool   `json:"autoEnabled,omitempty" doc:"Whether numbers are drawn on their own. Switching them on draws the first at once."`
	AutoInterval  *int    `json:"autoInterval,omitempty" doc:"Seconds between automatic draws, held between 5 and 600; 0 or less for the default, 30. Sent with autoEnabled, they start at it."`
}

// YoeverInput is a player's It's Yoever reaction.
type YoeverInput struct {
	CardID string `json:"cardId" pattern:"^[A-Za-z0-9]{6}$" patternDescription:"six letters or digits" doc:"The board sending it: its card's id, in any case."`
}

type gameStateOutput struct{ Body game.GameState }

type drawOutput struct{ Body game.DrawResult }

type endOutput struct{ Body game.EndResult }

type yoeverOutput struct{ Body game.YoeverResult }

type gameStartInput struct{ Body GameStartInput }

type drawInput struct{ Body DrawInput }

type gameEndInput struct{ Body GameEndInput }

type halftimeInput struct{ Body HalftimeAnswer }

type gameChangesInput struct{ Body GameChanges }

type yoeverInput struct{ Body YoeverInput }

func (h handlers) registerGame(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.BingoGame)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}

	huma.Register(api, huma.Operation{
		OperationID: "get-game", Method: http.MethodGet, Path: "/game",
		Summary: "The game running",
		Description: "Anyone may read it. Staff with the Game page see every number, the winners and the half-time " +
			"prompt at once; anyone else sees the numbers players have got (each after its draw delay), no " +
			"winners and no prompt. Since it differs by who asks, it is never cached.",
		Tags: gameTags,
	}, h.getGame)
	huma.Register(api, huma.Operation{
		OperationID: "start-game", Method: http.MethodPost, Path: "/game/start",
		Summary: "Start a game",
		Description: "Every console and board hears of it on its live stream (game_started). A game already " +
			"running is refused (409) unless the start says to replace it.",
		Tags: gameTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusConflict, http.StatusUnprocessableEntity),
	}, h.startGame)
	huma.Register(api, huma.Operation{
		OperationID: "draw-number", Method: http.MethodPost, Path: "/game/draw",
		Summary: "Draw a number",
		Description: "From the columns the game's patterns use. It takes over from the automatic draws, which " +
			"stop. Staff hear it at once and the boards after its delay (draw); a new " +
			"winner stops the automatic draws, and reaching half-time prompts the consoles (halftime_prompt). " +
			"No game running, or every number drawn, is refused (409).",
		Tags: gameTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusConflict, http.StatusUnprocessableEntity),
	}, h.drawNumber)
	huma.Register(api, huma.Operation{
		OperationID: "end-game", Method: http.MethodPost, Path: "/game/end",
		Summary: "End the game",
		Description: "In one with logging the winners staff confirmed. Everyone hears it ended (game_ended), and " +
			"the staff with the Winners Log page that the log changed (resource_changed). A game ended while " +
			"another has started since is refused (409).",
		Tags: gameTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusConflict, http.StatusUnprocessableEntity),
	}, h.endGame)
	huma.Register(api, huma.Operation{
		OperationID: "answer-halftime", Method: http.MethodPost, Path: "/game/halftime",
		Summary: "Answer the half-time prompt",
		Description: "Once per game: every console hears it was answered, and by whom (halftime_resolved), and " +
			"closes its prompt. A mini-game reaches the boards once the number that reached half-time has " +
			"(minigame). An answer for a game that isn't running, a second answer, or one before " +
			"half-time is refused (409). Answers the game.",
		Tags: gameTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusConflict, http.StatusUnprocessableEntity),
	}, h.answerHalftime)
	huma.Register(api, huma.Operation{
		OperationID: "update-game", Method: http.MethodPatch, Path: "/game",
		Summary: "Change the game's controls",
		Description: "The draw delay, the details, the It's Yoever reaction and the automatic draws: only what is " +
			"sent. Each change is heard with its value: the delay by the Game and Settings pages (draw_delay), " +
			"the details and the reaction by everyone (game_details, yoever_switched), the automatic draws by " +
			"the Game page (auto_draw). " +
			"Answers the game.",
		Tags: gameTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.updateGame)
	huma.Register(api, huma.Operation{
		OperationID: "send-yoever", Method: http.MethodPost, Path: "/game/yoever",
		Summary: "Send the It's Yoever reaction",
		Description: "From a player's board: the boards and the Game page hear it, with the player's name (yoever). A card waits the " +
			"settings' yoeverCooldownSeconds between reactions (429, with Retry-After). An unknown card is a " +
			"404, the reaction switched off a 403, and no game running a 409.",
		Tags: gameTags,
		Errors: []int{http.StatusForbidden, http.StatusNotFound, http.StatusConflict,
			http.StatusUnprocessableEntity, http.StatusTooManyRequests},
	}, h.sendYoever)
}

// by is who is signed in, for the game's log lines and the half-time answer.
func by(ctx context.Context) string {
	if u := Who(ctx); u != nil {
		return u.Username
	}
	return "unknown"
}

// gameStaff is whether the caller sees the game as staff do: every number at once.
func gameStaff(ctx context.Context) bool {
	return Permitted(Who(ctx), permissions.BingoGame)
}

func (h handlers) getGame(ctx context.Context, _ *struct{}) (*gameStateOutput, error) {
	state, err := h.Game.State(ctx, gameStaff(ctx))
	if err != nil {
		return nil, h.fail("read the game", err)
	}
	return &gameStateOutput{Body: state}, nil
}

func (h handlers) startGame(ctx context.Context, in *gameStartInput) (*gameStateOutput, error) {
	state, err := h.Game.Start(ctx, game.Start{
		PatternIDs: in.Body.PatternIDs, Auto: in.Body.Auto, AutoInterval: in.Body.AutoInterval,
		Replace: in.Body.Replace, By: by(ctx),
	})
	switch {
	case errors.Is(err, game.ErrNoPatterns):
		return nil, huma.Error422UnprocessableEntity("None of these patterns exists any more. Reload the page and pick again.",
			&huma.ErrorDetail{Location: "body.patternIds", Value: in.Body.PatternIDs, Message: "None of these patterns exists any more."})
	case errors.Is(err, game.ErrGameLive):
		return nil, huma.Error409Conflict("A game is already running. End it first, or start this one in its place (its winners won't be logged).")
	case err != nil:
		return nil, h.fail("start a game", err)
	}
	return &gameStateOutput{Body: state}, nil
}

func (h handlers) drawNumber(ctx context.Context, in *drawInput) (*drawOutput, error) {
	res, err := h.Game.Draw(ctx, in.Body.Delay)
	switch {
	case errors.Is(err, game.ErrNoGame):
		return nil, huma.Error409Conflict("No game is running.")
	case errors.Is(err, game.ErrAllDrawn):
		return nil, huma.Error409Conflict("Every number this game can call has been drawn.")
	case err != nil:
		return nil, h.fail("draw a number", err)
	}
	return &drawOutput{Body: *res}, nil
}

func (h handlers) endGame(ctx context.Context, in *gameEndInput) (*endOutput, error) {
	out, err := h.Game.End(ctx, in.Body.GameID, in.Body.ValidWinnerIDs, by(ctx))
	switch {
	case errors.Is(err, game.ErrEndStale):
		return nil, huma.Error409Conflict("That game has already ended, and another has started since. Its winners weren't logged.")
	case err != nil:
		return nil, h.fail("end the game", err)
	}
	if out.Logged > 0 {
		ResourceChanged(h.Live, "winners-log")
	}
	return &endOutput{Body: out}, nil
}

func (h handlers) answerHalftime(ctx context.Context, in *halftimeInput) (*gameStateOutput, error) {
	err := h.Game.AnswerHalftime(ctx, in.Body.GameID, in.Body.Minigame, by(ctx))
	switch {
	case errors.Is(err, game.ErrHalftimeStale):
		return nil, huma.Error409Conflict("That half-time prompt was for a game that is no longer running.")
	case errors.Is(err, game.ErrHalftimeAnswered):
		return nil, huma.Error409Conflict("The half-time prompt was already answered.")
	case errors.Is(err, game.ErrNoHalftime):
		return nil, huma.Error409Conflict("The game hasn't reached half-time.")
	case err != nil:
		return nil, h.fail("answer the half-time prompt", err)
	}
	return h.getGame(ctx, nil)
}

func (h handlers) updateGame(ctx context.Context, in *gameChangesInput) (*gameStateOutput, error) {
	b := in.Body
	if err := h.Game.Change(ctx, game.Changes{
		Delay: b.Delay, Details: b.Details, YoeverEnabled: b.YoeverEnabled, AutoEnabled: b.AutoEnabled, AutoInterval: b.AutoInterval,
	}); err != nil {
		return nil, h.fail("change the game", err)
	}
	return h.getGame(ctx, nil)
}

func (h handlers) sendYoever(ctx context.Context, in *yoeverInput) (*yoeverOutput, error) {
	out, wait, err := h.Game.Yoever(ctx, strings.ToUpper(in.Body.CardID))
	switch {
	case errors.Is(err, game.ErrNoCard):
		return nil, noSuchCard()
	case errors.Is(err, game.ErrYoeverOff):
		return nil, huma.Error403Forbidden("It's Yoever is switched off right now.")
	case errors.Is(err, game.ErrNoGame):
		return nil, huma.Error409Conflict("No game is running.")
	case err != nil:
		return nil, h.fail("send the reaction", err)
	case wait > 0:
		return nil, tooMany(wait, "You just did that. Give it a moment.")
	}
	return &yoeverOutput{Body: out}, nil
}
