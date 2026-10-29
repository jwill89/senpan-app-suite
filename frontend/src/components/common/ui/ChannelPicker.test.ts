import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'

const ep = vi.hoisted(() => ({ channels: vi.fn(), roles: vi.fn() }))
vi.mock('@/lib/endpoints', () => ({ endpoints: { discord: ep } }))

import ChannelPicker from './ChannelPicker.vue'
import { useDiscordStore } from '@/stores/discord'

beforeEach(() => {
  setActivePinia(createPinia())
  ep.channels.mockResolvedValue({ channels: [], available: false, problem: '' })
  ep.roles.mockResolvedValue({ roles: [], available: false, problem: '' })
})

function connected(): void {
  const d = useDiscordStore()
  d.directoryAvailable = true
  d.channels = [
    { id: '5', name: 'test-channel', type: 'text', category: '' },
    { id: '2', name: 'announcements', type: 'announcement', category: 'Tea House' },
    { id: '3', name: 'tea-rooms', type: 'text', category: 'Tea House' },
  ]
}

describe('ChannelPicker', () => {
  it('groups channels by category and marks announcement channels', () => {
    connected()
    const w = mount(ChannelPicker, { props: { modelValue: '' } })
    const groups = w.findAll('optgroup').map((g) => g.attributes('label'))
    expect(groups).toEqual(['No category', 'Tea House'])
    const labels = w.findAll('optgroup option').map((o) => o.text())
    expect(labels).toEqual(['#test-channel', '#announcements (announcement)', '#tea-rooms'])
  })

  it('emits the chosen channel id', async () => {
    connected()
    const w = mount(ChannelPicker, { props: { modelValue: '' } })
    await w.find('select').setValue('3')
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['3'])
  })

  it('keeps a stored channel the server no longer has, flagged', () => {
    connected()
    const w = mount(ChannelPicker, { props: { modelValue: '999' } })
    expect(w.find('select').text()).toContain('Channel 999 (not in the server now)')
    expect((w.find('select').element as HTMLSelectElement).value).toBe('999')
  })

  it('falls back to a typed id when the bot is not connected', async () => {
    const d = useDiscordStore()
    d.directoryProblem = 'Paste the bot token to connect the bot.'
    const w = mount(ChannelPicker, { props: { modelValue: '' } })
    expect(w.find('select').exists()).toBe(false)
    expect(w.text()).toContain('Paste the bot token')
    await w.find('input').setValue('12a34')
    expect(w.emitted('update:modelValue')?.at(-1)).toEqual(['1234'])
  })

  it('loads the lists when mounted without them', () => {
    mount(ChannelPicker, { props: { modelValue: '' } })
    expect(ep.channels).toHaveBeenCalled()
  })
})
