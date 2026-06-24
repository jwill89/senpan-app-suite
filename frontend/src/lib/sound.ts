/**
 * Tiny, dependency-free audio + haptics helper for the player's optional
 * "draw" feedback. Synthesizes a short two-note chime with the Web Audio API
 * (no audio asset to ship/precache) and triggers a brief vibration on devices
 * that support it.
 *
 * This is purely an ambient *alert* that a number was called — it announces the
 * caller's draw, exactly like a physical caller's voice. It deliberately knows
 * nothing about the player's board, so it never does the player's job for them.
 *
 * Everything is wrapped in try/catch and gated on feature support so a missing
 * AudioContext / blocked autoplay never throws into the WebSocket handler.
 */

let ctx: AudioContext | null = null

/** Lazily creates the shared AudioContext (handles the webkit-prefixed name). */
function getCtx(): AudioContext | null {
  if (ctx) return ctx
  const AC =
    window.AudioContext ||
    (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
  if (!AC) return null
  ctx = new AC()
  return ctx
}

// ── Volume ───────────────────────────────────────────────────────────────────
// Shared 0..1 master volume scaling both the synthesized "basic" chimes (applied
// to each note's gain peak) and the "game" MP3 samples (HTMLAudioElement.volume).
// Kept in sync with the player's preference via setSoundVolume.

let volume = 0.7

/** Sets the master sound volume (clamped to 0..1). */
export function setSoundVolume(v: number): void {
  volume = Math.min(1, Math.max(0, Number.isFinite(v) ? v : 0.7))
}

// ── Game sound samples (MP3s served from /sounds) ────────────────────────────
// One reusable <audio> per file, played by resetting currentTime. Best-effort:
// any failure (missing file, blocked autoplay) is swallowed.

const sampleCache = new Map<string, HTMLAudioElement>()

/** Public URL of a bundled sound effect (served from <base>/sounds/<name>.mp3). */
function soundUrl(name: string): string {
  return `${import.meta.env.BASE_URL}sounds/${name}.mp3`
}

function getSample(name: string): HTMLAudioElement | null {
  try {
    let a = sampleCache.get(name)
    if (!a) {
      a = new Audio(soundUrl(name))
      a.preload = 'auto'
      sampleCache.set(name, a)
    }
    return a
  } catch {
    return null
  }
}

/** Plays a bundled MP3 sample at the current volume (no-op when muted). */
function playSample(name: string): void {
  if (volume <= 0) return
  try {
    const a = getSample(name)
    if (!a) return
    a.volume = volume
    a.currentTime = 0
    void a.play()
  } catch {
    /* non-fatal */
  }
}

/**
 * Resumes the AudioContext from a user gesture (e.g. the moment the player
 * toggles sound on). Browsers start the context suspended until a gesture, so
 * priming here lets the first real draw chime play without delay.
 */
export function primeAudio(): void {
  try {
    const ac = getCtx()
    if (ac && ac.state === 'suspended') void ac.resume()
    // Warm the game samples so the first real event plays without a fetch delay.
    for (const name of ['moogle_noise', 'queue_pop', 'level_up']) getSample(name)?.load()
  } catch {
    /* non-fatal */
  }
}

/**
 * Plays a sequence of synthesized notes as a short chime, scaling each note's
 * peak gain by the master volume. Shared by the basic-mode chimes below; a muted
 * volume is a no-op (an exponential ramp can't target zero). Each note: oscillator
 * type, frequency, start offset, peak gain, and total duration.
 */
function playChime(
  notes: { freq: number; at: number; peak: number; dur: number; type?: OscillatorType }[],
): void {
  if (volume <= 0) return
  try {
    const ac = getCtx()
    if (!ac) return
    if (ac.state === 'suspended') void ac.resume()
    const now = ac.currentTime
    for (const n of notes) {
      const osc = ac.createOscillator()
      const gain = ac.createGain()
      osc.type = n.type ?? 'sine'
      osc.frequency.value = n.freq
      const start = now + n.at
      gain.gain.setValueAtTime(0.0001, start)
      gain.gain.exponentialRampToValueAtTime(Math.max(0.0001, n.peak * volume), start + 0.02)
      gain.gain.exponentialRampToValueAtTime(0.0001, start + n.dur)
      osc.connect(gain).connect(ac.destination)
      osc.start(start)
      osc.stop(start + n.dur + 0.02)
    }
  } catch {
    /* non-fatal */
  }
}

/** Plays a short rising two-note chime to signal a freshly called number. */
export function playDrawChime(): void {
  playChime([
    { freq: 880, at: 0, peak: 0.22, dur: 0.18 }, // A5
    { freq: 1318.5, at: 0.12, peak: 0.22, dur: 0.18 }, // E6
  ])
}

/**
 * Basic-mode alert for a half-time minigame announcement: a quick attention
 * "boop-beep" (square-ish), distinct from the draw and game-end chimes.
 */
export function playMinigameChime(): void {
  playChime([
    { freq: 987.77, at: 0, peak: 0.2, dur: 0.14, type: 'triangle' }, // B5
    { freq: 1318.5, at: 0.1, peak: 0.2, dur: 0.18, type: 'triangle' }, // E6
  ])
}

/**
 * Basic-mode alert for a game ending: a settled, resolving C-major triad
 * (C5+E5+G5) held a touch longer so it reads as a final "that's a wrap".
 */
export function playGameEndChime(): void {
  playChime([
    { freq: 523.25, at: 0, peak: 0.18, dur: 0.5, type: 'triangle' }, // C5
    { freq: 659.25, at: 0.04, peak: 0.16, dur: 0.5, type: 'triangle' }, // E5
    { freq: 783.99, at: 0.08, peak: 0.16, dur: 0.5, type: 'triangle' }, // G5
  ])
}

/** A player-selectable sound event. */
export type SoundEvent = 'draw' | 'minigame' | 'gameend'

/**
 * Plays the sound for an event in the chosen mode: "basic" uses the synthesized
 * chimes above; "game" uses the bundled MP3 effects (moogle_noise / queue_pop /
 * level_up). Callers gate on the player's mode being non-"off" first.
 */
export function playEvent(event: SoundEvent, mode: 'basic' | 'game'): void {
  if (mode === 'game') {
    const file = { draw: 'moogle_noise', minigame: 'queue_pop', gameend: 'level_up' }[event]
    playSample(file)
    return
  }
  const chime = { draw: playDrawChime, minigame: playMinigameChime, gameend: playGameEndChime }[event]
  chime()
}

/**
 * Plays a short, celebratory ascending arpeggio to signal a new winner. Used by
 * the admin's optional "winner sound" toggle so the caller hears a bingo without
 * watching the screen. Distinct from the single draw chime so the two are easy
 * to tell apart.
 */
export function playWinnerChime(): void {
  // C5 → E5 → G5 → C6 ascending arpeggio.
  playChime([
    { freq: 523.25, at: 0, peak: 0.25, dur: 0.32, type: 'triangle' },
    { freq: 659.25, at: 0.1, peak: 0.25, dur: 0.32, type: 'triangle' },
    { freq: 783.99, at: 0.2, peak: 0.25, dur: 0.32, type: 'triangle' },
    { freq: 1046.5, at: 0.3, peak: 0.25, dur: 0.32, type: 'triangle' },
  ])
}

/** Triggers a brief device vibration where supported (mobile). */
export function vibrate(pattern: number | number[] = 60): void {
  try {
    navigator.vibrate?.(pattern)
  } catch {
    /* non-fatal */
  }
}
