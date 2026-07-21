package store_test

import (
	"errors"
	"testing"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// makeRally creates a rally with two stamps and one prize, returning the reloaded
// rally (so callers get the assigned stamp/prize ids).
func makeRally(t *testing.T, s *store.Store, title string) *model.StampRally {
	t.Helper()
	id, err := s.CreateStampRally(&model.StampRally{
		Title:           title,
		CardImage:       "images/stamp_cards/card.png",
		NotStampedImage: "images/stamp_stamps/blank.png",
		Stamps: []model.StampRallyStamp{
			{Image: "images/stamp_stamps/a.png", Password: "alpha", Placement: model.Placement{X: 10, Y: 10, Width: 15, Height: 15}},
			{Image: "images/stamp_stamps/b.png", Password: "bravo", Placement: model.Placement{X: 50, Y: 50, Width: 15, Height: 15, Rotation: 30}},
		},
		Prizes: []model.StampRallyPrize{
			{Name: "Trophy", Image: "images/stamp_prizes/t.png", Placement: model.Placement{X: 70, Y: 20, Width: 20, Height: 20}},
		},
	})
	if err != nil {
		t.Fatalf("CreateStampRally: %v", err)
	}
	r, err := s.GetStampRally(id)
	if err != nil || r == nil {
		t.Fatalf("GetStampRally: r=%v err=%v", r, err)
	}
	return r
}

func TestStampRally_CreateGet(t *testing.T) {
	s := newTestStore(t)
	r := makeRally(t, s, "Summer Rally")

	if len(r.Stamps) != 2 || len(r.Prizes) != 1 {
		t.Fatalf("got %d stamps, %d prizes; want 2, 1", len(r.Stamps), len(r.Prizes))
	}
	// Placement round-trips (incl. rotation on the second stamp).
	if r.Stamps[1].Rotation != 30 {
		t.Errorf("stamp rotation = %v; want 30", r.Stamps[1].Rotation)
	}
	if r.Stamps[0].Password != "alpha" {
		t.Errorf("stamp password = %q; want alpha", r.Stamps[0].Password)
	}
}

func TestStampRally_UpdateKeepsCollections(t *testing.T) {
	s := newTestStore(t)
	r := makeRally(t, s, "Rally")
	stamp1 := r.Stamps[0]

	card, err := s.IssueRallyCard(r.ID, "Tataru")
	if err != nil {
		t.Fatalf("IssueRallyCard: %v", err)
	}
	if _, err := s.CollectStamp(r.ID, card.ID, stamp1.ID, "Tataru", "Senpan Tea House"); err != nil {
		t.Fatalf("CollectStamp: %v", err)
	}

	// Update the rally, KEEPING stamp1 by id (and editing its password) and stamp2.
	r.Title = "Rally (edited)"
	r.Stamps[0].Password = "alpha2"
	if err := s.UpdateStampRally(r); err != nil {
		t.Fatalf("UpdateStampRally: %v", err)
	}

	// stamp1 kept its id, so the collection survives the edit.
	collected, err := s.ListCollectedStampIDs(card.ID)
	if err != nil {
		t.Fatalf("ListCollectedStampIDs: %v", err)
	}
	if _, ok := collected[stamp1.ID]; !ok {
		t.Errorf("collection for stamp %d lost after update; want preserved", stamp1.ID)
	}
}

// TestStampRally_UpdateStampScopedToRally guards the rally-scoped stamp UPDATE: a
// stamp update must not be able to reach across events. Editing rally A while passing
// a stamp id that actually belongs to rally B must leave rally B's stamp untouched.
func TestStampRally_UpdateStampScopedToRally(t *testing.T) {
	s := newTestStore(t)
	a := makeRally(t, s, "Rally A")
	b := makeRally(t, s, "Rally B")

	victim := b.Stamps[0] // belongs to rally B
	origPassword := victim.Password

	// Craft an update to rally A that tries to hijack rally B's stamp id, rewriting
	// its password. The rally-scoped WHERE must prevent the cross-event write.
	a.Stamps = append(a.Stamps, model.StampRallyStamp{
		ID:       victim.ID, // spoofed: this id lives in rally B
		Image:    "images/stamp_stamps/x.png",
		Password: "hijacked",
	})
	if err := s.UpdateStampRally(a); err != nil {
		t.Fatalf("UpdateStampRally: %v", err)
	}

	rb, err := s.GetStampRally(b.ID)
	if err != nil || rb == nil {
		t.Fatalf("GetStampRally(B): rb=%v err=%v", rb, err)
	}
	var found *model.StampRallyStamp
	for i := range rb.Stamps {
		if rb.Stamps[i].ID == victim.ID {
			found = &rb.Stamps[i]
		}
	}
	if found == nil {
		t.Fatalf("rally B stamp %d vanished after cross-rally update", victim.ID)
	}
	if found.Password != origPassword {
		t.Errorf("rally B stamp password = %q; want %q (cross-rally update leaked)", found.Password, origPassword)
	}
}

func TestStampRally_ListStallCounts(t *testing.T) {
	s := newTestStore(t)
	r := makeRally(t, s, "Rally") // 2 stamps, none paused

	list, err := s.ListStampRallies()
	if err != nil {
		t.Fatalf("ListStampRallies: %v", err)
	}
	if list[0].StampCount != 2 || list[0].ActiveStampCount != 2 {
		t.Fatalf("counts = %d/%d active; want 2/2", list[0].ActiveStampCount, list[0].StampCount)
	}

	// Pausing one stall drops the active count (the at-a-glance "X/Y active" summary).
	if _, err := s.SetStampPaused(r.ID, r.Stamps[0].ID, true); err != nil {
		t.Fatalf("SetStampPaused: %v", err)
	}
	list, _ = s.ListStampRallies()
	if list[0].ActiveStampCount != 1 {
		t.Errorf("active after pause = %d; want 1", list[0].ActiveStampCount)
	}
}

func TestStampRally_CollectUniqueGuard(t *testing.T) {
	s := newTestStore(t)
	r := makeRally(t, s, "Rally")
	card, _ := s.IssueRallyCard(r.ID, "Solo")

	if _, err := s.CollectStamp(r.ID, card.ID, r.Stamps[0].ID, "Solo", "Senpan Tea House"); err != nil {
		t.Fatalf("first collect: %v", err)
	}
	_, err := s.CollectStamp(r.ID, card.ID, r.Stamps[0].ID, "Solo", "Senpan Tea House")
	if !errors.Is(err, store.ErrStampAlreadyCollected) {
		t.Errorf("second collect err = %v; want ErrStampAlreadyCollected", err)
	}
}

func TestStampRally_LogsAndDelete(t *testing.T) {
	s := newTestStore(t)
	r := makeRally(t, s, "Rally")
	c1, _ := s.IssueRallyCard(r.ID, "Aria")
	c2, _ := s.IssueRallyCard(r.ID, "Borin")
	_, _ = s.CollectStamp(r.ID, c1.ID, r.Stamps[0].ID, "Aria", "Senpan Tea House")
	_, _ = s.CollectStamp(r.ID, c2.ID, r.Stamps[1].ID, "Borin", "Senpan Tea House")

	logs, err := s.ListRallyCollections(r.ID)
	if err != nil {
		t.Fatalf("ListRallyCollections: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("logs = %d; want 2", len(logs))
	}
	// No affiliate → the default stall name.
	if logs[0].StallName != "Senpan Tea House" {
		t.Errorf("stall = %q; want Senpan Tea House", logs[0].StallName)
	}

	// Delete cascades cards + collections.
	deleted, err := s.DeleteStampRally(r.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteStampRally: deleted=%v err=%v", deleted, err)
	}
	if gone, _ := s.GetStampRally(r.ID); gone != nil {
		t.Errorf("rally still present after delete")
	}
	if logs, _ := s.ListRallyCollections(r.ID); len(logs) != 0 {
		t.Errorf("collections survived rally delete")
	}
}
