package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// ── Card operations ─────────────────────────────────────────────────────────

func TestCardSaveAndGet(t *testing.T) {
	s := newTestStore(t)

	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	if err := s.SaveCard("ABC123", board); err != nil {
		t.Fatal(err)
	}

	card, err := s.GetCard("ABC123")
	if err != nil {
		t.Fatal(err)
	}
	if card == nil {
		t.Fatal("expected card, got nil")
	}
	if card.ID != "ABC123" {
		t.Errorf("ID = %q; want ABC123", card.ID)
	}
	if len(card.BoardData) != 5 {
		t.Fatalf("board rows = %d; want 5", len(card.BoardData))
	}
	if card.BoardData[0][0] != 1 {
		t.Errorf("board[0][0] = %d; want 1", card.BoardData[0][0])
	}
	if card.BoardData[2][2] != 0 {
		t.Errorf("board[2][2] = %d; want 0", card.BoardData[2][2])
	}
}

func TestCardGetNotFound(t *testing.T) {
	s := newTestStore(t)

	card, err := s.GetCard("ZZZZZZ")
	if err != nil {
		t.Fatal(err)
	}
	if card != nil {
		t.Error("expected nil for missing card")
	}
}

func TestCardExists(t *testing.T) {
	s := newTestStore(t)

	exists, err := s.CardExists("ABC123")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("card should not exist yet")
	}

	board := [][]int{{1, 16, 31, 46, 61}, {2, 17, 32, 47, 62}, {3, 18, 0, 48, 63}, {4, 19, 34, 49, 64}, {5, 20, 35, 50, 65}}
	if err := s.SaveCard("ABC123", board); err != nil {
		t.Fatal(err)
	}

	exists, err = s.CardExists("ABC123")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("card should exist after save")
	}
}

func TestCardSaveBatch(t *testing.T) {
	s := newTestStore(t)

	board := [][]int{{1, 16, 31, 46, 61}, {2, 17, 32, 47, 62}, {3, 18, 0, 48, 63}, {4, 19, 34, 49, 64}, {5, 20, 35, 50, 65}}
	batch := []store.CardBatchEntry{
		{ID: "CARD01", Board: board},
		{ID: "CARD02", Board: board},
		{ID: "CARD03", Board: board},
	}
	if err := s.SaveCardsBatch(batch); err != nil {
		t.Fatal(err)
	}

	ids, err := s.ListCardIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 {
		t.Errorf("expected 3 card IDs, got %d", len(ids))
	}
}

func TestCardListCards(t *testing.T) {
	s := newTestStore(t)

	board := [][]int{{1, 16, 31, 46, 61}, {2, 17, 32, 47, 62}, {3, 18, 0, 48, 63}, {4, 19, 34, 49, 64}, {5, 20, 35, 50, 65}}
	if err := s.SaveCard("AAA111", board); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCard("BBB222", board); err != nil {
		t.Fatal(err)
	}

	cards, err := s.ListCards()
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}
	// Sorted by ID
	if cards[0].ID != "AAA111" {
		t.Errorf("first card ID = %q; want AAA111", cards[0].ID)
	}
	if len(cards[0].BoardData) != 5 {
		t.Error("expected decoded board data")
	}
}

func TestCardListIDs(t *testing.T) {
	s := newTestStore(t)

	board := [][]int{{1, 16, 31, 46, 61}, {2, 17, 32, 47, 62}, {3, 18, 0, 48, 63}, {4, 19, 34, 49, 64}, {5, 20, 35, 50, 65}}
	if err := s.SaveCard("AAA111", board); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCard("BBB222", board); err != nil {
		t.Fatal(err)
	}

	ids, err := s.ListCardIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	if ids[0] != "AAA111" || ids[1] != "BBB222" {
		t.Errorf("unexpected IDs: %v", ids)
	}
}

func TestCardDelete(t *testing.T) {
	s := newTestStore(t)

	board := [][]int{{1, 16, 31, 46, 61}, {2, 17, 32, 47, 62}, {3, 18, 0, 48, 63}, {4, 19, 34, 49, 64}, {5, 20, 35, 50, 65}}
	if err := s.SaveCard("DEL001", board); err != nil {
		t.Fatal(err)
	}

	deleted, err := s.DeleteCard("DEL001")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Error("expected true for deleted card")
	}

	// Delete again — should return false
	deleted, err = s.DeleteCard("DEL001")
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Error("expected false for already-deleted card")
	}
}

func TestCardDeleteAll(t *testing.T) {
	s := newTestStore(t)

	board := [][]int{{1, 16, 31, 46, 61}, {2, 17, 32, 47, 62}, {3, 18, 0, 48, 63}, {4, 19, 34, 49, 64}, {5, 20, 35, 50, 65}}
	for _, id := range []string{"DA0001", "DA0002", "DA0003"} {
		if err := s.SaveCard(id, board); err != nil {
			t.Fatal(err)
		}
	}

	count, err := s.DeleteAllCards()
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 deleted, got %d", count)
	}

	ids, err := s.ListCardIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 cards, got %d", len(ids))
	}
}

// ── Pattern operations ────────────────────────────────────────────────────��─

func testPattern() [][]bool {
	grid := make([][]bool, 5)
	for i := range grid {
		grid[i] = make([]bool, 5)
	}
	for c := 0; c < 5; c++ {
		grid[0][c] = true
	}
	return grid
}

func TestPatternSaveAndList(t *testing.T) {
	s := newTestStore(t)

	id, err := s.SavePattern("Top Row", testPattern(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	patterns, err := s.ListPatterns()
	if err != nil {
		t.Fatal(err)
	}
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	if patterns[0].Name != "Top Row" {
		t.Errorf("name = %q; want Top Row", patterns[0].Name)
	}
	if len(patterns[0].PatternData) != 5 {
		t.Error("expected decoded 5-row pattern data")
	}
	if patterns[0].SortOrder != 1 {
		t.Errorf("sort_order = %d; want 1", patterns[0].SortOrder)
	}
}

func TestPatternAutoIncrementSortOrder(t *testing.T) {
	s := newTestStore(t)

	_, _ = s.SavePattern("Pat A", testPattern(), 1)
	_, _ = s.SavePattern("Pat B", testPattern(), 1)
	_, _ = s.SavePattern("Pat C", testPattern(), 1)

	patterns, _ := s.ListPatterns()
	if len(patterns) != 3 {
		t.Fatalf("expected 3, got %d", len(patterns))
	}
	for i, p := range patterns {
		if p.SortOrder != i+1 {
			t.Errorf("pattern %d: sort_order = %d; want %d", i, p.SortOrder, i+1)
		}
	}
}

func TestPatternGetByIDs(t *testing.T) {
	s := newTestStore(t)

	idA, _ := s.SavePattern("A", testPattern(), 1)
	_, _ = s.SavePattern("B", testPattern(), 1)
	idC, _ := s.SavePattern("C", testPattern(), 1)

	got, err := s.GetPatternsByIDs([]int{int(idA), int(idC)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(got))
	}
}

func TestPatternGetByIDs_Empty(t *testing.T) {
	s := newTestStore(t)

	got, err := s.GetPatternsByIDs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 patterns, got %d", len(got))
	}
}

func TestPatternRename(t *testing.T) {
	s := newTestStore(t)

	id, _ := s.SavePattern("Old Name", testPattern(), 1)

	renamed, err := s.RenamePattern(int(id), "New Name")
	if err != nil {
		t.Fatal(err)
	}
	if !renamed {
		t.Error("expected true for renamed pattern")
	}

	patterns, _ := s.ListPatterns()
	if patterns[0].Name != "New Name" {
		t.Errorf("name = %q; want New Name", patterns[0].Name)
	}
}

func TestPatternRename_NotFound(t *testing.T) {
	s := newTestStore(t)

	renamed, err := s.RenamePattern(9999, "X")
	if err != nil {
		t.Fatal(err)
	}
	if renamed {
		t.Error("expected false for non-existent pattern")
	}
}

func TestPatternDelete(t *testing.T) {
	s := newTestStore(t)

	id, _ := s.SavePattern("To Delete", testPattern(), 1)

	deleted, err := s.DeletePattern(int(id))
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Error("expected true for deleted pattern")
	}

	deleted, _ = s.DeletePattern(int(id))
	if deleted {
		t.Error("expected false for already-deleted pattern")
	}
}

func TestPatternMove(t *testing.T) {
	s := newTestStore(t)

	_, _ = s.SavePattern("A", testPattern(), 1)
	idB, _ := s.SavePattern("B", testPattern(), 1)
	_, _ = s.SavePattern("C", testPattern(), 1)

	// Move B up → order should be B, A, C
	moved, err := s.MovePattern(int(idB), "up")
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Error("expected true for move up")
	}

	patterns, _ := s.ListPatterns()
	if patterns[0].Name != "B" || patterns[1].Name != "A" || patterns[2].Name != "C" {
		names := make([]string, len(patterns))
		for i, p := range patterns {
			names[i] = p.Name
		}
		t.Errorf("unexpected order after move up: %v", names)
	}
}

func TestPatternMove_Down(t *testing.T) {
	s := newTestStore(t)

	_, _ = s.SavePattern("A", testPattern(), 1)
	idB, _ := s.SavePattern("B", testPattern(), 1)
	_, _ = s.SavePattern("C", testPattern(), 1)

	// Move B down → order should be A, C, B
	moved, err := s.MovePattern(int(idB), "down")
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Error("expected true for move down")
	}

	patterns, _ := s.ListPatterns()
	if patterns[0].Name != "A" || patterns[1].Name != "C" || patterns[2].Name != "B" {
		names := make([]string, len(patterns))
		for i, p := range patterns {
			names[i] = p.Name
		}
		t.Errorf("unexpected order after move down: %v", names)
	}
}

func TestPatternMove_Boundary(t *testing.T) {
	s := newTestStore(t)

	idA, _ := s.SavePattern("A", testPattern(), 1)

	// Moving the only pattern up — should be a no-op
	moved, err := s.MovePattern(int(idA), "up")
	if err != nil {
		t.Fatal(err)
	}
	if moved {
		t.Error("expected false when at boundary")
	}
}

func TestPatternMove_NotFound(t *testing.T) {
	s := newTestStore(t)

	moved, err := s.MovePattern(9999, "up")
	if err != nil {
		t.Fatal(err)
	}
	if moved {
		t.Error("expected false for non-existent pattern")
	}
}

// ── Game operations ─────────────────────────────────────────────────────────

func TestGameCreateAndGetActive(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateGame()
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	game, err := s.GetActiveGame()
	if err != nil {
		t.Fatal(err)
	}
	if game == nil {
		t.Fatal("expected active game")
	}
	if game.ID != id {
		t.Errorf("ID = %d; want %d", game.ID, id)
	}
	if game.Status != "active" {
		t.Errorf("status = %q; want active", game.Status)
	}
	if game.WinnersCache != "[]" {
		t.Errorf("winners_cache = %q; want []", game.WinnersCache)
	}
}

func TestGameGetActive_None(t *testing.T) {
	s := newTestStore(t)

	game, err := s.GetActiveGame()
	if err != nil {
		t.Fatal(err)
	}
	if game != nil {
		t.Error("expected nil when no active game")
	}
}

func TestGameEnd(t *testing.T) {
	s := newTestStore(t)

	id, _ := s.CreateGame()
	if err := s.EndGame(id); err != nil {
		t.Fatal(err)
	}

	game, err := s.GetActiveGame()
	if err != nil {
		t.Fatal(err)
	}
	if game != nil {
		t.Error("expected nil after ending game")
	}
}

func TestGamePatterns(t *testing.T) {
	s := newTestStore(t)

	gameID, _ := s.CreateGame()

	pat := testPattern()
	if err := s.AddGamePattern(gameID, 1, "Top Row", pat); err != nil {
		t.Fatal(err)
	}

	patterns, err := s.GetGamePatterns(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(patterns) != 1 {
		t.Fatalf("expected 1 game pattern, got %d", len(patterns))
	}
	if patterns[0].Name != "Top Row" {
		t.Errorf("name = %q; want Top Row", patterns[0].Name)
	}
	if len(patterns[0].PatternData) != 5 {
		t.Error("expected decoded 5-row pattern data")
	}
}

func TestCalledNumbers(t *testing.T) {
	s := newTestStore(t)

	gameID, _ := s.CreateGame()

	if err := s.AddCalledNumber(gameID, 42, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCalledNumber(gameID, 7, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCalledNumber(gameID, 65, 3); err != nil {
		t.Fatal(err)
	}

	nums, err := s.GetCalledNumbers(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(nums) != 3 {
		t.Fatalf("expected 3 numbers, got %d", len(nums))
	}
	// Should be ordered by call_order
	if nums[0] != 42 || nums[1] != 7 || nums[2] != 65 {
		t.Errorf("unexpected order: %v", nums)
	}
}

func TestWinnersCache(t *testing.T) {
	s := newTestStore(t)

	gameID, _ := s.CreateGame()

	if err := s.UpdateWinnersCache(gameID, []string{"WIN001", "WIN002"}); err != nil {
		t.Fatal(err)
	}

	game, _ := s.GetActiveGame()
	if game.WinnersCache != `["WIN001","WIN002"]` {
		t.Errorf("winners_cache = %q", game.WinnersCache)
	}
}

func TestWinnersCache_Nil(t *testing.T) {
	s := newTestStore(t)

	gameID, _ := s.CreateGame()

	if err := s.UpdateWinnersCache(gameID, nil); err != nil {
		t.Fatal(err)
	}

	game, _ := s.GetActiveGame()
	if game.WinnersCache != "[]" {
		t.Errorf("winners_cache = %q; want []", game.WinnersCache)
	}
}

func TestCalledNumbers_Empty(t *testing.T) {
	s := newTestStore(t)

	gameID, _ := s.CreateGame()

	nums, err := s.GetCalledNumbers(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(nums) != 0 {
		t.Errorf("expected empty, got %v", nums)
	}
}

func TestGamePatterns_Empty(t *testing.T) {
	s := newTestStore(t)

	gameID, _ := s.CreateGame()

	patterns, err := s.GetGamePatterns(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(patterns) != 0 {
		t.Errorf("expected empty, got %d", len(patterns))
	}
}

// ── Raffle operations ───────────────────────────────────────────────────────

func TestRaffleCreateAndGet(t *testing.T) {
	s := newTestStore(t)

	r := &model.Raffle{
		Title:        "Test Raffle",
		Description:  "Win a prize!",
		Rules:        "Be nice",
		MaxEntries:   5,
		CostPerEntry: 100,
	}
	id, err := s.CreateRaffle(r)
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	got, err := s.GetRaffle(id)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected raffle, got nil")
	}
	if got.Title != "Test Raffle" {
		t.Errorf("title = %q; want Test Raffle", got.Title)
	}
	if got.Status != "open" {
		t.Errorf("status = %q; want open", got.Status)
	}
	if got.MaxEntries != 5 {
		t.Errorf("max_entries = %d; want 5", got.MaxEntries)
	}
}

func TestRaffleGetNotFound(t *testing.T) {
	s := newTestStore(t)

	got, err := s.GetRaffle(9999)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil for non-existent raffle")
	}
}

func TestRaffleUpdate(t *testing.T) {
	s := newTestStore(t)

	r := &model.Raffle{Title: "Original", MaxEntries: 1}
	id, _ := s.CreateRaffle(r)

	updated := &model.Raffle{ID: id, Title: "Updated", MaxEntries: 10, CostPerEntry: 500}
	if err := s.UpdateRaffle(updated); err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetRaffle(id)
	if got.Title != "Updated" {
		t.Errorf("title = %q; want Updated", got.Title)
	}
	if got.CostPerEntry != 500 {
		t.Errorf("cost_per_entry = %v; want 500", got.CostPerEntry)
	}
}

func TestRaffleDelete(t *testing.T) {
	s := newTestStore(t)

	r := &model.Raffle{Title: "To Delete", MaxEntries: 1}
	id, _ := s.CreateRaffle(r)

	deleted, err := s.DeleteRaffle(id)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Error("expected deleted=true")
	}

	got, _ := s.GetRaffle(id)
	if got != nil {
		t.Error("expected nil after deletion")
	}
}

func TestRaffleDelete_NotFound(t *testing.T) {
	s := newTestStore(t)

	deleted, err := s.DeleteRaffle(9999)
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Error("expected deleted=false for non-existent raffle")
	}
}

func TestRaffleListAdmin(t *testing.T) {
	s := newTestStore(t)

	_, _ = s.CreateRaffle(&model.Raffle{Title: "R1", MaxEntries: 1})
	_, _ = s.CreateRaffle(&model.Raffle{Title: "R2", MaxEntries: 1})

	raffles, err := s.ListRaffles(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(raffles) != 2 {
		t.Errorf("expected 2 raffles, got %d", len(raffles))
	}
}

func TestRaffleListPublic_OnlyOpen(t *testing.T) {
	s := newTestStore(t)

	id1, _ := s.CreateRaffle(&model.Raffle{Title: "Open", MaxEntries: 1})
	id2, _ := s.CreateRaffle(&model.Raffle{Title: "Closed", MaxEntries: 1})
	_ = id1
	_ = s.SetRaffleStatus(id2, "closed")

	raffles, err := s.ListRaffles(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(raffles) != 1 {
		t.Errorf("expected 1 open raffle, got %d", len(raffles))
	}
	if raffles[0].Title != "Open" {
		t.Errorf("expected Open raffle, got %q", raffles[0].Title)
	}
}

func TestRaffleEntryCreateAndList(t *testing.T) {
	s := newTestStore(t)

	rID, _ := s.CreateRaffle(&model.Raffle{Title: "R", MaxEntries: 5})

	eID, err := s.CreateRaffleEntry(rID, "Player1", "World1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if eID <= 0 {
		t.Errorf("expected positive entry ID, got %d", eID)
	}

	entries, err := s.ListRaffleEntries(rID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].CharacterName != "Player1" {
		t.Errorf("character_name = %q; want Player1", entries[0].CharacterName)
	}
	if entries[0].NumEntries != 3 {
		t.Errorf("num_entries = %d; want 3", entries[0].NumEntries)
	}
}

func TestRaffleEntryGetByCharWorld(t *testing.T) {
	s := newTestStore(t)

	rID, _ := s.CreateRaffle(&model.Raffle{Title: "R", MaxEntries: 5})
	_, _ = s.CreateRaffleEntry(rID, "Player1", "World1", 2)

	// Exact match
	entry, err := s.GetRaffleEntry(rID, "Player1", "World1")
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil {
		t.Fatal("expected entry")
	}

	// Case-insensitive match
	entry, err = s.GetRaffleEntry(rID, "player1", "world1")
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil {
		t.Fatal("expected case-insensitive match")
	}

	// No match
	entry, err = s.GetRaffleEntry(rID, "Nobody", "Nowhere")
	if err != nil {
		t.Fatal(err)
	}
	if entry != nil {
		t.Error("expected nil for non-existent entry")
	}
}

func TestRaffleAddEntries(t *testing.T) {
	s := newTestStore(t)

	rID, _ := s.CreateRaffle(&model.Raffle{Title: "R", MaxEntries: 10})
	eID, _ := s.CreateRaffleEntry(rID, "Player1", "World1", 2)

	if err := s.AddRaffleEntries(eID, 3); err != nil {
		t.Fatal(err)
	}

	entry, _ := s.GetRaffleEntry(rID, "Player1", "World1")
	if entry.NumEntries != 5 {
		t.Errorf("num_entries = %d; want 5", entry.NumEntries)
	}
}

func TestRaffleCountEntries(t *testing.T) {
	s := newTestStore(t)

	rID, _ := s.CreateRaffle(&model.Raffle{Title: "R", MaxEntries: 10})
	_, _ = s.CreateRaffleEntry(rID, "P1", "W1", 3)
	_, _ = s.CreateRaffleEntry(rID, "P2", "W1", 5)

	count, err := s.CountRaffleEntries(rID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 8 {
		t.Errorf("count = %d; want 8", count)
	}
}

func TestRaffleCountEntries_Empty(t *testing.T) {
	s := newTestStore(t)

	rID, _ := s.CreateRaffle(&model.Raffle{Title: "R", MaxEntries: 1})

	count, err := s.CountRaffleEntries(rID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("count = %d; want 0", count)
	}
}

func TestRaffleEntryPaid(t *testing.T) {
	s := newTestStore(t)

	rID, _ := s.CreateRaffle(&model.Raffle{Title: "R", MaxEntries: 5})
	eID, _ := s.CreateRaffleEntry(rID, "P1", "W1", 1)

	// Initially not paid
	entry, _ := s.GetRaffleEntry(rID, "P1", "W1")
	if entry.Paid {
		t.Error("expected unpaid initially")
	}

	// Mark paid
	if err := s.SetRaffleEntryPaid(eID, true); err != nil {
		t.Fatal(err)
	}
	entry, _ = s.GetRaffleEntry(rID, "P1", "W1")
	if !entry.Paid {
		t.Error("expected paid after SetRaffleEntryPaid(true)")
	}

	// Unmark
	if err := s.SetRaffleEntryPaid(eID, false); err != nil {
		t.Fatal(err)
	}
	entry, _ = s.GetRaffleEntry(rID, "P1", "W1")
	if entry.Paid {
		t.Error("expected unpaid after SetRaffleEntryPaid(false)")
	}
}

func TestRaffleDeleteEntry(t *testing.T) {
	s := newTestStore(t)

	rID, _ := s.CreateRaffle(&model.Raffle{Title: "R", MaxEntries: 5})
	eID, _ := s.CreateRaffleEntry(rID, "P1", "W1", 1)

	deleted, err := s.DeleteRaffleEntry(eID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Error("expected deleted=true")
	}

	entries, _ := s.ListRaffleEntries(rID)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after delete, got %d", len(entries))
	}
}

func TestRaffleDeleteEntry_NotFound(t *testing.T) {
	s := newTestStore(t)

	deleted, err := s.DeleteRaffleEntry(9999)
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Error("expected deleted=false")
	}
}

func TestRafflePickWinner(t *testing.T) {
	s := newTestStore(t)

	rID, _ := s.CreateRaffle(&model.Raffle{Title: "R", MaxEntries: 5})
	eID, _ := s.CreateRaffleEntry(rID, "P1", "W1", 3)
	_ = s.SetRaffleEntryPaid(eID, true)

	winner, err := s.PickRaffleWinner(rID)
	if err != nil {
		t.Fatal(err)
	}
	if winner == nil {
		t.Fatal("expected a winner")
	}
	if winner.CharacterName != "P1" {
		t.Errorf("winner = %q; want P1", winner.CharacterName)
	}
}

func TestRafflePickWinner_NoPaidEntries(t *testing.T) {
	s := newTestStore(t)

	rID, _ := s.CreateRaffle(&model.Raffle{Title: "R", MaxEntries: 5})
	_, _ = s.CreateRaffleEntry(rID, "P1", "W1", 3) // not paid

	winner, err := s.PickRaffleWinner(rID)
	if err != nil {
		t.Fatal(err)
	}
	if winner != nil {
		t.Error("expected nil winner when no paid entries")
	}
}

func TestRafflePickWinner_NoEntries(t *testing.T) {
	s := newTestStore(t)

	rID, _ := s.CreateRaffle(&model.Raffle{Title: "R", MaxEntries: 5})

	winner, err := s.PickRaffleWinner(rID)
	if err != nil {
		t.Fatal(err)
	}
	if winner != nil {
		t.Error("expected nil winner when no entries")
	}
}

func TestRaffleSetWinnerAndStatus(t *testing.T) {
	s := newTestStore(t)

	rID, _ := s.CreateRaffle(&model.Raffle{Title: "R", MaxEntries: 5})
	eID, _ := s.CreateRaffleEntry(rID, "P1", "W1", 1)
	_ = s.SetRaffleEntryPaid(eID, true)

	// Set winner
	if err := s.SetRaffleWinner(rID, &eID); err != nil {
		t.Fatal(err)
	}
	r, _ := s.GetRaffle(rID)
	if r.WinnerEntryID == nil || *r.WinnerEntryID != eID {
		t.Errorf("winner_entry_id = %v; want %d", r.WinnerEntryID, eID)
	}

	// Close raffle
	if err := s.SetRaffleStatus(rID, "closed"); err != nil {
		t.Fatal(err)
	}
	r, _ = s.GetRaffle(rID)
	if r.Status != "closed" {
		t.Errorf("status = %q; want closed", r.Status)
	}

	// Clear winner
	if err := s.SetRaffleWinner(rID, nil); err != nil {
		t.Fatal(err)
	}
	r, _ = s.GetRaffle(rID)
	if r.WinnerEntryID != nil {
		t.Error("expected nil winner_entry_id after clearing")
	}
}

// ── Settings operations ─────────────────────────────────────────────────────

func TestSettingsGetSet(t *testing.T) {
	s := newTestStore(t)

	// Get non-existent setting returns empty
	val, err := s.GetSetting("test_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %q", val)
	}

	// Set and get
	if err := s.SetSetting("test_key", "hello"); err != nil {
		t.Fatal(err)
	}
	val, err = s.GetSetting("test_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "hello" {
		t.Errorf("expected hello, got %q", val)
	}

	// Overwrite
	if err := s.SetSetting("test_key", "world"); err != nil {
		t.Fatal(err)
	}
	val, _ = s.GetSetting("test_key")
	if val != "world" {
		t.Errorf("expected world, got %q", val)
	}
}

// ── Style operations ─────────────────────────���──────────────────────────────

func TestStyleCRUD(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateStyle("Dark Theme", "body { background: #000; }")
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	style, err := s.GetStyle(id)
	if err != nil {
		t.Fatal(err)
	}
	if style.Name != "Dark Theme" {
		t.Errorf("name = %q; want Dark Theme", style.Name)
	}
	if style.CSSContent != "body { background: #000; }" {
		t.Errorf("css = %q", style.CSSContent)
	}

	// Update
	if err := s.UpdateStyle(id, "Light Theme", "body { background: #fff; }"); err != nil {
		t.Fatal(err)
	}
	style, _ = s.GetStyle(id)
	if style.Name != "Light Theme" {
		t.Errorf("name after update = %q", style.Name)
	}

	// List
	styles, err := s.ListStyles()
	if err != nil {
		t.Fatal(err)
	}
	if len(styles) != 1 {
		t.Errorf("expected 1 style, got %d", len(styles))
	}

	// Delete
	deleted, err := s.DeleteStyle(id)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Error("expected deleted=true")
	}
	style, _ = s.GetStyle(id)
	if style != nil {
		t.Error("expected nil after delete")
	}
}

func TestStyleGetNotFound(t *testing.T) {
	s := newTestStore(t)

	style, err := s.GetStyle(9999)
	if err != nil {
		t.Fatal(err)
	}
	if style != nil {
		t.Error("expected nil")
	}
}

func TestActiveStyleCSS(t *testing.T) {
	s := newTestStore(t)

	// No active style
	css, err := s.GetActiveStyleCSS()
	if err != nil {
		t.Fatal(err)
	}
	if css != "" {
		t.Errorf("expected empty CSS, got %q", css)
	}

	// Create and activate
	id, _ := s.CreateStyle("Test", ".foo { color: red; }")
	_ = s.SetSetting("active_style_id", fmt.Sprintf("%d", id))

	css, err = s.GetActiveStyleCSS()
	if err != nil {
		t.Fatal(err)
	}
	if css != ".foo { color: red; }" {
		t.Errorf("active CSS = %q", css)
	}
}

// ── Winners log operations ──────────────────────────────────────────────────

func TestWinnersLogInsertAndList(t *testing.T) {
	s := newTestStore(t)

	entries := []model.WinnersLogEntry{
		{CardID: "AAA111", PlayerName: "Alice", GameDetails: "Round 1", WinningPatterns: `["Top Row"]`},
		{CardID: "BBB222", PlayerName: "Bob", GameDetails: "Round 1", WinningPatterns: `["Full Card"]`},
	}
	if err := s.InsertWinnersLog(entries); err != nil {
		t.Fatal(err)
	}

	results, total, err := s.ListWinnersLog(10, 0, "logged_at", "DESC")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("total = %d; want 2", total)
	}
	if len(results) != 2 {
		t.Errorf("results = %d; want 2", len(results))
	}
}

func TestWinnersLogPagination(t *testing.T) {
	s := newTestStore(t)

	entries := make([]model.WinnersLogEntry, 5)
	for i := range entries {
		entries[i] = model.WinnersLogEntry{
			CardID: fmt.Sprintf("C%05d", i), PlayerName: "P", WinningPatterns: "[]",
		}
	}
	_ = s.InsertWinnersLog(entries)

	results, total, err := s.ListWinnersLog(2, 0, "logged_at", "DESC")
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("total = %d; want 5", total)
	}
	if len(results) != 2 {
		t.Errorf("page size = %d; want 2", len(results))
	}

	// Second page
	results, total, err = s.ListWinnersLog(2, 2, "logged_at", "DESC")
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("total = %d; want 5", total)
	}
	if len(results) != 2 {
		t.Errorf("page 2 size = %d; want 2", len(results))
	}
}

func TestWinnersLogDelete(t *testing.T) {
	s := newTestStore(t)

	entries := []model.WinnersLogEntry{
		{CardID: "AAA111", PlayerName: "Alice", WinningPatterns: "[]"},
		{CardID: "BBB222", PlayerName: "Bob", WinningPatterns: "[]"},
		{CardID: "CCC333", PlayerName: "Cara", WinningPatterns: "[]"},
	}
	if err := s.InsertWinnersLog(entries); err != nil {
		t.Fatal(err)
	}

	// Delete one by id (read an id back from the list).
	rows, _, err := s.ListWinnersLog(10, 0, "logged_at", "DESC")
	if err != nil {
		t.Fatal(err)
	}
	if deleted, err := s.DeleteWinnerLogEntry(rows[0].ID); err != nil || !deleted {
		t.Fatalf("DeleteWinnerLogEntry = %v, err=%v; want true", deleted, err)
	}
	if _, total, _ := s.ListWinnersLog(10, 0, "logged_at", "DESC"); total != 2 {
		t.Errorf("total after single delete = %d; want 2", total)
	}

	// Deleting a non-existent id reports false, no error.
	if deleted, err := s.DeleteWinnerLogEntry(999999); err != nil || deleted {
		t.Errorf("DeleteWinnerLogEntry(missing) = %v, err=%v; want false, nil", deleted, err)
	}

	// Delete-all clears the log and reports how many rows went.
	n, err := s.DeleteAllWinnersLog()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("DeleteAllWinnersLog count = %d; want 2", n)
	}
	if _, total, _ := s.ListWinnersLog(10, 0, "logged_at", "DESC"); total != 0 {
		t.Errorf("total after delete-all = %d; want 0", total)
	}
}

func TestFrequentWinners(t *testing.T) {
	s := newTestStore(t)

	// Insert 3 wins for Alice (should appear as frequent with minWins=3)
	entries := []model.WinnersLogEntry{
		{CardID: "A1", PlayerName: "Alice", WinningPatterns: "[]"},
		{CardID: "A2", PlayerName: "Alice", WinningPatterns: "[]"},
		{CardID: "A3", PlayerName: "Alice", WinningPatterns: "[]"},
		{CardID: "B1", PlayerName: "Bob", WinningPatterns: "[]"},
	}
	_ = s.InsertWinnersLog(entries)

	frequent, err := s.FrequentWinners(3, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(frequent) != 1 {
		t.Fatalf("expected 1 frequent winner, got %d", len(frequent))
	}
	if frequent[0].PlayerName != "Alice" {
		t.Errorf("frequent winner = %q; want Alice", frequent[0].PlayerName)
	}
	if frequent[0].WinCount != 3 {
		t.Errorf("win count = %d; want 3", frequent[0].WinCount)
	}
}

func TestDuplicatePatternDetection(t *testing.T) {
	s := newTestStore(t)

	pat := testPattern()
	_, _ = s.SavePattern("Original", pat, 1)

	dup, err := s.FindDuplicatePattern(pat)
	if err != nil {
		t.Fatal(err)
	}
	if dup == nil {
		t.Fatal("expected duplicate detected")
	}
	if dup.Name != "Original" {
		t.Errorf("dup name = %q; want Original", dup.Name)
	}

	// Different pattern should not be duplicate
	pat2 := testPattern()
	pat2[4][0] = true // modify one cell
	dup, err = s.FindDuplicatePattern(pat2)
	if err != nil {
		t.Fatal(err)
	}
	if dup != nil {
		t.Error("expected no duplicate for different pattern")
	}
}
