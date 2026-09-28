<script setup lang="ts">
/**
 * Admin Raffle create/edit form. Markdown fields (description, rules, sign-up
 * instructions), the entry mode and its costs, availability window, and prize
 * image.
 *
 * The entry mode decides which cost controls exist at all:
 *   Details Only        - no costs, no public sign-up form.
 *   Single Cost/Entry   - one price + a per-player max.
 *   Custom Cost/Entry   - a ladder of per-entry prices; its length IS the max.
 *
 * Hosted as a Back sub-page of the Raffles manager (RafflesTab): it emits `saved`
 * on a successful save and `cancel`/`back` to return to the list, rather than
 * navigating routes itself.
 */
import { computed, onMounted } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormRow from '@/components/common/ui/FormRow.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import ImagePicker from '@/components/common/ui/ImagePicker.vue'
import { useRafflesStore } from '@/stores/raffles'
import { RAFFLE_MODES, type RaffleMode } from '@/types/api'
import { RAFFLE_MAX_ENTRIES } from '@/lib/constants'
import { occupantListLabel, stallCaption } from '@/lib/festivalmap'

const emit = defineEmits<{ saved: []; cancel: [] }>()
const raffles = useRafflesStore()

onMounted(() => raffles.loadFormSources())

/**
 * Whether this raffle is filed under a festival map, which is what turns the
 * Stall select on. Read off the form rather than off the loaded stall list, which
 * is momentarily empty while a newly-picked map's stalls are fetched.
 */
const linkedToMap = computed(() => raffles.raffleForm?.festival_map_id != null)

/** File the raffle under a festival map ('' -> not part of a festival). */
function setFestivalMap(value: string): void {
  void raffles.setFestivalMap(value ? Number(value) : null)
}

/** Pin the raffle to one stall on the linked map ('' -> not pinned). */
function setOccupant(value: string): void {
  if (raffles.raffleForm) raffles.raffleForm.occupant_id = value ? Number(value) : null
}

const mode = computed<RaffleMode>(() => raffles.raffleForm?.entry_mode ?? 'single')

/** Help text for the selected mode, shown under the picker. */
const modeHelp = computed(() => RAFFLE_MODES.find((m) => m.value === mode.value)?.help ?? '')

/** True once the ladder is as long as a player is allowed to buy. */
const ladderFull = computed(
  () => (raffles.raffleForm?.tier_costs.length ?? 0) >= RAFFLE_MAX_ENTRIES,
)

/** Running total of the custom ladder - what buying every entry costs. */
const tiersTotal = computed(() =>
  (raffles.raffleForm?.tier_costs ?? []).reduce((sum, c) => sum + (Number.isFinite(c) ? c : 0), 0),
)

function setMode(value: RaffleMode): void {
  if (raffles.raffleForm) raffles.raffleForm.entry_mode = value
}

/** Save the form; on success let the parent return to the list. */
async function save(): Promise<void> {
  if (await raffles.saveRaffle()) emit('saved')
}

/** Discard the form and return to the list. */
function cancel(): void {
  raffles.cancelRaffleForm()
  emit('cancel')
}
</script>

<template>
  <AdminPanel>
    <SubPageHeader
      :icon="['fad', 'plus']"
      :title="`${raffles.raffleForm && raffles.raffleForm.id ? 'Edit' : 'New'} Raffle`"
      @back="cancel"
    />
    <template v-if="raffles.raffleForm">
      <FormField label="Title" required>
        <input
          v-model="raffles.raffleForm.title"
          placeholder="Raffle Title"
          aria-label="Raffle title"
        />
      </FormField>
      <FormField label="Description">
        <MarkdownEditor
          v-model="raffles.raffleForm.description"
          min-height="120px"
          placeholder="Description (supports markdown - bold, italics, lists, links...)"
        />
      </FormField>
      <FormField label="Rules">
        <MarkdownEditor
          v-model="raffles.raffleForm.rules"
          min-height="120px"
          placeholder="Rules (supports markdown)"
        />
      </FormField>
      <FormField
        label="Sign-Up Instructions"
        :help="
          mode === 'details'
            ? 'Shown on the raffle page itself - this is how players enter.'
            : 'Shown after a player signs up (how to pay, where to send the gil...).'
        "
      >
        <MarkdownEditor
          v-model="raffles.raffleForm.signup_instructions"
          min-height="120px"
          placeholder="Sign-up instructions (supports markdown)"
        />
      </FormField>
      <FormField
        label="Where to Pay"
        help="Usually a screenshot of where in the area to go to pay. Shown to players beneath the sign-up instructions."
      >
        <ImagePicker v-model="raffles.raffleForm.pay_image" />
      </FormField>

      <!-- Festival Map: files the raffle under a festival and, optionally, pins it
           to one stall so the public plan links to it. -->
      <FormRow>
        <FormField
          label="Festival Map"
          help="Optional. Files this raffle under a festival, the same way a stamp rally is."
        >
          <select
            :value="raffles.raffleForm.festival_map_id ?? ''"
            aria-label="Linked festival map"
            @change="setFestivalMap(($event.target as HTMLSelectElement).value)"
          >
            <option value="">Not part of a festival</option>
            <option v-for="m in raffles.festivalMaps" :key="m.id" :value="m.id">
              {{ m.title }}
            </option>
          </select>
        </FormField>
        <FormField
          v-if="linkedToMap"
          label="Stall"
          help="Optional. The stall running it - its panel on the map links here while the raffle is open and inside its dates."
        >
          <select
            :value="raffles.raffleForm.occupant_id ?? ''"
            aria-label="Festival map stall"
            @change="setOccupant(($event.target as HTMLSelectElement).value)"
          >
            <option value="">Not pinned to a stall</option>
            <option v-for="o in raffles.mapStalls" :key="o.id" :value="o.id">
              {{ occupantListLabel(o) }} ({{ stallCaption(o) || 'Other' }})
            </option>
          </select>
        </FormField>
      </FormRow>

      <!-- Entry mode: decides which cost controls below exist at all. -->
      <FormField label="Entry Type" :help="modeHelp">
        <div class="toggle-group">
          <button
            v-for="m in RAFFLE_MODES"
            :key="m.value"
            type="button"
            class="toggle-btn"
            :class="{ 'is-active': mode === m.value }"
            @click="setMode(m.value)"
          >
            {{ m.label }}
          </button>
        </div>
      </FormField>

      <!-- Single cost per entry: one price, one cap. -->
      <FormRow v-if="mode === 'single'">
        <FormField label="Max Entries Per Person" :help="`Up to ${RAFFLE_MAX_ENTRIES}.`">
          <input
            v-model.number="raffles.raffleForm.max_entries"
            type="number"
            min="1"
            :max="RAFFLE_MAX_ENTRIES"
            aria-label="Max entries per person"
          />
        </FormField>
        <FormField label="Cost Per Entry">
          <input
            v-model.number="raffles.raffleForm.cost_per_entry"
            type="number"
            min="0"
            step="any"
            aria-label="Cost per entry"
          />
        </FormField>
      </FormRow>

      <!-- Custom cost per entry: a price ladder whose length is the cap. -->
      <FormField
        v-else-if="mode === 'custom'"
        label="Cost Per Entry"
        :help="`One price per entry, in order. The number of rows is also the maximum a player may buy (up to ${RAFFLE_MAX_ENTRIES}).`"
      >
        <div class="stack">
          <div v-for="(_, i) in raffles.raffleForm.tier_costs" :key="i" class="stack-row">
            <span class="field-label" style="flex: 0 0 72px; margin: 0">Entry {{ i + 1 }}</span>
            <input
              v-model.number="raffles.raffleForm.tier_costs[i]"
              type="number"
              min="0"
              step="any"
              style="flex: 1; min-width: 100px"
              :aria-label="`Cost of entry ${i + 1}`"
            />
            <button
              type="button"
              class="btn-danger btn-sm"
              :disabled="raffles.raffleForm.tier_costs.length <= 1"
              :aria-label="`Remove entry ${i + 1}`"
              title="Remove this entry"
              @click="raffles.removeRaffleTier(i)"
            >
              <font-awesome-icon :icon="['fas', 'trash']" />
            </button>
          </div>
        </div>
        <div class="flex-toolbar mt-8">
          <button
            type="button"
            class="btn-confirm btn-sm"
            :disabled="ladderFull"
            :title="ladderFull ? `A player may buy at most ${RAFFLE_MAX_ENTRIES} entries` : ''"
            @click="raffles.addRaffleTier()"
          >
            <font-awesome-icon :icon="['fas', 'plus']" /> Add Entry Cost
          </button>
          <span class="text-sm text-muted">
            All {{ raffles.raffleForm.tier_costs.length }} entries:
            {{ tiersTotal.toLocaleString() }} gil
          </span>
        </div>
      </FormField>

      <!-- Details only: no costs to set, but the per-person cap is still worth
           publishing, since staff record the entries by hand. -->
      <FormField v-else label="Max Entries Per Person" :help="`Up to ${RAFFLE_MAX_ENTRIES}.`">
        <input
          v-model.number="raffles.raffleForm.max_entries"
          type="number"
          min="1"
          :max="RAFFLE_MAX_ENTRIES"
          aria-label="Max entries per person"
        />
      </FormField>

      <FormRow>
        <FormField label="Available From">
          <input
            v-model="raffles.raffleForm.available_from"
            type="datetime-local"
            aria-label="Available from"
          />
        </FormField>
        <FormField label="Available To">
          <input
            v-model="raffles.raffleForm.available_to"
            type="datetime-local"
            aria-label="Available to"
          />
        </FormField>
      </FormRow>
      <FormField
        label="Prize Image"
        help="Pick from any image category. Upload new images on the System -> Images page."
      >
        <ImagePicker v-model="raffles.raffleForm.prize_image" />
      </FormField>
      <FormActions align="start">
        <button class="btn-neutral" :disabled="raffles.savingRaffle" @click="cancel">Cancel</button>
        <button
          class="btn-confirm"
          :disabled="!raffles.raffleForm.title.trim() || raffles.savingRaffle"
          @click="save"
        >
          <LoadingSpinner v-if="raffles.savingRaffle" label="Saving..." />
          <template v-else>Save Raffle</template>
        </button>
      </FormActions>
    </template>
  </AdminPanel>
</template>
