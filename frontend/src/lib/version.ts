/**
 * App version info shown in the admin dashboard.
 *
 * The frontend (this SPA) and the backend (Go API) are versioned independently
 * with semver. `FRONTEND_VERSION` is baked in from package.json at build time;
 * the backend version is fetched from `GET /api/version`. The admin sidebar
 * shows both and flags a mismatch so operators can confirm the two halves are
 * compatible after a partial deploy.
 */

/** This build's frontend version (package.json → vite `define`). Falls back to a
 *  dev sentinel if the define is absent (e.g. an un-configured runtime). */
export const FRONTEND_VERSION: string =
  typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : '0.0.0-dev'

/** The MAJOR component of a semver string, or null if it doesn't parse. */
export function majorOf(version: string): number | null {
  const m = /^\s*v?(\d+)\./.exec(version || '')
  return m ? Number(m[1]) : null
}

/**
 * Whether a frontend and backend version are compatible. The contract: the
 * SPA and API agree as long as their MAJOR versions match (a major bump is
 * reserved for a breaking API change). Unknown/unparseable versions are treated
 * as compatible so a probe failure never raises a false alarm.
 */
export function versionsCompatible(frontend: string, backend: string): boolean {
  const fe = majorOf(frontend)
  const be = majorOf(backend)
  if (fe === null || be === null) return true
  return fe === be
}
