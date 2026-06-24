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
  LoginResponse,
  RegisterResponse,
  UsersResponse,
  BoardResponse,
  CardListEntry,
  CardResponse,
  DrawResult,
  FrequentWinnersResponse,
  GameStateResponse,
  GenerateCardsResponse,
  GenerateSingleCardResponse,
  OkResponse,
  PatternsResponse,
  PresetsResponse,
  PresetCreateResponse,
  RaffleDetailResponse,
  RaffleEnterResponse,
  RaffleEntryResponse,
  RaffleWinnerResponse,
  RafflesResponse,
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
} from '@/types/api'
import type { AppSettings } from '@/types/api'

const enc = encodeURIComponent

export const endpoints = {
  // ── Auth ───────────────────────────────────────────────────────────────────
  auth: {
    /** GET /api/auth — current auth status + the logged-in user (always 200). */
    check: () => apiGet<AuthCheckResponse>('auth'),
    /** POST /api/auth {login} — bad credentials 401 without a global redirect. */
    login: (username: string, password: string) =>
      apiPost<LoginResponse>(
        'auth',
        { action: 'login', username, password },
        { skipAuthRedirect: true },
      ),
    /** POST /api/auth {logout}. */
    logout: () =>
      apiPost<OkResponse>('auth', { action: 'logout' }, { skipAuthRedirect: true }),
    /** POST /api/register — create an account (hidden page; pending activation). */
    register: (username: string, password: string) =>
      apiPost<RegisterResponse>('register', { username, password }, { skipAuthRedirect: true }),
  },

  // ── Users (admin) + self-service account ─────────────────────────────────────
  users: {
    /** GET /api/users — all accounts (admin only). */
    list: () => apiGet<UsersResponse>('users'),
    setActive: (id: number, active: boolean) =>
      apiPost<OkResponse>('users', { action: 'set_active', id, active }),
    setAdmin: (id: number, admin: boolean) =>
      apiPost<OkResponse>('users', { action: 'set_admin', id, admin }),
    setPermissions: (id: number, permissions: string[]) =>
      apiPost<OkResponse>('users', { action: 'set_permissions', id, permissions }),
    setPassword: (id: number, password: string) =>
      apiPost<OkResponse>('users', { action: 'set_password', id, password }),
    delete: (id: number) => apiPost<OkResponse>('users', { action: 'delete', id }),
  },
  account: {
    /** POST /api/account — change the logged-in user's own password. */
    changePassword: (currentPassword: string, newPassword: string) =>
      apiPost<OkResponse>('account', {
        action: 'change_password',
        current_password: currentPassword,
        new_password: newPassword,
      }),
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
    setDelay: (delay: number) => apiPost<OkResponse>('game', { action: 'set_delay', delay }),
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
    delete: (id: number) => apiPost<OkResponse>('winners-log', { action: 'delete', id }),
    deleteAll: () => apiPost<OkResponse>('winners-log', { action: 'delete_all' }),
  },

  // ── Cards ────────────────────────────────────────────────────────────────────
  cards: {
    list: () => apiGet<{ cards: CardListEntry[] }>('cards'),
    generate: (count: number) =>
      apiPost<GenerateCardsResponse>('cards', { action: 'generate', count }),
    generateSingle: (playerName: string) =>
      apiPost<GenerateSingleCardResponse>('cards', {
        action: 'generate_single',
        player_name: playerName,
      }),
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

  // ── Game presets ─────────────────────────────────────────────────────────────
  presets: {
    /** GET /api/presets — all saved game presets. */
    list: () => apiGet<PresetsResponse>('presets'),
    create: (name: string, patternIds: number[], gameDetails: string) =>
      apiPost<PresetCreateResponse>('presets', {
        action: 'create',
        name,
        pattern_ids: patternIds,
        game_details: gameDetails,
      }),
    update: (id: number, name: string, patternIds: number[], gameDetails: string) =>
      apiPost<OkResponse>('presets', {
        action: 'update',
        id,
        name,
        pattern_ids: patternIds,
        game_details: gameDetails,
      }),
    delete: (id: number) => apiPost<OkResponse>('presets', { action: 'delete', id }),
  },

  // ── Styles / themes ──────────────────────────────────────────────────────────
  styles: {
    list: () => apiGet<StylesResponse>('styles'),
    activeCss: () => apiGet<ActiveCssResponse>('styles/active'),
    get: (id: number) => apiPost<StyleGetResponse>('styles', { action: 'get', id }),
    create: (name: string, cssContent: string, boardFlourish = '', numberFlourish = '') =>
      apiPost<StyleCreateResponse>('styles', {
        action: 'create',
        name,
        css_content: cssContent,
        board_flourish: boardFlourish,
        number_flourish: numberFlourish,
      }),
    update: (id: number, name: string, cssContent: string, boardFlourish = '', numberFlourish = '') =>
      apiPost<OkResponse>('styles', {
        action: 'update',
        id,
        name,
        css_content: cssContent,
        board_flourish: boardFlourish,
        number_flourish: numberFlourish,
      }),
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

  // ── Announcement management ──────────────────────────────────────────────────
  announcements: {
    /** GET /api/announcement-types — Discord destinations. */
    types: () => apiGet<AnnouncementTypesResponse>('announcement-types'),
    /** Create or update an announcement type (update when form.id is set). */
    saveType: (form: AnnouncementTypeForm) =>
      apiPost<AnnouncementTypeResponse>('announcement-types', {
        action: form.id ? 'update' : 'create',
        id: form.id || 0,
        name: form.name,
        webhook_url: form.webhook_url,
      }),
    deleteType: (id: number) =>
      apiPost<OkResponse>('announcement-types', { action: 'delete', id }),

    /** GET /api/announcement-roles — taggable Discord roles. */
    roles: () => apiGet<AnnouncementRolesResponse>('announcement-roles'),
    /** Create or update a taggable role (update when form.id is set). */
    saveRole: (form: AnnouncementRoleForm) =>
      apiPost<AnnouncementRoleResponse>('announcement-roles', {
        action: form.id ? 'update' : 'create',
        id: form.id || 0,
        name: form.name,
        role_id: form.role_id,
      }),
    deleteRole: (id: number) =>
      apiPost<OkResponse>('announcement-roles', { action: 'delete', id }),

    /** GET /api/announcements — all announcements (filtering is client-side). */
    list: () => apiGet<AnnouncementsResponse>('announcements'),
    /**
     * Create or update an announcement. The store builds `payload` from the form,
     * having already converted local times → the stored UTC instants / UTC
     * recurrence fields.
     */
    save: (id: number, payload: Partial<Announcement>) =>
      apiPost<AnnouncementResponse>('announcements', {
        action: id ? 'update' : 'create',
        id: id || 0,
        announcement: payload,
      }),
    delete: (id: number) => apiPost<OkResponse>('announcements', { action: 'delete', id }),
    /** Persist a new drag-and-drop order (top-first list of announcement ids). */
    reorder: (orderedIds: number[]) =>
      apiPost<OkResponse>('announcements', { action: 'reorder', ordered_ids: orderedIds }),
    /** Post an announcement's embed to Discord immediately. */
    sendNow: (id: number) =>
      apiPost<AnnouncementResponse>('announcements', { action: 'send_now', id }),
    /** Skip the next scheduled occurrence of an announcement. */
    skipNext: (id: number) =>
      apiPost<AnnouncementResponse>('announcements', { action: 'skip_next', id }),
  },

  // ── Central image hosting (System → Images) ──────────────────────────────────
  images: {
    /** GET /api/image-categories — permanent + custom categories. */
    categories: () => apiGet<ImageCategoriesResponse>('image-categories'),
    /** Create or rename a category (rename when an existing dir is supplied). */
    saveCategory: (action: 'create' | 'rename', name: string, dir = '', newDir = '') =>
      apiPost<ImageCategoryActionResponse>('image-categories', {
        action,
        name,
        dir,
        new_dir: newDir,
      }),
    /** POST /api/image-categories {delete} — delete a custom category + its files. */
    deleteCategory: (dir: string) =>
      apiPost<OkResponse>('image-categories', { action: 'delete', dir }),
    /** GET /api/images?dir=… — images in a category (newest first). */
    list: (dir: string) => apiGet<ImagesResponse>(`images?dir=${enc(dir)}`),
    /** POST /api/images/upload — multipart "dir" + one or more "files". */
    upload: (form: FormData) => apiPost<ImagesUploadResponse>('images/upload', form),
    /** POST /api/images {delete} — remove an image from a category. */
    deleteImage: (dir: string, name: string) =>
      apiPost<OkResponse>('images', { action: 'delete', dir, name }),
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

  // ── Carrd image hosting (System → Carrd Upload) ──────────────────────────────
  carrd: {
    /** GET /api/carrd/projects — list project folders under <webRoot>/carrd. */
    projects: () => apiGet<CarrdProjectsResponse>('carrd/projects'),
    /** POST /api/carrd/projects {create} — create a project (folder optional). */
    createProject: (title: string, folder: string) =>
      apiPost<CarrdProjectCreateResponse>('carrd/projects', { action: 'create', title, folder }),
    /** POST /api/carrd/projects {rename} — rename a project's title and/or folder. */
    renameProject: (folder: string, title: string, newFolder: string) =>
      apiPost<CarrdProjectCreateResponse>('carrd/projects', {
        action: 'rename',
        folder,
        title,
        new_folder: newFolder,
      }),
    /** POST /api/carrd/projects {delete} — delete a project folder + contents. */
    deleteProject: (folder: string) =>
      apiPost<OkResponse>('carrd/projects', { action: 'delete', folder }),
    /** GET /api/carrd/images?folder=…&path=… — sub-dirs + images at a path. */
    images: (folder: string, path = '') =>
      apiGet<CarrdImagesResponse>(`carrd/images?folder=${enc(folder)}&path=${enc(path)}`),
    /** POST /api/carrd/upload — multipart upload of "files" to "folder"/"path". */
    upload: (form: FormData) => apiPost<CarrdUploadResponse>('carrd/upload', form),
    /** POST /api/carrd/images {delete} — remove an image at a path in a project. */
    deleteImage: (folder: string, path: string, name: string) =>
      apiPost<OkResponse>('carrd/images', { action: 'delete', folder, path, name }),
    /** POST /api/carrd/images {create_dir} — create a sub-directory at a path. */
    createDir: (folder: string, path: string, name: string) =>
      apiPost<OkResponse>('carrd/images', { action: 'create_dir', folder, path, name }),
    /** POST /api/carrd/images {delete_dir} — delete a sub-directory + contents. */
    deleteDir: (folder: string, path: string) =>
      apiPost<OkResponse>('carrd/images', { action: 'delete_dir', folder, path }),
  },
}

// `CardResponse` is the documented shape for board fetches whose callers only
// read `.card` (winner verify, card preview); re-exported for those sites.
export type { CardResponse }
