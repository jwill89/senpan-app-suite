/**
 * "It's Yoever" reaction store.
 *
 * Holds the transient list of on-screen reactions (the bouncing image + the
 * triggering player's name) and the per-client opt-out. It is deliberately
 * view-agnostic — the overlay lives in the app shell so the effect shows on both
 * the player and admin views — and knows nothing about the trigger button or the
 * server toggle (those live in the player/game stores). The WebSocket dispatch
 * calls `show()` when a `yoever` message arrives; `muted` (persisted locally)
 * lets a user suppress the sound + animation on their own screen without
 * affecting anyone else.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'

/** How long one reaction stays on screen; must match the overlay CSS animation. */
export const YOEVER_DURATION_MS = 7000

/** Safety cap on simultaneous on-screen reactions (a burst can't flood the DOM). */
const MAX_ACTIVE = 8

/** One active on-screen reaction. */
export interface ActiveYoever {
  /** Unique key for the v-for / removal. */
  id: number
  /** Player name shown under the image (may be empty → the overlay shows a fallback). */
  name: string
  /** Random vertical baseline (vh) so stacked reactions don't perfectly overlap. */
  top: number
}

export const useYoeverStore = defineStore('yoever', () => {
  let seq = 0
  const active = ref<ActiveYoever[]>([])
  /** Client opt-out for the VISUAL: when true, this browser shows no animation. */
  const muted = ref(localStorage.getItem('bingo_yoever_muted') === '1')
  /**
   * Client toggle for the reaction SOUND (this_is_bad.mp3). Independent of the
   * main Sound mode (off/basic/game) — it plays whenever this is on, regardless —
   * but still respects the master sound volume. On by default.
   */
  const soundEnabled = ref(localStorage.getItem('bingo_yoever_sound') !== '0')

  /** Sets (and persists) whether this client shows the reaction animation. */
  function setMuted(on: boolean): void {
    muted.value = on
    localStorage.setItem('bingo_yoever_muted', on ? '1' : '0')
  }

  /** Sets (and persists) whether this client plays the reaction sound. */
  function setSoundEnabled(on: boolean): void {
    soundEnabled.value = on
    localStorage.setItem('bingo_yoever_sound', on ? '1' : '0')
  }

  /**
   * Master toggle for the whole reaction on this screen. "Show effects" governs
   * the sound too: turning effects ON enables the sound, turning them OFF disables
   * it. The sound is only independently adjustable while effects are shown.
   */
  function toggleShowEffects(): void {
    const willShow = muted.value // currently hidden → about to show
    setMuted(!willShow)
    setSoundEnabled(willShow)
  }

  /** Toggles the reaction sound — only while effects are shown (locked off
   *  otherwise, since the sound can't play without the effect being active). */
  function toggleSound(): void {
    if (muted.value) return
    setSoundEnabled(!soundEnabled.value)
  }

  /**
   * Adds one bouncing reaction for `name`, auto-removing it once its animation
   * finishes. No-op while muted, so a suppressed client never renders it.
   */
  function show(name: string): void {
    if (muted.value) return
    const id = ++seq
    // Bias toward the upper half so the name caption below the image stays visible.
    const top = 8 + Math.random() * 34
    active.value = [...active.value, { id, name, top }].slice(-MAX_ACTIVE)
    setTimeout(() => {
      active.value = active.value.filter((y) => y.id !== id)
    }, YOEVER_DURATION_MS)
  }

  return {
    active,
    muted,
    soundEnabled,
    setMuted,
    setSoundEnabled,
    toggleShowEffects,
    toggleSound,
    show,
  }
})
