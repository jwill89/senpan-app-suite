import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

// Stub the endpoint layer so the component doesn't hit the network on mount.
const versionFn = vi.fn()
vi.mock('@/lib/endpoints', () => ({ endpoints: { system: { version: () => versionFn() } } }))

import AppVersions from './AppVersions.vue'
import { FRONTEND_VERSION, majorOf } from '@/lib/version'

describe('AppVersions', () => {
  beforeEach(() => versionFn.mockReset())

  it('shows the frontend build version and the fetched backend version', async () => {
    versionFn.mockResolvedValue({ backend: FRONTEND_VERSION })
    const wrapper = mount(AppVersions)
    await flushPromises()

    expect(wrapper.text()).toContain(`v${FRONTEND_VERSION}`) // frontend row
    expect(wrapper.find('.app-versions__flag').exists()).toBe(false) // same major → no warning
  })

  it('flags a major-version mismatch', async () => {
    const otherMajor = `${(majorOf(FRONTEND_VERSION) ?? 0) + 1}.0.0`
    versionFn.mockResolvedValue({ backend: otherMajor })
    const wrapper = mount(AppVersions)
    await flushPromises()

    expect(wrapper.find('.app-versions__flag').exists()).toBe(true)
    expect(wrapper.classes()).toContain('app-versions--warn')
  })
})
