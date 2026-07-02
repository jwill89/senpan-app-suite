/**
 * Shared "load behind a spinner" wrapper for the admin stores.
 *
 * Every plain list loader repeated the same shape: flip a loading flag on, fetch
 * + assign, surface any thrown error as an error toast, and clear the flag in a
 * `finally`. This owns that boilerplate so a loader is just its fetch + assign.
 */
import type { Ref } from 'vue'
import { useUiStore } from '@/stores/ui'

/**
 * Runs `task` with `flag` held true for its duration. Any error thrown by `task`
 * is surfaced as an error toast (matching the stores' existing behavior); the
 * flag is always cleared when `task` settles.
 */
export async function withLoading(flag: Ref<boolean>, task: () => Promise<void>): Promise<void> {
  flag.value = true
  try {
    await task()
  } catch (e) {
    useUiStore().notify((e as Error).message, 'error')
  } finally {
    flag.value = false
  }
}
