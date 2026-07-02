/**
 * Announcement management store (admin).
 *
 * Covers Announcement Types (a named Discord webhook destination) and the
 * Announcements that post to them — CRUD, image upload + reuse, manual "send
 * now", "skip next occurrence", and a client-side search filter. Mirrors the
 * structure of the book-club store.
 *
 * One IANA `timezone` anchors every time on an announcement: the form holds
 * wall-clock values (start/end window, one-time post, and the recurring
 * time/weekday selections) which are sent as-is; the backend resolves them in the
 * timezone to the absolute UTC instants it stores, so all times survive DST.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { detectTimezone } from '@/lib/constants'
import { nextUid } from '@/lib/uid'
import {
  useImagesStore,
  IMAGE_DIR_ANNOUNCEMENTS_MAIN,
  IMAGE_DIR_ANNOUNCEMENTS_THUMB,
} from './images'
import type {
  Announcement,
  AnnouncementForm,
  AnnouncementType,
  AnnouncementTypeForm,
  AnnouncementRole,
  AnnouncementRoleForm,
  ScheduleKind,
} from '@/types/api'
import { useUiStore } from './ui'

/** A blank announcement-type form (used for "Add type" and reset after save). */
function emptyTypeForm(): AnnouncementTypeForm {
  return { id: 0, name: '', webhook_url: '' }
}

/** A blank taggable-role form (used for "Add role" and reset after save). */
function emptyRoleForm(): AnnouncementRoleForm {
  return { id: 0, name: '', role_id: '' }
}

/** A blank announcement form with sensible schedule defaults. */
function emptyForm(): AnnouncementForm {
  return {
    id: 0,
    type_id: 0,
    title: '',
    details: '',
    image: '',
    thumbnail: '',
    color: '#ff3131',
    location: '',
    start_local: '',
    end_local: '',
    start_format: 'F',
    end_format: 't',
    dynamic_dates: false,
    schedule_kind: '',
    timezone: detectTimezone(),
    once_local: '',
    time_local: '19:00',
    weekdays: [],
    week_of_month: 3,
    buttons: [],
    mention: '',
  }
}

const pad = (n: number): string => String(n).padStart(2, '0')

/** Parse an "HH:mm" wall-clock string into minutes-of-day (0–1439). */
function timeToMinutes(time: string): number {
  const [hh, mm] = time.split(':').map((n) => Number(n) || 0)
  return hh * 60 + mm
}

/** Format minutes-of-day back into an "HH:mm" wall-clock string. */
function minutesToTime(minutes: number): string {
  return `${pad(Math.floor(minutes / 60))}:${pad(minutes % 60)}`
}

/** Parse a CSV of weekday numbers into a sorted array. */
function parseWeekdays(csv: string): number[] {
  return csv
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s !== '')
    .map((s) => Number(s))
    .sort((a, b) => a - b)
}

export const useAnnouncementsStore = defineStore('announcements', () => {
  const ui = useUiStore()
  const imagesStore = useImagesStore()

  // Data.
  const types = ref<AnnouncementType[]>([])
  /** Types sorted alphabetically by name (for the form dropdown + types list). */
  const sortedTypes = computed(() => [...types.value].sort((a, b) => a.name.localeCompare(b.name)))
  const roles = ref<AnnouncementRole[]>([])
  const announcements = ref<Announcement[]>([])
  /** Reusable image URLs for the Main / Thumbnail pickers, sourced from the
   *  central Images page categories (announcements_main / announcements_thumb). */
  const mainImages = computed(() =>
    (imagesStore.imagesByDir[IMAGE_DIR_ANNOUNCEMENTS_MAIN] ?? []).map((i) => i.url),
  )
  const thumbImages = computed(() =>
    (imagesStore.imagesByDir[IMAGE_DIR_ANNOUNCEMENTS_THUMB] ?? []).map((i) => i.url),
  )

  // Forms.
  const typeForm = ref<AnnouncementTypeForm>(emptyTypeForm())
  const roleForm = ref<AnnouncementRoleForm>(emptyRoleForm())
  const form = ref<AnnouncementForm>(emptyForm())

  // Search + category (type) filters (client-side, like other admin tabs).
  const search = ref('')
  const typeFilter = ref<number>(0) // 0 = all categories
  const filteredAnnouncements = computed(() => {
    const q = search.value.trim().toLowerCase()
    const tf = typeFilter.value
    return announcements.value.filter((a) => {
      if (tf && a.type_id !== tf) return false
      if (!q) return true
      return [a.title, a.type_name ?? '', a.details].some((s) => s.toLowerCase().includes(q))
    })
  })

  // In-flight flags.
  const loading = ref(false)
  const savingType = ref(false)
  const savingRole = ref(false)
  const saving = ref(false)
  const sendingId = ref<number | null>(null)
  const skippingId = ref<number | null>(null)

  // ── Loads ──────────────────────────────────────────────────────────────────

  async function load(): Promise<void> {
    loading.value = true
    try {
      const [t, r, a] = await Promise.all([
        endpoints.announcements.types(),
        endpoints.announcements.roles(),
        endpoints.announcements.list(),
      ])
      types.value = t.types
      roles.value = r.roles
      announcements.value = a.announcements
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      loading.value = false
    }
    loadImages()
  }

  /** Loads the Main + Thumbnail reusable-image libraries from the Images page. */
  function loadImages(): void {
    void imagesStore.loadImages(IMAGE_DIR_ANNOUNCEMENTS_MAIN)
    void imagesStore.loadImages(IMAGE_DIR_ANNOUNCEMENTS_THUMB)
  }

  async function loadTypes(): Promise<void> {
    try {
      const data = await endpoints.announcements.types()
      types.value = data.types
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function loadRoles(): Promise<void> {
    try {
      const data = await endpoints.announcements.roles()
      roles.value = data.roles
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function loadAnnouncements(): Promise<void> {
    try {
      const data = await endpoints.announcements.list()
      announcements.value = data.announcements
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /**
   * Persists a drag-and-drop reorder of the announcement list. The list array is
   * mutated optimistically by the drag interaction; this saves the new order
   * (top-first ids) and reverts to the server order if the save fails.
   */
  async function reorder(orderedIds: number[]): Promise<void> {
    try {
      await endpoints.announcements.reorder(orderedIds)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      await loadAnnouncements() // revert to the persisted order
    }
  }

  // ── Types ──────────────────────────────────────────────────────────────────

  function resetTypeForm(): void {
    typeForm.value = emptyTypeForm()
  }

  function editType(t: AnnouncementType): void {
    typeForm.value = { id: t.id, name: t.name, webhook_url: t.webhook_url }
  }

  async function saveType(): Promise<boolean> {
    const f = typeForm.value
    if (!f.name.trim()) {
      ui.notify('Type name is required', 'error')
      return false
    }
    savingType.value = true
    try {
      const payload = { ...f, name: f.name.trim() }
      if (f.id) {
        await endpoints.announcements.updateType(f.id, payload)
      } else {
        await endpoints.announcements.createType(payload)
      }
      ui.notify(f.id ? 'Type updated' : 'Type added', 'success')
      resetTypeForm()
      await loadTypes()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingType.value = false
    }
  }

  async function deleteType(t: AnnouncementType): Promise<void> {
    if (
      !(await ui.confirm(`Delete the "${t.name}" type?`, {
        title: 'Delete announcement type',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.announcements.deleteType(t.id)
      ui.notify('Type deleted', 'info')
      if (typeForm.value.id === t.id) resetTypeForm()
      await loadTypes()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // ── Roles (taggable Discord roles) ───────────────────────────────────────────

  function resetRoleForm(): void {
    roleForm.value = emptyRoleForm()
  }

  function editRole(r: AnnouncementRole): void {
    roleForm.value = { id: r.id, name: r.name, role_id: r.role_id }
  }

  async function saveRole(): Promise<boolean> {
    const f = roleForm.value
    if (!f.name.trim()) {
      ui.notify('Role name is required', 'error')
      return false
    }
    if (!/^\d+$/.test(f.role_id.trim())) {
      ui.notify('Role ID must be a Discord role ID (digits only)', 'error')
      return false
    }
    savingRole.value = true
    try {
      const payload = { ...f, name: f.name.trim(), role_id: f.role_id.trim() }
      if (f.id) {
        await endpoints.announcements.updateRole(f.id, payload)
      } else {
        await endpoints.announcements.createRole(payload)
      }
      ui.notify(f.id ? 'Role updated' : 'Role added', 'success')
      resetRoleForm()
      await loadRoles()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingRole.value = false
    }
  }

  async function deleteRole(r: AnnouncementRole): Promise<void> {
    if (
      !(await ui.confirm(`Delete the "${r.name}" role?`, {
        title: 'Delete taggable role',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.announcements.deleteRole(r.id)
      ui.notify('Role deleted', 'info')
      if (roleForm.value.id === r.id) resetRoleForm()
      await loadRoles()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // ── Announcements ──────────────────────────────────────────────────────────

  function resetForm(): void {
    form.value = emptyForm()
  }

  function editAnnouncement(a: Announcement): void {
    const f = emptyForm()
    f.id = a.id
    f.type_id = a.type_id
    f.title = a.title
    f.details = a.details
    f.image = a.image
    f.thumbnail = a.thumbnail || ''
    f.color = a.color || '#ff3131'
    f.location = a.location || ''
    f.mention = a.mention || ''
    f.buttons = a.buttons.map((b) => ({ ...b, _uid: nextUid() }))
    f.start_local = a.start_local
    f.end_local = a.end_local
    f.start_format = (a.start_format || 'F') as AnnouncementForm['start_format']
    f.end_format = (a.end_format || 't') as AnnouncementForm['end_format']
    f.dynamic_dates = a.dynamic_dates
    f.schedule_kind = (a.schedule_kind || '') as ScheduleKind
    if (a.timezone) f.timezone = a.timezone
    if (a.schedule_kind === 'once') {
      f.once_local = a.once_local
    } else if (a.schedule_kind === 'daily') {
      f.time_local = minutesToTime(a.schedule_minutes)
    } else if (a.schedule_kind === 'weekly' || a.schedule_kind === 'monthly') {
      f.time_local = minutesToTime(a.schedule_minutes)
      f.weekdays = parseWeekdays(a.schedule_weekdays)
      f.week_of_month = a.schedule_week_of_month || 3
    }
    form.value = f
  }

  /**
   * Build the announcement payload from the form. Wall-clock values are sent
   * as-is together with the timezone; the backend resolves them to UTC instants.
   */
  function buildPayload(f: AnnouncementForm): Partial<Announcement> {
    const payload: Partial<Announcement> = {
      type_id: f.type_id,
      title: f.title.trim(),
      details: f.details,
      image: f.image,
      thumbnail: f.thumbnail,
      color: f.color,
      location: f.location,
      mention: f.mention,
      buttons: f.buttons
        .map((b) => ({ label: b.label.trim(), emoji: (b.emoji || '').trim(), url: b.url.trim() }))
        .filter((b) => b.label && /^https?:\/\//i.test(b.url))
        .slice(0, 5),
      timezone: f.timezone,
      start_local: f.start_local,
      end_local: f.end_local,
      start_format: f.start_format,
      end_format: f.end_format,
      dynamic_dates: f.dynamic_dates,
      schedule_kind: f.schedule_kind,
      once_local: '',
      schedule_minutes: 0,
      schedule_weekdays: '',
      schedule_week_of_month: 0,
    }
    if (f.schedule_kind === 'once') {
      payload.once_local = f.once_local
    } else if (f.schedule_kind === 'daily') {
      payload.schedule_minutes = timeToMinutes(f.time_local)
    } else if (f.schedule_kind === 'weekly') {
      payload.schedule_minutes = timeToMinutes(f.time_local)
      payload.schedule_weekdays = [...new Set(f.weekdays)].sort((a, b) => a - b).join(',')
    } else if (f.schedule_kind === 'monthly') {
      payload.schedule_minutes = timeToMinutes(f.time_local)
      payload.schedule_weekdays = String(f.weekdays[0] ?? 6)
      payload.schedule_week_of_month = f.week_of_month
    }
    return payload
  }

  /** Returns an error string if the form is invalid, else null. */
  function validate(f: AnnouncementForm): string | null {
    if (!f.title.trim()) return 'Title is required'
    if (!f.type_id) return 'An announcement type is required'
    if (!f.details.trim()) return 'Details are required'
    if (f.schedule_kind === 'once' && !f.once_local)
      return 'Pick a date & time for the one-time post'
    if ((f.schedule_kind === 'weekly' || f.schedule_kind === 'monthly') && !f.time_local)
      return 'Pick a time for the recurring post'
    if (f.schedule_kind === 'weekly' && f.weekdays.length === 0) return 'Pick at least one weekday'
    if (f.schedule_kind === 'monthly' && f.weekdays.length === 0) return 'Pick a weekday'
    return null
  }

  async function save(): Promise<boolean> {
    const f = form.value
    const err = validate(f)
    if (err) {
      ui.notify(err, 'error')
      return false
    }
    saving.value = true
    try {
      await endpoints.announcements.save(f.id, buildPayload(f))
      ui.notify(f.id ? 'Announcement updated' : 'Announcement created', 'success')
      resetForm()
      await loadAnnouncements()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      saving.value = false
    }
  }

  async function deleteAnnouncement(a: Announcement): Promise<void> {
    if (
      !(await ui.confirm(`Delete "${a.title}"?`, {
        title: 'Delete announcement',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.announcements.delete(a.id)
      ui.notify('Announcement deleted', 'info')
      if (form.value.id === a.id) resetForm()
      await loadAnnouncements()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function sendNow(a: Announcement): Promise<void> {
    if (
      !(await ui.confirm(`Post "${a.title}" to Discord now?`, {
        title: 'Send announcement',
        confirmText: 'Send now',
      }))
    )
      return
    sendingId.value = a.id
    try {
      await endpoints.announcements.sendNow(a.id)
      ui.notify('Announcement posted to Discord', 'success')
      await loadAnnouncements()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      sendingId.value = null
    }
  }

  async function skipNext(a: Announcement): Promise<void> {
    if (
      !(await ui.confirm(`Skip the next scheduled posting of "${a.title}"?`, {
        title: 'Skip next occurrence',
        confirmText: 'Skip next',
      }))
    )
      return
    skippingId.value = a.id
    try {
      await endpoints.announcements.skipNext(a.id)
      ui.notify('Next occurrence will be skipped', 'info')
      await loadAnnouncements()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      skippingId.value = null
    }
  }

  return {
    types,
    sortedTypes,
    roles,
    announcements,
    mainImages,
    thumbImages,
    typeForm,
    roleForm,
    form,
    search,
    typeFilter,
    filteredAnnouncements,
    loading,
    savingType,
    savingRole,
    saving,
    sendingId,
    skippingId,
    load,
    loadTypes,
    loadRoles,
    loadAnnouncements,
    reorder,
    loadImages,
    resetTypeForm,
    editType,
    saveType,
    deleteType,
    resetRoleForm,
    editRole,
    saveRole,
    deleteRole,
    resetForm,
    editAnnouncement,
    save,
    deleteAnnouncement,
    sendNow,
    skipNext,
  }
})
