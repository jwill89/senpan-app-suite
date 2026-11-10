import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))
const config = vi.fn()
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api: { GET: async () => ({ data: await config(), response: new Response(null) }) },
}))

import AdminLoginView from '@/views/AdminLoginView.vue'

// Stub the real Turnstile widget so its script-loading onMounted never runs.
const stubs = {
  TurnstileWidget: { name: 'TurnstileWidget', template: '<div class="turnstile-stub" />' },
  LoadingSpinner: true,
  FormField: { template: '<div class="ff"><slot /></div>' },
}

describe('AdminLoginView Turnstile', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    config.mockReset()
  })

  it('renders the challenge and disables login until verified when a site key is set', async () => {
    config.mockResolvedValue({ turnstileSiteKey: 'site-123' })
    const wrapper = mount(AdminLoginView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.find('.turnstile-stub').exists()).toBe(true)
    // Login stays disabled until a token arrives.
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeDefined()

    // Simulate a completed challenge -> login enables.
    await wrapper.findComponent({ name: 'TurnstileWidget' }).vm.$emit('verified', 'tok')
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })

  it('shows no challenge and leaves login enabled when Turnstile is disabled', async () => {
    config.mockResolvedValue({ turnstileSiteKey: '' })
    const wrapper = mount(AdminLoginView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.find('.turnstile-stub').exists()).toBe(false)
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })
})
