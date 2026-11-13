# Changelog

All notable changes to the **Senpan App Suite** are recorded here, in the format of [Keep a Changelog](https://keepachangelog.com/). Both the app and the plugin follow [Semantic Versioning](https://semver.org/).

Since 4.0.0 the app (the Vue SPA and the Go API, built into one binary) has one version, and the plugin (SenpanCompanion, a Dalamud/FFXIV plugin) keeps its own, as Dalamud needs; each has its section below, **App** and **Plugin**. Before 4.0.0 the frontend and the backend were versioned and deployed apart; their sections, **Frontend** and **Backend**, stay at the end as the history before it.

The app's major version rises for a breaking change to the JSON API or the live event streams the plugin relies on, its minor version for backward-compatible additions, and its patch version for fixes.

---

## [Unreleased]

---

## App

The SPA and the API, one binary, one version (since 4.0.0).

### [5.4.0] - 2026-11-13

Needs a one-time step in the Discord Developer Portal (docs/ops.md, Upgrading to 5.4.0).

#### Added

- Sign in with Discord, from the sign-in page, for an account linked to a Discord account. Nobody signs up by signing in with Discord.
- On the Users page, an admin makes a one-time Discord sign-up link: whoever opens it gets an active account named after their Discord username, with no pages until an admin grants them. The link works for a week.
- On the Users page, an admin makes a one-time link that links a Discord account to an existing account and signs its owner in, or unlinks one. The link works for a day; only the admin account links its own.
- The Users page shows the Discord account each account signs in with, and an admin can rename an account.
- An account made with Discord sets its first password, or adds a passkey, without being asked for a current password. Unlinking Discord or removing a passkey is refused when it would leave the account no way to sign in.

### [5.3.0] - 2026-11-11

Needs a one-time step at deploy (docs/ops.md, Upgrading to 5.3.0).

#### Changed

- The deploy installs the service's systemd unit and the Apache vhost from templates in the repo, so the host runs what the repo says, and disables any other enabled Apache site serving the vhost's names. The unit is no longer edited on the host.
- The service reads its secrets, the Turnstile secret and the session secret, from a root-only env file instead of `Environment=` lines in its unit. The deploy stops before building anything if that file is missing or readable by others, or if the unit still passes `-webroot`.
- The deploy needs a `ServiceUser` setting (the account the service runs as) and takes an optional `AppPort`, `EnvFile` and `HostKey`; `RemoteDbPath` is gone.
- The app listens on the loopback address only.

#### Security

- The server is built with Go 1.27.4, which fixes a way it could crash while reading a request.

### [5.2.0] - 2026-11-10

Needs a one-time step at deploy (docs/ops.md, Upgrading to 5.2.0).

#### Changed

- The app serves its own pictures (`/images/`), Carrd sites (`/carrd/`) and plugin repository (`/plugin/`) from a data folder beside its database; every address stays the same, `fonts.senpan.cafe/kit.css` and its font links included.
- Pictures and Carrd files are cached for a day, so a replaced picture can show the old one until then unless it is purged at Cloudflare.
- Fonts are reachable only through `kit.css` and its tokenized links, gated by each font's allowed sites, never as plain files.
- The `-webroot` flag is now `-data`. A unit still passing `-webroot` won't start.
- The deploy installs the Apache vhost itself and needs a `CertName` setting (the certificate the vhost uses) in place of a web root setting.

#### Fixed

- The game sounds play again. Since 4.0.0 a release carried none of the four sound effects, so the "game" sound mode and the Yoever reaction were silent; the app now serves `/sounds/*.mp3` from the data folder's `sounds/`.

### [5.1.0] - 2026-11-10

#### Changed

- Ema Requests is no longer a menu link: a plaque board's artist requests are a table on its own page, with Post Plaque, Complete and Reopen on each row. The Ema Plaques link counts plaques awaiting review and requests awaiting the artist together.
- `festival-ema` is now the one permission for all of Ema Plaques. An account that held `festival-ema-requests` holds it in its place, and so sees the boards' setup, webhooks included.
- `GET /api/v1/ema-requests` is gone; a board's requests are among its plaques (`GET /api/v1/ema-boards/{id}/plaques`, `kind` `artist`).

### [5.0.1] - 2026-11-10

#### Changed

- No user-visible changes.

### [5.0.0] - 2026-11-10

#### Changed

- The whole API moves under `/api/v1` with camelCase fields everywhere, described by an OpenAPI document. It needs plugin 4.0.0.0; an older plugin can't reach this server.
- Every error is problem details (`application/problem+json`) with a readable message and, for a failed validation, each field named. A rate-limited request says when to try again, and a failed Discord post answers 429, 503 or 500 instead of 502.
- The live feed is server-sent events (`GET /api/v1/events` for staff, `GET /api/v1/board/{id}/events` for a board) in place of the WebSocket. Staff hear only what their pages cover, and a number still inside its draw delay is hidden from anyone without the Game page.
- Signing in and out and signing up are under `/api/v1/auth/`. A personal access token can't make or revoke tokens, change the password or touch passkeys, and adding a passkey asks for the account's password.
- Saving settings is refused with a 409 when the copy saved from is older than what is stored, so a page left open can't undo a later save. The Google Fonts key is an admin-only integration, and the Discord webhooks of affiliates, tea rooms and each book club are set on their own pages.
- The public tea-room API at `/api/v1/tea-rooms/public` answers with camelCase fields (`{ "teaRooms": [...] }`) and, cross-origin, only to the sites on the new "Allowed sites for the public tea-room API" setting (System -> Settings). The Carrd pages' scripts need the new fields and address.
- Image uploads are checked on their bytes: JPEG and PNG are re-encoded, which drops metadata such as EXIF and GPS; GIF and WebP are kept as sent; an image over 40 megapixels or 16384 pixels a side is refused.
- A raffle sign-up is refused when the raffle is closed, outside its window, details-only or out of allowance, and an allowance over 100 is refused. Picking another winner or confirming one names the winner it means, so two consoles can't pick over each other.
- Other actions refuse with a reason: drawing from a closed or empty garapon, stamping a stamp already collected or at a closed or paused stall, publishing a map with no base image, deleting an announcement type or role still in use, and publishing an empty reading list.

#### Removed

- The WebSocket (`/api/ws`), the hand-written API spec at `/api/docs` and the old `/api/fonts/pub/` font address; `fonts.senpan.cafe`'s own addresses are unchanged.

#### Fixed

- Half-time reaches every console: answering it closes the prompt on all of them and in the plugin, and a console that opens mid-game gets the pending prompt.
- Ending a game from an out-of-date view no longer ends a newer game with the old one's winners, and starting a game while one is live is refused unless the request says `replace`.
- Admin pages stay in step when another admin changes patterns, categories, presets, the winners log, a map, the draw delay or an announcement, and a stamp card's completion reaches the Stamp Rally page. The card preview saves only the fields its editor changed.
- Renaming an image category no longer moves its folder, which broke every stored picture path under it.
- A right stamp password no longer clears the wrong guesses before it; ten wrong passwords in ten minutes from one address hold back even a right one.

### [4.1.0] - 2026-11-09

#### Changed

- Sign-ins are kept in the database, so a deploy or restart no longer signs everyone out.
- Every time the API sends is RFC 3339 in UTC to the second (`2026-11-09T21:30:00Z`), and a raffle or stamp-rally time that isn't in that form is refused with a 400.
- A database that 4.0.0 has not yet brought up to date is refused at start.

#### Fixed

- The admin's access-token and passkey dialogs show their times correctly.

### [4.0.0] - 2026-11-09

#### Changed

- The frontend (3.32.0) and the backend (3.28.0) give way to one app version, 4.0.0, for the site and the API together. The plugin keeps its own version.
- The Go binary serves the site itself, with the same cache rules and security headers Apache set; Apache serves only the web root's own folders (`images/`, `carrd/`, `plugin/`) and proxies the rest to the app.
- `GET /healthz` answers with the version running and the schema's (503 when the database doesn't answer), for the deploy and an uptime check.
- The admin footer shows the app's version and the plugin's, and says when the server runs a different version from the open tab.
- The deploy ships the newest release, skips one the host already runs (`-Force` deploys it again), and takes `-Target app` or `-Target plugin`; `task deploy:frontend` and `-SkipBuild` are gone.

---

## Plugin

**SenpanCompanion**, a Dalamud/FFXIV plugin ("Senpan Admin Companion") that lets Tea House staff drive app services from in-game. It signs in with a personal access token and is distributed through a Dalamud custom repository. Its versions are the four-part AssemblyVersion.

### [4.0.0.0] - 2026-11-10

#### Changed

- Needs app 5.0.0: the plugin moves to the API on `/api/v1`, so an older plugin can't reach a 5.0.0 server and this one can't reach an older server.
- A refused action shows the server's own reason, and a server that can't be reached says what to do.
- Live updates come over the staff event stream instead of the WebSocket, reopening on its own after a drop and reading the game and the cards again each time it opens. A token or account the server refuses stops it until the settings change, and the status bar says so.
- Every console stays in step: a half-time prompt answered elsewhere closes here with a line in your chat log saying who answered, the End Game confirm closes when the game ends or another starts, and game details edited elsewhere arrive without replacing what you are typing.
- The draw delay is the one every console shares, so the plugin's own delay setting is gone.
- Raffles, Garapon, Stamp Rally, Tea Rooms and the Winners log read again when their records change anywhere. A raffle's winner is the server's: a winner picked on any console shows here, and confirming or drawing another names that winner.
- A tea-room lock with no unlock time lasts until someone unlocks the room.
- The server address must be `https://`, or plain `http://` for a server on this PC, so the token is never sent in the clear.

### [3.9.1.0] - 2026-10-29

#### Fixed

- A tell or timed-macro message still waiting to be sent when the plugin unloads is dropped instead of reaching chat after the plugin has gone.

### [3.9.0.0] - 2026-10-17

#### Changed

- An expiring tea-room lock raises a "Tea room unlocked" notification naming the room, which stays on screen until you dismiss it instead of fading like the game toast. The chime is unchanged, the chat line carries the same sentence, and the website shows the same alert.
- Only staff who can manage tea rooms are alerted; bingo-only staff, who have no way to extend a lock, no longer get it.

### [3.8.0.0] - 2026-09-05

#### Added

- Lock a room from the Tea Rooms page, with an unlock time: the board gains a Lock column, and locking asks when the room should free up (a local date and time, or +30m / +1h / +2h / +4h). Left blank, the room stays locked until someone unlocks it.
- When a lock expires you get a two-note chime, a toast and a line in your chat log naming the room.

#### Fixed

- The room list corrects itself after a dropped connection, instead of claiming a room is still locked when its lock expired while the connection was down.

### [3.7.0.0] - 2026-08-28

#### Changed

- A drawing link or stamp card issued from the plugin now carries the home world, taken straight from the game when the name came from the Nearby... picker. Typing a name by hand still works: the server splits a "Name @ World", and the hint now says so.
- Player and participant lists show "Name @ World", as the rest of the suite does.

#### Fixed

- A stamp card issued alongside a drawing link is no longer the same secret. They used to share one token, so the shareable card link spent the player's draws; each now has its own.

### [3.6.0.0] - 2026-08-26

#### Fixed

- After a failed load (an invalid server URL, for example) the card list and the account could stay stuck, never refreshing again and with no error shown.
- The winner chime plays reliably.
- A half-time prompt raised near the end of a game no longer pops up during the next one, and one that arrives while the Game page isn't on screen chimes and prints a line in your chat log telling you to open the window.
- The Draws box starts at the garapon's configured default instead of always 1.
- Picking a raffle while another load was in flight no longer leaves the old raffle on screen, so Add entrant and Pick a winner apply to the raffle shown.
- Typing in the auto-tell message no longer saves the settings on every keystroke.
- The Bingo Game page and the cards reload when the live connection returns, so a draw missed during a drop no longer leaves the called-numbers grid short or ends the game with stale winners.
- Clicking Draw Number, the garapon and stamp-rally pickers or Save & Test Connection no longer garbles the rest of the window for a frame.

### [3.5.0.0] - 2026-08-24

#### Added

- A Type column on the Stalls table and the collected-stamp log, reading Food or Game; a stall the server sends without a type reads as Food.
- What finishes a card shows on the rally header (every stamp, or the required food and game counts), and the stall summary splits into food and game.

#### Changed

### [3.4.1.0] - 2026-08-23

#### Added

- A payment dialog on the Paid cell replaces the paid checkbox, with the website's settlement: how many tickets the payment covers and the gil waived. The cell reads Paid / Partial / Unpaid, and Clear payment is there too.
- Timed macros: Test send puts the message out once so it can be checked in chat. It doesn't count towards the send total or start the timer.
- Timed macros: Reset is offered whenever there is progress to clear, not only once a send cap is reached, and is refused while the macro is running.

#### Changed

- The macro start button reads Start rather than Send, now that Test send sits next to it.

#### Fixed

- A details-only raffle no longer offers to sign anyone up: the Add entrant form is replaced by a note that players enter it outside the app, and the draw's caption says it runs over every entrant rather than paid tickets. Entrants for one are still recorded on the website.

### [3.4.0.0] - 2026-08-21

#### Added

- The raffle header names the pricing per entry type: the ladder for a custom-cost raffle (`Cost per entry: 50000 / 100000 / 150000`), the flat price for a single-cost one, and "Details only - players sign up elsewhere" for a raffle that takes no sign-ups through the app.
- Part-paid entrants read as `settled / total` tickets with a tooltip, and hovering the Paid box reports what has been waived on that entry. Ticking the box settles the rest, waiving nothing; entering an amount waived stays on the website.

### [3.3.2.0] - 2026-07-28

#### Changed

- Status and table glyphs are icons now: the live/offline badge, the garapon ball swatch, the stamp-rally completion mark and the roll stars were drawn as plain text, which the game's font renders inconsistently.
- On-screen text uses plain ASCII punctuation: a hyphen for an empty table cell and three dots for progress labels such as "Loading...".

### [3.3.1.0] - 2026-07-24

#### Added

- The Tea Rooms page gains an Owner column beside the room name, showing the character who owns each room.

#### Changed

#### Fixed

- Accented characters now render. Text anywhere in the window (for example a room name containing an ō) used to drop any glyph the game's font doesn't include; macron romanizations (ō, ā, ē, ī, ū) now display, and every other glyph is unchanged.

### [3.3.0.0] - 2026-07-24

#### Added

- A Tea House -> Tea Rooms page lists every room by number and name (with its per-half-hour cost) and lets staff toggle each room's Open and Discount (50% off) status with a click. It needs the Tea Rooms permission, and everything else about a room stays on the website.

### [3.2.1.0] - 2026-07-21

#### Fixed

- Every outgoing chat message, auto-tells and Timed Text Macros alike, goes through one send queue that spaces messages a second apart, so concurrent features can no longer defeat the game's chat throttle.
- A rejected token (401 or 403) no longer makes the plugin retry every few seconds forever: it backs off until the token is changed, while transient drops back off progressively and reset on a clean reconnect.
- A failed initial card load retries after a short cooldown instead of leaving the card list empty until restart.
- The outgoing-message length limit counts bytes, as the game does, so a message with multibyte characters is no longer wrongly truncated or sent over budget, and never splits a character.
- Timed-macro progress saves in the background, so the file write no longer stalls the window.

#### Security

- The personal access token is encrypted at rest with Windows DPAPI instead of stored as plain text in the config, so a synced, backed-up or world-readable config no longer exposes a usable token. An existing plain-text token is migrated once on load and cleared from disk.
- The log warns once when the configured server URL is not HTTPS, since the token would be sent in cleartext.
- A hostile or broken server can no longer exhaust memory by streaming an unbounded live message; each is capped at 1 MB.

### [3.2.0.0] - 2026-07-20

#### Added

- Auto-draw controls: the New Game view gains an Auto-draw numbers checkbox and a Time Between Calls combo (a preset pre-fills them), and the live game view gains an Auto-Draw on/off checkbox and interval combo kept in sync with the website.

#### Changed

- The half-time prompt is server-driven: the plugin opens the mini-game prompt when the server announces it, for manual and automatic draws alike. Yes alerts players and leaves auto paused, No declines and resumes auto if it was paused, and the prompt notes when auto has been paused.

### [3.1.0.0] - 2026-07-19

#### Added

- Card status and actions in the Bingo Cards tab: a Status column marks pending custom-card requests, approved ones and Protected cards, and each row gains Approve and Protect / Unprotect. Approving makes the card playable and protects it, and Protected cards are spared by Delete all.

#### Changed

### [3.0.1.0] - 2026-07-17

#### Changed

- Button colors match the web admin dashboard's default theme (a tan primary action with dark text, an olive secondary and a red destructive), replacing the initial purple accent.
- Bingo win patterns gain a Collapse all / Show all toggle to fold or unfold every pattern category at once while building a game.

#### Fixed

- A Timed Text Macro's message is now actually hidden until you expand it. The preview text used to stay visible with only the button label flipping; Show message / Hide message now shows or hides the full text, and the button's stray glyph (it showed as "=") is gone.

### [3.0.0.0] - 2026-07-17

#### Added

- Edit a saved Timed Text Macro in place: each macro has an Edit button that reopens its name, channel, message, interval and send cap in an inline editor (Save / Cancel). Stop a running macro first to edit it.
- A shared design across every page: icon and accent section headers, a three-tier button system (primary / secondary / destructive), compact icon buttons for table-row actions (copy / delete), pill badges and rounded cards.

#### Changed

- The always-available tools (Rolls, Timed Text Macros, Settings, About) live under a collapsible General group at the bottom of the sidebar.
- Timed Text Macros shows each macro as its own card: a numbered title with channel and status badges, a schedule summary and a one-line message preview that expands on demand, so a long macro no longer floods the panel.
- Rolls is split into labeled Captured rolls / Filter / Find the winner / Rolls sections, and Settings groups its fields under Connection and Automatic tells.

### [2.3.0.0] - 2026-07-16

#### Added

- A Rolls page, needing no account or permission, that watches chat for `/random` and `/dice` rolls near you and lists the player, home world, number rolled (with its ceiling for `/random N`) and time.
- Rolls can be filtered to a player and/or the last N minutes, and a Highest / Lowest / Closest-to-N query brings the matching rolls to the top; a player who rolled more than once in the window is flagged. Rolls are held in memory only, clear on logout, and a Clear button wipes them.
- A Timed Text Macros page, needing no account or permission: any number of named announcements, each sent on a chosen public channel (Say / Yell / Shout) every N minutes, with an optional send cap and a live countdown. A long message is split into parts sent a second apart.
- Macros are saved across logout and restart but always reload stopped; logout also stops every running macro.

#### Fixed

- Long text wraps to the window instead of running off the right edge on the Rolls, Timed Text Macros, Settings, Garapon, Stamp Rally, Raffle and Bingo pages.

### [2.2.1.0] - 2026-07-16

#### Added

- Editable auto-tell templates for all three tells (bingo card, garapon, stamp rally): enabling a tell in Settings reveals a message editor, with placeholders for the recipient's name and the link. The defaults reproduce the earlier fixed messages.

#### Changed

- Long tells are split to fit the in-game chat limit, at a sentence end where possible and otherwise a word boundary, with the parts sent a second apart. The Settings editor warns when a template will split.

### [2.2.0.0] - 2026-07-16

#### Added

- A Garapon page to issue a per-player drawing link for an existing garapon (pick the name from the nearby list, set the draw count, copy the link and, with a Stamp Rally, the card link), and a Garapon Draw Log page. Needs the Festival Garapon permission.
- A Stamp Rally page to issue a participant card for a nearby player and pause or resume individual stalls, and a Stamp Rally Log page showing the collected-stamp log. Needs the Festival Stamp Rally permission.
- An optional `/tell` that sends a player their link or card when you issue one for someone picked from the nearby list; off by default and switched on per feature in Settings.
- A collapsible sidebar replaces the flat tab bar, with Bingo and Festival sections, Settings and About. Pages the account lacks the permission for are hidden, and the window opens on the first page it can reach.

### [2.1.0.0] - 2026-07-11

#### Added

- "It's Yoever" live controls on the Bingo Game tab: a checkbox to switch the reaction on or off for all players during a live game, and a "Yoevers: N" counter. Both follow the server live.

#### Changed

### [2.0.1.0] - 2026-07-02

#### Security

- The personal access token is sent in the `Authorization` header on the live connection instead of the URL query string, where it could land in access logs.

### [2.0.0.0] - 2026-07-01

#### Changed

- The plugin's API client was reworked for the backend's move to a hybrid RESTful style with proper HTTP status codes, and needs the correspondingly migrated backend.

### [1.0.0.0] - 2026-06-29

#### Added

- First published release, through the Dalamud custom repo.

#### Changed

- Cards are sorted by creation date.

### [0.1.0.0] - 2026-06-29

#### Added

- Initial Dalamud plugin (Senpan Companion): operate bingo and raffle features from within the game, authenticated by a personal access token.

---

## Frontend

The site's own history from before 4.0.0, when it was released on its own.

### [3.32.0] - 2026-11-07

#### Changed

- Adopted the shared lint rules and test settings.

#### Fixed

- Every page now has a main landmark and a level-one heading for screen readers; the visible page titles are unchanged.
- The footer's links stand out from its text by weight as well as color, and the Admin Portal button on the home page is easier to read.
- The player's volume control and the stamp lookup's world picker are now labeled for screen readers.

### [3.31.1] - 2026-11-01

#### Changed

- Building the app now needs Node 26.

### [3.31.0] - 2026-10-31

#### Added

- Each day on a plaque board has an Artist Requests limit beside its webhook (empty for no limit), with the number of requests so far shown next to it.
- On a limited day, the festival dashboard's Ask the Artist option shows how many requests are left and is disabled once the day is full; designing on the site is unaffected.
- Post Plaque in Ema Requests: the artist uploads the finished plaque (PNG, JPEG, GIF or WebP, up to 10 MB), looks it over and posts it to that day's Discord channel, which marks the request complete.

#### Fixed

- Sending a plaque from an out-of-date page now refreshes the dashboard and shows why it can't be sent.

### [3.30.0] - 2026-10-30

#### Added

- A festival with a plaque board gets an Ema section on its dashboard, offering one plaque per person per festival day and showing their plaques and where each stands.
- Design it yourself: save the blank plaque, decorate it in any image editor and post it in the festival's Discord, following the board's instructions.
- Design it here: an image editor on the page for placing pictures and text, with the finished plaque downloaded or sent for staff review before the app posts it to Discord under the visitor's name.
- Ask the artist: describe the plaque and the board's artist makes it.
- Festival -> Ema Plaques lets staff set up one board per festival (template, instructions, options offered, and each day's Discord webhook) and review submitted plaques; approving posts one, rejecting lets its author send another.
- Festival -> Ema Requests is the artist's own queue of what visitors asked for, marked complete once posted, under its own permission.

### [3.29.0] - 2026-10-29

#### Added

- A festival dashboard at `/festival/<shortcode>` puts the stall map, stamp rally, garapon and raffles of a festival on one page, so a visitor can take part in all of them and see their stamps collected, draws left and raffles entered at the top.
- On the dashboard, the map marks stalls you have already stamped with a ring and a check, and a stall you still need offers a stamp-password box in its panel.
- A stamp card or garapon link for a rally in a published festival offers "Open the festival dashboard", which brings the link along.
- The dashboard remembers who you are on this device only, and "Forget this device" clears it.

### [3.28.0] - 2026-10-17

#### Added

- A tea room whose unlock time arrives raises a pop-up on every open admin page for accounts that can manage tea rooms, naming the room, with a chime, a vibration on phones that support it and an Open Tea Rooms button; the site has to be open to hear it.
- Alerts stay until dismissed and stack in the top-right column instead of replacing each other, and a "Saved" toast no longer wipes one.

### [3.27.0] - 2026-09-19

#### Added

- A Hide Custom Cards toggle in System -> Settings removes just the Custom Card request from the home page while games carry on; Hide Bingo still hides both, and existing request links and pending requests are untouched.

### [3.26.0] - 2026-09-05

#### Added

- A tea room can be locked from a button on its row, optionally with a time (in your own timezone) at which it unlocks by itself; the row reads "Locked until 8:00 PM" or "Locked", and the same fields are on the room's edit form.

### [3.25.0] - 2026-08-31

#### Added

- A published festival map can be embedded on another site with an `<iframe>` snippet from its admin page; the embedded map has panning, zoom, the day switcher and stall details, and stays current as stalls change.

#### Changed

- Tapping a stall opens its details in a draggable, resizable panel over the map instead of further down the page, leading with whether the stall is open.
- The zoom, Fit and day controls sit in their own bar above the plan, and the legend is gone.
- A stall on a stamp rally is marked with a stamp icon instead of the stamp's artwork, which stays in the stall's panel.
- Festival times read like dates ("Sun, Aug 30, 2026, 8:00 PM - 1:00 AM EST") on festival pages.

#### Fixed

- The public festival map no longer opens cropped or too small, and Fit shows the whole plan.
- Closing and dragging the stall panel work again.
- A vendor shows only on the days its hours cover, and a shared stall shows both vendors only when two are really there the same day.
- A stall names a day only when it means something, and the stall type is back on the map under the name.
- The booth page link, selection outline color and label color set in the map editor now save.
- Both tenants of a shared stall are shown in full, and line breaks in descriptions render instead of a literal `<br />`.

### [3.24.0] - 2026-08-28

#### Added

- Your browser remembers the stamp rallies you signed up for, for 60 days: returning shows your links again, the rally list marks what you joined, and "Forget these links here" clears them.
- A raffle you have already entered fills in the name you used, so more entries top up your entry instead of starting a second one.

#### Changed

- Home world is picked from a list on every form (sign-up, raffle entry, "Find My Links" and the custom card request), and the rally sign-up asks for the name and world separately.

#### Security

- "Find My Links" no longer shows a drawing link, only the garapon and how many draws are left; the stamp-card link is still returned.

### [3.23.0] - 2026-08-26

#### Changed

- A first visit for a player or public visitor now downloads about a third as much, since the admin pages and Markdown editor are left out (admins still get them offline after one visit), and the winners log loads faster.

#### Fixed

- A mistyped current password when changing your own password no longer logs you out.
- The public raffle list shows only enterable raffles.
- Stamps on a rally linked to a festival map are labeled by their stall in the admin tables.
- Four icons that drew as nothing now render: the three Export CSV buttons and the Skip Occurrences heading.
- The festival map and stamp card placement editors are usable without a mouse, and book club row actions have accessible names.
- Stamp Rally and Festival Map Edit and Duplicate wait until the record has loaded, so a quick click can no longer wipe its stamps, prizes or pitches.
- Festival map pitch names are one size across the plan instead of varying with each pitch's height.

#### Security

- The Settings page hides the Google Fonts API key and per-club Discord webhook URLs from accounts that are not full admins.
- CSV exports of the stamp and garapon logs can no longer carry a live spreadsheet formula from a participant name.
- Affiliate Carrd and Discord links on the public festival map can no longer use a non-http scheme.

### [3.22.0] - 2026-08-24

#### Added

- Festival Map, a new page at the top of the admin Festival section: a titled floor plan with a markdown description, the festival's date ranges and a base image, with a status of In Progress, Published or Closed (only Published reaches the public).
- A visual stall editor draws stalls as circles or rectangles in their own color on the base image; drag to move, resize or rotate them, and positions scale to any screen.
- Each stall names what it offers under its title (Game, Food, Food & Game or Other, where Other takes its own wording such as "Omikuji"), which also seeds its shape and color.
- A stall can hold several occupants, each with its own affiliate, title, description, offering and dates, so a booth that changes hands between days is one shape; "Add another day" adds one.
- A day switcher on the public map, shown when the festival spans more than one date, leads with who is in each pitch that day and dims pitches nobody occupies.
- The public interactive map at `/festival-maps` and `/festival-maps/:id`: drag to pan, wheel or pinch to zoom, and tap a stall for what it offers, its hours, whether it is open now and the affiliate's card; the home page gets a Festival Map card once a map is published.
- A map can carry an optional shortcode such as "obon-2026" so it links as `/festival-maps/obon-2026`, while the id URL keeps working.
- Raffles can be filed under a Festival Map and pinned to one stall, and a pinned raffle appears in that stall's panel while it is open.
- A stamp rally can be linked to a festival map so its stamps are picked from that map's stalls, taking the stall's affiliate and stamp type; on the public map those stalls are ringed with their stamp art, with a "highlight the rally's stalls only" toggle and a link to the rally's sign-up page.
- Closed Garapons, Stamp Rallies and Festival Maps now have the row-level Copy action closed Raffles already had.

#### Changed

- Duplicating a stamp rally or raffle no longer keeps its festival link, so next year's copy doesn't file itself under the finished festival.

### [3.21.0] - 2026-08-24

#### Added

- Food stamps and game stamps: each stall on a rally card has a type picked in the rally form (Add Food Stamp / Add Game Stamp), and every existing stall is a food stamp.
- Card Completion on the rally form: keep the whole-card rule or require a number of each type, such as 3 food and 3 game stalls.
- The type shows in the rally's Stamps table, the stamp log and the participant's Stalls list, and the rally detail states what finishes a card.
- With a per-type rule, the participant's card shows progress per type ("1 of 3 food stamps - 0 of 3 game stamps collected") and a notice when the card can no longer be finished.

#### Changed

- A complete card can keep collecting from stalls still open when the rally counts types, instead of locking the participant out.

### [3.20.0] - 2026-08-24

#### Added

- Close / Reopen on a raffle, next to Duplicate, so a raffle with no entries or a details-only raffle can be taken off the public list; closing asks first, and neither touches a recorded winner.
- Duplicate on raffles, stamp rallies and garapons opens the create form pre-filled from an existing item, with `(Copy)` added to the title and the old run's dates cleared; issued cards, drawing links, draw logs and raffle entries stay with the original.
- Garapon and Stamp Rally show open counts in the admin sidebar.
- An "Entries Paid For" box in the raffle payment dialog records a part payment and shows how many entries remain unpaid.

### [3.19.0] - 2026-08-21

#### Added

- Three raffle entry types on the raffle form's new Entry Type picker: Details Only (no sign-up form, instructions shown under "How to Enter"), Single Cost per Entry (the original behavior) and Custom Cost per Entry (a price per entry, which also sets the per-player limit).
- The public raffle page lists a custom raffle's price ladder with running totals, the sign-up form's Total Cost follows it, and raffle cards name the pricing.
- Recording a payment can waive part of the cost: the Paid button opens a dialog showing the entry's tickets, cost, amount already waived and amount outstanding, and the admin Add Entry row takes a waived amount too.
- Partly paid entries are visible: the entries table reads `3 (1 paid)` with a Partial pay button, and the Cost column shows `-X waived = Y`.
- A "Hide Bingo" switch in System -> Settings removes Join Bingo and the Custom Card request from the public home page while no game is running, leaving existing `/play/:cardId` and `/card-requests` links working.
- A "Where to Pay" image on raffles, shown to players with the sign-up instructions and on the confirmation after signing up.
- An "Already Entered?" search on the raffle page reports each match's name, world, entry count and payment status, and "Use this name" fills the form with that exact spelling so a near-miss doesn't start a second entry.

#### Changed

- Custom Card now sits directly under Join Bingo on the home page.
- Stamp Rally sign-up links open in a new tab, and the warning reads "Save these links or keep this page open."
- The raffle sign-up confirmation leads with Amount Due and adds an "Already Covered" line for a returning entrant, and Max Entries Per Person is capped at 100.

#### Fixed

- A long price ladder no longer overruns the raffle card.
- A raffle with a large per-player allowance no longer freezes the page while pricing an entry.

### [3.18.0] - 2026-08-11

#### Added

- Public Stamp Rally sign-up: `/stamp-rallies` lists rallies open for sign-up, `/stamp-rallies/:id` is one rally's linkable sign-up form, and `/stamp-lookup` finds a participant's links; the home page shows a Stamp Rally card only while a rally is open.
- Sign-up asks for a full `Firstname Lastname @ World` name, with one card per name per rally; a repeat sign-up is sent to the lookup page.
- After signing up, the page shows the stamp card link and, when the rally has one, the Garapon draw link, each with a copy button and a warning that these links are the only way back in.
- The stamp rally form has a "Public sign-up" toggle (off by default), and the garapon form a "Default Draws" field for the allowance a self-service sign-up issues.
- The markdown editor has an emoji button.

#### Changed

- The markdown editor keeps its formatting controls in a bar at the top, limited to formatting Discord can render.

### [3.17.0] - 2026-07-28

#### Added

- Admin tables have banded rows and a hover highlight, and Manage Cards, the Winners Log, Server Logs and the garapon and rally logs let you drag a column's edge to resize it.
- Manage Cards and the Winners Log have row checkboxes and a "Delete N selected" action, with the header checkbox selecting every row matching the current filter.
- The garapon draw log can be filtered to a single prize, and the Winners Log, garapon draw log and rally stamp log export as CSV (every matching row, in the current sort order).
- A Konami code on the player board (up, up, down, down, left, right, left, right, B, A) clears every stamp and swells Drani's grin over the screen; it respects reduced motion.

#### Changed

- A visual pass aligns the app with the senpan.cafe website (square corners, no shadows, Montserrat body text, uppercase display-font titles, icons in place of text glyphs), and the home page now has a centered logo masthead with Join Bingo as the primary panel; no theme color changed.
- The player's "It's Yoever" controls are one row, `It's Yoever: [Effects] [Sounds]`, and turning Effects off locks Sounds.
- Every table has the same toolbar order (search, per-page selector, count, actions), and tables sort across the full list rather than just the visible page.
- The theme editor's live preview shows the app's real panels, buttons, badges and board, so the contrast report matches what players see.

#### Fixed

- Success and error text, including "request submitted", "you're entered" and "Sent to players!", was nearly invisible on every theme and is now readable.
- Page titles sat off-center on pages with a Back button, and pattern previews had uneven cell spacing.
- Three Server Logs buttons showed no fill, and buttons ignored a theme's accent hover colors.
- The connection-status dot kept pulsing for players who asked for reduced motion, and stamp-rally stall badges had unreadable white text on every theme.

### [3.16.0] - 2026-07-24

#### Added

- The tea-room form has an optional Room owner field naming the character who owns the room, shown in the plugin's tea-room table and the public tea-room API.

### [3.15.0] - 2026-07-24

#### Added

- The affiliate form has an optional Subtitle under Name, shown beneath the name in the affiliates list.
- Accounts with the Images permission get an Upload button beside every image picker's category selector and can drop files on the thumbnail grid, so a missing image no longer means leaving the form.

#### Changed

- Image pickers list images alphabetically by file name (digit-aware) instead of newest first.

### [3.14.1] - 2026-07-21

#### Changed

- Stacked dialogs behave correctly: only the top one reacts to Escape or holds focus.
- The admin sidebar sections, the pattern delete, rename and collapse controls and expandable table rows work from the keyboard and screen readers.

#### Fixed

- Moving between admin pages no longer drops the live connection, so draws, winners and log updates in the gap aren't missed.
- A re-delivered draw no longer double-counts a number or replays its chime, and the "Live" badge no longer flashes "closed" during a deliberate reconnect.
- Fast navigation could show an older response over a newer one (Carrd folders, public raffle details, stamp-rally panels).
- Double-clicking the custom card request submit button no longer sends it twice.
- The player board recovers from corrupted saved stamps and keeps a stamp opacity of 0.
- The draw hotkey (Space/Enter) no longer fires behind a dialog or while a button-like control is focused.
- Loading the admin login or register page while signed in now redirects instead of showing the form.

#### Security

- Theme values in the editor preview and theme flourish artwork are sanitized in the browser so a hostile theme can't inject CSS or script.

### [3.14.0] - 2026-07-20

#### Added

- The New Game form has an "Auto-draw numbers" toggle with a "Time Between Calls" selector (10s to 5m) so the server draws a number every interval; during a live game an Auto-Draw On/Off button and interval selector change it at any time.
- Game Presets have the same Auto and Time Between Calls fields, which applying a preset pre-fills.

#### Changed

- The half-time prompt appears on every admin screen at once for any draw; auto-draw pauses at half-time and when a winner is recognized, and "No mini-game" resumes it.
- The Current Game view leads with Draw Number, Delay and End Game, and moves Auto-Draw, It's Yoever and Winner Sound into a collapsible Game Settings panel.
- The Current Game "Live" badge and sidebar live-game dot are green instead of red.

#### Fixed

- Several buttons and notices (the Images and Carrd Save buttons, the theme editor's contrast warning, the version-mismatch flag) showed a blank space instead of their icons.

### [3.13.0] - 2026-07-19

#### Added

- A public theme picker in the site footer: themes marked Public in the Themes editor can be chosen by each player and are remembered per browser, with a "Default" option that follows the admin's active theme.
- A Personal Card Requests page (`/card-requests`, linked from the home page) where a player enters a character name and world, builds a 5x5 card by hand or with Generate Random, picks a custom 6-character card ID, and sees the gil cost and terms before submitting.
- Manage Cards shows a hollow star for a pending custom card, a filled star for an approved one and a lock for a Protected card, with Approve and Protect/Unprotect row actions; Delete All now keeps Protected cards.
- A Custom Card Cost setting (System -> Settings -> Gameplay), the gil price shown on the Personal Card Requests page.
- The Called Numbers tracker dims any BINGO column the game's win patterns never use.

### [3.12.0] - 2026-07-16

#### Changed

- The Plugin version in the admin sidebar footer now reflects the version currently published to the plugin repository.

### [3.11.0] - 2026-07-16

#### Added

- The Garapon drawing page shows a "View your Stamp Rally card" link when the garapon is tied to a stamp rally and the player holds a linked card.

### [3.10.0] - 2026-07-12

#### Added

- The stamp-rally editor has a "Where to Redeem" image picker, usually a screenshot, shown on the participant's card with the redeem instructions once the card is complete.

### [3.9.0] - 2026-07-12

#### Added

- The Images tab shows a percentage bar while files upload, then "Processing...", instead of a spinner that looked stuck.

#### Fixed

- Large or multiple image uploads over a slow connection were cut off after 30 seconds with "Request timed out. Please try again."; font, Carrd and book-club uploads had the same problem and now take as long as they need.

### [3.8.0] - 2026-07-12

#### Added

- Affiliates has a shared Discord webhook on a Webhook sub-screen and a Post button on each row that posts the affiliate as an embed.
- The affiliate list is drag-sortable with a persistent order, and each row shows the logo (or screenshot), an embed-color swatch and Discord, Carrd and open-times badges.
- The affiliate form has fields for an embed accent color, a Discord Link and a Carrd Link.

### [3.7.0] - 2026-07-11

#### Added

- An "It's Yoever" button on the player board: while a game is running and the reaction is on, a player triggers a sound and a bouncing head image on every connected screen, captioned with their name, followed by a cooldown (default 3 minutes) that survives a refresh.
- Two per-device toggles, "Show It's Yoever effects" and "Play It's Yoever sound", on the player board and the admin Game tab.
- In a live game the Game tab has an on/off switch for the reaction and a "Yoevers: N" counter, both reset for each new game.
- An "It's Yoever" Cooldown (seconds) setting in Gameplay, 0 to 3600, where 0 means no limit.
- The Frontend, Backend and Plugin versions in the admin sidebar footer open their changelog in a modal, and the Plugin version is new.

#### Fixed

- Saving a numeric setting failed with "Invalid JSON."

### [3.6.0] - 2026-07-10

#### Added

- A Tea Rooms page (Senpan Tea House -> Tea Rooms) with a drag-orderable list of bookable rooms, each with a name, room number, gil price per half hour, hashtags, description, status flags and image, and row actions to post to Discord, open/close, discount, copy the API link, edit and delete.
- A Webhook sub-page stores the shared Discord webhook the rooms post to, and the page is gated by a new `teahouse-tea-rooms` permission.
- A posted room's Discord embed shows the name, description, cost (halved with a "Currently Discounted!" note when discounted), room number and Open/Closed status.

### [3.5.0] - 2026-07-07

#### Added

- The Server Logs viewer has a User column naming who made each request: an admin's username, a verified bot's name, or "-" for anonymous traffic.

#### Fixed

- Admins without image access got a 403 error toast on every image change, and a renamed or deleted image category raised an "Unknown image category" toast on later updates.

#### Security

- A font name or header font containing characters that could break out of CSS is refused and replaced with the default header font.

### [3.4.0] - 2026-07-05

#### Added

- A Server Logs page (System -> Logs, admin-only): a live-tailing viewer with colored method and status badges, a minimum-level filter, text search, a pause/resume "Live" toggle, a per-page selector and pagination, with each line expanding to its full JSON on click.
- A Debug On/Off button changes the server's log level without a restart.

#### Fixed

- An error with no JSON body, such as an empty or HTML 502 from the gateway, now reads `Request failed (HTTP 502)` instead of a bare `Request failed`.

### [3.1.0] - 2026-07-03

#### Changed

- The image picker in the announcement, raffle, garapon, affiliate, stamp-rally and theme-flourish editors browses the whole image library through its own category dropdown, opening in the selected image's category (flourish pickers offer only `.svg`).
- Every image category can now be renamed and deleted, and the "Permanent" badge is gone; deleting a category still removes its folder and files.
- Open image pickers pick up another admin's uploads live.

### [3.0.0] - 2026-07-02

#### Added

- An "Embed on External Sites" panel on Font Upload copies a permanent `kit.css` `<link>` snippet for Carrd sites, each receiving only the fonts whose allowed sites include it; a font's Copy URL button copies a tokenized link that expires in 1 to 2 weeks.
- A live-preview format switch previews a font in each of its actual formats.

#### Changed

- Uploaded fonts load from rotating same-origin links instead of static `https://fonts.senpan.cafe/<file>` URLs, so the font picker and player-facing fonts work regardless of any external allowlist.
- Font Upload is organized around families: files sharing a base name appear as one font, the table shows CSS Name, Serves, Modified and actions, and an Edit modal holds the CSS name, served-version picker, allowed-sites list and per-file rename and delete.

### [2.2.0] - 2026-07-02

#### Added

- The login page offers a usernameless "Sign in with a passkey" option, and every account can add, name and remove passkeys under User Options -> Add Passkey, alongside the password.
- The public raffle sign-up shows the Cloudflare Turnstile bot check, and requires it, when Turnstile is configured.

### [2.1.0] - 2026-07-02

#### Changed

- Clickable cards, chips and collapsible headers work from the keyboard and screen readers.
- Pattern and announcement drag-and-drop no longer needs `unsafe-eval`, so the site can enforce a strict Content-Security-Policy.

#### Fixed

- Switching quickly between raffles, garapons, stamp rallies or book-club lists could leave the wrong record showing.
- Removing a row from a repeating list (garapon prizes, affiliate hours and owners, announcement buttons, reading-list sources) could leave the inputs showing the wrong row's values.
- API requests now time out after 30 seconds instead of leaving a spinner forever on a hung network.

#### Security

- The post-login `?redirect=` value is checked to be a same-origin path, and a 401 now clears the whole signed-in session so stale page permissions can't linger.

### [2.0.0] - 2026-06-30

#### Changed

- The app uses the REST resource paths introduced in backend 2.0.0, and requires backend 2.0.0 or later.

### [1.5.2] - 2026-06-30

#### Changed

- No user-visible changes.

### [1.5.1] - 2026-06-30

#### Fixed

- A corrupted saved board in the browser, or a full browser storage, could break loading stamps or stamping a cell; a bad saved board now starts clean and a full storage no longer throws.

### [1.5.0] - 2026-06-29

#### Added

- Personal access tokens (User Options -> Access Token): an account can generate a token so an external client, such as a Dalamud plugin, can sign in as that account with its page permissions. The secret is shown once with a copy button, and the token can be regenerated or revoked.

### [1.4.0] - 2026-06-29

#### Added

- A Garapon can link to an open Stamp Rally; issuing a drawing link then also issues a stamp card sharing the same token, with a copy button for each in the drawing-links table.
- Stamp rallies have an open or closed status separate from the date window: an Open card grid and a Closed table with Close/Reopen, where a closed rally is read-only and not offered for Garapon linking.
- Each open rally card shows "X/Y stalls active" and a Manage stalls panel to pause or resume vendor stalls.

### [1.3.1] - 2026-06-29

#### Changed

- Reordered the items within each admin sidebar section.

### [1.3.0] - 2026-06-29

#### Added

- A Stamp Rally page (Festival -> Stamp Rally) for authoring stamp-rally events: a card background and "not stamped" placeholder, availability window, markdown details and "How to Redeem" instructions, and stamps and prizes placed with a visual drag, resize and rotate editor.
- Each stamp links to an affiliate stall (or Senpan Tea House), has a password and an optional active window, and can be paused and resumed.
- Admins issue each participant a tokenized `/stamp-card/<token>` link; participants enter stall passwords to collect stamps, and a completed card reveals its prizes and redeem instructions.
- A per-event View Logs page lists every collection (participant, stall, time) and updates live.
- Three new image categories (Stamp Cards, Stamp Stamps, Stamp Prizes) and a `festival-stamp-rally` page permission.

### [1.2.0] - 2026-06-29

#### Added

- An Affiliates page (Senpan Tea House -> Affiliates) for partner establishments: name, owners, location, opening hours across time ranges under one timezone, markdown details, and a logo and screenshot picked from two new image categories; gated by a new `teahouse-affiliates` permission.

### [1.1.1] - 2026-06-28

#### Added

- The hidden registration form shows the Cloudflare Turnstile bot check too, gated the same way as the login form.

### [1.1.0] - 2026-06-28

#### Added

- The admin login shows a Cloudflare Turnstile bot check and requires it when the server has a Turnstile site key.

### [1.0.0] - 2026-06-28

#### Added

- A version readout in the admin sidebar footer showing the frontend and backend versions, with a flag when their major versions differ.
- A WCAG 2.1 contrast report in the theme editor: a live audit of the app's text and background pairings with AA and AAA verdicts, updating as colors change.
- A token-based theme editor with a collapsible live preview, per-token help text and color pickers (alpha-capable for translucent tokens).

#### Changed

- All built-in themes were retuned for contrast, and a Toji theme was added.

#### Removed

- The CodeMirror free-form CSS theme editor; themes are now structured tokens.

---

## Backend

The server's own history from before 4.0.0, when it was released on its own.

### [3.28.0] - 2026-11-07

#### Added

- Every backend deploy installs and checks a daily database backup kept for 28 days beside the database, and the deploy fails if the backup can't be installed.
- After a backend deploy the live database is pulled into the local development copy, keeping the three previous copies; `-NoDbPull` skips it.
- After a deploy, a smoke check of `/api/version`, the page, its entry script and the plugin index rolls back what the run changed if anything fails; the address it checks can be set with `-SiteUrl`, `DEPLOY_SITE_URL` or `$DeploySiteUrl`.

#### Changed

- A rollback now restores the database along with the binary, so anything written while the new release ran is lost with it.
- With the plugin in a deploy, a version not higher than the published one stops the run before the backend and frontend go live.
- Adopted the shared lint rules, CI workflows and repo files.

### [3.27.0] - 2026-10-31

#### Added

- A plaque day can cap its artist requests (0 means no limit), staff see how many requests each day holds, and a request past the limit is refused with a 409.
- The festival dashboard's Ema days report how many artist requests are left, so the request option can close when the day is full.
- The artist can post a finished plaque for an open request straight to its day's Discord webhook as `Requested by <character>, made by <artist>`, which marks the request complete (PNG, JPEG, GIF or WebP, up to 10 MB).

### [3.26.0] - 2026-10-30

#### Added

- Ema plaque boards: a festival can have one, taking plaques on days that may not overlap, each day with its own Discord webhook, and one plaque per person per day.
- Staff review plaques designed on the site: approving posts the image to the day's webhook, rejecting frees the day, and either way the stored image is deleted.
- The artist has a queue of requested plaques behind its own permission.
- Visitors can submit a plaque (a PNG) or ask the artist for one for the day open now, rate limited and bot-checked.
- The festival dashboard carries the board, its days (never their webhooks), the day open now and the visitor's own plaques.
- `GET /api/google-fonts` lists Google Fonts families, cached for 12 hours.
- Discord posts can carry an image.

#### Changed

- The festival list is also readable with the Ema permission, and deleting a festival deletes its plaque board and the plaque images still waiting for review.

### [3.25.0] - 2026-10-29

#### Added

- `POST /api/festival-dashboard/{id}` returns one visitor's standing at a festival: their stamp card, the garapon linked to its rally and every raffle filed under the festival with their own entry, from the tokens and name their device holds.
- Each public stamp says which map stall it belongs to, and the public stamp card and garapon responses carry `festival_link` when the rally belongs to a published festival.

#### Changed

- Queries and AniList lookups stop when the request that started them is abandoned, and a dropped connection is logged as `canceled=true` instead of reading as a server error.
- Adopted the shared lint rules.

#### Removed

- The `-secret` flag, which never did anything; a service that still passes it will not start.

#### Fixed

- An upload whose last write never reached the disk, such as on a full disk, is now reported as failed instead of as a success with a file missing or cut short.

### [3.24.1] - 2026-10-16

#### Fixed

- Hosted fonts now load in Safari, including on every iPhone, where font requests from an allowlisted Carrd site were refused with a 403.

### [3.24.0] - 2026-09-19

#### Added

- A `hide_custom_cards` setting takes the Custom Card request off the public home page while bingo itself stays up; it defaults off.

### [3.23.0] - 2026-09-05

#### Added

- Tea rooms record whether they are locked, separate from being lockable: `locked` and `locked_until` are set independently and both appear on the public rooms API.
- A lock with an expiry lifts on its own and tells the admin channel the room's name and number, which the in-game plugin uses to alert its operator.

### [3.22.0] - 2026-08-30

#### Added

- A per-stall label color (white by default) and a per-stall selected-outline color for the ring around a stall a visitor taps.
- A per-occupant Booth Page link, shown beside the affiliate's Website and Discord buttons in the stall panel.

#### Changed

- A stall must name its affiliate: the form starts on "Select an affiliate..." and a map won't save until every stall has one.
- A stall's title is optional, and an untitled stall is named by its affiliate in the editor, the stall lists and the raffle and stamp-rally pickers.
- The admin map page draws the map exactly as the public page does, and saving a map leaves you on that map.
- The stall info panel no longer repeats the affiliate name or lists the affiliate's owners.

### [3.21.0] - 2026-08-28

#### Changed

- A participant is now a character name plus a home world picked from a list everywhere, so stamp rally cards and garapon links no longer hold one typed "Firstname Lastname @ World" string.
- Stamp rally sign-up and lookup match on name and world, so two players sharing a character name on different worlds are two people.

#### Security

- A stamp-card link is no longer a spendable drawing token: new sign-ups issue separate tokens for the card and the drawing link, and the drawing link staff issue no longer doubles as a stamp card.
- `POST /api/stamp-lookup` no longer returns the garapon drawing token; it returns the card token and how many draws remain.
- `POST /api/garapon/{token}/draw` is now rate limited.

### [3.20.0] - 2026-08-26

#### Changed

- Holders of the Festival Map and Tea Rooms pages can use the shared image picker, the festival map and stamp rally forms can list affiliates, and the pending custom-card preview opens for the Manage Cards page without full admin.
- The backend deploy snapshots the live database before migrating it (keeping the newest three), deploys the API before the SPA, and moves the plugin download and index into place atomically.

#### Fixed

- A long announcement now posts instead of failing, and an announcement edited while it posts keeps the edit instead of reposting (`@everyone` and all) once sent. Skipping with an explicit count of 0 is now honored.
- Bulk card generation can no longer hand out duplicate ids, deleting a raffle's winning entry clears the winner, a failed theme delete no longer leaves the site on the default theme, and two admins editing fonts at once can't discard each other's changes.
- The live log tail no longer gets admins disconnected, and a player's live connection rejects an unknown card id, is capped at 500 connections and now receives the notice when its card is deleted.
- The first auto-draw number comes straight away, starting a game whose selected patterns were all deleted now says to reload and pick again, and a draw landing while a board loads is no longer undone.
- A festival map is no longer unpublished by a save with a misspelled or missing status, deleting an affiliate no longer erases the stalls it named, and the public map badges only a stamp rally that is running.
- Saving a stamp rally or festival map from an editor that never loaded the full record no longer wipes its stamps, prizes or stalls, and deleting a garapon drawing link no longer destroys a part-stamped rally card.
- Upgrades no longer risk stopping the server from booting or losing data, and the server now refuses to start against a database newer than itself, naming both versions.
- Signing in with a passkey now works for accounts whose password has ever been changed or reset.

#### Security

- Card and pattern changes no longer reach anonymous players' live connections; only admin connections receive them.
- Saving the Settings page as a non-admin no longer overwrites secret settings, such as every club's Discord webhook, with blanks.
- Public forms are bounded: a raffle sign-up above 100 entries per person is rejected, names are capped at 60 characters, wrong stamp passwords are rate limited, and opening the passkey prompt has its own rate limit.
- A reading list item can be reached only through its own list, and a festival map save naming another map's stall is refused with a 400.

### [3.19.0] - 2026-08-24

#### Added

- Festival maps: staff can build a floor plan from a title, description, dates and a base image, place stalls on it, and mark it in progress, published or closed, behind the new `festival-map` page permission.
- Public festival map endpoints show published maps only, with each stall's affiliate card, whether it is open now, and a stamp rally badge when an open linked rally places a stamp on it.
- A stall is a pitch that can hold several occupants (affiliate, title, description, offering, times), so a booth that changes hands between days is one shape, and a stall of type "other" can carry its own caption.
- A map can have a shortcode, so it links as `/festival-maps/obon-2026`, and links by numeric id keep working.
- A stamp rally can be linked to a festival map, its stamps then naming the map's stalls.
- A raffle can be filed under a festival and assigned to one of its stalls, and the public map links it only while the raffle is running.

#### Changed

- A stamp's stall name on the public card and the stamp log comes from its map stall, then its affiliate, then the venue.

### [3.18.0] - 2026-08-24

#### Added

- Stamps in a stamp rally have a type, food or game; existing stamps read as food.
- A rally can complete on a count of each type instead of the whole card.
- The public stamp card shows the completion rule and each stamp's type, and marks a stamp as expired once its stall can never be collected again.

#### Changed

- On a rally that completes on counts, expired stamps no longer complete a card; it completes only by meeting its counts.

### [3.17.0] - 2026-08-24

#### Added

- A raffle can be closed and reopened without a winner, so a raffle with no entries or a details-only raffle no longer stays on the public list indefinitely.

### [3.16.0] - 2026-08-21

#### Added

- Raffle entry modes: single (one flat cost per entry), custom (a price ladder, so 50k/100k/150k charges 300,000 for three entries) and details (nothing priced, entered elsewhere); existing raffles are single.
- Part payments: settling an entry can cover only some of its tickets, and the gil waived is recorded.
- The sign-up confirmation states the amount due, which is lower than the total when part of the entry is already settled.
- A public search for whether you have already entered a raffle (`POST /api/raffles/{id}/lookup`) returns the exact spelling used, matching part of a name and showing no gil figures.
- A raffle can have a "Where to Pay" screenshot, and a Hide Bingo setting controls whether bingo appears on the public home page.
- The per-player entry allowance, and so the price ladder, is capped at 100; going over is refused with a 400.

#### Changed

- The collected-gil total prices what each entry actually paid, counting part payments and waivers.
- The winner draw weights settled tickets, so a part-paid entrant gets one chance per ticket paid, and a details-only raffle draws from every entry.
- Adding tickets to an already-settled entry puts it back to unpaid so the new tickets are collected for.

#### Fixed

- Settling the same entry twice no longer stacks the waiver and understates what the raffle collected, and the admin raffle list no longer slows down when a raffle has a very large allowance.

#### Security

- The tickets paid and gil waived on an entry no longer appear on the public winner of a closed raffle.

### [3.15.0] - 2026-08-11

#### Added

- Stamp rally participants can sign themselves up: a rally has a public sign-up option (off by default), and `POST /api/stamp-signup/{id}` issues a card and, when an open garapon is linked, its drawing link.
- Sign-up names are unique per rally ignoring case, a repeat gets a 409 pointing at the lookup page, and sign-up is rate limited and bot-checked.
- `POST /api/stamp-lookup` hands a participant their links back from the exact name they used, and a miss returns an empty 200 so it can't be used to discover who signed up.
- A garapon has a default number of draws (1 unless changed), used when a link is issued without a number.
- `-webhook-dry-run` / `APPSUITE_WEBHOOK_DRY_RUN=1` suppresses every outbound Discord post and `-webhook-override` / `APPSUITE_WEBHOOK_OVERRIDE` redirects them to one test webhook, so a development server on a copy of the live database can't post to production channels.

#### Changed

- "Skip next occurrence" on an announcement is now "skip the next N occurrences" (up to 52), so a Friday and Saturday announcement can sit out a whole weekend; setting 0 clears it.

#### Fixed

- Saving an announcement no longer cancels a pending skip, and multi-word emoji shortcodes such as `:video_game:` now post as the emoji in announcements, affiliates and tea rooms.

#### Security

- Updated the Go toolchain, clearing six standard-library security advisories.

### [3.14.1] - 2026-08-09

#### Added

- Every delivered Discord post is logged (`discord post sent`) with the feature, record name, status and duration, so "did that announcement actually go out?" can be answered from the log, and manual sends and publishes name who did them.
- Scheduler start and stop, bingo game start, end and half-time decisions (with the admin), and consumed announcement skips are logged.

### [3.14.0] - 2026-07-24

#### Added

- Tea rooms have an optional Room Owner line, returned by every tea-room read including the public cross-origin API.

### [3.13.0] - 2026-07-24

#### Added

- Affiliates have an optional subtitle shown under the name, in any script.

### [3.12.0] - 2026-07-21

#### Changed

- A panic in a request or background task is now logged and answered with a 500 instead of crashing the server, slow clients are timed out, and a failed start shuts down cleanly.
- WebSocket upgrades are now access-logged, and Discord posts stop when the request is canceled or the server shuts down.

#### Fixed

- A draw held for its reveal delay is now dropped if its game ended meanwhile, so a number from one game can't show up in the next, and switching auto-draw on twice at once no longer triggers a redundant draw.
- The announcement scheduler could post `@everyone` twice after a brief database hiccup following a successful post.
- Deleting a garapon player with its stamp card, or a stamp rally with its garapon links, is now all-or-nothing, and a stamp can no longer be updated through another rally's id.

#### Security

- The sign-in throttle now keys on the real client IP behind Cloudflare, so one visitor tripping it no longer affects everyone, and starting a passkey sign-in counts toward the login limit.
- Registration no longer reveals which usernames exist: a new name, a taken name and the reserved name all get the same response.
- A personal access token in a `?token=` query is accepted only for the WebSocket upgrade, and a password change or admin reset logs out the account's other sessions and revokes its token.
- Uploaded SVGs and Carrd uploads are checked more strictly, so scripts can't ride in on a crafted SVG or a renamed `.png`, `.mp3` or `.mp4`.
- A raffle entry can be marked paid or deleted only through its own raffle, and raffle staff reads now require the same `teahouse-raffles` permission as writes.
- The AniList endpoint setting can't be pointed at an internal service, font access tokens can't be forged when the server's random source is unavailable, and token links in the logs are hashed whatever their letter casing.

### [3.11.0] - 2026-07-20

#### Added

- Auto-draw: a game can draw numbers by itself, the first the moment auto is switched on and then one every interval in seconds; it is off by default, including after a restart, and a manual draw switches it off.
- Starting a game accepts auto and an interval, both adjustable live, game presets gain an auto-call setting and an interval, and the state is pushed to every admin console.

#### Changed

- Half-time is detected by the server, so any draw, manual or automatic, crossing the midpoint prompts every admin console and pauses a running auto-draw. Answering yes alerts players and leaves auto paused, answering no resumes it, and auto switches off the moment a new winner is recognized.

### [3.10.0] - 2026-07-19

#### Added

- Themes can be public or private (existing themes stay private); the theme picker lists public ones, and a private theme answers 404.
- Custom card requests: the public `POST /api/cards/request` takes a hand-built board and a chosen 6-character ID (rate limited and bot-checked) and stores it as pending until approved, rejecting a taken ID or duplicate board.
- Cards have a Protected flag and a world; approving a pending custom card makes it approved and protected, and Delete All spares protected cards.
- A custom card cost setting (gil) is shown on the public request page.

### [3.9.0] - 2026-07-16

#### Added

- The public garapon page now includes the token of the stamp rally card issued with the drawing link, so it can link to the participant's stamp card.

### [3.8.0] - 2026-07-12

#### Added

- Stamp rallies have a "Where to Redeem" image, shown on the admin and public stamp card pages.

### [3.7.1] - 2026-07-12

#### Changed

- Affiliate embed open times show as Discord short time (4:00 PM) instead of long time with seconds, still in each viewer's own time zone.

### [3.7.0] - 2026-07-12

#### Added

- Affiliates have an embed color, a Discord link, a Carrd link and a drag order that editing keeps.
- Affiliates share one admin-set Discord webhook, and an affiliate can be posted to Discord as an embed with its color, name, details, logo, screenshot, location, opening hours (in each viewer's own time zone) and links.

### [3.6.0] - 2026-07-11

#### Added

- The "It's Yoever" bingo reaction: `POST /api/game/yoever` shows a reaction with the player's name and a count to every connected client, refused when no game is active, when it is switched off, or (with a 429) while the card is on cooldown.
- Admins can switch the reaction on or off live, and it resets at the start of each game.
- A `yoever_cooldown_seconds` setting sets the per-card cooldown (default 180, 0 disables it).

#### Fixed

- Saving settings no longer rejects a Google Fonts API key as a Discord webhook, which had stopped any admin with that key set from saving any setting.

#### Security

- Updated the Go toolchain to fix a TLS Encrypted Client Hello privacy leak.

### [3.5.0] - 2026-07-10

#### Added

- Tea Rooms: bookable rooms with a name, subtitle, unique room number, per-half-hour gil cost, hashtags, description, seasonal, open, lockable and discounted flags, an image and an embed color, managed by admins with drag ordering behind the new `teahouse-tea-rooms` page permission.
- A room can be posted to Discord as an embed with its cost (halved when discounted), room number and Open/Closed status, to one shared admin-set webhook.
- A public rooms API (`GET /api/tea-rooms/public` and `/public/{number}`) needs no sign-in and can be read from any site, so an external Carrd site can show a room's live availability and pricing.

### [3.4.0] - 2026-07-07

#### Added

- Request logs identify who made the request (an `auth` field of `session`, `token`, `bot` or `anon`, plus the account's `user` name) and name Cloudflare-verified bots.

#### Changed

- The service runs as a dedicated `senpan` user instead of root.

#### Fixed

- The image category list no longer loses custom categories, and categories can't be created or renamed onto folders owned by other features.
- The default app title is "Senpan App Suite" (it was "Nifty App Suite"), so an unconfigured instance no longer shows the wrong name.

#### Security

- The live server-log tail on the admin WebSocket is now admin-only; it had reached any signed-in account and any plugin token.
- Capability tokens (garapon and stamp card draw links, font kit tokens, the `token` query parameter and the Referer) are replaced in request logs with a short non-reversible hash.
- Font family names, the header font setting and theme flourish paths are validated before they are stored, rejecting characters that could break CSS.
- The client IP in the logs honors `CF-Connecting-IP` and `X-Forwarded-For` only from the local reverse proxy, so a client can't forge it.

### [3.3.0] - 2026-07-05

#### Added

- Structured JSON logs go to stdout and, with `-log-file` (default `/var/log/senpan/senpan.log`), to a rotating file; `-log-file=""` keeps development to stdout.
- An admin log viewer: `GET /api/logs` tails the file newest-first with level, text and limit filters, and every log line is pushed live to admin WebSocket clients.
- A DEBUG toggle that needs no restart (`POST /api/logs/level`) adds a richer `request detail` line per request, and the level reverts to INFO on restart.

#### Changed

- Book club covers post as the large full-width Discord embed image instead of the small thumbnail when a reading list is published.

#### Fixed

- Request logs now record the real client address instead of the Apache proxy's.
- A failed book-club AniList lookup now returns a 424 carrying AniList's actual message instead of a 502 that Cloudflare replaced with its own error page.

### [3.1.0] - 2026-07-03

#### Changed

- There are no permanent image categories any more: the ten built-in ones can be renamed and deleted like any other.
- Anyone holding a page permission that uses images can browse the image categories and images for the shared picker; upload, delete and category management still need `system-images`.

### [3.0.0] - 2026-07-02

#### Added

- Automatic WOFF2 conversion: a font with no uploaded WOFF2 gets one converted and served by default (selectable per font), and existing fonts are converted at startup.

#### Changed

- Breaking: uploaded fonts are no longer served as static files, so legacy direct font URLs stop working; external sites use the generated stylesheet at `GET /api/fonts/pub/kit.css`, and font files are served behind a token that rotates every 7-14 days.
- Font requests are checked per font: a cross-origin site needs an origin on that font's allowlist, and `kit.css` lists only the fonts the requesting site may use. The `uploaded_fonts` list in `GET /api/settings` changed from file names to `{name, family, token}` objects.
- Breaking: fonts are grouped into families by shared base name, `GET /api/fonts` returns the grouped shape, and admins can rename a file, edit a font's CSS name, served variant and allowed origins, or delete a whole font.

### [2.2.0] - 2026-07-02

#### Added

- Passkeys: an account can register, list and delete passkeys, and sign in with one without a username.
- Public raffle entry is bot-checked when Turnstile is configured, so flooded entries can't skew the weighted winner pick.

### [2.1.0] - 2026-07-02

#### Added

- A number can no longer be drawn twice in one game; the database now enforces it as a backstop.

#### Fixed

- A server panic when a WebSocket client disconnected, a 500 on a stamp card page after one of its stamps was removed, and a crash of the admin page when editing an announcement with no buttons.
- Shutting down mid-sweep could let the announcement scheduler post again on the next start.
- A failure while starting a game could leave a half-initialized active game, and uploads could leave temporary files behind after a large batch.

#### Security

- Uploaded SVGs are sanitized before they are stored (scripts, `<foreignObject>`, event handlers and external references are dropped), and font uploads reject hidden-file names.
- The `anilist_api_url` setting is limited to hosts under `anilist.co`, and the Google Fonts API key is kept out of the public `GET /api/settings` response.
- The public raffle-entry endpoint is rate limited per IP, and a raffle that is not yet open or past its window can no longer be read by non-admins by guessing its id.
- Cross-origin state-changing requests that ride a session cookie are rejected; plugin (Bearer-token) and cookie-less requests are exempt.
- The admin WebSocket is re-authorized periodically, so an account deactivated or deleted mid-session loses its admin socket, and an enforcing Content-Security-Policy is in place.

### [2.0.0] - 2026-06-30

#### Changed

- Breaking: the HTTP API moved from action dispatchers to REST (`POST` creates, `PUT` replaces, `PATCH` updates part of a record, `DELETE` removes, commands are `POST /api/<resource>/{id}/<verb>`), needing the 2.0.0 SPA; login, logout and register keep their small action bodies.
- Reading lists moved from `/api/reading-lists?club=...` to `/api/book-clubs/{club}/reading-lists[...]`, and an id from another club answers 404, so one club's permission can't reach another club's list.

### [1.7.0] - 2026-06-30

#### Added

- An OpenAPI 3 description of every endpoint is served at `GET /api/openapi.yaml`, with a hosted reference at `GET /api/docs`; both are public.

### [1.6.0] - 2026-06-30

#### Changed

- The book club cover upload keeps the uploaded file's (sanitized) name and overwrites a same-named file, and deleting a reading-list item or list removes its cover only once no other item uses the same file.

#### Fixed

- Simultaneous requests could exceed limits: two sign-ups for the same character and world could both pass the entry limit, and the public garapon draw cap could misfire.

### [1.5.0] - 2026-06-29

#### Added

- Personal access tokens for external API clients such as the FFXIV Dalamud plugin: a request sends `Authorization: Bearer <token>`, acts as the owning account and meets the same per-page permission checks.
- `GET` and `POST /api/account/token` let an account see its token's details, generate (replacing) or revoke it; there is one token per account, shown in full only when generated.

### [1.4.0] - 2026-06-29

#### Added

- A garapon can be linked to an open stamp rally, and creating a drawing link on it also issues a stamp rally card with the same token (`stamp_card_token`).
- A stamp rally is open or closed: a closed rally is read-only, and a card with collected stamps can be deleted only when the rally is closed.
- The stamp rally list returns each rally's stamp count and active stamp count.

#### Changed

- The stamp log is kept when a card or stamp is deleted, since each entry records the participant and stall names; deleting a rally still removes its whole log.

### [1.3.0] - 2026-06-29

#### Added

- Stamp Rally: admins create events with stamps and prizes placed by percentage, pause and resume a stamp, issue tokenized participant cards and see an event-wide collection log, behind the new `festival-stamp-rally` page permission.
- The public participant flow: `GET /api/stamp-card/{token}` shows the card (prizes hidden until it completes) and `POST /api/stamp-card/{token}/stamp` collects a stamp by password, updating the admin log live. A card completes once every stamp is collected or expired.
- Three new image categories for stamp cards, stamps and prizes.

### [1.2.0] - 2026-06-29

#### Added

- Affiliates: admins can manage partner establishments (`GET` and `POST /api/affiliates`) with owners and opening hours behind the new `teahouse-affiliates` page permission, with two new image categories, Affiliate Logos and Affiliate Images, for the logo and screenshot pickers.

### [1.1.1] - 2026-06-28

#### Added

- Cloudflare Turnstile is also checked on registration to stop mass sign-ups, enforced only when a secret is set, like login.

### [1.1.0] - 2026-06-28

#### Added

- Cloudflare Turnstile on login: when `APPSUITE_TURNSTILE_SECRET` (flag `-turnstile-secret`) is set, `POST /api/auth` requires a valid Turnstile token and refuses on any failure; it is skipped when no secret is set.
- `GET /api/config` is a public endpoint exposing the non-secret Turnstile site key (flag `-turnstile-sitekey` or `APPSUITE_TURNSTILE_SITEKEY`) for the login page.

### [1.0.1] - 2026-06-28

#### Fixed

- Scheduled announcements could post more than once: one overdue by more than a period re-posted on every scheduler tick, and a webhook call that timed out was retried even though Discord may have received it.

### [1.0.0] - 2026-06-28

#### Added

- `GET /api/version` is a public endpoint returning the backend's version.
- Token-based theming: themes are stored as a structured set of design tokens, and the stylesheet is generated server-side from an allowlist of tokens.
- Live admin updates: after an admin change, open admin pages are told to refresh, including garapon draws and raffle entries.
- Garapon, the festival lottery drum, with admin management and a tokenized public draw.
