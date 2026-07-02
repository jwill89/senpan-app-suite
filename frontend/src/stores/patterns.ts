/**
 * Patterns store: win patterns + categories CRUD, the new-pattern editor,
 * collapse state, and reordering. Drag-and-drop reordering is now handled by
 * vuedraggable in the components; this store exposes `persistPatternOrder` and
 * `persistCategoryOrder` (and a cross-category move) that the components call
 * after a drag, plus `setPatternCategory` — replacing the old manual HTML5 DnD.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { emptyGrid } from '@/lib/constants'
import type { Pattern, PatternCategory } from '@/types/api'
import { useUiStore } from './ui'

export interface PatternGroup {
  category: PatternCategory
  patterns: Pattern[]
}

export const usePatternsStore = defineStore('patterns', () => {
  const ui = useUiStore()

  const patterns = ref<Pattern[]>([])
  const categories = ref<PatternCategory[]>([])
  /** True while patterns/categories are loading (drives list spinners). */
  const patternsLoading = ref(false)
  /** True while a new pattern is being saved (drives the Save button). */
  const savingPattern = ref(false)
  /** True while the category form is saving (drives its Save button). */
  const savingCategory = ref(false)

  // New pattern editor
  const newPatternName = ref('')
  const newPatternGrid = ref<boolean[][]>(emptyGrid())
  const newPatternCategoryId = ref<number | null>(null)

  // Collapse state
  const patternsCollapsed = ref(false)
  const collapsedCategories = ref<Record<number, boolean>>({})

  // Category create/edit form (name + insert position; see startNewCategory).
  const categoryForm = ref<{ id: number; name: string; position: string }>({
    id: 0,
    name: '',
    position: 'start',
  })

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
      if (!(p.category_id in map)) {
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

  /**
   * Patterns currently *visible* in the picker: the category filter + search and,
   * when not searching, only patterns in expanded (non-collapsed) categories.
   * Drives "Select all" so it never touches patterns hidden by the filter or by a
   * collapsed category — those keep their current selection.
   */
  const displayedPatterns = computed<Pattern[]>(() => {
    if ((patternSearchQuery.value || '').trim()) return gameFilteredPatterns.value
    const groups = patternCategoryFilter.value
      ? patternsByCategory.value.filter((g) => g.category.id === patternCategoryFilter.value)
      : patternsByCategory.value
    return groups.filter((g) => !isCategoryCollapsed(g.category.id)).flatMap((g) => g.patterns)
  })

  // ── Load ─────────────────────────────────────────────────────────────────

  async function loadPatterns(): Promise<void> {
    patternsLoading.value = true
    try {
      const data = await endpoints.patterns.list()
      patterns.value = data.patterns
      categories.value = data.categories
      rebuildEditableGroups()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      patternsLoading.value = false
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

  /** Saves the new-pattern editor. Returns true on success (caller can navigate). */
  async function savePattern(): Promise<boolean> {
    const name = newPatternName.value.trim()
    if (!name) return false
    const hasCell = newPatternGrid.value.some((r) => r.some((c) => c))
    if (!hasCell) {
      ui.notify('Select at least one cell in the pattern', 'error')
      return false
    }
    // Client-side duplicate check.
    const dup = patterns.value.find((p) => {
      if (p.pattern_data.length !== 5) return false
      for (let r = 0; r < 5; r++) {
        for (let c = 0; c < 5; c++) {
          if (p.pattern_data[r][c] !== newPatternGrid.value[r][c]) return false
        }
      }
      return true
    })
    if (dup) {
      const catName = dup.category_name || 'Unknown'
      ui.notify(`Duplicate pattern! Matches "${dup.name}" in category "${catName}"`, 'error')
      return false
    }
    savingPattern.value = true
    try {
      const catId =
        newPatternCategoryId.value || (categories.value.length ? categories.value[0].id : 1)
      await endpoints.patterns.create(name, newPatternGrid.value, catId)
      ui.notify('Pattern saved', 'success')
      clearPatternEditor()
      await loadPatterns()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingPattern.value = false
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
    return collapsedCategories.value[catId]
  }

  // ── Pattern CRUD ───────────────────────────────────────────────────────────

  async function deletePattern(id: number): Promise<void> {
    try {
      await endpoints.patterns.delete(id)
      patterns.value = patterns.value.filter((p) => p.id !== id)
      ui.notify('Pattern deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function confirmDeletePattern(id: number): Promise<void> {
    if (
      !(await ui.confirm('Delete this pattern?', {
        title: 'Delete pattern',
        confirmText: 'Delete',
      }))
    )
      return
    void deletePattern(id)
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
      await endpoints.patterns.rename(id, newName)
      pat.name = newName
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // ── Category CRUD (table + position form) ──────────────────────────────────

  /** Open the form to add a new category (defaults to inserting at the end). */
  function startNewCategory(): void {
    const last = categories.value[categories.value.length - 1]
    categoryForm.value = {
      id: 0,
      name: '',
      position: categories.value.length ? `after:${last.id}` : 'start',
    }
  }

  /** Open the form to edit a category (position defaults to "keep current"). */
  function startEditCategory(cat: PatternCategory): void {
    categoryForm.value = { id: cat.id, name: cat.name, position: 'keep' }
  }

  /**
   * Reorders the categories so `targetId` lands at the chosen position token —
   * 'start' (first) or 'after:<id>' (right after that category) — and persists
   * the new order via the bulk-reorder endpoint. ('keep' is handled by callers.)
   */
  async function applyCategoryPosition(targetId: number, position: string): Promise<void> {
    const ids = categories.value.map((c) => c.id).filter((id) => id !== targetId)
    let insertAt = ids.length
    if (position === 'start') {
      insertAt = 0
    } else if (position.startsWith('after:')) {
      const afterId = Number(position.slice('after:'.length))
      const idx = ids.indexOf(afterId)
      insertAt = idx === -1 ? ids.length : idx + 1
    }
    ids.splice(insertAt, 0, targetId)
    await endpoints.patternCategories.bulkReorder(ids)
  }

  /**
   * Saves the category form: creates (then positions) a new category, or renames
   * + optionally repositions an existing one. Returns true on success so the
   * caller can return to the categories table.
   */
  async function saveCategoryForm(): Promise<boolean> {
    const name = categoryForm.value.name.trim()
    if (!name) {
      ui.notify('Category name is required', 'error')
      return false
    }
    savingCategory.value = true
    try {
      const { id, position } = categoryForm.value
      if (id === 0) {
        // create() appends; find the new id by diffing, then position it.
        const before = new Set(categories.value.map((c) => c.id))
        await endpoints.patternCategories.create(name)
        await loadPatterns()
        const created = categories.value.find((c) => !before.has(c.id))
        if (created && position !== 'keep') {
          await applyCategoryPosition(created.id, position)
          await loadPatterns()
        }
        ui.notify('Category created', 'success')
      } else {
        const cat = categories.value.find((c) => c.id === id)
        if (cat && cat.name !== name) await endpoints.patternCategories.rename(id, name)
        if (position !== 'keep') await applyCategoryPosition(id, position)
        await loadPatterns()
        ui.notify('Category updated', 'success')
      }
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingCategory.value = false
    }
  }

  async function deleteCategory(id: number): Promise<void> {
    try {
      await endpoints.patternCategories.delete(id)
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
    void deleteCategory(id)
  }

  // ── Pattern reordering (called by vuedraggable handlers) ───────────────────

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
      if (!(p.category_id in map)) {
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
        await endpoints.patterns.bulkReorder(
          group.category.id,
          group.patterns.map((p) => p.id),
        )
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
    patternsLoading,
    savingPattern,
    savingCategory,
    newPatternName,
    newPatternGrid,
    newPatternCategoryId,
    patternsCollapsed,
    collapsedCategories,
    categoryForm,
    editingPatternId,
    editingPatternName,
    patternCategoryFilter,
    patternSearchQuery,
    patternsByCategory,
    gameFilteredPatterns,
    displayedPatterns,
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
    startNewCategory,
    startEditCategory,
    saveCategoryForm,
    deleteCategory,
    confirmDeleteCategory,
    editableGroups,
    rebuildEditableGroups,
    applyGroupedOrder,
  }
})
