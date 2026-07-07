# Deploy configuration TEMPLATE. Copy this file to `deploy.config.ps1` (which is
# untracked — see .gitignore) and fill in your values. scripts/deploy.ps1
# dot-sources deploy.config.ps1 to resolve every environment-specific setting
# when it isn't passed as a -param or set via the matching $env:SENPAN_* variable.
#
# Resolution order per setting (first non-empty wins):
#   1. the script -parameter (e.g. -VpsHost, -WebRoot)
#   2. the environment variable (e.g. $env:SENPAN_VPS_HOST, $env:SENPAN_WEB_ROOT)
#   3. the $Senpan* value below (this file's copy)
#
# Keeping these here (not in the tracked deploy.ps1) means the repo never reveals
# the host, SSH user, key path, filesystem layout, or service name.
$SenpanVpsHost = "<your-droplet-ip-or-hostname>"
$SenpanVpsUser = "<ssh-user>"                          # e.g. root or a deploy user
$SenpanDeployKey = "<path-to-your-deploy-key.ppk>"
$SenpanWebRoot = "<apache-documentroot-on-host>"       # e.g. /var/www/example.com
$SenpanServiceName = "<systemd-service-name>"          # e.g. myapp
$SenpanRemoteOptDir = "<dir-holding-the-backend-binary>" # e.g. /opt/myapp
