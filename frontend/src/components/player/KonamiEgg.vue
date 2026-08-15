<script setup lang="ts">
/**
 * Konami-code easter egg for the player board: entering
 * ↑ ↑ ↓ ↓ ← → ← → B A wipes the player's stamps and puts Drani's grin over the
 * screen, growing and fading out as it goes.
 *
 * Mounted by PlayerView, so the sequence is only live while a board is on screen.
 * Keys are matched against a rolling window of the last ten rather than a progress
 * counter, so a fumbled repeat (↑ ↑ ↑ ↓ ↓ ...) still resolves instead of dropping
 * the whole run. The reveal is keyed on its run id, so entering the code again
 * mid-flight restarts the animation from the top instead of doing nothing.
 *
 * The image is a BUNDLED import, not a `${BASE_URL}images/...` URL like the yoever
 * head. `public/images/` is the persistent, admin-uploaded tree: the build strips
 * `dist/images` and the server serves that path from a folder outside the deploy,
 * so an asset that ships with the code and is only referenced there 404s in
 * production until someone copies it onto the server by hand. Importing it emits a
 * content-hashed file into `dist/assets/`, which works in dev, in a built preview
 * and on live with no deploy step.
 *
 * It is also warmed on mount at low priority rather than fetched when the egg
 * fires. Loading it lazily meant the first reveal of a session raced its own
 * download - and the reveal unmounts at REVEAL_MS, which cancels an unfinished
 * request, so a slow fetch could never complete across repeated triggers.
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'
import grinSrc from '@/assets/images/DraniGrin.webp'
import { usePlayerStore } from '@/stores/player'

const player = usePlayerStore()

/** The sequence in `KeyboardEvent.key` terms; single characters compare lower-cased. */
const SEQUENCE = [
  'ArrowUp',
  'ArrowUp',
  'ArrowDown',
  'ArrowDown',
  'ArrowLeft',
  'ArrowRight',
  'ArrowLeft',
  'ArrowRight',
  'b',
  'a',
]

/** How long the reveal stays up; must match `konami-egg-swell` in player.css. */
const REVEAL_MS = 2600

/** The last SEQUENCE.length keys seen, oldest first. */
const recent: string[] = []
/** Run id of the reveal on screen; null while nothing is showing. */
const runId = ref<number | null>(null)
let runs = 0
let hideTimer: number | undefined

/** True for a target that owns its own keystrokes (a search box, a text field). */
function isTypingInto(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el?.tagName) return false
  return el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName)
}

function onKeydown(e: KeyboardEvent): void {
  if (isTypingInto(e.target)) return
  recent.push(e.key.length === 1 ? e.key.toLowerCase() : e.key)
  if (recent.length > SEQUENCE.length) recent.shift()
  if (recent.length < SEQUENCE.length) return
  if (!SEQUENCE.every((key, i) => key === recent[i])) return
  recent.length = 0
  reveal()
}

/** Wipes the board, then starts (or restarts) the reveal. */
function reveal(): void {
  player.clearAllStamps()
  runId.value = ++runs
  window.clearTimeout(hideTimer)
  hideTimer = window.setTimeout(() => (runId.value = null), REVEAL_MS)
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  // Low priority so it queues behind the board's own assets - nothing is waiting
  // on it until someone finds the code.
  const warm = new Image()
  warm.fetchPriority = 'low'
  warm.src = grinSrc
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  window.clearTimeout(hideTimer)
})
</script>

<template>
  <div v-if="runId !== null" class="konami-egg">
    <!-- Keyed on the run so a re-trigger remounts the element and replays the
         animation; the duration is inline to stay in lockstep with hideTimer. -->
    <div :key="runId" class="konami-egg-reveal" :style="{ animationDuration: `${REVEAL_MS}ms` }">
      <img class="konami-egg-art" :src="grinSrc" alt="" />
      <!-- role=status so a keyboard-only player is told something happened - the
           board just lost every stamp and the art alone doesn't say so. -->
      <p class="konami-egg-line" role="status">Oi, what'd you think was gonna happen?</p>
    </div>
  </div>
</template>
