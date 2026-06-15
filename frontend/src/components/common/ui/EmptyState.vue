<script setup lang="ts">
/**
 * Centered placeholder for empty lists and tables. Provide the message via the
 * `text` prop or the default slot, with an optional leading FontAwesome `icon`.
 * Replaces the ad-hoc `.msg-block` placeholders.
 */
defineProps<{
  text?: string
  /** FontAwesome classes for a leading icon, e.g. "fa-duotone fa-inbox". */
  icon?: string
}>()
</script>

<template>
  <p class="empty-state">
    <i v-if="icon" :class="icon"></i>
    <slot>{{ text }}</slot>
  </p>
</template>
