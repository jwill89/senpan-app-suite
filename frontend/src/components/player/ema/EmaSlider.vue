<script setup lang="ts">
/**
 * A labeled range slider for the Ema designer, on the player's `.opacity-slider`
 * look (the stamp opacity control), with the current value beside the track.
 *
 * `update:modelValue` fires on every step for a live preview; `change` fires once
 * the slider is let go, which is when the designer records an undo step - one per
 * drag, not one per pixel.
 */
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    label: string
    min: number
    max: number
    step?: number
    /** Appended to the shown value ("px"). */
    unit?: string
    /** Show a 0-1 value as a percentage. */
    percent?: boolean
  }>(),
  { step: 1, unit: '', percent: false },
)
const emit = defineEmits<{ change: [] }>()
const value = defineModel<number>({ required: true })

/** Highlight fill ending at the thumb's center; the thumb is 16px (see player.css). */
const fill = computed(() => {
  const share = (value.value - props.min) / (props.max - props.min || 1)
  return `calc((100% - 16px) * ${Math.min(1, Math.max(0, share))} + 8px)`
})

const shown = computed(() =>
  props.percent ? `${Math.round(value.value * 100)}%` : `${value.value}${props.unit}`,
)

function onInput(e: Event): void {
  value.value = Number((e.target as HTMLInputElement).value)
}
</script>

<template>
  <div class="opacity-slider ema-slider">
    <span class="label">{{ label }}</span>
    <input
      type="range"
      :min="min"
      :max="max"
      :step="step"
      :value="value"
      :aria-label="label"
      :aria-valuetext="shown"
      :style="{ '--fill': fill }"
      @input="onInput"
      @change="emit('change')"
    />
    <span class="opacity-val">{{ shown }}</span>
  </div>
</template>
