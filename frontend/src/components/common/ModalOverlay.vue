<script setup lang="ts">
/**
 * Accessible modal overlay + centered dialog box.
 *
 * Behaviour:
 *  - Clicking the backdrop or pressing Escape emits `close`.
 *  - `role="dialog"` + `aria-modal` mark it as a modal for assistive tech.
 *  - Focus is moved into the dialog on open, trapped within it while open
 *    (Tab/Shift+Tab cycle), and restored to the previously focused element on
 *    close — so keyboard and screen-reader users aren't stranded.
 *
 * The dialog content is provided via the default slot.
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'

defineProps<{
  /** Adds the `modal-centered` class to the box (used by simple prompts). */
  centered?: boolean
  /** Inline style applied to the box (some modals set max-width/height). */
  boxStyle?: string | Record<string, string>
  /** Accessible label for the dialog (falls back to a generic label). */
  ariaLabel?: string
}>()

const emit = defineEmits<{ close: [] }>()

// ── Shared modal stack ───────────────────────────────────────────────────────
// Stacked modals (e.g. a ui.confirm() dialog opened from inside an edit modal)
// must not share Escape and focus. We keep a module-level stack of the open
// overlays: only the TOP-MOST one reacts to Escape and owns the focus trap, and
// every modal beneath it — plus the page behind — is made `inert` + aria-hidden
// so keyboard/pointer/AT focus can't reach it. Closing the top one restores the
// one beneath (or the page) automatically.
interface ModalEntry {
  overlay: HTMLElement | null
}
const modalStack: ModalEntry[] = []

/** Re-apply inert/aria-hidden across the stack: only the top overlay is live. */
function restack(): void {
  modalStack.forEach((m, i) => {
    if (!m.overlay) return
    const isTop = i === modalStack.length - 1
    m.overlay.inert = !isTop
    if (isTop) m.overlay.removeAttribute('aria-hidden')
    else m.overlay.setAttribute('aria-hidden', 'true')
  })
}

/**
 * Make everything behind `overlay` inert + aria-hidden (siblings along the path
 * up to <body>), skipping other modal overlays and anything already hidden.
 * Returns a restore fn. Only the bottom-most modal calls this — modals stacked
 * above it are handled by {@link restack}.
 */
function inertBackground(overlay: HTMLElement): () => void {
  const changed: HTMLElement[] = []
  let node: HTMLElement = overlay
  while (node.parentElement) {
    const parent = node.parentElement
    for (const sib of Array.from(parent.children)) {
      if (sib === node) continue
      const el = sib as HTMLElement
      if (el.classList.contains('modal-overlay') || el.querySelector('.modal-overlay')) continue
      if (el.inert || el.hasAttribute('aria-hidden')) continue
      el.inert = true
      el.setAttribute('aria-hidden', 'true')
      changed.push(el)
    }
    if (parent === document.body) break
    node = parent
  }
  return () => {
    for (const el of changed) {
      el.inert = false
      el.removeAttribute('aria-hidden')
    }
  }
}

const box = ref<HTMLElement | null>(null)
const overlay = ref<HTMLElement | null>(null)
const entry: ModalEntry = { overlay: null }
let previouslyFocused: HTMLElement | null = null
let restoreBackground: (() => void) | null = null

/** Whether this modal is the top-most open one (owns Escape + focus trap). */
function isTop(): boolean {
  return modalStack[modalStack.length - 1] === entry
}

/** Returns the focusable elements within the dialog, in DOM order. */
function focusable(): HTMLElement[] {
  if (!box.value) return []
  return Array.from(
    box.value.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((el) => el.offsetParent !== null || el === document.activeElement)
}

function onKeydown(e: KeyboardEvent): void {
  // Only the top-most modal reacts — a lower modal must ignore keys meant for
  // the dialog stacked above it (e.g. Escape on a confirm dialog opened from an
  // edit modal must not also close the edit modal).
  if (!isTop()) return
  if (e.key === 'Escape') {
    e.stopImmediatePropagation()
    emit('close')
    return
  }
  if (e.key !== 'Tab') return
  // Trap focus within the dialog.
  const items = focusable()
  if (items.length === 0) {
    e.preventDefault()
    box.value?.focus()
    return
  }
  const first = items[0]
  const last = items[items.length - 1]
  const active = document.activeElement as HTMLElement | null
  if (e.shiftKey && (active === first || !box.value?.contains(active))) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && active === last) {
    e.preventDefault()
    first.focus()
  }
}

onMounted(() => {
  previouslyFocused = document.activeElement as HTMLElement | null
  entry.overlay = overlay.value
  // Bottom-most modal makes the page behind it inert; ones stacked above rely
  // on restack() to inert the modal(s) beneath them.
  if (modalStack.length === 0 && overlay.value) {
    restoreBackground = inertBackground(overlay.value)
  }
  modalStack.push(entry)
  restack()
  document.addEventListener('keydown', onKeydown, true)
  // Focus the first focusable control, else the dialog itself.
  const items = focusable()
  ;(items[0] ?? box.value).focus()
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown, true)
  const i = modalStack.indexOf(entry)
  if (i !== -1) modalStack.splice(i, 1)
  restoreBackground?.()
  restoreBackground = null
  restack()
  // Restore focus to whatever was focused before the modal opened.
  previouslyFocused?.focus()
})
</script>

<template>
  <div ref="overlay" class="modal-overlay" @click.self="emit('close')">
    <div
      ref="box"
      class="modal-box"
      :class="{ 'modal-centered': centered }"
      :style="boxStyle"
      role="dialog"
      aria-modal="true"
      :aria-label="ariaLabel || 'Dialog'"
      tabindex="-1"
    >
      <slot />
    </div>
  </div>
</template>
