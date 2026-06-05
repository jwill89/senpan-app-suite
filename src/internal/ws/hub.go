// Package ws provides a WebSocket hub for broadcasting real-time updates.
package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	// writeWait is the maximum time allowed for writing a message to a client.
	writeWait = 10 * time.Second
	// pingPeriod is the interval between server-initiated WebSocket pings.
	// Must be shorter than the reverse proxy's idle timeout (typically 60s).
	pingPeriod = 30 * time.Second
	// maxMsgSize is the maximum size (bytes) of incoming messages from clients.
	// Only client keepalive pings are expected, so this can be small.
	maxMsgSize = 512
)

// Hub maintains a set of active WebSocket clients and broadcasts messages.
type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
	ctx     context.Context
	cancel  context.CancelFunc
}

// client represents a single WebSocket connection registered with the Hub.
// Each client has its own buffered send channel and a dedicated read/write
// goroutine pair that handles message delivery and connection health.
type client struct {
	hub    *Hub               // back-reference to the owning hub
	conn   *websocket.Conn    // underlying WebSocket connection
	send   chan []byte        // buffered channel of outbound messages
	cardID string             // non-empty for player connections; used to target disconnects
	cancel context.CancelFunc // cancels this client's context (signals pumps to exit)
}

// NewHub creates a new Hub with a background context.
// Call Shutdown() to gracefully close all connections.
func NewHub() *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	return &Hub{
		clients: make(map[*client]struct{}),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Shutdown gracefully closes all WebSocket connections and prevents new ones.
func (h *Hub) Shutdown(ctx context.Context) {
	h.cancel() // signal all client goroutines to stop

	// Close all connections with a proper close frame.
	h.mu.Lock()
	for c := range h.clients {
		if c.conn != nil {
			c.conn.Close(websocket.StatusGoingAway, "server shutting down")
		}
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

// Broadcast sends a JSON message to every connected client.
func (h *Hub) Broadcast(msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("ws: broadcast marshal failed", "error", err)
		return
	}
	h.broadcastRaw(data, nil)
}

// BroadcastToPlayers sends a JSON message only to clients with a non-empty cardID.
func (h *Hub) BroadcastToPlayers(msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("ws: broadcast marshal failed", "error", err)
		return
	}
	h.broadcastRaw(data, func(c *client) bool { return c.cardID != "" })
}

// BroadcastToAdmins sends a JSON message only to clients with an empty cardID.
func (h *Hub) BroadcastToAdmins(msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("ws: broadcast marshal failed", "error", err)
		return
	}
	h.broadcastRaw(data, func(c *client) bool { return c.cardID == "" })
}

// broadcastRaw sends pre-marshaled data to clients matching the optional filter.
// If filter is nil, all clients receive the message.
func (h *Hub) broadcastRaw(data []byte, filter func(*client) bool) {
	h.mu.RLock()
	var dead []*client
	for c := range h.clients {
		if filter != nil && !filter(c) {
			continue
		}
		select {
		case c.send <- data:
		default:
			dead = append(dead, c)
		}
	}
	h.mu.RUnlock()

	// Unregister stale clients outside the read lock.
	for _, c := range dead {
		h.unregister(c)
	}
}

// ServeWS upgrades an HTTP request to a WebSocket connection and registers it.
// cardID should be non-empty for player connections so they can be disconnected
// when their card is deleted.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, cardID string) {
	// Default same-origin check: Origin must match Host header.
	// Requires the reverse proxy to set ProxyPreserveHost On so the
	// original Host header reaches Go (e.g. Apache: ProxyPreserveHost On).
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		slog.Error("ws: accept failed", "error", err)
		return
	}
	conn.SetReadLimit(maxMsgSize)

	clientCtx, clientCancel := context.WithCancel(h.ctx)

	c := &client{
		hub:    h,
		conn:   conn,
		send:   make(chan []byte, 64),
		cardID: cardID,
		cancel: clientCancel,
	}
	h.register(c)

	go c.writePump(clientCtx)
	go c.readPump(clientCtx)
}

// DisconnectCardClients sends msg to every client associated with cardID,
// then unregisters them. The buffered send channel ensures the message is
// delivered before the close frame.
func (h *Hub) DisconnectCardClients(cardID string, msg []byte) {
	h.mu.RLock()
	var targets []*client
	for c := range h.clients {
		if c.cardID == cardID {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range targets {
		select {
		case c.send <- msg:
		default:
		}
		h.unregister(c)
	}
}

// DisconnectAllPlayerClients sends msg to every client that has a non-empty
// cardID (i.e. all player connections), then unregisters them.
func (h *Hub) DisconnectAllPlayerClients(msg []byte) {
	h.mu.RLock()
	var targets []*client
	for c := range h.clients {
		if c.cardID != "" {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range targets {
		select {
		case c.send <- msg:
		default:
		}
		h.unregister(c)
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// register adds a client to the hub's active set.
func (h *Hub) register(c *client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

// unregister removes a client from the hub, closes its send channel
// (which signals writePump to exit), and cancels its context.
// Safe to call multiple times — the map check prevents double-close.
func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
		c.cancel()
	}
	h.mu.Unlock()
}

// readPump reads (and discards) incoming messages; keeps the connection alive.
// coder/websocket handles ping/pong automatically.
func (c *client) readPump(ctx context.Context) {
	defer func() {
		c.hub.unregister(c)
		_ = c.conn.CloseNow()
	}()
	for {
		_, _, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
	}
}

// writePump sends queued messages and periodic pings.
func (c *client) writePump(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.hub.unregister(c)
		_ = c.conn.CloseNow()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.send:
			if !ok {
				c.conn.Close(websocket.StatusNormalClosure, "")
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, writeWait)
			err := c.conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
			// Drain any queued messages to reduce syscall overhead.
			for n := len(c.send); n > 0; n-- {
				writeCtx, cancel := context.WithTimeout(ctx, writeWait)
				err := c.conn.Write(writeCtx, websocket.MessageText, <-c.send)
				cancel()
				if err != nil {
					return
				}
			}
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, writeWait)
			err := c.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
