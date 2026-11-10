package server

import (
	"net/http"

	"app-suite/internal/bookclubs"
	"app-suite/internal/model"
	"app-suite/internal/permissions"
)

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
	for _, slug := range bookclubs.Slugs() {
		if userHasPermission(u, permissions.BookClub(slug)) {
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
		writeError(w, http.StatusUnauthorized, "Unauthorized - login required")
		return false
	}
	if anyBookClubPerm(u) {
		return true
	}
	writeError(w, http.StatusForbidden, "Forbidden - you do not have access to this feature")
	return false
}
