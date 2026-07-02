<script setup lang="ts">
/**
 * Stamp opacity slider. Mirrors the original "Stamp opacity" block.
 *
 * Uses a fully custom-styled range input (appearance:none) rather than the
 * native `accent-color` slider, whose thumb is inset from the track ends — that
 * inset made 0% / 100% look reachable "past" the visible track. Here the thumb
 * travels flush to both edges, and `--fill` positions the gold fill to end at
 * the thumb centre (accounting for the 16px thumb width) so the two stay aligned.
 */
import { computed } from 'vue'
import { usePlayerStore } from '@/stores/player'

const player = usePlayerStore()

/** Gold-fill length ending at the thumb centre (thumb is 16px on the track). */
const fillPct = computed(() => `calc((100% - 16px) * ${player.stampOpacity} + 8px)`)

function onInput(e: Event): void {
  player.setStampOpacity((e.target as HTMLInputElement).value)
}
</script>

<template>
  <div class="opacity-slider">
    <span class="label">Opacity:</span>
    <input
      type="range"
      min="0"
      max="1"
      step="0.05"
      aria-label="Stamp opacity"
      :value="player.stampOpacity"
      :style="{ '--fill': fillPct }"
      @input="onInput"
    />
    <span class="opacity-val">{{ Math.round(player.stampOpacity * 100) }}%</span>
  </div>
</template>
