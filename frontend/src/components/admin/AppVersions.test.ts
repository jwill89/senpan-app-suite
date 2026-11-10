import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

// Stub the API client so the component doesn't hit the network on mount.
const versionFn = vi.fn()
vi.mock('@/api/client', () => ({ api: { GET: () => versionFn() } }))

import AppVersions from './AppVersions.vue'
import { APP_VERSION } from '@/lib/version'

describe('AppVersions', () => {
  beforeEach(() => versionFn.mockReset())

  it("shows the app's version, with no flag while the server runs it", async () => {
    versionFn.mockResolvedValue({ data: { version: APP_VERSION } })
    const wrapper = mount(AppVersions)
    await flushPromises()

    expect(wrapper.text()).toContain(`v${APP_VERSION}`)
    expect(wrapper.text()).toContain('App')
    expect(wrapper.text()).not.toContain('Backend')
    expect(wrapper.find('.app-versions-flag').exists()).toBe(false)
  })

  it('tells a tab opened before a deploy that the server runs another version', async () => {
    versionFn.mockResolvedValue({ data: { version: '999.0.0' } })
    const wrapper = mount(AppVersions)
    await flushPromises()

    const flag = wrapper.find('.app-versions-flag')
    expect(flag.exists()).toBe(true)
    expect(flag.text()).toContain('v999.0.0')
    expect(wrapper.classes()).toContain('app-versions--warn')
  })

  it("marks the server's version unknown when the call fails", async () => {
    versionFn.mockResolvedValue({ data: undefined, error: { title: 'Bad Gateway', status: 502 } })
    const wrapper = mount(AppVersions)
    await flushPromises()

    expect(wrapper.find('.app-versions-flag').exists()).toBe(false)
    expect(wrapper.classes()).toContain('app-versions--warn')
  })
})
