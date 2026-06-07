<script setup lang="ts">
/**
 * Admin Open Raffles tab — the open-raffle list, plus the selected-raffle detail
 * view (prize image, winner pick/verify, entries table with paid toggles).
 * Mirrors the original `adminTab==='raffle-open'` block.
 *
 * NOTE: the original used `{{ '<i …>' }}` text interpolation for the Paid/✓
 * indicators, which rendered the raw tag as literal text (a FontAwesome-kit
 * quirk). Here those are real icon elements so they display as intended — the
 * fields/columns are otherwise unchanged.
 */
import { useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useRafflesStore } from '@/stores/raffles'
import { assetUrl } from '@/lib/assets'

const router = useRouter()
const raffles = useRafflesStore()

/** Open the edit form for the selected raffle. */
function editSelected(): void {
  if (!raffles.selectedRaffle) return
  raffles.editRaffleForm(raffles.selectedRaffle)
  router.push({ name: 'admin-raffle-new' })
}
</script>

<template>
  <div class="tab-body">
    <!-- Raffle detail view -->
    <div v-if="raffles.selectedRaffle" class="admin-panel">
      <div class="flex-between mb-16" style="flex-wrap: wrap; gap: 8px">
        <h3>
          {{ raffles.selectedRaffle.title }}
          <span :class="['raffle-badge', 'raffle-badge-' + raffles.selectedRaffle.status]">
            {{ raffles.selectedRaffle.status }}
          </span>
        </h3>
        <div class="flex-toolbar">
          <button class="btn-ghost btn-sm" @click="raffles.selectedRaffle = null">← Back</button>
          <button class="btn-secondary btn-sm" @click="editSelected">Edit</button>
          <button class="btn-danger btn-sm" @click="raffles.deleteRaffle(raffles.selectedRaffle.id)">
            Delete
          </button>
        </div>
      </div>

      <!-- Prize image -->
      <div v-if="raffles.selectedRaffle.prize_image" class="mb-16">
        <img :src="assetUrl(raffles.selectedRaffle.prize_image)" class="raffle-prize-img-sm" alt="Prize" />
      </div>

      <!-- Winner section -->
      <div v-if="raffles.raffleWinner" class="raffle-winner-panel">
        <h3 class="raffle-section-heading">
          <i class="fa-solid fa-trophy"></i> Winner: {{ raffles.raffleWinner.character_name }} @
          {{ raffles.raffleWinner.world }}
        </h3>
        <p class="text-dim text-sm mb-12">{{ raffles.raffleWinner.num_entries }} entries</p>
        <div v-if="raffles.selectedRaffle.status === 'open'" class="flex-toolbar">
          <button
            class="btn-primary"
            :disabled="raffles.pickingWinner"
            @click="raffles.verifyRaffleWinner()"
          >
            <i class="fa-solid fa-circle-check"></i> Verify Winner
          </button>
          <button
            class="btn-ghost"
            :disabled="raffles.pickingWinner"
            @click="raffles.pickAnotherWinner()"
          >
            <LoadingSpinner v-if="raffles.pickingWinner" label="Picking…" />
            <template v-else><i class="fa-solid fa-rotate"></i> Pick Another</template>
          </button>
        </div>
      </div>

      <!-- Pick winner button -->
      <div
        v-if="raffles.selectedRaffle.status === 'open' && !raffles.raffleWinner"
        class="mb-16"
      >
        <button
          class="btn-primary btn-lg"
          :disabled="raffles.pickingWinner"
          @click="raffles.pickRaffleWinner()"
        >
          <LoadingSpinner v-if="raffles.pickingWinner" label="Picking…" />
          <template v-else><i class="fa-solid fa-dice"></i> Pick a Winner</template>
        </button>
      </div>

      <!-- Entries table -->
      <h3 class="mb-8 mt-16">Entries ({{ raffles.raffleEntries.length }})</h3>
      <div v-if="raffles.raffleEntries.length" class="raffle-entries-table">
        <table class="entries-table">
          <thead>
            <tr>
              <th>Character</th>
              <th class="tc">Entries</th>
              <th class="tc">Cost</th>
              <th class="tc">Paid</th>
              <th class="tc">Actions</th>
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
                {{ (e.num_entries * raffles.selectedRaffle.cost_per_entry).toLocaleString() }}
              </td>
              <td class="tc">
                <button
                  :class="['btn-sm', e.paid ? 'btn-primary' : 'btn-ghost']"
                  @click="raffles.toggleEntryPaid(e)"
                >
                  <template v-if="e.paid"><i class="fa-solid fa-circle-check"></i> Paid</template>
                  <template v-else>Unpaid</template>
                </button>
              </td>
              <td class="tc">
                <button class="btn-danger btn-sm" @click="raffles.deleteEntry(e)">&times;</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="msg-block">No entries yet.</p>
    </div>

    <!-- Open raffle list -->
    <div v-else class="admin-panel">
      <h3 class="mb-16"><i class="fa-solid fa-clipboard-list"></i> Open Raffles</h3>
      <LoadingSpinner
        v-if="raffles.rafflesLoading && raffles.raffles.length === 0"
        block
        label="Loading raffles…"
      />
      <template v-else>
        <div class="raffle-list">
          <div
            v-for="r in raffles.openRaffles"
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
        <p v-if="raffles.openRaffles.length === 0" class="no-game-msg">No open raffles.</p>
      </template>
    </div>
  </div>
</template>
