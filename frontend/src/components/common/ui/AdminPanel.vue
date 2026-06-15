<script setup lang="ts">
/**
 * Admin content panel — the standard card surface every admin tab sits in.
 * Renders the shared `.admin-panel` chrome with an optional icon + title
 * heading (the heading picks up the panel's bottom-border rule automatically).
 *
 * Provide `title` (and optionally `icon`) for the common "heading + body" panel,
 * or omit `title` and supply your own header markup in the default slot when the
 * heading needs extra controls (e.g. a button on the right).
 */
import type { IconPrefix } from '@fortawesome/fontawesome-svg-core'

defineProps<{
  title?: string
  /** FontAwesome icon for the heading as `[prefix, name]`, e.g. `['fad', 'gear']`
   * (name is `string` since the Pro kit's icon names aren't in FA's `IconName`). */
  icon?: [IconPrefix, string]
}>()
</script>

<template>
  <div class="admin-panel">
    <h3 v-if="title"><font-awesome-icon v-if="icon" :icon="icon" />{{ title }}</h3>
    <slot />
  </div>
</template>
