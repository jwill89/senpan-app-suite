<script setup lang="ts">
/**
 * Edit Font modal (Atelier → Font Upload). Everything about ONE font that
 * doesn't belong in the slim table lives here:
 *
 *   - CSS name — the font-family used by the kit, the app, and the picker
 *     (blank = the font's base name).
 *   - Served version — which variant type external sites and the app receive
 *     (Auto = WOFF2 when available).
 *   - Allowed sites — THIS font's external-origin allowlist (the app itself is
 *     always allowed).
 *   - Files — every variant with its size; uploaded files can be renamed or
 *     deleted here (the converted WOFF2 copy is managed automatically).
 *
 * The metadata fields are drafts applied together with Save; file rename and
 * delete apply immediately (they're their own endpoints). The modal tracks the
 * font by base name, so it stays current across list reloads and closes itself
 * if the font disappears (deleted, or its last file renamed to a new base).
 */
import { computed, ref, watch } from 'vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import FormField from '@/components/common/ui/FormField.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useFontsStore } from '@/stores/fonts'
import { useUiStore } from '@/stores/ui'
import type { FontVariant } from '@/types/api'

const props = defineProps<{
  /** Base name of the font being edited (group identity). */
  base: string
}>()
const emit = defineEmits<{ close: [] }>()

const fonts = useFontsStore()
const ui = useUiStore()

/** The live font row (tracks list reloads); closing when it disappears. */
const font = computed(() => fonts.fonts.find((f) => f.base === props.base) ?? null)
watch(font, (f) => {
  if (!f) emit('close')
})

// ── Metadata drafts (applied together with Save) ─────────────────────────────

/** Custom CSS name draft ("" = base-name default). */
const familyDraft = ref('')
/** Served variant type draft ("" = auto). */
const serveDraft = ref('')
/** Per-font allowed-site origins draft. */
const originsDraft = ref<string[]>([])
/** New origin input value. */
const newOrigin = ref('')

// (Re)initialize the drafts when the modal opens or the font reloads under it.
watch(
  font,
  (f, old) => {
    if (!f) return
    // Only reset when the target font changes (not on every list refresh, or
    // typing would be clobbered by unrelated reloads) — except the first run.
    if (old && old.base === f.base) return
    familyDraft.value = f.family === f.base ? '' : f.family
    serveDraft.value = f.serve
    originsDraft.value = f.origins.slice()
    newOrigin.value = ''
  },
  { immediate: true },
)

const dirty = computed(() => {
  const f = font.value
  if (!f) return false
  const savedFamily = f.family === f.base ? '' : f.family
  return (
    familyDraft.value.trim() !== savedFamily ||
    serveDraft.value !== f.serve ||
    JSON.stringify(originsDraft.value) !== JSON.stringify(f.origins)
  )
})

/** Validates + normalizes the input to a bare origin and adds it to the draft. */
function addOrigin(): void {
  const raw = newOrigin.value.trim().replace(/\/+$/, '')
  if (!raw) return
  let origin = ''
  try {
    const u = new URL(raw.includes('://') ? raw : `https://${raw}`)
    if (
      (u.protocol === 'https:' || u.protocol === 'http:') &&
      u.pathname === '/' &&
      !u.search &&
      !u.hash &&
      !u.username
    ) {
      origin = u.origin.toLowerCase()
    }
  } catch {
    /* handled below */
  }
  if (!origin) {
    ui.notify('Enter a site origin like https://mysite.carrd.co (no path)', 'error')
    return
  }
  if (!originsDraft.value.includes(origin)) originsDraft.value.push(origin)
  newOrigin.value = ''
}

function removeOrigin(origin: string): void {
  originsDraft.value = originsDraft.value.filter((o) => o !== origin)
}

async function save(): Promise<void> {
  if (!font.value) return
  const ok = await fonts.updateFamily(font.value.base, {
    family: familyDraft.value.trim(),
    serve: serveDraft.value,
    origins: originsDraft.value,
  })
  if (ok) emit('close')
}

// ── Files (apply immediately) ────────────────────────────────────────────────

/** Name of the file currently being renamed inline, or null. */
const renamingFile = ref<string | null>(null)
/** Working value of the inline file-name input. */
const renameValue = ref('')

function startRename(name: string): void {
  renamingFile.value = name
  renameValue.value = name
}

function cancelRename(): void {
  renamingFile.value = null
  renameValue.value = ''
}

async function commitRename(name: string): Promise<void> {
  if (await fonts.renameFile(name, renameValue.value)) cancelRename()
}

/** Label for a variant row/option, e.g. "WOFF2 (converted)". */
function variantLabel(v: FontVariant): string {
  return v.converted ? `${v.type} (converted)` : v.type
}

/** Human-readable file size. */
function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
</script>

<template>
  <ModalOverlay
    v-if="font"
    :aria-label="`Edit font ${font.family}`"
    box-style="max-width: 640px; width: min(640px, 92vw)"
    @close="emit('close')"
  >
    <h3 class="m-0 mb-12">
      <font-awesome-icon :icon="['fad', 'font']" /> Edit Font —
      <span class="code-gold">{{ font.family }}</span>
    </h3>

    <FormField
      label="CSS name"
      html-for="font-edit-family"
      :help="`The font-family name used on external sites and in the app. Blank uses the default: '${font.base}'.`"
    >
      <input
        id="font-edit-family"
        v-model="familyDraft"
        :placeholder="font.base"
        aria-label="CSS font-family name"
      />
    </FormField>

    <FormField
      label="Served version"
      html-for="font-edit-serve"
      help="Which file external sites and the app receive. Auto serves the WOFF2 (smallest) when available."
    >
      <select id="font-edit-serve" v-model="serveDraft" aria-label="Served font version">
        <option value="">Auto (WOFF2 preferred)</option>
        <option v-for="v in font.variants" :key="v.name + v.type" :value="v.type">
          {{ variantLabel(v) }} — {{ formatSize(v.size) }}
        </option>
      </select>
    </FormField>

    <FormField
      label="Allowed sites"
      html-for="font-edit-origin"
      help="External sites allowed to use THIS font (origin only, no path). This app is always allowed — no need to list it."
    >
      <div class="font-modal-row">
        <input
          id="font-edit-origin"
          v-model="newOrigin"
          placeholder="https://mysite.carrd.co"
          @keyup.enter="addOrigin"
        />
        <button class="btn-action btn-sm" @click="addOrigin">
          <font-awesome-icon :icon="['fas', 'plus']" /> Add
        </button>
      </div>
    </FormField>
    <ul v-if="originsDraft.length" class="font-modal-origins">
      <li v-for="o in originsDraft" :key="o">
        <span class="code-gold">{{ o }}</span>
        <button class="btn-danger btn-sm" :title="`Remove ${o}`" @click="removeOrigin(o)">
          <font-awesome-icon :icon="['fas', 'trash']" /> Remove
        </button>
      </li>
    </ul>
    <p v-else class="text-dim text-xs" style="margin: 0 0 12px">
      No external sites yet — this font can only be used by the app itself.
    </p>

    <h4 class="font-modal-files-heading">Files</h4>
    <ul class="font-modal-files">
      <li v-for="v in font.variants" :key="v.name + v.type">
        <template v-if="renamingFile === v.name && !v.converted">
          <input
            v-model="renameValue"
            class="font-modal-rename"
            aria-label="New file name"
            @keyup.enter="commitRename(v.name)"
            @keyup.esc="cancelRename"
          />
          <span class="font-modal-file-actions">
            <button class="btn-confirm btn-sm" @click="commitRename(v.name)">Save</button>
            <button class="btn-neutral btn-sm" @click="cancelRename">Cancel</button>
          </span>
        </template>
        <template v-else>
          <span class="font-modal-file">
            <span class="code-gold">{{ v.name }}</span>
            <span class="text-dim text-xs">
              {{ variantLabel(v) }} · {{ formatSize(v.size) }}
              <template v-if="v.modified"> · {{ new Date(v.modified).toLocaleString() }}</template>
            </span>
          </span>
          <span v-if="!v.converted" class="font-modal-file-actions">
            <button
              class="btn-confirm btn-sm"
              title="Rename this file"
              @click="startRename(v.name)"
            >
              <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Rename
            </button>
            <button
              class="btn-danger btn-sm"
              title="Delete this file"
              @click="fonts.deleteFile(v.name)"
            >
              <font-awesome-icon :icon="['fas', 'trash']" /> Delete
            </button>
          </span>
          <span v-else class="text-dim text-xs">managed automatically</span>
        </template>
      </li>
    </ul>

    <div class="font-modal-actions">
      <button class="btn-confirm" :disabled="!dirty || fonts.saving" @click="save">
        <LoadingSpinner v-if="fonts.saving" label="Saving…" />
        <template v-else>Save</template>
      </button>
      <button class="btn-neutral" @click="emit('close')">Close</button>
    </div>
  </ModalOverlay>
</template>

<style scoped>
.font-modal-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.font-modal-row input {
  flex: 1;
  min-width: 0;
}
.font-modal-origins {
  list-style: none;
  margin: 0 0 12px;
  padding: 0;
}
.font-modal-origins li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 5px 0;
  border-bottom: 1px solid color-mix(in srgb, var(--highlight) 12%, transparent);
}
.font-modal-origins li:last-child {
  border-bottom: none;
}
.font-modal-files-heading {
  margin: 16px 0 6px;
}
.font-modal-files {
  list-style: none;
  margin: 0 0 16px;
  padding: 0;
}
.font-modal-files li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px solid color-mix(in srgb, var(--highlight) 12%, transparent);
}
.font-modal-files li:last-child {
  border-bottom: none;
}
.font-modal-file {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  overflow-wrap: anywhere;
}
.font-modal-file-actions {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}
.font-modal-rename {
  flex: 1;
  min-width: 0;
}
.font-modal-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  margin-top: 4px;
}
</style>
