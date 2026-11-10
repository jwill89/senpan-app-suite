# AGENTS.md

The deep guide to this repo, for coding agents and developers alike. Read it before any change.

The rules every project follows are in [`shared/standards/README.md`](shared/standards/README.md) and the topic files it indexes. This file holds only what is specific to Senpan: its layout, commands, domain rules, recipes and known traps. Where it says nothing, the shared rules win; where the code still does something they rule out, [Where this project departs from the standards](#where-this-project-departs-from-the-standards) says what and when it moves.

## Quick orientation

Go + SQLite backend serving a Vue 3 + TypeScript single-page frontend built with
Vite. One binary, built from `backend/`, serves all of it on `:8080`: the API, its live
event streams (server-sent events) and, since 4.0.0, the SPA, whose build it embeds (`backend/internal/web`).
Apache proxies to it and serves only the web root's own folders (images, Carrd sites,
the plugin repository). The app has one version (`frontend/package.json` and
`internal/version`); the plugin keeps its own.

Beyond bingo, the app is a small "suite" for a Discord community: **raffles**,
**book clubs** (reading lists pulled from AniList),
and **announcements** - these post **Discord embeds** (manually or on a
schedule via background goroutines). It has since grown a **Senpan Tea House** side
(partner **affiliates** and bookable **tea rooms**) and a **festival** side (the
**Garapon** lottery drum, the **Stamp Rally** and the interactive **Festival Map**),
plus player-submitted **custom card
requests** and an in-game **Yoever** reaction. It also hosts images for external
**Carrd** sites and admin-uploaded **fonts**. Admin login is per-user with optional
passwordless **passkeys** and an optional Cloudflare **Turnstile** bot check.

A third surface - the **FFXIV Dalamud plugin** (`plugins/SenpanCompanion/`,
C#/.NET 10 + ImGui, published as _Senpan Admin Companion_) - is a **second admin
UI** over the same server. FFXIV gives plugins no client<->client networking, so the
Go server stays the single source of truth: every plugin action goes through the
same REST API and live event stream, so its changes still reach the website. See **FFXIV plugin
(Dalamud)** below.

**At a glance:** Vue 3 SFCs + TypeScript (strict) - Pinia (setup stores) - Vue
Router (history mode, lazy routes) - Go 1.26+ stdlib HTTP - SQLite (WAL) -
server-sent events - per-user accounts (argon2id) + WebAuthn passkeys + optional
Cloudflare Turnstile + per-page permissions - embedded tzdata + background
schedulers (Discord embeds) - Vitest + Vue Test Utils - C#/.NET 10 Dalamud plugin
(ImGui) - GitHub Actions CI on shared-tooling's reusable workflows (the `shared/`
submodule, see **Shared tooling** below).

**Auth model (read this first):** the admin area is now gated by **per-user
accounts**, not a single shared password. Accounts log in with username +
password (argon2id hash, `internal/auth`) **or a passwordless WebAuthn passkey**,
are created **inactive** via a hidden registration page, and an admin activates
them and grants **per-page permissions** (one key per admin page). Admins
implicitly hold every permission. A password change/reset invalidates the
account's other sessions (a `password_epoch` checked each request). A bootstrap
`admin`/`admin` account is seeded by migration v22 and **must be rotated
immediately**. See **Authentication & authorization** below.

```text
+-- frontend/                         <- Vue 3 + TypeScript SPA (Vite)
|   +-- index.html                    <- Vite entry (mounts #app)
|   +-- package.json                  <- deps: vue, pinia, vue-router, vue-draggable-plus (sortablejs), markdown-it, @milkdown/crepe (Milkdown "Crepe" WYSIWYG authoring), @awesome.me/kit-... (FA Pro icons) + @fortawesome/fontawesome-svg-core + @fortawesome/vue-fontawesome (the <font-awesome-icon> component), @ckpack/vue-color, vue3-emoji-picker, html-to-image
|   +-- .npmrc                        <- Font Awesome registry scopes + install-links=true (tracked; holds no token - that lives in ~/.npmrc)
|   +-- .nvmrc                        <- the Node major CI installs (26)
|   +-- eslint.config.js              <- ESLint: @jwill89/lint-web (shared-tooling's lint/web) + Senpan's own rules; stylelint.config.mjs + prettier.config.js extend it the same way
|   +-- vite.config.ts                <- build -> ../backend/internal/web/dist (the binary embeds it), dev proxy for /api, manualChunks, strip the build's images/
|   +-- tsconfig*.json                <- TS project refs (app + node), extending @jwill89/lint-web's tsconfigs (strict, noUncheckedIndexedAccess)
|   +-- public/                       <- copied verbatim into the build
|   |   \-- images/                   <- logo, favicon, share banner (dev only; stripped from the build - served from the doc root in prod)
|   +-- vitest.config.ts              <- Vitest config: the shared vitestDefaults (jsdom, v8 coverage with its floor) + the @ alias; separate from vite.config so tests skip PWA/visualizer
|   \-- src/
|       +-- main.ts                   <- createApp + Pinia + Router + registers the global <font-awesome-icon> component (imports assets/app.css + lib/fontawesome; registers 401 handler)
|       +-- App.vue                   <- root shell: <RouterView>, the live stream's lifecycle, toast, route progress bar
|       +-- assets/app.css            <- stylesheet index: @imports assets/styles/*.css (tokens, base, utilities, components, player, admin, responsive); imported in main.ts so Vite content-hashes it
|       +-- router/index.ts           <- Vue Router (history mode): route map, lazy route components, admin auth guard
|       +-- api/                      <- the API on huma: openapi.json (the document, `go run . openapi`) + schema.d.ts (openapi-typescript) - both written by `task gen`, committed, DO NOT EDIT - and client.ts (the openapi-fetch client, `unwrap`, and the schema's types by name)
|       +-- lib/                      <- framework-agnostic helpers
|       |   +-- api.ts                <- the 401 handler, `errorMessage`/`refusedFiles` (problem details' `detail` and a refused upload's per-file reasons) and `apiUpload`, the XHR upload helper with progress, pointed at `/api/v1`
|       |   +-- feed.ts               <- Feed: one server-sent event stream (EventSource) with back-off reopening 1-16 s, silence detection, and a re-read on every open; the event types come from the generated document
|       |   +-- markdown.ts           <- markdown-it renderer (lazy-loaded; replaces CDN marked.js)
|       |   +-- fontawesome.ts        <- SVG-core icon library (the used Pro-kit icons added once; rendered via the global <font-awesome-icon> component - no dom.watch)
|       |   +-- theme-tokens.ts       <- theme design-token metadata + token->:root CSS (tokensToCss)
|       |   +-- theme.ts              <- header-font + custom-CSS <head> injection; registers uploaded-font @font-face rules (metric-clamped to tame oversized vertical metrics)
|       |   +-- assets.ts             <- resolves server image paths (images/...) to root-absolute URLs
|       |   +-- exportCard.ts         <- player card -> framed PNG export (html-to-image capture + canvas composite)
|       |   +-- sound.ts              <- opt-in draw chime (Web Audio synth) + haptics for the player view
|       |   +-- datetime.ts           <- UTC <-> <input datetime-local> (wall-clock) conversion; IANA tz helpers for announcements
|       |   +-- freshness.ts          <- createFreshness(): keyed time-gate so re-entered admin tabs skip redundant refetches (snappy navigation)
|       |   \-- constants.ts          <- stamp shapes/colors, column helpers, fallback fonts, BOOK_CLUBS registry, DEFAULT_SETTINGS
|       +-- types/
|       |   \-- api.ts                <- the form models and UI enums the SPA alone uses (every API type comes from api/)
|       +-- stores/                   <- Pinia stores (ui, app, auth, users, player, game, yoever, cards, cardRequests, patterns, presets, styles, raffles, affiliates, teaRooms, garapons, stampRallies, festivalMaps, festivalDashboard, ema, images, fonts, bookclub, carrd, announcements, logs, admin)
|       +-- composables/
|       |   +-- useLiveEvents.ts      <- opens the board's or the staff's stream and hands each event to the stores
|       |   \-- usePwaInstall.ts      <- beforeinstallprompt capture + install/standalone state for the PWA "Install" affordance
|       +-- components/
|       |   +-- common/               <- BingoBoard, CalledNumbers, PatternMini, ModalOverlay, ConfirmModal, ToastNotification, LoadingSpinner, RouteProgressBar, MarkdownEditor (WYSIWYG), AppFooter, CornerFlourish
|       |   |   \-- ui/                <- admin UI primitives (presentational, render stable classes). Forms/tables: AdminPanel, FormField, FormRow, FormActions, DataTable, DataTableToolbar, PaginationBar, EmptyState. Manager model: ManagerView (list page shell), ListRow (item row, actions far-right), SubPageHeader (Back sub-page header), SearchInput. Shared widgets: PatternPicker (v-model selected pattern ids - search + category filter + Select-All + collapse-all over a grouped collapsible checkbox grid; used by GameTab + the Preset editor), ImagePicker (pick from the central image library, with upload for `system-images` holders; the admin forms), WorldPicker, ColorPicker (lazy vue-color Chrome wrapper + .color-picker skin; player stamp-colour modal). Every admin "manage items" tab routes through these for one consistent structure
|       |   +-- player/               <- Stamp{Shape,Color,Opacity} pickers, WinPatternsPanel, GaraponWheel; StampCardPanel + GaraponPanel (the card/drum bodies their own pages AND the festival dashboard render) + FestivalRaffleCard
|       |   \-- admin/                <- AdminSidebar + one component per tab + modals (CardPreview, EndGame, WinnerVerify, HalftimePrompt) + ThemeTokenEditor. Tabs: Game, Cards, WinnersLog, Patterns (one manager unifying the patterns list + New Pattern / Manage Categories sub-pages), Presets, RaffleForm, Raffles, Announcements, Affiliates + AffiliateForm, BookClub (one generic tab serves every club), Garapon + GaraponForm, StampRallies + StampRallyForm, FestivalMaps + FestivalMapForm, Settings, Themes, Images, Users (admin-only account+permission manager), Logs (admin-only live server-log viewer - typed/colored columns, filters, live tail, DEBUG toggle), Fonts, CarrdUpload; PlacementEditor (shared %-based image placement widget for Garapon/Stamp Rally forms) + MapStallEditor (the same for Festival Map stalls - both share the drag geometry in `composables/usePlacementDrag.ts` and the `.placement-*`/`.map-*` styles in `assets/styles/mapeditor.css`)
|       +-- views/                    <- HomeView, PlayerView, RafflesView, RaffleDetailView, GaraponView (public token-gated draw), StampCardView (public token-gated stamp card), FestivalDashboardView (one visitor's whole festival), AdminLoginView, RegisterView (hidden), NoAccessView (active account, no granted pages), AdminView
|       \-- **/*.test.ts              <- Vitest unit/component tests, colocated next to the code they cover
+-- .github/workflows/ci.yml          <- CI: shared-tooling's reusable workflows - frontend (web.yml), backend (go.yml), plugin (dotnet.yml), scripts (pwsh.yml), docs (docs.yml) - + release (main: auto tag + GitHub Release for the app and the plugin on a version bump); the backend job builds on the frontend job's build, which the binary embeds
+-- .github/scripts/release.sh        <- the release job's script: reads the app's and the plugin's versions, tags v<version> / Plugin-v<version> + publishes a Release with that version's CHANGELOG.md entry
+-- .github/dependabot.yml            <- weekly updates for npm, Go modules, NuGet and Actions (shared/ is left out: a release is taken on purpose)
+-- shared/                           <- the shared-tooling submodule (lint configs, the CI workflows' scripts, SharedDeploy, repo files) - see **Shared tooling**
+-- Taskfile.yml                      <- day-to-day commands (`task` lists them); includes shared/repo/Taskfile.yml as shared:*
+-- scripts/                          <- check.ps1 (everything CI runs, on demand) + deploy.ps1 (on SharedDeploy; settings in the untracked deploy.config.ps1)
+-- deploy/                           <- Apache deploy artifacts (the web root's .htaccess template + persistent images/ + README)
+-- backend/                          <- Go backend
|   +-- main.go                       <- Entry point: flags, DB init, the embedded SPA, server start
|   +-- sqlc.yaml                     <- sqlc 1.31.1: internal/db/queries/*.sql against schema.sql -> internal/db (`task gen`)
|   +-- schema.sql                    <- the schema the migrations build (cmd/schema-gen), committed: sqlc reads it, a test holds it to the migrations
|   +-- go.mod / go.sum               <- Module deps (alexedwards/scs, modernc.org/sqlite (pure Go), golang.org/x/crypto for argon2id)
|   +-- .golangci.yml                 <- golangci-lint config WRITTEN from shared-tooling's template - never edit it by hand
|   +-- .golangci.local.yml           <- Senpan's own golangci-lint rules, merged into .golangci.yml by `go run -C shared/lint/go . -dir backend`
|   \-- internal/
|       +-- auth/                     <- the accounts' rules: argon2id Hash()/Verify() (PHC strings), the account limits and the reserved admin, personal access tokens (NewToken, HashToken), and SignIn (what a signed-in session carries); shared by store (seed), api (sign-in, accounts) and server (who a request is)
|       +-- api/                      <- the API on huma under /api/v1, one file per domain (version, config, settings, logs, styles, sign-in, me, passkeys, users, patterns, presets, winners, cards, game, board, live, raffles, affiliates, tearooms, announcements, bookclubs, garapons, stamprallies, festivalmaps, festivals (the dashboard), ema, googlefonts, images, carrd, fonts) with thin handlers; discord.go holds what the operations that post to Discord share (the webhook schema, the Discord problem answers); uploads.go what the multipart operations share (the declared parts, the size caps, the skipped-file answer); api.go holds the config, the guards (public, signedIn, permitted, admin) and Deps (including the web root and the limiters); apitest/ checks every 2xx answer against the document
|       +-- db/                       <- sqlc's package, generated (DO NOT EDIT) from db/queries/<domain>.sql; reads on db.New(st.ReadDB()), writes on db.New(st.DB()) or a transaction's
|       +-- themes/                   <- theme tokens: TokenOrder (the allowlist), Sanitize, CSS (the :root block), Marshal/Unmarshal, ValidFlourish
|       +-- settings/                 <- the setting keys and their defaults
|       +-- permissions/              <- the page-permission keys, BookClub(slug), Valid()
|       +-- bookclubs/                <- the book-club registry (Clubs, Slugs, each club's webhook key), which AniList addresses are allowed, and a reading-list item's embed
|       +-- anilist/                  <- the AniList client (15 s, every redirect hop checked against the allowlist), its errors classed like Discord's (503, 429, 500), and the mapping of a Media node to a reading-list item
|       +-- discord/                  <- the Discord transport and its pure parts: the Sender (the dry-run/override policy, 15 s timeout, token-safe errors), typed errors (Ambiguous, Unreachable, RateLimited, Refused) and HTTPStatus, the embed builder, markdown, link buttons, IsWebhookURL
|       +-- origins/                  <- the CORS allowlist rule: Normalize an origin (a `*.` wildcard only as the first label) and Allowed
|       +-- raffles/                  <- the raffle rules, pure: modes, the allowance cap, ticket and entry pricing, what is collected and outstanding, payment state, the open window, the weighted pick
|       +-- affiliates/               <- an affiliate's opening hours, external links and Discord embed
|       +-- tearooms/                 <- a tea room's hashtags, gil formatting, lock-time parse and Discord embed
|       +-- schedule/                 <- an announcement's recurrence, in its time zone (DST-safe), with the clock passed in
|       +-- announcements/            <- an announcement's validation, dynamic event times and Discord message
|       +-- fonts/                    <- the font rules, pure: NameUnsafe (a family that could break out of CSS), grouping files by base name, the served variant, the rotating token math, origin normalizing and the per-font access decision, the kit's CSS, the format and content-type tables
|       +-- festival/                 <- the festival rules, pure: garapon prize sanitizing and the weighted pick, the rally's window, stamp availability and completion rules, shortcodes and event times, ema's days and board validation
|       +-- uploads/                  <- what every upload is judged by, pure: names, slugs and extensions, byte sniffing (images, mp3, mp4), the SVG sanitizer, and `CheckImage` (size caps, the EXIF orientation parser, the JPEG and PNG re-encode, the GIF pre-scan)
|       +-- media/                    <- the files on disk under the web root, tested over `t.TempDir()`: `Images` (the image library and its manifest), `Carrd` (projects, folders and files, with the path checks) and `Fonts` (uploads, the WOFF2 conversion and the per-font metadata)
|       +-- model/                    <- the few types the API doesn't define itself: users.go, logs.go, participant.go (`ParticipantLabel` and its split) and stamprally.go (the stamp types)
|       +-- store/                    <- Store struct wrapping *sql.DB, the migrations and what tests need
|       |   +-- store.go              <- Store struct, New(ctx, path)/Close(), pragmas, shared helpers; cards.go, patterns.go and settings.go keep only `SaveCard`/`CardExists`, `SavePattern` and `GetSetting`/`SetSetting` for the tests and the few reads outside sqlc
|       |   +-- driver_test.go        <- what the store relies on the SQLite driver for: pragmas on every connection, BEGIN IMMEDIATE, cancellation, DATETIME read as stored
|       |   +-- migrate.go            <- goose: runs migrations/ at start; v71 (baseline.sql) is where it took over from the hand-written migrations
|       |   +-- migrations/           <- goose's numbered SQL files (00072_sessions.sql, 00073_timestamps.sql, ...), each setting PRAGMA user_version
|       |   +-- baseline.sql          <- the v71 schema, never edited
|       |   +-- schema.go             <- Schema(): what the migrations build, written to backend/schema.sql by cmd/schema-gen
|       |   \-- times.go              <- the one timestamp form: NormalizeTime, nullTime
|       +-- bingo/                    <- the pure rules of 75-ball bingo: no database, clock or network
|       |   +-- card.go               <- Card/board generation, ID generation, LetterForNumber
|       |   \-- rules.go              <- pattern matching, the columns a game draws from, the half-time mark, the auto-draw interval
|       +-- game/                     <- the game engine: the game played, its draws and its database
|       |   +-- game.go               <- game.Service (start, draw, end, state, winner matching, half-time prompt and answer, yoever, in-memory caching)
|       |   +-- auto.go               <- afterDraw (who hears a draw, when) and the auto-draw loop (Run)
|       |   +-- types.go              <- the game's shapes as the API answers them and the events carry them
|       |   \-- events.go             <- the game's live events and their audiences
|       +-- live/hub.go               <- the live feed's hub: envelope, per-viewer filtering, the last 1000 events for Last-Event-ID, per-stream queues (a stream 64 events behind is ended), the staff re-check on each heartbeat and after any account change, the lossy log tail (admins only)
|       +-- random/random.go          <- card IDs, boards and every draw, from crypto/rand
|       +-- version/version.go        <- the app's version (equals frontend/package.json's; TestVersionIsTheApps checks)
|       +-- web/                      <- the SPA, embedded: dist/ (Vite's build, git-ignored) served from memory with ETags, cache rules, the security headers + Content-Security-Policy; history-mode fallback to index.html
|       +-- logging/logging.go        <- slog setup: JSON handler -> stdout + a rotating file (timberjack; daily-midnight rotation, zstd) + a live-tail tap (io.Writer forwarding each line); runtime-settable level via slog.LevelVar
|       +-- logging/read.go           <- Read(): the log viewer's newest-first tail of the file, filtered by level and text
|       \-- server/
|           +-- sessionstore.go       <- sign-ins in the database (scs's SQLite store), keyed by a SHA-256 of each token
|           +-- site.go               <- GET /healthz + every path outside /api, to the embedded SPA
|           +-- apilayer.go           <- the /api layer: problem details for an unknown path or method, trailing slashes, the caching baseline (ETag, 304)
|           +-- version.go            <- GET /healthz (status, version, schema; 503 when the database doesn't answer)
|           +-- server.go             <- Server struct (deps, the mux, which holds no `/api/` route: huma's operations are mounted on it, broadcast helpers, the startup migrations of images and fonts) + the session layer (currentUser, withWho) + the request-logging middleware (structured JSON; actor identity + capability-token redaction; client IP via loopback-gated CF-Connecting-IP) + `adminMutationResource`, which maps a write's first path segment to the `resource_changed` it sends
|           +-- actor.go              <- per-request actor for the access log: withActor resolves session/PAT user + Cloudflare verified-bot (cf-verified-bot / x-verified-bot) -> auth/user/bot log fields
|           +-- logredact.go          <- redact capability tokens (garapon/stamp-card/font paths, PAT ?token=, Referer) to a short SHA-256 correlation hash in log lines
|           +-- tokens.go             <- who a request is by its personal access token (bearerToken), and account() for the session layer
|           +-- announcements.go      <- RunAnnouncementScheduler: the sweep that posts due announcements, on sqlc, through the discord.Sender
|           +-- tearoomlocks.go       <- RunTeaRoomLockScheduler (lifts a lock whose time has come) and the preflight answer for the public tea-room reads
|           +-- scheduler.go          <- runScheduler: shared ticker/sweep loop behind the announcement and tea-room-lock schedulers
|           +-- ratelimit.go          <- IP-based limiters: sign-in, registration, passkeys, and the public festival writes (garapon draws, stamp guesses, sign-up and lookup, the dashboard, ema submissions)
|           +-- turnstile.go          <- the Cloudflare Turnstile check (fail-closed)
|           \-- tzdata.go             <- blank-imports time/tzdata so IANA timezones resolve on hosts without zoneinfo (Windows)
+-- plugins/                          <- FFXIV Dalamud plugin (C#/.NET) - secondary admin UI over the server
|   +-- pluginmaster.json             <- Dalamud custom-repo index (hosted at /plugin/pluginmaster.json)
|   +-- Directory.Build.props         <- imports shared/lint/dotnet/analyzers.props: the .NET analyzers (latest-recommended) + the shared C# code style (.globalconfig), checked in the build
|   +-- dotnet-tools.json / global.json <- NSwag 14.7.1 pinned as a local tool; Microsoft.Testing.Platform for `dotnet test` (as Kindred Nights' plugins/)
|   +-- SenpanCompanion/              <- the plugin (DLL/InternalName "SenpanCompanionAdmin"; display "Senpan Admin Companion")
|   |   +-- Plugin.cs                 <- entry point: injected [PluginService]s, /senpan (+ /senpan config) command, window system, the HttpClient
|   |   +-- Configuration.cs          <- persisted config: server URL + personal-access token
|   |   +-- nswag.json                <- NSwag: the operations the plugin calls, from frontend/src/api/openapi.json into Api/SenpanApi.g.cs (`task gen`)
|   |   +-- Api/SenpanApi.g.cs        <- the generated client and every schema as a class (Newtonsoft) - committed, DO NOT EDIT
|   |   +-- Core/                     <- no Dalamud, so the tests compile it: SenpanApi.cs (the client's partial: the base address and Bearer token per request, tolerant reading, OpenEventsAsync), Feed (the SSE reader), Backoff (1-16 s), Problems (a refusal in the server's words), Times, Raffles (pricing)
|   |   +-- Services/                 <- LiveConnection (the staff event stream), Session (auth/perms from /me), CardCache (coalesced, framework-thread), NearbyPlayers (IObjectTable), ChatSender (/tell), Chime (winmm synth)
|   |   +-- Windows/                  <- ImGui pages: TabBase + Bingo{Cards,Game,Winners}Tab + RaffleTab + GaraponTab + StampRallyTab + TeaRoomsTab + RollsTab + TimedMacrosTab + MainWindow
|   |   \-- SenpanCompanion.csproj    <- Dalamud.NET.Sdk/15.0.0; <AssemblyName>SenpanCompanionAdmin; EnableWindowsTargeting (Linux CI)
|   \-- SenpanCompanion.Tests/        <- xUnit v3 over Core/ and Api/, compiled in (no Dalamud): the client against Fixtures/ shaped as the document declares, the feed reader, the back-off, Problems, the pricing
+-- .editorconfig / .gitattributes    <- from shared/repo (Sync-RepoFiles.ps1); Senpan's lines - the plugin's two C# exceptions - go below the project marker
+-- devdata/                          <- Local dev sandbox: SQLite DBs + built binaries + webroot uploads (gitignored)
```

## Where this project departs from the standards

Each place below is code that still does what the shared rules now rule out, checked against the code on 2026-11-07. The rest of this file describes the code as it is today. When a follow-up lands, its line here goes, and the lines that describe the old way change with it.

Planned follow-ups, from shared-tooling's `PLAN.md` (step 3, and step 7's five releases):

- **The document root.** The uploads (`images/`, `fonts/`), the Carrd sites and the plugin repository live in Apache's document root. Apache serves `images/`, the Carrd sites and the plugin repository straight from it; `fonts/` sits there unserved, streamed by the app. The goal is nothing served from it, as Home Library and Kindred Nights do (decided 2026-11-09): the app serves them from a data folder under `/opt/senpan`, and the vhost proxies everything to it. It comes after the move to huma, and the fonts need the most care: never reachable as plain files during the move, and `fonts.senpan.cafe/kit.css` and its token URLs unchanged.
- **Vitest layout.** Specs sit beside the code as `src/**/*.test.ts`, with the config in `vitest.config.ts`. They move to `src/__tests__/*.spec.ts`, with the config inside `vite.config.ts`.
- **Static inline styles.** `vue/no-static-inline-styles` is off (`eslint.config.js`, the `senpan/vue` block), and templates hold about 120 static `style=""` attributes, mostly one-off layout such as a form column's flex share. The shared rule comes back on, and those styles move into CSS.

Found in the docs and checked against the code, with no follow-up planned yet. The owner decides whether each moves or stays; one that stays gets its reason here:

- **Client address.** `logClientIP` (`server/ratelimit.go`) trusts `CF-Connecting-IP`, then the leftmost `X-Forwarded-For`, from a loopback peer. The guide has Apache restore the address with `mod_remoteip` and the app read the last `X-Forwarded-For` entry, never `CF-Connecting-IP`; the droplet's `mod_remoteip` change comes first.
- **Bootstrap admin.** Migration v22 seeds `admin`/`admin`, protected by its name. The guide makes the first admin with a command run on the host, and protects the owner accounts named in config. It stays (decided 2026-11-07): the seeded password is changed as soon as an install creates it, and otherwise the account serves only development.
- **Content-Security-Policy.** The app sets the policy on the SPA (`internal/web`), the one Apache set before 4.0.0, with `style-src 'unsafe-inline'` and `https:` sources. The guide sets it in the app with a per-response nonce, never `'unsafe-inline'`, naming only the hosts the app uses.
- **Upload names.** Image, font and Carrd uploads keep the uploaded file name, and a same-named upload overwrites (decided 2026-11-10): staff replace a picture in place by its name, and Carrd's external links depend on names. The guide stores an upload under a random name.
- **Images re-encoded.** The guide re-encodes every raster upload. Here JPEG and PNG are re-encoded (dropping EXIF, GPS and text chunks; the EXIF orientation is applied first), but GIF and WebP are checked and kept as sent (decided 2026-11-10): Go writes no WebP, and re-encoding a GIF risks its animation. An animated PNG keeps its first frame.
- **Markdown editor.** The editor is Milkdown (Crepe), `MarkdownEditor.vue`. The guide's one editor is Tiptap 3 with its Markdown package.
- **Class parts.** Shared objects join a part with `-` (`.list-row-title`) and never use BEM's `__`. The guide makes a reusable component's classes BEM (`.media-card__title`).

## Architecture

**Layered**: HTTP handlers (`api`, over the HTTP shell in `server`) -> the domains' rules (`game` over `bingo`, `raffles`, `festival`, `uploads`, `fonts`, ...) and the files on disk (`media`) -> Data access (`db`, `store`) -> SQLite. An operation in `api` stays thin: it decodes, guards, calls a rule or a query and answers.
All dependencies are wired in `main.go` and passed via structs (no globals, no singletons).

**Package responsibilities**:

- `model` - The few types the API doesn't define itself (accounts, log entries, the participant label, a stamp's display name). No imports beyond stdlib.
- `store` - Single `Store` struct wrapping the two pools, the migrations and the timestamp form; the queries themselves are sqlc's (`db`).
- `festival`, `uploads`, `media`, `fonts` - the festival's rules (the garapon pick, the rally and map and ema rules), what every upload is judged by (names, byte sniffing, the SVG sanitizer, the image check and re-encode), the files under the web root (the image library, the Carrd projects, the fonts and their WOFF2 copies) and the font rules; the first two and the last are pure, `media` is tested over a temporary directory.
- `bingo` - The pure rules: card/board generation, matching a card against a pattern, the draw columns, the half-time mark and the auto-draw interval.
- `game` - The game engine: `Service` for the game's lifecycle (start, draw, end, winner computation, the half-time prompt, yoever, the auto-draw loop), its SQL on sqlc. Caches game state and card data in memory for performance, and sends the game's events to the hub.
- `live` - The live feed's hub (server-sent events): publishes each event to the viewers it is for, keeps the last 1000 for `Last-Event-ID`, and owns each stream's queue and heartbeat. Self-contained: it knows nothing of bingo.
- `server` - `Server` struct implementing `http.Handler`. Holds Store, the game Service, the live Hub, session store, web root. Registers no route under `/api/`: huma mounts every operation (`internal/api`) and the `/api` layer answers the rest. Owns the **session layer** (who a request is, by session or token, handed to huma as `api.WithWho`/`api.WithRequest`), the request log and its redaction, the SPA and `/healthz`, the announcement and tea-room-lock **background schedulers**, the `resource_changed` each write sends (`adminMutationResource`), and the start-up migrations of the image library and the fonts (`server.New`).
- `auth` - the accounts' rules: argon2id `Hash()` / `Verify()` over PHC-format strings (64 MB / t=1 / p=4, the OWASP baseline; `Verify` is constant-time), the account limits and the reserved `admin`, personal access tokens, and `SignIn`, which every way of signing in (and a password change) goes through. Shared by `store` (seeding the bootstrap admin), `api` (sign-in, accounts) and `server` (resolving who a request is).
- `logging` - process-wide `slog` setup (leaf; imports only `model` + timberjack). Installs a JSON handler writing to stdout **and** a rotating file, holds the runtime level in a `slog.LevelVar` (`SetLevel`/`CurrentLevel`), and exposes a settable tail sink (`SetTailSink`) that forwards each finished log line to a callback. `main` wires that callback to `api.LogTail`, which publishes the line to the admins' streams for the live tail. See the **Server logs** operational note below.

**Authentication & authorization**:

- **Accounts, not a shared password.** `users` table holds `username`, argon2id `password_hash` (selected only by the two queries that check or set it, `GetPasswordHash` and `SetPassword` - never on `model.User` or an API type, so it can't leak through JSON), `is_admin`, `is_active`, and a JSON `permissions` array. Signing in (`POST /api/v1/auth/login`) verifies the hash, is **IP rate-limited**, rejects inactive accounts, and stores `user_id` in the SCS session (token rotated on login to prevent fixation). A missing user and a bad password return the same generic error so usernames can't be enumerated.
- **Registration is hidden + gated.** `POST /api/v1/auth/register` (page `/admin/register`, linked nowhere) creates accounts **inactive, non-admin, no permissions**; an admin must activate + grant access before they can log in. The reserved `admin` username can't be registered. Like signing in, it **answers identically** (202, no body) whether the username is new, already taken, or reserved, so it can't be used to enumerate accounts (a length/format error is still reported).
- **Bootstrap admin.** Migration v22 seeds `admin`/`admin` (active, full admin) via `INSERT OR IGNORE` (never clobbers a rotated password). `main.go` logs a warning that it must be rotated. The `admin` account is protected: only it can change its own password (`PUT /api/v1/me/password`), and no one can delete/deactivate/demote it.
- **Per-page permissions.** Each admin page has a permission key equal to its frontend `AdminTab` id (e.g. `bingo-cards`, `teahouse-raffles`, `bookclub-yaoi`). Admins implicitly hold all of them; non-admins hold only the keys granted to them. The **Users page (`system-users`) is intentionally not a grantable key** - it is admin-only (`requireAdmin`).
- **Password changes invalidate other sessions.** Each account carries a `password_epoch`; a change (self-service `PUT /api/v1/me/password`) or admin reset (`PATCH /api/v1/users/{id}`) bumps it (the `SetPassword` query does so in one statement). Every cookie session records the epoch it was minted with, and `currentUser` rejects a session whose stored epoch no longer matches - so a password change logs out **every other** browser session, while the initiating session is signed in again under the new epoch (`auth.SignIn`) so it stays valid. The change also revokes the account's personal access token, in the same transaction. A token can't change the account's credentials: making or revoking a token, changing the password and adding or removing a passkey take a session (403 for a token).
- **Real client IP for throttling.** The login/registration/passkey rate limiter keys on the same authoritative client IP as the access log: `CF-Connecting-IP`/`X-Forwarded-For` are trusted **only** when the immediate peer is the loopback reverse proxy, otherwise `RemoteAddr` is used - so behind Cloudflare users don't share (or forge) a throttle bucket. Passkey sign-in's start has a budget of its own (so opening the prompt can't lock out the password); its finish counts with the password's failures. A 429 says when to try again (`Retry-After`).
- **Passkeys (WebAuthn).** Accounts can also log in **passwordless** via discoverable WebAuthn credentials (`go-webauthn`, `internal/api/passkeys.go` + `internal/db/queries/passkeys.sql` + `user_passkeys`), offered alongside password login. A signed-in account adds one or more passkeys (`POST /api/v1/me/passkeys/begin`, which asks for the account's password, then `POST /api/v1/me/passkeys`; `GET`/`DELETE /api/v1/me/passkeys[/{id}]`); signing in (`POST /api/v1/auth/passkey/begin|finish`) resolves the account from an 8-byte user-ID handle, requires an active account, verifies the assertion (with signature-counter clone detection), and establishes the same SCS session (token rotated). The RP-ID/origin derive from the request host at runtime (no env), and the challenge is held in the session between begin and finish, for ten minutes at most, answering once. Frontend: `lib/passkeys.ts` + `stores/auth.ts`, surfaced on `AdminLoginView` and a manage-passkeys modal in `AdminView`.
- **Bot check (Cloudflare Turnstile).** When a Turnstile secret **and** sitekey are configured (`-turnstile-secret`/`-turnstile-sitekey` flags or `APPSUITE_TURNSTILE_SECRET`/`_SITEKEY` env), the public POST paths - login, registration, raffle sign-up, custom card request, stamp-rally sign-up, the public ema submissions - verify a one-time token server-side (`server/turnstile.go`, **fail-closed**: an empty/invalid token or a transport error is rejected) before doing credential/write work; a failed challenge returns **403**. Unconfigured (empty secret) disables the check entirely (logged at startup), and the public sitekey is served via `GET /api/v1/config` (`turnstileSiteKey`, empty when off) so the frontend widget (`TurnstileWidget.vue`) skips rendering. A failed login challenge is deliberately **not** counted against the rate limiter (so an expired token can't lock out a real user).
- **Guards.** `currentUser(r)` (server.go) loads the active account from the store **on every request** (so permission/activation changes take effect immediately - no stale session snapshot), and hands it to huma as `api.Who`. On huma, an operation's middleware guards it (`internal/api`): `signedIn` (any active account), `session` (signed in by a session, not a token), `permitted(perm...)` (an admin or the granted key) and `admin` - 401 when nobody is signed in, 403 without the access. The book-club guard (`bookclub-<slug>`: 401, 404 for an unknown club, 403) covers the AniList lookup and the cover upload, and the image library's readers (`permissions.ImageBrowsers()`, every page that picks an image, `festival-ema` included) read it while `system-images` writes. Every mutating admin operation guards first; a public one has its own limiter and, for the sign-ups and plaque submissions, the Turnstile check.
- **Frontend gating mirrors the server.** `stores/auth.ts` holds `user`/`isAdmin`/`hasPermission`; the router guard checks `meta.tab` as the permission key (redirecting to the first allowed page, or `/admin/no-access` when none); the sidebar hides pages/sections the account can't reach; `AdminView` only fires the data loads the account is allowed. **This is convenience, not security** - the server enforces every endpoint regardless of what the UI shows.
- **Live-stream access.** `GET /api/v1/events` (the staff's) takes a session or a Bearer token and answers 401 for nobody, 403 for an account with no permission, 503 when the feed can't take another stream; the hub filters every event by what the account's permissions cover (an admin holds all), so the draw-delay anti-peek holds: only Game staff get a draw at once, with the winners. `GET /api/v1/board/{id}/events` is public by card id (404 unknown, 403 a pending personal card, 422 a bad id, 503 with `Retry-After` past 500 open boards) and hears each number after the draw delay. A staff stream re-checks its account every heartbeat and right after any account change: deactivated, deleted, a password change, signing out or losing every permission ends it, and losing one permission narrows what it hears.

**Discord embeds**: `internal/discord` holds the shared embed schema, a fluent `NewEmbed()...Build()` builder (auto-truncates to Discord's per-field limits, skips empty fields), a `#rrggbb`->int color helper, and the `Sender`, whose `Post` / `PostEmbed` / `PostFile` are the one transport (a payload carries an optional `content` mention + `allowed_mentions`). Feature code (affiliates, tea rooms, announcements, reading-list items, ema) only assembles a builder chain - no feature has its own transport. **Dev safety:** because a development server is normally pointed at a COPY OF THE LIVE DATABASE - whose announcement types, book clubs, affiliates and tea rooms all carry real webhook URLs - two valves sit in the `Sender` (`Server.SetWebhookPolicy`, wired from `-webhook-dry-run` / `APPSUITE_WEBHOOK_DRY_RUN=1` and `-webhook-override` / `APPSUITE_WEBHOOK_OVERRIDE`). Dry run suppresses every outbound post and logs it; override redirects every post to one test webhook whatever the database says; dry run wins when both are set, both are off in production, and both log a WARN at startup. Put the override URL in the environment, never in the repo - it is a credential. Without one of these, simply starting a local server is enough for the announcement scheduler to post to production channels. **Failures are typed:** a failed post is an `Ambiguous` (no answer after the request may have gone out: never retry blindly), `Unreachable`, `RateLimited` (with `Retry-After`) or `Refused` error, and `discord.HTTPStatus` maps them to 503, 503, 429 and 500 - never 502. An operation answers them through `discordProblem` (`internal/api/discord.go`); a webhook that isn't set is a 500, since it is config. The AniList client (`internal/anilist`, 15 s timeout) is classed the same way.

**Background scheduler**: `main.go` launches `RunAnnouncementScheduler` (`server/announcements.go`, on sqlc, posting through the `discord.Sender`) in a goroutine tied to a shutdown-cancelled context. It uses the ticker/sweep loop in `scheduler.go` (`runScheduler`): ticks every 30s (and sweeps once on startup to catch up after downtime), posts due announcements to their type's webhook, and is resilient - an announcement whose webhook is unset is left pending, a failed post is retried next tick, and `skip_count`/recurrence advance the cursor without losing the schedule. `skip_count` is how many upcoming occurrences to pass over (each skipped one decrements it), so a Friday+Saturday announcement can sit out a whole weekend in one go; it is schedule-cursor state, written ONLY by the skip operation and the scheduler - saving an announcement deliberately leaves it alone, since writing it from the form is what used to clear a pending skip whenever an admin saved an unrelated edit. After a sweep that wrote anything (posted, consumed a skip, gave up) it sends `resource_changed` `announcements` once, so open Announcements pages read the list again. Wall-clock times are resolved against each announcement's IANA timezone (DST-safe, in `internal/schedule`) and stored as UTC; `tzdata.go` embeds the zone database so this works on Windows hosts too.

**Logging & observability**: the app logs **structured JSON** via `slog` (set up in the `logging` package). Output goes to **stdout** (captured by journald) and, when `-log-file` is set (default `/var/log/senpan/senpan.log`), to a **rotating file** (timberjack: daily-midnight rotation, zstd backups, bounded retention) - the file sink degrades gracefully to stdout-only if it can't be created (e.g. a read-only `/var/log` under `ProtectSystem=strict`; the systemd unit needs `LogsDirectory=senpan` - see `deploy/README.md`). The request-logging middleware records `method`/`path`/`status`/`duration`/`ip` per request, using the **real client IP** (`CF-Connecting-IP` behind Cloudflare, not the proxy's `[::1]`). Admins view logs **without SSH**: `GET /api/v1/logs` tails the file (level/text/limit filters), and every line is also pushed to the admins' event streams as a `log` event so **System -> Logs** live-tails them (see the tab in the frontend section). The minimum level is a `slog.LevelVar` flipped at runtime by `PUT /api/v1/logs/level` - turn **DEBUG** on to capture a richer `request detail` line per request plus DEBUG logs across the services, then back off, with no restart. The same NDJSON parse (`model.ParseLogEntry`) backs both the REST tail and the live feed.

**What gets an Info line.** Beyond per-request logging: every **delivered** Discord post (`discord post sent`, emitted once in the `discord.Sender` with a `webhookTarget` label - kind + record name - so a new posting feature is covered without adding a line); **scheduler** start/stop for both background loops; the **bingo game** lifecycle (`bingo game started` / `bingo game ended` / `bingo halftime decision`, with the acting admin via `Server.actorName`); a consumed **skip** on an announcement (with the count remaining); and manual announcement sends + reading-list publishes with their `by` attribute. Two things are deliberately quieter: a **failed** post is logged by the caller that knows whether it retries or answers the failure (never in the Sender too), and high-frequency events stay at **Debug** or are dropped entirely - individual scheduler sweeps (2,880/day at a 30s tick) and individual bingo draws (~75/game) would bury everything else.

**Key data flow - drawing a number**:
Admin clicks Draw -> `POST /api/v1/game/draw` -> the `draw-number` handler (`internal/api/game.go`) ->
`game.Service.Draw()` -> picks random uncalled number, computes winners (skipping already-known winners), caches in `games.winners_cache` ->
returns `DrawResult` (drawn number + game state + winners) -> handler writes JSON; the game service publishes a `draw` event (Game staff at once with the winners, boards after the draw delay).

**Key data flow - real-time updates**:
After a state-changing call (draw, start, end, card/pattern mutations, settings and theme saves, sign-ups), the handler or the game service calls `Hub.Publish(event, audience)` with the audience that may hear it. An event names what changed and the client reads it again over REST; a draw carries its number. Every stream starts with the events since its `Last-Event-ID`, and a client reads its state again each time a stream opens. API.md has the events and their audiences.

**Performance optimizations**:

- GameState is cached in `GameService` memory - invalidated on start/draw/end; board lookups avoid 3 DB queries.
- Card list is cached in `GameService` - invalidated on generate/delete; winner computation avoids full DB read.
- Winner computation skips already-known winners (only scans new cards on each draw).
- `FrequentWinners` query uses composite index `(player_name, logged_at)`.
- `ListWinnersLog` uses `COUNT(*) OVER()` window function (single query for data + total).
- SQLite connection pool allows 4 concurrent connections for WAL concurrent readers.

**Schema versioning**: goose runs the numbered migrations in `store/migrations/` at start, on the write pool, each setting `PRAGMA user_version` to its own number (currently **74**; v74 seeded the `tea_room_origins` setting with `https://senpan.cafe` and `https://*.senpan.cafe`; v73 moved every timestamp to the one form - RFC 3339 in UTC to the second, a `DATETIME` column with a CHECK, `NULL` for none - rebuilding each table by SQLite's procedure, and rebuilt the tables production had drifted on, so its schema is exactly a fresh build's; v72 added the `sessions` table). v71 is the baseline (`store/baseline.sql`), where goose took over from hand-written `if version < N` blocks; a database they left short of v71 is refused. Their history: v71 added `ema_windows.artist_limit` - the most artist requests a plaque day takes, 0 = no limit, how every day behaved before; v70 added the **Ema Plaques** tables (`ema_boards` one per festival map, cascading with it; `ema_windows`, the days plaques are taken on, each with its Discord webhook; `ema_plaques`, with a PARTIAL UNIQUE index on window + name + world, ignoring case, over every status but `rejected` - the one-plaque-per-person-per-day rule, enforced by the database so racing submissions can't both land); v69 added the tea-room lock - `tea_rooms.locked` (locked right now, as opposed to `lockable`, which only says a room CAN be locked) and `tea_rooms.locked_until` (the UTC instant a background sweeper lifts it at; '' = it stands until an admin unlocks the room, how every lock behaved before); v68 added `festival_stalls.text_color` - the colour a pitch's label is drawn in, since the old fixed dark ink only read on a light fill; v67 added `festival_stalls.selection_color` (the halo around a tapped pitch, its own colour so it can contrast with the fill) and `festival_stall_occupants.event_carrd` (a per-OCCUPANT event-page link, since a pitch that changes hands has a different page each day); v66 gave `stamp_rally_cards` and `garapon_players` their own `world` column so every system records a participant the same way - name and home world, kept apart; v65 added `raffles.festival_map_id` + `raffles.occupant_id` - the festival a raffle belongs to and the pitch occupant it is assigned to, so the public map's stall panel can link to a raffle that is running; v64 split a festival stall into the PITCH on the plan (`festival_stalls`) and the OCCUPANTS standing in it (`festival_stall_occupants`), so a booth that changes hands between days is one shape with two names, and renamed/remapped `stamp_rally_stamps.stall_id` to `occupant_id` since a stamp belongs to whoever runs the pitch that day; v63 added `festival_stalls.type_label` - the caption drawn under a stall's title, used when its type is `other` so a booth can read "Omikuji" rather than "Other"; v62 added `festival_maps.slug` - the optional shortcode a map is linked by - behind a PARTIAL unique index (`WHERE slug != ''`, since '' means "no shortcode" and every map without one would otherwise collide); v61 added the **Festival Map** tables (`festival_maps` + `festival_stalls`) and the two columns that tie a Stamp Rally to one - `stamp_rallies.festival_map_id` and `stamp_rally_stamps.stall_id`, both FK-less like the garapon link, so the store clears them itself on delete; v60 split stamp-rally stamps into food and game stamps (`stamp_rally_stamps.stamp_type` + a `stamp_rally_collected.stamp_type` snapshot, both defaulting to `food` so every stamp on every past rally stays what it was) and let a rally complete on a count of each rather than the whole card (`stamp_rallies.completion_mode` defaulting to `all`, plus `required_food`/`required_game` - a `counts` rally requiring nothing of either type is refused on save, and falls back to the whole-card rule if such a row is ever read); v59 added `raffles.pay_image`; v58 added the raffle-entry settlement columns (`paid_entries`/`waived`); v57 added `raffles.entry_mode` + `tier_costs`; v56 replaced `announcements.skip_next` (a boolean) with `skip_count` - how many upcoming occurrences to skip, carrying a pending skip over as 1; v55 added `garapons.default_draws` - the draw allowance a link carries when nobody picks a number, used by public stamp-rally sign-up and as the admin fallback; v54 added `stamp_rallies.public_signup` - the self-service sign-up opt-in, defaulting to 0 so no existing rally became public; v53 added `tea_rooms.room_owner`; v52 added `affiliates.subtitle`; v51 added `users.password_epoch` - a counter bumped on every password change/reset that invalidates the account's other sessions; v50 added the `game_presets.auto_call`/`auto_interval` columns for auto-run games). v22 added the `users` table + seeded the bootstrap admin; later migrations grew the announcements feature (roles, mention, location, thumbnail, dynamic dates, sort order) and dropped the retired `book_club_events` table (v28), moved themes to structured design tokens + added style flourishes (v34/v37), then shipped the **Garapon** festival lottery drum (v35-36), **Affiliates** (v38), the **Stamp Rally** (v39-41, including the v41 collected-log rebuild that keeps logs after a card/stamp is deleted), per-account **personal-access tokens** for the plugin/API (v42), and a `UNIQUE(game_id, number)` index on `called_numbers` as a duplicate-draw backstop (v43), then **passkeys** (WebAuthn `user_passkeys`, v44), **Tea Rooms** (v45; `subtitle` + a unique `room_number` v46), added affiliate fields (v47) and the stamp-rally redeem image (v48), and **public/private theme visibility** plus **custom card requests** (the `cards.protected`/`custom_status`/`world` columns, v49).

**Bingo card format**: 5x5 array `board[row][col]`; col 0=B(1-15) ... col 4=O(61-75); centre `[2][2] = 0` (FREE).
**Pattern format**: 5x5 `[][]bool` grid; `true` = cell must be called for win; centre always counts.
**Card IDs**: 6-char alphanumeric (no ambiguous chars 0/O/1/I/l), enforced unique in DB.

## Design philosophy

The principles below are _why_ Senpan's own conventions exist - keep changes aligned with them. The shared ones (layering, typed handlers, the server as the security boundary, broadcasting after a write) are in `shared/standards/`.

1. **Theme fidelity is non-negotiable.** Themes override **only** the `:root` design tokens via a structured token editor - no free-form CSS, and class names are **not** a theme API. So: never hard-code colors/fonts (drive everything off tokens), and prefer (1) an existing object, (2) a token on existing markup, (3) a new shared object - page-scoped CSS is the last resort. Layout is a stylesheet of object classes, not Tailwind; object classes can be refactored freely since themes don't target them.
2. **CSS models objects, not pages.** A small vocabulary of reusable primitives/objects (the `components/common/ui/` set, `.card`, `.chip`, `.badge`, the button system) is composed across features instead of per-page rules. Intent = fill colour only; never an outline/ghost (a surface-coloured control vanishes into its container - the recurring "ghost button" bug).
3. **Fast by default.** Lazy routes + `manualChunks` keep the player/home payload tiny; the game service caches state/cards/details in memory and invalidates precisely; winner computation is incremental; list queries are index-backed and single-pass (`COUNT(*) OVER()`); the admin nav freshness-gate skips redundant refetches.
4. **Registry-driven extensibility.** Book clubs and page permissions are declared once in a registry and wire up their routes, nav, settings, and guards automatically. Prefer adding a registry entry over copy-pasting a feature.

## Database tables

| Table                   | Purpose                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `users`                 | `id INTEGER PK`, `username TEXT UNIQUE`, `password_hash TEXT` (argon2id PHC), `is_admin INTEGER`, `is_active INTEGER`, `permissions TEXT` (JSON array of page-permission keys), `created_at` - admin accounts                                                                                                                                                                                                                                                          |
| `cards`                 | `id TEXT PK`, `board_data TEXT` (JSON 5x5 array), `player_name TEXT`, `details TEXT`, `world TEXT`, `protected INTEGER` (spared by Delete-All), `custom_status TEXT` (`''` normal / `pending` / `approved` - the custom-card-request lifecycle), `created_at`                                                                                                                                                                                                          |
| `pattern_categories`    | `id INTEGER PK`, `name`, `sort_order INTEGER`                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `patterns`              | `id INTEGER PK`, `name`, `pattern_data TEXT` (JSON), `sort_order INTEGER`, `category_id INTEGER`                                                                                                                                                                                                                                                                                                                                                                       |
| `games`                 | `id INTEGER PK`, `status` (active/ended), `created_at`, `winners_cache TEXT` (JSON array of card IDs)                                                                                                                                                                                                                                                                                                                                                                  |
| `game_patterns`         | Snapshot of patterns when game started (game_id + pattern_id + name + data)                                                                                                                                                                                                                                                                                                                                                                                            |
| `called_numbers`        | `game_id`, `number`, `call_order`, `UNIQUE(game_id, number)` (v43, the duplicate-draw backstop)                                                                                                                                                                                                                                                                                                                                                                        |
| `settings`              | `key TEXT PK`, `value TEXT` - key-value config (e.g. `game_details`, `active_style_id`, `app_title`, `default_draw_delay`, `frequent_winner_threshold`/`_hours`, `header_font`, `google_fonts_api_key`, `anilist_api_url`, `bingo_join_prompt`, `yoever_cooldown_seconds`, `custom_card_cost`, `hide_bingo`/`hide_custom_cards`, `settings_updated_at` (the 409 check), `tea_room_origins` (a JSON array of the origins that may read the public tea-room API; seeded by v74, in no defaults, so saved empty it stays empty), and the secret per-page Discord webhooks: `discord_webhook_url_<slug>` per book club, `tearoom_webhook_url`, `affiliate_webhook_url`) - note Turnstile keys are **not** here; they come from flags/env              |
| `styles`                | `id INTEGER PK`, `name`, `tokens TEXT` (JSON token map), `created_at`                                                                                                                                                                                                                                                                                                                                                                                                  |
| `game_presets`          | `id INTEGER PK`, `name`, `pattern_ids TEXT` (JSON), `game_details TEXT`, `auto_call INTEGER` (start with auto-draw on), `auto_interval INTEGER` (seconds between auto draws), `created_at` - reusable game templates                                                                                                                                                                                                                                                   |
| `raffles`               | `id INTEGER PK`, `title`, `description`, `rules`, `max_entries`, `signup_instructions`, `entry_mode`, `cost_per_entry`, `tier_costs`, `available_from`, `available_to`, `prize_image`, `status`, `winner_entry_id`, `created_at`                                                                                                                                                                                                                                                                   |
| `raffle_entries`        | `id INTEGER PK`, `raffle_id`, `character_name`, `world`, `num_entries`, `paid_entries`, `amount_waived`, `paid`, `created_at`                                                                                                                                                                                                                                                                                                                                                                           |
| `winners_log`           | `id INTEGER PK`, `logged_at`, `card_id`, `player_name`, `game_details`, `winning_patterns TEXT` (JSON)                                                                                                                                                                                                                                                                                                                                                                 |
| `reading_lists`         | `id INTEGER PK`, `club_slug`, `title`, `created_at` - a book club's named reading list                                                                                                                                                                                                                                                                                                                                                                                 |
| `reading_list_items`    | `id INTEGER PK`, `list_id`, `cover_image`, `title`, `summary`, `format`, `genres`, `tropes`, `chapters`, `comments`, `sources TEXT` (JSON), `sort_order`                                                                                                                                                                                                                                                                                                               |
| `announcement_types`    | `id INTEGER PK`, `name`, `webhook_url`, `created_at` - a named Discord destination                                                                                                                                                                                                                                                                                                                                                                                     |
| `announcement_roles`    | `id INTEGER PK`, `name`, `role_id` (Discord role snowflake), `created_at` - a taggable role an announcement can ping                                                                                                                                                                                                                                                                                                                                                   |
| `announcements`         | `id INTEGER PK`, `type_id`, `title`, `details` (markdown), `image`, `color`, `location`, event window (`start_local`/`end_local` + computed `start_at`/`end_at`), schedule (`schedule_kind`, `timezone`, `once_local`, `schedule_minutes`, `schedule_weekdays`, `schedule_week_of_month`), `next_post_at`, `skip_count` (upcoming occurrences to skip), `active`, `last_posted_at`, `buttons TEXT` (JSON array of up to 5 Discord link buttons), `mention` (`""`/`everyone`/`role:<id>`), `created_at` |
| `affiliates`            | `id INTEGER PK`, `name`, `owners TEXT` (JSON), `location`, `timezone`, `hours TEXT` (JSON opening-hours ranges), `details` (markdown), `logo`, `screenshot`, `created_at` - a partner establishment (Senpan Tea House -> Affiliates)                                                                                                                                                                                                                                   |
| `tea_rooms`             | `id INTEGER PK`, `name`, `subtitle`, `room_number` (UNIQUE public key), `cost_per_half_hour`, `hashtags`, `description` (markdown), `seasonal`/`open`/`lockable`/`discounted` (flags), `locked` + `locked_until` (the lock and the UTC instant it lifts at; '' = until an admin unlocks it), `image` (library URL), `color` (embed accent), `sort_order`, `created_at` - a bookable room (Senpan Tea House -> Tea Rooms); a public CORS read API (`/api/tea-rooms/public`) feeds external Carrd sites its live availability + gil pricing                                    |
| `garapons`              | `id INTEGER PK`, `title`, `details` (markdown), `grand_prize_image`, `status` (open/closed), `stamp_rally_id` (optional link), `default_draws` (draws per link when unspecified), `created_at` - a festival lottery drum                                                                                                                                                                                                                                                                                                  |
| `garapon_prizes`        | `id INTEGER PK`, `garapon_id`, `name`, `ball_color`, `rate REAL`, `is_grand`, `sort_order` - a garapon prize tier                                                                                                                                                                                                                                                                                                                                                      |
| `garapon_players`       | `id INTEGER PK`, `garapon_id`, `token TEXT UNIQUE`, `player_name`, `max_draws`, `stamp_card_id` (optional, for a linked rally), `created_at` - a per-player drawing link                                                                                                                                                                                                                                                                                               |
| `garapon_draws`         | `id INTEGER PK`, `garapon_id`, `player_id` (nullable, `ON DELETE SET NULL`), `prize_id`, snapshots `player_name`/`prize_name`/`ball_color`, `drawn_at` - the draw log (survives link deletion)                                                                                                                                                                                                                                                                         |
| `stamp_rallies`         | `id INTEGER PK`, `title`, `card_image`, `not_stamped_image`, `available_from`/`available_to`, `details`, `redeem_instructions`, `status` (open/closed), `public_signup` (self-service sign-up opt-in, default 0), `completion_mode` (`all` = collect the whole card, the default; `counts` = per-type requirement), `required_food`/`required_game` (the counts, read in `counts` mode only), `festival_map_id` (optional Festival Map link), `created_at` - a stamp-rally event (Festival -> Stamp Rally)                                                                                                                                                                                                                                                   |
| `stamp_rally_stamps`    | `id INTEGER PK`, `rally_id`, `occupant_id` (nullable; a `festival_stall_occupants` row when the rally is linked to a map - the stamp then takes its affiliate from that occupant, and names the occupant rather than the pitch because a pitch that changes hands hosts two different stalls), `affiliate_id` (nullable = Tea House default), `image`, `password`, `stamp_type` (`food`/`game`, default `food`), `%`-based placement (`pos_x`/`pos_y`/`width`/`height`/`rotation`), `active_from`/`active_to`, `paused`, `sort_order`                                                                                                                                                                                                                                                  |
| `stamp_rally_prizes`    | `id INTEGER PK`, `rally_id`, `name`, `image`, `%`-based placement (`pos_x`/`pos_y`/`width`/`height`/`rotation`), `sort_order`                                                                                                                                                                                                                                                                                                                                          |
| `stamp_rally_cards`     | `id INTEGER PK`, `rally_id`, `token TEXT UNIQUE`, `participant_name`, `completed`, `completed_at`, `created_at` - a per-participant card                                                                                                                                                                                                                                                                                                                               |
| `stamp_rally_collected` | `id INTEGER PK`, `rally_id`, `card_id`/`stamp_id` (nullable, `ON DELETE SET NULL`), snapshots `participant_name`/`stall_name`/`stamp_type`, `stamped_at`, `UNIQUE(card_id, stamp_id)` - the collected-stamp log (survives card/stamp deletion)                                                                                                                                                                                                                                      |
| `festival_maps`         | `id INTEGER PK`, `title`, `slug` (optional shortcode, '' = linked by id only; PARTIAL UNIQUE index), `description` (markdown), `times TEXT` (JSON datetime ranges), `map_image` (the bare floor plan), `status` (`in_progress`/`published`/`closed`; only `published` is public), `created_at` - an interactive festival floor plan (Festival -> Festival Map) |
| `festival_stalls`       | `id INTEGER PK`, `map_id`, `shape` (`circle`/`rect`), `color` (`#rrggbb`, '' = the leading occupant's type default), `%`-based placement (`pos_x`/`pos_y`/`width`/`height`/`rotation`), `sort_order` - one PITCH on the plan: where a booth sits and how it is drawn, not who is in it |
| `festival_stall_occupants` | `id INTEGER PK`, `stall_id`, `affiliate_id` (nullable = the venue itself), `title`, `description` (markdown), `stall_type` (`game`/`food`/`both`/`other`), `type_label` (the caption under the title, rendered only when the type is `other`), `times TEXT` (JSON; empty = the whole festival), `sort_order` - who stands in a pitch. Several rows when a booth changes hands between days |
| `ema_boards`            | `id INTEGER PK`, `festival_map_id` (UNIQUE, FK `ON DELETE CASCADE`), `title`, `template_image`, `instructions` (markdown), `discord_link`, `allow_self`/`allow_site`/`allow_artist`, `artist_name` (default `Tara`), `status` (`open`/`closed`), `created_at` - a festival's Ema plaque board (Festival -> Ema Plaques) |
| `ema_windows`           | `id INTEGER PK`, `board_id`, `label`, `start_at`/`end_at` (UTC; windows never overlap - checked on save), `webhook_url` (secret: staff only), `artist_limit` (most artist requests taken; 0 = no limit), `sort_order` - one plaque day |
| `ema_plaques`           | `id INTEGER PK`, `board_id`, `window_id` (nullable, `ON DELETE SET NULL`), `window_label` (snapshot), `kind` (`site`/`artist`), `character_name`, `world`, `description` (artist requests), `image` (a site plaque's PNG under `images/ema_plaques/` - cleared, and the file deleted, once posted or rejected), `status` (`pending`/`posting`/`posted`/`rejected`, or `requested`/`complete`), `reviewed_by`/`reviewed_at`, `created_at` |
| `user_tokens`           | `user_id INTEGER PK`, `token_hash TEXT UNIQUE` (SHA-256; plaintext shown once), `token_prefix`, `created_at`, `last_used_at` - one personal-access token per account for the plugin/API bearer auth                                                                                                                                                                                                                                                                    |
| `user_passkeys`         | `id INTEGER PK`, `user_id` (FK->users, `ON DELETE CASCADE`), `credential_id TEXT UNIQUE` (base64url), `credential TEXT` (go-webauthn Credential JSON - key material, never on the API model), `name`, `created_at`, `last_used_at` - a WebAuthn passkey for passwordless login                                                                                                                                                                                         |

Indexes: `games(status)`, `called_numbers(game_id, call_order)`, `called_numbers(game_id, number)` (UNIQUE), `game_patterns(game_id)`, `raffle_entries(raffle_id)`, `winners_log(logged_at)`, `winners_log(player_name, logged_at)`, `cards(player_name)`, `reading_list_items(list_id)`, `announcements(type_id)`, `announcements(active, next_post_at)` (due sweep), `garapon_prizes(garapon_id)`, `garapon_players(garapon_id)`, `garapon_players(token)`, `garapon_draws(garapon_id)`, `garapon_draws(player_id)`, `stamp_rally_stamps(rally_id)`, `stamp_rally_prizes(rally_id)`, `stamp_rally_cards(rally_id)`, `stamp_rally_cards(token)`, `stamp_rally_collected(card_id)`, `stamp_rally_collected(stamp_id)`, `stamp_rally_collected(rally_id)`.

**On-disk uploads (not in the DB)**, all under `<webRoot>`:

- `images/<category>/` - the **central image host** (System -> Images). Each category is a subfolder served at `/images/<dir>/<file>`. The first start seeds ten editable categories (Announcement Main and Thumbnail, Raffle, Garapon, Flourishes, Affiliate Logos and Images, Stamp Cards, Stamps and Prizes); admins may rename, add or delete categories, all tracked in a dotfile manifest `<webRoot>/images/.categories.json` (no DB table). Renaming keeps the folder unless a new `dir` is sent, since stored paths name it; `bookclub`, `announcements` and `ema_plaques` are reserved for other features and can't be a category. Affiliate logos/screenshots, garapon/stamp-rally artwork, etc. are picked from these categories.
- `images/bookclub/` - reading-list item cover uploads (AniList covers stay remote)
- `images/ema_plaques/` - Ema plaques designed on the site, held ONLY while they wait for staff review (unguessable names). Approving posts the image to Discord and deletes the file; rejecting deletes it; deleting the board or its festival deletes what's left. Not an image-library category.
- `fonts/` - admin-uploaded **font files** (`.ttf/.otf/.woff/.woff2/.eot`); files sharing a base name group into one logical FONT (its format variants). Licensed assets - NOT served statically: the Go server streams them via rotating tokenized URLs gated **per font** by its own origin allowlist (`internal/fonts` rules, `internal/api/fonts.go` serving); the `https://fonts.senpan.cafe` vhost is a reverse proxy onto `/api/v1/fonts/pub/` and external sites embed `kit.css` (filtered per site - see Deployment). A font with no uploaded WOFF2 gets one auto-converted into `fonts/.woff2/` (`internal/media/fonts.go`, tdewolff/font; upload-time + startup backfill) - the served variant defaults to WOFF2, selectable per font
- `carrd/<project>/...` - Carrd image-host projects (images + `.mp3`/`.mp4`), each with a `.carrd.json` title sidecar; served cross-origin from `https://carrd.senpan.cafe` (needs CORS - see Deployment)

## Frontend (Vue 3 + TypeScript + Vite)

The frontend lives in `frontend/` as a Vite project with Vue 3 SFCs, TypeScript,
and Pinia state management. It began as a faithful migration of a legacy
single-file `index.html` + `assets/js/app.js` prototype (since removed; preserved
on a separate branch) and has since grown well beyond it - decomposed into
fine-grained components and stores.

**State (Pinia stores)**: `ui` (toasts, themed confirm dialog, route-loading
flag, realtime connection status), `app` (settings/fonts/active CSS), `auth`,
`player`, `game`, `cards`, `patterns`, `presets` (game templates), `styles`,
`raffles`, `affiliates` (partner establishments), `garapons` (festival lottery
drums + per-player draw links), `stampRallies` (stamp-rally events + cards/logs),
`images` (central image-host categories), `fonts`, `bookclub` (reading lists/items
per club), `carrd` (image-host projects), `announcements` (types + roles +
scheduled embeds), `admin` (sidebar highlight state + per-tab data loads). The root `App.vue` hosts
`<RouterView>` and owns the live stream's lifecycle; `composables/useLiveEvents.ts`
hands each event of the board's or the staff's stream to the stores.

**Routing is Vue Router** (history mode, `router/index.ts`) - real linkable URLs,
not store-driven view switching:

- Public: `/`, `/play/:cardId`, `/raffles`, `/raffles/:id`, `/festival-maps`,
  `/festival-maps/:id` (the published festival maps and one map's interactive
  floor plan), `/festival/:id` (the festival dashboard - see **Festival
  dashboard**), `/garapon/:token`
  (per-player Garapon draw - the unguessable token is the capability, no admin
  auth), `/stamp-card/:token` (per-participant Stamp Rally card, likewise
  token-gated), `/admin/login`, `/admin/register` (hidden - linked nowhere; admins
  share the URL to onboard a new account, which starts inactive).
- Admin: `/admin` (layout, `requiresAdmin`) with child routes grouped into five
  sidebar sections that the route paths mostly mirror - **bingo** (`bingo/{game,
cards,winners-log,patterns,presets}`), **teahouse** ("Senpan Tea House":
  `teahouse/announcements`, `teahouse/affiliates`, `teahouse/tea-rooms`, and one
  `teahouse/bookclub/<slug>` route per registered club), **festival** (`festival/{map,garapon,stamp-rally,ema,ema-requests}`; the
  Raffles manager now lives in this section too, though its route/id stays
  `teahouse/raffles`), **atelier** (`atelier/{fonts,carrd}`), and **system**
  (`system/{settings,themes,images,users}`), plus `no-access` (landing for an active
  account with no granted pages). The admin tabs are **child routes** of the
  `AdminView` layout, so the sidebar/topbar persist while the matched child renders
  the active tab. The per-club book-club routes are generated from the `BOOK_CLUBS`
  registry (`lib/constants.ts`) and all render the single generic `BookClubTab.vue`.
  Routes mirror the nav exactly (no legacy redirects) - an unknown `/admin/*` path
  falls through the catch-all to home.
- Every view + admin tab is a lazy `import()` so heavy deps (the Milkdown
  editor, vue-draggable-plus, markdown-it) split into on-demand chunks. A `router.onError`
  guard recovers from stale lazy-chunk 404s after a redeploy by doing one full
  browser load of the target (sessionStorage-guarded against loops).
- A global `beforeEach` guard enforces auth + **per-page permission**: it
  redirects unauthenticated users to `/admin/login` (with a `redirect` query),
  then checks the matched child's `meta.tab` as the permission key - admins pass
  everything, `system-users` and `system-logs` are admin-only, and a user lacking
  the key is sent to
  their first allowed page (or `/admin/no-access` when they have none). An
  already-logged-in user hitting `/admin/login` or `/admin/register` is bounced
  to their first allowed page. The guard also calls `admin.setTabFromRoute(meta.tab)`
  to sync the sidebar highlight + run that tab's data load. Unknown paths redirect home.
- `lib/api.ts`'s global 401 handler (registered in `main.ts`) redirects to the login
  route with a "session expired" toast when any non-auth request 401s.

**Type sync**: the types of every call are generated from the document
(`frontend/src/api/schema.d.ts`, exported by name from `api/client.ts`), as is the
plugin's client (`plugins/SenpanCompanion/Api/SenpanApi.g.cs`), both **committed** and
regenerated by `task gen`; CI's `generated` job (and `scripts/check.ps1`) fails when a
committed copy is stale. Never edit either by hand. The form models the SPA alone uses
are in `types/api.ts`.

**Library choices** (migrated off CDNs):

- Markdown **rendering** -> `markdown-it`, **lazy-loaded** via `useMarkdown()` (`lib/markdown.ts`); the ~100 KB parser is dynamic-imported on first render (`breaks: true` to match the old marked output).
- Markdown **authoring** -> `MarkdownEditor.vue` wraps **Milkdown (Crepe)** (`@milkdown/crepe`), a WYSIWYG editor built via Crepe's tree-shakable `CrepeBuilder` (only the features used are imported, dropping the CodeMirror/LaTeX bundles). It's still a sizeable bundle, so both the library **and** its CSS are dynamic-imported on mount (kept out of the initial load); it maps its colours onto the app theme variables, is limited to a Discord-safe subset (inline formatting + headings/quotes/lists/dividers/links), and emits **markdown** (what we store and what Discord renders). Formatting lives on a persistent **top bar** (Crepe's `topBar`, not the floating selection toolbar); `buildTopBar` prunes the buttons for features the editor doesn't enable (image/table/code-block/math) plus task lists, and reuses the emptied "block" group for an **emoji button** - an empty group would still render its divider. The emoji picker inserts the literal character, never a `:shortcode:`, which the markdown serializer would escape. Reused by raffles, book-club items, and announcements.
- Drag-and-drop -> `vue-draggable-plus` (SortableJS) for category reorder + pattern reorder/cross-category move
- Theme editor -> structured token editor (`ThemeTokenEditor.vue`): a colour swatch + value per `:root` design token, no free-form CSS (CodeMirror removed). Alpha tokens use the `@ckpack/vue-color` picker; solid tokens the native `<input type=color>`
- Icons -> `@fortawesome/*` **Pro** packages via `@fortawesome/fontawesome-svg-core` + the `@fortawesome/vue-fontawesome` **component** (`<font-awesome-icon :icon="[prefix, name]" />`, registered globally in `main.ts`). Only the icons used are added to the library in `lib/fontawesome.ts`; templates reference them by `[prefix, name]` tuple (`['fad', ...]` duotone, `['fas', ...]` solid, `['fab', 'discord']` brands). Vue owns the rendered `<svg>`, so there is no `dom.watch()`/MutationObserver
- Card PNG export -> `html-to-image` captures the live themed board, then a canvas composites the framed card (`lib/exportCard.ts`)

**Performance / tooling**:

- **Lazy routes**: every view + admin tab is a dynamic `import()` in `router/index.ts`, so heavy deps (the Milkdown editor, vue-draggable-plus, markdown-it) load only when their route is visited - the player/home payload stays small. `manualChunks` (vite.config) keeps shared vendors cached across route chunks.
- **PWA**: `vite-plugin-pwa` (`registerType: 'autoUpdate'`) emits `sw.js` + `manifest.webmanifest`; the SW precaches the app shell and falls back to `index.html` for SPA routes, with `/api/` and `/images/` denylisted. The app (`internal/web`) keeps `sw.js`/`registerSW.js`/`*.webmanifest` out of the immutable cache that the hashed `assets/` get, so updates land. **The precache carries only what a public visitor can reach**: `markAdminOnlyOutput` in `vite.config.ts` walks the bundle graph treating admin chunks as barriers, and everything unreachable (Milkdown, the KaTeX faces, `AdminView`, the `*Tab` chunks, `DataTable`) is dropped from the manifest and left to a `CacheFirst` runtime rule, so an admin still gets it offline after one visit. Do not swap that for a file-name glob - names do not carry the answer (`DataTable` is admin-only; `emojipicker` and `fontawesome` are not). `verifyAdminNotPrecached` fails the build if any of it reaches the precache again.
- **Route progress + loading UX**: a top progress bar (`RouteProgressBar.vue`, driven by `ui.routeLoading` from the router guards) shows during async navigation/lazy-chunk loads; stores expose per-action loading flags (`joining`, `drawing`, `starting`, ...) that drive `LoadingSpinner.vue` + disabled buttons.
- **The one dialog**: `ModalOverlay.vue`, which traps focus, restores it on close, supports Escape, and sets `role="dialog"`/`aria-modal`.
- **Lint/format**: the configs extend **`@jwill89/lint-web`**, shared-tooling's front-end package, installed from `file:../shared/lint/web`. ESLint flat config (`eslint.config.js`: the shared rules, then Senpan's own `senpan/*` blocks) + Stylelint (`stylelint.config.mjs`: `@jwill89/lint-web/stylelint`, with class names held to the naming rules below) + Prettier (`prettier.config.js` re-exports `@jwill89/lint-web/prettier`); `npm run lint` (autofix both) / `npm run lint:check` (no fix, used by CI) / `npm run format`. `vue/no-static-inline-styles` is off for now (see the departures above). Bundle treemap via `npm run analyze` -> `frontend/stats.html`.
- **TypeScript**: `tsconfig.app.json` / `tsconfig.node.json` extend the package's tsconfigs (`strict` with `noUncheckedIndexedAccess`).

**Testing** (Vitest + Vue Test Utils, jsdom):

- Config in `vitest.config.ts` (kept separate from `vite.config.ts` so tests don't load the PWA/visualizer plugins), built on shared-tooling's `vitestDefaults` (`shared/test/web/vitest.js`); `vitest.setup.ts` stubs `<font-awesome-icon>` globally.
- **Coverage floor**: `npm run test:coverage` fails below 35.5% of statements, 25.5% of branches, 28% of functions and 35.5% of lines (the `floor` in `vitest.config.ts`). CI and `scripts/check.ps1` run it.
- Test files are colocated as `src/**/*.test.ts`. Current coverage: `lib/{constants,api,exportCard,theme,datetime,freshness,markdown,feed}.test.ts`, `stores/{player,ui,raffles,cards,patterns,announcements,auth,users}.test.ts`, `components/common/BingoBoard.test.ts`, `components/admin/AdminSidebar.test.ts` (mocks `vue-router`/`@/router` to assert accordion toggling + per-permission section hiding), `components/common/ui/{FormField,DataTable,PaginationBar,ManagerView,ListRow,SubPageHeader,SearchInput}.test.ts`. Backend mirrors this with `internal/auth/password_test.go`, `internal/store/users_test.go`, `internal/server/users_test.go`, the pure rule packages' own tests (`internal/{discord,origins,raffles,affiliates,tearooms,schedule,announcements,anilist}`), and the existing per-domain `*_test.go`. Every domain's operations are tested through `/api/v1` against the document (a public tea-room read for its CORS headers, the preflight and the 304; the garapon draw's cap, the stamp card's guesses, the media uploads over a temporary web root, the public font gate). The festival's, uploads' and fonts' pure rules have their own tests (`internal/{festival,uploads,fonts,media}`).
- Run: `npm run test` (one-shot), `npm run test:watch`, `npm run test:coverage` (v8, with the floor - what CI runs).
- Patterns: for a store or view, mock `@/api/client`'s `api` alone (spreading `importOriginal()` keeps the real `unwrap`), each fake answering `{ data, response }`; Pinia stores use `setActivePinia(createPinia())`; components use `mount()` from `@vue/test-utils`. Pure helpers tested directly (e.g. `exportCard.ts` exports `parseInlineRuns`/`parseDetailParagraphs` for this).

**Public routes**: `/` - `/play/:cardId` - `/raffles` - `/raffles/:id` - `/festival-maps` - `/festival-maps/:id` - `/festival/:id` (festival dashboard) - `/garapon/:token` (token-gated) - `/stamp-card/:token` (token-gated) - `/card-requests` (submit a self-designed bingo card for admin approval) - `/admin/login` - `/admin/register` (hidden).
**Admin sections** (`adminSection`, sidebar highlight): `bingo` | `teahouse` | `festival` | `atelier` | `system`.
**Admin tabs** (`adminTab` / route): `bingo-game` - `bingo-cards` - `bingo-winners-log` - `bingo-patterns` - `bingo-presets` - `teahouse-announcements` - `teahouse-affiliates` - `teahouse-tea-rooms` - `bookclub-<slug>` (one per registered club, e.g. `bookclub-yaoi`, `bookclub-yuri`) - `festival-garapon` - `festival-stamp-rally` - `festival-ema` - `festival-ema-requests` - `teahouse-raffles` (shown in the Festival section, id unchanged) - `system-settings` - `system-themes` - `system-images` - `system-users` (admin-only) - `system-logs` (admin-only) - `atelier-fonts` - `atelier-carrd`. Each tab id (except `system-users`/`system-logs`) doubles as its **page-permission key**.

**Player features**:

- Join by board ID; see bingo board, called numbers grid, and active win patterns
- Manually stamp cells (click to toggle); stamps persist in `localStorage` keyed by `stamps_{cardId}_{gameId}`
- Stamp customization: shape (blank default, heart, star, smiley, etc.), color (7 options), opacity slider, **custom uploaded image** (stored as a data URL in `localStorage`)
- **Export/save the board as a PNG card** (`lib/exportCard.ts`): captures the live themed `.board-wrap` via `html-to-image` (preserving theme, emoji + custom-image stamps), then composites it into a framed card (title/logo header + game-details footer) on a canvas and downloads it
- Real-time updates over the board's event stream (`GET /api/v1/board/{id}/events`: draws after the draw delay, game start/end, theme and settings changes, the half-time mini-game alert, yoever)
- **Live-game feedback** (ambient only - never tracks the player's own board, by design, to preserve player agency): a "Last Called" announcement banner for the most recent draw, an opt-in draw **chime + vibration** (`lib/sound.ts`, persisted in `localStorage`), a **Live / Reconnecting** connection badge (off `ui.feedStatus`), and an end-of-game thank-you summary
- **Yoever** ("It's Yoever") - a shared, opt-in reaction during a live game: a player taps the button (`POST /api/v1/game/yoever`), and every connected client plays a sound + shows a bouncing captioned overlay (`YoeverOverlay.vue`) while admins watch a "Yoevers: N" counter climb. State is **in-memory only** on `game.Service` (no table): a per-card cooldown (`yoever_cooldown_seconds`, default 180) is enforced server-side (429 + `Retry-After`) and mirrored client-side; admins toggle it via `PATCH /api/v1/game {yoeverEnabled}`; it is published as `yoever` / `yoever_switched`. Client-local mute/opt-out only.
- **Konami easter egg** (`KonamiEgg.vue`, mounted by `PlayerView`) - ↑ ↑ ↓ ↓ ← → ← → B A clears every stamp (the same `clearAllStamps()` the Clear Board button calls) and swells Drani's grin over the screen with "Oi, what'd you think was gonna happen?" before it fades out. Purely client-side: no store, no server, no broadcast. Keys match on a rolling ten-key window (a fumbled repeat still resolves) and keystrokes aimed at a text field are ignored. Its art is a **bundled import** (`src/assets/images/DraniGrin.webp`), warmed on mount - see the asset rule below
- **Public Stamp Rally sign-up** (`StampRalliesView` -> `StampRallySignupView`, plus `StampLookupView` at `/stamp-lookup`): a participant issues themselves a card for any rally an admin opted in (`public_signup`, off by default). A rally with an **open linked Garapon** also issues that drawing link, on its **own separate token**. The two used to share one string, which made the shareable card link a spendable drawing capability and meant any recovery of a card link handed the draws over with it; viewing and spending are now separate secrets. Cards issued before the split still share one token and are left alone on purpose, so links already in participants' hands keep working. Names are unique per rally (trimmed, case-insensitive); a repeat is a 409 pointing at the lookup page. The lookup matches the WHOLE name and returns an empty 200 on a miss, so it can't be walked to harvest links. Both endpoints are IP rate-limited (`RallyLimit`); **only sign-up is Turnstile-gated** - the lookup is not. The lookup returns the card token and how many garapon draws REMAIN, never the drawing token: it is keyed on a character name, which is public, and a draw cannot be undone. The drawing token is issued once, at sign-up, and lives on the participant's device or with staff. `POST /api/v1/garapon-players/{token}/draw` has its own limiter (`GaraponDrawLimit`, counted before the token is looked up) and the stamp card's `stamp` counts only wrong passwords against its own (`StampGuessLimit`)
- The event stream reopens with back-off on a drop and reads the board again whenever it opens
- **Hide Bingo** (`hideBingo`, System -> Settings): takes Join Bingo AND the Custom Card request off the public home page for a stretch when no game is running - the two are one feature and hide together. A second, narrower flag (`hideCustomCards`) takes ONLY the Custom Card request off, for when staff have stopped taking requests but games are still running; the home page combines them (`showCustomCard = showBingo && !hideCustomCards`), so the wider switch still wins and neither flag writes the other. They are stored as `'0'`/`'1'` and served as booleans; the home page reads them through the app store's `hideBingo`/`hideCustomCards`. Entry points only: `/play/:cardId` and `/card-requests` stay reachable so flipping it can't strand a player mid-game or lose a pending request. The home page waits for `app.settingsLoaded` before rendering either card - the defaults say "visible", so rendering first would flash the thing being hidden and leave it clickable - and shows a "nothing is running" line when bingo is hidden with no open raffle or rally either.
- Browse open raffles from home page (card shown only when open raffles exist); view raffle detail with prize image, markdown description/rules, and sign-up form (character name, world, number of entries); after sign-up see confirmation with total cost and sign-up instructions
- **Raffle entry modes** (`raffles.entry_mode`, see **Raffle entry modes** below) change what the detail page offers: `single` and `custom` show the sign-up form (a `custom` raffle also lists its per-entry price ladder up front, with the running total for 2, 3, ... entries), while `details` shows the sign-up instructions inline INSTEAD of a form - it is published for reference and entered elsewhere
- View closed raffles with winner announcement and total entry count

**Admin features**:

- Per-user account login (username + argon2id password, session-based auth, 24-hour cookie); accounts are activated and granted per-page access by an admin (see **Authentication & authorization**)
- **Game tab**: start game (select patterns with category filter + search, or apply a saved **preset**), draw numbers (optional player delay 0-60s; **press `Space`/`Enter` to draw**), live "Live" badge + elapsed-time clock, see called numbers, see winners; click winner ID to verify card with pattern-hit highlighting; frequent winners alert (3+ wins in 12h); end game with winner confirmation modal. **Auto-run**: a New Game "Auto-draw numbers" toggle + "Time Between Calls" interval (also on presets) starts the game auto-drawing; a live Auto-Draw on/off toggle + interval selector adjust it mid-game (never touches the preset). The server-side scheduler draws the first number immediately when auto is switched on, then draws every `interval` (the player draw delay only lags when each number reaches players - it never stretches the admin's cadence); it pauses at half-time (the prompt is server-driven - `halftime_prompt` - so it fires for auto draws too and on every console, including one that opens mid-way; the first answer is `halftime_resolved {minigame, by}`, which closes the prompt everywhere, and a second answer is a 409), stops the moment a winner is recognized, and switches off if the admin **manually draws** (taking over). Whenever auto turns off for any reason the scheduler cancels its pending draw, and the loop's draw (`drawAuto`) re-checks the enabled flag under the draw lock so a disable racing a scheduled fire can't leak a stray number. Auto state syncs via the `auto_draw` event
- **Cards tab**: generate cards (1-500), view as chips with player name indicators, click to preview board, edit player name/details, delete individual or all. Also reviews **custom card requests**: the public `/card-requests` page lets a visitor hand-build a 5x5 board + pick a 6-char ID / character name / world and submit (no login; IP-rate-limited + Turnstile) -> stored as a `pending` card; a `bingo-cards` admin **approves** it (`POST /api/cards/{id}/approve`), making it a live, auto-**protected** card (protected cards are spared by Delete-All). Submissions validate the ID/board and reject duplicate IDs or identical boards (409).
- **Senpan Tea House -> Tea Rooms tab** (`TeaRoomsTab.vue` + `TeaRoomFormTab.vue`, perm `teahouse-tea-rooms`): manage bookable **tea rooms** - name/subtitle, a unique public **room number**, per-half-hour **gil** cost, hashtags, markdown description, status flags (seasonal / open / lockable / discounted), a **lock** with an optional unlock time (a background sweeper lifts it and broadcasts `tea_room_unlocked`, which the plugin and every open admin page turn into a sticky pop-up + chime for accounts holding `teahouse-tea-rooms`; the sentence is built in two places - `teaRoomUnlockedMessage` and `TeaRoomsTab.UnlockedMessage` - that must agree), a library **image** + embed accent colour; drag-reorder. Each room can be **posted as a Discord embed** to the page's webhook (`GET`/`PUT /api/v1/tea-rooms/webhook`, `tearoom_webhook_url`, shown on the page and guarded by it). A public read API (`GET /api/v1/tea-rooms/public[/{number}]`, camelCase, `no-cache` with an ETag and 304) lets external Carrd sites render live availability + pricing. **CORS** is an allowlist, not `*`: an `Origin` on the `teaRoomOrigins` setting (System -> Settings, "Allowed sites for the public tea-room API"; a `*.` entry covers any subdomain but not the apex) is echoed in `Access-Control-Allow-Origin`, never with credentials, and `Vary: Origin` is always sent; the preflight is answered for allowed origins (`GET`, `If-None-Match`, max-age 600). The Carrd pages must therefore be served from an allowed origin. A `PATCH` with `locked: true` sets the lock afresh (until `lockedUntil` if sent beside it, otherwise until staff unlock it); `lockedUntil` alone moves a locked room's unlock time.
- **Patterns tab** (`PatternsTab.vue`): one manager merging the former Categories/New/Edit tabs - category-grouped collapsible drag-reorder list with search + category filter; "+ New Pattern" (5x5 grid editor, duplicate detection) and "Manage Categories" (a `DataTable` of categories with Edit/Delete; add/edit opens a form with a Title + a Position dropdown - "At the beginning" / "After X" per category, plus "Keep current position" when editing - applied via the bulk-reorder endpoint) as Back sub-pages
- **Game tab pattern picker** (`GameTab.vue`): when starting a new game, patterns render exactly like the Patterns manager - collapsible category groups (non-draggable checkboxes) with search + category filter + select-all-visible - reusing the patterns store's `patternsByCategory` + shared collapse state
- **Presets tab**: CRUD reusable game templates (a named set of win-pattern IDs + pre-written markdown game details); selectable on the Game tab to auto-apply patterns + details when starting a game
- **Winners Log tab**: paginated table of past winners with sorting and per-page controls; delete an individual entry (per-row trash button) or **Delete All** to clear the log (both confirm first)
- The sidebar shows a **count** beside Manage Cards, Festival Map, Raffles, Garapon and Stamp Rally (open items, or published maps). The counts read `openRaffles` / `openGarapons` / `openRallies` / `publishedMaps` from their stores, which `AdminView`'s mount preloads (permission-gated) - without that preload a count reads 0 until its own tab is visited.
- **Senpan Tea House -> Raffles tab** (`RafflesTab.vue`): one manager (replacing the former New/Open/Closed tabs) - **Current Raffles** (every non-closed raffle) as image cards with a corner status icon (calendar-clock when it opens later, red calendar-circle-exclamation when its open window has passed), then a searchable + paginated **Closed Raffles** table (title, winner, open period, and the gil collected from paid entries - `Raffle.winner_name`/`paid_total`, admin-only aggregates joined in `listRafflesAdmin`) with a **Copy** action that seeds a new raffle from a past one (`copyRaffleForm`). The form also carries a **"Where to Pay"** image (`pay_image`), shown to players under the sign-up instructions - the raffle counterpart to a stamp rally's Where to Redeem. Detail (winner pick/verify, **manually add an entry - optionally paid, with an amount waived**, record/clear payments via `RafflePaymentModal.vue`, delete; read-only when closed) and the create/edit form (`RaffleFormTab.vue`, emits `saved`/`cancel`) open as Back sub-pages. The form's **Entry Type** picker swaps the cost controls per mode, and in `custom` mode edits the price ladder (a `.stack` of rows; its length sets `max_entries`). Weighted random winner pick; **Close / Reopen** (a raffle with no entries and no winner - every details-only one - can't reach verify-winner, which closes only as the last step of confirming a winner, so without this it stays on the public list forever; neither verb touches a winner already recorded); delete raffles. **Duplicate** (detail toolbar, and the Copy action on the closed table) seeds the create form from an existing raffle - the same `copyRaffleForm` the Garapon and Stamp Rally managers mirror, see **Duplicating an event** below.
- **Senpan Tea House -> Announcements tab** (`AnnouncementsTab.vue`): manage **announcement types** (a named Discord channel webhook), **taggable roles** (a friendly name + Discord role ID - managed like types under "Manage Roles"), and **announcements** authored as Discord embeds (title, markdown details, accent colour, optional location + event window, image upload/reuse, an optional **role tag** posted in the message content above the embed - _Do Not Tag_ / _@everyone_ / a managed role, since mentions inside an embed don't notify - and **up to 5 Discord link buttons** - label + optional emoji + URL - rendered as an action row beneath the embed; sanitized server-side and stored as JSON in `announcements.buttons`). Post manually (**send now**) or on a schedule (once / daily / weekly / monthly, anchored to an IANA timezone so times survive DST); **skip next** occurrence (`POST /announcements/{id}/skip`, `{count?}`, 0-52, 1 when left out); deleting a type or role still in use is a 409 naming the count; the form doesn't send `active`, which follows from the schedule; the background scheduler posts due items. Client-side search + type filter. (Link buttons require a webhook whose target supports message components.) Book-club meeting/event posts are authored here too (the retired per-club "event posts" feature was merged in; its webhooks migrated to dedicated "\<Club> Book Club Events" types).
- **Senpan Tea House -> Book Club tabs** (`BookClubTab.vue`, one route per registered club): manage **reading lists** + their items - add items manually or pull them from **AniList** (search/by-id proxy that prefills title/summary/cover/format/genres/chapters/source), drag-reorder, upload covers; **publish** a list to the club's Discord channel (one embed per item; an empty list is a 409, and a failure part-way says "Posted N of M, then: ..."). The club's webhook is a field on its page (`GET`/`PUT /api/v1/book-clubs/{club}/webhook`). The guard answers 401 for nobody, 404 for an unknown club and 403 without `bookclub-<slug>`; the cover upload is `POST /api/v1/book-clubs/{club}/covers` (it tells no page, since it changes no reading list).
- **System -> Settings tab**: app title, bingo join prompt, default draw delay, frequent-winner threshold/window, the **header/board font** picker (combo box with optgroups for uploaded fonts + Google Fonts, optional Google Fonts API key, live preview), the AniList API URL, and the allowed sites for the public tea-room API (the per-club webhooks are on each Book Club page)
- **System -> Themes tab**: CRUD themes via a structured **token editor** (`ThemeTokenEditor.vue` - a swatch + value per `:root` design token, grouped, with a collapsible live preview), activate/deactivate live. A theme stores a **token map** (not CSS); the server generates the `:root{}` stylesheet (`store.TokensToCSS`). Alpha tokens (modal/shadow/glow) use the `@ckpack/vue-color` picker; solid tokens the native colour input
- **System -> Users tab** (`UsersTab.vue`, **admin-only**): manage accounts - activate/deactivate pending registrations, grant/revoke admin, edit the per-page permission set (checkbox grid grouped by section, derived from `ADMIN_PERMISSIONS`), set a user's password - all in one `PATCH /api/v1/users/{id}` of what changed - and delete accounts. The seeded `admin` account is protected (its destructive actions are hidden in the UI and rejected server-side). Separately, **every** logged-in account can rotate its own password via the **Change Password** modal in the admin topbar (`AdminView.vue` -> `PUT /api/v1/me/password`)
- **Atelier -> Font Upload tab** (`FontsTab.vue` + `FontEditModal.vue`): upload font files (`.ttf/.otf/.woff/.woff2/.eot`) to `<webRoot>/fonts/`; files sharing a base name are variants of ONE font (a font with no WOFF2 gets one auto-converted). The table is slim - **CSS Name** (the `font-family` used by the kit/app/picker), **Serves** (the served format; ✦ = the converted copy), **Modified**, actions - and the **Edit modal** holds the rest: CSS name, served-version picker (any variant type or Auto), the font's **per-font allowed sites**, and the files themselves (per-file rename/delete, sizes). The **live preview** (type any text, pick a font) can switch between the font's actual formats. Per-row **Copy URL** copies the served variant's tokenized URL (rotates every 1-2 weeks - the kit is for permanent embeds); an Embed panel shows the `kit.css` snippet. The app's own origin is always allowed implicitly (the font picker never breaks).
- **Atelier -> Carrd Upload tab** (`CarrdUploadTab.vue`): manage image-hosting **projects** (folders under `<webRoot>/carrd`, each with a human-readable title) and nested sub-directories; upload images plus `.mp3`/`.mp4` (same-name **overwrites** so external Carrd sites pick up new versions), copy public URLs, delete files/dirs/projects. Served cross-origin from the carrd vhost.

**Key UI patterns**:

- **Admin sidebar = accordion** (`AdminSidebar.vue`): section headers (Bingo / Tea House / Festival / Atelier / System) are pure **accordion toggles** - clicking one shows/hides the items it contains and never navigates; only the items navigate. Expanded sections are local component state (`openSections`, a reactive `Set`) and toggle **independently**, so any number can be open at once. A section auto-opens when navigation makes it active (so the highlighted item stays visible); otherwise sections are collapsed/expanded freely without changing the route. A whole section is hidden when the account can access none of its pages.
- Toast notifications for success/error feedback (`ui` store)
- Sticky alerts (`ui.alert` / `ui.dismissAlert`) for something a person has to go and act on: they stack in the same top-right column as the toast (`ToastNotification.vue`) and stay until dismissed. Use them sparingly - the toast has one slot and a 5.5s timer, so it is the right home for "Saved", and the wrong one for anything that must survive the next toast. A page that may chime without being touched calls `armAudioOnFirstGesture()` (`lib/sound.ts`) on mount, since browsers - iOS outright - play nothing before a gesture
- Winner toast only fires on new winners (compares count before/after draw)
- The event stream reopens with exponential back-off (1s -> 2s -> 4s -> 8s -> 16s, no attempt limit) and checks again when the tab wakes
- vue-draggable-plus handles drag-and-drop for patterns and categories (replaces the old manual HTML5 DnD placeholders)

### CSS conventions

Enforced by `npm run lint:conventions` (`frontend/scripts/check-conventions.mjs`), which
runs in `scripts/check.ps1` and CI. Stylelint checks the CSS itself (unknown
properties, deprecated values, class-name shape) and Prettier formats it, but neither
knows the rules below, so without that script none of this is checked.

**The two tiers.** Shared styles are the default; a `<style scoped>` block is the
exception and needs a reason. Scoped CSS is for what is genuinely one-of-a-kind about
_this_ component - its layout and its own bespoke pieces. Anything that describes a
_kind of thing_ (a card, a dropzone, a row, a badge, a form control) belongs in
`assets/styles/` under a role name, even the first time it is written.

The promotion trigger is **the second occurrence**: the moment a rule is about to be
written a second time, in any file, it becomes a shared object instead. Do not wait for
a third. Copying a block between two components and renaming its prefix is the specific
move that produced ~980 duplicated lines and the media-manager, sub-panel, link-button
and inline-checkbox clones that had to be unpicked later.

Scoped CSS is not free: it compiles to `[data-v-xxx]`, which raises specificity to
(0,2,0) and quietly outranks the global object it duplicates, so the two drift without
anything failing. Since the compiler already guarantees uniqueness there, the naming
rules in a scoped block exist for _readability_, not collision safety - the discipline
differs by layer:

| Layer  | Where                                                               | Rule                                                           |
| ------ | ------------------------------------------------------------------- | -------------------------------------------------------------- |
| Global | `assets/styles/*.css`, loaded in the cascade order set by `app.css` | Full discipline: shared objects, parts, modifiers, state       |
| Scoped | `<style scoped>` in 47 SFCs                                         | The component's name as prefix, plain kebab-case, nothing more |

Global is currently ~4,190 lines and scoped ~2,860 across 47 SFCs (40 other SFCs carry
none). Scoped total should trend **down**; a change that grows it wants a look.

**Naming.** The shared naming rules hold: `--` for a modifier (`.badge--accent`), `is-` for state (`.is-active`, never a bare `.active`), a scoped component's classes carry its full name (`.announcement-badge`; the `ann-`, `cl-` and `tp-` abbreviations are retired), and a class names its role or the token it applies, never its look (`.text-highlight`, not the retired `.text-gold`). Senpan adds:

1. **Shared objects are role-named with no prefix** and live in `utilities.css` /
   `components.css`: `.card`, `.badge`, `.chip`, `.field`, `.list-row`, `.data-table`.
2. **A part joins to its parent with `-`**, shared objects included: `.list-row-title`, `.empty-state-icon`, `.data-table-wrap`. BEM's `__` is not used, which departs from the guide's BEM for reusable components (see the departures above).

**Deliberate exceptions**: `.btn-sm` / `.btn-lg` (an established pair), and
`ball-swatch-{sm,md,lg}` (bound from a prop, so the class and the prop values would
have to change together).

**One way to write a participant.** Four systems record people - custom cards,
raffle entries, stamp rally cards and garapon links - and all four store a
**character name and a home world in two separate fields**. The world is always
**picked** (`components/common/ui/WorldPicker.vue`, backed by `FF14_WORLDS`), never
typed: a free-text world spells the same person three ways, which makes them
unmatchable across systems and, in a raffle, splits one entrant's tickets across
rows. `model.ParticipantLabel` / `SplitParticipantLabel` (Go) are the only place
those two fields become the display string `"Name @ World"` and back; a handler that
receives one composed string splits it on arrival rather than storing something no
other system can match. Schema v66 (`migrateParticipantWorlds`) added the column to
the two stamp/garapon tables and backfilled by splitting on the LAST `" @ "`; rows
with no separator keep the whole string and an empty world. Display is unchanged -
a public stamp card still reads `"Name @ World"`.

**Festival map label size.** A pitch's label is sized **once for the whole plan**, and
a pitch may only trim that size - never derive its own. Three declarations in
`mapeditor.css` do it together, and the gate checks them as a set:

| Where              | Declaration                        | Why there                                                                                              |
| ------------------ | ---------------------------------- | ------------------------------------------------------------------------------------------------------ |
| `.map-canvas`      | `container-type: inline-size`      | The plan is the yardstick. Only the inline axis: the canvas takes its **height** from the background image in flow, and `size` containment collapses it. |
| `.map-stall`       | `font-size: 1.3cqw`                | An element's own `container-type` does not apply to its **own** declarations, so `cqw` here resolves against the canvas. On the label it would resolve against the pitch. |
| `.map-stall-label` | `max(0.34rem, min(1em, 42cqh))`    | `1em` is the shared size; the pitch's own height is only a **ceiling**, engaging below roughly 5% of the map height. |

Sizing the label straight off its box - as `clamp(0.34rem, 26cqh, 0.85rem)` did - made
every pitch compute its own number: on one plan a tall pitch and a short one drew their
names at 13.6px and 5.5px, and dragging a resize handle rescaled the text the whole way.
This cannot be covered by a vitest file: container-query units need real layout and jsdom
resolves neither `cqw` nor `cqh`, so the stylesheet source is what gets asserted.

**Every object keeps its base rule.** `.x:hover`, `.x.is-y` and `.x .part` style a
state or a part of `.x`; if nothing defines `.x` itself, they modify an object that
does not exist. The checker fails on a class that only ever appears as the head of a
compound selector. Exempt: `is-*` state, and co-class modifiers - a class that always
shares its `class` attribute with another (`class="opacity-slider sound-volume"`)
modifies _that_ object and rightly has no base. This rule exists because a bulk
rule-dropper matching only `^\.name {` was once run over all of `src/` and stripped the
base rules out of `utilities.css`, leaving their `:hover` compounds behind - the file
still looked populated, every test passed, and the objects rendered unstyled.

**Scoped CSS may only shrink.** `frontend/scripts/scoped-css-baseline.json` records the
scoped line count of every SFC, and the checker fails when one grows. A file already in
the baseline is grandfathered at its current size - legacy bulk is tolerated, growth is
not. A file _not_ in the baseline is new work and may not exceed **120** scoped lines,
because new work has no legacy to plead. The total is pinned too, so rules cannot be
shuffled between files for free.

After a reduction, run `npm run lint:conventions:baseline` and commit the result. Going
_under_ baseline never fails - it prints a note - because a check that failed the moment
you improved something would only teach people to re-baseline without reading. This
exists because scoped CSS reached near-parity with the global stylesheets (3,360 vs
4,013) before anyone noticed, and unpicking it took days.

**A themeable token must have a consumer outside the theme editor.** The editor renders
every token by definition, so it cannot vouch for one; a token only it references is a
knob in the UI that changes nothing in the app. `--accent-hover` and `--accent-2-hover`
failed the other way - dropped from the stylesheets while still offered in the editor -
and stayed broken until this pass.

**No dead classes in markup.** A class hard-coded in a static `class="..."` must be
styled by something. The checker fails otherwise. Exempt: a class referenced from a
`.ts` file, which is a deliberate test or JS hook rather than dead markup
(`ChangelogModal.test.ts` queries `.changelog-entry`). Only static attributes are
checked - `:class` is a JS expression and cannot be parsed reliably enough to gate on.
This found twenty, and two of them were live defects rather than clutter: `mt-4` /
`mt-10` were never defined, so those elements got no margin at all, and `btn-secondary`
was never defined, so three Server Logs buttons rendered with no fill.

**Tokens.** `--radius`, `--radius-media`, `--body-font` and `--header-font` live in
`tokens.css` but are **not** themeable and must stay out of the themeable copies. Every other `:root` token must appear in all
three sources, which the checker verifies in both directions:

- `frontend/src/assets/styles/tokens.css` - the declarations themselves
- `frontend/src/lib/theme-tokens.ts` - the editor's allowlist + labels + defaults
- `backend/internal/themes/themes.go` - `TokenOrder`, used to generate the `:root{}`

The checker also fails on any `var(--x)` that resolves to nothing, because an
undefined custom property silently falls back to `inherit` - invisible until someone
notices a wrong colour.

## FFXIV plugin (Dalamud)

`plugins/SenpanCompanion/` is a **Final Fantasy XIV Dalamud plugin** (C#/.NET 10,
ImGui) that acts as a **second admin UI** over the same Go server - published as
_Senpan Admin Companion_ (Dalamud InternalName/DLL `SenpanCompanionAdmin`; the
folder, csproj, and C# namespace stay `SenpanCompanion`). The server remains the
single source of truth: every plugin action hits the same REST API the
website uses, and its changes reach all web clients through the live event streams. FFXIV gives plugins no
client<->client networking, so the server _must_ stay the relay - the plugin never
talks peer-to-peer.

- **Auth - personal access token.** The plugin sends `Authorization: Bearer pat_...`
  on every call and on the live event stream; the header is the only way in (there is
  no `?token=` fallback), keeping the token out of access logs/history/Referer. The
  server address must be https, or plain http only for a server on this PC
  (`SenpanApi.ApiBase`), so the token is never sent in the clear. It reads its account
  from `GET /api/v1/me`. The
  server URL + token are entered via
  `/senpan config` (PAT support shipped in FE+BE v1.5.0) and the **token is encrypted
  at rest** with the Windows Data Protection API (DPAPI, CurrentUser scope, see
  `Services/TokenProtector.cs`) - the config JSON stores ciphertext, not the
  plaintext token; a pre-encryption plaintext token migrates on first load.
  Permissions are enforced server-side exactly as for the web admin (`bingo-cards` /
  `bingo-game` / `teahouse-raffles`). `/senpan` opens the main window.
- **The client is generated** from the committed document (NSwag 14.7.1, a local tool
  in `plugins/dotnet-tools.json`; `nswag.json` lists the operations it calls), as
  Kindred Nights' plugin has it: `Api/SenpanApi.g.cs`, Newtonsoft, every schema a class.
  `Core/SenpanApi.cs` is its partial: each request goes to the configured server's
  `/api/v1/` with the token of the moment, a time stays text (`DateParseHandling.None`),
  and an enum value newer than the build reads as -1 instead of refusing the answer. A
  refusal is shown in the server's words (`Problems.Say`: each field a validation names,
  else the problem's `detail`).
- **The live event stream** (`Services/LiveConnection`, over `Core/Feed`): the staff's
  `GET /api/v1/events` with the Bearer header. A drop reopens after 1 to 16 seconds
  (`Core/Backoff`), sending `Last-Event-ID`; 70 seconds without even a ping is a drop; a
  401 or 403 stops it until the settings change (the status bar says so). On every open
  the game and the cards are read again, and on `resync` every page; `resource_changed`
  marks the page that shows it stale (`MainWindow.OnResourceChanged`). The game page
  follows `draw`, `game_started`/`game_ended`, `auto_draw`, `yoever`/`yoever_switched`,
  `draw_delay` (the delay is the one every console shares, kept by the server),
  `game_details`, `halftime_prompt` and `halftime_resolved`: a prompt answered elsewhere
  closes here, and a prompt still waiting when the page opens (the game's `halftime`) is
  shown.
- **Feature parity (admin subset).** Bingo: create named cards
  - delete / delete-all; game lifecycle (start with pattern IDs, draw with the shared
    delay, half-time, end naming the game and its winners); live draws + winner alerts
    over the event stream. Raffle: pick an open
    raffle, add / delete entrants, record payments (`…/settle`, `…/unsettle`),
    pick->confirm a winner, the server's pending winner from any console (no raffle
    creation). The Paid cell opens a popup carrying the same settlement the website
    offers - tickets covered (for a part payment) and gil waived - because both need
    a number typed, which a checkbox has nowhere to put. A **details-only** raffle
    takes no sign-up here: the Add entrant form is replaced by a note, and the draw
    says it runs over every entrant rather than paid tickets. A **nearby-players** picker (`IObjectTable`) prefills name + world.
- **Threading model (critical).** Dalamud has no SyncContext, so `await`
  continuations resume on the thread pool. **Every UI-state write from an async op
  must go through `Apply(() => ...)`** (`Plugin.Framework.RunOnFrameworkThread`),
  which marshals back to the framework thread - otherwise it races the render loop
  - live-connection handlers. Tab panels extend `Windows/TabBase` (shared `Run`/`Busy`/`Status`,
    `EnsureLoaded`/`MarkStale` with retry cooldown, `LoadAsync`); the card list is the
    shared `Services/CardCache` (coalesced refresh, framework-thread swap).
- **Game interop is minimal + ToS-sensitive.** `Chime` is a synth WAV via
  winmm (no game interop); `ChatSender` /tell uses
  `RaptureShellModule.ExecuteCommandInner` - the only game interop, and opt-out.
- **Rolls tool (`Services/RollTracker` + `Windows/RollsTab`).** A **permission-free,
  server-independent** helper: `RollTracker` subscribes to `IChatGui.ChatMessage`,
  keeps `XivChatType.RandomNumber` (`/random` - `/dice`) lines **in memory only** (no
  persistence, no server), and wipes them on `IClientState.Logout` + dispose. Read-only
  observation - no game interop.
- **Timed Text Macros (`Services/TimedMacroRunner` + `Windows/TimedMacrosTab`).** Another
  **permission-free, account-free** tool. `TimedTextMacro` records (persisted in
  `Configuration.TimedTextMacros`) repeat a Say/Yell/Shout message on an interval with an
  optional send cap. `TimedMacroRunner` drives them off `IFramework.Update`, sends via
  `ChatSender.SendChannelMessage` (split by `TellComposer.SplitPlain`, one part/second),
  and saves `SendsCompleted` after each fire; **run state is in-memory only** so macros
  reload **stopped**, and `IClientState.Logout` stops them all. Two escape hatches sit
  beside Start: **Test send** (`SendOnce`) puts the message out once without touching
  `SendsCompleted` or the schedule - a test is not one of the sends the operator asked
  for, and counting it would eat a send cap - and **Reset** (`ResetProgress`) puts the
  count back to 0, offered whenever there is progress to clear rather than only at the
  cap (an uncapped macro never completes, and a capped one is often stopped part-way and
  wanted fresh next time). Reset is refused while running, where a live countdown and
  the "N of M" would otherwise disagree. This is `/say`-`/yell`-
  `/shout` **game automation on a timer** - operator-initiated and opt-in per macro, but
  more automation than the single-shot tells; keep the ToS caveat in mind.
- Both account-free tools are reachable **without a token**, so `MainWindow.Draw` renders
  the sidebar even before setup (the suite pages stay hidden until an account connects).
- **Build.** `<Project Sdk="Dalamud.NET.Sdk/15.0.0">` supplies the `net*-windows`
  TFM and references Dalamud from the local XIVLauncher install; needs the **.NET 10
  SDK** + XIVLauncher/Dalamud. `dotnet build -c Release` emits
  `bin/Release/SenpanCompanionAdmin/latest.zip` (DalamudPackager); `bin/` + `obj/`
  are gitignored. **0/0 warnings is enforced - keep it that way.** The .NET
  analyzers (`latest-recommended`) and the C# code style come from shared-tooling's
  `lint/dotnet`, imported by `plugins/Directory.Build.props` and reported as build
  warnings; the plugin's two exceptions (braced `case` blocks not indented a level,
  classic `[DllImport]` for the winmm and DPAPI P/Invokes) sit below the project marker
  in the root `.editorconfig`. Note ImGui's namespace is `Dalamud.Bindings.ImGui`
  (not ImGuiNET).
- **Distribution.** A **custom Dalamud repo**, not the official list:
  `plugins/pluginmaster.json` + `latest.zip` are hosted at
  `https://apps.senpan.cafe/plugin/...` (publish with
  `scripts/deploy.ps1 -Target plugin`, or `task deploy`). Bump `AssemblyVersion` +
  `LastUpdate` in `pluginmaster.json` per release; the deploy reads the published version first and stops before building
  anything when the new one is not higher, since Dalamud updates to nothing else. A
  Dalamud API bump means bumping `DalamudApiLevel` (currently **15**).

## Shared tooling

The lint configs, the CI workflows, the deploy module and the repo files are
**shared-tooling**'s (`https://github.com/jwill89/shared-tooling`), held as a git
submodule at `shared/` and pinned to one release. `shared/README.md` says what each
folder holds; `shared/PLAN.md` says why. What Senpan takes from it:

- **Frontend:** `@jwill89/lint-web` (ESLint, Prettier, Stylelint, the tsconfigs) from
  `shared/lint/web`, and `vitestDefaults` from `shared/test/web/vitest.js`.
- **Backend:** the golangci-lint template in `shared/lint/go`, which writes
  `backend/.golangci.yml`; Senpan's own rules are in `backend/.golangci.local.yml`.
- **Plugin:** `shared/lint/dotnet` (the analyzers and the C# code style), imported by
  `plugins/Directory.Build.props`.
- **Scripts:** `shared/lint/pwsh` (PSScriptAnalyzer's settings and
  `Invoke-ScriptLint.ps1`, which lints every tracked script), and the SharedDeploy
  module in `shared/deploy/pwsh` that `scripts/deploy.ps1` is built on.
- **Repo:** `.editorconfig` and `.gitattributes` from `shared/repo`
  (`Sync-RepoFiles.ps1`), `.markdownlint-cli2.jsonc` extending
  `shared/lint/markdown`, and the shared tasks `Taskfile.yml` includes as `shared:*`.
- **CI:** the reusable workflows `.github/workflows/ci.yml` calls (see **Continuous
  integration**).

A rule only Senpan needs goes in its own file: `eslint.config.js`'s `senpan/*` blocks, `backend/.golangci.local.yml`, or below the `# ---- project: lines below this one are this repo's own ----` marker in `.editorconfig` and `.gitattributes`. The shared rules for the submodule, the files it writes and taking a release are in `shared/standards/docs-and-agents.md` and `code-style.md`.

## Developer commands

Prerequisites: Node 26 (`frontend/.nvmrc`), Go at the version in `backend/go.mod`, and optionally [Task](https://taskfile.dev). The plugin needs the .NET 10 SDK and a local XIVLauncher/Dalamud install, and the scripts gate needs PSScriptAnalyzer 1.25.0 (`Install-PSResource PSScriptAnalyzer -Version 1.25.0 -Scope CurrentUser -TrustRepository`).

```powershell
# -- First time, from the repo root --
git submodule update --init         # shared-tooling at shared/; npm ci, check.ps1 and deploy.ps1 all need it
# The Font Awesome Pro packages install from Font Awesome's registry with the account's
# token, kept in ~/.npmrc (never in the repo):  //npm.fontawesome.com/:_authToken=<token>
task                                # lists the Taskfile's commands (setup, dev, check, deploy, deploy:*, shared:*), if go-task is installed

# -- Frontend (Vite) --
cd frontend; npm ci                 # first time / after the lockfile changes (npm install <pkg> to add one)
cd frontend; npm run dev            # dev server on :5173, proxies /api -> :8080
cd frontend; npm run build          # type-check (vue-tsc) + build to backend/internal/web/dist/, which the binary embeds (task build:web)
cd frontend; npm run typecheck      # vue-tsc only
cd frontend; npm run test           # Vitest run (one-shot)
cd frontend; npm run test:watch     # Vitest watch mode
cd frontend; npm run test:coverage  # Vitest + v8 coverage, failing below the floor in vitest.config.ts (CI gate)
cd frontend; npm run lint           # ESLint + Stylelint with --fix
cd frontend; npm run lint:check     # ESLint + Stylelint without --fix (CI gate)
cd frontend; npm run format         # Prettier write over src/
cd frontend; npm run format:check   # Prettier check (CI gate)
cd frontend; npm run lint:conventions  # CSS naming, the scoped-CSS ratchet, theme tokens (CI gate)
cd frontend; npm run analyze        # build + emit frontend/stats.html bundle treemap (visualizer)
cd frontend; npm run test:e2e       # end-to-end: builds the binary (on the SPA's build: run npm run build first, or task test:e2e) and runs it on a throwaway database in %TEMP%, never devdata/ (e2e/)
task gen                            # (repo root) after a query or API change: sqlc, the document, the SPA's schema.d.ts and the plugin's client (NSwag); commit what it writes

# -- Go backend --
# Every Go build, test and lint compiles the SPA in (go:embed), so the front end is
# built first: cd frontend; npm run build (the task commands do it for you).
# Build the Go backend
cd backend; go build -o app-suite.exe .

# Run the server (from project root or with -db flag). It serves the API and the
# embedded SPA at http://localhost:8080; npm run dev is the way to work on the front end.
# -webhook-dry-run is not optional in practice: devdata is a COPY OF THE LIVE
# database, so its announcement types, book clubs, affiliates and tea rooms hold
# REAL webhook URLs - without it, the announcement scheduler posts to production
# channels on its own, no clicking required. Swap it for
# -webhook-override <test-webhook-url> when you need to see a post actually land.
cd backend; go run . -addr :8080 -db ../devdata/database.sqlite -webroot ../devdata/webroot -webhook-dry-run

# Auth is per-user now. On first run, migration v22 seeds a bootstrap account:
#   username: admin   password: admin   (full admin) - ROTATE IT IMMEDIATELY
# via the topbar "Change Password" modal after logging in at /admin/login.

# Vet / lint
cd backend; go vet ./...
cd backend; go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run ./...   # config: backend/.golangci.yml (built from source at the pin, as CI does)
go run -C shared/lint/go . -dir backend -check   # (repo root) .golangci.yml still = the shared template + .golangci.local.yml; drop -check to rewrite it after editing .golangci.local.yml
cd backend; go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...   # the release CI runs
cd backend; go test -race ./...       # CI runs this (shuffled, with an 83.5% statement-coverage floor, counted across packages); locally needs gcc on PATH (cgo for the race runtime)

# Run tests
cd backend; go test ./...

# After adding a migration: write the committed schema (TestSchemaIsCurrent fails until it matches)
cd backend; go run ./cmd/schema-gen

# Build for production
cd backend; go build -ldflags="-s -w" -o app-suite .

# -- FFXIV Dalamud plugin (C#/.NET 10) --
# Needs the .NET 10 SDK + a local XIVLauncher/Dalamud install (Windows).
cd plugins\SenpanCompanion; dotnet build -c Release   # -> bin/Release/SenpanCompanionAdmin/latest.zip (keep 0/0 warnings; the analyzers + code style run in the build - the C# lint gate)
cd plugins\SenpanCompanion; dotnet format             # apply the code style (CI runs --verify-no-changes)
cd plugins\SenpanCompanion.Tests; dotnet test --project SenpanCompanion.Tests.csproj   # the plugin tests (no Dalamud needed; plugins/global.json picks Microsoft.Testing.Platform)
.\scripts\deploy.ps1 -Target plugin                   # build + publish the custom Dalamud repo (pluginmaster.json + latest.zip)

# -- Scripts, docs and repo files (from the repo root) --
pwsh shared/lint/pwsh/Invoke-ScriptLint.ps1           # PSScriptAnalyzer 1.25.0 with the shared settings, on every tracked .ps1/.psm1/.psd1
npx --yes markdownlint-cli2@0.24.0                    # every .md, with the shared config (.markdownlint-cli2.jsonc)
pwsh shared/repo/Sync-RepoFiles.ps1 -Path . -Check    # .editorconfig/.gitattributes above their marker still match shared/repo
.\scripts\check.ps1                                   # everything CI runs, stopping at the first failure (-SkipFrontend / -SkipBackend for half)
```

## Continuous integration

`.github/workflows/ci.yml` runs on every push and pull request. Six of its gate jobs
each call one of shared-tooling's reusable workflows, pinned to the release the
`shared/` submodule holds (the docs job fails if the two ever differ); the
`generated` gate and the release job are Senpan's own. Each workflow checks the submodule out with the repo.
`scripts/check.ps1` runs the same checks locally. The jobs:

- **frontend** (`web.yml`, `dir: frontend`): Node from `frontend/.nvmrc`, the Font
  Awesome registry token from the `FONTAWESOME_NPM_TOKEN` secret (written to `~/.npmrc`
  for the install and removed straight after) -> `npm ci` -> `npm audit
  --audit-level=high --omit=dev` -> then the scripts the job lists, in order:
  `lint:check` (ESLint + Stylelint) -> `format:check` -> `lint:conventions` ->
  `typecheck` -> `test:coverage` (fails below the floor in `vitest.config.ts`) ->
  `build`, uploaded as the `web-dist` artifact for the backend job. It compiles
  against the committed API types.
- **generated** (Senpan's own, as Kindred Nights' and Home Library's, after the
  frontend job, whose build `go run` compiles in): sqlc -> `go run . openapi` ->
  openapi-typescript 7.13.0 -> Prettier -> NSwag 14.7.1 (`dotnet nswag run`, the
  plugin's client), then fails if `backend/internal/db`, `frontend/src/api` or
  `plugins/SenpanCompanion/Api` now differs from the committed copy (`task gen` runs
  the same).
- **backend** (`go.yml`, `dir: backend`, after the frontend job, whose build it
  downloads into `backend/internal/web/dist` for the binary to embed): `.golangci.yml` still matches the shared
  template merged with `.golangci.local.yml` -> `go vet ./...` -> golangci-lint
  v2.15.0 (built from source with `install-mode: goinstall`, since a prebuilt binary
  can't read export data from a newer Go; the template's linters - bodyclose,
  errorlint, exhaustive, gocritic, gosec, noctx, revive, rowserrcheck and the rest -
  and the gofmt + goimports formatters, with Senpan's exclusions from
  `.golangci.local.yml`) -> govulncheck v1.9.0 (pinned: an old release crashes on a
  newer Go) -> `go test -race -shuffle=on ./...` with coverage, failing below **83.5%**
  of statements, counted across packages (`coverpkg: ./...`) -> `go build -ldflags="-s -w"
  -o app-suite .`, the binary as the deploy builds it. Go version read from
  `backend/go.mod`; the server tests check every 2xx answer from an operation on
  huma against its document (`internal/api/apitest`); the SQLite driver is pure Go,
  so the race build runs the driver production ships.
- **e2e** (`e2e.yml`, after the frontend and backend jobs): `npm run test:e2e`, the
  Playwright suite in `frontend/e2e/` on shared-tooling's `test/e2e` settings, in the
  runner's Chrome. The front end is built first, then `e2e/server/prepare.ts` builds
  the binary with it embedded into a throwaway folder in the temp directory, refusing
  any path inside the checkout so `devdata/` is never touched, and the server runs
  with `-webhook-dry-run` and Turnstile off, so nothing reaches Discord. It serves the
  API, the event streams and the SPA, as production does; data is seeded through the API
  as the bootstrap admin. The main public pages must show their data and pass axe at WCAG 2.2 AA
  (shared-tooling's `test/a11y`), the floor under the AAA target; every theme's AAA
  contrast is held by `go run ./backend/cmd/themetool check <db>` (see
  `scripts/themes/README.md`). A player must join a board, see a draw arrive
  over the event stream and keep a stamp across a reload. The specs find elements by
  role and name, never by class. Pages keep their visible titles as `h2`: the home
  page's logo is its `h1`, and every other page has a visually hidden `h1` with the
  app's name (`App.vue`).
- **plugin** (`dotnet.yml`, `projects: plugins/SenpanCompanion`): builds the FFXIV
  Dalamud plugin on a **Linux** runner - there's no XIVLauncher there, so it
  downloads the official Dalamud dev distribution and points `DALAMUD_HOME` at it;
  the `net*-windows` TFM compiles via the `EnableWindowsTargeting` property
  (conditioned off on Windows, so local builds are unchanged). Gate =
  `dotnet build -c Release -p:TreatWarningsAsErrors=true` (the 0-warning rule) +
  `dotnet format --verify-no-changes`, then the tests (`test-project:
  plugins/SenpanCompanion.Tests`, `dotnet test` on Microsoft.Testing.Platform, which
  `plugins/global.json` picks). The build is also the C# lint gate:
  `plugins/Directory.Build.props` imports shared-tooling's `lint/dotnet`, which turns
  on the .NET analyzers at `latest-recommended` and checks the shared code style
  (`this.` qualification, `var`, file-scoped namespaces, naming) in the build with
  `EnforceCodeStyleInBuild`, so a departure fails it rather than waiting for
  `dotnet format`. Tracking Dalamud `latest` means an upstream API bump can redden
  it - the cue to bump `DalamudApiLevel`.
- **scripts** (`pwsh.yml`): PSScriptAnalyzer 1.25.0 through
  `shared/lint/pwsh/Invoke-ScriptLint.ps1`, on every tracked `.ps1`/`.psm1`/`.psd1`
  (the untracked `scripts/deploy.config.ps1` is never read), with the shared
  settings (default rules + the one-true-brace layout). Any finding fails it.
- **docs** (`docs.yml`): markdownlint-cli2 0.24.0 over every `.md` (the shared config,
  through `.markdownlint-cli2.jsonc`), `.editorconfig` / `.gitattributes` still
  matching `shared/repo` above their project marker, and every shared workflow ref
  naming the release the submodule holds.
- **release** (after every gate job, only on a push to `main`): runs
  `.github/scripts/release.sh`, which reads the app's version (`frontend/package.json`)
  and the plugin's (`SenpanCompanion.csproj`) and, for each whose release doesn't exist
  yet (`v<version>`, `Plugin-v<version>`), tags it and publishes a GitHub Release whose
  body is that version's `CHANGELOG.md` entry. The deploy ships these tags. Idempotent
  (already-released versions are skipped) and gated on the whole gate passing, so
  only green commits release.

`scripts/check.ps1` (or `task check`) runs, stopping at the first failure: the
front end's build, which the binary embeds; backend - the golangci config check, golangci-lint, `go build`, `go vet`, `go test`
(without `-race`, which needs gcc) and govulncheck; frontend - the frontend job's
scripts plus `npm audit`; then, on a full run only, the plugin's build,
`dotnet format` and its tests (skipped with a message when `dotnet` isn't installed) and the
scripts and docs jobs' checks. It needs `shared/` checked out, and says so when it
isn't.

A check only Senpan runs goes into both `scripts/check.ps1` and its job in `.github/workflows/ci.yml`; a new npm script goes in the frontend job's `scripts` list.

### Versions and releases

Since 4.0.0 the app has one version, for the SPA and the API together, since one binary ships both: `frontend/package.json` `"version"`, which `backend/internal/version/version.go` `Version` must equal (`TestVersionIsTheApps` checks; served at `GET /api/version` and `GET /healthz`). The plugin keeps its own, as Dalamud needs: `SenpanCompanion.csproj` `<Version>`, with `AssemblyVersion` in `plugins/pluginmaster.json`. A change that ships adds its entry under Unreleased in `CHANGELOG.md` (`### App` or `### Plugin`); raising a version moves those entries into a `### [version] - date` heading under the App or Plugin section, newest first, in the same commit. The Frontend and Backend sections are the history from before 4.0.0 and take no new entries. The admin sidebar footer shows the app's version and the plugin's, and flags an open tab whose server now runs a different version. The release job above tags each new version and publishes its CHANGELOG entry; nothing is tagged by hand.

## Deployment (Apache)

Only the maintainer deploys, from the owner's PC, through `scripts/deploy.ps1` on SharedDeploy, and only releases ship: the newest `v<version>` CI tagged on main for the app, the newest `Plugin-v<version>` for the plugin, each built in a checkout of its own (shared-tooling's newest-release deploy). `task deploy` deploys the app, then the plugin when its version is new (`-IfNewer`); an app release the host already runs is skipped (`-Force` deploys it again). `task deploy:backend` and `task deploy:plugin` deploy one each, with flags after `--`. The settings live in the untracked `scripts/deploy.config.ps1`; `deploy/README.md` has the whole run.

The Go binary serves the app: the API, its event streams and the SPA it embeds. The vhost's `/api/` ProxyPass has `timeout=3600` and `mod_deflate` must not compress `text/event-stream` (see `deploy/README.md`). The
vhost's ProxyPass sends `/api/` to it, and the web root's `.htaccess` sends it
everything else that isn't one of the web root's own folders. Uploads live in those
folders, apart from the binary, so redeploys never wipe them (full guide in
`deploy/README.md`):

```text
<DocumentRoot>/
+-- .htaccess        <- deploy/.htaccess, with the app's address filled in (serves the folders below; proxies everything else to the app)
+-- images/          <- deploy/images/    (PERSISTENT: logo/favicon/banner + raffles/, announcements/, bookclub/ uploads)
+-- fonts/           <- (PERSISTENT: admin-uploaded font files; NOT static - streamed by the Go server via tokenized URLs; fonts.senpan.cafe reverse-proxies to /api/v1/fonts/pub/)
+-- carrd/           <- (PERSISTENT: Carrd image-host projects; served by the carrd.senpan.cafe vhost)
\-- plugin/          <- the Dalamud custom repo: pluginmaster.json + SenpanCompanionAdmin/latest.zip + .htaccess (deploy/plugin.htaccess); replaced by -Target plugin, previous kept as plugin.old
```

Until 4.0.0 Apache served the SPA from `<DocumentRoot>/dist`; once 4.0.0 has stayed, `dist/` and `dist.old/` can go (`deploy/README.md`).

- Run the Go server with `-webroot <DocumentRoot>`; uploads are written under
  `<DocumentRoot>/images/{raffles,announcements,bookclub}/`,
  `<DocumentRoot>/fonts/`, and `<DocumentRoot>/carrd/<project>/`. Image/font/carrd
  URLs are returned absolute (built from the request scheme+host).
- `vite.config.ts` strips the build's `images/` (the `strip-dist-images`
  plugin), so the binary never carries a copy of the persistent root `images/`,
  which Apache serves.
- **Which folder a new image goes in.** A consequence of that strip: `frontend/public/images/`
  is a **dev-time mirror of the server's persistent tree**, not a way to ship a file. Put an
  image there and it works on `npm run dev` and 404s on live until someone uploads it by hand -
  and because the browser caches the miss, it stays broken for a while after they do. So:
  admin-uploadable or operator-replaceable art (raffle prizes, the logo, PWA icons) belongs in
  `public/images/` **and** `deploy/images/`; art that ships with the code belongs in
  `src/assets/images/` as an `import` (Vite emits a content-hashed file into the build's `assets/`,
  which needs no deploy step and can never serve a stale cache entry). `KonamiEgg.vue` does
  the latter; `YoeverOverlay.vue` predates the rule and still reads `${BASE_URL}images/`.
- Schema migration v10 rewrites legacy `assets/images/raffles/...` prize paths
  to `images/raffles/...` automatically on first start.
- **Font host (protected serving)**: uploaded fonts are licensed assets and are
  NOT served statically. The Go server streams them via rotating tokenized URLs
  (`/api/v1/fonts/pub/kit.css` + `/api/v1/fonts/pub/f/{token}`, `internal/api/fonts.go` over the `internal/fonts` rules) gated
  **per font** by its own origin allowlist (Font Upload -> Edit); Chrome and
  Firefox require the echoed CORS header on cross-origin `@font-face`, so
  non-listed sites can't render the fonts, and `kit.css` only emits each
  requesting site's allowed fonts. **Safari sends no `Origin` on font loads**
  (WebKit fetches `@font-face` in `no-cors` mode), so an Origin-less request is
  judged by its `Referer`'s site against the same allowlist - gate on `Origin`
  alone and every iPhone gets a 403 while desktop looks fine. The `fonts.senpan.cafe` vhost is a **reverse proxy** (`ProxyPass / ->
http://localhost:8080/api/v1/fonts/pub/`, edited on deploy day; the old `/api/fonts/pub/` no longer answers); external Carrd sites embed
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
  announcements to Discord webhooks - no cron needed; it
  starts with the process and stops on graceful shutdown.
- **Server logs**: structured JSON to stdout (journald) + a rotating file at
  `/var/log/senpan/senpan.log` (`-log-file`). Under `ProtectSystem=strict` the
  `senpan.service` unit needs `LogsDirectory=senpan` so that path is writable;
  otherwise the app degrades to stdout-only (a warning is logged). Admins view
  them at **System -> Logs** (live tail + `GET /api/v1/logs`) or on-box via `jlv`.
  See **Server logs** in `deploy/README.md`.
  - **Request line** carries `method`/`path`/`status`/`duration`/`ip` plus an
    **actor**: `auth` (`session`|`token`|`bot`|`anon`), and `user`/`bot` when they
    apply. Admins resolve via the cookie session, the plugin via its PAT (both
    through `currentUser`, resolved in `withActor` inside the handler chain and
    carried out to the log via a per-request holder, since the log runs outside
    the session middleware - see `actor.go`). A verified crawler is named from
    Cloudflare's signal: native `cf-verified-bot`(+category) on Bot Management, or
    a custom `x-verified-bot` header set by a Transform Rule on `cf.client.bot`
    (works on any plan). It's a logging hint only, never a security decision.
  - **Capability tokens are redacted** (garapon/stamp-card/font paths, the PAT
    `token` query, and Referer) to a short SHA-256 correlation hash (`logredact.go`).
    `logClientIP` trusts `CF-Connecting-IP`/XFF only from a loopback peer (the
    Apache proxy), so a direct client can't forge the `ip`.
  - **Live tail** fans each line out as a `log` event to the admins'
    staff streams only (the hub's admin audience, re-checked with the account), matching
    the admin-only guard on `GET /api/v1/logs`. It is lossy and never replayed: the
    viewer reads the log afresh when its stream opens.

### Festival Map

A **Festival Map** (`festival_maps` + `festival_stalls` + `festival_stall_occupants`)
is a floor plan the public can explore.

A stall on the plan is a **PITCH** (where it sits, how it is drawn) holding one or
more **OCCUPANTS** (affiliate, title, description, offering, times). Most pitches
keep the same business all festival and have exactly one; a booth that changes
hands between days has one per day. Splitting the two is what keeps the plan
readable - the pitch is drawn ONCE, in one place, whatever the rota, instead of two
stalls fighting for the same pixels. The base image is the bare venue - walls, rest areas, the stage - and
every stall is **drawn on top of it by the app**: a circle or rectangle at a %-based
`Placement` in the stall's color, with its title rendered inside. Nothing about a
stall is baked into the artwork, so renaming or moving one never means re-exporting
the image.

- **Admin** (`FestivalMapsTab.vue` -> `FestivalMapFormTab.vue` -> `MapStallEditor.vue`,
  permission `festival-map`, first item in the Festival section). The form carries the
  festival's title, markdown description, its datetime ranges and the base image; the
  editor drops stalls onto the plan and drags/resizes/rotates them (the handles sit
  OUTSIDE the stall's box, so `.map-stall` must never clip - the label clips itself
  instead), with a per-stall panel for the affiliate, title, markdown description,
  what it offers, shape, color, an exact rotation, and its own hours. Every stall
  draws TWO lines - its title and a caption naming what it offers (`stallCaption`):
  a named type captions itself, while `other` captions with the stall's own
  `type_label` wording, and drops the line entirely when that is blank, since
  "Other" tells a visitor nothing. A **circle
  takes its height from its own rendered width** (`aspect-ratio: 1`) rather than
  from `placement.height`: the map box is almost never square, so equal percentages
  draw an oval. `usePlacementDrag`'s `square` option bounds such a stall by that
  rendered height, and `placement.height` is left untouched so switching a stall
  back to a rectangle restores its proportions. `stall_type` (Game / Food / Food & Game / Other) seeds the shape
  and color, and changing it re-shapes the stall - the shape is the at-a-glance signal,
  so the type wins on change.
- **Publishing is a separate step** on the detail screen (`PATCH .../{id}` with a
  `status`), not a field in the form: `in_progress` -> `published` -> `closed`. Only a
  `published` map is public, and publishing one with **no base image** is refused - the
  stalls are positioned as a share of that image's box.
- **Shortcodes**: a map may carry an optional `slug` ("obon-2026") so it links as
  `/festival-maps/obon-2026`. `GET .../public/{id}` resolves the segment as a
  SHORTCODE first and falls back to a numeric id (`resolvePublicMap`), so both forms
  always work and adding or changing one never breaks a link already posted. What
  the admin types is normalized on save (trimmed, lowercased, whitespace/underscore
  runs folded to single dashes), an all-numeric shortcode is refused with a 400 - it
  would be indistinguishable from an id, which is exactly what makes the dual lookup
  safe - and one another map holds is a 409. `mapPath` (`internal/api/festivalmaps.go`) is the single place
  that decides which form a link uses, so the admin's "Copy public link", the public
  list and the map page can't disagree.
- **Public** (`FestivalMapsView` -> `FestivalMapView`, `FestivalMapCanvas.vue`): drag
  to pan, wheel or pinch to zoom (anchored on the cursor), plus zoom buttons and Fit.
  Tapping a stall opens its details, including the affiliate's public card - name,
  subtitle, owners, logo, Discord/Carrd links, and NEVER the webhook or embed colour.
  A drag that ends over a stall doesn't open it (`onStallPointerUp` compares against
  where the press started).
- **Day switcher** (public): built from the map's own `times` rows. Picking a day
  leads with the occupants standing in each pitch that day (`occupantsOnDay`, which
  overlaps each occupant's ranges against the day's) and pushes the rest back; a
  pitch nobody occupies that day dims out. An occupant that states NO times runs the
  whole festival and matches every day - hiding a permanent stall because it never
  spelled out its hours would take it off the plan entirely.
- **Raffle linkage**: a raffle may set `festival_map_id` (the festival it belongs
  to - the same grouping a rally gets) and `occupant_id` (one stall on that map).
  A pinned raffle shows in that stall's panel on the public plan, but ONLY while
  `raffleRunning` holds: status is not `closed` AND now is inside its availability
  window. A closed or not-yet-open raffle is left off the payload rather than
  linking a visitor somewhere they can't enter. `resolveStall` (`internal/api/raffles.go`) settles the
  pair on save exactly like the rally's resolver.
- **Stamp Rally linkage**: a rally may set `festival_map_id`, after which each stamp
  names an `occupant_id` on that map instead of a bare affiliate, takes its affiliate
  FROM the occupant, and seeds its stamp type from what the occupant offers. It names
  the OCCUPANT rather than the pitch because two tenants of one pitch are different
  businesses running different activities, each needing its own stamp. `settleStalls`
  (`internal/api/stamprallies.go`) settles this on every save: an unlinked rally has every `stall_id` cleared,
  and a `stall_id` that isn't on the linked map falls back to the affiliate rather than
  failing the whole save. `StampRallyStamp.stall()` then decides the ONE
  name the card, the stamp log and the map all use: the map stall's title, else the
  affiliate, else "Senpan Tea House". On the public map, stalls carrying a stamp on an
  **open** rally are ringed and show their stamp art, with a "highlight rally stalls
  only" toggle that dims the rest rather than removing them - the plan keeps its shape.
- **Deleting a map** keeps any rally and raffle that was linked to it: `delete-festival-map`
  clears `stamp_rallies.festival_map_id`, the occupants' `stamp_rally_stamps.occupant_id` and
  the raffles' links itself (plain columns, not enforced FKs), so the stamps survive and fall
  back to their affiliates. It deletes the map's ema board (and the plaque files still
  waiting), and tells the Stamp Rally, Raffles, Ema boards and artist's queue pages.
  Saving a map tells the Stamp Rally and Raffles pages, which show its stall names. A pitch
  or occupant id that isn't the map's own is a 409; a `PUT` without `stalls` is a 422.

### Festival dashboard

`/festival/:id` (`FestivalDashboardView`, `stores/festivalDashboard.ts`) is one
visitor's view of everything filed under a festival - the plan, the stamp rally,
the garapon and the raffles - with the actions on each, so nobody is sent off to a
page per system. The festival list (`/festival-maps`) opens it; the map page, the
stamp card page and the drawing page link to it. Same shortcode-or-id segment as
the map.

- **Two reads.** The plan is the public map (`GET /api/v1/festival-maps/public/{idOrSlug}`,
  the same payload every visitor gets). What depends on WHO is looking is
  `POST /api/v1/festivals/{idOrSlug}/dashboard` (`internal/api/festivals.go`): fed the card
  token, the drawing token and the name + world this device holds, it returns the
  card on the festival's rally, the drum linked to that rally, and every raffle
  filed under the festival (`raffles.festival_map_id`) a visitor may see, each with
  the visitor's own entry. POST so tokens and names stay out of URLs and logs; its
  own prefix because `adminMutationResource` treats every POST under
  `festival-maps` as an admin edit and would broadcast on each page load. Rate
  limited loosely (`dashboardLimiter`, 120 / 10 min).
- **Everything sent is checked against the festival** and a token that doesn't
  belong is ignored, not refused (a phone that has been to two festivals holds
  both). A card counts when its rally is filed under this map, whatever the rally's
  status - a finished card with its prizes is still worth showing. A drawing link
  counts only when its drum serves THAT rally. A drawing link carries its paired
  card (`garapon_players.stamp_card_id`), echoed back as `card_token`, so a device
  that only held the drawing link learns the card; the reverse never happens - a
  card alone reports `paired_draws_left`, never the drawing token.
- **Raffle entries match the whole name and world**, the way entries merge
  (`FestivalRaffleEntriesFor`), never a substring, and carry no gil figure
  (`RaffleLookupEntry`). The dashboard's entry form has no name field: it always
  enters as the visitor, so new tickets join their existing entry.
- **The panels are shared, not copied.** The store hands the card and the drum to
  the stamp-rally and garapon stores' public state (`showPublic`), and the same
  `StampCardPanel` / `GaraponPanel` their own pages render show them here. Every
  action goes through the endpoint that owns it (stamp, draw, enter, sign up,
  lookup). `GaraponPanel` copes with a drum loaded without a player - no drawing
  link on this device - by showing the prizes alone.
- **Who is visiting** is kept per festival in `lib/signups` (`kind: 'festival'`:
  name, world, and the rally once named - the public map stops naming a rally
  outside its window, and without its id the card saved for it couldn't be found).
  A card fixes the identity; without one the visitor types it. "Forget this
  device" drops the festival's links, name and raffle spellings.
- **Hand-off from the card and drawing pages** goes in history state
  (`festivalConnectState`), never the URL, and is taken off the history entry as
  it's read, so a reload can't replay it after a "forget". A handed-over card
  replaces the device's own for that rally (and drops a drawing link kept for the
  old one); a handed-over drawing link brings its own card.
- **Collected stamps on the plan.** `PublicStamp.occupant_id` lets the dashboard
  pass `collectedOccupantIds` to `FestivalMapWidget`/`FestivalMapCanvas`: a stall
  whose stamps the visitor holds gets a success-colored ring and a check in place
  of the stamp mark, and its panel says so. The widget's `rally-action` and
  `raffle-action` slots let the dashboard put a stamp-password box in the stall
  panel and jump to the raffle in-page instead of linking away. Every other map
  view passes nothing and is unchanged.
- **Layout.** A wide screen shows every section: the plan across the top, the rest
  in auto-fit columns. Under 900px the tiles (stamps collected, draws left,
  raffles entered) pick ONE section to show; the others are `display: none`
  rather than unmounted, so the plan keeps its zoom and the drum its spin. The
  styles live in `assets/styles/festival.css` with the card and drum objects.
- **Refresh on return**: coming back to the tab re-reads after 60s (never
  mid-spin). There is no live stream for these visitors.

### Ema Plaques

Ema are wooden wishing plaques. A festival may have one **plaque board**
(`ema_boards`) offering up to three ways to hang one, shown as a section of the
festival dashboard (`FestivalEmaSection.vue`):

1. **Design it yourself** - download the board's template, decorate it anywhere,
   post it in Discord (the board's `instructions` + `discord_link`). Nothing is
   recorded, so it isn't counted.
2. **Design it here** - the on-site designer (`components/player/ema/EmaDesigner.vue`,
   Konva, loaded only when opened): image and text layers, Google or basic fonts,
   stroke, an eraser scoped to one layer, a clip to the plaque's shape, undo. It
   emits a PNG; the section shows a confirm step (inside the same modal, the designer
   hidden but still mounted so "Back to Editing" keeps the work) and sends it to
   `POST /api/v1/festivals/{idOrSlug}/ema-plaques`. Uploaded images are read as `data:` URLs, never
   `blob:` - production's CSP allows only the former in `img-src`.
3. **Ask the artist** - a description, saved to the artist's queue.

- **One plaque per person per day**, across options 2 and 3, by window: a partial
  unique index in the database, mirrored in the UI (`has_plaque_this_window`). A
  submission goes to the window open NOW; none open is a 400.
- **Staff review every site plaque** (owner's choice: anyone can type a character
  name, so nothing reaches Discord unseen). Approving claims it (`pending` ->
  `posting`, so a second approval is refused rather than posted twice), posts it
  with the image attached through the shared Discord funnel
  (through the `discord.Sender`'s `PostFile`, so dry-run/override apply), records it `posted`, and
  **deletes the file** - Discord holds the posted copy. A failed post returns it to
  `pending`; an ambiguous one (no answer from Discord) says to check the channel
  before retrying. Rejecting deletes the file and frees the day.
- **A day can cap the artist's requests** (`ema_windows.artist_limit`, 0 = no
  limit, set per day in the board form beside the webhook). the submission
  inserts first and counts after, inside one write-locked (`h.begin`)
  transaction: the lock is what stops two visitors racing for the last place from
  both counting the day as open, and inserting first keeps "you already have a
  plaque today" ahead of "full" for someone who has one. Past the limit is a 409.
  Staff see `artist_count` per window; the dashboard gets `artist_left` and
  disables the request option at 0. Only artist requests count - designing on the
  site is never capped.
- **The artist posts a finished request from the queue** (`POST
  /api/v1/ema-requests/{id}/post`, `EmaRequestPostModal.vue`): the uploaded image goes
  straight to the request's day webhook as "Requested by \<character>, made by
  \<artist>" and the request is completed. No review (the artist is staff) and **no
  copy kept** - the multipart body is capped under the parse's memory limit so it
  never spills to a temp file, and nothing is written. Same claim as an approval
  (`requested` -> `posting` -> `complete`, back to `requested` on a failed post).
  Its type is sniffed from the bytes (PNG, JPEG, GIF, WebP) and its size held to
  Discord's 10 MB default. Plain Complete stays for a plaque posted by hand.
- **Permissions:** `festival-ema` for the boards and the review queue;
  `festival-ema-requests` for the artist's queue alone, so the artist can work it
  without seeing the boards' webhooks. `GET /api/v1/festival-maps` is readable with
  `festival-ema` too, for the board editor's festival picker.
- **Fonts:** `GET /api/v1/google-fonts` returns the family names read with the server's
  own key (the key is a secret setting) and cached 12 hours; the designer falls
  back to `FALLBACK_GOOGLE_FONTS` when the list is empty.
- **Webhooks, not the bot:** built on `main` against webhooks; the Discord-bot
  migration on `bot-replacement` converts each window's webhook to a channel like
  every other.

### Duplicating an event

Raffles, Stamp Rallies, Garapons and Festival Maps each expose a **Duplicate** on
their detail toolbar AND a row-level **Copy** on their closed table, both backed
by a `copy*Form` in the store. Each one runs the normal `edit*Form`
first and then strips what belongs to the run that already happened, so all four
behave the same way:

- **every id goes to 0** (the item's own, plus nested stamp/prize rows), so saving
  CREATES instead of overwriting the thing it was copied from - the single most
  damaging way to get this wrong;
- **the title gains `(Copy)`**, since the form can be submitted as-is and two
  identically named events are a support problem;
- **dates are cleared** - a raffle's availability window, a rally's window AND its
  per-stamp active windows (a stale one silently gates a stall on last event's
  dates), plus a rally's per-stamp `paused` flags;
- **a garapon drops its `stamp_rally_id`** - it points at the rally the ORIGINAL
  drum served, and keeping it would leave two garapons issuing draws off one
  rally's sign-ups.

**Every copy drops its FESTIVAL link** - a raffle's and a rally's
`festival_map_id`, a rally's per-stamp `occupant_id`, a garapon's `stamp_rally_id`.
A copy is almost always next year's, and those rows still exist, so nothing on the
server would clear them: the duplicate would quietly file itself under the finished
festival, naming last year's stalls. Pick the festival again on the copy.

A festival map is the one exception worth naming: it also drops its SHORTCODE (a
shortcode names exactly one map, so keeping it would be refused on save and would
take over the URL the original is already linked by), and its stalls keep their
placements (re-laying out a venue by hand is exactly what a copy exists to avoid)
while losing the festival's dates AND each stall's own hours, since a stale window
would mark the new festival as long over.

**Copying from a closed TABLE loads the detail first.** Every closed table (raffles,
garapons, rallies, maps) carries a row-level Copy beside View/Delete, but a list row
holds no sub-entities - the list endpoints omit garapon prizes, rally stamps/prizes
and map stalls on purpose. Seeding a form straight off the row produces an empty
duplicate, which is precisely what the button exists to avoid, so each row action
awaits the detail fetch and copies that.

What is NOT copied is anything the original owns: issued stamp cards, garapon
drawing links and draw logs, raffle entries. Those stay with the item they were
created against.

### Raffle entry modes & payments

`raffles.entry_mode` decides how a raffle is priced and whether it takes sign-ups at
all. `raffles.NormalizeMode` (`internal/raffles`) maps anything unrecognized - including the empty
string on a pre-v57 row and an API client that omits the field - onto `single`, so
an unknown value can never read as "free" or "no sign-ups".

| Mode      | Admin label            | Pricing                                                   | Public sign-up form |
| --------- | ---------------------- | --------------------------------------------------------- | ------------------- |
| `details` | Details Only           | none - the raffle tracks no money                          | no (instructions shown inline instead) |
| `single`  | Single Cost per Entry  | every ticket costs `cost_per_entry`                        | yes                 |
| `custom`  | Custom Cost per Entry  | the Nth ticket costs `tier_costs[N-1]` (50k/100k/150k = 300k for three) | yes     |

**One pricing rule, three implementations.** `raffles.Pricing.EntryCost(n)` (Go; `model.Raffle` delegates to it for the festival code) is the
authority; `raffleEntryCost` (`stores/raffles.ts`) and `Raffles.EntryCost` (the
plugin's `Core/Raffles.cs`) mirror it so a previewed total can't disagree with what the
server charges. Change one, change all three. All three walk the **ladder**, never
the ticket count - `max_entries` is admin-set and unbounded, so a per-ticket loop
would let a large allowance spin every time the admin raffle list loads.

Two consequences worth knowing before touching this code:

- **`custom` mode pins `max_entries` to the ladder length** (server-side, in
  the operation's `columns`) - a 4th ticket in a 3-rung ladder would have no price. The save also
  drops the cost fields the chosen mode doesn't use, so switching a raffle back to
  `single` can't leave a stale ladder behind as a shadow price.
- **The per-player allowance is capped at 100** (`raffles.MaxEntries`, mirrored as
  `RAFFLE_MAX_ENTRIES` in `lib/constants.ts`), and so is the ladder, whose length is
  that allowance. Over the cap is a **422, not a silent clamp** - an admin who typed
  1000 meant something, and quietly storing 100 would surface later as a limit
  nobody chose. Raffles saved before the cap keep their value until someone edits
  them.
- **`details` raffles draw from every entry, not just paid ones**
  (`raffles.Tickets` / `raffles.Pick`): they collect nothing through the app, so
  requiring a paid flag would make them undrawable.
- **A paying raffle draws on SETTLED tickets** (`paid_entries`), not on tickets
  held. A part-paid entrant gets one chance per ticket they actually paid for -
  dropping them from the draw and giving them all their chances are both wrong. A
  fully-settled entry has `paid_entries == num_entries`, so nothing changes for it.

**Payments are a settlement record, not a flag.** Entries merge per
character+world, so one row grows as a player buys more tickets. `paid_entries`
tracks how many of `num_entries` are settled and `amount_waived` accumulates the gil
forgiven across settlements; `paid` is the derived "nothing outstanding" flag the
draw and the collected-gil totals read.

- The sign-up confirmation quotes **`amount_due`**, not just `total_cost`.
  `total_cost` is the sticker price of every ticket the character now holds;
  `amount_due` is `Raffle.AmountOutstanding` - what is left after the tickets they
  already settled. They differ only for someone coming back for more entries, which
  is exactly when quoting the full total would invite paying for the first ticket
  twice. `amount_due` folds settled tickets and waived gil together on purpose, so
  an unauthenticated response can't be used to work out that a character was comped.
  The amount is never quoted BEFORE sign-up, only in the confirmation - see the
  lookup below for what the public search does and doesn't answer.
- **Public entry lookup** (`POST /api/v1/raffles/{id}/lookup`, `look-up-raffle-entries`).
  Entries merge on character+world, so a returning entrant who retypes their name
  slightly differently silently starts a SECOND entry and splits their tickets; the
  search exists so they can copy back the exact spelling. It matches a **substring**
  (case-insensitive) - deliberately looser than the stamp-rally lookup, which matches
  whole names to keep participation private, because a raffle's entrant list is
  public by decision. Guards: at least 2 characters (a 1-char query is a request for
  the entrant list, not a name search) - counted in CODE POINTS on both sides
  (`RaffleLookupInput`'s `minLength` / `RAFFLE_LOOKUP_MIN_QUERY`), 50 rows with a `truncated`
  flag rather than a silent clip, the raffle must already be publicly viewable, the search is a literal substring match (`SearchRaffleEntries`
  reads `%` and `_` as themselves, not as wildcards), and its
  **own** rate limiter (`RaffleLookupLimit`) so searching can't spend the budget
  for the sign-up it precedes. `RaffleLookupEntry` is a SEPARATE type from
  `RaffleEntry`, not a filtered view, so a field added to the stored entry can never
  reach this response by accident - it carries no entry id and **no gil figures**,
  because a waiver is a private arrangement. The lookup publishes no live event (it is a read).
- Settling is a command, `POST .../entries/{entryId}/settle` with `{paidEntries?, waived}`: it settles `paidEntries` tickets - **all of them when the field is
  omitted**, fewer to record a part payment - and **adds** to `amount_waived` - it never
  overwrites it, so a player whose first entry was comped and who later buys two more
  keeps both waivers. `waived` is therefore what is being
  waived NOW, never a running total; gil amounts are whole numbers. `.../unsettle` clears both.
- Settling only ever moves **forward** (`WHERE MIN(?, num_entries) > paid_entries`),
  which is both the idempotency guard and what stops a stale count walking a
  settlement backwards; clearing is the only undo. The ticket count is clamped with
  SQL `MIN()`, not in Go - `num_entries` can grow between a read and the write.
- Settling is **idempotent**: the UPDATE carries its forward-only guard,
  so a double-submit or two staff on the same entry can't stack one waiver twice
  and understate what was collected. The guard lives in the statement because a
  read-then-write would race identically. The settle queries report whether they
  applied; the operation re-reads the row regardless.
- The settlement columns are **staff-only**. A closed raffle publishes its winner by
  name, so the public `winnerEntry` is a separate `RaffleWinner` (name, world and tickets only) with no
  `paidEntries`/`amountWaived` at all - "that person had 300,000 waived" is nobody
  else's business and reads as favouritism either way.
- Buying more tickets on a settled row drops `paid` back to 0 while keeping
  `paid_entries`/`amount_waived`, which is the **partial** state (`3 (1 paid)` in the
  entries table, `Partial` on the pay button). Clearing a payment resets both, so a
  mis-click can't leave a waiver credited against an unpaid entry.
- `paidTotal` is no longer a SQL `SUM`: a tiered raffle prices "one entry
  holding 3 tickets" differently from "three entries holding 1", and each carries its
  own waiver. `list-raffles` reads the settlements and prices them through
  `Pricing.AmountCollected` (floored at 0, so an over-generous waiver can't eat another
  entry's contribution). `winnerName` and `paidTotal` are filled only there, for a caller holding the Raffles page; `get-raffle` sends them null.
- **Picking a winner is guarded against two consoles.** `pick-winner` is a 409 while a winner is pending. `pick-another` names the winner it replaces (`replaces`) and `verify-winner` the one it confirms (`winnerEntryId`); either is a 409 when another console has changed the winner, and the replaced entry is left out of the redraw. `enter-raffle` answers 404 only for an unknown or hidden raffle, and 409 when the raffle is closed, outside its window, details-only or over the allowance; it publishes `resource_changed` `raffles` itself.

## Conventions & patterns

- **CSS naming + token drift**: see **CSS conventions** under **Frontend**. Shared styles are the default; a rule becomes a shared object at its **second** occurrence. Scoped CSS may only shrink (ratcheted against `scoped-css-baseline.json`; new SFCs cap at 120 lines). The themeable token list is duplicated in three sources that must agree, and every themeable token must have a consumer outside the theme editor. All of it is gated by `npm run lint:conventions`.
- **Contexts**: every store method and operation takes `ctx context.Context` first, and the process context `main.go` cancels on SIGINT/SIGTERM also runs the migrations in `store.New(ctx, path)`. What runs under `context.WithoutCancel(r.Context())`: a delayed player draw, a half-time timer, a broadcast's reads, and a run of writes a cancellation would leave half-done. A canceled query fails with `context.Canceled`, and `store.Interrupted` (SQLite's own interrupt code) is the backstop for an interrupt that arrives unconverted; the operations log both at Debug (`clientGone`), and the access line logs such a 500 at Info with `canceled=true`.
- **Routing**: huma owns every route under `/api/`; the mux registers only the HTTP shell (the SPA, `/healthz`, the `/api` layer for what huma doesn't have).
- **Typed requests**: an operation's input is a named Go type whose fields carry `doc:` and validation tags; huma decodes and validates it (400 for a body it can't parse, 422 with each field named for a failed validation).
- **Errors**: RFC 9457 problem details (`application/problem+json`) with a `detail` a person can read and, for validation, `errors` naming each field; a hand check adds `huma.ErrorDetail{Location: "body.x"}`. A 429 carries `Retry-After`.
- **REST shape**: the shared API rules hold; the one action-shaped exception is signing in and out and signing up, under `/api/v1/auth/*` (`login`, `logout`, `register`, `passkey/begin`, `passkey/finish`); adding a passkey is resource-shaped (`POST /api/v1/me/passkeys`).
- **Account-based auth + per-page permissions**: login stores `user_id` in the SCS session cookie; `currentUser` reloads the account from the DB on each request (no stale snapshot). Every mutating admin operation opens with a guard - `signedIn` / `session` / `permitted(perm...)` / `admin` (see **Authentication & authorization**). Permission keys equal the frontend `AdminTab` ids. The legacy `-password` / `APPSUITE_ADMIN_PASSWORD` flag is **deprecated and unused** (kept only for backward-compatible startup); don't add new code paths that read `Server.password`.
- **Password hashing**: argon2id via the `internal/auth` package (`Hash`/`Verify`). The hash lives only in the store layer and is never placed on `model.User`. The seeded `admin`/`admin` bypasses the 8-character minimum and must be rotated.
- **Pattern snapshots**: `game_patterns` stores a copy of pattern name + data so deleting a pattern doesn't break active games.
- **Winner caching**: `games.winners_cache` stores JSON array of winner card IDs; updated only on draw, read on board requests.
- **In-memory game state cache**: `GameService` caches the built `GameState` and invalidates on start/draw/end - eliminates 3 DB queries per board request.
- **In-memory card cache**: `GameService` caches all cards for winner computation; invalidated on generate/delete.
- **In-memory game_details cache**: `GameService` caches the `game_details` setting via read-through `GameDetails()` / write-through `SetGameDetails()`.
- **Incremental winner computation**: `computeWinners` skips cards already in the winners list.
- **Serialized game lifecycle**: `Service.Start/Draw/End` hold `opMu` so concurrent draws can't race on the called-numbers set or draw a duplicate number, and the `UNIQUE(game_id, number)` index on `called_numbers` (v43) is the database's backstop should that ever be bypassed.
- **Player state is client-side**: stamp marks stored in `localStorage` keyed by `stamps_{cardId}_{gameId}`.
- **Real-time updates**: the live hub publishes game/card/pattern/theme/settings changes as events, each to the viewers it is for; separate board and staff streams.
- **Draw delay**: admin can set 0-60s delay before boards receive a drawn number over their stream; staff with the Game page get it at once, and `GET /game` and the board hide it until then.
- **Schema versioning**: goose's version, which `PRAGMA user_version` always equals (each migration sets it, and `migrate` checks the two agree); a database newer than the binary is refused. The committed `backend/schema.sql` is what the migrations build (`TestSchemaIsCurrent`).
- **Lightweight endpoints**: `GET /api/cards` returns only IDs + player names (no board data); `GET /api/v1/board/{id}?preview` returns only the card (no game state).
- **Batch operations**: `GetCardPlayerNames()` fetches multiple cards in one query; `SaveCardsBatch()` uses transactions.
- **Randomness**: card IDs, boards and every draw (bingo, raffles, the garapon) come from `crypto/rand` through `internal/random`, which uses `math/rand/v2`'s bounding and shuffle over a `crypto/rand` source.
- **Crash isolation**: the goroutines that each `recover()` are the auto-draw loop, the scheduler sweep, and the deferred player-draw/half-time timers; the top-level wrapper in `ServeHTTP` logs a panic with its path redacted.
- **SQLite pragmas**: WAL mode, synchronous=NORMAL, busy_timeout=5000, cache_size=8MB, mmap_size=32MB, foreign_keys=ON, temp_store=MEMORY, carried in the DSN by `connectPragmas` in `store.go`, on two pools: a write pool of one connection with `_txlock=immediate`, which every write and transaction uses, and a read pool of four that is `query_only`. `beginImmediate` names the transactions that rely on taking the write lock up front; inside one, everything goes through the transaction, since a call on the write pool would wait for its one connection forever. `store/driver_test.go` checks all of this against the real driver.
- **Timestamps** (`store/times.go`): RFC 3339 in UTC to the second, `2026-11-09T21:30:00Z`, in a `DATETIME` column whose CHECK refuses any other form, `NULL` for none. SQL says "now" as `strftime('%Y-%m-%dT%H:%M:%SZ', 'now')`, never `CURRENT_TIMESTAMP` or `datetime()`. Go writes a client's time through `nullTime` (handlers check it first with `normalizeTimes`, a 400 when it isn't one), and reads a `DATETIME` straight into a string, which comes back as stored; a nullable one is read as `COALESCE(col, '')`, since the API sends `""` for none until the move to huma. `TestTimestampsInTheStoredForm` (`store/driver_test.go`) guards the paths the frontend and plugin parse.
- **Pre-built session handler**: `sessions.LoadAndSave(mux)` is constructed once in `New()` and stored as `sessHandler` - not rebuilt per-request.
- **Stream heartbeat and caching**: the server sends a `ping` event on every stream every 25s to keep reverse proxies from dropping it; a stream is `private, no-store` with no ETag, and the access log writes it once, when it closes. The CSP's `connect-src` has no `wss:`.
- **Rate limiter**: IP-based brute-force protection for login / registration / passkey-login (begin + finish). Its per-IP key comes from the same `logClientIP` logic the access log uses - `CF-Connecting-IP`/`X-Forwarded-For` trusted **only** from the loopback proxy peer, else `RemoteAddr` - so a client behind Cloudflare can neither share nor forge another IP's throttle bucket (`clientIP` delegates to `logClientIP`).
- **FontAwesome**: bundled via `@fortawesome/fontawesome-svg-core`; only the used Pro-kit icons are added to the library in `frontend/src/lib/fontawesome.ts`. Templates render them with the global **`<font-awesome-icon :icon="[prefix, name]" />`** component (`@fortawesome/vue-fontawesome`, registered in `main.ts`) - `['fad', ...]` duotone, `['fas', ...]` solid, `['fab', 'discord']` brands. The shared `ui/` primitives (AdminPanel/ManagerView/SubPageHeader/EmptyState) take an `icon?: [IconPrefix, string]` prop and forward it (name is `string` because Pro icon names aren't in FA's free `IconName`). CSS targets the rendered `.svg-inline--fa` (not `<i>`). Component tests stub `<font-awesome-icon>` globally via `vitest.setup.ts`.
- **Theme editor**: structured token editor (`ThemeTokenEditor.vue`) bound to the edited theme's `tokens` map; `lib/theme-tokens.ts` is the token source of truth. Themes are token-only (no free-form CSS); the applied `:root{}` is generated server-side (`store.TokensToCSS`) and locally for the live preview (`tokensToCss`). CodeMirror was removed.
- **Types from the document**: the SPA's types (`schema.d.ts`, openapi-typescript) and the plugin's client (`Api/SenpanApi.g.cs`, NSwag) are generated from the committed document by `task gen` and committed; CI's `generated` job fails on a stale copy. Never edit either by hand.
- **Typed API calls**: every call goes through `api` from `@/api/client` (openapi-fetch, typed from the generated schema), usually wrapped in `unwrap()`, which answers the data or throws an `ApiError` with the problem's `detail`. A file upload goes through `apiUpload` (`lib/api.ts`, XHR for progress), typed with the schema's response types; `refusedFiles` reads a 422's per-file reasons.
- **Global 401 handling**: `lib/api.ts` holds a registered handler (set in `main.ts`) that the client calls on any non-auth 401 -> redirect to `/admin/login` + "session expired" toast. Auth operations skip it so a bad-password login doesn't trigger it.
- **Themed confirm dialog**: use `await ui.confirm(message, opts)` (renders `ConfirmModal.vue`) instead of the native `window.confirm`.
- **Uploaded fonts**: `applyUploadedFonts()` (`lib/theme.ts`) writes one `<style id="uploaded-fonts">` with an `@font-face` per file from `FONT_BASE_URL` (`stores/fonts.ts`), so a font is registered app-wide (board/header + every preview), not per-component. It then measures each loaded font via the Canvas TextMetrics API and, for fonts with oversized vertical metrics, rewrites the rule with `ascent-override`/`descent-override`/`line-gap-override` to clamp the box (`clampFontMetrics()` - pure + unit-tested). The Settings preview and the Font Upload live preview both rely on these shared rules, so previews always match what players see.
- **Discord embeds (shared plumbing)**: every webhook-posting feature (affiliates, tea rooms, announcements, reading-list items, ema) builds its embed with the fluent `discord.NewEmbed()...Build()` builder in `internal/discord` and posts through the `discord.Sender` (`PostEmbed`, or `Post` for a payload with an optional `content` mention + `allowed_mentions`, or `PostFile`). The builder auto-truncates to Discord's per-field limits, skips empty fields, and caps at 25 fields, so callers pass raw content. New embed shapes add a builder chain, not new transport. An operation turns a failed post into its answer with `discordProblem`, and declares 429, 500 and 503. Times are emitted as Discord `<t:unix:...>` tokens so each viewer sees their own zone.
- **Timezone-anchored scheduling**: announcements keep the admin's wall-clock input in the `*_local` columns with its IANA `timezone`, the recurrence math is `nextAnnouncementOccurrence` and `nthWeekdayOfMonth`, and `server/tzdata.go` blank-imports `time/tzdata`.
- **Background scheduler**: `RunAnnouncementScheduler` is launched as a goroutine in `main.go` against a shutdown-cancelled context, ticks every 30s, sweeps once on startup, and is crash-tolerant (unset webhook -> left pending; failed post -> retried; `skip_count`/recurrence advance the cursor). The due-row query is index-backed (`(active, next_post_at)`). It uses the ticker/sweep loop in `server/scheduler.go` (`runScheduler(ctx, name, interval, sweep)`) - the `Run...Scheduler` is just a one-line wrapper supplying its interval + sweep func.
- **Uploads** (`internal/uploads`, `internal/media`, `internal/api/uploads.go`): a multipart operation takes `huma.MultipartFormFiles` with its `MaxBodyBytes` set (over it: 413); the multi-file ones declare their parts `application/octet-stream` so a stray file is skipped, not the batch refused, and a single cover declares `image/*`. The handler never trusts the extension or the browser's type: `uploads.CheckImage` judges the bytes, refusing an image over 40 megapixels or 16384 px a side before any full decode, re-encoding JPEG (EXIF orientation applied first) and PNG, and checking GIF (a frame and total-pixel pre-scan) and WebP, which are kept as sent. An image whose extension doesn't match its bytes is accepted under its name, as before. Parts up to 16 MiB stay in memory (`humago.MultipartMaxMemory`, so the ema artist post is never written to disk). Uploads **keep the uploaded filename** (a same-named upload overwrites), so an endpoint that auto-cleans its files must guard against a name shared by another record (see `removeUnusedCovers`). Absolute URLs are built from the request's site, the name path-escaped. Most upload areas live under the image library (System -> Images); add a bespoke area only when it isn't a managed category.
- **AniList proxy**: `GET /api/v1/book-clubs/{club}/lookup?q=|id=` proxies the public AniList GraphQL API server-side (avoids browser CORS + centralizes field mapping); neither or both of `q` and `id` is a 422. The endpoint is configurable via the `anilist_api_url` setting. That URL is validated against `bookclubs.IsAllowedAniListURL` (https on an `anilist.co` host) **at save time and again at read time** (a disallowed stored value falls back to the default), and the `internal/anilist` client re-validates **every redirect hop** against the same allowlist - SSRF defense in depth so the outbound POST can't be pointed at an internal service or cloud-metadata host. A failure is 503 (unreachable), 429 (rate limited, with `Retry-After`) or 500, never 424. `anilist.ToSuggestion` maps a Media node to a reading-list item suggestion.
- **Per-club registry**: book clubs live in one registry - `bookclubs.Clubs` (`internal/bookclubs`) on the backend and `BOOK_CLUBS` (`lib/constants.ts`) on the frontend. Adding an entry wires up its route, sidebar button, comments label, page permission, and its reading-list webhook (a field on its page, `discord_webhook_url_<slug>`) automatically.
- **Integrations**: `GET /api/v1/settings` is public, so the Google Fonts key is not among the settings: it is the one integration (`GET`/`PUT /api/v1/settings/integrations`), an admin's alone. A Discord webhook is read and set through its own page's `/webhook` operation (affiliates, tea rooms, each book club), guarded by that page; an announcement type carries its own.
- **datetime conversion**: `lib/datetime.ts` converts between stored UTC (RFC-3339) and the LOCAL wall-clock value an `<input type="datetime-local">` expects (raffle windows); announcement forms instead send the raw wall-clock + an explicit IANA zone the backend resolves.
- **Snappy admin navigation (load-gate)**: re-entering an admin tab would otherwise re-run its data load every time. `admin.setTabFromRoute()` routes each tab's load through `loadFresh(key, ...)` (backed by `lib/freshness.ts`), which skips the refetch when that dataset loaded within a 30s TTL - so tab switches are instant instead of re-spinning. The gate lives at the **navigation layer only**: store loaders still always fetch, so post-mutation refreshes (which call the loaders directly) show edits immediately, and live game/cards/patterns stay current over the staff stream. `bookclub.openClub()` uses its own per-club freshness so re-entering the same club keeps the open list while switching clubs refetches.
- **Admin UI primitives** (`components/common/ui/`): admin forms and tables are built from a small set of presentational components so every screen is consistent by construction - don't hand-roll `.field`/label/table markup. Use `AdminPanel` (the `.admin-panel` card; pass `title`+`icon` or supply a custom header slot), `FormField` (label + control slot + optional `help`/`#help`; controls auto-stretch full-width, so no `.field-input-full` on direct children), `FormRow` (equal-width side-by-side fields - for unequal widths use a plain `.flex-row` and pass `style="flex:..."` to each `FormField`, which falls through to its root), `FormActions` (button row; `align="start|end|between"`), `DataTable` (generic sortable table - `columns`/`rows`/`row-key`, `#cell-<key>` slots, `#empty` slot, optional `rowClass`. **It owns the row pipeline: filter -> sort -> paginate**, on TanStack Table **v9** (headless - row model only, no library markup or CSS). v9 composes the engine from explicit `tableFeatures({...})` rather than bundling everything: each capability is opted in and every row model (`createFilteredRowModel`, `createSortedRowModel`, `createPaginatedRowModel`, `createFacetedRowModel`, `createFacetedUniqueValues`) must be listed AFTER the feature it depends on; the core row model is implicit. The row type is `T extends Record<string, any>`, matching v9's own `RowData`. Pass the FULL list, never a page: sorting must run before pagination, or a header click reorders only the page on screen. Optional `filter`/`filter-fn`, `page-size` + `v-model:page`, `default-sort`; it reports `{ total, totalPages, facets }` via `@update:view` for the toolbar count, facet dropdowns + `PaginationBar`. A first click sorts ascending and sorting is locale-/numeric-aware. Opt-in extras, each off unless asked for: `resizable` (drag a column edge; declared widths and auto columns are untouched until one is actually dragged), `selectable` + `v-model:selected` (checkbox column; the header box selects every row matching the CURRENT filter, not just the page - bulk delete confirms once for the set), `group-by` (keeps rows sharing a value contiguous as a PRIMARY sort key, so column sorting still orders within each group), `column-filters` + a `facetable` column (exact-match facet filtering), and `exportCsv(name)` via a template ref (the filtered rows in sort order across ALL pages, RFC 4180 quoted, UTF-8 BOM; skips `noExport` columns); the trailing **actions column uses `label: ''`** (no header) **+ `align: 'right'`**, and its `#cell-actions` content is wrapped in **`.row-actions`** (flex, `gap`, right-justified) - match this for every table with row actions, never a bare inline span), `DataTableToolbar` (the ONE toolbar above a table - fixed order `[search filters] ... [per-page count actions]`, with `count`/`total`/`noun` driving a uniform count string; never hand-roll a `.flex-toolbar` above a `DataTable`, that is exactly how seven tables drifted apart), `PaginationBar`, and `EmptyState`. The CSS for these lives in `styles/utilities.css` (the "FORM & TABLE PRIMITIVES" and "MANAGER LAYOUT" blocks) and uses theme tokens so themes restyle them.
- **Manager model** (`components/common/ui/`): every admin tab that manages a collection follows one shape. `ManagerView` = the list page shell (`.admin-panel` + `title`/`icon` header, `#actions` slot top-right for buttons like "Manage Categories" / "+ New", optional `#toolbar` slot for `SearchInput` + filter selects, default slot = the list, `#pagination` slot). `ListRow` = one item (`#media` left, body default, `#actions` pinned **far right**); stack rows in a `.list-rows` container. `SubPageHeader` = a sub-page header (title + Back, emits `back`). Convention: the tab holds a `screen` ref (`'list' | 'new' | ...`); `#actions` buttons switch `screen`; each sub-screen opens with `SubPageHeader @back`. Reference implementations: `PatternsTab` (list/new/categories), `AnnouncementsTab`, `PresetsTab`, `RafflesTab`, `BookClubTab`. Manage Cards / Fonts / Carrd keep their bespoke shapes (chip grid / sortable table / image manager) rather than the full ManagerView model, but still compose the shared primitives: Fonts uses `DataTable` (sortable, `rowClass` for the preview-selected row) + `SearchInput`; Cards + Carrd use `SearchInput` and the `.chip` object. What stays genuinely page-unique: the Fonts live-preview panel, and Carrd's breadcrumb / drag-drop dropzone / square asset grid.
- **Theme tokens** (`:root` in `styles/tokens.css`, mirrored in `lib/theme-tokens.ts` + Go `themes.TokenOrder`): role-descriptive CSS custom properties. Every colour and effect token below is overridable by saved themes + the theme editor; the non-colour ones are not. Backgrounds: `--page-bg`, `--panel-bg` (cards/panels), `--panel-raised-bg` (nested/row/chip surface), `--control-border` (control & divider outline - readable on _both_ panel surfaces), `--input-bg` (form-control fill). Accents: `--accent` / `--accent-hover`, `--accent-2` / `--accent-2-hover`, `--highlight` (called numbers, headings, gold trim). Text: `--text`, `--text-muted`, `--text-on-accent`, `--text-on-fill`. Status: `--success`, `--danger`, `--warning` (skip badges/alerts; use `color-mix(in srgb, var(--warning) N%, transparent)` for tints). Board: `--board-cell-bg`, `--board-cell-hover-bg`, `--board-free-bg`, `--board-gradient-start/end`. Effects: `--modal-overlay`, `--shadow`, `--highlight-glow`. Non-colour, and not themeable (see **Tokens** under **CSS conventions**): `--radius`, `--radius-media`, `--body-font`, `--header-font` (the header font is set through System -> Settings instead). Use `--control-border` for outlines and `--input-bg` for control fills (the old overloaded `--surface2`-as-border was invisible on nested boxes), so no one-off contrast overrides are needed inside a `--panel-raised-bg` box.
- **Class rename map** (structural classes consolidated 2026-06): `.entries-table`/`.winners-log-table` -> `.data-table`, `.msg-block` -> `.empty-state`, `.btns` action rows -> `.form-actions`; `.field-input-full` is retained only for controls nested inside a flex row (not a direct `.field` child).
- **Token rename (2026-06)**: the theme tokens were renamed to role-descriptive names (`--surface`->`--panel-bg`, `--surface2`->`--panel-raised-bg`, `--border`->`--control-border`, `--field-bg`->`--input-bg`, `--primary`->`--accent`, `--secondary`->`--accent-2`, `--gold`->`--highlight`, `--text-dim`->`--text-muted`, `--text-on-primary`->`--text-on-accent`, `--text-on-toast`->`--text-on-fill`, `--bg`->`--page-bg`, `--board-cell`->`--board-cell-bg`, `--overlay`->`--modal-overlay`, `--shadow-color`->`--shadow`, `--glow-color`->`--highlight-glow`; `--warning` added). **Saved themes must use the new names** - the live DB themes were migrated at the time; a theme still using old token names silently falls back to the `tokens.css` defaults.
- **CSS direction - objects, not pages**: app.css is migrating away from page/feature sections (`HOME VIEW`, `RAFFLES`, `ADMIN GAME`, ...) toward a small vocabulary of reusable **objects** composed across pages - the `components/common/ui/` primitives are the first wave. When styling new UI prefer, in order: (1) an existing primitive/object class, (2) a theme token on existing markup, (3) a new shared object class in the Primitives band - and only as a last resort a page-scoped rule. **Don't** re-declare control chrome inline: form controls already inherit the global `input,select`/`button` rules (`2px solid var(--control-border)`, `--input-bg`), so an inline `style="border:1px solid var(--panel-raised-bg)"` on a `<select>` is both redundant and off-token. **Don't** add a per-feature one-off that duplicates an existing object. **Never fill an object or button intent with a _surface_ token** (`--panel-bg`/`--panel-raised-bg`): a fill that matches the box it sits on vanishes (the recurring "ghost button" bug - a low-emphasis control blending into its container). Colored intents use semantic colours; the neutral button uses **`--control-border`** (a _non-surface_ outline token tuned to read on _both_ panel surfaces), so nothing can blend into the surface beneath it. **Prefer the small helper utilities over inline `style=`** for common single properties: `.nowrap`, `.w-full`, `.m-0`, `.fw-normal`, `.text-xs`/`.text-sm`, `.ta-center`/`.ta-left`/`.ta-right`, `.mb-*`/`.mt-*`, `.push-right`, `.flex-*` (in the utility band). Inline `style=` is fine for genuinely per-instance values - dynamic `:style` bindings (colour swatches, live font previews) and one-off layout (`flex: N; min-width: Xpx` form columns) - but not for repeated static literals (until the static inline styles move; see the departures above). Layout stays a stylesheet of object classes, not Tailwind; themes override only tokens, never classes, so the objects can be refactored freely. Extracted objects so far: the `ui/` form/table/manager primitives; **`.card`** (shared "bordered surface tile" chrome - `home-card`/`media-card`/`saved-pattern` only set their own size/radius/hover; new tiles can use `.card` directly, but it carries **no padding** by design - the image tiles bleed their picture to the border and pad `.media-card-body` beneath it - so a card of plain content needs **`.card--padded`**, or its text ends up against the border); **`.chip`** (raised, rounded, clickable pill - transparent border -> `--highlight` on hover; `.chip-del` = muted->danger delete affordance; shared by the card-id chips and the Carrd project/sub-folder chips, each keeping only its own extras); **`.toggle-btn`** (segmented/selectable pill - transparent border -> `--accent` on hover, `.is-active` = `--accent` fill, `.toggle-btn--sm` the small size; shared by the announcement weekday picker + book-club view switcher, stacked in a flex container); **`.del-x`** (prominent "x" delete glyph, `--danger` -> `--text` on hover - the loud counterpart to `.chip-del`; used by card-id + saved-pattern chips, consumer adds positioning); the **button system** - ONE solid shape; intent = fill colour only, never size/border (the old ghost/danger outlines made bordered buttons read as heavier). Var-driven: each intent sets `--btn-fill`/`--btn-text`, a shared rule applies them, and the hover **darkens the fill via `color-mix`** - so the five semantic colours + the raised/border surface cover all six intents with **no per-state or per-intent tokens added** (the deliberate answer to "we don't have enough colour variables": derive, don't multiply tokens). Six intents by _consequence, not verb_: **`.btn-confirm`** (save/add/edit/create -> `--success`), **`.btn-action`** (send/generate/publish/set-active/start -> `--accent`), **`.btn-view`** (open/view/copy/preview/browse -> `--accent-2`), **`.btn-caution`** (skip/reset/end-game -> `--warning`), **`.btn-danger`** (delete/remove -> `--danger`), **`.btn-neutral`** (cancel/close/back/toggles/field-clears -> `--control-border` - a non-surface token, so neutral stays visible even when it sits on a `--panel-raised-bg` box); `.btn-sm`/`.btn-lg` size modifiers; `.btn-danger` keeps its name. The old `.btn-primary`/`.btn-secondary`/`.btn-ghost` are **removed** - don't reintroduce them (intent = fill colour, never an outline/ghost); **`.badge`** (uppercase pill chrome - `status-badge`/`live-badge`/`conn-badge`/`style-active-badge`/`style-public-badge`/`changelog-badge` only set colours/margins; `nav-count` is intentionally _not_ a badge, it's inline count text) **+ shared state modifiers `.badge--accent`/`--muted`/`--warning`/`--success`/`--danger`** (theme-aware `color-mix` tints; the Announcement + Book-Club status pills compose `class="badge badge--*"` - no per-page badge colours); **`PatternPicker.vue`** (`components/common/ui/`, `v-model` = selected ids) which both GameTab and the Preset editor use, so the win-pattern picker can never desync; **`.section-heading`** (the `--highlight` sub-heading, level 3 of the heading scale); the **toolbar** object (`.flex-toolbar`, with `.manager-toolbar` = same + bottom spacing); the **`.color-picker`** skin (wrap the `@ckpack/vue-color` Chrome picker in `.color-picker` - global descendant rules re-map it to theme tokens, beating the lib's own styles; used by StampColorPicker and the Themes token editor's alpha tokens); **`.captioned-figure`** (a labelled reference screenshot - `<figure>` + `<figcaption>` + one `<img>`; the "where to go and do this" pictures that sit under the instructions they illustrate: a stamp rally's Where to Redeem, a raffle's Where to Pay); the **media** objects `.img-picker` / `.img-thumb` (+`img`/`:hover`/`.is-active`) / `.media-empty` (placeholder) / **`.media-cover`** (cover/preview image chrome; size set by a modifier - `--wide` 16:9 for announcement + event covers, `--book`/`--book-sm`/`--book-lg` portrait for reading-list item/lookup-result/form-preview covers; pair with `.media-empty` for the no-image box); **`ImagePicker.vue`** (`ui/`, `v-model` = the image reference, `valueKey` `'path'` or `'url'`) - the preview, Remove button and central-library browser every admin image field uses, with an Upload button for `system-images` holders; **`ColorPicker.vue`** (`ui/`, `:value` + `@change` -> `{rgba,hex}`) wrapping the lazy `@ckpack/vue-color` Chrome picker with the `.color-picker` skin (used by StampColorPicker and, for alpha tokens, the Themes token editor); and **`EmojiPickerModal.vue`** (`components/common/`, render with `v-if`, emits `select`(char)/`close`, optional `allow-clear`) wrapping the lazy `vue3-emoji-picker` in a themed modal (used by the player stamp shape picker + the announcement Discord-button emoji field). Note: a global object can lose to a component's _scoped_ class on the same element (scoped `[data-v]` adds specificity) - e.g. a size class setting `display` will override `.media-empty`'s flex, so size classes that share an element with an object must not redeclare the object's properties. Carrd's breadcrumb / dropzone / asset grid + Fonts' live-preview panel stay intentionally bespoke (page-unique, correctly local). `DataTable` takes an optional `rowClass` (string or `(row)=>class`) to mark rows (e.g. the Fonts preview-selected row). Still page-bound: home/raffle/board section CSS that doesn't reuse a primitive.
- **Carrd path safety**: every Carrd folder/sub-path/filename received from the client is validated (`internal/uploads`' name and path checks, used by `media.Carrd`) and the resolved absolute path is checked to stay within the project dir - guarding against traversal. A bad project name in a URL is a 404, a bad `path` (query or body) a 422. Same-name uploads intentionally overwrite.
- **SVG sanitization**: uploaded theme/flourish SVGs are rendered **inline** (via `v-html`, e.g. `CornerFlourish.vue` and the player-board flourishes), so they are sanitized server-side in `uploads.SanitizeSVG` (`internal/uploads/svg.go`) before storage/serving: `<script>`/`<foreignObject>`/`<style>` subtrees are dropped, `on*` and unsafe `href`/`src`/`xlink:href` attributes and dangerous SMIL animations (`<set>`/`<animate>` targeting `href`/`src`) are stripped, and `url()`/`@import` are removed from inline styles. The frontend also defensively sanitizes fetched SVG before `v-html` (`CornerFlourish.vue`), and theme-token values are injection-checked before they reach a `:root{...}` preview (`lib/theme-tokens.ts`).

## Key files to inspect first

1. `backend/internal/server/server.go` - Server struct, the mux, middleware, the session layer (`currentUser`, `withWho`), `adminMutationResource`, broadcast helpers
   - `backend/internal/api/{signin,passkeys,me,users}.go` + `backend/internal/auth/` + the session layer in `backend/internal/server/{server,tokens}.go` - read before touching sign-in, registration, accounts, or page gating
2. `backend/internal/db/queries/<domain>.sql` - every domain's SQL (sqlc); `backend/internal/store/` holds the migrations and helpers; `store.go` holds the `Store` struct, `New()`/`Close()`, pragmas, and shared helpers
3. `backend/internal/game/game.go` - The game engine: start, draw + winner compute + cache, state reads, half-time, in-memory caching; `events.go` says who hears each event
4. `backend/internal/bingo/card.go` - Card/board generation algorithm with column-range constraints
   - `backend/internal/live/hub.go` + `backend/internal/api/live.go` - the live feed: the hub, the two streams and the audiences
5. `backend/internal/model/` - All domain types, one file per domain (Card, Pattern, GamePreset, BingoGameState, BingoDrawnNumber, Raffle, ReadingList(+Item/Source), AnnouncementType, AnnouncementRole, Announcement, WinnersLogEntry, etc.)
   - `backend/internal/discord/` (`sender.go`, `embed.go`, `errors.go`) + `backend/internal/api/discord.go` - the Discord embed builder, transport and typed errors (read before touching any webhook feature)
6. `frontend/src/router/index.ts` - route map, lazy route components, admin auth guard + tab sync
7. `frontend/src/App.vue` + `frontend/src/composables/useLiveEvents.ts` + `frontend/src/lib/feed.ts` - root shell (`<RouterView>`), live event dispatch
8. `frontend/src/api/client.ts` - the client for the operations on huma; `frontend/src/lib/api.ts` - the 401 handler and `apiUpload`
9. `frontend/src/stores/*.ts` - all client state + actions (mirror of the old app.js data/methods)
10. `frontend/src/assets/app.css` - stylesheet **index** that `@imports` `assets/styles/*.css` (tokens, base, utilities, components, mapeditor, player, admin, responsive - split by domain); imported in `main.ts` (content-hashed by Vite)

## Extending the project

- **New API endpoint**: an operation on huma in `internal/api`, in its domain's file (`huma.Register` with an `operationId`, a summary, tags, its guard middleware, and named input and output types whose fields are all described with `doc:` tags), then `task gen` and commit `frontend/src/api` with it; the SPA calls it through `@/api/client`. The release C plan (`plans/release-c.md`) has the checklist each operation meets.
- **New request or response type**: an operation's own types live beside it in `internal/api`, and `task gen` documents them and regenerates the SPA's types and the plugin's client from the document. `internal/model` keeps only the few types shared below the API (accounts, log entries, the participant label, the stamp types); add there only what the store or a service needs too.
- **New store method**: add the query to the domain's `internal/db/queries/<domain>.sql` and run `task gen` (a hand-written store method only for a test helper), taking `ctx context.Context` first and returning typed structs. Keep timestamps in the one form (see **Timestamps** above): write a client's through `nullTime`, read a nullable one as `COALESCE(col, '')`, and add the path to `TestTimestampsInTheStoredForm` when the frontend or plugin parses it. Reads go through `s.read`, writes through `s.write`. A multi-statement write goes through `beginImmediate`.
- **New migration**: a new file in `store/migrations/`, numbered after the last (`00074_name.sql`), starting `-- +goose Up` and ending `PRAGMA user_version = 74;`; a Go migration only where it needs logic. A table rebuild follows SQLite's procedure (`-- +goose NO TRANSACTION`, as `00073_timestamps.sql` does). Then `go run ./cmd/schema-gen` in `backend/` and commit `schema.sql` with it.
- **New admin tab**: add the tab id to the `AdminTab` union in `frontend/src/stores/admin.ts` (mapping its prefix to a section in `setTabFromRoute()` + any per-tab data load - wrap the load in `loadFresh('<key>', ...)` so revisits stay snappy), add a child route with `meta.tab` in `router/index.ts`, add a sidebar link (gated with `v-if="can('<key>')"`) in `components/admin/AdminSidebar.vue`, and create a `components/admin/<Tab>.vue` referenced by the route's lazy `import()`. **Wire its permission too** (see "New admin page permission" below) unless it's admin-only. For a "manage items" tab use the **manager model** (`ManagerView` + `ListRow` + `SubPageHeader` + `SearchInput`, with a `screen` ref for sub-pages - see `PatternsTab.vue`); otherwise wrap content in `<AdminPanel>`. Either way build forms/tables from the `components/common/ui/` primitives (`FormField`, `FormRow`, `FormActions`, `DataTable`, `PaginationBar`, `EmptyState`) rather than hand-rolled markup.
- **New admin page permission**: the page-permission key **must equal** the tab id. Add the constant + include it in `Valid()` in `internal/permissions`; guard the page's operations with `permitted(api, perm...)` (`internal/api`); add an entry to `ADMIN_PERMISSIONS` in `frontend/src/lib/constants.ts` (label + section) so the Users-page editor, sidebar gating, and router guard all pick it up. (An **admin-only** page instead guards with `requireAdmin` and is left out of `ADMIN_PERMISSIONS` - see `system-users`.)
- **New route/view**: add a record to `routes` in `router/index.ts` with a lazy `component: () => import(...)`; mark `meta.requiresAdmin` if it needs auth.
- **New live event type**: a struct embedding `live.Envelope` with a `Name()` and a typed `payload`, added to `staffEvents()` and/or `boardEvents()` in `internal/api/live.go` (or the game's in `internal/game/events.go`); publish it with `hub.Publish(event, audience)` naming who may hear it, then `task gen`. Add a handler for it in `composables/useLiveEvents.ts` (the handler maps are typed from the document, so a missing one fails `vue-tsc`), and describe it in API.md.
- **New Discord-embed feature**: build the embed with `discord.NewEmbed()...Build()` (extend the builder if a new shape is needed) and post with `h.deps.Discord.PostEmbed(ctx, target, webhookURL, embed)`; answer a failure with `discordProblem` and declare `discordErrors`; never write transport code per feature.
- **New book club**: add an entry to `Clubs` (`internal/bookclubs`) **and** `BOOK_CLUBS` (`lib/constants.ts`) - keep the slug identical. The route, sidebar button, comments label, its webhook (on its page), and its `bookclub-<slug>` page permission are then derived automatically.
- **New setting**: add the key and its default to `internal/settings`, then the field to `SettingsInput` (`internal/api/settings.go`) with its `doc` and validation tags, read in `loadSettings` and written in the save; run `task gen`, then add it to `DEFAULT_SETTINGS` (`lib/constants.ts`) and `SettingsTab.vue`. A value only an admin may read belongs in `Integrations` instead.
- **New test**: colocate a `*.test.ts` next to the code; import `{ describe, it, expect }` from `vitest`; run `npm run test`. Add it to the same gate CI runs. (Go: colocate `*_test.go` and run `go test ./...`. A test that needs a database opens a migrated copy, `storetest.Path(t)` or `storetest.Open(t)` (`internal/store/storetest`; `migratedPath(t)` inside `package store`), never a fresh one: the migrations run once per test binary, since under `-race` migration 73 alone takes over a second, and a fresh database per test ran the server package past CI's ten minutes. Only the migration tests migrate from scratch.)
- **New plugin feature/tab** (`plugins/SenpanCompanion/`): the plugin is API-only - add the operation's id to `includedOperationIds` in `nswag.json` and run `task gen`, so the generated `SenpanApi` gains the call and its types, against an **existing** server operation; never add client<->client logic. A page that shows a resource marks itself stale on its `resource_changed` (routed in `MainWindow.OnResourceChanged`); code with no Dalamud in it goes in `Core/`, where `SenpanCompanion.Tests` can cover it. A new ImGui tab subclasses `Windows/TabBase` and is registered in `Windows/MainWindow.cs`; **route every post-await UI write through `Apply(() => ...)`**. Keep the build at **0/0 warnings** and run `dotnet format` before pushing (CI gates both). If the backend gained a new endpoint, expose it on the server first (see **New API endpoint**).
