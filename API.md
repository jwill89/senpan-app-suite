# Senpan App Suite — API

The HTTP & WebSocket API is described by a machine-readable **OpenAPI 3** spec:

- **Spec (source of truth):** [`backend/openapi.yaml`](backend/openapi.yaml) — also
  served live at **`GET /api/openapi.yaml`**.
- **Interactive reference:** **`GET /api/docs`** renders the spec with
  [Scalar](https://scalar.com) (an explorable, try-it-out API reference). Both
  endpoints are public — the API contract carries no secrets.

## How the spec stays accurate

The spec is **generated from the Go code**, not hand-written prose:

- **Schemas** are reflected from the `model` package structs (the same structs the
  handlers return and tygo generates the frontend types from), so a documented
  response shape can never drift from the code. See
  [`internal/apidoc`](backend/internal/apidoc/).
- **Paths** (auth, params, request bodies, response refs) are a hand-maintained
  table in `internal/apidoc/paths*.go`.
- Regenerate after changing a model struct or a route:

  ```bash
  cd backend && go run ./cmd/openapi-gen   # rewrites backend/openapi.yaml
  ```

- Two CI tests (`internal/apidoc/openapi_test.go`) keep it honest: one fails if the
  committed `openapi.yaml` is stale (regenerate + commit), the other fails if any
  registered route is undocumented (or vice-versa).

## Authentication (orientation)

Identity comes from either a **session cookie** (set by `POST /api/auth` login) or a
**personal access token** — `Authorization: Bearer pat_…` (or `?token=pat_…` on the
WebSocket upgrade). A token inherits its account's permissions, so external clients
(e.g. the FFXIV plugin) pass through the same guards as the SPA.

Each operation's description states its guard: **public**, **auth** (any active
account), **admin**, or **permission:<key>**. Grantable page-permission keys:
`bingo-game`, `bingo-cards`, `bingo-winners-log`, `bingo-patterns`, `bingo-presets`,
`teahouse-announcements`, `teahouse-affiliates`, `teahouse-tea-rooms`, `teahouse-raffles`,
`festival-garapon`, `festival-stamp-rally`, `atelier-fonts`, `atelier-carrd`,
`system-settings`, `system-themes`, `system-images`, plus per-book-club
`bookclub-yaoi` / `bookclub-yuri`. The Users page is admin-only (not grantable).

## WebSocket

`GET /api/ws` isn't an HTTP operation (OpenAPI can't model the upgrade), so it's
documented in the spec's top-level description: the channel is chosen by the `id`
query param (player vs. admin), and the broadcast message types are listed there.

## Conventions

The API is **hybrid REST**: HTTP methods carry the intent, resources live at
predictable paths, and non-CRUD commands are explicit verb sub-paths.

- **Methods:** `GET` reads, `POST` creates (→ `201`) or runs a command, `PUT`
  replaces, `PATCH` applies a partial update, `DELETE` removes (→ `204`).
- **Resources & items:** a collection is `/api/<resource>` and a single item is
  `/api/<resource>/{id}` (string keys such as a card id, font filename, or carrd
  folder use the same shape). Sub-collections nest: `/api/<resource>/{id}/<subs>`
  and `/api/<resource>/{id}/<subs>/{subId}`.
- **Commands (non-CRUD):** state transitions and processes that aren't a plain
  field-set are `POST /api/<resource>/{id}/<verb>` — e.g. `…/close`, `…/reopen`,
  `…/activate`, `…/pick-winner`, an announcement's `…/send`, the game's
  `/api/game/start`. A declarative field-set (including a single reorder or
  toggling a flag) is a `PATCH` instead.
- **Bulk:** bulk writes are `POST` (e.g. `…/reorder`, `/api/cards/generate`);
  bulk deletes are `DELETE /api/<resource>/all` (returning `{ "deleted": N }`).
- All responses are `application/json` with `Cache-Control: no-store`; errors use
  `{ "error": "message" }`. Request bodies are JSON (1 MB cap); uploads are
  `multipart/form-data`.

The one intentional exception is `POST /api/auth` (login/logout) and
`POST /api/register`, which stay small action/credential bodies rather than
resource CRUD.
