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
import { apiGet, apiPost, apiPut, apiPatch, apiDelete } from '@/lib/api'
import type {
  ActiveCSSResponse,
  AuthCheckResponse,
  LoginResponse,
  RegisterResponse,
  UsersResponse,
  AccountTokenInfoResponse,
  AccountTokenGenerateResponse,
  TokenRevokeResponse,
  BoardResponse,
  CardsListResponse,
  CardResponse,
  DeletedCountResponse,
  DrawResult,
  EndGameResponse,
  FrequentWinnersResponse,
  GameStateResponse,
  GenerateCardsResponse,
  GenerateSingleCardResponse,
  OKResponse,
  NamedOKResponse,
  PatternsResponse,
  PatternCreateResponse,
  CategoriesResponse,
  CategoryCreateResponse,
  PresetsResponse,
  PresetCreateResponse,
  RaffleDetailResponse,
  RaffleEnterResponse,
  RaffleEntryResponse,
  RaffleWinnerResponse,
  RaffleResponse,
  RafflesResponse,
  StatusResponse,
  AffiliatesResponse,
  AffiliateResponse,
  TeaRoomsResponse,
  TeaRoomResponse,
  TeaRoomWebhookResponse,
  StampRalliesResponse,
  StampRallyResponse,
  StampRallyDetailResponse,
  StampRallyCardResponse,
  StampRallyLogsResponse,
  PausedResponse,
  PublicStampCard,
  StampSubmitResponse,
  GaraponsResponse,
  GaraponResponse,
  GaraponDetailResponse,
  GaraponPlayerResponse,
  GaraponPublicResponse,
  GaraponDrawResponse,
  ReadingListsResponse,
  ReadingListDetailResponse,
  ReadingListItemResponse,
  ReadingListItem,
  BookclubUploadResponse,
  BookclubLookupResponse,
  PublishResponse,
  Announcement,
  AnnouncementTypesResponse,
  AnnouncementTypeResponse,
  AnnouncementTypeForm,
  AnnouncementRolesResponse,
  AnnouncementRoleResponse,
  AnnouncementRoleForm,
  AnnouncementsResponse,
  AnnouncementResponse,
  ImageCategoriesResponse,
  ImageCategoryActionResponse,
  ImagesResponse,
  ImagesUploadResponse,
  SettingsResponse,
  StyleCreateResponse,
  StyleGetResponse,
  StylesResponse,
  WinnersLogResponse,
  FontsResponse,
  FontUploadResponse,
  CarrdProjectsResponse,
  CarrdProjectCreateResponse,
  CarrdImagesResponse,
  CarrdUploadResponse,
  LogsResponse,
  LogLevelResponse,
} from '@/types/api'
import type { AppSettings } from '@/types/api'

const enc = encodeURIComponent

export const endpoints = {
  // ── Auth ───────────────────────────────────────────────────────────────────
  auth: {
    /** GET /api/auth — current auth status + the logged-in user (always 200). */
    check: () => apiGet<AuthCheckResponse>('auth'),
    /** POST /api/auth {login} — bad credentials 401 without a global redirect.
     *  turnstileToken carries the Cloudflare Turnstile result when the bot check
     *  is enabled (omitted/ignored otherwise). */
    login: (username: string, password: string, turnstileToken?: string) =>
      apiPost<LoginResponse>(
        'auth',
        { action: 'login', username, password, turnstile_token: turnstileToken },
        { skipAuthRedirect: true },
      ),
    /** POST /api/auth {logout}. */
    logout: () => apiPost<OKResponse>('auth', { action: 'logout' }, { skipAuthRedirect: true }),
    /** POST /api/register — create an account (hidden page; pending activation).
     *  turnstileToken carries the Turnstile result when the bot check is enabled. */
    register: (username: string, password: string, turnstileToken?: string) =>
      apiPost<RegisterResponse>(
        'register',
        { username, password, turnstile_token: turnstileToken },
        { skipAuthRedirect: true },
      ),
  },

  // ── Users (admin, hybrid REST) + self-service account ────────────────────────
  users: {
    /** GET /api/users — all accounts (admin only). */
    list: () => apiGet<UsersResponse>('users'),
    /** PATCH /api/users/{id} {active} — activate/deactivate the account. */
    setActive: (id: number, active: boolean) => apiPatch<OKResponse>(`users/${id}`, { active }),
    /** PATCH /api/users/{id} {admin} — grant/revoke admin. */
    setAdmin: (id: number, admin: boolean) => apiPatch<OKResponse>(`users/${id}`, { admin }),
    /** PATCH /api/users/{id} {permissions} — set the account's page-permission set. */
    setPermissions: (id: number, permissions: string[]) =>
      apiPatch<OKResponse>(`users/${id}`, { permissions }),
    /** PATCH /api/users/{id} {password} — reset the account's password. */
    setPassword: (id: number, password: string) =>
      apiPatch<OKResponse>(`users/${id}`, { password }),
    /** DELETE /api/users/{id} — delete the account (204). */
    delete: (id: number) => apiDelete(`users/${id}`),
  },
  account: {
    /** POST /api/account/change-password — change the logged-in user's own password. */
    changePassword: (currentPassword: string, newPassword: string) =>
      apiPost<OKResponse>('account/change-password', {
        current_password: currentPassword,
        new_password: newPassword,
      }),
    /** GET /api/account/token — the account's personal-access-token metadata
     *  (never the secret itself; that is only returned once at generation). */
    tokenInfo: () => apiGet<AccountTokenInfoResponse>('account/token'),
    /** POST /api/account/token — mint (replacing any existing) a token.
     *  The returned `token` plaintext is shown to the user exactly once. */
    generateToken: () => apiPost<AccountTokenGenerateResponse>('account/token', {}),
    /** DELETE /api/account/token — delete the account's token (200 w/ deleted flag). */
    revokeToken: () => apiDelete<TokenRevokeResponse>('account/token'),
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
    save: (settings: AppSettings) => apiPost<OKResponse>('settings', { settings }),
  },

  // ── System ───────────────────────────────────────────────────────────────────
  system: {
    /** GET /api/version — the backend's semantic version (public). */
    version: () => apiGet<{ backend: string }>('version'),
    /** GET /api/config — client bootstrap config (Turnstile site key; "" = off). */
    config: () => apiGet<{ turnstile_site_key: string }>('config'),
  },

  // ── Game lifecycle (hybrid REST: verb sub-paths + PATCH for controls) ────────
  game: {
    getState: () => apiGet<GameStateResponse>('game'),
    start: (patternIds: number[]) =>
      apiPost<GameStateResponse>('game/start', { pattern_ids: patternIds }),
    draw: (delay: number) => apiPost<DrawResult>('game/draw', { delay }),
    /** PATCH /api/game {delay} — persist + broadcast the shared draw delay. */
    setDelay: (delay: number) => apiPatch<OKResponse>('game', { delay }),
    end: (validWinnerIds: string[]) =>
      apiPost<EndGameResponse>('game/end', { valid_winner_ids: validWinnerIds }),
    /** PATCH /api/game {details} — set + broadcast the markdown game details. */
    updateDetails: (details: string) => apiPatch<OKResponse>('game', { details }),
    triggerHalftime: () => apiPost<OKResponse>('game/halftime', undefined),
  },

  // ── Winners log ──────────────────────────────────────────────────────────────
  winnersLog: {
    list: (params: { page: number; perPage: number; sort: string; dir: 'asc' | 'desc' }) =>
      apiGet<WinnersLogResponse>(
        `winners-log?page=${params.page}&per_page=${params.perPage}&sort=${params.sort}&dir=${params.dir}`,
      ),
    frequent: () => apiGet<FrequentWinnersResponse>('winners-log/frequent'),
    /** DELETE /api/winners-log/{id} — remove a single entry (204). */
    delete: (id: number) => apiDelete(`winners-log/${id}`),
    /** DELETE /api/winners-log/all — clear the log, returning the deleted count. */
    deleteAll: () => apiDelete<DeletedCountResponse>('winners-log/all'),
  },

  // ── Server logs (admin-only) ─────────────────────────────────────────────────
  logs: {
    /** GET /api/logs — tail the server JSON log, newest-first, filtered. */
    list: (params: { level?: string; q?: string; limit?: number } = {}) => {
      const qs = new URLSearchParams()
      if (params.level) qs.set('level', params.level)
      if (params.q) qs.set('q', params.q)
      if (params.limit) qs.set('limit', String(params.limit))
      const s = qs.toString()
      return apiGet<LogsResponse>(`logs${s ? `?${s}` : ''}`)
    },
    /** POST /api/logs/level — set the runtime minimum log level (live DEBUG toggle). */
    setLevel: (level: string) => apiPost<LogLevelResponse>('logs/level', { level }),
  },

  // ── Cards (hybrid REST) ──────────────────────────────────────────────────────
  cards: {
    list: () => apiGet<CardsListResponse>('cards'),
    /** POST /api/cards — create one card, optionally assigned to a player name. */
    create: (playerName: string) =>
      apiPost<GenerateSingleCardResponse>('cards', { player_name: playerName }),
    /** POST /api/cards/generate — bulk-generate `count` random cards. */
    generate: (count: number) => apiPost<GenerateCardsResponse>('cards/generate', { count }),
    /** DELETE /api/cards/{id} — delete a single card (204). */
    delete: (id: string) => apiDelete(`cards/${enc(id)}`),
    /** DELETE /api/cards/all — delete every card, returning the deleted count. */
    deleteAll: () => apiDelete<DeletedCountResponse>('cards/all'),
    /** PATCH /api/cards/{id} — update the card's assigned player name + details. */
    updatePlayer: (id: string, playerName: string, details: string) =>
      apiPatch<OKResponse>(`cards/${enc(id)}`, { player_name: playerName, details }),
  },

  // ── Patterns (hybrid REST) ───────────────────────────────────────────────────
  patterns: {
    list: () => apiGet<PatternsResponse>('patterns'),
    /** POST /api/patterns — create a win pattern (201). */
    create: (name: string, patternData: boolean[][], categoryId: number) =>
      apiPost<PatternCreateResponse>('patterns', {
        name,
        pattern_data: patternData,
        category_id: categoryId,
      }),
    /** DELETE /api/patterns/{id} — delete a single pattern (204). */
    delete: (id: number) => apiDelete(`patterns/${id}`),
    /** PATCH /api/patterns/{id} {name} — rename (merged PATCH). */
    rename: (id: number, name: string) => apiPatch<OKResponse>(`patterns/${id}`, { name }),
    /** PATCH /api/patterns/{id} {category_id} — move to a category (merged PATCH). */
    setCategory: (id: number, categoryId: number) =>
      apiPatch<OKResponse>(`patterns/${id}`, { category_id: categoryId }),
    /** PATCH /api/patterns/{id} {direction} — reorder within its category; returns the fresh list. */
    reorder: (id: number, direction: 'up' | 'down') =>
      apiPatch<PatternsResponse>(`patterns/${id}`, { direction }),
    /** POST /api/patterns/reorder — persist a category's new drag order; returns the fresh list. */
    bulkReorder: (categoryId: number, orderedIds: number[]) =>
      apiPost<PatternsResponse>('patterns/reorder', {
        category_id: categoryId,
        ordered_ids: orderedIds,
      }),
  },

  // ── Pattern categories (hybrid REST) ─────────────────────────────────────────
  patternCategories: {
    /** POST /api/pattern-categories — create a category (201). */
    create: (name: string) => apiPost<CategoryCreateResponse>('pattern-categories', { name }),
    /** PATCH /api/pattern-categories/{id} {name} — rename. */
    rename: (id: number, name: string) =>
      apiPatch<OKResponse>(`pattern-categories/${id}`, { name }),
    /** DELETE /api/pattern-categories/{id} — delete (204; 409 on the last category). */
    delete: (id: number) => apiDelete(`pattern-categories/${id}`),
    /** PATCH /api/pattern-categories/{id} {direction} — reorder; returns the fresh list. */
    reorder: (id: number, direction: 'up' | 'down') =>
      apiPatch<CategoriesResponse>(`pattern-categories/${id}`, { direction }),
    /** POST /api/pattern-categories/reorder — persist a new order; returns the fresh list. */
    bulkReorder: (orderedIds: number[]) =>
      apiPost<CategoriesResponse>('pattern-categories/reorder', { ordered_ids: orderedIds }),
  },

  // ── Game presets ─────────────────────────────────────────────────────────────
  presets: {
    /** GET /api/presets — all saved game presets. */
    list: () => apiGet<PresetsResponse>('presets'),
    create: (name: string, patternIds: number[], gameDetails: string) =>
      apiPost<PresetCreateResponse>('presets', {
        name,
        pattern_ids: patternIds,
        game_details: gameDetails,
      }),
    update: (id: number, name: string, patternIds: number[], gameDetails: string) =>
      apiPut<OKResponse>(`presets/${id}`, {
        name,
        pattern_ids: patternIds,
        game_details: gameDetails,
      }),
    delete: (id: number) => apiDelete(`presets/${id}`),
  },

  // ── Styles / themes (hybrid REST) ────────────────────────────────────────────
  styles: {
    list: () => apiGet<StylesResponse>('styles'),
    activeCss: () => apiGet<ActiveCSSResponse>('styles/active'),
    /** GET /api/styles/{id} — one theme (tokens + generated CSS). */
    get: (id: number) => apiGet<StyleGetResponse>(`styles/${id}`),
    /** POST /api/styles — create a theme (201). */
    create: (
      name: string,
      tokens: Record<string, string>,
      boardFlourish = '',
      numberFlourish = '',
    ) =>
      apiPost<StyleCreateResponse>('styles', {
        name,
        tokens,
        board_flourish: boardFlourish,
        number_flourish: numberFlourish,
      }),
    /** PUT /api/styles/{id} — full replace of a theme. */
    update: (
      id: number,
      name: string,
      tokens: Record<string, string>,
      boardFlourish = '',
      numberFlourish = '',
    ) =>
      apiPut<OKResponse>(`styles/${id}`, {
        name,
        tokens,
        board_flourish: boardFlourish,
        number_flourish: numberFlourish,
      }),
    /** DELETE /api/styles/{id} — delete a theme (204). */
    delete: (id: number) => apiDelete(`styles/${id}`),
    /** Activate a theme (id>0) or clear the active theme (id≤0, the "None" button). */
    setActive: (id: number) =>
      id > 0
        ? apiPost<OKResponse>(`styles/${id}/activate`, undefined)
        : apiPost<OKResponse>('styles/deactivate', undefined),
  },

  // ── Raffles ──────────────────────────────────────────────────────────────────
  raffles: {
    list: () => apiGet<RafflesResponse>('raffles'),
    detail: (id: number) => apiGet<RaffleDetailResponse>(`raffles/${id}`),
    create: (raffle: Record<string, unknown>) => apiPost<RaffleResponse>('raffles', raffle),
    update: (id: number, raffle: Record<string, unknown>) =>
      apiPut<RaffleResponse>(`raffles/${id}`, raffle),
    delete: (id: number) => apiDelete(`raffles/${id}`),
    enter: (
      id: number,
      body: {
        character_name: string
        world: string
        num_entries: number
        turnstile_token?: string
      },
    ) => apiPost<RaffleEnterResponse>(`raffles/${id}/enter`, body),
    addEntry: (
      raffleId: number,
      body: { character_name: string; world: string; num_entries: number; paid: boolean },
    ) => apiPost<RaffleEntryResponse>(`raffles/${raffleId}/entries`, body),
    markEntryPaid: (raffleId: number, entryId: number, paid: boolean) =>
      apiPatch<RaffleEntryResponse>(`raffles/${raffleId}/entries/${entryId}`, { paid }),
    deleteEntry: (raffleId: number, entryId: number) =>
      apiDelete(`raffles/${raffleId}/entries/${entryId}`),
    pickWinner: (raffleId: number) =>
      apiPost<RaffleWinnerResponse>(`raffles/${raffleId}/pick-winner`, undefined),
    pickAnotherWinner: (raffleId: number) =>
      apiPost<RaffleWinnerResponse>(`raffles/${raffleId}/pick-another`, undefined),
    verifyWinner: (raffleId: number) =>
      apiPost<StatusResponse>(`raffles/${raffleId}/verify-winner`, undefined),
  },

  // ── Garapon (admin, hybrid REST) ─────────────────────────────────────────────
  garapons: {
    /** GET /api/garapons — all garapons (admin). */
    list: () => apiGet<GaraponsResponse>('garapons'),
    /** GET /api/garapons/{id} — a garapon with prizes, drawing links, draw log. */
    detail: (id: number) => apiGet<GaraponDetailResponse>(`garapons/${id}`),
    /** POST /api/garapons — create a garapon (201). The form omits an id. */
    create: (garapon: Record<string, unknown>) => apiPost<GaraponResponse>('garapons', garapon),
    /** PUT /api/garapons/{id} — full replace of the editable fields. */
    update: (garapon: { id: number } & Record<string, unknown>) =>
      apiPut<OKResponse>(`garapons/${garapon.id}`, garapon),
    /** DELETE /api/garapons/{id} — delete a garapon (204). */
    delete: (id: number) => apiDelete(`garapons/${id}`),
    /** Close (POST /{id}/close) or reopen (POST /{id}/reopen) a garapon. */
    setStatus: (id: number, status: 'open' | 'closed') =>
      status === 'closed'
        ? apiPost<StatusResponse>(`garapons/${id}/close`, undefined)
        : apiPost<StatusResponse>(`garapons/${id}/reopen`, undefined),
    /** POST /api/garapons/{id}/players — create a per-player drawing link (returns its token). */
    createPlayer: (garaponId: number, body: { player_name: string; max_draws: number }) =>
      apiPost<GaraponPlayerResponse>(`garapons/${garaponId}/players`, body),
    /** DELETE /api/garapons/{id}/players/{playerId} — delete a drawing link (204). */
    deletePlayer: (garaponId: number, playerId: number) =>
      apiDelete(`garapons/${garaponId}/players/${playerId}`),
  },

  // ── Garapon (public player view, via per-player token) ───────────────────────
  garapon: {
    /** GET /api/garapon/{token} — the player view (garapon + prizes + their record). */
    get: (token: string) => apiGet<GaraponPublicResponse>(`garapon/${enc(token)}`),
    /** POST /api/garapon/{token}/draw — perform one authoritative draw. */
    draw: (token: string) => apiPost<GaraponDrawResponse>(`garapon/${enc(token)}/draw`, {}),
  },

  // ── Affiliates (admin) ───────────────────────────────────────────────────────
  affiliates: {
    /** GET /api/affiliates — all affiliates (admin). */
    list: () => apiGet<AffiliatesResponse>('affiliates'),
    create: (affiliate: Record<string, unknown>) =>
      apiPost<AffiliateResponse>('affiliates', affiliate),
    update: (id: number, affiliate: Record<string, unknown>) =>
      apiPut<OKResponse>(`affiliates/${id}`, affiliate),
    delete: (id: number) => apiDelete(`affiliates/${id}`),
  },

  // ── Tea Rooms (admin) ────────────────────────────────────────────────────────
  teaRooms: {
    /** GET /api/tea-rooms — all rooms + the shared Discord webhook (admin/perm). */
    list: () => apiGet<TeaRoomsResponse>('tea-rooms'),
    /** POST /api/tea-rooms — create a room (201). */
    create: (room: Record<string, unknown>) =>
      apiPost<TeaRoomResponse>('tea-rooms', { tea_room: room }),
    /** PUT /api/tea-rooms/{id} — full replace of the editable fields. */
    update: (id: number, room: Record<string, unknown>) =>
      apiPut<TeaRoomResponse>(`tea-rooms/${id}`, { tea_room: room }),
    /** PATCH /api/tea-rooms/{id} — toggle the open and/or discounted flag. */
    patch: (id: number, fields: { open?: boolean; discounted?: boolean }) =>
      apiPatch<TeaRoomResponse>(`tea-rooms/${id}`, fields),
    /** DELETE /api/tea-rooms/{id} — delete a room (204). */
    delete: (id: number) => apiDelete(`tea-rooms/${id}`),
    /** POST /api/tea-rooms/reorder — persist a new drag order (top-first ids). */
    reorder: (orderedIds: number[]) =>
      apiPost<OKResponse>('tea-rooms/reorder', { ordered_ids: orderedIds }),
    /** POST /api/tea-rooms/{id}/post — post a room's embed to the shared webhook now. */
    post: (id: number) => apiPost<TeaRoomResponse>(`tea-rooms/${id}/post`, undefined),
    /** PUT /api/tea-rooms/webhook — set the single shared Discord webhook ('' clears). */
    setWebhook: (webhookUrl: string) =>
      apiPut<TeaRoomWebhookResponse>('tea-rooms/webhook', { webhook_url: webhookUrl }),
  },

  // ── Stamp Rally (admin, hybrid REST) ─────────────────────────────────────────
  stampRallies: {
    /** GET /api/stamp-rallies — all rallies (admin). */
    list: () => apiGet<StampRalliesResponse>('stamp-rallies'),
    /** GET /api/stamp-rallies/{id} — a rally with stamps, prizes, and issued cards. */
    detail: (id: number) => apiGet<StampRallyDetailResponse>(`stamp-rallies/${id}`),
    /** GET /api/stamp-rallies/{id}/logs — the event-wide stamp collection log. */
    logs: (id: number) => apiGet<StampRallyLogsResponse>(`stamp-rallies/${id}/logs`),
    /** POST /api/stamp-rallies — create a rally (201). The form omits an id. */
    create: (rally: Record<string, unknown>) => apiPost<StampRallyResponse>('stamp-rallies', rally),
    /** PUT /api/stamp-rallies/{id} — full replace of the editable fields. */
    update: (rally: { id: number } & Record<string, unknown>) =>
      apiPut<OKResponse>(`stamp-rallies/${rally.id}`, rally),
    /** DELETE /api/stamp-rallies/{id} — delete a rally (204). */
    delete: (id: number) => apiDelete(`stamp-rallies/${id}`),
    /** Close (POST /{id}/close) or reopen (POST /{id}/reopen) a rally. */
    setStatus: (id: number, status: 'open' | 'closed') =>
      status === 'closed'
        ? apiPost<StatusResponse>(`stamp-rallies/${id}/close`, undefined)
        : apiPost<StatusResponse>(`stamp-rallies/${id}/reopen`, undefined),
    /** PATCH /api/stamp-rallies/{id}/stamps/{stampId} — pause/resume a single stamp. */
    setStampPaused: (rallyId: number, stampId: number, paused: boolean) =>
      apiPatch<PausedResponse>(`stamp-rallies/${rallyId}/stamps/${stampId}`, { paused }),
    /** POST /api/stamp-rallies/{id}/cards — issue a tokenized participant card (returns its token). */
    createCard: (rallyId: number, participantName: string) =>
      apiPost<StampRallyCardResponse>(`stamp-rallies/${rallyId}/cards`, {
        participant_name: participantName,
      }),
    /** DELETE /api/stamp-rallies/{id}/cards/{cardId} — delete a participant card (204). */
    deleteCard: (rallyId: number, cardId: number) =>
      apiDelete(`stamp-rallies/${rallyId}/cards/${cardId}`),
  },

  // ── Stamp Rally (public, via per-participant card token) ─────────────────────
  stampCard: {
    /** GET /api/stamp-card/{token} — the participant card view (no passwords). */
    get: (token: string) => apiGet<PublicStampCard>(`stamp-card/${enc(token)}`),
    /** POST /api/stamp-card/{token}/stamp — collect a stamp by password. */
    stamp: (token: string, password: string) =>
      apiPost<StampSubmitResponse>(`stamp-card/${enc(token)}/stamp`, { password }),
  },

  // ── Book clubs / reading lists ───────────────────────────────────────────────
  //
  // Reading lists are nested under their owning club: every function takes the
  // club slug as its first argument and targets /api/book-clubs/{club}/reading-
  // lists…. The two club-agnostic utilities (uploadImage, lookup*) keep the flat
  // /api/bookclub/* paths.
  bookclub: {
    /** GET /api/book-clubs/{club}/reading-lists — reading lists for a club (no items). */
    lists: (club: string) => apiGet<ReadingListsResponse>(`book-clubs/${enc(club)}/reading-lists`),
    /** GET /api/book-clubs/{club}/reading-lists/{id} — a reading list with its items. */
    listDetail: (club: string, id: number) =>
      apiGet<ReadingListDetailResponse>(`book-clubs/${enc(club)}/reading-lists/${id}`),
    /** POST /api/book-clubs/{club}/reading-lists — create a list (201; returns it with its items). */
    createList: (club: string, title: string) =>
      apiPost<ReadingListDetailResponse>(`book-clubs/${enc(club)}/reading-lists`, { title }),
    /** PUT /api/book-clubs/{club}/reading-lists/{id} — rename a list. */
    renameList: (club: string, id: number, title: string) =>
      apiPut<OKResponse>(`book-clubs/${enc(club)}/reading-lists/${id}`, { title }),
    /** DELETE /api/book-clubs/{club}/reading-lists/{id} — delete a list + its items (204). */
    deleteList: (club: string, id: number) =>
      apiDelete(`book-clubs/${enc(club)}/reading-lists/${id}`),
    /**
     * Create (POST) or update (PUT /{itemId}) an item, chosen by whether the item
     * already has an id. Both wrap the item under `{item}`.
     */
    saveItem: (club: string, listId: number, item: Partial<ReadingListItem> & { id?: number }) =>
      item.id
        ? apiPut<ReadingListItemResponse>(
            `book-clubs/${enc(club)}/reading-lists/${listId}/items/${item.id}`,
            { item },
          )
        : apiPost<ReadingListItemResponse>(
            `book-clubs/${enc(club)}/reading-lists/${listId}/items`,
            { item },
          ),
    /** DELETE /api/book-clubs/{club}/reading-lists/{id}/items/{itemId} — delete an item (204). */
    deleteItem: (club: string, listId: number, itemId: number) =>
      apiDelete(`book-clubs/${enc(club)}/reading-lists/${listId}/items/${itemId}`),
    publish: (club: string, listId: number) =>
      apiPost<PublishResponse>(`book-clubs/${enc(club)}/reading-lists/${listId}/publish`, {}),
    uploadImage: (form: FormData) => apiPost<BookclubUploadResponse>('bookclub/upload', form),
    /** GET /api/bookclub/lookup?q=… — AniList suggestions shaped like items. */
    lookup: (query: string) => apiGet<BookclubLookupResponse>(`bookclub/lookup?q=${enc(query)}`),
    /** GET /api/bookclub/lookup?id=… — a single AniList title by numeric id. */
    lookupById: (id: number) => apiGet<BookclubLookupResponse>(`bookclub/lookup?id=${id}`),
  },

  // ── Announcement management ──────────────────────────────────────────────────
  announcements: {
    /** GET /api/announcement-types — Discord destinations. */
    types: () => apiGet<AnnouncementTypesResponse>('announcement-types'),
    /** POST /api/announcement-types — create a type. */
    createType: (form: AnnouncementTypeForm) =>
      apiPost<AnnouncementTypeResponse>('announcement-types', {
        name: form.name,
        webhook_url: form.webhook_url,
      }),
    /** PUT /api/announcement-types/{id} — replace a type. */
    updateType: (id: number, form: AnnouncementTypeForm) =>
      apiPut<AnnouncementTypeResponse>(`announcement-types/${id}`, {
        name: form.name,
        webhook_url: form.webhook_url,
      }),
    deleteType: (id: number) => apiDelete(`announcement-types/${id}`),

    /** GET /api/announcement-roles — taggable Discord roles. */
    roles: () => apiGet<AnnouncementRolesResponse>('announcement-roles'),
    /** POST /api/announcement-roles — create a taggable role. */
    createRole: (form: AnnouncementRoleForm) =>
      apiPost<AnnouncementRoleResponse>('announcement-roles', {
        name: form.name,
        role_id: form.role_id,
      }),
    /** PUT /api/announcement-roles/{id} — replace a taggable role. */
    updateRole: (id: number, form: AnnouncementRoleForm) =>
      apiPut<AnnouncementRoleResponse>(`announcement-roles/${id}`, {
        name: form.name,
        role_id: form.role_id,
      }),
    deleteRole: (id: number) => apiDelete(`announcement-roles/${id}`),

    /** GET /api/announcements — all announcements (filtering is client-side). */
    list: () => apiGet<AnnouncementsResponse>('announcements'),
    /**
     * Create (POST) or update (PUT /{id}) an announcement, chosen by whether an id
     * is supplied. The store builds `payload` from the form, having already
     * converted local times → the stored UTC instants / UTC recurrence fields.
     * Both wrap the payload under `{announcement}`.
     */
    save: (id: number, payload: Partial<Announcement>) =>
      id
        ? apiPut<AnnouncementResponse>(`announcements/${id}`, { announcement: payload })
        : apiPost<AnnouncementResponse>('announcements', { announcement: payload }),
    /** DELETE /api/announcements/{id} — delete an announcement (204). */
    delete: (id: number) => apiDelete(`announcements/${id}`),
    /** POST /api/announcements/reorder — persist a new drag order (top-first ids). */
    reorder: (orderedIds: number[]) =>
      apiPost<OKResponse>('announcements/reorder', { ordered_ids: orderedIds }),
    /** POST /api/announcements/{id}/send — post an announcement's embed to Discord now. */
    sendNow: (id: number) => apiPost<AnnouncementResponse>(`announcements/${id}/send`, undefined),
    /** POST /api/announcements/{id}/skip — skip the next scheduled occurrence. */
    skipNext: (id: number) => apiPost<AnnouncementResponse>(`announcements/${id}/skip`, undefined),
  },

  // ── Central image hosting (System → Images) ──────────────────────────────────
  images: {
    /** GET /api/image-categories — permanent + custom categories. */
    categories: () => apiGet<ImageCategoriesResponse>('image-categories'),
    /**
     * Create or rename a category. `create` POSTs to the collection; `rename`
     * PATCHes the existing category resource (keyed by its directory).
     */
    saveCategory: (action: 'create' | 'rename', name: string, dir = '', newDir = '') =>
      action === 'rename'
        ? apiPatch<ImageCategoryActionResponse>(`image-categories/${enc(dir)}`, {
            name,
            new_dir: newDir,
          })
        : apiPost<ImageCategoryActionResponse>('image-categories', { name, dir }),
    /** DELETE /api/image-categories/{dir} — delete a custom category + its files (204). */
    deleteCategory: (dir: string) => apiDelete(`image-categories/${enc(dir)}`),
    /** GET /api/images?dir=… — images in a category (newest first). */
    list: (dir: string) => apiGet<ImagesResponse>(`images?dir=${enc(dir)}`),
    /** POST /api/images/upload — multipart "dir" + one or more "files". */
    upload: (form: FormData) => apiPost<ImagesUploadResponse>('images/upload', form),
    /** DELETE /api/images?dir=…&name=… — remove an image from a category (204). */
    deleteImage: (dir: string, name: string) =>
      apiDelete(`images?dir=${enc(dir)}&name=${enc(name)}`),
  },

  // ── Fonts (Atelier → Font Upload) ────────────────────────────────────────────
  fonts: {
    /** GET /api/fonts — fonts grouped by base name with their variants. */
    list: () => apiGet<FontsResponse>('fonts'),
    /** POST /api/fonts/upload — multipart upload of one or more "files" fields. */
    upload: (form: FormData) => apiPost<FontUploadResponse>('fonts/upload', form),
    /** DELETE /api/fonts/{name} — remove ONE variant file by name (204). */
    deleteFile: (name: string) => apiDelete(`fonts/${enc(name)}`),
    /** PATCH /api/fonts/{name} — rename one variant file (fails if the target exists). */
    renameFile: (name: string, newName: string) =>
      apiPatch<NamedOKResponse>(`fonts/${enc(name)}`, { new_name: newName }),
    /** PATCH /api/fonts/families/{base} — partial update of a font's metadata:
     *  CSS family name ("" = base default), served variant type ("" = auto),
     *  and/or its per-font allowed-site origins. */
    updateFamily: (base: string, fields: { family?: string; serve?: string; origins?: string[] }) =>
      apiPatch<OKResponse>(`fonts/families/${enc(base)}`, fields),
    /** DELETE /api/fonts/families/{base} — delete a whole font (all variants, 204). */
    deleteFont: (base: string) => apiDelete(`fonts/families/${enc(base)}`),
  },

  // ── Carrd image hosting (System → Carrd Upload) ──────────────────────────────
  carrd: {
    /** GET /api/carrd/projects — list project folders under <webRoot>/carrd. */
    projects: () => apiGet<CarrdProjectsResponse>('carrd/projects'),
    /** POST /api/carrd/projects — create a project (folder optional). */
    createProject: (title: string, folder: string) =>
      apiPost<CarrdProjectCreateResponse>('carrd/projects', { title, folder }),
    /** PATCH /api/carrd/projects/{folder} — rename a project's title and/or folder. */
    renameProject: (folder: string, title: string, newFolder: string) =>
      apiPatch<CarrdProjectCreateResponse>(`carrd/projects/${enc(folder)}`, {
        title,
        new_folder: newFolder,
      }),
    /** DELETE /api/carrd/projects/{folder} — delete a project folder + contents (204). */
    deleteProject: (folder: string) => apiDelete(`carrd/projects/${enc(folder)}`),
    /** GET /api/carrd/images?folder=…&path=… — sub-dirs + images at a path. */
    images: (folder: string, path = '') =>
      apiGet<CarrdImagesResponse>(`carrd/images?folder=${enc(folder)}&path=${enc(path)}`),
    /** POST /api/carrd/upload — multipart upload of "files" to "folder"/"path". */
    upload: (form: FormData) => apiPost<CarrdUploadResponse>('carrd/upload', form),
    /** DELETE /api/carrd/images?folder=…&path=…&name=… — remove an image at a path (204). */
    deleteImage: (folder: string, path: string, name: string) =>
      apiDelete(`carrd/images?folder=${enc(folder)}&path=${enc(path)}&name=${enc(name)}`),
    /** POST /api/carrd/images/dirs — create a sub-directory at a path. */
    createDir: (folder: string, path: string, name: string) =>
      apiPost<NamedOKResponse>('carrd/images/dirs', { folder, path, name }),
    /** DELETE /api/carrd/images/dirs?folder=…&path=… — delete a sub-directory + contents (204). */
    deleteDir: (folder: string, path: string) =>
      apiDelete(`carrd/images/dirs?folder=${enc(folder)}&path=${enc(path)}`),
  },
}

// `CardResponse` is the documented shape for board fetches whose callers only
// read `.card` (winner verify, card preview); re-exported for those sites.
export type { CardResponse }
