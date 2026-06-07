<script setup lang="ts">
/**
 * Admin Raffle create/edit form. Markdown fields (description, rules, sign-up
 * instructions), entry limits/cost, availability window, and prize-image upload.
 * Rendered at the `/admin/raffles/new` route.
 */
import { useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useRafflesStore } from '@/stores/raffles'
import { assetUrl } from '@/lib/assets'

const router = useRouter()
const raffles = useRafflesStore()

/** Save the form; on success return to the open raffles list. */
async function save(): Promise<void> {
  const ok = await raffles.saveRaffle()
  if (ok) router.push({ name: 'admin-raffle-open' })
}
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel">
      <h3 class="mb-12">
        <i class="fa-solid fa-plus"></i>
        {{ raffles.raffleForm && raffles.raffleForm.id ? 'Edit' : 'New' }} Raffle
      </h3>
      <template v-if="raffles.raffleForm">
        <div class="field mb-10">
          <label class="field-label">Title *</label>
          <input
            v-model="raffles.raffleForm.title"
            placeholder="Raffle Title"
            class="field-input-full"
            aria-label="Raffle title"
          />
        </div>
        <div class="field mb-10">
          <label class="field-label">Description (Markdown)</label>
          <textarea
            v-model="raffles.raffleForm.description"
            rows="3"
            class="form-textarea"
            aria-label="Description"
          ></textarea>
        </div>
        <div class="field mb-10">
          <label class="field-label">Rules (Markdown)</label>
          <textarea
            v-model="raffles.raffleForm.rules"
            rows="3"
            class="form-textarea"
            aria-label="Rules"
          ></textarea>
        </div>
        <div class="field mb-10">
          <label class="field-label">Sign-Up Instructions (Markdown)</label>
          <textarea
            v-model="raffles.raffleForm.signup_instructions"
            rows="3"
            class="form-textarea"
            aria-label="Sign-up instructions"
          ></textarea>
        </div>
        <div class="flex-row mb-10">
          <div class="field" style="flex: 1; min-width: 120px">
            <label class="field-label">Max Entries Per Person</label>
            <input
              v-model.number="raffles.raffleForm.max_entries"
              type="number"
              min="1"
              class="field-input-full"
              aria-label="Max entries per person"
            />
          </div>
          <div class="field" style="flex: 1; min-width: 120px">
            <label class="field-label">Cost Per Entry</label>
            <input
              v-model.number="raffles.raffleForm.cost_per_entry"
              type="number"
              min="0"
              step="any"
              class="field-input-full"
              aria-label="Cost per entry"
            />
          </div>
        </div>
        <div class="flex-row mb-10">
          <div class="field" style="flex: 1; min-width: 180px">
            <label class="field-label">Available From</label>
            <input
              v-model="raffles.raffleForm.available_from"
              type="datetime-local"
              class="field-input-full"
              aria-label="Available from"
            />
          </div>
          <div class="field" style="flex: 1; min-width: 180px">
            <label class="field-label">Available To</label>
            <input
              v-model="raffles.raffleForm.available_to"
              type="datetime-local"
              class="field-input-full"
              aria-label="Available to"
            />
          </div>
        </div>
        <div class="field mb-12">
          <label class="field-label">Prize Image</label>
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
              class="btn-ghost btn-sm"
              style="margin-left: 8px"
              @click="raffles.raffleForm.prize_image = ''"
            >
              Remove
            </button>
          </div>
        </div>
        <div class="btns flex-toolbar">
          <button class="btn-ghost" :disabled="raffles.savingRaffle" @click="raffles.cancelRaffleForm()">
            Cancel
          </button>
          <button
            class="btn-primary"
            :disabled="!raffles.raffleForm.title.trim() || raffles.savingRaffle"
            @click="save"
          >
            <LoadingSpinner v-if="raffles.savingRaffle" label="Saving…" />
            <template v-else>Save Raffle</template>
          </button>
        </div>
      </template>
    </div>
  </div>
</template>
