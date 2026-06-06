# AGENTS.md

Guidance for AI coding agents working in this codebase.

## Quick orientation

Go + SQLite backend serving a Vue 3 + TypeScript single-page frontend built with
Vite. The backend binary is built from `src/` and serves only the API/WebSocket
on `:8080`; the built frontend (`frontend/dist/`) is served statically (nginx),
with `/api/*` proxied to the Go server.

```
├── frontend/                         ← Vue 3 + TypeScript SPA (Vite)
│   ├── index.html                    ← Vite entry (mounts #app)
│   ├── package.json                  ← deps: vue, pinia, vuedraggable, markdown-it, vue-codemirror, @fortawesome/*
│   ├── vite.config.ts                ← build → dist/, dev proxy for /api, manualChunks, strip dist/images
│   ├── tsconfig*.json                ← TS project refs (app + node)
│   ├── public/                       ← copied verbatim into dist/ at build
│   │   └── images/                   ← logo, favicon, share banner (dev only; stripped from dist — served from doc root in prod)
│   └── src/
│       ├── main.ts                   ← createApp + Pinia + FontAwesome init (imports assets/app.css)
│       ├── App.vue                   ← root shell: view router, WebSocket lifecycle, toast, mounted
│       ├── assets/app.css            ← All styles (dark theme, board, admin, modals); imported in main.ts so Vite content-hashes it
│       ├── lib/                      ← framework-agnostic helpers
│       │   ├── api.ts                ← typed fetch client (credentials, JSON, error extraction)
│       │   ├── ws.ts                 ← WsClient: reconnect back-off + keepalive ping
│       │   ├── markdown.ts           ← markdown-it renderer (replaces CDN marked.js)
│       │   ├── fontawesome.ts        ← SVG-core library + dom.watch() (replaces FA kit script)
│       │   ├── codemirror.ts         ← CM6 CSS-editor extensions + dark theme/highlight
│       │   ├── theme.ts              ← header-font + custom-CSS <head> injection
│       │   └── constants.ts          ← stamp shapes/colors, column helpers, fallback fonts
│       ├── types/
│       │   ├── api.generated.ts      ← tygo-generated from Go model (DO NOT EDIT)
│       │   └── api.ts                ← re-exports + hand-written request/response/WS envelopes
│       ├── stores/                   ← Pinia stores (ui, app, auth, player, game, cards, patterns, styles, raffles, admin)
│       ├── composables/useWebSocket.ts ← wires WsClient → stores (message dispatch)
│       ├── components/
│       │   ├── common/               ← BingoBoard, CalledNumbers, PatternMini, ModalOverlay, ToastNotification
│       │   ├── player/               ← Stamp{Shape,Color,Opacity} pickers, WinPatternsPanel
│       │   └── admin/                ← Sidebar + one component per tab + 4 modals
│       └── views/                    ← HomeView, PlayerView, RafflesView, RaffleDetailView, AdminLoginView, AdminView
├── index.html                        ← LEGACY single-file SPA (pre-Vite reference; not built)
├── assets/css/app.css, js/app.js     ← LEGACY source of truth for look/behaviour (kept for reference)
├── deploy/                           ← Apache deploy artifacts (.htaccess + persistent images/ + README)
├── src/                              ← Go backend
│   ├── main.go                       ← Entry point: flags, DB init, server start
│   ├── tygo.yaml                     ← Go→TS type generation config (run `npm run gen:types`)
│   ├── go.mod / go.sum               ← Module deps (alexedwards/scs, coder/websocket, ncruces sqlite)
│   └── internal/
│       ├── model/model.go            ← Domain types (Card, Pattern, BingoGame, BingoGameState, Raffle, etc.)
│       ├── store/
│       │   ├── store.go              ← Store struct wrapping *sql.DB; all typed CRUD methods
│       │   └── migrate.go            ← Schema versioning + migrations (PRAGMA user_version)
│       ├── bingo/
│       │   ├── card.go               ← Card/board generation, ID generation, LetterForNumber
│       │   └── game.go               ← bingo.Service (start, draw, end, state, winner matching, caching)
│       ├── ws/hub.go                 ← WebSocket hub, client pumps, broadcast (player/admin channels)
│       └── server/
│           ├── server.go             ← Server struct (deps, routes, CORS, JSON/auth helpers, broadcast helpers)
│           ├── auth.go               ← GET/POST /api/auth
│           ├── board.go              ← GET /api/board
│           ├── cards.go              ← GET/POST /api/cards
│           ├── game.go               ← GET/POST /api/game
│           ├── patterns.go           ← GET/POST /api/patterns, GET/POST /api/pattern-categories
│           ├── styles.go             ← GET/POST /api/styles, GET /api/styles/active
│           ├── raffles.go            ← GET/POST /api/raffles, raffle entries, image upload
│           ├── winners.go            ← GET /api/winners-log, GET /api/winners-log/frequent
│           └── ws.go                 ← GET /api/ws (delegates to hub)
├── data/                             ← SQLite DB created at runtime (gitignored)
```

## Architecture

**Layered**: HTTP handlers (`server`) → Game logic (`bingo`) → Data access (`store`) → SQLite.
All dependencies are wired in `main.go` and passed via structs (no globals, no singletons).

**Package responsibilities**:
- `model` — Pure data types with JSON struct tags. No logic, no imports beyond stdlib.
- `store` — Single `Store` struct wrapping `*sql.DB`. All database reads/writes. Returns typed structs, never `map[string]interface{}`.
- `bingo` — Stateless card/board generation functions + `GameService` for game lifecycle (start, draw, end, winner computation). Caches game state and card data in memory for performance.
- `ws` — WebSocket `Hub` for real-time broadcasts. Self-contained: manages client lifecycle, ping/pong, and message fan-out. Supports separate player/admin channels.
- `server` — `Server` struct implementing `http.Handler`. Holds Store, GameService, Hub, session store, admin password. Registers routes using Go 1.22+ method-pattern routing (`"GET /api/auth"`).

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

**Schema versioning**: `store/migrate.go` uses `PRAGMA user_version` to track schema version (currently **10**).
On the hot path (version == current), zero migration queries execute. Migrations run
incrementally only when the version is behind.

**Bingo card format**: 5×5 array `board[row][col]`; col 0=B(1-15) … col 4=O(61-75); centre `[2][2] = 0` (FREE).
**Pattern format**: 5×5 `[][]bool` grid; `true` = cell must be called for win; centre always counts.
**Card IDs**: 6-char alphanumeric (no ambiguous chars 0/O/1/I/l), enforced unique in DB.

## Database tables

| Table | Purpose |
|---|---|
| `cards` | `id TEXT PK`, `board_data TEXT` (JSON 5×5 array), `player_name TEXT`, `details TEXT` |
| `pattern_categories` | `id INTEGER PK`, `name`, `sort_order INTEGER` |
| `patterns` | `id INTEGER PK`, `name`, `pattern_data TEXT` (JSON), `sort_order INTEGER`, `category_id INTEGER` |
| `games` | `id INTEGER PK`, `status` (active/ended), `created_at`, `winners_cache TEXT` (JSON array of card IDs) |
| `game_patterns` | Snapshot of patterns when game started (game_id + pattern_id + name + data) |
| `called_numbers` | `game_id`, `number`, `call_order` |
| `settings` | `key TEXT PK`, `value TEXT` — key-value config (e.g. `game_details`, `active_style_id`) |
| `styles` | `id INTEGER PK`, `name`, `css_content TEXT`, `created_at` |
| `raffles` | `id INTEGER PK`, `title`, `description`, `rules`, `max_entries`, `signup_instructions`, `cost_per_entry`, `available_from`, `available_to`, `prize_image`, `status`, `winner_entry_id`, `created_at` |
| `raffle_entries` | `id INTEGER PK`, `raffle_id`, `character_name`, `world`, `num_entries`, `paid`, `created_at` |
| `winners_log` | `id INTEGER PK`, `logged_at`, `card_id`, `player_name`, `game_details`, `winning_patterns TEXT` (JSON) |

Indexes: `games(status)`, `called_numbers(game_id, call_order)`, `game_patterns(game_id)`, `raffle_entries(raffle_id)`, `winners_log(logged_at)`, `winners_log(player_name, logged_at)`, `cards(player_name)`.

## Frontend (Vue 3 + TypeScript + Vite)

The frontend lives in `frontend/` as a Vite project with Vue 3 SFCs, TypeScript,
and Pinia state management. It is a faithful migration of the legacy single-file
`index.html` + `assets/js/app.js` — same look, same behaviour — decomposed into
fine-grained components and stores.

**State (Pinia stores)**: `ui` (view routing + toasts), `app` (settings/fonts/
active CSS), `auth`, `player`, `game`, `cards`, `patterns`, `styles`, `raffles`,
`admin` (sidebar nav). The root `App.vue` owns the WebSocket lifecycle and view
transitions; `composables/useWebSocket.ts` dispatches WS messages into the stores.

**Routing is store-driven** (no vue-router): `ui.view` selects the top-level view
(`home` | `player` | `raffles` | `raffle-detail` | `admin-login` | `admin`);
`admin.adminSection` (`bingo` | `raffles` | `system`) + `admin.adminTab` select the
admin tab. `AdminView.vue` renders one component per tab + the four modals.

**Type sync**: TS domain types are generated from the Go `model` package via tygo
(`frontend/src/types/api.generated.ts`, committed; regenerate with `npm run gen:types`).
Request/response/WebSocket envelopes are hand-written in `types/api.ts`.

**Library choices** (migrated off CDNs):
- Markdown → `markdown-it`, **lazy-loaded** via `useMarkdown()` (`lib/markdown.ts`); the ~100 KB parser is dynamic-imported on first render (`breaks: true` to match the old marked output)
- Drag-and-drop → `vuedraggable` (SortableJS) for category reorder + pattern reorder/cross-category move
- Theme CSS editor → CodeMirror 6 via `vue-codemirror`; dark look reproduced in `lib/codemirror.ts` + app.css §26
- Icons → `@fortawesome/fontawesome-svg-core` + `dom.watch()` (keeps the existing `<i class="fa-…">` markup)

**Performance / tooling**:
- **Lazy routes**: every view + admin tab is a dynamic `import()` in `router/index.ts`, so heavy deps (CodeMirror, vuedraggable, markdown-it) load only when their route is visited — the player/home payload stays small. `manualChunks` (vite.config) keeps shared vendors cached across route chunks.
- **PWA**: `vite-plugin-pwa` (`registerType: 'autoUpdate'`) emits `sw.js` + `manifest.webmanifest`; the SW precaches the app shell and falls back to `index.html` for SPA routes, with `/api/` and `/images/` denylisted. The deploy `.htaccess` exempts `sw.js`/`registerSW.js`/`*.webmanifest` from the immutable cache so updates land.
- **Global error handler**: `app.config.errorHandler` (`main.ts`) surfaces uncaught errors as a toast.
- **Accessible modals**: `ModalOverlay.vue` traps focus, restores it on close, supports Escape, and sets `role="dialog"`/`aria-modal`.
- **Lint/format**: ESLint flat config (`eslint.config.js`) + Prettier (`.prettierrc.json`); `npm run lint` / `npm run format`. Bundle treemap via `npm run analyze` → `dist/stats.html`.

**Views** (`ui.view`): `home` → `admin-login` → `admin` | `player` | `raffles` → `raffle-detail`.
**Admin sections** (`adminSection`): `bingo` | `raffles` | `system`.
**Admin tabs** (`adminTab`): `bingo-game` | `bingo-cards` | `bingo-patterns` | `bingo-winners-log` | `raffle-open` | `raffle-closed` | `system-themes`.

**Player features**:
- Join by board ID; see bingo board, called numbers grid, and active win patterns
- Manually stamp cells (click to toggle); stamps persist in `localStorage` keyed by `stamps_{cardId}_{gameId}`
- Stamp customization: shape (blank default, heart, star, smiley, etc.), color (7 options), opacity slider
- Real-time updates via WebSocket (draws, game start/end, style changes, halftime alerts)
- WebSocket reconnect with exponential back-off on disconnect
- Browse open raffles from home page (card shown only when open raffles exist); view raffle detail with prize image, markdown description/rules, and sign-up form (character name, world, number of entries); after sign-up see confirmation with total cost and sign-up instructions
- View closed raffles with winner announcement and total entry count

**Admin features**:
- Password-protected login (session-based auth, 24-hour cookie)
- **Game tab**: start game (select patterns with category filter + search), draw numbers (optional player delay 0–60s), see called numbers, see winners; click winner ID to verify card with pattern-hit highlighting; frequent winners alert (3+ wins in 12h); end game with winner confirmation modal
- **Cards tab**: generate cards (1–500), view as chips with player name indicators, click to preview board, edit player name/details, delete individual or all
- **Patterns tabs**: create (5×5 grid editor with duplicate detection), organize into categories, rename inline (double-click name), reorder/move between categories via vuedraggable, delete; category CRUD with vuedraggable reorder
- **Winners Log tab**: paginated table of past winners with sorting and per-page controls
- **Raffles section**: create/edit raffles (title, description, rules as markdown, cost per entry, max entries, signup instructions, availability window, prize image upload); manage entries (view, toggle paid status, delete); weighted random winner pick; close/reopen raffles; delete raffles
- **Themes section**: CRUD custom CSS themes with CodeMirror 6 editor, activate/deactivate live

**Key UI patterns**:
- `[v-cloak]` (in app.css) prevents a flash before mount; the `#app` div carries it
- Optimistic updates: pattern/category reorder swaps locally then persists in background
- Toast notifications for success/error feedback (`ui` store)
- Admin login separates auth failure from data-loading failure to prevent false login rejections
- Winner toast only fires on new winners (compares count before/after draw)
- WebSocket reconnect with exponential back-off (1s → 2s → 4s → 8s → 16s, max 10 attempts)
- vuedraggable handles drag-and-drop for patterns and categories (replaces the old manual HTML5 DnD placeholders)

## Developer commands

```powershell
# ── Frontend (Vite) ──
cd frontend; npm install            # first time / after dependency changes
cd frontend; npm run dev            # dev server on :5173, proxies /api → :8080
cd frontend; npm run build          # type-check (vue-tsc) + build to frontend/dist/
cd frontend; npm run typecheck      # vue-tsc only
cd frontend; npm run lint           # ESLint (flat config) with --fix
cd frontend; npm run format         # Prettier write over src/
cd frontend; npm run analyze        # build + emit dist/stats.html bundle treemap (visualizer)
cd frontend; npm run gen:types      # regenerate TS types from Go models (runs tygo in ../src)

# ── Go backend ──
# Build the Go backend
cd src; go build -o app-suite.exe .

# Run the server (from project root or with -db flag)
cd src; go run . -addr :8080 -db ../data/bingo.sqlite

# Run with custom admin password
cd src; go run . -password "my-secret"

# Or via environment variable
$env:APPSUITE_ADMIN_PASSWORD = "my-secret"; cd src; go run .

# Vet / lint
cd src; go vet ./...

# Run tests
cd src; go test ./...

# Build for production
cd src; go build -ldflags="-s -w" -o app-suite .
```

## Deployment (Apache)

The frontend is served statically by Apache; `/api/*` + `/api/ws` are reverse-
proxied to the Go server. The document root layout keeps uploads separate from
the built SPA so redeploys never wipe them (full guide in `deploy/README.md`):

```
<DocumentRoot>/
├── .htaccess        ← deploy/.htaccess  (SPA fallback: serves dist/index.html; routes /assets → dist/assets; /images served from root)
├── images/          ← deploy/images/    (PERSISTENT: logo/favicon/banner + raffles/ uploads)
└── dist/            ← frontend/dist/    (built SPA; replaced each deploy)
```

- Run the Go server with `-webroot <DocumentRoot>`; uploads are written to
  `<DocumentRoot>/images/raffles/` and returned as `images/raffles/<file>`.
- `vite.config.ts` strips `dist/images/` after build (the `strip-dist-images`
  plugin) so the redundant copy doesn't shadow the persistent root `images/`.
- Schema migration v10 rewrites legacy `assets/images/raffles/...` prize paths
  to `images/raffles/...` automatically on first start.

## Conventions & patterns

- **Typed structs everywhere**: all store methods return typed `model.*` structs — no `map[string]interface{}`.
- **Dependency injection**: `Server` struct holds all deps (`Store`, `GameService`, `Hub`, sessions, password). No package-level globals.
- **Method-pattern routing**: Go 1.22+ `"GET /api/auth"` patterns — no manual method checks.
- **Typed JSON requests**: each handler defines a request struct and uses `readJSON[T]()` generic decoder.
- **JSON errors**: API failures return `{"error":"message"}` with appropriate HTTP status via `writeError()`.
- **Action-based POST**: POST endpoints use `{"action":"…"}` in JSON body to multiplex operations.
- **Session auth**: admin state stored in SCS session cookie; password configurable via `-password` flag or `APPSUITE_ADMIN_PASSWORD` env.
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
- **Schema versioning**: `PRAGMA user_version` in SQLite tracks migration state; `schemaVersion` constant in `store/migrate.go` controls the target (currently 10).
- **Optimistic UI**: pattern/category reordering swaps locally before API call; reverts on failure.
- **Lightweight endpoints**: `GET /api/cards` returns only IDs + player names (no board data); `GET /api/board?preview=1` returns only the card (no game state).
- **Batch operations**: `GetCardPlayerNames()` fetches multiple cards in one query; `SaveCardsBatch()` uses transactions.
- **`math/rand/v2`**: card generation uses `math/rand/v2` (auto-seeded, not crypto — appropriate for bingo).
- **`log/slog`**: structured logging for server startup and errors.
- **SQLite pragmas**: WAL mode, synchronous=NORMAL, busy_timeout=5000, cache_size=8MB, mmap_size=32MB, foreign_keys=ON, temp_store=MEMORY, max_open_conns=4.
- **WebSocket bypass**: `/api/ws` is routed directly to the mux, bypassing the SCS session middleware and `responseWriter` wrapper — `coder/websocket` requires the raw `http.ResponseWriter` for the upgrade handshake.
- **Pre-built session handler**: `sessions.LoadAndSave(mux)` is constructed once in `New()` and stored as `sessHandler` — not rebuilt per-request.
- **Client-side keepalive**: JS sends a `ping` text message every 25s to prevent reverse-proxy idle timeouts from dropping the WebSocket.
- **Rate limiter**: IP-based brute-force protection for admin login; reads `X-Forwarded-For` for the real client IP behind a reverse proxy.
- **FontAwesome**: bundled via `@fortawesome/fontawesome-svg-core`; only the icons used are registered in `frontend/src/lib/fontawesome.ts`, and `dom.watch()` replaces the existing `<i class="fa-…">` markup with inline SVG (no kit/account needed).
- **Theme CSS editor**: CodeMirror 6 via `vue-codemirror`, bound with `v-model` to the edited theme's `css_content`; the dark look/syntax palette lives in `frontend/src/lib/codemirror.ts` (theme + HighlightStyle) plus structural rules in app.css §26.
- **TS↔Go type sync**: `frontend/src/types/api.generated.ts` is generated from `internal/model` by tygo (`src/tygo.yaml`); regenerate with `npm run gen:types`. Never edit the generated file.

## Key files to inspect first

1. `src/internal/server/server.go` — Server struct, route registration, middleware, JSON/auth helpers, broadcast helpers
2. `src/internal/store/store.go` — All database access methods with typed returns
3. `src/internal/bingo/game.go` — Core game logic: start, draw + winner compute + cache, state reads, in-memory caching
4. `src/internal/bingo/card.go` — Card/board generation algorithm with column-range constraints
5. `src/internal/model/model.go` — All domain types (Card, Pattern, BingoGame, BingoGameState, BingoDrawnNumber, Raffle, WinnersLogEntry, etc.)
6. `frontend/src/App.vue` + `frontend/src/composables/useWebSocket.ts` — root shell, view routing, WebSocket message dispatch
7. `frontend/src/stores/*.ts` — all client state + actions (mirror of the old app.js data/methods)
8. `frontend/src/assets/app.css` — all styles (the look to preserve); imported in `main.ts` (content-hashed by Vite); §26 covers the CM6 editor

## Extending the project

- **New API endpoint**: add a handler method on `*Server` in `internal/server/`, register the route in `routes()`.
- **New domain type**: add to `internal/model/model.go`, then run `npm run gen:types` to refresh the TS types.
- **New store method**: add to `internal/store/store.go`, returning typed structs.
- **New migration**: bump `schemaVersion` in `store/migrate.go`, add an `if version < N` block in `ensureSchema()`.
- **New admin tab**: add the tab id to `AdminTab` + `adminNav()` in `frontend/src/stores/admin.ts`, add a button in `components/admin/AdminSidebar.vue`, create a `components/admin/<Tab>.vue` (wrap content in `.admin-panel`), and render it in `views/AdminView.vue`.
- **New WebSocket message type**: add a case in `composables/useWebSocket.ts` (and the `WsMessage` union in `types/api.ts`), send from server via `hub.Broadcast()` / `hub.BroadcastToPlayers()` / `hub.BroadcastToAdmins()`.
