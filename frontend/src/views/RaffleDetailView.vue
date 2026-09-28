<script setup lang="ts">
/**
 * Public raffle detail + sign-up.
 *
 * Receives the raffle id via the `id` route param and loads it on mount (and
 * when the param changes) so the URL is directly linkable and survives a
 * refresh. Redirects back to the list if the raffle can't be loaded. Back
 * navigates to the public raffle list.
 */
import { computed, onMounted, ref, useTemplateRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import TurnstileWidget from '@/components/common/TurnstileWidget.vue'
import WorldPicker from '@/components/common/ui/WorldPicker.vue'
import {
  useRafflesStore,
  isRaffleEnterable,
  raffleAcceptsSignups,
  raffleCostLabel,
  raffleTicketCost,
} from '@/stores/raffles'
import { assetUrl } from '@/lib/assets'
import { endpoints } from '@/lib/endpoints'
import { RAFFLE_LOOKUP_MIN_QUERY } from '@/lib/constants'
import { savedRaffleSignup } from '@/lib/signups'
import type { RaffleLookupEntry } from '@/types/api'

const props = defineProps<{ id: string }>()

/** Payment status wording + badge tint for a search hit. */
const PAYMENT_LABELS: Record<string, { text: string; badge: string }> = {
  paid: { text: 'Paid', badge: 'badge--success' },
  partial: { text: 'Partly paid', badge: 'badge--warning' },
  unpaid: { text: 'Not paid yet', badge: 'badge--muted' },
}

const router = useRouter()
const raffles = useRafflesStore()

const raffleId = computed(() => Number(props.id))

/**
 * A "Details Only" raffle that is currently live: it has no sign-up form, so the
 * page shows its sign-up instructions inline (that IS how a player enters) rather
 * than the "this raffle is closed" notice a non-enterable raffle would get.
 */
const detailsOnly = computed(
  () =>
    !!raffles.selectedRaffle &&
    !raffleAcceptsSignups(raffles.selectedRaffle) &&
    isRaffleEnterable(raffles.selectedRaffle),
)

/**
 * Gil this character's entry is already square for - settled tickets and anything
 * waived, folded together. Nonzero only for someone coming back for more entries,
 * which is exactly when quoting the full total would invite an overpayment.
 */
const alreadyCovered = computed(() => {
  const r = raffles.raffleSignupResult
  return r ? Math.max(0, r.total_cost - r.amount_due) : 0
})

/**
 * The per-entry price ladder of a custom-cost raffle, so a player can see up
 * front what a 2nd and 3rd entry cost. Empty for every other mode.
 */
const costTiers = computed(() => {
  const r = raffles.selectedRaffle
  if (!r || r.entry_mode !== 'custom') return []
  return r.tier_costs.map((_, i) => ({
    n: i + 1,
    cost: raffleTicketCost(r, i + 1),
    running: r.tier_costs.slice(0, i + 1).reduce((sum, c) => sum + c, 0),
  }))
})

/** Whether the form was filled from a previous entry made on this device. */
const prefilledFromDevice = ref(false)

// Cloudflare Turnstile bot check for the public sign-up (empty site key = disabled).
const turnstileSiteKey = ref('')
const turnstile = useTemplateRef<InstanceType<typeof TurnstileWidget>>('turnstile')

async function load(id: number): Promise<void> {
  if (!Number.isFinite(id)) {
    void router.replace({ name: 'raffles' })
    return
  }
  const ok = await raffles.loadPublicRaffleById(id)
  if (!ok) {
    void router.replace({ name: 'raffles' })
    return
  }

  // This browser has entered this raffle before, so reuse the EXACT name and world
  // it used. Entries merge on that pair: retyping it even slightly differently
  // starts a SECOND entry and splits the tickets across both. That is precisely
  // what the name search below exists to prevent - this just gets there without
  // making the entrant search for themselves.
  //
  // Only the spelling is restored. How many entries they may still take is the
  // server's business, not this browser's, so nothing here is treated as a count.
  const mine = savedRaffleSignup(id)
  prefilledFromDevice.value = !!mine
  if (mine) {
    raffles.raffleSignup.characterName = mine.name
    raffles.raffleSignup.world = mine.world
  }
}

onMounted(async () => {
  void load(raffleId.value)
  try {
    turnstileSiteKey.value = (await endpoints.system.config()).turnstile_site_key
  } catch {
    turnstileSiteKey.value = '' // config probe failed -> behave as if disabled
  }
})
watch(raffleId, (id) => load(id))

function onTurnstileVerified(token: string): void {
  raffles.signupTurnstileToken = token
}
function onTurnstileCleared(): void {
  raffles.signupTurnstileToken = ''
}
/** Sign up, then re-issue a fresh Turnstile token if the attempt failed (the
 *  token is single-use and the store cleared it). */
async function signUp(): Promise<void> {
  await raffles.enterRaffle()
  if (!raffles.raffleSignupResult) {
    turnstile.value?.reset()
  }
}

// What the entrant typed into the "have I already entered?" box.
const lookupName = ref('')

/** Long enough to search - same code-point rule the store and server apply. */
const lookupReady = computed(
  () => Array.from(lookupName.value.trim()).length >= RAFFLE_LOOKUP_MIN_QUERY,
)

function runLookup(): void {
  void raffles.lookupEntries(lookupName.value)
}

/**
 * Copy a hit's exact name into the sign-up form. The whole point of the search is
 * to reproduce the spelling that was used before - entries merge on
 * character+world, so retyping it slightly differently starts a second entry and
 * splits the tickets across both.
 */
function useHit(hit: RaffleLookupEntry): void {
  raffles.raffleSignup.characterName = hit.character_name
  raffles.raffleSignup.world = hit.world
  // Came from the search, not from this device's saved entry - the notice would
  // be claiming the wrong source.
  prefilledFromDevice.value = false
}

function back(): void {
  raffles.selectedRaffle = null
  raffles.clearEntryLookup()
  void router.push({ name: 'raffles' })
}
</script>

<template>
  <div v-if="raffles.selectedRaffle">
    <div class="topbar">
      <button class="btn-neutral btn-sm" @click="back">
        <font-awesome-icon :icon="['fas', 'arrow-left']" /> Back
      </button>
      <h2>{{ raffles.selectedRaffle.title }}</h2>
      <span></span>
    </div>
    <div class="tab-body raffle-detail-body">
      <!-- Prize image -->
      <div v-if="raffles.selectedRaffle.prize_image" class="prize-container">
        <img :src="assetUrl(raffles.selectedRaffle.prize_image)" class="prize-img" alt="Prize" />
      </div>

      <!-- Description -->
      <MarkdownText
        v-if="raffles.selectedRaffle.description"
        class="game-details mb-16"
        :source="raffles.selectedRaffle.description"
      />

      <!-- Rules -->
      <div v-if="raffles.selectedRaffle.rules" class="mb-16">
        <h3 class="section-heading">Rules</h3>
        <MarkdownText class="game-details" :source="raffles.selectedRaffle.rules" />
      </div>

      <!-- Entry costs. A custom-cost raffle lists every rung so a player can
           see what a 2nd and 3rd entry cost before committing to the first. -->
      <div v-if="costTiers.length" class="mb-16">
        <h3 class="section-heading">Entry Costs</h3>
        <ul class="stack">
          <li v-for="tier in costTiers" :key="tier.n" class="stack-row">
            <span class="stack-label">Entry {{ tier.n }}</span>
            <strong class="text-highlight">{{ tier.cost.toLocaleString() }} gil</strong>
            <span v-if="tier.n > 1" class="text-muted text-sm">
              ({{ tier.running.toLocaleString() }} gil for all {{ tier.n }})
            </span>
          </li>
        </ul>
      </div>

      <!-- Sign-up result (shown after signing up) -->
      <div v-if="raffles.raffleSignupResult" class="raffle-signup-result">
        <h3 class="mb-8">
          <font-awesome-icon :icon="['fad', 'circle-check']" />
          {{ raffles.raffleSignupResult.message }}
        </h3>
        <p><strong>Total Entries:</strong> {{ raffles.raffleSignupResult.total_entries }}</p>
        <p>
          <strong>Total Cost:</strong>
          {{ raffles.raffleSignupResult.total_cost.toLocaleString() }} gil
        </p>
        <!-- What they actually send. It only differs from the total once part of
             this character's entry has already been settled, so the "already
             covered" line only appears for a returning entrant. -->
        <p v-if="alreadyCovered > 0" class="text-muted">
          <strong>Already Covered:</strong> {{ alreadyCovered.toLocaleString() }} gil
        </p>
        <!-- The figure the player acts on, so it outweighs the totals above it. -->
        <p class="text-lg text-highlight">
          <strong>Amount Due:</strong>
          {{ raffles.raffleSignupResult.amount_due.toLocaleString() }} gil
        </p>
        <div v-if="raffles.raffleSignupResult.signup_instructions" class="game-details mt-12">
          <h4 class="text-highlight mb-6">Sign-Up Instructions</h4>
          <MarkdownText :source="raffles.raffleSignupResult.signup_instructions" />
        </div>
        <figure v-if="raffles.selectedRaffle.pay_image" class="captioned-figure">
          <figcaption>Where to pay</figcaption>
          <img :src="assetUrl(raffles.selectedRaffle.pay_image)" alt="Where to pay" />
        </figure>
      </div>

      <!-- Details-only raffle: there is no form here, so the instructions for
           entering elsewhere are the point of the page. -->
      <div v-if="detailsOnly" class="raffle-signup-form">
        <h3 class="mb-12">How to Enter</h3>
        <MarkdownText
          v-if="raffles.selectedRaffle.signup_instructions"
          class="game-details"
          :source="raffles.selectedRaffle.signup_instructions"
        />
        <p v-else class="text-muted m-0">
          Sign-ups for this raffle are handled outside the site - check the description above.
        </p>
        <figure v-if="raffles.selectedRaffle.pay_image" class="captioned-figure">
          <figcaption>Where to pay</figcaption>
          <img :src="assetUrl(raffles.selectedRaffle.pay_image)" alt="Where to pay" />
        </figure>
      </div>

      <!-- "Have I already entered?" - shown alongside the sign-up form, because
           entries merge on character + world and a different spelling starts a
           second entry that splits the tickets. -->
      <div
        v-if="raffles.selectedRaffleEnterable && !raffles.raffleSignupResult"
        class="raffle-signup-form mb-16"
      >
        <h3 class="mb-8">Already Entered?</h3>
        <p class="text-muted text-sm mb-10">
          Search your character name to check, and to sign up again under the same spelling.
        </p>
        <div class="flex-toolbar mb-10">
          <input
            v-model="lookupName"
            placeholder="Character name"
            aria-label="Search your character name"
            style="flex: 1; min-width: 160px"
            @keyup.enter="runLookup"
          />
          <button
            class="btn-view"
            :disabled="raffles.entryLookupLoading || !lookupReady"
            @click="runLookup"
          >
            <LoadingSpinner v-if="raffles.entryLookupLoading" label="Searching..." />
            <template v-else>
              <font-awesome-icon :icon="['fas', 'magnifying-glass']" /> Search
            </template>
          </button>
        </div>

        <!-- null = no search yet, [] = a search that matched nothing. Saying
             "nothing matched" before a search has run would tell someone they
             hadn't entered when nobody had looked. -->
        <template v-if="raffles.entryLookupResults !== null">
          <ul v-if="raffles.entryLookupResults.length" class="stack">
            <li
              v-for="hit in raffles.entryLookupResults"
              :key="hit.character_name + hit.world"
              class="stack-row"
            >
              <strong>{{ hit.character_name }} @ {{ hit.world }}</strong>
              <span class="text-sm text-muted">
                {{ hit.num_entries }} {{ hit.num_entries === 1 ? 'entry' : 'entries' }}
              </span>
              <span :class="['badge', PAYMENT_LABELS[hit.payment_state]?.badge ?? 'badge--muted']">
                {{ PAYMENT_LABELS[hit.payment_state]?.text ?? hit.payment_state }}
                <template v-if="hit.payment_state === 'partial'">
                  ({{ hit.paid_entries }}/{{ hit.num_entries }})
                </template>
              </span>
              <button
                class="btn-view btn-sm push-right"
                title="Fill the sign-up form with this exact name"
                @click="useHit(hit)"
              >
                Use this name
              </button>
            </li>
          </ul>
          <p v-else class="text-muted m-0">No entries match that name yet.</p>
          <p v-if="raffles.entryLookupTruncated" class="text-muted text-sm mt-8">
            More entries matched than are shown - try a longer part of the name.
          </p>
        </template>
      </div>

      <!-- Sign-up form (only while the raffle is open, not past its end, and no result yet) -->
      <div
        v-if="raffles.selectedRaffleEnterable && !raffles.raffleSignupResult"
        class="raffle-signup-form"
      >
        <h3 class="mb-12">Enter This Raffle</h3>

        <!--
          Says where the name came from, because it is not something the entrant
          typed on this visit. It also tells them adding entries is expected: the
          per-player cap is a total, so someone who took 5 of 10 can come back for
          the rest under the same name. How many remain is the server's answer,
          not this browser's, so no count is claimed here.
        -->
        <p v-if="prefilledFromDevice" class="form-alert form-alert-info" role="status">
          <font-awesome-icon :icon="['fas', 'circle-info']" class="form-alert-icon" />
          <span>
            You entered this raffle from this device. We've filled in the same name so any entries
            you add here join the ones you already have instead of starting a second entry.
          </span>
        </p>

        <div class="field mb-10">
          <label class="field-label">Character Name</label>
          <input
            v-model="raffles.raffleSignup.characterName"
            placeholder="Character Name"
            aria-label="Character Name"
          />
        </div>
        <WorldPicker v-model="raffles.raffleSignup.world" class="mb-10" />
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
        <p v-if="raffleCostLabel(raffles.selectedRaffle)" class="mb-12" style="font-size: 0.95rem">
          <strong>Total Cost:</strong> {{ raffles.raffleTotalCost().toLocaleString() }} gil
        </p>
        <!-- Cloudflare Turnstile bot check (only when a site key is configured). -->
        <TurnstileWidget
          v-if="turnstileSiteKey"
          ref="turnstile"
          :site-key="turnstileSiteKey"
          class="mb-10"
          @verified="onTurnstileVerified"
          @expired="onTurnstileCleared"
          @error="onTurnstileCleared"
        />
        <button
          class="btn-confirm"
          :disabled="
            !raffles.raffleSignup.characterName.trim() ||
            !raffles.raffleSignup.world.trim() ||
            raffles.entering ||
            (!!turnstileSiteKey && !raffles.signupTurnstileToken)
          "
          @click="signUp()"
        >
          <LoadingSpinner v-if="raffles.entering" label="Signing up..." />
          <template v-else>Sign Up</template>
        </button>
      </div>

      <div
        v-if="!raffles.selectedRaffleEnterable && !detailsOnly && !raffles.raffleSignupResult"
        class="raffle-closed-msg"
      >
        <p class="text-muted" style="text-align: center; padding: 20px; font-size: 1.1rem">
          <font-awesome-icon :icon="['fad', 'lock']" /> This raffle is closed.
        </p>
      </div>
    </div>
  </div>
  <div v-else-if="raffles.detailLoading" class="tab-body">
    <LoadingSpinner block label="Loading raffle..." />
  </div>
</template>
