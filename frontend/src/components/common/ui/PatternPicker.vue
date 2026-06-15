<script setup lang="ts">
/**
 * Shared win-pattern picker — one toolbar row (search + category filter + Select
 * All + Hide/Show All) over a grouped, collapsible checkbox grid that flattens
 * while searching. Used by the New Game setup (GameTab) and the Game Preset
 * editor (PresetsTab), so the two stay identical by construction.
 *
 * `v-model` is the selected pattern-id array. "Select All" acts only on the
 * patterns actually *displayed* — it respects the category filter, the search,
 * and collapsed categories — so patterns hidden by any of those keep their
 * current selection (`patterns.displayedPatterns`).
 *
 * Search/filter/collapse state lives in the patterns store (shared with the
 * Patterns manager), so the picker is purely presentational over it.
 */
import { computed } from 'vue'
import PatternMini from '@/components/common/PatternMini.vue'
import SearchInput from './SearchInput.vue'
import EmptyState from './EmptyState.vue'
import { usePatternsStore } from '@/stores/patterns'

const model = defineModel<number[]>({ required: true })
const patterns = usePatternsStore()

/** A text search flattens the grid to matching patterns (no category groups). */
const searching = computed(() => patterns.patternSearchQuery.trim().length > 0)

/** Non-empty category groups for the grouped view, limited by the category filter. */
const visibleGroups = computed(() =>
  (patterns.patternCategoryFilter
    ? patterns.patternsByCategory.filter((g) => g.category.id === patterns.patternCategoryFilter)
    : patterns.patternsByCategory
  ).filter((g) => g.patterns.length > 0),
)

/** True when every currently-displayed pattern is already selected. */
const allDisplayedSelected = computed(
  () =>
    patterns.displayedPatterns.length > 0 &&
    patterns.displayedPatterns.every((p) => model.value.includes(p.id)),
)

/** Select / deselect exactly the displayed patterns (others keep their status). */
function toggleSelectAll(): void {
  const ids = patterns.displayedPatterns.map((p) => p.id)
  if (allDisplayedSelected.value) {
    const remove = new Set(ids)
    model.value = model.value.filter((id) => !remove.has(id))
  } else {
    model.value = [...new Set([...model.value, ...ids])]
  }
}
</script>

<template>
  <div class="pattern-picker">
    <div class="flex-toolbar mb-12">
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
        <option :value="null">All Categories</option>
        <option v-for="c in patterns.categories" :key="c.id" :value="c.id">{{ c.name }}</option>
      </select>
      <button
        class="btn-neutral btn-sm"
        :disabled="patterns.displayedPatterns.length === 0"
        :title="
          allDisplayedSelected
            ? 'Deselect the patterns shown below (others stay selected)'
            : 'Select the patterns shown below (others keep their status)'
        "
        @click="toggleSelectAll"
      >
        <font-awesome-icon :icon="['fas', 'circle-check']" />
        {{ allDisplayedSelected ? 'Deselect All' : 'Select All' }}
      </button>
      <button
        v-if="!searching"
        class="btn-neutral btn-sm"
        @click="patterns.togglePatternsCollapsed()"
      >
        {{ patterns.patternsCollapsed ? '▶ Show all' : '▼ Hide all' }}
      </button>
    </div>

    <!-- Flat search results -->
    <div v-if="searching" class="pattern-checks">
      <label
        v-for="p in patterns.gameFilteredPatterns"
        :key="p.id"
        :class="['pattern-check', model.includes(p.id) ? 'selected' : '']"
      >
        <input type="checkbox" :value="p.id" v-model="model" />
        <span class="dot"></span>
        <PatternMini :pattern-data="p.pattern_data" size="pattern-mini-sm" />
        <span class="pattern-check-name">{{ p.name }}</span>
        <span class="pattern-check-cat">{{ p.category_name }}</span>
      </label>
    </div>

    <!-- Grouped, collapsible -->
    <template v-else>
      <EmptyState v-if="visibleGroups.length === 0" text="No patterns match the current filter." />
      <template v-for="group in visibleGroups" :key="group.category.id">
        <div class="pattern-group-head" @click="patterns.toggleCategoryCollapsed(group.category.id)">
          <span class="text-dim">
            {{ patterns.isCategoryCollapsed(group.category.id) ? '▶' : '▼' }}
          </span>
          <h4>
            {{ group.category.name }}
            <span class="text-dim fw-normal">({{ group.patterns.length }})</span>
          </h4>
        </div>
        <div v-show="!patterns.isCategoryCollapsed(group.category.id)" class="pattern-checks">
          <label
            v-for="p in group.patterns"
            :key="p.id"
            :class="['pattern-check', model.includes(p.id) ? 'selected' : '']"
          >
            <input type="checkbox" :value="p.id" v-model="model" />
            <span class="dot"></span>
            <PatternMini :pattern-data="p.pattern_data" size="pattern-mini-sm" />
            <span class="pattern-check-name">{{ p.name }}</span>
          </label>
        </div>
      </template>
    </template>
  </div>
</template>
