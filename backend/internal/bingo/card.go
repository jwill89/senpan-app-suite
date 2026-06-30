// Package bingo implements the core game logic: card/board generation,
// pattern matching, winner computation, and game lifecycle management.
package bingo

import "math/rand/v2"

// Column ranges for standard 75-ball bingo: B(1–15), I(16–30), N(31–45), G(46–60), O(61–75).
var columnRanges = [5][2]int{
	{1, 15}, {16, 30}, {31, 45}, {46, 60}, {61, 75},
}

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
