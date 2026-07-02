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
import { ref } from 'vue'
import EmojiPickerModal from '@/components/common/EmojiPickerModal.vue'
import { usePlayerStore } from '@/stores/player'

const player = usePlayerStore()

const pickerOpen = ref(false)

function onSelectEmoji(emoji: string): void {
  player.setStampEmoji(emoji)
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
      <font-awesome-icon v-else :icon="['fad', 'face-smile']" />
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
      <font-awesome-icon :icon="['fad', 'folder-open']" />
      <span class="sr-only">Upload custom stamp image</span>
      <input type="file" accept="image/*" hidden @change="player.uploadCustomStamp($event)" />
    </label>

    <!-- Emoji picker modal -->
    <EmojiPickerModal v-if="pickerOpen" @select="onSelectEmoji" @close="pickerOpen = false" />
  </div>
</template>
