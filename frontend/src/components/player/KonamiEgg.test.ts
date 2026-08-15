import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

// The player store pulls in the sound lib at import time; stub it so mounting
// doesn't reach for the Web Audio API under jsdom.
vi.mock('@/lib/sound', () => ({ setSoundVolume: vi.fn(), playWinnerChime: vi.fn() }))

import KonamiEgg from './KonamiEgg.vue'
import { usePlayerStore } from '@/stores/player'

const KONAMI = [
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

/** Fires `keys` as keydown events, from `target` when one is given. */
function press(keys: string[], target: EventTarget = window): void {
  for (const key of keys) target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
}

/** A player store with a stamped board, as if mid-game. */
function stampedStore(): ReturnType<typeof usePlayerStore> {
  const player = usePlayerStore()
  player.stamps = { '0-0': true, '1-2': true }
  return player
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})
afterEach(() => vi.useRealTimers())

describe('KonamiEgg', () => {
  it('clears the stamps and reveals the image on the full sequence', async () => {
    const player = stampedStore()
    const wrapper = mount(KonamiEgg)

    press(KONAMI)
    await wrapper.vm.$nextTick()

    expect(player.stamps).toEqual({})
    expect(wrapper.find('.konami-egg-art').attributes('src')).toContain('DraniGrin.webp')
    expect(wrapper.find('.konami-egg-line').text()).toBe("Oi, what'd you think was gonna happen?")
  })

  it('renders nothing until the code is entered', () => {
    stampedStore()
    const wrapper = mount(KonamiEgg)
    expect(wrapper.find('.konami-egg').exists()).toBe(false)
  })

  it('accepts the letters in either case', async () => {
    const player = stampedStore()
    const wrapper = mount(KonamiEgg)

    press([...KONAMI.slice(0, 8), 'B', 'A'])
    await wrapper.vm.$nextTick()

    expect(player.stamps).toEqual({})
    expect(wrapper.find('.konami-egg').exists()).toBe(true)
  })

  it('ignores a wrong sequence', async () => {
    const player = stampedStore()
    const wrapper = mount(KonamiEgg)

    press([...KONAMI.slice(0, 9), 'x'])
    await wrapper.vm.$nextTick()

    expect(player.stamps).toEqual({ '0-0': true, '1-2': true })
    expect(wrapper.find('.konami-egg').exists()).toBe(false)
  })

  it('still fires when a key is fumbled before the real run', async () => {
    // The rolling window means the trailing ten keys are what count, so a stray
    // extra Up (or any other lead-in) does not poison the attempt.
    const player = stampedStore()
    const wrapper = mount(KonamiEgg)

    press(['ArrowUp', 'ArrowDown', 'ArrowUp', ...KONAMI])
    await wrapper.vm.$nextTick()

    expect(player.stamps).toEqual({})
    expect(wrapper.find('.konami-egg').exists()).toBe(true)
  })

  it('ignores keys typed into a field', async () => {
    const player = stampedStore()
    const wrapper = mount(KonamiEgg)
    const input = document.createElement('input')
    document.body.appendChild(input)

    press(KONAMI, input)
    await wrapper.vm.$nextTick()

    expect(player.stamps).toEqual({ '0-0': true, '1-2': true })
    expect(wrapper.find('.konami-egg').exists()).toBe(false)
    input.remove()
  })

  it('removes the reveal once the animation is over', async () => {
    vi.useFakeTimers()
    stampedStore()
    const wrapper = mount(KonamiEgg)

    press(KONAMI)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.konami-egg').exists()).toBe(true)

    vi.advanceTimersByTime(3000)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.konami-egg').exists()).toBe(false)
  })

  it('stops listening once unmounted', async () => {
    const player = stampedStore()
    const wrapper = mount(KonamiEgg)
    wrapper.unmount()

    press(KONAMI)
    expect(player.stamps).toEqual({ '0-0': true, '1-2': true })
  })
})
