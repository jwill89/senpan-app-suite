/**
 * A live event stream: the server's word that something changed, over server-sent
 * events. The stream carries what changed; the page reads the state again over
 * REST, so `onOpen` runs on every open and reopen (anything may have been missed
 * while it was closed) and when the tab comes back into view.
 *
 * The browser retries a dropped stream by itself and resumes it from the last event
 * id. When it gives up (an error answer, as for a moment while the server restarts),
 * the stream is opened again with back-off from 1 second to 16. A stream that goes
 * quiet for nearly three of the server's 25-second pings has died without saying
 * so, and is opened again too. One Feed is one stream; a page keeps one per tab.
 */
import type { paths } from '@/api/schema'

type Stream<P extends '/events' | '/board/{id}/events'> =
  paths[P]['get']['responses'][200]['content']['text/event-stream'][number]['data']

/** An event on the staff's stream (GET /api/v1/events), as the API describes it. */
export type StaffEvent = Stream<'/events'>
/** An event on a board's stream (GET /api/v1/board/{id}/events). */
export type BoardEvent = Stream<'/board/{id}/events'>

/** Where the stream is, for a "Live" badge. */
export type FeedStatus = 'closed' | 'connecting' | 'open' | 'reconnecting'

/** One handler for each event a stream carries, by its type. */
export type Handlers<E extends { type: string }> = {
  [K in E['type']]: (event: Extract<E, { type: K }>) => void
}

export interface FeedOptions<E extends { type: string }> {
  /** The stream's URL. */
  url: string
  /** What each event does. Every event type of the stream has one. */
  handlers: Handlers<E>
  /**
   * Reads again the state the page shows: on every open (`reopened` after a drop)
   * and when the tab comes back into view.
   */
  onOpen?: (reopened: boolean) => void
  /** The stream's status changed. */
  onStatus?: (status: FeedStatus) => void
  /** The server refused the stream (an error answer rather than a dropped line). */
  onRefused?: () => void
  /** Says something to the user (a lost connection, and its return). */
  notify?: (message: string, type: 'info' | 'success') => void
}

const FIRST_REOPEN_MS = 1000
const LAST_REOPEN_MS = 16_000
/** Nearly three missed pings: the line is dead even if nothing has said so. */
export const QUIET_MS = 70_000
/** A drop the browser mends within a moment, as a phone's does on waking, is not worth a word. */
export const GRACE_MS = 4000

export class Feed<E extends { type: string }> {
  private source: EventSource | null = null
  private reopenTimer: ReturnType<typeof setTimeout> | undefined
  private quietTimer: ReturnType<typeof setTimeout> | undefined
  private graceTimer: ReturnType<typeof setTimeout> | undefined
  private attempts = 0
  private dropped = false
  private announced = false
  private running = false
  private readonly opts: FeedOptions<E>

  constructor(opts: FeedOptions<E>) {
    this.opts = opts
  }

  /** Opens the stream, and keeps it open until stop. */
  start(): void {
    if (this.running) {
      return
    }
    this.running = true
    this.attempts = 0
    this.dropped = false
    this.announced = false
    document.addEventListener('visibilitychange', this.onVisible)
    this.open()
  }

  /** Closes the stream for good. */
  stop(): void {
    if (!this.running) {
      return
    }
    this.running = false
    clearTimeout(this.reopenTimer)
    clearTimeout(this.quietTimer)
    clearTimeout(this.graceTimer)
    this.source?.close()
    this.source = null
    document.removeEventListener('visibilitychange', this.onVisible)
    this.opts.onStatus?.('closed')
  }

  private open(): void {
    clearTimeout(this.reopenTimer)
    this.source?.close()
    this.opts.onStatus?.(this.dropped ? 'reconnecting' : 'connecting')
    const source = new EventSource(this.opts.url)
    this.source = source
    this.heard()
    for (const name of Object.keys(this.opts.handlers)) {
      source.addEventListener(name, (e) => this.receive(source, name, e as MessageEvent<string>))
    }
    source.onopen = () => {
      if (source !== this.source) {
        return
      }
      clearTimeout(this.graceTimer)
      this.attempts = 0
      this.heard()
      this.opts.onStatus?.('open')
      if (this.announced) {
        this.opts.notify?.('Reconnected!', 'success')
      }
      const reopened = this.dropped
      this.dropped = false
      this.announced = false
      this.opts.onOpen?.(reopened)
    }
    source.onerror = () => {
      if (source !== this.source) {
        return
      }
      this.lost()
      // The browser retries a dropped line itself; when it has given up instead, the
      // stream is opened again here.
      if (source.readyState === EventSource.CLOSED) {
        this.opts.onRefused?.()
        this.reopenLater()
      }
    }
  }

  private receive(source: EventSource, name: string, e: MessageEvent<string>): void {
    if (source !== this.source) {
      return
    }
    this.heard()
    let event: E
    try {
      event = JSON.parse(e.data) as E
    } catch {
      // A malformed event is the server's to log; the next one stands alone.
      return
    }
    if (event.type !== name) {
      return
    }
    // The event's type is the handler's key, so the handler takes this event.
    const handlers = this.opts.handlers as unknown as Record<
      string,
      ((event: E) => void) | undefined
    >
    handlers[name]?.(event)
  }

  /** Marks the stream as dropped, saying so if it stays down past the grace. */
  private lost(): void {
    if (!this.dropped) {
      this.dropped = true
      this.opts.onStatus?.('reconnecting')
      clearTimeout(this.graceTimer)
      this.graceTimer = setTimeout(() => {
        this.announced = true
        this.opts.notify?.('Connection lost. Reconnecting...', 'info')
      }, GRACE_MS)
    }
  }

  private reopenLater(): void {
    clearTimeout(this.reopenTimer)
    clearTimeout(this.quietTimer)
    this.source?.close()
    this.source = null
    const delay = Math.min(FIRST_REOPEN_MS * 2 ** this.attempts, LAST_REOPEN_MS)
    this.attempts++
    this.reopenTimer = setTimeout(() => this.open(), delay)
  }

  /** The line is alive: silence for long enough from here is a drop. */
  private heard(): void {
    clearTimeout(this.quietTimer)
    this.quietTimer = setTimeout(() => {
      this.lost()
      this.reopenLater()
    }, QUIET_MS)
  }

  private readonly onVisible = (): void => {
    if (!this.running || document.visibilityState !== 'visible') {
      return
    }
    // A phone drops the line while its screen sleeps: open it again at once rather
    // than waiting out a back-off, and read the state again either way.
    if (!this.source || this.source.readyState === EventSource.CLOSED) {
      this.attempts = 0
      this.open()
      return
    }
    this.opts.onOpen?.(true)
  }
}
