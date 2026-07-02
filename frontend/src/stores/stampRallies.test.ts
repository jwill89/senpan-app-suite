import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { PublicStampCard, StampRally, StampRallyLogEntry } from '@/types/api'

// Mock the typed endpoint layer so store actions run without the network.
const ep = vi.hoisted(() => ({
  list: vi.fn(async () => ({ stamp_rallies: [] as StampRally[] })),
  create: vi.fn(async () => ({ ok: true })),
  update: vi.fn(async () => ({ ok: true })),
  stamp: vi.fn(async () => ({ card: {} as PublicStampCard, collected_stamp_id: 5 })),
  get: vi.fn(async () => ({}) as PublicStampCard),
  setStatus: vi.fn(async () => ({ ok: true })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    stampRallies: {
      list: ep.list,
      detail: vi.fn(),
      logs: vi.fn(),
      create: ep.create,
      update: ep.update,
      delete: vi.fn(),
      setStatus: ep.setStatus,
      setStampPaused: vi.fn(),
      createCard: vi.fn(),
      deleteCard: vi.fn(),
    },
    stampCard: { get: ep.get, stamp: ep.stamp },
    affiliates: { list: vi.fn(async () => ({ affiliates: [] })) },
  },
}))

import { useStampRalliesStore, groupedByParticipant } from './stampRallies'
import { useUiStore } from './ui'

function logRow(over: Partial<StampRallyLogEntry>): StampRallyLogEntry {
  return {
    card_id: 1,
    participant_name: 'A',
    stamp_id: 1,
    stall_name: 'X',
    stamped_at: '',
    ...over,
  }
}

function publicCard(over: Partial<PublicStampCard> = {}): PublicStampCard {
  return {
    rally: {
      id: 1,
      title: 'R',
      card_image: '',
      not_stamped_image: '',
      details: '',
      redeem_instructions: '',
      available_from: '',
      available_to: '',
      is_active: true,
    },
    participant_name: 'Tataru',
    completed: false,
    completed_at: '',
    stamps: [],
    prizes: [],
    prizes_revealed: false,
    ...over,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockClear())
})

describe('groupedByParticipant', () => {
  it('keeps each participant rows contiguous, groups ordered by first appearance', () => {
    const rows = [
      logRow({ participant_name: 'Bo', stamp_id: 1 }),
      logRow({ participant_name: 'Aria', stamp_id: 2 }),
      logRow({ participant_name: 'Bo', stamp_id: 3 }), // belongs with the first "Bo" block
      logRow({ participant_name: 'Aria', stamp_id: 4 }),
    ]
    const out = groupedByParticipant(rows)
    // Groups keyed on participant name (the snapshot, which survives card deletion).
    expect(out.map((r) => r.participant_name)).toEqual(['Bo', 'Bo', 'Aria', 'Aria'])
    // permutation: same length, same stamp ids present
    expect(out.map((r) => r.stamp_id).sort()).toEqual([1, 2, 3, 4])
  })
})

describe('admin', () => {
  it('loadRallies populates the list', async () => {
    ep.list.mockResolvedValueOnce({
      stamp_rallies: [{ id: 1 } as StampRally, { id: 2 } as StampRally],
    })
    const s = useStampRalliesStore()
    await s.loadRallies()
    expect(s.rallies).toHaveLength(2)
  })

  it('splits rallies into open and closed by status', () => {
    const s = useStampRalliesStore()
    s.rallies = [
      { id: 1, status: 'open' } as StampRally,
      { id: 2, status: 'closed' } as StampRally,
      { id: 3, status: 'open' } as StampRally,
    ]
    expect(s.openRallies.map((r) => r.id)).toEqual([1, 3])
    expect(s.closedRallies.map((r) => r.id)).toEqual([2])
  })

  it('setRallyStatus updates the selected + listed rally', async () => {
    const s = useStampRalliesStore()
    s.rallies = [{ id: 1, status: 'open' } as StampRally]
    s.selectedRally = s.rallies[0]
    await s.setRallyStatus(1, 'closed')
    expect(ep.setStatus).toHaveBeenCalledWith(1, 'closed')
    expect(s.rallies[0].status).toBe('closed')
    expect(s.selectedRally.status).toBe('closed')
  })

  it('setStampPausedInList toggles the loaded stall and adjusts the active count', async () => {
    const s = useStampRalliesStore()
    s.rallies = [{ id: 1, stamp_count: 2, active_stamp_count: 2 } as StampRally]
    s.cardStamps[1] = [
      { id: 10, paused: false },
      { id: 11, paused: false },
    ] as never
    await s.setStampPausedInList(1, 10, true)
    expect(s.cardStamps[1][0].paused).toBe(true)
    expect(s.rallies[0].active_stamp_count).toBe(1)
  })

  it('saveRally requires a title', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    s.newRallyForm()
    expect(await s.saveRally()).toBe(false)
    expect(ep.create).not.toHaveBeenCalled()
  })

  it('saveRally creates and clears the form', async () => {
    const s = useStampRalliesStore()
    s.newRallyForm()
    s.rallyForm!.title = 'Summer'
    expect(await s.saveRally()).toBe(true)
    expect(ep.create).toHaveBeenCalledTimes(1)
    expect(s.rallyForm).toBeNull()
  })
})

describe('public', () => {
  it('submitPassword commits the refreshed card + last collected id', async () => {
    const card = publicCard({ completed: false })
    ep.stamp.mockResolvedValueOnce({ card, collected_stamp_id: 9 })
    const s = useStampRalliesStore()
    const ok = await s.submitPassword('tok', 'alpha')
    expect(ok).toBe(true)
    expect(s.publicCard).toEqual(card)
    expect(s.lastCollectedId).toBe(9)
  })

  it('submitPassword rejects an empty password without calling the API', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    expect(await s.submitPassword('tok', '   ')).toBe(false)
    expect(ep.stamp).not.toHaveBeenCalled()
  })
})
