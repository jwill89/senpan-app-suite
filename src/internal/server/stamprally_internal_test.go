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
			"zero collected, all expired → not complete",
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
			"event ended → complete with one collected",
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
