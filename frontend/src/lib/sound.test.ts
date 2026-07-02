import { describe, it, expect, beforeEach, vi } from 'vitest'

// Fake <audio> so we can assert the file URL + volume without real playback.
// jsdom has no working HTMLAudioElement.play(); this also records each instance.
const created: { src: string; volume: number; play: ReturnType<typeof vi.fn> }[] = []
class FakeAudio {
  src = ''
  volume = 1
  preload = ''
  currentTime = 0
  play = vi.fn(() => Promise.resolve())
  load = vi.fn()
  constructor(s?: string) {
    if (s !== undefined) this.src = s
    created.push(this)
  }
}

beforeEach(() => {
  created.length = 0
  vi.resetModules() // fresh module = fresh sample cache + volume
  vi.stubGlobal('Audio', FakeAudio)
})

/** Fresh import so each test gets a clean sample cache. */
async function loadSound() {
  return await import('./sound')
}

function sampleFor(file: string) {
  return created.find((a) => a.src.endsWith(`/sounds/${file}.mp3`))
}

describe('playEvent — game mode', () => {
  it('maps each event to its bundled sound file and plays it', async () => {
    const { playEvent, setSoundVolume } = await loadSound()
    setSoundVolume(0.8)
    playEvent('draw', 'game')
    playEvent('minigame', 'game')
    playEvent('gameend', 'game')
    expect(sampleFor('moogle_noise')?.play).toHaveBeenCalled()
    expect(sampleFor('queue_pop')?.play).toHaveBeenCalled()
    expect(sampleFor('level_up')?.play).toHaveBeenCalled()
  })

  it('applies the master volume to the sample', async () => {
    const { playEvent, setSoundVolume } = await loadSound()
    setSoundVolume(0.3)
    playEvent('draw', 'game')
    expect(sampleFor('moogle_noise')?.volume).toBe(0.3)
  })

  it('clamps volume into 0..1', async () => {
    const { playEvent, setSoundVolume } = await loadSound()
    setSoundVolume(9)
    playEvent('draw', 'game')
    expect(sampleFor('moogle_noise')?.volume).toBe(1)
  })

  it('plays nothing when muted (volume 0)', async () => {
    const { playEvent, setSoundVolume } = await loadSound()
    setSoundVolume(0)
    playEvent('draw', 'game')
    expect(created.length).toBe(0)
  })
})

describe('playEvent — basic mode', () => {
  it('does not touch the game samples (uses synthesized chimes)', async () => {
    const { playEvent, setSoundVolume } = await loadSound()
    setSoundVolume(0.8)
    // No AudioContext in jsdom, so the chime no-ops — but crucially it must not
    // fall through to an MP3 sample.
    playEvent('draw', 'basic')
    expect(created.length).toBe(0)
  })
})
