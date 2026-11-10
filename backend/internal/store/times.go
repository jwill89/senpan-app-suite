package store

import (
	"errors"
	"strings"
	"time"
)

// Timestamps are stored in one form: RFC
// 3339 in UTC to the second, 2026-11-09T21:30:00Z, which every DATETIME column's
// CHECK holds them to, and NULL where there is none. SQL takes "now" as
// strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), never CURRENT_TIMESTAMP or datetime(),
// whose space form the CHECK refuses and which sorts wrong against it. The API
// still sends "" for none until its move to huma (5.0.0), so reads COALESCE a
// nullable one to ''.

// ErrInvalidTime is returned for a time that is neither RFC 3339 nor the naive
// "2006-01-02T15:04" form older raffles stored, which is read as UTC.
var ErrInvalidTime = errors.New("not a date and time (RFC 3339, such as 2026-06-13T20:00:00Z)")

// NormalizeTime returns s in the stored form, or "" for none. Clients send the
// browser's milliseconds (2026-06-13T20:00:00.000Z) or an offset; both come back
// in UTC to the second.
func NormalizeTime(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
	}
	return "", ErrInvalidTime
}
