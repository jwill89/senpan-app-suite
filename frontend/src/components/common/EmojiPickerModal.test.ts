import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import EmojiPickerModal from './EmojiPickerModal.vue'

afterEach(() => {
  vi.restoreAllMocks()
  document.body.innerHTML = ''
})

describe('EmojiPickerModal', () => {
  // The real vue3-emoji-picker (2.x), lazy-loaded as in the app. It warns about each 1.x
  // prop it is given, but only once per prop for the whole run, so this is the first and
  // only mount: a clean console says the modal uses the 2.x names.
  it('emits the chosen emoji character, and gives the picker no 1.x props', async () => {
    const warn = vi.spyOn(console, 'warn')
    const wrapper = mount(EmojiPickerModal, { attachTo: document.body })
    let emoji: HTMLElement | null = null
    await vi.waitFor(async () => {
      await flushPromises()
      emoji = document.querySelector<HTMLElement>('.v3-emojis [data-i]')
      expect(emoji).not.toBeNull()
    })

    const fromPicker = warn.mock.calls.filter((args) =>
      String(args[0]).includes('[vue3-emoji-picker]'),
    )
    expect(fromPicker).toEqual([])

    ;(emoji as unknown as HTMLElement).click()
    await flushPromises()
    const [[chosen]] = wrapper.emitted('select') as [[string]]
    expect(chosen).toBeTruthy()
    // The character itself, not a code such as "1f600".
    expect(chosen).not.toMatch(/^[0-9a-f-]+$/i)
    wrapper.unmount()
  })
})
