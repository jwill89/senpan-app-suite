package bingo

import (
	"path/filepath"
	"sync"
	"testing"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// ── MatchesPattern ──────────────────────────────────────────────────────────

func TestMatchesPattern_FullMatch(t *testing.T) {
	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	// Top-row pattern
	pattern := [][]bool{
		{true, true, true, true, true},
		{false, false, false, false, false},
		{false, false, false, false, false},
		{false, false, false, false, false},
		{false, false, false, false, false},
	}
	called := map[int]bool{1: true, 16: true, 31: true, 46: true, 61: true}
	if !MatchesPattern(board, pattern, called) {
		t.Error("expected match for top-row pattern with all numbers called")
	}
}

func TestMatchesPattern_MissingNumber(t *testing.T) {
	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	pattern := [][]bool{
		{true, true, true, true, true},
		{false, false, false, false, false},
		{false, false, false, false, false},
		{false, false, false, false, false},
		{false, false, false, false, false},
	}
	// Missing 61
	called := map[int]bool{1: true, 16: true, 31: true, 46: true}
	if MatchesPattern(board, pattern, called) {
		t.Error("expected no match when a required number is missing")
	}
}

func TestMatchesPattern_FreeSpace(t *testing.T) {
	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	// Pattern that only requires the FREE centre cell
	pattern := [][]bool{
		{false, false, false, false, false},
		{false, false, false, false, false},
		{false, false, true, false, false},
		{false, false, false, false, false},
		{false, false, false, false, false},
	}
	// No numbers called at all — FREE always counts
	if !MatchesPattern(board, pattern, map[int]bool{}) {
		t.Error("pattern requiring only FREE space should always match")
	}
}

func TestMatchesPattern_EmptyCalledSet(t *testing.T) {
	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	pattern := [][]bool{
		{true, false, false, false, false},
		{false, false, false, false, false},
		{false, false, false, false, false},
		{false, false, false, false, false},
		{false, false, false, false, false},
	}
	if MatchesPattern(board, pattern, map[int]bool{}) {
		t.Error("should not match with empty called set and non-free required cell")
	}
}

func TestMatchesPattern_UndersizedBoard(t *testing.T) {
	board := [][]int{{1, 2}} // too small
	pattern := [][]bool{
		{true, true, true, true, true},
		{false, false, false, false, false},
		{false, false, false, false, false},
		{false, false, false, false, false},
		{false, false, false, false, false},
	}
	if MatchesPattern(board, pattern, map[int]bool{1: true, 2: true}) {
		t.Error("undersized board should not match")
	}
}

func TestMatchesPattern_AllTrue(t *testing.T) {
	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	pattern := make([][]bool, 5)
	for r := range pattern {
		pattern[r] = []bool{true, true, true, true, true}
	}
	// Call every number on the board
	called := map[int]bool{}
	for _, row := range board {
		for _, v := range row {
			if v != 0 {
				called[v] = true
			}
		}
	}
	if !MatchesPattern(board, pattern, called) {
		t.Error("full blackout pattern should match when all numbers called")
	}
}

// ── makeCalledSet ───────────────────────────────────────────────────────────

func TestMakeCalledSet(t *testing.T) {
	s := makeCalledSet([]int{5, 22, 48})
	if len(s) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(s))
	}
	for _, n := range []int{5, 22, 48} {
		if !s[n] {
			t.Errorf("expected %d in set", n)
		}
	}
	if s[99] {
		t.Error("99 should not be in set")
	}
}

func TestMakeCalledSet_Empty(t *testing.T) {
	s := makeCalledSet(nil)
	if len(s) != 0 {
		t.Errorf("expected empty set, got %d entries", len(s))
	}
}

// ── parseWinnersCache ───────────────────────────────────────────────────────

func TestParseWinnersCache_Empty(t *testing.T) {
	w := parseWinnersCache("")
	if len(w) != 0 {
		t.Errorf("expected empty slice, got %v", w)
	}
}

func TestParseWinnersCache_Valid(t *testing.T) {
	w := parseWinnersCache(`["ABC123","DEF456"]`)
	if len(w) != 2 || w[0] != "ABC123" || w[1] != "DEF456" {
		t.Errorf("unexpected result: %v", w)
	}
}

func TestParseWinnersCache_Invalid(t *testing.T) {
	w := parseWinnersCache("{bad json")
	if len(w) != 0 {
		t.Errorf("expected empty slice for bad JSON, got %v", w)
	}
}

func TestParseWinnersCache_Null(t *testing.T) {
	w := parseWinnersCache("null")
	if len(w) != 0 {
		t.Errorf("expected empty slice for null, got %v", w)
	}
}

// ── GameService integration (with real SQLite) ──────────────────────────────

func testStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func testPattern5x5() [][]bool {
	grid := make([][]bool, 5)
	for i := range grid {
		grid[i] = make([]bool, 5)
	}
	// Top-row pattern
	for c := 0; c < 5; c++ {
		grid[0][c] = true
	}
	return grid
}

func TestGameService_StartAndCurrentState(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)

	// No active game initially
	state, winners, err := gs.CurrentState()
	if err != nil {
		t.Fatal(err)
	}
	if state != nil {
		t.Error("expected nil state with no active game")
	}
	if len(winners) != 0 {
		t.Error("expected empty winners")
	}

	// Create a pattern and start a game
	patID, err := st.SavePattern("Top Row", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}

	gameState, err := gs.Start([]int{int(patID)})
	if err != nil {
		t.Fatal(err)
	}
	if gameState == nil {
		t.Fatal("expected non-nil game state after start")
	}
	if gameState.TotalCalled != 0 {
		t.Errorf("expected 0 called numbers, got %d", gameState.TotalCalled)
	}
	if len(gameState.Patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(gameState.Patterns))
	}

	// CurrentState should return the active game
	state, _, err = gs.CurrentState()
	if err != nil {
		t.Fatal(err)
	}
	if state == nil {
		t.Fatal("expected active game state")
	}
	if state.ID != gameState.ID {
		t.Errorf("game ID mismatch: %d != %d", state.ID, gameState.ID)
	}
}

func TestGameService_Draw(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)

	// Draw with no active game returns nil
	result, err := gs.Draw()
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil draw result with no active game")
	}

	// Start a game
	patID, err := st.SavePattern("Top Row", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Start([]int{int(patID)}); err != nil {
		t.Fatal(err)
	}

	// Draw a number
	result, err = gs.Draw()
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil draw result")
	}
	if result.Drawn.Number < 1 || result.Drawn.Number > 75 {
		t.Errorf("drawn number %d out of range [1,75]", result.Drawn.Number)
	}
	if result.Drawn.CallOrder != 1 {
		t.Errorf("expected call_order 1, got %d", result.Drawn.CallOrder)
	}
	if result.Drawn.Letter == "" {
		t.Error("expected non-empty letter")
	}
	if result.Game.TotalCalled != 1 {
		t.Errorf("expected 1 total called, got %d", result.Game.TotalCalled)
	}

	// Draw again — number should be different, call_order increments
	result2, err := gs.Draw()
	if err != nil {
		t.Fatal(err)
	}
	if result2.Drawn.Number == result.Drawn.Number {
		t.Error("second draw should produce a different number")
	}
	if result2.Drawn.CallOrder != 2 {
		t.Errorf("expected call_order 2, got %d", result2.Drawn.CallOrder)
	}
}

func TestGameService_Draw_AllNumbers(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)

	patID, err := st.SavePattern("Top Row", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Start([]int{int(patID)}); err != nil {
		t.Fatal(err)
	}

	// Draw all 75 numbers
	seen := make(map[int]bool)
	for i := 0; i < 75; i++ {
		result, err := gs.Draw()
		if err != nil {
			t.Fatalf("draw %d: %v", i+1, err)
		}
		if result == nil {
			t.Fatalf("draw %d: unexpected nil result", i+1)
		}
		if seen[result.Drawn.Number] {
			t.Fatalf("draw %d: duplicate number %d", i+1, result.Drawn.Number)
		}
		seen[result.Drawn.Number] = true
	}

	// 76th draw should return nil
	result, err := gs.Draw()
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil result after all 75 numbers drawn")
	}
}

func TestGameService_End(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)

	// End with no active game
	ended, err := gs.End()
	if err != nil {
		t.Fatal(err)
	}
	if ended {
		t.Error("expected false when no game active")
	}

	// Start and end a game
	patID, err := st.SavePattern("Top Row", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Start([]int{int(patID)}); err != nil {
		t.Fatal(err)
	}

	ended, err = gs.End()
	if err != nil {
		t.Fatal(err)
	}
	if !ended {
		t.Error("expected true after ending active game")
	}

	// End again should return false
	ended, err = gs.End()
	if err != nil {
		t.Fatal(err)
	}
	if ended {
		t.Error("expected false when no game active after ending")
	}
}

func TestGameService_StartEndsActiveGame(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)

	patID, err := st.SavePattern("Top Row", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}

	// Start first game
	g1, err := gs.Start([]int{int(patID)})
	if err != nil {
		t.Fatal(err)
	}

	// Start second game — should end the first
	g2, err := gs.Start([]int{int(patID)})
	if err != nil {
		t.Fatal(err)
	}
	if g2.ID == g1.ID {
		t.Error("new game should have a different ID")
	}

	// Only one active game should exist
	state, _, err := gs.CurrentState()
	if err != nil {
		t.Fatal(err)
	}
	if state.ID != g2.ID {
		t.Errorf("active game should be the latest: got %d, want %d", state.ID, g2.ID)
	}
}

func TestGameService_WinnerComputation(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)

	// Create a card and a pattern where we can force a win
	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	if err := st.SaveCard("WIN001", board); err != nil {
		t.Fatal(err)
	}

	// Top-row pattern: requires 1, 16, 31, 46, 61
	patID, err := st.SavePattern("Top Row", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := gs.Start([]int{int(patID)}); err != nil {
		t.Fatal(err)
	}

	// Draw all 75 numbers to guarantee the top-row numbers are called
	var lastResult *DrawResult
	for i := 0; i < 75; i++ {
		r, err := gs.Draw()
		if err != nil {
			t.Fatal(err)
		}
		if r == nil {
			break
		}
		lastResult = r
	}

	if lastResult == nil {
		t.Fatal("expected at least one draw result")
	}

	// After all numbers drawn, WIN001 must be a winner
	found := false
	for _, w := range lastResult.Winners {
		if w == "WIN001" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WIN001 in winners, got %v", lastResult.Winners)
	}
}

func TestGameService_InvalidateCardCache(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)

	// Populate cache by starting a game and drawing
	board := GenerateBoard()
	if err := st.SaveCard("CACHE1", board); err != nil {
		t.Fatal(err)
	}

	patID, err := st.SavePattern("Test", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Start([]int{int(patID)}); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Draw(); err != nil {
		t.Fatal(err)
	}

	// Invalidate and add a new card
	gs.InvalidateCardCache()
	if err := st.SaveCard("CACHE2", GenerateBoard()); err != nil {
		t.Fatal(err)
	}

	// Draw again — cache should be repopulated with both cards
	result, err := gs.Draw()
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result after cache invalidation")
	}
	// Winners list is computed from cached cards — we can't directly inspect
	// the cache, but the draw should succeed without error.
}

// ── MatchesPattern with GamePattern ─────────────────────────────────────────

func TestMatchesPattern_DiagonalPattern(t *testing.T) {
	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	// Diagonal pattern
	pattern := [][]bool{
		{true, false, false, false, false},
		{false, true, false, false, false},
		{false, false, true, false, false},
		{false, false, false, true, false},
		{false, false, false, false, true},
	}
	called := map[int]bool{1: true, 17: true, 49: true, 65: true}
	// FREE space at [2][2] is auto-counted, so we need 1, 17, 49, 65
	if !MatchesPattern(board, pattern, called) {
		t.Error("diagonal with FREE should match")
	}
}

func TestMatchesPattern_ColumnPattern(t *testing.T) {
	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	// First column (B column)
	pattern := [][]bool{
		{true, false, false, false, false},
		{true, false, false, false, false},
		{true, false, false, false, false},
		{true, false, false, false, false},
		{true, false, false, false, false},
	}
	called := map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true}
	if !MatchesPattern(board, pattern, called) {
		t.Error("B-column pattern should match")
	}
	// Remove one number
	delete(called, 3)
	if MatchesPattern(board, pattern, called) {
		t.Error("B-column should not match with missing number")
	}
}

// ── computeWinners edge cases ───────────────────────────────────────────────

func TestGameService_ComputeWinners_NoCardsOrPatterns(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)

	// No cards in DB, start a game and draw — should return empty winners
	patID, err := st.SavePattern("Test", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Start([]int{int(patID)}); err != nil {
		t.Fatal(err)
	}
	result, err := gs.Draw()
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Winners) != 0 {
		t.Errorf("expected 0 winners with no cards, got %d", len(result.Winners))
	}
}

func TestMatchesPattern_EmptyPattern(t *testing.T) {
	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	// All-false pattern — vacuously true
	pattern := make([][]bool, 5)
	for i := range pattern {
		pattern[i] = make([]bool, 5)
	}
	if !MatchesPattern(board, pattern, map[int]bool{}) {
		t.Error("all-false pattern should vacuously match")
	}
}

// ── Helpers: verify LetterForNumber agrees with drawn numbers ────────────────

func TestDrawResult_LetterMatchesNumber(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)

	patID, err := st.SavePattern("Test", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Start([]int{int(patID)}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		result, err := gs.Draw()
		if err != nil {
			t.Fatal(err)
		}
		if result == nil {
			break
		}
		want := LetterForNumber(result.Drawn.Number)
		if result.Drawn.Letter != want {
			t.Errorf("draw %d: number %d → letter %q; want %q",
				i+1, result.Drawn.Number, result.Drawn.Letter, want)
		}
	}
}

// ── MatchesPattern with GamePattern type (matches real usage) ───────────────

func TestMatchesPattern_WithGamePatternType(t *testing.T) {
	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	gp := model.BingoGamePattern{
		ID:   1,
		Name: "Top Row",
		PatternData: [][]bool{
			{true, true, true, true, true},
			{false, false, false, false, false},
			{false, false, false, false, false},
			{false, false, false, false, false},
			{false, false, false, false, false},
		},
	}
	called := map[int]bool{1: true, 16: true, 31: true, 46: true, 61: true}
	if !MatchesPattern(board, gp.PatternData, called) {
		t.Error("should match with GamePattern.PatternData")
	}
}

// TestGameService_ConcurrentDrawsNoDuplicates verifies that many simultaneous
// Draw() calls never produce a duplicate number or duplicate call order. This
// guards the opMu serialization that prevents a draw race (there is no UNIQUE
// constraint on called_numbers(game_id, number)).
func TestGameService_ConcurrentDrawsNoDuplicates(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)

	patID, err := st.SavePattern("Top Row", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Start([]int{int(patID)}); err != nil {
		t.Fatal(err)
	}

	// Launch more drawers than there are numbers (75) so some calls return nil.
	const goroutines = 120
	var wg sync.WaitGroup
	results := make(chan int, goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			res, err := gs.Draw()
			if err != nil {
				t.Errorf("draw error: %v", err)
				return
			}
			if res != nil {
				results <- res.Drawn.Number
			}
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[int]bool)
	count := 0
	for n := range results {
		if seen[n] {
			t.Errorf("number %d drawn more than once", n)
		}
		seen[n] = true
		count++
	}
	if count != 75 {
		t.Errorf("expected exactly 75 numbers drawn, got %d", count)
	}
}
