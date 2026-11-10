package schedule_test

import (
	"testing"
	"time"

	"app-suite/internal/schedule"
)

// mustParse parses a UTC RFC 3339 instant or fails the test.
func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v.UTC()
}

func TestNextDaily(t *testing.T) {
	r := schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", Minutes: 19*60 + 30}
	if got := schedule.Next(r, mustParse(t, "2026-06-13T10:00:00Z")); got != "2026-06-13T19:30:00Z" {
		t.Errorf("daily (before): got %q", got)
	}
	if got := schedule.Next(r, mustParse(t, "2026-06-13T20:00:00Z")); got != "2026-06-14T19:30:00Z" {
		t.Errorf("daily (after): got %q", got)
	}
}

func TestNextWeekly(t *testing.T) {
	// Every Saturday at 19:00 UTC; 2026-06-13 is a Saturday.
	r := schedule.Rule{Kind: schedule.Weekly, Timezone: "UTC", Weekdays: "6", Minutes: 19 * 60}
	if got := schedule.Next(r, mustParse(t, "2026-06-13T09:00:00Z")); got != "2026-06-13T19:00:00Z" {
		t.Errorf("weekly same-day: got %q", got)
	}
	if got := schedule.Next(r, mustParse(t, "2026-06-13T19:30:00Z")); got != "2026-06-20T19:00:00Z" {
		t.Errorf("weekly next-week: got %q", got)
	}
}

func TestNextWeeklyMultiDay(t *testing.T) {
	// Tuesday and Thursday at 18:00 UTC.
	r := schedule.Rule{Kind: schedule.Weekly, Timezone: "UTC", Weekdays: "2,4", Minutes: 18 * 60}
	if got := schedule.Next(r, mustParse(t, "2026-06-13T12:00:00Z")); got != "2026-06-16T18:00:00Z" {
		t.Errorf("multi-day -> Tue: got %q", got)
	}
	if got := schedule.Next(r, mustParse(t, "2026-06-16T19:00:00Z")); got != "2026-06-18T18:00:00Z" {
		t.Errorf("multi-day -> Thu: got %q", got)
	}
}

func TestNextMonthlyNth(t *testing.T) {
	// The third Saturday of every month at 20:21 UTC.
	r := schedule.Rule{Kind: schedule.Monthly, Timezone: "UTC", Weekdays: "6", WeekOfMonth: 3, Minutes: 20*60 + 21}
	if got := schedule.Next(r, mustParse(t, "2026-06-01T00:00:00Z")); got != "2026-06-20T20:21:00Z" {
		t.Errorf("monthly 3rd Sat (June): got %q", got)
	}
	if got := schedule.Next(r, mustParse(t, "2026-06-20T21:00:00Z")); got != "2026-07-18T20:21:00Z" {
		t.Errorf("monthly 3rd Sat (July): got %q", got)
	}
}

func TestNextMonthlyLast(t *testing.T) {
	r := schedule.Rule{Kind: schedule.Monthly, Timezone: "UTC", Weekdays: "5", WeekOfMonth: -1, Minutes: 17 * 60}
	if got := schedule.Next(r, mustParse(t, "2026-06-01T00:00:00Z")); got != "2026-06-26T17:00:00Z" {
		t.Errorf("monthly last Fri: got %q", got)
	}
}

func TestNextMonthlyFifthSkipsShortMonths(t *testing.T) {
	// June 2026 has four Saturdays; the next month with a fifth is August, the 29th.
	r := schedule.Rule{Kind: schedule.Monthly, Timezone: "UTC", Weekdays: "6", WeekOfMonth: 5, Minutes: 12 * 60}
	if got := schedule.Next(r, mustParse(t, "2026-06-01T00:00:00Z")); got != "2026-08-29T12:00:00Z" {
		t.Errorf("monthly 5th Sat: got %q", got)
	}
}

// TestNextTimezoneDST: the same wall-clock time is a different UTC instant in summer
// and winter, so the recurrence is anchored to the zone.
func TestNextTimezoneDST(t *testing.T) {
	r := schedule.Rule{Kind: schedule.Weekly, Timezone: "America/New_York", Weekdays: "0", Minutes: 10 * 60}
	if got := schedule.Next(r, mustParse(t, "2026-07-01T00:00:00Z")); got != "2026-07-05T14:00:00Z" {
		t.Errorf("DST summer: got %q, want 2026-07-05T14:00:00Z", got)
	}
	if got := schedule.Next(r, mustParse(t, "2026-12-01T00:00:00Z")); got != "2026-12-06T15:00:00Z" {
		t.Errorf("DST winter: got %q, want 2026-12-06T15:00:00Z", got)
	}
}

func TestNextOnceAndUnscheduled(t *testing.T) {
	for _, kind := range []string{schedule.Once, ""} {
		if got := schedule.Next(schedule.Rule{Kind: kind}, time.Now()); got != "" {
			t.Errorf("kind %q: got %q; want no recurrence", kind, got)
		}
	}
}

func TestNextWithoutAWeekday(t *testing.T) {
	for _, kind := range []string{schedule.Weekly, schedule.Monthly} {
		if got := schedule.Next(schedule.Rule{Kind: kind, Timezone: "UTC", WeekOfMonth: 1}, time.Now()); got != "" {
			t.Errorf("%s with no weekday: got %q; want none", kind, got)
		}
	}
}

func TestAdvance(t *testing.T) {
	weekly := schedule.Rule{Kind: schedule.Weekly, Timezone: "UTC", Weekdays: "6", Minutes: 19 * 60}

	// Fired on time: rolls to next week's slot.
	next, live := schedule.Advance(weekly, "2026-06-13T19:00:00Z", mustParse(t, "2026-06-13T19:00:05Z"))
	if !live || next != "2026-06-20T19:00:00Z" {
		t.Errorf("on time: next=%q live=%v", next, live)
	}

	// Overdue by more than a week: jumps to the next future slot instead of replaying
	// a past one, which would stay due and fire every tick.
	now := mustParse(t, "2026-06-15T12:00:00Z")
	next, live = schedule.Advance(weekly, "2026-05-30T19:00:00Z", now)
	if !live || next != "2026-06-20T19:00:00Z" {
		t.Errorf("overdue: next=%q live=%v", next, live)
	}
	if !mustParse(t, next).After(now) {
		t.Errorf("overdue: next %q is not after now %q", next, now)
	}

	// A one-time schedule has no next: it ends.
	if next, live := schedule.Advance(schedule.Rule{Kind: schedule.Once}, "2026-06-13T19:00:00Z", mustParse(t, "2026-06-14T00:00:00Z")); live || next != "" {
		t.Errorf("once: next=%q live=%v", next, live)
	}
}

func TestResolveLocal(t *testing.T) {
	chicago := schedule.Location("America/Chicago")
	got, err := schedule.ResolveLocal("2026-06-13T19:00", chicago)
	if err != nil || got != "2026-06-14T00:00:00Z" {
		t.Errorf("ResolveLocal = %q, %v; want 2026-06-14T00:00:00Z", got, err)
	}
	if got, err := schedule.ResolveLocal("2026-06-13T19:00:30", time.UTC); err != nil || got != "2026-06-13T19:00:30Z" {
		t.Errorf("with seconds = %q, %v", got, err)
	}
	if got, err := schedule.ResolveLocal("  ", time.UTC); err != nil || got != "" {
		t.Errorf("blank = %q, %v; want empty", got, err)
	}
	if _, err := schedule.ResolveLocal("next tuesday", time.UTC); err == nil {
		t.Error("an unreadable value should be an error")
	}
}

func TestLocationFallsBackToUTC(t *testing.T) {
	for _, tz := range []string{"", "  ", "Not/AZone"} {
		if schedule.Location(tz) != time.UTC {
			t.Errorf("Location(%q) should be UTC", tz)
		}
	}
}

func TestKindsAndWeeks(t *testing.T) {
	for _, k := range []string{"", "once", "daily", "weekly", "monthly"} {
		if !schedule.ValidKind(k) {
			t.Errorf("ValidKind(%q) = false", k)
		}
	}
	if schedule.ValidKind("hourly") {
		t.Error("hourly is not a schedule kind")
	}
	for _, n := range []int{-1, 1, 2, 3, 4, 5} {
		if !schedule.ValidWeekOfMonth(n) {
			t.Errorf("ValidWeekOfMonth(%d) = false", n)
		}
	}
	for _, n := range []int{-2, 0, 6} {
		if schedule.ValidWeekOfMonth(n) {
			t.Errorf("ValidWeekOfMonth(%d) = true", n)
		}
	}
	if !schedule.HasWeekday("1, 3") || schedule.HasWeekday("") || schedule.HasWeekday("9,x") {
		t.Error("HasWeekday misreads its list")
	}
}
