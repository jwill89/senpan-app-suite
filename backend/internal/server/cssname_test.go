package server

import "testing"

func TestCSSNameUnsafe(t *testing.T) {
	safe := []string{
		"Arapey",
		"My Font",
		"Playfair Display",
		"Noto Sans JP",
		"Font-Name 2",
	}
	for _, s := range safe {
		if cssNameUnsafe(s) {
			t.Errorf("cssNameUnsafe(%q) = true; want false (legitimate family)", s)
		}
	}

	unsafe := []string{
		"a'}body{display:none",  // quote + braces
		"a\"b",                  // double quote
		"a\\b",                  // backslash
		"a;b",                   // semicolon
		"a{b",                   // brace
		"a}b",                   // brace
		"a<b",                   // angle
		"a>b",                   // angle
		"Evil\n}body{}",         // newline (must be \A-escaped in CSS)
		"a\tb",                  // tab (control)
		"a\x7fb",                // DEL (control)
	}
	for _, s := range unsafe {
		if !cssNameUnsafe(s) {
			t.Errorf("cssNameUnsafe(%q) = false; want true (CSS-breaking)", s)
		}
	}
}

func TestSafeFontName_RejectsCSSBreakingBase(t *testing.T) {
	// A benign font filename is accepted and returned unchanged.
	if n, ok := safeFontName("My Font.ttf"); !ok || n != "My Font.ttf" {
		t.Errorf("safeFontName(%q) = (%q, %v); want (%q, true)", "My Font.ttf", n, ok, "My Font.ttf")
	}
	// A filename whose base carries CSS-breaking characters is rejected, because
	// the base becomes the default CSS font-family.
	for _, name := range []string{"x'}.ttf", "a;b.otf", "ev{il.woff2"} {
		if _, ok := safeFontName(name); ok {
			t.Errorf("safeFontName(%q) accepted; want rejected", name)
		}
	}
}
