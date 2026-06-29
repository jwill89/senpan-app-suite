import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'

vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
const config = vi.fn()
vi.mock('@/lib/endpoints', () => ({ endpoints: { system: { config: () => config() } } }))

import RegisterView from './RegisterView.vue'

// Stub the real Turnstile widget so its script-loading onMounted never runs.
const stubs = {
  TurnstileWidget: { name: 'TurnstileWidget', template: '<div class="turnstile-stub" />' },
  LoadingSpinner: true,
  FormField: { template: '<div class="ff"><slot /></div>' },
}

describe('RegisterView Turnstile', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    config.mockReset()
  })

  it('renders the challenge and disables submit until verified when a site key is set', async () => {
    config.mockResolvedValue({ turnstile_site_key: 'site-123' })
    const wrapper = mount(RegisterView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.find('.turnstile-stub').exists()).toBe(true)
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeDefined()

    await wrapper.findComponent({ name: 'TurnstileWidget' }).vm.$emit('verified', 'tok')
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })

  it('shows no challenge and leaves submit enabled when Turnstile is disabled', async () => {
    config.mockResolvedValue({ turnstile_site_key: '' })
    const wrapper = mount(RegisterView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.find('.turnstile-stub').exists()).toBe(false)
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })
})
