import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock the low-level client so we can assert on the exact path / body / options
// each typed endpoint produces, without touching the network. `vi.hoisted` lets
// the spies be referenced inside the hoisted `vi.mock` factory.
const { apiGet, apiPost } = vi.hoisted(() => ({
  apiGet: vi.fn(async () => ({})),
  apiPost: vi.fn(async () => ({})),
}))
vi.mock('./api', () => ({ apiGet, apiPost }))

import { endpoints } from './endpoints'

beforeEach(() => {
  apiGet.mockClear()
  apiPost.mockClear()
})

describe('board.get', () => {
  it('URL-encodes the card id', async () => {
    await endpoints.board.get('a b/c')
    expect(apiGet).toHaveBeenCalledWith('board?id=a%20b%2Fc')
  })

  it('adds the preview flag when requested', async () => {
    await endpoints.board.get('XYZ', { preview: true })
    expect(apiGet).toHaveBeenCalledWith('board?id=XYZ&preview=1')
  })
})

describe('auth', () => {
  it('login posts the username + password and opts out of the global 401 redirect', async () => {
    await endpoints.auth.login('yao', 'hunter2')
    expect(apiPost).toHaveBeenCalledWith(
      'auth',
      { action: 'login', username: 'yao', password: 'hunter2' },
      { skipAuthRedirect: true },
    )
  })

  it('register posts the username + password and opts out of the global 401 redirect', async () => {
    await endpoints.auth.register('yao', 'hunter2')
    expect(apiPost).toHaveBeenCalledWith(
      'register',
      { username: 'yao', password: 'hunter2' },
      { skipAuthRedirect: true },
    )
  })

  it('logout opts out of the global 401 redirect', async () => {
    await endpoints.auth.logout()
    expect(apiPost).toHaveBeenCalledWith('auth', { action: 'logout' }, { skipAuthRedirect: true })
  })
})

describe('users + account', () => {
  it('setPermissions posts the action with the permission keys', async () => {
    await endpoints.users.setPermissions(3, ['bingo-game', 'bingo-cards'])
    expect(apiPost).toHaveBeenCalledWith('users', {
      action: 'set_permissions',
      id: 3,
      permissions: ['bingo-game', 'bingo-cards'],
    })
  })

  it('changePassword posts the current + new password', async () => {
    await endpoints.account.changePassword('old', 'new')
    expect(apiPost).toHaveBeenCalledWith('account', {
      action: 'change_password',
      current_password: 'old',
      new_password: 'new',
    })
  })
})

describe('game lifecycle', () => {
  it('start sends the selected pattern ids', async () => {
    await endpoints.game.start([1, 2, 3])
    expect(apiPost).toHaveBeenCalledWith('game', { action: 'start', pattern_ids: [1, 2, 3] })
  })

  it('draw sends the configured delay', async () => {
    await endpoints.game.draw(5)
    expect(apiPost).toHaveBeenCalledWith('game', { action: 'draw', delay: 5 })
  })

  it('end sends the verified winner ids', async () => {
    await endpoints.game.end(['AAA', 'BBB'])
    expect(apiPost).toHaveBeenCalledWith('game', { action: 'end', valid_winner_ids: ['AAA', 'BBB'] })
  })
})

describe('winnersLog.list', () => {
  it('builds the paginated/sorted query string', async () => {
    await endpoints.winnersLog.list({ page: 2, perPage: 25, sort: 'date', dir: 'desc' })
    expect(apiGet).toHaveBeenCalledWith('winners-log?page=2&per_page=25&sort=date&dir=desc')
  })
})

describe('raffles nested paths', () => {
  it('detail embeds the raffle id in the path', async () => {
    await endpoints.raffles.detail(7)
    expect(apiGet).toHaveBeenCalledWith('raffles/7')
  })

  it('enter posts to the raffle-scoped entries path', async () => {
    const body = { character_name: 'Cloud', world: 'Gaia', num_entries: 2 }
    await endpoints.raffles.enter(7, body)
    expect(apiPost).toHaveBeenCalledWith('raffles/7/enter', body)
  })

  it('addEntry posts the add_entry action with the paid flag', async () => {
    await endpoints.raffles.addEntry(7, {
      character_name: 'Cloud',
      world: 'Gaia',
      num_entries: 2,
      paid: true,
    })
    expect(apiPost).toHaveBeenCalledWith('raffles/7/entries', {
      action: 'add_entry',
      character_name: 'Cloud',
      world: 'Gaia',
      num_entries: 2,
      paid: true,
    })
  })

  it('markEntryPaid posts the mark_paid action', async () => {
    await endpoints.raffles.markEntryPaid(7, 42, false)
    expect(apiPost).toHaveBeenCalledWith('raffles/7/entries', {
      action: 'mark_paid',
      entry_id: 42,
      paid: false,
    })
  })
})
