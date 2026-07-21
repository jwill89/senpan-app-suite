package server

import "testing"

// TestIsAllowedAniListURL guards the SSRF allowlist that gates the admin-set
// anilist_api_url (and every redirect hop the AniList client follows). Only
// https URLs on anilist.co or a subdomain may pass; internal hosts, look-alike
// domains, and non-https schemes must be rejected so the server's outbound
// lookup can't be aimed at a private service or cloud metadata endpoint.
func TestIsAllowedAniListURL(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://graphql.anilist.co", true},
		{"https://anilist.co", true},
		{"https://anilist.co/graphql", true}, // path is ignored
		{"https://GraphQL.AniList.co", true}, // host compared case-insensitively

		{"http://anilist.co", false},           // non-https
		{"https://evil.com", false},            // unrelated host
		{"https://anilist.co.evil.com", false}, // suffix trick: not under anilist.co
		{"https://evilanilist.co", false},      // no dot boundary before anilist.co
		{"https://127.0.0.1", false},           // loopback
		{"https://169.254.169.254", false},     // cloud metadata
		{"https://localhost", false},           // internal name
		{"https://", false},                    // no host
		{"ftp://anilist.co", false},            // wrong scheme
		{"not a url", false},                   // unparseable
		{"", false},                            // empty
	}
	for _, c := range cases {
		if got := isAllowedAniListURL(c.raw); got != c.want {
			t.Errorf("isAllowedAniListURL(%q) = %v; want %v", c.raw, got, c.want)
		}
	}
}
