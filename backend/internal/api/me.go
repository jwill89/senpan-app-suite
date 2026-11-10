package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/auth"
	"app-suite/internal/db"
	"app-suite/internal/model"
)

// The signed-in account's own things: who it is, its password, its personal access
// token and its passkeys (passkeys.go). A token reads them but can't change the
// account's credentials, so those writes take a session.

var accountTags = []string{"Account"}

// Account is an account as its own pages and the guards see it.
type Account struct {
	ID          int64    `json:"id" doc:"The account's id."`
	Username    string   `json:"username" doc:"Its username."`
	IsAdmin     bool     `json:"isAdmin" doc:"Whether it is an admin, who holds every permission and alone manages accounts and the integrations."`
	Permissions []string `json:"permissions" nullable:"false" doc:"The page permissions it has been granted. An admin holds every one, listed here or not."`
}

// Me is who is signed in.
type Me struct {
	Account *Account `json:"account" doc:"The signed-in account, or null when nobody is."`
}

// TokenSummary is a personal access token without its secret.
type TokenSummary struct {
	Prefix     string  `json:"prefix" doc:"Its first characters, to tell it by."`
	CreatedAt  *string `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
	LastUsedAt *string `json:"lastUsedAt" nullable:"true" doc:"When it was last used, RFC 3339 in UTC, or null for never."`
}

// MyToken is the account's personal access token, if it has one.
type MyToken struct {
	Token *TokenSummary `json:"token" doc:"The account's personal access token, or null when it has none."`
}

// NewToken is a token just made: the only time its secret is shown.
type NewToken struct {
	Token     string `json:"token" doc:"The token. Shown this once: send it as an Authorization: Bearer header."`
	Prefix    string `json:"prefix" doc:"Its first characters, which the account page shows to tell it by."`
	CreatedAt string `json:"createdAt" doc:"When it was made, RFC 3339 in UTC."`
}

type meOutput struct{ Body Me }

type myTokenOutput struct{ Body MyToken }

type newTokenOutput struct{ Body NewToken }

type changePasswordInput struct {
	Body struct {
		CurrentPassword string `json:"currentPassword" maxLength:"1024" doc:"The account's password now."`
		NewPassword     string `json:"newPassword" minLength:"8" maxLength:"1024" doc:"The password to change it to, at least 8 characters."`
	}
}

func (h handlers) registerMe(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-me", Method: http.MethodGet, Path: "/me",
		Summary: "Who is signed in", Tags: accountTags,
		Description: "Always answers: the account is null when nobody is signed in.",
	}, h.getMe)
	huma.Register(api, huma.Operation{
		OperationID: "change-password", Method: http.MethodPut, Path: "/me/password",
		Summary: "Change the account's password", Tags: accountTags, DefaultStatus: http.StatusNoContent,
		Description: "Signs out every other session and revokes the account's token; this browser stays signed in.",
		Security:    secured, Middlewares: huma.Middlewares{h.session(api)},
	}, h.changePassword)
	huma.Register(api, huma.Operation{
		OperationID: "get-token", Method: http.MethodGet, Path: "/me/token",
		Summary: "The account's personal access token, without its secret", Tags: accountTags,
		Security: secured, Middlewares: huma.Middlewares{h.signedIn(api)},
	}, h.getToken)
	huma.Register(api, huma.Operation{
		OperationID: "create-token", Method: http.MethodPost, Path: "/me/token",
		Summary: "Make the account's personal access token", Tags: accountTags, DefaultStatus: http.StatusCreated,
		Description: "Replaces the account's token, if it has one. The answer is the only time the token is shown.",
		Security:    secured, Middlewares: huma.Middlewares{h.session(api)},
	}, h.createToken)
	huma.Register(api, huma.Operation{
		OperationID: "revoke-token", Method: http.MethodDelete, Path: "/me/token",
		Summary: "Revoke the account's personal access token", Tags: accountTags, DefaultStatus: http.StatusNoContent,
		Security: secured, Middlewares: huma.Middlewares{h.session(api)},
		Errors: []int{http.StatusNotFound},
	}, h.revokeToken)
}

func (h handlers) getMe(ctx context.Context, _ *struct{}) (*meOutput, error) {
	return &meOutput{Body: Me{Account: accountOf(Who(ctx))}}, nil
}

func (h handlers) changePassword(ctx context.Context, in *changePasswordInput) (*struct{}, error) {
	u := Who(ctx)
	if err := h.checkPassword(ctx, u.ID, in.Body.CurrentPassword, "body.currentPassword"); err != nil {
		return nil, err
	}
	fresh, err := auth.Hash(in.Body.NewPassword)
	if err != nil {
		return nil, h.fail("hash a password", err)
	}
	// The change and what must come with it run detached: stopping between them would
	// leave a leaked token working past the change.
	detached := context.WithoutCancel(ctx)
	if err := h.setPassword(detached, u.ID, fresh); err != nil {
		return nil, h.fail("change a password", err)
	}
	// The account's epoch moved on, so every other session is signed out; this one
	// carries on under a new id and the new epoch.
	row, err := h.read().GetUser(detached, u.ID)
	if err != nil {
		return nil, h.fail("read an account", err)
	}
	now, err := auth.Row(*row).User()
	if err != nil {
		return nil, h.fail("read an account", err)
	}
	if err := auth.SignIn(ctx, h.Sessions, now); err != nil {
		return nil, h.fail("carry a session on", err)
	}
	return nil, nil
}

// checkPassword is nil when password is account id's, or a 422 naming the field
// (location) it came in.
func (h handlers) checkPassword(ctx context.Context, id int64, password, location string) error {
	hash, err := h.read().GetPasswordHash(ctx, id)
	if err != nil {
		return h.fail("read a password", err)
	}
	if ok, _ := auth.Verify(password, hash); !ok {
		return huma.Error422UnprocessableEntity("That isn't this account's password.", &huma.ErrorDetail{
			Location: location, Message: "isn't this account's password",
		})
	}
	return nil
}

// setPassword sets an account's password and revokes its token, together: moving the
// password epoch on signs out every session from before.
func (h handlers) setPassword(ctx context.Context, id int64, hash string) error {
	tx, q, err := h.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := q.SetPassword(ctx, db.SetPasswordParams{PasswordHash: hash, ID: id}); err != nil {
		return err
	}
	if _, err := q.DeleteToken(ctx, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (h handlers) getToken(ctx context.Context, _ *struct{}) (*myTokenOutput, error) {
	row, err := h.read().GetToken(ctx, Who(ctx).ID)
	if errors.Is(err, sql.ErrNoRows) {
		return &myTokenOutput{}, nil
	}
	if err != nil {
		return nil, h.fail("read a token", err)
	}
	return &myTokenOutput{Body: MyToken{Token: &TokenSummary{
		Prefix: row.TokenPrefix, CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt,
	}}}, nil
}

func (h handlers) createToken(ctx context.Context, _ *struct{}) (*newTokenOutput, error) {
	token, hash, prefix, err := auth.NewToken()
	if err != nil {
		return nil, h.fail("make a token", err)
	}
	row, err := h.write().SetToken(ctx, db.SetTokenParams{UserID: Who(ctx).ID, TokenHash: hash, TokenPrefix: prefix})
	if err != nil {
		return nil, h.fail("keep a token", err)
	}
	out := NewToken{Token: token, Prefix: row.TokenPrefix}
	if row.CreatedAt != nil {
		out.CreatedAt = *row.CreatedAt
	}
	return &newTokenOutput{Body: out}, nil
}

func (h handlers) revokeToken(ctx context.Context, _ *struct{}) (*struct{}, error) {
	n, err := h.write().DeleteToken(ctx, Who(ctx).ID)
	if err != nil {
		return nil, h.fail("revoke a token", err)
	}
	if n == 0 {
		return nil, huma.Error404NotFound("This account has no token.")
	}
	return nil, nil
}

// accountOf is u as the API shows it, or nil for nobody.
func accountOf(u *model.User) *Account {
	if u == nil {
		return nil
	}
	perms := u.Permissions
	if perms == nil {
		perms = []string{}
	}
	return &Account{ID: u.ID, Username: u.Username, IsAdmin: u.IsAdmin, Permissions: perms}
}
