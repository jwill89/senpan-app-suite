# Plan: release C, the API on huma (5.0.0)

Written 2026-11-09 against app 4.1.0 / plugin 3.9.1.0 / schema v73. Release C of the
overhaul in shared-tooling's `PLAN.md` (step 7): the backend moves to huma and the shape
the standards give an API (`shared/standards/api-and-realtime.md`), and the SPA and the
plugin move with it, all released together as 5.0.0.

## Decisions (settled 2026-11-09)

| Question | Decision |
|---|---|
| The plugin | Updated in the same release, deployed the same day: no endpoint keeps snake_case for old plugins. |
| JSON field names | camelCase everywhere: bodies, query parameters, the live feed's events, and `GET /api/tea-rooms/public`, which the Carrd pages read (their scripts change the day 5.0.0 deploys). |
| Upstream failures | The standard: 503 when Discord is unreachable, 500 for bad credentials or config, 429 when it rate-limits. Never 502. |
| Caching | The standard's baseline: signed-in responses `private, no-store`; public reads `no-cache` with an `ETag`, a repeat answered 304. |
| CORS on the tea-room API | An allowlist setting (Settings), echoed back; `*` goes. Only the owner's own domain reads it (the Carrd pages are served on it), so a migration seeds `https://senpan.cafe` and `https://*.senpan.cafe`, a `*.` entry matching any subdomain: nothing to enter before the deploy. |
| SQL | sqlc, domain by domain as each moves (decided with release B). |
| Missing times | `null` in the API (the database has held NULL since 4.1.0). |
| Base path | `/api/v1`, as Kindred Nights and Home Library have it (decided 2026-11-09, over the standard's `/api`). The font host's vhost (`fonts.senpan.cafe` -> `/api/fonts/pub/`) moves to `/api/v1/fonts/pub/` on deploy day; the kit.css address Carrd pages embed stays. `/healthz` stays where it is; `/api/ws` goes (the live feed below). The API contract's own version is the document's `info.version`, starting at 5.0.0. |
| Sign-in | Inside huma, under `/api/v1/auth/*` (the owner, 2026-11-09): the standard's one action-shaped exception, in the document and the typed client. Kindred Nights and Home Library sign in through plain handlers under `/auth/*`, which came from Kindred's Discord redirects rather than from huma; they move the same way (shared-tooling's `plans/sign-in-on-huma.md`). |
| Live feed | Server-sent events, replacing the WebSocket (the owner, 2026-11-09). The traffic is one-way: the server discards what a client sends (the SPA sends only a keepalive), and calls, claims and every other write go over REST, so the standard's channel rule (api-and-realtime.md) picks SSE. As Kindred Nights does it: huma's `sse` package, so the events are typed in the document and reach TypeScript through openapi-typescript; sign-in through the API's own layers (a cookie, or the plugin's Bearer header, so no token in a URL); the browser's `EventSource` reconnecting and resuming by `Last-Event-ID`. The draw delay players see is kept. Events name what changed and the client re-reads it over REST, as the standard has it, except a draw, which carries the called number (the owner, 2026-11-09): it goes to every player's phone at once on every call, so re-reading would set off a request from each at the same instant; the number is tiny, the server already decides when each audience gets it, and a reconnect still re-reads. Events with nothing to re-read carry what they report: a log line, a yoever reaction, who answered half-time. Shared-tooling's real-time table had Senpan on WebSockets for "calls and claims", but claims go over REST. |
| Matching the siblings | Where Kindred Nights and Home Library agree, Senpan does the same, with no exception of its own (the owner, 2026-11-09): huma's config as Home Library sets it, the document committed as `frontend/src/api/openapi.json` by `go run . openapi`, openapi-typescript into `schema.d.ts`, and an openapi-fetch `client.ts`. |

## Layout

As Kindred Nights does, the reference for huma:

- `internal/api`: the huma API, `humago` on the server's `ServeMux` under `/api/v1`, one file
  per domain registering its operations. Handlers are thin: read the input, call the
  domain or the queries, return a typed output with `Status` set.
- `internal/db`: sqlc's package, generated from `internal/db/queries/<domain>.sql` against
  the committed `backend/schema.sql`, on the store's two pools.
- Domain packages for the rules (pure code: no database, clock or network): `bingo`
  (cards, boards, draws, patterns, half-time), `raffles` (allowances, tiers, draws),
  `festival` (garapon odds, stamp-rally completion, map windows), `schedule`
  (announcement recurrence), and the rest as each domain shows a rule worth testing alone.
- `internal/server` shrinks to the HTTP shell: the site, sessions, CSRF, the access log,
  the live feed's hub, and the schedulers.

## The checklist each domain meets

- Operations registered with huma: an `operationId`, a summary, tags, and every error
  response declared as problem details; every request and response a named schema whose
  properties are all described; nullable fields marked so.
- camelCase fields and query parameters; kebab-case paths; a trailing slash accepted.
- `POST` creates and answers 201 with the resource; `PUT` replaces; `PATCH` changes only
  what it sends; `DELETE` answers 204; a command is `POST …/{id}/<verb>` and answers the
  resource or `{ "success": true }`; `DELETE …/all` answers `{ "deleted": N }`.
- Errors: 400 unparseable, 401 not signed in, 403 not allowed, 404 unknown or hidden, 409
  conflict (a conditional update lost), 422 validation (each field in `errors`), 429 with
  `Retry-After`, 503/500 for Discord. `detail` is a message staff can read.
- Caching per the baseline above.
- Queries in sqlc; rules in the domain package with their own tests; the handler tests
  check each documented 2xx response against the spec.
- The SPA's store, views and tests for the domain move to the generated types and the
  camelCase fields in the same commit, so main stays consistent; e2e stays green.
- The old routes, handlers and store methods for the domain are deleted in that commit.

## Phases

Each phase is one commit (or a few) on main, with `task check` green. Nothing deploys
until the last phase raises the version: CI tags only on a version bump.

Merged 2026-11-09, the owner leaving it to how much risk a merge adds: 4 with 5, two
small domains, and 6 with 7, which share the Discord posting (`embeds.go`) and its
move to 503/500/429. A phase's fixed cost (regenerating, the full check, the docs) is
paid once for the pair; reading each domain's code isn't saved by merging, so bingo,
the festival and media, each the size of two small phases, stay on their own. The
numbers stay as they were, so the references to them hold.

0. **Foundation** (built 2026-11-09). huma on the mux beside the old routes, under
   `/api/v1`; sign-in (session or bearer token) and permissions as huma middleware
   (401 for nobody, 403 for an account without access); problem details for every
   `/api` error, the mux's own 404 and 405 (keeping `Allow`) and a crash included;
   trailing slashes; the caching baseline (private, no-store, or no-cache with an
   ETag and 304); the document from huma (`go run . openapi`) committed as
   `frontend/src/api/openapi.json`, with `schema.d.ts` from openapi-typescript 7.13.0
   and an openapi-fetch `client.ts`, all current by CI's generated job and
   `task gen`; every 2xx answer in the server tests checked against the document
   (`internal/api/apitest`). The hand-written spec (`internal/apidoc`, kin-openapi,
   `openapi.yaml` and its `/api/docs` page) is gone; tygo stays for the routes not yet
   moved. `GET /api/v1/version` is the first operation. sqlc arrives with the first
   domain that has queries (phase 1).
1. **Site and settings** (built 2026-11-09): config, settings, logs and styles (themes)
   on huma, beside phase 0's version. sqlc's first queries (`settings.sql`,
   `styles.sql`), and the domain packages `themes`, `settings`, `permissions`,
   `bookclubs`, `discord` and `fonts`, with `logging.Read` for the log viewer. The
   settings are typed (numbers and booleans, not strings): anyone reads them, a
   `system-settings` grantee saves them with `PUT /settings`, and a save made from an
   older copy is refused 409 by its `updatedAt`. The Google Fonts key and the book-club
   webhooks are the integrations (`/settings/integrations`), an admin's alone.
   Activating a theme answers with its stylesheet. The CORS allowlist moved to phase 5,
   where it is enforced, with its seed migration.
2. **Accounts** (built 2026-11-09): signing in and out, signing up, passkeys, who is
   signed in, the password, the access token and the Users page, on huma with sqlc
   (`users.sql`, `tokens.sql`, `passkeys.sql`). Sign-in is action-shaped under
   `/auth/*` (`login`, `logout`, `register`, `passkey/begin`, `passkey/finish`); the
   account's own things are under `/me` (`GET /me` always answers, `null` for nobody;
   `PUT /me/password`; `GET`/`POST`/`DELETE /me/token`; `/me/passkeys`); an admin
   changes an account with one `PATCH /users/{id}`. The session works from inside an
   operation, since the server's session layer wraps the mux. A token can't change the
   account's credentials (403), adding a passkey asks for the password, a passkey
   challenge holds ten minutes, and every 429 carries `Retry-After`. The rules moved
   to `internal/auth` (limits, tokens, `SignIn`); `api.Document()` gives the tests the
   document as the clients have it. The plugin reads `GET /api/auth` for its account
   and moves to `GET /api/v1/me` in phase 10.
3. **Bingo** (built 2026-11-09): patterns and categories, presets, the winners log,
   cards, the game and the board on huma with sqlc, and the live feed moved from the
   WebSocket to server-sent events (huma's `sse`, as Kindred Nights' feed), its events
   typed in the document. `internal/bingo` keeps the pure card and pattern rules; the
   game engine is `internal/game` (`game.go`, `auto.go`, `types.go`, `events.go`),
   and the feed's hub is `internal/live`. Release times and the pending half-time
   prompt are kept in memory; draw delays are validated to 0 to 60 seconds; a replaced
   game's winners aren't logged; ending a game answers `{ended, logged}`;
   `halftime_resolved` carries `{minigame, by}`, and a second answer is 409;
   `GET /board/{id}?preview` gives the card alone; `GET /game` is private and hides
   numbers still inside their draw delay from anyone without the Game page.
   The feed is two streams, `GET /events` for staff (a session or a Bearer token) and
   `GET /board/{id}/events` for boards (public by card id), each event
   `{id, type, gameId, at, payload}`. The hub filters every event by viewer, keeps the
   last 1000 with ids that rise across restarts, replays by `Last-Event-ID` (`resync`
   when the id is too old), ends a stream 64 events behind so it resumes, and re-checks
   a staff account every heartbeat and after any account change. `internal/ws`,
   `server/ws.go`, the `?token=` fallback, `lib/ws.ts`, `useWebSocket.ts`, coder/websocket
   and the `/api/ws` proxy are gone; the SPA reads its state again whenever a stream
   opens. The events are built from an audit of every write against every surface that
   shows its state (each admin console, the plugin, the players' boards), so several
   staff and the plugin stay in step (the owner, 2026-11-09); what it found is fixed
   (see "Keeping every viewer in step" below). The half-time gap is closed: the prompt
   is game state, an answer tells every admin surface to close it, and a second answer
   is refused with 409.
4. **Raffles and the tea house** (built 2026-11-10, with 5 and with 6): raffles,
   affiliates and tea rooms on huma with sqlc (`raffles.sql`, `affiliates.sql`,
   `tearooms.sql`), the rules in `internal/raffles`, `internal/affiliates` and
   `internal/tearooms`. Payments are commands (`…/settle` with `{paidEntries?, waived}`,
   `…/unsettle`); gil is whole numbers; a winner pick is 409 while one is pending, and
   `pick-another` and `verify-winner` name the winner they mean (409 if another console
   changed it). `get-raffle` sends `winnerName` and `paidTotal` null: only `list-raffles`
   fills them, for the Raffles page. The public sign-up (`enter-raffle`) publishes
   `resource_changed` `raffles` itself; the public entry search (`…/lookup`, a
   case-insensitive substring) publishes nothing, where it used to broadcast by mistake.
   Affiliate, tea-room and book-club webhooks are read and set through their page's
   `/webhook` (`GET`/`PUT`, `{webhookUrl}`), so the book-club ones left the integrations,
   which hold only the Google Fonts key (same setting keys, no migration). A tea-room
   `PATCH` with `locked: true` sets the lock afresh (until `lockedUntil` if sent
   beside it, otherwise until staff unlock it). The public tea-room API
   (`GET /tea-rooms/public[/{number}]`) keeps every field in camelCase with `no-cache`
   and an ETag (a repeat is 304), and its CORS is the `teaRoomOrigins` allowlist (`internal/origins`):
   the origin echoed, never `*`, never with credentials, `Vary: Origin` always, the
   preflight answered for allowed origins; migration 74 seeds `https://senpan.cafe` and
   `https://*.senpan.cafe` (a `*.` entry covers subdomains, not the apex), and it is
   edited on System -> Settings. Schema v74. The festival code keeps compiling on the
   model types and store reads it still uses (`ListFestivalRaffles`, `GetAffiliate`).
5. Merged into 4.
6. **Announcements and book clubs** (built 2026-11-10, with 4): types, roles and
   announcements (`internal/announcements`, `internal/schedule`) and book clubs'
   reading lists (`internal/bookclubs`, `internal/anilist`) on huma with sqlc
   (`announcements.sql`, `bookclubs.sql`). The Discord transport moved to
   `internal/discord` (the `Sender` with the dry-run/override policy, typed errors
   `Ambiguous`, `Unreachable`, `RateLimited`, `Refused`, `discord.HTTPStatus`): 429 with
   `Retry-After`, 503, 500, never 502, ema's posts included; AniList fails the same way
   (never 424). The announcement scheduler stays in `server` on sqlc and sends
   `resource_changed` `announcements` once after a sweep that wrote anything.
   `AnnouncementSkip.count` is optional (1 when left out, 0-52); deleting a type or role
   in use is a 409 with the count. The book-club guard answers 401, 404 for an unknown
   club and 403 without `bookclub-<slug>`; publishing an empty list is a 409, a cancelled
   request a 408, and a failure part-way says "Posted N of M, then: …". The cover upload
   (`POST /api/bookclub/upload`) stays on its old handler until the media phase.
7. Merged into 6.
8. **Festival**: garapons, stamp rallies, sign-up, festival maps and dashboard, ema.
9. **Media**: images and categories, Carrd, fonts (the public font paths unchanged).
10. **The plugin and the release**: the deploy's smoke check asks
    `/api/v1/version` (left on `/api/version` until then, so a 4.x hotfix still
    deploys); the plugin's naming policy to camelCase (query values too: the winners log's `sort=loggedAt`, `perPage`), its account read moved to
    `GET /api/v1/me`, its models
    checked against the spec (nulls, problem details, the live feed's events), its live
    connection moved to the SSE stream with a Bearer header (Kindred's plugin reads its
    feed so, `Core/Venue.cs`): Bearer on `/api/v1/events`, `Last-Event-ID` on reconnect,
    back-off from 1 to 16 seconds, 70 seconds of silence treated as a drop, `/game` read
    again on open and on `resync`, and a stop on 401 or 403. The events are renamed
    (`game_draw` to `draw`, `game_update` to `game_started` and `game_ended`,
    `cards_update` to `cards_changed`, `yoever_config` to `yoever_switched`,
    `auto_config` to `auto_draw`), their fields sit under `payload` with the game id in
    the envelope, and tea rooms use `roomNumber`; the plugin handles the new
    `halftime_resolved`, `draw_delay` and `game_details`. Its version
    raised; tygo removed; AGENTS.md's departures for
    the package layout, errors, validation, JSON names, routes, front-end types, SQL,
    502s and caching closed; version 5.0.0 and the CHANGELOG; deploy/README.md's deploy
    day below.

## How the rest is run

Phases 0 to 3 cost far more than they needed: small edits each followed by a test run,
every agent's work re-run and re-read, the docs redone per sub-step, and one session
carrying the whole history. From phase 4 on, the work goes in batches: 4 with 6, then 8
with 9, then 10.

1. **One session per batch**, started fresh with this file as its brief, so no turn
   carries an earlier batch's history.
2. **Decisions first.** One read of the batch's old routes, handlers and SPA stores lists
   every choice the checklist above leaves open; they are asked in one round before any
   code, so nothing stops mid-batch.
3. **Opus decides, Sonnet builds.** The decision round ends in a sheet per operation
   (path, guard and permission, status codes, the live event it sends and to whom), so
   the building is transcription; Sonnet agents build phases 4, 6, 8 and 9 from it, and
   Opus reads only the parts of the diff that touch sign-in, permissions and live
   events. A domain Sonnet can't get through the checks goes back to Opus. Phase 10
   (the plugin and the release) stays on Opus.
4. **Build whole, test once.** One agent per domain group writes everything (queries,
   domain rules and tests, operations, server tests, the SPA's store and views) against a
   finished domain as its template (`api/patterns.go`, `api/presets.go`), running only
   `go build` and `vue-tsc` while it writes, then its own package tests once. It reports
   in a few lines: files, decisions it made, anything left.
5. **One full check per batch**, its output to a log and only the failures shown; the
   failures fixed together and the check run once more. Agent work is read only where a
   check fails or the diff touches sign-in or permissions.
6. **Docs once per batch**: CHANGELOG, AGENTS.md, API.md, this file and the deploy notes
   in one pass at the end.
7. **Stop for the commit**, then the next batch in a new session.

## Keeping every viewer in step

From the 2026-11-09 audit of every write against what each surface shows: the other
staff consoles, the plugin, and the players' boards. Phase 3 fixed what it lists
below; the plugin's share is phase 10's.

- **Cards** (fixed in phase 3): the card preview follows `cards_changed` and saves only
  the fields this person changed, instead of writing back a stale copy over another
  admin's edit. A preview no longer overwrites another.
- **The game** (fixed in phase 3):
  - Half-time: the pending prompt is game state (`gameId`, `autoPaused`) and reaches
    every console, including one that opens mid-way; the answer names its game; a stale
    or repeat answer is 409; every admin surface hears it was answered, and by whom
    (`halftime_resolved`).
  - Ending: the confirm names its game, so a confirm left open across a new game is
    409 rather than ending the new one with the old winners; a second confirm says it
    was already ended instead of "Game ended"; the winners log and frequent winners
    refresh on every console.
  - Starting while a game is live is 409 unless the request says to replace it; a
    replaced game's winners are not logged.
  - The public game read and the board no longer leak the numbers still inside the draw
    delay; staff with the game page see them at once, as on the live channel.
  - The game state carries what a console or board opening mid-way needs: the draw
    delay, the pending half-time, whether the mini-game alert went out, and the last
    draw with when players get it. The board gains the mini-game flag, and its modal
    closes when the game ends.
  - The winner chime sounds on every console whenever the winners grow, not only after
    your own draw.
- **The feed** (fixed in phase 3):
  - On connect and reconnect a client gets the state it shows (the SPA reads it again
    on every open, and `resync` says when to), and `Last-Event-ID` replays what was
    missed in a drop.
  - Admin events go only to those whose permissions cover them.
  - Changes that sent nothing now send an event: the draw delay saved through the
    settings (`draw_delay`), `settings_changed`, every theme write (`theme_changed`),
    and every sign-up (`resource_changed`).
- **Raffles and announcements** (fixed in phases 4 and 6): a raffle pick is refused (409)
  while a winner is pending, and picking another or confirming names the winner it
  means; the public raffle sign-up and the announcement scheduler's sweeps send
  `resource_changed`, and the public entry search no longer does.
- **Later phases**: the plugin handles the details, draw
  delay, pattern and resource events, takes the draw delay from the game state, and
  closes its half-time and end-game windows when another surface answers them (10).

## Deploy day

1. `task deploy`: the app, then the plugin (its version is new, so it publishes). The
   CORS allowlist arrives seeded with the senpan.cafe origins.
2. On the droplet, point the `fonts.senpan.cafe` vhost's ProxyPass at
   `/api/v1/fonts/pub/` (deploy/README.md, Font host), and reload Apache.
3. Update each Carrd page's tea-room script to the camelCase fields and the
   `/api/v1/tea-rooms/public` address. The pages must be served from an origin on the
   `teaRoomOrigins` allowlist (seeded with `https://senpan.cafe` and
   `https://*.senpan.cafe`), or the browser refuses the read.
4. Staff update the plugin in Dalamud; until they do, its calls fail with a clear error.
5. In the site's vhost, drop the `/api/ws` ProxyPass and give the `/api/` one
   `timeout=3600`, as Kindred's keeps its stream open, and reload Apache. The feed's
   25-second heartbeat keeps a stream alive either way; the timeout is the belt to it.

## Risks

- The plugin is only built in CI, never run against the server: its model check against
  the spec is the guard, plus a manual run before the deploy.
- The SPA touches snake_case fields in 84 source files and 35 test files; each domain's
  commit moves its share, and the type checker finds what grep misses once the types are
  generated.
- The live feed drives the boards: phase 3 changes both ends together, and e2e covers a
  draw arriving live. A stream behind Apache and Cloudflare must arrive unbuffered: the
  feed flushes each event and sends no compressible body, and Kindred's runs on the same
  stack.
