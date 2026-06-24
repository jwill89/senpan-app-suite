<script setup lang="ts">
/**
 * Admin Carrd Upload tab (Atelier Yao section) — image hosting for external Carrd
 * sites. The admin creates "projects" (folders under <webRoot>/carrd, served at
 * carrd.senpan.cafe/<folder>/…), then uploads images into the project root or
 * into arbitrarily nested sub-directories.
 *
 * Two screens, following the standard manager model:
 *   - list:   a "+ New Project" create form plus a DataTable of projects (title,
 *             folder, sub-folder count, file count, total size) with edit
 *             (rename title + folder) and delete row actions. Clicking a project
 *             opens it.
 *   - detail: the open project (breadcrumb, sub-folders, upload drop zone, media
 *             grid) on its own page with a Back button.
 *
 * Uploading an image whose name already exists overwrites it server-side, so a
 * Carrd site referencing that URL picks up the replacement.
 */
import { computed, onMounted, ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import FormField from '@/components/common/ui/FormField.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import { useCarrdStore, carrdImageUrl, joinCarrdPath, CARRD_BASE_URL } from '@/stores/carrd'
import { useUiStore } from '@/stores/ui'
import type { CarrdProject } from '@/types/api'

const carrd = useCarrdStore()
const ui = useUiStore()

type Screen = 'list' | 'detail'
const screen = ref<Screen>('list')

/** Free-text filter applied to the projects table (title + folder). */
const search = ref('')

/**
 * Create/rename modal state. `null` = closed; otherwise the mode and (for
 * "edit") the folder of the project being renamed. `formTitle`/`formFolder`
 * back the inputs for both modes.
 */
const projectModal = ref<{ mode: 'create' | 'edit'; folder: string } | null>(null)
const formTitle = ref('')
const formFolder = ref('')
const savingProject = ref(false)

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

/** Columns for the projects table (the last is the right-aligned actions). */
const projectColumns: DataColumn[] = [
  { key: 'title', label: 'Project' },
  { key: 'folder', label: 'Folder' },
  { key: 'subfolder_count', label: 'Sub-folders', align: 'center' },
  { key: 'file_count', label: 'Files', align: 'center' },
  { key: 'total_size', label: 'Size', align: 'right' },
  { key: 'actions', label: '', align: 'right' },
]

/** Projects filtered by the search box (matches title or folder). */
const filteredProjects = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return carrd.projects
  return carrd.projects.filter((p) =>
    [p.title, p.folder].some((s) => s.toLowerCase().includes(q)),
  )
})

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

/** Live preview of the folder slug the modal's inputs would produce. */
const formDerivedFolder = computed(() => slugify(formFolder.value || formTitle.value))

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

// ── Navigation ─────────────────────────────────────────────────────────────────
async function openProject(p: CarrdProject): Promise<void> {
  await carrd.openProject(p.folder)
  screen.value = 'detail'
}

function backToList(): void {
  screen.value = 'list'
  carrd.loadProjects() // refresh counts/size after edits in the project
}

// ── Create / rename modal ──────────────────────────────────────────────────────
function openNew(): void {
  formTitle.value = ''
  formFolder.value = ''
  projectModal.value = { mode: 'create', folder: '' }
}

function startEdit(p: CarrdProject): void {
  formTitle.value = p.title
  formFolder.value = p.folder
  projectModal.value = { mode: 'edit', folder: p.folder }
}

function closeModal(): void {
  projectModal.value = null
}

async function submitProject(): Promise<void> {
  if (!projectModal.value || !formTitle.value.trim()) return
  savingProject.value = true
  try {
    if (projectModal.value.mode === 'edit') {
      const ok = await carrd.renameProject(projectModal.value.folder, formTitle.value, formFolder.value)
      if (ok) closeModal()
    } else {
      // The store creates and opens the project (loads its contents) on success.
      const folder = await carrd.createProject(formTitle.value, formFolder.value)
      if (folder) {
        closeModal()
        screen.value = 'detail'
      }
    }
  } finally {
    savingProject.value = false
  }
}

// ── Upload + sub-folders (detail screen) ───────────────────────────────────────
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
    <!-- ── Detail: open project (breadcrumb + folders + upload + media grid) ──── -->
    <AdminPanel v-if="screen === 'detail' && selectedProject">
      <SubPageHeader @back="backToList">
        <font-awesome-icon :icon="['fad', 'folder-open']" /> {{ selectedProject.title }}
      </SubPageHeader>

      <p class="text-dim text-xs mb-12">
        Served from
        <span class="code-gold">{{ CARRD_BASE_URL }}/{{ selectedProject.folder }}/…</span>.
        Allowed types: .jpg, .jpeg, .png, .webp, .gif, .mp3, .mp4. Uploading a file with an
        existing name replaces it.
      </p>

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
              <font-awesome-icon v-if="i === 0" :icon="['fad', 'folder-open']" /> {{ crumb.label }}
            </button>
          </template>
        </nav>
        <button
          class="btn-action btn-sm push-right"
          :disabled="carrd.uploading"
          @click="pickFiles"
        >
          <LoadingSpinner v-if="carrd.uploading" label="Uploading…" />
          <template v-else><font-awesome-icon :icon="['fas', 'upload']" /> Upload Images</template>
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
          class="chip"
          @click="carrd.navigate(joinCarrdPath(carrd.currentPath, dir))"
        >
          <font-awesome-icon :icon="['fad', 'folder']" /> {{ dir }}
          <span
            class="chip-del carrd-dir-del"
            role="button"
            tabindex="0"
            title="Delete this folder"
            @click.stop="carrd.deleteDir(dir)"
            @keyup.enter.stop="carrd.deleteDir(dir)"
          >
            <font-awesome-icon :icon="['fas', 'trash']" />
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
          <button class="btn-confirm btn-sm" @click="submitDir">Add</button>
          <button class="btn-neutral btn-sm" @click="cancelAddDir">Cancel</button>
        </template>
        <button v-else class="chip carrd-dir-add" @click="addingDir = true">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Folder
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
        <font-awesome-icon :icon="['fad', 'cloud-arrow-up']" />
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
              <font-awesome-icon :icon="['fad', 'file-audio']" />
            </div>
            <button
              class="carrd-del-overlay"
              title="Delete this file"
              @click="carrd.deleteImage(img.name)"
            >
              <font-awesome-icon :icon="['fas', 'trash']" />
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
              class="btn-view btn-sm carrd-copy-btn"
              title="Copy public URL to clipboard"
              @click="copyUrl(img.name)"
            >
              <font-awesome-icon :icon="['fas', 'copy']" /> Copy URL
            </button>
          </figcaption>
        </figure>
      </div>

      <p v-else-if="!carrd.loadingImages" class="text-dim" style="padding: 16px 0">
        No images in this folder yet. Drop some above, or create a sub-folder to organize them.
      </p>
    </AdminPanel>

    <!-- ── List: projects table (create/rename via modal) ─────────────────────── -->
    <ManagerView v-else title="Carrd Upload" :icon="['fad', 'images']">
      <template #actions>
        <button class="btn-confirm btn-sm" @click="openNew">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Project
        </button>
      </template>

      <p class="text-dim text-xs mb-12">
        Images are served from
        <span class="code-gold">{{ CARRD_BASE_URL }}/&lt;folder&gt;/…</span>
        for embedding in external Carrd sites. A project can hold files directly or in nested
        sub-folders. Open a project to upload and organize its files.
      </p>

      <LoadingSpinner
        v-if="carrd.loading && carrd.projects.length === 0"
        block
        label="Loading projects…"
      />

      <template v-else-if="carrd.projects.length">
        <div class="manager-toolbar">
          <SearchInput
            v-model="search"
            placeholder="Search projects…"
            aria-label="Search projects by title or folder"
          />
          <span class="text-dim text-xs push-right">
            {{ filteredProjects.length }} of {{ carrd.projects.length }}
          </span>
        </div>

        <DataTable :columns="projectColumns" :rows="filteredProjects" row-key="folder">
          <template #cell-title="{ row }">
            <button class="carrd-open-btn" @click="openProject(row)">
              <font-awesome-icon :icon="['fad', 'folder']" /> {{ row.title }}
            </button>
          </template>
          <template #cell-folder="{ row }">
            <span class="code-gold">/{{ row.folder }}</span>
          </template>
          <template #cell-subfolder_count="{ row }">
            <span class="text-dim">{{ row.subfolder_count }}</span>
          </template>
          <template #cell-file_count="{ row }">
            <span class="text-dim">{{ row.file_count }}</span>
          </template>
          <template #cell-total_size="{ row }">
            <span class="text-dim">{{ formatSize(row.total_size) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="row-actions">
              <button class="btn-confirm btn-sm" title="Rename this project" @click="startEdit(row)">
                <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Edit
              </button>
              <button
                class="btn-danger btn-sm"
                title="Delete this project"
                @click="carrd.deleteProject(row.folder, row.title)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" /> Delete
              </button>
            </div>
          </template>
          <template #empty>
            <EmptyState text="No projects match your search." />
          </template>
        </DataTable>
      </template>

      <EmptyState
        v-else-if="!carrd.loading"
        text="No projects yet. Use “New Project” to start uploading images."
      />
    </ManagerView>

    <!-- ── Create / rename project modal ─────────────────────────────────────── -->
    <ModalOverlay
      v-if="projectModal"
      centered
      :aria-label="projectModal.mode === 'edit' ? 'Rename project' : 'New project'"
      box-style="max-width: 420px"
      @close="closeModal"
    >
      <h3 class="m-0 mb-12">
        <template v-if="projectModal.mode === 'edit'">
          <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Rename Project
        </template>
        <template v-else><font-awesome-icon :icon="['fad', 'images']" /> New Project</template>
      </h3>
      <FormField label="Project Title" html-for="carrd-form-title">
        <input
          id="carrd-form-title"
          v-model="formTitle"
          placeholder="My Carrd Project"
          @keyup.enter="submitProject"
        />
      </FormField>
      <FormField html-for="carrd-form-folder">
        <template #label>
          Folder Name
          <span v-if="projectModal.mode === 'create'" class="text-dim">(optional)</span>
        </template>
        <input
          id="carrd-form-folder"
          v-model="formFolder"
          :placeholder="projectModal.mode === 'create' ? 'auto from title' : 'folder name'"
          @keyup.enter="submitProject"
        />
      </FormField>
      <p v-if="formDerivedFolder" class="text-dim text-xs mb-12">
        URL folder: <span class="code-gold">{{ CARRD_BASE_URL }}/{{ formDerivedFolder }}/</span>
      </p>
      <p v-if="projectModal.mode === 'edit'" class="text-dim text-xs mb-12">
        Renaming the folder changes the public URL of every file in this project — existing Carrd
        embeds pointing at the old folder will break.
      </p>
      <div class="flex-toolbar flex-end">
        <button class="btn-neutral btn-sm" @click="closeModal">Cancel</button>
        <button
          class="btn-confirm btn-sm"
          :disabled="savingProject || !formTitle.trim()"
          @click="submitProject"
        >
          <LoadingSpinner v-if="savingProject" label="Saving…" />
          <template v-else-if="projectModal.mode === 'edit'">
            <font-awesome-icon :icon="['fas', 'check']" /> Save
          </template>
          <template v-else><font-awesome-icon :icon="['fas', 'plus']" /> Create</template>
        </button>
      </div>
    </ModalOverlay>
  </div>
</template>

<style scoped>
/* Project title cell rendered as a link-styled button that opens the project. */
.carrd-open-btn {
  background: none;
  border: 0;
  padding: 0;
  margin: 0;
  font: inherit;
  font-weight: 600;
  color: var(--highlight);
  cursor: pointer;
  text-align: left;
}
.carrd-open-btn:hover {
  text-decoration: underline;
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
/* Sub-folder chips are plain `.chip`s; the delete affordance is `.chip-del`
   (only its left margin is local). The "New Folder" chip is a `.chip` with a
   dashed, muted outline. */
.carrd-dir-del {
  margin-left: 2px;
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
.carrd-dropzone .svg-inline--fa {
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
  background: var(--danger);
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
/* Full-width copy action under each media card. Fill + hover come from the
   `.btn-view` intent; only the full-width layout is component-specific. */
.carrd-copy-btn {
  margin-top: 4px;
  width: 100%;
  white-space: nowrap;
}
</style>
