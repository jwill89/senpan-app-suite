import { expect, test } from './fixtures.ts'
import { STAFF } from './env.ts'

/**
 * Signing in through the sign-in page, as staff do: a wrong password is said so on
 * the page (never taken for an expired session), the right one opens the admin
 * area, and signing out leaves it.
 */
test('staff sign in, see a wrong password refused, and sign out', async ({ page }) => {
  await page.goto('/admin/login')
  await page.getByLabel('Username').fill(STAFF.username)
  await page.getByLabel('Password').fill('not the password')
  await page.getByRole('button', { name: 'Login' }).click()
  await expect(page.getByText("That username and password don't match.")).toBeVisible()
  await expect(page).toHaveURL(/\/admin\/login/)

  await page.getByLabel('Password').fill(STAFF.password)
  await page.getByRole('button', { name: 'Login' }).click()
  await expect(page).toHaveURL(/\/admin\/bingo\/game/)

  await page.getByText('User Options').click()
  await page.getByRole('button', { name: 'Logout' }).click()
  await expect(page).toHaveURL(/\/$/)
  const me = await page.request.get('/api/v1/me')
  expect(await me.json()).toEqual({ account: null })
})
