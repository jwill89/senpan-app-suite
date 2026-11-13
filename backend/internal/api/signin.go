package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/auth"
	"app-suite/internal/db"
	"app-suite/internal/model"
)

// Signing in and out and signing up are actions, not resources, so each is a POST
// under /auth. They run on the session the server's session layer loaded
// into the request's context: signing in moves it to a new id and records the
// account (auth.SignIn); the layer then writes the cookie.

var signInTags = []string{"Sign-in"}

type signInInput struct {
	Body struct {
		Username       string `json:"username" maxLength:"64" doc:"The account's username."`
		Password       string `json:"password" maxLength:"1024" doc:"Its password."`
		TurnstileToken string `json:"turnstileToken,omitempty" maxLength:"2048" doc:"The Cloudflare Turnstile response, when the check is on (GET /config gives its site key)."`
	}
}

type signUpInput struct {
	Body struct {
		Username       string `json:"username" minLength:"1" maxLength:"32" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The username to sign in with."`
		Password       string `json:"password" minLength:"8" maxLength:"1024" doc:"The password to sign in with, at least 8 characters."`
		TurnstileToken string `json:"turnstileToken,omitempty" maxLength:"2048" doc:"The Cloudflare Turnstile response, when the check is on (GET /config gives its site key)."`
	}
}

func (h handlers) registerSignIn(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "sign-in", Method: http.MethodPost, Path: "/auth/login",
		Summary: "Sign in with a username and password", Tags: signInTags,
		Description: "Answers who is now signed in. Five failed tries from one address within 15 minutes " +
			"hold that address back for the rest of the 15 minutes (429, with Retry-After).",
		Errors: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests},
	}, h.signIn)
	huma.Register(api, huma.Operation{
		OperationID: "sign-out", Method: http.MethodPost, Path: "/auth/logout",
		Summary: "Sign out", Tags: signInTags, DefaultStatus: http.StatusNoContent,
	}, h.signOut)
	huma.Register(api, huma.Operation{
		OperationID: "sign-up", Method: http.MethodPost, Path: "/auth/register",
		Summary: "Ask for an account", Tags: signInTags, DefaultStatus: http.StatusAccepted,
		Description: "Makes an account an admin must activate before it signs in. The answer is the same " +
			"whether or not the name was free, so it never tells who has an account. Five tries from one " +
			"address an hour (429, with Retry-After).",
		Errors: []int{http.StatusForbidden, http.StatusTooManyRequests},
	}, h.signUp)
}

func (h handlers) signIn(ctx context.Context, in *signInInput) (*meOutput, error) {
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.SignInLimit, ip); wait > 0 {
		h.Logger.Warn("sign-in held back", "ip", ip)
		return nil, tooMany(wait, "Too many failed sign-ins from here. Try again later.")
	}
	if !h.turnstile(ctx, in.Body.TurnstileToken, ip) {
		h.Logger.Warn("turnstile check failed", "ip", ip)
		return nil, huma.Error403Forbidden("The bot check didn't pass. Complete it and try again.")
	}
	username := strings.TrimSpace(in.Body.Username)
	u, hash, err := h.accountByName(ctx, username)
	if err != nil {
		return nil, h.fail("read the account signing in", err)
	}
	// An unknown name, or an account with no password (made with Discord), costs the
	// same argon2 work as a wrong password and answers the same, so the form can't be
	// used to find out who has an account.
	valid := false
	if u != nil && hash != "" {
		valid, _ = auth.Verify(in.Body.Password, hash)
	} else {
		auth.DummyVerify(in.Body.Password)
	}
	if !valid {
		if h.SignInLimit != nil {
			h.SignInLimit.Fail(ip)
		}
		h.Logger.Warn("sign-in failed", "ip", ip)
		h.Logger.Debug("sign-in failed", "ip", ip, "username", username)
		return nil, huma.Error401Unauthorized("That username and password don't match.")
	}
	if !u.IsActive {
		// The password was right, so this doesn't count against the address.
		return nil, huma.Error403Forbidden("An admin hasn't activated this account yet.")
	}
	if err := h.startSession(ctx, u, ip); err != nil {
		return nil, h.fail("sign in", err)
	}
	return &meOutput{Body: Me{Account: accountOf(u)}}, nil
}

func (h handlers) signOut(ctx context.Context, _ *struct{}) (*struct{}, error) {
	// Signed out either way: a session the store couldn't delete has already lost
	// its cookie and expires on its own.
	if err := h.Sessions.Destroy(ctx); err != nil {
		h.Logger.Error("end a session", "error", err)
	}
	return nil, nil
}

func (h handlers) signUp(ctx context.Context, in *signUpInput) (*struct{}, error) {
	// Every try counts, made or not, so nobody can make accounts in bulk.
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.RegisterLimit, ip); wait > 0 {
		h.Logger.Warn("sign-up held back", "ip", ip)
		return nil, tooMany(wait, "Too many sign-ups from here. Try again later.")
	}
	if h.RegisterLimit != nil {
		h.RegisterLimit.Fail(ip)
	}
	if !h.turnstile(ctx, in.Body.TurnstileToken, ip) {
		h.Logger.Warn("turnstile check failed", "ip", ip)
		return nil, huma.Error403Forbidden("The bot check didn't pass. Complete it and try again.")
	}
	// The reserved name and a taken one answer like a new account (202), so this
	// can't be used to find out who has an account.
	username := strings.TrimSpace(in.Body.Username)
	if strings.EqualFold(username, auth.ReservedUsername) {
		h.Logger.Warn("sign-up asked for the reserved name", "ip", ip)
		return nil, nil
	}
	hash, err := auth.Hash(in.Body.Password)
	if err != nil {
		return nil, h.fail("hash a password", err)
	}
	err = h.write().CreateUser(ctx, db.CreateUserParams{Username: username, PasswordHash: hash})
	switch {
	case uniqueViolation(err):
		h.Logger.Info("sign-up asked for a taken name", "ip", ip)
		h.Logger.Debug("sign-up asked for a taken name", "username", username)
	case err != nil:
		return nil, h.fail("make an account", err)
	default:
		ResourceChanged(h.Live, "users")
		h.Logger.Info("account made, waiting for an admin to activate it")
		h.Logger.Debug("account made", "username", username)
	}
	return nil, nil
}

// accountByName is the account named username and its password hash, or nil when
// there is none.
func (h handlers) accountByName(ctx context.Context, username string) (*model.User, string, error) {
	q := h.read()
	row, err := q.GetUserByUsername(ctx, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	hash, err := q.GetPasswordHash(ctx, row.ID)
	if err != nil {
		return nil, "", err
	}
	u, err := auth.Row(*row).User()
	return u, hash, err
}

// startSession signs u in on the request's session, from the address ip, and notes
// the time. Every way of signing in ends here.
func (h handlers) startSession(ctx context.Context, u *model.User, ip string) error {
	if err := auth.SignIn(ctx, h.Sessions, u); err != nil {
		return err
	}
	if h.SignInLimit != nil {
		h.SignInLimit.Reset(ip)
	}
	// Detached: the account is signed in whether or not the client stays for the
	// reply. The answer still carries the previous time, which the users page reads
	// as "last seen".
	if err := h.write().TouchLastLogin(context.WithoutCancel(ctx), u.ID); err != nil {
		h.Logger.Error("note a sign-in's time", "error", err, "user_id", u.ID)
	}
	return nil
}

// turnstile reports whether a Turnstile response passes, or true when the check is
// off.
func (h handlers) turnstile(ctx context.Context, response, ip string) bool {
	return h.Turnstile == nil || h.Turnstile(ctx, response, ip)
}

// uniqueViolation reports whether err is SQLite refusing a duplicate (the pure-Go
// driver says so in its message).
func uniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
