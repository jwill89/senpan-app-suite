package server

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"app-suite/internal/model"
)

// WebAuthn / passkey support. Passkeys are a second login factor alongside the
// password: a logged-in account registers one or more passkeys, then logs in
// "usernameless" via a discoverable-credential ceremony. Credentials are stored
// as JSON (store layer) and the transient challenge lives in the SCS session
// between the begin/finish halves of each ceremony.

// webAuthn builds a relying-party config for this request. The RP ID is the
// request host (no scheme/port) and the origin is scheme://host, so passkeys work
// on whatever domain serves the app (prod + dev) without extra config — relying
// on the reverse proxy passing the real Host (ProxyPreserveHost On), like
// siteBaseURL does.
func (s *Server) webAuthn(r *http.Request) (*webauthn.WebAuthn, error) {
	origin := s.siteBaseURL(r)
	u, err := url.Parse(origin)
	if err != nil {
		return nil, err
	}
	return webauthn.New(&webauthn.Config{
		RPDisplayName: "Senpan App Suite",
		RPID:          u.Hostname(),
		RPOrigins:     []string{origin},
	})
}

// webauthnUser adapts a model.User + its stored credentials to webauthn.User. The
// user handle is the 8-byte big-endian user id — stable, opaque, and free of PII
// (the WebAuthn spec requires authz decisions be made on this handle, not name).
type webauthnUser struct {
	user  *model.User
	creds []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(u.user.ID))
	return b
}
func (u *webauthnUser) WebAuthnName() string                       { return u.user.Username }
func (u *webauthnUser) WebAuthnDisplayName() string                { return u.user.Username }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// excludeList returns descriptors for the user's existing credentials so the
// authenticator won't register the same passkey twice.
func (u *webauthnUser) excludeList() []protocol.CredentialDescriptor {
	out := make([]protocol.CredentialDescriptor, 0, len(u.creds))
	for i := range u.creds {
		out = append(out, u.creds[i].Descriptor())
	}
	return out
}

// userHandleToID decodes an 8-byte big-endian user handle back to a user id.
func userHandleToID(handle []byte) (int64, bool) {
	if len(handle) != 8 {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(handle)), true
}

// loadWebAuthnUser loads a user's stored credentials and wraps them for go-webauthn.
func (s *Server) loadWebAuthnUser(u *model.User) (*webauthnUser, error) {
	jsons, err := s.store.GetPasskeyCredentialsJSON(u.ID)
	if err != nil {
		return nil, err
	}
	creds := make([]webauthn.Credential, 0, len(jsons))
	for _, j := range jsons {
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(j), &c); err != nil {
			continue // skip a corrupt row rather than failing the whole ceremony
		}
		creds = append(creds, c)
	}
	return &webauthnUser{user: u, creds: creds}, nil
}

// putWebAuthnSession stashes the ceremony challenge in the SCS session as JSON.
func (s *Server) putWebAuthnSession(r *http.Request, key string, data *webauthn.SessionData) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	s.sessions.Put(r.Context(), key, string(b))
	return nil
}

// popWebAuthnSession reads and clears a stashed ceremony challenge.
func (s *Server) popWebAuthnSession(r *http.Request, key string) (*webauthn.SessionData, bool) {
	raw := s.sessions.GetString(r.Context(), key)
	if raw == "" {
		return nil, false
	}
	s.sessions.Remove(r.Context(), key)
	var data webauthn.SessionData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, false
	}
	return &data, true
}

const (
	sessKeyPasskeyRegister = "webauthn_register"
	sessKeyPasskeyLogin    = "webauthn_login"
)

// handlePasskeyRegisterBegin starts registering a new passkey for the logged-in
// account. Auth: any active account (self-service).
func (s *Server) handlePasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	wa, err := s.webAuthn(r)
	if err != nil {
		writeInternalError(w, "webauthn config", err)
		return
	}
	wu, err := s.loadWebAuthnUser(user)
	if err != nil {
		writeInternalError(w, "load passkeys", err)
		return
	}
	// Prefer a resident (discoverable) key so login can be usernameless.
	creation, sessionData, err := wa.BeginRegistration(wu,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		}),
		webauthn.WithExclusions(wu.excludeList()),
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Could not start passkey registration")
		return
	}
	if err := s.putWebAuthnSession(r, sessKeyPasskeyRegister, sessionData); err != nil {
		writeInternalError(w, "session", err)
		return
	}
	writeJSON(w, http.StatusOK, creation)
}

// handlePasskeyRegisterFinish verifies the attestation and stores the credential.
// The passkey's friendly name comes from the ?name= query. Auth: self-service.
//
//	Request: the raw attestation JSON from navigator.credentials.create().
func (s *Server) handlePasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	sessionData, found := s.popWebAuthnSession(r, sessKeyPasskeyRegister)
	if !found {
		writeError(w, http.StatusBadRequest, "No passkey registration in progress")
		return
	}
	wa, err := s.webAuthn(r)
	if err != nil {
		writeInternalError(w, "webauthn config", err)
		return
	}
	wu, err := s.loadWebAuthnUser(user)
	if err != nil {
		writeInternalError(w, "load passkeys", err)
		return
	}
	credential, err := wa.FinishRegistration(wu, *sessionData, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Passkey registration failed")
		return
	}
	credID := base64.RawURLEncoding.EncodeToString(credential.ID)
	if exists, err := s.store.PasskeyCredentialExists(credID); err != nil {
		writeInternalError(w, "check passkey", err)
		return
	} else if exists {
		writeError(w, http.StatusConflict, "That passkey is already registered")
		return
	}
	credJSON, err := json.Marshal(credential)
	if err != nil {
		writeInternalError(w, "encode passkey", err)
		return
	}
	name := passkeyName(r.URL.Query().Get("name"))
	if _, err := s.store.CreatePasskey(user.ID, credID, string(credJSON), name); err != nil {
		writeInternalError(w, "save passkey", err)
		return
	}
	s.writePasskeyList(w, user.ID)
}

// handlePasskeyList returns the logged-in account's passkeys. Auth: self-service.
func (s *Server) handlePasskeyList(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	s.writePasskeyList(w, user.ID)
}

// handlePasskeyDelete removes one of the logged-in account's passkeys by id.
func (s *Server) handlePasskeyDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid passkey ID")
		return
	}
	deleted, err := s.store.DeletePasskey(user.ID, id)
	if err != nil {
		writeInternalError(w, "delete passkey", err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "Passkey not found")
		return
	}
	s.writePasskeyList(w, user.ID)
}

// writePasskeyList writes a user's current passkeys as the response.
func (s *Server) writePasskeyList(w http.ResponseWriter, userID int64) {
	list, err := s.store.ListPasskeys(userID)
	if err != nil {
		writeInternalError(w, "list passkeys", err)
		return
	}
	writeJSON(w, http.StatusOK, model.PasskeysResponse{Passkeys: list})
}

// handlePasskeyLoginBegin starts a usernameless (discoverable) passkey login.
// Auth: public. The browser picks a resident credential; we return the challenge.
func (s *Server) handlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	// Reuse the login brute-force limiter so an unauthenticated client can't hammer
	// the begin endpoint to amplify challenge/session writes (each begin stashes a
	// WebAuthn challenge in the session store). Same per-IP budget as login/finish.
	ip := clientIP(r)
	if s.limiter.isLimited(ip) {
		writeError(w, http.StatusTooManyRequests, "Too many attempts. Please try again later.")
		return
	}
	wa, err := s.webAuthn(r)
	if err != nil {
		writeInternalError(w, "webauthn config", err)
		return
	}
	assertion, sessionData, err := wa.BeginDiscoverableLogin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not start passkey login")
		return
	}
	if err := s.putWebAuthnSession(r, sessKeyPasskeyLogin, sessionData); err != nil {
		writeInternalError(w, "session", err)
		return
	}
	writeJSON(w, http.StatusOK, assertion)
}

// handlePasskeyLoginFinish verifies a discoverable-login assertion and, on
// success, establishes the session (same as a password login). Auth: public.
func (s *Server) handlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	// Reuse the login brute-force limiter so passkey login can't be hammered.
	if s.limiter.isLimited(ip) {
		writeError(w, http.StatusTooManyRequests, "Too many attempts. Please try again later.")
		return
	}
	sessionData, found := s.popWebAuthnSession(r, sessKeyPasskeyLogin)
	if !found {
		writeError(w, http.StatusBadRequest, "No passkey login in progress")
		return
	}
	wa, err := s.webAuthn(r)
	if err != nil {
		writeInternalError(w, "webauthn config", err)
		return
	}
	// Resolve the asserting account from the credential's user handle, loading its
	// stored credentials so go-webauthn can verify the signature against them.
	handler := func(_, userHandle []byte) (webauthn.User, error) {
		id, ok := userHandleToID(userHandle)
		if !ok {
			return nil, errors.New("invalid user handle")
		}
		u, err := s.store.GetUserByID(id)
		if err != nil {
			return nil, err
		}
		if u == nil || !u.IsActive {
			return nil, errors.New("account not found or inactive")
		}
		return s.loadWebAuthnUser(u)
	}
	user, credential, err := wa.FinishPasskeyLogin(handler, *sessionData, r)
	if err != nil {
		s.limiter.recordFailure(ip)
		slog.Warn("passkey login failed", "ip", ip, "error", err)
		writeError(w, http.StatusUnauthorized, "Passkey login failed")
		return
	}
	wu, ok := user.(*webauthnUser)
	if !ok {
		writeInternalError(w, "passkey user", errors.New("unexpected user type"))
		return
	}
	acct := wu.user

	// Persist the updated signature counter (clone-detection state).
	credID := base64.RawURLEncoding.EncodeToString(credential.ID)
	if j, err := json.Marshal(credential); err == nil {
		if err := s.store.UpdatePasskeyCredential(credID, string(j)); err != nil {
			slog.Error("update passkey credential", "error", err)
		}
	}

	// Establish the session (rotate token to prevent fixation), same as password login.
	_ = s.sessions.RenewToken(r.Context())
	s.sessions.Put(r.Context(), "user_id", acct.ID)
	s.limiter.resetFailures(ip)
	if err := s.store.UpdateLastLogin(acct.ID); err != nil {
		slog.Error("update last login", "error", err, "user_id", acct.ID)
	}
	writeJSON(w, http.StatusOK, model.LoginResponse{Success: true, User: *acct})
}

// passkeyName normalizes a user-supplied passkey label (trim, cap length, default).
func passkeyName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "Passkey"
	}
	if len(name) > 60 {
		name = strings.TrimSpace(name[:60])
	}
	return name
}
