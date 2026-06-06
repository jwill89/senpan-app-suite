<script setup lang="ts">
/**
 * Admin Winners Log tab — paginated, sortable table of past game winners with
 * per-page controls. Mirrors the original `adminTab==='bingo-winners-log'` block.
 */
import { useGameStore } from '@/stores/game'

const game = useGameStore()

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
    <div class="admin-panel" style="padding: 24px">
      <h3 class="mb-12"><i class="fa-solid fa-trophy"></i> Winners Log</h3>
      <div class="flex-toolbar mb-12" style="gap: 12px; align-items: center">
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
        <span class="text-dim text-xs" style="margin-left: auto">
          {{ game.winnersLogTotal }} total entries
        </span>
      </div>
      <div v-if="game.winnersLog.length" style="overflow-x: auto">
        <table class="winners-log-table">
          <thead>
            <tr>
              <th style="cursor: pointer" @click="game.winnersLogSetSort('logged_at')">
                Date
                {{
                  game.winnersLogSort === 'logged_at'
                    ? game.winnersLogDir === 'asc'
                      ? '▲'
                      : '▼'
                    : ''
                }}
              </th>
              <th style="cursor: pointer" @click="game.winnersLogSetSort('card_id')">
                Card ID
                {{
                  game.winnersLogSort === 'card_id'
                    ? game.winnersLogDir === 'asc'
                      ? '▲'
                      : '▼'
                    : ''
                }}
              </th>
              <th style="cursor: pointer" @click="game.winnersLogSetSort('player_name')">
                Player
                {{
                  game.winnersLogSort === 'player_name'
                    ? game.winnersLogDir === 'asc'
                      ? '▲'
                      : '▼'
                    : ''
                }}
              </th>
              <th>Details</th>
              <th>Patterns</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="entry in game.winnersLog" :key="entry.id">
              <td>{{ new Date(entry.logged_at).toLocaleString() }}</td>
              <td class="code-gold">{{ entry.card_id }}</td>
              <td>{{ entry.player_name || '—' }}</td>
              <td>{{ entry.game_details || '—' }}</td>
              <td>{{ patternsLabel(entry.winning_patterns) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="msg-block" style="padding: 24px">No winners logged yet.</p>
      <div v-if="game.winnersLogTotalPages() > 1" class="pagination-bar mt-12">
        <button
          class="btn-ghost btn-sm"
          :disabled="game.winnersLogPage <= 1"
          @click="game.winnersLogGoPage(game.winnersLogPage - 1)"
        >
          ‹ Prev
        </button>
        <span class="text-dim text-xs">
          Page {{ game.winnersLogPage }} / {{ game.winnersLogTotalPages() }}
        </span>
        <button
          class="btn-ghost btn-sm"
          :disabled="game.winnersLogPage >= game.winnersLogTotalPages()"
          @click="game.winnersLogGoPage(game.winnersLogPage + 1)"
        >
          Next ›
        </button>
      </div>
    </div>
  </div>
</template>
