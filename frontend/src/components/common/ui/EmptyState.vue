<script setup lang="ts">
/**
 * Centered placeholder for empty lists and tables. Provide the message via the
 * `text` prop or the default slot, with an optional leading FontAwesome `icon`.
 * Replaces the ad-hoc `.msg-block` placeholders.
 */
import type { IconPrefix } from '@fortawesome/fontawesome-svg-core'

defineProps<{
  text?: string
  /** FontAwesome icon for a leading glyph as `[prefix, name]`, e.g. `['fad', 'inbox']`. */
  icon?: [IconPrefix, string]
}>()
</script>

<template>
  <p class="empty-state">
    <font-awesome-icon v-if="icon" :icon="icon" />
    <slot>{{ text }}</slot>
  </p>
</template>
