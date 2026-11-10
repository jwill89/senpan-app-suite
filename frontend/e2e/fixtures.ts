import { existsSync } from 'node:fs'
import { resolve, sep } from 'node:path'
import { test as base, expect, type APIRequestContext, type APIResponse } from '@playwright/test'
import { SITE_URL, STAFF, WEB_ROOT } from './env.ts'

/** The name the run's bingo card is issued to. */
export const PLAYER = 'Mochi Teacup'

/** An open raffle, as the public list and its own page show it. */
export interface Raffle {
  id: number
  title: string
  description: string
}

/**
 * The pattern the run's games are played for: the middle row. Its centre is the free
 * space, so the game calls from B, I, G and O only.
 */
const MIDDLE_ROW = [0, 1, 2, 3, 4].map((row) => [0, 1, 2, 3, 4].map(() => row === 2))

/** A card issued to {@link PLAYER} in a game that has just started. */
export interface Bingo {
  cardId: string
  board: number[][]
  /** Draws the next number, as the Draw button on the admin Game page does, and returns it. */
  draw: () => Promise<number>
}

/**
 * A page title's accessible name. The titles end in a full stop the stylesheet draws
 * (`.topbar h2::after`), and the name includes it.
 */
export function title(text: string): RegExp {
  return new RegExp(`^${text.replace(/[\\^$.*+?()[\]{}|/]/g, '\\$&')}\\.?$`)
}

/** The response, once it has succeeded; a failure says which call and why. */
async function ok(response: Promise<APIResponse>): Promise<APIResponse> {
  const answer = await response
  expect(answer.ok(), `${answer.url()}: ${await answer.text()}`).toBe(true)
  return answer
}

async function body<T>(response: Promise<APIResponse>): Promise<T> {
  return (await (await ok(response)).json()) as T
}

/**
 * Playwright's test, with what the pages need from the run's server:
 *
 * - `/images/` answered from the run's web root, as Apache answers it from the
 *   document root (deploy/.htaccess); the app itself answers a 404 there.
 * - `staff`: the API, signed in as the bootstrap admin, which seeds the rest through
 *   the endpoints the admin pages call.
 * - `bingo`: a game that has just started, with a card in it.
 * - `raffle`: an open raffle, removed again after the test.
 */
export const test = base.extend<
  { images: undefined; bingo: Bingo; raffle: Raffle },
  { staff: APIRequestContext }
>({
  images: [
    async ({ page }, use) => {
      const images = resolve(WEB_ROOT, 'images')
      await page.route(
        (url) => url.origin === SITE_URL && url.pathname.startsWith('/images/'),
        async (route) => {
          const { pathname } = new URL(route.request().url())
          const file = resolve(images, decodeURIComponent(pathname.slice('/images/'.length)))
          // Inside images/, and no dotfile: the .htaccess denies those.
          const served =
            file.startsWith(images + sep) &&
            !file.slice(images.length).includes(`${sep}.`) &&
            existsSync(file)
          await (served
            ? route.fulfill({ path: file })
            : route.fulfill({ status: 404, body: 'Not Found' }))
        },
      )
      await use(undefined)
    },
    { auto: true },
  ],

  staff: [
    async ({ playwright }, use) => {
      const staff = await playwright.request.newContext({ baseURL: SITE_URL })
      await ok(staff.post('/api/v1/auth/login', { data: STAFF }))
      await use(staff)
      await staff.dispose()
    },
    { scope: 'worker' },
  ],

  bingo: async ({ staff }, use) => {
    const { patterns } = await body<{ patterns: { id: number; name: string }[] }>(
      staff.get('/api/v1/patterns'),
    )
    // The API refuses a second pattern with the same grid, so later tests reuse it.
    // Category 1 is the one the migrations seed.
    const pattern =
      patterns.find((p) => p.name === 'Middle Row') ??
      (await body<{ id: number }>(
        staff.post('/api/v1/patterns', {
          data: { name: 'Middle Row', patternData: MIDDLE_ROW, categoryId: 1 },
        }),
      ))
    const card = await body<{ id: string; boardData: number[][] }>(
      staff.post('/api/v1/cards', { data: { playerName: PLAYER } }),
    )
    // A game left running by an earlier test is replaced (its winners unlogged).
    await ok(
      staff.post('/api/v1/game/start', { data: { patternIds: [pattern.id], replace: true } }),
    )
    await use({
      cardId: card.id,
      board: card.boardData,
      // No player delay: the board hears the number as soon as it is drawn.
      draw: async () =>
        (
          await body<{ drawn: { number: number } }>(
            staff.post('/api/v1/game/draw', { data: { delay: 0 } }),
          )
        ).drawn.number,
    })
  },

  raffle: async ({ staff }, use) => {
    const raffle = await body<Raffle>(
      staff.post('/api/v1/raffles', {
        data: {
          title: 'Tea Set Raffle',
          description: 'A porcelain tea set for one lucky guest.',
          rules: '',
          signupInstructions: '',
          entryMode: 'single',
          costPerEntry: 50000,
          tierCosts: [],
          maxEntries: 3,
          prizeImage: '',
          payImage: '',
        },
      }),
    )
    await use(raffle)
    await ok(staff.delete(`/api/v1/raffles/${String(raffle.id)}`))
  },
})

export { expect }
