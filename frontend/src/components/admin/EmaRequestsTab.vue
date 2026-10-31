<script setup lang="ts">
/**
 * Admin Festival -> Ema Requests: the artist's queue. Every plaque a visitor asked
 * the artist to make, across festivals, with what they asked for. The artist makes
 * it, then either uploads it here - the app posts it to the day's Discord channel
 * and completes the request (EmaRequestPostModal) - or posts it by hand and marks
 * it complete.
 *
 * Its own page and permission (festival-ema-requests), so the artist can work the
 * queue without seeing the boards' webhooks.
 */
import { computed, onMounted, ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import EmaRequestPostModal from '@/components/admin/EmaRequestPostModal.vue'
import { useEmaStore } from '@/stores/ema'
import { formatServerTimestamp } from '@/lib/datetime'
import { participantLabel } from '@/lib/participant'
import type { EmaPlaque } from '@/types/api'

const ema = useEmaStore()

onMounted(() => ema.loadRequests())

/** The request whose finished plaque is being posted. */
const posting = ref<EmaPlaque | null>(null)

/** Finished requests are hidden until asked for: the queue is what's left to do. */
const showFinished = ref(false)
const rows = computed(() =>
  showFinished.value ? ema.requests : ema.requests.filter((r) => r.status !== 'complete'),
)

const columns: DataColumn[] = [
  { key: 'festival_map_name', label: 'Festival', sortable: true, facetable: true },
  { key: 'window_label', label: 'Day', sortable: true },
  { key: 'character_name', label: 'Character', sortable: true },
  { key: 'description', label: 'What they asked for' },
  { key: 'created_at', label: 'Asked', sortable: true },
  { key: 'actions', label: '', align: 'right' },
]
</script>

<template>
  <ManagerView title="Ema Requests" :icon="['fad', 'palette']">
    <template #actions>
      <label class="checkbox-inline">
        <input v-model="showFinished" type="checkbox" />
        Show finished
      </label>
    </template>

    <p class="text-muted text-sm mb-12">
      Plaques visitors asked you to make. When one is done, post it here - it goes to that day's
      Discord channel with their name and yours, and leaves the queue. Posted it yourself? Mark it
      complete instead.
    </p>

    <LoadingSpinner
      v-if="ema.requestsLoading && !ema.requests.length"
      block
      label="Loading requests..."
    />
    <DataTable
      v-else-if="rows.length"
      :columns="columns"
      :rows="rows"
      row-key="id"
      :page-size="25"
      :default-sort="{ key: 'created_at', dir: 'asc' }"
      :row-class="(r) => (r.status === 'complete' ? 'ema-request-done' : '')"
    >
      <template #cell-character_name="{ row }">
        <strong>{{ participantLabel(row.character_name, row.world) }}</strong>
      </template>
      <template #cell-description="{ row }">
        <span class="ema-request-text">{{ row.description }}</span>
      </template>
      <template #cell-created_at="{ row }">
        <span class="text-sm">{{ formatServerTimestamp(row.created_at) }}</span>
      </template>
      <template #cell-actions="{ row }">
        <div class="row-actions">
          <template v-if="row.status === 'requested'">
            <button type="button" class="btn-action btn-sm" @click="posting = row">
              <font-awesome-icon :icon="['fab', 'discord']" /> Post Plaque
            </button>
            <button
              type="button"
              class="btn-confirm btn-sm"
              title="Mark it done without posting - for a plaque you posted yourself"
              @click="ema.setRequestComplete(row, true)"
            >
              <font-awesome-icon :icon="['fas', 'check']" /> Complete
            </button>
          </template>
          <span v-else-if="row.status === 'posting'" class="badge badge--warning">Posting</span>
          <button
            v-else
            type="button"
            class="btn-neutral btn-sm"
            title="Put it back in the queue"
            @click="ema.setRequestComplete(row, false)"
          >
            Reopen
          </button>
        </div>
      </template>
    </DataTable>
    <EmptyState
      v-else
      :icon="['fad', 'palette']"
      :text="showFinished ? 'No plaque requests yet.' : 'Nothing in the queue.'"
    />

    <EmaRequestPostModal v-if="posting" :request="posting" @close="posting = null" />
  </ManagerView>
</template>
