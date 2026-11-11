import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import type { Pattern, PatternCategory } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`: a list for GET and
// the reorders, a category for its create and rename, nothing for a delete.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PATCH: vi.fn(), DELETE: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { usePatternsStore } from '@/stores/patterns'
import { useUiStore } from '@/stores/ui'

function cat(id: number, name: string, sort = id): PatternCategory {
  return { id, name, sortOrder: sort }
}
function pat(id: number, name: string, categoryId: number): Pattern {
  return { id, name, categoryId, categoryName: '', patternData: [], sortOrder: 0 }
}
const empty = { patterns: [], categories: [] }

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  api.GET.mockResolvedValue(ok(empty))
  api.POST.mockResolvedValue(ok(empty))
  api.PATCH.mockResolvedValue(ok(cat(1, 'A')))
  api.DELETE.mockResolvedValue(ok(undefined, 204))
})

describe('loading and saving', () => {
  it('loadPatterns takes in the patterns and categories', async () => {
    api.GET.mockResolvedValueOnce(ok({ patterns: [pat(10, 'Row', 1)], categories: [cat(1, 'A')] }))
    const s = usePatternsStore()
    await s.loadPatterns()
    expect(api.GET).toHaveBeenCalledWith('/patterns')
    expect(s.patterns.map((p) => p.name)).toEqual(['Row'])
    expect(s.categories.map((c) => c.name)).toEqual(['A'])
  })

  it('savePattern POSTs the grid into the chosen category, then reloads', async () => {
    const s = usePatternsStore()
    s.categories = [cat(4, 'Lines')]
    s.newPatternName = ' Corner '
    s.toggleNewPatternCell(0, 0)
    api.POST.mockResolvedValueOnce(ok(pat(11, 'Corner', 4), 201))
    expect(await s.savePattern()).toBe(true)
    const [path, init] = api.POST.mock.calls[0] as [string, { body: Record<string, unknown> }]
    expect(path).toBe('/patterns')
    expect(init.body.name).toBe('Corner')
    expect(init.body.categoryId).toBe(4)
    expect((init.body.patternData as boolean[][])[0]![0]).toBe(true)
    expect(api.GET).toHaveBeenCalledWith('/patterns')
    expect(s.newPatternName).toBe('')
  })

  it('savePattern refuses a grid another pattern has, naming it, without a call', async () => {
    const s = usePatternsStore()
    const grid = [0, 1, 2, 3, 4].map(() => [false, false, false, false, false])
    grid[0]![0] = true
    s.patterns = [{ ...pat(10, 'Corner', 1), patternData: grid, categoryName: 'Lines' }]
    s.newPatternName = 'Again'
    s.toggleNewPatternCell(0, 0)
    const ui = useUiStore()
    ui.notify = vi.fn()
    expect(await s.savePattern()).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
    expect(ui.notify).toHaveBeenCalledWith(
      'Duplicate pattern! Matches "Corner" in category "Lines"',
      'error',
    )
  })

  it('finishPatternRename PATCHes the new name', async () => {
    const s = usePatternsStore()
    s.patterns = [pat(10, 'Row', 1)]
    s.startPatternRename(s.patterns[0]!)
    s.editingPatternName = 'Top Row'
    await s.finishPatternRename(10)
    expect(api.PATCH).toHaveBeenCalledWith('/patterns/{id}', {
      params: { path: { id: 10 } },
      body: { name: 'Top Row' },
    })
    expect(s.patterns[0]!.name).toBe('Top Row')
  })

  it('deletePattern DELETEs it and drops it from the list', async () => {
    const s = usePatternsStore()
    s.patterns = [pat(10, 'Row', 1), pat(11, 'Col', 1)]
    await s.deletePattern(10)
    expect(api.DELETE).toHaveBeenCalledWith('/patterns/{id}', { params: { path: { id: 10 } } })
    expect(s.patterns.map((p) => p.id)).toEqual([11])
  })

  it('deleteCategory DELETEs it and reloads', async () => {
    const s = usePatternsStore()
    await s.deleteCategory(2)
    expect(api.DELETE).toHaveBeenCalledWith('/pattern-categories/{id}', {
      params: { path: { id: 2 } },
    })
    expect(api.GET).toHaveBeenCalledWith('/patterns')
  })
})

describe('patternsByCategory', () => {
  it('groups patterns under their category in category order', () => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'Lines'), cat(2, 'Shapes')]
    s.patterns = [pat(10, 'Row', 1), pat(11, 'Col', 1), pat(12, 'Box', 2)]
    const groups = s.patternsByCategory
    expect(groups.map((g) => g.category.name)).toEqual(['Lines', 'Shapes'])
    expect(groups[0]!.patterns.map((p) => p.id)).toEqual([10, 11])
    expect(groups[1]!.patterns.map((p) => p.id)).toEqual([12])
  })
})

describe('gameFilteredPatterns', () => {
  beforeEach(() => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'Lines'), cat(2, 'Shapes')]
    s.patterns = [pat(10, 'Top Row', 1), pat(11, 'Left Col', 1), pat(12, 'Big Box', 2)]
  })

  it('filters by category', () => {
    const s = usePatternsStore()
    s.patternCategoryFilter = 2
    expect(s.gameFilteredPatterns.map((p) => p.id)).toEqual([12])
  })

  it('filters by search query (case-insensitive substring)', () => {
    const s = usePatternsStore()
    s.patternSearchQuery = 'row'
    expect(s.gameFilteredPatterns.map((p) => p.id)).toEqual([10])
  })

  it('applies category filter and search together', () => {
    const s = usePatternsStore()
    s.patternCategoryFilter = 1
    s.patternSearchQuery = 'col'
    expect(s.gameFilteredPatterns.map((p) => p.id)).toEqual([11])
  })
})

describe('displayedPatterns (Select-all scope)', () => {
  it('excludes patterns in collapsed categories when not searching', () => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'Lines'), cat(2, 'Shapes')]
    s.patterns = [pat(10, 'Row', 1), pat(12, 'Box', 2)]
    s.toggleCategoryCollapsed(2) // collapse Shapes
    expect(s.displayedPatterns.map((p) => p.id)).toEqual([10])
  })

  it('ignores collapse state while a search is active (uses the filtered list)', () => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'Lines'), cat(2, 'Shapes')]
    s.patterns = [pat(10, 'Row', 1), pat(12, 'Box', 2)]
    s.toggleCategoryCollapsed(2)
    s.patternSearchQuery = 'box'
    expect(s.displayedPatterns.map((p) => p.id)).toEqual([12])
  })
})

// applyCategoryPosition is internal; exercise its index math through the public
// saveCategoryForm (edit path, name unchanged so only the reposition fires).
describe('category positioning (index math + persistence)', () => {
  beforeEach(() => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'A'), cat(2, 'B'), cat(3, 'C')]
  })

  const reorderedTo = (orderedIds: number[]) =>
    expect(api.POST).toHaveBeenCalledWith('/pattern-categories/reorder', { body: { orderedIds } })

  it('moves a category to the start', async () => {
    const s = usePatternsStore()
    s.categoryForm = { id: 3, name: 'C', position: 'start' }
    await s.saveCategoryForm()
    reorderedTo([3, 1, 2])
  })

  it('moves a category to right after another', async () => {
    const s = usePatternsStore()
    s.categoryForm = { id: 3, name: 'C', position: 'after:1' } // C after A -> [A, C, B]
    await s.saveCategoryForm()
    reorderedTo([1, 3, 2])
  })

  it('falls back to the end when the after-target is unknown', async () => {
    const s = usePatternsStore()
    s.categoryForm = { id: 3, name: 'C', position: 'after:999' }
    await s.saveCategoryForm()
    reorderedTo([1, 2, 3])
  })

  it('takes in the list the reorder answers', async () => {
    const s = usePatternsStore()
    api.POST.mockResolvedValueOnce(ok({ patterns: [], categories: [cat(3, 'C'), cat(1, 'A')] }))
    s.categoryForm = { id: 3, name: 'C', position: 'start' }
    await s.saveCategoryForm()
    expect(s.categories.map((c) => c.id)).toEqual([3, 1])
  })

  it('renames an edited category before positioning it', async () => {
    const s = usePatternsStore()
    s.categoryForm = { id: 2, name: 'Bee', position: 'keep' }
    await s.saveCategoryForm()
    expect(api.PATCH).toHaveBeenCalledWith('/pattern-categories/{id}', {
      params: { path: { id: 2 } },
      body: { name: 'Bee' },
    })
    expect(api.POST).not.toHaveBeenCalled()
    expect(api.GET).toHaveBeenCalledWith('/patterns')
  })

  it('creates a category, then puts it where the form says', async () => {
    const s = usePatternsStore()
    api.POST.mockResolvedValueOnce(ok(cat(9, 'New', 4), 201))
    s.categoryForm = { id: 0, name: 'New', position: 'after:1' }
    expect(await s.saveCategoryForm()).toBe(true)
    expect(api.POST).toHaveBeenNthCalledWith(1, '/pattern-categories', { body: { name: 'New' } })
    reorderedTo([1, 9, 2, 3])
  })
})

describe('collapse toggles', () => {
  it('togglePatternsCollapsed sets every category to the new state', () => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'A'), cat(2, 'B')]
    s.togglePatternsCollapsed()
    expect(s.isCategoryCollapsed(1)).toBe(true)
    expect(s.isCategoryCollapsed(2)).toBe(true)
    s.togglePatternsCollapsed()
    expect(s.isCategoryCollapsed(1)).toBe(false)
  })
})

describe('editableGroups stays in step with server state', () => {
  /**
   * The drag view holds shallow COPIES of the patterns so vue-draggable-plus can
   * reorder them, which means it drifts the moment `patterns` changes underneath
   * it. It must be rebuilt wherever `patterns` changes - deleting a pattern, renaming
   * one, or reading the list again after a live change - or the admin edits a list
   * that does not match what a save would write.
   */
  it('follows a replaced patterns array (delete, or the list read again)', async () => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'Lines')]
    s.patterns = [pat(10, 'Row', 1), pat(11, 'Col', 1)]
    await nextTick()
    expect(s.editableGroups[0]!.patterns.map((p) => p.id)).toEqual([10, 11])

    // The shape deletePattern uses.
    s.patterns = s.patterns.filter((p) => p.id !== 10)
    await nextTick()
    expect(s.editableGroups[0]!.patterns.map((p) => p.id)).toEqual([11])
  })

  it('follows an in-place rename', async () => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'Lines')]
    s.patterns = [pat(10, 'Row', 1)]
    await nextTick()
    expect(s.editableGroups[0]!.patterns[0]!.name).toBe('Row')

    // The shape finishPatternRename uses: mutate the object, not the array.
    s.patterns[0]!.name = 'Top Row'
    await nextTick()
    expect(s.editableGroups[0]!.patterns[0]!.name).toBe('Top Row')
  })

  it('follows a new category appearing', async () => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'Lines')]
    s.patterns = [pat(10, 'Row', 1)]
    await nextTick()
    expect(s.editableGroups).toHaveLength(1)

    s.categories = [cat(1, 'Lines'), cat(2, 'Shapes')]
    s.patterns = [pat(10, 'Row', 1), pat(20, 'Box', 2)]
    await nextTick()
    expect(s.editableGroups.map((g) => g.category.id)).toEqual([1, 2])
  })
})

describe('applyGroupedOrder', () => {
  it('flattens editable groups into patterns (reassigning category) and persists each group', async () => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'Lines'), cat(2, 'Shapes')]
    s.editableGroups = [
      { category: cat(1, 'Lines'), patterns: [pat(11, 'Col', 1), pat(10, 'Row', 1)] },
      { category: cat(2, 'Shapes'), patterns: [pat(12, 'Box', 1)] }, // moved from cat 1 -> 2
    ]
    await s.applyGroupedOrder()
    // Flattened order + reassigned category for the moved pattern.
    expect(s.patterns.map((p) => [p.id, p.categoryId])).toEqual([
      [11, 1],
      [10, 1],
      [12, 2],
    ])
    expect(api.POST).toHaveBeenCalledWith('/patterns/reorder', {
      body: { categoryId: 1, orderedIds: [11, 10] },
    })
    expect(api.POST).toHaveBeenCalledWith('/patterns/reorder', {
      body: { categoryId: 2, orderedIds: [12] },
    })
  })

  it('reloads from the server when a reorder is refused', async () => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'Lines')]
    s.editableGroups = [{ category: cat(1, 'Lines'), patterns: [pat(10, 'Row', 1)] }]
    api.POST.mockResolvedValueOnce({
      error: { status: 422, detail: 'There is no such pattern.' },
      response: new Response(null, { status: 422 }),
    })
    const ui = useUiStore()
    ui.notify = vi.fn()
    await s.applyGroupedOrder()
    expect(ui.notify).toHaveBeenCalledWith('There is no such pattern.', 'error')
    expect(api.GET).toHaveBeenCalledWith('/patterns')
  })
})
