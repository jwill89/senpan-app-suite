import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// The app store imports the endpoints layer at module load; stub it so the store
// can be instantiated without a real API.
vi.mock('@/lib/endpoints', () => ({ endpoints: { styles: {}, settings: {} } }))

import { useAppStore } from './app'

beforeEach(() => setActivePinia(createPinia()))
afterEach(() => document.documentElement.style.removeProperty('--number-flourish-url'))

describe('app applyFlourishes', () => {
  it('stores both flourishes and applies the number-flourish CSS variable', () => {
    const app = useAppStore()
    app.applyFlourishes('images/flourishes/board.svg', 'images/flourishes/num.svg')
    expect(app.activeBoardFlourish).toBe('images/flourishes/board.svg')
    expect(app.activeNumberFlourish).toBe('images/flourishes/num.svg')
    expect(document.documentElement.style.getPropertyValue('--number-flourish-url')).toBe(
      'url("/images/flourishes/num.svg")',
    )
  })

  it('clears the refs and the CSS variable when given empty values', () => {
    const app = useAppStore()
    app.applyFlourishes('images/flourishes/board.svg', 'images/flourishes/num.svg')
    app.applyFlourishes('', '')
    expect(app.activeBoardFlourish).toBe('')
    expect(app.activeNumberFlourish).toBe('')
    expect(document.documentElement.style.getPropertyValue('--number-flourish-url')).toBe('')
  })
})
