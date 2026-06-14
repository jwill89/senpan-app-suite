<script setup lang="ts">
/**
 * Public raffle detail + sign-up.
 *
 * Receives the raffle id via the `id` route param and loads it on mount (and
 * when the param changes) so the URL is directly linkable and survives a
 * refresh. Redirects back to the list if the raffle can't be loaded. Back
 * navigates to the public raffle list.
 */
import { computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useMarkdown } from '@/lib/markdown'
import { useRafflesStore } from '@/stores/raffles'
import { assetUrl } from '@/lib/assets'

const props = defineProps<{ id: string }>()

const router = useRouter()
const raffles = useRafflesStore()
const { render: renderMarkdown } = useMarkdown()

const raffleId = computed(() => Number(props.id))

async function load(id: number): Promise<void> {
  if (!Number.isFinite(id)) {
    router.replace({ name: 'raffles' })
    return
  }
  const ok = await raffles.loadPublicRaffleById(id)
  if (!ok) router.replace({ name: 'raffles' })
}

onMounted(() => load(raffleId.value))
watch(raffleId, (id) => load(id))

function back(): void {
  raffles.selectedRaffle = null
  router.push({ name: 'raffles' })
}
</script>

<template>
  <div v-if="raffles.selectedRaffle">
    <div class="topbar">
      <button class="btn-ghost btn-sm" @click="back">← Back</button>
      <h2>{{ raffles.selectedRaffle.title }}</h2>
      <span></span>
    </div>
    <div class="tab-body raffle-detail-body">
      <!-- Prize image -->
      <div v-if="raffles.selectedRaffle.prize_image" class="raffle-prize-container">
        <img :src="assetUrl(raffles.selectedRaffle.prize_image)" class="raffle-prize-img" alt="Prize" />
      </div>

      <!-- Description -->
      <div
        v-if="raffles.selectedRaffle.description"
        class="game-details game-details--wide mb-16"
        v-html="renderMarkdown(raffles.selectedRaffle.description)"
      ></div>

      <!-- Rules -->
      <div v-if="raffles.selectedRaffle.rules" class="mb-16">
        <h3 class="raffle-section-heading">Rules</h3>
        <div
          class="game-details game-details--wide"
          v-html="renderMarkdown(raffles.selectedRaffle.rules)"
        ></div>
      </div>

      <!-- Sign-up result (shown after signing up) -->
      <div v-if="raffles.raffleSignupResult" class="raffle-signup-result">
        <h3 class="text-success mb-8">
          <i class="fa-duotone fa-circle-check"></i> {{ raffles.raffleSignupResult.message }}
        </h3>
        <p><strong>Total Entries:</strong> {{ raffles.raffleSignupResult.total_entries }}</p>
        <p>
          <strong>Total Cost:</strong>
          {{ raffles.raffleSignupResult.total_cost.toLocaleString() }} gil
        </p>
        <div
          v-if="raffles.raffleSignupResult.signup_instructions"
          class="game-details game-details--wide mt-12"
        >
          <h4 class="text-gold mb-6">Sign-Up Instructions</h4>
          <div v-html="renderMarkdown(raffles.raffleSignupResult.signup_instructions)"></div>
        </div>
      </div>

      <!-- Sign-up form (only while the raffle is open, not past its end, and no result yet) -->
      <div
        v-if="raffles.selectedRaffleEnterable && !raffles.raffleSignupResult"
        class="raffle-signup-form"
      >
        <h3 class="mb-12">Enter This Raffle</h3>
        <div class="field mb-10">
          <label class="field-label">Character Name</label>
          <input
            v-model="raffles.raffleSignup.characterName"
            placeholder="Character Name"
            aria-label="Character Name"
          />
        </div>
        <div class="field mb-10">
          <label class="field-label">World</label>
          <input v-model="raffles.raffleSignup.world" placeholder="World" aria-label="World" />
        </div>
        <div v-if="raffles.selectedRaffle.max_entries > 1" class="field mb-10">
          <label class="field-label">
            Number of Entries (max {{ raffles.selectedRaffle.max_entries }})
          </label>
          <input
            v-model.number="raffles.raffleSignup.numEntries"
            type="number"
            min="1"
            step="1"
            :max="raffles.selectedRaffle.max_entries"
            aria-label="Number of entries"
            @change="raffles.clampSignupEntries()"
            @blur="raffles.clampSignupEntries()"
          />
        </div>
        <p v-if="raffles.selectedRaffle.cost_per_entry > 0" class="mb-12" style="font-size: 0.95rem">
          <strong>Total Cost:</strong> {{ raffles.raffleTotalCost().toLocaleString() }} gil
        </p>
        <button
          class="btn-primary"
          :disabled="
            !raffles.raffleSignup.characterName.trim() ||
            !raffles.raffleSignup.world.trim() ||
            raffles.entering
          "
          @click="raffles.enterRaffle()"
        >
          <LoadingSpinner v-if="raffles.entering" label="Signing up…" />
          <template v-else>Sign Up</template>
        </button>
      </div>

      <div
        v-if="!raffles.selectedRaffleEnterable && !raffles.raffleSignupResult"
        class="raffle-closed-msg"
      >
        <p class="msg-block" style="font-size: 1.1rem">
          <i class="fa-duotone fa-lock"></i> This raffle is closed.
        </p>
      </div>
    </div>
  </div>
  <div v-else-if="raffles.detailLoading" class="tab-body">
    <LoadingSpinner block label="Loading raffle…" />
  </div>
</template>
