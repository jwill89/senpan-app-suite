<script setup lang="ts">
/**
 * Admin Cards tab — generate cards (batch or a single named card), search, and
 * manage them in a paginated, sortable table (board id, player name, created
 * date, and view/edit + delete actions). Clicking view/edit opens the board
 * preview modal (which also allows inline player-name/details edits).
 */
import { useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import PaginationBar from '@/components/common/ui/PaginationBar.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import { useCardsStore } from '@/stores/cards'
import { useUiStore } from '@/stores/ui'
import { formatServerTimestamp } from '@/lib/datetime'

const cards = useCardsStore()
const ui = useUiStore()
const router = useRouter()

const columns: DataColumn[] = [
  { key: 'id', label: 'Board ID', sortable: true },
  { key: 'player_name', label: 'Player', sortable: true },
  { key: 'created_at', label: 'Created', sortable: true },
  { key: 'actions', label: '', align: 'right' },
]

/** Copies a card's full playable URL (origin + /play/:cardId) to the clipboard. */
function copyCardUrl(id: string): void {
  const href = router.resolve({ name: 'player', params: { cardId: id } }).href
  ui.copyToClipboard(window.location.origin + href)
}
</script>

<template>
  <div class="tab-body">
    <AdminPanel title="Manage Cards" :icon="['fad', 'id-card']">
      <!-- All generation controls on one line (wraps on narrow widths): a batch
           count generator, a single named-card generator, and Delete All. -->
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
        <button class="btn-action" :disabled="cards.generating" @click="cards.generateCards()">
          <LoadingSpinner v-if="cards.generating" label="Generating…" />
          <template v-else>Generate</template>
        </button>

        <span class="text-dim">or one for</span>
        <input
          v-model="cards.singleCardName"
          type="text"
          class="cards-name-input"
          aria-label="Player name for the new card"
          placeholder="Player name"
          maxlength="100"
          @keyup.enter="cards.generateSingleCard()"
        />
        <button
          class="btn-action"
          :disabled="cards.generatingSingle || !cards.singleCardName.trim()"
          @click="cards.generateSingleCard()"
        >
          <LoadingSpinner v-if="cards.generatingSingle" label="Creating…" />
          <template v-else>Generate card</template>
        </button>

        <button
          v-if="cards.cards.length"
          class="btn-danger btn-sm push-right"
          @click="cards.deleteAllCards()"
        >
          Delete All
        </button>
      </div>

      <div class="flex-toolbar mb-12">
        <label class="text-dim text-xs">Per page:</label>
        <select v-model.number="cards.cardsPerPage" aria-label="Cards per page" style="width: 70px">
          <option :value="10">10</option>
          <option :value="25">25</option>
          <option :value="50">50</option>
          <option :value="100">100</option>
        </select>
        <span class="text-dim text-xs push-right">
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
      <template v-else>
        <DataTable
          :columns="columns"
          :rows="cards.pagedCards"
          row-key="id"
          :sort-key="cards.cardsSortKey"
          :sort-dir="cards.cardsSortDir"
          @sort="cards.cardsSetSort"
        >
          <template #cell-id="{ row }">
            <span class="code-gold">{{ row.id }}</span>
          </template>
          <template #cell-player_name="{ row }">{{ row.player_name || '—' }}</template>
          <template #cell-created_at="{ row }">
            {{ row.created_at ? formatServerTimestamp(row.created_at) : '—' }}
          </template>
          <template #cell-actions="{ row }">
            <div class="row-actions">
              <button
                class="btn-view btn-sm"
                aria-label="View or edit board"
                title="View / edit board"
                @click="cards.openCardPreview(row.id)"
              >
                <font-awesome-icon :icon="['fas', 'eye']" />
              </button>
              <button
                class="btn-view btn-sm"
                aria-label="Copy board ID"
                title="Copy ID"
                @click="ui.copyToClipboard(row.id)"
              >
                <font-awesome-icon :icon="['fas', 'copy']" />
              </button>
              <button
                class="btn-view btn-sm"
                aria-label="Copy playable card link"
                title="Copy link"
                @click="copyCardUrl(row.id)"
              >
                <font-awesome-icon :icon="['fas', 'link']" />
              </button>
              <button
                class="btn-danger btn-sm"
                aria-label="Delete board"
                title="Delete board"
                @click="cards.deleteCard(row.id)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" />
              </button>
            </div>
          </template>
          <template #empty>
            <EmptyState
              v-if="!cards.cardsLoading"
              :text="cards.cards.length ? 'No cards match your search.' : 'No cards generated yet.'"
            />
          </template>
        </DataTable>
        <PaginationBar
          class="mt-12"
          :page="cards.cardsPage"
          :total-pages="cards.cardsTotalPages"
          @go="cards.cardsGoPage"
        />
      </template>
    </AdminPanel>
  </div>
</template>

<style scoped>
/* Keep the single-line generation toolbar compact — a fixed-ish name field so
   the row doesn't stretch (it still wraps on narrow widths via .flex-toolbar). */
.cards-name-input {
  width: 160px;
}
</style>
