/**
 * Shared WebSocket connection with auto-reconnect (exponential back-off) and a
 * client-side keepalive ping. Mirrors the original app.js WebSocket logic.
 *
 * The connection is opened for both the player and admin views. Incoming
 * messages are dispatched to the registered handler. Reconnect is attempted
 * while `shouldReconnect()` returns true.
 */
import type { WsMessage } from '@/types/api'

export interface WsCallbacks {
  /** Called for every parsed message. */
  onMessage: (msg: WsMessage) => void
  /** Called once a reconnect succeeds (to refresh missed state). */
  onReconnected?: () => void
  /** Show a transient notification to the user. */
  notify?: (message: string, type?: 'info' | 'success' | 'error') => void
  /** Whether to keep reconnecting after a close (e.g. still on player/admin view). */
  shouldReconnect: () => boolean
}

const API_BASE = 'api'
const MAX_RECONNECT = 10

export class WsClient {
  private ws: WebSocket | null = null
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private reconnectAttempts = 0
  private keepaliveTimer: ReturnType<typeof setInterval> | null = null
  private cardId: string | null = null
  private cb: WsCallbacks

  constructor(cb: WsCallbacks) {
    this.cb = cb
  }

  /** Builds the ws:// or wss:// URL, optionally with the player's card id. */
  private url(): string {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    let url = `${proto}//${location.host}/${API_BASE}/ws`
    if (this.cardId) {
      url += '?id=' + encodeURIComponent(this.cardId)
    }
    return url
  }

  /** Open (or re-open) the connection. Pass a card id for player connections. */
  connect(cardId?: string | null): void {
    if (cardId !== undefined) this.cardId = cardId

    const attempts = this.reconnectAttempts
    this.disconnect()
    this.reconnectAttempts = attempts

    const isReconnect = this.reconnectAttempts > 0
    const ws = new WebSocket(this.url())

    ws.onopen = () => {
      if (this.reconnectTimer) {
        clearTimeout(this.reconnectTimer)
        this.reconnectTimer = null
      }
      if (isReconnect) {
        this.cb.notify?.('Reconnected!', 'success')
        this.cb.onReconnected?.()
      }
      this.reconnectAttempts = 0
    }

    ws.onmessage = (evt) => {
      let msg: WsMessage
      try {
        msg = JSON.parse(evt.data)
      } catch {
        return
      }
      this.cb.onMessage(msg)
    }

    ws.onclose = () => {
      this.ws = null
      if (this.cb.shouldReconnect()) {
        this.scheduleReconnect()
      }
    }

    ws.onerror = () => {
      // onclose fires after onerror — reconnect handled there.
    }

    this.ws = ws

    // Keepalive: send a text "ping" every 25s to defeat reverse-proxy idle
    // timeouts that only reset on data frames. The Go hub's readPump reads and
    // *discards* all incoming client frames (it never JSON-parses them), so this
    // is a safe no-op server-side — it's purely traffic to keep the link warm.
    // (The server also sends its own protocol-level pings every 30s.)
    if (this.keepaliveTimer) clearInterval(this.keepaliveTimer)
    this.keepaliveTimer = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.ws.send('ping')
      }
    }, 25000)
  }

  /** Close the connection and cancel reconnect/keepalive timers. */
  disconnect(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    this.reconnectAttempts = 0
    if (this.keepaliveTimer) {
      clearInterval(this.keepaliveTimer)
      this.keepaliveTimer = null
    }
    if (this.ws) {
      this.ws.onclose = null // prevent reconnect loop
      this.ws.close()
      this.ws = null
    }
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer) return
    this.reconnectAttempts++

    if (this.reconnectAttempts > MAX_RECONNECT) {
      this.cb.notify?.('Connection lost. Please refresh the page.', 'error')
      this.reconnectAttempts = 0
      return
    }

    const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts - 1), 16000)
    this.cb.notify?.(
      `Connection lost. Reconnecting (${this.reconnectAttempts}/${MAX_RECONNECT})…`,
      'info',
    )

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      this.connect()
    }, delay)
  }
}
