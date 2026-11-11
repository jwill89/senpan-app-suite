# Deploy configuration TEMPLATE. Copy this file to `deploy.config.ps1` (which is
# untracked - see .gitignore) and fill in your values. scripts/deploy.ps1 reads
# deploy.config.ps1 (through SharedDeploy's Resolve-DeploySetting) for every
# environment-specific setting that isn't passed as a -param or set via the matching
# $env:DEPLOY_* variable.
#
# Resolution order per setting (first non-empty wins):
#   1. the script -parameter (e.g. -VpsHost, -ServiceName)
#   2. the environment variable (e.g. $env:DEPLOY_VPS_HOST, $env:DEPLOY_SERVICE_NAME)
#   3. the $Deploy* value below (this file's copy)
#
# Keeping these here (not in the tracked deploy.ps1) means the repo never reveals
# the host, SSH user, key path, filesystem layout, service name or certificate.
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseDeclaredVarsMoreThanAssignments', '', Justification = 'deploy.ps1 dot-sources this file and reads the $Deploy* values')]
param()

$DeployVpsHost = "<your-droplet-ip-or-hostname>"
$DeployVpsUser = "<ssh-user>"                          # e.g. root or a deploy user
$DeployKeyPath = "<path-to-your-deploy-key.ppk>"       # load it into Pageant first if it has a passphrase
$DeployServiceName = "<systemd-service-name>"          # e.g. myapp
$DeployRemoteOptDir = "<dir-holding-the-app-binary>"   # e.g. /opt/myapp (its data folder is <this>/data)
$DeployCertName = "<certificate-name>"                 # the vhost's certificate: its folder under /etc/letsencrypt/live, e.g. example.com
# Optional: the site's public address, fetched after every deploy (the smoke check):
# the page, /healthz, /api/version and the plugin index, as visitors reach them through
# Cloudflare. Unset, it is https://apps.senpan.cafe; set it only to check another.
# $DeploySiteUrl = "<https-address-of-the-site>"       # e.g. https://example.com
# Optional: the live SQLite database on the host. Leave it unset: the deploy reads it
# from the -db on the service's ExecStart. When set, it must match that path or the
# deploy stops before changing anything; it is used on its own only when the unit
# names no -db.
# $DeployRemoteDbPath = "<remote-db-path>"             # e.g. /opt/myapp/data/database.sqlite
