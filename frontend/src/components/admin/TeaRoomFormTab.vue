<script setup lang="ts">
/**
 * Admin Tea Room create/edit form. Name, room number, per-half-hour gil cost,
 * hashtags, markdown description, the seasonal/open/lockable/discounted flags, an
 * image picker (stored as an absolute URL for the Discord embed), and an embed
 * accent colour.
 *
 * Hosted as a Back sub-page of the Tea Rooms manager (TeaRoomsTab): emits `saved`
 * on a successful save and `cancel` to return to the list.
 */
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormRow from '@/components/common/ui/FormRow.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import ImagePicker from '@/components/common/ui/ImagePicker.vue'
import { useTeaRoomsStore } from '@/stores/teaRooms'

const emit = defineEmits<{ saved: []; cancel: [] }>()
const store = useTeaRoomsStore()

async function save(): Promise<void> {
  if (await store.saveTeaRoom()) emit('saved')
}
function cancel(): void {
  store.cancelTeaRoomForm()
  emit('cancel')
}
</script>

<template>
  <AdminPanel>
    <SubPageHeader
      :icon="['fad', 'booth-curtain']"
      :title="`${store.teaRoomForm && store.teaRoomForm.id ? 'Edit' : 'New'} Tea Room`"
      @back="cancel"
    />
    <template v-if="store.teaRoomForm">
      <FormRow>
        <FormField label="Room name" required>
          <input
            v-model="store.teaRoomForm.name"
            placeholder="e.g. The Jasmine Room"
            aria-label="Room name"
          />
        </FormField>
        <FormField
          label="Room number"
          required
          help="Unique — this is the public key the Carrd site and public API look the room up by."
        >
          <input
            v-model="store.teaRoomForm.room_number"
            placeholder="e.g. 12"
            aria-label="Room number"
          />
        </FormField>
      </FormRow>

      <FormField
        label="Subtitle"
        help="A short second line under the name (any language, e.g. a Japanese phrase)."
      >
        <input
          v-model="store.teaRoomForm.subtitle"
          placeholder="e.g. 「 桃の森の床の間。」"
          aria-label="Subtitle"
        />
      </FormField>

      <div class="flex-row items-start mb-10">
        <FormField
          label="Cost (gil / half hour)"
          style="flex: 1 1 auto; min-width: 200px"
          help="Whole gil. When a room is discounted the embed shows this halved (50% off)."
        >
          <input
            v-model.number="store.teaRoomForm.cost_per_half_hour"
            type="number"
            min="0"
            step="1000"
            placeholder="125000"
            aria-label="Cost per half hour in gil"
          />
        </FormField>
        <FormField
          label="Embed color"
          style="flex: 0 0 auto"
          help="Accent stripe on the Discord embed."
        >
          <div class="room-color-row">
            <input
              v-model="store.teaRoomForm.color"
              type="color"
              class="room-color-input"
              aria-label="Embed accent color"
            />
            <code class="room-color-hex">{{ store.teaRoomForm.color }}</code>
            <button
              type="button"
              class="btn-neutral btn-sm"
              :disabled="store.teaRoomForm.color === '#ff3131'"
              @click="store.teaRoomForm.color = '#ff3131'"
            >
              Reset
            </button>
          </div>
        </FormField>
      </div>

      <FormField
        label="Hashtags"
        help="Space- or comma-separated (the “#” is optional). Shown as subtext on the embed and served by the public API."
      >
        <input
          v-model="store.teaRoomForm.hashtags"
          placeholder="cozy private vip"
          aria-label="Hashtags"
        />
      </FormField>

      <FormField label="Description">
        <MarkdownEditor
          v-model="store.teaRoomForm.description"
          min-height="120px"
          placeholder="Describe the room (supports markdown — bold, italics, lists, links…)"
        />
      </FormField>

      <FormField label="Options">
        <div class="room-flags">
          <label class="checkbox-inline">
            <input v-model="store.teaRoomForm.open" type="checkbox" />
            Open
          </label>
          <label class="checkbox-inline">
            <input v-model="store.teaRoomForm.seasonal" type="checkbox" />
            Seasonal
          </label>
          <label class="checkbox-inline">
            <input v-model="store.teaRoomForm.lockable" type="checkbox" />
            Lockable
          </label>
          <label class="checkbox-inline">
            <input v-model="store.teaRoomForm.discounted" type="checkbox" />
            Discounted (50% off)
          </label>
        </div>
      </FormField>

      <FormField
        label="Image"
        help="Pick from any image category. Upload new images on the System → Images page."
      >
        <ImagePicker v-model="store.teaRoomForm.image" value-key="url" />
      </FormField>

      <FormActions align="start">
        <button class="btn-neutral" :disabled="store.saving" @click="cancel">Cancel</button>
        <button
          class="btn-confirm"
          :disabled="
            !store.teaRoomForm.name.trim() || !store.teaRoomForm.room_number.trim() || store.saving
          "
          @click="save"
        >
          <LoadingSpinner v-if="store.saving" label="Saving…" />
          <template v-else>{{
            store.teaRoomForm.id ? 'Save Changes' : 'Create Tea Room'
          }}</template>
        </button>
      </FormActions>
    </template>
  </AdminPanel>
</template>

<style scoped>
.room-flags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
}
.checkbox-inline {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
.room-color-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.room-color-input {
  width: 48px;
  height: 36px;
  padding: 2px;
  border: 1px solid var(--panel-raised-bg);
  border-radius: 6px;
  background: var(--panel-bg);
  cursor: pointer;
}
.room-color-hex {
  font-family: monospace;
  text-transform: uppercase;
  color: var(--text-muted);
}
</style>
