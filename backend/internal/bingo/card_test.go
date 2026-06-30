package bingo

import (
	"fmt"
	"testing"
)

func TestGenerateBoard_Dimensions(t *testing.T) {
	board := GenerateBoard()
	if len(board) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(board))
	}
	for r, row := range board {
		if len(row) != 5 {
			t.Fatalf("row %d: expected 5 cols, got %d", r, len(row))
		}
	}
}

func TestGenerateBoard_FreeSpace(t *testing.T) {
	board := GenerateBoard()
	if board[2][2] != 0 {
		t.Errorf("centre cell should be 0 (FREE), got %d", board[2][2])
	}
}

func TestGenerateBoard_ColumnRanges(t *testing.T) {
	board := GenerateBoard()
	for col := 0; col < 5; col++ {
		lo, hi := columnRanges[col][0], columnRanges[col][1]
		for row := 0; row < 5; row++ {
			if row == 2 && col == 2 {
				continue // FREE space
			}
			val := board[row][col]
			if val < lo || val > hi {
				t.Errorf("board[%d][%d] = %d; want [%d, %d]", row, col, val, lo, hi)
			}
		}
	}
}

func TestGenerateBoard_NoDuplicatesInColumn(t *testing.T) {
	// Run multiple iterations to catch randomness issues.
	for iter := 0; iter < 100; iter++ {
		board := GenerateBoard()
		for col := 0; col < 5; col++ {
			seen := make(map[int]bool)
			for row := 0; row < 5; row++ {
				v := board[row][col]
				if v == 0 {
					continue
				}
				if seen[v] {
					t.Fatalf("iter %d: duplicate %d in col %d", iter, v, col)
				}
				seen[v] = true
			}
		}
	}
}

func TestGenerateID(t *testing.T) {
	id, err := GenerateID(func(string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 6 {
		t.Fatalf("expected length 6, got %d", len(id))
	}
	for _, c := range id {
		found := false
		for _, allowed := range idChars {
			if c == allowed {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ID contains invalid char %c", c)
		}
	}
}

func TestGenerateID_RetriesOnCollision(t *testing.T) {
	calls := 0
	id, err := GenerateID(func(string) (bool, error) {
		calls++
		return calls <= 3, nil // first 3 say "taken"
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Errorf("expected 4 calls to exists, got %d", calls)
	}
	if len(id) != 6 {
		t.Errorf("expected length 6, got %d", len(id))
	}
}

func TestGenerateID_PropagatesError(t *testing.T) {
	_, err := GenerateID(func(string) (bool, error) {
		return false, fmt.Errorf("db error")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLetterForNumber(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{1, "B"}, {8, "B"}, {15, "B"},
		{16, "I"}, {23, "I"}, {30, "I"},
		{31, "N"}, {38, "N"}, {45, "N"},
		{46, "G"}, {53, "G"}, {60, "G"},
		{61, "O"}, {68, "O"}, {75, "O"},
	}
	for _, tt := range tests {
		if got := LetterForNumber(tt.n); got != tt.want {
			t.Errorf("LetterForNumber(%d) = %q; want %q", tt.n, got, tt.want)
		}
	}
}
