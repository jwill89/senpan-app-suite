<script setup lang="ts">
/**
 * Picks a Discord channel the bot posts to. Shared by every posting system and the
 * Discord page.
 *
 * The list comes from the discord store (the bot's own view of the server), grouped by
 * category the way Discord shows it. Two cases a plain <select> would get wrong:
 *
 * - A stored channel that is no longer in the server (deleted or renamed away) stays
 *   selectable and is flagged, instead of the form silently blanking it on the next save.
 * - When the bot isn't connected (no token yet, or the bot isn't in the server) there is
 *   no list, so the picker becomes a digits-only text box for a pasted channel id.
 */
import { computed, onMounted } from 'vue'
import { useDiscordStore } from '@/stores/discord'

const props = withDefaults(
  defineProps<{
    /** Selected channel id; '' when none. */
    modelValue: string
    /** Text of the empty option. */
    placeholder?: string
    /** Let the empty option be chosen (clearing the channel). */
    allowNone?: boolean
    disabled?: boolean
    /** id for an outside <label for>. */
    id?: string
  }>(),
  { placeholder: 'Choose a channel...', allowNone: false, disabled: false, id: undefined },
)

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const discord = useDiscordStore()

onMounted(() => {
  if (!discord.directoryAvailable && !discord.loadingDirectory) void discord.loadDirectory()
})

/** The channels grouped by category, in the order the server lists them. */
const groups = computed(() => {
  const out: { label: string; channels: typeof discord.channels }[] = []
  for (const c of discord.channels) {
    const label = c.category || 'No category'
    let group = out.find((g) => g.label === label)
    if (!group) {
      group = { label, channels: [] }
      out.push(group)
    }
    group.channels.push(c)
  }
  return out
})

/** A stored id the server no longer has. */
const missing = computed(() => {
  const id = props.modelValue.trim()
  if (!id || !discord.directoryAvailable) return ''
  return discord.channels.some((c) => c.id === id) ? '' : id
})

function onTyped(value: string): void {
  emit('update:modelValue', value.replace(/\D/g, ''))
}
</script>

<template>
  <div class="discord-picker">
    <select
      v-if="discord.directoryAvailable"
      :id="id"
      :value="modelValue"
      :disabled="disabled"
      @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
    >
      <option value="" :disabled="!allowNone">{{ placeholder }}</option>
      <option v-if="missing" :value="missing">Channel {{ missing }} (not in the server now)</option>
      <optgroup v-for="g in groups" :key="g.label" :label="g.label">
        <option v-for="c in g.channels" :key="c.id" :value="c.id">
          #{{ c.name }}{{ c.type === 'announcement' ? ' (announcement)' : '' }}
        </option>
      </optgroup>
    </select>
    <template v-else>
      <input
        :id="id"
        type="text"
        inputmode="numeric"
        autocomplete="off"
        :value="modelValue"
        :disabled="disabled"
        placeholder="Channel ID"
        @input="onTyped(($event.target as HTMLInputElement).value)"
      />
      <small class="field-help">
        {{ discord.loadingDirectory ? 'Loading channels...' : discord.directoryProblem }}
        Until then, paste a channel ID (Discord Developer Mode, right-click the channel, Copy
        Channel ID).
      </small>
    </template>
  </div>
</template>
