import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { createFreshness } from './freshness'

describe('createFreshness', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00Z'))
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('reports a never-touched key as stale', () => {
    const f = createFreshness()
    expect(f.isStale('a')).toBe(true)
  })

  it('treats a key as fresh until the TTL elapses, then stale again', () => {
    const f = createFreshness(30_000)
    f.touch('a')
    expect(f.isStale('a')).toBe(false)

    vi.advanceTimersByTime(29_999)
    expect(f.isStale('a')).toBe(false)

    vi.advanceTimersByTime(1)
    expect(f.isStale('a')).toBe(true)
  })

  it('tracks keys independently', () => {
    const f = createFreshness()
    f.touch('a')
    expect(f.isStale('a')).toBe(false)
    expect(f.isStale('b')).toBe(true)
  })

  it('invalidate() forces the next check to reload', () => {
    const f = createFreshness()
    f.touch('a')
    expect(f.isStale('a')).toBe(false)
    f.invalidate('a')
    expect(f.isStale('a')).toBe(true)
  })

  it('defaults the key to the empty string', () => {
    const f = createFreshness()
    f.touch()
    expect(f.isStale()).toBe(false)
  })
})
