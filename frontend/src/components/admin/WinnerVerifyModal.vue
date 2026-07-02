<script setup lang="ts">
/**
 * Winner verification modal (admin) — shows a winning card's board with the
 * cells that complete the win pattern highlighted. Mirrors the original
 * "Winner verification modal" block. Driven by game.winnerPreview.
 */
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import BingoBoard from '@/components/common/BingoBoard.vue'
import { useGameStore } from '@/stores/game'

const game = useGameStore()
</script>

<template>
  <ModalOverlay v-if="game.winnerPreview" @close="game.winnerPreview = null">
    <div class="flex-between mb-12">
      <h3>
        <font-awesome-icon :icon="['fad', 'trophy']" /> Card
        <span class="code-gold">{{ game.winnerPreview.card.id }}</span>
      </h3>
      <button class="btn-neutral btn-sm" @click="game.winnerPreview = null">Close</button>
    </div>
    <p class="text-dim text-sm mb-12 text-center">Highlighted cells complete the winning pattern</p>
    <div class="flex-center">
      <BingoBoard
        :board="game.winnerPreview.card.board_data"
        mode="preview"
        preview
        :is-cell-match="game.isWinnerCellMatch"
      />
    </div>
    <div
      v-if="game.winnerPreview.card.player_name || game.winnerPreview.card.details"
      class="mt-12 text-center"
    >
      <p v-if="game.winnerPreview.card.player_name" style="font-weight: 600">
        {{ game.winnerPreview.card.player_name }}
      </p>
      <p v-if="game.winnerPreview.card.details" class="text-dim text-sm">
        {{ game.winnerPreview.card.details }}
      </p>
    </div>
  </ModalOverlay>
</template>
