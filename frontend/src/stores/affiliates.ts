/**
 * Affiliates store: admin management (list + create/edit/delete) of partner
 * establishments. Each affiliate has one or more owners, a location, opening
 * hours (multiple time ranges under a single timezone), markdown details, and a
 * logo + establishment screenshot picked from dedicated permanent image
 * categories (System -> Images), exactly like raffle/garapon prize images.
 *
 * A lean cousin of the garapons store: no sub-tables, no public token view, no
 * status lifecycle - owners and hours are edited as repeatable form rows. Each
 * affiliate can be posted to Discord and kept there, exactly like a tea room.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { detectTimezone } from '@/lib/constants'
import type {
  Affiliate,
  AffiliateForm,
  AffiliateHourForm,
  AffiliateOwnerForm,
  DiscordDestination,
  RepostMode,
} from '@/types/api'
import { useUiStore } from './ui'
import { useDiscordStore } from './discord'
import { postVerb, setPostState } from '@/lib/discordPosts'
import { nextUid } from '@/lib/uid'
import { withLoading } from '@/lib/withLoading'

/** Default embed accent (brand red) for a new affiliate, matching Tea Rooms. */
const DEFAULT_EMBED_COLOR = '#ff3131'

/** A fresh, empty opening-hours row for the editor. */
function blankHour(): AffiliateHourForm {
  return { label: '', start: '', end: '', _uid: nextUid() }
}

/** An owner form row wrapping a plain name so the repeater can key on `_uid`. */
function ownerRow(value = ''): AffiliateOwnerForm {
  return { value, _uid: nextUid() }
}

export const useAffiliatesStore = defineStore('affiliates', () => {
  const ui = useUiStore()

  const affiliates = ref<Affiliate[]>([])
  const selectedAffiliate = ref<Affiliate | null>(null)
  const affiliateForm = ref<AffiliateForm | null>(null)
  /**
   * Where affiliates post in Discord. Null until loaded, and for the festival editors,
   * which read the list only to name a stall's operator.
   */
  const destination = ref<DiscordDestination | null>(null)

  const search = ref('')
  const filteredAffiliates = computed(() => {
    const q = search.value.trim().toLowerCase()
    if (!q) return affiliates.value
    return affiliates.value.filter((a) =>
      [a.name, a.location, ...a.owners].some((s) => s.toLowerCase().includes(q)),
    )
  })

  const affiliatesLoading = ref(false)
  const savingAffiliate = ref(false)
  const savingDestination = ref(false)
  const postingId = ref<number | null>(null)

  // -- Load -------------------------------------------------------------------
  async function loadAffiliates(): Promise<void> {
    await withLoading(affiliatesLoading, async () => {
      const data = await endpoints.affiliates.list()
      affiliates.value = data.affiliates
      destination.value = data.destination ?? null
    })
  }

  // -- Form -------------------------------------------------------------------
  function newAffiliateForm(): void {
    affiliateForm.value = {
      id: 0,
      name: '',
      subtitle: '',
      owners: [ownerRow()],
      location: '',
      timezone: detectTimezone(),
      hours: [blankHour()],
      details: '',
      logo: '',
      screenshot: '',
      embed_color: DEFAULT_EMBED_COLOR,
      discord_link: '',
      carrd_link: '',
    }
  }

  function editAffiliateForm(a: Affiliate): void {
    affiliateForm.value = {
      id: a.id,
      name: a.name,
      subtitle: a.subtitle,
      owners: a.owners.length ? a.owners.map((o) => ownerRow(o)) : [ownerRow()],
      location: a.location,
      timezone: a.timezone || detectTimezone(),
      hours: a.hours.length
        ? a.hours.map((h) => ({ label: h.label, start: h.start, end: h.end, _uid: nextUid() }))
        : [blankHour()],
      details: a.details,
      logo: a.logo,
      screenshot: a.screenshot,
      embed_color: a.embed_color || DEFAULT_EMBED_COLOR,
      discord_link: a.discord_link,
      carrd_link: a.carrd_link,
    }
  }

  function cancelAffiliateForm(): void {
    affiliateForm.value = null
  }

  function addOwner(): void {
    affiliateForm.value?.owners.push(ownerRow())
  }
  function removeOwner(index: number): void {
    const f = affiliateForm.value
    if (!f || f.owners.length <= 1) return
    f.owners.splice(index, 1)
  }

  function addHour(): void {
    affiliateForm.value?.hours.push(blankHour())
  }
  function removeHour(index: number): void {
    const f = affiliateForm.value
    if (!f || f.hours.length <= 1) return
    f.hours.splice(index, 1)
  }

  /** Saves the affiliate form. Returns true on success (caller navigates back). */
  async function saveAffiliate(): Promise<boolean> {
    const f = affiliateForm.value
    if (!f) return false
    if (!f.name.trim()) {
      ui.notify('Name is required', 'error')
      return false
    }
    // Drop blank repeater rows before sending (the backend also sanitizes). Owners
    // are unwrapped from their form rows back to plain name strings.
    const owners = f.owners.map((o) => o.value.trim()).filter((o) => o)
    if (!owners.length) {
      ui.notify('Add at least one owner', 'error')
      return false
    }
    const hours = f.hours
      .filter((h) => h.start.trim())
      .map((h) => ({ label: h.label.trim(), start: h.start.trim(), end: h.end.trim() }))

    savingAffiliate.value = true
    try {
      const payload = { ...f, owners, hours }
      if (f.id) {
        await endpoints.affiliates.update(f.id, payload)
        ui.notify('Affiliate updated', 'success')
      } else {
        await endpoints.affiliates.create(payload)
        ui.notify('Affiliate created', 'success')
      }
      affiliateForm.value = null
      await loadAffiliates()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingAffiliate.value = false
    }
  }

  async function deleteAffiliate(id: number): Promise<void> {
    if (
      !(await ui.confirm('Delete this affiliate?', {
        title: 'Delete affiliate',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.affiliates.delete(id)
      affiliates.value = affiliates.value.filter((a) => a.id !== id)
      if (selectedAffiliate.value?.id === id) selectedAffiliate.value = null
      ui.notify('Affiliate deleted', 'info')
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
      await endpoints.affiliates.reorder(orderedIds)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      await loadAffiliates() // revert to the persisted order
    }
  }

  /**
   * Post an affiliate to Discord (after confirmation) - or, when the affiliates' channel
   * edits in place and the affiliate has a post up, update that post.
   */
  async function postAffiliate(a: Affiliate): Promise<void> {
    if (!destination.value?.channel_id) {
      ui.notify('Pick a Discord channel first (Discord button above).', 'error')
      return
    }
    const verb = postVerb(destination.value, a.discord_post)
    const question =
      verb === 'Update' ? `Update "${a.name}" in Discord?` : `Post "${a.name}" to Discord now?`
    if (!(await ui.confirm(question, { title: `${verb} affiliate`, confirmText: verb }))) return
    postingId.value = a.id
    try {
      const res = await endpoints.affiliates.post(a.id)
      setPostState(affiliates.value, a.id, res.post)
      ui.notify(
        res.note || (verb === 'Update' ? 'Discord post updated' : 'Affiliate posted to Discord'),
        'success',
      )
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      postingId.value = null
    }
  }

  /** Delete an affiliate's post from Discord (after confirmation). */
  async function takeDownAffiliate(a: Affiliate): Promise<void> {
    if (
      !(await ui.confirm(`Delete the Discord post for "${a.name}"?`, {
        title: 'Take down',
        confirmText: 'Take down',
      }))
    )
      return
    postingId.value = a.id
    try {
      const res = await endpoints.affiliates.takeDown(a.id)
      setPostState(affiliates.value, a.id, res.post)
      ui.notify(res.note || 'Discord post deleted', res.note ? 'info' : 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      postingId.value = null
    }
  }

  /** Save where affiliates post. Returns true on success. */
  async function saveDestination(dest: {
    channel_id: string
    mention: string
    repost_mode: RepostMode
  }): Promise<boolean> {
    savingDestination.value = true
    try {
      const saved = await useDiscordStore().saveDestination('affiliates', dest)
      if (saved) destination.value = saved
      return saved !== null
    } finally {
      savingDestination.value = false
    }
  }

  return {
    affiliates,
    selectedAffiliate,
    affiliateForm,
    destination,
    search,
    filteredAffiliates,
    affiliatesLoading,
    savingAffiliate,
    savingDestination,
    postingId,
    loadAffiliates,
    newAffiliateForm,
    editAffiliateForm,
    cancelAffiliateForm,
    addOwner,
    removeOwner,
    addHour,
    removeHour,
    saveAffiliate,
    deleteAffiliate,
    reorder,
    postAffiliate,
    takeDownAffiliate,
    saveDestination,
  }
})
