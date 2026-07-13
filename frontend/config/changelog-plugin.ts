import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import type { Plugin } from 'vite'

// The single source of truth for release notes lives at the repo root, outside
// the frontend project. This plugin reads and parses it at build/dev time and
// exposes the per-component sections as the virtual module `virtual:changelog`
// (declared in env.d.ts), so the admin dashboard can render each component's
// changelog without a backend round-trip or a committed generated file. Shared
// between vite.config.ts and vitest.config.ts so tests resolve it too.
const CHANGELOG_PATH = fileURLToPath(new URL('../../CHANGELOG.md', import.meta.url))

// A structured view of the changelog so the UI can render it with proper
// typography + navigation instead of dumping raw markdown. Each component section
// becomes an ordered list of version entries; each entry has an intro paragraph
// and labelled change-groups (Added / Fixed / …) whose `body` is the markdown of
// that group's bullet list (rendered with markdown-it in the modal).
interface ChangelogGroup {
  label: string
  body: string
}
interface ChangelogEntry {
  version: string
  date: string
  intro: string
  groups: ChangelogGroup[]
}
interface ChangelogSection {
  latest: string
  entries: ChangelogEntry[]
}
interface Changelog {
  frontend: ChangelogSection
  backend: ChangelogSection
  plugin: ChangelogSection
}

const HEADING_TO_KEY: Record<string, keyof Changelog> = {
  Frontend: 'frontend',
  Backend: 'backend',
  Plugin: 'plugin',
}

/** Parses one component section's markdown into its version entries. */
function parseEntries(lines: string[]): ChangelogEntry[] {
  const entries: ChangelogEntry[] = []
  let cur: { version: string; date: string; intro: string[]; groups: ChangelogGroup[] } | null =
    null
  let group: ChangelogGroup | null = null

  const flush = () => {
    if (!cur) return
    entries.push({
      version: cur.version,
      date: cur.date,
      intro: cur.intro.join('\n').trim(),
      groups: cur.groups
        .map((g) => ({ label: g.label, body: g.body.trim() }))
        .filter((g) => g.body),
    })
  }

  for (const line of lines) {
    // `### [version] — date` starts a version entry (date optional).
    const v = /^###\s+\[([^\]]+)\]\s*(?:[—–-]\s*(.+?))?\s*$/.exec(line)
    if (v) {
      flush()
      // .at() is typed `string | undefined` (the date group is optional), unlike
      // v[2] which the RegExpExecArray type widens to `string`.
      cur = { version: v[1].trim(), date: (v.at(2) ?? '').trim(), intro: [], groups: [] }
      group = null
      continue
    }
    if (!cur) continue
    // `#### Added` / `#### Fixed` / … starts a change-group within the entry.
    const g = /^####\s+(.+?)\s*$/.exec(line)
    if (g) {
      group = { label: g[1].trim(), body: '' }
      cur.groups.push(group)
      continue
    }
    if (group) group.body += line + '\n'
    else cur.intro.push(line)
  }
  flush()
  return entries
}

/**
 * Splits CHANGELOG.md into its `## Frontend` / `## Backend` / `## Plugin`
 * sections and parses each into structured version entries. Only level-2 headings
 * delimit a section (a `### [x.y.z]` version or `#### Added` never matches `^## `),
 * and the preamble before the first component heading is ignored.
 */
export function parseChangelog(md: string): Changelog {
  const buf: Record<keyof Changelog, string[]> = { frontend: [], backend: [], plugin: [] }
  let current: keyof Changelog | null = null
  for (const line of md.split(/\r?\n/)) {
    const h2 = /^##\s+(.+?)\s*$/.exec(line)
    if (h2) {
      // A level-2 heading either starts a known component section or (any other
      // `## …`) ends the current one.
      current = HEADING_TO_KEY[h2[1].trim()] ?? null
      continue
    }
    if (current) buf[current].push(line)
  }
  const build = (lines: string[]): ChangelogSection => {
    const entries = parseEntries(lines)
    return { latest: entries[0]?.version ?? '', entries }
  }
  return {
    frontend: build(buf.frontend),
    backend: build(buf.backend),
    plugin: build(buf.plugin),
  }
}

/** Vite plugin exposing the parsed changelog as `import { changelog } from 'virtual:changelog'`. */
export function changelogPlugin(): Plugin {
  const virtualId = 'virtual:changelog'
  const resolvedId = '\0' + virtualId
  return {
    name: 'app-changelog',
    resolveId(id) {
      return id === virtualId ? resolvedId : undefined
    },
    load(id) {
      if (id !== resolvedId) return undefined
      // Re-read on every load and register the file so `vite dev` picks up edits.
      this.addWatchFile(CHANGELOG_PATH)
      let md = ''
      try {
        md = readFileSync(CHANGELOG_PATH, 'utf-8')
      } catch {
        md = ''
      }
      return `export const changelog = ${JSON.stringify(parseChangelog(md))}\n`
    },
  }
}
