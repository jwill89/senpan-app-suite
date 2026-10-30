# Plan: replace Discord webhooks with a built-in bot

Status: all build steps done on branch `bot-replacement` (2026-10-24); ready to commit and run the rollout runbook. Local working document (gitignored).
Written 2026-10-23 against backend 3.24.1 / frontend 3.28.0 / plugin 3.9.0.0 / schema v69.

## Decisions (settled)

| Question | Decision |
|---|---|
| Post identity | **One bot identity** for every post. No personas, no bot-managed webhooks. |
| Token storage | **Entered in the UI, stored in the database**, encrypted with a key derived from the existing `APPSUITE_SESSION_SECRET`. No new setting or env var. |
| Server config | **No changes to the unit file.** Secrets stay where they are. |
| Dev bot / test server | **None available.** Verification is automated tests against fake Discord servers, plus a production **test mode** for the live check. |
| Interactions | **Out of scope** for this pass. Possible later over the gateway (natively, no HTTP endpoint needed). See "Out of scope". |
| Bot permissions | **Administrator** in the one server. Invite URL uses `permissions=8`. |
| Servers | **One:** Senpan Tea House, guild `999011226568888410`. Seeded as the default. |
| Relationship to Kindred Nights | **Separate bot application.** Kindred's code is a reference and a porting source, not a shared module. |
| Edit in place | **In this pass**, for tea rooms and affiliates (auto-update the live post when the item changes). |
| Rollout | One release (backend 4.0.0 + frontend 4.0.0), built in green steps. |

Consequences of "one identity": every post shows the bot's name and avatar. Today's
webhook personas ("Senpan Tea House", "Senpan Apps", "YaoiBot", "YuriBot", "Yuri Bot",
"Atelier YAO", "Obon Matsuri", "Test") go away. Set the bot's own name/avatar in the
Developer Portal to what staff should see (likely "Senpan Tea House").

## What exists today (inventory)

All outbound posts go through one funnel: `postDiscordWebhook` in
`backend/internal/server/embeds.go:347-406`. No post uses `?wait=true`, so **no Discord
message id is stored anywhere**.

| # | System | Trigger | Destination today | Payload | On failure | Stored |
|---|---|---|---|---|---|---|
| 1 | Announcements (manual) | `POST /api/announcements/{id}/send` (`announcements.go:447-482`) | `announcement_types.webhook_url` | content mention + 1 embed + link-button row + allowed_mentions (`buildAnnouncementMessage` 803-815) | 502 | `last_posted_at` |
| 2 | Announcements (scheduled) | `RunAnnouncementScheduler` / `postDueAnnouncements` (`announcements.go:1274-1400`) | same | same | definite error: retry next tick, give up after 5 (`maxAnnouncementPostRetries`), ambiguous (transport): advance, never retry (`errWebhookAmbiguous`) | `MarkAnnouncementPosted` cursor |
| 3 | Book club reading list | `POST /api/book-clubs/{club}/reading-lists/{id}/publish` (`bookclubs.go:879-951`) | setting `discord_webhook_url_<slug>` | one embed per item, 350 ms apart (`buildItemEmbed` 956-982) | stop at first error, 502 "Published N of M" | nothing |
| 4 | Tea rooms | `POST /api/tea-rooms/{id}/post` (`tearooms.go:362-390`) | setting `tearoom_webhook_url` (not in `settingsKeys`) | 1 embed (`buildTeaRoomEmbed` 553-579) | 502 | nothing |
| 5 | Affiliates | `POST /api/affiliates/{id}/post` (`affiliates.go:272-301`) | setting `affiliate_webhook_url` (not in `settingsKeys`) | 1 embed (`buildAffiliateEmbed(a, s.siteBaseURL(r), now)` 311-336) | 502 | nothing |

Nothing else posts: raffles, garapon, stamp rally, festival map, bingo, the plugin.

Live data (devdata copy, 2026-10-23): 16 webhooks, all reachable.

| Source | Server | Migrates automatically? |
|---|---|---|
| 10 announcement types: Announcements, Yuri/Yaoi Book Club Events, Events, Openings, TEST CHANNEL: Yao, Affiliate Openings, Affiliate Events, Obon Event Handguide, Obon Ema Plaques | Senpan Tea House | yes |
| `discord_webhook_url_yaoi`, `discord_webhook_url_yuri` | Senpan Tea House | yes |
| type "TEST CHANNEL: Math", `tearoom_webhook_url`, `affiliate_webhook_url` (all the same channel) | another server (a test server) | **no - pick a channel after deploy** |
| type "Atelier YAO Live" | another server (Atelier YAO) | **no - pick a channel or delete the type** |

Mentions: `announcements.mention` is `''` (9), `'everyone'` (4) or `'role:<announcement_roles.id>'`
(10, ids 3/4/5). The `<id>` is the internal row id; the snowflake is
`announcement_roles.role_id` (3 rows).

Dev-safety valves today: `-webhook-dry-run` / `APPSUITE_WEBHOOK_DRY_RUN=1` and
`-webhook-override` / `APPSUITE_WEBHOOK_OVERRIDE` (`main.go:49-50, 119-130`,
`Server.SetWebhookPolicy` `server.go:124-139`). They exist because `deploy.ps1` pulls a copy
of the live DB to the dev machine on every backend deploy (`scripts/deploy.ps1:640`).

## Target architecture

### Backend package `internal/discord` (ported from Kindred Nights)

Source: `C:\Web Projects\Kindred Nights\internal\discord\`. Port, then cut every
`kindred/internal/*` import.

| Port | Kindred file (lines) | Notes |
|---|---|---|
| REST client | `client.go` (248) | `Authorization: Bot`, `User-Agent: DiscordBot (https://apps.senpan.cafe, <version>)`, 10 s / 1 min clients, ctx everywhere, 1 MB body cap, errors `ErrNotFound`/`ErrForbidden`/`ErrRateLimited`/rejection with `Said()` flattening Discord's `errors` object. Error strings never contain paths or IDs. |
| Endpoint wrappers | `rest.go` (342) | Keep: `/applications/@me`, `/users/@me`, guild, guild channels, guild roles, channel get, message create/edit/delete/get, `/gateway/bot`. Drop: members, bans, threads, commands, interactions, scheduled events. |
| Gateway | `gateway.go` (452) | As-is. Lessons baked in: 16 MB read limit (READY/GUILD_CREATE overrun 32 KB), never send a close frame (keeps sessions resumable), forget the session on 4004/4007/4009-4014, 15 s handshake-only deadline, jittered heartbeat start, zombie detection, backoff 1 s to 5 min. |
| Message model | `messages.go` (159) | `Message{content, embeds, allowed_mentions, components, nonce, enforce_nonce}`. PATCH quirk: a missing list means "unchanged", an empty list means "remove" - always send `embeds`/`components` on edit. No multipart needed (images stay URLs). |
| Permissions | `permissions.go` (126) | Only `allowed()` for one status-page check: "the bot does not hold Administrator". Optional. |

Merge Senpan's embed model and builder (`embeds.go:82-297`: `discordEmbed`, `newEmbed()`,
`build()`, truncation, `linkButtonRow`, `parseEmoji`) into the package (`compose.go`), so the
feature builders (`buildAnnouncementEmbed`, `buildItemEmbed`, `buildTeaRoomEmbed`,
`buildAffiliateEmbed`) stay in `server` and return `discord.Embed`.

Additions beyond Kindred:

- **Rate limits.** Kindred waits out a 429 once if `retry_after <= 5s`. Keep that, plus a
  "patient" mode (up to 60 s, 4 tries) for batch work (reading-list publish, deletes).
  Read `retry_after` from the body; also honor `X-RateLimit-Remaining: 0` +
  `X-RateLimit-Reset-After` on message create/edit/delete so a 20-item publish paces itself
  instead of eating 429s. Per-channel message create is ~5 per 5 s.
- **Idempotent create.** Every create sends `nonce` (<= 25 chars) with `enforce_nonce: true`.
  Discord returns the existing message instead of posting twice when the same nonce repeats
  within a few minutes. This retires the "ambiguous failure, never retry" rule for the
  scheduler (see Announcements).
- **Service.** `discord.Service` owns: the client (rebuilt when the token changes), the
  gateway session (restarted when the token changes), the guild id, a 30 s cache of
  channels + roles + guild (Kindred `bot.go:27-60, 291-401` pattern, own mutex), presence,
  and the write policy (dry run / redirect). A nil/unconfigured service means "Discord off";
  every caller checks `Enabled()`.
- **Gateway use.** Intents: `GUILDS` (1<<0) only, non-privileged. Purpose: the bot shows
  online, READY gives application id + bot user, and CHANNEL_*/GUILD_ROLE_* events drop the
  picker cache early. No slash commands, no interactions, no message content in this pass.
  Posting never depends on the gateway being up.

### Configuration and secrets

- **Token.** Settings key `discord_bot_token`, value `v1:<base64(nonce|AES-256-GCM ciphertext)>`.
  - Key: **derived from the session secret** (`-secret` flag / `APPSUITE_SESSION_SECRET`,
    `main.go:76-90`) with HKDF-SHA256 (`golang.org/x/crypto/hkdf`, already a dependency via
    `x/crypto`), info string `senpan discord bot token v1`, 32 bytes. New helper
    `internal/secretbox` (`Seal`/`Open`, stdlib `crypto/aes` + `crypto/cipher`), with tests.
  - Only a **configured** secret counts. When the secret was generated at random (none set,
    `main.go:81-90`), `main` passes no key: the token can't be stored ("set
    APPSUITE_SESSION_SECRET so the token survives a restart"), since a random key would make
    it unreadable after the next restart.
  - Changing the session secret makes the stored token unreadable: the Discord page then says
    "re-enter the bot token". That is the only cost.
  - Why not the password hashing: argon2id uses a per-password random salt stored in the
    database (`internal/auth/password.go:37-42`), and hashing is one-way; the token must be
    read back, so it needs reversible encryption with a key kept outside the database.
  - **Not** in `settingsKeys` (same treatment `tearoom_webhook_url` gets today), so it can
    never be read or written through `/api/settings`. Never returned in any response. Never
    logged at any level. `PUT` validates the token with `GET /users/@me` before saving.
  - No key on the server: the token can't be saved (clear 409 message). Key present but
    the stored ciphertext won't open (dev copy, rotated key): status says "re-enter the
    token", Discord stays off. **This is the dev-safety mechanism**: devdata copies carry
    ciphertext the dev machine can't open.
- **Guild.** Settings key `discord_guild_id`, seeded `999011226568888410` by v70. Editable on
  the Discord page (validated: the bot must be in it).
- **Application id.** Settings key `discord_application_id`, **seeded by v70 with
  `1563399760994635906`** (given by the user 2026-10-23). Used for the Invite link, so it works
  before the token is entered. Once a token is set, the status check compares it with
  `GET /applications/@me`; a mismatch shows "this token belongs to a different application".
  Editable on the Discord page (admin), digits only.
- **Public key:** `dbb18b96a85259d2c21c6edea03e60655ba45a9d89562c4dfb0f6f39760cad45`. Not used
  in this pass (it is only for verifying an HTTP interactions endpoint; this plan has none, and
  any future slash commands would arrive over the gateway like Kindred's). Recorded here so it
  doesn't have to be looked up again. It is public, not a secret.
- **Not needed:** client secret (only for OAuth login).
- **Presence text.** Settings key `discord_status_text` (optional, e.g. "Watching the tea
  house"). Kindred `presence.go` `payload()` (~45 lines).
- **Public base URL.** Settings key `public_base_url`, seeded `https://apps.senpan.cafe`.
  Needed because edit-in-place runs in the background with no request to derive
  `siteBaseURL(r)` from (affiliate logo/screenshot are stored relative). Used for every
  Discord image URL from now on.
- **Dev valves (replace the webhook ones):**
  - `-discord-dry-run` / `APPSUITE_DISCORD_DRY_RUN=1`: suppress every write (create, edit,
    delete), log what would have happened. Reads (pickers, status) still work.
  - `-discord-redirect-channel <id>` / `APPSUITE_DISCORD_REDIRECT_CHANNEL`: every create goes
    to that channel; edits/deletes only touch messages the app posted in that channel.
  - Both log a WARN at startup, dry run wins, both off in production.
- **Test mode (runtime, admin-only, on the Discord page).** There is no dev bot, so the live
  check happens in production, safely:
  - Settings keys `discord_test_mode` (`'0'`/`'1'`) and `discord_test_channel_id`.
  - While on: every **create** goes to the test channel (the existing "TEST CHANNEL: Yao"
    channel in Senpan Tea House is the natural choice); edits/deletes only touch messages the
    app posted in that channel; **the announcement scheduler holds** (due announcements stay
    pending, the cursor does not advance), so nothing real is missed or posted early.
  - Turning it off: held announcements post on the next sweep (normal catch-up), and every
    system goes back to its real channel.
  - A banner shows on every admin page while it is on, so it can't be left on by accident.
  - Same funnel code path as the redirect valve (one implementation, two sources).
- **Local runs.** Dev copies of the database can't decrypt the live token (the dev machine
  doesn't have the production session secret), so Discord is simply off locally: the
  "not configured" states, the pickers' raw-ID fallback and everything up to the funnel can be
  exercised. Discord behavior itself is proven by the fake-server tests and the fake-backed
  end-to-end run (see Testing).

### Data model (schema v70)

```sql
-- Where each non-announcement system posts. One row per system.
CREATE TABLE discord_destinations (
  key         TEXT PRIMARY KEY,           -- 'bookclub:yaoi', 'bookclub:yuri', 'tea-rooms', 'affiliates'
  channel_id  TEXT NOT NULL DEFAULT '',
  mention     TEXT NOT NULL DEFAULT '',   -- '' | 'everyone' | 'role:<snowflake>'
  repost_mode TEXT NOT NULL DEFAULT 'new',-- 'new' | 'replace' | 'edit'  (edit: tea-rooms/affiliates only)
  updated_at  TEXT NOT NULL DEFAULT ''
);

-- Every message the app has posted, so it can be edited or deleted later.
CREATE TABLE discord_posts (
  id          INTEGER PRIMARY KEY,
  kind        TEXT NOT NULL,              -- 'announcement' | 'reading_list' | 'tea_room' | 'affiliate' | 'test'
  ref_id      INTEGER NOT NULL DEFAULT 0, -- announcement / reading list / tea room / affiliate id
  scope       TEXT NOT NULL DEFAULT '',   -- club slug for reading lists, else ''
  batch       TEXT NOT NULL DEFAULT '',   -- groups one publish's messages (reading list = N messages)
  seq         INTEGER NOT NULL DEFAULT 0, -- order within a batch
  channel_id  TEXT NOT NULL,
  message_id  TEXT NOT NULL,
  fingerprint TEXT NOT NULL DEFAULT '',   -- hash of the body last sent; skips no-op edits
  posted_at   TEXT NOT NULL,
  edited_at   TEXT NOT NULL DEFAULT '',
  gone_at     TEXT NOT NULL DEFAULT ''    -- we deleted it, or Discord said 404 Unknown Message
);
CREATE INDEX discord_posts_ref   ON discord_posts(kind, ref_id) WHERE gone_at = '';
CREATE INDEX discord_posts_scope ON discord_posts(kind, scope)  WHERE gone_at = '';

-- One-time import of the old webhooks (credentials are blanked as each row settles).
CREATE TABLE discord_legacy_webhooks (
  source      TEXT PRIMARY KEY,           -- 'announcement_type:<id>', 'bookclub:yaoi', 'tea-rooms', 'affiliates'
  label       TEXT NOT NULL,              -- type name / "Yaoi Book Club" / "Tea Rooms" for the report
  url         TEXT NOT NULL DEFAULT '',   -- blanked once settled
  webhook_id  TEXT NOT NULL DEFAULT '',   -- kept, so the old webhook can be deleted in Discord later
  guild_id    TEXT NOT NULL DEFAULT '',
  channel_id  TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'pending', -- pending | imported | other_server | webhook_deleted | error
  detail      TEXT NOT NULL DEFAULT '',
  settled_at  TEXT NOT NULL DEFAULT ''
);
```

Column changes:

- `announcement_types`: add `channel_id TEXT NOT NULL DEFAULT ''`. Keep `webhook_url`
  (blanked by the importer) for one release so a rollback binary still starts.
- `announcements`: add `repost_mode TEXT NOT NULL DEFAULT 'new'` (`'new' | 'replace'`).
  `mention` changes meaning to `'role:<snowflake>'`; v70 rewrites existing rows from the
  `announcement_roles` table (role row id -> `role_id`).
- `announcement_roles`: no longer read. Kept (unused) for one release.
- `settings`: seed `discord_guild_id` (`999011226568888410`), `discord_application_id`
  (`1563399760994635906`), `public_base_url` (`https://apps.senpan.cafe`); `discord_bot_token` and
  `discord_status_text` appear when set. The four webhook keys are copied into
  `discord_legacy_webhooks` and deleted.
- v71 (next release, after the cutover is proven): drop `announcement_types.webhook_url`,
  `announcement_roles`, `discord_legacy_webhooks`.

v70 itself is SQL only (offline, deterministic): create tables, seed `discord_destinations`
rows for the four keys, copy webhook URLs into `discord_legacy_webhooks` (parse `webhook_id`
from the URL), rewrite mentions, delete the four webhook settings. Network work happens in the
importer.

### Importer (first start after deploy)

Runs once per start while `discord_legacy_webhooks` has `pending`/`error` rows. Background
goroutine on the server context, does not block startup.

1. `GET https://discord.com/api/v10/webhooks/{id}/{token}` (no auth needed; verified
   2026-10-23 that it returns `guild_id` + `channel_id` for all 16).
2. `guild_id == discord_guild_id`: write `channel_id` into the destination
   (`discord_destinations.channel_id` or `announcement_types.channel_id`) **only if still
   empty**, status `imported`.
3. Other guild: `other_server` ("pick a channel"). 404: `webhook_deleted`. Network error:
   `error`, retried next start or via a Retry button.
4. Once a row is settled (anything but `error`), blank its `url` and the source column. The
   row stays for the report.

Expected result on live: 12 imported, 4 `other_server` (TEST CHANNEL: Math, Atelier YAO Live,
Tea Rooms, Affiliates).

Optional cleanup button (admin): "Remove the old webhooks from Discord" deletes each imported
webhook by id with the bot (`DELETE /webhooks/{id}`, Manage Webhooks via Administrator), only
for rows in the Senpan guild. Irreversible, so it sits behind a confirm.

### The posting funnel

Replaces `postDiscordWebhook`. Lives in `server/discordpost.go`, calls `discord.Service`.

```
s.discordPost(ctx, post{kind, refID, scope, channelID, mention, message, nonce}) (discord.Message, error)
s.discordEdit(ctx, postRow, message) error      // 404 -> mark gone, return errPostGone
s.discordDelete(ctx, postRow) error             // 404 -> success (already gone)
```

- Builds `allowed_mentions` from `mention` exactly as `announcementMention` does today
  (`announcements.go:871-889`): `everyone` -> content `@everyone` + `parse:["everyone"]`;
  `role:<snowflake>` -> content `<@&id>` + `roles:[id]`; otherwise `parse:[]`. Mentions inside
  embeds never notify, so the ping stays in `content`.
- Link buttons: send `components` directly. The `?with_components=true` webhook parameter and
  `withComponentsParam` go away.
- Records every create in `discord_posts` with a context that outlives the request
  (`context.WithoutCancel`; Kindred lesson: a browser giving up mid-post must not leave a
  message the DB doesn't know about).
- Process-wide posting mutex per (kind, ref) to stop double posts from double-clicks.
- Logs `discord post sent` (Info) with kind + record name, never the channel/message id above
  Debug.
- Failures: 403/404 channel -> definite error naming the destination; 400 -> `Said()` text
  shown to the admin; 429 handled in the client; transport -> retryable (nonce makes it safe).
  HTTP handlers still answer 502 with the reason (`writeUpstreamError`).

**Repost modes**

- `new`: post; leave earlier posts alone (today's behavior).
- `replace`: post the new message(s) **first**, then delete the previous live post(s) of the
  same item/scope. A failed post never leaves the channel empty.
- `edit`: one live post per item. Posting edits it in place; if it was deleted in Discord
  (edit returns 404) mark it gone and post a fresh one; if the destination channel changed,
  post in the new channel then delete the old message.

### Behavior per system

**Announcements**
- Type form: Name + **channel picker** (replaces the webhook URL field). Types stay as named
  destinations, since the list filters by type.
- Announcement form: ping picker = No ping / @everyone / live server roles. New checkbox
  **"Delete the previous post when this posts again"** (`repost_mode` new/replace). Edit mode is
  not offered: edits don't notify, and recurring announcements exist to notify.
- Roles manager screens and `announcement_roles` endpoints are removed.
- Scheduler (`postDueAnnouncements`):
  - not configured, or type has no channel -> leave pending (as today with no webhook);
  - nonce = `a<id>-<occurrence unix>`; a transport failure is now retried on the next tick
    (inside the nonce window) instead of advancing the cursor;
  - definite error -> retry, give up after 5 (unchanged);
  - `replace` deletes the previous post after the new one lands.
- Manual Send uses nonce `a<id>-m<unix now>`.
- `last_posted_at` unchanged; the list row gets a "view in Discord" link from the newest live post.

**Book clubs**
- Each club tab gets a **Discord** button opening a destination form (channel, optional ping,
  "Replace the previous reading list"). Gated by `bookclub-<slug>`. Removed from System ->
  Settings.
- Publish posts one message per item (keep one-per-item so long summaries fit the 6000-char
  message budget), paced by the client's rate-limit handling instead of a fixed 350 ms sleep,
  all rows sharing one `batch` id.
- `replace`: after the whole new batch lands, delete every live message of the previous batch
  for that club (any list). If the new batch fails partway, delete what it posted and keep the
  old batch (Kindred's applications rollback pattern). Deletes go one by one, patient mode; bulk
  delete is not used (it refuses messages older than 14 days).
- The optional ping goes on the first message only.

**Tea rooms** (edit in place)
- Destination form behind the existing header button (renamed **Discord**): channel, optional
  ping, mode `new` / `replace` / `edit` (default `edit`).
- Row actions: **Post** (or **Update** when live), **Take down** (delete the live post), and a
  link to the message. Row shows "Live in #channel", "Deleted in Discord", or nothing.
- Auto-update when `repost_mode = edit` and the room has a live post, triggered after:
  `PUT /api/tea-rooms/{id}`, `PATCH /api/tea-rooms/{id}` (open, discount, lock), the lock
  sweeper lifting a lock (`RunTeaRoomLockScheduler`, `tearooms.go:~430-447`), reorder (no-op).
  Room delete -> delete its live post.
- Auto-updates go through a small queue worker (below), debounced ~3 s per room so a burst of
  toggles makes one edit, and skipped when the fingerprint is unchanged. A 404 marks the post
  gone and stops auto-updates until someone posts again (never auto-reposts).
- Note the plugin's PATCH calls (open/discount/lock) trigger the same updates for free.

**Affiliates** (edit in place)
- Same as tea rooms: destination form, Post/Update/Take down, auto-update after
  `PUT /api/affiliates/{id}`, delete -> take down.

**Edit queue worker**
- `RunDiscordWorker(ctx)` started in `main.go` with its own done channel, like the three
  schedulers. Handlers enqueue `{kind, refID}`; the worker coalesces, rebuilds the embed from
  the DB at send time (so it always sends the latest state), edits, records `edited_at`.
- On shutdown: stop accepting, drain what is due within the shared 10 s budget, after the
  announcement scheduler stops and before `hub.Shutdown` / `db.Close`.

### HTTP API

New (all JSON, documented in `internal/apidoc/paths_features.go` + `schemas.go`, then
`go run ./cmd/openapi-gen`):

| Method + path | Guard | Purpose |
|---|---|---|
| `GET /api/discord/status` | admin | configured / key present / token set / token opens / online / connected_at / last_error / bot user (name, avatar) / application id / guild (id, name, icon, bot is member, bot has Administrator) / invite URL / dry run / redirect channel / import report |
| `PUT /api/discord/token` | admin | `{token}`; validate via `/users/@me`, encrypt, store, restart client + gateway |
| `DELETE /api/discord/token` | admin | forget the token, stop the gateway |
| `PUT /api/discord/config` | admin | `{guild_id, status_text, public_base_url}` |
| `POST /api/discord/test` | admin | `{channel_id}` -> post a test message (kind `test`) |
| `GET /api/discord/channels` | any posting permission | text (0) + announcement (5) channels: `{id, name, type, category, position}`, sorted uncategorized first then category position then position (Kindred `bot.go:318-328`) |
| `GET /api/discord/roles` | any posting permission | `{id, name, color, managed}`, highest first, @everyone excluded (the picker adds it as a special option) |
| `GET /api/discord/destinations/{key}` / `PUT` | per key: `bookclub:<slug>` -> `bookclub-<slug>`, `tea-rooms` -> `teahouse-tea-rooms`, `affiliates` -> `teahouse-affiliates` | channel, mention, repost_mode |
| `POST /api/discord/import/retry` | admin | re-run the importer for `error` rows |
| `DELETE /api/discord/legacy-webhooks` | admin | delete imported old webhooks in Discord, clear the report |
| `DELETE /api/tea-rooms/{id}/post`, `DELETE /api/affiliates/{id}/post` | feature perm | take down the live post |

"Any posting permission" = admin or any of `teahouse-announcements`, `teahouse-tea-rooms`,
`teahouse-affiliates`, `bookclub-*` (new guard `requireAnyDiscordPoster`, modeled on
`requireAnyBookClub`). It exposes channel names (including private ones) to those staff;
accepted.

Changed:
- `GET /api/tea-rooms`, `GET /api/affiliates`: `webhook_url` -> `destination` + per-item
  `discord_post {state: unposted|live|gone, channel_id, url, posted_at, edited_at}`.
  Keep the `affiliates`/`tea_rooms` arrays (festival-map and stamp-rally stores read
  `.affiliates`).
- `POST /api/tea-rooms/{id}/post`, `POST /api/affiliates/{id}/post`: honor `repost_mode`;
  return the post state.
- `GET/POST/PUT /api/announcement-types`: `webhook_url` -> `channel_id`.
- Announcement model: `mention` is `role:<snowflake>`; add `repost_mode`; responses carry the
  newest live post url.
- Publish response: add `batch`, `replaced` (count deleted).

Removed: `PUT /api/tea-rooms/webhook`, `PUT /api/affiliates/webhook`, the
`/api/announcement-roles` CRUD, `discord_webhook_url_<slug>` from `/api/settings` (and its
validation `settings.go:121-124`), `isDiscordWebhookURL`, `bookclubHTTPClient` (if AniList no
longer shares it - it doesn't; fix the AGENTS.md claim).

Compatibility: removing fields is breaking under the project's rule -> **backend 4.0.0 and
frontend 4.0.0** together (`lib/version.ts` major check). The plugin ignores `webhook_url`
(`ApiModels.cs:453-458`) and posts nothing: no plugin change, but run its build once against
the new API shapes.

### Frontend

- **New admin page** System -> **Discord** (`components/admin/DiscordTab.vue`, route
  `system/discord`, name `admin-system-discord`, `meta.tab: 'system-discord'`), **admin-only**
  like Users/Logs: router guard list (`router/index.ts:393-399`), sidebar entry beside Settings
  gated `auth.isAdmin` (`AdminSidebar.vue:331-386`), `AdminTab` union + `setTabFromRoute`
  (`stores/admin.ts`). Not added to `ADMIN_PERMISSIONS`.
  - Sections: Connection (status, bot card, online badge, last error), Token (write-only
    password field: Save / Replace / Remove; shows "Set" never the value), Server (guild id,
    name, "bot is not in this server" warning + **Invite** button), Presence text, Public URL,
    Test message (channel picker + Send), Import report (table of the 16 with status + Retry +
    "Remove old webhooks"), dev banner when dry run / redirect is on.
- **New store** `stores/discord.ts`: status, channels, roles (cached; refreshed on mount and by
  a Refresh button; refetched on `resource_changed: discord`), token save/remove, config save,
  test, import retry, destinations get/put.
- **New shared primitives** (`components/common/ui/`):
  - `ChannelPicker.vue`: `<select>` with `<optgroup>` per category (the `WorldPicker` pattern),
    keeps an unknown stored id as "Channel <id> (no longer in the server)", falls back to a
    digits-only text input when the list is empty (bot off), shows `#name`.
  - `RolePicker.vue`: options No ping / @everyone / `@role` (colored dot), same stale-id and
    fallback behavior. Value format `'' | 'everyone' | 'role:<snowflake>'`.
  - `DiscordDestinationForm.vue`: channel + ping + repost mode (radio or `.toggle-group`; `edit`
    offered only where the caller allows it) + Save. Used by tea rooms, affiliates, book clubs.
  - `DiscordPostStatus.vue`: "Live in #channel · View" / "Deleted in Discord" / nothing.
  - Styles as shared objects in `assets/styles/components.css` (CSS conventions: role names,
    `-` parts, `--` modifiers, `is-` state; any new SFC scoped CSS <= 120 lines; run
    `npm run lint:conventions`).
- **Changed screens**
  - `AnnouncementsTab.vue`: type form channel picker; remove the Roles screens (`screen`
    values `roles`/`role-form`, 766-837) and "Manage Roles" button; mention picker ->
    `RolePicker`; new "Delete the previous post" checkbox; row "View in Discord" link;
    empty-state copy (252-256) no longer mentions webhooks.
  - `TeaRoomsTab.vue`: `webhook` screen -> Discord destination screen; Post/Update/Take down;
    status per row; warning copy (182-186) -> "Pick a Discord channel".
  - `AffiliatesTab.vue`: same (77-131, 209-219).
  - `BookClubTab.vue`: Discord button + destination screen; publish confirm mentions
    replace; header comment (13).
  - `SettingsTab.vue`: delete the "Book Club Integrations" section (274-307).
- **Stores/types/endpoints**: `stores/announcements.ts` (types form, drop role CRUD, mention,
  repost_mode), `stores/teaRooms.ts` + `stores/affiliates.ts` (webhookUrl -> destination, post
  state, takeDown, posting guard copy), `stores/bookclub.ts` (publish result), `stores/app.ts`
  untouched; `lib/endpoints.ts` (new `discord` group, removed `setWebhook`/role endpoints);
  `types/api.ts` (`AppSettings` loses the `discord_webhook_url_${string}` index signature,
  form types, new Discord types); `lib/constants.ts` (remove `clubWebhookKey` and the
  `DEFAULT_APP_SETTINGS` webhook entries 371-373; update `BOOK_CLUBS` comment). Regenerate
  `api.generated.ts` (`npm run gen:types`).
- `composables/useWebSocket.ts`: nothing new beyond `resource_changed` keys
  (`adminMutationResource` in `server.go:1025-1045` gains `discord`).

### Testing

Backend
- Port Kindred's fakes: `fakeDiscord` (httptest REST with auth check, injectable 429/403/404,
  fake channels/roles/messages; `discord_test.go:27-433`) and `fakeGateway`
  (`gateway_test.go:22-163`: identify -> reconnect -> resume, presence before READY, clean stop).
- New tests: secretbox (round trip, wrong key, tampered ciphertext); token never appears in any
  response or log line; `PUT token` rejects an invalid token; status without key / with
  undecryptable token; destination guards per key; channel/role listing guard + sort.
- Funnel: mention -> allowed_mentions table test; components sent without query param; nonce
  present; `replace` posts first then deletes; `edit` edits, 404 -> gone -> repost on next
  Post; dry run writes nothing; redirect sends creates to one channel and refuses foreign edits.
- Rewrite the webhook tests listed below against the fake bot.
- Scheduler: transport failure is retried (not advanced) with the same nonce; definite errors
  give up after 5; replace deletes the previous post; unconfigured leaves it pending.
- Reading list: one batch, paced; replace deletes old batch; partial failure rolls back new.
- Tea rooms/affiliates: PATCH/PUT/sweeper enqueue an edit; debounce coalesces; unchanged
  fingerprint skips; delete takes down.
- Migration v70 on a v69 fixture (mentions rewritten, legacy rows created, settings removed);
  importer against a fake webhook-lookup server (same guild / other guild / 404 / network
  error; idempotent; URLs blanked).
- `internal/apidoc` route-coverage + spec-freshness tests stay green.

Tests to rewrite or delete (from the inventory):
`embeds_test.go` (`TestPostDiscordWebhookComponentsFlag`, `...LogsSuccess`, `TestWebhookPolicy`,
`TestWithComponentsParam`, `...SurfacesDiscordReason`, `TestDiscordErrorMessage`,
`TestSanitizeWebhookErrHidesToken`; keep `TestNormalizeEmbedURL`, `TestBuildEmbedEncodesImageURL`),
`embeds_internal_test.go` (keep, move with the builder), `announcements_test.go`
(`newSchedulerEnv` and the five `TestPostDueAnnouncements*`, `TestAnnouncementMention`),
`bookclubs_test.go` (`TestDiscordWebhookSettingRedactedForPublic`, `TestPublishPostsOneEmbedPerItem`,
`TestPublishUsesPerClubCommentsLabel`, `TestPublishWithoutWebhookFails`), `affiliates_test.go`
(`TestAffiliates_Webhook`, `TestAffiliates_Post`), `part5_test.go`
(`TestAffiliateListReadableByMapGranteeWithoutWebhook`), `server_test.go`
(`TestSettings_GoogleFontsKeyNotValidatedAsWebhook`, `TestSettings_WebhookStillValidated`),
store `announcements_test.go` role tests.

Frontend
- `stores/discord.test.ts`; `ChannelPicker.test.ts` / `RolePicker.test.ts` (grouping, stale id
  kept, fallback input, value format); `DiscordDestinationForm.test.ts`.
- Rewrite: `stores/affiliates.test.ts` (webhook mocks 7/13/24, tests 60-68, 209-235),
  `stores/teaRooms.test.ts` (7/21/33, 90-98, 256-274, 298-306), `stores/announcements.test.ts`
  (mention `role:4` 135-140 -> snowflake), `lib/endpoints.test.ts` (types/roles 588-623),
  `AdminSidebar.test.ts` (System items), `stores/admin.test.ts` (tab table 81-100).

End to end (no dev bot exists)
- **Fake-backed end to end, locally.** Run the real binary with its Discord base URL pointed at
  a local fake (`-discord-api-url`, accepted only for `http://localhost:*`; production always
  uses `https://discord.com/api/v10`). The fake is the ported `fakeDiscord` wrapped as a small
  `cmd/fakediscord` program: canned guild, channels and roles, accepts any token, records and
  prints every create/edit/delete, and can be told to fail item N with a 500 or answer 404 for a
  message. Exercise through the real UI: paste a token, status, pickers, test message,
  announcement manual send + scheduled + replace, reading list publish + replace + forced
  failure rollback, tea room edit-in-place via toggles and the lock sweeper, affiliate edit,
  take down, a message "deleted in Discord" (gone state), token removal, test mode holding the
  scheduler.
- **Migration rehearsal:** run the new binary against a fresh copy of the live DB with
  `-discord-dry-run`; confirm 12 imported / 4 other_server and blanked URLs. The importer's
  webhook lookups need no bot token, so this works without one.
- **Live check** in production under test mode (see the runbook).

### Documentation

- `AGENTS.md`: overview (12-14, 34), file map (74, 124, 126, 132), server package (163), the
  Discord embeds + dev valves paragraph (181, also fix the shared-client claim), scheduler
  (183, 783), logging (187), tables (241, 249-251 + the three new tables, schema v70), tabs
  (394, 401-403 + Discord page), conventions (1079-1085, 1105), "New Discord-embed feature"
  (1122-1123), dev command (662-667). Add a **Discord bot** section: token handling, dev
  safety, repost modes, edit queue, importer.
- `README.md` 4, 20, 50-54 (dev flags).
- `deploy/README.md`: credential rotation (148-174) gains bot-token rotation, and notes that
  changing `APPSUITE_SESSION_SECRET` now also means re-entering the bot token; how to invite
  the bot.
- `deploy/senpan.service` template: one comment next to the session secret saying it also
  protects the stored bot token. No structural change.
- `scripts/deploy.ps1:640` comment: dev copies now carry an encrypted token and channel ids,
  no webhook URLs.
- Stale comments: `server/scheduler.go:11`, `server/embeds.go:27`, `model/announcements.go:133-134`,
  `utilities.css:671`, `router/index.ts:96-97, 113-114`.
- `CHANGELOG.md`: Backend 4.0.0, Frontend 4.0.0 entries (breaking: API fields removed; posts
  now come from the bot; old webhook personas retired).

## Build order (one release, each step green)

1. **Core package.** `internal/discord` port (client, rest subset, gateway, messages, presence,
   `HasAdministrator`) + the exported fake `internal/discord/discordtest` + tests. No behavior
   change yet. **Done 2026-10-23** on branch `bot-replacement`. Deviation: the embed builder
   (`server/embeds.go`) is NOT moved yet - it moves in step 5 together with the funnel, so the
   feature builders are renamed once instead of twice. `discord.Embed`/`discord.Component`
   already match its JSON.
2. **Secrets + Discord page.** `internal/secretbox` with the session-secret-derived key,
   `discord.Service` wiring in `main.go` (start/stop with done channel), status/token/config/test
   endpoints, `DiscordTab.vue` + store, dev valves (`-discord-dry-run`, `-discord-redirect-channel`,
   `-discord-api-url`) alongside the webhook ones for now, `cmd/fakediscord`, and **test mode**
   (settings + banner + funnel redirect + scheduler hold).
   **Done 2026-10-24.** Also pulled forward from step 3: `GET /api/discord/channels|roles`
   (guard `requireAnyDiscordPoster`), `ChannelPicker.vue`, `RolePicker.vue` (shared
   `.discord-picker` object), plus a shared `.avatar` object. Notes for later steps:
   - Defaults (guild, application id, public base URL) live in code (`discordDefaults` in
     `server/discord.go`), so v70 does NOT need to seed them.
   - Test mode is stored, validated and shown (Discord page + banner on every admin page for
     admins), but the redirect and the scheduler hold are wired in **step 5** with the funnel.
   - Status is admin-only and answers non-admins with 401, which the SPA treats as an expired
     session - never fetch it for non-admins (AdminView and `admin.refreshResource('discord')`
     both check `auth.isAdmin`).
   - `DiscordDestinationForm` and `DiscordPostStatus` remain for step 5 (they need the
     destination API from step 4).
3. **Directory + pickers.** channels/roles endpoints + guard, `ChannelPicker`, `RolePicker`,
   `DiscordDestinationForm`, `DiscordPostStatus`. **Folded into steps 2 and 5**: the endpoints
   and both pickers shipped in step 2; the two forms need the funnel and come with step 5.
4. **Schema v70 + importer + report.** Tables, column changes, mention rewrite, legacy copy,
   importer goroutine, report on the Discord page.
   **Done 2026-10-24.** v70 creates `discord_destinations`, `discord_posts` and
   `discord_legacy_webhooks`, adds `announcement_types.channel_id` + `announcements.repost_mode`,
   copies every webhook URL (type URLs, `discord_webhook_url_<slug>`, `tearoom_webhook_url`,
   `affiliate_webhook_url`) into the legacy table, then blanks/deletes the originals.
   `RunDiscordImport` (goroutine in `main.go`) resolves each URL with no bot token and assigns
   the channel only where none is set yet. Report + Try again + Remove old webhooks live on the
   Discord page (`GET /api/discord/import`, `POST .../import/retry`,
   `DELETE /api/discord/legacy-webhooks`). Verified end to end against `cmd/fakediscord`, which
   gained `POST /_fake/webhook` for registering old webhooks. Notes for later steps:
   - The **mention rewrite** (`role:<rowid>` -> `role:<snowflake>`) is NOT in v70 yet. It goes
     into v70 itself in step 5, together with the RolePicker and the roles-CRUD removal. v70 is
     unreleased, so it is still free to change.
   - A settled row's URL is blanked; only `error` rows keep theirs, so Try again can re-check
     them.
   - Remove old webhooks deletes only `imported` rows (same guild) and is skipped entirely under
     `-discord-dry-run`, which still clears the report. That is fine for dev, but it means a dry
     run never deletes anything in Discord.
   - The rows say "Set it at Senpan Tea House -> X -> Discord". Step 5 must create exactly
     those destination sections, or the wording has to change with it.
   - **Rollback needs the DB snapshot**, not just the old binary: `ensureSchema` refuses a
     schema newer than the binary, so 3.x will not start on a v70 database.
5. **Funnel + systems**, simplest first, each with its tests and UI:
   tea rooms (post + edit queue worker + auto-update) -> affiliates -> book clubs (batch +
   replace + rollback) -> announcements (manual, scheduler with nonce, replace, role picker,
   remove roles CRUD).
   **Done 2026-10-24.** Verified against `cmd/fakediscord` in the browser (tea room post ->
   "Live in #tea-rooms", Close -> the post edited itself ~3 s later; destination forms for tea
   rooms and book clubs; announcement type channel picker, role picker, replace checkbox).
   What was built, and decisions made on the way:
   - Embed builder moved to `discord/compose.go` (exported: `NewEmbed()...Build()`,
     `LinkButtons`, `ParseEmoji`, `ColorFromHex`, `NormalizeURL`, `Truncate`, `NoHeading`,
     `FieldValueMax`). Feature builders return `discord.Embed`.
   - Funnel `server/discordpost.go`: `discordCreate/Edit/Delete`, the policy (dry run >
     redirect flag > test mode), `discordMention` (pings switched OFF when redirected to the
     test channel - text kept), per-item mutex, fingerprints, post states, destinations API.
     Edit queue + post/take-down for tea rooms and affiliates in `server/discordqueue.go`;
     `RunDiscordWorker` started in `main.go`, drained on shutdown before the gateway stops.
   - **Nonces only on scheduled announcements** (`a<id36>-<occurrence unix36>`). A manual
     post has none: Discord answers a repeated nonce for minutes, and after a member deletes
     the message it hands the deleted message back. Manual double-clicks are serialized by
     the item lock (and the UI disables the button). `discordCreate` also recognizes a
     repeat (Discord returned a message the item already has) and never records it twice,
     and every "previous posts" list excludes the message just posted.
   - An explicit Post in edit mode FORCES the edit (that is how a message deleted in Discord
     is noticed); automatic edits skip when the fingerprint is unchanged.
   - A message the app deletes is forgotten (row removed); a `gone_at` row now only ever
     means "deleted in Discord". No schema change was needed for that.
   - Default repost mode (`model.DefaultRepostMode`): tea rooms + affiliates `edit`, book
     clubs `new`. The importer creates destinations with the same default.
   - Deleting a tea room/affiliate takes its post down; deleting an announcement forgets
     its posts but leaves them up (they are channel history).
   - v70 now also rewrites `role:<row id>` mentions to `role:<snowflake>` (inside the same
     transaction, rerun-safe by a length guard).
   - Pulled forward from step 6, because unused code fails golangci-lint: `embeds.go`
     transport, `-webhook-*` flags + `SetWebhookPolicy`, `PUT .../webhook` routes, per-club
     webhook settings + their validation, `isDiscordWebhookURL`, `bookclubHTTPClient`, the
     roles CRUD (server, store, model, frontend), Settings book-club webhook fields, all
     frontend webhook state. README/AGENTS dev commands switched to `-discord-dry-run` so
     they still run (the full doc pass is step 7).
6. **Remove webhooks.** `postDiscordWebhook`, `isDiscordWebhookURL`, webhook settings keys,
   `PUT .../webhook` routes, `-webhook-*` flags + `SetWebhookPolicy`, Settings book-club section,
   frontend webhook state.
   **Mostly done in step 5.** What remains is a final sweep for leftovers. Deliberately
   kept until v71: `announcement_types.webhook_url` column, `announcement_roles` table,
   `discord_legacy_webhooks` (the import report).
7. **Docs, OpenAPI, generated types, versions** (backend 4.0.0, frontend 4.0.0), CHANGELOG,
   full local gate (`scripts/check.ps1`: golangci-lint, vet, tests, frontend lint/typecheck/
   conventions/tests/build, plugin build), migration rehearsal, fake-backed end to end.
   **Done 2026-10-24.** AGENTS.md (new **Discord bot** section + every webhook-era
   paragraph, table, tab and convention), README.md, deploy/README.md (bot setup, token
   rotation, the session secret now also seals the token, rollback needs the snapshot),
   the unit-file comment, the deploy.ps1 comment, versions 4.0.0 + openapi regenerated,
   CHANGELOG entries for both. Plugin: comment-only change, no version bump. The full
   `scripts/check.ps1` gate passed (plugin 0/0 warnings, govulncheck clean). Migration
   rehearsal on a fresh copy of the live DB (v69 -> v70, `-discord-dry-run`): **12
   imported, 4 other_server, 0 errors** - exactly as predicted; every URL blanked, 10 of 12
   types got their channel (TEST CHANNEL: Math and Atelier YAO Live did not), both book
   clubs got theirs, Tea Rooms/Affiliates wait for a pick, 10 role pings rewritten to
   snowflakes. Nothing committed; the user commits.

## Rollout runbook

1. Nothing to do on the droplet. The unit already sets `APPSUITE_SESSION_SECRET`, which the
   token encryption reuses.
2. Deploy backend (`deploy.ps1 -Target backend`; it snapshots the DB first). v70 runs; the
   importer settles the 16 webhooks in the background.
3. Deploy frontend.
   From the deploy until the token is entered, Discord is "not configured": scheduled
   announcements stay pending and catch up afterwards. Do the cutover in a quiet window.
4. System -> Discord: turn **test mode on** first, with "TEST CHANNEL: Yao" as the test channel.
5. Paste the token. Confirm the bot card and "online". If the bot isn't in Senpan Tea House
   yet, use **Invite** (`permissions=8`, guild preselected). The Invite link works even before
   the token is pasted, since the application ID is pre-populated.
6. Import report: expect 12 imported, 4 to fix. Pick channels for **Tea Rooms** and
   **Affiliates** (they were posting to a test server), and decide **TEST CHANNEL: Math** and
   **Atelier YAO Live** (new channel or delete the type).
7. Under test mode, everything lands in the test channel: send a test message, Send one
   announcement, publish a reading list (with replace), post a tea room in edit mode and toggle
   it open/closed to watch the post update, post an affiliate, take one down.
8. Turn **test mode off**. Held announcements go out on the next sweep; watch System -> Logs
   for `discord post sent`. Posts now show the bot's name and avatar.
9. When satisfied: "Remove old webhooks" on the Discord page (or delete them by hand in
   Discord).
10. Next release: v71 drops the legacy columns and tables.

Rollback: `deploy.ps1` keeps the previous binary (`/opt/senpan/app-suite.old`) and a DB
snapshot from before the migration (`database.sqlite.pre-<timestamp>`). Restore both and the
webhooks work again (the snapshot still holds the URLs). Messages the bot posted in between
stay in Discord.

## Risks and mitigations

1. **Visible identity change.** All posts come from the bot. Accepted; set the bot's name and
   avatar before cutover.
2. **Administrator bot.** A compromised app or leaked token can do anything in the server.
   Mitigations: token encrypted at rest with a key derived from the session secret (never in
   the database), admin-only page, never returned
   or logged, validated on save; 2FA on the Discord developer account; reset the token in the
   Developer Portal on any suspicion (the page's Remove + paste flow makes that a minute).
3. **Dev copies.** The dev machine doesn't have the production session secret, so a copied
   token can't be decrypted and Discord stays off locally. Dry-run and redirect valves remain
   as a second line.
3a. **No dev bot.** Covered by fake-server tests, a fake-backed local end-to-end run, and the
   production test mode that redirects posts to one channel and holds the scheduler.
4. **Duplicate posts.** Nonce + `enforce_nonce` on every create; posting mutex per item;
   `WithoutCancel` when recording.
5. **Rate limits.** Header-aware pacing, patient mode for batches and deletes, debounced edits.
6. **Things deleted in Discord.** Messages: 404 -> gone, surfaced in the UI, never
   auto-reposted. Channels: pickers keep the stale id and flag it; posts fail with a named
   error; scheduled posts stay pending. Roles: post goes out without the ping (as today).
7. **Gateway trouble.** Ported with Kindred's fixes; posting doesn't depend on it; status page
   shows `last_error`.
8. **Background edits need absolute image URLs.** `public_base_url` setting.
9. **Session secret changes.** Only the stored token becomes unreadable; the page says
   "re-enter the token". (Sessions are logged out by that change anyway.)
10. **Four destinations can't migrate.** Expected and listed in the report.
11. **Forum channels** are not offered by the pickers in this pass (none are used today).

## Out of scope (possible follow-ups)

- **Interactions** (decided 2026-10-23: later, not now). A bot can send interactive components
  that webhooks never could - action buttons (`custom_id`), select menus, modals - plus slash
  commands. Clicks can arrive **natively over the gateway** as `INTERACTION_CREATE` (Kindred's
  `interactions.go` path is portable: dispatch off the gateway goroutine, reply via
  `POST /interactions/{id}/{token}/callback` within 3 s, `default_member_permissions` on
  commands). HTTP delivery (an Interactions Endpoint URL, Ed25519 signature check with the
  public key above, a Cloudflare WAF skip for that path) is the alternative; the two are
  mutually exclusive. The gateway built in this pass is the foundation either way.
  Ideas discussed: "Enter raffle" button -> modal (character, world, entries) -> ephemeral cost
  reply; "Get my stamp card" ephemeral link; "I'm going" RSVP on event announcements; "Request
  this room" on tea-room posts alerting staff.
- Other bot-only abilities not used yet: native Discord scheduled events from dated
  announcements, a discussion thread per post, starter reactions, pins.
- Note: embeds and the newer layout components are the same for webhooks and bots; the gain is
  interactivity, not richer embeds.
- Auto-publishing in announcement channels (`/crosspost`, 10 per hour per channel).
- Edit in place for announcements and reading lists.
- Posting from raffles, garapon, stamp rally or festival maps.
- Plugin "post to Discord" buttons.
