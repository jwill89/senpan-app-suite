<script setup lang="ts">
/**
 * Admin Font Upload tab (System section) — lists the font files in the
 * <webRoot>/fonts directory with a public link to each, supports uploading one
 * or more font files at once, and lets the admin rename or delete files. The
 * table refreshes after a successful upload. A file whose name already exists
 * is rejected by the server (the existing one must be deleted first).
 */
import { onMounted, ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useFontsStore, fontUrl, FONT_BASE_URL } from '@/stores/fonts'

const fonts = useFontsStore()

/** Hidden <input type="file"> used by the Upload button. */
const fileInput = ref<HTMLInputElement | null>(null)

/** Name of the font currently being renamed (inline editor), or null. */
const renamingName = ref<string | null>(null)
/** Working value of the inline rename input. */
const renameValue = ref('')

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
  const ok = await fonts.renameFont(name, renameValue.value)
  if (ok) cancelRename()
}

/** Human-readable file size. */
function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

onMounted(() => fonts.loadFonts())
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel" style="padding: 24px">
      <div class="flex-toolbar mb-12" style="gap: 12px; align-items: center">
        <h3 style="margin: 0"><i class="fa-solid fa-font"></i> Font Upload</h3>
        <button
          class="btn-primary btn-sm"
          style="margin-left: auto"
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

      <LoadingSpinner
        v-if="fonts.loading && fonts.fonts.length === 0"
        block
        label="Loading fonts…"
      />

      <div v-else-if="fonts.fonts.length" style="overflow-x: auto">
        <table class="winners-log-table">
          <thead>
            <tr>
              <th>File</th>
              <th>Link</th>
              <th>Size</th>
              <th>Modified</th>
              <th style="text-align: right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="font in fonts.fonts" :key="font.name">
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
              <td>
                <a :href="fontUrl(font.name)" target="_blank" rel="noopener" class="font-link">
                  {{ fontUrl(font.name) }}
                </a>
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
          </tbody>
        </table>
      </div>

      <p v-else-if="!fonts.loading" class="msg-block" style="padding: 24px">
        No fonts uploaded yet. Use “Upload Fonts” to add some.
      </p>
    </div>
  </div>
</template>

<style scoped>
.font-rename-input {
  width: 100%;
  min-width: 160px;
}
.font-link {
  word-break: break-all;
}
</style>

