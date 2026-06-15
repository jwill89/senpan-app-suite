<script setup lang="ts">
/**
 * Header for a manager sub-page (new / edit / manage-categories screen): the
 * title on the left and a "Back" button on the right. Emits `back` when Back is
 * clicked — the parent sets its `screen` ref back to the list. Title via the
 * `title` prop (+ optional `icon`) or the default slot for markup.
 */
defineProps<{
  title?: string
  /** FontAwesome classes for the heading icon. */
  icon?: string
}>()
defineEmits<{ back: [] }>()
</script>

<template>
  <div class="subpage-header">
    <h3 v-if="title || $slots.default">
      <slot><i v-if="icon" :class="icon"></i>{{ title }}</slot>
    </h3>
    <button class="btn-ghost btn-sm" @click="$emit('back')">
      <i class="fa-solid fa-arrow-left"></i> Back
    </button>
  </div>
</template>
