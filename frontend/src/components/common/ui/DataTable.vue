<script lang="ts">
/**
 * Column descriptor for {@link DataTable}. `key` is the field on each row and
 * the `sort` event payload; `align` controls text alignment; `width` sets a
 * fixed column width (any CSS length) on the header cell.
 */
export interface DataColumn {
  key: string
  label: string
  sortable?: boolean
  align?: 'left' | 'center' | 'right'
  width?: string
}
</script>

<script setup lang="ts" generic="T">
/**
 * Generic admin data table — the single table style behind the winners log,
 * raffle entries, the server-log viewer, and any future tabular admin view
 * (replaces the former `.entries-table` and `.winners-log-table`).
 *
 * Define `columns` (key + label, optional `sortable`, `align`, `width`) and pass
 * `rows`. Each cell defaults to `row[col.key]` but can be overridden per column
 * via a `#cell-<key>` slot that receives `{ row, expanded }`. Sortable headers
 * show a ▲/▼ arrow for the active `sortKey`/`sortDir` and emit `sort` with the
 * column key — the parent owns sort + data state, so sorting can happen
 * server-side.
 *
 * **Expandable rows (opt-in):** provide a `#detail` slot and each row becomes
 * clickable to toggle a full-width detail row rendered beneath it (the slot
 * receives `{ row }`). Cell slots get an `expanded` flag so a column can render
 * an expand caret. Tables without a `#detail` slot behave exactly as before.
 *
 * Use the `#empty` slot for an empty-state placeholder (shown when `rows` is
 * empty); the table header stays visible so the layout doesn't jump.
 */
import { computed, ref, useSlots } from 'vue'

const props = defineProps<{
  columns: DataColumn[]
  rows: T[]
  /** Stable per-row key — a field name or a function deriving one. */
  rowKey: string | ((row: T) => string | number)
  sortKey?: string
  sortDir?: 'asc' | 'desc'
  /** Optional per-row class hook — a static class or a function of the row
   *  (e.g. to highlight a selected row). Applied to each `<tr>`. */
  rowClass?: string | ((row: T) => string | Record<string, boolean>)
}>()

defineEmits<{ sort: [key: string] }>()

const slots = useSlots()
// Rows are expandable only when the parent provides a #detail slot.
const hasDetail = computed(() => Boolean(slots.detail))
const expandedKeys = ref<Set<string | number>>(new Set())
function toggleExpand(row: T): void {
  const k = keyFor(row)
  const next = new Set(expandedKeys.value)
  if (next.has(k)) next.delete(k)
  else next.add(k)
  expandedKeys.value = next
}
function isExpanded(row: T): boolean {
  return expandedKeys.value.has(keyFor(row))
}

/** Toggle expand on row click — but let clicks on interactive cell controls
 *  (buttons, links, form fields) through without also toggling the row. */
function onRowClick(row: T, e: MouseEvent): void {
  if (!hasDetail.value) return
  const target = e.target as HTMLElement | null
  if (target?.closest('button, a, input, select, textarea, label')) return
  toggleExpand(row)
}

/** Enter/Space toggle the row — only when the row itself (not an inner control)
 *  holds focus, so activating a cell button doesn't also expand the row. */
function onRowKeydown(row: T, e: KeyboardEvent): void {
  if (!hasDetail.value || e.target !== e.currentTarget) return
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    toggleExpand(row)
  }
}

/** Stable key for v-for. */
function keyFor(row: T): string | number {
  return typeof props.rowKey === 'function'
    ? props.rowKey(row)
    : ((row as Record<string, unknown>)[props.rowKey] as string | number)
}

/** Default cell value when no `#cell-<key>` slot is provided. */
function cellValue(row: T, key: string): unknown {
  return (row as Record<string, unknown>)[key]
}

/** Sort arrow for the active column, empty otherwise. */
function arrow(col: DataColumn): string {
  if (!col.sortable || props.sortKey !== col.key) return ''
  return props.sortDir === 'asc' ? ' ▲' : ' ▼'
}

/** Resolve the optional per-row class (static string or row-derived). */
function rowClassFor(row: T): string | Record<string, boolean> | undefined {
  return typeof props.rowClass === 'function' ? props.rowClass(row) : props.rowClass
}
</script>

<template>
  <div class="data-table-wrap">
    <table class="data-table">
      <thead>
        <tr>
          <th
            v-for="col in columns"
            :key="col.key"
            :class="[col.align ? `ta-${col.align}` : '', { 'is-sortable': col.sortable }]"
            :style="col.width ? { width: col.width } : undefined"
            @click="col.sortable && $emit('sort', col.key)"
          >
            {{ col.label }}{{ arrow(col) }}
          </th>
        </tr>
      </thead>
      <tbody>
        <template v-for="row in rows" :key="keyFor(row)">
          <tr
            :class="[rowClassFor(row), { 'dt-expandable': hasDetail }]"
            :role="hasDetail ? 'button' : undefined"
            :tabindex="hasDetail ? 0 : undefined"
            :aria-expanded="hasDetail ? isExpanded(row) : undefined"
            @click="onRowClick(row, $event)"
            @keydown="onRowKeydown(row, $event)"
          >
            <td v-for="col in columns" :key="col.key" :class="col.align ? `ta-${col.align}` : ''">
              <slot :name="`cell-${col.key}`" :row="row" :expanded="isExpanded(row)">{{
                cellValue(row, col.key)
              }}</slot>
            </td>
          </tr>
          <tr v-if="hasDetail && isExpanded(row)" class="dt-detail-row">
            <td :colspan="columns.length"><slot name="detail" :row="row" /></td>
          </tr>
        </template>
      </tbody>
    </table>
    <slot v-if="!rows.length" name="empty" />
  </div>
</template>

<style scoped>
.dt-expandable {
  cursor: pointer;
}
.dt-detail-row > td {
  background: rgba(127, 127, 127, 0.08);
}
</style>
