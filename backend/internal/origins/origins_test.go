package origins

import "testing"

func TestNormalize(t *testing.T) {
	good := []struct{ in, want string }{
		{"https://senpan.cafe", "https://senpan.cafe"},
		{"  HTTPS://Senpan.Cafe/ ", "https://senpan.cafe"},
		{"http://localhost:5173", "http://localhost:5173"},
		{"https://*.senpan.cafe", "https://*.senpan.cafe"},
		{"https://*.Senpan.cafe:8443/", "https://*.senpan.cafe:8443"},
		{"https://a.b.example.com", "https://a.b.example.com"},
	}
	for _, c := range good {
		got, err := Normalize(c.in)
		if err != nil || got != c.want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, in := range []string{
		"", "   ", "*", "https://*", "https://*.com.*", "https://foo.*.example.com", "https://fo*.example.com",
		"senpan.cafe", "ftp://senpan.cafe", "https://", "https://senpan.cafe/path", "https://senpan.cafe?x=1",
		"https://senpan.cafe#top", "https://user@senpan.cafe", "https://sen pan.cafe", "*.senpan.cafe", "https://*.",
	} {
		if got, err := Normalize(in); err == nil {
			t.Errorf("Normalize(%q) = %q; want an error", in, got)
		}
	}
}

func TestAllowed(t *testing.T) {
	list := []string{"https://senpan.cafe", "https://*.senpan.cafe", "http://localhost:5173", "https://*.dev.example.com:8443"}
	cases := map[string]bool{
		"https://senpan.cafe":            true,
		"https://SENPAN.cafe":            true,
		"https://tea.senpan.cafe":        true,
		"https://a.b.senpan.cafe":        true,
		"http://senpan.cafe":             false, // another scheme
		"http://tea.senpan.cafe":         false,
		"https://senpan.cafe:8443":       false, // another port
		"https://tea.senpan.cafe:8443":   false,
		"https://notsenpan.cafe":         false,
		"https://evilsenpan.cafe":        false,
		"https://senpan.cafe.evil.com":   false,
		"https://evil.com":               false,
		"http://localhost:5173":          true,
		"http://localhost:5174":          false,
		"https://x.dev.example.com:8443": true,
		"https://dev.example.com:8443":   false, // the apex is not a subdomain
		"https://x.dev.example.com":      false,
		"":                               false,
		"null":                           false,
		"*":                              false,
	}
	for origin, want := range cases {
		if got := Allowed(list, origin); got != want {
			t.Errorf("Allowed(%q) = %v; want %v", origin, got, want)
		}
	}
	if Allowed(nil, "https://senpan.cafe") {
		t.Error("an empty list allowed an origin")
	}
	if Allowed([]string{"*", "https://*"}, "https://senpan.cafe") {
		t.Error("a bare wildcard entry allowed an origin")
	}
}
