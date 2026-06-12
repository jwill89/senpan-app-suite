<script setup lang="ts">
/**
 * Stamp shape picker — lets the player choose a stamp emoji (or upload a custom
 * image). Mirrors the original "Stamp shape picker" block.
 */
import { usePlayerStore } from '@/stores/player'

const player = usePlayerStore()
</script>

<template>
  <div class="stamp-picker" role="group" aria-label="Stamp shape">
    <span class="label">Stamp:</span>
    <button
      v-for="s in player.stampShapes"
      :key="s.id"
      type="button"
      :class="['stamp-option', player.stampShape === s.id ? 'active' : '']"
      :title="s.name"
      :aria-label="`Stamp shape: ${s.name}`"
      :aria-pressed="player.stampShape === s.id"
      @click="player.setStampShape(s.id)"
    >
      {{ s.emoji || '⊘' }}
    </button>
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
    <label class="stamp-option stamp-upload-btn" title="Upload custom stamp image">
      <i class="fa-duotone fa-folder-open" aria-hidden="true"></i>
      <span class="sr-only">Upload custom stamp image</span>
      <input type="file" accept="image/*" hidden @change="player.uploadCustomStamp($event)" />
    </label>
  </div>
</template>
