package apidoc

import "github.com/getkin/kin-openapi/openapi3"

// apiDescription is the spec-level overview (Markdown, rendered by Scalar). It
// documents the cross-cutting auth model, conventions, and the WebSocket channel
// (which OpenAPI can't describe natively).
const apiDescription = `JSON / WebSocket API for the **Senpan App Suite** (real-time Bingo, raffles, a
festival lottery drum, Discord announcements, book-club reading lists, font/image
hosting, and a themeable, permissioned admin dashboard).

## Conventions

- All responses are ` + "`application/json`" + ` with ` + "`Cache-Control: no-store`" + `.
- Request bodies are JSON, capped at 1 MB (multipart uploads have their own caps).
- Errors use the envelope ` + "`{ \"error\": \"message\" }`" + ` (the **Error** schema) with an
  appropriate status. A malformed JSON body returns ` + "`400 Invalid JSON`" + `.
- Most ` + "`POST`" + ` endpoints are **action dispatchers**: the body carries an ` + "`action`" + `
  field and the server switches on it. Each operation lists its actions and the
  fields each one uses.

## Authentication

Identity is resolved from either a **session cookie** (set by ` + "`POST /api/auth`" + `
login) or a **personal access token** (PAT) supplied as ` + "`Authorization: Bearer pat_…`" + `
(or ` + "`?token=pat_…`" + ` on the WebSocket upgrade). A PAT inherits its owning account's
permissions, so external clients use the same guards as the SPA.

Guard levels referenced in each operation's description:
- **public** — no auth.
- **auth** — any authenticated, active account.
- **admin** — an active admin account.
- **permission:<key>** — an admin, or a non-admin holding that page-permission key.

Page-permission keys: ` + "`bingo-game`, `bingo-cards`, `bingo-winners-log`, `bingo-patterns`, `bingo-presets`, `teahouse-announcements`, `teahouse-affiliates`, `teahouse-raffles`, `festival-garapon`, `festival-stamp-rally`, `atelier-fonts`, `atelier-carrd`, `system-settings`, `system-themes`, `system-images`" + `,
plus per-book-club ` + "`bookclub-yaoi` / `bookclub-yuri`" + `. The Users page is admin-only
(not a grantable key).

## WebSocket — ` + "`GET /api/ws`" + `

Not an HTTP operation (OpenAPI can't model the upgrade), so it is documented here.
Connect with ` + "`GET /api/ws`" + `; the channel is chosen by the ` + "`id`" + ` query param:

- ` + "`?id=<cardID>`" + ` → **player** channel (public): receives draws after the
  configured delay; targeted by card-deletion disconnects.
- **no ` + "`id`" + `** → **admin** channel: requires an authenticated active account
  (cookie or ` + "`?token=pat_…`" + `); streams draws immediately plus winner card IDs.

Broadcast messages (each carries a ` + "`type`" + ` field): ` + "`resource_changed`, `cards_update`, `card_deleted`, `patterns_update`, `game_update`, `game_draw`, `halftime_minigame`, `draw_delay_update`, `details_update`, `style_update`, `settings_update`" + `.`

// securitySchemes declares the two ways a request can authenticate.
func securitySchemes() openapi3.SecuritySchemes {
	cookie := openapi3.NewSecurityScheme()
	cookie.Type = "apiKey"
	cookie.In = "cookie"
	cookie.Name = "session"
	cookie.Description = "SCS session cookie set by POST /api/auth (login)."

	bearer := openapi3.NewSecurityScheme()
	bearer.Type = "http"
	bearer.Scheme = "bearer"
	bearer.Description = "Personal access token (pat_…). Also accepted as ?token= on the WebSocket upgrade."

	return openapi3.SecuritySchemes{
		"cookieAuth": &openapi3.SecuritySchemeRef{Value: cookie},
		"bearerAuth": &openapi3.SecuritySchemeRef{Value: bearer},
	}
}

// tags groups operations by feature area in the rendered docs.
func tags() openapi3.Tags {
	names := []struct{ name, desc string }{
		{"System", "Version, config, and the WebSocket entry point"},
		{"Auth", "Login, logout, registration, current user"},
		{"Users & Account", "Admin user management and self-service account/token"},
		{"Bingo", "Board, cards, game lifecycle, patterns, presets, styles"},
		{"Raffles", "Raffles and entries"},
		{"Garapon", "Festival lottery drum (admin + tokenized public)"},
		{"Affiliates", "Partner establishments"},
		{"Stamp Rally", "Stamp-rally events (admin + tokenized public)"},
		{"Book Club", "Reading lists and Discord publishing"},
		{"Announcements", "Scheduled Discord announcements, types, and roles"},
		{"Winners Log", "Confirmed winners log"},
		{"Settings", "App settings"},
		{"Files", "Fonts, Carrd projects, and central image hosting"},
	}
	out := make(openapi3.Tags, 0, len(names))
	for _, t := range names {
		out = append(out, &openapi3.Tag{Name: t.name, Description: t.desc})
	}
	return out
}
