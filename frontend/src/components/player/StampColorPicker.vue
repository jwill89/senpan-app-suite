<script setup lang="ts">
/**
 * Stamp color picker — a single swatch button that opens a modal color picker.
 *
 * The modal hosts the Chrome picker (`@ckpack/vue-color`) so the player can dial
 * in any colour via the spectrum/hue interface, a HEX field, *and* an alpha
 * channel. The picked value is stored as a full CSS `rgba()` string (including
 * its alpha) and used directly as the stamp tint.
 *
 * Note: this alpha is intentionally distinct from the separate opacity slider —
 * the alpha tints only the stamp's background fill, whereas opacity fades the
 * entire mark (the emoji/custom-image icon included). The picker is lazy-loaded
 * so its weight only lands when a player actually opens it.
 */
import { computed, defineAsyncComponent, ref } from 'vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import { usePlayerStore } from '@/stores/player'

/** Lazy-load the Chrome picker so it never bloats the initial player payload. */
const ChromePicker = defineAsyncComponent(() =>
  import('@ckpack/vue-color').then((m) => m.Chrome),
)

/** Payload shape emitted by the picker (only the bits we read). */
interface ColorPayload {
  rgba: { r: number | string; g: number | string; b: number | string; a: number }
}

const player = usePlayerStore()

const open = ref(false)

/** The picker's current value — bound to the live stamp colour. */
const pickerValue = computed(() => player.stampColor)

/** Persists the chosen colour as a full CSS rgba() string (alpha included). */
function onColorChange(payload: ColorPayload): void {
  const { r, g, b, a } = payload.rgba
  player.setStampColor(`rgba(${Number(r)}, ${Number(g)}, ${Number(b)}, ${a})`)
}
</script>

<template>
  <div class="stamp-color-control" role="group" aria-label="Stamp color">
    <span class="label">Color:</span>
    <button
      type="button"
      class="color-picker-btn"
      title="Choose stamp color"
      aria-label="Choose stamp color"
      @click="open = true"
    >
      <span class="color-picker-swatch" :style="{ background: player.currentStampBg }"></span>
    </button>

    <ModalOverlay
      v-if="open"
      centered
      aria-label="Stamp color picker"
      :box-style="{ maxWidth: '340px' }"
      @close="open = false"
    >
      <h3 class="mb-8"><i class="fa-duotone fa-palette"></i> Stamp Color</h3>
      <p class="text-dim mb-16 color-picker-help">
        Choose any color and transparency for your board stamps.
      </p>

      <div class="color-picker-wrap">
        <ChromePicker :model-value="pickerValue" @update:model-value="onColorChange" />
      </div>

      <div class="color-picker-presets" role="group" aria-label="Quick colors">
        <span class="color-picker-presets-label">Quick colors</span>
        <div class="color-picker-swatches">
          <button
            v-for="c in player.stampColors"
            :key="c.id"
            type="button"
            class="color-swatch"
            :style="{ background: c.value }"
            :title="c.name"
            :aria-label="`Quick color: ${c.name}`"
            @click="player.setStampColor(c.value)"
          ></button>
        </div>
      </div>

      <button class="btn-primary mt-20 color-picker-done" @click="open = false">Done</button>
    </ModalOverlay>
  </div>
</template>

<style scoped>
.color-picker-help {
  font-size: 0.85rem;
}

.color-picker-done {
  width: 100%;
}

/* -- Dark-theme skin for the Chrome picker (@ckpack/vue-color) --
   The picker ships a light skin and its inputs inherit the app's global
   `input` background (dark) while keeping their own dark text — unreadable.
   These deep overrides re-map the picker surfaces + fields to the app theme so
   the widget blends with the modal and the HEX/RGB fields stay legible. */
.color-picker-wrap :deep(.vc-chrome) {
  width: 100%;
  max-width: 280px;
  background: var(--surface2);
  border-radius: var(--radius);
  box-shadow: 0 6px 18px var(--shadow-color);
  font-family: inherit;
}

.color-picker-wrap :deep(.vc-chrome-body) {
  background: var(--surface2);
}

.color-picker-wrap :deep(.vc-input__input) {
  width: 100%;
  height: 26px;
  padding: 2px 4px;
  border: none;
  border-radius: 4px;
  font-size: 0.72rem;
  text-align: center;
  background: var(--surface);
  color: var(--text);
  box-shadow: inset 0 0 0 1px var(--surface2);
}

.color-picker-wrap :deep(.vc-input__input:focus) {
  box-shadow: inset 0 0 0 1px var(--secondary);
}

.color-picker-wrap :deep(.vc-input__label) {
  color: var(--text-dim);
}

/* The Markdown/eyedropper toggle arrows that switch HEX ⇄ RGBA ⇄ HSLA. */
.color-picker-wrap :deep(.vc-chrome-toggle-icon path) {
  fill: var(--text-dim);
}

.color-picker-wrap :deep(.vc-chrome-toggle-icon-highlight) {
  background: var(--surface);
}

/* Quick-pick presets row: label above a centered row of swatches. */
.color-picker-presets {
  flex-direction: column;
  align-items: center;
  gap: 10px;
}

.color-picker-presets-label {
  font-size: 0.8rem;
  color: var(--text-dim);
}

.color-picker-swatches {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 10px;
}
</style>

