<script setup lang="ts">
/**
 * Admin Cards tab — generate cards, search, view as chips (with player-name
 * indicators), click a chip to preview, delete individual or all cards.
 * Mirrors the original `adminTab==='bingo-cards'` block.
 */
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import { useCardsStore } from '@/stores/cards'

const cards = useCardsStore()
</script>

<template>
  <div class="tab-body">
    <AdminPanel title="Manage Cards" icon="fa-duotone fa-id-card">
      <div class="flex-toolbar cards-toolbar mb-20">
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
        <span class="text-dim push-right">
          {{ cards.filteredCards.length }}/{{ cards.cards.length }} cards
        </span>
      </div>
      <SearchInput
        v-model="cards.cardSearchQuery"
        class="mb-12"
        placeholder="Search by ID or player name…"
        aria-label="Search cards"
      />
      <LoadingSpinner
        v-if="cards.cardsLoading && cards.cards.length === 0"
        block
        label="Loading cards…"
      />
      <div v-else class="cards-grid">
        <div
          v-for="c in cards.filteredCards"
          :key="c.id"
          class="chip card-chip"
          :class="{ 'has-player': c.player_name }"
          @click="cards.openCardPreview(c.id)"
        >
          <span v-if="c.player_name" class="card-player-icon" :title="c.player_name">
            <i class="fa-duotone fa-user"></i>
          </span>
          <span>{{ c.id }}</span>
          <span class="del-x" title="Delete card" @click.stop="cards.deleteCard(c.id)">×</span>
        </div>
      </div>

      <EmptyState
        v-if="!cards.cardsLoading && cards.cards.length === 0"
        text="No cards generated yet."
      />
    </AdminPanel>
  </div>
</template>
