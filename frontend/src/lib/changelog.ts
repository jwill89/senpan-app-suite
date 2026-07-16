/**
 * Changelog access for the admin dashboard.
 *
 * Re-exports the parsed CHANGELOG.md (provided by the `virtual:changelog` build
 * plugin — see config/changelog-plugin.ts) and adds the Dalamud install steps
 * shown for the plugin. The admin sidebar shows each component's version and opens
 * its changelog on click (see AppVersions.vue / ChangelogModal.vue).
 */
import {
  changelog,
  type ChangelogSection,
  type ChangelogEntry,
  type ChangelogGroup,
} from 'virtual:changelog'

export { changelog }
export type { ChangelogSection, ChangelogEntry, ChangelogGroup }

/** The three independently-versioned components. */
export type ChangelogComponent = 'frontend' | 'backend' | 'plugin'

/** Human labels for each component (modal titles, etc.). */
export const CHANGELOG_LABELS: Record<ChangelogComponent, string> = {
  frontend: 'Frontend',
  backend: 'Backend',
  plugin: 'Plugin',
}

/** The Dalamud custom-repository URL (matches pluginmaster.json's DownloadLink*). */
export const PLUGIN_REPO_URL = 'https://apps.senpan.cafe/plugin/pluginmaster.json'

/**
 * Same-origin path to the deployed Dalamud repo index. Fetched at runtime (see
 * {@link fetchLivePluginVersion}) so the admin footer can show the LIVE plugin
 * version — what Dalamud actually serves — instead of the version baked into this
 * bundle at build time. Publishing a new plugin (deploy `-Target plugin`) then
 * refreshes the shown version without a frontend rebuild. Relative so it stays
 * same-origin (the SPA and `/plugin/` share the doc root in production); in dev it
 * 404s and the caller falls back to the bundled changelog version.
 */
export const PLUGIN_MASTER_PATH = '/plugin/pluginmaster.json'

/**
 * Reads the live plugin version: the `AssemblyVersion` of the first entry in the
 * deployed pluginmaster.json. Returns `null` on any failure (dev/offline/parse) so
 * the caller can fall back to the bundled changelog version. Never throws. Sends
 * `cache: 'no-store'` so a browser cache can't mask a fresh deploy (belt-and-braces
 * with the no-cache headers Apache serves for the plugin repo).
 */
export async function fetchLivePluginVersion(): Promise<string | null> {
  try {
    const res = await fetch(PLUGIN_MASTER_PATH, { cache: 'no-store' })
    if (!res.ok) return null
    const data: unknown = await res.json()
    const first = Array.isArray(data) ? data[0] : null
    const version =
      first && typeof first === 'object'
        ? (first as { AssemblyVersion?: unknown }).AssemblyVersion
        : null
    return typeof version === 'string' && version.length > 0 ? version : null
  } catch {
    return null
  }
}

/** One install step (title + markdown detail) for the plugin "How to install" view. */
export interface InstallStep {
  title: string
  detail: string
}

/**
 * Dalamud install steps for the plugin, shown as a distinct panel (not mixed into
 * the changelog). Mirrors plugins/README.md.
 */
export const PLUGIN_INSTALL_STEPS: InstallStep[] = [
  {
    title: 'Generate an access token',
    detail: 'On this site, open **User Options → Access Token → Generate**, and copy the token.',
  },
  {
    title: 'Open Dalamud’s custom repositories',
    detail: 'In game, run **`/xlsettings`** → **Experimental** → **Custom Plugin Repositories**.',
  },
  {
    title: 'Add the Senpan repository',
    detail: `Paste the repo URL into the empty row, click the **＋**, then **Save**.`,
  },
  {
    title: 'Install the plugin',
    detail:
      'Open the plugin installer (**`/xlplugins`**), find **Senpan Admin Companion**, and click **Install**.',
  },
  {
    title: 'Connect it',
    detail:
      'Run **`/senpan`**, paste your token, and **Save & Connect**. A green **● Live** badge means it’s connected.',
  },
]
