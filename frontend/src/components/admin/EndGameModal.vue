<script setup lang="ts">
/**
 * End-game winner selection modal (admin) — lets the admin uncheck card IDs
 * that should NOT count as valid winners before ending the game. Mirrors the
 * original "End Game winner selection modal" block.
 */
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import { useGameStore } from '@/stores/game'
import { useCardsStore } from '@/stores/cards'

const game = useGameStore()
const cards = useCardsStore()

function playerNameFor(id: string): string | undefined {
  return cards.cards.find((c) => c.id === id)?.player_name
}
</script>

<template>
  <ModalOverlay
    v-if="game.showEndGameModal"
    box-style="max-width:450px;max-height:90vh;display:flex;flex-direction:column"
    @close="game.showEndGameModal = false"
  >
    <h3 class="mb-12"><i class="fa-duotone fa-flag-checkered"></i> End Game — Select Valid Winners</h3>
    <p class="text-dim text-xs mb-12">
      Uncheck any card IDs that should NOT count as valid winners.
    </p>
    <div style="display: flex; gap: 8px; margin-bottom: 12px">
      <button class="btn-neutral btn-sm" @click="game.endGameSelectedWinners = [...game.winners]">
        Check All
      </button>
      <button class="btn-neutral btn-sm" @click="game.endGameSelectedWinners = []">Uncheck All</button>
    </div>
    <div
      style="
        display: flex;
        flex-direction: column;
        gap: 8px;
        margin-bottom: 16px;
        overflow-y: auto;
        max-height: 50vh;
        padding-right: 4px;
      "
    >
      <label
        v-for="w in game.winners"
        :key="w"
        style="display: flex; align-items: center; gap: 8px; cursor: pointer"
      >
        <input type="checkbox" :value="w" v-model="game.endGameSelectedWinners" />
        <span class="code-gold">{{ w }}</span>
        <small v-if="playerNameFor(w)" class="text-dim">— {{ playerNameFor(w) }}</small>
      </label>
    </div>
    <div style="display: flex; gap: 8px; justify-content: flex-end">
      <button class="btn-neutral" @click="game.showEndGameModal = false">Cancel</button>
      <button class="btn-caution" @click="game.confirmEndGame(game.endGameSelectedWinners)">
        End Game
      </button>
    </div>
  </ModalOverlay>
</template>
