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
import type {
  Announcement,
  AnnouncementForm,
  AnnouncementType,
  AnnouncementTypeForm,
  ScheduleKind,
} from '@/types/api'
import { useUiStore } from './ui'

/** A blank announcement-type form (used for "Add type" and reset after save). */
function emptyTypeForm(): AnnouncementTypeForm {
  return { id: 0, name: '', webhook_url: '' }
}

/** A blank announcement form with sensible schedule defaults. */
function emptyForm(): AnnouncementForm {
  return {
    id: 0,
    type_id: 0,
    title: '',
    details: '',
    image: '',
    color: '#e53170',
    start_local: '',
    end_local: '',
    schedule_kind: '',
    timezone: detectTimezone(),
    once_local: '',
    time_local: '19:00',
    weekdays: [],
    week_of_month: 3,
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

  // Data.
  const types = ref<AnnouncementType[]>([])
  const announcements = ref<Announcement[]>([])
  const images = ref<string[]>([])

  // Forms.
  const typeForm = ref<AnnouncementTypeForm>(emptyTypeForm())
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
  const saving = ref(false)
  const uploading = ref(false)
  const sendingId = ref<number | null>(null)
  const skippingId = ref<number | null>(null)

  // ── Loads ──────────────────────────────────────────────────────────────────

  async function load(): Promise<void> {
    loading.value = true
    try {
      const [t, a] = await Promise.all([endpoints.announcements.types(), endpoints.announcements.list()])
      types.value = t.types || []
      announcements.value = a.announcements || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      loading.value = false
    }
    loadImages()
  }

  async function loadTypes(): Promise<void> {
    try {
      const data = await endpoints.announcements.types()
      types.value = data.types || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function loadAnnouncements(): Promise<void> {
    try {
      const data = await endpoints.announcements.list()
      announcements.value = data.announcements || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function loadImages(): Promise<void> {
    try {
      const data = await endpoints.announcements.images()
      images.value = data.images || []
    } catch {
      /* non-fatal: the picker just shows nothing */
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
      await endpoints.announcements.saveType({ ...f, name: f.name.trim() })
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
    f.color = a.color || '#e53170'
    f.start_local = a.start_local
    f.end_local = a.end_local
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
      color: f.color,
      timezone: f.timezone,
      start_local: f.start_local,
      end_local: f.end_local,
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
    if (f.schedule_kind === 'once' && !f.once_local) return 'Pick a date & time for the one-time post'
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

  async function uploadImage(event: Event): Promise<void> {
    const input = event.target as HTMLInputElement
    const file = input.files && input.files[0]
    if (!file) return
    uploading.value = true
    try {
      const formData = new FormData()
      formData.append('image', file)
      const data = await endpoints.announcements.uploadImage(formData)
      form.value.image = data.url
      ui.notify('Image uploaded', 'success')
      await loadImages()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      uploading.value = false
      input.value = ''
    }
  }

  /** Reuse an existing uploaded image (no duplicate upload). */
  function pickImage(url: string): void {
    form.value.image = url
  }

  return {
    types,
    announcements,
    images,
    typeForm,
    form,
    search,
    typeFilter,
    filteredAnnouncements,
    loading,
    savingType,
    saving,
    uploading,
    sendingId,
    skippingId,
    load,
    loadTypes,
    loadAnnouncements,
    loadImages,
    resetTypeForm,
    editType,
    saveType,
    deleteType,
    resetForm,
    editAnnouncement,
    save,
    deleteAnnouncement,
    sendNow,
    skipNext,
    uploadImage,
    pickImage,
  }
})
