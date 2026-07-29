<script setup lang="ts">
/**
 * Admin Garapon manager (Festival -> Garapon). Screens:
 *
 *   - list: "Current Garapons" (open) as image cards (with a card-level delete),
 *     then a searchable + paginated "Closed Garapons" table.
 *   - detail: the selected garapon - status/actions, grand-prize image, prizes -
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
import DataTableToolbar from '@/components/common/ui/DataTableToolbar.vue'
import DataTable, {
  type DataColumn,
  type DataTableView,
} from '@/components/common/ui/DataTable.vue'
import PaginationBar from '@/components/common/ui/PaginationBar.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import BallSwatch from '@/components/common/ui/BallSwatch.vue'
import GaraponFormTab from './GaraponFormTab.vue'
import { useGaraponsStore } from '@/stores/garapons'
import { rateTotal, ratePct as normalizedPct } from '@/lib/garapon'
import { assetUrl } from '@/lib/assets'
import { formatServerTimestamp } from '@/lib/datetime'
import type { Garapon, GaraponPrize, GaraponPlayer, GaraponDraw } from '@/types/api'

const garapons = useGaraponsStore()

type Screen = 'list' | 'detail' | 'links' | 'log' | 'form'
const screen = ref<Screen>('list')

const isOpen = computed(() => garapons.selectedGarapon?.status === 'open')

// Each admin table (closed garapons, drawing links, draw log) is the same
// search + column-sort + paginate view, and DataTable owns that pipeline - only
// the columns, match predicate, and starting sort differ here. Column keys match
// the row fields so the table's sort reads them directly.
const closedColumns: DataColumn[] = [
  { key: 'title', label: 'Title', sortable: true },
  { key: 'player_count', label: 'Drawings', align: 'right', sortable: true },
  { key: 'draw_count', label: 'Draws', align: 'right', sortable: true },
  { key: 'created_at', label: 'Created', sortable: true },
  { key: 'actions', label: '', align: 'right' },
]
const closedSearch = ref('')
const closedPage = ref(1)
const closedView = ref<DataTableView>({ total: 0, totalPages: 1, facets: {} })
const closedMatches = (g: Garapon, q: string): boolean => g.title.toLowerCase().includes(q)

const linkColumns: DataColumn[] = [
  { key: 'player_name', label: 'Player', sortable: true },
  { key: 'draws_used', label: 'Draws', align: 'center', sortable: true },
  { key: 'created_at', label: 'Created', sortable: true },
  { key: 'actions', label: '', align: 'right' },
]
const linkSearch = ref('')
const linkPage = ref(1)
const linkView = ref<DataTableView>({ total: 0, totalPages: 1, facets: {} })
const linkMatches = (p: GaraponPlayer, q: string): boolean =>
  p.player_name.toLowerCase().includes(q)
// Newest links first by default (a freshly generated link lands on top).
const linkSort = { key: 'created_at', dir: 'desc' } as const

const logColumns: DataColumn[] = [
  { key: 'player_name', label: 'Player', sortable: true },
  // Facetable: "which prizes actually went out, and how many of each" is the
  // question this log gets asked, so its distinct values drive a filter.
  { key: 'prize_name', label: 'Prize', sortable: true, facetable: true },
  { key: 'drawn_at', label: 'When', sortable: true, align: 'right' },
]
const logSearch = ref('')
const logPage = ref(1)
const logView = ref<DataTableView>({ total: 0, totalPages: 1, facets: {} })
const logMatches = (d: GaraponDraw, q: string): boolean =>
  d.player_name.toLowerCase().includes(q) || d.prize_name.toLowerCase().includes(q)
const logSort = { key: 'drawn_at', dir: 'desc' } as const
const logPrizeFilter = ref('')
const logFilters = computed(() => ({ prize_name: logPrizeFilter.value }))
const logTableRef = ref<{ exportCsv: (name?: string) => void } | null>(null)

// -- Display helpers ----------------------------------------------------------

/** Sum of the selected garapon's prize weights (for the normalized-% column). */
const prizeRateTotal = computed(() => rateTotal(garapons.selectedGarapon?.prizes || []))
/** A prize's odds as a normalized percentage string (relative weights). */
function ratePct(p: GaraponPrize): string {
  return normalizedPct(p.rate, prizeRateTotal.value)
}

function created(ts: string): string {
  return ts ? formatServerTimestamp(ts) : '-'
}

// -- Navigation ---------------------------------------------------------------
/** Reset the per-sub-page search/sort/pagination when opening a garapon. */
function resetSubPages(): void {
  linkSearch.value = ''
  linkPage.value = 1
  logSearch.value = ''
  logPage.value = 1
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
  const g = garapons.selectedGarapon
  if (!g) return
  await garapons.deleteGarapon(g.id)
  if (!garapons.selectedGarapon) screen.value = 'list'
}
function toggleClosed(): void {
  if (!garapons.selectedGarapon) return
  void garapons.setGaraponStatus(
    garapons.selectedGarapon.id,
    garapons.selectedGarapon.status === 'open' ? 'closed' : 'open',
  )
}
</script>

<template>
  <div class="tab-body">
    <!-- -- Form ---------------------------------------------------------------- -->
    <GaraponFormTab v-if="screen === 'form'" @saved="onFormDone" @cancel="onFormDone" />

    <!-- -- Detail ---------------------------------------------------------------- -->
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
              <td class="ta-right text-muted">{{ ratePct(p) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </AdminPanel>

    <!-- -- Drawing Links sub-page ------------------------------------------------ -->
    <AdminPanel v-else-if="screen === 'links' && garapons.selectedGarapon">
      <SubPageHeader @back="backToDetail">
        <font-awesome-icon :icon="['fad', 'link']" /> Drawing Links -
        {{ garapons.selectedGarapon.title }}
      </SubPageHeader>

      <!-- Generate a drawing (open only) -->
      <div v-if="isOpen" class="subpanel mb-16">
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
            <LoadingSpinner v-if="garapons.creatingPlayer" label="Creating..." />
            <template v-else><font-awesome-icon :icon="['fas', 'plus']" /> Create Link</template>
          </button>
        </div>
      </div>

      <!-- Links table (searchable + paginated) -->
      <template v-if="garapons.garaponPlayers.length">
        <DataTableToolbar
          :count="linkView.total"
          :total="garapons.garaponPlayers.length"
          noun="link"
        >
          <template #search>
            <SearchInput
              v-model="linkSearch"
              placeholder="Search by player..."
              aria-label="Search drawing links"
            />
          </template>
        </DataTableToolbar>
        <DataTable
          v-model:page="linkPage"
          :columns="linkColumns"
          :rows="garapons.garaponPlayers"
          row-key="id"
          :filter="linkSearch"
          :filter-fn="linkMatches"
          :page-size="10"
          :default-sort="linkSort"
          @update:view="linkView = $event"
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
                    ? 'Cannot delete - player has drawn (garapon is open)'
                    : 'Delete drawing link'
                "
                :title="
                  row.draws_used > 0 && isOpen
                    ? 'Player has already drawn - close the garapon to delete this link (its draws stay in the log)'
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
          v-if="linkView.totalPages > 1"
          class="mt-12"
          :page="linkPage"
          :total-pages="linkView.totalPages"
          @go="(p: number) => (linkPage = p)"
        />
      </template>
      <EmptyState v-else text="No drawing links yet." />
    </AdminPanel>

    <!-- -- Draw Log sub-page ----------------------------------------------------- -->
    <AdminPanel v-else-if="screen === 'log' && garapons.selectedGarapon">
      <SubPageHeader @back="backToDetail">
        <font-awesome-icon :icon="['fad', 'clipboard-list']" /> Draw Log -
        {{ garapons.selectedGarapon.title }}
      </SubPageHeader>

      <template v-if="garapons.garaponDraws.length">
        <DataTableToolbar :count="logView.total" :total="garapons.garaponDraws.length" noun="draw">
          <template #search>
            <SearchInput
              v-model="logSearch"
              placeholder="Search by player or prize..."
              aria-label="Search draw log"
            />
          </template>
          <template #filters>
            <select v-model="logPrizeFilter" aria-label="Filter by prize" style="width: 170px">
              <option value="">All prizes</option>
              <option v-for="f in logView.facets.prize_name" :key="f.value" :value="f.value">
                {{ f.value }} ({{ f.count }})
              </option>
            </select>
          </template>
          <template #actions>
            <button
              class="btn-view btn-sm"
              :disabled="!logView.total"
              title="Download the draws currently shown, as CSV"
              @click="logTableRef?.exportCsv('garapon-draw-log')"
            >
              <font-awesome-icon :icon="['fas', 'file-arrow-down']" /> Export CSV
            </button>
          </template>
        </DataTableToolbar>
        <DataTable
          ref="logTableRef"
          v-model:page="logPage"
          :columns="logColumns"
          :rows="garapons.garaponDraws"
          row-key="id"
          :filter="logSearch"
          :filter-fn="logMatches"
          :column-filters="logFilters"
          :page-size="10"
          :default-sort="logSort"
          resizable
          @update:view="logView = $event"
        >
          <template #cell-prize_name="{ row }">
            <span class="prize-cell"
              ><BallSwatch :color="row.ball_color" size="sm" /> {{ row.prize_name }}</span
            >
          </template>
          <template #cell-drawn_at="{ row }">
            <span class="text-sm text-muted">{{ created(row.drawn_at) }}</span>
          </template>
          <template #empty>
            <EmptyState text="No draws match your search." />
          </template>
        </DataTable>
        <PaginationBar
          v-if="logView.totalPages > 1"
          class="mt-12"
          :page="logPage"
          :total-pages="logView.totalPages"
          @go="(p: number) => (logPage = p)"
        />
      </template>
      <EmptyState v-else text="No draws yet." />
    </AdminPanel>

    <!-- -- List ---------------------------------------------------------------- -->
    <ManagerView v-else title="Garapon" :icon="['fad', 'ferris-wheel']">
      <template #actions>
        <button class="btn-confirm btn-sm" @click="openNew">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Garapon
        </button>
      </template>

      <LoadingSpinner
        v-if="garapons.garaponsLoading && garapons.garapons.length === 0"
        block
        label="Loading garapons..."
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
            role="button"
            tabindex="0"
            @click="openGarapon(g)"
            @keydown.enter="openGarapon(g)"
            @keydown.space.prevent="openGarapon(g)"
          >
            <img
              v-if="g.grand_prize_image"
              :src="assetUrl(g.grand_prize_image)"
              class="media-card-image"
              alt="Grand prize"
            />
            <div class="media-card-body">
              <h3>{{ g.title }}</h3>
              <p class="text-muted text-sm">
                {{ g.player_count || 0 }} drawing{{ g.player_count === 1 ? '' : 's' }} -
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
          <DataTableToolbar
            :count="closedView.total"
            :total="garapons.closedGarapons.length"
            noun="closed garapon"
          >
            <template #search>
              <SearchInput
                v-model="closedSearch"
                placeholder="Search closed garapons..."
                aria-label="Search closed garapons"
              />
            </template>
          </DataTableToolbar>
          <DataTable
            v-model:page="closedPage"
            :columns="closedColumns"
            :rows="garapons.closedGarapons"
            row-key="id"
            :filter="closedSearch"
            :filter-fn="closedMatches"
            :page-size="10"
            @update:view="closedView = $event"
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
            v-if="closedView.totalPages > 1"
            class="mt-12"
            :page="closedPage"
            :total-pages="closedView.totalPages"
            @go="(p: number) => (closedPage = p)"
          />
        </template>
        <EmptyState v-else text="No closed garapons yet." />
      </template>
    </ManagerView>
  </div>
</template>

<style scoped>
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
