package server_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"app-suite/internal/db"
)

// onceAnnouncement creates a one-time announcement scheduled at onceLocal in
// America/Chicago, returning its id and the body it was built from so a test can
// edit and PUT it back the way the admin form does.
func (e *testEnv) onceAnnouncement(t *testing.T, title, onceLocal string) (int64, map[string]any) {
	t.Helper()
	typeID := e.makeAnnouncementType(t, title+" type")
	body := announcementBody(typeID, map[string]any{
		"title": title, "details": "One night only", "scheduleKind": "once",
		"timezone": "America/Chicago", "onceLocal": onceLocal,
	})
	var made map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/announcements", body), http.StatusCreated, &made)
	return int64(made["id"].(float64)), body
}

// announcementRow reads one announcement's current row off the list endpoint.
func (e *testEnv) announcementRow(t *testing.T, id int64) map[string]any {
	t.Helper()
	var list struct{ Announcements []map[string]any }
	decodeInto(t, e.get(t, "/api/v1/announcements"), http.StatusOK, &list)
	for _, row := range list.Announcements {
		if int64(row["id"].(float64)) == id {
			return row
		}
	}
	t.Fatalf("announcement %d not found in the list", id)
	return nil
}

// spend puts an announcement in the state the scheduler leaves one in after it
// fires: no next post, and not live.
func (e *testEnv) spend(t *testing.T, id int64) {
	t.Helper()
	cur, _ := e.announcementRow(t, id)["nextPostAt"].(string)
	var prev *string
	if cur != "" {
		prev = &cur
	}
	// The cursor the row currently holds, so the guarded update matches.
	n, err := db.New(e.store.DB()).MarkAnnouncementPosted(t.Context(), db.MarkAnnouncementPostedParams{ID: id, PrevNextPostAt: prev})
	if err != nil || n != 1 {
		t.Fatalf("MarkAnnouncementPosted = %d, %v", n, err)
	}
}

// TestAnnouncementEditDoesNotRepostSpentOnce guards against the worst kind of bug
// this codebase can have: one that talks to Discord. A one-time announcement that
// has already gone out is left by the scheduler with no cursor and not live. An
// edit must not re-resolve onceLocal and arm it unconditionally, or fixing a typo
// in a posted announcement would re-arm it and the next 30-second sweep would send
// the whole thing again - @everyone and all. The cursor belongs to the scheduler,
// not the form, exactly as skipCount does.
func TestAnnouncementEditDoesNotRepostSpentOnce(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	past := time.Now().Add(-48 * time.Hour).Format("2006-01-02T15:04")
	id, fields := env.onceAnnouncement(t, "Spent Notice", past)
	env.spend(t, id)
	if row := env.announcementRow(t, id); row["active"] != false || row["nextPostAt"] != nil {
		t.Fatalf("precondition: active=%v nextPostAt=%v; want false and null", row["active"], row["nextPostAt"])
	}

	// The admin edits something unrelated and saves, exactly as the form does: the
	// whole announcement goes back, onceLocal included.
	fields["details"] = "One night only (fixed typo)"
	decodeInto(t, env.putJSON(t, fmt.Sprintf("/api/v1/announcements/%d", id), fields), http.StatusOK, nil)

	row := env.announcementRow(t, id)
	if row["active"] == true {
		t.Error("editing a spent one-time announcement re-armed it; the scheduler will post it again")
	}
	if row["nextPostAt"] != nil {
		t.Errorf("nextPostAt = %v; want null (cursor resurrected by an edit)", row["nextPostAt"])
	}
	if row["details"] != "One night only (fixed typo)" {
		t.Errorf("the edit itself did not save: details = %v", row["details"])
	}
}

// TestAnnouncementEditToFutureStillArms is the other half: moving the date to a
// future instant is a deliberate re-arm and must keep working, otherwise a
// one-time announcement could never be rescheduled.
func TestAnnouncementEditToFutureStillArms(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	past := time.Now().Add(-48 * time.Hour).Format("2006-01-02T15:04")
	id, fields := env.onceAnnouncement(t, "Reschedule Me", past)
	env.spend(t, id)

	// Chicago is UTC-5/-6, so a week out in local wall-clock is unambiguously in the
	// future whatever the offset.
	fields["onceLocal"] = time.Now().Add(7 * 24 * time.Hour).Format("2006-01-02T15:04")
	decodeInto(t, env.putJSON(t, fmt.Sprintf("/api/v1/announcements/%d", id), fields), http.StatusOK, nil)

	row := env.announcementRow(t, id)
	if row["active"] != true {
		t.Error("moving a one-time announcement to a future date must re-arm it")
	}
	if row["nextPostAt"] == nil {
		t.Error("nextPostAt was not set when rescheduling to the future")
	}
}

// A create keeps a past date, so a one-time announcement made for a moment already
// gone goes out on the next sweep.
func TestAnnouncementCreateInThePastIsArmed(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	past := time.Now().Add(-48 * time.Hour).Format("2006-01-02T15:04")
	id, _ := env.onceAnnouncement(t, "Overdue", past)
	row := env.announcementRow(t, id)
	if row["active"] != true || row["nextPostAt"] == nil {
		t.Errorf("a one-time announcement made in the past: active=%v nextPostAt=%v; want it armed", row["active"], row["nextPostAt"])
	}
}
