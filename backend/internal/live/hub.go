// Package live is the in-process feed behind the event streams. A handler publishes
// an event, after the write it reports has landed, with the audience that may hear
// it; every open stream whose viewer is in that audience gets it. A ring of recent
// events lets a stream that dropped resume from the last event id it got.
package live

import (
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Envelope is what every event carries, whatever it reports.
type Envelope struct {
	ID     int64     `json:"id" doc:"The event's place in the feed. Ids rise with every event, across a restart too; a client resumes after a drop by sending the last one it got as Last-Event-ID. 0 for an event outside the feed (the ping heartbeat, a log line), which also carries no SSE id."`
	Type   string    `json:"type" doc:"What happened: the same as the SSE event name."`
	GameID *int64    `json:"gameId" nullable:"true" doc:"The bingo game the event is about; null for an event about no game."`
	At     time.Time `json:"at" doc:"When it happened, RFC 3339 in UTC."`
}

func (e *Envelope) envelope() *Envelope { return e }

// Message is an event: a struct embedding Envelope, with its payload, named by Name
// (its type, and its SSE event name).
type Message interface {
	Name() string
	envelope() *Envelope
}

// Empty is the payload of an event that names what changed and nothing more.
type Empty struct{}

// Game is the envelope of an event about game id.
func Game(id int64) Envelope { return Envelope{GameID: &id} }

// ID is m's id: 0 until it is published, and for an event outside the feed.
func ID(m Message) int64 { return m.envelope().ID }

// Stamp fills in m's envelope for an event sent outside Publish: id (0 for none), its
// type and the time now.
func Stamp(m Message, id int64) Message {
	env := m.envelope()
	env.ID, env.Type, env.At = id, m.Name(), time.Now().UTC()
	return m
}

// Audience is who may hear an event. A player stream hears it when Players is set,
// or when it is Card's. A staff stream hears it when Staff is set and the account
// covers it: Admins limits it to admins; otherwise an account holding any of Perms
// hears it (an admin holds every permission), and an empty Perms is every account.
type Audience struct {
	Players bool
	Card    string
	Staff   bool
	Perms   []string
	Admins  bool
}

// Viewer is who a stream is for: a player's board (Card), or a signed-in account.
type Viewer struct {
	Card    string
	Account int64
	Admin   bool
	Perms   []string
}

func (a Audience) reaches(v *Viewer) bool {
	if v.Card != "" {
		return a.Players || a.Card == v.Card
	}
	switch {
	case !a.Staff:
		return false
	case a.Admins:
		return v.Admin
	case len(a.Perms) == 0 || v.Admin:
		return true
	}
	for _, p := range a.Perms {
		if slices.Contains(v.Perms, p) {
			return true
		}
	}
	return false
}

const (
	// ringSize is how many recent events a stream can resume across.
	ringSize = 1000
	// queueSize is how many events a stream may fall behind by. One further behind
	// is ended, and resumes from its last event id when its client reopens it.
	queueSize = 64
	// DefaultHeartbeat is how often a stream sends its ping: inside Cloudflare's
	// 100-second and Apache's 120-second idle timeouts, so neither closes a quiet
	// stream, and often enough for a client to notice one that died silently.
	DefaultHeartbeat = 25 * time.Second
)

// ErrFull is a player stream refused because the players' streams are at their limit.
var ErrFull = errors.New("too many player streams are open")

// Hub keeps the recent events and fans new ones out to the open streams. Ids start
// from the time the hub was made, in milliseconds, so they keep rising across a
// restart: a server publishes far fewer events than a millisecond each.
//
// Nothing logs while holding mu: the log tail publishes from inside the logger.
type Hub struct {
	mu        sync.Mutex
	first     int64
	next      int64
	ring      []entry
	forgotten int64 // the newest id the ring has let go of
	subs      map[*Subscriber]struct{}
	players   int
	closed    bool

	playerLimit int
	heartbeat   atomic.Int64
}

type entry struct {
	msg Message
	to  Audience
}

// New is an empty hub, its ids starting now.
func New() *Hub { return newAt(time.Now()) }

func newAt(now time.Time) *Hub {
	h := &Hub{first: now.UnixMilli(), subs: map[*Subscriber]struct{}{}, playerLimit: 500}
	h.next = h.first
	h.heartbeat.Store(int64(DefaultHeartbeat))
	return h
}

// Heartbeat is how often a stream sends its ping.
func (h *Hub) Heartbeat() time.Duration { return time.Duration(h.heartbeat.Load()) }

// SetHeartbeat changes how often a stream sends its ping, from its next one; a test
// sets it short.
func (h *Hub) SetHeartbeat(d time.Duration) { h.heartbeat.Store(int64(d)) }

// SetPlayerLimit changes how many player streams may be open at once. They take no
// sign-in, so the limit keeps one client from holding the server's memory open.
func (h *Hub) SetPlayerLimit(n int) {
	h.mu.Lock()
	h.playerLimit = n
	h.mu.Unlock()
}

// Publish gives msg the next id, keeps it, and sends it to every open stream in to.
// A stream that has fallen too far behind is ended rather than holding up the
// publisher; its client reopens it and resumes from its last event id.
func (h *Hub) Publish(msg Message, to Audience) {
	h.mu.Lock()
	defer h.mu.Unlock()
	env := msg.envelope()
	env.ID, env.Type, env.At = h.next, msg.Name(), time.Now().UTC()
	h.next++
	h.ring = append(h.ring, entry{msg, to})
	if over := len(h.ring) - ringSize; over > 0 {
		h.forgotten = h.ring[over-1].msg.envelope().ID
		h.ring = slices.Delete(h.ring, 0, over)
	}
	for s := range h.subs {
		if !to.reaches(s.viewer.Load()) {
			continue
		}
		select {
		case s.events <- msg:
		default:
			h.drop(s)
		}
	}
}

// PublishLossy sends msg to the open streams in to now, without an id or a place in
// the ring: a stream that is behind misses it, and a stream that reopens doesn't get
// it again. For the log tail, whose page reads the log afresh when its stream opens.
func (h *Hub) PublishLossy(msg Message, to Audience) {
	h.mu.Lock()
	defer h.mu.Unlock()
	Stamp(msg, 0)
	for s := range h.subs {
		if !to.reaches(s.viewer.Load()) {
			continue
		}
		select {
		case s.lossy <- msg:
		default:
		}
	}
}

// Hearing reports whether any open stream would hear an event for to, so a caller
// can skip building one nobody gets.
func (h *Hub) Hearing(to Audience) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if to.reaches(s.viewer.Load()) {
			return true
		}
	}
	return false
}

// Opened is what a stream starts with: the events since the client's last event id
// that its viewer may hear, or, when that id is outside what the hub keeps, Resync
// set and Resume the id the stream picks up from.
type Opened struct {
	Backlog []Message
	Resync  bool
	Resume  int64
}

// Subscribe opens a stream for v, resuming after the event id after (0 for a stream
// opened afresh). The backlog and the stream's place among new events are taken
// together, so nothing published meanwhile is missed or sent twice. A hub that has
// closed answers a stream already ended. A player stream past the limit is ErrFull.
func (h *Hub) Subscribe(v Viewer, after int64) (*Subscriber, Opened, error) {
	s := &Subscriber{
		hub:     h,
		events:  make(chan Message, queueSize),
		lossy:   make(chan Message, queueSize),
		recheck: make(chan struct{}, 1),
	}
	s.viewer.Store(&v)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		s.ended = true
		close(s.events)
		return s, Opened{}, nil
	}
	if v.Card != "" {
		if h.players >= h.playerLimit {
			return nil, Opened{}, ErrFull
		}
		h.players++
	}
	h.subs[s] = struct{}{}
	var out Opened
	if after > 0 {
		if after < h.first || after < h.forgotten || after >= h.next {
			out.Resync, out.Resume = true, h.next-1
		} else {
			for _, e := range h.ring {
				if e.msg.envelope().ID > after && e.to.reaches(&v) {
					out.Backlog = append(out.Backlog, e.msg)
				}
			}
		}
	}
	return s, out, nil
}

// RecheckStaff asks every staff stream to check its account again now, after a
// change to an account or a sign-in.
func (h *Hub) RecheckStaff() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if s.viewer.Load().Card == "" {
			select {
			case s.recheck <- struct{}{}:
			default:
			}
		}
	}
}

// Streams is how many streams are open: players' and staff's.
func (h *Hub) Streams() (players, staff int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.players, len(h.subs) - h.players
}

// Close ends every open stream, and any opened afterwards, for shutdown.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for s := range h.subs {
		h.drop(s)
	}
}

// drop ends s's stream. The caller holds mu.
func (h *Hub) drop(s *Subscriber) {
	if s.ended {
		return
	}
	s.ended = true
	delete(h.subs, s)
	if s.viewer.Load().Card != "" {
		h.players--
	}
	close(s.events)
}

// Subscriber is an open stream's place in the hub.
type Subscriber struct {
	hub     *Hub
	viewer  atomic.Pointer[Viewer]
	events  chan Message
	lossy   chan Message
	recheck chan struct{}
	ended   bool // guarded by hub.mu
}

// Events are the stream's events in order. It closes when the stream is ended: by
// Close, by falling too far behind, or by the hub closing.
func (s *Subscriber) Events() <-chan Message { return s.events }

// Lossy are the events published to the stream without a place in the feed.
func (s *Subscriber) Lossy() <-chan Message { return s.lossy }

// Recheck says the stream's account should be checked again now.
func (s *Subscriber) Recheck() <-chan struct{} { return s.recheck }

// SetViewer changes who the stream is for, from the next event on: an account whose
// permissions changed hears what they cover now.
func (s *Subscriber) SetViewer(v Viewer) { s.viewer.Store(&v) }

// Viewer is who the stream is for now.
func (s *Subscriber) Viewer() Viewer { return *s.viewer.Load() }

// Close ends the stream and lets its place in the hub go. Calling it again does
// nothing.
func (s *Subscriber) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	s.hub.drop(s)
}
