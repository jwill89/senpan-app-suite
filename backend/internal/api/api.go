// Package api registers the JSON API on huma: typed operations with validation, RFC
// 9457 problem details for every error, and the OpenAPI 3.1 document the SPA and the
// plugin are built against (`go run . openapi`, committed as
// frontend/src/api/openapi.json).
//
// Every route under /api is an operation here; internal/server supplies the session,
// CSRF and access-log layers around them.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"app-suite/internal/anilist"
	"app-suite/internal/db"
	"app-suite/internal/discord"
	"app-suite/internal/game"
	"app-suite/internal/live"
	"app-suite/internal/media"
	"app-suite/internal/model"
	"app-suite/internal/store"
)

// Prefix is the path every operation lives under.
const Prefix = "/api/v1"

// ContractVersion is the API contract's own version, apart from the app's: it moves
// only when the API surface changes, and the document carries it
// as info.version.
const ContractVersion = "5.0.0"

// SessionCookie is the session cookie's name, for the document's security scheme.
const SessionCookie = "session"

// Deps is what the operations need, built in main and passed in.
type Deps struct {
	Store *store.Store
	// Version is the app's version (internal/version).
	Version string
	Logger  *slog.Logger
	// Live is the live feed every change is published to, and the streams read; nil
	// publishes nothing.
	Live *live.Hub
	// TurnstileSiteKey is the Cloudflare Turnstile site key the sign-in and
	// sign-up pages show, "" when the check is off.
	TurnstileSiteKey func() string
	// LogFile is the rotating JSON log's path, "" when the log goes to stdout only.
	LogFile func() string
	// Sessions is the session manager whose session every request's context carries
	// (the server's LoadAndSave), so signing in and out happen on it.
	Sessions *scs.SessionManager
	// SignInLimit counts failed sign-ins, RegisterLimit every sign-up and
	// PasskeyLimit every passkey sign-in started, each by client address.
	SignInLimit, RegisterLimit, PasskeyLimit Limiter
	// CardRequestLimit counts every personal card request, by client address.
	CardRequestLimit Limiter
	// RaffleEntryLimit counts every public raffle sign-up and RaffleLookupLimit every
	// public entry search, each by client address.
	RaffleEntryLimit, RaffleLookupLimit Limiter
	// GaraponDrawLimit counts every public garapon draw, by client address, before the
	// drawing link is looked up.
	GaraponDrawLimit Limiter
	// StampGuessLimit counts the wrong stamp passwords tried, by client address.
	StampGuessLimit Limiter
	// RallyLimit counts every public stamp rally sign-up and card lookup, by client address.
	RallyLimit Limiter
	// DashboardLimit counts every festival dashboard read, by client address.
	DashboardLimit Limiter
	// EmaLimit counts every public plaque submission and artist request, by client
	// address, before the request is read.
	EmaLimit Limiter
	// WebRoot is the folder the uploads live under; the plaques waiting for review are
	// kept in images/ema_plaques within it.
	WebRoot string
	// Images is the image library, Carrd the Carrd projects and Fonts the uploaded
	// fonts, each over the web root.
	Images *media.Images
	Carrd  *media.Carrd
	Fonts  *media.Fonts
	// Game is the bingo game: its state, its commands, and the live events each one
	// publishes.
	Game *game.Service
	// CardsChanged says the cards changed (made, edited, approved, protected or
	// deleted), so the game reads them afresh before it next checks for winners.
	CardsChanged func()
	// Turnstile reports whether a Cloudflare Turnstile response passes, or true when
	// the check is off.
	Turnstile func(ctx context.Context, response, ip string) bool
	// Discord posts to Discord webhooks.
	Discord *discord.Sender
	// AniList looks titles up for the book clubs.
	AniList *anilist.Client

	// fonts is the Google Fonts list kept between requests; New makes it.
	fonts *googleFontList
}

// Limiter counts attempts by key (a client address) within a window.
type Limiter interface {
	// RetryAfter is how long until key may try again: 0 when it may now.
	RetryAfter(key string) time.Duration
	// Fail counts an attempt against key.
	Fail(key string)
	// Reset forgets key's attempts (a sign-in that worked).
	Reset(key string)
}

// Request is what the server knows of a request that an operation can't read for
// itself.
type Request struct {
	// IP is the client's address, which the rate limits count by.
	IP string
	// Site is the scheme and host the request was made to, WebAuthn's relying party.
	Site string
	// ByToken is whether the caller is signed in by a personal access token rather
	// than a session.
	ByToken bool
	// Account reads the signed-in account afresh, past the request's own memo: its
	// session or token from the store, then the account behind it, nil when either
	// is gone or the account may no longer sign in. A stream checks it while open.
	Account func(ctx context.Context) *model.User
}

type requestKey struct{}

// WithRequest returns ctx carrying r; the server's session layer adds it to every
// API request.
func WithRequest(ctx context.Context, r Request) context.Context {
	return context.WithValue(ctx, requestKey{}, r)
}

// requestOf is the Request ctx carries, or none.
func requestOf(ctx context.Context) Request {
	r, _ := ctx.Value(requestKey{}).(Request)
	return r
}

type whoKey struct{}

// WithWho returns ctx carrying who, which answers the signed-in account behind the
// request (by its session or a personal access token), or nil. The server's session
// layer puts it on every API request; it is asked only by an operation that needs
// it, so a public one costs no account lookup.
func WithWho(ctx context.Context, who func() *model.User) context.Context {
	return context.WithValue(ctx, whoKey{}, who)
}

// Who is the signed-in account a request carries (WithWho), or nil.
func Who(ctx context.Context) *model.User {
	if who, ok := ctx.Value(whoKey{}).(func() *model.User); ok && who != nil {
		return who()
	}
	return nil
}

// Either a session cookie or a bearer token satisfies a protected operation.
var secured = []map[string][]string{{"session": {}}, {"token": {}}}

// New registers every operation on the mux and returns the API for its document.
func New(mux *http.ServeMux, d Deps) huma.API {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	d.fonts = &googleFontList{}
	// Parts of a form up to 16 MiB stay in memory, so the artist's plaque and a visitor's
	// are never written to disk; larger uploads spill to temporary files that net/http
	// removes after the request. Set once, since every server reads the same variable.
	multipartMemory.Do(func() { humago.MultipartMaxMemory = 16 << 20 })
	api := humago.NewWithPrefix(mux, Prefix, config())
	h := handlers{Deps: d}
	h.registerVersion(api)
	h.registerConfig(api)
	h.registerSettings(api)
	h.registerLogs(api)
	h.registerStyles(api)
	h.registerSignIn(api)
	h.registerPasskeys(api)
	h.registerMe(api)
	h.registerUsers(api)
	h.registerPatterns(api)
	h.registerPresets(api)
	h.registerTeaRooms(api)
	h.registerAffiliates(api)
	h.registerAnnouncements(api)
	h.registerBookClubs(api)
	h.registerWinners(api)
	h.registerCards(api)
	h.registerGame(api)
	h.registerBoard(api)
	h.registerRaffles(api)
	h.registerGarapons(api)
	h.registerStampRallies(api)
	h.registerFestivalMaps(api)
	h.registerFestivals(api)
	h.registerEma(api)
	h.registerImages(api)
	h.registerCarrd(api)
	h.registerFonts(api)
	h.registerLive(api)
	return api
}

// multipartMemory guards the one-time setting of humago.MultipartMaxMemory.
var multipartMemory sync.Once

// config is huma's with no document, docs or schema routes (the
// document is committed, and the clients are built from it), and no $schema link in
// a response.
func config() huma.Config {
	cfg := huma.DefaultConfig("Senpan App Suite", ContractVersion)
	cfg.OpenAPIPath = ""
	cfg.DocsPath = ""
	cfg.SchemasPath = ""
	cfg.CreateHooks = nil
	cfg.Info.Description = "The Senpan App Suite's API, used by the SPA and the Senpan Admin Companion plugin. " +
		"Errors are RFC 9457 problem details; fields are camelCase; times are RFC 3339 in UTC."
	cfg.Servers = []*huma.Server{{URL: Prefix}}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"session": {Type: "apiKey", In: "cookie", Name: SessionCookie, Description: "The SPA's sign-in session."},
		"token":   {Type: "http", Scheme: "bearer", Description: "A personal access token (Account -> Access Token), as the plugin sends."},
	}
	return cfg
}

// Spec renders the OpenAPI document both clients are built against, without a
// database or a running server.
func Spec() ([]byte, error) {
	b, err := json.MarshalIndent(Document(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Document is the OpenAPI document both clients are built against, and the one the
// tests hold answers to (apitest), built without a database or a running server.
func Document() *huma.OpenAPI {
	api := New(http.NewServeMux(), Deps{})
	nullableRefs(api.OpenAPI().Components.Schemas)
	return api.OpenAPI()
}

// nullableRefs says in the document which object fields can be null. huma writes a
// pointer to a struct as a bare reference, which tells a client the value is always
// there, so a strict generated client refuses the null the server sends. For a
// document being rendered only, never a running server: requests are checked
// against these same schemas.
func nullableRefs(reg huma.Registry) {
	for name, s := range reg.Map() {
		t := reg.TypeFromRef("#/components/schemas/" + name)
		if t == nil || t.Kind() != reflect.Struct {
			continue
		}
		for _, f := range reflect.VisibleFields(t) {
			key, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
			p := s.Properties[key]
			if p == nil || p.Ref == "" || f.Type.Kind() != reflect.Pointer || strings.Contains(opts, "omitempty") {
				continue
			}
			s.Properties[key] = &huma.Schema{
				OneOf:       []*huma.Schema{{Ref: p.Ref}, {Type: "null"}},
				Description: p.Description,
			}
		}
	}
}

type handlers struct {
	Deps
}

// read is the queries on the read pool.
func (h handlers) read() *db.Queries { return db.New(h.Store.ReadDB()) }

// write is the queries on the write pool, for a single statement; several go in a
// transaction (begin).
func (h handlers) write() *db.Queries { return db.New(h.Store.DB()) }

// begin opens a write transaction (BEGIN IMMEDIATE, the write pool's) and returns
// queries bound to it.
func (h handlers) begin(ctx context.Context) (*sql.Tx, *db.Queries, error) {
	tx, err := h.Store.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	return tx, db.New(tx), nil
}

// fail logs an unexpected error once, here, and answers a generic 500: its message
// never reaches the client, so it can't leak a query, a path or a stack. A query
// cancelled because its client went away is not a fault of the server's, so it is
// logged at Debug.
func (h handlers) fail(where string, err error) error {
	level := slog.LevelError
	if errors.Is(err, context.Canceled) || store.Interrupted(err) {
		level = slog.LevelDebug
	}
	h.Logger.Log(context.Background(), level, "request failed", "where", where, "error", err)
	return huma.Error500InternalServerError("Something went wrong on the server.")
}

// public marks an operation anyone may call whose answer is the same for everyone:
// it is cached by the baseline's revalidation (no-cache, with the ETag and 304 the
// server's /api layer adds), where every other answer is private, no-store.
func public(ctx huma.Context, next func(huma.Context)) {
	ctx.SetHeader("Cache-Control", "no-cache")
	next(ctx)
}

// signedIn lets through a signed-in, active account (a session or a personal access
// token); anyone else gets 401.
func (h handlers) signedIn(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if h.who(ctx) == nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Sign in to continue.")
			return
		}
		next(ctx)
	}
}

// permitted lets through an admin, who holds every permission, or an account granted
// any of perms: 401 when nobody is signed in, 403 when the account lacks them.
func (h handlers) permitted(api huma.API, perms ...string) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		u := h.who(ctx)
		switch {
		case u == nil:
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Sign in to continue.")
		case !Permitted(u, perms...):
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "Your account doesn't have access to this.")
		default:
			next(ctx)
		}
	}
}

// admin lets through an admin: 401 when nobody is signed in, 403 for anyone else.
func (h handlers) admin(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		u := h.who(ctx)
		switch {
		case u == nil:
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Sign in to continue.")
		case !u.IsAdmin:
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "Only an admin can do this.")
		default:
			next(ctx)
		}
	}
}

// session lets through an account signed in by its session: 401 when nobody is
// signed in, 403 for a personal access token, which can't make or revoke the
// account's credentials, so a stolen token can't keep itself alive or lock the
// owner out.
func (h handlers) session(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		switch {
		case h.who(ctx) == nil:
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Sign in to continue.")
		case requestOf(ctx.Context()).ByToken:
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "An access token can't do this. Sign in on the site.")
		default:
			next(ctx)
		}
	}
}

// tooMany is the 429 a rate limit answers, saying when to try again.
func tooMany(wait time.Duration, msg string) error {
	seconds := int((wait + time.Second - 1) / time.Second)
	return huma.ErrorWithHeaders(huma.Error429TooManyRequests(msg),
		http.Header{"Retry-After": {strconv.Itoa(max(seconds, 1))}})
}

// retryAfter asks a limiter how long key must wait; a missing limiter never holds
// anyone back.
func retryAfter(l Limiter, key string) time.Duration {
	if l == nil {
		return 0
	}
	return l.RetryAfter(key)
}

func (handlers) who(ctx huma.Context) *model.User {
	return Who(ctx.Context())
}

// Permitted reports whether u may use a page guarded by any of perms: an admin holds
// every permission; anyone else needs one of perms among their own.
func Permitted(u *model.User, perms ...string) bool {
	if u == nil {
		return false
	}
	if u.IsAdmin {
		return true
	}
	for _, have := range u.Permissions {
		for _, want := range perms {
			if have == want {
				return true
			}
		}
	}
	return false
}
