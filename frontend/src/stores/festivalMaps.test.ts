import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { FestivalMap, PublicFestivalMap, PublicFestivalMapSummary } from '@/types/api'

// Mock the typed endpoint layer so store actions run without the network.
const ep = vi.hoisted(() => ({
  list: vi.fn(async () => ({ maps: [] as FestivalMap[] })),
  detail: vi.fn(async () => ({ map: {} as FestivalMap })),
  create: vi.fn(async () => ({ map: {} as FestivalMap })),
  update: vi.fn(async () => ({ ok: true })),
  del: vi.fn(async () => undefined),
  setStatus: vi.fn(async () => ({ ok: true, status: 'published' })),
  publicList: vi.fn(async () => ({ maps: [] as PublicFestivalMapSummary[] })),
  publicDetail: vi.fn(async () => ({}) as PublicFestivalMap),
  affiliates: vi.fn(async () => ({ affiliates: [] })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    festivalMaps: {
      list: ep.list,
      detail: ep.detail,
      create: ep.create,
      update: ep.update,
      delete: ep.del,
      setStatus: ep.setStatus,
      publicList: ep.publicList,
      publicDetail: ep.publicDetail,
    },
    affiliates: { list: ep.affiliates },
  },
}))

import { useFestivalMapsStore } from './festivalMaps'
import { useUiStore } from './ui'

function makeMap(over: Partial<FestivalMap> = {}): FestivalMap {
  return {
    id: 7,
    title: 'Obon Matsuri 2026',
    slug: 'obon-2026',
    description: 'Come along.',
    times: [{ label: 'Day 1', start: '2026-08-01T18:00:00Z', end: '2026-08-01T22:00:00Z' }],
    map_image: 'images/festival_maps/plan.png',
    status: 'published',
    created_at: '2026-07-01T00:00:00Z',
    stalls: [
      {
        id: 11,
        map_id: 7,
        shape: 'rect',
        color: '#e0a480',
        placement: { x: 10, y: 20, width: 15, height: 10, rotation: 0 },
        sort_order: 0,
        occupants: [
          {
            id: 21,
            stall_id: 11,
            affiliate_id: 3,
            affiliate_name: 'Flora Teahouse',
            title: 'Flora Teahouse',
            description: 'Tea and cakes.',
            stall_type: 'food',
            type_label: '',
            times: [{ label: 'Day 1', start: '2026-08-01T19:00:00Z', end: '' }],
            sort_order: 0,
          },
        ],
      },
    ],
    ...over,
  }
}

/** The payload of the most recent create/update call. */
function lastPayload(fn: { mock: { calls: unknown[][] } }): Record<string, unknown> {
  return fn.mock.calls.at(-1)?.[0] as Record<string, unknown>
}

describe('festivalMaps store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
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
      const start = store.mapForm?.times[0].start ?? ''
      expect(start).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/)
      expect(start).not.toContain('Z')
    })

    it('gives every row a stable client key for the repeaters', () => {
      const store = useFestivalMapsStore()
      store.editMapForm(makeMap())
      expect(store.mapForm?.times[0]._uid).toBeTypeOf('number')
      expect(store.mapForm?.stalls[0]._uid).toBeTypeOf('number')
      expect(store.mapForm?.stalls[0].occupants[0]._uid).toBeTypeOf('number')
    })

    it("loads a pitch's occupants, keeping their ids so a rally keeps naming them", () => {
      const store = useFestivalMapsStore()
      store.editMapForm(makeMap())
      const occupants = store.mapForm?.stalls[0].occupants ?? []
      expect(occupants).toHaveLength(1)
      expect(occupants[0].id).toBe(21)
      expect(occupants[0].title).toBe('Flora Teahouse')
    })
  })

  describe('copyMapForm', () => {
    it('keeps the layout but drops every id and datetime from the run that happened', () => {
      const store = useFestivalMapsStore()
      store.copyMapForm(makeMap())
      const form = store.mapForm
      expect(form?.id).toBe(0)
      expect(form?.title).toBe('Obon Matsuri 2026 (Copy)')
      expect(form?.map_image).toBe('images/festival_maps/plan.png')
      // The pitch is still in place, with its position, but is a new row - and so
      // is everyone standing in it.
      expect(form?.stalls).toHaveLength(1)
      expect(form?.stalls[0].id).toBe(0)
      expect(form?.stalls[0].placement).toEqual({
        x: 10,
        y: 20,
        width: 15,
        height: 10,
        rotation: 0,
      })
      expect(form?.stalls[0].occupants[0].id).toBe(0)
      expect(form?.stalls[0].occupants[0].title).toBe('Flora Teahouse')
      // A stale window would mark the new festival as long over.
      expect(form?.stalls[0].occupants[0].times).toEqual([])
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
      expect(pitch?.occupants[0].stall_type).toBe('game')
    })
  })

  describe('occupants', () => {
    it('adds another occupant matching what the pitch already hosts', () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      store.addStall('food')
      const added = store.addOccupant(0)
      expect(added?.stall_type).toBe('food')
      expect(store.mapForm?.stalls[0].occupants).toHaveLength(2)
    })

    it('removes an occupant', () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      store.addStall('food')
      store.addOccupant(0)
      store.removeOccupant(0, 1)
      expect(store.mapForm?.stalls[0].occupants).toHaveLength(1)
    })

    it('refuses to remove the last one - a pitch with nobody in it is dropped on save', () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      store.addStall('food')
      store.removeOccupant(0, 0)
      expect(store.mapForm?.stalls[0].occupants).toHaveLength(1)
    })
  })

  describe('saveMap', () => {
    it('refuses a map with no title, without calling the API', async () => {
      const store = useFestivalMapsStore()
      const ui = useUiStore()
      store.newMapForm()
      expect(await store.saveMap()).toBe(false)
      expect(ep.create).not.toHaveBeenCalled()
      expect(ui.toast.message).toBe('Title is required')
    })

    it('drops a datetime range with no start - an unstarted range says nothing', async () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      store.mapForm!.title = 'Obon'
      store.mapForm!.times[0].start = ''
      store.mapForm!.times[0].label = 'Ghost'
      expect(await store.saveMap()).toBe(true)
      expect(lastPayload(ep.create).times).toEqual([])
    })

    it('sends UTC datetimes and no client-only row keys', async () => {
      const store = useFestivalMapsStore()
      store.newMapForm()
      store.mapForm!.title = 'Obon'
      store.mapForm!.times[0] = { label: 'Day 1', start: '2026-08-01T18:00', end: '', _uid: 99 }
      store.addStall('game')
      await store.saveMap()

      const payload = lastPayload(ep.create)
      const times = payload.times as Record<string, unknown>[]
      expect(times[0].start).toMatch(/Z$/)
      expect(times[0]).not.toHaveProperty('_uid')
      const pitch = (payload.stalls as Record<string, unknown>[])[0]
      expect(pitch).not.toHaveProperty('_uid')
      expect((pitch.occupants as Record<string, unknown>[])[0]).not.toHaveProperty('_uid')
    })

    it('updates rather than creates once the map has an id', async () => {
      const store = useFestivalMapsStore()
      store.editMapForm(makeMap())
      expect(await store.saveMap()).toBe(true)
      expect(ep.update).toHaveBeenCalled()
      expect(ep.create).not.toHaveBeenCalled()
    })
  })

  describe('setStatus', () => {
    it('applies the new status to the list row and the open map', async () => {
      const store = useFestivalMapsStore()
      const listed = makeMap({ status: 'in_progress' })
      store.maps = [listed]
      store.selectedMap = listed
      await store.setStatus(7, 'published')
      expect(store.maps[0].status).toBe('published')
      expect(store.selectedMap.status).toBe('published')
    })
  })

  describe('mapPath', () => {
    it('links a map by its shortcode when it has one, else by id', () => {
      const store = useFestivalMapsStore()
      expect(store.mapPath({ id: 7, slug: 'obon-2026' })).toBe('obon-2026')
      expect(store.mapPath({ id: 7, slug: '' })).toBe('7')
    })
  })

  describe('loadPublicMap', () => {
    it('reports false for a map the public endpoint refuses', async () => {
      ep.publicDetail.mockRejectedValueOnce(new Error('Festival map not found'))
      const store = useFestivalMapsStore()
      expect(await store.loadPublicMap('99')).toBe(false)
      expect(store.publicMap).toBeNull()
    })
  })
})
