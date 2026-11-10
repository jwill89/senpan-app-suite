package bingo

import "testing"

// testBoard is a card whose top row is B1, I16, N31, G46, O61.
func testBoard() [][]int {
	return [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
}

// grid is a 5x5 pattern marking the cells given as {row, col}.
func grid(cells ...[2]int) [][]bool {
	g := make([][]bool, 5)
	for i := range g {
		g[i] = make([]bool, 5)
	}
	for _, rc := range cells {
		g[rc[0]][rc[1]] = true
	}
	return g
}

func topRow() [][]bool {
	return grid([2]int{0, 0}, [2]int{0, 1}, [2]int{0, 2}, [2]int{0, 3}, [2]int{0, 4})
}

// postageStamps is a 2x2 block in each corner: columns B, I, G and O, never N.
func postageStamps() [][]bool {
	return grid(
		[2]int{0, 0}, [2]int{0, 1}, [2]int{1, 0}, [2]int{1, 1},
		[2]int{0, 3}, [2]int{0, 4}, [2]int{1, 3}, [2]int{1, 4},
		[2]int{3, 0}, [2]int{3, 1}, [2]int{4, 0}, [2]int{4, 1},
		[2]int{3, 3}, [2]int{3, 4}, [2]int{4, 3}, [2]int{4, 4},
	)
}

func TestMatchesPattern(t *testing.T) {
	all := make([][]bool, 5)
	for r := range all {
		all[r] = []bool{true, true, true, true, true}
	}
	everyNumber := map[int]bool{}
	for _, row := range testBoard() {
		for _, v := range row {
			everyNumber[v] = true
		}
	}
	for _, c := range []struct {
		name    string
		board   [][]int
		pattern [][]bool
		called  map[int]bool
		want    bool
	}{
		{"the top row called", testBoard(), topRow(), CalledSet([]int{1, 16, 31, 46, 61}), true},
		{"one number short", testBoard(), topRow(), CalledSet([]int{1, 16, 31, 46}), false},
		{"the free space alone", testBoard(), grid([2]int{2, 2}), map[int]bool{}, true},
		{"nothing called", testBoard(), grid([2]int{0, 0}), map[int]bool{}, false},
		{"a board too small", [][]int{{1, 2}}, topRow(), CalledSet([]int{1, 2}), false},
		{"a blackout, every number called", testBoard(), all, everyNumber, true},
		{"a diagonal through the free space", testBoard(),
			grid([2]int{0, 0}, [2]int{1, 1}, [2]int{2, 2}, [2]int{3, 3}, [2]int{4, 4}), CalledSet([]int{1, 17, 49, 65}), true},
		{"the B column", testBoard(),
			grid([2]int{0, 0}, [2]int{1, 0}, [2]int{2, 0}, [2]int{3, 0}, [2]int{4, 0}), CalledSet([]int{1, 2, 3, 4, 5}), true},
		{"the B column, one short", testBoard(),
			grid([2]int{0, 0}, [2]int{1, 0}, [2]int{2, 0}, [2]int{3, 0}, [2]int{4, 0}), CalledSet([]int{1, 2, 4, 5}), false},
		{"an empty pattern", testBoard(), grid(), map[int]bool{}, true},
	} {
		if got := MatchesPattern(c.board, c.pattern, c.called); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPatternColumns(t *testing.T) {
	for _, c := range []struct {
		name     string
		patterns [][][]bool
		want     [5]bool
	}{
		{"postage stamps skip N", [][][]bool{postageStamps()}, [5]bool{true, true, false, true, true}},
		{"the free centre alone falls back to all", [][][]bool{grid([2]int{2, 2})}, [5]bool{true, true, true, true, true}},
		{"no patterns", nil, [5]bool{true, true, true, true, true}},
		{"the union of the patterns", [][][]bool{grid([2]int{0, 0}), grid([2]int{0, 4})}, [5]bool{true, false, false, false, true}},
	} {
		if got := PatternColumns(c.patterns); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHalftimeThreshold(t *testing.T) {
	// A full five-column game keeps the classic 35 of 75.
	if got := HalftimeThreshold([][][]bool{topRow()}); got != 35 {
		t.Errorf("five columns: %d, want 35", got)
	}
	// Postage stamps use four columns (60 callable): round(35/75*60) = 28.
	if got := HalftimeThreshold([][][]bool{postageStamps()}); got != 28 {
		t.Errorf("four columns: %d, want 28", got)
	}
	// One column (15 callable): round(7) = 7.
	if got := HalftimeThreshold([][][]bool{grid([2]int{0, 0})}); got != 7 {
		t.Errorf("one column: %d, want 7", got)
	}
	if got := HalftimeThreshold(nil); got != 35 {
		t.Errorf("no patterns: %d, want 35", got)
	}
}

func TestClampAutoInterval(t *testing.T) {
	for _, c := range []struct{ in, want int }{
		{0, DefaultAutoInterval},
		{-5, DefaultAutoInterval},
		{3, MinAutoInterval},
		{45, 45},
		{9999, MaxAutoInterval},
		{MinAutoInterval, MinAutoInterval},
		{MaxAutoInterval, MaxAutoInterval},
	} {
		if got := ClampAutoInterval(c.in); got != c.want {
			t.Errorf("ClampAutoInterval(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCalledSet(t *testing.T) {
	s := CalledSet([]int{5, 22, 48})
	if len(s) != 3 || !s[5] || !s[22] || !s[48] || s[99] {
		t.Errorf("CalledSet: %v", s)
	}
	if len(CalledSet(nil)) != 0 {
		t.Error("CalledSet(nil) isn't empty")
	}
}
