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
defineProps<{
  title?: string
  /** FontAwesome classes for the heading icon, e.g. "fa-duotone fa-gear". */
  icon?: string
}>()
</script>

<template>
  <div class="admin-panel">
    <h3 v-if="title"><i v-if="icon" :class="icon"></i>{{ title }}</h3>
    <slot />
  </div>
</template>
