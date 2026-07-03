<script setup lang="ts">
/**
 * Admin Garapon create/edit form. Title, a markdown details field, a repeatable
 * prize editor (each row: name + ball color + appearance-rate weight + a
 * grand-prize radio), and the grand-prize image picker (the "Garapon" image
 * category). Appearance rates are relative weights — a live normalized % shows
 * each prize's real odds — so they need not total 100.
 *
 * Hosted as a Back sub-page of the Garapon manager (GaraponTab): it emits `saved`
 * on a successful save and `cancel` to return to the list.
 */
import { computed, onMounted } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import ImagePicker from '@/components/common/ui/ImagePicker.vue'
import { useGaraponsStore } from '@/stores/garapons'
import { rateTotal, ratePct as normalizedPct } from '@/lib/garapon'

const emit = defineEmits<{ saved: []; cancel: [] }>()
const garapons = useGaraponsStore()

// Load the open stamp rallies offered in the "Linked Stamp Rally" picker.
onMounted(() => {
  void garapons.loadStampRallyOptions()
})

/** Set the linked stamp rally from the select ('' → null = not linked). */
function setStampRally(value: string): void {
  if (garapons.garaponForm) garapons.garaponForm.stamp_rally_id = value ? Number(value) : null
}

/** Sum of positive prize weights, for the live normalized-% readouts. */
const total = computed(() => rateTotal(garapons.garaponForm?.prizes || []))
/** A prize's odds as a normalized percentage (relative weights). */
function ratePct(rate: number): string {
  return normalizedPct(rate, total.value)
}

async function save(): Promise<void> {
  if (await garapons.saveGarapon()) emit('saved')
}
function cancel(): void {
  garapons.cancelGaraponForm()
  emit('cancel')
}
</script>

<template>
  <AdminPanel>
    <SubPageHeader
      :icon="['fad', 'ferris-wheel']"
      :title="`${garapons.garaponForm && garapons.garaponForm.id ? 'Edit' : 'New'} Garapon`"
      @back="cancel"
    />
    <template v-if="garapons.garaponForm">
      <FormField label="Title" required>
        <input
          v-model="garapons.garaponForm.title"
          placeholder="Garapon Title"
          aria-label="Garapon title"
        />
      </FormField>

      <FormField
        label="Linked Stamp Rally"
        help="Optional. When linked, every drawing link you issue also issues that participant a stamp card for this rally — sharing the same link. Only open rallies are listed."
      >
        <select
          :value="garapons.garaponForm.stamp_rally_id ?? ''"
          aria-label="Linked stamp rally"
          @change="setStampRally(($event.target as HTMLSelectElement).value)"
        >
          <option value="">None — not linked</option>
          <option v-for="r in garapons.stampRallyOptions" :key="r.id" :value="r.id">
            {{ r.title }}
          </option>
        </select>
      </FormField>

      <FormField label="Details">
        <MarkdownEditor
          v-model="garapons.garaponForm.details"
          min-height="120px"
          placeholder="Describe the event (supports markdown — bold, italics, lists, links…)"
        />
      </FormField>

      <!-- Prizes editor -->
      <FormField
        label="Prizes"
        help="Each prize has a ball color and an appearance rate (a relative weight; the % shows its real odds). Mark exactly one as the grand prize — it gets the picture below."
      >
        <div class="prize-editor">
          <div class="prize-row prize-row-head text-dim text-xs">
            <span class="prize-grand-col">Grand</span>
            <span class="prize-name-col">Name</span>
            <span class="prize-color-col">Ball Color</span>
            <span class="prize-rate-col">Draw Weight</span>
            <span class="prize-pct-col">Odds</span>
            <span class="prize-del-col"></span>
          </div>
          <div v-for="(p, i) in garapons.garaponForm.prizes" :key="p._uid" class="prize-row">
            <span class="prize-grand-col">
              <input
                type="radio"
                name="garapon-grand"
                :checked="p.is_grand"
                aria-label="Mark as grand prize"
                @change="garapons.setGrandPrize(i)"
              />
            </span>
            <span class="prize-name-col">
              <input v-model="p.name" placeholder="Prize name" aria-label="Prize name" />
            </span>
            <span class="prize-color-col">
              <input
                v-model="p.ball_color"
                type="color"
                class="prize-color"
                aria-label="Ball color"
              />
            </span>
            <span class="prize-rate-col">
              <input
                v-model.number="p.rate"
                type="number"
                min="0"
                step="any"
                aria-label="Appearance rate"
              />
            </span>
            <span class="prize-pct-col text-dim">{{ ratePct(p.rate) }}</span>
            <span class="prize-del-col">
              <button
                class="btn-danger btn-sm"
                :disabled="garapons.garaponForm.prizes.length <= 1"
                aria-label="Remove prize"
                title="Remove prize"
                @click="garapons.removePrizeRow(i)"
              >
                &times;
              </button>
            </span>
          </div>
          <button class="btn-neutral btn-sm mt-8" @click="garapons.addPrizeRow()">
            <font-awesome-icon :icon="['fas', 'plus']" /> Add Prize
          </button>
        </div>
      </FormField>

      <FormField
        label="Grand Prize Image"
        help="Pick from any image category. Upload new images on the System → Images page."
      >
        <ImagePicker v-model="garapons.garaponForm.grand_prize_image" />
      </FormField>

      <FormActions align="start">
        <button class="btn-neutral" :disabled="garapons.savingGarapon" @click="cancel">
          Cancel
        </button>
        <button
          class="btn-confirm"
          :disabled="!garapons.garaponForm.title.trim() || garapons.savingGarapon"
          @click="save"
        >
          <LoadingSpinner v-if="garapons.savingGarapon" label="Saving…" />
          <template v-else>Save Garapon</template>
        </button>
      </FormActions>
    </template>
  </AdminPanel>
</template>

<style scoped>
.prize-editor {
  display: flex;
  flex-direction: column;
  gap: 8px;
  /* Scroll horizontally on very narrow screens rather than cramping columns. */
  overflow-x: auto;
}
/* The header row and every field row use ONE shared grid template, so the column
   headers line up exactly with the inputs below regardless of input intrinsic widths. */
.prize-row {
  display: grid;
  /* Last track is wide enough for the small delete button so it doesn't overflow
     and trigger the editor's horizontal scrollbar. */
  grid-template-columns: 48px minmax(110px, 1fr) 76px 96px 56px 46px;
  align-items: center;
  column-gap: 8px;
}
.prize-row-head {
  padding-bottom: 2px;
}
/* Let inputs shrink to their grid track instead of forcing the track wider. */
.prize-row > span {
  min-width: 0;
}
.prize-grand-col {
  text-align: center;
}
.prize-pct-col {
  text-align: right;
  font-size: 0.85rem;
}
.prize-del-col {
  text-align: right;
}
/* Text + number inputs fill their column for consistent alignment. */
.prize-name-col input,
.prize-rate-col input {
  width: 100%;
}
/* Native color input trimmed to a neat swatch that fills its column. */
.prize-color {
  width: 100%;
  height: 38px;
  padding: 2px;
  cursor: pointer;
}
</style>
