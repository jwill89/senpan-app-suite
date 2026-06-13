<script setup lang="ts">
/**
 * Stamp shape picker — lets the player choose the stamp icon:
 *  - "None": no emoji, just the colored circle.
 *  - Any emoji: opens an emoji picker modal (replaces the old fixed emoji row,
 *    which wrapped once a custom stamp was added).
 *  - A custom uploaded image.
 *
 * The emoji picker (and its CSS) is lazy-loaded so it only lands when a player
 * actually opens it — the initial player payload stays small.
 */
import { defineAsyncComponent, ref } from 'vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import { usePlayerStore } from '@/stores/player'

/** Lazy-load the emoji picker + its stylesheet together into one on-demand chunk. */
const EmojiPicker = defineAsyncComponent(async () => {
  await import('vue3-emoji-picker/css')
  return (await import('vue3-emoji-picker')).default
})

/** The emoji-picker `select` payload — `i` is the chosen emoji character. */
interface EmojiSelect {
  i: string
}

const player = usePlayerStore()

const pickerOpen = ref(false)

function onSelectEmoji(emoji: EmojiSelect): void {
  player.setStampEmoji(emoji.i)
  pickerOpen.value = false
}
</script>

<template>
  <div class="stamp-picker" role="group" aria-label="Stamp shape">
    <span class="label">Stamp:</span>

    <!-- None: color circle only, no emoji -->
    <button
      type="button"
      :class="['stamp-option', player.stampShape === 'blank' ? 'active' : '']"
      title="No emoji (color only)"
      aria-label="No stamp emoji"
      :aria-pressed="player.stampShape === 'blank'"
      @click="player.setStampShape('blank')"
    >
      ⊘
    </button>

    <!-- Emoji selector: shows the chosen emoji, opens the picker -->
    <button
      type="button"
      :class="['stamp-option', player.stampShape === 'emoji' ? 'active' : '']"
      title="Pick an emoji"
      aria-label="Pick a stamp emoji"
      :aria-pressed="player.stampShape === 'emoji'"
      @click="pickerOpen = true"
    >
      <span v-if="player.stampShape === 'emoji' && player.stampEmoji">{{ player.stampEmoji }}</span>
      <i v-else class="fa-duotone fa-face-smile" aria-hidden="true"></i>
    </button>

    <!-- Custom uploaded image -->
    <button
      v-if="player.customStampImage"
      type="button"
      :class="['stamp-option', player.stampShape === 'custom' ? 'active' : '']"
      title="Custom Image"
      aria-label="Stamp shape: custom image"
      :aria-pressed="player.stampShape === 'custom'"
      @click="player.setStampShape('custom')"
    >
      <img :src="player.customStampImage" class="stamp-custom-preview" alt="custom" />
    </button>

    <!-- Upload a custom image -->
    <label class="stamp-option stamp-upload-btn" title="Upload custom stamp image">
      <i class="fa-duotone fa-folder-open" aria-hidden="true"></i>
      <span class="sr-only">Upload custom stamp image</span>
      <input type="file" accept="image/*" hidden @change="player.uploadCustomStamp($event)" />
    </label>

    <!-- Emoji picker modal -->
    <ModalOverlay
      v-if="pickerOpen"
      centered
      aria-label="Emoji picker"
      :box-style="{ maxWidth: 'fit-content' }"
      @close="pickerOpen = false"
    >
      <h3 class="mb-12"><i class="fa-duotone fa-face-smile"></i> Pick an Emoji</h3>
      <div class="emoji-picker-wrap">
        <EmojiPicker :native="true" theme="dark" :display-recent="true" @select="onSelectEmoji" />
      </div>
    </ModalOverlay>
  </div>
</template>

<style scoped>
.emoji-picker-wrap {
  display: flex;
  justify-content: center;
}

/* The picker ships its own dark theme, but the app's global `input` rule would
   otherwise override the search field — re-skin it to the app surface so it
   stays legible and on-theme. */
.emoji-picker-wrap :deep(input) {
  background: var(--surface);
  color: var(--text);
  border-color: var(--surface2);
}
</style>

