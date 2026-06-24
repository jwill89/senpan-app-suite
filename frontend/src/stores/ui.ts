/**
 * UI store: global toast notifications + clipboard helper.
 *
 * Top-level view routing previously lived here (`view` / `setView`) but is now
 * handled by Vue Router (see `src/router`). Toasts auto-dismiss after 5.5s so
 * admins have time to read them before they fade.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'

export type ToastType = 'info' | 'success' | 'error'

/** Options for a themed confirm dialog (replaces the native window.confirm). */
export interface ConfirmOptions {
  title?: string
  /** Label for the confirm button (default "Confirm"). */
  confirmText?: string
  /** Label for the cancel button (default "Cancel"). */
  cancelText?: string
  /** Style the confirm button as destructive (red). Default true. */
  danger?: boolean
}

interface ConfirmState extends Required<ConfirmOptions> {
  show: boolean
  message: string
}

export const useUiStore = defineStore('ui', () => {
  const toast = ref<{ show: boolean; message: string; type: ToastType }>({
    show: false,
    message: '',
    type: 'info',
  })
  let toastTimer: ReturnType<typeof setTimeout> | undefined

  // ── Route loading ──────────────────────────────────────────────────────────
  // True while a route navigation is in flight (async guard + lazy chunk fetch),
  // driving the top progress bar. Set from the router guards (see src/router).
  const routeLoading = ref(false)
  function setRouteLoading(v: boolean): void {
    routeLoading.value = v
  }

  // ── Realtime connection status ───────────────────────────────────────────────
  // Reflects the shared WebSocket state so the player view can show a "Live" /
  // "Reconnecting…" badge. Driven by WsClient via the useWebSocket composable.
  const wsStatus = ref<'closed' | 'connecting' | 'open' | 'reconnecting'>('closed')
  function setWsStatus(s: 'closed' | 'connecting' | 'open' | 'reconnecting'): void {
    wsStatus.value = s
  }

  // ── Themed confirm dialog ──────────────────────────────────────────────────
  const confirmState = ref<ConfirmState>({
    show: false,
    message: '',
    title: 'Are you sure?',
    confirmText: 'Confirm',
    cancelText: 'Cancel',
    danger: true,
  })
  let confirmResolve: ((value: boolean) => void) | null = null

  /**
   * Shows a themed confirmation modal and resolves to the user's choice.
   * Drop-in async replacement for the native `window.confirm`.
   *
   * @example if (!(await ui.confirm('Delete this?'))) return
   */
  function confirm(message: string, opts: ConfirmOptions = {}): Promise<boolean> {
    // If a previous confirm is somehow open, resolve it false first.
    if (confirmResolve) confirmResolve(false)
    confirmState.value = {
      show: true,
      message,
      title: opts.title ?? 'Are you sure?',
      confirmText: opts.confirmText ?? 'Confirm',
      cancelText: opts.cancelText ?? 'Cancel',
      danger: opts.danger ?? true,
    }
    return new Promise<boolean>((resolve) => {
      confirmResolve = resolve
    })
  }

  /** Resolves the open confirm dialog with the given result and closes it. */
  function resolveConfirm(result: boolean): void {
    confirmState.value.show = false
    const r = confirmResolve
    confirmResolve = null
    if (r) r(result)
  }

  /**
   * Displays a toast that auto-dismisses after `duration` ms (default 5.5s).
   * Pass a longer duration for important messages (e.g. winner alerts). The
   * toast can also be dismissed early by the user via `dismissToast()`.
   */
  function notify(message: string, type: ToastType = 'info', duration = 5500): void {
    clearTimeout(toastTimer)
    toast.value = { show: true, message, type }
    toastTimer = setTimeout(() => {
      toast.value.show = false
    }, duration)
  }

  /** Hides the current toast immediately (e.g. when the user clicks it). */
  function dismissToast(): void {
    clearTimeout(toastTimer)
    toast.value.show = false
  }

  /** Copies text to the clipboard and shows a toast. Pass `successMsg` to combine
   *  the copy confirmation into a caller-specific message (one toast, not two). */
  function copyToClipboard(text: string, successMsg = 'Copied to clipboard!'): void {
    navigator.clipboard
      .writeText(text)
      .then(() => notify(successMsg, 'success'))
      .catch(() => notify('Failed to copy', 'error'))
  }

  return {
    toast,
    confirmState,
    routeLoading,
    setRouteLoading,
    wsStatus,
    setWsStatus,
    notify,
    dismissToast,
    confirm,
    resolveConfirm,
    copyToClipboard,
  }
})
