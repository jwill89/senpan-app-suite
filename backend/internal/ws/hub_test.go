package ws

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestNewHub_Empty(t *testing.T) {
	h := NewHub()
	defer h.Shutdown(context.Background())
	if h.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", h.ClientCount())
	}
}

func TestHub_RegisterUnregister(t *testing.T) {
	h := NewHub()
	defer h.Shutdown(context.Background())
	c := &client{hub: h, send: make(chan []byte, 64), cancel: func() {}}
	h.register(c)

	if h.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", h.ClientCount())
	}

	h.unregister(c)
	if h.ClientCount() != 0 {
		t.Errorf("expected 0 clients after unregister, got %d", h.ClientCount())
	}
}

func TestHub_UnregisterIdempotent(t *testing.T) {
	h := NewHub()
	defer h.Shutdown(context.Background())
	c := &client{hub: h, send: make(chan []byte, 64), cancel: func() {}}
	h.register(c)

	h.unregister(c)
	h.unregister(c) // should not panic or double-close
	if h.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", h.ClientCount())
	}
}

func TestHub_Broadcast(t *testing.T) {
	h := NewHub()
	defer h.Shutdown(context.Background())
	c1 := &client{hub: h, send: make(chan []byte, 64), cancel: func() {}}
	c2 := &client{hub: h, send: make(chan []byte, 64), cancel: func() {}}
	h.register(c1)
	h.register(c2)

	msg := map[string]string{"type": "test", "data": "hello"}
	h.Broadcast(msg)

	expected, _ := json.Marshal(msg)
	for i, c := range []*client{c1, c2} {
		select {
		case got := <-c.send:
			if string(got) != string(expected) {
				t.Errorf("client %d: got %s; want %s", i, got, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("client %d: timeout waiting for broadcast", i)
		}
	}
}

func TestHub_Broadcast_EmptyHub(t *testing.T) {
	h := NewHub()
	defer h.Shutdown(context.Background())
	// Should not panic on empty hub
	h.Broadcast(map[string]string{"type": "test"})
}

func TestHub_Broadcast_DropsSlow(t *testing.T) {
	h := NewHub()
	defer h.Shutdown(context.Background())
	// Use a zero-capacity channel so any non-blocking send will go to default
	slow := &client{hub: h, send: make(chan []byte), cancel: func() {}}
	fast := &client{hub: h, send: make(chan []byte, 64), cancel: func() {}}
	h.register(slow)
	h.register(fast)

	h.Broadcast(map[string]string{"type": "test"})

	// Fast client should receive the message
	select {
	case <-fast.send:
	case <-time.After(time.Second):
		t.Fatal("fast client should have received message")
	}

	// Slow client should be unregistered
	if h.ClientCount() != 1 {
		t.Errorf("expected 1 client (slow dropped), got %d", h.ClientCount())
	}
}

func TestHub_DisconnectCardClients(t *testing.T) {
	h := NewHub()
	defer h.Shutdown(context.Background())
	player1 := &client{hub: h, send: make(chan []byte, 64), cardID: "CARD1", cancel: func() {}}
	player2 := &client{hub: h, send: make(chan []byte, 64), cardID: "CARD1", cancel: func() {}}
	other := &client{hub: h, send: make(chan []byte, 64), cardID: "CARD2", cancel: func() {}}
	admin := &client{hub: h, send: make(chan []byte, 64), cardID: "", cancel: func() {}}

	h.register(player1)
	h.register(player2)
	h.register(other)
	h.register(admin)

	msg := []byte(`{"type":"card_deleted"}`)
	h.DisconnectCardClients("CARD1", msg)

	// player1 and player2 should be unregistered
	if h.ClientCount() != 2 {
		t.Errorf("expected 2 remaining clients, got %d", h.ClientCount())
	}

	// player1 should have received the message, then channel closed
	select {
	case got := <-player1.send:
		if string(got) != string(msg) {
			t.Errorf("player1 got %s; want %s", got, msg)
		}
	default:
		t.Error("player1 should have received disconnect message")
	}

	// Channel should be closed after message is drained
	if _, ok := <-player1.send; ok {
		t.Error("player1 send channel should be closed")
	}

	// Same for player2
	select {
	case got := <-player2.send:
		if string(got) != string(msg) {
			t.Errorf("player2 got %s; want %s", got, msg)
		}
	default:
		t.Error("player2 should have received disconnect message")
	}

	// other and admin should still have open channels
	select {
	case <-other.send:
		t.Error("other should not have received any message")
	default:
		// expected
	}
}

func TestHub_DisconnectAllPlayerClients(t *testing.T) {
	h := NewHub()
	defer h.Shutdown(context.Background())
	p1 := &client{hub: h, send: make(chan []byte, 64), cardID: "CARD1", cancel: func() {}}
	p2 := &client{hub: h, send: make(chan []byte, 64), cardID: "CARD2", cancel: func() {}}
	admin := &client{hub: h, send: make(chan []byte, 64), cardID: "", cancel: func() {}}

	h.register(p1)
	h.register(p2)
	h.register(admin)

	msg := []byte(`{"type":"card_deleted"}`)
	h.DisconnectAllPlayerClients(msg)

	// Only admin should remain
	if h.ClientCount() != 1 {
		t.Errorf("expected 1 client (admin), got %d", h.ClientCount())
	}

	// Both players should have received the message
	for _, p := range []*client{p1, p2} {
		select {
		case got := <-p.send:
			if string(got) != string(msg) {
				t.Errorf("player got %s; want %s", got, msg)
			}
		default:
			t.Error("player should have received disconnect message")
		}
	}

	// Admin should not have received anything
	select {
	case <-admin.send:
		t.Error("admin should not have received disconnect message")
	default:
		// expected
	}
}

func TestHub_DisconnectCardClients_NoMatch(t *testing.T) {
	h := NewHub()
	defer h.Shutdown(context.Background())
	c := &client{hub: h, send: make(chan []byte, 64), cardID: "CARD1", cancel: func() {}}
	h.register(c)

	h.DisconnectCardClients("NONEXISTENT", []byte(`{}`))

	// Nothing should change
	if h.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", h.ClientCount())
	}
}

func TestHub_DisconnectAllPlayerClients_NoPlayers(t *testing.T) {
	h := NewHub()
	defer h.Shutdown(context.Background())
	admin := &client{hub: h, send: make(chan []byte, 64), cardID: "", cancel: func() {}}
	h.register(admin)

	h.DisconnectAllPlayerClients([]byte(`{}`))

	if h.ClientCount() != 1 {
		t.Errorf("expected 1 client (admin), got %d", h.ClientCount())
	}
}
