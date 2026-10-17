import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'

const push = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))

import ToastNotification from './ToastNotification.vue'
import { useUiStore } from '@/stores/ui'

beforeEach(() => {
  setActivePinia(createPinia())
  push.mockReset()
})

describe('ToastNotification alerts', () => {
  it('renders each alert with its message, above the toast', async () => {
    const ui = useUiStore()
    ui.alert({
      title: 'Tea room unlocked',
      message: 'Jasmine Room (Room #3) has been marked as unlocked.',
    })
    ui.notify('Saved', 'success')
    const w = mount(ToastNotification)
    await w.vm.$nextTick()
    const toasts = w.findAll('.toast')
    expect(toasts).toHaveLength(2)
    expect(toasts[0].classes()).toContain('toast--alert')
    expect(toasts[0].text()).toContain('Jasmine Room (Room #3)')
  })

  it('clicking the card does not dismiss it; its close button does', async () => {
    const ui = useUiStore()
    ui.alert({ title: 'Tea room unlocked', message: 'check it' })
    const w = mount(ToastNotification)
    await w.find('.toast--alert').trigger('click')
    expect(ui.alerts).toHaveLength(1)
    await w.find('.toast-close').trigger('click')
    expect(ui.alerts).toHaveLength(0)
  })

  it('the action button navigates and dismisses', async () => {
    const ui = useUiStore()
    ui.alert({
      title: 'Tea room unlocked',
      message: 'check it',
      action: { label: 'Open Tea Rooms', to: '/admin/teahouse/tea-rooms' },
    })
    const w = mount(ToastNotification)
    await w.find('.toast-actions button').trigger('click')
    expect(push).toHaveBeenCalledWith('/admin/teahouse/tea-rooms')
    expect(ui.alerts).toHaveLength(0)
  })
})
