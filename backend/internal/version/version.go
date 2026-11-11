// Package version exposes the app's semantic version (major.minor.patch).
//
// The SPA and the API are one app, built into one binary, with one version:
// frontend/package.json's, which CI tags as v<version> and the deploy ships. Version
// repeats it here because the API reports it (GET /api/v1/version, GET /healthz) and
// the committed OpenAPI document names it; a test fails if the two differ, so a
// release changes both, with its CHANGELOG.md section.
//
// Semver intent:
//   - MAJOR: a breaking change to the JSON API or its live events, which the plugin, a
//     client released on its own, relies on.
//   - MINOR: a backward-compatible capability (new endpoint, field, event or page).
//   - PATCH: a bug fix or internal change with no API-surface effect.
package version

// Version is the app's current semantic version, frontend/package.json's.
const Version = "5.2.0"
