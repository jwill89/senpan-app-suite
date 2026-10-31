import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { EmaBoard, EmaPlaque } from '@/types/api'

const ep = vi.hoisted(() => ({
  list: vi.fn(),
  detail: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  plaques: vi.fn(),
  approve: vi.fn(),
  reject: vi.fn(),
  maps: vi.fn(),
  requests: vi.fn(),
  setComplete: vi.fn(),
  post: vi.fn(),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    emaBoards: {
      list: ep.list,
      detail: ep.detail,
      create: ep.create,
      update: ep.update,
      plaques: ep.plaques,
      approve: ep.approve,
      reject: ep.reject,
    },
    emaRequests: { list: ep.requests, setComplete: ep.setComplete, post: ep.post },
    festivalMaps: { list: ep.maps },
  },
}))

import { useEmaStore } from './ema'
import { useUiStore } from './ui'
import { utcToDatetimeLocal } from '@/lib/datetime'

function board(over: Partial<EmaBoard> = {}): EmaBoard {
  return {
    id: 4,
    festival_map_id: 7,
    title: 'Wishes',
    template_image: 'images/ema/t.png',
    instructions: '',
    discord_link: '',
    allow_self: true,
    allow_site: true,
    allow_artist: true,
    artist_name: 'Tara',
    status: 'open',
    created_at: '',
    windows: [
      {
        id: 9,
        board_id: 4,
        label: 'Day 1',
        start_at: '2026-08-01T18:00:00Z',
        end_at: '2026-08-02T04:00:00Z',
        webhook_url: 'https://discord.com/api/webhooks/1/x',
        artist_limit: 5,
        artist_count: 2,
        sort_order: 0,
      },
    ],
    ...over,
  }
}

function plaque(over: Partial<EmaPlaque> = {}): EmaPlaque {
  return {
    id: 11,
    board_id: 4,
    window_label: 'Day 1',
    kind: 'site',
    character_name: 'Aria',
    world: 'Gilgamesh',
    description: '',
    image: 'images/ema_plaques/4-x.png',
    status: 'pending',
    reviewed_by: '',
    reviewed_at: '',
    created_at: '',
    ...over,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockReset())
  ep.list.mockResolvedValue({ boards: [] })
  ep.maps.mockResolvedValue({ maps: [] })
})

describe('the board form', () => {
  it("round-trips a window's times through the local wall clock the inputs show", async () => {
    const ema = useEmaStore()
    ema.editBoardForm(board())
    const w = ema.boardForm!.windows[0]
    expect(w.start_at).toBe(utcToDatetimeLocal('2026-08-01T18:00:00Z'))

    ep.update.mockResolvedValue({ ok: true })
    expect(await ema.saveBoard()).toBe(4)
    const sent = ep.update.mock.calls[0][1] as {
      windows: { id: number; start_at: string; artist_limit: number }[]
    }
    // Back to the same UTC instant (the server normalizes the format), keeping the
    // window's id so its plaques stay attached.
    expect(sent.windows[0].id).toBe(9)
    expect(Date.parse(sent.windows[0].start_at)).toBe(Date.parse('2026-08-01T18:00:00Z'))
    // The limit goes back; the count of requests already made is the server's.
    expect(sent.windows[0]).toMatchObject({ artist_limit: 5 })
    expect(sent.windows[0]).not.toHaveProperty('artist_count')
    expect(ema.boardForm).toBeNull()
  })

  it('starts a new day where the last one ended, taking as many requests', () => {
    const ema = useEmaStore()
    ema.newBoardForm()
    ema.boardForm!.windows[0].end_at = '2026-08-02T04:00'
    ema.boardForm!.windows[0].artist_limit = 8
    ema.addWindow()
    expect(ema.boardForm!.windows[1].start_at).toBe('2026-08-02T04:00')
    expect(ema.boardForm!.windows[1].artist_limit).toBe(8)
  })

  it('keeps the form open when the server refuses it', async () => {
    const ema = useEmaStore()
    ema.newBoardForm()
    ep.create.mockRejectedValue(new Error('Day 1 and Day 2 overlap'))
    expect(await ema.saveBoard()).toBeNull()
    expect(ema.boardForm).not.toBeNull()
  })
})

describe('review', () => {
  it('swaps in the reviewed plaque and refreshes the counts', async () => {
    const ema = useEmaStore()
    ema.plaques = [plaque()]
    ep.approve.mockResolvedValue({ plaque: plaque({ status: 'posted', image: '' }) })
    await ema.review(ema.plaques[0], 'approve')
    expect(ep.approve).toHaveBeenCalledWith(4, 11)
    expect(ema.plaques[0].status).toBe('posted')
    expect(ema.pendingPlaques).toHaveLength(0)
    expect(ep.list).toHaveBeenCalled()
  })

  it('asks before rejecting, and does nothing on no', async () => {
    const ema = useEmaStore()
    const ui = useUiStore()
    vi.spyOn(ui, 'confirm').mockResolvedValue(false)
    ema.plaques = [plaque()]
    await ema.review(ema.plaques[0], 'reject')
    expect(ep.reject).not.toHaveBeenCalled()
  })

  it('counts what waits for review across boards', () => {
    const ema = useEmaStore()
    ema.boards = [board({ pending_count: 2 }), board({ id: 5, pending_count: 1 })]
    expect(ema.pendingTotal).toBe(3)
  })
})

describe("the artist's queue", () => {
  it('counts only the requests not yet done', () => {
    const ema = useEmaStore()
    ema.requests = [
      plaque({ kind: 'artist', status: 'requested' }),
      plaque({ id: 12, kind: 'artist', status: 'complete' }),
    ]
    expect(ema.openRequests).toHaveLength(1)
  })

  it('posts the finished plaque and swaps in the completed request', async () => {
    const ema = useEmaStore()
    const open = plaque({ kind: 'artist', status: 'requested', image: '' })
    ema.requests = [open]
    ep.post.mockResolvedValue({ plaque: { ...open, status: 'complete' } })
    const image = new File([new Uint8Array([1, 2, 3])], 'done.png', { type: 'image/png' })

    expect(await ema.postRequest(open, image)).toBe(true)
    const [id, form] = ep.post.mock.calls[0] as [number, FormData]
    expect(id).toBe(11)
    expect(form.get('image')).toBeInstanceOf(File)
    expect(ema.requests[0].status).toBe('complete')
    expect(ema.postingRequest).toBe(false)
  })

  it('keeps the request open, and re-reads the queue, when the post fails', async () => {
    const ema = useEmaStore()
    const open = plaque({ kind: 'artist', status: 'requested', image: '' })
    ema.requests = [open]
    ep.post.mockRejectedValue(new Error('Discord refused'))
    ep.requests.mockResolvedValue({ plaques: [open] })

    expect(await ema.postRequest(open, new File(['x'], 'done.png'))).toBe(false)
    expect(ep.requests).toHaveBeenCalled()
    expect(ema.requests[0].status).toBe('requested')
  })
})
