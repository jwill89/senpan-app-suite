# Senpan App Suite — HTTP & WebSocket API

The backend exposes a JSON/WebSocket API under `/api`. This document is the
canonical reference for every endpoint: its method, path, authentication
requirement, request body, and response shape. It is generated from the handler
source in [`backend/internal/server/`](backend/internal/server/) — when you add
or change an endpoint, update this file alongside the handler (see
[CONTRIBUTING.md](CONTRIBUTING.md)).

> **Source of truth.** Data-transfer struct fields use the exact JSON tags from
> [`backend/internal/model/model.go`](backend/internal/model/model.go). The Go
> models are also the source for the generated TS types
> (`frontend/src/types/api.generated.ts`, via `npm run gen:types`).

## Conventions

- **Base path:** every endpoint is under `/api`.
- **Content type:** all responses are `application/json; charset=utf-8` with
  `Cache-Control: no-store` (the WebSocket upgrade is the only exception).
- **Request bodies:** JSON, capped at **1 MB** (`readJSON`). Multipart uploads
  have their own caps, noted per endpoint.
- **Errors:** the envelope is always `{"error": "<message>"}` with an
  appropriate HTTP status. A malformed JSON body returns `400 "Invalid JSON"`.
- **Action dispatchers:** most `POST` endpoints take an `{"action": "..."}`
  field and switch on it. Each action and its extra fields are documented below.
- **CORS:** no CORS headers are sent unless the request `Origin` is in an
  explicit allowlist (normally empty — the SPA and API are same-origin).
  `OPTIONS` preflight returns `204`.
- **Timestamps:** read-only timestamps are ISO-8601 strings; scheduling/event
  fields are UTC RFC-3339 unless documented as wall-clock.

## Authentication

Identity is resolved per request (`currentUser`) from one of two sources:

1. **Cookie session** — `scs` session cookie (HttpOnly, Secure,
   `SameSite=Lax`, 24-hour lifetime), set by `POST /api/auth` (login). Stores
   the account's `user_id`.
2. **Personal access token (PAT)** — used when there is no cookie session, so
   external clients (e.g. the FFXIV plugin) authenticate through the same
   guards. Supply it as `Authorization: Bearer pat_…` or, for the WebSocket
   upgrade, `?token=pat_…`. The token is SHA-256 hashed for lookup; a successful
   call stamps `last_used_at`.

### Guards

| Guard | Passes when | Failure |
| ----- | ----------- | ------- |
| _public_ | always | — |
| `requireAuth` | any authenticated **active** account | `401 "Unauthorized – login required"` |
| `requireAdmin` | active **admin** account | `401 "Unauthorized – admin login required"` |
| `requirePermission("<key>")` | admin, or non-admin holding the page-permission key | `401` if unauthenticated, `403 "Forbidden – you do not have access to this feature"` if lacking the key |
| `requireAnyBookClub` | admin, or holder of at least one `bookclub-<slug>` key | `401` / `403` |

Admins implicitly hold **every** permission. A PAT inherits its owning account's
guards.

### Page-permission keys

Grantable keys (`validPermissions()`):

`bingo-game`, `bingo-cards`, `bingo-winners-log`, `bingo-patterns`,
`bingo-presets`, `teahouse-announcements`, `teahouse-affiliates`,
`teahouse-raffles`, `festival-garapon`, `festival-stamp-rally`,
`atelier-fonts`, `atelier-carrd`, `system-settings`, `system-themes`,
`system-images`, and per-book-club `bookclub-yaoi`, `bookclub-yuri`.

The **Users** page (`system-users`) is intentionally **not** grantable — it is
admin-only via `requireAdmin`.

### Public (unauthenticated) endpoints

`GET /api/version`, `GET /api/config`, `GET /api/auth`, `POST /api/auth`,
`POST /api/register`, `GET /api/board`, `GET /api/game`,
`GET /api/styles/active`, `GET /api/settings` (secret values admin-gated),
`GET /api/raffles`, `GET /api/raffles/{id}`, `POST /api/raffles/{id}/enter`,
`GET /api/garapon/{token}`, `POST /api/garapon/{token}/draw`,
`GET /api/stamp-card/{token}`, `POST /api/stamp-card/{token}/stamp`, and the
player WebSocket (`/api/ws?id=…`). Everything else requires a session or PAT.

---

## System

### `GET /api/version`
Public. Reports the backend semver (doubles as a health probe).
**200** → `{"backend": "1.5.0"}`

### `GET /api/config`
Public. Client bootstrap config — the non-secret Cloudflare Turnstile site key
(empty when disabled).
**200** → `{"turnstile_site_key": "..."}`

---

## Auth, Users & Account

### `GET /api/auth`
Public. Current auth status and the active user (for UI gating).
**200** → `{"authenticated": bool, "user": User|null}`

### `POST /api/auth`
Public. Login is IP rate-limited (5 failures / 15 min) and Turnstile-gated when
configured.

Body (`authRequest`): `action` (`"login"` default | `"logout"`), `username`,
`password`, `turnstile_token` (required when Turnstile enabled).

| action | result |
| ------ | ------ |
| `login` | verifies argon2id hash, rotates session, stamps last login → **200** `{"success": true, "user": User}` |
| `logout` | destroys session → **200** `{"success": true}` |

Errors: `429` too many attempts · `403` bot check / `"Account pending activation by an administrator"` · `401 "Invalid username or password"`.

### `POST /api/register`
Public. IP rate-limited (5 / hour) and Turnstile-gated. Creates an **inactive,
non-admin, no-permission** account that an admin must activate.

Body (`registerRequest`): `username` (1–32 chars, may not be `admin`),
`password` (min 8), `turnstile_token`.

**200** → `{"success": true, "message": "..."}` · Errors: `400`, `403`, `429`, `409 "That username is already taken"`.

### `GET /api/users`  — `requireAdmin`
**200** → `{"users": [User, ...]}` (never includes password hashes).

### `POST /api/users`  — `requireAdmin`
Body (`usersRequest`): `action`, `id` (int64, required), `active`, `admin`,
`permissions` ([]string), `password`. Target must exist (`404`). The reserved
`admin` account is protected — `set_active` / `set_admin` / `set_password` /
`delete` on it return `403`; only `set_permissions` is allowed.

| action | extra fields | notes |
| ------ | ------------ | ----- |
| `set_active` | `active` | |
| `set_admin` | `admin` | |
| `set_permissions` | `permissions` | each must be in `validPermissions()` (else `400`); deduped |
| `set_password` | `password` | min 8 |
| `delete` | — | |

**200** → `{"ok": true}`

### `POST /api/account`  — `requireAuth` (PAT ok)
Self-service. The protected `admin` account rotates its own password here.

Body (`accountRequest`): `action` (`change_password`), `current_password`,
`new_password` (min 8).

**200** → `{"ok": true}` · Errors: `401 "Current password is incorrect"`, `400` short password.

### `GET /api/account/token`  — `requireAuth` (PAT ok)
PAT metadata (never the token itself).
**200** → `{"has_token": bool, "prefix": "...", "created_at": "...", "last_used_at": "..."}`

### `POST /api/account/token`  — `requireAuth` (PAT ok)
Body (`accountTokenRequest`): `action`.

| action | result |
| ------ | ------ |
| `generate` | creates `pat_<base64url>` (replaces any existing); plaintext returned **once** → **200** `{"token": "...", "prefix": "...", "created_at": "..."}` |
| `revoke` | deletes the token → **200** `{"ok": true, "deleted": bool}` |

**`User`**: `id`, `username`, `is_admin`, `is_active`, `permissions` ([]string,
ignored when admin), `created_at`, `last_login_at` (`""` if never).

---

## Bingo — Board, Cards, Game, Patterns

### `GET /api/board`
Public. The player's card plus current game state.
Query: `id` (required, 6-char card ID) · `preview` (any non-empty value → card only).
**200** → `{"card": Card, "game": BingoGameState|null, "game_details": string}` (preview: `{"card": Card}`).
Errors: `400 "Board ID is required"`, `404 "Board not found"`.

### `GET /api/cards`  — `requirePermission("bingo-cards")`
**200** → `{"cards": [{id, player_name, details, created_at}, ...]}`

### `POST /api/cards`  — `requirePermission("bingo-cards")`
Body (`cardsRequest`): `action`, `id`, `count`, `player_name`, `details`.

| action | extra fields | result |
| ------ | ------------ | ------ |
| `generate` | `count` (clamped 1–500) | `{"cards": [{id, board_data}], "count": int}` |
| `generate_single` | `player_name` (optional) | `{"card": {id, player_name, board_data}, "count": 1}` |
| `delete` | `id` (required) | `{"deleted": bool}`; sends `card_deleted` to that card's players |
| `delete_all` | — | `{"deleted": int}`; disconnects all players |
| `update_player` | `id` (required), `player_name`, `details` | `{"ok": true}` |

All actions broadcast `cards_update`.

### `GET /api/game`
Public.
**200** → `{"game": BingoGameState|null, "winners": [string, ...], "game_details": string}`

### `POST /api/game`  — `requirePermission("bingo-game")`
Body (`gameRequest`): `action`, `pattern_ids` ([]int), `details`, `delay` (int),
`valid_winner_ids` ([]string).

| action | extra fields | result / notes |
| ------ | ------------ | -------------- |
| `start` | `pattern_ids` (≥1) | `{"game": BingoGameState, "winners": [], "game_details": string}`; `400 "Select at least one pattern"` |
| `draw` | `delay` (0–60) | `{"drawn": BingoDrawnNumber, "game": BingoGameState, "winners": [string]}`; admins notified immediately, players after `delay`; `400` when all 75 drawn |
| `end` | `valid_winner_ids` (optional) | `{"ended": bool}`; logs winners |
| `trigger_halftime` | — | `{"ok": true}`; broadcasts `halftime_minigame` |
| `set_delay` | `delay` (0–60) | `{"ok": true}`; persists `default_draw_delay`; broadcasts `draw_delay_update` |
| `update_details` | `details` | `{"ok": true}`; broadcasts `details_update` |

### `GET /api/patterns`  — `requirePermission("bingo-patterns")`
**200** → `{"patterns": [Pattern], "categories": [PatternCategory]}`

### `POST /api/patterns`  — `requirePermission("bingo-patterns")`
Body (`patternRequest`): `action`, `id` (int), `name`, `pattern_data` ([][]bool,
5×5), `direction` (`"up"`|`"down"`), `category_id` (int64), `ordered_ids` ([]int).

| action | extra fields | result |
| ------ | ------------ | ------ |
| `create` | `name`, `pattern_data` | **201** `{"pattern": {...}}`; `409` on duplicate |
| `delete` | `id` | `{"deleted": bool}` |
| `rename` | `id`, `name` | `{"renamed": bool}` |
| `reorder` | `id`, `direction` | `{"patterns": [...], "categories": [...]}` |
| `set_category` | `id`, `category_id` | `{"ok": true}` |
| `bulk_reorder` | `category_id`, `ordered_ids` | `{"patterns": [...], "categories": [...]}` |

All mutations broadcast `patterns_update`.

### `GET /api/pattern-categories`  — `requirePermission("bingo-patterns")`
**200** → `{"categories": [PatternCategory]}`

### `POST /api/pattern-categories`  — `requirePermission("bingo-patterns")`
Body (`categoryRequest`): `action`, `id` (int64), `name`, `direction`,
`ordered_ids` ([]int64).

| action | extra fields | result |
| ------ | ------------ | ------ |
| `create` | `name` | **201** `{"id": int64, "name": string}` |
| `rename` | `id`, `name` | `{"renamed": bool}` |
| `delete` | `id` | `{"deleted": true}`; `400 "Cannot delete the last category"` |
| `reorder` | `id`, `direction` | `{"categories": [...]}` |
| `bulk_reorder` | `ordered_ids` | `{"categories": [...]}` |

**Structs.** `Card` = `id`, `board_data` ([][]int; `0` = FREE), `player_name`,
`details`, `created_at`. `BingoGameState` = `id`, `created_at`,
`called_numbers` ([]int), `patterns` ([]BingoGamePattern), `total_called`.
`BingoDrawnNumber` = `number` (1–75), `letter` (B/I/N/G/O), `call_order`.
`Pattern` = `id`, `name`, `pattern_data` ([][]bool), `sort_order`,
`category_id`, `category_name`. `PatternCategory` = `id`, `name`, `sort_order`.

---

## Bingo — Presets & Styles

### `GET /api/presets`  — `requirePermission("bingo-presets")`
**200** → `{"presets": [GamePreset]}` (`GamePreset` = `id`, `name`,
`pattern_ids` []int64, `game_details`, `created_at`).

### `POST /api/presets`  — `requirePermission("bingo-presets")`
Body (`presetRequest`): `action`, `id`, `name`, `pattern_ids`, `game_details`.

| action | extra fields | result |
| ------ | ------------ | ------ |
| `create` | `name`, `pattern_ids` (≥1) | **201** `{"id": int64}` |
| `update` | `id`, `name`, `pattern_ids` | `{"ok": true}` |
| `delete` | `id` | `{"deleted": bool}` |

### `GET /api/styles`  — `requirePermission("system-themes")`
**200** → `{"styles": [Style], "active_style_id": "..."}` (CSS omitted in list).

### `POST /api/styles`  — `requirePermission("system-themes")`
Body (`styleRequest`): `action`, `id`, `name`, `tokens` (map[string]string),
`board_flourish`, `number_flourish`.

| action | extra fields | result |
| ------ | ------------ | ------ |
| `get` | `id` | `{"style": Style}` (includes `tokens`); `404` |
| `create` | `name` | **201** `{"id": int64, "name": string}` |
| `update` | `id`, `name` | `{"ok": true}`; broadcasts `style_update` if active |
| `delete` | `id` | `{"deleted": bool}`; clears active if it was active |
| `set_active` | `id` (≤0 clears) | `{"ok": true}`; broadcasts `style_update` |

`Style` = `id`, `name`, `tokens` (map; omitempty), `css_content` (generated,
omitempty), `board_flourish`, `number_flourish`, `created_at`.

### `GET /api/styles/active`
Public. The active theme's generated CSS + flourishes (JSON, not raw CSS).
**200** → `{"css": string, "board_flourish": string, "number_flourish": string}` (all `""` when none active).

---

## Raffles

### `GET /api/raffles`
Public; response is role-filtered (admins see all; non-admins see only open
raffles inside their availability window).
**200** → `{"raffles": [Raffle]}`

### `POST /api/raffles`  — `requirePermission("teahouse-raffles")`
Body (`raffleRequest`): `action`, `id`, `title`, `description`, `rules`,
`max_entries`, `signup_instructions`, `cost_per_entry`, `available_from`,
`available_to` (UTC RFC-3339), `prize_image`.

| action | extra fields | result |
| ------ | ------------ | ------ |
| `create` | `title` (required) | **201** `{"raffle": Raffle}` (status forced `"open"`) |
| `update` | `id`, `title` | `{"ok": true}` |
| `delete` | `id` | `{"deleted": bool}` |

### `GET /api/raffles/{id}`
Public; response varies by admin status.
**200** → `{"raffle": Raffle, "total_entries": int}` plus, for admins,
`"entries": [RaffleEntry]`; for the public on a closed raffle with a winner,
`"winner_entry": RaffleEntry`. Errors: `400`, `404`.

### `POST /api/raffles/{id}/enter`
Public, unauthenticated.
Body (`raffleEntryRequest`): `character_name` (required), `world` (required),
`num_entries` (≥1).
**201/200** → `{"message": ..., "total_entries": int, "total_cost": float, "signup_instructions": string}`.
Errors: `400` (missing fields / not open / outside window / exceeds `max_entries`), `404`.

### `POST /api/raffles/{id}/entries`  — `requirePermission("teahouse-raffles")`
Body (`raffleEntriesRequest`): `action`, `entry_id`, `paid`, `character_name`,
`world`, `num_entries`.

| action | extra fields | result |
| ------ | ------------ | ------ |
| `add_entry` | `character_name`, `world`, `num_entries`, `paid` | `{"entry": RaffleEntry}` |
| `mark_paid` | `entry_id` | `{"ok": true}` |
| `delete_entry` | `entry_id` | `{"deleted": bool}` |
| `pick_winner` | — | `{"winner": RaffleEntry}` (random paid entry); `400 "No paid entries to pick from"` |
| `verify_winner` | — | `{"ok": true, "status": "closed"}` |
| `pick_another` | — | `{"winner": RaffleEntry}` (re-picks) |

`Raffle` = `id`, `title`, `description`, `rules`, `max_entries`,
`signup_instructions`, `cost_per_entry`, `available_from`, `available_to`,
`prize_image`, `status`, `winner_entry_id` (*int64), `created_at` (+ admin-list
aggregates `winner_name`, `paid_total`). `RaffleEntry` = `id`, `raffle_id`,
`character_name`, `world`, `num_entries`, `paid`, `created_at`.

---

## Garapon (festival lottery drum)

### `GET /api/garapons`  — `requirePermission("festival-garapon")`
**200** → `{"garapons": [Garapon]}` (with `player_count`, `draw_count`; prizes omitted in list).

### `POST /api/garapons`  — `requirePermission("festival-garapon")`
Body (`garaponRequest`): `action`, `id`, `title`, `details`,
`grand_prize_image`, `status` (set_status only), `stamp_rally_id` (*int64;
optional link to an **open** rally), `prizes` ([]GaraponPrize; ≥1, exactly one
`is_grand`). Prize fields: `name`, `ball_color` (default `#cccccc`), `rate`,
`is_grand`.

| action | extra fields | result |
| ------ | ------------ | ------ |
| `create` | `title`, `prizes` | **201** `{"garapon": Garapon}`; `400` title/prize validation |
| `update` | `id` | `{"ok": true}` |
| `delete` | `id` | `{"deleted": bool}` |
| `set_status` | `id`, `status` (`open`/`closed`) | `{"ok": true, "status": ...}` |

### `GET /api/garapons/{id}`  — `requirePermission("festival-garapon")`
**200** → `{"garapon": Garapon, "players": [GaraponPlayer], "draws": [GaraponDraw]}`. `404`.

### `POST /api/garapons/{id}/players`  — `requirePermission("festival-garapon")`
Body (`garaponPlayersRequest`): `action`, `player_id`, `player_name`,
`max_draws` (≥1).

| action | extra fields | result |
| ------ | ------------ | ------ |
| `create_player` | `player_name` | **201** `{"player": GaraponPlayer}`; also issues a stamp card sharing the token when the garapon links an open rally |
| `delete_player` | `player_id` | `{"deleted": bool}`; `404`; `409` if the player already drew while open |

### `GET /api/garapon/{token}`
Public (token is the capability).
**200** → `{"garapon": {id, title, details, grand_prize_image, status, prizes:[…with rate zeroed]}, "player": {player_name, max_draws, draws_used}, "draws": [GaraponDraw]}`. `404`.

### `POST /api/garapon/{token}/draw`
Public. No body.
**200** → `{"draw": GaraponDraw, "draws_used": int, "max_draws": int}`.
Errors: `400 "This garapon is closed"` / `"…has no prizes configured"`,
`409 "No draws remaining"`, `404`.

`Garapon` = `id`, `title`, `details`, `grand_prize_image`, `status`,
`stamp_rally_id` (*int64), `stamp_rally_title`, `created_at`, `prizes`
(detail only), `player_count`/`draw_count` (list only). `GaraponPrize` = `id`,
`garapon_id`, `name`, `ball_color`, `rate`, `is_grand`, `sort_order`.
`GaraponPlayer` = `id`, `garapon_id`, `token`, `player_name`, `max_draws`,
`draws_used`, `created_at`, `stamp_card_token`. `GaraponDraw` = `id`,
`garapon_id`, `player_id`, `prize_id`, `player_name`, `prize_name`,
`ball_color`, `drawn_at`.

---

## Affiliates

### `GET /api/affiliates`  — `requirePermission("teahouse-affiliates")`
**200** → `{"affiliates": [Affiliate]}` (alphabetical; admin-only, no public view).

### `POST /api/affiliates`  — `requirePermission("teahouse-affiliates")`
Body (`affiliateRequest`): `action`, `id`, `name`, `owners` ([]string),
`location`, `timezone` (IANA), `hours` ([]AffiliateHour: `label`, `start`,
`end`), `details`, `logo`, `screenshot`.

| action | extra fields | result |
| ------ | ------------ | ------ |
| `create` | `name` (required) | **201** `{"affiliate": Affiliate}` |
| `update` | `id`, `name` | `{"ok": true}` |
| `delete` | `id` | `{"deleted": bool}` |

---

## Stamp Rally

### `GET /api/stamp-rallies`  — `requirePermission("festival-stamp-rally")`
**200** → `{"stamp_rallies": [StampRally]}` (aggregates only).

### `POST /api/stamp-rallies`  — `requirePermission("festival-stamp-rally")`
Body (`stampRallyRequest`): `action`, `id`, `title`, `card_image`,
`not_stamped_image`, `available_from`, `available_to` (UTC RFC-3339), `details`,
`redeem_instructions`, `status` (set_status only), `stamps` ([]StampRallyStamp),
`prizes` ([]StampRallyPrize).

| action | extra fields | result |
| ------ | ------------ | ------ |
| `create` | `title` (required) | **201** `{"stamp_rally": StampRally}` (status `"open"`) |
| `update` | `id`, `title` | `{"ok": true}` |
| `set_status` | `id`, `status` | `{"ok": true, "status": ...}` |
| `delete` | `id` | `{"deleted": bool}` |

### `GET /api/stamp-rallies/{id}`  — `requirePermission("festival-stamp-rally")`
**200** → `{"stamp_rally": StampRally (+ stamps, prizes), "cards": [StampRallyCard]}`. `404`.

### `GET /api/stamp-rallies/{id}/logs`  — `requirePermission("festival-stamp-rally")`
**200** → `{"logs": [{card_id, participant_name, stamp_id, stall_name, stamped_at}]}`

### `POST /api/stamp-rallies/{id}/stamps`  — `requirePermission("festival-stamp-rally")`
Body (`stampRallyStampsRequest`): `action` (`set_paused` only), `stamp_id`
(required), `paused`.
**200** → `{"ok": true, "paused": bool}`. `404 "Stamp not found"`.

### `POST /api/stamp-rallies/{id}/cards`  — `requirePermission("festival-stamp-rally")`
Body (`stampRallyCardsRequest`): `action`, `card_id`, `participant_name`.

| action | extra fields | result |
| ------ | ------------ | ------ |
| `create_card` | `participant_name` | **201** `{"card": StampRallyCard}` |
| `delete_card` | `card_id` | `{"deleted": bool}`; `409` if rally open and card has collected stamps |

### `GET /api/stamp-card/{token}`
Public (token is the capability). Completion is recomputed and persisted lazily
on read.
**200** → `{"rally": {id, title, card_image, not_stamped_image, details, redeem_instructions, available_from, available_to, is_active}, "participant_name": string, "completed": bool, "completed_at": string, "stamps": [{id, affiliate_name, image, placement, active_from, active_to, available, collected, collected_at}], "prizes": [{id, name, image, placement}], "prizes_revealed": bool}`. (Prize `name`/`image` only filled when complete.) `404`.

### `POST /api/stamp-card/{token}/stamp`
Public.
Body (`stampSubmitRequest`): `password` (required).
**200** → `{"card": <publicCard>, "collected_stamp_id": int64}`.
Errors: `400 "Enter a password"` / `"That password doesn't match any stamp on this card"` / `"This stall is currently closed and cannot be stamped"`, `409 "You've already collected this stamp"`.

`StampRally` = `id`, `title`, `card_image`, `not_stamped_image`,
`available_from`, `available_to`, `details`, `redeem_instructions`, `status`,
`created_at`, `stamps`/`prizes` (detail only), list aggregates
(`card_count`, `completed_count`, `stamp_count`, `active_stamp_count`).
`Placement` = `x`, `y`, `width`, `height` (percent 0–100), `rotation` (deg).

---

## Book Club / Reading Lists

### `POST /api/bookclub/upload`  — `requireAnyBookClub`
Upload a reading-list item cover. `multipart/form-data`, field **`image`**
(max 5 MB; ext jpg/jpeg/png/webp/gif **and** content-sniffed as a real image). The
uploaded filename is preserved (a same-named upload overwrites), matching the
central image and Carrd uploads.
**200** → `{"url": "<scheme>://<host>/images/bookclub/<original-filename>"}`

### `GET /api/bookclub/lookup`  — `requireAnyBookClub`
Proxies the AniList GraphQL API → reading-list-item shapes.
Query: `q` (search) **or** `id` (single AniList media id; `id` wins). One required.
**200** → `{"results": [ReadingListItem-shaped]}`. `400`, `502`.

### `GET /api/reading-lists`  — `requirePermission("bookclub-<club>")`
Query: `club` (slug; default `"yaoi"`).
**200** → `{"reading_lists": [ReadingList]}` (items omitted).

### `POST /api/reading-lists`  — `requireAuth` + `requirePermission("bookclub-<club>")`
Club = body `club_slug` (create) or the record's club (update/delete).
Body (`readingListRequest`): `action`, `id`, `club_slug`, `title`.

| action | extra fields | result |
| ------ | ------------ | ------ |
| `create` | `title` | **201** `{"reading_list": ReadingList}` |
| `update` | `id`, `title` | `{"ok": true}` |
| `delete` | `id` | `{"deleted": ...}`; removes uploaded cover files |

### `GET /api/reading-lists/{id}`  — `requireAuth` + `requirePermission("bookclub-<list.club>")`
**200** → `{"reading_list": ReadingList}` (items populated). `404`.

### `POST /api/reading-lists/{id}/items`  — `requireAuth` + `requirePermission("bookclub-<list.club>")`
Body (`readingListItemRequest`): `action`, `item_id`, `item`
(`ReadingListItem`: `cover_image`, `title` (required), `summary`, `format`,
`genres`, `tropes`, `chapters`, `comments`, `sources` ([{title, url}]),
`sort_order` — server forces `list_id`).

| action | extra fields | result |
| ------ | ------------ | ------ |
| `create` | `item` | **201** `{"item": ReadingListItem}` |
| `update` | `item_id`, `item` | `{"item": ReadingListItem}` |
| `delete` | `item_id` | `{"deleted": ...}`; removes uploaded cover file |

### `POST /api/reading-lists/{id}/publish`  — `requireAuth` + `requirePermission("bookclub-<list.club>")`
Publishes every item as its own Discord embed to the club webhook. No body.
**200** → `{"published": <count>}`. `400` (no items / no webhook), `404`, `502`.

`ReadingList` = `id`, `club_slug`, `title`, `created_at`, `items` (detail only).

---

## Announcements

All six endpoints use `requirePermission("teahouse-announcements")`.

### `GET /api/announcement-types`
**200** → `{"types": [{id, name, webhook_url, created_at}]}`

### `POST /api/announcement-types`
Body (`announcementTypeRequest`): `action` (`create`/`update`/`delete`), `id`,
`name`, `webhook_url` (if set, must be a Discord webhook URL).
Returns `{"type": AnnouncementType}` / `{"deleted": ...}`; `delete` is `400` if
referenced by an announcement.

### `GET /api/announcement-roles`
**200** → `{"roles": [{id, name, role_id, created_at}]}`

### `POST /api/announcement-roles`
Body (`announcementRoleRequest`): `action`, `id`, `name`, `role_id` (Discord
snowflake, digits only). `delete` is `400` if tagged by an announcement.

### `GET /api/announcements`
**200** → `{"announcements": [Announcement]}` (with joined `type_name`).

### `POST /api/announcements`
Body (`announcementRequest`): `action`, `id`, `announcement`
(`model.Announcement`), `ordered_ids` ([]int64, reorder).

`announcement` fields: `type_id` (required, must exist), `title` (required),
`details` (required, markdown), `image`, `thumbnail`, `color` (`#rrggbb`),
`location`, `start_local`/`end_local`/`once_local` (wall-clock
`2006-01-02T15:04`), `start_format`/`end_format` (Discord style `t|T|d|D|f|F|R`),
`dynamic_dates`, `schedule_kind` (`""|once|daily|weekly|monthly`), `timezone`
(IANA; required when any time is present), `schedule_minutes`,
`schedule_weekdays` (CSV `0..6`), `schedule_week_of_month` (1..5 or -1),
`buttons` ([{label, emoji, url}], max 5), `mention`
(`""|"everyone"|"role:<id>"`). Server-set: `start_at`/`end_at`/`next_post_at`,
`active`, `last_posted_at`.

| action | extra fields | result |
| ------ | ------------ | ------ |
| `create` | `announcement` | **201** `{"announcement": Announcement}` |
| `update` | `id`, `announcement` | `{"announcement": ...}`; `404` |
| `delete` | `id` | `{"deleted": ...}` (images left intact) |
| `send_now` | `id` | posts to Discord → `{"announcement": ...}`; `400` if type has no webhook, `502` on Discord failure |
| `skip_next` | `id` | `{"announcement": ...}`; `400` if not scheduled |
| `reorder` | `ordered_ids` | `{"ok": true}` |

---

## Winners Log

All three use `requirePermission("bingo-winners-log")`.

### `GET /api/winners-log`
Query: `page` (default 1), `per_page` (1–200, else 25),
`sort` (`logged_at|card_id|player_name|game_details`), `dir` (`asc|desc`).
**200** → `{"entries": [{id, logged_at, card_id, player_name, game_details, winning_patterns}], "total": int, "page": int, "per_page": int}`

### `POST /api/winners-log`
Body (`winnersLogRequest`): `action` (`delete`/`delete_all`), `id`.
**200** → `{"ok": true}`.

### `GET /api/winners-log/frequent`
Players with N+ wins in the last H hours (thresholds from settings
`frequent_winner_threshold` (default 3) and `frequent_winner_hours` (default 12)).
**200** → `{"winners": [{player_name, win_count}]}`

---

## Settings

### `GET /api/settings`
Public; reads admin status only to decide whether to blank "secret" settings.
**200** → `{"settings": {<key>: "<value>", ...}, "uploaded_fonts": ["<name>", ...]}`.
Keys: `app_title`, `default_draw_delay`, `frequent_winner_threshold`,
`frequent_winner_hours`, `header_font`, `google_fonts_api_key`,
`anilist_api_url`, `bingo_join_prompt`. Secret settings (e.g. per-club Discord
webhook URLs) return `""` to non-admins.

### `POST /api/settings`  — `requirePermission("system-settings")`
Body (`settingsRequest`): `settings` (map[string]string, required).
Validation: unknown key → `400 "Unknown setting: <key>"`; secret values must be
a Discord webhook URL; `default_draw_delay` 0–60; `frequent_winner_threshold`
1–100; `frequent_winner_hours` 1–168. Broadcasts `settings_update` when
`app_title`/`header_font` change.
**200** → `{"ok": true}`

---

## File hosting — Fonts, Carrd, Images

### Fonts (`<webRoot>/fonts`; ext `.ttf .otf .woff .woff2 .eot`)

| Endpoint | Auth | Notes |
| -------- | ---- | ----- |
| `GET /api/fonts` | `requirePermission("atelier-fonts")` | `{"fonts": [{name, size, modified}]}` |
| `POST /api/fonts` | `requirePermission("atelier-fonts")` | `fontsActionRequest`: `delete` / `rename` (`name`, `new_name`) |
| `POST /api/fonts/upload` | `requirePermission("atelier-fonts")` | multipart field **`files`** (repeated), 64 MB cap; same-name files are **skipped** (no overwrite) → `{"uploaded": [...], "skipped": [{name, reason}]}` |

### Carrd (projects = folders under `<webRoot>/carrd`; uploads **overwrite**)

| Endpoint | Auth | Notes |
| -------- | ---- | ----- |
| `GET /api/carrd/projects` | `requirePermission("atelier-carrd")` | `{"projects": [{title, folder, file_count, subfolder_count, total_size, modified}]}` |
| `POST /api/carrd/projects` | `requirePermission("atelier-carrd")` | `create` / `rename` / `delete` (`title`, `folder`, `new_folder`) |
| `GET /api/carrd/images` | `requirePermission("atelier-carrd")` | query `folder` (required), `path` (traversal-validated) → `{folder, path, dirs, images}` |
| `POST /api/carrd/images` | `requirePermission("atelier-carrd")` | `delete` / `create_dir` / `delete_dir` (`folder`, `path`, `name`) |
| `POST /api/carrd/upload` | `requirePermission("atelier-carrd")` | multipart `folder` (required), `path`, **`files`**, 64 MB cap; accepts `.jpg .jpeg .png .webp .gif .mp3 .mp4` |

### Images (categories = subfolders of `<webRoot>/images`; uploads **overwrite**; ext `.jpg .jpeg .png .webp .gif .svg`)

Permanent categories (not renamable/deletable): `announcements_main`,
`announcements_thumb`, `raffles`, `garapons`, `flourishes`, `affiliate_logos`,
`affiliate_images`, `stamp_cards`, `stamp_stamps`, `stamp_prizes`.

| Endpoint | Auth | Notes |
| -------- | ---- | ----- |
| `GET /api/image-categories` | `requirePermission("system-images")` | `{"categories": [{name, dir, permanent, file_count, total_size}]}` |
| `POST /api/image-categories` | `requirePermission("system-images")` | `create` / `rename` / `delete` (`name`, `dir`, `new_dir`); `403` on permanent |
| `GET /api/images` | `requireAuth` + `canAccessImageDir` | query `dir` (required, known category). Access: admin, `system-images`, **or** the editor permission owning that dir (e.g. `raffles`→`teahouse-raffles`, `garapons`→`festival-garapon`, `flourishes`→`system-themes`, `affiliate_*`→`teahouse-affiliates`, `stamp_*`→`festival-stamp-rally`, `announcements_*`→`teahouse-announcements`). → `{dir, images: [{name, url, path, size, modified}]}` |
| `POST /api/images/upload` | `requirePermission("system-images")` | multipart `dir` (required), **`files`**, 64 MB cap; raster sniffed, `.svg` validated |
| `POST /api/images` | `requirePermission("system-images")` | `imagesActionRequest`: `delete` (`dir`, `name`) |

---

## WebSocket — `/api/ws`

Upgrade with `GET /api/ws`. The route bypasses the session middleware
(coder/websocket needs the raw `ResponseWriter`), so the handler authenticates
manually. Same-origin is enforced (the request `Origin` must match `Host`; a
reverse proxy must preserve the `Host` header).

**Channel selection** is by the presence of the `id` query param — there is no
`role` param:

- **`?id=<cardID>`** → **player** connection (public, no auth). Receives draws
  after the configured delay; targeted by card-deletion disconnects.
- **no `id`** → **admin** connection. Requires an authenticated active account
  via cookie session **or** `?token=pat_…`. Returns `401` otherwise. Streams
  draws immediately (bypassing the player delay) plus winner card IDs.

Keepalive: the server pings every 30 s; inbound messages are capped at 512 bytes
and discarded (clients only send keepalives).

### Broadcast messages

Every message carries a `type` field.

| `type` | Audience | Payload | Trigger |
| ------ | -------- | ------- | ------- |
| `resource_changed` | admins | `resource` (string) | successful admin-mutation POST (thin "refetch this" signal) |
| `cards_update` | all | `cards` ([{id, player_name, details, created_at}]) | card create/delete/update |
| `card_deleted` | targeted players | (type only, then disconnect) | card delete / delete_all |
| `patterns_update` | all | `patterns` ([Pattern]), `categories` ([PatternCategory]) | pattern/category mutations |
| `game_update` | players | `game` (BingoGameState), `game_details` | game start |
| `game_update` | admins | `game` (BingoGameState), `winners` ([]) | game start |
| `game_update` | all | `game` (null) | game end |
| `game_draw` | players | `drawn` (BingoDrawnNumber) | draw (after delay) |
| `game_draw` | admins | `drawn` (BingoDrawnNumber), `winners` ([string]) | draw (immediate) |
| `halftime_minigame` | players | (type only) | `trigger_halftime` |
| `draw_delay_update` | all | `delay` (int) | `set_delay` |
| `details_update` | all | `details` (string) | `update_details` |
| `style_update` | all | `css`, `board_flourish`, `number_flourish` | active style update/set_active/delete |
| `settings_update` | all | (type only) | settings change to `app_title`/`header_font` |

---

_This document is maintained by hand from the handler source. When you add,
remove, or change an endpoint (route, guard, request/response shape, or action),
update the matching section here in the same change._
