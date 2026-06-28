import { describe, it, expect } from 'vitest'
import { contrastRatio, auditTheme, CAUTION_INK } from './wcag'
import { defaultTokens } from './theme-tokens'

describe('contrastRatio', () => {
  it('returns 21:1 for black on white', () => {
    expect(contrastRatio('#000000', '#ffffff')).toBeCloseTo(21, 0)
  })

  it('returns 1:1 for identical colours', () => {
    expect(contrastRatio('#777777', '#777777')).toBeCloseTo(1, 5)
  })

  it('parses rgb()/modern slash syntax', () => {
    const a = contrastRatio('rgb(0 0 0)', '#ffffff')
    expect(a).toBeCloseTo(21, 0)
  })

  it('returns null for an unparseable colour', () => {
    expect(contrastRatio('not-a-colour', '#fff')).toBeNull()
  })
})

describe('auditTheme', () => {
  it('rates the built-in default theme as AAA with no errors', () => {
    const report = auditTheme(defaultTokens())
    expect(report.level).toBe('AAA')
    expect(report.errors).toHaveLength(0)
    expect(report.warnings).toHaveLength(0)
    expect(report.passes.length).toBe(report.results.length)
  })

  it('flags failures when text matches its background', () => {
    const broken = { ...defaultTokens(), text: '#1a1c17', 'page-bg': '#1a1c17' }
    const report = auditTheme(broken)
    expect(report.level).toBe('fail')
    expect(report.errors.some((r) => r.label.includes('Body text on the page'))).toBe(true)
  })

  it('classifies an AA-but-not-AAA pairing as a warning, not an error', () => {
    // control-border feeds only the neutral-button pair; ~6:1 here is AA but not AAA.
    const tokens = { ...defaultTokens(), 'control-border': '#565a49' }
    const report = auditTheme(tokens)
    expect(report.level).toBe('AA')
    expect(report.errors).toHaveLength(0)
    expect(report.warnings.some((r) => r.label.includes('Neutral button'))).toBe(true)
  })

  it('audits the caution button against the fixed dark ink, not a token', () => {
    // Make warning dark: the fixed ink (#1f1a06) on it should now fail.
    const report = auditTheme({ ...defaultTokens(), warning: '#201a08' })
    expect(report.errors.some((r) => r.label.includes('Caution button'))).toBe(true)
    expect(CAUTION_INK).toBe('#1f1a06')
  })

  it('covers the B-I-N-G-O header letters and the button hover states', () => {
    const ids = auditTheme(defaultTokens()).results.map((r) => r.id)
    // Pairings that earlier sweeps missed — guard against regressions.
    expect(ids).toContain('bingo-top')
    expect(ids).toContain('bingo-bottom')
    expect(ids).toContain('primary-btn-hover')
    expect(ids).toContain('secondary-btn-hover')
  })

  it('flags the BINGO header when --highlight matches a dark board gradient', () => {
    // The Summer-style bug: a dark highlight on a dark board frame.
    const report = auditTheme({
      ...defaultTokens(),
      highlight: '#063644',
      'board-gradient-start': '#0e4055',
      'board-gradient-end': '#0a3142',
    })
    expect(report.errors.some((r) => r.id === 'bingo-top')).toBe(true)
  })

  it('returns resolved colours, token names, and dual AA/AAA flags per pairing', () => {
    const report = auditTheme(defaultTokens())
    const board = report.results.find((r) => r.id === 'board-num')!
    expect(board.fg).toBe('text-on-accent')
    expect(board.bg).toBe('board-cell-bg')
    // Resolved colours are the actual token values, ready for a live swatch.
    expect(board.fgColor).toBe(defaultTokens()['text-on-accent'])
    expect(board.bgColor).toBe(defaultTokens()['board-cell-bg'])
    // status === 'aaa' implies both flags true; every result exposes both.
    for (const r of report.results) {
      expect(typeof r.aaPass).toBe('boolean')
      expect(typeof r.aaaPass).toBe('boolean')
      if (r.status === 'aaa') expect(r.aaPass && r.aaaPass).toBe(true)
      if (r.status === 'fail') expect(r.aaPass).toBe(false)
    }
  })

  it('resolves the caution pairing foreground to the literal ink', () => {
    const caution = auditTheme(defaultTokens()).results.find((r) => r.id === 'caution-btn')!
    expect(caution.fgColor).toBe(CAUTION_INK)
  })
})
