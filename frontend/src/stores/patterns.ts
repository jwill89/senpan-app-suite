/**
 * Patterns store: win patterns + categories CRUD, the new-pattern editor,
 * collapse state, and reordering. Drag-and-drop reordering is now handled by
 * vuedraggable in the components; this store exposes `persistPatternOrder` and
 * `persistCategoryOrder` (and a cross-category move) that the components call
 * after a drag, plus `setPatternCategory` — replacing the old manual HTML5 DnD.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api } from '@/lib/api'
import { emptyGrid } from '@/lib/constants'
import type { Pattern, PatternCategory, PatternsResponse } from '@/types/api'
import { useUiStore } from './ui'

export interface PatternGroup {
  category: PatternCategory
  patterns: Pattern[]
}

export const usePatternsStore = defineStore('patterns', () => {
  const ui = useUiStore()

  const patterns = ref<Pattern[]>([])
  const categories = ref<PatternCategory[]>([])

  // New pattern editor
  const newPatternName = ref('')
  const newPatternGrid = ref<boolean[][]>(emptyGrid())
  const newPatternCategoryId = ref<number | null>(null)

  // Collapse state
  const patternsCollapsed = ref(false)
  const collapsedCategories = ref<Record<number, boolean>>({})

  // Category create / inline edit
  const newCategoryName = ref('')
  const editingCategoryId = ref<number | null>(null)
  const editingCategoryName = ref('')

  // Pattern inline edit
  const editingPatternId = ref<number | null>(null)
  const editingPatternName = ref('')

  // Game-start filtering
  const patternCategoryFilter = ref<number | null>(null)
  const patternSearchQuery = ref('')

  // ── Computed ───────────────────────────────────────────────────────────────

  /** Patterns grouped by category, in category sort order. */
  const patternsByCategory = computed<PatternGroup[]>(() => {
    const map: Record<number, PatternGroup> = {}
    for (const cat of categories.value) {
      map[cat.id] = { category: cat, patterns: [] }
    }
    for (const p of patterns.value) {
      if (!map[p.category_id]) {
        map[p.category_id] = {
          category: { id: p.category_id, name: p.category_name || 'Unknown', sort_order: 0 },
          patterns: [],
        }
      }
      map[p.category_id].patterns.push(p)
    }
    return categories.value.map((c) => map[c.id]).filter(Boolean)
  })

  /** Patterns filtered by category + search query (game-start picker). */
  const gameFilteredPatterns = computed(() => {
    let list = patterns.value
    if (patternCategoryFilter.value) {
      list = list.filter((p) => p.category_id === patternCategoryFilter.value)
    }
    const q = (patternSearchQuery.value || '').trim().toLowerCase()
    if (q) list = list.filter((p) => p.name.toLowerCase().includes(q))
    return list
  })

  // ── Load ─────────────────────────────────────────────────────────────────

  async function loadPatterns(): Promise<void> {
    try {
      const data = await api<PatternsResponse>('patterns')
      patterns.value = data.patterns
      categories.value = data.categories || []
      rebuildEditableGroups()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // ── New pattern editor ─────────────────────────────────────────────────────

  function clearPatternEditor(): void {
    newPatternName.value = ''
    newPatternGrid.value = emptyGrid()
  }

  function toggleNewPatternCell(ri: number, ci: number): void {
    newPatternGrid.value[ri][ci] = !newPatternGrid.value[ri][ci]
  }

  async function savePattern(): Promise<void> {
    const name = newPatternName.value.trim()
    if (!name) return
    const hasCell = newPatternGrid.value.some((r) => r.some((c) => c))
    if (!hasCell) {
      ui.notify('Select at least one cell in the pattern', 'error')
      return
    }
    // Client-side duplicate check.
    const dup = patterns.value.find((p) => {
      if (!p.pattern_data || p.pattern_data.length !== 5) return false
      for (let r = 0; r < 5; r++) {
        for (let c = 0; c < 5; c++) {
          if (!!p.pattern_data[r][c] !== !!newPatternGrid.value[r][c]) return false
        }
      }
      return true
    })
    if (dup) {
      const catName = dup.category_name || 'Unknown'
      ui.notify(`Duplicate pattern! Matches "${dup.name}" in category "${catName}"`, 'error')
      return
    }
    try {
      const catId =
        newPatternCategoryId.value || (categories.value.length ? categories.value[0].id : 1)
      await api('patterns', {
        method: 'POST',
        body: { action: 'create', name, pattern_data: newPatternGrid.value, category_id: catId },
      })
      ui.notify('Pattern saved', 'success')
      clearPatternEditor()
      await loadPatterns()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // ── Collapse ───────────────────────────────────────────────────────────────

  function togglePatternsCollapsed(): void {
    patternsCollapsed.value = !patternsCollapsed.value
    const newState: Record<number, boolean> = {}
    for (const cat of categories.value) newState[cat.id] = patternsCollapsed.value
    collapsedCategories.value = newState
  }

  function toggleCategoryCollapsed(catId: number): void {
    collapsedCategories.value = {
      ...collapsedCategories.value,
      [catId]: !collapsedCategories.value[catId],
    }
  }

  function isCategoryCollapsed(catId: number): boolean {
    return !!collapsedCategories.value[catId]
  }

  // ── Pattern CRUD ───────────────────────────────────────────────────────────

  async function deletePattern(id: number): Promise<void> {
    try {
      await api('patterns', { method: 'POST', body: { action: 'delete', id } })
      patterns.value = patterns.value.filter((p) => p.id !== id)
      ui.notify('Pattern deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function confirmDeletePattern(id: number): Promise<void> {
    if (!(await ui.confirm('Delete this pattern?', { title: 'Delete pattern', confirmText: 'Delete' })))
      return
    deletePattern(id)
  }

  function startPatternRename(pat: Pattern): void {
    editingPatternId.value = pat.id
    editingPatternName.value = pat.name
  }

  async function finishPatternRename(id: number): Promise<void> {
    const newName = (editingPatternName.value || '').trim()
    editingPatternId.value = null
    if (!newName) return
    const pat = patterns.value.find((p) => p.id === id)
    if (!pat || pat.name === newName) return
    try {
      await api('patterns', { method: 'POST', body: { action: 'rename', id, name: newName } })
      pat.name = newName
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // ── Category CRUD ──────────────────────────────────────────────────────────

  async function createCategory(): Promise<void> {
    const name = (newCategoryName.value || '').trim()
    if (!name) return
    try {
      await api('pattern-categories', { method: 'POST', body: { action: 'create', name } })
      newCategoryName.value = ''
      ui.notify('Category created', 'success')
      await loadPatterns()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  function startCategoryRename(cat: PatternCategory): void {
    editingCategoryId.value = cat.id
    editingCategoryName.value = cat.name
  }

  async function finishCategoryRename(id: number): Promise<void> {
    const newName = (editingCategoryName.value || '').trim()
    editingCategoryId.value = null
    if (!newName) return
    const cat = categories.value.find((c) => c.id === id)
    if (!cat || cat.name === newName) return
    try {
      await api('pattern-categories', { method: 'POST', body: { action: 'rename', id, name: newName } })
      cat.name = newName
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function deleteCategory(id: number): Promise<void> {
    try {
      await api('pattern-categories', { method: 'POST', body: { action: 'delete', id } })
      ui.notify('Category deleted', 'info')
      await loadPatterns()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function confirmDeleteCategory(id: number): Promise<void> {
    if (
      !(await ui.confirm(
        'Delete this category? Its patterns will be moved to the first remaining category.',
        { title: 'Delete category', confirmText: 'Delete' },
      ))
    )
      return
    deleteCategory(id)
  }

  // ── Reordering (called by vuedraggable handlers) ───────────────────────────

  /** Persists the current category order to the server. Reverts on failure. */
  async function persistCategoryOrder(): Promise<void> {
    try {
      await api('pattern-categories', {
        method: 'POST',
        body: { action: 'bulk_reorder', ordered_ids: categories.value.map((c) => c.id) },
      })
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      await loadPatterns()
    }
  }


  /**
   * Editable grouping used by the Edit Patterns drag-and-drop view. Unlike
   * `patternsByCategory`, the inner `patterns` arrays here are mutable copies
   * that vuedraggable can reorder/move directly. Call `applyGroupedOrder` after
   * a drag to push the new ordering + category assignments back into `patterns`
   * and persist them. Rebuild it from server state with `rebuildEditableGroups`.
   */
  const editableGroups = ref<PatternGroup[]>([])

  /** Rebuilds `editableGroups` from the current `patterns` + `categories`. */
  function rebuildEditableGroups(): void {
    const map: Record<number, PatternGroup> = {}
    for (const cat of categories.value) map[cat.id] = { category: cat, patterns: [] }
    for (const p of patterns.value) {
      if (!map[p.category_id]) {
        map[p.category_id] = {
          category: { id: p.category_id, name: p.category_name || 'Unknown', sort_order: 0 },
          patterns: [],
        }
      }
      map[p.category_id].patterns.push({ ...p })
    }
    editableGroups.value = categories.value.map((c) => map[c.id]).filter(Boolean)
  }

  /**
   * After a vuedraggable change in the Edit Patterns view, flattens
   * `editableGroups` back into `patterns` (updating each pattern's category +
   * order) and persists the order of every category. A single call covers both
   * in-category reordering and cross-category moves.
   */
  async function applyGroupedOrder(): Promise<void> {
    // Flatten the editable groups back into the canonical patterns list,
    // updating category assignment + denormalized category name.
    const flat: Pattern[] = []
    for (const group of editableGroups.value) {
      for (const p of group.patterns) {
        flat.push({
          ...p,
          category_id: group.category.id,
          category_name: group.category.name,
        })
      }
    }
    patterns.value = flat

    try {
      for (const group of editableGroups.value) {
        await api('patterns', {
          method: 'POST',
          body: {
            action: 'bulk_reorder',
            category_id: group.category.id,
            ordered_ids: group.patterns.map((p) => p.id),
          },
        })
      }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      await loadPatterns()
      rebuildEditableGroups()
    }
  }

  return {
    patterns,
    categories,
    newPatternName,
    newPatternGrid,
    newPatternCategoryId,
    patternsCollapsed,
    collapsedCategories,
    newCategoryName,
    editingCategoryId,
    editingCategoryName,
    editingPatternId,
    editingPatternName,
    patternCategoryFilter,
    patternSearchQuery,
    patternsByCategory,
    gameFilteredPatterns,
    loadPatterns,
    clearPatternEditor,
    toggleNewPatternCell,
    savePattern,
    togglePatternsCollapsed,
    toggleCategoryCollapsed,
    isCategoryCollapsed,
    deletePattern,
    confirmDeletePattern,
    startPatternRename,
    finishPatternRename,
    createCategory,
    startCategoryRename,
    finishCategoryRename,
    deleteCategory,
    confirmDeleteCategory,
    persistCategoryOrder,
    editableGroups,
    rebuildEditableGroups,
    applyGroupedOrder,
  }
})
