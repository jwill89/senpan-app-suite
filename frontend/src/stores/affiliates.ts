/**
 * Affiliates store: admin management (list + create/edit/delete) of partner
 * establishments. Each affiliate has one or more owners, a location, opening
 * hours (multiple time ranges under a single timezone), markdown details, and a
 * logo + establishment screenshot picked from dedicated permanent image
 * categories (System -> Images), exactly like raffle/garapon prize images.
 *
 * No sub-tables, no public token view and no status lifecycle: owners and hours
 * are edited as repeatable form rows.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, unwrap, type Affiliate, type AffiliateHour, type AffiliateInput } from '@/api/client'
import { detectTimezone } from '@/lib/constants'
import { useUiStore } from './ui'
import { nextUid } from '@/lib/uid'
import { withLoading } from '@/lib/withLoading'

/** An opening-hours row in the form. */
export interface AffiliateHourForm extends AffiliateHour {
  /** Client-only stable key for the hours repeater (see lib/uid.ts). */
  _uid?: number
}

/**
 * An owner row in the form. Owners are plain name strings on the wire, but the form
 * wraps each in an object so the repeater can key on a stable `_uid` instead of the
 * array index; a save sends the `value`s only.
 */
export interface AffiliateOwnerForm {
  value: string
  _uid?: number
}

/** The editable form model for an affiliate (id 0 = new). */
export interface AffiliateForm extends Omit<AffiliateInput, 'owners' | 'hours'> {
  id: number
  owners: AffiliateOwnerForm[]
  hours: AffiliateHourForm[]
}

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
  /** The single shared Discord webhook every affiliate posts to ('' = none). */
  const webhookUrl = ref('')

  const search = ref('')
  const filteredAffiliates = computed(() => {
    const q = search.value.trim().toLowerCase()
    if (!q) {
      return affiliates.value
    }
    return affiliates.value.filter((a) =>
      [a.name, a.location, ...a.owners].some((s) => s.toLowerCase().includes(q)),
    )
  })

  const affiliatesLoading = ref(false)
  const savingAffiliate = ref(false)
  const savingWebhook = ref(false)
  const postingId = ref<number | null>(null)

  // -- Load -------------------------------------------------------------------
  async function loadAffiliates(): Promise<void> {
    await withLoading(affiliatesLoading, async () => {
      const [list, hook] = await Promise.all([
        unwrap(api.GET('/affiliates')),
        unwrap(api.GET('/affiliates/webhook')),
      ])
      affiliates.value = list.affiliates
      webhookUrl.value = hook.webhookUrl
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
      embedColor: DEFAULT_EMBED_COLOR,
      discordLink: '',
      carrdLink: '',
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
      embedColor: a.embedColor || DEFAULT_EMBED_COLOR,
      discordLink: a.discordLink,
      carrdLink: a.carrdLink,
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
    if (!f || f.owners.length <= 1) {
      return
    }
    f.owners.splice(index, 1)
  }

  function addHour(): void {
    affiliateForm.value?.hours.push(blankHour())
  }
  function removeHour(index: number): void {
    const f = affiliateForm.value
    if (!f || f.hours.length <= 1) {
      return
    }
    f.hours.splice(index, 1)
  }

  /** Saves the affiliate form. Returns true on success (caller navigates back). */
  async function saveAffiliate(): Promise<boolean> {
    const f = affiliateForm.value
    if (!f) {
      return false
    }
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
      const { id, ...rest } = f
      const body: AffiliateInput = { ...rest, owners, hours }
      if (id) {
        await unwrap(api.PUT('/affiliates/{id}', { params: { path: { id } }, body }))
        ui.notify('Affiliate updated', 'success')
      } else {
        await unwrap(api.POST('/affiliates', { body }))
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
    ) {
      return
    }
    try {
      await unwrap(api.DELETE('/affiliates/{id}', { params: { path: { id } } }))
      affiliates.value = affiliates.value.filter((a) => a.id !== id)
      if (selectedAffiliate.value?.id === id) {
        selectedAffiliate.value = null
      }
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
      await unwrap(api.POST('/affiliates/reorder', { body: { orderedIds } }))
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      await loadAffiliates() // revert to the persisted order
    }
  }

  /** Post an affiliate's embed to the shared Discord webhook (after confirmation). */
  async function postAffiliate(a: Affiliate): Promise<void> {
    if (!webhookUrl.value.trim()) {
      ui.notify('Set a Discord webhook first (Webhook button above).', 'error')
      return
    }
    if (
      !(await ui.confirm(`Post "${a.name}" to Discord now?`, {
        title: 'Post affiliate',
        confirmText: 'Post',
      }))
    ) {
      return
    }
    postingId.value = a.id
    try {
      await unwrap(api.POST('/affiliates/{id}/post', { params: { path: { id: a.id } } }))
      ui.notify('Affiliate posted to Discord', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      postingId.value = null
    }
  }

  /** Save the shared Discord webhook. Returns true on success. */
  async function saveWebhook(url: string): Promise<boolean> {
    savingWebhook.value = true
    try {
      const data = await unwrap(
        api.PUT('/affiliates/webhook', { body: { webhookUrl: url.trim() } }),
      )
      webhookUrl.value = data.webhookUrl
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
    affiliates,
    selectedAffiliate,
    affiliateForm,
    webhookUrl,
    search,
    filteredAffiliates,
    affiliatesLoading,
    savingAffiliate,
    savingWebhook,
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
    saveWebhook,
  }
})
