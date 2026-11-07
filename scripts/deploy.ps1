#!/usr/bin/env pwsh
#Requires -Version 7.4

<#
.SYNOPSIS
    Build and deploy the Senpan App Suite to the host: the frontend, the backend, the Dalamud plugin, or several at once.

.DESCRIPTION
    Built on shared-tooling's SharedDeploy module (shared/deploy/pwsh), the shape every
    deploy follows: one archive, one held SSH connection, one remote run.

    Locally, before anything connects, for each target in the run:
      backend   cross-compiles the static linux/amd64 binary (backend/app-suite);
      frontend  builds frontend/ (vue-tsc + vite) into frontend/dist;
      plugin    builds the plugin from clean and lays out its repository: latest.zip,
                plugins/pluginmaster.json with ?v=<version> on its download links, and
                deploy/plugin.htaccess. The built version must equal pluginmaster.json's.
    A broken build changes nothing. Everything goes into ONE archive.

    Then, on one SSH connection held for the whole run:
      1. With the backend: read the service from the host - the user it runs as, and the
         database (-db) and listen address (-addr) on its ExecStart. Nothing changes if
         the unit is missing or names a different database than RemoteDbPath.
      2. Upload the archive.
      3. One remote run checks the archive's SHA-256 and file count, then, API first:
           backend   stop the service, keep app-suite.old, snapshot the database to
                     <db>.pre-<time> (sqlite3 .backup, newest three kept), install the
                     binary, start it, and wait for /api/version to answer on the host;
                     if it does not, the previous binary goes back with the snapshot,
                     since the new one may have migrated a schema the old one refuses;
           frontend  swap dist in by renames (previous kept as dist.old), with
                     deploy/.htaccess beside it (previous kept as .htaccess.old);
           plugin    refuse a version not higher than the published one, then swap the
                     plugin folder in (previous kept as plugin.old).
      4. Fetch the site from here, through Cloudflare: /api/version with the version this
         run built, the page and its entry script, and the plugin index with this
         release's ?v=. If any fails, what this run changed goes back, plugin and
         frontend first, then the backend.
      5. With the backend, once that passes: check the daily database backup
         (Install-BackupTimer's shared job, run as the service's user at 10:00 UTC: a
         VACUUM INTO copy in a backups folder beside the database, named
         <service>-YYYY-MM-DD.sqlite, 28 days kept), then pull an online snapshot of the
         live database into devdata/database.sqlite, keeping the three previous local
         copies (skip with -NoDbPull).

    Settings resolve from the parameter, then $env:DEPLOY_*, then the git-ignored
    scripts/deploy.config.ps1 (copy scripts/deploy.config.example.ps1). Nothing about the
    host is in this file.

    Needs PuTTY's plink 0.85 or later (pscp is no longer used), with a key that has a
    passphrase loaded into Pageant first: pageant.exe "<KeyPath>". The host needs sqlite3
    and curl. The whole run is one SSH connection (plus one only if it drops and
    reconnects), which stays under the droplet's `ufw limit ssh`.

.PARAMETER Target
    What to deploy: main (the default: backend, then frontend; 'both' is a kept alias),
    frontend, backend, all (backend + frontend + plugin, in one run), or plugin (the Dalamud
    custom repo: SenpanCompanionAdmin/latest.zip, pluginmaster.json and plugin.htaccess).
    task deploy runs main, then plugin with -IfNewer; task deploy:frontend, deploy:backend
    and deploy:plugin run one each.

.PARAMETER IfNewer
    With the plugin in the run: a plugin whose version is already the published one is left
    out of the run (which ends there if it was the only target), where without it the run
    refuses. A lower version still refuses. task deploy passes it, so a deploy that didn't
    raise the plugin's version still succeeds.

.PARAMETER VpsHost
    Droplet IP or hostname. If omitted, resolved from $env:DEPLOY_VPS_HOST, then
    scripts/deploy.config.ps1 ($DeployVpsHost).

.PARAMETER VpsUser
    SSH user. If omitted, resolved from $env:DEPLOY_VPS_USER, then $DeployVpsUser.

.PARAMETER KeyPath
    Path to the PuTTY .ppk private key. If omitted, resolved from $env:DEPLOY_KEY_PATH,
    then $DeployKeyPath.

.PARAMETER WebRoot
    Apache DocumentRoot on the host (holds dist, .htaccess and plugin). If omitted,
    resolved from $env:DEPLOY_WEB_ROOT, then $DeployWebRoot.

.PARAMETER ServiceName
    The backend's systemd service. Its unit lives only on the host; this script never
    ships it. If omitted, resolved from $env:DEPLOY_SERVICE_NAME, then $DeployServiceName.

.PARAMETER RemoteOptDir
    Directory on the host holding the backend binary (app-suite). If omitted, resolved
    from $env:DEPLOY_REMOTE_OPT_DIR, then $DeployRemoteOptDir.

.PARAMETER RemoteDbPath
    The live SQLite database on the host. Normally left unset: the deploy reads it from
    the -db on the service's ExecStart. When set (here, $env:DEPLOY_REMOTE_DB_PATH or
    $DeployRemoteDbPath), it must match that path, or the deploy stops before anything
    changes; it is used on its own only when the unit names no -db.

.PARAMETER SiteUrl
    The site's public address, fetched after the deploy as visitors reach it. If omitted,
    resolved from $env:DEPLOY_SITE_URL, then $DeploySiteUrl, then https://apps.senpan.cafe
    (the address plugins/pluginmaster.json already publishes).

.PARAMETER ConfigPath
    The untracked settings file; by default scripts/deploy.config.ps1. It need not exist.

.PARAMETER SkipBuild
    Deploy the existing build artifact(s) without rebuilding. The smoke check then only
    asks /api/version to answer, since the binary's version is not known.

.PARAMETER NoBackup
    Remove the rollback copies (app-suite.old, dist.old, .htaccess.old, plugin.old) once
    the run succeeds on the host. The smoke check then has nothing to go back to, so a
    failure only stops the run. The database snapshot is still taken.

.PARAMETER NoDbPull
    Skip refreshing the local dev DB (devdata/database.sqlite) from live after a deploy
    that includes the backend.

.EXAMPLE
    .\scripts\deploy.ps1                 # backend then frontend (default)

.EXAMPLE
    .\scripts\deploy.ps1 -Target backend

.EXAMPLE
    .\scripts\deploy.ps1 -Target backend -NoDbPull   # deploy backend, don't refresh dev DB

.EXAMPLE
    .\scripts\deploy.ps1 -Target frontend

.EXAMPLE
    .\scripts\deploy.ps1 -Target plugin   # build + publish the Dalamud plugin repo
#>
[CmdletBinding()]
param(
    [ValidateSet('frontend', 'backend', 'both', 'main', 'all', 'plugin')]
    [string]$Target = 'main',

    # Server-specific settings are NOT in this tracked script: nothing here reveals the
    # host, SSH user, key, filesystem paths, service name or site address. Each resolves
    # from its -param, then $env:DEPLOY_*, then the untracked scripts/deploy.config.ps1.
    [string]$VpsHost,
    [string]$VpsUser,
    [string]$KeyPath,
    [string]$WebRoot,
    [string]$ServiceName,
    [string]$RemoteOptDir,
    [string]$RemoteDbPath,
    [string]$SiteUrl,
    [string]$ConfigPath = (Join-Path -Path $PSScriptRoot -ChildPath 'deploy.config.ps1'),

    [switch]$SkipBuild,
    [switch]$NoBackup,
    [switch]$NoDbPull,
    [switch]$IfNewer
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
Import-Module (Join-Path $PSScriptRoot '..\shared\deploy\pwsh\SharedDeploy.psd1') -Force
trap { Close-DeployConnection; Write-Host "`nERROR: $($_.Exception.Message)" -ForegroundColor Red; exit 1 }

$settings = Resolve-DeploySetting -ConfigPath $ConfigPath -Value @{
    VpsHost      = $VpsHost
    VpsUser      = $VpsUser
    KeyPath      = $KeyPath
    WebRoot      = $WebRoot
    ServiceName  = $ServiceName
    RemoteOptDir = $RemoteOptDir
    RemoteDbPath = $RemoteDbPath
    SiteUrl      = $SiteUrl
} -Required VpsHost, VpsUser, KeyPath, WebRoot, ServiceName, RemoteOptDir -Default @{
    # Public already (plugins/pluginmaster.json's download links), so not a host secret.
    SiteUrl = 'https://apps.senpan.cafe'
}

$web = $settings.WebRoot.TrimEnd('/')
$service = $settings.ServiceName
$remoteBinary = "$($settings.RemoteOptDir.TrimEnd('/'))/app-suite"
$site = $settings.SiteUrl.TrimEnd('/')
if ($site -notmatch '^https?://[^/\s]+$') {
    throw "SiteUrl must be the site's address alone, such as https://example.com, not '$($settings.SiteUrl)'."
}

$withBackend = $Target -in 'backend', 'both', 'main', 'all'
$withFrontend = $Target -in 'frontend', 'both', 'main', 'all'
$withPlugin = $Target -in 'plugin', 'all'
$targets = @(
    if ($withBackend) { 'backend' }
    if ($withFrontend) { 'frontend' }
    if ($withPlugin) { 'plugin' }
)

$repoRoot = Split-Path -Path $PSScriptRoot -Parent
$backendDir = Join-Path $repoRoot 'backend'
$frontendDir = Join-Path $repoRoot 'frontend'
$distDir = Join-Path $frontendDir 'dist'
$pluginDir = Join-Path $repoRoot 'plugins\SenpanCompanion'
$devDb = Join-Path $repoRoot 'devdata\database.sqlite'

Write-Host "Deploy target: $Target ($($targets -join ', ')) -> $($settings.VpsUser)@$($settings.VpsHost)" -ForegroundColor Yellow

# The plugin's version must be higher than the published one, since Dalamud updates to
# nothing else. The host refuses a lower one in the remote run as well, but by then a
# backend and frontend deployed in the same run (-Target all) are already live, so the
# published index is read here first, before anything is built or changed. If it can't be
# read, the host's check still stands.
if ($withPlugin) {
    $project = [xml](Get-Content -LiteralPath (Join-Path $pluginDir 'SenpanCompanion.csproj') -Raw)
    $planned = @($project.Project.PropertyGroup.Version | Where-Object { $_ })[0]
    $published = $null
    try {
        $published = @(Invoke-RestMethod -Uri "$site/plugin/pluginmaster.json" -TimeoutSec 15)[0].AssemblyVersion
    } catch {
        Write-Warning "The published plugin index could not be read ($($_.Exception.Message)); the host will check the version."
    }
    if ($planned -and $published -and [version]$planned -le [version]$published) {
        if ($IfNewer -and [version]$planned -eq [version]$published) {
            Write-Ok "Plugin $planned is already published; leaving it out of this run."
            $withPlugin = $false
            $targets = @($targets | Where-Object { $_ -ne 'plugin' })
            if ($targets.Count -eq 0) { exit 0 }
        } else {
            throw "The plugin's version $planned is not higher than the published $published, and Dalamud updates to nothing else. Raise <Version> in plugins/SenpanCompanion/SenpanCompanion.csproj (and plugins/pluginmaster.json); nothing was built or changed."
        }
    }
}

# -- Build, before anything connects -------------------------------------------
# Every piece goes into one archive, laid out as the host's steps want it: the binary as
# app-suite, the SPA as dist with .htaccess beside it, the plugin repository as plugin.
$content = [ordered]@{}
$backendVersion = $null
$entryScript = $null
$repo = $null
$archive = $null

if ($withBackend) {
    $binary = Join-Path $backendDir 'app-suite'
    if ($SkipBuild) {
        Write-Step 'Skipping the backend build (-SkipBuild); using the existing backend/app-suite'
        if (-not (Test-Path -LiteralPath $binary)) { throw "Backend binary not found at $binary - nothing to deploy." }
    } else {
        # The version this build answers /api/version with, which the smoke check looks for.
        $versionSource = Join-Path $backendDir 'internal\version\version.go'
        if (Test-Path -LiteralPath $versionSource) {
            $match = [regex]::Match((Get-Content -LiteralPath $versionSource -Raw), 'const\s+Version\s*=\s*"([^"]+)"')
            if ($match.Success) { $backendVersion = $match.Groups[1].Value }
        }
        $binary = Invoke-GoBuild -Path $backendDir -Output 'app-suite'
    }
    $content['app-suite'] = $binary
}

if ($withFrontend) {
    $index = Join-Path $distDir 'index.html'
    if ($SkipBuild) {
        Write-Step 'Skipping the frontend build (-SkipBuild); using the existing frontend/dist'
        if (-not (Test-Path -LiteralPath $index)) { throw 'frontend/dist/index.html is missing - nothing to deploy.' }
    } else {
        Invoke-NpmBuild -Path $frontendDir -Output $index
    }
    $content['dist'] = $distDir
    # The repo copy is the source of truth for the Apache config: edit it there, since each
    # frontend deploy replaces the live one.
    $content['.htaccess'] = Join-Path $repoRoot 'deploy\.htaccess'
    # The build's entry script: the smoke check wants the page to name it, and to fetch it.
    $entryScript = [regex]::Match((Get-Content -LiteralPath $index -Raw), 'src="(/assets/[^"]+\.js)"').Groups[1].Value
}

if ($withPlugin) {
    if ($SkipBuild) {
        Write-Step 'Skipping the plugin build (-SkipBuild); using the existing build'
    } else {
        Invoke-PluginBuild -Project (Join-Path $pluginDir 'SenpanCompanion.csproj')
    }
    $built = Join-Path $pluginDir 'bin\Release\SenpanCompanionAdmin'
    # pluginmaster.json is the repo index; its AssemblyVersion must equal the built one.
    # plugin.htaccess keeps Cloudflare from edge-caching the package (see the file).
    $repo = Export-PluginRepository -Manifest (Join-Path $built 'SenpanCompanionAdmin.json') `
        -IndexPath 'pluginmaster.json' -IndexSource (Join-Path $repoRoot 'plugins\pluginmaster.json') -File ([ordered]@{
            'SenpanCompanionAdmin/latest.zip' = Join-Path $built 'latest.zip'
            '.htaccess'                       = Join-Path $repoRoot 'deploy\plugin.htaccess'
        })
    $content['plugin'] = $repo.Path
}

try {
    Write-Step 'Packing the deploy...'
    $archive = Compress-DeployArchive -Content $content
    Write-Ok ('{0} files, {1:N1} MB, one archive' -f $archive.FileCount, ($archive.Size / 1MB))

    $connection = $settings | Open-DeployConnection

    # -- The service, as the host runs it (backend) ------------------------------
    # The unit lives only on the host, so it is the one place that says where the live
    # database is (-db), where the API listens (-addr) and whom the service runs as.
    if ($withBackend) {
        if ($service -notmatch '^[A-Za-z0-9_.@-]+$') { throw "ServiceName '$service' is not a systemd service name." }
        $configuredDb = [string]$settings.RemoteDbPath
        if ($configuredDb -match '[''"\s]') { throw "RemoteDbPath '$configuredDb' cannot hold quotes or spaces." }
        Write-Step "Reading $service.service on the host..."
        $read = Format-RemoteScript -Value @{ SERVICE = $service; DB = $configuredDb } -Template @'
svc='__SERVICE__'
show() { systemctl show "$svc.service" -p "$1" --value 2>/dev/null; }
if [ "$(show LoadState)" != loaded ]; then
  echo "$svc.service is not installed on the host; install its unit first (deploy/README.md). Nothing was changed." >&2
  exit 3
fi
user=$(show User)
group=$(show Group)
[ -n "$user" ] || user=root
[ -n "$group" ] || group=$(id -gn "$user" 2>/dev/null || true)
args=$(show ExecStart | tr ' ' '\n')
flag() { printf '%s\n' "$args" | sed -n -e "/^--\{0,1\}$1\$/{n;p;q;}" -e "s/^--\{0,1\}$1=//p" | head -n 1; }
db='__DB__'
[ -n "$db" ] || db=$(flag db)
echo "user=$user"
echo "group=$group"
echo "dynamic=$(show DynamicUser)"
echo "unit_db=$(flag db)"
echo "unit_addr=$(flag addr)"
if [ -n "$db" ] && [ -f "$db" ]; then echo "db_owner=$(stat -c '%U:%G' "$db")"; fi
'@
        $found = @{}
        foreach ($line in @(Invoke-Remote -Script $read)) {
            if ($line -match '^([a-z_]+)=(.*)$') { $found[$Matches[1]] = $Matches[2].Trim() }
        }

        $unitDb = $found['unit_db']
        if ($configuredDb -and $unitDb -and $configuredDb -ne $unitDb) {
            throw "RemoteDbPath is $configuredDb, but $service.service opens $unitDb (the -db on its ExecStart). Correct or clear RemoteDbPath; nothing was changed."
        }
        $db = if ($configuredDb) { $configuredDb } else { $unitDb }
        if (-not $db) {
            throw "$service.service names no -db on its ExecStart, so the live database is unknown. Set RemoteDbPath (-RemoteDbPath / `$env:DEPLOY_REMOTE_DB_PATH / `$DeployRemoteDbPath); nothing was changed."
        }

        # Where /api/version answers on the host: -addr, or the binary's own default (:8080,
        # backend/main.go). An address on every interface is asked on the loopback.
        $addr = if ($found['unit_addr']) { $found['unit_addr'] } else { ':8080' }
        if ($addr -match '^(|0\.0\.0\.0|\[::\])(:\d+)$') { $addr = "127.0.0.1$($Matches[2])" }
        if ($addr -notmatch '^[A-Za-z0-9.\[\]:-]+:\d+$') { throw "Could not tell where $service listens from its -addr '$addr'; nothing was changed." }

        $serviceOwner = if ($found['group']) { "$($found['user']):$($found['group'])" } else { $found['user'] }
        $dbOwner = if ($found['db_owner']) { $found['db_owner'] } else { 'not there yet' }
        Write-Ok "Live database: $db (owner $dbOwner); $service runs as $serviceOwner and listens on $addr."
        if ($found['user'] -eq 'root' -and $found['dynamic'] -ne 'yes') {
            Write-Warning "$service runs as root, so its backup will too. deploy/README.md (Running as a non-root user) says how to move it to its own account."
        }
    }

    # -- Upload, then one remote run ---------------------------------------------
    Write-Step 'Uploading...'
    $upload = Invoke-Upload -Path $archive.Path -Destination 'senpan-deploy.tgz' -Activity "Uploading Senpan App Suite ($($targets -join ', '))" -FileCount $archive.FileCount
    if (-not $upload.Ok) { throw "Upload failed; nothing on the host changed.`n$($upload.Err)" }
    Write-Ok ('Uploaded in {0:mm\:ss}.' -f $upload.Elapsed)

    # API first: a new backend is additive to the old SPA, while a new SPA calls endpoints an
    # old backend does not have yet; the plugin, a second client of the API, comes last.
    # Nothing changes until the archive's SHA-256 and file count check.
    $steps = @(
        if ($withBackend) {
            # The database snapshot is the way back from a migration: starting the new binary
            # migrates the live database, and the previous binary refuses a schema newer than
            # its own, so a rollback puts the snapshot back with the binary (decided
            # 2026-11-06, as Kindred Nights does). -Umask 077 keeps the snapshot (live
            # accounts, tokens, webhooks) for root.
            Install-ServiceBinary -AsStep -ServiceName $service -Binary $remoteBinary -Database $db -Umask '077' `
                -RestoreDatabaseOnRollback -HealthUrl "http://$addr/api/version" -NoBackup:$NoBackup
        }
        if ($withFrontend) {
            Switch-RemoteDirectory -AsStep -Live "$web/dist" -File @{ '.htaccess' = "$web/.htaccess" } -NoBackup:$NoBackup
        }
        if ($withPlugin) {
            Publish-PluginRepository -AsStep -Live "$web/plugin" -Version $repo.Version -IndexPath 'pluginmaster.json' -NoBackup:$NoBackup
        }
    )
    Write-Step "Deploying on the host ($($targets -join ', '))..."
    try {
        Invoke-DeployBundle -Archive 'senpan-deploy.tgz' -Sha256 $archive.Sha256 -ExpectedFileCount $archive.FileCount -Step $steps
    } catch {
        if ($targets.Count -gt 1) {
            throw "$($_.Exception.Message). With several targets, the ones before the failing step are live (the lines above say which) and were not smoke-checked."
        }
        throw
    }

    # -- Smoke check, through Cloudflare -----------------------------------------
    $checks = @(
        if ($withBackend) {
            if ($backendVersion) { @{ Url = "$site/api/version"; Contains = "`"$backendVersion`"" } } else { @{ Url = "$site/api/version" } }
        }
        if ($withFrontend) {
            if ($entryScript) {
                @{ Url = "$site/"; Contains = $entryScript }
                @{ Url = "$site$entryScript" }
            } else {
                @{ Url = "$site/"; Contains = 'id="app"' }
            }
        }
        if ($withPlugin) { @{ Url = "$site/plugin/pluginmaster.json"; Contains = "latest.zip?v=$($repo.Version)" } }
    )
    Write-Step "Checking $site..."
    if ($NoBackup) {
        # Nothing was kept to go back to.
        Invoke-SmokeCheck -Check $checks -Attempts 3 -DelaySeconds 2 -NoRollback
    } else {
        # The reverse of the deploy: the old SPA works against the new API, not the other way.
        $rollback = @(
            if ($withPlugin) { Restore-RemoteDirectory -Live "$web/plugin" -AsScript }
            if ($withFrontend) { Restore-RemoteDirectory -Live "$web/dist" -File "$web/.htaccess" -AsScript }
            # The previous binary with the snapshot taken just before this deploy's start, since
            # the new binary may have migrated the schema. What was written in the seconds the
            # new release ran is lost with it (decided 2026-11-06).
            if ($withBackend) { Restore-ServiceBinary -ServiceName $service -Binary $remoteBinary -Database $db -AsScript }
        )
        Invoke-SmokeCheck -Check $checks -Attempts 3 -DelaySeconds 2 -Rollback {
            Invoke-RemoteStep -Script $rollback -FailureMessage 'Rolling back failed on the host'
        }
    }

    # -- The data, once the release stays (backend) ------------------------------
    if ($withBackend) {
        # The daily copies go in a backups folder beside the database, named
        # <service>-YYYY-MM-DD.sqlite: apart from the deploy's own <db>.pre-<time> snapshots,
        # whose pruning (the newest three <db>.pre-*) can never match them, and the backup's
        # pruning touches only its own dated names.
        Write-Step 'Checking the daily database backup...'
        try {
            if ($found['dynamic'] -eq 'yes') {
                throw "$service.service runs with DynamicUser=yes, so there is no fixed user for the backup to run as."
            }
            $backupName = ($service -replace '\.service$', '').ToLowerInvariant() -replace '[^a-z0-9-]', '-' -replace '^-+', ''
            Install-BackupTimer -Name $backupName -Database $db -Owner $serviceOwner -Description 'Senpan App Suite daily database backup'
        } catch {
            throw "Senpan is deployed and live, but its daily database backup is not: $($_.Exception.Message)"
        }

        if (-not $NoDbPull) {
            $null = Copy-LiveDatabase -Database $db -Destination $devDb
        }
    }
} finally {
    Close-DeployConnection
    if ($archive) { Remove-Item -LiteralPath $archive.Path -ErrorAction SilentlyContinue }
    if ($repo) { Remove-Item -LiteralPath $repo.Path -Recurse -ErrorAction SilentlyContinue }
}

Write-Host "`n[OK] Deployed $($targets -join ', ') to $($connection.Target)" -ForegroundColor Green
if (-not $NoBackup) {
    if ($withBackend) { Write-Host "     Previous binary kept at $remoteBinary.old." -ForegroundColor DarkGray }
    if ($withFrontend) { Write-Host "     Previous build kept at $web/dist.old." -ForegroundColor DarkGray }
    if ($withPlugin) { Write-Host "     Previous plugin repo kept at $web/plugin.old." -ForegroundColor DarkGray }
}
if ($withFrontend) {
    Write-Host '     Assets are content-hashed and the PWA auto-updates, so no manual cache bust is needed.' -ForegroundColor DarkGray
}
if ($withPlugin) {
    Write-Host "     Dalamud custom repo URL: $site/plugin/pluginmaster.json (v$($repo.Version))." -ForegroundColor DarkGray
    Write-Host '     Raise AssemblyVersion + LastUpdate in plugins/pluginmaster.json for each release; a version not higher than the published one is refused.' -ForegroundColor DarkGray
}
