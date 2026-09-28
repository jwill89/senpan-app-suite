package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"app-suite/internal/model"
	"app-suite/internal/server"

	"github.com/coder/websocket"
)

// -- Tea-room locks ----------------------------------------------------------
//
// A room can be locked indefinitely (lifted by hand, how locking has always
// worked) or until a set moment, after which the background sweeper lifts it and
// tells the admin clients - the in-game plugin alerts its operator off that
// message, so it must carry the room's name.

// createTeaRoom creates a room through the API and returns its id.
func (e *testEnv) createTeaRoom(t *testing.T, name, number string) int64 {
	t.Helper()
	resp := e.postJSON(t, "/api/tea-rooms", map[string]any{
		"tea_room": map[string]any{"name": name, "room_number": number},
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create tea room status = %d; want 201", resp.StatusCode)
	}
	room, _ := decodeBody(t, resp)["tea_room"].(map[string]any)
	if room == nil {
		t.Fatal("create tea room returned no room")
	}
	return int64(room["id"].(float64))
}

// patchTeaRoom PATCHes a room and returns the saved room from the response.
func (e *testEnv) patchTeaRoom(t *testing.T, id int64, fields map[string]any) map[string]any {
	t.Helper()
	resp := e.patchJSON(t, fmt.Sprintf("/api/tea-rooms/%d", id), fields)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("patch tea room %v status = %d; want 200", fields, resp.StatusCode)
	}
	room, _ := decodeBody(t, resp)["tea_room"].(map[string]any)
	if room == nil {
		t.Fatal("patch tea room returned no room")
	}
	return room
}

// assertLock checks a room JSON object's lock state.
func assertLock(t *testing.T, room map[string]any, wantLocked bool, wantUntil string) {
	t.Helper()
	if room["locked"] != wantLocked || room["locked_until"] != wantUntil {
		t.Errorf("lock = %v / %q; want %v / %q",
			room["locked"], room["locked_until"], wantLocked, wantUntil)
	}
}

// TestTeaRoomLockPatch covers the lock half of PATCH /api/tea-rooms/{id}: the two
// fields move independently, an expiry is stored canonically whatever RFC-3339
// spelling arrives, and unlocking takes the expiry with it.
func TestTeaRoomLockPatch(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createTeaRoom(t, "Jasmine Room", "12")

	// Locking with an expiry. The browser sends the millisecond form
	// (Date.toISOString()); it is stored as plain RFC-3339 so the sweeper's
	// compare-and-set has one spelling to match.
	room := env.patchTeaRoom(t, id, map[string]any{
		"locked": true, "locked_until": "2030-01-02T15:04:05.000Z",
	})
	assertLock(t, room, true, "2030-01-02T15:04:05Z")

	// locked_until alone re-times a lock already in place.
	room = env.patchTeaRoom(t, id, map[string]any{"locked_until": "2030-02-03T04:05:06Z"})
	assertLock(t, room, true, "2030-02-03T04:05:06Z")

	// A discount toggle in between leaves the lock exactly as it was.
	room = env.patchTeaRoom(t, id, map[string]any{"discounted": true})
	assertLock(t, room, true, "2030-02-03T04:05:06Z")

	// Unlocking clears the expiry, so a stale time can't outlive its lock.
	room = env.patchTeaRoom(t, id, map[string]any{"locked": false})
	assertLock(t, room, false, "")

	// Locking with no expiry is the manual-unlock lock.
	room = env.patchTeaRoom(t, id, map[string]any{"locked": true})
	assertLock(t, room, true, "")

	// An unreadable time is refused rather than quietly dropped - a lock that
	// silently never expires is the failure this endpoint must not have. The
	// refusal also has to be all-or-nothing: each flag is its own write, so a
	// rejected time must not leave the flag sent beside it already applied.
	resp := env.patchJSON(t, fmt.Sprintf("/api/tea-rooms/%d", id), map[string]any{
		"discounted": false, "locked": true, "locked_until": "tomorrow evening",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unreadable locked_until status = %d; want 400", resp.StatusCode)
	}
	got, _ := env.store.GetTeaRoom(id)
	if got.LockedUntil != "" {
		t.Errorf("rejected time was still stored: %q", got.LockedUntil)
	}
	if !got.Discounted {
		t.Error("the discount toggle sent with the rejected time was applied anyway")
	}
}

// TestParseTeaRoomLockTime pins the accepted spelling: RFC-3339 only, because
// every value the server stores has been through normalizeLockUntil. A naive
// local-looking string is rejected rather than guessed at as UTC.
func TestParseTeaRoomLockTime(t *testing.T) {
	at, ok := server.ParseTeaRoomLockTimeForTest("2030-01-02T15:04:05Z")
	if !ok || !at.Equal(time.Date(2030, 1, 2, 15, 4, 5, 0, time.UTC)) {
		t.Errorf("RFC-3339 = %v, %v", at, ok)
	}
	// An offset is normalized to UTC.
	at, ok = server.ParseTeaRoomLockTimeForTest("2030-01-02T10:04:05-05:00")
	if !ok || !at.Equal(time.Date(2030, 1, 2, 15, 4, 5, 0, time.UTC)) {
		t.Errorf("offset form = %v, %v", at, ok)
	}
	for _, bad := range []string{"", "2030-01-02T15:04", "tomorrow", "2030-01-02"} {
		if _, ok := server.ParseTeaRoomLockTimeForTest(bad); ok {
			t.Errorf("parseTeaRoomLockTime(%q) accepted", bad)
		}
	}
}

// expectTeaRoomUnlocked reads WS frames until a tea_room_unlocked for wantName
// arrives, or fails on timeout. Other message types are skipped - the sweep also
// emits resource_changed, and the socket carries unrelated traffic.
func expectTeaRoomUnlocked(t *testing.T, conn *websocket.Conn, wantName string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for tea_room_unlocked(%q): %v", wantName, err)
		}
		var msg struct {
			Type       string `json:"type"`
			Name       string `json:"name"`
			RoomNumber string `json:"room_number"`
		}
		if json.Unmarshal(data, &msg) != nil || msg.Type != "tea_room_unlocked" {
			continue
		}
		if msg.Name != wantName {
			t.Fatalf("tea_room_unlocked name = %q; want %q", msg.Name, wantName)
		}
		if msg.RoomNumber == "" {
			t.Error("tea_room_unlocked carried no room number")
		}
		return
	}
}

// TestTeaRoomLockExpirySweep is the whole point of the expiry: a lock whose time
// has passed lifts on its own and says so by name, while a lock with no expiry
// and one still in the future are left for the people who set them.
func TestTeaRoomLockExpirySweep(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	due, err := env.store.CreateTeaRoom(&model.TeaRoom{Name: "Jasmine Room", RoomNumber: "12"})
	if err != nil {
		t.Fatal(err)
	}
	indefinite, _ := env.store.CreateTeaRoom(&model.TeaRoom{Name: "Peony Room", RoomNumber: "13"})
	future, _ := env.store.CreateTeaRoom(&model.TeaRoom{Name: "Lotus Room", RoomNumber: "14"})

	now := time.Now().UTC()
	if err := env.store.SetTeaRoomLock(due, true, now.Add(-time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetTeaRoomLock(indefinite, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetTeaRoomLock(future, true, now.Add(time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}

	admin := env.dialAdminWS(t)
	defer admin.Close(websocket.StatusNormalClosure, "")

	server.ExpireDueTeaRoomLocksForTest(env.srv)

	if got, _ := env.store.GetTeaRoom(due); got.Locked || got.LockedUntil != "" {
		t.Errorf("expired room: locked=%v until=%q; want unlocked with no expiry", got.Locked, got.LockedUntil)
	}
	if got, _ := env.store.GetTeaRoom(indefinite); !got.Locked {
		t.Error("a lock with no expiry was swept; it waits on a person, not the clock")
	}
	if got, _ := env.store.GetTeaRoom(future); !got.Locked {
		t.Error("a lock still in the future was swept")
	}

	expectTeaRoomUnlocked(t, admin, "Jasmine Room")

	// A second pass has nothing left to do, so nothing is announced twice.
	server.ExpireDueTeaRoomLocksForTest(env.srv)
	expectNoTeaRoomUnlocked(t, admin, 500*time.Millisecond)
}

// expectNoTeaRoomUnlocked reads until the deadline and fails if any
// tea_room_unlocked arrives. Other traffic is ignored - the point is the absence
// of this one message, not silence on the socket.
func expectNoTeaRoomUnlocked(t *testing.T, conn *websocket.Conn, within time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), within)
	defer cancel()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return // deadline reached with nothing announced: the expected outcome.
		}
		var msg struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &msg) == nil && msg.Type == "tea_room_unlocked" {
			t.Fatalf("lock expiry announced twice: %s", data)
		}
	}
}
