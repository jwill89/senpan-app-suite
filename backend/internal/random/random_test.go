package random

import "testing"

func TestIntNStaysInRangeAndReachesEveryValue(t *testing.T) {
	seen := map[int]int{}
	for range 6000 {
		n := IntN(6)
		if n < 0 || n >= 6 {
			t.Fatalf("IntN(6) = %d, outside [0, 6)", n)
		}
		seen[n]++
	}
	for v := range 6 {
		// 1000 expected each; a fair source falls this far short about never.
		if seen[v] < 800 {
			t.Errorf("IntN(6) gave %d %d times in 6000, expected about 1000", v, seen[v])
		}
	}
}

func TestShuffleKeepsEveryElement(t *testing.T) {
	xs := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	Shuffle(len(xs), func(i, j int) { xs[i], xs[j] = xs[j], xs[i] })
	sum := 0
	for _, x := range xs {
		sum += x
	}
	if len(xs) != 10 || sum != 55 {
		t.Fatalf("Shuffle lost or changed elements: %v", xs)
	}
}
