/**
 * Shared "load behind a spinner" wrapper for the admin stores.
 *
 * Every plain list loader repeated the same shape: flip a loading flag on, fetch
 * + assign, surface any thrown error as an error toast, and clear the flag in a
 * `finally`. This owns that boilerplate so a loader is just its fetch + assign.
 */
import type { Ref } from 'vue'
import { useUiStore } from '@/stores/ui'

// Reference-counts the concurrent holders of each loading flag. Two overlapping
// withLoading calls on the same flag would otherwise fight in their `finally`:
// the first to settle would clear the flag while the other is still running.
// The flag reads true while any holder is active and only flips false when the
// last one settles.
const depth = new WeakMap<Ref<boolean>, number>()

/**
 * Runs `task` with `flag` held true for its duration. Any error thrown by `task`
 * is surfaced as an error toast (matching the stores' existing behavior); the
 * flag is only cleared once every concurrent `withLoading` on it has settled.
 */
export async function withLoading(flag: Ref<boolean>, task: () => Promise<void>): Promise<void> {
  depth.set(flag, (depth.get(flag) ?? 0) + 1)
  flag.value = true
  try {
    await task()
  } catch (e) {
    useUiStore().notify((e as Error).message, 'error')
  } finally {
    const n = (depth.get(flag) ?? 1) - 1
    if (n > 0) {
      depth.set(flag, n)
    } else {
      depth.delete(flag)
      flag.value = false
    }
  }
}
