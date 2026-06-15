<script setup lang="ts">
/**
 * Admin Raffle create/edit form. Markdown fields (description, rules, sign-up
 * instructions), entry limits/cost, availability window, and prize-image upload.
 *
 * Hosted as a Back sub-page of the Raffles manager (RafflesTab): it emits `saved`
 * on a successful save and `cancel`/`back` to return to the list, rather than
 * navigating routes itself.
 */
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormRow from '@/components/common/ui/FormRow.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import { useRafflesStore } from '@/stores/raffles'
import { assetUrl } from '@/lib/assets'

const emit = defineEmits<{ saved: []; cancel: [] }>()
const raffles = useRafflesStore()

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
      icon="fa-duotone fa-plus"
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
          placeholder="Description (supports markdown — bold, italics, lists, links…)"
        />
      </FormField>
      <FormField label="Rules">
        <MarkdownEditor
          v-model="raffles.raffleForm.rules"
          min-height="120px"
          placeholder="Rules (supports markdown)"
        />
      </FormField>
      <FormField label="Sign-Up Instructions">
        <MarkdownEditor
          v-model="raffles.raffleForm.signup_instructions"
          min-height="120px"
          placeholder="Sign-up instructions (supports markdown)"
        />
      </FormField>
      <FormRow>
        <FormField label="Max Entries Per Person">
          <input
            v-model.number="raffles.raffleForm.max_entries"
            type="number"
            min="1"
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
      <FormField label="Prize Image">
        <div class="flex-toolbar">
          <input
            type="file"
            accept="image/*"
            aria-label="Prize image"
            :disabled="raffles.raffleImageUploading"
            @change="raffles.uploadRaffleImage($event)"
          />
          <span v-if="raffles.raffleImageUploading" class="text-dim text-sm">Uploading...</span>
        </div>
        <div v-if="raffles.raffleForm.prize_image" class="mt-8">
          <img
            :src="assetUrl(raffles.raffleForm.prize_image)"
            style="max-width: 200px; max-height: 150px; border-radius: 8px"
            alt="Prize preview"
          />
          <button
            class="btn-neutral btn-sm"
            style="margin-left: 8px"
            @click="raffles.raffleForm.prize_image = ''"
          >
            Remove
          </button>
        </div>
      </FormField>
      <FormActions align="start">
        <button class="btn-neutral" :disabled="raffles.savingRaffle" @click="cancel">Cancel</button>
        <button
          class="btn-confirm"
          :disabled="!raffles.raffleForm.title.trim() || raffles.savingRaffle"
          @click="save"
        >
          <LoadingSpinner v-if="raffles.savingRaffle" label="Saving…" />
          <template v-else>Save Raffle</template>
        </button>
      </FormActions>
    </template>
  </AdminPanel>
</template>
