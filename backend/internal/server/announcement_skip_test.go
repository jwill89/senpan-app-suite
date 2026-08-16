package server_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// -- Skipping scheduled occurrences -------------------------------------------

// scheduledAnnouncement creates a weekly (Friday) announcement and returns its id
// plus the request body it was built from, so a test can edit and PUT it back the
// way the admin form does.
func (e *testEnv) scheduledAnnouncement(t *testing.T, title string) (int, map[string]any) {
	t.Helper()
	tr := e.postJSON(t, "/api/announcement-types", map[string]any{"name": title + " type", "webhook_url": ""})
	if tr.StatusCode != http.StatusCreated {
		t.Fatalf("create announcement type = %d; want 201", tr.StatusCode)
	}
	typeID := int(decodeBody(t, tr)["type"].(map[string]any)["id"].(float64))

	fields := map[string]any{
		"type_id":           typeID,
		"title":             title,
		"details":           "Tonight!",
		"schedule_kind":     "weekly",
		"timezone":          "America/Chicago",
		"schedule_weekdays": "5",
		"schedule_minutes":  1320,
		"active":            true,
	}
	cr := e.postJSON(t, "/api/announcements", map[string]any{"announcement": fields})
	if cr.StatusCode != http.StatusCreated {
		t.Fatalf("create announcement = %d; want 201", cr.StatusCode)
	}
	a := decodeBody(t, cr)["announcement"].(map[string]any)
	if got := a["skip_count"]; got != float64(0) {
		t.Fatalf("new announcement skip_count = %v; want 0", got)
	}
	return int(a["id"].(float64)), fields
}

/** skipCount reads an announcement's current skip_count from the list endpoint. */
func (e *testEnv) skipCount(t *testing.T, id int) int {
	t.Helper()
	resp := e.get(t, "/api/announcements")
	for _, row := range decodeBody(t, resp)["announcements"].([]any) {
		m := row.(map[string]any)
		if int(m["id"].(float64)) == id {
			return int(m["skip_count"].(float64))
		}
	}
	t.Fatalf("announcement %d not found in the list", id)
	return 0
}

func TestAnnouncementSkip_SetsACount(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id, _ := env.scheduledAnnouncement(t, "Bingo Night")

	// An empty body means "skip the next one" - what the endpoint did before it
	// took a count at all, so an older client keeps working.
	resp := env.postJSON(t, fmt.Sprintf("/api/announcements/%d/skip", id), map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("skip = %d; want 200", resp.StatusCode)
	}
	if got := env.skipCount(t, id); got != 1 {
		t.Errorf("skip_count after an empty body = %d; want 1", got)
	}

	// A count skips that many upcoming occurrences - the Friday+Saturday case.
	resp = env.postJSON(t, fmt.Sprintf("/api/announcements/%d/skip", id), map[string]any{"count": 2})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("skip 2 = %d; want 200", resp.StatusCode)
	}
	if got := env.skipCount(t, id); got != 2 {
		t.Errorf("skip_count = %d; want 2", got)
	}

	// 0 clears it, so a mis-click is undoable without editing the announcement.
	resp = env.postJSON(t, fmt.Sprintf("/api/announcements/%d/skip", id), map[string]any{"count": -1})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear = %d; want 200", resp.StatusCode)
	}
	if got := env.skipCount(t, id); got != 0 {
		t.Errorf("skip_count after clearing = %d; want 0", got)
	}

	// Absurd counts are refused rather than silently stored.
	resp = env.postJSON(t, fmt.Sprintf("/api/announcements/%d/skip", id), map[string]any{"count": 500})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("skip 500 = %d; want 400", resp.StatusCode)
	}
}

// TestAnnouncementSkip_SurvivesAnEdit is the regression for the bug this replaced:
// saving ANY edit used to clear a pending skip, because the write path reset the
// flag and the update statement wrote it. The skip belongs to the schedule cursor,
// not the form, so an edit must leave it alone.
func TestAnnouncementSkip_SurvivesAnEdit(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id, fields := env.scheduledAnnouncement(t, "Bingo Night")

	resp := env.postJSON(t, fmt.Sprintf("/api/announcements/%d/skip", id), map[string]any{"count": 2})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("skip = %d; want 200", resp.StatusCode)
	}

	// Edit something unrelated and save, exactly as the admin form does.
	fields["details"] = "Tonight, with prizes!"
	ur := env.putJSON(t, fmt.Sprintf("/api/announcements/%d", id), map[string]any{"announcement": fields})
	if ur.StatusCode != http.StatusOK {
		t.Fatalf("update = %d; want 200", ur.StatusCode)
	}
	if got := env.skipCount(t, id); got != 2 {
		t.Errorf("skip_count after an unrelated edit = %d; want 2 (the edit must not clear it)", got)
	}

	// A save can't set one either - the endpoint owns that.
	fields["skip_count"] = 9
	ur = env.putJSON(t, fmt.Sprintf("/api/announcements/%d", id), map[string]any{"announcement": fields})
	if ur.StatusCode != http.StatusOK {
		t.Fatalf("update = %d; want 200", ur.StatusCode)
	}
	if got := env.skipCount(t, id); got != 2 {
		t.Errorf("a form save changed skip_count to %d; want it untouched at 2", got)
	}
}

func TestAnnouncementSkip_RejectsUnscheduled(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	tr := env.postJSON(t, "/api/announcement-types", map[string]any{"name": "Manual", "webhook_url": ""})
	typeID := int(decodeBody(t, tr)["type"].(map[string]any)["id"].(float64))
	cr := env.postJSON(t, "/api/announcements", map[string]any{"announcement": map[string]any{
		"type_id": typeID, "title": "Manual only", "details": "x",
	}})
	if cr.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d; want 201", cr.StatusCode)
	}
	id := int(decodeBody(t, cr)["announcement"].(map[string]any)["id"].(float64))

	// Nothing to skip on an announcement that has no schedule.
	resp := env.postJSON(t, fmt.Sprintf("/api/announcements/%d/skip", id), map[string]any{"count": 1})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("skip on a manual announcement = %d; want 400", resp.StatusCode)
	}
}

// TestAnnouncementSkip_DecrementsPerOccurrence is the point of the count: a
// Friday+Saturday announcement told to skip 2 sits out BOTH, one occurrence at a
// time, instead of skipping once and resuming. Driven through the real scheduler,
// whose startup sweep consumes whatever is due.
func TestAnnouncementSkip_DecrementsPerOccurrence(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id, _ := env.scheduledAnnouncement(t, "Bingo Night")

	// Two skips pending, and an occurrence already due.
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := env.store.AdvanceAnnouncement(int64(id), past, true, 2); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}

	// One sweep should consume exactly one of them.
	env.sweepAnnouncements(t)
	if got := env.skipCount(t, id); got != 1 {
		t.Fatalf("after one due occurrence, skip_count = %d; want 1 (one consumed, one left)", got)
	}

	// The cursor moved forward, so make the NEXT occurrence due and sweep again.
	if err := env.store.AdvanceAnnouncement(int64(id), past, true, 1); err != nil {
		t.Fatalf("re-seed cursor: %v", err)
	}
	env.sweepAnnouncements(t)
	if got := env.skipCount(t, id); got != 0 {
		t.Errorf("after the second occurrence, skip_count = %d; want 0 (both consumed)", got)
	}
}

// sweepAnnouncements runs the announcement scheduler just long enough for its
// immediate startup sweep, then stops it.
func (e *testEnv) sweepAnnouncements(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.srv.RunAnnouncementScheduler(ctx)
	}()
	// The sweep runs before the first tick; give it a moment, then stop.
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("announcement scheduler did not stop")
	}
}
