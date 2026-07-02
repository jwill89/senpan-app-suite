/**
 * Client-side table view state: search → sort → paginate, in one place.
 *
 * Admin tables repeat the same trio — a search box, optional column sorting, and
 * a pager — each with its own refs, derived lists, and page-clamping watchers.
 * This composable owns that pipeline so a tab just supplies the source rows, a
 * match predicate, and (optionally) a starting sort; it gets back the slice to
 * render plus the bindings for `SearchInput`, `DataTable`, and `PaginationBar`.
 *
 * Sorting is a locale-aware, numeric-aware string compare on the active column
 * key — which orders text, numbers, and the app's fixed-format timestamps
 * correctly — matching the behavior the tabs previously hand-rolled.
 */
import { computed, ref, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'

/** Options for {@link useDataTableView}. */
export interface DataTableViewOptions<T> {
  /** Whether a row matches the (already lowercased, trimmed) search query. */
  matches: (row: T, query: string) => boolean
  /** Rows per page (default 10). */
  perPage?: number
  /** Initial sort column + direction. Omit for an unsorted view. */
  sort?: { key: string; dir?: 'asc' | 'desc' }
}

/** The reactive bindings returned by {@link useDataTableView}. */
export interface DataTableView<T> {
  search: Ref<string>
  page: Ref<number>
  /** Active sort column key ('' when the view is unsorted). */
  sortKey: Ref<string>
  sortDir: Ref<'asc' | 'desc'>
  /** Rows after search + sort (before pagination) — for result counts. */
  filtered: Ref<T[]>
  /** The current page's rows (what the table renders). */
  paged: Ref<T[]>
  totalPages: Ref<number>
  /** Toggle a column's direction, or switch to it ascending — a `@sort` handler. */
  setSort: (key: string) => void
  /** Clear the search, sort, and page back to their initial state. */
  reset: () => void
}

export function useDataTableView<T>(
  source: MaybeRefOrGetter<T[]>,
  options: DataTableViewOptions<T>,
): DataTableView<T> {
  const perPage = options.perPage ?? 10
  const initialSortKey = options.sort?.key ?? ''
  const initialSortDir = options.sort?.dir ?? 'asc'

  const search = ref('')
  const page = ref(1)
  const sortKey = ref(initialSortKey)
  const sortDir = ref<'asc' | 'desc'>(initialSortDir)

  const searched = computed(() => {
    const q = search.value.trim().toLowerCase()
    const rows = toValue(source)
    return q ? rows.filter((r) => options.matches(r, q)) : rows
  })

  const filtered = computed(() => {
    if (!sortKey.value) return searched.value
    const key = sortKey.value
    const dir = sortDir.value === 'asc' ? 1 : -1
    // Copy before sorting so the source array isn't mutated in place.
    return [...searched.value].sort((a, b) => {
      const av = String((a as Record<string, string | number | null | undefined>)[key] ?? '')
      const bv = String((b as Record<string, string | number | null | undefined>)[key] ?? '')
      return av.localeCompare(bv, undefined, { numeric: true }) * dir
    })
  })

  const totalPages = computed(() => Math.max(1, Math.ceil(filtered.value.length / perPage)))
  const paged = computed(() => {
    const start = (page.value - 1) * perPage
    return filtered.value.slice(start, start + perPage)
  })

  function setSort(key: string): void {
    if (sortKey.value === key) {
      sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
    } else {
      sortKey.value = key
      sortDir.value = 'asc'
    }
  }

  function reset(): void {
    search.value = ''
    page.value = 1
    sortKey.value = initialSortKey
    sortDir.value = initialSortDir
  }

  // Any change that can shrink the result set returns to page 1; a page left past
  // the end (e.g. after filtering) clamps back into range.
  watch([search, sortKey, sortDir], () => (page.value = 1))
  watch(totalPages, (n) => {
    if (page.value > n) page.value = n
  })

  return { search, page, sortKey, sortDir, filtered, paged, totalPages, setSort, reset }
}
