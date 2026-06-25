<script setup lang="ts">
/**
 * The animated Garapon drum — a hand-crank lottery barrel (ガラポン) on a stand.
 *
 * Tapping the drum (when draws remain) spins the barrel a few turns while the
 * authoritative draw resolves on the server, then drops a ball — tinted to the
 * won prize's color — out of the chute with a "pon" and a bounce, and rests it in
 * the tray. The result is emitted via `settled` once the ball lands, so the page
 * reveals the prize in sync with the animation. Tapping again flings the resting
 * ball off-screen and runs another round.
 *
 * The draw is provided by the parent (`performDraw`) so this component owns only
 * the animation, never the odds — it animates to whatever the server returned.
 */
import { nextTick, ref } from 'vue'
import { playPonSound, playGaraponRoll, vibrate } from '@/lib/sound'
import type { GaraponDrawResponse } from '@/types/api'

const props = defineProps<{
  /** Whether drawing is currently disabled (closed garapon or no draws left). */
  disabled: boolean
  /** Performs one authoritative draw; resolves to the result (or null on error). */
  performDraw: () => Promise<GaraponDrawResponse | null>
}>()
const emit = defineEmits<{ settled: [resp: GaraponDrawResponse] }>()

const rotation = ref(0)
const spinning = ref(false)
const restingBall = ref<{ color: string } | null>(null)
const leavingBall = ref<{ color: string } | null>(null)
const ballEl = ref<HTMLElement | null>(null)

// Regular hexagon barrel (pointy top/bottom), centered (160,140), radius 78.
const hexPoints = '160,62 227.5,101 227.5,179 160,218 92.5,179 92.5,101'

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

async function spin(): Promise<void> {
  if (props.disabled || spinning.value) return
  spinning.value = true

  // Fling the previous resting ball off-screen before the new round.
  if (restingBall.value) {
    leavingBall.value = restingBall.value
    restingBall.value = null
    window.setTimeout(() => (leavingBall.value = null), 650)
  }

  // Spin a few full turns (plus a random extra) — CSS eases it to a stop.
  const turns = 3 + Math.floor(Math.random() * 3)
  rotation.value += turns * 360 + Math.floor(Math.random() * 160)

  // The authoritative draw runs in parallel with a minimum spin time, so the
  // prize never reveals before the drum has visibly spun.
  const SPIN_MS = 1900
  playGaraponRoll(SPIN_MS) // wooden balls tumbling while it spins
  const [resp] = await Promise.all([props.performDraw(), delay(SPIN_MS)])
  if (!resp) {
    spinning.value = false
    return
  }

  await dropBall(resp.draw.ball_color)
  spinning.value = false
  emit('settled', resp)
}

async function dropBall(color: string): Promise<void> {
  restingBall.value = { color }
  await nextTick()
  const el = ballEl.value
  if (!el) {
    playPonSound()
    vibrate(40)
    return
  }
  const anim = el.animate(
    [
      { transform: 'translate(-4px, -200px) scale(0.9)' },
      { transform: 'translate(0, 0) scale(1)', offset: 0.5 }, // first landing
      { transform: 'translate(4px, -26px)', offset: 0.66 }, // bounce + roll
      { transform: 'translate(7px, 0)', offset: 0.8 },
      { transform: 'translate(9px, -8px)', offset: 0.9 },
      { transform: 'translate(10px, 0)', offset: 1 }, // rolled to a stop
    ],
    { duration: 760, easing: 'cubic-bezier(0.33, 0, 0.4, 1)', fill: 'forwards' },
  )
  // "pon" at the first landing (~50% through the drop).
  window.setTimeout(() => {
    playPonSound()
    vibrate(40)
  }, 380)
  await anim.finished.catch(() => {})
}
</script>

<template>
  <div class="wheel-stage">
    <!-- The drum + ball share their own box so the hint below is never under the
         dropped ball. -->
    <div class="drum-area">
      <button
        type="button"
        class="drum-button"
        :class="{ 'is-spinning': spinning, 'can-draw': !disabled && !spinning }"
        :disabled="disabled || spinning"
        :aria-label="disabled ? 'No draws remaining' : 'Spin the Garapon'"
        @click="spin"
      >
        <svg class="drum-svg" viewBox="0 0 320 320" xmlns="http://www.w3.org/2000/svg">
          <defs>
            <!-- Natural wood tone for the barrel facets. -->
            <linearGradient id="garapon-wood" x1="0" y1="0" x2="0.25" y2="1">
              <stop offset="0" stop-color="#cda06b" />
              <stop offset="0.55" stop-color="#b07f4e" />
              <stop offset="1" stop-color="#8f6438" />
            </linearGradient>
          </defs>
          <!-- Stand -->
          <g class="drum-stand">
            <path d="M110 290 L138 150 L150 150 L126 290 Z" />
            <path d="M210 290 L182 150 L170 150 L194 290 Z" />
            <rect x="92" y="288" width="136" height="14" rx="5" />
          </g>
          <!-- Catch tray -->
          <path class="drum-tray" d="M120 286 Q160 312 200 286 L196 302 Q160 324 124 302 Z" />
          <!-- Rotating barrel + crank around the axle (160,140) -->
          <g class="barrel" :style="{ transform: `rotate(${rotation}deg)` }">
            <polygon class="barrel-face" :points="hexPoints" fill="url(#garapon-wood)" />
            <g class="barrel-spokes">
              <line x1="160" y1="140" x2="160" y2="62" />
              <line x1="160" y1="140" x2="227.5" y2="101" />
              <line x1="160" y1="140" x2="227.5" y2="179" />
              <line x1="160" y1="140" x2="160" y2="218" />
              <line x1="160" y1="140" x2="92.5" y2="179" />
              <line x1="160" y1="140" x2="92.5" y2="101" />
            </g>
            <polygon class="barrel-rim" :points="hexPoints" />
            <circle class="barrel-hub" cx="160" cy="140" r="14" />
            <!-- crank -->
            <line class="crank-arm" x1="160" y1="140" x2="252" y2="140" />
            <circle class="crank-knob" cx="252" cy="140" r="9" />
          </g>
          <!-- Chute spout under the barrel -->
          <path class="drum-chute" d="M148 208 L172 208 L178 250 L142 250 Z" />
        </svg>
      </button>

      <!-- Ball overlay (resting + the one flinging away) -->
      <div class="ball-layer" aria-hidden="true">
        <div
          v-if="restingBall"
          ref="ballEl"
          class="garapon-ball"
          :style="{ '--ball': restingBall.color }"
        ></div>
        <div
          v-if="leavingBall"
          class="garapon-ball is-leaving"
          :style="{ '--ball': leavingBall.color }"
        ></div>
      </div>
    </div>

    <p v-if="!disabled && !spinning" class="drum-hint text-dim">
      <font-awesome-icon :icon="['fad', 'ferris-wheel']" /> Tap the drum to draw!
    </p>
    <p v-else-if="spinning" class="drum-hint text-dim">Spinning…</p>
  </div>
</template>

<style scoped>
.wheel-stage {
  position: relative;
  width: 100%;
  max-width: 340px;
  margin: 0 auto;
}
.drum-button {
  display: block;
  width: 100%;
  padding: 0;
  background: none;
  border: none;
  cursor: pointer;
}
.drum-button:disabled {
  cursor: default;
}
.drum-svg {
  width: 100%;
  height: auto;
  display: block;
  overflow: visible;
}
/* Positioning context for the ball overlay — sized to the drum only, so the hint
   text below sits clear of a dropped ball. */
.drum-area {
  position: relative;
}

/* The barrel rotates around the axle in view-box coordinates (no bbox wobble). */
.barrel {
  transform-box: view-box;
  transform-origin: 160px 140px;
  transition: transform 1.9s cubic-bezier(0.16, 0.62, 0.18, 1);
}
/* Wooden barrel: a subtle seam stroke over the wood gradient, a gold band rim,
   dark-brown facet seams, and a brass axle pin. */
.barrel-face {
  stroke: #6f4a25;
  stroke-width: 1.5;
}
.barrel-rim {
  fill: none;
  stroke: var(--highlight);
  stroke-width: 5;
}
.barrel-spokes line {
  stroke: #7a5430;
  stroke-width: 2;
  opacity: 0.55;
}
.barrel-hub {
  fill: var(--highlight);
  stroke: #6f4a25;
  stroke-width: 2;
}
.crank-arm {
  stroke: var(--control-border);
  stroke-width: 6;
  stroke-linecap: round;
}
.crank-knob {
  fill: var(--highlight);
}
.drum-stand path,
.drum-stand rect {
  fill: var(--control-border);
}
.drum-tray {
  fill: color-mix(in srgb, var(--control-border) 75%, #000);
}
.drum-chute {
  fill: color-mix(in srgb, var(--control-border) 85%, #000);
}

/* A gentle glow on hover signals the drum is tappable. This stays off the barrel
   itself (no competing transform animation), so the spin transition always fires. */
.drum-button.can-draw:hover .drum-svg {
  filter: drop-shadow(0 0 5px color-mix(in srgb, var(--highlight) 26%, transparent));
}

.ball-layer {
  position: absolute;
  inset: 0;
  pointer-events: none;
}
.garapon-ball {
  position: absolute;
  left: calc(50% - 21px);
  bottom: 7%;
  width: 42px;
  height: 42px;
  border-radius: 50%;
  background: radial-gradient(
    circle at 32% 28%,
    rgba(255, 255, 255, 0.85),
    var(--ball) 46%,
    color-mix(in srgb, var(--ball) 65%, #000) 100%
  );
  box-shadow: 0 6px 12px var(--shadow);
}
.garapon-ball.is-leaving {
  animation: garapon-ball-away 0.6s ease-in forwards;
}
@keyframes garapon-ball-away {
  0% {
    transform: translate(10px, 0) scale(1);
    opacity: 1;
  }
  30% {
    transform: translate(48px, -70px) scale(1.05) rotate(45deg);
    opacity: 1;
  }
  100% {
    transform: translate(340px, 240px) scale(0.5) rotate(240deg);
    opacity: 0;
  }
}
.drum-hint {
  text-align: center;
  margin-top: 6px;
}

@media (prefers-reduced-motion: reduce) {
  .barrel {
    transition-duration: 0.4s;
  }
}
</style>
