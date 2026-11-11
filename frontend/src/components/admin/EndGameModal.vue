<script setup lang="ts">
/**
 * End-game winner selection modal (admin) - lets the admin uncheck card IDs
 * that should NOT count as valid winners before ending the game.
 */
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import { useGameStore } from '@/stores/game'
import { useCardsStore } from '@/stores/cards'

const game = useGameStore()
const cards = useCardsStore()

function playerNameFor(id: string): string | undefined {
  return cards.cards.find((c) => c.id === id)?.playerName
}
</script>

<template>
  <ModalOverlay
    v-if="game.showEndGameModal"
    box-style="max-width:450px;max-height:90vh;display:flex;flex-direction:column"
    @close="game.showEndGameModal = false"
  >
    <h3 class="mb-12">
      <font-awesome-icon :icon="['fad', 'flag-checkered']" /> End Game - Select Valid Winners
    </h3>
    <p class="text-muted text-xs mb-12">
      Uncheck any card IDs that should NOT count as valid winners.
    </p>
    <div class="flex-toolbar flex-toolbar--tight items-stretch mb-12">
      <button
        type="button"
        class="btn-neutral btn-sm"
        @click="game.endGameSelectedWinners = [...game.winners]"
      >
        Check All
      </button>
      <button type="button" class="btn-neutral btn-sm" @click="game.endGameSelectedWinners = []">
        Uncheck All
      </button>
    </div>
    <div class="check-list">
      <label v-for="w in game.winners" :key="w" class="check-list-item">
        <input v-model="game.endGameSelectedWinners" type="checkbox" :value="w" />
        <span class="code-highlight">{{ w }}</span>
        <small v-if="playerNameFor(w)" class="text-muted">- {{ playerNameFor(w) }}</small>
      </label>
    </div>
    <div class="flex-toolbar flex-toolbar--tight items-stretch flex-end">
      <button type="button" class="btn-neutral" @click="game.showEndGameModal = false">
        Cancel
      </button>
      <button
        type="button"
        class="btn-caution"
        @click="game.confirmEndGame(game.endGameSelectedWinners)"
      >
        End Game
      </button>
    </div>
  </ModalOverlay>
</template>
