# Deployment (Apache)

This folder holds what the host runs beside the binary: the Apache vhost the deploy
installs (`apache-vhost.conf`), the systemd unit's template (`senpan.service`) and the
persistent `images/` a new host's data folder starts from. The app is one Go binary that
serves everything: the API, its live event streams, the SPA it embeds, and the public
files of its data folder (the image library, the Carrd projects and the plugin
repository). Apache terminates TLS and proxies every path of every name to it; nothing is
served from a document root.

## The data folder

```text
/opt/senpan/data/              <- the -data on the unit's ExecStart, beside the database
+-- database.sqlite            <- the -db (and its -wal/-shm)
+-- backups/                   <- the daily database backups (Install-BackupTimer)
+-- images/                    <- PERSISTENT: served at /images/ (from deploy/images/ on a new host)
|   +-- logo.png, favicon.png, apple-touch-icon.png, share_banner.png
|   +-- pwa-192x192.png, pwa-512x512.png, pwa-maskable-512x512.png   <- the PWA's icons
|   +-- raffles/, announcements_main/, announcements_thumb/, garapons/, affiliate_logos/,
|   |   affiliate_images/, stamp_cards/, stamp_stamps/, stamp_prizes/, flourishes/
|   |                          <- the seeded image categories (System -> Images), and any added
|   +-- .categories.json       <- the categories' manifest (never served)
|   +-- announcements/         <- legacy announcement images; copied into announcements_main on first start
|   +-- bookclub/              <- reading-list cover uploads (reserved: not an image category)
|   \-- ema_plaques/           <- ema plaques awaiting review (reserved: not an image category)
+-- fonts/                     <- PERSISTENT: uploaded fonts, never served as files (see Font host)
|   \-- .woff2/                <- the converted WOFF2 copies
+-- carrd/                     <- PERSISTENT: the Carrd projects, served at carrd.senpan.cafe (see Carrd host)
+-- plugin/                    <- the Dalamud custom repo, replaced whole by each plugin publish (previous kept as plugin.old)
\-- sounds/                    <- the game's sound effects, copied in by hand (see below); served at /sounds/
```

The app serves `/images/<path>` (the library's picture types: jpg, png, webp, gif, svg),
`/carrd/<path>` (those and mp3, mp4) and `/plugin/pluginmaster.json` and
`/plugin/SenpanCompanionAdmin/latest.zip`, and `/sounds/<name>.mp3`, from the data folder
(`internal/media`, `serve.go`). It never serves a dotfile (the manifest, a project's `.carrd.json`), never
lists a folder, never follows a link out of one, and has no route that reads `fonts/`.
Pictures, Carrd files and sounds are cached a day (`public, max-age=86400`): a picture replaced
under its name can show the old one until then unless purged at Cloudflare. The plugin's
two files are `no-store`. A missing file is a 404 that nothing caches. Every stored path
and public URL is the one it was when Apache served these from the document root.

A deploy never touches the data folder (the plugin publish swaps only `plugin/`). The
build strips its own `images/` and `sounds/`, so the binary never carries a copy of either.

**The sounds** (`level_up`, `moogle_noise`, `queue_pop` and `this_is_bad`, each `.mp3`) are
the player's "game" sounds and the Yoever reaction (`frontend/src/lib/sound.ts`). They are
kept out of the repository (`frontend/public/sounds/` is git-ignored, and holds them for
`vite dev`), so no release carries them: copy them into the data folder once, by hand, from
a machine that has them:

```powershell
plink -i "<KeyPath>" <user>@<host> "install -d -m 755 /opt/senpan/data/sounds"
pscp -i "<KeyPath>" frontend\public\sounds\*.mp3 <user>@<host>:/opt/senpan/data/sounds/
```

Readable is enough: the app never writes them. Until they are there, `/sounds/*.mp3`
answers 404 and the sounds stay silent; nothing else breaks.

## The vhost

`apache-vhost.conf` holds the three names' VirtualHosts, with the app's address filled in
from the unit's `-addr`; every app deploy installs it as
`/etc/apache2/sites-available/<service>.conf` where it differs, checks it with `apachectl
configtest` before anything stops (the previous is kept as `.old` and put back if Apache
refuses it), and reloads Apache once the new release answers. Edit it in the repo, never
on the host.

| Name | Proxied to |
| --- | --- |
| `apps.senpan.cafe` | the app, every path (`timeout=3600` for the event streams) |
| `fonts.senpan.cafe` | `/api/v1/fonts/pub/`: `kit.css` and `/f/<token>` |
| `carrd.senpan.cafe` | `/carrd/`: `<project>/.../<file>` |

Port 80 redirects all three to https. All three use the certificate the `CertName` setting
names (`/etc/letsencrypt/live/<CertName>/`). `mod_deflate` is kept off the event streams
(`SetEnvIfNoCase Request_URI "/events$" no-gzip`), or their events would arrive in
batches. Apache needs `mod_proxy`, `mod_proxy_http`, `mod_headers`, `mod_rewrite`,
`mod_setenvif` and `mod_ssl`.

## One-time server setup

For a new host (the live one already has all of it):

1. The service's account and folders: `useradd --system --no-create-home --shell
   /usr/sbin/nologin senpan`, then `install -d -o senpan -g senpan -m 755 /opt/senpan
   /opt/senpan/data`, and copy `deploy/images/` to `/opt/senpan/data/images/`, owned by
   `senpan`.
2. The unit: copy `deploy/senpan.service` to `/etc/systemd/system/senpan.service`, fill in
   its placeholders, then `systemctl daemon-reload && systemctl enable senpan`; the
   first app deploy installs the binary and starts it.
3. The vhost: the first app deploy installs `senpan.conf` in `sites-available`; enable it
   with `a2ensite senpan`, then `apachectl configtest && systemctl reload apache2`.
4. The sounds: copy them into `/opt/senpan/data/sounds/` (see [The data folder](#the-data-folder)).

## Running as a non-root user

The standards run each service as its own locked system account, and the repo's
`deploy/senpan.service` template does (`User=` and `Group=`, `<service-user>` there).
Nothing in the server needs root - it binds `:8080` (non-privileged) and only touches
ordinary files under `/opt/senpan` - so a dedicated account shrinks the blast radius of any
compromise.

> **Important:** `scripts/deploy.ps1` does **NOT** ship the systemd unit - it updates the
> binary and the vhost, publishes the plugin folder, and installs the daily backup's own
> units. It **reads** the running unit (its `User=`, `Group=`, `-db`, `-addr` and
> `-data`), so the binary it installs is owned by that account, as the standards have it,
> and the backup runs as it too. The repo `deploy/senpan.service` is a template; the unit
> that actually runs lives on the host at `/etc/systemd/system/senpan.service`, and a
> change to it is made there by hand (the one exception was 5.2.0's move, which edited
> its `-webroot` and `ReadWritePaths` lines once).

Everything the process writes is under `/opt/senpan`, owned by `senpan`, so
`ReadWritePaths=/opt/senpan` covers it all. Verify:

```bash
systemctl show senpan -p User -p Group -p ReadWritePaths   # senpan / senpan / /opt/senpan
ps -o user= -C app-suite                                   # not root
# Smoke-test each writable folder from the admin UI: upload an image, a font (confirm
# fonts/.woff2/ is written) and a Carrd project file; confirm an admin write succeeds
# and /var/log/senpan/senpan.log keeps growing.
```

## Credential rotation

- **Admin (and pre-existing staff) passwords.** Rotate the admin password to a
  fresh high-entropy value via the app's change-password flow, plus any staff
  accounts that predate a known exposure. A password change or reset logs that
  account's other sessions out (each session records the account's password
  epoch) and revokes its personal-access token.
- **Sessions.** Sessions are held in the server's memory, so restarting the
  service (`systemctl restart senpan`) ends every session at once - that is how to
  force every browser to log in again. The FFXIV plugin uses PAT bearer tokens,
  not the admin cookie, so it is unaffected.
- **`APPSUITE_SESSION_SECRET` - keep it set.** This version reads nothing from it
  (sessions are in memory and signed with nothing, so rotating it never logged
  anyone out), and the `-secret` flag is gone: a unit still passing `-secret`
  fails to start, so keep it off `ExecStart`. But the Discord bot release seals the
  bot's token under a key derived from this value, so leave it in
  `/etc/senpan/senpan.env`, unchanged, ready for that release. Changing or removing
  it after the bot ships would leave the stored token unreadable.
- **`APPSUITE_TURNSTILE_SECRET` / `APPSUITE_TURNSTILE_SITEKEY`.** Configure the
  Cloudflare Turnstile pair (also settable as `-turnstile-secret`/`-turnstile-sitekey`
  flags) to enable the bot check on the public POST paths - login, registration,
  raffle sign-up, and custom card requests. When the **secret** is empty the check
  is disabled entirely (the server logs `Turnstile not configured` at boot and the
  frontend skips the widget), so a bad/rotated secret degrades to "no bot check,"
  not a lockout. The **secret** is sensitive; the **sitekey** is public (it is
  served to browsers via `GET /api/v1/config`). Keep the secret out of the unit's
  inline `Environment=` (visible via `systemctl show` / `/proc/<pid>/environ`) -
  put both in a root-owned `chmod 600` file
  loaded with `EnvironmentFile=-/etc/senpan/senpan.env` (see the commented block in
  `deploy/senpan.service`). Rotate the secret from the Cloudflare dashboard and
  update the env file.

## Font host (protected serving)

Uploaded fonts are licensed assets, so they are **never served as files**: they live in
the data folder's `fonts/`, which no vhost names and no route of the app reads. The Go
server streams them itself through obfuscated token URLs that rotate every 1-2 weeks,
gated by the requesting site's `Origin` against **that font's own allowlist** (Atelier ->
Font Upload -> Edit -> Allowed sites). Chrome and Firefox hard-require CORS on
cross-origin `@font-face`, so a site that isn't on a font's list cannot render it. Safari
is the exception: it fetches fonts in `no-cors` mode, sending **no** `Origin`, so for an
Origin-less request the gate checks the site in the `Referer` (the embedding page)
against the same allowlist instead. Requests naming no site at all (pasting the URL into a
browser, `wget`, casual scrapers) get a 403, and `kit.css` only emits each requesting
site's allowed fonts. The SPA itself loads fonts **same-origin**
(`/api/v1/fonts/pub/f/<token>`), so the app and its font picker always work - the
allowlists only govern external sites.

External (Carrd) sites embed one stylesheet whose URL never changes:

```html
<link rel="stylesheet" href="https://fonts.senpan.cafe/kit.css">
```

and then use `font-family: '<CSS Name>'` (shown per font on the Font Upload tab; defaults
to the filename without extension). The kit's tokenized font URLs refresh automatically on
rotation. `fonts.senpan.cafe` is the vhost's proxy onto `/api/v1/fonts/pub/`, so
`https://fonts.senpan.cafe/kit.css` is `/api/v1/fonts/pub/kit.css` and
`https://fonts.senpan.cafe/f/<token>` is `/api/v1/fonts/pub/f/<token>`.

Files sharing a base name (e.g. `Jasper.otf` + `Jasper.woff2`) are format variants of
**one font**. A font with no uploaded WOFF2 gets one **auto-converted** (stored in the
hidden `fonts/.woff2/` sub-folder; a startup backfill converts pre-existing fonts and
sweeps stale copies). The WOFF2 is served by default - uploads stay on disk untouched, and
the served version is selectable per font in the Edit modal.

**Checking it** (from any machine):

```bash
# The gated responses MUST carry Cache-Control: private, or Cloudflare (which ignores
# Vary) can serve one site's kit to another.
curl -sI https://fonts.senpan.cafe/kit.css | grep -i cache-control                      # -> private
curl -s -H 'Referer: https://<allowed-site>/' https://fonts.senpan.cafe/kit.css          # -> that site's fonts
curl -s -o /dev/null -w '%{http_code}\n' -H 'Origin: https://<allowed-site>' https://fonts.senpan.cafe/f/<token>   # -> 200
curl -s -o /dev/null -w '%{http_code}\n' -H 'Referer: https://<allowed-site>/' https://fonts.senpan.cafe/f/<token> # -> 200 (Safari)
curl -s -o /dev/null -w '%{http_code}\n' -H 'Origin: https://elsewhere.example' https://fonts.senpan.cafe/f/<token> # -> 403
curl -s -o /dev/null -w '%{http_code}\n' https://fonts.senpan.cafe/f/<token>             # -> 403 (no site named)
curl -s -o /dev/null -w '%{http_code}\n' 'https://apps.senpan.cafe/fonts/<File>.ttf'     # -> 404 (never a file)
```

## Carrd image host

The **Atelier -> Carrd Upload** admin tab creates per-project folders under the data
folder's `carrd/` (each of which may contain arbitrarily nested sub-folders) and writes
uploaded pictures, `.mp3` and `.mp4` files to them. They are served at
`https://carrd.senpan.cafe/<project>/.../<file>`, the vhost's proxy onto the app's
`/carrd/`, and embedded by external Carrd sites, so every file goes out with
`Access-Control-Allow-Origin: *` and a day's cache; audio and video answer range
requests. A project's `.carrd.json` (its title) is never served, and neither is a folder
listing. A file uploaded under a name already there replaces it, so a site referencing
its address picks up the new one (within the day's cache, or at once after a Cloudflare
purge).

## Plugin repository

The FFXIV plugin's custom Dalamud repository is `https://apps.senpan.cafe/plugin/pluginmaster.json`,
whose download links are `https://apps.senpan.cafe/plugin/SenpanCompanionAdmin/latest.zip?v=<version>`.
The app serves those two files from the data folder's `plugin/`, read on every request and
sent `no-store`: Cloudflare would otherwise keep an old `latest.zip` while the index
already names the new version, and Dalamud refuses a package whose version differs from
the index's. The `?v=` on the links is the second guard. `.\scripts\deploy.ps1 -Target
plugin` publishes the folder (see [Each deploy](#each-deploy)).

## Server logs

The Go server writes **structured JSON** logs to **stdout** (captured by
`journalctl -u senpan`) and, by default, to a rotating file at
`/var/log/senpan/senpan.log` (`-log-file`; pass `""` to disable, e.g. local dev).
timberjack rotates the file **daily at local midnight**, zstd-compresses backups,
and bounds retention (14 backups / 30 days / 100 MB safety cap). The admin
**System -> Logs** tab and the on-box `jlv` tool both read this file;
`GET /api/v1/logs` tails it and every line also streams to connected admins over
the staff event stream for a live tail, so most log inspection needs no SSH.

**Required systemd setting (`LogsDirectory`).** `senpan.service` runs with
`ProtectSystem=strict`, which mounts `/var/log` **read-only** for the service - so
the app cannot create its own log dir (it falls back to stdout-only and logs a
`file logging disabled ... read-only file system` warning). Grant it a writable log
directory with systemd's `LogsDirectory`, which creates `/var/log/senpan` (owned
by the service, writable inside the sandbox) on every start - this survives
reboots and rebuilds:

```ini
# /etc/systemd/system/senpan.service.d/logdir.conf
[Service]
LogsDirectory=senpan
```

Then `systemctl daemon-reload && systemctl restart senpan`, and verify with
`tail /var/log/senpan/senpan.log` (JSON lines). A drop-in keeps the change out of
the packaged unit; equivalently add the `LogsDirectory=senpan` line to the unit's
`[Service]` section directly. **Without this, only the live tail / journald work -
the on-disk file, the Logs tab's historical snapshot, and `jlv` stay empty.**

On-box viewing: `jlv /var/log/senpan/senpan.log` (install the Linux `.deb` from
<https://github.com/hedhyw/json-log-viewer/releases>).

### Systemd sandboxing

`senpan.service` ships with a defense-in-depth sandbox: `NoNewPrivileges`, `ProtectSystem=strict` + `ProtectHome`, `PrivateTmp`/`PrivateDevices`, `ProtectKernel*`/`ProtectControlGroups`, `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX`, `RestrictNamespaces`, `LockPersonality`, `SystemCallFilter=@system-service` + `SystemCallArchitectures=native`, and **`MemoryDenyWriteExecute=yes`** (W^X). Everything the app writes is under its opt folder (the database and the data folder's `images/`, `fonts/` and `carrd/`), so `ReadWritePaths=/opt/<app>` covers it, with `LogsDirectory=senpan` for the log; a path outside `ReadWritePaths` fails with `EACCES` under `ProtectSystem=strict`.

> **`MemoryDenyWriteExecute` caveat.** W^X is safe here because the binary is pure
> Go (no cgo, `CGO_ENABLED=0`) and so is its SQLite driver: `modernc.org/sqlite` is
> SQLite's C translated to Go ahead of time, compiled into the binary like the rest
> of it - there is no runtime JIT and no writable-executable memory. If the backend
> ever takes a driver that generates code at run time (a WebAssembly build of SQLite
> run under a JIT, say), `MemoryDenyWriteExecute=yes` will crash the service with
> `SIGSYS` on the first query and must be removed. Verify on the box with
> `systemctl show senpan -p MemoryDenyWriteExecute -p SystemCallArchitectures`.

### Who made each request (actor identity)

Every request line names the actor via an `auth` field (`session` | `token` |
`bot` | `anon`) plus `user` (account username) and `bot` (crawler) when they
apply. Admin actions resolve through the cookie session and plugin actions
through the personal access token - both automatic, no configuration. The
**System -> Logs** tab surfaces this in a **User** column.

**Enabling verified-bot names (Cloudflare).** Anonymous requests are labelled
`bot` only when Cloudflare tells the origin the request is a *verified* good bot
(Googlebot, Bingbot, GPTBot, ...). Cloudflare does not send that signal by default;
enable it one of two ways:

- **Any plan** - add a **Request Header Transform Rule**: Cloudflare dashboard ->
  your zone -> **Rules -> Transform Rules -> Modify Request Header -> Create rule**.
  Set the filter to the `cf.client.bot` field (UI: *Verified Bot* *equals*
  *True*) and the action to **Set static** header `X-Verified-Bot` = `true`.
  Deploy. The backend reads `X-Verified-Bot` and names the bot from its (now
  Cloudflare-verified, so trustworthy) User-Agent.
- **Enterprise + Bot Management** - enable the **"Add bot protection headers"**
  managed transform (Rules -> Managed Transforms). The backend also reads its
  native `cf-verified-bot` / `cf-verified-bot-category` headers, giving a cleaner
  category name - no code change.

This is a logging hint, never a security decision: like `CF-Connecting-IP`, the
header is forgeable by a client that reaches the origin without going through
Cloudflare. Lock the origin to Cloudflare's IP ranges (or Authenticated Origin
Pulls) if you need it to be tamper-proof. Capability tokens in request
paths/queries/Referer are redacted to a short correlation hash before logging, so
draw/stamp/font links never appear verbatim in the log or the viewer.

## Deploy day: 5.2.0

5.2.0 moves everything out of Apache's document root, and the deploy does the move itself:
the first `task deploy` of 5.2.0 finds `-webroot` on the unit and, in the same run, moves
`images/`, `fonts/`, `carrd/` and `plugin/` into `<RemoteOptDir>/data`, edits the unit to
`-data` and its `ReadWritePaths` to the opt folder, and swaps the six apps/fonts/carrd site
files for the one vhost (see [Each deploy](#each-deploy), "The move"). Every public URL
stays as it is, `fonts.senpan.cafe/kit.css` and its token links included. In order:

1. **Push and deploy.** Add `$DeployCertName` (the certificate the three names use) to
   `scripts/deploy.config.ps1` first. Once CI is green on `main` it tags `v5.2.0`;
   `task deploy` ships it.
   Watch for the backups line (`/root/senpan-before-data-move-<UTC>/`), the four `Moved`
   lines, the unit's `-data` line, `Apache took the new sites`, the font check (every line
   `asked 404`; a 400 or 403 also counts as refused) and the smoke checks. Any failure puts 5.1.0, the folders, the unit and the
   old sites back on its own. The plugin step skips: 4.0.0.0 is published already.
2. **Copy the sounds** into `/opt/senpan/data/sounds/` (see [The data folder](#the-data-folder)),
   before or after the deploy: the data folder is already there, and the move leaves a
   `sounds/` in it alone. Then `/sounds/this_is_bad.mp3` answers 200 (`audio/mpeg`).
3. **Check from outside:** a font link with an allowed site's `Origin` (200, echoed), with
   its `Referer` alone, as Safari sends (200), with another site's `Origin` (403) and with
   neither (403); a font file's name under `/fonts/` on each name (404); a picture under
   `/images/` (200); a Carrd file with any `Origin` (200, `Access-Control-Allow-Origin: *`)
   and its project's `.carrd.json` (404); `/plugin/pluginmaster.json` (200, `no-store`); the
   tea-room API with `Origin: https://senpan.cafe` (the origin echoed); `/healthz`.
4. **Check by hand:** a Carrd page shows its fonts (in Chrome and on an iPhone) and its tea
   rooms; a board with game sounds on plays them on a draw; an upload to Images, Carrd
   Upload and Font Upload each lands; System -> Logs shows
   no `a public file could not be opened`.
5. **Once 5.2.0 has stayed:** the old document root is read by nothing. Remove its `dist/`,
   `dist.old/`, `.htaccess`, `.htaccess.old`, `.deploy-backup/` and `plugin.old/`, move
   `themes.json` and `insert_styles.php` (`scripts/themes/`) to wherever they are run, and
   remove the folder; delete the six disabled site files from `sites-available`; remove the
   inert `fonts/.htaccess`, `carrd/.htaccess` and `plugin/.htaccess` from the data folder;
   and, when nothing needs the way back, the backups folder and
   `~/.shared-deploy/senpan-data-move/`.

**Rolling the move back by hand**, after the deploy finished (on the host, as root). 5.2.0
adds no migration, so 5.1.0 runs on the database as it is:

```bash
state=/root/.shared-deploy/senpan-data-move
systemctl stop senpan
install -m 755 -o senpan -g senpan /opt/senpan/app-suite.old /opt/senpan/app-suite
for d in $(cat "$state/moved"); do mv "/opt/senpan/data/$d" "/var/www/apps.senpan.cafe/$d"; done
cat "$state/unit.orig" > "$(cat "$state/unit")" && systemctl daemon-reload
a2dissite senpan && a2ensite $(cat "$state/sites")
apachectl configtest && systemctl reload apache2
systemctl start senpan
```

The fonts go back with their `fonts/.htaccess` (`Require all denied`), which the move never
removes. The host's record still names v5.2.0, so the next deploy skips it unless `-Force`.

## Each deploy

**Scripted (recommended).** From the repo root, on Windows. Only releases ship: the app's newest `v<version>` tag CI made on main, the plugin's newest `Plugin-v<version>` (the tags are fetched first). Each is checked out into a folder of its own under `%TEMP%` and built there, with `npm ci` from its own lockfile, so commits after it, a shared-tooling update or anything uncommitted are never part of it (shared-tooling's newest-release deploy). `task deploy` deploys the app, then the plugin with `-IfNewer`, which skips it when its version is already published. `task deploy:backend` (the app) and `task deploy:plugin` deploy one each, with flags after `--`. The script itself:

```powershell
.\scripts\deploy.ps1                  # the app's newest release (skipped when the host already runs it)
.\scripts\deploy.ps1 -Force           # the same release again
.\scripts\deploy.ps1 -Target plugin   # the plugin's newest release, to the Dalamud custom repo
```

The script is built on shared-tooling's SharedDeploy module (`shared/deploy/pwsh/`, from the `shared/` submodule; its README describes each function), so it takes the shape every deploy there takes: one archive, one SSH connection held for the whole run, one remote run.

> **First-time setup:** the script is version-controlled, but **no** environment-specific value is baked into it - the host, SSH user, key path, service name, opt dir and certificate all live outside the repo. Copy `scripts/deploy.config.example.ps1` to `scripts/deploy.config.ps1` (untracked) and fill in the six required `$Deploy*` values. Each can instead be passed as a `-param` or set via its `$env:DEPLOY_*` variable (`DEPLOY_VPS_HOST`, `DEPLOY_VPS_USER`, `DEPLOY_KEY_PATH`, `DEPLOY_SERVICE_NAME`, `DEPLOY_REMOTE_OPT_DIR`, `DEPLOY_CERT_NAME`); the script fails fast, listing any that are unset, before it builds or connects. `CertName` is the certificate the vhost uses, its folder's name under `/etc/letsencrypt/live`; a plugin deploy doesn't need it. The site's address, which the smoke check fetches, is public already (`plugins/pluginmaster.json` links to it), so it defaults to `https://apps.senpan.cafe`; `-SiteUrl`, `DEPLOY_SITE_URL` or `$DeploySiteUrl` overrides it. `-ConfigPath` points it at another settings file. The live database and the data folder need no setting: they are read from the running unit (step 1). `RemoteDbPath` (`DEPLOY_REMOTE_DB_PATH`, `$DeployRemoteDbPath`) is only a cross-check, used on its own when the unit names no `-db`. A `$DeployWebRoot` left in an older `deploy.config.ps1` is ignored.

**What an app deploy does:**

1. **Read the service on the host**, before anything is built. The unit must be installed. Its `User=` (and `Group=`) is who the backup runs as; the `-db` on its `ExecStart` is the live database; its `-addr` is where the app answers on the host (`:8080`, the binary's default, when it has none, asked on `127.0.0.1`), which `/healthz` is asked on and the vhost proxies to; its `-data` is the data folder. A `RemoteDbPath` that differs from `-db` stops the run with nothing changed. The host also says which release it last took (`~/.shared-deploy/senpan`): when that is this release, the run ends there, unless `-Force`.
2. **Build the release, in its own checkout.** The front end first (`npm run build` in `frontend/`: vue-tsc + vite, into `backend/internal/web/dist/`), then a static `linux/amd64` binary that embeds it (`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o app-suite .` in `backend/`). The release's `deploy/apache-vhost.conf` is filled in with the app's address and `CertName`. A broken build changes nothing. Both go into **one** `.tar.gz`: a truncated archive refuses to unpack, where a half-copied folder can look whole.
3. **Upload** the archive as one stream through plink, into the SSH user's home.
4. **One remote run.** The host checks the archive's SHA-256 and file count, and only then: installs the vhost as `/etc/apache2/sites-available/<service>.conf` where it differs and checks it with `apachectl configtest` (a refusal puts the previous back and stops the run with nothing else changed); stops the service, keeps `app-suite.old`, snapshots the database with `sqlite3 .backup`, checked with `PRAGMA integrity_check`, compressed with gzip and checked with `gzip -t`, to `<db>.pre-<yyyyMMdd-HHmmss>.gz` (UTC; readable by root only; the newest three are kept), installs the binary, owned by the account the unit runs as, starts the service and waits up to 30 s for `/healthz` to answer on the host. If it does not, the previous binary and vhost go back with the snapshot and restart: the new binary may already have migrated the database, and the previous one refuses a newer schema (`database schema vN is newer than this binary`). Once it answers, Apache reloads if the vhost changed. Brief downtime (a few seconds) while the service is stopped. The data folder is never touched.
5. **Smoke check from this machine, through Cloudflare**, each URL tried three times: `<SiteUrl>/healthz` and `<SiteUrl>/api/v1/version` must name this release's version; `<SiteUrl>/` must name the build's entry script, which must load too; `/images/pwa-512x512.png` and `/plugin/pluginmaster.json` must answer. If any fails, the previous binary and vhost go back with the pre-deploy snapshot, so it starts on the schema it knows. Whatever was written in the seconds the new release ran is lost with it (decided 2026-11-06, as Kindred Nights does). Once the checks pass, the host records the release.
6. **Daily backup**, once the smoke check passes: see [Database backups](#database-backups). If it cannot be installed, the run fails saying the site is live but its backup is not.
7. **Dev-database pull** (unless `-NoDbPull`): an online `sqlite3 .backup` on the host (the service keeps running), downloaded and checked against the host's SHA-256 into `devdata/database.sqlite`. The previous local copy is kept as `database.sqlite.bak-<time>` (the newest three), stale `-wal`/`-shm` files are removed, and the host's copy is deleted. While a local dev server holds the database open, the pull is skipped with a warning. These copies hold **live** data (accounts, password hashes, tokens, webhook URLs) - keep them local.

The checkout under `%TEMP%` is removed at the end, whether the run passed or not.

**The move** (a unit that still passes `-webroot`, which is how 5.2.0 found the live one). The same run also:

- **before anything stops, changing nothing:** checks that the web root's `images/` is there, that the data folder (`<RemoteOptDir>/data`) holds none of the four folders, that the unit passes `-webroot` and no drop-in sets `ExecStart` or `ReadWritePaths`, that the sites now serving the vhost's names (found with `apachectl -S`) serve no other name, that Apache takes the vhost in their place (tried on a copy of `/etc/apache2`), and that there is room; then backs up each folder, all of `/etc/apache2`, the unit and the web root's `.htaccess`, each a `tar.gz` checked with `gzip -t`, into `~/senpan-before-data-move-<UTC>/` (mode 0700), and records what a rollback needs in `~/.shared-deploy/senpan-data-move/`;
- **after the stop:** moves each folder whole into the data folder (a rename on one filesystem; the fonts with their `.woff2/` copies), edits the unit (`-webroot <web root>` becomes `-data <data folder>`, the web root's paths leave `ReadWritePaths`, which keeps or gains one covering the data folder) and checks it with `systemctl show`, disables those sites, enables the vhost and runs `apachectl configtest`. A failure from here to the start moves everything back before the previous release restarts;
- **after the start:** reloads Apache (graceful), then asks every name the vhost serves, through Apache on the host, for a real font file by four paths; anything but a refusal (400, 403 or 404, redirects followed; no answer is not a refusal), or a `fonts/` still in the web root, rolls the whole run back, as a failed smoke check does too: the previous binary and snapshot, the folders, the unit and the old sites, Apache reloaded.

**What a plugin deploy does.** It checks out the plugin's newest release, reads the published `<SiteUrl>/plugin/pluginmaster.json` and stops before building when the release's version is not higher, since Dalamud updates to nothing else (with `-IfNewer`, the same version ends the run without error; if the index can't be read, the host's own check still stands). Then `dotnet build -c Release` from clean (its `bin/` and `obj/` deleted first, so the package can never lag the manifest), and its repository laid out: `SenpanCompanionAdmin/latest.zip` and `plugins/pluginmaster.json` with `?v=<version>` on its download links. The built version must equal `pluginmaster.json`'s `AssemblyVersion`. On the host, the data folder is read from the unit, the whole `plugin/` folder in it is swapped in, owned by the service's account (the previous kept as `plugin.old`), and the published index must then carry the new `?v=`, or the previous folder goes back.

**Options.** `-Force` deploys the app's release even when the host already runs it. `-NoBackup` removes `app-suite.old`, the previous vhost and `plugin.old` once the host run succeeds; the smoke check then has nothing to go back to, so a failure only stops the run. The database snapshot is still taken. The run that moves the files out of the web root refuses `-NoBackup`, since that move is undone through the previous release. `-NoDbPull` skips step 7.

**The connection.** The droplet runs `ufw limit ssh`, which refuses an address after six connections in 30 s. The whole run is **one** SSH connection, held by a background `plink -share`; every command, the upload and the download ride it. If it drops, the module reconnects (one new connection) and retries the step once, resuming where it stopped; only a refused reconnect waits 35 s. Each part of the move is safe to run again.

**Rolling back by hand** (on the host): the vhost with `mv -f /etc/apache2/sites-available/<service>.conf.old /etc/apache2/sites-available/<service>.conf && apachectl configtest && systemctl reload apache2`; the plugin with `mv plugin plugin.failed && mv plugin.old plugin && rm -rf plugin.failed` in the data folder; the binary with `systemctl stop <service> && install -m 755 -o <user> -g <group> app-suite.old app-suite && systemctl start <service>` in the opt dir (the unit's `User=` and `Group=`). If the previous binary refuses the database, decompress the newest snapshot over it in place, which keeps the file's owner: `systemctl stop <service> && rm -f <db>-wal <db>-shm && gunzip -c "$(ls -1t <db>.pre-*.gz | head -n 1)" > <db> && systemctl start <service>`. Going back past 5.2.0 means the move too (see [Deploy day: 5.2.0](#deploy-day-520)). A rollback by hand leaves the host's record naming the release it rolled back from, so the next deploy skips that release unless `-Force`.

**Requirements.**

- PowerShell 7.4 or later on Windows, with Windows' own `tar.exe` (shipped since Windows 10 1803), git, Node and Go.
- PuTTY's `plink` 0.85 or later (connection sharing), on PATH or in `C:\Program Files\PuTTY`; pscp is no longer used. plink runs in batch mode, so a passphrase-protected `.ppk` must be loaded into Pageant first: run `pageant.exe "<KeyPath>"` once and enter the passphrase (it stays unlocked for the session; a `shell:startup` shortcut loads it at login). The first-ever connection from PuTTY also needs the host key cached once (`plink -i "<KeyPath>" <user>@<host>`, accept the fingerprint).
- On the host: bash, systemd, GNU coreutils, `curl` (the on-host checks) and `sqlite3` (the snapshot, the backup and the pull); Apache with the modules [the vhost](#the-vhost) needs, and `a2ensite`/`a2dissite` and `tar` for the move.
- For the plugin: the .NET 10 SDK.

**Manual fallback.**

1. Locally, at the release's tag: `cd frontend && npm ci && npm run build`, then build the binary in `backend/` as in step 2.
2. On the host: stop the service, install the binary over `app-suite`, start it, and check `/healthz`. When `deploy/apache-vhost.conf` changed, fill in its `__APP__` with the app's address and `__CERT__` with the certificate's name, copy it over `/etc/apache2/sites-available/<service>.conf`, and `apachectl configtest && systemctl reload apache2`.

## Database backups

Every app deploy keeps a **daily backup** of the live database installed and checked: shared-tooling's `Install-BackupTimer` with its shared job. It is two systemd units named after the service, `<service>-backup.service` (oneshot) and `<service>-backup.timer`, and the job script `/usr/local/libexec/shared-deploy/<service>-backup`. All three are rewritten by the deploy, so change them in `scripts/deploy.ps1`, not on the host.

- **When:** daily at 10:00 UTC, spread over up to 15 minutes; a missed run happens at the next boot.
- **As whom:** the service's own `User=` and `Group=`, read from the running unit at each deploy, the account that owns the database. A unit with `DynamicUser=yes` has no fixed user, so the deploy refuses to install the backup; one with no `User=` runs it as root, with a warning.
- **What:** a `VACUUM INTO` copy of the live database, taken while the app keeps running (WAL included), checked with `PRAGMA integrity_check`, compressed with gzip and checked with `gzip -t` before it takes the day's name. No uncompressed copy is left behind.
- **Where:** a `backups/` folder beside the database (created `0700`, owned by the service user), one file per UTC day: `<db folder>/backups/<service>-YYYY-MM-DD.sqlite.gz`. A second run the same day replaces that day's copy. **28 days** are kept; the job removes only its own dated names.
- **Sandbox:** the same hardening as Kindred Nights' backup (`ProtectSystem=strict`, `UMask=0077`, no capabilities and the rest); it may write only to the database's folder.

The backups never collide with the deploy's own files: the pre-deploy snapshots are `<db>.pre-<yyyyMMdd-HHmmss>.gz` in the database's own folder, pruned to the newest three by exactly that pattern, and the dev pull's `<db>.pull` lasts only for the pull. Neither pattern can match a dated name in `backups/`, and the backup's pruning cannot match theirs.

Most deploys find the units unchanged and run nothing. When the units or the job change, or on the first deploy, they are installed and the backup runs once while the deploy waits; if that run fails, the previous units come back (or none are left), and the deploy fails saying the site is live but its backup is not. The copies stay on the droplet; the copy that leaves it is the dev-database pull.

Checking and restoring, on the host:

```bash
systemctl list-timers '<service>-backup.timer'   # the next run
journalctl -u '<service>-backup.service' -n 20   # the last runs
ls -l '<db folder>/backups/'
# Restore a day's copy over the live database (decompressing in place keeps the file's owner):
systemctl stop <service> && rm -f <db>-wal <db>-shm && gunzip -c '<db folder>/backups/<service>-YYYY-MM-DD.sqlite.gz' > <db> && systemctl start <service>
```

## Notes

- Image URLs are root-absolute (`/images/...`), served by the app from the data folder's
  `images/`: the static art and the uploads alike.
- Existing databases are auto-migrated (schema v10) to rewrite old
  `assets/images/raffles/...` prize paths to `images/raffles/...` on first start.
- **PWA**: the build also emits `sw.js`, `registerSW.js`, and `manifest.webmanifest` at its
  root; the app serves them at the site's root, `sw.js` and `registerSW.js` with
  `no-store` and the manifest with `no-cache`, never the 1-year immutable cache its hashed
  `assets/` get, so the app can update (`backend/internal/web`). The PWA icons
  (`pwa-192x192.png`, `pwa-512x512.png`, `pwa-maskable-512x512.png`) and the iOS
  `apple-touch-icon.png` are served from the data folder's `images/` - make sure they're
  present there (they ship in `deploy/images/`).
