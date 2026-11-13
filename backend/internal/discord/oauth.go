package discord

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"golang.org/x/oauth2"
)

// Discord's addresses for signing in with OAuth 2.0.
const (
	oauthAuthorizeURL = "https://discord.com/oauth2/authorize"
	oauthTokenURL     = "https://discord.com/api/oauth2/token" //nolint:gosec // an address, not a credential
	oauthMeURL        = "https://discord.com/api/v10/users/@me"
)

// OAuth signs people in with their Discord account: the authorization code grant
// (RFC 6749) with a PKCE challenge (RFC 7636), asking only for the identify scope. It
// holds no state of its own; the caller keeps the state and the verifier between the
// consent page and the code coming back.
type OAuth struct {
	// ClientID is the Discord application's id and ClientSecret its secret. With
	// either empty, Enabled is false.
	ClientID, ClientSecret string
	// AuthURL, TokenURL and MeURL are Discord's when empty; a test points them at a
	// stand-in.
	AuthURL, TokenURL, MeURL string
	// HTTP is the client the code is redeemed and the account read through; nil uses
	// one with Timeout.
	HTTP *http.Client
}

// Identity is the Discord account that signed in.
type Identity struct {
	// ID is the account's snowflake, which never changes.
	ID string
	// Username is its unique handle and DisplayName the name it shows, "" when it has
	// none. Either can change; the ID is what links it to an account here.
	Username, DisplayName string
}

// NewState is the state a sign-in sends to Discord and expects back, 128 random bits
// (rand.Text), and NewVerifier its PKCE verifier, which the challenge on the consent
// page is the hash of. The caller keeps both, in the session that began the sign-in,
// until the code comes back.
func NewState() string {
	return rand.Text()
}

// NewVerifier is a sign-in's PKCE verifier (see NewState).
func NewVerifier() string {
	return oauth2.GenerateVerifier()
}

// Enabled reports whether the application's credentials are set.
func (o *OAuth) Enabled() bool {
	return o != nil && o.ClientID != "" && o.ClientSecret != ""
}

// AuthCodeURL is Discord's consent page for a sign-in that comes back to redirectURL
// with state, carrying the S256 challenge of verifier (the only method Discord
// accepts). Someone who already allowed the application isn't asked again.
func (o *OAuth) AuthCodeURL(redirectURL, state, verifier string) string {
	return o.config(redirectURL).AuthCodeURL(state,
		oauth2.SetAuthURLParam("prompt", "none"), oauth2.S256ChallengeOption(verifier))
}

// Identify redeems code, proving with verifier that it comes from the sign-in that
// asked for it, and answers who signed in. redirectURL must be the one AuthCodeURL was
// given.
func (o *OAuth) Identify(ctx context.Context, redirectURL, code, verifier string) (Identity, error) {
	if !o.Enabled() {
		return Identity{}, errors.New("discord sign-in isn't configured")
	}
	if code == "" || verifier == "" {
		return Identity{}, errors.New("no code to redeem")
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, o.client())
	cfg := o.config(redirectURL)
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("redeem the code: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, or(o.MeURL, oauthMeURL), http.NoBody)
	if err != nil {
		return Identity{}, err
	}
	resp, err := cfg.Client(ctx, tok).Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("read the account: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() // read to the end: closing has nothing to report
	if resp.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("read the account: status %d", resp.StatusCode)
	}
	return readIdentity(io.LimitReader(resp.Body, 64<<10))
}

// readIdentity reads Discord's user object strictly: the id must be a snowflake and
// the username a string, or the sign-in fails rather than linking something else.
func readIdentity(r io.Reader) (Identity, error) {
	var u struct {
		ID         string  `json:"id"`
		Username   string  `json:"username"`
		GlobalName *string `json:"global_name"`
	}
	if err := json.NewDecoder(r).Decode(&u); err != nil {
		return Identity{}, fmt.Errorf("read the account: %w", err)
	}
	if !snowflake(u.ID) {
		return Identity{}, errors.New("read the account: no snowflake id")
	}
	if u.Username == "" || !utf8.ValidString(u.Username) {
		return Identity{}, errors.New("read the account: no username")
	}
	id := Identity{ID: u.ID, Username: u.Username}
	if u.GlobalName != nil && utf8.ValidString(*u.GlobalName) {
		id.DisplayName = strings.TrimSpace(*u.GlobalName)
	}
	return id, nil
}

// snowflake reports whether s is a Discord id: 1 to 20 digits.
func snowflake(s string) bool {
	if s == "" || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (o *OAuth) config(redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     o.ClientID,
		ClientSecret: o.ClientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"identify"},
		Endpoint: oauth2.Endpoint{
			AuthURL:   or(o.AuthURL, oauthAuthorizeURL),
			TokenURL:  or(o.TokenURL, oauthTokenURL),
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}

func (o *OAuth) client() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	return defaultClient
}

// or is s, or fallback when s is empty.
func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
