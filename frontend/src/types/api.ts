/**
 * API types barrel.
 *
 * Re-exports the tygo-generated types from `api.generated.ts` - both the domain
 * models AND the request/response envelopes, which now live as Go structs in the
 * backend `model` package (so the Go handlers are the single source of truth for
 * every wire shape). This file adds only the handful of types that are
 * intentionally frontend-only: the admin *form* models and a couple of UI
 * enums/constants. The domains already on huma, and the live events, take their
 * types from the generated API schema instead (`@/api/client`, `@/lib/feed`).
 *
 * Regenerate the domain + envelope types with `npm run gen:types` after changing
 * any Go `model` struct.
 */
import type {
  // Domain types referenced by the form models below.
  Placement,
} from './api.generated'

// -- Domain models (generated from the Go `model` package) --------------------
export type {
  Placement,
  StampRally,
  StampRallyStamp,
  StampRallyPrize,
  StampRallyCard,
  StampRallyCollected,
  StampRallyLogEntry,
  EventTime,
  FestivalMap,
  FestivalStall,
  FestivalStallOccupant,
  Raffle,
  Garapon,
  GaraponPrize,
  GaraponPlayer,
  GaraponDraw,
  LogEntry,
} from './api.generated'

// -- Response envelopes (generated; backend `model` is the source of truth) ---
export type {
  // Shared
  OKResponse,
  DeletedResponse,
  StatusResponse,
  NamedOKResponse,
  RenamedResponse,
  PausedResponse,
  SkippedUpload,
  // Raffles
  RaffleLookupEntry,
  // Garapons
  GaraponsResponse,
  GaraponResponse,
  GaraponDetailResponse,
  GaraponPlayerResponse,
  PublicGarapon,
  GaraponPublicPlayer,
  GaraponPublicResponse,
  GaraponDrawResponse,
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
  SignupRally,
  SignupRalliesResponse,
  StampSignupResponse,
  StampLookupEntry,
  StampLookupResponse,
  // Festival maps
  FestivalMapsResponse,
  FestivalMapResponse,
  FestivalMapDetailResponse,
  PublicFestivalMapSummary,
  PublicFestivalMapsResponse,
  PublicStallAffiliate,
  PublicStallOccupant,
  PublicStallRaffle,
  PublicFestivalStall,
  PublicFestivalMap,
  FestivalDashboard,
  FestivalDashboardRally,
  FestivalDashboardGarapon,
  FestivalDashboardRaffle,
  FestivalDashboardEma,
  // Ema Plaques
  EmaBoard,
  EmaWindow,
  EmaPlaque,
  EmaBoardsResponse,
  EmaBoardResponse,
  EmaPlaquesResponse,
  EmaPlaqueResponse,
  PublicEmaWindow,
  PublicEmaPlaque,
  GoogleFontsResponse,
  // Book club cover upload
  BookclubUploadResponse,
  // Fonts / Carrd / Images
  Font,
  FontVariant,
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

// -- Form models (frontend-only) ----------------------------------------------

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
  { value: 'f', label: 'Short date & time - June 13, 2026 7:00 PM' },
  { value: 'F', label: 'Long date & time - Saturday, June 13, 2026 7:00 PM' },
  { value: 't', label: 'Short time - 7:00 PM' },
  { value: 'T', label: 'Long time - 7:00:00 PM' },
  { value: 'd', label: 'Short date - 06/13/2026' },
  { value: 'D', label: 'Long date - June 13, 2026' },
  { value: 'R', label: 'Relative - in 3 days' },
]

/**
 * How a raffle prices entries, and whether it takes sign-ups here at all.
 * Mirrors the server's `raffles.Mode*` constants.
 *   'details' - published for reference; players sign up somewhere else.
 *   'single'  - every ticket costs `costPerEntry` (the original behavior).
 *   'custom'  - the Nth ticket costs `tierCosts[N-1]`.
 */
export type RaffleMode = 'details' | 'single' | 'custom'

/** Raffle entry modes with their admin-facing labels, in picker order. */
export const RAFFLE_MODES: { value: RaffleMode; label: string; help: string }[] = [
  {
    value: 'details',
    label: 'Details Only',
    help: 'Show the raffle without a sign-up form - players follow the sign-up instructions to enter elsewhere.',
  },
  {
    value: 'single',
    label: 'Single Cost per Entry',
    help: 'Every entry costs the same.',
  },
  {
    value: 'custom',
    label: 'Custom Cost per Entry',
    help: 'Each entry has its own price, and the number of prices sets the per-player limit.',
  },
]

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
  /** Draws a link carries when nobody picks a number (public sign-up, or a blank admin field). */
  default_draws: number
  prizes: GaraponPrizeForm[]
}

/** A stamp rally stall's kind. Everything that predates types is a food stamp. */
export type StampType = 'food' | 'game'
/** What finishing a stamp card takes - the whole card, or a count of each type. */
export type RallyCompletionMode = 'all' | 'counts'

// Form models for the admin stamp-rally editor. Placement is the same %-based box as
// the model. A stamp's affiliate_id is null for the "Senpan Tea House" default.
export interface StampRallyStampForm {
  id: number
  /** Festival Map pitch OCCUPANT this stamp belongs to; null when unlinked. */
  occupant_id: number | null
  affiliate_id: number | null
  image: string
  password: string
  stamp_type: StampType
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
  redeem_image: string
  /** Opt-in that lists the rally publicly and lets participants issue themselves a card. */
  public_signup: boolean
  /** 'all' = collect the whole card; 'counts' = required_food + required_game. */
  completion_mode: RallyCompletionMode
  required_food: number
  required_game: number
  /** Optional Festival Map link; when set, each stamp names one of its stalls. */
  festival_map_id: number | null
  stamps: StampRallyStampForm[]
  prizes: StampRallyPrizeForm[]
}

// -- Festival Map -------------------------------------------------------------

/** A festival map's publish state. Only `published` maps reach the public pages. */
export type FestivalMapStatus = 'in_progress' | 'published' | 'closed'
/** How a stall is drawn on the map. */
export type StallShape = 'circle' | 'rect'
/** What a stall offers - seeds its default shape/color and its stamp's type. */
export type StallType = 'game' | 'food' | 'both' | 'other'

/**
 * One datetime range on the festival-map form. The wire format is UTC RFC-3339
 * (model.EventTime); the form holds the admin's local `datetime-local` strings
 * and converts on save, exactly like the rally availability window.
 */
export interface EventTimeForm {
  label: string
  start: string
  end: string
  /** Client-only stable key for the repeater (see lib/uid.ts); never sent. */
  _uid?: number
}

// Form models for the admin festival-map editor.
//
// A stall on the plan is a PITCH - where it sits and how it is drawn - holding one
// or more OCCUPANTS, the businesses standing in it. Most pitches keep the same
// occupant all festival; a booth that changes hands between days has one per day,
// each with its own affiliate, offering and times.
export interface FestivalStallOccupantForm {
  id: number
  /** null = the venue's own booth (Senpan Tea House). */
  affiliate_id: number | null
  /** Optional: the map labels a pitch by its AFFILIATE, not by this. */
  title: string
  description: string
  /** This occupant's own event page ('' = none; no fallback to the affiliate's). */
  event_carrd: string
  stall_type: StallType
  /** Caption shown under the title when `stall_type` is 'other' ('' = none). */
  type_label: string
  /** When this occupant is here ([] = the whole festival). */
  times: EventTimeForm[]
  /** Client-only stable key for the occupant list; never sent. */
  _uid?: number
}
export interface FestivalStallForm {
  id: number
  shape: StallShape
  /** '#rrggbb', or '' to follow the first occupant's stall-type default color. */
  color: string
  /** Halo drawn when a visitor selects this pitch ('' = the app highlight). */
  selection_color: string
  /** Colour of this pitch's label ('' = white). */
  text_color: string
  placement: Placement
  occupants: FestivalStallOccupantForm[]
  /** Client-only stable key for the pitch list; never sent. */
  _uid?: number
}
export interface FestivalMapForm {
  id: number
  title: string
  /** Optional shortcode, e.g. 'obon-2026' ('' = the map is linked by id only). */
  slug: string
  description: string
  map_image: string
  times: EventTimeForm[]
  stalls: FestivalStallForm[]
}

/**
 * One plaque window in the Ema board form. `start_at`/`end_at` hold the LOCAL
 * wall-clock value an `<input type="datetime-local">` expects; the store converts
 * them to and from UTC. `artist_limit` 0 means no limit; `artist_count` is the
 * requests the saved window already holds, shown beside it and never sent. `_uid`
 * is a client-only stable key for the repeater.
 */
export interface EmaWindowForm {
  id: number
  label: string
  start_at: string
  end_at: string
  webhook_url: string
  artist_limit: number
  artist_count?: number
  _uid?: number
}

/** The Ema board form (create when `id` is 0). */
export interface EmaBoardForm {
  id: number
  festival_map_id: number | null
  title: string
  template_image: string
  instructions: string
  discord_link: string
  allow_self: boolean
  allow_site: boolean
  allow_artist: boolean
  artist_name: string
  windows: EmaWindowForm[]
}
