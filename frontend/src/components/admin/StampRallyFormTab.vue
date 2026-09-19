<script setup lang="ts">
/**
 * Admin Stamp Rally create/edit form. Event fields (title, availability window, card
 * + not-stamped images, markdown details + redeem instructions), then the visual
 * placement editor where stamps and prizes are dragged/resized/rotated onto the card,
 * with a per-item panel below for the selected item's settings (a stamp's stall +
 * image + password + active window + pause; a prize's name + image).
 *
 * A rally can also be LINKED to a Festival Map. When it is, each stamp names one
 * of that map's stalls instead of a bare affiliate - so the map can badge the
 * stalls that are part of the rally, and the stamp log records the stall by the
 * name it carries on the plan.
 *
 * Hosted as a Back sub-page of the Stamp Rally manager: emits `saved` / `cancel`.
 */
import { computed, onMounted, ref, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormRow from '@/components/common/ui/FormRow.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import ImagePicker from '@/components/common/ui/ImagePicker.vue'
import PlacementEditor, { type PlaceItem } from './PlacementEditor.vue'
import { toStampCount, useStampRalliesStore } from '@/stores/stampRallies'
import { STAMP_TYPES, stampTypeLabel } from '@/lib/stampcard'
import { occupantListLabel, stallCaption } from '@/lib/festivalmap'
import type { Placement, StampType } from '@/types/api'

const emit = defineEmits<{ saved: []; cancel: [] }>()
const store = useStampRalliesStore()

onMounted(() => store.loadFormSources())

const selectedKey = ref<string | null>(null)

/** Combined stamp + prize items for the placement editor (empty image -> placeholder). */
const items = computed<PlaceItem[]>(() => {
  const f = store.rallyForm
  if (!f) return []
  const stamps = f.stamps.map((s, i) => ({
    key: `s${i}`,
    label: `${stampTypeLabel(s.stamp_type)} ${i + 1}`,
    image: s.image || f.not_stamped_image,
    placement: s.placement,
    kind: 'stamp' as const,
  }))
  const prizes = f.prizes.map((p, i) => ({
    key: `p${i}`,
    label: p.name || `Prize ${i + 1}`,
    image: p.image || f.not_stamped_image,
    placement: p.placement,
    kind: 'prize' as const,
  }))
  return [...stamps, ...prizes]
})

/** Apply an editor placement change back onto the matching form item. */
function applyUpdate(key: string, placement: Placement): void {
  const f = store.rallyForm
  if (!f) return
  const idx = Number(key.slice(1))
  const target = key[0] === 's' ? f.stamps[idx] : f.prizes[idx]
  Object.assign(target.placement, placement)
}

/** The currently-selected stamp or prize (for the editing panel). */
const selected = computed(() => {
  const f = store.rallyForm
  const k = selectedKey.value
  if (!f || !k) return null
  const idx = Number(k.slice(1))
  if (k[0] === 's') {
    const stamp = f.stamps[idx]
    return { kind: 'stamp' as const, index: idx, stamp }
  }
  const prize = f.prizes[idx]
  return { kind: 'prize' as const, index: idx, prize }
})

function addStamp(type: StampType): void {
  store.addStamp(type)
  selectedKey.value = `s${(store.rallyForm?.stamps.length ?? 1) - 1}`
}
function addPrize(): void {
  store.addPrize()
  selectedKey.value = `p${(store.rallyForm?.prizes.length ?? 1) - 1}`
}
function removeSelected(): void {
  const sel = selected.value
  if (!sel) return
  if (sel.kind === 'stamp') store.removeStamp(sel.index)
  else store.removePrize(sel.index)
  selectedKey.value = null
}

/** How many stamps of each type the card carries - the ceiling on what completion
 *  can require (the server clamps to the same numbers on save). */
const foodCount = computed(
  () => store.rallyForm?.stamps.filter((s) => s.stamp_type !== 'game').length ?? 0,
)
const gameCount = computed(
  () => store.rallyForm?.stamps.filter((s) => s.stamp_type === 'game').length ?? 0,
)

/** Set a stamp's affiliate from the select ('' -> Senpan Tea House default). */
function setAffiliate(stampIndex: number, value: string): void {
  const f = store.rallyForm
  if (!f || !f.stamps[stampIndex]) return
  f.stamps[stampIndex].affiliate_id = value ? Number(value) : null
}

/**
 * Whether this rally names Festival Map stalls instead of bare affiliates. The
 * link is what swaps the "Stall / Vendor" select's contents, so it is read off
 * the form rather than off the loaded stall list (which is momentarily empty
 * while a newly-picked map's stalls are fetched).
 */
const linkedToMap = computed(() => store.rallyForm?.festival_map_id != null)

/** Link (or unlink) the rally to a festival map ('' -> not linked). */
function setFestivalMap(value: string): void {
  void store.setFestivalMap(value ? Number(value) : null)
}

/** Point a stamp at one of the linked map's stalls ('' -> no stall). */
function setStall(stampIndex: number, value: string): void {
  store.setStampStall(stampIndex, value ? Number(value) : null)
}

/**
 * Settles both requirements into whole counts no larger than the stamps of that
 * type on the card - the same ceiling the server applies on save, applied here so
 * the field always shows the number that will actually be stored.
 *
 * Deliberately NOT done on every keystroke: clamping in an `@input` handler leaves
 * the typed text on screen whenever the clamped result equals the value already
 * held (the model doesn't change, so nothing re-renders), and the field ends up
 * displaying a requirement that was never saved.
 */
function clampRequirements(): void {
  const f = store.rallyForm
  if (!f) return
  f.required_food = Math.min(foodCount.value, toStampCount(f.required_food))
  f.required_game = Math.min(gameCount.value, toStampCount(f.required_game))
}

// Removing stalls (or switching one between food and game) lowers a ceiling, so
// re-settle the requirements against the new counts instead of showing a number
// the save would silently reduce.
watch([foodCount, gameCount], clampRequirements)

/**
 * Switching to per-type completion seeds the requirements from the stalls on the
 * card, so the mode never starts out requiring nothing of either type - which
 * would finish every card at its first stamp (and is refused on save).
 */
function setCompletionMode(mode: string): void {
  const f = store.rallyForm
  if (!f) return
  f.completion_mode = mode === 'counts' ? 'counts' : 'all'
  if (f.completion_mode === 'counts' && f.required_food === 0 && f.required_game === 0) {
    f.required_food = foodCount.value
    f.required_game = gameCount.value
  }
}

async function save(): Promise<void> {
  if (await store.saveRally()) emit('saved')
}
function cancel(): void {
  store.cancelRallyForm()
  emit('cancel')
}
</script>

<template>
  <AdminPanel>
    <SubPageHeader
      :icon="['fad', 'stamp']"
      :title="`${store.rallyForm && store.rallyForm.id ? 'Edit' : 'New'} Stamp Rally`"
      @back="cancel"
    />
    <template v-if="store.rallyForm">
      <FormField label="Title" required>
        <input
          v-model="store.rallyForm.title"
          placeholder="Event name"
          aria-label="Stamp rally title"
        />
      </FormField>

      <FormRow>
        <FormField label="Available From" help="When the card opens (optional).">
          <input
            v-model="store.rallyForm.available_from"
            type="datetime-local"
            aria-label="Available from"
          />
        </FormField>
        <FormField label="Available To" help="When the card closes (optional).">
          <input
            v-model="store.rallyForm.available_to"
            type="datetime-local"
            aria-label="Available to"
          />
        </FormField>
      </FormRow>

      <FormField
        label="Public sign-up"
        help="Off by default. When on, this rally is listed publicly and anyone can issue themselves a card - one per character name. If a Garapon is linked to it, signing up also issues that drawing link. Leave off for a rally whose cards staff hand out."
      >
        <label class="checkbox-inline">
          <input v-model="store.rallyForm.public_signup" type="checkbox" />
          Let participants sign themselves up
        </label>
      </FormField>

      <FormField label="Details">
        <MarkdownEditor
          v-model="store.rallyForm.details"
          min-height="100px"
          placeholder="Describe the stamp rally (supports markdown)"
        />
      </FormField>

      <FormField label="How to Redeem" help="Shown to participants once their card is complete.">
        <MarkdownEditor
          v-model="store.rallyForm.redeem_instructions"
          min-height="100px"
          placeholder="How to claim the prizes (supports markdown)"
        />
      </FormField>

      <FormField
        label="Where to Redeem"
        help="Usually a screenshot of where in the area to go to redeem the completed card. Shown to participants alongside the redeem instructions once their card is complete."
      >
        <ImagePicker v-model="store.rallyForm.redeem_image" />
      </FormField>

      <FormRow>
        <FormField
          label="Stamp Card Image"
          help="The full designed card - its frame, slot placeholders, stall labels, and any prize panel are all part of this image. Earned stamp/prize art is overlaid on top."
        >
          <ImagePicker v-model="store.rallyForm.card_image" />
        </FormField>
        <FormField
          label="Not-Stamped Overlay (optional)"
          help="Drawn over uncollected stamp + locked prize slots. Leave empty if your card already marks them (e.g. with “?”)."
        >
          <ImagePicker v-model="store.rallyForm.not_stamped_image" />
        </FormField>
      </FormRow>

      <FormField
        label="Festival Map"
        help="Link this rally to a festival map and each stamp names one of its stalls instead of an affiliate. The map then badges those stalls for visitors."
      >
        <select
          :value="store.rallyForm.festival_map_id ?? ''"
          aria-label="Linked festival map"
          @change="setFestivalMap(($event.target as HTMLSelectElement).value)"
        >
          <option value="">Not linked to a map</option>
          <option v-for="m in store.festivalMaps" :key="m.id" :value="m.id">{{ m.title }}</option>
        </select>
      </FormField>

      <FormField
        label="Card Completion"
        help="What finishes a card. Requiring a number of each type leaves the rest of the stalls optional - e.g. 3 of 5 food stalls and 3 of 5 games."
      >
        <select
          :value="store.rallyForm.completion_mode"
          aria-label="Card completion rule"
          @change="setCompletionMode(($event.target as HTMLSelectElement).value)"
        >
          <option value="all">Every stamp on the card</option>
          <option value="counts">A number of food stamps and game stamps</option>
        </select>
      </FormField>

      <FormRow v-if="store.rallyForm.completion_mode === 'counts'">
        <FormField label="Food Stamps Required" :help="`${foodCount} food stamp(s) on this card.`">
          <input
            v-model.number="store.rallyForm.required_food"
            type="number"
            min="0"
            :max="foodCount"
            aria-label="Food stamps required"
            @blur="clampRequirements"
          />
        </FormField>
        <FormField label="Game Stamps Required" :help="`${gameCount} game stamp(s) on this card.`">
          <input
            v-model.number="store.rallyForm.required_game"
            type="number"
            min="0"
            :max="gameCount"
            aria-label="Game stamps required"
            @blur="clampRequirements"
          />
        </FormField>
      </FormRow>

      <!-- Placement editor -->
      <h3 class="section-heading mt-16">
        <font-awesome-icon :icon="['fad', 'stamp']" /> Stamps &amp; Prizes
      </h3>
      <div class="flex-toolbar flex-end mb-10">
        <button class="btn-neutral btn-sm" @click="addStamp('food')">
          <font-awesome-icon :icon="['fas', 'plus']" /> Add Food Stamp
        </button>
        <button class="btn-neutral btn-sm" @click="addStamp('game')">
          <font-awesome-icon :icon="['fas', 'plus']" /> Add Game Stamp
        </button>
        <button class="btn-neutral btn-sm" @click="addPrize">
          <font-awesome-icon :icon="['fas', 'plus']" /> Add Prize
        </button>
      </div>

      <PlacementEditor
        :card-image="store.rallyForm.card_image"
        :items="items"
        :selected-key="selectedKey"
        @select="selectedKey = $event"
        @update="applyUpdate"
      />

      <!-- Selected item panel -->
      <div v-if="selected" class="subpanel mt-16">
        <div class="flex-toolbar flex-between mb-10">
          <h4 class="section-heading no-margin">
            <font-awesome-icon :icon="['fad', selected.kind === 'prize' ? 'gift' : 'stamp']" />
            {{
              selected.kind === 'prize'
                ? `Prize ${selected.index + 1}`
                : `${stampTypeLabel(selected.stamp.stamp_type)} ${selected.index + 1}`
            }}
          </h4>
          <button class="btn-danger btn-sm" @click="removeSelected">
            <font-awesome-icon :icon="['fas', 'trash']" /> Remove
          </button>
        </div>

        <!-- Stamp settings -->
        <template v-if="selected.kind === 'stamp'">
          <FormRow>
            <FormField
              label="Stall / Vendor"
              :help="
                linkedToMap
                  ? 'A stall on the linked festival map. A pitch that changes hands between days lists each day separately. Picking one takes its affiliate and seeds the stamp type.'
                  : `Recorded in the View Logs (the card's stall labels are part of the card art).`
              "
            >
              <select
                v-if="linkedToMap"
                :value="selected.stamp.occupant_id ?? ''"
                aria-label="Map stall"
                @change="setStall(selected.index, ($event.target as HTMLSelectElement).value)"
              >
                <option value="">Pick a stall on the map</option>
                <option v-for="o in store.mapStalls" :key="o.id" :value="o.id">
                  {{ occupantListLabel(o) }} ({{ stallCaption(o) || 'Other' }})
                </option>
              </select>
              <select
                v-else
                :value="selected.stamp.affiliate_id ?? ''"
                aria-label="Stall affiliate"
                @change="setAffiliate(selected.index, ($event.target as HTMLSelectElement).value)"
              >
                <option value="">Senpan Tea House (default)</option>
                <option v-for="a in store.affiliates" :key="a.id" :value="a.id">
                  {{ a.name }}
                </option>
              </select>
            </FormField>
            <FormField label="Password" help="Participants enter this to collect the stamp.">
              <input
                v-model="selected.stamp.password"
                placeholder="Stamp password"
                aria-label="Stamp password"
              />
            </FormField>
            <FormField label="Stamp Type" help="What a completion requirement counts it as.">
              <select v-model="selected.stamp.stamp_type" aria-label="Stamp type">
                <option v-for="t in STAMP_TYPES" :key="t.value" :value="t.value">
                  {{ t.label }}
                </option>
              </select>
            </FormField>
          </FormRow>
          <FormField label="Stamp Image" help="Pick from any image category.">
            <ImagePicker v-model="selected.stamp.image" />
          </FormField>
          <FormRow>
            <FormField label="Active From" help="Optional - defaults to the whole event.">
              <input
                v-model="selected.stamp.active_from"
                type="datetime-local"
                aria-label="Stamp active from"
              />
            </FormField>
            <FormField label="Active To" help="Optional.">
              <input
                v-model="selected.stamp.active_to"
                type="datetime-local"
                aria-label="Stamp active to"
              />
            </FormField>
          </FormRow>
          <label class="checkbox-row">
            <input v-model="selected.stamp.paused" type="checkbox" />
            Paused (temporarily unavailable even within its window)
          </label>
        </template>

        <!-- Prize settings -->
        <template v-else>
          <FormField label="Prize Name">
            <input v-model="selected.prize.name" placeholder="Prize name" aria-label="Prize name" />
          </FormField>
          <FormField label="Prize Image" help="Pick from any image category.">
            <ImagePicker v-model="selected.prize.image" />
          </FormField>
        </template>
      </div>
      <p v-else class="text-muted text-sm mt-10">
        Add a stamp or prize, then click it on the card to position it and edit its settings.
      </p>

      <FormActions align="start">
        <button class="btn-neutral" :disabled="store.savingRally" @click="cancel">Cancel</button>
        <button
          class="btn-confirm"
          :disabled="!store.rallyForm.title.trim() || store.savingRally"
          @click="save"
        >
          <LoadingSpinner v-if="store.savingRally" label="Saving..." />
          <template v-else>Save Stamp Rally</template>
        </button>
      </FormActions>
    </template>
  </AdminPanel>
</template>

<style scoped>
.no-margin {
  margin: 0;
}
.checkbox-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
  font-size: 0.9rem;
}
</style>
