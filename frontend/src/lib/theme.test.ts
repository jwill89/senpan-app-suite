import { describe, it, expect, afterEach } from 'vitest'
import { clampFontMetrics, fontFamilyFromFile, applyNumberFlourish } from './theme'

describe('applyNumberFlourish', () => {
  afterEach(() => document.documentElement.style.removeProperty('--number-flourish-url'))

  it('sets --number-flourish-url to the resolved asset URL for a relative path', () => {
    applyNumberFlourish('images/flourishes/swirl.svg')
    expect(document.documentElement.style.getPropertyValue('--number-flourish-url')).toBe(
      'url("/images/flourishes/swirl.svg")',
    )
  })

  it('clears the variable when given an empty value (falls back to built-in)', () => {
    applyNumberFlourish('images/flourishes/swirl.svg')
    applyNumberFlourish('')
    expect(document.documentElement.style.getPropertyValue('--number-flourish-url')).toBe('')
  })
})

describe('fontFamilyFromFile', () => {
  it('strips the extension and trims', () => {
    expect(fontFamilyFromFile('Norse.otf')).toBe('Norse')
    expect(fontFamilyFromFile('  My Font.woff2 ')).toBe('My Font')
    expect(fontFamilyFromFile('Playfair-Display.ttf')).toBe('Playfair-Display')
  })
})

describe('clampFontMetrics', () => {
  it('leaves well-proportioned fonts untouched (returns null)', () => {
    // Typical Latin font: ~0.8 ascent + ~0.2 descent ≈ 1.0 em.
    expect(clampFontMetrics(0.8, 0.2)).toBeNull()
    // Just under the threshold.
    expect(clampFontMetrics(1.1, 0.39)).toBeNull()
  })

  it('does not clamp exactly at the threshold (1.5)', () => {
    expect(clampFontMetrics(1.2, 0.3)).toBeNull()
  })

  it('clamps an oversized font proportionally to the target box (1.25)', () => {
    // Oversized: 1.5 + 0.8 = 2.3 em of reserved space.
    const o = clampFontMetrics(1.5, 0.8)
    expect(o).not.toBeNull()
    // Total is scaled down to the 1.25 target…
    expect(o!.ascent + o!.descent).toBeCloseTo(1.25, 5)
    // …while preserving the original ascent:descent ratio (centring is kept).
    expect(o!.ascent / o!.descent).toBeCloseTo(1.5 / 0.8, 5)
  })

  it('clamps a very large box too', () => {
    const o = clampFontMetrics(2.0, 1.0) // total 3.0
    expect(o!.ascent + o!.descent).toBeCloseTo(1.25, 5)
  })

  it('returns null for non-finite or negative inputs', () => {
    expect(clampFontMetrics(NaN, 0.2)).toBeNull()
    expect(clampFontMetrics(0.8, Infinity)).toBeNull()
    expect(clampFontMetrics(-1, 2)).toBeNull()
  })
})
