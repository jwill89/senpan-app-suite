/**
 * Carrd store: manages image-hosting "projects" (folders under <webRoot>/carrd)
 * and the images/sub-directories within them (the System → Carrd Upload admin
 * tab).
 *
 * A project has a human-readable title and a URL folder name. Within a project,
 * images can live at the root or in arbitrarily nested sub-directories; the
 * directory tree is the source of truth on disk. Images are served publicly
 * from CARRD_BASE_URL/<folder>/<path>/<image> for embedding in external Carrd
 * sites; the tab lets the admin copy that URL. Uploading an image whose name
 * already exists overwrites it (so a Carrd site picks up the replacement).
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type { CarrdProject, CarrdImage } from '@/types/api'
import { useUiStore } from './ui'

/** Public base URL the carrd directory is served from (no trailing slash). */
export const CARRD_BASE_URL = 'https://carrd.senpan.cafe'

/** Joins a folder, optional subpath, and name into a public carrd URL. */
export function carrdImageUrl(folder: string, path: string, name: string): string {
  const segs = [folder, ...path.split('/').filter(Boolean), name]
  return `${CARRD_BASE_URL}/${segs.map(encodeURIComponent).join('/')}`
}

/** Appends a segment to a relative path ("" stays clean, no leading slash). */
export function joinCarrdPath(path: string, segment: string): string {
  return path ? `${path}/${segment}` : segment
}

export const useCarrdStore = defineStore('carrd', () => {
  const ui = useUiStore()

  const projects = ref<CarrdProject[]>([])
  /** Folder of the project currently being viewed, or null. */
  const selectedFolder = ref<string | null>(null)
  /** Relative subpath being viewed within the project ("" = project root). */
  const currentPath = ref('')
  /** Immediate sub-directories at the current path. */
  const dirs = ref<string[]>([])
  /** Images at the current path. */
  const images = ref<CarrdImage[]>([])

  /** True while the project list is loading. */
  const loading = ref(false)
  /** True while the current directory's contents are loading. */
  const loadingImages = ref(false)
  /** True while an upload is in flight (drives the upload button). */
  const uploading = ref(false)
  /** True while a project is being created (drives the create button). */
  const creating = ref(false)

  async function loadProjects(): Promise<void> {
    loading.value = true
    try {
      const data = await endpoints.carrd.projects()
      projects.value = data.projects || []
      // Drop the selection if its project no longer exists.
      if (selectedFolder.value && !projects.value.some((p) => p.folder === selectedFolder.value)) {
        selectedFolder.value = null
        currentPath.value = ''
        dirs.value = []
        images.value = []
      }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      loading.value = false
    }
  }

  /** Loads the contents (sub-dirs + images) of the current folder/path. */
  async function loadContents(): Promise<void> {
    if (!selectedFolder.value) return
    loadingImages.value = true
    try {
      const data = await endpoints.carrd.images(selectedFolder.value, currentPath.value)
      dirs.value = data.dirs || []
      images.value = data.images || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      dirs.value = []
      images.value = []
    } finally {
      loadingImages.value = false
    }
  }

  /** Opens a project at its root. */
  async function openProject(folder: string): Promise<void> {
    selectedFolder.value = folder
    currentPath.value = ''
    await loadContents()
  }

  /** Navigates to a path within the open project (loads its contents). */
  async function navigate(path: string): Promise<void> {
    currentPath.value = path
    await loadContents()
  }

  /** Creates a project; returns the new folder name on success, else null. */
  async function createProject(title: string, folder = ''): Promise<string | null> {
    const trimmed = title.trim()
    if (!trimmed) {
      ui.notify('Project title is required', 'error')
      return null
    }
    creating.value = true
    try {
      const res = await endpoints.carrd.createProject(trimmed, folder.trim())
      ui.notify(`Created project “${res.project.title}”`, 'success')
      await loadProjects()
      await openProject(res.project.folder)
      return res.project.folder
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return null
    } finally {
      creating.value = false
    }
  }

  /**
   * Renames a project's title and (optionally) its URL folder. When the open
   * project's folder changes, follow it so the detail view stays on the same
   * project. Returns true on success.
   */
  async function renameProject(folder: string, title: string, newFolder: string): Promise<boolean> {
    const trimmed = title.trim()
    if (!trimmed) {
      ui.notify('Project title is required', 'error')
      return false
    }
    try {
      const res = await endpoints.carrd.renameProject(folder, trimmed, newFolder.trim())
      ui.notify(`Renamed project to “${res.project.title}”`, 'success')
      // If the open project's folder changed on disk, re-point the selection.
      if (selectedFolder.value === folder && res.project.folder !== folder) {
        selectedFolder.value = res.project.folder
      }
      await loadProjects()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    }
  }

  async function deleteProject(folder: string, title: string): Promise<void> {
    if (
      !(await ui.confirm(
        `Delete project “${title}” and all of its images? This cannot be undone.`,
        { title: 'Delete project', confirmText: 'Delete' },
      ))
    )
      return
    try {
      await endpoints.carrd.deleteProject(folder)
      ui.notify('Project deleted', 'info')
      if (selectedFolder.value === folder) {
        selectedFolder.value = null
        currentPath.value = ''
        dirs.value = []
        images.value = []
      }
      await loadProjects()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Creates a sub-directory at the current path; returns true on success. */
  async function createDir(name: string): Promise<boolean> {
    if (!selectedFolder.value) return false
    const trimmed = name.trim()
    if (!trimmed) {
      ui.notify('Folder name is required', 'error')
      return false
    }
    try {
      await endpoints.carrd.createDir(selectedFolder.value, currentPath.value, trimmed)
      ui.notify('Folder created', 'success')
      await loadContents()
      await loadProjects()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    }
  }

  /** Deletes a sub-directory (and its contents) under the current path. */
  async function deleteDir(name: string): Promise<void> {
    if (!selectedFolder.value) return
    if (
      !(await ui.confirm(
        `Delete folder “${name}” and everything inside it? This cannot be undone.`,
        { title: 'Delete folder', confirmText: 'Delete' },
      ))
    )
      return
    try {
      await endpoints.carrd.deleteDir(selectedFolder.value, joinCarrdPath(currentPath.value, name))
      ui.notify('Folder deleted', 'info')
      await loadContents()
      await loadProjects()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Uploads files into the current folder/path, then refreshes. */
  async function uploadImages(files: FileList | File[]): Promise<void> {
    if (!selectedFolder.value) return
    const list = Array.from(files)
    if (list.length === 0) return
    const form = new FormData()
    form.append('folder', selectedFolder.value)
    form.append('path', currentPath.value)
    for (const f of list) form.append('files', f)

    uploading.value = true
    try {
      const res = await endpoints.carrd.upload(form)
      const ok = res.uploaded?.length ?? 0
      const skipped = res.skipped ?? []
      if (ok > 0) {
        ui.notify(`Uploaded ${ok} image${ok === 1 ? '' : 's'}`, 'success')
      }
      for (const s of skipped) {
        ui.notify(`${s.name}: ${s.reason}`, 'error')
      }
      if (ok === 0 && skipped.length === 0) {
        ui.notify('No images were uploaded', 'info')
      }
      await loadContents()
      await loadProjects() // refresh image counts
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      uploading.value = false
    }
  }

  async function deleteImage(name: string): Promise<void> {
    if (!selectedFolder.value) return
    if (
      !(await ui.confirm(`Delete image “${name}”? This cannot be undone.`, {
        title: 'Delete image',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.carrd.deleteImage(selectedFolder.value, currentPath.value, name)
      ui.notify('Image deleted', 'info')
      await loadContents()
      await loadProjects() // refresh image counts
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  return {
    projects,
    selectedFolder,
    currentPath,
    dirs,
    images,
    loading,
    loadingImages,
    uploading,
    creating,
    loadProjects,
    loadContents,
    openProject,
    navigate,
    createProject,
    renameProject,
    deleteProject,
    createDir,
    deleteDir,
    uploadImages,
    deleteImage,
  }
})
