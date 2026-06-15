<script setup lang="ts">
/**
 * Admin Winners Log tab — paginated, sortable table of past game winners with
 * per-page controls. Mirrors the original `adminTab==='bingo-winners-log'` block.
 */
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import PaginationBar from '@/components/common/ui/PaginationBar.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import { useGameStore } from '@/stores/game'
import { formatServerTimestamp } from '@/lib/datetime'

const game = useGameStore()

const columns: DataColumn[] = [
  { key: 'logged_at', label: 'Date', sortable: true },
  { key: 'card_id', label: 'Card ID', sortable: true },
  { key: 'player_name', label: 'Player', sortable: true },
  { key: 'game_details', label: 'Details' },
  { key: 'winning_patterns', label: 'Patterns' },
  { key: 'actions', label: '', align: 'right' },
]

/** Parses the JSON winning_patterns array into a comma-joined string. */
function patternsLabel(json: string): string {
  try {
    const arr = JSON.parse(json || '[]') as string[]
    return arr.join(', ') || '—'
  } catch {
    return '—'
  }
}

function onPerPageChange(): void {
  game.winnersLogPage = 1
  game.loadWinnersLog()
}
</script>

<template>
  <div class="tab-body">
    <AdminPanel title="Winners Log" :icon="['fad', 'trophy']">
      <div class="flex-toolbar mb-12">
        <label class="text-dim text-xs">Per page:</label>
        <select
          v-model.number="game.winnersLogPerPage"
          aria-label="Entries per page"
          style="width: 70px"
          @change="onPerPageChange"
        >
          <option :value="10">10</option>
          <option :value="25">25</option>
          <option :value="50">50</option>
          <option :value="100">100</option>
        </select>
        <span class="text-dim text-xs push-right">
          {{ game.winnersLogTotal }} total entries
        </span>
        <button
          class="btn-danger btn-sm"
          :disabled="!game.winnersLogTotal"
          title="Delete all winners-log entries"
          @click="game.deleteAllWinnersLog()"
        >
          <font-awesome-icon :icon="['fas', 'trash']" /> Delete All
        </button>
      </div>
      <LoadingSpinner
        v-if="game.winnersLogLoading && game.winnersLog.length === 0"
        block
        label="Loading winners…"
      />
      <template v-else>
        <DataTable
          :columns="columns"
          :rows="game.winnersLog"
          row-key="id"
          :sort-key="game.winnersLogSort"
          :sort-dir="game.winnersLogDir"
          @sort="game.winnersLogSetSort"
        >
          <template #cell-logged_at="{ row }">{{ formatServerTimestamp(row.logged_at) }}</template>
          <template #cell-card_id="{ row }">
            <span class="code-gold">{{ row.card_id }}</span>
          </template>
          <template #cell-player_name="{ row }">{{ row.player_name || '—' }}</template>
          <template #cell-game_details="{ row }">{{ row.game_details || '—' }}</template>
          <template #cell-winning_patterns="{ row }">
            {{ patternsLabel(row.winning_patterns) }}
          </template>
          <template #cell-actions="{ row }">
            <div class="row-actions">
              <button
                class="btn-danger btn-sm"
                aria-label="Delete entry"
                title="Delete entry"
                @click="game.deleteWinnerLogEntry(row.id)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" />
              </button>
            </div>
          </template>
          <template #empty>
            <EmptyState v-if="!game.winnersLogLoading" text="No winners logged yet." />
          </template>
        </DataTable>
        <PaginationBar
          class="mt-12"
          :page="game.winnersLogPage"
          :total-pages="game.winnersLogTotalPages()"
          @go="game.winnersLogGoPage"
        />
      </template>
    </AdminPanel>
  </div>
</template>
