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

func TestTeaRoomLock(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateTeaRoom(&model.TeaRoom{Name: "R", RoomNumber: "lock-1", Open: true})

	// A fresh room is unlocked with no expiry.
	got, _ := s.GetTeaRoom(id)
	if got.Locked || got.LockedUntil != "" {
		t.Errorf("new room: locked=%v until=%q; want unlocked with no expiry", got.Locked, got.LockedUntil)
	}

	// Locked with an expiry.
	until := "2030-01-02T15:04:05Z"
	if err := s.SetTeaRoomLock(id, true, until); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetTeaRoom(id)
	if !got.Locked || got.LockedUntil != until {
		t.Errorf("locked=%v until=%q; want true / %q", got.Locked, got.LockedUntil, until)
	}
	if !got.Open {
		t.Error("locking must not change the open flag")
	}

	// Unlocking drops the expiry with the lock - a leftover time must not outlive it.
	if err := s.SetTeaRoomLock(id, false, until); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetTeaRoom(id)
	if got.Locked || got.LockedUntil != "" {
		t.Errorf("after unlock: locked=%v until=%q; want unlocked with no expiry", got.Locked, got.LockedUntil)
	}

	// A lock with no expiry is the manual-unlock kind.
	if err := s.SetTeaRoomLock(id, true, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetTeaRoom(id)
	if !got.Locked || got.LockedUntil != "" {
		t.Errorf("indefinite lock: locked=%v until=%q", got.Locked, got.LockedUntil)
	}
}

func TestTeaRoomExpiringLocks(t *testing.T) {
	s := newTestStore(t)
	timed, _ := s.CreateTeaRoom(&model.TeaRoom{Name: "Timed", RoomNumber: "exp-1"})
	forever, _ := s.CreateTeaRoom(&model.TeaRoom{Name: "Forever", RoomNumber: "exp-2"})
	_, _ = s.CreateTeaRoom(&model.TeaRoom{Name: "Free", RoomNumber: "exp-3"})

	until := "2030-01-02T15:04:05Z"
	if err := s.SetTeaRoomLock(timed, true, until); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTeaRoomLock(forever, true, ""); err != nil {
		t.Fatal(err)
	}

	// Only the lock that can lift on its own is listed - the indefinite one waits
	// on a person, and the unlocked room has no lock at all.
	rooms, err := s.ExpiringTeaRoomLocks()
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 1 || rooms[0].ID != timed {
		t.Fatalf("ExpiringTeaRoomLocks = %+v; want only room %d", rooms, timed)
	}

	// Expiring is conditional on the expiry the caller saw still being in place.
	applied, err := s.ExpireTeaRoomLock(timed, "2030-01-02T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Error("expected a stale expiry not to unlock the room")
	}
	if got, _ := s.GetTeaRoom(timed); !got.Locked {
		t.Error("room unlocked on a stale expiry")
	}

	applied, err = s.ExpireTeaRoomLock(timed, until)
	if err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("expected the matching expiry to unlock the room")
	}
	got, _ := s.GetTeaRoom(timed)
	if got.Locked || got.LockedUntil != "" {
		t.Errorf("after expiry: locked=%v until=%q", got.Locked, got.LockedUntil)
	}

	// Expiring the same lock twice is a no-op, so a re-run of the sweep can't
	// announce an unlock that already happened.
	if applied, _ := s.ExpireTeaRoomLock(timed, until); applied {
		t.Error("expected the second expiry to be a no-op")
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
