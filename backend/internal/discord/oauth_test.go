package discord

import (
	"net/url"
	"strings"
	"testing"
)

// Discord's account is read strictly: an id that isn't a snowflake string, or a
// missing username, fails the sign-in rather than linking something else.
func TestReadIdentity(t *testing.T) {
	for _, c := range []struct {
		body string
		want Identity
		ok   bool
	}{
		{`{"id":"80351110224678912","username":"lune","global_name":"Lune"}`, Identity{"80351110224678912", "lune", "Lune"}, true},
		{`{"id":"80351110224678912","username":"lune","global_name":null}`, Identity{"80351110224678912", "lune", ""}, true},
		{`{"id":"80351110224678912","username":"lune"}`, Identity{"80351110224678912", "lune", ""}, true},
		{`{"id":80351110224678912,"username":"lune"}`, Identity{}, false},
		{`{"id":"","username":"lune"}`, Identity{}, false},
		{`{"id":"8035x","username":"lune"}`, Identity{}, false},
		{`{"id":"123456789012345678901","username":"lune"}`, Identity{}, false},
		{`{"id":"80351110224678912","username":""}`, Identity{}, false},
		{`{"id":"80351110224678912","username":true}`, Identity{}, false},
		{`not json`, Identity{}, false},
	} {
		got, err := readIdentity(strings.NewReader(c.body))
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("readIdentity(%s) = %+v, %v; want %+v, ok %v", c.body, got, err, c.want, c.ok)
		}
	}
}

// The consent page asks for identify alone, with an S256 challenge of the verifier
// and the return address given.
func TestAuthCodeURL(t *testing.T) {
	o := &OAuth{ClientID: "1563399760994635906", ClientSecret: "x"}
	verifier := NewVerifier()
	got, err := url.Parse(o.AuthCodeURL("https://apps.example/api/v1/auth/discord/callback", "the-state", verifier))
	if err != nil {
		t.Fatal(err)
	}
	if got.Scheme+"://"+got.Host+got.Path != oauthAuthorizeURL {
		t.Errorf("consent page %s; want %s", got, oauthAuthorizeURL)
	}
	q := got.Query()
	for key, want := range map[string]string{
		"client_id":             "1563399760994635906",
		"response_type":         "code",
		"scope":                 "identify",
		"state":                 "the-state",
		"redirect_uri":          "https://apps.example/api/v1/auth/discord/callback",
		"code_challenge_method": "S256",
		"prompt":                "none",
	} {
		if q.Get(key) != want {
			t.Errorf("%s = %q; want %q", key, q.Get(key), want)
		}
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge") == verifier {
		t.Errorf("code_challenge %q is not the verifier's hash", q.Get("code_challenge"))
	}
	if q.Has("client_secret") {
		t.Error("the consent page carries the client secret")
	}
	if first, second := NewState(), NewState(); first == second {
		t.Error("two states alike")
	}
}

// Without both credentials, sign-in is off.
func TestOAuthEnabled(t *testing.T) {
	var none *OAuth
	for _, o := range []*OAuth{none, {}, {ClientID: "1"}, {ClientSecret: "s"}} {
		if o.Enabled() {
			t.Errorf("%+v is enabled", o)
		}
	}
	if !(&OAuth{ClientID: "1", ClientSecret: "s"}).Enabled() {
		t.Error("credentials set, but not enabled")
	}
}
