<script lang="ts">
/**
 * Column descriptor for {@link DataTable}. `key` is the field on each row and
 * the `sort` event payload; `align` controls text alignment of the column.
 */
export interface DataColumn {
  key: string
  label: string
  sortable?: boolean
  align?: 'left' | 'center' | 'right'
}
</script>

<script setup lang="ts" generic="T">
/**
 * Generic admin data table — the single table style behind the winners log,
 * raffle entries, and any future tabular admin view (replaces the former
 * `.entries-table` and `.winners-log-table`).
 *
 * Define `columns` (key + label, optional `sortable` and `align`) and pass
 * `rows`. Each cell defaults to `row[col.key]` but can be overridden per column
 * via a `#cell-<key>` slot that receives `{ row }`. Sortable headers show a ▲/▼
 * arrow for the active `sortKey`/`sortDir` and emit `sort` with the column key —
 * the parent owns sort + data state, so sorting can happen server-side.
 *
 * Use the `#empty` slot for an empty-state placeholder (shown when `rows` is
 * empty); the table header stays visible so the layout doesn't jump.
 */
const props = defineProps<{
  columns: DataColumn[]
  rows: T[]
  /** Stable per-row key — a field name or a function deriving one. */
  rowKey: string | ((row: T) => string | number)
  sortKey?: string
  sortDir?: 'asc' | 'desc'
}>()

defineEmits<{ sort: [key: string] }>()

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
            @click="col.sortable && $emit('sort', col.key)"
          >
            {{ col.label }}{{ arrow(col) }}
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="keyFor(row)">
          <td v-for="col in columns" :key="col.key" :class="col.align ? `ta-${col.align}` : ''">
            <slot :name="`cell-${col.key}`" :row="row">{{ cellValue(row, col.key) }}</slot>
          </td>
        </tr>
      </tbody>
    </table>
    <slot v-if="!rows.length" name="empty" />
  </div>
</template>
