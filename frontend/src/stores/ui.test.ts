import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUiStore } from './ui'

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('toasts', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('notify shows a typed toast', () => {
    const ui = useUiStore()
    ui.notify('saved', 'success')
    expect(ui.toast).toMatchObject({ show: true, message: 'saved', type: 'success' })
  })

  it('auto-dismisses after the given duration', () => {
    const ui = useUiStore()
    ui.notify('bye', 'info', 1000)
    expect(ui.toast.show).toBe(true)
    vi.advanceTimersByTime(999)
    expect(ui.toast.show).toBe(true)
    vi.advanceTimersByTime(1)
    expect(ui.toast.show).toBe(false)
  })

  it('dismissToast hides immediately', () => {
    const ui = useUiStore()
    ui.notify('hi', 'info', 5000)
    ui.dismissToast()
    expect(ui.toast.show).toBe(false)
  })

  it('a new toast resets the previous auto-dismiss timer', () => {
    const ui = useUiStore()
    ui.notify('first', 'info', 1000)
    vi.advanceTimersByTime(800)
    ui.notify('second', 'info', 1000)
    // The first timer would have fired at 1000ms; ensure the new toast is still up.
    vi.advanceTimersByTime(300)
    expect(ui.toast).toMatchObject({ show: true, message: 'second' })
    vi.advanceTimersByTime(700)
    expect(ui.toast.show).toBe(false)
  })
})

describe('confirm dialog', () => {
  it('opens with merged options and resolves true on confirm', async () => {
    const ui = useUiStore()
    const p = ui.confirm('Delete this?', { confirmText: 'Yes' })
    expect(ui.confirmState).toMatchObject({ show: true, message: 'Delete this?', confirmText: 'Yes' })
    ui.resolveConfirm(true)
    await expect(p).resolves.toBe(true)
    expect(ui.confirmState.show).toBe(false)
  })

  it('resolves false on cancel', async () => {
    const ui = useUiStore()
    const p = ui.confirm('Sure?')
    ui.resolveConfirm(false)
    await expect(p).resolves.toBe(false)
  })

  it('auto-resolves a superseded confirm to false', async () => {
    const ui = useUiStore()
    const first = ui.confirm('first?')
    const second = ui.confirm('second?')
    await expect(first).resolves.toBe(false)
    ui.resolveConfirm(true)
    await expect(second).resolves.toBe(true)
  })
})

describe('routeLoading', () => {
  it('setRouteLoading toggles the flag', () => {
    const ui = useUiStore()
    expect(ui.routeLoading).toBe(false)
    ui.setRouteLoading(true)
    expect(ui.routeLoading).toBe(true)
  })
})

describe('wsStatus', () => {
  it('defaults to closed and follows setWsStatus', () => {
    const ui = useUiStore()
    expect(ui.wsStatus).toBe('closed')
    ui.setWsStatus('connecting')
    expect(ui.wsStatus).toBe('connecting')
    ui.setWsStatus('open')
    expect(ui.wsStatus).toBe('open')
  })
})
