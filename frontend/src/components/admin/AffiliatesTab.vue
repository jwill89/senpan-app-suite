<script setup lang="ts">
/**
 * Admin Affiliates manager (Senpan Tea House → Affiliates). Two screens:
 *
 *   - list: a searchable grid of affiliate cards (logo + name + owners/location
 *     summary) with per-card edit/delete, and a "New Affiliate" header action.
 *   - form: the create/edit form (AffiliateFormTab), a Back sub-page.
 *
 * All state + actions come from the affiliates store; the search filter is local
 * client-side state. The list is already alphabetical by name from the API.
 */
import { computed, ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import AffiliateFormTab from './AffiliateFormTab.vue'
import { useAffiliatesStore } from '@/stores/affiliates'
import { assetUrl } from '@/lib/assets'
import type { Affiliate } from '@/types/api'

const affiliates = useAffiliatesStore()

type Screen = 'list' | 'form'
const screen = ref<Screen>('list')

const search = ref('')
const filtered = computed<Affiliate[]>(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return affiliates.affiliates
  return affiliates.affiliates.filter(
    (a) =>
      a.name.toLowerCase().includes(q) ||
      a.location.toLowerCase().includes(q) ||
      a.owners.some((o) => o.toLowerCase().includes(q)),
  )
})

function openNew(): void {
  affiliates.newAffiliateForm()
  screen.value = 'form'
}
function editAffiliate(a: Affiliate): void {
  affiliates.editAffiliateForm(a)
  screen.value = 'form'
}
function onFormDone(): void {
  screen.value = 'list'
}
</script>

<template>
  <div class="tab-body">
    <!-- ── Form ──────────────────────────────────────────────────────────────── -->
    <AffiliateFormTab v-if="screen === 'form'" @saved="onFormDone" @cancel="onFormDone" />

    <!-- ── List ──────────────────────────────────────────────────────────────── -->
    <ManagerView v-else title="Affiliates" :icon="['fad', 'handshake']">
      <template #actions>
        <button class="btn-confirm btn-sm" @click="openNew">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Affiliate
        </button>
      </template>

      <LoadingSpinner
        v-if="affiliates.affiliatesLoading && affiliates.affiliates.length === 0"
        block
        label="Loading affiliates…"
      />
      <template v-else-if="affiliates.affiliates.length">
        <div class="manager-toolbar">
          <SearchInput
            v-model="search"
            placeholder="Search affiliates…"
            aria-label="Search affiliates"
          />
          <span class="text-dim text-xs push-right">
            {{ filtered.length }} affiliate{{ filtered.length === 1 ? '' : 's' }}
          </span>
        </div>

        <div v-if="filtered.length" class="card-grid">
          <div v-for="a in filtered" :key="a.id" class="media-card">
            <img
              v-if="a.logo"
              :src="assetUrl(a.logo)"
              class="media-card-image"
              alt="Affiliate logo"
            />
            <div class="media-card-body">
              <h3>{{ a.name }}</h3>
              <p v-if="a.owners.length" class="text-dim text-sm">
                <font-awesome-icon :icon="['fad', 'user']" /> {{ a.owners.join(', ') }}
              </p>
              <p v-if="a.location" class="text-dim text-sm">
                <font-awesome-icon :icon="['fad', 'location-dot']" /> {{ a.location }}
              </p>
              <div class="affiliate-card-actions">
                <button
                  class="btn-confirm btn-sm"
                  aria-label="Edit affiliate"
                  title="Edit affiliate"
                  @click="editAffiliate(a)"
                >
                  <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Edit
                </button>
                <button
                  class="btn-danger btn-sm"
                  aria-label="Delete affiliate"
                  title="Delete affiliate"
                  @click="affiliates.deleteAffiliate(a.id)"
                >
                  <font-awesome-icon :icon="['fas', 'trash']" /> Delete
                </button>
              </div>
            </div>
          </div>
        </div>
        <EmptyState v-else text="No affiliates match your search." />
      </template>
      <EmptyState v-else text="No affiliates yet." />
    </ManagerView>
  </div>
</template>

<style scoped>
.affiliate-card-actions {
  margin-top: 10px;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
