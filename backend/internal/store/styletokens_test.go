package store

import (
	"strings"
	"testing"
)

func TestTokensToCSS(t *testing.T) {
	// Emitted in canonical :root order regardless of input order, and only known
	// tokens are included.
	css := TokensToCSS(map[string]string{
		"accent":  "#fff",
		"page-bg": "#000",
		"bogus":   "drop-me",
	})
	if css != ":root{--page-bg:#000;--accent:#fff;}" {
		t.Errorf("css = %q", css)
	}

	if TokensToCSS(nil) != "" {
		t.Error("nil tokens should generate no CSS")
	}
	if TokensToCSS(map[string]string{"unknown": "x"}) != "" {
		t.Error("only-unknown tokens should generate no CSS")
	}
}

func TestTokensToCSS_StripsInjection(t *testing.T) {
	css := TokensToCSS(map[string]string{"accent": "red; } body{ display:none } /*"})
	// The value can't break out of its declaration: no extra braces or semicolons
	// survive inside the value, so a single --accent declaration remains.
	if strings.Contains(css, "body{") || strings.Contains(css, "display:none") && strings.Contains(css, "}") && strings.Count(css, "}") > 1 {
		t.Errorf("injection not neutralized: %q", css)
	}
	if strings.Count(css, "{") != 1 || strings.Count(css, "}") != 1 {
		t.Errorf("expected exactly one rule block, got %q", css)
	}
}

func TestSanitizeTokens(t *testing.T) {
	out := SanitizeTokens(map[string]string{
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

func TestParseRootTokens(t *testing.T) {
	css := `/* a legacy theme */
:root {
    --page-bg: #1a1c17; /* bg */
    --accent: #d6bdae;
    --not-a-real-token: #fff;
}
.some-class { color: red; }
:root { --ignored-second-block: #000; }`
	tokens := parseRootTokens(css)
	if tokens["page-bg"] != "#1a1c17" {
		t.Errorf("page-bg = %q", tokens["page-bg"])
	}
	if tokens["accent"] != "#d6bdae" {
		t.Errorf("accent = %q", tokens["accent"])
	}
	if _, ok := tokens["not-a-real-token"]; ok {
		t.Error("unknown token should be ignored")
	}
	// Only the first :root block is parsed.
	if _, ok := tokens["ignored-second-block"]; ok {
		t.Error("second :root block should not be parsed")
	}
}
