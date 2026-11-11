/**
 * The app's version, shown in the admin dashboard.
 *
 * The SPA and the API are one app, built into one binary, with one version:
 * package.json's, baked into this build as `APP_VERSION`. The server reports the
 * version it runs (GET /api/v1/version), so a tab opened before a deploy can tell it
 * is behind the server, and the admin footer says so.
 */

/** This build's version (package.json -> vite `define`). Falls back to a dev sentinel
 *  if the define is absent (e.g. an un-configured runtime). */
export const APP_VERSION: string =
  typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : '0.0.0-dev'

/**
 * Whether the server runs another version than this tab's build: the tab was opened
 * before a deploy (or after a rollback), so a reload brings the page and the API back
 * together. An unknown server version (null) is never a false alarm.
 */
export function serverDiffers(server: string | null): boolean {
  return server !== null && server !== APP_VERSION
}
