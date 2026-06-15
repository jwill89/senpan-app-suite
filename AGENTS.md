# AGENTS.md

Guidance for AI coding agents working in this codebase.

## Quick orientation

Go + SQLite backend serving a Vue 3 + TypeScript single-page frontend built with
Vite. The backend binary is built from `src/` and serves only the API/WebSocket
on `:8080`; the built frontend (`frontend/dist/`) is served statically (Apache),
with `/api/*` proxied to the Go server.

Beyond bingo, the app is a small "suite" for a Discord community: **raffles**,
**book clubs** (reading lists pulled from AniList + scheduled meeting events),
and **announcements** — the latter two post **Discord embeds** (manually or on a
schedule via background goroutines). It also hosts images for external **Carrd**
sites and admin-uploaded **fonts**.

**At a glance:** Vue 3 SFCs + TypeScript (strict) · Pinia (setup stores) · Vue
Router (history mode, lazy routes) · Go 1.22+ stdlib HTTP · SQLite (WAL) ·
coder/websocket · embedded tzdata + background schedulers (Discord embeds) ·
Vitest + Vue Test Utils · GitHub Actions CI.

```
├── frontend/                         ← Vue 3 + TypeScript SPA (Vite)
│   ├── index.html                    ← Vite entry (mounts #app)
│   ├── package.json                  ← deps: vue, pinia, vuedraggable, markdown-it, vue-codemirror, sortablejs, @fortawesome/* (Pro), html-to-image
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
│       │   ├── datetime.ts           ← UTC ⇄ <input datetime-local> (wall-clock) conversion; IANA tz helpers for events/announcements
│       │   ├── freshness.ts          ← createFreshness(): keyed time-gate so re-entered admin tabs skip redundant refetches (snappy navigation)
│       │   └── constants.ts          ← stamp shapes/colors, column helpers, fallback fonts, BOOK_CLUBS registry + per-club webhook setting keys
│       ├── types/
│       │   ├── api.generated.ts      ← tygo-generated from Go model — GITIGNORED, DO NOT EDIT (run `npm run gen:types`)
│       │   └── api.ts                ← re-exports + hand-written request/response/WS envelopes
│       ├── stores/                   ← Pinia stores (ui, app, auth, player, game, cards, patterns, presets, styles, raffles, fonts, bookclub, carrd, announcements, admin)
│       ├── composables/
│       │   ├── useWebSocket.ts       ← wires WsClient → stores (message dispatch)
│       │   └── usePwaInstall.ts      ← beforeinstallprompt capture + install/standalone state for the PWA "Install" affordance
│       ├── components/
│       │   ├── common/               ← BingoBoard, CalledNumbers, PatternMini, ModalOverlay, ConfirmModal, ToastNotification, LoadingSpinner, RouteProgressBar, MarkdownEditor (WYSIWYG), AppFooter, CornerFlourish
│       │   │   └── ui/                ← admin UI primitives (presentational, render stable themeable classes). Forms/tables: AdminPanel, FormField, FormRow, FormActions, DataTable, PaginationBar, EmptyState. Manager model: ManagerView (list page shell), ListRow (item row, actions far-right), SubPageHeader (Back sub-page header), SearchInput. Shared widgets: PatternPicker (v-model selected pattern ids — search + category filter + Select-All + collapse-all over a grouped collapsible checkbox grid; used by GameTab + the Preset editor), ImageField (upload-or-reuse-an-image field; announcement + book-club event forms), ColorPicker (lazy vue-color Chrome wrapper + .color-picker skin; stamp + theme tools). Every admin "manage items" tab routes through these for one consistent structure
│       │   ├── player/               ← Stamp{Shape,Color,Opacity} pickers, WinPatternsPanel
│       │   └── admin/                ← AdminSidebar + one component per tab + modals (CardPreview, EndGame, WinnerVerify, HalftimePrompt) + ThemeColorPickerTool. Tabs: Game, Cards, WinnersLog, Patterns (one manager unifying the patterns list + New Pattern / Manage Categories sub-pages), Presets, RaffleForm, Open/ClosedRaffles, Announcements, BookClub (one generic tab serves every club), Settings, Themes, Fonts, CarrdUpload
│       ├── views/                    ← HomeView, PlayerView, RafflesView, RaffleDetailView, AdminLoginView, AdminView
│       └── **/*.test.ts              ← Vitest unit/component tests, colocated next to the code they cover
├── .github/workflows/ci.yml          ← CI: frontend (lint·typecheck·test·build) + backend (build·vet·test)
├── deploy/                           ← Apache deploy artifacts (.htaccess + persistent images/ + README)
├── src/                              ← Go backend
│   ├── main.go                       ← Entry point: flags, DB init, server start
│   ├── tygo.yaml                     ← Go→TS type generation config (run `npm run gen:types`)
│   ├── go.mod / go.sum               ← Module deps (alexedwards/scs, coder/websocket, ncruces sqlite)
│   └── internal/
│       ├── model/model.go            ← Domain types (Card, Pattern, GamePreset, BingoGame, BingoGameState, Raffle, ReadingList(+Item/Source), BookClubEvent, AnnouncementType, Announcement, etc.)
│       ├── store/                    ← Store struct wrapping *sql.DB; one file per feature's typed CRUD
│       │   ├── store.go              ← Store struct, New()/Close(), pragmas, shared helpers (one domain per file: cards.go, winners.go, patterns.go, games.go, settings.go, styles.go, presets.go, raffles.go, bookclubs.go, bookclub_events.go, announcements.go)
│       │   ├── bookclubs.go          ← reading_lists + reading_list_items CRUD
│       │   ├── bookclub_events.go    ← book_club_events CRUD + due-event query (scheduler)
│       │   ├── announcements.go      ← announcement_types + announcements CRUD + due/advance helpers (scheduler)
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
│           ├── presets.go            ← GET/POST /api/presets (reusable game templates)
│           ├── styles.go             ← GET/POST /api/styles, GET /api/styles/active
│           ├── raffles.go            ← GET/POST /api/raffles, raffle entries (add/mark-paid/delete/pick), image upload
│           ├── bookclubs.go          ← reading lists + items, AniList lookup proxy, cover upload, Discord publish; book-club registry + per-club webhook setting keys (init)
│           ├── bookclub_events.go    ← GET/POST /api/bookclub/events, event image upload/list, embed builder + RunEventScheduler
│           ├── announcements.go      ← announcement types + announcements (send_now/skip_next), image upload/list, recurrence math + RunAnnouncementScheduler
│           ├── embeds.go             ← shared Discord embed schema + fluent builder + colour helper + postDiscordEmbed transport
│           ├── uploads.go            ← shared upload helpers: saveMultipartFile + saveSingleImageUpload/listUploadedImageURLs (+ image rel-dir consts)
│           ├── scheduler.go          ← runScheduler: shared ticker/sweep loop behind the announcement + event schedulers
│           ├── carrd.go              ← Carrd image-host projects/dirs/uploads under <webRoot>/carrd (System → Atelier → Carrd Upload)
│           ├── winners.go            ← GET /api/winners-log, GET /api/winners-log/frequent
│           ├── settings.go           ← GET/POST /api/settings (app title, draw delay, fonts, AniList URL, join prompt; secret per-club webhooks)
│           ├── fonts.go              ← GET/POST /api/fonts, POST /api/fonts/upload (uploaded font files in <webRoot>/fonts)
│           ├── ratelimit.go          ← IP-based brute-force limiter for admin login
│           ├── tzdata.go             ← blank-imports time/tzdata so IANA timezones resolve on hosts without zoneinfo (Windows)
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
- `server` — `Server` struct implementing `http.Handler`. Holds Store, GameService, Hub, session store, admin password, web root. Registers routes using Go 1.22+ method-pattern routing (`"GET /api/auth"`). Also owns the **Discord-embed** features (announcements, book-club reading lists + events) and their **background schedulers**, the **AniList** lookup proxy, and the on-disk upload areas (raffle/announcement/book-club/event images + Carrd projects).

**Discord embeds**: `embeds.go` holds the shared embed schema, a fluent `newEmbed()…build()` builder (auto-truncates to Discord's per-field limits, skips empty fields), a `#rrggbb`→int colour helper, and `postDiscordEmbed()` transport. Feature code (announcements, reading-list items, events) only assembles a builder chain — no feature has its own transport. Outbound calls (AniList + webhooks) share `bookclubHTTPClient` (15s timeout).

**Background schedulers**: `main.go` launches two goroutines tied to a shutdown-cancelled context — `RunEventScheduler` (book-club events) and `RunAnnouncementScheduler` (announcements). Both share the ticker/sweep loop in `scheduler.go` (`runScheduler`): each ticks every 30s (and sweeps once on startup to catch up after downtime), posts due items to the relevant webhook, and is resilient — an item whose webhook is unset is left pending, a failed post is retried next tick, and `skip_next`/recurrence advance the cursor without losing the schedule. Wall-clock times are resolved against each item's IANA timezone (DST-safe) and stored as UTC; `tzdata.go` embeds the zone database so this works on Windows hosts too.

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

**Schema versioning**: `store/migrate.go` uses `PRAGMA user_version` to track schema version (currently **20**).
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
| `settings` | `key TEXT PK`, `value TEXT` — key-value config (e.g. `game_details`, `active_style_id`, `app_title`, `default_draw_delay`, `frequent_winner_threshold`/`_hours`, `header_font`, `google_fonts_api_key`, `anilist_api_url`, `bingo_join_prompt`, and per-club `discord_webhook_url_<slug>` / `discord_events_webhook_url_<slug>`) |
| `styles` | `id INTEGER PK`, `name`, `css_content TEXT`, `created_at` |
| `game_presets` | `id INTEGER PK`, `name`, `pattern_ids TEXT` (JSON), `game_details TEXT`, `created_at` — reusable game templates |
| `raffles` | `id INTEGER PK`, `title`, `description`, `rules`, `max_entries`, `signup_instructions`, `cost_per_entry`, `available_from`, `available_to`, `prize_image`, `status`, `winner_entry_id`, `created_at` |
| `raffle_entries` | `id INTEGER PK`, `raffle_id`, `character_name`, `world`, `num_entries`, `paid`, `created_at` |
| `winners_log` | `id INTEGER PK`, `logged_at`, `card_id`, `player_name`, `game_details`, `winning_patterns TEXT` (JSON) |
| `reading_lists` | `id INTEGER PK`, `club_slug`, `title`, `created_at` — a book club's named reading list |
| `reading_list_items` | `id INTEGER PK`, `list_id`, `cover_image`, `title`, `summary`, `format`, `genres`, `tropes`, `chapters`, `comments`, `sources TEXT` (JSON), `sort_order` |
| `book_club_events` | `id INTEGER PK`, `club_slug`, `title`, `start_local`, `timezone`, `length_hours`, `location`, `details`, `image`, `post_at_local`, `start_at`/`post_at` (computed UTC RFC-3339), `posted`, `posted_at`, `created_at` |
| `announcement_types` | `id INTEGER PK`, `name`, `webhook_url`, `created_at` — a named Discord destination |
| `announcements` | `id INTEGER PK`, `type_id`, `title`, `details` (markdown), `image`, `color`, event window (`start_local`/`end_local` + computed `start_at`/`end_at`), schedule (`schedule_kind`, `timezone`, `once_local`, `schedule_minutes`, `schedule_weekdays`, `schedule_week_of_month`), `next_post_at`, `skip_next`, `active`, `last_posted_at`, `created_at` |

Indexes: `games(status)`, `called_numbers(game_id, call_order)`, `game_patterns(game_id)`, `raffle_entries(raffle_id)`, `winners_log(logged_at)`, `winners_log(player_name, logged_at)`, `cards(player_name)`, `reading_list_items(list_id)`, `book_club_events(club_slug)`, `book_club_events(posted, post_at)` (due sweep), `announcements(type_id)`, `announcements(active, next_post_at)` (due sweep).

**On-disk uploads (not in the DB)**, all under `<webRoot>`:
- `images/raffles/` — raffle prize images
- `images/announcements/` — announcement embed images (reusable across announcements)
- `images/bookclub/` — reading-list item cover uploads (AniList covers stay remote)
- `images/bookclub/events/` — book-club event images (reusable across events)
- `fonts/` — admin-uploaded **fonts** (`.ttf/.otf/.woff/.woff2/.eot`), served by a **separate** vhost (`https://fonts.senpan.cafe`) — cross-origin `@font-face` needs CORS headers (see Deployment)
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
`raffles`, `fonts`, `bookclub` (reading lists/items + events per club), `carrd`
(image-host projects), `announcements` (types + scheduled embeds), `admin`
(sidebar highlight state + per-tab data loads). The root `App.vue` hosts
`<RouterView>` and owns the WebSocket lifecycle; `composables/useWebSocket.ts`
dispatches WS messages into the stores.

**Routing is Vue Router** (history mode, `router/index.ts`) — real linkable URLs,
not store-driven view switching:
- Public: `/`, `/play/:cardId`, `/raffles`, `/raffles/:id`, `/admin/login`.
- Admin: `/admin` (layout, `requiresAdmin`) with child routes grouped into four
  sidebar sections that the route paths mirror — **bingo** (`bingo/{game,cards,
  winners-log,patterns,presets}`), **teahouse** ("Senpan Tea House":
  `teahouse/announcements`, `teahouse/raffles`, and one `teahouse/bookclub/<slug>`
  route per registered club), **atelier** (`atelier/{fonts,carrd}`), and **system**
  (`system/{settings,themes}`). The admin tabs are **child routes** of the
  `AdminView` layout, so the sidebar/topbar persist while the matched child renders
  the active tab. The per-club book-club routes are generated from the `BOOK_CLUBS`
  registry (`lib/constants.ts`) and all render the single generic `BookClubTab.vue`.
  Routes mirror the nav exactly (no legacy redirects) — an unknown `/admin/*` path
  falls through the catch-all to home.
- Every view + admin tab is a lazy `import()` so heavy deps (CodeMirror,
  vuedraggable, markdown-it) split into on-demand chunks. A `router.onError`
  guard recovers from stale lazy-chunk 404s after a redeploy by doing one full
  browser load of the target (sessionStorage-guarded against loops).
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
- Markdown → `markdown-it`, **lazy-loaded** via `useMarkdown()` (`lib/markdown.ts`); the ~100 KB parser is dynamic-imported on first render (`breaks: true` to match the old marked output). Authoring uses `MarkdownEditor.vue`, a lightweight WYSIWYG-ish editor (toolbar + live preview) reused by raffles, book-club items, events, and announcements
- Drag-and-drop → `vuedraggable` (SortableJS) for category reorder + pattern reorder/cross-category move
- Theme CSS editor → CodeMirror 6 via `vue-codemirror`; dark look reproduced in `lib/codemirror.ts` + app.css §30
- Icons → `@fortawesome/*` **Pro** packages via `@fortawesome/fontawesome-svg-core` + `dom.watch()` (keeps the existing `<i class="fa-…">` markup); only the icons used are registered in `lib/fontawesome.ts`
- Card PNG export → `html-to-image` captures the live themed board, then a canvas composites the framed card (`lib/exportCard.ts`)

**Performance / tooling**:
- **Lazy routes**: every view + admin tab is a dynamic `import()` in `router/index.ts`, so heavy deps (CodeMirror, vuedraggable, markdown-it) load only when their route is visited — the player/home payload stays small. `manualChunks` (vite.config) keeps shared vendors cached across route chunks.
- **PWA**: `vite-plugin-pwa` (`registerType: 'autoUpdate'`) emits `sw.js` + `manifest.webmanifest`; the SW precaches the app shell and falls back to `index.html` for SPA routes, with `/api/` and `/images/` denylisted. The deploy `.htaccess` exempts `sw.js`/`registerSW.js`/`*.webmanifest` from the immutable cache so updates land.
- **Route progress + loading UX**: a top progress bar (`RouteProgressBar.vue`, driven by `ui.routeLoading` from the router guards) shows during async navigation/lazy-chunk loads; stores expose per-action loading flags (`joining`, `drawing`, `starting`, …) that drive `LoadingSpinner.vue` + disabled buttons.
- **Global error handler**: `app.config.errorHandler` (`main.ts`) surfaces uncaught errors as a toast.
- **Accessible modals**: `ModalOverlay.vue` traps focus, restores it on close, supports Escape, and sets `role="dialog"`/`aria-modal`.
- **Lint/format**: ESLint flat config (`eslint.config.js`) + Prettier (`.prettierrc.json`); `npm run lint` (autofix) / `npm run lint:check` (no fix, used by CI) / `npm run format`. Bundle treemap via `npm run analyze` → `dist/stats.html`.

**Testing** (Vitest + Vue Test Utils, jsdom):
- Config in `vitest.config.ts` (kept separate from `vite.config.ts` so tests don't load the PWA/visualizer plugins). Tests **import `describe/it/expect/vi` from `vitest`** explicitly (`globals: false`) so they type-check with no extra global-types config.
- Test files are colocated as `src/**/*.test.ts`. Current coverage: `lib/{constants,api,endpoints,exportCard,theme,datetime,freshness}.test.ts`, `stores/{player,ui,raffles}.test.ts`, `components/common/BingoBoard.test.ts`, `components/common/ui/{FormField,DataTable,PaginationBar,ManagerView,ListRow,SubPageHeader,SearchInput}.test.ts`.
- Run: `npm run test` (CI), `npm run test:watch`, `npm run test:coverage` (v8).
- Patterns: mock `fetch` via `vi.stubGlobal` for `api.ts`; mock the `./api` module with `vi.hoisted` + `vi.mock` for `endpoints.ts`; Pinia stores use `setActivePinia(createPinia())`; components use `mount()` from `@vue/test-utils`. Pure helpers tested directly (e.g. `exportCard.ts` exports `parseInlineRuns`/`parseDetailParagraphs` for this).

**Public routes**: `/` · `/play/:cardId` · `/raffles` · `/raffles/:id` · `/admin/login`.
**Admin sections** (`adminSection`, sidebar highlight): `bingo` | `teahouse` | `atelier` | `system`.
**Admin tabs** (`adminTab` / route): `bingo-game` · `bingo-cards` · `bingo-winners-log` · `bingo-patterns` · `bingo-presets` · `teahouse-announcements` · `teahouse-raffles` · `bookclub-<slug>` (one per registered club, e.g. `bookclub-yaoi`, `bookclub-yuri`) · `system-settings` · `system-themes` · `atelier-fonts` · `atelier-carrd`.

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
- **Game tab**: start game (select patterns with category filter + search, or apply a saved **preset**), draw numbers (optional player delay 0–60s; **press `Space`/`Enter` to draw**), live "Live" badge + elapsed-time clock, see called numbers, see winners; click winner ID to verify card with pattern-hit highlighting; frequent winners alert (3+ wins in 12h); end game with winner confirmation modal
- **Cards tab**: generate cards (1–500), view as chips with player name indicators, click to preview board, edit player name/details, delete individual or all
- **Patterns tab** (`PatternsTab.vue`): one manager merging the former Categories/New/Edit tabs — category-grouped collapsible drag-reorder list with search + category filter; "+ New Pattern" (5×5 grid editor, duplicate detection) and "Manage Categories" (a `DataTable` of categories with Edit/Delete; add/edit opens a form with a Title + a Position dropdown — "At the beginning" / "After X" per category, plus "Keep current position" when editing — applied via the bulk-reorder endpoint) as Back sub-pages
- **Game tab pattern picker** (`GameTab.vue`): when starting a new game, patterns render exactly like the Patterns manager — collapsible category groups (non-draggable checkboxes) with search + category filter + select-all-visible — reusing the patterns store's `patternsByCategory` + shared collapse state
- **Presets tab**: CRUD reusable game templates (a named set of win-pattern IDs + pre-written markdown game details); selectable on the Game tab to auto-apply patterns + details when starting a game
- **Winners Log tab**: paginated table of past winners with sorting and per-page controls
- **Senpan Tea House → Raffles tab** (`RafflesTab.vue`): one manager (replacing the former New/Open/Closed tabs) — **Current Raffles** (every non-closed raffle) as image cards with a corner status icon (calendar-clock when it opens later, red calendar-circle-exclamation when its open window has passed), then a searchable + paginated **Closed Raffles** table (title, winner, open period, and the gil collected from paid entries — `Raffle.winner_name`/`paid_total`, admin-only aggregates joined in `listRafflesAdmin`) with a **Copy** action that seeds a new raffle from a past one (`copyRaffleForm`). Detail (winner pick/verify, **manually add an entry — optionally paid**, toggle paid, delete; read-only when closed) and the create/edit form (`RaffleFormTab.vue`, emits `saved`/`cancel`) open as Back sub-pages. Weighted random winner pick; delete raffles.
- **Senpan Tea House → Announcements tab** (`AnnouncementsTab.vue`): manage **announcement types** (a named Discord channel webhook) and **announcements** authored as Discord embeds (title, markdown details, accent colour, optional event window, image upload/reuse, and **up to 5 Discord link buttons** — label + optional emoji + URL — rendered as an action row beneath the embed; sanitized server-side and stored as JSON in `announcements.buttons`). Post manually (**send now**) or on a schedule (once / daily / weekly / monthly, anchored to an IANA timezone so times survive DST); **skip next** occurrence; the background scheduler posts due items. Client-side search + type filter. (Link buttons require a webhook whose target supports message components.)
- **Senpan Tea House → Book Club tabs** (`BookClubTab.vue`, one route per registered club): manage **reading lists** + their items — add items manually or pull them from **AniList** (search/by-id proxy that prefills title/summary/cover/format/genres/chapters/source), drag-reorder, upload covers; **publish** a list to the club's Discord channel (one embed per item). Also schedule **book-club events** (meeting title, start time + length, location, markdown details, image) that post as a Discord embed to the club's events channel — manually (**post now**) or automatically at the post time via the scheduler.
- **System → Settings tab**: app title, bingo join prompt, default draw delay, frequent-winner threshold/window, the **header/board font** picker (combo box with optgroups for uploaded fonts + Google Fonts, optional Google Fonts API key, live preview), the AniList API URL, and the per-club Discord webhook URLs (reading-list + events channels, treated as secret)
- **System → Themes tab**: CRUD custom CSS themes with CodeMirror 6 editor, activate/deactivate live; a `ThemeColorPickerTool.vue` helps author `:root` token overrides
- **Atelier → Font Upload tab** (`FontsTab.vue`): upload one or more font files (`.ttf/.otf/.woff/.woff2/.eot`) to `<webRoot>/fonts/`, rename/delete them, copy each file's public URL, and a **live preview** (type any text, pick a font) — all with the same metric clamping used app-wide so oversized fonts render sensibly. Uploaded fonts become selectable as the header/board font for everyone, not just the admin preview.
- **Atelier → Carrd Upload tab** (`CarrdUploadTab.vue`): manage image-hosting **projects** (folders under `<webRoot>/carrd`, each with a human-readable title) and nested sub-directories; upload images plus `.mp3`/`.mp4` (same-name **overwrites** so external Carrd sites pick up new versions), copy public URLs, delete files/dirs/projects. Served cross-origin from the carrd vhost.

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
├── images/          ← deploy/images/    (PERSISTENT: logo/favicon/banner + raffles/, announcements/, bookclub/ (+ events/) uploads)
├── fonts/           ← (PERSISTENT: admin-uploaded font files; served by the fonts.senpan.cafe vhost)
├── carrd/           ← (PERSISTENT: Carrd image-host projects; served by the carrd.senpan.cafe vhost)
└── dist/            ← frontend/dist/    (built SPA; replaced each deploy)
```

- Run the Go server with `-webroot <DocumentRoot>`; uploads are written under
  `<DocumentRoot>/images/{raffles,announcements,bookclub,bookclub/events}/`,
  `<DocumentRoot>/fonts/`, and `<DocumentRoot>/carrd/<project>/`. Image/font/carrd
  URLs are returned absolute (built from the request scheme+host).
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
- **Carrd host (CORS)**: Carrd projects are served cross-origin from
  `https://carrd.senpan.cafe` and embedded by external Carrd sites, so reads need
  CORS. Deploy `deploy/carrd.htaccess` to `<webRoot>/carrd/.htaccess` (adds
  `Access-Control-Allow-Origin`, hides the `.carrd.json` sidecars, disables
  listings). See the **Carrd image host (CORS)** section of `deploy/README.md`.
- **Schedulers**: the server runs two background goroutines that post due
  book-club events and announcements to Discord webhooks — no cron needed; they
  start with the process and stop on graceful shutdown.

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
- **Schema versioning**: `PRAGMA user_version` in SQLite tracks migration state; `schemaVersion` constant in `store/migrate.go` controls the target (currently 20). Migrations are idempotent (`hasColumn` guards) and run incrementally only when behind.
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
- **Theme CSS editor**: CodeMirror 6 via `vue-codemirror`, bound with `v-model` to the edited theme's `css_content`; the dark look/syntax palette lives in `frontend/src/lib/codemirror.ts` (theme + HighlightStyle) plus structural rules in app.css §30.
- **TS↔Go type sync**: `frontend/src/types/api.generated.ts` is generated from `internal/model` by tygo (`src/tygo.yaml`); it is **gitignored** — regenerate with `npm run gen:types` (needs Go) after a fresh clone or model change. Never edit the generated file.
- **Typed endpoint layer**: stores never call `api<T>('path')` directly — they call `endpoints.*` (`frontend/src/lib/endpoints.ts`), which wraps every backend path in a typed function. Add new endpoints there so paths/bodies/response types live in one place.
- **Global 401 handling**: `api.ts` invokes a registered handler (set in `main.ts`) on any non-auth 401 → redirect to `/admin/login` + "session expired" toast. Auth endpoints pass `skipAuthRedirect` so a bad-password login doesn't trigger it.
- **Themed confirm dialog**: use `await ui.confirm(message, opts)` (renders `ConfirmModal.vue`) instead of the native `window.confirm`.
- **Theme fidelity**: new UI must reuse existing `app.css` class names + theme tokens (CSS custom properties in `:root`) so user-authored custom CSS themes keep working — never hard-code colors/fonts.
- **Uploaded fonts**: `applyUploadedFonts()` (`lib/theme.ts`) writes one `<style id="uploaded-fonts">` with an `@font-face` per file from `FONT_BASE_URL` (`stores/fonts.ts`), so a font is registered app-wide (board/header + every preview), not per-component. It then measures each loaded font via the Canvas TextMetrics API and, for fonts with oversized vertical metrics, rewrites the rule with `ascent-override`/`descent-override`/`line-gap-override` to clamp the box (`clampFontMetrics()` — pure + unit-tested). The Settings preview and the Font Upload live preview both rely on these shared rules, so previews always match what players see.
- **Discord embeds (shared plumbing)**: every webhook-posting feature (announcements, reading-list items, book-club events) builds its embed with the fluent `newEmbed()…build()` builder in `server/embeds.go` and posts via `postDiscordEmbed()`. The builder auto-truncates to Discord's per-field limits, skips empty fields, and caps at 25 fields, so callers pass raw content. New embed shapes add a builder chain, not new transport. Times are emitted as Discord `<t:unix:…>` tokens so each viewer sees their own zone.
- **Timezone-anchored scheduling**: events + announcements store the admin's wall-clock input (`*_local`) **and** its IANA `timezone`, plus the computed absolute UTC instant. Recurrence math (`nextAnnouncementOccurrence`, `nthWeekdayOfMonth`) runs in that zone so schedules survive DST. `server/tzdata.go` blank-imports `time/tzdata` so `time.LoadLocation` works on hosts without system zoneinfo (e.g. Windows).
- **Background schedulers**: `RunEventScheduler` + `RunAnnouncementScheduler` are launched as goroutines in `main.go` against a shutdown-cancelled context, tick every 30s, sweep once on startup, and are crash-tolerant (unset webhook → left pending; failed post → retried; `skip_next`/recurrence advance the cursor). The due-row queries are index-backed (`(posted, post_at)`, `(active, next_post_at)`). Both share the ticker/sweep loop in `server/scheduler.go` (`runScheduler(ctx, interval, sweep)`) — each `Run…Scheduler` is just a one-line wrapper supplying its interval + sweep func.
- **Shared upload helpers** (`server/uploads.go`): the single-image upload flow (5 MB cap, `image` field, allowed-ext check, write, return `{"url"}`) is `s.saveSingleImageUpload(w, r, relDir, prefix)`, and the "pick an existing image" listing is `s.listUploadedImageURLs(r, relDir)` — both used by the announcement, book-club cover, and event endpoints. `relDir` (e.g. `announcementImageRelDir`) is a forward-slash path that doubles as the on-disk location and URL path. `saveMultipartFile` streams one multipart part to a path (filename-agnostic; reused by fonts + Carrd). Add a new image upload area by adding a rel-dir const and calling these — don't re-inline the flow.
- **AniList proxy**: `GET /api/bookclub/lookup?q=|id=` proxies the public AniList GraphQL API server-side (avoids browser CORS + centralizes field mapping); the endpoint is configurable via the `anilist_api_url` setting. `anilistToItem` maps a Media node to a reading-list item suggestion.
- **Per-club registry**: book clubs live in one registry — `bookClubs` (`server/bookclubs.go`) on the backend and `BOOK_CLUBS` (`lib/constants.ts`) on the frontend. Adding an entry wires up its route, sidebar button, comments label, and the two secret webhook settings automatically. The per-club webhook setting keys (`discord_webhook_url_<slug>`, `discord_events_webhook_url_<slug>`) are registered in `bookclubs.go`'s `init()` and must match `clubWebhookKey`/`clubEventsWebhookKey` on the frontend.
- **Secret settings**: `GET /api/settings` is public, so keys in `secretSettings` (the per-club Discord webhooks) are blanked out for non-admin callers and only returned to an authenticated admin who needs them to edit.
- **datetime conversion**: `lib/datetime.ts` converts between stored UTC (RFC-3339) and the LOCAL wall-clock value an `<input type="datetime-local">` expects (raffle windows); event/announcement forms instead send the raw wall-clock + an explicit IANA zone the backend resolves.
- **Snappy admin navigation (load-gate)**: re-entering an admin tab would otherwise re-run its data load every time. `admin.setTabFromRoute()` routes each tab's load through `loadFresh(key, …)` (backed by `lib/freshness.ts`), which skips the refetch when that dataset loaded within a 30s TTL — so tab switches are instant instead of re-spinning. The gate lives at the **navigation layer only**: store loaders still always fetch, so post-mutation refreshes (which call the loaders directly) show edits immediately, and live game/cards/patterns stay current over WebSocket. `bookclub.openClub()` uses its own per-club freshness so re-entering the same club keeps the open list/sub-view while switching clubs refetches.
- **Admin UI primitives** (`components/common/ui/`): admin forms and tables are built from a small set of presentational components so every screen is consistent by construction — don't hand-roll `.field`/label/table markup. Use `AdminPanel` (the `.admin-panel` card; pass `title`+`icon` or supply a custom header slot), `FormField` (label + control slot + optional `help`/`#help`; controls auto-stretch full-width, so no `.field-input-full` on direct children), `FormRow` (equal-width side-by-side fields — for unequal widths use a plain `.flex-row` and pass `style="flex:…"` to each `FormField`, which falls through to its root), `FormActions` (button row; `align="start|end|between"`), `DataTable` (generic sortable table — `columns`/`rows`/`row-key`, `#cell-<key>` slots, `@sort`, `#empty` slot), `PaginationBar`, and `EmptyState`. The CSS for these lives in app.css §3 (the "FORM & TABLE PRIMITIVES" and "MANAGER LAYOUT" blocks, in the Primitives band) and uses theme tokens so custom themes restyle them.
- **Manager model** (`components/common/ui/`): every admin tab that manages a collection follows one shape. `ManagerView` = the list page shell (`.admin-panel` + `title`/`icon` header, `#actions` slot top-right for buttons like "Manage Categories" / "+ New", optional `#toolbar` slot for `SearchInput` + filter selects, default slot = the list, `#pagination` slot). `ListRow` = one item (`#media` left, body default, `#actions` pinned **far right**); stack rows in a `.list-rows` container. `SubPageHeader` = a sub-page header (title + Back, emits `back`). Convention: the tab holds a `screen` ref (`'list' | 'new' | …`); `#actions` buttons switch `screen`; each sub-screen opens with `SubPageHeader @back`. Reference implementations: `PatternsTab` (list/new/categories), `AnnouncementsTab`, `PresetsTab`, `Open/ClosedRafflesTab`, `BookClubTab`. Manage Cards / Fonts / Carrd are intentionally bespoke (chip grid / table / image manager) and not on the model.
- **Theme tokens** (`:root` in app.css §1): role-descriptive CSS custom properties, all overridable by saved themes + the theme editor. Backgrounds: `--page-bg`, `--panel-bg` (cards/panels), `--panel-raised-bg` (nested/row/chip surface), `--control-border` (control & divider outline — readable on *both* panel surfaces), `--input-bg` (form-control fill). Accents: `--accent` / `--accent-hover`, `--accent-2` / `--accent-2-hover`, `--highlight` (called numbers, headings, gold trim). Text: `--text`, `--text-muted`, `--text-on-accent`, `--text-on-fill`. Status: `--success`, `--danger`, `--warning` (skip badges/alerts; use `color-mix(in srgb, var(--warning) N%, transparent)` for tints). Board: `--board-cell-bg`, `--board-cell-hover-bg`, `--board-free-bg`, `--board-gradient-start/end`. Effects: `--modal-overlay`, `--shadow`, `--highlight-glow`. Non-colour: `--radius`, `--header-font`. Use `--control-border` for outlines and `--input-bg` for control fills (the old overloaded `--surface2`-as-border was invisible on nested boxes), so no one-off contrast overrides are needed inside a `--panel-raised-bg` box.
- **Class rename map** (structural classes consolidated 2026-06): `.entries-table`/`.winners-log-table` → `.data-table`, `.msg-block` → `.empty-state`, `.btns` action rows → `.form-actions`; `.field-input-full` is retained only for controls nested inside a flex row (not a direct `.field` child).
- **Token rename (2026-06)**: the theme tokens were renamed to role-descriptive names (`--surface`→`--panel-bg`, `--surface2`→`--panel-raised-bg`, `--border`→`--control-border`, `--field-bg`→`--input-bg`, `--primary`→`--accent`, `--secondary`→`--accent-2`, `--gold`→`--highlight`, `--text-dim`→`--text-muted`, `--text-on-primary`→`--text-on-accent`, `--text-on-toast`→`--text-on-fill`, `--bg`→`--page-bg`, `--board-cell`→`--board-cell-bg`, `--overlay`→`--modal-overlay`, `--shadow-color`→`--shadow`, `--glow-color`→`--highlight-glow`; `--warning` added). **Saved themes must use the new names** — the live DB themes were migrated in `CURRENT_THEMES.css` (root of repo) for re-import; a theme still using old token names silently falls back to the app.css defaults.
- **CSS direction — objects, not pages**: app.css is migrating away from page/feature sections (`HOME VIEW`, `RAFFLES`, `ADMIN GAME`, …) toward a small vocabulary of reusable **objects** composed across pages — the `components/common/ui/` primitives are the first wave. When styling new UI prefer, in order: (1) an existing primitive/object class, (2) a theme token on existing markup, (3) a new shared object class in the Primitives band — and only as a last resort a page-scoped rule. **Don't** re-declare control chrome inline: form controls already inherit the global `input,select`/`button` rules (`2px solid var(--control-border)`, `--input-bg`), so an inline `style="border:1px solid var(--panel-raised-bg)"` on a `<select>` is both redundant and off-token. **Don't** add a per-feature one-off that duplicates an existing object. Object classes are still themeable (the runtime theme editor targets semantic classes), which is why a utility-first framework (Tailwind) is *not* the direction. Extracted objects so far: the `ui/` form/table/manager primitives; **`.card`** (shared "bordered surface tile" chrome — `home-card`/`raffle-card`/`saved-pattern` only set their own size/radius/hover; new tiles can use `.card` directly); **`.badge`** (uppercase pill chrome — `raffle-badge`/`live-badge`/`conn-badge`/`ann-badge`/`style-active-badge` only set colours/margins; `nav-count` is intentionally *not* a badge, it's inline count text); **`PatternPicker.vue`** (`components/common/ui/`, `v-model` = selected ids) which both GameTab and the Preset editor use, so the win-pattern picker can never desync; **`.section-heading`** (gold sub-heading; `.raffle-section-heading` is its legacy alias); the **toolbar** object (`.flex-toolbar`, with `.manager-toolbar` = same + bottom spacing); the **`.color-picker`** skin (wrap the `@ckpack/vue-color` Chrome picker in `.color-picker` — global descendant rules re-map it to theme tokens, beating the lib's own styles; shared by StampColorPicker + ThemeColorPickerTool); the **media** objects `.img-picker` / `.img-thumb` (+`img`/`:hover`/`.active`) / `.media-empty` (placeholder); **`ImageField.vue`** (`ui/`, `v-model` = image URL + `images`/`uploading` props, emits `upload`) — the "upload or reuse an image" field shared by the announcement + book-club event forms; **`ColorPicker.vue`** (`ui/`, `:value` + `@change` → `{rgba,hex}`) wrapping the lazy `@ckpack/vue-color` Chrome picker with the `.color-picker` skin (used by StampColorPicker + ThemeColorPickerTool); and **`EmojiPickerModal.vue`** (`components/common/`, render with `v-if`, emits `select`(char)/`close`, optional `allow-clear`) wrapping the lazy `vue3-emoji-picker` in a themed modal (used by the player stamp shape picker + the announcement Discord-button emoji field). Note: a global object can lose to a component's *scoped* class on the same element (scoped `[data-v]` adds specificity) — e.g. a size class setting `display` will override `.media-empty`'s flex, so size classes that share an element with an object must not redeclare the object's properties. Carrd's media manager + Fonts' preview stay intentionally bespoke. Still page-bound: home/raffle/board section CSS that doesn't reuse a primitive.
- **Carrd path safety**: every Carrd folder/sub-path/filename received from the client is validated (`validCarrdFolder`, `cleanCarrdRelPath`, `safeCarrdFileName`) and the resolved absolute path is checked to stay within the project dir — guarding against traversal. Uploads reuse `saveMultipartFile` (it is filename-agnostic) and same-name uploads intentionally overwrite.

## Key files to inspect first

1. `src/internal/server/server.go` — Server struct, route registration, middleware, JSON/auth helpers, broadcast helpers
2. `src/internal/store/` — all database access, one file per domain (`cards.go`, `patterns.go`, `raffles.go`, …); `store.go` holds the `Store` struct, `New()`/`Close()`, pragmas, and shared helpers
3. `src/internal/bingo/game.go` — Core game logic: start, draw + winner compute + cache, state reads, in-memory caching
4. `src/internal/bingo/card.go` — Card/board generation algorithm with column-range constraints
5. `src/internal/model/model.go` — All domain types (Card, Pattern, GamePreset, BingoGameState, BingoDrawnNumber, Raffle, ReadingList(+Item/Source), BookClubEvent, AnnouncementType, Announcement, WinnersLogEntry, etc.)
   - `src/internal/server/embeds.go` — shared Discord embed builder + transport (read before touching any webhook feature)
6. `frontend/src/router/index.ts` — route map, lazy route components, admin auth guard + tab sync
7. `frontend/src/App.vue` + `frontend/src/composables/useWebSocket.ts` — root shell (`<RouterView>`), WebSocket message dispatch
8. `frontend/src/lib/endpoints.ts` — the typed surface over every backend path (what stores call)
9. `frontend/src/stores/*.ts` — all client state + actions (mirror of the old app.js data/methods)
10. `frontend/src/assets/app.css` — all styles (the look to preserve); imported in `main.ts` (content-hashed by Vite); §30 covers the CM6 editor

## Extending the project

- **New API endpoint**: add a handler method on `*Server` in `internal/server/`, register the route in `routes()`, then add a typed wrapper in `frontend/src/lib/endpoints.ts` for the frontend to call.
- **New domain type**: add to `internal/model/model.go`, then run `npm run gen:types` to refresh the TS types.
- **New store method**: add to the matching domain file in `internal/store/` (`raffles.go`, `patterns.go`, …; new domain → new file), returning typed structs.
- **New migration**: bump `schemaVersion` in `store/migrate.go`, add an `if version < N` block in `ensureSchema()`.
- **New admin tab**: add the tab id to the `AdminTab` union in `frontend/src/stores/admin.ts` (mapping its prefix to a section in `setTabFromRoute()` + any per-tab data load — wrap the load in `loadFresh('<key>', …)` so revisits stay snappy), add a child route with `meta.tab` in `router/index.ts`, add a sidebar link in `components/admin/AdminSidebar.vue`, and create a `components/admin/<Tab>.vue` referenced by the route's lazy `import()`. For a "manage items" tab use the **manager model** (`ManagerView` + `ListRow` + `SubPageHeader` + `SearchInput`, with a `screen` ref for sub-pages — see `PatternsTab.vue`); otherwise wrap content in `<AdminPanel>`. Either way build forms/tables from the `components/common/ui/` primitives (`FormField`, `FormRow`, `FormActions`, `DataTable`, `PaginationBar`, `EmptyState`) rather than hand-rolled markup.
- **New route/view**: add a record to `routes` in `router/index.ts` with a lazy `component: () => import(...)`; mark `meta.requiresAdmin` if it needs auth.
- **New WebSocket message type**: add a case in `composables/useWebSocket.ts` (and the `WsMessage` union in `types/api.ts`), send from server via `hub.Broadcast()` / `hub.BroadcastToPlayers()` / `hub.BroadcastToAdmins()`.
- **New Discord-embed feature**: build the embed with `newEmbed()…build()` in `server/embeds.go` (extend the builder if a new shape is needed) and post with `postDiscordEmbed(webhookURL, embed)`; never write transport code per feature.
- **New book club**: add an entry to `bookClubs` (`server/bookclubs.go`) **and** `BOOK_CLUBS` (`lib/constants.ts`) — keep the slug identical. The route, sidebar button, comments label, and both secret webhook settings are derived automatically.
- **New setting**: add the key to `settingsKeys` (and a fallback in `settingsDefaults`) in `server/settings.go`; mark it secret by adding to `secretSettings` if it shouldn't be public; surface it in `SettingsTab.vue` + the `AppSettings` type.
- **New test**: colocate a `*.test.ts` next to the code; import `{ describe, it, expect }` from `vitest`; run `npm run test`. Add it to the same gate CI runs. (Go: colocate `*_test.go` and run `go test ./...`.)
