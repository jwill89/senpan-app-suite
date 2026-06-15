<script setup lang="ts">
/**
 * Admin Patterns manager — one tab (under Bingo) unifying the former Pattern
 * Categories, New Pattern, and Edit Patterns tabs into the standard manager model:
 *
 *   - list: patterns grouped by category, collapsible, drag to reorder / move
 *     between categories, with a search box + category filter. A search query
 *     flattens the body to the matching patterns (drag is a no-search affordance).
 *   - "+ New Pattern": the 5×5 grid editor on a Back sub-page.
 *   - "Manage Categories": category create / rename / delete / reorder sub-page.
 *
 * All state + actions come from the patterns store (unchanged); this only
 * restructures the three former tabs into one shell.
 */
import { computed, onMounted, ref } from 'vue'
import draggable from 'vuedraggable'
import PatternMini from '@/components/common/PatternMini.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import { usePatternsStore } from '@/stores/patterns'
import type { PatternCategory } from '@/types/api'

const patterns = usePatternsStore()

type Screen = 'list' | 'new' | 'categories' | 'category-form'
const screen = ref<Screen>('list')

const categoryColumns: DataColumn[] = [
  { key: 'name', label: 'Category' },
  { key: 'actions', label: '', align: 'right' },
]

/**
 * Position choices for the category form: "At the beginning", then "After <cat>"
 * for every other category (the last one = at the end). Editing adds a leading
 * "Keep current position".
 */
const categoryPositionOptions = computed(() => {
  const opts: { value: string; label: string }[] = []
  if (patterns.categoryForm.id !== 0) opts.push({ value: 'keep', label: 'Keep current position' })
  opts.push({ value: 'start', label: 'At the beginning' })
  for (const c of patterns.categories) {
    if (c.id === patterns.categoryForm.id) continue
    opts.push({ value: `after:${c.id}`, label: `After ${c.name}` })
  }
  return opts
})

function goNewCategory(): void {
  patterns.startNewCategory()
  screen.value = 'category-form'
}
function goEditCategory(cat: PatternCategory): void {
  patterns.startEditCategory(cat)
  screen.value = 'category-form'
}
async function saveCategory(): Promise<void> {
  if (await patterns.saveCategoryForm()) screen.value = 'categories'
}

onMounted(() => patterns.rebuildEditableGroups())

/** Persist after any drag (reorder within / move between categories). */
function onChange(): void {
  patterns.applyGroupedOrder()
}

/** True when a text search is active — switches the body to a flat result list. */
const searching = computed(() => patterns.patternSearchQuery.trim().length > 0)

/** Groups for the grouped (no-search) view, limited by the category filter. */
const visibleGroups = computed(() =>
  patterns.patternCategoryFilter
    ? patterns.editableGroups.filter((g) => g.category.id === patterns.patternCategoryFilter)
    : patterns.editableGroups,
)

function goNew(): void {
  patterns.clearPatternEditor()
  screen.value = 'new'
}
async function saveNew(): Promise<void> {
  if (await patterns.savePattern()) screen.value = 'list'
}
</script>

<template>
  <div class="tab-body">
    <!-- ── List ──────────────────────────────────────────────────────────── -->
    <ManagerView v-if="screen === 'list'" title="Patterns" icon="fa-duotone fa-grid">
      <template #actions>
        <button class="btn-ghost btn-sm" @click="screen = 'categories'">
          <i class="fa-duotone fa-folder-open"></i> Manage Categories
        </button>
        <button class="btn-primary btn-sm" @click="goNew">
          <i class="fa-solid fa-plus"></i> New Pattern
        </button>
      </template>

      <template #toolbar>
        <SearchInput
          v-model="patterns.patternSearchQuery"
          placeholder="Search patterns…"
          aria-label="Search patterns"
        />
        <select
          v-model="patterns.patternCategoryFilter"
          aria-label="Filter by category"
          class="manager-filter"
        >
          <option :value="null">All categories</option>
          <option v-for="c in patterns.categories" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
        <span class="text-dim text-xs push-right">
          {{ patterns.patterns.length }} pattern{{ patterns.patterns.length === 1 ? '' : 's' }}
        </span>
      </template>

      <LoadingSpinner
        v-if="patterns.patternsLoading && patterns.patterns.length === 0"
        block
        label="Loading patterns…"
      />

      <EmptyState
        v-else-if="patterns.patterns.length === 0"
        text="No patterns saved yet. Use “New Pattern” to add one."
      />

      <!-- Flat search results (no drag while searching) -->
      <template v-else-if="searching">
        <div v-if="patterns.gameFilteredPatterns.length" class="saved-patterns">
          <div v-for="p in patterns.gameFilteredPatterns" :key="p.id" class="saved-pattern">
            <span class="del" title="Delete pattern" @click="patterns.confirmDeletePattern(p.id)"
              >&times;</span
            >
            <PatternMini :pattern-data="p.pattern_data" />
            <input
              v-if="patterns.editingPatternId === p.id"
              v-model="patterns.editingPatternName"
              aria-label="Pattern name"
              class="inline-edit-input pattern-name-input"
              @blur="patterns.finishPatternRename(p.id)"
              @keyup.enter="patterns.finishPatternRename(p.id)"
              @keyup.escape="patterns.editingPatternId = null"
            />
            <span v-else class="pattern-name" @dblclick="patterns.startPatternRename(p)">
              {{ p.name }}
            </span>
            <span class="text-dim text-xs">{{ p.category_name }}</span>
          </div>
        </div>
        <EmptyState v-else :text="`No patterns match “${patterns.patternSearchQuery}”.`" />
      </template>

      <!-- Grouped, draggable view -->
      <template v-else>
        <div class="flex-toolbar mb-12">
          <button class="btn-ghost btn-sm" @click="patterns.togglePatternsCollapsed()">
            {{ patterns.patternsCollapsed ? '▶ Show all' : '▼ Hide all' }}
          </button>
        </div>

        <template v-for="group in visibleGroups" :key="group.category.id">
          <div class="pattern-group-head" @click="patterns.toggleCategoryCollapsed(group.category.id)">
            <span class="text-dim">
              {{ patterns.isCategoryCollapsed(group.category.id) ? '▶' : '▼' }}
            </span>
            <h4>
              {{ group.category.name }}
              <span class="text-dim" style="font-weight: 400">({{ group.patterns.length }})</span>
            </h4>
          </div>

          <draggable
            v-show="!patterns.isCategoryCollapsed(group.category.id)"
            v-model="group.patterns"
            class="saved-patterns pattern-drop-zone"
            :group="{ name: 'patterns' }"
            item-key="id"
            handle=".drag-handle"
            :animation="150"
            ghost-class="dragging"
            @change="onChange"
          >
            <template #item="{ element: p }">
              <div class="saved-pattern">
                <span class="drag-handle pattern-drag"><i class="fa-duotone fa-bars"></i></span>
                <span
                  class="del"
                  title="Delete pattern"
                  @click="patterns.confirmDeletePattern(p.id)"
                  >&times;</span
                >
                <PatternMini :pattern-data="p.pattern_data" />
                <input
                  v-if="patterns.editingPatternId === p.id"
                  v-model="patterns.editingPatternName"
                  aria-label="Pattern name"
                  class="inline-edit-input pattern-name-input"
                  @blur="patterns.finishPatternRename(p.id)"
                  @keyup.enter="patterns.finishPatternRename(p.id)"
                  @keyup.escape="patterns.editingPatternId = null"
                />
                <span v-else class="pattern-name" @dblclick="patterns.startPatternRename(p)">
                  {{ p.name }}
                </span>
              </div>
            </template>
            <template #footer>
              <div v-if="group.patterns.length === 0" class="drop-placeholder">
                Drop patterns here
              </div>
            </template>
          </draggable>
        </template>

        <p class="text-dim text-xs mt-12">
          Double-click a name to rename. Drag to reorder or move between categories.
        </p>
      </template>
    </ManagerView>

    <!-- ── New pattern ───────────────────────────────────────────────────── -->
    <div v-else-if="screen === 'new'" class="admin-panel">
      <SubPageHeader title="New Pattern" icon="fa-duotone fa-plus" @back="screen = 'list'" />
      <div class="pattern-editor">
        <div class="field" style="display: flex; gap: 8px; flex-wrap: wrap">
          <input
            v-model="patterns.newPatternName"
            placeholder="Pattern name (e.g. Diagonal)"
            aria-label="Pattern name"
            style="flex: 1; min-width: 140px"
            @keyup.enter="saveNew"
          />
          <select v-model="patterns.newPatternCategoryId" aria-label="Pattern category">
            <option :value="null" disabled>Category…</option>
            <option v-for="c in patterns.categories" :key="c.id" :value="c.id">{{ c.name }}</option>
          </select>
        </div>
        <p class="text-center text-dim text-sm mb-4">Click cells to toggle</p>
        <div class="pattern-editor-grid">
          <template v-for="(row, ri) in patterns.newPatternGrid" :key="'r' + ri">
            <div
              v-for="(cell, ci) in row"
              :key="ri + '-' + ci"
              :class="['pattern-editor-cell', cell ? 'on' : '']"
              @click="patterns.toggleNewPatternCell(ri, ci)"
            ></div>
          </template>
        </div>
        <div class="btns">
          <button class="btn-ghost btn-sm" @click="patterns.clearPatternEditor()">Clear</button>
          <button
            class="btn-secondary"
            :disabled="!patterns.newPatternName.trim() || patterns.savingPattern"
            @click="saveNew"
          >
            <LoadingSpinner v-if="patterns.savingPattern" label="Saving…" />
            <template v-else>Save Pattern</template>
          </button>
        </div>
      </div>
    </div>

    <!-- ── Manage categories (table) ─────────────────────────────────────── -->
    <div v-else-if="screen === 'categories'" class="admin-panel">
      <SubPageHeader
        title="Manage Categories"
        icon="fa-duotone fa-folder-open"
        @back="screen = 'list'"
      />
      <div class="flex-toolbar flex-end mb-16">
        <button class="btn-primary btn-sm" @click="goNewCategory">
          <i class="fa-solid fa-plus"></i> New Category
        </button>
      </div>
      <DataTable :columns="categoryColumns" :rows="patterns.categories" row-key="id">
        <template #cell-name="{ row }">{{ row.name }}</template>
        <template #cell-actions="{ row }">
          <div class="row-actions">
            <button
              class="btn-secondary btn-sm"
              aria-label="Edit"
              title="Edit"
              @click="goEditCategory(row)"
            >
              <i class="fa-solid fa-pen-to-square"></i>
            </button>
            <button
              class="btn-danger btn-sm"
              aria-label="Delete"
              title="Delete"
              :disabled="patterns.categories.length <= 1"
              @click="patterns.confirmDeleteCategory(row.id)"
            >
              <i class="fa-solid fa-trash"></i>
            </button>
          </div>
        </template>
        <template #empty><EmptyState text="No categories yet." /></template>
      </DataTable>
    </div>

    <!-- ── Category form (add / edit) ────────────────────────────────────── -->
    <div v-else class="admin-panel">
      <SubPageHeader
        :title="patterns.categoryForm.id ? 'Edit Category' : 'New Category'"
        icon="fa-duotone fa-folder-open"
        @back="screen = 'categories'"
      />
      <FormField label="Title" required>
        <input
          v-model="patterns.categoryForm.name"
          placeholder="Category name"
          aria-label="Category name"
          @keyup.enter="saveCategory"
        />
      </FormField>
      <FormField label="Position">
        <select
          v-model="patterns.categoryForm.position"
          aria-label="Category position"
          class="manager-filter"
        >
          <option v-for="o in categoryPositionOptions" :key="o.value" :value="o.value">
            {{ o.label }}
          </option>
        </select>
      </FormField>
      <FormActions align="start">
        <button class="btn-ghost" @click="screen = 'categories'">Cancel</button>
        <button
          class="btn-primary"
          :disabled="!patterns.categoryForm.name.trim() || patterns.savingCategory"
          @click="saveCategory"
        >
          <LoadingSpinner v-if="patterns.savingCategory" label="Saving…" />
          <template v-else>{{ patterns.categoryForm.id ? 'Save Changes' : 'Add Category' }}</template>
        </button>
      </FormActions>
    </div>
  </div>
</template>
