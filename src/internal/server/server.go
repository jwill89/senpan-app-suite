// Package server wires up the HTTP API, middleware, and JSON helpers.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"

	"app-suite/internal/bingo"
	"app-suite/internal/model"
	"app-suite/internal/store"
	"app-suite/internal/ws"
)

// Server holds all dependencies and implements http.Handler.
type Server struct {
	store       *store.Store
	game        *bingo.Service
	hub         *ws.Hub
	sessions    *scs.SessionManager
	sessHandler http.Handler // pre-built session middleware wrapping mux
	password    string
	webRoot     string
	mux         *http.ServeMux
	limiter     *rateLimiter // failed-login brute-force limiter
	regLimiter  *rateLimiter // registration-rate limiter (mass-signup abuse)
}

// New creates a Server, registers all API routes, and returns it.
func New(st *store.Store, hub *ws.Hub, sessionSecret, adminPassword, webRoot string) *Server {
	sm := scs.New()
	sm.Lifetime = 24 * time.Hour
	sm.Cookie.Path = "/"
	sm.Cookie.HttpOnly = true
	sm.Cookie.Secure = true
	sm.Cookie.SameSite = http.SameSiteLaxMode

	s := &Server{
		store:      st,
		game:       bingo.NewService(st),
		hub:        hub,
		sessions:   sm,
		password:   adminPassword,
		webRoot:    webRoot,
		mux:        http.NewServeMux(),
		limiter:    newRateLimiter(5, 15*time.Minute), // 5 failed logins per 15 minutes
		regLimiter: newRateLimiter(5, time.Hour),      // 5 registration attempts per hour
	}

	s.routes()
	// LoadAndSave (outer) populates the session, then withUserCache adds a
	// per-request memo so currentUser hits the DB at most once per request.
	s.sessHandler = sm.LoadAndSave(s.withUserCache(s.mux))
	return s
}

// userCacheCtxKey is the context key under which each request carries its
// per-request user memo.
type userCacheCtxKey struct{}

// userCache memoizes currentUser for the lifetime of a single request, so a
// handler that runs several guards (e.g. requireAuth then requirePermission)
// loads the account from the store once instead of per guard.
type userCache struct {
	loaded bool
	user   *model.User
}

// withUserCache injects an empty per-request user memo into the request context.
func (s *Server) withUserCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), userCacheCtxKey{}, &userCache{})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// routes registers all API endpoint handlers on the internal mux.
// Uses Go 1.22+ method-pattern routing ("GET /api/..." syntax).
func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/auth", s.handleAuthCheck)
	s.mux.HandleFunc("POST /api/auth", s.handleAuthAction)
	s.mux.HandleFunc("POST /api/register", s.handleRegister)

	// User management (admin) + self-service account (any logged-in user).
	s.mux.HandleFunc("GET /api/users", s.handleUsersList)
	s.mux.HandleFunc("POST /api/users", s.handleUsersAction)
	s.mux.HandleFunc("POST /api/account", s.handleAccountAction)
	s.mux.HandleFunc("GET /api/board", s.handleBoard)
	s.mux.HandleFunc("GET /api/cards", s.handleCardsList)
	s.mux.HandleFunc("POST /api/cards", s.handleCardsAction)
	s.mux.HandleFunc("GET /api/game", s.handleGameState)
	s.mux.HandleFunc("POST /api/game", s.handleGameAction)
	s.mux.HandleFunc("GET /api/patterns", s.handlePatternsList)
	s.mux.HandleFunc("POST /api/patterns", s.handlePatternsAction)
	s.mux.HandleFunc("GET /api/pattern-categories", s.handleCategoriesList)
	s.mux.HandleFunc("POST /api/pattern-categories", s.handleCategoriesAction)
	s.mux.HandleFunc("GET /api/presets", s.handlePresetsList)
	s.mux.HandleFunc("POST /api/presets", s.handlePresetsAction)
	s.mux.HandleFunc("GET /api/styles", s.handleStylesList)
	s.mux.HandleFunc("POST /api/styles", s.handleStylesAction)
	s.mux.HandleFunc("GET /api/styles/active", s.handleActiveStyleCSS)
	s.mux.HandleFunc("/api/ws", s.handleWS) // WebSocket: method-agnostic for upgrade

	// Raffle routes (upload before {id} to avoid path conflict)
	s.mux.HandleFunc("GET /api/raffles", s.handleRafflesList)
	s.mux.HandleFunc("POST /api/raffles", s.handleRafflesAction)
	s.mux.HandleFunc("POST /api/raffles/upload", s.handleRaffleUpload)
	s.mux.HandleFunc("GET /api/raffles/{id}", s.handleRaffleDetail)
	s.mux.HandleFunc("POST /api/raffles/{id}/enter", s.handleRaffleEnter)
	s.mux.HandleFunc("POST /api/raffles/{id}/entries", s.handleRaffleEntries)

	// Book club / reading list routes (specific paths before {id} to avoid conflicts)
	s.mux.HandleFunc("POST /api/bookclub/upload", s.handleBookclubUpload)
	s.mux.HandleFunc("GET /api/bookclub/lookup", s.handleBookclubLookup)
	// Book club event posts (scheduled Discord embeds)
	s.mux.HandleFunc("GET /api/bookclub/events", s.handleBookClubEventsList)
	s.mux.HandleFunc("POST /api/bookclub/events", s.handleBookClubEventsAction)
	s.mux.HandleFunc("GET /api/bookclub/events/images", s.handleBookClubEventImages)
	s.mux.HandleFunc("POST /api/bookclub/events/upload", s.handleBookClubEventUpload)
	s.mux.HandleFunc("GET /api/reading-lists", s.handleReadingListsList)
	s.mux.HandleFunc("POST /api/reading-lists", s.handleReadingListsAction)
	s.mux.HandleFunc("GET /api/reading-lists/{id}", s.handleReadingListDetail)
	s.mux.HandleFunc("POST /api/reading-lists/{id}/items", s.handleReadingListItems)
	s.mux.HandleFunc("POST /api/reading-lists/{id}/publish", s.handlePublishReadingList)

	// Announcement management (typed Discord destinations + scheduled embeds).
	// Specific paths before any {id} routes to avoid pattern conflicts.
	s.mux.HandleFunc("GET /api/announcement-types", s.handleAnnouncementTypesList)
	s.mux.HandleFunc("POST /api/announcement-types", s.handleAnnouncementTypesAction)
	s.mux.HandleFunc("GET /api/announcements/images", s.handleAnnouncementImages)
	s.mux.HandleFunc("POST /api/announcements/upload", s.handleAnnouncementUpload)
	s.mux.HandleFunc("GET /api/announcements", s.handleAnnouncementsList)
	s.mux.HandleFunc("POST /api/announcements", s.handleAnnouncementsAction)

	// Winners log routes
	s.mux.HandleFunc("GET /api/winners-log", s.handleWinnersLog)
	s.mux.HandleFunc("POST /api/winners-log", s.handleWinnersLogAction)
	s.mux.HandleFunc("GET /api/winners-log/frequent", s.handleFrequentWinners)

	// App settings
	s.mux.HandleFunc("GET /api/settings", s.handleSettingsGet)
	s.mux.HandleFunc("POST /api/settings", s.handleSettingsUpdate)

	// Font file management (System → Font Upload)
	s.mux.HandleFunc("GET /api/fonts", s.handleFontsList)
	s.mux.HandleFunc("POST /api/fonts", s.handleFontsAction)
	s.mux.HandleFunc("POST /api/fonts/upload", s.handleFontUpload)

	// Carrd image hosting (System → Carrd Upload)
	s.mux.HandleFunc("GET /api/carrd/projects", s.handleCarrdProjectsList)
	s.mux.HandleFunc("POST /api/carrd/projects", s.handleCarrdProjectsAction)
	s.mux.HandleFunc("GET /api/carrd/images", s.handleCarrdImagesList)
	s.mux.HandleFunc("POST /api/carrd/images", s.handleCarrdImagesAction)
	s.mux.HandleFunc("POST /api/carrd/upload", s.handleCarrdUpload)
}

// ServeHTTP applies CORS middleware, logs the request, then dispatches to the router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Vary", "Origin")
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// WebSocket connections must bypass session middleware and response
	// wrappers — coder/websocket needs the raw ResponseWriter for upgrade.
	if r.URL.Path == "/api/ws" {
		s.mux.ServeHTTP(w, r)
		return
	}

	start := time.Now()
	rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

	// Wrap with SCS session middleware so session data is available in handlers.
	s.sessHandler.ServeHTTP(rw, r)
	duration := time.Since(start)

	level := slog.LevelInfo
	if rw.status >= 500 {
		level = slog.LevelError
	} else if rw.status >= 400 {
		level = slog.LevelWarn
	}
	slog.Log(r.Context(), level, "http request",
		"method", r.Method,
		"path", r.URL.Path,
		"status", rw.status,
		"duration", duration.Round(time.Microsecond),
		"ip", r.RemoteAddr,
	)
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
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

// ── JSON helpers ────────────────────────────────────────────────────────────

// writeJSON serializes v as JSON and writes it with the given status code.
// Sets Content-Type and Cache-Control: no-store to prevent caching of API responses.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response: {"error": msg} with the given HTTP status.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeInternalError logs the error with context and writes a generic 500 response.
func writeInternalError(w http.ResponseWriter, context string, err error) {
	slog.Error("internal server error", "context", context, "error", err)
	writeError(w, http.StatusInternalServerError, "Internal server error")
}

// readJSON decodes the request body into a typed struct T using generics.
// Limits request body to 1MB to prevent memory abuse.
// Returns the zero value and an error if decoding fails.
func readJSON[T any](r *http.Request) (T, error) {
	var v T
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1MB limit
	err := json.NewDecoder(r.Body).Decode(&v)
	return v, err
}

// ── Auth helpers ────────────────────────────────────────────────────────────

// sessionUserID returns the logged-in user's id from the session, or 0 if none.
// The id is stored as int64 (see handleAuthAction), so it is read back via a
// type assertion rather than scs's GetInt (which only matches plain int).
func (s *Server) sessionUserID(r *http.Request) int64 {
	if id, ok := s.sessions.Get(r.Context(), "user_id").(int64); ok {
		return id
	}
	return 0
}

// wsSessionUser loads the account for a request that bypasses the session
// middleware — specifically the /api/ws upgrade, which is dispatched straight to
// the mux (coder/websocket needs the raw ResponseWriter, so it can't go through
// LoadAndSave/withUserCache). It reads the session cookie, loads the SCS session
// manually, and returns the authenticated, active account or nil. Used to gate
// the privileged admin WebSocket channel.
func (s *Server) wsSessionUser(r *http.Request) *model.User {
	cookie, err := r.Cookie(s.sessions.Cookie.Name)
	if err != nil {
		return nil
	}
	ctx, err := s.sessions.Load(r.Context(), cookie.Value)
	if err != nil {
		return nil
	}
	id, ok := s.sessions.Get(ctx, "user_id").(int64)
	if !ok || id == 0 {
		return nil
	}
	u, err := s.store.GetUserByID(id)
	if err != nil || u == nil || !u.IsActive {
		return nil
	}
	return u
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
		return nil
	}
	u, err := s.store.GetUserByID(id)
	if err != nil || u == nil || !u.IsActive {
		return nil
	}
	return u
}

// isAdmin reports whether the current request is from an authenticated, active
// admin user.
func (s *Server) isAdmin(r *http.Request) bool {
	u := s.currentUser(r)
	return u != nil && u.IsAdmin
}

// requireAuth is a guard for endpoints any logged-in (active) user may call. It
// returns the user and true, or writes a 401 and returns false. Handlers should
// return immediately when this returns false.
func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) (*model.User, bool) {
	u := s.currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized – login required")
		return nil, false
	}
	return u, true
}

// requireAdmin is a guard that writes a 401 error and returns false if the
// request is not from an authenticated admin. Handlers should return immediately
// when this returns false.
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.isAdmin(r) {
		writeError(w, http.StatusUnauthorized, "Unauthorized – admin login required")
		return false
	}
	return true
}

// requirePermission is the per-page guard: it allows admins (who hold every
// permission) and non-admin users granted the given page-permission key. It
// writes a 401 when unauthenticated and a 403 when authenticated but lacking the
// permission, returning false in both cases.
func (s *Server) requirePermission(w http.ResponseWriter, r *http.Request, perm string) bool {
	u := s.currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized – login required")
		return false
	}
	if u.IsAdmin || userHasPermission(u, perm) {
		return true
	}
	writeError(w, http.StatusForbidden, "Forbidden – you do not have access to this feature")
	return false
}

// ── Broadcast helpers ───────────────────────────────────────────────────────

// cardEntry is the lightweight JSON shape for card lists.
type cardEntry struct {
	ID string `json:"id"`
}

// broadcastCards sends an updated card ID list to all WebSocket clients.
func (s *Server) broadcastCards() {
	ids, err := s.store.ListCardIDs()
	if err != nil {
		return
	}
	cards := make([]cardEntry, len(ids))
	for i, id := range ids {
		cards[i] = cardEntry{ID: id}
	}
	s.hub.Broadcast(struct {
		Type  string      `json:"type"`
		Cards []cardEntry `json:"cards"`
	}{Type: "cards_update", Cards: cards})
}

// broadcastPatterns sends updated patterns and categories to all WebSocket clients.
func (s *Server) broadcastPatterns() {
	patterns, err := s.store.ListPatterns()
	if err != nil {
		return
	}
	categories, err := s.store.ListPatternCategories()
	if err != nil {
		return
	}
	s.hub.Broadcast(struct {
		Type       string                  `json:"type"`
		Patterns   []model.Pattern         `json:"patterns"`
		Categories []model.PatternCategory `json:"categories"`
	}{Type: "patterns_update", Patterns: patterns, Categories: categories})
}

// broadcastGameStart sends the new game state to all clients.
// Players receive game state + details; admins receive game state only
// (admin who started the game already received details via the HTTP response).
func (s *Server) broadcastGameStart(state *model.BingoGameState, details string) {
	s.hub.BroadcastToPlayers(struct {
		Type    string                `json:"type"`
		Game    *model.BingoGameState `json:"game"`
		Details string                `json:"game_details"`
	}{Type: "game_update", Game: state, Details: details})

	s.hub.BroadcastToAdmins(struct {
		Type    string                `json:"type"`
		Game    *model.BingoGameState `json:"game"`
		Winners []string              `json:"winners"`
	}{Type: "game_update", Game: state, Winners: []string{}})
}

// broadcastGameEnd notifies all clients that the game has ended.
func (s *Server) broadcastGameEnd() {
	s.hub.Broadcast(struct {
		Type string `json:"type"`
		Game any    `json:"game"`
	}{Type: "game_update", Game: nil})
}

// broadcastDrawToPlayers sends just the drawn number to all player clients.
func (s *Server) broadcastDrawToPlayers(drawn model.BingoDrawnNumber) {
	s.hub.BroadcastToPlayers(struct {
		Type  string                 `json:"type"`
		Drawn model.BingoDrawnNumber `json:"drawn"`
	}{Type: "game_draw", Drawn: drawn})
}

// broadcastDrawToAdmins sends the drawn number + updated winners to all admin clients.
func (s *Server) broadcastDrawToAdmins(drawn model.BingoDrawnNumber, winners []string) {
	s.hub.BroadcastToAdmins(struct {
		Type    string                 `json:"type"`
		Drawn   model.BingoDrawnNumber `json:"drawn"`
		Winners []string               `json:"winners"`
	}{Type: "game_draw", Drawn: drawn, Winners: winners})
}

// broadcastStyleUpdate sends a style_update message with the new CSS to all clients.
func (s *Server) broadcastStyleUpdate(css string) {
	s.hub.Broadcast(struct {
		Type string `json:"type"`
		CSS  string `json:"css"`
	}{Type: "style_update", CSS: css})
}
