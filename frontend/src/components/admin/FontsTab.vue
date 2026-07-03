<script setup lang="ts">
/**
 * Admin Font Upload tab (Atelier Yao section). A logical FONT groups the
 * uploaded files sharing a base name as format variants (TTF/OTF/WOFF/WOFF2/
 * EOT, plus an auto-converted WOFF2 copy — created only when no WOFF2 was
 * uploaded). Fonts are licensed assets, so they are not downloadable by direct
 * link: they're served through obfuscated token URLs that rotate every 1–2
 * weeks, and each font may only be loaded by ITS allowed sites (this app is
 * always allowed — the font selector keeps working no matter what).
 *
 * The table stays slim — CSS name, served version, modified, actions — and
 * everything else lives in the Edit modal (FontEditModal): file names with
 * rename/delete, sizes, the served-version picker, and the font's allowed
 * sites. A live-preview panel renders custom text in any variant of the
 * selected font, so the converted WOFF2 can be compared against the uploads.
 */
import { computed, onMounted, ref, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import FormField from '@/components/common/ui/FormField.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import FontEditModal from '@/components/admin/FontEditModal.vue'
import { applyUploadedFonts, uploadedFontUrl } from '@/lib/theme'
import { useFontsStore, fontShareUrl, toUploadedFont, FONT_KIT_URL } from '@/stores/fonts'
import { useUiStore } from '@/stores/ui'
import type { Font, FontVariant } from '@/types/api'

const fonts = useFontsStore()
const ui = useUiStore()

// ── External use (the kit embed) ─────────────────────────────────────────────

/** The <link> tag external sites paste into their custom code. */
const kitSnippet = `<link rel="stylesheet" href="${FONT_KIT_URL}">`
/** Whether the external-use panel is expanded. */
const externalExpanded = ref(false)

async function copyKitSnippet(): Promise<void> {
  try {
    await navigator.clipboard.writeText(kitSnippet)
    ui.notify('Embed code copied to clipboard', 'success')
  } catch {
    ui.notify(kitSnippet, 'info')
  }
}

// ── Upload ───────────────────────────────────────────────────────────────────

/** Hidden <input type="file"> used by the Upload button. */
const fileInput = ref<HTMLInputElement | null>(null)

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

// ── Edit modal ───────────────────────────────────────────────────────────────

/** Base name of the font open in the Edit modal (null = closed). */
const editingBase = ref<string | null>(null)

// ── Live preview ─────────────────────────────────────────────────────────────

/** Custom text shown in the live-preview panel. */
const previewText = ref('The quick brown fox jumps over the lazy dog 1234567890')
/** Whether the live-preview panel is expanded. Collapsed by default since the
 *  oversized sample stage takes up a fair bit of vertical space. */
const previewExpanded = ref(false)
/** Base name of the font selected for the live preview (null = none yet). */
const selectedBase = ref<string | null>(null)
/** The selected font's listing row (null when none selected). */
const selectedFont = computed(() => fonts.fonts.find((f) => f.base === selectedBase.value) ?? null)
/** Token of the variant the live preview renders. Defaults to the served one. */
const previewToken = ref('')
watch(selectedFont, (f, old) => {
  // Re-target on selection change or when the current token vanished (e.g. a
  // variant was deleted); keep the user's variant choice across list reloads.
  if (!f) return
  const stillThere = f.variants.some((v) => v.token === previewToken.value)
  if (old?.base !== f.base || !stillThere) previewToken.value = f.served_token
})

/** Escapes a value for a single-quoted CSS string. */
function cssQuote(s: string): string {
  return s.replace(/\\/g, '\\\\').replace(/'/g, "\\'")
}

/** Ad-hoc preview family name for one variant of the selected font. */
function previewVariantFamily(f: Font, v: FontVariant): string {
  return `${f.family} (${v.name}${v.converted ? ' converted' : ''} preview)`
}

// Register ad-hoc @font-face rules for EVERY variant of the selected font, so
// the preview toggle can compare formats. These live in their own <style> and
// use "(… preview)" family names that never collide with the app-registered
// set; a variant's font only downloads when the preview actually renders it.
watch(
  [selectedFont, () => fonts.fonts],
  () => {
    let el = document.getElementById('font-preview-variants') as HTMLStyleElement | null
    if (!el) {
      el = document.createElement('style')
      el.id = 'font-preview-variants'
      document.head.appendChild(el)
    }
    const f = selectedFont.value
    if (!f) {
      el.textContent = ''
      return
    }
    el.textContent = f.variants
      .map(
        (v) =>
          `@font-face{font-family:'${cssQuote(previewVariantFamily(f, v))}';src:url('${uploadedFontUrl(v.token)}');font-display:swap;}`,
      )
      .join('\n')
  },
  { deep: true },
)

/** The variant currently previewed (null when none). */
const previewVariant = computed(
  () => selectedFont.value?.variants.find((v) => v.token === previewToken.value) ?? null,
)

/** CSS family the preview stage renders with (empty when none selected). */
const selectedFamily = computed(() => {
  const f = selectedFont.value
  const v = previewVariant.value
  return f && v ? previewVariantFamily(f, v) : ''
})

/** Selects a font for the live-preview panel above the table. Expands the panel
 *  so the chosen font is visible even if the preview was collapsed. */
function selectForPreview(base: string): void {
  selectedBase.value = base
  previewExpanded.value = true
}

// ── Table ────────────────────────────────────────────────────────────────────

type SortKey = 'family' | 'served_type' | 'modified'
/** Active sort column + direction. */
const sortKey = ref<SortKey>('family')
const sortDir = ref<'asc' | 'desc'>('asc')

/** Columns for the shared DataTable (the last is the right-aligned actions). */
const fontColumns: DataColumn[] = [
  { key: 'family', label: 'CSS Name', sortable: true },
  { key: 'served_type', label: 'Serves', sortable: true },
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
function previewRowClass(f: Font): Record<string, boolean> {
  return { 'row-selected': selectedBase.value === f.base }
}

/** Filtered + sorted rows for display (searches CSS names and file names). */
const displayedFonts = computed(() => {
  const term = search.value.trim().toLowerCase()
  const rows = term
    ? fonts.fonts.filter(
        (f) =>
          f.family.toLowerCase().includes(term) ||
          f.variants.some((v) => v.name.toLowerCase().includes(term)),
      )
    : fonts.fonts.slice()

  const dir = sortDir.value === 'asc' ? 1 : -1
  rows.sort((a, b) => {
    let cmp = 0
    if (sortKey.value === 'family') {
      cmp = a.family.toLowerCase().localeCompare(b.family.toLowerCase())
    } else if (sortKey.value === 'served_type') {
      cmp = a.served_type.localeCompare(b.served_type)
    } else {
      cmp = new Date(a.modified).getTime() - new Date(b.modified).getTime()
    }
    return cmp * dir
  })
  return rows
})

/** Free-text filter applied to CSS names and file names. */
const search = ref('')

/** True when the served variant is the auto-converted WOFF2 copy. */
function servesConverted(f: Font): boolean {
  return f.variants.some((v) => v.converted && v.token === f.served_token)
}

/** Copies the tokenized URL of a font's SERVED variant to the clipboard. The
 *  token rotates every 1–2 weeks — for anything permanent, embed the kit. */
async function copyLink(f: Font): Promise<void> {
  const url = fontShareUrl(f.served_token)
  try {
    await navigator.clipboard.writeText(url)
    ui.notify('Link copied — note it expires in 1–2 weeks (use the kit for embeds)', 'success')
  } catch {
    ui.notify(url, 'info')
  }
}

// Register (and re-register on upload/delete) the fonts' @font-face rules
// (each font's SERVED variant under its CSS name) so the picker and app-wide
// uses stay current. applyUploadedFonts also runs the oversized-metric clamp
// used by the board/header.
onMounted(() => fonts.loadFonts())
watch(
  () => fonts.fonts,
  () => {
    applyUploadedFonts(fonts.fonts.map(toUploadedFont))
    // Keep a valid font selected for the live preview: default to the first,
    // and re-pick if the selected one was deleted/renamed away.
    const stillPresent =
      selectedBase.value !== null && fonts.fonts.some((f) => f.base === selectedBase.value)
    if (!stillPresent) selectedBase.value = fonts.fonts[0]?.base ?? null
  },
  { deep: true },
)
</script>

<template>
  <div class="tab-body">
    <AdminPanel>
      <div class="flex-toolbar mb-12">
        <h3 class="m-0"><font-awesome-icon :icon="['fad', 'font']" /> Font Upload</h3>
        <button class="btn-action btn-sm push-right" :disabled="fonts.uploading" @click="pickFiles">
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
        Allowed types: .ttf, .otf, .woff, .woff2, .eot. Files sharing a name (e.g.
        <span class="code-gold">Jasper.ttf</span> + <span class="code-gold">Jasper.woff2</span>) are
        versions of one font; fonts without a WOFF2 get one converted automatically. Fonts are
        protected: served through obfuscated links that rotate every 1–2 weeks, usable only by this
        app and each font's own allowed sites (managed via <strong>Edit</strong>).
      </p>

      <!-- External use: the kit embed snippet. -->
      <div class="font-external-panel mb-12">
        <button
          type="button"
          class="font-panel-toggle"
          :aria-expanded="externalExpanded"
          aria-controls="font-external-body"
          @click="externalExpanded = !externalExpanded"
        >
          <font-awesome-icon :icon="['fas', externalExpanded ? 'chevron-up' : 'chevron-down']" />
          Embed on External Sites
        </button>
        <div v-show="externalExpanded" id="font-external-body">
          <FormField
            label="Embed on an external site (e.g. Carrd)"
            help="Paste this into the site's custom <head> code, then use each font with CSS: font-family: '<CSS Name>' (the table below). The kit URL never changes — font links inside it refresh automatically, and each site only receives the fonts whose Allowed Sites (Edit) include it."
          >
            <div class="font-inline-row">
              <input
                readonly
                :value="kitSnippet"
                aria-label="Font kit embed code"
                @focus="($event.target as HTMLInputElement).select()"
              />
              <button class="btn-view btn-sm" title="Copy embed code" @click="copyKitSnippet">
                <font-awesome-icon :icon="['fas', 'copy']" /> Copy
              </button>
            </div>
          </FormField>
        </div>
      </div>

      <!-- Live preview: type any text, pick a font (row "Preview" action), and
           switch between its format variants. Collapsed by default. -->
      <div v-if="fonts.fonts.length" class="font-preview-panel mb-12">
        <button
          type="button"
          class="font-panel-toggle"
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
          <!-- Variant toggle: compare the font's formats (shown when it has
               more than one). -->
          <div
            v-if="selectedFont && selectedFont.variants.length > 1"
            class="font-variant-toggle"
            role="group"
          >
            <span class="text-dim text-xs">Version:</span>
            <button
              v-for="v in selectedFont.variants"
              :key="v.token"
              class="btn-view btn-sm"
              :class="{ active: previewToken === v.token }"
              @click="previewToken = v.token"
            >
              {{ v.type }}{{ v.converted ? ' (converted)' : '' }}
            </button>
          </div>
          <div
            class="font-preview-stage"
            :style="selectedFamily ? { fontFamily: `'${selectedFamily}', serif` } : undefined"
          >
            <span v-if="selectedFamily">{{ previewText || 'Type text to preview…' }}</span>
            <span v-else class="text-dim">
              Select a font below (the “Preview” action) to see your text rendered here.
            </span>
          </div>
          <p v-if="selectedFont && previewVariant" class="text-dim text-xs" style="margin: 6px 0 0">
            Previewing <span class="code-gold">{{ selectedFont.family }}</span> —
            {{ previewVariant.type }}{{ previewVariant.converted ? ' (converted)' : '' }}
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
        row-key="base"
        :sort-key="sortKey"
        :sort-dir="sortDir"
        :row-class="previewRowClass"
        @sort="sortBy"
      >
        <template #cell-family="{ row }">
          <span class="code-gold">{{ row.family }}</span>
        </template>
        <template #cell-served_type="{ row }">
          <span :title="servesConverted(row) ? 'Serving the auto-converted WOFF2 copy' : undefined">
            {{ row.served_type
            }}<span v-if="servesConverted(row)" class="text-dim text-xs"> ✦</span>
          </span>
        </template>
        <template #cell-modified="{ row }">
          <span class="text-dim">{{ new Date(row.modified).toLocaleString() }}</span>
        </template>
        <template #cell-actions="{ row }">
          <div class="row-actions">
            <button
              class="btn-view btn-sm"
              :class="{ active: selectedBase === row.base }"
              title="Preview this font in the panel above"
              @click="selectForPreview(row.base)"
            >
              <font-awesome-icon :icon="['fas', 'eye']" /> Preview
            </button>
            <button
              class="btn-view btn-sm"
              title="Copy the served version's tokenized URL (expires in 1–2 weeks — embed the kit stylesheet for anything permanent)"
              @click="copyLink(row)"
            >
              <font-awesome-icon :icon="['fas', 'copy']" /> Copy URL
            </button>
            <button
              class="btn-confirm btn-sm"
              title="Edit this font: CSS name, served version, allowed sites, and files"
              @click="editingBase = row.base"
            >
              <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Edit
            </button>
            <button
              class="btn-danger btn-sm"
              title="Delete this font (all of its files)"
              @click="fonts.deleteFont(row)"
            >
              <font-awesome-icon :icon="['fas', 'trash']" /> Delete
            </button>
          </div>
        </template>
        <template #empty>
          <p class="text-dim ta-center" style="padding: 20px">No fonts match “{{ search }}”.</p>
        </template>
      </DataTable>

      <EmptyState
        v-else-if="!fonts.loading"
        text="No fonts uploaded yet. Use “Upload Fonts” to add some."
      />
    </AdminPanel>

    <FontEditModal v-if="editingBase" :base="editingBase" @close="editingBase = null" />
  </div>
</template>

<style scoped>
/* External-use and live-preview panels share the raised-card look. */
.font-external-panel,
.font-preview-panel {
  background: var(--panel-raised-bg);
  border-radius: var(--radius);
  padding: 16px 18px;
}
#font-external-body,
#font-preview-body {
  margin-top: 12px;
}
/* Collapse toggle: a full-width, borderless header that reads as a label. */
.font-panel-toggle {
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
/* Input + button on one line (kit snippet). */
.font-inline-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.font-inline-row input {
  flex: 1;
  min-width: 0;
}
/* Format-variant toggle above the preview stage. */
.font-variant-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
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
