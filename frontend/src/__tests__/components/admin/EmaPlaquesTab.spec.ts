import { describe, it, expect, beforeEach, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { EmaBoard, EmaPlaque } from '@/api/client'

// The tab reads through the API client; only its calls are faked (unwrap is the real one).
const ok = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })
const api = vi.hoisted(() => ({
  GET: vi.fn(),
  POST: vi.fn(),
  PUT: vi.fn(),
  DELETE: vi.fn(),
}))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import EmaPlaquesTab from '@/components/admin/EmaPlaquesTab.vue'
import EmaRequestPostModal from '@/components/admin/EmaRequestPostModal.vue'
import { useEmaStore } from '@/stores/ema'

function window(id: number, label: string, artistLimit: number, artistCount: number) {
  return {
    id,
    label,
    startAt: '2026-08-01T18:00:00Z',
    endAt: '2026-08-02T04:00:00Z',
    webhookUrl: 'https://discord.com/api/webhooks/1/x',
    artistLimit,
    artistCount,
    sortOrder: id,
  }
}

const board: EmaBoard = {
  id: 4,
  festivalMapId: 7,
  festivalMapName: 'Obon',
  title: 'Wishes',
  templateImage: '',
  instructions: '',
  discordLink: '',
  allowSelf: true,
  allowSite: true,
  allowArtist: true,
  artistName: 'Tara',
  status: 'open',
  createdAt: '2026-07-01T00:00:00Z',
  pendingCount: 0,
  requestedCount: 2,
  windows: [window(9, 'Day 1', 5, 2), window(10, 'Day 2', 0, 1)],
}

function plaque(over: Partial<EmaPlaque>): EmaPlaque {
  return {
    id: 1,
    boardId: 4,
    windowId: 9,
    windowLabel: 'Day 1',
    kind: 'artist',
    characterName: 'Aria',
    world: 'Gilgamesh',
    description: 'A koi under the moon',
    image: '',
    status: 'requested',
    reviewedBy: '',
    reviewedAt: null,
    createdAt: '2026-08-01T18:30:00Z',
    festivalMapName: 'Obon',
    ...over,
  }
}

const plaques: EmaPlaque[] = [
  plaque({ id: 1, characterName: 'Aria', description: 'A koi under the moon' }),
  plaque({
    id: 2,
    characterName: 'Bea',
    description: 'Two cranes',
    status: 'complete',
    reviewedBy: 'admin',
  }),
  plaque({
    id: 3,
    characterName: 'Cleo',
    description: 'A lantern',
    windowId: 10,
    windowLabel: 'Day 2',
  }),
  plaque({ id: 4, kind: 'site', characterName: 'Dara', description: '', status: 'posted' }),
]

/** Opens the board from the list, the way staff do. */
async function openBoard(): Promise<VueWrapper> {
  // The admin shell loads the boards when the tab opens; the tab only shows them.
  await useEmaStore().loadBoards()
  const wrapper = mount(EmaPlaquesTab)
  await flushPromises()
  await wrapper.find('.list-row button.btn-view').trigger('click')
  await flushPromises()
  return wrapper
}

/** What the artist-requests table lists, as one string per row. */
function requestRows(wrapper: VueWrapper): string[] {
  return wrapper.findAll('.ema-request-text').map((cell) => cell.element.closest('tr')!.textContent)
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  api.GET.mockImplementation(async (path: string) =>
    ok(
      path === '/ema-boards'
        ? { boards: [board] }
        : path === '/ema-boards/{id}'
          ? board
          : path === '/ema-boards/{id}/plaques'
            ? { plaques }
            : { maps: [] },
    ),
  )
})

describe("a board's artist requests", () => {
  it("list what is left to make, across the board's days, and none of the site plaques", async () => {
    const wrapper = await openBoard()
    const rows = requestRows(wrapper)
    expect(rows).toHaveLength(2)
    expect(rows[0]).toContain('Day 1')
    expect(rows[0]).toContain('Aria')
    expect(rows[1]).toContain('Day 2')
    expect(rows[1]).toContain('Cleo')
    expect(rows.join(' ')).not.toContain('Bea')
    expect(rows.join(' ')).not.toContain('Dara')
    expect(wrapper.text()).toContain('2 to make')
  })

  it('bring the finished ones back on request', async () => {
    const wrapper = await openBoard()
    await wrapper.find('.checkbox-inline input').setValue(true)
    expect(requestRows(wrapper).join(' ')).toContain('Bea')
    expect(wrapper.text()).toContain('3 requests')
  })

  it("narrow to one of the board's days", async () => {
    const wrapper = await openBoard()
    const day = wrapper.find('select[aria-label="Show the requests for a day"]')
    // Each day says how many requests it has taken, and its limit when it has one.
    expect(day.findAll('option').map((o) => o.text())).toEqual([
      'All days',
      'Day 1 (2 of 5)',
      'Day 2 (1)',
    ])
    await day.setValue(10)
    const rows = requestRows(wrapper)
    expect(rows).toHaveLength(1)
    expect(rows[0]).toContain('Cleo')
    expect(wrapper.text()).toContain('1 of 2 request')
  })

  it('open the dialog that posts the finished plaque', async () => {
    const wrapper = await openBoard()
    expect(wrapper.findComponent(EmaRequestPostModal).exists()).toBe(false)
    const first = wrapper.find('.ema-request-text').element.closest('tr')!
    const post = [...first.querySelectorAll('button')].find((b) => b.textContent.includes('Post'))!
    post.click()
    await flushPromises()
    const modal = wrapper.findComponent(EmaRequestPostModal)
    expect(modal.exists()).toBe(true)
    expect(modal.props('request').id).toBe(1)
  })

  it('say so when nobody has asked yet', async () => {
    api.GET.mockImplementation(async (path: string) =>
      ok(
        path === '/ema-boards'
          ? { boards: [board] }
          : path === '/ema-boards/{id}'
            ? board
            : { plaques: [plaques[3]] },
      ),
    )
    const wrapper = await openBoard()
    expect(wrapper.text()).toContain('No plaque requests yet.')
    expect(wrapper.find('.ema-request-text').exists()).toBe(false)
  })
})
