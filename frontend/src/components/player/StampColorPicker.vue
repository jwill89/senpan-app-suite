<script setup lang="ts">
/**
 * Stamp color picker — lets the player choose the stamp tint.
 * Mirrors the original "Stamp color picker" block.
 */
import { usePlayerStore } from '@/stores/player'

const player = usePlayerStore()
</script>

<template>
  <div class="stamp-picker">
    <span class="label">Color:</span>
    <div
      v-for="c in player.stampColors"
      :key="c.id"
      :class="['color-swatch', player.stampColor === c.id ? 'active' : '']"
      :style="{ background: c.value }"
      :title="c.name"
      @click="player.setStampColor(c.id)"
    ></div>
  </div>
</template>
