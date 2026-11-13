package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/auth"
	"app-suite/internal/db"
	"app-suite/internal/model"
	"app-suite/internal/permissions"
)

// The Users page: an admin's alone. The bootstrap admin (auth.ReservedUsername) is
// kept: nobody renames, deactivates, demotes or deletes it here, or sets its password,
// which it changes itself, or links or unlinks its Discord account but itself. Its
// permissions may be edited, harmlessly, since an admin holds every one.

var userTags = []string{"Users"}

// User is an account as the Users page manages it.
type User struct {
	Account
	IsActive    bool            `json:"isActive" doc:"Whether it may sign in. A new account waits for an admin to activate it."`
	CreatedAt   *string         `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
	LastLoginAt *string         `json:"lastLoginAt" nullable:"true" doc:"When it last signed in, RFC 3339 in UTC, or null for never."`
	Discord     *DiscordAccount `json:"discord" doc:"The Discord account it signs in with, or null when none is linked."`
}

// UserList is every account.
type UserList struct {
	Users []User `json:"users" nullable:"false" doc:"Every account, by username."`
}

// UserChanges is what an admin changes on an account: only what is sent.
type UserChanges struct {
	Username    *string  `json:"username,omitempty" minLength:"1" maxLength:"32" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"A new username. Its sessions, token, passkeys and Discord account carry on."`
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
		Description: "A username another account has, or the admin account's, is refused (409).",
		Errors:      []int{http.StatusForbidden, http.StatusNotFound, http.StatusConflict},
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
	links, err := h.discordLinks(ctx)
	if err != nil {
		return nil, h.fail("list Discord links", err)
	}
	out := UserList{Users: make([]User, 0, len(rows))}
	for _, r := range rows {
		u, err := auth.Row(*r).User()
		if err != nil {
			return nil, h.fail("read an account", err)
		}
		out.Users = append(out.Users, userOf(u, r.CreatedAt, r.LastLoginAt, links[u.ID]))
	}
	return &userListOutput{Body: out}, nil
}

func (h handlers) updateUser(ctx context.Context, in *updateUserInput) (*userOutput, error) {
	target, err := h.user(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	change := in.Body
	if target.Username == auth.ReservedUsername &&
		(change.Username != nil || change.IsActive != nil || change.IsAdmin != nil || change.Password != nil) {
		return nil, huma.Error403Forbidden("The admin account can't be changed here.")
	}
	name := target.Username
	if change.Username != nil {
		name = strings.TrimSpace(*change.Username)
		if strings.EqualFold(name, auth.ReservedUsername) {
			return nil, huma.Error409Conflict("That username is the admin account's.")
		}
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
	err = h.applyUserChanges(detached, in.ID, name, active, admin, perms, hash)
	if uniqueViolation(err) {
		return nil, huma.Error409Conflict("Another account has that username.")
	}
	if err != nil {
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
	link, err := h.discordLink(detached, in.ID)
	if err != nil {
		return nil, h.fail("read a Discord link", err)
	}
	return &userOutput{Body: userOf(u, row.CreatedAt, row.LastLoginAt, link)}, nil
}

func (h handlers) applyUserChanges(ctx context.Context, id int64, name string, active, admin bool, perms []string, hash string) error {
	tx, q, err := h.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := q.RenameUser(ctx, db.RenameUserParams{Username: name, ID: id}); err != nil {
		return err
	}
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
	// Its passkeys, token, Discord link and waiting links go with it (the foreign keys
	// cascade); a session it left behind finds no account and acts as a guest.
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

// userOf is u as the Users page shows it, with the Discord account linked to it (nil
// for none).
func userOf(u *model.User, createdAt, lastLoginAt *string, link *DiscordAccount) User {
	return User{Account: *accountOf(u), IsActive: u.IsActive, CreatedAt: createdAt, LastLoginAt: lastLoginAt, Discord: link}
}
