#!/usr/bin/env bash
#
# Auto-tag + GitHub Release when a version has no release yet. Run by the CI
# "release" job on a push to main, AFTER the gate jobs pass - so only green commits
# are ever released.
#
# Two things are released, each when its version is new:
#   - the app (the SPA and the API, one binary, one version since 4.0.0): tag
#     v<version> from frontend/package.json, notes from the "### [<version>]" entry
#     under CHANGELOG.md's "## App" section. The deploy ships the newest of these tags.
#   - the plugin, which keeps its own four-part version as Dalamud needs: tag
#     Plugin-v<version> from SenpanCompanion.csproj, notes from the "### [<version>]"
#     entry under CHANGELOG.md's "## Plugin" section.
#
# Idempotent: a version already released is skipped, so ordinary pushes are a no-op.
# When the git tag already exists (e.g. created by hand) but has no release, the
# release is attached to it; otherwise the tag is created at the pushed commit.
set -euo pipefail

# The `|| true` keeps a malformed or missing version file from aborting the run under
# `set -e`; an empty result just skips that release (below).
app_version()    { jq -r '.version // empty' frontend/package.json || true; }
plugin_version() { grep -oP -m1 '<Version>\K[^<]+' plugins/SenpanCompanion/SenpanCompanion.csproj || true; }

# notes <section> <version>: the "### [<version>]" entry under "## <section>" (App or
# Plugin), up to the next entry (### [), section (## ) or separator (---). index() is a
# literal (non-regex) match anchored at column 1, so the version's dots are literal, and
# the section scope keeps an app and a plugin version that look alike apart.
notes() {
  awk -v sec="## $1" -v ver="### [$2]" '
    $0 == sec { insec=1; next }
    insec && /^## / { insec=0 }
    insec && index($0, ver)==1 { cap=1; print; next }
    cap && (/^### \[/ || /^## / || /^---$/) { cap=0 }
    cap { print }
  ' CHANGELOG.md
}

# release <tag> <title> <notes file>
release() {
  local tag="$1" title="$2" notes_file="$3"
  if gh release view "$tag" >/dev/null 2>&1; then
    echo "== ${tag}: release already exists - skipping"
    return
  fi
  if [ ! -s "$notes_file" ]; then
    printf '_No CHANGELOG entry found for %s._\n' "$title" > "$notes_file"
    echo "!! ${tag}: no CHANGELOG section found - releasing with a placeholder" >&2
  fi
  if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null 2>&1; then
    echo "++ ${tag}: tag exists, attaching a release"
    gh release create "$tag" --verify-tag --title "$title" --notes-file "$notes_file"
  else
    echo "++ ${tag}: creating tag + release at ${GITHUB_SHA}"
    gh release create "$tag" --target "$GITHUB_SHA" --title "$title" --notes-file "$notes_file"
  fi
}

version="$(app_version)"
if [ -n "$version" ]; then
  notes_file="$(mktemp)"
  notes App "$version" > "$notes_file"
  release "v${version}" "v${version}" "$notes_file"
else
  echo "!! Could not read the app's version from frontend/package.json - skipping" >&2
fi

version="$(plugin_version)"
if [ -n "$version" ]; then
  notes_file="$(mktemp)"
  notes Plugin "$version" > "$notes_file"
  release "Plugin-v${version}" "Plugin v${version}" "$notes_file"
else
  echo "!! Could not read the plugin's version - skipping" >&2
fi
