package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"app-suite/internal/discord"
	"app-suite/internal/model"
)

// -- The posting funnel ----------------------------------------------------------
//
// Every message the app puts in Discord goes through discordCreate, and every later change
// to one through discordEdit or discordDelete. That is where the safety valves live, so no
// feature can forget them:
//
//   - dry run (-discord-dry-run) suppresses every write and logs it;
//   - the redirect channel (-discord-redirect-channel) and test mode (Discord page) send
//     every new post to one channel instead, with its pings switched off, and let edits and
//     deletes touch only messages in that channel.
//
// Every create is recorded in discord_posts so it can be edited or deleted later. A message
// the app deletes itself is forgotten; one Discord reports missing is kept, marked gone, so
// the page can say "deleted in Discord".

// Discord post kinds (discord_posts.kind).
const (
	postKindAnnouncement = "announcement"
	postKindReadingList  = "reading_list"
	postKindTeaRoom      = "tea_room"
	postKindAffiliate    = "affiliate"
)

// Destination keys (discord_destinations.key). Book clubs use "bookclub:<slug>".
const (
	destTeaRooms   = "tea-rooms"
	destAffiliates = "affiliates"
)

// bookClubDestination is a club's destination key.
func bookClubDestination(slug string) string { return "bookclub:" + slug }

var (
	// errNoChannel: the system has no channel picked yet.
	errNoChannel = errors.New("no Discord channel picked")
	// errDiscordHeld: the valves don't allow touching this message (dry run, or it isn't
	// in the test/redirect channel). Nothing was sent.
	errDiscordHeld = errors.New("held back by the Discord safety settings")
	// errPostGone: the message was deleted in Discord. It has been marked gone.
	errPostGone = errors.New("the message was deleted in Discord")
)

// discordPolicy is what the dev valves and test mode allow right now.
type discordPolicy struct {
	dryRun bool
	// redirect is the channel every new post goes to instead ("" = none), and the only
	// channel whose messages may be edited or deleted.
	redirect string
	// why names the redirect for logs and notes: "test mode" or "redirect".
	why string
}

func (s *Server) discordPolicy() discordPolicy {
	switch {
	case s.discordDryRun:
		return discordPolicy{dryRun: true}
	case s.discordRedirect != "":
		return discordPolicy{redirect: s.discordRedirect, why: "redirect"}
	case s.discordSetting(settingDiscordTestMode) == "1":
		return discordPolicy{redirect: s.discordSetting(settingDiscordTestChannel), why: "test mode"}
	}
	return discordPolicy{}
}

// mayTouch reports whether an existing message in channelID may be edited or deleted.
func (p discordPolicy) mayTouch(channelID string) bool {
	return !p.dryRun && (p.redirect == "" || channelID == p.redirect)
}

// discordMention turns a ping choice into the message text and the list of who it may
// notify. Mentions inside an embed never notify, so the ping travels in the content. quiet
// keeps the text but notifies no one - a post landing in a test channel must not ping.
//
//	""                -> no text, no ping
//	"everyone"        -> "@everyone"
//	"role:<role id>"  -> "<@&id>", that role only
func discordMention(mention string, quiet bool) (string, discord.AllowedMentions) {
	allowed := discord.Nobody()
	mention = strings.TrimSpace(mention)
	switch {
	case mention == "everyone":
		if !quiet {
			allowed.Parse = []string{"everyone"}
		}
		return "@everyone", allowed
	case strings.HasPrefix(mention, "role:") && isDiscordSnowflake(strings.TrimPrefix(mention, "role:")):
		id := strings.TrimPrefix(mention, "role:")
		if !quiet {
			allowed.Roles = []string{id}
		}
		return "<@&" + id + ">", allowed
	}
	return "", allowed
}

// validMention reports whether a stored ping choice is one discordMention understands.
func validMention(mention string) bool {
	return mention == "" || mention == "everyone" ||
		(strings.HasPrefix(mention, "role:") && isDiscordSnowflake(strings.TrimPrefix(mention, "role:")))
}

// discordFingerprint hashes what a message says, so an edit that would change nothing is
// skipped.
func discordFingerprint(m discord.Message) string {
	b, _ := json.Marshal(struct {
		C string
		E []discord.Embed
		K []discord.Component
		A discord.AllowedMentions
	}{m.Content, m.Embeds, m.Components, m.AllowedMentions})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:16])
}

// discordItemLock serializes posting for one item, so a double-click can't post twice.
func (s *Server) discordItemLock(kind string, refID int64) func() {
	v, _ := s.discordLocks.LoadOrStore(kind+":"+strconv.FormatInt(refID, 10), &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// discordOut is one message to create.
type discordOut struct {
	Kind  string
	RefID int64
	Scope string
	Batch string
	Seq   int
	// Name is the record's human name, for the log.
	Name string
	// ChannelID is the destination's channel; the valves may send it elsewhere.
	ChannelID string
	Mention   string
	// Message carries the embeds and buttons; its content and pings come from Mention.
	Message discord.Message
	// Nonce makes a retry of the same post safe: Discord answers a repeat within a few
	// minutes with the message it already made. Only for a post that is retried after a
	// failure that may have landed - a scheduled occurrence. A manual post has none: a
	// nonce reused after its message was deleted in Discord would get that deleted
	// message back. At most 25 characters.
	Nonce string
	// Patient waits out longer rate limits, as one of a batch has to.
	Patient bool
}

// discordCreated is what became of a create.
type discordCreated struct {
	Post model.DiscordPost
	// Suppressed: a dry run, nothing was sent.
	Suppressed bool
	// Redirected names why the post went to the test/redirect channel ("" = it didn't).
	Redirected string
	// Repeat: Discord answered the nonce with a message this item already has (a retry
	// of a post that had landed), so nothing new was posted.
	Repeat bool
}

// discordCreate posts a message and records it.
func (s *Server) discordCreate(ctx context.Context, out discordOut) (discordCreated, error) {
	if out.ChannelID == "" {
		return discordCreated{}, errNoChannel
	}
	c := s.discord.Client()
	if c == nil {
		return discordCreated{}, discord.ErrOff
	}
	p := s.discordPolicy()
	if p.dryRun {
		slog.Warn("discord post SUPPRESSED (dry run)", "kind", out.Kind, "name", out.Name)
		return discordCreated{Suppressed: true}, nil
	}
	channel := out.ChannelID
	if p.redirect != "" {
		channel = p.redirect
	}
	msg := out.Message
	msg.Content, msg.AllowedMentions = discordMention(out.Mention, p.redirect != "")
	msg.Nonce = out.Nonce
	posted, err := c.SendMessage(ctx, channel, msg, out.Patient)
	if err != nil {
		return discordCreated{}, err
	}
	// A repeated nonce gets the existing message back. Recording it again would make it
	// its own "previous post", for a replace to delete.
	if live, err := s.store.LiveDiscordPosts(out.Kind, out.RefID); err == nil {
		for _, prev := range live {
			if prev.MessageID == posted.ID {
				slog.Info("discord post repeated; Discord kept the first", "kind", out.Kind, "name", out.Name)
				return discordCreated{Post: prev, Redirected: p.why, Repeat: true}, nil
			}
		}
	}
	row := model.DiscordPost{
		Kind: out.Kind, RefID: out.RefID, Scope: out.Scope, Batch: out.Batch, Seq: out.Seq,
		ChannelID: posted.ChannelID, MessageID: posted.ID, Fingerprint: discordFingerprint(msg),
	}
	if row.ChannelID == "" {
		row.ChannelID = channel
	}
	// The message is up whatever happens next. A failure to record it is logged, not
	// returned: returning it would invite a retry, and the retry would post twice.
	if id, err := s.store.RecordDiscordPost(row); err != nil {
		slog.Error("discord post sent but not recorded; the app can't edit or delete it",
			"kind", out.Kind, "name", out.Name, "error", err)
	} else {
		row.ID = id
		// Superseded: a "deleted in Discord" row no longer describes the item.
		if err := s.store.ForgetGoneDiscordPosts(out.Kind, out.RefID); err != nil {
			slog.Error("forget superseded discord posts", "kind", out.Kind, "error", err)
		}
	}
	slog.Info("discord post sent", "kind", out.Kind, "name", out.Name, "redirected", p.why)
	slog.Debug("discord post ids", "channel", row.ChannelID, "message", row.MessageID)
	return discordCreated{Post: row, Redirected: p.why}, nil
}

// discordEdit replaces a recorded message with msg. Unless forced, it skips an edit that
// would change nothing (reporting false) - which also means it can't notice the message
// was deleted, so an admin's explicit Post forces it. errPostGone means the message was
// deleted in Discord; it is marked gone.
func (s *Server) discordEdit(ctx context.Context, post model.DiscordPost, name, mention string, msg discord.Message, force bool) (bool, error) {
	c := s.discord.Client()
	if c == nil {
		return false, discord.ErrOff
	}
	p := s.discordPolicy()
	if !p.mayTouch(post.ChannelID) {
		if p.dryRun {
			slog.Warn("discord edit SUPPRESSED (dry run)", "kind", post.Kind, "name", name)
		}
		return false, errDiscordHeld
	}
	msg.Content, msg.AllowedMentions = discordMention(mention, p.redirect != "")
	fp := discordFingerprint(msg)
	if fp == post.Fingerprint && !force {
		return false, nil
	}
	err := c.EditMessage(ctx, post.ChannelID, post.MessageID, msg)
	if errors.Is(err, discord.ErrNotFound) {
		if mErr := s.store.MarkDiscordPostGone(post.ID, post.MessageID); mErr != nil {
			slog.Error("mark discord post gone", "kind", post.Kind, "error", mErr)
		}
		slog.Info("discord post was deleted in Discord", "kind", post.Kind, "name", name)
		return false, errPostGone
	}
	if err != nil {
		return false, err
	}
	if err := s.store.MarkDiscordPostEdited(post.ID, fp); err != nil {
		slog.Error("record discord edit", "kind", post.Kind, "error", err)
	}
	slog.Info("discord post edited", "kind", post.Kind, "name", name)
	return true, nil
}

// discordDelete deletes a recorded message and forgets it. A message already gone in
// Discord counts as deleted. errDiscordHeld means the valves kept it.
func (s *Server) discordDelete(ctx context.Context, post model.DiscordPost, name string, patient bool) error {
	c := s.discord.Client()
	if c == nil {
		return discord.ErrOff
	}
	p := s.discordPolicy()
	if !p.mayTouch(post.ChannelID) {
		if p.dryRun {
			slog.Warn("discord delete SUPPRESSED (dry run)", "kind", post.Kind, "name", name)
		}
		return errDiscordHeld
	}
	if err := c.DeleteMessage(ctx, post.ChannelID, post.MessageID, patient); err != nil && !errors.Is(err, discord.ErrNotFound) {
		return err
	}
	if err := s.store.ForgetDiscordPost(post.ID); err != nil {
		slog.Error("forget deleted discord post", "kind", post.Kind, "error", err)
	}
	slog.Info("discord post deleted", "kind", post.Kind, "name", name)
	return nil
}

// otherPosts drops the post with messageID from posts: what is "previous" to a post that
// may have been a repeat of one of them.
func otherPosts(posts []model.DiscordPost, messageID string) []model.DiscordPost {
	return slices.DeleteFunc(slices.Clone(posts), func(p model.DiscordPost) bool { return p.MessageID == messageID })
}

// discordDeleteAll deletes every listed message the valves allow, reporting how many
// were deleted, how many the valves kept, and the first error.
func (s *Server) discordDeleteAll(ctx context.Context, posts []model.DiscordPost, name string) (deleted, held int, firstErr error) {
	for _, post := range posts {
		err := s.discordDelete(ctx, post, name, true)
		switch {
		case err == nil:
			deleted++
		case errors.Is(err, errDiscordHeld):
			held++
		case firstErr == nil:
			firstErr = err
		}
	}
	return deleted, held, firstErr
}

// discordMessageURL opens a message in Discord.
func (s *Server) discordMessageURL(channelID, messageID string) string {
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s",
		s.discordSetting(settingDiscordGuild), channelID, messageID)
}

// discordPostState describes a recorded post for the admin pages.
func (s *Server) discordPostState(p model.DiscordPost) *model.DiscordPostState {
	st := &model.DiscordPostState{State: model.PostLive, ChannelID: p.ChannelID,
		PostedAt: p.PostedAt, EditedAt: p.EditedAt}
	if p.GoneAt != "" {
		st.State = model.PostGone
	} else {
		st.URL = s.discordMessageURL(p.ChannelID, p.MessageID)
	}
	return st
}

// discordPostStates reads where every item of a kind stands, by id.
func (s *Server) discordPostStates(kind string) map[int64]*model.DiscordPostState {
	posts, err := s.store.DiscordPostStates(kind)
	if err != nil {
		slog.Error("read discord post states", "kind", kind, "error", err)
		return nil
	}
	out := make(map[int64]*model.DiscordPostState, len(posts))
	for id, p := range posts {
		out[id] = s.discordPostState(p)
	}
	return out
}

// discordFailureText says why a post failed, naming the destination when the cause is the
// channel itself.
func discordFailureText(err error, where string) string {
	switch {
	case errors.Is(err, errNoChannel):
		return "Pick a Discord channel for " + where + " first."
	case errors.Is(err, discord.ErrNotFound):
		return "The Discord channel for " + where + " no longer exists. Pick another one."
	case errors.Is(err, discord.ErrForbidden):
		return "The bot isn't allowed to post in the channel for " + where + "."
	}
	return discordErrorText(err)
}

// writeDiscordFailure answers a failed Discord action: 400 when the admin has something to
// fix here (no channel), 409 when Discord is off, 502 when Discord said no. It logs, because
// a proxy may swap a 502's body for its own page and the log is then the only record.
func writeDiscordFailure(w http.ResponseWriter, what, where string, err error) {
	text := discordFailureText(err, where)
	switch {
	case errors.Is(err, errNoChannel):
		writeError(w, http.StatusBadRequest, text)
		return
	case errors.Is(err, discord.ErrOff):
		writeError(w, http.StatusConflict, text)
		return
	}
	slog.Error("discord action failed", "context", what, "error", err)
	writeError(w, http.StatusBadGateway, text)
}

// discordWriteTimeout bounds one post, edit or delete made for a request.
const discordWriteTimeout = 30 * time.Second

// detachedDiscordContext is the context a request's Discord writes run under. It outlives
// the request: a browser that gives up mid-post must not leave a message posted that the
// app never recorded.
func detachedDiscordContext(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), d)
}

// -- Destinations ---------------------------------------------------------------------

// destinationRules says who may change a destination and which repost modes it offers.
// ok is false for an unknown key.
func destinationRules(key string) (perm string, modes []string, ok bool) {
	switch key {
	case destTeaRooms:
		return permTeahouseTeaRooms, []string{model.RepostNew, model.RepostReplace, model.RepostEdit}, true
	case destAffiliates:
		return permTeahouseAffiliates, []string{model.RepostNew, model.RepostReplace, model.RepostEdit}, true
	}
	// A reading list is several messages, so it can't be edited in place - only replaced.
	if slug, found := strings.CutPrefix(key, "bookclub:"); found && slices.Contains(bookClubSlugs, slug) {
		return bookClubPerm(slug), []string{model.RepostNew, model.RepostReplace}, true
	}
	return "", nil, false
}

// handleDiscordDestinationGet returns where a system posts.
//
//	Endpoint:  GET /api/discord/destinations/{key}
//	Auth:      admin, or the system's page permission
//	Response:  model.DiscordDestinationResponse
func (s *Server) handleDiscordDestinationGet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	perm, _, ok := destinationRules(key)
	if !ok {
		writeError(w, http.StatusNotFound, "Unknown Discord destination")
		return
	}
	if !s.requirePermission(w, r, perm) {
		return
	}
	d, err := s.store.GetDiscordDestination(key)
	if err != nil {
		writeInternalError(w, "read discord destination", err)
		return
	}
	writeJSON(w, http.StatusOK, model.DiscordDestinationResponse{Destination: d})
}

type discordDestinationRequest struct {
	ChannelID  string `json:"channel_id"`
	Mention    string `json:"mention"`
	RepostMode string `json:"repost_mode"`
}

// handleDiscordDestinationSet saves where a system posts: its channel, who to ping, and
// what to do with its previous post.
//
//	Endpoint:  PUT /api/discord/destinations/{key}
//	Auth:      admin, or the system's page permission
//	Request:   discordDestinationRequest
//	Response:  model.DiscordDestinationResponse
func (s *Server) handleDiscordDestinationSet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	perm, modes, ok := destinationRules(key)
	if !ok {
		writeError(w, http.StatusNotFound, "Unknown Discord destination")
		return
	}
	if !s.requirePermission(w, r, perm) {
		return
	}
	req, err := readJSON[discordDestinationRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	d := model.DiscordDestination{
		Key:        key,
		ChannelID:  strings.TrimSpace(req.ChannelID),
		Mention:    strings.TrimSpace(req.Mention),
		RepostMode: strings.TrimSpace(req.RepostMode),
	}
	switch {
	case d.ChannelID != "" && !isDiscordSnowflake(d.ChannelID):
		writeError(w, http.StatusBadRequest, "Pick a Discord channel")
		return
	case !validMention(d.Mention):
		writeError(w, http.StatusBadRequest, "Invalid ping")
		return
	case !slices.Contains(modes, d.RepostMode):
		writeError(w, http.StatusBadRequest, "Invalid repost mode")
		return
	}
	if err := s.store.SaveDiscordDestination(d); err != nil {
		writeInternalError(w, "save discord destination", err)
		return
	}
	saved, err := s.store.GetDiscordDestination(key)
	if err != nil {
		writeInternalError(w, "read discord destination", err)
		return
	}
	slog.Info("discord destination saved", "key", key, "repost_mode", d.RepostMode, "by", s.actorName(r))
	writeJSON(w, http.StatusOK, model.DiscordDestinationResponse{Destination: saved})
}
