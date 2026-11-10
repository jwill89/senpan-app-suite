<script setup lang="ts">
/**
 * Admin Game Presets tab - manage reusable game templates. Each preset bundles
 * a set of win patterns with pre-written (markdown) game details so an admin can
 * start a recurring game in one click from the Game tab.
 */
import { onMounted } from 'vue'
import PatternMini from '@/components/common/PatternMini.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import ListRow from '@/components/common/ui/ListRow.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import PatternPicker from '@/components/common/ui/PatternPicker.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import { AUTO_INTERVAL_OPTIONS, autoIntervalLabel } from '@/lib/constants'
import { usePresetsStore } from '@/stores/presets'
import { usePatternsStore } from '@/stores/patterns'

const presets = usePresetsStore()
const patterns = usePatternsStore()

onMounted(() => {
  void presets.loadPresets()
  // The pattern picker needs patterns/categories; AdminView loads them on entry,
  // but reload if they're somehow empty (e.g. deep-link straight to this tab).
  if (patterns.patterns.length === 0) {
    void patterns.loadPatterns()
  }
})

/** Pattern name lookup for rendering a preset's pattern chips in the list. */
function patternById(id: number) {
  return patterns.patterns.find((p) => p.id === id)
}
</script>

<template>
  <div class="tab-body">
    <!-- -- Editor sub-page (create / edit) --------------------------------- -->
    <div v-if="presets.editingPreset" class="admin-panel">
      <SubPageHeader
        :icon="['fad', 'ballot']"
        :title="presets.editingPreset.id ? 'Edit Preset' : 'New Preset'"
        @back="presets.cancelEdit()"
      />

      <FormField label="Preset Name">
        <input
          v-model="presets.editingPreset.name"
          placeholder="e.g. Friday Night Blackout"
          aria-label="Preset name"
          class="max-w-360"
        />
      </FormField>

      <!-- Pattern picker -->
      <p v-if="patterns.patterns.length === 0" class="text-muted mb-12">
        Create some win patterns first.
      </p>
      <FormField v-else label="Win Patterns">
        <PatternPicker v-model="presets.editingPreset.patternIds" />
      </FormField>

      <FormField>
        <template #label>
          Game Details
          <span class="text-muted fw-normal text-xs"> (Markdown supported) </span>
        </template>
        <MarkdownEditor
          v-model="presets.editingPreset.gameDetails"
          min-height="120px"
          placeholder="Enter game details, rules, prizes, etc. Supports bold, italics, lists, and more..."
        />
      </FormField>

      <FormField label="Auto-Draw">
        <label class="checkbox-inline checkbox-inline--strong">
          <input v-model="presets.editingPreset.auto" type="checkbox" />
          <span>Start with auto-draw on</span>
        </label>
        <div v-if="presets.editingPreset.auto" class="preset-auto-interval">
          <label class="text-muted text-sm" for="preset-auto-interval">Time Between Calls:</label>
          <select
            id="preset-auto-interval"
            v-model.number="presets.editingPreset.autoInterval"
            class="manager-filter"
          >
            <option v-for="s in AUTO_INTERVAL_OPTIONS" :key="s" :value="s">
              {{ autoIntervalLabel(s) }}
            </option>
          </select>
        </div>
      </FormField>

      <FormActions align="start">
        <button
          type="button"
          class="btn-confirm"
          :disabled="presets.savingPreset"
          @click="presets.savePreset()"
        >
          <LoadingSpinner v-if="presets.savingPreset" label="Saving..." />
          <template v-else>
            <font-awesome-icon :icon="['fas', 'circle-check']" />
            {{ presets.editingPreset.id ? 'Save Changes' : 'Create Preset' }}
          </template>
        </button>
        <button
          type="button"
          class="btn-neutral"
          :disabled="presets.savingPreset"
          @click="presets.cancelEdit()"
        >
          Cancel
        </button>
      </FormActions>
    </div>

    <!-- -- List ------------------------------------------------------------ -->
    <ManagerView v-else title="Game Presets" :icon="['fad', 'ballot']">
      <template #actions>
        <button type="button" class="btn-confirm btn-sm" @click="presets.newPreset()">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Preset
        </button>
      </template>

      <p class="text-muted mb-12">
        Save reusable game templates - a set of win patterns plus pre-written game details. Pick a
        preset on the Game tab to start a game with everything filled in.
      </p>

      <LoadingSpinner v-if="presets.presetsLoading" label="Loading presets..." />
      <EmptyState v-else-if="presets.presets.length === 0" text="No presets yet." />
      <div v-else class="list-rows">
        <ListRow v-for="preset in presets.presets" :key="preset.id">
          <h4 class="mt-0 mb-8">{{ preset.name }}</h4>
          <div class="pattern-cards">
            <div
              v-for="pid in preset.patternIds"
              :key="pid"
              class="pattern-card"
              :title="patternById(pid)?.name || 'Deleted pattern'"
            >
              <PatternMini v-if="patternById(pid)" :pattern-data="patternById(pid)!.patternData" />
              <span>{{ patternById(pid)?.name || 'Deleted pattern' }}</span>
            </div>
          </div>
          <p class="text-muted text-xs mt-8">
            {{ preset.gameDetails ? 'Includes game details' : 'No game details' }}
            <template v-if="preset.auto">
              - Auto-draw every {{ autoIntervalLabel(preset.autoInterval) }}</template
            >
          </p>
          <template #actions>
            <button
              type="button"
              class="btn-confirm btn-sm"
              title="Edit preset"
              @click="presets.editPreset(preset)"
            >
              <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Edit
            </button>
            <button
              type="button"
              class="btn-danger btn-sm"
              title="Delete preset"
              @click="presets.deletePreset(preset.id)"
            >
              <font-awesome-icon :icon="['fas', 'trash']" />
            </button>
          </template>
        </ListRow>
      </div>
    </ManagerView>
  </div>
</template>

<style scoped>
.preset-auto-interval {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
}
</style>
