package store_test

import (
	"testing"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

func mustCreateAnnouncement(t *testing.T, s *store.Store, typeID int64, title string) int64 {
	t.Helper()
	id, err := s.CreateAnnouncement(&model.Announcement{
		TypeID:  typeID,
		Title:   title,
		Details: "body",
		Active:  true,
	})
	if err != nil {
		t.Fatalf("CreateAnnouncement(%s): %v", title, err)
	}
	return id
}

func listAnnouncementTitles(t *testing.T, s *store.Store) []string {
	t.Helper()
	items, err := s.ListAnnouncements()
	if err != nil {
		t.Fatalf("ListAnnouncements: %v", err)
	}
	titles := make([]string, len(items))
	for i, a := range items {
		titles[i] = a.Title
	}
	return titles
}

// TestListAnnouncements_DefaultOrder verifies the default (un-reordered) list is
// newest-first — the behavior preserved by sort_order defaulting to 0.
func TestListAnnouncements_DefaultOrder(t *testing.T) {
	s := newTestStore(t)
	typeID, err := s.CreateAnnouncementType("Events", "https://discord/webhook")
	if err != nil {
		t.Fatal(err)
	}
	mustCreateAnnouncement(t, s, typeID, "first")
	mustCreateAnnouncement(t, s, typeID, "second")
	mustCreateAnnouncement(t, s, typeID, "third")

	if got := listAnnouncementTitles(t, s); !equalSlice(got, []string{"third", "second", "first"}) {
		t.Errorf("default order = %v; want newest-first [third second first]", got)
	}
}

// TestBulkReorderAnnouncements verifies a drag-reorder persists and that a newly
// created announcement afterwards lands at the top (sort_order 0, id tie-break).
func TestBulkReorderAnnouncements(t *testing.T) {
	s := newTestStore(t)
	typeID, err := s.CreateAnnouncementType("Events", "https://discord/webhook")
	if err != nil {
		t.Fatal(err)
	}
	a := mustCreateAnnouncement(t, s, typeID, "alpha")
	b := mustCreateAnnouncement(t, s, typeID, "bravo")
	c := mustCreateAnnouncement(t, s, typeID, "charlie")

	// Reorder to alpha, charlie, bravo.
	if err := s.BulkReorderAnnouncements([]int64{a, c, b}); err != nil {
		t.Fatalf("BulkReorderAnnouncements: %v", err)
	}
	if got := listAnnouncementTitles(t, s); !equalSlice(got, []string{"alpha", "charlie", "bravo"}) {
		t.Fatalf("after reorder = %v; want [alpha charlie bravo]", got)
	}

	// A new announcement defaults to sort_order 0 → appears at the very top.
	mustCreateAnnouncement(t, s, typeID, "delta")
	if got := listAnnouncementTitles(t, s); got[0] != "delta" {
		t.Errorf("new announcement should be first; got %v", got)
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
