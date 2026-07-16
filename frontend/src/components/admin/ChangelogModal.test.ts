import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ChangelogModal from './ChangelogModal.vue'
import { changelog } from '@/lib/changelog'

// Structural/navigation checks against the real parsed CHANGELOG.md (via the
// virtual:changelog plugin). Text rendered through markdown-it loads async and is
// not asserted here; the rail, badges, and install steps render synchronously.

describe('ChangelogModal', () => {
  it('lists every version and shows the latest release with labelled groups', () => {
    const wrapper = mount(ChangelogModal, { props: { component: 'frontend' } })
    const versions = changelog.frontend.entries

    // One rail item per version (frontend has no install item).
    expect(wrapper.findAll('.cl__railitem')).toHaveLength(versions.length)

    // Opens on the newest release, with its change-group badges (Added/Changed/…).
    expect(wrapper.find('.cl__detailtitle').text()).toContain(`v${versions[0].version}`)
    const badges = wrapper.findAll('.cl-badge').map((b) => b.text())
    expect(badges.length).toBeGreaterThan(0)
    // Badges are the newest release's actual change-group labels — don't hardcode one
    // (a Changed/Fixed-only release has no "Added" group).
    expect(badges).toEqual(versions[0].groups.map((g) => g.label))
  })

  it('navigates to an older version when its rail item is clicked', async () => {
    const wrapper = mount(ChangelogModal, { props: { component: 'frontend' } })
    const versions = changelog.frontend.entries
    // Click the second-newest version.
    await wrapper.findAll('.cl__railitem')[1].trigger('click')
    expect(wrapper.find('.cl__detailtitle').text()).toContain(`v${versions[1].version}`)
  })

  it('opens the plugin on a distinct install view with the copyable repo URL', () => {
    const wrapper = mount(ChangelogModal, { props: { component: 'plugin' } })

    // The install rail item exists and the install view is shown by default.
    expect(wrapper.find('.cl__railitem--install').exists()).toBe(true)
    expect(wrapper.find('.cl__install').exists()).toBe(true)
    expect(wrapper.text()).toContain('https://apps.senpan.cafe/plugin/pluginmaster.json')
    // Numbered steps render (from PLUGIN_INSTALL_STEPS), not dumped as prose.
    expect(wrapper.findAll('.cl__steps li').length).toBeGreaterThan(0)
    expect(wrapper.text()).toContain('Generate an access token')

    // Selecting a version switches away from the install view to that release.
    expect(wrapper.find('.cl__entry').exists()).toBe(false)
  })

  it('switches from the plugin install view to a version', async () => {
    const wrapper = mount(ChangelogModal, { props: { component: 'plugin' } })
    // First non-install rail item = newest plugin version.
    const versionItem = wrapper
      .findAll('.cl__railitem')
      .find((b) => !b.classes('cl__railitem--install'))
    await versionItem!.trigger('click')
    expect(wrapper.find('.cl__install').exists()).toBe(false)
    expect(wrapper.find('.cl__entry').exists()).toBe(true)
    expect(wrapper.find('.cl__detailtitle').text()).toContain(`v${changelog.plugin.latest}`)
  })
})
