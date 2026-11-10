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
import { apiGet, apiPost, apiPut, apiPatch, apiDelete, apiUpload } from '@/lib/api'
import type {
  OKResponse,
  NamedOKResponse,
  RaffleDetailResponse,
  RaffleEnterResponse,
  RaffleLookupResponse,
  RaffleEntryResponse,
  RaffleWinnerResponse,
  RaffleResponse,
  RafflesResponse,
  StatusResponse,
  AffiliatesResponse,
  AffiliateResponse,
  AffiliateWebhookResponse,
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
  SignupRalliesResponse,
  StampSignupResponse,
  StampLookupResponse,
  FestivalMapsResponse,
  FestivalMapResponse,
  FestivalMapDetailResponse,
  PublicFestivalMapsResponse,
  PublicFestivalMap,
  FestivalDashboard,
  EmaBoardsResponse,
  EmaBoardResponse,
  EmaPlaquesResponse,
  EmaPlaqueResponse,
  GoogleFontsResponse,
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
  FontsResponse,
  FontUploadResponse,
  CarrdProjectsResponse,
  CarrdProjectCreateResponse,
  CarrdImagesResponse,
  CarrdUploadResponse,
} from '@/types/api'

const enc = encodeURIComponent

export const endpoints = {
  // -- Raffles ------------------------------------------------------------------
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
    /**
     * POST /api/raffles/{id}/lookup - the public "have I already entered?" search.
     * Matches a substring of the character name (at least 2 characters); the name
     * travels in the body so it stays out of proxy and access logs.
     */
    lookup: (id: number, name: string) =>
      apiPost<RaffleLookupResponse>(`raffles/${id}/lookup`, { name }),
    addEntry: (
      raffleId: number,
      body: {
        character_name: string
        world: string
        num_entries: number
        paid: boolean
        amount_waived?: number
      },
    ) => apiPost<RaffleEntryResponse>(`raffles/${raffleId}/entries`, body),
    /**
     * Record a settlement (`paid: true`) or clear one (`paid: false`, which resets
     * both counters).
     *
     * `paidEntries` is how many of the entry's tickets the payment covers - 0 means
     * all of them, a smaller number records a PART payment. The waiver is ADDITIVE
     * server-side: send only what is being forgiven now, never a running total.
     */
    markEntryPaid: (
      raffleId: number,
      entryId: number,
      paid: boolean,
      amountWaived = 0,
      paidEntries = 0,
    ) =>
      apiPatch<RaffleEntryResponse>(`raffles/${raffleId}/entries/${entryId}`, {
        paid,
        paid_entries: paidEntries,
        amount_waived: amountWaived,
      }),
    deleteEntry: (raffleId: number, entryId: number) =>
      apiDelete(`raffles/${raffleId}/entries/${entryId}`),
    pickWinner: (raffleId: number) =>
      apiPost<RaffleWinnerResponse>(`raffles/${raffleId}/pick-winner`, undefined),
    pickAnotherWinner: (raffleId: number) =>
      apiPost<RaffleWinnerResponse>(`raffles/${raffleId}/pick-another`, undefined),
    verifyWinner: (raffleId: number) =>
      apiPost<StatusResponse>(`raffles/${raffleId}/verify-winner`, undefined),
    /**
     * Close (POST /{id}/close) or reopen (POST /{id}/reopen) a raffle without
     * touching its winner. verify-winner also closes, but only as the last step of
     * confirming a winner - a raffle that drew nobody needs this instead.
     */
    setStatus: (raffleId: number, closed: boolean) =>
      apiPost<StatusResponse>(`raffles/${raffleId}/${closed ? 'close' : 'reopen'}`, undefined),
  },

  // -- Garapon (admin, hybrid REST) ---------------------------------------------
  garapons: {
    /** GET /api/garapons - all garapons (admin). */
    list: () => apiGet<GaraponsResponse>('garapons'),
    /** GET /api/garapons/{id} - a garapon with prizes, drawing links, draw log. */
    detail: (id: number) => apiGet<GaraponDetailResponse>(`garapons/${id}`),
    /** POST /api/garapons - create a garapon (201). The form omits an id. */
    create: (garapon: Record<string, unknown>) => apiPost<GaraponResponse>('garapons', garapon),
    /** PUT /api/garapons/{id} - full replace of the editable fields. */
    update: (garapon: { id: number } & Record<string, unknown>) =>
      apiPut<OKResponse>(`garapons/${garapon.id}`, garapon),
    /** DELETE /api/garapons/{id} - delete a garapon (204). */
    delete: (id: number) => apiDelete(`garapons/${id}`),
    /** Close (POST /{id}/close) or reopen (POST /{id}/reopen) a garapon. */
    setStatus: (id: number, status: 'open' | 'closed') =>
      status === 'closed'
        ? apiPost<StatusResponse>(`garapons/${id}/close`, undefined)
        : apiPost<StatusResponse>(`garapons/${id}/reopen`, undefined),
    /** POST /api/garapons/{id}/players - create a per-player drawing link (returns its token). */
    createPlayer: (garaponId: number, body: { player_name: string; max_draws: number }) =>
      apiPost<GaraponPlayerResponse>(`garapons/${garaponId}/players`, body),
    /** DELETE /api/garapons/{id}/players/{playerId} - delete a drawing link (204). */
    deletePlayer: (garaponId: number, playerId: number) =>
      apiDelete(`garapons/${garaponId}/players/${playerId}`),
  },

  // -- Garapon (public player view, via per-player token) -----------------------
  garapon: {
    /** GET /api/garapon/{token} - the player view (garapon + prizes + their record). */
    get: (token: string) => apiGet<GaraponPublicResponse>(`garapon/${enc(token)}`),
    /** POST /api/garapon/{token}/draw - perform one authoritative draw. */
    draw: (token: string) => apiPost<GaraponDrawResponse>(`garapon/${enc(token)}/draw`, {}),
  },

  // -- Affiliates (admin) -------------------------------------------------------
  affiliates: {
    /** GET /api/affiliates - all affiliates + the shared Discord webhook (admin). */
    list: () => apiGet<AffiliatesResponse>('affiliates'),
    create: (affiliate: Record<string, unknown>) =>
      apiPost<AffiliateResponse>('affiliates', affiliate),
    update: (id: number, affiliate: Record<string, unknown>) =>
      apiPut<OKResponse>(`affiliates/${id}`, affiliate),
    delete: (id: number) => apiDelete(`affiliates/${id}`),
    /** POST /api/affiliates/reorder - persist a new drag order (top-first ids). */
    reorder: (orderedIds: number[]) =>
      apiPost<OKResponse>('affiliates/reorder', { ordered_ids: orderedIds }),
    /** POST /api/affiliates/{id}/post - post the affiliate's embed to the shared webhook now. */
    post: (id: number) => apiPost<AffiliateResponse>(`affiliates/${id}/post`, undefined),
    /** PUT /api/affiliates/webhook - set the single shared Discord webhook ('' clears). */
    setWebhook: (webhookUrl: string) =>
      apiPut<AffiliateWebhookResponse>('affiliates/webhook', { webhook_url: webhookUrl }),
  },

  // -- Tea Rooms (admin) --------------------------------------------------------
  teaRooms: {
    /** GET /api/tea-rooms - all rooms + the shared Discord webhook (admin/perm). */
    list: () => apiGet<TeaRoomsResponse>('tea-rooms'),
    /** POST /api/tea-rooms - create a room (201). */
    create: (room: Record<string, unknown>) =>
      apiPost<TeaRoomResponse>('tea-rooms', { tea_room: room }),
    /** PUT /api/tea-rooms/{id} - full replace of the editable fields. */
    update: (id: number, room: Record<string, unknown>) =>
      apiPut<TeaRoomResponse>(`tea-rooms/${id}`, { tea_room: room }),
    /**
     * PATCH /api/tea-rooms/{id} - toggle the open/discounted flags or set the lock.
     * `locked` and `locked_until` move independently: `locked` alone locks or
     * unlocks, `locked_until` alone re-times a lock already in place. A UTC
     * RFC-3339 `locked_until` expires the lock server-side; '' leaves it standing
     * until someone unlocks the room.
     */
    patch: (
      id: number,
      fields: {
        open?: boolean
        discounted?: boolean
        locked?: boolean
        locked_until?: string
      },
    ) => apiPatch<TeaRoomResponse>(`tea-rooms/${id}`, fields),
    /** DELETE /api/tea-rooms/{id} - delete a room (204). */
    delete: (id: number) => apiDelete(`tea-rooms/${id}`),
    /** POST /api/tea-rooms/reorder - persist a new drag order (top-first ids). */
    reorder: (orderedIds: number[]) =>
      apiPost<OKResponse>('tea-rooms/reorder', { ordered_ids: orderedIds }),
    /** POST /api/tea-rooms/{id}/post - post a room's embed to the shared webhook now. */
    post: (id: number) => apiPost<TeaRoomResponse>(`tea-rooms/${id}/post`, undefined),
    /** PUT /api/tea-rooms/webhook - set the single shared Discord webhook ('' clears). */
    setWebhook: (webhookUrl: string) =>
      apiPut<TeaRoomWebhookResponse>('tea-rooms/webhook', { webhook_url: webhookUrl }),
  },

  // -- Festival Map (admin CRUD + the public read API) --------------------------
  festivalMaps: {
    /** GET /api/festival-maps - every map with its stall count (admin). */
    list: () => apiGet<FestivalMapsResponse>('festival-maps'),
    /** GET /api/festival-maps/{id} - a map with its stalls. */
    detail: (id: number) => apiGet<FestivalMapDetailResponse>(`festival-maps/${id}`),
    /** POST /api/festival-maps - create a map (201). The form omits an id. */
    create: (map: Record<string, unknown>) => apiPost<FestivalMapResponse>('festival-maps', map),
    /** PUT /api/festival-maps/{id} - full replace of the editable fields. */
    update: (map: { id: number } & Record<string, unknown>) =>
      apiPut<OKResponse>(`festival-maps/${map.id}`, map),
    /** PATCH /api/festival-maps/{id} - set the publish status. */
    setStatus: (id: number, status: string) =>
      apiPatch<StatusResponse>(`festival-maps/${id}`, { status }),
    /** DELETE /api/festival-maps/{id} - delete a map and its stalls (204). */
    delete: (id: number) => apiDelete(`festival-maps/${id}`),
    /** GET /api/festival-maps/public - the published maps (no auth). */
    publicList: () => apiGet<PublicFestivalMapsResponse>('festival-maps/public'),
    /** GET /api/festival-maps/public/{id} - one published map with its stalls (no
     *  auth). The segment is the map's shortcode or its numeric id; the server
     *  resolves either. */
    publicDetail: (idOrSlug: string) =>
      apiGet<PublicFestivalMap>(`festival-maps/public/${enc(idOrSlug)}`),
  },

  // -- Festival dashboard (public) ----------------------------------------------
  //
  // Its own prefix rather than festival-maps/: the server treats every POST under
  // that one as an admin edit.
  festivalDashboard: {
    /** POST /api/festival-dashboard/{id} - one participant's standing at a festival,
     *  read from whatever this device holds. POST so the tokens and the name stay
     *  out of URLs and the access log. */
    get: (
      idOrSlug: string,
      body: { card_token: string; garapon_token: string; name: string; world: string },
    ) => apiPost<FestivalDashboard>(`festival-dashboard/${enc(idOrSlug)}`, body),
  },

  // -- Ema Plaques (admin) -------------------------------------------------------
  emaBoards: {
    /** GET /api/ema-boards - every board, with review/request counts. */
    list: () => apiGet<EmaBoardsResponse>('ema-boards'),
    /** GET /api/ema-boards/{id} - one board with its windows (webhooks included). */
    detail: (id: number) => apiGet<EmaBoardResponse>(`ema-boards/${id}`),
    /** POST /api/ema-boards - create a board (201). */
    create: (board: Record<string, unknown>) => apiPost<EmaBoardResponse>('ema-boards', board),
    /** PUT /api/ema-boards/{id} - replace a board's fields and windows. */
    update: (id: number, board: Record<string, unknown>) =>
      apiPut<OKResponse>(`ema-boards/${id}`, board),
    /** PATCH /api/ema-boards/{id} - open or close a board. */
    setStatus: (id: number, status: 'open' | 'closed') =>
      apiPatch<StatusResponse>(`ema-boards/${id}`, { status }),
    /** DELETE /api/ema-boards/{id} - delete a board and its plaques (204). */
    delete: (id: number) => apiDelete(`ema-boards/${id}`),
    /** GET /api/ema-boards/{id}/plaques - the review queue and the record. */
    plaques: (id: number) => apiGet<EmaPlaquesResponse>(`ema-boards/${id}/plaques`),
    /** POST .../plaques/{plaqueId}/approve - post a site plaque to Discord. */
    approve: (boardId: number, plaqueId: number) =>
      apiPost<EmaPlaqueResponse>(`ema-boards/${boardId}/plaques/${plaqueId}/approve`, undefined),
    /** POST .../plaques/{plaqueId}/reject - refuse a site plaque. */
    reject: (boardId: number, plaqueId: number) =>
      apiPost<EmaPlaqueResponse>(`ema-boards/${boardId}/plaques/${plaqueId}/reject`, undefined),
  },
  emaRequests: {
    /** GET /api/ema-requests - the artist's queue across every board. */
    list: () => apiGet<EmaPlaquesResponse>('ema-requests'),
    /** PATCH /api/ema-requests/{id} - mark a request complete, or reopen it. */
    setComplete: (id: number, complete: boolean) =>
      apiPatch<EmaPlaqueResponse>(`ema-requests/${id}`, { complete }),
    /**
     * POST /api/ema-requests/{id}/post - post the artist's finished plaque (multipart
     * `image`) to its day's Discord webhook and mark the request complete.
     */
    post: (id: number, form: FormData) =>
      apiUpload<EmaPlaqueResponse>(`ema-requests/${id}/post`, form),
  },

  // -- Ema Plaques (public submissions) ------------------------------------------
  //
  // Singular /api/ema/... so the admin prefixes' broadcast middleware leaves them
  // alone.
  ema: {
    /** POST /api/ema/{id}/plaques - a site-designed plaque, held for review. */
    submitPlaque: (boardId: number, form: FormData) =>
      apiUpload<EmaPlaqueResponse>(`ema/${boardId}/plaques`, form),
    /** POST /api/ema/{id}/requests - ask the board's artist for a plaque. */
    request: (
      boardId: number,
      body: { name: string; world: string; description: string; turnstile_token: string },
    ) => apiPost<EmaPlaqueResponse>(`ema/${boardId}/requests`, body),
    /** GET /api/google-fonts - Google Fonts family names, read with the server's key. */
    googleFonts: () => apiGet<GoogleFontsResponse>('google-fonts'),
  },

  // -- Stamp Rally (admin, hybrid REST) -----------------------------------------
  stampRallies: {
    /** GET /api/stamp-rallies - all rallies (admin). */
    list: () => apiGet<StampRalliesResponse>('stamp-rallies'),
    /** GET /api/stamp-rallies/{id} - a rally with stamps, prizes, and issued cards. */
    detail: (id: number) => apiGet<StampRallyDetailResponse>(`stamp-rallies/${id}`),
    /** GET /api/stamp-rallies/{id}/logs - the event-wide stamp collection log. */
    logs: (id: number) => apiGet<StampRallyLogsResponse>(`stamp-rallies/${id}/logs`),
    /** POST /api/stamp-rallies - create a rally (201). The form omits an id. */
    create: (rally: Record<string, unknown>) => apiPost<StampRallyResponse>('stamp-rallies', rally),
    /** PUT /api/stamp-rallies/{id} - full replace of the editable fields. */
    update: (rally: { id: number } & Record<string, unknown>) =>
      apiPut<OKResponse>(`stamp-rallies/${rally.id}`, rally),
    /** DELETE /api/stamp-rallies/{id} - delete a rally (204). */
    delete: (id: number) => apiDelete(`stamp-rallies/${id}`),
    /** Close (POST /{id}/close) or reopen (POST /{id}/reopen) a rally. */
    setStatus: (id: number, status: 'open' | 'closed') =>
      status === 'closed'
        ? apiPost<StatusResponse>(`stamp-rallies/${id}/close`, undefined)
        : apiPost<StatusResponse>(`stamp-rallies/${id}/reopen`, undefined),
    /** PATCH /api/stamp-rallies/{id}/stamps/{stampId} - pause/resume a single stamp. */
    setStampPaused: (rallyId: number, stampId: number, paused: boolean) =>
      apiPatch<PausedResponse>(`stamp-rallies/${rallyId}/stamps/${stampId}`, { paused }),
    /** POST /api/stamp-rallies/{id}/cards - issue a tokenized participant card (returns its token). */
    createCard: (rallyId: number, participantName: string) =>
      apiPost<StampRallyCardResponse>(`stamp-rallies/${rallyId}/cards`, {
        participant_name: participantName,
      }),
    /** DELETE /api/stamp-rallies/{id}/cards/{cardId} - delete a participant card (204). */
    deleteCard: (rallyId: number, cardId: number) =>
      apiDelete(`stamp-rallies/${rallyId}/cards/${cardId}`),
  },

  // -- Stamp Rally (public, via per-participant card token) ---------------------
  stampCard: {
    /** GET /api/stamp-card/{token} - the participant card view (no passwords). */
    get: (token: string) => apiGet<PublicStampCard>(`stamp-card/${enc(token)}`),
    /** POST /api/stamp-card/{token}/stamp - collect a stamp by password. */
    stamp: (token: string, password: string) =>
      apiPost<StampSubmitResponse>(`stamp-card/${enc(token)}/stamp`, { password }),
  },

  // -- Public stamp-rally sign-up (self-service) --------------------------------
  //
  // Singular paths, like stamp-card above: the plural /api/stamp-rallies tree is
  // the permission-gated admin surface.
  stampSignup: {
    /** GET /api/stamp-signup - rallies currently open to public sign-up. */
    list: () => apiGet<SignupRalliesResponse>('stamp-signup'),
    /** POST /api/stamp-signup/{id} - issue yourself a card (+ garapon link if paired). */
    signUp: (rallyId: number, name: string, world: string, turnstileToken: string) =>
      apiPost<StampSignupResponse>(`stamp-signup/${rallyId}`, {
        name,
        world,
        turnstile_token: turnstileToken,
      }),
    /** POST /api/stamp-lookup - find your links by the exact name you signed up with.
     *  POST, not GET, so the name stays out of URLs and the access log. */
    lookup: (name: string, world: string) =>
      apiPost<StampLookupResponse>('stamp-lookup', { name, world }),
  },

  // -- Book clubs / reading lists -----------------------------------------------
  //
  // Reading lists are nested under their owning club: every function takes the
  // club slug as its first argument and targets /api/book-clubs/{club}/reading-
  // lists.... The two club-agnostic utilities (uploadImage, lookup*) keep the flat
  // /api/bookclub/* paths.
  bookclub: {
    /** GET /api/book-clubs/{club}/reading-lists - reading lists for a club (no items). */
    lists: (club: string) => apiGet<ReadingListsResponse>(`book-clubs/${enc(club)}/reading-lists`),
    /** GET /api/book-clubs/{club}/reading-lists/{id} - a reading list with its items. */
    listDetail: (club: string, id: number) =>
      apiGet<ReadingListDetailResponse>(`book-clubs/${enc(club)}/reading-lists/${id}`),
    /** POST /api/book-clubs/{club}/reading-lists - create a list (201; returns it with its items). */
    createList: (club: string, title: string) =>
      apiPost<ReadingListDetailResponse>(`book-clubs/${enc(club)}/reading-lists`, { title }),
    /** PUT /api/book-clubs/{club}/reading-lists/{id} - rename a list. */
    renameList: (club: string, id: number, title: string) =>
      apiPut<OKResponse>(`book-clubs/${enc(club)}/reading-lists/${id}`, { title }),
    /** DELETE /api/book-clubs/{club}/reading-lists/{id} - delete a list + its items (204). */
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
    /** DELETE /api/book-clubs/{club}/reading-lists/{id}/items/{itemId} - delete an item (204). */
    deleteItem: (club: string, listId: number, itemId: number) =>
      apiDelete(`book-clubs/${enc(club)}/reading-lists/${listId}/items/${itemId}`),
    publish: (club: string, listId: number) =>
      apiPost<PublishResponse>(`book-clubs/${enc(club)}/reading-lists/${listId}/publish`, {}),
    uploadImage: (form: FormData, onProgress?: (percent: number) => void) =>
      apiUpload<BookclubUploadResponse>('bookclub/upload', form, { onProgress }),
    /** GET /api/bookclub/lookup?q=... - AniList suggestions shaped like items. */
    lookup: (query: string) => apiGet<BookclubLookupResponse>(`bookclub/lookup?q=${enc(query)}`),
    /** GET /api/bookclub/lookup?id=... - a single AniList title by numeric id. */
    lookupById: (id: number) => apiGet<BookclubLookupResponse>(`bookclub/lookup?id=${id}`),
  },

  // -- Announcement management --------------------------------------------------
  announcements: {
    /** GET /api/announcement-types - Discord destinations. */
    types: () => apiGet<AnnouncementTypesResponse>('announcement-types'),
    /** POST /api/announcement-types - create a type. */
    createType: (form: AnnouncementTypeForm) =>
      apiPost<AnnouncementTypeResponse>('announcement-types', {
        name: form.name,
        webhook_url: form.webhook_url,
      }),
    /** PUT /api/announcement-types/{id} - replace a type. */
    updateType: (id: number, form: AnnouncementTypeForm) =>
      apiPut<AnnouncementTypeResponse>(`announcement-types/${id}`, {
        name: form.name,
        webhook_url: form.webhook_url,
      }),
    deleteType: (id: number) => apiDelete(`announcement-types/${id}`),

    /** GET /api/announcement-roles - taggable Discord roles. */
    roles: () => apiGet<AnnouncementRolesResponse>('announcement-roles'),
    /** POST /api/announcement-roles - create a taggable role. */
    createRole: (form: AnnouncementRoleForm) =>
      apiPost<AnnouncementRoleResponse>('announcement-roles', {
        name: form.name,
        role_id: form.role_id,
      }),
    /** PUT /api/announcement-roles/{id} - replace a taggable role. */
    updateRole: (id: number, form: AnnouncementRoleForm) =>
      apiPut<AnnouncementRoleResponse>(`announcement-roles/${id}`, {
        name: form.name,
        role_id: form.role_id,
      }),
    deleteRole: (id: number) => apiDelete(`announcement-roles/${id}`),

    /** GET /api/announcements - all announcements (filtering is client-side). */
    list: () => apiGet<AnnouncementsResponse>('announcements'),
    /**
     * Create (POST) or update (PUT /{id}) an announcement, chosen by whether an id
     * is supplied. The store builds `payload` from the form, having already
     * converted local times -> the stored UTC instants / UTC recurrence fields.
     * Both wrap the payload under `{announcement}`.
     */
    save: (id: number, payload: Partial<Announcement>) =>
      id
        ? apiPut<AnnouncementResponse>(`announcements/${id}`, { announcement: payload })
        : apiPost<AnnouncementResponse>('announcements', { announcement: payload }),
    /** DELETE /api/announcements/{id} - delete an announcement (204). */
    delete: (id: number) => apiDelete(`announcements/${id}`),
    /** POST /api/announcements/reorder - persist a new drag order (top-first ids). */
    reorder: (orderedIds: number[]) =>
      apiPost<OKResponse>('announcements/reorder', { ordered_ids: orderedIds }),
    /** POST /api/announcements/{id}/send - post an announcement's embed to Discord now. */
    sendNow: (id: number) => apiPost<AnnouncementResponse>(`announcements/${id}/send`, undefined),
    /** POST /api/announcements/{id}/skip - set how many upcoming scheduled
     *  occurrences to pass over (0 clears a pending skip). */
    setSkip: (id: number, count: number) =>
      apiPost<AnnouncementResponse>(`announcements/${id}/skip`, { count }),
  },

  // -- Central image hosting (System -> Images) ----------------------------------
  images: {
    /** GET /api/image-categories - permanent + custom categories. */
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
    /** DELETE /api/image-categories/{dir} - delete a custom category + its files (204). */
    deleteCategory: (dir: string) => apiDelete(`image-categories/${enc(dir)}`),
    /** GET /api/images?dir=... - images in a category (newest first). */
    list: (dir: string) => apiGet<ImagesResponse>(`images?dir=${enc(dir)}`),
    /**
     * POST /api/images/upload - multipart "dir" + one or more "files".
     * Uses apiUpload (XHR) so large uploads aren't killed by the 30s fetch
     * timeout and can report progress; pass `onProgress` for a live percentage.
     */
    upload: (form: FormData, onProgress?: (percent: number) => void) =>
      apiUpload<ImagesUploadResponse>('images/upload', form, { onProgress }),
    /** DELETE /api/images?dir=...&name=... - remove an image from a category (204). */
    deleteImage: (dir: string, name: string) =>
      apiDelete(`images?dir=${enc(dir)}&name=${enc(name)}`),
  },

  // -- Fonts (Atelier -> Font Upload) --------------------------------------------
  fonts: {
    /** GET /api/fonts - fonts grouped by base name with their variants. */
    list: () => apiGet<FontsResponse>('fonts'),
    /** POST /api/fonts/upload - multipart upload of one or more "files" fields.
     *  XHR-based (apiUpload) so large font files aren't cut off by the fetch timeout. */
    upload: (form: FormData, onProgress?: (percent: number) => void) =>
      apiUpload<FontUploadResponse>('fonts/upload', form, { onProgress }),
    /** DELETE /api/fonts/{name} - remove ONE variant file by name (204). */
    deleteFile: (name: string) => apiDelete(`fonts/${enc(name)}`),
    /** PATCH /api/fonts/{name} - rename one variant file (fails if the target exists). */
    renameFile: (name: string, newName: string) =>
      apiPatch<NamedOKResponse>(`fonts/${enc(name)}`, { new_name: newName }),
    /** PATCH /api/fonts/families/{base} - partial update of a font's metadata:
     *  CSS family name ("" = base default), served variant type ("" = auto),
     *  and/or its per-font allowed-site origins. */
    updateFamily: (base: string, fields: { family?: string; serve?: string; origins?: string[] }) =>
      apiPatch<OKResponse>(`fonts/families/${enc(base)}`, fields),
    /** DELETE /api/fonts/families/{base} - delete a whole font (all variants, 204). */
    deleteFont: (base: string) => apiDelete(`fonts/families/${enc(base)}`),
  },

  // -- Carrd image hosting (System -> Carrd Upload) ------------------------------
  carrd: {
    /** GET /api/carrd/projects - list project folders under <webRoot>/carrd. */
    projects: () => apiGet<CarrdProjectsResponse>('carrd/projects'),
    /** POST /api/carrd/projects - create a project (folder optional). */
    createProject: (title: string, folder: string) =>
      apiPost<CarrdProjectCreateResponse>('carrd/projects', { title, folder }),
    /** PATCH /api/carrd/projects/{folder} - rename a project's title and/or folder. */
    renameProject: (folder: string, title: string, newFolder: string) =>
      apiPatch<CarrdProjectCreateResponse>(`carrd/projects/${enc(folder)}`, {
        title,
        new_folder: newFolder,
      }),
    /** DELETE /api/carrd/projects/{folder} - delete a project folder + contents (204). */
    deleteProject: (folder: string) => apiDelete(`carrd/projects/${enc(folder)}`),
    /** GET /api/carrd/images?folder=...&path=... - sub-dirs + images at a path. */
    images: (folder: string, path = '') =>
      apiGet<CarrdImagesResponse>(`carrd/images?folder=${enc(folder)}&path=${enc(path)}`),
    /** POST /api/carrd/upload - multipart upload of "files" to "folder"/"path".
     *  XHR-based (apiUpload) so large uploads aren't cut off by the fetch timeout. */
    upload: (form: FormData, onProgress?: (percent: number) => void) =>
      apiUpload<CarrdUploadResponse>('carrd/upload', form, { onProgress }),
    /** DELETE /api/carrd/images?folder=...&path=...&name=... - remove an image at a path (204). */
    deleteImage: (folder: string, path: string, name: string) =>
      apiDelete(`carrd/images?folder=${enc(folder)}&path=${enc(path)}&name=${enc(name)}`),
    /** POST /api/carrd/images/dirs - create a sub-directory at a path. */
    createDir: (folder: string, path: string, name: string) =>
      apiPost<NamedOKResponse>('carrd/images/dirs', { folder, path, name }),
    /** DELETE /api/carrd/images/dirs?folder=...&path=... - delete a sub-directory + contents (204). */
    deleteDir: (folder: string, path: string) =>
      apiDelete(`carrd/images/dirs?folder=${enc(folder)}&path=${enc(path)}`),
  },
}
