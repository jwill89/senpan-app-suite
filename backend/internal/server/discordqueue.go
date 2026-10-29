package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"app-suite/internal/discord"
	"app-suite/internal/model"
)

// -- Edit in place: tea rooms and affiliates ------------------------------------------
//
// A tea room or affiliate can keep ONE post in Discord that follows the item: with its
// destination in "edit" mode, posting again edits that message instead of adding another,
// and every later change to the item (a save, an open/closed toggle, a lock lifting) is
// queued and applied to it a few seconds later. A message someone deleted in Discord is
// marked gone and left alone until someone posts again - the app never reposts on its own.

// discordItems is one kind of postable item.
type discordItems struct {
	kind  string // discord_posts.kind
	dest  string // destination key
	where string // the system's name, for messages ("Tea Rooms")
	// resource is the admin resource key whose pages refetch after a background edit.
	resource string
}

var (
	teaRoomItems   = discordItems{kind: postKindTeaRoom, dest: destTeaRooms, where: "Tea Rooms", resource: "tea-rooms"}
	affiliateItems = discordItems{kind: postKindAffiliate, dest: destAffiliates, where: "Affiliates", resource: "affiliates"}
)

// discordItemsOf returns the item kind for a post kind.
func discordItemsOf(kind string) (discordItems, bool) {
	switch kind {
	case postKindTeaRoom:
		return teaRoomItems, true
	case postKindAffiliate:
		return affiliateItems, true
	}
	return discordItems{}, false
}

// discordItemMessage loads an item and builds its post. ok is false when it no longer
// exists.
func (s *Server) discordItemMessage(kind string, id int64) (msg discord.Message, name string, ok bool, err error) {
	switch kind {
	case postKindTeaRoom:
		room, err := s.store.GetTeaRoom(id)
		if err != nil || room == nil {
			return discord.Message{}, "", false, err
		}
		return discord.Message{Embeds: []discord.Embed{buildTeaRoomEmbed(*room)}}, room.Name, true, nil
	case postKindAffiliate:
		a, err := s.store.GetAffiliate(id)
		if err != nil || a == nil {
			return discord.Message{}, "", false, err
		}
		embed := buildAffiliateEmbed(*a, s.discordSetting(settingPublicBaseURL), time.Now())
		return discord.Message{Embeds: []discord.Embed{embed}}, a.Name, true, nil
	}
	return discord.Message{}, "", false, fmt.Errorf("unknown discord item kind %q", kind)
}

// discordItemState is where one item stands in Discord (nil = not posted).
func (s *Server) discordItemState(kind string, id int64) *model.DiscordPostState {
	return s.discordPostStates(kind)[id]
}

// postDiscordItem posts an item to its destination, honoring the repost mode:
//
//   - new: post, and leave earlier posts up;
//   - replace: post, then delete the earlier posts;
//   - edit: edit the item's post in place; post fresh (and clear the old ones) when there
//     is none in the destination channel, or it was deleted in Discord.
//
// The caller has checked the item exists.
func (s *Server) postDiscordItem(ctx context.Context, items discordItems, id int64) (model.DiscordPostResult, error) {
	unlock := s.discordItemLock(items.kind, id)
	defer unlock()

	msg, name, ok, err := s.discordItemMessage(items.kind, id)
	if err != nil {
		return model.DiscordPostResult{}, err
	}
	if !ok {
		return model.DiscordPostResult{}, fmt.Errorf("%s %d no longer exists", items.kind, id)
	}
	dest, err := s.store.GetDiscordDestination(items.dest)
	if err != nil {
		return model.DiscordPostResult{}, err
	}
	if dest.ChannelID == "" {
		return model.DiscordPostResult{}, errNoChannel
	}
	live, err := s.store.LiveDiscordPosts(items.kind, id)
	if err != nil {
		return model.DiscordPostResult{}, err
	}

	if dest.RepostMode == model.RepostEdit && len(live) > 0 {
		target := dest.ChannelID
		if p := s.discordPolicy(); p.redirect != "" {
			target = p.redirect
		}
		if newest := live[len(live)-1]; newest.ChannelID == target {
			_, err := s.discordEdit(ctx, newest, name, dest.Mention, msg, true)
			switch {
			case err == nil:
				note := s.clearOlderPosts(ctx, live[:len(live)-1], name)
				return model.DiscordPostResult{Post: s.discordItemState(items.kind, id), Note: note}, nil
			case errors.Is(err, errPostGone), errors.Is(err, errDiscordHeld):
				// Deleted in Discord (now marked gone), or a dry run: post fresh below.
				live = live[:len(live)-1]
			default:
				return model.DiscordPostResult{}, err
			}
		}
	}

	created, err := s.discordCreate(ctx, discordOut{
		Kind: items.kind, RefID: id, Name: name,
		ChannelID: dest.ChannelID, Mention: dest.Mention, Message: msg,
	})
	if err != nil {
		return model.DiscordPostResult{}, err
	}
	if created.Suppressed {
		return model.DiscordPostResult{Post: s.discordItemState(items.kind, id),
			Note: "This server is in dry-run mode, so nothing was sent to Discord."}, nil
	}
	note := ""
	if created.Redirected != "" {
		note = "Posted to the test channel, without pings (" + created.Redirected + ")."
	}
	if dest.RepostMode != model.RepostNew {
		note = joinNotes(note, s.clearOlderPosts(ctx, otherPosts(live, created.Post.MessageID), name))
	}
	return model.DiscordPostResult{Post: s.discordPostState(created.Post), Note: note}, nil
}

// clearOlderPosts deletes an item's earlier posts, returning a note when some stayed up.
func (s *Server) clearOlderPosts(ctx context.Context, posts []model.DiscordPost, name string) string {
	if len(posts) == 0 {
		return ""
	}
	_, held, err := s.discordDeleteAll(ctx, posts, name)
	switch {
	case err != nil:
		slog.Error("discord: remove earlier posts", "name", name, "error", err)
		return "The earlier post couldn't be removed: " + discordErrorText(err)
	case held > 0:
		return "The earlier post is outside the test channel, so it was left up."
	}
	return ""
}

// takeDownDiscordItem deletes an item's posts.
func (s *Server) takeDownDiscordItem(ctx context.Context, items discordItems, id int64, name string) (model.DiscordPostResult, error) {
	unlock := s.discordItemLock(items.kind, id)
	defer unlock()
	live, err := s.store.LiveDiscordPosts(items.kind, id)
	if err != nil {
		return model.DiscordPostResult{}, err
	}
	_, held, delErr := s.discordDeleteAll(ctx, live, name)
	if err := s.store.ForgetGoneDiscordPosts(items.kind, id); err != nil {
		slog.Error("forget discord posts", "kind", items.kind, "error", err)
	}
	if delErr != nil {
		return model.DiscordPostResult{}, delErr
	}
	note := ""
	if held > 0 {
		note = "A post outside the test channel was left up."
		if s.discordDryRun {
			note = "This server is in dry-run mode, so nothing was deleted in Discord."
		}
	}
	return model.DiscordPostResult{Post: s.discordItemState(items.kind, id), Note: note}, nil
}

// forgetDiscordItem takes down a deleted item's posts and forgets them. Best effort: the
// item is gone either way, and a message that couldn't be deleted is logged.
func (s *Server) forgetDiscordItem(ctx context.Context, items discordItems, id int64, name string) {
	if live, err := s.store.LiveDiscordPosts(items.kind, id); err == nil && len(live) > 0 && s.discord.Enabled() {
		unlock := s.discordItemLock(items.kind, id)
		_, held, err := s.discordDeleteAll(ctx, live, name)
		unlock()
		if err != nil || held > 0 {
			slog.Warn("a deleted item's Discord post was left up", "kind", items.kind, "name", name,
				"held", held, "error", err)
		}
	}
	if err := s.store.ForgetDiscordPosts(items.kind, id); err != nil {
		slog.Error("forget discord posts", "kind", items.kind, "error", err)
	}
}

func joinNotes(a, b string) string {
	return strings.TrimSpace(a + " " + b)
}

// -- The edit queue ---------------------------------------------------------------------

// discordEditDelay is how long a change waits before its edit is sent. Each new change to
// the same item pushes it back, so a burst of toggles makes one edit.
const discordEditDelay = 3 * time.Second

type discordRef struct {
	kind string
	id   int64
}

// discordEditQueue holds the items whose posts are due an edit, and when.
type discordEditQueue struct {
	mu   sync.Mutex
	due  map[discordRef]time.Time
	wake chan struct{}
}

// queueDiscordEdit asks for an item's post to be brought up to date shortly. Cheap and
// unconditional: whether the item has a post, and its destination edits in place, is
// decided when the edit runs.
func (s *Server) queueDiscordEdit(kind string, id int64) {
	q := &s.editQueue
	q.mu.Lock()
	if q.due == nil {
		q.due = map[discordRef]time.Time{}
	}
	q.due[discordRef{kind, id}] = time.Now().Add(discordEditDelay)
	wake := q.wake
	q.mu.Unlock()
	if wake != nil {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

// next reports when the earliest pending edit is due.
func (q *discordEditQueue) next() (time.Time, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var first time.Time
	for _, at := range q.due {
		if first.IsZero() || at.Before(first) {
			first = at
		}
	}
	return first, !first.IsZero()
}

// take removes and returns the edits due by now (every pending one when all is set).
func (q *discordEditQueue) take(now time.Time, all bool) []discordRef {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []discordRef
	for ref, at := range q.due {
		if all || !at.After(now) {
			out = append(out, ref)
			delete(q.due, ref)
		}
	}
	return out
}

// discordShutdownBudget bounds the edits sent while shutting down.
const discordShutdownBudget = 8 * time.Second

// RunDiscordWorker applies queued edits until ctx is cancelled, then sends whatever is
// still pending (within a short budget) so a change made just before a restart still
// reaches Discord. Safe to call in a goroutine.
func (s *Server) RunDiscordWorker(ctx context.Context) {
	q := &s.editQueue
	q.mu.Lock()
	if q.wake == nil {
		q.wake = make(chan struct{}, 1)
	}
	wake := q.wake
	q.mu.Unlock()
	slog.Info("scheduler started", "scheduler", "discord-edits")
	defer slog.Info("scheduler stopped", "scheduler", "discord-edits")

	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		if at, ok := q.next(); ok {
			timer.Reset(time.Until(at))
		}
		select {
		case <-ctx.Done():
			timer.Stop()
			drain, cancel := context.WithTimeout(context.Background(), discordShutdownBudget)
			s.flushDiscordEdits(drain, true)
			cancel()
			return
		case <-wake:
		case <-timer.C:
		}
		timer.Stop()
		s.flushDiscordEdits(context.Background(), false)
	}
}

// flushDiscordEdits applies the due edits.
func (s *Server) flushDiscordEdits(ctx context.Context, all bool) {
	for _, ref := range s.editQueue.take(time.Now(), all) {
		if ctx.Err() != nil {
			slog.Warn("discord edit dropped at shutdown", "kind", ref.kind, "id", ref.id)
			continue
		}
		func() {
			defer func() {
				if v := recover(); v != nil {
					slog.Error("panic in discord edit", "kind", ref.kind, "id", ref.id, "panic", v)
				}
			}()
			editCtx, cancel := context.WithTimeout(ctx, discordWriteTimeout)
			defer cancel()
			s.applyDiscordEdit(editCtx, ref.kind, ref.id)
		}()
	}
}

// applyDiscordEdit brings an item's post up to date, when its destination edits in place
// and it has a post the valves let the app touch.
func (s *Server) applyDiscordEdit(ctx context.Context, kind string, id int64) {
	items, ok := discordItemsOf(kind)
	if !ok || !s.discord.Enabled() {
		return
	}
	dest, err := s.store.GetDiscordDestination(items.dest)
	if err != nil || dest.RepostMode != model.RepostEdit {
		return
	}
	unlock := s.discordItemLock(kind, id)
	defer unlock()
	live, err := s.store.LiveDiscordPosts(kind, id)
	if err != nil || len(live) == 0 {
		return
	}
	msg, name, ok, err := s.discordItemMessage(kind, id)
	if err != nil || !ok {
		return
	}
	changed, err := s.discordEdit(ctx, live[len(live)-1], name, dest.Mention, msg, false)
	switch {
	case err == nil:
		if changed {
			s.broadcastResourceChanged(items.resource)
		}
	case errors.Is(err, errPostGone):
		s.broadcastResourceChanged(items.resource)
	case errors.Is(err, errDiscordHeld):
		slog.Debug("discord auto-update held back", "kind", kind, "name", name)
	default:
		slog.Error("discord auto-update failed", "kind", kind, "name", name, "error", err)
	}
}
