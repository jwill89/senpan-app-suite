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
import { Codemirror } from 'vue-codemirror'
import { cssEditorExtensions } from '@/lib/codemirror'
import { useStylesStore } from '@/stores/styles'

const styles = useStylesStore()
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel">
      <h3 class="mb-12"><i class="fa-solid fa-palette"></i> Themes</h3>
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
            <p v-if="styles.styles.length === 0" class="text-dim text-sm" style="padding: 8px">
              No themes yet.
            </p>
          </div>
          <button class="btn-secondary btn-sm mt" style="width: 100%" @click="styles.newStyle()">
            + New Theme
          </button>
          <button
            v-if="
              styles.editingStyle &&
              styles.editingStyle.id &&
              String(styles.editingStyle.id) !== styles.activeStyleId
            "
            class="btn-primary btn-sm mt"
            style="width: 100%"
            @click="styles.setActiveStyle(styles.editingStyle.id)"
          >
            Set Active
          </button>
          <button
            v-if="styles.activeStyleId"
            class="btn-ghost btn-sm mt"
            style="width: 100%"
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
              <button class="btn-primary btn-sm" @click="styles.saveStyle()">Save</button>
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
              :extensions="cssEditorExtensions"
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
