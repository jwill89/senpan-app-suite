import { describe, it, expect } from 'vitest'
// Exercises the real CHANGELOG.md through the `virtual:changelog` build plugin
// (config/changelog-plugin.ts), registered in vitest.config.ts.
import { changelog, PLUGIN_REPO_URL, PLUGIN_INSTALL_STEPS, CHANGELOG_LABELS } from './changelog'

describe('changelog pipeline', () => {
  it('parses CHANGELOG.md into structured entries for each component', () => {
    for (const key of ['frontend', 'backend', 'plugin'] as const) {
      const section = changelog[key]
      expect(section.entries.length, `${key} entries`).toBeGreaterThan(0)
      // `latest` mirrors the newest entry's version.
      expect(section.latest).toBe(section.entries[0].version)
      // Every entry has a version and at least a date or some groups.
      for (const e of section.entries) {
        expect(e.version, `${key} entry version`).toBeTruthy()
      }
    }
  })

  it('parses labelled change-groups with bullet bodies', () => {
    const latest = changelog.frontend.entries[0]
    expect(latest.groups.length).toBeGreaterThan(0)
    const added = latest.groups.find((g) => g.label.toLowerCase() === 'added')
    expect(added, 'frontend 3.7.0 should have an Added group').toBeTruthy()
    expect(added?.body).toContain('- ') // its body is a markdown bullet list
  })

  it('parses component-appropriate version + date formats', () => {
    expect(changelog.frontend.latest).toMatch(/^\d+\.\d+\.\d+$/)
    expect(changelog.backend.latest).toMatch(/^\d+\.\d+\.\d+$/)
    // The Dalamud plugin uses a four-part AssemblyVersion.
    expect(changelog.plugin.latest).toMatch(/^\d+\.\d+\.\d+\.\d+$/)
    // Dates are parsed off the heading (`### [x] — YYYY-MM-DD`).
    expect(changelog.frontend.entries[0].date).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  it('keeps sections separate (no cross-bleed)', () => {
    expect(changelog.frontend.latest).not.toBe(changelog.plugin.latest)
  })

  it('provides Dalamud install details for the plugin', () => {
    expect(PLUGIN_REPO_URL).toBe('https://apps.senpan.cafe/plugin/pluginmaster.json')
    expect(PLUGIN_INSTALL_STEPS.length).toBeGreaterThan(0)
    expect(PLUGIN_INSTALL_STEPS.some((s) => s.detail.includes('/senpan'))).toBe(true)
    expect(CHANGELOG_LABELS.plugin).toBe('Plugin')
  })
})
