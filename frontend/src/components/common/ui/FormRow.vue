<script setup lang="ts">
/**
 * Horizontal group of form fields that wraps to a stacked layout on narrow
 * widths. Direct `.field` children share the row width equally (see the
 * `.form-row` rule in app.css), replacing the old inline
 * `style="flex:1;min-width:…"` patches.
 */
</script>

<template>
  <div class="form-row"><slot /></div>
</template>
