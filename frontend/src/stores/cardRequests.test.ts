import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { req } = vi.hoisted(() => ({ req: vi.fn() }))
vi.mock('@/lib/endpoints', () => ({
  endpoints: { cards: { request: req } },
}))

import { useCardRequestsStore } from '@/stores/cardRequests'
import { useUiStore } from '@/stores/ui'
import { randomBoard } from '@/lib/constants'

describe('cardRequests store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    req.mockReset()
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
    const notify = vi.spyOn(useUiStore(), 'notify').mockImplementation(() => {})
    await s.submit()
    expect(req).not.toHaveBeenCalled()
    expect(notify).toHaveBeenCalled()
  })

  it('submit() posts the request (uppercasing the id) and records the pending result', async () => {
    const s = useCardRequestsStore()
    vi.spyOn(useUiStore(), 'notify').mockImplementation(() => {})
    req.mockResolvedValue({ id: 'ABC123', status: 'pending' })
    s.characterName = 'Aria'
    s.world = 'Gilgamesh'
    s.cardId = 'abc123'
    s.board = randomBoard()
    await s.submit()
    expect(req).toHaveBeenCalledWith(
      expect.objectContaining({ character_name: 'Aria', world: 'Gilgamesh', card_id: 'ABC123' }),
    )
    expect(s.result).toEqual({ id: 'ABC123', status: 'pending' })
  })
})
