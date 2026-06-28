package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokensToCSS(t *testing.T) {
	// Emitted in canonical :root order regardless of input order, and only known
	// tokens are included.
	css := TokensToCSS(map[string]string{
		"accent":  "#fff",
		"page-bg": "#000",
		"bogus":   "drop-me",
	})
	if css != ":root{--page-bg:#000;--accent:#fff;}" {
		t.Errorf("css = %q", css)
	}

	if TokensToCSS(nil) != "" {
		t.Error("nil tokens should generate no CSS")
	}
	if TokensToCSS(map[string]string{"unknown": "x"}) != "" {
		t.Error("only-unknown tokens should generate no CSS")
	}
}

func TestTokensToCSS_StripsInjection(t *testing.T) {
	css := TokensToCSS(map[string]string{"accent": "red; } body{ display:none } /*"})
	// The value can't break out of its declaration: no extra braces or semicolons
	// survive inside the value, so a single --accent declaration remains.
	if strings.Contains(css, "body{") || strings.Contains(css, "display:none") && strings.Contains(css, "}") && strings.Count(css, "}") > 1 {
		t.Errorf("injection not neutralized: %q", css)
	}
	if strings.Count(css, "{") != 1 || strings.Count(css, "}") != 1 {
		t.Errorf("expected exactly one rule block, got %q", css)
	}
}

func TestSanitizeTokens(t *testing.T) {
	out := SanitizeTokens(map[string]string{
		"--page-bg": "  #abc  ", // leading "--" tolerated; trimmed
		"accent":    "red",
		"unknown":   "nope", // dropped (not in allowlist)
		"shadow":    "   ",  // dropped (empty after trim)
	})
	if out["page-bg"] != "#abc" {
		t.Errorf("page-bg = %q; want #abc", out["page-bg"])
	}
	if out["accent"] != "red" {
		t.Errorf("accent = %q", out["accent"])
	}
	if _, ok := out["unknown"]; ok {
		t.Error("unknown token should be dropped")
	}
	if _, ok := out["shadow"]; ok {
		t.Error("empty-value token should be dropped")
	}
}

func TestParseRootTokens(t *testing.T) {
	css := `/* a legacy theme */
:root {
    --page-bg: #1a1c17; /* bg */
    --accent: #d6bdae;
    --not-a-real-token: #fff;
}
.some-class { color: red; }
:root { --ignored-second-block: #000; }`
	tokens := parseRootTokens(css)
	if tokens["page-bg"] != "#1a1c17" {
		t.Errorf("page-bg = %q", tokens["page-bg"])
	}
	if tokens["accent"] != "#d6bdae" {
		t.Errorf("accent = %q", tokens["accent"])
	}
	if _, ok := tokens["not-a-real-token"]; ok {
		t.Error("unknown token should be ignored")
	}
	// Only the first :root block is parsed.
	if _, ok := tokens["ignored-second-block"]; ok {
		t.Error("second :root block should not be parsed")
	}
}

// TestMigrateStyleTokens_Backfill builds a legacy v36 styles table (css_content,
// no tokens column), then runs the migrations and verifies each theme's :root
// tokens were parsed into the new tokens column and the legacy column was dropped.
func TestMigrateStyleTokens_Backfill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	setup := []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE styles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			css_content TEXT NOT NULL DEFAULT '',
			board_flourish TEXT NOT NULL DEFAULT '',
			number_flourish TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, stmt := range setup {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup legacy schema: %v", err)
		}
	}
	if _, err := db.Exec(
		`INSERT INTO styles (name, css_content) VALUES (?, ?)`,
		"Obon", ":root{ --page-bg: #1a0a0b; --accent: #fbc95d; }",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version = 36"); err != nil {
		t.Fatal(err)
	}

	if err := ensureSchema(db); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}

	if !hasColumn(db, "styles", "tokens") {
		t.Fatal("styles.tokens column should exist after migration")
	}
	if hasColumn(db, "styles", "css_content") {
		t.Error("legacy styles.css_content column should have been dropped")
	}

	var raw string
	if err := db.QueryRow(`SELECT tokens FROM styles WHERE name = 'Obon'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &tokens); err != nil {
		t.Fatalf("tokens not valid JSON: %v (%q)", err, raw)
	}
	if tokens["page-bg"] != "#1a0a0b" || tokens["accent"] != "#fbc95d" {
		t.Errorf("backfilled tokens = %v", tokens)
	}
}
