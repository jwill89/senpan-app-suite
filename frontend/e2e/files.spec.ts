import { writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { SERVER_DATA } from './env.ts'
import { expect, test } from './fixtures.ts'

/** A 1x1 PNG. The run's own picture: the brand art in deploy/images is untracked. */
const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=',
  'base64',
)

/**
 * The data folder's files, as the server answers them in production, where nothing is
 * served from a document root: the image library is public, its manifest is not, and a
 * font is never a plain file.
 */
test('the server serves the image library and never a font file', async ({ request }) => {
  writeFileSync(join(SERVER_DATA, 'images', 'e2e-probe.png'), PNG)

  const picture = await request.get('/images/e2e-probe.png')
  expect(picture.status()).toBe(200)
  expect(picture.headers()['content-type']).toBe('image/png')
  expect(picture.headers()['x-content-type-options']).toBe('nosniff')

  expect((await request.get('/images/.categories.json')).status()).toBe(404)
  expect((await request.get('/fonts/Any.woff2')).status()).toBe(404)
  expect((await request.get('/plugin/pluginmaster.json')).status()).toBe(404)
})
