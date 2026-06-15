import { fileURLToPath, URL } from 'node:url'
import { rm } from 'node:fs/promises'
import { readFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import { VitePWA } from 'vite-plugin-pwa'
import { visualizer } from 'rollup-plugin-visualizer'

// Static images (logo/favicon/share_banner) live in `public/images/` so they
// are available during `vite dev`. In production, they are served from a
// PERSISTENT `images/` folder that sits next to `dist/` at the Apache document
// root (see deploy/.htaccess), so the copies Vite would otherwise bake into
// `dist/images/` are redundant. This plugin removes `dist/images/` after the
// build so `dist/` contains only the SPA shell + hashed `assets/` — keeping the
// build output cleanly separate from the root `images/` folder.
function stripDistImages(): Plugin {
  return {
    name: 'strip-dist-images',
    apply: 'build',
    async closeBundle() {
      await rm(fileURLToPath(new URL('./dist/images', import.meta.url)), {
        recursive: true,
        force: true,
      })
    },
  }
}

// Social platforms (Discord, Twitter/X, Facebook…) aggressively cache an OG/
// Twitter card image by URL, so a replaced share banner keeps showing the stale
// one. This plugin appends a `?v=<hash>` cache-buster to the og:image/
// twitter:image URLs in index.html (replacing the `__OG_VERSION__` placeholder),
// derived from the SHA-256 of the actual share_banner.png. The query string
// doesn't affect file serving (Apache ignores it), but a new hash is a new URL
// to scrapers — so the card refreshes exactly when the image changes, and stays
// stable (no needless re-scrapes) when it doesn't. Falls back to a build
// timestamp if the file can't be read.
function ogImageCacheBust(): Plugin {
  let version = 'dev'
  try {
    const buf = readFileSync(
      fileURLToPath(new URL('./public/images/share_banner.png', import.meta.url)),
    )
    version = createHash('sha256').update(buf).digest('hex').slice(0, 12)
  } catch {
    version = Date.now().toString(36)
  }
  return {
    name: 'og-image-cache-bust',
    transformIndexHtml(html) {
      return html.replaceAll('__OG_VERSION__', version)
    },
  }
}

// https://vite.dev/config/
//
// The Go backend serves only `/api/*` (REST + WebSocket). During development, we
// proxy those to the Go server (default :8080) so the SPA can talk to it without
// CORS friction. In production the built `dist/` is served statically by Apache
// (with /api proxied to the Go server), so relative `api/...` URLs resolve.
//
// Static assets (logo, favicon, share banner) and uploaded raffle images live
// under `/images/` — copied verbatim from `public/` into `dist/` at build time.
// For uploaded-image preview to work in dev, run the Go server with
// `-webroot ../frontend/public` so uploads land in `public/images/raffles/`,
// which Vite serves directly (the proxy below is a fallback for other setups).
export default defineConfig({
  plugins: [
    vue(),
    // Installable PWA + offline app-shell. The service worker auto-updates on
    // each deploy (new precache manifest). API/WebSocket and the persistent
    // root images/ are explicitly excluded from the SPA navigation fallback so
    // they always hit the network (and Apache's proxy/static handling).
    VitePWA({
      registerType: 'autoUpdate',
      // Static images live in the persistent root images/ folder (stripped from
      // dist), so they are not bundled/precached — referenced by absolute URL.
      includeAssets: [],
      manifest: {
        name: 'Senpan App Suite',
        short_name: 'Senpan',
        description: 'Bingo Night + raffles for the Senpan Tea House.',
        theme_color: '#1a1c17',
        background_color: '#1a1c17',
        display: 'standalone',
        start_url: '/',
        scope: '/',
        icons: [
          // Generated as full-bleed resizes of the 512×512 favicon (see
          // deploy/images + public/images). The favicon already has its own
          // square, centered background (sage), so it doubles as the maskable
          // icon — the solid bg fills the mask's margins while the logo stays
          // centered (no extra padding/letter-boxing, no white-on-white).
          { src: '/images/pwa-192x192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
          { src: '/images/pwa-512x512.png', sizes: '512x512', type: 'image/png', purpose: 'any' },
          {
            src: '/images/pwa-maskable-512x512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable',
          },
        ],
      },
      workbox: {
        // Precache the built SPA shell + hashed assets.
        globPatterns: ['**/*.{js,css,html,svg,woff,woff2}'],
        // SPA deep-link fallback, but never intercept the API or root images.
        navigateFallback: '/index.html',
        navigateFallbackDenylist: [/^\/api\//, /^\/images\//],
        cleanupOutdatedCaches: true,
      },
    }),
    stripDistImages(),
    ogImageCacheBust(),
    // `npm run analyze` writes dist/stats.html with a treemap of bundle sizes.
    ...(process.env.ANALYZE
      ? [
          visualizer({
            filename: 'dist/stats.html',
            gzipSize: true,
            brotliSize: true,
          }) as Plugin,
        ]
      : []),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  // Absolute base: required for Vue Router history mode so deep-link refreshes
  // (e.g. /admin/cards) resolve hashed assets from the document root /assets/
  // rather than relative to the current path. Apache serves the SPA at the
  // document root and falls back to index.html for unknown paths (deploy/.htaccess).
  base: '/',
  server: {
    port: 5173,
    proxy: {
      // WebSocket upgrade for /api/ws — MUST come before the '/api' entry so it
      // matches first. `changeOrigin` is left false here on purpose: the Go hub
      // (coder/websocket) enforces a same-origin check (the request's Origin host
      // must equal its Host header). Rewriting Host to the target (:8080) — as the
      // REST proxy below does — would leave Origin as the browser's :5173 and fail
      // that check (403 → the socket drops → "Connection lost. Reconnecting").
      // Preserving Host keeps it equal to Origin, mirroring production's Apache
      // `ProxyPreserveHost On`, so the check passes without weakening it.
      '/api/ws': {
        target: process.env.VITE_API_TARGET || 'http://localhost:8080',
        ws: true,
        changeOrigin: false,
      },
      // REST API → Go backend
      '/api': {
        target: process.env.VITE_API_TARGET || 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
    // The two largest chunks (Toast UI editor ~570 kB, CodeMirror ~440 kB) are
    // monolithic third-party libraries that are already split into their own
    // lazy-loaded chunks (fetched only when an admin opens a view that needs
    // them) — they never touch the initial player/home load. Neither can be
    // split further (each ships as one bundle), so we lift the advisory warning
    // to 600 kB: above these intentional vendor chunks, but still low enough to
    // flag genuinely new bloat.
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        // Split heavy, rarely-changing vendor libs into their own chunks so
        // they cache independently of app code (CodeMirror + FontAwesome are
        // only needed in the admin views). Purely a caching/loading win — no
        // behavioural change.
        manualChunks: {
          vue: ['vue', 'pinia', 'vue-router'],
          codemirror: [
            'codemirror',
            'vue-codemirror',
            '@codemirror/lang-css',
            '@codemirror/language',
            '@codemirror/state',
            '@codemirror/view',
            '@lezer/highlight',
            '@replit/codemirror-css-color-picker',
          ],
          fontawesome: [
            '@fortawesome/fontawesome-svg-core',
            '@fortawesome/free-brands-svg-icons',
          ],
          markdown: ['markdown-it'],
          draggable: ['vuedraggable', 'sortablejs'],
          // Toast UI editor (lazy-loaded by MarkdownEditor.vue) — named so it
          // caches independently and is clearly identifiable in the build output.
          toastui: ['@toast-ui/editor'],
          // Emoji picker (lazy-loaded by StampShapePicker.vue) — same rationale.
          emojipicker: ['vue3-emoji-picker'],
        },
      },
    },
  },
})
