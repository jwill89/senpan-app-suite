import { defineConfig } from '@playwright/test'
import { playwrightDefaults } from '../shared/test/e2e/playwright.js'
import { BACKEND_URL, SERVER, SITE_PORT, SITE_URL, serverEnv, serverFlags } from './e2e/env.ts'

const flags = Object.entries(serverFlags)
  .map(([flag, value]) => `${flag} "${value}"`)
  .join(' ')

// The end-to-end run (`npm run test:e2e`), on shared-tooling's Playwright settings: the
// Go API built from this checkout and started on a throwaway database and web root in
// the OS temp folder (e2e/server/prepare.ts), and the built SPA under `vite preview`,
// which proxies /api and the WebSocket to it as Apache does in production. Locally on
// Windows it drives the installed Edge.
export default defineConfig({
  ...playwrightDefaults,
  use: {
    ...playwrightDefaults.use,
    baseURL: SITE_URL,
    // The production build registers a service worker that precaches the shell; the
    // run checks the pages, not the worker.
    serviceWorkers: 'block',
  },
  webServer: [
    {
      // Never without -webhook-dry-run: nothing the run does may post to Discord.
      command: `node e2e/server/prepare.ts && "${SERVER}" ${flags} -webhook-dry-run`,
      url: `${BACKEND_URL}/api/version`,
      env: serverEnv,
      reuseExistingServer: false,
      timeout: 300_000,
    },
    {
      // vite.config.ts's proxy target, which `vite preview` uses too.
      command: `npx vite build --logLevel warn && npx vite preview --host 127.0.0.1 --port ${String(SITE_PORT)} --strictPort`,
      url: SITE_URL,
      env: { VITE_API_TARGET: BACKEND_URL },
      reuseExistingServer: false,
      timeout: 240_000,
    },
  ],
})
