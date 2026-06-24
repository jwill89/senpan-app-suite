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
import SearchInput from '@/components/common/ui/SearchInput.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import { applyUploadedFonts, fontFamilyFromFile } from '@/lib/theme'
import { useFontsStore, fontUrl, FONT_BASE_URL } from '@/stores/fonts'
import { useUiStore } from '@/stores/ui'
import type { FontFile } from '@/types/api'

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
/** Whether the live-preview panel is expanded. Collapsed by default since the
 *  oversized sample stage takes up a fair bit of vertical space. */
const previewExpanded = ref(false)
/** File name of the font selected for the live preview (null = none yet). */
const selectedFontName = ref<string | null>(null)
/** CSS family of the selected preview font (empty when none selected). */
const selectedFamily = computed(() =>
  selectedFontName.value ? fontFamilyFromFile(selectedFontName.value) : '',
)

/** Selects a font for the live-preview panel above the table. Expands the panel
 *  so the chosen font is visible even if the preview was collapsed. */
function selectForPreview(name: string): void {
  selectedFontName.value = name
  previewExpanded.value = true
}

type SortKey = 'name' | 'size' | 'modified'
/** Active sort column + direction. */
const sortKey = ref<SortKey>('name')
const sortDir = ref<'asc' | 'desc'>('asc')

/** Columns for the shared DataTable (the last is the right-aligned actions). */
const fontColumns: DataColumn[] = [
  { key: 'name', label: 'File', sortable: true },
  { key: 'size', label: 'Size', sortable: true },
  { key: 'modified', label: 'Modified', sortable: true },
  { key: 'actions', label: '', align: 'right' },
]

/** Toggles direction when re-clicking the active column, else sorts ascending.
 *  Receives the column key (string) from DataTable's `sort` event. */
function sortBy(key: string): void {
  if (sortKey.value === key) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortKey.value = key as SortKey
    sortDir.value = 'asc'
  }
}

/** Highlights the row whose font is currently shown in the live preview. */
function previewRowClass(f: FontFile): Record<string, boolean> {
  return { 'row-selected': selectedFontName.value === f.name }
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
        <h3 class="m-0"><font-awesome-icon :icon="['fad', 'font']" /> Font Upload</h3>
        <button
          class="btn-action btn-sm push-right"
          :disabled="fonts.uploading"
          @click="pickFiles"
        >
          <LoadingSpinner v-if="fonts.uploading" label="Uploading…" />
          <template v-else><font-awesome-icon :icon="['fas', 'plus']" /> Upload Fonts</template>
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

      <!-- Live preview: type any text, then pick a font (row "Preview" action).
           Collapsed by default since the oversized sample stage is tall. -->
      <div v-if="fonts.fonts.length" class="font-preview-panel mb-12">
        <button
          type="button"
          class="font-preview-toggle"
          :aria-expanded="previewExpanded"
          aria-controls="font-preview-body"
          @click="previewExpanded = !previewExpanded"
        >
          <font-awesome-icon :icon="['fas', previewExpanded ? 'chevron-up' : 'chevron-down']" />
          Live Preview
        </button>
        <div v-show="previewExpanded" id="font-preview-body">
          <FormField label="Preview text" html-for="font-preview-text">
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
      </div>

      <SearchInput
        v-if="fonts.fonts.length"
        v-model="search"
        class="mb-12"
        placeholder="Search fonts by name…"
        aria-label="Search fonts by name"
      />

      <LoadingSpinner
        v-if="fonts.loading && fonts.fonts.length === 0"
        block
        label="Loading fonts…"
      />

      <DataTable
        v-else-if="fonts.fonts.length"
        :columns="fontColumns"
        :rows="displayedFonts"
        row-key="name"
        :sort-key="sortKey"
        :sort-dir="sortDir"
        :row-class="previewRowClass"
        @sort="sortBy"
      >
        <template #cell-name="{ row }">
          <input
            v-if="renamingName === row.name"
            v-model="renameValue"
            class="font-rename-input"
            aria-label="New file name"
            @keyup.enter="commitRename(row.name)"
            @keyup.esc="cancelRename"
          />
          <span v-else class="code-gold">{{ row.name }}</span>
        </template>
        <template #cell-size="{ row }">
          <span class="text-dim">{{ formatSize(row.size) }}</span>
        </template>
        <template #cell-modified="{ row }">
          <span class="text-dim">{{ new Date(row.modified).toLocaleString() }}</span>
        </template>
        <template #cell-actions="{ row }">
          <div class="row-actions">
            <template v-if="renamingName === row.name">
              <button class="btn-confirm btn-sm" @click="commitRename(row.name)">Save</button>
              <button class="btn-neutral btn-sm" @click="cancelRename">Cancel</button>
            </template>
            <template v-else>
              <button
                class="btn-view btn-sm"
                :class="{ active: selectedFontName === row.name }"
                title="Preview this font in the panel above"
                @click="selectForPreview(row.name)"
              >
                <font-awesome-icon :icon="['fas', 'eye']" /> Preview
              </button>
              <button
                class="btn-view btn-sm"
                title="Copy public URL to clipboard"
                @click="copyLink(row.name)"
              >
                <font-awesome-icon :icon="['fas', 'copy']" /> Copy URL
              </button>
              <button
                class="btn-confirm btn-sm"
                title="Rename this font file"
                @click="startRename(row.name)"
              >
                <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Rename
              </button>
              <button
                class="btn-danger btn-sm"
                title="Delete this font file"
                @click="fonts.deleteFont(row.name)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" /> Delete
              </button>
            </template>
          </div>
        </template>
        <template #empty>
          <p class="text-dim ta-center" style="padding: 20px">
            No fonts match “{{ search }}”.
          </p>
        </template>
      </DataTable>

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
/* Collapse toggle: a full-width, borderless header that reads as a label. */
.font-preview-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 0;
  background: none;
  border: none;
  cursor: pointer;
  color: var(--highlight);
  font-weight: 600;
  font-size: inherit;
  text-align: left;
}
#font-preview-body {
  margin-top: 12px;
}
.font-preview-stage {
  margin-top: 12px;
  padding: 16px 18px;
  min-height: 144px;
  background: var(--panel-bg);
  border-radius: var(--radius);
  color: var(--highlight);
  /* The clamped font's own metrics; sample text wraps for long input.
     Doubled from 2rem so the sample is large enough to judge fine details. */
  font-size: 4rem;
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

</style>

