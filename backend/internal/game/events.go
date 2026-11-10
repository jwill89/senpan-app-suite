package game

import (
	"app-suite/internal/live"
	"app-suite/internal/permissions"
)

// The live events the game publishes. Each names its game in the envelope where it
// has one. The game state's own controls (the draw delay, the details, the reaction
// and the automatic draws) carry their new value, which every console shows; a start
// and an end name the game and the client reads the state again.

// Who hears the game's events: staff with the Game page, and the players' boards.
var (
	toStaff        = live.Audience{Staff: true, Perms: []string{permissions.BingoGame}}
	toPlayers      = live.Audience{Players: true}
	toEveryone     = live.Audience{Players: true, Staff: true, Perms: []string{permissions.BingoGame}}
	toDelayHolders = live.Audience{Staff: true, Perms: []string{permissions.BingoGame, permissions.SystemSettings}}
)

// GameStartedEvent says a game started: read the game (or the board) again.
type GameStartedEvent struct {
	live.Envelope
	Payload live.Empty `json:"payload" doc:"Nothing: read the game, or the board, again."`
}

// Name is the event's type.
func (GameStartedEvent) Name() string { return "game_started" }

// GameEndedEvent says the game ended: read the game (or the board) again.
type GameEndedEvent struct {
	live.Envelope
	Payload live.Empty `json:"payload" doc:"Nothing: read the game, or the board, again. A console's dialogs for this game close."`
}

// Name is the event's type.
func (GameEndedEvent) Name() string { return "game_ended" }

// DrawPayload is a number drawn, as staff hear it.
type DrawPayload struct {
	Drawn   DrawnNumber `json:"drawn" doc:"The number drawn, with its place among the game's calls."`
	Winners []string    `json:"winners" nullable:"false" doc:"The ids of the cards that have won the game so far, in the order they won."`
}

// DrawEvent is a number drawn, by hand or on its own: staff hear it at once.
type DrawEvent struct {
	live.Envelope
	Payload DrawPayload `json:"payload" doc:"The number, and the winners with it."`
}

// Name is the event's type.
func (DrawEvent) Name() string { return "draw" }

// PlayerDrawPayload is a number drawn, as players hear it.
type PlayerDrawPayload struct {
	Drawn DrawnNumber `json:"drawn" doc:"The number drawn, with its place among the game's calls."`
}

// PlayerDrawEvent is a number drawn, sent to the boards once its draw delay has
// passed.
type PlayerDrawEvent struct {
	live.Envelope
	Payload PlayerDrawPayload `json:"payload" doc:"The number."`
}

// Name is the event's type.
func (PlayerDrawEvent) Name() string { return "draw" }

// HalftimePromptPayload is the half-time prompt raised.
type HalftimePromptPayload struct {
	AutoPaused bool `json:"autoPaused" doc:"Whether the automatic draws were paused for the prompt, so declining the mini-game starts them again."`
}

// HalftimePromptEvent says the game reached its half-time mark: staff decide on a
// mini-game.
type HalftimePromptEvent struct {
	live.Envelope
	Payload HalftimePromptPayload `json:"payload" doc:"The prompt."`
}

// Name is the event's type.
func (HalftimePromptEvent) Name() string { return "halftime_prompt" }

// HalftimeResolvedPayload is the half-time prompt's answer.
type HalftimeResolvedPayload struct {
	Minigame bool   `json:"minigame" doc:"Whether a mini-game is on."`
	By       string `json:"by" doc:"The username of whoever answered."`
}

// HalftimeResolvedEvent says the half-time prompt was answered, and by whom: every
// other console closes its prompt.
type HalftimeResolvedEvent struct {
	live.Envelope
	Payload HalftimeResolvedPayload `json:"payload" doc:"The answer."`
}

// Name is the event's type.
func (HalftimeResolvedEvent) Name() string { return "halftime_resolved" }

// MinigameEvent tells the boards a half-time mini-game is on, once the number that
// reached the mark has got to them.
type MinigameEvent struct {
	live.Envelope
	Payload live.Empty `json:"payload" doc:"Nothing more: the board shows the mini-game alert."`
}

// Name is the event's type.
func (MinigameEvent) Name() string { return "minigame" }

// AutoDrawPayload is the automatic draws' state.
type AutoDrawPayload struct {
	Enabled  bool `json:"enabled" doc:"Whether numbers are being drawn on their own."`
	Interval int  `json:"interval" doc:"Seconds between automatic draws, 5 to 600."`
}

// AutoDrawEvent says the automatic draws were switched or retimed, by staff, a
// manual draw, a winner or the half-time prompt.
type AutoDrawEvent struct {
	live.Envelope
	Payload AutoDrawPayload `json:"payload" doc:"Their state now."`
}

// Name is the event's type.
func (AutoDrawEvent) Name() string { return "auto_draw" }

// YoeverPayload is a player's It's Yoever reaction.
type YoeverPayload struct {
	PlayerName string `json:"playerName" doc:"The name on the card that sent it; empty for an unassigned card."`
	Count      int    `json:"count" doc:"How many times players have sent it this game, this one included."`
}

// YoeverEvent is a player's It's Yoever reaction: every board and console shows it.
type YoeverEvent struct {
	live.Envelope
	Payload YoeverPayload `json:"payload" doc:"Who sent it, and the count."`
}

// Name is the event's type.
func (YoeverEvent) Name() string { return "yoever" }

// YoeverSwitchedPayload is the reaction switched on or off.
type YoeverSwitchedPayload struct {
	Enabled bool `json:"enabled" doc:"Whether players may send the reaction."`
}

// YoeverSwitchedEvent says staff switched the It's Yoever reaction on or off.
type YoeverSwitchedEvent struct {
	live.Envelope
	Payload YoeverSwitchedPayload `json:"payload" doc:"Whether it is on."`
}

// Name is the event's type.
func (YoeverSwitchedEvent) Name() string { return "yoever_switched" }

// DrawDelayPayload is the draw delay.
type DrawDelayPayload struct {
	Delay int `json:"delay" minimum:"0" maximum:"60" doc:"Seconds a drawn number waits before players get it (the settings' defaultDrawDelay)."`
}

// DrawDelayEvent says the draw delay changed, from the Game page or the settings.
type DrawDelayEvent struct {
	live.Envelope
	Payload DrawDelayPayload `json:"payload" doc:"The delay now."`
}

// Name is the event's type.
func (DrawDelayEvent) Name() string { return "draw_delay" }

// GameDetailsPayload is the game's details.
type GameDetailsPayload struct {
	Details string `json:"details" doc:"The game's details (Markdown), shown to players; empty for none."`
}

// GameDetailsEvent says staff changed the game's details.
type GameDetailsEvent struct {
	live.Envelope
	Payload GameDetailsPayload `json:"payload" doc:"The details now."`
}

// Name is the event's type.
func (GameDetailsEvent) Name() string { return "game_details" }

// DrawDelayChanged publishes the draw delay to the consoles and the settings page,
// after it is saved.
func DrawDelayChanged(hub Hub, delay int) {
	hub.Publish(&DrawDelayEvent{Payload: DrawDelayPayload{Delay: delay}}, toDelayHolders)
}
