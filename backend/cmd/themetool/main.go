// Command themetool inspects and adjusts the saved themes in a Bingo-App SQLite
// database for WCAG 2.1 contrast. It reuses the app's own store package so it
// reads/writes tokens exactly as the server does.
//
//	go run ./cmd/themetool dump   <db>
//	go run ./cmd/themetool check  <db>
//	go run ./cmd/themetool apply  <db> <changes.json>
//
// changes.json: {"update":{"<id>":{"<token>":"<value>",...}},
//
//	"create":[{"name":"Toji","tokens":{...},"board_flourish":"","number_flourish":""}]}
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"app-suite/internal/store"
)

// defaultTokens mirrors the built-in theme (frontend/src/assets/styles/tokens.css).
// Used to check the default and to fill any token a saved theme omits.
var defaultTokens = map[string]string{
	"page-bg": "#1a1c17", "panel-bg": "#272a22", "panel-raised-bg": "#2f3228",
	"control-border": "#4a4d3f", "input-bg": "#272a22",
	"accent": "#d6bdae", "accent-hover": "#c4a999", "accent-2": "#474b3c",
	"accent-2-hover": "#3a3d30", "highlight": "#d6bdae",
	"text": "#f0ebe3", "text-muted": "#d3d3bf", "text-on-accent": "#1a1c17", "text-on-fill": "#f5f1ea",
	"success": "#175020", "danger": "#9a2018", "warning": "#e0a82e",
	"board-cell-bg": "#f0ebe3", "board-cell-hover-bg": "#e5ded4", "board-free-bg": "#d6bdae",
	"board-gradient-start": "#2f3328", "board-gradient-end": "#272a22",
	"modal-overlay": "rgb(0 0 0 / 70%)", "shadow": "rgb(0 0 0 / 50%)", "highlight-glow": "rgb(214 189 174 / 50%)",
}

// pair is a foreground/background token pairing that renders real text. large is
// true for display text (board numbers, called number) where WCAG's large-text
// thresholds apply (AA 3:1, AAA 4.5:1 vs 4.5/7 for normal text).
type pair struct {
	fg, bg, label string
	large         bool
}

// CautionText is the fixed dark ink the caution button paints on the (always
// light) --warning fill, decoupled from --text-on-accent (which goes light in
// light themes). Keep in sync with .btn-caution in base.css.
const CautionText = "#1f1a06"

// A pair's fg may be a token name OR a literal "#hex" (used for the fixed
// caution-button ink). bg is always a token name.
var pairs = []pair{
	{"text", "page-bg", "body / page", false},
	{"text", "panel-bg", "body / panel", false},
	{"text", "panel-raised-bg", "body / raised", false},
	{"text", "input-bg", "input text / field", false},
	{"text", "control-border", "neutral btn / fill", false},
	{"text-muted", "page-bg", "muted / page", false},
	{"text-muted", "panel-bg", "muted / panel", false},
	{"text-muted", "panel-raised-bg", "muted / raised", false},
	{"text-muted", "input-bg", "placeholder / field", false},
	{"accent", "panel-bg", "link / panel", false},
	{"accent", "page-bg", "link / page", false},
	{"highlight", "panel-bg", "heading / panel", false},
	{"highlight", "page-bg", "heading / page", false},
	{"highlight", "panel-raised-bg", "heading / raised", false},
	// B-I-N-G-O header letters (--highlight) over the board-wrap gradient; check
	// both gradient stops. Large display type, so large-text thresholds apply.
	{"highlight", "board-gradient-start", "BINGO hdr / board top", true},
	{"highlight", "board-gradient-end", "BINGO hdr / board bot", true},
	{"text-on-accent", "accent", "action btn / accent", false},
	{"text-on-accent", "accent-hover", "action btn / accent-hov", false},
	{"text-on-accent", "highlight", "called# / highlight", true},
	{"text-on-accent", "text-muted", "winner chip end", false},
	{"text-on-accent", "board-cell-bg", "board# / cell", true},
	{"text-on-accent", "board-cell-hover-bg", "board# / cell-hover", true},
	{"text-on-accent", "board-free-bg", "board# / FREE", true},
	{"text-on-fill", "accent-2", "view btn / accent-2", false},
	{"text-on-fill", "accent-2-hover", "view btn / accent-2-hov", false},
	{"text-on-fill", "success", "confirm btn / success", false},
	{"text-on-fill", "danger", "danger btn / danger", false},
	{CautionText, "warning", "caution btn / warning", false},
}

func aaTarget(large bool) float64 {
	if large {
		return 3.0
	}
	return 4.5
}
func aaaTarget(large bool) float64 {
	if large {
		return 4.5
	}
	return 7.0
}

var rgbFuncRe = regexp.MustCompile(`rgba?\(([^)]+)\)`)

// parseColor converts a hex or rgb()/rgba() string to 0-255 channels (alpha
// ignored — none of the checked pairs use alpha tokens).
func parseColor(s string) (r, g, b float64, ok bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "#") {
		h := s[1:]
		if len(h) == 3 || len(h) == 4 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if len(h) >= 6 {
			ri, e1 := strconv.ParseInt(h[0:2], 16, 0)
			gi, e2 := strconv.ParseInt(h[2:4], 16, 0)
			bi, e3 := strconv.ParseInt(h[4:6], 16, 0)
			if e1 == nil && e2 == nil && e3 == nil {
				return float64(ri), float64(gi), float64(bi), true
			}
		}
		return 0, 0, 0, false
	}
	if m := rgbFuncRe.FindStringSubmatch(s); m != nil {
		body := strings.NewReplacer("/", " ", ",", " ").Replace(m[1])
		f := strings.Fields(body)
		if len(f) >= 3 {
			ch := func(x string) float64 {
				if strings.HasSuffix(x, "%") {
					v, _ := strconv.ParseFloat(strings.TrimSuffix(x, "%"), 64)
					return v / 100 * 255
				}
				v, _ := strconv.ParseFloat(x, 64)
				return v
			}
			return ch(f[0]), ch(f[1]), ch(f[2]), true
		}
	}
	return 0, 0, 0, false
}

func relLum(r, g, b float64) float64 {
	lin := func(c float64) float64 {
		c /= 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func contrast(fg, bg string) (float64, bool) {
	fr, fg2, fb, ok1 := parseColor(fg)
	br, bg2, bb, ok2 := parseColor(bg)
	if !ok1 || !ok2 {
		return 0, false
	}
	l1, l2 := relLum(fr, fg2, fb), relLum(br, bg2, bb)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05), true
}

// merged returns a theme's tokens filled in over the defaults.
func merged(tok map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range defaultTokens {
		out[k] = v
	}
	for k, v := range tok {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

func checkTheme(name string, tok map[string]string) (aaFail, aaaFail int) {
	m := merged(tok)
	fmt.Printf("\n=== %s ===\n", name)
	for _, p := range pairs {
		fgVal := m[p.fg]
		if strings.HasPrefix(p.fg, "#") {
			fgVal = p.fg // literal colour (e.g. the fixed caution-button ink)
		}
		ratio, ok := contrast(fgVal, m[p.bg])
		if !ok {
			fmt.Printf("  %-24s  ??? (unparsable: %s / %s)\n", p.label, fgVal, m[p.bg])
			continue
		}
		aa, aaa := aaTarget(p.large), aaaTarget(p.large)
		mark := func(pass bool) string {
			if pass {
				return "OK "
			}
			return "XX "
		}
		aaOK, aaaOK := ratio >= aa, ratio >= aaa
		if !aaOK {
			aaFail++
		}
		if !aaaOK {
			aaaFail++
		}
		tag := ""
		if p.large {
			tag = " (lg)"
		}
		fmt.Printf("  %-24s %5.2f:1  AA %s AAA %s%s\n", p.label, ratio, mark(aaOK), mark(aaaOK), tag)
	}
	fmt.Printf("  -> AA fails: %d   AAA fails: %d\n", aaFail, aaaFail)
	return
}

func openDB(path string) *store.Store {
	st, err := store.New(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	return st
}

func loadThemes(st *store.Store) []map[string]string {
	list, err := st.ListStyles()
	if err != nil {
		fmt.Fprintln(os.Stderr, "list:", err)
		os.Exit(1)
	}
	var out []map[string]string
	for _, s := range list {
		full, _ := st.GetStyle(s.ID)
		m := map[string]string{"__id": strconv.FormatInt(s.ID, 10), "__name": s.Name}
		for k, v := range full.Tokens {
			m[k] = v
		}
		out = append(out, m)
	}
	return out
}

func tokensOnly(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		if !strings.HasPrefix(k, "__") {
			out[k] = v
		}
	}
	return out
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: themetool <dump|check|apply> <db> [changes.json]")
		os.Exit(2)
	}
	cmd, dbPath := os.Args[1], os.Args[2]
	st := openDB(dbPath)
	defer st.Close()

	switch cmd {
	case "dump":
		for _, m := range loadThemes(st) {
			b, _ := json.Marshal(tokensOnly(m))
			fmt.Printf("[%s] %s\n%s\n\n", m["__id"], m["__name"], b)
		}

	case "check":
		totalAA, totalAAA := 0, 0
		aa, aaa := checkTheme("[default] (built-in)", defaultTokens)
		totalAA, totalAAA = totalAA+aa, totalAAA+aaa
		themes := loadThemes(st)
		sort.Slice(themes, func(i, j int) bool { return themes[i]["__name"] < themes[j]["__name"] })
		for _, m := range themes {
			aa, aaa := checkTheme(fmt.Sprintf("[%s] %s", m["__id"], m["__name"]), tokensOnly(m))
			totalAA, totalAAA = totalAA+aa, totalAAA+aaa
		}
		fmt.Printf("\nTOTAL across all themes — AA fails: %d   AAA fails: %d\n", totalAA, totalAAA)

	case "sqldump":
		// Emit targeted SQL to bring another DB's `styles` table to this DB's
		// token state: UPDATE every theme by id (ids match because the target was
		// seeded from this DB). Touches only `styles` — safe on the live prod DB.
		esc := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
		fmt.Println("BEGIN IMMEDIATE;")
		for _, m := range loadThemes(st) {
			b, _ := json.Marshal(tokensOnly(m))
			fmt.Printf("UPDATE styles SET tokens='%s' WHERE id=%s;\n", esc(string(b)), m["__id"])
		}
		fmt.Println("COMMIT;")

	case "apply":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "apply needs a changes.json path")
			os.Exit(2)
		}
		raw, err := os.ReadFile(os.Args[3])
		if err != nil {
			fmt.Fprintln(os.Stderr, "read changes:", err)
			os.Exit(1)
		}
		var changes struct {
			Update map[string]map[string]string `json:"update"`
			Create []struct {
				Name           string            `json:"name"`
				Tokens         map[string]string `json:"tokens"`
				BoardFlourish  string            `json:"board_flourish"`
				NumberFlourish string            `json:"number_flourish"`
			} `json:"create"`
		}
		if err := json.Unmarshal(raw, &changes); err != nil {
			fmt.Fprintln(os.Stderr, "parse changes:", err)
			os.Exit(1)
		}
		for idStr, over := range changes.Update {
			id, _ := strconv.ParseInt(idStr, 10, 64)
			cur, err := st.GetStyle(id)
			if err != nil || cur == nil {
				fmt.Printf("update %s: not found\n", idStr)
				continue
			}
			tok := map[string]string{}
			for k, v := range cur.Tokens {
				tok[k] = v
			}
			for k, v := range over {
				tok[k] = v
			}
			if err := st.UpdateStyle(id, cur.Name, tok, cur.BoardFlourish, cur.NumberFlourish); err != nil {
				fmt.Printf("update %s: %v\n", idStr, err)
				continue
			}
			fmt.Printf("updated [%d] %s (%d token overrides)\n", id, cur.Name, len(over))
		}
		for _, c := range changes.Create {
			tok := merged(c.Tokens) // fill any omitted token from defaults
			id, err := st.CreateStyle(c.Name, tok, c.BoardFlourish, c.NumberFlourish)
			if err != nil {
				fmt.Printf("create %s: %v\n", c.Name, err)
				continue
			}
			fmt.Printf("created [%d] %s\n", id, c.Name)
		}

	default:
		fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		os.Exit(2)
	}
}
