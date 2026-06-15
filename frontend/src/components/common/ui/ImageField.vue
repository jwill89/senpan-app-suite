<script setup lang="ts">
/**
 * "Upload or reuse an image" field — a preview (or empty placeholder), a file
 * input + Remove button, and a grid of previously-uploaded images to reuse.
 * Shared by the announcement image and book-club event image forms.
 *
 * `v-model` is the selected image URL (set to '' to clear, or to a reuse-grid
 * URL on pick). The actual upload is the parent's job: the file input emits
 * `upload` with the change Event, and the parent's store handler uploads and
 * writes the resulting URL back through `v-model`.
 */
const model = defineModel<string>({ required: true })

defineProps<{
  /** Previously-uploaded image URLs offered in the reuse grid. */
  images: string[]
  /** True while an upload is in flight (disables the file input). */
  uploading?: boolean
  /** Accessible label for the file input (e.g. "Upload event image"). */
  uploadLabel?: string
}>()

const emit = defineEmits<{ upload: [event: Event] }>()
</script>

<template>
  <div class="flex-row items-start">
    <div class="image-field-main">
      <img v-if="model" :src="model" class="image-field-preview" alt="Image preview" />
      <div v-else class="image-field-preview media-empty"><font-awesome-icon :icon="['fad', 'image']" /></div>
      <input
        type="file"
        accept="image/*"
        :aria-label="uploadLabel || 'Upload image'"
        :disabled="uploading"
        @change="emit('upload', $event)"
      />
      <span v-if="uploading" class="text-dim text-sm">Uploading…</span>
      <button v-if="model" class="btn-neutral btn-sm mt-8" @click="model = ''">Remove</button>
    </div>
    <div class="image-field-reuse">
      <label class="field-label">Or reuse an uploaded image</label>
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
          <img :src="img" alt="" />
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
