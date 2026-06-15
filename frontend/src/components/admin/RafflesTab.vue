<script setup lang="ts">
/**
 * Admin Raffles manager — one tab (under Senpan Tea House) unifying the former
 * New / Open / Closed raffle tabs into the standard manager model:
 *
 *   - list: "Current Raffles" (every non-closed raffle) as image cards with a
 *     corner status icon (a calendar-clock when it opens later, a red
 *     calendar-circle-exclamation when its open window has already passed), then a
 *     searchable + paginated "Closed Raffles" table (title, winner, open period,
 *     and the gil collected from paid entries) with a Copy action that seeds a new
 *     raffle from a past one.
 *   - detail: the selected raffle (winner pick/verify, add entry, entries table) —
 *     open-only controls are gated by status, so it doubles as the read-only closed
 *     view. Opened with a Back sub-header.
 *   - form: the create/edit/copy form (RaffleFormTab), also a Back sub-page.
 *
 * All state + actions come from the raffles store (unchanged besides copyRaffleForm).
 */
import { computed, ref, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import FormField from '@/components/common/ui/FormField.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import PaginationBar from '@/components/common/ui/PaginationBar.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import RaffleFormTab from './RaffleFormTab.vue'
import { useRafflesStore } from '@/stores/raffles'
import { assetUrl } from '@/lib/assets'
import { formatServerTimestamp, parseServerTimestamp } from '@/lib/datetime'
import type { Raffle } from '@/types/api'

const raffles = useRafflesStore()

type Screen = 'list' | 'detail' | 'form'
const screen = ref<Screen>('list')

const isOpen = computed(() => raffles.selectedRaffle?.status === 'open')

// ── Closed-raffle table: client-side search + pagination ─────────────────────
const closedSearch = ref('')
const closedPage = ref(1)
const PER_PAGE = 10

const closedColumns: DataColumn[] = [
  { key: 'title', label: 'Title' },
  { key: 'winner', label: 'Winner' },
  { key: 'period', label: 'Open period' },
  { key: 'total', label: 'Total', align: 'right' },
  { key: 'actions', label: '', align: 'right' },
]

const filteredClosed = computed(() => {
  const q = closedSearch.value.trim().toLowerCase()
  if (!q) return raffles.closedRaffles
  return raffles.closedRaffles.filter((r) =>
    [r.title, r.winner_name ?? ''].some((s) => s.toLowerCase().includes(q)),
  )
})
const closedTotalPages = computed(() => Math.max(1, Math.ceil(filteredClosed.value.length / PER_PAGE)))
const pagedClosed = computed(() => {
  const start = (closedPage.value - 1) * PER_PAGE
  return filteredClosed.value.slice(start, start + PER_PAGE)
})
// A new search resets to the first page; a shrinking list clamps the page.
watch(closedSearch, () => (closedPage.value = 1))
watch(closedTotalPages, (n) => {
  if (closedPage.value > n) closedPage.value = n
})

// ── Display helpers ──────────────────────────────────────────────────────────

/**
 * Timing badge for an open raffle relative to its availability window:
 * 'upcoming' when it opens later, 'ended' when its window has passed, '' when
 * it's live (no icon). Closed raffles never show one.
 */
function raffleTiming(r: Raffle): 'upcoming' | 'ended' | '' {
  if (r.status !== 'open') return ''
  const now = Date.now()
  const from = parseServerTimestamp(r.available_from)
  if (!Number.isNaN(from) && from > now) return 'upcoming'
  const to = parseServerTimestamp(r.available_to)
  if (!Number.isNaN(to) && to <= now) return 'ended'
  return ''
}

/** Compact "from – to" availability window for the closed table. */
function periodLabel(r: Raffle): string {
  const from = r.available_from ? formatServerTimestamp(r.available_from) : ''
  const to = r.available_to ? formatServerTimestamp(r.available_to) : ''
  if (!from && !to) return 'Always open'
  return `${from || '—'} – ${to || '—'}`
}

/** Gil amount formatted with thousands separators (or "—" for nothing). */
function gilLabel(amount: number | undefined): string {
  if (!amount) return '—'
  return `${amount.toLocaleString()} gil`
}

// ── Navigation ───────────────────────────────────────────────────────────────
function openNew(): void {
  raffles.newRaffleForm()
  screen.value = 'form'
}
function openRaffle(r: Raffle): void {
  raffles.viewRaffle(r)
  screen.value = 'detail'
}
function editSelected(): void {
  if (!raffles.selectedRaffle) return
  raffles.editRaffleForm(raffles.selectedRaffle)
  screen.value = 'form'
}
function copyRaffle(r: Raffle): void {
  raffles.copyRaffleForm(r)
  screen.value = 'form'
}
function backToList(): void {
  raffles.selectedRaffle = null
  screen.value = 'list'
}
function onFormDone(): void {
  screen.value = 'list'
}
async function deleteSelected(): Promise<void> {
  if (!raffles.selectedRaffle) return
  await raffles.deleteRaffle(raffles.selectedRaffle.id)
  // deleteRaffle clears selectedRaffle on success → return to the list.
  if (!raffles.selectedRaffle) screen.value = 'list'
}
</script>

<template>
  <div class="tab-body">
    <!-- ── Form ──────────────────────────────────────────────────────────────── -->
    <RaffleFormTab v-if="screen === 'form'" @saved="onFormDone" @cancel="onFormDone" />

    <!-- ── Detail (open controls gated by status; doubles as the closed view) ──── -->
    <AdminPanel v-else-if="screen === 'detail' && raffles.selectedRaffle">
      <SubPageHeader @back="backToList">
        {{ raffles.selectedRaffle.title }}
        <span :class="['raffle-badge', 'raffle-badge-' + raffles.selectedRaffle.status]">
          {{ raffles.selectedRaffle.status }}
        </span>
      </SubPageHeader>
      <div class="flex-toolbar flex-end mb-16">
        <button v-if="isOpen" class="btn-secondary btn-sm" @click="editSelected">
          <i class="fa-solid fa-pen-to-square"></i> Edit
        </button>
        <button class="btn-danger btn-sm" @click="deleteSelected">
          <i class="fa-solid fa-trash"></i> Delete
        </button>
      </div>

      <!-- Prize image -->
      <div v-if="raffles.selectedRaffle.prize_image" class="mb-16">
        <img
          :src="assetUrl(raffles.selectedRaffle.prize_image)"
          class="raffle-prize-img-sm"
          alt="Prize"
        />
      </div>

      <!-- Winner section -->
      <div v-if="raffles.raffleWinner" class="raffle-winner-panel">
        <h3 class="raffle-section-heading">
          <i class="fa-duotone fa-trophy"></i> Winner: {{ raffles.raffleWinner.character_name }} @
          {{ raffles.raffleWinner.world }}
        </h3>
        <p class="text-dim text-sm mb-12">{{ raffles.raffleWinner.num_entries }} entries</p>
        <div v-if="isOpen" class="flex-toolbar">
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
      <div v-if="isOpen && !raffles.raffleWinner" class="mb-16">
        <button
          class="btn-primary btn-lg"
          :disabled="raffles.pickingWinner"
          @click="raffles.pickRaffleWinner()"
        >
          <LoadingSpinner v-if="raffles.pickingWinner" label="Picking…" />
          <template v-else><i class="fa-solid fa-dice"></i> Pick a Winner</template>
        </button>
      </div>

      <!-- Add entry (admin, open only) -->
      <div v-if="isOpen" class="entry-add mt-16 mb-16">
        <h3 class="raffle-section-heading"><i class="fa-duotone fa-plus"></i> Add Entry</h3>
        <div class="flex-row mb-10">
          <FormField label="Character Name" style="flex: 2; min-width: 160px">
            <input
              v-model="raffles.entryAdd.characterName"
              placeholder="Character name"
              aria-label="Character name"
              @keyup.enter="raffles.addRaffleEntry()"
            />
          </FormField>
          <FormField label="World" style="flex: 1; min-width: 120px">
            <input
              v-model="raffles.entryAdd.world"
              placeholder="World"
              aria-label="World"
              @keyup.enter="raffles.addRaffleEntry()"
            />
          </FormField>
          <FormField label="Entries" style="flex: 0 0 96px; min-width: 80px">
            <input
              v-model.number="raffles.entryAdd.numEntries"
              type="number"
              min="1"
              :max="raffles.selectedRaffle.max_entries"
              aria-label="Number of entries"
            />
          </FormField>
        </div>
        <div class="flex-toolbar entry-add-actions">
          <label class="entry-add-paid">
            <input v-model="raffles.entryAdd.paid" type="checkbox" />
            Mark as paid
          </label>
          <button
            class="btn-primary btn-sm"
            :disabled="
              raffles.addingEntry ||
              !raffles.entryAdd.characterName.trim() ||
              !raffles.entryAdd.world.trim()
            "
            @click="raffles.addRaffleEntry()"
          >
            <LoadingSpinner v-if="raffles.addingEntry" label="Adding…" />
            <template v-else><i class="fa-solid fa-plus"></i> Add Entry</template>
          </button>
        </div>
      </div>

      <!-- Entries table -->
      <h3 class="mb-8 mt-16">Entries ({{ raffles.raffleEntries.length }})</h3>
      <div v-if="raffles.raffleEntries.length" class="raffle-entries-table">
        <table class="data-table">
          <thead>
            <tr>
              <th>Character</th>
              <th class="ta-center">Entries</th>
              <th class="ta-center">Cost</th>
              <th class="ta-center">Paid</th>
              <th v-if="isOpen" class="ta-center">Actions</th>
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
              <td class="ta-center">{{ e.num_entries }}</td>
              <td class="ta-center">
                {{ (e.num_entries * raffles.selectedRaffle.cost_per_entry).toLocaleString() }}
              </td>
              <td class="ta-center">
                <button
                  v-if="isOpen"
                  :class="['btn-sm', e.paid ? 'btn-primary' : 'btn-ghost']"
                  @click="raffles.toggleEntryPaid(e)"
                >
                  <template v-if="e.paid"><i class="fa-solid fa-circle-check"></i> Paid</template>
                  <template v-else>Unpaid</template>
                </button>
                <template v-else>
                  <i v-if="e.paid" class="fa-duotone fa-circle-check"></i>
                  <template v-else>—</template>
                </template>
              </td>
              <td v-if="isOpen" class="ta-center">
                <button class="btn-danger btn-sm" @click="raffles.deleteEntry(e)">&times;</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <EmptyState v-else text="No entries yet." />
    </AdminPanel>

    <!-- ── List ──────────────────────────────────────────────────────────────── -->
    <ManagerView v-else title="Raffles" icon="fa-duotone fa-ticket">
      <template #actions>
        <button class="btn-primary btn-sm" @click="openNew">
          <i class="fa-solid fa-plus"></i> New Raffle
        </button>
      </template>

      <LoadingSpinner
        v-if="raffles.rafflesLoading && raffles.raffles.length === 0"
        block
        label="Loading raffles…"
      />
      <template v-else>
        <!-- Current (non-closed) raffles -->
        <h4 class="section-heading"><i class="fa-duotone fa-clipboard-list"></i> Current Raffles</h4>
        <div v-if="raffles.openRaffles.length" class="raffle-list">
          <div
            v-for="r in raffles.openRaffles"
            :key="r.id"
            class="raffle-card"
            @click="openRaffle(r)"
          >
            <span
              v-if="raffleTiming(r) === 'upcoming'"
              class="raffle-status-icon"
              title="Scheduled — opens later"
            >
              <i class="fa-duotone fa-calendar-clock"></i>
            </span>
            <span
              v-else-if="raffleTiming(r) === 'ended'"
              class="raffle-status-icon raffle-status-ended"
              title="Open period has passed"
            >
              <i class="fa-duotone fa-calendar-circle-exclamation"></i>
            </span>
            <img
              v-if="r.prize_image"
              :src="assetUrl(r.prize_image)"
              class="raffle-card-image"
              alt="Prize"
            />
            <div class="raffle-card-body">
              <h3>{{ r.title }}</h3>
              <p v-if="r.cost_per_entry > 0" class="raffle-cost">
                {{ r.cost_per_entry.toLocaleString() }} gil per entry
              </p>
            </div>
          </div>
        </div>
        <EmptyState v-else text="No current raffles." />

        <!-- Closed raffles table -->
        <h4 class="section-heading mt-20"><i class="fa-duotone fa-lock"></i> Closed Raffles</h4>
        <template v-if="raffles.closedRaffles.length">
          <div class="manager-toolbar">
            <SearchInput
              v-model="closedSearch"
              placeholder="Search closed raffles…"
              aria-label="Search closed raffles"
            />
            <span class="text-dim text-xs push-right">
              {{ filteredClosed.length }} closed
            </span>
          </div>
          <DataTable :columns="closedColumns" :rows="pagedClosed" row-key="id">
            <template #cell-title="{ row }">{{ row.title }}</template>
            <template #cell-winner="{ row }">{{ row.winner_name || '—' }}</template>
            <template #cell-period="{ row }">
              <span class="text-sm">{{ periodLabel(row) }}</span>
            </template>
            <template #cell-total="{ row }">{{ gilLabel(row.paid_total) }}</template>
            <template #cell-actions="{ row }">
              <div class="row-actions">
                <button
                  class="btn-secondary btn-sm"
                  aria-label="View"
                  title="View"
                  @click="openRaffle(row)"
                >
                  <i class="fa-solid fa-eye"></i>
                </button>
                <button
                  class="btn-secondary btn-sm"
                  aria-label="Copy to new raffle"
                  title="Copy to new raffle"
                  @click="copyRaffle(row)"
                >
                  <i class="fa-solid fa-copy"></i>
                </button>
                <button
                  class="btn-danger btn-sm"
                  aria-label="Delete"
                  title="Delete"
                  @click="raffles.deleteRaffle(row.id)"
                >
                  <i class="fa-solid fa-trash"></i>
                </button>
              </div>
            </template>
            <template #empty>
              <EmptyState text="No closed raffles match your search." />
            </template>
          </DataTable>
          <PaginationBar
            v-if="closedTotalPages > 1"
            class="mt-12"
            :page="closedPage"
            :total-pages="closedTotalPages"
            @go="(p: number) => (closedPage = p)"
          />
        </template>
        <EmptyState v-else text="No closed raffles yet." />
      </template>
    </ManagerView>
  </div>
</template>

<style scoped>
.entry-add {
  background: var(--panel-raised-bg);
  border-radius: var(--radius);
  padding: 14px 16px;
}
.entry-add-actions {
  justify-content: space-between;
}
.entry-add-paid {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
/* Corner timing badge over a current-raffle card's image. */
.raffle-card {
  position: relative;
}
.raffle-status-icon {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.55);
  color: var(--highlight);
  font-size: 0.95rem;
}
.raffle-status-ended {
  color: var(--danger);
}
</style>
