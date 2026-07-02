import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, type VueWrapper } from '@vue/test-utils'

// Mock the router so we can assert navigation happens only for item clicks.
const push = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
vi.mock('@/router', () => ({ adminTabRouteName: (tab: string) => 'admin-' + tab }))

import AdminSidebar from './AdminSidebar.vue'
import { useAuthStore } from '@/stores/auth'
import type { User } from '@/types/api'

function makeUser(partial: Partial<User>): User {
  return {
    id: 1,
    username: 'u',
    is_admin: false,
    is_active: true,
    permissions: [],
    created_at: '',
    last_login_at: '',
    ...partial,
  }
}

/** Mounts the sidebar with the given logged-in account. The AppVersions footer
 * (which fetches GET /api/version on mount) is stubbed — it's covered by its own
 * test and irrelevant to the navigation assertions here. */
function mountAs(user: User): VueWrapper {
  const auth = useAuthStore()
  auth.user = user
  auth.isAdmin = user.is_admin
  return mount(AdminSidebar, { global: { stubs: { AppVersions: true } } })
}

/** The section wrapper whose header text contains `label` (looked up fresh). */
function section(wrapper: VueWrapper, label: string) {
  return wrapper
    .findAll('.admin-nav-section')
    .find((s) => s.find('.admin-nav-header').text().includes(label))!
}

/** Whether a section is expanded — its header carries the `open` class, which is
 * the exact expression (`openSection === <section>`) that also drives the items'
 * `v-show`, so it is the source of truth for accordion state. */
function isExpanded(wrapper: VueWrapper, label: string): boolean {
  return section(wrapper, label).find('.admin-nav-header').classes().includes('open')
}

/** Clicks a section's accordion header. */
async function clickHeader(wrapper: VueWrapper, label: string): Promise<void> {
  await section(wrapper, label).find('.admin-nav-header').trigger('click')
}

describe('AdminSidebar', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    push.mockClear()
  })

  it('shows every section for an admin, with the active section expanded', () => {
    const wrapper = mountAs(makeUser({ is_admin: true }))
    // Bingo, Senpan Tea House, Festival, Atelier Yao, System, User Options.
    expect(wrapper.findAll('.admin-nav-section')).toHaveLength(6)
    // admin.adminSection defaults to 'bingo', so Bingo starts expanded.
    expect(isExpanded(wrapper, 'Bingo')).toBe(true)
    expect(section(wrapper, 'Bingo').find('.admin-nav-items').isVisible()).toBe(true)
    expect(isExpanded(wrapper, 'System')).toBe(false)
    expect(section(wrapper, 'System').find('.admin-nav-items').isVisible()).toBe(false)
    // The Festival section carries Garapon + Stamp Rally + Raffles (Raffles moved
    // here from Tea House).
    const festivalText = section(wrapper, 'Festival')
      .findAll('.admin-nav-items button')
      .map((b) => b.text())
    expect(festivalText).toHaveLength(3)
    expect(festivalText.join(' ')).toContain('Garapon')
    expect(festivalText.join(' ')).toContain('Stamp Rally')
    expect(festivalText.join(' ')).toContain('Raffles')
    // The always-present User Options section carries Access Token, Change
    // Password + Logout.
    const userText = section(wrapper, 'User Options')
      .findAll('.admin-nav-items button')
      .map((b) => b.text())
      .join(' ')
    expect(userText).toContain('Access Token')
    expect(userText).toContain('Change Password')
    expect(userText).toContain('Logout')
  })

  it('header click expands a section independently (others stay open) WITHOUT navigating', async () => {
    const wrapper = mountAs(makeUser({ is_admin: true }))
    expect(isExpanded(wrapper, 'Bingo')).toBe(true) // active section starts open

    await clickHeader(wrapper, 'System')

    // Independent accordion: System expands while the already-open Bingo stays
    // open, and no navigation happened — headers are pure toggles now.
    expect(isExpanded(wrapper, 'System')).toBe(true)
    expect(isExpanded(wrapper, 'Bingo')).toBe(true)
    expect(push).not.toHaveBeenCalled()
  })

  it('clicking an open section header again collapses it (no navigation)', async () => {
    const wrapper = mountAs(makeUser({ is_admin: true }))
    expect(isExpanded(wrapper, 'Bingo')).toBe(true)

    await clickHeader(wrapper, 'Bingo')

    expect(isExpanded(wrapper, 'Bingo')).toBe(false)
    expect(push).not.toHaveBeenCalled()
  })

  it('item click navigates via the router', async () => {
    const wrapper = mountAs(makeUser({ is_admin: true }))
    await clickHeader(wrapper, 'System') // expand System to reveal its items
    const settingsBtn = section(wrapper, 'System')
      .findAll('.admin-nav-items button')
      .find((b) => b.text().includes('Settings'))!

    await settingsBtn.trigger('click')

    expect(push).toHaveBeenCalledWith({ name: 'admin-system-settings' })
  })

  it('hides sections the account cannot access any item in', () => {
    // A non-admin with only the Manage Cards permission sees just the Bingo
    // section (and only its Cards item) — no empty Tea House / Atelier / System —
    // plus the always-present User Options section.
    const wrapper = mountAs(makeUser({ permissions: ['bingo-cards'] }))
    const sections = wrapper.findAll('.admin-nav-section')
    expect(sections).toHaveLength(2)
    expect(sections[0].find('.admin-nav-header').text()).toContain('Bingo')
    expect(sections[1].find('.admin-nav-header').text()).toContain('User Options')
    const buttons = sections[0].findAll('.admin-nav-items button')
    expect(buttons).toHaveLength(1)
    expect(buttons[0].text()).toContain('Manage Cards')
  })
})
