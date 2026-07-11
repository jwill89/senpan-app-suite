<script setup lang="ts">
/**
 * "It's Yoever" overlay — a full-viewport, click-through layer that renders each
 * active reaction from the yoever store as a reduced-size, reduced-opacity image
 * that bounces across the screen and fades out over a few seconds, with the
 * triggering player's name captioned below it. Mounted once in the app shell so
 * it appears on both the player and admin views. Respects prefers-reduced-motion
 * (fades in place instead of flying), and shows nothing while the client has
 * opted out (the store never adds reactions when muted).
 */
import { useYoeverStore, YOEVER_DURATION_MS } from '@/stores/yoever'

const yoever = useYoeverStore()

/** Bundled reaction image (served same-origin from <base>/images/). */
const headSrc = `${import.meta.env.BASE_URL}images/big-yoey-head.png`

/** Drives the CSS animation length so it always matches the store's removal timer. */
const durationMs = YOEVER_DURATION_MS
</script>

<template>
  <div class="yoever-layer" aria-hidden="true">
    <div
      v-for="y in yoever.active"
      :key="y.id"
      class="yoever-fly"
      :style="{ top: y.top + 'vh', animationDuration: durationMs + 'ms' }"
    >
      <div class="yoever-bob">
        <img class="yoever-head" :src="headSrc" alt="" />
        <div class="yoever-name">
          <span class="yoever-quote">&ldquo;It's Yoever.&rdquo;</span>
          <span class="yoever-attrib">~{{ y.name || 'Someone' }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.yoever-layer {
  position: fixed;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
  z-index: 9999;
}

/* Horizontal traverse + overall fade. Left offset is animated via translateX so
   only the transform/opacity compositor properties change (cheap to animate). The
   duration is supplied inline (animationDuration) so it stays in lockstep with the
   store's removal timer; keeping it out of a var()-in-shorthand also avoids a
   fragile custom-property resolution that could collapse the animation to 0s. */
.yoever-fly {
  position: absolute;
  left: 0;
  will-change: transform, opacity;
  animation-name: yoever-cross;
  animation-timing-function: linear;
  animation-fill-mode: forwards;
}

/* Vertical bounce, decoupled from the horizontal traverse so both transforms can
   run at once (parent = X, child = Y). */
.yoever-bob {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  will-change: transform;
  animation: yoever-bob 1.05s ease-in-out infinite;
}

.yoever-head {
  width: clamp(90px, 14vw, 170px);
  height: auto;
  user-select: none;
  filter: drop-shadow(0 6px 10px rgba(0, 0, 0, 0.45));
}

.yoever-name {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1px;
  max-width: 44vw;
  padding: 4px 12px;
  border-radius: 14px;
  background: rgba(0, 0, 0, 0.55);
  color: #fff;
  text-align: center;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.6);
}

.yoever-quote {
  font-weight: 700;
  font-style: italic;
  font-size: clamp(0.85rem, 2.4vw, 1.1rem);
  white-space: nowrap;
}

.yoever-attrib {
  font-weight: 600;
  font-size: clamp(0.72rem, 2vw, 0.92rem);
  opacity: 0.9;
  max-width: 40vw;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

@keyframes yoever-cross {
  0% {
    transform: translateX(-25vw);
    opacity: 0;
  }
  10% {
    opacity: 0.8;
  }
  78% {
    opacity: 0.8;
  }
  100% {
    transform: translateX(115vw);
    opacity: 0;
  }
}

@keyframes yoever-bob {
  0%,
  100% {
    transform: translateY(0);
  }
  50% {
    transform: translateY(-56px);
  }
}

/* Reduced motion: don't fly or bounce — fade in and out, centred. */
@media (prefers-reduced-motion: reduce) {
  .yoever-fly {
    left: 50%;
    transform: translateX(-50%);
    animation-name: yoever-fade;
    animation-timing-function: ease-in-out;
  }
  .yoever-bob {
    animation: none;
  }
  @keyframes yoever-fade {
    0%,
    100% {
      opacity: 0;
    }
    15%,
    75% {
      opacity: 0.8;
    }
  }
}
</style>
