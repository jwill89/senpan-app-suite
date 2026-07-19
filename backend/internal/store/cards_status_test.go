package store_test

import "testing"

// validBoard returns a structurally valid 5×5 bingo board (centre = FREE).
func validBoard() [][]int {
	return [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
}

func TestCardProtectionAndDeleteAll(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveCard("KEEP01", validBoard()); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCard("GONE01", validBoard()); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.SetCardProtected("KEEP01", true); err != nil || !ok {
		t.Fatalf("SetCardProtected: ok=%v err=%v", ok, err)
	}

	deleted, err := s.DeleteAllCards()
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "GONE01" {
		t.Fatalf("DeleteAllCards deleted = %v; want [GONE01]", deleted)
	}

	kept, err := s.GetCard("KEEP01")
	if err != nil || kept == nil {
		t.Fatalf("protected card missing after Delete All: %v", err)
	}
	if !kept.Protected {
		t.Error("surviving card should still be Protected")
	}
	if gone, _ := s.GetCard("GONE01"); gone != nil {
		t.Error("unprotected card should have been deleted")
	}
}

func TestCustomCardLifecycle(t *testing.T) {
	s := newTestStore(t)
	board := validBoard()
	if err := s.CreateCustomCard("CUST01", board, "Aria Nightsong", "Gilgamesh"); err != nil {
		t.Fatal(err)
	}

	c, err := s.GetCard("CUST01")
	if err != nil || c == nil {
		t.Fatalf("GetCard: %v", err)
	}
	if c.CustomStatus != "pending" || c.Protected {
		t.Errorf("new custom card = status %q protected %v; want pending / false", c.CustomStatus, c.Protected)
	}
	if c.PlayerName != "Aria Nightsong" || c.World != "Gilgamesh" {
		t.Errorf("custom card identity = %q / %q", c.PlayerName, c.World)
	}

	// A matching board is detected as a duplicate.
	if id, dup, _ := s.FindDuplicateBoard(board); !dup || id != "CUST01" {
		t.Errorf("FindDuplicateBoard = %q,%v; want CUST01,true", id, dup)
	}

	// Approving flips it to approved + protected.
	if ok, err := s.ApproveCustomCard("CUST01"); err != nil || !ok {
		t.Fatalf("ApproveCustomCard: ok=%v err=%v", ok, err)
	}
	c, _ = s.GetCard("CUST01")
	if c.CustomStatus != "approved" || !c.Protected {
		t.Errorf("approved custom card = status %q protected %v; want approved / true", c.CustomStatus, c.Protected)
	}

	// Re-approving a non-pending card is a no-op.
	if ok, _ := s.ApproveCustomCard("CUST01"); ok {
		t.Error("re-approving an already-approved card should be a no-op")
	}

	// An approved card is auto-protected, so it survives Delete All.
	if deleted, _ := s.DeleteAllCards(); len(deleted) != 0 {
		t.Errorf("Delete All removed an approved custom card: %v", deleted)
	}
}

func TestFindDuplicateBoardDistinct(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveCard("AAA001", validBoard()); err != nil {
		t.Fatal(err)
	}
	// A different board (swap two numbers in the B column) is not a duplicate.
	other := validBoard()
	other[0][0], other[1][0] = other[1][0], other[0][0]
	if _, dup, _ := s.FindDuplicateBoard(other); dup {
		t.Error("a distinct board should not be flagged as a duplicate")
	}
}
