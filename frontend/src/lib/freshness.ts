/**
 * Minimal time-based freshness gate for data loads.
 *
 * Admin tabs re-run their data loader every time they're entered, which makes
 * switching tabs (or bouncing between two that share data) flash a spinner and
 * re-download unchanged rows. A freshness gate lets the navigation layer skip
 * the reload when the data was fetched within `ttlMs`, so revisits are instant.
 *
 * It deliberately lives at the navigation layer, not inside the store loaders:
 * explicit reloads after a create/edit/delete call the loaders directly and so
 * always hit the server (edits show immediately), and the live game/cards/
 * patterns data stays current over WebSocket regardless of this gate.
 *
 * Keys let one gate cover several independent datasets (e.g. one per admin tab,
 * or one per book club slug).
 */
export function createFreshness(ttlMs = 30_000) {
  const stamps = new Map<string, number>()
  return {
    /** True when `key` has no fresh stamp — i.e. the caller should (re)load. */
    isStale(key = ''): boolean {
      const at = stamps.get(key)
      return at === undefined || Date.now() - at >= ttlMs
    },
    /** Stamp `key` as freshly loaded as of now. */
    touch(key = ''): void {
      stamps.set(key, Date.now())
    },
    /** Drop `key`'s stamp so the next `isStale` check reloads. */
    invalidate(key = ''): void {
      stamps.delete(key)
    },
  }
}

export type Freshness = ReturnType<typeof createFreshness>
