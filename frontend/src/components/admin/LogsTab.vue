<script setup lang="ts">
/**
 * Admin Server Logs tab (admin-only) — a live-tailing viewer over the backend's
 * structured JSON log, rendered through the shared DataTable.
 *
 * On mount it loads a filtered snapshot from `GET /api/logs`; while open, each
 * new server log line arrives over the shared admin WebSocket (`log` message)
 * and is prepended by the logs store. Common HTTP-request fields (method, status,
 * duration, ip, path) are promoted to typed, colored columns; any other fields
 * expand to full JSON (DataTable's #detail slot). Entries are paginated with a
 * per-page control; the "Debug" toggle flips the server's runtime level live and
 * "Live" pauses the feed for inspection.
 */
import { computed, onMounted, ref, watch } from 'vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import PaginationBar from '@/components/common/ui/PaginationBar.vue'
import { useLogsStore, type LogRow } from '@/stores/logs'

const logs = useLogsStore()

const columns: DataColumn[] = [
  { key: '_expand', label: '', width: '24px' },
  { key: 'time', label: 'Time', width: '108px' },
  { key: 'level', label: 'Level', width: '56px' },
  { key: 'method', label: 'Method', width: '58px' },
  { key: 'status', label: 'Status', width: '52px' },
  { key: 'dur', label: 'Dur', width: '68px' },
  { key: 'ip', label: 'IP', width: '120px' },
  { key: 'user', label: 'User', width: '116px' },
  { key: 'msg', label: 'Path / Message' },
]

// ── Pagination (client-side over the in-memory buffer) ────────────────────────
const page = ref(1)
const perPage = ref(50)
const totalPages = computed(() => Math.max(1, Math.ceil(logs.entries.length / perPage.value)))
const pagedEntries = computed(() => {
  const start = (page.value - 1) * perPage.value
  return logs.entries.slice(start, start + perPage.value)
})
// Keep the page in range as the list shrinks (filter change / 1000-entry cap).
watch(totalPages, (tp) => {
  if (page.value > tp) page.value = tp
})
function goPage(p: number): void {
  page.value = Math.min(Math.max(1, p), totalPages.value)
}
function resetToFirstPage(): void {
  page.value = 1
}

// Debounce the text search so typing doesn't refetch on every keystroke.
let searchTimer: ReturnType<typeof setTimeout> | undefined
function onQueryInput(): void {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    logs.applyFilters()
    resetToFirstPage()
  }, 300)
}
function onLevelChange(): void {
  logs.applyFilters()
  resetToFirstPage()
}

// ── Field accessors (fields is a free-form map; coerce safely) ────────────────
function field(row: LogRow, key: string): unknown {
  return row.fields?.[key]
}
function fieldStr(row: LogRow, key: string): string {
  const v = field(row, key)
  if (v == null) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  return JSON.stringify(v)
}
function fieldNum(row: LogRow, key: string): number | null {
  const v = field(row, key)
  if (typeof v === 'number') return v
  if (typeof v === 'string' && v.trim() !== '' && !Number.isNaN(Number(v))) return Number(v)
  return null
}

/** HTTP method label — the WebSocket upgrade path reads as "WS". */
function methodLabel(row: LogRow): string {
  const m = fieldStr(row, 'method')
  if (!m) return ''
  return fieldStr(row, 'path') === '/api/ws' ? 'WS' : m
}
function methodClass(row: LogRow): string {
  const m = methodLabel(row)
  return m ? `m m-${m.toLowerCase()}` : ''
}
function statusClass(status: number): string {
  if (status >= 500) return 'st st-5xx'
  if (status >= 400) return 'st st-4xx'
  if (status >= 300) return 'st st-3xx'
  if (status >= 200) return 'st st-2xx'
  return 'st'
}

/** Nanoseconds → compact human duration. */
function formatDuration(ns: number | null): string {
  if (ns == null) return ''
  if (ns < 1000) return `${ns}ns`
  if (ns < 1_000_000) return `${(ns / 1000).toFixed(0)}µs`
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(1)}ms`
  return `${(ns / 1_000_000_000).toFixed(2)}s`
}

/** Compact HH:MM:SS.mmm for the row; full ISO timestamp on hover. */
function shortTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return (
    d.toLocaleTimeString(undefined, { hour12: false }) +
    '.' +
    String(d.getMilliseconds()).padStart(3, '0')
  )
}

/** Message shown in the Message column — blank for the redundant "http request"
 *  label (its method/path/status carry the info), otherwise the msg. */
function messageText(row: LogRow): string {
  return row.message === 'http request' ? '' : row.message
}

/** Who made the request: the account username, else a verified-bot name, else
 *  empty (anonymous). `auth` (session|token|bot|anon) drives the styling. */
function actorLabel(row: LogRow): string {
  return fieldStr(row, 'user') || fieldStr(row, 'bot')
}
function actorClass(row: LogRow): string {
  const auth = fieldStr(row, 'auth')
  return auth ? `actor actor-${auth}` : 'actor'
}

const PROMOTED = new Set(['method', 'path', 'status', 'ip', 'duration', 'auth', 'user', 'bot'])
/** Inline preview of the non-promoted fields (full set on expand). */
function extraFieldsPreview(row: LogRow): string {
  if (!row.fields) return ''
  return Object.entries(row.fields)
    .filter(([k]) => !PROMOTED.has(k))
    .map(([k, v]) => `${k}=${typeof v === 'string' ? v : JSON.stringify(v)}`)
    .join('  ')
}

function levelClass(level: string): string {
  const l = level.toUpperCase()
  if (l === 'ERROR') return 'lvl lvl-error'
  if (l === 'WARN' || l === 'WARNING') return 'lvl lvl-warn'
  if (l === 'DEBUG') return 'lvl lvl-debug'
  return 'lvl lvl-info'
}

onMounted(() => void logs.load())
</script>

<template>
  <div class="tab-body">
    <AdminPanel title="Server Logs" :icon="['fad', 'clipboard-clock']">
      <div class="flex-toolbar mb-12">
        <label class="text-dim text-xs">Show:</label>
        <select
          v-model="logs.level"
          aria-label="Minimum level to show"
          style="width: 100px"
          @change="onLevelChange"
        >
          <option value="">All</option>
          <option value="debug">Debug+</option>
          <option value="info">Info+</option>
          <option value="warn">Warn+</option>
          <option value="error">Error</option>
        </select>
        <input
          v-model="logs.query"
          type="search"
          class="log-search"
          placeholder="Filter text…"
          aria-label="Filter logs by text"
          @input="onQueryInput"
        />
        <button
          class="btn-sm"
          :class="logs.live ? 'btn-success' : 'btn-secondary'"
          :title="logs.live ? 'Pause live tail' : 'Resume live tail'"
          @click="logs.toggleLive()"
        >
          <font-awesome-icon :icon="['fas', logs.live ? 'pause' : 'play']" />
          {{ logs.live ? 'Live' : 'Paused' }}
          <span v-if="logs.live" class="live-dot" role="status" aria-label="Live"></span>
        </button>
        <button
          class="btn-sm"
          :class="logs.serverLevel === 'debug' ? 'debug-on' : 'btn-secondary'"
          :disabled="logs.settingLevel"
          :title="
            logs.serverLevel === 'debug'
              ? 'Server-wide DEBUG logging is ON — click to turn off'
              : 'Turn on server-wide DEBUG logging (live, no restart)'
          "
          @click="logs.setDebug(logs.serverLevel !== 'debug')"
        >
          <font-awesome-icon :icon="['fad', 'bug']" />
          Debug {{ logs.serverLevel === 'debug' ? 'On' : 'Off' }}
        </button>
        <label class="text-dim text-xs">Per page:</label>
        <select
          v-model.number="perPage"
          aria-label="Entries per page"
          style="width: 70px"
          @change="resetToFirstPage"
        >
          <option :value="25">25</option>
          <option :value="50">50</option>
          <option :value="100">100</option>
          <option :value="200">200</option>
        </select>
        <span class="text-dim text-xs push-right">{{ logs.entries.length }} shown</span>
        <button class="btn-secondary btn-sm" title="Clear the view" @click="logs.clear()">
          <font-awesome-icon :icon="['fas', 'eraser']" /> Clear
        </button>
      </div>

      <p v-if="logs.truncated" class="text-dim text-xs mb-12">
        Showing the most recent lines; older history is in the rotated log files
        <code v-if="logs.file">({{ logs.file }})</code>.
      </p>

      <LoadingSpinner
        v-if="logs.loading && logs.entries.length === 0"
        block
        label="Loading logs…"
      />
      <template v-else>
        <DataTable :columns="columns" :rows="pagedEntries" row-key="_id" class="log-table">
          <template #cell-_expand="{ row, expanded }">
            <span v-if="row.fields" class="log-caret">{{ expanded ? '▾' : '▸' }}</span>
          </template>
          <template #cell-time="{ row }">
            <span class="log-time" :title="row.time">{{ shortTime(row.time) }}</span>
          </template>
          <template #cell-level="{ row }">
            <span :class="levelClass(row.level)">{{ row.level || '—' }}</span>
          </template>
          <template #cell-method="{ row }">
            <span v-if="methodLabel(row)" :class="methodClass(row)">{{ methodLabel(row) }}</span>
          </template>
          <template #cell-status="{ row }">
            <span
              v-if="fieldNum(row, 'status') !== null"
              :class="statusClass(fieldNum(row, 'status')!)"
              >{{ fieldNum(row, 'status') }}</span
            >
          </template>
          <template #cell-dur="{ row }">
            <span class="log-dur">{{ formatDuration(fieldNum(row, 'duration')) }}</span>
          </template>
          <template #cell-ip="{ row }">
            <span class="log-ip">{{ fieldStr(row, 'ip') }}</span>
          </template>
          <template #cell-user="{ row }">
            <span :class="actorClass(row)" :title="fieldStr(row, 'auth')">{{
              actorLabel(row) || '—'
            }}</span>
          </template>
          <template #cell-msg="{ row, expanded }">
            <span v-if="fieldStr(row, 'path')" class="log-path">{{ fieldStr(row, 'path') }}</span>
            <span v-if="messageText(row)" class="log-msg-text">{{ messageText(row) }}</span>
            <span v-if="!expanded && extraFieldsPreview(row)" class="log-fields-preview">{{
              extraFieldsPreview(row)
            }}</span>
          </template>
          <template #detail="{ row }">
            <pre class="log-detail">{{ JSON.stringify(row.fields, null, 2) }}</pre>
          </template>
          <template #empty>
            <EmptyState v-if="!logs.loading" text="No log entries match the current filters." />
          </template>
        </DataTable>
        <PaginationBar class="mt-12" :page="page" :total-pages="totalPages" @go="goPage" />
      </template>
    </AdminPanel>
  </div>
</template>

<style scoped>
.log-search {
  width: 180px;
}
.log-table {
  font-size: 0.85rem;
}
.log-caret {
  color: var(--text-dim, #888);
}
.log-time,
.log-dur,
.log-ip {
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  color: var(--text-dim, #888);
  font-family: ui-monospace, monospace;
  font-size: 0.78rem;
}
.actor {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: inline-block;
  max-width: 100%;
  font-size: 0.78rem;
}
/* Anonymous rows fade the "—"; a verified bot is italic; a plugin token (token)
   auth and admin (session) auth read as normal account names. */
.actor-anon {
  color: var(--text-dim, #888);
}
.actor-bot {
  font-style: italic;
  color: var(--text-dim, #888);
}
.log-path {
  font-family: ui-monospace, monospace;
  font-size: 0.8rem;
}
.log-msg-text {
  font-weight: 500;
  margin-left: 0.4rem;
}
.log-fields-preview {
  margin-left: 0.5rem;
  color: var(--text-dim, #888);
  font-family: ui-monospace, monospace;
  font-size: 0.78rem;
  opacity: 0.85;
}
.log-detail {
  margin: 0;
  padding: 0.5rem 0.25rem;
  font-family: ui-monospace, monospace;
  font-size: 0.78rem;
  white-space: pre-wrap;
  word-break: break-word;
}
/* Debug toggle (on) — amber, distinct from the green Live button. */
.debug-on {
  color: #1a1a1a;
  background: #e0a100;
  border-color: #e0a100;
}
/* Level badges. */
.lvl {
  display: inline-block;
  padding: 0.05rem 0.4rem;
  border-radius: 4px;
  font-size: 0.7rem;
  font-weight: 700;
  letter-spacing: 0.02em;
}
.lvl-error {
  color: #ff6b6b;
  background: rgba(229, 49, 49, 0.16);
}
.lvl-warn {
  color: #e0a100;
  background: rgba(245, 166, 35, 0.18);
}
.lvl-info {
  color: #5aa0e6;
  background: rgba(90, 160, 230, 0.15);
}
.lvl-debug {
  color: var(--text-dim, #888);
  background: rgba(127, 127, 127, 0.14);
}
/* Method badges. */
.m {
  display: inline-block;
  padding: 0.05rem 0.35rem;
  border-radius: 4px;
  font-size: 0.68rem;
  font-weight: 700;
  font-family: ui-monospace, monospace;
}
.m-get {
  color: #4aa3df;
  background: rgba(74, 163, 223, 0.15);
}
.m-post {
  color: #2ecc71;
  background: rgba(46, 204, 113, 0.15);
}
.m-put {
  color: #e0a100;
  background: rgba(245, 166, 35, 0.16);
}
.m-patch {
  color: #e67e22;
  background: rgba(230, 126, 34, 0.16);
}
.m-delete {
  color: #ff6b6b;
  background: rgba(229, 49, 49, 0.16);
}
.m-ws {
  color: #9b59b6;
  background: rgba(155, 89, 182, 0.16);
}
/* Status codes. */
.st {
  font-variant-numeric: tabular-nums;
  font-weight: 700;
  font-family: ui-monospace, monospace;
  font-size: 0.78rem;
}
.st-2xx {
  color: #2ecc71;
}
.st-3xx {
  color: #4aa3df;
}
.st-4xx {
  color: #e0a100;
}
.st-5xx {
  color: #ff6b6b;
}
</style>
