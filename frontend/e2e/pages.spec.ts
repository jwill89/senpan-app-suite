import type { Page } from '@playwright/test'
import { axeViolations } from '@jwill89/test-a11y'
import { PLAYER, expect, test, title } from './fixtures.ts'

/**
 * The public pages, each opened by its own address: it shows its heading and what the
 * server sent for it, then passes axe at WCAG 2.2 AA with its best practices. Everything
 * is found by role and name, never by a class, so the suite still holds when a shared
 * component kit replaces the hand-written components.
 */
const pages: { path: string; heading: string; shows: (page: Page) => Promise<void> }[] = [
  {
    // The logo is the front page's heading, named by the app title.
    path: '/',
    heading: 'Senpan App Suite',
    shows: async (page) => {
      await expect(page.getByRole('heading', { name: 'Join Bingo' })).toBeVisible()
      await expect(page.getByRole('textbox', { name: 'Board ID' })).toBeVisible()
    },
  },
  {
    path: '/card-requests',
    heading: 'Personal Card Request',
    shows: (page) =>
      expect(page.getByRole('spinbutton', { name: 'Column B, row 1' })).toBeVisible(),
  },
  {
    // A new install has published no map; the page says so.
    path: '/festival-maps',
    heading: 'Festival Maps',
    shows: (page) =>
      expect(page.getByText('No festival maps are published right now.')).toBeVisible(),
  },
  {
    path: '/stamp-rallies',
    heading: 'Stamp Rallies',
    shows: (page) => expect(page.getByRole('button', { name: 'Find my links' })).toBeVisible(),
  },
  {
    path: '/stamp-lookup',
    heading: 'Find My Links',
    shows: (page) =>
      expect(page.getByRole('textbox', { name: 'The name you signed up with' })).toBeVisible(),
  },
  {
    path: '/admin/login',
    heading: 'Sign In',
    shows: (page) => expect(page.getByLabel('Username')).toBeVisible(),
  },
]

for (const { path, heading, shows } of pages) {
  test(`${path} loads and passes axe`, async ({ page }) => {
    await page.goto(path)
    await expect(page.getByRole('heading', { name: title(heading) })).toBeVisible()
    await shows(page)
    expect(await axeViolations(page)).toEqual([])
  })
}

test('/raffles lists the open raffle and passes axe', async ({ page, raffle }) => {
  await page.goto('/raffles')
  await expect(page.getByRole('heading', { name: title('Raffles') })).toBeVisible()
  await expect(page.getByRole('heading', { name: title(raffle.title) })).toBeVisible()
  expect(await axeViolations(page)).toEqual([])
})

test('/raffles/:id shows the raffle and its sign-up form, and passes axe', async ({
  page,
  raffle,
}) => {
  await page.goto(`/raffles/${String(raffle.id)}`)
  await expect(page.getByRole('heading', { name: title(raffle.title) })).toBeVisible()
  await expect(page.getByText(raffle.description)).toBeVisible()
  await expect(page.getByRole('textbox', { name: 'Character Name', exact: true })).toBeVisible()
  expect(await axeViolations(page)).toEqual([])
})

test('/play/:cardId shows the board and passes axe', async ({ page, bingo }) => {
  await page.goto(`/play/${bingo.cardId}`)
  await expect(page.getByRole('heading', { name: title(`Board ${bingo.cardId}`) })).toBeVisible()
  await expect(page.getByText(PLAYER)).toBeVisible()
  await expect(page.getByRole('status', { name: 'Connection status: Live' })).toBeVisible()
  expect(await axeViolations(page)).toEqual([])
})
