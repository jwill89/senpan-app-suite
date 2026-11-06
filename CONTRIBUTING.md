# Contributing

The **Senpan App Suite** is proprietary software (see [`LICENSE`](LICENSE)) -
contributions are by invitation. By submitting a change you confirm you have the
right to do so and agree it becomes part of the Software under that license.

This guide covers the practical workflow. For the architecture, data flow, DB
schema, and "how to extend" details, read [`AGENTS.md`](AGENTS.md) first - it is
the primary reference. New to the repo? Start with [`README.md`](README.md).

## Getting set up

**Prerequisites:** Node 26 LTS (`frontend/.nvmrc`) and Go (version per
`backend/go.mod`). [Task](https://taskfile.dev) is optional: `task` lists the
day-to-day commands in `Taskfile.yml`.

**First-time setup (once per clone):**

```bash
git submodule update --init                # shared-tooling at shared/ - npm ci, check.ps1 and deploy.ps1 need it
git config core.hooksPath .githooks        # pre-push runs scripts/check.ps1 (the CI mirror)
```

The Font Awesome Pro packages install from Font Awesome's registry, which needs the
account's package token in your **`~/.npmrc`**:

```ini
//npm.fontawesome.com/:_authToken=<token>
```

`frontend/.npmrc` is tracked and names the registry, but never holds the token.

The lint configs come from **shared-tooling**, the submodule at `shared/`, and each
project file adds only what is Senpan's own:

- `frontend/eslint.config.js`, `stylelint.config.mjs`, `prettier.config.js` and the
  tsconfigs extend `@jwill89/lint-web`, which npm installs from `../shared/lint/web`
  (as a copy, because `frontend/.npmrc` sets `install-links=true`).
- `backend/.golangci.yml` is written from the shared template - don't edit it.
  Senpan's own golangci-lint rules go in `backend/.golangci.local.yml`; after editing
  it, run `go run -C shared/lint/go . -dir backend` from the repo root and commit
  both.
- The plugin's analyzers and code style are `shared/lint/dotnet`, imported by
  `plugins/Directory.Build.props`; its two exceptions sit below the project marker in
  the root `.editorconfig`.
- PowerShell is linted with `shared/lint/pwsh/PSScriptAnalyzerSettings.psd1`, and the
  Markdown with the shared markdownlint config (`.markdownlint-cli2.jsonc`).
- `.editorconfig` and `.gitattributes` are copied from `shared/repo` above their
  project marker; edit only below it.

```bash
cd backend && go run . -addr :8080 -db ../devdata/database.sqlite -webroot ../devdata/webroot -webhook-dry-run   # backend
cd frontend && npm ci && npm run dev   # frontend
```

`frontend/src/types/api.generated.ts` is generated from the Go models by tygo and
**committed**. Whenever you change a Go model type, run `npm run gen:types` and
commit the result with the change; `npm run gen:check` (and CI) fails when the
committed copy is stale.

## Branches & pull requests

- The default branch is `main`. Don't commit directly to it - work on a feature
  branch and open a pull request.
- Keep each PR focused on one logical change. Rebase on `main` before requesting
  review so CI runs against current code.
- Commit messages: a short, capitalized, imperative subject line (e.g.
  `Add Garapon festival lottery-drum feature.`), with a body when the *why* isn't
  obvious from the diff.

## Before you push - run the same gates as CI

CI fails the pipeline on any of these, so run them locally first.

**Frontend (in `frontend/`):**

```bash
npm run lint:check     # ESLint + Stylelint (no --fix)
npm run format:check   # Prettier
npm run lint:conventions  # CSS naming, the scoped-CSS ratchet, theme tokens
npm run typecheck      # vue-tsc
npm run test:coverage  # Vitest, failing below the coverage floor in vitest.config.ts
npm run build          # production build
```

**Backend (in `backend/`):**

```bash
go run -C ../shared/lint/go . -check -dir backend   # .golangci.yml still matches the shared template
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run ./...
go build ./...
go vet ./...
go test ./...          # CI adds -race and a 63% coverage floor; -race locally needs gcc on PATH
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

(CI also checks the committed API types (`gen:check`) and runs `npm audit`, and checks the plugin, the scripts and
the docs; see [`.github/workflows/ci.yml`](.github/workflows/ci.yml), which calls
shared-tooling's reusable workflows.)

> **One command:** run [`scripts/check.ps1`](scripts/check.ps1) (PowerShell; or
> `task check`) to run every gate above - backend golangci config/`golangci-lint`/
> build/vet/test/govulncheck, then frontend `gen:check`/lint/format/conventions/
> typecheck/tests with coverage/`npm audit`/build, then the plugin `dotnet`
> build (warnings as errors - the shared analyzers and code style run in it, so it
> is the C# lint gate) and `dotnet format`, then PSScriptAnalyzer on every tracked
> script, markdownlint, and the shared repo files and version - and stop at the
> first failure. With the pre-push hook enabled
> (`git config core.hooksPath .githooks`), this runs automatically on `git push` and
> aborts it on any failure - bypass a single push with `git push --no-verify`.
> `golangci-lint` is **not** covered
> by `go build`/`go vet`, so don't skip it; `check.ps1` builds the pinned v2.15.0
> from source with `go run`, so there is nothing to install. PSScriptAnalyzer is
> 1.25.0, with the shared settings; install it once with
> `Install-PSResource PSScriptAnalyzer -Version 1.25.0 -Scope CurrentUser -TrustRepository`
> (without it the scripts step fails and says how; without `dotnet` the plugin
> steps are skipped with a message). The
> race detector runs in CI only - it needs gcc for its runtime; see the note in
> `check.ps1`. (`scripts/check.ps1`
> and `scripts/deploy.ps1` are version-controlled; all deploy server settings
> (host, user, key, webroot, service, opt dir, site address) live in an untracked
> `scripts/deploy.config.ps1` - copy `deploy.config.example.ps1`.)

## Conventions

- **Match the surrounding code.** Mirror the existing naming, comment density,
  and idioms of the file you're editing rather than introducing a new style.
- **Tests live next to the code.** Frontend: colocated `src/**/*.test.ts`
  (Vitest + Vue Test Utils, jsdom). Backend: `_test.go` in the same package. Add
  or update tests with every behaviour change. See the **Testing** section of
  [`AGENTS.md`](AGENTS.md) for conventions.
- **Types flow from Go.** Backend model changes must be followed by
  `npm run gen:types`; don't hand-edit `api.generated.ts`.
- **Keep the API reference current.** Response shapes are **typed structs** in
  `internal/model` (never `map[string]any`), which feed both the frontend types
  (`npm run gen:types`) and the OpenAPI schemas. Adding/changing an endpoint means
  updating the paths table in `internal/apidoc/` and running
  `go run ./cmd/openapi-gen` to refresh [`backend/openapi.yaml`](backend/openapi.yaml).
  CI (`internal/apidoc` tests) fails if the spec is stale or a route is undocumented.
- **Theming is structured tokens**, not free-form CSS - edit the token list in
  the three places that must stay in sync (`frontend/src/assets/styles/tokens.css`,
  `store.themeTokenOrder` in Go, `THEME_TOKEN_GROUPS` in `lib/theme-tokens.ts`).

## Versioning & changelog

The frontend, backend, and plugin are versioned **independently** with semver
(MAJOR = breaking API change, MINOR = backward-compatible addition, PATCH = fix).
When your change ships:

1. Bump the matching version source - `frontend/package.json` `"version"`,
   `backend/internal/version/version.go` (`Version`), and/or the plugin's
   `SenpanCompanion.csproj` `<Version>`.
2. Add an entry under the matching section (Frontend / Backend / Plugin) of
   [`CHANGELOG.md`](CHANGELOG.md), newest first
   (*Added / Changed / Fixed / Removed / Security*).

The admin dashboard shows the frontend + backend versions and flags a
major-version mismatch, so keeping them in step matters.

**Releases are automatic.** On a push to `main`, once the CI gate passes, the
`release` job tags each component whose version has no release yet
(`<Component>-v<version>`) and publishes a GitHub Release using its CHANGELOG
section as the body. So bump the version **and** update its CHANGELOG section in
the same commit - that's what becomes the release notes. No manual `git tag`.

## Deploying

Deployment is maintainer-only via `scripts/deploy.ps1`
(`-Target frontend|backend|main|all|plugin`; `both` is a kept alias for `main`).
The script is version-controlled and built on shared-tooling's SharedDeploy module;
every environment-specific setting (host, SSH user, key, webroot, service name, opt
dir) lives in an untracked
`scripts/deploy.config.ps1` (copy `deploy.config.example.ps1`) - nothing in the
tracked script reveals the server layout. See [`deploy/README.md`](deploy/README.md).
Database changes on production (outside the app's own migrations) are applied
manually and require explicit sign-off - never include them in a routine deploy.
