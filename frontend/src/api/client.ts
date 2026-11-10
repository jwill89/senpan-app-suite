import createClient from 'openapi-fetch'
import { ApiError, handleUnauthorized } from '@/lib/api'
import type { components, paths } from './schema'

// The API client for the operations on huma, typed from the generated schema (`task gen`
// writes openapi.json and schema.d.ts beside this file; never edit either, and never
// hand-write a type the schema has). The routes not yet on huma go through lib/api.ts
// until their domain moves. A list in a query is sent as huma reads
// one: comma-separated in a single parameter.
export const api = createClient<paths>({
  baseUrl: '/api/v1',
  credentials: 'include',
  querySerializer: { array: { style: 'form', explode: false } },
})

// A 401 means the session has gone, wherever it is met - except from signing in
// (/auth/...), where it means the username, password or passkey was wrong.
api.use({
  onResponse({ response, schemaPath }) {
    if (response.status === 401 && !schemaPath.startsWith('/auth/')) {
      handleUnauthorized()
    }
    return response
  },
})

type Schemas = components['schemas']
export type AppVersion = Schemas['AppVersion']
export type ClientConfig = Schemas['ClientConfig']
export type ErrorModel = Schemas['ErrorModel']
export type Settings = Schemas['Settings']
export type SettingsInput = Schemas['SettingsInput']
export type Integrations = Schemas['Integrations']
export type UploadedFont = Schemas['UploadedFont']
export type Logs = Schemas['Logs']
export type LogEntry = Schemas['LogEntry']
export type LogLevel = Schemas['LogLevel']['level']
export type Style = Schemas['Style']
export type StyleInput = Schemas['StyleInput']
export type StyleList = Schemas['StyleList']
export type StyleSummary = Schemas['StyleSummary']
export type ThemeCSS = Schemas['ThemeCSS']
export type PublicStyle = Schemas['PublicStyle']
export type Account = Schemas['Account']
export type User = Schemas['User']
export type Passkey = Schemas['Passkey']
export type TokenSummary = Schemas['TokenSummary']
export type NewToken = Schemas['NewToken']
export type Pattern = Schemas['Pattern']
export type PatternCategory = Schemas['PatternCategory']
export type PatternList = Schemas['PatternList']
export type GamePreset = Schemas['GamePreset']
export type GamePresetInput = Schemas['GamePresetInput']
export type WinnersLogEntry = Schemas['WinnersLogEntry']
export type FrequentWinner = Schemas['FrequentWinner']
export type Card = Schemas['Card']
export type CardSummary = Schemas['CardSummary']
export type CardList = Schemas['CardList']
export type CardChanges = Schemas['CardChanges']
export type GameState = Schemas['GameState']
export type Game = Schemas['Game']
export type GamePattern = Schemas['GamePattern']
export type DrawnNumber = Schemas['DrawnNumber']
export type LastDraw = Schemas['LastDraw']
export type DrawResult = Schemas['DrawResult']
export type Board = Schemas['Board']

/** The message a failed call shows: the server's `detail`, which says why, else its title. */
export function problemMessage(error: ErrorModel | undefined, response: Response): string {
  return error?.detail || error?.title || `Request failed (HTTP ${response.status})`
}

/**
 * A call's data, or an ApiError carrying the problem's message (`detail`), its status
 * and the problem itself (with `errors` naming each field of a failed validation). For
 * a caller that only shows a failure; one that tells failures apart reads the call's
 * `{ data, error, response }` itself.
 */
export async function unwrap<T>(
  call: Promise<{ data?: T; error?: ErrorModel; response: Response }>,
): Promise<T> {
  const { data, error, response } = await call
  if (response.ok) {
    return data as T
  }
  throw new ApiError(problemMessage(error, response), response.status, error)
}
