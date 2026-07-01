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
