<script setup lang="ts">
/**
 * The home-world field, shared by every form that records a participant.
 *
 * Four systems store a person - custom cards, raffle entries, stamp rally cards
 * and garapon links - and they only recognize each other's records if the world is
 * spelled identically. A free-text box cannot promise that: "Gilgamesh", "gilga",
 * and a typo are three different people to a lookup, and in a raffle they split one
 * entrant's tickets across separate rows. Picking from the list removes the
 * question.
 *
 * `allowUnknown` keeps an existing value that is not on the list selectable rather
 * than silently blanking it - a record written before the picker existed, or one a
 * staff member typed by hand, has to survive being edited on a form that would not
 * offer that value today.
 */
import { computed } from 'vue'
import { FF14_WORLDS } from '@/lib/constants'

const props = withDefaults(
  defineProps<{
    /** Selected world; '' when nothing is chosen yet. */
    modelValue: string
    label?: string
    /** Placeholder shown while nothing is selected. */
    placeholder?: string
    disabled?: boolean
    /** Keep a stored value that is not in FF14_WORLDS selectable. */
    allowUnknown?: boolean
  }>(),
  {
    label: 'World',
    placeholder: 'Select your world...',
    disabled: false,
    allowUnknown: true,
  },
)

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

/**
 * A stored world the list no longer offers (a retired world, a hand-typed one, or
 * anything predating the picker). Surfaced as its own option so opening the form
 * does not quietly discard it.
 */
const unknownValue = computed(() => {
  const current = props.modelValue.trim()
  if (!current || !props.allowUnknown) return ''
  const known = FF14_WORLDS.some((dc) => dc.worlds.includes(current))
  return known ? '' : current
})
</script>

<template>
  <div class="field">
    <label class="field-label">{{ label }}</label>
    <select
      :value="modelValue"
      :disabled="disabled"
      :aria-label="label"
      @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
    >
      <option value="" disabled>{{ placeholder }}</option>
      <option v-if="unknownValue" :value="unknownValue">{{ unknownValue }} (not listed)</option>
      <optgroup v-for="dc in FF14_WORLDS" :key="dc.name" :label="`${dc.name} (${dc.region})`">
        <option v-for="w in dc.worlds" :key="w" :value="w">{{ w }}</option>
      </optgroup>
    </select>
  </div>
</template>
