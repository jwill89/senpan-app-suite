<script setup lang="ts">
/**
 * Admin Garapon manager (Festival → Garapon). One tab with three screens,
 * modeled on RafflesTab:
 *
 *   - list: "Current Garapons" (open) as image cards, then a searchable +
 *     paginated "Closed Garapons" table.
 *   - detail: the selected garapon — prizes, a "Generate Drawing" form that
 *     issues per-player links, the drawing-links table (copy link / draw-gated
 *     delete / used-over-max), and the draw log. Open-only controls are gated by
 *     status so it doubles as the read-only closed view.
 *   - form: the create/edit form (GaraponFormTab), a Back sub-page.
 *
 * All state + actions come from the garapons store.
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
import GaraponFormTab from './GaraponFormTab.vue'
import { useGaraponsStore } from '@/stores/garapons'
import { assetUrl } from '@/lib/assets'
import { formatServerTimestamp } from '@/lib/datetime'
import type { Garapon, GaraponPrize } from '@/types/api'

const garapons = useGaraponsStore()

type Screen = 'list' | 'detail' | 'form'
const screen = ref<Screen>('list')

const isOpen = computed(() => garapons.selectedGarapon?.status === 'open')

// ── Closed-garapon table: client-side search + pagination ────────────────────
const closedSearch = ref('')
const closedPage = ref(1)
const PER_PAGE = 10

const closedColumns: DataColumn[] = [
  { key: 'title', label: 'Title' },
  { key: 'players', label: 'Drawings', align: 'right' },
  { key: 'draws', label: 'Draws', align: 'right' },
  { key: 'created', label: 'Created' },
  { key: 'actions', label: '', align: 'right' },
]

const playerColumns: DataColumn[] = [
  { key: 'player', label: 'Player' },
  { key: 'draws', label: 'Draws', align: 'center' },
  { key: 'created', label: 'Created' },
  { key: 'actions', label: '', align: 'right' },
]

const filteredClosed = computed(() => {
  const q = closedSearch.value.trim().toLowerCase()
  if (!q) return garapons.closedGarapons
  return garapons.closedGarapons.filter((g) => g.title.toLowerCase().includes(q))
})
const closedTotalPages = computed(() => Math.max(1, Math.ceil(filteredClosed.value.length / PER_PAGE)))
const pagedClosed = computed(() => {
  const start = (closedPage.value - 1) * PER_PAGE
  return filteredClosed.value.slice(start, start + PER_PAGE)
})
watch(closedSearch, () => (closedPage.value = 1))
watch(closedTotalPages, (n) => {
  if (closedPage.value > n) closedPage.value = n
})

// ── Display helpers ──────────────────────────────────────────────────────────

/** Sum of the selected garapon's prize rates (for the normalized-% column). */
const prizeRateTotal = computed(() =>
  (garapons.selectedGarapon?.prizes || []).reduce((sum, p) => sum + (p.rate > 0 ? p.rate : 0), 0),
)
/** A prize's odds as a normalized percentage string (relative weights). */
function ratePct(p: GaraponPrize): string {
  const total = prizeRateTotal.value
  if (total <= 0) return '—'
  return `${((Math.max(0, p.rate) / total) * 100).toFixed(1)}%`
}

function created(ts: string): string {
  return ts ? formatServerTimestamp(ts) : '—'
}

// ── Navigation ───────────────────────────────────────────────────────────────
function openNew(): void {
  garapons.newGaraponForm()
  screen.value = 'form'
}
function openGarapon(g: Garapon): void {
  garapons.viewGarapon(g)
  screen.value = 'detail'
}
function editSelected(): void {
  if (!garapons.selectedGarapon) return
  garapons.editGaraponForm(garapons.selectedGarapon)
  screen.value = 'form'
}
function backToList(): void {
  garapons.selectedGarapon = null
  screen.value = 'list'
}
function onFormDone(): void {
  screen.value = 'list'
}
async function deleteSelected(): Promise<void> {
  if (!garapons.selectedGarapon) return
  await garapons.deleteGarapon(garapons.selectedGarapon.id)
  if (!garapons.selectedGarapon) screen.value = 'list'
}
function toggleClosed(): void {
  if (!garapons.selectedGarapon) return
  garapons.setGaraponStatus(
    garapons.selectedGarapon.id,
    garapons.selectedGarapon.status === 'open' ? 'closed' : 'open',
  )
}
</script>

<template>
  <div class="tab-body">
    <!-- ── Form ──────────────────────────────────────────────────────────────── -->
    <GaraponFormTab v-if="screen === 'form'" @saved="onFormDone" @cancel="onFormDone" />

    <!-- ── Detail ──────────────────────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'detail' && garapons.selectedGarapon">
      <SubPageHeader @back="backToList">
        {{ garapons.selectedGarapon.title }}
        <span :class="['raffle-badge', 'raffle-badge-' + garapons.selectedGarapon.status]">
          {{ garapons.selectedGarapon.status }}
        </span>
      </SubPageHeader>
      <div class="flex-toolbar flex-end mb-16">
        <button v-if="isOpen" class="btn-confirm btn-sm" @click="editSelected">
          <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Edit
        </button>
        <button class="btn-caution btn-sm" @click="toggleClosed">
          <font-awesome-icon :icon="['fas', isOpen ? 'lock' : 'rotate']" />
          {{ isOpen ? 'Close' : 'Reopen' }}
        </button>
        <button class="btn-danger btn-sm" @click="deleteSelected">
          <font-awesome-icon :icon="['fas', 'trash']" /> Delete
        </button>
      </div>

      <!-- Grand prize image -->
      <div v-if="garapons.selectedGarapon.grand_prize_image" class="mb-16">
        <img
          :src="assetUrl(garapons.selectedGarapon.grand_prize_image)"
          class="raffle-prize-img-sm"
          alt="Grand prize"
        />
      </div>

      <!-- Prizes -->
      <h3 class="section-heading mt-8">
        <font-awesome-icon :icon="['fad', 'gift']" /> Prizes
      </h3>
      <div class="garapon-table-wrap mb-16">
        <table class="data-table">
          <thead>
            <tr>
              <th>Prize</th>
              <th class="ta-center">Ball</th>
              <th class="ta-right">Rate</th>
              <th class="ta-right">Odds</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in garapons.selectedGarapon.prizes" :key="p.id">
              <td>
                <span v-if="p.is_grand" class="grand-star" title="Grand prize">
                  <font-awesome-icon :icon="['fad', 'trophy']" />
                </span>
                {{ p.name }}
              </td>
              <td class="ta-center">
                <span class="ball-swatch" :style="{ background: p.ball_color }"></span>
              </td>
              <td class="ta-right">{{ p.rate }}</td>
              <td class="ta-right text-dim">{{ ratePct(p) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Generate a drawing (open only) -->
      <div v-if="isOpen" class="entry-add mb-16">
        <h3 class="section-heading">
          <font-awesome-icon :icon="['fad', 'ticket']" /> Generate Drawing
        </h3>
        <div class="flex-row mb-10">
          <FormField label="Player Name" style="flex: 2; min-width: 160px">
            <input
              v-model="garapons.playerAdd.playerName"
              placeholder="Player name"
              aria-label="Player name"
              @keyup.enter="garapons.createPlayer()"
            />
          </FormField>
          <FormField label="Number of Draws" style="flex: 0 0 140px; min-width: 110px">
            <input
              v-model.number="garapons.playerAdd.maxDraws"
              type="number"
              min="1"
              aria-label="Number of draws"
            />
          </FormField>
        </div>
        <div class="flex-toolbar flex-end">
          <button
            class="btn-confirm btn-sm"
            :disabled="garapons.creatingPlayer || !garapons.playerAdd.playerName.trim()"
            @click="garapons.createPlayer()"
          >
            <LoadingSpinner v-if="garapons.creatingPlayer" label="Creating…" />
            <template v-else><font-awesome-icon :icon="['fas', 'plus']" /> Create Link</template>
          </button>
        </div>
      </div>

      <!-- Drawing links -->
      <h3 class="section-heading mt-8">
        <font-awesome-icon :icon="['fad', 'link']" /> Drawing Links ({{ garapons.garaponPlayers.length }})
      </h3>
      <DataTable
        v-if="garapons.garaponPlayers.length"
        :columns="playerColumns"
        :rows="garapons.garaponPlayers"
        row-key="id"
      >
        <template #cell-player="{ row }">{{ row.player_name }}</template>
        <template #cell-draws="{ row }">{{ row.draws_used }}/{{ row.max_draws }}</template>
        <template #cell-created="{ row }">
          <span class="text-sm">{{ created(row.created_at) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <div class="row-actions">
            <button
              class="btn-view btn-sm"
              aria-label="Copy link"
              title="Copy link"
              @click="garapons.copyPlayerLink(row)"
            >
              <font-awesome-icon :icon="['fas', 'link']" />
            </button>
            <button
              class="btn-danger btn-sm"
              :disabled="row.draws_used > 0"
              :aria-label="row.draws_used > 0 ? 'Cannot delete — player has drawn' : 'Delete'"
              :title="row.draws_used > 0 ? 'Player has already drawn' : 'Delete'"
              @click="garapons.deletePlayer(row)"
            >
              <font-awesome-icon :icon="['fas', 'trash']" />
            </button>
          </div>
        </template>
      </DataTable>
      <EmptyState v-else text="No drawing links yet." />

      <!-- Draw log -->
      <h3 class="section-heading mt-16">
        <font-awesome-icon :icon="['fad', 'clipboard-list']" /> Draw Log ({{ garapons.garaponDraws.length }})
      </h3>
      <div v-if="garapons.garaponDraws.length" class="garapon-table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>Player</th>
              <th>Prize</th>
              <th class="ta-right">When</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="d in garapons.garaponDraws" :key="d.id">
              <td>{{ d.player_name }}</td>
              <td>
                <span class="ball-swatch ball-swatch-sm" :style="{ background: d.ball_color }"></span>
                {{ d.prize_name }}
              </td>
              <td class="ta-right text-sm text-dim">{{ created(d.drawn_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <EmptyState v-else text="No draws yet." />
    </AdminPanel>

    <!-- ── List ──────────────────────────────────────────────────────────────── -->
    <ManagerView v-else title="Garapon" :icon="['fad', 'ferris-wheel']">
      <template #actions>
        <button class="btn-confirm btn-sm" @click="openNew">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Garapon
        </button>
      </template>

      <LoadingSpinner
        v-if="garapons.garaponsLoading && garapons.garapons.length === 0"
        block
        label="Loading garapons…"
      />
      <template v-else>
        <!-- Current (open) garapons -->
        <h4 class="section-heading">
          <font-awesome-icon :icon="['fad', 'ferris-wheel']" /> Current Garapons
        </h4>
        <div v-if="garapons.openGarapons.length" class="raffle-list">
          <div
            v-for="g in garapons.openGarapons"
            :key="g.id"
            class="raffle-card"
            @click="openGarapon(g)"
          >
            <img
              v-if="g.grand_prize_image"
              :src="assetUrl(g.grand_prize_image)"
              class="raffle-card-image"
              alt="Grand prize"
            />
            <div class="raffle-card-body">
              <h3>{{ g.title }}</h3>
              <p class="text-dim text-sm">
                {{ g.player_count || 0 }} drawing{{ g.player_count === 1 ? '' : 's' }} ·
                {{ g.draw_count || 0 }} draw{{ g.draw_count === 1 ? '' : 's' }}
              </p>
            </div>
          </div>
        </div>
        <EmptyState v-else text="No current garapons." />

        <!-- Closed garapons table -->
        <h4 class="section-heading mt-20">
          <font-awesome-icon :icon="['fad', 'lock']" /> Closed Garapons
        </h4>
        <template v-if="garapons.closedGarapons.length">
          <div class="manager-toolbar">
            <SearchInput
              v-model="closedSearch"
              placeholder="Search closed garapons…"
              aria-label="Search closed garapons"
            />
            <span class="text-dim text-xs push-right">{{ filteredClosed.length }} closed</span>
          </div>
          <DataTable :columns="closedColumns" :rows="pagedClosed" row-key="id">
            <template #cell-title="{ row }">{{ row.title }}</template>
            <template #cell-players="{ row }">{{ row.player_count || 0 }}</template>
            <template #cell-draws="{ row }">{{ row.draw_count || 0 }}</template>
            <template #cell-created="{ row }">
              <span class="text-sm">{{ created(row.created_at) }}</span>
            </template>
            <template #cell-actions="{ row }">
              <div class="row-actions">
                <button
                  class="btn-view btn-sm"
                  aria-label="View"
                  title="View"
                  @click="openGarapon(row)"
                >
                  <font-awesome-icon :icon="['fas', 'eye']" />
                </button>
                <button
                  class="btn-danger btn-sm"
                  aria-label="Delete"
                  title="Delete"
                  @click="garapons.deleteGarapon(row.id)"
                >
                  <font-awesome-icon :icon="['fas', 'trash']" />
                </button>
              </div>
            </template>
            <template #empty>
              <EmptyState text="No closed garapons match your search." />
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
        <EmptyState v-else text="No closed garapons yet." />
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
.garapon-table-wrap {
  overflow-x: auto;
}
/* A round colored chip standing in for a Garapon ball. */
.ball-swatch {
  display: inline-block;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  border: 1px solid var(--control-border);
  box-shadow: inset 0 -2px 3px rgba(0, 0, 0, 0.25), inset 0 2px 3px rgba(255, 255, 255, 0.35);
  vertical-align: middle;
}
.ball-swatch-sm {
  width: 14px;
  height: 14px;
  margin-right: 6px;
}
.grand-star {
  color: var(--highlight);
  margin-right: 4px;
}
</style>
