import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock the low-level client so we can assert on the exact path / body / options
// each typed endpoint produces, without touching the network. `vi.hoisted` lets
// the spies be referenced inside the hoisted `vi.mock` factory.
const { apiGet, apiPost, apiPut, apiPatch, apiDelete } = vi.hoisted(() => ({
  apiGet: vi.fn(async () => ({})),
  apiPost: vi.fn(async () => ({})),
  apiPut: vi.fn(async () => ({})),
  apiPatch: vi.fn(async () => ({})),
  apiDelete: vi.fn(async () => ({})),
}))
vi.mock('./api', () => ({ apiGet, apiPost, apiPut, apiPatch, apiDelete }))

import { endpoints } from './endpoints'

beforeEach(() => {
  apiGet.mockClear()
  apiPost.mockClear()
  apiPut.mockClear()
  apiPatch.mockClear()
  apiDelete.mockClear()
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

describe('users + account (hybrid REST)', () => {
  it('setActive PATCHes the user resource with the active flag', async () => {
    await endpoints.users.setActive(3, true)
    expect(apiPatch).toHaveBeenCalledWith('users/3', { active: true })
  })

  it('setAdmin PATCHes the user resource with the admin flag', async () => {
    await endpoints.users.setAdmin(3, false)
    expect(apiPatch).toHaveBeenCalledWith('users/3', { admin: false })
  })

  it('setPermissions PATCHes the user resource with the permission keys', async () => {
    await endpoints.users.setPermissions(3, ['bingo-game', 'bingo-cards'])
    expect(apiPatch).toHaveBeenCalledWith('users/3', {
      permissions: ['bingo-game', 'bingo-cards'],
    })
  })

  it('setPassword PATCHes the user resource with the new password', async () => {
    await endpoints.users.setPassword(3, 'hunter2secret')
    expect(apiPatch).toHaveBeenCalledWith('users/3', { password: 'hunter2secret' })
  })

  it('delete DELETEs the user resource by id', async () => {
    await endpoints.users.delete(3)
    expect(apiDelete).toHaveBeenCalledWith('users/3')
  })

  it('changePassword POSTs the current + new password to the change-password sub-path', async () => {
    await endpoints.account.changePassword('old', 'new')
    expect(apiPost).toHaveBeenCalledWith('account/change-password', {
      current_password: 'old',
      new_password: 'new',
    })
  })

  it('generateToken POSTs to the token resource with no action field', async () => {
    await endpoints.account.generateToken()
    expect(apiPost).toHaveBeenCalledWith('account/token', {})
  })

  it('revokeToken DELETEs the token resource', async () => {
    await endpoints.account.revokeToken()
    expect(apiDelete).toHaveBeenCalledWith('account/token')
  })
})

describe('game lifecycle (hybrid REST)', () => {
  it('start POSTs the selected pattern ids to the /start sub-path', async () => {
    await endpoints.game.start([1, 2, 3])
    expect(apiPost).toHaveBeenCalledWith('game/start', { pattern_ids: [1, 2, 3] })
  })

  it('draw POSTs the configured delay to the /draw sub-path', async () => {
    await endpoints.game.draw(5)
    expect(apiPost).toHaveBeenCalledWith('game/draw', { delay: 5 })
  })

  it('end POSTs the verified winner ids to the /end sub-path', async () => {
    await endpoints.game.end(['AAA', 'BBB'])
    expect(apiPost).toHaveBeenCalledWith('game/end', { valid_winner_ids: ['AAA', 'BBB'] })
  })

  it('triggerHalftime POSTs to the /halftime sub-path', async () => {
    await endpoints.game.triggerHalftime()
    expect(apiPost).toHaveBeenCalledWith('game/halftime', undefined)
  })

  it('setDelay PATCHes the shared draw delay', async () => {
    await endpoints.game.setDelay(15)
    expect(apiPatch).toHaveBeenCalledWith('game', { delay: 15 })
  })

  it('updateDetails PATCHes the game details', async () => {
    await endpoints.game.updateDetails('GL HF')
    expect(apiPatch).toHaveBeenCalledWith('game', { details: 'GL HF' })
  })
})

describe('cards (hybrid REST)', () => {
  it('create POSTs the player name to the cards collection', async () => {
    await endpoints.cards.create('Aerith')
    expect(apiPost).toHaveBeenCalledWith('cards', { player_name: 'Aerith' })
  })

  it('generate POSTs the count to the /generate sub-path', async () => {
    await endpoints.cards.generate(10)
    expect(apiPost).toHaveBeenCalledWith('cards/generate', { count: 10 })
  })

  it('delete DELETEs the URL-encoded card id', async () => {
    await endpoints.cards.delete('a b/c')
    expect(apiDelete).toHaveBeenCalledWith('cards/a%20b%2Fc')
  })

  it('deleteAll DELETEs the /all bulk sub-path', async () => {
    await endpoints.cards.deleteAll()
    expect(apiDelete).toHaveBeenCalledWith('cards/all')
  })

  it('updatePlayer PATCHes the card resource with the player name + details', async () => {
    await endpoints.cards.updatePlayer('XYZ', 'Tifa', 'VIP')
    expect(apiPatch).toHaveBeenCalledWith('cards/XYZ', { player_name: 'Tifa', details: 'VIP' })
  })
})

describe('winnersLog (hybrid REST)', () => {
  it('list builds the paginated/sorted query string', async () => {
    await endpoints.winnersLog.list({ page: 2, perPage: 25, sort: 'date', dir: 'desc' })
    expect(apiGet).toHaveBeenCalledWith('winners-log?page=2&per_page=25&sort=date&dir=desc')
  })

  it('delete DELETEs the entry resource by id', async () => {
    await endpoints.winnersLog.delete(42)
    expect(apiDelete).toHaveBeenCalledWith('winners-log/42')
  })

  it('deleteAll DELETEs the /all bulk sub-path', async () => {
    await endpoints.winnersLog.deleteAll()
    expect(apiDelete).toHaveBeenCalledWith('winners-log/all')
  })
})

describe('patterns (hybrid REST)', () => {
  it('create POSTs the pattern fields to the collection', async () => {
    const grid = [[true]]
    await endpoints.patterns.create('Top Row', grid, 2)
    expect(apiPost).toHaveBeenCalledWith('patterns', {
      name: 'Top Row',
      pattern_data: grid,
      category_id: 2,
    })
  })

  it('delete DELETEs the pattern resource by id', async () => {
    await endpoints.patterns.delete(9)
    expect(apiDelete).toHaveBeenCalledWith('patterns/9')
  })

  it('rename PATCHes the pattern resource with the new name', async () => {
    await endpoints.patterns.rename(9, 'Renamed')
    expect(apiPatch).toHaveBeenCalledWith('patterns/9', { name: 'Renamed' })
  })

  it('setCategory PATCHes the pattern resource with the category id', async () => {
    await endpoints.patterns.setCategory(9, 3)
    expect(apiPatch).toHaveBeenCalledWith('patterns/9', { category_id: 3 })
  })

  it('reorder PATCHes the pattern resource with the direction', async () => {
    await endpoints.patterns.reorder(9, 'up')
    expect(apiPatch).toHaveBeenCalledWith('patterns/9', { direction: 'up' })
  })

  it('bulkReorder POSTs the category + ordered ids to the /reorder sub-path', async () => {
    await endpoints.patterns.bulkReorder(2, [3, 1, 2])
    expect(apiPost).toHaveBeenCalledWith('patterns/reorder', {
      category_id: 2,
      ordered_ids: [3, 1, 2],
    })
  })
})

describe('pattern categories (hybrid REST)', () => {
  it('create POSTs the name to the collection', async () => {
    await endpoints.patternCategories.create('Bonus')
    expect(apiPost).toHaveBeenCalledWith('pattern-categories', { name: 'Bonus' })
  })

  it('rename PATCHes the category resource with the new name', async () => {
    await endpoints.patternCategories.rename(4, 'Bonus Patterns')
    expect(apiPatch).toHaveBeenCalledWith('pattern-categories/4', { name: 'Bonus Patterns' })
  })

  it('delete DELETEs the category resource by id', async () => {
    await endpoints.patternCategories.delete(4)
    expect(apiDelete).toHaveBeenCalledWith('pattern-categories/4')
  })

  it('reorder PATCHes the category resource with the direction', async () => {
    await endpoints.patternCategories.reorder(4, 'down')
    expect(apiPatch).toHaveBeenCalledWith('pattern-categories/4', { direction: 'down' })
  })

  it('bulkReorder POSTs the ordered ids to the /reorder sub-path', async () => {
    await endpoints.patternCategories.bulkReorder([3, 1, 2])
    expect(apiPost).toHaveBeenCalledWith('pattern-categories/reorder', { ordered_ids: [3, 1, 2] })
  })
})

describe('styles (hybrid REST)', () => {
  it('get GETs the style resource by id', async () => {
    await endpoints.styles.get(5)
    expect(apiGet).toHaveBeenCalledWith('styles/5')
  })

  it('create POSTs the theme fields to the collection', async () => {
    await endpoints.styles.create('Midnight', { 'page-bg': '#000' }, 'b.svg', 'n.svg')
    expect(apiPost).toHaveBeenCalledWith('styles', {
      name: 'Midnight',
      tokens: { 'page-bg': '#000' },
      board_flourish: 'b.svg',
      number_flourish: 'n.svg',
    })
  })

  it('update PUTs the style resource by id', async () => {
    await endpoints.styles.update(5, 'Midnight', { accent: '#fff' })
    expect(apiPut).toHaveBeenCalledWith('styles/5', {
      name: 'Midnight',
      tokens: { accent: '#fff' },
      board_flourish: '',
      number_flourish: '',
    })
  })

  it('delete DELETEs the style resource by id', async () => {
    await endpoints.styles.delete(5)
    expect(apiDelete).toHaveBeenCalledWith('styles/5')
  })

  it('setActive POSTs to the /{id}/activate sub-resource when id>0', async () => {
    await endpoints.styles.setActive(5)
    expect(apiPost).toHaveBeenCalledWith('styles/5/activate', undefined)
  })

  it('setActive POSTs to /deactivate when id≤0 (the "None" button)', async () => {
    await endpoints.styles.setActive(0)
    expect(apiPost).toHaveBeenCalledWith('styles/deactivate', undefined)
  })
})

describe('fonts (hybrid REST)', () => {
  it('deleteFile DELETEs the URL-encoded file name', async () => {
    await endpoints.fonts.deleteFile('My Font.ttf')
    expect(apiDelete).toHaveBeenCalledWith('fonts/My%20Font.ttf')
  })

  it('renameFile PATCHes the file resource with the new name', async () => {
    await endpoints.fonts.renameFile('My Font.ttf', 'Renamed.ttf')
    expect(apiPatch).toHaveBeenCalledWith('fonts/My%20Font.ttf', { new_name: 'Renamed.ttf' })
  })

  it('updateFamily PATCHes the URL-encoded family resource', async () => {
    await endpoints.fonts.updateFamily('My Font', {
      family: 'Fancy',
      serve: 'WOFF2',
      origins: ['https://mysite.carrd.co'],
    })
    expect(apiPatch).toHaveBeenCalledWith('fonts/families/My%20Font', {
      family: 'Fancy',
      serve: 'WOFF2',
      origins: ['https://mysite.carrd.co'],
    })
  })

  it('deleteFont DELETEs the whole family', async () => {
    await endpoints.fonts.deleteFont('My Font')
    expect(apiDelete).toHaveBeenCalledWith('fonts/families/My%20Font')
  })
})

describe('carrd (hybrid REST)', () => {
  it('createProject POSTs the collection with title + folder', async () => {
    await endpoints.carrd.createProject('My Art', 'my-art')
    expect(apiPost).toHaveBeenCalledWith('carrd/projects', { title: 'My Art', folder: 'my-art' })
  })

  it('renameProject PATCHes the URL-encoded project resource', async () => {
    await endpoints.carrd.renameProject('old folder', 'New Title', 'new folder')
    expect(apiPatch).toHaveBeenCalledWith('carrd/projects/old%20folder', {
      title: 'New Title',
      new_folder: 'new folder',
    })
  })

  it('deleteProject DELETEs the URL-encoded project resource', async () => {
    await endpoints.carrd.deleteProject('a b')
    expect(apiDelete).toHaveBeenCalledWith('carrd/projects/a%20b')
  })

  it('deleteImage DELETEs with encoded folder/path/name query params', async () => {
    await endpoints.carrd.deleteImage('proj', 'sub dir/x', 'hero image.png')
    expect(apiDelete).toHaveBeenCalledWith(
      'carrd/images?folder=proj&path=sub%20dir%2Fx&name=hero%20image.png',
    )
  })

  it('createDir POSTs to the /dirs sub-path with folder/path/name', async () => {
    await endpoints.carrd.createDir('proj', 'sub', 'New Dir')
    expect(apiPost).toHaveBeenCalledWith('carrd/images/dirs', {
      folder: 'proj',
      path: 'sub',
      name: 'New Dir',
    })
  })

  it('deleteDir DELETEs the /dirs sub-path with encoded folder/path query params', async () => {
    await endpoints.carrd.deleteDir('proj', 'sub dir/deep')
    expect(apiDelete).toHaveBeenCalledWith('carrd/images/dirs?folder=proj&path=sub%20dir%2Fdeep')
  })
})

describe('images (hybrid REST)', () => {
  it('saveCategory create POSTs the collection with name + dir', async () => {
    await endpoints.images.saveCategory('create', 'Event Banners', 'event_banners')
    expect(apiPost).toHaveBeenCalledWith('image-categories', {
      name: 'Event Banners',
      dir: 'event_banners',
    })
  })

  it('saveCategory rename PATCHes the URL-encoded category resource', async () => {
    await endpoints.images.saveCategory('rename', 'New Name', 'old dir', 'new_dir')
    expect(apiPatch).toHaveBeenCalledWith('image-categories/old%20dir', {
      name: 'New Name',
      new_dir: 'new_dir',
    })
  })

  it('deleteCategory DELETEs the URL-encoded category resource', async () => {
    await endpoints.images.deleteCategory('a b')
    expect(apiDelete).toHaveBeenCalledWith('image-categories/a%20b')
  })

  it('deleteImage DELETEs with encoded dir + name query params', async () => {
    await endpoints.images.deleteImage('raffles', 'my image.png')
    expect(apiDelete).toHaveBeenCalledWith('images?dir=raffles&name=my%20image.png')
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

  it('addEntry posts the entry to the entries collection', async () => {
    const body = { character_name: 'Cloud', world: 'Gaia', num_entries: 2, paid: true }
    await endpoints.raffles.addEntry(7, body)
    expect(apiPost).toHaveBeenCalledWith('raffles/7/entries', body)
  })

  it('markEntryPaid PATCHes the entry resource', async () => {
    await endpoints.raffles.markEntryPaid(7, 42, false)
    expect(apiPatch).toHaveBeenCalledWith('raffles/7/entries/42', { paid: false })
  })

  it('deleteEntry DELETEs the entry resource', async () => {
    await endpoints.raffles.deleteEntry(7, 42)
    expect(apiDelete).toHaveBeenCalledWith('raffles/7/entries/42')
  })

  it('update PUTs the raffle resource by id', async () => {
    await endpoints.raffles.update(7, { title: 'X' })
    expect(apiPut).toHaveBeenCalledWith('raffles/7', { title: 'X' })
  })

  it('delete DELETEs the raffle resource', async () => {
    await endpoints.raffles.delete(7)
    expect(apiDelete).toHaveBeenCalledWith('raffles/7')
  })

  it('pickWinner POSTs to the verb sub-resource', async () => {
    await endpoints.raffles.pickWinner(7)
    expect(apiPost).toHaveBeenCalledWith('raffles/7/pick-winner', undefined)
  })
})

describe('affiliates (hybrid REST)', () => {
  it('create POSTs the affiliate body to the collection', async () => {
    const body = { name: 'Tavern', owners: ['Solo'] }
    await endpoints.affiliates.create(body)
    expect(apiPost).toHaveBeenCalledWith('affiliates', body)
  })

  it('update PUTs the affiliate resource by id', async () => {
    const body = { name: 'Renamed', owners: ['Solo'] }
    await endpoints.affiliates.update(4, body)
    expect(apiPut).toHaveBeenCalledWith('affiliates/4', body)
  })

  it('delete DELETEs the affiliate resource', async () => {
    await endpoints.affiliates.delete(4)
    expect(apiDelete).toHaveBeenCalledWith('affiliates/4')
  })
})

describe('garapons (hybrid REST)', () => {
  it('create POSTs the garapon body to the collection (no action field)', async () => {
    const body = { id: 0, title: 'Drum', prizes: [] }
    await endpoints.garapons.create(body)
    expect(apiPost).toHaveBeenCalledWith('garapons', body)
  })

  it('update PUTs the garapon resource by its id', async () => {
    const body = { id: 7, title: 'Renamed', prizes: [] }
    await endpoints.garapons.update(body)
    expect(apiPut).toHaveBeenCalledWith('garapons/7', body)
  })

  it('delete DELETEs the garapon resource', async () => {
    await endpoints.garapons.delete(7)
    expect(apiDelete).toHaveBeenCalledWith('garapons/7')
  })

  it('setStatus POSTs to the /close verb when closing', async () => {
    await endpoints.garapons.setStatus(7, 'closed')
    expect(apiPost).toHaveBeenCalledWith('garapons/7/close', undefined)
  })

  it('setStatus POSTs to the /reopen verb when reopening', async () => {
    await endpoints.garapons.setStatus(7, 'open')
    expect(apiPost).toHaveBeenCalledWith('garapons/7/reopen', undefined)
  })

  it('createPlayer POSTs the player fields to the players sub-collection', async () => {
    await endpoints.garapons.createPlayer(3, { player_name: 'Hero', max_draws: 2 })
    expect(apiPost).toHaveBeenCalledWith('garapons/3/players', {
      player_name: 'Hero',
      max_draws: 2,
    })
  })

  it('deletePlayer DELETEs the drawing-link sub-resource', async () => {
    await endpoints.garapons.deletePlayer(3, 9)
    expect(apiDelete).toHaveBeenCalledWith('garapons/3/players/9')
  })
})

describe('stamp rallies (hybrid REST)', () => {
  it('create POSTs the rally body to the collection (no action field)', async () => {
    const body = { id: 0, title: 'Summer', stamps: [], prizes: [] }
    await endpoints.stampRallies.create(body)
    expect(apiPost).toHaveBeenCalledWith('stamp-rallies', body)
  })

  it('update PUTs the rally resource by its id', async () => {
    const body = { id: 4, title: 'Renamed', stamps: [], prizes: [] }
    await endpoints.stampRallies.update(body)
    expect(apiPut).toHaveBeenCalledWith('stamp-rallies/4', body)
  })

  it('delete DELETEs the rally resource', async () => {
    await endpoints.stampRallies.delete(4)
    expect(apiDelete).toHaveBeenCalledWith('stamp-rallies/4')
  })

  it('setStatus POSTs to the /close verb when closing', async () => {
    await endpoints.stampRallies.setStatus(4, 'closed')
    expect(apiPost).toHaveBeenCalledWith('stamp-rallies/4/close', undefined)
  })

  it('setStatus POSTs to the /reopen verb when reopening', async () => {
    await endpoints.stampRallies.setStatus(4, 'open')
    expect(apiPost).toHaveBeenCalledWith('stamp-rallies/4/reopen', undefined)
  })

  it('setStampPaused PATCHes the per-stamp sub-resource with the paused flag', async () => {
    await endpoints.stampRallies.setStampPaused(4, 10, true)
    expect(apiPatch).toHaveBeenCalledWith('stamp-rallies/4/stamps/10', { paused: true })
  })

  it('createCard POSTs the participant name to the cards sub-collection', async () => {
    await endpoints.stampRallies.createCard(4, 'Tataru')
    expect(apiPost).toHaveBeenCalledWith('stamp-rallies/4/cards', { participant_name: 'Tataru' })
  })

  it('deleteCard DELETEs the card sub-resource', async () => {
    await endpoints.stampRallies.deleteCard(4, 12)
    expect(apiDelete).toHaveBeenCalledWith('stamp-rallies/4/cards/12')
  })
})

describe('announcement types (hybrid REST)', () => {
  it('createType POSTs the type fields to the collection', async () => {
    await endpoints.announcements.createType({ id: 0, name: 'News', webhook_url: 'https://d/w' })
    expect(apiPost).toHaveBeenCalledWith('announcement-types', {
      name: 'News',
      webhook_url: 'https://d/w',
    })
  })

  it('updateType PUTs the type resource by id', async () => {
    await endpoints.announcements.updateType(5, { id: 5, name: 'News', webhook_url: '' })
    expect(apiPut).toHaveBeenCalledWith('announcement-types/5', { name: 'News', webhook_url: '' })
  })

  it('deleteType DELETEs the type resource', async () => {
    await endpoints.announcements.deleteType(5)
    expect(apiDelete).toHaveBeenCalledWith('announcement-types/5')
  })
})

describe('announcement roles (hybrid REST)', () => {
  it('createRole POSTs the role fields to the collection', async () => {
    await endpoints.announcements.createRole({ id: 0, name: 'Crew', role_id: '123' })
    expect(apiPost).toHaveBeenCalledWith('announcement-roles', { name: 'Crew', role_id: '123' })
  })

  it('updateRole PUTs the role resource by id', async () => {
    await endpoints.announcements.updateRole(6, { id: 6, name: 'Crew', role_id: '123' })
    expect(apiPut).toHaveBeenCalledWith('announcement-roles/6', { name: 'Crew', role_id: '123' })
  })

  it('deleteRole DELETEs the role resource', async () => {
    await endpoints.announcements.deleteRole(6)
    expect(apiDelete).toHaveBeenCalledWith('announcement-roles/6')
  })
})

describe('announcements (hybrid REST)', () => {
  it('save POSTs a new announcement (no id) wrapped under announcement', async () => {
    await endpoints.announcements.save(0, { title: 'Hi' })
    expect(apiPost).toHaveBeenCalledWith('announcements', { announcement: { title: 'Hi' } })
  })

  it('save PUTs an existing announcement (id>0) to its resource', async () => {
    await endpoints.announcements.save(7, { title: 'Edited' })
    expect(apiPut).toHaveBeenCalledWith('announcements/7', { announcement: { title: 'Edited' } })
  })

  it('delete DELETEs the announcement resource by id', async () => {
    await endpoints.announcements.delete(7)
    expect(apiDelete).toHaveBeenCalledWith('announcements/7')
  })

  it('reorder POSTs the ordered ids to the /reorder sub-path', async () => {
    await endpoints.announcements.reorder([3, 1, 2])
    expect(apiPost).toHaveBeenCalledWith('announcements/reorder', { ordered_ids: [3, 1, 2] })
  })

  it('sendNow POSTs to the /{id}/send verb sub-resource', async () => {
    await endpoints.announcements.sendNow(7)
    expect(apiPost).toHaveBeenCalledWith('announcements/7/send', undefined)
  })

  it('skipNext POSTs to the /{id}/skip verb sub-resource', async () => {
    await endpoints.announcements.skipNext(7)
    expect(apiPost).toHaveBeenCalledWith('announcements/7/skip', undefined)
  })
})

describe('bookclub reading lists (nested under the club)', () => {
  it('lists GETs the club-nested collection', async () => {
    await endpoints.bookclub.lists('yuri')
    expect(apiGet).toHaveBeenCalledWith('book-clubs/yuri/reading-lists')
  })

  it('listDetail GETs the club-nested list resource', async () => {
    await endpoints.bookclub.listDetail('yuri', 4)
    expect(apiGet).toHaveBeenCalledWith('book-clubs/yuri/reading-lists/4')
  })

  it('createList POSTs just the title to the club-nested collection', async () => {
    await endpoints.bookclub.createList('yuri', 'Summer Reads')
    expect(apiPost).toHaveBeenCalledWith('book-clubs/yuri/reading-lists', {
      title: 'Summer Reads',
    })
  })

  it('renameList PUTs the club-nested list resource with the new title', async () => {
    await endpoints.bookclub.renameList('yuri', 4, 'Renamed')
    expect(apiPut).toHaveBeenCalledWith('book-clubs/yuri/reading-lists/4', { title: 'Renamed' })
  })

  it('deleteList DELETEs the club-nested list resource by id', async () => {
    await endpoints.bookclub.deleteList('yuri', 4)
    expect(apiDelete).toHaveBeenCalledWith('book-clubs/yuri/reading-lists/4')
  })

  it('saveItem POSTs a new item (no id) wrapped under item', async () => {
    await endpoints.bookclub.saveItem('yuri', 4, { title: 'New Book' })
    expect(apiPost).toHaveBeenCalledWith('book-clubs/yuri/reading-lists/4/items', {
      item: { title: 'New Book' },
    })
  })

  it('saveItem PUTs an existing item (id set) to its sub-resource', async () => {
    await endpoints.bookclub.saveItem('yuri', 4, { id: 9, title: 'Edited Book' })
    expect(apiPut).toHaveBeenCalledWith('book-clubs/yuri/reading-lists/4/items/9', {
      item: { id: 9, title: 'Edited Book' },
    })
  })

  it('deleteItem DELETEs the item sub-resource', async () => {
    await endpoints.bookclub.deleteItem('yuri', 4, 9)
    expect(apiDelete).toHaveBeenCalledWith('book-clubs/yuri/reading-lists/4/items/9')
  })

  it('publish POSTs to the club-nested list publish verb', async () => {
    await endpoints.bookclub.publish('yuri', 4)
    expect(apiPost).toHaveBeenCalledWith('book-clubs/yuri/reading-lists/4/publish', {})
  })

  it('encodeURIComponents the club slug in the path', async () => {
    await endpoints.bookclub.lists('a b/c')
    expect(apiGet).toHaveBeenCalledWith('book-clubs/a%20b%2Fc/reading-lists')
  })
})
