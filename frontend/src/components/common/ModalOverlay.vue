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

const box = ref<HTMLElement | null>(null)
let previouslyFocused: HTMLElement | null = null

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
  if (e.key === 'Escape') {
    e.stopPropagation()
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
  document.addEventListener('keydown', onKeydown, true)
  // Focus the first focusable control, else the dialog itself.
  const items = focusable()
  ;(items[0] ?? box.value)?.focus()
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown, true)
  // Restore focus to whatever was focused before the modal opened.
  previouslyFocused?.focus?.()
})
</script>

<template>
  <div class="modal-overlay" @click.self="emit('close')">
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
