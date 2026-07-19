// Package bingo implements the core game logic: card/board generation,
// pattern matching, winner computation, and game lifecycle management.
package bingo

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"
)

// Column ranges for standard 75-ball bingo: B(1–15), I(16–30), N(31–45), G(46–60), O(61–75).
var columnRanges = [5][2]int{
	{1, 15}, {16, 30}, {31, 45}, {46, 60}, {61, 75},
}

// columnLetters maps a column index (0–4) to its bingo letter.
var columnLetters = [5]string{"B", "I", "N", "G", "O"}

// idChars excludes ambiguous characters 0/O/1/I/l.
const idChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateBoard creates a standard 75-ball bingo board as a 5×5 array.
// board[row][col] — col 0=B … col 4=O. Center cell [2][2] = 0 (FREE).
func GenerateBoard() [][]int {
	board := make([][]int, 5)
	for r := range board {
		board[r] = make([]int, 5)
	}

	for col := 0; col < 5; col++ {
		lo, hi := columnRanges[col][0], columnRanges[col][1]
		pool := make([]int, hi-lo+1)
		for i := range pool {
			pool[i] = lo + i
		}
		rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		for row := 0; row < 5; row++ {
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
			id[i] = idChars[rand.IntN(len(idChars))]
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

// ValidateBoard checks that board is a structurally valid 75-ball bingo card: a 5×5
// grid, the FREE (0) space at centre [2][2], and every other cell within its
// column's range (B 1–15 … O 61–75) with no repeats inside a column. It returns a
// human-readable error describing the first problem found, or nil when the card is
// valid. Used to validate hand-built custom-card requests before they are stored.
func ValidateBoard(board [][]int) error {
	if len(board) != 5 {
		return fmt.Errorf("card must have exactly 5 rows")
	}
	for r := range board {
		if len(board[r]) != 5 {
			return fmt.Errorf("row %d must have exactly 5 columns", r+1)
		}
	}
	if board[2][2] != 0 {
		return fmt.Errorf("the centre cell must be left as the FREE space")
	}
	for col := 0; col < 5; col++ {
		lo, hi := columnRanges[col][0], columnRanges[col][1]
		seen := make(map[int]bool, 5)
		for row := 0; row < 5; row++ {
			if col == 2 && row == 2 {
				continue // FREE centre
			}
			n := board[row][col]
			if n < lo || n > hi {
				return fmt.Errorf("column %s must only contain numbers %d–%d (found %d)", columnLetters[col], lo, hi, n)
			}
			if seen[n] {
				return fmt.Errorf("column %s has the number %d more than once", columnLetters[col], n)
			}
			seen[n] = true
		}
	}
	return nil
}

// customIDRe matches a normalised (upper-cased) custom card ID: exactly 6
// alphanumeric characters. Unlike GenerateID's alphabet this permits the ambiguous
// characters (0/O/1/I) because the user typed them deliberately; uniqueness is still
// enforced separately via CardExists.
var customIDRe = regexp.MustCompile(`^[A-Z0-9]{6}$`)

// ValidateCustomID upper-cases and validates a user-chosen card ID, returning the
// normalised ID or an error when it isn't exactly 6 alphanumeric characters.
func ValidateCustomID(raw string) (string, error) {
	id := strings.ToUpper(strings.TrimSpace(raw))
	if !customIDRe.MatchString(id) {
		return "", fmt.Errorf("card ID must be exactly 6 letters or numbers")
	}
	return id, nil
}

// LetterForNumber returns the bingo column letter for a number (1–75).
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
