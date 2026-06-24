<script setup lang="ts">
/**
 * Centered placeholder for empty lists and tables. The message comes from the
 * `text` prop or the default slot; an optional `icon` renders a large muted
 * glyph above it, `hint` (prop or `#hint` slot) adds a secondary line, and the
 * `#action` slot holds an optional call-to-action button. Replaces the ad-hoc
 * `.msg-block` placeholders.
 */
import type { IconPrefix } from '@fortawesome/fontawesome-svg-core'

defineProps<{
  text?: string
  /** FontAwesome icon for a leading glyph as `[prefix, name]`, e.g. `['fad', 'inbox']`. */
  icon?: [IconPrefix, string]
  /** Secondary hint line beneath the message. Use the #hint slot for markup. */
  hint?: string
}>()
</script>

<template>
  <div class="empty-state">
    <font-awesome-icon v-if="icon" :icon="icon" class="empty-state__icon" />
    <div class="empty-state__title"><slot>{{ text }}</slot></div>
    <div v-if="hint || $slots.hint" class="empty-state__hint">
      <slot name="hint">{{ hint }}</slot>
    </div>
    <div v-if="$slots.action" class="empty-state__action"><slot name="action" /></div>
  </div>
</template>
