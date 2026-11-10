// Package festival holds the rules of the festival side that need no database,
// clock or network of their own: how a garapon's prizes are kept and drawn, and
// when a stamp rally is open, which stamps can be collected and when a card is
// complete. Times are passed in.
package festival

import (
	"errors"
	"strings"
)

// DefaultBallColor is the ball color of a prize saved without one.
const DefaultBallColor = "#cccccc"

// Prize is one prize tier of a garapon.
type Prize struct {
	Name      string
	BallColor string
	// Rate is the prize's appearance weight, relative to the others.
	Rate    float64
	IsGrand bool
}

// Why a list of prizes can't be kept.
var (
	// ErrNoPrizes means no prize with a name was left.
	ErrNoPrizes = errors.New("at least one prize is required")
	// ErrManyGrand means more than one prize is flagged grand.
	ErrManyGrand = errors.New("only one prize can be the grand prize")
)

// SanitizePrizes cleans prizes as they are saved: a blank name drops the prize, a
// blank ball color becomes DefaultBallColor, a negative rate becomes 0, and exactly
// one prize is grand (the first when none is flagged).
func SanitizePrizes(in []Prize) ([]Prize, error) {
	out := make([]Prize, 0, len(in))
	grand := 0
	for _, p := range in {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		color := strings.TrimSpace(p.BallColor)
		if color == "" {
			color = DefaultBallColor
		}
		if p.IsGrand {
			grand++
		}
		out = append(out, Prize{Name: name, BallColor: color, Rate: max(p.Rate, 0), IsGrand: p.IsGrand})
	}
	switch {
	case len(out) == 0:
		return nil, ErrNoPrizes
	case grand == 0:
		out[0].IsGrand = true
	case grand > 1:
		return nil, ErrManyGrand
	}
	return out, nil
}

// DefaultDraws is a draw allowance of at least 1: a zero-draw link couldn't draw at
// all, and zero is what an omitted field reads as, so it means unset.
func DefaultDraws(n int64) int64 { return max(n, 1) }

// pickGranularity is how finely a weighted pick divides the total rate.
const pickGranularity = 1_000_000

// PickPrize chooses the index of a prize weighted by its rate. Rates are relative
// weights that need not total 100. A prize whose rate is not positive is never
// picked unless every rate is not positive, when the choice is uniform. intn is the
// source of randomness: a number in [0, n). It panics on no rates.
func PickPrize(rates []float64, intn func(int) int) int {
	total := 0.0
	for _, r := range rates {
		if r > 0 {
			total += r
		}
	}
	if total <= 0 {
		return intn(len(rates))
	}
	at := float64(intn(pickGranularity)) / pickGranularity * total
	cum := 0.0
	for i, r := range rates {
		if r <= 0 {
			continue
		}
		cum += r
		if at < cum {
			return i
		}
	}
	// Floating-point rounding can leave at past the sum: the last prize in play.
	for i := len(rates) - 1; i >= 0; i-- {
		if rates[i] > 0 {
			return i
		}
	}
	return len(rates) - 1
}
