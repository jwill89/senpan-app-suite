<script setup lang="ts">
/**
 * Admin Themes tab — theme CRUD with a structured design-token editor
 * (ThemeTokenEditor). A theme is a set of token overrides, not free-form CSS;
 * the applied stylesheet is generated from the tokens (server-side, and locally
 * for the live preview). A theme-list sidebar + the editor pane, with
 * set-active / clear-active controls. Activating applies the theme live.
 */
import { computed, onMounted } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ThemeTokenEditor from '@/components/admin/ThemeTokenEditor.vue'
import ImagePicker from '@/components/common/ui/ImagePicker.vue'
import { useStylesStore } from '@/stores/styles'
import { useImagesStore, IMAGE_DIR_FLOURISHES } from '@/stores/images'

const styles = useStylesStore()
const images = useImagesStore()

// SVG flourishes (root-relative paths) for the Board/Number Flourish pickers.
// Only .svg is offered — the board flourish inlines for card export and the
// number flourish is a CSS mask, both of which require SVG.
const flourishPaths = computed(() =>
  (IMAGE_DIR_FLOURISHES in images.imagesByDir ? images.imagesByDir[IMAGE_DIR_FLOURISHES] : [])
    .filter((i) => i.name.toLowerCase().endsWith('.svg'))
    .map((i) => i.path),
)

onMounted(() => images.loadImages(IMAGE_DIR_FLOURISHES))

// Writable view of the edited theme's token map (the store guarantees it's set
// while editing); lets the token editor bind via v-model.
const tokens = computed<Record<string, string>>({
  get: () => styles.editingStyle?.tokens ?? {},
  set: (v) => {
    if (styles.editingStyle) styles.editingStyle.tokens = v
  },
})
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel">
      <div class="manager-header">
        <h3><font-awesome-icon :icon="['fad', 'palette']" /> Themes</h3>
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
            <p v-else-if="styles.styles.length === 0" class="text-dim text-sm" style="padding: 8px">
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

            <ThemeTokenEditor v-model="tokens" />

            <!-- Decorative flourishes (SVG only) sourced from the Flourishes image
                 category. Empty = the app's built-in flourishes. -->
            <div class="flourish-options">
              <div class="flourish-option">
                <label class="field-label">Board Flourish</label>
                <p class="text-dim text-xs mb-8">
                  SVG drawn at the four corners of the player bingo board. Upload SVGs under System
                  → Images → Flourishes. Leave unset to use the built-in flourish.
                </p>
                <ImagePicker v-model="styles.editingStyle.board_flourish" :images="flourishPaths" />
              </div>
              <div class="flourish-option">
                <label class="field-label">Number Flourish</label>
                <p class="text-dim text-xs mb-8">
                  SVG shown either side of the “Last Called” number (player view + Game tab). Leave
                  unset to use the built-in flourish.
                </p>
                <ImagePicker
                  v-model="styles.editingStyle.number_flourish"
                  :images="flourishPaths"
                />
              </div>
            </div>
          </div>
          <div v-else class="no-game-msg">Select a theme to edit or create a new one.</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Flourish pickers below the token editor. */
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
