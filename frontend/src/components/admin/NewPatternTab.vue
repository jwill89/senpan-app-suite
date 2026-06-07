<script setup lang="ts">
/**
 * Admin New Pattern tab — the 5×5 grid editor for creating a win pattern, with
 * a name field, category selector, click-to-toggle cells, and duplicate
 * detection (in the store). Mirrors the original `adminTab==='bingo-new-pattern'`
 * block.
 */
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { usePatternsStore } from '@/stores/patterns'

const patterns = usePatternsStore()
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel">
      <h3 class="mb-12"><i class="fa-solid fa-plus"></i> New Pattern</h3>
      <div class="pattern-editor">
        <div class="field" style="display: flex; gap: 8px; flex-wrap: wrap">
          <input
            v-model="patterns.newPatternName"
            placeholder="Pattern name (e.g. Diagonal)"
            aria-label="Pattern name"
            style="flex: 1; min-width: 140px"
          />
          <select
            v-model="patterns.newPatternCategoryId"
            aria-label="Pattern category"
            style="
              padding: 6px 10px;
              border-radius: 6px;
              background: var(--surface);
              color: var(--text);
              border: 1px solid var(--surface2);
            "
          >
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
            @click="patterns.savePattern()"
          >
            <LoadingSpinner v-if="patterns.savingPattern" label="Saving…" />
            <template v-else>Save Pattern</template>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
