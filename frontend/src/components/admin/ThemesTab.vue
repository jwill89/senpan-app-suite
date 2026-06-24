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
import { computed, onMounted, ref, watch } from 'vue'
import { Codemirror } from 'vue-codemirror'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import ThemeColorPickerTool from '@/components/admin/ThemeColorPickerTool.vue'
import ImagePicker from '@/components/common/ui/ImagePicker.vue'
import { cssEditorExtensions, type EditorColorMode } from '@/lib/codemirror'
import { useStylesStore } from '@/stores/styles'
import { useImagesStore, IMAGE_DIR_FLOURISHES } from '@/stores/images'

const styles = useStylesStore()
const images = useImagesStore()

// SVG flourishes (root-relative paths) for the Board/Number Flourish pickers.
// Only .svg is offered — the board flourish inlines for card export and the
// number flourish is a CSS mask, both of which require SVG.
const flourishPaths = computed(() =>
  (images.imagesByDir[IMAGE_DIR_FLOURISHES] || [])
    .filter((i) => i.name.toLowerCase().endsWith('.svg'))
    .map((i) => i.path),
)

onMounted(() => images.loadImages(IMAGE_DIR_FLOURISHES))

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

// The color-picker helper now opens in a modal (it was an always-visible bar
// whose relationship to the editor was unclear). It's an authoring aid only —
// pick/preview a color and copy its HEX/RGBA to paste into the CSS editor.
const showColorTool = ref(false)
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel">
      <div class="manager-header">
        <h3><font-awesome-icon :icon="['fad', 'palette']" /> Themes</h3>
        <div class="manager-actions">
          <button class="btn-view btn-sm" @click="showColorTool = true">
            <font-awesome-icon :icon="['fad', 'palette']" /> Color Tool
          </button>
        </div>
      </div>

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

            <!-- Decorative flourishes (SVG only) sourced from the Flourishes image
                 category. Empty = the app's built-in flourishes. -->
            <div class="flourish-options">
              <div class="flourish-option">
                <label class="field-label">Board Flourish</label>
                <p class="text-dim text-xs mb-8">
                  SVG drawn at the four corners of the player bingo board. Upload SVGs under
                  System → Images → Flourishes. Leave unset to use the built-in flourish.
                </p>
                <ImagePicker v-model="styles.editingStyle.board_flourish" :images="flourishPaths" />
              </div>
              <div class="flourish-option">
                <label class="field-label">Number Flourish</label>
                <p class="text-dim text-xs mb-8">
                  SVG shown either side of the “Last Called” number (player view + Game tab). Leave
                  unset to use the built-in flourish.
                </p>
                <ImagePicker v-model="styles.editingStyle.number_flourish" :images="flourishPaths" />
              </div>
            </div>
          </div>
          <div v-else class="no-game-msg">Select a theme to edit or create a new one.</div>
        </div>
      </div>
    </div>

    <!-- Color-picker helper modal (authoring aid; copies HEX/RGBA to paste). -->
    <ModalOverlay
      v-if="showColorTool"
      aria-label="Theme color tool"
      box-style="max-width: 440px"
      @close="showColorTool = false"
    >
      <h3 class="mt-0"><font-awesome-icon :icon="['fad', 'palette']" /> Color Tool</h3>
      <p class="text-dim text-sm mb-12">
        Pick or paste any CSS color, then copy its HEX or RGBA value to paste into the theme CSS
        editor.
      </p>
      <ThemeColorPickerTool />
    </ModalOverlay>
  </div>
</template>

<style scoped>
/* Flourish pickers below the CSS editor. */
.flourish-options {
  display: flex;
  flex-wrap: wrap;
  gap: 20px;
  margin-top: 16px;
}
.flourish-option {
  flex: 1 1 280px;
  min-width: 260px;
}
.flourish-option .field-label {
  font-weight: 600;
}
</style>
