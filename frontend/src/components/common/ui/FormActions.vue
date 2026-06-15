<script setup lang="ts">
/**
 * Button row for a form's actions. Defaults to right-aligned; pass `align` to
 * left-align (`start`) or push items to opposite ends (`between`). Replaces the
 * ad-hoc `.btns.flex-toolbar` action rows.
 */
withDefaults(defineProps<{ align?: 'start' | 'end' | 'between' }>(), {
  align: 'end',
})
</script>

<template>
  <div class="form-actions" :class="`form-actions--${align}`"><slot /></div>
</template>
