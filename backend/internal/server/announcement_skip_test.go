package server_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"app-suite/internal/db"
)

// -- Skipping scheduled occurrences -------------------------------------------

// scheduledAnnouncement creates a weekly (Friday) announcement and returns its id
// plus the body it was built from, so a test can edit and PUT it back the way the
// admin form does.
func (e *testEnv) scheduledAnnouncement(t *testing.T, title string) (int64, map[string]any) {
	t.Helper()
	typeID := e.makeAnnouncementType(t, title+" type")
	body := announcementBody(typeID, map[string]any{
		"title": title, "details": "Tonight!", "scheduleKind": "weekly", "timezone": "America/Chicago",
		"scheduleWeekdays": "5", "scheduleMinutes": 1320,
	})
	var made map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/announcements", body), http.StatusCreated, &made)
	if got := made["skipCount"]; got != float64(0) {
		t.Fatalf("new announcement skipCount = %v; want 0", got)
	}
	return int64(made["id"].(float64)), body
}

// skipCount reads an announcement's current skipCount from the list endpoint.
func (e *testEnv) skipCount(t *testing.T, id int64) int {
	t.Helper()
	return int(e.announcementRow(t, id)["skipCount"].(float64))
}

func TestAnnouncementSkip_SetsACount(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id, _ := env.scheduledAnnouncement(t, "Bingo Night")
	path := fmt.Sprintf("/api/v1/announcements/%d/skip", id)

	// An empty body means "skip the next one".
	var skipped map[string]any
	decodeInto(t, env.postJSON(t, path, map[string]any{}), http.StatusOK, &skipped)
	if skipped["skipCount"] != float64(1) {
		t.Errorf("skipCount after an empty body = %v; want 1", skipped["skipCount"])
	}

	// A count skips that many upcoming occurrences - the Friday+Saturday case.
	decodeInto(t, env.postJSON(t, path, map[string]any{"count": 2}), http.StatusOK, nil)
	if got := env.skipCount(t, id); got != 2 {
		t.Errorf("skipCount = %d; want 2", got)
	}

	// 0 clears it, so a mis-click is undoable without editing the announcement.
	decodeInto(t, env.postJSON(t, path, map[string]any{"count": 0}), http.StatusOK, nil)
	if got := env.skipCount(t, id); got != 0 {
		t.Errorf("skipCount after clearing = %d; want 0 (0 means post as scheduled)", got)
	}

	// Absurd or negative counts are refused rather than silently stored.
	for _, count := range []int{500, -1} {
		if at := invalid(t, env.postJSON(t, path, map[string]any{"count": count})); !slices.Contains(at, "body.count") {
			t.Errorf("skip %d: errors at %v; want body.count", count, at)
		}
	}
	if got := status(env.postJSON(t, "/api/v1/announcements/999999/skip", map[string]any{})); got != http.StatusNotFound {
		t.Errorf("an unknown announcement: %d, want 404", got)
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

	decodeInto(t, env.postJSON(t, fmt.Sprintf("/api/v1/announcements/%d/skip", id), map[string]any{"count": 2}), http.StatusOK, nil)

	// Edit something unrelated and save, exactly as the admin form does.
	fields["details"] = "Tonight, with prizes!"
	decodeInto(t, env.putJSON(t, fmt.Sprintf("/api/v1/announcements/%d", id), fields), http.StatusOK, nil)
	if got := env.skipCount(t, id); got != 2 {
		t.Errorf("skipCount after an unrelated edit = %d; want 2 (the edit must not clear it)", got)
	}

	// A save can't set one either - the skip operation owns that, and the field is not
	// in the schema, so a client that sends it is refused.
	fields["skipCount"] = 9
	if at := invalid(t, env.putJSON(t, fmt.Sprintf("/api/v1/announcements/%d", id), fields)); !slices.Contains(at, "body.skipCount") {
		t.Errorf("a save carrying skipCount: errors at %v; want body.skipCount", at)
	}
	if got := env.skipCount(t, id); got != 2 {
		t.Errorf("a form save changed skipCount to %d; want it untouched at 2", got)
	}
}

func TestAnnouncementSkip_RejectsUnscheduled(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	a := env.makeAnnouncement(t, env.makeAnnouncementType(t, "Manual"), map[string]any{"title": "Manual only"})

	// Nothing to skip on an announcement that has no schedule.
	if got := status(env.postJSON(t, announcementPath(a)+"/skip", map[string]any{"count": 1})); got != http.StatusConflict {
		t.Errorf("skip on a manual announcement = %d; want 409", got)
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
	env.seedCursor(t, id, past, 2)

	// One sweep should consume exactly one of them.
	env.sweepAnnouncements(t)
	if got := env.skipCount(t, id); got != 1 {
		t.Fatalf("after one due occurrence, skipCount = %d; want 1 (one consumed, one left)", got)
	}

	// The cursor moved forward, so make the NEXT occurrence due and sweep again.
	env.seedCursor(t, id, past, 1)
	env.sweepAnnouncements(t)
	if got := env.skipCount(t, id); got != 0 {
		t.Errorf("after the second occurrence, skipCount = %d; want 0 (both consumed)", got)
	}
}

// seedCursor sets an announcement's next post and pending skips, as a schedule left
// to run would.
func (e *testEnv) seedCursor(t *testing.T, id int64, next string, skips int64) {
	t.Helper()
	cur := e.announcementRow(t, id)["nextPostAt"].(string)
	n, err := db.New(e.store.DB()).AdvanceAnnouncement(t.Context(), db.AdvanceAnnouncementParams{
		NextPostAt: &next, Active: 1, SkipCount: skips, ID: id, PrevNextPostAt: &cur,
	})
	if err != nil || n != 1 {
		t.Fatalf("seed cursor = %d, %v", n, err)
	}
}

// A sweep that changed something tells the staff with the Announcements page, once;
// one that found nothing due tells nobody.
func TestAnnouncementSchedulerTellsItsPage(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id, _ := env.scheduledAnnouncement(t, "Bingo Night")
	s := staffStream(t, env, env.client)

	env.sweepAnnouncements(t)
	s.expectNone("resource_changed", 300*time.Millisecond)

	env.seedCursor(t, id, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), 1)
	// The seed itself is a database write, not an API one, so nothing is heard yet.
	env.sweepAnnouncements(t)
	if e := s.expect("resource_changed", 3*time.Second); e.str("resource") != "announcements" {
		t.Errorf("resource_changed: %+v; want announcements", e)
	}
	s.expectNone("resource_changed", 300*time.Millisecond)
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
