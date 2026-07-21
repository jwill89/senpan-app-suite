package bingo

import (
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// DrawResult is the response from a draw action; it lives in the model package
// (model.DrawResult) so it is one source of truth for the wire shape, the
// generated frontend type, and the OpenAPI schema. Aliased here for brevity.
type DrawResult = model.DrawResult

// Errors returned by TriggerYoever so the HTTP layer can map them to statuses.
var (
	// ErrYoeverNoGame is returned when there is no active game to react to.
	ErrYoeverNoGame = errors.New("no active game")
	// ErrYoeverDisabled is returned when an admin has switched the reaction off.
	ErrYoeverDisabled = errors.New("yoever reaction disabled")
)

// Service handles game lifecycle, drawing, and winner computation.
type Service struct {
	store *store.Store

	// opMu serializes state-mutating lifecycle operations (Start, Draw, End)
	// so concurrent calls cannot race on the called-numbers set — e.g. two
	// simultaneous draws picking the same call order or drawing a duplicate
	// number (there is no UNIQUE constraint on called_numbers).
	opMu sync.Mutex

	cacheMu    sync.RWMutex
	cardCache  []model.Card
	cacheReady bool

	stateMu     sync.RWMutex
	stateCache  *model.BingoGameState
	stateGameID int64

	detailsMu    sync.RWMutex
	detailsCache string
	detailsReady bool

	// "It's Yoever" reaction state. This is transient, per-game state (reset by
	// resetYoever on each Start): whether the reaction is currently allowed, how
	// many times it has fired this game, and the last time each card triggered it
	// (for the anti-spam cooldown). Guarded by its own mutex, independent of the
	// game caches above.
	yoeverMu      sync.Mutex
	yoeverEnabled bool
	yoeverCount   int
	yoeverLast    map[string]time.Time // card ID (upper-case) → last trigger time

	// Automatic-draw state. Transient, per-game (reset by resetAuto on each Start):
	// whether the auto loop is running and the seconds between draws. resumeAuto
	// remembers that auto was paused for the half-time prompt so choosing "no
	// mini-game" can switch it back on. The loop itself lives in the server layer
	// (it needs the WebSocket hub to broadcast); the Service only owns the state so
	// it can be stamped onto BingoGameState and toggled from the HTTP handlers.
	// Defaults off, including after a restart, so draws never resume unattended.
	autoMu       sync.Mutex
	autoEnabled  bool
	autoInterval int
	resumeAuto   bool
}

// NewService creates a Service backed by the given store. The "It's Yoever"
// reaction defaults to enabled so that if the process restarts while a game is
// live (e.g. a redeploy), the reaction stays on for that game rather than
// silently switching off until the next Start() — matching its enabled-by-default
// semantics. (The per-game count resets to 0 on restart, which is acceptable for
// an ephemeral fun stat.)
func NewService(s *store.Store) *Service {
	return &Service{
		store:         s,
		yoeverEnabled: true,
		yoeverLast:    make(map[string]time.Time),
	}
}

// InvalidateCardCache clears the cached card list.
// Call after generating or deleting cards.
func (g *Service) InvalidateCardCache() {
	g.cacheMu.Lock()
	g.cardCache = nil
	g.cacheReady = false
	g.cacheMu.Unlock()
}

// invalidateStateCache clears the cached game state.
func (g *Service) invalidateStateCache() {
	g.stateMu.Lock()
	g.stateCache = nil
	g.stateGameID = 0
	g.stateMu.Unlock()
}

// GameDetails returns the cached game details string, loading from DB on first call.
func (g *Service) GameDetails() (string, error) {
	g.detailsMu.RLock()
	if g.detailsReady {
		v := g.detailsCache
		g.detailsMu.RUnlock()
		return v, nil
	}
	g.detailsMu.RUnlock()

	g.detailsMu.Lock()
	defer g.detailsMu.Unlock()
	if g.detailsReady {
		return g.detailsCache, nil
	}
	v, err := g.store.GetSetting("game_details")
	if err != nil {
		return "", err
	}
	g.detailsCache = v
	g.detailsReady = true
	return v, nil
}

// SetGameDetails updates the cached game details and persists to DB.
func (g *Service) SetGameDetails(details string) error {
	if err := g.store.SetSetting("game_details", details); err != nil {
		return err
	}
	g.detailsMu.Lock()
	g.detailsCache = details
	g.detailsReady = true
	g.detailsMu.Unlock()
	return nil
}

// cachedCards returns all cards, loading from the DB on first call.
func (g *Service) cachedCards() ([]model.Card, error) {
	g.cacheMu.RLock()
	if g.cacheReady {
		cards := g.cardCache
		g.cacheMu.RUnlock()
		return cards, nil
	}
	g.cacheMu.RUnlock()

	// Upgrade to write lock to populate cache.
	g.cacheMu.Lock()
	defer g.cacheMu.Unlock()

	// Double-check after acquiring write lock.
	if g.cacheReady {
		return g.cardCache, nil
	}

	cards, err := g.store.ListCards()
	if err != nil {
		return nil, err
	}
	// Pending custom-card requests are awaiting staff approval and are not yet
	// playable, so they must never be eligible to win. Exclude them from the cache
	// that drives winner computation. (InvalidateCardCache is called on approval, so
	// a card becomes eligible the moment it is approved.)
	playable := make([]model.Card, 0, len(cards))
	for _, c := range cards {
		if c.CustomStatus == "pending" {
			continue
		}
		playable = append(playable, c)
	}
	g.cardCache = playable
	g.cacheReady = true
	return playable, nil
}

// Start begins a new game with the given pattern IDs. Any currently active game
// is ended first. auto/autoInterval seed the automatic-draw controls for the new
// game (interval clamped to a sane range); pass auto=false to start a manual game.
func (g *Service) Start(patternIDs []int, auto bool, autoInterval int) (*model.BingoGameState, error) {
	g.opMu.Lock()
	defer g.opMu.Unlock()

	// Read the selected patterns to snapshot (a read, so outside the write tx).
	patterns, err := g.store.GetPatternsByIDs(patternIDs)
	if err != nil {
		return nil, err
	}
	snapshots := make([]model.BingoGamePattern, len(patterns))
	for i, p := range patterns {
		snapshots[i] = model.BingoGamePattern{ID: int(p.ID), Name: p.Name, PatternData: p.PatternData}
	}

	// End the active game, create the new one, and snapshot all patterns in a
	// single transaction, so a partial failure can't leave a new active game with
	// an incomplete pattern set (which miscalibrates winner detection). The
	// createdAt timestamp (for the admin elapsed-time clock) comes back with it.
	gameID, createdAt, err := g.store.StartGame(snapshots)
	if err != nil {
		return nil, err
	}
	g.invalidateStateCache()
	// A fresh game re-enables the reaction, zeroes the counter, and forgets every
	// card's cooldown, so the on/off toggle and "Yoevers: N" count are per-game.
	g.resetYoever()
	// Seed the automatic-draw controls for this game (off unless started with auto).
	g.resetAuto(auto, autoInterval)

	state, err := g.buildGameState(gameID, createdAt)
	if err != nil {
		return nil, err
	}
	state.YoeverEnabled, state.YoeverCount = g.yoeverSnapshot()
	state.AutoEnabled, state.AutoInterval = g.AutoState()
	g.setStateCache(gameID, state)
	return state, nil
}

// Draw picks a random uncalled number, computes winners, caches them, and returns
// the full game state. The bool reports whether this draw produced a *new* winner
// (a card not already in the winners cache) — the caller uses it to switch off the
// automatic-draw loop the moment a winner is recognized. Returns (nil, false, nil)
// when there is no active game or all callable numbers have been drawn.
func (g *Service) Draw() (*DrawResult, bool, error) {
	g.opMu.Lock()
	defer g.opMu.Unlock()
	return g.drawLocked()
}

// DrawAuto is Draw for the automatic loop: it draws only while auto is still
// enabled, testing the flag under the *same* op lock that serializes every draw.
// So a disable that lands between the scheduler deciding to draw and the draw
// itself — a manual draw taking over, a winner, an admin toggling auto off — is
// serialized against the draw and reliably prevents the stray auto-draw. Returns
// (nil, false, nil) when auto is off, the same signal the scheduler treats as
// "park" (like an exhausted or absent game).
func (g *Service) DrawAuto() (*DrawResult, bool, error) {
	g.opMu.Lock()
	defer g.opMu.Unlock()
	if enabled, _ := g.AutoState(); !enabled {
		return nil, false, nil
	}
	return g.drawLocked()
}

// drawLocked is the shared draw body; the caller must already hold opMu.
func (g *Service) drawLocked() (*DrawResult, bool, error) {
	game, err := g.store.GetActiveGame()
	if err != nil {
		return nil, false, err
	}
	if game == nil {
		return nil, false, nil
	}

	called, err := g.store.GetCalledNumbers(game.ID)
	if err != nil {
		return nil, false, err
	}

	patterns, err := g.store.GetGamePatterns(game.ID)
	if err != nil {
		return nil, false, err
	}

	// Only draw from columns whose numbers can complete one of the game's
	// patterns. A column no pattern uses (e.g. N for a postage-stamp game, where
	// no winning cell sits in the centre column) is skipped entirely, so those
	// numbers are never called.
	cols := PatternColumns(patterns)

	// Find remaining numbers: 1–75 minus already called, restricted to the
	// active columns above (column index = (n-1)/15).
	calledSet := makeCalledSet(called)
	remaining := make([]int, 0, 75-len(called))
	for i := 1; i <= 75; i++ {
		if !calledSet[i] && cols[(i-1)/15] {
			remaining = append(remaining, i)
		}
	}
	if len(remaining) == 0 {
		return nil, false, nil
	}

	number := remaining[rand.IntN(len(remaining))]
	order := len(called) + 1

	if err := g.store.AddCalledNumber(game.ID, number, order); err != nil {
		return nil, false, err
	}
	called = append(called, number)
	calledSet[number] = true

	// Compute and cache winners (skip already-known winners).
	existingWinners := parseWinnersCache(game.WinnersCache)
	winners, err := g.computeWinners(calledSet, patterns, existingWinners)
	if err != nil {
		return nil, false, err
	}
	if err := g.store.UpdateWinnersCache(game.ID, winners); err != nil {
		return nil, false, err
	}
	newWinner := len(winners) > len(existingWinners)

	state := &model.BingoGameState{
		ID:            game.ID,
		CreatedAt:     game.CreatedAt,
		CalledNumbers: called,
		Patterns:      patterns,
		TotalCalled:   len(called),
	}
	state.YoeverEnabled, state.YoeverCount = g.yoeverSnapshot()
	state.AutoEnabled, state.AutoInterval = g.AutoState()
	g.setStateCache(game.ID, state)

	return &DrawResult{
		Drawn: model.BingoDrawnNumber{
			Number:    number,
			Letter:    LetterForNumber(number),
			CallOrder: order,
		},
		Game:    *state,
		Winners: winners,
	}, newWinner, nil
}

// End ends the currently active game. Returns true if a game was ended.
func (g *Service) End() (bool, error) {
	g.opMu.Lock()
	defer g.opMu.Unlock()

	game, err := g.store.GetActiveGame()
	if err != nil {
		return false, err
	}
	if game == nil {
		return false, nil
	}
	g.invalidateStateCache()
	// A game with auto running must stop drawing the moment it ends.
	g.DisableAuto()
	return true, g.store.EndGame(game.ID)
}

// CurrentState returns the active game state and cached winners.
// Returns (nil, empty winners, nil) when no game is active.
func (g *Service) CurrentState() (*model.BingoGameState, []string, error) {
	game, err := g.store.GetActiveGame()
	if err != nil {
		return nil, nil, err
	}
	if game == nil {
		return nil, []string{}, nil
	}

	// Try to serve from cache. Return a shallow copy stamped with the current
	// yoever snapshot rather than the shared cached pointer: the count/toggle
	// change far more often than the game state, so mutating the cached struct in
	// place would race with concurrent readers serializing it.
	g.stateMu.RLock()
	if g.stateCache != nil && g.stateGameID == game.ID {
		stateCopy := *g.stateCache
		g.stateMu.RUnlock()
		stateCopy.YoeverEnabled, stateCopy.YoeverCount = g.yoeverSnapshot()
		stateCopy.AutoEnabled, stateCopy.AutoInterval = g.AutoState()
		winners := parseWinnersCache(game.WinnersCache)
		return &stateCopy, winners, nil
	}
	g.stateMu.RUnlock()

	state, err := g.buildGameState(game.ID, game.CreatedAt)
	if err != nil {
		return nil, nil, err
	}
	// Stamp the yoever + auto snapshots before caching so the cached struct is never
	// mutated after another goroutine can observe it (see the cache-hit path).
	state.YoeverEnabled, state.YoeverCount = g.yoeverSnapshot()
	state.AutoEnabled, state.AutoInterval = g.AutoState()
	g.setStateCache(game.ID, state)

	// Read cached winners.
	winners := parseWinnersCache(game.WinnersCache)

	return state, winners, nil
}

// ── "It's Yoever" reaction ──────────────────────────────────────────────────

// resetYoever restores the per-game reaction state: enabled, zero count, and no
// per-card cooldowns. Called from Start so each game begins fresh.
func (g *Service) resetYoever() {
	g.yoeverMu.Lock()
	g.yoeverEnabled = true
	g.yoeverCount = 0
	g.yoeverLast = make(map[string]time.Time)
	g.yoeverMu.Unlock()
}

// yoeverSnapshot returns the current enabled flag and trigger count for stamping
// onto a BingoGameState.
func (g *Service) yoeverSnapshot() (enabled bool, count int) {
	g.yoeverMu.Lock()
	defer g.yoeverMu.Unlock()
	return g.yoeverEnabled, g.yoeverCount
}

// YoeverEnabled reports whether the reaction is currently allowed.
func (g *Service) YoeverEnabled() bool {
	g.yoeverMu.Lock()
	defer g.yoeverMu.Unlock()
	return g.yoeverEnabled
}

// SetYoeverEnabled switches the reaction on or off (admin control).
func (g *Service) SetYoeverEnabled(on bool) {
	g.yoeverMu.Lock()
	g.yoeverEnabled = on
	g.yoeverMu.Unlock()
}

// TriggerYoever records a reaction trigger for cardID and returns the new
// per-game count. It enforces, in order: an active game must exist
// (ErrYoeverNoGame), the reaction must be enabled (ErrYoeverDisabled), and the
// same card must not have triggered within the cooldown window — in which case
// it returns retryAfter > 0 (and does not count the trigger). `now` is passed in
// so tests can control the clock. A non-positive cooldown disables the throttle.
func (g *Service) TriggerYoever(cardID string, now time.Time, cooldown time.Duration) (count int, retryAfter time.Duration, err error) {
	game, err := g.store.GetActiveGame()
	if err != nil {
		return 0, 0, err
	}
	if game == nil {
		return 0, 0, ErrYoeverNoGame
	}

	g.yoeverMu.Lock()
	defer g.yoeverMu.Unlock()
	if !g.yoeverEnabled {
		return 0, 0, ErrYoeverDisabled
	}
	if g.yoeverLast == nil {
		g.yoeverLast = make(map[string]time.Time)
	}
	if cooldown > 0 {
		if last, ok := g.yoeverLast[cardID]; ok {
			if elapsed := now.Sub(last); elapsed < cooldown {
				return 0, cooldown - elapsed, nil
			}
		}
	}
	g.yoeverLast[cardID] = now
	g.yoeverCount++
	return g.yoeverCount, 0, nil
}

// ── Automatic draw ──────────────────────────────────────────────────────────

// Automatic-draw interval bounds (seconds). DefaultAutoInterval is used when auto
// is switched on without a chosen interval; every interval is clamped to
// [MinAutoInterval, MaxAutoInterval] so the loop can never spin or stall forever.
const (
	DefaultAutoInterval = 30
	MinAutoInterval     = 5
	MaxAutoInterval     = 600
)

// ClampAutoInterval coerces a requested interval (seconds) into the supported
// range, substituting the default for a non-positive value.
func ClampAutoInterval(sec int) int {
	if sec <= 0 {
		return DefaultAutoInterval
	}
	if sec < MinAutoInterval {
		return MinAutoInterval
	}
	if sec > MaxAutoInterval {
		return MaxAutoInterval
	}
	return sec
}

// resetAuto seeds the per-game auto-draw state at the start of a game: enabled
// only when the game was started with auto, the interval clamped to range, and no
// pending half-time resume. Called from Start.
func (g *Service) resetAuto(enabled bool, interval int) {
	g.autoMu.Lock()
	g.autoEnabled = enabled
	g.autoInterval = ClampAutoInterval(interval)
	g.resumeAuto = false
	g.autoMu.Unlock()
}

// AutoState returns the current auto-draw enabled flag and interval (seconds), for
// stamping onto a BingoGameState and for the server's draw loop.
func (g *Service) AutoState() (enabled bool, interval int) {
	g.autoMu.Lock()
	defer g.autoMu.Unlock()
	return g.autoEnabled, g.autoInterval
}

// SetAutoEnabled switches the auto-draw loop on or off (admin control). Clears any
// pending half-time resume so a later "no mini-game" choice can't re-arm a loop
// the admin has since turned off. Returns the interval so the caller can broadcast it.
func (g *Service) SetAutoEnabled(on bool) (interval int) {
	g.autoMu.Lock()
	defer g.autoMu.Unlock()
	g.autoEnabled = on
	g.resumeAuto = false
	return g.autoInterval
}

// EnableAutoOnce switches the auto-draw loop on and atomically reports whether
// THIS call actually flipped it off→on. Unlike a separate AutoState read followed
// by SetAutoEnabled, the check and the set happen under a single lock, so two
// concurrent enables can't both observe "was off" and each arm a redundant
// immediate first draw (the TOCTOU the get-then-set had). Clears any pending
// half-time resume, matching SetAutoEnabled. Returns the interval too, for
// broadcasting.
func (g *Service) EnableAutoOnce() (transitioned bool, interval int) {
	g.autoMu.Lock()
	defer g.autoMu.Unlock()
	transitioned = !g.autoEnabled
	g.autoEnabled = true
	g.resumeAuto = false
	return transitioned, g.autoInterval
}

// SetAutoInterval updates the seconds between automatic draws (clamped). This is a
// live, game-level control; it never writes back to a preset. Returns the applied
// (clamped) value so the caller can broadcast the value that actually took effect.
func (g *Service) SetAutoInterval(sec int) int {
	g.autoMu.Lock()
	defer g.autoMu.Unlock()
	g.autoInterval = ClampAutoInterval(sec)
	return g.autoInterval
}

// DisableAuto switches the loop off and forgets any pending half-time resume. Used
// when a winner is recognized, the callable pool is exhausted, or the game ends —
// cases where auto must not silently come back. Returns whether auto had been
// running, so the caller can decide whether to broadcast the change.
func (g *Service) DisableAuto() (wasEnabled bool) {
	g.autoMu.Lock()
	defer g.autoMu.Unlock()
	wasEnabled = g.autoEnabled
	g.autoEnabled = false
	g.resumeAuto = false
	return wasEnabled
}

// PauseAutoForHalftime switches auto off for the half-time mini-game decision,
// remembering (resumeAuto) that it was running so ResumeAutoAfterHalftime can turn
// it back on if the admin declines the mini-game. Returns true only when it
// actually paused a running loop, so the caller knows whether to broadcast.
func (g *Service) PauseAutoForHalftime() (paused bool) {
	g.autoMu.Lock()
	defer g.autoMu.Unlock()
	if !g.autoEnabled {
		return false
	}
	g.autoEnabled = false
	g.resumeAuto = true
	return true
}

// ResumeAutoAfterHalftime re-enables auto if it was paused for half-time (the
// admin chose "no mini-game"). Returns true when it switched auto back on, so the
// caller can broadcast + restart the loop; false when there was nothing to resume.
func (g *Service) ResumeAutoAfterHalftime() (resumed bool) {
	g.autoMu.Lock()
	defer g.autoMu.Unlock()
	if !g.resumeAuto {
		return false
	}
	g.autoEnabled = true
	g.resumeAuto = false
	return true
}

// ClearHalftimeResume forgets a pending half-time resume without switching auto
// back on. Used when the admin chooses to run a mini-game: auto stays off until
// they re-enable it, and no later action can silently resume it.
func (g *Service) ClearHalftimeResume() {
	g.autoMu.Lock()
	g.resumeAuto = false
	g.autoMu.Unlock()
}

// ── internal helpers ────────────────────────────────────────────────────────

// setStateCache stores the built game state in the in-memory cache.
func (g *Service) setStateCache(gameID int64, state *model.BingoGameState) {
	g.stateMu.Lock()
	g.stateCache = state
	g.stateGameID = gameID
	g.stateMu.Unlock()
}

// buildGameState assembles a GameState from the database for the given game ID.
// Fetches called numbers and pattern snapshots. The createdAt timestamp is
// passed in by the caller (the game's start time); it may be empty.
func (g *Service) buildGameState(gameID int64, createdAt string) (*model.BingoGameState, error) {
	called, err := g.store.GetCalledNumbers(gameID)
	if err != nil {
		return nil, err
	}
	patterns, err := g.store.GetGamePatterns(gameID)
	if err != nil {
		return nil, err
	}
	return &model.BingoGameState{
		ID:            gameID,
		CreatedAt:     createdAt,
		CalledNumbers: called,
		Patterns:      patterns,
		TotalCalled:   len(called),
	}, nil
}

// computeWinners scans all cards against the active patterns and called numbers,
// returning the full list of winning card IDs. Skips cards already in existingWinners
// to avoid redundant checks on each draw.
func (g *Service) computeWinners(calledSet map[int]bool, patterns []model.BingoGamePattern, existingWinners []string) ([]string, error) {
	if len(calledSet) == 0 || len(patterns) == 0 {
		return []string{}, nil
	}

	cards, err := g.cachedCards()
	if err != nil {
		return nil, err
	}

	// Build set of already-known winners to skip.
	knownSet := make(map[string]bool, len(existingWinners))
	for _, id := range existingWinners {
		knownSet[id] = true
	}

	winners := make([]string, 0, len(existingWinners))
	winners = append(winners, existingWinners...)
	for _, card := range cards {
		if knownSet[card.ID] {
			continue
		}
		for _, pat := range patterns {
			if MatchesPattern(card.BoardData, pat.PatternData, calledSet) {
				winners = append(winners, card.ID)
				break
			}
		}
	}
	return winners, nil
}

// PatternColumns reports which of the five bingo columns (B,I,N,G,O → indices
// 0–4) hold at least one required cell across the given patterns, ignoring the
// FREE centre [2][2] (which never needs a number drawn). Draw uses this to skip
// columns no pattern can win from — e.g. a postage-stamp game has no required N
// cells, so no N numbers are drawn. A column is active if ANY pattern uses it,
// which keeps every pattern winnable (each pattern's columns are a subset). If no
// pattern marks a real cell (e.g. a game with no patterns), every column is
// reported active so the game can still draw.
func PatternColumns(patterns []model.BingoGamePattern) [5]bool {
	var cols [5]bool
	for _, p := range patterns {
		for r := 0; r < 5 && r < len(p.PatternData); r++ {
			for c := 0; c < 5 && c < len(p.PatternData[r]); c++ {
				if p.PatternData[r][c] && !(r == 2 && c == 2) {
					cols[c] = true
				}
			}
		}
	}
	if cols == ([5]bool{}) {
		return [5]bool{true, true, true, true, true}
	}
	return cols
}

// HalftimeThreshold is the call count at which the half-time mini-game prompt
// fires: the classic 35-of-75 mark scaled to this game's callable pool (active
// columns × 15), rounded and clamped to at least 1. Mirrors the frontend
// lib/halftime.ts and the plugin so all three surfaces agree on the midpoint.
func HalftimeThreshold(patterns []model.BingoGamePattern) int {
	cols := PatternColumns(patterns)
	active := 0
	for _, on := range cols {
		if on {
			active++
		}
	}
	if active == 0 {
		active = 5
	}
	maxCallable := active * 15
	threshold := int(math.Round(35.0 / 75.0 * float64(maxCallable)))
	if threshold < 1 {
		return 1
	}
	return threshold
}

// MatchesPattern checks if a card matches a win pattern given the called numbers.
func MatchesPattern(board [][]int, pattern [][]bool, calledSet map[int]bool) bool {
	for r := 0; r < 5; r++ {
		if r >= len(board) || r >= len(pattern) {
			return false
		}
		for c := 0; c < 5; c++ {
			if c >= len(board[r]) || c >= len(pattern[r]) {
				return false
			}
			if !pattern[r][c] {
				continue
			}
			// FREE space always counts.
			if r == 2 && c == 2 {
				continue
			}
			if !calledSet[board[r][c]] {
				return false
			}
		}
	}
	return true
}

// makeCalledSet converts a slice of called numbers into a set (map) for O(1) lookups.
func makeCalledSet(called []int) map[int]bool {
	s := make(map[int]bool, len(called))
	for _, n := range called {
		s[n] = true
	}
	return s
}

// parseWinnersCache deserializes the JSON winners_cache string from the database
// into a string slice. Returns an empty slice on empty input or parse failure.
func parseWinnersCache(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var winners []string
	if err := json.Unmarshal([]byte(raw), &winners); err != nil {
		return []string{}
	}
	if winners == nil {
		return []string{}
	}
	return winners
}
