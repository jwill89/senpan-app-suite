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
│   ├── share_banner.png
│   └── raffles/               ← raffle prize uploads are written here by the Go server
└── dist/                      ← from frontend/dist/    (the built Vue app; replace on each deploy)
    ├── index.html
    └── assets/…               ← content-hashed JS/CSS
```

`images/` is a **sibling** of `dist/`, so re-uploading/replacing `dist/` never
touches uploaded raffle images. The build also strips `dist/images/` (it would
otherwise be a redundant copy) so the two never get confused.

Uploaded **fonts** live in a separate folder, `<webRoot>/fonts/`, served by its
own vhost at `https://fonts.senpan.cafe/` — see [Font host (CORS)](#font-host-cors).

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

## Font host (CORS)

Uploaded fonts are written by the Go server to `<webRoot>/fonts/` and served by
a separate vhost at `https://fonts.senpan.cafe/`. The SPA (at
`https://apps.senpan.cafe`) loads them via CSS `@font-face`, which — unlike
`<img>` — is **cross-origin/CORS-restricted**. Without an
`Access-Control-Allow-Origin` header the browser blocks the font and the
header/board font silently falls back to serif (e.g. the App Settings preview
shows the fallback instead of the uploaded font).

One-time setup for the font host:

1. Upload `deploy/fonts.htaccess` → `<webRoot>/fonts/.htaccess`. It adds
   `Access-Control-Allow-Origin: *` (plus a cache header) for font files. The Go
   server only adds/removes font files in that folder, so the `.htaccess`
   persists across uploads.
2. Ensure the font vhost has `mod_headers` enabled and allows `.htaccess`
   overrides (`AllowOverride All` or at least `FileInfo`). If overrides are
   disabled, put the `<FilesMatch>` block from `deploy/fonts.htaccess` directly
   in the vhost's `<Directory>` instead.

## Each deploy

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
  go stale. The PWA icons reference `/images/logo.png` + `/images/favicon.png`
  from the persistent root `images/` folder.

