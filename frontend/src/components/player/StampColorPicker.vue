<script setup lang="ts">
/**
 * Stamp color picker — lets the player choose the stamp tint.
 * Mirrors the original "Stamp color picker" block.
 */
import { usePlayerStore } from '@/stores/player'

const player = usePlayerStore()
</script>

<template>
  <div class="stamp-picker" role="group" aria-label="Stamp color">
    <span class="label">Color:</span>
    <button
      v-for="c in player.stampColors"
      :key="c.id"
      type="button"
      :class="['color-swatch', player.stampColor === c.id ? 'active' : '']"
      :style="{ background: c.value }"
      :title="c.name"
      :aria-label="`Stamp color: ${c.name}`"
      :aria-pressed="player.stampColor === c.id"
      @click="player.setStampColor(c.id)"
    ></button>
  </div>
</template>
