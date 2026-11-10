#!/usr/bin/env pwsh
#Requires -Version 7.4

<#
.SYNOPSIS
    Deploy the Senpan App Suite's newest release to the host: the app (one binary, the SPA inside it), or the Dalamud plugin.

.DESCRIPTION
    Built on shared-tooling's SharedDeploy module (shared/deploy/pwsh), the shape every
    deploy follows: one archive, one held SSH connection, one remote run.

    What ships is the newest release CI tagged on main (the tags are fetched first): the
    app's newest v<version>, or the plugin's newest Plugin-v<version>, checked out into a
    folder of its own and built there, with npm ci from its lockfile. Commits after it, a
    shared-tooling update or work not yet released, are not part of it, and neither is
    anything uncommitted. Since 4.0.0 the SPA and the API are one app: one version, one
    binary, which embeds the SPA's build.

    app (the default), on one SSH connection held for the whole run:
      1. Read the service from the host - the user it runs as, and the database (-db) and
         listen address (-addr) on its ExecStart - and the release the host last took.
         When that is this release, nothing is done (-Force deploys it again). Nothing
         changes if the unit is missing or names a different database than RemoteDbPath.
      2. Build the release: the front end into backend/internal/web/dist, then the static
         linux/amd64 binary that embeds it. Pack it with the web root's .htaccess
         (deploy/.htaccess, with the app's address filled in) into one archive, and
         upload it. A broken build changes nothing.
      3. One remote run checks the archive's SHA-256 and file count, then: stop the
         service, keep app-suite.old, snapshot the database to <db>.pre-<time>.gz
         (sqlite3 .backup, checked and compressed, newest three kept), install the binary,
         owned by the account the unit runs as, start it, and wait for /healthz
         to answer on the host; if it does not, the previous binary goes back with the
         snapshot, since the new one may have migrated a schema the old one refuses. Then
         swap the web root's .htaccess in, keeping the previous in .deploy-backup: it
         serves the web root's own folders and proxies the rest to the app.
      4. Fetch the site from here, through Cloudflare: /healthz and /api/version naming
         this release, the page naming its entry script, and that script. If any fails,
         the previous .htaccess goes back, then the previous binary with the snapshot.
         Once they pass, the host records the release.
      5. Check the daily database backup (Install-BackupTimer's shared job, run as the
         service's user at 10:00 UTC: a VACUUM INTO copy in a backups folder beside the
         database, named <service>-YYYY-MM-DD.sqlite.gz, 28 days kept), then pull an online
         snapshot of the live database into devdata/database.sqlite, keeping the three
         previous local copies (skip with -NoDbPull).

    plugin: build the release's plugin from clean and lay out its repository (latest.zip,
    plugins/pluginmaster.json with ?v=<version> on its download links, and
    deploy/plugin.htaccess), refuse a version not higher than the published one (the
    built version must equal pluginmaster.json's), swap the plugin folder in on the host
    (the previous kept as plugin.old), and check the published index names it.

    Settings resolve from the parameter, then $env:DEPLOY_*, then the git-ignored
    scripts/deploy.config.ps1 (copy scripts/deploy.config.example.ps1). Nothing about the
    host is in this file.

    Needs PuTTY's plink 0.85 or later (pscp is no longer used), with a key that has a
    passphrase loaded into Pageant first: pageant.exe "<KeyPath>". The host needs sqlite3
    and curl, and Apache's mod_proxy_http (the vhost's ProxyPass already uses it). The
    whole run is one SSH connection (plus one only if it drops and reconnects), which
    stays under the droplet's `ufw limit ssh`.

.PARAMETER Target
    What to deploy: app (the default: the binary, which serves the API and the SPA, with
    the web root's .htaccess) or plugin (the Dalamud custom repo:
    SenpanCompanionAdmin/latest.zip, pluginmaster.json and plugin.htaccess). task deploy
    runs app, then plugin with -IfNewer; task deploy:backend and deploy:plugin run one each.

.PARAMETER IfNewer
    With -Target plugin: a plugin whose version is already the published one is left
    alone and the run ends without error, where without it the run refuses. A lower
    version still refuses. task deploy passes it, so a deploy that didn't raise the
    plugin's version still succeeds.

.PARAMETER Force
    With -Target app: deploy the release even when the host already runs it.

.PARAMETER VpsHost
    Droplet IP or hostname. If omitted, resolved from $env:DEPLOY_VPS_HOST, then
    scripts/deploy.config.ps1 ($DeployVpsHost).

.PARAMETER VpsUser
    SSH user. If omitted, resolved from $env:DEPLOY_VPS_USER, then $DeployVpsUser.

.PARAMETER KeyPath
    Path to the PuTTY .ppk private key. If omitted, resolved from $env:DEPLOY_KEY_PATH,
    then $DeployKeyPath.

.PARAMETER WebRoot
    Apache DocumentRoot on the host (holds .htaccess, images, fonts, carrd and plugin). If
    omitted, resolved from $env:DEPLOY_WEB_ROOT, then $DeployWebRoot.

.PARAMETER ServiceName
    The app's systemd service. Its unit lives only on the host; this script never ships
    it. If omitted, resolved from $env:DEPLOY_SERVICE_NAME, then $DeployServiceName.

.PARAMETER RemoteOptDir
    Directory on the host holding the binary (app-suite). If omitted, resolved from
    $env:DEPLOY_REMOTE_OPT_DIR, then $DeployRemoteOptDir.

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

.PARAMETER NoBackup
    Remove the rollback copies (app-suite.old, the previous .htaccess, plugin.old) once
    the run succeeds on the host. The smoke check then has nothing to go back to, so a
    failure only stops the run. The database snapshot is still taken.

.PARAMETER NoDbPull
    Skip refreshing the local dev DB (devdata/database.sqlite) from live after an app
    deploy.

.EXAMPLE
    .\scripts\deploy.ps1                         # the app's newest release

.EXAMPLE
    .\scripts\deploy.ps1 -Force                  # the release again, though the host runs it

.EXAMPLE
    .\scripts\deploy.ps1 -NoDbPull               # don't refresh the dev DB

.EXAMPLE
    .\scripts\deploy.ps1 -Target plugin          # build + publish the Dalamud plugin repo
#>
[CmdletBinding()]
param(
    [ValidateSet('app', 'plugin')]
    [string]$Target = 'app',

    # Server-specific settings are NOT in this tracked script. Each resolves from its
    # -param, then $env:DEPLOY_*, then the untracked scripts/deploy.config.ps1.
    [string]$VpsHost,
    [string]$VpsUser,
    [string]$KeyPath,
    [string]$WebRoot,
    [string]$ServiceName,
    [string]$RemoteOptDir,
    [string]$RemoteDbPath,
    [string]$SiteUrl,
    [string]$ConfigPath = (Join-Path -Path $PSScriptRoot -ChildPath 'deploy.config.ps1'),

    [switch]$NoBackup,
    [switch]$NoDbPull,
    [switch]$IfNewer,
    [switch]$Force
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

$repoRoot = Split-Path -Path $PSScriptRoot -Parent
# The dev database the pull refreshes; git-ignored, so this checkout's, never the release's.
$devDb = Join-Path $repoRoot 'devdata\database.sqlite'
# What the host records as running, for the next deploy to compare with.
$record = 'senpan'

# Only a release ships, as CI tagged it on main: the app's v<version>, the plugin's
# Plugin-v<version> (.github/scripts/release.sh). Each is tagged when its own version
# rises, so a plugin release need not have an app release beside it.
$release = if ($Target -eq 'plugin') {
    Get-DeployRelease -Path $repoRoot -Match 'Plugin-v[0-9]*'
} else {
    Get-DeployRelease -Path $repoRoot
}
$version = $release.Tag
# The version the app reports (GET /api/version, /healthz): the tag without its v.
$appVersion = $version -replace '^v', ''

Write-Host "Deploy target: $Target, release $version -> $($settings.VpsUser)@$($settings.VpsHost)" -ForegroundColor Yellow
if ($release.Since) { Write-Step "The $($release.Since) commit(s) on $($release.Branch) after $version are not part of it." }

$tree = $null
$archive = $null
$repo = $null
$connection = $null

if ($Target -eq 'plugin') {
    try {
        $tree = New-ReleaseTree -Path $repoRoot -Tag $version
        $pluginDir = Join-Path $tree 'plugins\SenpanCompanion'

        # The plugin's version must be higher than the published one, since Dalamud
        # updates to nothing else. The host refuses a lower one too, but the published
        # index is read here first, before the plugin is built. If it can't be read, the
        # host's check still stands.
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
                Write-Ok "Plugin $planned is already published; nothing to publish."
                return
            }
            throw "The plugin's version $planned is not higher than the published $published, and Dalamud updates to nothing else. Raise <Version> in plugins/SenpanCompanion/SenpanCompanion.csproj (and plugins/pluginmaster.json); nothing was built or changed."
        }

        Invoke-PluginBuild -Project (Join-Path $pluginDir 'SenpanCompanion.csproj')
        $built = Join-Path $pluginDir 'bin\Release\SenpanCompanionAdmin'
        # pluginmaster.json is the repo index; its AssemblyVersion must equal the built one.
        # plugin.htaccess keeps Cloudflare from edge-caching the package (see the file).
        $repo = Export-PluginRepository -Manifest (Join-Path $built 'SenpanCompanionAdmin.json') `
            -IndexPath 'pluginmaster.json' -IndexSource (Join-Path $tree 'plugins\pluginmaster.json') -File ([ordered]@{
                'SenpanCompanionAdmin/latest.zip' = Join-Path $built 'latest.zip'
                '.htaccess'                       = Join-Path $tree 'deploy\plugin.htaccess'
            })
        $archive = Compress-DeployArchive -Content ([ordered]@{ 'plugin' = $repo.Path })
        Write-Ok ('{0} files, {1:N1} MB, one archive' -f $archive.FileCount, ($archive.Size / 1MB))

        $connection = $settings | Open-DeployConnection
        Write-Step 'Uploading...'
        $upload = Invoke-Upload -Path $archive.Path -Destination 'senpan-plugin.tgz' -Activity "Uploading Senpan Companion $($repo.Version)" -FileCount $archive.FileCount
        if (-not $upload.Ok) { throw "Upload failed; the published plugin is untouched.`n$($upload.Err)" }

        Write-Step "Publishing Senpan Companion $($repo.Version)..."
        Invoke-DeployBundle -Archive 'senpan-plugin.tgz' -Sha256 $archive.Sha256 -ExpectedFileCount $archive.FileCount -Step @(
            Publish-PluginRepository -AsStep -Live "$web/plugin" -Version $repo.Version -IndexPath 'pluginmaster.json' -NoBackup:$NoBackup
        )
        $checks = @(@{ Url = "$site/plugin/pluginmaster.json"; Contains = "latest.zip?v=$($repo.Version)" })
        Write-Step "Checking $site..."
        if ($NoBackup) {
            Invoke-SmokeCheck -Check $checks -Attempts 3 -DelaySeconds 2 -NoRollback
        } else {
            Invoke-SmokeCheck -Check $checks -Attempts 3 -DelaySeconds 2 -Rollback {
                Restore-RemoteDirectory -Live "$web/plugin"
            }
        }
    } finally {
        Close-DeployConnection
        if ($archive) { Remove-Item -LiteralPath $archive.Path -ErrorAction SilentlyContinue }
        if ($repo) { Remove-Item -LiteralPath $repo.Path -Recurse -ErrorAction SilentlyContinue }
        if ($tree) { Remove-ReleaseTree -Path $tree -Repository $repoRoot }
    }
    Write-Host "`n[OK] Published Senpan Companion $($repo.Version) to $($connection.Target)" -ForegroundColor Green
    if (-not $NoBackup) { Write-Host "     Previous plugin repo kept at $web/plugin.old." -ForegroundColor DarkGray }
    Write-Host "     Dalamud custom repo URL: $site/plugin/pluginmaster.json (v$($repo.Version))." -ForegroundColor DarkGray
    Write-Host '     Raise AssemblyVersion + LastUpdate in plugins/pluginmaster.json for each release; a version not higher than the published one is refused.' -ForegroundColor DarkGray
    return
}

# -- The app ------------------------------------------------------------------------
try {
    $connection = $settings | Open-DeployConnection

    # -- The service, as the host runs it, and the release it took last -------------
    # The unit lives only on the host, so it is the one place that says where the live
    # database is (-db), where the app listens (-addr) and whom the service runs as.
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

    # Where the app answers on the host: -addr, or the binary's own default (:8080,
    # backend/main.go). An address on every interface is asked on the loopback. The web
    # root's .htaccess proxies to the same address.
    $addr = if ($found['unit_addr']) { $found['unit_addr'] } else { ':8080' }
    if ($addr -match '^(|0\.0\.0\.0|\[::\])(:\d+)$') { $addr = "127.0.0.1$($Matches[2])" }
    if ($addr -notmatch '^[A-Za-z0-9.\[\]:-]+:\d+$') { throw "Could not tell where $service listens from its -addr '$addr'; nothing was changed." }

    $serviceOwner = if ($found['group']) { "$($found['user']):$($found['group'])" } else { $found['user'] }
    $dbOwner = if ($found['db_owner']) { $found['db_owner'] } else { 'not there yet' }
    Write-Ok "Live database: $db (owner $dbOwner); $service runs as $serviceOwner and listens on $addr."
    if ($found['user'] -eq 'root' -and $found['dynamic'] -ne 'yes') {
        Write-Warning "$service runs as root, so its backup will too. deploy/README.md (Running as a non-root user) says how to move it to its own account."
    }

    $live = Get-DeployedRelease -Name $record
    if ($live -eq $version -and -not $Force) {
        Write-Ok "$version is already running on the host; nothing to deploy. (A version pushed since goes out once CI tags it; -Force deploys this one again.)"
        return
    }
    Write-Step "Releasing $version (the host runs $(if ($live) { $live } else { 'a release not recorded' }))"

    # -- Build the release, in its own checkout --------------------------------------
    $tree = New-ReleaseTree -Path $repoRoot -Tag $version -Npm 'frontend'
    # The front end first: it builds into backend/internal/web/dist, which the binary embeds.
    $shell = Join-Path $tree 'backend\internal\web\dist\index.html'
    Invoke-NpmBuild -Path (Join-Path $tree 'frontend') -Output $shell
    $binary = Invoke-GoBuild -Path (Join-Path $tree 'backend') -Output 'app-suite'
    # The build's entry script: the smoke check wants the page to name it, and to fetch it.
    $entryScript = [regex]::Match((Get-Content -LiteralPath $shell -Raw), 'src="(/assets/[^"]+\.js)"').Groups[1].Value
    if (-not $entryScript) { throw 'The built index.html names no /assets/*.js entry script; the smoke check would have nothing to fetch.' }

    # The web root's .htaccess, with the app's address filled in.
    $htaccessDir = Join-Path $tree 'deploy\webroot'
    New-Item -ItemType Directory -Force -Path $htaccessDir | Out-Null
    $htaccess = Join-Path $htaccessDir '.htaccess'
    $template = Get-Content -LiteralPath (Join-Path $tree 'deploy\.htaccess') -Raw
    if (-not $template.Contains('__APP__')) { throw 'deploy/.htaccess has no __APP__ placeholder for the app''s address; nothing was changed.' }
    Set-Content -LiteralPath $htaccess -Value ($template.Replace('__APP__', $addr)) -NoNewline

    Write-Step 'Packing the deploy...'
    $archive = Compress-DeployArchive -Content ([ordered]@{ 'app-suite' = $binary; 'webroot' = $htaccessDir })
    Write-Ok ('{0} files, {1:N1} MB, one archive' -f $archive.FileCount, ($archive.Size / 1MB))

    # -- Upload, then one remote run ---------------------------------------------
    Write-Step 'Uploading...'
    $upload = Invoke-Upload -Path $archive.Path -Destination 'senpan-deploy.tgz' -Activity "Uploading Senpan App Suite $version" -FileCount $archive.FileCount
    if (-not $upload.Ok) { throw "Upload failed; nothing on the host changed.`n$($upload.Err)" }
    Write-Ok ('Uploaded in {0:mm\:ss}.' -f $upload.Elapsed)

    # The binary first, then the .htaccess that sends the SPA's paths to it: until the
    # swap, the previous .htaccess serves the previous SPA from dist/, which runs against
    # this API as it did against the last. Nothing changes until the archive's SHA-256
    # and file count check.
    $steps = @(
        # The database snapshot is the way back from a migration: starting the new binary
        # migrates the live database, and the previous binary refuses a schema newer than
        # its own, so a rollback puts the snapshot back with the binary. -Umask 077
        # keeps the snapshot (live accounts, tokens, webhooks) for root. The binary
        # belongs to the service's own account, not the SSH user's.
        Install-ServiceBinary -AsStep -ServiceName $service -Binary $remoteBinary -Owner $serviceOwner -Database $db -Umask '077' `
            -RestoreDatabaseOnRollback -HealthUrl "http://$addr/healthz" -NoBackup:$NoBackup
        Switch-RemoteItemSet -AsStep -Root $web -Item '.htaccess' -Source 'webroot' -NoBackup:$NoBackup
    )
    Write-Step "Deploying $version on the host..."
    Invoke-DeployBundle -Archive 'senpan-deploy.tgz' -Sha256 $archive.Sha256 -ExpectedFileCount $archive.FileCount -Step $steps

    # -- Smoke check, through Cloudflare -----------------------------------------
    # A fresh token on each address, so no answer Cloudflare kept from before can pass.
    $token = [guid]::NewGuid().ToString('N').Substring(0, 12)
    $checks = @(
        @{ Url = "$site/healthz?deploy=$token"; Contains = "`"version`":`"$appVersion`"" }
        @{ Url = "$site/api/version?deploy=$token"; Contains = "`"$appVersion`"" }
        @{ Url = "$site/?deploy=$token"; Contains = $entryScript }
        @{ Url = "$site$entryScript" }
    )
    Write-Step "Checking $site..."
    if ($NoBackup) {
        # Nothing was kept to go back to.
        Invoke-SmokeCheck -Check $checks -Attempts 3 -DelaySeconds 2 -NoRollback
    } else {
        # The reverse of the deploy: the previous .htaccess, which serves the previous
        # SPA, then the previous binary with the snapshot taken just before this deploy's
        # start, since the new binary may have migrated the schema. What was written in
        # the seconds the new release ran is lost with it.
        $rollback = @(
            Restore-RemoteItemSet -Root $web -Item '.htaccess' -AsScript
            Restore-ServiceBinary -ServiceName $service -Binary $remoteBinary -Owner $serviceOwner -Database $db -AsScript
        )
        Invoke-SmokeCheck -Check $checks -Attempts 3 -DelaySeconds 2 -Rollback {
            Invoke-RemoteStep -Script $rollback -FailureMessage 'Rolling back failed on the host'
        }
    }
    # Only a deploy that stayed is recorded.
    Set-DeployedRelease -Name $record -Release $version

    # -- The data, once the release stays ----------------------------------------
    # The daily copies go in a backups folder beside the database, named
    # <service>-YYYY-MM-DD.sqlite.gz: apart from the deploy's own <db>.pre-<time>.gz snapshots,
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
        throw "Senpan $version is deployed and live, but its daily database backup is not: $($_.Exception.Message)"
    }

    if (-not $NoDbPull) {
        $null = Copy-LiveDatabase -Database $db -Destination $devDb
    }
} finally {
    Close-DeployConnection
    if ($archive) { Remove-Item -LiteralPath $archive.Path -ErrorAction SilentlyContinue }
    if ($tree) { Remove-ReleaseTree -Path $tree -Repository $repoRoot }
}

Write-Host "`n[OK] Deployed Senpan App Suite $version to $($connection.Target)" -ForegroundColor Green
if (-not $NoBackup) {
    Write-Host "     Previous binary kept at $remoteBinary.old, the previous .htaccess in $web/.deploy-backup." -ForegroundColor DarkGray
}
Write-Host '     Assets are content-hashed and the PWA auto-updates, so no manual cache bust is needed.' -ForegroundColor DarkGray
