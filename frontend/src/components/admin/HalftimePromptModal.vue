<script setup lang="ts">
/**
 * Halftime prompt modal (admin) — appears at the game's half-way point (the
 * 35-of-75 mark scaled to this game's callable pool), asking whether to alert
 * players about a half-time minigame.
 */
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import { useGameStore } from '@/stores/game'

const game = useGameStore()
</script>

<template>
  <ModalOverlay v-if="game.showHalftimePrompt" centered @close="game.dismissHalftime()">
    <h3 class="mb-16"><font-awesome-icon :icon="['fad', 'circle-pause']" /> Half-Time!</h3>
    <p class="text-dim mb-8">
      You've drawn {{ game.halftimeThreshold }} numbers! Would you like to alert users about a
      half-time minigame?
    </p>
    <p v-if="game.halftimeAutoPaused" class="text-dim text-sm mb-20">
      <font-awesome-icon :icon="['fas', 'circle-pause']" /> Auto-draw has been paused. Choose
      <strong>No</strong> to resume it, or <strong>Yes</strong> to run a mini-game (auto stays off
      until you switch it back on).
    </p>
    <div v-else class="mb-20"></div>
    <div class="flex-center" style="gap: 12px">
      <button class="btn-action" @click="game.confirmHalftime()">Yes</button>
      <button class="btn-neutral" @click="game.dismissHalftime()">No</button>
    </div>
  </ModalOverlay>
</template>
