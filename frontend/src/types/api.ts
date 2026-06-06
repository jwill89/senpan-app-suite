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
  Card,
  FrequentWinner,
  Pattern,
  PatternCategory,
  Raffle,
  RaffleEntry,
  Style,
  WinnersLogEntry,
} from './api.generated'

export type {
  BingoDrawnNumber,
  BingoGame,
  BingoGamePattern,
  BingoGameState,
  Card,
  FrequentWinner,
  Pattern,
  PatternCategory,
  Raffle,
  RaffleEntry,
  Style,
  WinnersLogEntry,
} from './api.generated'

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
}

export interface SettingsResponse {
  settings: AppSettings
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
  | { type: 'settings_update'; app_title?: string; header_font?: string }
  | { type: 'halftime_minigame' }
