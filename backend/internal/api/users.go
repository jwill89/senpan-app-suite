package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/auth"
	"app-suite/internal/db"
	"app-suite/internal/model"
	"app-suite/internal/permissions"
)

// The Users page: an admin's alone. The bootstrap admin (auth.ReservedUsername) is
// kept: nobody deactivates, demotes or deletes it here, or sets its password, which
// it changes itself. Its permissions may be edited, harmlessly, since an admin holds
// every one.

var userTags = []string{"Users"}

// User is an account as the Users page manages it.
type User struct {
	Account
	IsActive    bool    `json:"isActive" doc:"Whether it may sign in. A new account waits for an admin to activate it."`
	CreatedAt   *string `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
	LastLoginAt *string `json:"lastLoginAt" nullable:"true" doc:"When it last signed in, RFC 3339 in UTC, or null for never."`
}

// UserList is every account.
type UserList struct {
	Users []User `json:"users" nullable:"false" doc:"Every account, by username."`
}

// UserChanges is what an admin changes on an account: only what is sent.
type UserChanges struct {
	IsActive    *bool    `json:"isActive,omitempty" doc:"Whether it may sign in."`
	IsAdmin     *bool    `json:"isAdmin,omitempty" doc:"Whether it is an admin."`
	Permissions []string `json:"permissions,omitempty" nullable:"false" maxItems:"100" doc:"Its page permissions, replacing what it holds (an empty list takes them all away)."`
	Password    *string  `json:"password,omitempty" minLength:"8" maxLength:"1024" doc:"A new password, at least 8 characters. Signs out its sessions and revokes its token."`
}

type userOutput struct{ Body User }

type userListOutput struct{ Body UserList }

type userIDInput struct {
	ID int64 `path:"id" doc:"The account's id."`
}

type updateUserInput struct {
	ID   int64 `path:"id" doc:"The account's id."`
	Body UserChanges
}

func (h handlers) registerUsers(api huma.API) {
	admin := huma.Middlewares{h.admin(api)}
	huma.Register(api, huma.Operation{
		OperationID: "list-users", Method: http.MethodGet, Path: "/users",
		Summary: "Every account", Tags: userTags, Security: secured, Middlewares: admin,
	}, h.listUsers)
	huma.Register(api, huma.Operation{
		OperationID: "update-user", Method: http.MethodPatch, Path: "/users/{id}",
		Summary: "Change an account", Tags: userTags, Security: secured, Middlewares: admin,
		Errors: []int{http.StatusForbidden, http.StatusNotFound},
	}, h.updateUser)
	huma.Register(api, huma.Operation{
		OperationID: "delete-user", Method: http.MethodDelete, Path: "/users/{id}",
		Summary: "Delete an account", Tags: userTags, DefaultStatus: http.StatusNoContent,
		Security: secured, Middlewares: admin,
		Errors: []int{http.StatusForbidden, http.StatusNotFound},
	}, h.deleteUser)
}

func (h handlers) listUsers(ctx context.Context, _ *struct{}) (*userListOutput, error) {
	rows, err := h.read().ListUsers(ctx)
	if err != nil {
		return nil, h.fail("list accounts", err)
	}
	out := UserList{Users: make([]User, 0, len(rows))}
	for _, r := range rows {
		u, err := auth.Row(*r).User()
		if err != nil {
			return nil, h.fail("read an account", err)
		}
		out.Users = append(out.Users, userOf(u, r.CreatedAt, r.LastLoginAt))
	}
	return &userListOutput{Body: out}, nil
}

func (h handlers) updateUser(ctx context.Context, in *updateUserInput) (*userOutput, error) {
	target, err := h.user(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	change := in.Body
	if target.Username == auth.ReservedUsername && (change.IsActive != nil || change.IsAdmin != nil || change.Password != nil) {
		return nil, huma.Error403Forbidden("The admin account can't be changed here.")
	}
	perms := target.Permissions
	if change.Permissions != nil {
		if perms, err = checkPermissions(change.Permissions); err != nil {
			return nil, err
		}
	}
	active, admin := target.IsActive, target.IsAdmin
	if change.IsActive != nil {
		active = *change.IsActive
	}
	if change.IsAdmin != nil {
		admin = *change.IsAdmin
	}
	var hash string
	if change.Password != nil {
		if hash, err = auth.Hash(*change.Password); err != nil {
			return nil, h.fail("hash a password", err)
		}
	}
	// One transaction, detached: a reset that stopped before revoking the token would
	// leave a leaked token working past it.
	detached := context.WithoutCancel(ctx)
	if err := h.applyUserChanges(detached, in.ID, active, admin, perms, hash); err != nil {
		return nil, h.fail("change an account", err)
	}
	row, err := h.read().GetUser(detached, in.ID)
	if err != nil {
		return nil, h.fail("read an account", err)
	}
	u, err := auth.Row(*row).User()
	if err != nil {
		return nil, h.fail("read an account", err)
	}
	return &userOutput{Body: userOf(u, row.CreatedAt, row.LastLoginAt)}, nil
}

func (h handlers) applyUserChanges(ctx context.Context, id int64, active, admin bool, perms []string, hash string) error {
	tx, q, err := h.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := q.UpdateUser(ctx, db.UpdateUserParams{
		IsActive: boolInt(active), IsAdmin: boolInt(admin), Permissions: auth.EncodePermissions(perms), ID: id,
	}); err != nil {
		return err
	}
	if hash != "" {
		if err := q.SetPassword(ctx, db.SetPasswordParams{PasswordHash: hash, ID: id}); err != nil {
			return err
		}
		if _, err := q.DeleteToken(ctx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (h handlers) deleteUser(ctx context.Context, in *userIDInput) (*struct{}, error) {
	target, err := h.user(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if target.Username == auth.ReservedUsername {
		return nil, huma.Error403Forbidden("The admin account can't be deleted.")
	}
	// Its passkeys and token go with it (the foreign keys cascade); a session it left
	// behind finds no account and acts as a guest.
	if _, err := h.write().DeleteUser(ctx, in.ID); err != nil {
		return nil, h.fail("delete an account", err)
	}
	return nil, nil
}

// user is the account with id, or a 404.
func (h handlers) user(ctx context.Context, id int64) (*model.User, error) {
	row, err := h.read().GetUser(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound("There is no such account.")
	}
	if err != nil {
		return nil, h.fail("read an account", err)
	}
	u, err := auth.Row(*row).User()
	if err != nil {
		return nil, h.fail("read an account", err)
	}
	return u, nil
}

// checkPermissions is keys without repeats, or a 422 naming each key that isn't a
// permission.
func checkPermissions(keys []string) ([]string, error) {
	valid := permissions.Valid()
	out := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	var unknown []error
	for i, k := range keys {
		switch {
		case !valid[k]:
			unknown = append(unknown, &huma.ErrorDetail{
				Location: fmt.Sprintf("body.permissions[%d]", i), Message: "isn't a permission", Value: k,
			})
		case !seen[k]:
			seen[k] = true
			out = append(out, k)
		}
	}
	if len(unknown) > 0 {
		return nil, huma.Error422UnprocessableEntity("Some of those aren't permissions.", unknown...)
	}
	return out, nil
}

// userOf is u as the Users page shows it.
func userOf(u *model.User, createdAt, lastLoginAt *string) User {
	return User{Account: *accountOf(u), IsActive: u.IsActive, CreatedAt: createdAt, LastLoginAt: lastLoginAt}
}
