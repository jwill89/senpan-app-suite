# Senpan App Suite - API

The HTTP API is on [huma](https://huma.rocks) (release 5.0.0,
[`plans/release-c.md`](plans/release-c.md)), on the shape shared-tooling's standards
give an API ([`shared/standards/api-and-realtime.md`](shared/standards/api-and-realtime.md)),
as Kindred Nights and Home Library have it.

## The document

- The operations on huma live under **`/api/v1`**, registered in
  [`backend/internal/api`](backend/internal/api/), one file per domain.
- huma generates their **OpenAPI 3.1** document from the code: every operation's
  inputs and outputs are Go types, so the document can't drift from them. It is
  committed as [`frontend/src/api/openapi.json`](frontend/src/api/openapi.json), and
  the SPA's types are generated from it into `frontend/src/api/schema.d.ts`
  (openapi-typescript); `frontend/src/api/client.ts` is the client (openapi-fetch).
  Neither generated file is edited by hand.
- After an API change, regenerate both and commit them with it:

  ```bash
  task gen   # go run . openapi, then openapi-typescript and Prettier
  ```

  CI's `generated` job, and `scripts/check.ps1`, run the same and fail on a difference.
- The server tests hold every documented 2xx answer to the document
  (`internal/api/apitest`): a test that calls an operation fails if the answer isn't
  one the document declares.

Nothing under `/api/` is registered by hand any more: `routes()` in
`backend/internal/server` holds the HTTP shell only. The Dalamud plugin's client is
generated from the same document (NSwag, `plugins/SenpanCompanion/nswag.json` into
`Api/SenpanApi.g.cs`, also by `task gen`), so the SPA and the plugin read the API as it
declares itself.

## Errors

Every error under `/api` is an RFC 9457 problem details body
(`application/problem+json`): `title`, `status`, a `detail` a person can read, and,
for a failed validation, `errors` naming each field. That holds for huma's
operations, a path or method the API doesn't have (404, or 405 with `Allow`), and a
crash (a generic 500).

A failed validation is a 422, each field named in `errors`; a state conflict is a 409
(a raffle that is closed, a winner changed by another console, a tea-room number taken,
a type still used by announcements, a garapon with no draws left, a stamp already
collected); an unknown or hidden item is a 404, and so is deleting one that isn't
there. A body over an operation's size cap is a 413.

### Posting to Discord

An operation that posts to Discord (`post-affiliate`, `post-tea-room`,
`send-announcement`, `publish-reading-list`, `approve-ema-plaque`, `post-ema-request`)
declares the same three errors, and never
answers 502:

| Status | When |
|---|---|
| 429 | Discord is rate-limiting; `Retry-After` says when to try again |
| 503 | Discord can't be reached, or didn't answer after the post may have gone out (the detail then says to check the channel before trying again) |
| 500 | The webhook isn't set, or Discord refused the post (a deleted webhook, a bad token); the detail carries Discord's message |

Publishing a reading list is a 409 when it has no items and a 408 when the request is
cancelled; when a post fails part-way, the detail says how far it got ("Posted N of M,
then: ..."). The AniList lookup (`look-up-title`) fails the same way: 503, 429 or 500.

A Discord webhook is read and set through its own page: `GET`/`PUT /affiliates/webhook`,
`/tea-rooms/webhook` and `/book-clubs/{club}/webhook` (`{ webhookUrl }`, empty for
none), guarded by that page. An announcement type carries its own `webhookUrl`.

## Festival

Page keys are in brackets. A visitor's operation is public; where it is token-gated the
unguessable token is the capability. A public write has its own rate limit (429 with
`Retry-After`) and, where marked, a Cloudflare Turnstile check (403 "Bot check failed.
Please try again."). A time a map or an ema day states is `{ label, start, end }`, `end`
null when unstated.

| Operations | Notes |
|---|---|
| `list-garapons`, `create-garapon`, `get-garapon`, `update-garapon`, `delete-garapon`, `close-garapon`, `reopen-garapon` [`festival-garapon`] | `/garapons[/{id}[/close or /reopen]]`. A save replaces the prizes (exactly one grand prize, the first when none); linking a closed rally is a 409 |
| `create-garapon-player`, `delete-garapon-player` [`festival-garapon`] | `/garapons/{id}/players[/{playerId}]`. A link on a garapon that serves a rally also makes the paired stamp card, in one transaction; deleting a player who has drawn is a 409 while the garapon is open |
| `get-garapon-player`, `draw-garapon` | Public, by the drawing token: `/garapon-players/{token}[/draw]`. The draw takes no body; 409 when closed, out of draws or without prizes; 429 |
| `list-stamp-rallies`, `create-stamp-rally`, `get-stamp-rally`, `update-stamp-rally`, `delete-stamp-rally`, `close-stamp-rally`, `reopen-stamp-rally`, `list-stamp-rally-logs`, `update-stamp-rally-stamp`, `create-stamp-rally-card`, `delete-stamp-rally-card` [`festival-stamp-rally`] | `/stamp-rallies[/{id}[/close, /reopen, /logs, /stamps/{stampId} or /cards[/{cardId}]]]`. A `PUT` carries `stamps` and `prizes` (`[]` clears; a stamp id not on the rally, or a festival map that doesn't exist, is a 422) |
| `list-open-stamp-rallies` | Public, `no-cache` with an ETag: open rallies that take self-service sign-ups, inside their window |
| `sign-up-for-stamp-rally` | Public, Turnstile: `POST /stamp-rallies/{id}/sign-up`, 201 with the card token (and the drawing token, once, when a link was issued); 404 for a rally not open for sign-ups; 409 when closed or the name is already signed up |
| `look-up-stamp-cards` | Public: `POST /stamp-cards/lookup` with a name (and optionally a world), a whole-name match, an empty 200 on a miss; never returns a drawing token |
| `get-stamp-card`, `stamp-stamp-card` | Public, by the card token: `/stamp-cards/{token}[/stamp]`. A password that matches no stamp is a 422 and counts against a guess limit; 409 for a stamp already collected or a stall that is closed, paused or out of hours |
| `list-festival-maps`, `create-festival-map`, `get-festival-map`, `update-festival-map`, `change-festival-map`, `delete-festival-map` [`festival-map`; the list also `festival-ema`] | `/festival-maps[/{id}]`. A `PUT` carries `stalls` (`[]` clears, a missing list is a 422); `PATCH` sets the status (publishing without a base image is a 409); a shortcode is a 409 when taken and a 422 when all digits |
| `list-public-festival-maps`, `get-public-festival-map` | Public, `no-cache` with an ETag: `/festival-maps/public[/{idOrSlug}]`, published maps only, each with `publicPath` |
| `read-festival-dashboard` | Public: `POST /festivals/{idOrSlug}/dashboard` with the card token, drawing token, name and world this device holds (all optional); a token that doesn't belong to the festival is ignored |
| `list-ema-boards`, `create-ema-board`, `get-ema-board`, `update-ema-board`, `close-ema-board`, `reopen-ema-board`, `delete-ema-board`, `list-ema-plaques`, `approve-ema-plaque`, `reject-ema-plaque` [`festival-ema`] | `/ema-boards[/{id}[/close, /reopen or /plaques[/{plaqueId}/approve or /reject]]]`. Approving claims the plaque (a second approval is a 409), posts it to the day's Discord webhook and deletes the file |
| `complete-ema-request`, `reopen-ema-request`, `post-ema-request` [`festival-ema`] | `/ema-requests/{id}/complete, /reopen or /post`. A board's requests are among its `list-ema-plaques` rows (`kind` `artist`). `post` is multipart (`image`: PNG, JPEG, GIF or WebP judged from the bytes, at most 10 MB, never written to disk) |
| `submit-ema-plaque`, `request-ema-plaque` | Public, Turnstile: `POST /festivals/{idOrSlug}/ema-plaques` (multipart: `image` PNG, `characterName`, `world`, `turnstileToken`) and `…/ema-requests` (JSON with a `description`). 409 for a second plaque the same day, a day whose artist requests are full, or no day open now |
| `list-google-fonts` | Public, `no-cache` with an ETag: the family names read with the server's key, `[]` without one |

## Media

A multipart operation declares its parts: `files` (1 to 50) for the image library, Carrd
and fonts, `image` for a cover or an ema plaque. The server judges a file by its bytes,
never by its name or the browser's type. A multi-file upload answers 201
`{ uploaded, skipped }` (a refused file is in `skipped` with its reason) when at least one
file was saved, and 422 with one error per file when none was. JPEG and PNG are
re-encoded, which drops their metadata; GIF and WebP are checked and kept as sent. Upload
names are kept, so a same-name upload replaces the file.

| Operations | Notes |
|---|---|
| `list-image-categories`, `list-images` [`system-images`, or any page that picks an image] | `/image-categories[/{dir}/images]` |
| `create-image-category`, `update-image-category`, `delete-image-category`, `upload-images`, `delete-image` [`system-images`] | `/image-categories[/{dir}[/images[/{name}]]]`. A blank `dir` on a `PATCH` keeps the folder; `bookclub`, `announcements` and `ema_plaques` are reserved (409); the upload is multipart, 64 MB |
| `list-carrd-projects`, `create-carrd-project`, `update-carrd-project`, `delete-carrd-project`, `list-carrd-files`, `upload-carrd-files`, `delete-carrd-file`, `create-carrd-folder`, `delete-carrd-folder` [`atelier-carrd`] | `/carrd-projects[/{folder}[/files or /folders]]`. A bad project name is a 404; a bad `path` (query or body) is a 422 |
| `list-fonts`, `upload-fonts`, `update-font`, `delete-font`, `rename-font-file`, `delete-font-file` [`atelier-fonts`] | `/fonts[/{base}]` and `/font-files/{name}`. A font write also sends `settings_changed` |
| `get-font-kit`, `get-font-file` | Public: `/fonts/pub/kit.css` and `/fonts/pub/f/{token}`, gated per font by its allowed sites (the `Origin`, or the `Referer` for Safari). `fonts.senpan.cafe/kit.css` and `/f/<token>` proxy to these |
| `upload-book-club-cover` [`bookclub-<slug>`] | `POST /book-clubs/{club}/covers`, multipart `image`, 5 MB, 201 `{ url }` (absolute); stored in `images/bookclub/` |

## Caching

An answer is `Cache-Control: private, no-store`, except a public read (the same for
everyone), which is `no-cache` with an `ETag`; a request that already holds that ETag
gets `304 Not Modified` with no body. The public reads are the two public tea-room ones,
the open stamp rallies, the published festival maps and the Google Fonts list. The two
font operations set their own `private` rules (the kit for five minutes, a font file for
a week) and honor `Range` and `If-None-Match`.

### The public tea-room API

`GET /api/v1/tea-rooms/public` answers `{ teaRooms: [...] }` and
`GET /api/v1/tea-rooms/public/{number}` the room itself (404 for an unknown number), for
the Carrd pages that show live availability and pricing. Every field is camelCase, and
no webhook is in it.

A browser may read it cross-origin only from an origin on the `teaRoomOrigins` setting
(System -> Settings). `Vary: Origin` is always sent; for an allowed origin the answer
echoes it in `Access-Control-Allow-Origin` (never `*`, never with credentials). A
`https://*.example.com` entry covers every subdomain of `example.com`, not the apex.
The preflight is answered for an allowed origin (`GET`, the `If-None-Match` header,
`Access-Control-Max-Age` 600). Migration 74 seeds `https://senpan.cafe` and
`https://*.senpan.cafe`; saved empty, the list allows no cross-origin read.

## Authentication (orientation)

Identity comes from either a **session cookie** (set by signing in, `POST /api/v1/auth/login` or
`POST /api/v1/auth/passkey/finish`; `GET /api/v1/me` says who it is) or a
**personal access token** - `Authorization: Bearer pat_...`, never in a URL. A token inherits its account's permissions, so external clients
(e.g. the FFXIV plugin) pass through the same guards as the SPA. An operation that
needs sign-in answers 401 when there is none, and 403 when the account lacks the
access.

Grantable page-permission keys: `bingo-game`, `bingo-cards`, `bingo-winners-log`,
`bingo-patterns`, `bingo-presets`, `teahouse-announcements`, `teahouse-affiliates`,
`teahouse-tea-rooms`, `teahouse-raffles`, `festival-map`, `festival-garapon`,
`festival-stamp-rally`, `festival-ema`, `atelier-fonts`, `atelier-carrd`, `system-settings`, `system-themes`, `system-images`,
plus per-book-club `bookclub-yaoi` / `bookclub-yuri`. The Users page is admin-only
(not grantable).

## Live events

Two streams of server-sent events replace the WebSocket. They are operations in the
document (`stream-staff-events`, `stream-board-events`), each event a named schema, so
the SPA's types for them are generated. Both are `private, no-store` with no ETag, and
the access log writes a stream once, when it closes.

| Stream | Who | Refused |
|---|---|---|
| `GET /api/v1/events` | A session or a Bearer token: the consoles and the plugin | 401 for nobody, 403 for an account with no permission, 503 when the feed can't take another stream |
| `GET /api/v1/board/{id}/events` | Anyone, by the board's card id | 404 for an unknown card, 403 for a pending personal card, 422 for a bad id, 503 with `Retry-After` past 500 open boards |

### The envelope

Every event is `{ id, type, gameId, at, payload }`: `id` is the event's number (also the
SSE `id:`; 0, with none, for a `ping` or a `log`), `type` its name (also the SSE `event:`), `gameId` the game it belongs to or
`null`, `at` an RFC 3339 time in UTC. The `payload` is the event's own. An event names
what changed and the client reads it again over REST. Two kinds carry their data
instead: a draw carries the number called, and an event with nothing to read again
carries what it reports (a log line, a reaction, who answered half-time).

### Events and who hears them

The hub filters every event by viewer. Admins hold every permission.

| Event | Payload | Heard by |
|---|---|---|
| `game_started`, `game_ended` | none: read the game again | boards, Game staff |
| `draw` | `{ drawn, winners }` | Game staff, at once |
| `draw` | `{ drawn }` | boards, after the draw delay |
| `yoever` | `{ playerName, count }` | boards, Game staff |
| `yoever_switched` | `{ enabled }` | boards, Game staff |
| `game_details` | `{ details }` | boards, Game staff |
| `halftime_prompt` | `{ autoPaused }` | Game staff |
| `halftime_resolved` | `{ minigame, by }` | Game staff |
| `auto_draw` | `{ enabled, interval }` | Game staff |
| `draw_delay` | `{ delay }` | Game or Settings staff |
| `minigame` | none | boards |
| `card_deleted` | none; that board's stream then ends | that board |
| `cards_changed` | none | holders of Cards |
| `patterns_changed` | none | holders of Patterns |
| `resource_changed` | `{ resource }` | holders of the page that shows the resource; `users` and `integrations` go to admins; `bookclub-<slug>` to that club's page. A write under `/raffles`, `/affiliates`, `/tea-rooms`, `/announcements`, `/announcement-types`, `/announcement-roles`, `/garapons`, `/stamp-rallies`, `/festival-maps`, `/ema-boards` and `/ema-requests` (both `ema-boards`), `/image-categories` (`images`), `/carrd-projects` (`carrd`), `/fonts` and `/font-files` (`fonts`) or `/book-clubs/{club}` sends it by itself (a cover upload sends nothing). Public writes send what they changed themselves: a raffle sign-up `raffles`, a garapon draw `garapons`, a stamp or a rally sign-up `stamp-rallies` (and `garapons` when a drawing link was issued), a stamp-card or dashboard read that wrote a card's completion `stamp-rallies`, a public ema submission or artist request `ema-boards`, and the announcement scheduler `announcements` after a sweep that wrote; the public raffle entry search sends nothing. A garapon player made or deleted with its paired card, a rally delete, a map save or delete and a board delete also tell the pages they unlink; a font write also sends `settings_changed` |
| `settings_changed` | none | everyone |
| `theme_changed` | `{ styleId }`, `null` when the site went back to the default look | everyone |
| `tea_room_unlocked` | `{ id, name, roomNumber }` | Tea Rooms staff |
| `log` | `{ entry }` | admins only; lossy, never replayed |
| `resync`, `ping` | none | every stream |

A `halftime_resolved` is the first answer to the prompt; a second answer is refused with
409. A number inside its draw delay is hidden from anyone without the Game page, here
and in `GET /game` and `GET /board/{id}`.

### Staying in step

- **Heartbeat.** Every stream gets a `ping` every 25 seconds, so a quiet stream isn't
  cut by a proxy. A client that hears nothing for much longer has lost the stream and
  opens it again.
- **Resuming.** The hub keeps the last 1000 events, with ids that keep rising across
  restarts. A client that reopens sends `Last-Event-ID` (a browser's `EventSource` does
  it by itself) and gets the events after it that it may hear. An id too old, or from
  before a restart, gets `resync`: read everything again.
- **Re-read on open.** The stream carries only what changes, so a client reads the state
  it shows again every time a stream opens, and on `resync`.
- **Slow clients.** A stream that falls 64 events behind is ended, and resumes from its
  last id when it reconnects.
- **Accounts.** A staff stream checks its account on every heartbeat and right after any
  account change. An account deactivated or deleted, a password change, signing out, or
  losing every permission ends the stream; losing one permission narrows what it hears.
  A client that gets 401 or 403 on reopening stops.

## Conventions

The standards' ([`api-and-realtime.md`](shared/standards/api-and-realtime.md)), in
short:

- **Methods:** `GET` reads, `POST` creates (-> `201` with the resource) or runs a
  command, `PUT` replaces, `PATCH` changes only what it sends, `DELETE` removes
  (-> `204`).
- **Resources & items:** a collection is `/api/v1/<resource>`, an item
  `/api/v1/<resource>/{id}`; sub-collections nest. Path segments are kebab-case; JSON
  fields and query parameters are camelCase. A trailing slash answers as the path
  without it.
- **Commands:** a state change that isn't CRUD is `POST …/{id}/<verb>` (`…/close`,
  `…/pick-winner`); bulk writes are `POST` (`…/reorder`); deleting everything is
  `DELETE …/all`, answering `{ "deleted": N }`.
- **Times** are RFC 3339 in UTC, `null` for none.

The one intentional exception is signing in and out and signing up, under
`/api/v1/auth/*` (`login`, `logout`, `register`, `passkey/begin`, `passkey/finish`),
which stay small action-shaped credential calls rather than resource CRUD. A 401 from
one of them means the credentials were wrong, not that a session ended.
