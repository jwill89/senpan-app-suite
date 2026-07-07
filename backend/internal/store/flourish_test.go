package store

import "testing"

func TestValidFlourishPath(t *testing.T) {
	valid := []string{
		"",                                // unset
		"images/flourishes/a.svg",         // canonical picker output
		"images/flourishes/Board.svg",     // uppercase filename allowed
		"images/flourishes/My Flourish.svg", // space in filename allowed
		"images/flourishes/swirl.SVG",     // case-insensitive extension
		"images/custom_cat/x.svg",         // underscore category
	}
	for _, p := range valid {
		if !ValidFlourishPath(p) {
			t.Errorf("ValidFlourishPath(%q) = false; want true", p)
		}
	}

	invalid := []string{
		"data:image/svg+xml,<svg onload=alert(1)>", // data URI XSS vector
		"https://evil.example/x.svg",               // external URL
		"//evil.example/x.svg",                     // scheme-relative URL
		"../../etc/passwd.svg",                      // traversal
		"images/flourishes/x.png",                  // wrong extension
		"images/flourishes/x",                      // no extension
		"images/flourishes/",                       // no filename
		"images/x.svg",                             // missing category segment
		"images/Bad-Cat/x.svg",                     // category not a slug
		"images/flourishes/a.svg\n<script>",        // trailing injection
	}
	for _, p := range invalid {
		if ValidFlourishPath(p) {
			t.Errorf("ValidFlourishPath(%q) = true; want false", p)
		}
	}
}
