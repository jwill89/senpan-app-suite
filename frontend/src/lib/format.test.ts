import { describe, it, expect } from 'vitest'
import { slugify, formatSize } from './format'

describe('slugify', () => {
  it('lowercases, trims, and hyphenates whitespace by default', () => {
    expect(slugify('  My Carrd Project  ')).toBe('my-carrd-project')
  })

  it('folds underscores into hyphens and drops other punctuation', () => {
    expect(slugify('Foo_Bar! & Baz')).toBe('foo-bar-baz')
  })

  it('collapses repeated separators and trims leading/trailing ones', () => {
    expect(slugify('--a---b--')).toBe('a-b')
  })

  it('uses underscores (and folds hyphens) when sep is "_"', () => {
    expect(slugify('Event Banners - 2026', '_')).toBe('event_banners_2026')
  })

  it('drops characters outside the allowed set for each separator', () => {
    expect(slugify('a.b/c', '_')).toBe('abc')
    expect(slugify('a.b/c', '-')).toBe('abc')
  })
})

describe('formatSize', () => {
  it('formats bytes under 1 KiB as B', () => {
    expect(formatSize(512)).toBe('512 B')
  })
  it('formats KiB with one decimal', () => {
    expect(formatSize(2048)).toBe('2.0 KB')
  })
  it('formats MiB with one decimal', () => {
    expect(formatSize(3 * 1024 * 1024)).toBe('3.0 MB')
  })
})
