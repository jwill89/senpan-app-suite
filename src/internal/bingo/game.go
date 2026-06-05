package bingo

import (
	"encoding/json"
	"math/rand/v2"
	"sync"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// DrawResult is the full response from a draw action.
type DrawResult struct {
	Drawn   model.BingoDrawnNumber `json:"drawn"`
	Game    model.BingoGameState   `json:"game"`
	Winners []string               `json:"winners"`
}

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
}

// NewService creates a Service backed by the given store.
func NewService(s *store.Store) *Service {
	return &Service{store: s}
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
	g.cardCache = cards
	g.cacheReady = true
	return cards, nil
}

// Start begins a new game with the given pattern IDs.
// Any currently active game is ended first.
func (g *Service) Start(patternIDs []int) (*model.BingoGameState, error) {
	g.opMu.Lock()
	defer g.opMu.Unlock()

	// End any active game.
	if active, err := g.store.GetActiveGame(); err != nil {
		return nil, err
	} else if active != nil {
		if err := g.store.EndGame(active.ID); err != nil {
			return nil, err
		}
	}
	g.invalidateStateCache()

	gameID, err := g.store.CreateGame()
	if err != nil {
		return nil, err
	}

	// Snapshot each selected pattern into game_patterns.
	patterns, err := g.store.GetPatternsByIDs(patternIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range patterns {
		if err := g.store.AddGamePattern(gameID, int(p.ID), p.Name, p.PatternData); err != nil {
			return nil, err
		}
	}

	state, err := g.buildGameState(gameID)
	if err != nil {
		return nil, err
	}
	g.setStateCache(gameID, state)
	return state, nil
}

// Draw picks a random uncalled number, computes winners, caches them,
// and returns the full game state. Returns nil when no active game or all
// 75 numbers have been drawn.
func (g *Service) Draw() (*DrawResult, error) {
	g.opMu.Lock()
	defer g.opMu.Unlock()

	game, err := g.store.GetActiveGame()
	if err != nil {
		return nil, err
	}
	if game == nil {
		return nil, nil
	}

	called, err := g.store.GetCalledNumbers(game.ID)
	if err != nil {
		return nil, err
	}

	// Find remaining numbers (1–75 minus already called).
	calledSet := makeCalledSet(called)
	remaining := make([]int, 0, 75-len(called))
	for i := 1; i <= 75; i++ {
		if !calledSet[i] {
			remaining = append(remaining, i)
		}
	}
	if len(remaining) == 0 {
		return nil, nil
	}

	number := remaining[rand.IntN(len(remaining))]
	order := len(called) + 1

	if err := g.store.AddCalledNumber(game.ID, number, order); err != nil {
		return nil, err
	}
	called = append(called, number)
	calledSet[number] = true

	// Compute and cache winners (skip already-known winners).
	patterns, err := g.store.GetGamePatterns(game.ID)
	if err != nil {
		return nil, err
	}
	existingWinners := parseWinnersCache(game.WinnersCache)
	winners, err := g.computeWinners(calledSet, patterns, existingWinners)
	if err != nil {
		return nil, err
	}
	if err := g.store.UpdateWinnersCache(game.ID, winners); err != nil {
		return nil, err
	}

	state := &model.BingoGameState{
		ID:            game.ID,
		CalledNumbers: called,
		Patterns:      patterns,
		TotalCalled:   len(called),
	}
	g.setStateCache(game.ID, state)

	return &DrawResult{
		Drawn: model.BingoDrawnNumber{
			Number:    number,
			Letter:    LetterForNumber(number),
			CallOrder: order,
		},
		Game:    *state,
		Winners: winners,
	}, nil
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

	// Try to serve from cache.
	g.stateMu.RLock()
	if g.stateCache != nil && g.stateGameID == game.ID {
		state := g.stateCache
		g.stateMu.RUnlock()
		winners := parseWinnersCache(game.WinnersCache)
		return state, winners, nil
	}
	g.stateMu.RUnlock()

	state, err := g.buildGameState(game.ID)
	if err != nil {
		return nil, nil, err
	}
	g.setStateCache(game.ID, state)

	// Read cached winners.
	winners := parseWinnersCache(game.WinnersCache)

	return state, winners, nil
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
// Fetches called numbers and pattern snapshots.
func (g *Service) buildGameState(gameID int64) (*model.BingoGameState, error) {
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
