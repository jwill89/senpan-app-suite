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
- Backend version → `src/internal/version/version.go` (`Version`), served at
  `GET /api/version`.

**Compatibility rule:** the SPA and API are compatible while their **MAJOR**
versions match. Bump MAJOR only for a breaking change to the JSON/WebSocket API
the SPA depends on; MINOR for backward-compatible additions; PATCH for fixes.
When you change one side, bump its version and add an entry under its section.

The format follows [Keep a Changelog](https://keepachangelog.com/).

---

## Frontend

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
   (`frontend/package.json` and/or `src/internal/version/version.go`).
2. Add an entry under the matching section above, newest first, grouped as
   _Added / Changed / Fixed / Removed_.
3. Keep the version string in the source file and the heading here in sync.
