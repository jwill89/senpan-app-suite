<script setup lang="ts">
/**
 * Admin Game Presets tab — manage reusable game templates. Each preset bundles
 * a set of win patterns with pre-written (markdown) game details so an admin can
 * start a recurring game in one click from the Game tab.
 */
import { computed, onMounted } from 'vue'
import PatternMini from '@/components/common/PatternMini.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import { usePresetsStore } from '@/stores/presets'
import { usePatternsStore } from '@/stores/patterns'

const presets = usePresetsStore()
const patterns = usePatternsStore()

onMounted(() => {
  presets.loadPresets()
  // The pattern picker needs patterns/categories; AdminView loads them on entry,
  // but reload if they're somehow empty (e.g. deep-link straight to this tab).
  if (patterns.patterns.length === 0) patterns.loadPatterns()
})

/** Pattern name lookup for rendering a preset's pattern chips in the list. */
function patternById(id: number) {
  return patterns.patterns.find((p) => p.id === id)
}

/** True when every currently-visible pattern is selected in the editor. */
const allVisibleSelected = computed(() => {
  const form = presets.editingPreset
  if (!form) return false
  return (
    patterns.gameFilteredPatterns.length > 0 &&
    patterns.gameFilteredPatterns.every((p) => form.pattern_ids.includes(p.id))
  )
})

/** Select / deselect all currently-visible patterns in the editor. */
function toggleSelectAllVisible(): void {
  const form = presets.editingPreset
  if (!form) return
  const visibleIds = patterns.gameFilteredPatterns.map((p) => p.id)
  if (allVisibleSelected.value) {
    const remove = new Set(visibleIds)
    form.pattern_ids = form.pattern_ids.filter((id) => !remove.has(id))
  } else {
    form.pattern_ids = [...new Set([...form.pattern_ids, ...visibleIds])]
  }
}
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel">
      <h3 class="mb-12">Game Presets</h3>
      <p class="text-dim mb-12">
        Save reusable game templates — a set of win patterns plus pre-written game details. Pick a
        preset on the Game tab to start a game with everything filled in.
      </p>

      <!-- Editor (create / edit) -->
      <div v-if="presets.editingPreset" class="preset-editor mb-16">
        <div class="mb-12">
          <label
            style="color: var(--text-dim); font-size: 0.9rem; display: block; margin-bottom: 6px"
          >
            Preset Name
          </label>
          <input
            v-model="presets.editingPreset.name"
            placeholder="e.g. Friday Night Blackout"
            aria-label="Preset name"
            style="width: 100%; max-width: 360px"
          />
        </div>

        <!-- Pattern picker -->
        <div v-if="patterns.patterns.length === 0" class="mb-12">
          <p class="text-dim">Create some win patterns first.</p>
        </div>
        <div v-else class="mb-12">
          <label
            style="color: var(--text-dim); font-size: 0.9rem; display: block; margin-bottom: 6px"
          >
            Win Patterns
          </label>

          <div class="flex-toolbar mb-12">
            <input
              v-model="patterns.patternSearchQuery"
              placeholder="Search patterns…"
              aria-label="Search patterns"
              style="flex: 1; min-width: 140px; max-width: 260px"
            />
            <select
              v-model="patterns.patternCategoryFilter"
              aria-label="Filter by category"
              style="
                padding: 6px 10px;
                border-radius: 6px;
                background: var(--surface);
                color: var(--text);
                border: 1px solid var(--surface2);
              "
            >
              <option :value="null">All Categories</option>
              <option v-for="c in patterns.categories" :key="c.id" :value="c.id">
                {{ c.name }}
              </option>
            </select>
            <button
              class="btn-ghost btn-sm"
              :disabled="patterns.gameFilteredPatterns.length === 0"
              @click="toggleSelectAllVisible"
            >
              <i class="fa-solid fa-circle-check" aria-hidden="true"></i>
              {{ allVisibleSelected ? 'Deselect All' : 'Select All' }}
            </button>
          </div>

          <div class="pattern-checks">
            <label
              v-for="p in patterns.gameFilteredPatterns"
              :key="p.id"
              :class="[
                'pattern-check',
                presets.editingPreset.pattern_ids.includes(p.id) ? 'selected' : '',
              ]"
            >
              <input type="checkbox" :value="p.id" v-model="presets.editingPreset.pattern_ids" />
              <span class="dot"></span>
              <span>{{ p.name }}</span>
              <span style="font-size: 0.75rem; color: var(--text-dim); margin-left: 4px">
                ({{ p.category_name }})
              </span>
              <PatternMini
                :pattern-data="p.pattern_data"
                size="pattern-mini-sm"
                inline
                style="margin-left: 6px"
              />
            </label>
          </div>
        </div>

        <!-- Game details editor -->
        <div class="game-details-editor mb-12">
          <label
            style="color: var(--text-dim); font-size: 0.9rem; display: block; margin-bottom: 6px"
          >
            Game Details <span style="font-size: 0.8rem; opacity: 0.6">(Markdown supported)</span>
          </label>
          <MarkdownEditor
            v-model="presets.editingPreset.game_details"
            min-height="120px"
            placeholder="Enter game details, rules, prizes, etc. Supports bold, italics, lists, and more…"
          />
        </div>

        <div class="flex-toolbar">
          <button class="btn-primary" :disabled="presets.savingPreset" @click="presets.savePreset()">
            <LoadingSpinner v-if="presets.savingPreset" label="Saving…" />
            <template v-else>
              <i class="fa-solid fa-circle-check"></i>
              {{ presets.editingPreset.id ? 'Save Changes' : 'Create Preset' }}
            </template>
          </button>
          <button class="btn-ghost" :disabled="presets.savingPreset" @click="presets.cancelEdit()">
            Cancel
          </button>
        </div>
      </div>

      <!-- New preset button (when not editing) -->
      <div v-else class="mb-16">
        <button class="btn-primary" @click="presets.newPreset()">
          <i class="fa-solid fa-plus"></i> New Preset
        </button>
      </div>

      <!-- Preset list -->
      <LoadingSpinner v-if="presets.presetsLoading" label="Loading presets…" />
      <p v-else-if="presets.presets.length === 0" class="text-dim">No presets yet.</p>
      <div v-else class="preset-list">
        <div v-for="preset in presets.presets" :key="preset.id" class="preset-card">
          <div class="preset-card-head">
            <h4>{{ preset.name }}</h4>
            <div class="flex-toolbar">
              <button class="btn-ghost btn-sm" title="Edit preset" @click="presets.editPreset(preset)">
                <i class="fa-solid fa-pen-to-square"></i> Edit
              </button>
              <button
                class="btn-danger btn-sm"
                title="Delete preset"
                @click="presets.deletePreset(preset.id)"
              >
                <i class="fa-solid fa-trash"></i>
              </button>
            </div>
          </div>
          <div class="pattern-cards">
            <div
              v-for="pid in preset.pattern_ids"
              :key="pid"
              class="pattern-card"
              :title="patternById(pid)?.name || 'Deleted pattern'"
            >
              <PatternMini v-if="patternById(pid)" :pattern-data="patternById(pid)!.pattern_data" />
              <span>{{ patternById(pid)?.name || 'Deleted pattern' }}</span>
            </div>
          </div>
          <p class="text-dim text-xs mt-8">
            {{ preset.game_details ? 'Includes game details' : 'No game details' }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/*
 * Preset editor + list cards sit inside the `.admin-panel` (which is --surface),
 * so they use the lighter --surface2 to stand out — matching the nested-panel
 * convention on the Game management screen (`.game-setup`). This also gives the
 * pattern-mini off-cells (--surface) contrast so the previews read clearly.
 */
.preset-editor {
  border: 1px solid var(--surface2);
  border-radius: var(--radius);
  padding: 16px;
  background: var(--surface2);
}
.preset-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.preset-card {
  border: 1px solid var(--surface2);
  border-radius: var(--radius);
  padding: 12px 14px;
  background: var(--surface2);
}
.preset-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}
.preset-card-head h4 {
  margin: 0;
}
</style>

