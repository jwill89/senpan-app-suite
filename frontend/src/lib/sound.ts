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

/**
 * Resumes the AudioContext from a user gesture (e.g. the moment the player
 * toggles sound on). Browsers start the context suspended until a gesture, so
 * priming here lets the first real draw chime play without delay.
 */
export function primeAudio(): void {
  try {
    const ac = getCtx()
    if (ac && ac.state === 'suspended') void ac.resume()
  } catch {
    /* non-fatal */
  }
}

/** Plays a short rising two-note chime to signal a freshly called number. */
export function playDrawChime(): void {
  try {
    const ac = getCtx()
    if (!ac) return
    if (ac.state === 'suspended') void ac.resume()
    const now = ac.currentTime
    const notes = [
      { freq: 880, at: 0 }, // A5
      { freq: 1318.5, at: 0.12 }, // E6
    ]
    for (const n of notes) {
      const osc = ac.createOscillator()
      const gain = ac.createGain()
      osc.type = 'sine'
      osc.frequency.value = n.freq
      const start = now + n.at
      gain.gain.setValueAtTime(0.0001, start)
      gain.gain.exponentialRampToValueAtTime(0.22, start + 0.02)
      gain.gain.exponentialRampToValueAtTime(0.0001, start + 0.18)
      osc.connect(gain).connect(ac.destination)
      osc.start(start)
      osc.stop(start + 0.2)
    }
  } catch {
    /* non-fatal */
  }
}

/**
 * Plays a short, celebratory ascending arpeggio to signal a new winner. Used by
 * the admin's optional "winner sound" toggle so the caller hears a bingo without
 * watching the screen. Distinct from the single draw chime so the two are easy
 * to tell apart.
 */
export function playWinnerChime(): void {
  try {
    const ac = getCtx()
    if (!ac) return
    if (ac.state === 'suspended') void ac.resume()
    const now = ac.currentTime
    // C5 → E5 → G5 → C6 arpeggio.
    const notes = [
      { freq: 523.25, at: 0 },
      { freq: 659.25, at: 0.1 },
      { freq: 783.99, at: 0.2 },
      { freq: 1046.5, at: 0.3 },
    ]
    for (const n of notes) {
      const osc = ac.createOscillator()
      const gain = ac.createGain()
      osc.type = 'triangle'
      osc.frequency.value = n.freq
      const start = now + n.at
      gain.gain.setValueAtTime(0.0001, start)
      gain.gain.exponentialRampToValueAtTime(0.25, start + 0.02)
      gain.gain.exponentialRampToValueAtTime(0.0001, start + 0.32)
      osc.connect(gain).connect(ac.destination)
      osc.start(start)
      osc.stop(start + 0.34)
    }
  } catch {
    /* non-fatal */
  }
}

/** Triggers a brief device vibration where supported (mobile). */
export function vibrate(pattern: number | number[] = 60): void {
  try {
    navigator.vibrate?.(pattern)
  } catch {
    /* non-fatal */
  }
}
