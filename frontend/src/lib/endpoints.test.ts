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
