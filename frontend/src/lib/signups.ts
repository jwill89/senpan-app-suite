/**
 * What this browser has signed up for, kept in localStorage.
 *
 * Sign-up is the only moment the app can hand someone their links, because the
 * garapon drawing token is deliberately not recoverable from any name-keyed
 * lookup: a draw is irreversible and a character name is public, so the server
 * will report how many draws are LEFT but never the token that spends them (see
 * backend model.StampLookupEntry). Remembering it here is what keeps that
 * restriction from costing the participant anything on their own device.
 *
 * This is a convenience store, never an authority. Everything in it is
 * user-editable, so nothing read from here may authorize anything: entry caps,
 * ownership and draw allowances are all enforced server-side. What it does is
 * spare people a lookup, and spare them re-typing a name whose exact spelling
 * their entries are keyed on.
 *
 * Every accessor is defensive. localStorage throws outright in some privacy
 * modes, is absent in SSR-ish contexts, and can hold anything a previous version
 * (or a curious user) left behind - so a failure degrades to "nothing saved"
 * rather than breaking the page. Records written before the home world had its own
 * field are split on read (see splitName), so an upgrade does not strand anyone.
 */
import { splitParticipantLabel } from './participant'

/** One rally sign-up: the card link, and the drawing link when one was issued. */
export interface SavedRallySignup {
  kind: 'rally'
  rallyId: number
  rallyTitle: string
  name: string
  /** Home world, kept apart from the name as the server now stores it. */
  world: string
  cardToken: string
  /** Absent when the rally had no open linked garapon at sign-up time. */
  garaponToken?: string
  garaponTitle?: string
  savedAt: number
}

/**
 * One raffle entry. No token: a raffle entry has no link and no capability - what
 * is worth keeping is the exact name and world, because entries merge on that
 * pair and a different spelling silently starts a second entry that splits the
 * person's tickets.
 */
export interface SavedRaffleSignup {
  kind: 'raffle'
  raffleId: number
  raffleTitle: string
  name: string
  world: string
  savedAt: number
}

export type SavedSignup = SavedRallySignup | SavedRaffleSignup

const KEY = 'senpan_signups'

/**
 * How long a saved sign-up is kept. Festivals run days, not months, so this is
 * long enough that nobody loses a link mid-event and short enough that a shared
 * or public machine does not accumulate strangers' links indefinitely.
 */
const MAX_AGE_MS = 60 * 24 * 60 * 60 * 1000

function read(): SavedSignup[] {
  let raw: string | null
  try {
    raw = localStorage.getItem(KEY)
  } catch {
    return [] // storage disabled or blocked
  }
  if (!raw) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return [] // corrupt - treated as empty, and rewritten on the next save
  }
  if (!Array.isArray(parsed)) return []

  const cutoff = Date.now() - MAX_AGE_MS
  const kept: SavedSignup[] = []
  for (const entry of parsed) {
    if (!entry || typeof entry !== 'object') continue
    const e = entry as Partial<SavedSignup> & { savedAt?: unknown }
    if (typeof e.savedAt !== 'number' || e.savedAt < cutoff) continue

    if (e.kind === 'rally') {
      const r = entry as Partial<SavedRallySignup>
      if (typeof r.rallyId !== 'number' || typeof r.cardToken !== 'string' || !r.cardToken) continue
      kept.push({ ...(r as SavedRallySignup), ...splitName(r.name, r.world) })
      continue
    }
    if (e.kind === 'raffle') {
      const r = entry as Partial<SavedRaffleSignup>
      if (typeof r.raffleId !== 'number' || typeof r.name !== 'string' || !r.name) continue
      kept.push({ ...(r as SavedRaffleSignup), ...splitName(r.name, r.world) })
    }
  }
  return kept
}

/**
 * Normalizes a stored name/world pair, splitting a composed "Name @ World" when the
 * world is missing.
 *
 * Entries written before the world had its own field hold the whole thing in `name`
 * and no `world` at all - so without this, `world` would be undefined at runtime
 * while the type promised a string, and the name would no longer match what the
 * server now stores. This is the browser-side counterpart to schema v66's backfill,
 * and it uses the same splitter, so both sides land on the same person.
 */
function splitName(name: unknown, world: unknown): { name: string; world: string } {
  const storedName = typeof name === 'string' ? name : ''
  const storedWorld = typeof world === 'string' ? world.trim() : ''
  if (storedWorld) return { name: storedName.trim(), world: storedWorld }
  return splitParticipantLabel(storedName)
}

function write(entries: SavedSignup[]): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(entries))
  } catch {
    // Full, or blocked. The links are on screen either way; losing the
    // convenience copy must never break the sign-up that just succeeded.
  }
}

/** Everything this browser has saved, newest first, expired entries dropped. */
export function savedSignups(): SavedSignup[] {
  return read().sort((a, b) => b.savedAt - a.savedAt)
}

/**
 * Replaces any saved entry for the same thing, so re-signing up (or entering a
 * raffle again) refreshes rather than duplicates.
 */
function put(entry: SavedSignup): void {
  const same = (other: SavedSignup) =>
    other.kind === entry.kind &&
    (entry.kind === 'rally'
      ? (other as SavedRallySignup).rallyId === entry.rallyId
      : (other as SavedRaffleSignup).raffleId === entry.raffleId)
  write([entry, ...read().filter((other) => !same(other))])
}

export function saveRallySignup(
  entry: Omit<SavedRallySignup, 'kind' | 'savedAt'>,
): SavedRallySignup {
  const saved: SavedRallySignup = { ...entry, kind: 'rally', savedAt: Date.now() }
  put(saved)
  return saved
}

export function saveRaffleSignup(
  entry: Omit<SavedRaffleSignup, 'kind' | 'savedAt'>,
): SavedRaffleSignup {
  const saved: SavedRaffleSignup = { ...entry, kind: 'raffle', savedAt: Date.now() }
  put(saved)
  return saved
}

/** The saved sign-up for one rally, if this browser made it. */
export function savedRallySignup(rallyId: number): SavedRallySignup | undefined {
  return read().find(
    (entry): entry is SavedRallySignup => entry.kind === 'rally' && entry.rallyId === rallyId,
  )
}

/** The saved entry for one raffle, if this browser made it. */
export function savedRaffleSignup(raffleId: number): SavedRaffleSignup | undefined {
  return read().find(
    (entry): entry is SavedRaffleSignup => entry.kind === 'raffle' && entry.raffleId === raffleId,
  )
}

/**
 * Drops one saved sign-up. Offered wherever the links are shown: a participant on
 * a friend's phone or a shared machine needs a way to take their drawing link off
 * it, and that link is the one thing here that spends something.
 */
export function forgetSignup(kind: SavedSignup['kind'], id: number): void {
  write(
    read().filter((entry) =>
      entry.kind !== kind
        ? true
        : kind === 'rally'
          ? (entry as SavedRallySignup).rallyId !== id
          : (entry as SavedRaffleSignup).raffleId !== id,
    ),
  )
}
