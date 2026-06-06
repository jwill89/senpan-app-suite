<script setup lang="ts">
/**
 * Admin Edit Patterns tab — patterns grouped by category, each group collapsible.
 * Reordering within a category and moving patterns between categories now use
 * vuedraggable (SortableJS) with a shared drag group, replacing the original
 * hand-rolled HTML5 drag-and-drop. The `.saved-patterns` / `.saved-pattern`
 * markup and the FontAwesome drag handle are preserved so the look is identical.
 *
 * vuedraggable mutates the bound arrays in `patterns.editableGroups`; after any
 * drag we flatten + persist via `applyGroupedOrder`.
 */
import { onMounted } from 'vue'
import draggable from 'vuedraggable'
import PatternMini from '@/components/common/PatternMini.vue'
import { usePatternsStore } from '@/stores/patterns'

const patterns = usePatternsStore()

// Build the editable grouping from whatever patterns are already loaded.
onMounted(() => patterns.rebuildEditableGroups())

/** Persist after any add/remove/move/reorder across the grouped lists. */
function onChange(): void {
  patterns.applyGroupedOrder()
}
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel">
      <h3 class="mb-12"><i class="fa-solid fa-pen-to-square"></i> Edit Patterns</h3>
      <div v-if="patterns.patterns.length" style="margin-bottom: 12px">
        <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 12px">
          <span style="font-size: 0.85rem; color: var(--text-dim)">
            ({{ patterns.patterns.length }} total)
          </span>
          <button
            class="btn-ghost btn-sm"
            style="margin-left: auto"
            @click="patterns.togglePatternsCollapsed()"
          >
            {{ patterns.patternsCollapsed ? '▶ Show All' : '▼ Hide All' }}
          </button>
        </div>
      </div>

      <template v-for="group in patterns.editableGroups" :key="group.category.id">
        <div
          style="
            display: flex;
            align-items: center;
            gap: 8px;
            margin-bottom: 8px;
            margin-top: 14px;
            cursor: pointer;
            user-select: none;
          "
          @click="patterns.toggleCategoryCollapsed(group.category.id)"
        >
          <span style="font-size: 0.85rem; color: var(--text-dim)">
            {{ patterns.isCategoryCollapsed(group.category.id) ? '▶' : '▼' }}
          </span>
          <h3 style="margin: 0">
            {{ group.category.name }}
            <span style="font-size: 0.8rem; color: var(--text-dim); font-weight: normal">
              ({{ group.patterns.length }})
            </span>
          </h3>
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
              <span class="drag-handle pattern-drag"><i class="fa-solid fa-bars"></i></span>
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
            </div>
          </template>
          <template #footer>
            <div v-if="group.patterns.length === 0" class="drop-placeholder">Drop patterns here</div>
          </template>
        </draggable>
      </template>

      <p v-if="patterns.patterns.length === 0" class="msg-block">No patterns saved yet.</p>
      <p class="text-dim text-xs mt-12">
        Double-click a name to rename. Drag to reorder or move between categories.
      </p>
    </div>
  </div>
</template>
