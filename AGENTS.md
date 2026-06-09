# AGENTS.md

Guidance for AI coding agents working in this codebase.

## Quick orientation

Go + SQLite backend serving a Vue 3 + TypeScript single-page frontend built with
Vite. The backend binary is built from `src/` and serves only the API/WebSocket
on `:8080`; the built frontend (`frontend/dist/`) is served statically (Apache),
with `/api/*` proxied to the Go server.

**At a glance:** Vue 3 SFCs + TypeScript (strict) · Pinia (setup stores) · Vue
Router (history mode, lazy routes) · Go 1.22+ stdlib HTTP · SQLite (WAL) ·
coder/websocket · Vitest + Vue Test Utils · GitHub Actions CI.

```
├── frontend/                         ← Vue 3 + TypeScript SPA (Vite)
│   ├── index.html                    ← Vite entry (mounts #app)
│   ├── package.json                  ← deps: vue, pinia, vuedraggable, markdown-it, vue-codemirror, @fortawesome/*
│   ├── vite.config.ts                ← build → dist/, dev proxy for /api, manualChunks, strip dist/images
│   ├── tsconfig*.json                ← TS project refs (app + node)
│   ├── public/                       ← copied verbatim into dist/ at build
│   │   └── images/                   ← logo, favicon, share banner (dev only; stripped from dist — served from doc root in prod)
│   ├── vitest.config.ts              ← Vitest config (jsdom, @ alias; separate from vite.config so tests skip PWA/visualizer)
│   └── src/
│       ├── main.ts                   ← createApp + Pinia + Router + FontAwesome init (imports assets/app.css; registers 401 handler)
│       ├── App.vue                   ← root shell: <RouterView>, WebSocket lifecycle, toast, route progress bar
│       ├── assets/app.css            ← All styles (dark theme, board, admin, modals); imported in main.ts so Vite content-hashes it
│       ├── router/index.ts           ← Vue Router (history mode): route map, lazy route components, admin auth guard
│       ├── lib/                      ← framework-agnostic helpers
│       │   ├── api.ts                ← typed fetch client (credentials, JSON, error extraction, global 401 handler)
│       │   ├── endpoints.ts          ← typed endpoint layer over api() — one fn per backend path (stores call these)
│       │   ├── ws.ts                 ← WsClient: reconnect back-off + keepalive ping
│       │   ├── markdown.ts           ← markdown-it renderer (lazy-loaded; replaces CDN marked.js)
│       │   ├── fontawesome.ts        ← SVG-core library + dom.watch() (replaces FA kit script)
│       │   ├── codemirror.ts         ← CM6 CSS-editor extensions + dark theme/highlight
│       │   ├── theme.ts              ← header-font + custom-CSS <head> injection; registers uploaded-font @font-face rules (metric-clamped to tame oversized vertical metrics)
│       │   ├── assets.ts             ← resolves server image paths (images/...) to root-absolute URLs
│       │   ├── exportCard.ts         ← player card → framed PNG export (html-to-image capture + canvas composite)
│       │   ├── sound.ts              ← opt-in draw chime (Web Audio synth) + haptics for the player view
│       │   └── constants.ts          ← stamp shapes/colors, column helpers, fallback fonts
│       ├── types/
│       │   ├── api.generated.ts      ← tygo-generated from Go model — GITIGNORED, DO NOT EDIT (run `npm run gen:types`)
│       │   └── api.ts                ← re-exports + hand-written request/response/WS envelopes
│       ├── stores/                   ← Pinia stores (ui, app, auth, player, game, cards, patterns, styles, raffles, fonts, admin)
│       ├── composables/useWebSocket.ts ← wires WsClient → stores (message dispatch)
│       ├── components/
│       │   ├── common/               ← BingoBoard, CalledNumbers, PatternMini, ModalOverlay, ConfirmModal, ToastNotification, LoadingSpinner, RouteProgressBar
│       │   ├── player/               ← Stamp{Shape,Color,Opacity} pickers, WinPatternsPanel
│       │   └── admin/                ← Sidebar + one component per tab (12, incl. SettingsTab + FontsTab) + modals (CardPreview, EndGame, WinnerVerify, HalftimePrompt)
│       ├── views/                    ← HomeView, PlayerView, RafflesView, RaffleDetailView, AdminLoginView, AdminView
│       └── **/*.test.ts              ← Vitest unit/component tests, colocated next to the code they cover
├── .github/workflows/ci.yml          ← CI: frontend (lint·typecheck·test·build) + backend (build·vet·test)
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
│           ├── raffles.go            ← GET/POST /api/raffles, raffle entries (add/mark-paid/delete/pick), image upload
│           ├── winners.go            ← GET /api/winners-log, GET /api/winners-log/frequent
│           ├── settings.go           ← GET/POST /api/settings (app title, draw delay, header font, Google Fonts key)
│           ├── fonts.go              ← GET/POST /api/fonts, POST /api/fonts/upload (uploaded font files in <webRoot>/fonts)
│           ├── ratelimit.go          ← IP-based brute-force limiter for admin login
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
| `settings` | `key TEXT PK`, `value TEXT` — key-value config (e.g. `game_details`, `active_style_id`, `app_title`, `default_draw_delay`, `frequent_winner_threshold`/`_hours`, `header_font`, `google_fonts_api_key`) |
| `styles` | `id INTEGER PK`, `name`, `css_content TEXT`, `created_at` |
| `raffles` | `id INTEGER PK`, `title`, `description`, `rules`, `max_entries`, `signup_instructions`, `cost_per_entry`, `available_from`, `available_to`, `prize_image`, `status`, `winner_entry_id`, `created_at` |
| `raffle_entries` | `id INTEGER PK`, `raffle_id`, `character_name`, `world`, `num_entries`, `paid`, `created_at` |
| `winners_log` | `id INTEGER PK`, `logged_at`, `card_id`, `player_name`, `game_details`, `winning_patterns TEXT` (JSON) |

Indexes: `games(status)`, `called_numbers(game_id, call_order)`, `game_patterns(game_id)`, `raffle_entries(raffle_id)`, `winners_log(logged_at)`, `winners_log(player_name, logged_at)`, `cards(player_name)`.

**On-disk uploads (not in the DB)**: raffle prize images in `<webRoot>/images/raffles/`, and admin-uploaded **fonts** in `<webRoot>/fonts/` (`.ttf/.otf/.woff/.woff2/.eot`). Fonts are served by a **separate** vhost (`https://fonts.senpan.cafe`), so cross-origin `@font-face` needs CORS headers — see Deployment.

## Frontend (Vue 3 + TypeScript + Vite)

The frontend lives in `frontend/` as a Vite project with Vue 3 SFCs, TypeScript,
and Pinia state management. It is a faithful migration of the legacy single-file
`index.html` + `assets/js/app.js` — same look, same behaviour — decomposed into
fine-grained components and stores.

**State (Pinia stores)**: `ui` (toasts, themed confirm dialog, route-loading
flag, realtime connection status), `app` (settings/fonts/active CSS), `auth`,
`player`, `game`, `cards`,
`patterns`, `styles`, `raffles`, `admin` (sidebar highlight state + per-tab data
loads). The root `App.vue` hosts `<RouterView>` and owns the WebSocket lifecycle;
`composables/useWebSocket.ts` dispatches WS messages into the stores.

**Routing is Vue Router** (history mode, `router/index.ts`) — real linkable URLs,
not store-driven view switching:
- Public: `/`, `/play/:cardId`, `/raffles`, `/raffles/:id`, `/admin/login`.
- Admin: `/admin` (layout, `requiresAdmin`) with child routes
  `bingo/{game,cards,winners-log,categories,new-pattern,patterns}`,
  `raffles/{new,open,closed}`, `system/{settings,themes,fonts}`. The admin tabs are
  **child routes** of the `AdminView` layout, so the sidebar/topbar persist while
  the matched child renders the active tab.
- Every view + admin tab is a lazy `import()` so heavy deps (CodeMirror,
  vuedraggable, markdown-it) split into on-demand chunks.
- A global `beforeEach` guard enforces admin auth (redirect to `/admin/login`
  with a `redirect` query), and calls `admin.setTabFromRoute(meta.tab)` to sync
  the sidebar highlight + run that tab's data load. Unknown paths redirect home.
- `api.ts`'s global 401 handler (registered in `main.ts`) redirects to the login
  route with a "session expired" toast when any non-auth request 401s.

**Type sync**: TS domain types are generated from the Go `model` package via tygo
into `frontend/src/types/api.generated.ts` — this file is **gitignored** (each dev
regenerates it locally with `npm run gen:types`; CI/build regenerate as needed).
Never edit it by hand. Request/response/WebSocket envelopes are hand-written in
`types/api.ts`.

**Library choices** (migrated off CDNs):
- Markdown → `markdown-it`, **lazy-loaded** via `useMarkdown()` (`lib/markdown.ts`); the ~100 KB parser is dynamic-imported on first render (`breaks: true` to match the old marked output)
- Drag-and-drop → `vuedraggable` (SortableJS) for category reorder + pattern reorder/cross-category move
- Theme CSS editor → CodeMirror 6 via `vue-codemirror`; dark look reproduced in `lib/codemirror.ts` + app.css §26
- Icons → `@fortawesome/fontawesome-svg-core` + `dom.watch()` (keeps the existing `<i class="fa-…">` markup)

**Performance / tooling**:
- **Lazy routes**: every view + admin tab is a dynamic `import()` in `router/index.ts`, so heavy deps (CodeMirror, vuedraggable, markdown-it) load only when their route is visited — the player/home payload stays small. `manualChunks` (vite.config) keeps shared vendors cached across route chunks.
- **PWA**: `vite-plugin-pwa` (`registerType: 'autoUpdate'`) emits `sw.js` + `manifest.webmanifest`; the SW precaches the app shell and falls back to `index.html` for SPA routes, with `/api/` and `/images/` denylisted. The deploy `.htaccess` exempts `sw.js`/`registerSW.js`/`*.webmanifest` from the immutable cache so updates land.
- **Route progress + loading UX**: a top progress bar (`RouteProgressBar.vue`, driven by `ui.routeLoading` from the router guards) shows during async navigation/lazy-chunk loads; stores expose per-action loading flags (`joining`, `drawing`, `starting`, …) that drive `LoadingSpinner.vue` + disabled buttons.
- **Global error handler**: `app.config.errorHandler` (`main.ts`) surfaces uncaught errors as a toast.
- **Accessible modals**: `ModalOverlay.vue` traps focus, restores it on close, supports Escape, and sets `role="dialog"`/`aria-modal`.
- **Lint/format**: ESLint flat config (`eslint.config.js`) + Prettier (`.prettierrc.json`); `npm run lint` (autofix) / `npm run lint:check` (no fix, used by CI) / `npm run format`. Bundle treemap via `npm run analyze` → `dist/stats.html`.

**Testing** (Vitest + Vue Test Utils, jsdom):
- Config in `vitest.config.ts` (kept separate from `vite.config.ts` so tests don't load the PWA/visualizer plugins). Tests **import `describe/it/expect/vi` from `vitest`** explicitly (`globals: false`) so they type-check with no extra global-types config.
- Test files are colocated as `src/**/*.test.ts`. Current coverage: `lib/{constants,api,endpoints,exportCard,theme}.test.ts`, `stores/{player,ui,raffles}.test.ts`, `components/common/BingoBoard.test.ts`.
- Run: `npm run test` (CI), `npm run test:watch`, `npm run test:coverage` (v8).
- Patterns: mock `fetch` via `vi.stubGlobal` for `api.ts`; mock the `./api` module with `vi.hoisted` + `vi.mock` for `endpoints.ts`; Pinia stores use `setActivePinia(createPinia())`; components use `mount()` from `@vue/test-utils`. Pure helpers tested directly (e.g. `exportCard.ts` exports `parseInlineRuns`/`parseDetailParagraphs` for this).

**Public routes**: `/` · `/play/:cardId` · `/raffles` · `/raffles/:id` · `/admin/login`.
**Admin sections** (`adminSection`, sidebar highlight): `bingo` | `raffles` | `system`.
**Admin tabs** (`adminTab` / route): `bingo-game` · `bingo-cards` · `bingo-winners-log` · `bingo-categories` · `bingo-new-pattern` · `bingo-patterns` · `raffle-new` · `raffle-open` · `raffle-closed` · `system-settings` · `system-themes` · `system-fonts`.

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
- Password-protected login (session-based auth, 24-hour cookie)
- **Game tab**: start game (select patterns with category filter + search), draw numbers (optional player delay 0–60s; **press `Space`/`Enter` to draw**), live "Live" badge + elapsed-time clock, see called numbers, see winners; click winner ID to verify card with pattern-hit highlighting; frequent winners alert (3+ wins in 12h); end game with winner confirmation modal
- **Cards tab**: generate cards (1–500), view as chips with player name indicators, click to preview board, edit player name/details, delete individual or all
- **Patterns tabs**: create (5×5 grid editor with duplicate detection), organize into categories, rename inline (double-click name), reorder/move between categories via vuedraggable, delete; category CRUD with vuedraggable reorder
- **Winners Log tab**: paginated table of past winners with sorting and per-page controls
- **Raffles section**: create/edit raffles (title, description, rules as markdown, cost per entry, max entries, signup instructions, availability window, prize image upload); manage entries (view, **manually add an entry — optionally marked paid right away**, toggle paid status, delete); weighted random winner pick; close/reopen raffles; delete raffles
- **System → Settings tab**: app title, default draw delay, frequent-winner threshold/window, and the **header/board font** picker — a combo box with optgroups for uploaded fonts (first) and Google Fonts, plus an optional Google Fonts API key and a live font preview
- **System → Themes tab**: CRUD custom CSS themes with CodeMirror 6 editor, activate/deactivate live
- **System → Font Upload tab** (`FontsTab.vue`): upload one or more font files (`.ttf/.otf/.woff/.woff2/.eot`) to `<webRoot>/fonts/`, rename/delete them, copy each file's public URL, and a **live preview** (type any text, pick a font) — all with the same metric clamping used app-wide so oversized fonts render sensibly. Uploaded fonts become selectable as the header/board font for everyone, not just the admin preview.

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
cd frontend; npm run test           # Vitest run (CI mode, one-shot)
cd frontend; npm run test:watch     # Vitest watch mode
cd frontend; npm run test:coverage  # Vitest + v8 coverage report
cd frontend; npm run lint           # ESLint (flat config) with --fix
cd frontend; npm run lint:check     # ESLint without --fix (CI gate)
cd frontend; npm run format         # Prettier write over src/
cd frontend; npm run analyze        # build + emit dist/stats.html bundle treemap (visualizer)
cd frontend; npm run gen:types      # regenerate TS types from Go models (runs tygo in ../src; needs Go)

# NOTE: src/types/api.generated.ts is gitignored. Run `npm run gen:types` after a
# fresh clone (or after changing Go model types) before build/typecheck/test.

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

## Continuous integration

`.github/workflows/ci.yml` runs on every push and pull request, with two jobs:

- **frontend** (`working-directory: frontend`): `npm ci` → `npm run gen:types`
  (needs Go, so the job also sets up the Go toolchain) → `lint:check` →
  `typecheck` → `test` → `build`. Mirrors the local gate, so a green CI ==
  the checks a developer runs locally have passed.
- **backend** (`working-directory: src`): `go build ./...` → `go vet ./...` →
  `go test ./...` (Go version read from `src/go.mod`).

When adding a check, wire it into both the relevant npm/go script **and** the
workflow so local and CI stay in lockstep.

## Deployment (Apache)

The frontend is served statically by Apache; `/api/*` + `/api/ws` are reverse-
proxied to the Go server. The document root layout keeps uploads separate from
the built SPA so redeploys never wipe them (full guide in `deploy/README.md`):

```
<DocumentRoot>/
├── .htaccess        ← deploy/.htaccess  (SPA fallback: serves dist/index.html; routes /assets → dist/assets; /images served from root)
├── images/          ← deploy/images/    (PERSISTENT: logo/favicon/banner + raffles/ uploads)
├── fonts/           ← (PERSISTENT: admin-uploaded font files; served by the fonts.senpan.cafe vhost)
└── dist/            ← frontend/dist/    (built SPA; replaced each deploy)
```

- Run the Go server with `-webroot <DocumentRoot>`; uploads are written to
  `<DocumentRoot>/images/raffles/` (prize images) and `<DocumentRoot>/fonts/`
  (uploaded fonts), and returned as `images/raffles/<file>` / `<file>`.
- `vite.config.ts` strips `dist/images/` after build (the `strip-dist-images`
  plugin) so the redundant copy doesn't shadow the persistent root `images/`.
- Schema migration v10 rewrites legacy `assets/images/raffles/...` prize paths
  to `images/raffles/...` automatically on first start.
- **Font host (CORS)**: uploaded fonts are served cross-origin from
  `https://fonts.senpan.cafe`, so `@font-face` loads are CORS-restricted (unlike
  `<img>`). The font vhost must send `Access-Control-Allow-Origin` — deploy
  `deploy/fonts.htaccess` to `<webRoot>/fonts/.htaccess` (needs `mod_headers` +
  `AllowOverride`). Without it, uploaded fonts silently fall back to serif. See
  the **Font host (CORS)** section of `deploy/README.md`.

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
- **TS↔Go type sync**: `frontend/src/types/api.generated.ts` is generated from `internal/model` by tygo (`src/tygo.yaml`); it is **gitignored** — regenerate with `npm run gen:types` (needs Go) after a fresh clone or model change. Never edit the generated file.
- **Typed endpoint layer**: stores never call `api<T>('path')` directly — they call `endpoints.*` (`frontend/src/lib/endpoints.ts`), which wraps every backend path in a typed function. Add new endpoints there so paths/bodies/response types live in one place.
- **Global 401 handling**: `api.ts` invokes a registered handler (set in `main.ts`) on any non-auth 401 → redirect to `/admin/login` + "session expired" toast. Auth endpoints pass `skipAuthRedirect` so a bad-password login doesn't trigger it.
- **Themed confirm dialog**: use `await ui.confirm(message, opts)` (renders `ConfirmModal.vue`) instead of the native `window.confirm`.
- **Theme fidelity**: new UI must reuse existing `app.css` class names + theme tokens (CSS custom properties in `:root`) so user-authored custom CSS themes keep working — never hard-code colors/fonts.
- **Uploaded fonts**: `applyUploadedFonts()` (`lib/theme.ts`) writes one `<style id="uploaded-fonts">` with an `@font-face` per file from `FONT_BASE_URL` (`stores/fonts.ts`), so a font is registered app-wide (board/header + every preview), not per-component. It then measures each loaded font via the Canvas TextMetrics API and, for fonts with oversized vertical metrics, rewrites the rule with `ascent-override`/`descent-override`/`line-gap-override` to clamp the box (`clampFontMetrics()` — pure + unit-tested). The Settings preview and the Font Upload live preview both rely on these shared rules, so previews always match what players see.

## Key files to inspect first

1. `src/internal/server/server.go` — Server struct, route registration, middleware, JSON/auth helpers, broadcast helpers
2. `src/internal/store/store.go` — All database access methods with typed returns
3. `src/internal/bingo/game.go` — Core game logic: start, draw + winner compute + cache, state reads, in-memory caching
4. `src/internal/bingo/card.go` — Card/board generation algorithm with column-range constraints
5. `src/internal/model/model.go` — All domain types (Card, Pattern, BingoGame, BingoGameState, BingoDrawnNumber, Raffle, WinnersLogEntry, etc.)
6. `frontend/src/router/index.ts` — route map, lazy route components, admin auth guard + tab sync
7. `frontend/src/App.vue` + `frontend/src/composables/useWebSocket.ts` — root shell (`<RouterView>`), WebSocket message dispatch
8. `frontend/src/lib/endpoints.ts` — the typed surface over every backend path (what stores call)
9. `frontend/src/stores/*.ts` — all client state + actions (mirror of the old app.js data/methods)
10. `frontend/src/assets/app.css` — all styles (the look to preserve); imported in `main.ts` (content-hashed by Vite); §26 covers the CM6 editor

## Extending the project

- **New API endpoint**: add a handler method on `*Server` in `internal/server/`, register the route in `routes()`, then add a typed wrapper in `frontend/src/lib/endpoints.ts` for the frontend to call.
- **New domain type**: add to `internal/model/model.go`, then run `npm run gen:types` to refresh the TS types.
- **New store method**: add to `internal/store/store.go`, returning typed structs.
- **New migration**: bump `schemaVersion` in `store/migrate.go`, add an `if version < N` block in `ensureSchema()`.
- **New admin tab**: add the tab id to the `AdminTab` union in `frontend/src/stores/admin.ts` (and any per-tab data load in `setTabFromRoute()`), add a child route with `meta.tab` in `router/index.ts`, add a sidebar link in `components/admin/AdminSidebar.vue`, and create a `components/admin/<Tab>.vue` (wrap content in `.admin-panel`) referenced by the route's lazy `import()`.
- **New route/view**: add a record to `routes` in `router/index.ts` with a lazy `component: () => import(...)`; mark `meta.requiresAdmin` if it needs auth.
- **New WebSocket message type**: add a case in `composables/useWebSocket.ts` (and the `WsMessage` union in `types/api.ts`), send from server via `hub.Broadcast()` / `hub.BroadcastToPlayers()` / `hub.BroadcastToAdmins()`.
- **New test**: colocate a `*.test.ts` next to the code; import `{ describe, it, expect }` from `vitest`; run `npm run test`. Add it to the same gate CI runs.
