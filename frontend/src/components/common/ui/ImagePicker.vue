<script setup lang="ts">
/**
 * Image picker — a preview (or empty placeholder), a Remove button, and a
 * browser over the central image library (System → Images): pick any category,
 * then click an image in it. Every category is available to every picker, and
 * images are listed alphabetically by file name.
 *
 * Users holding `system-images` also get a compact Upload button beside the
 * category select (and can drop files straight onto the thumbnail grid), so a
 * missing image can be added without leaving the form. Uploads land in the
 * category currently being browsed. Category management still lives on the
 * System → Images page.
 *
 * `v-model` is the selected image reference ('' = none). `valueKey` picks the
 * stored form: 'path' (root-relative, the default) or 'url' (absolute —
 * announcement Discord embeds need absolute URLs). Previews resolve through
 * `assetUrl`, which leaves absolute URLs untouched and prefixes relative paths
 * with "/".
 */
import { computed, onMounted, ref, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { assetUrl } from '@/lib/assets'
import { useAuthStore } from '@/stores/auth'
import { useImagesStore } from '@/stores/images'

const model = defineModel<string>({ required: true })

const props = withDefaults(
  defineProps<{
    /** ImageEntry field stored in the model: root-relative path or absolute URL. */
    valueKey?: 'path' | 'url'
    /** Only offer images with one of these extensions (lowercase, with dot). */
    extensions?: string[]
  }>(),
  { valueKey: 'path', extensions: undefined },
)

const auth = useAuthStore()
const images = useImagesStore()

/** Directory of the category currently being browsed in this picker. */
const selectedDir = ref('')

/** Extracts the category directory embedded in a stored path/URL ('' if none). */
function dirFromValue(value: string): string {
  const match = /(?:^|\/)images\/([a-z0-9_]+)\//.exec(value)
  return match?.[1] ?? ''
}

// Case-insensitive, digit-aware so "img2.png" sorts before "img10.png".
const byName = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

const entries = computed(() => {
  const list = images.imagesByDir[selectedDir.value] ?? []
  const exts = props.extensions
  const filtered = exts
    ? list.filter((img) => exts.some((ext) => img.name.toLowerCase().endsWith(ext)))
    : list
  // Copy before sorting — the store's array must not be reordered in place.
  return [...filtered].sort((a, b) => byName.compare(a.name, b.name))
})

function browseDir(dir: string): void {
  selectedDir.value = dir
  if (dir) void images.ensureImages(dir)
}

function onDirChange(e: Event): void {
  browseDir((e.target as HTMLSelectElement).value)
}

// ── Uploading ───────────────────────────────────────────────────────────────
// Only `system-images` holders may upload; every other editor sees the picker
// exactly as before (the backend enforces the same permission on the endpoint).
const canUpload = computed(() => auth.hasPermission('system-images'))

const fileInput = ref<HTMLInputElement | null>(null)
const dragOver = ref(false)
/**
 * True only while THIS picker's upload runs. The store's `uploading` flag is
 * global, and a form can host several pickers — keying the button off a local
 * flag keeps the progress in the one the user actually clicked.
 */
const busy = ref(false)

/** Narrow the file dialog to the picker's extensions when it has any. */
const accept = computed(() => props.extensions?.join(',') ?? '.jpg,.jpeg,.png,.webp,.gif,.svg')

/** Compact progress text — this sits inside a small button, so keep it short. */
const uploadLabel = computed(() => {
  const p = images.uploadProgress
  if (p < 0) return 'Uploading…'
  if (p < 100) return `${p}%`
  return 'Saving…'
})

function pickFiles(): void {
  fileInput.value?.click()
}

async function upload(files: FileList | File[]): Promise<void> {
  if (!canUpload.value || !selectedDir.value || busy.value) return
  busy.value = true
  try {
    await images.uploadImages(selectedDir.value, files)
  } finally {
    busy.value = false
  }
}

async function onFilesSelected(e: Event): Promise<void> {
  const input = e.target as HTMLInputElement
  if (input.files && input.files.length > 0) await upload(input.files)
  input.value = '' // reset so selecting the same file re-triggers change
}

function onDragOver(): void {
  if (canUpload.value && selectedDir.value) dragOver.value = true
}

function onDrop(e: DragEvent): void {
  dragOver.value = false
  const files = e.dataTransfer?.files
  if (files && files.length > 0) void upload(files)
}

// Follow external model changes (e.g. the form loads another record) into the
// new value's category — but never interrupt the user's own browsing.
watch(model, (value) => {
  const dir = dirFromValue(value)
  if (dir && dir !== selectedDir.value && images.categories.some((c) => c.dir === dir)) {
    browseDir(dir)
  }
})

onMounted(async () => {
  await images.ensureCategories()
  const fromValue = dirFromValue(model.value)
  const dirs = images.sortedCategories.map((c) => c.dir)
  browseDir(dirs.includes(fromValue) ? fromValue : (dirs[0] ?? ''))
})
</script>

<template>
  <div class="flex-row items-start">
    <div class="image-field-main">
      <img v-if="model" :src="assetUrl(model)" class="image-field-preview" alt="Image preview" />
      <div v-else class="image-field-preview media-empty">
        <font-awesome-icon :icon="['fad', 'image']" />
      </div>
      <button v-if="model" class="btn-neutral btn-sm mt-8" @click="model = ''">Remove</button>
    </div>
    <div class="image-field-reuse">
      <label class="field-label">Select an uploaded image</label>
      <p v-if="!images.sortedCategories.length" class="text-dim text-sm">
        No image categories yet — create one on the System → Images page.
      </p>
      <template v-else>
        <div class="image-picker-bar">
          <select
            class="image-picker-category"
            :value="selectedDir"
            aria-label="Image category"
            @change="onDirChange"
          >
            <option v-for="cat in images.sortedCategories" :key="cat.dir" :value="cat.dir">
              {{ cat.name }}
            </option>
          </select>
          <template v-if="canUpload">
            <button
              class="btn-neutral btn-sm image-picker-upload"
              :disabled="busy || !selectedDir"
              title="Upload images into this category"
              aria-label="Upload images into this category"
              @click="pickFiles"
            >
              <LoadingSpinner v-if="busy" :label="uploadLabel" />
              <template v-else><font-awesome-icon :icon="['fas', 'upload']" /> Upload</template>
            </button>
            <input
              ref="fileInput"
              type="file"
              :accept="accept"
              multiple
              hidden
              @change="onFilesSelected"
            />
          </template>
        </div>
        <div
          class="image-picker-drop"
          :class="{ 'is-dragover': dragOver }"
          @dragover.prevent="onDragOver"
          @dragenter.prevent="onDragOver"
          @dragleave.prevent="dragOver = false"
          @drop.prevent="onDrop"
        >
          <div v-if="entries.length" class="img-picker">
            <button
              v-for="img in entries"
              :key="img.name"
              type="button"
              class="img-thumb"
              :class="{ active: model === img[valueKey] }"
              :title="img.name"
              aria-label="Use this image"
              @click="model = img[valueKey]"
            >
              <img :src="assetUrl(img.path)" alt="" />
            </button>
          </div>
          <p v-else-if="images.loadingImages" class="text-dim text-sm">Loading images…</p>
          <p v-else class="text-dim text-sm">
            No images in this category yet.<span v-if="canUpload"> Upload or drop one here.</span>
          </p>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.image-field-main {
  flex: 0 0 150px;
}
.image-field-reuse {
  flex: 1;
  min-width: 160px;
}
.image-field-preview {
  width: 150px;
  height: 85px;
  object-fit: cover;
  border-radius: 6px;
  margin-bottom: 8px;
}
.image-picker-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 8px;
}
.image-picker-category {
  flex: 1 1 auto;
  max-width: 280px;
}
.image-picker-upload {
  flex: 0 0 auto;
  white-space: nowrap;
}
/* The thumbnail grid doubles as the drop target, so drag-to-upload costs no
   extra vertical space. The outline only appears mid-drag. */
.image-picker-drop {
  border-radius: 6px;
  outline: 2px dashed transparent;
  outline-offset: 3px;
  transition: outline-color 0.15s ease;
}
.image-picker-drop.is-dragover {
  outline-color: var(--accent);
}
</style>
