<script setup lang="ts">
/**
 * Public raffles list. Loads the open raffles on mount (so a direct link /
 * refresh to /raffles works) and navigates to `/raffles/:id` on selection.
 */
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { Raffle } from '@/types/api'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useRafflesStore } from '@/stores/raffles'
import { assetUrl } from '@/lib/assets'

const raffles = useRafflesStore()
const router = useRouter()

// Local loading flag: the public list preloads via the silent `loadHomeRaffles`
// (which doesn't toggle the store's admin-facing `rafflesLoading`).
const loading = ref(false)

onMounted(async () => {
  // Populate the list if arriving directly (not via the Home card click).
  if (raffles.raffles.length === 0) {
    loading.value = true
    try {
      await raffles.loadHomeRaffles()
      raffles.raffles = raffles.homeRaffles
    } finally {
      loading.value = false
    }
  }
})

/** Open a raffle's public detail view. */
function openRaffle(r: Raffle): void {
  router.push({ name: 'raffle-detail', params: { id: r.id } })
}

function goHome(): void {
  router.push({ name: 'home' })
}
</script>

<template>
  <div>
    <div class="topbar">
      <button class="btn-ghost btn-sm" @click="goHome">← Home</button>
      <h2><i class="fa-solid fa-ticket"></i> Raffles</h2>
      <span></span>
    </div>
    <div class="tab-body">
      <LoadingSpinner v-if="loading" block label="Loading raffles…" />
      <div v-else class="raffle-list">
        <div
          v-for="r in raffles.raffles"
          :key="r.id"
          class="raffle-card"
          @click="openRaffle(r)"
        >
          <img v-if="r.prize_image" :src="assetUrl(r.prize_image)" class="raffle-card-image" alt="Prize" />
          <div class="raffle-card-body">
            <h3>{{ r.title }}</h3>
            <p v-if="r.cost_per_entry > 0" class="raffle-cost">
              {{ r.cost_per_entry.toLocaleString() }} gil per entry
            </p>
            <p v-if="r.max_entries > 1" class="text-sm text-dim">
              Up to {{ r.max_entries }} entries
            </p>
          </div>
        </div>
      </div>
      <p v-if="!loading && raffles.raffles.length === 0" class="no-game-msg">
        No raffles are currently open.
      </p>
    </div>
  </div>
</template>
