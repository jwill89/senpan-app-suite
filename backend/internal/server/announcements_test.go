package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"app-suite/internal/db"
	"app-suite/internal/discord"
	"app-suite/internal/store"
	"app-suite/internal/store/storetest"
)

// mustParse parses a UTC RFC 3339 instant or fails the test.
func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v.UTC()
}

// announcementRow reads one announcement as stored.
func announcementRow(t *testing.T, s *Server, id int64) *db.Announcement {
	t.Helper()
	a, err := db.New(s.store.ReadDB()).GetAnnouncement(t.Context(), id)
	if err != nil {
		t.Fatalf("GetAnnouncement: %v", err)
	}
	return a
}

// cursorOf is an announcement's stored next post, "" for none.
func cursorOf(a *db.Announcement) string {
	if a.NextPostAt == nil {
		return ""
	}
	return *a.NextPostAt
}

// newSchedulerEnv builds a minimal Server backed by a temp store plus a daily
// announcement that is already overdue (cursor in the past), for scheduler tests.
// Returns the server, the announcement id, and its starting next_post_at.
func newSchedulerEnv(t *testing.T, webhook string) (*Server, int64, string) {
	t.Helper()
	st, err := store.New(t.Context(), storetest.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	q := db.New(st.DB())
	typ, err := q.CreateAnnouncementType(t.Context(), db.CreateAnnouncementTypeParams{Name: "T", WebhookUrl: webhook})
	if err != nil {
		t.Fatal(err)
	}
	startCursor := time.Now().UTC().AddDate(0, 0, -3).Format(time.RFC3339) // 3 days overdue
	a, err := q.CreateAnnouncement(t.Context(), db.CreateAnnouncementParams{
		TypeID: typ.ID, Title: "Daily", Details: "d", Buttons: "[]",
		ScheduleKind: "daily", Timezone: "UTC", ScheduleMinutes: 9 * 60,
		NextPostAt: &startCursor, Active: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Server{store: st, discord: &discord.Sender{}}, a.ID, startCursor
}

// TestPostDueAnnouncementsNoDoubleWhenOverdue is the regression test for the
// reported double-posting: an overdue recurring announcement must post exactly
// once and advance its cursor to a future slot, so a second sweep posts nothing.
func TestPostDueAnnouncementsNoDoubleWhenOverdue(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	s, id, _ := newSchedulerEnv(t, ts.URL)

	s.postDueAnnouncements(context.Background())
	s.postDueAnnouncements(context.Background()) // back-to-back: a correct scheduler still posts once

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("posted %d times across two sweeps; want exactly 1", got)
	}
	reloaded := announcementRow(t, s, id)
	if !mustParse(t, cursorOf(reloaded)).After(time.Now()) {
		t.Errorf("cursor not advanced to a future slot: %+v", reloaded)
	}
	if reloaded.LastPostedAt == nil {
		t.Error("the post was not stamped")
	}
}

// TestPostDueAnnouncementsAmbiguousAdvances verifies a transport failure (the
// webhook may have been delivered) advances the cursor instead of retrying, so a
// possibly-delivered post is never duplicated on the next tick.
func TestPostDueAnnouncementsAmbiguousAdvances(t *testing.T) {
	// The connection drops after the request is read: it reached the server, which
	// may have posted, so the failure is ambiguous.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer ts.Close()

	s, id, startCursor := newSchedulerEnv(t, ts.URL)
	s.postDueAnnouncements(context.Background())

	reloaded := announcementRow(t, s, id)
	if cursorOf(reloaded) == startCursor {
		t.Errorf("ambiguous failure should advance the cursor (no retry), got %q", cursorOf(reloaded))
	}
	if !mustParse(t, cursorOf(reloaded)).After(time.Now()) {
		t.Errorf("advanced cursor should be in the future, got %q", cursorOf(reloaded))
	}
}

// TestPostDueAnnouncementsHTTPErrorRetries verifies a definite non-delivery (an
// HTTP error status, e.g. a 429 rate limit or 5xx) leaves the cursor untouched so
// the next tick retries - preserving delivery without duplicating.
func TestPostDueAnnouncementsHTTPErrorRetries(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	s, id, startCursor := newSchedulerEnv(t, ts.URL)
	s.postDueAnnouncements(context.Background())

	if got := cursorOf(announcementRow(t, s, id)); got != startCursor {
		t.Errorf("HTTP error should leave the cursor pending for retry; got %q want %q", got, startCursor)
	}
}

// A type with no webhook yet leaves its announcements pending, untouched.
func TestPostDueAnnouncementsWithoutAWebhookStaysPending(t *testing.T) {
	s, id, startCursor := newSchedulerEnv(t, "")
	s.postDueAnnouncements(context.Background())
	if got := cursorOf(announcementRow(t, s, id)); got != startCursor {
		t.Errorf("cursor = %q; want it left at %q", got, startCursor)
	}
}

// TestPostDueAnnouncementsGivesUpAfterRepeatedFailures pins the retry bound. A
// definite non-delivery leaves the cursor pending so the next tick retries, which
// is right for a blip - but a webhook that has been DELETED answers 404 every
// time, and retrying that every 30 seconds forever both floods the log and keeps a
// stuck announcement permanently due, so it is reconsidered ahead of everything
// else on every sweep. After a handful of consecutive failures the occurrence is
// given up on and the cursor moves.
func TestPostDueAnnouncementsGivesUpAfterRepeatedFailures(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound) // a deleted webhook, permanently
	}))
	defer ts.Close()

	s, id, startCursor := newSchedulerEnv(t, ts.URL)

	// Every sweep up to the limit must leave the cursor alone, so a transient
	// outage is still ridden out rather than skipped.
	for i := 1; i < maxAnnouncementPostRetries; i++ {
		s.postDueAnnouncements(context.Background())
		if cursorOf(announcementRow(t, s, id)) != startCursor {
			t.Fatalf("sweep %d advanced the cursor early; a short outage must still be retried", i)
		}
	}

	// The one that crosses the limit gives up on this occurrence and moves on.
	s.postDueAnnouncements(context.Background())
	reloaded := announcementRow(t, s, id)
	if cursorOf(reloaded) == startCursor {
		t.Error("a permanently failing webhook is still being retried every sweep; the cursor never moved")
	}
	if reloaded.Active != 1 {
		t.Error("a recurring announcement should stay active and try again at its next occurrence")
	}
}

// TestPostDueAnnouncementsRespectsAConcurrentEdit pins the snapshot guard on the
// cursor write. The sweep reads an announcement, posts it (up to 15 seconds of
// HTTP), and then advances the cursor from the values it read at the start - keyed
// on id alone, so an admin edit that landed during the post was silently reverted.
// The write now only applies while the row still holds the cursor the sweep read.
func TestPostDueAnnouncementsRespectsAConcurrentEdit(t *testing.T) {
	var s *Server
	var id int64
	edited := make(chan struct{})

	// Rescheduling mid-post is what a real admin edit does: it moves next_post_at.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		moved := "2099-01-01T00:00:00Z"
		cur := announcementRow(t, s, id).NextPostAt
		if _, err := db.New(s.store.DB()).AdvanceAnnouncement(t.Context(), db.AdvanceAnnouncementParams{
			NextPostAt: &moved, Active: 1, ID: id, PrevNextPostAt: cur,
		}); err != nil {
			t.Errorf("simulated admin edit: %v", err)
		}
		close(edited)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	s, id, _ = newSchedulerEnv(t, ts.URL)
	s.postDueAnnouncements(context.Background())
	<-edited

	if got := cursorOf(announcementRow(t, s, id)); got != "2099-01-01T00:00:00Z" {
		t.Errorf("cursor = %q; want the admin's edit preserved, not overwritten from the pre-post snapshot", got)
	}
}
