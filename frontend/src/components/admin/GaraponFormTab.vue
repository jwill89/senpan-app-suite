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

const emit = defineEmits<{ saved: []; cancel: [] }>()
const garapons = useGaraponsStore()

// Load the reusable grand-prize images (the "Garapon" category on System → Images).
onMounted(() => garapons.loadGrandPrizeImages())

/** Sum of positive prize rates, for the live normalized-% readouts. */
const rateTotal = computed(() =>
  (garapons.garaponForm?.prizes || []).reduce((sum, p) => sum + (p.rate > 0 ? p.rate : 0), 0),
)
/** A prize's odds as a normalized percentage (relative weights). */
function ratePct(rate: number): string {
  if (rateTotal.value <= 0) return '—'
  return `${((Math.max(0, rate) / rateTotal.value) * 100).toFixed(1)}%`
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
            <span class="prize-color-col">Ball</span>
            <span class="prize-rate-col">Rate</span>
            <span class="prize-pct-col">Odds</span>
            <span class="prize-del-col"></span>
          </div>
          <div
            v-for="(p, i) in garapons.garaponForm.prizes"
            :key="i"
            class="prize-row"
          >
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
        help="Pick from the “Garapon” image category. Upload new images on the System → Images page."
      >
        <ImagePicker
          v-model="garapons.garaponForm.grand_prize_image"
          :images="garapons.grandPrizeImages"
        />
      </FormField>

      <FormActions align="start">
        <button class="btn-neutral" :disabled="garapons.savingGarapon" @click="cancel">Cancel</button>
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
}
.prize-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.prize-row-head {
  padding-bottom: 2px;
}
.prize-grand-col {
  flex: 0 0 44px;
  text-align: center;
}
.prize-name-col {
  flex: 2;
  min-width: 120px;
}
.prize-color-col {
  flex: 0 0 52px;
}
.prize-rate-col {
  flex: 0 0 84px;
}
.prize-pct-col {
  flex: 0 0 56px;
  text-align: right;
  font-size: 0.85rem;
}
.prize-del-col {
  flex: 0 0 40px;
  text-align: right;
}
/* Native color input trimmed to a neat square swatch. */
.prize-color {
  width: 100%;
  height: 38px;
  padding: 2px;
  cursor: pointer;
}
</style>
