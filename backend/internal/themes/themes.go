// Package themes holds the rules for a theme: a set of design-token overrides (a
// map of token name to CSS value), never free-form CSS. The stylesheet a theme
// applies is generated from its tokens as one ":root{...}" block (CSS), so a theme
// can only retint the app's design tokens: it can't break the layout for every
// player, and the base stylesheet's class names can be refactored freely.
//
// Pure code: no database, clock or network, so its tests need no setup.
package themes

import (
	"encoding/json"
	"regexp"
	"strings"
)

// TokenOrder is the canonical list of theme-overridable design tokens, in the
// order they appear in :root (frontend styles/tokens.css), without their leading
// "--". It is the one allowlist: only these names are stored or emitted, and CSS is
// generated in this order. --radius and --header-font are left out (the radius is a
// structural default; the header font is an app setting).
var TokenOrder = []string{
	// Backgrounds
	"page-bg", "panel-bg", "panel-raised-bg", "control-border", "input-bg",
	// Accents & actions
	"accent", "accent-hover", "accent-2", "accent-2-hover", "highlight",
	// Text
	"text", "text-muted", "text-on-accent", "text-on-fill",
	// Status
	"success", "danger", "warning",
	// Bingo board
	"board-cell-bg", "board-cell-hover-bg", "board-free-bg", "board-gradient-start", "board-gradient-end",
	// Effects
	"modal-overlay", "shadow", "highlight-glow",
}

// known is the allowlist built from TokenOrder.
var known = func() map[string]bool {
	m := make(map[string]bool, len(TokenOrder))
	for _, n := range TokenOrder {
		m[n] = true
	}
	return m
}()

// valueStripper removes what could let a value break out of its `--name: value;`
// declaration, keeping it to one inert declaration value.
var valueStripper = strings.NewReplacer(
	";", "", "{", "", "}", "", "/*", "", "*/", "", "\n", "", "\r", "", "\t", "",
)

// MaxValueLength is the longest a token's value is kept.
const MaxValueLength = 64

// sanitizeValue trims a value, strips anything that could inject CSS, and caps its
// length; "" when nothing usable is left.
func sanitizeValue(v string) string {
	v = strings.TrimSpace(valueStripper.Replace(strings.TrimSpace(v)))
	if len(v) > MaxValueLength {
		v = strings.TrimSpace(v[:MaxValueLength])
	}
	return v
}

// Sanitize returns a clean token map: only known names (a leading "--" tolerated),
// each value stripped of anything that could inject CSS, and empty values dropped.
// The result is safe to store and to render.
func Sanitize(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		name := strings.TrimPrefix(strings.TrimSpace(k), "--")
		if !known[name] {
			continue
		}
		if sv := sanitizeValue(v); sv != "" {
			out[name] = sv
		}
	}
	return out
}

// CSS renders a token map as a ":root{...}" stylesheet, known tokens only, in
// canonical order; "" when it has none, so a default theme injects nothing.
func CSS(tokens map[string]string) string {
	var b strings.Builder
	b.WriteString(":root{")
	wrote := false
	for _, name := range TokenOrder {
		v := sanitizeValue(tokens[name])
		if v == "" {
			continue
		}
		b.WriteString("--" + name + ":" + v + ";")
		wrote = true
	}
	if !wrote {
		return ""
	}
	return b.String() + "}"
}

// Marshal is a token map as its stored JSON, sanitized first.
func Marshal(tokens map[string]string) (string, error) {
	buf, err := json.Marshal(Sanitize(tokens))
	return string(buf), err
}

// Unmarshal is a stored token map; an empty or unreadable value is no tokens.
func Unmarshal(raw string) map[string]string {
	tokens := map[string]string{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &tokens)
	}
	return tokens
}

// flourishPath is an SVG in one of the image categories.
var flourishPath = regexp.MustCompile(`^images/[a-z0-9_]+/[^/\\]+(?i:\.svg)$`)

// ValidFlourish reports whether p may be a theme's flourish: "" (none), or a
// relative images/<category>/<file>.svg path. A flourish is fetched and inlined on
// the public board (CornerFlourish.vue), so this keeps out a data: URI or an outside
// URL that never went through the upload's SVG sanitizer. The category segment is a
// slug; the file name stays permissive, so an uploaded name with capitals, spaces or
// Unicode passes; the extension is matched in any case.
func ValidFlourish(p string) bool {
	return p == "" || flourishPath.MatchString(p)
}
