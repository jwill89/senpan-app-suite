// Package server wires up the HTTP API, middleware, and JSON helpers.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"

	"app-suite/internal/anilist"
	"app-suite/internal/api"
	"app-suite/internal/auth"
	"app-suite/internal/discord"
	"app-suite/internal/game"
	"app-suite/internal/live"
	"app-suite/internal/media"
	"app-suite/internal/model"
	"app-suite/internal/store"
	"app-suite/internal/version"
)

// Server holds all dependencies and implements http.Handler.
type Server struct {
	store       *store.Store
	game        *game.Service
	live        *live.Hub
	sessions    *scs.SessionManager
	sessHandler http.Handler // pre-built session middleware wrapping mux
	// apiHandler is everything under /api past the CORS and CSRF checks: the caching
	// baseline, then sessHandler (apilayer.go).
	apiHandler http.Handler
	webRoot    string
	// allowedOrigins is the CORS allowlist (exact origin strings). Normally
	// empty - the SPA and API are same-origin - so no CORS headers are sent.
	allowedOrigins map[string]bool
	mux            *http.ServeMux
	// siteMux answers everything outside /api/: GET /healthz, and the SPA (site,
	// set by SetSite) for the rest. It sits outside the API's session, CSRF and
	// access-log layers.
	siteMux       *http.ServeMux
	site          http.Handler
	limiter       *rateLimiter // failed-login brute-force limiter
	regLimiter    *rateLimiter // registration-rate limiter (mass-signup abuse)
	raffleLimiter *rateLimiter // public raffle-entry limiter (entry flooding)
	// Separate from raffleLimiter on purpose: a search is what somebody does right
	// before signing up, so the two must not share a budget.
	raffleLookupLimiter *rateLimiter
	cardReqLimiter      *rateLimiter // public custom-card-request limiter (request flooding)
	rallyLimiter        *rateLimiter // public stamp-rally sign-up + link-lookup limiter
	// Wrong stamp passwords. Counted only on a MISS, so a participant collecting
	// stamps normally never touches it; a script guessing short human-typed
	// passwords ("alpha", "bravo") runs out quickly.
	stampGuessLimiter *rateLimiter
	// Garapon draws. Every other public mutating path has a limiter and this one
	// had none, even though it is the only IRREVERSIBLE public action in the app:
	// a draw is recorded against the player's allowance and cannot be undone. The
	// token is the capability, so anyone who obtains one - and a stamp-card link
	// doubles as one - could spend a whole allowance in a single burst. Counted on
	// every draw, not just failures: a legitimate player has a handful of draws
	// total, so the ceiling is only reachable by a script.
	garaponDrawLimiter *rateLimiter
	// Public passkey-login BEGIN. Separate from limiter on purpose: every begin
	// stores a WebAuthn challenge in the session store, so it needs a budget of its
	// own, but counting it against the 5-per-15-minutes login limiter would lock a
	// user out of password login for merely opening the passkey prompt a few times.
	passkeyBeginLimiter *rateLimiter
	// The festival dashboard's read. It matches raffle entries on a whole name, so it
	// is bounded like the other name-keyed reads, but loosely, since the page reads it
	// on every visit and after each entry.
	dashboardLimiter *rateLimiter
	// Public Ema plaque submissions (designs and artist requests share it). A visitor
	// sends one or two a festival, so this only stops a flood.
	emaLimiter *rateLimiter
	// discord sends every outbound Discord post. Its dev-safety valves (see
	// SetWebhookPolicy) are off in production, where a post goes to the webhook the
	// feature configured.
	discord *discord.Sender
	// anilist looks titles up for the book clubs.
	anilist *anilist.Client
	// Cloudflare Turnstile bot check on the admin login. Disabled (verification
	// skipped) when turnstileSecret is empty - see SetTurnstile / turnstile.go.
	turnstileSecret  string
	turnstileSiteKey string
	// logFile is the path to the rotating JSON log file the admin log viewer
	// (GET /api/logs) tails. Empty disables the viewer. Injected via SetLogFile.
	logFile string
	// images, carrd and fonts are the uploads under webRoot; the operations in
	// internal/api and the startup migrations below share them.
	images *media.Images
	carrd  *media.Carrd
	fonts  *media.Fonts

	// Consecutive failed Discord posts per announcement id, used to stop the
	// scheduler retrying a permanently-broken webhook every 30 seconds forever (see
	// postDueAnnouncements). In memory on purpose: it is a back-off, not a record,
	// and a restart is a perfectly good reason to try again. Guarded by postFailMu.
	postFailMu sync.Mutex
	postFails  map[int64]int
}

// RunAutoDrawScheduler makes the game's automatic draws until ctx ends (game.Run).
func (s *Server) RunAutoDrawScheduler(ctx context.Context) {
	s.game.Run(ctx)
}

// SetTurnstile enables the Cloudflare Turnstile bot check on the login form.
// secret is the server-side secret key (kept private); siteKey is the public key
// served to the browser. An empty secret leaves Turnstile disabled, so the test
// harness and local dev (no keys) keep working without a challenge.
func (s *Server) SetTurnstile(secret, siteKey string) {
	s.turnstileSecret = strings.TrimSpace(secret)
	s.turnstileSiteKey = strings.TrimSpace(siteKey)
}

// SetWebhookPolicy installs the outbound-Discord safety valves used when the
// server runs against non-production data. A development database is usually a
// copy of the live one, so it carries REAL webhook URLs: without these, starting a
// local server is enough for the announcement scheduler to post to production.
//
//   - dryRun blocks every outbound post and logs it instead. Strongest option:
//     the process becomes incapable of posting anywhere.
//   - override redirects every outbound post to one URL (a test channel),
//     whatever the feature or database row says. Use it to exercise the real
//     posting path without touching production channels.
//
// dryRun wins when both are set. Both are off in production.
func (s *Server) SetWebhookPolicy(override string, dryRun bool) {
	s.discord.Override = strings.TrimSpace(override)
	s.discord.DryRun = dryRun
}

// SetLogFile tells the admin log viewer (GET /api/logs) which rotating JSON log
// file to tail. Empty leaves the viewer with nothing to read. Injected from main
// so it matches the -log-file the process actually writes to.
func (s *Server) SetLogFile(path string) {
	s.logFile = strings.TrimSpace(path)
}

// New creates a Server, registers all API routes, and returns it. allowedOrigins
// is the CORS allowlist (exact origin strings); pass nil/empty for a same-origin
// deployment, which sends no CORS headers.
//
// Sessions live in the database (sessionStore), so there is nothing to sign and no
// secret to configure: a session is an opaque random token the server looks up
// by its hash, and it outlives a restart.
//
// ctx is used only for the startup migrations below that touch the database; the
// Server keeps no context of its own. hub is the live feed the changes are published
// to and the event streams read.
func New(ctx context.Context, st *store.Store, hub *live.Hub, webRoot string, allowedOrigins []string) *Server {
	sm := scs.New()
	sm.Store = newSessionStore(st.DB())
	sm.Lifetime = 24 * time.Hour
	sm.Cookie.Path = "/"
	sm.Cookie.HttpOnly = true
	sm.Cookie.Secure = true
	sm.Cookie.SameSite = http.SameSiteLaxMode

	origins := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" {
			origins[o] = true
		}
	}

	s := &Server{
		store:          st,
		discord:        &discord.Sender{},
		anilist:        anilist.New(),
		game:           game.New(st.ReadDB(), st.DB(), hub),
		live:           hub,
		sessions:       sm,
		webRoot:        webRoot,
		images:         media.NewImages(webRoot),
		carrd:          media.NewCarrd(webRoot),
		fonts:          media.NewFonts(webRoot, media.StoreSettings(st)),
		allowedOrigins: origins,
		mux:            http.NewServeMux(),
		limiter:        newRateLimiter(5, 15*time.Minute),  // 5 failed logins per 15 minutes
		regLimiter:     newRateLimiter(5, time.Hour),       // 5 registration attempts per hour
		raffleLimiter:  newRateLimiter(20, 10*time.Minute), // 20 raffle entries per 10 minutes per IP
		// Reads, and a legitimate entrant may try a few spellings.
		raffleLookupLimiter: newRateLimiter(30, 10*time.Minute),
		cardReqLimiter:      newRateLimiter(10, 10*time.Minute), // 10 custom-card requests per 10 minutes per IP
		// Covers sign-up AND link lookup: the lookup is the enumeration risk (guessing
		// names to harvest links), so both share one budget per IP. Generous enough for
		// a participant who signs up for a couple of rallies and re-checks their links.
		rallyLimiter:      newRateLimiter(15, 10*time.Minute),
		stampGuessLimiter: newRateLimiter(10, 10*time.Minute),
		// A generous allowance is still far below a burst: real garapons issue a
		// few draws per player, and a player who draws them one at a time while
		// watching the animation stays well under this.
		garaponDrawLimiter: newRateLimiter(20, 10*time.Minute),
		postFails:          make(map[int64]int),
		// Generous: a real user may open the passkey prompt, cancel, pick the wrong
		// authenticator and retry several times in a sitting. It exists to bound
		// unauthenticated session-store growth, not to police login attempts.
		passkeyBeginLimiter: newRateLimiter(30, 10*time.Minute),
		dashboardLimiter:    newRateLimiter(120, 10*time.Minute),
		emaLimiter:          newRateLimiter(10, 10*time.Minute),
	}

	// The operations already on huma (internal/api), on the same mux: they share its
	// session, CSRF and access-log layers with the routes still registered by hand.
	api.New(s.mux, api.Deps{
		Store:             st,
		Discord:           s.discord,
		AniList:           s.anilist,
		Version:           version.Version,
		Live:              hub,
		TurnstileSiteKey:  func() string { return s.turnstileSiteKey },
		LogFile:           func() string { return s.logFile },
		Sessions:          sm,
		SignInLimit:       s.limiter,
		RegisterLimit:     s.regLimiter,
		PasskeyLimit:      s.passkeyBeginLimiter,
		CardRequestLimit:  s.cardReqLimiter,
		RaffleEntryLimit:  s.raffleLimiter,
		RaffleLookupLimit: s.raffleLookupLimiter,
		GaraponDrawLimit:  s.garaponDrawLimiter,
		StampGuessLimit:   s.stampGuessLimiter,
		RallyLimit:        s.rallyLimiter,
		DashboardLimit:    s.dashboardLimiter,
		EmaLimit:          s.emaLimiter,
		WebRoot:           webRoot,
		Images:            s.images,
		Carrd:             s.carrd,
		Fonts:             s.fonts,
		Turnstile: func(ctx context.Context, response, ip string) bool {
			return !s.turnstileEnabled() || s.verifyTurnstile(ctx, response, ip)
		},
		Game:         s.game,
		CardsChanged: s.game.CardsChanged,
	})
	// Everything outside /api/: GET /healthz and the SPA (site.go).
	s.siteRoutes()
	// Upgrade the image-category manifest to the current schema (folds the
	// formerly hardcoded permanent categories into the manifest; runs once).
	s.images.MigrateManifest()
	// One-time, idempotent migration of legacy announcement images into the new
	// announcements_main category dir (safe to run on every startup).
	s.images.MigrateAnnouncementImages()
	// Seed the built-in flourish SVGs into the Flourishes image category so they
	// are pickable in the theme flourish selectors (idempotent).
	s.images.SeedFlourishes()
	// Upgrade pre-group font metadata (file-keyed entries, the old global
	// origin allowlist), then reconcile every font's WOFF2 conversion -
	// backfilling missing ones and sweeping stale copies. Both idempotent;
	// conversion failures log and fall back to serving an uploaded format.
	s.fonts.MigrateMeta(ctx)
	s.fonts.MigrateDerivatives()
	// LoadAndSave (outer) populates the session, then withUserCache adds a
	// per-request memo so currentUser hits the DB at most once per request.
	s.sessHandler = sm.LoadAndSave(s.withUserCache(s.withActor(s.withWho(http.HandlerFunc(s.routeAPI)))))
	s.apiHandler = withCaching(s.sessHandler)
	return s
}

// userCacheCtxKey is the context key under which each request carries its
// per-request user memo.
type userCacheCtxKey struct{}

// userCache memoizes currentUser for the lifetime of a single request, so an
// operation that asks who is signed in more than once
// loads the account from the store once instead of per call.
type userCache struct {
	loaded bool
	user   *model.User
}

// withWho tells the operations on huma who is signed in (api.WithWho): the same
// account currentUser answers, looked up only if an operation asks. With it goes
// what an operation can't read for itself (api.WithRequest): the client's address,
// the site's, and whether a token rather than a session signs the caller in.
func (s *Server) withWho(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := api.WithWho(r.Context(), func() *model.User { return s.currentUser(r) })
		ctx = api.WithRequest(ctx, api.Request{
			IP:      clientIP(r),
			Site:    s.siteBaseURL(r),
			ByToken: s.sessionUserID(r) == 0 && bearerToken(r) != "",
			Account: func(ctx context.Context) *model.User { return s.callerAfresh(ctx, r) },
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// withUserCache injects an empty per-request user memo into the request context.
func (s *Server) withUserCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), userCacheCtxKey{}, &userCache{})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ServeHTTP applies CORS middleware, logs the request, then dispatches to the router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS: only echo the Origin (and allow credentials) for explicitly
	// allow-listed cross-origin sites. The SPA and API are same-origin in both
	// prod (Apache) and dev (Vite proxies /api), so the allowlist is normally
	// empty and these headers are simply not sent - the browser doesn't consult
	// CORS for same-origin requests. This replaces a previous "reflect ANY Origin
	// with credentials" policy, under which any website could make credentialed
	// cross-origin requests to the API.
	if origin := r.Header.Get("Origin"); origin != "" && s.allowedOrigins[origin] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Vary", "Origin")
	}

	if r.Method == http.MethodOptions {
		s.teaRoomPreflight(w, r)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Everything outside /api/ - the SPA and its files, and GET /healthz - skips the
	// API's session, CSRF and access-log layers: a page load is dozens of asset
	// requests, which Apache served (and logged) before 4.0.0.
	if r.URL.Path != "/api" && !strings.HasPrefix(r.URL.Path, "/api/") {
		s.siteMux.ServeHTTP(w, r)
		return
	}

	// CSRF defense-in-depth: reject a cross-origin state-changing request that
	// rides an ambient session cookie, layered on top of the SameSite=Lax cookie.
	if !s.checkCSRF(w, r) {
		return
	}

	start := time.Now()
	rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

	// Carry a per-request actor holder through the handler chain so the access log
	// (which runs out here, outside the session middleware) can name who made the
	// request. withActor fills it in from inside, where the session/token resolve.
	actor := &requestActor{kind: "anon"}
	r = r.WithContext(context.WithValue(r.Context(), actorCtxKey{}, actor))

	// Panic recovery: a handler panic must not tear down the connection silently.
	// Recover here - OUTSIDE the session middleware but INSIDE ServeHTTP - so the
	// stack is logged, the client still gets a 500 JSON, and control falls through
	// to the access-log emit below (which then records status 500 instead of the
	// line being lost). Wrap with SCS session middleware so session data is
	// available in handlers.
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("handler panic recovered",
					"panic", rec,
					"method", r.Method,
					"path", redactSensitivePath(r.URL.Path),
					"stack", string(debug.Stack()))
				if rw.wroteHeader {
					// Response already started; can't rewrite the status line, but
					// flag the failure for the access log below.
					rw.status = http.StatusInternalServerError
				} else {
					writeProblem(rw, http.StatusInternalServerError, "Something went wrong on the server.")
				}
			}
		}()
		s.apiHandler.ServeHTTP(rw, r)
	}()
	duration := time.Since(start)

	// After a successful write to one of the pages' data, the staff whose pages show
	// it hear that it changed and read it again over REST, which applies their own
	// permission guard (resource_changed). The operations with events of their own
	// (the game, cards, patterns, themes, settings) and the public, sign-in and
	// self-service paths are left out by adminMutationResource. A write to an account
	// or a sign-out has every staff stream check its account again at once.
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		if rw.status >= 200 && rw.status < 300 {
			if resource, ok := adminMutationResource(r.URL.Path); ok {
				s.resourceChanged(resource)
			}
			if accountMutation(r.URL.Path) {
				s.live.RecheckStaff()
			}
		}
	}

	// A 500 whose client had already gone is not a failure of ours: the handler's
	// queries were cancelled with the request, and the operation said so at
	// debug. The line still records the request, at Info and marked as canceled,
	// so a dropped phone connection doesn't read as an outage.
	canceled := rw.status >= 500 && r.Context().Err() != nil
	level := slog.LevelInfo
	switch {
	case canceled:
	case rw.status >= 500:
		level = slog.LevelError
	case rw.status >= 400:
		level = slog.LevelWarn
	}
	ip := logClientIP(r)
	// Redact capability-token path segments so bearer tokens (garapon/stamp-card
	// draw links, font-kit tokens) never land in the rotating log, the /api/logs
	// viewer, or the admin WS tail.
	path := redactSensitivePath(r.URL.Path)
	// actor names who made the request: a logged-in account (session or plugin
	// PAT), a Cloudflare-verified bot, or anonymous. `auth` is always present so
	// every line is classifiable; `user`/`bot` only when they apply.
	attrs := []any{
		"method", r.Method,
		"path", path,
		"status", rw.status,
		"duration", duration.Round(time.Microsecond),
		"ip", ip,
		"auth", actor.kind,
	}
	if canceled {
		attrs = append(attrs, "canceled", true)
	}
	if actor.user != "" {
		attrs = append(attrs, "user", actor.user)
	}
	if actor.bot != "" {
		attrs = append(attrs, "bot", actor.bot)
	}
	slog.Log(r.Context(), level, "http request", attrs...)
	// When DEBUG is on (live-toggleable), emit a richer companion line for every
	// request. Guarded so the header lookups are skipped entirely at INFO+. Query,
	// path, and Referer are all scrubbed of capability tokens (tokens can ride the
	// query as the PAT fallback, and the Referer when navigating from a token page).
	if slog.Default().Enabled(r.Context(), slog.LevelDebug) {
		dattrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", path),
			slog.String("query", redactTokenQuery(r.URL.RawQuery)),
			slog.Int("status", rw.status),
			slog.String("ua", r.UserAgent()),
			slog.String("referer", redactReferer(r.Referer())),
			slog.String("ip", ip),
			slog.String("auth", actor.kind),
		}
		if actor.user != "" {
			dattrs = append(dattrs, slog.String("user", actor.user))
		}
		if actor.bot != "" {
			dattrs = append(dattrs, slog.String("bot", actor.bot))
		}
		slog.LogAttrs(r.Context(), slog.LevelDebug, "request detail", dattrs...)
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code. Unwrap and
// Flush let an event stream flush through it, and set its write deadline
// (http.ResponseController).
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rw *responseWriter) Unwrap() http.ResponseWriter { return rw.ResponseWriter }

func (rw *responseWriter) Flush() {
	if !rw.wroteHeader {
		rw.wroteHeader = true
	}
	_ = http.NewResponseController(rw.ResponseWriter).Flush()
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.wroteHeader = true
	}
	return rw.ResponseWriter.Write(b)
}

// clientGone reports whether err only says that the work's context was canceled -
// the request's client went away, or the process is shutting down under a
// background loop - so a query was refused or stopped part-way. database/sql
// reports the first as context.Canceled, and the driver the second too;
// store.Interrupted catches SQLite's interrupt should it arrive as the driver's
// own error instead.
func clientGone(err error) bool {
	return errors.Is(err, context.Canceled) || store.Interrupted(err)
}

// errorLevel is the level for logging a failure: Debug when the work was only
// canceled (clientGone), Error otherwise.
func errorLevel(err error) slog.Level {
	if clientGone(err) {
		return slog.LevelDebug
	}
	return slog.LevelError
}

// -- Auth helpers ------------------------------------------------------------

// sessionUserID returns the logged-in user's id from the session, or 0 if none.
// The id is stored as int64 (see handleAuthAction), so it is read back via a
// type assertion rather than scs's GetInt (which only matches plain int).
func (s *Server) sessionUserID(r *http.Request) int64 {
	if id, ok := s.sessions.Get(r.Context(), auth.SessionUserID).(int64); ok {
		return id
	}
	return 0
}

// sessionUserEpoch returns the password epoch the cookie session was minted with
// (0 when absent - e.g. a session predating this feature, which matches the
// default password_epoch of an unchanged account). Compared against the user's
// current PasswordEpoch to invalidate sessions after a password change/reset.
func (s *Server) sessionUserEpoch(r *http.Request) int64 {
	if e, ok := s.sessions.Get(r.Context(), auth.SessionEpoch).(int64); ok {
		return e
	}
	return 0
}

// checkCSRF is a defense-in-depth CSRF guard layered on top of the SameSite=Lax
// session cookie. It scrutinizes only cookie-authenticated, state-changing
// requests - the shape a cross-site page could drive using the victim's ambient
// cookie. Bearer-token (plugin) and cookie-less requests carry no ambient
// credential and are exempt, as are safe methods. For a checked request it
// requires the Origin header (when the browser sends one) to be same-host or an
// explicitly allow-listed cross-origin site; this blocks a malicious page,
// including one served from a sibling same-site subdomain that SameSite alone
// would permit. Returns false and writes a 403 when the request is blocked.
func (s *Server) checkCSRF(w http.ResponseWriter, r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return true // safe/idempotent methods can't mutate state
	}
	// Only ambient-cookie auth is CSRF-exposed. No session cookie (public and
	// bearer-token requests) means there's nothing for an attacker to ride.
	if _, err := r.Cookie(s.sessions.Cookie.Name); err != nil {
		return true
	}
	if bearerToken(r) != "" {
		return true // authenticated by PAT, not the cookie
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Browsers send Origin on cross-origin mutations, so an absent Origin
		// isn't the attack shape; the SameSite=Lax cookie remains the primary
		// defense. Allow it rather than break a non-browser same-origin client.
		return true
	}
	if s.allowedOrigins[origin] {
		return true // operator opted this cross-origin site in (see CORS above)
	}
	if u, err := url.Parse(origin); err == nil && sameHost(u.Hostname(), r.Host) {
		return true
	}
	slog.Warn("blocked cross-origin state-changing request",
		"origin", origin, "host", r.Host, "method", r.Method, "path", r.URL.Path)
	writeProblem(w, http.StatusForbidden, "Cross-origin request blocked.")
	return false
}

// sameHost reports whether an Origin's hostname matches the request's host. It
// compares hostnames (port-stripped) so a default-vs-explicit port - e.g. the dev
// setup's :5173 origin against a :8080 host, both "localhost" - isn't a false
// mismatch, while a different host (including a sibling subdomain) still fails.
func sameHost(originHost, requestHost string) bool {
	if h, _, err := net.SplitHostPort(requestHost); err == nil {
		requestHost = h
	}
	return strings.EqualFold(originHost, requestHost)
}

// callerAfresh is the account behind r read again now, as loadCurrentUser reads it
// but past the request's memo and its session as loaded when the request began: the
// session is loaded from the store again, so one signed out or expired since signs
// nobody in, and a token is looked up again, so one revoked since signs nobody in.
// An open event stream checks its account with it.
func (s *Server) callerAfresh(ctx context.Context, r *http.Request) *model.User {
	if cookie, err := r.Cookie(s.sessions.Cookie.Name); err == nil {
		// A context that already holds a session is answered with that session, so
		// the store is read from one that holds none.
		lctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sctx, err := s.sessions.Load(lctx, cookie.Value)
		if err != nil {
			return nil
		}
		if id, ok := s.sessions.Get(sctx, auth.SessionUserID).(int64); ok && id != 0 {
			u := s.account(ctx, id)
			if u == nil || !u.IsActive {
				return nil
			}
			if epoch, _ := s.sessions.Get(sctx, auth.SessionEpoch).(int64); epoch != u.PasswordEpoch {
				return nil
			}
			return u
		}
	}
	return s.userForToken(ctx, bearerToken(r))
}

// currentUser loads the active account for the current session, or nil if the
// request is unauthenticated, the user was deleted, or the account has since
// been deactivated. The result is memoized per request (see withUserCache) so
// the several guards a handler may call share one store read; a *new* request
// always reloads, so permission and activation changes still take effect
// immediately (no stale session snapshot across requests).
func (s *Server) currentUser(r *http.Request) *model.User {
	cache, _ := r.Context().Value(userCacheCtxKey{}).(*userCache)
	if cache != nil && cache.loaded {
		return cache.user
	}
	u := s.loadCurrentUser(r)
	if cache != nil {
		cache.loaded = true
		cache.user = u
	}
	return u
}

// loadCurrentUser reads the session's user id and loads the active account from
// the store (uncached). Returns nil when unauthenticated, deleted, or inactive.
func (s *Server) loadCurrentUser(r *http.Request) *model.User {
	id := s.sessionUserID(r)
	if id == 0 {
		// No cookie session - fall back to a personal access token, so external
		// API clients (e.g. the FFXIV plugin) authenticate through the same guards.
		return s.userFromToken(r)
	}
	u := s.account(r.Context(), id)
	if u == nil || !u.IsActive {
		return nil
	}
	// Reject a session minted before the account's last password change/reset
	// (its stored epoch no longer matches), so a password change logs out every
	// other session. The current session is kept valid by signing it in again under
	// the new epoch (the change-password operation, auth.SignIn).
	if s.sessionUserEpoch(r) != u.PasswordEpoch {
		return nil
	}
	return u
}

// -- Live events ---------------------------------------------------------------

// resourceChanged tells the staff whose pages show resource that it changed, so a
// page showing it reads it again over REST. The event carries nothing more: the
// read applies the reader's own permission guard and keeps the REST answer the one
// shape of the data.
func (s *Server) resourceChanged(resource string) {
	api.ResourceChanged(s.live, resource)
}

// adminMutationResource maps a successful admin-mutation request (POST/PUT/PATCH/
// DELETE) to the resource_changed resource to tell, returning ok=false when the
// path tells none. It keys on the first path segment after /api/ (and v1/), so
// every resource-oriented sub-path (/{id}, /{id}/entries/{entryId}, /{id}/{verb},
// ...) is covered. The public, sign-in and self-service paths and the operations
// with events of their own (the game, cards, patterns, themes, settings) are
// absent. The public raffle sign-up (.../enter) and stamp rally sign-up (.../sign-up) are
// left out here because their operations tell the pages themselves, and the public entry search (.../lookup)
// changes nothing. A book club's paths tell that club's resource, except a cover upload.
func adminMutationResource(path string) (string, bool) {
	rest := strings.TrimPrefix(strings.TrimPrefix(path, "/api/"), "v1/")
	seg, after, _ := strings.Cut(rest, "/")
	switch seg {
	case "garapons", "affiliates", "tea-rooms", "festival-maps", "presets", "users", "winners-log",
		"ema-boards", "ema-requests":
		return seg, true
	case "fonts", "font-files":
		return "fonts", true
	case "image-categories":
		return "images", true
	case "carrd-projects":
		return "carrd", true
	case "stamp-rallies":
		if strings.HasSuffix(path, "/sign-up") {
			return "", false
		}
		return seg, true
	case "raffles":
		if strings.HasSuffix(path, "/enter") || strings.HasSuffix(path, "/lookup") {
			return "", false
		}
		return "raffles", true
	case "announcements", "announcement-types", "announcement-roles":
		return "announcements", true
	case "book-clubs":
		club, _, _ := strings.Cut(after, "/")
		// A cover upload changes no reading list.
		if club == "" || strings.HasSuffix(path, "/covers") {
			return "", false
		}
		return "bookclub-" + club, true
	}
	return "", false
}

// accountMutation reports whether a successful write to path may have changed what
// an open staff stream's account may follow: an account changed or deleted, a
// password changed, a token made or revoked, or a sign-out.
func accountMutation(path string) bool {
	rest := strings.TrimPrefix(strings.TrimPrefix(path, "/api/"), "v1/")
	seg, after, _ := strings.Cut(rest, "/")
	switch seg {
	case "users", "me":
		return true
	case "auth":
		return after == "logout"
	}
	return false
}

// siteBaseURL is this site's scheme://host, for the absolute addresses the operations
// build. It honors X-Forwarded-Proto, since the app runs behind a reverse proxy, and
// falls back to whether the connection is TLS.
func (s *Server) siteBaseURL(r *http.Request) string {
	scheme := "https"
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	} else if r.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}
