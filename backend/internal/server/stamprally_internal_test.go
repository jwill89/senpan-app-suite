package server

import (
	"testing"
	"time"

	"app-suite/internal/model"
)

// Fixed reference instant for the time-window matrix below.
var (
	srNow    = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	srPast   = "2026-06-10T00:00:00Z"
	srFuture = "2026-06-20T00:00:00Z"
)

// TestRallyCardComplete covers the completion rule: complete when every stamp is
// collected or permanently expired AND at least one was collected; a paused-in-window
// stamp keeps the card incomplete.
func TestRallyCardComplete(t *testing.T) {
	open := &model.StampRally{}
	two := []model.StampRallyStamp{{ID: 1}, {ID: 2}}

	cases := []struct {
		name      string
		rally     *model.StampRally
		stamps    []model.StampRallyStamp
		collected map[int64]string
		want      bool
	}{
		{"none collected", open, two, map[int64]string{}, false},
		{"one of two, other collectable", open, two, map[int64]string{1: ""}, false},
		{"both collected", open, two, map[int64]string{1: "", 2: ""}, true},
		{
			"one collected, other expired",
			open,
			[]model.StampRallyStamp{{ID: 1}, {ID: 2, ActiveTo: srPast}},
			map[int64]string{1: ""},
			true,
		},
		{
			"zero collected, all expired -> not complete",
			open,
			[]model.StampRallyStamp{{ID: 1, ActiveTo: srPast}, {ID: 2, ActiveTo: srPast}},
			map[int64]string{},
			false,
		},
		{
			"paused but in-window blocks completion",
			open,
			[]model.StampRallyStamp{{ID: 1}, {ID: 2, Paused: true, ActiveTo: srFuture}},
			map[int64]string{1: ""},
			false,
		},
		{
			"event ended -> complete with one collected",
			&model.StampRally{AvailableTo: srPast},
			two,
			map[int64]string{1: ""},
			true,
		},
	}
	for _, c := range cases {
		if got := rallyCardComplete(c.rally, c.stamps, c.collected, srNow); got != c.want {
			t.Errorf("%s: complete = %v; want %v", c.name, got, c.want)
		}
	}
}

// TestRallyCardCompleteCounts covers the per-type rule: a "counts" rally completes
// on the required number of food + game stamps, whatever is left uncollected, and
// never on expiry alone (running out of time short of the requirement is a miss).
func TestRallyCardCompleteCounts(t *testing.T) {
	// Two food stalls + two game stalls, requiring one of each.
	stamps := []model.StampRallyStamp{
		{ID: 1, StampType: model.StampTypeFood},
		{ID: 2, StampType: model.StampTypeFood},
		{ID: 3, StampType: model.StampTypeGame},
		{ID: 4, StampType: model.StampTypeGame},
	}
	rally := &model.StampRally{CompletionMode: model.RallyCompletionCounts, RequiredFood: 1, RequiredGame: 1}
	ended := &model.StampRally{CompletionMode: model.RallyCompletionCounts, RequiredFood: 1, RequiredGame: 1,
		AvailableTo: srPast}

	cases := []struct {
		name      string
		rally     *model.StampRally
		collected map[int64]string
		want      bool
	}{
		{"nothing collected", rally, map[int64]string{}, false},
		{"one food only", rally, map[int64]string{1: ""}, false},
		{"both food, no game", rally, map[int64]string{1: "", 2: ""}, false},
		{"one of each - the rest are optional", rally, map[int64]string{1: "", 3: ""}, true},
		{"more than required", rally, map[int64]string{1: "", 2: "", 3: "", 4: ""}, true},
		{"event ended short of the requirement", ended, map[int64]string{1: ""}, false},
		{"event ended having met it", ended, map[int64]string{2: "", 4: ""}, true},
	}
	for _, c := range cases {
		if got := rallyCardComplete(c.rally, stamps, c.collected, srNow); got != c.want {
			t.Errorf("%s: complete = %v; want %v", c.name, got, c.want)
		}
	}

	// A "counts" rally requiring nothing of either type must NOT finish a card at
	// its first stamp - it falls back to the whole-card rule (saving one is
	// rejected, so this only guards a row that predates that check).
	none := &model.StampRally{CompletionMode: model.RallyCompletionCounts}
	if rallyCardComplete(none, stamps, map[int64]string{1: ""}, srNow) {
		t.Error("counts rally requiring 0/0 completed on one stamp; want the whole-card rule")
	}
	all := map[int64]string{1: "", 2: "", 3: "", 4: ""}
	if !rallyCardComplete(none, stamps, all, srNow) {
		t.Error("counts rally requiring 0/0 did not complete on the whole card")
	}
}

// TestCompletionRuleError guards the save-time refusal of a per-type rule that
// requires nothing, which would finish every card at its first stamp.
func TestCompletionRuleError(t *testing.T) {
	cases := []struct {
		name   string
		rally  model.StampRally
		reject bool
	}{
		{"counts, nothing required", model.StampRally{CompletionMode: model.RallyCompletionCounts}, true},
		{"counts, food only", model.StampRally{CompletionMode: model.RallyCompletionCounts, RequiredFood: 2}, false},
		{"counts, game only", model.StampRally{CompletionMode: model.RallyCompletionCounts, RequiredGame: 1}, false},
		{"all mode ignores the counts", model.StampRally{CompletionMode: model.RallyCompletionAll}, false},
	}
	for _, c := range cases {
		r := c.rally
		if got := completionRuleError(&r) != ""; got != c.reject {
			t.Errorf("%s: rejected = %v; want %v", c.name, got, c.reject)
		}
	}
}

// TestClampRequired guards the save-time clamp: a rally can never be configured to
// need more stamps of a type than it carries (which no card could ever satisfy).
func TestClampRequired(t *testing.T) {
	cases := []struct{ want, available, expect int }{
		{-1, 5, 0},
		{0, 5, 0},
		{3, 5, 3},
		{9, 5, 5},
		{2, 0, 0},
	}
	for _, c := range cases {
		if got := clampRequired(c.want, c.available); got != c.expect {
			t.Errorf("clampRequired(%d, %d) = %d; want %d", c.want, c.available, got, c.expect)
		}
	}
}

// TestStampAvailable covers the availability rule used to gate collection.
func TestStampAvailable(t *testing.T) {
	open := &model.StampRally{}
	cases := []struct {
		name  string
		rally *model.StampRally
		stamp model.StampRallyStamp
		want  bool
	}{
		{"open, no window, not paused", open, model.StampRallyStamp{}, true},
		{"paused", open, model.StampRallyStamp{Paused: true}, false},
		{"rally manually closed", &model.StampRally{Status: "closed"}, model.StampRallyStamp{}, false},
		{"event window past", &model.StampRally{AvailableTo: srPast}, model.StampRallyStamp{}, false},
		{"event not yet open", &model.StampRally{AvailableFrom: srFuture}, model.StampRallyStamp{}, false},
		{"stamp not yet active", open, model.StampRallyStamp{ActiveFrom: srFuture}, false},
		{"stamp window ended", open, model.StampRallyStamp{ActiveTo: srPast}, false},
		{"stamp active now", open, model.StampRallyStamp{ActiveFrom: srPast, ActiveTo: srFuture}, true},
	}
	for _, c := range cases {
		st := c.stamp
		if got := stampAvailable(c.rally, &st, srNow); got != c.want {
			t.Errorf("%s: available = %v; want %v", c.name, got, c.want)
		}
	}
}
