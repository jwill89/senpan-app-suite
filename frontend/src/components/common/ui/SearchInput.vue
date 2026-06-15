<script setup lang="ts">
/**
 * Search box with a leading magnifying-glass icon, `v-model`'d to a string.
 * Used in manager toolbars (replaces `.ann-search`, `.font-search`, etc.).
 */
withDefaults(
  defineProps<{ modelValue: string; placeholder?: string; ariaLabel?: string }>(),
  { placeholder: 'Search…', ariaLabel: 'Search' },
)
defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <span class="search-input">
    <font-awesome-icon :icon="['fad', 'magnifying-glass']" />
    <input
      :value="modelValue"
      type="search"
      :placeholder="placeholder"
      :aria-label="ariaLabel"
      @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
  </span>
</template>
