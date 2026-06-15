<script setup lang="ts">
/**
 * One item in a manager list. A standard `--panel-raised-bg` row: optional `#media` on
 * the left (cover image, color swatch, icon), the item body in the default slot,
 * and actions pinned to the **far right** via the `#actions` slot.
 *
 * Stack rows inside a `.list-rows` container (flex column with gaps). Replaces
 * the per-page bespoke row chrome (`.ann-card`, `.bc-item-card`, …).
 */
</script>

<template>
  <div class="list-row">
    <div v-if="$slots.media" class="list-row-media"><slot name="media" /></div>
    <div class="list-row-body"><slot /></div>
    <div v-if="$slots.actions" class="list-row-actions"><slot name="actions" /></div>
  </div>
</template>
