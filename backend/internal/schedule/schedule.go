// Package schedule works out when a recurring post is next due. Every time is a
// wall-clock value in an IANA timezone, so the calendar math runs in that zone and
// survives daylight saving, and the result is an absolute UTC instant. It needs no
// database and takes the clock as an argument.
package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The kinds of schedule. An empty kind is an unscheduled post, made by hand.
const (
	Once    = "once"
	Daily   = "daily"
	Weekly  = "weekly"
	Monthly = "monthly"
)

// MaxMinutes is the last minute of a day, the highest a time of day can be.
const MaxMinutes = 1439

// ValidKind reports whether kind is a schedule kind or empty.
func ValidKind(kind string) bool {
	switch kind {
	case "", Once, Daily, Weekly, Monthly:
		return true
	}
	return false
}

// ValidWeekOfMonth reports whether n names a week of a month: 1 to 5, or -1 for the
// last.
func ValidWeekOfMonth(n int) bool {
	return n == -1 || (n >= 1 && n <= 5)
}

// Rule is how a post recurs.
type Rule struct {
	// Kind is Once, Daily, Weekly or Monthly; empty is unscheduled.
	Kind string
	// Timezone is the IANA zone the wall-clock times are in; empty or unknown is UTC.
	Timezone string
	// Minutes is the time of day, in minutes after midnight in Timezone.
	Minutes int
	// Weekdays is a comma-separated list of weekday numbers, 0 for Sunday to 6 for
	// Saturday. A weekly rule uses all of them, a monthly rule the first.
	Weekdays string
	// WeekOfMonth is which weekday of the month a monthly rule means: 1 to 5, or -1
	// for the last.
	WeekOfMonth int
}

// Location is the zone named by tz, or UTC when it is empty or unknown.
func Location(tz string) *time.Location {
	if tz = strings.TrimSpace(tz); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			return l
		}
	}
	return time.UTC
}

// Next is the first occurrence of r strictly after the given time, as an RFC 3339
// string in UTC, or "" when there is none: a rule with no kind or a once-only one,
// or one with no weekday to land on.
func Next(r Rule, after time.Time) string {
	loc := Location(r.Timezone)
	after = after.In(loc)
	h, m := r.Minutes/60, r.Minutes%60

	switch r.Kind {
	case Daily:
		cand := time.Date(after.Year(), after.Month(), after.Day(), h, m, 0, 0, loc)
		for !cand.After(after) {
			cand = cand.AddDate(0, 0, 1)
		}
		return cand.UTC().Format(time.RFC3339)

	case Weekly:
		days := weekdaySet(r.Weekdays)
		if len(days) == 0 {
			return ""
		}
		// Eight days, so a weekday that is today but already past comes round again.
		for i := range 8 {
			d := after.AddDate(0, 0, i)
			if days[int(d.Weekday())] {
				cand := time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, loc)
				if cand.After(after) {
					return cand.UTC().Format(time.RFC3339)
				}
			}
		}
		return ""

	case Monthly:
		wd, ok := firstWeekday(r.Weekdays)
		if !ok {
			return ""
		}
		// A month may lack the occurrence (a fifth Friday), so look a year ahead.
		for i := range 13 {
			base := time.Date(after.Year(), after.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, i, 0)
			cand := NthWeekday(base.Year(), base.Month(), wd, r.WeekOfMonth, h, m, loc)
			if !cand.IsZero() && cand.After(after) {
				return cand.UTC().Format(time.RFC3339)
			}
		}
		return ""
	}
	return ""
}

// HasWeekday reports whether weekdays names at least one weekday.
func HasWeekday(weekdays string) bool {
	return len(weekdaySet(weekdays)) > 0
}

// weekdaySet reads a comma-separated list of weekday numbers into a set.
func weekdaySet(csv string) map[int]bool {
	out := map[int]bool{}
	for part := range strings.SplitSeq(csv, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && n >= 0 && n <= 6 {
			out[n] = true
		}
	}
	return out
}

// firstWeekday is the first weekday in a comma-separated list.
func firstWeekday(csv string) (time.Weekday, bool) {
	for part := range strings.SplitSeq(csv, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && n >= 0 && n <= 6 {
			return time.Weekday(n), true
		}
	}
	return 0, false
}

// NthWeekday is the nth (1 to 5, or -1 for the last) wd of the month at h:m in loc,
// or the zero time when the month has no such day, as a fifth Friday in a month with
// four.
func NthWeekday(year int, month time.Month, wd time.Weekday, n, h, m int, loc *time.Location) time.Time {
	first := time.Date(year, month, 1, h, m, 0, 0, loc)
	firstDay := 1 + (int(wd)-int(first.Weekday())+7)%7
	daysInMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()

	if n == -1 {
		last := firstDay
		for last+7 <= daysInMonth {
			last += 7
		}
		return time.Date(year, month, last, h, m, 0, 0, loc)
	}
	day := firstDay + (n-1)*7
	if day < 1 || day > daysInMonth {
		return time.Time{}
	}
	return time.Date(year, month, day, h, m, 0, 0, loc)
}

// Advance is the cursor after the occurrence at due has fired: the next occurrence
// of r and whether the schedule stays live. A recurring rule rolls on to the first
// occurrence strictly in the future, anchored on the later of now and due: from
// the old cursor alone, a post overdue by more than a period (the server was down
// across a slot) would land on another past slot, stay due, and fire again every
// tick. Missed occurrences are skipped, not replayed. A once-only or unscheduled
// rule has no further occurrence, so it ends.
func Advance(r Rule, due string, now time.Time) (next string, live bool) {
	if r.Kind == "" || r.Kind == Once {
		return "", false
	}
	after := now
	if t, err := time.Parse(time.RFC3339, due); err == nil && t.After(after) {
		after = t
	}
	next = Next(r, after)
	return next, next != ""
}

// localLayouts are the wall-clock forms a datetime-local input sends, the seconds
// optional.
var localLayouts = []string{"2006-01-02T15:04", "2006-01-02T15:04:05"}

// ParseLocal reads a wall-clock value in loc as the instant it names.
func ParseLocal(value string, loc *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty datetime")
	}
	for _, layout := range localLayouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid datetime %q", value)
}

// ResolveLocal is a wall-clock value in loc as an RFC 3339 string in UTC. An empty
// value resolves to "".
func ResolveLocal(value string, loc *time.Location) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	t, err := ParseLocal(value, loc)
	if err != nil {
		return "", err
	}
	return t.UTC().Format(time.RFC3339), nil
}
