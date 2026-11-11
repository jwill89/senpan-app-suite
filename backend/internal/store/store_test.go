package store_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"app-suite/internal/db"
	"app-suite/internal/store"
	"app-suite/internal/store/storetest"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.Context(), storetest.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// -- Card operations ---------------------------------------------------------

func TestCardSaveAndGet(t *testing.T) {
	s := newTestStore(t)

	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	if err := s.SaveCard(t.Context(), "ABC123", board); err != nil {
		t.Fatal(err)
	}

	// Kept as the compact JSON the API writes, so the same numbers are the same text.
	card, err := db.New(s.ReadDB()).GetCard(t.Context(), "ABC123")
	if err != nil {
		t.Fatal(err)
	}
	if card.ID != "ABC123" || card.BoardData != "[[1,16,31,46,61],[2,17,32,47,62],[3,18,0,48,63],[4,19,34,49,64],[5,20,35,50,65]]" {
		t.Errorf("card = %+v", card)
	}
}

func TestCardGetNotFound(t *testing.T) {
	s := newTestStore(t)

	if _, err := db.New(s.ReadDB()).GetCard(t.Context(), "ZZZZZZ"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("a missing card: %v, want no rows", err)
	}
}

func TestCardExists(t *testing.T) {
	s := newTestStore(t)

	exists, err := s.CardExists(t.Context(), "ABC123")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("card should not exist yet")
	}

	board := [][]int{{1, 16, 31, 46, 61}, {2, 17, 32, 47, 62}, {3, 18, 0, 48, 63}, {4, 19, 34, 49, 64}, {5, 20, 35, 50, 65}}
	if err := s.SaveCard(t.Context(), "ABC123", board); err != nil {
		t.Fatal(err)
	}

	exists, err = s.CardExists(t.Context(), "ABC123")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("card should exist after save")
	}
}

// The cards a game checks for winners: every one but the personal cards waiting for
// approval, by id.
func TestCardListPlayable(t *testing.T) {
	s := newTestStore(t)
	q := db.New(s.DB())

	board := [][]int{{1, 16, 31, 46, 61}, {2, 17, 32, 47, 62}, {3, 18, 0, 48, 63}, {4, 19, 34, 49, 64}, {5, 20, 35, 50, 65}}
	for _, id := range []string{"BBB222", "AAA111"} {
		if err := s.SaveCard(t.Context(), id, board); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.CreateCardRequest(t.Context(), db.CreateCardRequestParams{ID: "PEND01", BoardData: "[]", PlayerName: "Aria", World: "Gilgamesh"}); err != nil {
		t.Fatal(err)
	}

	cards, err := q.ListPlayableCards(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 || cards[0].ID != "AAA111" || cards[1].ID != "BBB222" {
		t.Fatalf("playable cards = %+v; want AAA111, BBB222", cards)
	}
	names, err := q.ListCardPlayerNames(t.Context(), []string{"PEND01", "AAA111", "NOPE99"})
	if err != nil || len(names) != 2 {
		t.Errorf("player names = %+v, %v; want PEND01's and AAA111's", names, err)
	}
}

// -- Pattern operations -------------------------------------------------------

func testPattern() [][]bool {
	grid := make([][]bool, 5)
	for i := range grid {
		grid[i] = make([]bool, 5)
	}
	for c := range 5 {
		grid[0][c] = true
	}
	return grid
}

func TestPatternSaveAndList(t *testing.T) {
	s := newTestStore(t)

	id, err := s.SavePattern(t.Context(), "Top Row", testPattern(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	patterns, err := db.New(s.ReadDB()).ListPatterns(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	if patterns[0].Name != "Top Row" {
		t.Errorf("name = %q; want Top Row", patterns[0].Name)
	}
	var grid [][]bool
	if err := json.Unmarshal([]byte(patterns[0].PatternData), &grid); err != nil || len(grid) != 5 {
		t.Errorf("pattern data %q, want a 5-row grid", patterns[0].PatternData)
	}
	if patterns[0].SortOrder != 1 {
		t.Errorf("sort_order = %d; want 1", patterns[0].SortOrder)
	}
}

func TestPatternAutoIncrementSortOrder(t *testing.T) {
	s := newTestStore(t)

	_, _ = s.SavePattern(t.Context(), "Pat A", testPattern(), 1)
	_, _ = s.SavePattern(t.Context(), "Pat B", testPattern(), 1)
	_, _ = s.SavePattern(t.Context(), "Pat C", testPattern(), 1)

	patterns, _ := db.New(s.ReadDB()).ListPatterns(t.Context())
	if len(patterns) != 3 {
		t.Fatalf("expected 3, got %d", len(patterns))
	}
	for i, p := range patterns {
		if p.SortOrder != int64(i+1) {
			t.Errorf("pattern %d: sort_order = %d; want %d", i, p.SortOrder, i+1)
		}
	}
}

func TestPatternGetByIDs(t *testing.T) {
	s := newTestStore(t)

	idA, _ := s.SavePattern(t.Context(), "A", testPattern(), 1)
	_, _ = s.SavePattern(t.Context(), "B", testPattern(), 1)
	idC, _ := s.SavePattern(t.Context(), "C", testPattern(), 1)

	got, err := db.New(s.ReadDB()).ListPatternsByIDs(t.Context(), []int64{idC, idA, 999})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != idA || got[1].ID != idC {
		t.Fatalf("patterns = %+v; want A and C", got)
	}
}

func TestPatternGetByIDs_Empty(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.SavePattern(t.Context(), "A", testPattern(), 1); err != nil {
		t.Fatal(err)
	}

	got, err := db.New(s.ReadDB()).ListPatternsByIDs(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 patterns, got %d", len(got))
	}
}

// A grid is found again by the JSON it is kept as, which the API's duplicate check
// relies on: SavePattern and the API encode it the same way.
func TestPatternFoundByGrid(t *testing.T) {
	s := newTestStore(t)
	q := db.New(s.ReadDB())

	pat := testPattern()
	if _, err := s.SavePattern(t.Context(), "Original", pat, 1); err != nil {
		t.Fatal(err)
	}
	same, _ := json.Marshal(pat)
	dup, err := q.FindPatternByGrid(t.Context(), string(same))
	if err != nil || dup.Name != "Original" {
		t.Fatalf("the same grid: %+v, %v; want Original", dup, err)
	}

	pat[4][0] = true
	other, _ := json.Marshal(pat)
	if _, err := q.FindPatternByGrid(t.Context(), string(other)); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("another grid: %v, want no rows", err)
	}
}

// -- Game operations ---------------------------------------------------------

// A game's rows: made active with no winners, its patterns and calls read back in
// order, its winners kept, and ended once.
func TestGameQueries(t *testing.T) {
	s := newTestStore(t)
	q := db.New(s.DB())
	ctx := t.Context()

	if _, err := q.GetActiveGame(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("no game yet: %v, want no rows", err)
	}
	created, err := q.CreateGame(ctx)
	if err != nil {
		t.Fatal(err)
	}
	game, err := q.GetActiveGame(ctx)
	if err != nil || game.ID != created.ID || game.Status != "active" || game.WinnersCache != "[]" || game.CreatedAt == nil {
		t.Fatalf("the active game = %+v, %v", game, err)
	}

	for _, p := range []db.AddGamePatternParams{
		{GameID: created.ID, PatternID: 7, PatternName: "Bottom Row", PatternData: "[]"},
		{GameID: created.ID, PatternID: 2, PatternName: "Top Row", PatternData: "[]"},
	} {
		if err := q.AddGamePattern(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	patterns, err := q.ListGamePatterns(ctx, created.ID)
	if err != nil || len(patterns) != 2 || patterns[0].PatternName != "Top Row" {
		t.Errorf("patterns = %+v, %v; want Top Row first (by id)", patterns, err)
	}

	for i, n := range []int64{42, 7, 65} {
		if err := q.AddCalledNumber(ctx, db.AddCalledNumberParams{GameID: created.ID, Number: n, CallOrder: int64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	// The same number twice in a game is refused.
	if err := q.AddCalledNumber(ctx, db.AddCalledNumberParams{GameID: created.ID, Number: 7, CallOrder: 4}); err == nil {
		t.Error("a number called twice was taken")
	}
	numbers, err := q.ListCalledNumbers(ctx, created.ID)
	if err != nil || len(numbers) != 3 || numbers[0] != 42 || numbers[1] != 7 || numbers[2] != 65 {
		t.Errorf("called = %v, %v; want 42, 7, 65", numbers, err)
	}

	if err := q.SetWinnersCache(ctx, db.SetWinnersCacheParams{WinnersCache: `["WIN001"]`, ID: created.ID}); err != nil {
		t.Fatal(err)
	}
	if game, _ := q.GetActiveGame(ctx); game.WinnersCache != `["WIN001"]` {
		t.Errorf("winners = %q", game.WinnersCache)
	}

	if n, err := q.EndGame(ctx, created.ID); err != nil || n != 1 {
		t.Fatalf("end: %d, %v", n, err)
	}
	if n, _ := q.EndGame(ctx, created.ID); n != 0 {
		t.Errorf("ending it again changed %d rows", n)
	}
	if _, err := q.GetActiveGame(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("after the end: %v, want no rows", err)
	}
}

// -- Settings operations -----------------------------------------------------

func TestSettingsGetSet(t *testing.T) {
	s := newTestStore(t)

	// Get non-existent setting returns empty
	val, err := s.GetSetting(t.Context(), "test_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %q", val)
	}

	// Set and get
	if err := s.SetSetting(t.Context(), "test_key", "hello"); err != nil {
		t.Fatal(err)
	}
	val, err = s.GetSetting(t.Context(), "test_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "hello" {
		t.Errorf("expected hello, got %q", val)
	}

	// Overwrite
	if err := s.SetSetting(t.Context(), "test_key", "world"); err != nil {
		t.Fatal(err)
	}
	val, _ = s.GetSetting(t.Context(), "test_key")
	if val != "world" {
		t.Errorf("expected world, got %q", val)
	}
}

// -- Winners log operations --------------------------------------------------

// addWinners writes entries to the winners log, as a game's end does.
func addWinners(t *testing.T, s *store.Store, entries ...db.AddWinnerParams) {
	t.Helper()
	q := db.New(s.DB())
	for _, e := range entries {
		if err := q.AddWinner(t.Context(), e); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWinnersLogInsertAndList(t *testing.T) {
	s := newTestStore(t)

	addWinners(t, s,
		db.AddWinnerParams{CardID: "AAA111", PlayerName: "Alice", GameDetails: "Round 1", WinningPatterns: `["Top Row"]`},
		db.AddWinnerParams{CardID: "BBB222", PlayerName: "Bob", GameDetails: "Round 1", WinningPatterns: `["Full Card"]`},
	)

	q := db.New(s.ReadDB())
	if total, err := q.CountWinners(t.Context()); err != nil || total != 2 {
		t.Errorf("total = %d, %v; want 2", total, err)
	}
	results, err := q.ListWinners(t.Context(), db.ListWinnersParams{Sort: "cardId", Dir: "asc", PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].CardID != "AAA111" || results[1].PlayerName != "Bob" ||
		results[0].WinningPatterns != `["Top Row"]` || results[0].LoggedAt == nil {
		t.Errorf("results = %+v, %+v", results[0], results[1])
	}
}

// The log sorts by each of its columns, either way, and pages by size and offset.
func TestWinnersLogSortsAndPages(t *testing.T) {
	s := newTestStore(t)
	addWinners(t, s,
		db.AddWinnerParams{CardID: "C00002", PlayerName: "Alice", GameDetails: "b", WinningPatterns: "[]"},
		db.AddWinnerParams{CardID: "C00000", PlayerName: "Cyra", GameDetails: "c", WinningPatterns: "[]"},
		db.AddWinnerParams{CardID: "C00001", PlayerName: "Bob", GameDetails: "a", WinningPatterns: "[]"},
	)
	q := db.New(s.ReadDB())
	cards := func(sort, dir string, size, skip int64) string {
		t.Helper()
		rows, err := q.ListWinners(t.Context(), db.ListWinnersParams{Sort: sort, Dir: dir, PageSize: size, Skip: skip})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range rows {
			out = append(out, r.CardID[5:])
		}
		return strings.Join(out, "")
	}
	for _, c := range []struct {
		sort, dir string
		want      string
	}{
		{"cardId", "asc", "012"}, {"cardId", "desc", "210"},
		{"playerName", "asc", "210"}, {"gameDetails", "desc", "021"},
		// Logged in the same second, so the order they came in breaks the tie.
		{"loggedAt", "asc", "201"}, {"loggedAt", "desc", "102"},
	} {
		if got := cards(c.sort, c.dir, 10, 0); got != c.want {
			t.Errorf("%s %s: %s, want %s", c.sort, c.dir, got, c.want)
		}
	}
	if got := cards("cardId", "asc", 2, 1); got != "12" {
		t.Errorf("a page of 2 from the second: %s, want 12", got)
	}
}

// A frequent winner has at least the wins asked for since a time; a card issued to
// nobody never is one.
func TestFrequentWinners(t *testing.T) {
	s := newTestStore(t)
	addWinners(t, s,
		db.AddWinnerParams{CardID: "A1", PlayerName: "Alice", WinningPatterns: "[]"},
		db.AddWinnerParams{CardID: "A2", PlayerName: "Alice", WinningPatterns: "[]"},
		db.AddWinnerParams{CardID: "A3", PlayerName: "Alice", WinningPatterns: "[]"},
		db.AddWinnerParams{CardID: "B1", PlayerName: "Bob", WinningPatterns: "[]"},
		db.AddWinnerParams{CardID: "N1", WinningPatterns: "[]"}, db.AddWinnerParams{CardID: "N2", WinningPatterns: "[]"},
		db.AddWinnerParams{CardID: "N3", WinningPatterns: "[]"},
	)
	q := db.New(s.ReadDB())
	since := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	frequent, err := q.ListFrequentWinners(t.Context(), db.ListFrequentWinnersParams{Since: &since, MinWins: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(frequent) != 1 || frequent[0].PlayerName != "Alice" || frequent[0].WinCount != 3 {
		t.Fatalf("frequent winners = %+v; want Alice with 3", frequent)
	}
	later := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if frequent, _ = q.ListFrequentWinners(t.Context(), db.ListFrequentWinnersParams{Since: &later, MinWins: 1}); len(frequent) != 0 {
		t.Errorf("since a time after every win: %+v", frequent)
	}
}

// TestOpenNoMigrateLeavesTheSchemaAlone pins the read-only opener. themetool is
// routinely pointed at a copy of production, and a migrating opener would
// irreversibly upgrade that copy's schema just by INSPECTING it.
func TestOpenNoMigrateLeavesTheSchemaAlone(t *testing.T) {
	path := storetest.Path(t)

	// A database one version behind: created normally, then stamped backwards.
	s, err := store.New(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserVersionForTest(t.Context(), 1); err != nil {
		t.Fatalf("stamp user_version: %v", err)
	}
	before, err := s.UserVersionForTest(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	ro, err := store.OpenNoMigrate(path)
	if err != nil {
		t.Fatalf("OpenNoMigrate: %v", err)
	}
	after, err := ro.UserVersionForTest(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ro.Close()

	if after != before {
		t.Errorf("read-only open migrated the database: user_version %d -> %d", before, after)
	}

	// A missing file is an error, not a silently created empty database.
	if _, err := store.OpenNoMigrate(filepath.Join(t.TempDir(), "nope.db")); err == nil {
		t.Error("OpenNoMigrate created a database that did not exist")
	}
}
