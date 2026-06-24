<script setup lang="ts">
/**
 * Admin Images tab (System section) — central image hosting. The admin picks a
 * category (which maps to a subdirectory of <webRoot>/images) and uploads any
 * number of images at once (drag-and-drop or click to browse), browses the
 * images in that category, and deletes them.
 *
 * Three categories are permanent (Announcement Main / Announcement Thumbnail /
 * Raffle) and feed the announcement + raffle editors' pickers; admins can add
 * custom categories (a display name + a directory) and rename/delete those.
 *
 * Two screens (a lightweight in-tab router via `screen`):
 *   - browse:     category picker + upload drop zone + image grid.
 *   - categories: manage categories (add / rename / delete via a modal).
 */
import { computed, onMounted, ref, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import FormField from '@/components/common/ui/FormField.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import { useImagesStore } from '@/stores/images'
import { useUiStore } from '@/stores/ui'
import { assetUrl } from '@/lib/assets'
import type { ImageCategory } from '@/types/api'

const images = useImagesStore()
const ui = useUiStore()

type Screen = 'browse' | 'categories'
const screen = ref<Screen>('browse')

/** Directory of the category currently being browsed. */
const selectedDir = ref('')

/** Hidden <input type="file"> used by the Upload button. */
const fileInput = ref<HTMLInputElement | null>(null)
/** True while a file is being dragged over the drop zone. */
const dragOver = ref(false)

const selectedCategory = computed(
  () => images.sortedCategories.find((c) => c.dir === selectedDir.value) ?? null,
)
const currentImages = computed(() => images.imagesByDir[selectedDir.value] || [])

/** Keep a valid category selected as the list loads/changes. */
watch(
  () => images.sortedCategories,
  (cats) => {
    if (cats.length && !cats.some((c) => c.dir === selectedDir.value)) {
      selectedDir.value = cats[0].dir
    }
  },
  { immediate: true },
)
// Load the selected category's images whenever the selection changes.
watch(selectedDir, (dir) => {
  if (dir) images.loadImages(dir)
})

// ── Category management modal ────────────────────────────────────────────────
const categoryModal = ref<{ mode: 'create' | 'edit'; dir: string } | null>(null)
const formName = ref('')
const formDir = ref('')
const savingCategory = ref(false)

/** Mirrors the server's directory slug rules (spaces → underscores) for preview. */
function slugifyDir(s: string): string {
  return s
    .toLowerCase()
    .trim()
    .replace(/[\s-]+/g, '_')
    .replace(/[^a-z0-9_]/g, '')
    .replace(/_+/g, '_')
    .replace(/^_+|_+$/g, '')
}
const formDerivedDir = computed(() => slugifyDir(formDir.value || formName.value))

const categoryColumns: DataColumn[] = [
  { key: 'name', label: 'Category' },
  { key: 'dir', label: 'Directory' },
  { key: 'file_count', label: 'Images', align: 'center' },
  { key: 'total_size', label: 'Size', align: 'right' },
  { key: 'actions', label: '', align: 'right' },
]

function openNewCategory(): void {
  formName.value = ''
  formDir.value = ''
  categoryModal.value = { mode: 'create', dir: '' }
}
function startEditCategory(c: ImageCategory): void {
  formName.value = c.name
  formDir.value = c.dir
  categoryModal.value = { mode: 'edit', dir: c.dir }
}
function closeModal(): void {
  categoryModal.value = null
}

async function submitCategory(): Promise<void> {
  if (!categoryModal.value || !formName.value.trim()) return
  savingCategory.value = true
  try {
    const dir =
      categoryModal.value.mode === 'edit'
        ? await images.saveCategory('rename', formName.value, categoryModal.value.dir, formDir.value)
        : await images.saveCategory('create', formName.value, formDir.value)
    if (dir) {
      selectedDir.value = dir
      closeModal()
    }
  } finally {
    savingCategory.value = false
  }
}

// ── Upload + delete (browse screen) ──────────────────────────────────────────
function pickFiles(): void {
  fileInput.value?.click()
}
async function onFilesSelected(e: Event): Promise<void> {
  const input = e.target as HTMLInputElement
  if (input.files && input.files.length > 0 && selectedDir.value) {
    await images.uploadImages(selectedDir.value, input.files)
  }
  input.value = '' // reset so selecting the same file re-triggers change
}
function onDrop(e: DragEvent): void {
  dragOver.value = false
  const files = e.dataTransfer?.files
  if (files && files.length > 0 && selectedDir.value) {
    images.uploadImages(selectedDir.value, files)
  }
}

/** Copies an image's absolute public URL to the clipboard. */
async function copyUrl(url: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(url)
    ui.notify('URL copied to clipboard', 'success')
  } catch {
    ui.notify(url, 'info')
  }
}

/** Human-readable file size. */
function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

onMounted(() => images.loadCategories())
</script>

<template>
  <div class="tab-body">
    <!-- ── Manage categories ─────────────────────────────────────────────────── -->
    <AdminPanel v-if="screen === 'categories'">
      <SubPageHeader title="Image Categories" :icon="['fad', 'folder-open']" @back="screen = 'browse'" />
      <p class="text-dim text-xs mb-12">
        Each category maps to a subdirectory of <span class="code-gold">images/</span>. The three
        permanent categories back the announcement and raffle editors and can't be renamed or
        deleted. Deleting a custom category removes its folder and all images in it.
      </p>
      <div class="flex-toolbar flex-end mb-16">
        <button class="btn-confirm btn-sm" @click="openNewCategory">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Category
        </button>
      </div>

      <DataTable :columns="categoryColumns" :rows="images.sortedCategories" row-key="dir">
        <template #cell-name="{ row }">
          <font-awesome-icon :icon="['fad', 'folder']" /> {{ row.name }}
          <span v-if="row.permanent" class="badge badge--muted" style="margin-left: 6px">
            Permanent
          </span>
        </template>
        <template #cell-dir="{ row }">
          <span class="code-gold">images/{{ row.dir }}</span>
        </template>
        <template #cell-file_count="{ row }">
          <span class="text-dim">{{ row.file_count }}</span>
        </template>
        <template #cell-total_size="{ row }">
          <span class="text-dim">{{ formatSize(row.total_size) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <div class="row-actions">
            <button
              class="btn-confirm btn-sm"
              :disabled="row.permanent"
              :title="row.permanent ? 'Permanent categories can\'t be renamed' : 'Rename this category'"
              @click="startEditCategory(row)"
            >
              <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Edit
            </button>
            <button
              class="btn-danger btn-sm"
              :disabled="row.permanent"
              :title="row.permanent ? 'Permanent categories can\'t be deleted' : 'Delete this category'"
              @click="images.deleteCategory(row)"
            >
              <font-awesome-icon :icon="['fas', 'trash']" /> Delete
            </button>
          </div>
        </template>
        <template #empty>
          <EmptyState text="No categories yet." />
        </template>
      </DataTable>
    </AdminPanel>

    <!-- ── Browse: category picker + upload + image grid ─────────────────────── -->
    <ManagerView v-else title="Images" :icon="['fad', 'images']">
      <template #actions>
        <button class="btn-view btn-sm" @click="screen = 'categories'">
          <font-awesome-icon :icon="['fad', 'folder-open']" /> Manage Categories
        </button>
      </template>

      <p class="text-dim text-xs mb-12">
        Pick a category, then drag &amp; drop images (or click to browse) to upload them.
        Allowed types: .jpg, .jpeg, .png, .webp, .gif. Uploading a file with an existing name
        replaces it.
      </p>

      <LoadingSpinner
        v-if="images.loading && images.categories.length === 0"
        block
        label="Loading categories…"
      />

      <template v-else>
        <div class="flex-toolbar mb-12" style="gap: 12px; align-items: center">
          <FormField label="Category" html-for="image-category" style="margin: 0; min-width: 240px">
            <select id="image-category" v-model="selectedDir" aria-label="Image category">
              <option v-for="c in images.sortedCategories" :key="c.dir" :value="c.dir">
                {{ c.name }}
              </option>
            </select>
          </FormField>
          <button
            class="btn-action btn-sm push-right"
            :disabled="images.uploading || !selectedDir"
            @click="pickFiles"
          >
            <LoadingSpinner v-if="images.uploading" label="Uploading…" />
            <template v-else><font-awesome-icon :icon="['fas', 'upload']" /> Upload Images</template>
          </button>
          <input
            ref="fileInput"
            type="file"
            accept=".jpg,.jpeg,.png,.webp,.gif"
            multiple
            hidden
            @change="onFilesSelected"
          />
        </div>

        <p v-if="selectedCategory" class="text-dim text-xs mb-12">
          Uploading to <span class="code-gold">images/{{ selectedCategory.dir }}/</span>
        </p>

        <!-- Drag-and-drop zone -->
        <div
          class="image-dropzone"
          :class="{ 'drag-over': dragOver, disabled: !selectedDir }"
          @dragover.prevent="dragOver = true"
          @dragenter.prevent="dragOver = true"
          @dragleave.prevent="dragOver = false"
          @drop.prevent="onDrop"
          @click="pickFiles"
        >
          <font-awesome-icon :icon="['fad', 'cloud-arrow-up']" />
          <span>Drag &amp; drop images here, or click to browse</span>
        </div>

        <LoadingSpinner
          v-if="images.loadingImages && currentImages.length === 0"
          block
          label="Loading images…"
        />

        <div v-else-if="currentImages.length" class="image-grid">
          <figure v-for="img in currentImages" :key="img.name" class="image-card">
            <div class="image-thumb-wrap">
              <a :href="assetUrl(img.url)" target="_blank" rel="noopener" class="image-thumb">
                <img :src="assetUrl(img.path)" :alt="img.name" loading="lazy" />
              </a>
              <button
                class="image-del-overlay"
                title="Delete this image"
                @click="images.deleteImage(selectedDir, img.name)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" />
              </button>
            </div>
            <figcaption class="image-card-body">
              <span class="image-name code-gold" :title="img.name">{{ img.name }}</span>
              <span class="text-dim text-xs">{{ formatSize(img.size) }}</span>
              <button
                class="btn-view btn-sm image-copy-btn"
                title="Copy public URL to clipboard"
                @click="copyUrl(img.url)"
              >
                <font-awesome-icon :icon="['fas', 'copy']" /> Copy URL
              </button>
            </figcaption>
          </figure>
        </div>

        <p v-else-if="!images.loadingImages" class="text-dim" style="padding: 16px 0">
          No images in this category yet. Drop some above to get started.
        </p>
      </template>
    </ManagerView>

    <!-- ── Create / rename category modal ────────────────────────────────────── -->
    <ModalOverlay
      v-if="categoryModal"
      centered
      :aria-label="categoryModal.mode === 'edit' ? 'Rename category' : 'New category'"
      box-style="max-width: 420px"
      @close="closeModal"
    >
      <h3 class="m-0 mb-12">
        <template v-if="categoryModal.mode === 'edit'">
          <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Rename Category
        </template>
        <template v-else><font-awesome-icon :icon="['fad', 'images']" /> New Category</template>
      </h3>
      <FormField label="Category Name" html-for="image-form-name">
        <input
          id="image-form-name"
          v-model="formName"
          placeholder="e.g. Event Banners"
          @keyup.enter="submitCategory"
        />
      </FormField>
      <FormField html-for="image-form-dir">
        <template #label>Directory Name <span class="text-dim">(optional)</span></template>
        <input
          id="image-form-dir"
          v-model="formDir"
          placeholder="auto from name"
          @keyup.enter="submitCategory"
        />
      </FormField>
      <p v-if="formDerivedDir" class="text-dim text-xs mb-12">
        Directory: <span class="code-gold">images/{{ formDerivedDir }}/</span>
      </p>
      <p v-if="categoryModal.mode === 'edit'" class="text-dim text-xs mb-12">
        Renaming the directory moves the folder on disk and changes the public URL of every image
        in it — existing references to the old path will break.
      </p>
      <div class="flex-toolbar flex-end">
        <button class="btn-neutral btn-sm" @click="closeModal">Cancel</button>
        <button
          class="btn-confirm btn-sm"
          :disabled="savingCategory || !formName.trim()"
          @click="submitCategory"
        >
          <LoadingSpinner v-if="savingCategory" label="Saving…" />
          <template v-else-if="categoryModal.mode === 'edit'">
            <font-awesome-icon :icon="['fas', 'check']" /> Save
          </template>
          <template v-else><font-awesome-icon :icon="['fas', 'plus']" /> Create</template>
        </button>
      </div>
    </ModalOverlay>
  </div>
</template>

<style scoped>
/* Drag-and-drop zone. */
.image-dropzone {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 28px;
  margin-bottom: 16px;
  border: 2px dashed var(--panel-raised-bg);
  border-radius: var(--radius);
  color: var(--text-muted);
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}
.image-dropzone:hover {
  border-color: var(--highlight);
}
.image-dropzone.drag-over {
  border-color: var(--highlight);
  background: color-mix(in srgb, var(--highlight) 12%, transparent);
  color: var(--highlight);
}
.image-dropzone.disabled {
  opacity: 0.5;
  pointer-events: none;
}
.image-dropzone .svg-inline--fa {
  font-size: 1.6rem;
}

/* Image grid. */
.image-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 14px;
}
.image-card {
  margin: 0;
  background: var(--panel-raised-bg);
  border-radius: var(--radius);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.image-thumb-wrap {
  position: relative;
}
.image-thumb {
  display: block;
  aspect-ratio: 1 / 1;
  background: var(--panel-bg);
}
.image-thumb img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.image-del-overlay {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: color-mix(in srgb, #000 55%, transparent);
  color: #fff;
  cursor: pointer;
  opacity: 0.85;
  transition: background 0.15s, opacity 0.15s;
}
.image-del-overlay:hover {
  opacity: 1;
  background: var(--danger);
}
.image-card-body {
  padding: 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.image-name {
  font-size: 0.78rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.image-copy-btn {
  margin-top: 4px;
  width: 100%;
  white-space: nowrap;
}
</style>
