package server

import "testing"

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
