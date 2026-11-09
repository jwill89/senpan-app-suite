package server

import (
	"fmt"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// normalizeTimes puts each time a client sent in the stored form, in place
// (store.NormalizeTime: RFC 3339 in UTC to the second, "" for none), and returns
// the message for a 400 when one isn't a time, or "". Every DATETIME column's
// CHECK refuses any other form, so a time is checked here rather than failing the
// write.
func normalizeTimes(times ...*string) string {
	for _, t := range times {
		n, err := store.NormalizeTime(*t)
		if err != nil {
			return fmt.Sprintf("%q is not a date and time (RFC 3339, such as 2026-06-13T20:00:00Z).", *t)
		}
		*t = n
	}
	return ""
}

// rallyTimesError normalizes a rally's window and each stamp's (normalizeTimes).
func rallyTimesError(r *model.StampRally) string {
	times := []*string{&r.AvailableFrom, &r.AvailableTo}
	for i := range r.Stamps {
		times = append(times, &r.Stamps[i].ActiveFrom, &r.Stamps[i].ActiveTo)
	}
	return normalizeTimes(times...)
}
