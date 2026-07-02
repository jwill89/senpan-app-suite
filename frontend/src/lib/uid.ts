/**
 * Stable client-side row ids for `v-for` keys.
 *
 * Repeatable form rows (prizes, owners, sources, …) are keyed by a `_uid` rather
 * than the array index so that removing a middle row doesn't make Vue reuse the
 * wrong row's DOM/input state. The counter is process-wide and monotonic; the
 * `_uid` is a purely client-side concern and is stripped before any payload is
 * sent to the server.
 */
let counter = 0

/** Returns the next unique client-side row id. */
export function nextUid(): number {
  return ++counter
}
