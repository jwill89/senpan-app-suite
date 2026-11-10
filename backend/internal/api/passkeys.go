package api

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"app-suite/internal/auth"
	"app-suite/internal/db"
	"app-suite/internal/model"
)

// Passkeys sign in alongside the password: a signed-in account adds them, then signs
// in "usernameless", the browser offering its own (discoverable) credential. Each
// ceremony has two steps; its challenge waits in the session between them, for ten
// minutes at most.

// ceremonyTimeout is how long a passkey ceremony's challenge holds.
const ceremonyTimeout = 10 * time.Minute

// PasskeyOptions is a ceremony's first step: what the browser needs to make or
// offer a passkey.
type PasskeyOptions struct {
	PublicKey json.RawMessage `json:"publicKey" doc:"The WebAuthn options, as PublicKeyCredential.parseCreationOptionsFromJSON (adding a passkey) or parseRequestOptionsFromJSON (signing in) takes them."`
}

// Passkey is one of an account's passkeys, without its key.
type Passkey struct {
	ID         int64   `json:"id" doc:"The passkey's id."`
	Name       string  `json:"name" doc:"The name it was added under."`
	CreatedAt  string  `json:"createdAt" doc:"When it was added, RFC 3339 in UTC."`
	LastUsedAt *string `json:"lastUsedAt" nullable:"true" doc:"When it last signed in, RFC 3339 in UTC, or null for never."`
}

// Passkeys is an account's passkeys.
type Passkeys struct {
	Passkeys []Passkey `json:"passkeys" nullable:"false" doc:"Its passkeys, oldest first."`
}

type passkeyOptionsOutput struct{ Body PasskeyOptions }

type passkeyOutput struct{ Body Passkey }

type passkeysOutput struct{ Body Passkeys }

type passkeySignInInput struct {
	Body struct {
		Credential json.RawMessage `json:"credential" doc:"The browser's answer: the PublicKeyCredential's toJSON()."`
	}
}

type addPasskeyInput struct {
	Body struct {
		Name       string          `json:"name,omitempty" maxLength:"60" doc:"What to call it, such as the device it lives on; \"Passkey\" when left out."`
		Credential json.RawMessage `json:"credential" doc:"The browser's answer: the PublicKeyCredential's toJSON()."`
	}
}

type beginPasskeyInput struct {
	Body struct {
		Password string `json:"password" maxLength:"1024" doc:"The account's password: adding a way to sign in asks for the one it has, so an unattended session can't add one."`
	}
}

type passkeyIDInput struct {
	ID int64 `path:"id" doc:"The passkey's id."`
}

func (h handlers) registerPasskeys(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "begin-passkey-sign-in", Method: http.MethodPost, Path: "/auth/passkey/begin",
		Summary: "Start signing in with a passkey", Tags: signInTags,
		Description: "Thirty starts from one address within 10 minutes hold it back for the rest of the 10 minutes (429, with Retry-After).",
		Errors:      []int{http.StatusTooManyRequests},
	}, h.beginPasskeySignIn)
	huma.Register(api, huma.Operation{
		OperationID: "finish-passkey-sign-in", Method: http.MethodPost, Path: "/auth/passkey/finish",
		Summary: "Sign in with the passkey the browser offered", Tags: signInTags,
		Description: "Answers who is now signed in. A failure counts with the password's (429, with Retry-After).",
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusTooManyRequests},
	}, h.finishPasskeySignIn)

	huma.Register(api, huma.Operation{
		OperationID: "list-passkeys", Method: http.MethodGet, Path: "/me/passkeys",
		Summary: "The signed-in account's passkeys", Tags: accountTags,
		Security: secured, Middlewares: huma.Middlewares{h.signedIn(api)},
	}, h.listPasskeys)
	huma.Register(api, huma.Operation{
		OperationID: "begin-passkey", Method: http.MethodPost, Path: "/me/passkeys/begin",
		Summary: "Start adding a passkey", Tags: accountTags,
		Description: "Asks for the account's password, so a session left open can't add a way in.",
		Security:    secured, Middlewares: huma.Middlewares{h.session(api)},
	}, h.beginPasskey)
	huma.Register(api, huma.Operation{
		OperationID: "add-passkey", Method: http.MethodPost, Path: "/me/passkeys",
		Summary: "Add the passkey the browser made", Tags: accountTags, DefaultStatus: http.StatusCreated,
		Security: secured, Middlewares: huma.Middlewares{h.session(api)},
		Errors: []int{http.StatusBadRequest, http.StatusConflict},
	}, h.addPasskey)
	huma.Register(api, huma.Operation{
		OperationID: "delete-passkey", Method: http.MethodDelete, Path: "/me/passkeys/{id}",
		Summary: "Remove a passkey", Tags: accountTags, DefaultStatus: http.StatusNoContent,
		Security: secured, Middlewares: huma.Middlewares{h.session(api)},
		Errors: []int{http.StatusNotFound},
	}, h.deletePasskey)
}

func (h handlers) beginPasskeySignIn(ctx context.Context, _ *struct{}) (*passkeyOptionsOutput, error) {
	// Each start leaves a challenge in the session store until it expires, so starts
	// are counted, apart from failed sign-ins: opening the prompt a few times must not
	// hold the address back from its password.
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.PasskeyLimit, ip); wait > 0 {
		return nil, tooMany(wait, "Too many tries from here. Try again later.")
	}
	if h.PasskeyLimit != nil {
		h.PasskeyLimit.Fail(ip)
	}
	wa, err := h.webAuthn(ctx)
	if err != nil {
		return nil, h.fail("set up WebAuthn", err)
	}
	assertion, ceremony, err := wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, h.fail("start a passkey sign-in", err)
	}
	if err := h.putCeremony(ctx, auth.SessionPasskeyLogin, ceremony); err != nil {
		return nil, h.fail("keep a passkey challenge", err)
	}
	return passkeyOptions(assertion.Response)
}

func (h handlers) finishPasskeySignIn(ctx context.Context, in *passkeySignInInput) (*meOutput, error) {
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.SignInLimit, ip); wait > 0 {
		return nil, tooMany(wait, "Too many failed sign-ins from here. Try again later.")
	}
	ceremony, ok := h.takeCeremony(ctx, auth.SessionPasskeyLogin)
	if !ok {
		return nil, huma.Error400BadRequest("No passkey sign-in is under way. Start again.")
	}
	wa, err := h.webAuthn(ctx)
	if err != nil {
		return nil, h.fail("set up WebAuthn", err)
	}
	refused := func(err error) error {
		if h.SignInLimit != nil {
			h.SignInLimit.Fail(ip)
		}
		h.Logger.Warn("passkey sign-in failed", "ip", ip, "error", err)
		return huma.Error401Unauthorized("That passkey didn't sign you in.")
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(in.Body.Credential)
	if err != nil {
		return nil, refused(err)
	}
	// The browser names the account by its user handle; its stored passkeys are what
	// the signature is checked against.
	found, credential, err := wa.ValidatePasskeyLogin(func(_, handle []byte) (webauthn.User, error) {
		id, ok := userHandleID(handle)
		if !ok {
			return nil, errors.New("a user handle that isn't an account id")
		}
		row, err := h.read().GetUser(ctx, id)
		if err != nil {
			return nil, err
		}
		u, err := auth.Row(*row).User()
		if err != nil {
			return nil, err
		}
		if !u.IsActive {
			return nil, errors.New("an account that isn't active")
		}
		return h.passkeyUser(ctx, u)
	}, *ceremony, parsed)
	if err != nil {
		return nil, refused(err)
	}
	pu, ok := found.(*passkeyUser)
	if !ok {
		return nil, h.fail("read a passkey's account", fmt.Errorf("unexpected %T", found))
	}
	// The assertion is spent, so what it changed is kept whether or not the client
	// stays for the reply: above all the signature counter, which clone detection
	// compares the next sign-in against.
	if stored, err := json.Marshal(credential); err == nil {
		if err := h.write().UsePasskey(context.WithoutCancel(ctx), db.UsePasskeyParams{
			Credential:   string(stored),
			CredentialID: base64.RawURLEncoding.EncodeToString(credential.ID),
		}); err != nil {
			h.Logger.Error("keep a passkey's counter", "error", err)
		}
	}
	if err := h.startSession(ctx, pu.user, ip); err != nil {
		return nil, h.fail("sign in", err)
	}
	return &meOutput{Body: Me{Account: accountOf(pu.user)}}, nil
}

func (h handlers) listPasskeys(ctx context.Context, _ *struct{}) (*passkeysOutput, error) {
	rows, err := h.read().ListPasskeys(ctx, Who(ctx).ID)
	if err != nil {
		return nil, h.fail("list passkeys", err)
	}
	out := Passkeys{Passkeys: make([]Passkey, 0, len(rows))}
	for _, r := range rows {
		out.Passkeys = append(out.Passkeys, Passkey{ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt})
	}
	return &passkeysOutput{Body: out}, nil
}

func (h handlers) beginPasskey(ctx context.Context, in *beginPasskeyInput) (*passkeyOptionsOutput, error) {
	if err := h.checkPassword(ctx, Who(ctx).ID, in.Body.Password, "body.password"); err != nil {
		return nil, err
	}
	wa, err := h.webAuthn(ctx)
	if err != nil {
		return nil, h.fail("set up WebAuthn", err)
	}
	pu, err := h.passkeyUser(ctx, Who(ctx))
	if err != nil {
		return nil, h.fail("read passkeys", err)
	}
	// A resident (discoverable) key, so signing in needs no username; and none the
	// authenticator already holds for this account.
	creation, ceremony, err := wa.BeginRegistration(pu,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		}),
		webauthn.WithExclusions(pu.exclusions()),
	)
	if err != nil {
		return nil, h.fail("start adding a passkey", err)
	}
	if err := h.putCeremony(ctx, auth.SessionPasskeyRegister, ceremony); err != nil {
		return nil, h.fail("keep a passkey challenge", err)
	}
	return passkeyOptions(creation.Response)
}

func (h handlers) addPasskey(ctx context.Context, in *addPasskeyInput) (*passkeyOutput, error) {
	u := Who(ctx)
	ceremony, ok := h.takeCeremony(ctx, auth.SessionPasskeyRegister)
	if !ok {
		return nil, huma.Error400BadRequest("No passkey is being added. Start again.")
	}
	wa, err := h.webAuthn(ctx)
	if err != nil {
		return nil, h.fail("set up WebAuthn", err)
	}
	pu, err := h.passkeyUser(ctx, u)
	if err != nil {
		return nil, h.fail("read passkeys", err)
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(in.Body.Credential)
	if err != nil {
		return nil, huma.Error400BadRequest("The passkey wasn't accepted. Try again.")
	}
	credential, err := wa.CreateCredential(pu, *ceremony, parsed)
	if err != nil {
		h.Logger.Warn("passkey not accepted", "error", err)
		return nil, huma.Error400BadRequest("The passkey wasn't accepted. Try again.")
	}
	id := base64.RawURLEncoding.EncodeToString(credential.ID)
	taken, err := h.read().PasskeyExists(ctx, id)
	if err != nil {
		return nil, h.fail("check a passkey", err)
	}
	if taken {
		return nil, huma.Error409Conflict("That passkey is already added.")
	}
	stored, err := json.Marshal(credential)
	if err != nil {
		return nil, h.fail("encode a passkey", err)
	}
	row, err := h.write().CreatePasskey(ctx, db.CreatePasskeyParams{
		UserID: u.ID, CredentialID: id, Credential: string(stored), Name: auth.PasskeyName(in.Body.Name),
	})
	switch {
	case uniqueViolation(err):
		return nil, huma.Error409Conflict("That passkey is already added.")
	case err != nil:
		return nil, h.fail("add a passkey", err)
	}
	return &passkeyOutput{Body: Passkey{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt}}, nil
}

func (h handlers) deletePasskey(ctx context.Context, in *passkeyIDInput) (*struct{}, error) {
	n, err := h.write().DeletePasskey(ctx, db.DeletePasskeyParams{ID: in.ID, UserID: Who(ctx).ID})
	if err != nil {
		return nil, h.fail("remove a passkey", err)
	}
	if n == 0 {
		return nil, huma.Error404NotFound("This account has no such passkey.")
	}
	return nil, nil
}

// webAuthn is the relying party for the site the request came to: its host is the
// RP ID and its address the one origin allowed, so passkeys work on whatever name
// serves the app (the proxy passes the real Host on).
func (h handlers) webAuthn(ctx context.Context) (*webauthn.WebAuthn, error) {
	site := requestOf(ctx).Site
	u, err := url.Parse(site)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("no site address for WebAuthn (%q)", site)
	}
	timeout := webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTimeout, TimeoutUVD: ceremonyTimeout}
	return webauthn.New(&webauthn.Config{
		RPDisplayName: "Senpan App Suite",
		RPID:          u.Hostname(),
		RPOrigins:     []string{site},
		Timeouts:      webauthn.TimeoutsConfig{Login: timeout, Registration: timeout},
	})
}

// putCeremony keeps a ceremony's challenge in the session under key.
func (h handlers) putCeremony(ctx context.Context, key string, ceremony *webauthn.SessionData) error {
	b, err := json.Marshal(ceremony)
	if err != nil {
		return err
	}
	h.Sessions.Put(ctx, key, string(b))
	return nil
}

// takeCeremony takes back the challenge kept under key: it answers once.
func (h handlers) takeCeremony(ctx context.Context, key string) (*webauthn.SessionData, bool) {
	raw := h.Sessions.PopString(ctx, key)
	if raw == "" {
		return nil, false
	}
	var ceremony webauthn.SessionData
	if err := json.Unmarshal([]byte(raw), &ceremony); err != nil {
		return nil, false
	}
	return &ceremony, true
}

// passkeyOptions answers a ceremony's options as the JSON the browser takes.
func passkeyOptions(options any) (*passkeyOptionsOutput, error) {
	b, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	return &passkeyOptionsOutput{Body: PasskeyOptions{PublicKey: b}}, nil
}

// passkeyUser is an account and its stored passkeys, as go-webauthn asks for them.
// Its user handle is the account id's 8 bytes, big-endian: stable, opaque and free
// of anything personal (WebAuthn decides by the handle, never the name).
type passkeyUser struct {
	user        *model.User
	credentials []webauthn.Credential
}

func (h handlers) passkeyUser(ctx context.Context, u *model.User) (*passkeyUser, error) {
	stored, err := h.read().ListPasskeyCredentials(ctx, u.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	pu := &passkeyUser{user: u, credentials: make([]webauthn.Credential, 0, len(stored))}
	for _, s := range stored {
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(s), &c); err != nil {
			h.Logger.Error("a stored passkey doesn't read", "error", err, "user_id", u.ID)
			continue // one bad row doesn't stop the account's other passkeys
		}
		pu.credentials = append(pu.credentials, c)
	}
	return pu, nil
}

func (u *passkeyUser) WebAuthnID() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(u.user.ID)) //nolint:gosec // the id's bits, kept as they are: a positive row id comes back through userHandleID
	return b
}

func (u *passkeyUser) WebAuthnName() string                       { return u.user.Username }
func (u *passkeyUser) WebAuthnDisplayName() string                { return u.user.Username }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// exclusions are the account's passkeys, so an authenticator isn't asked to add
// one it already holds.
func (u *passkeyUser) exclusions() []protocol.CredentialDescriptor {
	out := make([]protocol.CredentialDescriptor, 0, len(u.credentials))
	for i := range u.credentials {
		out = append(out, u.credentials[i].Descriptor())
	}
	return out
}

// userHandleID reads an account id back from its user handle.
func userHandleID(handle []byte) (int64, bool) {
	if len(handle) != 8 {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(handle)), true //nolint:gosec // WebAuthnID's bits read back: the row id it wrote
}
