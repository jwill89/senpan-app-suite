<script setup lang="ts">
/**
 * Admin Tea Rooms manager (Senpan Tea House → Tea Rooms). Three screens:
 *
 *   - list: a drag-orderable list of rooms (image + name + cost + status badges)
 *     with per-row actions — post to Discord, toggle open/closed, toggle the
 *     discount, edit, delete — plus a search box. Reordering persists like the
 *     Announcements list.
 *   - form: the create/edit form (TeaRoomFormTab), a Back sub-page.
 *   - webhook: set the single shared Discord webhook every room posts to.
 *
 * All state + actions come from the tea-rooms store.
 */
import { computed, ref } from 'vue'
import { VueDraggable } from 'vue-draggable-plus'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import ListRow from '@/components/common/ui/ListRow.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import TeaRoomFormTab from './TeaRoomFormTab.vue'
import { useTeaRoomsStore } from '@/stores/teaRooms'
import { useUiStore } from '@/stores/ui'
import { assetUrl } from '@/lib/assets'
import type { TeaRoom } from '@/types/api'

const store = useTeaRoomsStore()
const ui = useUiStore()

/**
 * The public (cross-origin) rooms API URL an external site fetches — the all-rooms
 * list, or one room when a room number is given (the public lookup key). Built from
 * the current origin so it points at whichever host serves the admin
 * (apps.senpan.cafe in production).
 */
function publicApiUrl(roomNumber?: string): string {
  const base = `${window.location.origin}/api/tea-rooms/public`
  return roomNumber ? `${base}/${encodeURIComponent(roomNumber)}` : base
}
function copyApiLink(roomNumber?: string): void {
  ui.copyToClipboard(
    publicApiUrl(roomNumber),
    roomNumber ? "Room's API link copied" : 'All-rooms API link copied',
  )
}

type Screen = 'list' | 'form' | 'webhook'
const screen = ref<Screen>('list')

// Drag-reorder is only allowed when the full list is shown (an unfiltered search):
// reordering a filtered subset would be ambiguous to persist. Mirrors Announcements.
const canReorder = computed(() => !store.search.trim())
const visibleIds = computed(() => new Set(store.filteredTeaRooms.map((t) => t.id)))
function onReorder(): void {
  void store.reorder(store.teaRooms.map((t) => t.id))
}

/** Cost line for a room, halved (50% off) with a note when discounted. */
function costText(t: TeaRoom): string {
  if (t.discounted) {
    return `${Math.floor(t.cost_per_half_hour / 2).toLocaleString()} gil/half hour (50% off)`
  }
  return `${t.cost_per_half_hour.toLocaleString()} gil/half hour`
}

// ── Navigation ───────────────────────────────────────────────────────────────
function openNew(): void {
  store.newTeaRoomForm()
  screen.value = 'form'
}
function openEdit(t: TeaRoom): void {
  store.editTeaRoom(t)
  screen.value = 'form'
}
function onFormDone(): void {
  screen.value = 'list'
}

// ── Webhook sub-page ─────────────────────────────────────────────────────────
const webhookDraft = ref('')
function openWebhook(): void {
  webhookDraft.value = store.webhookUrl
  screen.value = 'webhook'
}
async function submitWebhook(): Promise<void> {
  if (await store.saveWebhook(webhookDraft.value)) screen.value = 'list'
}
</script>

<template>
  <div class="tab-body">
    <!-- ── Form ──────────────────────────────────────────────────────────────── -->
    <TeaRoomFormTab v-if="screen === 'form'" @saved="onFormDone" @cancel="onFormDone" />

    <!-- ── Webhook ───────────────────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'webhook'">
      <SubPageHeader
        :icon="['fab', 'discord']"
        title="Tea Rooms Discord Webhook"
        @back="screen = 'list'"
      />
      <p class="text-dim text-sm mb-16">
        Every tea room posts to this one channel webhook. In Discord: Channel Settings →
        Integrations → Webhooks → New Webhook → Copy Webhook URL.
      </p>
      <FormField
        label="Discord webhook URL"
        help="Leave blank to disable posting. Only Discord webhook URLs are accepted."
      >
        <input
          v-model="webhookDraft"
          placeholder="https://discord.com/api/webhooks/…"
          aria-label="Discord webhook URL"
        />
      </FormField>
      <FormActions align="start">
        <button class="btn-neutral" :disabled="store.savingWebhook" @click="screen = 'list'">
          Cancel
        </button>
        <button class="btn-confirm" :disabled="store.savingWebhook" @click="submitWebhook">
          <LoadingSpinner v-if="store.savingWebhook" label="Saving…" />
          <template v-else>Save Webhook</template>
        </button>
      </FormActions>
    </AdminPanel>

    <!-- ── List ──────────────────────────────────────────────────────────────── -->
    <ManagerView v-else title="Tea Rooms" :icon="['fad', 'booth-curtain']">
      <template #actions>
        <button
          class="btn-view btn-sm"
          title="Copy the public API link for all rooms (for the Carrd site)"
          @click="copyApiLink()"
        >
          <font-awesome-icon :icon="['fas', 'link']" /> Copy API Link
        </button>
        <button class="btn-view btn-sm" @click="openWebhook">
          <font-awesome-icon :icon="['fab', 'discord']" /> Webhook
        </button>
        <button class="btn-confirm btn-sm" @click="openNew">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Tea Room
        </button>
      </template>

      <template #toolbar>
        <SearchInput
          v-model="store.search"
          placeholder="Search tea rooms…"
          aria-label="Search tea rooms"
        />
      </template>

      <div v-if="!store.webhookUrl && store.teaRooms.length" class="text-dim text-xs mb-12">
        <font-awesome-icon :icon="['fad', 'triangle-exclamation']" /> No Discord webhook set yet —
        add one with the <button class="link-btn" @click="openWebhook">Webhook</button> button to
        post rooms.
      </div>

      <LoadingSpinner
        v-if="store.loading && store.teaRooms.length === 0"
        block
        label="Loading tea rooms…"
      />
      <template v-else>
        <p v-if="store.teaRooms.length > 1" class="text-dim text-xs mb-12">
          <template v-if="canReorder">
            <font-awesome-icon :icon="['fad', 'bars']" /> Drag a row by its handle to reorder the
            list. The order is saved automatically.
          </template>
          <template v-else>Clear the search to drag-and-drop reorder the list.</template>
        </p>

        <VueDraggable
          v-if="store.teaRooms.length"
          v-show="store.filteredTeaRooms.length"
          v-model="store.teaRooms"
          class="list-rows"
          handle=".room-drag"
          :animation="150"
          ghost-class="dragging"
          :disabled="!canReorder"
          @end="onReorder"
        >
          <ListRow v-for="t in store.teaRooms" v-show="visibleIds.has(t.id)" :key="t.id">
            <template #media>
              <span
                v-if="canReorder"
                class="room-drag drag-handle"
                title="Drag to reorder"
                aria-label="Drag to reorder"
              >
                <font-awesome-icon :icon="['fad', 'bars']" />
              </span>
              <span
                class="room-swatch"
                :style="{ background: t.color || '#ff3131' }"
                aria-hidden="true"
              ></span>
              <img
                v-if="t.image"
                :src="assetUrl(t.image)"
                class="media-cover media-cover--wide"
                alt="Tea room image"
              />
              <div v-else class="media-cover media-cover--wide media-empty">
                <font-awesome-icon :icon="['fad', 'booth-curtain']" />
              </div>
            </template>

            <h4 class="room-title">
              {{ t.name }}
              <span v-if="t.room_number" class="text-dim fw-normal"
                >· Room {{ t.room_number }}</span
              >
            </h4>
            <p v-if="t.subtitle" class="text-sm text-dim room-meta">{{ t.subtitle }}</p>
            <p class="text-sm room-meta">
              <font-awesome-icon :icon="['fad', 'coins']" /> {{ costText(t) }}
            </p>
            <p class="room-meta">
              <span class="badge" :class="t.open ? 'badge--success' : 'badge--muted'">
                {{ t.open ? 'Open' : 'Closed' }}
              </span>
              <span class="badge" :class="t.seasonal ? 'badge--accent' : 'badge--muted'">
                {{ t.seasonal ? 'Seasonal' : 'Permanent' }}
              </span>
              <span v-if="t.discounted" class="badge badge--warning">50% off</span>
              <span v-if="t.lockable" class="badge badge--muted">Lockable</span>
            </p>
            <p v-if="t.hashtags" class="text-sm text-dim room-meta">{{ t.hashtags }}</p>

            <template #actions>
              <button
                class="btn-action btn-sm"
                :disabled="store.postingId === t.id"
                title="Post to Discord now"
                @click="store.postRoom(t)"
              >
                <LoadingSpinner v-if="store.postingId === t.id" label="Posting…" />
                <template v-else
                  ><font-awesome-icon :icon="['fas', 'paper-plane']" /> Post</template
                >
              </button>
              <button
                class="btn-view btn-sm"
                :disabled="store.togglingId === t.id"
                :title="t.open ? 'Mark closed' : 'Mark open'"
                @click="store.toggleOpen(t)"
              >
                <font-awesome-icon :icon="['fas', t.open ? 'circle-xmark' : 'circle-check']" />
                {{ t.open ? 'Close' : 'Open' }}
              </button>
              <button
                class="btn-caution btn-sm"
                :disabled="store.togglingId === t.id"
                :title="t.discounted ? 'Remove discount' : 'Mark discounted (50% off)'"
                @click="store.toggleDiscounted(t)"
              >
                <font-awesome-icon :icon="['fas', 'tag']" />
                {{ t.discounted ? 'Undiscount' : 'Discount' }}
              </button>
              <button
                class="btn-view btn-sm"
                aria-label="Copy this room's API link"
                title="Copy this room's API link"
                @click="copyApiLink(t.room_number)"
              >
                <font-awesome-icon :icon="['fas', 'link']" />
              </button>
              <button
                class="btn-confirm btn-sm"
                aria-label="Edit"
                title="Edit"
                @click="openEdit(t)"
              >
                <font-awesome-icon :icon="['fas', 'pen-to-square']" />
              </button>
              <button
                class="btn-danger btn-sm"
                aria-label="Delete"
                title="Delete"
                @click="store.deleteTeaRoom(t)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" />
              </button>
            </template>
          </ListRow>
        </VueDraggable>
        <EmptyState
          v-if="store.teaRooms.length && !store.filteredTeaRooms.length"
          text="No tea rooms match your search."
        />
        <EmptyState
          v-else-if="!store.teaRooms.length"
          text="No tea rooms yet. Create one with “New Tea Room”."
        />
      </template>
    </ManagerView>
  </div>
</template>

<style scoped>
.link-btn {
  background: none;
  border: none;
  color: var(--accent);
  cursor: pointer;
  text-decoration: underline;
  padding: 0;
  font: inherit;
}
/* Accent swatch mirroring the room's embed colour (like the announcement swatch). */
.room-swatch {
  width: 6px;
  align-self: stretch;
  border-radius: 3px;
  flex: 0 0 auto;
}
/* Drag handle for reordering; grab cursor + muted until hovered. */
.room-drag {
  display: inline-flex;
  align-items: center;
  align-self: stretch;
  padding: 0 2px;
  color: var(--text-muted);
  cursor: grab;
}
.room-drag:hover {
  color: var(--highlight);
}
.room-drag:active {
  cursor: grabbing;
}
/* vue-draggable-plus ghost while dragging a row. */
.dragging {
  opacity: 0.5;
}
.room-title {
  margin: 0 0 4px;
}
.room-meta {
  margin: 0 0 4px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
</style>
