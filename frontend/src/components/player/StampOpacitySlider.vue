<script setup lang="ts">
/**
 * Stamp opacity slider. Mirrors the original "Stamp opacity" block.
 */
import { usePlayerStore } from '@/stores/player'

const player = usePlayerStore()

function onInput(e: Event): void {
  player.setStampOpacity((e.target as HTMLInputElement).value)
}
</script>

<template>
  <div class="opacity-slider">
    <span class="label">Opacity:</span>
    <input
      type="range"
      min="0.1"
      max="1"
      step="0.05"
      aria-label="Stamp opacity"
      :value="player.stampOpacity"
      @input="onInput"
    />
    <span class="opacity-val">{{ Math.round(player.stampOpacity * 100) }}%</span>
  </div>
</template>
