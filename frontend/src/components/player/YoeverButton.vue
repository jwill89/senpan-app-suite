<script setup lang="ts">
/**
 * "It's Yoever" trigger button, shown next to Save Board whenever a game is
 * running. Clicking broadcasts the reaction to every connected client (sound + a
 * bouncing image with this player's name). The button stays visible but is
 * DISABLED when the host has switched the reaction off (so the feature never
 * looks like it vanished), while this player is on cooldown, or mid-send. Each
 * board is rate-limited: after a trigger the button disables and counts down
 * until the cooldown clears (mirrored from the server and persisted per
 * card+game, so it survives a refresh).
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { usePlayerStore } from '@/stores/player'
import { primeAudio } from '@/lib/sound'

const player = usePlayerStore()

// Ticking clock so the countdown label updates and the button re-enables live.
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  timer = setInterval(() => {
    now.value = Date.now()
  }, 500)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})

/** Whole seconds left on this client's cooldown (0 = ready). */
const remaining = computed(() =>
  Math.max(0, Math.ceil((player.yoeverCooldownUntil - now.value) / 1000)),
)
const onCooldown = computed(() => remaining.value > 0)
const canTrigger = computed(
  () => player.yoeverEnabled && !player.yoeverTriggering && !onCooldown.value,
)

/** mm:ss (or Ns under a minute) for the cooldown label. */
const countdown = computed(() => {
  const s = remaining.value
  const m = Math.floor(s / 60)
  const sec = s % 60
  return m > 0 ? `${m}:${String(sec).padStart(2, '0')}` : `${sec}s`
})

/** Tooltip explaining the current (possibly disabled) state. */
const title = computed(() => {
  if (!player.yoeverEnabled) return `"It's Yoever" is switched off by the host`
  if (onCooldown.value) return `Hold on — you can do this again in ${countdown.value}`
  if (player.yoeverTriggering) return 'Sending…'
  return `Trigger "It's Yoever" for everyone`
})

function onTrigger(): void {
  // The click is a user gesture — warm audio so the reaction sound (arriving via
  // the broadcast a moment later) isn't blocked by the browser's autoplay policy.
  primeAudio()
  void player.triggerYoever()
}
</script>

<template>
  <button
    v-if="player.playerGame"
    class="btn-view btn-sm yoever-trigger"
    :disabled="!canTrigger"
    :title="title"
    @click="onTrigger"
  >
    <font-awesome-icon :icon="['fad', 'megaphone']" />
    <span class="player-actions__label">{{
      onCooldown ? `Yoever ${countdown}` : "It's Yoever"
    }}</span>
  </button>
</template>
