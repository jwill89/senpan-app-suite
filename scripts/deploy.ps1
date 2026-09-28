#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Build and deploy the Senpan App Suite frontend and/or backend to the host.

.DESCRIPTION
    -Target frontend (default):
        1. Builds frontend/ (vue-tsc + vite) -> frontend/dist.
        2. Packs dist/ into ONE .tar.gz and uploads that to <WebRoot>/dist.new via
           pscp (one stream; elapsed-time indicator, not per-file output). SCP
           negotiates each file separately, so uploading ~180 loose files spends
           most of its wall clock on round trips rather than payload; one archive
           also halves the bytes and fails cleanly - a truncated archive refuses to
           extract, where a half-finished directory copy can look plausible.
        3. Extracts it on the host, verifies every file arrived, then swaps it in:
           live "dist" -> "dist.old" (rollback backup, overwritten each deploy),
           new build -> "dist". Extract/verify/swap share one connection and one
           `set -e`, so a bad archive never reaches the swap and the live site keeps
           serving the previous build.
        4. Syncs deploy/.htaccess -> <WebRoot>/.htaccess so Apache header/caching/
           rewrite changes ship with the frontend (non-fatal if it fails). The repo
           copy is the source of truth - edit it there, not on the host.

    -Target backend:
        1. Cross-compiles a static linux/amd64 binary:
               GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o app-suite .
        2. Stops <ServiceName>.service, backs up the current binary AND snapshots
           the live DB to <RemoteDbPath>.pre-<timestamp> (the newest three are
           kept). Starting the new binary migrates the database irreversibly, and
           the step-4 binary rollback cannot undo that - this snapshot is the way
           back.
        3. Uploads the new binary to <RemoteOptDir>/app-suite (via a .new temp).
        4. Installs it and starts the service, rolling back to the previous binary
           if the new one fails to stay active.
        5. Refreshes the local dev DB from live (unless -NoDbPull): stops the service
           so no connection is mid-write, snapshots the just-migrated DB on the host,
           restarts the service immediately, then downloads the snapshot to
           devdata/database.sqlite (backing up the previous dev DB). This gives local
           dev fresh live data on the current schema; the service is down only for a
           local file copy, not the network transfer.

    -Target main (alias: both): backend, then frontend (with a short pause between
    to stay under the SSH rate limit). The API leads because a new backend is
    additive to the old SPA, while a new SPA calls endpoints an old backend does
    not have yet.

    -Target all: backend, then frontend, then plugin (paused between each).

    Uses PuTTY's pscp/plink so the DigitalOcean .ppk key works directly. pscp/plink
    run in batch mode, so a passphrase-protected .ppk must be loaded into Pageant
    first:  pageant.exe "<KeyPath>"  (enter the passphrase once per Windows session).
    The frontend target also needs `tar`, which Windows has shipped since 10 1803
    (System32/tar.exe); the host needs it too, to extract.

    Each target uses at most FOUR SSH connections and does not poll during transfers,
    to stay under `ufw limit ssh` (which drops the IP after 6 connections in 30s).
    A connection that gets rate-limited is retried once after a 35s wait. The
    post-backend dev-DB refresh is a separate connection group (snapshot, download,
    cleanup); the dispatcher inserts the standard 35s pause before it to reset the
    rate-limit window.

.PARAMETER Target
    What to deploy: frontend (default), backend, main (backend + frontend; 'both'
    is a kept alias), all (backend + frontend + plugin), or plugin (build + publish
    the Dalamud custom-repo files: SenpanCompanionAdmin/latest.zip, pluginmaster.json
    with a ?v=<version> cache-bust on its download links, and plugin.htaccess).

.PARAMETER VpsHost
    Droplet IP or hostname. If omitted, resolved from $env:DEPLOY_VPS_HOST, then
    the untracked scripts/deploy.config.ps1 ($DeployVpsHost).

.PARAMETER VpsUser
    SSH user. If omitted, resolved from $env:DEPLOY_VPS_USER, then
    scripts/deploy.config.ps1 ($DeployVpsUser).

.PARAMETER KeyPath
    Path to the PuTTY .ppk private key. If omitted, resolved from
    $env:DEPLOY_KEY_PATH, then scripts/deploy.config.ps1 ($DeployKeyPath).

.PARAMETER WebRoot
    Apache DocumentRoot on the host (holds "dist" + "plugin"; frontend + plugin).
    If omitted, resolved from $env:DEPLOY_WEB_ROOT, then scripts/deploy.config.ps1
    ($DeployWebRoot).

.PARAMETER ServiceName
    systemd service to restart for the backend. If omitted, resolved from
    $env:DEPLOY_SERVICE_NAME, then scripts/deploy.config.ps1 ($DeployServiceName).

.PARAMETER RemoteOptDir
    Directory on the host holding the backend binary. If omitted, resolved from
    $env:DEPLOY_REMOTE_OPT_DIR, then scripts/deploy.config.ps1 ($DeployRemoteOptDir).

.PARAMETER RemoteDbPath
    Full path to the live SQLite DB on the host, used by the post-backend dev-DB
    refresh. If omitted, resolved from $env:DEPLOY_REMOTE_DB_PATH, then
    scripts/deploy.config.ps1 ($DeployRemoteDbPath), then defaults to
    "<RemoteOptDir>/data/database.sqlite" (matches deploy/senpan.service).

.PARAMETER SkipBuild
    Deploy the existing build artifact(s) without rebuilding.

.PARAMETER NoBackup
    Remove the rollback backup (dist.old / app-suite.old) after a successful deploy.

.PARAMETER NoDbPull
    Skip refreshing the local dev DB from live after a backend deploy. By default a
    backend deploy snapshots the just-migrated live DB and pulls it into
    devdata/database.sqlite (backing up the previous dev DB first).

.EXAMPLE
    .\scripts\deploy.ps1                 # frontend (default)

.EXAMPLE
    .\scripts\deploy.ps1 -Target backend

.EXAMPLE
    .\scripts\deploy.ps1 -Target backend -NoDbPull   # deploy backend, don't refresh dev DB

.EXAMPLE
    .\scripts\deploy.ps1 -Target both

.EXAMPLE
    .\scripts\deploy.ps1 -Target plugin   # build + publish the Dalamud plugin repo
#>
[CmdletBinding()]
param(
    [ValidateSet('frontend', 'backend', 'both', 'main', 'all', 'plugin')]
    [string]$Target = 'frontend',

    # Server-specific settings are NOT baked into this (tracked) script - nothing
    # here reveals the host, SSH user, filesystem paths, or service name. Each is
    # resolved below from its -param, then the matching $env:DEPLOY_* variable (the
    # param default), then the untracked scripts/deploy.config.ps1. See
    # scripts/deploy.config.example.ps1.
    [string]$VpsHost = $env:DEPLOY_VPS_HOST,

    [string]$VpsUser = $env:DEPLOY_VPS_USER,

    [string]$KeyPath = $env:DEPLOY_KEY_PATH,

    [string]$WebRoot = $env:DEPLOY_WEB_ROOT,

    [string]$ServiceName = $env:DEPLOY_SERVICE_NAME,

    [string]$RemoteOptDir = $env:DEPLOY_REMOTE_OPT_DIR,

    # Full path to the live SQLite DB on the host. Optional; defaults to
    # "<RemoteOptDir>/data/database.sqlite" (matches deploy/senpan.service). Resolved
    # from -RemoteDbPath, then $env:DEPLOY_REMOTE_DB_PATH, then $DeployRemoteDbPath,
    # then the derived default. Only used by the post-backend dev-DB refresh.
    [string]$RemoteDbPath = $env:DEPLOY_REMOTE_DB_PATH,

    [switch]$SkipBuild,

    [switch]$NoBackup,

    # Skip refreshing the local dev DB (devdata/database.sqlite) from live after a
    # backend deploy. By default a backend deploy snapshots the just-migrated live DB
    # and pulls it down so local dev has fresh data on the current schema.
    [switch]$NoDbPull
)

$ErrorActionPreference = 'Stop'
# PowerShell 7.4+ makes a non-zero *native* exit throw under ErrorActionPreference
# 'Stop', which would bypass our own $LASTEXITCODE checks and can exit silently if
# the window closes. Turn it off so our checks/messages always run. (No-op on 5.1.)
$PSNativeCommandUseErrorActionPreference = $false

function Write-Step($msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
function Write-Ok($msg) { Write-Host "    $msg" -ForegroundColor Green }
function Fail($msg) { Write-Host "`nERROR: $msg" -ForegroundColor Red; exit 1 }

# Catch-all: show any unexpected terminating error instead of exiting silently.
trap { Write-Host "`nERROR: $($_.Exception.Message)" -ForegroundColor Red; exit 1 }

# -- Server settings resolution ------------------------------------------------
# Every environment-specific value (host, SSH user, key, webroot, service name,
# opt dir) lives OUTSIDE this tracked script. Resolution order for each: its
# -param, then the matching $env:DEPLOY_* variable (already applied as the param
# default), then the untracked scripts/deploy.config.ps1. Copy
# scripts/deploy.config.example.ps1 to deploy.config.ps1 and fill it in once.
$configPath = Join-Path $PSScriptRoot 'deploy.config.ps1'
if (Test-Path $configPath) { . $configPath }
if (-not $VpsHost) { $VpsHost = $DeployVpsHost }
if (-not $VpsUser) { $VpsUser = $DeployVpsUser }
if (-not $KeyPath) { $KeyPath = $DeployKeyPath }
if (-not $WebRoot) { $WebRoot = $DeployWebRoot }
if (-not $ServiceName) { $ServiceName = $DeployServiceName }
if (-not $RemoteOptDir) { $RemoteOptDir = $DeployRemoteOptDir }

$missing = @()
if (-not $VpsHost) { $missing += 'VpsHost (-VpsHost / $env:DEPLOY_VPS_HOST / $DeployVpsHost)' }
if (-not $VpsUser) { $missing += 'VpsUser (-VpsUser / $env:DEPLOY_VPS_USER / $DeployVpsUser)' }
if (-not $KeyPath) { $missing += 'KeyPath (-KeyPath / $env:DEPLOY_KEY_PATH / $DeployKeyPath)' }
if (-not $WebRoot) { $missing += 'WebRoot (-WebRoot / $env:DEPLOY_WEB_ROOT / $DeployWebRoot)' }
if (-not $ServiceName) { $missing += 'ServiceName (-ServiceName / $env:DEPLOY_SERVICE_NAME / $DeployServiceName)' }
if (-not $RemoteOptDir) { $missing += 'RemoteOptDir (-RemoteOptDir / $env:DEPLOY_REMOTE_OPT_DIR / $DeployRemoteOptDir)' }
if ($missing.Count -gt 0) {
    Fail "Missing deploy settings:`n  - $($missing -join "`n  - ")`nSet these via -params, `$env:DEPLOY_* variables, or scripts/deploy.config.ps1 (copy scripts/deploy.config.example.ps1)."
}

# The live DB path is optional (not in the required set): fall back to the config
# value, then a default derived from the resolved opt dir (see deploy/senpan.service:
# ExecStart runs -db <RemoteOptDir>/data/database.sqlite). Only the dev-DB refresh
# after a backend deploy uses it.
if (-not $RemoteDbPath) { $RemoteDbPath = $DeployRemoteDbPath }
if (-not $RemoteDbPath) { $RemoteDbPath = "$RemoteOptDir/data/database.sqlite" }

# Restore an env var to a prior value ($null => remove it).
function Restore-Env([string]$name, $value) {
    if ($null -eq $value) { Remove-Item "Env:$name" -ErrorAction SilentlyContinue }
    else { Set-Item "Env:$name" $value }
}

# Run a remote command via plink. Retry once (after the 30s ufw window) ONLY when
# the failure looks like a dropped/refused connection - not a genuine command
# error (so a real failure isn't misreported as a rate-limit and re-run).
function Invoke-RemoteWithRetry([string]$cmd) {
    $out = & $plink -batch -i $KeyPath $remoteTarget $cmd 2>&1
    foreach ($line in $out) { if ("$line".Trim()) { Write-Host "    $line" } }
    if ($LASTEXITCODE -eq 0) { return $true }
    $text = $out | Out-String
    if ($text -notmatch 'Network error|Connection (timed out|refused|reset|abandoned)|Unable to (open connection|authenticate)|Server unexpectedly closed') {
        return $false   # genuine command failure - do not retry
    }
    Write-Host "    Connection failed - likely the SSH rate limit (ufw limit ssh). Waiting 35s, then retrying once..." -ForegroundColor Yellow
    Start-Sleep -Seconds 35
    & $plink -batch -i $KeyPath $remoteTarget $cmd
    return ($LASTEXITCODE -eq 0)
}

# Upload a single local file to a remote path (the backend binary). pscp shows its
# own single-file progress. Retried once on failure (usually a rate-limited link).
function Send-File([string]$localPath, [string]$remotePath) {
    & $pscp -batch -C -i $KeyPath "$localPath" "${remoteTarget}:$remotePath"
    if ($LASTEXITCODE -eq 0) { return $true }
    Write-Host "    Upload failed - possibly the SSH rate limit. Waiting 35s, then retrying once..." -ForegroundColor Yellow
    Start-Sleep -Seconds 35
    & $pscp -batch -C -i $KeyPath "$localPath" "${remoteTarget}:$remotePath"
    return ($LASTEXITCODE -eq 0)
}

# Upload a file and only then move it into place, refusing to publish a torn copy.
#
# Send-File writes straight onto the live path, which is fine for the backend
# binary (its caller stages to a .new name and swaps separately) but not for the
# files served directly to users: a truncated latest.zip is a broken plugin
# download, a truncated pluginmaster.json breaks the Dalamud repo listing, and a
# truncated .htaccess changes how the site is served. Upload beside the target,
# check the SHA-256 the host computes against the one we computed locally, and mv
# only on a match - the mv itself is atomic within a filesystem, so readers see
# either the old file or the new one.
#
# The remote command is idempotent so a retry after a mid-command network drop
# does not report failure for work that already succeeded: if the staged file is
# gone the mv already ran, and the final check confirms what is actually in place.
function Send-FileVerified([string]$localPath, [string]$remotePath) {
    $staged = "$remotePath.new"
    if (-not (Send-File $localPath $staged)) { return $false }
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $localPath).Hash.ToLower()
    $cmd = @"
set -e
if [ -f '$staged' ]; then
  echo '$hash  $staged' | sha256sum -c - >/dev/null
  mv -f '$staged' '$remotePath'
fi
echo '$hash  $remotePath' | sha256sum -c - >/dev/null
"@
    if (Invoke-RemoteWithRetry $cmd) { return $true }
    # Leave the live file untouched and clear the staged copy.
    & $plink -batch -i $KeyPath $remoteTarget "rm -f '$staged'" 2>&1 | Out-Null
    return $false
}

# Download a single remote file to a local path (the live DB snapshot). Mirrors
# Send-File: one pscp stream, retried once on failure (usually a rate-limited link).
function Get-File([string]$remotePath, [string]$localPath) {
    & $pscp -batch -C -i $KeyPath "${remoteTarget}:$remotePath" "$localPath"
    if ($LASTEXITCODE -eq 0) { return $true }
    Write-Host "    Download failed - possibly the SSH rate limit. Waiting 35s, then retrying once..." -ForegroundColor Yellow
    Start-Sleep -Seconds 35
    & $pscp -batch -C -i $KeyPath "${remoteTarget}:$remotePath" "$localPath"
    return ($LASTEXITCODE -eq 0)
}

# One pscp upload of the frontend TARBALL -> the staging dir, with an elapsed-time
# indicator and no polling. Returns @{ Ok; Err; Elapsed }.
#
# A single archive rather than `pscp -r dist`: SCP negotiates each file separately,
# so a ~180-file bundle spends most of its wall clock on per-file round trips
# rather than on payload. One stream also fails cleanly - a truncated archive
# refuses to extract, where a half-finished directory copy can look plausible.
function Invoke-Upload([string]$ArchivePath, [string]$RemotePath) {
    $outFile = Join-Path $env:TEMP ("pscp-out-{0}.log" -f [guid]::NewGuid())
    $errFile = Join-Path $env:TEMP ("pscp-err-{0}.log" -f [guid]::NewGuid())
    $sizeKb = [math]::Round((Get-Item $ArchivePath).Length / 1KB)
    $proc = Start-Process -FilePath $pscp -NoNewWindow -PassThru -WorkingDirectory $FrontendDir `
        -RedirectStandardOutput $outFile -RedirectStandardError $errFile `
        -ArgumentList @('-batch', '-C', '-i', $KeyPath, $ArchivePath, "${remoteTarget}:$RemotePath")
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    $maxSeconds = 900
    $killed = $false
    while (-not $proc.HasExited) {
        Start-Sleep -Seconds 1
        Write-Progress -Activity "Uploading frontend" -Status ("{0} KB archive - {1:mm\:ss} elapsed" -f $sizeKb, $sw.Elapsed)
        if ($sw.Elapsed.TotalSeconds -gt $maxSeconds) { try { $proc.Kill() } catch {}; $killed = $true; break }
    }
    $proc.WaitForExit()
    $sw.Stop()
    Write-Progress -Activity "Uploading frontend" -Completed
    $code = $proc.ExitCode
    $err = if ($killed) { "Upload exceeded ${maxSeconds}s and was aborted." }
    elseif ($code -ne 0) { Get-Content $errFile -Raw -ErrorAction SilentlyContinue } else { '' }
    Remove-Item $outFile, $errFile -Force -ErrorAction SilentlyContinue
    return [pscustomobject]@{ Ok = ($code -eq 0); Err = $err; Elapsed = $sw.Elapsed }
}

# -- Resolve paths & tools -----------------------------------------------------
$RepoRoot = Split-Path $PSScriptRoot -Parent
$FrontendDir = Join-Path $RepoRoot "frontend"
$DistDir = Join-Path $FrontendDir "dist"
$HtaccessLocal = Join-Path $RepoRoot "deploy\.htaccess"
$BackendDir = Join-Path $RepoRoot "backend"
$LocalBinary = Join-Path $BackendDir "app-suite"
$DevDataDir = Join-Path $RepoRoot "devdata"
$DevDbPath = Join-Path $DevDataDir "database.sqlite"
$PluginDir = Join-Path $RepoRoot "plugins\SenpanCompanion"
$PluginZip = Join-Path $PluginDir "bin\Release\SenpanCompanionAdmin\latest.zip"
$PluginManifest = Join-Path $PluginDir "bin\Release\SenpanCompanionAdmin\SenpanCompanionAdmin.json"
$PluginMaster = Join-Path $RepoRoot "plugins\pluginmaster.json"
$PluginHtaccess = Join-Path $RepoRoot "deploy\plugin.htaccess"

if (-not (Test-Path $KeyPath)) { Fail "SSH key not found: $KeyPath" }

function Resolve-PuttyTool($name) {
    $cmd = Get-Command $name -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $candidate = Join-Path "C:\Program Files\PuTTY" "$name.exe"
    if (Test-Path $candidate) { return $candidate }
    Fail "$name not found. Install PuTTY (winget install PuTTY.PuTTY) or add it to PATH."
}
$pscp = Resolve-PuttyTool "pscp"
$plink = Resolve-PuttyTool "plink"

# bsdtar, shipped with Windows since 10 1803 (C:\Windows\System32\tar.exe). Used to
# pack frontend/dist into one archive for upload - see Invoke-Upload.
$tar = (Get-Command tar -ErrorAction SilentlyContinue)?.Source
if (-not $tar) { $tar = "$env:SystemRoot\System32\tar.exe" }
if (-not (Test-Path $tar)) {
    Fail "tar not found (expected $env:SystemRoot\System32\tar.exe, shipped with Windows 10 1803+). The frontend deploy packs dist/ into one archive before uploading."
}

$remoteTarget = "$VpsUser@$VpsHost"
$remoteLive = "$WebRoot/dist"
$remoteNew = "$WebRoot/dist.new"
$remoteArchive = "$remoteNew/dist.tar.gz"
$remoteOld = "$WebRoot/dist.old"
$remotePluginDir = "$WebRoot/plugin"
$remotePluginZipDir = "$remotePluginDir/SenpanCompanionAdmin"

# == Frontend ==================================================================
function Deploy-Frontend {
    # 1. Build
    if ($SkipBuild) {
        Write-Step "Skipping frontend build (-SkipBuild); using existing frontend/dist"
    }
    else {
        Write-Step "Building frontend (vue-tsc + vite)..."
        Push-Location $FrontendDir
        try {
            npm run build
            if ($LASTEXITCODE -ne 0) { Fail "Frontend build failed (exit $LASTEXITCODE). Live site untouched." }
        }
        finally { Pop-Location }
    }
    if (-not (Test-Path (Join-Path $DistDir "index.html"))) {
        Fail "frontend/dist/index.html is missing - nothing to deploy."
    }
    Write-Ok "Build present: $DistDir"

    # 2. Connect + prepare staging (1 connection)
    Write-Step "Connecting and preparing staging dir..."
    if (-not (Invoke-RemoteWithRetry "test -d '$WebRoot' && rm -rf '$remoteNew' && mkdir -p '$remoteNew'")) {
        Fail @"
Could not connect, or $WebRoot does not exist / is not writable.
  - If the .ppk has a passphrase, load it into Pageant first, then re-run:
        pageant.exe "$KeyPath"
  - First-ever connection from PuTTY also needs the host key cached once:
        plink -i "$KeyPath" $remoteTarget   (accept the fingerprint)
  - Otherwise verify the host ($VpsHost), user ($VpsUser), and that $WebRoot exists.
"@
    }
    Write-Ok "Connected; staging dir ready."

    # 3. Pack + upload (1 connection)
    $localFileCount = (Get-ChildItem -Recurse -File $DistDir).Count
    if ($localFileCount -eq 0) { Fail "No files in $DistDir to upload." }

    $archive = Join-Path $env:TEMP ("senpan-dist-{0}.tar.gz" -f [guid]::NewGuid())
    Write-Step ("Packing {0} files into one archive..." -f $localFileCount)
    Push-Location $FrontendDir
    try {
        # Paths inside the archive stay rooted at `dist/`, so extracting with
        # `-C $remoteNew` lands exactly where `pscp -r dist` used to put them.
        & $tar -czf $archive dist
        if ($LASTEXITCODE -ne 0) { Fail "Could not pack $DistDir into an archive (tar exit $LASTEXITCODE)." }
    }
    finally { Pop-Location }
    $archiveKb = [math]::Round((Get-Item $archive).Length / 1KB)
    $rawKb = [math]::Round(((Get-ChildItem -Recurse -File $DistDir | Measure-Object Length -Sum).Sum / 1KB))
    Write-Ok ("Packed {0} KB -> {1} KB." -f $rawKb, $archiveKb)

    try {
        Write-Step ("Uploading the {0} KB archive (single stream)..." -f $archiveKb)
        $r = Invoke-Upload -ArchivePath $archive -RemotePath $remoteArchive
        if (-not $r.Ok) {
            Write-Host "    Upload failed - possibly the SSH rate limit (ufw limit ssh)." -ForegroundColor Yellow
            Write-Host "    Waiting 35s for the 30s window to clear, then retrying once..." -ForegroundColor Yellow
            Start-Sleep -Seconds 35
            $r = Invoke-Upload -ArchivePath $archive -RemotePath $remoteArchive
        }
    }
    finally { Remove-Item $archive -Force -ErrorAction SilentlyContinue }
    if (-not $r.Ok) { Fail "Upload failed. Live site untouched (still serving the previous build).`n$($r.Err)" }
    Write-Ok ("Upload finished in {0:mm\:ss}." -f $r.Elapsed)

    # 4. Extract + verify + swap (1 connection)
    #    All three in one command so the connection count is unchanged. `set -e`
    #    means a truncated archive fails at tar and never reaches the swap, so the
    #    live site keeps serving the previous build.
    Write-Step "Extracting, verifying and swapping into place..."
    $swap = @"
set -e
tar -xzf '$remoteArchive' -C '$remoteNew'
rm -f '$remoteArchive'
cnt=`$(find '$remoteNew/dist' -type f | wc -l)
if [ "`$cnt" -ne $localFileCount ]; then echo "incomplete upload: `$cnt/$localFileCount files" >&2; exit 3; fi
chmod -R a+rX '$remoteNew/dist'
rm -rf '$remoteOld'
if [ -d '$remoteLive' ]; then mv '$remoteLive' '$remoteOld'; fi
mv '$remoteNew/dist' '$remoteLive'
rm -rf '$remoteNew'
"@
    if ($NoBackup) { $swap += "`nrm -rf '$remoteOld'" }
    if (-not (Invoke-RemoteWithRetry $swap)) {
        Fail "Extract/verify/swap failed. The previous build may be at $remoteOld - on the host, restore with: mv '$remoteOld' '$remoteLive'"
    }

    # 5. Sync the Apache config so header/caching/rewrite changes ship with the
    #    frontend. The repo copy (deploy/.htaccess) is the source of truth - edit it
    #    there, not on the host, since this overwrites the live file on each deploy.
    #    Non-fatal: the bundle is already live, so a failed config sync only warns.
    Write-Step "Syncing .htaccess (Apache config)..."
    if (Test-Path $HtaccessLocal) {
        if (Send-FileVerified $HtaccessLocal "$WebRoot/.htaccess") {
            Write-Ok ".htaccess synced to $WebRoot/.htaccess"
        }
        else {
            Write-Host "    WARNING: .htaccess upload failed; the site bundle is deployed but the Apache config was NOT updated. Re-run, or upload deploy/.htaccess manually." -ForegroundColor Yellow
        }
    }
    else {
        Write-Host "    WARNING: $HtaccessLocal not found; skipped the .htaccess sync." -ForegroundColor Yellow
    }

    Write-Host "`n[OK] Frontend deployed to ${remoteTarget}:$remoteLive" -ForegroundColor Green
    if (-not $NoBackup) {
        Write-Host "     Previous build kept at $remoteOld (rollback: mv '$remoteOld' '$remoteLive')." -ForegroundColor DarkGray
    }
    Write-Host "     Assets are content-hashed and the PWA auto-updates, so no manual cache bust is needed." -ForegroundColor DarkGray
}

# == Backend ===================================================================
function Deploy-Backend {
    $remoteBin = "$RemoteOptDir/app-suite"
    $remoteBinNew = "$RemoteOptDir/app-suite.new"
    $remoteBinOld = "$RemoteOptDir/app-suite.old"

    # 1. Cross-compile the static linux/amd64 binary
    if ($SkipBuild) {
        Write-Step "Skipping backend build (-SkipBuild); using existing $LocalBinary"
    }
    else {
        if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
            Fail "Go toolchain not found (go). Install Go or add it to PATH."
        }
        Write-Step "Building backend (GOOS=linux GOARCH=amd64 CGO_ENABLED=0, stripped)..."
        $savedGOOS = $env:GOOS; $savedGOARCH = $env:GOARCH; $savedCGO = $env:CGO_ENABLED
        Push-Location $BackendDir
        try {
            $env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
            go build "-ldflags=-s -w" -o app-suite .
            if ($LASTEXITCODE -ne 0) { Fail "Backend build failed (exit $LASTEXITCODE). Service untouched." }
        }
        finally {
            Pop-Location
            Restore-Env 'GOOS' $savedGOOS
            Restore-Env 'GOARCH' $savedGOARCH
            Restore-Env 'CGO_ENABLED' $savedCGO
        }
    }
    if (-not (Test-Path $LocalBinary)) { Fail "Backend binary not found at $LocalBinary - nothing to deploy." }
    Write-Ok ("Binary present: $LocalBinary ({0:N1} MB)" -f ((Get-Item $LocalBinary).Length / 1MB))

    # 2. Stop the service, back up the current binary AND snapshot the database
    #    (1 connection).
    #
    #    The database snapshot is the important half. Starting the new binary runs
    #    ensureSchema against the LIVE database, and several migrations rebuild
    #    tables (create-copy-drop-rename) - irreversible once committed. Step 4
    #    rolls the BINARY back if the new one won't stay up, but nothing can
    #    un-migrate the database, so without a snapshot taken here a bad release
    #    leaves no way back. The service is already stopped, so a plain cp is a
    #    consistent copy (the same reasoning Sync-LiveDbToDev relies on) - and the
    #    -wal/-shm sidecars are checkpointed into the file by the clean stop.
    $dbSnap = "$RemoteDbPath.pre-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
    Write-Step "Stopping $ServiceName.service, backing up the binary and snapshotting the database..."
    $stopCmd = @"
systemctl stop '$ServiceName'
cp -f '$remoteBin' '$remoteBinOld' 2>/dev/null || true
if [ -f '$RemoteDbPath' ]; then cp -f '$RemoteDbPath' '$dbSnap'; fi
# Keep the three most recent pre-deploy snapshots; older ones are just disk.
ls -1t '$RemoteDbPath'.pre-* 2>/dev/null | tail -n +4 | xargs -r rm -f
"@
    if (-not (Invoke-RemoteWithRetry $stopCmd)) {
        Fail "Could not stop $ServiceName.service (or connect). Check: plink -i `"$KeyPath`" $remoteTarget `"systemctl status $ServiceName`""
    }
    Write-Ok "Service stopped; database snapshotted -> $dbSnap"

    # 3. Upload the new binary to a temp name (1 connection)
    Write-Step "Uploading binary -> $remoteBinNew ..."
    if (-not (Send-File $LocalBinary $remoteBinNew)) {
        Write-Host "    Upload failed; restarting the previous binary so the service isn't left down..." -ForegroundColor Yellow
        Invoke-RemoteWithRetry "systemctl start '$ServiceName'" | Out-Null
        Fail "Binary upload failed. The previous build was restarted; verify with: systemctl status $ServiceName"
    }
    Write-Ok "Binary uploaded."

    # 4. Install + start, rolling back if the new binary doesn't stay active (1 connection)
    Write-Step "Installing the new binary and starting $ServiceName.service..."
    $install = @"
set -e
chmod +x '$remoteBinNew'
mv -f '$remoteBinNew' '$remoteBin'
systemctl start '$ServiceName'
sleep 2
if ! systemctl is-active --quiet '$ServiceName'; then
  echo 'new binary did not stay active - rolling back' >&2
  if [ -f '$remoteBinOld' ]; then mv -f '$remoteBinOld' '$remoteBin'; systemctl restart '$ServiceName'; fi
  exit 1
fi
echo active
"@
    if ($NoBackup) { $install += "`nrm -f '$remoteBinOld'" }
    if (-not (Invoke-RemoteWithRetry $install)) {
        # The new binary already migrated the live database before it failed, and a
        # binary rollback cannot undo that. A backend at or below the old schema
        # version now refuses to start rather than running against a schema it has
        # never seen, so say plainly how to get back.
        Write-Host ""
        Write-Host "    The new binary ran its migrations against the live database before failing." -ForegroundColor Yellow
        Write-Host "    If the rolled-back binary refuses to start because the schema is newer than it," -ForegroundColor Yellow
        Write-Host "    restore the pre-deploy snapshot on the host:" -ForegroundColor Yellow
        Write-Host "      systemctl stop $ServiceName && cp -f '$dbSnap' '$RemoteDbPath' && systemctl start $ServiceName" -ForegroundColor Yellow
        Fail "New binary failed to start; rolled back to the previous one (if a backup existed). Check: plink -i `"$KeyPath`" $remoteTarget `"journalctl -u $ServiceName -n 50 --no-pager`""
    }
    Write-Host "`n[OK] Backend deployed; $ServiceName.service is active on $remoteTarget." -ForegroundColor Green
    if (-not $NoBackup) {
        Write-Host "     Previous binary kept at $remoteBinOld (rollback: mv it over app-suite + systemctl restart $ServiceName)." -ForegroundColor DarkGray
    }
}

# == Refresh the local dev DB from live (after a backend deploy) ================
# Runs after Deploy-Backend so the copy reflects any schema migrations the new
# binary applied on startup. Snapshots the live DB on the host WHILE the service is
# stopped (so no live connection is mid-write), restarts the service immediately,
# then pulls the frozen snapshot down - the service is down only for a local file
# copy, not the network transfer. Its SSH connections are a separate group; the
# dispatcher inserts the standard 35s rate-limit pause before calling it.
function Sync-LiveDbToDev {
    # Snapshot beside the live DB (a name SQLite will never open as a database).
    $remoteSnap = "$RemoteDbPath.snapshot"

    # 1. On the host (1 connection): stop -> snapshot -> restart. No `set -e`: once
    #    the service is stopped, `systemctl start` must ALWAYS run, so a failed cp (or
    #    a slow start) never leaves the service down. A failed *stop* aborts before
    #    the copy (the service was never taken down, so nothing to restart).
    Write-Step "Snapshotting live DB on the host (stop -> copy -> restart $ServiceName)..."
    $snap = @"
rm -f '$remoteSnap'
systemctl stop '$ServiceName' || { echo 'could not stop service for snapshot' >&2; exit 3; }
cp -f '$RemoteDbPath' '$remoteSnap'; rc=`$?
systemctl start '$ServiceName'
if [ `$rc -ne 0 ]; then echo 'DB snapshot copy failed' >&2; exit 1; fi
sleep 2
if ! systemctl is-active --quiet '$ServiceName'; then echo 'service did not return active after snapshot' >&2; exit 2; fi
echo snapshotted
"@
    if (-not (Invoke-RemoteWithRetry $snap)) {
        Fail @"
Live DB snapshot step failed. Depending on where it stopped, $ServiceName may be DOWN.
  Check it:   plink -i "$KeyPath" $remoteTarget "systemctl status $ServiceName"
  Restart it: plink -i "$KeyPath" $remoteTarget "systemctl start $ServiceName"
The local dev DB was NOT modified.
"@
    }
    Write-Ok "Live DB snapshotted; $ServiceName restarted."

    # 2. Back up the current local dev DB (timestamped, matching devdata's convention)
    #    before it's overwritten.
    if (-not (Test-Path $DevDataDir)) { New-Item -ItemType Directory -Path $DevDataDir | Out-Null }
    if (Test-Path $DevDbPath) {
        $bak = "$DevDbPath.bak-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
        Copy-Item $DevDbPath $bak -Force
        Write-Ok "Backed up existing dev DB -> $bak"
    }

    # 3. Download the snapshot into devdata/ (1 connection).
    Write-Step "Downloading live DB -> $DevDbPath ..."
    if (-not (Get-File $remoteSnap $DevDbPath)) {
        # Best-effort remote cleanup even on failure, then bail. The dev DB backup
        # (if any) is untouched and the live service is running.
        & $plink -batch -i $KeyPath $remoteTarget "rm -f '$remoteSnap'" 2>&1 | Out-Null
        Fail "Live DB download failed. Local dev DB left as-is; $ServiceName is running on the host."
    }

    # 4. Clear any stale local WAL/SHM sidecars from the OLD dev DB so SQLite doesn't
    #    try to replay them onto the freshly copied file (mismatched salts = refusal
    #    or corruption). The pulled file is self-contained (the host checkpointed the
    #    WAL into it on the clean stop before the copy).
    Remove-Item "$DevDbPath-wal", "$DevDbPath-shm" -Force -ErrorAction SilentlyContinue

    # 5. Best-effort: remove the remote snapshot so a full copy of the live DB doesn't
    #    linger in the host's data dir. Non-fatal, no retry (a stale snapshot is just
    #    rm -f'd at the start of the next run anyway).
    & $plink -batch -i $KeyPath $remoteTarget "rm -f '$remoteSnap'" 2>&1 | Out-Null

    # 6. Prune old dev-DB backups. Every backend deploy timestamps a fresh copy, and
    #    nothing removed the previous ones - so full copies of the LIVE database,
    #    each carrying real password hashes, API tokens and Discord webhook URLs,
    #    accumulated on the workstation indefinitely. Keep the three most recent.
    $keep = 3
    $stale = @(Get-ChildItem "$DevDbPath.bak-*" -ErrorAction SilentlyContinue |
        Sort-Object Name -Descending | Select-Object -Skip $keep)
    if ($stale.Count -gt 0) {
        $stale | Remove-Item -Force -ErrorAction SilentlyContinue
        Write-Ok "Pruned $($stale.Count) old dev DB backup(s); keeping the newest $keep."
    }

    Write-Host "`n[OK] Local dev DB refreshed from live: $DevDbPath" -ForegroundColor Green
    Write-Host "     The previous dev DB is kept alongside it as *.bak-<timestamp> (newest $keep retained)." -ForegroundColor DarkGray
    Write-Host "     These hold LIVE data (real accounts, password hashes, tokens) - keep them local." -ForegroundColor DarkGray
}

# == Plugin (Dalamud custom repo) ==============================================
function Deploy-Plugin {
    # 1. Build the plugin (DalamudPackager emits .../SenpanCompanionAdmin/latest.zip).
    if ($SkipBuild) {
        Write-Step "Skipping plugin build (-SkipBuild); using existing $PluginZip"
    }
    else {
        if (-not (Get-Command dotnet -ErrorAction SilentlyContinue)) {
            Fail ".NET SDK not found (dotnet). Install the .NET 10 SDK or add it to PATH."
        }
        Write-Step "Building plugin (dotnet build -c Release)..."
        Push-Location $PluginDir
        try {
            dotnet build -c Release --nologo -v minimal
            if ($LASTEXITCODE -ne 0) { Fail "Plugin build failed (exit $LASTEXITCODE)." }
        }
        finally { Pop-Location }
    }
    if (-not (Test-Path $PluginZip)) { Fail "Plugin package not found at $PluginZip - nothing to deploy." }
    if (-not (Test-Path $PluginMaster)) { Fail "pluginmaster.json not found at $PluginMaster." }
    if (-not (Test-Path $PluginManifest)) { Fail "Built plugin manifest not found at $PluginManifest (build the plugin first)." }

    # 2. Version guardrail. Read the version DalamudPackager baked into latest.zip's
    #    manifest and make sure the repo index (pluginmaster.json) advertises the same
    #    one - Dalamud refuses an update whose packaged version doesn't match the repo
    #    ("Distributed plugin version does not match repo version"), so fail early with
    #    guidance rather than publishing a package that can't install.
    $pkgVersion = (Get-Content $PluginManifest -Raw | ConvertFrom-Json).AssemblyVersion
    if (-not $pkgVersion) { Fail "Could not read AssemblyVersion from $PluginManifest." }
    $masterText = Get-Content $PluginMaster -Raw
    $masterVersion = if ($masterText -match '"AssemblyVersion"\s*:\s*"([^"]+)"') { $Matches[1] } else { $null }
    if ($masterVersion -ne $pkgVersion) {
        Fail ("Version mismatch: the built package is $pkgVersion but pluginmaster.json says '$masterVersion'.`n" +
            "  Bump AssemblyVersion (and LastUpdate) in plugins/pluginmaster.json to $pkgVersion, then re-run.")
    }
    Write-Ok "Package present: $PluginZip (v$pkgVersion)"

    # 3. Cache-bust the download links. apps.senpan.cafe is behind Cloudflare, which
    #    edge-caches .zip; appending ?v=<version> makes each release a distinct cache
    #    key (a fresh MISS that fetches the new package). The repo file keeps clean
    #    links; the version is injected HERE so it can never drift out of sync. This is
    #    belt-and-braces with the no-store headers in plugin.htaccess (step 6).
    $bustedMaster = [regex]::Replace($masterText, 'latest\.zip(\?v=[^"\s]*)?', "latest.zip?v=$pkgVersion")
    $tmpMaster = Join-Path $env:TEMP ("pluginmaster-{0}.json" -f [guid]::NewGuid())
    [System.IO.File]::WriteAllText($tmpMaster, $bustedMaster)

    try {
        # 4. Prepare the remote repo dir (connection 1).
        Write-Step "Preparing remote plugin dir..."
        if (-not (Invoke-RemoteWithRetry "mkdir -p '$remotePluginZipDir'")) {
            Fail "Could not create $remotePluginZipDir on the host."
        }
        Write-Ok "Remote dir ready."

        # 5. Upload the package (connection 2).
        Write-Step "Uploading latest.zip -> $remotePluginZipDir/latest.zip ..."
        if (-not (Send-FileVerified $PluginZip "$remotePluginZipDir/latest.zip")) {
            Fail "Plugin package upload failed."
        }
        Write-Ok "Package uploaded."

        # 6. Upload the repo index with cache-busted download links (connection 3).
        Write-Step "Uploading pluginmaster.json (cache-bust ?v=$pkgVersion) -> $remotePluginDir/pluginmaster.json ..."
        if (-not (Send-FileVerified $tmpMaster "$remotePluginDir/pluginmaster.json")) {
            Fail "pluginmaster.json upload failed."
        }
        Write-Ok "Repo index uploaded."
    }
    finally {
        Remove-Item $tmpMaster -Force -ErrorAction SilentlyContinue
    }

    # 7. Sync the no-cache Apache config so Cloudflare never serves a stale package
    #    (connection 4). The repo copy (deploy/plugin.htaccess) is the source of truth.
    #    Non-fatal: the package + index are already live and the ?v= cache-bust still
    #    protects updates, so a failed header sync only warns.
    Write-Step "Syncing plugin .htaccess (no-cache headers)..."
    if (Test-Path $PluginHtaccess) {
        if (Send-FileVerified $PluginHtaccess "$remotePluginDir/.htaccess") {
            Write-Ok ".htaccess synced to $remotePluginDir/.htaccess"
        }
        else {
            Write-Host "    WARNING: plugin .htaccess upload failed; the package is live but Cloudflare may still edge-cache latest.zip (the ?v= cache-bust still applies). Re-run, or upload deploy/plugin.htaccess to $remotePluginDir/.htaccess manually." -ForegroundColor Yellow
        }
    }
    else {
        Write-Host "    WARNING: $PluginHtaccess not found; skipped the plugin .htaccess sync." -ForegroundColor Yellow
    }

    Write-Host "`n[OK] Plugin repo published on $remoteTarget (v$pkgVersion)." -ForegroundColor Green
    Write-Host "     Dalamud custom repo URL: https://apps.senpan.cafe/plugin/pluginmaster.json" -ForegroundColor DarkGray
    Write-Host "     Bump AssemblyVersion + LastUpdate in pluginmaster.json before each release; the ?v= cache-bust and no-cache headers are automatic." -ForegroundColor DarkGray
}

# -- Dispatch ------------------------------------------------------------------
Write-Host "Deploy target: $Target -> $remoteTarget" -ForegroundColor Yellow
$pause = { Write-Step "Pausing 35s between targets to stay under the SSH connection rate limit..."; Start-Sleep -Seconds 35 }
# After a backend deploy, refresh the local dev DB from live (unless -NoDbPull). Its
# SSH connections are a fresh group, so pause first to reset the rate-limit window.
$dbPull = { if (-not $NoDbPull) { & $pause; Sync-LiveDbToDev } }
switch ($Target) {
    'frontend' { Deploy-Frontend }
    'backend' { Deploy-Backend; & $dbPull }
    'plugin' { Deploy-Plugin }
    # 'main' is the preferred name for frontend + backend; 'both' is kept as an alias.
    #
    # The API goes FIRST. A new backend is additive to the old SPA - the endpoints
    # and fields it adds are simply unused - whereas a new SPA against the old API
    # calls routes that do not exist yet. Deploying the shell first left the site
    # serving a frontend that 404s on this release's new endpoints for the whole
    # backend step (the 35s rate-limit pause plus a local go build, about a minute).
    # $dbPull already opens with its own pause, so it goes last and no extra pause
    # is needed between the two deploys.
    { $_ -in 'both', 'main' } {
        Deploy-Backend
        & $pause
        Deploy-Frontend
        & $dbPull
    }
    # 'all' is everything: backend + frontend + plugin, API first for the same reason.
    'all' {
        Deploy-Backend
        & $pause
        Deploy-Frontend
        & $pause
        Deploy-Plugin
        & $dbPull
    }
}
