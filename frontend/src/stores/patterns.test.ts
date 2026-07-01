import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Pattern, PatternCategory } from '@/types/api'

// Capture the bulk-reorder calls so the category position math can be asserted.
// listPatterns backs the loadPatterns() refresh that saveCategoryForm runs.
// (Cross-category moves + drag-order persistence go through the bulk `/reorder`
// endpoints — patterns.bulkReorder / patternCategories.bulkReorder.)
const { reorderCats, reorderPats, listPatterns } = vi.hoisted(() => ({
  reorderCats: vi.fn(async () => ({})),
  reorderPats: vi.fn(async () => ({})),
  listPatterns: vi.fn(async () => ({ patterns: [], categories: [] })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    patterns: { bulkReorder: reorderPats, list: listPatterns },
    patternCategories: {
      bulkReorder: reorderCats,
      rename: vi.fn(async () => ({})),
      create: vi.fn(async () => ({})),
    },
  },
}))

import { usePatternsStore } from './patterns'

function cat(id: number, name: string, sort = id): PatternCategory {
  return { id, name, sort_order: sort } as PatternCategory
}
function pat(id: number, name: string, categoryId: number): Pattern {
  return { id, name, category_id: categoryId, category_name: '', pattern_data: [] } as unknown as Pattern
}

beforeEach(() => {
  setActivePinia(createPinia())
  reorderCats.mockClear()
  reorderPats.mockClear()
})

describe('patternsByCategory', () => {
  it('groups patterns under their category in category order', () => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'Lines'), cat(2, 'Shapes')]
    s.patterns = [pat(10, 'Row', 1), pat(11, 'Col', 1), pat(12, 'Box', 2)]
    const groups = s.patternsByCategory
    expect(groups.map((g) => g.category.name)).toEqual(['Lines', 'Shapes'])
    expect(groups[0].patterns.map((p) => p.id)).toEqual([10, 11])
    expect(groups[1].patterns.map((p) => p.id)).toEqual([12])
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

  it('moves a category to the start', async () => {
    const s = usePatternsStore()
    s.categoryForm = { id: 3, name: 'C', position: 'start' }
    await s.saveCategoryForm()
    expect(reorderCats).toHaveBeenCalledWith([3, 1, 2])
  })

  it('moves a category to right after another', async () => {
    const s = usePatternsStore()
    s.categoryForm = { id: 3, name: 'C', position: 'after:1' } // C after A → [A, C, B]
    await s.saveCategoryForm()
    expect(reorderCats).toHaveBeenCalledWith([1, 3, 2])
  })

  it('falls back to the end when the after-target is unknown', async () => {
    const s = usePatternsStore()
    s.categoryForm = { id: 3, name: 'C', position: 'after:999' }
    await s.saveCategoryForm()
    expect(reorderCats).toHaveBeenCalledWith([1, 2, 3])
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

describe('applyGroupedOrder', () => {
  it('flattens editable groups into patterns (reassigning category) and persists each group', async () => {
    const s = usePatternsStore()
    s.categories = [cat(1, 'Lines'), cat(2, 'Shapes')]
    s.editableGroups = [
      { category: cat(1, 'Lines'), patterns: [pat(11, 'Col', 1), pat(10, 'Row', 1)] },
      { category: cat(2, 'Shapes'), patterns: [pat(12, 'Box', 1)] }, // moved from cat 1 → 2
    ]
    await s.applyGroupedOrder()
    // Flattened order + reassigned category for the moved pattern.
    expect(s.patterns.map((p) => [p.id, p.category_id])).toEqual([
      [11, 1],
      [10, 1],
      [12, 2],
    ])
    expect(reorderPats).toHaveBeenCalledWith(1, [11, 10])
    expect(reorderPats).toHaveBeenCalledWith(2, [12])
  })
})
