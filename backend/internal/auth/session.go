package auth

import (
	"context"

	"github.com/alexedwards/scs/v2"

	"app-suite/internal/model"
)

// What a session keeps, by key.
const (
	// SessionUserID is the signed-in account's id.
	SessionUserID = "user_id"
	// SessionEpoch is the account's password epoch when the session signed in. A
	// password change or reset moves the account's epoch on, so every session
	// signed in before it stops counting.
	SessionEpoch = "user_epoch"
	// SessionPasskeyLogin and SessionPasskeyRegister hold a passkey ceremony's
	// challenge between its two steps.
	SessionPasskeyLogin    = "webauthn_login"    //nolint:gosec // a session key's name, not a credential
	SessionPasskeyRegister = "webauthn_register" //nolint:gosec // a session key's name, not a credential
	// A Discord sign-in keeps, between its start and Discord sending the browser
	// back: the state it sent and when that expires, the PKCE verifier whose
	// challenge went with it, the page to land on, and the admin's link it was
	// started from (its id), if any.
	SessionDiscordState    = "discord_state"
	SessionDiscordUntil    = "discord_state_until"
	SessionDiscordVerifier = "discord_verifier"
	SessionDiscordNext     = "discord_next"
	SessionDiscordInvite   = "discord_invite"
)

// SignIn makes the request's session u's: it moves to a new id first (so a session
// id planted before signing in is worthless) and records the account and its
// password epoch. Every way of signing in goes through here, and so does a password
// change, which carries this session on under the new epoch.
func SignIn(ctx context.Context, sessions *scs.SessionManager, u *model.User) error {
	if err := sessions.RenewToken(ctx); err != nil {
		return err
	}
	sessions.Put(ctx, SessionUserID, u.ID)
	sessions.Put(ctx, SessionEpoch, u.PasswordEpoch)
	return nil
}
