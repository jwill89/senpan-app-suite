<script setup lang="ts">
/**
 * One of a festival's raffles on the festival dashboard: what it is, whether it is
 * running, the visitor's own entry and how far it is paid, the winner once drawn,
 * and - while it runs - an entry form under the visitor's name.
 *
 * Entries merge on character + world, which is why the form has no name field: it
 * always enters as the visitor the dashboard knows, so buying more tickets adds to
 * the entry they already hold rather than starting a second one.
 */
import { computed, ref, useTemplateRef } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import TurnstileWidget from '@/components/common/TurnstileWidget.vue'
import { useFestivalDashboardStore } from '@/stores/festivalDashboard'
import { raffleAcceptsSignups, raffleCostLabel, raffleEntryCost } from '@/stores/raffles'
import { assetUrl } from '@/lib/assets'
import type { FestivalDashboardRaffle } from '@/types/api'

const props = defineProps<{
  row: FestivalDashboardRaffle
  /** Cloudflare Turnstile site key ('' = the check is off). */
  turnstileSiteKey: string
}>()

/** Payment status wording + badge tint, as on the raffle page's search. */
const PAYMENT_LABELS: Record<string, { text: string; badge: string }> = {
  paid: { text: 'Paid', badge: 'badge--success' },
  partial: { text: 'Partly paid', badge: 'badge--warning' },
  unpaid: { text: 'Not paid yet', badge: 'badge--muted' },
}

const store = useFestivalDashboardStore()

const raffle = computed(() => props.row.raffle)
const held = computed(() => props.row.entry?.num_entries ?? 0)
/** Entries the visitor may still add: the allowance is per player, in total. */
const remaining = computed(() => Math.max(0, raffle.value.max_entries - held.value))
const takesSignups = computed(() => raffleAcceptsSignups(raffle.value))
const costLabel = computed(() => raffleCostLabel(raffle.value))
const confirmation = computed(() => store.entered[raffle.value.id] ?? null)
const payment = computed(() =>
  props.row.entry ? (PAYMENT_LABELS[props.row.entry.payment_state] ?? null) : null,
)

const formOpen = ref(false)
const count = ref(1)
const turnstileToken = ref('')
const turnstile = useTemplateRef<InstanceType<typeof TurnstileWidget>>('turnstile')

/** The count actually sent: a whole number inside what the visitor may still add. */
const clamped = computed(() =>
  Math.min(Math.max(1, Math.floor(count.value || 1)), Math.max(1, remaining.value)),
)
/**
 * What the new tickets cost. Measured from the tickets already held, because on a
 * tiered raffle a 3rd ticket costs its own rung, not the 1st's.
 */
const addedCost = computed(
  () =>
    raffleEntryCost(raffle.value, held.value + clamped.value) -
    raffleEntryCost(raffle.value, held.value),
)

function toggleForm(): void {
  formOpen.value = !formOpen.value
  count.value = 1
  turnstileToken.value = ''
}

async function submit(): Promise<void> {
  const ok = await store.enterRaffle(raffle.value.id, clamped.value, turnstileToken.value)
  if (ok) {
    formOpen.value = false
    return
  }
  // The token is single-use, so a refused attempt needs a fresh one.
  turnstileToken.value = ''
  turnstile.value?.reset()
}
</script>

<template>
  <article :id="`festival-raffle-${raffle.id}`" class="festival-raffle card card--padded">
    <div class="festival-raffle-head">
      <img
        v-if="raffle.prize_image"
        :src="assetUrl(raffle.prize_image)"
        class="festival-raffle-prize"
        alt="Prize"
      />
      <div>
        <h4 class="festival-raffle-title">
          {{ raffle.title }}
          <span v-if="row.running" class="status-badge status-badge-open">running</span>
          <span v-else class="status-badge status-badge-closed">closed</span>
        </h4>
        <p v-if="raffle.stall_name" class="text-sm text-muted m-0">
          <font-awesome-icon :icon="['fad', 'store']" /> {{ raffle.stall_name }}
        </p>
        <p class="text-sm text-muted m-0">
          <template v-if="costLabel">{{ costLabel }} - </template>{{ row.total_entries }}
          {{ row.total_entries === 1 ? 'entry' : 'entries' }} so far
        </p>
      </div>
    </div>

    <!-- The visitor's own standing in it. -->
    <p v-if="row.entry" class="festival-raffle-mine">
      <span>
        You hold <strong>{{ row.entry.num_entries }}</strong>
        {{ row.entry.num_entries === 1 ? 'entry' : 'entries' }}
      </span>
      <span v-if="payment" :class="['badge', payment.badge]">
        {{ payment.text }}
        <template v-if="row.entry.payment_state === 'partial'">
          ({{ row.entry.paid_entries }}/{{ row.entry.num_entries }})
        </template>
      </span>
    </p>

    <!-- Drawn. -->
    <div v-if="row.is_winner" class="callout">
      <font-awesome-icon :icon="['fad', 'champagne-glasses']" /> You won this raffle!
    </div>
    <p v-else-if="row.winner_name" class="text-sm m-0">
      <font-awesome-icon :icon="['fad', 'trophy']" /> Won by <strong>{{ row.winner_name }}</strong>
    </p>

    <!-- Just entered here: what to send, and where. -->
    <div v-if="confirmation" class="festival-raffle-confirmation">
      <p class="m-0">
        <font-awesome-icon :icon="['fad', 'circle-check']" /> {{ confirmation.message }}
      </p>
      <p v-if="confirmation.amount_due > 0" class="text-lg text-highlight m-0">
        <strong>Amount due:</strong> {{ confirmation.amount_due.toLocaleString() }} gil
      </p>
      <MarkdownText
        v-if="confirmation.signup_instructions"
        class="game-details"
        :source="confirmation.signup_instructions"
      />
      <figure v-if="raffle.pay_image" class="captioned-figure">
        <figcaption>Where to pay</figcaption>
        <img :src="assetUrl(raffle.pay_image)" alt="Where to pay" />
      </figure>
    </div>

    <details class="festival-raffle-about">
      <summary>About this raffle</summary>
      <MarkdownText v-if="raffle.description" class="game-details" :source="raffle.description" />
      <template v-if="raffle.rules">
        <h5 class="section-heading">Rules</h5>
        <MarkdownText class="game-details" :source="raffle.rules" />
      </template>
      <!-- A details-only raffle is entered elsewhere: these ARE how to enter. -->
      <template v-if="!takesSignups && raffle.signup_instructions">
        <h5 class="section-heading">How to enter</h5>
        <MarkdownText class="game-details" :source="raffle.signup_instructions" />
      </template>
    </details>

    <!-- Entering. -->
    <template v-if="row.running && takesSignups">
      <p v-if="!store.visitorName" class="text-sm text-muted m-0">
        Say who you're visiting as at the top of the page to enter.
      </p>
      <p v-else-if="remaining === 0" class="text-sm text-muted m-0">
        You hold the most entries this raffle allows.
      </p>
      <button v-else-if="!formOpen" type="button" class="btn-action btn-sm" @click="toggleForm">
        <font-awesome-icon :icon="['fas', 'ticket']" />
        {{ row.entry ? 'Add entries' : 'Enter this raffle' }}
      </button>

      <form v-else class="festival-raffle-form" @submit.prevent="submit">
        <p class="text-sm m-0">
          Entering as <strong>{{ store.visitorName }} @ {{ store.visitorWorld }}</strong>
        </p>
        <label v-if="remaining > 1" class="field">
          <span class="field-label">Entries to add (up to {{ remaining }})</span>
          <input v-model.number="count" type="number" min="1" step="1" :max="remaining" />
        </label>
        <p v-if="addedCost > 0" class="m-0">
          <strong>Cost:</strong> {{ addedCost.toLocaleString() }} gil
        </p>
        <TurnstileWidget
          v-if="turnstileSiteKey"
          ref="turnstile"
          :site-key="turnstileSiteKey"
          @verified="turnstileToken = $event"
          @expired="turnstileToken = ''"
          @error="turnstileToken = ''"
        />
        <div class="form-actions">
          <button
            type="submit"
            class="btn-confirm"
            :disabled="store.busy || (!!turnstileSiteKey && !turnstileToken)"
          >
            <LoadingSpinner v-if="store.busy" label="Entering..." />
            <template v-else>Enter</template>
          </button>
          <button type="button" class="btn-neutral" @click="toggleForm">Cancel</button>
        </div>
      </form>
    </template>
  </article>
</template>
