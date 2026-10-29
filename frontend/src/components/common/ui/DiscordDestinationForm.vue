<script setup lang="ts">
/**
 * Where a system posts in Discord: its channel, who to ping, and what to do with the
 * previous post when it posts again. Shared by the tea rooms, affiliates and book club
 * pages; each passes the repost modes it offers (a reading list can't be edited in place).
 *
 * Edits a local draft, so Cancel leaves the saved destination untouched.
 */
import { computed, reactive, watch } from 'vue'
import FormField from './FormField.vue'
import FormActions from './FormActions.vue'
import ChannelPicker from './ChannelPicker.vue'
import RolePicker from './RolePicker.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { DiscordDestination, RepostMode } from '@/types/api'
import type { RepostOption } from '@/lib/discordPosts'

const props = withDefaults(
  defineProps<{
    destination: DiscordDestination | null
    options: RepostOption[]
    saving?: boolean
    /** A prefix for the field ids, unique on the page. */
    idPrefix?: string
  }>(),
  { saving: false, idPrefix: 'discord-dest' },
)

const emit = defineEmits<{
  save: [dest: { channel_id: string; mention: string; repost_mode: RepostMode }]
  cancel: []
}>()

const draft = reactive<{ channel_id: string; mention: string; repost_mode: RepostMode }>({
  channel_id: '',
  mention: '',
  repost_mode: 'new',
})

watch(
  () => props.destination,
  (d) => {
    draft.channel_id = d?.channel_id ?? ''
    draft.mention = d?.mention ?? ''
    const mode = props.options.find((o) => o.value === d?.repost_mode) ?? props.options.at(0)
    draft.repost_mode = mode?.value ?? 'new'
  },
  { immediate: true },
)

const modeHelp = computed(
  () => props.options.find((o) => o.value === draft.repost_mode)?.help ?? '',
)
</script>

<template>
  <div>
    <FormField label="Channel" :html-for="`${idPrefix}-channel`">
      <ChannelPicker
        :id="`${idPrefix}-channel`"
        v-model="draft.channel_id"
        allow-none
        placeholder="No channel"
      />
    </FormField>
    <FormField
      label="Ping"
      :html-for="`${idPrefix}-ping`"
      help="Who is notified when a new post goes up. Edits never notify anyone."
    >
      <RolePicker :id="`${idPrefix}-ping`" v-model="draft.mention" />
    </FormField>
    <FormField label="When posting again" :help="modeHelp">
      <div class="toggle-group" role="radiogroup" aria-label="When posting again">
        <button
          v-for="o in options"
          :key="o.value"
          type="button"
          class="toggle-btn"
          role="radio"
          :aria-checked="draft.repost_mode === o.value"
          :class="{ 'is-active': draft.repost_mode === o.value }"
          @click="draft.repost_mode = o.value"
        >
          {{ o.label }}
        </button>
      </div>
    </FormField>
    <FormActions>
      <button type="button" class="btn-neutral" :disabled="saving" @click="emit('cancel')">
        Cancel
      </button>
      <button
        type="button"
        class="btn-confirm"
        :disabled="saving"
        @click="emit('save', { ...draft })"
      >
        <LoadingSpinner v-if="saving" label="Saving..." />
        <template v-else>Save</template>
      </button>
    </FormActions>
  </div>
</template>
