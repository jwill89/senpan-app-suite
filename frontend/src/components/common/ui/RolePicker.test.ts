import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'

const ep = vi.hoisted(() => ({ channels: vi.fn(), roles: vi.fn() }))
vi.mock('@/lib/endpoints', () => ({ endpoints: { discord: ep } }))

import RolePicker from './RolePicker.vue'
import { useDiscordStore } from '@/stores/discord'

beforeEach(() => {
  setActivePinia(createPinia())
  ep.channels.mockResolvedValue({ channels: [], available: false, problem: '' })
  ep.roles.mockResolvedValue({ roles: [], available: false, problem: '' })
})

function connected(): void {
  const d = useDiscordStore()
  d.directoryAvailable = true
  d.roles = [
    { id: '30', name: 'Senpan Bot', color: '', managed: true },
    { id: '31', name: 'Staff', color: '#e53170', managed: false },
  ]
}

describe('RolePicker', () => {
  it('offers no ping, @everyone and every role, in the stored format', async () => {
    connected()
    const w = mount(RolePicker, { props: { modelValue: '' } })
    const options = w.findAll('option').map((o) => [o.attributes('value'), o.text()])
    expect(options).toEqual([
      ['', 'No ping'],
      ['everyone', '@everyone'],
      ['role:30', '@Senpan Bot (bot role)'],
      ['role:31', '@Staff'],
    ])
    await w.find('select').setValue('role:31')
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['role:31'])
  })

  it('keeps a stored role the server no longer has, flagged', () => {
    connected()
    const w = mount(RolePicker, { props: { modelValue: 'role:99' } })
    expect(w.text()).toContain('Role 99 (not in the server now)')
    expect((w.find('select').element as HTMLSelectElement).value).toBe('role:99')
  })

  it('takes a typed role id when the bot is not connected', async () => {
    const w = mount(RolePicker, { props: { modelValue: '' } })
    expect(w.find('input').exists()).toBe(false)
    await w.find('select').setValue('role')
    expect(w.emitted('update:modelValue')?.at(-1)).toEqual(['role:'])
    await w.setProps({ modelValue: 'role:' })
    await w.find('input').setValue('3x1')
    expect(w.emitted('update:modelValue')?.at(-1)).toEqual(['role:31'])
  })

  it('shows @everyone as such without a connected bot', () => {
    const w = mount(RolePicker, { props: { modelValue: 'everyone' } })
    expect((w.find('select').element as HTMLSelectElement).value).toBe('everyone')
  })
})
