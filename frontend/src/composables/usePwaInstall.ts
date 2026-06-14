/**
 * PWA install helper.
 *
 * Wraps the (non-standard) `beforeinstallprompt` flow so the UI can offer an
 * explicit "Install App" action instead of relying on the browser's mini-infobar:
 *
 *   - Chromium (Android/desktop) fires `beforeinstallprompt` before any component
 *     mounts, so we capture + defer it at MODULE load (singleton) and expose it
 *     reactively. Calling `promptInstall()` shows the native install dialog.
 *   - iOS Safari has no install API at all — apps are added via Share → "Add to
 *     Home Screen". There we surface manual instructions instead (the caller
 *     checks `needsIosInstructions`).
 *   - When the app is already running installed (standalone display-mode, or
 *     iOS's `navigator.standalone`), nothing is shown.
 *
 * `showInstall` additionally gates to mobile devices so the link only appears
 * where "add to home screen" is meaningful.
 */
import { computed, ref } from 'vue'

/** Minimal shape of the non-standard `beforeinstallprompt` event. */
interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed'; platform: string }>
}

// Module-level singletons: the deferred prompt can fire before any component
// mounts, so we capture it exactly once here and share it across all callers.
const deferredPrompt = ref<BeforeInstallPromptEvent | null>(null)
const installed = ref(false)
let registered = false

function registerListeners(): void {
  if (registered || typeof window === 'undefined') return
  registered = true
  window.addEventListener('beforeinstallprompt', (e: Event) => {
    // Suppress Chrome's default mini-infobar so we can trigger the prompt from
    // our own footer link on demand.
    e.preventDefault()
    deferredPrompt.value = e as BeforeInstallPromptEvent
  })
  window.addEventListener('appinstalled', () => {
    installed.value = true
    deferredPrompt.value = null
  })
}
registerListeners()

/** True when the app is already running as an installed PWA. */
function isStandalone(): boolean {
  if (typeof window === 'undefined') return false
  return (
    window.matchMedia?.('(display-mode: standalone)').matches ||
    // iOS Safari exposes its own standalone flag rather than display-mode.
    (window.navigator as Navigator & { standalone?: boolean }).standalone === true
  )
}

/** iPhone/iPod/iPad — including iPadOS 13+ which reports as desktop Safari. */
function isIosDevice(): boolean {
  if (typeof navigator === 'undefined') return false
  const ua = navigator.userAgent
  return /iphone|ipod|ipad/i.test(ua) || (/Macintosh/.test(ua) && navigator.maxTouchPoints > 1)
}

/**
 * Coarse mobile/tablet check — where "add to home screen" makes sense.
 *
 * We deliberately do NOT rely on the user-agent string alone: privacy-focused
 * browsers (e.g. Vivaldi) mask/omit the usual "Android"/"Mobile" tokens, so a UA
 * regex misses them. A touchscreen with a coarse pointer is a far more reliable
 * signal for a phone/tablet, so we treat either signal as mobile.
 */
function isMobile(): boolean {
  if (typeof navigator === 'undefined') return false
  const uaMobile = /android|iphone|ipod|ipad|mobile/i.test(navigator.userAgent) || isIosDevice()
  const coarsePointer =
    typeof window !== 'undefined' && window.matchMedia?.('(pointer: coarse)').matches === true
  const touch = navigator.maxTouchPoints > 0
  return uaMobile || (coarsePointer && touch)
}

export function usePwaInstall() {
  const standalone = isStandalone()
  const ios = isIosDevice()
  const mobile = isMobile()

  /** A native installation prompt is available (Chromium browsers that support it). */
  const canPrompt = computed(() => deferredPrompt.value !== null)

  /** iOS has no prompt API — show iOS-specific Add-to-Home-Screen instructions. */
  const needsIosInstructions = computed(() => ios && !standalone)

  /**
   * Whether the footer install link should render. Shown on any mobile device
   * that isn't already running the installed app — NOT gated on `canPrompt`,
   * because several Chromium-based mobile browsers (Vivaldi, Firefox, Samsung
   * Internet, …) never fire `beforeinstallprompt` yet can still install via their
   * menu. When no native prompt is available the caller shows manual steps.
   */
  const showInstall = computed(() => mobile && !standalone && !installed.value)

  /**
   * Triggers the native install prompt. Resolves to the user's choice, or
   * `'unavailable'` when there's no deferred prompt (e.g. iOS — the caller then
   * shows manual instructions).
   */
  async function promptInstall(): Promise<'accepted' | 'dismissed' | 'unavailable'> {
    const evt = deferredPrompt.value
    if (!evt) return 'unavailable'
    await evt.prompt()
    const choice = await evt.userChoice
    // A deferred prompt can only be used once.
    deferredPrompt.value = null
    return choice.outcome
  }

  return { showInstall, canPrompt, needsIosInstructions, promptInstall }
}


