<script setup lang="ts">
/**
 * Posts the artist's finished plaque for one request: pick the image, look it over,
 * send it. The server posts it to the request's day on Discord - "Requested by
 * <character>, made by <artist>" - marks the request complete, and keeps no copy.
 * There is no review step; the artist is staff.
 */
import { ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import FormField from '@/components/common/ui/FormField.vue'
import { participantLabel } from '@/lib/participant'
import { useEmaStore } from '@/stores/ema'
import { useUiStore } from '@/stores/ui'
import type { EmaPlaque } from '@/types/api'

const props = defineProps<{ request: EmaPlaque }>()
const emit = defineEmits<{ close: [] }>()

const ema = useEmaStore()
const ui = useUiStore()

/** Discord's default upload limit, which the server holds too. */
const MAX_BYTES = 10 * 1024 * 1024
/** The image types Discord shows inline, which are all the server accepts. */
const ACCEPT = 'image/png,image/jpeg,image/gif,image/webp'

const file = ref<File | null>(null)
const preview = ref('')

function onPick(e: Event): void {
  const input = e.target as HTMLInputElement
  const picked = input.files?.[0] ?? null
  file.value = null
  preview.value = ''
  if (!picked) return
  if (picked.size > MAX_BYTES) {
    ui.notify('That image is too large - Discord takes up to 10 MB', 'error')
    input.value = ''
    return
  }
  file.value = picked
  const reader = new FileReader()
  // A data: URL, not a blob: one - the site's Content-Security-Policy allows the
  // former for images and not the latter.
  reader.onload = () => {
    if (typeof reader.result === 'string') preview.value = reader.result
  }
  reader.readAsDataURL(picked)
}

async function post(): Promise<void> {
  if (file.value && (await ema.postRequest(props.request, file.value))) emit('close')
}
</script>

<template>
  <ModalOverlay aria-label="Post a finished plaque" @close="emit('close')">
    <div class="flex-between mb-12">
      <h3>
        <font-awesome-icon :icon="['fab', 'discord']" />
        Post {{ participantLabel(request.character_name, request.world) }}'s Plaque
      </h3>
      <button type="button" class="btn-neutral btn-sm" @click="emit('close')">Close</button>
    </div>

    <p class="text-sm text-muted m-0">They asked for:</p>
    <p class="ema-request-text mb-12">{{ request.description }}</p>

    <FormField label="Finished Plaque" help="PNG, JPEG, GIF or WebP, up to 10 MB.">
      <input type="file" :accept="ACCEPT" aria-label="Finished plaque image" @change="onPick" />
    </FormField>
    <img v-if="preview" :src="preview" class="ema-review-image mb-12" alt="The finished plaque" />

    <p class="text-sm m-0">
      Posts to {{ request.window_label }}'s Discord channel as requested by
      <strong>{{ participantLabel(request.character_name, request.world) }}</strong> and made by the
      board's artist, then marks the request complete. The image isn't kept on the site.
    </p>

    <FormActions>
      <button type="button" class="btn-neutral" @click="emit('close')">Cancel</button>
      <button
        type="button"
        class="btn-action"
        :disabled="!file || ema.postingRequest"
        @click="post"
      >
        <LoadingSpinner v-if="ema.postingRequest" label="Posting..." />
        <template v-else>
          <font-awesome-icon :icon="['fab', 'discord']" /> Post to Discord
        </template>
      </button>
    </FormActions>
  </ModalOverlay>
</template>
