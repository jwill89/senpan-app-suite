package discord

import "testing"

func TestTimeStyle(t *testing.T) {
	cases := []struct{ in, def, want string }{
		{"", "F", "F"},     // empty -> given default
		{"", "t", "t"},     // empty -> given default
		{"f", "F", "f"},    // valid honored
		{"R", "t", "R"},    // valid honored
		{" t ", "F", "t"},  // trimmed
		{"bad", "F", "F"},  // unknown -> default
		{"long", "t", "t"}, // unknown -> default
	}
	for _, c := range cases {
		if got := TimeStyle(c.in, c.def); got != c.want {
			t.Errorf("TimeStyle(%q, %q) = %q, want %q", c.in, c.def, got, c.want)
		}
	}
}

// TestMarkdown verifies the three Milkdown->Discord normalizations:
// backslash hard breaks, <br> tags, and escaped timestamp tokens.
func TestMarkdown(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"plain", "Just text.", "Just text."},
		{"backslash break after slash url", "See https://example.com/\\\nNext line", "See https://example.com/\nNext line"},
		{"trailing backslash at end", "ends here\\", "ends here"},
		{"br tag", "line one<br />line two", "line one\nline two"},
		{"br variants", "a<br>b<br/>c<BR />d", "a\nb\nc\nd"},
		// A <br> hard break the serializer follows with a source newline must
		// collapse to ONE line break, not a blank line between every line.
		{"br hard break + source newline", "line one<br />\nline two", "line one\nline two"},
		{"br with crlf", "a<br/>\r\nb", "a\nb"},
		{"real paragraph preserved", "para one\n\npara two", "para one\n\npara two"},
		{"br before blank stays a paragraph", "a<br />\n\nb", "a\n\nb"},
		{"escaped timestamp", "Starts \\<t:1782009000:t> sharp", "Starts <t:1782009000:t> sharp"},
		{"escaped both brackets", "\\<t:1782009000:F\\>", "<t:1782009000:F>"},
		{"unescaped timestamp untouched", "<t:1782009000:R>", "<t:1782009000:R>"},
		{"timestamp no style", "<t:1782009000>", "<t:1782009000>"},
		// CRLF (the editor stores it) is normalized to LF.
		{"crlf normalized", "a\r\nb", "a\nb"},
		// A <br> used as a standalone spacer paragraph (blank lines on both sides,
		// CRLF) collapses to a single blank line - not a stack of them.
		{"br spacer paragraph crlf", "a\r\n\r\n<br />\r\n\r\nb", "a\n\nb"},
		// Loose list (blank line between every item) -> tight list.
		{"loose bullet list", "* one\r\n\r\n* two\r\n\r\n* three", "* one\n* two\n* three"},
		{"loose ordered list", "1. a\r\n\r\n2. b", "1. a\n2. b"},
		// A loose list whose items span multiple (indented) lines still tightens.
		{"loose list multiline items", "* one\r\n  cont\r\n\r\n* two", "* one\n  cont\n* two"},
		// The blank line before a list and before a following paragraph is kept.
		{"blank around list kept", "intro\r\n\r\n- a\r\n\r\n- b\r\n\r\nouttro", "intro\n\n- a\n- b\n\nouttro"},
		// A bullet (*) item followed by a dash (-) list joins into one tight list.
		{"mixed markers tighten", "* a\r\n\r\n- b\r\n- c", "* a\n- b\n- c"},
		// Emoji shortcodes: the serializer escapes the underscores in a multi-word
		// name, which stops Discord resolving it - so ":tada:" rendered and
		// ":video_game:" posted as literal text. Unescape inside the token only.
		// These inputs are verbatim mdast-util-to-markdown output (the serializer
		// Milkdown uses): it escapes the underscore and leaves the colons alone.
		{"escaped shortcode mid-sentence", "Play :video\\_game: now", "Play :video_game: now"},
		{"escaped shortcode at start", ":video\\_game: at start", ":video_game: at start"},
		{"escaped shortcode at end", "end with :video\\_game:", "end with :video_game:"},
		{"escaped three-word shortcode", ":white\\_check\\_mark:", ":white_check_mark:"},
		{"single word shortcode untouched", ":tada:", ":tada:"},
		{"unescaped shortcode untouched", ":video_game:", ":video_game:"},
		{"plus shortcode untouched", ":+1:", ":+1:"},
		{"hyphen and underscore name", ":non-potable\\_water:", ":non-potable_water:"},
		{"two shortcodes on a line", ":video\\_game: and :white\\_check\\_mark:", ":video_game: and :white_check_mark:"},
		// Prose keeps its escape: Discord honors "\_" and renders the underscore
		// without italicizing, so unescaping globally would be the wrong fix.
		{"prose underscore stays escaped", "a snake\\_case name", "a snake\\_case name"},
		{"colon prose not a shortcode", "note: some\\_thing: here", "note: some\\_thing: here"},
		// A realistic announcement: emoji and a timestamp in one line, both escaped
		// by the serializer, both repaired without interfering with each other.
		{
			"shortcodes and timestamp together",
			"Bingo night :video\\_game: - starts \\<t:1782009000:t\\>",
			"Bingo night :video_game: - starts <t:1782009000:t>",
		},
	}
	for _, tc := range cases {
		if got := Markdown(tc.in); got != tc.want {
			t.Errorf("%s: Markdown(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestColorFromHex(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"#1abc9c", 0x1abc9c},
		{"1abc9c", 0x1abc9c},
		{"  #FFFFFF  ", 0xFFFFFF},
		{"", AccentColor},            // empty -> default
		{"not-a-color", AccentColor}, // invalid hex -> default
		{"#ffffffff", AccentColor},   // out of 24-bit range -> default
	}
	for _, c := range cases {
		if got := ColorFromHex(c.in, AccentColor); got != c.want {
			t.Errorf("ColorFromHex(%q): got %#x, want %#x", c.in, got, c.want)
		}
	}

}

func TestParseEmoji(t *testing.T) {
	if e := ParseEmoji("  "); e != nil {
		t.Errorf("blank -> nil, got %+v", e)
	}
	if e := ParseEmoji("🎉"); e == nil || e.Name != "🎉" || e.ID != "" {
		t.Errorf("unicode emoji: %+v", e)
	}
	if e := ParseEmoji("<:wave:123>"); e == nil || e.Name != "wave" || e.ID != "123" || e.Animated {
		t.Errorf("custom emoji: %+v", e)
	}
	if e := ParseEmoji("<a:spin:456>"); e == nil || e.Name != "spin" || e.ID != "456" || !e.Animated {
		t.Errorf("animated custom emoji: %+v", e)
	}
}
