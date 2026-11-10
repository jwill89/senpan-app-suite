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
export type Affiliate = Schemas['Affiliate']
export type AffiliateInput = Schemas['AffiliateInput']
export type AffiliateHour = Schemas['AffiliateHour']
export type DiscordWebhook = Schemas['DiscordWebhook']

export type TeaRoom = Schemas['TeaRoom']
export type TeaRoomInput = Schemas['TeaRoomInput']
export type TeaRoomChanges = Schemas['TeaRoomChanges']
export type TeaRoomList = Schemas['TeaRoomList']
export type PublicTeaRoom = Schemas['PublicTeaRoom']

export type Raffle = Schemas['Raffle']
export type RaffleInput = Schemas['RaffleInput']
export type RaffleList = Schemas['RaffleList']
export type RaffleDetail = Schemas['RaffleDetail']
export type RaffleWinner = Schemas['RaffleWinner']
export type RaffleEntry = Schemas['RaffleEntry']
export type RaffleEntryInput = Schemas['RaffleEntryInput']
export type RaffleSettlement = Schemas['RaffleSettlement']
export type RaffleEnterInput = Schemas['RaffleEnterInput']
export type RaffleEnterResult = Schemas['RaffleEnterResult']
export type RaffleLookup = Schemas['RaffleLookup']
export type RaffleLookupEntry = Schemas['RaffleLookupEntry']

export type Announcement = Schemas['Announcement']
export type AnnouncementInput = Schemas['AnnouncementInput']
export type AnnouncementButton = Schemas['AnnouncementButton']
export type AnnouncementType = Schemas['AnnouncementType']
export type AnnouncementTypeInput = Schemas['AnnouncementTypeInput']
export type AnnouncementRole = Schemas['AnnouncementRole']
export type AnnouncementRoleInput = Schemas['AnnouncementRoleInput']

export type ReadingList = Schemas['ReadingList']
export type ReadingListSummary = Schemas['ReadingListSummary']
export type ReadingListItem = Schemas['ReadingListItem']
export type ReadingListItemInput = Schemas['ReadingListItemInput']
export type ReadingListSource = Schemas['ReadingListSource']

export type Garapon = Schemas['Garapon']
export type GaraponInput = Schemas['GaraponInput']
export type GaraponPrize = Schemas['GaraponPrize']
export type GaraponPrizeInput = Schemas['GaraponPrizeInput']
export type GaraponList = Schemas['GaraponList']
export type GaraponDetail = Schemas['GaraponDetail']
export type GaraponPlayer = Schemas['GaraponPlayer']
export type GaraponPlayerInput = Schemas['GaraponPlayerInput']
export type GaraponDraw = Schemas['GaraponDraw']
export type PublicGarapon = Schemas['PublicGarapon']
export type PublicGaraponPrize = Schemas['PublicGaraponPrize']
export type PublicGaraponDraw = Schemas['PublicGaraponDraw']
export type PublicGaraponPlayer = Schemas['PublicGaraponPlayer']
export type GaraponPlayerSummary = Schemas['GaraponPlayerSummary']
export type GaraponDrawResult = Schemas['GaraponDrawResult']

export type Placement = Schemas['Placement']
export type StampRally = Schemas['StampRally']
export type StampRallyInput = Schemas['StampRallyInput']
export type StampRallyStamp = Schemas['StampRallyStamp']
export type StampRallyStampInput = Schemas['StampRallyStampInput']
export type StampRallyPrize = Schemas['StampRallyPrize']
export type StampRallyPrizeInput = Schemas['StampRallyPrizeInput']
export type StampRallyList = Schemas['StampRallyList']
export type StampRallyDetail = Schemas['StampRallyDetail']
export type StampRallyCard = Schemas['StampRallyCard']
export type StampRallyLogEntry = Schemas['StampRallyLogEntry']
export type SignupRally = Schemas['SignupRally']
export type StampSignup = Schemas['StampSignup']
export type StampLookupEntry = Schemas['StampLookupEntry']
export type PublicStampCard = Schemas['PublicStampCard']
export type PublicStampRally = Schemas['PublicStampRally']
export type PublicStamp = Schemas['PublicStamp']
export type PublicPrize = Schemas['PublicPrize']
export type StampResult = Schemas['StampResult']

export type EventTime = Schemas['EventTime']
export type EventTimeInput = Schemas['EventTimeInput']
export type FestivalMap = Schemas['FestivalMap']
export type FestivalMapInput = Schemas['FestivalMapInput']
export type FestivalMapList = Schemas['FestivalMapList']
export type FestivalStall = Schemas['FestivalStall']
export type FestivalStallInput = Schemas['FestivalStallInput']
export type FestivalStallOccupant = Schemas['FestivalStallOccupant']
export type FestivalStallOccupantInput = Schemas['FestivalStallOccupantInput']
export type PublicFestivalMap = Schemas['PublicFestivalMap']
export type PublicFestivalMapSummary = Schemas['PublicFestivalMapSummary']
export type PublicFestivalStall = Schemas['PublicFestivalStall']
export type PublicStallOccupant = Schemas['PublicStallOccupant']
export type PublicStallAffiliate = Schemas['PublicStallAffiliate']
export type PublicStallRaffle = Schemas['PublicStallRaffle']

export type FestivalDashboard = Schemas['FestivalDashboard']
export type FestivalDashboardRally = Schemas['FestivalDashboardRally']
export type FestivalDashboardGarapon = Schemas['FestivalDashboardGarapon']
export type FestivalRaffle = Schemas['FestivalRaffle']

export type EmaBoard = Schemas['EmaBoard']
export type EmaBoardInput = Schemas['EmaBoardInput']
export type EmaWindow = Schemas['EmaWindow']
export type EmaWindowInput = Schemas['EmaWindowInput']
export type EmaPlaque = Schemas['EmaPlaque']
export type PublicEmaPlaque = Schemas['PublicEmaPlaque']
export type PublicEmaWindow = Schemas['PublicEmaWindow']
export type FestivalEma = Schemas['FestivalEma']

export type ImageCategory = Schemas['ImageCategory']
export type ImageCategoryList = Schemas['ImageCategoryList']
export type ImageCategoryInput = Schemas['ImageCategoryInput']
export type ImageCategoryChanges = Schemas['ImageCategoryChanges']
export type Image = Schemas['Image']
export type ImageList = Schemas['ImageList']
export type ImageUpload = Schemas['ImageUpload']
export type SkippedUpload = Schemas['SkippedUpload']

export type CarrdProject = Schemas['CarrdProject']
export type CarrdProjectList = Schemas['CarrdProjectList']
export type CarrdProjectInput = Schemas['CarrdProjectInput']
export type CarrdProjectChanges = Schemas['CarrdProjectChanges']
export type CarrdFolder = Schemas['CarrdFolder']
export type CarrdFile = Schemas['CarrdFile']
export type CarrdSubfolder = Schemas['CarrdSubfolder']
export type CarrdUpload = Schemas['CarrdUpload']

export type Font = Schemas['Font']
export type FontVariant = Schemas['FontVariant']
export type FontList = Schemas['FontList']
export type FontChanges = Schemas['FontChanges']
export type FontUpload = Schemas['FontUpload']
export type FontFileRename = Schemas['FontFileRename']

export type BookClubCover = Schemas['BookClubCover']

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
