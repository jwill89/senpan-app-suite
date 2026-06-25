<script setup lang="ts">
/**
 * A round colored chip standing in for a Garapon prize ball — the single source
 * of truth for that swatch, shared by the admin manager and the public player
 * view (it previously lived as near-identical scoped CSS in both, at clashing
 * sizes). The `color` is any CSS color; `size` picks one of three fixed footprints.
 */
withDefaults(defineProps<{ color: string; size?: 'sm' | 'md' | 'lg' }>(), { size: 'md' })
</script>

<template>
  <span class="ball-swatch" :class="`ball-swatch-${size}`" :style="{ background: color }"></span>
</template>

<style scoped>
.ball-swatch {
  display: inline-block;
  /* Never grow or shrink — stays a circle even as a flex item (e.g. the grand
     prize row), where it would otherwise deform into an ellipse. */
  flex: none;
  border-radius: 50%;
  border: 1px solid var(--control-border);
  box-shadow:
    inset 0 -2px 3px rgba(0, 0, 0, 0.25),
    inset 0 2px 3px rgba(255, 255, 255, 0.35);
  vertical-align: middle;
}
.ball-swatch-sm {
  width: 14px;
  height: 14px;
}
.ball-swatch-md {
  width: 20px;
  height: 20px;
}
.ball-swatch-lg {
  width: 30px;
  height: 30px;
}
</style>
