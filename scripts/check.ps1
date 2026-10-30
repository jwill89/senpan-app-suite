#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Run the full local quality gate (the same checks CI runs) before pushing.

.DESCRIPTION
    Backend (backend/):   golangci-lint - go build - go vet - go test - govulncheck
    Frontend (frontend/): gen:types - lint:check (ESLint + Stylelint) - format:check - lint:conventions - typecheck - test - npm audit - build
    Plugin (plugins/SenpanCompanion/): dotnet build (warnings=errors) - dotnet format - roslynator analyze
    Scripts (scripts/*.ps1): PSScriptAnalyzer (PSScriptAnalyzerSettings.psd1)

    Stops at the first failing step with a clear message, so `golangci-lint`
    (which is NOT part of `go build`/`go vet`) can't be forgotten and slip
    through to CI. Mirrors .github/workflows/ci.yml. golangci-lint is built from
    source at the pinned version with `go run`, the way CI installs it, so there
    is nothing to install and local and CI results cannot drift apart.

    The plugin (.NET) steps no-op with a message if the `dotnet` SDK isn't
    installed locally, and the scripts step does the same without PSScriptAnalyzer
    1.25.0, so the script still runs on a machine without them.

    Note: `go test -race` runs in CI, not here. The race detector needs cgo for
    its runtime and a C toolchain to build it, which not every Windows machine
    has; the SQLite driver itself is pure Go, so the race build exercises the same
    driver production ships. With gcc on PATH, run it by hand from backend/:
    `go test -race ./...` (about four minutes).

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
    Push-Location "$root\backend"
    try {
        # The version pinned in backend/.golangci.yml and ci.yml.
        Invoke-Step "backend: golangci-lint" {
            go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run ./...
        }
        Invoke-Step "backend: go build" { go build ./... }
        Invoke-Step "backend: go vet" { go vet ./... }
        Invoke-Step "backend: go test" { go test ./... }
        # Scan the module + dependencies for known Go vulns that reach our code.
        # Run latest (as CI does) so newly disclosed advisories are caught.
        Invoke-Step "backend: govulncheck" { go run golang.org/x/vuln/cmd/govulncheck@latest ./... }
    } finally { Pop-Location }
}

if (-not $SkipFrontend) {
    Push-Location "$root\frontend"
    try {
        # api.generated.ts is gitignored; regenerate so typecheck/build are current.
        Invoke-Step "frontend: gen:types" { npm run gen:types }
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
        Invoke-Step "frontend: test" { npm run test }
        # Known vulnerabilities in what ships (dev tooling excluded), at high or above.
        Invoke-Step "frontend: npm audit" { npm audit --audit-level=high --omit=dev }
        Invoke-Step "frontend: build" { npm run build }
    } finally { Pop-Location }
}

# Plugin (FFXIV Dalamud) - the third CI job. Only runs on a full check (neither
# -SkipFrontend nor -SkipBackend), so those flags keep their "only X" meaning.
# Requires the .NET SDK; no-ops with a clear message when `dotnet` isn't installed
# so this script still runs on a machine without it.
if (-not $SkipFrontend -and -not $SkipBackend) {
    if (-not (Get-Command dotnet -ErrorAction SilentlyContinue)) {
        Write-Host "`n==> plugin: dotnet SDK not found on PATH - skipping plugin build/format checks." -ForegroundColor Yellow
    } else {
        Push-Location "$root\plugins\SenpanCompanion"
        try {
            # Promote warnings to errors so the plugin's 0-warning rule is enforced.
            Invoke-Step "plugin: dotnet build (warnings = errors)" {
                dotnet build .\SenpanCompanion.csproj -c Release -p:TreatWarningsAsErrors=true
            }
            # Whitespace/style gate; rules pinned in .editorconfig.
            Invoke-Step "plugin: dotnet format" { dotnet format --verify-no-changes }
            # C# lint gate (the analog to golangci-lint / eslint): the Roslynator CLI,
            # pinned in .config/dotnet-tools.json, fails on any analyzer diagnostic at
            # info or above. The ruleset is curated in the plugin's .editorconfig.
            Invoke-Step "plugin: roslynator restore" { dotnet tool restore }
            Invoke-Step "plugin: roslynator analyze" {
                dotnet roslynator analyze .\SenpanCompanion.csproj --severity-level info
            }
        } finally { Pop-Location }
    }
}

# Scripts (scripts/*.ps1) - PSScriptAnalyzer with the repo-root PSScriptAnalyzerSettings.psd1,
# as CI runs it. Like the plugin, only on a full check. Only TRACKED scripts are linted:
# scripts/deploy.config.ps1 is the owner's local copy. Requires PSScriptAnalyzer 1.25.0
# (the version CI pins); skips with a clear message when it isn't installed.
if (-not $SkipFrontend -and -not $SkipBackend) {
    $analyzerVersion = '1.25.0'
    if (-not (Get-Module -ListAvailable PSScriptAnalyzer | Where-Object Version -EQ $analyzerVersion)) {
        Write-Host "`n==> scripts: PSScriptAnalyzer $analyzerVersion not installed - skipping script lint. Install it with:" -ForegroundColor Yellow
        Write-Host "      Install-PSResource PSScriptAnalyzer -Version $analyzerVersion -Scope CurrentUser -TrustRepository" -ForegroundColor Yellow
    } else {
        Write-Host "`n==> scripts: PSScriptAnalyzer" -ForegroundColor Cyan
        Import-Module PSScriptAnalyzer -RequiredVersion $analyzerVersion
        $settings = Join-Path $root 'PSScriptAnalyzerSettings.psd1'
        $findings = foreach ($file in (git -C $root ls-files 'scripts/*.ps1')) {
            Invoke-ScriptAnalyzer -Path (Join-Path $root $file) -Settings $settings
        }
        if ($findings) {
            $findings | Format-Table -AutoSize | Out-String | Write-Host
            Write-Host "`nFAILED: scripts: PSScriptAnalyzer ($(@($findings).Count) finding(s))" -ForegroundColor Red
            exit 1
        }
    }
}

Write-Host "`n[OK] All checks passed." -ForegroundColor Green
