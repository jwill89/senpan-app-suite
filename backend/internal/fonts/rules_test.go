package fonts

import (
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func TestSafeName(t *testing.T) {
	valid := map[string]string{
		"Roboto.ttf":      "Roboto.ttf",
		"  spaced.otf  ":  "spaced.otf",
		"font.WOFF2":      "font.WOFF2",  // the extension check ignores case
		"a/b/nested.woff": "nested.woff", // directories are stripped to the base name
		"My Font.ttf":     "My Font.ttf",
	}
	for in, want := range valid {
		if got, ok := SafeName(in); !ok || got != want {
			t.Errorf("SafeName(%q) = (%q, %v); want (%q, true)", in, got, ok, want)
		}
	}
	for _, in := range []string{
		"", "   ", "..", "noext", "evil.exe", "shell.ttf.sh",
		// The base becomes the default CSS family, so one that could break out of it is refused.
		"x'}.ttf", "a;b.otf", "ev{il.woff2",
	} {
		if got, ok := SafeName(in); ok {
			t.Errorf("SafeName(%q) = (%q, true); want refused", in, got)
		}
	}
}

func TestGroupsCollectFormatsByBaseName(t *testing.T) {
	groups := Groups([]string{"Jasper.otf", "jasper.WOFF2", "Zed.ttf"})
	if len(groups) != 2 {
		t.Fatalf("groups = %+v; want 2", groups)
	}
	g := groups[0]
	if g.Key != "jasper" || g.Base != "Jasper" || len(g.Files) != 2 {
		t.Errorf("first group = %+v", g)
	}
	if !g.HasUploadedWOFF2() || g.VariantByType("WOFF2") != "jasper.WOFF2" || g.VariantByType("TTF") != "" {
		t.Errorf("variants of %+v are wrong", g)
	}
	if got := g.ConvertSource(); got != "Jasper.otf" {
		t.Errorf("ConvertSource = %q; want Jasper.otf", got)
	}
}

func TestServedVariant(t *testing.T) {
	bucket := int64(100)
	both := Group{Key: "f", Base: "F", Files: []string{"F.ttf", "F.otf"}}

	tok, label := ServedVariant(testSecret, both, Meta{}, false, bucket)
	if label != "TTF" || tok != FileToken(testSecret, "F.ttf", bucket) {
		t.Errorf("auto without WOFF2 = %q %q; want the TTF", label, tok)
	}
	tok, label = ServedVariant(testSecret, both, Meta{}, true, bucket)
	if label != "WOFF2" || tok != DerivedToken(testSecret, "f", bucket) {
		t.Errorf("auto with a converted copy = %q %q; want the copy", label, tok)
	}
	tok, label = ServedVariant(testSecret, both, Meta{Serve: "OTF"}, true, bucket)
	if label != "OTF" || tok != FileToken(testSecret, "F.otf", bucket) {
		t.Errorf("explicit OTF = %q %q", label, tok)
	}
	// A saved choice that no longer exists falls back to automatic.
	if _, label = ServedVariant(testSecret, both, Meta{Serve: "EOT"}, false, bucket); label != "TTF" {
		t.Errorf("stale choice served %q; want the automatic TTF", label)
	}
	if tok, label = ServedVariant(testSecret, Group{Key: "e"}, Meta{}, false, bucket); tok != "" || label != "" {
		t.Errorf("empty font served %q %q", tok, label)
	}
}

func TestResolveAcceptsTwoBuckets(t *testing.T) {
	now := time.Unix(BucketSeconds*50+10, 0)
	bucket := TokenBucket(now)
	groups := []Group{{Key: "old", Base: "Old", Files: []string{"Old.ttf"}}, {Key: "conv", Base: "Conv", Files: []string{"Conv.ttf"}}}
	converted := func(key string) bool { return key == "conv" }

	if r, ok := Resolve(testSecret, groups, converted, FileToken(testSecret, "Old.ttf", bucket-1), now); !ok || r.File != "Old.ttf" {
		t.Errorf("previous bucket = %+v, %v; want Old.ttf", r, ok)
	}
	if _, ok := Resolve(testSecret, groups, converted, FileToken(testSecret, "Old.ttf", bucket-2), now); ok {
		t.Error("a token two buckets old should have expired")
	}
	if r, ok := Resolve(testSecret, groups, converted, DerivedToken(testSecret, "conv", bucket), now); !ok || !r.Derived || r.GroupKey != "conv" {
		t.Errorf("derived token = %+v, %v", r, ok)
	}
	if _, ok := Resolve(testSecret, groups, converted, DerivedToken(testSecret, "old", bucket), now); ok {
		t.Error("a converted copy that isn't on disk shouldn't resolve")
	}
	if _, ok := Resolve([]byte("another secret"), groups, converted, FileToken(testSecret, "Old.ttf", bucket), now); ok {
		t.Error("a token made with another secret resolved")
	}
	for _, bad := range []string{"", strings.Repeat("a", MaxTokenLen+1)} {
		if _, ok := Resolve(testSecret, groups, converted, bad, now); ok {
			t.Errorf("token %q resolved", bad)
		}
	}
}

func TestNormalizeOrigin(t *testing.T) {
	valid := map[string]string{
		"https://mysite.carrd.co":   "https://mysite.carrd.co",
		"https://MySite.Carrd.co/":  "https://mysite.carrd.co",
		"http://localhost:5173":     "http://localhost:5173",
		"  https://a.example.com ":  "https://a.example.com", //nolint:gocritic // the padding is the input under test
		"HTTPS://UPPER.example.com": "https://upper.example.com",
	}
	for in, want := range valid {
		if got, ok := NormalizeOrigin(in); !ok || got != want {
			t.Errorf("NormalizeOrigin(%q) = (%q, %v); want (%q, true)", in, got, ok, want)
		}
	}
	for _, in := range []string{
		"", "mysite.carrd.co", "https://site.com/path", "https://site.com?q=1",
		"https://site.com#f", "ftp://site.com", "https://user:pw@site.com", "https://",
	} {
		if got, ok := NormalizeOrigin(in); ok {
			t.Errorf("NormalizeOrigin(%q) = (%q, true); want refused", in, got)
		}
	}
}

func TestNormalizeOrigins(t *testing.T) {
	got, _, ok := NormalizeOrigins([]string{"https://MySite.Carrd.co/", "https://mysite.carrd.co", " ", " https://other.example.com "})
	if !ok || len(got) != 2 || got[0] != "https://mysite.carrd.co" || got[1] != "https://other.example.com" {
		t.Errorf("got %v, %v; want the two sites, deduplicated", got, ok)
	}
	if _, bad, ok := NormalizeOrigins([]string{"https://fine.example.com", "nope"}); ok || bad != "nope" {
		t.Errorf("bad = %q, ok = %v; want nope refused", bad, ok)
	}
}

func TestRequestAllowed(t *testing.T) {
	m := Meta{Origins: []string{"https://mysite.carrd.co"}}
	const host = "apps.senpan.cafe"
	for _, tc := range []struct {
		name    string
		req     Request
		echo    string
		allowed bool
	}{
		{"no site named at all", Request{Host: host}, "", false},
		{"allowed origin", Request{Host: host, Origin: "https://mysite.carrd.co"}, "https://mysite.carrd.co", true},
		{"allowed origin in other case", Request{Host: host, Origin: "https://MySite.Carrd.co"}, "https://MySite.Carrd.co", true},
		{"foreign origin", Request{Host: host, Origin: "https://other.carrd.co"}, "", false},
		{"same-host origin", Request{Host: host, Origin: "https://apps.senpan.cafe"}, "https://apps.senpan.cafe", true},
		{"dev origin on another port", Request{Host: "localhost:8080", Origin: "http://localhost:5173"}, "http://localhost:5173", true},
		{"same-origin fetch metadata", Request{Host: host, SecFetchSite: "same-origin"}, "", true},
		{"same-host referer", Request{Host: host, Referer: "https://apps.senpan.cafe/admin"}, "", true},
		{"safari: allowed page", Request{Host: host, Referer: "https://MySite.Carrd.co/about", SecFetchSite: "cross-site"}, "", true},
		{"safari: foreign page", Request{Host: host, Referer: "https://other.carrd.co/", SecFetchSite: "cross-site"}, "", false},
		{"a refused origin isn't rescued by the referer", Request{Host: host, Origin: "https://other.carrd.co", Referer: "https://mysite.carrd.co/"}, "", false},
	} {
		echo, allowed := RequestAllowed(tc.req, m)
		if echo != tc.echo || allowed != tc.allowed {
			t.Errorf("%s: got (%q, %v); want (%q, %v)", tc.name, echo, allowed, tc.echo, tc.allowed)
		}
	}
}

func TestKitSite(t *testing.T) {
	for _, tc := range []struct{ referer, want string }{
		{"", ""},
		{"https://apps.senpan.cafe/page", ""},
		{"https://MySite.Carrd.co/about", "https://mysite.carrd.co"},
	} {
		if got := KitSite(tc.referer, "apps.senpan.cafe"); got != tc.want {
			t.Errorf("KitSite(%q) = %q; want %q", tc.referer, got, tc.want)
		}
	}
}

func TestKitCSS(t *testing.T) {
	css := KitCSS([]KitFont{
		{Family: "My Font", Token: "abc.ttf"},
		{Family: "Second", Token: "def.woff2"},
		{Family: "a'}body{display:none", Token: "bad.ttf"}, // could inject CSS: left out
		{Family: "", Token: "none.ttf"},
		{Family: "No Token", Token: ""},
	})
	if !strings.Contains(css, "font-family:'My Font';src:url('f/abc.ttf') format('truetype')") {
		t.Errorf("missing the first rule:\n%s", css)
	}
	if !strings.Contains(css, "font-family:'Second'") || !strings.Contains(css, "format('woff2')") {
		t.Errorf("missing the second rule:\n%s", css)
	}
	if strings.Count(css, "@font-face") != 2 || strings.Contains(css, "display:none") {
		t.Errorf("unsafe or empty fonts were written:\n%s", css)
	}
}
