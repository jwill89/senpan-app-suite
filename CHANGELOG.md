# Changelog

All notable changes to the **Senpan App Suite** are recorded here.

The **frontend** (Vue SPA), **backend** (Go API), and **plugin** (SenpanCompanion,
a Dalamud/FFXIV plugin) are versioned independently with
[Semantic Versioning](https://semver.org/) and tracked in their own sections below
— a change usually touches only one, and they deploy separately. The admin
dashboard shows the live frontend + backend versions (sidebar footer) so operators
can confirm the two web halves are compatible; the plugin ships through its own
Dalamud custom repo and talks to the backend over the same API (PAT-authenticated).

**Sources of truth**

- Frontend version → `frontend/package.json` (`"version"`), baked into the build
  and read via `frontend/src/lib/version.ts`.
- Backend version → `backend/internal/version/version.go` (`Version`), served at
  `GET /api/version`.
- Plugin version → `plugins/SenpanCompanion/SenpanCompanion.csproj` (`<Version>`,
  a four-part Dalamud AssemblyVersion) + `plugins/pluginmaster.json` (the repo
  listing consumed by Dalamud).

**Compatibility rule:** the SPA and API are compatible while their **MAJOR**
versions match. Bump MAJOR only for a breaking change to the JSON/WebSocket API
the SPA depends on; MINOR for backward-compatible additions; PATCH for fixes.
When you change one side, bump its version and add an entry under its section.

**Releases & tags.** Each version corresponds to a Git tag and GitHub Release
named `<Component>-v<version>` — e.g. `Frontend-v3.5.0`, `Backend-v3.4.0`,
`Plugin-v2.0.1.0`. These are created **automatically** by the CI `release` job
(`.github/workflows/ci.yml` → `.github/scripts/release.sh`): on a push to `main`,
after the full gate passes, any component whose current version has no release
yet is tagged and released, with the release body taken verbatim from that
component's section below. So bumping a version here and pushing a green commit
is all it takes — no manual tagging. To publish a release, make sure the version
source and its section here are updated in the same commit.

The format follows [Keep a Changelog](https://keepachangelog.com/).

---

## Frontend

### [3.14.0] — 2026-07-20

Adds **auto-run bingo games** — the system draws numbers on a timer (paired with
backend 3.11.0).

#### Added

- **Auto-draw controls on the Game tab.** The New Game form has an **Auto-draw
  numbers** toggle under Game Details; switching it on reveals a **Time Between
  Calls** selector (10s–5m). Start the game and the server draws a number every
  interval — no clicking. During a live game the same controls appear as an
  **Auto-Draw On/Off** button and interval selector, so auto can be turned off or
  re-paced at any time (this never changes the preset the game came from).
- **Auto-draw fields on Game Presets.** The preset editor gains the same Auto +
  Time Between Calls fields; applying a preset on the Game tab pre-fills them.

#### Changed

- **The half-time prompt is now server-driven.** The backend detects the midpoint
  on any draw (manual or automatic) and prompts every admin surface at once, so the
  prompt is consistent across admins and works for auto draws. Auto-draw is paused
  automatically at half-time and the instant a winner is recognized; choosing **No
  mini-game** resumes it, choosing **Yes** leaves it off until you switch it back on.
  Turning auto on draws the first number immediately, then spaces draws by the
  interval. Each draw still respects the player draw delay — players lag the admin
  by the delay — but the delay never stretches the admin's cadence (admins see
  numbers exactly one interval apart, immediately as they're drawn).
- **Tidier live-game controls.** The Current Game view now leads with three
  equal-sized primary controls — **Draw Number**, the **Delay** selector, and **End
  Game** — while the per-feature toggles (Auto-Draw, It's Yoever, Winner Sound) move
  into a collapsible **Game Settings** panel with a labelled section each, so the
  main controls stay front-and-centre.
- **The "Live" indicator is now green.** The Current Game **Live** badge and the
  sidebar's game "live" dot use the success colour (matching the player site's
  connected/"Live" indicator) so an active game reads as a positive, connected
  state instead of an alarming red.

#### Fixed

- **Missing admin icons.** Registered three Font Awesome **solid** glyphs that were
  used but never added to the icon library, so they rendered blank and logged
  "Could not find one or more icon(s)" console errors: `check` (the Images / Carrd
  upload **Save** buttons), `circle-info` (the plugin **changelog** note), and
  `triangle-exclamation` (the version-mismatch flag, the theme editor's contrast
  warning, and the Personal Card Request form alert).

### [3.13.0] — 2026-07-19

Adds **public/private themes with a client-side theme picker**, and a public
**Personal Card Requests** page, plus admin card **statuses** (paired with backend
3.10.0).

#### Added

- **Public theme picker.** Themes can now be marked **Public** in the admin Themes
  editor (with a "Public" badge in the theme list). Public themes appear in a
  **theme picker in the site footer** so each player can choose their own look; the
  choice is remembered per browser. A **"Default"** option follows whatever theme the
  admin has activated — it always reads "Default", never the admin theme's real name.
  A player on a specific public theme is no longer overridden when the admin changes
  the active theme (players on "Default" still follow it live).
- **Personal Card Requests page** (`/card-requests`, linked from the home page). A
  player enters their character name + **World** (a data-center-grouped dropdown of
  FF14 worlds), builds a 5×5 bingo card by hand (per-column ranges enforced, invalid
  cells highlighted) or with **Generate Random** (repeatable, still editable), and
  picks a custom 6-character card ID. The page shows the gil cost and the terms, and
  submission is blocked until the card is valid; a taken ID or a duplicate card is
  rejected with a clear message.
- **Card status icons + actions** on the admin Manage Cards table: a hollow star for
  a **pending** custom card, a filled star for an **approved** one, and a lock for a
  **Protected** card, plus **Approve** and **Protect/Unprotect** row actions. "Delete
  All" now keeps Protected cards and says so.
- **Custom Card Cost** setting (System → Settings → Gameplay) — the gil price shown
  on the Personal Card Requests page.
- **Dimmed unused columns in "Called Numbers"** (player board + admin Game tab).
  When the active game's win patterns don't use a whole BINGO column, no number from
  it is ever drawn (the caller already skips those columns), so that column now gets
  a subtle dark overlay in the Called Numbers tracker to show it won't be used this
  game — e.g. a postage-stamp game dims the N column.

### [3.12.0] — 2026-07-16

Shows the **live** plugin version in the admin footer.

#### Changed

- The **Plugin** version in the sidebar footer is now fetched at runtime from the
  deployed Dalamud repo index (`/plugin/pluginmaster.json`) rather than baked into
  the bundle from CHANGELOG.md at build time. So publishing a new plugin (deploy
  `-Target plugin`) refreshes the shown version without a frontend rebuild, and the
  number always matches what Dalamud actually serves. It falls back to the bundled
  changelog version when the index isn't reachable (e.g. local dev). The plugin's
  changelog **modal** content is still bundled at build time.

### [3.11.0] — 2026-07-16

Adds a **Stamp Rally link on the public Garapon page** (paired with backend 3.9.0).

#### Added

- **"View your Stamp Rally card" link** on the Garapon drawing page
  (`/garapon/:token`). When a drawing link was issued for a garapon tied to a Stamp
  Rally — so the player also holds a stamp card sharing the same token — the page
  now shows a link straight to their `/stamp-card/:token`. It's gated on the new
  `stamp_card_token` the public endpoint returns, so it appears only when a linked
  card actually exists.

### [3.10.0] — 2026-07-12

Adds a **"Where to Redeem"** image to stamp rallies (paired with backend 3.8.0).

#### Added

- **"Where to Redeem"** field on the stamp-rally editor — an image picker below
  **How to Redeem**, usually a screenshot of where in the area a participant goes
  to redeem their completed card. It's shown on the participant's card, alongside
  the redeem instructions, once the card is complete.

### [3.9.0] — 2026-07-12

Fixes large image uploads timing out, and adds a live upload-progress indicator.

#### Fixed

- **Image uploads no longer time out.** Uploads went through the API client's
  30-second request timeout, so a large image (or several at once) over a slow
  connection was aborted mid-transfer with _"Request timed out. Please try
  again."_ Uploads now run over `XMLHttpRequest` with no client-side deadline —
  the transfer takes as long as it needs (the server still caps a request at
  64 MB). The same fix covers the **font**, **Carrd**, and **book-club** uploads,
  which shared the timeout.

#### Added

- **Live upload progress** on the Images tab — a real percentage bar while the
  bytes are in flight, then a "Processing…" state while the server saves the
  files, replacing the indeterminate spinner that looked stuck on long uploads.

### [3.8.0] — 2026-07-12

Reworks the **Affiliates** admin page (Senpan Tea House → Affiliates) to match Tea
Rooms — a drag-sortable list that posts rich embeds to Discord (paired with backend
3.7.0).

#### Added

- **Shared Discord webhook** for Affiliates — a Webhook sub-screen (with a "no
  webhook set yet" hint on the list), and a per-row **Post** button that posts the
  affiliate to that channel as an embed.
- **Drag-and-drop reordering** of the affiliate list (the order persists), replacing
  the old alphabetical card grid with a Tea Rooms-style list.
- Each row shows the **logo — or the establishment screenshot when there's no logo**
  — plus an embed-colour swatch and Discord / Carrd / open-times badges.
- Form fields for an **embed accent colour** (native colour picker), a **Discord
  Link**, and a **Carrd Link**.

### [3.7.0] — 2026-07-11

Adds the **"It's Yoever"** bingo reaction, and makes the admin sidebar version
numbers open per-component changelogs (paired with backend 3.6.0).

#### Added

- **"It's Yoever" button** (player board, next to Save Board). While a game is
  running and the reaction is switched on, a player can trigger it to broadcast a
  sound (`this_is_bad.mp3`) and a reduced-size, reduced-opacity **big-yoey-head**
  image that bounces across every connected client's screen and fades over a few
  seconds, captioned with the triggering player's name (`"It's Yoever." ~Name`). A
  global `YoeverOverlay` (mounted in the app shell) renders it on both the player
  and admin views; it respects `prefers-reduced-motion`. When the host switches the
  reaction off the button stays visible but disabled, so it never looks missing.
- **Per-player rate limit.** After triggering, the button disables and shows a
  live countdown until the cooldown (default 3 minutes, admin-configurable) clears.
  The expiry is mirrored from the server (`retry_after`) and persisted per
  card + game, so it survives a refresh; the server's `429` is the backstop.
- **Per-client toggles** (above the board actions, and mirrored on the admin Game
  tab), on by default and affecting only your own screen. **"Show It's Yoever
  effects"** is the master switch for the reaction; **"Play It's Yoever sound"** is
  a sub-toggle that only applies while effects are shown — turning the master off
  also mutes the sound (and disables that sub-toggle), and turning it back on
  re-enables the sound. While effects are shown, the sound toggle is independent of
  the main Sound options (off/basic/game) but still plays at your master sound
  volume.
- **Admin controls** (Game tab, live game): an **on/off switch** for the reaction
  (broadcast to every client so players' buttons show/hide live) and a
  **"Yoevers: N"** running counter. Both reset when a new game starts.
- **Settings.** New **"It's Yoever" Cooldown (seconds)** field (Gameplay section,
  0–3600; 0 disables the limit).
- **Version changelogs in the admin sidebar.** The **Frontend / Backend / Plugin**
  version numbers in the sidebar footer are now clickable — each opens that
  component's changelog in a modal, parsed from `CHANGELOG.md` at build time (via
  a `virtual:changelog` Vite plugin). The **Plugin** version is new, and its
  changelog is prefixed with **Dalamud install steps** (the custom-repo URL and the
  `/senpan` token setup).

#### Fixed

- **Saving a numeric setting no longer fails with "Invalid JSON."** A number
  `<input>` bound with `v-model` yields a JS *number* for any edited field, which
  the string→string settings API rejected on decode; every value is now coerced to
  a string before the save. Affected all numeric settings, not just the new
  cooldown.

### [3.6.0] — 2026-07-10

Adds the **Tea Rooms** admin page under Senpan Tea House (paired with backend
3.5.0).

#### Added

- **Tea Rooms** (Senpan Tea House → Tea Rooms, `booth-curtain` icon). A
  drag-orderable list of bookable rooms — name, subtitle (any language), a
  **required + unique** room number, per-half-hour gil cost, hashtags, markdown
  description, seasonal/open/lockable/discounted flags, an image, and an embed
  accent colour. Each row has Post-to-Discord, open/close toggle, discount toggle,
  copy-API-link, edit, and delete actions; the list reorders like Announcements.
  A Webhook sub-page stores the single shared Discord webhook the rooms post to.
  Gated by the new **`teahouse-tea-rooms`** page permission (in the Users-page
  permission editor). The Copy-API-link buttons build the public URL from the
  room's **room number** (the one number the admin manages).
- The Discord embed shows the room name, description, then three inline fields —
  the cost (halved with a "Currently Discounted!" note when discounted), the room
  number, and the Open/Closed status — with the hashtags (capitalized) in the
  footer.

### [3.5.0] — 2026-07-07

A security/correctness bugfix pass plus request-log identity, released together
(paired with backend 3.4.0).

#### Added

- **Server Logs viewer now shows a "User" column.** Each request line names who
  made it — the account username for an admin (cookie session) or the FFXIV
  plugin (personal access token), a verified-bot name for a Cloudflare-verified
  crawler, or "—" for anonymous traffic. A verified bot renders italic; the
  `auth` classifier (`session` / `token` / `bot` / `anon`) is shown on hover and
  in the expanded JSON.

#### Fixed

- **Font-family CSS-injection defense (defense in depth).** `theme.ts` now
  refuses to emit an `@font-face` rule, and falls back to the default
  `--header-font`, for any family carrying CSS-breaking characters (control
  chars, quotes, backslash, or `{ } ; < >`). The server validates these too;
  this closes the client-side `<style>`/`--header-font` sinks for any value that
  predates that validation, so an uploaded font (or `header_font`) can no longer
  inject CSS into every visitor's board.
- **Image library no longer spams error toasts on live updates.** The
  `resource_changed` handler now refreshes the shared image-picker caches
  **silently** and only when something is actually cached: an admin without image
  access no longer gets a 403 toast on every image mutation, and a renamed or
  deleted category self-prunes from the cache instead of re-raising a 400
  "Unknown image category" toast on every subsequent update. Explicit user
  actions still surface errors loudly.

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

### [3.11.0] — 2026-07-20

Adds the **automatic bingo-draw scheduler** and moves half-time detection
server-side (paired with frontend 3.14.0). Backward-compatible additions only.

#### Added

- **Auto-draw scheduler.** A single background goroutine (`RunAutoDrawScheduler`,
  launched from `main.go` on the shutdown-cancelled context) draws the first number
  the instant auto is switched on, then draws one every `auto_interval` seconds
  while a game has auto switched on. The draw reuses the exact manual-draw path
  (admins immediately, players after the delay), so the player draw delay only lags
  when each number reaches players — it never stretches the admin's cadence. Auto
  state (enabled + interval) lives on the game service, is stamped onto
  `BingoGameState` (`auto_enabled`, `auto_interval`), and **defaults off, including
  after a restart**, so draws never resume unattended. A **manual draw switches auto
  off** (the admin is taking over), and whenever auto turns off — for any reason —
  the scheduler cancels its pending draw. The loop's draw is guarded under the same
  lock that serializes every draw (`DrawAuto`), so a disable racing with a scheduled
  fire can never leak a stray number.
- **Game-start + control fields.** `POST /api/game/start` accepts `auto` +
  `auto_interval`; `PATCH /api/game` accepts `auto_enabled` + `auto_interval` (live,
  never written back to a preset). `game_presets` gains `auto_call` + `auto_interval`
  columns (**migration v50**), surfaced on the preset create/replace endpoints.
- **`auto_config` WebSocket broadcast** — the new auto state (enabled + interval) is
  pushed to every admin surface whenever it changes, like `yoever_config`.

#### Changed

- **Half-time detection moved server-side.** Any draw (manual or automatic) that
  crosses the midpoint (`bingo.HalftimeThreshold`, the 35-of-75 mark scaled to the
  callable pool) now broadcasts a **`halftime_prompt`** to admins, so every surface
  prompts consistently and auto games prompt at all. Crossing it **pauses** a running
  auto loop; `POST /api/game/halftime` now takes `{"minigame": bool}` — `true` alerts
  players (held until the triggering number has reached them) and leaves auto paused;
  `false` declines and resumes auto if it was paused. An absent body defaults to
  `true`, preserving the old "trigger the alert" behavior. Auto is also switched off
  the moment a new winner is recognized.

### [3.10.0] — 2026-07-19

Adds **theme visibility**, a **custom-card request** flow, and card **statuses**
(paired with frontend 3.13.0). Backward-compatible additions only; no breaking
contract changes.

#### Added

- **Theme visibility.** The `styles` table gains an `is_public` flag (default 0 =
  Private, so every existing theme stays admin-only). New **public** endpoints
  `GET /api/styles/public` (id + name of public themes) and `GET /api/styles/public/{id}`
  (a public theme's generated CSS) let the client-side theme picker list and apply
  public themes; the by-id endpoint 404s for a private theme so its CSS can't be
  fetched by guessing an id. `POST/PUT /api/styles` accept `is_public`.
- **Personal Card Requests.** New public `POST /api/cards/request` (rate-limited, and
  Cloudflare-Turnstile-gated when configured) validates a hand-built board and a
  chosen 6-character ID, rejects a taken ID or a duplicate board, and stores the card
  as **pending** — not yet playable. Pending cards are blocked from the public board
  join and excluded from winner computation until approved.
- **Card statuses.** The `cards` table gains `protected`, `custom_status`
  (`''`/`pending`/`approved`), and `world`. `POST /api/cards/{id}/approve` approves a
  pending custom card (→ approved **and** protected); `POST /api/cards/{id}/protect`
  toggles a card's Protected flag. **`DELETE /api/cards/all` now spares Protected
  cards** (and only disconnects players whose card was actually deleted).
- **`custom_card_cost`** app setting (gil), returned by the public `GET /api/settings`
  for display on the request page.

### [3.9.0] — 2026-07-16

Surfaces the linked **stamp-card token** on the public garapon payload (paired with
frontend 3.11.0). Backward-compatible addition only; no breaking contract changes.

#### Added

- **`stamp_card_token` on `GET /api/garapon/{token}`** — the public player object
  (`GaraponPublicResponse.player`) now carries the token of the Stamp Rally card
  auto-issued alongside the drawing link (equal to the drawing-link token) when the
  garapon is tied to a rally, and is omitted otherwise. The value was already loaded
  server-side (the `LEFT JOIN` in `GetGaraponPlayerByToken`); it is now forwarded so
  the public Garapon page can link to the participant's stamp card. No schema change.

### [3.8.0] — 2026-07-12

Adds a **"Where to Redeem"** image to stamp rallies (paired with frontend 3.10.0).
Backward-compatible addition only (one new column via schema **v48**); no breaking
contract changes.

#### Added

- **Stamp-rally `redeem_image`** — a card image (the "Where to Redeem" screenshot)
  stored on the rally, accepted on create/replace and returned on the admin and
  public (`GET /api/stamp-card/{token}`) payloads. Schema **v48** (idempotent
  `ALTER TABLE stamp_rallies ADD COLUMN redeem_image`).

### [3.7.1] — 2026-07-12

#### Changed

- **Affiliate embed open times** now render as Discord **Short Time** (`<t:…:t>`,
  e.g. "4:00 PM") instead of Long Time (which included seconds). Still local to
  each viewer's time zone.

### [3.7.0] — 2026-07-12

Adds Discord posting + drag ordering to **Affiliates** (paired with frontend 3.8.0).
Backward-compatible additions only (new columns via schema **v47**, new endpoints,
and a settings-stored webhook); no breaking contract changes.

#### Added

- **Affiliate fields** `embed_color`, `discord_link`, `carrd_link`, and a
  `sort_order` for the drag order (schema **v47** — idempotent `ALTER TABLE` column
  adds + an `idx_affiliates_sort` index). Editing an affiliate preserves its order.
- **Shared webhook** (settings key `affiliate_webhook_url`, deliberately kept out of
  the public settings) returned on `GET /api/affiliates` and set via
  `PUT /api/affiliates/webhook` (validated as a Discord webhook URL).
- **Reorder** — `POST /api/affiliates/reorder` persists a new top-first id order.
- **Post to Discord** — `POST /api/affiliates/{id}/post` builds and sends the embed:
  the embed **colour**, the **name** as the title, the markdown **details** as the
  description, the **logo as the thumbnail** and the **establishment screenshot as
  the image**, and — each shown only when set — the **location** (full-width), the
  **opening hours** (full-width, as Discord "Long Time" tokens `<t:unix:T>` anchored
  in the affiliate's IANA timezone so each viewer sees them in their own, with the
  footer "Times are displayed in your local time zone."), and finally the
  **Discord/Carrd links as two side-by-side fields**.

### [3.6.0] — 2026-07-11

Adds the **"It's Yoever"** bingo reaction. Backward-compatible additions only
(one new endpoint, one folded PATCH field, two new game-state fields, a new
setting, and a new WebSocket message type); no breaking contract changes.

#### Added

- **Trigger endpoint.** `POST /api/game/yoever` (public, body `{card_id}`)
  broadcasts a `yoever` message — `{player_name, count}` — to every connected
  client. Guarded, in order: an active game must exist (409), the reaction must be
  enabled (403), and the card must be off cooldown (429 with `Retry-After` +
  `retry_after`). The player name for the broadcast is resolved from the card.
- **Per-game reaction state** lives in the bingo service (enabled flag, trigger
  count, and a per-card last-trigger map for the cooldown), reset at the start of
  every game. The game-state object (`GET /api/game`, `GET /api/board`, and the
  `game_update` push) now carries `yoever_enabled` and `yoever_count`.
- **Admin on/off switch** folded into `PATCH /api/game` as `yoever_enabled`,
  broadcasting a `yoever_config` message so every client updates live.
- **Setting `yoever_cooldown_seconds`** (default `180`, range 0–3600; 0 disables
  the per-card cooldown).

#### Fixed

- **Saving settings no longer rejects a Google Fonts API key as a Discord
  webhook.** The webhook-URL validation was gated on `secretSettings`, which also
  contains `google_fonts_api_key` — so any admin with that key set got
  "Discord webhook URLs must look like…" and couldn't save *any* setting. The
  check is now scoped to the `discord_webhook_url_*` keys only.
- The reaction now defaults to **enabled** on service construction, so a backend
  redeploy mid-game keeps it available for the running game instead of silently
  switching it off until the next game starts.

#### Security

- **Bumped the Go toolchain to 1.26.5** (`go.mod`) to pick up the `crypto/tls` fix
  for **GO-2026-5856** (Encrypted Client Hello privacy leak). `govulncheck` is now
  clean. CI installs the toolchain from `go.mod` (`go-version-file`), so no
  workflow change was needed.

### [3.5.0] — 2026-07-10

Adds the **Tea Rooms** feature under Senpan Tea House. Backward-compatible
additions only (new endpoints + a schema migration); no breaking API/WebSocket
contract changes.

#### Added

- **Tea Rooms.** A new single-table entity (`tea_rooms`, schema **v45–46**) for
  bookable tea rooms — name, subtitle (UTF-8, e.g. a Japanese phrase), a
  **required + unique** room number, per-half-hour gil cost, hashtags, markdown
  description, seasonal/open/lockable/discounted flags, an image, and a Discord
  embed accent colour. Admin CRUD lives under `GET/POST /api/tea-rooms`,
  `PUT/PATCH/DELETE /api/tea-rooms/{id}` (PATCH toggles the open/discounted
  flags), and `POST /api/tea-rooms/reorder` for the drag order, all gated by the
  new **`teahouse-tea-rooms`** page permission.
- **Post a room to Discord.** `POST /api/tea-rooms/{id}/post` renders the room as
  an embed (name title, description body, then three inline fields — the cost,
  halved with a "Currently Discounted!" note when discounted; the room number; and
  the Open/Closed status — with the hashtags capitalized in the footer) and posts
  it to a single shared webhook stored via `PUT /api/tea-rooms/webhook` (kept out
  of the public settings so it never leaks).
- **Public rooms API (cross-origin).** `GET /api/tea-rooms/public` (all rooms) and
  `GET /api/tea-rooms/public/{number}` (one room by its **room number**, all data +
  status flags) are unauthenticated and send `Access-Control-Allow-Origin: *`, so an
  external Carrd site can read live availability/pricing keyed off the one number
  the admin already knows.

### [3.4.0] — 2026-07-07

A security/correctness bugfix pass plus request-log identity, released together
(paired with frontend 3.5.0). No breaking API/WebSocket contract changes.

#### Added

- **Request logs now identify the actor.** Every access-log line carries an
  `auth` field (`session` | `token` | `bot` | `anon`) and, when applicable, a
  `user` (account username) and `bot` (verified-bot name). Admin actions resolve
  via the cookie session; plugin actions resolve via the personal access token —
  both through the existing `currentUser` path, so no extra store reads for
  authenticated requests and none for anonymous ones. The actor is resolved
  inside the handler chain and carried out to the logging layer via a
  per-request holder (the log runs outside the session middleware).
- **Cloudflare-verified bot detection.** For anonymous requests, the log reads
  Cloudflare's verified-bot signal — either the native `cf-verified-bot`
  (+`cf-verified-bot-category`) headers from the "Add bot protection headers"
  managed transform (Enterprise Bot Management), or a custom `x-verified-bot`
  header set by a Transform Rule on `cf.client.bot` (works on any plan). A
  verified bot is named by its category, or by its User-Agent — trustworthy here
  because Cloudflare has already vouched for the source. This is a logging hint
  only, never a security decision (like `CF-Connecting-IP`, it's forgeable by a
  client that bypasses Cloudflare; lock the origin to Cloudflare IPs for
  assurance). No new endpoints or response-shape changes — the fields ride the
  existing free-form log-entry map, so the log viewer surfaces them
  automatically.

#### Security

- **Live server-log WebSocket tail is now admin-only.** The log stream was
  broadcast to every account on the admin channel (`cardID == ""`), which admits
  any authenticated active account and any plugin PAT — not just admins — even
  though `GET /api/logs` is admin-gated. Log lines carry client IPs, request
  paths (which embed capability tokens), and failed-login usernames. The hub now
  gates the tail on a per-connection `isAdmin` flag, refreshed on each revalidate
  tick so a mid-session demotion stops the tail without dropping the socket.
  `resource_changed` and the draw feed still reach non-admin staff and the plugin
  as before.
- **Capability tokens are redacted from request logs.** Garapon/stamp-card draw
  tokens and font-kit tokens in the URL path (and the PAT `token` query
  parameter, and Referer) are replaced with a short non-reversible hash in both
  the standard and DEBUG log lines — so they no longer land verbatim in the
  rotating log file, the `GET /api/logs` viewer, or the admin WS tail. The hash
  keeps per-link correlation for abuse investigation.
- **Font-family / header-font / theme-flourish injection closed at the source.**
  Font family names (set via `PATCH`, or derived from an uploaded filename), the
  `header_font` setting, and theme `board_flourish`/`number_flourish` paths are
  now validated before storage. Family/header-font values reject CSS-breaking
  characters (control chars, quotes, backslash, `{ } ; < >`); flourishes must be
  an `images/<category>/<file>.svg` path, blocking `data:`/external-URL SVGs that
  bypassed the upload-time sanitizer. The flourish check runs in the store layer,
  so the `themetool` CLI is covered too.
- **`logClientIP` no longer trusts spoofable headers from a direct peer.**
  `CF-Connecting-IP` / `X-Forwarded-For` are honored only when the immediate peer
  is the loopback reverse proxy, so a client reaching the backend directly can't
  poison the audit log's `ip` field. (Rate limiting was already spoof-resistant.)

#### Fixed

- **Image-category manifest robustness.** The manifest read-modify-write
  (create/rename/delete + startup migration) is now serialized by a mutex, and
  the manifest is written atomically (temp file + rename). A present-but-corrupt
  manifest is left untouched and logged (once, at startup) instead of being
  silently reseeded to defaults — which used to wipe custom categories. A
  reserved-directory denylist stops a category from being created/renamed onto a
  folder owned by another feature (book-club covers, the legacy announcements
  dir).
- **`app_title` default** is now `"Senpan App Suite"` (was `"Nifty App Suite"`),
  matching the frontend default so an unconfigured instance no longer flashes the
  wrong product name.

#### Deploy / docs

- **`senpan.service` runs as a dedicated `senpan` user** (was root), with
  `ReadWritePaths` widened to every document-root subtree the app writes
  (`images/`, `fonts/`, `carrd/`). `deploy/.htaccess` denies all dotfiles, hiding
  `images/.categories.json`. `deploy/README.md` documents the non-root setup, the
  font-host cutover verification, credential/session-secret rotation, and the
  Cloudflare Transform Rule that enables verified-bot labelling in the request
  log. `AGENTS.md` / `CONTRIBUTING.md` were brought current for the same.
- **Deploy tooling is now version-controlled.** Both `scripts/check.ps1` and
  `scripts/deploy.ps1` are tracked; **every** environment-specific deploy setting
  (VPS host, SSH user, key path, webroot, service name, opt dir) was moved into an
  untracked `scripts/deploy.config.ps1` (dot-sourced at runtime, or via
  `$env:DEPLOY_*` / `-params`; `deploy.config.example.ps1` is the tracked
  template), so nothing in the tracked script reveals the server layout. The prod
  host IP / key path were removed from all tracked files, and the tracked
  `deploy/senpan.service` is now a placeholder template (no concrete host paths,
  user, or service name). `deploy.ps1` also gained `-Target main` (frontend +
  backend; `both` kept as an alias) and `-Target all` (frontend + backend +
  plugin).

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

## Plugin

**SenpanCompanion** — a Dalamud/FFXIV plugin ("Senpan Admin Companion") that lets
Tea House staff drive app services from in-game. It authenticates to the backend
with a personal access token and is distributed through a Dalamud custom repo
(`plugins/pluginmaster.json`). Versions use the four-part AssemblyVersion in
`SenpanCompanion.csproj`. Entries below the current release were reconstructed
from the `<Version>` history and commit messages.

### [3.2.0.0] — 2026-07-20

Adds **auto-run bingo** controls to the Bingo Game tab (paired with backend 3.11.0).

#### Added

- **Auto-draw controls.** The New Game view gains an **Auto-draw numbers** checkbox
  and a **Time Between Calls** combo; applying a preset pre-fills them. The live game
  view gains an **Auto-Draw** on/off checkbox and interval combo, kept in sync with
  the website and other operators via the new `auto_config` WebSocket message.

#### Changed

- **Half-time prompt is server-driven.** The plugin now opens the mini-game prompt
  when the server broadcasts `halftime_prompt` (fires for manual and automatic draws
  alike), instead of detecting the midpoint locally. Its **Yes/No** buttons send the
  mini-game choice (`Yes` alerts players and leaves auto paused; `No` declines and
  resumes auto if it was paused); the prompt notes when auto has been paused.

### [3.1.0.0] — 2026-07-19

Adds custom-card request handling to the Bingo Cards tab (paired with backend 3.10.0).

#### Added

- **Card status + actions in the Bingo Cards tab.** A new **Status** column shows a
  grey star for a **pending** custom-card request, a gold star for an **approved**
  one, and a lock for a **Protected** card (hover a star for the requester's
  character + world). Each row gains **Approve** (pending only) and
  **Protect/Unprotect** actions. Approving a request makes the card playable and
  automatically Protects it; Protected cards are spared by **Delete all** (still
  individually deletable). Uses the backend's `POST /api/cards/{id}/approve` and
  `/protect` endpoints with the existing `bingo-cards` permission.

### [3.0.1.0] — 2026-07-17

Polish on the 3.0.0.0 UI overhaul.

#### Changed

- **Button colours now match the web admin dashboard's default theme** — a tan primary
  action (dark text), an olive secondary, and a red destructive — replacing the initial
  purple accent, so the plugin and the site read as one product.
- **Bingo win patterns** gain a **Collapse all / Show all** toggle to fold or unfold every
  pattern category at once while building a game.

#### Fixed

- **A Timed Text Macro's message is now actually hidden until you expand it.** Previously
  the preview text stayed visible and only the button label flipped; **Show message** /
  **Hide message** now genuinely shows or hides the full text, and the button's stray glyph
  (which didn't render in the game font, showing as a stray "≡") is gone.

### [3.0.0.0] — 2026-07-17

A major **UI/UX overhaul** of the companion window, plus the ability to **edit saved
macros**. What the tools do is unchanged — this is about how they look and how easy they
are to operate.

#### Added

- **Edit a saved Timed Text Macro in place.** Each macro now has an **Edit** button that
  reopens its name, channel, message, interval, and send cap in an inline editor
  (Save / Cancel) — no more delete-and-recreate. (Stop a running macro first to edit it.)
- **Shared design system across every page:** icon + accent **section headers**, a
  three-tier **button system** (primary / secondary / destructive), compact **icon buttons**
  for table-row actions (copy / delete), pill **badges**, and rounded **cards** — so related
  controls read as a group and a Delete never looks like a Create.

#### Changed

- **Sidebar:** the always-available tools (**Rolls**, **Timed Text Macros**, **Settings**,
  **About**) now live under a collapsible **General** group at the bottom, mirroring the
  Bingo and Festival accordions.
- **Timed Text Macros** renders each macro as its own **card** — an enumerated title with
  channel and status badges, a schedule summary, and a one-line message preview that
  **expands on demand**, so a long (multi-message) macro no longer floods the panel.
- **Rolls** is split into labelled **Captured rolls / Filter / Find the winner / Rolls**
  sections instead of one continuous flow.
- **Settings** groups its fields under **Connection** and **Automatic tells** headers, and
  the **Bingo, Raffle, Garapon, and Stamp Rally** pages pick up the same section headers,
  button tiers, and copy / delete icon buttons.

### [2.3.0.0] — 2026-07-16

Adds two permission-free, account-free tools: a **Rolls** helper for in-game roll games
and **Timed Text Macros** for repeating announcements.

#### Added

- **Rolls page** (no account or permission required — it works even before a token is
  set). It watches chat for `/random` · `/dice` rolls near you and lists them in a
  **paginated table** (15 / 30 / 45 per page, scrolls when needed) showing the player,
  home world, the number rolled (with its ceiling for `/random N`, e.g. _"11 (out of
  20)"_), and the time.
- **Rolls filtering & search:** narrow to a player by name and/or to the **last N
  minutes**, and run a **Highest / Lowest / Closest-to-N** query over that window.
  Matching rolls are brought to the top and highlighted, with a summary banner — and
  when the winning player rolled more than once in the window, a **"multiple rolls
  detected in the time frame"** notice lists all of their rolls.
- **Rolls privacy by design:** rolls are held **in memory only** — nothing is saved or
  sent anywhere. The log clears automatically on **logout** (and is gone entirely once
  the game closes and the plugin unloads); a **Clear** button wipes it on demand. A
  disclaimer on the page states this.
- **Timed Text Macros page** (also no account or permission required). Create any number
  of named announcements, each sent on a chosen public channel (**Say / Yell / Shout**)
  on a **fixed minute interval**, with an optional **cap on the number of sends** (e.g.
  every 15 minutes, 8 times). The **first send fires when you press Send** and it repeats
  until stopped or the cap is reached; each running macro shows a **live countdown to the
  next send** and its **remaining count**.
- **Macro message splitting:** the text can be any length — if it exceeds one in-game
  chat message it is split with the **same logic as the auto-tells** and the page tells
  you up front **how many messages** each send will become; the parts are delivered **one
  second apart**.
- **Macro persistence:** macros are saved and **survive logout and restart**. Progress is
  saved after every send, so a crash leaves the remaining count accurate; a macro always
  reloads **stopped** and must be started again by hand (logout also stops every macro).

#### Changed

- **Split auto-`/tell` messages are now sent one second apart** via the same shared pacing
  as the new macros (previously they were already spaced a second apart; this unifies the
  timing so both paths stay in step).

#### Fixed

- **Long text now wraps to the window** instead of running off the right edge. Disclaimers,
  notices, warnings, and other multi-sentence text across the pages (Rolls, Timed Text
  Macros, Settings, Garapon, Stamp Rally, Raffle, Bingo) now wrap at the window width.

### [2.2.1.0] — 2026-07-16

Makes the auto-`/tell` messages fully customizable, with length-aware splitting.

#### Added

- **Editable auto-tell templates** for all three tells (bingo card, garapon, stamp
  rally): enabling a tell in Settings now reveals a message editor. Templates use
  placeholders the plugin expands — `<t>` (recipient's character name) and
  `<bingocard-link>` / `<garapon-link>` / `<stamprally-link>` (the relevant link).
  Defaults reproduce the previous fixed messages, so an already-enabled tell behaves
  the same until it's customized.

#### Changed

- **Long tells are split to fit the in-game chat limit.** Because the link
  placeholders are expanded by the plugin, not the game, they're **measured at the
  full width of the URL** they become. A message longer than one chat message is split
  into multiple tells at the best break point (a sentence end where possible, else a
  word boundary — nothing is dropped) and the parts are delivered a second apart to
  respect the chat throttle. The Settings editor shows a live warning — _"This message
  is too long and will be split into two separate tells."_ — when a template will split.

### [2.2.0.0] — 2026-07-16

Adds **Garapon** and **Stamp Rally** management, and replaces the tab bar with a
**collapsible sidebar**. Uses only existing server endpoints (paired with backend
3.9.0 for the public-page link, which is a web/backend concern).

#### Added

- **Garapon management** (gated on `festival-garapon`): a **Garapon** page to issue
  a per-player drawing link for an existing garapon — quick-fill the name from the
  nearby list, set the draw count, and **copy the link** (plus a **copy stamp-card
  link** button when the garapon is tied to a Stamp Rally; the server issues that
  paired card automatically on create, sharing the token). It is create-only, by
  design — no edit/delete from in-game. A separate **Garapon Draw Log** page lists
  the recorded pulls.
- **Stamp Rally management** (gated on `festival-stamp-rally`): a **Stamp Rally**
  page to issue a participant card for a nearby player (**copy the card link**) and
  **pause/resume** individual stalls. A separate **Stamp Rally Log** page shows the
  collected-stamp log.
- **Optional `/tell` on create** for both, **off by default** (opt-in per feature in
  Settings): when you issue a link/card for a player picked from the nearby list, the
  plugin can `/tell` them the URL. Outgoing chat — see the ToS note in Settings.
- **Collapsible sidebar navigation** replacing the flat tab bar: **Bingo** and
  **Festival** sections with per-page links, plus **Settings** and **About**.
  Sections and links are hidden for permissions the account lacks, mirroring the web
  admin sidebar; the window opens on the first page the account can reach.

### [2.1.0.0] — 2026-07-11

Adds the **"It's Yoever"** bingo controls (paired with backend 3.6.0).

#### Added

- **"It's Yoever" live controls** on the Bingo Game tab: a checkbox to switch the
  reaction on/off for all players during a live game (`PATCH /api/game` →
  `yoever_enabled`), and a **"Yoevers: N"** counter. Both track the server live —
  the count updates on the `yoever` broadcast (between draws) and the toggle syncs
  on `yoever_config` — and the game-state model carries `yoever_enabled` /
  `yoever_count`.

### [2.0.1.0] — 2026-07-02

#### Security

- **Personal access token moved to the `Authorization` header on the WebSocket
  connection** (was previously on the WS URL query string, where it could land in
  access logs). REST calls already used the header. Plus assorted bug fixes.

### [2.0.0.0] — 2026-07-01

#### Changed

- **Major API migration** to match the backend's move to a hybrid RESTful-RPC
  style with proper HTTP status codes — the plugin's API client was reworked
  accordingly. (Major bump: it requires the correspondingly migrated backend.)

### [1.0.0.0] — 2026-06-29

#### Added

- **First published release** through the Dalamud custom repo (repo listing
  finalized).

#### Changed

- Cards sorted by creation date.

### [0.1.0.0] — 2026-06-29

#### Added

- **Initial Dalamud plugin** (Senpan Companion): operate bingo and raffle features
  from within the game, authenticated by a personal access token.

---

## How to update this file

1. Make your change and bump the relevant version source
   (`frontend/package.json`, `backend/internal/version/version.go`, and/or the
   plugin's `SenpanCompanion.csproj` `<Version>` + `plugins/pluginmaster.json`).
2. Add an entry under the matching section above (Frontend / Backend / Plugin),
   newest first, grouped as _Added / Changed / Fixed / Removed / Security_.
3. Keep the version string in the source file and the heading here in sync.
