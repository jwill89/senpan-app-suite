#!/usr/bin/env node
/**
 * CSS convention + token-drift gate.
 *
 * ESLint does not lint CSS and Prettier only formats it, so nothing enforced the
 * naming rules or kept the FIVE copies of the theme-token list in agreement. This
 * script does both. Run via `npm run lint:conventions`; wired into
 * scripts/check.ps1 and .github/workflows/ci.yml alongside lint/format.
 *
 * The rules it enforces are documented in AGENTS.md -> "CSS conventions".
 */
import { readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { join, relative, sep } from 'node:path'

const SRC = fileURLToPath(new URL('../src', import.meta.url))
const REPO = fileURLToPath(new URL('../..', import.meta.url))

const failures = []
const fail = (rule, detail) => failures.push({ rule, detail })

// ---------------------------------------------------------------- helpers
function walk(dir, test, out = []) {
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    if (e.name === 'node_modules' || e.name === 'dist' || e.name === 'coverage') continue
    const p = join(dir, e.name)
    if (e.isDirectory()) walk(p, test, out)
    else if (test(e.name)) out.push(p)
  }
  return out
}
const rel = (p) => relative(REPO, p).split(sep).join('/')
const srcFiles = walk(SRC, (n) => /\.(vue|css|ts)$/.test(n))

/** Style text of a file: whole file for .css, anchored <style> blocks for .vue. */
function styleBlocks(file, src) {
  if (file.endsWith('.css')) return [src]
  return [...src.matchAll(/^<style\b[^>]*>([\s\S]*?)^<\/style>/gm)].map((m) => m[1])
}

/** Class names declared by a stylesheet's selectors. */
function declaredClasses(css) {
  const out = new Set()
  const clean = css.replace(/\/\*[\s\S]*?\*\//g, '')
  for (const m of clean.matchAll(/([^{}]+)\{[^{}]*\}/g)) {
    if (/^\s*@/.test(m[1])) continue
    for (const c of m[1].matchAll(/\.(-?[_a-zA-Z][\w-]*)/g)) out.add(c[1])
  }
  return out
}

const THIRD_PARTY = /^(vc-|milkdown|crepe|ProseMirror|v3-|cm-|fa-|svg-inline)/

// ---------------------------------------------------------------- rule 1
// Class names must describe a role, never an appearance. This is the rule the
// retired `--gold` token broke: `.text-gold` named the colour, so renaming the
// token to `--highlight` left the class lying about itself.
const APPEARANCE =
  /(^|-)(gold|silver|bronze|red|green|blue|yellow|amber|orange|purple|pink|grey|gray|white|black|dim|bright)(-|$)/

// ---------------------------------------------------------------- rule 2
// One separator style. `-` joins a name to its part, `--` marks a modifier, and
// `is-` marks state. BEM's `__` is not used: over half the CSS lives in Vue
// `<style scoped>` blocks, where the compiler already guarantees uniqueness, so
// `__` bought ceremony and no isolation.
const BEM_ELEMENT = /__/

// ---------------------------------------------------------------- rule 3
// Bare state words (`.active`, `.open`) collide with data values and HTML
// attributes of the same name. State is always `is-*`.
const BARE_STATE = new Set([
  'active',
  'open',
  'on',
  'off',
  'current',
  'selected',
  'disabled',
  'dragging',
  'called',
  'free',
  'stamped',
  'expanded',
  'collapsed',
  'hidden',
  'visible',
  'loading',
  'checked',
  'drag-over',
  'dragover',
])

for (const f of srcFiles) {
  const src = readFileSync(f, 'utf8')
  for (const css of styleBlocks(f, src)) {
    for (const cls of declaredClasses(css)) {
      if (THIRD_PARTY.test(cls)) continue
      if (APPEARANCE.test(cls))
        fail(
          'appearance-named class',
          `.${cls} in ${rel(f)} - name the role or the token, not the colour`,
        )
      if (BEM_ELEMENT.test(cls))
        fail('BEM __ separator', `.${cls} in ${rel(f)} - use "-" for a part, "--" for a modifier`)
      if (BARE_STATE.has(cls))
        fail('bare state class', `.${cls} in ${rel(f)} - state must be prefixed "is-"`)
    }
  }
}

// ---------------------------------------------------------------- rule 4
// Every var(--x) must resolve. A typo'd or retired token silently falls back to
// `inherit`, which is invisible until someone notices the wrong colour.
const rootTokens = new Set(
  (
    readFileSync(join(SRC, 'assets/styles/tokens.css'), 'utf8').match(/^\s+--[a-z0-9-]+(?=:)/gm) ||
    []
  ).map((s) => s.trim()),
)
const localProps = new Set()
for (const f of srcFiles)
  for (const m of readFileSync(f, 'utf8').matchAll(/(--[a-z0-9-]+)\s*:/g)) localProps.add(m[1])
// Custom properties set from JS (`style.setProperty`, `:style="{ '--x': ... }"`).
for (const f of srcFiles)
  for (const m of readFileSync(f, 'utf8').matchAll(/['"](--[a-z0-9-]+)['"]/g)) localProps.add(m[1])

for (const f of srcFiles) {
  const src = readFileSync(f, 'utf8')
  for (const m of src.matchAll(/var\(\s*(--[a-z0-9-]+)/g)) {
    const t = m[1]
    if (rootTokens.has(t) || localProps.has(t)) continue
    fail('undefined CSS variable', `var(${t}) in ${rel(f)} - not declared in tokens.css or locally`)
  }
}

// ---------------------------------------------------------------- rule 5
// The themeable token list is duplicated in five places. They must agree, or the
// theme editor, the seeder and the contrast auditor disagree about what a theme is.
//
// tokens.css also holds tokens that are deliberately NOT themeable; they are listed
// here and must be absent from the themeable copies.
const NON_THEMEABLE = new Set(['--radius', '--radius-media', '--body-font', '--header-font'])
const themeableCss = [...rootTokens]
  .filter((t) => !NON_THEMEABLE.has(t))
  .map((t) => t.slice(2))
  .sort()

const sources = {
  'lib/theme-tokens.ts': [
    join(SRC, 'lib/theme-tokens.ts'),
    (s) => (s.match(/^\s*name: '([a-z0-9-]+)'/gm) || []).map((x) => x.split("'")[1]),
  ],
  'backend/internal/store/styles.go': [
    join(REPO, 'backend/internal/store/styles.go'),
    (s) => {
      const block = s.match(/themeTokenOrder\s*=\s*\[\]string\{([\s\S]*?)\}/)
      return block ? [...block[1].matchAll(/"([a-z0-9-]+)"/g)].map((m) => m[1]) : null
    },
  ],
}

for (const [label, [path, extract]] of Object.entries(sources)) {
  let names
  try {
    names = extract(readFileSync(path, 'utf8'))
  } catch {
    fail('token source unreadable', label)
    continue
  }
  if (!names || !names.length) {
    fail(
      'token list not found',
      `${label} - the extractor matched nothing; update check-conventions.mjs`,
    )
    continue
  }
  const set = new Set(names)
  for (const t of themeableCss)
    if (!set.has(t)) fail('token missing from a copy', `"${t}" is in tokens.css but not ${label}`)
  for (const t of set)
    if (!themeableCss.includes(t))
      fail('token missing from tokens.css', `"${t}" is in ${label} but not tokens.css`)
  for (const t of set)
    if (NON_THEMEABLE.has('--' + t))
      fail(
        'non-themeable token exposed',
        `"${t}" is documented non-themeable but appears in ${label}`,
      )
}

// ---------------------------------------------------------------- rule 6
// An object must keep its base rule. `.x:hover` / `.x.is-y` / `.x .child` style a
// STATE or PART of `.x`; if no rule defines `.x` itself, the object has no box and
// the states modify nothing.
//
// This exists because a bulk rule-dropper that matched only simple selectors
// (`^\.name {`) was pointed at all of src/ and stripped the base rules out of
// utilities.css while leaving their `:hover`/`.is-*` compounds behind. The file
// still looked populated and every test passed - the objects just silently
// rendered unstyled.
//
// "Defined" means a rule whose LAST compound is exactly `.x`, so a contextual base
// (`.admin-login .box {}`) counts. Skipped: `is-*` (state never has a base) and
// co-class modifiers - a class that always shares its class attribute with another
// (`class="opacity-slider sound-volume"`) is a modifier of that object, not an
// object itself, so it legitimately has no base of its own.
const defined = new Set()
const objectOf = new Map() // class -> files where it heads a compound rule
const standsAlone = new Map() // class -> true if it is ever the only class on an element
const anyRule = new Set() // class -> appears anywhere in any selector
const staticAttr = new Map() // class -> files whose markup hard-codes it
const tsText = [] // .ts sources, for the test/JS-hook exemption in rule 7

for (const f of srcFiles) {
  if (f.endsWith('.ts')) {
    tsText.push(readFileSync(f, 'utf8'))
    continue
  }
  const src = readFileSync(f, 'utf8')
  for (const css of styleBlocks(f, src)) {
    // `:deep(.x)` / `::v-deep(.x)` target `.x` itself - unwrap before parsing.
    const clean = css
      .replace(/\/\*[\s\S]*?\*\//g, '')
      .replace(/::?v-deep\s*\(([^)]*)\)|:deep\(([^)]*)\)/g, '$1$2')
    for (const m of clean.matchAll(/([^{}]+)\{/g)) {
      const list = m[1].trim()
      if (!list || list.startsWith('@') || list.includes(';')) continue
      for (const sel of list.split(',')) {
        const s = sel.trim()
        if (!s) continue
        for (const c of s.matchAll(/\.(-?[_a-zA-Z][\w-]*)/g)) anyRule.add(c[1])
        const last =
          s
            .split(/\s+|>|\+|~/)
            .filter(Boolean)
            .pop() || ''
        if (/^\.(-?[_a-zA-Z][\w-]*)$/.test(last)) {
          defined.add(last.slice(1))
        } else {
          const head = last.match(/\.(-?[_a-zA-Z][\w-]*)/)
          if (head) {
            if (!objectOf.has(head[1])) objectOf.set(head[1], new Set())
            objectOf.get(head[1]).add(rel(f))
          }
        }
      }
    }
  }
  // Static class="..." only. `:class` is a JS expression, not reliably parseable.
  const tpl = src.match(/^<template>([\s\S]*?)^<\/template>/m)
  if (!tpl) continue
  for (const m of tpl[1].matchAll(/(?<![:\w-])class="([^"]*)"/g)) {
    const cs = m[1].split(/\s+/).filter(Boolean)
    for (const c of cs) {
      if (!staticAttr.has(c)) staticAttr.set(c, new Set())
      staticAttr.get(c).add(rel(f))
    }
    for (const c of cs) if (cs.length === 1) standsAlone.set(c, true)
  }
}

for (const [cls, files] of objectOf) {
  if (defined.has(cls) || THIRD_PARTY.test(cls) || /^is-/.test(cls)) continue
  if (!standsAlone.get(cls)) continue
  fail(
    'object has no base rule',
    `.${cls} is styled only as a compound (${[...files].join(', ')}) - nothing defines .${cls} itself`,
  )
}

// ---------------------------------------------------------------- rule 7
// No dead classes in markup. A class hard-coded in a static class="..." must be
// styled by something, or the markup is describing an intent nothing delivers.
//
// This found 20: four spacing utilities that were never defined (`mt-4`, `mt-10`
// on elements that then got no margin at all), `btn-secondary` on three Server
// Logs buttons that consequently rendered with no fill, and fourteen inert
// wrapper classes.
//
// Exempt: a class referenced from a .ts file is a deliberate test or JS hook
// (ChangelogModal.test.ts queries `.changelog-entry`), not dead markup. Only
// static attributes are checked - `:class` is a JS expression and cannot be
// parsed reliably enough to gate on.
const tsBlob = tsText.join('\n')
for (const [cls, files] of staticAttr) {
  if (anyRule.has(cls) || THIRD_PARTY.test(cls)) continue
  if (new RegExp(`['"\`.]${cls.replace(/[-]/g, '\\-')}\\b`).test(tsBlob)) continue
  fail(
    'dead class in markup',
    `.${cls} is hard-coded in ${[...files].join(', ')} but no rule styles it`,
  )
}

// ---------------------------------------------------------------- rule 8
// Scoped CSS may only shrink. Shared styles are the default and a <style scoped>
// block is the exception (AGENTS.md -> "The two tiers"), but nothing measured that,
// so scoped CSS grew to near parity with the global stylesheets before anyone
// noticed - the extraction that followed took days. A ratchet makes the next
// regression cost one line of review instead.
//
// Two guards, because they fail differently:
//   - GRANDFATHERED: a file already in the baseline may not exceed its recorded
//     count. Legacy size is tolerated; growth is not.
//   - CAPPED: a file NOT in the baseline is new work, and new work has no legacy
//     to plead. It may not exceed NEW_FILE_CAP without an explicit re-baseline.
// The total is also pinned, so churn cannot move lines between files for free.
//
// Run `node scripts/check-conventions.mjs --update-baseline` after a reduction to
// lock the gain in. Slack is reported but never fails - a check that failed the
// moment you improved something would just train people to re-baseline blindly.
const NEW_FILE_CAP = 120
const BASELINE = join(SRC, '..', 'scripts', 'scoped-css-baseline.json')

/** Scoped <style> line count for one SFC, braces included. */
function scopedLines(src) {
  return [...src.matchAll(/^<style\b[^>]*>([\s\S]*?)^<\/style>/gm)].reduce(
    (n, m) => n + m[1].split('\n').filter((l) => l.trim()).length,
    0,
  )
}

const scopedNow = {}
for (const f of srcFiles) {
  if (!f.endsWith('.vue')) continue
  const n = scopedLines(readFileSync(f, 'utf8'))
  if (n > 0) scopedNow[rel(f)] = n
}
const totalNow = Object.values(scopedNow).reduce((a, b) => a + b, 0)

if (process.argv.includes('--update-baseline')) {
  const next = { total: totalNow, cap: NEW_FILE_CAP, files: {} }
  for (const k of Object.keys(scopedNow).sort()) next.files[k] = scopedNow[k]
  writeFileSync(BASELINE, JSON.stringify(next, null, 2) + '\n')
  console.log(
    `scoped-css baseline updated - ${Object.keys(scopedNow).length} files, ${totalNow} lines`,
  )
  process.exit(0)
}

let baseline = null
try {
  baseline = JSON.parse(readFileSync(BASELINE, 'utf8'))
} catch {
  fail(
    'scoped-css baseline missing',
    `${rel(BASELINE)} - run: npm run lint:conventions -- --update-baseline`,
  )
}

let slack = 0
if (baseline) {
  for (const [file, n] of Object.entries(scopedNow)) {
    const was = baseline.files[file]
    if (was === undefined) {
      if (n > NEW_FILE_CAP)
        fail(
          'new SFC over the scoped-CSS cap',
          `${file} has ${n} scoped lines (cap ${NEW_FILE_CAP}) - promote the reusable parts to assets/styles/ first`,
        )
    } else if (n > was) {
      fail(
        'scoped CSS grew',
        `${file} ${was} -> ${n} lines - move the new rules to assets/styles/, or re-baseline deliberately`,
      )
    } else if (n < was) slack += was - n
  }
  if (totalNow > baseline.total)
    fail('total scoped CSS grew', `${baseline.total} -> ${totalNow} lines across all SFCs`)
}

// ---------------------------------------------------------------- rule 9
// A themeable token must be consumed by something OTHER than the theme editor.
// The editor renders every token by definition, so it cannot vouch for one: a
// token only it references is a knob in the UI that changes nothing in the app.
// `--accent-hover` and `--accent-2-hover` went the other way - dropped from the
// stylesheets while still offered in the editor - and stayed broken for weeks.
const EDITOR_OWNED = /(ThemeTokenEditor|theme-tokens|assets\/styles\/tokens\.css)/
const consumerBlob = srcFiles
  .filter((f) => !EDITOR_OWNED.test(rel(f)))
  .map((f) => readFileSync(f, 'utf8'))
  .join('\n')
for (const t of themeableCss) {
  if (!new RegExp(`var\\(\\s*--${t}\\b`).test(consumerBlob))
    fail(
      'themeable token has no consumer',
      `--${t} is offered in the theme editor but no app stylesheet reads it - editing it changes nothing`,
    )
}

// ---------------------------------------------------------------- report
if (!failures.length) {
  console.log(
    `conventions OK - ${srcFiles.length} files, ${themeableCss.length} themeable tokens in sync across 3 sources, ` +
      `${totalNow} scoped CSS lines in ${Object.keys(scopedNow).length} SFCs`,
  )
  if (slack)
    console.log(
      `  note: scoped CSS is ${slack} line(s) below baseline - run \`npm run lint:conventions -- --update-baseline\` to lock it in`,
    )
  process.exit(0)
}
const byRule = new Map()
for (const { rule, detail } of failures) byRule.set(rule, (byRule.get(rule) || []).concat(detail))
console.error(`\n${failures.length} convention problem(s):\n`)
for (const [rule, details] of byRule) {
  console.error(`  ${rule} (${details.length}):`)
  for (const d of details.slice(0, 12)) console.error(`    - ${d}`)
  if (details.length > 12) console.error(`    ... and ${details.length - 12} more`)
}
console.error('\nSee AGENTS.md -> "CSS conventions".\n')
process.exit(1)
