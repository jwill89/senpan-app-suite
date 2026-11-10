package fonts

import "testing"

func TestNameUnsafe(t *testing.T) {
	safe := []string{
		"Arapey",
		"My Font",
		"Playfair Display",
		"Noto Sans JP",
		"Font-Name 2",
	}
	for _, s := range safe {
		if NameUnsafe(s) {
			t.Errorf("NameUnsafe(%q) = true; want false (legitimate family)", s)
		}
	}

	unsafe := []string{
		"a'}body{display:none", // quote + braces
		"a\"b",                 // double quote
		"a\\b",                 // backslash
		"a;b",                  // semicolon
		"a{b",                  // brace
		"a}b",                  // brace
		"a<b",                  // angle
		"a>b",                  // angle
		"Evil\n}body{}",        // newline (must be \A-escaped in CSS)
		"a\tb",                 // tab (control)
		"a\x7fb",               // DEL (control)
	}
	for _, s := range unsafe {
		if !NameUnsafe(s) {
			t.Errorf("NameUnsafe(%q) = false; want true (CSS-breaking)", s)
		}
	}
}
