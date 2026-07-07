#!/usr/bin/env bash
#
# Auto-tag + GitHub Release per component when its current version has no release
# yet. Run by the CI "release" job on pushes to `main`, AFTER the frontend /
# backend / plugin gate jobs pass — so only green commits are ever released.
#
# Each component's tag is <Component>-v<version> (matching the manual scheme:
# Frontend-v3.5.0, Backend-v3.4.0, Plugin-v2.0.1.0) and the release body is that
# component's section from CHANGELOG.md, so every tag is linked to its notes.
#
# Idempotent: a component whose <Component>-v<version> release already exists is
# skipped, so this can run on every main push. When the git tag already exists
# (e.g. created by hand) but has no release, the release is attached to it;
# otherwise the tag is created at the pushed commit.
set -euo pipefail

# ── Version extraction (source of truth per component) ────────────────────────
# The frontend version is JSON, so parse it with jq (preinstalled on the CI
# runner). The backend (Go const) and plugin (XML csproj) are not JSON — grep
# them. The `|| true` keeps a malformed/missing version file from aborting the
# whole run under `set -e`; an empty result just skips that component (below).
frontend_version() { jq -r '.version // empty' frontend/package.json || true; }
backend_version()  { grep -oP 'const Version = "\K[^"]+' backend/internal/version/version.go || true; }
plugin_version()   { grep -oP -m1 '<Version>\K[^<]+' plugins/SenpanCompanion/SenpanCompanion.csproj || true; }

# ── CHANGELOG section extraction ──────────────────────────────────────────────
# Print the block under "## <section>" / "### [<version>]" up to the next
# version (### [), section (## ), or separator (---). Section-scoped so a version
# number shared across components (e.g. 3.4.0) resolves to the right section.
changelog_section() {
  awk -v sec="## $1" -v ver="### [$2]" '
    index($0, sec)==1 { insec=1; next }
    insec && /^## / { insec=0 }
    insec && index($0, ver)==1 { incap=1; print; next }
    incap && (/^### \[/ || /^## / || /^---$/) { incap=0 }
    incap { print }
  ' CHANGELOG.md
}

# ── Release one component ─────────────────────────────────────────────────────
release_component() {
  local component="$1" section="$2" version="$3"
  local tag="${component}-v${version}"

  if [ -z "$version" ]; then
    echo "!! Could not read a version for ${component} — skipping" >&2
    return
  fi
  if gh release view "$tag" >/dev/null 2>&1; then
    echo "== ${tag}: release already exists — skipping"
    return
  fi

  local notes_file
  notes_file="$(mktemp)"
  changelog_section "$section" "$version" > "$notes_file"
  if [ ! -s "$notes_file" ]; then
    printf '_No CHANGELOG entry found for %s %s._\n' "$section" "$version" > "$notes_file"
    echo "!! ${tag}: no CHANGELOG section found — releasing with a placeholder" >&2
  fi

  if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null 2>&1; then
    echo "++ ${tag}: tag exists, attaching a release"
    gh release create "$tag" --verify-tag \
      --title "${component} v${version}" --notes-file "$notes_file"
  else
    echo "++ ${tag}: creating tag + release at ${GITHUB_SHA}"
    gh release create "$tag" --target "$GITHUB_SHA" \
      --title "${component} v${version}" --notes-file "$notes_file"
  fi
}

release_component "Frontend" "Frontend" "$(frontend_version)"
release_component "Backend"  "Backend"  "$(backend_version)"
release_component "Plugin"   "Plugin"   "$(plugin_version)"
