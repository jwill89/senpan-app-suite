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
import { computed, ref } from 'vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import ColorPicker from '@/components/common/ui/ColorPicker.vue'
import { usePlayerStore } from '@/stores/player'

const player = usePlayerStore()

const open = ref(false)

/** The picker's current value — bound to the live stamp colour. */
const pickerValue = computed(() => player.stampColor)

/** Persists the chosen colour as a full CSS rgba() string (alpha included). */
function onColorChange(c: { rgba: string }): void {
  player.setStampColor(c.rgba)
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
      <h3 class="mb-8"><font-awesome-icon :icon="['fad', 'palette']" /> Stamp Color</h3>
      <p class="text-dim mb-16 color-picker-help">
        Choose any color and transparency for your board stamps.
      </p>

      <ColorPicker :value="pickerValue" @change="onColorChange" />

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

      <button class="btn-neutral mt-20 color-picker-done" @click="open = false">Done</button>
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

/* The Chrome picker skin lives globally on the shared `.color-picker` object
   (app.css) so it's identical here and in the Themes tool. */

/* Quick-pick presets row: label above a centered row of swatches. */
.color-picker-presets {
  flex-direction: column;
  align-items: center;
  gap: 10px;
}

.color-picker-presets-label {
  font-size: 0.8rem;
  color: var(--text-muted);
}

.color-picker-swatches {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 10px;
}
</style>

