<script setup lang="ts">
/**
 * Public raffles list. Loads the open raffles on mount (so a direct link /
 * refresh to /raffles works) and navigates to `/raffles/:id` on selection.
 */
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { Raffle } from '@/types/api'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import { useRafflesStore, raffleAcceptsSignups, raffleCostLabel } from '@/stores/raffles'
import { assetUrl } from '@/lib/assets'

const raffles = useRafflesStore()
const router = useRouter()

// Local loading flag: the public list preloads via the silent `loadHomeRaffles`
// (which doesn't toggle the store's admin-facing `rafflesLoading`).
const loading = ref(false)

onMounted(async () => {
  // Always load, and read `homeRaffles` rather than the shared `raffles` array.
  // That array is the ADMIN list - unfiltered, including closed and out-of-window
  // raffles - so a session where the admin surface had already populated it showed
  // staff-only rows on this public page, and the old "only if empty" guard meant
  // the public list was never fetched at all in that case.
  loading.value = true
  try {
    await raffles.loadHomeRaffles()
  } finally {
    loading.value = false
  }
})

/** Open a raffle's public detail view. */
function openRaffle(r: Raffle): void {
  void router.push({ name: 'raffle-detail', params: { id: r.id } })
}

function goHome(): void {
  void router.push({ name: 'home' })
}
</script>

<template>
  <div>
    <div class="topbar">
      <button class="btn-neutral btn-sm" @click="goHome">
        <font-awesome-icon :icon="['fas', 'arrow-left']" /> Home
      </button>
      <h2><font-awesome-icon :icon="['fad', 'ticket']" /> Raffles</h2>
      <span></span>
    </div>
    <div class="tab-body content-container">
      <LoadingSpinner v-if="loading" block label="Loading raffles..." />
      <div v-else-if="raffles.homeRaffles.length" class="card-grid card-grid--center">
        <div
          v-for="r in raffles.homeRaffles"
          :key="r.id"
          class="media-card"
          role="button"
          tabindex="0"
          @click="openRaffle(r)"
          @keydown.enter="openRaffle(r)"
          @keydown.space.prevent="openRaffle(r)"
        >
          <img
            v-if="r.prize_image"
            :src="assetUrl(r.prize_image)"
            class="media-card-image"
            alt="Prize"
          />
          <div class="media-card-body">
            <h3>{{ r.title }}</h3>
            <p v-if="raffleCostLabel(r)" class="raffle-cost">{{ raffleCostLabel(r) }}</p>
            <p v-if="r.max_entries > 1" class="text-sm text-muted">
              Up to {{ r.max_entries }} entries
            </p>
            <p v-if="!raffleAcceptsSignups(r)" class="text-sm text-muted">
              Sign up outside the site - see details
            </p>
          </div>
        </div>
      </div>
      <EmptyState
        v-else
        :icon="['fad', 'ticket']"
        text="No raffles are currently open."
        hint="Check back soon - new raffles appear here when they go live."
      />
    </div>
  </div>
</template>
