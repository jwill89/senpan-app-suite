// Package game is the bingo game being played: starting one with its win patterns,
// drawing numbers (by hand, or on their own every few seconds), finding the cards
// that win, the half-time mini-game prompt, the players' It's Yoever reaction, and
// ending it with its confirmed winners logged. It keeps what a game needs between
// requests (who has won, the automatic draws, the prompt waiting for an answer, when
// each number reaches players) and publishes each change to the live feed (Hub),
// which the consoles, the plugin and the boards follow. The rules themselves are
// internal/bingo's.
package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"app-suite/internal/bingo"
	"app-suite/internal/db"
	"app-suite/internal/live"
	"app-suite/internal/random"
	"app-suite/internal/settings"
)

// detailsKey is the setting the game's details are kept under.
const detailsKey = "game_details"

// maxDrawDelay is the longest a drawn number may wait for players, in seconds.
const maxDrawDelay = 60

// What a game refuses, and why. The API answers each with its status.
var (
	// ErrNoPatterns is a start whose patterns have all been deleted since the page
	// offered them: a game with none could never be won.
	ErrNoPatterns = errors.New("none of the selected patterns exist any more")
	// ErrGameLive is a start while a game is running, without saying to replace it.
	ErrGameLive = errors.New("a game is running")
	// ErrNoGame is a draw, an answer or a reaction with no game running.
	ErrNoGame = errors.New("no game is running")
	// ErrAllDrawn is a draw when the game has called every number it can.
	ErrAllDrawn = errors.New("every callable number has been drawn")
	// ErrEndStale is an end naming a game that has ended while another has started.
	ErrEndStale = errors.New("that game has ended and another has started")
	// ErrHalftimeStale is a half-time answer naming a game that isn't the one
	// running.
	ErrHalftimeStale = errors.New("that game is no longer running")
	// ErrHalftimeAnswered is a second answer to a game's half-time prompt.
	ErrHalftimeAnswered = errors.New("the half-time prompt was already answered")
	// ErrNoHalftime is an answer when the game hasn't reached its half-time mark.
	ErrNoHalftime = errors.New("no half-time prompt is waiting")
	// ErrNoCard is a reaction from a card that doesn't exist.
	ErrNoCard = errors.New("no such card")
	// ErrYoeverOff is a reaction while staff have it switched off.
	ErrYoeverOff = errors.New("the reaction is switched off")
)

// Hub is the live feed: an event published reaches the open streams in its audience
// (internal/live).
type Hub interface {
	Publish(msg live.Message, to live.Audience)
}

// Service is the game. One serves the whole server.
type Service struct {
	read, write *sql.DB
	hub         Hub
	// now is the clock, which a test may set.
	now func() time.Time

	// opMu puts starting, drawing, ending and answering the half-time prompt one
	// after another, so two draws can't call the same number and an end can't land
	// between a draw's number and its winners.
	opMu sync.Mutex

	// The playable cards, read once and kept until they change (CardsChanged).
	cardsMu    sync.RWMutex
	cards      []card
	cardsReady bool

	// The running game's numbers and patterns, kept between draws.
	snapMu sync.RWMutex
	snap   *snapshot

	detailsMu    sync.RWMutex
	details      string
	detailsReady bool

	// The It's Yoever reaction, per game: whether it is on, how many times it was
	// sent, and when each card last sent it (for its cooldown). Starting a game
	// resets it; it is on when the server starts, so a game running through a
	// restart keeps it.
	yoeverMu      sync.Mutex
	yoeverEnabled bool
	yoeverCount   int
	yoeverLast    map[string]time.Time

	// The automatic draws, per game: whether they run, the seconds between them,
	// and whether the half-time prompt paused them, so declining the mini-game
	// starts them again. Off when the server starts, so draws never resume
	// unattended.
	autoMu       sync.Mutex
	autoEnabled  bool
	autoInterval int
	resumeAuto   bool

	// What the running game holds only in memory, for liveGame: when each number
	// (by its call order) reaches players, the half-time prompt waiting for an
	// answer, whether it was answered, and whether players were told about a
	// mini-game. A server that restarts mid-game starts it afresh: every number
	// called is then the players' already.
	liveMu           sync.Mutex
	liveGame         int64
	playersAt        map[int]time.Time
	halftime         *pendingHalftime
	halftimeAnswered bool
	minigameAlerted  bool

	// autoWake nudges the automatic-draw loop (Run) to look again after a change;
	// one slot, so a burst is one look. autoDrawNow asks it to draw at once rather
	// than after an interval: a game started with them on, or them switched on.
	autoWake    chan struct{}
	autoDrawNow atomic.Bool
}

// pendingHalftime is a game's half-time prompt waiting for an answer, and when the
// number that reached the mark gets to players: a mini-game alert waits for it.
type pendingHalftime struct {
	gameID  int64
	readyAt time.Time
}

// card is a playable card, as winners are checked against it.
type card struct {
	id    string
	board [][]int
}

// snapshot is what the database holds of the running game.
type snapshot struct {
	id        int64
	createdAt *string
	called    []int
	patterns  []GamePattern
}

// New is the game, on the store's read and write pools, telling hub what happens.
func New(read, write *sql.DB, hub Hub) *Service {
	return &Service{
		read: read, write: write, hub: hub, now: time.Now,
		yoeverEnabled: true,
		yoeverLast:    map[string]time.Time{},
		autoInterval:  bingo.DefaultAutoInterval,
		playersAt:     map[int]time.Time{},
		autoWake:      make(chan struct{}, 1),
	}
}

// CardsChanged forgets the playable cards, so the next draw reads them afresh: a card
// made, approved or deleted plays (or stops) from the next draw on.
func (g *Service) CardsChanged() {
	g.cardsMu.Lock()
	g.cards, g.cardsReady = nil, false
	g.cardsMu.Unlock()
}

// -- Reading the game ---------------------------------------------------------

// State is the game as viewer sees it: staff (an account with the Game page) see
// every number at once, the winners and the half-time prompt; anyone else sees the
// numbers players have got, as the boards' live feed gives them.
func (g *Service) State(ctx context.Context, staff bool) (GameState, error) {
	snap, winners, err := g.current(ctx)
	if err != nil {
		return GameState{}, err
	}
	details, err := g.Details(ctx)
	if err != nil {
		return GameState{}, err
	}
	out := GameState{Winners: []string{}, GameDetails: details, DrawDelay: g.DrawDelay(ctx)}
	if snap == nil {
		return out, nil
	}
	view := g.view(snap, staff)
	out.Game = &view
	if staff {
		out.Winners = winners
	}
	return out, nil
}

// Running is the running game's id, or 0 for none.
func (g *Service) Running(ctx context.Context) (int64, error) {
	row, err := db.New(g.read).GetActiveGame(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

// current is the running game and its winners, or nil for none.
func (g *Service) current(ctx context.Context) (*snapshot, []string, error) {
	row, err := db.New(g.read).GetActiveGame(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, []string{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	winners := parseWinners(row.WinnersCache)
	g.snapMu.RLock()
	snap := g.snap
	g.snapMu.RUnlock()
	if snap != nil && snap.id == row.ID {
		return snap, winners, nil
	}
	snap, err = g.load(ctx, db.New(g.read), row.ID, row.CreatedAt)
	if err != nil {
		return nil, nil, err
	}
	g.keep(snap)
	return snap, winners, nil
}

// load reads a game's numbers and patterns.
func (g *Service) load(ctx context.Context, q *db.Queries, id int64, createdAt *string) (*snapshot, error) {
	numbers, err := q.ListCalledNumbers(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGamePatterns(ctx, id)
	if err != nil {
		return nil, err
	}
	snap := &snapshot{id: id, createdAt: createdAt, called: make([]int, 0, len(numbers)), patterns: make([]GamePattern, 0, len(rows))}
	for _, n := range numbers {
		snap.called = append(snap.called, int(n))
	}
	for _, r := range rows {
		p := GamePattern{ID: r.PatternID, Name: r.PatternName}
		if err := json.Unmarshal([]byte(r.PatternData), &p.PatternData); err != nil {
			return nil, fmt.Errorf("game %d's pattern %d: %w", id, r.PatternID, err)
		}
		snap.patterns = append(snap.patterns, p)
	}
	return snap, nil
}

// keep holds snap as the running game's, unless it is older than the one held: a
// read that began before a draw must not put back the numbers before it.
func (g *Service) keep(snap *snapshot) {
	g.snapMu.Lock()
	defer g.snapMu.Unlock()
	if g.snap != nil && g.snap.id == snap.id && len(snap.called) < len(g.snap.called) {
		return
	}
	g.snap = snap
}

// forget drops the running game's numbers, when it ends or another starts.
func (g *Service) forget() {
	g.snapMu.Lock()
	g.snap = nil
	g.snapMu.Unlock()
}

// view is the game as viewer sees it (see State).
func (g *Service) view(snap *snapshot, staff bool) Game {
	now := g.now()
	g.liveMu.Lock()
	ours := g.liveGame == snap.id
	at := func(order int) (time.Time, bool) {
		if !ours {
			return time.Time{}, false
		}
		t, ok := g.playersAt[order]
		return t, ok
	}
	type call struct{ number, order int }
	shown := make([]call, 0, len(snap.called))
	for i, n := range snap.called {
		if t, known := at(i + 1); !staff && known && t.After(now) {
			continue // still inside its draw delay
		}
		shown = append(shown, call{n, i + 1})
	}
	out := Game{
		ID: snap.id, CreatedAt: snap.createdAt, CalledNumbers: make([]int, 0, len(shown)),
		Patterns: snap.patterns, TotalCalled: len(shown),
		MinigameAlerted: ours && g.minigameAlerted,
	}
	for _, c := range shown {
		out.CalledNumbers = append(out.CalledNumbers, c.number)
	}
	if len(shown) > 0 {
		last := shown[len(shown)-1]
		out.LastDraw = &LastDraw{DrawnNumber: drawn(last.number, last.order)}
		if t, known := at(last.order); known {
			out.LastDraw.PlayersAt = stamp(t)
		}
	}
	pending := ours && g.halftime != nil && g.halftime.gameID == snap.id
	g.liveMu.Unlock()

	out.YoeverEnabled, out.YoeverCount = g.yoeverState()
	out.AutoEnabled, out.AutoInterval = g.AutoState()
	if staff && pending {
		g.autoMu.Lock()
		out.Halftime = &Halftime{GameID: snap.id, AutoPaused: g.resumeAuto}
		g.autoMu.Unlock()
	}
	return out
}

// drawn is a number called at order, with its letter.
func drawn(number, order int) DrawnNumber {
	return DrawnNumber{Number: number, Letter: bingo.LetterForNumber(number), CallOrder: order}
}

// stamp is t as the API writes a time: RFC 3339 in UTC, to the millisecond, since a
// draw delay is counted down to the second.
func stamp(t time.Time) *string {
	s := t.UTC().Format("2006-01-02T15:04:05.000Z07:00")
	return &s
}

// Details is the game's details (Markdown), read once and kept.
func (g *Service) Details(ctx context.Context) (string, error) {
	g.detailsMu.RLock()
	if g.detailsReady {
		v := g.details
		g.detailsMu.RUnlock()
		return v, nil
	}
	g.detailsMu.RUnlock()
	g.detailsMu.Lock()
	defer g.detailsMu.Unlock()
	if g.detailsReady {
		return g.details, nil
	}
	v, err := setting(ctx, db.New(g.read), detailsKey)
	if err != nil {
		return "", err
	}
	g.details, g.detailsReady = v, true
	return v, nil
}

// DrawDelay is the seconds a drawn number waits for players (the settings'
// defaultDrawDelay), 0 to 60: the automatic draws use it. A failed read is none.
func (g *Service) DrawDelay(ctx context.Context) int {
	return settingNumber(ctx, db.New(g.read), settings.DefaultDrawDelay, 0, 0, maxDrawDelay)
}

// yoeverCooldown is how long a card waits between reactions (the settings'
// yoeverCooldownSeconds).
func (g *Service) yoeverCooldown(ctx context.Context) time.Duration {
	secs := settingNumber(ctx, db.New(g.read), settings.YoeverCooldownSeconds, settings.DefaultYoeverCooldown, 0, 1<<20)
	return time.Duration(secs) * time.Second
}

// setting reads a setting's text, "" when it was never saved.
func setting(ctx context.Context, q *db.Queries, key string) (string, error) {
	v, err := q.GetSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// settingNumber reads a numeric setting held to [lo, hi], or fallback when it is
// unset, not a number or can't be read.
func settingNumber(ctx context.Context, q *db.Queries, key string, fallback, lo, hi int) int {
	v, err := setting(ctx, q, key)
	if err != nil || v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return min(max(n, lo), hi)
}

// parseWinners reads a game's winners_cache: a JSON array of card ids. Anything
// else is none.
func parseWinners(raw string) []string {
	var winners []string
	if raw == "" || json.Unmarshal([]byte(raw), &winners) != nil || winners == nil {
		return []string{}
	}
	return winners
}

// -- The live state of the running game --------------------------------------

// live makes the in-memory state the running game's, starting it afresh for a game
// it doesn't yet hold. The caller holds liveMu.
func (g *Service) live(gameID int64) {
	if g.liveGame == gameID {
		return
	}
	g.liveGame = gameID
	g.playersAt = map[int]time.Time{}
	g.halftime = nil
	g.halftimeAnswered = false
	g.minigameAlerted = false
}

// -- Starting -----------------------------------------------------------------

// Start is what a new game is started with.
type Start struct {
	PatternIDs   []int64
	Auto         bool
	AutoInterval int
	// Replace ends a game that is running (its winners unlogged: nobody confirmed
	// them) rather than refusing with ErrGameLive.
	Replace bool
	// By is who starts it, for the log.
	By string
}

// Start starts a game, and tells everyone. It answers the game as staff see it.
func (g *Service) Start(ctx context.Context, in Start) (GameState, error) {
	g.opMu.Lock()
	defer g.opMu.Unlock()

	rows, err := db.New(g.read).ListPatternsByIDs(ctx, in.PatternIDs)
	if err != nil {
		return GameState{}, err
	}
	// Only the ids that still exist come back: a selection whose patterns were all
	// deleted since the page loaded would start a game nobody can win.
	if len(rows) == 0 {
		return GameState{}, ErrNoPatterns
	}
	var replaced int64
	var created *db.CreateGameRow
	err = g.transact(ctx, func(q *db.Queries) error {
		running, err := q.GetActiveGame(ctx)
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return err
		case !in.Replace:
			return ErrGameLive
		default:
			replaced = running.ID
			if _, err := q.EndActiveGames(ctx); err != nil {
				return err
			}
		}
		if created, err = q.CreateGame(ctx); err != nil {
			return err
		}
		for _, p := range rows {
			if err := q.AddGamePattern(ctx, db.AddGamePatternParams{
				GameID: created.ID, PatternID: p.ID, PatternName: p.Name, PatternData: p.PatternData,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return GameState{}, err
	}
	// The game exists from here on: everything after runs whether or not the caller
	// is still waiting, so no console is left not knowing about it.
	ctx = context.WithoutCancel(ctx)
	g.forget()
	g.resetYoever()
	g.resetAuto(in.Auto, in.AutoInterval)
	g.liveMu.Lock()
	g.live(created.ID)
	g.liveMu.Unlock()

	snap, err := g.load(ctx, db.New(g.write), created.ID, created.CreatedAt)
	if err != nil {
		return GameState{}, err
	}
	g.keep(snap)
	details, err := g.Details(ctx)
	if err != nil {
		return GameState{}, err
	}
	staff := g.view(snap, true)
	g.hub.Publish(&GameStartedEvent{Envelope: live.Game(created.ID)}, toEveryone)
	slog.Info("bingo game started", "game_id", created.ID, "patterns", len(rows), "auto", in.Auto,
		"auto_interval", staff.AutoInterval, "replaced", replaced, "by", in.By)
	// The automatic draws start at once for a game started with them on; either way
	// the loop looks again.
	if in.Auto {
		g.autoDrawNow.Store(true)
	}
	g.wake()
	return GameState{Game: &staff, Winners: []string{}, GameDetails: details, DrawDelay: g.DrawDelay(ctx)}, nil
}

// transact runs f in one write transaction, committing when it returns nil.
func (g *Service) transact(ctx context.Context, f func(*db.Queries) error) error {
	tx, err := g.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := f(db.New(tx)); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

// -- Drawing ------------------------------------------------------------------

// Draw draws a number by hand, which takes over from the automatic draws (they stop),
// and tells everyone: staff at once, players after delay seconds. It answers the
// draw as staff see it.
func (g *Service) Draw(ctx context.Context, delay int) (*DrawResult, error) {
	// Switched off first, so the loop drops the draw it was waiting to make; the
	// draw itself checks again under the lock.
	if g.DisableAuto() {
		g.autoChanged()
	}
	g.opMu.Lock()
	res, newWinner, err := g.drawLocked(ctx, delay)
	g.opMu.Unlock()
	if err != nil {
		return nil, err
	}
	g.afterDraw(context.WithoutCancel(ctx), res, newWinner, delay)
	return res, nil
}

// drawAuto is the automatic loop's draw: only while the automatic draws are still
// on, checked under the same lock every draw takes, so one switched off a moment
// before (a manual draw, a winner, staff) can't slip a number in. It answers nil
// when they are off.
func (g *Service) drawAuto(ctx context.Context, delay int) (*DrawResult, bool, error) {
	g.opMu.Lock()
	defer g.opMu.Unlock()
	if on, _ := g.AutoState(); !on {
		return nil, false, nil
	}
	return g.drawLocked(ctx, delay)
}

// drawLocked draws: a number the game can still call, at random, with the winners it
// makes. It reports whether a card won with it. The caller holds opMu.
func (g *Service) drawLocked(ctx context.Context, delay int) (*DrawResult, bool, error) {
	q := db.New(g.read)
	row, err := q.GetActiveGame(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrNoGame
	}
	if err != nil {
		return nil, false, err
	}
	snap, err := g.load(ctx, q, row.ID, row.CreatedAt)
	if err != nil {
		return nil, false, err
	}
	// Only from the columns the patterns use: a number no pattern can need (an N for
	// postage stamps) is never called.
	cols := bingo.PatternColumns(grids(snap.patterns))
	called := bingo.CalledSet(snap.called)
	remaining := make([]int, 0, 75-len(snap.called))
	for n := 1; n <= 75; n++ {
		if !called[n] && cols[(n-1)/15] {
			remaining = append(remaining, n)
		}
	}
	if len(remaining) == 0 {
		return nil, false, ErrAllDrawn
	}
	number := remaining[random.IntN(len(remaining))]
	order := len(snap.called) + 1
	called[number] = true

	// From here the draw is made whole, whether or not the caller is still waiting.
	ctx = context.WithoutCancel(ctx)
	before := parseWinners(row.WinnersCache)
	winners, err := g.winners(ctx, called, snap.patterns, before)
	if err != nil {
		return nil, false, err
	}
	// When players get it is known before anyone can read the number, so a read in
	// between never shows it early.
	g.liveMu.Lock()
	g.live(row.ID)
	g.playersAt[order] = g.now().Add(time.Duration(delay) * time.Second)
	g.liveMu.Unlock()
	cache, err := json.Marshal(winners)
	if err == nil {
		err = g.transact(ctx, func(q *db.Queries) error {
			if err := q.AddCalledNumber(ctx, db.AddCalledNumberParams{GameID: row.ID, Number: int64(number), CallOrder: int64(order)}); err != nil {
				return err
			}
			return q.SetWinnersCache(ctx, db.SetWinnersCacheParams{WinnersCache: string(cache), ID: row.ID})
		})
	}
	if err != nil {
		g.liveMu.Lock()
		if g.liveGame == row.ID {
			delete(g.playersAt, order)
		}
		g.liveMu.Unlock()
		return nil, false, err
	}
	snap.called = append(snap.called, number)
	g.keep(snap)
	return &DrawResult{Drawn: drawn(number, order), Game: g.view(snap, true), Winners: winners},
		len(winners) > len(before), nil
}

// grids is the patterns' grids, for the rules.
func grids(patterns []GamePattern) [][][]bool {
	out := make([][][]bool, len(patterns))
	for i, p := range patterns {
		out[i] = p.PatternData
	}
	return out
}

// winners is the cards that win with called: those that had won already, then any
// card one of the patterns now completes.
func (g *Service) winners(ctx context.Context, called map[int]bool, patterns []GamePattern, before []string) ([]string, error) {
	cards, err := g.playable(ctx)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(before))
	for _, id := range before {
		known[id] = true
	}
	winners := append(make([]string, 0, len(before)), before...)
	for _, c := range cards {
		if known[c.id] {
			continue
		}
		for _, p := range patterns {
			if bingo.MatchesPattern(c.board, p.PatternData, called) {
				winners = append(winners, c.id)
				break
			}
		}
	}
	return winners, nil
}

// playable is every card that can win: a personal card still waiting for staff to
// approve it can't, until it is.
func (g *Service) playable(ctx context.Context) ([]card, error) {
	g.cardsMu.RLock()
	if g.cardsReady {
		cards := g.cards
		g.cardsMu.RUnlock()
		return cards, nil
	}
	g.cardsMu.RUnlock()
	g.cardsMu.Lock()
	defer g.cardsMu.Unlock()
	if g.cardsReady {
		return g.cards, nil
	}
	rows, err := db.New(g.read).ListPlayableCards(ctx)
	if err != nil {
		return nil, err
	}
	cards := make([]card, 0, len(rows))
	for _, r := range rows {
		c := card{id: r.ID}
		if err := json.Unmarshal([]byte(r.BoardData), &c.board); err != nil {
			return nil, fmt.Errorf("card %s's numbers: %w", r.ID, err)
		}
		cards = append(cards, c)
	}
	g.cards, g.cardsReady = cards, true
	return cards, nil
}

// -- Ending -------------------------------------------------------------------

// End ends the game gameID names, logging the winners staff confirmed, and tells
// everyone. A game already ended is ended=false, with nothing logged; one ended while
// another has started since is ErrEndStale, so a confirm left open across games
// can't end the new one with the old one's winners.
func (g *Service) End(ctx context.Context, gameID int64, winners []string, by string) (EndResult, error) {
	g.opMu.Lock()
	defer g.opMu.Unlock()

	// Ending and logging are one: detached, so a caller gone part-way can't leave an
	// ended game with its winners unlogged.
	ctx = context.WithoutCancel(ctx)
	var out EndResult
	var names []string
	var calls int
	err := g.transact(ctx, func(q *db.Queries) error {
		running, err := q.GetActiveGame(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if running.ID != gameID {
			return ErrEndStale
		}
		if _, err := q.EndGame(ctx, gameID); err != nil {
			return err
		}
		out.Ended = true
		patterns, err := q.ListGamePatterns(ctx, gameID)
		if err != nil {
			return err
		}
		names = make([]string, 0, len(patterns))
		for _, p := range patterns {
			names = append(names, p.PatternName)
		}
		numbers, err := q.ListCalledNumbers(ctx, gameID)
		if err != nil {
			return err
		}
		calls = len(numbers)
		if len(winners) == 0 {
			return nil
		}
		patternNames, err := json.Marshal(names)
		if err != nil {
			return err
		}
		details, err := setting(ctx, q, detailsKey)
		if err != nil {
			return err
		}
		rows, err := q.ListCardPlayerNames(ctx, winners)
		if err != nil {
			return err
		}
		players := make(map[string]string, len(rows))
		for _, r := range rows {
			players[r.ID] = r.PlayerName
		}
		for _, id := range winners {
			if err := q.AddWinner(ctx, db.AddWinnerParams{
				CardID: id, PlayerName: players[id], GameDetails: details, WinningPatterns: string(patternNames),
			}); err != nil {
				return err
			}
			out.Logged++
		}
		return nil
	})
	if err != nil {
		return EndResult{}, err
	}
	if !out.Ended {
		return out, nil
	}
	g.forget()
	g.DisableAuto()
	g.liveMu.Lock()
	g.live(0)
	g.liveMu.Unlock()
	slog.Info("bingo game ended", "game_id", gameID, "called", calls, "winners", len(winners),
		"winner_cards", winners, "patterns", names, "by", by)
	g.hub.Publish(&GameEndedEvent{Envelope: live.Game(gameID)}, toEveryone)
	// Ending switched the automatic draws off: the loop parks.
	g.wake()
	return out, nil
}

// -- Half-time ----------------------------------------------------------------

// onHalftime is a draw reaching the half-time mark: the automatic draws pause (if
// running) while staff decide on a mini-game, every console is asked, and the game
// remembers when the number that reached the mark gets to players, since a
// mini-game alert waits for it.
func (g *Service) onHalftime(gameID int64, readyAt time.Time) {
	// The prompt waits before the draws pause, so whoever sees them paused finds it.
	g.liveMu.Lock()
	g.live(gameID)
	g.halftime = &pendingHalftime{gameID: gameID, readyAt: readyAt}
	g.halftimeAnswered = false
	g.liveMu.Unlock()
	paused := g.pauseForHalftime()
	if paused {
		g.autoChanged()
	}
	g.hub.Publish(&HalftimePromptEvent{Envelope: live.Game(gameID), Payload: HalftimePromptPayload{AutoPaused: paused}}, toStaff)
}

// AnswerHalftime takes staff's answer to gameID's half-time prompt, once: minigame
// tells the players (when the number that reached the mark has got to them) and leaves
// the automatic draws paused; no mini-game starts them again if the prompt paused
// them. Every console hears it was answered, and by whom, and closes its prompt. A
// second answer is ErrHalftimeAnswered, and one for a game no longer running
// ErrHalftimeStale.
func (g *Service) AnswerHalftime(ctx context.Context, gameID int64, minigame bool, by string) error {
	g.opMu.Lock()
	defer g.opMu.Unlock()
	running, err := g.Running(ctx)
	if err != nil {
		return err
	}
	g.liveMu.Lock()
	ours := g.liveGame == gameID
	var pending *pendingHalftime
	switch {
	case running == 0 || running != gameID:
		err = ErrHalftimeStale
	case ours && g.halftime != nil && g.halftime.gameID == gameID:
		pending, g.halftime, g.halftimeAnswered = g.halftime, nil, true
	case ours && g.halftimeAnswered:
		err = ErrHalftimeAnswered
	default:
		err = ErrNoHalftime
	}
	g.liveMu.Unlock()
	if err != nil {
		return err
	}
	if minigame {
		g.clearResume()
		g.alertMinigame(context.WithoutCancel(ctx), gameID, pending.readyAt)
	} else if g.resume() {
		g.autoChanged()
	}
	slog.Info("bingo halftime decision", "game_id", gameID, "minigame", minigame, "by", by)
	g.hub.Publish(&HalftimeResolvedEvent{Envelope: live.Game(gameID), Payload: HalftimeResolvedPayload{Minigame: minigame, By: by}}, toStaff)
	return nil
}

// alertMinigame tells the players about the mini-game, once the number that reached
// the half-time mark has got to them (so they never hear of it first), and only if
// that game is still the one running.
func (g *Service) alertMinigame(ctx context.Context, gameID int64, readyAt time.Time) {
	send := func() {
		defer recoverPanic("halftime minigame broadcast")
		if running, err := g.Running(ctx); err != nil || running != gameID {
			return
		}
		g.liveMu.Lock()
		if g.liveGame == gameID {
			g.minigameAlerted = true
		}
		g.liveMu.Unlock()
		g.hub.Publish(&MinigameEvent{Envelope: live.Game(gameID)}, toPlayers)
	}
	if wait := readyAt.Sub(g.now()); wait > 0 {
		time.AfterFunc(wait, send)
		return
	}
	send()
}

// -- The It's Yoever reaction -------------------------------------------------

// resetYoever starts a game's reaction afresh: on, never sent, no card waiting.
func (g *Service) resetYoever() {
	g.yoeverMu.Lock()
	g.yoeverEnabled, g.yoeverCount, g.yoeverLast = true, 0, map[string]time.Time{}
	g.yoeverMu.Unlock()
}

func (g *Service) yoeverState() (enabled bool, count int) {
	g.yoeverMu.Lock()
	defer g.yoeverMu.Unlock()
	return g.yoeverEnabled, g.yoeverCount
}

// Yoever is a player's reaction from cardID (upper-case): everyone sees it, with the
// player's name. A card waits the cooldown between reactions: one sent sooner isn't
// counted, and answers how long is left (wait > 0).
func (g *Service) Yoever(ctx context.Context, cardID string) (out YoeverResult, wait time.Duration, err error) {
	c, err := db.New(g.read).GetCard(ctx, cardID)
	if errors.Is(err, sql.ErrNoRows) {
		return out, 0, ErrNoCard
	}
	if err != nil {
		return out, 0, err
	}
	running, err := g.Running(ctx)
	if err != nil {
		return out, 0, err
	}
	if running == 0 {
		return out, 0, ErrNoGame
	}
	cooldown := g.yoeverCooldown(ctx)
	out.CooldownSeconds = int(cooldown / time.Second)
	count, wait, err := g.countYoever(cardID, g.now(), cooldown)
	if err != nil || wait > 0 {
		return out, wait, err
	}
	out.Count = count
	g.hub.Publish(&YoeverEvent{Envelope: live.Game(running), Payload: YoeverPayload{PlayerName: c.PlayerName, Count: count}}, toEveryone)
	return out, 0, nil
}

// countYoever counts a reaction from cardID at now, unless the reaction is off
// (ErrYoeverOff) or the card sent one within cooldown (wait > 0, not counted). A
// cooldown of 0 never holds one back.
func (g *Service) countYoever(cardID string, now time.Time, cooldown time.Duration) (count int, wait time.Duration, err error) {
	g.yoeverMu.Lock()
	defer g.yoeverMu.Unlock()
	if !g.yoeverEnabled {
		return 0, 0, ErrYoeverOff
	}
	if last, ok := g.yoeverLast[cardID]; ok && cooldown > 0 {
		if elapsed := now.Sub(last); elapsed < cooldown {
			return 0, cooldown - elapsed, nil
		}
	}
	g.yoeverLast[cardID] = now
	g.yoeverCount++
	return g.yoeverCount, 0, nil
}

// -- Staff's controls ---------------------------------------------------------

// Changes is what staff change on the game: only what is set.
type Changes struct {
	// Delay is the seconds a drawn number waits for players, 0 to 60 (kept as the
	// settings' defaultDrawDelay).
	Delay         *int
	Details       *string
	YoeverEnabled *bool
	AutoEnabled   *bool
	AutoInterval  *int
}

// Change applies changes and tells everyone each one, so every console's controls
// follow. The delay and the details are saved together.
func (g *Service) Change(ctx context.Context, c Changes) error {
	ctx = context.WithoutCancel(ctx)
	if c.Delay != nil || c.Details != nil {
		err := g.transact(ctx, func(q *db.Queries) error {
			if c.Delay != nil {
				if err := q.SetSetting(ctx, db.SetSettingParams{Key: settings.DefaultDrawDelay, Value: strconv.Itoa(*c.Delay)}); err != nil {
					return err
				}
			}
			if c.Details != nil {
				return q.SetSetting(ctx, db.SetSettingParams{Key: detailsKey, Value: *c.Details})
			}
			return nil
		})
		if err != nil {
			return err
		}
		if c.Delay != nil {
			DrawDelayChanged(g.hub, *c.Delay)
		}
		if c.Details != nil {
			g.detailsMu.Lock()
			g.details, g.detailsReady = *c.Details, true
			g.detailsMu.Unlock()
			g.hub.Publish(&GameDetailsEvent{Payload: GameDetailsPayload{Details: *c.Details}}, toEveryone)
		}
	}
	if c.YoeverEnabled != nil {
		g.yoeverMu.Lock()
		g.yoeverEnabled = *c.YoeverEnabled
		g.yoeverMu.Unlock()
		g.hub.Publish(&YoeverSwitchedEvent{Payload: YoeverSwitchedPayload{Enabled: *c.YoeverEnabled}}, toEveryone)
	}
	// The interval first, so switching them on in the same change runs at it.
	if c.AutoInterval != nil {
		g.autoMu.Lock()
		g.autoInterval = bingo.ClampAutoInterval(*c.AutoInterval)
		g.autoMu.Unlock()
	}
	if c.AutoEnabled != nil {
		if *c.AutoEnabled {
			// Only a switch from off draws at once: two consoles switching them on
			// together make one first draw, not two.
			if g.enableAuto() {
				g.autoDrawNow.Store(true)
			}
		} else {
			g.setAuto(false)
			// Dropped here, where they are switched off, never in the loop: the loop
			// clearing it raced a switch back on and lost its first draw.
			g.autoDrawNow.Store(false)
		}
	}
	if c.AutoInterval != nil || c.AutoEnabled != nil {
		g.autoChanged()
	}
	return nil
}
