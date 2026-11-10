import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { LogEntry, Logs } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, response }`.
const ok = (data: unknown) => ({ data, response: new Response(null) })
const snapshot: Logs = {
  entries: [
    { time: '2026-07-05T19:00:00Z', level: 'INFO', message: 'started' },
    { time: '2026-07-05T19:00:01Z', level: 'ERROR', message: 'boom', fields: { code: 500 } },
  ],
  file: '/var/log/senpan/senpan.log',
  truncated: true,
  level: 'info',
}
const api = vi.hoisted(() => ({ GET: vi.fn(), PUT: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { useLogsStore } from './logs'

const entry = (level: string, message: string, fields?: Record<string, unknown>): LogEntry => ({
  time: '2026-07-05T19:00:02Z',
  level,
  message,
  fields,
})

beforeEach(() => {
  setActivePinia(createPinia())
  api.GET.mockReset().mockResolvedValue(ok(snapshot))
  api.PUT.mockReset().mockImplementation(async (_path: string, init: { body: unknown }) =>
    ok(init.body),
  )
})

describe('load', () => {
  it('maps the snapshot newest-first with stable ids, and captures truncated/file/level', async () => {
    const s = useLogsStore()
    await s.load()
    expect(s.entries).toHaveLength(2)
    expect(s.entries[0]!.message).toBe('started')
    expect(s.entries[0]!._id).not.toBe(s.entries[1]!._id)
    expect(s.truncated).toBe(true)
    expect(s.file).toBe('/var/log/senpan/senpan.log')
    expect(s.serverLevel).toBe('info')
  })

  it('sends only the filters that are set', async () => {
    const s = useLogsStore()
    await s.load()
    expect(api.GET).toHaveBeenLastCalledWith('/logs', {
      params: { query: { level: undefined, q: undefined, limit: 1000 } },
    })
    s.level = 'warn'
    s.query = 'lookup'
    await s.load()
    expect(api.GET).toHaveBeenLastCalledWith('/logs', {
      params: { query: { level: 'warn', q: 'lookup', limit: 1000 } },
    })
  })
})

describe('setDebug', () => {
  it('turns debug on/off server-side and tracks the returned level', async () => {
    const s = useLogsStore()
    await s.setDebug(true)
    expect(api.PUT).toHaveBeenCalledWith('/logs/level', { body: { level: 'debug' } })
    expect(s.serverLevel).toBe('debug')
    await s.setDebug(false)
    expect(api.PUT).toHaveBeenCalledWith('/logs/level', { body: { level: 'info' } })
    expect(s.serverLevel).toBe('info')
  })
})

describe('appendLive', () => {
  it('prepends a passing entry (newest-first)', () => {
    const s = useLogsStore()
    s.appendLive(entry('INFO', 'first'))
    s.appendLive(entry('WARN', 'second'))
    expect(s.entries.map((e) => e.message)).toEqual(['second', 'first'])
  })

  it('drops entries below the minimum-level filter', () => {
    const s = useLogsStore()
    s.level = 'warn'
    s.appendLive(entry('INFO', 'noise'))
    s.appendLive(entry('ERROR', 'kept'))
    expect(s.entries.map((e) => e.message)).toEqual(['kept'])
  })

  it('drops entries that do not match the text filter (message, level, or fields)', () => {
    const s = useLogsStore()
    s.query = 'lookup'
    s.appendLive(entry('INFO', 'unrelated'))
    s.appendLive(entry('INFO', 'hit', { path: '/api/bookclub/lookup' }))
    expect(s.entries.map((e) => e.message)).toEqual(['hit'])
  })

  it('ignores live entries while paused', () => {
    const s = useLogsStore()
    s.live = false
    s.appendLive(entry('ERROR', 'while paused'))
    expect(s.entries).toHaveLength(0)
  })
})

describe('toggleLive', () => {
  it('re-loads when resuming to catch up', () => {
    const s = useLogsStore()
    s.live = false
    s.toggleLive() // resume
    expect(s.live).toBe(true)
    expect(api.GET).toHaveBeenCalledTimes(1)
  })
})
