<script setup lang="ts">
/**
 * Admin Cards tab — generate cards, search, view as chips (with player-name
 * indicators), click a chip to preview, delete individual or all cards.
 * Mirrors the original `adminTab==='bingo-cards'` block.
 */
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useCardsStore } from '@/stores/cards'

const cards = useCardsStore()
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel">
      <h3 class="mb-12"><i class="fa-duotone fa-id-card"></i> Manage Cards</h3>
      <div class="cards-toolbar">
        <span class="text-dim">Generate</span>
        <input
          v-model.number="cards.generateCount"
          type="number"
          aria-label="Number of cards"
          min="1"
          max="500"
        />
        <span class="text-dim">cards</span>
        <button class="btn-primary" :disabled="cards.generating" @click="cards.generateCards()">
          <LoadingSpinner v-if="cards.generating" label="Generating…" />
          <template v-else>Generate</template>
        </button>
        <button
          v-if="cards.cards.length"
          class="btn-danger btn-sm"
          @click="cards.deleteAllCards()"
        >
          Delete All
        </button>
        <span class="text-dim" style="margin-left: auto">
          {{ cards.filteredCards.length }}/{{ cards.cards.length }} cards
        </span>
      </div>
      <div class="cards-toolbar" style="margin-top: 8px">
        <input
          v-model="cards.cardSearchQuery"
          placeholder="Search by ID or player name…"
          aria-label="Search cards"
          style="flex: 1; min-width: 180px; max-width: 320px"
        />
      </div>
      <LoadingSpinner
        v-if="cards.cardsLoading && cards.cards.length === 0"
        block
        label="Loading cards…"
      />
      <div v-else class="cards-grid">
        <div
          v-for="c in cards.filteredCards"
          :key="c.id"
          class="card-chip"
          :class="{ 'has-player': c.player_name }"
          @click="cards.openCardPreview(c.id)"
        >
          <span v-if="c.player_name" class="card-player-icon" :title="c.player_name">
            <i class="fa-duotone fa-user"></i>
          </span>
          <span>{{ c.id }}</span>
          <span class="del" title="Delete card" @click.stop="cards.deleteCard(c.id)">×</span>
        </div>
      </div>

      <p
        v-if="!cards.cardsLoading && cards.cards.length === 0"
        class="msg-block"
        style="padding: 24px"
      >
        No cards generated yet.
      </p>
    </div>
  </div>
</template>
