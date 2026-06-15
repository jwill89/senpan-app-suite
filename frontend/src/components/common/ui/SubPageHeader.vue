<script setup lang="ts">
/**
 * Header for a manager sub-page (new / edit / manage-categories screen): the
 * title on the left and a "Back" button on the right. Emits `back` when Back is
 * clicked — the parent sets its `screen` ref back to the list. Title via the
 * `title` prop (+ optional `icon`) or the default slot for markup.
 */
import type { IconPrefix } from '@fortawesome/fontawesome-svg-core'

defineProps<{
  title?: string
  /** FontAwesome icon for the heading as `[prefix, name]`, e.g. `['fad', 'plus']`. */
  icon?: [IconPrefix, string]
}>()
defineEmits<{ back: [] }>()
</script>

<template>
  <div class="subpage-header">
    <h3 v-if="title || $slots.default">
      <slot><font-awesome-icon v-if="icon" :icon="icon" />{{ title }}</slot>
    </h3>
    <button class="btn-neutral btn-sm" @click="$emit('back')">
      <font-awesome-icon :icon="['fas', 'arrow-left']" /> Back
    </button>
  </div>
</template>
