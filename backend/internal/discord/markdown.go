package discord

import (
	"regexp"
	"strings"
)

var (
	// brBreakRe matches a <br> hard-break tag plus any inline spaces and a single
	// trailing newline, so the whole thing collapses to one line break rather than a
	// blank line.
	brBreakRe = regexp.MustCompile(`(?i)<br\s*/?>[ \t]*\r?\n?`)
	// backslashBreakRe matches a backslash used as a markdown hard break: a "\"
	// immediately before a (CR)LF. The captured newline is preserved.
	backslashBreakRe = regexp.MustCompile(`\\(\r?\n)`)
	// blankRe matches a run of three or more newlines.
	blankRe = regexp.MustCompile(`\n{3,}`)
	// listItemRe matches a bullet (-, *, +) or ordered (1. / 1)) list item line.
	// The required space after the marker keeps inline "*emphasis*" or a bare "-"
	// from being mistaken for a list item.
	listItemRe = regexp.MustCompile(`^[ \t]*(?:[-*+]|\d+[.)])[ \t]`)
	// timestampEscapeRe matches a Discord timestamp token whose "<"/">" may have
	// been backslash-escaped by the markdown serializer (e.g. "\<t:123:F\>").
	timestampEscapeRe = regexp.MustCompile(`\\?<t:(\d+)(:[tTdDfFR])?\\?>`)
	// emojiShortcodeEscapeRe matches an emoji shortcode carrying at least one
	// underscore, escaped or not (":video\_game:", ":white\_check\_mark:"). The name
	// charset is Discord's own: letters, digits, underscore, and the "+" and "-" of
	// ":+1:" and ":non-potable_water:". Requiring an underscore keeps it off the
	// single-word shortcodes that were never broken, and requiring name characters on
	// both sides of every underscore keeps it off prose like "a: b_c:" (the space ends
	// the match).
	emojiShortcodeEscapeRe = regexp.MustCompile(`:[A-Za-z0-9+\-]+(?:\\?_[A-Za-z0-9+\-]+)+:`)
)

// Markdown normalizes the Markdown the site's editor stores into the flavor Discord
// renders, repairing what the editor's serializer leaves behind:
//
//   - a hard line break written as a literal <br> tag (with the source newline the
//     serializer follows it with): one newline.
//   - a hard line break written as a trailing backslash: the backslash is dropped
//     and the break kept. This is what makes a URL ending in "/" gain a stray "\".
//   - a "loose" list, with a blank line between items: the blank line goes, so
//     Discord doesn't gap every bullet. The blank lines around the list and between
//     paragraphs stay.
//   - a timestamp token whose angle brackets are backslash-escaped, which stops
//     Discord parsing it: the escapes go.
//   - an emoji shortcode whose underscores are backslash-escaped (":video\_game:"),
//     which stops Discord resolving the name: the escapes go, inside the token only.
func Markdown(s string) string {
	if s == "" {
		return s
	}
	// The editor stores CRLF; every rule below reasons in "\n", which Discord
	// renders fine.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = brBreakRe.ReplaceAllString(s, "\n")
	s = backslashBreakRe.ReplaceAllString(s, "$1")
	s = strings.TrimRight(s, "\\")
	// Never more than a paragraph break, whatever stacked breaks came before.
	s = blankRe.ReplaceAllString(s, "\n\n")
	s = tightenLists(s)
	s = timestampEscapeRe.ReplaceAllString(s, "<t:$1$2>")
	// Scoped to the token on purpose: an escaped underscore in prose ("snake\_case")
	// is correct, Discord renders it without italicizing.
	s = emojiShortcodeEscapeRe.ReplaceAllStringFunc(s, func(tok string) string {
		return strings.ReplaceAll(tok, `\_`, "_")
	})
	return s
}

// tightenLists drops the blank line the serializer puts between the items of a
// loose list. A blank line goes only when the text before it is inside a list (an
// item marker line or its indented continuation) and the next non-blank line is
// another item marker. Reading line by line lets it handle items that span lines.
func tightenLists(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	inList := false
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			next := ""
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) != "" {
					next = lines[j]
					break
				}
			}
			if inList && listItemRe.MatchString(next) {
				continue
			}
			inList = false
			out = append(out, line)
			continue
		}
		switch {
		case listItemRe.MatchString(line):
			inList = true
		case inList && (line[0] == ' ' || line[0] == '\t'):
			// An indented continuation of the current item stays in the list.
		default:
			inList = false
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
