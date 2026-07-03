<script setup lang="ts">
/**
 * Image picker — a preview (or empty placeholder), a Remove button, and a
 * browser over the central image library (System → Images): pick any category,
 * then click an image in it. Every category is available to every picker;
 * uploading and category management happen on the System → Images page.
 *
 * `v-model` is the selected image reference ('' = none). `valueKey` picks the
 * stored form: 'path' (root-relative, the default) or 'url' (absolute —
 * announcement Discord embeds need absolute URLs). Previews resolve through
 * `assetUrl`, which leaves absolute URLs untouched and prefixes relative paths
 * with "/".
 */
import { computed, onMounted, ref, watch } from 'vue'
import { assetUrl } from '@/lib/assets'
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

const images = useImagesStore()

/** Directory of the category currently being browsed in this picker. */
const selectedDir = ref('')

/** Extracts the category directory embedded in a stored path/URL ('' if none). */
function dirFromValue(value: string): string {
  const match = /(?:^|\/)images\/([a-z0-9_]+)\//.exec(value)
  return match?.[1] ?? ''
}

const entries = computed(() => {
  const list = images.imagesByDir[selectedDir.value] ?? []
  const exts = props.extensions
  if (!exts) return list
  return list.filter((img) => exts.some((ext) => img.name.toLowerCase().endsWith(ext)))
})

function browseDir(dir: string): void {
  selectedDir.value = dir
  if (dir) void images.ensureImages(dir)
}

function onDirChange(e: Event): void {
  browseDir((e.target as HTMLSelectElement).value)
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
        <p v-else class="text-dim text-sm">No images in this category yet.</p>
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
.image-picker-category {
  display: block;
  margin-bottom: 8px;
  max-width: 280px;
}
</style>
