<script setup lang="ts">
/**
 * Player sound controls: a mode selector (Off / Basic / Game) plus a master
 * volume slider. Basic mode uses the synthesized beeps; Game mode uses the
 * bundled sound effects. Selecting a sound mode counts as the user gesture
 * browsers require to start audio, so we prime the context and play that mode's
 * "number called" sound as confirmation. The volume slider is disabled while
 * sound is off. Mirrors the StampOpacitySlider's custom range styling.
 */
import { computed } from 'vue'
import { usePlayerStore, type SoundMode } from '@/stores/player'
import { primeAudio, playEvent } from '@/lib/sound'

const player = usePlayerStore()

const MODES: { value: SoundMode; label: string; icon: string; title: string }[] = [
  { value: 'off', label: 'Off', icon: 'volume-xmark', title: 'No sound' },
  { value: 'basic', label: 'Basic', icon: 'volume-low', title: 'Simple beeps' },
  { value: 'game', label: 'Game', icon: 'volume-high', title: 'Game sound effects' },
]

function selectMode(mode: SoundMode): void {
  player.setSoundMode(mode)
  if (mode !== 'off') {
    primeAudio()
    playEvent('draw', mode) // confirm the chosen mode is audible
  }
}

/** Gold-fill length ending at the thumb centre (thumb is 16px on the track). */
const fillPct = computed(() => `calc((100% - 16px) * ${Number(player.soundVolume)} + 8px)`)

function onVolume(e: Event): void {
  player.setSoundVolume(Number((e.target as HTMLInputElement).value))
}
</script>

<template>
  <div class="sound-controls">
    <div class="sound-mode-row">
      <span class="label">Sound:</span>
      <div class="sound-mode-btns">
        <button
          v-for="m in MODES"
          :key="m.value"
          type="button"
          class="toggle-btn sound-mode-btn"
          :class="{ active: player.soundMode === m.value }"
          :aria-pressed="player.soundMode === m.value"
          :title="m.title"
          @click="selectMode(m.value)"
        >
          <font-awesome-icon :icon="['fas', m.icon]" /> {{ m.label }}
        </button>
      </div>
    </div>

    <div class="opacity-slider sound-volume" :class="{ 'is-disabled': !player.soundOn }">
      <span class="label">Volume:</span>
      <input
        type="range"
        min="0"
        max="1"
        step="0.05"
        aria-label="Sound volume"
        :value="player.soundVolume"
        :disabled="!player.soundOn"
        :style="{ '--fill': fillPct }"
        @input="onVolume"
      />
      <span class="opacity-val">{{ Math.round(player.soundVolume * 100) }}%</span>
    </div>
  </div>
</template>

<style scoped>
.sound-controls {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.sound-mode-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sound-mode-row .label {
  font-size: 0.85rem;
  color: var(--text-muted);
  min-width: 58px;
}
.sound-mode-btns {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
/* Compact toggle buttons (reuse the global .toggle-btn intent + active state). */
.sound-mode-btn {
  padding: 4px 10px;
  font-size: 0.85rem;
}
/* The volume row reuses the global .opacity-slider styling; only the disabled
   (sound-off) affordance is local. */
.sound-volume .label {
  min-width: 58px;
}
.sound-volume input[type='range'] {
  flex: 1;
  width: auto;
  min-width: 120px;
}
.sound-volume.is-disabled {
  opacity: 0.5;
}
.sound-volume.is-disabled input[type='range'] {
  cursor: not-allowed;
}
</style>
