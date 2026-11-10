import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { ReadingList, ReadingListItem, ReadingListSummary } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const failed = (detail: string, status = 500) => ({
  error: { detail },
  response: new Response(null, { status }),
})
const api = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PUT: vi.fn(), DELETE: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { useBookclubStore } from '@/stores/bookclub'
import { useUiStore } from '@/stores/ui'

const HOOK = 'https://discord.com/api/webhooks/1/x'

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  api.GET.mockImplementation(async (path: string, init?: { params: { path: { id: number } } }) => {
    if (path === '/book-clubs/{club}/webhook') {
      return ok({ webhookUrl: HOOK })
    }
    if (path === '/book-clubs/{club}/reading-lists/{id}') {
      return ok({ id: init?.params.path.id, club: 'yaoi', title: 'L', createdAt: null, items: [] })
    }
    return ok({ readingLists: [] })
  })
  api.POST.mockResolvedValue(ok({}, 201))
  api.PUT.mockResolvedValue(ok({}))
  api.DELETE.mockResolvedValue(ok(undefined, 204))
})

/** The calls made to a method's path. */
const calls = (method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string) =>
  api[method].mock.calls.filter((c) => c[0] === path)

const listsPath = '/book-clubs/{club}/reading-lists'

describe('club selection (computed labels)', () => {
  it('defaults to the first club', () => {
    const s = useBookclubStore()
    expect(s.activeClubSlug).toBe('yaoi')
    expect(s.clubName).toBe('Yaoi Book Club')
    expect(s.commentsLabel).toBe("Yao's Comments")
  })
})

describe('openClub (per-club freshness)', () => {
  it('loads lists and the webhook on first entry and skips a quick re-entry', async () => {
    const s = useBookclubStore()
    s.openClub('yaoi')
    expect(calls('GET', listsPath)).toHaveLength(1)
    expect(calls('GET', listsPath)[0]![1]).toEqual({ params: { path: { club: 'yaoi' } } })
    await vi.waitFor(() => expect(s.webhookUrl).toBe(HOOK))
    s.openClub('yaoi') // within TTL -> no refetch
    expect(calls('GET', listsPath)).toHaveLength(1)
  })

  it('switching clubs clears the open list and refetches', () => {
    const s = useBookclubStore()
    s.openClub('yaoi')
    s.selectedList = { id: 9 } as ReadingList
    s.openClub('yuri')
    expect(s.activeClubSlug).toBe('yuri')
    expect(s.selectedList).toBeNull()
    expect(calls('GET', listsPath)).toHaveLength(2)
    expect(calls('GET', listsPath)[1]![1]).toEqual({ params: { path: { club: 'yuri' } } })
    expect(s.clubName).toBe('Yuri Book Club')
  })
})

describe('applyExternalChange (live invalidation)', () => {
  it('refetches the open club (+ open list detail) when viewing', () => {
    const s = useBookclubStore()
    s.selectedList = { id: 4 } as ReadingList
    s.applyExternalChange('yaoi', true)
    expect(calls('GET', listsPath)).toHaveLength(1)
    expect(calls('GET', `${listsPath}/{id}`)[0]![1]).toEqual({
      params: { path: { club: 'yaoi', id: 4 } },
    })
  })

  it('only invalidates when not viewing, so the next entry refetches', () => {
    const s = useBookclubStore()
    s.openClub('yaoi') // stamps freshness
    api.GET.mockClear()
    s.applyExternalChange('yaoi', false) // not viewing -> invalidate, no immediate fetch
    expect(calls('GET', listsPath)).toHaveLength(0)
    s.openClub('yaoi') // now stale again -> refetch
    expect(calls('GET', listsPath)).toHaveLength(1)
  })

  it('invalidates the club the change names, not the open one', () => {
    const s = useBookclubStore()
    s.openClub('yuri')
    s.openClub('yaoi')
    api.GET.mockClear()
    s.applyExternalChange('yuri', false)
    expect(calls('GET', listsPath)).toHaveLength(0)
    s.openClub('yuri')
    expect(calls('GET', listsPath)).toHaveLength(1)
  })
})

describe('lists', () => {
  it('createList rejects a blank title', async () => {
    const s = useBookclubStore()
    s.newListTitle = '   '
    await s.createList()
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('createList creates with the active club and trimmed title, then opens the list', async () => {
    api.POST.mockResolvedValueOnce(
      ok({ id: 2, club: 'yaoi', title: 'Summer Reads', createdAt: null, items: [] }, 201),
    )
    const s = useBookclubStore()
    s.newListTitle = '  Summer Reads  '
    await s.createList()
    expect(calls('POST', listsPath)[0]![1]).toEqual({
      params: { path: { club: 'yaoi' } },
      body: { title: 'Summer Reads' },
    })
    expect(s.newListTitle).toBe('')
    expect(s.selectedList?.id).toBe(2)
  })

  it('selectList shows the list at once and reads its items', () => {
    const s = useBookclubStore()
    s.selectList({ id: 6, club: 'yaoi', title: 'L', createdAt: null })
    expect(s.selectedList).toMatchObject({ id: 6, items: [] })
    expect(calls('GET', `${listsPath}/{id}`)).toHaveLength(1)
  })

  it('renameList sends the new title and keeps the open list in step', async () => {
    const s = useBookclubStore()
    const list = { id: 4, club: 'yaoi', title: 'Old', createdAt: null } as ReadingListSummary
    s.selectedList = { ...list, items: [] }
    expect(await s.renameList(list, '  New  ')).toBe(true)
    expect(calls('PUT', `${listsPath}/{id}`)[0]![1]).toEqual({
      params: { path: { club: 'yaoi', id: 4 } },
      body: { title: 'New' },
    })
    expect(list.title).toBe('New')
    expect(s.selectedList.title).toBe('New')
  })

  it('deleteList removes the list after confirming', async () => {
    const ui = useUiStore()
    vi.spyOn(ui, 'confirm').mockResolvedValue(true)
    const s = useBookclubStore()
    const list = { id: 4, club: 'yaoi', title: 'L', createdAt: null } as ReadingListSummary
    s.lists = [list]
    await s.deleteList(list)
    expect(calls('DELETE', `${listsPath}/{id}`)[0]![1]).toEqual({
      params: { path: { club: 'yaoi', id: 4 } },
    })
    expect(s.lists).toEqual([])
  })
})

describe("the club's webhook", () => {
  it('saveWebhook sends the trimmed address for the active club', async () => {
    api.PUT.mockResolvedValueOnce(ok({ webhookUrl: HOOK }))
    const s = useBookclubStore()
    s.activeClubSlug = 'yuri'
    expect(await s.saveWebhook(`  ${HOOK}  `)).toBe(true)
    expect(calls('PUT', '/book-clubs/{club}/webhook')[0]![1]).toEqual({
      params: { path: { club: 'yuri' } },
      body: { webhookUrl: HOOK },
    })
    expect(s.webhookUrl).toBe(HOOK)
    expect(s.savingWebhook).toBe(false)
  })

  it('saveWebhook keeps the old address and says why when it is refused', async () => {
    api.PUT.mockResolvedValueOnce(failed("The webhook can't be saved as it is.", 422))
    const s = useBookclubStore()
    s.webhookUrl = HOOK
    expect(await s.saveWebhook('https://evil.example/hook')).toBe(false)
    expect(s.webhookUrl).toBe(HOOK)
  })
})

describe('publishList', () => {
  const list = { id: 4, club: 'yaoi', title: 'L', createdAt: null } as ReadingListSummary

  it('posts the list after confirming', async () => {
    api.POST.mockResolvedValueOnce(ok({ published: 3 }))
    const ui = useUiStore()
    vi.spyOn(ui, 'confirm').mockResolvedValue(true)
    const s = useBookclubStore()
    s.webhookUrl = HOOK
    await s.publishList(list)
    expect(calls('POST', `${listsPath}/{id}/publish`)[0]![1]).toEqual({
      params: { path: { club: 'yaoi', id: 4 } },
    })
    expect(ui.toast.message).toBe('Published 3 item(s) to Discord')
    expect(s.publishing).toBe(false)
  })

  it('asks for a webhook first when the club has none', async () => {
    const s = useBookclubStore()
    await s.publishList(list)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('shows how far a failed run got', async () => {
    api.POST.mockResolvedValueOnce(
      failed('Posted 2 of 5, then: Discord is limiting posts right now.', 429),
    )
    const ui = useUiStore()
    vi.spyOn(ui, 'confirm').mockResolvedValue(true)
    const s = useBookclubStore()
    s.webhookUrl = HOOK
    await s.publishList(list)
    expect(ui.toast.message).toBe('Posted 2 of 5, then: Discord is limiting posts right now.')
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
    s.itemForm.sources[0]!.url = 'changed'
    expect(item.sources[0]!.url).toBe('u')
  })

  it('applyLookupResult fills the form but preserves id, tropes, and comments', () => {
    const s = useBookclubStore()
    s.itemForm.id = 5
    s.itemForm.tropes = 'enemies-to-lovers'
    s.itemForm.comments = 'curator note'
    s.applyLookupResult({ title: 'Found', summary: 'sum', coverImage: 'c', sources: [] } as never)
    expect(s.itemForm.title).toBe('Found')
    expect(s.itemForm.coverImage).toBe('c')
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
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('adds with blank-URL source rows filtered out, then reloads detail', async () => {
    const s = useBookclubStore()
    s.selectedList = { id: 3 } as ReadingList
    s.itemForm.title = ' My Item '
    s.itemForm.sources = [
      { title: 'Keep', url: 'https://x', _uid: 1 },
      { title: 'Drop', url: '   ' },
    ]
    await s.saveItem()
    const sent = calls('POST', `${listsPath}/{id}/items`)[0]![1]
    expect(sent.params).toEqual({ path: { club: 'yaoi', id: 3 } }) // club and the open list's id
    expect(sent.body.title).toBe('My Item')
    expect(sent.body.sources).toEqual([{ title: 'Keep', url: 'https://x' }]) // no _uid
    expect(sent.body).not.toHaveProperty('id')
    expect(calls('GET', `${listsPath}/{id}`)).toHaveLength(1)
  })

  it('replaces an item that has an id through its own path', async () => {
    const s = useBookclubStore()
    s.selectedList = { id: 3 } as ReadingList
    s.itemForm.id = 9
    s.itemForm.title = 'Edited'
    await s.saveItem()
    expect(calls('PUT', `${listsPath}/{id}/items/{itemId}`)[0]![1].params).toEqual({
      path: { club: 'yaoi', id: 3, itemId: 9 },
    })
    expect(api.POST).not.toHaveBeenCalled()
  })
})

describe('runLookup', () => {
  it("searches the active club's lookup with the trimmed query", async () => {
    api.GET.mockImplementation(async () => ok({ results: [{ title: 'Found' }] }))
    const s = useBookclubStore()
    s.lookupQuery = '  painter  '
    await s.runLookup()
    expect(calls('GET', '/book-clubs/{club}/lookup')[0]![1]).toEqual({
      params: { path: { club: 'yaoi' }, query: { q: 'painter' } },
    })
    expect(s.lookupResults).toEqual([{ title: 'Found' }])
    expect(s.looking).toBe(false)
  })

  it('shows why AniList could not answer', async () => {
    api.GET.mockResolvedValue(
      failed("AniList can't be reached right now: it did not respond.", 503),
    )
    const ui = useUiStore()
    const s = useBookclubStore()
    s.lookupQuery = 'x'
    await s.runLookup()
    expect(ui.toast.message).toBe("AniList can't be reached right now: it did not respond.")
    expect(s.lookupResults).toEqual([])
  })
})
