#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Run every check CI runs, locally, when you want the answer before CI gives it.

.DESCRIPTION
    Backend (backend/):   golangci-lint - go build - go vet - go test - govulncheck
    Frontend (frontend/): gen:check - lint:check (ESLint + Stylelint) - format:check - lint:conventions - typecheck - test:coverage - npm audit - build - test:e2e
    Plugin (plugins/SenpanCompanion/): dotnet build (warnings=errors) - dotnet format
    Scripts and repo: PSScriptAnalyzer on every tracked script, markdownlint, the shared
    .editorconfig/.gitattributes and the shared-tooling version (shared-tooling's own checks)

    The settings come from shared-tooling, the submodule at shared/ (run
    `git submodule update --init` once after cloning).

    Stops at the first failing step with a clear message, so `golangci-lint`
    (which is NOT part of `go build`/`go vet`) can't be forgotten and slip
    through to CI. Mirrors .github/workflows/ci.yml. golangci-lint is built from
    source at the pinned version with `go run`, the way CI installs it, so there
    is nothing to install and local and CI results cannot drift apart.

    The plugin (.NET) steps no-op with a message if the `dotnet` SDK isn't
    installed locally. The scripts step fails without PSScriptAnalyzer; install it once
    with `Install-PSResource PSScriptAnalyzer -Version 1.25.0 -Scope CurrentUser
    -TrustRepository`.

    Note: `go test -race` runs in CI, not here. The race detector needs cgo for
    its runtime and a C toolchain to build it, which not every Windows machine
    has; the SQLite driver itself is pure Go, so the race build exercises the same
    driver production ships. With gcc on PATH, run it by hand from backend/:
    `go test -race ./...` (about four minutes).

.PARAMETER SkipFrontend
    Run only the backend checks.

.PARAMETER SkipBackend
    Run only the frontend checks.

.PARAMETER Only
    Run only the named parts: backend, frontend, plugin, repo (the scripts, docs and
    shared-file checks). `task check` runs each part in its own process, all at once; a
    -Skip flag still leaves its half out, and the plugin and repo parts run only when
    neither half is skipped.

.EXAMPLE
    .\scripts\check.ps1
.EXAMPLE
    .\scripts\check.ps1 -SkipFrontend
.EXAMPLE
    .\scripts\check.ps1 -Only plugin
#>
[CmdletBinding()]
param(
    [switch]$SkipFrontend,
    [switch]$SkipBackend,
    [ValidateSet('backend', 'frontend', 'plugin', 'repo')]
    [string[]]$Only
)

$ErrorActionPreference = 'Stop'
# Don't let a non-zero native exit auto-throw before our own $LASTEXITCODE check.
$PSNativeCommandUseErrorActionPreference = $false
trap { Write-Host "`nERROR: $($_.Exception.Message)" -ForegroundColor Red; exit 1 }

$root = Split-Path $PSScriptRoot -Parent

# Which parts run: each one -Only names (all when it names none), less what a -Skip flag
# leaves out. The plugin and repo parts belong to a full check, so either -Skip drops them.
$full = -not $SkipFrontend -and -not $SkipBackend
$run = @{
    backend  = -not $SkipBackend
    frontend = -not $SkipFrontend
    plugin   = $full
    repo     = $full
}
if ($Only) {
    foreach ($part in @($run.Keys)) { $run[$part] = $run[$part] -and $Only -contains $part }
}

function Invoke-Step($name, [scriptblock]$body) {
    Write-Host "`n==> $name" -ForegroundColor Cyan
    & $body
    if ($LASTEXITCODE -ne 0) {
        Write-Host "`nFAILED: $name (exit $LASTEXITCODE)" -ForegroundColor Red
        exit 1
    }
}

if ($run.backend) {
    Push-Location "$root\backend"
    try {
        # .golangci.yml is written from shared-tooling's template (Senpan's own rules are in
        # .golangci.local.yml), so a hand edit there, or a template change not yet taken, fails.
        # (-dir is the module's folder from the repo root, which the tool finds itself.)
        Invoke-Step "backend: golangci config" { go run -C ../shared/lint/go . -check -dir backend }
        # The version shared-tooling's go.yml builds in CI.
        Invoke-Step "backend: golangci-lint" {
            go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run ./...
        }
        Invoke-Step "backend: go build" { go build ./... }
        Invoke-Step "backend: go vet" { go vet ./... }
        Invoke-Step "backend: go test" { go test ./... }
        # Known Go vulnerabilities that reach the code, at the release go.yml runs.
        Invoke-Step "backend: govulncheck" { go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./... }
    } finally { Pop-Location }
}

if ($run.frontend) {
    Push-Location "$root\frontend"
    try {
        # api.generated.ts is committed; this regenerates it from the Go models and fails
        # if the committed copy is stale (CI's generated job does the same).
        Invoke-Step "frontend: gen:check" { npm run gen:check }
        # ESLint (eslint.config.js) for the scripts and templates, Stylelint
        # (stylelint.config.mjs) for the stylesheets and <style> blocks.
        Invoke-Step "frontend: lint" { npm run lint:check }
        # Prettier covers what ESLint doesn't: the CSS layer. Formatting used to be
        # hand-maintained there because themes were authored as raw CSS in a code
        # editor; themes are token-only now, so the stylesheets are ordinary source
        # and get the same enforced formatting as everything else.
        Invoke-Step "frontend: format" { npm run format:check }
        # Naming rules Prettier can't see, plus the theme-token list agreeing across
        # tokens.css / theme-tokens.ts / styles.go. See AGENTS.md "CSS conventions".
        Invoke-Step "frontend: conventions" { npm run lint:conventions }
        Invoke-Step "frontend: typecheck" { npm run typecheck }
        # With coverage, so the floor in vitest.config.ts fails here as in CI.
        Invoke-Step "frontend: test" { npm run test:coverage }
        # Known vulnerabilities in what ships (dev tooling excluded), at high or above.
        Invoke-Step "frontend: npm audit" { npm audit --audit-level=high --omit=dev }
        Invoke-Step "frontend: build" { npm run build }
        # The API and the built SPA on a throwaway database, never devdata/ (e2e/server/prepare.ts).
        Invoke-Step "frontend: end-to-end" { npm run test:e2e }
    } finally { Pop-Location }
}

# Plugin (FFXIV Dalamud) - the third CI job. Only runs on a full check (neither
# -SkipFrontend nor -SkipBackend), so those flags keep their "only X" meaning.
# Requires the .NET SDK; no-ops with a clear message when `dotnet` isn't installed
# so this script still runs on a machine without it.
if ($run.plugin) {
    if (-not (Get-Command dotnet -ErrorAction SilentlyContinue)) {
        Write-Host "`n==> plugin: dotnet SDK not found on PATH - skipping plugin build/format checks." -ForegroundColor Yellow
    } else {
        Push-Location "$root\plugins\SenpanCompanion"
        try {
            # Promote warnings to errors so the plugin's 0-warning rule is enforced.
            Invoke-Step "plugin: dotnet build (warnings = errors)" {
                dotnet build .\SenpanCompanion.csproj -c Release -p:TreatWarningsAsErrors=true
            }
            # The analyzers and code style are shared-tooling's (plugins/Directory.Build.props
            # imports them), so the build above is the lint gate; this checks the formatting.
            Invoke-Step "plugin: dotnet format" { dotnet format --verify-no-changes }
        } finally { Pop-Location }
    }
}

# Scripts and the repo - shared-tooling's own checks, as CI's pwsh and docs jobs run them.
# Like the plugin, only on a full check. PSScriptAnalyzer runs on every TRACKED script
# (scripts/deploy.config.ps1 is the owner's local copy) with the shared settings; the
# script says how to install the pinned PSScriptAnalyzer if it is missing.
if ($run.repo) {
    $shared = Join-Path $root 'shared'
    if (-not (Test-Path -LiteralPath (Join-Path $shared 'VERSION'))) {
        throw 'shared/ is empty: run `git submodule update --init` once after cloning.'
    }
    Push-Location $root
    try {
        Invoke-Step "scripts: PSScriptAnalyzer" { pwsh -NoProfile -File (Join-Path $shared 'lint/pwsh/Invoke-ScriptLint.ps1') }
        Invoke-Step "docs: markdownlint" { npx --yes markdownlint-cli2@0.24.0 }
        Invoke-Step "repo: .editorconfig and .gitattributes" { pwsh -NoProfile -File (Join-Path $shared 'repo/Sync-RepoFiles.ps1') -Path . -Check }
        Invoke-Step "repo: shared-tooling version" { pwsh -NoProfile -File (Join-Path $shared 'repo/Test-SharedVersion.ps1') }
    } finally { Pop-Location }
}

$ran = @('backend', 'frontend', 'plugin', 'repo' | Where-Object { $run[$_] })
if ($ran) {
    Write-Host "`n[OK] Checks passed: $($ran -join ', ')." -ForegroundColor Green
} else {
    Write-Host "`nNothing to check: -Only and the -Skip flags left out every part." -ForegroundColor Yellow
}
