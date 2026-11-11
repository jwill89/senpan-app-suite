import { defineConfig } from '@playwright/test'
import { playwrightDefaults } from '../shared/test/e2e/playwright.js'
import { SERVER, SITE_URL, serverEnv, serverFlags } from './e2e/env.ts'

const flags = Object.entries(serverFlags)
  .map(([flag, value]) => `${flag} "${value}"`)
  .join(' ')

// The end-to-end run (`npm run test:e2e`), on the shared Playwright settings: the
// app as production runs it - the Go binary, built from this checkout with the front
// end's build embedded (build it first: `task test:e2e` does), serving the API, its
// live event streams and the SPA - started on a throwaway database and data folder in
// the OS temp folder (e2e/server/prepare.ts). Locally on Windows it drives the
// installed Edge.
export default defineConfig({
  ...playwrightDefaults,
  use: {
    ...playwrightDefaults.use,
    baseURL: SITE_URL,
    // The production build registers a service worker that precaches the shell; the
    // run checks the pages, not the worker.
    serviceWorkers: 'block',
  },
  webServer: {
    // Never without -webhook-dry-run: nothing the run does may post to Discord.
    command: `node e2e/server/prepare.ts && "${SERVER}" ${flags} -webhook-dry-run`,
    url: `${SITE_URL}/healthz`,
    env: serverEnv,
    reuseExistingServer: false,
    timeout: 300_000,
  },
})
