import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// The store calls the API client; only its calls are faked (unwrap is the real one).
const ok = (data: unknown, status = 202) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({ POST: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { useCardRequestsStore } from '@/stores/cardRequests'
import { useUiStore } from '@/stores/ui'
import { randomBoard } from '@/lib/constants'

describe('cardRequests store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    api.POST.mockReset()
  })

  it('validate() flags missing fields, a bad id, then an incomplete board', () => {
    const s = useCardRequestsStore()
    expect(s.validate()).toMatch(/character name/i)
    s.characterName = 'Aria'
    s.world = 'Gilgamesh'
    expect(s.validate()).toMatch(/card id/i)
    s.cardId = 'ABC123'
    // A blank board (all zeros) is not yet a valid card.
    expect(s.validate()).not.toBe('')
    s.board = randomBoard()
    expect(s.validate()).toBe('')
  })

  it('submit() blocks and notifies when the form is invalid', async () => {
    const s = useCardRequestsStore()
    const notify = vi.spyOn(useUiStore(), 'notify').mockImplementation(() => undefined)
    await s.submit()
    expect(api.POST).not.toHaveBeenCalled()
    expect(notify).toHaveBeenCalled()
  })

  it('submit() posts the request (uppercasing the id) and records the pending result', async () => {
    const s = useCardRequestsStore()
    vi.spyOn(useUiStore(), 'notify').mockImplementation(() => undefined)
    api.POST.mockResolvedValue(ok({ id: 'ABC123', status: 'pending' }))
    s.characterName = 'Aria'
    s.world = 'Gilgamesh'
    s.cardId = 'abc123'
    s.board = randomBoard()
    s.turnstileToken = 'token'
    await s.submit()
    expect(api.POST).toHaveBeenCalledWith('/cards/request', {
      body: expect.objectContaining({
        characterName: 'Aria',
        world: 'Gilgamesh',
        cardId: 'ABC123',
        turnstileToken: 'token',
      }),
    })
    expect(s.result).toEqual({ id: 'ABC123', status: 'pending' })
    expect(s.turnstileToken).toBe('')
  })

  it("submit() shows the server's reason when it refuses the request", async () => {
    const s = useCardRequestsStore()
    const notify = vi.spyOn(useUiStore(), 'notify').mockImplementation(() => undefined)
    api.POST.mockResolvedValue({
      error: { status: 409, detail: 'That card ID is taken. Choose another.' },
      response: new Response(null, { status: 409 }),
    })
    s.characterName = 'Aria'
    s.world = 'Gilgamesh'
    s.cardId = 'ABC123'
    s.board = randomBoard()
    await s.submit()
    expect(notify).toHaveBeenCalledWith('That card ID is taken. Choose another.', 'error')
    expect(s.result).toBeNull()
  })
})
