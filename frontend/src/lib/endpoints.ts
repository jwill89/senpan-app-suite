/**
 * Typed API endpoint layer.
 *
 * A single, typed surface over the raw `api()` helper: every backend path is
 * wrapped in a function whose parameters and return type are bound to the
 * shapes in `types/api.ts` (which mirror the Go handlers + the tygo-generated
 * `model` types). Stores call these instead of hand-annotating `api<T>(...)`
 * with stringly-typed paths, so:
 *
 *   - endpoint paths + action names live in exactly one place;
 *   - request bodies are checked against the expected shape at compile time;
 *   - response types flow to call sites automatically;
 *   - backend/frontend drift surfaces as a type error after `npm run gen:types`.
 *
 * Grouped by resource. Action-based POST endpoints expose one function per
 * action so each can carry its own precise request/response types.
 */
import { apiGet, apiPost } from '@/lib/api'
import type {
  ActiveCssResponse,
  AuthCheckResponse,
  BoardResponse,
  CardListEntry,
  CardResponse,
  DrawResult,
  FrequentWinnersResponse,
  GameStateResponse,
  GenerateCardsResponse,
  OkResponse,
  PatternsResponse,
  RaffleDetailResponse,
  RaffleEnterResponse,
  RaffleEntryResponse,
  RaffleUploadResponse,
  RaffleWinnerResponse,
  RafflesResponse,
  ReadingListsResponse,
  ReadingListDetailResponse,
  ReadingListItemResponse,
  ReadingListItem,
  BookclubUploadResponse,
  BookclubLookupResponse,
  PublishResponse,
  BookClubEventsResponse,
  BookClubEventResponse,
  BookClubEventForm,
  EventImagesResponse,
  EventImageUploadResponse,
  SettingsResponse,
  StyleCreateResponse,
  StyleGetResponse,
  StylesResponse,
  WinnersLogResponse,
  FontsResponse,
  FontUploadResponse,
} from '@/types/api'
import type { AppSettings } from '@/types/api'

const enc = encodeURIComponent

export const endpoints = {
  // ── Auth ───────────────────────────────────────────────────────────────────
  auth: {
    /** GET /api/auth — current admin auth status (always 200). */
    check: () => apiGet<AuthCheckResponse>('auth'),
    /** POST /api/auth {login} — a bad password 401s without a global redirect. */
    login: (password: string) =>
      apiPost<OkResponse>('auth', { action: 'login', password }, { skipAuthRedirect: true }),
    /** POST /api/auth {logout}. */
    logout: () =>
      apiPost<OkResponse>('auth', { action: 'logout' }, { skipAuthRedirect: true }),
  },

  // ── Board (player + admin card fetch) ────────────────────────────────────────
  board: {
    /**
     * GET /api/board?id=… — a card plus (for players) the active game + details.
     * `preview` requests the admin preview shape (card board data only).
     */
    get: (id: string, opts: { preview?: boolean } = {}) =>
      apiGet<BoardResponse>(`board?id=${enc(id)}${opts.preview ? '&preview=1' : ''}`),
  },

  // ── Settings ─────────────────────────────────────────────────────────────────
  settings: {
    get: () => apiGet<SettingsResponse>('settings'),
    save: (settings: AppSettings) => apiPost<OkResponse>('settings', { settings }),
  },

  // ── Game lifecycle ───────────────────────────────────────────────────────────
  game: {
    getState: () => apiGet<GameStateResponse>('game'),
    start: (patternIds: number[]) =>
      apiPost<GameStateResponse>('game', { action: 'start', pattern_ids: patternIds }),
    draw: (delay: number) => apiPost<DrawResult>('game', { action: 'draw', delay }),
    end: (validWinnerIds: string[]) =>
      apiPost<OkResponse>('game', { action: 'end', valid_winner_ids: validWinnerIds }),
    updateDetails: (details: string) =>
      apiPost<OkResponse>('game', { action: 'update_details', details }),
    triggerHalftime: () => apiPost<OkResponse>('game', { action: 'trigger_halftime' }),
  },

  // ── Winners log ──────────────────────────────────────────────────────────────
  winnersLog: {
    list: (params: { page: number; perPage: number; sort: string; dir: 'asc' | 'desc' }) =>
      apiGet<WinnersLogResponse>(
        `winners-log?page=${params.page}&per_page=${params.perPage}&sort=${params.sort}&dir=${params.dir}`,
      ),
    frequent: () => apiGet<FrequentWinnersResponse>('winners-log/frequent'),
  },

  // ── Cards ────────────────────────────────────────────────────────────────────
  cards: {
    list: () => apiGet<{ cards: CardListEntry[] }>('cards'),
    generate: (count: number) =>
      apiPost<GenerateCardsResponse>('cards', { action: 'generate', count }),
    delete: (id: string) => apiPost<OkResponse>('cards', { action: 'delete', id }),
    deleteAll: () => apiPost<OkResponse>('cards', { action: 'delete_all' }),
    updatePlayer: (id: string, playerName: string, details: string) =>
      apiPost<OkResponse>('cards', {
        action: 'update_player',
        id,
        player_name: playerName,
        details,
      }),
  },

  // ── Patterns ─────────────────────────────────────────────────────────────────
  patterns: {
    list: () => apiGet<PatternsResponse>('patterns'),
    create: (name: string, patternData: boolean[][], categoryId: number) =>
      apiPost<OkResponse>('patterns', {
        action: 'create',
        name,
        pattern_data: patternData,
        category_id: categoryId,
      }),
    delete: (id: number) => apiPost<OkResponse>('patterns', { action: 'delete', id }),
    rename: (id: number, name: string) =>
      apiPost<OkResponse>('patterns', { action: 'rename', id, name }),
    reorder: (categoryId: number, orderedIds: number[]) =>
      apiPost<OkResponse>('patterns', {
        action: 'bulk_reorder',
        category_id: categoryId,
        ordered_ids: orderedIds,
      }),
  },

  // ── Pattern categories ───────────────────────────────────────────────────────
  patternCategories: {
    create: (name: string) =>
      apiPost<OkResponse>('pattern-categories', { action: 'create', name }),
    rename: (id: number, name: string) =>
      apiPost<OkResponse>('pattern-categories', { action: 'rename', id, name }),
    delete: (id: number) =>
      apiPost<OkResponse>('pattern-categories', { action: 'delete', id }),
    reorder: (orderedIds: number[]) =>
      apiPost<OkResponse>('pattern-categories', {
        action: 'bulk_reorder',
        ordered_ids: orderedIds,
      }),
  },

  // ── Styles / themes ──────────────────────────────────────────────────────────
  styles: {
    list: () => apiGet<StylesResponse>('styles'),
    activeCss: () => apiGet<ActiveCssResponse>('styles/active'),
    get: (id: number) => apiPost<StyleGetResponse>('styles', { action: 'get', id }),
    create: (name: string, cssContent: string) =>
      apiPost<StyleCreateResponse>('styles', { action: 'create', name, css_content: cssContent }),
    update: (id: number, name: string, cssContent: string) =>
      apiPost<OkResponse>('styles', { action: 'update', id, name, css_content: cssContent }),
    delete: (id: number) => apiPost<OkResponse>('styles', { action: 'delete', id }),
    setActive: (id: number) => apiPost<OkResponse>('styles', { action: 'set_active', id }),
  },

  // ── Raffles ──────────────────────────────────────────────────────────────────
  raffles: {
    list: () => apiGet<RafflesResponse>('raffles'),
    detail: (id: number) => apiGet<RaffleDetailResponse>(`raffles/${id}`),
    create: (raffle: Record<string, unknown>) =>
      apiPost<OkResponse>('raffles', { action: 'create', ...raffle }),
    update: (raffle: Record<string, unknown>) =>
      apiPost<OkResponse>('raffles', { action: 'update', ...raffle }),
    delete: (id: number) => apiPost<OkResponse>('raffles', { action: 'delete', id }),
    uploadImage: (form: FormData) =>
      apiPost<RaffleUploadResponse>('raffles/upload', form),
    enter: (id: number, body: { character_name: string; world: string; num_entries: number }) =>
      apiPost<RaffleEnterResponse>(`raffles/${id}/enter`, body),
    addEntry: (
      raffleId: number,
      body: { character_name: string; world: string; num_entries: number; paid: boolean },
    ) =>
      apiPost<RaffleEntryResponse>(`raffles/${raffleId}/entries`, {
        action: 'add_entry',
        ...body,
      }),
    markEntryPaid: (raffleId: number, entryId: number, paid: boolean) =>
      apiPost<OkResponse>(`raffles/${raffleId}/entries`, {
        action: 'mark_paid',
        entry_id: entryId,
        paid,
      }),
    deleteEntry: (raffleId: number, entryId: number) =>
      apiPost<OkResponse>(`raffles/${raffleId}/entries`, {
        action: 'delete_entry',
        entry_id: entryId,
      }),
    pickWinner: (raffleId: number) =>
      apiPost<RaffleWinnerResponse>(`raffles/${raffleId}/entries`, { action: 'pick_winner' }),
    verifyWinner: (raffleId: number) =>
      apiPost<OkResponse>(`raffles/${raffleId}/entries`, { action: 'verify_winner' }),
    pickAnotherWinner: (raffleId: number) =>
      apiPost<RaffleWinnerResponse>(`raffles/${raffleId}/entries`, { action: 'pick_another' }),
  },

  // ── Book clubs / reading lists ───────────────────────────────────────────────
  bookclub: {
    /** GET /api/reading-lists?club=… — reading lists for a book club (no items). */
    lists: (club = 'yaoi') => apiGet<ReadingListsResponse>(`reading-lists?club=${enc(club)}`),
    /** GET /api/reading-lists/{id} — a reading list with its items. */
    listDetail: (id: number) => apiGet<ReadingListDetailResponse>(`reading-lists/${id}`),
    createList: (title: string, club = 'yaoi') =>
      apiPost<ReadingListDetailResponse>('reading-lists', { action: 'create', title, club_slug: club }),
    renameList: (id: number, title: string) =>
      apiPost<OkResponse>('reading-lists', { action: 'update', id, title }),
    deleteList: (id: number) => apiPost<OkResponse>('reading-lists', { action: 'delete', id }),
    /** Create or update an item (update when item.id is set). */
    saveItem: (listId: number, item: Partial<ReadingListItem> & { id?: number }) =>
      apiPost<ReadingListItemResponse>(`reading-lists/${listId}/items`, {
        action: item.id ? 'update' : 'create',
        item_id: item.id || 0,
        item,
      }),
    deleteItem: (listId: number, itemId: number) =>
      apiPost<OkResponse>(`reading-lists/${listId}/items`, { action: 'delete', item_id: itemId }),
    publish: (listId: number) =>
      apiPost<PublishResponse>(`reading-lists/${listId}/publish`, {}),
    uploadImage: (form: FormData) => apiPost<BookclubUploadResponse>('bookclub/upload', form),
    /** GET /api/bookclub/lookup?q=… — AniList suggestions shaped like items. */
    lookup: (query: string) =>
      apiGet<BookclubLookupResponse>(`bookclub/lookup?q=${enc(query)}`),
    /** GET /api/bookclub/lookup?id=… — a single AniList title by numeric id. */
    lookupById: (id: number) =>
      apiGet<BookclubLookupResponse>(`bookclub/lookup?id=${id}`),
  },

  // ── Book club event posts (scheduled Discord embeds) ─────────────────────────
  bookclubEvents: {
    /** GET /api/bookclub/events?club=… — scheduled events for a club. */
    list: (club = 'yaoi') =>
      apiGet<BookClubEventsResponse>(`bookclub/events?club=${enc(club)}`),
    /** Create or update an event (update when form.id is set). */
    save: (club: string, form: BookClubEventForm) =>
      apiPost<BookClubEventResponse>('bookclub/events', {
        action: form.id ? 'update' : 'create',
        id: form.id || 0,
        club_slug: club,
        event: {
          title: form.title,
          start_local: form.start_local,
          timezone: form.timezone,
          length_hours: form.length_hours,
          location: form.location,
          image: form.image,
          post_at_local: form.post_at_local,
        },
      }),
    delete: (id: number) =>
      apiPost<OkResponse>('bookclub/events', { action: 'delete', id }),
    /** Post an event's embed to Discord immediately (and mark it posted). */
    postNow: (id: number) =>
      apiPost<OkResponse>('bookclub/events', { action: 'post_now', id }),
    /** GET /api/bookclub/events/images — existing event images (pick to reuse). */
    images: () => apiGet<EventImagesResponse>('bookclub/events/images'),
    uploadImage: (form: FormData) =>
      apiPost<EventImageUploadResponse>('bookclub/events/upload', form),
  },

  // ── Fonts (System → Font Upload) ─────────────────────────────────────────────
  fonts: {
    /** GET /api/fonts — list font files in <webRoot>/fonts. */
    list: () => apiGet<FontsResponse>('fonts'),
    /** POST /api/fonts/upload — multipart upload of one or more "files" fields. */
    upload: (form: FormData) => apiPost<FontUploadResponse>('fonts/upload', form),
    /** POST /api/fonts {delete} — remove a font file by name. */
    delete: (name: string) => apiPost<OkResponse>('fonts', { action: 'delete', name }),
    /** POST /api/fonts {rename} — rename a font file (fails if the target exists). */
    rename: (name: string, newName: string) =>
      apiPost<OkResponse>('fonts', { action: 'rename', name, new_name: newName }),
  },
}

// `CardResponse` is the documented shape for board fetches whose callers only
// read `.card` (winner verify, card preview); re-exported for those sites.
export type { CardResponse }
