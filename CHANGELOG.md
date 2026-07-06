# Changelog

All notable changes to the **Senpan App Suite** are recorded here.

The **frontend** (Vue SPA) and **backend** (Go API) are versioned independently
with [Semantic Versioning](https://semver.org/) and tracked in their own sections
below — a change usually touches only one side, and they deploy separately. The
admin dashboard shows both live versions (sidebar footer) so operators can confirm
a deploy left the two halves compatible.

**Sources of truth**

- Frontend version → `frontend/package.json` (`"version"`), baked into the build
  and read via `frontend/src/lib/version.ts`.
- Backend version → `backend/internal/version/version.go` (`Version`), served at
  `GET /api/version`.

**Compatibility rule:** the SPA and API are compatible while their **MAJOR**
versions match. Bump MAJOR only for a breaking change to the JSON/WebSocket API
the SPA depends on; MINOR for backward-compatible additions; PATCH for fixes.
When you change one side, bump its version and add an entry under its section.

The format follows [Keep a Changelog](https://keepachangelog.com/).

---

## Frontend

### [3.4.0] — 2026-07-05

#### Added

- **Server Logs admin tab (System → Logs, admin-only).** A live-tailing viewer
  over the backend's structured log: loads a filtered snapshot from
  `GET /api/logs`, then live-appends new lines over the admin WebSocket. Runs on
  the shared `DataTable` with a per-page selector (25/50/100/200) and pagination.
  Common HTTP-request fields are promoted to typed, colored columns — **Method**
  badges (GET/POST/PUT/PATCH/DELETE/WS), **Status** (2xx green → 5xx red),
  formatted **Duration**, and **IP** — with any other fields previewed inline and
  click-to-expand to full JSON. Includes a minimum-level filter + debounced text
  search, a pause/resume "Live" toggle, and a **Debug On/Off** button that flips
  the server's runtime log level live (`POST /api/logs/level`) — capture detail on
  demand and quiet it again without a restart. The in-memory buffer self-caps at
  1000 entries.

#### Changed

- **`DataTable` gained opt-in expandable rows + column widths** (used by the logs
  viewer). Providing a `#detail` slot makes each row toggle a full-width detail
  row on click (cell slots receive an `expanded` flag), and a column can set a
  fixed `width`. Both are backward-compatible — tables without a `#detail`
  slot/`width` are unchanged.

#### Fixed

- **API errors with no JSON body now show the HTTP status** — a non-JSON gateway
  failure (e.g. an empty/HTML `502` from Cloudflare/Apache) surfaces as `Request
  failed (HTTP 502)` instead of a bare `Request failed`; our own `{ "error": … }`
  messages are still preferred when present.

### [3.1.0] — 2026-07-03

#### Changed

- **Image picker browses the whole image library.** The picker used by the
  announcement, raffle, garapon, affiliate, stamp-rally, and theme-flourish
  editors now has its own category dropdown: any image in any category can be
  picked from any editor (announcements keep storing absolute URLs for Discord
  embeds; the theme flourish pickers only offer `.svg`). When editing, the
  picker opens in the category of the currently selected image. The per-feature
  image lists (and their store plumbing) are gone.
- **All image categories are editable.** The "Permanent" badge and the disabled
  Edit/Delete buttons on System → Images → Manage Categories are gone — every
  category can be renamed and deleted (deleting still removes its folder and
  files, so existing references lose their images).
- System → Images now lists `.svg` in the upload help + file-browser filter
  (SVG uploads were already accepted and sanitized server-side).
- Image caches refresh live on any image change, whichever admin tab is open,
  so open pickers pick up another admin's uploads.

### [3.0.0] — 2026-07-02

#### Changed

- **BREAKING (with backend 3.0.0): protected font serving.** Uploaded fonts are
  no longer referenced by static `https://fonts.senpan.cafe/<file>` URLs. The
  app registers `@font-face` rules from the rotating tokenized URLs in the
  settings payload (`uploaded_fonts` is now `[{name, family, token}]`), loading
  fonts **same-origin** via `/api/fonts/pub/f/<token>` — so the header/board
  font picker and player-facing fonts always work regardless of any external
  allowlist. Requires backend 3.0.0.
- **Font Upload reworked around font families.** Files sharing a base name
  (e.g. `Jasper.ttf` + `Jasper.woff2`) appear as ONE font. The table stays
  slim — **CSS Name**, **Serves** (the actual served format:
  TTF/OTF/WOFF/WOFF2/EOT; ✦ marks the auto-converted copy), **Modified**, and
  actions — and a new **Edit** modal holds everything else: the CSS
  `font-family` name (blank = base-name default; flows through the kit, the
  app, and the picker), the served-version picker (any uploaded format or the
  converted WOFF2), the font's **own allowed-sites list**, and the files
  themselves (per-file rename/delete with sizes).

#### Added

- **Embed on External Sites** panel: copy the permanent `kit.css`
  `<link>` snippet for Carrd sites; each site automatically receives only the
  fonts whose allowed sites include it. Per-font **Copy URL** copies the
  served version's tokenized link (expires in 1–2 weeks) instead of a
  permanent direct download link.
- **Live-preview format switch**: preview any font's text sample in each of
  its actual formats (e.g. TTF vs the converted WOFF2) to sanity-check a
  conversion before serving it.

### [2.2.0] — 2026-07-02

#### Added

- **Sign in with a passkey.** The login page offers a usernameless "Sign in with
  a passkey" option (WebAuthn discoverable credentials), and every account can
  add / name / remove passkeys under **User Options → Add Passkey**. Passkeys
  complement the password — both continue to work.
- **Cloudflare Turnstile on public raffle sign-up.** When Turnstile is
  configured, the raffle entry form shows the bot check and requires it before
  submitting, matching the admin login.

### [2.1.0] — 2026-07-02

#### Fixed

- Guard the announcement form's Main/Thumbnail image pickers so opening the form
  before the image lists finish loading no longer throws.
- Detail loaders (raffles, garapons, stamp rallies, book-club lists) ignore stale
  responses via a per-loader request token, so quickly switching between items
  can't settle the view on the wrong record.
- API requests now time out after 30s instead of leaving a spinner (and the
  caller's promise) pending forever on a hung network.

#### Added

- `timeoutMs` option on the API client (pass `0` to disable), with a default
  request timeout that surfaces a clean error on abort.

#### Security

- Validate the post-login `?redirect=` value as a same-origin path before
  navigating (falls back to `/admin`).
- Clear the full auth session (not just the `isAdmin` flag) on a 401, so stale
  permission gating can't linger after a session expires.

#### Accessibility

- Clickable cards, chips, and collapsible headers (raffles, stamp rallies,
  garapons, the game winner chip, the pattern-picker group headers) are now
  keyboard- and screen-reader-operable (`role`/`tabindex`/keydown).

#### Changed

- Removable repeater rows (garapon prizes, affiliate hours/owners, announcement
  buttons, reading-list sources) key on a stable id so removing a row can't
  rebind inputs to the wrong logical row.
- RafflesTab now reuses the shared `DataTable` + `useDataTableView` composable
  instead of hand-rolled table/pagination state; extracted shared `slugify`,
  `formatSize`, and `withLoading` helpers.
- Replaced `vuedraggable` with `vue-draggable-plus` for pattern/announcement
  drag-and-drop. The old library's bundled runtime used `new Function()` (which a
  strict CSP `script-src` would block); the modern one doesn't, so the site can
  enforce CSP without `'unsafe-eval'`. Also ~200 KB smaller in that chunk.

### [2.0.0] — 2026-06-30

#### Changed

- **API client migrated to hybrid REST (breaking).** Every admin data call in
  `src/lib/endpoints.ts` moved off the old action-dispatcher POSTs to REST
  methods — `apiGet`/`apiPost`/`apiPut`/`apiPatch`/`apiDelete` (new helpers in
  `src/lib/api.ts`) against resource paths (`/api/<resource>/{id}`, nested
  sub-resources, `POST …/{id}/<verb>` commands such as `close`/`reopen`/
  `activate`/`send`/`pick-winner`, `DELETE …/all` bulk deletes). Store call-site
  names/signatures were preserved, so components are unchanged; the wire calls
  now line up with the backend 2.0 contract. Requires **backend ≥ 2.0.0**.
- **Book-club calls carry the club slug in the path.** The `bookclub` endpoint
  group targets `/api/book-clubs/{club}/reading-lists…`; the store threads its
  `activeClubSlug` through, so component-facing signatures are unchanged.

### [1.5.2] — 2026-06-30

#### Changed

- **Response types are now generated, not hand-maintained.** The hand-written
  response envelopes in `src/types/api.ts` were replaced by re-exports of the
  tygo-generated types (the backend `model` structs are now the source of truth),
  eliminating the backend↔frontend drift surface. No behavioural change; the
  public garapon view is now precisely typed as `PublicGarapon`.

### [1.5.1] — 2026-06-30

#### Fixed

- **Player stamp persistence is crash-safe.** Loading stamps now tolerates corrupt
  or tampered `localStorage` (a bad value starts the board clean instead of
  throwing during load), and saving stamps no longer throws out of the
  high-frequency toggle path when storage is full — bringing both in line with the
  custom-stamp save, which already degraded gracefully.

### [1.5.0] — 2026-06-29

#### Added

- **Personal access tokens** (User Options → **Access Token**). Generate a token from the
  account menu so an external client — such as a Final Fantasy XIV Dalamud plugin — can
  sign in to this server as you, with your exact page permissions. The modal shows the
  token's prefix and its created / last-used times; the secret itself is revealed **once**
  at generation (with a copy button) and can be **regenerated** (invalidating the old one)
  or **revoked**.

### [1.4.0] — 2026-06-29

#### Added

- **Garapon ↔ Stamp Rally linking.** A Garapon can optionally **link to an open Stamp
  Rally**; issuing a participant a drawing link then also issues them a Stamp Rally card
  **sharing the same token** (one hash works for both `/garapon/<token>` and
  `/stamp-card/<token>`), with a copy button for each in the drawing-links table.
- **Stamp Rally open/closed status** (separate from the date window): the manager now
  splits into an **Open** card grid and a **Closed** table (like Raffles/Garapon), with a
  Close/Reopen button. A closed rally is read-only and isn't offered for Garapon linking.
  A card with collected stamps can only be deleted once the rally is closed — and its
  **View Logs** entries are preserved (the log now groups by the participant-name snapshot).
- **Inline stall pause from the main list.** Each open rally card shows an at-a-glance
  "X/Y stalls active" summary and a **Manage stalls** panel to Pause/Resume individual
  vendor stalls right there — no need to open the rally or its placement editor.

### [1.3.1] — 2026-06-29

#### Changed

- Reordered the admin sidebar: Bingo ends with **Winners Log**; Senpan Tea House
  leads with **Affiliates** (above Announcements); Festival lists **Stamp Rally**
  below Raffles; Atelier Yao shows **Carrd Upload** before Font Upload; and System is
  ordered **Images → Themes → Users → Settings**.

### [1.3.0] — 2026-06-29

#### Added

- **Stamp Rally** (Festival → **Stamp Rally**): an admin tool to author stamp-rally
  events — a card background + "not stamped" placeholder, an availability window,
  markdown details + "How to Redeem" instructions, and **stamps** and **prizes**
  placed on the card with a visual **drag / resize / rotate** editor. The card image
  is the full designed card (frame, slot placeholders, labels, prize panel); earned
  stamp/prize art is overlaid on its slots, turning the "empty" card into the "full"
  one. Each stamp links
  to an affiliate stall (or Senpan Tea House), has a password and an optional active
  window with pause/resume. Admins issue each participant a tokenized
  `/stamp-card/<token>` link; participants enter stall passwords to collect stamps,
  and once every still-collectable stamp is accounted for the card completes and its
  prizes + redeem instructions reveal. A per-event **View Logs** page shows every
  collection (participant · stall · time), sortable with each participant's rows
  grouped, and updates **live** over the WebSocket as stamps come in. Three new
  permanent image categories: **Stamp Cards**, **Stamp Stamps**, **Stamp Prizes**.
  Gated by the new `festival-stamp-rally` page permission.

### [1.2.0] — 2026-06-29

#### Added

- **Affiliates management** (Senpan Tea House → **Affiliates**): an admin page to
  manage partner establishments — name, one or more owners, location, opening
  hours (multiple time ranges under a single timezone), markdown details, and a
  logo + establishment screenshot picked from two new permanent image categories
  (**Affiliate Logos**, **Affiliate Images**). Gated by the new
  `teahouse-affiliates` page permission; live-refreshes across admin sessions.

### [1.1.1] — 2026-06-28

#### Added

- Cloudflare Turnstile bot check on the hidden **registration** form too (same
  widget + gating as the login form).

### [1.1.0] — 2026-06-28

#### Added

- **Cloudflare Turnstile bot check on the admin login.** When the backend reports
  a Turnstile site key (`GET /api/config`), the login form renders the challenge
  widget and requires it before logging in (the token is sent with the login
  request and verified server-side). When no key is configured, the form behaves
  as before. New `TurnstileWidget` component + `endpoints.system.config()`.

### [1.0.0] — 2026-06-28

First tracked release — establishes versioning for the current production build.

#### Added

- **App version readout** in the admin sidebar footer (frontend + backend
  versions, with a flag when their major versions differ).
- **WCAG 2.1 contrast report** in the theme editor: a live audit of 28 real
  text/background pairings (body, muted, links, headings, inputs, neutral/primary/
  secondary/success/danger/caution buttons + their hover states, the B-I-N-G-O
  header, every board state, called number, winner chip). Each finding shows a
  live contrast chip, the token trail, separate AA/AAA verdicts, and a
  **Find-in-preview** button; the verdict updates live as colours change.
- **Token-based theme editor** with a collapsible live preview, per-token help
  text, native colour pickers for solid tokens, and an alpha-capable picker for
  translucent tokens (modal overlay, shadow, glow).

#### Changed

- `app.css` split from a monolith into logical files under
  `src/assets/styles/` (`tokens`, `base`, `utilities`, `components`, `player`,
  `admin`, `responsive`).
- All built-in themes retuned for contrast (WCAG 2.1 AAA, with two documented
  light-theme neutral-button pairs at strong AA); added the **Toji** theme.

#### Removed

- CodeMirror free-form CSS theme editor (themes are now structured tokens).

---

## Backend

### [3.3.0] — 2026-07-05

#### Added

- **Structured logging + admin log viewer.** `slog` now emits JSON (was the
  default text handler) to stdout **and**, when `-log-file` is set (default
  `/var/log/senpan/senpan.log`), to a rotating file via timberjack — daily
  rotation at local midnight, zstd-compressed backups, bounded retention
  (`MaxBackups=14`, `MaxAge=30d`, plus a 100 MB safety cap). `GET /api/logs`
  (admin) tails the file newest-first with `level`/`q`/`limit` filters (4 MB read
  cap, `truncated` flag) and reports the current runtime `level`; each line is
  also forwarded to admin WebSocket clients as a `{"type":"log","entry":LogEntry}`
  message for a live tail — tapped at the slog writer (so it needs no file
  polling and works even with file logging off), admin-gated, and **lossy** so a
  burst can't disconnect anyone. The on-box `jlv` tool reads the same file. Pass
  `-log-file=""` in dev for stdout-only. Requires `LogsDirectory=senpan` on the
  systemd unit (see `deploy/README.md`) so the log dir is writable under
  `ProtectSystem=strict`.
- **Live DEBUG toggle (no restart).** `POST /api/logs/level` (admin) flips the
  process-wide minimum level at runtime via a `slog.LevelVar` — effective
  immediately across stdout, the file, and the live tail, reverting to INFO on
  restart. When DEBUG is on, every request emits a richer `request detail`
  companion line (query, user-agent, referer, ip), and DEBUG logs run across the
  major services (AniList/Discord calls, WebSocket connect/disconnect, login, the
  announcement scheduler).

#### Changed

- **Book club covers post as the Discord embed image** — the large, full-width
  image instead of the small top-right thumbnail — when publishing a reading list.

#### Fixed

- **Request logs record the real client IP.** The `ip` field used `RemoteAddr`
  (always the Apache proxy, e.g. `[::1]:44686`); it now prefers `CF-Connecting-IP`
  (Cloudflare's authoritative client address), then the leftmost
  `X-Forwarded-For`, then the RemoteAddr host. Display-only — rate limiting still
  uses the spoof-resistant `clientIP`.
- **Book-club AniList lookup surfaces the real error.** Lookups now return
  **`424 Failed Dependency`** carrying AniList's actual message (e.g. "API
  temporarily disabled… (status 403)", or a transport reason like "did not
  respond within 15s") instead of a `502`. Cloudflare replaces origin **5xx**
  bodies with its own error page, so a 5xx hid the message from the SPA; a 4xx
  body passes through. Outbound AniList requests also send a descriptive
  `User-Agent` (`SenpanAppSuite (+https://apps.senpan.cafe)`).

### [3.1.0] — 2026-07-03

#### Changed

- **No more permanent image categories.** The ten formerly hardcoded categories
  (announcements, raffle, garapon, flourishes, affiliates ×2, stamp rally ×3)
  are folded into the `.categories.json` manifest by a one-time startup
  migration (manifest schema v2) and become ordinary categories — renamable and
  deletable like any other. Fresh installs seed the same set as defaults. The
  `permanent` field is gone from `ImageCategory`, and rename/delete no longer
  return `403 Permanent category`.
- **Image read access widened for the shared picker.** `GET
  /api/image-categories` and `GET /api/images` now allow any user holding an
  image-using page permission (announcements, raffles, garapon, affiliates,
  stamp rally, themes, or system-images) instead of gating each directory to
  the single editor permission that owned it. Management endpoints
  (upload/delete/category CRUD) still require `system-images`.

### [3.0.0] — 2026-07-02

#### Changed

- **BREAKING: protected font serving.** Uploaded fonts are licensed assets and
  are no longer served as static files. The server streams them itself:
  `GET /api/fonts/pub/kit.css` (generated `@font-face` stylesheet for external
  sites) and `GET /api/fonts/pub/f/{token}` (font bytes behind an opaque,
  rotating HMAC token, valid 7–14 days; key auto-generated into the
  `font_url_secret` settings key). Font requests are **origin-gated per
  font**: same-origin (the SPA) is always allowed; cross-origin needs an
  origin on THAT font's allowlist (echoed via `Access-Control-Allow-Origin`,
  which browsers require for cross-origin fonts); requests with no usable
  Origin (address bar, plain fetch tools) are refused. `kit.css` filters its
  `@font-face` rules by the requesting site's Referer, so each site only sees
  the fonts that allow it. Both endpoints are served with **private** caching:
  Cloudflare fronts the site, caches `*.woff2` by default, and ignores `Vary`,
  so a shared-cache copy would silently bypass the origin gate (browsers still
  cache per-visitor). `GET /api/settings` `uploaded_fonts` changed from
  `["file.ttf"]` to `[{name, family, token}]` (hence the MAJOR bump).
  **Deployment:** the `fonts.senpan.cafe` vhost must become a reverse proxy to
  `/api/fonts/pub/` (see `deploy/README.md`); legacy direct font URLs stop
  working.
- **BREAKING: fonts are grouped into families.** Files sharing a base name
  (e.g. `Jasper.otf` + `Jasper.woff2`) are format VARIANTS of one font.
  `GET /api/fonts` returns the grouped shape (base, family, served type/token,
  per-font origins, variants); `PATCH /api/fonts/{name}` renames one file;
  `PATCH /api/fonts/families/{base}` edits a font's metadata (`family` — the
  CSS name, `serve` — the served variant type, `origins` — its allowlist,
  stored under the `font_meta` settings key) and
  `DELETE /api/fonts/families/{base}` removes a whole font.

#### Added

- **Automatic WOFF2 conversion** (via `github.com/tdewolff/font`, pure Go). A
  font with no uploaded WOFF2 gets one converted from its best source into
  `<webRoot>/fonts/.woff2/` — the served variant by default (selectable per
  font); uploading a real WOFF2 suppresses/removes the converted copy. A
  startup backfill converts pre-existing fonts and sweeps stale copies.
  Conversion failure keeps the upload and serves an uploaded format instead
  (reported via the new `warnings` field on the upload response).

### [2.2.0] — 2026-07-02

#### Added

- **Passkey (WebAuthn) support.** Register / list / delete passkeys for an account
  (`/api/account/passkeys…`) and a usernameless, discoverable-credential login
  (`POST /api/auth/passkey/{begin,finish}`) that establishes the session like a
  password login. Credentials (the full go-webauthn `Credential`, as JSON) live in
  the new `user_passkeys` table (schema **v44**); the relying-party id/origin
  derive from the request host, and the one-time challenge is held in the session
  between the begin/finish halves. Uses `github.com/go-webauthn/webauthn`.
- **Turnstile on public raffle entry.** When Cloudflare Turnstile is configured,
  `POST /api/raffles/{id}/enter` verifies a token before recording the entry, so
  bot-flooded entries can't skew the weighted winner pick (on top of the per-IP
  rate limiter).

### [2.1.0] — 2026-07-02

#### Security

- Sanitize uploaded SVGs server-side before persisting — drop `<script>`/
  `<foreignObject>`, event-handler attributes, `javascript:`/external refs, and
  dangerous inline styles — closing a stored-XSS vector (SVGs are served from our
  origin and inlined on the player board).
- Restrict the `anilist_api_url` setting to hosts under `anilist.co` (SSRF
  defense), mirroring the existing Discord-webhook allowlist.
- Keep the Google Fonts API key out of the public `GET /api/settings` response
  (admin-only, like the webhook secrets).
- Rate-limit the public raffle-entry endpoint (per IP) and gate raffle detail so
  non-admins can no longer read a not-yet-open/closed-window raffle by guessing
  its id.
- Add a CSRF Origin check (defense-in-depth over the SameSite=Lax cookie): a
  cross-origin state-changing request that rides an ambient session cookie is
  rejected. Bearer-token (plugin) and cookie-less requests are exempt.
- Re-authorize the admin WebSocket periodically instead of only at connect, so an
  account deactivated or deleted mid-session has its admin (undelayed-draw) socket
  dropped rather than kept open. Server-side only; the plugin reconnects and is
  rejected at connect while deactivated.
- Add an **enforcing** full resource Content-Security-Policy (`deploy/.htaccess`).
  Verified via a Report-Only pass; the only external scripts (Cloudflare Turnstile
  + Web Analytics) are allow-listed and nothing needs `'unsafe-eval'` after the
  drag-library swap, so `script-src` stays strict.

#### Fixed

- Fix a send-on-closed-channel panic in the WebSocket hub's disconnect helpers
  (the message send now happens under the read lock).
- Fix an HTTP 500 on a participant's stamp-card page after a stamp is removed —
  the nullable `stamp_id` is now scanned via `sql.NullInt64`.
- Wait for the announcement scheduler's in-flight sweep on shutdown before
  closing the DB, so a post that already succeeded advances its cursor and can't
  re-post on the next boot.
- Clean up multipart temp files after each upload (`RemoveAll`), so a batch that
  spills over the in-memory budget no longer leaves files behind in the temp dir.
- Announcement `buttons` now serialize as an empty array (never `null`) when a
  post has none, matching the generated TS type — fixes a client-side crash when
  editing a button-less announcement.

#### Added

- `UNIQUE(game_id, number)` index on `called_numbers` (schema **v43**) as a
  database-level backstop against a number being drawn twice in one game.

#### Changed

- Starting a game is now a single transaction (end active + create + snapshot all
  patterns), so a partial failure can't leave a half-initialized active game.
- Made the retired book-club events-webhook migration transactional and
  idempotent (no duplicate announcement types if it re-runs after a partial
  failure).
- Consolidated the upload-name and slug validators into shared helpers (fixing a
  missing hidden-file check on font uploads) and the JSON-array / raffle-row scan
  helpers in the store layer.

### [2.0.0] — 2026-06-30

#### Changed

- **HTTP API migrated from action dispatchers to hybrid REST (breaking).** Every
  admin/resource endpoint that previously multiplexed on a `{"action":"…"}` body
  now uses HTTP methods and resource paths: `POST` create (`201`), `PUT` replace,
  `PATCH` partial-update, `DELETE` remove (`204`), plus `POST /api/<resource>/{id}/<verb>`
  for non-CRUD commands (`close`/`reopen`, `activate`/`deactivate`, `send`/`skip`,
  `pick-winner`, `/api/game/{start,draw,end,halftime}`, …). Bulk writes are `POST`
  (`…/reorder`, `/api/cards/generate`); bulk deletes are `DELETE /api/<resource>/all`
  → `{ "deleted": N }`. A single reorder or flag toggle is a declarative `PATCH`.
  All per-handler auth guards, validation, error codes, and WebSocket broadcasts
  are unchanged; the protected `admin` account and permission-key validation on
  `PATCH /api/users/{id}` are preserved (and now covered by dedicated tests). The
  regenerated `openapi.yaml` documents every new route (CI enforces coverage +
  freshness). `POST /api/auth` (login/logout) and `POST /api/register` intentionally
  remain small action/credential bodies. Requires the **2.0.0 SPA**; the Dalamud
  plugin was migrated in lockstep.
- **Book clubs are now a first-class path entity.** Reading lists moved from the
  flat, query-scoped `/api/reading-lists?club=…` to nested resources under the
  club slug: `/api/book-clubs/{club}/reading-lists[/{id}[/items/{itemId}|/publish]]`.
  The club slug leaves request bodies (it's in the path), and every load-by-id
  handler now verifies the record belongs to the `{club}` in the path — a caller
  holding one club's permission can no longer reach another club's list by id
  (404 on mismatch). The shared utilities `POST /api/bookclub/upload` and
  `GET /api/bookclub/lookup` are unchanged.

### [1.7.0] — 2026-06-30

#### Added

- **OpenAPI 3 spec + hosted API reference.** Every endpoint is described in
  [`backend/openapi.yaml`](backend/openapi.yaml), **generated** from the Go code:
  component schemas are reflected from the `model` structs (via
  `internal/apidoc` + `cmd/openapi-gen`) so they can't drift, and a hand-maintained
  paths table adds auth, params, action-dispatcher requests, and the WebSocket
  channel (in the description). The server serves it at **`GET /api/openapi.yaml`**
  and renders it with **Scalar** at **`GET /api/docs`** (both public — the API
  contract carries no secrets). The spec is embedded so the binary is
  self-contained.
- **CI accuracy guards** (`internal/apidoc/openapi_test.go`): one test regenerates
  the spec and fails if the committed `openapi.yaml` is stale (the OpenAPI analog
  of `gen:types`); another parses `routes()` and fails if any registered route is
  undocumented, or the spec documents a route that doesn't exist.

#### Changed

- **Typed responses everywhere.** Every handler now returns a named struct from
  the `model` package instead of ad-hoc `map[string]any` (the new
  `model/responses*.go`). The wire shapes are **unchanged** (asserted by the
  existing tests); the structs are the single source of truth for the JSON wire
  format, the tygo-generated frontend types, and the OpenAPI schemas.

### [1.6.0] — 2026-06-30

#### Changed

- **Uploads keep their original filename everywhere.** The book-club cover upload
  (`POST /api/bookclub/upload`) was the last endpoint that rewrote uploaded names
  (`cover_<nanos>.ext`); it now preserves the uploaded filename (sanitized) and
  overwrites a same-named file, matching the central image-hosting and Carrd
  uploads. Cover cleanup on item/list delete is now **reference-safe** — a shared
  file is removed only once no reading-list item still points at it
  (`CountReadingListItemsByCover`).

#### Fixed

- **Race-free writes.** Multi-statement store mutations now run as write
  transactions (`BEGIN IMMEDIATE`, via a `beginImmediate` helper) instead of the
  default deferred mode, so concurrent read-modify-write paths serialize on the
  busy-timeout rather than colliding on a stale snapshot. This makes the public
  **garapon draw** (remaining-draw cap) and similar count checks correct under
  concurrency instead of risking a spurious error.
- **Atomic raffle sign-ups.** `POST /api/raffles/{id}/enter` (and the admin
  `add_entry`) now record entries through a single cap-enforced transaction
  (`AddOrCreateRaffleEntry`), so two simultaneous sign-ups for the same
  character+world can't both pass a stale count check and exceed `max_entries` or
  create duplicate rows.

#### Documentation

- Added [`API.md`](API.md) — a full HTTP & WebSocket API reference (auth, request/
  response shapes, action types, broadcast messages) — linked from the README.
- Corrected stale comments (the `.golangci.yml` run directory, the `cards` and
  `game` action lists) and the `uploads.go` / AGENTS.md upload-helper docs.

### [1.5.0] — 2026-06-29

#### Added

- **Personal access token (bearer) auth** for external API clients (e.g. a FFXIV Dalamud
  plugin), letting them use the existing REST + WebSocket API without the browser's
  cookie-session / Turnstile login. A request carries `Authorization: Bearer <token>` (or
  `?token=` on the `/api/ws` upgrade); it resolves to the owning account and the **same
  per-page permission guards** apply, so a token never grants more than the account holds.
  Resolution is wired into `loadCurrentUser` + `wsSessionUser`, so every existing endpoint
  accepts a token with no per-handler change.
- `GET` / `POST /api/account/token` — self-service token metadata + generate (replace) /
  revoke. One token per account; only a SHA-256 hash is stored and the plaintext is
  returned **exactly once**, at generation. Schema migration **v42** adds the `user_tokens`
  table (cascade-deleted with its user; resolves active accounts only).

### [1.4.0] — 2026-06-29

#### Added

- **Garapon ↔ Stamp Rally linking.** Garapons carry an optional `stamp_rally_id` (to an
  **open** rally; closed/unknown rejected). Creating a drawing link on a linked garapon
  auto-issues a Stamp Rally card via `IssueRallyCardWithToken` using the **same token** as
  the drawing link, recorded on `garapon_players.stamp_card_id` (and surfaced as
  `stamp_card_token`). Deleting the link removes the paired card; deleting the rally clears
  any garapon link.
- **Stamp Rally `status` (open/closed)** with a `set_status` action — closed rallies are
  read-only (`rallyOpen`/`stampAvailable` reject them). Cards with collected stamps are
  deletable only when the rally is closed.
- `GET /api/stamp-rallies` now returns per-rally `stamp_count` + `active_stamp_count`
  (stamps not paused) for the list's at-a-glance stall summary + inline pause panel.

#### Changed

- **Stamp logs are preserved on card/stamp deletion** (schema migration v40 + v41):
  `stamp_rally_collected` now snapshots `participant_name`/`stall_name` and carries a
  `rally_id` (CASCADE) with nullable `card_id`/`stamp_id` (`ON DELETE SET NULL`) — mirroring
  `garapon_draws`. Deleting a rally still removes its whole log.

### [1.3.0] — 2026-06-29

#### Added

- **Stamp Rally API** — admin CRUD at `GET/POST /api/stamp-rallies` (events with
  stamps + prizes carrying %-based placements, saved inline; stamps upserted by id so
  collection history survives edits), per-stamp pause/resume and tokenized participant
  cards (`/{id}/stamps`, `/{id}/cards`), and the event-wide collection log
  (`/{id}/logs`). Public, tokenized participant flow at `GET /api/stamp-card/{token}`
  (passwords stripped; prizes hidden until the card completes) and
  `POST /api/stamp-card/{token}/stamp` (collect by password; "this stall is currently
  closed" when unavailable; lazy completion when every stamp is collected-or-expired).
  The public collect broadcasts the `stamp-rallies` resource-changed signal for live
  admin log updates. Schema migration v39 (five `stamp_rally*` tables) and three new
  permanent image categories (`stamp_cards`, `stamp_stamps`, `stamp_prizes`). Gated by
  the new `festival-stamp-rally` page permission.

### [1.2.0] — 2026-06-29

#### Added

- **Affiliates API** — `GET/POST /api/affiliates` (admin CRUD of partner
  establishments), gated by the new `teahouse-affiliates` page permission. Owners
  and opening hours persist as JSON columns on a new `affiliates` table (schema
  migration v38). Adds two permanent image categories — **Affiliate Logos**
  (`affiliate_logos`) and **Affiliate Images** (`affiliate_images`) — readable by
  affiliate editors for the logo/screenshot pickers.

### [1.1.1] — 2026-06-28

#### Added

- Cloudflare Turnstile verification on `POST /api/register` as well (mass-signup
  protection), gated the same way as login — enforced only when a secret is set.

### [1.1.0] — 2026-06-28

#### Added

- **Cloudflare Turnstile verification on login.** When `APPSUITE_TURNSTILE_SECRET`
  (flag `-turnstile-secret`) is set, `POST /api/auth` requires a valid Turnstile
  token — verified against Cloudflare's siteverify API (fail-closed) — before any
  credential work, blocking automated brute-force at the door. Disabled (skipped)
  when no secret is configured, so dev/test keep working.
- **`GET /api/config`** — public endpoint exposing the non-secret Turnstile site
  key (flag `-turnstile-sitekey` / `APPSUITE_TURNSTILE_SITEKEY`) for the login page.

### [1.0.1] — 2026-06-28

#### Fixed

- **Scheduled announcements could post more than once.** Two independent causes:
  1. An announcement overdue by more than one period (e.g. the server was down
     across a slot) advanced its schedule cursor from the stale slot, landing on
     _another_ past slot — so it stayed "due" and re-posted on every scheduler
     tick until it caught up. The cursor now advances to the next occurrence
     strictly in the **future** (missed slots are skipped, not replayed).
  2. A webhook call that failed at the **transport** layer (timeout / connection
     reset) was retried on the next tick even though Discord may already have
     received it. Transport failures are now treated as _possibly delivered_ and
     the cursor is advanced (no retry); genuine HTTP error responses (e.g. a 429
     rate limit or a 5xx) are still retried, so delivery isn't dropped.

### [1.0.0] — 2026-06-28

First tracked release — establishes versioning for the current production build.

#### Added

- **`GET /api/version`** — public endpoint returning the backend's semantic
  version (powers the admin compatibility readout; doubles as a version probe).
- **Token-based theming**: themes stored as a structured token map; the applied
  stylesheet is generated server-side (`:root { … }`) and sanitized against a
  token allowlist (migration `user_version` 37 backfills + drops `css_content`).
- **Live admin invalidation**: a thin `resource_changed` WebSocket signal after
  admin-mutation POSTs prompts a scoped REST refetch (Garapon draws and raffle
  entries included).
- **Garapon** festival lottery-drum feature (admin CRUD + tokenized public draw).

---

## How to update this file

1. Make your change and bump the relevant version source
   (`frontend/package.json` and/or `backend/internal/version/version.go`).
2. Add an entry under the matching section above, newest first, grouped as
   _Added / Changed / Fixed / Removed_.
3. Keep the version string in the source file and the heading here in sync.
