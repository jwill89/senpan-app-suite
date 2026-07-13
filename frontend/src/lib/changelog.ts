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
