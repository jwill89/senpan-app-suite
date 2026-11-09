# Deployment (Apache)

This folder holds the files for the **Apache document root**: its `.htaccess`
template and the persistent `images/`. Since 4.0.0 the app is one Go binary that
serves the API, the WebSocket and the SPA, whose build it embeds; Apache serves only
the document root's own folders and proxies everything else to it.

## Document root layout

```text
<DocumentRoot>/                <- e.g. /var/www/senpan  (Apache DocumentRoot, also the Go -webroot)
+-- .htaccess                  <- from deploy/.htaccess, with the app's address filled in (the deploy writes it)
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
\-- plugin/                    <- the Dalamud custom repo (pluginmaster.json, SenpanCompanionAdmin/latest.zip, .htaccess); replaced whole on each plugin publish
```

`.htaccess` serves `images/`, `carrd/` and `plugin/` as they are (a file missing
there is a 404), refuses `fonts/` (its own `.htaccess`) and every dotfile, and
proxies every other path to the app: its pages, its content-hashed `assets/`, its
sounds and the PWA files. The app sets their cache rules, security headers and
Content-Security-Policy itself. A deploy replaces only the binary and `.htaccess`, so
uploads are never touched. The build strips its own `images/`, so the binary never
carries a copy of the persistent one.

Before 4.0.0 Apache served the SPA from `<DocumentRoot>/dist/`, which the deploy
replaced (keeping `dist.old/`). The 4.0.0 deploy leaves both in place, so a rollback
to the previous `.htaccess` still finds the previous SPA. Once 4.0.0 has stayed, remove
them on the host: `rm -rf <DocumentRoot>/dist <DocumentRoot>/dist.old`.

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

1. The deploy writes `<DocumentRoot>/.htaccess` from `deploy/.htaccess`; by hand,
   copy it and replace `__APP__` with the app's address (`127.0.0.1:8080`).
2. Upload `deploy/images/` -> `<DocumentRoot>/images/` (creates `images/raffles/`).
3. Ensure Apache has `mod_rewrite`, `mod_headers`, `mod_proxy` and `mod_proxy_http`
   enabled (the `.htaccess` proxies with `[P]`; the vhost's ProxyPass already needs
   the last two) and the vhost allows `.htaccess` overrides for the doc root:

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

The standards run each service as its own locked system account, and the repo's
`deploy/senpan.service` template does (`User=` and `Group=`, `<service-user>` there).
Nothing in the server needs root - it binds `:8080` (non-privileged) and only touches
ordinary files - so a dedicated account shrinks the blast radius of any compromise.

> **Important:** `scripts/deploy.ps1` does **NOT** ship the systemd unit - it only
> updates the binary, the web root's `.htaccess` and the plugin folder, and installs the
> daily backup's own units. It **reads** the running unit (its `User=`, `Group=`, `-db`
> and `-addr`), so the binary it installs is owned by that account, as the standards
> have it, and the backup runs as it too. The repo `deploy/senpan.service` is a
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

**Scripted (recommended).** From the repo root, on Windows. Only releases ship: the app's newest `v<version>` tag CI made on main, the plugin's newest `Plugin-v<version>` (the tags are fetched first). Each is checked out into a folder of its own under `%TEMP%` and built there, with `npm ci` from its own lockfile, so commits after it, a shared-tooling update or anything uncommitted are never part of it (shared-tooling's newest-release deploy). `task deploy` deploys the app, then the plugin with `-IfNewer`, which skips it when its version is already published. `task deploy:backend` (the app) and `task deploy:plugin` deploy one each, with flags after `--`. The script itself:

```powershell
.\scripts\deploy.ps1                  # the app's newest release (skipped when the host already runs it)
.\scripts\deploy.ps1 -Force           # the same release again
.\scripts\deploy.ps1 -Target plugin   # the plugin's newest release, to the Dalamud custom repo
```

The script is built on shared-tooling's SharedDeploy module (`shared/deploy/pwsh/`, from the `shared/` submodule; its README describes each function), so it takes the shape every deploy there takes: one archive, one SSH connection held for the whole run, one remote run.

> **First-time setup:** the script is version-controlled, but **no** environment-specific value is baked into it - the host, SSH user, key path, webroot, service name and opt dir all live outside the repo. Copy `scripts/deploy.config.example.ps1` to `scripts/deploy.config.ps1` (untracked) and fill in the six required `$Deploy*` values. Each can instead be passed as a `-param` or set via its `$env:DEPLOY_*` variable (`DEPLOY_VPS_HOST`, `DEPLOY_VPS_USER`, `DEPLOY_KEY_PATH`, `DEPLOY_WEB_ROOT`, `DEPLOY_SERVICE_NAME`, `DEPLOY_REMOTE_OPT_DIR`); the script fails fast, listing any that are unset, before it builds or connects. The site's address, which the smoke check fetches, is public already (`plugins/pluginmaster.json` links to it), so it defaults to `https://apps.senpan.cafe`; `-SiteUrl`, `DEPLOY_SITE_URL` or `$DeploySiteUrl` overrides it. `-ConfigPath` points it at another settings file. The live database needs no setting: it is read from the running unit (step 1). `RemoteDbPath` (`DEPLOY_REMOTE_DB_PATH`, `$DeployRemoteDbPath`) is only a cross-check, used on its own when the unit names no `-db`.

**What an app deploy does:**

1. **Read the service on the host**, before anything is built. The unit must be installed. Its `User=` (and `Group=`) is who the backup runs as; the `-db` on its `ExecStart` is the live database; its `-addr` is where the app answers on the host (`:8080`, the binary's default, when it has none, asked on `127.0.0.1`), which `/healthz` is asked on and the `.htaccess` proxies to. A `RemoteDbPath` that differs from `-db` stops the run with nothing changed. The host also says which release it last took (`~/.shared-deploy/senpan`): when that is this release, the run ends there, unless `-Force`.
2. **Build the release, in its own checkout.** The front end first (`npm run build` in `frontend/`: vue-tsc + vite, into `backend/internal/web/dist/`), then a static `linux/amd64` binary that embeds it (`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o app-suite .` in `backend/`). The release's `deploy/.htaccess` is filled in with the app's address. A broken build changes nothing. Both go into **one** `.tar.gz`: a truncated archive refuses to unpack, where a half-copied folder can look whole.
3. **Upload** the archive as one stream through plink, into the SSH user's home.
4. **One remote run.** The host checks the archive's SHA-256 and file count, and only then:
   - **the binary:** stops the service, keeps `app-suite.old`, snapshots the database with `sqlite3 .backup`, checked with `PRAGMA integrity_check`, compressed with gzip and checked with `gzip -t`, to `<db>.pre-<yyyyMMdd-HHmmss>.gz` (UTC; readable by root only; the newest three are kept), installs the binary, owned by the account the unit runs as, starts the service and waits up to 30 s for `/healthz` to answer on the host. If it does not, the previous binary goes back with the snapshot and restarts: the new binary may already have migrated the database, and the previous one refuses a newer schema (`database schema vN is newer than this binary`). Brief downtime (a few seconds) while the service is stopped.
   - **then `.htaccess`:** swapped in by renames, the previous kept in `<DocumentRoot>/.deploy-backup/`. The repo's `deploy/.htaccess` is the source of truth - edit it there, not on the host. `images/` and the other folders are never touched.
5. **Smoke check from this machine, through Cloudflare**, each URL tried three times: `<SiteUrl>/healthz` and `<SiteUrl>/api/version` must name this release's version; `<SiteUrl>/` must name the build's entry script, which must load too. If any fails, the previous `.htaccess` goes back, then the previous binary with the pre-deploy snapshot, so it starts on the schema it knows. Whatever was written in the seconds the new release ran is lost with it (decided 2026-11-06, as Kindred Nights does). Once the checks pass, the host records the release.
6. **Daily backup**, once the smoke check passes: see [Database backups](#database-backups). If it cannot be installed, the run fails saying the site is live but its backup is not.
7. **Dev-database pull** (unless `-NoDbPull`): an online `sqlite3 .backup` on the host (the service keeps running), downloaded and checked against the host's SHA-256 into `devdata/database.sqlite`. The previous local copy is kept as `database.sqlite.bak-<time>` (the newest three), stale `-wal`/`-shm` files are removed, and the host's copy is deleted. While a local dev server holds the database open, the pull is skipped with a warning. These copies hold **live** data (accounts, password hashes, tokens, webhook URLs) - keep them local.

The checkout under `%TEMP%` is removed at the end, whether the run passed or not.

**What a plugin deploy does.** It checks out the plugin's newest release, reads the published `<SiteUrl>/plugin/pluginmaster.json` and stops before building when the release's version is not higher, since Dalamud updates to nothing else (with `-IfNewer`, the same version ends the run without error; if the index can't be read, the host's own check still stands). Then `dotnet build -c Release` from clean (its `bin/` and `obj/` deleted first, so the package can never lag the manifest), and its repository laid out: `SenpanCompanionAdmin/latest.zip`, `plugins/pluginmaster.json` with `?v=<version>` on its download links, and `deploy/plugin.htaccess`. The built version must equal `pluginmaster.json`'s `AssemblyVersion`. On the host the whole `plugin/` folder is swapped in (the previous kept as `plugin.old`), and the published index must then carry the new `?v=`, or the previous folder goes back.

**Options.** `-Force` deploys the app's release even when the host already runs it. `-NoBackup` removes `app-suite.old`, the previous `.htaccess` and `plugin.old` once the host run succeeds; the smoke check then has nothing to go back to, so a failure only stops the run. The database snapshot is still taken. `-NoDbPull` skips step 7.

**The connection.** The droplet runs `ufw limit ssh`, which refuses an address after six connections in 30 s. The whole run is **one** SSH connection, held by a background `plink -share`; every command, the upload and the download ride it. If it drops, the module reconnects (one new connection) and retries the step once, resuming where it stopped; only a refused reconnect waits 35 s.

**Rolling back by hand** (on the host): `.htaccess` with `mv -f <DocumentRoot>/.deploy-backup/.htaccess <DocumentRoot>/.htaccess`; the plugin with `mv plugin plugin.failed && mv plugin.old plugin && rm -rf plugin.failed` in the webroot; the binary with `systemctl stop <service> && install -m 755 -o <user> -g <group> app-suite.old app-suite && systemctl start <service>` in the opt dir (the unit's `User=` and `Group=`). If the previous binary refuses the database, decompress the newest snapshot over it in place, which keeps the file's owner: `systemctl stop <service> && rm -f <db>-wal <db>-shm && gunzip -c "$(ls -1t <db>.pre-*.gz | head -n 1)" > <db> && systemctl start <service>`. Going back past 4.0.0 means the 3.x `.htaccess`, which serves `dist/`, as well as the 3.x binary. A rollback by hand leaves the host's record naming the release it rolled back from, so the next deploy skips that release unless `-Force`.

**Requirements.**

- PowerShell 7.4 or later on Windows, with Windows' own `tar.exe` (shipped since Windows 10 1803), git, Node and Go.
- PuTTY's `plink` 0.85 or later (connection sharing), on PATH or in `C:\Program Files\PuTTY`; pscp is no longer used. plink runs in batch mode, so a passphrase-protected `.ppk` must be loaded into Pageant first: run `pageant.exe "<KeyPath>"` once and enter the passphrase (it stays unlocked for the session; a `shell:startup` shortcut loads it at login). The first-ever connection from PuTTY also needs the host key cached once (`plink -i "<KeyPath>" <user>@<host>`, accept the fingerprint).
- On the host: bash, systemd, GNU coreutils, `curl` (the on-host health check) and `sqlite3` (the snapshot, the backup and the pull); Apache with `mod_proxy_http`.
- For the plugin: the .NET 10 SDK.

**Manual fallback.**

1. Locally, at the release's tag: `cd frontend && npm ci && npm run build`, then build the binary in `backend/` as in step 2.
2. On the host: stop the service, install the binary over `app-suite`, start it, and check `/healthz`. Fill in and copy `.htaccess` as in [One-time server setup](#one-time-server-setup) when it changed.

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

- Image URLs are absolute (`/images/...`), served by Apache straight from the
  root `images/` folder - both the static assets and uploaded raffle prizes.
- Existing databases are auto-migrated (schema v10) to rewrite old
  `assets/images/raffles/...` prize paths to `images/raffles/...` on first start.
- `/api/*` and `/api/ws` are never handled by `.htaccess`; the vhost ProxyPass
  intercepts them first.
- **PWA**: the build also emits `sw.js`, `registerSW.js`, and
  `manifest.webmanifest` at its root; the app serves them at the site's root,
  `sw.js` and `registerSW.js` with `no-store` and the manifest with `no-cache`,
  never the 1-year immutable cache its hashed `assets/` get, so the app can update
  (`backend/internal/web`). The PWA icons (`pwa-192x192.png`, `pwa-512x512.png`,
  `pwa-maskable-512x512.png`) and the iOS `apple-touch-icon.png` are served from
  the persistent root `images/` folder - make sure they're present there (they
  ship in `deploy/images/`).
