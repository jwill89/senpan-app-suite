// Package server wires up the HTTP API, middleware, and JSON helpers.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
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
	webRoot     string
	// allowedOrigins is the CORS allowlist (exact origin strings). Normally
	// empty — the SPA and API are same-origin — so no CORS headers are sent.
	allowedOrigins map[string]bool
	mux            *http.ServeMux
	limiter        *rateLimiter // failed-login brute-force limiter
	regLimiter     *rateLimiter // registration-rate limiter (mass-signup abuse)
	raffleLimiter  *rateLimiter // public raffle-entry limiter (entry flooding)
	// Cloudflare Turnstile bot check on the admin login. Disabled (verification
	// skipped) when turnstileSecret is empty — see SetTurnstile / turnstile.go.
	turnstileSecret  string
	turnstileSiteKey string
	// openAPISpec is the embedded openapi.yaml served at GET /api/openapi.yaml
	// and rendered by GET /api/docs. Injected from main via SetOpenAPISpec.
	openAPISpec []byte
	// logFile is the path to the rotating JSON log file the admin log viewer
	// (GET /api/logs) tails. Empty disables the viewer. Injected via SetLogFile.
	logFile string
	// Lazily-loaded HMAC key for the tokenized public font URLs — see
	// fontserve.go. Generated and persisted to settings on first use.
	fontSecretMu  sync.Mutex
	fontSecretVal []byte
	// Serializes the image-category manifest read-modify-write (create/rename/
	// delete + startup migration) so concurrent admin edits can't lose a write or
	// orphan a directory. See images.go. Distinct from fontSecretMu — the font
	// metadata lives in the DB, this manifest is a filesystem dotfile.
	imageManifestMu sync.Mutex
}

// SetTurnstile enables the Cloudflare Turnstile bot check on the login form.
// secret is the server-side secret key (kept private); siteKey is the public key
// served to the browser. An empty secret leaves Turnstile disabled, so the test
// harness and local dev (no keys) keep working without a challenge.
func (s *Server) SetTurnstile(secret, siteKey string) {
	s.turnstileSecret = strings.TrimSpace(secret)
	s.turnstileSiteKey = strings.TrimSpace(siteKey)
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
func New(st *store.Store, hub *ws.Hub, sessionSecret, webRoot string, allowedOrigins []string) *Server {
	sm := scs.New()
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
		game:           bingo.NewService(st),
		hub:            hub,
		sessions:       sm,
		webRoot:        webRoot,
		allowedOrigins: origins,
		mux:            http.NewServeMux(),
		limiter:        newRateLimiter(5, 15*time.Minute),  // 5 failed logins per 15 minutes
		regLimiter:     newRateLimiter(5, time.Hour),       // 5 registration attempts per hour
		raffleLimiter:  newRateLimiter(20, 10*time.Minute), // 20 raffle entries per 10 minutes per IP
	}

	s.routes()
	// API-reference endpoints (GET /api/docs, GET /api/openapi.yaml). Registered
	// apart from routes() so routes() stays the authoritative API-surface list.
	s.registerDocs()
	// Upgrade the image-category manifest to the current schema (folds the
	// formerly hardcoded permanent categories into the manifest; runs once).
	s.migrateImageCategoryManifest()
	// One-time, idempotent migration of legacy announcement images into the new
	// announcements_main category dir (safe to run on every startup).
	s.migrateAnnouncementImages()
	// Seed the built-in flourish SVGs into the Flourishes image category so they
	// are pickable in the theme flourish selectors (idempotent).
	s.seedFlourishes()
	// Upgrade pre-group font metadata (file-keyed entries, the old global
	// origin allowlist), then reconcile every font's WOFF2 conversion —
	// backfilling missing ones and sweeping stale copies. Both idempotent;
	// conversion failures log and fall back to serving an uploaded format.
	s.migrateFontMetaV2()
	s.migrateFontDerivatives()
	// LoadAndSave (outer) populates the session, then withUserCache adds a
	// per-request memo so currentUser hits the DB at most once per request.
	s.sessHandler = sm.LoadAndSave(s.withUserCache(s.withActor(s.mux)))
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
	s.mux.HandleFunc("GET /api/version", s.handleVersion)
	s.mux.HandleFunc("GET /api/config", s.handleConfig)
	s.mux.HandleFunc("GET /api/auth", s.handleAuthCheck)
	s.mux.HandleFunc("POST /api/auth", s.handleAuthAction)
	s.mux.HandleFunc("POST /api/register", s.handleRegister)
	// Passkey (WebAuthn) login — public, usernameless discoverable-credential flow.
	s.mux.HandleFunc("POST /api/auth/passkey/begin", s.handlePasskeyLoginBegin)
	s.mux.HandleFunc("POST /api/auth/passkey/finish", s.handlePasskeyLoginFinish)

	// User management (admin, resource-oriented: PATCH merges the former
	// set_active/set_admin/set_permissions/set_password actions) + self-service
	// account (any logged-in user). The literal /account/change-password and
	// /account/token sub-paths are matched ahead of one another by the Go 1.22 mux.
	s.mux.HandleFunc("GET /api/users", s.handleUsersList)
	s.mux.HandleFunc("PATCH /api/users/{id}", s.handleUserPatch)
	s.mux.HandleFunc("DELETE /api/users/{id}", s.handleUserDelete)
	s.mux.HandleFunc("POST /api/account/change-password", s.handleAccountChangePassword)
	// Personal access tokens (self-service): GET reads the non-secret metadata,
	// POST generates (returning the plaintext once), DELETE revokes. Used by
	// external API clients (e.g. the FFXIV plugin).
	s.mux.HandleFunc("GET /api/account/token", s.handleAccountTokenInfo)
	s.mux.HandleFunc("POST /api/account/token", s.handleAccountTokenGenerate)
	s.mux.HandleFunc("DELETE /api/account/token", s.handleAccountTokenRevoke)
	// Passkeys (self-service): register a new passkey (begin/finish), list, delete.
	s.mux.HandleFunc("POST /api/account/passkeys/register/begin", s.handlePasskeyRegisterBegin)
	s.mux.HandleFunc("POST /api/account/passkeys/register/finish", s.handlePasskeyRegisterFinish)
	s.mux.HandleFunc("GET /api/account/passkeys", s.handlePasskeyList)
	s.mux.HandleFunc("DELETE /api/account/passkeys/{id}", s.handlePasskeyDelete)
	s.mux.HandleFunc("GET /api/board", s.handleBoard)
	// Cards (resource-oriented). The literal /generate and /all sub-paths are
	// matched ahead of the {id} wildcard by the Go 1.22 mux, so there's no
	// conflict. Card handlers self-broadcast (cards_update / card_deleted), so
	// cards are intentionally absent from the adminMutationResource middleware.
	s.mux.HandleFunc("GET /api/cards", s.handleCardsList)
	s.mux.HandleFunc("POST /api/cards", s.handleCardCreate)
	s.mux.HandleFunc("POST /api/cards/generate", s.handleCardsGenerate)
	s.mux.HandleFunc("DELETE /api/cards/all", s.handleCardsDeleteAll)
	s.mux.HandleFunc("DELETE /api/cards/{id}", s.handleCardDelete)
	s.mux.HandleFunc("PATCH /api/cards/{id}", s.handleCardUpdate)
	// Game (singleton resource): GET state, POST lifecycle verbs, PATCH controls.
	// The game handlers self-broadcast (game_update / game_draw / etc.), so game
	// is intentionally absent from the adminMutationResource middleware.
	s.mux.HandleFunc("GET /api/game", s.handleGameState)
	s.mux.HandleFunc("POST /api/game/start", s.handleGameStart)
	s.mux.HandleFunc("POST /api/game/draw", s.handleGameDraw)
	s.mux.HandleFunc("POST /api/game/end", s.handleGameEnd)
	s.mux.HandleFunc("POST /api/game/halftime", s.handleGameHalftime)
	s.mux.HandleFunc("POST /api/game/yoever", s.handleGameYoever)
	s.mux.HandleFunc("PATCH /api/game", s.handleGamePatch)
	// Patterns (resource-oriented). Single reorder is folded into PATCH /{id}
	// (direction field); bulk reorder is the literal POST /reorder sub-path,
	// matched ahead of the {id} wildcard by the Go 1.22 mux. Handlers
	// self-broadcast (patterns_update), so patterns/categories are intentionally
	// absent from the adminMutationResource middleware.
	s.mux.HandleFunc("GET /api/patterns", s.handlePatternsList)
	s.mux.HandleFunc("POST /api/patterns", s.handlePatternCreate)
	s.mux.HandleFunc("POST /api/patterns/reorder", s.handlePatternsReorder)
	s.mux.HandleFunc("PATCH /api/patterns/{id}", s.handlePatternPatch)
	s.mux.HandleFunc("DELETE /api/patterns/{id}", s.handlePatternDelete)
	s.mux.HandleFunc("GET /api/pattern-categories", s.handleCategoriesList)
	s.mux.HandleFunc("POST /api/pattern-categories", s.handleCategoryCreate)
	s.mux.HandleFunc("POST /api/pattern-categories/reorder", s.handleCategoriesReorder)
	s.mux.HandleFunc("PATCH /api/pattern-categories/{id}", s.handleCategoryPatch)
	s.mux.HandleFunc("DELETE /api/pattern-categories/{id}", s.handleCategoryDelete)
	s.mux.HandleFunc("GET /api/presets", s.handlePresetsList)
	s.mux.HandleFunc("POST /api/presets", s.handlePresetCreate)
	s.mux.HandleFunc("PUT /api/presets/{id}", s.handlePresetUpdate)
	s.mux.HandleFunc("DELETE /api/presets/{id}", s.handlePresetDelete)
	// Styles (resource-oriented). The literal /active (public CSS) and
	// /deactivate sub-paths are matched ahead of the {id} wildcard by the Go 1.22
	// mux. Style write handlers self-broadcast (style_update), so styles are
	// intentionally absent from the adminMutationResource middleware.
	s.mux.HandleFunc("GET /api/styles", s.handleStylesList)
	s.mux.HandleFunc("POST /api/styles", s.handleStyleCreate)
	s.mux.HandleFunc("POST /api/styles/deactivate", s.handleStyleDeactivate)
	s.mux.HandleFunc("GET /api/styles/active", s.handleActiveStyleCSS)
	s.mux.HandleFunc("GET /api/styles/{id}", s.handleStyleGet)
	s.mux.HandleFunc("PUT /api/styles/{id}", s.handleStyleUpdate)
	s.mux.HandleFunc("DELETE /api/styles/{id}", s.handleStyleDelete)
	s.mux.HandleFunc("POST /api/styles/{id}/activate", s.handleStyleActivate)
	s.mux.HandleFunc("/api/ws", s.handleWS) // WebSocket: method-agnostic for upgrade

	// Raffles (resource-oriented: methods for CRUD, POST /{id}/{verb} for commands)
	s.mux.HandleFunc("GET /api/raffles", s.handleRafflesList)
	s.mux.HandleFunc("POST /api/raffles", s.handleRaffleCreate)
	s.mux.HandleFunc("GET /api/raffles/{id}", s.handleRaffleDetail)
	s.mux.HandleFunc("PUT /api/raffles/{id}", s.handleRaffleUpdate)
	s.mux.HandleFunc("DELETE /api/raffles/{id}", s.handleRaffleDelete)
	s.mux.HandleFunc("POST /api/raffles/{id}/enter", s.handleRaffleEnter) // public sign-up
	s.mux.HandleFunc("POST /api/raffles/{id}/entries", s.handleRaffleEntryAdd)
	s.mux.HandleFunc("PATCH /api/raffles/{id}/entries/{entryId}", s.handleRaffleEntryPatch)
	s.mux.HandleFunc("DELETE /api/raffles/{id}/entries/{entryId}", s.handleRaffleEntryDelete)
	s.mux.HandleFunc("POST /api/raffles/{id}/pick-winner", s.handleRafflePickWinner)
	s.mux.HandleFunc("POST /api/raffles/{id}/pick-another", s.handleRafflePickAnother)
	s.mux.HandleFunc("POST /api/raffles/{id}/verify-winner", s.handleRaffleVerifyWinner)

	// Garapon routes (festival lottery drum; resource-oriented: methods for CRUD,
	// POST /{id}/{verb} for status). Admin CRUD + close/reopen + per-player drawing
	// links, plus the tokenized public player view + draw. The "garapon/{token}"
	// (singular) public paths don't collide with the "garapons" (plural) admin
	// paths; the literal /close, /reopen, /players sub-paths are matched ahead of
	// the {id} wildcard by the Go 1.22 mux.
	s.mux.HandleFunc("GET /api/garapons", s.handleGaraponsList)
	s.mux.HandleFunc("POST /api/garapons", s.handleGaraponCreate)
	s.mux.HandleFunc("GET /api/garapons/{id}", s.handleGaraponDetail)
	s.mux.HandleFunc("PUT /api/garapons/{id}", s.handleGaraponUpdate)
	s.mux.HandleFunc("DELETE /api/garapons/{id}", s.handleGaraponDelete)
	s.mux.HandleFunc("POST /api/garapons/{id}/close", s.handleGaraponClose)
	s.mux.HandleFunc("POST /api/garapons/{id}/reopen", s.handleGaraponReopen)
	s.mux.HandleFunc("POST /api/garapons/{id}/players", s.handleGaraponPlayerCreate)
	s.mux.HandleFunc("DELETE /api/garapons/{id}/players/{playerId}", s.handleGaraponPlayerDelete)
	s.mux.HandleFunc("GET /api/garapon/{token}", s.handleGaraponPublic)
	s.mux.HandleFunc("POST /api/garapon/{token}/draw", s.handleGaraponDraw)

	// Affiliates (Senpan Tea House → Affiliates). Admin-only CRUD of partner
	// establishments; logo/screenshot images are picked from the shared image
	// library managed on System → Images.
	s.mux.HandleFunc("GET /api/affiliates", s.handleAffiliatesList)
	s.mux.HandleFunc("POST /api/affiliates", s.handleAffiliateCreate)
	s.mux.HandleFunc("PUT /api/affiliates/{id}", s.handleAffiliateUpdate)
	s.mux.HandleFunc("DELETE /api/affiliates/{id}", s.handleAffiliateDelete)

	// Tea Rooms (Senpan Tea House → Tea Rooms). Admin CRUD + drag reorder + the
	// open/discounted flag toggles (PATCH) + post-to-Discord, the shared webhook
	// setter, and a public cross-origin read API for external Carrd sites. The
	// literal /reorder, /webhook, /public sub-paths are matched ahead of the {id}
	// wildcard by the Go 1.22 mux; /public/{id} coexists with them.
	s.mux.HandleFunc("GET /api/tea-rooms", s.handleTeaRoomsList)
	s.mux.HandleFunc("POST /api/tea-rooms", s.handleTeaRoomCreate)
	s.mux.HandleFunc("POST /api/tea-rooms/reorder", s.handleTeaRoomsReorder)
	s.mux.HandleFunc("PUT /api/tea-rooms/webhook", s.handleTeaRoomWebhookSet)
	s.mux.HandleFunc("GET /api/tea-rooms/public", s.handleTeaRoomsPublic)
	s.mux.HandleFunc("GET /api/tea-rooms/public/{number}", s.handleTeaRoomPublic)
	s.mux.HandleFunc("PUT /api/tea-rooms/{id}", s.handleTeaRoomUpdate)
	s.mux.HandleFunc("PATCH /api/tea-rooms/{id}", s.handleTeaRoomPatch)
	s.mux.HandleFunc("DELETE /api/tea-rooms/{id}", s.handleTeaRoomDelete)
	s.mux.HandleFunc("POST /api/tea-rooms/{id}/post", s.handleTeaRoomPost)

	// Stamp Rally (Festival → Stamp Rally; resource-oriented: methods for CRUD,
	// POST /{id}/{verb} for status, PATCH for the per-stamp pause toggle). Admin
	// CRUD of events (stamps + prizes with placements) + close/reopen, tokenized
	// participant cards, and the event-wide stamp log, plus the tokenized public
	// card view + password-driven stamp collection. The singular "stamp-card/{token}"
	// public paths don't collide with the plural "stamp-rallies" admin paths
	// (mirrors the garapon singular/plural split); the literal /logs, /close,
	// /reopen, /stamps, /cards sub-paths are matched ahead of the {id} wildcard.
	s.mux.HandleFunc("GET /api/stamp-rallies", s.handleStampRalliesList)
	s.mux.HandleFunc("POST /api/stamp-rallies", s.handleStampRallyCreate)
	s.mux.HandleFunc("GET /api/stamp-rallies/{id}", s.handleStampRallyDetail)
	s.mux.HandleFunc("PUT /api/stamp-rallies/{id}", s.handleStampRallyUpdate)
	s.mux.HandleFunc("DELETE /api/stamp-rallies/{id}", s.handleStampRallyDelete)
	s.mux.HandleFunc("GET /api/stamp-rallies/{id}/logs", s.handleStampRallyLogs)
	s.mux.HandleFunc("POST /api/stamp-rallies/{id}/close", s.handleStampRallyClose)
	s.mux.HandleFunc("POST /api/stamp-rallies/{id}/reopen", s.handleStampRallyReopen)
	s.mux.HandleFunc("PATCH /api/stamp-rallies/{id}/stamps/{stampId}", s.handleStampRallyStampPatch)
	s.mux.HandleFunc("POST /api/stamp-rallies/{id}/cards", s.handleStampRallyCardCreate)
	s.mux.HandleFunc("DELETE /api/stamp-rallies/{id}/cards/{cardId}", s.handleStampRallyCardDelete)
	s.mux.HandleFunc("GET /api/stamp-card/{token}", s.handleStampCardPublic)
	s.mux.HandleFunc("POST /api/stamp-card/{token}/stamp", s.handleStampCardStamp)

	// Book club / reading list routes. Reading lists are nested under their owning
	// club ({club} path segment), making each book club a first-class parent
	// entity (resource-oriented: methods for CRUD on lists and their /items
	// sub-resource; POST /{id}/publish is a verb route). The two club-agnostic
	// utility endpoints (cover upload, AniList lookup) stay under /api/bookclub.
	s.mux.HandleFunc("POST /api/bookclub/upload", s.handleBookclubUpload)
	s.mux.HandleFunc("GET /api/bookclub/lookup", s.handleBookclubLookup)
	s.mux.HandleFunc("GET /api/book-clubs/{club}/reading-lists", s.handleReadingListsList)
	s.mux.HandleFunc("POST /api/book-clubs/{club}/reading-lists", s.handleReadingListCreate)
	s.mux.HandleFunc("GET /api/book-clubs/{club}/reading-lists/{id}", s.handleReadingListDetail)
	s.mux.HandleFunc("PUT /api/book-clubs/{club}/reading-lists/{id}", s.handleReadingListUpdate)
	s.mux.HandleFunc("DELETE /api/book-clubs/{club}/reading-lists/{id}", s.handleReadingListDelete)
	s.mux.HandleFunc("POST /api/book-clubs/{club}/reading-lists/{id}/items", s.handleReadingListItemCreate)
	s.mux.HandleFunc("PUT /api/book-clubs/{club}/reading-lists/{id}/items/{itemId}", s.handleReadingListItemUpdate)
	s.mux.HandleFunc("DELETE /api/book-clubs/{club}/reading-lists/{id}/items/{itemId}", s.handleReadingListItemDelete)
	s.mux.HandleFunc("POST /api/book-clubs/{club}/reading-lists/{id}/publish", s.handlePublishReadingList)

	// Announcement management (typed Discord destinations + scheduled embeds).
	// Specific paths before any {id} routes to avoid pattern conflicts.
	s.mux.HandleFunc("GET /api/announcement-types", s.handleAnnouncementTypesList)
	s.mux.HandleFunc("POST /api/announcement-types", s.handleAnnouncementTypeCreate)
	s.mux.HandleFunc("PUT /api/announcement-types/{id}", s.handleAnnouncementTypeUpdate)
	s.mux.HandleFunc("DELETE /api/announcement-types/{id}", s.handleAnnouncementTypeDelete)
	s.mux.HandleFunc("GET /api/announcement-roles", s.handleAnnouncementRolesList)
	s.mux.HandleFunc("POST /api/announcement-roles", s.handleAnnouncementRoleCreate)
	s.mux.HandleFunc("PUT /api/announcement-roles/{id}", s.handleAnnouncementRoleUpdate)
	s.mux.HandleFunc("DELETE /api/announcement-roles/{id}", s.handleAnnouncementRoleDelete)
	// Announcements (resource-oriented: methods for CRUD, POST /{id}/{verb} for
	// send/skip, POST /reorder for the bulk drag-order). The literal /reorder
	// sub-path is matched ahead of the {id} wildcard by the Go 1.22 mux.
	s.mux.HandleFunc("GET /api/announcements", s.handleAnnouncementsList)
	s.mux.HandleFunc("POST /api/announcements", s.handleAnnouncementCreate)
	s.mux.HandleFunc("POST /api/announcements/reorder", s.handleAnnouncementsReorder)
	s.mux.HandleFunc("PUT /api/announcements/{id}", s.handleAnnouncementUpdate)
	s.mux.HandleFunc("DELETE /api/announcements/{id}", s.handleAnnouncementDelete)
	s.mux.HandleFunc("POST /api/announcements/{id}/send", s.handleAnnouncementSend)
	s.mux.HandleFunc("POST /api/announcements/{id}/skip", s.handleAnnouncementSkip)

	// Winners log routes. The literal /all and /frequent sub-paths are matched
	// ahead of the {id} wildcard by the Go 1.22 mux, so there's no conflict.
	s.mux.HandleFunc("GET /api/winners-log", s.handleWinnersLog)
	s.mux.HandleFunc("DELETE /api/winners-log/all", s.handleWinnersLogDeleteAll)
	s.mux.HandleFunc("DELETE /api/winners-log/{id}", s.handleWinnersLogDelete)
	s.mux.HandleFunc("GET /api/winners-log/frequent", s.handleFrequentWinners)

	// App settings
	s.mux.HandleFunc("GET /api/settings", s.handleSettingsGet)
	s.mux.HandleFunc("POST /api/settings", s.handleSettingsUpdate)

	// Server log viewer (admin-only): tails the rotating JSON log file, and flips
	// the runtime log level (live DEBUG toggle).
	s.mux.HandleFunc("GET /api/logs", s.handleLogs)
	s.mux.HandleFunc("POST /api/logs/level", s.handleLogLevelSet)

	// Font management (Atelier → Font Upload). Font FILES are keyed by filename
	// ({name}); logical FONTS (groups of variants sharing a base name) are keyed
	// by base under the literal /families sub-path, which the Go 1.22 mux
	// matches ahead of the single-segment {name} wildcard. The literal /upload
	// sub-path likewise precedes {name}.
	s.mux.HandleFunc("GET /api/fonts", s.handleFontsList)
	s.mux.HandleFunc("POST /api/fonts/upload", s.handleFontUpload)
	s.mux.HandleFunc("DELETE /api/fonts/{name}", s.handleFontDelete)
	s.mux.HandleFunc("PATCH /api/fonts/{name}", s.handleFontRename)
	s.mux.HandleFunc("PATCH /api/fonts/families/{base}", s.handleFontFamilyPatch)
	s.mux.HandleFunc("DELETE /api/fonts/families/{base}", s.handleFontFamilyDelete)
	// Public tokenized font serving (fontserve.go). The fonts.senpan.cafe vhost
	// reverse-proxies to these ("/" → /api/fonts/pub/); the SPA loads the same
	// endpoints same-origin via the /api ProxyPass, so per-font allowlists never
	// affect the app itself.
	s.mux.HandleFunc("GET /api/fonts/pub/kit.css", s.handleFontKitCSS)
	s.mux.HandleFunc("GET /api/fonts/pub/f/{token}", s.handleFontPublicFile)

	// Carrd image hosting (System → Carrd Upload). Projects are keyed by folder
	// name (a path param); images/sub-dirs are keyed by folder+path (+name), which
	// may contain slashes, so those deletes take query params. The literal
	// /images/dirs sub-path coexists with /images (different method+path pairs).
	s.mux.HandleFunc("GET /api/carrd/projects", s.handleCarrdProjectsList)
	s.mux.HandleFunc("POST /api/carrd/projects", s.handleCarrdProjectCreate)
	s.mux.HandleFunc("PATCH /api/carrd/projects/{folder}", s.handleCarrdProjectRename)
	s.mux.HandleFunc("DELETE /api/carrd/projects/{folder}", s.handleCarrdProjectDelete)
	s.mux.HandleFunc("GET /api/carrd/images", s.handleCarrdImagesList)
	s.mux.HandleFunc("DELETE /api/carrd/images", s.handleCarrdImageDelete)
	s.mux.HandleFunc("POST /api/carrd/images/dirs", s.handleCarrdDirCreate)
	s.mux.HandleFunc("DELETE /api/carrd/images/dirs", s.handleCarrdDirDelete)
	s.mux.HandleFunc("POST /api/carrd/upload", s.handleCarrdUpload)

	// Central image hosting (System → Images). Categories are keyed by directory
	// name (a path param); image deletes take dir+name query params. /upload
	// before the bare path so the more specific pattern wins.
	s.mux.HandleFunc("GET /api/image-categories", s.handleImageCategoriesList)
	s.mux.HandleFunc("POST /api/image-categories", s.handleImageCategoryCreate)
	s.mux.HandleFunc("PATCH /api/image-categories/{dir}", s.handleImageCategoryRename)
	s.mux.HandleFunc("DELETE /api/image-categories/{dir}", s.handleImageCategoryDelete)
	s.mux.HandleFunc("GET /api/images", s.handleImagesList)
	s.mux.HandleFunc("POST /api/images/upload", s.handleImagesUpload)
	s.mux.HandleFunc("DELETE /api/images", s.handleImageDelete)
}

// ServeHTTP applies CORS middleware, logs the request, then dispatches to the router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS: only echo the Origin (and allow credentials) for explicitly
	// allow-listed cross-origin sites. The SPA and API are same-origin in both
	// prod (Apache) and dev (Vite proxies /api), so the allowlist is normally
	// empty and these headers are simply not sent — the browser doesn't consult
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
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// WebSocket connections must bypass session middleware and response
	// wrappers — coder/websocket needs the raw ResponseWriter for upgrade.
	if r.URL.Path == "/api/ws" {
		s.mux.ServeHTTP(w, r)
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

	// Wrap with SCS session middleware so session data is available in handlers.
	s.sessHandler.ServeHTTP(rw, r)
	duration := time.Since(start)

	// Live admin invalidation: after a successful admin-mutation POST, push a thin
	// "resource changed" signal so any admin viewing that resource refetches it via
	// REST (which re-applies the per-feature permission guard). The signal carries
	// no data — see broadcastResourceChanged. Public/auth/self-service paths and the
	// rich-realtime endpoints (game/cards/patterns/styles/settings, which emit their
	// own targeted events) are excluded by adminMutationResource.
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		if rw.status >= 200 && rw.status < 300 {
			if resource, ok := adminMutationResource(r.URL.Path); ok {
				s.broadcastResourceChanged(resource)
			}
		}
	}

	level := slog.LevelInfo
	if rw.status >= 500 {
		level = slog.LevelError
	} else if rw.status >= 400 {
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
// Limits the request body to 1MB to prevent memory abuse; passing w lets
// MaxBytesReader signal the server to close the connection on an oversized body.
// Returns the zero value and an error if decoding fails.
func readJSON[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var v T
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	err := json.NewDecoder(r.Body).Decode(&v)
	return v, err
}

// pathInt64 parses an int64 path wildcard (e.g. {id}). On a missing/invalid
// value it writes a 400 "Invalid <label> ID" and returns ok=false; handlers
// should return immediately. Shared by the resource-oriented (REST) routes.
func pathInt64(w http.ResponseWriter, r *http.Request, name, label string) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid "+label+" ID")
		return 0, false
	}
	return v, true
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

// checkCSRF is a defense-in-depth CSRF guard layered on top of the SameSite=Lax
// session cookie. It scrutinizes only cookie-authenticated, state-changing
// requests — the shape a cross-site page could drive using the victim's ambient
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
	writeError(w, http.StatusForbidden, "Cross-origin request blocked")
	return false
}

// sameHost reports whether an Origin's hostname matches the request's host. It
// compares hostnames (port-stripped) so a default-vs-explicit port — e.g. the dev
// setup's :5173 origin against a :8080 host, both "localhost" — isn't a false
// mismatch, while a different host (including a sibling subdomain) still fails.
func sameHost(originHost, requestHost string) bool {
	if h, _, err := net.SplitHostPort(requestHost); err == nil {
		requestHost = h
	}
	return strings.EqualFold(originHost, requestHost)
}

// wsSessionUser loads the account for a request that bypasses the session
// middleware — specifically the /api/ws upgrade, which is dispatched straight to
// the mux (coder/websocket needs the raw ResponseWriter, so it can't go through
// LoadAndSave/withUserCache). It reads the session cookie, loads the SCS session
// manually, and returns the authenticated, active account or nil. A request with
// no usable cookie session falls back to a personal access token (the WS upgrade
// accepts it via `?token=`), so the FFXIV plugin can open the admin channel too.
// Used to gate the privileged admin WebSocket channel.
func (s *Server) wsSessionUser(r *http.Request) *model.User {
	cookie, err := r.Cookie(s.sessions.Cookie.Name)
	if err != nil {
		return s.userFromToken(r)
	}
	ctx, err := s.sessions.Load(r.Context(), cookie.Value)
	if err != nil {
		return s.userFromToken(r)
	}
	id, ok := s.sessions.Get(ctx, "user_id").(int64)
	if !ok || id == 0 {
		return s.userFromToken(r)
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
		// No cookie session — fall back to a personal access token, so external
		// API clients (e.g. the FFXIV plugin) authenticate through the same guards.
		return s.userFromToken(r)
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

// broadcastResourceChanged notifies all admin clients that a named admin resource
// changed, so any admin currently viewing it refetches via REST. It deliberately
// carries no payload: the refetch re-applies that feature's permission guard
// (keeping authorization fresh, unlike a per-connection cached check) and keeps
// the REST handler the single source of truth for the data's shape. Rich,
// high-frequency streams (the bingo game, cards, patterns, theme, settings) emit
// their own targeted broadcasts instead — see adminMutationResource.
func (s *Server) broadcastResourceChanged(resource string) {
	s.hub.BroadcastToAdmins(struct {
		Type     string `json:"type"`
		Resource string `json:"resource"`
	}{Type: "resource_changed", Resource: resource})
}

// adminMutationResource maps a successful admin-mutation request (POST/PUT/PATCH/
// DELETE) to the frontend resource key to refetch, returning ok=false when the
// path should emit no invalidation. It keys on the first path segment after
// /api/, so every resource-oriented sub-path (/{id}, /{id}/entries/{entryId},
// /{id}/{verb}, …) is covered automatically. Public, auth, self-service
// (/api/account), and the rich-realtime endpoints (game/cards/patterns/styles/
// settings) are intentionally absent — they broadcast their own targeted events
// (or none). The public raffle sign-up (.../enter) is excluded explicitly; it
// broadcasts "raffles" itself.
func adminMutationResource(path string) (string, bool) {
	seg, _, _ := strings.Cut(strings.TrimPrefix(path, "/api/"), "/")
	switch seg {
	case "garapons", "affiliates", "tea-rooms", "stamp-rallies", "presets", "users", "winners-log", "fonts":
		return seg, true
	case "raffles":
		if strings.HasSuffix(path, "/enter") {
			return "", false // public sign-up self-broadcasts
		}
		return "raffles", true
	case "announcements", "announcement-types", "announcement-roles":
		return "announcements", true
	case "book-clubs":
		return "bookclub", true
	case "carrd":
		return "carrd", true
	case "images", "image-categories":
		return "images", true
	}
	return "", false
}

// cardEntry is the lightweight JSON shape for card lists.
type cardEntry struct {
	ID         string `json:"id"`
	PlayerName string `json:"player_name"`
	Details    string `json:"details"`
	CreatedAt  string `json:"created_at"`
}

// broadcastCards sends the updated card list to all WebSocket clients. It carries
// the same shape as GET /api/cards (id + player_name + details) so admins viewing
// the Manage Cards page keep their player-assignment indicators when the list is
// replaced by this broadcast — sending IDs alone would blank them out.
func (s *Server) broadcastCards() {
	cards, err := s.store.ListCardIDsWithNames()
	if err != nil {
		return
	}
	entries := make([]cardEntry, len(cards))
	for i, c := range cards {
		entries[i] = cardEntry{ID: c.ID, PlayerName: c.PlayerName, Details: c.Details, CreatedAt: c.CreatedAt}
	}
	s.hub.Broadcast(struct {
		Type  string      `json:"type"`
		Cards []cardEntry `json:"cards"`
	}{Type: "cards_update", Cards: entries})
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

// broadcastStyleUpdate sends a style_update message with the active theme's CSS
// and decorative flourishes to all clients (empty strings when the theme is
// cleared), so the live board/last-called flourishes update without a reload.
func (s *Server) broadcastStyleUpdate(css, boardFlourish, numberFlourish string) {
	s.hub.Broadcast(struct {
		Type           string `json:"type"`
		CSS            string `json:"css"`
		BoardFlourish  string `json:"board_flourish"`
		NumberFlourish string `json:"number_flourish"`
	}{Type: "style_update", CSS: css, BoardFlourish: boardFlourish, NumberFlourish: numberFlourish})
}
