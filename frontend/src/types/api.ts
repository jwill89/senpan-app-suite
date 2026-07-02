/**
 * API types barrel.
 *
 * Re-exports the tygo-generated types from `api.generated.ts` — both the domain
 * models AND the request/response envelopes, which now live as Go structs in the
 * backend `model` package (so the Go handlers are the single source of truth for
 * every wire shape). This file adds only the handful of types that are
 * intentionally frontend-only: the admin *form* models, the richer `AppSettings`
 * view, the WebSocket message union, and a couple of UI enums/constants.
 *
 * Regenerate the domain + envelope types with `npm run gen:types` after changing
 * any Go `model` struct.
 */
import type {
  // Domain types referenced by the form models and WsMessage below.
  Placement,
  AnnouncementButton,
  ReadingListSource,
  BingoDrawnNumber,
  BingoGameState,
  CardListEntry,
  Pattern,
  PatternCategory,
  TokenInfo,
} from './api.generated'

// ── Domain models (generated from the Go `model` package) ────────────────────
export type {
  Affiliate,
  AffiliateHour,
  Placement,
  StampRally,
  StampRallyStamp,
  StampRallyPrize,
  StampRallyCard,
  StampRallyCollected,
  StampRallyLogEntry,
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
  TokenInfo,
} from './api.generated'

// ── Response envelopes (generated; backend `model` is the source of truth) ───
export type {
  // Shared
  OKResponse,
  DeletedResponse,
  DeletedCountResponse,
  StatusResponse,
  NamedOKResponse,
  RenamedResponse,
  PausedResponse,
  SkippedUpload,
  // Auth / users / account
  AuthCheckResponse,
  LoginResponse,
  LogoutResponse,
  RegisterResponse,
  UsersResponse,
  AccountTokenGenerateResponse,
  TokenRevokeResponse,
  Passkey,
  PasskeysResponse,
  // Bingo: board / cards / game / patterns / presets / styles
  ActiveCSSResponse,
  CardListEntry,
  CardsListResponse,
  GeneratedCard,
  GenerateCardsResponse,
  GeneratedNamedCard,
  GenerateSingleCardResponse,
  CardResponse,
  BoardResponse,
  GameStateResponse,
  DrawResult,
  EndGameResponse,
  PatternsResponse,
  CreatedPattern,
  PatternCreateResponse,
  CategoriesResponse,
  CategoryCreateResponse,
  PresetsResponse,
  PresetCreateResponse,
  StylesResponse,
  StyleGetResponse,
  StyleCreateResponse,
  // Raffles
  RafflesResponse,
  RaffleResponse,
  RaffleDetailResponse,
  RaffleEnterResponse,
  RaffleEntryResponse,
  RaffleWinnerResponse,
  // Garapons
  GaraponsResponse,
  GaraponResponse,
  GaraponDetailResponse,
  GaraponPlayerResponse,
  PublicGarapon,
  GaraponPublicPlayer,
  GaraponPublicResponse,
  GaraponDrawResponse,
  // Affiliates
  AffiliatesResponse,
  AffiliateResponse,
  // Stamp rally
  StampRalliesResponse,
  StampRallyResponse,
  StampRallyDetailResponse,
  StampRallyCardResponse,
  StampRallyLogsResponse,
  PublicStampRally,
  PublicStamp,
  PublicPrize,
  PublicStampCard,
  StampSubmitResponse,
  // Book club / reading lists
  ReadingListsResponse,
  ReadingListDetailResponse,
  ReadingListItemResponse,
  BookclubUploadResponse,
  BookclubLookupResponse,
  PublishResponse,
  // Announcements
  AnnouncementTypesResponse,
  AnnouncementTypeResponse,
  AnnouncementRolesResponse,
  AnnouncementRoleResponse,
  AnnouncementsResponse,
  AnnouncementResponse,
  // Winners log
  WinnersLogResponse,
  FrequentWinnersResponse,
  // Fonts / Carrd / Images
  FontFile,
  FontsResponse,
  FontUploadResponse,
  CarrdProject,
  CarrdProjectsResponse,
  CarrdProjectCreateResponse,
  CarrdImage,
  CarrdImagesResponse,
  CarrdUploadResponse,
  ImageCategory,
  ImageCategoriesResponse,
  ImageCategoryActionResponse,
  ImageEntry,
  ImagesResponse,
  ImagesUploadResponse,
} from './api.generated'

// GET /api/account/token returns the account's token metadata, which IS the
// domain `TokenInfo` shape (the handler writes it directly). Aliased for a
// self-documenting name at call sites.
export type AccountTokenInfoResponse = TokenInfo

// ── Settings (kept hand-written) ─────────────────────────────────────────────
// The backend response types `settings` as a dynamic string→string map; this
// richer view enumerates the known keys (plus the per-club webhook index
// signature) for better editor support on the Settings page. Kept in sync by
// hand with the backend `settingsKeys`.
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

// GET /api/settings — the richer settings view (named keys) + uploaded fonts.
export interface SettingsResponse {
  settings: AppSettings
  /**
   * Filenames of fonts uploaded via System → Font Upload (e.g. "My Font.ttf").
   * The frontend registers an @font-face for each (family = name without the
   * extension) so they can be used for the header/board font.
   */
  uploaded_fonts?: string[]
}

// ── Form models (frontend-only) ──────────────────────────────────────────────

/**
 * A reading-list source row in the item form. `_uid` is a client-only stable key
 * for the repeater (see lib/uid.ts); it's stripped before the payload is sent.
 */
export interface ReadingListSourceForm extends ReadingListSource {
  _uid?: number
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
  sources: ReadingListSourceForm[]
}

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
 * A Discord button row in the announcement form. `_uid` is a client-only stable
 * key for the repeater (see lib/uid.ts); it's stripped when the payload is built.
 */
export interface AnnouncementButtonForm extends AnnouncementButton {
  _uid?: number
}

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
  buttons: AnnouncementButtonForm[]
  /**
   * Optional role tag posted above the embed: '' (don't tag), 'everyone'
   * (@everyone), or 'role:<announcement_role_id>' (a managed taggable role).
   */
  mention: string
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
  /** Client-only stable key for the prize repeater (see lib/uid.ts); stripped
   *  from the payload before it's sent to the server. */
  _uid?: number
}
export interface GaraponForm {
  id: number
  title: string
  details: string
  grand_prize_image: string
  /** Optional link to an open Stamp Rally (null = not linked). */
  stamp_rally_id: number | null
  prizes: GaraponPrizeForm[]
}

// Form models for the admin affiliate create/edit form. The hours form mirrors
// model.AffiliateHour; owners is a list of plain name strings.
export interface AffiliateHourForm {
  label: string
  start: string
  end: string
  /** Client-only stable key for the hours repeater (see lib/uid.ts). */
  _uid?: number
}
/**
 * An owner row in the affiliate form. Owners are plain name strings on the wire,
 * but the form wraps each in an object so the repeater can key on a stable
 * `_uid` (see lib/uid.ts) instead of the array index; the payload sends
 * `value`s only.
 */
export interface AffiliateOwnerForm {
  value: string
  _uid?: number
}
export interface AffiliateForm {
  id: number
  name: string
  owners: AffiliateOwnerForm[]
  location: string
  timezone: string
  hours: AffiliateHourForm[]
  details: string
  logo: string
  screenshot: string
}

// Form models for the admin stamp-rally editor. Placement is the same %-based box as
// the model. A stamp's affiliate_id is null for the "Senpan Tea House" default.
export interface StampRallyStampForm {
  id: number
  affiliate_id: number | null
  image: string
  password: string
  placement: Placement
  active_from: string
  active_to: string
  paused: boolean
}
export interface StampRallyPrizeForm {
  id: number
  name: string
  image: string
  placement: Placement
}
export interface StampRallyForm {
  id: number
  title: string
  card_image: string
  not_stamped_image: string
  available_from: string
  available_to: string
  details: string
  redeem_instructions: string
  stamps: StampRallyStampForm[]
  prizes: StampRallyPrizeForm[]
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
