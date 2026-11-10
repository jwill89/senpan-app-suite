package live

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type testEvent struct {
	Envelope
	Payload Empty `json:"payload"`
}

func (testEvent) Name() string { return "test" }

func ev() *testEvent { return &testEvent{} }

var (
	players = Audience{Players: true}
	staff   = Audience{Staff: true}
	cards   = Audience{Staff: true, Perms: []string{"bingo-cards"}}
	admins  = Audience{Staff: true, Admins: true}
)

// received drains what s has waiting.
func received(s *Subscriber) []Message {
	var out []Message
	for {
		select {
		case m, open := <-s.Events():
			if !open {
				return out
			}
			out = append(out, m)
		default:
			return out
		}
	}
}

func subscribe(t *testing.T, h *Hub, v Viewer) *Subscriber {
	t.Helper()
	s, _, err := h.Subscribe(v, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestPublishStampsTheEnvelope(t *testing.T) {
	h := New()
	first, second := ev(), &testEvent{Envelope: Game(7)}
	h.Publish(first, staff)
	h.Publish(second, staff)
	if first.ID != h.first || second.ID != first.ID+1 {
		t.Fatalf("ids %d then %d, from %d", first.ID, second.ID, h.first)
	}
	if first.Type != "test" || first.At.IsZero() || first.At.Location() != time.UTC {
		t.Errorf("envelope: %+v", first.Envelope)
	}
	if first.GameID != nil || second.GameID == nil || *second.GameID != 7 {
		t.Errorf("game ids: %v, %v", first.GameID, second.GameID)
	}
	out, err := json.Marshal(first)
	if err != nil || !strings.Contains(string(out), `"payload":{}`) || !strings.Contains(string(out), `"gameId":null`) {
		t.Errorf("an event with nothing to name: %s %v", out, err)
	}
}

// Ids start from the boot time in milliseconds, so a server started again later
// carries on above the ids it gave before.
func TestIDsRiseAcrossARestart(t *testing.T) {
	boot := time.Now()
	before := newAt(boot)
	var last int64
	for range 20 {
		m := ev()
		before.Publish(m, staff)
		last = m.ID
	}
	after := newAt(boot.Add(time.Second))
	m := ev()
	after.Publish(m, staff)
	if m.ID <= last {
		t.Errorf("after the restart %d; before it the last was %d", m.ID, last)
	}
}

// The ring keeps the newest events; a stream resuming from one it let go of is told
// to read everything again.
func TestRing(t *testing.T) {
	h := New()
	var ids []int64
	for range ringSize + 5 {
		m := ev()
		h.Publish(m, staff)
		ids = append(ids, m.ID)
	}
	if len(h.ring) != ringSize || h.ring[0].msg.envelope().ID != ids[5] || h.forgotten != ids[4] {
		t.Fatalf("%d kept, the oldest %d, forgotten %d", len(h.ring), h.ring[0].msg.envelope().ID, h.forgotten)
	}
	_, opened, err := h.Subscribe(Viewer{Admin: true}, ids[3])
	if err != nil || !opened.Resync || opened.Resume != ids[len(ids)-1] || len(opened.Backlog) != 0 {
		t.Errorf("from a forgotten id: %+v, %v", opened, err)
	}
	_, opened, _ = h.Subscribe(Viewer{Admin: true}, ids[4])
	if opened.Resync || len(opened.Backlog) != ringSize {
		t.Errorf("from the newest forgotten id: resync %v, %d events", opened.Resync, len(opened.Backlog))
	}
}

// Each stream hears what its viewer may: players what is theirs, staff what their
// permissions cover (an admin, everything), the log admins alone.
func TestFanOutFollowsTheAudience(t *testing.T) {
	h := New()
	playerA := subscribe(t, h, Viewer{Card: "AAAAAA"})
	playerB := subscribe(t, h, Viewer{Card: "BBBBBB"})
	admin := subscribe(t, h, Viewer{Account: 1, Admin: true})
	cardStaff := subscribe(t, h, Viewer{Account: 2, Perms: []string{"bingo-cards"}})
	gameStaff := subscribe(t, h, Viewer{Account: 3, Perms: []string{"bingo-game"}})

	h.Publish(ev(), players)
	h.Publish(ev(), Audience{Card: "AAAAAA"})
	h.Publish(ev(), staff)
	h.Publish(ev(), cards)
	h.Publish(ev(), admins)
	h.Publish(ev(), Audience{Players: true, Staff: true, Perms: []string{"bingo-game"}})

	for name, c := range map[string]struct {
		s    *Subscriber
		want int
	}{
		"player A": {playerA, 3}, "player B": {playerB, 2},
		"admin": {admin, 4}, "card staff": {cardStaff, 2}, "game staff": {gameStaff, 2},
	} {
		if got := len(received(c.s)); got != c.want {
			t.Errorf("%s heard %d; want %d", name, got, c.want)
		}
	}
}

// A viewer that loses a permission stops hearing what it covered, from the next event.
func TestSetViewer(t *testing.T) {
	h := New()
	s := subscribe(t, h, Viewer{Account: 2, Perms: []string{"bingo-cards", "bingo-game"}})
	h.Publish(ev(), cards)
	s.SetViewer(Viewer{Account: 2, Perms: []string{"bingo-game"}})
	h.Publish(ev(), cards)
	if got := len(received(s)); got != 1 {
		t.Errorf("heard %d; want the one before the permission went", got)
	}
	if v := s.Viewer(); len(v.Perms) != 1 {
		t.Errorf("viewer %+v", v)
	}
}

// A stream resuming from an id gets what it missed that it may hear, in order; one
// from before the hub started, or from an id it never gave, is told to read again.
func TestResume(t *testing.T) {
	h := New()
	first := ev()
	h.Publish(first, staff)
	h.Publish(ev(), cards)
	h.Publish(ev(), players)
	h.Publish(ev(), admins)
	last := ev()
	h.Publish(last, staff)

	_, opened, _ := h.Subscribe(Viewer{Account: 2, Perms: []string{"bingo-cards"}}, first.ID)
	if opened.Resync || len(opened.Backlog) != 2 || ID(opened.Backlog[1]) != last.ID {
		t.Errorf("card staff resuming: %+v", opened)
	}
	_, opened, _ = h.Subscribe(Viewer{Card: "AAAAAA"}, first.ID)
	if opened.Resync || len(opened.Backlog) != 1 {
		t.Errorf("a player resuming: %+v", opened)
	}
	_, opened, _ = h.Subscribe(Viewer{Admin: true}, last.ID)
	if opened.Resync || len(opened.Backlog) != 0 {
		t.Errorf("resuming from the newest: %+v", opened)
	}
	for name, after := range map[string]int64{"before the hub": h.first - 1, "never given": last.ID + 1} {
		if _, opened, _ = h.Subscribe(Viewer{Admin: true}, after); !opened.Resync || opened.Resume != last.ID {
			t.Errorf("%s: %+v", name, opened)
		}
	}
}

// A stream that falls behind is ended rather than holding up the publisher, and the
// others carry on.
func TestASlowStreamIsEnded(t *testing.T) {
	h := New()
	slow, _, _ := h.Subscribe(Viewer{Admin: true}, 0)
	other := subscribe(t, h, Viewer{Admin: true})
	for range queueSize + 1 {
		h.Publish(ev(), staff)
		received(other)
	}
	n := 0
	for range slow.Events() {
		n++
	}
	if n != queueSize {
		t.Errorf("the slow stream got %d before it ended; want %d", n, queueSize)
	}
	h.Publish(ev(), staff)
	if len(received(other)) != 1 {
		t.Error("the other stream stopped hearing")
	}
	if _, staffStreams := h.Streams(); staffStreams != 1 {
		t.Errorf("%d staff streams open; want 1", staffStreams)
	}
	slow.Close()
}

// A lossy event has no id and no place in the ring; a stream behind on them misses
// some and stays open.
func TestLossy(t *testing.T) {
	h := New()
	if h.Hearing(admins) {
		t.Fatal("hearing with nobody open")
	}
	staffOnly := subscribe(t, h, Viewer{Account: 2, Perms: []string{"bingo-game"}})
	if h.Hearing(admins) {
		t.Fatal("a staff account counts as an admin")
	}
	admin := subscribe(t, h, Viewer{Admin: true})
	if !h.Hearing(admins) {
		t.Fatal("an admin is open")
	}
	for range queueSize + 10 {
		h.PublishLossy(ev(), admins)
	}
	if len(admin.Lossy()) != queueSize || len(staffOnly.Lossy()) != 0 || len(h.ring) != 0 {
		t.Errorf("admin %d waiting, staff %d, ring %d", len(admin.Lossy()), len(staffOnly.Lossy()), len(h.ring))
	}
	if m := <-admin.Lossy(); ID(m) != 0 || m.(*testEvent).Type != "test" {
		t.Errorf("a lossy event: %+v", m)
	}
	h.Publish(ev(), staff)
	if len(received(admin)) != 1 {
		t.Error("the admin's stream ended over its lossy queue")
	}
}

func TestPlayerLimit(t *testing.T) {
	h := New()
	h.SetPlayerLimit(1)
	first, _, err := h.Subscribe(Viewer{Card: "AAAAAA"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.Subscribe(Viewer{Card: "BBBBBB"}, 0); !errors.Is(err, ErrFull) {
		t.Fatalf("over the limit: %v", err)
	}
	subscribe(t, h, Viewer{Admin: true})
	first.Close()
	first.Close()
	if players, staffStreams := h.Streams(); players != 0 || staffStreams != 1 {
		t.Errorf("open: %d players, %d staff", players, staffStreams)
	}
	subscribe(t, h, Viewer{Card: "BBBBBB"})
}

func TestRecheckStaff(t *testing.T) {
	h := New()
	player := subscribe(t, h, Viewer{Card: "AAAAAA"})
	admin := subscribe(t, h, Viewer{Admin: true})
	h.RecheckStaff()
	h.RecheckStaff()
	if len(admin.Recheck()) != 1 || len(player.Recheck()) != 0 {
		t.Errorf("recheck waiting: admin %d, player %d", len(admin.Recheck()), len(player.Recheck()))
	}
}

func TestClose(t *testing.T) {
	h := New()
	open, _, _ := h.Subscribe(Viewer{Admin: true}, 0)
	h.Close()
	if _, ok := <-open.Events(); ok {
		t.Fatal("an open stream should end")
	}
	late, _, _ := h.Subscribe(Viewer{Card: "AAAAAA"}, 0)
	if _, ok := <-late.Events(); ok {
		t.Fatal("a stream opened after the close should end at once")
	}
	// Publishing and closing after the hub has closed must not touch a closed channel.
	h.Publish(ev(), staff)
	h.PublishLossy(ev(), admins)
	open.Close()
	late.Close()
}

func TestStamp(t *testing.T) {
	m := Stamp(ev(), 42)
	if ID(m) != 42 || m.(*testEvent).Type != "test" || m.(*testEvent).At.IsZero() {
		t.Errorf("stamped: %+v", m)
	}
}
