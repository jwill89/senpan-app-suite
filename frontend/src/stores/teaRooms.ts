/**
 * Tea Rooms store: admin management (list + create/edit/delete) of bookable tea
 * rooms, drag-reorder of the list, quick open/discounted toggles, locking a room
 * (with or without an unlock time), posting a room to Discord, and the single
 * shared Discord webhook the posts go to.
 *
 * A cousin of the affiliates store (a flat single-table entity with an image
 * picked from the shared library) crossed with the announcements store (a
 * drag-orderable list plus per-row action flags + a webhook).
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, unwrap, type TeaRoom, type TeaRoomChanges, type TeaRoomInput } from '@/api/client'
import { useUiStore } from './ui'
import { withLoading } from '@/lib/withLoading'
import { datetimeLocalToUtc, utcToDatetimeLocal } from '@/lib/datetime'

/**
 * The alert shown when a room's timed lock runs out on its own. The plugin builds
 * the same sentence (Windows/TeaRoomsTab.cs) - change one, change both.
 */
export function teaRoomUnlockedMessage(name: string, roomNumber: string): string {
  const room = name.trim() || 'A tea room'
  const number = roomNumber.trim().replace(/^#/, '')
  const label = number ? `${room} (Room #${number})` : room
  return `${label} has been marked as unlocked. Please check the room to remind the occupants or extend the time.`
}

/**
 * The editable form model for a tea room (id 0 = new). The unlock time is held as the
 * local wall-clock value of a datetime-local input, and sent as a UTC instant.
 */
export interface TeaRoomForm extends Omit<TeaRoomInput, 'lockedUntil'> {
  id: number
  lockedUntil: string
}

/** A fresh, empty tea-room form (open by default, brand-pink accent). */
function emptyForm(): TeaRoomForm {
  return {
    id: 0,
    name: '',
    subtitle: '',
    roomOwner: '',
    roomNumber: '',
    costPerHalfHour: 0,
    hashtags: '',
    description: '',
    seasonal: false,
    open: true,
    lockable: false,
    discounted: false,
    locked: false,
    lockedUntil: '',
    image: '',
    color: '#ff3131',
  }
}

export const useTeaRoomsStore = defineStore('teaRooms', () => {
  const ui = useUiStore()

  const teaRooms = ref<TeaRoom[]>([])
  /** The single shared Discord webhook every room posts to ('' = none). */
  const webhookUrl = ref('')
  const teaRoomForm = ref<TeaRoomForm | null>(null)

  const search = ref('')
  const filteredTeaRooms = computed(() => {
    const q = search.value.trim().toLowerCase()
    if (!q) {
      return teaRooms.value
    }
    return teaRooms.value.filter((t) =>
      [t.name, t.roomNumber, t.hashtags, t.description].some((s) => s.toLowerCase().includes(q)),
    )
  })

  const loading = ref(false)
  const saving = ref(false)
  const savingWebhook = ref(false)
  const postingId = ref<number | null>(null)
  const togglingId = ref<number | null>(null)

  // -- Load -------------------------------------------------------------------
  async function loadTeaRooms(): Promise<void> {
    await withLoading(loading, async () => {
      const [list, hook] = await Promise.all([
        unwrap(api.GET('/tea-rooms')),
        unwrap(api.GET('/tea-rooms/webhook')),
      ])
      teaRooms.value = list.teaRooms
      webhookUrl.value = hook.webhookUrl
    })
  }

  // -- Form -------------------------------------------------------------------
  function newTeaRoomForm(): void {
    teaRoomForm.value = emptyForm()
  }

  function editTeaRoom(t: TeaRoom): void {
    teaRoomForm.value = {
      id: t.id,
      name: t.name,
      subtitle: t.subtitle,
      roomOwner: t.roomOwner,
      roomNumber: t.roomNumber,
      costPerHalfHour: t.costPerHalfHour,
      hashtags: t.hashtags,
      description: t.description,
      seasonal: t.seasonal,
      open: t.open,
      lockable: t.lockable,
      discounted: t.discounted,
      locked: t.locked,
      // Stored as a UTC instant; the form edits it as local wall-clock time.
      lockedUntil: utcToDatetimeLocal(t.lockedUntil),
      image: t.image,
      color: t.color || '#ff3131',
    }
  }

  function cancelTeaRoomForm(): void {
    teaRoomForm.value = null
  }

  /** Saves the tea-room form. Returns true on success (caller navigates back). */
  async function saveTeaRoom(): Promise<boolean> {
    const f = teaRoomForm.value
    if (!f) {
      return false
    }
    if (!f.name.trim()) {
      ui.notify('Room name is required', 'error')
      return false
    }
    if (!f.roomNumber.trim()) {
      ui.notify('Room number is required', 'error')
      return false
    }
    if (f.costPerHalfHour < 0) {
      ui.notify('Cost cannot be negative', 'error')
      return false
    }
    saving.value = true
    try {
      // The form holds a local datetime-local value; the stored unlock time is a
      // UTC instant, so the two clients (and the sweeper) agree on the moment.
      const { id, ...rest } = f
      const body: TeaRoomInput = { ...rest, lockedUntil: datetimeLocalToUtc(f.lockedUntil) || null }
      if (id) {
        await unwrap(api.PUT('/tea-rooms/{id}', { params: { path: { id } }, body }))
        ui.notify('Tea room updated', 'success')
      } else {
        await unwrap(api.POST('/tea-rooms', { body }))
        ui.notify('Tea room created', 'success')
      }
      teaRoomForm.value = null
      await loadTeaRooms()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      saving.value = false
    }
  }

  async function deleteTeaRoom(t: TeaRoom): Promise<void> {
    if (
      !(await ui.confirm(`Delete "${t.name}"?`, {
        title: 'Delete tea room',
        confirmText: 'Delete',
      }))
    ) {
      return
    }
    try {
      await unwrap(api.DELETE('/tea-rooms/{id}', { params: { path: { id: t.id } } }))
      teaRooms.value = teaRooms.value.filter((r) => r.id !== t.id)
      if (teaRoomForm.value?.id === t.id) {
        teaRoomForm.value = null
      }
      ui.notify('Tea room deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /**
   * Persists a drag-and-drop reorder. The list array is mutated optimistically by
   * the drag interaction; this saves the new order and reverts on failure.
   */
  async function reorder(orderedIds: number[]): Promise<void> {
    try {
      await unwrap(api.POST('/tea-rooms/reorder', { body: { orderedIds } }))
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      await loadTeaRooms() // revert to the persisted order
    }
  }

  /** Post a room's embed to the shared Discord webhook (after confirmation). */
  async function postRoom(t: TeaRoom): Promise<void> {
    if (!webhookUrl.value.trim()) {
      ui.notify('Set a Discord webhook first (Webhook button above).', 'error')
      return
    }
    if (
      !(await ui.confirm(`Post "${t.name}" to Discord now?`, {
        title: 'Post tea room',
        confirmText: 'Post',
      }))
    ) {
      return
    }
    postingId.value = t.id
    try {
      await unwrap(api.POST('/tea-rooms/{id}/post', { params: { path: { id: t.id } } }))
      ui.notify('Tea room posted to Discord', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      postingId.value = null
    }
  }

  /** Flip a room's open/closed flag (optimistic; reverts on failure). */
  async function toggleOpen(t: TeaRoom): Promise<void> {
    await patchFlag(t, { open: !t.open })
  }

  /** Flip a room's discounted flag (optimistic; reverts on failure). */
  async function toggleDiscounted(t: TeaRoom): Promise<void> {
    await patchFlag(t, { discounted: !t.discounted })
  }

  /**
   * Lock a room. `untilLocal` is a datetime-local value (local wall clock) for a
   * lock that lifts by itself, or '' for one that stands until someone unlocks
   * the room - the way every lock behaved before expiries existed.
   *
   * A time already gone is refused here rather than sent: the server would take
   * it, expire the lock on its next sweep and alert the staff about a room that
   * was never really locked, which reads as a bug rather than as the typo it is.
   */
  async function lockRoom(t: TeaRoom, untilLocal: string): Promise<boolean> {
    const until = datetimeLocalToUtc(untilLocal)
    if (untilLocal && !until) {
      ui.notify('That unlock time is not a valid date and time', 'error')
      return false
    }
    if (until && new Date(until).getTime() <= Date.now()) {
      ui.notify('The unlock time must be in the future', 'error')
      return false
    }
    await patchFlag(t, until ? { locked: true, lockedUntil: until } : { locked: true })
    return true
  }

  /** Unlock a room now, whatever expiry its lock carried. */
  async function unlockRoom(t: TeaRoom): Promise<void> {
    await patchFlag(t, { locked: false })
  }

  /** Shared PATCH for the quick-toggle flags, replacing the row with the server's copy. */
  async function patchFlag(t: TeaRoom, body: TeaRoomChanges): Promise<void> {
    togglingId.value = t.id
    try {
      const saved = await unwrap(
        api.PATCH('/tea-rooms/{id}', { params: { path: { id: t.id } }, body }),
      )
      const i = teaRooms.value.findIndex((r) => r.id === t.id)
      if (i !== -1) {
        teaRooms.value[i] = saved
      }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      togglingId.value = null
    }
  }

  /** Save the shared Discord webhook. Returns true on success. */
  async function saveWebhook(url: string): Promise<boolean> {
    savingWebhook.value = true
    try {
      const saved = await unwrap(
        api.PUT('/tea-rooms/webhook', { body: { webhookUrl: url.trim() } }),
      )
      webhookUrl.value = saved.webhookUrl
      ui.notify('Discord webhook saved', 'success')
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingWebhook.value = false
    }
  }

  return {
    teaRooms,
    webhookUrl,
    teaRoomForm,
    search,
    filteredTeaRooms,
    loading,
    saving,
    savingWebhook,
    postingId,
    togglingId,
    loadTeaRooms,
    newTeaRoomForm,
    editTeaRoom,
    cancelTeaRoomForm,
    saveTeaRoom,
    deleteTeaRoom,
    reorder,
    postRoom,
    toggleOpen,
    toggleDiscounted,
    lockRoom,
    unlockRoom,
    saveWebhook,
  }
})
