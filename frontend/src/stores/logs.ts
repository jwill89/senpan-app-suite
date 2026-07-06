/**
 * Server-log viewer store (admin-only).
 *
 * Holds the newest-first tail of the backend's structured log. On load it
 * fetches a filtered snapshot from `GET /api/logs`; while open it also receives
 * a live feed — each server log line arrives as a `log` WebSocket message and is
 * prepended via {@link appendLive} (see useWebSocket). The in-memory list is
 * capped so a long-running session can't grow unbounded, mirroring the server's
 * own rotation/retention.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { LogEntry } from '@/types/api'
import { endpoints } from '@/lib/endpoints'
import { useUiStore } from '@/stores/ui'

/** Level filter: '' = all. Others are minimum severities. */
export type LogLevelFilter = '' | 'debug' | 'info' | 'warn' | 'error'

/** A log row with a stable client-side id for the table key + expand tracking. */
export interface LogRow extends LogEntry {
  _id: number
}

/** Most entries kept in memory (matches the server's default fetch limit). */
const MAX_ENTRIES = 1000

// slog numeric severities, keyed by the uppercase level name in the JSON.
// Optional-valued so an unrecognized level reads as undefined (and is dropped
// when a level filter is active) rather than being mistyped as always-present.
const LEVEL_VALUE: Record<string, number | undefined> = {
  DEBUG: -4,
  INFO: 0,
  WARN: 4,
  WARNING: 4,
  ERROR: 8,
}
const FILTER_MIN: Record<Exclude<LogLevelFilter, ''>, number> = {
  debug: -4,
  info: 0,
  warn: 4,
  error: 8,
}

// Monotonic id source for row keys (stable across re-renders and live prepends).
let seq = 0

export const useLogsStore = defineStore('logs', () => {
  const ui = useUiStore()

  const entries = ref<LogRow[]>([]) // newest-first
  const loading = ref(false)
  const truncated = ref(false) // server dropped older lines past its read cap
  const file = ref('')
  const level = ref<LogLevelFilter>('') // view filter (minimum level)
  const query = ref('')
  const live = ref(true) // live tail on by default; pause to freeze the view
  const serverLevel = ref('info') // process-wide runtime level (server-side)
  const settingLevel = ref(false) // a level change is in flight

  /** Whether an entry passes the active level + text filters (for the live feed;
   *  the initial snapshot is filtered server-side). */
  function passesFilter(e: LogEntry): boolean {
    if (level.value) {
      const v = LEVEL_VALUE[(e.level || '').toUpperCase()]
      if (v === undefined || v < FILTER_MIN[level.value]) return false
    }
    if (query.value) {
      const q = query.value.toLowerCase()
      const hay = `${e.message} ${e.level} ${JSON.stringify(e.fields ?? {})}`.toLowerCase()
      if (!hay.includes(q)) return false
    }
    return true
  }

  /** Fetch the current filtered snapshot (replaces the list). */
  async function load(): Promise<void> {
    loading.value = true
    try {
      const data = await endpoints.logs.list({
        level: level.value,
        q: query.value,
        limit: MAX_ENTRIES,
      })
      entries.value = data.entries.map((e) => ({ ...e, _id: ++seq }))
      truncated.value = data.truncated
      file.value = data.file
      serverLevel.value = data.level || 'info'
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      loading.value = false
    }
  }

  /** Turn live DEBUG logging on/off server-side (affects the whole process). */
  async function setDebug(on: boolean): Promise<void> {
    settingLevel.value = true
    try {
      const data = await endpoints.logs.setLevel(on ? 'debug' : 'info')
      serverLevel.value = data.level
      ui.notify(`Server log level set to ${data.level.toUpperCase()}`, 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      settingLevel.value = false
    }
  }

  /** Prepend a live log line (from the WebSocket feed), honoring pause + filters. */
  function appendLive(entry: LogEntry): void {
    if (!live.value || !passesFilter(entry)) return
    entries.value.unshift({ ...entry, _id: ++seq })
    if (entries.value.length > MAX_ENTRIES) entries.value.length = MAX_ENTRIES
  }

  /** Re-fetch when a filter changes (the historical part is filtered server-side). */
  function applyFilters(): void {
    void load()
  }

  /** Empty the in-memory view without touching the server's files. */
  function clear(): void {
    entries.value = []
  }

  /** Toggle the live tail; resuming re-loads to catch up on missed lines. */
  function toggleLive(): void {
    live.value = !live.value
    if (live.value) void load()
  }

  return {
    entries,
    loading,
    truncated,
    file,
    level,
    query,
    live,
    serverLevel,
    settingLevel,
    load,
    appendLive,
    applyFilters,
    clear,
    toggleLive,
    setDebug,
  }
})
