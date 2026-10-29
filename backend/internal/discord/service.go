package discord

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Config is what the Service runs with. An empty Token switches Discord off.
type Config struct {
	Token    string
	GuildID  string
	Presence Presence
}

// Service is the app's bot: the REST client for the current token, the gateway session that
// keeps it online, and a short cache of the server's channels and roles for the pickers.
//
// It knows nothing about where its configuration comes from; the server reads settings and
// calls Configure, at startup and whenever an admin changes them. A token change rebuilds
// the client and restarts the gateway without a process restart.
type Service struct {
	base       string
	gatewayURL string

	// configMu serializes Configure, so two admin saves can't interleave a gateway restart.
	configMu sync.Mutex

	mu     sync.Mutex
	cfg    Config
	client *Client
	gw     *Gateway
	root   context.Context
	stopGW context.CancelFunc
	gwDone chan struct{}

	// presence is read by the gateway goroutine; kept apart from mu so a gateway being
	// stopped can never wait on a lock its stopper holds.
	presence atomic.Pointer[Presence]

	// fetchMu serializes directory fetches (at most one set of calls to Discord at a time).
	// cacheMu guards only the cached value and is never held across a call, because the
	// gateway goroutine takes it to invalidate - and a gateway goroutine left waiting on a
	// slow fetch misses heartbeats and loses its session.
	fetchMu  sync.Mutex
	cacheMu  sync.Mutex
	cache    *Directory
	cachedAt time.Time
	// cacheGen counts invalidations, so a fetch that started before one doesn't store a
	// stale answer after it.
	cacheGen uint64
}

// directoryTTL is how long the server's channels and roles are trusted. The gateway drops
// the cache early when they change, so this only bounds staleness while it is offline.
const directoryTTL = 30 * time.Second

// NewService returns a stopped, unconfigured service talking to base (DefaultAPI when "").
// gatewayURL overrides the gateway address Discord advertises; "" asks Discord.
func NewService(base, gatewayURL string) *Service {
	s := &Service{base: base, gatewayURL: gatewayURL}
	s.presence.Store(&Presence{})
	return s
}

// Configure applies a configuration. A changed token rebuilds the client and restarts the
// gateway (when running); a changed server or token drops the cached directory; a changed
// presence is told to the open session.
func (s *Service) Configure(cfg Config) {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	s.mu.Lock()
	old := s.cfg
	s.cfg = cfg
	s.mu.Unlock()
	p := cfg.Presence
	s.presence.Store(&p)

	if cfg.Token != old.Token || cfg.GuildID != old.GuildID {
		s.Invalidate()
	}
	if cfg.Token != old.Token {
		s.stopGateway()
		s.mu.Lock()
		s.client = New(cfg.Token, s.base)
		s.mu.Unlock()
		s.startGateway()
		return
	}
	if cfg.Presence != old.Presence {
		s.mu.Lock()
		gw := s.gw
		s.mu.Unlock()
		gw.RefreshPresence()
	}
}

// Run keeps the gateway running for the current token until ctx ends, then stops it.
func (s *Service) Run(ctx context.Context) {
	s.mu.Lock()
	s.root = ctx
	s.mu.Unlock()
	s.startGateway()
	<-ctx.Done()
	s.stopGateway()
}

func (s *Service) startGateway() {
	s.mu.Lock()
	if s.root == nil || s.root.Err() != nil || s.client == nil || s.stopGW != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(s.root)
	gw := NewGateway(s.client)
	gw.URL = s.gatewayURL
	gw.Intents = IntentGuilds
	gw.Presence = func() Presence { return *s.presence.Load() }
	gw.Handle = s.handle
	done := make(chan struct{})
	s.gw, s.stopGW, s.gwDone = gw, cancel, done
	s.mu.Unlock()
	go func() {
		defer close(done)
		gw.Run(ctx)
	}()
}

func (s *Service) stopGateway() {
	s.mu.Lock()
	cancel, done := s.stopGW, s.gwDone
	s.stopGW, s.gwDone, s.gw = nil, nil, nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

// handle drops the directory cache when the server's channels or roles change. It runs on
// the gateway goroutine, so it does no I/O.
func (s *Service) handle(event string, _ json.RawMessage) {
	switch event {
	case "CHANNEL_CREATE", "CHANNEL_UPDATE", "CHANNEL_DELETE",
		"GUILD_ROLE_CREATE", "GUILD_ROLE_UPDATE", "GUILD_ROLE_DELETE", "GUILD_UPDATE":
		s.Invalidate()
	}
}

// Base is the REST base the service talks to (Discord's own, or a local fake).
func (s *Service) Base() string {
	if s.base == "" {
		return DefaultAPI
	}
	return s.base
}

// Probe checks a token without adopting it: who the bot is and which application it
// belongs to. ErrUnauthorized means Discord refused the token.
func (s *Service) Probe(ctx context.Context, token string) (User, Application, error) {
	c := New(token, s.base)
	if c == nil {
		return User{}, Application{}, ErrUnauthorized
	}
	me, err := c.Me(ctx)
	if err != nil {
		return User{}, Application{}, err
	}
	app, err := c.Application(ctx)
	return me, app, err
}

// Client is the REST client for the current token, or nil when Discord is off.
func (s *Service) Client() *Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client
}

// Enabled reports whether a token is configured.
func (s *Service) Enabled() bool { return s.Client() != nil }

// GuildID is the server the bot works in.
func (s *Service) GuildID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.GuildID
}

// GatewayStatus reports the live session, or an offline status when there is none.
func (s *Service) GatewayStatus() GatewayStatus {
	s.mu.Lock()
	gw := s.gw
	s.mu.Unlock()
	return gw.Status()
}

// Directory is what the bot knows about itself and its server.
type Directory struct {
	Bot         User
	Application Application
	// InGuild is false when the bot has not been invited to the configured server (or the
	// server id is wrong); Guild, Channels, Roles and BotRoles are then empty.
	InGuild  bool
	Guild    Guild
	Channels []Channel
	Roles    []Role
	BotRoles []string
}

// IsAdmin reports whether the bot holds Administrator in its server.
func (d Directory) IsAdmin() bool {
	return d.InGuild && HasAdministrator(d.Guild.ID, d.BotRoles, d.Roles)
}

// ErrOff is returned by Directory when no token is configured.
var ErrOff = errors.New("discord: not configured")

// Directory returns the bot's identity and its server's channels and roles, from a cache
// trusted for directoryTTL. Only a full answer is cached; an error (a bad token, Discord
// unreachable) is returned and tried again on the next call.
func (s *Service) Directory(ctx context.Context) (Directory, error) {
	if d, ok := s.cached(); ok {
		return d, nil
	}
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()
	// Another caller may have fetched while this one waited.
	if d, ok := s.cached(); ok {
		return d, nil
	}
	s.cacheMu.Lock()
	gen := s.cacheGen
	s.cacheMu.Unlock()

	c := s.Client()
	if c == nil {
		return Directory{}, ErrOff
	}
	guildID := s.GuildID()
	var d Directory
	var err error
	if d.Bot, err = c.Me(ctx); err != nil {
		return Directory{}, err
	}
	if d.Application, err = c.Application(ctx); err != nil {
		return Directory{}, err
	}
	if guildID != "" {
		d.Guild, err = c.Guild(ctx, guildID)
		switch {
		case errors.Is(err, ErrNotFound), errors.Is(err, ErrForbidden):
			// Not invited (or a wrong id): a real answer, cached like any other.
		case err != nil:
			return Directory{}, err
		default:
			d.InGuild = true
			if d.Channels, err = c.GuildChannels(ctx, guildID); err != nil {
				return Directory{}, err
			}
			if d.Roles, err = c.GuildRoles(ctx, guildID); err != nil {
				return Directory{}, err
			}
			member, err := c.Member(ctx, guildID, d.Bot.ID)
			if err != nil {
				return Directory{}, err
			}
			d.BotRoles = member.Roles
		}
	}
	s.cacheMu.Lock()
	if s.cacheGen == gen {
		s.cache, s.cachedAt = &d, time.Now()
	}
	s.cacheMu.Unlock()
	return d, nil
}

func (s *Service) cached() (Directory, bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.cache != nil && time.Since(s.cachedAt) < directoryTTL {
		return *s.cache, true
	}
	return Directory{}, false
}

// Invalidate drops the cached directory, so the next read asks Discord. Safe to call from
// the gateway goroutine: it never waits on a fetch.
func (s *Service) Invalidate() {
	s.cacheMu.Lock()
	s.cache = nil
	s.cacheGen++
	s.cacheMu.Unlock()
}
