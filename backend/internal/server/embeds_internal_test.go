package server

import (
	"strings"
	"testing"
)

// TestTruncateRunesRespectsTheLimit pins the arithmetic. The ellipsis counts
// toward the cap, and keeping n-1 runes before appending three more returned n+2 -
// so every truncated field came back OVER the Discord limit it was truncated to
// satisfy, and Discord rejects the whole payload with a 400. That only showed up
// on long content, which is exactly when truncation runs.
func TestTruncateRunesRespectsTheLimit(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"under the limit is untouched", "hello", 10, "hello"},
		{"exactly at the limit is untouched", "hello", 5, "hello"},
		{"truncates to exactly n runes", "abcdefghij", 8, "abcde..."},
		{"no room for an ellipsis", "abcdefghij", 3, "abc"},
		{"zero", "abcdefghij", 0, ""},
		// Multi-byte: the cap is in RUNES, and slicing by byte would split these.
		{"japanese truncates by rune", "あいうえおかきくけこ", 6, "あいう..."},
	}
	for _, c := range cases {
		got := truncateRunes(c.in, c.n)
		if got != c.want {
			t.Errorf("%s: truncateRunes(%q, %d) = %q; want %q", c.name, c.in, c.n, got, c.want)
		}
		if n := len([]rune(got)); n > c.n {
			t.Errorf("%s: result is %d runes, over the limit of %d", c.name, n, c.n)
		}
	}
}

// TestEmbedRespectsDiscordTotalBudget pins the 6000-character cap. Discord sums
// title, description, every field name and value, and the footer, and rejects the
// WHOLE payload with a 400 when that exceeds 6000 - so a long enough announcement
// simply failed to post, with the failure surfacing as a webhook error rather than
// anything about length. The 25-field cap was enforced but dropped silently, which
// is worse than failing: the announcement arrived looking complete.
func TestEmbedRespectsDiscordTotalBudget(t *testing.T) {
	b := newEmbed().title("A long announcement")
	// Each field is at the per-field cap, so ~6 of them reach the total budget.
	big := strings.Repeat("x", embedFieldValueMax)
	for range 20 {
		b.field(embedNoHeading, big, false)
	}
	e := b.build()

	total := len([]rune(e.Title)) + len([]rune(e.Description))
	for _, f := range e.Fields {
		total += len([]rune(f.Name)) + len([]rune(f.Value))
	}
	if total > embedTotalMax {
		t.Errorf("embed totals %d characters; Discord rejects anything over %d", total, embedTotalMax)
	}
	if len(e.Fields) > maxEmbedFields {
		t.Errorf("embed carries %d fields; Discord caps them at %d", len(e.Fields), maxEmbedFields)
	}
	// And it must SAY it was clipped rather than quietly arriving short.
	last := e.Fields[len(e.Fields)-1]
	if last.Value != embedTruncatedNotice {
		t.Errorf("last field = %q; want the truncation notice so the clip is visible", last.Value)
	}
}

// TestEmbedWithinBudgetIsUntouched guards the other direction: an ordinary
// announcement must not gain a truncation notice it did not earn.
func TestEmbedWithinBudgetIsUntouched(t *testing.T) {
	e := newEmbed().title("Tea time").field("When", "Friday", false).build()
	if len(e.Fields) != 1 {
		t.Fatalf("fields = %d; want 1", len(e.Fields))
	}
	if e.Fields[0].Value != "Friday" {
		t.Errorf("field value = %q; want it left alone", e.Fields[0].Value)
	}
}
