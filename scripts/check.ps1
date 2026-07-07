#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Run the full local quality gate (the same checks CI runs) before pushing.

.DESCRIPTION
    Backend (backend/):   golangci-lint · go build · go vet · go test · govulncheck
    Frontend (frontend/): gen:types · lint:check · typecheck · test · build
    Plugin (plugins/SenpanCompanion/): dotnet build (warnings=errors) · dotnet format

    Stops at the first failing step with a clear message, so `golangci-lint`
    (which is NOT part of `go build`/`go vet`) can't be forgotten and slip
    through to CI. Mirrors .github/workflows/ci.yml.

    The plugin (.NET) steps no-op with a message if the `dotnet` SDK isn't
    installed locally, so the script still runs on a machine without it.

    Note: `go test -race` is intentionally NOT run. The production binary builds
    CGO_ENABLED=0 with the pure-Go SQLite driver, and the race detector needs
    cgo; a cgo-linked driver just for the race run would exercise DIFFERENT SQL/
    locking semantics than production ships (false confidence). Concurrency
    correctness instead rests on the game service's opMu serialization, the v43
    UNIQUE(game_id, number) constraint, the ws hub's lock discipline, and the
    manifest/settings mutexes — all covered by targeted tests under this gate.

.PARAMETER SkipFrontend
    Run only the backend checks.

.PARAMETER SkipBackend
    Run only the frontend checks.

.EXAMPLE
    .\scripts\check.ps1
.EXAMPLE
    .\scripts\check.ps1 -SkipFrontend
#>
[CmdletBinding()]
param(
    [switch]$SkipFrontend,
    [switch]$SkipBackend
)

$ErrorActionPreference = 'Stop'
# Don't let a non-zero native exit auto-throw before our own $LASTEXITCODE check.
$PSNativeCommandUseErrorActionPreference = $false
trap { Write-Host "`nERROR: $($_.Exception.Message)" -ForegroundColor Red; exit 1 }

$root = Split-Path $PSScriptRoot -Parent

function Invoke-Step($name, [scriptblock]$body) {
    Write-Host "`n==> $name" -ForegroundColor Cyan
    & $body
    if ($LASTEXITCODE -ne 0) {
        Write-Host "`nFAILED: $name (exit $LASTEXITCODE)" -ForegroundColor Red
        exit 1
    }
}

if (-not $SkipBackend) {
    if (-not (Get-Command golangci-lint -ErrorAction SilentlyContinue)) {
        Write-Host "golangci-lint not found on PATH. Install the pinned version:" -ForegroundColor Red
        Write-Host "  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2" -ForegroundColor Yellow
        exit 1
    }
    Push-Location "$root\backend"
    try {
        Invoke-Step "backend: golangci-lint" { golangci-lint run ./... }
        Invoke-Step "backend: go build" { go build ./... }
        Invoke-Step "backend: go vet" { go vet ./... }
        Invoke-Step "backend: go test" { go test ./... }
        # Scan the module + dependencies for known Go vulns that reach our code.
        # Run latest (as CI does) so newly disclosed advisories are caught.
        Invoke-Step "backend: govulncheck" { go run golang.org/x/vuln/cmd/govulncheck@latest ./... }
    }
    finally { Pop-Location }
}

if (-not $SkipFrontend) {
    Push-Location "$root\frontend"
    try {
        # api.generated.ts is gitignored; regenerate so typecheck/build are current.
        Invoke-Step "frontend: gen:types" { npm run gen:types }
        Invoke-Step "frontend: lint" { npm run lint:check }
        Invoke-Step "frontend: typecheck" { npm run typecheck }
        Invoke-Step "frontend: test" { npm run test }
        Invoke-Step "frontend: build" { npm run build }
    }
    finally { Pop-Location }
}

# Plugin (FFXIV Dalamud) — the third CI job. Only runs on a full check (neither
# -SkipFrontend nor -SkipBackend), so those flags keep their "only X" meaning.
# Requires the .NET SDK; no-ops with a clear message when `dotnet` isn't installed
# so this script still runs on a machine without it.
if (-not $SkipFrontend -and -not $SkipBackend) {
    if (-not (Get-Command dotnet -ErrorAction SilentlyContinue)) {
        Write-Host "`n==> plugin: dotnet SDK not found on PATH — skipping plugin build/format checks." -ForegroundColor Yellow
    }
    else {
        Push-Location "$root\plugins\SenpanCompanion"
        try {
            # Promote warnings to errors so the plugin's 0-warning rule is enforced.
            Invoke-Step "plugin: dotnet build (warnings = errors)" {
                dotnet build .\SenpanCompanion.csproj -c Release -p:TreatWarningsAsErrors=true
            }
            # Whitespace/style gate; rules pinned in .editorconfig.
            Invoke-Step "plugin: dotnet format" { dotnet format --verify-no-changes }
        }
        finally { Pop-Location }
    }
}

Write-Host "`n[OK] All checks passed." -ForegroundColor Green
