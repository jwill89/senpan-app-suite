<script setup lang="ts">
/**
 * Thin wrapper around the `@ckpack/vue-color` Chrome picker: lazy-loads it (so
 * its weight only lands when a picker actually opens), applies the shared
 * `.color-picker` theme skin (app.css), and normalizes the change payload to a
 * CSS `rgba()` string plus a `#rrggbb(aa)` hex. Used by the stamp-colour modal
 * (StampColorPicker).
 */
import { defineAsyncComponent } from 'vue'

/** Lazy-load the Chrome picker so it never bloats the initial payload. */
const ChromePicker = defineAsyncComponent(() =>
  import('@ckpack/vue-color').then((m) => m.Chrome),
)

/** The picker's emitted payload (only the bits we read). */
interface ColorPayload {
  hex: string
  hex8: string
  rgba: { r: number | string; g: number | string; b: number | string; a: number }
}

defineProps<{
  /** Seed colour bound into the picker (hex or rgba). */
  value: string
}>()

const emit = defineEmits<{ change: [value: { rgba: string; hex: string }] }>()

/** Normalize the picker payload to an rgba() string + #rrggbb(aa) hex. */
function onChange(p: ColorPayload): void {
  const { r, g, b, a } = p.rgba
  const rgba = `rgba(${Number(r)}, ${Number(g)}, ${Number(b)}, ${a})`
  const hex = (a < 1 ? p.hex8 : p.hex) || p.hex
  emit('change', { rgba, hex })
}
</script>

<template>
  <div class="color-picker">
    <ChromePicker :model-value="value" @update:model-value="onChange" />
  </div>
</template>
