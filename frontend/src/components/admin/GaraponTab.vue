<script setup lang="ts">
/**
 * Admin Garapon manager (Festival → Garapon). Screens:
 *
 *   - list: "Current Garapons" (open) as image cards (with a card-level delete),
 *     then a searchable + paginated "Closed Garapons" table.
 *   - detail: the selected garapon — status/actions, grand-prize image, prizes —
 *     plus links into the two sub-pages below (with live counts).
 *   - links: "Generate Drawing" form (open garapons) + the per-player drawing
 *     links as a searchable, paginated table.
 *   - log: the draw log as a searchable, column-sortable, paginated table.
 *   - form: the create/edit form (GaraponFormTab), a Back sub-page.
 *
 * Open-only controls (edit, generate) are gated by status, so the detail/links
 * pages double as the read-only closed view. All state + actions come from the
 * garapons store; the per-page search/sort/pagination is local client-side state.
 */
import { computed, ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import FormField from '@/components/common/ui/FormField.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import PaginationBar from '@/components/common/ui/PaginationBar.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import BallSwatch from '@/components/common/ui/BallSwatch.vue'
import GaraponFormTab from './GaraponFormTab.vue'
import { useGaraponsStore } from '@/stores/garapons'
import { useDataTableView } from '@/composables/useDataTableView'
import { rateTotal, ratePct as normalizedPct } from '@/lib/garapon'
import { assetUrl } from '@/lib/assets'
import { formatServerTimestamp } from '@/lib/datetime'
import type { Garapon, GaraponPrize, GaraponPlayer, GaraponDraw } from '@/types/api'

const garapons = useGaraponsStore()

type Screen = 'list' | 'detail' | 'links' | 'log' | 'form'
const screen = ref<Screen>('list')

const isOpen = computed(() => garapons.selectedGarapon?.status === 'open')

// Each admin table (closed garapons, drawing links, draw log) is the same
// search + column-sort + paginate view, so useDataTableView owns that pipeline
// and only the columns, match predicate, and starting sort differ here. Column
// keys match the row fields so the generic sort reads them directly.
const closedColumns: DataColumn[] = [
  { key: 'title', label: 'Title', sortable: true },
  { key: 'player_count', label: 'Drawings', align: 'right', sortable: true },
  { key: 'draw_count', label: 'Draws', align: 'right', sortable: true },
  { key: 'created_at', label: 'Created', sortable: true },
  { key: 'actions', label: '', align: 'right' },
]
const {
  search: closedSearch,
  page: closedPage,
  totalPages: closedTotalPages,
  paged: pagedClosed,
  filtered: filteredClosed,
  sortKey: closedSortKey,
  sortDir: closedSortDir,
  setSort: setClosedSort,
} = useDataTableView<Garapon>(() => garapons.closedGarapons, {
  matches: (g, q) => g.title.toLowerCase().includes(q),
})

const linkColumns: DataColumn[] = [
  { key: 'player_name', label: 'Player', sortable: true },
  { key: 'draws_used', label: 'Draws', align: 'center', sortable: true },
  { key: 'created_at', label: 'Created', sortable: true },
  { key: 'actions', label: '', align: 'right' },
]
const {
  search: linkSearch,
  page: linkPage,
  totalPages: linkTotalPages,
  paged: pagedLinks,
  filtered: filteredLinks,
  sortKey: linkSortKey,
  sortDir: linkSortDir,
  setSort: setLinkSort,
  reset: resetLinks,
} = useDataTableView<GaraponPlayer>(() => garapons.garaponPlayers, {
  matches: (p, q) => p.player_name.toLowerCase().includes(q),
  // Newest links first by default (a freshly generated link lands on top).
  sort: { key: 'created_at', dir: 'desc' },
})

const logColumns: DataColumn[] = [
  { key: 'player_name', label: 'Player', sortable: true },
  { key: 'prize_name', label: 'Prize', sortable: true },
  { key: 'drawn_at', label: 'When', sortable: true, align: 'right' },
]
const {
  search: logSearch,
  page: logPage,
  totalPages: logTotalPages,
  paged: pagedLog,
  filtered: filteredLog,
  sortKey: logSortKey,
  sortDir: logSortDir,
  setSort: setLogSort,
  reset: resetLog,
} = useDataTableView<GaraponDraw>(() => garapons.garaponDraws, {
  matches: (d, q) =>
    d.player_name.toLowerCase().includes(q) || d.prize_name.toLowerCase().includes(q),
  sort: { key: 'drawn_at', dir: 'desc' },
})

// ── Display helpers ──────────────────────────────────────────────────────────

/** Sum of the selected garapon's prize weights (for the normalized-% column). */
const prizeRateTotal = computed(() => rateTotal(garapons.selectedGarapon?.prizes || []))
/** A prize's odds as a normalized percentage string (relative weights). */
function ratePct(p: GaraponPrize): string {
  return normalizedPct(p.rate, prizeRateTotal.value)
}

function created(ts: string): string {
  return ts ? formatServerTimestamp(ts) : '—'
}

// ── Navigation ───────────────────────────────────────────────────────────────
/** Reset the per-sub-page search/sort/pagination when opening a garapon. */
function resetSubPages(): void {
  resetLinks()
  resetLog()
}
function openNew(): void {
  garapons.newGaraponForm()
  screen.value = 'form'
}
function openGarapon(g: Garapon): void {
  garapons.viewGarapon(g)
  resetSubPages()
  screen.value = 'detail'
}
function openLinks(): void {
  screen.value = 'links'
}
function openLog(): void {
  screen.value = 'log'
}
function backToDetail(): void {
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
        <span :class="['status-badge', 'status-badge-' + garapons.selectedGarapon.status]">
          {{ garapons.selectedGarapon.status }}
        </span>
      </SubPageHeader>
      <div class="flex-toolbar flex-end mb-16">
        <button class="btn-neutral btn-sm" @click="openLinks">
          <font-awesome-icon :icon="['fad', 'link']" /> Drawing Links ({{
            garapons.garaponPlayers.length
          }})
        </button>
        <button class="btn-neutral btn-sm" @click="openLog">
          <font-awesome-icon :icon="['fad', 'clipboard-list']" /> Draw Log ({{
            garapons.garaponDraws.length
          }})
        </button>
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
          class="prize-img-sm"
          alt="Grand prize"
        />
      </div>

      <!-- Prizes -->
      <h3 class="section-heading mt-8"><font-awesome-icon :icon="['fad', 'gift']" /> Prizes</h3>
      <div class="garapon-table-wrap mb-16">
        <table class="data-table">
          <thead>
            <tr>
              <th>Prize</th>
              <th class="ta-center">Ball Color</th>
              <th class="ta-right">Draw Weight</th>
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
                <BallSwatch :color="p.ball_color" />
              </td>
              <td class="ta-right">{{ p.rate }}</td>
              <td class="ta-right text-dim">{{ ratePct(p) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

    </AdminPanel>

    <!-- ── Drawing Links sub-page ──────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'links' && garapons.selectedGarapon">
      <SubPageHeader @back="backToDetail">
        <font-awesome-icon :icon="['fad', 'link']" /> Drawing Links —
        {{ garapons.selectedGarapon.title }}
      </SubPageHeader>

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

      <!-- Links table (searchable + paginated) -->
      <template v-if="garapons.garaponPlayers.length">
        <div class="manager-toolbar">
          <SearchInput
            v-model="linkSearch"
            placeholder="Search by player…"
            aria-label="Search drawing links"
          />
          <span class="text-dim text-xs push-right">
            {{ filteredLinks.length }} link{{ filteredLinks.length === 1 ? '' : 's' }}
          </span>
        </div>
        <DataTable
          :columns="linkColumns"
          :rows="pagedLinks"
          row-key="id"
          :sort-key="linkSortKey"
          :sort-dir="linkSortDir"
          @sort="setLinkSort"
        >
          <template #cell-draws_used="{ row }">{{ row.draws_used }}/{{ row.max_draws }}</template>
          <template #cell-created_at="{ row }">
            <span class="text-sm">{{ created(row.created_at) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="row-actions">
              <button
                class="btn-view btn-sm"
                aria-label="Copy drawing link"
                title="Copy drawing link"
                @click="garapons.copyPlayerLink(row)"
              >
                <font-awesome-icon :icon="['fas', 'link']" />
              </button>
              <button
                v-if="row.stamp_card_token"
                class="btn-view btn-sm"
                aria-label="Copy stamp card link"
                title="Copy stamp card link (same hash)"
                @click="garapons.copyStampCardLink(row)"
              >
                <font-awesome-icon :icon="['fad', 'stamp']" />
              </button>
              <button
                class="btn-danger btn-sm"
                :disabled="row.draws_used > 0 && isOpen"
                :aria-label="
                  row.draws_used > 0 && isOpen
                    ? 'Cannot delete — player has drawn (garapon is open)'
                    : 'Delete drawing link'
                "
                :title="
                  row.draws_used > 0 && isOpen
                    ? 'Player has already drawn — close the garapon to delete this link (its draws stay in the log)'
                    : 'Delete drawing link'
                "
                @click="garapons.deletePlayer(row)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" />
              </button>
            </div>
          </template>
          <template #empty>
            <EmptyState text="No drawing links match your search." />
          </template>
        </DataTable>
        <PaginationBar
          v-if="linkTotalPages > 1"
          class="mt-12"
          :page="linkPage"
          :total-pages="linkTotalPages"
          @go="(p: number) => (linkPage = p)"
        />
      </template>
      <EmptyState v-else text="No drawing links yet." />
    </AdminPanel>

    <!-- ── Draw Log sub-page ───────────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'log' && garapons.selectedGarapon">
      <SubPageHeader @back="backToDetail">
        <font-awesome-icon :icon="['fad', 'clipboard-list']" /> Draw Log —
        {{ garapons.selectedGarapon.title }}
      </SubPageHeader>

      <template v-if="garapons.garaponDraws.length">
        <div class="manager-toolbar">
          <SearchInput
            v-model="logSearch"
            placeholder="Search by player or prize…"
            aria-label="Search draw log"
          />
          <span class="text-dim text-xs push-right">
            {{ filteredLog.length }} draw{{ filteredLog.length === 1 ? '' : 's' }}
          </span>
        </div>
        <DataTable
          :columns="logColumns"
          :rows="pagedLog"
          row-key="id"
          :sort-key="logSortKey"
          :sort-dir="logSortDir"
          @sort="setLogSort"
        >
          <template #cell-prize_name="{ row }">
            <span class="prize-cell"><BallSwatch :color="row.ball_color" size="sm" /> {{ row.prize_name }}</span>
          </template>
          <template #cell-drawn_at="{ row }">
            <span class="text-sm text-dim">{{ created(row.drawn_at) }}</span>
          </template>
          <template #empty>
            <EmptyState text="No draws match your search." />
          </template>
        </DataTable>
        <PaginationBar
          v-if="logTotalPages > 1"
          class="mt-12"
          :page="logPage"
          :total-pages="logTotalPages"
          @go="(p: number) => (logPage = p)"
        />
      </template>
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
        <div v-if="garapons.openGarapons.length" class="card-grid">
          <div
            v-for="g in garapons.openGarapons"
            :key="g.id"
            class="media-card"
            @click="openGarapon(g)"
          >
            <img
              v-if="g.grand_prize_image"
              :src="assetUrl(g.grand_prize_image)"
              class="media-card-image"
              alt="Grand prize"
            />
            <div class="media-card-body">
              <h3>{{ g.title }}</h3>
              <p class="text-dim text-sm">
                {{ g.player_count || 0 }} drawing{{ g.player_count === 1 ? '' : 's' }} ·
                {{ g.draw_count || 0 }} draw{{ g.draw_count === 1 ? '' : 's' }}
              </p>
              <!-- @click.stop so deleting doesn't also open the detail view. -->
              <div class="garapon-card-actions">
                <button
                  class="btn-danger btn-sm"
                  aria-label="Delete garapon"
                  title="Delete garapon and all its links and results"
                  @click.stop="garapons.deleteGarapon(g.id)"
                >
                  <font-awesome-icon :icon="['fas', 'trash']" /> Delete
                </button>
              </div>
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
          <DataTable
            :columns="closedColumns"
            :rows="pagedClosed"
            row-key="id"
            :sort-key="closedSortKey"
            :sort-dir="closedSortDir"
            @sort="setClosedSort"
          >
            <template #cell-player_count="{ row }">{{ row.player_count || 0 }}</template>
            <template #cell-draw_count="{ row }">{{ row.draw_count || 0 }}</template>
            <template #cell-created_at="{ row }">
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
/* List-level delete on a Current Garapons card. */
.garapon-card-actions {
  margin-top: 10px;
  display: flex;
  justify-content: flex-end;
}
/* Prize-name cell: swatch + label aligned on one baseline. */
.prize-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.grand-star {
  color: var(--highlight);
  margin-right: 4px;
}
</style>
