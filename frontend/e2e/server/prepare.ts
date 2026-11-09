/**
 * Builds the end-to-end run's API and its throwaway web root before the API starts
 * (playwright.config.ts): a fresh data folder, the binary built from this checkout into
 * it, and the document root's persistent images/ as a deploy leaves it. The API makes
 * and migrates its database on start, seeding the bootstrap admin the run's staff sign
 * in with; the tests seed the rest through the API.
 *
 * It refuses to run unless the data folder lies outside the checkout (and does not hold
 * it, since it is emptied first) and every path the API is given is inside that folder:
 * devdata/ is a copy of the live data, real webhook URLs and all.
 */
import { execFileSync } from 'node:child_process'
import { cpSync, existsSync, mkdirSync, rmSync } from 'node:fs'
import { isAbsolute, join, relative, resolve } from 'node:path'
import { DATA_DIR, ROOT, SERVER, WEB_ROOT, serverFlags } from '../env.ts'

function inside(folder: string, path: string): boolean {
  const rel = relative(folder, path)
  return rel !== '' && !rel.startsWith('..') && !isAbsolute(rel)
}

const devdata = resolve(ROOT, 'devdata')
if (resolve(DATA_DIR) === ROOT || inside(ROOT, DATA_DIR) || inside(DATA_DIR, ROOT)) {
  throw new Error(`The run's data folder ${DATA_DIR} overlaps the checkout; nothing was changed.`)
}
for (const [name, path] of Object.entries({ ...serverFlags, SERVER })) {
  if (name === '-addr') {
    continue
  }
  if (!isAbsolute(path) || !inside(DATA_DIR, path) || inside(devdata, resolve(path))) {
    throw new Error(
      `${name} is ${path || 'empty'}, not a path in ${DATA_DIR}; nothing was changed.`,
    )
  }
}

rmSync(DATA_DIR, { recursive: true, force: true })
mkdirSync(WEB_ROOT, { recursive: true })

// deploy/images, as deploy/README.md lays out the document root; the API writes its
// uploads and image categories beside it. The brand art there is the owner's and
// untracked, so a CI run has the folders without it (and the pages their alt text).
cpSync(join(ROOT, 'deploy', 'images'), join(WEB_ROOT, 'images'), { recursive: true })

// Built as `task build` builds it, to the run's folder, with the front end's build
// embedded: the binary serves the SPA, so that build comes first.
if (!existsSync(join(ROOT, 'backend', 'internal', 'web', 'dist', 'index.html'))) {
  throw new Error(
    'The front end is not built, and the server embeds it: run `task build:web` (or `npm run build`) first.',
  )
}
execFileSync('go', ['build', '-o', SERVER, '.'], { cwd: join(ROOT, 'backend'), stdio: 'inherit' })
