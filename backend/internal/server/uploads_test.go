package server

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSafeUploadName covers the shared upload-name guard that turns an
// attacker-controlled multipart filename into a safe on-disk (and URL) basename.
// The security invariant is: whenever a name is accepted, the result is a bare
// basename — no path separators, no traversal, no leading-dot dotfile — with an
// allowed extension. Traversal attempts must either be rejected or reduced to
// that safe basename.
func TestSafeUploadName(t *testing.T) {
	// The image allow predicate (raster set, no SVG) is representative.
	allow := isAllowedImageExt

	// Deterministic cases: same result on every platform.
	cases := []struct {
		name     string
		wantOK   bool
		wantName string // only checked when wantOK
	}{
		{"../../etc/passwd", false, ""},        // Base → "passwd", no ext → rejected
		{"foo/bar.png", true, "bar.png"},       // sub-path stripped to basename
		{".htaccess", false, ""},               // leading-dot dotfile rejected
		{"", false, ""},                        // empty rejected
		{".", false, ""},                       // "." rejected
		{"..", false, ""},                      // ".." rejected
		{"IMAGE.PNG", true, "IMAGE.PNG"},       // uppercase name kept; ext matched case-insensitively
		{"photo.gif", true, "photo.gif"},       // plain allowed image
		{"script.svg", false, ""},              // SVG not in the raster allow set
		{"note.txt", false, ""},                // disallowed extension
		{"noext", false, ""},                   // no extension
		{"café.png", true, "café.png"},         // unicode basename accepted
		{"  spaced.png  ", true, "spaced.png"}, // surrounding whitespace trimmed
	}
	for _, c := range cases {
		got, ok := safeUploadName(c.name, allow)
		if ok != c.wantOK {
			t.Errorf("safeUploadName(%q) ok = %v; want %v (got %q)", c.name, ok, c.wantOK, got)
			continue
		}
		if ok {
			if got != c.wantName {
				t.Errorf("safeUploadName(%q) = %q; want %q", c.name, got, c.wantName)
			}
			assertSafeBasename(t, c.name, got)
		}
	}

	// Backslash traversal ("..\..\x.png") is platform-dependent: on Windows the
	// backslash is a path separator (stripped to "x.png"); on POSIX it is a
	// literal filename character (caught by the separator check → rejected).
	// Either way the traversal must be neutralized, so assert only the invariant.
	if got, ok := safeUploadName(`..\..\x.png`, allow); ok {
		assertSafeBasename(t, `..\..\x.png`, got)
	}
}

// assertSafeBasename fails if an accepted upload name is anything other than a
// safe bare basename with no separators or traversal.
func assertSafeBasename(t *testing.T, in, got string) {
	t.Helper()
	if got == "" || got == "." || got == ".." {
		t.Errorf("safeUploadName(%q) accepted unsafe name %q", in, got)
	}
	if strings.ContainsAny(got, `/\`) {
		t.Errorf("safeUploadName(%q) accepted name with separator: %q", in, got)
	}
	if strings.HasPrefix(got, ".") {
		t.Errorf("safeUploadName(%q) accepted dotfile: %q", in, got)
	}
	if filepath.Base(got) != got {
		t.Errorf("safeUploadName(%q) result is not a bare basename: %q", in, got)
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct {
		in   string
		sep  byte
		want string
	}{
		{"Hello World", '-', "hello-world"},
		{"  Trim  Me  ", '-', "trim-me"},
		{"a__b--c  d", '-', "a-b-c-d"}, // separators collapse
		{"***", '-', ""},               // nothing keepable
		{"UPPER_case-123", '-', "upper-case-123"},
		{"café ölà", '-', "caf-l"}, // non-ascii letters dropped
		{"Hello World", '_', "hello_world"},
		{"a - b", '_', "a_b"},
		{"-leading-and-trailing-", '-', "leading-and-trailing"},
	}
	for _, c := range cases {
		if got := slugify(c.in, c.sep); got != c.want {
			t.Errorf("slugify(%q, %q) = %q; want %q", c.in, c.sep, got, c.want)
		}
	}
}

func TestValidSlug(t *testing.T) {
	cases := []struct {
		name string
		sep  byte
		want bool
	}{
		{"", '-', false},            // empty
		{"hello-world", '-', true},  // canonical hyphen slug
		{"hello_world", '-', false}, // underscore not allowed when sep is '-'
		{"hello_world", '_', true},  // underscore allowed when sep is '_'
		{"Hello", '-', false},       // uppercase rejected
		{"abc123", '-', true},       // alphanumerics only
		{"a-b", '-', true},
		{"a b", '-', false},  // space rejected
		{"../x", '-', false}, // traversal chars rejected
		{`..\x`, '-', false}, // backslash rejected
		{"café", '-', false}, // non-ascii rejected
	}
	for _, c := range cases {
		if got := validSlug(c.name, c.sep); got != c.want {
			t.Errorf("validSlug(%q, %q) = %v; want %v", c.name, c.sep, got, c.want)
		}
	}
}
