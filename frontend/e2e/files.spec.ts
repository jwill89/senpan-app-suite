import { expect, test } from './fixtures.ts'

/**
 * The data folder's files, as the server answers them in production, where nothing is
 * served from a document root: the image library is public, its manifest is not, and a
 * font is never a plain file.
 */
test('the server serves the image library and never a font file', async ({ request }) => {
  const icon = await request.get('/images/pwa-192x192.png')
  expect(icon.status()).toBe(200)
  expect(icon.headers()['content-type']).toBe('image/png')
  expect(icon.headers()['x-content-type-options']).toBe('nosniff')

  expect((await request.get('/images/.categories.json')).status()).toBe(404)
  expect((await request.get('/fonts/Any.woff2')).status()).toBe(404)
  expect((await request.get('/plugin/pluginmaster.json')).status()).toBe(404)
})
