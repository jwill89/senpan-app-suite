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
    expect(apiPatch).toHaveBeenCalledWith('raffles/7/entries/42', {
      paid: false,
      paid_entries: 0,
      amount_waived: 0,
    })
  })

  it('markEntryPaid sends the waiver for this settlement only', async () => {
    await endpoints.raffles.markEntryPaid(7, 42, true, 5000)
    expect(apiPatch).toHaveBeenCalledWith('raffles/7/entries/42', {
      paid: true,
      paid_entries: 0,
      amount_waived: 5000,
    })
  })

  it('lookup POSTs the search name in the body, not the URL', async () => {
    await endpoints.raffles.lookup(7, 'fairwind')
    expect(apiPost).toHaveBeenCalledWith('raffles/7/lookup', { name: 'fairwind' })
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

  it('setSkip POSTs the occurrence count to the /{id}/skip verb sub-resource', async () => {
    await endpoints.announcements.setSkip(7, 2)
    expect(apiPost).toHaveBeenCalledWith('announcements/7/skip', { count: 2 })
  })

  it('setSkip sends 0 to clear a pending skip', async () => {
    await endpoints.announcements.setSkip(7, 0)
    expect(apiPost).toHaveBeenCalledWith('announcements/7/skip', { count: 0 })
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
