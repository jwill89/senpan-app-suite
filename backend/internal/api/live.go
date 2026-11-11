package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"

	"app-suite/internal/bookclubs"
	"app-suite/internal/game"
	"app-suite/internal/live"
	"app-suite/internal/model"
	"app-suite/internal/permissions"
)

// The live events: two streams of server-sent events, one for the staff (the
// consoles and the plugin) and one for a player's board. An event names what changed
// and the client reads it again over REST; a draw carries its number, and an event
// with nothing to read again carries what it reports. Each stream starts with what
// it missed since the Last-Event-ID it sends, and sends a ping every 25 seconds.

var liveTags = []string{"Live"}

// -- The events not about the game ---------------------------------------------

// CardsChangedEvent says a card was made, changed, approved, protected or deleted:
// read the cards again.
type CardsChangedEvent struct {
	live.Envelope
	Payload live.Empty `json:"payload" doc:"Nothing: read the cards again."`
}

// Name is the event's type.
func (CardsChangedEvent) Name() string { return "cards_changed" }

// PatternsChangedEvent says a pattern or a category was made, changed, moved or
// deleted: read the patterns again.
type PatternsChangedEvent struct {
	live.Envelope
	Payload live.Empty `json:"payload" doc:"Nothing: read the patterns again."`
}

// Name is the event's type.
func (PatternsChangedEvent) Name() string { return "patterns_changed" }

// ResourceChangedPayload names a page's data that changed.
type ResourceChangedPayload struct {
	Resource string `json:"resource" doc:"The data that changed, by the page that shows it: read it again if that page is open."`
}

// ResourceChangedEvent says one of the pages' data changed: read it again.
type ResourceChangedEvent struct {
	live.Envelope
	Payload ResourceChangedPayload `json:"payload" doc:"What changed."`
}

// Name is the event's type.
func (ResourceChangedEvent) Name() string { return "resource_changed" }

// SettingsChangedEvent says the site's settings were saved: read them again.
type SettingsChangedEvent struct {
	live.Envelope
	Payload live.Empty `json:"payload" doc:"Nothing: read the settings again."`
}

// Name is the event's type.
func (SettingsChangedEvent) Name() string { return "settings_changed" }

// ThemeChangedPayload names the theme written.
type ThemeChangedPayload struct {
	StyleID *int64 `json:"styleId" nullable:"true" doc:"The theme made, changed, deleted or made the site's; null when the site went back to the default look."`
}

// ThemeChangedEvent says a theme was written: a page wearing the site's theme reads
// it again, as does one wearing this theme, and the Themes page its list.
type ThemeChangedEvent struct {
	live.Envelope
	Payload ThemeChangedPayload `json:"payload" doc:"Which theme."`
}

// Name is the event's type.
func (ThemeChangedEvent) Name() string { return "theme_changed" }

// TeaRoomUnlockedPayload is a tea room whose lock ran out.
type TeaRoomUnlockedPayload struct {
	ID         int64  `json:"id" doc:"The room's id."`
	Name       string `json:"name" doc:"The room's name, to announce."`
	RoomNumber string `json:"roomNumber" doc:"The room's number, to announce."`
}

// TeaRoomUnlockedEvent says a tea room's timed lock ran out and the server lifted
// it. Nobody did anything, so nobody is looking: it carries what to announce.
type TeaRoomUnlockedEvent struct {
	live.Envelope
	Payload TeaRoomUnlockedPayload `json:"payload" doc:"The room."`
}

// Name is the event's type.
func (TeaRoomUnlockedEvent) Name() string { return "tea_room_unlocked" }

// LogPayload is a line of the server's log.
type LogPayload struct {
	Entry LogEntry `json:"entry" doc:"The line, as list-logs answers it."`
}

// LogEvent is a line written to the server's log, for an admin's log viewer. It has
// no place in the feed: a stream that falls behind misses lines, and one that
// reopens doesn't get them again, so the viewer reads the log afresh when its stream
// opens.
type LogEvent struct {
	live.Envelope
	Payload LogPayload `json:"payload" doc:"The line."`
}

// Name is the event's type.
func (LogEvent) Name() string { return "log" }

// CardDeletedEvent tells a board its card was deleted. The stream ends after it.
type CardDeletedEvent struct {
	live.Envelope
	Payload live.Empty `json:"payload" doc:"Nothing more: the board goes home."`
}

// Name is the event's type.
func (CardDeletedEvent) Name() string { return "card_deleted" }

// ResyncEvent opens a stream whose Last-Event-ID is outside what the server keeps
// (it restarted, or the client was away too long): read everything again.
type ResyncEvent struct {
	live.Envelope
	Payload live.Empty `json:"payload" doc:"Nothing: read everything shown again."`
}

// Name is the event's type.
func (ResyncEvent) Name() string { return "resync" }

// PingEvent is the stream's heartbeat, every 25 seconds. A stream that goes quiet
// for much longer has died without saying so, and the client opens it again.
type PingEvent struct {
	live.Envelope
	Payload live.Empty `json:"payload" doc:"Nothing."`
}

// Name is the event's type.
func (PingEvent) Name() string { return "ping" }

// staffEvents are the events on the staff's stream.
func staffEvents() []live.Message {
	return []live.Message{
		&game.GameStartedEvent{}, &game.GameEndedEvent{}, &game.DrawEvent{},
		&game.HalftimePromptEvent{}, &game.HalftimeResolvedEvent{}, &game.AutoDrawEvent{},
		&game.YoeverEvent{}, &game.YoeverSwitchedEvent{}, &game.DrawDelayEvent{}, &game.GameDetailsEvent{},
		&CardsChangedEvent{}, &PatternsChangedEvent{}, &ResourceChangedEvent{}, &SettingsChangedEvent{},
		&ThemeChangedEvent{}, &TeaRoomUnlockedEvent{}, &LogEvent{}, &ResyncEvent{}, &PingEvent{},
	}
}

// boardEvents are the events on a board's stream.
func boardEvents() []live.Message {
	return []live.Message{
		&game.GameStartedEvent{}, &game.GameEndedEvent{}, &game.PlayerDrawEvent{}, &game.MinigameEvent{},
		&game.YoeverEvent{}, &game.YoeverSwitchedEvent{}, &game.GameDetailsEvent{},
		&SettingsChangedEvent{}, &ThemeChangedEvent{}, &CardDeletedEvent{}, &ResyncEvent{}, &PingEvent{},
	}
}

// eventMap names each event as sse.Register takes them.
func eventMap(events []live.Message) map[string]any {
	m := make(map[string]any, len(events))
	for _, e := range events {
		m[e.Name()] = e
	}
	return m
}

// typeEvents says in the document that each event's type is its name alone, so a
// generated client tells the events apart by it.
func typeEvents(reg huma.Registry, events []live.Message) {
	for _, e := range events {
		t := reflect.TypeOf(e).Elem()
		s := reg.SchemaFromRef(reg.Schema(t, true, "").Ref)
		if s == nil || s.Properties["type"] == nil {
			continue
		}
		typed := &huma.Schema{Type: huma.TypeString, Enum: []any{e.Name()}, Description: s.Properties["type"].Description}
		typed.PrecomputeMessages()
		s.Properties["type"] = typed
	}
}

// -- Who hears what -------------------------------------------------------------

// everyone is every board and every staff account.
var everyone = live.Audience{Players: true, Staff: true}

// toPage is the staff accounts holding any of perms.
func toPage(perms ...string) live.Audience { return live.Audience{Staff: true, Perms: perms} }

// toAdmins is the admins alone.
var toAdmins = live.Audience{Staff: true, Admins: true}

// resources are the data resource_changed names, each with the pages that show it:
// an account holding any of them hears of a change (an admin holds every one). An
// empty list is the admins' alone. A book club's reading lists are its own,
// "bookclub-<slug>", heard by that club's page.
var resources = map[string][]string{
	"affiliates":    {permissions.TeahouseAffiliates},
	"announcements": {permissions.TeahouseAnnouncements},
	"carrd":         {permissions.AtelierCarrd},
	"ema-boards":    {permissions.FestivalEma},
	"festival-maps": {permissions.FestivalMap, permissions.FestivalEma},
	"fonts":         {permissions.AtelierFonts},
	"garapons":      {permissions.FestivalGarapon},
	"images":        permissions.ImageBrowsers(),
	"integrations":  nil,
	"presets":       {permissions.BingoPresets},
	"raffles":       {permissions.TeahouseRaffles},
	"stamp-rallies": {permissions.FestivalStampRally},
	"tea-rooms":     {permissions.TeahouseTeaRooms},
	"users":         nil,
	"winners-log":   {permissions.BingoWinnersLog},
}

// resourceAudience is who hears that resource changed, and whether it is one.
func resourceAudience(resource string) (live.Audience, bool) {
	if slug, ok := strings.CutPrefix(resource, "bookclub-"); ok {
		if !slices.Contains(bookclubs.Slugs(), slug) {
			return live.Audience{}, false
		}
		return toPage(permissions.BookClub(slug)), true
	}
	pages, ok := resources[resource]
	switch {
	case !ok:
		return live.Audience{}, false
	case len(pages) == 0:
		return toAdmins, true
	}
	return toPage(pages...), true
}

// ResourceChanged tells the staff whose pages show resource that it changed. An
// unknown resource is logged and told to nobody.
func ResourceChanged(hub *live.Hub, resource string) {
	to, ok := resourceAudience(resource)
	if !ok {
		slog.Error("resource_changed for an unknown resource", "resource", resource)
		return
	}
	if hub != nil {
		hub.Publish(&ResourceChangedEvent{Payload: ResourceChangedPayload{Resource: resource}}, to)
	}
}

// TeaRoomUnlocked tells the staff with the Tea Rooms page that a room's lock ran out.
func TeaRoomUnlocked(hub *live.Hub, id int64, name, roomNumber string) {
	if hub != nil {
		hub.Publish(&TeaRoomUnlockedEvent{Payload: TeaRoomUnlockedPayload{ID: id, Name: name, RoomNumber: roomNumber}},
			toPage(permissions.TeahouseTeaRooms))
	}
}

// LogTail is the logging sink that sends each line of the log to the admins' streams.
// It reads a line only while an admin is listening, and never logs itself, since it
// runs inside the logger.
func LogTail(hub *live.Hub) func(line []byte) {
	return func(line []byte) {
		if !hub.Hearing(toAdmins) {
			return
		}
		if e, ok := model.ParseLogEntry(line); ok {
			hub.PublishLossy(&LogEvent{Payload: LogPayload{Entry: LogEntry{Time: e.Time, Level: e.Level, Message: e.Message, Fields: e.Fields}}}, toAdmins)
		}
	}
}

// publish sends an event to its audience, when there is a feed.
func (h handlers) publish(msg live.Message, to live.Audience) {
	if h.Live != nil {
		h.Live.Publish(msg, to)
	}
}

// -- The streams ------------------------------------------------------------------

type staffStreamInput struct {
	LastEventID string `header:"Last-Event-ID" doc:"The id of the last event this client got, sent when it reopens the stream: the events since it that the account may hear come first. One the server doesn't keep is answered with resync."`
}

type boardStreamInput struct {
	ID          string `path:"id" pattern:"^[A-Za-z0-9]{6}$" patternDescription:"six letters or digits" doc:"The board's card id, in any case."`
	LastEventID string `header:"Last-Event-ID" doc:"The id of the last event this board got, sent when it reopens the stream: the events since it come first. One the server doesn't keep is answered with resync."`
}

var cardIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{6}$`)

func (h handlers) registerLive(api huma.API) {
	staff, board := staffEvents(), boardEvents()
	sse.Register(api, huma.Operation{
		OperationID: "stream-staff-events", Method: http.MethodGet, Path: "/events",
		Summary: "The staff's live events",
		Description: "Server-sent events for a signed-in account: what changed on the pages its permissions cover " +
			"(an admin's, every page, and the server's log as it is written). The game's events need the Game page, " +
			"and a draw reaches it at once, with the winners. The account is checked again every heartbeat and " +
			"whenever an account changes: one deactivated or deleted, whose password changed or whose session ended " +
			"has its stream ended, and one that lost a permission stops hearing what it covered. An account with no " +
			"permission has nothing to follow (403). Send Last-Event-ID when reopening to resume; read the state " +
			"shown again on every open, since the stream carries only what changes.",
		Tags: liveTags, Security: secured,
		Middlewares: huma.Middlewares{h.staffFeed(api)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusServiceUnavailable},
	}, eventMap(staff), h.streamStaff)
	sse.Register(api, huma.Operation{
		OperationID: "stream-board-events", Method: http.MethodGet, Path: "/board/{id}/events",
		Summary: "A board's live events",
		Description: "Server-sent events for a player's board, by its card id: the game starting and ending, each " +
			"number once its draw delay has passed, the half-time mini-game, the It's Yoever reaction, the game's " +
			"details, and the site's look and settings. The stream ends after card_deleted. A personal card waiting " +
			"for approval is refused (403), as get-board refuses it; past 500 open boards, one more is refused (503). " +
			"Send Last-Event-ID when reopening to resume; read the board again on every open.",
		Tags:        liveTags,
		Middlewares: huma.Middlewares{h.boardFeed(api)},
		Errors:      []int{http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusServiceUnavailable},
	}, eventMap(board), h.streamBoard)
	typeEvents(api.OpenAPI().Components.Schemas, append(staff, board...))
}

// feedKey carries an opened stream from its middleware to its handler.
type feedKey struct{}

// feed is a stream opened before its answer starts, so nothing published between the
// answer and the handler is missed.
type feed struct {
	sub    *live.Subscriber
	opened live.Opened
	// account is the staff stream's account, 0 for a board.
	account int64
}

// follows reports whether u has anything on the staff stream to follow.
func follows(u *model.User) bool {
	return u != nil && (u.IsAdmin || len(u.Permissions) > 0)
}

func staffViewer(u *model.User) live.Viewer {
	return live.Viewer{Account: u.ID, Admin: u.IsAdmin, Perms: u.Permissions}
}

// lastEventID reads a Last-Event-ID header: 0 for none, or one that isn't an id.
func lastEventID(header string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(header), 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}

// staffFeed lets a signed-in account with something to follow open the staff's
// stream, opening it here.
func (h handlers) staffFeed(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		u := h.who(ctx)
		switch {
		case u == nil:
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Sign in to continue.")
			return
		case !follows(u):
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "This account has no page to follow live yet.")
			return
		case h.Live == nil:
			_ = huma.WriteErr(api, ctx, http.StatusServiceUnavailable, "The live events aren't running.")
			return
		}
		sub, opened, err := h.Live.Subscribe(staffViewer(u), lastEventID(ctx.Header("Last-Event-ID")))
		if err != nil {
			_ = huma.WriteErr(api, ctx, http.StatusServiceUnavailable, "The live events can't take another stream right now.")
			return
		}
		defer sub.Close()
		next(huma.WithValue(ctx, feedKey{}, &feed{sub: sub, opened: opened, account: u.ID}))
	}
}

// boardFeed lets anyone with a board's card id open its stream, opening it here. An
// id that isn't one is left to the operation's validation (422).
func (h handlers) boardFeed(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		id := ctx.Param("id")
		if !cardIDPattern.MatchString(id) {
			next(ctx)
			return
		}
		if h.Live == nil {
			_ = huma.WriteErr(api, ctx, http.StatusServiceUnavailable, "The live events aren't running.")
			return
		}
		c, err := h.cardAnswer(h.read().GetCard(ctx.Context(), cardKey(id)))
		if err != nil {
			_ = huma.WriteErr(api, ctx, statusOf(err), messageOf(err))
			return
		}
		if c.CustomStatus == "pending" {
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "This personal card is waiting for staff to approve it, and can't be played yet.")
			return
		}
		sub, opened, err := h.Live.Subscribe(live.Viewer{Card: c.ID}, lastEventID(ctx.Header("Last-Event-ID")))
		if errors.Is(err, live.ErrFull) {
			ctx.SetHeader("Retry-After", "30")
			_ = huma.WriteErr(api, ctx, http.StatusServiceUnavailable, "Too many boards are open right now. Try again in a moment.")
			return
		}
		defer sub.Close()
		next(huma.WithValue(ctx, feedKey{}, &feed{sub: sub, opened: opened}))
	}
}

// statusOf is the status an operation's error answers, 500 for one that isn't huma's.
func statusOf(err error) int {
	var se huma.StatusError
	if errors.As(err, &se) {
		return se.GetStatus()
	}
	return http.StatusInternalServerError
}

// messageOf is the detail an operation's error answers.
func messageOf(err error) string {
	var em *huma.ErrorModel
	if errors.As(err, &em) {
		return em.Detail
	}
	return "Something went wrong on the server."
}

func (h handlers) streamStaff(ctx context.Context, _ *staffStreamInput, send sse.Sender) {
	f, _ := ctx.Value(feedKey{}).(*feed)
	if f == nil {
		return
	}
	afresh := requestOf(ctx).Account
	h.stream(ctx, f, send, func() bool {
		if afresh == nil {
			return true
		}
		u := afresh(ctx)
		if u == nil || u.ID != f.account || !follows(u) {
			h.Logger.Info("live stream ended: the account may no longer follow it", "user_id", f.account)
			return false
		}
		f.sub.SetViewer(staffViewer(u))
		return true
	})
}

func (h handlers) streamBoard(ctx context.Context, _ *boardStreamInput, send sse.Sender) {
	if f, _ := ctx.Value(feedKey{}).(*feed); f != nil {
		h.stream(ctx, f, send, nil)
	}
}

// stream sends a stream's events until it ends: its client goes, the hub ends it,
// its card is deleted, or recheck (staff's) says its account may not follow it.
// Each event carries its id as the SSE id, but the ping and a log line don't, so a
// client resumes from the last event in the feed.
func (h handlers) stream(ctx context.Context, f *feed, send sse.Sender, recheck func() bool) {
	deliver := func(m live.Message) error {
		return send(sse.Message{IDString: strconv.FormatInt(live.ID(m), 10), Data: m})
	}
	if f.opened.Resync {
		if deliver(live.Stamp(&ResyncEvent{}, f.opened.Resume)) != nil {
			return
		}
	}
	for _, m := range f.opened.Backlog {
		if deliver(m) != nil {
			return
		}
	}
	every := h.Live.Heartbeat()
	beat := time.NewTicker(every)
	defer beat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case m, open := <-f.sub.Events():
			if !open || deliver(m) != nil {
				return
			}
			if _, gone := m.(*CardDeletedEvent); gone {
				return
			}
		case m := <-f.sub.Lossy():
			if send.Data(m) != nil {
				return
			}
		case <-f.sub.Recheck():
			if recheck != nil && !recheck() {
				return
			}
		case <-beat.C:
			if recheck != nil && !recheck() {
				return
			}
			if send.Data(live.Stamp(&PingEvent{}, 0)) != nil {
				return
			}
			if now := h.Live.Heartbeat(); now != every {
				every = now
				beat.Reset(every)
			}
		}
	}
}
