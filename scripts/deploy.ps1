#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Build and deploy the Senpan App Suite frontend and/or backend to the host.

.DESCRIPTION
    -Target frontend (default):
        1. Builds frontend/ (vue-tsc + vite) -> frontend/dist.
        2. Uploads it to <WebRoot>/dist.new via pscp (one stream; elapsed-time
           indicator, not per-file output).
        3. Verifies every file arrived, then swaps it in: live "dist" -> "dist.old"
           (rollback backup, overwritten each deploy), new build -> "dist".
        4. Syncs deploy/.htaccess -> <WebRoot>/.htaccess so Apache header/caching/
           rewrite changes ship with the frontend (non-fatal if it fails). The repo
           copy is the source of truth — edit it there, not on the host.

    -Target backend:
        1. Cross-compiles a static linux/amd64 binary:
               GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o app-suite .
        2. Stops <ServiceName>.service and backs up the current binary.
        3. Uploads the new binary to <RemoteOptDir>/app-suite (via a .new temp).
        4. Installs it and starts the service, rolling back to the previous binary
           if the new one fails to stay active.

    -Target main (alias: both): frontend, then backend (with a short pause between
    to stay under the SSH rate limit).

    -Target all: frontend, then backend, then plugin (paused between each).

    Uses PuTTY's pscp/plink so the DigitalOcean .ppk key works directly. pscp/plink
    run in batch mode, so a passphrase-protected .ppk must be loaded into Pageant
    first:  pageant.exe "<KeyPath>"  (enter the passphrase once per Windows session).

    Each target uses just THREE SSH connections and does not poll during transfers,
    to stay under `ufw limit ssh` (which drops the IP after 6 connections in 30s).
    A connection that gets rate-limited is retried once after a 35s wait.

.PARAMETER Target
    What to deploy: frontend (default), backend, main (frontend + backend; 'both'
    is a kept alias), all (frontend + backend + plugin), or plugin (build + publish
    the Dalamud custom-repo files: SenpanCompanionAdmin/latest.zip + pluginmaster.json).

.PARAMETER VpsHost
    Droplet IP or hostname. If omitted, resolved from $env:SENPAN_VPS_HOST, then
    the untracked scripts/deploy.config.ps1 ($SenpanVpsHost).

.PARAMETER VpsUser
    SSH user. If omitted, resolved from $env:SENPAN_VPS_USER, then
    scripts/deploy.config.ps1 ($SenpanVpsUser).

.PARAMETER KeyPath
    Path to the PuTTY .ppk private key. If omitted, resolved from
    $env:SENPAN_DEPLOY_KEY, then scripts/deploy.config.ps1 ($SenpanDeployKey).

.PARAMETER WebRoot
    Apache DocumentRoot on the host (holds "dist" + "plugin"; frontend + plugin).
    If omitted, resolved from $env:SENPAN_WEB_ROOT, then scripts/deploy.config.ps1
    ($SenpanWebRoot).

.PARAMETER ServiceName
    systemd service to restart for the backend. If omitted, resolved from
    $env:SENPAN_SERVICE_NAME, then scripts/deploy.config.ps1 ($SenpanServiceName).

.PARAMETER RemoteOptDir
    Directory on the host holding the backend binary. If omitted, resolved from
    $env:SENPAN_REMOTE_OPT_DIR, then scripts/deploy.config.ps1 ($SenpanRemoteOptDir).

.PARAMETER SkipBuild
    Deploy the existing build artifact(s) without rebuilding.

.PARAMETER NoBackup
    Remove the rollback backup (dist.old / app-suite.old) after a successful deploy.

.EXAMPLE
    .\scripts\deploy.ps1                 # frontend (default)

.EXAMPLE
    .\scripts\deploy.ps1 -Target backend

.EXAMPLE
    .\scripts\deploy.ps1 -Target both

.EXAMPLE
    .\scripts\deploy.ps1 -Target plugin   # build + publish the Dalamud plugin repo
#>
[CmdletBinding()]
param(
    [ValidateSet('frontend', 'backend', 'both', 'main', 'all', 'plugin')]
    [string]$Target = 'frontend',

    # Server-specific settings are NOT baked into this (tracked) script — nothing
    # here reveals the host, SSH user, filesystem paths, or service name. Each is
    # resolved below from its -param, then the matching $env:SENPAN_* variable (the
    # param default), then the untracked scripts/deploy.config.ps1. See
    # scripts/deploy.config.example.ps1.
    [string]$VpsHost = $env:SENPAN_VPS_HOST,

    [string]$VpsUser = $env:SENPAN_VPS_USER,

    [string]$KeyPath = $env:SENPAN_DEPLOY_KEY,

    [string]$WebRoot = $env:SENPAN_WEB_ROOT,

    [string]$ServiceName = $env:SENPAN_SERVICE_NAME,

    [string]$RemoteOptDir = $env:SENPAN_REMOTE_OPT_DIR,

    [switch]$SkipBuild,

    [switch]$NoBackup
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

# ── Server settings resolution ────────────────────────────────────────────────
# Every environment-specific value (host, SSH user, key, webroot, service name,
# opt dir) lives OUTSIDE this tracked script. Resolution order for each: its
# -param, then the matching $env:SENPAN_* variable (already applied as the param
# default), then the untracked scripts/deploy.config.ps1. Copy
# scripts/deploy.config.example.ps1 to deploy.config.ps1 and fill it in once.
$configPath = Join-Path $PSScriptRoot 'deploy.config.ps1'
if (Test-Path $configPath) { . $configPath }
if (-not $VpsHost) { $VpsHost = $SenpanVpsHost }
if (-not $VpsUser) { $VpsUser = $SenpanVpsUser }
if (-not $KeyPath) { $KeyPath = $SenpanDeployKey }
if (-not $WebRoot) { $WebRoot = $SenpanWebRoot }
if (-not $ServiceName) { $ServiceName = $SenpanServiceName }
if (-not $RemoteOptDir) { $RemoteOptDir = $SenpanRemoteOptDir }

$missing = @()
if (-not $VpsHost) { $missing += 'VpsHost (-VpsHost / $env:SENPAN_VPS_HOST / $SenpanVpsHost)' }
if (-not $VpsUser) { $missing += 'VpsUser (-VpsUser / $env:SENPAN_VPS_USER / $SenpanVpsUser)' }
if (-not $KeyPath) { $missing += 'KeyPath (-KeyPath / $env:SENPAN_DEPLOY_KEY / $SenpanDeployKey)' }
if (-not $WebRoot) { $missing += 'WebRoot (-WebRoot / $env:SENPAN_WEB_ROOT / $SenpanWebRoot)' }
if (-not $ServiceName) { $missing += 'ServiceName (-ServiceName / $env:SENPAN_SERVICE_NAME / $SenpanServiceName)' }
if (-not $RemoteOptDir) { $missing += 'RemoteOptDir (-RemoteOptDir / $env:SENPAN_REMOTE_OPT_DIR / $SenpanRemoteOptDir)' }
if ($missing.Count -gt 0) {
    Fail "Missing deploy settings:`n  - $($missing -join "`n  - ")`nSet these via -params, `$env:SENPAN_* variables, or scripts/deploy.config.ps1 (copy scripts/deploy.config.example.ps1)."
}

# Restore an env var to a prior value ($null => remove it).
function Restore-Env([string]$name, $value) {
    if ($null -eq $value) { Remove-Item "Env:$name" -ErrorAction SilentlyContinue }
    else { Set-Item "Env:$name" $value }
}

# Run a remote command via plink. Retry once (after the 30s ufw window) ONLY when
# the failure looks like a dropped/refused connection — not a genuine command
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

# One pscp upload of dist/ -> the frontend staging dir, with an elapsed-time
# indicator and no polling. Returns @{ Ok; Err; Elapsed }.
function Invoke-Upload([int]$FileCount) {
    $outFile = Join-Path $env:TEMP ("pscp-out-{0}.log" -f [guid]::NewGuid())
    $errFile = Join-Path $env:TEMP ("pscp-err-{0}.log" -f [guid]::NewGuid())
    $proc = Start-Process -FilePath $pscp -NoNewWindow -PassThru -WorkingDirectory $FrontendDir `
        -RedirectStandardOutput $outFile -RedirectStandardError $errFile `
        -ArgumentList @('-batch', '-C', '-r', '-i', $KeyPath, 'dist', "${remoteTarget}:$remoteNew")
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    $maxSeconds = 900
    $killed = $false
    while (-not $proc.HasExited) {
        Start-Sleep -Seconds 1
        Write-Progress -Activity "Uploading frontend" -Status ("{0} files - {1:mm\:ss} elapsed" -f $FileCount, $sw.Elapsed)
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

# ── Resolve paths & tools ─────────────────────────────────────────────────────
$RepoRoot = Split-Path $PSScriptRoot -Parent
$FrontendDir = Join-Path $RepoRoot "frontend"
$DistDir = Join-Path $FrontendDir "dist"
$HtaccessLocal = Join-Path $RepoRoot "deploy\.htaccess"
$BackendDir = Join-Path $RepoRoot "backend"
$LocalBinary = Join-Path $BackendDir "app-suite"
$PluginDir = Join-Path $RepoRoot "plugins\SenpanCompanion"
$PluginZip = Join-Path $PluginDir "bin\Release\SenpanCompanionAdmin\latest.zip"
$PluginMaster = Join-Path $RepoRoot "plugins\pluginmaster.json"

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

$remoteTarget = "$VpsUser@$VpsHost"
$remoteLive = "$WebRoot/dist"
$remoteNew = "$WebRoot/dist.new"
$remoteOld = "$WebRoot/dist.old"
$remotePluginDir = "$WebRoot/plugin"
$remotePluginZipDir = "$remotePluginDir/SenpanCompanionAdmin"

# ══ Frontend ══════════════════════════════════════════════════════════════════
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

    # 3. Upload (1 connection)
    $localFileCount = (Get-ChildItem -Recurse -File $DistDir).Count
    if ($localFileCount -eq 0) { Fail "No files in $DistDir to upload." }
    Write-Step ("Uploading {0} files (single stream)..." -f $localFileCount)
    $r = Invoke-Upload -FileCount $localFileCount
    if (-not $r.Ok) {
        Write-Host "    Upload failed - possibly the SSH rate limit (ufw limit ssh)." -ForegroundColor Yellow
        Write-Host "    Waiting 35s for the 30s window to clear, then retrying once..." -ForegroundColor Yellow
        Start-Sleep -Seconds 35
        $r = Invoke-Upload -FileCount $localFileCount
    }
    if (-not $r.Ok) { Fail "Upload failed. Live site untouched (still serving the previous build).`n$($r.Err)" }
    Write-Ok ("Upload finished in {0:mm\:ss}." -f $r.Elapsed)

    # 4. Verify + swap (1 connection)
    Write-Step "Verifying upload and swapping into place..."
    $swap = @"
set -e
cnt=`$(find '$remoteNew/dist' -type f | wc -l)
if [ "`$cnt" -ne $localFileCount ]; then echo "incomplete upload: `$cnt/$localFileCount files" >&2; exit 3; fi
rm -rf '$remoteOld'
if [ -d '$remoteLive' ]; then mv '$remoteLive' '$remoteOld'; fi
mv '$remoteNew/dist' '$remoteLive'
rm -rf '$remoteNew'
"@
    if ($NoBackup) { $swap += "`nrm -rf '$remoteOld'" }
    if (-not (Invoke-RemoteWithRetry $swap)) {
        Fail "Verify/swap failed. The previous build may be at $remoteOld - on the host, restore with: mv '$remoteOld' '$remoteLive'"
    }

    # 5. Sync the Apache config so header/caching/rewrite changes ship with the
    #    frontend. The repo copy (deploy/.htaccess) is the source of truth — edit it
    #    there, not on the host, since this overwrites the live file on each deploy.
    #    Non-fatal: the bundle is already live, so a failed config sync only warns.
    Write-Step "Syncing .htaccess (Apache config)..."
    if (Test-Path $HtaccessLocal) {
        if (Send-File $HtaccessLocal "$WebRoot/.htaccess") {
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

# ══ Backend ═══════════════════════════════════════════════════════════════════
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

    # 2. Stop the service + back up the current binary (1 connection)
    Write-Step "Stopping $ServiceName.service and backing up the current binary..."
    if (-not (Invoke-RemoteWithRetry "systemctl stop '$ServiceName' && (cp -f '$remoteBin' '$remoteBinOld' 2>/dev/null || true)")) {
        Fail "Could not stop $ServiceName.service (or connect). Check: plink -i `"$KeyPath`" $remoteTarget `"systemctl status $ServiceName`""
    }
    Write-Ok "Service stopped."

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
        Fail "New binary failed to start; rolled back to the previous one (if a backup existed). Check: plink -i `"$KeyPath`" $remoteTarget `"journalctl -u $ServiceName -n 50 --no-pager`""
    }
    Write-Host "`n[OK] Backend deployed; $ServiceName.service is active on $remoteTarget." -ForegroundColor Green
    if (-not $NoBackup) {
        Write-Host "     Previous binary kept at $remoteBinOld (rollback: mv it over app-suite + systemctl restart $ServiceName)." -ForegroundColor DarkGray
    }
}

# ══ Plugin (Dalamud custom repo) ══════════════════════════════════════════════
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
    Write-Ok "Package present: $PluginZip"

    # 2. Prepare the remote repo dir (1 connection).
    Write-Step "Preparing remote plugin dir..."
    if (-not (Invoke-RemoteWithRetry "mkdir -p '$remotePluginZipDir'")) {
        Fail "Could not create $remotePluginZipDir on the host."
    }
    Write-Ok "Remote dir ready."

    # 3. Upload the package (1 connection).
    Write-Step "Uploading latest.zip -> $remotePluginZipDir/latest.zip ..."
    if (-not (Send-File $PluginZip "$remotePluginZipDir/latest.zip")) {
        Fail "Plugin package upload failed."
    }
    Write-Ok "Package uploaded."

    # 4. Upload the repo index (1 connection).
    Write-Step "Uploading pluginmaster.json -> $remotePluginDir/pluginmaster.json ..."
    if (-not (Send-File $PluginMaster "$remotePluginDir/pluginmaster.json")) {
        Fail "pluginmaster.json upload failed."
    }

    Write-Host "`n[OK] Plugin repo published on $remoteTarget." -ForegroundColor Green
    Write-Host "     Dalamud custom repo URL: https://apps.senpan.cafe/plugin/pluginmaster.json" -ForegroundColor DarkGray
    Write-Host "     Bump AssemblyVersion + LastUpdate in pluginmaster.json before each release." -ForegroundColor DarkGray
}

# ── Dispatch ──────────────────────────────────────────────────────────────────
Write-Host "Deploy target: $Target -> $remoteTarget" -ForegroundColor Yellow
$pause = { Write-Step "Pausing 35s between targets to stay under the SSH connection rate limit..."; Start-Sleep -Seconds 35 }
switch ($Target) {
    'frontend' { Deploy-Frontend }
    'backend' { Deploy-Backend }
    'plugin' { Deploy-Plugin }
    # 'main' is the preferred name for frontend + backend; 'both' is kept as an alias.
    { $_ -in 'both', 'main' } {
        Deploy-Frontend
        & $pause
        Deploy-Backend
    }
    # 'all' is everything: frontend + backend + plugin.
    'all' {
        Deploy-Frontend
        & $pause
        Deploy-Backend
        & $pause
        Deploy-Plugin
    }
}
