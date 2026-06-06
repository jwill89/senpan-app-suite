/**
 * Shared constants and small pure helpers mirrored from the original app.js.
 * Keeping these identical preserves the exact look and behavior.
 */

export interface StampShape {
  id: string
  emoji: string
  name: string
}

/** Stamp shapes — identical set/order to the original app. */
export const STAMP_SHAPES: StampShape[] = [
  { id: 'blank', emoji: '', name: 'Blank' },
  { id: 'heart', emoji: '♥️', name: 'Heart' },
  { id: 'star', emoji: '⭐', name: 'Star' },
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

/** Default app settings — matches the original app.js defaults. */
export const DEFAULT_APP_SETTINGS = {
  app_title: 'Senpan App Suite',
  default_draw_delay: '0',
  frequent_winner_threshold: '3',
  frequent_winner_hours: '12',
  header_font: 'Arapey',
  google_fonts_api_key: '',
}

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
