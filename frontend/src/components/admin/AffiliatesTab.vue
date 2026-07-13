<script setup lang="ts">
/**
 * Admin Affiliates manager (Senpan Tea House → Affiliates). Three screens:
 *
 *   - list: a drag-orderable list of affiliates (logo/screenshot + name + owners
 *     + location) with per-row actions — post to Discord, edit, delete — plus a
 *     search box. Reordering persists like the Tea Rooms / Announcements lists.
 *   - form: the create/edit form (AffiliateFormTab), a Back sub-page.
 *   - webhook: set the single shared Discord webhook every affiliate posts to.
 *
 * All state + actions come from the affiliates store.
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
import AffiliateFormTab from './AffiliateFormTab.vue'
import { useAffiliatesStore } from '@/stores/affiliates'
import { assetUrl } from '@/lib/assets'
import type { Affiliate } from '@/types/api'

const store = useAffiliatesStore()

type Screen = 'list' | 'form' | 'webhook'
const screen = ref<Screen>('list')

// Drag-reorder is only allowed when the full list is shown (search cleared):
// reordering a filtered subset would be ambiguous to persist. Mirrors Tea Rooms.
const canReorder = computed(() => !store.search.trim())
const visibleIds = computed(() => new Set(store.filteredAffiliates.map((a) => a.id)))
function onReorder(): void {
  void store.reorder(store.affiliates.map((a) => a.id))
}

/** The image to show on a row: the logo if set, otherwise the establishment screenshot. */
function rowImage(a: Affiliate): string {
  return a.logo || a.screenshot
}

// ── Navigation ───────────────────────────────────────────────────────────────
function openNew(): void {
  store.newAffiliateForm()
  screen.value = 'form'
}
function openEdit(a: Affiliate): void {
  store.editAffiliateForm(a)
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
    <AffiliateFormTab v-if="screen === 'form'" @saved="onFormDone" @cancel="onFormDone" />

    <!-- ── Webhook ───────────────────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'webhook'">
      <SubPageHeader
        :icon="['fab', 'discord']"
        title="Affiliates Discord Webhook"
        @back="screen = 'list'"
      />
      <p class="text-dim text-sm mb-16">
        Every affiliate posts to this one channel webhook. In Discord: Channel Settings →
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
    <ManagerView v-else title="Affiliates" :icon="['fad', 'handshake']">
      <template #actions>
        <button class="btn-view btn-sm" @click="openWebhook">
          <font-awesome-icon :icon="['fab', 'discord']" /> Webhook
        </button>
        <button class="btn-confirm btn-sm" @click="openNew">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Affiliate
        </button>
      </template>

      <template #toolbar>
        <SearchInput
          v-model="store.search"
          placeholder="Search affiliates…"
          aria-label="Search affiliates"
        />
      </template>

      <div v-if="!store.webhookUrl && store.affiliates.length" class="text-dim text-xs mb-12">
        <font-awesome-icon :icon="['fad', 'triangle-exclamation']" /> No Discord webhook set yet —
        add one with the <button class="link-btn" @click="openWebhook">Webhook</button> button to
        post affiliates.
      </div>

      <LoadingSpinner
        v-if="store.affiliatesLoading && store.affiliates.length === 0"
        block
        label="Loading affiliates…"
      />
      <template v-else>
        <p v-if="store.affiliates.length > 1" class="text-dim text-xs mb-12">
          <template v-if="canReorder">
            <font-awesome-icon :icon="['fad', 'bars']" /> Drag a row by its handle to reorder the
            list. The order is saved automatically.
          </template>
          <template v-else>Clear the search to drag-and-drop reorder the list.</template>
        </p>

        <VueDraggable
          v-if="store.affiliates.length"
          v-show="store.filteredAffiliates.length"
          v-model="store.affiliates"
          class="list-rows"
          handle=".aff-drag"
          :animation="150"
          ghost-class="dragging"
          :disabled="!canReorder"
          @end="onReorder"
        >
          <ListRow v-for="a in store.affiliates" v-show="visibleIds.has(a.id)" :key="a.id">
            <template #media>
              <span
                v-if="canReorder"
                class="aff-drag drag-handle"
                title="Drag to reorder"
                aria-label="Drag to reorder"
              >
                <font-awesome-icon :icon="['fad', 'bars']" />
              </span>
              <span
                class="aff-swatch"
                :style="{ background: a.embed_color || '#ff3131' }"
                aria-hidden="true"
              ></span>
              <img
                v-if="rowImage(a)"
                :src="assetUrl(rowImage(a))"
                class="media-cover media-cover--wide"
                alt="Affiliate logo"
              />
              <div v-else class="media-cover media-cover--wide media-empty">
                <font-awesome-icon :icon="['fad', 'handshake']" />
              </div>
            </template>

            <h4 class="aff-title">{{ a.name }}</h4>
            <p v-if="a.owners.length" class="text-sm text-dim aff-meta">
              <font-awesome-icon :icon="['fad', 'user']" /> {{ a.owners.join(', ') }}
            </p>
            <p v-if="a.location" class="text-sm text-dim aff-meta">
              <font-awesome-icon :icon="['fad', 'location-dot']" /> {{ a.location }}
            </p>
            <p v-if="a.discord_link || a.carrd_link || a.hours.length" class="aff-meta">
              <span v-if="a.discord_link" class="badge badge--accent">Discord</span>
              <span v-if="a.carrd_link" class="badge badge--muted">Carrd</span>
              <span v-if="a.hours.length" class="badge badge--muted">
                {{ a.hours.length }} open time{{ a.hours.length === 1 ? '' : 's' }}
              </span>
            </p>

            <template #actions>
              <button
                class="btn-action btn-sm"
                :disabled="store.postingId === a.id"
                title="Post to Discord now"
                @click="store.postAffiliate(a)"
              >
                <LoadingSpinner v-if="store.postingId === a.id" label="Posting…" />
                <template v-else
                  ><font-awesome-icon :icon="['fas', 'paper-plane']" /> Post</template
                >
              </button>
              <button
                class="btn-confirm btn-sm"
                aria-label="Edit"
                title="Edit"
                @click="openEdit(a)"
              >
                <font-awesome-icon :icon="['fas', 'pen-to-square']" />
              </button>
              <button
                class="btn-danger btn-sm"
                aria-label="Delete"
                title="Delete"
                @click="store.deleteAffiliate(a.id)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" />
              </button>
            </template>
          </ListRow>
        </VueDraggable>
        <EmptyState
          v-if="store.affiliates.length && !store.filteredAffiliates.length"
          text="No affiliates match your search."
        />
        <EmptyState
          v-else-if="!store.affiliates.length"
          text="No affiliates yet. Create one with “New Affiliate”."
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
/* Accent swatch mirroring the affiliate's embed colour. */
.aff-swatch {
  width: 6px;
  align-self: stretch;
  border-radius: 3px;
  flex: 0 0 auto;
}
/* Drag handle for reordering; grab cursor + muted until hovered. */
.aff-drag {
  display: inline-flex;
  align-items: center;
  align-self: stretch;
  padding: 0 2px;
  color: var(--text-muted);
  cursor: grab;
}
.aff-drag:hover {
  color: var(--highlight);
}
.aff-drag:active {
  cursor: grabbing;
}
/* vue-draggable-plus ghost while dragging a row. */
.dragging {
  opacity: 0.5;
}
.aff-title {
  margin: 0 0 4px;
}
.aff-meta {
  margin: 0 0 4px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
</style>
