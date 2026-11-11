# Senpan App Suite

A self-hosted suite of community tools for the **Senpan Tea House** and
**Atelier Yao** - real-time Bingo, raffles, a festival lottery drum, Discord
announcements, book-club reading lists, font/image hosting, and a fully
themeable, permissioned admin dashboard. A **Go + SQLite** backend serves a
JSON API (`/api/*`) and its live event streams, documented in [`API.md`](API.md), and the **Vue 3 +
TypeScript** single-page app (built with Vite, installable as a PWA), whose build
it embeds: one binary, one version.

## What's in the suite

- **Bingo** - admins run live games (draw numbers, verify winners, manage
  cards/patterns/presets) and keep a winners log; players join a board by ID,
  stamp cells in real time, export their card as an image, and fire an in-game
  **Yoever** reaction. Visitors can submit a self-designed card for admin approval.
- **Raffles** - create raffles, collect entries from players, and draw winners.
- **Festival** - a **Garapon** lottery-drum and a **Stamp Rally**, each with
  per-player tokenized links (draw / stamp cards) plus public token-gated views.
- **Senpan Tea House** - scheduled **Discord announcements** (typed
  destinations + roles), **book-club reading lists** (with cover lookup), partner
  **affiliates**, and bookable **tea rooms** (with a public availability API).
- **Atelier Yao** - **font upload** (served with CORS for use on external sites)
  and **Carrd image hosting**.
- **System** - app **settings**, a structured **theme editor** (design tokens
  with a live preview and a WCAG 2.1 contrast report), central **image hosting**,
  **user accounts** with per-page permissions (password **or** passwordless
  WebAuthn passkeys, with an optional Cloudflare Turnstile bot check), and a live
  **server-log viewer** (typed/colored columns, live tail over the server-sent event stream, and a
  runtime DEBUG toggle).

## Stack

- **Frontend:** Vue 3 (SFC, `<script setup>`) - TypeScript (strict) - Pinia -
  Vue Router (history mode, lazy routes) - Vite 8 - PWA - Vitest
- **Backend:** Go 1.27.4+ (stdlib HTTP, method-pattern routing) - SQLite (WAL,
  pure-Go `modernc.org/sqlite`) - `alexedwards/scs` sessions -
  argon2id + `go-webauthn` passkeys - structured JSON logging (`slog` +
  `timberjack` rotation)
- **Tooling:** openapi-typescript (the SPA's types) and NSwag (the plugin's client),
  both from the API's OpenAPI document - ESLint + Stylelint + Prettier -
  golangci-lint - GitHub Actions CI - Dependabot. The lint configs, the CI workflows
  and the deploy module come from
  [shared-tooling](https://github.com/jwill89/shared-tooling), a git submodule at
  `shared/`.

## Quick start

**Prerequisites:** Node 26 LTS and Go (version per `backend/go.mod`), and the
Font Awesome Pro package token in your `~/.npmrc`
(`//npm.fontawesome.com/:_authToken=<token>`) - `frontend/.npmrc` names the registry
but holds no token.

```bash
# 0. Once per clone: shared-tooling, at shared/ (npm ci needs it)
git submodule update --init

# 1. Backend - run the API server on :8080. It embeds the front end's
#    build, so build that once first: cd frontend && npm ci && npm run build
cd backend
# -log-file="" keeps logs on stdout only (the default writes to /var/log/senpan)
# -webhook-dry-run suppresses outbound Discord posts. Keep it on: devdata is a copy
# of the live database and carries real webhook URLs, so the announcement scheduler
# would otherwise post to production channels by itself. Use
# -webhook-override <url> to send everything to a test channel instead.
go run . -addr :8080 -db ../devdata/database.sqlite -data ../devdata/webroot -log-file "" -webhook-dry-run

# 2. Frontend (in a second terminal)
cd frontend
npm ci
npm run dev           # Vite dev server on :5173, proxies /api -> :8080
```

With [Task](https://taskfile.dev) installed, `task setup` installs the frontend, and
`task dev` runs both servers (`task` lists every command).
Open <http://localhost:5173>. The admin area is at `/admin/login`. A fresh database
seeds a bootstrap **`admin` / `admin`** account - change its password after first
login.

> **Note:** the API's OpenAPI document (`frontend/src/api/openapi.json`), the SPA's
> types from it (`schema.d.ts`) and the plugin's client
> (`plugins/SenpanCompanion/Api/SenpanApi.g.cs`) are generated and **committed**.
> After an API change, run `task gen` (needs Go, Node and the .NET SDK) and commit
> the result with the change; CI's `generated` job fails when a committed copy is
> stale.

## Versioning

Since 4.0.0 the app has **one** semver version, since one binary ships the SPA and
the API together: `frontend/package.json`, which
`backend/internal/version/version.go` must equal (a test checks; served at
`GET /api/v1/version` and `GET /healthz`). The plugin keeps its own, in
`SenpanCompanion.csproj` and `plugins/pluginmaster.json`. The admin sidebar footer
shows both, and flags an open tab whose server now runs a different version.

- Record every change in **[`CHANGELOG.md`](CHANGELOG.md)** (App or Plugin) and
  bump the matching version.

## Common commands

| Command (in `frontend/`)     | Purpose                                            |
| ---------------------------- | -------------------------------------------------- |
| `npm run dev`                | Dev server (HMR), proxies `/api` -> `:8080`         |
| `npm run test`               | Vitest unit/component tests (one-shot)             |
| `npm run test:watch`         | Vitest watch mode                                  |
| `npm run test:coverage`      | Tests with v8 coverage, failing below the floor (CI) |
| `npm run typecheck`          | `vue-tsc` type check                               |
| `npm run lint` / `lint:check`| ESLint + Stylelint with / without `--fix`          |
| `npm run format:check`       | Prettier check                                     |
| `npm run build`              | Type-check + production build -> `backend/internal/web/dist/` (embedded by the binary) |

| Command (in `backend/`)      | Purpose                          |
| ---------------------------- | -------------------------------- |
| `go build ./...`             | Build the backend (after `npm run build`) |
| `go vet ./...`               | Static analysis                  |
| `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run ./...` | Lint (`.golangci.yml`, written from the shared template; Senpan's own rules are in `.golangci.local.yml`) |
| `go test ./...`              | Backend tests                    |
| `go run ./cmd/schema-gen`    | Write `schema.sql`, the schema the migrations build |

## Deploying

`scripts/deploy.ps1` ships the newest release CI tagged on main, built in a
checkout of its own: the app (`-Target app`, the default: one binary, the SPA inside
it) or the plugin (`-Target plugin`). An app deploy installs the service's systemd unit
and the Apache vhost from `deploy/` with it. It checks the live site through Cloudflare
and rolls back what it changed if that fails. An app deploy also keeps the daily
database backup installed and pulls the live database into `devdata/` (`-NoDbPull`
skips that). See [`docs/ops.md`](docs/ops.md) for the host layout (the data
folder, the vhost, the env file) and host setup.

## Testing & CI

- Frontend tests use **Vitest + Vue Test Utils** (jsdom); test files are
  in `src/__tests__/`, named `*.spec.ts`. See the **Testing** section of
  [`AGENTS.md`](AGENTS.md) for conventions.
- CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs the same gates
  on every push / pull request, on shared-tooling's reusable workflows: a
  **frontend** job (lint -> format -> conventions -> typecheck -> tests with a
  coverage floor -> build, plus `npm audit`), a **generated** job (the committed API
  document and the types built from it are what the generators make now), a **backend** job (golangci-lint
  -> vet -> govulncheck -> race-detector tests with a coverage floor -> build, on the
  frontend job's build, which the binary embeds; the
  tests check every answer from an operation on huma against its document), a **plugin**
  job (build with warnings as errors -> `dotnet format`), an **e2e** job (Playwright
  against the binary, the SPA inside it), a **scripts** job
  (PSScriptAnalyzer) and a **docs** job (markdownlint and the shared repo files).
  `scripts/check.ps1` runs them all locally.

## Documentation

- **[`API.md`](API.md)** - the API: its OpenAPI 3.1 document, generated by huma and
  committed as `frontend/src/api/openapi.json` (`task gen`), its errors, caching and
  conventions.
- **[`AGENTS.md`](AGENTS.md)** - full architecture, data flow, DB schema,
  conventions, and "how to extend" guide (the primary reference for contributors
  and AI agents).
- **[`CONTRIBUTING.md`](CONTRIBUTING.md)** - dev workflow, the local checks that
  mirror CI, and versioning/changelog rules.
- **[`CHANGELOG.md`](CHANGELOG.md)** - the app's and the plugin's version history.
- **[`docs/ops.md`](docs/ops.md)** - deployment (the data folder and the
  vhost, persistent uploads, the font and Carrd hosts, PWA caching).

## License

Proprietary - © 2026 mathdad, All Rights Reserved. See [`LICENSE`](LICENSE).

## Project layout

```text
frontend/   Vue 3 + TS SPA (Vite)        - src/{views,components,stores,router,lib,types}
backend/    Go backend                    - internal/{server,api,store,bingo,game,live,model,version,web}
plugins/    FFXIV Dalamud plugin (C#)     - SenpanCompanion/ + pluginmaster.json (its custom repo)
scripts/    check.ps1 (the local gate) + deploy.ps1
shared/     shared-tooling (git submodule) - lint configs, CI workflows, the deploy module
deploy/     Host artifacts                - the systemd unit, the Apache vhost + persistent images/
devdata/    Local dev sandbox: SQLite DBs + built binaries + the dev data folder (created at runtime, gitignored)
```
