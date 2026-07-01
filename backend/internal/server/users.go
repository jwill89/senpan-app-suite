package server

import (
	"net/http"

	"app-suite/internal/auth"
	"app-suite/internal/model"
)

// ── User management (admin) + self-service account (any user) ─────────────────
//
// The Users page is admin-only: GET /api/users lists accounts, PATCH/DELETE
// /api/users/{id} modify one. Every account, admin or not, can change its own
// password via POST /api/account/change-password.
//
// The seeded "admin" account is protected: no one but "admin" itself can delete,
// deactivate, demote, or change the password of "admin". (Its page permissions
// may still be edited here — permissions are deliberately not part of the
// protected set.)

// handleUsersList returns all accounts (without password hashes).
//
//	Endpoint:  GET /api/users
//	Auth:      admin
//	Response:  {"users": [User, ...]}
func (s *Server) handleUsersList(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	users, err := s.store.ListUsers()
	if err != nil {
		writeInternalError(w, "list users", err)
		return
	}
	writeJSON(w, http.StatusOK, model.UsersResponse{Users: users})
}

// userPatchRequest is the JSON body for PATCH /api/users/{id}. Every field is a
// pointer so the handler can tell "field omitted" from "field set to its zero
// value" (e.g. active:false vs. active absent) and apply only what was supplied.
type userPatchRequest struct {
	Active      *bool     `json:"active"`
	Admin       *bool     `json:"admin"`
	Permissions *[]string `json:"permissions"`
	Password    *string   `json:"password"`
}

// handleUserPatch applies one or more account changes in a single request. It
// merges the former set_active / set_admin / set_permissions / set_password
// actions: any present field is applied. The bootstrap "admin" account is
// protected — active/admin/password may not be changed on it (permissions may,
// matching the former behavior where set_permissions was not a protected action).
//
//	Endpoint:  PATCH /api/users/{id}
//	Auth:      admin
//	Request:   {"active"?: bool, "admin"?: bool, "permissions"?: [...], "password"?: "..."}
//	Response:  {"ok": true}
func (s *Server) handleUserPatch(w http.ResponseWriter, r *http.Request) {
	actor := s.currentUser(r)
	if actor == nil || !actor.IsAdmin {
		writeError(w, http.StatusUnauthorized, "Unauthorized – admin login required")
		return
	}
	id, ok := pathInt64(w, r, "id", "user")
	if !ok {
		return
	}
	req, err := readJSON[userPatchRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	target, err := s.store.GetUserByID(id)
	if err != nil {
		writeInternalError(w, "get user", err)
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}

	// Protect the bootstrap "admin" account: it can't be activated/deactivated,
	// promoted/demoted, or have its password reset by anyone here (it rotates its
	// own password via /api/account/change-password). Permissions are deliberately
	// allowed — mirrors the former action set, where set_permissions was not
	// protected.
	protected := target.Username == reservedUsername
	if protected && (req.Active != nil || req.Admin != nil || req.Password != nil) {
		writeError(w, http.StatusForbidden, "The admin account cannot be modified here")
		return
	}

	if req.Active == nil && req.Admin == nil && req.Permissions == nil && req.Password == nil {
		writeError(w, http.StatusBadRequest, "No fields to update")
		return
	}

	if req.Active != nil {
		if err := s.store.SetUserActive(id, *req.Active); err != nil {
			writeInternalError(w, "set user active", err)
			return
		}
	}
	if req.Admin != nil {
		if err := s.store.SetUserAdmin(id, *req.Admin); err != nil {
			writeInternalError(w, "set user admin", err)
			return
		}
	}
	if req.Permissions != nil {
		valid := validPermissions()
		cleaned := make([]string, 0, len(*req.Permissions))
		seen := make(map[string]bool, len(*req.Permissions))
		for _, p := range *req.Permissions {
			if !valid[p] {
				writeError(w, http.StatusBadRequest, "Unknown permission: "+p)
				return
			}
			if !seen[p] {
				seen[p] = true
				cleaned = append(cleaned, p)
			}
		}
		if err := s.store.SetUserPermissions(id, cleaned); err != nil {
			writeInternalError(w, "set user permissions", err)
			return
		}
	}
	if req.Password != nil {
		if len(*req.Password) < minPasswordLen {
			writeError(w, http.StatusBadRequest, "Password must be at least 8 characters")
			return
		}
		hash, err := auth.Hash(*req.Password)
		if err != nil {
			writeInternalError(w, "hash password", err)
			return
		}
		if err := s.store.SetUserPassword(id, hash); err != nil {
			writeInternalError(w, "set user password", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// handleUserDelete deletes an account. The bootstrap "admin" account is protected.
//
//	Endpoint:  DELETE /api/users/{id}
//	Auth:      admin
//	Response:  204 No Content
func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	actor := s.currentUser(r)
	if actor == nil || !actor.IsAdmin {
		writeError(w, http.StatusUnauthorized, "Unauthorized – admin login required")
		return
	}
	id, ok := pathInt64(w, r, "id", "user")
	if !ok {
		return
	}
	target, err := s.store.GetUserByID(id)
	if err != nil {
		writeInternalError(w, "get user", err)
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}
	if target.Username == reservedUsername {
		writeError(w, http.StatusForbidden, "The admin account cannot be modified here")
		return
	}
	if _, err := s.store.DeleteUser(id); err != nil {
		writeInternalError(w, "delete user", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// changePasswordRequest is the JSON body for POST /api/account/change-password.
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleAccountChangePassword lets the logged-in user rotate their own password
// (every active user, including the protected "admin" account).
//
//	Endpoint:  POST /api/account/change-password
//	Auth:      any active user
//	Request:   {"current_password": "...", "new_password": "..."}
//	Response:  {"ok": true}
func (s *Server) handleAccountChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	req, err := readJSON[changePasswordRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if len(req.NewPassword) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "New password must be at least 8 characters")
		return
	}
	// Re-read with the hash to verify the current password.
	_, hash, err := s.store.GetUserByUsername(user.Username)
	if err != nil {
		writeInternalError(w, "get user", err)
		return
	}
	valid, _ := auth.Verify(req.CurrentPassword, hash)
	if !valid {
		writeError(w, http.StatusUnauthorized, "Current password is incorrect")
		return
	}
	newHash, err := auth.Hash(req.NewPassword)
	if err != nil {
		writeInternalError(w, "hash password", err)
		return
	}
	if err := s.store.SetUserPassword(user.ID, newHash); err != nil {
		writeInternalError(w, "set password", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}
