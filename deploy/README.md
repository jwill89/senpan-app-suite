# Deployment (Apache)

This folder holds the files you place at the **Apache document root** — i.e. the
things that live *alongside* the built SPA but are **not** part of `dist/`.

## Document root layout

```
<DocumentRoot>/                ← e.g. /var/www/senpan  (Apache DocumentRoot, also the Go -webroot)
├── .htaccess                  ← from deploy/.htaccess  (SPA fallback + routing)
├── images/                    ← from deploy/images/    (PERSISTENT — never wiped on redeploy)
│   ├── logo.png
│   ├── favicon.png
│   ├── apple-touch-icon.png   ← iOS home-screen icon (180×180)
│   ├── pwa-192x192.png        ← PWA install icons (manifest)
│   ├── pwa-512x512.png
│   ├── pwa-maskable-512x512.png
│   ├── share_banner.png
│   ├── raffles/               ← "Raffle" image category (prize images)
│   ├── announcements_main/    ← "Announcement Main" image category (embed main image)
│   ├── announcements_thumb/   ← "Announcement Thumbnail" image category (embed thumbnail)
│   ├── flourishes/            ← "Flourishes" image category (SVG board/number flourishes; seeded from corner_flourish.svg + called_flourish.svg)
│   ├── announcements/         ← legacy announcement images; copied into announcements_main on first start
│   ├── <custom>/              ← any custom image categories added on System → Images
│   ├── .categories.json       ← manifest of custom image categories (name ↔ dir)
│   └── bookclub/              ← reading-list cover uploads
├── fonts/                     ← admin-uploaded font files; NOT served statically — the Go server streams them via tokenized URLs (see Font host)
├── carrd/                     ← Carrd image-host projects; served by the carrd.senpan.cafe vhost (see Carrd host)
└── dist/                      ← from frontend/dist/    (the built Vue app; replace on each deploy)
    ├── index.html
    └── assets/…               ← content-hashed JS/CSS
```

`images/` is a **sibling** of `dist/`, so re-uploading/replacing `dist/` never
touches uploaded raffle images. The build also strips `dist/images/` (it would
otherwise be a redundant copy) so the two never get confused.

All image uploads are now managed centrally on the **System → Images** admin
page, which writes into per-category subdirectories of `images/` (the three
permanent categories above plus any custom ones). They all live under the
persistent `images/` tree, so no extra vhost or one-time setup is needed. On
first start the Go server copies any legacy `images/announcements/` files into
`images/announcements_main/` (idempotent — safe to leave the legacy folder).

Uploaded **fonts** live in a separate folder, `<webRoot>/fonts/`, but are NOT
served statically: the `fonts.senpan.cafe` vhost reverse-proxies to the Go
server's tokenized, origin-gated endpoints — see
[Font host (protected serving)](#font-host-protected-serving).

## One-time server setup

1. Upload `deploy/.htaccess` → `<DocumentRoot>/.htaccess`.
2. Upload `deploy/images/` → `<DocumentRoot>/images/` (creates `images/raffles/`).
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

   ```
   /opt/senpan/app-suite -addr :8080 -db /opt/senpan/data/database.sqlite -webroot /var/www/senpan
   ```

   The web server (Apache) must have write access to `<DocumentRoot>/images/raffles/`
   only if the Go process and Apache run as different users; the Go process is
   what writes the files, so it needs write access to that directory.

## Running as a non-root user (recommended)

The repo's `senpan.service` specifies `User=senpan`/`Group=senpan`. Nothing in the
server needs root — it binds `:8080` (non-privileged) and only touches ordinary
files — so run it as a dedicated, locked system account to shrink the blast radius
of any compromise.

> **Important:** `scripts/deploy.ps1` does **NOT** ship the systemd unit — it only
> updates the binary (and syncs `.htaccess`). The repo `deploy/senpan.service` is a
> reference/template; the unit that actually runs lives on the host at
> `/etc/systemd/system/senpan.service`. So editing `User=` in the repo and
> redeploying does nothing to the running service — you must update the unit **on
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
#   placeholders — <service-user>, /opt/<app>, <webroot>, <service-name> — with
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
> `ReadWritePaths` is already broader than an older repo copy — run
> `systemctl cat senpan` and reconcile before shipping the repo unit so a
> `daemon-reload` never *narrows* the writable set.

## Credential rotation

- **Admin (and pre-existing staff) passwords.** Rotate the admin password to a
  fresh high-entropy value via the app's change-password flow, plus any staff
  accounts that predate a known exposure. Rotating the password alone does **not**
  invalidate already-issued session cookies — the session secret does (below).
- **`APPSUITE_SESSION_SECRET`.** When unset, the server generates a *random*
  secret on every boot, so all sessions drop on each restart and there is no
  operator-controlled secret to rotate. Set it explicitly in the unit's
  `Environment=` (or an `EnvironmentFile=`) to a stable high-entropy value, and
  rotate it (to a **new** value) in the same maintenance window as a password
  rotation — changing the secret is what forcibly logs out every outstanding
  session. The FFXIV plugin uses PAT bearer tokens, not the admin cookie, so it
  is unaffected.
- **`APPSUITE_TURNSTILE_SECRET` / `APPSUITE_TURNSTILE_SITEKEY`.** Configure the
  Cloudflare Turnstile pair (also settable as `-turnstile-secret`/`-turnstile-sitekey`
  flags) to enable the bot check on the public POST paths — login, registration,
  raffle sign-up, and custom card requests. When the **secret** is empty the check
  is disabled entirely (the server logs `Turnstile not configured` at boot and the
  frontend skips the widget), so a bad/rotated secret degrades to "no bot check,"
  not a lockout. The **secret** is sensitive; the **sitekey** is public (it is
  served to browsers via `GET /api/config`). Keep the secret out of the unit's
  inline `Environment=` (visible via `systemctl show` / `/proc/<pid>/environ`) —
  put both, alongside `APPSUITE_SESSION_SECRET`, in a root-owned `chmod 600` file
  loaded with `EnvironmentFile=-/etc/senpan/senpan.env` (see the commented block in
  `deploy/senpan.service`). Rotate the secret from the Cloudflare dashboard and
  update the env file.

## Font host (protected serving)

Uploaded fonts are licensed assets, so they are **never served as static
files**. The Go server streams them itself through obfuscated token URLs that
rotate every 1–2 weeks, gated by the requesting site's `Origin` against
**that font's own allowlist** (Atelier → Font Upload → Edit → Allowed sites).
Cross-origin `@font-face` loads hard-require CORS, so a site that isn't on a
font's list cannot render it; requests with no usable Origin (pasting the URL
into a browser, `wget`, casual scrapers) get a 403, and `kit.css` only emits
each requesting site's allowed fonts. The SPA itself loads fonts
**same-origin** via the existing `/api` ProxyPass
(`/api/fonts/pub/f/<token>`), so the app and its font picker always work — the
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
stale copies). The WOFF2 is served by default — uploads stay on disk
untouched, and the served version is selectable per font in the Edit modal.

One-time setup for the font host:

1. Make the `fonts.senpan.cafe` vhost a **reverse proxy** to the Go server
   (replacing any DocumentRoot-based static config):

   ```apache
   ProxyPass        / http://localhost:8080/api/fonts/pub/
   ProxyPassReverse / http://localhost:8080/api/fonts/pub/
   ProxyPreserveHost On
   ```

   `https://fonts.senpan.cafe/kit.css` → `/api/fonts/pub/kit.css` and
   `https://fonts.senpan.cafe/f/<token>` → `/api/fonts/pub/f/<token>`.
2. Upload `deploy/fonts.htaccess` → `<webRoot>/fonts/.htaccess`. It now DENIES
   all direct access — pure defense in depth in case the vhost is ever
   reverted to serving the folder statically. The Go server only adds/removes
   font files in that folder, so the `.htaccess` persists across uploads.

> **Migration note:** legacy direct URLs
> (`https://fonts.senpan.cafe/My%20Font.ttf`) stop working the moment the
> vhost becomes a proxy. Any external site using them must switch to the
> kit stylesheet above, and its origin must be added to the allowlist.

**Verify the cutover** (the repo can't confirm host-side vhost state):

```bash
# The gated responses MUST carry Cache-Control: private, or Cloudflare (which
# ignores Vary) can serve one origin's kit to another. Check this FIRST —
# it is a blocking precondition before migrating any external site.
curl -sI https://fonts.senpan.cafe/kit.css | grep -i cache-control     # → private

apachectl -S | grep fonts.senpan.cafe                                  # a proxy vhost, no DocumentRoot
curl -s -o /dev/null -w '%{http_code}\n' https://fonts.senpan.cafe/AnyFont.ttf   # → 403/404 (legacy URL dead)
curl -s -o /dev/null -w '%{http_code}\n' https://fonts.senpan.cafe/f/anytoken    # → 403 (no Origin)
curl -s -H 'Origin: https://<allowed-site>' https://fonts.senpan.cafe/kit.css    # → that site's fonts
```

## Carrd image host (CORS)

The **System → Carrd Upload** admin tab creates per-project folders under
`<webRoot>/carrd/` (each of which may contain arbitrarily nested sub-folders)
and writes uploaded images to them. They are served by a separate vhost at
`https://carrd.senpan.cafe/<folder>/.../<image>` and embedded by external Carrd
sites (a different origin), so cross-origin reads must be allowed.

One-time setup for the carrd host (same pattern as the font host):

1. Upload `deploy/carrd.htaccess` → `<webRoot>/carrd/.htaccess`. It adds
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
**System → Logs** tab and the on-box `jlv` tool both read this file;
`GET /api/logs` tails it and every line also streams to connected admins over the
WebSocket for a live tail, so most log inspection needs no SSH.

**Required systemd setting (`LogsDirectory`).** `senpan.service` runs with
`ProtectSystem=strict`, which mounts `/var/log` **read-only** for the service — so
the app cannot create its own log dir (it falls back to stdout-only and logs a
`file logging disabled … read-only file system` warning). Grant it a writable log
directory with systemd's `LogsDirectory`, which creates `/var/log/senpan` (owned
by the service, writable inside the sandbox) on every start — this survives
reboots and rebuilds:

```ini
# /etc/systemd/system/senpan.service.d/logdir.conf
[Service]
LogsDirectory=senpan
```

Then `systemctl daemon-reload && systemctl restart senpan`, and verify with
`tail /var/log/senpan/senpan.log` (JSON lines). A drop-in keeps the change out of
the packaged unit; equivalently add the `LogsDirectory=senpan` line to the unit's
`[Service]` section directly. **Without this, only the live tail / journald work —
the on-disk file, the Logs tab's historical snapshot, and `jlv` stay empty.**

On-box viewing: `jlv /var/log/senpan/senpan.log` (install the Linux `.deb` from
<https://github.com/hedhyw/json-log-viewer/releases>).

### Systemd sandboxing

`senpan.service` ships with a defense-in-depth sandbox: `NoNewPrivileges`,
`ProtectSystem=strict` + `ProtectHome`, `PrivateTmp`/`PrivateDevices`,
`ProtectKernel*`/`ProtectControlGroups`, `RestrictAddressFamilies=AF_INET AF_INET6
AF_UNIX`, `RestrictNamespaces`, `LockPersonality`, `SystemCallFilter=@system-service`
+ `SystemCallArchitectures=native`, and **`MemoryDenyWriteExecute=yes`** (W^X). Every
directory the app writes (the DB dir + the `-webroot` `images/`, `fonts/`, `carrd/`
subtrees, plus `LogsDirectory=senpan`) must be listed in `ReadWritePaths` or writes
fail with `EACCES` under `ProtectSystem=strict` — keep `ReadWritePaths` in sync with
your deployed `-webroot`.

> **`MemoryDenyWriteExecute` caveat.** W^X is safe here because the binary is pure
> Go (no cgo) and the SQLite driver (`ncruces/go-sqlite3` v0.35+) **transpiles**
> SQLite's WebAssembly to native Go ahead of time (`wasm2go`) — there is no runtime
> JIT and no writable-executable memory. If the SQLite driver is ever pinned back to
> an older, **wazero-JIT** version, `MemoryDenyWriteExecute=yes` will crash the
> service with `SIGSYS` on the first query and must be removed. Verify on the box
> with `systemctl show senpan -p MemoryDenyWriteExecute -p SystemCallArchitectures`.

### Who made each request (actor identity)

Every request line names the actor via an `auth` field (`session` | `token` |
`bot` | `anon`) plus `user` (account username) and `bot` (crawler) when they
apply. Admin actions resolve through the cookie session and plugin actions
through the personal access token — both automatic, no configuration. The
**System → Logs** tab surfaces this in a **User** column.

**Enabling verified-bot names (Cloudflare).** Anonymous requests are labelled
`bot` only when Cloudflare tells the origin the request is a *verified* good bot
(Googlebot, Bingbot, GPTBot, …). Cloudflare does not send that signal by default;
enable it one of two ways:

- **Any plan** — add a **Request Header Transform Rule**: Cloudflare dashboard →
  your zone → **Rules → Transform Rules → Modify Request Header → Create rule**.
  Set the filter to the `cf.client.bot` field (UI: *Verified Bot* *equals*
  *True*) and the action to **Set static** header `X-Verified-Bot` = `true`.
  Deploy. The backend reads `X-Verified-Bot` and names the bot from its (now
  Cloudflare-verified, so trustworthy) User-Agent.
- **Enterprise + Bot Management** — enable the **"Add bot protection headers"**
  managed transform (Rules → Managed Transforms). The backend also reads its
  native `cf-verified-bot` / `cf-verified-bot-category` headers, giving a cleaner
  category name — no code change.

This is a logging hint, never a security decision: like `CF-Connecting-IP`, the
header is forgeable by a client that reaches the origin without going through
Cloudflare. Lock the origin to Cloudflare's IP ranges (or Authenticated Origin
Pulls) if you need it to be tamper-proof. Capability tokens in request
paths/queries/Referer are redacted to a short correlation hash before logging, so
draw/stamp/font links never appear verbatim in the log or the viewer.

## Each deploy

**Scripted (recommended).** From the repo root, on Windows:

```powershell
.\scripts\deploy.ps1                  # frontend (default)
.\scripts\deploy.ps1 -Target backend  # Go binary + service restart
.\scripts\deploy.ps1 -Target main     # frontend, then backend ('both' is a kept alias)
.\scripts\deploy.ps1 -Target all      # frontend, backend, then plugin
.\scripts\deploy.ps1 -Target plugin   # Dalamud custom-repo files only
```

> **First-time setup:** the script is version-controlled, but **no**
> environment-specific value is baked into it — the host, SSH user, key path,
> webroot, service name, and opt dir all live outside the repo. Copy
> `scripts/deploy.config.example.ps1` to `scripts/deploy.config.ps1` (untracked)
> and fill in the six `$Deploy*` values. Each can instead be passed as a
> `-param` or set via its `$env:DEPLOY_*` variable (`DEPLOY_VPS_HOST`,
> `DEPLOY_VPS_USER`, `DEPLOY_KEY_PATH`, `DEPLOY_WEB_ROOT`,
> `DEPLOY_SERVICE_NAME`, `DEPLOY_REMOTE_OPT_DIR`); the script fails fast, listing
> any that are unset.

**Frontend** (`-Target frontend`, the default): builds `frontend/` (vue-tsc +
vite), uploads the result to a staging dir on the host (`<DocumentRoot>/dist.new`)
via PuTTY's `pscp` — so the DigitalOcean `.ppk` key works directly — verifies
every file arrived, then swaps it in (`dist` → `dist.old` rollback backup,
`dist.new` → `dist`). The build runs first, so a broken build never reaches the
server; only `dist/` is replaced — `images/` and `.htaccess` are never touched;
and the single `dist.old` backup is overwritten each deploy (no accumulation).

**Backend** (`-Target backend`): cross-compiles a static `linux/amd64` binary
(`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o app-suite .`
in `backend/`), stops `senpan.service`, backs up the current binary to
`app-suite.old`, uploads the new one, installs it at `/opt/senpan/app-suite`, and
starts the service — **rolling back to the previous binary if the new one fails
to stay active**. Brief downtime (a few seconds) while the service is stopped.

Each target uses just **three SSH connections** and does not poll during
transfers, to stay under `ufw limit ssh` (6 within 30s, which otherwise drops the
transfer with "Network error: Connection timed out"); a rate-limited connection
is retried once after a 35s wait. Defaults (set per operator, e.g. via the
script's parameters or environment): host `<your-droplet-ip>`, user `root`,
webroot `/var/www/apps.senpan.cafe`, service `senpan`, opt dir `/opt/senpan`, key
at `<path-to-your-deploy-key.ppk>` (override with
`-VpsHost` / `-VpsUser` / `-WebRoot` / `-KeyPath` / `-ServiceName` /
`-RemoteOptDir`). Use `-SkipBuild` to deploy an existing build, `-NoBackup` to
drop the rollback backup. Rollback by hand: frontend `mv dist.old dist`; backend
`mv app-suite.old app-suite && systemctl restart senpan` (on the host).

`pscp`/`plink` run in batch mode, so a passphrase-protected `.ppk` must be loaded
into PuTTY's agent first — run `pageant.exe "<KeyPath>"` once and enter the
passphrase (it stays unlocked for the session; a `shell:startup` shortcut loads
it at login). First-ever connection from PuTTY also needs the host key cached
once (`plink -i "<KeyPath>" root@<host>`, accept the fingerprint).

**Manual fallback.**

1. Locally: `cd frontend && npm run build`.
2. Upload the contents of `frontend/dist/` → `<DocumentRoot>/dist/` (replace).
   Leave `<DocumentRoot>/images/` and `<DocumentRoot>/.htaccess` untouched.

## Notes

- Image URLs are absolute (`/images/...`), served by Apache straight from the
  root `images/` folder — both the static assets and uploaded raffle prizes.
- Existing databases are auto-migrated (schema v10) to rewrite old
  `assets/images/raffles/...` prize paths to `images/raffles/...` on first start.
- `/api/*` and `/api/ws` are never handled by `.htaccess`; the vhost ProxyPass
  intercepts them first.
- **PWA**: the build also emits `sw.js`, `registerSW.js`, and
  `manifest.webmanifest` at the `dist/` root — they're uploaded as part of
  `dist/` and served at the document root by the `.htaccess` real-file rule. The
  caching block in `.htaccess` deliberately exempts `sw.js`/`registerSW.js`/
  `*.webmanifest` from the 1-year immutable cache so the app can update; if you
  re-copy an older `.htaccess`, keep those exemptions or the service worker will
  go stale. The PWA icons (`pwa-192x192.png`, `pwa-512x512.png`,
  `pwa-maskable-512x512.png`) and the iOS `apple-touch-icon.png` are served from
  the persistent root `images/` folder — make sure they're present there (they
  ship in `deploy/images/`).

