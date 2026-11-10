import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { Feed, GRACE_MS, QUIET_MS, type FeedOptions } from './feed'

// A stand-in for the browser's EventSource: records the URL and the listeners, and
// lets a test drive open, error and events by hand.
class MockSource {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 2
  static instances: MockSource[] = []
  url: string
  readyState = MockSource.CONNECTING
  closed = false
  listeners = new Map<string, ((e: MessageEvent<string>) => void)[]>()
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  constructor(url: string) {
    this.url = url
    MockSource.instances.push(this)
  }
  addEventListener(name: string, fn: (e: MessageEvent<string>) => void): void {
    this.listeners.set(name, [...(this.listeners.get(name) ?? []), fn])
  }
  close(): void {
    this.closed = true
    this.readyState = MockSource.CLOSED
  }
  open(): void {
    this.readyState = MockSource.OPEN
    this.onopen?.()
  }
  emit(name: string, data: unknown): void {
    const raw = typeof data === 'string' ? data : JSON.stringify(data)
    for (const fn of this.listeners.get(name) ?? []) {
      fn({ data: raw } as MessageEvent<string>)
    }
  }
  /** The line dropped and the browser is retrying it. */
  drop(): void {
    this.readyState = MockSource.CONNECTING
    this.onerror?.()
  }
  /** The browser gave up: an error answer. */
  refuse(): void {
    this.readyState = MockSource.CLOSED
    this.onerror?.()
  }
}

const latest = (): MockSource => MockSource.instances[MockSource.instances.length - 1]!

type TestEvent = { type: 'draw'; n: number } | { type: 'ping' }

function makeFeed(overrides: Partial<FeedOptions<TestEvent>> = {}) {
  const draw = vi.fn()
  const opts: FeedOptions<TestEvent> = {
    url: '/api/v1/events',
    handlers: { draw, ping: vi.fn() },
    onOpen: vi.fn(),
    onStatus: vi.fn(),
    onRefused: vi.fn(),
    notify: vi.fn(),
    ...overrides,
  }
  return { feed: new Feed(opts), opts, draw }
}

function setVisibility(state: 'visible' | 'hidden'): void {
  Object.defineProperty(document, 'visibilityState', { value: state, configurable: true })
  document.dispatchEvent(new Event('visibilitychange'))
}

beforeEach(() => {
  MockSource.instances = []
  vi.stubGlobal('EventSource', MockSource)
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  setVisibility('visible')
})

describe('opening', () => {
  it('opens the URL, says connecting then open, and reads the state on the first open', () => {
    const { feed, opts } = makeFeed()
    feed.start()
    expect(latest().url).toBe('/api/v1/events')
    expect(opts.onStatus).toHaveBeenCalledWith('connecting')
    latest().open()
    expect(opts.onStatus).toHaveBeenLastCalledWith('open')
    expect(opts.onOpen).toHaveBeenCalledWith(false)
    feed.stop()
  })

  it('listens for every event it has a handler for', () => {
    const { feed } = makeFeed()
    feed.start()
    expect([...latest().listeners.keys()].sort()).toEqual(['draw', 'ping'])
    feed.stop()
  })

  it('starting twice opens one stream', () => {
    const { feed } = makeFeed()
    feed.start()
    feed.start()
    expect(MockSource.instances).toHaveLength(1)
    feed.stop()
  })
})

describe('dispatch', () => {
  it('hands each event to its handler, and drops a malformed one or one whose type is not its name', () => {
    const { feed, draw } = makeFeed()
    feed.start()
    latest().open()
    latest().emit('draw', { type: 'draw', n: 7 })
    expect(draw).toHaveBeenCalledWith({ type: 'draw', n: 7 })
    latest().emit('draw', 'not json')
    latest().emit('draw', { type: 'ping' })
    expect(draw).toHaveBeenCalledTimes(1)
    feed.stop()
  })

  it('ignores an event from a stream it has replaced', () => {
    const { feed, draw } = makeFeed()
    feed.start()
    const old = latest()
    old.refuse()
    vi.advanceTimersByTime(1000)
    old.emit('draw', { type: 'draw', n: 1 })
    expect(draw).not.toHaveBeenCalled()
    feed.stop()
  })
})

describe('reopening', () => {
  it('leaves a dropped line to the browser, and reads the state again when it is back', () => {
    const { feed, opts } = makeFeed()
    feed.start()
    latest().open()
    latest().drop()
    expect(opts.onStatus).toHaveBeenLastCalledWith('reconnecting')
    expect(MockSource.instances).toHaveLength(1)
    latest().open()
    expect(opts.onOpen).toHaveBeenLastCalledWith(true)
    feed.stop()
  })

  it('opens a refused stream again with back-off from 1 second to 16', () => {
    const { feed, opts } = makeFeed()
    feed.start()
    const waits = [1000, 2000, 4000, 8000, 16000, 16000]
    for (const [i, wait] of waits.entries()) {
      latest().refuse()
      expect(opts.onRefused).toHaveBeenCalledTimes(i + 1)
      vi.advanceTimersByTime(wait - 1)
      expect(MockSource.instances).toHaveLength(i + 1)
      vi.advanceTimersByTime(1)
      expect(MockSource.instances).toHaveLength(i + 2)
    }
    // Once it opens, the back-off starts again from a second.
    latest().open()
    latest().refuse()
    vi.advanceTimersByTime(1000)
    expect(MockSource.instances).toHaveLength(waits.length + 2)
    feed.stop()
  })

  it('opens a stream that went quiet again, and a ping keeps it alive', () => {
    const { feed, opts } = makeFeed()
    feed.start()
    latest().open()
    vi.advanceTimersByTime(QUIET_MS - 1000)
    latest().emit('ping', { type: 'ping' })
    vi.advanceTimersByTime(QUIET_MS - 1000)
    expect(MockSource.instances).toHaveLength(1)
    vi.advanceTimersByTime(1000)
    expect(opts.onStatus).toHaveBeenLastCalledWith('reconnecting')
    expect(latest().closed).toBe(true)
    vi.advanceTimersByTime(1000)
    expect(MockSource.instances).toHaveLength(2)
    latest().open()
    expect(opts.onOpen).toHaveBeenLastCalledWith(true)
    feed.stop()
  })

  it('says the line is lost only once it stays down past a moment, and says when it is back', () => {
    const { feed, opts } = makeFeed()
    feed.start()
    latest().open()
    latest().drop()
    vi.advanceTimersByTime(GRACE_MS - 1)
    latest().open()
    expect(opts.notify).not.toHaveBeenCalled()

    latest().drop()
    vi.advanceTimersByTime(GRACE_MS)
    expect(opts.notify).toHaveBeenCalledWith('Connection lost. Reconnecting...', 'info')
    latest().open()
    expect(opts.notify).toHaveBeenLastCalledWith('Reconnected!', 'success')
    feed.stop()
  })
})

describe('the tab waking', () => {
  it('opens a stream the browser gave up on at once, and reads the state again', () => {
    const { feed, opts } = makeFeed()
    feed.start()
    latest().open()
    latest().refuse()
    setVisibility('hidden')
    setVisibility('visible')
    expect(MockSource.instances).toHaveLength(2)
    latest().open()
    expect(opts.onOpen).toHaveBeenLastCalledWith(true)
    feed.stop()
  })

  it('reads the state again over a stream still open', () => {
    const { feed, opts } = makeFeed()
    feed.start()
    latest().open()
    setVisibility('visible')
    expect(MockSource.instances).toHaveLength(1)
    expect(opts.onOpen).toHaveBeenLastCalledWith(true)
    feed.stop()
  })
})

describe('stopping', () => {
  it('closes the stream, says closed, and never opens it again', () => {
    const { feed, opts } = makeFeed()
    feed.start()
    const source = latest()
    source.open()
    feed.stop()
    expect(source.closed).toBe(true)
    expect(opts.onStatus).toHaveBeenLastCalledWith('closed')
    vi.advanceTimersByTime(QUIET_MS * 2)
    setVisibility('visible')
    expect(MockSource.instances).toHaveLength(1)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('cancels a pending reopen', () => {
    const { feed } = makeFeed()
    feed.start()
    latest().refuse()
    feed.stop()
    vi.advanceTimersByTime(20_000)
    expect(MockSource.instances).toHaveLength(1)
  })
})
