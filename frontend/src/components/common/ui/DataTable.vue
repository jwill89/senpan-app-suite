<script lang="ts">
/**
 * Column descriptor for {@link DataTable}. `key` is the field on each row;
 * `align` controls text alignment; `width` sets the column's width (any CSS
 * length; the starting width when the table is `resizable`).
 */
export interface DataColumn {
  key: string
  label: string
  sortable?: boolean
  align?: 'left' | 'center' | 'right'
  width?: string
  /** Offer this column's distinct values as a facet (see `DataTableView.facets`). */
  facetable?: boolean
  /** Keep this column out of a CSV export (action buttons, expand carets). */
  noExport?: boolean
}

/** One distinct value of a facetable column, with how many rows carry it. */
export interface DataFacet {
  value: string
  count: number
}

/** What the table reports back about the current view (counts, pager, facets). */
export interface DataTableView {
  /** Rows remaining after the filters - what "12 links" style counts mean. */
  total: number
  /** Page count at the current `pageSize` (always >= 1). */
  totalPages: number
  /** Distinct values per `facetable` column, most frequent first. */
  facets: Record<string, DataFacet[]>
}
</script>

<script setup lang="ts" generic="T extends Record<string, any>">
/**
 * Generic admin data table - the single table style behind every tabular admin
 * view (cards, fonts, server logs, garapon links + draws, raffle + rally logs,
 * the winners log).
 *
 * **The table owns the row pipeline: filter -> sort -> paginate.** Pass the FULL
 * row list; the table does the rest. This ordering is the whole point - sorting
 * must happen before pagination, or a click on a header only reorders the page
 * you happen to be looking at, which looks right and is wrong. That is why the
 * pipeline cannot be split between the table and its parent.
 *
 * The pipeline is TanStack Table, which is headless: it supplies the row model
 * only. Every element below is still our own markup with our own classes, and
 * nothing here is styled by a library - the theme tokens and the WCAG audit see
 * this table exactly as before.
 *
 * Core usage: `columns` + `rows`, plus any of
 *  - `filter` / `filter-fn` - search text and how a row matches it (default:
 *    a case-insensitive substring test across every column's value).
 *  - `page-size` + `v-model:page` - omit `page-size` to render every row.
 *  - `default-sort` - the column the view opens on.
 *
 * Opt-in features, each off unless asked for:
 *  - `resizable` - drag a column edge to resize it. `width` becomes the start.
 *  - `selectable` + `v-model:selected` - a checkbox column plus a header box
 *    that selects/clears every row matching the CURRENT filter (not just the
 *    page), for bulk actions like "Delete selected".
 *  - `group-by` - keep rows sharing that column's value contiguous. Implemented
 *    as a primary sort key, so column sorting still works *within* each group.
 *  - `column-filters` - `{ columnKey: value }` exact-match filters, for facet
 *    dropdowns built from `DataTableView.facets` (mark a column `facetable`).
 *  - `exportCsv(filename?)` via a template ref - writes the rows currently
 *    matched by the filters, in the current sort order, across ALL pages.
 *
 * `@update:view` reports `{ total, totalPages, facets }` so a toolbar can show a
 * result count, drive facet dropdowns, and `PaginationBar` can live in the
 * parent where it belongs.
 *
 * Each cell defaults to `row[col.key]` but can be overridden per column via a
 * `#cell-<key>` slot receiving `{ row, expanded }`. Use `#empty` for an
 * empty-state placeholder; the header stays visible so the layout doesn't jump.
 *
 * **Expandable rows (opt-in):** provide a `#detail` slot and each row becomes
 * clickable to toggle a full-width detail row beneath it (the slot receives
 * `{ row }`). Cell slots get an `expanded` flag so a column can render a caret.
 */
import { computed, ref, useSlots, watch } from 'vue'
import {
  columnFacetingFeature,
  columnFilteringFeature,
  columnResizingFeature,
  columnSizingFeature,
  createFacetedRowModel,
  createFacetedUniqueValues,
  createFilteredRowModel,
  createPaginatedRowModel,
  createSortedRowModel,
  globalFilteringFeature,
  rowPaginationFeature,
  rowSortingFeature,
  tableFeatures,
  useTable,
  type ColumnDef,
  type ColumnFiltersState,
  type SortingState,
} from '@tanstack/vue-table'

// v9 composes the engine from explicit features instead of bundling everything:
// each capability is opted into here, and a row model must come AFTER the feature
// it depends on. The core row model is implicit now, so it has no entry. This is
// the whole engine this table uses - filtering (global + per-column facets),
// sorting, paging, and interactive column resizing (which needs BOTH the sizing
// and resizing features).
const features = tableFeatures({
  columnFilteringFeature,
  globalFilteringFeature,
  rowSortingFeature,
  rowPaginationFeature,
  columnFacetingFeature,
  columnSizingFeature,
  columnResizingFeature,
  filteredRowModel: createFilteredRowModel(),
  sortedRowModel: createSortedRowModel(),
  paginatedRowModel: createPaginatedRowModel(),
  facetedRowModel: createFacetedRowModel(),
  facetedUniqueValues: createFacetedUniqueValues(),
})

const props = defineProps<{
  columns: DataColumn[]
  /** The FULL row list - the table filters, sorts and paginates it. */
  rows: T[]
  /** Stable per-row key - a field name or a function deriving one. */
  rowKey: string | ((row: T) => string | number)
  /** Optional per-row class hook - a static class or a function of the row
   *  (e.g. to highlight a selected row). Applied to each `<tr>`. */
  rowClass?: string | ((row: T) => string | Record<string, boolean>)
  /** Search text. Empty/undefined disables filtering. */
  filter?: string
  /** How a row matches the (lowercased, trimmed) query. Defaults to a substring
   *  test across every column value - pass one to search specific fields. */
  filterFn?: (row: T, query: string) => boolean
  /** Exact-match filters by column key; '' or absent means "no filter". */
  columnFilters?: Record<string, string>
  /** Rows per page. Omit to render every row without pagination. */
  pageSize?: number
  /** The column the view opens on. */
  defaultSort?: { key: string; dir?: 'asc' | 'desc' }
  /** Keep rows sharing this column's value together (primary sort key). */
  groupBy?: string
  /** Allow dragging column edges to resize. */
  resizable?: boolean
  /** Render a leading checkbox column for bulk actions. */
  selectable?: boolean
}>()

const emit = defineEmits<{ 'update:view': [view: DataTableView] }>()

/** Current page, 1-based. `v-model:page` from the parent, which owns the pager. */
const page = defineModel<number>('page', { default: 1 })
/** Selected row keys. `v-model:selected`; only meaningful with `selectable`. */
const selected = defineModel<(string | number)[]>('selected', { default: () => [] })

const slots = useSlots()
// Rows are expandable only when the parent provides a #detail slot.
const hasDetail = computed(() => Boolean(slots.detail))
const expandedKeys = ref<Set<string | number>>(new Set())

/** Stable key for v-for and for TanStack's row id. */
function keyFor(row: T): string | number {
  return typeof props.rowKey === 'function'
    ? props.rowKey(row)
    : ((row as Record<string, unknown>)[props.rowKey] as string | number)
}

const cellOf = (row: T, key: string): unknown => (row as Record<string, unknown>)[key]

/** Cell value as comparable/searchable text. Narrowed explicitly rather than
 *  `String(v)` so an object cell yields its JSON instead of "[object Object]". */
function asText(v: unknown): string {
  if (v == null) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  return JSON.stringify(v)
}

/**
 * Locale- and numeric-aware compare on the raw cell value, which orders text,
 * numbers and this app's fixed-format timestamps correctly. Kept explicit rather
 * than using a built-in so sorting behaves identically to the hand-written
 * comparators this replaced.
 */
function compareCells(a: unknown, b: unknown): number {
  return asText(a).localeCompare(asText(b), undefined, { numeric: true })
}

// -- Row pipeline -------------------------------------------------------------
const userSorting = ref<SortingState>(
  props.defaultSort ? [{ id: props.defaultSort.key, desc: props.defaultSort.dir === 'desc' }] : [],
)

/**
 * `group-by` is a PRIMARY sort key, not TanStack's row grouping. Grouping here
 * means "keep a participant's rows together", not "collapse them into an
 * aggregate row" - so prepending the group column to the sort keeps every row
 * visible and still lets a column sort order rows within each group.
 */
const sorting = computed<SortingState>(() => {
  const active = userSorting.value
  if (!props.groupBy || active[0]?.id === props.groupBy) return active
  return [{ id: props.groupBy, desc: false }, ...active]
})

/** Live column widths, keyed by column id - populated only once a user drags. */
const sizing = ref<Record<string, number>>({})

const filterState = computed<ColumnFiltersState>(() =>
  Object.entries(props.columnFilters ?? {})
    .filter(([, v]) => v !== '')
    .map(([id, value]) => ({ id, value })),
)

const tableColumns = computed<ColumnDef<typeof features, T>[]>(() =>
  props.columns.map((col) => ({
    id: col.key,
    accessorFn: (row: T) => cellOf(row, col.key),
    enableSorting: Boolean(col.sortable),
    enableResizing: props.resizable,
    size: col.width ? Number.parseInt(col.width, 10) || 150 : 150,
    // A first click is always ascending. TanStack otherwise flips numeric columns
    // to descending-first, which would silently reverse the meaning of a click on
    // half the admin tables relative to how they behaved before.
    sortDescFirst: false,
    sortFn: (rowA, rowB, id) => compareCells(rowA.getValue(id), rowB.getValue(id)),
    filterFn: (row, id, value: string) => asText(row.getValue(id)) === value,
  })),
)

const table = useTable<typeof features, T>({
  features,
  get data() {
    return props.rows
  },
  get columns() {
    return tableColumns.value
  },
  state: {
    get sorting() {
      return sorting.value
    },
    get globalFilter() {
      return props.filter?.trim().toLowerCase() ?? ''
    },
    get columnFilters() {
      return filterState.value
    },
    get columnSizing() {
      return sizing.value
    },
    get pagination() {
      return {
        pageIndex: Math.max(0, page.value - 1),
        pageSize: props.pageSize ?? Number.MAX_SAFE_INTEGER,
      }
    },
  },
  getRowId: (row) => String(keyFor(row)),
  onSortingChange: (updater) => {
    const next = typeof updater === 'function' ? updater(sorting.value) : updater
    // Drop the injected group key so the user's own sort is what round-trips.
    userSorting.value = props.groupBy ? next.filter((s) => s.id !== props.groupBy) : next
  },
  enableColumnResizing: true,
  columnResizeMode: 'onChange',
  onColumnSizingChange: (updater) => {
    sizing.value = typeof updater === 'function' ? updater(sizing.value) : updater
  },
  globalFilterFn: (row, _columnId, query: string) => {
    if (!query) return true
    const original = row.original
    if (props.filterFn) return props.filterFn(original, query)
    return props.columns.some((c) => asText(cellOf(original, c.key)).toLowerCase().includes(query))
  },
})

/** Rows after the filters AND the sort, before paging - the count, the CSV, and
 *  select-all all mean "everything matching, in the order shown". Deliberately
 *  the SORTED model: TanStack's filtered model runs before sorting, so using it
 *  would export and select in source order rather than the order on screen. */
const matchedRows = computed(() => table.getSortedRowModel().rows)
const totalPages = computed(() =>
  props.pageSize ? Math.max(1, Math.ceil(matchedRows.value.length / props.pageSize)) : 1,
)
/** The rows actually rendered. */
const visibleRows = computed(() => table.getRowModel().rows.map((r) => r.original))

/** Distinct values per facetable column, most frequent first. */
const facets = computed<Record<string, DataFacet[]>>(() => {
  const out: Record<string, DataFacet[]> = {}
  for (const col of props.columns) {
    if (!col.facetable) continue
    const uniq = table.getColumn(col.key)?.getFacetedUniqueValues()
    if (!uniq) continue
    out[col.key] = [...uniq.entries()]
      .map(([value, count]) => ({ value: asText(value), count }))
      .filter((f) => f.value !== '')
      .sort((a, b) => b.count - a.count || a.value.localeCompare(b.value))
  }
  return out
})

// Any change that can shrink the result set returns to page 1; a page left past
// the end (e.g. after filtering) clamps back into range.
// Anything that changes which rows land on a page sends you back to page 1:
// a filter, a facet, or a new page size. (Sorting does the same, below.)
watch(
  [() => props.filter, filterState, () => props.pageSize],
  () => {
    page.value = 1
  },
  { deep: true },
)
watch(sorting, () => (page.value = 1))
watch(
  totalPages,
  (n) => {
    if (page.value > n) page.value = n
  },
  { immediate: true },
)
watch(
  [matchedRows, totalPages, facets],
  ([rows, pages, f]) => emit('update:view', { total: rows.length, totalPages: pages, facets: f }),
  { immediate: true, deep: true },
)

// -- Selection ----------------------------------------------------------------
const selectedSet = computed(() => new Set(selected.value))
/** Header checkbox reflects EVERY matching row, not just the visible page. */
const allMatchedSelected = computed(
  () =>
    matchedRows.value.length > 0 &&
    matchedRows.value.every((r) => selectedSet.value.has(keyFor(r.original))),
)
const someMatchedSelected = computed(
  () =>
    !allMatchedSelected.value &&
    matchedRows.value.some((r) => selectedSet.value.has(keyFor(r.original))),
)

function toggleRowSelected(row: T): void {
  const k = keyFor(row)
  selected.value = selectedSet.value.has(k)
    ? selected.value.filter((s) => s !== k)
    : [...selected.value, k]
}
/** Select or clear every row the current filters match, across all pages. */
function toggleSelectAll(): void {
  const keys = matchedRows.value.map((r) => keyFor(r.original))
  if (allMatchedSelected.value) {
    const drop = new Set(keys)
    selected.value = selected.value.filter((s) => !drop.has(s))
  } else {
    selected.value = [...new Set([...selected.value, ...keys])]
  }
}

// -- CSV ----------------------------------------------------------------------
/** RFC 4180 field: quote when it contains a comma, quote or newline. */
function csvField(v: string): string {
  return /[",\n\r]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v
}

/**
 * Download the rows the filters currently match, in the current sort order,
 * across every page - i.e. what the user believes they are looking at, not the
 * page on screen and not the unfiltered source. Columns marked `noExport`
 * (action buttons, expand carets) are left out.
 */
function exportCsv(filename = 'export.csv'): void {
  const cols = props.columns.filter((c) => !c.noExport && c.label !== '')
  const head = cols.map((c) => csvField(c.label)).join(',')
  const body = matchedRows.value
    .map((r) => cols.map((c) => csvField(asText(cellOf(r.original, c.key)))).join(','))
    .join('\r\n')
  // The BOM makes Excel read it as UTF-8 rather than the local codepage.
  const blob = new Blob(['﻿' + head + '\r\n' + body], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename.endsWith('.csv') ? filename : `${filename}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

defineExpose({ exportCsv })

// -- Expandable rows ----------------------------------------------------------
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

/** Toggle expand on row click - but let clicks on interactive cell controls
 *  (buttons, links, form fields) through without also toggling the row. */
function onRowClick(row: T, e: MouseEvent): void {
  if (!hasDetail.value) return
  const target = e.target as HTMLElement | null
  if (target?.closest('button, a, input, select, textarea, label')) return
  toggleExpand(row)
}

/** Enter/Space toggle the row - only when the row itself (not an inner control)
 *  holds focus, so activating a cell button doesn't also expand the row. */
function onRowKeydown(row: T, e: KeyboardEvent): void {
  if (!hasDetail.value || e.target !== e.currentTarget) return
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    toggleExpand(row)
  }
}

// -- Header -------------------------------------------------------------------
function onHeaderClick(col: DataColumn): void {
  if (col.sortable) table.getColumn(col.key)?.toggleSorting()
}

/** Sort-direction icon for the active column; null when this column isn't sorted.
 *  A FontAwesome chevron rather than a text glyph, so it renders in the icon font
 *  everywhere instead of depending on the platform having the arrow characters. */
function sortIcon(col: DataColumn): ['fas', string] | null {
  if (!col.sortable) return null
  const dir = table.getColumn(col.key)?.getIsSorted()
  return dir ? ['fas', dir === 'asc' ? 'chevron-up' : 'chevron-down'] : null
}

/**
 * Header width. A column the user has actually dragged uses its live px size;
 * everything else keeps its declared `width` (or none, so the browser sizes it
 * to content). Reading `getSize()` unconditionally would pin every unsized
 * column to TanStack's 150px default the moment a table became resizable,
 * silently relaying out tables nobody had touched.
 */
function headerStyle(col: DataColumn): Record<string, string> | undefined {
  if (props.resizable && col.key in sizing.value) {
    return { width: `${table.getColumn(col.key)?.getSize() ?? 150}px` }
  }
  return col.width ? { width: col.width } : undefined
}

/** Start a drag-resize from the column's edge grip. */
function onResizeStart(col: DataColumn, e: MouseEvent | TouchEvent): void {
  const header = table.getHeaderGroups()[0]?.headers.find((h) => h.column.id === col.key)
  header?.getResizeHandler()(e)
}

/** Resolve the optional per-row class (static string or row-derived). */
function rowClassFor(row: T): string | Record<string, boolean> | undefined {
  return typeof props.rowClass === 'function' ? props.rowClass(row) : props.rowClass
}
</script>

<template>
  <div class="data-table-wrap">
    <table class="data-table" :class="{ 'is-resizable': resizable }">
      <thead>
        <tr>
          <th v-if="selectable" class="dt-select-col">
            <input
              type="checkbox"
              :checked="allMatchedSelected"
              :indeterminate="someMatchedSelected"
              :aria-label="allMatchedSelected ? 'Clear selection' : 'Select all matching rows'"
              @change="toggleSelectAll"
            />
          </th>
          <th
            v-for="col in columns"
            :key="col.key"
            :class="[col.align ? `ta-${col.align}` : '', { 'is-sortable': col.sortable }]"
            :style="headerStyle(col)"
            @click="onHeaderClick(col)"
          >
            {{ col.label }}
            <font-awesome-icon
              v-if="sortIcon(col)"
              :icon="sortIcon(col)!"
              class="dt-sort-icon"
              aria-hidden="true"
            />
            <span
              v-if="resizable"
              class="dt-resizer"
              role="separator"
              aria-orientation="vertical"
              :aria-label="`Resize ${col.label} column`"
              @click.stop
              @mousedown="onResizeStart(col, $event)"
              @touchstart.passive="onResizeStart(col, $event)"
            />
          </th>
        </tr>
      </thead>
      <tbody>
        <template v-for="row in visibleRows" :key="keyFor(row)">
          <tr
            :class="[
              rowClassFor(row),
              {
                'dt-expandable': hasDetail,
                'is-selected': selectable && selectedSet.has(keyFor(row)),
              },
            ]"
            :role="hasDetail ? 'button' : undefined"
            :tabindex="hasDetail ? 0 : undefined"
            :aria-expanded="hasDetail ? isExpanded(row) : undefined"
            @click="onRowClick(row, $event)"
            @keydown="onRowKeydown(row, $event)"
          >
            <td v-if="selectable" class="dt-select-col">
              <input
                type="checkbox"
                :checked="selectedSet.has(keyFor(row))"
                aria-label="Select row"
                @change="toggleRowSelected(row)"
              />
            </td>
            <td v-for="col in columns" :key="col.key" :class="col.align ? `ta-${col.align}` : ''">
              <slot :name="`cell-${col.key}`" :row="row" :expanded="isExpanded(row)">{{
                cellOf(row, col.key)
              }}</slot>
            </td>
          </tr>
          <tr v-if="hasDetail && isExpanded(row)" class="dt-detail-row">
            <td :colspan="columns.length + (selectable ? 1 : 0)">
              <slot name="detail" :row="row" />
            </td>
          </tr>
        </template>
      </tbody>
    </table>
    <slot v-if="!visibleRows.length" name="empty" />
  </div>
</template>

<style scoped>
/* Sort chevron: smaller than the label and dimmed, so it marks the active column
   without competing with the header text. */
.dt-sort-icon {
  margin-left: 0.35em;
  font-size: 0.72em;
  color: var(--highlight);
}
</style>
