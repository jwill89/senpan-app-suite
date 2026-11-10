package bookclubs

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
		if got := IsAllowedAniListURL(c.raw); got != c.want {
			t.Errorf("IsAllowedAniListURL(%q) = %v; want %v", c.raw, got, c.want)
		}
	}
}

func TestSanitizeSources(t *testing.T) {
	got := SanitizeSources([]Source{
		{Title: " OmegaScans ", URL: " https://omegascans.org/series/x "},
		{Title: "", URL: ""},
		{Title: "No link", URL: "   "},
		{Title: "", URL: "https://example.com"},
	})
	if len(got) != 2 || got[0].Title != "OmegaScans" || got[0].URL != "https://omegascans.org/series/x" || got[1].Title != "" {
		t.Errorf("SanitizeSources = %+v", got)
	}
	if got := SanitizeSources(nil); got == nil || len(got) != 0 {
		t.Errorf("nil input should give an empty, non-nil list; got %#v", got)
	}
}

func TestItemEmbedUsesTheClubsCommentsLabel(t *testing.T) {
	it := Item{Title: "Bloom Into You", Comments: "Loved it.", Format: "Manga", Chapters: "10",
		Sources: []Source{{Title: "AniList", URL: "https://anilist.co/x"}, {URL: "https://b.test"}, {Title: "Blank"}}}
	byName := func(slug string) map[string]string {
		out := map[string]string{}
		for _, f := range ItemEmbed(it, slug).Fields {
			out[f.Name] = f.Value
		}
		return out
	}
	if f := byName("yuri"); f["Drani's Comments"] != "Loved it." || f["Yao's Comments"] != "" {
		t.Errorf("yuri fields = %v", f)
	}
	if f := byName("yaoi"); f["Yao's Comments"] != "Loved it." {
		t.Errorf("yaoi fields = %v", f)
	}
	if f := byName("nope"); f["Comments"] != "Loved it." {
		t.Errorf("an unknown club should label them Comments; fields = %v", f)
	}
	if got := byName("yaoi")["Sources"]; got != "[AniList](https://anilist.co/x)\n[https://b.test](https://b.test)" {
		t.Errorf("sources = %q", got)
	}
}

func TestExists(t *testing.T) {
	if !Exists("yaoi") || !Exists("yuri") || Exists("") || Exists("nope") {
		t.Error("Exists disagrees with the registry")
	}
}
