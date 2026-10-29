<script setup lang="ts">
/**
 * Admin Affiliates manager (Senpan Tea House -> Affiliates). Three screens:
 *
 *   - list: a drag-orderable list of affiliates (logo/screenshot + name + owners
 *     + location) with per-row actions - post to Discord, edit, delete - plus a
 *     search box. Reordering persists like the Tea Rooms / Announcements lists.
 *   - form: the create/edit form (AffiliateFormTab), a Back sub-page.
 *   - discord: where affiliates post in Discord (channel, ping, and whether posting
 *     again edits the affiliate's post in place).
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
import EmptyState from '@/components/common/ui/EmptyState.vue'
import DiscordDestinationForm from '@/components/common/ui/DiscordDestinationForm.vue'
import DiscordPostStatus from '@/components/common/ui/DiscordPostStatus.vue'
import { ITEM_REPOST_OPTIONS, postVerb } from '@/lib/discordPosts'
import AffiliateFormTab from './AffiliateFormTab.vue'
import { useAffiliatesStore } from '@/stores/affiliates'
import { assetUrl } from '@/lib/assets'
import type { Affiliate } from '@/types/api'

const store = useAffiliatesStore()

type Screen = 'list' | 'form' | 'discord'
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

// -- Navigation ---------------------------------------------------------------
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
</script>

<template>
  <div class="tab-body">
    <!-- -- Form ---------------------------------------------------------------- -->
    <AffiliateFormTab v-if="screen === 'form'" @saved="onFormDone" @cancel="onFormDone" />

    <!-- -- Discord -------------------------------------------------------------- -->
    <AdminPanel v-else-if="screen === 'discord'">
      <SubPageHeader
        :icon="['fab', 'discord']"
        title="Affiliates in Discord"
        @back="screen = 'list'"
      />
      <DiscordDestinationForm
        :destination="store.destination"
        :options="ITEM_REPOST_OPTIONS"
        :saving="store.savingDestination"
        id-prefix="aff-discord"
        @save="async (d) => (await store.saveDestination(d)) && (screen = 'list')"
        @cancel="screen = 'list'"
      />
    </AdminPanel>

    <!-- -- List ---------------------------------------------------------------- -->
    <ManagerView v-else title="Affiliates" :icon="['fad', 'handshake']">
      <template #actions>
        <button class="btn-view btn-sm" @click="screen = 'discord'">
          <font-awesome-icon :icon="['fab', 'discord']" /> Discord
        </button>
        <button class="btn-confirm btn-sm" @click="openNew">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Affiliate
        </button>
      </template>

      <template #toolbar>
        <SearchInput
          v-model="store.search"
          placeholder="Search affiliates..."
          aria-label="Search affiliates"
        />
      </template>

      <div
        v-if="!store.destination?.channel_id && store.affiliates.length"
        class="text-muted text-xs mb-12"
      >
        <font-awesome-icon :icon="['fad', 'triangle-exclamation']" /> No Discord channel picked yet
        - pick one with the <button class="link-btn" @click="screen = 'discord'">Discord</button>
        button to post affiliates.
      </div>

      <LoadingSpinner
        v-if="store.affiliatesLoading && store.affiliates.length === 0"
        block
        label="Loading affiliates..."
      />
      <template v-else>
        <p v-if="store.affiliates.length > 1" class="text-muted text-xs mb-12">
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
          handle=".drag-handle"
          :animation="150"
          ghost-class="is-dragging"
          :disabled="!canReorder"
          @end="onReorder"
        >
          <ListRow v-for="a in store.affiliates" v-show="visibleIds.has(a.id)" :key="a.id">
            <template #media>
              <span
                v-if="canReorder"
                class="drag-handle"
                title="Drag to reorder"
                aria-label="Drag to reorder"
              >
                <font-awesome-icon :icon="['fad', 'bars']" />
              </span>
              <span
                class="list-row-swatch"
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

            <h4 class="list-row-title">{{ a.name }}</h4>
            <p v-if="a.subtitle" class="text-sm text-muted list-row-meta list-row-meta--inline">
              {{ a.subtitle }}
            </p>
            <p
              v-if="a.owners.length"
              class="text-sm text-muted list-row-meta list-row-meta--inline"
            >
              <font-awesome-icon :icon="['fad', 'user']" /> {{ a.owners.join(', ') }}
            </p>
            <p v-if="a.location" class="text-sm text-muted list-row-meta list-row-meta--inline">
              <font-awesome-icon :icon="['fad', 'location-dot']" /> {{ a.location }}
            </p>
            <p
              v-if="a.discord_link || a.carrd_link || a.hours.length"
              class="list-row-meta list-row-meta--inline"
            >
              <span v-if="a.discord_link" class="badge badge--accent">Discord</span>
              <span v-if="a.carrd_link" class="badge badge--muted">Carrd</span>
              <span v-if="a.hours.length" class="badge badge--muted">
                {{ a.hours.length }} open time{{ a.hours.length === 1 ? '' : 's' }}
              </span>
            </p>
            <p v-if="a.discord_post" class="list-row-meta list-row-meta--inline">
              <DiscordPostStatus :post="a.discord_post" />
            </p>

            <template #actions>
              <button
                class="btn-action btn-sm"
                :disabled="store.postingId === a.id"
                :title="
                  postVerb(store.destination, a.discord_post) === 'Update'
                    ? 'Update the post in Discord'
                    : 'Post to Discord now'
                "
                @click="store.postAffiliate(a)"
              >
                <LoadingSpinner v-if="store.postingId === a.id" label="Posting..." />
                <template v-else
                  ><font-awesome-icon :icon="['fas', 'paper-plane']" />
                  {{ postVerb(store.destination, a.discord_post) }}</template
                >
              </button>
              <button
                v-if="a.discord_post?.state === 'live'"
                class="btn-caution btn-sm"
                :disabled="store.postingId === a.id"
                title="Delete the post from Discord"
                @click="store.takeDownAffiliate(a)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" /> Take down
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
/* vue-draggable-plus ghost while dragging a row. */
</style>
