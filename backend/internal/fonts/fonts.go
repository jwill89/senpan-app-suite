// Package fonts holds the rules for uploaded fonts that need no files, database or
// network.
package fonts

import "strings"

// NameUnsafe reports whether s holds anything that could break out of a quoted CSS
// string or a font-family name: a control character (a newline included, which CSS
// needs escaped as \A), a quote, a backslash, or one of { } ; < >. A family name
// (set by hand, or taken from an uploaded file's name) is written into kit.css and
// the SPA's uploaded-fonts <style>, so an unescaped one would let a fonts grantee
// inject CSS into outside sites and every player's board; none belongs in a real
// family name. The header-font setting is held to it too.
func NameUnsafe(s string) bool {
	if strings.ContainsAny(s, "'\"\\{};<>") {
		return true
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
