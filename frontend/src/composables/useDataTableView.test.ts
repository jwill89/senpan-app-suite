import { describe, it, expect } from 'vitest'
import { nextTick, ref } from 'vue'
import { useDataTableView } from './useDataTableView'

interface Row {
  name: string
  score: number
}

const rows: Row[] = [
  { name: 'Carol', score: 2 },
  { name: 'alice', score: 10 },
  { name: 'Bob', score: 9 },
]

const matches = (r: Row, q: string) => r.name.toLowerCase().includes(q)

describe('useDataTableView', () => {
  it('filters by the predicate (case-insensitively via the lowercased query)', () => {
    const v = useDataTableView(ref(rows), { matches })
    v.search.value = 'a' // Carol, alice (both contain "a")
    expect(v.filtered.value.map((r) => r.name)).toEqual(['Carol', 'alice'])
  })

  it('sorts on the active column, numeric-aware, and toggles direction', () => {
    const v = useDataTableView(ref(rows), { matches, sort: { key: 'score' } })
    // 2 < 9 < 10 (numeric compare, not lexicographic where "10" < "2").
    expect(v.filtered.value.map((r) => r.score)).toEqual([2, 9, 10])
    v.setSort('score') // same key → flip to desc
    expect(v.sortDir.value).toBe('desc')
    expect(v.filtered.value.map((r) => r.score)).toEqual([10, 9, 2])
  })

  it('does not mutate the source array while sorting', () => {
    const source = ref([...rows])
    const v = useDataTableView(source, { matches, sort: { key: 'name' } })
    void v.filtered.value
    expect(source.value).toEqual(rows) // original order preserved
  })

  it('paginates and exposes the page count', () => {
    const v = useDataTableView(ref(rows), { matches, perPage: 2 })
    expect(v.totalPages.value).toBe(2)
    expect(v.paged.value).toHaveLength(2)
    v.page.value = 2
    expect(v.paged.value).toHaveLength(1)
  })

  it('returns to page 1 when the search changes', async () => {
    const v = useDataTableView(ref(rows), { matches, perPage: 2 })
    v.page.value = 2
    v.search.value = 'o' // Carol, Bob
    await nextTick()
    expect(v.page.value).toBe(1)
  })

  it('clamps the page back into range when results shrink', async () => {
    const v = useDataTableView(ref(rows), { matches, perPage: 1 })
    v.page.value = 3
    v.search.value = 'alice' // one match → one page
    await nextTick()
    expect(v.page.value).toBe(1)
  })

  it('reset() clears search, sort, and page to their initial state', async () => {
    const v = useDataTableView(ref(rows), { matches, sort: { key: 'score', dir: 'desc' } })
    v.search.value = 'bob'
    v.page.value = 2
    v.setSort('score') // desc → asc
    await nextTick()
    v.reset()
    expect(v.search.value).toBe('')
    expect(v.page.value).toBe(1)
    expect(v.sortKey.value).toBe('score')
    expect(v.sortDir.value).toBe('desc')
  })
})
