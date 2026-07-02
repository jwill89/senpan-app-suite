import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { WsClient, type WsCallbacks } from './ws'

// Minimal stand-in for the browser WebSocket: records the URL + sent frames and
// exposes the lifecycle handlers so tests can drive open/close/message manually.
class MockWS {
  static OPEN = 1
  static instances: MockWS[] = []
  url: string
  readyState = 0
  sent: string[] = []
  closed = false
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  onerror: (() => void) | null = null
  constructor(url: string) {
    this.url = url
    MockWS.instances.push(this)
  }
  send(d: string): void {
    this.sent.push(d)
  }
  close(): void {
    this.closed = true
  }
}

const latest = (): MockWS => MockWS.instances[MockWS.instances.length - 1]

function makeCb(overrides: Partial<WsCallbacks> = {}): WsCallbacks {
  return {
    onMessage: vi.fn(),
    onReconnected: vi.fn(),
    notify: vi.fn(),
    shouldReconnect: vi.fn(() => true),
    onStatus: vi.fn(),
    ...overrides,
  }
}

beforeEach(() => {
  MockWS.instances = []
  vi.stubGlobal('WebSocket', MockWS)
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('WsClient URL building', () => {
  it('builds a ws:// /api/ws URL with no card id', () => {
    new WsClient(makeCb()).connect()
    expect(latest().url).toMatch(/^ws:\/\/.+\/api\/ws$/)
  })

  it('appends a URL-encoded ?id= for player connections', () => {
    new WsClient(makeCb()).connect('a b/c')
    expect(latest().url).toContain('/api/ws?id=a%20b%2Fc')
  })
})

describe('WsClient lifecycle', () => {
  it('reports connecting → open and does not fire onReconnected on the first open', () => {
    const cb = makeCb()
    const c = new WsClient(cb)
    c.connect()
    expect(cb.onStatus).toHaveBeenCalledWith('connecting')
    latest().onopen?.()
    expect(cb.onStatus).toHaveBeenLastCalledWith('open')
    expect(cb.onReconnected).not.toHaveBeenCalled()
  })

  it('dispatches parsed messages and ignores malformed JSON', () => {
    const cb = makeCb()
    new WsClient(cb).connect()
    latest().onmessage?.({ data: '{"type":"draw","number":42}' })
    expect(cb.onMessage).toHaveBeenCalledWith({ type: 'draw', number: 42 })
    latest().onmessage?.({ data: 'not json' })
    expect(cb.onMessage).toHaveBeenCalledTimes(1)
  })

  it('schedules a reconnect on close, then reopens after the backoff delay', () => {
    const cb = makeCb()
    const c = new WsClient(cb)
    c.connect()
    expect(MockWS.instances).toHaveLength(1)

    latest().onclose?.()
    expect(cb.onStatus).toHaveBeenLastCalledWith('reconnecting')
    expect(cb.notify).toHaveBeenCalledWith(expect.stringContaining('(1/10)'), 'info')

    vi.advanceTimersByTime(1000) // first backoff = 1000ms
    expect(MockWS.instances).toHaveLength(2) // reconnected
    latest().onopen?.()
    expect(cb.onReconnected).toHaveBeenCalledTimes(1) // reconnect fires the refresh hook
  })

  it('does not reconnect when shouldReconnect is false', () => {
    const cb = makeCb({ shouldReconnect: vi.fn(() => false) })
    new WsClient(cb).connect()
    latest().onclose?.()
    expect(cb.onStatus).toHaveBeenLastCalledWith('closed')
    vi.advanceTimersByTime(20000)
    expect(MockWS.instances).toHaveLength(1)
  })

  it('gives up after the reconnect cap with a refresh prompt', () => {
    const cb = makeCb()
    const c = new WsClient(cb)
    c.connect()
    // Drive repeated close→backoff cycles until the limit (10) is exceeded.
    for (let i = 0; i < 11; i++) {
      latest().onclose?.()
      vi.advanceTimersByTime(16000) // >= max backoff cap
    }
    expect(cb.notify).toHaveBeenCalledWith(expect.stringContaining('Please refresh'), 'error')
  })

  it('disconnect() closes the socket and stops reconnecting', () => {
    const cb = makeCb()
    const c = new WsClient(cb)
    c.connect()
    const ws = latest()
    c.disconnect()
    expect(ws.closed).toBe(true)
    expect(cb.onStatus).toHaveBeenLastCalledWith('closed')
    vi.advanceTimersByTime(20000)
    expect(MockWS.instances).toHaveLength(1) // no reconnect attempts
  })

  it('sends a keepalive ping every 25s while the socket is open', () => {
    const cb = makeCb()
    new WsClient(cb).connect()
    const ws = latest()
    ws.readyState = MockWS.OPEN
    vi.advanceTimersByTime(25000)
    expect(ws.sent).toContain('ping')
  })
})
