import { describe, it, expect } from 'vitest'
import {
  utcToDatetimeLocal,
  datetimeLocalToUtc,
  parseServerTimestamp,
  formatServerTimestamp,
} from './datetime'

describe('datetime helpers', () => {
  it('returns empty string for empty/invalid input', () => {
    expect(utcToDatetimeLocal('')).toBe('')
    expect(utcToDatetimeLocal(null)).toBe('')
    expect(utcToDatetimeLocal(undefined)).toBe('')
    expect(utcToDatetimeLocal('not-a-date')).toBe('')
    expect(datetimeLocalToUtc('')).toBe('')
    expect(datetimeLocalToUtc(null)).toBe('')
    expect(datetimeLocalToUtc('not-a-date')).toBe('')
  })

  it('round-trips a local input value through UTC unchanged', () => {
    // local → UTC → local is identity regardless of the runner's timezone.
    const local = '2026-06-13T20:00'
    expect(utcToDatetimeLocal(datetimeLocalToUtc(local))).toBe(local)
  })

  it('produces a UTC RFC-3339 (Z) string from a local input', () => {
    const iso = datetimeLocalToUtc('2026-06-13T20:00')
    expect(iso).toMatch(/Z$/)
    // It denotes the same instant as parsing the local value as local time.
    expect(new Date(iso).getTime()).toBe(new Date('2026-06-13T20:00').getTime())
  })

  it('treats a legacy naive timestamp (no zone) as UTC', () => {
    // A zone-less value must be read as UTC — same instant as the explicit Z form.
    expect(utcToDatetimeLocal('2026-06-13T20:00')).toBe(
      utcToDatetimeLocal('2026-06-13T20:00:00.000Z'),
    )
  })

  it('converts a UTC instant to the matching local wall-clock', () => {
    // Build a UTC instant, then check the helper yields that instant's local time.
    const d = new Date(Date.UTC(2026, 5, 13, 20, 0)) // 2026-06-13T20:00:00Z
    const pad = (n: number) => String(n).padStart(2, '0')
    const expected =
      `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
      `T${pad(d.getHours())}:${pad(d.getMinutes())}`
    expect(utcToDatetimeLocal('2026-06-13T20:00:00.000Z')).toBe(expected)
  })
})

describe('parseServerTimestamp', () => {
  it('returns NaN for empty/invalid input', () => {
    expect(parseServerTimestamp('')).toBeNaN()
    expect(parseServerTimestamp(null)).toBeNaN()
    expect(parseServerTimestamp('nonsense')).toBeNaN()
  })

  it('treats a SQLite "YYYY-MM-DD HH:MM:SS" timestamp as UTC, not local', () => {
    // The zone-less space form must be read as UTC — same instant as the Z form.
    expect(parseServerTimestamp('2026-06-13 20:00:00')).toBe(Date.UTC(2026, 5, 13, 20, 0, 0))
  })

  it('treats a zone-less ISO timestamp as UTC', () => {
    expect(parseServerTimestamp('2026-06-13T20:00:00')).toBe(Date.UTC(2026, 5, 13, 20, 0, 0))
  })

  it('respects an explicit zone designator', () => {
    expect(parseServerTimestamp('2026-06-13T20:00:00Z')).toBe(Date.UTC(2026, 5, 13, 20, 0, 0))
    expect(parseServerTimestamp('2026-06-13T20:00:00+00:00')).toBe(Date.UTC(2026, 5, 13, 20, 0, 0))
  })
})

describe('formatServerTimestamp', () => {
  it('returns empty string for invalid input', () => {
    expect(formatServerTimestamp('')).toBe('')
    expect(formatServerTimestamp('nonsense')).toBe('')
  })

  it('formats a UTC timestamp as the local string for that instant', () => {
    const expected = new Date(Date.UTC(2026, 5, 13, 20, 0, 0)).toLocaleString()
    expect(formatServerTimestamp('2026-06-13 20:00:00')).toBe(expected)
  })
})
