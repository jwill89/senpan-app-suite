package festival

import (
	"errors"
	"testing"
)

func TestSanitizePrizes(t *testing.T) {
	t.Run("drops blanks, defaults the color, floors a negative rate", func(t *testing.T) {
		out, err := SanitizePrizes([]Prize{
			{Name: "  ", Rate: 5},                    // blank: dropped
			{Name: "Grand", Rate: -3, IsGrand: true}, // negative rate: 0
			{Name: " Other ", BallColor: "", Rate: 2},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 2 {
			t.Fatalf("%d prizes kept, want 2", len(out))
		}
		if out[0].Rate != 0 {
			t.Errorf("a negative rate was kept as %v", out[0].Rate)
		}
		if out[1].Name != "Other" || out[1].BallColor != DefaultBallColor {
			t.Errorf("the second prize: %+v, want it trimmed with the default color", out[1])
		}
	})

	t.Run("the first prize is grand when none is marked", func(t *testing.T) {
		out, err := SanitizePrizes([]Prize{{Name: "A", Rate: 1}, {Name: "B", Rate: 1}})
		if err != nil {
			t.Fatal(err)
		}
		if !out[0].IsGrand || out[1].IsGrand {
			t.Errorf("grand flags: %v %v, want true false", out[0].IsGrand, out[1].IsGrand)
		}
	})

	t.Run("refuses none and more than one grand", func(t *testing.T) {
		if _, err := SanitizePrizes(nil); !errors.Is(err, ErrNoPrizes) {
			t.Errorf("no prizes: %v, want ErrNoPrizes", err)
		}
		if _, err := SanitizePrizes([]Prize{{Name: " "}}); !errors.Is(err, ErrNoPrizes) {
			t.Errorf("only a blank prize: %v, want ErrNoPrizes", err)
		}
		if _, err := SanitizePrizes([]Prize{{Name: "A", IsGrand: true}, {Name: "B", IsGrand: true}}); !errors.Is(err, ErrManyGrand) {
			t.Errorf("two grand prizes: %v, want ErrManyGrand", err)
		}
	})
}

func TestDefaultDraws(t *testing.T) {
	for in, want := range map[int64]int64{-4: 1, 0: 1, 1: 1, 7: 7} {
		if got := DefaultDraws(in); got != want {
			t.Errorf("DefaultDraws(%d) = %d, want %d", in, got, want)
		}
	}
}

// fixed returns a source that answers the given numbers in turn.
func fixed(values ...int) func(int) int {
	i := 0
	return func(n int) int {
		v := values[i%len(values)] % n
		i++
		return v
	}
}

func TestPickPrize(t *testing.T) {
	t.Run("walks the weights", func(t *testing.T) {
		rates := []float64{1, 3} // 25% and 75%
		for at, want := range map[int]int{0: 0, 249_999: 0, 250_000: 1, 999_999: 1} {
			if got := PickPrize(rates, fixed(at)); got != want {
				t.Errorf("at %d picked %d, want %d", at, got, want)
			}
		}
	})

	t.Run("a prize with no rate is never picked", func(t *testing.T) {
		rates := []float64{0, 5, -2, 5}
		for at := 0; at < 1_000_000; at += 997 {
			got := PickPrize(rates, fixed(at))
			if got == 0 || got == 2 {
				t.Fatalf("at %d picked %d, a prize with no rate", at, got)
			}
		}
	})

	t.Run("with no rate anywhere the choice is even", func(t *testing.T) {
		rates := []float64{0, 0, -1}
		seen := map[int]bool{}
		for i := range rates {
			seen[PickPrize(rates, fixed(i))] = true
		}
		if len(seen) != 3 {
			t.Errorf("picked %v, want every prize reachable", seen)
		}
	})

	t.Run("rounding at the top still lands on a prize in play", func(t *testing.T) {
		if got := PickPrize([]float64{0.1, 0.2, 0}, fixed(999_999)); got != 1 {
			t.Errorf("picked %d, want the last prize with a rate", got)
		}
	})
}
