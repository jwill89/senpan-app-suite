<script setup lang="ts">
/**
 * Halftime prompt modal (admin) — appears after the 35th number is drawn,
 * asking whether to alert players about a half-time minigame. Mirrors the
 * original "Halftime minigame prompt (admin)" block.
 */
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import { useGameStore } from '@/stores/game'

const game = useGameStore()
</script>

<template>
  <ModalOverlay v-if="game.showHalftimePrompt" centered @close="game.dismissHalftime()">
    <h3 class="mb-16"><font-awesome-icon :icon="['fad', 'circle-pause']" /> Half-Time!</h3>
    <p class="text-dim mb-20">
      You've drawn 35 numbers! Would you like to alert users about a half-time minigame?
    </p>
    <div class="flex-center" style="gap: 12px">
      <button class="btn-action" @click="game.confirmHalftime()">Yes</button>
      <button class="btn-neutral" @click="game.dismissHalftime()">No</button>
    </div>
  </ModalOverlay>
</template>
