// Package discordtest is a fake Discord for tests and local runs: the slice of the REST API
// and the gateway that the app's bot uses, held in memory.
//
// It checks the bot token, keeps the server's channels and roles, records every message the
// bot posts, edits or deletes, honors nonces the way Discord does, and can be told to fail:
// a rate limit, a refusal, a status, or a message "deleted in Discord".
//
// Mount Handler at the root of an httptest.Server (or any listener) and point the client's
// base URL at it. The gateway is served at /gateway, and GET /gateway/bot advertises it.
package discordtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"app-suite/internal/discord"
)

// IDs the fake uses unless told otherwise.
const (
	GuildID   = "999011226568888410"
	AppID     = "1563399760994635906"
	BotUserID = "100000000000000001"
	Token     = "test-bot-token"
)

// Message is a message the bot posted, as the fake holds it.
type Message struct {
	ID              string
	ChannelID       string
	Content         string
	Embeds          []discord.Embed
	Components      []discord.Component
	AllowedMentions discord.AllowedMentions
	Nonce           string
	Edits           int
	Deleted         bool
}

// Server is the fake. Its exported fields are its configuration; set them before use or
// under Lock/Unlock.
type Server struct {
	mu sync.Mutex

	Token string
	// AnyToken accepts every bot token (for a local run, where whatever was pasted into the
	// UI should work). The token is still required to be present.
	AnyToken  bool
	AppID     string
	AppName   string
	BotUserID string
	BotName   string
	GuildID   string
	GuildName string
	Channels  []discord.Channel
	Roles     []discord.Role
	// BotRoles are the roles the bot holds in the server.
	BotRoles []string
	// Webhooks are old-style channel webhooks, by id, for the move off webhooks.
	Webhooks map[string]Webhook

	messages map[string]*Message
	order    []string
	nonces   map[string]string
	nextID   int64
	calls    []string

	// Queued failures, consumed one per matching request.
	failures []failure
	// exhaust marks the next answer's rate-limit bucket as used up for this long.
	exhaust time.Duration

	// Gateway bookkeeping.
	identifies     int
	resumes        int
	heartbeats     int
	presences      []string
	reconnectAfter int
}

// Webhook is an old-style channel webhook the fake knows: looked up by id + token, deleted
// by id (as the bot).
type Webhook struct {
	Token     string
	Name      string
	GuildID   string
	ChannelID string
	Deleted   bool
}

// AddWebhook registers a webhook and returns its URL against this fake's base.
func (s *Server) AddWebhook(id string, w Webhook) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Webhooks == nil {
		s.Webhooks = map[string]Webhook{}
	}
	s.Webhooks[id] = w
}

// WebhookDeleted reports whether the bot deleted a webhook.
func (s *Server) WebhookDeleted(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Webhooks[id].Deleted
}

type failure struct {
	match      string // "METHOD prefix", e.g. "POST /channels/"; "" matches anything
	status     int
	body       string
	retryAfter float64
	skip       int // matching requests to let through first
}

// New returns a fake with one server, a handful of channels and roles, and a bot holding an
// Administrator role.
func New() *Server {
	return &Server{
		Token:     Token,
		AppID:     AppID,
		AppName:   "Senpan Tea House",
		BotUserID: BotUserID,
		BotName:   "Senpan Tea House",
		GuildID:   GuildID,
		GuildName: "Senpan Tea House",
		Channels: []discord.Channel{
			{ID: "200000000000000001", Type: discord.ChannelCategory, Name: "Tea House", Position: 0},
			{ID: "200000000000000002", Type: discord.ChannelAnnouncement, Name: "announcements", ParentID: "200000000000000001", Position: 0},
			{ID: "200000000000000003", Type: discord.ChannelText, Name: "tea-rooms", ParentID: "200000000000000001", Position: 1},
			{ID: "200000000000000004", Type: discord.ChannelText, Name: "book-club", ParentID: "200000000000000001", Position: 2},
			{ID: "200000000000000005", Type: discord.ChannelText, Name: "test-channel", Position: 0},
			{ID: "200000000000000006", Type: 2, Name: "Voice Lounge", ParentID: "200000000000000001", Position: 3},
		},
		Roles: []discord.Role{
			{ID: GuildID, Name: "@everyone", Position: 0, Permissions: "0"},
			{ID: "300000000000000001", Name: "Staff", Color: 0xE53170, Position: 2, Permissions: "0", Mentionable: true},
			{ID: "300000000000000002", Name: "Event Pings", Color: 0x3498DB, Position: 1, Permissions: "0", Mentionable: true},
			{ID: "300000000000000003", Name: "Senpan Bot", Position: 3, Permissions: strconv.Itoa(1 << 3), Managed: true},
		},
		BotRoles: []string{"300000000000000003"},
		messages: map[string]*Message{},
		nonces:   map[string]string{},
		nextID:   400000000000000000,
	}
}

// Lock and Unlock guard the exported configuration while a test changes it mid-run.
func (s *Server) Lock()   { s.mu.Lock() }
func (s *Server) Unlock() { s.mu.Unlock() }

// Fail queues a failure for the next request whose "METHOD /path" starts with match (any
// request when match is ""): an HTTP status with a body.
func (s *Server) Fail(match string, status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, failure{match: match, status: status, body: body})
}

// FailAfter queues a failure like Fail, for the matching request after the next skip of
// them - the third post of a batch, say.
func (s *Server) FailAfter(match string, skip, status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, failure{match: match, status: status, body: body, skip: skip})
}

// RateLimit queues a 429 asking the client to wait retryAfter seconds.
func (s *Server) RateLimit(match string, retryAfter float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, failure{match: match, status: http.StatusTooManyRequests, retryAfter: retryAfter})
}

// ExhaustNext makes the next answer say its route's bucket is used up for d.
func (s *Server) ExhaustNext(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exhaust = d
}

// DeleteInDiscord removes a message as a member deleting it in Discord would: the bot's
// next edit or delete of it gets a 404.
func (s *Server) DeleteInDiscord(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.messages[id]; ok {
		m.Deleted = true
	}
}

// Messages is every message the bot has posted that is still up, oldest first.
func (s *Server) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Message
	for _, id := range s.order {
		if m := s.messages[id]; !m.Deleted {
			out = append(out, *m)
		}
	}
	return out
}

// Message returns one message by id, deleted or not.
func (s *Server) Message(id string) (Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.messages[id]
	if !ok {
		return Message{}, false
	}
	return *m, true
}

// Calls is every request the fake has answered, as "METHOD /path", in order.
func (s *Server) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

// Gateway reports what the gateway side has seen: identifies, resumes, and every presence
// it was given (identify and updates), as JSON.
func (s *Server) Gateway() (identifies, resumes int, presences []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.identifies, s.resumes, slices.Clone(s.presences)
}

// ReconnectAfter makes the gateway ask for a reconnect (op 7) once, after n heartbeats.
func (s *Server) ReconnectAfter(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconnectAfter = n
}

// Handler serves the fake's REST API and its gateway.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// The client dials the advertised address plus "/?v=10&encoding=json".
	mux.HandleFunc("GET /gateway", s.serveGateway)
	mux.HandleFunc("GET /gateway/{$}", s.serveGateway)
	mux.HandleFunc("GET /gateway/bot", s.rest(func(w http.ResponseWriter, r *http.Request) {
		scheme := "ws"
		if r.TLS != nil {
			scheme = "wss"
		}
		writeJSON(w, http.StatusOK, map[string]string{"url": scheme + "://" + r.Host + "/gateway"})
	}))
	mux.HandleFunc("GET /applications/@me", s.rest(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"id": s.AppID, "name": s.AppName,
			"bot": map[string]any{"id": s.BotUserID, "username": s.BotName, "bot": true}})
	}))
	mux.HandleFunc("GET /users/@me", s.rest(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"id": s.BotUserID, "username": s.BotName, "bot": true})
	}))
	mux.HandleFunc("GET /guilds/{g}", s.rest(func(w http.ResponseWriter, r *http.Request) {
		if !s.inGuild(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": s.GuildID, "name": s.GuildName, "owner_id": "1"})
	}))
	mux.HandleFunc("GET /guilds/{g}/channels", s.rest(func(w http.ResponseWriter, r *http.Request) {
		if !s.inGuild(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, s.Channels)
	}))
	mux.HandleFunc("GET /guilds/{g}/roles", s.rest(func(w http.ResponseWriter, r *http.Request) {
		if !s.inGuild(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, s.Roles)
	}))
	mux.HandleFunc("GET /guilds/{g}/members/{u}", s.rest(func(w http.ResponseWriter, r *http.Request) {
		if !s.inGuild(w, r) {
			return
		}
		if r.PathValue("u") != s.BotUserID {
			discordError(w, http.StatusNotFound, 10007, "Unknown Member")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"roles": s.BotRoles})
	}))
	// Looking a webhook up by its own token needs no bot token.
	mux.HandleFunc("GET /webhooks/{id}/{token}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		call := r.Method + " /webhooks/" + r.PathValue("id")
		s.calls = append(s.calls, call+"/{token}")
		for i, f := range s.failures {
			if f.match == "" || strings.HasPrefix(call, f.match) {
				s.failures = slices.Delete(s.failures, i, i+1)
				discordError(w, f.status, 0, "Failure injected")
				return
			}
		}
		hook, ok := s.Webhooks[r.PathValue("id")]
		if !ok || hook.Deleted {
			discordError(w, http.StatusNotFound, 10015, "Unknown Webhook")
			return
		}
		if hook.Token != r.PathValue("token") {
			discordError(w, http.StatusUnauthorized, 50027, "Invalid Webhook Token")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "name": hook.Name,
			"guild_id": hook.GuildID, "channel_id": hook.ChannelID, "type": 1})
	})
	mux.HandleFunc("DELETE /webhooks/{id}", s.rest(func(w http.ResponseWriter, r *http.Request) {
		hook, ok := s.Webhooks[r.PathValue("id")]
		if !ok || hook.Deleted {
			discordError(w, http.StatusNotFound, 10015, "Unknown Webhook")
			return
		}
		if hook.GuildID != s.GuildID {
			discordError(w, http.StatusForbidden, 50001, "Missing Access")
			return
		}
		hook.Deleted = true
		s.Webhooks[r.PathValue("id")] = hook
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /channels/{c}/messages", s.rest(s.createMessage))
	mux.HandleFunc("PATCH /channels/{c}/messages/{m}", s.rest(s.editMessage))
	mux.HandleFunc("DELETE /channels/{c}/messages/{m}", s.rest(s.deleteMessage))
	return mux
}

// rest wraps a REST handler: checks the token, records the call, applies a queued failure,
// and runs the handler under the lock.
func (s *Server) rest(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		call := r.Method + " " + r.URL.Path
		s.calls = append(s.calls, call)
		if !s.tokenOK(strings.TrimPrefix(r.Header.Get("Authorization"), "Bot ")) {
			discordError(w, http.StatusUnauthorized, 0, "401: Unauthorized")
			return
		}
		if s.exhaust > 0 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset-After", strconv.FormatFloat(s.exhaust.Seconds(), 'f', 3, 64))
			s.exhaust = 0
		}
		for i, f := range s.failures {
			if f.match != "" && !strings.HasPrefix(call, f.match) {
				continue
			}
			if f.skip > 0 {
				s.failures[i].skip--
				break
			}
			s.failures = slices.Delete(s.failures, i, i+1)
			if f.status == http.StatusTooManyRequests {
				writeJSON(w, f.status, map[string]any{"message": "You are being rate limited.", "retry_after": f.retryAfter, "global": false})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.status)
			_, _ = w.Write([]byte(f.body))
			return
		}
		h(w, r)
	}
}

// tokenOK checks a bot token; the caller holds the lock.
func (s *Server) tokenOK(token string) bool {
	if s.AnyToken {
		return token != ""
	}
	return token == s.Token
}

func (s *Server) inGuild(w http.ResponseWriter, r *http.Request) bool {
	if r.PathValue("g") != s.GuildID {
		discordError(w, http.StatusNotFound, 10004, "Unknown Guild")
		return false
	}
	return true
}

func (s *Server) postable(channelID string) bool {
	return slices.ContainsFunc(s.Channels, func(c discord.Channel) bool {
		return c.ID == channelID && (c.Type == discord.ChannelText || c.Type == discord.ChannelAnnouncement)
	})
}

type messageBody struct {
	Content         string                  `json:"content"`
	Embeds          []discord.Embed         `json:"embeds"`
	Components      []discord.Component     `json:"components"`
	AllowedMentions discord.AllowedMentions `json:"allowed_mentions"`
	Nonce           string                  `json:"nonce"`
	EnforceNonce    bool                    `json:"enforce_nonce"`
}

func (s *Server) createMessage(w http.ResponseWriter, r *http.Request) {
	channelID := r.PathValue("c")
	if !s.postable(channelID) {
		discordError(w, http.StatusNotFound, 10003, "Unknown Channel")
		return
	}
	var b messageBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		discordError(w, http.StatusBadRequest, 50109, "The request body contains invalid JSON.")
		return
	}
	if len(b.Nonce) > discord.MaxNonce {
		discordError(w, http.StatusBadRequest, 50035, "Invalid Form Body")
		return
	}
	if b.Content == "" && len(b.Embeds) == 0 {
		discordError(w, http.StatusBadRequest, 50006, "Cannot send an empty message")
		return
	}
	// A repeated nonce answers with the message it already made, as Discord does.
	if b.EnforceNonce && b.Nonce != "" {
		if id, ok := s.nonces[channelID+"/"+b.Nonce]; ok {
			writeJSON(w, http.StatusOK, map[string]string{"id": id, "channel_id": channelID})
			return
		}
	}
	s.nextID++
	id := strconv.FormatInt(s.nextID, 10)
	s.messages[id] = &Message{ID: id, ChannelID: channelID, Content: b.Content, Embeds: b.Embeds,
		Components: b.Components, AllowedMentions: b.AllowedMentions, Nonce: b.Nonce}
	s.order = append(s.order, id)
	if b.EnforceNonce && b.Nonce != "" {
		s.nonces[channelID+"/"+b.Nonce] = id
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "channel_id": channelID})
}

func (s *Server) found(w http.ResponseWriter, r *http.Request) (*Message, bool) {
	m, ok := s.messages[r.PathValue("m")]
	if !ok || m.Deleted || m.ChannelID != r.PathValue("c") {
		discordError(w, http.StatusNotFound, 10008, "Unknown Message")
		return nil, false
	}
	return m, true
}

func (s *Server) editMessage(w http.ResponseWriter, r *http.Request) {
	m, ok := s.found(w, r)
	if !ok {
		return
	}
	var b messageBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		discordError(w, http.StatusBadRequest, 50109, "The request body contains invalid JSON.")
		return
	}
	m.Content, m.Embeds, m.Components, m.AllowedMentions = b.Content, b.Embeds, b.Components, b.AllowedMentions
	m.Edits++
	writeJSON(w, http.StatusOK, map[string]string{"id": m.ID, "channel_id": m.ChannelID})
}

func (s *Server) deleteMessage(w http.ResponseWriter, r *http.Request) {
	m, ok := s.found(w, r)
	if !ok {
		return
	}
	m.Deleted = true
	w.WriteHeader(http.StatusNoContent)
}

// serveGateway speaks enough of the gateway protocol: hello, identify or resume, heartbeats,
// presence updates, and an optional forced reconnect.
func (s *Server) serveGateway(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = ws.CloseNow() }()
	// The request's context is not for use once the connection is taken over.
	ctx := context.Background()
	send := func(v any) { _ = wsjson.Write(ctx, ws, v) }
	send(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 80}})
	var first struct {
		Op int             `json:"op"`
		D  json.RawMessage `json:"d"`
	}
	if err := wsjson.Read(ctx, ws, &first); err != nil {
		return
	}
	var d struct {
		Token    string          `json:"token"`
		Presence json.RawMessage `json:"presence"`
	}
	_ = json.Unmarshal(first.D, &d)
	s.mu.Lock()
	if !s.tokenOK(d.Token) {
		s.mu.Unlock()
		_ = ws.Close(4004, "Authentication failed.")
		return
	}
	switch first.Op {
	case 2:
		s.identifies++
		s.presences = append(s.presences, string(d.Presence))
	case 6:
		s.resumes++
	}
	appID, userID := s.AppID, s.BotUserID
	s.mu.Unlock()
	resumeURL := "ws://" + r.Host + "/gateway"
	if first.Op == 2 {
		send(map[string]any{"op": 0, "s": 1, "t": "READY", "d": map[string]any{
			"session_id": "fake-session", "resume_gateway_url": resumeURL,
			"user": map[string]any{"id": userID}, "application": map[string]any{"id": appID},
		}})
	} else {
		send(map[string]any{"op": 0, "s": 2, "t": "RESUMED", "d": map[string]any{}})
	}
	for {
		var p struct {
			Op int             `json:"op"`
			D  json.RawMessage `json:"d"`
		}
		if err := wsjson.Read(ctx, ws, &p); err != nil {
			return
		}
		switch p.Op {
		case 3:
			s.mu.Lock()
			s.presences = append(s.presences, string(p.D))
			s.mu.Unlock()
		case 1:
			s.mu.Lock()
			s.heartbeats++
			beats, after := s.heartbeats, s.reconnectAfter
			if after > 0 && beats >= after {
				s.reconnectAfter = 0
			}
			s.mu.Unlock()
			send(map[string]any{"op": 11})
			if after > 0 && beats >= after {
				send(map[string]any{"op": 7})
				return
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func discordError(w http.ResponseWriter, status, code int, message string) {
	writeJSON(w, status, map[string]any{"message": message, "code": code})
}

// String describes a message for test failure output.
func (m Message) String() string {
	return fmt.Sprintf("%s in %s: %q (%d embeds, edits %d)", m.ID, m.ChannelID, m.Content, len(m.Embeds), m.Edits)
}
