package themes

import (
	"strings"
	"testing"
)

func TestCSS(t *testing.T) {
	// Emitted in canonical :root order regardless of input order, and only known
	// tokens are included.
	css := CSS(map[string]string{
		"accent":  "#fff",
		"page-bg": "#000",
		"bogus":   "drop-me",
	})
	if css != ":root{--page-bg:#000;--accent:#fff;}" {
		t.Errorf("css = %q", css)
	}

	if CSS(nil) != "" {
		t.Error("nil tokens should generate no CSS")
	}
	if CSS(map[string]string{"unknown": "x"}) != "" {
		t.Error("only-unknown tokens should generate no CSS")
	}
}

func TestCSSStripsInjection(t *testing.T) {
	css := CSS(map[string]string{"accent": "red; } body{ display:none } /*"})
	// The value can't break out of its declaration: no extra braces or semicolons
	// survive inside the value, so a single --accent declaration remains.
	if strings.Contains(css, "body{") || strings.Contains(css, "display:none") && strings.Contains(css, "}") && strings.Count(css, "}") > 1 {
		t.Errorf("injection not neutralized: %q", css)
	}
	if strings.Count(css, "{") != 1 || strings.Count(css, "}") != 1 {
		t.Errorf("expected exactly one rule block, got %q", css)
	}
}

func TestSanitize(t *testing.T) {
	out := Sanitize(map[string]string{
		"--page-bg": "  #abc  ", // leading "--" tolerated; trimmed
		"accent":    "red",
		"unknown":   "nope", // dropped (not in allowlist)
		"shadow":    "   ",  // dropped (empty after trim)
	})
	if out["page-bg"] != "#abc" {
		t.Errorf("page-bg = %q; want #abc", out["page-bg"])
	}
	if out["accent"] != "red" {
		t.Errorf("accent = %q", out["accent"])
	}
	if _, ok := out["unknown"]; ok {
		t.Error("unknown token should be dropped")
	}
	if _, ok := out["shadow"]; ok {
		t.Error("empty-value token should be dropped")
	}
}

func TestValidFlourish(t *testing.T) {
	valid := []string{
		"",                                  // unset
		"images/flourishes/a.svg",           // canonical picker output
		"images/flourishes/Board.svg",       // uppercase filename allowed
		"images/flourishes/My Flourish.svg", // space in filename allowed
		"images/flourishes/swirl.SVG",       // case-insensitive extension
		"images/custom_cat/x.svg",           // underscore category
	}
	for _, p := range valid {
		if !ValidFlourish(p) {
			t.Errorf("ValidFlourish(%q) = false; want true", p)
		}
	}

	invalid := []string{
		"data:image/svg+xml,<svg onload=alert(1)>", // data URI XSS vector
		"https://evil.example/x.svg",               // external URL
		"//evil.example/x.svg",                     // scheme-relative URL
		"../../etc/passwd.svg",                     // traversal
		"images/flourishes/x.png",                  // wrong extension
		"images/flourishes/x",                      // no extension
		"images/flourishes/",                       // no filename
		"images/x.svg",                             // missing category segment
		"images/Bad-Cat/x.svg",                     // category not a slug
		"images/flourishes/a.svg\n<script>",        // trailing injection
	}
	for _, p := range invalid {
		if ValidFlourish(p) {
			t.Errorf("ValidFlourish(%q) = true; want false", p)
		}
	}
}
