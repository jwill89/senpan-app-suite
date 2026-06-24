<script setup lang="ts">
/**
 * Image picker — a preview (or empty placeholder), a Remove button, and a grid of
 * previously-uploaded images to choose from. `v-model` is the selected image URL
 * ('' = none). Several pickers can share one `images` list (e.g. an embed's
 * thumbnail and main image), each highlighting its own selection. Uploading now
 * happens centrally on the System → Images page; this is a pure picker.
 *
 * Values may be absolute URLs (announcement embeds) or root-relative web paths
 * (raffle prize images), so previews resolve through `assetUrl` which leaves
 * absolute URLs untouched and prefixes relative paths with "/".
 */
import { assetUrl } from '@/lib/assets'

const model = defineModel<string>({ required: true })

defineProps<{
  /** Previously-uploaded image URLs/paths offered in the selection grid. */
  images: string[]
}>()
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
      <div v-if="images.length" class="img-picker">
        <button
          v-for="img in images"
          :key="img"
          type="button"
          class="img-thumb"
          :class="{ active: model === img }"
          aria-label="Use this image"
          @click="model = img"
        >
          <img :src="assetUrl(img)" alt="" />
        </button>
      </div>
      <p v-else class="text-dim text-sm">No images uploaded yet.</p>
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
</style>
