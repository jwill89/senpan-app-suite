// Package random draws numbers from crypto/rand: card IDs, boards, and every draw
// people rely on being fair (bingo, raffles, the garapon), so none can be
// predicted from the ones before it.
//
// It uses math/rand/v2's algorithms over a crypto/rand source, so a bounded number
// carries no modulo bias and a shuffle is Fisher-Yates.
package random

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand/v2"
)

// source reads every number from crypto/rand. It holds no state, so it is safe to
// use from any goroutine. crypto/rand.Read never fails: the runtime ends the
// program if the system's source does.
type source struct{}

func (source) Uint64() uint64 {
	var b [8]byte
	_, _ = crand.Read(b[:])
	return binary.LittleEndian.Uint64(b[:])
}

// generator returns a math/rand/v2 generator over crypto/rand.
func generator() *rand.Rand { return rand.New(source{}) } //nolint:gosec // the source is crypto/rand; math/rand/v2 supplies only the unbiased bounding and the shuffle

// IntN returns a number in [0, n). It panics if n <= 0.
func IntN(n int) int { return generator().IntN(n) }

// Shuffle puts n elements in a random order, calling swap to exchange two.
func Shuffle(n int, swap func(i, j int)) { generator().Shuffle(n, swap) }
