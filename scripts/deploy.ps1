#!/usr/bin/env pwsh
#Requires -Version 7.4

<#
.SYNOPSIS
    Deploy the Senpan App Suite's newest release to the host: the app (one binary, the SPA inside it, with the service's unit and the Apache vhost), or the Dalamud plugin.

.DESCRIPTION
    Built on the SharedDeploy module (shared/deploy/pwsh): one archive, one held SSH
    connection, one remote run.

    What ships is the newest release CI tagged on main (the tags are fetched first): the
    app's newest v<version>, or the plugin's newest Plugin-v<version>, checked out into a
    folder of its own and built there, with npm ci from its lockfile. Commits after it, a
    submodule update or work not yet released, are not part of it, and neither is
    anything uncommitted. The SPA and the API are one app: one version, one binary, which
    embeds the SPA's build and serves the data folder's public files.

    app (the default), on one SSH connection held for the whole run:
      1. When the host already runs this release, nothing is done (-Force deploys it
         again). Otherwise the host is read, before anything is built or changed: the
         service's account and data folder are there, its env file is root's alone, and the
         unit does not pass -webroot. A unit that does keeps the app's files in a web root,
         and the unit this deploy installs looks for them in the data folder. A drop-in on
         the unit is named, since it overrides the template.
      2. Build the release: the front end into backend/internal/web/dist, then the static
         linux/amd64 binary that embeds it. Pack it with the service's unit
         (deploy/senpan.service) and the Apache vhost (deploy/apache-vhost.conf), their
         placeholders filled from the settings, into one archive, and upload it. A broken
         build changes nothing.
      3. One remote run checks the archive's SHA-256 and file count, then: try the
         vhost on a copy of Apache's config and install it as sites-available/<service>.conf
         where it differs, retiring any other enabled site that serves its names (certbot's
         -le-ssl file, say) and checking it with apachectl configtest, stop the service,
         keep app-suite.old, snapshot the database to <db>.pre-<time>.gz (sqlite3 .backup,
         checked and compressed, newest three kept), install the unit where it differs (the
         previous kept as <service>.service.old beside the binary) and the binary, owned by
         the service's account, start it, and wait for /healthz to answer on the host; if
         it does not, the previous binary, unit and vhost go back with the snapshot, since
         the new binary may have migrated a schema the previous one refuses. Apache reloads
         once it does.
      4. Fetch the site from here, through Cloudflare: /healthz and /api/v1/version naming
         this release, the page naming its entry script, that script, a picture and the
         plugin index. If any fails, the previous binary, unit, vhost and snapshot go back.
         Once they pass, the host records the release.
      5. Check the daily database backup (Install-BackupTimer's shared job, run as the
         service's user at 10:00 UTC: a VACUUM INTO copy in a backups folder beside the
         database, named <service>-YYYY-MM-DD.sqlite.gz, 28 days kept), then pull an online
         snapshot of the live database into devdata/database.sqlite, keeping the three
         previous local copies (skip with -NoDbPull).

    plugin: build the release's plugin from clean and lay out its repository (latest.zip,
    and plugins/pluginmaster.json with ?v=<version> on its download links), refuse a
    version not higher than the published one (the built version must equal
    pluginmaster.json's), swap the plugin folder in the data folder (the previous kept as
    plugin.old), and check the published index names it. The app serves the repository
    with no-store.

    Settings resolve from the parameter, then $env:DEPLOY_*, then the git-ignored
    scripts/deploy.config.ps1 (copy scripts/deploy.config.example.ps1). Nothing about the
    host is in this file or in the unit and vhost templates.

    Needs PuTTY's plink 0.85 or later, with a key that has a passphrase loaded into
    Pageant first: pageant.exe "<KeyPath>". The host needs sqlite3 and curl, and Apache
    with mod_proxy_http, mod_headers and mod_rewrite. The whole run is one SSH connection
    (plus one only if it drops and reconnects), which stays under the droplet's
    `ufw limit ssh`.

.PARAMETER Target
    What to deploy: app (the default: the binary, which serves the API, the SPA and the
    data folder's public files, with the service's unit and the Apache vhost) or plugin
    (the Dalamud custom repo: SenpanCompanionAdmin/latest.zip and pluginmaster.json). task
    deploy runs app, then plugin with -IfNewer; task deploy:backend and deploy:plugin run
    one each.

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

.PARAMETER HostKey
    Optional host key fingerprint (SHA256:...) for the SSH connection. If omitted, resolved
    from $env:DEPLOY_HOST_KEY, then $DeployHostKey; without one plink uses its cached key.

.PARAMETER ServiceName
    The app's systemd service. If omitted, resolved from $env:DEPLOY_SERVICE_NAME, then
    $DeployServiceName.

.PARAMETER ServiceUser
    The account the service runs as: the unit's User= and Group=, and the owner of the
    binary and the plugin repository. Never root. If omitted, resolved from
    $env:DEPLOY_SERVICE_USER, then $DeployServiceUser.

.PARAMETER RemoteOptDir
    Directory on the host holding the binary (app-suite). Its data folder, with the
    database, the uploads and the plugin repository, is <RemoteOptDir>/data. If omitted,
    resolved from $env:DEPLOY_REMOTE_OPT_DIR, then $DeployRemoteOptDir.

.PARAMETER EnvFile
    The env file holding the service's secrets, root's alone, which the unit loads. If
    omitted, resolved from $env:DEPLOY_ENV_FILE, then $DeployEnvFile, then
    /etc/<service>/<service>.env.

.PARAMETER AppPort
    The port the app listens on, on the loopback address: the unit's -addr, the vhost's
    proxy and the health check. If omitted, resolved from $env:DEPLOY_APP_PORT, then
    $DeployAppPort, then 8080.

.PARAMETER SiteUrl
    The site's public address, fetched after the deploy as visitors reach it. If omitted,
    resolved from $env:DEPLOY_SITE_URL, then $DeploySiteUrl, then https://apps.senpan.cafe
    (the address plugins/pluginmaster.json already publishes).

.PARAMETER CertName
    The certificate the vhost uses: its folder's name under /etc/letsencrypt/live, one
    that covers all three of the vhost's names. Needed for the app, not the plugin. If
    omitted, resolved from $env:DEPLOY_CERT_NAME, then $DeployCertName.

.PARAMETER ConfigPath
    The untracked settings file; by default scripts/deploy.config.ps1. It need not exist.

.PARAMETER NoBackup
    Remove the rollback copies (app-suite.old, the previous unit and vhost, plugin.old)
    once the run succeeds on the host. The smoke check then has nothing to go back to, so a
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
    [string]$HostKey,
    [string]$ServiceName,
    [string]$ServiceUser,
    [string]$RemoteOptDir,
    [string]$AppPort,
    [string]$EnvFile,
    [string]$SiteUrl,
    [string]$CertName,
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

# A path the remote scripts take literally: absolute, and nothing a shell or sed reads
# as anything but itself.
function Test-RemotePath {
    [CmdletBinding()]
    [OutputType([bool])]
    param([string]$Path)
    $Path -match '^/[A-Za-z0-9._/@+-]*[A-Za-z0-9._@+-]$' -and $Path -notmatch '(^|/)\.\.?(/|$)'
}

$settings = Resolve-DeploySetting -ConfigPath $ConfigPath -Value @{
    VpsHost      = $VpsHost
    VpsUser      = $VpsUser
    KeyPath      = $KeyPath
    HostKey      = $HostKey
    ServiceName  = $ServiceName
    ServiceUser  = $ServiceUser
    RemoteOptDir = $RemoteOptDir
    AppPort      = $AppPort
    EnvFile      = $EnvFile
    SiteUrl      = $SiteUrl
    CertName     = $CertName
} -Required (@('VpsHost', 'VpsUser', 'KeyPath', 'ServiceName', 'ServiceUser', 'RemoteOptDir') + @(if ($Target -eq 'app') { 'CertName' })) -Default @{
    # Public already (plugins/pluginmaster.json's download links), so not a host secret.
    SiteUrl = 'https://apps.senpan.cafe'
    # The app's own default port (backend/main.go).
    AppPort = '8080'
}

$service = $settings.ServiceName
if ($service -notmatch '^[A-Za-z0-9_.@-]+$') { throw "ServiceName '$service' is not a systemd service name." }
$serviceUser = [string]$settings.ServiceUser
if ($serviceUser -notmatch '^[a-z_][a-z0-9_-]*$' -or $serviceUser -eq 'root') {
    throw "ServiceUser must be the service's own unprivileged account, such as myapp, not '$serviceUser'."
}
$optDir = $settings.RemoteOptDir.TrimEnd('/')
if (-not (Test-RemotePath -Path $optDir)) { throw "RemoteOptDir '$optDir' is not a plain absolute path." }
$port = [string]$settings.AppPort
if ($port -notmatch '^\d{4,5}$' -or [int]$port -lt 1024 -or [int]$port -gt 65535) {
    throw "AppPort must be a port from 1024 to 65535, not '$port'."
}
$site = $settings.SiteUrl.TrimEnd('/')
if ($site -notmatch '^https?://[^/\s]+$') {
    throw "SiteUrl must be the site's address alone, such as https://example.com, not '$($settings.SiteUrl)'."
}
$certName = [string]$settings.CertName
if ($Target -eq 'app' -and $certName -notmatch '^[A-Za-z0-9][A-Za-z0-9.-]*$') {
    throw "CertName must be a certificate's folder name under /etc/letsencrypt/live, such as example.com, not '$certName'."
}

# The service as the host holds it. The app listens on the loopback only, the one address
# Apache proxies to and the health check asks.
$addr = "127.0.0.1:$port"
$data = "$optDir/data"
$db = "$data/database.sqlite"
$remoteBinary = "$optDir/app-suite"
$unitFile = "$service.service"
$serviceOwner = "${serviceUser}:$serviceUser"
# The service's secrets, read by the unit, root's alone.
$envFile = if ($settings.EnvFile) { [string]$settings.EnvFile } else { "/etc/$service/$service.env" }
if (-not (Test-RemotePath -Path $envFile)) { throw "The env file '$envFile' is not a plain absolute path; check ServiceName and EnvFile." }
$repoRoot = Split-Path -Path $PSScriptRoot -Parent
# The dev database the pull refreshes; git-ignored, so this checkout's, never the release's.
$devDb = Join-Path $repoRoot 'devdata\database.sqlite'
# What the host records as running, for the next deploy to compare with.
$record = 'senpan'

# Writes a tracked template with its placeholders filled in, for the archive. Every
# placeholder given must be in the template and none may be left over, so a renamed or
# new one cannot reach the host unfilled.
function Expand-DeployTemplate {
    [CmdletBinding()]
    [OutputType([string])]
    param([string]$Template, [string]$Destination, [hashtable]$Value)
    $name = Split-Path -Path $Template -Leaf
    $text = Get-Content -LiteralPath $Template -Raw
    foreach ($key in $Value.Keys) {
        if (-not $text.Contains("__${key}__")) { throw "deploy/$name has no __${key}__ placeholder; nothing was changed." }
    }
    $filled = Format-RemoteScript -Template $text -Value $Value
    $left = [regex]::Match($filled, '__[A-Z][A-Z0-9]*__')
    if ($left.Success) { throw "deploy/$name has a placeholder, $($left.Value), that nothing fills; nothing was changed." }
    New-Item -ItemType Directory -Force -Path (Split-Path -Path $Destination -Parent) | Out-Null
    [System.IO.File]::WriteAllText($Destination, $filled)
    $Destination
}

# What the host must hold before anything is built or changed, asked in one run; a check
# that fails ends the run with its reason. With -ForApp the env file is checked too. The
# answer is the host's key=value lines. Only the env file's variable NAMES come back,
# never a value.
function Read-ServiceHost {
    [CmdletBinding()]
    [OutputType([hashtable])]
    param([switch]$ForApp)
    $everyRun = @'
svc='__SERVICE__'
say() { printf '%s\n' "$*" >&2; }
# A unit that passes -webroot keeps the app's files in a web root, and the unit this deploy
# installs looks for them in the data folder.
if systemctl show "$svc.service" -p ExecStart --value 2>/dev/null | grep -Eq -- '[[:space:]]--?webroot([[:space:]=]|$)'; then
  say "$svc.service still passes -webroot, so the app's files are in a web root and not in __DATA__, where the unit this deploy installs looks for them. 5.2.0's deploy moved them (docs/ops.md, Upgrading to 5.2.0). Nothing was changed."
  exit 3
fi
if [ ! -d '__DATA__' ]; then
  say "__DATA__ is not on the host; create the data folder first (docs/ops.md, One-time server setup). Nothing was changed."
  exit 3
fi
if ! getent passwd '__USER__' >/dev/null; then
  say "The account __USER__ is not on the host; create it first (docs/ops.md, One-time server setup). Nothing was changed."
  exit 3
fi
echo "dropins=$(systemctl show "$svc.service" -p DropInPaths --value 2>/dev/null)"
'@
    $appOnly = @'
envfile='__ENVFILE__'
if [ ! -f "$envfile" ]; then
  say "$envfile is not on the host, and the unit reads the service's secrets from it (docs/ops.md, Upgrading to 5.3.0). Nothing was changed."
  exit 3
fi
have=$(stat -c '%U:%G %a' "$envfile")
if [ "$have" != 'root:root 600' ]; then
  say "$envfile is $have and holds secrets; make it root:root 600 with: chown root:root '$envfile' && chmod 600 '$envfile'. Nothing was changed."
  exit 3
fi
echo "env_names=$(sed -n 's/^\([A-Za-z_][A-Za-z0-9_]*\)=.*/\1/p' "$envfile" | tr '\n' ' ')"
'@
    $template = (@($everyRun) + @(if ($ForApp) { $appOnly })) -join "`n"
    $read = Format-RemoteScript -Value @{ SERVICE = $service; USER = $serviceUser; DATA = $data; ENVFILE = $envFile } -Template $template
    $found = @{}
    foreach ($line in @(Invoke-Remote -Script $read)) {
        if ($line -match '^([a-z_]+)=(.*)$') { $found[$Matches[1]] = $Matches[2].Trim() }
    }
    $found
}

$release = if ($Target -eq 'plugin') {
    Get-DeployRelease -Path $repoRoot -Match 'Plugin-v[0-9]*'
} else {
    Get-DeployRelease -Path $repoRoot
}
$version = $release.Tag
# The version the app reports (GET /api/v1/version, /healthz): the tag without its v.
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
        $repo = Export-PluginRepository -Manifest (Join-Path $built 'SenpanCompanionAdmin.json') `
            -IndexPath 'pluginmaster.json' -IndexSource (Join-Path $tree 'plugins\pluginmaster.json') -File ([ordered]@{
                'SenpanCompanionAdmin/latest.zip' = Join-Path $built 'latest.zip'
            })
        $archive = Compress-DeployArchive -Content ([ordered]@{ 'plugin' = $repo.Path })
        Write-Ok ('{0} files, {1:N1} MB, one archive' -f $archive.FileCount, ($archive.Size / 1MB))

        $connection = $settings | Open-DeployConnection
        # The repository is the data folder's plugin/, which the app serves.
        $null = Read-ServiceHost

        Write-Step 'Uploading...'
        $upload = Invoke-Upload -Path $archive.Path -Destination 'senpan-plugin.tgz' -Activity "Uploading Senpan Companion $($repo.Version)" -FileCount $archive.FileCount
        if (-not $upload.Ok) { throw "Upload failed; the published plugin is untouched.`n$($upload.Err)" }

        Write-Step "Publishing Senpan Companion $($repo.Version) to $data/plugin..."
        Invoke-DeployBundle -Archive 'senpan-plugin.tgz' -Sha256 $archive.Sha256 -ExpectedFileCount $archive.FileCount -Step @(
            Publish-PluginRepository -AsStep -Live "$data/plugin" -Version $repo.Version -IndexPath 'pluginmaster.json' -Owner $serviceOwner -NoBackup:$NoBackup
        )
        $checks = @(@{ Url = "$site/plugin/pluginmaster.json"; Contains = "latest.zip?v=$($repo.Version)" })
        Write-Step "Checking $site..."
        if ($NoBackup) {
            Invoke-SmokeCheck -Check $checks -Attempts 3 -DelaySeconds 2 -NoRollback
        } else {
            Invoke-SmokeCheck -Check $checks -Attempts 3 -DelaySeconds 2 -Rollback {
                Restore-RemoteDirectory -Live "$data/plugin"
            }
        }
    } finally {
        Close-DeployConnection
        if ($archive) { Remove-Item -LiteralPath $archive.Path -ErrorAction SilentlyContinue }
        if ($repo) { Remove-Item -LiteralPath $repo.Path -Recurse -ErrorAction SilentlyContinue }
        if ($tree) { Remove-ReleaseTree -Path $tree -Repository $repoRoot }
    }
    Write-Host "`n[OK] Published Senpan Companion $($repo.Version) to $($connection.Target)" -ForegroundColor Green
    if (-not $NoBackup) { Write-Host "     Previous plugin repo kept at $data/plugin.old." -ForegroundColor DarkGray }
    Write-Host "     Dalamud custom repo URL: $site/plugin/pluginmaster.json (v$($repo.Version))." -ForegroundColor DarkGray
    Write-Host '     Raise AssemblyVersion + LastUpdate in plugins/pluginmaster.json for each release; a version not higher than the published one is refused.' -ForegroundColor DarkGray
    return
}

# -- The app ------------------------------------------------------------------------
try {
    $connection = $settings | Open-DeployConnection

    # -- The release the host took last ----------------------------------------------
    $live = Get-DeployedRelease -Name $record
    if ($live -eq $version -and -not $Force) {
        Write-Ok "$version is already running on the host; nothing to deploy. (A version pushed since goes out once CI tags it; -Force deploys this one again.)"
        return
    }
    Write-Step "Releasing $version (the host runs $(if ($live) { $live } else { 'a release not recorded' }))"

    # -- The host, before anything is built or changed --------------------------------
    Write-Step "Checking $service on the host..."
    $found = Read-ServiceHost -ForApp
    $names = @($found['env_names'] -split '\s+' | Where-Object { $_ })
    Write-Ok "$service runs as $serviceOwner, listens on $addr and keeps its files in $data; $envFile holds $(if ($names) { $names -join ', ' } else { 'no variables' })."
    if ($names -notcontains 'APPSUITE_TURNSTILE_SECRET') {
        Write-Warning "$envFile sets no APPSUITE_TURNSTILE_SECRET, so the login bot check is off."
    }
    if ($found['dropins']) {
        Write-Warning "$service.service has a drop-in ($($found['dropins'])), which overrides the unit this deploy installs. Remove it so the host runs what the repo says."
    }

    # -- Build the release, in its own checkout --------------------------------------
    $tree = New-ReleaseTree -Path $repoRoot -Tag $version -Npm 'frontend'
    # The front end first: it builds into backend/internal/web/dist, which the binary embeds.
    $shell = Join-Path $tree 'backend\internal\web\dist\index.html'
    Invoke-NpmBuild -Path (Join-Path $tree 'frontend') -Output $shell
    $binary = Invoke-GoBuild -Path (Join-Path $tree 'backend') -Output 'app-suite'
    # The build's entry script: the smoke check wants the page to name it, and to fetch it.
    $entryScript = [regex]::Match((Get-Content -LiteralPath $shell -Raw), 'src="(/assets/[^"]+\.js)"').Groups[1].Value
    if (-not $entryScript) { throw 'The built index.html names no /assets/*.js entry script; the smoke check would have nothing to fetch.' }

    # The unit and the vhost, with the host's values filled in.
    $hostDir = Join-Path $tree 'deploy\host'
    $unit = Expand-DeployTemplate -Template (Join-Path $tree 'deploy\senpan.service') -Destination (Join-Path $hostDir $unitFile) -Value @{
        SERVICE = $service; USER = $serviceUser; OPT = $optDir; ADDR = $addr; ENVFILE = $envFile
    }
    $vhost = Expand-DeployTemplate -Template (Join-Path $tree 'deploy\apache-vhost.conf') -Destination (Join-Path $hostDir 'apache-vhost.conf') -Value @{
        APP = $addr; CERT = $certName
    }

    # -- Pack the deploy -----------------------------------------------------------
    Write-Step 'Packing the deploy...'
    $archive = Compress-DeployArchive -Content ([ordered]@{ 'app-suite' = $binary; $unitFile = $unit; 'apache-vhost.conf' = $vhost })
    Write-Ok ('{0} files, {1:N1} MB, one archive' -f $archive.FileCount, ($archive.Size / 1MB))

    # The database snapshot is the way back from a migration: starting the new binary
    # migrates the live database, and the previous binary refuses a schema newer than its
    # own, so a rollback puts the snapshot back with the binary. -Umask 077 keeps the
    # snapshot (live accounts, tokens, webhooks) for root. The binary belongs to the
    # service's own account, not the SSH user's.
    $install = @{
        ServiceName               = $service
        Binary                    = $remoteBinary
        Owner                     = $serviceOwner
        Unit                      = $unitFile
        Database                  = $db
        Umask                     = '077'
        RestoreDatabaseOnRollback = $true
        HealthUrl                 = "http://$addr/healthz"
        Vhost                     = 'apache-vhost.conf'
        NoBackup                  = $NoBackup
    }
    $steps = @(Install-ServiceBinary -AsStep @install)
    # The smoke check's way back, built now so a mistake in it shows before anything changes.
    $rollbackScript = @(
        Restore-ServiceBinary -ServiceName $service -Binary $remoteBinary -Owner $serviceOwner -Unit $unitFile -Database $db -AsScript
    )

    # -- Upload, then one remote run ---------------------------------------------
    Write-Step 'Uploading...'
    $upload = Invoke-Upload -Path $archive.Path -Destination 'senpan-deploy.tgz' -Activity "Uploading Senpan App Suite $version" -FileCount $archive.FileCount
    if (-not $upload.Ok) { throw "Upload failed; nothing on the host changed.`n$($upload.Err)" }
    Write-Ok ('Uploaded in {0:mm\:ss}.' -f $upload.Elapsed)
    Write-Step "Deploying $version on the host..."
    Invoke-DeployBundle -Archive 'senpan-deploy.tgz' -Sha256 $archive.Sha256 -ExpectedFileCount $archive.FileCount -Step $steps

    # -- Smoke check, through Cloudflare -----------------------------------------
    # The reverse of the deploy: the previous binary, unit and vhost with the snapshot taken
    # just before this deploy's start, since the new binary may have migrated the schema.
    # What was written in the seconds the new release ran is lost with it.
    $rollback = { Invoke-RemoteStep -Script $rollbackScript -FailureMessage 'Rolling back failed on the host' }

    # A fresh token on each address, so no answer Cloudflare kept from before can pass.
    $token = [guid]::NewGuid().ToString('N').Substring(0, 12)
    $checks = @(
        @{ Url = "$site/healthz?deploy=$token"; Contains = "`"version`":`"$appVersion`"" }
        @{ Url = "$site/api/v1/version?deploy=$token"; Contains = "`"$appVersion`"" }
        @{ Url = "$site/?deploy=$token"; Contains = $entryScript }
        @{ Url = "$site$entryScript" }
        # The data folder's files, which the app serves itself.
        @{ Url = "$site/images/pwa-512x512.png?deploy=$token" }
        @{ Url = "$site/plugin/pluginmaster.json?deploy=$token"; Contains = '"AssemblyVersion"' }
    )
    Write-Step "Checking $site..."
    if ($NoBackup) {
        # Nothing was kept to go back to.
        Invoke-SmokeCheck -Check $checks -Attempts 3 -DelaySeconds 2 -NoRollback
    } else {
        Invoke-SmokeCheck -Check $checks -Attempts 3 -DelaySeconds 2 -Rollback $rollback
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
    Write-Host "     Previous binary kept at $remoteBinary.old, and the unit and vhost before, if they changed, at $optDir/$unitFile.old and /etc/apache2/sites-available/$service.conf.old." -ForegroundColor DarkGray
}
Write-Host '     Assets are content-hashed and the PWA auto-updates, so no manual cache bust is needed.' -ForegroundColor DarkGray
