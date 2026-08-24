<script setup lang="ts">
/**
 * Records a payment against one raffle entry (admin), with an optional amount
 * waived - the gil forgiven this time round (a comped entry, a prize credit).
 *
 * Entries merge per character+world, so one row grows as a player buys more
 * tickets. That makes two things matter here:
 *   - the modal settles whatever is currently OUTSTANDING, not the whole row, so
 *     the summary always quotes the tickets that are actually being paid for;
 *   - the waiver is what is forgiven NOW. The server adds it to the entry's
 *     running total rather than replacing it, so a player whose first entry was
 *     free and who later buys two more keeps both waivers. The field therefore
 *     opens at zero every time, never seeded with what was waived before.
 *
 * Rendered with `v-if` from RafflesTab; emits `saved` after a successful write
 * and `close` otherwise.
 */
import { computed, ref } from 'vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import FormField from '@/components/common/ui/FormField.vue'
import {
  useRafflesStore,
  entryAmountCollected,
  entryAmountOutstanding,
  entryPaymentState,
  entryTicketPrice,
  raffleEntryCost,
  raffleHasCost,
} from '@/stores/raffles'
import type { Raffle, RaffleEntry } from '@/types/api'

const props = defineProps<{ raffle: Raffle; entry: RaffleEntry }>()
const emit = defineEmits<{ saved: []; close: [] }>()

const raffles = useRafflesStore()

/** Gil forgiven on THIS settlement. Always starts at zero - it is not a total. */
const waived = ref(0)

/**
 * How many of the entry's tickets this payment covers. Starts at all of them -
 * paying up in full is the common counter case - and can be dialled back to
 * record a part payment ("gil for two now, the third later").
 */
const ticketsPaid = ref(props.entry.num_entries)

const state = computed(() => entryPaymentState(props.entry))
const charges = computed(() => raffleHasCost(props.raffle))
const ticketPrice = computed(() => entryTicketPrice(props.raffle, props.entry))
const collected = computed(() => entryAmountCollected(props.raffle, props.entry))
const outstanding = computed(() => entryAmountOutstanding(props.raffle, props.entry))

/** Unsettled tickets - what this settlement covers. */
const unsettled = computed(() => Math.max(0, props.entry.num_entries - props.entry.paid_entries))

/** The ticket box, normalized: at least one, never past what the entry holds. */
const ticketsSettled = computed(() =>
  Math.min(Math.max(Math.floor(ticketsPaid.value) || 1, 1), props.entry.num_entries),
)

/** True when this payment deliberately leaves some tickets unsettled. */
const isPartPayment = computed(() => ticketsSettled.value < props.entry.num_entries)

/** What this payment covers, priced through the raffle's entry mode. */
const payingFor = computed(
  () =>
    raffleEntryCost(props.raffle, ticketsSettled.value) -
    raffleEntryCost(props.raffle, props.entry.paid_entries),
)

/** The waived field, normalized: never negative, never more than is outstanding. */
const waiveAmount = computed(() => {
  if (!Number.isFinite(waived.value)) return 0
  return Math.min(Math.max(waived.value, 0), Math.max(0, payingFor.value))
})

/** What the player actually hands over for this settlement. */
const dueNow = computed(() => Math.max(0, payingFor.value - waiveAmount.value))

const gil = (n: number): string => `${n.toLocaleString()} gil`

async function settle(): Promise<void> {
  if (await raffles.setEntryPaid(props.entry, true, waiveAmount.value, ticketsSettled.value))
    emit('saved')
}

async function clearPayment(): Promise<void> {
  if (await raffles.setEntryPaid(props.entry, false)) emit('saved')
}
</script>

<template>
  <ModalOverlay aria-label="Record payment" @close="emit('close')">
    <div class="flex-between mb-12">
      <h3>
        <font-awesome-icon :icon="['fad', 'coins']" /> {{ entry.character_name }} @
        {{ entry.world }}
      </h3>
      <button class="btn-neutral btn-sm" @click="emit('close')">Close</button>
    </div>

    <ul class="stack mb-16">
      <li class="stack-row">
        <span class="stack-label">Entries</span>
        <strong>{{ entry.num_entries }}</strong>
        <span v-if="state === 'partial'" class="badge badge--warning">
          {{ entry.paid_entries }} paid, {{ unsettled }} outstanding
        </span>
        <span v-else-if="state === 'paid'" class="badge badge--success">Fully paid</span>
      </li>
      <template v-if="charges">
        <li class="stack-row">
          <span class="stack-label">Tickets total</span>
          <strong>{{ gil(ticketPrice) }}</strong>
        </li>
        <li v-if="entry.amount_waived > 0" class="stack-row">
          <span class="stack-label">Already waived</span>
          <strong>{{ gil(entry.amount_waived) }}</strong>
        </li>
        <li class="stack-row">
          <span class="stack-label">Collected</span>
          <strong>{{ gil(collected) }}</strong>
        </li>
      </template>
    </ul>

    <!-- Settlement form: only while something is outstanding. -->
    <template v-if="state !== 'paid'">
      <!-- Part payments: dial the count back to settle only some of the tickets
           ("gil for two now, the third later"). Only worth offering when the entry
           holds more than one. -->
      <FormField
        v-if="entry.num_entries > 1"
        label="Entries Paid For"
        :help="`Out of ${entry.num_entries}. Lower it to record a part payment.`"
      >
        <input
          v-model.number="ticketsPaid"
          type="number"
          min="1"
          :max="entry.num_entries"
          aria-label="Entries paid for"
        />
      </FormField>
      <FormField
        v-if="charges"
        label="Amount Waived"
        help="Gil forgiven on this payment only - it is added to anything waived earlier, never replacing it."
      >
        <input
          v-model.number="waived"
          type="number"
          min="0"
          :max="outstanding"
          step="any"
          aria-label="Amount waived"
        />
      </FormField>
      <p v-if="charges" class="mb-12">
        <span class="text-muted">
          {{ isPartPayment ? 'This payment' : 'Outstanding' }} {{ gil(payingFor) }}
        </span>
        <template v-if="waiveAmount > 0"> - waived {{ gil(waiveAmount) }}</template>
        <strong class="text-highlight"> = {{ gil(dueNow) }} due</strong>
      </p>
      <p v-if="isPartPayment" class="text-muted text-sm mb-12">
        Leaves {{ entry.num_entries - ticketsSettled }} of {{ entry.num_entries }} unpaid.
      </p>
      <div class="flex-toolbar">
        <button class="btn-confirm" :disabled="raffles.settlingEntry" @click="settle">
          <LoadingSpinner v-if="raffles.settlingEntry" label="Saving..." />
          <template v-else>
            <font-awesome-icon :icon="['fas', 'circle-check']" /> Mark Paid
          </template>
        </button>
        <button
          v-if="state === 'partial'"
          class="btn-danger"
          :disabled="raffles.settlingEntry"
          @click="clearPayment"
        >
          Clear Payment
        </button>
      </div>
    </template>

    <!-- Fully settled: the only move left is to undo it. -->
    <template v-else>
      <p class="text-muted mb-12">
        Clearing this resets the entry to unpaid and removes everything waived on it.
      </p>
      <button class="btn-danger" :disabled="raffles.settlingEntry" @click="clearPayment">
        <LoadingSpinner v-if="raffles.settlingEntry" label="Saving..." />
        <template v-else>Mark Unpaid</template>
      </button>
    </template>
  </ModalOverlay>
</template>
