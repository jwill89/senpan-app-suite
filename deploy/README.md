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

## Each deploy

**Scripted (recommended).** From the repo root, on Windows:

```powershell
.\scripts\deploy.ps1                  # frontend (default)
.\scripts\deploy.ps1 -Target backend  # Go binary + service restart
.\scripts\deploy.ps1 -Target both     # frontend, then backend
```

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
is retried once after a 35s wait. Defaults: host `68.183.138.141`, user `root`,
webroot `/var/www/apps.senpan.cafe`, service `senpan`, opt dir `/opt/senpan`, key
at `C:\Users\jwill\OneDrive\Documents\digitalocean-key.ppk` (override with
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

