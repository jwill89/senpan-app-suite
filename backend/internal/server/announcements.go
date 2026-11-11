package server

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"app-suite/internal/api"
	"app-suite/internal/db"
	"app-suite/internal/discord"
	"app-suite/internal/schedule"
)

// announcementSchedulerInterval is how often the background scheduler checks for
// due announcement posts. Announcements post within one interval of their time.
const announcementSchedulerInterval = 30 * time.Second

// maxAnnouncementPostRetries is how many consecutive failed posts one occurrence
// gets before the scheduler moves its cursor on. At the 30-second sweep interval
// this is a couple of minutes of retrying, which covers a Discord blip while not
// letting a deleted webhook wedge the sweep indefinitely.
const maxAnnouncementPostRetries = 5

// RunAnnouncementScheduler posts due announcements to their type's webhook on a
// fixed interval until ctx is cancelled. Safe to call in a goroutine.
func (s *Server) RunAnnouncementScheduler(ctx context.Context) {
	runScheduler(ctx, "announcements", announcementSchedulerInterval, func() { s.postDueAnnouncements(ctx) })
}

// recordPostFailure counts one consecutive failure for an announcement and returns
// the new count.
func (s *Server) recordPostFailure(id int64) int {
	s.postFailMu.Lock()
	defer s.postFailMu.Unlock()
	// Lazily created: tests construct a Server directly rather than through New.
	if s.postFails == nil {
		s.postFails = make(map[int64]int)
	}
	s.postFails[id]++
	return s.postFails[id]
}

// clearPostFailures forgets an announcement's failure streak - on a successful
// post, or once its occurrence has been given up on.
func (s *Server) clearPostFailures(id int64) {
	s.postFailMu.Lock()
	defer s.postFailMu.Unlock()
	delete(s.postFails, id)
}

// nextPostColumn is a schedule cursor as its column holds it: nil for none.
func nextPostColumn(next string) *string {
	if next == "" {
		return nil
	}
	return &next
}

// advanceCursor is the schedule cursor for a after its next post has fired.
func advanceCursor(a *db.Announcement) (nextPostAt string, live bool) {
	due := ""
	if a.NextPostAt != nil {
		due = *a.NextPostAt
	}
	return schedule.Advance(api.AnnouncementRule(a), due, time.Now())
}

// postDueAnnouncements posts every announcement whose scheduled time has arrived.
// A pending skip passes over an occurrence without posting; an announcement whose
// type has no webhook is left pending; a failed post is retried next tick. Posts
// run sequentially; ctx is checked between them so a graceful shutdown isn't
// delayed by a long backlog of outbound Discord calls. When the sweep changed
// anything, the staff's Announcements pages are told once, at the end.
func (s *Server) postDueAnnouncements(ctx context.Context) {
	read, write := db.New(s.store.ReadDB()), db.New(s.store.DB())
	wrote := false
	defer func() {
		if wrote {
			api.ResourceChanged(s.live, "announcements")
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339)
	due, err := read.DueAnnouncements(ctx, &now)
	if err != nil {
		slog.Log(ctx, errorLevel(err), "announcement scheduler: load due", "error", err)
		return
	}
	if len(due) > 0 {
		slog.Debug("announcement scheduler: due", "count", len(due))
	}
	for _, a := range due {
		if ctx.Err() != nil {
			// Shutting down - stop issuing outbound posts; the rest stay due and are
			// picked up on the next startup sweep.
			return
		}
		if a.SkipCount > 0 {
			// Consume ONE occurrence per due tick: an announcement told to skip 2 sits
			// out this occurrence and the next, rather than the whole run at once.
			remaining := a.SkipCount - 1
			next, live := advanceCursor(a)
			applied, err := advanceRow(ctx, write, a, next, live, remaining)
			if err != nil {
				slog.Error("announcement scheduler: consume skip", "id", a.ID, "error", err)
				continue
			}
			if !applied {
				// An admin edited the schedule while this sweep was running; their
				// cursor wins over one computed from a snapshot taken before it.
				slog.Info("announcement changed during the sweep; leaving the admin's cursor alone",
					"id", a.ID, "title", a.Title)
				continue
			}
			wrote = true
			// Worth a line: from the outside a consumed skip looks exactly like an
			// announcement that failed to post, since neither puts anything in Discord.
			slog.Info("announcement occurrence skipped", "id", a.ID, "title", a.Title,
				"next", next, "still_active", live, "skips_remaining", remaining)
			continue
		}
		typ, err := read.GetAnnouncementType(ctx, a.TypeID)
		if err != nil || strings.TrimSpace(typ.WebhookUrl) == "" {
			continue // no webhook yet; try again next tick
		}
		target := discord.Target{Kind: "announcement_scheduled", Name: a.Title}
		err = s.discord.Post(ctx, target, typ.WebhookUrl, api.AnnouncementPayload(ctx, read, a, time.Now()))
		if err != nil && !discord.IsAmbiguous(err) {
			// Definitely not delivered (HTTP error status, incl. 429 rate limit):
			// leave the cursor where it is so the next tick retries - but only up to a
			// point. A webhook that has been deleted answers 404 every time, and
			// retrying that every 30 seconds forever both spams the log and keeps a
			// stuck announcement permanently "due", so nothing after it in the sweep
			// ever gets a clean run. After maxAnnouncementPostRetries consecutive
			// failures, give up on THIS occurrence and move the cursor on; a recurring
			// announcement simply tries again at its next occurrence.
			n := s.recordPostFailure(a.ID)
			slog.Error("announcement scheduler: post", "id", a.ID, "error", err, "consecutive_failures", n)
			if n < maxAnnouncementPostRetries {
				continue
			}
			next, live := advanceCursor(a)
			applied, err := advanceRow(ctx, write, a, next, live, a.SkipCount)
			if err != nil || !applied {
				slog.Error("announcement scheduler: give up on occurrence",
					"id", a.ID, "error", err, "applied", applied)
				continue
			}
			wrote = true
			slog.Error("announcement scheduler: giving up on this occurrence after repeated failures",
				"id", a.ID, "title", a.Title, "failures", n, "next", next, "still_active", live)
			s.clearPostFailures(a.ID)
			continue
		}
		s.clearPostFailures(a.ID)
		// Success OR an ambiguous transport failure: advance the cursor either way.
		// On ambiguity the message may already be on Discord, so retrying would
		// duplicate it - we advance instead and log a warning. Recurring posts
		// resume at their next occurrence; a one-time post that truly failed needs
		// a manual resend.
		next, live := advanceCursor(a)
		// Detached: the post has gone out, so the cursor must move even when
		// shutdown canceled ctx during it - or the next boot posts it again.
		applied, mErr := s.markPostedWithRetry(context.WithoutCancel(ctx), write, a, next, live)
		switch {
		case mErr != nil:
			// The Discord post already went out, but the cursor could not be advanced
			// even after retrying - the announcement is still "due", so the next tick
			// would re-post it (a duplicate @everyone blast). Log loudly; a persistent
			// failure here means the SQLite store itself is unhealthy.
			slog.Error("announcement scheduler: mark posted failed after retries (possible duplicate next tick)",
				"id", a.ID, "title", a.Title, "error", mErr)
		case err != nil:
			wrote = wrote || applied
			slog.Warn("announcement post ambiguous; advanced cursor to avoid a duplicate",
				"id", a.ID, "title", a.Title, "error", err)
		default:
			wrote = wrote || applied
			slog.Info("posted scheduled announcement", "id", a.ID, "title", a.Title)
		}
	}
}

// advanceRow moves a's schedule cursor and the skips it has left without stamping a
// post. It applies only while the row still holds the cursor a was read with, and
// reports whether it did.
func advanceRow(ctx context.Context, q *db.Queries, a *db.Announcement, next string, live bool, skips int64) (bool, error) {
	n, err := q.AdvanceAnnouncement(ctx, db.AdvanceAnnouncementParams{
		NextPostAt: nextPostColumn(next), Active: boolInt(live), SkipCount: skips, ID: a.ID, PrevNextPostAt: a.NextPostAt,
	})
	return n > 0, err
}

// markPostedWithRetry stamps a post and advances the schedule cursor in one atomic
// update, so the two move together and a retry is idempotent. Because the Discord
// post has ALREADY succeeded by the time this runs, a failed cursor write would
// leave the announcement still due and re-post it next tick - a duplicate @everyone
// blast - so a transient DB error (e.g. brief SQLite-WAL contention) is retried a
// few times before giving up.
//
// The write applies only while the row still holds the cursor read before posting.
// One that doesn't match means an admin edited the schedule during the post:
// their cursor stands, and this reports false and no error, because retrying would
// only overwrite the edit again. The message is already on Discord either way.
func (s *Server) markPostedWithRetry(ctx context.Context, q *db.Queries, a *db.Announcement, next string, live bool) (bool, error) {
	var err error
	for range 3 {
		var n int64
		n, err = q.MarkAnnouncementPosted(ctx, db.MarkAnnouncementPostedParams{
			NextPostAt: nextPostColumn(next), Active: boolInt(live), ID: a.ID, PrevNextPostAt: a.NextPostAt,
		})
		if err == nil {
			if n == 0 {
				slog.Info("announcement changed during its post; leaving the admin's cursor alone", "id", a.ID)
			}
			return n > 0, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false, err
}

// boolInt is a bool as SQLite's 0 or 1.
func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
