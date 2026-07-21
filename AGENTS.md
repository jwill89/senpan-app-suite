# AGENTS.md

Guidance for AI coding agents working in this codebase.

## Quick orientation

Go + SQLite backend serving a Vue 3 + TypeScript single-page frontend built with
Vite. The backend binary is built from `backend/` and serves only the API/WebSocket
on `:8080`; the built frontend (`frontend/dist/`) is served statically (Apache),
with `/api/*` proxied to the Go server.

Beyond bingo, the app is a small "suite" for a Discord community: **raffles**,
**book clubs** (reading lists pulled from AniList),
and **announcements** — the latter two post **Discord embeds** (manually or on a
schedule via background goroutines). It also hosts images for external **Carrd**
sites and admin-uploaded **fonts**.

A third surface — the **FFXIV Dalamud plugin** (`plugins/SenpanCompanion/`,
C#/.NET 10 + ImGui, published as *Senpan Admin Companion*) — is a **second admin
UI** over the same server. FFXIV gives plugins no client↔client networking, so the
Go server stays the single source of truth: every plugin action goes through the
same REST/WebSocket API and still broadcasts to the website. See **FFXIV plugin
(Dalamud)** below.

**At a glance:** Vue 3 SFCs + TypeScript (strict) · Pinia (setup stores) · Vue
Router (history mode, lazy routes) · Go 1.26+ stdlib HTTP · SQLite (WAL) ·
coder/websocket · per-user accounts (argon2id) + per-page permissions ·
embedded tzdata + background schedulers (Discord embeds) · Vitest + Vue Test
Utils · C#/.NET 10 Dalamud plugin (ImGui) · GitHub Actions CI.

**Auth model (read this first):** the admin area is now gated by **per-user
accounts**, not a single shared password. Accounts log in with username +
password (argon2id hash, `internal/auth`), are created **inactive** via a hidden
registration page, and an admin activates them and grants **per-page
permissions** (one key per admin page). Admins implicitly hold every permission.
A bootstrap `admin`/`admin` account is seeded by migration v22 and **must be
rotated immediately**. See **Authentication & authorization** below.

```
├── frontend/                         ← Vue 3 + TypeScript SPA (Vite)
│   ├── index.html                    ← Vite entry (mounts #app)
│   ├── package.json                  ← deps: vue, pinia, vue-router, vue-draggable-plus (sortablejs), markdown-it, @milkdown/crepe (Milkdown "Crepe" WYSIWYG authoring), @awesome.me/kit-… (FA Pro icons) + @fortawesome/fontawesome-svg-core + @fortawesome/vue-fontawesome (the <font-awesome-icon> component), @ckpack/vue-color, vue3-emoji-picker, html-to-image
│   ├── vite.config.ts                ← build → dist/, dev proxy for /api, manualChunks, strip dist/images
│   ├── tsconfig*.json                ← TS project refs (app + node)
│   ├── public/                       ← copied verbatim into dist/ at build
│   │   └── images/                   ← logo, favicon, share banner (dev only; stripped from dist — served from doc root in prod)
│   ├── vitest.config.ts              ← Vitest config (jsdom, @ alias; separate from vite.config so tests skip PWA/visualizer)
│   └── src/
│       ├── main.ts                   ← createApp + Pinia + Router + registers the global <font-awesome-icon> component (imports assets/app.css + lib/fontawesome; registers 401 handler)
│       ├── App.vue                   ← root shell: <RouterView>, WebSocket lifecycle, toast, route progress bar
│       ├── assets/app.css            ← stylesheet index: @imports assets/styles/*.css (tokens, base, utilities, components, player, admin, responsive); imported in main.ts so Vite content-hashes it
│       ├── router/index.ts           ← Vue Router (history mode): route map, lazy route components, admin auth guard
│       ├── lib/                      ← framework-agnostic helpers
│       │   ├── api.ts                ← typed fetch client (credentials, JSON, error extraction, global 401 handler)
│       │   ├── endpoints.ts          ← typed endpoint layer over api() — one fn per backend path (stores call these)
│       │   ├── ws.ts                 ← WsClient: reconnect back-off + keepalive ping
│       │   ├── markdown.ts           ← markdown-it renderer (lazy-loaded; replaces CDN marked.js)
│       │   ├── fontawesome.ts        ← SVG-core icon library (the used Pro-kit icons added once; rendered via the global <font-awesome-icon> component — no dom.watch)
│       │   ├── theme-tokens.ts       ← theme design-token metadata + token→:root CSS (tokensToCss)
│       │   ├── theme.ts              ← header-font + custom-CSS <head> injection; registers uploaded-font @font-face rules (metric-clamped to tame oversized vertical metrics)
│       │   ├── assets.ts             ← resolves server image paths (images/...) to root-absolute URLs
│       │   ├── exportCard.ts         ← player card → framed PNG export (html-to-image capture + canvas composite)
│       │   ├── sound.ts              ← opt-in draw chime (Web Audio synth) + haptics for the player view
│       │   ├── datetime.ts           ← UTC ⇄ <input datetime-local> (wall-clock) conversion; IANA tz helpers for announcements
│       │   ├── freshness.ts          ← createFreshness(): keyed time-gate so re-entered admin tabs skip redundant refetches (snappy navigation)
│       │   └── constants.ts          ← stamp shapes/colors, column helpers, fallback fonts, BOOK_CLUBS registry + per-club webhook setting keys
│       ├── types/
│       │   ├── api.generated.ts      ← tygo-generated from Go model — GITIGNORED, DO NOT EDIT (run `npm run gen:types`)
│       │   └── api.ts                ← re-exports + hand-written request/response/WS envelopes
│       ├── stores/                   ← Pinia stores (ui, app, auth, users, player, game, cards, patterns, presets, styles, raffles, affiliates, garapons, stampRallies, images, fonts, bookclub, carrd, announcements, logs, admin)
│       ├── composables/
│       │   ├── useWebSocket.ts       ← wires WsClient → stores (message dispatch)
│       │   └── usePwaInstall.ts      ← beforeinstallprompt capture + install/standalone state for the PWA "Install" affordance
│       ├── components/
│       │   ├── common/               ← BingoBoard, CalledNumbers, PatternMini, ModalOverlay, ConfirmModal, ToastNotification, LoadingSpinner, RouteProgressBar, MarkdownEditor (WYSIWYG), AppFooter, CornerFlourish
│       │   │   └── ui/                ← admin UI primitives (presentational, render stable themeable classes). Forms/tables: AdminPanel, FormField, FormRow, FormActions, DataTable, PaginationBar, EmptyState. Manager model: ManagerView (list page shell), ListRow (item row, actions far-right), SubPageHeader (Back sub-page header), SearchInput. Shared widgets: PatternPicker (v-model selected pattern ids — search + category filter + Select-All + collapse-all over a grouped collapsible checkbox grid; used by GameTab + the Preset editor), ImageField (upload-or-reuse-an-image field; announcement forms), ColorPicker (lazy vue-color Chrome wrapper + .color-picker skin; player stamp-colour modal). Every admin "manage items" tab routes through these for one consistent structure
│       │   ├── player/               ← Stamp{Shape,Color,Opacity} pickers, WinPatternsPanel
│       │   └── admin/                ← AdminSidebar + one component per tab + modals (CardPreview, EndGame, WinnerVerify, HalftimePrompt) + ThemeTokenEditor. Tabs: Game, Cards, WinnersLog, Patterns (one manager unifying the patterns list + New Pattern / Manage Categories sub-pages), Presets, RaffleForm, Raffles, Announcements, Affiliates + AffiliateForm, BookClub (one generic tab serves every club), Garapon + GaraponForm, StampRallies + StampRallyForm, Settings, Themes, Images, Users (admin-only account+permission manager), Logs (admin-only live server-log viewer — typed/colored columns, filters, live tail, DEBUG toggle), Fonts, CarrdUpload; PlacementEditor (shared %-based image placement widget for Garapon/Stamp Rally forms)
│       ├── views/                    ← HomeView, PlayerView, RafflesView, RaffleDetailView, GaraponView (public token-gated draw), StampCardView (public token-gated stamp card), AdminLoginView, RegisterView (hidden), NoAccessView (active account, no granted pages), AdminView
│       └── **/*.test.ts              ← Vitest unit/component tests, colocated next to the code they cover
├── .github/workflows/ci.yml          ← CI: frontend (lint·typecheck·test·build) + backend (lint·build·vet·test·govulncheck) + plugin (build·format·lint) + release (main: auto tag + GitHub Release per component on version bump)
├── .github/scripts/release.sh        ← the release job's script: reads each component's version, tags <Component>-v<version> + publishes a Release with that component's CHANGELOG.md section
├── deploy/                           ← Apache deploy artifacts (.htaccess + persistent images/ + README)
├── backend/                          ← Go backend
│   ├── main.go                       ← Entry point: flags, DB init, server start
│   ├── tygo.yaml                     ← Go→TS type generation config (run `npm run gen:types`)
│   ├── go.mod / go.sum               ← Module deps (alexedwards/scs, coder/websocket, ncruces sqlite, golang.org/x/crypto for argon2id)
│   └── internal/
│       ├── auth/password.go          ← argon2id Hash()/Verify() (PHC strings); own package so store (seed) + server (login) share it without an import cycle
│       ├── model/model.go            ← Domain types (User, Card, Pattern, GamePreset, BingoGame, BingoGameState, Raffle, ReadingList(+Item/Source), AnnouncementType, AnnouncementRole, Announcement(+Button), etc.)
│       ├── store/                    ← Store struct wrapping *sql.DB; one file per feature's typed CRUD
│       │   ├── store.go              ← Store struct, New()/Close(), pragmas, shared helpers (one domain per file: users.go, cards.go, winners.go, patterns.go, games.go, settings.go, styles.go, presets.go, raffles.go, bookclubs.go, announcements.go)
│       │   ├── users.go              ← users CRUD: create (inactive), get/list, set active/admin/permissions/password, delete (password hash stays here, never on model.User)
│       │   ├── bookclubs.go          ← reading_lists + reading_list_items CRUD
│       │   ├── announcements.go      ← announcement_types + announcement_roles + announcements CRUD + due/advance helpers (scheduler)
│       │   └── migrate.go            ← Schema versioning + migrations (PRAGMA user_version)
│       ├── bingo/
│       │   ├── card.go               ← Card/board generation, ID generation, LetterForNumber
│       │   └── game.go               ← bingo.Service (start, draw, end, state, winner matching, caching, auto-draw state + HalftimeThreshold); the auto-draw *loop* lives in server/game.go (it needs the hub to broadcast)
│       ├── ws/hub.go                 ← WebSocket hub, client pumps, broadcast (player/admin channels; BroadcastLog = lossy live-log fan-out to true-admin (IsAdmin) clients only, never disconnects on a full buffer; isAdmin bit refreshed on the revalidate tick)
│       ├── logging/logging.go        ← slog setup: JSON handler → stdout + a rotating file (timberjack; daily-midnight rotation, zstd) + a live-tail tap (io.Writer forwarding each line); runtime-settable level via slog.LevelVar
│       └── server/
│           ├── server.go             ← Server struct (deps, routes, CORS, JSON helpers, broadcast helpers) + auth helpers (currentUser/isAdmin/requireAuth/requireAdmin/requirePermission) + the request-logging middleware (structured JSON; actor identity + capability-token redaction; client IP via loopback-gated CF-Connecting-IP)
│           ├── actor.go              ← per-request actor for the access log: withActor resolves session/PAT user + Cloudflare verified-bot (cf-verified-bot / x-verified-bot) → auth/user/bot log fields
│           ├── logredact.go          ← redact capability tokens (garapon/stamp-card/font paths, PAT ?token=, Referer) to a short SHA-256 correlation hash in log lines
│           ├── auth.go               ← GET/POST /api/auth (login/logout, argon2id verify, rate-limited) + POST /api/register (hidden, creates inactive accounts)
│           ├── users.go              ← GET/POST /api/users (admin user management) + POST /api/account (self-service change-password)
│           ├── permissions.go        ← page-permission key constants, validPermissions(), userHasPermission(), requireAnyBookClub(); bookClubSlugs (keep in sync with BOOK_CLUBS)
│           ├── board.go              ← GET /api/board
│           ├── cards.go              ← GET/POST /api/cards
│           ├── game.go               ← GET/POST /api/game
│           ├── patterns.go           ← GET/POST /api/patterns, GET/POST /api/pattern-categories
│           ├── presets.go            ← GET/POST /api/presets (reusable game templates)
│           ├── styles.go             ← GET/POST /api/styles, GET /api/styles/active
│           ├── raffles.go            ← GET/POST /api/raffles, raffle entries (add/mark-paid/delete/pick), image upload
│           ├── bookclubs.go          ← reading lists + items, AniList lookup proxy, cover upload, Discord publish; book-club registry + per-club webhook setting keys (init)
│           ├── announcements.go      ← announcement types + roles + announcements (send_now/skip_next), optional role tag + location, image upload/list, recurrence math + RunAnnouncementScheduler
│           ├── embeds.go             ← shared Discord embed schema + fluent builder + colour helper + postDiscordEmbed transport
│           ├── uploads.go            ← shared upload helpers: saveMultipartFile + saveSingleImageUpload (+ image rel-dir consts, isAllowedImageExt, safeImageUploadName)
│           ├── scheduler.go          ← runScheduler: shared ticker/sweep loop behind the announcement scheduler
│           ├── carrd.go              ← Carrd image-host projects/dirs/uploads under <webRoot>/carrd (System → Atelier → Carrd Upload)
│           ├── winners.go            ← GET/POST /api/winners-log (list + delete/delete_all), GET /api/winners-log/frequent
│           ├── logs.go               ← GET /api/logs (admin: tail the JSON log file, level/q/limit filters) + POST /api/logs/level (live DEBUG toggle); shared NDJSON parse in model.ParseLogEntry
│           ├── settings.go           ← GET/POST /api/settings (app title, draw delay, fonts, AniList URL, join prompt; secret per-club webhooks)
│           ├── fonts.go              ← fonts admin API: grouped GET /api/fonts, upload, per-file rename/delete, PATCH/DELETE /api/fonts/families/{base} (metadata / whole font)
│           ├── fontserve.go          ← protected font serving: tokenized public URLs (GET /api/fonts/pub/kit.css + /f/{token}), gated per font by its origin allowlist; kit filtered by Referer
│           ├── fontconvert.go        ← font GROUPS (base-name variants), WOFF2 conversion (tdewolff/font) into <webRoot>/fonts/.woff2/ (skipped when a WOFF2 was uploaded) + per-font meta (family/serve/origins) + startup migrations
│           ├── ratelimit.go          ← IP-based brute-force limiter for admin login
│           ├── tzdata.go             ← blank-imports time/tzdata so IANA timezones resolve on hosts without zoneinfo (Windows)
│           └── ws.go                 ← GET /api/ws (delegates to hub)
├── plugins/                          ← FFXIV Dalamud plugin (C#/.NET) — secondary admin UI over the server
│   ├── pluginmaster.json             ← Dalamud custom-repo index (hosted at /plugin/pluginmaster.json)
│   └── SenpanCompanion/              ← the plugin (DLL/InternalName "SenpanCompanionAdmin"; display "Senpan Admin Companion")
│       ├── Plugin.cs                 ← entry point: injected [PluginService]s, /senpan (+ /senpan config) command, window system
│       ├── Configuration.cs          ← persisted config: server URL + personal-access token
│       ├── Api/                      ← ApiClient (REST, Bearer pat_…) + ApiModels + LiveConnection (admin WS: game_draw/game_update/cards_update/yoever(_config)/auto_config/halftime_prompt)
│       ├── Services/                 ← Session (auth/perms), CardCache (coalesced, framework-thread), NearbyPlayers (IObjectTable), ChatSender (/tell), WinnerChime (winmm synth)
│       ├── Windows/                  ← ImGui tabs: TabBase + Bingo{Cards,Game,Winners}Tab + RaffleTab + MainWindow
│       ├── SenpanCompanion.csproj    ← Dalamud.NET.Sdk/15.0.0; <AssemblyName>SenpanCompanionAdmin; EnableWindowsTargeting (Linux CI)
│       └── .editorconfig             ← dotnet format rules + Roslynator lint severities (root; curated ruleset)
├── devdata/                          ← Local dev sandbox: SQLite DBs + built binaries + webroot uploads (gitignored)
```

## Architecture

**Layered**: HTTP handlers (`server`) → Game logic (`bingo`) → Data access (`store`) → SQLite.
All dependencies are wired in `main.go` and passed via structs (no globals, no singletons).

**Package responsibilities**:
- `model` — Pure data types with JSON struct tags. No logic, no imports beyond stdlib.
- `store` — Single `Store` struct wrapping `*sql.DB`. All database reads/writes. Returns typed structs, never `map[string]interface{}`.
- `bingo` — Stateless card/board generation functions + `GameService` for game lifecycle (start, draw, end, winner computation). Caches game state and card data in memory for performance.
- `ws` — WebSocket `Hub` for real-time broadcasts. Self-contained: manages client lifecycle, ping/pong, and message fan-out. Supports separate player/admin channels.
- `server` — `Server` struct implementing `http.Handler`. Holds Store, GameService, Hub, session store, web root. Registers routes using Go 1.26+ method-pattern routing (`"GET /api/auth"`). Owns **authentication & per-page authorization** (see below), the **Discord-embed** features (announcements, book-club reading lists) and the announcement **background scheduler**, the **AniList** lookup proxy, and the on-disk upload areas (raffle/announcement/book-club images + Carrd projects).
- `auth` — argon2id `Hash()` / `Verify()` over PHC-format strings. Its own package (depends only on `golang.org/x/crypto`) so both `store` (seeding the bootstrap admin) and `server` (login / change-password) can hash without an import cycle. Params: 64 MB / t=1 / p=4 (OWASP argon2id baseline); `Verify` is constant-time.
- `logging` — process-wide `slog` setup (leaf; imports only `model` + timberjack). Installs a JSON handler writing to stdout **and** a rotating file, holds the runtime level in a `slog.LevelVar` (`SetLevel`/`CurrentLevel`), and exposes a settable tail sink (`SetTailSink`) that forwards each finished log line to a callback. `main` wires that callback to `Hub.BroadcastLog` for the live tail. See the **Server logs** operational note below.

**Authentication & authorization**:
- **Accounts, not a shared password.** `users` table holds `username`, argon2id `password_hash` (only ever read in the store layer — never on `model.User`, so it can't leak through JSON), `is_admin`, `is_active`, and a JSON `permissions` array. Login (`POST /api/auth`) verifies the hash, is **IP rate-limited**, rejects inactive accounts, and stores `user_id` in the SCS session (token rotated on login to prevent fixation). A missing user and a bad password return the same generic error so usernames can't be enumerated.
- **Registration is hidden + gated.** `POST /api/register` (page `/admin/register`, linked nowhere) creates accounts **inactive, non-admin, no permissions**; an admin must activate + grant access before they can log in. The reserved `admin` username can't be registered.
- **Bootstrap admin.** Migration v22 seeds `admin`/`admin` (active, full admin) via `INSERT OR IGNORE` (never clobbers a rotated password). `main.go` logs a warning that it must be rotated. The `admin` account is protected: only it can change its own password (via `/api/account`), and no one can delete/deactivate/demote it.
- **Per-page permissions.** Each admin page has a permission key equal to its frontend `AdminTab` id (e.g. `bingo-cards`, `teahouse-raffles`, `bookclub-yaoi`). Admins implicitly hold all of them; non-admins hold only the keys granted to them. The **Users page (`system-users`) is intentionally not a grantable key** — it is admin-only (`requireAdmin`).
- **Guards (server.go).** `currentUser(r)` loads the active account from the store **on every request** (so permission/activation changes take effect immediately — no stale session snapshot). `requireAuth` (any active user), `requireAdmin` (admin only), `requirePermission(perm)` (admin or the granted key → 401 unauth / 403 forbidden), and `requireAnyBookClub` (shared book-club endpoints not tied to one slug, e.g. AniList lookup + cover upload). Every mutating admin handler calls one of these first.
- **Frontend gating mirrors the server.** `stores/auth.ts` holds `user`/`isAdmin`/`hasPermission`; the router guard checks `meta.tab` as the permission key (redirecting to the first allowed page, or `/admin/no-access` when none); the sidebar hides pages/sections the account can't reach; `AdminView` only fires the data loads the account is allowed. **This is convenience, not security** — the server enforces every endpoint regardless of what the UI shows.
- **WebSocket gating:** the `/api/ws` upgrade bypasses the session middleware (coder/websocket needs the raw `ResponseWriter`), so `handleWS` loads the session manually via `wsSessionUser`. **Player** connections (carrying a card `id`) are public; **admin** connections (no `id`) join the channel that streams draws immediately + winner card IDs, so they require an authenticated, active account — otherwise the draw-delay anti-peek could be bypassed. Admin connections only originate from admin views, which already require login.

**Discord embeds**: `embeds.go` holds the shared embed schema, a fluent `newEmbed()…build()` builder (auto-truncates to Discord's per-field limits, skips empty fields), a `#rrggbb`→int colour helper, and `postDiscordEmbed()` / `postDiscordWebhook()` transport (the latter also carries an optional `content` mention + `allowed_mentions`). Feature code (announcements, reading-list items) only assembles a builder chain — no feature has its own transport. Outbound calls (AniList + webhooks) share `bookclubHTTPClient` (15s timeout).

**Background scheduler**: `main.go` launches `RunAnnouncementScheduler` in a goroutine tied to a shutdown-cancelled context. It uses the ticker/sweep loop in `scheduler.go` (`runScheduler`): ticks every 30s (and sweeps once on startup to catch up after downtime), posts due announcements to their type's webhook, and is resilient — an announcement whose webhook is unset is left pending, a failed post is retried next tick, and `skip_next`/recurrence advance the cursor without losing the schedule. Wall-clock times are resolved against each announcement's IANA timezone (DST-safe) and stored as UTC; `tzdata.go` embeds the zone database so this works on Windows hosts too.

**Logging & observability**: the app logs **structured JSON** via `slog` (set up in the `logging` package). Output goes to **stdout** (captured by journald) and, when `-log-file` is set (default `/var/log/senpan/senpan.log`), to a **rotating file** (timberjack: daily-midnight rotation, zstd backups, bounded retention) — the file sink degrades gracefully to stdout-only if it can't be created (e.g. a read-only `/var/log` under `ProtectSystem=strict`; the systemd unit needs `LogsDirectory=senpan` — see `deploy/README.md`). The request-logging middleware records `method`/`path`/`status`/`duration`/`ip` per request, using the **real client IP** (`CF-Connecting-IP` behind Cloudflare, not the proxy's `[::1]`). Admins view logs **without SSH**: `GET /api/logs` tails the file (level/text/limit filters), and every line is also pushed to admin WebSocket clients as a `log` message so **System → Logs** live-tails them (see the tab in the frontend section). The minimum level is a `slog.LevelVar` flipped at runtime by `POST /api/logs/level` — turn **DEBUG** on to capture a richer `request detail` line per request plus DEBUG logs across the services, then back off, with no restart. The same NDJSON parse (`model.ParseLogEntry`) backs both the REST tail and the live feed.

**Key data flow — drawing a number**:
Admin clicks Draw → `POST /api/game {action:"draw"}` → `server.handleGameAction` →
`GameService.Draw()` → picks random uncalled number, computes winners (skipping already-known winners), caches in `games.winners_cache` →
returns `DrawResult` (drawn number + game state + winners) → handler writes JSON + broadcasts via WebSocket (admins immediately, players optionally delayed).


**Key data flow — real-time updates**:
After any state-changing POST (draw, start, end, card/pattern mutations), the handler broadcasts
the updated state to all connected WebSocket clients via `Hub.Broadcast()`, `Hub.BroadcastToPlayers()`, or `Hub.BroadcastToAdmins()`.

**Performance optimizations**:
- GameState is cached in `GameService` memory — invalidated on start/draw/end; board lookups avoid 3 DB queries.
- Card list is cached in `GameService` — invalidated on generate/delete; winner computation avoids full DB read.
- Winner computation skips already-known winners (only scans new cards on each draw).
- `FrequentWinners` query uses composite index `(player_name, logged_at)`.
- `ListWinnersLog` uses `COUNT(*) OVER()` window function (single query for data + total).
- SQLite connection pool allows 4 concurrent connections for WAL concurrent readers.

**Schema versioning**: `store/migrate.go` uses `PRAGMA user_version` to track schema version (currently **50**; v50 added the `game_presets.auto_call`/`auto_interval` columns for auto-run games). v22 added the `users` table + seeded the bootstrap admin; later migrations grew the announcements feature (roles, mention, location, thumbnail, dynamic dates, sort order) and dropped the retired `book_club_events` table (v28), moved themes to structured design tokens + added style flourishes (v34/v37), then shipped the **Garapon** festival lottery drum (v35–36), **Affiliates** (v38), the **Stamp Rally** (v39–41, including the v41 collected-log rebuild that keeps logs after a card/stamp is deleted), per-account **personal-access tokens** for the plugin/API (v42), and a `UNIQUE(game_id, number)` index on `called_numbers` as a duplicate-draw backstop (v43).
On the hot path (version == current), zero migration queries execute. Migrations run
incrementally only when the version is behind.

**Bingo card format**: 5×5 array `board[row][col]`; col 0=B(1-15) … col 4=O(61-75); centre `[2][2] = 0` (FREE).
**Pattern format**: 5×5 `[][]bool` grid; `true` = cell must be called for win; centre always counts.
**Card IDs**: 6-char alphanumeric (no ambiguous chars 0/O/1/I/l), enforced unique in DB.

## Design philosophy

The principles below are *why* the conventions exist — keep changes aligned with them.

1. **Layered & dependency-injected, no globals.** Handlers → game logic → store → SQLite, each layer depending only downward. Everything is wired in `main.go` and passed via structs (`Server` holds its deps); there are no package-level singletons. New shared state goes on a struct, not a global.
2. **Typed end-to-end.** Go domain types (`model`) are the single source of truth; tygo generates the TS types, and stores reach the backend only through the one typed `endpoints.ts` layer. The store returns typed structs, never `map[string]interface{}`. A change to a shape happens in `model` and flows outward — never hand-edit the generated types.
3. **The server is the security boundary; the client is convenience.** Every endpoint enforces its own auth/permission guard. The router guard, sidebar gating, and conditional data loads exist purely for UX — never assume hidden UI means a protected endpoint, and never move an authorization decision client-only.
4. **Theme fidelity is non-negotiable.** Themes override **only** the `:root` design tokens via a structured token editor — no free-form CSS, and class names are **not** a theme API. So: never hard-code colors/fonts (drive everything off tokens), and prefer (1) an existing object, (2) a token on existing markup, (3) a new shared object — page-scoped CSS is the last resort. A utility-first framework (Tailwind) is explicitly *not* the direction; object classes can now be refactored freely since themes don't target them.
5. **CSS models objects, not pages.** A small vocabulary of reusable primitives/objects (the `components/common/ui/` set, `.card`, `.chip`, `.badge`, the button system) is composed across features instead of per-page rules. Intent = fill colour only; never an outline/ghost (a surface-coloured control vanishes into its container — the recurring "ghost button" bug). Derive states with `color-mix`, don't multiply tokens.
6. **Fast by default.** Lazy routes + `manualChunks` keep the player/home payload tiny; the game service caches state/cards/details in memory and invalidates precisely; winner computation is incremental; list queries are index-backed and single-pass (`COUNT(*) OVER()`); the admin nav freshness-gate skips redundant refetches. Add an index/cache rather than an N+1.
7. **Resilient & idempotent.** Migrations are guarded (`hasColumn`, `INSERT OR IGNORE`) and run incrementally; the schedulers sweep on startup, leave unconfigured items pending, and retry failed posts without losing the cursor; optimistic UI reverts on failure. Assume restarts and partial failure.
8. **Registry-driven extensibility.** Book clubs and page permissions are declared once in a registry and wire up their routes, nav, settings, and guards automatically. Prefer adding a registry entry over copy-pasting a feature.
9. **Real-time is a broadcast side-effect.** State-changing POSTs persist, then broadcast over the WebSocket hub (player/admin channels). The HTTP response and the broadcast carry the same truth; clients reconcile from it rather than polling.
10. **Tests colocate and CI mirrors the local gate.** A green CI == the checks you run locally passed. Wire any new check into both the npm/go script **and** the workflow.

## Database tables

| Table | Purpose |
|---|---|
| `users` | `id INTEGER PK`, `username TEXT UNIQUE`, `password_hash TEXT` (argon2id PHC), `is_admin INTEGER`, `is_active INTEGER`, `permissions TEXT` (JSON array of page-permission keys), `created_at` — admin accounts |
| `cards` | `id TEXT PK`, `board_data TEXT` (JSON 5×5 array), `player_name TEXT`, `details TEXT` |
| `pattern_categories` | `id INTEGER PK`, `name`, `sort_order INTEGER` |
| `patterns` | `id INTEGER PK`, `name`, `pattern_data TEXT` (JSON), `sort_order INTEGER`, `category_id INTEGER` |
| `games` | `id INTEGER PK`, `status` (active/ended), `created_at`, `winners_cache TEXT` (JSON array of card IDs) |
| `game_patterns` | Snapshot of patterns when game started (game_id + pattern_id + name + data) |
| `called_numbers` | `game_id`, `number`, `call_order` |
| `settings` | `key TEXT PK`, `value TEXT` — key-value config (e.g. `game_details`, `active_style_id`, `app_title`, `default_draw_delay`, `frequent_winner_threshold`/`_hours`, `header_font`, `google_fonts_api_key`, `anilist_api_url`, `bingo_join_prompt`, and per-club reading-list `discord_webhook_url_<slug>`) |
| `styles` | `id INTEGER PK`, `name`, `tokens TEXT` (JSON token map), `created_at` |
| `game_presets` | `id INTEGER PK`, `name`, `pattern_ids TEXT` (JSON), `game_details TEXT`, `auto_call INTEGER` (start with auto-draw on), `auto_interval INTEGER` (seconds between auto draws), `created_at` — reusable game templates |
| `raffles` | `id INTEGER PK`, `title`, `description`, `rules`, `max_entries`, `signup_instructions`, `cost_per_entry`, `available_from`, `available_to`, `prize_image`, `status`, `winner_entry_id`, `created_at` |
| `raffle_entries` | `id INTEGER PK`, `raffle_id`, `character_name`, `world`, `num_entries`, `paid`, `created_at` |
| `winners_log` | `id INTEGER PK`, `logged_at`, `card_id`, `player_name`, `game_details`, `winning_patterns TEXT` (JSON) |
| `reading_lists` | `id INTEGER PK`, `club_slug`, `title`, `created_at` — a book club's named reading list |
| `reading_list_items` | `id INTEGER PK`, `list_id`, `cover_image`, `title`, `summary`, `format`, `genres`, `tropes`, `chapters`, `comments`, `sources TEXT` (JSON), `sort_order` |
| `announcement_types` | `id INTEGER PK`, `name`, `webhook_url`, `created_at` — a named Discord destination |
| `announcement_roles` | `id INTEGER PK`, `name`, `role_id` (Discord role snowflake), `created_at` — a taggable role an announcement can ping |
| `announcements` | `id INTEGER PK`, `type_id`, `title`, `details` (markdown), `image`, `color`, `location`, event window (`start_local`/`end_local` + computed `start_at`/`end_at`), schedule (`schedule_kind`, `timezone`, `once_local`, `schedule_minutes`, `schedule_weekdays`, `schedule_week_of_month`), `next_post_at`, `skip_next`, `active`, `last_posted_at`, `buttons TEXT` (JSON array of up to 5 Discord link buttons), `mention` (`""`/`everyone`/`role:<id>`), `created_at` |
| `affiliates` | `id INTEGER PK`, `name`, `owners TEXT` (JSON), `location`, `timezone`, `hours TEXT` (JSON opening-hours ranges), `details` (markdown), `logo`, `screenshot`, `created_at` — a partner establishment (Senpan Tea House → Affiliates) |
| `garapons` | `id INTEGER PK`, `title`, `details` (markdown), `grand_prize_image`, `status` (open/closed), `stamp_rally_id` (optional link), `created_at` — a festival lottery drum |
| `garapon_prizes` | `id INTEGER PK`, `garapon_id`, `name`, `ball_color`, `rate REAL`, `is_grand`, `sort_order` — a garapon prize tier |
| `garapon_players` | `id INTEGER PK`, `garapon_id`, `token TEXT UNIQUE`, `player_name`, `max_draws`, `stamp_card_id` (optional, for a linked rally), `created_at` — a per-player drawing link |
| `garapon_draws` | `id INTEGER PK`, `garapon_id`, `player_id` (nullable, `ON DELETE SET NULL`), `prize_id`, snapshots `player_name`/`prize_name`/`ball_color`, `drawn_at` — the draw log (survives link deletion) |
| `stamp_rallies` | `id INTEGER PK`, `title`, `card_image`, `not_stamped_image`, `available_from`/`available_to`, `details`, `redeem_instructions`, `status` (open/closed), `created_at` — a stamp-rally event (Festival → Stamp Rally) |
| `stamp_rally_stamps` | `id INTEGER PK`, `rally_id`, `affiliate_id` (nullable = Tea House default), `image`, `password`, `%`-based placement (`pos_x`/`pos_y`/`width`/`height`/`rotation`), `active_from`/`active_to`, `paused`, `sort_order` |
| `stamp_rally_prizes` | `id INTEGER PK`, `rally_id`, `name`, `image`, `%`-based placement (`pos_x`/`pos_y`/`width`/`height`/`rotation`), `sort_order` |
| `stamp_rally_cards` | `id INTEGER PK`, `rally_id`, `token TEXT UNIQUE`, `participant_name`, `completed`, `completed_at`, `created_at` — a per-participant card |
| `stamp_rally_collected` | `id INTEGER PK`, `rally_id`, `card_id`/`stamp_id` (nullable, `ON DELETE SET NULL`), snapshots `participant_name`/`stall_name`, `stamped_at`, `UNIQUE(card_id, stamp_id)` — the collected-stamp log (survives card/stamp deletion) |
| `user_tokens` | `user_id INTEGER PK`, `token_hash TEXT UNIQUE` (SHA-256; plaintext shown once), `token_prefix`, `created_at`, `last_used_at` — one personal-access token per account for the plugin/API bearer auth |

Indexes: `games(status)`, `called_numbers(game_id, call_order)`, `game_patterns(game_id)`, `raffle_entries(raffle_id)`, `winners_log(logged_at)`, `winners_log(player_name, logged_at)`, `cards(player_name)`, `reading_list_items(list_id)`, `announcements(type_id)`, `announcements(active, next_post_at)` (due sweep), `garapon_prizes(garapon_id)`, `garapon_players(garapon_id)`, `garapon_players(token)`, `garapon_draws(garapon_id)`, `garapon_draws(player_id)`, `stamp_rally_stamps(rally_id)`, `stamp_rally_prizes(rally_id)`, `stamp_rally_cards(rally_id)`, `stamp_rally_cards(token)`, `stamp_rally_collected(card_id)`, `stamp_rally_collected(stamp_id)`, `stamp_rally_collected(rally_id)`.

**On-disk uploads (not in the DB)**, all under `<webRoot>`:
- `images/<category>/` — the **central image host** (System → Images). Each category is a subfolder served at `/images/<dir>/<file>`. Three categories are PERMANENT (hardcoded): `images/raffles` (Raffle), `images/announcements_main` (Announcement Main), `images/announcements_thumb` (Announcement Thumbnail). Admins may add custom categories, tracked in a dotfile manifest `<webRoot>/images/.categories.json` (no DB table). Affiliate logos/screenshots, garapon/stamp-rally artwork, etc. are picked from these categories.
- `images/bookclub/` — reading-list item cover uploads (AniList covers stay remote)
- `fonts/` — admin-uploaded **font files** (`.ttf/.otf/.woff/.woff2/.eot`); files sharing a base name group into one logical FONT (its format variants). Licensed assets — NOT served statically: the Go server streams them via rotating tokenized URLs gated **per font** by its own origin allowlist (`fontserve.go`); the `https://fonts.senpan.cafe` vhost is a reverse proxy onto `/api/fonts/pub/` and external sites embed `kit.css` (filtered per site — see Deployment). A font with no uploaded WOFF2 gets one auto-converted into `fonts/.woff2/` (`fontconvert.go`, tdewolff/font; upload-time + startup backfill) — the served variant defaults to WOFF2, selectable per font
- `carrd/<project>/…` — Carrd image-host projects (images + `.mp3`/`.mp4`), each with a `.carrd.json` title sidecar; served cross-origin from `https://carrd.senpan.cafe` (needs CORS — see Deployment)

## Frontend (Vue 3 + TypeScript + Vite)

The frontend lives in `frontend/` as a Vite project with Vue 3 SFCs, TypeScript,
and Pinia state management. It began as a faithful migration of a legacy
single-file `index.html` + `assets/js/app.js` prototype (since removed; preserved
on a separate branch) and has since grown well beyond it — decomposed into
fine-grained components and stores.

**State (Pinia stores)**: `ui` (toasts, themed confirm dialog, route-loading
flag, realtime connection status), `app` (settings/fonts/active CSS), `auth`,
`player`, `game`, `cards`, `patterns`, `presets` (game templates), `styles`,
`raffles`, `affiliates` (partner establishments), `garapons` (festival lottery
drums + per-player draw links), `stampRallies` (stamp-rally events + cards/logs),
`images` (central image-host categories), `fonts`, `bookclub` (reading lists/items
per club), `carrd` (image-host projects), `announcements` (types + roles +
scheduled embeds), `admin` (sidebar highlight state + per-tab data loads). The root `App.vue` hosts
`<RouterView>` and owns the WebSocket lifecycle; `composables/useWebSocket.ts`
dispatches WS messages into the stores.

**Routing is Vue Router** (history mode, `router/index.ts`) — real linkable URLs,
not store-driven view switching:
- Public: `/`, `/play/:cardId`, `/raffles`, `/raffles/:id`, `/garapon/:token`
  (per-player Garapon draw — the unguessable token is the capability, no admin
  auth), `/stamp-card/:token` (per-participant Stamp Rally card, likewise
  token-gated), `/admin/login`, `/admin/register` (hidden — linked nowhere; admins
  share the URL to onboard a new account, which starts inactive).
- Admin: `/admin` (layout, `requiresAdmin`) with child routes grouped into five
  sidebar sections that the route paths mostly mirror — **bingo** (`bingo/{game,
  cards,winners-log,patterns,presets}`), **teahouse** ("Senpan Tea House":
  `teahouse/announcements`, `teahouse/affiliates`, and one `teahouse/bookclub/<slug>`
  route per registered club), **festival** (`festival/{garapon,stamp-rally}`; the
  Raffles manager now lives in this section too, though its route/id stays
  `teahouse/raffles`), **atelier** (`atelier/{fonts,carrd}`), and **system**
  (`system/{settings,themes,images,users}`), plus `no-access` (landing for an active
  account with no granted pages). The admin tabs are **child routes** of the
  `AdminView` layout, so the sidebar/topbar persist while the matched child renders
  the active tab. The per-club book-club routes are generated from the `BOOK_CLUBS`
  registry (`lib/constants.ts`) and all render the single generic `BookClubTab.vue`.
  Routes mirror the nav exactly (no legacy redirects) — an unknown `/admin/*` path
  falls through the catch-all to home.
- Every view + admin tab is a lazy `import()` so heavy deps (the Milkdown
  editor, vue-draggable-plus, markdown-it) split into on-demand chunks. A `router.onError`
  guard recovers from stale lazy-chunk 404s after a redeploy by doing one full
  browser load of the target (sessionStorage-guarded against loops).
- A global `beforeEach` guard enforces auth + **per-page permission**: it
  redirects unauthenticated users to `/admin/login` (with a `redirect` query),
  then checks the matched child's `meta.tab` as the permission key — admins pass
  everything, `system-users` and `system-logs` are admin-only, and a user lacking
  the key is sent to
  their first allowed page (or `/admin/no-access` when they have none). An
  already-logged-in user hitting `/admin/login` or `/admin/register` is bounced
  to their first allowed page. The guard also calls `admin.setTabFromRoute(meta.tab)`
  to sync the sidebar highlight + run that tab's data load. Unknown paths redirect home.
- `api.ts`'s global 401 handler (registered in `main.ts`) redirects to the login
  route with a "session expired" toast when any non-auth request 401s.

**Type sync**: TS domain types are generated from the Go `model` package via tygo
into `frontend/src/types/api.generated.ts` — this file is **gitignored** (each dev
regenerates it locally with `npm run gen:types`; CI/build regenerate as needed).
Never edit it by hand. Request/response/WebSocket envelopes are hand-written in
`types/api.ts`.

**Library choices** (migrated off CDNs):
- Markdown **rendering** → `markdown-it`, **lazy-loaded** via `useMarkdown()` (`lib/markdown.ts`); the ~100 KB parser is dynamic-imported on first render (`breaks: true` to match the old marked output).
- Markdown **authoring** → `MarkdownEditor.vue` wraps **Milkdown (Crepe)** (`@milkdown/crepe`), a WYSIWYG editor built via Crepe's tree-shakable `CrepeBuilder` (only the features used are imported, dropping the CodeMirror/LaTeX bundles). It's still a sizeable bundle, so both the library **and** its CSS are dynamic-imported on mount (kept out of the initial load); it maps its colours onto the app theme variables, is limited to a Discord-safe subset (inline formatting + headings/quotes/lists/dividers/links), and emits **markdown** (what we store and what Discord renders). Reused by raffles, book-club items, and announcements.
- Drag-and-drop → `vue-draggable-plus` (SortableJS) for category reorder + pattern reorder/cross-category move
- Theme editor → structured token editor (`ThemeTokenEditor.vue`): a colour swatch + value per `:root` design token, no free-form CSS (CodeMirror removed). Alpha tokens use the `@ckpack/vue-color` picker; solid tokens the native `<input type=color>`
- Icons → `@fortawesome/*` **Pro** packages via `@fortawesome/fontawesome-svg-core` + the `@fortawesome/vue-fontawesome` **component** (`<font-awesome-icon :icon="[prefix, name]" />`, registered globally in `main.ts`). Only the icons used are added to the library in `lib/fontawesome.ts`; templates reference them by `[prefix, name]` tuple (`['fad', …]` duotone, `['fas', …]` solid, `['fab', 'discord']` brands). Vue owns the rendered `<svg>`, so there is no `dom.watch()`/MutationObserver
- Card PNG export → `html-to-image` captures the live themed board, then a canvas composites the framed card (`lib/exportCard.ts`)

**Performance / tooling**:
- **Lazy routes**: every view + admin tab is a dynamic `import()` in `router/index.ts`, so heavy deps (the Milkdown editor, vue-draggable-plus, markdown-it) load only when their route is visited — the player/home payload stays small. `manualChunks` (vite.config) keeps shared vendors cached across route chunks.
- **PWA**: `vite-plugin-pwa` (`registerType: 'autoUpdate'`) emits `sw.js` + `manifest.webmanifest`; the SW precaches the app shell and falls back to `index.html` for SPA routes, with `/api/` and `/images/` denylisted. The deploy `.htaccess` exempts `sw.js`/`registerSW.js`/`*.webmanifest` from the immutable cache so updates land.
- **Route progress + loading UX**: a top progress bar (`RouteProgressBar.vue`, driven by `ui.routeLoading` from the router guards) shows during async navigation/lazy-chunk loads; stores expose per-action loading flags (`joining`, `drawing`, `starting`, …) that drive `LoadingSpinner.vue` + disabled buttons.
- **Global error handler**: `app.config.errorHandler` (`main.ts`) surfaces uncaught errors as a toast.
- **Accessible modals**: `ModalOverlay.vue` traps focus, restores it on close, supports Escape, and sets `role="dialog"`/`aria-modal`.
- **Lint/format**: ESLint flat config (`eslint.config.js`) + Prettier (`.prettierrc.json`); `npm run lint` (autofix) / `npm run lint:check` (no fix, used by CI) / `npm run format`. Bundle treemap via `npm run analyze` → `dist/stats.html`.

**Testing** (Vitest + Vue Test Utils, jsdom):
- Config in `vitest.config.ts` (kept separate from `vite.config.ts` so tests don't load the PWA/visualizer plugins). Tests **import `describe/it/expect/vi` from `vitest`** explicitly (`globals: false`) so they type-check with no extra global-types config.
- Test files are colocated as `src/**/*.test.ts`. Current coverage: `lib/{constants,api,endpoints,exportCard,theme,datetime,freshness,markdown,ws}.test.ts`, `stores/{player,ui,raffles,cards,patterns,announcements,auth,users}.test.ts`, `components/common/BingoBoard.test.ts`, `components/admin/AdminSidebar.test.ts` (mocks `vue-router`/`@/router` to assert accordion toggling + per-permission section hiding), `components/common/ui/{FormField,DataTable,PaginationBar,ManagerView,ListRow,SubPageHeader,SearchInput}.test.ts`. Backend mirrors this with `internal/auth/password_test.go`, `internal/store/users_test.go`, `internal/server/users_test.go`, and the existing per-domain `*_test.go`.
- Run: `npm run test` (CI), `npm run test:watch`, `npm run test:coverage` (v8).
- Patterns: mock `fetch` via `vi.stubGlobal` for `api.ts`; mock the `./api` module with `vi.hoisted` + `vi.mock` for `endpoints.ts`; Pinia stores use `setActivePinia(createPinia())`; components use `mount()` from `@vue/test-utils`. Pure helpers tested directly (e.g. `exportCard.ts` exports `parseInlineRuns`/`parseDetailParagraphs` for this).

**Public routes**: `/` · `/play/:cardId` · `/raffles` · `/raffles/:id` · `/garapon/:token` (token-gated) · `/stamp-card/:token` (token-gated) · `/admin/login` · `/admin/register` (hidden).
**Admin sections** (`adminSection`, sidebar highlight): `bingo` | `teahouse` | `festival` | `atelier` | `system`.
**Admin tabs** (`adminTab` / route): `bingo-game` · `bingo-cards` · `bingo-winners-log` · `bingo-patterns` · `bingo-presets` · `teahouse-announcements` · `teahouse-affiliates` · `bookclub-<slug>` (one per registered club, e.g. `bookclub-yaoi`, `bookclub-yuri`) · `festival-garapon` · `festival-stamp-rally` · `teahouse-raffles` (shown in the Festival section, id unchanged) · `system-settings` · `system-themes` · `system-images` · `system-users` (admin-only) · `system-logs` (admin-only) · `atelier-fonts` · `atelier-carrd`. Each tab id (except `system-users`/`system-logs`) doubles as its **page-permission key**.

**Player features**:
- Join by board ID; see bingo board, called numbers grid, and active win patterns
- Manually stamp cells (click to toggle); stamps persist in `localStorage` keyed by `stamps_{cardId}_{gameId}`
- Stamp customization: shape (blank default, heart, star, smiley, etc.), color (7 options), opacity slider, **custom uploaded image** (stored as a data URL in `localStorage`)
- **Export/save the board as a PNG card** (`lib/exportCard.ts`): captures the live themed `.board-wrap` via `html-to-image` (preserving theme, emoji + custom-image stamps), then composites it into a framed card (title/logo header + game-details footer) on a canvas and downloads it
- Real-time updates via WebSocket (draws, game start/end, style changes, halftime alerts)
- **Live-game feedback** (ambient only — never tracks the player's own board, by design, to preserve player agency): a "Last Called" announcement banner for the most recent draw, an opt-in draw **chime + vibration** (`lib/sound.ts`, persisted in `localStorage`), a **Live / Reconnecting** connection badge (off `ui.wsStatus`), and an end-of-game thank-you summary
- WebSocket reconnect with exponential back-off on disconnect
- Browse open raffles from home page (card shown only when open raffles exist); view raffle detail with prize image, markdown description/rules, and sign-up form (character name, world, number of entries); after sign-up see confirmation with total cost and sign-up instructions
- View closed raffles with winner announcement and total entry count

**Admin features**:
- Per-user account login (username + argon2id password, session-based auth, 24-hour cookie); accounts are activated and granted per-page access by an admin (see **Authentication & authorization**)
- **Game tab**: start game (select patterns with category filter + search, or apply a saved **preset**), draw numbers (optional player delay 0–60s; **press `Space`/`Enter` to draw**), live "Live" badge + elapsed-time clock, see called numbers, see winners; click winner ID to verify card with pattern-hit highlighting; frequent winners alert (3+ wins in 12h); end game with winner confirmation modal. **Auto-run**: a New Game "Auto-draw numbers" toggle + "Time Between Calls" interval (also on presets) starts the game auto-drawing; a live Auto-Draw on/off toggle + interval selector adjust it mid-game (never touches the preset). The server-side scheduler draws every `interval + player-delay`, pauses at half-time (the prompt is now server-driven — `halftime_prompt` — so it fires for auto draws too and across all admins), and stops the moment a winner is recognized. Auto state syncs via the `auto_config` WebSocket message
- **Cards tab**: generate cards (1–500), view as chips with player name indicators, click to preview board, edit player name/details, delete individual or all
- **Patterns tab** (`PatternsTab.vue`): one manager merging the former Categories/New/Edit tabs — category-grouped collapsible drag-reorder list with search + category filter; "+ New Pattern" (5×5 grid editor, duplicate detection) and "Manage Categories" (a `DataTable` of categories with Edit/Delete; add/edit opens a form with a Title + a Position dropdown — "At the beginning" / "After X" per category, plus "Keep current position" when editing — applied via the bulk-reorder endpoint) as Back sub-pages
- **Game tab pattern picker** (`GameTab.vue`): when starting a new game, patterns render exactly like the Patterns manager — collapsible category groups (non-draggable checkboxes) with search + category filter + select-all-visible — reusing the patterns store's `patternsByCategory` + shared collapse state
- **Presets tab**: CRUD reusable game templates (a named set of win-pattern IDs + pre-written markdown game details); selectable on the Game tab to auto-apply patterns + details when starting a game
- **Winners Log tab**: paginated table of past winners with sorting and per-page controls; delete an individual entry (per-row trash button) or **Delete All** to clear the log (both confirm first)
- **Senpan Tea House → Raffles tab** (`RafflesTab.vue`): one manager (replacing the former New/Open/Closed tabs) — **Current Raffles** (every non-closed raffle) as image cards with a corner status icon (calendar-clock when it opens later, red calendar-circle-exclamation when its open window has passed), then a searchable + paginated **Closed Raffles** table (title, winner, open period, and the gil collected from paid entries — `Raffle.winner_name`/`paid_total`, admin-only aggregates joined in `listRafflesAdmin`) with a **Copy** action that seeds a new raffle from a past one (`copyRaffleForm`). Detail (winner pick/verify, **manually add an entry — optionally paid**, toggle paid, delete; read-only when closed) and the create/edit form (`RaffleFormTab.vue`, emits `saved`/`cancel`) open as Back sub-pages. Weighted random winner pick; delete raffles.
- **Senpan Tea House → Announcements tab** (`AnnouncementsTab.vue`): manage **announcement types** (a named Discord channel webhook), **taggable roles** (a friendly name + Discord role ID — managed like types under "Manage Roles"), and **announcements** authored as Discord embeds (title, markdown details, accent colour, optional location + event window, image upload/reuse, an optional **role tag** posted in the message content above the embed — *Do Not Tag* / *@everyone* / a managed role, since mentions inside an embed don't notify — and **up to 5 Discord link buttons** — label + optional emoji + URL — rendered as an action row beneath the embed; sanitized server-side and stored as JSON in `announcements.buttons`). Post manually (**send now**) or on a schedule (once / daily / weekly / monthly, anchored to an IANA timezone so times survive DST); **skip next** occurrence; the background scheduler posts due items. Client-side search + type filter. (Link buttons require a webhook whose target supports message components.) Book-club meeting/event posts are authored here too (the retired per-club "event posts" feature was merged in; its webhooks migrated to dedicated "<Club> Book Club Events" types).
- **Senpan Tea House → Book Club tabs** (`BookClubTab.vue`, one route per registered club): manage **reading lists** + their items — add items manually or pull them from **AniList** (search/by-id proxy that prefills title/summary/cover/format/genres/chapters/source), drag-reorder, upload covers; **publish** a list to the club's Discord channel (one embed per item).
- **System → Settings tab**: app title, bingo join prompt, default draw delay, frequent-winner threshold/window, the **header/board font** picker (combo box with optgroups for uploaded fonts + Google Fonts, optional Google Fonts API key, live preview), the AniList API URL, and the per-club reading-list Discord webhook URLs (treated as secret)
- **System → Themes tab**: CRUD themes via a structured **token editor** (`ThemeTokenEditor.vue` — a swatch + value per `:root` design token, grouped, with a collapsible live preview), activate/deactivate live. A theme stores a **token map** (not CSS); the server generates the `:root{}` stylesheet (`store.TokensToCSS`). Alpha tokens (modal/shadow/glow) use the `@ckpack/vue-color` picker; solid tokens the native colour input
- **System → Users tab** (`UsersTab.vue`, **admin-only**): manage accounts — activate/deactivate pending registrations, grant/revoke admin, edit the per-page permission set (checkbox grid grouped by section, derived from `ADMIN_PERMISSIONS`), set a user's password, and delete accounts. The seeded `admin` account is protected (its destructive actions are hidden in the UI and rejected server-side). Separately, **every** logged-in account can rotate its own password via the **Change Password** modal in the admin topbar (`AdminView.vue` → `POST /api/account`)
- **Atelier → Font Upload tab** (`FontsTab.vue` + `FontEditModal.vue`): upload font files (`.ttf/.otf/.woff/.woff2/.eot`) to `<webRoot>/fonts/`; files sharing a base name are variants of ONE font (a font with no WOFF2 gets one auto-converted). The table is slim — **CSS Name** (the `font-family` used by the kit/app/picker), **Serves** (the served format; ✦ = the converted copy), **Modified**, actions — and the **Edit modal** holds the rest: CSS name, served-version picker (any variant type or Auto), the font's **per-font allowed sites**, and the files themselves (per-file rename/delete, sizes). The **live preview** (type any text, pick a font) can switch between the font's actual formats. Per-row **Copy URL** copies the served variant's tokenized URL (rotates every 1–2 weeks — the kit is for permanent embeds); an Embed panel shows the `kit.css` snippet. The app's own origin is always allowed implicitly (the font picker never breaks).
- **Atelier → Carrd Upload tab** (`CarrdUploadTab.vue`): manage image-hosting **projects** (folders under `<webRoot>/carrd`, each with a human-readable title) and nested sub-directories; upload images plus `.mp3`/`.mp4` (same-name **overwrites** so external Carrd sites pick up new versions), copy public URLs, delete files/dirs/projects. Served cross-origin from the carrd vhost.

**Key UI patterns**:
- **Admin sidebar = accordion** (`AdminSidebar.vue`): section headers (Bingo / Tea House / Festival / Atelier / System) are pure **accordion toggles** — clicking one shows/hides the items it contains and never navigates; only the items navigate. Expanded sections are local component state (`openSections`, a reactive `Set`) and toggle **independently**, so any number can be open at once. A section auto-opens when navigation makes it active (so the highlighted item stays visible); otherwise sections are collapsed/expanded freely without changing the route. A whole section is hidden when the account can access none of its pages.
- `[v-cloak]` (in app.css) prevents a flash before mount; the `#app` div carries it
- Optimistic updates: pattern/category reorder swaps locally then persists in background
- Toast notifications for success/error feedback (`ui` store)
- Admin login separates auth failure from data-loading failure to prevent false login rejections
- Winner toast only fires on new winners (compares count before/after draw)
- WebSocket reconnect with exponential back-off (1s → 2s → 4s → 8s → 16s, max 10 attempts)
- vue-draggable-plus handles drag-and-drop for patterns and categories (replaces the old manual HTML5 DnD placeholders)

## FFXIV plugin (Dalamud)

`plugins/SenpanCompanion/` is a **Final Fantasy XIV Dalamud plugin** (C#/.NET 10,
ImGui) that acts as a **second admin UI** over the same Go server — published as
*Senpan Admin Companion* (Dalamud InternalName/DLL `SenpanCompanionAdmin`; the
folder, csproj, and C# namespace stay `SenpanCompanion`). The server remains the
single source of truth: every plugin action hits the same REST/WebSocket API the
website uses, so it still broadcasts to all web clients. FFXIV gives plugins no
client↔client networking, so the server *must* stay the relay — the plugin never
talks peer-to-peer.

- **Auth — personal access token.** The plugin sends `Authorization: Bearer pat_…`
  on REST and `?token=` on the WebSocket; the server URL + token are entered via
  `/senpan config` (PAT support shipped in FE+BE v1.5.0). Permissions are enforced
  server-side exactly as for the web admin (`bingo-cards` / `bingo-game` /
  `teahouse-raffles`). `/senpan` opens the main window.
- **Feature parity (admin subset).** Bingo: create named cards (`generate_single`)
  + delete / delete-all; game lifecycle (start with pattern IDs, draw w/ delay,
  halftime, end with winner IDs); live draws + winner alerts over the **admin
  WebSocket** (`game_draw` / `game_update` / `cards_update`). Raffle: pick an open
  raffle, add / mark-paid / delete entrants, pick→confirm a winner (no raffle
  creation). A **nearby-players** picker (`IObjectTable`) prefills name + world.
- **Threading model (critical).** Dalamud has no SyncContext, so `await`
  continuations resume on the thread pool. **Every UI-state write from an async op
  must go through `Apply(() => …)`** (`Plugin.Framework.RunOnFrameworkThread`),
  which marshals back to the framework thread — otherwise it races the render loop
  + WS handlers. Tab panels extend `Windows/TabBase` (shared `Run`/`Busy`/`Status`,
  `EnsureLoaded`/`MarkStale` with retry cooldown, `LoadAsync`); the card list is the
  shared `Services/CardCache` (coalesced refresh, framework-thread swap).
- **Game interop is minimal + ToS-sensitive.** `WinnerChime` is a synth WAV via
  winmm (no game interop); `ChatSender` /tell uses
  `RaptureShellModule.ExecuteCommandInner` — the only game interop, and opt-out.
- **Rolls tool (`Services/RollTracker` + `Windows/RollsTab`).** A **permission-free,
  server-independent** helper: `RollTracker` subscribes to `IChatGui.ChatMessage`,
  keeps `XivChatType.RandomNumber` (`/random` · `/dice`) lines **in memory only** (no
  persistence, no server), and wipes them on `IClientState.Logout` + dispose. Read-only
  observation — no game interop.
- **Timed Text Macros (`Services/TimedMacroRunner` + `Windows/TimedMacrosTab`).** Another
  **permission-free, account-free** tool. `TimedTextMacro` records (persisted in
  `Configuration.TimedTextMacros`) repeat a Say/Yell/Shout message on an interval with an
  optional send cap. `TimedMacroRunner` drives them off `IFramework.Update`, sends via
  `ChatSender.SendChannelMessage` (split by `TellComposer.SplitPlain`, one part/second),
  and saves `SendsCompleted` after each fire; **run state is in-memory only** so macros
  reload **stopped**, and `IClientState.Logout` stops them all. This is `/say`-`/yell`-
  `/shout` **game automation on a timer** — operator-initiated and opt-in per macro, but
  more automation than the single-shot tells; keep the ToS caveat in mind.
- Both account-free tools are reachable **without a token**, so `MainWindow.Draw` renders
  the sidebar even before setup (the suite pages stay hidden until an account connects).
- **Build.** `<Project Sdk="Dalamud.NET.Sdk/15.0.0">` supplies the `net*-windows`
  TFM and references Dalamud from the local XIVLauncher install; needs the **.NET 10
  SDK** + XIVLauncher/Dalamud. `dotnet build -c Release` emits
  `bin/Release/SenpanCompanionAdmin/latest.zip` (DalamudPackager); `bin/` + `obj/`
  are gitignored. **0/0 warnings is enforced — keep it that way.** Note ImGui's
  namespace is `Dalamud.Bindings.ImGui` (not ImGuiNET).
- **Distribution.** A **custom Dalamud repo**, not the official list:
  `plugins/pluginmaster.json` + `latest.zip` are hosted at
  `https://apps.senpan.cafe/plugin/…` (publish with `scripts/deploy.ps1 -Target
  plugin`). Bump `AssemblyVersion` + `LastUpdate` in `pluginmaster.json` per release;
  a Dalamud API bump means bumping `DalamudApiLevel` (currently **15**).

## Developer commands

```powershell
# ── Frontend (Vite) ──
cd frontend; npm install            # first time / after dependency changes
cd frontend; npm run dev            # dev server on :5173, proxies /api → :8080
cd frontend; npm run build          # type-check (vue-tsc) + build to frontend/dist/
cd frontend; npm run typecheck      # vue-tsc only
cd frontend; npm run test           # Vitest run (CI mode, one-shot)
cd frontend; npm run test:watch     # Vitest watch mode
cd frontend; npm run test:coverage  # Vitest + v8 coverage report
cd frontend; npm run lint           # ESLint (flat config) with --fix
cd frontend; npm run lint:check     # ESLint without --fix (CI gate)
cd frontend; npm run format         # Prettier write over src/
cd frontend; npm run analyze        # build + emit dist/stats.html bundle treemap (visualizer)
cd frontend; npm run gen:types      # regenerate TS types from Go models (runs tygo in ../backend; needs Go)

# NOTE: src/types/api.generated.ts is gitignored. Run `npm run gen:types` after a
# fresh clone (or after changing Go model types) before build/typecheck/test.

# ── Go backend ──
# Build the Go backend
cd backend; go build -o app-suite.exe .

# Run the server (from project root or with -db flag)
cd backend; go run . -addr :8080 -db ../devdata/database.sqlite -webroot ../devdata/webroot

# Auth is per-user now. On first run, migration v22 seeds a bootstrap account:
#   username: admin   password: admin   (full admin) — ROTATE IT IMMEDIATELY
# via the topbar "Change Password" modal after logging in at /admin/login.

# Vet / lint
cd backend; go vet ./...
cd backend; golangci-lint run ./...   # config: backend/.golangci.yml (pinned v2.12.2 in CI)

# Run tests
cd backend; go test ./...

# Build for production
cd backend; go build -ldflags="-s -w" -o app-suite .

# ── FFXIV Dalamud plugin (C#/.NET 10) ──
# Needs the .NET 10 SDK + a local XIVLauncher/Dalamud install (Windows).
cd plugins\SenpanCompanion; dotnet build -c Release   # → bin/Release/SenpanCompanionAdmin/latest.zip (keep 0/0 warnings)
cd plugins\SenpanCompanion; dotnet format             # apply .editorconfig style (CI runs --verify-no-changes)
dotnet tool restore; dotnet roslynator analyze plugins\SenpanCompanion\SenpanCompanion.csproj --severity-level info   # C# lint gate (Roslynator CLI, pinned in .config/dotnet-tools.json; ruleset curated in .editorconfig)
.\scripts\deploy.ps1 -Target plugin                   # build + publish the custom Dalamud repo (pluginmaster.json + latest.zip)
```

## Continuous integration

`.github/workflows/ci.yml` runs on every push and pull request, with three gate
jobs plus a release job:

- **frontend** (`working-directory: frontend`): `npm ci` → `npm run gen:types`
  (needs Go, so the job also sets up the Go toolchain) → `lint:check` →
  `typecheck` → `test` → `build`. Mirrors the local gate, so a green CI ==
  the checks a developer runs locally have passed.
- **backend** (`working-directory: backend`): `golangci-lint run` (pinned
  v2.12.2, config `backend/.golangci.yml`) → `go build ./...` → `go vet ./...` →
  `go test ./...` (Go version read from `backend/go.mod`; the tests include the
  OpenAPI spec-freshness + route-coverage checks in `internal/apidoc`) →
  `govulncheck ./...` (`go run golang.org/x/vuln/cmd/govulncheck@latest`, run at
  latest so newly disclosed advisories are caught).
- **plugin** (`working-directory: plugins/SenpanCompanion`): builds the FFXIV
  Dalamud plugin on a **Linux** runner — there's no XIVLauncher there, so it
  downloads the official Dalamud dev distribution and points `DALAMUD_HOME` at it;
  the `net*-windows` TFM compiles via the `EnableWindowsTargeting` property
  (conditioned off on Windows, so local builds are unchanged). Gate =
  `dotnet build -c Release -p:TreatWarningsAsErrors=true` (the 0-warning rule) +
  `dotnet format --verify-no-changes` + `roslynator analyze --severity-level info`
  (the C# lint gate — Roslynator CLI pinned in `.config/dotnet-tools.json`; ruleset
  curated in `plugins/SenpanCompanion/.editorconfig`).
  Tracking Dalamud `latest` means an upstream API bump can redden it — the cue to
  bump `DalamudApiLevel`.
- **release** (`needs: [frontend, backend, plugin]`, only on a push to `main`):
  runs `.github/scripts/release.sh`, which for each component reads its current
  version (`package.json` / `version.go` / `SenpanCompanion.csproj`) and, if the
  `<Component>-v<version>` release doesn't exist yet, tags it and publishes a
  GitHub Release whose body is that component's `CHANGELOG.md` section. Idempotent
  (already-released versions are skipped) and gated on the whole gate passing, so
  only green commits release. Needs `contents: write` (set at the job level; the
  workflow default is `contents: read`).

When adding a check, wire it into both the relevant npm/go/dotnet script **and**
`.github/workflows/ci.yml` so local and CI stay in lockstep.

## Deployment (Apache)

The frontend is served statically by Apache; `/api/*` + `/api/ws` are reverse-
proxied to the Go server. The document root layout keeps uploads separate from
the built SPA so redeploys never wipe them (full guide in `deploy/README.md`):

```
<DocumentRoot>/
├── .htaccess        ← deploy/.htaccess  (SPA fallback: serves dist/index.html; routes /assets → dist/assets; /images served from root)
├── images/          ← deploy/images/    (PERSISTENT: logo/favicon/banner + raffles/, announcements/, bookclub/ uploads)
├── fonts/           ← (PERSISTENT: admin-uploaded font files; NOT static — streamed by the Go server via tokenized URLs; fonts.senpan.cafe reverse-proxies to /api/fonts/pub/)
├── carrd/           ← (PERSISTENT: Carrd image-host projects; served by the carrd.senpan.cafe vhost)
└── dist/            ← frontend/dist/    (built SPA; replaced each deploy)
```

- Run the Go server with `-webroot <DocumentRoot>`; uploads are written under
  `<DocumentRoot>/images/{raffles,announcements,bookclub}/`,
  `<DocumentRoot>/fonts/`, and `<DocumentRoot>/carrd/<project>/`. Image/font/carrd
  URLs are returned absolute (built from the request scheme+host).
- `vite.config.ts` strips `dist/images/` after build (the `strip-dist-images`
  plugin) so the redundant copy doesn't shadow the persistent root `images/`.
- Schema migration v10 rewrites legacy `assets/images/raffles/...` prize paths
  to `images/raffles/...` automatically on first start.
- **Font host (protected serving)**: uploaded fonts are licensed assets and are
  NOT served statically. The Go server streams them via rotating tokenized URLs
  (`/api/fonts/pub/kit.css` + `/api/fonts/pub/f/{token}`, `fontserve.go`) gated
  **per font** by its own origin allowlist (Font Upload → Edit); cross-origin
  `@font-face` requires the echoed CORS header, so non-listed sites can't
  render the fonts, and `kit.css` only emits each requesting site's allowed
  fonts. The `fonts.senpan.cafe` vhost is a **reverse proxy** (`ProxyPass / →
  http://localhost:8080/api/fonts/pub/`); external Carrd sites embed
  `https://fonts.senpan.cafe/kit.css`. The SPA loads fonts same-origin through
  the `/api` ProxyPass, so it never depends on any allowlist. Deploy
  `deploy/fonts.htaccess` (deny-all, defense in depth) to
  `<webRoot>/fonts/.htaccess`. See **Font host (protected serving)** in
  `deploy/README.md`.
- **Carrd host (CORS)**: Carrd projects are served cross-origin from
  `https://carrd.senpan.cafe` and embedded by external Carrd sites, so reads need
  CORS. Deploy `deploy/carrd.htaccess` to `<webRoot>/carrd/.htaccess` (adds
  `Access-Control-Allow-Origin`, hides the `.carrd.json` sidecars, disables
  listings). See the **Carrd image host (CORS)** section of `deploy/README.md`.
- **Scheduler**: the server runs a background goroutine that posts due
  announcements to Discord webhooks — no cron needed; it
  starts with the process and stops on graceful shutdown.
- **Server logs**: structured JSON to stdout (journald) + a rotating file at
  `/var/log/senpan/senpan.log` (`-log-file`). Under `ProtectSystem=strict` the
  `senpan.service` unit needs `LogsDirectory=senpan` so that path is writable;
  otherwise the app degrades to stdout-only (a warning is logged). Admins view
  them at **System → Logs** (live tail + `GET /api/logs`) or on-box via `jlv`.
  See **Server logs** in `deploy/README.md`.
  - **Request line** carries `method`/`path`/`status`/`duration`/`ip` plus an
    **actor**: `auth` (`session`|`token`|`bot`|`anon`), and `user`/`bot` when they
    apply. Admins resolve via the cookie session, the plugin via its PAT (both
    through `currentUser`, resolved in `withActor` inside the handler chain and
    carried out to the log via a per-request holder, since the log runs outside
    the session middleware — see `actor.go`). A verified crawler is named from
    Cloudflare's signal: native `cf-verified-bot`(+category) on Bot Management, or
    a custom `x-verified-bot` header set by a Transform Rule on `cf.client.bot`
    (works on any plan). It's a logging hint only, never a security decision.
  - **Capability tokens are redacted** (garapon/stamp-card/font paths, the PAT
    `token` query, and Referer) to a short SHA-256 correlation hash (`logredact.go`).
    `logClientIP` trusts `CF-Connecting-IP`/XFF only from a loopback peer (the
    Apache proxy), so a direct client can't forge the `ip`.
  - **Live tail** fans each line out to admin WebSocket clients gated on the
    per-connection `isAdmin` bit (not merely the admin channel), matching the
    `requireAdmin` gate on `GET /api/logs`.

## Conventions & patterns

- **Typed structs everywhere**: all store methods return typed `model.*` structs — no `map[string]interface{}`.
- **Dependency injection**: `Server` struct holds all deps (`Store`, `GameService`, `Hub`, sessions, password). No package-level globals.
- **Method-pattern routing**: Go 1.26+ `"GET /api/auth"` patterns — no manual method checks.
- **Typed JSON requests**: each handler defines a request struct and uses `readJSON[T]()` generic decoder.
- **JSON errors**: API failures return `{"error":"message"}` with appropriate HTTP status via `writeError()`.
- **Hybrid REST**: the HTTP method carries intent — `GET` read, `POST` create (`201`) or run a command, `PUT` replace, `PATCH` partial-update, `DELETE` remove (`204`). Items are `/api/<resource>/{id}` (numeric or string keys), sub-collections nest (`…/{id}/<subs>/{subId}`), non-CRUD commands are `POST …/{id}/<verb>` (e.g. `…/close`, `…/activate`, `/api/game/start`), bulk writes are `POST` (`…/reorder`, `…/generate`) and bulk deletes are `DELETE …/all`. A single reorder / flag toggle is a declarative `PATCH`. The only action-body holdouts are `POST /api/auth` (login/logout) and `POST /api/register`.
- **Account-based auth + per-page permissions**: login stores `user_id` in the SCS session cookie; `currentUser` reloads the account from the DB on each request (no stale snapshot). Every mutating admin handler opens with a guard — `requireAuth` / `requireAdmin` / `requirePermission(perm)` / `requireAnyBookClub` (see **Authentication & authorization**). Permission keys equal the frontend `AdminTab` ids. The legacy `-password` / `APPSUITE_ADMIN_PASSWORD` flag is **deprecated and unused** (kept only for backward-compatible startup); don't add new code paths that read `Server.password`.
- **Password hashing**: argon2id via the `internal/auth` package (`Hash`/`Verify`, PHC strings, constant-time compare). The hash lives only in the store layer and is never placed on `model.User`. Min password length 8 (the seeded `admin`/`admin` bypasses this and must be rotated).
- **Pattern snapshots**: `game_patterns` stores a copy of pattern name + data so deleting a pattern doesn't break active games.
- **Winner caching**: `games.winners_cache` stores JSON array of winner card IDs; updated only on draw, read on board requests.
- **In-memory game state cache**: `GameService` caches the built `GameState` and invalidates on start/draw/end — eliminates 3 DB queries per board request.
- **In-memory card cache**: `GameService` caches all cards for winner computation; invalidated on generate/delete.
- **In-memory game_details cache**: `GameService` caches the `game_details` setting via read-through `GameDetails()` / write-through `SetGameDetails()`.
- **Incremental winner computation**: `computeWinners` skips cards already in the winners list.
- **Serialized game lifecycle**: `Service.Start/Draw/End` hold `opMu` so concurrent draws can't race on the called-numbers set or draw a duplicate number (no UNIQUE constraint on `called_numbers`).
- **Player state is client-side**: stamp marks stored in `localStorage` keyed by `stamps_{cardId}_{gameId}`.
- **Real-time updates**: WebSocket hub broadcasts game/card/pattern/style changes; separate player/admin channels.
- **Draw delay**: admin can set 0–60s delay before players receive drawn number via WebSocket.
- **Schema versioning**: `PRAGMA user_version` in SQLite tracks migration state; `schemaVersion` constant in `store/migrate.go` controls the target (currently 43). Migrations are idempotent (`hasColumn` guards) and run incrementally only when behind.
- **Optimistic UI**: pattern/category reordering swaps locally before API call; reverts on failure.
- **Lightweight endpoints**: `GET /api/cards` returns only IDs + player names (no board data); `GET /api/board?preview=1` returns only the card (no game state).
- **Batch operations**: `GetCardPlayerNames()` fetches multiple cards in one query; `SaveCardsBatch()` uses transactions.
- **`math/rand/v2`**: card generation uses `math/rand/v2` (auto-seeded, not crypto — appropriate for bingo).
- **`log/slog`**: structured logging for server startup and errors.
- **SQLite pragmas**: WAL mode, synchronous=NORMAL, busy_timeout=5000, cache_size=8MB, mmap_size=32MB, foreign_keys=ON, temp_store=MEMORY, max_open_conns=4. They are applied **per connection** via the ncruces `driver.Open` connect hook (`connectPragmas` in `store.go`) — not once after open — because most of these are per-connection in SQLite. This matters for correctness, not just tuning: `foreign_keys` (default OFF) is what makes the `ON DELETE CASCADE` rules fire, so a delete on a connection without it would silently orphan child rows.
- **WebSocket bypass**: `/api/ws` is routed directly to the mux, bypassing the SCS session middleware and `responseWriter` wrapper — `coder/websocket` requires the raw `http.ResponseWriter` for the upgrade handshake. Because it skips the middleware, `handleWS` reads the session manually (`wsSessionUser`) to gate the admin channel (no-`id` connections require an authenticated, active account); player connections (with a card `id`) stay public.
- **Pre-built session handler**: `sessions.LoadAndSave(mux)` is constructed once in `New()` and stored as `sessHandler` — not rebuilt per-request.
- **Client-side keepalive**: JS sends a `ping` text message every 25s to prevent reverse-proxy idle timeouts from dropping the WebSocket.
- **Rate limiter**: IP-based brute-force protection for admin login; reads `X-Forwarded-For` for the real client IP behind a reverse proxy.
- **FontAwesome**: bundled via `@fortawesome/fontawesome-svg-core`; only the used Pro-kit icons are added to the library in `frontend/src/lib/fontawesome.ts`. Templates render them with the global **`<font-awesome-icon :icon="[prefix, name]" />`** component (`@fortawesome/vue-fontawesome`, registered in `main.ts`) — `['fad', …]` duotone, `['fas', …]` solid, `['fab', 'discord']` brands. Vue owns the `<svg>` directly (no `dom.watch()`/MutationObserver, no nest-mode hack). The shared `ui/` primitives (AdminPanel/ManagerView/SubPageHeader/EmptyState) take an `icon?: [IconPrefix, string]` prop and forward it (name is `string` because Pro icon names aren't in FA's free `IconName`). CSS targets the rendered `.svg-inline--fa` (not `<i>`). Component tests stub `<font-awesome-icon>` globally via `vitest.setup.ts`.
- **Theme editor**: structured token editor (`ThemeTokenEditor.vue`) bound to the edited theme's `tokens` map; `lib/theme-tokens.ts` is the token source of truth. Themes are token-only (no free-form CSS); the applied `:root{}` is generated server-side (`store.TokensToCSS`) and locally for the live preview (`tokensToCss`). CodeMirror was removed.
- **TS↔Go type sync**: `frontend/src/types/api.generated.ts` is generated from `internal/model` by tygo (`backend/tygo.yaml`); it is **gitignored** — regenerate with `npm run gen:types` (needs Go) after a fresh clone or model change. Never edit the generated file.
- **Typed endpoint layer**: stores never call `api<T>('path')` directly — they call `endpoints.*` (`frontend/src/lib/endpoints.ts`), which wraps every backend path in a typed function. Add new endpoints there so paths/bodies/response types live in one place.
- **Global 401 handling**: `api.ts` invokes a registered handler (set in `main.ts`) on any non-auth 401 → redirect to `/admin/login` + "session expired" toast. Auth endpoints pass `skipAuthRedirect` so a bad-password login doesn't trigger it.
- **Themed confirm dialog**: use `await ui.confirm(message, opts)` (renders `ConfirmModal.vue`) instead of the native `window.confirm`.
- **Theme fidelity**: never hard-code colors/fonts — drive everything off the `:root` design tokens (`styles/tokens.css`). Themes override only those tokens (not class names), so reuse tokens on existing markup; classes can be refactored freely.
- **Uploaded fonts**: `applyUploadedFonts()` (`lib/theme.ts`) writes one `<style id="uploaded-fonts">` with an `@font-face` per file from `FONT_BASE_URL` (`stores/fonts.ts`), so a font is registered app-wide (board/header + every preview), not per-component. It then measures each loaded font via the Canvas TextMetrics API and, for fonts with oversized vertical metrics, rewrites the rule with `ascent-override`/`descent-override`/`line-gap-override` to clamp the box (`clampFontMetrics()` — pure + unit-tested). The Settings preview and the Font Upload live preview both rely on these shared rules, so previews always match what players see.
- **Discord embeds (shared plumbing)**: every webhook-posting feature (announcements, reading-list items) builds its embed with the fluent `newEmbed()…build()` builder in `server/embeds.go` and posts via `postDiscordEmbed()` (or `postDiscordWebhook()` for a payload with an optional `content` mention + `allowed_mentions`). The builder auto-truncates to Discord's per-field limits, skips empty fields, and caps at 25 fields, so callers pass raw content. New embed shapes add a builder chain, not new transport. Times are emitted as Discord `<t:unix:…>` tokens so each viewer sees their own zone.
- **Timezone-anchored scheduling**: announcements store the admin's wall-clock input (`*_local`) **and** its IANA `timezone`, plus the computed absolute UTC instant. Recurrence math (`nextAnnouncementOccurrence`, `nthWeekdayOfMonth`) runs in that zone so schedules survive DST. `server/tzdata.go` blank-imports `time/tzdata` so `time.LoadLocation` works on hosts without system zoneinfo (e.g. Windows).
- **Background scheduler**: `RunAnnouncementScheduler` is launched as a goroutine in `main.go` against a shutdown-cancelled context, ticks every 30s, sweeps once on startup, and is crash-tolerant (unset webhook → left pending; failed post → retried; `skip_next`/recurrence advance the cursor). The due-row query is index-backed (`(active, next_post_at)`). It uses the ticker/sweep loop in `server/scheduler.go` (`runScheduler(ctx, interval, sweep)`) — the `Run…Scheduler` is just a one-line wrapper supplying its interval + sweep func.
- **Shared upload helpers** (`server/uploads.go`): the single-image upload flow (5 MB cap, `image` field, `safeImageUploadName` ext check, content-sniff, write, return `{"url"}`) is `s.saveSingleImageUpload(w, r, relDir)`, used by the book-club cover endpoint. `relDir` (e.g. `bookclubCoverRelDir`) is a forward-slash path that doubles as the on-disk location and URL path. Like the central-image and Carrd uploads, it **keeps the uploaded filename** (a same-named upload overwrites) — the app does not rewrite upload names — so an endpoint that auto-cleans its files must guard against a name shared by another record (see `removeBookclubCoverIfUnused`). `saveMultipartFile` streams one multipart part to a path (filename-agnostic; reused by fonts + Carrd). Most upload areas now live under the central image host (System → Images); add a bespoke area only when it isn't a managed category.
- **AniList proxy**: `GET /api/bookclub/lookup?q=|id=` proxies the public AniList GraphQL API server-side (avoids browser CORS + centralizes field mapping); the endpoint is configurable via the `anilist_api_url` setting. `anilistToItem` maps a Media node to a reading-list item suggestion.
- **Per-club registry**: book clubs live in one registry — `bookClubs` (`server/bookclubs.go`) on the backend and `BOOK_CLUBS` (`lib/constants.ts`) on the frontend. Adding an entry wires up its route, sidebar button, comments label, and its secret reading-list webhook setting automatically. The per-club webhook setting key (`discord_webhook_url_<slug>`) is registered in `bookclubs.go`'s `init()` and must match `clubWebhookKey` on the frontend.
- **Secret settings**: `GET /api/settings` is public, so keys in `secretSettings` (the per-club Discord webhooks) are blanked out for non-admin callers and only returned to an authenticated admin who needs them to edit.
- **datetime conversion**: `lib/datetime.ts` converts between stored UTC (RFC-3339) and the LOCAL wall-clock value an `<input type="datetime-local">` expects (raffle windows); announcement forms instead send the raw wall-clock + an explicit IANA zone the backend resolves.
- **Snappy admin navigation (load-gate)**: re-entering an admin tab would otherwise re-run its data load every time. `admin.setTabFromRoute()` routes each tab's load through `loadFresh(key, …)` (backed by `lib/freshness.ts`), which skips the refetch when that dataset loaded within a 30s TTL — so tab switches are instant instead of re-spinning. The gate lives at the **navigation layer only**: store loaders still always fetch, so post-mutation refreshes (which call the loaders directly) show edits immediately, and live game/cards/patterns stay current over WebSocket. `bookclub.openClub()` uses its own per-club freshness so re-entering the same club keeps the open list while switching clubs refetches.
- **Admin UI primitives** (`components/common/ui/`): admin forms and tables are built from a small set of presentational components so every screen is consistent by construction — don't hand-roll `.field`/label/table markup. Use `AdminPanel` (the `.admin-panel` card; pass `title`+`icon` or supply a custom header slot), `FormField` (label + control slot + optional `help`/`#help`; controls auto-stretch full-width, so no `.field-input-full` on direct children), `FormRow` (equal-width side-by-side fields — for unequal widths use a plain `.flex-row` and pass `style="flex:…"` to each `FormField`, which falls through to its root), `FormActions` (button row; `align="start|end|between"`), `DataTable` (generic sortable table — `columns`/`rows`/`row-key`, `#cell-<key>` slots, `@sort`, `#empty` slot, optional `rowClass`; the trailing **actions column uses `label: ''`** (no header) **+ `align: 'right'`**, and its `#cell-actions` content is wrapped in **`.row-actions`** (flex, `gap`, right-justified) — match this for every table with row actions, never a bare inline span), `PaginationBar`, and `EmptyState`. The CSS for these lives in `styles/utilities.css` (the "FORM & TABLE PRIMITIVES" and "MANAGER LAYOUT" blocks) and uses theme tokens so themes restyle them.
- **Manager model** (`components/common/ui/`): every admin tab that manages a collection follows one shape. `ManagerView` = the list page shell (`.admin-panel` + `title`/`icon` header, `#actions` slot top-right for buttons like "Manage Categories" / "+ New", optional `#toolbar` slot for `SearchInput` + filter selects, default slot = the list, `#pagination` slot). `ListRow` = one item (`#media` left, body default, `#actions` pinned **far right**); stack rows in a `.list-rows` container. `SubPageHeader` = a sub-page header (title + Back, emits `back`). Convention: the tab holds a `screen` ref (`'list' | 'new' | …`); `#actions` buttons switch `screen`; each sub-screen opens with `SubPageHeader @back`. Reference implementations: `PatternsTab` (list/new/categories), `AnnouncementsTab`, `PresetsTab`, `Open/ClosedRafflesTab`, `BookClubTab`. Manage Cards / Fonts / Carrd keep their bespoke shapes (chip grid / sortable table / image manager) rather than the full ManagerView model, but still compose the shared primitives: Fonts uses `DataTable` (sortable, `rowClass` for the preview-selected row) + `SearchInput`; Cards + Carrd use `SearchInput` and the `.chip` object. What stays genuinely page-unique: the Fonts live-preview panel, and Carrd's breadcrumb / drag-drop dropzone / square asset grid.
- **Theme tokens** (`:root` in `styles/tokens.css`, mirrored in `lib/theme-tokens.ts` + Go `themeTokenOrder`): role-descriptive CSS custom properties, all overridable by saved themes + the theme editor. Backgrounds: `--page-bg`, `--panel-bg` (cards/panels), `--panel-raised-bg` (nested/row/chip surface), `--control-border` (control & divider outline — readable on *both* panel surfaces), `--input-bg` (form-control fill). Accents: `--accent` / `--accent-hover`, `--accent-2` / `--accent-2-hover`, `--highlight` (called numbers, headings, gold trim). Text: `--text`, `--text-muted`, `--text-on-accent`, `--text-on-fill`. Status: `--success`, `--danger`, `--warning` (skip badges/alerts; use `color-mix(in srgb, var(--warning) N%, transparent)` for tints). Board: `--board-cell-bg`, `--board-cell-hover-bg`, `--board-free-bg`, `--board-gradient-start/end`. Effects: `--modal-overlay`, `--shadow`, `--highlight-glow`. Non-colour: `--radius`, `--header-font`. Use `--control-border` for outlines and `--input-bg` for control fills (the old overloaded `--surface2`-as-border was invisible on nested boxes), so no one-off contrast overrides are needed inside a `--panel-raised-bg` box.
- **Class rename map** (structural classes consolidated 2026-06): `.entries-table`/`.winners-log-table` → `.data-table`, `.msg-block` → `.empty-state`, `.btns` action rows → `.form-actions`; `.field-input-full` is retained only for controls nested inside a flex row (not a direct `.field` child).
- **Token rename (2026-06)**: the theme tokens were renamed to role-descriptive names (`--surface`→`--panel-bg`, `--surface2`→`--panel-raised-bg`, `--border`→`--control-border`, `--field-bg`→`--input-bg`, `--primary`→`--accent`, `--secondary`→`--accent-2`, `--gold`→`--highlight`, `--text-dim`→`--text-muted`, `--text-on-primary`→`--text-on-accent`, `--text-on-toast`→`--text-on-fill`, `--bg`→`--page-bg`, `--board-cell`→`--board-cell-bg`, `--overlay`→`--modal-overlay`, `--shadow-color`→`--shadow`, `--glow-color`→`--highlight-glow`; `--warning` added). **Saved themes must use the new names** — the live DB themes were migrated in `CURRENT_THEMES.css` (root of repo) for re-import; a theme still using old token names silently falls back to the app.css defaults.
- **CSS direction — objects, not pages**: app.css is migrating away from page/feature sections (`HOME VIEW`, `RAFFLES`, `ADMIN GAME`, …) toward a small vocabulary of reusable **objects** composed across pages — the `components/common/ui/` primitives are the first wave. When styling new UI prefer, in order: (1) an existing primitive/object class, (2) a theme token on existing markup, (3) a new shared object class in the Primitives band — and only as a last resort a page-scoped rule. **Don't** re-declare control chrome inline: form controls already inherit the global `input,select`/`button` rules (`2px solid var(--control-border)`, `--input-bg`), so an inline `style="border:1px solid var(--panel-raised-bg)"` on a `<select>` is both redundant and off-token. **Don't** add a per-feature one-off that duplicates an existing object. **Never fill an object or button intent with a *surface* token** (`--panel-bg`/`--panel-raised-bg`): a fill that matches the box it sits on vanishes (the recurring "ghost button" bug — a low-emphasis control blending into its container). Colored intents use semantic colours; the neutral button uses **`--control-border`** (a *non-surface* outline token tuned to read on *both* panel surfaces), so nothing can blend into the surface beneath it. **Prefer the small helper utilities over inline `style=`** for common single properties: `.nowrap`, `.w-full`, `.m-0`, `.fw-normal`, `.text-xs`/`.text-sm`, `.ta-center`/`.ta-left`/`.ta-right`, `.mb-*`/`.mt-*`, `.push-right`, `.flex-*` (in the utility band). Inline `style=` is fine for genuinely per-instance values — dynamic `:style` bindings (colour swatches, live font previews) and one-off layout (`flex: N; min-width: Xpx` form columns) — but not for repeated static literals. Object classes are still themeable (the runtime theme editor targets semantic classes), which is why a utility-first framework (Tailwind) is *not* the direction. Extracted objects so far: the `ui/` form/table/manager primitives; **`.card`** (shared "bordered surface tile" chrome — `home-card`/`raffle-card`/`saved-pattern` only set their own size/radius/hover; new tiles can use `.card` directly); **`.chip`** (raised, rounded, clickable pill — transparent border → `--highlight` on hover, `.active` inset ring; `.chip--stack` = column layout, `.chip-del` = muted→danger delete affordance; shared by the card-id chips and the Carrd project/sub-folder chips, each keeping only its own extras); **`.toggle-btn`** (segmented/selectable pill — transparent border → `--accent` on hover, `.active` = `--accent` fill; shared by the announcement weekday picker + book-club view switcher, stacked in a flex container); **`.del-x`** (prominent "×" delete glyph, `--danger` → `--text` on hover — the loud counterpart to `.chip-del`; used by card-id + saved-pattern chips, consumer adds positioning); the **button system** — ONE solid shape; intent = fill colour only, never size/border (the old ghost/danger outlines made bordered buttons read as heavier). Var-driven: each intent sets `--btn-fill`/`--btn-text`, a shared rule applies them, and the hover **darkens the fill via `color-mix`** — so the five semantic colours + the raised/border surface cover all six intents with **no per-state or per-intent tokens added** (the deliberate answer to "we don't have enough colour variables": derive, don't multiply tokens). Six intents by *consequence, not verb*: **`.btn-confirm`** (save/add/edit/create → `--success`), **`.btn-action`** (send/generate/publish/set-active/start → `--accent`), **`.btn-view`** (open/view/copy/preview/browse → `--accent-2`), **`.btn-caution`** (skip/reset/end-game → `--warning`), **`.btn-danger`** (delete/remove → `--danger`), **`.btn-neutral`** (cancel/close/back/toggles/field-clears → `--control-border` — a non-surface token, so neutral stays visible even when it sits on a `--panel-raised-bg` box); `.btn-sm`/`.btn-lg` size modifiers; `.btn-danger` keeps its name. The old `.btn-primary`/`.btn-secondary`/`.btn-ghost` are **removed** — don't reintroduce them (intent = fill colour, never an outline/ghost); **`.badge`** (uppercase pill chrome — `raffle-badge`/`live-badge`/`conn-badge`/`ann-badge`/`style-active-badge` only set colours/margins; `nav-count` is intentionally *not* a badge, it's inline count text) **+ shared state modifiers `.badge--accent`/`--muted`/`--warning`/`--success`** (theme-aware `color-mix` tints; the Announcement + Book-Club status pills compose `class="badge badge--*"` — no per-page badge colours); **`PatternPicker.vue`** (`components/common/ui/`, `v-model` = selected ids) which both GameTab and the Preset editor use, so the win-pattern picker can never desync; **`.section-heading`** (gold sub-heading; `.raffle-section-heading` is its legacy alias); the **toolbar** object (`.flex-toolbar`, with `.manager-toolbar` = same + bottom spacing); the **`.color-picker`** skin (wrap the `@ckpack/vue-color` Chrome picker in `.color-picker` — global descendant rules re-map it to theme tokens, beating the lib's own styles; used by StampColorPicker and the Themes token editor's alpha tokens); the **media** objects `.img-picker` / `.img-thumb` (+`img`/`:hover`/`.active`) / `.media-empty` (placeholder) / **`.media-cover`** (cover/preview image chrome; size set by a modifier — `--wide` 16:9 for announcement + event covers, `--book`/`--book-sm`/`--book-lg` portrait for reading-list item/lookup-result/form-preview covers; pair with `.media-empty` for the no-image box); **`ImageField.vue`** (`ui/`, `v-model` = image URL + `images`/`uploading` props, emits `upload`) — the "upload or reuse an image" field shared by the announcement + book-club event forms; **`ColorPicker.vue`** (`ui/`, `:value` + `@change` → `{rgba,hex}`) wrapping the lazy `@ckpack/vue-color` Chrome picker with the `.color-picker` skin (used by StampColorPicker and, for alpha tokens, the Themes token editor); and **`EmojiPickerModal.vue`** (`components/common/`, render with `v-if`, emits `select`(char)/`close`, optional `allow-clear`) wrapping the lazy `vue3-emoji-picker` in a themed modal (used by the player stamp shape picker + the announcement Discord-button emoji field). Note: a global object can lose to a component's *scoped* class on the same element (scoped `[data-v]` adds specificity) — e.g. a size class setting `display` will override `.media-empty`'s flex, so size classes that share an element with an object must not redeclare the object's properties. Carrd's breadcrumb / dropzone / asset grid + Fonts' live-preview panel stay intentionally bespoke (page-unique, correctly local). `DataTable` takes an optional `rowClass` (string or `(row)=>class`) to mark rows (e.g. the Fonts preview-selected row). Still page-bound: home/raffle/board section CSS that doesn't reuse a primitive.
- **Carrd path safety**: every Carrd folder/sub-path/filename received from the client is validated (`validCarrdFolder`, `cleanCarrdRelPath`, `safeCarrdFileName`) and the resolved absolute path is checked to stay within the project dir — guarding against traversal. Uploads reuse `saveMultipartFile` (it is filename-agnostic) and same-name uploads intentionally overwrite.

## Key files to inspect first

1. `backend/internal/server/server.go` — Server struct, route registration, middleware, JSON helpers, the auth/permission guards (`currentUser`/`requireAuth`/`requireAdmin`/`requirePermission`), broadcast helpers
   - `backend/internal/server/{auth,users,permissions}.go` + `backend/internal/auth/password.go` — read before touching login, registration, accounts, or page gating
2. `backend/internal/store/` — all database access, one file per domain (`cards.go`, `patterns.go`, `raffles.go`, …); `store.go` holds the `Store` struct, `New()`/`Close()`, pragmas, and shared helpers
3. `backend/internal/bingo/game.go` — Core game logic: start, draw + winner compute + cache, state reads, in-memory caching
4. `backend/internal/bingo/card.go` — Card/board generation algorithm with column-range constraints
5. `backend/internal/model/model.go` — All domain types (Card, Pattern, GamePreset, BingoGameState, BingoDrawnNumber, Raffle, ReadingList(+Item/Source), AnnouncementType, AnnouncementRole, Announcement, WinnersLogEntry, etc.)
   - `backend/internal/server/embeds.go` — shared Discord embed builder + transport (read before touching any webhook feature)
6. `frontend/src/router/index.ts` — route map, lazy route components, admin auth guard + tab sync
7. `frontend/src/App.vue` + `frontend/src/composables/useWebSocket.ts` — root shell (`<RouterView>`), WebSocket message dispatch
8. `frontend/src/lib/endpoints.ts` — the typed surface over every backend path (what stores call)
9. `frontend/src/stores/*.ts` — all client state + actions (mirror of the old app.js data/methods)
10. `frontend/src/assets/app.css` — stylesheet **index** that `@imports` `assets/styles/*.css` (tokens, base, utilities, components, player, admin, responsive — split by domain); imported in `main.ts` (content-hashed by Vite)

## Extending the project

- **New API endpoint**: add a handler method on `*Server` in `internal/server/` returning a **typed response struct** from `internal/model` (add it to `model/responses*.go` — not `map[string]any`), register the route in `routes()`, add a typed wrapper in `frontend/src/lib/endpoints.ts`, then add the endpoint to the OpenAPI paths table in `internal/apidoc/paths*.go` (add its response schema name to `internal/apidoc/schemas.go` too) and run `go run ./cmd/openapi-gen`. The `internal/apidoc` tests fail if a route is undocumented or `openapi.yaml` is stale.
- **New domain type / response struct**: add to `internal/model/` (domain in `model.go`, response envelopes in `model/responses*.go`), then run `npm run gen:types` (frontend types) and, if it's referenced by the spec, add it to `internal/apidoc/schemas.go` + `go run ./cmd/openapi-gen`.
- **New store method**: add to the matching domain file in `internal/store/` (`raffles.go`, `patterns.go`, …; new domain → new file), returning typed structs.
- **New migration**: bump `schemaVersion` in `store/migrate.go`, add an `if version < N` block in `ensureSchema()`.
- **New admin tab**: add the tab id to the `AdminTab` union in `frontend/src/stores/admin.ts` (mapping its prefix to a section in `setTabFromRoute()` + any per-tab data load — wrap the load in `loadFresh('<key>', …)` so revisits stay snappy), add a child route with `meta.tab` in `router/index.ts`, add a sidebar link (gated with `v-if="can('<key>')"`) in `components/admin/AdminSidebar.vue`, and create a `components/admin/<Tab>.vue` referenced by the route's lazy `import()`. **Wire its permission too** (see "New admin page permission" below) unless it's admin-only. For a "manage items" tab use the **manager model** (`ManagerView` + `ListRow` + `SubPageHeader` + `SearchInput`, with a `screen` ref for sub-pages — see `PatternsTab.vue`); otherwise wrap content in `<AdminPanel>`. Either way build forms/tables from the `components/common/ui/` primitives (`FormField`, `FormRow`, `FormActions`, `DataTable`, `PaginationBar`, `EmptyState`) rather than hand-rolled markup.
- **New admin page permission**: the page-permission key **must equal** the tab id. Add the constant + include it in `validPermissions()` in `server/permissions.go`; guard the page's handlers with `s.requirePermission(w, r, perm…)`; add an entry to `ADMIN_PERMISSIONS` in `frontend/src/lib/constants.ts` (label + section) so the Users-page editor, sidebar gating, and router guard all pick it up. (An **admin-only** page instead guards with `requireAdmin` and is left out of `ADMIN_PERMISSIONS` — see `system-users`.)
- **New route/view**: add a record to `routes` in `router/index.ts` with a lazy `component: () => import(...)`; mark `meta.requiresAdmin` if it needs auth.
- **New WebSocket message type**: add a case in `composables/useWebSocket.ts` (and the `WsMessage` union in `types/api.ts`), send from server via `hub.Broadcast()` / `hub.BroadcastToPlayers()` / `hub.BroadcastToAdmins()`.
- **New Discord-embed feature**: build the embed with `newEmbed()…build()` in `server/embeds.go` (extend the builder if a new shape is needed) and post with `postDiscordEmbed(webhookURL, embed)`; never write transport code per feature.
- **New book club**: add an entry to `bookClubs` (`server/bookclubs.go`) **and** `BOOK_CLUBS` (`lib/constants.ts`) — keep the slug identical — **and** add the slug to `bookClubSlugs` in `server/permissions.go` so its `bookclub-<slug>` page permission is grantable + enforced. The route, sidebar button, comments label, both secret webhook settings, and the permission entry are then derived automatically.
- **New setting**: add the key to `settingsKeys` (and a fallback in `settingsDefaults`) in `server/settings.go`; mark it secret by adding to `secretSettings` if it shouldn't be public; surface it in `SettingsTab.vue` + the `AppSettings` type.
- **New test**: colocate a `*.test.ts` next to the code; import `{ describe, it, expect }` from `vitest`; run `npm run test`. Add it to the same gate CI runs. (Go: colocate `*_test.go` and run `go test ./...`.)
- **New plugin feature/tab** (`plugins/SenpanCompanion/`): the plugin is API-only — add the call to `Api/ApiClient.cs` (+ a shape in `Api/ApiModels.cs`) against an **existing** server endpoint; never add client↔client logic. A new ImGui tab subclasses `Windows/TabBase` and is registered in `Windows/MainWindow.cs`; **route every post-await UI write through `Apply(() => …)`**. Keep the build at **0/0 warnings** and run `dotnet format` before pushing (CI gates both). If the backend gained a new endpoint, expose it on the server first (see **New API endpoint**).
