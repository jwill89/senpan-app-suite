package store_test

import (
	"testing"

	"app-suite/internal/model"
)

func TestTeaRoomCRUD(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateTeaRoom(&model.TeaRoom{
		Name:            "Jasmine Room",
		Subtitle:        "「 静かな部屋 」", // UTF-8 (Japanese) round-trips
		RoomNumber:      "1",
		CostPerHalfHour: 125000,
		Hashtags:        "#cozy #private",
		Description:     "A quiet room.",
		Seasonal:        true,
		Open:            true,
		Lockable:        true,
		Image:           "https://example.com/a.png",
		Color:           "#abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}

	got, err := s.GetTeaRoom(id)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected room, got nil")
	}
	if got.Name != "Jasmine Room" || got.RoomNumber != "1" {
		t.Errorf("name/number = %q / %q", got.Name, got.RoomNumber)
	}
	if got.Subtitle != "「 静かな部屋 」" {
		t.Errorf("subtitle = %q", got.Subtitle)
	}
	if got.CostPerHalfHour != 125000 {
		t.Errorf("cost = %d; want 125000", got.CostPerHalfHour)
	}
	if !got.Seasonal || !got.Open || !got.Lockable || got.Discounted {
		t.Errorf("flags = seasonal:%v open:%v lockable:%v discounted:%v",
			got.Seasonal, got.Open, got.Lockable, got.Discounted)
	}
	if got.Hashtags != "#cozy #private" || got.Color != "#abcdef" {
		t.Errorf("hashtags/color = %q / %q", got.Hashtags, got.Color)
	}

	// Update replaces the editable fields (sort_order is preserved).
	got.Name = "Jasmine Suite"
	got.CostPerHalfHour = 200000
	got.Discounted = true
	got.Open = false
	if err := s.UpdateTeaRoom(got); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetTeaRoom(id)
	if got.Name != "Jasmine Suite" || got.CostPerHalfHour != 200000 || !got.Discounted || got.Open {
		t.Errorf("after update: name=%q cost=%d discounted=%v open=%v",
			got.Name, got.CostPerHalfHour, got.Discounted, got.Open)
	}

	rooms, err := s.ListTeaRooms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 1 {
		t.Fatalf("expected 1 room, got %d", len(rooms))
	}

	deleted, err := s.DeleteTeaRoom(id)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Error("expected deleted=true")
	}
	if got, _ := s.GetTeaRoom(id); got != nil {
		t.Error("expected nil after delete")
	}
}

func TestTeaRoomGetNotFound(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetTeaRoom(9999)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil for missing room")
	}
}

func TestTeaRoomByNumber(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateTeaRoom(&model.TeaRoom{Name: "Room", RoomNumber: "West 3"})

	got, err := s.GetTeaRoomByNumber("West 3")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != id {
		t.Fatalf("GetTeaRoomByNumber = %+v; want id %d", got, id)
	}
	if missing, _ := s.GetTeaRoomByNumber("nope"); missing != nil {
		t.Error("expected nil for an unknown room number")
	}
}

func TestTeaRoomToggles(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateTeaRoom(&model.TeaRoom{Name: "R", RoomNumber: "toggle-1", Open: true})

	if err := s.SetTeaRoomOpen(id, false); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTeaRoom(id)
	if got.Open {
		t.Error("expected open=false after SetTeaRoomOpen(false)")
	}

	if err := s.SetTeaRoomDiscounted(id, true); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetTeaRoom(id)
	if !got.Discounted {
		t.Error("expected discounted=true")
	}
	if got.Open {
		t.Error("toggling discount must not change the open flag")
	}
}

func TestTeaRoomReorder(t *testing.T) {
	s := newTestStore(t)
	idA, _ := s.CreateTeaRoom(&model.TeaRoom{Name: "A", RoomNumber: "A"})
	idB, _ := s.CreateTeaRoom(&model.TeaRoom{Name: "B", RoomNumber: "B"})
	idC, _ := s.CreateTeaRoom(&model.TeaRoom{Name: "C", RoomNumber: "C"})

	// Reorder to B, C, A.
	if err := s.BulkReorderTeaRooms([]int64{idB, idC, idA}); err != nil {
		t.Fatal(err)
	}
	rooms, err := s.ListTeaRooms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 3 {
		t.Fatalf("expected 3 rooms, got %d", len(rooms))
	}
	if rooms[0].ID != idB || rooms[1].ID != idC || rooms[2].ID != idA {
		t.Errorf("unexpected order: %d, %d, %d (want %d, %d, %d)",
			rooms[0].ID, rooms[1].ID, rooms[2].ID, idB, idC, idA)
	}
}
