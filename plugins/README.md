# FFXIV plugins

Dalamud plugins that act as a **second UI** over the Senpan App Suite server. They
are pure API clients: the Go backend stays the single source of truth, and every
action a plugin takes still broadcasts to the website over the server's WebSocket.

## Senpan Admin Companion

Display name **Senpan Admin Companion**; Dalamud internal name / DLL is
**`SenpanCompanionAdmin`** (the folder and csproj keep the shorter `SenpanCompanion`
name). Its panels:

- **Bingo Game** — run a game with the same controls as the web admin: start from a
  **preset** or hand-pick win patterns (collapsible, by category), edit **game
  details** (pre-filled from the last game), draw numbers, and watch the live
  **B-I-N-G-O called-numbers grid**, last-drawn, winners (card ID + player name),
  and active patterns. The **half-time** prompt pops up automatically at the same
  scaled mid-point the website uses, and a short **chime** plays when a new winner
  appears. Draws/winners arrive over the WebSocket, so it stays in sync even when
  the website drives the game.
- **Bingo Cards** — create named cards (with a "Nearby…" picker that fills the name
  from visible players, and an optional /tell of the card URL), copy a card's URL,
  and delete cards.
- **Bingo Winners** — the winners log (player, card, time, winning patterns) with
  per-entry delete (no clear-all, so it can't be wiped from in-game).
- **Raffles** — pick an open raffle, add entrants (with the nearby picker), toggle
  their paid status, and draw a winner (pick → confirm, or draw another). Raffle
  *creation* stays on the website by design.

Everything is gated by your account's permissions on the server: a token can only
do what its owner can (`bingo-cards`, `bingo-game`, `teahouse-raffles`).

### Requirements

- **XIVLauncher + Dalamud** installed (the build references the Dalamud assemblies
  from `%AppData%\XIVLauncher\addon\Hooks\dev\`).
- **.NET 10 SDK** (matches current Dalamud). The project uses the
  `Dalamud.NET.Sdk` MSBuild SDK, which supplies the target framework and Dalamud
  references — no NuGet Dalamud packages to add.

### Build

```sh
cd plugins/SenpanCompanion
dotnet build -c Release
```

The loadable plugin is written to `bin/Release/` as **`SenpanCompanionAdmin.dll`** +
`SenpanCompanionAdmin.json`, and DalamudPackager also produces the installable
**`bin/Release/SenpanCompanionAdmin/latest.zip`** (the file a custom repo serves).
`bin/` and `obj/` are git-ignored.

### Install as a dev plugin

1. In game, open **Dalamud Settings** (`/xlsettings`) → **Experimental**.
2. Under **Dev Plugin Locations**, add the full path to the built
   `SenpanCompanionAdmin.dll` (e.g. `…\plugins\SenpanCompanion\bin\Release\SenpanCompanionAdmin.dll`).
3. Open the **Dev Plugins** section of the plugin installer (`/xlplugins`) and
   enable **Senpan Admin Companion**.

### Configure & use

1. On the website, generate a token: **User Options → Access Token → Generate**.
2. In game, run **`/senpan`**. Before a token is set, the window shows a setup panel —
   paste the token and **Save & Connect**. (The server URL defaults to
   `apps.senpan.cafe`; only change it if you're self-hosting/dev.)
3. The data tabs populate automatically — no manual refresh — and a green **● Live**
   badge means the WebSocket is connected. Change the token/URL or the toggles any
   time on the **Settings** tab (there's no separate settings window).

### Custom plugin repository (pluginmaster.json)

`pluginmaster.json` (in this folder) is the repo index Dalamud reads to install and
update the plugin. A release hosts two files publicly:

- `pluginmaster.json` — the index.
- `SenpanCompanionAdmin/latest.zip` — the packaged plugin (from `bin/Release/`).

**Recommended hosting — the app server** (it already serves the icon publicly, so
this needs no extra infrastructure):

1. Build Release (above), then grab `bin/Release/SenpanCompanionAdmin/latest.zip`.
2. On the host, drop the files under the Apache web root:
   - `/var/www/apps.senpan.cafe/plugin/pluginmaster.json`
   - `/var/www/apps.senpan.cafe/plugin/SenpanCompanionAdmin/latest.zip`
3. In game: **Dalamud Settings → Experimental → Custom Plugin Repositories**, add
   `https://apps.senpan.cafe/plugin/pluginmaster.json`, save, then install
   **Senpan Admin Companion** from `/xlplugins`.

The `DownloadLink*` URLs in `pluginmaster.json` already point at those paths.

**Alternative — GitLab.** Raw URLs work only if the repo (or those file paths) are
**publicly readable**, e.g.
`https://gitlab.com/mathdad-apps/senpan-app-suite/-/raw/main/plugins/dist/SenpanCompanionAdmin/latest.zip`.
If the repo is private, host via **GitLab Pages** or a **public Release asset**
instead, and update the `DownloadLink*` URLs (and the repo URL you add to Dalamud)
to match.

**On each release:** bump `<Version>` in the csproj, rebuild, re-upload `latest.zip`,
then in `pluginmaster.json` bump `AssemblyVersion`, set `Changelog`, and refresh
`LastUpdate` (Unix seconds — `date -u +%s`). `DownloadCount` is a static field;
custom repos don't auto-track it. Bump `DalamudApiLevel` only when you rebuild
against a newer Dalamud SDK (it's currently **15**).

### Notes

- **Permission-aware.** Each tab appears only if your account holds the matching
  permission (Bingo Game ↔ `bingo-game`, Cards ↔ `bingo-cards`, Winners ↔
  `bingo-winners-log`, Raffles ↔ `teahouse-raffles`); admins see everything and the
  Settings tab is always available. This mirrors the website, which hides nav
  sections you can't access — and the server enforces the same permissions on the
  token regardless. An account with no Senpan permissions sees only Settings.
- The token is stored locally in the plugin's Dalamud config and sent only to your
  server as a Bearer credential (and, for the live WebSocket, as a `?token=` query
  parameter on the `/api/ws` upgrade).
- This talks to your own backend, so it is intended for distribution via a **custom
  Dalamud repository**, not the official plugin list. The nearby-player picker only
  reads visible character names + home worlds (the same fields a player would type
  to enter) and sends nothing until you pick someone and submit.
- **Auto-/tell (opt-out).** Creating a card for a player picked from the nearby list
  sends them a `/tell` with the card URL. This is the one feature that issues an
  outgoing chat message on your behalf — the kind of automation the *official*
  Dalamud repo discourages — so it's deliberately a one-shot, operator-initiated
  action (never a loop) and can be turned off under **Settings**.
