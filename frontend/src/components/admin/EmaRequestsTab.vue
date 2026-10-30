<script setup lang="ts">
/**
 * Admin Festival -> Ema Requests: the artist's queue. Every plaque a visitor asked
 * the artist to make, across festivals, with what they asked for. The artist makes
 * it, posts it to Discord themselves, and marks it complete here.
 *
 * Its own page and permission (festival-ema-requests), so the artist can work the
 * queue without seeing the boards' webhooks.
 */
import { computed, onMounted, ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import { useEmaStore } from '@/stores/ema'
import { formatServerTimestamp } from '@/lib/datetime'
import { participantLabel } from '@/lib/participant'

const ema = useEmaStore()

onMounted(() => ema.loadRequests())

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
      Plaques visitors asked you to make. Make it, post it to the festival's Discord yourself, then
      mark it complete so it leaves the queue.
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
          <button
            v-if="row.status === 'requested'"
            type="button"
            class="btn-confirm btn-sm"
            @click="ema.setRequestComplete(row, true)"
          >
            <font-awesome-icon :icon="['fas', 'check']" /> Complete
          </button>
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
  </ManagerView>
</template>
