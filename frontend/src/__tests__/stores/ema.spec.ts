import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { EmaBoard, EmaPlaque } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real one).
// Each fake answers a call's `{ data, error, response }`. The finished plaque is a
// file upload, which goes through the XHR upload helper.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const problem = (detail: string, status = 409) => ({
  error: { title: 'Conflict', detail },
  response: new Response(null, { status }),
})
const api = vi.hoisted(() => ({
  GET: vi.fn(),
  POST: vi.fn(),
  PUT: vi.fn(),
  DELETE: vi.fn(),
}))
const apiUpload = vi.hoisted(() => vi.fn())
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  apiUpload,
}))

import { useEmaStore } from '@/stores/ema'
import { useUiStore } from '@/stores/ui'
import { utcToDatetimeLocal } from '@/lib/datetime'

/** The call to `path` among a method's calls, as `[path, init]`. */
function callTo(method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string) {
  return api[method].mock.calls.find(([p]) => p === path) as
    | [string, { params: { path: Record<string, number> }; body: Record<string, unknown> }]
    | undefined
}

function board(over: Partial<EmaBoard> = {}): EmaBoard {
  return {
    id: 4,
    festivalMapId: 7,
    festivalMapName: 'Obon',
    title: 'Wishes',
    templateImage: 'images/ema/t.png',
    instructions: '',
    discordLink: '',
    allowSelf: true,
    allowSite: true,
    allowArtist: true,
    artistName: 'Tara',
    status: 'open',
    createdAt: '2026-07-01T00:00:00Z',
    pendingCount: 0,
    requestedCount: 0,
    windows: [
      {
        id: 9,
        label: 'Day 1',
        startAt: '2026-08-01T18:00:00Z',
        endAt: '2026-08-02T04:00:00Z',
        webhookUrl: 'https://discord.com/api/webhooks/1/x',
        artistLimit: 5,
        artistCount: 2,
        sortOrder: 0,
      },
    ],
    ...over,
  }
}

function plaque(over: Partial<EmaPlaque> = {}): EmaPlaque {
  return {
    id: 11,
    boardId: 4,
    windowId: 9,
    windowLabel: 'Day 1',
    kind: 'site',
    characterName: 'Aria',
    world: 'Gilgamesh',
    description: '',
    image: 'images/ema_plaques/4-x.png',
    status: 'pending',
    reviewedBy: '',
    reviewedAt: null,
    createdAt: '2026-08-01T18:30:00Z',
    festivalMapName: 'Obon',
    ...over,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  apiUpload.mockReset()
  api.GET.mockImplementation(async (path: string) =>
    ok(path === '/ema-boards' ? { boards: [] } : { maps: [] }),
  )
})

describe('the board form', () => {
  it("round-trips a window's times through the local wall clock the inputs show", async () => {
    const ema = useEmaStore()
    ema.editBoardForm(board())
    const w = ema.boardForm!.windows[0]!
    expect(w.startAt).toBe(utcToDatetimeLocal('2026-08-01T18:00:00Z'))

    api.PUT.mockResolvedValue(ok(board()))
    expect(await ema.saveBoard()).toBe(4)
    const [, init] = callTo('PUT', '/ema-boards/{id}')!
    expect(init.params.path.id).toBe(4)
    const sent = init.body as {
      windows: { id: number; startAt: string; artistLimit: number }[]
    }
    // Back to the same UTC instant (the server normalizes the format), keeping the
    // window's id so its plaques stay attached.
    expect(sent.windows[0]!.id).toBe(9)
    expect(Date.parse(sent.windows[0]!.startAt)).toBe(Date.parse('2026-08-01T18:00:00Z'))
    // The limit goes back; the count of requests already made is the server's.
    expect(sent.windows[0]).toMatchObject({ artistLimit: 5 })
    expect(sent.windows[0]).not.toHaveProperty('artistCount')
    expect(ema.boardForm).toBeNull()
  })

  it('starts a new day where the last one ended, taking as many requests', () => {
    const ema = useEmaStore()
    ema.newBoardForm()
    ema.boardForm!.windows[0]!.endAt = '2026-08-02T04:00'
    ema.boardForm!.windows[0]!.artistLimit = 8
    ema.addWindow()
    expect(ema.boardForm!.windows[1]!.startAt).toBe('2026-08-02T04:00')
    expect(ema.boardForm!.windows[1]!.artistLimit).toBe(8)
  })

  it('keeps the form open when the server refuses it', async () => {
    const ema = useEmaStore()
    ema.newBoardForm()
    api.POST.mockResolvedValue(problem('Day 1 and Day 2 overlap', 422))
    expect(await ema.saveBoard()).toBeNull()
    expect(ema.boardForm).not.toBeNull()
  })
})

describe('review', () => {
  it('swaps in the reviewed plaque and refreshes the counts', async () => {
    const ema = useEmaStore()
    ema.plaques = [plaque()]
    api.POST.mockResolvedValue(ok(plaque({ status: 'posted', image: '' })))
    await ema.review(ema.plaques[0]!, 'approve')
    const [, init] = callTo('POST', '/ema-boards/{id}/plaques/{plaqueId}/approve')!
    expect(init.params.path).toEqual({ id: 4, plaqueId: 11 })
    expect(ema.plaques[0]!.status).toBe('posted')
    expect(ema.pendingPlaques).toHaveLength(0)
    expect(callTo('GET', '/ema-boards')).toBeDefined()
  })

  it('asks before rejecting, and does nothing on no', async () => {
    const ema = useEmaStore()
    const ui = useUiStore()
    vi.spyOn(ui, 'confirm').mockResolvedValue(false)
    ema.plaques = [plaque()]
    await ema.review(ema.plaques[0]!, 'reject')
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('counts what waits on staff across boards: plaques to review and requests to make', () => {
    const ema = useEmaStore()
    ema.boards = [
      board({ pendingCount: 2, requestedCount: 1 }),
      board({ id: 5, pendingCount: 1, requestedCount: 0 }),
    ]
    expect(ema.waitingTotal).toBe(4)
  })

  it('says why a post failed and re-reads the plaques', async () => {
    const ema = useEmaStore()
    const ui = useUiStore()
    ema.selectedBoard = board()
    ema.plaques = [plaque()]
    api.POST.mockResolvedValue(problem('Someone else is reviewing this plaque.'))
    await ema.review(ema.plaques[0]!, 'approve')
    expect(ui.toast.message).toBe('Someone else is reviewing this plaque.')
    expect(callTo('GET', '/ema-boards/{id}/plaques')).toBeDefined()
  })
})

describe("the artist's requests", () => {
  it("are the open board's artist plaques, whatever their day or state", () => {
    const ema = useEmaStore()
    ema.plaques = [
      plaque({ kind: 'artist', status: 'requested' }),
      plaque({ id: 12, kind: 'artist', status: 'complete', windowId: 10, windowLabel: 'Day 2' }),
      plaque({ id: 13, kind: 'site' }),
    ]
    expect(ema.artistRequests.map((p) => p.id)).toEqual([11, 12])
  })

  it('marks a request done and reopens it through its own commands', async () => {
    const ema = useEmaStore()
    const open = plaque({ kind: 'artist', status: 'requested', image: '' })
    ema.plaques = [open]
    api.POST.mockResolvedValueOnce(ok({ ...open, status: 'complete' }))
    await ema.setRequestComplete(open, true)
    expect(callTo('POST', '/ema-requests/{id}/complete')![1].params.path.id).toBe(11)
    expect(ema.plaques[0]!.status).toBe('complete')

    api.POST.mockResolvedValueOnce(ok({ ...open, status: 'requested' }))
    await ema.setRequestComplete(ema.plaques[0]!, false)
    expect(callTo('POST', '/ema-requests/{id}/reopen')).toBeDefined()
    expect(ema.plaques[0]!.status).toBe('requested')
  })

  it('posts the finished plaque and swaps in the completed request', async () => {
    const ema = useEmaStore()
    const open = plaque({ kind: 'artist', status: 'requested', image: '' })
    ema.plaques = [open]
    apiUpload.mockResolvedValue({ ...open, status: 'complete' })
    const image = new File([new Uint8Array([1, 2, 3])], 'done.png', { type: 'image/png' })

    expect(await ema.postRequest(open, image)).toBe(true)
    const [endpoint, form] = apiUpload.mock.calls[0] as [string, FormData]
    expect(endpoint).toBe('ema-requests/11/post')
    expect(form.get('image')).toBeInstanceOf(File)
    expect(ema.plaques[0]!.status).toBe('complete')
    expect(ema.postingRequest).toBe(false)
  })

  it("keeps the request open, and re-reads the board's plaques, when the post fails", async () => {
    const ema = useEmaStore()
    const open = plaque({ kind: 'artist', status: 'requested', image: '' })
    ema.selectedBoard = board()
    ema.plaques = [open]
    apiUpload.mockRejectedValue(new Error('Discord refused'))
    api.GET.mockResolvedValue(ok({ plaques: [open] }))

    expect(await ema.postRequest(open, new File(['x'], 'done.png'))).toBe(false)
    expect(callTo('GET', '/ema-boards/{id}/plaques')).toBeDefined()
    expect(ema.plaques[0]!.status).toBe('requested')
  })
})
