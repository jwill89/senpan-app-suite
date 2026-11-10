/**
 * Announcement management store (admin).
 *
 * Covers Announcement Types (a named Discord webhook destination) and the
 * Announcements that post to them - CRUD, manual "send now", "skip next
 * occurrence", and a client-side search filter. Mirrors the structure of the
 * book-club store.
 *
 * One IANA `timezone` anchors every time on an announcement: the form holds
 * wall-clock values (start/end window, one-time post, and the recurring
 * time/weekday selections) which are sent as-is; the backend resolves them in the
 * timezone to the absolute UTC instants it stores, so all times survive DST.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  api,
  unwrap,
  type Announcement,
  type AnnouncementButton,
  type AnnouncementInput,
  type AnnouncementRole,
  type AnnouncementRoleInput,
  type AnnouncementType,
  type AnnouncementTypeInput,
} from '@/api/client'
import { detectTimezone } from '@/lib/constants'
import { nextUid } from '@/lib/uid'
import type { DiscordTimeFormat, ScheduleKind } from '@/types/api'
import { useUiStore } from './ui'

/** The editable form model for an announcement type (id 0 = new). */
export interface AnnouncementTypeForm extends AnnouncementTypeInput {
  id: number
}

/** The editable form model for a taggable role (id 0 = new). */
export interface AnnouncementRoleForm extends AnnouncementRoleInput {
  id: number
}

/**
 * A Discord button row in the announcement form. `_uid` is a client-only stable
 * key for the repeater (see lib/uid.ts); it's stripped when the payload is built.
 */
export interface AnnouncementButtonForm extends AnnouncementButton {
  _uid?: number
}

/**
 * Form model for the admin announcement create/edit form (id 0 = new). Holds
 * *local* wall-clock strings and a friendly recurrence builder in place of the
 * wire's schedule fields: `startLocal`/`endLocal`/`onceLocal` are `datetime-local`
 * values; `timeLocal` is an "HH:mm" wall-clock time anchored to `timezone` (an IANA
 * name, so recurring times survive DST); `weekdays` are weekday numbers
 * (0=Sun..6=Sat) in that zone.
 */
export interface AnnouncementForm extends Omit<
  AnnouncementInput,
  | 'buttons'
  | 'scheduleKind'
  | 'scheduleMinutes'
  | 'scheduleWeekdays'
  | 'scheduleWeekOfMonth'
  | 'startFormat'
  | 'endFormat'
> {
  id: number
  /** Discord timestamp style for the start display (see DISCORD_TIME_FORMATS). */
  startFormat: DiscordTimeFormat
  /** Discord timestamp style for the end display (see DISCORD_TIME_FORMATS). */
  endFormat: DiscordTimeFormat
  scheduleKind: ScheduleKind
  timeLocal: string
  weekdays: number[]
  weekOfMonth: number
  /** Optional Discord link buttons (max 5) rendered beneath the embed. */
  buttons: AnnouncementButtonForm[]
}

/** A blank announcement-type form (used for "Add type" and reset after save). */
function emptyTypeForm(): AnnouncementTypeForm {
  return { id: 0, name: '', webhookUrl: '' }
}

/** A blank taggable-role form (used for "Add role" and reset after save). */
function emptyRoleForm(): AnnouncementRoleForm {
  return { id: 0, name: '', roleId: '' }
}

/** A blank announcement form with sensible schedule defaults. */
function emptyForm(): AnnouncementForm {
  return {
    id: 0,
    typeId: 0,
    title: '',
    details: '',
    image: '',
    thumbnail: '',
    color: '#ff3131',
    location: '',
    startLocal: '',
    endLocal: '',
    startFormat: 'F',
    endFormat: 't',
    dynamicDates: false,
    scheduleKind: '',
    timezone: detectTimezone(),
    onceLocal: '',
    timeLocal: '19:00',
    weekdays: [],
    weekOfMonth: 3,
    buttons: [],
    mention: '',
  }
}

const pad = (n: number): string => String(n).padStart(2, '0')

/** Parse an "HH:mm" wall-clock string into minutes-of-day (0-1439). */
function timeToMinutes(time: string): number {
  const [hh = 0, mm = 0] = time.split(':').map((n) => Number(n) || 0)
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
  /** Types sorted alphabetically by name (for the form dropdown + types list). */
  const sortedTypes = computed(() => [...types.value].sort((a, b) => a.name.localeCompare(b.name)))
  const roles = ref<AnnouncementRole[]>([])
  const announcements = ref<Announcement[]>([])

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
      if (tf && a.typeId !== tf) {
        return false
      }
      if (!q) {
        return true
      }
      return [a.title, a.typeName, a.details].some((s) => s.toLowerCase().includes(q))
    })
  })

  // In-flight flags.
  const loading = ref(false)
  const savingType = ref(false)
  const savingRole = ref(false)
  const saving = ref(false)
  const sendingId = ref<number | null>(null)
  const skippingId = ref<number | null>(null)

  // -- Loads ------------------------------------------------------------------

  async function load(): Promise<void> {
    loading.value = true
    try {
      const [t, r, a] = await Promise.all([
        unwrap(api.GET('/announcement-types')),
        unwrap(api.GET('/announcement-roles')),
        unwrap(api.GET('/announcements')),
      ])
      types.value = t.types
      roles.value = r.roles
      announcements.value = a.announcements
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      loading.value = false
    }
  }

  async function loadTypes(): Promise<void> {
    try {
      types.value = (await unwrap(api.GET('/announcement-types'))).types
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function loadRoles(): Promise<void> {
    try {
      roles.value = (await unwrap(api.GET('/announcement-roles'))).roles
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function loadAnnouncements(): Promise<void> {
    try {
      announcements.value = (await unwrap(api.GET('/announcements'))).announcements
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
      await unwrap(api.POST('/announcements/reorder', { body: { orderedIds } }))
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      await loadAnnouncements() // revert to the persisted order
    }
  }

  // -- Types ------------------------------------------------------------------

  function resetTypeForm(): void {
    typeForm.value = emptyTypeForm()
  }

  function editType(t: AnnouncementType): void {
    typeForm.value = { id: t.id, name: t.name, webhookUrl: t.webhookUrl }
  }

  async function saveType(): Promise<boolean> {
    const { id, ...rest } = typeForm.value
    if (!rest.name.trim()) {
      ui.notify('Type name is required', 'error')
      return false
    }
    savingType.value = true
    try {
      const body: AnnouncementTypeInput = {
        name: rest.name.trim(),
        webhookUrl: rest.webhookUrl.trim(),
      }
      if (id) {
        await unwrap(api.PUT('/announcement-types/{id}', { params: { path: { id } }, body }))
      } else {
        await unwrap(api.POST('/announcement-types', { body }))
      }
      ui.notify(id ? 'Type updated' : 'Type added', 'success')
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
    ) {
      return
    }
    try {
      await unwrap(api.DELETE('/announcement-types/{id}', { params: { path: { id: t.id } } }))
      ui.notify('Type deleted', 'info')
      if (typeForm.value.id === t.id) {
        resetTypeForm()
      }
      await loadTypes()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // -- Roles (taggable Discord roles) -------------------------------------------

  function resetRoleForm(): void {
    roleForm.value = emptyRoleForm()
  }

  function editRole(r: AnnouncementRole): void {
    roleForm.value = { id: r.id, name: r.name, roleId: r.roleId }
  }

  async function saveRole(): Promise<boolean> {
    const { id, ...rest } = roleForm.value
    if (!rest.name.trim()) {
      ui.notify('Role name is required', 'error')
      return false
    }
    if (!/^\d+$/.test(rest.roleId.trim())) {
      ui.notify('Role ID must be a Discord role ID (digits only)', 'error')
      return false
    }
    savingRole.value = true
    try {
      const body: AnnouncementRoleInput = { name: rest.name.trim(), roleId: rest.roleId.trim() }
      if (id) {
        await unwrap(api.PUT('/announcement-roles/{id}', { params: { path: { id } }, body }))
      } else {
        await unwrap(api.POST('/announcement-roles', { body }))
      }
      ui.notify(id ? 'Role updated' : 'Role added', 'success')
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
    ) {
      return
    }
    try {
      await unwrap(api.DELETE('/announcement-roles/{id}', { params: { path: { id: r.id } } }))
      ui.notify('Role deleted', 'info')
      if (roleForm.value.id === r.id) {
        resetRoleForm()
      }
      await loadRoles()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // -- Announcements ----------------------------------------------------------

  function resetForm(): void {
    form.value = emptyForm()
  }

  function editAnnouncement(a: Announcement): void {
    const f = emptyForm()
    f.id = a.id
    f.typeId = a.typeId
    f.title = a.title
    f.details = a.details
    f.image = a.image
    f.thumbnail = a.thumbnail || ''
    f.color = a.color || '#ff3131'
    f.location = a.location || ''
    f.mention = a.mention || ''
    f.buttons = a.buttons.map((b) => ({ ...b, _uid: nextUid() }))
    f.startLocal = a.startLocal
    f.endLocal = a.endLocal
    f.startFormat = (a.startFormat || 'F') as DiscordTimeFormat
    f.endFormat = (a.endFormat || 't') as DiscordTimeFormat
    f.dynamicDates = a.dynamicDates
    f.scheduleKind = (a.scheduleKind || '') as ScheduleKind
    if (a.timezone) {
      f.timezone = a.timezone
    }
    if (a.scheduleKind === 'once') {
      f.onceLocal = a.onceLocal
    } else if (a.scheduleKind === 'daily') {
      f.timeLocal = minutesToTime(a.scheduleMinutes)
    } else if (a.scheduleKind === 'weekly' || a.scheduleKind === 'monthly') {
      f.timeLocal = minutesToTime(a.scheduleMinutes)
      f.weekdays = parseWeekdays(a.scheduleWeekdays)
      f.weekOfMonth = a.scheduleWeekOfMonth || 3
    }
    form.value = f
  }

  /**
   * Build the announcement body from the form. Wall-clock values are sent as-is
   * together with the timezone; the backend resolves them to UTC instants.
   */
  function buildPayload(f: AnnouncementForm): AnnouncementInput {
    const payload: AnnouncementInput = {
      typeId: f.typeId,
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
      startLocal: f.startLocal,
      endLocal: f.endLocal,
      startFormat: f.startFormat,
      endFormat: f.endFormat,
      dynamicDates: f.dynamicDates,
      scheduleKind: f.scheduleKind,
      onceLocal: '',
      scheduleMinutes: 0,
      scheduleWeekdays: '',
      scheduleWeekOfMonth: 0,
    }
    if (f.scheduleKind === 'once') {
      payload.onceLocal = f.onceLocal
    } else if (f.scheduleKind === 'daily') {
      payload.scheduleMinutes = timeToMinutes(f.timeLocal)
    } else if (f.scheduleKind === 'weekly') {
      payload.scheduleMinutes = timeToMinutes(f.timeLocal)
      payload.scheduleWeekdays = [...new Set(f.weekdays)].sort((a, b) => a - b).join(',')
    } else if (f.scheduleKind === 'monthly') {
      payload.scheduleMinutes = timeToMinutes(f.timeLocal)
      payload.scheduleWeekdays = String(f.weekdays[0] ?? 6)
      payload.scheduleWeekOfMonth = f.weekOfMonth
    }
    return payload
  }

  /** Returns an error string if the form is invalid, else null. */
  function validate(f: AnnouncementForm): string | null {
    if (!f.title.trim()) {
      return 'Title is required'
    }
    if (!f.typeId) {
      return 'An announcement type is required'
    }
    if (!f.details.trim()) {
      return 'Details are required'
    }
    if (f.scheduleKind === 'once' && !f.onceLocal) {
      return 'Pick a date & time for the one-time post'
    }
    if ((f.scheduleKind === 'weekly' || f.scheduleKind === 'monthly') && !f.timeLocal) {
      return 'Pick a time for the recurring post'
    }
    if (f.scheduleKind === 'weekly' && f.weekdays.length === 0) {
      return 'Pick at least one weekday'
    }
    if (f.scheduleKind === 'monthly' && f.weekdays.length === 0) {
      return 'Pick a weekday'
    }
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
      const body = buildPayload(f)
      if (f.id) {
        await unwrap(api.PUT('/announcements/{id}', { params: { path: { id: f.id } }, body }))
      } else {
        await unwrap(api.POST('/announcements', { body }))
      }
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
    ) {
      return
    }
    try {
      await unwrap(api.DELETE('/announcements/{id}', { params: { path: { id: a.id } } }))
      ui.notify('Announcement deleted', 'info')
      if (form.value.id === a.id) {
        resetForm()
      }
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
    ) {
      return
    }
    sendingId.value = a.id
    try {
      await unwrap(api.POST('/announcements/{id}/send', { params: { path: { id: a.id } } }))
      ui.notify('Announcement posted to Discord', 'success')
      await loadAnnouncements()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      sendingId.value = null
    }
  }

  /**
   * Sets how many upcoming occurrences to skip (0 clears it). No confirm dialog:
   * the caller's number prompt IS the deliberate step, and it is reversible -
   * setting 0 undoes it.
   */
  async function setSkip(a: Announcement, count: number): Promise<boolean> {
    skippingId.value = a.id
    try {
      await unwrap(
        api.POST('/announcements/{id}/skip', { params: { path: { id: a.id } }, body: { count } }),
      )
      ui.notify(
        count > 0
          ? `Skipping the next ${count} ${count === 1 ? 'occurrence' : 'occurrences'}`
          : 'Skip cleared - this announcement posts as scheduled',
        'info',
      )
      await loadAnnouncements()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      skippingId.value = null
    }
  }

  return {
    types,
    sortedTypes,
    roles,
    announcements,
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
    setSkip,
  }
})
