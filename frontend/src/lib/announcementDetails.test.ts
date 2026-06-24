import { describe, it, expect } from 'vitest'
import { splitDetailParts, detailPartCount, EMBED_FIELD_VALUE_MAX } from './announcementDetails'

describe('splitDetailParts', () => {
  it('returns [] for empty/whitespace input', () => {
    expect(splitDetailParts('')).toEqual([])
    expect(splitDetailParts('   \n  ')).toEqual([])
  })

  it('keeps short details as a single part', () => {
    expect(splitDetailParts('Hello world')).toEqual(['Hello world'])
    expect(detailPartCount('a'.repeat(EMBED_FIELD_VALUE_MAX))).toBe(1)
  })

  it('splits at the last newline within the cap', () => {
    const text = 'a'.repeat(8) + '\n' + 'b'.repeat(8)
    expect(splitDetailParts(text, 10)).toEqual(['a'.repeat(8), 'b'.repeat(8)])
  })

  it('falls back to the last space when there is no newline', () => {
    const text = 'aaaa bbbb cccc' // 14 chars, spaces at 4 and 9
    const parts = splitDetailParts(text, 10)
    expect(parts.every((p) => p.length <= 10)).toBe(true)
    expect(parts.join(' ')).toBe(text)
  })

  it('hard-splits when there is no break within the cap', () => {
    const parts = splitDetailParts('a'.repeat(25), 10)
    expect(parts).toEqual(['a'.repeat(10), 'a'.repeat(10), 'a'.repeat(5)])
  })

  it('reports the part count past the real 1024 cap', () => {
    expect(detailPartCount('a'.repeat(1000) + '\n' + 'b'.repeat(1000))).toBe(2)
  })
})
