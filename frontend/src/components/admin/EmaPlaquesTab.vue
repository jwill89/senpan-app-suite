<script setup lang="ts">
/**
 * Admin Festival -> Ema Plaques: one plaque board per festival.
 *
 * The list shows every board with how many plaques wait for review. A board's page
 * is where staff work: the review queue of plaques designed on the site - each
 * approved one is posted to its day's Discord webhook and its stored image deleted,
 * each rejected one just deleted - and the board's whole plaque record. The board's
 * setup is a Back sub-page (EmaBoardFormTab).
 */
import { ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import ListRow from '@/components/common/ui/ListRow.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import EmaBoardFormTab from './EmaBoardFormTab.vue'
import { useEmaStore } from '@/stores/ema'
import { useUiStore } from '@/stores/ui'
import { assetUrl } from '@/lib/assets'
import { formatServerTimestamp } from '@/lib/datetime'
import { participantLabel } from '@/lib/participant'
import type { EmaBoard } from '@/types/api'

const ema = useEmaStore()
const ui = useUiStore()

type Screen = 'list' | 'detail' | 'form'
const screen = ref<Screen>('list')

/** Status wording + badge tint for the plaque record. */
const STATUS: Record<string, { text: string; badge: string }> = {
  pending: { text: 'Waiting for review', badge: 'badge--warning' },
  posting: { text: 'Posting...', badge: 'badge--warning' },
  posted: { text: 'Posted', badge: 'badge--success' },
  rejected: { text: 'Rejected', badge: 'badge--muted' },
  requested: { text: 'With the artist', badge: 'badge--accent' },
  complete: { text: 'Artist finished', badge: 'badge--success' },
}

const recordColumns: DataColumn[] = [
  { key: 'character_name', label: 'Character', sortable: true },
  { key: 'window_label', label: 'Day', sortable: true, facetable: true },
  { key: 'kind', label: 'Made', sortable: true, facetable: true },
  { key: 'status', label: 'Status', sortable: true, facetable: true },
  { key: 'reviewed_by', label: 'Reviewed By', sortable: true },
  { key: 'created_at', label: 'Sent', sortable: true, align: 'right' },
]

function when(ts: string): string {
  return ts ? formatServerTimestamp(ts) : ''
}

async function openBoard(b: EmaBoard): Promise<void> {
  screen.value = 'detail'
  await ema.openBoard(b.id)
}

function openNew(): void {
  ema.newBoardForm()
  screen.value = 'form'
}

function editBoard(): void {
  if (!ema.selectedBoard) return
  ema.editBoardForm(ema.selectedBoard)
  screen.value = 'form'
}

async function onSaved(id: number): Promise<void> {
  screen.value = 'detail'
  await ema.openBoard(id)
}

function onFormCancel(): void {
  screen.value = ema.selectedBoard ? 'detail' : 'list'
}

function backToList(): void {
  ema.selectedBoard = null
  screen.value = 'list'
}

async function deleteBoard(): Promise<void> {
  if (ema.selectedBoard && (await ema.deleteBoard(ema.selectedBoard))) screen.value = 'list'
}

/** The festival dashboard the board appears on, for posting in Discord. */
function copyDashboardLink(): void {
  if (!ema.selectedBoard) return
  ui.copyToClipboard(
    `${window.location.origin}/festival/${ema.selectedBoard.festival_map_id}`,
    'Dashboard link copied',
  )
}
</script>

<template>
  <div>
    <EmaBoardFormTab v-if="screen === 'form'" @saved="onSaved" @cancel="onFormCancel" />

    <!-- -- A board ------------------------------------------------------------- -->
    <AdminPanel v-else-if="screen === 'detail' && ema.selectedBoard">
      <SubPageHeader @back="backToList">
        <font-awesome-icon :icon="['fad', 'torii-gate']" />
        {{ ema.selectedBoard.title || 'Ema Plaques' }}
        <span class="text-muted text-sm">- {{ ema.selectedBoard.festival_map_name }}</span>
      </SubPageHeader>

      <div class="flex-toolbar mb-16">
        <span
          :class="[
            'status-badge',
            ema.selectedBoard.status === 'open' ? 'status-badge-open' : 'status-badge-closed',
          ]"
          >{{ ema.selectedBoard.status === 'open' ? 'taking plaques' : 'closed' }}</span
        >
        <button type="button" class="btn-confirm btn-sm" @click="editBoard">
          <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Edit
        </button>
        <button type="button" class="btn-view btn-sm" @click="copyDashboardLink">
          <font-awesome-icon :icon="['fas', 'link']" /> Copy Dashboard Link
        </button>
        <button
          type="button"
          class="btn-caution btn-sm"
          @click="
            ema.setBoardStatus(
              ema.selectedBoard,
              ema.selectedBoard.status === 'open' ? 'closed' : 'open',
            )
          "
        >
          {{ ema.selectedBoard.status === 'open' ? 'Close' : 'Reopen' }}
        </button>
        <button type="button" class="btn-danger btn-sm push-right" @click="deleteBoard">
          <font-awesome-icon :icon="['fas', 'trash']" /> Delete
        </button>
      </div>

      <!-- Review queue -->
      <h4 class="section-heading">
        <font-awesome-icon :icon="['fad', 'clipboard-list']" /> Waiting for Review
        <span class="text-muted text-sm">({{ ema.pendingPlaques.length }})</span>
      </h4>
      <p class="text-muted text-sm mb-12">
        Approving posts the plaque to its day's Discord webhook under the character's name, then
        deletes the image from the site. Rejecting deletes it, and that person can send another.
      </p>
      <LoadingSpinner
        v-if="ema.plaquesLoading && !ema.plaques.length"
        block
        label="Loading plaques..."
      />
      <div v-else-if="ema.pendingPlaques.length" class="ema-review-grid mb-20">
        <article
          v-for="p in ema.pendingPlaques"
          :key="p.id"
          class="card card--padded ema-review-card"
        >
          <img
            v-if="p.image"
            :src="assetUrl(p.image)"
            class="ema-review-image"
            :alt="`${p.character_name}'s plaque`"
          />
          <p class="m-0">
            <strong>{{ participantLabel(p.character_name, p.world) }}</strong>
          </p>
          <p class="text-muted text-sm m-0">{{ p.window_label }} - sent {{ when(p.created_at) }}</p>
          <div class="form-actions">
            <button
              type="button"
              class="btn-confirm btn-sm"
              :disabled="ema.reviewing.has(p.id) || p.status === 'posting'"
              @click="ema.review(p, 'approve')"
            >
              <LoadingSpinner v-if="ema.reviewing.has(p.id)" label="Posting..." />
              <template v-else><font-awesome-icon :icon="['fas', 'check']" /> Approve</template>
            </button>
            <button
              type="button"
              class="btn-danger btn-sm"
              :disabled="ema.reviewing.has(p.id) || p.status === 'posting'"
              @click="ema.review(p, 'reject')"
            >
              <font-awesome-icon :icon="['fas', 'circle-xmark']" /> Reject
            </button>
          </div>
        </article>
      </div>
      <EmptyState v-else text="Nothing waiting for review." />

      <!-- The record -->
      <h4 class="section-heading mt-20">
        <font-awesome-icon :icon="['fad', 'torii-gate']" /> Every Plaque
        <span class="text-muted text-sm">({{ ema.plaques.length }})</span>
      </h4>
      <DataTable
        v-if="ema.plaques.length"
        :columns="recordColumns"
        :rows="ema.plaques"
        row-key="id"
        :page-size="25"
        :default-sort="{ key: 'created_at', dir: 'desc' }"
      >
        <template #cell-character_name="{ row }">
          {{ participantLabel(row.character_name, row.world) }}
        </template>
        <template #cell-kind="{ row }">
          {{ row.kind === 'site' ? 'On the site' : 'By the artist' }}
        </template>
        <template #cell-status="{ row }">
          <span :class="['badge', STATUS[row.status]?.badge ?? 'badge--muted']">
            {{ STATUS[row.status]?.text ?? row.status }}
          </span>
        </template>
        <template #cell-created_at="{ row }">
          <span class="text-sm">{{ when(row.created_at) }}</span>
        </template>
      </DataTable>
      <EmptyState v-else text="No plaques yet." />
    </AdminPanel>

    <!-- -- The boards ---------------------------------------------------------- -->
    <ManagerView v-else title="Ema Plaques" :icon="['fad', 'torii-gate']">
      <template #actions>
        <button type="button" class="btn-confirm btn-sm" @click="openNew">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Board
        </button>
      </template>

      <LoadingSpinner
        v-if="ema.boardsLoading && !ema.boards.length"
        block
        label="Loading plaque boards..."
      />
      <div v-else-if="ema.boards.length" class="list-rows">
        <ListRow v-for="b in ema.boards" :key="b.id">
          <template #media>
            <img
              v-if="b.template_image"
              :src="assetUrl(b.template_image)"
              class="ema-board-thumb"
              alt=""
            />
          </template>
          <div class="list-row-title">{{ b.title || 'Ema Plaques' }}</div>
          <div class="list-row-meta">
            {{ b.festival_map_name }}
            <span v-if="b.status !== 'open'" class="badge badge--muted">closed</span>
            <span v-if="b.pending_count" class="badge badge--warning">
              {{ b.pending_count }} to review
            </span>
            <span v-if="b.requested_count" class="badge badge--accent">
              {{ b.requested_count }} with the artist
            </span>
          </div>
          <template #actions>
            <button type="button" class="btn-view btn-sm" @click="openBoard(b)">Open</button>
          </template>
        </ListRow>
      </div>
      <EmptyState
        v-else
        :icon="['fad', 'torii-gate']"
        text="No plaque boards yet."
        hint="Make one for a festival and its plaques show on that festival's dashboard."
      />
    </ManagerView>
  </div>
</template>
