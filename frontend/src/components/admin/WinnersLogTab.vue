<script setup lang="ts">
/**
 * Admin Winners Log tab - paginated, sortable table of past game winners with
 * per-page controls. Mirrors the original `adminTab==='bingo-winners-log'` block.
 */
import { ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import DataTableToolbar from '@/components/common/ui/DataTableToolbar.vue'
import DataTable, {
  type DataColumn,
  type DataTableView,
} from '@/components/common/ui/DataTable.vue'
import PaginationBar from '@/components/common/ui/PaginationBar.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import { useGameStore } from '@/stores/game'
import { formatServerTimestamp } from '@/lib/datetime'

const game = useGameStore()
const view = ref<DataTableView>({ total: 0, totalPages: 1, facets: {} })
const selectedEntries = ref<(string | number)[]>([])
const tableRef = ref<{ exportCsv: (name?: string) => void } | null>(null)

/** Bulk-delete the checked entries, then drop the now-stale selection. */
async function deleteSelectedEntries(): Promise<void> {
  const done = await game.deleteWinnerLogEntries(selectedEntries.value.map(Number))
  if (done) selectedEntries.value = []
}

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
    return arr.join(', ') || '-'
  } catch {
    return '-'
  }
}
</script>

<template>
  <div class="tab-body">
    <AdminPanel title="Winners Log" :icon="['fad', 'trophy']">
      <DataTableToolbar
        v-model:per-page="game.winnersLogPerPage"
        :count="view.total"
        :total="game.winnersLogTotal"
        noun="entry"
        plural="entries"
        :per-page-options="[10, 25, 50, 100]"
      >
        <template #actions>
          <button
            class="btn-view btn-sm"
            :disabled="!view.total"
            title="Download the entries currently shown, as CSV"
            @click="tableRef?.exportCsv('winners-log')"
          >
            <font-awesome-icon :icon="['fas', 'file-arrow-down']" /> Export CSV
          </button>
          <button
            v-if="selectedEntries.length"
            class="btn-danger btn-sm"
            @click="deleteSelectedEntries"
          >
            <font-awesome-icon :icon="['fas', 'trash']" /> Delete
            {{ selectedEntries.length }} selected
          </button>
          <button
            class="btn-danger btn-sm"
            :disabled="!game.winnersLogTotal"
            title="Delete all winners-log entries"
            @click="game.deleteAllWinnersLog()"
          >
            <font-awesome-icon :icon="['fas', 'trash']" /> Delete All
          </button>
        </template>
      </DataTableToolbar>
      <LoadingSpinner
        v-if="game.winnersLogLoading && game.winnersLog.length === 0"
        block
        label="Loading winners..."
      />
      <template v-else>
        <DataTable
          ref="tableRef"
          v-model:page="game.winnersLogPage"
          v-model:selected="selectedEntries"
          :columns="columns"
          :rows="game.winnersLog"
          row-key="id"
          selectable
          resizable
          :page-size="game.winnersLogPerPage"
          :default-sort="{ key: 'logged_at', dir: 'desc' }"
          @update:view="view = $event"
        >
          <template #cell-logged_at="{ row }">{{ formatServerTimestamp(row.logged_at) }}</template>
          <template #cell-card_id="{ row }">
            <span class="code-highlight">{{ row.card_id }}</span>
          </template>
          <template #cell-player_name="{ row }">{{ row.player_name || '-' }}</template>
          <template #cell-game_details="{ row }">{{ row.game_details || '-' }}</template>
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
          :total-pages="view.totalPages"
          @go="(p: number) => (game.winnersLogPage = p)"
        />
      </template>
    </AdminPanel>
  </div>
</template>
