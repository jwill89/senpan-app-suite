// Package bingo holds the rules of 75-ball bingo, as pure code: making and checking
// cards, matching a card against a win pattern, the columns a game draws from, the
// half-time mark, and the automatic-draw interval. The game that is played (its
// state, its draws and the database) is internal/game.
package bingo

import (
	"fmt"

	"app-suite/internal/random"
)

// Column ranges for standard 75-ball bingo: B(1-15), I(16-30), N(31-45), G(46-60), O(61-75).
var columnRanges = [5][2]int{
	{1, 15}, {16, 30}, {31, 45}, {46, 60}, {61, 75},
}

// columnLetters maps a column index (0-4) to its bingo letter.
var columnLetters = [5]string{"B", "I", "N", "G", "O"}

// idChars excludes ambiguous characters 0/O/1/I/l.
const idChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateBoard creates a standard 75-ball bingo board as a 5x5 array.
// board[row][col] - col 0=B ... col 4=O. Center cell [2][2] = 0 (FREE).
func GenerateBoard() [][]int {
	board := make([][]int, 5)
	for r := range board {
		board[r] = make([]int, 5)
	}

	for col := range 5 {
		lo, hi := columnRanges[col][0], columnRanges[col][1]
		pool := make([]int, hi-lo+1)
		for i := range pool {
			pool[i] = lo + i
		}
		random.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		for row := range 5 {
			board[row][col] = pool[row]
		}
	}

	board[2][2] = 0 // FREE space
	return board
}

// GenerateID creates a 6-char alphanumeric ID (no ambiguous chars).
// It retries until it finds one not yet taken, using the provided exists check.
func GenerateID(exists func(string) (bool, error)) (string, error) {
	for {
		id := make([]byte, 6)
		for i := range id {
			id[i] = idChars[random.IntN(len(idChars))]
		}
		taken, err := exists(string(id))
		if err != nil {
			return "", err
		}
		if !taken {
			return string(id), nil
		}
	}
}

// BoardError is a board's first problem and where it is: Row and Col count from 0,
// and are -1 when the problem is the whole board's (Row) or a whole row's (Col).
type BoardError struct {
	Row, Col int
	Reason   string
}

func (e *BoardError) Error() string { return e.Reason }

// ValidateBoard checks that board is a structurally valid 75-ball bingo card: a 5x5
// grid, the FREE (0) space at centre [2][2], and every other cell within its
// column's range (B 1-15 ... O 61-75) with no repeats inside a column. It returns the
// first problem found, or nil when the card is valid. Used to check hand-built
// personal cards before they are stored.
func ValidateBoard(board [][]int) *BoardError {
	if len(board) != 5 {
		return &BoardError{Row: -1, Col: -1, Reason: "A card has exactly 5 rows."}
	}
	for r := range board {
		if len(board[r]) != 5 {
			return &BoardError{Row: r, Col: -1, Reason: fmt.Sprintf("Row %d must have exactly 5 numbers.", r+1)}
		}
	}
	if board[2][2] != 0 {
		return &BoardError{Row: 2, Col: 2, Reason: "The centre cell is the FREE space: leave it as 0."}
	}
	for col := range 5 {
		lo, hi := columnRanges[col][0], columnRanges[col][1]
		seen := make(map[int]bool, 5)
		for row := range 5 {
			if col == 2 && row == 2 {
				continue // FREE centre
			}
			n := board[row][col]
			if n < lo || n > hi {
				return &BoardError{Row: row, Col: col, Reason: fmt.Sprintf(
					"Column %s holds only the numbers %d-%d, not %d.", columnLetters[col], lo, hi, n)}
			}
			if seen[n] {
				return &BoardError{Row: row, Col: col, Reason: fmt.Sprintf(
					"Column %s has the number %d more than once.", columnLetters[col], n)}
			}
			seen[n] = true
		}
	}
	return nil
}

// LetterForNumber returns the bingo column letter for a number (1-75).
func LetterForNumber(n int) string {
	switch {
	case n <= 15:
		return "B"
	case n <= 30:
		return "I"
	case n <= 45:
		return "N"
	case n <= 60:
		return "G"
	default:
		return "O"
	}
}
