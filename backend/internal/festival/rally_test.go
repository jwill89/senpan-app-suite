package festival

import (
	"testing"
	"time"

	"app-suite/internal/model"
)

// A fixed instant for the time matrix below.
var (
	now    = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	past   = "2026-06-10T00:00:00Z"
	future = "2026-06-20T00:00:00Z"
)

func TestSanitizePlacement(t *testing.T) {
	got := SanitizePlacement(Placement{X: -5, Y: 140, Width: 0, Height: 300, Rotation: 45})
	want := Placement{X: 0, Y: 100, Width: 10, Height: 100, Rotation: 45}
	if got != want {
		t.Errorf("SanitizePlacement = %+v, want %+v", got, want)
	}
}

func TestWithinWindow(t *testing.T) {
	cases := []struct {
		name     string
		from, to string
		want     bool
	}{
		{"no bounds", "", "", true},
		{"after the start", past, "", true},
		{"before the start", future, "", false},
		{"before the end", "", future, true},
		{"after the end", "", past, false},
		{"inside", past, future, true},
		{"an unreadable bound is none", "soon", "later", true},
	}
	for _, c := range cases {
		if got := WithinWindow(c.from, c.to, now); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRallyOpenAndStampAvailable(t *testing.T) {
	open := Rally{Status: "open"}
	if !RallyOpen(open, now) {
		t.Error("an open rally with no window is closed")
	}
	if RallyOpen(Rally{Status: "closed"}, now) {
		t.Error("a closed rally is open")
	}
	if RallyOpen(Rally{Status: "open", AvailableTo: past}, now) {
		t.Error("a rally past its end is open")
	}

	cases := []struct {
		name  string
		rally Rally
		stamp Stamp
		want  bool
	}{
		{"collectable", open, Stamp{}, true},
		{"paused", open, Stamp{Paused: true}, false},
		{"its own window not started", open, Stamp{ActiveFrom: future}, false},
		{"its own window over", open, Stamp{ActiveTo: past}, false},
		{"its rally closed", Rally{Status: "closed"}, Stamp{}, false},
	}
	for _, c := range cases {
		if got := StampAvailable(c.rally, c.stamp, now); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStampExpired(t *testing.T) {
	open := Rally{Status: "open"}
	if StampExpired(open, Stamp{Paused: true, ActiveTo: future}, now) {
		t.Error("a paused stamp inside its window has expired")
	}
	if !StampExpired(open, Stamp{ActiveTo: past}, now) {
		t.Error("a stamp past its window has not expired")
	}
	if !StampExpired(Rally{AvailableTo: past}, Stamp{}, now) {
		t.Error("a stamp of a rally that is over has not expired")
	}
}

// Complete when every stamp is collected or gone for good and at least one was collected;
// a paused stamp still inside its window keeps the card incomplete.
func TestCardComplete(t *testing.T) {
	open := Rally{}
	two := []Stamp{{ID: 1}, {ID: 2}}

	cases := []struct {
		name      string
		rally     Rally
		stamps    []Stamp
		collected map[int64]string
		want      bool
	}{
		{"none collected", open, two, map[int64]string{}, false},
		{"one of two, the other collectable", open, two, map[int64]string{1: ""}, false},
		{"both collected", open, two, map[int64]string{1: "", 2: ""}, true},
		{"one collected, the other expired", open, []Stamp{{ID: 1}, {ID: 2, ActiveTo: past}}, map[int64]string{1: ""}, true},
		{"all expired, none collected", open, []Stamp{{ID: 1, ActiveTo: past}, {ID: 2, ActiveTo: past}}, map[int64]string{}, false},
		{"paused but inside its window", open, []Stamp{{ID: 1}, {ID: 2, Paused: true, ActiveTo: future}}, map[int64]string{1: ""}, false},
		{"the rally is over", Rally{AvailableTo: past}, two, map[int64]string{1: ""}, true},
	}
	for _, c := range cases {
		if got := CardComplete(c.rally, c.stamps, c.collected, now); got != c.want {
			t.Errorf("%s: complete = %v, want %v", c.name, got, c.want)
		}
	}
}

// A counts rally completes on the required stamps of each type, the rest optional, and
// never on expiry alone.
func TestCardCompleteCounts(t *testing.T) {
	stamps := []Stamp{
		{ID: 1, Type: model.StampTypeFood}, {ID: 2, Type: model.StampTypeFood},
		{ID: 3, Type: model.StampTypeGame}, {ID: 4, Type: model.StampTypeGame},
	}
	rally := Rally{CompletionMode: model.RallyCompletionCounts, RequiredFood: 1, RequiredGame: 1}
	ended := rally
	ended.AvailableTo = past

	cases := []struct {
		name      string
		rally     Rally
		collected map[int64]string
		want      bool
	}{
		{"nothing collected", rally, map[int64]string{}, false},
		{"one food only", rally, map[int64]string{1: ""}, false},
		{"both food, no game", rally, map[int64]string{1: "", 2: ""}, false},
		{"one of each", rally, map[int64]string{1: "", 3: ""}, true},
		{"more than required", rally, map[int64]string{1: "", 2: "", 3: "", 4: ""}, true},
		{"over, short of the requirement", ended, map[int64]string{1: ""}, false},
		{"over, having met it", ended, map[int64]string{2: "", 4: ""}, true},
	}
	for _, c := range cases {
		if got := CardComplete(c.rally, stamps, c.collected, now); got != c.want {
			t.Errorf("%s: complete = %v, want %v", c.name, got, c.want)
		}
	}

	// Requiring nothing of either type follows the whole-card rule instead of finishing at
	// the first stamp.
	none := Rally{CompletionMode: model.RallyCompletionCounts}
	if CardComplete(none, stamps, map[int64]string{1: ""}, now) {
		t.Error("a counts rally requiring 0/0 completed on one stamp")
	}
	if !CardComplete(none, stamps, map[int64]string{1: "", 2: "", 3: "", 4: ""}, now) {
		t.Error("a counts rally requiring 0/0 did not complete on the whole card")
	}
}

func TestCompletionRuleError(t *testing.T) {
	if CompletionRuleError(model.RallyCompletionCounts, 0, 0) == "" {
		t.Error("counts requiring nothing was accepted")
	}
	for _, c := range []struct {
		mode       string
		food, game int
	}{
		{model.RallyCompletionCounts, 1, 0},
		{model.RallyCompletionCounts, 0, 2},
		{model.RallyCompletionAll, 0, 0},
	} {
		if msg := CompletionRuleError(c.mode, c.food, c.game); msg != "" {
			t.Errorf("(%s, %d, %d) refused: %s", c.mode, c.food, c.game, msg)
		}
	}
}

func TestClampRequired(t *testing.T) {
	for _, c := range []struct{ want, available, out int }{{-3, 4, 0}, {2, 4, 2}, {9, 4, 4}, {5, 0, 0}} {
		if got := ClampRequired(c.want, c.available); got != c.out {
			t.Errorf("ClampRequired(%d, %d) = %d, want %d", c.want, c.available, got, c.out)
		}
	}
}
