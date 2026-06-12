<script setup lang="ts">
/**
 * Admin Pattern Categories tab — create, inline-rename (double-click), delete,
 * and reorder categories. Reordering now uses vuedraggable (SortableJS) instead
 * of the original hand-rolled HTML5 drag-and-drop; the visual `.category-grid` /
 * `.category-chip` markup and the `☰` drag handle are preserved so the look is
 * identical. After a drag the new order is persisted via the store.
 */
import draggable from 'vuedraggable'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { usePatternsStore } from '@/stores/patterns'

const patterns = usePatternsStore()

/** Persist the new category order after a drag completes. */
function onReorder(): void {
  patterns.persistCategoryOrder()
}
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel" style="padding: 24px">
      <h3 class="mb-12"><i class="fa-duotone fa-folder-open"></i> Pattern Categories</h3>
      <div class="flex-toolbar" style="margin-bottom: 14px">
        <input
          v-model="patterns.newCategoryName"
          placeholder="New category name"
          aria-label="New category name"
          style="flex: 1; min-width: 140px; max-width: 240px"
          @keyup.enter="patterns.createCategory()"
        />
        <button
          class="btn-secondary btn-sm"
          :disabled="!patterns.newCategoryName.trim() || patterns.creatingCategory"
          @click="patterns.createCategory()"
        >
          <LoadingSpinner v-if="patterns.creatingCategory" label="Adding…" />
          <template v-else>+ Add Category</template>
        </button>
      </div>

      <draggable
        v-model="patterns.categories"
        class="category-grid"
        item-key="id"
        handle=".drag-handle"
        :animation="150"
        ghost-class="dragging"
        @end="onReorder"
      >
        <template #item="{ element: cat }">
          <div class="category-chip">
            <span class="drag-handle">☰</span>
            <input
              v-if="patterns.editingCategoryId === cat.id"
              v-model="patterns.editingCategoryName"
              aria-label="Category name"
              class="inline-edit-input"
              @blur="patterns.finishCategoryRename(cat.id)"
              @keyup.enter="patterns.finishCategoryRename(cat.id)"
              @keyup.escape="patterns.editingCategoryId = null"
            />
            <span v-else class="category-name" @dblclick="patterns.startCategoryRename(cat)">
              {{ cat.name }}
            </span>
            <span
              v-if="patterns.categories.length > 1"
              class="category-del"
              title="Delete category"
              @click="patterns.confirmDeleteCategory(cat.id)"
              >&times;</span
            >
          </div>
        </template>
      </draggable>

      <p class="text-dim text-xs" style="margin-top: 10px">
        Double-click a name to rename. Drag to reorder.
      </p>
    </div>
  </div>
</template>
