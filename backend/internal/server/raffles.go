package server

import (
	"time"

	"app-suite/internal/model"
	"app-suite/internal/raffles"
)

// parseRaffleTime reads a raffle or rally time into a UTC instant (raffles.ParseTime).
func parseRaffleTime(s string) (time.Time, bool) { return raffles.ParseTime(s) }

// raffleIsPubliclyViewable reports whether anyone may see a raffle now: a closed one,
// or one open inside its availability window.
func raffleIsPubliclyViewable(raffle *model.Raffle) bool {
	return raffles.PubliclyViewable(raffle.Status, raffle.AvailableFrom, raffle.AvailableTo, time.Now().UTC())
}
