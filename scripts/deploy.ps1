#!/usr/bin/env pwsh
#Requires -Version 7.4

<#
.SYNOPSIS
    Deploy the Senpan App Suite's newest release to the host: the app (one binary, the SPA inside it), or the Dalamud plugin.

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
      1. Read the service from the host - the user it runs as, and the database (-db),
         listen address (-addr) and data folder (-data) on its ExecStart - and the release
         the host last took. When that is this release, nothing is done (-Force deploys it
         again). Nothing changes if the unit is missing or names a different database than
         RemoteDbPath.
      2. Build the release: the front end into backend/internal/web/dist, then the static
         linux/amd64 binary that embeds it. Pack it with the Apache vhost
         (deploy/apache-vhost.conf, with the app's address and CertName filled in) into one
         archive, and upload it. A broken build changes nothing.
      3. One remote run checks the archive's SHA-256 and file count, then: install the
         vhost where it differs and check it with apachectl configtest, stop the service,
         keep app-suite.old, snapshot the database to <db>.pre-<time>.gz (sqlite3 .backup,
         checked and compressed, newest three kept), install the binary, owned by the
         account the unit runs as, start it, and wait for /healthz to answer on the host; if
         it does not, the previous binary and vhost go back with the snapshot, since the new
         binary may have migrated a schema the previous one refuses. Apache reloads once it does.
      4. Fetch the site from here, through Cloudflare: /healthz and /api/v1/version naming
         this release, the page naming its entry script, that script, a picture and the
         plugin index. If any fails, the previous binary, vhost and snapshot go back. Once
         they pass, the host records the release.
      5. Check the daily database backup (Install-BackupTimer's shared job, run as the
         service's user at 10:00 UTC: a VACUUM INTO copy in a backups folder beside the
         database, named <service>-YYYY-MM-DD.sqlite.gz, 28 days kept), then pull an online
         snapshot of the live database into devdata/database.sqlite, keeping the three
         previous local copies (skip with -NoDbPull).

    The move out of the web root. A unit that still passes -webroot (Apache serving the
    folders from a document root) is moved in the same run, while the service is stopped:
      - Before anything stops: the folders to move are there and the data folder
        (<RemoteOptDir>/data) holds none of them; the sites now serving the vhost's names
        are found from apachectl -S and serve no other name; Apache takes the new vhost in
        their place, tried on a copy of its config; and each folder, Apache's config, the
        unit, the web root's .htaccess and its dist/sounds are backed up, compressed and
        checked, to ~/senpan-before-data-move-<time> (root's alone).
      - After the stop: images, fonts, carrd and plugin move whole into the data folder
        (the fonts with their converted copies), the web root's dist/sounds becomes the
        data folder's sounds/ unless it has one, the unit's -webroot becomes -data and
        its ReadWritePaths drop the web root's folders for the data folder, those sites
        are disabled and the vhost enabled, and apachectl configtest must pass. Apache
        reloads once the new release answers.
      - After the start, every name the vhost serves is asked on the host, through Apache,
        for a font file as a plain file: anything but a refusal (400, 403 or 404, redirects
        followed) rolls the run back.
      - Any failure, from the stop to the smoke check, puts everything back: the folders,
        the unit and the sites with it.
      The move's shell travels in the archive (data-move*.sh), and the steps source it, so
      the host run's command stays within what plink's shared connection carries.

    plugin: build the release's plugin from clean and lay out its repository (latest.zip,
    and plugins/pluginmaster.json with ?v=<version> on its download links), refuse a
    version not higher than the published one (the built version must equal
    pluginmaster.json's), swap the plugin folder in the data folder (the previous kept as
    plugin.old), and check the published index names it. The app serves the repository
    with no-store.

    Settings resolve from the parameter, then $env:DEPLOY_*, then the git-ignored
    scripts/deploy.config.ps1 (copy scripts/deploy.config.example.ps1). Nothing about the
    host is in this file.

    Needs PuTTY's plink 0.85 or later, with a key that has a passphrase loaded into
    Pageant first: pageant.exe "<KeyPath>". The host needs sqlite3 and curl, and Apache
    with mod_proxy_http, mod_headers and mod_rewrite. The whole run is one SSH connection
    (plus one only if it drops and reconnects), which stays under the droplet's
    `ufw limit ssh`.

.PARAMETER Target
    What to deploy: app (the default: the binary, which serves the API, the SPA and the
    data folder's public files, with the Apache vhost) or plugin (the Dalamud custom repo:
    SenpanCompanionAdmin/latest.zip and pluginmaster.json). task deploy runs app, then
    plugin with -IfNewer; task deploy:backend and deploy:plugin run one each.

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

.PARAMETER ServiceName
    The app's systemd service. Its unit lives only on the host; this script never ships
    it. If omitted, resolved from $env:DEPLOY_SERVICE_NAME, then $DeployServiceName.

.PARAMETER RemoteOptDir
    Directory on the host holding the binary (app-suite). Its data folder, where a unit
    that still passes -webroot has its files moved, is <RemoteOptDir>/data. If omitted,
    resolved from $env:DEPLOY_REMOTE_OPT_DIR, then $DeployRemoteOptDir.

.PARAMETER RemoteDbPath
    The live SQLite database on the host. Normally left unset: the deploy reads it from
    the -db on the service's ExecStart. When set (here, $env:DEPLOY_REMOTE_DB_PATH or
    $DeployRemoteDbPath), it must match that path, or the deploy stops before anything
    changes; it is used on its own only when the unit names no -db.

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
    Remove the rollback copies (app-suite.old, the previous vhost, plugin.old) once the
    run succeeds on the host. The smoke check then has nothing to go back to, so a failure
    only stops the run. The database snapshot is still taken. A run that moves the files
    out of the web root refuses it, since that move is undone through the previous release.

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
    [string]$ServiceName,
    [string]$RemoteOptDir,
    [string]$RemoteDbPath,
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

$settings = Resolve-DeploySetting -ConfigPath $ConfigPath -Value @{
    VpsHost      = $VpsHost
    VpsUser      = $VpsUser
    KeyPath      = $KeyPath
    ServiceName  = $ServiceName
    RemoteOptDir = $RemoteOptDir
    RemoteDbPath = $RemoteDbPath
    SiteUrl      = $SiteUrl
    CertName     = $CertName
} -Required (@('VpsHost', 'VpsUser', 'KeyPath', 'ServiceName', 'RemoteOptDir') + @(if ($Target -eq 'app') { 'CertName' })) -Default @{
    # Public already (plugins/pluginmaster.json's download links), so not a host secret.
    SiteUrl = 'https://apps.senpan.cafe'
}

$service = $settings.ServiceName
if ($service -notmatch '^[A-Za-z0-9_.@-]+$') { throw "ServiceName '$service' is not a systemd service name." }
$optDir = $settings.RemoteOptDir.TrimEnd('/')
$remoteBinary = "$optDir/app-suite"
$site = $settings.SiteUrl.TrimEnd('/')
if ($site -notmatch '^https?://[^/\s]+$') {
    throw "SiteUrl must be the site's address alone, such as https://example.com, not '$($settings.SiteUrl)'."
}
$certName = [string]$settings.CertName
if ($Target -eq 'app' -and $certName -notmatch '^[A-Za-z0-9][A-Za-z0-9.-]*$') {
    throw "CertName must be a certificate's folder name under /etc/letsencrypt/live, such as example.com, not '$certName'."
}

$repoRoot = Split-Path -Path $PSScriptRoot -Parent
# The dev database the pull refreshes; git-ignored, so this checkout's, never the release's.
$devDb = Join-Path $repoRoot 'devdata\database.sqlite'
# What the host records as running, for the next deploy to compare with.
$record = 'senpan'
# The vhost every app deploy installs, named for the service.
$vhostPath = "/etc/apache2/sites-available/$service.conf"
# The folders the move carries from the web root into the data folder.
$dataFolders = 'images fonts carrd plugin'

# A path the remote scripts take literally: absolute, and nothing a shell or sed reads
# as anything but itself.
function Test-RemotePath {
    [CmdletBinding()]
    [OutputType([bool])]
    param([string]$Path)
    $Path -match '^/[A-Za-z0-9._/@+-]*[A-Za-z0-9._@+-]$' -and $Path -notmatch '(^|/)\.\.?(/|$)'
}

# plink carries a remote command in one packet over its shared connection, whose buffer
# holds about 16 KB; a longer one ends the pipe (Error 109) before the host runs any of
# it. The limit leaves room for the packet's own fields.
$commandLimit = 15000
function Assert-CommandFit {
    [CmdletBinding()]
    param([string]$Script, [string]$What)
    $size = [System.Text.Encoding]::UTF8.GetByteCount(($Script -replace "`r", ''))
    if ($size -gt $commandLimit) {
        throw "$What is $size bytes, more than the $commandLimit plink's shared connection carries in one command; nothing was changed."
    }
}

# The service as the host runs it: the unit lives only there, so it is the one place
# that says where the live database is (-db), where the app listens (-addr), where its
# data folder is (-data, or -webroot on a unit not yet moved) and whom the service runs as.
function Read-ServiceUnit {
    [CmdletBinding()]
    [OutputType([hashtable])]
    param([string]$Database)
    if ($Database -match '[''"\s]') { throw "RemoteDbPath '$Database' cannot hold quotes or spaces." }
    $read = Format-RemoteScript -Value @{ SERVICE = $service; DB = $Database } -Template @'
svc='__SERVICE__'
show() { systemctl show "$svc.service" -p "$1" --value 2>/dev/null; }
if [ "$(show LoadState)" != loaded ]; then
  echo "$svc.service is not installed on the host; install its unit first (docs/ops.md). Nothing was changed." >&2
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
echo "unit_data=$(flag data)"
echo "unit_webroot=$(flag webroot)"
if [ -n "$db" ] && [ -f "$db" ]; then echo "db_owner=$(stat -c '%U:%G' "$db")"; fi
'@
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
        # The repository is the data folder's plugin/, which the app serves; a unit not
        # yet moved out of the web root still names that folder by -webroot.
        $found = Read-ServiceUnit -Database ''
        $data = if ($found['unit_data']) { $found['unit_data'] } else { $found['unit_webroot'] }
        if (-not $data -or -not (Test-RemotePath -Path $data)) {
            throw "$service.service names no usable -data on its ExecStart ('$data'), so where the plugin repository goes is unknown; nothing was changed."
        }
        $owner = if ($found['group']) { "$($found['user']):$($found['group'])" } else { $found['user'] }

        Write-Step 'Uploading...'
        $upload = Invoke-Upload -Path $archive.Path -Destination 'senpan-plugin.tgz' -Activity "Uploading Senpan Companion $($repo.Version)" -FileCount $archive.FileCount
        if (-not $upload.Ok) { throw "Upload failed; the published plugin is untouched.`n$($upload.Err)" }

        Write-Step "Publishing Senpan Companion $($repo.Version) to $data/plugin..."
        Invoke-DeployBundle -Archive 'senpan-plugin.tgz' -Sha256 $archive.Sha256 -ExpectedFileCount $archive.FileCount -Step @(
            Publish-PluginRepository -AsStep -Live "$data/plugin" -Version $repo.Version -IndexPath 'pluginmaster.json' -Owner $owner -NoBackup:$NoBackup
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
$moving = $false
$backups = $null
try {
    $connection = $settings | Open-DeployConnection

    # -- The service, as the host runs it, and the release it took last -------------
    $configuredDb = [string]$settings.RemoteDbPath
    Write-Step "Reading $service.service on the host..."
    $found = Read-ServiceUnit -Database $configuredDb

    $unitDb = $found['unit_db']
    if ($configuredDb -and $unitDb -and $configuredDb -ne $unitDb) {
        throw "RemoteDbPath is $configuredDb, but $service.service opens $unitDb (the -db on its ExecStart). Correct or clear RemoteDbPath; nothing was changed."
    }
    $db = if ($configuredDb) { $configuredDb } else { $unitDb }
    if (-not $db) {
        throw "$service.service names no -db on its ExecStart, so the live database is unknown. Set RemoteDbPath (-RemoteDbPath / `$env:DEPLOY_REMOTE_DB_PATH / `$DeployRemoteDbPath); nothing was changed."
    }

    # Where the app answers on the host: -addr, or the binary's own default (:8080,
    # backend/main.go). An address on every interface is asked on the loopback. The
    # vhost proxies to the same address.
    $addr = if ($found['unit_addr']) { $found['unit_addr'] } else { ':8080' }
    if ($addr -match '^(|0\.0\.0\.0|\[::\])(:\d+)$') { $addr = "127.0.0.1$($Matches[2])" }
    if ($addr -notmatch '^[A-Za-z0-9.\[\]:-]+:\d+$') { throw "Could not tell where $service listens from its -addr '$addr'; nothing was changed." }

    # The data folder: -data, or, on a unit that still passes -webroot, the folder this
    # run moves the files into.
    $webRoot = $null
    if ($found['unit_data']) {
        $data = $found['unit_data']
    } elseif ($found['unit_webroot']) {
        $moving = $true
        $webRoot = $found['unit_webroot'].TrimEnd('/')
        $data = "$optDir/data"
        if (-not (Test-RemotePath -Path $webRoot)) { throw "The unit's -webroot '$webRoot' is not a plain absolute path; nothing was changed." }
        if ($NoBackup) { throw "-NoBackup cannot go with the move out of $webRoot, which is undone through the previous release that -NoBackup removes; nothing was changed." }
    } else {
        throw "$service.service names neither -data nor -webroot on its ExecStart, so its files are unknown; nothing was changed."
    }
    if (-not (Test-RemotePath -Path $data)) { throw "The data folder '$data' is not a plain absolute path; nothing was changed." }

    $serviceOwner = if ($found['group']) { "$($found['user']):$($found['group'])" } else { $found['user'] }
    $dbOwner = if ($found['db_owner']) { $found['db_owner'] } else { 'not there yet' }
    Write-Ok "Live database: $db (owner $dbOwner); $service runs as $serviceOwner, listens on $addr and keeps its files in $data."
    if ($found['user'] -eq 'root' -and $found['dynamic'] -ne 'yes') {
        Write-Warning "$service runs as root, so its backup will too. docs/ops.md (Running as a non-root user) says how to move it to its own account."
    }

    $live = Get-DeployedRelease -Name $record
    if ($live -eq $version -and -not $Force) {
        Write-Ok "$version is already running on the host; nothing to deploy. (A version pushed since goes out once CI tags it; -Force deploys this one again.)"
        return
    }
    Write-Step "Releasing $version (the host runs $(if ($live) { $live } else { 'a release not recorded' }))"
    if ($moving) { Write-Step "This run also moves $dataFolders from $webRoot into $data, and the unit to -data." }

    # -- Build the release, in its own checkout --------------------------------------
    $tree = New-ReleaseTree -Path $repoRoot -Tag $version -Npm 'frontend'
    # The front end first: it builds into backend/internal/web/dist, which the binary embeds.
    $shell = Join-Path $tree 'backend\internal\web\dist\index.html'
    Invoke-NpmBuild -Path (Join-Path $tree 'frontend') -Output $shell
    $binary = Invoke-GoBuild -Path (Join-Path $tree 'backend') -Output 'app-suite'
    # The build's entry script: the smoke check wants the page to name it, and to fetch it.
    $entryScript = [regex]::Match((Get-Content -LiteralPath $shell -Raw), 'src="(/assets/[^"]+\.js)"').Groups[1].Value
    if (-not $entryScript) { throw 'The built index.html names no /assets/*.js entry script; the smoke check would have nothing to fetch.' }

    # The vhost, with the app's address and the certificate's name filled in.
    $hostDir = Join-Path $tree 'deploy\host'
    New-Item -ItemType Directory -Force -Path $hostDir | Out-Null
    $vhost = Join-Path $hostDir 'apache-vhost.conf'
    $template = Get-Content -LiteralPath (Join-Path $tree 'deploy\apache-vhost.conf') -Raw
    foreach ($placeholder in '__APP__', '__CERT__') {
        if (-not $template.Contains($placeholder)) { throw "deploy/apache-vhost.conf has no $placeholder placeholder; nothing was changed." }
    }
    Set-Content -LiteralPath $vhost -Value ($template.Replace('__APP__', $addr).Replace('__CERT__', $certName)) -NoNewline

    # -- The move out of the web root, as shell for the remote run -------------------
    $prepare = $null
    $move = $null
    $undo = $null
    $fontCheck = $null
    if ($moving) {
        $vars = Format-RemoteScript -Value @{ WEB = $webRoot; DATA = $data; SITE = $service; FOLDERS = $dataFolders; RECORD = $record } -Template @'
web='__WEB__'
data='__DATA__'
site='__SITE__'
folders='__FOLDERS__'
state="$HOME/.shared-deploy/__RECORD__-data-move"
'@
        # Puts back whatever the move changed, from what it recorded: the folders it
        # moved, the unit as it was and the sites it switched. Each part is safe to run
        # again, or when its part never happened.
        $undoFunction = @'
e_undo() {
  set +e
  echo 'Putting the web root back: its folders, the unit and the Apache sites.' >&2
  sites=$(cat "$state/sites" 2>/dev/null)
  unit=$(cat "$state/unit" 2>/dev/null)
  if [ -f "$state/moved" ]; then
    while read -r d; do
      if [ -n "$d" ] && [ -e "$data/$d" ] && [ ! -e "$web/$d" ]; then mv "$data/$d" "$web/$d" && echo "Moved $data/$d back to $web/$d." >&2; fi
    done < "$state/moved"
    rm -f "$state/moved"
  fi
  if [ -f "$state/sounds" ]; then
    if [ -e "$data/sounds" ] && [ ! -e "$web/dist/sounds" ]; then mv "$data/sounds" "$web/dist/sounds" && echo "Moved $data/sounds back to $web/dist/sounds." >&2; fi
    rm -f "$state/sounds"
  fi
  if [ -n "$unit" ] && [ -f "$state/unit.orig" ] && ! cmp -s "$state/unit.orig" "$unit"; then
    cat "$state/unit.orig" > "$unit"
    systemctl daemon-reload
  fi
  a2dissite -q "$site" >/dev/null 2>&1
  rm -f "/etc/apache2/sites-enabled/$site.conf"
  for s in $sites; do a2ensite -q "$s" >/dev/null 2>&1; done
  set -e
}
'@
        # Before anything stops, and changing nothing: what the move needs is checked,
        # and everything it touches is backed up.
        $prepareBody = @'
vhost="$bundle/apache-vhost.conf"
say() { printf '%s\n' "$*" >&2; }
for t in a2ensite a2dissite apache2ctl tar gzip awk curl; do
  command -v "$t" >/dev/null || { say "$t is not installed on the host; nothing was changed."; exit 3; }
done
# The folders: images/ in the web root to move, and no folder in the data folder already.
if [ ! -d "$web/images" ]; then say "There is no $web/images to move; nothing was changed."; exit 3; fi
for d in $folders; do
  if [ -L "$web/$d" ]; then say "$web/$d is a link, not a folder; nothing was changed."; exit 3; fi
  if [ -e "$web/$d" ] && [ -e "$data/$d" ]; then say "$data/$d is there already, and so is $web/$d; nothing was changed."; exit 3; fi
done
if [ ! -d "$(dirname "$data")" ]; then say "$(dirname "$data") is not there to hold $data; nothing was changed."; exit 3; fi
if [ -L "$web/dist/sounds" ]; then say "$web/dist/sounds is a link, not a folder; nothing was changed."; exit 3; fi
# The unit: -webroot outside a comment, and no drop-in that sets what the move edits.
unit=$(systemctl show "$svc" -p FragmentPath --value)
if [ ! -f "$unit" ] || ! grep -Ev '^[[:space:]]*#' "$unit" | grep -Eq -- '(^|[[:space:]=])--?webroot([[:space:]]|=)'; then
  say "$svc's unit ($unit) passes no -webroot, so it can't be moved to -data; nothing was changed."; exit 3
fi
for f in $(systemctl show "$svc" -p DropInPaths --value); do
  if grep -Eq '^[[:space:]]*(ExecStart|ReadWritePaths)=' "$f"; then say "$f sets ExecStart or ReadWritePaths; put them in $unit first. Nothing was changed."; exit 3; fi
done
# The sites now serving the vhost's names (apachectl -S), each serving those names alone.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
# A site file may end its lines with CRLF; the CR is no part of a name.
sed -n -E 's/^[[:space:]]*Server(Name|Alias)[[:space:]]+//p' "$vhost" | tr -s ' \t\r' '\n\n\n' | sed '/^$/d' | sort -u > "$tmp/names"
if [ ! -s "$tmp/names" ]; then say 'The new vhost names no host; nothing was changed.'; exit 3; fi
if ! apache2ctl -S > "$tmp/dump" 2>&1; then cat "$tmp/dump" >&2; say 'apachectl -S failed; nothing was changed.'; exit 3; fi
: > "$tmp/files"
while read -r n; do
  re=$(printf '%s' "$n" | sed 's/[.]/\\./g')
  sed -n -E "s/.*[[:space:]]${re}[[:space:]]+\\((\\/[^:]+):[0-9]+\\).*/\\1/p" "$tmp/dump" >> "$tmp/files"
done < "$tmp/names"
sites=
for f in $(sort -u "$tmp/files"); do
  s=$(basename "$f" .conf)
  if [ "$s" = "$site" ]; then continue; fi
  case "$f" in
    /etc/apache2/sites-enabled/*) ;;
    *) say "$f serves one of the vhost's names but is not an enabled site; nothing was changed."; exit 3 ;;
  esac
  sed -n -E 's/^[[:space:]]*Server(Name|Alias)[[:space:]]+//p' "$f" | tr -s ' \t\r' '\n\n\n' | sed '/^$/d' | sort -u > "$tmp/served"
  other=$(grep -vxF -f "$tmp/names" "$tmp/served" | tr '\n' ' ' || true)
  if [ -n "$other" ]; then say "$f also serves $other, which the new vhost does not; nothing was changed."; exit 3; fi
  sites="$sites $s"
done
if [ -z "$sites" ]; then say "No enabled site serves $(tr '\n' ' ' < "$tmp/names"); nothing was changed."; exit 3; fi
# Apache with the new vhost in place of those sites, tried on a copy of its config.
mkdir "$tmp/apache2"
cp -a /etc/apache2/. "$tmp/apache2/"
for s in $sites; do rm -f "$tmp/apache2/sites-enabled/$s.conf"; done
cp "$vhost" "$tmp/apache2/sites-available/$site.conf"
ln -sfn "../sites-available/$site.conf" "$tmp/apache2/sites-enabled/$site.conf"
if ! out=$(APACHE_CONFDIR="$tmp/apache2" apache2ctl -t 2>&1); then
  say "Apache would refuse the new vhost in place of$sites, so nothing was changed:"; say "$out"; exit 5
fi
# Room for the backups, and for the move itself when it copies to another filesystem.
need=0
for d in $folders; do
  if [ -e "$web/$d" ]; then need=$((need + $(du -sk "$web/$d" | awk '{print $1}'))); fi
done
free=$(df -Pk "$HOME" | awk 'NR == 2 {print $4}')
if [ "$free" -lt $((need + 262144)) ]; then say "The backups need up to $((need / 1024)) MB and $HOME has $((free / 1024)) MB free; nothing was changed."; exit 3; fi
if [ "$(stat -c %d "$web")" != "$(stat -c %d "$(dirname "$data")")" ]; then
  free=$(df -Pk "$(dirname "$data")" | awk 'NR == 2 {print $4}')
  if [ "$free" -lt $((need + 262144)) ]; then say "Moving the folders copies $((need / 1024)) MB to another filesystem with $((free / 1024)) MB free; nothing was changed."; exit 3; fi
fi
# The backups, compressed and checked, for root alone: each folder (the fonts with their
# converted copies), Apache's config, the unit and the web root's .htaccess.
umask 077
bdir="$HOME/senpan-before-data-move-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$bdir"
for d in $folders; do
  if [ -e "$web/$d" ]; then tar -czf "$bdir/$d.tar.gz" -C "$web" "$d"; gzip -t "$bdir/$d.tar.gz"; fi
done
tar -czf "$bdir/apache2.tar.gz" -C /etc apache2
gzip -t "$bdir/apache2.tar.gz"
tar -czf "$bdir/unit.tar.gz" -C / "${unit#/}"
gzip -t "$bdir/unit.tar.gz"
if [ -f "$web/.htaccess" ]; then tar -czf "$bdir/webroot-htaccess.tar.gz" -C "$web" .htaccess; gzip -t "$bdir/webroot-htaccess.tar.gz"; fi
if [ -d "$web/dist/sounds" ]; then tar -czf "$bdir/sounds.tar.gz" -C "$web/dist" sounds; gzip -t "$bdir/sounds.tar.gz"; fi
# What a rollback needs to put each back.
mkdir -p "$state"
chmod 700 "$state"
printf '%s\n' $sites > "$state/sites"
printf '%s\n' "$unit" > "$state/unit"
printf '%s\n' "$bdir" > "$state/backups"
cp -p "$unit" "$state/unit.orig"
rm -f "$state/moved" "$state/sounds"
echo "Checked: the sites to switch are$sites, and Apache takes the new vhost in their place."
if [ -e "$data/sounds" ]; then
  echo "$data/sounds is there already, so the sounds in $web/dist/sounds stay where they are."
elif [ -d "$web/dist/sounds" ]; then
  echo "The sounds in $web/dist/sounds move too."
else
  say "There is no $web/dist/sounds, so the sounds stay silent until they are copied into $data/sounds by hand (docs/ops.md)."
fi
echo "Backed up the web root's folders, Apache's config and the unit to $bdir."
'@
        $prepare = "(`nset -e`n$vars`nsvc='$service'`n. `"`$bundle/data-move-prepare.sh`"`n)"
        # After the stop (Install-ServiceBinary's -AfterStop): the folders move, the unit
        # takes -data, the sites switch. A failure from here to the start puts it all back
        # before the module restarts the previous release.
        $moveBody = @'
trap 'e_undo; abort_restart' EXIT
sites=$(cat "$state/sites")
unit=$(cat "$state/unit")
if [ ! -d "$data" ]; then install -d -m 750 ${owner:+-o "$owner"} ${group:+-g "$group"} "$data"; fi
for d in $folders; do
  if [ -e "$web/$d" ]; then
    mv "$web/$d" "$data/$d"
    echo "$d" >> "$state/moved"
    echo "Moved $web/$d to $data/$d."
  fi
done
# The sounds, from the web root's dist/sounds, where Apache served the SPA's sounds
# from, unless the data folder has its own already.
if [ -d "$web/dist/sounds" ] && [ ! -e "$data/sounds" ]; then
  : > "$state/sounds"
  mv "$web/dist/sounds" "$data/sounds"
  chmod -R a+rX "$data/sounds"
  echo "Moved $web/dist/sounds to $data/sounds."
fi
if [ -e "$web/fonts" ]; then echo "$web/fonts is still there after the move." >&2; exit 1; fi
# -data in place of -webroot, and the data folder writable in place of the web root's.
sed -E "/^[[:space:]]*#/!s#(^|[[:space:]=])--?webroot(=|[[:space:]]+)[^[:space:]\\\\]+#\\1-data $data#" "$state/unit.orig" |
awk -v web="$web" -v data="$data" '
  { line[NR] = $0 }
  /^[[:space:]]*ReadWritePaths=/ { rw[NR] = 1 }
  END {
    covered = 0
    for (i = 1; i <= NR; i++) {
      if (!(i in rw)) continue
      v = line[i]; sub(/^[[:space:]]*ReadWritePaths=/, "", v)
      n = split(v, p, /[[:space:]]+/); out = ""
      for (j = 1; j <= n; j++) {
        t = p[j]; if (t == "") continue
        b = t; sub(/^[-+]+/, "", b)
        if (b == web || index(b, web "/") == 1) continue
        if (b == data || index(data, b "/") == 1) covered = 1
        out = out (out == "" ? "" : " ") t
      }
      line[i] = out == "" ? "" : "ReadWritePaths=" out
      if (out == "") drop[i] = 1
    }
    for (i = 1; i <= NR; i++) {
      if (i in drop) {
        if (!covered && (i in rw)) { print "ReadWritePaths=" data; covered = 1 }
        continue
      }
      if (!covered && (i in rw)) { line[i] = line[i] " " data; covered = 1 }
      print line[i]
    }
  }' > "$state/unit.new"
cat "$state/unit.new" > "$unit"
systemctl daemon-reload
exec_line=" $(systemctl show "$main" -p ExecStart --value) "
case "$exec_line" in *" -data $data "*) ;; *) echo "After the edit the unit's ExecStart names no -data $data:$exec_line" >&2; exit 1 ;; esac
case "$exec_line" in *webroot*) echo "After the edit the unit's ExecStart still names -webroot:$exec_line" >&2; exit 1 ;; esac
rw=$(systemctl show "$main" -p ReadWritePaths --value)
covered=
for p in $rw; do
  p=${p#-}; p=${p#+}
  case "$p" in "$web"|"$web"/*) echo "After the edit ReadWritePaths still names $p." >&2; exit 1 ;; esac
  case "$data/" in "$p"/*) covered=1 ;; esac
done
if [ -z "$covered" ]; then echo "After the edit ReadWritePaths ($rw) does not cover $data." >&2; exit 1; fi
echo "The unit runs the app with -data $data now."
# The sites serving the vhost's names off, the vhost on. Apache takes them once the new release answers.
for s in $sites; do a2dissite -q "$s"; done
a2ensite -q "$site"
if ! out=$(apache2ctl configtest 2>&1); then echo "$out" >&2; echo 'Apache refused the new vhost.' >&2; exit 5; fi
echo "Apache's sites switched to $site; it takes them once the new release answers."
'@
        $move = "$vars`n. `"`$bundle/data-move-undo.sh`"`n. `"`$bundle/data-move.sh`""
        $moveFiles = [ordered]@{
            'data-move-prepare.sh' = $prepareBody
            'data-move-undo.sh'    = $undoFunction
            'data-move.sh'         = $moveBody
        }
        # A start that fails (Install-ServiceBinary's -OnRollback): the same undo, before
        # the module starts the previous release. The function is the one -AfterStop defined.
        $onRollback = 'e_undo'
        # Apache takes the new sites now that the new release answers.
        $afterSuccess = @'
if systemctl reload apache2; then echo 'Apache took the new sites.'; else echo 'apache2 did not reload; the old sites still answer.' >&2; fi
'@
        # The smoke check's rollback (Restore-ServiceBinary's -OnRollback): the undo,
        # then Apache back on the sites the move switched, before the previous release starts.
        $undo = "$vars`n$undoFunction`ne_undo`nsystemctl reload apache2 || echo 'apache2 did not reload after the sites went back.' >&2"
        # Every name the vhost serves, asked on the host through Apache for a font file
        # as a plain file, by the paths that could reach it, redirects followed. Read only;
        # prints one line per ask with the last answer's status (000: no answer).
        $fontCheck = Format-RemoteScript -Value @{ DATA = $data; WEB = $webRoot; VHOST = $vhostPath } -Template @'
data='__DATA__'
web='__WEB__'
if [ -e "$web/fonts" ]; then echo "present $web/fonts"; fi
f=$(find "$data/fonts" -maxdepth 1 -type f ! -name '.*' 2>/dev/null | head -n 1)
if [ -z "$f" ]; then echo 'nofont'; exit 0; fi
name=$(basename "$f" | sed -e 's/%/%25/g' -e 's/ /%20/g' -e 's/#/%23/g' -e 's/?/%3F/g')
for n in $(sed -n -E 's/^[[:space:]]*ServerName[[:space:]]+//p' '__VHOST__' | tr -d '\r' | sort -u); do
  for p in "/fonts/$name" "/$name" "/carrd/../fonts/$name" "/images/../fonts/$name"; do
    code=$(curl -s -k -L --max-redirs 5 --path-as-is -o /dev/null -w '%{http_code}' -m 10 --resolve "$n:443:127.0.0.1" "https://$n$p" || true)
    echo "asked $code https://$n$p"
  done
done
'@
    }

    # -- Pack the deploy -----------------------------------------------------------
    $content = [ordered]@{ 'app-suite' = $binary; 'apache-vhost.conf' = $vhost }
    if ($moving) {
        foreach ($part in $moveFiles.GetEnumerator()) {
            $path = Join-Path $hostDir $part.Key
            [System.IO.File]::WriteAllText($path, ($part.Value -replace "`r", ''))
            $content[$part.Key] = $path
        }
    }
    Write-Step 'Packing the deploy...'
    $archive = Compress-DeployArchive -Content $content
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
        Database                  = $db
        Umask                     = '077'
        RestoreDatabaseOnRollback = $true
        HealthUrl                 = "http://$addr/healthz"
        Vhost                     = 'apache-vhost.conf'
        VhostPath                 = $vhostPath
        NoBackup                  = $NoBackup
    }
    if ($moving) {
        $install.AfterStop = $move
        $install.OnRollback = $onRollback
        $install.AfterSuccess = $afterSuccess
    }
    $steps = @(
        if ($moving) { $prepare }
        Install-ServiceBinary -AsStep @install
    )
    # The smoke check's way back, built now so it is measured with the host run.
    $rollbackScript = @(
        Restore-ServiceBinary -ServiceName $service -Binary $remoteBinary -Owner $serviceOwner -Database $db -VhostPath $vhostPath -OnRollback $undo -AsScript
    )
    Assert-CommandFit -What 'The host run' -Script (Invoke-DeployBundle -AsScript -Archive 'senpan-deploy.tgz' -Sha256 $archive.Sha256 -ExpectedFileCount $archive.FileCount -Step $steps)
    Assert-CommandFit -What 'The rollback' -Script ($rollbackScript -join "`n")
    if ($moving) { Assert-CommandFit -What 'The font check' -Script $fontCheck }

    # -- Upload, then one remote run ---------------------------------------------
    Write-Step 'Uploading...'
    $upload = Invoke-Upload -Path $archive.Path -Destination 'senpan-deploy.tgz' -Activity "Uploading Senpan App Suite $version" -FileCount $archive.FileCount
    if (-not $upload.Ok) { throw "Upload failed; nothing on the host changed.`n$($upload.Err)" }
    Write-Ok ('Uploaded in {0:mm\:ss}.' -f $upload.Elapsed)
    Write-Step "Deploying $version on the host..."
    Invoke-DeployBundle -Archive 'senpan-deploy.tgz' -Sha256 $archive.Sha256 -ExpectedFileCount $archive.FileCount -Step $steps
    if ($moving) {
        $readBackups = 'cat "$HOME/.shared-deploy/' + $record + '-data-move/backups"'
        $backups = @(Invoke-Remote -Script $readBackups) | Select-Object -First 1
    }

    # -- Smoke check, through Cloudflare -----------------------------------------
    # The reverse of the deploy: the previous binary and vhost with the snapshot taken just
    # before this deploy's start, since the new binary may have migrated the schema, and
    # after a move, the web root's folders, the unit and the sites it switched. What was written in
    # the seconds the new release ran is lost with it.
    $rollback = { Invoke-RemoteStep -Script $rollbackScript -FailureMessage 'Rolling back failed on the host' }

    if ($moving) {
        Write-Step 'Asking every name, through Apache on the host, for a font as a plain file...'
        $asked = @(Invoke-Remote -Script $fontCheck)
        $asked | ForEach-Object { Write-Host "     $_" -ForegroundColor DarkGray }
        # Only a refusal (400, 403 or 404) shows a name keeps the fonts back. Any other
        # status, 000 included, leaves it unproven, and so does a run that asked nothing.
        $unproven = @($asked | Where-Object { $_ -match '^present ' -or ($_ -match '^asked (\d{3}) ' -and $Matches[1] -notin '400', '403', '404') })
        if (-not ($asked | Where-Object { $_ -match '^asked ' }) -and $asked -notcontains 'nofont') { $unproven += 'No name was asked.' }
        if ($unproven) {
            & $rollback
            throw "Not every name refused a font as a plain file, or the web root still holds fonts/:`n$($unproven -join "`n")`nThe move was put back."
        }
        if ($asked -contains 'nofont') { Write-Warning 'The data folder holds no font, so none was asked for.' }
    }

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
    Write-Host "     Previous binary kept at $remoteBinary.old, and the vhost before, if it changed, at $vhostPath.old." -ForegroundColor DarkGray
}
if ($moving) {
    Write-Host "     The files moved from $webRoot to $data; the web root's folders, Apache's config and the unit as they were are in $backups." -ForegroundColor DarkGray
}
Write-Host '     Assets are content-hashed and the PWA auto-updates, so no manual cache bust is needed.' -ForegroundColor DarkGray
