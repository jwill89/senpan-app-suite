<script setup lang="ts">
/**
 * Stamp shape picker — lets the player choose a stamp emoji (or upload a custom
 * image). Mirrors the original "Stamp shape picker" block.
 */
import { usePlayerStore } from '@/stores/player'

const player = usePlayerStore()
</script>

<template>
  <div class="stamp-picker">
    <span class="label">Stamp:</span>
    <div
      v-for="s in player.stampShapes"
      :key="s.id"
      :class="['stamp-option', player.stampShape === s.id ? 'active' : '']"
      :title="s.name"
      @click="player.setStampShape(s.id)"
    >
      {{ s.emoji || '⊘' }}
    </div>
    <div
      v-if="player.customStampImage"
      :class="['stamp-option', player.stampShape === 'custom' ? 'active' : '']"
      title="Custom Image"
      @click="player.setStampShape('custom')"
    >
      <img :src="player.customStampImage" class="stamp-custom-preview" alt="custom" />
    </div>
    <label class="stamp-option stamp-upload-btn" title="Upload custom stamp image">
      <i class="fa-solid fa-folder-open"></i>
      <input type="file" accept="image/*" hidden @change="player.uploadCustomStamp($event)" />
    </label>
  </div>
</template>
