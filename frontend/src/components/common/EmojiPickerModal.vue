<script setup lang="ts">
/**
 * Shared emoji-picker modal — the `vue3-emoji-picker` inside a centered
 * ModalOverlay, lazy-loaded (picker + its CSS in one on-demand chunk) and
 * re-skinned to the app theme. Render it with `v-if` when open; it emits the
 * chosen emoji character via `select` and a `close` request.
 *
 * Used by the player stamp shape picker and the announcement Discord-button
 * emoji field, so the lazy import + modal + skin live in one place.
 */
import { defineAsyncComponent } from 'vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'

/** Lazy-load the emoji picker + its stylesheet together into one on-demand chunk. */
const EmojiPicker = defineAsyncComponent(async () => {
  await import('vue3-emoji-picker/css')
  return (await import('vue3-emoji-picker')).default
})

/** The emoji-picker `select` payload — `i` is the chosen emoji character. */
interface EmojiSelect {
  i: string
}

defineProps<{
  /** Show a "No emoji" button that selects an empty string (clears the value). */
  allowClear?: boolean
}>()

const emit = defineEmits<{ select: [emoji: string]; close: [] }>()

function onSelect(e: EmojiSelect): void {
  emit('select', e.i)
}
</script>

<template>
  <ModalOverlay
    centered
    aria-label="Emoji picker"
    :box-style="{ maxWidth: 'fit-content' }"
    @close="emit('close')"
  >
    <div class="emoji-picker-head">
      <h3 class="mb-0"><i class="fa-duotone fa-face-smile"></i> Pick an Emoji</h3>
      <button v-if="allowClear" type="button" class="btn-neutral btn-sm" @click="emit('select', '')">
        No emoji
      </button>
    </div>
    <div class="emoji-picker-wrap">
      <EmojiPicker :native="true" theme="dark" :display-recent="true" @select="onSelect" />
    </div>
  </ModalOverlay>
</template>

<style scoped>
.emoji-picker-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}
.emoji-picker-wrap {
  display: flex;
  justify-content: center;
}

/* The picker ships its own dark theme, but the app's global `input` rule would
   otherwise override the search field — re-skin it to the app surface so it
   stays legible and on-theme. */
.emoji-picker-wrap :deep(input) {
  background: var(--panel-bg);
  color: var(--text);
  border-color: var(--panel-raised-bg);
}
</style>
