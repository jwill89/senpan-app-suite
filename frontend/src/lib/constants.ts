/**
 * Shared constants and small pure helpers mirrored from the original app.js.
 * Keeping these identical preserves the exact look and behavior.
 */
import type { AppSettings } from '@/types/api'

export interface StampShape {
  id: string
  emoji: string
  name: string
}

/** Stamp shapes — identical set/order to the original app. */
export const STAMP_SHAPES: StampShape[] = [
  { id: 'blank', emoji: '', name: 'Blank' },
  { id: 'heart', emoji: '♥️', name: 'Heart' },
  { id: 'smiley', emoji: '😊', name: 'Smiley' },
  { id: 'upside-down-face', emoji: '🙃', name: 'Upside-Down Face' },
  { id: 'expressionless', emoji: '😑', name: 'Expressionless' },
  { id: 'crying', emoji: '😭', name: 'Crying' },
  { id: 'skull', emoji: '💀', name: 'Skull' },
]

export interface StampColor {
  id: string
  name: string
  value: string
}

/** Stamp colors — identical set/order to the original app. */
export const STAMP_COLORS: StampColor[] = [
  { id: 'pink', name: 'Pink', value: 'rgba(229,49,112,.55)' },
  { id: 'red', name: 'Red', value: 'rgba(255,0,0,.55)' },
  { id: 'orange', name: 'Orange', value: 'rgba(255,152,0,.55)' },
  { id: 'gold', name: 'Gold', value: 'rgba(255,216,3,.55)' },
  { id: 'green', name: 'Green', value: 'rgba(44,182,125,.55)' },
  { id: 'blue', name: 'Blue', value: 'rgba(56,128,255,.55)' },
  { id: 'purple', name: 'Purple', value: 'rgba(127,90,240,.55)' },
]

/** The BINGO column letters, in order. */
export const BINGO_LETTERS = ['B', 'I', 'N', 'G', 'O'] as const

/** Draw delay options offered in the admin Draw control (seconds). */
export const DRAW_DELAY_OPTIONS = [0, 3, 5, 10, 15, 20, 30, 45, 60] as const

/** Returns the 15 bingo numbers for a column index (0=B … 4=O). */
export function columnNumbers(colIndex: number): number[] {
  const start = colIndex * 15 + 1
  return Array.from({ length: 15 }, (_, i) => start + i)
}

/** Returns the inclusive [min, max] number range for a column index (0=B … 4=O). */
export function columnRange(colIndex: number): [number, number] {
  const start = colIndex * 15 + 1
  return [start, start + 14]
}

/** An empty 5×5 number board (all 0; centre [2][2] is the FREE space). */
export function emptyNumberBoard(): number[][] {
  return Array.from({ length: 5 }, () => Array.from({ length: 5 }, () => 0))
}

/**
 * Generates a random valid 5×5 bingo board (each column shuffled from its range;
 * centre [2][2] = 0 = FREE). Mirrors the backend `bingo.GenerateBoard`.
 */
export function randomBoard(): number[][] {
  const board = emptyNumberBoard()
  for (let col = 0; col < 5; col++) {
    const [lo, hi] = columnRange(col)
    const pool = Array.from({ length: hi - lo + 1 }, (_, i) => lo + i)
    for (let i = pool.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1))
      ;[pool[i], pool[j]] = [pool[j], pool[i]]
    }
    for (let row = 0; row < 5; row++) board[row][col] = pool[row]
  }
  board[2][2] = 0 // FREE centre
  return board
}

/**
 * Validates a hand-built board client-side (mirrors the backend
 * `bingo.ValidateBoard`): 5×5, FREE centre, each column within its range with no
 * repeats. Returns a human-readable error message, or '' when the card is valid.
 */
export function validateBoard(board: number[][]): string {
  if (board.length !== 5 || board.some((r) => r.length !== 5)) return 'Card must be a 5×5 grid.'
  if (board[2][2] !== 0) return 'The centre cell must be left as the FREE space.'
  for (let col = 0; col < 5; col++) {
    const [lo, hi] = columnRange(col)
    const seen = new Set<number>()
    for (let row = 0; row < 5; row++) {
      if (col === 2 && row === 2) continue // FREE centre
      const n = board[row][col]
      if (!Number.isInteger(n) || n < lo || n > hi) {
        return `Column ${BINGO_LETTERS[col]} must only contain numbers ${lo}–${hi}.`
      }
      if (seen.has(n)) return `Column ${BINGO_LETTERS[col]} has the number ${n} more than once.`
      seen.add(n)
    }
  }
  return ''
}

/** Creates an empty 5×5 boolean grid for the pattern editor. */
export function emptyGrid(): boolean[][] {
  return Array.from({ length: 5 }, () => Array.from({ length: 5 }, () => false))
}

/**
 * Reports which of the five bingo columns (B,I,N,G,O → indices 0–4) are "active"
 * for a game: a column is active when at least one active pattern marks a non-FREE
 * cell in it, so numbers from that column can be drawn. Mirrors the backend
 * `bingo.PatternColumns` exactly — the caller skips columns no pattern uses (e.g. a
 * postage-stamp game draws no N numbers), and with no patterns every column is
 * active. The Called Numbers panel darkens the inactive columns to show they won't
 * be used that game.
 */
export function patternColumns(patterns: { pattern_data: boolean[][] }[]): boolean[] {
  const cols = [false, false, false, false, false]
  for (const p of patterns) {
    const grid = p.pattern_data
    for (let r = 0; r < 5 && r < grid.length; r++) {
      for (let c = 0; c < 5 && c < grid[r].length; c++) {
        if (grid[r][c] && !(r === 2 && c === 2)) cols[c] = true
      }
    }
  }
  return cols.some((active) => active) ? cols : [true, true, true, true, true]
}

/**
 * FF14 worlds grouped by data center, for the World picker on the public Personal
 * Card Requests page. Maintained by hand; update when Square Enix adds/moves worlds.
 */
export interface WorldDataCenter {
  /** Data center name (the <optgroup> label). */
  name: string
  /** Region the data center belongs to (NA / EU / JP / OCE). */
  region: string
  worlds: string[]
}

export const FF14_WORLDS: WorldDataCenter[] = [
  // North America
  {
    name: 'Aether',
    region: 'NA',
    worlds: [
      'Adamantoise',
      'Cactuar',
      'Faerie',
      'Gilgamesh',
      'Jenova',
      'Midgardsormr',
      'Sargatanas',
      'Siren',
    ],
  },
  {
    name: 'Crystal',
    region: 'NA',
    worlds: ['Balmung', 'Brynhildr', 'Coeurl', 'Diabolos', 'Goblin', 'Malboro', 'Mateus', 'Zalera'],
  },
  {
    name: 'Dynamis',
    region: 'NA',
    worlds: [
      'Cuchulainn',
      'Golem',
      'Halicarnassus',
      'Kraken',
      'Maduin',
      'Marilith',
      'Rafflesia',
      'Seraph',
    ],
  },
  {
    name: 'Primal',
    region: 'NA',
    worlds: [
      'Behemoth',
      'Excalibur',
      'Exodus',
      'Famfrit',
      'Hyperion',
      'Lamia',
      'Leviathan',
      'Ultros',
    ],
  },
  // Europe
  {
    name: 'Chaos',
    region: 'EU',
    worlds: [
      'Cerberus',
      'Louisoix',
      'Moogle',
      'Omega',
      'Phantom',
      'Ragnarok',
      'Sagittarius',
      'Spriggan',
    ],
  },
  {
    name: 'Light',
    region: 'EU',
    worlds: ['Alpha', 'Lich', 'Odin', 'Phoenix', 'Raiden', 'Shiva', 'Twintania', 'Zodiark'],
  },
  // Japan
  {
    name: 'Elemental',
    region: 'JP',
    worlds: ['Aegis', 'Atomos', 'Carbuncle', 'Garuda', 'Gungnir', 'Kujata', 'Tonberry', 'Typhon'],
  },
  {
    name: 'Gaia',
    region: 'JP',
    worlds: ['Alexander', 'Bahamut', 'Durandal', 'Fenrir', 'Ifrit', 'Ridill', 'Tiamat', 'Ultima'],
  },
  {
    name: 'Mana',
    region: 'JP',
    worlds: ['Anima', 'Asura', 'Chocobo', 'Hades', 'Ixion', 'Masamune', 'Pandaemonium', 'Titan'],
  },
  {
    name: 'Meteor',
    region: 'JP',
    worlds: [
      'Belias',
      'Mandragora',
      'Ramuh',
      'Shinryu',
      'Unicorn',
      'Valefor',
      'Yojimbo',
      'Zeromus',
    ],
  },
  // Oceania
  {
    name: 'Materia',
    region: 'OCE',
    worlds: ['Bismarck', 'Ravana', 'Sephirot', 'Sophia', 'Zurvan'],
  },
]

/** Flat, sorted list of every FF14 world name (for validation / lookup). */
export const FF14_WORLD_NAMES: string[] = FF14_WORLDS.flatMap((dc) => dc.worlds).sort()

/**
 * Registry of book clubs shown under the admin "Book Clubs" section. Add an
 * entry to introduce a new club; its sidebar button, route, settings webhook
 * field, and tab all wire up automatically from this list (mirror the backend
 * `bookClubs` registry in bookclubs.go — keep `slug`/`commentsLabel` in sync).
 *
 *   - `commentsLabel`: label for the per-item curator comments field (e.g.
 *     "Yao's Comments" vs "Drani's Comments"); also used in the Discord embed.
 *   - `icon`: bare duotone FontAwesome icon name for the sidebar/heading, e.g.
 *     'bicep' — rendered as `:icon="['fad', icon]"` (must be registered in
 *     lib/fontawesome.ts).
 */
export const BOOK_CLUBS = [
  { slug: 'yaoi', name: 'Yaoi Book Club', commentsLabel: "Yao's Comments", icon: 'bicep' },
  {
    slug: 'yuri',
    name: 'Yuri Book Club',
    commentsLabel: "Drani's Comments",
    icon: 'flower-daffodil',
  },
] as const

/** Per-club reading-list Discord webhook setting key (matches webhookSettingKey on the backend). */
export function clubWebhookKey(slug: string): `discord_webhook_url_${string}` {
  return `discord_webhook_url_${slug}`
}

/**
 * Canonical list of grantable admin page permissions. Each `key` matches both a
 * frontend AdminTab id and the backend permission constant (see permissions.go),
 * so it doubles as the router `meta.tab`/permission key. Used by the Users-page
 * permission editor, the sidebar gating, and the router guard.
 *
 * NOTE: the Users page itself ("system-users") is intentionally NOT listed — it
 * is admin-only and never granted to non-admins. Book-club entries are derived
 * from BOOK_CLUBS so adding a club wires up its permission automatically.
 */
export interface AdminPermission {
  key: string
  label: string
  section: string
}

export const ADMIN_PERMISSIONS: AdminPermission[] = [
  { key: 'bingo-game', label: 'Current/New Game', section: 'Bingo' },
  { key: 'bingo-cards', label: 'Manage Cards', section: 'Bingo' },
  { key: 'bingo-winners-log', label: 'Winners Log', section: 'Bingo' },
  { key: 'bingo-patterns', label: 'Patterns', section: 'Bingo' },
  { key: 'bingo-presets', label: 'Game Presets', section: 'Bingo' },
  { key: 'teahouse-announcements', label: 'Announcements', section: 'Senpan Tea House' },
  { key: 'teahouse-affiliates', label: 'Affiliates', section: 'Senpan Tea House' },
  { key: 'teahouse-tea-rooms', label: 'Tea Rooms', section: 'Senpan Tea House' },
  ...BOOK_CLUBS.map((c) => ({
    key: `bookclub-${c.slug}`,
    label: c.name,
    section: 'Senpan Tea House',
  })),
  { key: 'festival-garapon', label: 'Garapon', section: 'Festival' },
  { key: 'festival-stamp-rally', label: 'Stamp Rally', section: 'Festival' },
  // Raffles moved under Festival; the permission/route id stays `teahouse-raffles`.
  { key: 'teahouse-raffles', label: 'Raffles', section: 'Festival' },
  { key: 'atelier-fonts', label: 'Font Upload', section: 'Atelier Yao' },
  { key: 'atelier-carrd', label: 'Carrd Upload', section: 'Atelier Yao' },
  { key: 'system-settings', label: 'Settings', section: 'System' },
  { key: 'system-themes', label: 'Themes', section: 'System' },
  { key: 'system-images', label: 'Images', section: 'System' },
]

/** Default app settings — matches the original app.js defaults. */
export const DEFAULT_APP_SETTINGS: AppSettings = {
  app_title: 'Senpan App Suite',
  default_draw_delay: '0',
  frequent_winner_threshold: '3',
  frequent_winner_hours: '12',
  header_font: 'Arapey',
  google_fonts_api_key: '',
  anilist_api_url: 'https://graphql.anilist.co',
  bingo_join_prompt: 'Enter your unique bingo board ID to play',
  yoever_cooldown_seconds: '180',
  custom_card_cost: '0',
  // One blank reading-list webhook default per known club so the settings form
  // binds cleanly before the server response loads.
  ...Object.fromEntries(BOOK_CLUBS.map((c) => [clubWebhookKey(c.slug), ''])),
}

/** The browser's current IANA timezone (e.g. "America/New_York"); 'UTC' fallback. */
export function detectTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

/**
 * Sorted IANA timezone names for the event timezone picker. Uses the runtime's
 * `Intl.supportedValuesOf('timeZone')` when available (all current browsers),
 * else a small curated fallback that still includes the detected zone.
 */
export function supportedTimezones(): string[] {
  try {
    const intl = Intl as unknown as { supportedValuesOf?: (key: string) => string[] }
    if (typeof intl.supportedValuesOf === 'function') {
      const zones = intl.supportedValuesOf('timeZone')
      if (zones.length) return zones
    }
  } catch {
    /* fall through to the curated list */
  }
  const detected = detectTimezone()
  const set = new Set<string>([detected, ...FALLBACK_TIMEZONES])
  return [...set].sort()
}

/** Small fallback timezone list when Intl.supportedValuesOf is unavailable. */
export const FALLBACK_TIMEZONES: string[] = [
  'UTC',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Anchorage',
  'America/Halifax',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Madrid',
  'Europe/Moscow',
  'Africa/Cairo',
  'Africa/Johannesburg',
  'Asia/Dubai',
  'Asia/Kolkata',
  'Asia/Bangkok',
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Asia/Seoul',
  'Australia/Sydney',
  'Pacific/Auckland',
  'Pacific/Honolulu',
]

/** Curated fallback Google Fonts list (used when no API key is configured). */
export const FALLBACK_GOOGLE_FONTS: string[] = [
  'Arapey',
  'Abel',
  'Abril Fatface',
  'Alegreya',
  'Alfa Slab One',
  'Amatic SC',
  'Anton',
  'Archivo',
  'Archivo Black',
  'Arvo',
  'Bangers',
  'Barlow',
  'Barlow Condensed',
  'Bebas Neue',
  'Bitter',
  'Black Ops One',
  'Cabin',
  'Cairo',
  'Caveat',
  'Cinzel',
  'Comfortaa',
  'Concert One',
  'Cormorant Garamond',
  'Creepster',
  'Crimson Text',
  'Dancing Script',
  'DM Sans',
  'DM Serif Display',
  'Dosis',
  'EB Garamond',
  'Exo 2',
  'Fira Sans',
  'Fjalla One',
  'Fredoka One',
  'Great Vibes',
  'IBM Plex Sans',
  'Inter',
  'Josefin Sans',
  'Kanit',
  'Karla',
  'Lato',
  'Lexend',
  'Libre Baskerville',
  'Lilita One',
  'Lobster',
  'Lora',
  'Merriweather',
  'Montserrat',
  'Mulish',
  'Nunito',
  'Open Sans',
  'Orbitron',
  'Oswald',
  'Outfit',
  'Pacifico',
  'Permanent Marker',
  'Playfair Display',
  'Poppins',
  'Press Start 2P',
  'PT Sans',
  'PT Serif',
  'Quicksand',
  'Raleway',
  'Righteous',
  'Roboto',
  'Roboto Condensed',
  'Roboto Slab',
  'Rubik',
  'Russo One',
  'Sacramento',
  'Shadows Into Light',
  'Source Sans 3',
  'Space Grotesk',
  'Special Elite',
  'Teko',
  'Titan One',
  'Ubuntu',
  'Vollkorn',
  'Work Sans',
  'Yanone Kaffeesatz',
  'Zilla Slab',
]
