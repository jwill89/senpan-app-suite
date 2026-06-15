<script setup lang="ts">
/**
 * Admin Font Upload tab (Atelier Yao section) — lists the font files in the
 * <webRoot>/fonts directory with a public link to each, supports uploading one
 * or more font files at once, and lets the admin rename or delete files. The
 * table refreshes after a successful upload. A file whose name already exists
 * is rejected by the server (the existing one must be deleted first).
 *
 * The table is searchable (by file name) and sortable by name / size / modified.
 * A live-preview panel above the table renders custom text in whichever font the
 * admin selects (the "Preview" row action), with the same oversized-metric
 * clamping the board/header use (via applyUploadedFonts → @font-face overrides).
 * Each row's actions include "Copy URL" (copies the public URL to the clipboard).
 */
import { computed, onMounted, ref, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import FormField from '@/components/common/ui/FormField.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import { applyUploadedFonts, fontFamilyFromFile } from '@/lib/theme'
import { useFontsStore, fontUrl, FONT_BASE_URL } from '@/stores/fonts'
import { useUiStore } from '@/stores/ui'

const fonts = useFontsStore()
const ui = useUiStore()

/** Hidden <input type="file"> used by the Upload button. */
const fileInput = ref<HTMLInputElement | null>(null)

/** Name of the font currently being renamed (inline editor), or null. */
const renamingName = ref<string | null>(null)
/** Working value of the inline rename input. */
const renameValue = ref('')

/** Free-text filter applied to the file name. */
const search = ref('')

/** Custom text shown in the live-preview panel. */
const previewText = ref('The quick brown fox jumps over the lazy dog 1234567890')
/** File name of the font selected for the live preview (null = none yet). */
const selectedFontName = ref<string | null>(null)
/** CSS family of the selected preview font (empty when none selected). */
const selectedFamily = computed(() =>
  selectedFontName.value ? fontFamilyFromFile(selectedFontName.value) : '',
)

/** Selects a font for the live-preview panel above the table. */
function selectForPreview(name: string): void {
  selectedFontName.value = name
}

type SortKey = 'name' | 'size' | 'modified'
/** Active sort column + direction. */
const sortKey = ref<SortKey>('name')
const sortDir = ref<'asc' | 'desc'>('asc')

/** Toggles direction when re-clicking the active column, else sorts ascending. */
function sortBy(key: SortKey): void {
  if (sortKey.value === key) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortKey.value = key
    sortDir.value = 'asc'
  }
}

/** Arrow indicator for a column header (empty when not the active sort). */
function sortIndicator(key: SortKey): string {
  if (sortKey.value !== key) return ''
  return sortDir.value === 'asc' ? ' ▲' : ' ▼'
}

/** Filtered + sorted rows for display. */
const displayedFonts = computed(() => {
  const term = search.value.trim().toLowerCase()
  const rows = term
    ? fonts.fonts.filter((f) => f.name.toLowerCase().includes(term))
    : fonts.fonts.slice()

  const dir = sortDir.value === 'asc' ? 1 : -1
  rows.sort((a, b) => {
    let cmp = 0
    if (sortKey.value === 'name') {
      cmp = a.name.toLowerCase().localeCompare(b.name.toLowerCase())
    } else if (sortKey.value === 'size') {
      cmp = a.size - b.size
    } else {
      cmp = new Date(a.modified).getTime() - new Date(b.modified).getTime()
    }
    return cmp * dir
  })
  return rows
})

/** Copies a font's public URL to the clipboard. */
async function copyLink(name: string): Promise<void> {
  const url = fontUrl(name)
  try {
    await navigator.clipboard.writeText(url)
    ui.notify('Link copied to clipboard', 'success')
  } catch {
    ui.notify(url, 'info')
  }
}

function pickFiles(): void {
  fileInput.value?.click()
}

async function onFilesSelected(e: Event): Promise<void> {
  const input = e.target as HTMLInputElement
  if (input.files && input.files.length > 0) {
    await fonts.uploadFonts(input.files)
  }
  // Reset so selecting the same file again re-triggers change.
  input.value = ''
}

function startRename(name: string): void {
  renamingName.value = name
  renameValue.value = name
}

function cancelRename(): void {
  renamingName.value = null
  renameValue.value = ''
}

async function commitRename(name: string): Promise<void> {
  const newName = renameValue.value.trim()
  const ok = await fonts.renameFont(name, renameValue.value)
  if (ok) {
    // Follow the rename so the live preview keeps pointing at the same font.
    if (selectedFontName.value === name) selectedFontName.value = newName
    cancelRename()
  }
}

/** Human-readable file size. */
function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

// Register (and re-register on upload/delete) the uploaded fonts' @font-face
// rules so each preview cell renders in its own font. applyUploadedFonts also
// runs the oversized-metric clamp, so the previews match the board/header.
onMounted(() => fonts.loadFonts())
watch(
  () => fonts.fonts,
  () => {
    applyUploadedFonts(fonts.fonts.map((f) => f.name))
    // Keep a valid font selected for the live preview: default to the first,
    // and re-pick if the selected one was deleted/renamed away.
    const stillPresent =
      selectedFontName.value !== null &&
      fonts.fonts.some((f) => f.name === selectedFontName.value)
    if (!stillPresent) selectedFontName.value = fonts.fonts[0]?.name ?? null
  },
  { deep: true },
)
</script>

<template>
  <div class="tab-body">
    <AdminPanel>
      <div class="flex-toolbar mb-12">
        <h3 style="margin: 0"><i class="fa-duotone fa-font"></i> Font Upload</h3>
        <button
          class="btn-primary btn-sm push-right"
          :disabled="fonts.uploading"
          @click="pickFiles"
        >
          <LoadingSpinner v-if="fonts.uploading" label="Uploading…" />
          <template v-else><i class="fa-solid fa-plus"></i> Upload Fonts</template>
        </button>
        <input
          ref="fileInput"
          type="file"
          accept=".ttf,.otf,.woff,.woff2,.eot"
          multiple
          hidden
          @change="onFilesSelected"
        />
      </div>

      <p class="text-dim text-xs mb-12">
        Files are served from
        <span class="code-gold">{{ FONT_BASE_URL }}</span>. Allowed types: .ttf, .otf, .woff,
        .woff2, .eot. To replace a font, delete the old file first.
      </p>

      <!-- Live preview: type any text, then pick a font (row "Preview" action). -->
      <div v-if="fonts.fonts.length" class="font-preview-panel mb-12">
        <FormField label="Live Preview" html-for="font-preview-text">
          <input
            id="font-preview-text"
            v-model="previewText"
            placeholder="Type text to preview…"
            aria-label="Preview text"
          />
        </FormField>
        <div
          class="font-preview-stage"
          :style="selectedFamily ? { fontFamily: `'${selectedFamily}', serif` } : undefined"
        >
          <span v-if="selectedFamily">{{ previewText || 'Type text to preview…' }}</span>
          <span v-else class="text-dim">
            Select a font below (the “Preview” action) to see your text rendered here.
          </span>
        </div>
        <p v-if="selectedFamily" class="text-dim text-xs" style="margin: 6px 0 0">
          Previewing <span class="code-gold">{{ selectedFamily }}</span>
        </p>
      </div>

      <div v-if="fonts.fonts.length" class="font-search mb-12">
        <i class="fa-duotone fa-magnifying-glass" aria-hidden="true"></i>
        <input
          v-model="search"
          type="search"
          placeholder="Search fonts by name…"
          aria-label="Search fonts by name"
        />
      </div>

      <LoadingSpinner
        v-if="fonts.loading && fonts.fonts.length === 0"
        block
        label="Loading fonts…"
      />

      <div v-else-if="fonts.fonts.length" class="data-table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>
                <button class="th-sort" @click="sortBy('name')">
                  File<span class="th-sort-arrow">{{ sortIndicator('name') }}</span>
                </button>
              </th>
              <th>
                <button class="th-sort" @click="sortBy('size')">
                  Size<span class="th-sort-arrow">{{ sortIndicator('size') }}</span>
                </button>
              </th>
              <th>
                <button class="th-sort" @click="sortBy('modified')">
                  Modified<span class="th-sort-arrow">{{ sortIndicator('modified') }}</span>
                </button>
              </th>
              <th style="text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="font in displayedFonts"
              :key="font.name"
              :class="{ 'row-selected': selectedFontName === font.name }"
            >
              <td>
                <template v-if="renamingName === font.name">
                  <input
                    v-model="renameValue"
                    class="font-rename-input"
                    aria-label="New file name"
                    @keyup.enter="commitRename(font.name)"
                    @keyup.esc="cancelRename"
                  />
                </template>
                <span v-else class="code-gold">{{ font.name }}</span>
              </td>
              <td class="text-dim">{{ formatSize(font.size) }}</td>
              <td class="text-dim">{{ new Date(font.modified).toLocaleString() }}</td>
              <td style="text-align: right; white-space: nowrap">
                <template v-if="renamingName === font.name">
                  <button class="btn-primary btn-sm" @click="commitRename(font.name)">
                    Save
                  </button>
                  <button class="btn-ghost btn-sm" @click="cancelRename">Cancel</button>
                </template>
                <template v-else>
                  <button
                    class="btn-ghost btn-sm"
                    :class="{ active: selectedFontName === font.name }"
                    title="Preview this font in the panel above"
                    @click="selectForPreview(font.name)"
                  >
                    <i class="fa-solid fa-eye"></i> Preview
                  </button>
                  <button
                    class="btn-ghost btn-sm"
                    title="Copy public URL to clipboard"
                    @click="copyLink(font.name)"
                  >
                    <i class="fa-solid fa-copy"></i> Copy URL
                  </button>
                  <button
                    class="btn-ghost btn-sm"
                    title="Rename this font file"
                    @click="startRename(font.name)"
                  >
                    <i class="fa-solid fa-pen-to-square"></i> Rename
                  </button>
                  <button
                    class="btn-danger btn-sm"
                    title="Delete this font file"
                    @click="fonts.deleteFont(font.name)"
                  >
                    <i class="fa-solid fa-trash"></i> Delete
                  </button>
                </template>
              </td>
            </tr>
            <tr v-if="displayedFonts.length === 0">
              <td colspan="4" class="text-dim" style="text-align: center; padding: 20px">
                No fonts match “{{ search }}”.
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <EmptyState
        v-else-if="!fonts.loading"
        text="No fonts uploaded yet. Use “Upload Fonts” to add some."
      />
    </AdminPanel>
  </div>
</template>

<style scoped>
.font-rename-input {
  width: 100%;
  min-width: 160px;
}

/* Live-preview panel: text input + the rendered sample stage. */
.font-preview-panel {
  background: var(--panel-raised-bg);
  border-radius: var(--radius);
  padding: 16px 18px;
}
.font-preview-stage {
  margin-top: 12px;
  padding: 16px 18px;
  min-height: 72px;
  background: var(--panel-bg);
  border-radius: var(--radius);
  color: var(--highlight);
  /* The clamped font's own metrics; sample text wraps for long input. */
  font-size: 2rem;
  line-height: 1.3;
  overflow-wrap: anywhere;
}

/* Highlight the row whose font is currently in the live preview. A warm gold
   tint (not the gray --panel-raised-bg used by the ghost buttons' borders) keeps the
   action buttons clearly visible; a gold bar marks the selected row. */
.row-selected td {
  background: color-mix(in srgb, var(--highlight) 14%, transparent);
}
.row-selected td:first-child {
  box-shadow: inset 4px 0 0 0 var(--highlight);
}

/* Search box */
.font-search {
  display: flex;
  align-items: center;
  gap: 8px;
  max-width: 360px;
  color: var(--text-muted);
}
.font-search input {
  flex: 1;
}

/* Sortable column headers — render as plain text buttons. */
.th-sort {
  background: none;
  border: 0;
  padding: 0;
  margin: 0;
  font: inherit;
  color: inherit;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
}
.th-sort:hover {
  color: var(--highlight);
}
.th-sort-arrow {
  font-size: 0.7em;
  min-width: 1em;
}
</style>

