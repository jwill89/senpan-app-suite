<script setup lang="ts">
/**
 * Picks who a Discord post pings: nobody, @everyone, or one of the server's roles.
 * Shared by every posting system.
 *
 * The value is the format the server stores: '' (no ping), 'everyone', or
 * 'role:<role id>'. Roles come from the discord store, highest first, like the server's
 * own role list. As with ChannelPicker, a stored role the server no longer has stays
 * selectable and flagged, and without a connected bot the role is typed as an id.
 */
import { computed, onMounted } from 'vue'
import { useDiscordStore } from '@/stores/discord'

const props = withDefaults(
  defineProps<{
    /** '' | 'everyone' | 'role:<id>' */
    modelValue: string
    disabled?: boolean
    /** id for an outside <label for>. */
    id?: string
  }>(),
  { disabled: false, id: undefined },
)

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const discord = useDiscordStore()

onMounted(() => {
  if (!discord.directoryAvailable && !discord.loadingDirectory) void discord.loadDirectory()
})

/** The role id in the current value, or ''. */
const roleId = computed(() =>
  props.modelValue.startsWith('role:') ? props.modelValue.slice(5) : '',
)

/** A stored role the server no longer has. */
const missing = computed(() => {
  if (!roleId.value || !discord.directoryAvailable) return ''
  return discord.roles.some((r) => r.id === roleId.value) ? '' : roleId.value
})

/** Offline choice: what kind of ping, with the role id typed beside it. */
const offlineKind = computed(() =>
  props.modelValue === 'everyone' ? 'everyone' : props.modelValue.startsWith('role:') ? 'role' : '',
)

function onOfflineKind(kind: string): void {
  emit('update:modelValue', kind === 'role' ? `role:${roleId.value}` : kind)
}

function onTypedRole(value: string): void {
  emit('update:modelValue', `role:${value.replace(/\D/g, '')}`)
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
      <option value="">No ping</option>
      <option value="everyone">@everyone</option>
      <option v-if="missing" :value="`role:${missing}`">
        Role {{ missing }} (not in the server now)
      </option>
      <optgroup v-if="discord.roles.length" label="Roles">
        <option v-for="r in discord.roles" :key="r.id" :value="`role:${r.id}`">
          @{{ r.name }}{{ r.managed ? ' (bot role)' : '' }}
        </option>
      </optgroup>
    </select>
    <template v-else>
      <select
        :id="id"
        :value="offlineKind"
        :disabled="disabled"
        @change="onOfflineKind(($event.target as HTMLSelectElement).value)"
      >
        <option value="">No ping</option>
        <option value="everyone">@everyone</option>
        <option value="role">A role (type its ID)</option>
      </select>
      <input
        v-if="offlineKind === 'role'"
        type="text"
        inputmode="numeric"
        autocomplete="off"
        :value="roleId"
        :disabled="disabled"
        placeholder="Role ID"
        aria-label="Role ID"
        @input="onTypedRole(($event.target as HTMLInputElement).value)"
      />
      <small class="field-help">
        {{ discord.loadingDirectory ? 'Loading roles...' : discord.directoryProblem }}
      </small>
    </template>
  </div>
</template>
