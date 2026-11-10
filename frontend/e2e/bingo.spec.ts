import { axeViolations } from '@jwill89/test-a11y'
import { expect, test, title } from './fixtures.ts'

/**
 * The key flow, as a player meets a bingo night: they type the board ID staff gave them
 * on the front page and join; the caller draws until a number on their card comes up,
 * and each one reaches the board live; they stamp it, and the stamp outlasts a full
 * reload. It needs the board API, the board's live event stream and the browser's own
 * stamp store to work end to end.
 */
test('a player joins with their board ID, follows the calls and stamps one', async ({
  page,
  bingo,
}) => {
  await page.goto('/')
  await page.getByRole('textbox', { name: 'Board ID' }).fill(bingo.cardId)
  await page.getByRole('button', { name: 'Join', exact: true }).click()

  await expect(page.getByRole('heading', { name: title(`Board ${bingo.cardId}`) })).toBeVisible()
  await expect(page.getByRole('status', { name: 'Connection status: Live' })).toBeVisible()
  await expect(
    page.getByRole('heading', { name: 'Called Numbers (0 / 75)', exact: true }),
  ).toBeVisible()

  // The caller draws on the admin Game page; the board hears each draw on its stream.
  const onCard = new Set(bingo.board.flat())
  let calls = 0
  let called = 0
  do {
    called = await bingo.draw()
    calls += 1
  } while (!onCard.has(called))
  await expect(
    page.getByRole('heading', { name: `Called Numbers (${String(calls)} / 75)`, exact: true }),
  ).toBeVisible()

  const cell = page.getByRole('button', { name: String(called), exact: true })
  await expect(cell).toHaveAttribute('aria-pressed', 'false')
  await cell.click()
  await expect(cell).toHaveAttribute('aria-pressed', 'true')
  expect(await axeViolations(page)).toEqual([])

  // A full load: the stamps are the browser's to keep, and the calls the server's.
  await page.reload()
  await expect(
    page.getByRole('heading', { name: `Called Numbers (${String(calls)} / 75)`, exact: true }),
  ).toBeVisible()
  await expect(cell).toHaveAttribute('aria-pressed', 'true')
})
