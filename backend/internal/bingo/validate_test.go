package bingo

import "testing"

func TestValidateBoard(t *testing.T) {
	// A freshly generated board is always valid.
	if err := ValidateBoard(GenerateBoard()); err != nil {
		t.Errorf("GenerateBoard produced an invalid board: %v", err)
	}

	valid := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	if err := ValidateBoard(valid); err != nil {
		t.Errorf("valid board rejected: %v", err)
	}

	// Each problem names the cell it is in.
	tests := []struct {
		name     string
		mutate   func(b [][]int)
		row, col int
	}{
		{"free centre not 0", func(b [][]int) { b[2][2] = 33 }, 2, 2},
		{"out of column range", func(b [][]int) { b[0][0] = 99 }, 0, 0}, // 99 not in B (1-15)
		{"wrong column band", func(b [][]int) { b[3][0] = 20 }, 3, 0},   // 20 belongs in I, not B
		{"duplicate in column", func(b [][]int) { b[1][4] = b[0][4] }, 1, 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := [][]int{
				{1, 16, 31, 46, 61},
				{2, 17, 32, 47, 62},
				{3, 18, 0, 48, 63},
				{4, 19, 34, 49, 64},
				{5, 20, 35, 50, 65},
			}
			tc.mutate(b)
			err := ValidateBoard(b)
			if err == nil {
				t.Fatalf("%s: expected an error, got nil", tc.name)
			}
			if err.Row != tc.row || err.Col != tc.col || err.Error() == "" {
				t.Errorf("%s: %+v, want row %d col %d with a reason", tc.name, *err, tc.row, tc.col)
			}
		})
	}

	// Wrong shapes: the whole board, or one row.
	if err := ValidateBoard([][]int{{1, 2, 3}}); err == nil || err.Row != -1 {
		t.Errorf("a board of one row: %+v, want the whole board's problem", err)
	}
	short := [][]int{{1, 16, 31, 46, 61}, {2, 17, 32, 47}, {3, 18, 0, 48, 63}, {4, 19, 34, 49, 64}, {5, 20, 35, 50, 65}}
	if err := ValidateBoard(short); err == nil || err.Row != 1 || err.Col != -1 {
		t.Errorf("a short row: %+v, want row 1's problem", err)
	}
}
