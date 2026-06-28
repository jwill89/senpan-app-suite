// Package version exposes the backend's semantic version (major.minor.patch).
//
// Bump Version in the SAME change that ships a backend behaviour change, and add
// a matching entry to the top-level CHANGELOG.md (Backend section). The admin
// dashboard fetches this (GET /api/version) and shows it next to the frontend
// version so operators can confirm the two halves are compatible.
//
// Semver intent for the backend's API contract:
//   - MAJOR: a breaking change to the JSON/WebSocket API the SPA relies on.
//   - MINOR: a backward-compatible capability (new endpoint/field/event).
//   - PATCH: a bug fix or internal change with no API-surface effect.
package version

// Version is the backend's current semantic version. Keep it in sync with
// CHANGELOG.md's latest Backend entry.
const Version = "1.0.1"
