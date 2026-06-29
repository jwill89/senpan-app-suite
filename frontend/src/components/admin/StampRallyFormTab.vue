<script setup lang="ts">
/**
 * Admin Stamp Rally create/edit form. Event fields (title, availability window, card
 * + not-stamped images, markdown details + redeem instructions), then the visual
 * placement editor where stamps and prizes are dragged/resized/rotated onto the card,
 * with a per-item panel below for the selected item's settings (a stamp's stall +
 * image + password + active window + pause; a prize's name + image).
 *
 * Hosted as a Back sub-page of the Stamp Rally manager: emits `saved` / `cancel`.
 */
import { computed, onMounted, ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormRow from '@/components/common/ui/FormRow.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import ImagePicker from '@/components/common/ui/ImagePicker.vue'
import PlacementEditor, { type PlaceItem } from './PlacementEditor.vue'
import { useStampRalliesStore } from '@/stores/stampRallies'
import type { Placement } from '@/types/api'

const emit = defineEmits<{ saved: []; cancel: [] }>()
const store = useStampRalliesStore()

onMounted(() => store.loadFormSources())

const selectedKey = ref<string | null>(null)

/** Combined stamp + prize items for the placement editor (empty image → placeholder). */
const items = computed<PlaceItem[]>(() => {
  const f = store.rallyForm
  if (!f) return []
  const stamps = f.stamps.map((s, i) => ({
    key: `s${i}`,
    label: `Stamp ${i + 1}`,
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
  if (target) Object.assign(target.placement, placement)
}

/** The currently-selected stamp or prize (for the editing panel). */
const selected = computed(() => {
  const f = store.rallyForm
  const k = selectedKey.value
  if (!f || !k) return null
  const idx = Number(k.slice(1))
  if (k[0] === 's') {
    const stamp = f.stamps[idx]
    return stamp ? { kind: 'stamp' as const, index: idx, stamp } : null
  }
  const prize = f.prizes[idx]
  return prize ? { kind: 'prize' as const, index: idx, prize } : null
})

function addStamp(): void {
  store.addStamp()
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

/** Set a stamp's affiliate from the select ('' → Senpan Tea House default). */
function setAffiliate(stampIndex: number, value: string): void {
  const f = store.rallyForm
  if (!f || !f.stamps[stampIndex]) return
  f.stamps[stampIndex].affiliate_id = value ? Number(value) : null
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
        <input v-model="store.rallyForm.title" placeholder="Event name" aria-label="Stamp rally title" />
      </FormField>

      <FormRow>
        <FormField label="Available From" help="When the card opens (optional).">
          <input v-model="store.rallyForm.available_from" type="datetime-local" aria-label="Available from" />
        </FormField>
        <FormField label="Available To" help="When the card closes (optional).">
          <input v-model="store.rallyForm.available_to" type="datetime-local" aria-label="Available to" />
        </FormField>
      </FormRow>

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

      <FormRow>
        <FormField
          label="Stamp Card Image"
          help="The full designed card — its frame, slot placeholders, stall labels, and any prize panel are all part of this image. Earned stamp/prize art is overlaid on top. From “Stamp Cards”."
        >
          <ImagePicker v-model="store.rallyForm.card_image" :images="store.cardImages" />
        </FormField>
        <FormField
          label="Not-Stamped Overlay (optional)"
          help="Drawn over uncollected stamp + locked prize slots. Leave empty if your card already marks them (e.g. with “?”). From “Stamp Stamps”."
        >
          <ImagePicker v-model="store.rallyForm.not_stamped_image" :images="store.stampImages" />
        </FormField>
      </FormRow>

      <!-- Placement editor -->
      <h3 class="section-heading mt-16">
        <font-awesome-icon :icon="['fad', 'stamp']" /> Stamps &amp; Prizes
      </h3>
      <div class="flex-toolbar flex-end mb-10">
        <button class="btn-neutral btn-sm" @click="addStamp">
          <font-awesome-icon :icon="['fas', 'plus']" /> Add Stamp
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
      <div v-if="selected" class="item-panel mt-16">
        <div class="flex-toolbar flex-between mb-10">
          <h4 class="section-heading no-margin">
            <font-awesome-icon :icon="['fad', selected.kind === 'prize' ? 'gift' : 'stamp']" />
            {{ selected.kind === 'prize' ? `Prize ${selected.index + 1}` : `Stamp ${selected.index + 1}` }}
          </h4>
          <button class="btn-danger btn-sm" @click="removeSelected">
            <font-awesome-icon :icon="['fas', 'trash']" /> Remove
          </button>
        </div>

        <!-- Stamp settings -->
        <template v-if="selected.kind === 'stamp'">
          <FormRow>
            <FormField label="Stall / Vendor" help="Recorded in the View Logs (the card's stall labels are part of the card art).">
              <select
                :value="selected.stamp.affiliate_id ?? ''"
                aria-label="Stall affiliate"
                @change="setAffiliate(selected.index, ($event.target as HTMLSelectElement).value)"
              >
                <option value="">Senpan Tea House (default)</option>
                <option v-for="a in store.affiliates" :key="a.id" :value="a.id">{{ a.name }}</option>
              </select>
            </FormField>
            <FormField label="Password" help="Participants enter this to collect the stamp.">
              <input v-model="selected.stamp.password" placeholder="Stamp password" aria-label="Stamp password" />
            </FormField>
          </FormRow>
          <FormField label="Stamp Image" help="From the “Stamp Stamps” image category.">
            <ImagePicker v-model="selected.stamp.image" :images="store.stampImages" />
          </FormField>
          <FormRow>
            <FormField label="Active From" help="Optional — defaults to the whole event.">
              <input v-model="selected.stamp.active_from" type="datetime-local" aria-label="Stamp active from" />
            </FormField>
            <FormField label="Active To" help="Optional.">
              <input v-model="selected.stamp.active_to" type="datetime-local" aria-label="Stamp active to" />
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
          <FormField label="Prize Image" help="From the “Stamp Prizes” image category.">
            <ImagePicker v-model="selected.prize.image" :images="store.prizeImages" />
          </FormField>
        </template>
      </div>
      <p v-else class="text-dim text-sm mt-10">
        Add a stamp or prize, then click it on the card to position it and edit its settings.
      </p>

      <FormActions align="start">
        <button class="btn-neutral" :disabled="store.savingRally" @click="cancel">Cancel</button>
        <button
          class="btn-confirm"
          :disabled="!store.rallyForm.title.trim() || store.savingRally"
          @click="save"
        >
          <LoadingSpinner v-if="store.savingRally" label="Saving…" />
          <template v-else>Save Stamp Rally</template>
        </button>
      </FormActions>
    </template>
  </AdminPanel>
</template>

<style scoped>
.item-panel {
  background: var(--panel-raised-bg);
  border-radius: var(--radius);
  padding: 14px 16px;
}
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
