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
  Announcement,
  AnnouncementButton,
  AnnouncementType,
  AnnouncementRole,
  BingoDrawnNumber,
  BingoGameState,
  Card,
  FrequentWinner,
  GamePreset,
  Pattern,
  PatternCategory,
  Raffle,
  RaffleEntry,
  Garapon,
  GaraponPlayer,
  GaraponDraw,
  ReadingList,
  ReadingListItem,
  ReadingListSource,
  Style,
  User,
  WinnersLogEntry,
} from './api.generated'

export type {
  Announcement,
  AnnouncementButton,
  AnnouncementType,
  AnnouncementRole,
  BingoDrawnNumber,
  BingoGame,
  BingoGamePattern,
  BingoGameState,
  Card,
  FrequentWinner,
  GamePreset,
  Pattern,
  PatternCategory,
  Raffle,
  RaffleEntry,
  Garapon,
  GaraponPrize,
  GaraponPlayer,
  GaraponDraw,
  ReadingList,
  ReadingListItem,
  ReadingListSource,
  Style,
  User,
  WinnersLogEntry,
} from './api.generated'

// ── Generic / action responses ───────────────────────────────────────────────
// Many POST action endpoints reply with a simple {"success": true} (or an
// error envelope handled by ApiError). Callers that don't read the body use this.
export interface OkResponse {
  success?: boolean
}

// GET /api/auth and POST /api/auth (login/logout). When authenticated, `user`
// carries the current account so the frontend can gate UI by permission.
export interface AuthCheckResponse {
  authenticated: boolean
  user: User | null
}

// POST /api/auth {action:"login"} — success plus the logged-in user.
export interface LoginResponse {
  success?: boolean
  user: User
}

// POST /api/register — account created (pending admin activation).
export interface RegisterResponse {
  success?: boolean
  message: string
}

// GET /api/users — all accounts (admin only).
export interface UsersResponse {
  users: User[]
}

// GET /api/styles/active — the active theme's raw CSS + decorative flourishes
// (root-relative paths into images/flourishes, "" = built-in art).
export interface ActiveCssResponse {
  css: string
  board_flourish: string
  number_flourish: string
}

// POST /api/cards {action:"generate"} — number of cards generated.
export interface GenerateCardsResponse {
  count: number
}

// POST /api/cards {action:"generate_single"} — the one card created (with its
// assigned player name), so the caller can surface or open it immediately.
export interface GenerateSingleCardResponse {
  count: number
  card: { id: string; player_name: string; board_data: number[][] }
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
  /** ISO timestamp the card was generated ("" for cards predating tracking). */
  created_at: string
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

// ── Game presets (GET /api/presets) ─────────────────────────────────────────
export interface PresetsResponse {
  presets: GamePreset[]
}

// POST /api/presets {action:"create"} — the new preset's id.
export interface PresetCreateResponse {
  id: number
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
  /** Markdown prompt shown on the home page above the board-ID join field. */
  bingo_join_prompt: string
  /**
   * Per-club Discord webhook URLs, keyed `discord_webhook_url_<club_slug>`
   * (e.g. `discord_webhook_url_yaoi`). Admin-only (redacted for public). Each
   * book club publishes its reading lists to its own channel. See BOOK_CLUBS.
   */
  [key: `discord_webhook_url_${string}`]: string
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

export interface RaffleWinnerResponse {
  winner: RaffleEntry
}

// POST /api/raffles/{id}/entries {add_entry} — the created/updated entry.
export interface RaffleEntryResponse {
  entry: RaffleEntry
}

// ── Garapon (festival lottery drum) ─────────────────────────────────────────
// GET /api/garapons — admin list (each carries player_count/draw_count aggregates).
export interface GaraponsResponse {
  garapons: Garapon[]
}

// GET /api/garapons/{id} — admin detail: the garapon (with prizes), its drawing
// links, and the full draw log.
export interface GaraponDetailResponse {
  garapon: Garapon
  players: GaraponPlayer[]
  draws: GaraponDraw[]
}

// POST /api/garapons/{id}/players {create_player} — the new drawing link.
export interface GaraponPlayerResponse {
  player: GaraponPlayer
}

// GET /api/garapon/{token} — the public player view. `garapon.prizes` carry ball
// colors + which is grand but NOT the appearance rates (rate is zeroed). `player`
// is the trimmed shape (name + allowance/usage, no token); `draws` is this
// player's own history.
export interface GaraponPublicPlayer {
  player_name: string
  max_draws: number
  draws_used: number
}
export interface GaraponPublicResponse {
  garapon: Garapon
  player: GaraponPublicPlayer
  draws: GaraponDraw[]
}

// POST /api/garapon/{token}/draw — the recorded draw + the fresh usage counts.
export interface GaraponDrawResponse {
  draw: GaraponDraw
  draws_used: number
  max_draws: number
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

// ── Announcement management ──────────────────────────────────────────────────
// GET /api/announcement-types — Discord destinations.
export interface AnnouncementTypesResponse {
  types: AnnouncementType[]
}

// POST /api/announcement-types {create|update} — the saved type.
export interface AnnouncementTypeResponse {
  type: AnnouncementType
}

// GET /api/announcement-roles — taggable Discord roles.
export interface AnnouncementRolesResponse {
  roles: AnnouncementRole[]
}

// POST /api/announcement-roles {create|update} — the saved role.
export interface AnnouncementRoleResponse {
  role: AnnouncementRole
}

// GET /api/announcements — all announcements (with type_name joined).
export interface AnnouncementsResponse {
  announcements: Announcement[]
}

// POST /api/announcements {create|update|send_now|skip_next} — the saved announcement.
export interface AnnouncementResponse {
  announcement: Announcement
}

// Announcement images are now sourced from the central Images page (categories
// "Announcement Main" / "Announcement Thumbnail") via the image endpoints below.

// Form model for the admin announcement-type create/edit form.
export interface AnnouncementTypeForm {
  id: number
  name: string
  webhook_url: string
}

// Form model for the admin taggable-role create/edit form.
export interface AnnouncementRoleForm {
  id: number
  name: string
  role_id: string
}

/**
 * Recurrence builder kinds for the schedule UI.
 *   '' = unscheduled (manual only); 'once'|'daily'|'weekly'|'monthly' as labelled.
 */
export type ScheduleKind = '' | 'once' | 'daily' | 'weekly' | 'monthly'

/**
 * Discord timestamp style letter, used to render an announcement's start/end
 * times as `<t:unix:STYLE>` tokens so each viewer sees their own zone.
 */
export type DiscordTimeFormat = 't' | 'T' | 'd' | 'D' | 'f' | 'F' | 'R'

/**
 * Selectable Discord timestamp styles (value + human label) shown in the form's
 * "Time format" picker, in a sensible reading order. The example text mirrors
 * how Discord renders each style. Keep in sync with the backend's
 * `validTimeFormats`.
 */
export const DISCORD_TIME_FORMATS: { value: DiscordTimeFormat; label: string }[] = [
  { value: 'f', label: 'Short date & time — June 13, 2026 7:00 PM' },
  { value: 'F', label: 'Long date & time — Saturday, June 13, 2026 7:00 PM' },
  { value: 't', label: 'Short time — 7:00 PM' },
  { value: 'T', label: 'Long time — 7:00:00 PM' },
  { value: 'd', label: 'Short date — 06/13/2026' },
  { value: 'D', label: 'Long date — June 13, 2026' },
  { value: 'R', label: 'Relative — in 3 days' },
]

/**
 * Form model for the admin announcement create/edit form. Holds *local*
 * wall-clock strings + a friendly recurrence builder. `start_local`/`end_local`/
 * `once_local` are `datetime-local` values (converted to absolute UTC on save
 * via `lib/datetime.ts`); `time_local` is an "HH:mm" wall-clock time anchored to
 * `timezone` (an IANA name, so recurring times survive DST); `weekdays` are
 * weekday numbers (0=Sun..6=Sat) in that zone.
 */
export interface AnnouncementForm {
  id: number
  type_id: number
  title: string
  details: string
  image: string
  /** Small top-right embed thumbnail URL (empty = none). Shares uploads with image. */
  thumbnail: string
  /** Embed accent colour as "#rrggbb" (empty falls back to the brand default). */
  color: string
  /** Optional free-text location (e.g. a Discord voice channel). */
  location: string
  start_local: string
  end_local: string
  /** Discord timestamp style for the start display (see DISCORD_TIME_FORMATS). */
  start_format: DiscordTimeFormat
  /** Discord timestamp style for the end display (see DISCORD_TIME_FORMATS). */
  end_format: DiscordTimeFormat
  /** Re-anchor start/end onto the day each post goes out (recurring day-of events). */
  dynamic_dates: boolean
  schedule_kind: ScheduleKind
  timezone: string
  once_local: string
  time_local: string
  weekdays: number[]
  week_of_month: number
  /** Optional Discord link buttons (max 5) rendered beneath the embed. */
  buttons: AnnouncementButton[]
  /**
   * Optional role tag posted above the embed: '' (don't tag), 'everyone'
   * (@everyone), or 'role:<announcement_role_id>' (a managed taggable role).
   */
  mention: string
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
  /** Total media files across the project (root + nested sub-folders). */
  file_count: number
  /** Number of nested sub-folders (recursive). */
  subfolder_count: number
  /** Combined size in bytes of all media files in the project. */
  total_size: number
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

// ── Central image hosting (System → Images) ─────────────────────────────────
// An image category maps to a subdirectory of <webRoot>/images. Three are
// permanent (announcements_main, announcements_thumb, raffles); admins may add
// custom ones. Used by the Images page and by the announcement/raffle pickers.
export interface ImageCategory {
  name: string
  /** Subdirectory of <webRoot>/images this category maps to. */
  dir: string
  /** Permanent categories cannot be renamed or deleted. */
  permanent: boolean
  /** Number of image files in the category directory. */
  file_count: number
  /** Combined size in bytes of the images in the category. */
  total_size: number
}

export interface ImageCategoriesResponse {
  categories: ImageCategory[]
}

export interface ImageCategoryActionResponse {
  ok: boolean
  category: ImageCategory
}

// A single image within a category directory.
export interface ImageEntry {
  name: string
  /** Absolute public URL (used by announcement embeds, which need absolute URLs). */
  url: string
  /** Root-relative web path ("images/<dir>/<name>"); raffles store this. */
  path: string
  size: number
  /** RFC3339 last-modified timestamp. */
  modified: string
}

export interface ImagesResponse {
  dir: string
  images: ImageEntry[]
}

// POST /api/images/upload — per-file result of a (possibly multi-file) upload.
export interface ImagesUploadResponse {
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

// Form models for the admin garapon create/edit form. A prize row carries a name,
// ball color (hex), an appearance-rate weight, and whether it's the grand prize.
export interface GaraponPrizeForm {
  name: string
  ball_color: string
  rate: number
  is_grand: boolean
}
export interface GaraponForm {
  id: number
  title: string
  details: string
  grand_prize_image: string
  prizes: GaraponPrizeForm[]
}

// ── WebSocket message types ─────────────────────────────────────────────────
export type WsMessage =
  | { type: 'game_update'; game: BingoGameState | null; game_details?: string; winners?: string[] }
  | { type: 'game_draw'; drawn: BingoDrawnNumber; winners?: string[] }
  | { type: 'cards_update'; cards: CardListEntry[] }
  | { type: 'patterns_update'; patterns: Pattern[]; categories?: PatternCategory[] }
  | { type: 'card_deleted' }
  | { type: 'details_update'; game_details: string }
  | { type: 'style_update'; css: string; board_flourish?: string; number_flourish?: string }
  | { type: 'settings_update'; app_title?: string; header_font?: string; uploaded_fonts?: string[] }
  | { type: 'halftime_minigame' }
  | { type: 'draw_delay_update'; delay: number }
  // Thin "an admin resource changed" signal (no payload): an admin viewing that
  // resource refetches it via REST. `resource` is a key like 'garapons',
  // 'raffles', 'announcements', 'bookclub', 'presets', 'users', etc.
  | { type: 'resource_changed'; resource: string }
