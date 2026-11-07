# Deployment (Apache)

This folder holds the files you place at the **Apache document root** - i.e. the
things that live *alongside* the built SPA but are **not** part of `dist/`.

## Document root layout

```text
<DocumentRoot>/                <- e.g. /var/www/senpan  (Apache DocumentRoot, also the Go -webroot)
+-- .htaccess                  <- from deploy/.htaccess  (SPA fallback + routing)
+-- images/                    <- from deploy/images/    (PERSISTENT - never wiped on redeploy)
|   +-- logo.png
|   +-- favicon.png
|   +-- apple-touch-icon.png   <- iOS home-screen icon (180x180)
|   +-- pwa-192x192.png        <- PWA install icons (manifest)
|   +-- pwa-512x512.png
|   +-- pwa-maskable-512x512.png
|   +-- share_banner.png
|   +-- raffles/               <- "Raffle" image category (prize images)
|   +-- announcements_main/    <- "Announcement Main" image category (embed main image)
|   +-- announcements_thumb/   <- "Announcement Thumbnail" image category (embed thumbnail)
|   +-- flourishes/            <- "Flourishes" image category (SVG board/number flourishes; seeded from corner_flourish.svg + called_flourish.svg)
|   +-- announcements/         <- legacy announcement images; copied into announcements_main on first start
|   +-- <custom>/              <- any custom image categories added on System -> Images
|   +-- .categories.json       <- manifest of custom image categories (name <-> dir)
|   \-- bookclub/              <- reading-list cover uploads
+-- fonts/                     <- admin-uploaded font files; NOT served statically - the Go server streams them via tokenized URLs (see Font host)
+-- carrd/                     <- Carrd image-host projects; served by the carrd.senpan.cafe vhost (see Carrd host)
+-- dist/                      <- from frontend/dist/    (the built Vue app; replaced on each deploy)
|   +-- index.html
|   \-- assets/...             <- content-hashed JS/CSS
\-- plugin/                    <- the Dalamud custom repo (pluginmaster.json, SenpanCompanionAdmin/latest.zip, .htaccess); replaced whole on each plugin publish
```

`images/` is a **sibling** of `dist/`, so re-uploading/replacing `dist/` never
touches uploaded raffle images. The build also strips `dist/images/` (it would
otherwise be a redundant copy) so the two never get confused.

All image uploads are now managed centrally on the **System -> Images** admin
page, which writes into per-category subdirectories of `images/` (the three
permanent categories above plus any custom ones). They all live under the
persistent `images/` tree, so no extra vhost or one-time setup is needed. On
first start the Go server copies any legacy `images/announcements/` files into
`images/announcements_main/` (idempotent - safe to leave the legacy folder).

Uploaded **fonts** live in a separate folder, `<webRoot>/fonts/`, but are NOT
served statically: the `fonts.senpan.cafe` vhost reverse-proxies to the Go
server's tokenized, origin-gated endpoints - see
[Font host (protected serving)](#font-host-protected-serving).

## One-time server setup

1. Upload `deploy/.htaccess` -> `<DocumentRoot>/.htaccess`.
2. Upload `deploy/images/` -> `<DocumentRoot>/images/` (creates `images/raffles/`).
3. Ensure Apache has `mod_rewrite` + `mod_headers` enabled and the vhost allows
   `.htaccess` overrides for the doc root:

   ```apache
   <Directory /var/www/senpan>
       AllowOverride All
       Require all granted
   </Directory>
   ```

4. Confirm the vhost proxies the API to the Go server (already in place):

   ```apache
   ProxyPass        /api/ws ws://localhost:8080/api/ws
   ProxyPass        /api/   http://localhost:8080/api/
   ProxyPassReverse /api/   http://localhost:8080/api
   ProxyPreserveHost On
   ```

5. Run the Go server with `-webroot` pointing at the **document root** so uploads
   land in `<DocumentRoot>/images/raffles/`:

   ```text
   /opt/senpan/app-suite -addr :8080 -db /opt/senpan/data/database.sqlite -webroot /var/www/senpan
   ```

   The web server (Apache) must have write access to `<DocumentRoot>/images/raffles/`
   only if the Go process and Apache run as different users; the Go process is
   what writes the files, so it needs write access to that directory.

## Running as a non-root user (recommended)

The repo's `senpan.service` specifies `User=senpan`/`Group=senpan`. Nothing in the
server needs root - it binds `:8080` (non-privileged) and only touches ordinary
files - so run it as a dedicated, locked system account to shrink the blast radius
of any compromise.

> **Important:** `scripts/deploy.ps1` does **NOT** ship the systemd unit - it only
> updates the binary, `dist/` + `.htaccess` and the plugin folder, and installs the
> daily backup's own units. It **reads** the running unit (its `User=`, `-db` and
> `-addr`), so the backup runs as whatever `User=` says. The repo `deploy/senpan.service` is a
> reference/template; the unit that actually runs lives on the host at
> `/etc/systemd/system/senpan.service`. So editing `User=` in the repo and
> redeploying does nothing to the running service - you must update the unit **on
> the host** (step 3 below). This is why a `daemon-reload`/`restart` alone leaves
> `systemctl show senpan -p User` reporting `root`.

One-time host setup:

```bash
# 1. Create a locked system account (no home, no login shell).
useradd --system --no-create-home --shell /usr/sbin/nologin senpan

# 2. Give it ownership of everything the process writes: its own dir (binary,
#    DB + SQLite -wal/-shm) and EVERY document-root subtree it writes into.
chown -R senpan:senpan /opt/senpan
chown -R senpan:senpan /var/www/apps.senpan.cafe/images \
                       /var/www/apps.senpan.cafe/fonts \
                       /var/www/apps.senpan.cafe/carrd

# 3. Update the UNIT FILE ON THE HOST. Inspect the current one first, then edit it:
systemctl cat senpan                 # note the path + current User/ReadWritePaths
systemctl edit --full senpan         # opens the effective unit for editing
#   In the editor, set:
#     User=senpan
#     Group=senpan
#     ReadWritePaths=/opt/senpan /var/www/apps.senpan.cafe/images \
#                    /var/www/apps.senpan.cafe/fonts /var/www/apps.senpan.cafe/carrd
#   (match the ExecStart -webroot to your host). Alternatively, copy the repo's
#   deploy/senpan.service onto /etc/systemd/system/senpan.service and fill in its
#   placeholders - <service-user>, /opt/<app>, <webroot>, <service-name> - with
#   your values (the tracked template deliberately carries no concrete paths).

# 4. Reload + restart.
systemctl daemon-reload
systemctl restart senpan
```

`ReadWritePaths` in the running unit must list every one of those subtrees or
writes fail with `EACCES` under `ProtectSystem=strict`. Apache keeps serving the
files as its own user (read is world-readable); only writes are the `senpan`
user's concern. Verify:

```bash
systemctl show senpan -p User -p Group          # senpan / senpan
ps -o user= -C app-suite                         # not root
# Smoke-test each writable subtree from the admin UI: upload an image, a font
# (confirm fonts/.woff2/ is written), and a Carrd project image; confirm an
# admin CRUD write succeeds and /var/log/senpan/senpan.log keeps growing.
```

> If font/carrd uploads currently work in production, the **live** unit's
> `ReadWritePaths` is already broader than an older repo copy - run
> `systemctl cat senpan` and reconcile before shipping the repo unit so a
> `daemon-reload` never *narrows* the writable set.

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
  served to browsers via `GET /api/config`). Keep the secret out of the unit's
  inline `Environment=` (visible via `systemctl show` / `/proc/<pid>/environ`) -
  put both in a root-owned `chmod 600` file
  loaded with `EnvironmentFile=-/etc/senpan/senpan.env` (see the commented block in
  `deploy/senpan.service`). Rotate the secret from the Cloudflare dashboard and
  update the env file.

## Font host (protected serving)

Uploaded fonts are licensed assets, so they are **never served as static
files**. The Go server streams them itself through obfuscated token URLs that
rotate every 1-2 weeks, gated by the requesting site's `Origin` against
**that font's own allowlist** (Atelier -> Font Upload -> Edit -> Allowed sites).
Chrome and Firefox hard-require CORS on cross-origin `@font-face`, so a site
that isn't on a font's list cannot render it. Safari is the exception: it fetches
fonts in `no-cors` mode, sending **no** `Origin`, so for an Origin-less request
the gate checks the site in the `Referer` (the embedding page) against the same
allowlist instead. Requests naming no site at all (pasting the URL into a
browser, `wget`, casual scrapers) get a 403, and `kit.css` only emits each
requesting site's allowed fonts. The SPA itself loads fonts
**same-origin** via the existing `/api` ProxyPass
(`/api/fonts/pub/f/<token>`), so the app and its font picker always work - the
allowlists only govern external sites.

External (Carrd) sites embed one stylesheet whose URL never changes:

```html
<link rel="stylesheet" href="https://fonts.senpan.cafe/kit.css">
```

and then use `font-family: '<CSS Name>'` (shown per font on the Font Upload
tab; defaults to the filename without extension). The kit's tokenized font
URLs refresh automatically on rotation.

Files sharing a base name (e.g. `Jasper.otf` + `Jasper.woff2`) are format
variants of **one font**. A font with no uploaded WOFF2 gets one
**auto-converted** (stored in the hidden `<webRoot>/fonts/.woff2/`
sub-directory; a startup backfill converts pre-existing fonts and sweeps
stale copies). The WOFF2 is served by default - uploads stay on disk
untouched, and the served version is selectable per font in the Edit modal.

One-time setup for the font host:

1. Make the `fonts.senpan.cafe` vhost a **reverse proxy** to the Go server
   (replacing any DocumentRoot-based static config):

   ```apache
   ProxyPass        / http://localhost:8080/api/fonts/pub/
   ProxyPassReverse / http://localhost:8080/api/fonts/pub/
   ProxyPreserveHost On
   ```

   `https://fonts.senpan.cafe/kit.css` -> `/api/fonts/pub/kit.css` and
   `https://fonts.senpan.cafe/f/<token>` -> `/api/fonts/pub/f/<token>`.
2. Upload `deploy/fonts.htaccess` -> `<webRoot>/fonts/.htaccess`. It now DENIES
   all direct access - pure defense in depth in case the vhost is ever
   reverted to serving the folder statically. The Go server only adds/removes
   font files in that folder, so the `.htaccess` persists across uploads.

> **Migration note:** legacy direct URLs
> (`https://fonts.senpan.cafe/My%20Font.ttf`) stop working the moment the
> vhost becomes a proxy. Any external site using them must switch to the
> kit stylesheet above, and its origin must be added to the allowlist.

**Verify the cutover** (the repo can't confirm host-side vhost state):

```bash
# The gated responses MUST carry Cache-Control: private, or Cloudflare (which
# ignores Vary) can serve one origin's kit to another. Check this FIRST -
# it is a blocking precondition before migrating any external site.
curl -sI https://fonts.senpan.cafe/kit.css | grep -i cache-control     # -> private

apachectl -S | grep fonts.senpan.cafe                                  # a proxy vhost, no DocumentRoot
curl -s -o /dev/null -w '%{http_code}\n' https://fonts.senpan.cafe/AnyFont.ttf   # -> 403/404 (legacy URL dead)
curl -s -o /dev/null -w '%{http_code}\n' https://fonts.senpan.cafe/f/anytoken    # -> 403 (no Origin)
curl -s -H 'Origin: https://<allowed-site>' https://fonts.senpan.cafe/kit.css    # -> that site's fonts
```

## Carrd image host (CORS)

The **System -> Carrd Upload** admin tab creates per-project folders under
`<webRoot>/carrd/` (each of which may contain arbitrarily nested sub-folders)
and writes uploaded images to them. They are served by a separate vhost at
`https://carrd.senpan.cafe/<folder>/.../<image>` and embedded by external Carrd
sites (a different origin), so cross-origin reads must be allowed.

One-time setup for the carrd host (same pattern as the font host):

1. Upload `deploy/carrd.htaccess` -> `<webRoot>/carrd/.htaccess`. It adds
   `Access-Control-Allow-Origin: *` (plus a cache header) for image files, hides
   the per-project `.carrd.json` metadata sidecar, and disables directory
   listings. The Go server only adds/removes project folders and images in that
   tree, so the `.htaccess` persists across uploads.
2. Ensure the carrd vhost has `mod_headers` enabled and allows `.htaccess`
   overrides (`AllowOverride All` or at least `FileInfo`). If overrides are
   disabled, put the `<FilesMatch>` blocks from `deploy/carrd.htaccess` directly
   in the vhost's `<Directory>` instead.

## Server logs

The Go server writes **structured JSON** logs to **stdout** (captured by
`journalctl -u senpan`) and, by default, to a rotating file at
`/var/log/senpan/senpan.log` (`-log-file`; pass `""` to disable, e.g. local dev).
timberjack rotates the file **daily at local midnight**, zstd-compresses backups,
and bounds retention (14 backups / 30 days / 100 MB safety cap). The admin
**System -> Logs** tab and the on-box `jlv` tool both read this file;
`GET /api/logs` tails it and every line also streams to connected admins over the
WebSocket for a live tail, so most log inspection needs no SSH.

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

`senpan.service` ships with a defense-in-depth sandbox: `NoNewPrivileges`, `ProtectSystem=strict` + `ProtectHome`, `PrivateTmp`/`PrivateDevices`, `ProtectKernel*`/`ProtectControlGroups`, `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX`, `RestrictNamespaces`, `LockPersonality`, `SystemCallFilter=@system-service` + `SystemCallArchitectures=native`, and **`MemoryDenyWriteExecute=yes`** (W^X). Every directory the app writes (the DB dir + the `-webroot` `images/`, `fonts/`, `carrd/` subtrees, plus `LogsDirectory=senpan`) must be listed in `ReadWritePaths` or writes fail with `EACCES` under `ProtectSystem=strict` - keep `ReadWritePaths` in sync with your deployed `-webroot`.

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

## Each deploy

**Scripted (recommended).** From the repo root, on Windows. `task deploy` deploys everything: the backend, then the frontend (`-Target main`), then the plugin with `-IfNewer`, which skips it when its version is already published. `task deploy:backend`, `task deploy:frontend` and `task deploy:plugin` deploy one each, with flags after `--`. The script itself:

```powershell
.\scripts\deploy.ps1                  # backend, then frontend (default: main; 'both' is a kept alias)
.\scripts\deploy.ps1 -Target frontend # the SPA only
.\scripts\deploy.ps1 -Target backend  # Go binary + service restart, the daily backup check, the dev-database pull
.\scripts\deploy.ps1 -Target all      # backend, frontend, then plugin
.\scripts\deploy.ps1 -Target plugin   # Dalamud custom-repo files only
```

The script is built on shared-tooling's SharedDeploy module (`shared/deploy/pwsh/`, from the `shared/` submodule; its README describes each function), so it takes the shape every deploy there takes: one archive, one SSH connection held for the whole run, one remote run. Several targets share the archive, the connection and the remote run, with no pauses between them.

> **First-time setup:** the script is version-controlled, but **no** environment-specific value is baked into it - the host, SSH user, key path, webroot, service name and opt dir all live outside the repo. Copy `scripts/deploy.config.example.ps1` to `scripts/deploy.config.ps1` (untracked) and fill in the six required `$Deploy*` values. Each can instead be passed as a `-param` or set via its `$env:DEPLOY_*` variable (`DEPLOY_VPS_HOST`, `DEPLOY_VPS_USER`, `DEPLOY_KEY_PATH`, `DEPLOY_WEB_ROOT`, `DEPLOY_SERVICE_NAME`, `DEPLOY_REMOTE_OPT_DIR`); the script fails fast, listing any that are unset, before it builds or connects. The site's address, which the smoke check fetches, is public already (`plugins/pluginmaster.json` links to it), so it defaults to `https://apps.senpan.cafe`; `-SiteUrl`, `DEPLOY_SITE_URL` or `$DeploySiteUrl` overrides it. `-ConfigPath` points it at another settings file. The live database needs no setting: it is read from the running unit (step 2). `RemoteDbPath` (`DEPLOY_REMOTE_DB_PATH`, `$DeployRemoteDbPath`) is only a cross-check, used on its own when the unit names no `-db`.

**What a run does:**

1. **Build, before anything connects.** Backend: a static `linux/amd64` binary (`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o app-suite .` in `backend/`). Frontend: `npm run build` in `frontend/` (vue-tsc + vite). Plugin: `dotnet build -c Release` from clean (its `bin/` and `obj/` deleted first, so the package can never lag the manifest), then its repository laid out: `SenpanCompanionAdmin/latest.zip`, `plugins/pluginmaster.json` with `?v=<version>` on its download links, and `deploy/plugin.htaccess`. The built version must equal `pluginmaster.json`'s `AssemblyVersion`. With the plugin in the run, the published `<SiteUrl>/plugin/pluginmaster.json` is read before anything is built: a version not higher than the published one stops the run there, since a backend and frontend in the same run (`-Target all`) would otherwise be live before the host refused the plugin (if the index can't be read, the host's own check still stands). A broken build changes nothing. Every piece goes into **one** `.tar.gz`: a truncated archive refuses to unpack, where a half-copied folder can look whole.
2. **Read the service on the host** (backend), before anything is uploaded. The unit must be installed. Its `User=` (and `Group=`) is who the backup runs as; the `-db` on its `ExecStart` is the live database; its `-addr` is where `/api/version` answers on the host (`:8080`, the binary's default, when it has none). A `RemoteDbPath` that differs from `-db` stops the run with nothing changed.
3. **Upload** the archive as one stream through plink, into the SSH user's home.
4. **One remote run.** The host checks the archive's SHA-256 and file count, and only then runs the steps, API first: a new backend is additive to the old SPA, while a new SPA calls endpoints an old backend does not have; the plugin, a second client of the API, comes last.
   - **backend:** stops the service, keeps `app-suite.old`, snapshots the database with `sqlite3 .backup`, checked with `PRAGMA integrity_check`, compressed with gzip and checked with `gzip -t`, to `<db>.pre-<yyyyMMdd-HHmmss>.gz` (UTC; readable by root only; the newest three are kept), installs the binary, starts the service and waits up to 30 s for `/api/version` to answer on the host. If it does not, the previous binary goes back with the snapshot and restarts: the new binary may already have migrated the database, and the previous one refuses a newer schema (`database schema vN is newer than this binary`). Brief downtime (a few seconds) while the service is stopped.
   - **frontend:** swaps `dist/` in by renames (the previous kept as `dist.old`), with `deploy/.htaccess` beside it (the previous kept as `.htaccess.old`). The repo copy of `.htaccess` is the source of truth - edit it there, not on the host. `images/` is never touched.
   - **plugin:** refuses a version that is not higher than the published one, since Dalamud updates to nothing else, then swaps the whole `plugin/` folder in (the previous kept as `plugin.old`).

   A step that fails stops the run there. With several targets, the steps before it stay live and are not smoke-checked, so raise the plugin's version before `-Target all`, or use `-Target main`.
5. **Smoke check from this machine, through Cloudflare**, each URL tried three times: `<SiteUrl>/api/version` must name the version this run built; `<SiteUrl>/` must name the build's entry script, which must load too; `<SiteUrl>/plugin/pluginmaster.json` must carry this release's `?v=`. If any fails, what this run changed goes back in reverse order: plugin, frontend, then backend, the backend with the pre-deploy snapshot, so the previous binary starts on the schema it knows. Whatever was written in the seconds the new release ran is lost with it (decided 2026-11-06, as Kindred Nights does).
6. **Daily backup** (backend, once the smoke check passes): see [Database backups](#database-backups). If it cannot be installed, the run fails saying the site is live but its backup is not.
7. **Dev-database pull** (backend, unless `-NoDbPull`): an online `sqlite3 .backup` on the host (the service keeps running), downloaded and checked against the host's SHA-256 into `devdata/database.sqlite`. The previous local copy is kept as `database.sqlite.bak-<time>` (the newest three), stale `-wal`/`-shm` files are removed, and the host's copy is deleted. While a local dev server holds the database open, the pull is skipped with a warning. These copies hold **live** data (accounts, password hashes, tokens, webhook URLs) - keep them local.

**Options.** `-SkipBuild` deploys the existing build artifacts (the `/api/version` check then only asks for an answer, since the binary's version is unknown). `-NoBackup` removes `app-suite.old`, `dist.old`, `.htaccess.old` and `plugin.old` once the host run succeeds; the smoke check then has nothing to go back to, so a failure only stops the run. The database snapshot is still taken. `-NoDbPull` skips step 7.

**The connection.** The droplet runs `ufw limit ssh`, which refuses an address after six connections in 30 s. The whole run is **one** SSH connection, held by a background `plink -share`; every command, the upload and the download ride it. If it drops, the module reconnects (one new connection) and retries the step once, resuming where it stopped; only a refused reconnect waits 35 s.

**Rolling back by hand** (on the host, in the webroot or opt dir): frontend `mv dist dist.failed && mv dist.old dist && rm -rf dist.failed`, and `mv -f .htaccess.old .htaccess`; the plugin the same way with `plugin` and `plugin.old`; backend `systemctl stop <service> && install -m 755 app-suite.old app-suite && systemctl start <service>`. If the previous binary refuses the database, decompress the newest snapshot over it in place, which keeps the file's owner: `systemctl stop <service> && rm -f <db>-wal <db>-shm && gunzip -c "$(ls -1t <db>.pre-*.gz | head -n 1)" > <db> && systemctl start <service>`.

**Requirements.**

- PowerShell 7.4 or later on Windows, with Windows' own `tar.exe` (shipped since Windows 10 1803).
- PuTTY's `plink` 0.85 or later (connection sharing), on PATH or in `C:\Program Files\PuTTY`; pscp is no longer used. plink runs in batch mode, so a passphrase-protected `.ppk` must be loaded into Pageant first: run `pageant.exe "<KeyPath>"` once and enter the passphrase (it stays unlocked for the session; a `shell:startup` shortcut loads it at login). The first-ever connection from PuTTY also needs the host key cached once (`plink -i "<KeyPath>" <user>@<host>`, accept the fingerprint).
- On the host: bash, systemd, GNU coreutils, `curl` (the on-host health check) and `sqlite3` (the snapshot, the backup and the pull).
- For the plugin: the .NET 10 SDK.

**Manual fallback.**

1. Locally: `cd frontend && npm run build`.
2. Upload the contents of `frontend/dist/` -> `<DocumentRoot>/dist/` (replace).
   Leave `<DocumentRoot>/images/` and `<DocumentRoot>/.htaccess` untouched.

## Database backups

Every deploy that includes the backend keeps a **daily backup** of the live database installed and checked: shared-tooling's `Install-BackupTimer` with its shared job. It is two systemd units named after the service, `<service>-backup.service` (oneshot) and `<service>-backup.timer`, and the job script `/usr/local/libexec/shared-deploy/<service>-backup`. All three are rewritten by the deploy, so change them in `scripts/deploy.ps1`, not on the host.

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

- Image URLs are absolute (`/images/...`), served by Apache straight from the
  root `images/` folder - both the static assets and uploaded raffle prizes.
- Existing databases are auto-migrated (schema v10) to rewrite old
  `assets/images/raffles/...` prize paths to `images/raffles/...` on first start.
- `/api/*` and `/api/ws` are never handled by `.htaccess`; the vhost ProxyPass
  intercepts them first.
- **PWA**: the build also emits `sw.js`, `registerSW.js`, and
  `manifest.webmanifest` at the `dist/` root - they're uploaded as part of
  `dist/` and served at the document root by the `.htaccess` real-file rule. The
  caching block in `.htaccess` deliberately exempts `sw.js`/`registerSW.js`/
  `*.webmanifest` from the 1-year immutable cache so the app can update; if you
  re-copy an older `.htaccess`, keep those exemptions or the service worker will
  go stale. The PWA icons (`pwa-192x192.png`, `pwa-512x512.png`,
  `pwa-maskable-512x512.png`) and the iOS `apple-touch-icon.png` are served from
  the persistent root `images/` folder - make sure they're present there (they
  ship in `deploy/images/`).
