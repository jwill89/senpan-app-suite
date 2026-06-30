package store_test

import (
	"testing"

	"app-suite/internal/model"
)

func TestReadingListCRUD(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateReadingList("yaoi", "Summer Picks")
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Fatalf("CreateReadingList id = %d; want > 0", id)
	}

	lists, err := s.ListReadingLists("yaoi")
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || lists[0].Title != "Summer Picks" || lists[0].ClubSlug != "yaoi" {
		t.Fatalf("ListReadingLists = %+v; want one 'Summer Picks' yaoi list", lists)
	}

	// A different club must not see this list.
	if other, err := s.ListReadingLists("other"); err != nil || len(other) != 0 {
		t.Fatalf("ListReadingLists(other) = %+v, err=%v; want empty", other, err)
	}

	if err := s.UpdateReadingListTitle(id, "Winter Picks"); err != nil {
		t.Fatal(err)
	}
	list, err := s.GetReadingList(id)
	if err != nil {
		t.Fatal(err)
	}
	if list == nil || list.Title != "Winter Picks" {
		t.Fatalf("GetReadingList title = %+v; want 'Winter Picks'", list)
	}
	if list.Items == nil {
		t.Error("Items should be a non-nil (empty) slice")
	}

	deleted, err := s.DeleteReadingList(id)
	if err != nil || !deleted {
		t.Fatalf("DeleteReadingList = %v, err=%v; want true", deleted, err)
	}
	if got, _ := s.GetReadingList(id); got != nil {
		t.Error("expected nil after delete")
	}
}

func TestReadingListItemsRoundTrip(t *testing.T) {
	s := newTestStore(t)

	listID, err := s.CreateReadingList("yaoi", "My List")
	if err != nil {
		t.Fatal(err)
	}

	item := &model.ReadingListItem{
		ListID:     listID,
		CoverImage: "https://media.example.org/cover.webp",
		Title:      "Sex Stopwatch",
		Summary:    "He stopped time…",
		Format:     "Manhwa",
		Genres:     "Fantasy, Comedy",
		Tropes:     "Enemies to Lovers, Time Travel",
		Chapters:   "156",
		Comments:   "Yao says: peak.",
		Sources: []model.ReadingListSource{
			{Title: "OmegaScans", URL: "https://omegascans.org/series/sex-stopwatch"},
			{Title: "Official", URL: "https://example.com/buy"},
		},
	}
	itemID, err := s.CreateReadingListItem(item)
	if err != nil {
		t.Fatal(err)
	}

	// Add a second item to verify ordering by sort_order.
	if _, err := s.CreateReadingListItem(&model.ReadingListItem{ListID: listID, Title: "Second"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetReadingList(listID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items = %d; want 2", len(got.Items))
	}
	first := got.Items[0]
	if first.ID != itemID || first.Title != "Sex Stopwatch" {
		t.Fatalf("first item = %+v; want the created one first", first)
	}
	if first.Tropes != "Enemies to Lovers, Time Travel" {
		t.Errorf("tropes round-trip failed: %q", first.Tropes)
	}
	if len(first.Sources) != 2 || first.Sources[0].Title != "OmegaScans" ||
		first.Sources[1].URL != "https://example.com/buy" {
		t.Fatalf("sources round-trip failed: %+v", first.Sources)
	}
	if got.Items[1].Title != "Second" {
		t.Errorf("ordering wrong: %+v", got.Items[1])
	}

	// Update the first item.
	first.Title = "Renamed"
	first.Sources = []model.ReadingListSource{{Title: "Only", URL: "https://x.test"}}
	if err := s.UpdateReadingListItem(&first); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetReadingList(listID)
	if got2.Items[0].Title != "Renamed" || len(got2.Items[0].Sources) != 1 {
		t.Fatalf("update failed: %+v", got2.Items[0])
	}

	// Delete the first item.
	deleted, err := s.DeleteReadingListItem(itemID)
	if err != nil || !deleted {
		t.Fatalf("DeleteReadingListItem = %v, err=%v", deleted, err)
	}
	got3, _ := s.GetReadingList(listID)
	if len(got3.Items) != 1 || got3.Items[0].Title != "Second" {
		t.Fatalf("after delete items = %+v; want only 'Second'", got3.Items)
	}
}

func TestDeleteReadingListCascadesItems(t *testing.T) {
	s := newTestStore(t)
	listID, _ := s.CreateReadingList("yaoi", "L")
	if _, err := s.CreateReadingListItem(&model.ReadingListItem{ListID: listID, Title: "A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteReadingList(listID); err != nil {
		t.Fatal(err)
	}
	// Recreate a list with a new id and confirm no stray items leak in.
	newID, _ := s.CreateReadingList("yaoi", "L2")
	got, _ := s.GetReadingList(newID)
	if len(got.Items) != 0 {
		t.Errorf("items leaked across lists: %+v", got.Items)
	}
}

// TestCountReadingListItemsByCover backs the reference-safe cover cleanup: covers
// keep their uploaded filename so two items can share one file, and a file must
// only be deleted once nothing references it.
func TestCountReadingListItemsByCover(t *testing.T) {
	s := newTestStore(t)
	listID, _ := s.CreateReadingList("yaoi", "L")
	const shared = "https://host/images/bookclub/cat.jpg"
	a, _ := s.CreateReadingListItem(&model.ReadingListItem{ListID: listID, Title: "A", CoverImage: shared})
	if _, err := s.CreateReadingListItem(&model.ReadingListItem{ListID: listID, Title: "B", CoverImage: shared}); err != nil {
		t.Fatal(err)
	}

	if n, err := s.CountReadingListItemsByCover(shared); err != nil || n != 2 {
		t.Fatalf("CountReadingListItemsByCover = %d, %v; want 2, nil", n, err)
	}
	// After deleting one item the shared cover is still referenced by the other.
	if _, err := s.DeleteReadingListItem(a); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountReadingListItemsByCover(shared); n != 1 {
		t.Fatalf("after one delete: count = %d; want 1 (file must NOT be removed yet)", n)
	}
	if n, _ := s.CountReadingListItemsByCover("https://host/images/bookclub/none.jpg"); n != 0 {
		t.Fatalf("unreferenced cover: count = %d; want 0", n)
	}
}
