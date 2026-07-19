package bingo

import "testing"

// TestPendingCustomCardNotEligibleToWin guards the cachedCards filter: a pending
// custom-card request must never be computed as a winner (it isn't playable until
// approved), while a normal card with the same winning row does win.
func TestPendingCustomCardNotEligibleToWin(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)

	// A normal card and a PENDING custom card that both complete the top-row pattern
	// (identical winning top row, different lower rows so they're distinct cards).
	normal := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	pending := [][]int{
		{1, 16, 31, 46, 61},
		{6, 21, 36, 51, 66},
		{7, 22, 0, 52, 67},
		{8, 23, 37, 53, 68},
		{9, 24, 38, 54, 69},
	}
	if err := st.SaveCard("WIN001", normal); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateCustomCard("PEND01", pending, "Aria", "Gilgamesh"); err != nil {
		t.Fatal(err)
	}

	patID, err := st.SavePattern("Top Row", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Start([]int{int(patID)}); err != nil {
		t.Fatal(err)
	}

	// Draw every number so both top rows are fully called.
	var last *DrawResult
	for i := 0; i < 75; i++ {
		r, err := gs.Draw()
		if err != nil {
			t.Fatal(err)
		}
		if r == nil {
			break
		}
		last = r
	}
	if last == nil {
		t.Fatal("expected at least one draw result")
	}

	var sawWin, sawPending bool
	for _, w := range last.Winners {
		switch w {
		case "WIN001":
			sawWin = true
		case "PEND01":
			sawPending = true
		}
	}
	if !sawWin {
		t.Errorf("normal card WIN001 should win, got winners %v", last.Winners)
	}
	if sawPending {
		t.Errorf("pending custom card PEND01 must NOT be eligible to win, got winners %v", last.Winners)
	}
}
