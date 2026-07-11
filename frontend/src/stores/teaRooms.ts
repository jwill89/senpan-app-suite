/**
 * Tea Rooms store: admin management (list + create/edit/delete) of bookable tea
 * rooms, drag-reorder of the list, quick open/discounted toggles, posting a room
 * to Discord, and the single shared Discord webhook the posts go to.
 *
 * A cousin of the affiliates store (a flat single-table entity with an image
 * picked from the shared library) crossed with the announcements store (a
 * drag-orderable list plus per-row action flags + a webhook).
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type { TeaRoom, TeaRoomForm } from '@/types/api'
import { useUiStore } from './ui'
import { withLoading } from '@/lib/withLoading'

/** A fresh, empty tea-room form (open by default, brand-pink accent). */
function emptyForm(): TeaRoomForm {
  return {
    id: 0,
    name: '',
    room_number: '',
    cost_per_half_hour: 0,
    hashtags: '',
    description: '',
    seasonal: false,
    open: true,
    lockable: false,
    discounted: false,
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
    if (!q) return teaRooms.value
    return teaRooms.value.filter((t) =>
      [t.name, t.room_number, t.hashtags, t.description].some((s) => s.toLowerCase().includes(q)),
    )
  })

  const loading = ref(false)
  const saving = ref(false)
  const savingWebhook = ref(false)
  const postingId = ref<number | null>(null)
  const togglingId = ref<number | null>(null)

  // ── Load ───────────────────────────────────────────────────────────────────
  async function loadTeaRooms(): Promise<void> {
    await withLoading(loading, async () => {
      const data = await endpoints.teaRooms.list()
      teaRooms.value = data.tea_rooms
      webhookUrl.value = data.webhook_url
    })
  }

  // ── Form ───────────────────────────────────────────────────────────────────
  function newTeaRoomForm(): void {
    teaRoomForm.value = emptyForm()
  }

  function editTeaRoom(t: TeaRoom): void {
    teaRoomForm.value = {
      id: t.id,
      name: t.name,
      room_number: t.room_number,
      cost_per_half_hour: t.cost_per_half_hour,
      hashtags: t.hashtags,
      description: t.description,
      seasonal: t.seasonal,
      open: t.open,
      lockable: t.lockable,
      discounted: t.discounted,
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
    if (!f) return false
    if (!f.name.trim()) {
      ui.notify('Room name is required', 'error')
      return false
    }
    if (f.cost_per_half_hour < 0) {
      ui.notify('Cost cannot be negative', 'error')
      return false
    }
    saving.value = true
    try {
      const { id, ...payload } = f
      if (id) {
        await endpoints.teaRooms.update(id, payload)
        ui.notify('Tea room updated', 'success')
      } else {
        await endpoints.teaRooms.create(payload)
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
    )
      return
    try {
      await endpoints.teaRooms.delete(t.id)
      teaRooms.value = teaRooms.value.filter((r) => r.id !== t.id)
      if (teaRoomForm.value?.id === t.id) teaRoomForm.value = null
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
      await endpoints.teaRooms.reorder(orderedIds)
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
    )
      return
    postingId.value = t.id
    try {
      await endpoints.teaRooms.post(t.id)
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

  /** Shared PATCH for the quick-toggle flags, replacing the row with the server's copy. */
  async function patchFlag(
    t: TeaRoom,
    fields: { open?: boolean; discounted?: boolean },
  ): Promise<void> {
    togglingId.value = t.id
    try {
      const data = await endpoints.teaRooms.patch(t.id, fields)
      if (data.tea_room) {
        const i = teaRooms.value.findIndex((r) => r.id === t.id)
        if (i !== -1) teaRooms.value[i] = data.tea_room
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
      const data = await endpoints.teaRooms.setWebhook(url.trim())
      webhookUrl.value = data.webhook_url
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
    saveWebhook,
  }
})
