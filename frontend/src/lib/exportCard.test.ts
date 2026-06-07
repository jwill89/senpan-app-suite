import { describe, it, expect } from 'vitest'
import { parseInlineRuns, parseDetailParagraphs } from './exportCard'

describe('parseInlineRuns', () => {
  it('returns a single plain run for unstyled text', () => {
    expect(parseInlineRuns('hello world')).toEqual([
      { text: 'hello world', bold: false, italic: false },
    ])
  })

  it('marks **bold** runs', () => {
    expect(parseInlineRuns('a **b** c')).toEqual([
      { text: 'a ', bold: false, italic: false },
      { text: 'b', bold: true, italic: false },
      { text: ' c', bold: false, italic: false },
    ])
  })

  it('marks *italic* runs', () => {
    expect(parseInlineRuns('*hi*')).toEqual([{ text: 'hi', bold: false, italic: true }])
  })

  it('marks ***bold italic*** runs', () => {
    expect(parseInlineRuns('***x***')).toEqual([{ text: 'x', bold: true, italic: true }])
  })

  it('strips inline-code backticks but keeps the text plain', () => {
    expect(parseInlineRuns('use `code` here')).toEqual([
      { text: 'use ', bold: false, italic: false },
      { text: 'code', bold: false, italic: false },
      { text: ' here', bold: false, italic: false },
    ])
  })
})

describe('parseDetailParagraphs', () => {
  it('splits lines into paragraphs of words', () => {
    const paras = parseDetailParagraphs('first line\nsecond line')
    expect(paras).toHaveLength(2)
    // Each paragraph is an array of words; each word an array of styled chars.
    expect(paras[0].map((w) => w.map((c) => c.ch).join(''))).toEqual(['first', 'line'])
  })

  it('renders headings as bold and drops the # markers', () => {
    const [para] = parseDetailParagraphs('## Prizes')
    const word = para[0]
    expect(word.map((c) => c.ch).join('')).toBe('Prizes')
    expect(word.every((c) => c.bold)).toBe(true)
  })

  it('converts list bullets to a • marker', () => {
    const [para] = parseDetailParagraphs('- item one')
    const firstWord = para[0].map((c) => c.ch).join('')
    expect(firstWord).toBe('•')
  })

  it('strips images and keeps link text', () => {
    const paras = parseDetailParagraphs('see ![alt](img.png) [our site](https://x.test)')
    const text = paras
      .flat()
      .map((w) => w.map((c) => c.ch).join(''))
      .join(' ')
    expect(text).toContain('our')
    expect(text).toContain('site')
    expect(text).not.toContain('img.png')
    expect(text).not.toContain('https://x.test')
  })

  it('ignores blank lines', () => {
    expect(parseDetailParagraphs('\n\n   \n')).toEqual([])
  })

  it('preserves bold styling across a paragraph word', () => {
    const [para] = parseDetailParagraphs('plain **loud**')
    const loud = para.find((w) => w.map((c) => c.ch).join('') === 'loud')
    expect(loud?.every((c) => c.bold)).toBe(true)
  })
})
