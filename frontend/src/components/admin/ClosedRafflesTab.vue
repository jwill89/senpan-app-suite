<script setup lang="ts">
/**
 * Admin Closed Raffles tab — the closed-raffle list and a read-only detail view
 * (winner + entries, no paid toggles or actions). Mirrors the original
 * `adminTab==='raffle-closed'` block. The `✓` paid indicator is a real icon
 * element here (the original interpolated the raw tag as text).
 */
import { useRafflesStore } from '@/stores/raffles'
import { assetUrl } from '@/lib/assets'

const raffles = useRafflesStore()
</script>

<template>
  <div class="tab-body">
    <!-- Raffle detail view (closed) -->
    <div v-if="raffles.selectedRaffle" class="admin-panel">
      <div class="flex-between mb-16" style="flex-wrap: wrap; gap: 8px">
        <h3>
          {{ raffles.selectedRaffle.title }}
          <span class="raffle-badge raffle-badge-closed">closed</span>
        </h3>
        <div class="flex-toolbar">
          <button class="btn-ghost btn-sm" @click="raffles.selectedRaffle = null">← Back</button>
          <button class="btn-danger btn-sm" @click="raffles.deleteRaffle(raffles.selectedRaffle.id)">
            Delete
          </button>
        </div>
      </div>
      <div v-if="raffles.selectedRaffle.prize_image" class="mb-16">
        <img :src="assetUrl(raffles.selectedRaffle.prize_image)" class="raffle-prize-img-sm" alt="Prize" />
      </div>
      <div v-if="raffles.raffleWinner" class="raffle-winner-panel">
        <h3 class="raffle-section-heading">
          <i class="fa-solid fa-trophy"></i> Winner: {{ raffles.raffleWinner.character_name }} @
          {{ raffles.raffleWinner.world }}
        </h3>
        <p class="text-dim text-sm">{{ raffles.raffleWinner.num_entries }} entries</p>
      </div>
      <h3 class="mb-8 mt-16">Entries ({{ raffles.raffleEntries.length }})</h3>
      <div v-if="raffles.raffleEntries.length" class="raffle-entries-table">
        <table class="entries-table">
          <thead>
            <tr>
              <th>Character</th>
              <th class="tc">Entries</th>
              <th class="tc">Paid</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="e in raffles.raffleEntries"
              :key="e.id"
              :style="
                raffles.raffleWinner && raffles.raffleWinner.id === e.id
                  ? 'background:rgba(214,189,174,.12)'
                  : ''
              "
            >
              <td>{{ e.character_name }} @ {{ e.world }}</td>
              <td class="tc">{{ e.num_entries }}</td>
              <td class="tc">
                <i v-if="e.paid" class="fa-solid fa-circle-check"></i>
                <template v-else>—</template>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="msg-block">No entries.</p>
    </div>

    <!-- Closed raffle list -->
    <div v-else class="admin-panel">
      <h3 class="mb-16"><i class="fa-solid fa-lock"></i> Closed Raffles</h3>
      <div class="raffle-list">
        <div
          v-for="r in raffles.closedRaffles"
          :key="r.id"
          class="raffle-card"
          @click="raffles.viewRaffle(r)"
        >
          <img v-if="r.prize_image" :src="assetUrl(r.prize_image)" class="raffle-card-image" alt="Prize" />
          <div class="raffle-card-body">
            <h3>{{ r.title }}</h3>
            <p v-if="r.cost_per_entry > 0" class="raffle-cost">
              {{ r.cost_per_entry.toLocaleString() }} gil per entry
            </p>
          </div>
        </div>
      </div>
      <p v-if="raffles.closedRaffles.length === 0" class="no-game-msg">No closed raffles.</p>
    </div>
  </div>
</template>
