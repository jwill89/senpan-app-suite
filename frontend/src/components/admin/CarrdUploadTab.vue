<script setup lang="ts">
/**
 * Admin Carrd Upload tab (Atelier Yao section) — image hosting for external Carrd
 * sites. The admin creates "projects" (folders under <webRoot>/carrd, served at
 * carrd.senpan.cafe/<folder>/…), then uploads images into the project root or
 * into arbitrarily nested sub-directories via drag-and-drop or a file picker.
 * A breadcrumb navigates the tree; sub-folders can be created and deleted. Each
 * image shows a preview with a "Copy URL" action (the public carrd.senpan.cafe
 * URL) and a delete action. Projects can be deleted (folder + all contents).
 *
 * Uploading an image whose name already exists overwrites it server-side, so a
 * Carrd site referencing that URL picks up the replacement.
 */
import { computed, onMounted, ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import FormField from '@/components/common/ui/FormField.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import { useCarrdStore, carrdImageUrl, joinCarrdPath, CARRD_BASE_URL } from '@/stores/carrd'
import { useUiStore } from '@/stores/ui'

const carrd = useCarrdStore()
const ui = useUiStore()

/** New-project form fields. */
const newTitle = ref('')
const newFolder = ref('')

/** New sub-directory inline-form state. */
const addingDir = ref(false)
const newDirName = ref('')

/** Hidden <input type="file"> used by the Upload button. */
const fileInput = ref<HTMLInputElement | null>(null)
/** True while a file is being dragged over the drop zone. */
const dragOver = ref(false)

/** The currently selected project record (for header display), or null. */
const selectedProject = computed(
  () => carrd.projects.find((p) => p.folder === carrd.selectedFolder) ?? null,
)

/**
 * Breadcrumb trail for the current path within the open project: the project
 * root followed by one crumb per path segment, each carrying the path to
 * navigate to when clicked.
 */
const breadcrumbs = computed(() => {
  const project = selectedProject.value
  if (!project) return []
  const crumbs = [{ label: project.title, path: '' }]
  let acc = ''
  for (const seg of carrd.currentPath.split('/').filter(Boolean)) {
    acc = joinCarrdPath(acc, seg)
    crumbs.push({ label: seg, path: acc })
  }
  return crumbs
})

/** Live preview of the folder name that would be derived from the title. */
const derivedFolder = computed(() => slugify(newFolder.value || newTitle.value))

/** Mirrors the server's folder slug rules for the create-form preview. */
function slugify(s: string): string {
  return s
    .toLowerCase()
    .trim()
    .replace(/[\s_]+/g, '-')
    .replace(/[^a-z0-9-]/g, '')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '')
}

async function createProject(): Promise<void> {
  const folder = await carrd.createProject(newTitle.value, newFolder.value)
  if (folder) {
    newTitle.value = ''
    newFolder.value = ''
  }
}

function pickFiles(): void {
  fileInput.value?.click()
}

async function onFilesSelected(e: Event): Promise<void> {
  const input = e.target as HTMLInputElement
  if (input.files && input.files.length > 0) {
    await carrd.uploadImages(input.files)
  }
  input.value = '' // reset so selecting the same file re-triggers change
}

function onDrop(e: DragEvent): void {
  dragOver.value = false
  const files = e.dataTransfer?.files
  if (files && files.length > 0) {
    carrd.uploadImages(files)
  }
}

/** Submits the inline new-sub-directory form. */
async function submitDir(): Promise<void> {
  if (!newDirName.value.trim()) return
  const ok = await carrd.createDir(newDirName.value)
  if (ok) {
    newDirName.value = ''
    addingDir.value = false
  }
}

function cancelAddDir(): void {
  addingDir.value = false
  newDirName.value = ''
}

/** Copies an image's public URL (including the current path) to the clipboard. */
async function copyUrl(name: string): Promise<void> {
  const folder = carrd.selectedFolder
  if (!folder) return
  const url = carrdImageUrl(folder, carrd.currentPath, name)
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

/** Classifies a file by extension so the grid renders the right media element. */
function fileKind(name: string): 'image' | 'audio' | 'video' {
  const ext = name.slice(name.lastIndexOf('.') + 1).toLowerCase()
  if (ext === 'mp3') return 'audio'
  if (ext === 'mp4') return 'video'
  return 'image'
}

/** Public URL for a file at the current path (shorthand for the grid). */
function fileUrl(name: string): string {
  return carrdImageUrl(carrd.selectedFolder ?? '', carrd.currentPath, name)
}

onMounted(() => carrd.loadProjects())
</script>

<template>
  <div class="tab-body">
    <AdminPanel>
      <div class="flex-toolbar mb-12">
        <h3 style="margin: 0"><i class="fa-duotone fa-images"></i> Carrd Upload</h3>
      </div>

      <p class="text-dim text-xs mb-12">
        Images are served from
        <span class="code-gold">{{ CARRD_BASE_URL }}/&lt;folder&gt;/…</span>
        for embedding in external Carrd sites. A project can hold files directly or in nested
        sub-folders. Allowed types: .jpg, .jpeg, .png, .webp, .gif, .mp3, .mp4. Uploading a file
        with an existing name replaces it.
      </p>

      <!-- Create project -->
      <div class="carrd-create mb-12">
        <FormField label="New Project Title" html-for="carrd-title">
          <input
            id="carrd-title"
            v-model="newTitle"
            placeholder="My Carrd Project"
            @keyup.enter="createProject"
          />
        </FormField>
        <FormField html-for="carrd-folder">
          <template #label>Folder Name <span class="text-dim">(optional)</span></template>
          <input
            id="carrd-folder"
            v-model="newFolder"
            placeholder="auto from title"
            @keyup.enter="createProject"
          />
        </FormField>
        <div class="carrd-create-action">
          <button
            class="btn-primary btn-sm"
            :disabled="carrd.creating || !newTitle.trim()"
            @click="createProject"
          >
            <LoadingSpinner v-if="carrd.creating" label="Creating…" />
            <template v-else><i class="fa-solid fa-plus"></i> Create</template>
          </button>
        </div>
      </div>
      <p v-if="derivedFolder" class="text-dim text-xs mb-12" style="margin-top: -4px">
        URL folder: <span class="code-gold">{{ CARRD_BASE_URL }}/{{ derivedFolder }}/</span>
      </p>

      <LoadingSpinner
        v-if="carrd.loading && carrd.projects.length === 0"
        block
        label="Loading projects…"
      />

      <!-- Project list -->
      <div v-else-if="carrd.projects.length" class="carrd-projects mb-12">
        <button
          v-for="p in carrd.projects"
          :key="p.folder"
          class="carrd-project-chip"
          :class="{ active: carrd.selectedFolder === p.folder }"
          @click="carrd.openProject(p.folder)"
        >
          <span class="carrd-project-title">
            <i class="fa-duotone fa-folder"></i> {{ p.title }}
          </span>
          <span class="carrd-project-meta">
            /{{ p.folder }} · {{ p.image_count }} image{{ p.image_count === 1 ? '' : 's' }}
          </span>
          <span
            class="carrd-project-del"
            role="button"
            tabindex="0"
            title="Delete this project"
            @click.stop="carrd.deleteProject(p.folder, p.title)"
            @keyup.enter.stop="carrd.deleteProject(p.folder, p.title)"
          >
            <i class="fa-solid fa-trash"></i>
          </span>
        </button>
      </div>

      <EmptyState
        v-else-if="!carrd.loading"
        text="No projects yet. Create one above to start uploading images."
      />

      <!-- Selected project: breadcrumb + folders + upload + image grid -->
      <div v-if="selectedProject" class="carrd-project-detail">
        <div class="flex-toolbar mb-12" style="gap: 12px; align-items: center">
          <!-- Breadcrumb: project root → nested path -->
          <nav class="carrd-breadcrumb" aria-label="Folder path">
            <template v-for="(crumb, i) in breadcrumbs" :key="crumb.path">
              <span v-if="i > 0" class="carrd-crumb-sep">/</span>
              <button
                class="carrd-crumb"
                :class="{ current: i === breadcrumbs.length - 1 }"
                :disabled="i === breadcrumbs.length - 1"
                @click="carrd.navigate(crumb.path)"
              >
                <i v-if="i === 0" class="fa-duotone fa-folder-open"></i> {{ crumb.label }}
              </button>
            </template>
          </nav>
          <button
            class="btn-primary btn-sm push-right"
            :disabled="carrd.uploading"
            @click="pickFiles"
          >
            <LoadingSpinner v-if="carrd.uploading" label="Uploading…" />
            <template v-else><i class="fa-solid fa-upload"></i> Upload Images</template>
          </button>
          <input
            ref="fileInput"
            type="file"
            accept=".jpg,.jpeg,.png,.webp,.gif,.mp3,.mp4"
            multiple
            hidden
            @change="onFilesSelected"
          />
        </div>

        <!-- Sub-folders at the current path + create-folder control -->
        <div class="carrd-dirs mb-12">
          <button
            v-for="dir in carrd.dirs"
            :key="dir"
            class="carrd-dir-chip"
            @click="carrd.navigate(joinCarrdPath(carrd.currentPath, dir))"
          >
            <i class="fa-duotone fa-folder"></i> {{ dir }}
            <span
              class="carrd-dir-del"
              role="button"
              tabindex="0"
              title="Delete this folder"
              @click.stop="carrd.deleteDir(dir)"
              @keyup.enter.stop="carrd.deleteDir(dir)"
            >
              <i class="fa-solid fa-trash"></i>
            </span>
          </button>

          <template v-if="addingDir">
            <input
              v-model="newDirName"
              class="carrd-dir-input"
              placeholder="folder name"
              aria-label="New folder name"
              @keyup.enter="submitDir"
              @keyup.esc="cancelAddDir"
            />
            <button class="btn-primary btn-sm" @click="submitDir">Add</button>
            <button class="btn-ghost btn-sm" @click="cancelAddDir">Cancel</button>
          </template>
          <button v-else class="carrd-dir-chip carrd-dir-add" @click="addingDir = true">
            <i class="fa-solid fa-plus"></i> New Folder
          </button>
        </div>

        <!-- Drag-and-drop zone -->
        <div
          class="carrd-dropzone"
          :class="{ 'drag-over': dragOver }"
          @dragover.prevent="dragOver = true"
          @dragenter.prevent="dragOver = true"
          @dragleave.prevent="dragOver = false"
          @drop.prevent="onDrop"
          @click="pickFiles"
        >
          <i class="fa-duotone fa-cloud-arrow-up"></i>
          <span>Drag &amp; drop images, audio or video here, or click to browse</span>
        </div>

        <LoadingSpinner
          v-if="carrd.loadingImages && carrd.images.length === 0"
          block
          label="Loading images…"
        />

        <div v-else-if="carrd.images.length" class="carrd-grid">
          <figure v-for="img in carrd.images" :key="img.name" class="carrd-card">
            <div class="carrd-thumb-wrap">
              <!-- Image: clickable thumbnail -->
              <a
                v-if="fileKind(img.name) === 'image'"
                :href="fileUrl(img.name)"
                target="_blank"
                rel="noopener"
                class="carrd-thumb"
              >
                <img :src="fileUrl(img.name)" :alt="img.name" loading="lazy" />
              </a>
              <!-- Video: inline player -->
              <video
                v-else-if="fileKind(img.name) === 'video'"
                :src="fileUrl(img.name)"
                class="carrd-thumb carrd-media"
                controls
                preload="metadata"
              ></video>
              <!-- Audio: icon placeholder (player lives in the card body) -->
              <div v-else class="carrd-thumb carrd-file-icon">
                <i class="fa-duotone fa-file-audio"></i>
              </div>
              <button
                class="carrd-del-overlay"
                title="Delete this file"
                @click="carrd.deleteImage(img.name)"
              >
                <i class="fa-solid fa-trash"></i>
              </button>
            </div>
            <figcaption class="carrd-card-body">
              <span class="carrd-img-name code-gold" :title="img.name">{{ img.name }}</span>
              <span class="text-dim text-xs">{{ formatSize(img.size) }}</span>
              <audio
                v-if="fileKind(img.name) === 'audio'"
                :src="fileUrl(img.name)"
                class="carrd-audio"
                controls
                preload="none"
              ></audio>
              <button
                class="btn-ghost btn-sm carrd-copy-btn"
                title="Copy public URL to clipboard"
                @click="copyUrl(img.name)"
              >
                <i class="fa-solid fa-copy"></i> Copy URL
              </button>
            </figcaption>
          </figure>
        </div>

        <p v-else-if="!carrd.loadingImages" class="text-dim" style="padding: 16px 0">
          No images in this folder yet. Drop some above, or create a sub-folder to organize them.
        </p>
      </div>
    </AdminPanel>
  </div>
</template>

<style scoped>
/* Create-project form: title + folder inputs side by side, button trailing. */
.carrd-create {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: flex-end;
}
.carrd-create .field {
  flex: 1 1 220px;
  margin: 0;
}
.carrd-create-action {
  flex: 0 0 auto;
}

/* Project chips. */
.carrd-projects {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.carrd-project-chip {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  padding: 10px 14px;
  background: var(--panel-raised-bg);
  border: 1px solid transparent;
  border-radius: var(--radius);
  cursor: pointer;
  text-align: left;
  position: relative;
}
.carrd-project-chip:hover {
  border-color: var(--highlight);
}
.carrd-project-chip.active {
  border-color: var(--highlight);
  box-shadow: inset 0 0 0 1px var(--highlight);
}
.carrd-project-title {
  color: var(--highlight);
  font-weight: 600;
  padding-right: 22px;
}
.carrd-project-meta {
  font-size: 0.75rem;
  color: var(--text-muted);
}
.carrd-project-del {
  position: absolute;
  top: 8px;
  right: 10px;
  color: var(--text-muted);
  cursor: pointer;
}
.carrd-project-del:hover {
  color: var(--danger, #e25555);
}

/* Selected-project detail. */
.carrd-project-detail {
  border-top: 1px solid var(--panel-raised-bg);
  padding-top: 16px;
}

/* Breadcrumb path. */
.carrd-breadcrumb {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px;
  min-width: 0;
}
.carrd-crumb {
  background: none;
  border: 0;
  padding: 2px 4px;
  margin: 0;
  font: inherit;
  color: var(--text-muted);
  cursor: pointer;
  border-radius: 4px;
}
.carrd-crumb:hover:not(:disabled) {
  color: var(--highlight);
}
.carrd-crumb.current {
  color: var(--highlight);
  font-weight: 600;
  cursor: default;
}
.carrd-crumb-sep {
  color: var(--text-muted);
  opacity: 0.6;
}

/* Sub-folder chips + create control. */
.carrd-dirs {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.carrd-dir-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  background: var(--panel-raised-bg);
  border: 1px solid transparent;
  border-radius: var(--radius);
  color: var(--text);
  cursor: pointer;
}
.carrd-dir-chip:hover {
  border-color: var(--highlight);
}
.carrd-dir-del {
  color: var(--text-muted);
  cursor: pointer;
  margin-left: 2px;
}
.carrd-dir-del:hover {
  color: var(--danger, #e25555);
}
.carrd-dir-add {
  color: var(--text-muted);
  border-style: dashed;
  border-color: var(--panel-raised-bg);
}
.carrd-dir-input {
  width: 160px;
}

/* Drag-and-drop zone. */
.carrd-dropzone {
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
.carrd-dropzone:hover {
  border-color: var(--highlight);
}
.carrd-dropzone.drag-over {
  border-color: var(--highlight);
  background: color-mix(in srgb, var(--highlight) 12%, transparent);
  color: var(--highlight);
}
.carrd-dropzone i {
  font-size: 1.6rem;
}

/* Image grid. */
.carrd-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 14px;
}
.carrd-card {
  margin: 0;
  background: var(--panel-raised-bg);
  border-radius: var(--radius);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.carrd-thumb-wrap {
  position: relative;
}
.carrd-thumb {
  display: block;
  aspect-ratio: 1 / 1;
  background: var(--panel-bg);
}
.carrd-thumb img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
/* Video preview fills the square thumb like an image. */
.carrd-media {
  width: 100%;
  object-fit: contain;
}
/* Audio (and other non-previewable) files show a centered file-type glyph. */
.carrd-file-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  font-size: 2.6rem;
}
/* Inline audio player in the card body. */
.carrd-audio {
  width: 100%;
  height: 32px;
  margin-top: 2px;
}
/* Delete control sits in the thumbnail corner so the caption row has space for
   a full-width Copy URL button (which would otherwise wrap in a narrow cell). */
.carrd-del-overlay {
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
.carrd-del-overlay:hover {
  opacity: 1;
  background: var(--danger, #e25555);
}
.carrd-card-body {
  padding: 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.carrd-img-name {
  font-size: 0.78rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* Filled (not transparent) so the button reads clearly against the surface2
   card; the default ghost border is surface2 and would be invisible here. */
.carrd-copy-btn {
  margin-top: 4px;
  width: 100%;
  white-space: nowrap;
  background: var(--panel-bg);
  border-color: var(--panel-bg);
}
.carrd-copy-btn:hover {
  border-color: var(--highlight);
  color: var(--highlight);
}
</style>
