/**
 * API types barrel.
 *
 * Re-exports the auto-generated domain types (mirrored from the Go `model`
 * package via tygo) and adds the request/response envelope types that the API
 * handlers use (these live in the Go `server` package as ad-hoc structs /
 * `map[string]any`, so they are mirrored here by hand).
 *
 * Keep this file in sync with `src/internal/server/*.go` handler shapes.
 * The domain models in `api.generated.ts` regenerate via `npm run gen:types`.
 */
import type {
  BingoDrawnNumber,
  BingoGameState,
  BookClubEvent,
  Card,
  FrequentWinner,
  Pattern,
  PatternCategory,
  Raffle,
  RaffleEntry,
  ReadingList,
  ReadingListItem,
  ReadingListSource,
  Style,
  WinnersLogEntry,
} from './api.generated'

export type {
  BingoDrawnNumber,
  BingoGame,
  BingoGamePattern,
  BingoGameState,
  BookClubEvent,
  Card,
  FrequentWinner,
  Pattern,
  PatternCategory,
  Raffle,
  RaffleEntry,
  ReadingList,
  ReadingListItem,
  ReadingListSource,
  Style,
  WinnersLogEntry,
} from './api.generated'

// ── Generic / action responses ───────────────────────────────────────────────
// Many POST action endpoints reply with a simple {"success": true} (or an
// error envelope handled by ApiError). Callers that don't read the body use this.
export interface OkResponse {
  success?: boolean
}

// GET /api/auth and POST /api/auth (login/logout).
export interface AuthCheckResponse {
  authenticated: boolean
}

// GET /api/styles/active — the active theme's raw CSS.
export interface ActiveCssResponse {
  css: string
}

// POST /api/cards {action:"generate"} — number of cards generated.
export interface GenerateCardsResponse {
  count: number
}

// Board fetch variants that only return the card (winner verify / preview).
export interface CardResponse {
  card: Card
}

// POST /api/styles {action:"get"} — a single theme with its CSS.
export interface StyleGetResponse {
  style: Style
}

// POST /api/styles {action:"create"} — the new theme's id.
export interface StyleCreateResponse {
  id: number
}

// ── Card list entry (GET /api/cards) ────────────────────────────────────────
// The list endpoint returns a lightweight shape (no board_data).
export interface CardListEntry {
  id: string
  player_name: string
  details: string
}

// ── Board (GET /api/board) ──────────────────────────────────────────────────
export interface BoardResponse {
  card: Card
  game?: BingoGameState | null
  game_details?: string
}

// ── Game (GET /api/game) ────────────────────────────────────────────────────
export interface GameStateResponse {
  game: BingoGameState | null
  winners: string[]
  game_details: string
}

// ── Draw result (POST /api/game {action:"draw"}) ────────────────────────────
export interface DrawResult {
  drawn: BingoDrawnNumber
  game: BingoGameState
  winners: string[]
}

// ── Patterns (GET /api/patterns) ────────────────────────────────────────────
export interface PatternsResponse {
  patterns: Pattern[]
  categories: PatternCategory[]
}

// ── Styles (GET /api/styles) ────────────────────────────────────────────────
export interface StylesResponse {
  styles: Style[]
  active_style_id: string
}

// ── Settings (GET /api/settings) ────────────────────────────────────────────
export interface AppSettings {
  app_title: string
  default_draw_delay: string
  frequent_winner_threshold: string
  frequent_winner_hours: string
  header_font: string
  google_fonts_api_key: string
  /** AniList GraphQL endpoint used for manga lookups. */
  anilist_api_url: string
  /**
   * Per-club Discord webhook URLs, keyed `discord_webhook_url_<club_slug>`
   * (e.g. `discord_webhook_url_yaoi`). Admin-only (redacted for public). Each
   * book club publishes its reading lists to its own channel. See BOOK_CLUBS.
   */
  [key: `discord_webhook_url_${string}`]: string
  /**
   * Per-club Discord *events*-channel webhook URLs, keyed
   * `discord_events_webhook_url_<club_slug>`. Admin-only (redacted for public).
   * Scheduled event posts are sent here. See clubEventsWebhookKey in constants.ts.
   */
  [key: `discord_events_webhook_url_${string}`]: string
}

export interface SettingsResponse {
  settings: AppSettings
  /**
   * Filenames of fonts uploaded via System → Font Upload (e.g. "My Font.ttf").
   * The frontend registers an @font-face for each (family = name without the
   * extension) so they can be used for the header/board font.
   */
  uploaded_fonts?: string[]
}

// ── Winners log (GET /api/winners-log) ──────────────────────────────────────
export interface WinnersLogResponse {
  entries: WinnersLogEntry[]
  total: number
  page: number
  per_page: number
}

export interface FrequentWinnersResponse {
  winners: FrequentWinner[]
}

// ── Raffles ─────────────────────────────────────────────────────────────────
export interface RafflesResponse {
  raffles: Raffle[]
}

export interface RaffleDetailResponse {
  raffle: Raffle
  total_entries?: number
  entries?: RaffleEntry[]
  winner_entry?: RaffleEntry
}

export interface RaffleEnterResponse {
  message: string
  total_entries: number
  total_cost: number
  signup_instructions: string
}

export interface RaffleUploadResponse {
  path: string
}

export interface RaffleWinnerResponse {
  winner: RaffleEntry
}

// POST /api/raffles/{id}/entries {add_entry} — the created/updated entry.
export interface RaffleEntryResponse {
  entry: RaffleEntry
}

// ── Book clubs / reading lists ──────────────────────────────────────────────
export interface ReadingListsResponse {
  reading_lists: ReadingList[]
}

export interface ReadingListDetailResponse {
  reading_list: ReadingList
}

// POST /api/reading-lists/{id}/items — the created/updated item.
export interface ReadingListItemResponse {
  item: ReadingListItem
}

// POST /api/bookclub/upload — full URL of the stored cover image.
export interface BookclubUploadResponse {
  url: string
}

// GET /api/bookclub/lookup — AniList suggestions, shaped like reading list items.
export interface BookclubLookupResponse {
  results: ReadingListItem[]
}

// POST /api/reading-lists/{id}/publish — number of items posted to Discord.
export interface PublishResponse {
  published: number
}

// Form model for the admin reading-list item create/edit form.
export interface ReadingListItemForm {
  id: number
  cover_image: string
  title: string
  summary: string
  format: string
  genres: string
  tropes: string
  chapters: string
  comments: string
  sources: ReadingListSource[]
}

// ── Book club event posts ────────────────────────────────────────────────────
// GET /api/bookclub/events — scheduled events for a club.
export interface BookClubEventsResponse {
  events: BookClubEvent[]
}

// POST /api/bookclub/events {create|update} — the saved event.
export interface BookClubEventResponse {
  event: BookClubEvent
}

// GET /api/bookclub/events/images — existing event images (full URLs).
export interface EventImagesResponse {
  images: string[]
}

// POST /api/bookclub/events/upload — full URL of the stored event image.
export interface EventImageUploadResponse {
  url: string
}

// Form model for the admin event create/edit form. Wall-clock times plus the
// admin's IANA timezone; the server computes the absolute instants.
export interface BookClubEventForm {
  id: number
  title: string
  start_local: string
  timezone: string
  length_hours: number
  location: string
  details: string
  image: string
  post_at_local: string
}

// ── Fonts (System → Font Upload) ────────────────────────────────────────────
// A single font file in <webRoot>/fonts.
export interface FontFile {
  name: string
  size: number
  /** RFC3339 last-modified timestamp. */
  modified: string
}

export interface FontsResponse {
  fonts: FontFile[]
}

// POST /api/fonts/upload — per-file result of a (possibly multi-file) upload.
export interface FontUploadResponse {
  uploaded: string[]
  skipped: { name: string; reason: string }[]
}

// ── Carrd image hosting (System → Carrd Upload) ─────────────────────────────
// A project (folder) under <webRoot>/carrd, served at carrd.senpan.cafe/<folder>.
export interface CarrdProject {
  title: string
  folder: string
  image_count: number
  /** RFC3339 folder last-modified timestamp. */
  modified: string
}

export interface CarrdProjectsResponse {
  projects: CarrdProject[]
}

export interface CarrdProjectCreateResponse {
  ok: boolean
  project: CarrdProject
}

// A single image inside a carrd project folder.
export interface CarrdImage {
  name: string
  size: number
  /** RFC3339 last-modified timestamp. */
  modified: string
}

export interface CarrdImagesResponse {
  folder: string
  /** Relative subpath within the project ("" = project root). */
  path: string
  /** Names of the immediate sub-directories at this path. */
  dirs: string[]
  images: CarrdImage[]
}

// POST /api/carrd/upload — per-file result of a (possibly multi-file) upload.
export interface CarrdUploadResponse {
  uploaded: string[]
  skipped: { name: string; reason: string }[]
}

// Form model for the admin raffle create/edit form.
export interface RaffleForm {
  id: number
  title: string
  description: string
  rules: string
  max_entries: number
  signup_instructions: string
  cost_per_entry: number
  available_from: string
  available_to: string
  prize_image: string
}

// ── WebSocket message types ─────────────────────────────────────────────────
export type WsMessage =
  | { type: 'game_update'; game: BingoGameState | null; game_details?: string; winners?: string[] }
  | { type: 'game_draw'; drawn: BingoDrawnNumber; winners?: string[] }
  | { type: 'cards_update'; cards: CardListEntry[] }
  | { type: 'patterns_update'; patterns: Pattern[]; categories?: PatternCategory[] }
  | { type: 'card_deleted' }
  | { type: 'details_update'; game_details: string }
  | { type: 'style_update'; css: string }
  | { type: 'settings_update'; app_title?: string; header_font?: string; uploaded_fonts?: string[] }
  | { type: 'halftime_minigame' }
