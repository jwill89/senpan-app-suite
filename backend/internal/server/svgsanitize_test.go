package server

import (
	"strings"
	"testing"
)

func TestSanitizeSVG_StripsScriptVectors(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		mustDrop   []string // substrings that must NOT survive
		mustKeep   []string // substrings that must survive
		wantReject bool
	}{
		{
			name:     "script element",
			in:       `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script><path d="M0 0"/></svg>`,
			mustDrop: []string{"script", "alert(1)"},
			mustKeep: []string{"<svg", "path", "M0 0"},
		},
		{
			name:     "event handler attribute",
			in:       `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><circle r="5" onclick="steal()"/></svg>`,
			mustDrop: []string{"onload", "onclick", "alert(1)", "steal()"},
			mustKeep: []string{"<svg", "circle", `r="5"`},
		},
		{
			name:     "javascript href on anchor",
			in:       `<svg xmlns="http://www.w3.org/2000/svg"><a href="javascript:alert(1)"><rect/></a></svg>`,
			mustDrop: []string{"javascript:"},
			mustKeep: []string{"<svg", "rect"},
		},
		{
			// Well-formed foreignObject subtree must be dropped entirely. (A
			// *malformed*-HTML foreignObject makes the whole parse fail, which is
			// also safe — the upload is rejected — but here we prove the strip.)
			name:     "foreignObject html",
			in:       `<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><img src="x" onerror="alert(1)"/></foreignObject><path d="Z"/></svg>`,
			mustDrop: []string{"foreignObject", "onerror", "alert(1)"},
			mustKeep: []string{"<svg", "path"},
		},
		{
			name:     "animate setting href to javascript",
			in:       `<svg xmlns="http://www.w3.org/2000/svg"><a><set attributeName="href" to="javascript:alert(1)"/><rect/></a></svg>`,
			mustDrop: []string{"javascript:", "attributeName"},
			mustKeep: []string{"<svg", "rect"},
		},
		{
			name:     "dangerous style",
			in:       `<svg xmlns="http://www.w3.org/2000/svg"><rect style="background:url(javascript:alert(1))"/></svg>`,
			mustDrop: []string{"javascript:"},
			mustKeep: []string{"<svg", "rect"},
		},
		{
			name:       "not an svg",
			in:         `<html><body>hi</body></html>`,
			wantReject: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, ok := sanitizeSVG([]byte(tc.in))
			if tc.wantReject {
				if ok {
					t.Fatalf("expected rejection, got accepted: %s", out)
				}
				return
			}
			if !ok {
				t.Fatalf("expected acceptance, got rejected")
			}
			got := string(out)
			low := strings.ToLower(got)
			for _, bad := range tc.mustDrop {
				if strings.Contains(low, strings.ToLower(bad)) {
					t.Errorf("sanitized output still contains %q:\n%s", bad, got)
				}
			}
			for _, good := range tc.mustKeep {
				if !strings.Contains(got, good) {
					t.Errorf("sanitized output missing expected %q:\n%s", good, got)
				}
			}
		})
	}
}

// TestSanitizeSVG_PreservesLegitArtwork ensures a normal path/gradient SVG with an
// xlink:href fragment reference survives sanitization intact (renders correctly).
func TestSanitizeSVG_PreservesLegitArtwork(t *testing.T) {
	in := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 10 10">` +
		`<defs><linearGradient id="g"><stop offset="0" stop-color="#fff"/></linearGradient></defs>` +
		`<rect fill="url(#g)" width="10" height="10"/>` +
		`<use xlink:href="#g"/>` +
		`<path d="M1 2 L3 4"/></svg>`
	out, ok := sanitizeSVG([]byte(in))
	if !ok {
		t.Fatal("legit SVG was rejected")
	}
	got := string(out)
	for _, want := range []string{"<svg", "linearGradient", `id="g"`, "url(#g)", `xlink:href="#g"`, "M1 2 L3 4"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected sanitized output to keep %q:\n%s", want, got)
		}
	}
}
