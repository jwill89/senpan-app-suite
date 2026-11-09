import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

/**
 * Where the end-to-end run serves the suite: the app as production runs it, the Go
 * binary serving the API and the SPA it embeds. It is built from this checkout (after
 * the front end's build, which it embeds) into the OS temp folder and runs there on a
 * throwaway database and web root, on its own port, with every Discord post held back.
 * The web root's images, which Apache serves in production, are served from that web
 * root by the `images` fixture.
 */
export const PORT = 8184
export const SITE_URL = `http://127.0.0.1:${String(PORT)}`

export const ROOT = resolve(import.meta.dirname, '../..')
export const DATA_DIR = join(tmpdir(), 'senpan-e2e')
export const DATABASE = join(DATA_DIR, 'database.sqlite')
export const WEB_ROOT = join(DATA_DIR, 'webroot')
export const LOG_FILE = join(DATA_DIR, 'senpan.log')
export const SERVER = join(DATA_DIR, process.platform === 'win32' ? 'app-suite.exe' : 'app-suite')

/** The account migration v22 seeds on a new database; the run's staff sign in with it. */
export const STAFF = { username: 'admin', password: 'admin' }

/**
 * The flags `task dev:backend` runs the API with, every path the run's own. The server's
 * defaults point at the production layout, and devdata/ is a copy of the live data, so
 * each one is given. `-webhook-dry-run` logs every Discord post instead of sending it.
 */
export const serverFlags: Record<string, string> = {
  '-addr': `127.0.0.1:${String(PORT)}`,
  '-db': DATABASE,
  '-webroot': WEB_ROOT,
  '-log-file': LOG_FILE,
}

/**
 * The environment the API starts with, over the one it is started from: each setting
 * main.go falls back to an environment variable for is set here, so nothing in the
 * shell can turn on Turnstile, open CORS or redirect a webhook to a real channel.
 */
export const serverEnv: Record<string, string> = {
  APPSUITE_TURNSTILE_SECRET: '',
  APPSUITE_TURNSTILE_SITEKEY: '',
  APPSUITE_CORS_ORIGINS: '',
  APPSUITE_WEBHOOK_OVERRIDE: '',
  APPSUITE_WEBHOOK_DRY_RUN: '1',
}
