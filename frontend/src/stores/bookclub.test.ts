import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { ReadingList, ReadingListItem } from '@/types/api'

const ep = vi.hoisted(() => ({
  lists: vi.fn(async () => ({ reading_lists: [] as ReadingList[] })),
  listDetail: vi.fn(async () => ({ reading_list: { id: 1, items: [] } })),
  createList: vi.fn(async () => ({ reading_list: { id: 2, title: 'New' } })),
  saveItem: vi.fn(async () => ({ ok: true })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    bookclub: {
      lists: ep.lists,
      listDetail: ep.listDetail,
      createList: ep.createList,
      saveItem: ep.saveItem,
    },
  },
}))

import { useBookclubStore } from './bookclub'

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockClear())
})

describe('club selection (computed labels)', () => {
  it('defaults to the first club', () => {
    const s = useBookclubStore()
    expect(s.activeClubSlug).toBe('yaoi')
    expect(s.clubName).toBe('Yaoi Book Club')
    expect(s.commentsLabel).toBe("Yao's Comments")
  })
})

describe('openClub (per-club freshness)', () => {
  it('loads lists on first entry and skips a quick re-entry', () => {
    const s = useBookclubStore()
    s.openClub('yaoi')
    expect(ep.lists).toHaveBeenCalledTimes(1)
    s.openClub('yaoi') // within TTL → no refetch
    expect(ep.lists).toHaveBeenCalledTimes(1)
  })

  it('switching clubs clears the open list and refetches', () => {
    const s = useBookclubStore()
    s.openClub('yaoi')
    s.selectedList = { id: 9 } as ReadingList
    s.openClub('yuri')
    expect(s.activeClubSlug).toBe('yuri')
    expect(s.selectedList).toBeNull()
    expect(ep.lists).toHaveBeenCalledTimes(2)
    expect(s.clubName).toBe('Yuri Book Club')
  })
})

describe('applyExternalChange (live invalidation)', () => {
  it('refetches the open club (+ open list detail) when viewing', () => {
    const s = useBookclubStore()
    s.selectedList = { id: 4 } as ReadingList
    s.applyExternalChange(true)
    expect(ep.lists).toHaveBeenCalled()
    expect(ep.listDetail).toHaveBeenCalledWith('yaoi', 4)
  })

  it('only invalidates when not viewing, so the next entry refetches', () => {
    const s = useBookclubStore()
    s.openClub('yaoi') // stamps freshness
    ep.lists.mockClear()
    s.applyExternalChange(false) // not viewing → invalidate, no immediate fetch
    expect(ep.lists).not.toHaveBeenCalled()
    s.openClub('yaoi') // now stale again → refetch
    expect(ep.lists).toHaveBeenCalledTimes(1)
  })
})

describe('createList', () => {
  it('rejects a blank title', async () => {
    const s = useBookclubStore()
    s.newListTitle = '   '
    await s.createList()
    expect(ep.createList).not.toHaveBeenCalled()
  })

  it('creates with the active club and trimmed title', async () => {
    const s = useBookclubStore()
    s.newListTitle = '  Summer Reads  '
    await s.createList()
    expect(ep.createList).toHaveBeenCalledWith('yaoi', 'Summer Reads')
    expect(s.newListTitle).toBe('')
  })
})

describe('item form', () => {
  it('addSourceRow / removeSourceRow manage the sources array', () => {
    const s = useBookclubStore()
    s.addSourceRow()
    s.addSourceRow()
    expect(s.itemForm.sources).toHaveLength(2)
    s.removeSourceRow(0)
    expect(s.itemForm.sources).toHaveLength(1)
  })

  it('editItem deep-copies the sources so edits do not mutate the original', () => {
    const s = useBookclubStore()
    const item = { id: 1, title: 'T', sources: [{ title: 'A', url: 'u' }] } as ReadingListItem
    s.editItem(item)
    s.itemForm.sources[0].url = 'changed'
    expect(item.sources[0].url).toBe('u')
  })

  it('applyLookupResult fills the form but preserves id, tropes, and comments', () => {
    const s = useBookclubStore()
    s.itemForm.id = 5
    s.itemForm.tropes = 'enemies-to-lovers'
    s.itemForm.comments = 'curator note'
    s.applyLookupResult({ title: 'Found', summary: 'sum', sources: [] } as never)
    expect(s.itemForm.title).toBe('Found')
    expect(s.itemForm.id).toBe(5)
    expect(s.itemForm.tropes).toBe('enemies-to-lovers')
    expect(s.itemForm.comments).toBe('curator note')
  })
})

describe('saveItem', () => {
  it('requires a title', async () => {
    const s = useBookclubStore()
    s.selectedList = { id: 1 } as ReadingList
    s.itemForm.title = '   '
    await s.saveItem()
    expect(ep.saveItem).not.toHaveBeenCalled()
  })

  it('saves with blank-URL source rows filtered out, then reloads detail', async () => {
    const s = useBookclubStore()
    s.selectedList = { id: 3 } as ReadingList
    s.itemForm.title = 'My Item'
    s.itemForm.sources = [
      { title: 'Keep', url: 'https://x' },
      { title: 'Drop', url: '   ' },
    ]
    await s.saveItem()
    const call = ep.saveItem.mock.calls[0] as unknown[]
    expect(call[0]).toBe('yaoi') // club threaded through as the first arg
    expect(call[1]).toBe(3) // the open list's id
    const payload = call[2] as { sources: unknown[] }
    expect(payload.sources).toHaveLength(1)
    expect(ep.listDetail).toHaveBeenCalledWith('yaoi', 3)
  })
})
