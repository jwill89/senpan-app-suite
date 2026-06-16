package server

import (
	"net/http"

	"app-suite/internal/auth"
)

// ── User management (admin) + self-service account (any user) ─────────────────
//
// The Users page is admin-only: GET/POST /api/users require an admin. Every
// account, admin or not, can change its own password via POST /api/account.
//
// The seeded "admin" account is protected: no one but "admin" itself can delete,
// deactivate, demote, or change the password of "admin".

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
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

// usersRequest is the JSON body for POST /api/users.
type usersRequest struct {
	Action      string   `json:"action"`
	ID          int64    `json:"id"`
	Active      bool     `json:"active"`      // for set_active
	Admin       bool     `json:"admin"`       // for set_admin
	Permissions []string `json:"permissions"` // for set_permissions
	Password    string   `json:"password"`    // for set_password
}

// handleUsersAction performs admin user-management operations.
//
//	Endpoint:  POST /api/users
//	Auth:      admin
//	Request:   {"action": "set_active"|"set_admin"|"set_permissions"|"set_password"|"delete", "id": N, ...}
//	Response:  {"ok": true}
func (s *Server) handleUsersAction(w http.ResponseWriter, r *http.Request) {
	actor := s.currentUser(r)
	if actor == nil || !actor.IsAdmin {
		writeError(w, http.StatusUnauthorized, "Unauthorized – admin login required")
		return
	}

	req, err := readJSON[usersRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.ID == 0 {
		writeError(w, http.StatusBadRequest, "User id is required")
		return
	}

	target, err := s.store.GetUserByID(req.ID)
	if err != nil {
		writeInternalError(w, "get user", err)
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}

	// Protect the bootstrap "admin" account from changes by anyone else. (The
	// "admin" account rotates its own password via /api/account, not here.)
	protected := target.Username == reservedUsername
	switch req.Action {
	case "set_active", "set_admin", "set_password", "delete":
		if protected {
			writeError(w, http.StatusForbidden, "The admin account cannot be modified here")
			return
		}
	}

	switch req.Action {
	case "set_active":
		if err := s.store.SetUserActive(req.ID, req.Active); err != nil {
			writeInternalError(w, "set user active", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "set_admin":
		if err := s.store.SetUserAdmin(req.ID, req.Admin); err != nil {
			writeInternalError(w, "set user admin", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "set_permissions":
		valid := validPermissions()
		cleaned := make([]string, 0, len(req.Permissions))
		seen := make(map[string]bool, len(req.Permissions))
		for _, p := range req.Permissions {
			if !valid[p] {
				writeError(w, http.StatusBadRequest, "Unknown permission: "+p)
				return
			}
			if !seen[p] {
				seen[p] = true
				cleaned = append(cleaned, p)
			}
		}
		if err := s.store.SetUserPermissions(req.ID, cleaned); err != nil {
			writeInternalError(w, "set user permissions", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "set_password":
		if len(req.Password) < minPasswordLen {
			writeError(w, http.StatusBadRequest, "Password must be at least 8 characters")
			return
		}
		hash, err := auth.Hash(req.Password)
		if err != nil {
			writeInternalError(w, "hash password", err)
			return
		}
		if err := s.store.SetUserPassword(req.ID, hash); err != nil {
			writeInternalError(w, "set user password", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "delete":
		if _, err := s.store.DeleteUser(req.ID); err != nil {
			writeInternalError(w, "delete user", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: set_active, set_admin, set_permissions, set_password, delete")
	}
}

// accountRequest is the JSON body for POST /api/account.
type accountRequest struct {
	Action          string `json:"action"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleAccountAction handles self-service account operations for the logged-in
// user. Currently the only action is change_password, which every active user
// (including the protected "admin" account) uses to rotate its own password.
//
//	Endpoint:  POST /api/account
//	Auth:      any active user
//	Request:   {"action": "change_password", "current_password": "...", "new_password": "..."}
//	Response:  {"ok": true}
func (s *Server) handleAccountAction(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	req, err := readJSON[accountRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "change_password":
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
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: change_password")
	}
}
