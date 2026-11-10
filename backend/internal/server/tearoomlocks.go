package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"app-suite/internal/api"
	"app-suite/internal/db"
	"app-suite/internal/tearooms"
)

// teaRoomPreflight answers a browser's preflight for the public tea-room reads: for
// an origin on the saved allowlist it permits the one method and the one request
// header those reads use. Any other preflight is left as the caller answers it.
func (s *Server) teaRoomPreflight(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" || !strings.HasPrefix(r.URL.Path, api.Prefix+"/tea-rooms/public") {
		return
	}
	h := w.Header()
	h.Add("Vary", "Origin")
	if !api.TeaRoomOriginAllowed(r.Context(), s.store, origin) {
		return
	}
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Methods", "GET")
	h.Set("Access-Control-Allow-Headers", "If-None-Match")
	h.Set("Access-Control-Max-Age", "600")
	h.Del("Access-Control-Allow-Credentials")
}

// A tea room can be locked, booked out, either until staff unlock it or until a set
// moment, after which this sweeper lifts it and tells every staff client (including
// the in-game plugin, which alerts its operator).

// teaRoomLockSchedulerInterval is how often the sweeper looks for locks that have
// run out. Shorter than the announcement scheduler's 30s: an expiry is a moment
// staff are waiting on, and the sweep is one small query against a table of a few
// dozen rows.
const teaRoomLockSchedulerInterval = 15 * time.Second

// RunTeaRoomLockScheduler lifts tea-room locks whose set expiry has passed, until
// ctx is cancelled. Safe to call in a goroutine.
func (s *Server) RunTeaRoomLockScheduler(ctx context.Context) {
	runScheduler(ctx, "tea-room-locks", teaRoomLockSchedulerInterval, func() { s.expireDueTeaRoomLocks(ctx) })
}

// expireDueTeaRoomLocks unlocks every room whose lock expiry has arrived and
// announces each one. Locks with no expiry are never touched: they wait on a person.
// It runs on startup too, so a lock that ran out while the process was down is lifted
// as soon as it is back.
func (s *Server) expireDueTeaRoomLocks(ctx context.Context) {
	rooms, err := db.New(s.store.ReadDB()).ListExpiringTeaRoomLocks(ctx)
	if err != nil {
		slog.Log(ctx, errorLevel(err), "tea room lock scheduler: load expiring locks", "error", err)
		return
	}
	now := time.Now()
	for _, room := range rooms {
		until := ""
		if room.LockedUntil != nil {
			until = *room.LockedUntil
		}
		at, ok := tearooms.ParseLockTime(until)
		// An expiry we can't read is not one we can honor, and leaving it would wedge
		// the room locked while logging the same complaint every sweep. Treat it as
		// due: the room unlocks, the warning is written once, and the bad value is
		// gone with it.
		if !ok {
			slog.Warn("tea room lock has an unreadable expiry; unlocking it",
				"id", room.ID, "name", room.Name, "locked_until", until)
		} else if now.Before(at) {
			continue
		}
		applied, err := db.New(s.store.DB()).ExpireTeaRoomLock(ctx, db.ExpireTeaRoomLockParams{ID: room.ID, LockedUntil: room.LockedUntil})
		if err != nil {
			slog.Error("tea room lock scheduler: expire lock", "id", room.ID, "error", err)
			continue
		}
		if applied == 0 {
			// An admin re-locked or re-timed the room while this sweep was running;
			// their lock wins over one judged from a snapshot taken before it.
			slog.Info("tea room lock changed during the sweep; leaving the admin's lock alone",
				"id", room.ID, "name", room.Name)
			continue
		}
		slog.Info("tea room lock expired", "id", room.ID, "name", room.Name, "room_number", room.RoomNumber)
		// The staff with the Tea Rooms page are told by name and number: the plugin
		// announces it to its operator the moment it lands, with no read to make.
		api.TeaRoomUnlocked(s.live, room.ID, room.Name, room.RoomNumber)
		// The rooms list itself changed, and nothing here went through the admin
		// mutation middleware (no request made this happen), so tell it too.
		s.resourceChanged("tea-rooms")
	}
}
