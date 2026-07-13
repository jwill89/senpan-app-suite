/**
 * Affiliates store: admin management (list + create/edit/delete) of partner
 * establishments. Each affiliate has one or more owners, a location, opening
 * hours (multiple time ranges under a single timezone), markdown details, and a
 * logo + establishment screenshot picked from dedicated permanent image
 * categories (System → Images), exactly like raffle/garapon prize images.
 *
 * A lean cousin of the garapons store: no sub-tables, no public token view, no
 * status lifecycle — owners and hours are edited as repeatable form rows.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { detectTimezone } from '@/lib/constants'
import type { Affiliate, AffiliateForm, AffiliateHourForm, AffiliateOwnerForm } from '@/types/api'
import { useUiStore } from './ui'
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
  /** The single shared Discord webhook every affiliate posts to ('' = none). */
  const webhookUrl = ref('')

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
  const savingWebhook = ref(false)
  const postingId = ref<number | null>(null)

  // ── Load ───────────────────────────────────────────────────────────────────
  async function loadAffiliates(): Promise<void> {
    await withLoading(affiliatesLoading, async () => {
      const data = await endpoints.affiliates.list()
      affiliates.value = data.affiliates
      webhookUrl.value = data.webhook_url
    })
  }

  // ── Form ───────────────────────────────────────────────────────────────────
  function newAffiliateForm(): void {
    affiliateForm.value = {
      id: 0,
      name: '',
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
    )
      return
    postingId.value = a.id
    try {
      await endpoints.affiliates.post(a.id)
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
      const data = await endpoints.affiliates.setWebhook(url.trim())
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
