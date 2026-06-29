<script setup lang="ts">
/**
 * Admin Stamp Rally manager (Festival → Stamp Rally). Screens:
 *
 *   - list:   searchable grid of event cards (card image + title + counts).
 *   - detail: the selected event — a read-only card preview, the stamps (with
 *             pause/resume + passwords) and prizes, and links to the sub-pages.
 *   - cards:  issue participant card links + the issued-card table (copy/delete).
 *   - logs:   the event-wide "View Logs" stamp collection table — sortable, with a
 *             participant's rows always grouped together. Live-refreshed over the WS.
 *   - form:   the create/edit form (StampRallyFormTab), a Back sub-page.
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
import StampCardCanvas, { type CanvasItem } from '@/components/common/ui/StampCardCanvas.vue'
import StampRallyFormTab from './StampRallyFormTab.vue'
import { useStampRalliesStore, groupedByParticipant } from '@/stores/stampRallies'
import { useDataTableView } from '@/composables/useDataTableView'
import { assetUrl } from '@/lib/assets'
import { formatServerTimestamp } from '@/lib/datetime'
import { stallName } from '@/lib/stampcard'
import type { StampRally, StampRallyLogEntry } from '@/types/api'

const store = useStampRalliesStore()

type Screen = 'list' | 'detail' | 'cards' | 'logs' | 'form'
const screen = ref<Screen>('list')

// ── List: open card grid (searchable) + closed table ─────────────────────────
const search = ref('')
const filteredOpen = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return store.openRallies
  return store.openRallies.filter((r) => r.title.toLowerCase().includes(q))
})

const closedColumns: DataColumn[] = [
  { key: 'title', label: 'Title', sortable: true },
  { key: 'card_count', label: 'Cards', align: 'right', sortable: true },
  { key: 'completed_count', label: 'Complete', align: 'right', sortable: true },
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
} = useDataTableView<StampRally>(() => store.closedRallies, {
  matches: (r, q) => r.title.toLowerCase().includes(q),
})

const isClosed = computed(() => store.selectedRally?.status === 'closed')
function toggleClosed(): void {
  if (!store.selectedRally) return
  store.setRallyStatus(store.selectedRally.id, isClosed.value ? 'open' : 'closed')
}

// Inline "Manage stalls" panel on the open list cards (one expanded at a time).
const expandedCard = ref<number | null>(null)
function toggleManage(r: StampRally): void {
  if (expandedCard.value === r.id) {
    expandedCard.value = null
  } else {
    expandedCard.value = r.id
    store.loadCardStamps(r.id)
  }
}

// ── Detail preview (card + stamps + prizes at their placements) ──────────────
const previewItems = computed<CanvasItem[]>(() => {
  const r = store.selectedRally
  if (!r) return []
  const stamps = (r.stamps || []).map((s) => ({
    key: `s${s.id}`,
    image: s.image || r.not_stamped_image,
    placement: s.placement,
  }))
  const prizes = (r.prizes || []).map((p) => ({
    key: `p${p.id}`,
    image: p.image || r.not_stamped_image,
    placement: p.placement,
  }))
  return [...stamps, ...prizes]
})

// ── View Logs table (sortable, grouped by participant) ───────────────────────
const logColumns: DataColumn[] = [
  { key: 'participant_name', label: 'Participant', sortable: true },
  { key: 'stall_name', label: 'Stall / Vendor', sortable: true },
  { key: 'stamped_at', label: 'When', sortable: true, align: 'right' },
]
const {
  search: logSearch,
  filtered: logFiltered,
  sortKey: logSortKey,
  sortDir: logSortDir,
  setSort: setLogSort,
  reset: resetLogs,
} = useDataTableView<StampRallyLogEntry>(() => store.rallyLogs, {
  matches: (e, q) =>
    e.participant_name.toLowerCase().includes(q) || e.stall_name.toLowerCase().includes(q),
  sort: { key: 'participant_name', dir: 'asc' },
})
// Keep each participant's rows contiguous regardless of the active column sort.
const groupedLogs = computed(() => groupedByParticipant(logFiltered.value))

function when(ts: string): string {
  return ts ? formatServerTimestamp(ts) : '—'
}

// ── Navigation ───────────────────────────────────────────────────────────────
function openNew(): void {
  store.newRallyForm()
  screen.value = 'form'
}
function openRally(r: StampRally): void {
  store.viewRally(r)
  screen.value = 'detail'
}
function openCards(): void {
  screen.value = 'cards'
}
function openLogs(): void {
  if (store.selectedRally) store.loadRallyLogs(store.selectedRally.id)
  resetLogs()
  screen.value = 'logs'
}
function backToDetail(): void {
  screen.value = 'detail'
}
function editSelected(): void {
  if (!store.selectedRally) return
  store.editRallyForm(store.selectedRally)
  screen.value = 'form'
}
function backToList(): void {
  store.selectedRally = null
  screen.value = 'list'
}
function onFormDone(): void {
  screen.value = 'list'
}
async function deleteSelected(): Promise<void> {
  if (!store.selectedRally) return
  await store.deleteRally(store.selectedRally.id)
  if (!store.selectedRally) screen.value = 'list'
}
</script>

<template>
  <div class="tab-body">
    <!-- ── Form ──────────────────────────────────────────────────────────────── -->
    <StampRallyFormTab v-if="screen === 'form'" @saved="onFormDone" @cancel="onFormDone" />

    <!-- ── Detail ────────────────────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'detail' && store.selectedRally">
      <SubPageHeader @back="backToList">
        {{ store.selectedRally.title }}
        <span :class="['status-badge', 'status-badge-' + store.selectedRally.status]">
          {{ store.selectedRally.status }}
        </span>
      </SubPageHeader>
      <div class="flex-toolbar flex-end mb-16">
        <button class="btn-neutral btn-sm" @click="openCards">
          <font-awesome-icon :icon="['fad', 'id-card']" /> Cards ({{ store.rallyCards.length }})
        </button>
        <button class="btn-neutral btn-sm" @click="openLogs">
          <font-awesome-icon :icon="['fad', 'clipboard-list']" /> View Logs
        </button>
        <button v-if="!isClosed" class="btn-confirm btn-sm" @click="editSelected">
          <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Edit
        </button>
        <button class="btn-caution btn-sm" @click="toggleClosed">
          <font-awesome-icon :icon="['fas', isClosed ? 'rotate' : 'lock']" />
          {{ isClosed ? 'Reopen' : 'Close' }}
        </button>
        <button class="btn-danger btn-sm" @click="deleteSelected">
          <font-awesome-icon :icon="['fas', 'trash']" /> Delete
        </button>
      </div>

      <LoadingSpinner v-if="store.detailLoading && !store.selectedRally.stamps" block label="Loading…" />
      <template v-else>
        <!-- Read-only card preview -->
        <div class="rally-preview mb-16">
          <StampCardCanvas :card-image="store.selectedRally.card_image" :items="previewItems" />
        </div>

        <!-- Stamps -->
        <h3 class="section-heading"><font-awesome-icon :icon="['fad', 'stamp']" /> Stamps</h3>
        <div v-if="store.selectedRally.stamps && store.selectedRally.stamps.length" class="rally-table-wrap mb-16">
          <table class="data-table">
            <thead>
              <tr>
                <th>Stall</th>
                <th>Password</th>
                <th class="ta-center">Status</th>
                <th class="ta-right"></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in store.selectedRally.stamps" :key="s.id">
                <td>{{ stallName(s.affiliate_name) }}</td>
                <td><code>{{ s.password || '—' }}</code></td>
                <td class="ta-center">
                  <span v-if="s.paused" class="status-badge status-badge-closed">paused</span>
                  <span v-else class="status-badge status-badge-open">active</span>
                </td>
                <td class="ta-right">
                  <button class="btn-caution btn-sm" @click="store.setStampPaused(s.id, !s.paused)">
                    <font-awesome-icon :icon="['fas', s.paused ? 'rotate' : 'lock']" />
                    {{ s.paused ? 'Resume' : 'Pause' }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <EmptyState v-else text="No stamps on this card yet." />

        <!-- Prizes -->
        <h3 class="section-heading"><font-awesome-icon :icon="['fad', 'gift']" /> Prizes</h3>
        <div v-if="store.selectedRally.prizes && store.selectedRally.prizes.length" class="prize-chips mb-8">
          <span v-for="p in store.selectedRally.prizes" :key="p.id" class="prize-chip">
            <img v-if="p.image" :src="assetUrl(p.image)" alt="" />
            {{ p.name || 'Prize' }}
          </span>
        </div>
        <EmptyState v-else text="No prizes on this card yet." />
      </template>
    </AdminPanel>

    <!-- ── Cards sub-page ────────────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'cards' && store.selectedRally">
      <SubPageHeader @back="backToDetail">
        <font-awesome-icon :icon="['fad', 'id-card']" /> Cards — {{ store.selectedRally.title }}
      </SubPageHeader>

      <div class="entry-add mb-16">
        <h3 class="section-heading"><font-awesome-icon :icon="['fad', 'id-card']" /> Issue Card</h3>
        <div class="flex-row mb-10">
          <FormField label="Participant Name" style="flex: 1; min-width: 160px">
            <input
              v-model="store.cardAdd.participantName"
              placeholder="Participant name"
              aria-label="Participant name"
              @keyup.enter="store.createCard()"
            />
          </FormField>
        </div>
        <div class="flex-toolbar flex-end">
          <button
            class="btn-confirm btn-sm"
            :disabled="store.creatingCard || !store.cardAdd.participantName.trim()"
            @click="store.createCard()"
          >
            <LoadingSpinner v-if="store.creatingCard" label="Creating…" />
            <template v-else><font-awesome-icon :icon="['fas', 'plus']" /> Create Card Link</template>
          </button>
        </div>
      </div>

      <template v-if="store.rallyCards.length">
        <table class="data-table">
          <thead>
            <tr>
              <th>Participant</th>
              <th class="ta-center">Stamps</th>
              <th class="ta-center">Status</th>
              <th class="ta-right"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in store.rallyCards" :key="c.id">
              <td>{{ c.participant_name }}</td>
              <td class="ta-center">{{ c.collected_count || 0 }}</td>
              <td class="ta-center">
                <span v-if="c.completed" class="status-badge status-badge-open">complete</span>
                <span v-else class="text-dim text-sm">in progress</span>
              </td>
              <td class="ta-right">
                <div class="row-actions">
                  <button class="btn-view btn-sm" title="Copy link" aria-label="Copy link" @click="store.copyCardLink(c)">
                    <font-awesome-icon :icon="['fas', 'link']" />
                  </button>
                  <button
                    class="btn-danger btn-sm"
                    :disabled="(c.collected_count || 0) > 0 && !isClosed"
                    :aria-label="
                      (c.collected_count || 0) > 0 && !isClosed
                        ? 'Cannot delete — card has stamps (rally is open)'
                        : 'Delete card'
                    "
                    :title="
                      (c.collected_count || 0) > 0 && !isClosed
                        ? 'This card has stamps — close the rally to delete it (its log is kept)'
                        : 'Delete card'
                    "
                    @click="store.deleteCard(c)"
                  >
                    <font-awesome-icon :icon="['fas', 'trash']" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </template>
      <EmptyState v-else text="No cards issued yet." />
    </AdminPanel>

    <!-- ── View Logs sub-page ────────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'logs' && store.selectedRally">
      <SubPageHeader @back="backToDetail">
        <font-awesome-icon :icon="['fad', 'clipboard-list']" /> Stamp Log — {{ store.selectedRally.title }}
      </SubPageHeader>

      <template v-if="store.rallyLogs.length">
        <div class="manager-toolbar">
          <SearchInput v-model="logSearch" placeholder="Search by participant or stall…" aria-label="Search logs" />
          <span class="text-dim text-xs push-right">{{ groupedLogs.length }} stamps</span>
        </div>
        <DataTable
          :columns="logColumns"
          :rows="groupedLogs"
          :row-key="(row: StampRallyLogEntry) => `${row.card_id}-${row.stamp_id}`"
          :sort-key="logSortKey"
          :sort-dir="logSortDir"
          @sort="setLogSort"
        >
          <template #cell-stamped_at="{ row }">
            <span class="text-sm text-dim">{{ when((row as StampRallyLogEntry).stamped_at) }}</span>
          </template>
          <template #empty><EmptyState text="No stamps match your search." /></template>
        </DataTable>
      </template>
      <EmptyState v-else text="No stamps collected yet." />
    </AdminPanel>

    <!-- ── List ──────────────────────────────────────────────────────────────── -->
    <ManagerView v-else title="Stamp Rally" :icon="['fad', 'stamp']">
      <template #actions>
        <button class="btn-confirm btn-sm" @click="openNew">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Stamp Rally
        </button>
      </template>

      <LoadingSpinner
        v-if="store.ralliesLoading && store.rallies.length === 0"
        block
        label="Loading stamp rallies…"
      />
      <template v-else>
        <!-- Open rallies -->
        <h4 class="section-heading"><font-awesome-icon :icon="['fad', 'stamp']" /> Open Stamp Rallies</h4>
        <template v-if="store.openRallies.length">
          <div class="manager-toolbar">
            <SearchInput v-model="search" placeholder="Search open stamp rallies…" aria-label="Search stamp rallies" />
            <span class="text-dim text-xs push-right">{{ filteredOpen.length }} open</span>
          </div>
          <div v-if="filteredOpen.length" class="card-grid">
            <div v-for="r in filteredOpen" :key="r.id" class="media-card" @click="openRally(r)">
              <img v-if="r.card_image" :src="assetUrl(r.card_image)" class="media-card-image" alt="Stamp card" />
              <div class="media-card-body">
                <h3>{{ r.title }}</h3>
                <p class="text-dim text-sm">
                  {{ r.card_count || 0 }} card{{ r.card_count === 1 ? '' : 's' }} ·
                  {{ r.completed_count || 0 }} complete
                </p>
                <p v-if="(r.stamp_count || 0) > 0" class="text-dim text-sm">
                  <font-awesome-icon :icon="['fad', 'stamp']" />
                  <strong :class="{ 'has-paused': (r.active_stamp_count ?? 0) < (r.stamp_count || 0) }">
                    {{ r.active_stamp_count ?? 0 }}/{{ r.stamp_count }}
                  </strong>
                  stall{{ r.stamp_count === 1 ? '' : 's' }} active
                </p>
                <div class="rally-card-actions">
                  <button
                    v-if="(r.stamp_count || 0) > 0"
                    class="btn-neutral btn-sm"
                    :aria-expanded="expandedCard === r.id"
                    aria-label="Manage stalls"
                    title="Pause or resume individual stalls"
                    @click.stop="toggleManage(r)"
                  >
                    <font-awesome-icon :icon="['fad', 'sliders']" /> Manage stalls
                    <span class="text-xs">{{ expandedCard === r.id ? '▾' : '▸' }}</span>
                  </button>
                  <button
                    class="btn-danger btn-sm"
                    aria-label="Delete stamp rally"
                    title="Delete"
                    @click.stop="store.deleteRally(r.id)"
                  >
                    <font-awesome-icon :icon="['fas', 'trash']" /> Delete
                  </button>
                </div>

                <!-- Inline stall pause panel (no full edit needed). -->
                <div v-if="expandedCard === r.id" class="stall-panel" @click.stop>
                  <div v-if="!store.cardStamps[r.id]" class="text-dim text-sm">Loading stalls…</div>
                  <template v-else-if="store.cardStamps[r.id].length">
                    <div v-for="s in store.cardStamps[r.id]" :key="s.id" class="stall-row">
                      <span class="stall-row-name">{{ stallName(s.affiliate_name) }}</span>
                      <span :class="['status-badge', s.paused ? 'status-badge-closed' : 'status-badge-open']">
                        {{ s.paused ? 'paused' : 'active' }}
                      </span>
                      <button
                        class="btn-caution btn-sm"
                        @click.stop="store.setStampPausedInList(r.id, s.id, !s.paused)"
                      >
                        <font-awesome-icon :icon="['fas', s.paused ? 'rotate' : 'lock']" />
                        {{ s.paused ? 'Resume' : 'Pause' }}
                      </button>
                    </div>
                  </template>
                  <div v-else class="text-dim text-sm">No stalls on this card.</div>
                </div>
              </div>
            </div>
          </div>
          <EmptyState v-else text="No open stamp rallies match your search." />
        </template>
        <EmptyState v-else text="No open stamp rallies." />

        <!-- Closed rallies -->
        <h4 class="section-heading mt-20"><font-awesome-icon :icon="['fad', 'lock']" /> Closed Stamp Rallies</h4>
        <template v-if="store.closedRallies.length">
          <div class="manager-toolbar">
            <SearchInput v-model="closedSearch" placeholder="Search closed stamp rallies…" aria-label="Search closed stamp rallies" />
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
            <template #cell-card_count="{ row }">{{ (row as StampRally).card_count || 0 }}</template>
            <template #cell-completed_count="{ row }">{{ (row as StampRally).completed_count || 0 }}</template>
            <template #cell-created_at="{ row }">
              <span class="text-sm">{{ when((row as StampRally).created_at) }}</span>
            </template>
            <template #cell-actions="{ row }">
              <div class="row-actions">
                <button class="btn-view btn-sm" aria-label="View" title="View" @click="openRally(row as StampRally)">
                  <font-awesome-icon :icon="['fas', 'eye']" />
                </button>
                <button class="btn-danger btn-sm" aria-label="Delete" title="Delete" @click="store.deleteRally((row as StampRally).id)">
                  <font-awesome-icon :icon="['fas', 'trash']" />
                </button>
              </div>
            </template>
            <template #empty><EmptyState text="No closed stamp rallies match your search." /></template>
          </DataTable>
          <PaginationBar
            v-if="closedTotalPages > 1"
            class="mt-12"
            :page="closedPage"
            :total-pages="closedTotalPages"
            @go="(p: number) => (closedPage = p)"
          />
        </template>
        <EmptyState v-else text="No closed stamp rallies yet." />
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
.rally-table-wrap {
  overflow-x: auto;
}
.rally-preview {
  max-width: 520px;
}
.rally-card-actions {
  margin-top: 10px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
}
/* "X/Y active" turns to the caution colour when any stall is paused. */
.has-paused {
  color: var(--caution, #d98324);
}
/* Inline stall pause panel on a list card. */
.stall-panel {
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px solid var(--control-border);
  display: flex;
  flex-direction: column;
  gap: 6px;
  cursor: default;
}
.stall-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.stall-row-name {
  flex: 1;
  min-width: 0;
  font-size: 0.9rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.prize-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.prize-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--panel-raised-bg);
  border: 1px solid var(--control-border);
  border-radius: 999px;
  padding: 4px 12px 4px 4px;
  font-size: 0.9rem;
}
.prize-chip img {
  width: 28px;
  height: 28px;
  object-fit: cover;
  border-radius: 50%;
}
</style>
