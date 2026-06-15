<script setup lang="ts">
/**
 * Admin Themes tab — custom CSS theme CRUD with a CodeMirror 6 editor
 * (vue-codemirror, replacing the CDN CodeMirror 5). Mirrors the original
 * `adminTab==='system-themes'` block: a theme-list sidebar + the editor pane,
 * with set-active / clear-active controls. Activating applies the CSS live.
 *
 * The CSS editor is bound via v-model to the edited theme's css_content; the
 * dark look matches the original via lib/codemirror.ts + app.css section 26.
 */
import { computed, ref, watch } from 'vue'
import { Codemirror } from 'vue-codemirror'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ThemeColorPickerTool from '@/components/admin/ThemeColorPickerTool.vue'
import { cssEditorExtensions, type EditorColorMode } from '@/lib/codemirror'
import { useStylesStore } from '@/stores/styles'

const styles = useStylesStore()

/**
 * The CSS editor's own colour scheme (dark/light), kept independent of the app
 * theme so authoring stays readable whatever theme is active. Persisted so the
 * admin's preference survives reloads; drives the reactive extension set.
 */
const editorMode = ref<EditorColorMode>(
  localStorage.getItem('theme_editor_cm_mode') === 'light' ? 'light' : 'dark',
)
watch(editorMode, (m) => localStorage.setItem('theme_editor_cm_mode', m))
const editorExtensions = computed(() => cssEditorExtensions(editorMode.value))

function toggleEditorMode(): void {
  editorMode.value = editorMode.value === 'dark' ? 'light' : 'dark'
}
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel">
      <h3 class="mb-12"><font-awesome-icon :icon="['fad', 'palette']" /> Themes</h3>

      <!-- Color picker helper (pick/preview/copy colors to paste into the CSS) -->
      <ThemeColorPickerTool />

      <div class="styles-layout">
        <!-- Style list sidebar -->
        <div class="styles-sidebar">
          <div class="styles-list">
            <div
              v-for="st in styles.styles"
              :key="st.id"
              :class="[
                'style-item',
                styles.editingStyle && styles.editingStyle.id === st.id ? 'selected' : '',
              ]"
              @click="styles.loadStyle(st.id)"
            >
              <span class="style-name">{{ st.name }}</span>
              <span v-if="String(st.id) === styles.activeStyleId" class="style-active-badge">
                Active
              </span>
            </div>
            <LoadingSpinner
              v-if="styles.stylesLoading && styles.styles.length === 0"
              block
              label="Loading…"
            />
            <p
              v-else-if="styles.styles.length === 0"
              class="text-dim text-sm"
              style="padding: 8px"
            >
              No themes yet.
            </p>
          </div>
          <button class="btn-confirm btn-sm mt w-full" @click="styles.newStyle()">
            + New Theme
          </button>
          <button
            v-if="
              styles.editingStyle &&
              styles.editingStyle.id &&
              String(styles.editingStyle.id) !== styles.activeStyleId
            "
            class="btn-action btn-sm mt w-full"
            @click="styles.setActiveStyle(styles.editingStyle.id)"
          >
            Set Active
          </button>
          <button
            v-if="styles.activeStyleId"
            class="btn-caution btn-sm mt w-full"
            @click="styles.setActiveStyle(0)"
          >
            Clear Active Theme
          </button>
        </div>

        <!-- Style editor -->
        <div class="styles-editor">
          <div v-if="styles.editingStyle">
            <div class="flex gap-sm mb">
              <input
                v-model="styles.editingStyle.name"
                placeholder="Theme name"
                aria-label="Theme name"
                style="flex: 1"
              />
              <button
                class="btn-neutral btn-sm"
                :title="`Editor theme: ${editorMode}. Click to switch.`"
                :aria-label="`Switch editor to ${editorMode === 'dark' ? 'light' : 'dark'} mode`"
                @click="toggleEditorMode"
              >
                <font-awesome-icon v-if="editorMode === 'dark'" :icon="['fas', 'moon']" />
                <font-awesome-icon v-else :icon="['fas', 'sun']" />
                {{ editorMode === 'dark' ? 'Dark' : 'Light' }}
              </button>
              <button
                class="btn-confirm btn-sm"
                :disabled="styles.savingStyle"
                @click="styles.saveStyle()"
              >
                <LoadingSpinner v-if="styles.savingStyle" label="Saving…" />
                <template v-else>Save</template>
              </button>
              <button
                v-if="styles.editingStyle.id"
                class="btn-danger btn-sm"
                @click="styles.deleteStyle(styles.editingStyle.id)"
              >
                Delete
              </button>
            </div>
            <Codemirror
              v-model="styles.editingStyle.css_content"
              class="style-css-editor"
              :extensions="editorExtensions"
              :indent-with-tab="false"
              :tab-size="4"
            />
          </div>
          <div v-else class="no-game-msg">Select a theme to edit or create a new one.</div>
        </div>
      </div>
    </div>
  </div>
</template>
