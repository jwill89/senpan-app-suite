<script setup lang="ts">
/**
 * Secondary-stamp control: an on/off toggle for an optional second stamp (a plain
 * coloured circle, no emoji/image) plus its own colour picker. When enabled, the
 * secondary stamp auto-marks board cells that are NOT part of a win pattern,
 * while the primary stamp marks the pattern cells. It shares the single opacity
 * slider. The toggle + colour swatch fill the row like the Stamp/Color row above;
 * the colour picker mirrors the primary StampColorPicker (Chrome picker + alpha in
 * a modal, quick-pick presets), bound to the secondary colour.
 */
import { computed, ref } from 'vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import ColorPicker from '@/components/common/ui/ColorPicker.vue'
import { usePlayerStore } from '@/stores/player'

const player = usePlayerStore()

const open = ref(false)

/** The picker's current value — bound to the live secondary stamp colour. */
const pickerValue = computed(() => player.secondaryStampColor)

function onColorChange(c: { rgba: string }): void {
  player.setSecondaryStampColor(c.rgba)
}
</script>

<template>
  <div class="secondary-stamp stamp-shape-color-row" role="group" aria-label="Secondary stamp">
    <div class="secondary-stamp-toggle">
      <span class="label">2nd stamp:</span>
      <button
        type="button"
        class="switch"
        role="switch"
        :class="{ on: player.secondaryStampEnabled }"
        :aria-checked="player.secondaryStampEnabled"
        title="Mark cells outside the win pattern with a second colour"
        @click="player.setSecondaryStampEnabled(!player.secondaryStampEnabled)"
      >
        <span class="switch-knob"></span>
      </button>
    </div>

    <div class="stamp-color-control">
      <span class="label">Color:</span>
      <button
        type="button"
        class="color-picker-btn"
        :disabled="!player.secondaryStampEnabled"
        title="Choose secondary stamp color"
        aria-label="Choose secondary stamp color"
        @click="open = true"
      >
        <span
          class="color-picker-swatch"
          :style="{ background: player.currentSecondaryStampBg }"
        ></span>
      </button>
    </div>

    <ModalOverlay
      v-if="open"
      centered
      aria-label="Secondary stamp color picker"
      :box-style="{ maxWidth: '340px' }"
      @close="open = false"
    >
      <h3 class="mb-8"><font-awesome-icon :icon="['fad', 'palette']" /> Secondary Stamp Color</h3>
      <p class="text-dim mb-16 color-picker-help">
        The colour used on cells that aren't part of a winning pattern.
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
            @click="player.setSecondaryStampColor(c.value)"
          ></button>
        </div>
      </div>

      <button class="btn-neutral mt-20 color-picker-done" @click="open = false">Done</button>
    </ModalOverlay>
  </div>
</template>

<style scoped>
/* Reuses the global .stamp-shape-color-row (space-between, fills the row) +
   .stamp-color-control so the swatch lines up with the primary Color row above. */
.secondary-stamp-toggle {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.secondary-stamp-toggle .label {
  font-size: 0.85rem;
  color: var(--text-muted);
}

.color-picker-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.color-picker-help {
  font-size: 0.85rem;
}
.color-picker-done {
  width: 100%;
}
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
