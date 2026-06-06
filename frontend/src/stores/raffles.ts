/**
 * Raffles store: public browsing + sign-up, and admin management (CRUD,
 * entries, winner picking, image upload). Mirrors all raffle logic from app.js.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api } from '@/lib/api'
import type {
  Raffle,
  RaffleDetailResponse,
  RaffleEnterResponse,
  RaffleEntry,
  RaffleForm,
  RaffleUploadResponse,
  RaffleWinnerResponse,
} from '@/types/api'
import { useUiStore } from './ui'

export const useRafflesStore = defineStore('raffles', () => {
  const ui = useUiStore()

  const homeRaffles = ref<Raffle[]>([]) // open raffles for home card visibility
  const raffles = ref<Raffle[]>([])
  const selectedRaffle = ref<Raffle | null>(null)
  const raffleEntries = ref<RaffleEntry[]>([])
  const raffleForm = ref<RaffleForm | null>(null)
  const raffleSignup = ref<{ characterName: string; world: string; numEntries: number }>({
    characterName: '',
    world: '',
    numEntries: 1,
  })
  const raffleSignupResult = ref<RaffleEnterResponse | null>(null)
  const raffleWinner = ref<RaffleEntry | null>(null)
  const raffleWinnerEntry = ref<RaffleEntry | null>(null) // public closed view
  const raffleTotalEntryCount = ref(0)
  const raffleImageUploading = ref(false)

  // ── Computed ───────────────────────────────────────────────────────────────

  const openRaffles = computed(() => raffles.value.filter((r) => r.status === 'open'))
  const closedRaffles = computed(() => raffles.value.filter((r) => r.status === 'closed'))

  // ── Load ─────────────────────────────────────────────────────────────────

  async function loadRaffles(): Promise<void> {
    try {
      const data = await api<{ raffles: Raffle[] }>('raffles')
      raffles.value = data.raffles || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Preloads open raffles (home page card visibility). */
  async function loadHomeRaffles(): Promise<void> {
    try {
      const data = await api<{ raffles: Raffle[] }>('raffles')
      homeRaffles.value = (data.raffles || []).filter((r) => r.status === 'open')
    } catch {
      /* silent */
    }
  }

  async function loadRaffleDetail(id: number): Promise<void> {
    try {
      const data = await api<RaffleDetailResponse>('raffles/' + id)
      selectedRaffle.value = data.raffle
      raffleEntries.value = data.entries || []
      raffleWinner.value = null
      if (data.raffle.winner_entry_id && raffleEntries.value.length) {
        raffleWinner.value =
          raffleEntries.value.find((e) => e.id === data.raffle.winner_entry_id) || null
      }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Admin: open a raffle's detail view. */
  function viewRaffle(raffle: Raffle): void {
    selectedRaffle.value = raffle
    raffleSignup.value = { characterName: '', world: '', numEntries: 1 }
    raffleSignupResult.value = null
    raffleEntries.value = []
    raffleWinner.value = null
    loadRaffleDetail(raffle.id)
  }

  /** Public: open a raffle's detail view (loads winner + total entries). */
  function viewPublicRaffle(raffle: Raffle): void {
    selectedRaffle.value = raffle
    raffleSignup.value = { characterName: '', world: '', numEntries: 1 }
    raffleSignupResult.value = null
    raffleWinnerEntry.value = null
    raffleTotalEntryCount.value = 0
    api<RaffleDetailResponse>('raffles/' + raffle.id)
      .then((data) => {
        selectedRaffle.value = data.raffle
        raffleTotalEntryCount.value = data.total_entries || 0
        if (data.winner_entry) raffleWinnerEntry.value = data.winner_entry
      })
      .catch(() => {})
  }

  /**
   * Public: load a raffle's detail by id (used when navigating directly to
   * /raffles/:id, e.g. on refresh or a shared link). Resets the sign-up state,
   * then fetches the raffle + winner + total entry count. Returns true on
   * success, false on failure (so the view can redirect back to the list).
   */
  async function loadPublicRaffleById(id: number): Promise<boolean> {
    selectedRaffle.value = null
    raffleSignup.value = { characterName: '', world: '', numEntries: 1 }
    raffleSignupResult.value = null
    raffleWinnerEntry.value = null
    raffleTotalEntryCount.value = 0
    try {
      const data = await api<RaffleDetailResponse>('raffles/' + id)
      selectedRaffle.value = data.raffle
      raffleTotalEntryCount.value = data.total_entries || 0
      if (data.winner_entry) raffleWinnerEntry.value = data.winner_entry
      return true
    } catch {
      return false
    }
  }

  // ── Admin form ─────────────────────────────────────────────────────────────

  function newRaffleForm(): void {
    raffleForm.value = {
      id: 0,
      title: '',
      description: '',
      rules: '',
      max_entries: 1,
      signup_instructions: '',
      cost_per_entry: 0,
      available_from: '',
      available_to: '',
      prize_image: '',
    }
  }

  function editRaffleForm(raffle: Raffle): void {
    raffleForm.value = { ...raffle } as unknown as RaffleForm
  }

  function cancelRaffleForm(): void {
    raffleForm.value = null
  }

  /** Saves the raffle form. Returns true on success (caller navigates). */
  async function saveRaffle(): Promise<boolean> {
    if (!raffleForm.value) return false
    const f = raffleForm.value
    if (!f.title.trim()) {
      ui.notify('Title is required', 'error')
      return false
    }
    try {
      if (f.id) {
        await api('raffles', { method: 'POST', body: { action: 'update', ...f } })
        ui.notify('Raffle updated', 'success')
      } else {
        await api('raffles', { method: 'POST', body: { action: 'create', ...f } })
        ui.notify('Raffle created', 'success')
      }
      raffleForm.value = null
      await loadRaffles()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    }
  }

  async function deleteRaffle(id: number): Promise<void> {
    if (
      !(await ui.confirm('Delete this raffle and all its entries?', {
        title: 'Delete raffle',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await api('raffles', { method: 'POST', body: { action: 'delete', id } })
      raffles.value = raffles.value.filter((r) => r.id !== id)
      if (selectedRaffle.value && selectedRaffle.value.id === id) selectedRaffle.value = null
      ui.notify('Raffle deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function uploadRaffleImage(event: Event): Promise<void> {
    const input = event.target as HTMLInputElement
    const file = input.files && input.files[0]
    if (!file) return
    raffleImageUploading.value = true
    try {
      const formData = new FormData()
      formData.append('image', file)
      const data = await api<RaffleUploadResponse>('raffles/upload', {
        method: 'POST',
        body: formData,
      })
      if (raffleForm.value) raffleForm.value.prize_image = data.path
      ui.notify('Image uploaded', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      raffleImageUploading.value = false
    }
  }

  // ── Public sign-up ─────────────────────────────────────────────────────────

  function raffleTotalCost(): number {
    if (!selectedRaffle.value) return 0
    return (raffleSignup.value.numEntries || 1) * selectedRaffle.value.cost_per_entry
  }

  async function enterRaffle(): Promise<void> {
    if (!selectedRaffle.value) return
    const s = raffleSignup.value
    if (!s.characterName.trim() || !s.world.trim()) {
      ui.notify('Character name and world are required', 'error')
      return
    }
    try {
      const data = await api<RaffleEnterResponse>(
        'raffles/' + selectedRaffle.value.id + '/enter',
        {
          method: 'POST',
          body: {
            character_name: s.characterName.trim(),
            world: s.world.trim(),
            num_entries: s.numEntries,
          },
        },
      )
      raffleSignupResult.value = data
      ui.notify(data.message, 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // ── Admin entries ──────────────────────────────────────────────────────────

  async function toggleEntryPaid(entry: RaffleEntry): Promise<void> {
    if (!selectedRaffle.value) return
    try {
      await api('raffles/' + selectedRaffle.value.id + '/entries', {
        method: 'POST',
        body: { action: 'mark_paid', entry_id: entry.id, paid: !entry.paid },
      })
      entry.paid = !entry.paid
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function deleteEntry(entry: RaffleEntry): Promise<void> {
    if (!selectedRaffle.value) return
    if (!(await ui.confirm('Delete this entry?', { title: 'Delete entry', confirmText: 'Delete' })))
      return
    try {
      await api('raffles/' + selectedRaffle.value.id + '/entries', {
        method: 'POST',
        body: { action: 'delete_entry', entry_id: entry.id },
      })
      raffleEntries.value = raffleEntries.value.filter((e) => e.id !== entry.id)
      ui.notify('Entry deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function pickRaffleWinner(): Promise<void> {
    if (!selectedRaffle.value) return
    try {
      const data = await api<RaffleWinnerResponse>(
        'raffles/' + selectedRaffle.value.id + '/entries',
        { method: 'POST', body: { action: 'pick_winner' } },
      )
      raffleWinner.value = data.winner
      selectedRaffle.value.winner_entry_id = data.winner.id
      ui.notify('Winner picked!', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function verifyRaffleWinner(): Promise<void> {
    if (!selectedRaffle.value) return
    try {
      await api('raffles/' + selectedRaffle.value.id + '/entries', {
        method: 'POST',
        body: { action: 'verify_winner' },
      })
      selectedRaffle.value.status = 'closed'
      ui.notify('Winner verified! Raffle closed.', 'success')
      await loadRaffles()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function pickAnotherWinner(): Promise<void> {
    if (!selectedRaffle.value) return
    try {
      const data = await api<RaffleWinnerResponse>(
        'raffles/' + selectedRaffle.value.id + '/entries',
        { method: 'POST', body: { action: 'pick_another' } },
      )
      raffleWinner.value = data.winner
      selectedRaffle.value.winner_entry_id = data.winner.id
      ui.notify('New winner picked!', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  return {
    homeRaffles,
    raffles,
    selectedRaffle,
    raffleEntries,
    raffleForm,
    raffleSignup,
    raffleSignupResult,
    raffleWinner,
    raffleWinnerEntry,
    raffleTotalEntryCount,
    raffleImageUploading,
    openRaffles,
    closedRaffles,
    loadRaffles,
    loadHomeRaffles,
    loadRaffleDetail,
    viewRaffle,
    viewPublicRaffle,
    loadPublicRaffleById,
    newRaffleForm,
    editRaffleForm,
    cancelRaffleForm,
    saveRaffle,
    deleteRaffle,
    uploadRaffleImage,
    raffleTotalCost,
    enterRaffle,
    toggleEntryPaid,
    deleteEntry,
    pickRaffleWinner,
    verifyRaffleWinner,
    pickAnotherWinner,
  }
})
