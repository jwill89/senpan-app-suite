<script setup lang="ts">
/**
 * Admin Ema plaque board create/edit form: which festival it belongs to, the
 * blank plaque template, the do-it-yourself instructions and where those plaques
 * are posted, which of the three ways to write a plaque are offered, and the days
 * plaques are taken on - each with the Discord webhook its approved site plaques
 * post to.
 *
 * Hosted as a Back sub-page of the Ema manager (EmaPlaquesTab): it emits `saved`
 * with the board's id on a successful save, and `cancel` to go back.
 */
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import ImagePicker from '@/components/common/ui/ImagePicker.vue'
import { useEmaStore } from '@/stores/ema'

const emit = defineEmits<{ saved: [id: number]; cancel: [] }>()
const ema = useEmaStore()

function setFestival(value: string): void {
  if (ema.boardForm) ema.boardForm.festival_map_id = value ? Number(value) : null
}

async function save(): Promise<void> {
  const id = await ema.saveBoard()
  if (id) emit('saved', id)
}

function cancel(): void {
  ema.cancelBoardForm()
  emit('cancel')
}
</script>

<template>
  <AdminPanel>
    <SubPageHeader
      :icon="['fad', 'torii-gate']"
      :title="`${ema.boardForm?.id ? 'Edit' : 'New'} Plaque Board`"
      @back="cancel"
    />
    <template v-if="ema.boardForm">
      <FormField
        label="Festival"
        required
        help="The festival this board belongs to. Its plaques show on that festival's dashboard, and each festival has one board."
      >
        <select
          :value="ema.boardForm.festival_map_id ?? ''"
          aria-label="Festival"
          @change="setFestival(($event.target as HTMLSelectElement).value)"
        >
          <option value="" disabled>Pick a festival...</option>
          <option v-for="m in ema.festivals" :key="m.id" :value="m.id">{{ m.title }}</option>
        </select>
      </FormField>

      <FormField label="Title" help="Shown above the plaque options on the dashboard.">
        <input v-model="ema.boardForm.title" placeholder="Ema Plaques" aria-label="Board title" />
      </FormField>

      <FormField
        label="Plaque Template"
        help="The blank plaque everyone starts from: downloaded for the do-it-yourself option, and drawn on in the site's designer. A transparent PNG works best."
      >
        <ImagePicker v-model="ema.boardForm.template_image" />
      </FormField>

      <FormField label="Ways to Write a Plaque">
        <div class="ema-options">
          <label class="checkbox-inline">
            <input v-model="ema.boardForm.allow_self" type="checkbox" />
            Design it yourself (download the template, post it in Discord)
          </label>
          <label class="checkbox-inline">
            <input v-model="ema.boardForm.allow_site" type="checkbox" />
            Design it on the site (staff approve it, then it posts for them)
          </label>
          <label class="checkbox-inline">
            <input v-model="ema.boardForm.allow_artist" type="checkbox" />
            Ask the artist to make it
          </label>
        </div>
      </FormField>

      <FormField
        v-if="ema.boardForm.allow_artist"
        label="Artist"
        help="Who takes plaque requests. Shown on the request option."
      >
        <input v-model="ema.boardForm.artist_name" placeholder="Tara" aria-label="Artist name" />
      </FormField>

      <template v-if="ema.boardForm.allow_self">
        <FormField
          label="Do-It-Yourself Instructions"
          help="How to decorate the template and where to post it."
        >
          <MarkdownEditor
            v-model="ema.boardForm.instructions"
            min-height="100px"
            placeholder="Save the template, decorate it in Paint or Photoshop, and post it in the festival channel..."
          />
        </FormField>
        <FormField
          label="Discord Link"
          help="The channel or invite self-made plaques are posted to."
        >
          <input
            v-model="ema.boardForm.discord_link"
            type="url"
            placeholder="https://discord.com/channels/..."
            aria-label="Discord link"
          />
        </FormField>
      </template>

      <FormField
        label="Days"
        required
        help="When plaques are taken - usually one row per festival day. Each person gets one plaque per day, whichever way they make it. A plaque designed on the site posts to its day's webhook once approved. Days can't overlap."
      >
        <div class="repeater">
          <div class="repeater-row ema-window-row text-muted text-xs">
            <span>Label</span>
            <span>Starts</span>
            <span>Ends</span>
            <span>Discord Webhook</span>
            <span></span>
          </div>
          <div
            v-for="(w, i) in ema.boardForm.windows"
            :key="w._uid"
            class="repeater-row ema-window-row"
          >
            <input v-model="w.label" :placeholder="`Day ${i + 1}`" aria-label="Day label" />
            <input v-model="w.start_at" type="datetime-local" aria-label="Starts" />
            <input v-model="w.end_at" type="datetime-local" aria-label="Ends" />
            <input
              v-model="w.webhook_url"
              type="url"
              placeholder="https://discord.com/api/webhooks/..."
              aria-label="Discord webhook for this day"
              autocomplete="off"
            />
            <button
              type="button"
              class="btn-danger btn-sm"
              :disabled="ema.boardForm.windows.length <= 1"
              aria-label="Remove day"
              title="Remove day"
              @click="ema.removeWindow(i)"
            >
              &times;
            </button>
          </div>
          <button type="button" class="btn-neutral btn-sm mt-8" @click="ema.addWindow()">
            <font-awesome-icon :icon="['fas', 'plus']" /> Add Day
          </button>
        </div>
      </FormField>

      <FormActions align="start">
        <button type="button" class="btn-neutral" :disabled="ema.saving" @click="cancel">
          Cancel
        </button>
        <button
          type="button"
          class="btn-confirm"
          :disabled="!ema.boardForm.festival_map_id || ema.saving"
          @click="save"
        >
          <LoadingSpinner v-if="ema.saving" label="Saving..." />
          <template v-else>Save Board</template>
        </button>
      </FormActions>
    </template>
  </AdminPanel>
</template>
