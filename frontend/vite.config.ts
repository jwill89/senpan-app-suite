import { fileURLToPath, URL } from 'node:url'
import { rm } from 'node:fs/promises'
import { readFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import { VitePWA } from 'vite-plugin-pwa'
import { visualizer } from 'rollup-plugin-visualizer'
import { changelogPlugin } from './config/changelog-plugin.ts'

// Static images (logo/favicon/share_banner) live in `public/images/` so they
// are available during `vite dev`. In production, they are served from a
// PERSISTENT `images/` folder that sits next to `dist/` at the Apache document
// root (see deploy/.htaccess), so the copies Vite would otherwise bake into
// `dist/images/` are redundant. This plugin removes `dist/images/` after the
// build so `dist/` contains only the SPA shell + hashed `assets/` - keeping the
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

// Social platforms (Discord, Twitter/X, Facebook...) aggressively cache an OG/
// Twitter card image by URL, so a replaced share banner keeps showing the stale
// one. This plugin appends a `?v=<hash>` cache-buster to the og:image/
// twitter:image URLs in index.html (replacing the `__OG_VERSION__` placeholder),
// derived from the SHA-256 of the actual share_banner.png. The query string
// doesn't affect file serving (Apache ignores it), but a new hash is a new URL
// to scrapers - so the card refreshes exactly when the image changes, and stays
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

// Heavy, rarely-changing vendor libs each get their own chunk so they cache
// independently of app code (FontAwesome is only needed in the admin views;
// Milkdown + the emoji picker are lazy-loaded). Vite 8 / Rolldown dropped the
// object form of `manualChunks`, so the same grouping is expressed as a function
// that maps a module's node_modules package to its chunk. Purely a
// caching/loading win - no behavioural change.
const vendorChunkGroups: ReadonlyArray<readonly [string, readonly string[]]> = [
  ['vue', ['vue', '@vue', 'pinia', 'vue-router']],
  ['fontawesome', ['@fortawesome']],
  ['markdown', ['markdown-it']],
  ['draggable', ['vue-draggable-plus', 'sortablejs']],
  // Milkdown (Crepe) WYSIWYG editor + its ProseMirror/KaTeX engine.
  ['milkdown', ['@milkdown', 'prosemirror-', 'katex']],
  ['emojipicker', ['vue3-emoji-picker']],
]

function manualChunks(id: string): string | undefined {
  for (const [chunk, packages] of vendorChunkGroups) {
    // A token ending in `-` is a package-name prefix (e.g. `prosemirror-*`);
    // otherwise it's a whole package/scope name (trailing slash anchors it).
    const hit = packages.some((pkg) =>
      pkg.endsWith('-')
        ? id.includes(`/node_modules/${pkg}`)
        : id.includes(`/node_modules/${pkg}/`),
    )
    if (hit) return chunk
  }
  return undefined
}

// Output files a public visitor can never reach. Filled during the build and read
// by the service worker's `manifestTransforms` below.
//
// Precaching is a SEPARATE question from code-splitting, and the two disagreed.
// The admin bundles are lazy-loaded, so they never touch a player's initial page
// load - but `globPatterns` swept every emitted file into the precache, so the
// service worker downloaded them anyway on first visit: ~1.2 MB of admin-only
// JavaScript plus the KaTeX web faces, for visitors who cannot open the admin at
// all.
//
// This is computed from the real module graph rather than from file-name
// patterns, because the names do not carry the answer: `DataTable` is admin-only
// while `emojipicker` and `fontawesome` - which look like admin tooling - are
// both reachable from public views.
const adminOnlyOutput = new Set<string>()

function markAdminOnlyOutput(): Plugin {
  type Chunk = {
    isEntry: boolean
    facadeModuleId: string | null
    imports: string[]
    dynamicImports: string[]
    viteMetadata?: { importedCss?: Set<string>; importedAssets?: Set<string> }
  }
  return {
    name: 'mark-admin-only-output',
    apply: 'build',
    generateBundle(_options, bundle) {
      const chunks = new Map<string, Chunk>()
      for (const [file, output] of Object.entries(bundle))
        if (output.type === 'chunk') chunks.set(file, output)

      const facadeOf = (file: string) =>
        (chunks.get(file)?.facadeModuleId || '').split('\\').join('/')
      const isAdmin = (file: string) =>
        facadeOf(file).includes('/components/admin/') || facadeOf(file).includes('/AdminView.')

      // The entry dynamically imports the router, which in turn dynamically
      // imports AdminView - so an unrestricted walk reaches every chunk and
      // proves nothing. Admin chunks are barriers: what a public visitor can
      // reach is what the walk finds WITHOUT passing through one.
      const reachable = new Set<string>()
      const queue = [...chunks.keys()].filter(
        (f) => chunks.get(f)?.isEntry || (facadeOf(f).includes('/views/') && !isAdmin(f)),
      )
      while (queue.length) {
        const file = queue.pop()
        if (!file || reachable.has(file) || !chunks.has(file) || isAdmin(file)) continue
        reachable.add(file)
        const chunk = chunks.get(file)
        if (chunk) queue.push(...chunk.imports, ...chunk.dynamicImports)
      }

      // A chunk's stylesheet and the fonts that stylesheet pulls ride with it:
      // the KaTeX faces (~546 kB) are reached only through Milkdown's CSS. A
      // sidecar shared with anything public stays precached.
      const sidecarsOf = (file: string) => [
        ...(chunks.get(file)?.viteMetadata?.importedCss ?? []),
        ...(chunks.get(file)?.viteMetadata?.importedAssets ?? []),
      ]
      const publicSidecars = new Set([...reachable].flatMap(sidecarsOf))
      adminOnlyOutput.clear()
      for (const file of chunks.keys()) {
        if (reachable.has(file)) continue
        adminOnlyOutput.add(file)
        for (const sidecar of sidecarsOf(file))
          if (!publicSidecars.has(sidecar)) adminOnlyOutput.add(sidecar)
      }
    },
  }
}

// Proves the exclusion above actually reached the generated service worker.
//
// It rests on two things that are not ours: that `generateBundle` runs before
// vite-plugin-pwa's `closeBundle`, and that workbox still honours
// `manifestTransforms`. If either stops holding, the precache silently returns to
// carrying every admin bundle - no error, no visible symptom, just ~1.8 MB back
// on every first visit. Registered AFTER VitePWA so sw.js exists by the time this
// runs; `closeBundle` hooks fire in plugin order.
function verifyAdminNotPrecached(): Plugin {
  let outDir = 'dist'
  return {
    name: 'verify-admin-not-precached',
    apply: 'build',
    enforce: 'post',
    configResolved(config) {
      outDir = config.build.outDir
    },
    closeBundle() {
      const sw = fileURLToPath(new URL(`./${outDir}/sw.js`, import.meta.url))
      let source: string
      try {
        source = readFileSync(sw, 'utf-8')
      } catch {
        // Not "no PWA in this build" - this plugin is ordered after the one that
        // writes sw.js, so by here it must exist. Reading a stale copy from an
        // earlier build is exactly how this check silently passed while the
        // exclusion was broken, so a missing file is a failure, not a skip.
        this.error(`expected a generated service worker at ${sw}, found none`)
        return
      }
      const precached = new Set([...source.matchAll(/url:"([^"]+)"/g)].map((match) => match[1]))
      const leaked = [...adminOnlyOutput].filter((file) => precached.has(file))
      if (leaked.length)
        this.error(
          `${leaked.length} admin-only file(s) reached the service-worker precache, ` +
            `so every visitor would download them: ${leaked.slice(0, 3).join(', ')}` +
            (leaked.length > 3 ? ', ...' : ''),
        )
      if (!adminOnlyOutput.size)
        this.error(
          'no admin-only output was identified - the reachability walk in ' +
            'markAdminOnlyOutput is no longer finding the admin chunks',
        )
    },
  }
}

// The frontend's semantic version, read from package.json and baked into the
// bundle as the global `__APP_VERSION__` (see env.d.ts). The admin dashboard
// shows it next to the backend version for a compatibility check; bump
// package.json's "version" in the same change as a matching CHANGELOG.md entry.
const frontendVersion = JSON.parse(
  readFileSync(fileURLToPath(new URL('./package.json', import.meta.url)), 'utf-8'),
).version as string

// https://vite.dev/config/
//
// The Go backend serves only `/api/*` (REST + WebSocket). During development, we
// proxy those to the Go server (default :8080) so the SPA can talk to it without
// CORS friction. In production the built `dist/` is served statically by Apache
// (with /api proxied to the Go server), so relative `api/...` URLs resolve.
//
// Static assets (logo, favicon, share banner) and uploaded raffle images live
// under `/images/` - copied verbatim from `public/` into `dist/` at build time.
// For uploaded-image preview to work in dev, run the Go server with
// `-webroot ../frontend/public` so uploads land in `public/images/raffles/`,
// which Vite serves directly (the proxy below is a fallback for other setups).
export default defineConfig({
  plugins: [
    vue(),
    changelogPlugin(),
    // Must precede VitePWA: it fills `adminOnlyOutput` in `generateBundle`, which
    // the PWA plugin's manifest transform reads later, at `closeBundle`.
    markAdminOnlyOutput(),
    // Installable PWA + offline app-shell. The service worker auto-updates on
    // each deploy (new precache manifest). API/WebSocket and the persistent
    // root images/ are explicitly excluded from the SPA navigation fallback so
    // they always hit the network (and Apache's proxy/static handling).
    VitePWA({
      registerType: 'autoUpdate',
      // Static images live in the persistent root images/ folder (stripped from
      // dist), so they are not bundled/precached - referenced by absolute URL.
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
          // Generated as full-bleed resizes of the 512x512 favicon (see
          // deploy/images + public/images). The favicon already has its own
          // square, centered background (sage), so it doubles as the maskable
          // icon - the solid bg fills the mask's margins while the logo stays
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
        // ...minus the admin-only half of them (see markAdminOnlyOutput). A
        // player or public visitor never downloads the admin bundles; an admin
        // fetches them on demand and the runtime rule below keeps them, so they
        // still work offline after one visit.
        manifestTransforms: [
          (entries) => ({
            manifest: entries.filter((entry) => !adminOnlyOutput.has(entry.url)),
            warnings: [],
          }),
        ],
        runtimeCaching: [
          {
            // Everything under assets/ is content-hashed, so a cached copy can
            // never be stale - a changed file is a changed URL. This is what
            // catches the admin bundles the precache no longer carries.
            urlPattern: /\/assets\/.+\.(?:js|css|woff2?|ttf)$/,
            handler: 'CacheFirst',
            options: {
              cacheName: 'app-assets',
              expiration: { maxEntries: 150, maxAgeSeconds: 60 * 60 * 24 * 30 },
            },
          },
        ],
        // SPA deep-link fallback, but never intercept the API or root images.
        navigateFallback: '/index.html',
        navigateFallbackDenylist: [/^\/api\//, /^\/images\//],
        cleanupOutdatedCaches: true,
      },
    }),
    verifyAdminNotPrecached(),
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
  define: {
    __APP_VERSION__: JSON.stringify(frontendVersion),
  },
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
      // WebSocket upgrade for /api/ws - MUST come before the '/api' entry so it
      // matches first. `changeOrigin` is left false here on purpose: the Go hub
      // (coder/websocket) enforces a same-origin check (the request's Origin host
      // must equal its Host header). Rewriting Host to the target (:8080) - as the
      // REST proxy below does - would leave Origin as the browser's :5173 and fail
      // that check (403 -> the socket drops -> "Connection lost. Reconnecting").
      // Preserving Host keeps it equal to Origin, mirroring production's Apache
      // `ProxyPreserveHost On`, so the check passes without weakening it.
      '/api/ws': {
        target: process.env.VITE_API_TARGET || 'http://localhost:8080',
        ws: true,
        changeOrigin: false,
      },
      // REST API -> Go backend
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
    // The largest chunk (the Milkdown editor ~620 kB) is a monolithic
    // third-party library already split into its own lazy-loaded chunk (fetched
    // only when an admin opens a view that needs it) - it never touches the
    // initial player/home load. It can't be split further (it ships as one
    // bundle), so we lift the advisory warning to 650 kB: above this intentional
    // vendor chunk, but still low enough to flag genuinely new bloat.
    chunkSizeWarningLimit: 650,
    rollupOptions: {
      // Vendor-chunk grouping lives in `manualChunks` above (Milkdown and the
      // emoji picker are additionally lazy-loaded by MarkdownEditor.vue /
      // StampShapePicker.vue, so they're fetched only when those views open).
      output: { manualChunks },
    },
  },
})
