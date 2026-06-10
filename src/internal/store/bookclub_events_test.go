package store_test

import (
	"testing"

	"app-suite/internal/model"
)

func TestBookClubEventCRUD(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateBookClubEvent(&model.BookClubEvent{
		ClubSlug:    "yaoi",
		Title:       "August Meeting",
		StartLocal:  "2026-08-20T19:00",
		Timezone:    "America/New_York",
		LengthHours: 2,
		Location:    "VC #1",
		Image:       "https://example.com/a.png",
		PostAtLocal: "2026-08-15T09:00",
		StartAtUnix: 1_780_000_000,
		PostAtUnix:  1_779_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Fatalf("CreateBookClubEvent id = %d; want > 0", id)
	}

	got, err := s.GetBookClubEvent(id)
	if err != nil || got == nil {
		t.Fatalf("GetBookClubEvent = %+v, err=%v", got, err)
	}
	if got.Title != "August Meeting" || got.LengthHours != 2 || got.Timezone != "America/New_York" {
		t.Fatalf("event mismatch: %+v", got)
	}
	if got.StartAtUnix != 1_780_000_000 || got.PostAtUnix != 1_779_000_000 {
		t.Fatalf("unix fields not persisted: %+v", got)
	}
	if got.Posted {
		t.Fatalf("new event should not be posted")
	}

	// A different club must not see it.
	if other, err := s.ListBookClubEvents("yuri"); err != nil || len(other) != 0 {
		t.Fatalf("ListBookClubEvents(yuri) = %+v, err=%v; want empty", other, err)
	}

	// Update.
	got.LengthHours = 5
	got.Image = "https://example.com/b.png"
	if err := s.UpdateBookClubEvent(got); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := s.GetBookClubEvent(id)
	if reloaded.LengthHours != 5 || reloaded.Image != "https://example.com/b.png" {
		t.Fatalf("update not applied: %+v", reloaded)
	}

	// Delete.
	ok, err := s.DeleteBookClubEvent(id)
	if err != nil || !ok {
		t.Fatalf("DeleteBookClubEvent = %v, err=%v", ok, err)
	}
	if gone, _ := s.GetBookClubEvent(id); gone != nil {
		t.Fatalf("event still present after delete: %+v", gone)
	}
}

func TestDueBookClubEventsAndMarkPosted(t *testing.T) {
	s := newTestStore(t)

	// Due (post time in the past), pending.
	dueID, _ := s.CreateBookClubEvent(&model.BookClubEvent{
		ClubSlug: "yaoi", Title: "Due", LengthHours: 1, PostAtUnix: 1000, StartAtUnix: 2000,
	})
	// Future post time → not due.
	s.CreateBookClubEvent(&model.BookClubEvent{
		ClubSlug: "yaoi", Title: "Future", LengthHours: 1, PostAtUnix: 9_000_000_000, StartAtUnix: 9_000_100_000,
	})

	due, err := s.DueBookClubEvents(5000)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != dueID {
		t.Fatalf("DueBookClubEvents = %+v; want only the past event", due)
	}

	// After marking posted, it is no longer due.
	if err := s.MarkBookClubEventPosted(dueID); err != nil {
		t.Fatal(err)
	}
	posted, _ := s.GetBookClubEvent(dueID)
	if !posted.Posted || posted.PostedAt == "" {
		t.Fatalf("event not marked posted: %+v", posted)
	}
	due, _ = s.DueBookClubEvents(5000)
	if len(due) != 0 {
		t.Fatalf("DueBookClubEvents after mark = %d; want 0", len(due))
	}
}
