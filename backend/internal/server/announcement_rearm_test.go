package server_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// onceAnnouncement creates a one-time announcement scheduled at onceLocal in
// America/Chicago, returning its id and the request body it was built from so a
// test can edit and PUT it back the way the admin form does.
func (e *testEnv) onceAnnouncement(t *testing.T, title, onceLocal string) (int, map[string]any) {
	t.Helper()
	tr := e.postJSON(t, "/api/announcement-types", map[string]any{"name": title + " type", "webhook_url": ""})
	if tr.StatusCode != http.StatusCreated {
		t.Fatalf("create announcement type = %d; want 201", tr.StatusCode)
	}
	typeID := int(decodeBody(t, tr)["type"].(map[string]any)["id"].(float64))

	fields := map[string]any{
		"type_id":       typeID,
		"title":         title,
		"details":       "One night only",
		"schedule_kind": "once",
		"timezone":      "America/Chicago",
		"once_local":    onceLocal,
	}
	cr := e.postJSON(t, "/api/announcements", map[string]any{"announcement": fields})
	if cr.StatusCode != http.StatusCreated {
		t.Fatalf("create announcement = %d; want 201", cr.StatusCode)
	}
	return int(decodeBody(t, cr)["announcement"].(map[string]any)["id"].(float64)), fields
}

// announcementRow reads one announcement's current row off the list endpoint.
func (e *testEnv) announcementRow(t *testing.T, id int) map[string]any {
	t.Helper()
	for _, row := range decodeBody(t, e.get(t, "/api/announcements"))["announcements"].([]any) {
		m := row.(map[string]any)
		if int(m["id"].(float64)) == id {
			return m
		}
	}
	t.Fatalf("announcement %d not found in the list", id)
	return nil
}

// TestAnnouncementEditDoesNotRepostSpentOnce is the regression for the worst kind
// of bug this codebase can have: one that talks to Discord. A one-time
// announcement that has already gone out is left by the scheduler with an empty
// cursor and active=false. The edit handler re-resolved once_local and set
// next_post_at/active unconditionally, so fixing a typo in a posted announcement
// re-armed it and the next 30-second sweep sent the whole thing again - @everyone
// and all. The cursor belongs to the scheduler, not the form, exactly as
// skip_count already does.
func TestAnnouncementEditDoesNotRepostSpentOnce(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	past := time.Now().Add(-48 * time.Hour).Format("2006-01-02T15:04")
	id, fields := env.onceAnnouncement(t, "Spent Notice", past)

	// Put it in the state the scheduler leaves after it fires.
	// The cursor the row currently holds, so the guarded update matches.
	cur := env.announcementRow(t, id)["next_post_at"].(string)
	if _, err := env.store.MarkAnnouncementPosted(int64(id), cur, "", false); err != nil {
		t.Fatalf("MarkAnnouncementPosted: %v", err)
	}
	if row := env.announcementRow(t, id); row["active"] != false || row["next_post_at"] != "" {
		t.Fatalf("precondition: active=%v next_post_at=%v; want false and empty", row["active"], row["next_post_at"])
	}

	// The admin edits something unrelated and saves, exactly as the form does:
	// the whole announcement goes back, once_local included.
	fields["details"] = "One night only (fixed typo)"
	resp := env.putJSON(t, fmt.Sprintf("/api/announcements/%d", id),
		map[string]any{"announcement": fields})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	row := env.announcementRow(t, id)
	if row["active"] == true {
		t.Error("editing a spent one-time announcement re-armed it; the scheduler will post it again")
	}
	if row["next_post_at"] != "" {
		t.Errorf("next_post_at = %v; want empty (cursor resurrected by an edit)", row["next_post_at"])
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
	// The cursor the row currently holds, so the guarded update matches.
	cur := env.announcementRow(t, id)["next_post_at"].(string)
	if _, err := env.store.MarkAnnouncementPosted(int64(id), cur, "", false); err != nil {
		t.Fatalf("MarkAnnouncementPosted: %v", err)
	}

	// Chicago is UTC-5/-6, so a week out in local wall-clock is unambiguously
	// in the future whatever the offset.
	fields["once_local"] = time.Now().Add(7 * 24 * time.Hour).Format("2006-01-02T15:04")
	resp := env.putJSON(t, fmt.Sprintf("/api/announcements/%d", id),
		map[string]any{"announcement": fields})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	row := env.announcementRow(t, id)
	if row["active"] != true {
		t.Error("moving a one-time announcement to a future date must re-arm it")
	}
	if row["next_post_at"] == "" {
		t.Error("next_post_at was not set when rescheduling to the future")
	}
}

// TestAnnouncementSkipZeroClears pins the contract the form promises. The handler
// folded an explicit 0 into "skip 1" so it could keep supporting an older client
// that sent no body at all - which meant the UI's "Set 0 to post as scheduled"
// did the exact opposite, arming a skip. An absent count still means 1.
func TestAnnouncementSkipZeroClears(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id, _ := env.scheduledAnnouncement(t, "Skippable")

	// Arm a skip, then clear it with an explicit 0.
	resp := env.postJSON(t, fmt.Sprintf("/api/announcements/%d/skip", id), map[string]any{"count": 2})
	resp.Body.Close()
	if got := env.skipCount(t, id); got != 2 {
		t.Fatalf("skip_count after count:2 = %d; want 2", got)
	}

	resp = env.postJSON(t, fmt.Sprintf("/api/announcements/%d/skip", id), map[string]any{"count": 0})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("count:0 status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	if got := env.skipCount(t, id); got != 0 {
		t.Errorf("skip_count after count:0 = %d; want 0 (the form says 0 means post as scheduled)", got)
	}

	// An omitted count still means "skip the next one" - the old no-body contract.
	resp = env.postJSON(t, fmt.Sprintf("/api/announcements/%d/skip", id), map[string]any{})
	resp.Body.Close()
	if got := env.skipCount(t, id); got != 1 {
		t.Errorf("skip_count with no count field = %d; want 1 (older clients sent no body)", got)
	}
}
