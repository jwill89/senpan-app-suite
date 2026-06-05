# AGENTS.md

Guidance for AI coding agents working in this codebase.

## Quick orientation

Go + SQLite backend serving a Vue 3 (CDN, no build step) single-page frontend.
The backend binary is built from `src/` and serves the API on `:8080` by default.

```
├── index.html                        ← Vue 3 SPA template (all views in one file)
├── assets/
│   ├── css/app.css                   ← All styles (dark theme, board, admin, modals)
│   └── js/app.js                     ← Vue 3 app (data, methods, WebSocket)
├── src/                              ← Go backend
│   ├── main.go                       ← Entry point: flags, DB init, server start
│   ├── go.mod / go.sum               ← Module deps (alexedwards/scs, coder/websocket, ncruces sqlite)
│   └── internal/
│       ├── model/model.go            ← Domain types (Card, Pattern, Game, GameState, Raffle, etc.)
│       ├── store/
│       │   ├── store.go              ← Store struct wrapping *sql.DB; all typed CRUD methods
│       │   └── migrate.go            ← Schema versioning + migrations (PRAGMA user_version)
│       ├── bingo/
│       │   ├── card.go               ← Card/board generation, ID generation, LetterForNumber
│       │   └── game.go               ← GameService (start, draw, end, state, winner matching, caching)
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

**Schema versioning**: `store/migrate.go` uses `PRAGMA user_version` to track schema version (currently **9**).
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

## Frontend (Vue 3 SPA)

**Views** (`view` state): `home` → `admin-login` → `admin` | `player` | `raffles` → `raffle-detail`.
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
- **Patterns tab**: create (5×5 grid editor with duplicate detection), organize into categories, rename inline (click name or pencil), reorder via drag-and-drop with visual placeholder, delete; category CRUD with drag-and-drop reorder
- **Winners Log tab**: paginated table of past winners with sorting and per-page controls
- **Raffles section**: create/edit raffles (title, description, rules as markdown, cost per entry, max entries, signup instructions, availability window, prize image upload); manage entries (view, toggle paid status, delete); weighted random winner pick; close/reopen raffles; delete raffles
- **Themes section**: CRUD custom CSS themes with CodeMirror editor, activate/deactivate live

**Key UI patterns**:
- `v-cloak` prevents flash of unstyled template on load
- Optimistic updates: pattern/category reorder swaps locally then persists in background
- Toast notifications for success/error feedback
- Admin login separates auth failure from data-loading failure to prevent false login rejections
- Winner toast only fires on new winners (compares count before/after draw)
- WebSocket reconnect with exponential back-off (1s → 2s → 4s → 8s → 16s, max 5 attempts)
- Drag-and-drop with empty-box placeholder (pointer-events: none) for patterns and categories

## Developer commands

```powershell
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
- **Schema versioning**: `PRAGMA user_version` in SQLite tracks migration state; `schemaVersion` constant in `store/migrate.go` controls the target (currently 9).
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
- **FontAwesome 7**: loaded via a FontAwesome Kit script tag (requires a free FA account); kit ID is configured in `index.html`.
- **CodeMirror debounce**: the theme editor debounces CodeMirror change events (300ms) to prevent Vue reactivity thrashing on every keystroke; flushed before save.

## Key files to inspect first

1. `src/internal/server/server.go` — Server struct, route registration, middleware, JSON/auth helpers, broadcast helpers
2. `src/internal/store/store.go` — All database access methods with typed returns
3. `src/internal/bingo/game.go` — Core game logic: start, draw + winner compute + cache, state reads, in-memory caching
4. `src/internal/bingo/card.go` — Card/board generation algorithm with column-range constraints
5. `src/internal/model/model.go` — All domain types (Card, Pattern, Game, GameState, DrawnNumber, Raffle, WinnersLogEntry, etc.)
6. `assets/js/app.js` — Vue app: data model, all methods, WebSocket handling, optimistic updates

## Extending the project

- **New API endpoint**: add a handler method on `*Server` in `internal/server/`, register the route in `routes()`.
- **New domain type**: add to `internal/model/model.go`.
- **New store method**: add to `internal/store/store.go`, returning typed structs.
- **New migration**: bump `schemaVersion` in `store/migrate.go`, add an `if version < N` block in `ensureSchema()`.
- **New admin tab**: add tab ID to `adminNav()` in `app.js`, add template section in `index.html`, wrap in `.admin-panel` for consistent styling.
- **New WebSocket message type**: add case in `ws.onmessage` handler in `app.js`, send from server via `hub.Broadcast()` / `hub.BroadcastToPlayers()` / `hub.BroadcastToAdmins()`.
