package server

import (
	"net/http"

	"app-suite/internal/model"
)

// Page-permission keys. Each corresponds to exactly one admin page and mirrors
// the frontend AdminTab ids (and the router's meta.perm). Admins implicitly hold
// every permission; non-admin users are limited to the keys granted to them.
//
// The Users page itself ("system-users") is intentionally NOT a grantable
// permission — it is admin-only and guarded with requireAdmin.
const (
	permBingoGame          = "bingo-game"
	permBingoCards         = "bingo-cards"
	permBingoWinnersLog    = "bingo-winners-log"
	permBingoPatterns      = "bingo-patterns"
	permBingoPresets       = "bingo-presets"
	permTeahouseAnnounce   = "teahouse-announcements"
	permTeahouseAffiliates = "teahouse-affiliates"
	permTeahouseRaffles    = "teahouse-raffles"
	permFestivalGarapon    = "festival-garapon"
	permAtelierFonts       = "atelier-fonts"
	permAtelierCarrd       = "atelier-carrd"
	permSystemSettings     = "system-settings"
	permSystemThemes       = "system-themes"
	permSystemImages       = "system-images"
)

// bookClubSlugs lists the known book clubs that get their own page permission.
//
// NOTE: keep this in sync with BOOK_CLUBS in frontend/src/lib/constants.ts.
// Adding a club there means adding its slug here so its page permission is both
// grantable (validated in set_permissions) and enforceable by the handlers.
var bookClubSlugs = []string{"yaoi", "yuri"}

// bookClubPerm returns the page-permission key for a book club slug.
func bookClubPerm(slug string) string { return "bookclub-" + slug }

// validPermissions returns the set of all grantable page-permission keys.
func validPermissions() map[string]bool {
	keys := []string{
		permBingoGame, permBingoCards, permBingoWinnersLog, permBingoPatterns, permBingoPresets,
		permTeahouseAnnounce, permTeahouseAffiliates, permTeahouseRaffles,
		permFestivalGarapon,
		permAtelierFonts, permAtelierCarrd,
		permSystemSettings, permSystemThemes, permSystemImages,
	}
	set := make(map[string]bool, len(keys)+len(bookClubSlugs))
	for _, k := range keys {
		set[k] = true
	}
	for _, slug := range bookClubSlugs {
		set[bookClubPerm(slug)] = true
	}
	return set
}

// userHasPermission reports whether a non-admin user holds a specific page
// permission. Callers should special-case admins (who hold everything) before
// calling this.
func userHasPermission(u *model.User, perm string) bool {
	for _, p := range u.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

// anyBookClubPerm reports whether the user can access at least one book club.
// Used by shared book-club endpoints (lookup, image uploads) that aren't tied
// to a single club slug.
func anyBookClubPerm(u *model.User) bool {
	if u.IsAdmin {
		return true
	}
	for _, slug := range bookClubSlugs {
		if userHasPermission(u, bookClubPerm(slug)) {
			return true
		}
	}
	return false
}

// requireAnyBookClub guards book-club endpoints not tied to a single club
// (AniList lookup, cover/image uploads): it allows admins and any user with at
// least one book-club page permission. Writes 401/403 and returns false
// otherwise.
func (s *Server) requireAnyBookClub(w http.ResponseWriter, r *http.Request) bool {
	u := s.currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized – login required")
		return false
	}
	if anyBookClubPerm(u) {
		return true
	}
	writeError(w, http.StatusForbidden, "Forbidden – you do not have access to this feature")
	return false
}
