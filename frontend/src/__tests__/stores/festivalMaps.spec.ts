import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { FestivalMap } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({
  GET: vi.fn(),
  POST: vi.fn(),
  PUT: vi.fn(),
  PATCH: vi.fn(),
  DELETE: vi.fn(),
}))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { useFestivalMapsStore } from '@/stores/festivalMaps'
import { useUiStore } from '@/stores/ui'

function makeMap(over: Partial<FestivalMap> = {}): FestivalMap {
  return {
    id: 7,
    title: 'Obon Matsuri 2026',
    slug: 'obon-2026',
    publicPath: 'obon-2026',
    description: 'Come along.',
    times: [{ label: 'Day 1', start: '2026-08-01T18:00:00Z', end: '2026-08-01T22:00:00Z' }],
    mapImage: 'images/festival_maps/plan.png',
    status: 'published',
    createdAt: '2026-07-01T00:00:00Z',
    stallCount: 1,
    stalls: [
      {
        id: 11,
        shape: 'rect',
        color: '#e0a480',
        selectionColor: '',
        textColor: '',
        placement: { x: 10, y: 20, width: 15, height: 10, rotation: 0 },
        sortOrder: 0,
        occupants: [
          {
            id: 21,
            affiliateId: 3,
            affiliateName: 'Flora Teahouse',
            title: 'Flora Teahouse',
            description: 'Tea and cakes.',
            eventCarrd: '',
            stallType: 'food',
            typeLabel: '',
            times: [{ label: 'Day 1', start: '2026-08-01T19:00:00Z', end: null }],
            sortOrder: 0,
          },
        ],
      },
    ],
    ...over,
  }
}

/** The body of the most recent call a method made. */
function lastPayload(fn: { mock: { calls: unknown[][] } }): Record<string, unknown> {
  return (fn.mock.calls.at(-1)?.[1] as { body: Record<string, unknown> }).body
}

describe('festivalMaps store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(api).forEach((fn) => fn.mockReset())
    api.POST.mockResolvedValue(ok({ id: 7 }, 201))
    api.PUT.mockResolvedValue(ok({}))
    api.PATCH.mockResolvedValue(ok({}))
    api.DELETE.mockResolvedValue(ok(null, 204))
    api.GET.mockImplementation(async (path: string) =>
      ok(path === '/affiliates' ? { affiliates: [] } : { maps: [], ...makeMap() }),
    )
  })

  describe('status buckets', () => {
    it('splits the list by publish state', () => {
      const store = useFestivalMapsStore()
      store.maps = [
        makeMap({ id: 1, status: 'published' }),
        makeMap({ id: 2, status: 'in_progress' }),
        makeMap({ id: 3, status: 'closed' }),
      ]
      expect(store.publishedMaps.map((m) => m.id)).toEqual([1])
      expect(store.draftMaps.map((m) => m.id)).toEqual([2])
      expect(store.closedMaps.map((m) => m.id)).toEqual([3])
    })
  })

  describe('editMapForm', () => {
    it('converts stored UTC ranges into local datetime-local values', () => {
      const store = useFestivalMapsStore()
      store.editMapForm(makeMap())
      // The exact local string depends on the runner's zone; what matters is
      // that it is the `datetime-local` shape, not the stored RFC-3339 one.
      const start = store.mapForm?.times[0]!.start ?? ''
      expect(start).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/)
      expect(start).not.toContain('Z')
    })

    it('gives every row a stable client key for the repeaters', () => {
      const store = useFestivalMapsStore()
      store.editMapForm(makeMap())
      expect(store.mapForm?.times[0]!._uid).toBeTypeOf('number')
      expect(store.mapForm?.stalls[0]!._uid).toBeTypeOf('number')
      expect(store.mapForm?.stalls[0]!.occupants[0]!._uid).toBeTypeOf('number')
    })

    it("loads a pitch's occupants, keeping their ids so a rally keeps naming them", () => {
      const store = useFestivalMapsStore()
      store.editMapForm(makeMap())
      const occupants = store.mapForm?.stalls[0]!.occupants ?? []
      expect(occupants).toHaveLength(1)
      expect(occupants[0]!.id).toBe(21)
      expect(occupants[0]!.title).toBe('Flora Teahouse')
    })
  })

  describe('copyMapForm', () => {
    it('keeps the layout but drops every id and datetime from the run that happened', () => {
      const store = useFestivalMapsStore()
      store.copyMapForm(makeMap())
      const form = store.mapForm
      expect(form?.id).toBe(0)
      expect(form?.title).toBe('Obon Matsuri 2026 (Copy)')
      expect(form?.mapImage).toBe('images/festival_maps/plan.png')
      // The pitch is still in place, with its position, but is a new row - and so
      // is everyone standing in it.
      expect(form?.stalls).toHaveLength(1)
      expect(form?.stalls[0]!.id).toBe(0)
      expect(form?.stalls[0]!.placement).toEqual({
        x: 10,
        y: 20,
        width: 15,
        height: 10,
        rotation: 0,
      })
      expect(form?.stalls[0]!.occupants[0]!.id).toBe(0)
      expect(form?.stalls[0]!.occupants[0]!.title).toBe('Flora Teahouse')
      // A stale window would mark the new festival as long over.
      expect(form?.stalls[0]!.occupants[0]!.times).toEqual([])
      expect(form?.times.every((t) => t.start === '')).toBe(true)
      // A shortcode names exactly one map - keeping it would be refused on save,
      // and would take over the URL the original is already linked by.
      expect(form?.slug).toBe('')
    })
  })

  describe('addStall', () => {
    it('shapes a new pitch the way its type is usually drawn', () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      expect(store.addStall('game')?.shape).toBe('circle')
      expect(store.addStall('food')?.shape).toBe('rect')
    })

    it('leaves the color empty so the pitch follows its type until one is picked', () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      expect(store.addStall('game')?.color).toBe('')
    })

    it('starts a pitch with one occupant - the common case, one business all festival', () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      const pitch = store.addStall('game')
      expect(pitch?.occupants).toHaveLength(1)
      expect(pitch?.occupants[0]!.stallType).toBe('game')
    })
  })

  describe('occupants', () => {
    it('adds another occupant matching what the pitch already hosts', () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      store.addStall('food')
      const added = store.addOccupant(0)
      expect(added?.stallType).toBe('food')
      expect(store.mapForm?.stalls[0]!.occupants).toHaveLength(2)
    })

    it('removes an occupant', () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      store.addStall('food')
      store.addOccupant(0)
      store.removeOccupant(0, 1)
      expect(store.mapForm?.stalls[0]!.occupants).toHaveLength(1)
    })

    it('refuses to remove the last one - a pitch with nobody in it is dropped on save', () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      store.addStall('food')
      store.removeOccupant(0, 0)
      expect(store.mapForm?.stalls[0]!.occupants).toHaveLength(1)
    })
  })

  describe('saveMap', () => {
    it('refuses a map with no title, without calling the API', async () => {
      const store = useFestivalMapsStore()
      const ui = useUiStore()
      store.newMapForm()
      expect(await store.saveMap()).toBe(false)
      expect(api.POST).not.toHaveBeenCalled()
      expect(ui.toast.message).toBe('Title is required')
    })

    /**
     * The map labels a stall by its AFFILIATE, so one saved without leaves a blank
     * shape on the plan. There is no default to fall back on either: the venue is
     * an affiliate in its own right now, with its own logo and links, rather than a
     * hard-coded name in the form.
     */
    it('refuses a stall whose occupant names no affiliate', async () => {
      const store = useFestivalMapsStore()
      const ui = useUiStore()
      store.newMapForm()
      store.mapForm!.title = 'Obon'
      store.addStall('food')
      store.mapForm!.stalls[0]!.occupants[0]!.affiliateId = null

      expect(await store.saveMap()).toBe(false)
      expect(api.POST).not.toHaveBeenCalled()
      expect(ui.toast.message).toMatch(/affiliate/i)
    })

    it('saves once every occupant names one', async () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      store.mapForm!.title = 'Obon'
      store.addStall('food')
      store.mapForm!.stalls[0]!.occupants[0]!.affiliateId = 7
      expect(await store.saveMap()).toBe(true)
    })

    it('drops a datetime range with no start - an unstarted range says nothing', async () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      store.mapForm!.title = 'Obon'
      store.mapForm!.times[0]!.start = ''
      store.mapForm!.times[0]!.label = 'Ghost'
      expect(await store.saveMap()).toBe(true)
      expect(lastPayload(api.POST).times).toEqual([])
    })

    /**
     * The payload used to be built from an explicit list of fields, which had to be
     * extended every time the editor gained one - and three were missed: the event
     * Carrd link, the selection colour and the label colour. Each was set in the
     * editor, saved without complaint, and gone on reload.
     *
     * Asserted field-by-field against the form rather than by naming today's
     * fields, so a field added tomorrow is covered without anyone remembering to
     * come back here.
     */
    it('sends every field the editor holds, not a hand-maintained subset', async () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      store.mapForm!.title = 'Obon'
      store.addStall('food')

      const stall = store.mapForm!.stalls[0]!
      stall.occupants[0]!.affiliateId = 7 // required before a map will save
      stall.color = '#112233'
      stall.selectionColor = '#ff3ea5'
      stall.textColor = '#ffe9a8'
      stall.occupants[0]!.eventCarrd = 'https://ev.carrd.co'
      stall.occupants[0]!.title = 'Tea Bar'
      await store.saveMap()

      const pitch = (lastPayload(api.POST).stalls as Record<string, unknown>[])[0]!
      const occupant = (pitch.occupants as Record<string, unknown>[])[0]!
      // Every form key except the client-only row id must reach the wire.
      for (const [key, value] of Object.entries(stall)) {
        if (key === '_uid' || key === 'occupants') {
          continue
        }
        expect({ key, value: pitch[key] }).toEqual({ key, value })
      }
      for (const [key, value] of Object.entries(stall.occupants[0]!)) {
        if (key === '_uid' || key === 'times') {
          continue
        }
        expect({ key, value: occupant[key] }).toEqual({ key, value })
      }
    })

    it('sends UTC datetimes and no client-only row keys', async () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      store.mapForm!.title = 'Obon'
      store.mapForm!.times[0] = { label: 'Day 1', start: '2026-08-01T18:00', end: '', _uid: 99 }
      store.addStall('game')
      store.mapForm!.stalls[0]!.occupants[0]!.affiliateId = 7 // required before a map will save
      await store.saveMap()

      const payload = lastPayload(api.POST)
      const times = payload.times as Record<string, unknown>[]
      expect(times[0]!.start).toMatch(/Z$/)
      expect(times[0]).not.toHaveProperty('_uid')
      const pitch = (payload.stalls as Record<string, unknown>[])[0]!
      expect(pitch).not.toHaveProperty('_uid')
      expect((pitch.occupants as Record<string, unknown>[])[0]).not.toHaveProperty('_uid')
    })

    it('updates rather than creates once the map has an id', async () => {
      const store = useFestivalMapsStore()
      store.editMapForm(makeMap())
      expect(await store.saveMap()).toBe(true)
      expect(api.PUT).toHaveBeenCalled()
      expect(api.POST).not.toHaveBeenCalled()
    })
  })

  describe('setStatus', () => {
    it('applies the new status to the list row and the open map', async () => {
      const store = useFestivalMapsStore()
      const listed = makeMap({ status: 'in_progress' })
      store.maps = [listed]
      store.selectedMap = listed
      await store.setStatus(7, 'published')
      expect(store.maps[0]!.status).toBe('published')
      expect(store.selectedMap.status).toBe('published')
    })
  })

  describe('mapPath', () => {
    it('links a map by the segment the server names: its shortcode, else its id', () => {
      const store = useFestivalMapsStore()
      expect(store.mapPath({ publicPath: 'obon-2026' })).toBe('obon-2026')
      expect(store.mapPath({ publicPath: '7' })).toBe('7')
    })
  })

  describe('loadPublicMap', () => {
    it('reports false for a map the public endpoint refuses', async () => {
      api.GET.mockResolvedValueOnce({
        error: { title: 'Not Found', detail: 'There is no such festival map.' },
        response: new Response(null, { status: 404 }),
      })
      const store = useFestivalMapsStore()
      expect(await store.loadPublicMap('99')).toBe(false)
      expect(store.publicMap).toBeNull()
    })
  })
})
