import { describe, it, expect } from 'vitest'
import {
  THEME_TOKENS,
  defaultTokens,
  withDefaults,
  tokensToCss,
  toHex,
  toHex8,
  toRgb,
} from './theme-tokens'

describe('defaultTokens', () => {
  it('seeds a value for every token', () => {
    const d = defaultTokens()
    expect(Object.keys(d)).toHaveLength(THEME_TOKENS.length)
    expect(d['page-bg']).toBe('#1a1c17')
    expect(d['modal-overlay']).toBe('rgb(0 0 0 / 70%)')
  })
})

describe('colour helpers', () => {
  it('toHex renders opaque #rrggbb (alpha dropped)', () => {
    expect(toHex({ r: 26, g: 28, b: 23, a: 0.5 })).toBe('#1a1c17')
  })

  it('toHex8 appends the alpha byte', () => {
    expect(toHex8({ r: 0, g: 0, b: 0, a: 1 })).toBe('#000000ff')
    expect(toHex8({ r: 255, g: 255, b: 255, a: 0 })).toBe('#ffffff00')
  })

  it('toRgb uses modern slash syntax, omitting alpha when fully opaque', () => {
    expect(toRgb({ r: 0, g: 0, b: 0, a: 0.7 })).toBe('rgb(0 0 0 / 70%)')
    expect(toRgb({ r: 214, g: 189, b: 174, a: 1 })).toBe('rgb(214 189 174)')
  })
})

describe('token metadata', () => {
  it('gives every token a label and a usage description', () => {
    for (const t of THEME_TOKENS) {
      expect(t.label.length, `${t.name} label`).toBeGreaterThan(0)
      expect(t.desc.length, `${t.name} desc`).toBeGreaterThan(0)
    }
  })
})

describe('withDefaults', () => {
  it('overlays saved tokens over the defaults and drops unknown keys', () => {
    const merged = withDefaults({ 'page-bg': '#000', bogus: 'x' })
    expect(merged['page-bg']).toBe('#000') // overridden
    expect(merged['panel-bg']).toBe('#272a22') // default kept
    expect('bogus' in merged).toBe(false) // unknown dropped
  })

  it('returns full defaults for undefined input', () => {
    expect(Object.keys(withDefaults(undefined))).toHaveLength(THEME_TOKENS.length)
  })
})

describe('tokensToCss', () => {
  it('renders a :root block in canonical order, prefixing --', () => {
    const css = tokensToCss({ accent: '#fff', 'page-bg': '#000' })
    // page-bg precedes accent in canonical order regardless of input order.
    expect(css).toBe(':root{--page-bg:#000;--accent:#fff;}')
  })

  it('skips empty values and unknown tokens', () => {
    expect(tokensToCss({ 'page-bg': '  ', bogus: 'x' })).toBe('')
  })

  it('is empty for an empty map', () => {
    expect(tokensToCss({})).toBe('')
  })
})
