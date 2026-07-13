<script setup lang="ts">
/**
 * Admin Images tab (System section) — central image hosting. The admin picks a
 * category (which maps to a subdirectory of <webRoot>/images) and uploads any
 * number of images at once (drag-and-drop or click to browse), browses the
 * images in that category, and deletes them.
 *
 * Every category is admin-managed (a display name + a directory): all of them
 * can be created, renamed, and deleted. The shared ImagePicker used by the
 * feature editors (announcements, raffles, garapons, …) browses all of them.
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
import { slugify, formatSize } from '@/lib/format'
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
const currentImages = computed(() =>
  selectedDir.value in images.imagesByDir ? images.imagesByDir[selectedDir.value] : [],
)

// Upload progress. `uploadProgress` is 0–100 while bytes transfer, then sits at
// 100 while the server saves the files; -1 when idle.
const uploadLabel = computed(() => {
  const p = images.uploadProgress
  if (p < 0) return 'Uploading…'
  if (p < 100) return `Uploading ${p}%`
  return 'Processing…'
})
const uploadBarWidth = computed(() => `${images.uploadProgress < 0 ? 0 : images.uploadProgress}%`)

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
  if (dir) void images.loadImages(dir)
})

// ── Category management modal ────────────────────────────────────────────────
const categoryModal = ref<{ mode: 'create' | 'edit'; dir: string } | null>(null)
const formName = ref('')
const formDir = ref('')
const savingCategory = ref(false)

// Directory slugs use underscores (spaces + hyphens fold to `_`).
const formDerivedDir = computed(() => slugify(formDir.value || formName.value, '_'))

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
        ? await images.saveCategory(
            'rename',
            formName.value,
            categoryModal.value.dir,
            formDir.value,
          )
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
    void images.uploadImages(selectedDir.value, files)
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

onMounted(() => images.loadCategories())
</script>

<template>
  <div class="tab-body">
    <!-- ── Manage categories ─────────────────────────────────────────────────── -->
    <AdminPanel v-if="screen === 'categories'">
      <SubPageHeader
        title="Image Categories"
        :icon="['fad', 'folder-open']"
        @back="screen = 'browse'"
      />
      <p class="text-dim text-xs mb-12">
        Each category maps to a subdirectory of <span class="code-gold">images/</span>. Deleting a
        category removes its folder and all images in it — anything still referencing those images
        (announcements, raffles, …) will lose them.
      </p>
      <div class="flex-toolbar flex-end mb-16">
        <button class="btn-confirm btn-sm" @click="openNewCategory">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Category
        </button>
      </div>

      <DataTable :columns="categoryColumns" :rows="images.sortedCategories" row-key="dir">
        <template #cell-name="{ row }">
          <font-awesome-icon :icon="['fad', 'folder']" /> {{ row.name }}
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
              title="Rename this category"
              @click="startEditCategory(row)"
            >
              <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Edit
            </button>
            <button
              class="btn-danger btn-sm"
              title="Delete this category"
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
        Pick a category, then drag &amp; drop images (or click to browse) to upload them. Allowed
        types: .jpg, .jpeg, .png, .webp, .gif, .svg. Uploading a file with an existing name replaces
        it.
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
            <LoadingSpinner v-if="images.uploading" :label="uploadLabel" />
            <template v-else
              ><font-awesome-icon :icon="['fas', 'upload']" /> Upload Images</template
            >
          </button>
          <input
            ref="fileInput"
            type="file"
            accept=".jpg,.jpeg,.png,.webp,.gif,.svg"
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

        <!-- Live upload progress (large uploads can take a while over a slow link). -->
        <div
          v-if="images.uploading"
          class="upload-progress"
          role="progressbar"
          aria-label="Upload progress"
          :aria-valuenow="images.uploadProgress < 0 ? undefined : images.uploadProgress"
          aria-valuemin="0"
          aria-valuemax="100"
        >
          <div class="upload-progress__track">
            <div
              class="upload-progress__bar"
              :class="{ 'is-processing': images.uploadProgress >= 100 }"
              :style="{ width: uploadBarWidth }"
            ></div>
          </div>
          <span class="upload-progress__label">{{ uploadLabel }}</span>
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
        Renaming the directory moves the folder on disk and changes the public URL of every image in
        it — existing references to the old path will break.
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
  transition:
    border-color 0.15s,
    background 0.15s;
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

/* Upload progress bar (shown while an upload is in flight — large uploads over a
   slow link can take a while, so a real percentage beats an idle spinner). */
.upload-progress {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
}
.upload-progress__track {
  flex: 1;
  height: 8px;
  background: var(--panel-raised-bg);
  border-radius: 999px;
  overflow: hidden;
}
.upload-progress__bar {
  height: 100%;
  background: var(--highlight);
  border-radius: inherit;
  transition: width 0.2s ease;
}
/* Bytes are all sent; the bar is full while the server saves the files. */
.upload-progress__bar.is-processing {
  animation: upload-pulse 1s ease-in-out infinite;
}
.upload-progress__label {
  min-width: 92px;
  font-size: 0.8rem;
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
  text-align: right;
}
@keyframes upload-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.55;
  }
}
@media (prefers-reduced-motion: reduce) {
  .upload-progress__bar {
    transition: none;
  }
  .upload-progress__bar.is-processing {
    animation: none;
  }
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
  transition:
    background 0.15s,
    opacity 0.15s;
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
