<script setup lang="ts">
/**
 * Admin Affiliate create/edit form. Name, a repeatable owners list, location, a
 * single timezone for the affiliate, a repeatable opening-hours editor (optional
 * label + start + optional end), markdown details, and two image pickers — the
 * logo (the "Affiliate Logos" category) and the establishment screenshot (the
 * "Affiliate Images" category).
 *
 * Hosted as a Back sub-page of the Affiliates manager (AffiliatesTab): it emits
 * `saved` on a successful save and `cancel` to return to the list.
 */
import { onMounted } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import ImagePicker from '@/components/common/ui/ImagePicker.vue'
import { useAffiliatesStore } from '@/stores/affiliates'
import { supportedTimezones } from '@/lib/constants'

const emit = defineEmits<{ saved: []; cancel: [] }>()
const affiliates = useAffiliatesStore()

const timezones = supportedTimezones()

// Load the reusable logo + screenshot images for the pickers.
onMounted(() => affiliates.loadPickerImages())

async function save(): Promise<void> {
  if (await affiliates.saveAffiliate()) emit('saved')
}
function cancel(): void {
  affiliates.cancelAffiliateForm()
  emit('cancel')
}
</script>

<template>
  <AdminPanel>
    <SubPageHeader
      :icon="['fad', 'handshake']"
      :title="`${affiliates.affiliateForm && affiliates.affiliateForm.id ? 'Edit' : 'New'} Affiliate`"
      @back="cancel"
    />
    <template v-if="affiliates.affiliateForm">
      <FormField label="Name" required>
        <input
          v-model="affiliates.affiliateForm.name"
          placeholder="Establishment name"
          aria-label="Affiliate name"
        />
      </FormField>

      <!-- Owners (repeatable) -->
      <FormField label="Owner(s)" help="Add one or more owners.">
        <div class="repeater">
          <div
            v-for="(_, i) in affiliates.affiliateForm.owners"
            :key="i"
            class="repeater-row owner-row"
          >
            <input
              v-model="affiliates.affiliateForm.owners[i]"
              placeholder="Owner name"
              aria-label="Owner name"
            />
            <button
              class="btn-danger btn-sm"
              :disabled="affiliates.affiliateForm.owners.length <= 1"
              aria-label="Remove owner"
              title="Remove owner"
              @click="affiliates.removeOwner(i)"
            >
              &times;
            </button>
          </div>
          <button class="btn-neutral btn-sm mt-8" @click="affiliates.addOwner()">
            <font-awesome-icon :icon="['fas', 'plus']" /> Add Owner
          </button>
        </div>
      </FormField>

      <FormField label="Location">
        <input
          v-model="affiliates.affiliateForm.location"
          placeholder="Location"
          aria-label="Location"
        />
      </FormField>

      <FormField label="Timezone" help="Anchors all the opening-hours times below.">
        <select v-model="affiliates.affiliateForm.timezone" aria-label="Timezone">
          <option v-for="tz in timezones" :key="tz" :value="tz">{{ tz }}</option>
        </select>
      </FormField>

      <!-- Opening hours (repeatable) -->
      <FormField
        label="Time(s) Open"
        help="Each row has an optional label (e.g. “Mon–Fri”), a start time, and an optional end time."
      >
        <div class="repeater">
          <div class="repeater-row hour-row hour-row-head text-dim text-xs">
            <span>Label</span>
            <span>Start</span>
            <span>End</span>
            <span></span>
          </div>
          <div
            v-for="(h, i) in affiliates.affiliateForm.hours"
            :key="i"
            class="repeater-row hour-row"
          >
            <input v-model="h.label" placeholder="e.g. Mon–Fri" aria-label="Hours label" />
            <input v-model="h.start" type="time" aria-label="Start time" />
            <input v-model="h.end" type="time" aria-label="End time (optional)" />
            <button
              class="btn-danger btn-sm"
              :disabled="affiliates.affiliateForm.hours.length <= 1"
              aria-label="Remove hours row"
              title="Remove hours row"
              @click="affiliates.removeHour(i)"
            >
              &times;
            </button>
          </div>
          <button class="btn-neutral btn-sm mt-8" @click="affiliates.addHour()">
            <font-awesome-icon :icon="['fas', 'plus']" /> Add Time
          </button>
        </div>
      </FormField>

      <FormField label="Details">
        <MarkdownEditor
          v-model="affiliates.affiliateForm.details"
          min-height="120px"
          placeholder="Describe the affiliate (supports markdown — bold, italics, lists, links…)"
        />
      </FormField>

      <FormField
        label="Logo"
        help="Pick from the “Affiliate Logos” image category. Upload new images on the System → Images page."
      >
        <ImagePicker v-model="affiliates.affiliateForm.logo" :images="affiliates.logoImages" />
      </FormField>

      <FormField
        label="Establishment Screenshot"
        help="Pick from the “Affiliate Images” image category. Upload new images on the System → Images page."
      >
        <ImagePicker
          v-model="affiliates.affiliateForm.screenshot"
          :images="affiliates.screenshotImages"
        />
      </FormField>

      <FormActions align="start">
        <button class="btn-neutral" :disabled="affiliates.savingAffiliate" @click="cancel">
          Cancel
        </button>
        <button
          class="btn-confirm"
          :disabled="!affiliates.affiliateForm.name.trim() || affiliates.savingAffiliate"
          @click="save"
        >
          <LoadingSpinner v-if="affiliates.savingAffiliate" label="Saving…" />
          <template v-else>Save Affiliate</template>
        </button>
      </FormActions>
    </template>
  </AdminPanel>
</template>

<style scoped>
.repeater {
  display: flex;
  flex-direction: column;
  gap: 8px;
  overflow-x: auto;
}
.repeater-row {
  display: grid;
  align-items: center;
  column-gap: 8px;
}
/* Owner: a single growing text input + the small delete button. */
.owner-row {
  grid-template-columns: minmax(140px, 1fr) 46px;
}
/* Hours: label + start + end + delete, header and rows sharing one template. */
.hour-row {
  grid-template-columns: minmax(120px, 1fr) 120px 120px 46px;
}
.repeater-row > input {
  width: 100%;
  min-width: 0;
}
</style>
