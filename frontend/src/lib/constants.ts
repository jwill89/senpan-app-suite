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

/** Creates an empty 5×5 boolean grid for the pattern editor. */
export function emptyGrid(): boolean[][] {
  return Array.from({ length: 5 }, () => Array.from({ length: 5 }, () => false))
}

/**
 * Registry of book clubs shown under the admin "Book Clubs" section. Add an
 * entry to introduce a new club; its sidebar button, route, settings webhook
 * field, and tab all wire up automatically from this list (mirror the backend
 * `bookClubs` registry in bookclubs.go — keep `slug`/`commentsLabel` in sync).
 *
 *   - `commentsLabel`: label for the per-item curator comments field (e.g.
 *     "Yao's Comments" vs "Drani's Comments"); also used in the Discord embed.
 *   - `icon`: FontAwesome icon class for the sidebar/heading (must be registered
 *     in lib/fontawesome.ts).
 */
export const BOOK_CLUBS = [
  { slug: 'yaoi', name: 'Yaoi Book Club', commentsLabel: "Yao's Comments", icon: 'fa-bicep' },
  { slug: 'yuri', name: 'Yuri Book Club', commentsLabel: "Drani's Comments", icon: 'fa-flower-daffodil' },
] as const

/** Per-club reading-list Discord webhook setting key (matches webhookSettingKey on the backend). */
export function clubWebhookKey(slug: string): `discord_webhook_url_${string}` {
  return `discord_webhook_url_${slug}`
}

/** Per-club events-channel Discord webhook setting key (matches eventsWebhookSettingKey on the backend). */
export function clubEventsWebhookKey(slug: string): `discord_events_webhook_url_${string}` {
  return `discord_events_webhook_url_${slug}`
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
  { key: 'teahouse-raffles', label: 'Raffles', section: 'Senpan Tea House' },
  ...BOOK_CLUBS.map((c) => ({
    key: `bookclub-${c.slug}`,
    label: c.name,
    section: 'Senpan Tea House',
  })),
  { key: 'atelier-fonts', label: 'Font Upload', section: 'Atelier Yao' },
  { key: 'atelier-carrd', label: 'Carrd Upload', section: 'Atelier Yao' },
  { key: 'system-settings', label: 'App Settings', section: 'System' },
  { key: 'system-themes', label: 'Themes', section: 'System' },
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
  // One blank webhook default per known club (reading-list + events channel) so
  // the settings form binds cleanly before the server response loads.
  ...Object.fromEntries(
    BOOK_CLUBS.flatMap((c) => [
      [clubWebhookKey(c.slug), ''],
      [clubEventsWebhookKey(c.slug), ''],
    ]),
  ),
}

/** Meeting-length options (hours) for a book club event. */
export const MEETING_LENGTH_OPTIONS = [1, 2, 3, 4, 5] as const

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
      if (zones?.length) return zones
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
  'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
  'America/Anchorage', 'America/Halifax', 'America/Sao_Paulo', 'Europe/London',
  'Europe/Paris', 'Europe/Berlin', 'Europe/Madrid', 'Europe/Moscow', 'Africa/Cairo',
  'Africa/Johannesburg', 'Asia/Dubai', 'Asia/Kolkata', 'Asia/Bangkok', 'Asia/Shanghai',
  'Asia/Tokyo', 'Asia/Seoul', 'Australia/Sydney', 'Pacific/Auckland', 'Pacific/Honolulu',
]

/** Curated fallback Google Fonts list (used when no API key is configured). */
export const FALLBACK_GOOGLE_FONTS: string[] = [
  'Arapey', 'Abel', 'Abril Fatface', 'Alegreya', 'Alfa Slab One', 'Amatic SC', 'Anton',
  'Archivo', 'Archivo Black', 'Arvo', 'Bangers', 'Barlow', 'Barlow Condensed', 'Bebas Neue',
  'Bitter', 'Black Ops One', 'Cabin', 'Cairo', 'Caveat', 'Cinzel', 'Comfortaa', 'Concert One',
  'Cormorant Garamond', 'Creepster', 'Crimson Text', 'Dancing Script', 'DM Sans',
  'DM Serif Display', 'Dosis', 'EB Garamond', 'Exo 2', 'Fira Sans', 'Fjalla One', 'Fredoka One',
  'Great Vibes', 'IBM Plex Sans', 'Inter', 'Josefin Sans', 'Kanit', 'Karla', 'Lato', 'Lexend',
  'Libre Baskerville', 'Lilita One', 'Lobster', 'Lora', 'Merriweather', 'Montserrat', 'Mulish',
  'Nunito', 'Open Sans', 'Orbitron', 'Oswald', 'Outfit', 'Pacifico', 'Permanent Marker',
  'Playfair Display', 'Poppins', 'Press Start 2P', 'PT Sans', 'PT Serif', 'Quicksand', 'Raleway',
  'Righteous', 'Roboto', 'Roboto Condensed', 'Roboto Slab', 'Rubik', 'Russo One', 'Sacramento',
  'Shadows Into Light', 'Source Sans 3', 'Space Grotesk', 'Special Elite', 'Teko', 'Titan One',
  'Ubuntu', 'Vollkorn', 'Work Sans', 'Yanone Kaffeesatz', 'Zilla Slab',
]
