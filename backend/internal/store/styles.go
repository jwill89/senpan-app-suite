package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"app-suite/internal/model"
)

// ── Style operations ────────────────────────────────────────────────────────
//
// A theme ("style") is a set of design-token overrides — a map of token name to
// CSS value — not free-form CSS. The applied stylesheet is generated from those
// tokens as a ":root{…}" block (see TokensToCSS), so themes can only retint the
// app's design tokens and never carry arbitrary CSS. This keeps themes safe (an
// admin can't break the layout for every player) and lets the base stylesheet's
// class names be refactored freely.

// themeTokenOrder is the canonical list of theme-overridable design tokens, in
// the order they appear in :root (see frontend app.css). Token names are stored
// without the leading "--". This list is the single allowlist: only these names
// are persisted or emitted, and CSS is generated in this order. The non-colour
// tokens --radius and --header-font are intentionally excluded (radius is a
// structural default; the header font is managed via app settings).
var themeTokenOrder = []string{
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

// themeTokenSet is the allowlist built from themeTokenOrder for O(1) membership.
var themeTokenSet = func() map[string]bool {
	m := make(map[string]bool, len(themeTokenOrder))
	for _, n := range themeTokenOrder {
		m[n] = true
	}
	return m
}()

// tokenValueStripper removes characters that could let a token value break out
// of its `--name: value;` declaration (CSS injection), keeping a theme value to
// a single, inert declaration value.
var tokenValueStripper = strings.NewReplacer(
	";", "", "{", "", "}", "", "/*", "", "*/", "", "\n", "", "\r", "", "\t", "",
)

// sanitizeTokenValue trims a token value and neutralizes any CSS-injection
// attempt, capping its length. Returns "" for an empty/unusable value.
func sanitizeTokenValue(v string) string {
	v = tokenValueStripper.Replace(strings.TrimSpace(v))
	v = strings.TrimSpace(v)
	if len(v) > 64 {
		v = strings.TrimSpace(v[:64])
	}
	return v
}

// SanitizeTokens returns a clean token map: only known token names are kept
// (leading "--" tolerated on input), each value injection-stripped, and empty
// values dropped. The result is safe to persist and to render into CSS.
func SanitizeTokens(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		name := strings.TrimPrefix(strings.TrimSpace(k), "--")
		if !themeTokenSet[name] {
			continue
		}
		if sv := sanitizeTokenValue(v); sv != "" {
			out[name] = sv
		}
	}
	return out
}

// TokensToCSS renders a token map as a ":root{…}" stylesheet, emitting only
// known tokens in canonical order. Returns "" when no known tokens are present
// (so an empty/again-default theme injects nothing).
func TokensToCSS(tokens map[string]string) string {
	if len(tokens) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(":root{")
	wrote := false
	for _, name := range themeTokenOrder {
		v := sanitizeTokenValue(tokens[name])
		if v == "" {
			continue
		}
		b.WriteString("--")
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(v)
		b.WriteByte(';')
		wrote = true
	}
	b.WriteString("}")
	if !wrote {
		return ""
	}
	return b.String()
}

// rootBlockRe matches the first ":root { … }" block; declRe matches each
// "--name: value;" declaration within it. Used to migrate legacy css_content.
var (
	rootBlockRe = regexp.MustCompile(`(?s):root\s*\{(.*?)\}`)
	declRe      = regexp.MustCompile(`--([a-zA-Z0-9-]+)\s*:\s*([^;]+);`)
)

// parseRootTokens extracts known design tokens from the first :root{…} block of
// a CSS string (the legacy theme format). Unknown declarations are ignored. Used
// by the css_content → tokens migration backfill.
func parseRootTokens(css string) map[string]string {
	out := make(map[string]string)
	m := rootBlockRe.FindStringSubmatch(css)
	if m == nil {
		return out
	}
	for _, d := range declRe.FindAllStringSubmatch(m[1], -1) {
		name := d[1]
		if !themeTokenSet[name] {
			continue
		}
		if v := sanitizeTokenValue(d[2]); v != "" {
			out[name] = v
		}
	}
	return out
}

// marshalTokens serializes a sanitized token map to its JSON column value.
func marshalTokens(tokens map[string]string) (string, error) {
	buf, err := json.Marshal(SanitizeTokens(tokens))
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// scanTokens decodes a tokens JSON column value into a map (nil-safe).
func scanTokens(raw string) map[string]string {
	tokens := make(map[string]string)
	if raw == "" {
		return tokens
	}
	_ = json.Unmarshal([]byte(raw), &tokens)
	return tokens
}

// ListStyles returns all styles (name only — no tokens) for lightweight listing.
func (s *Store) ListStyles() ([]model.Style, error) {
	rows, err := s.db.Query("SELECT id, name, created_at FROM styles ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	styles := make([]model.Style, 0)
	for rows.Next() {
		var st model.Style
		if err := rows.Scan(&st.ID, &st.Name, &st.CreatedAt); err != nil {
			return nil, err
		}
		styles = append(styles, st)
	}
	return styles, rows.Err()
}

// GetStyle retrieves a single style by ID with its tokens (and the generated
// CSS + flourishes). Returns nil if not found.
func (s *Store) GetStyle(id int64) (*model.Style, error) {
	return s.scanStyle(s.db.QueryRow(
		"SELECT id, name, tokens, board_flourish, number_flourish, created_at FROM styles WHERE id = ?", id,
	))
}

// scanStyle scans a style row (id, name, tokens, board, number, created_at),
// decoding the tokens and filling in the generated CSSContent. Returns nil on
// sql.ErrNoRows.
func (s *Store) scanStyle(row *sql.Row) (*model.Style, error) {
	var st model.Style
	var rawTokens string
	err := row.Scan(&st.ID, &st.Name, &rawTokens, &st.BoardFlourish, &st.NumberFlourish, &st.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	st.Tokens = scanTokens(rawTokens)
	st.CSSContent = TokensToCSS(st.Tokens)
	return &st, nil
}

// ErrInvalidFlourish is returned by CreateStyle/UpdateStyle when a flourish path
// is neither empty nor a safe images/<dir>/<file>.svg reference.
var ErrInvalidFlourish = errors.New("invalid flourish path")

var flourishPathRe = regexp.MustCompile(`^images/[a-z0-9_]+/[^/\\]+(?i:\.svg)$`)

// ValidFlourishPath reports whether p is an acceptable theme flourish reference:
// empty (unset) or a relative images/<category>/<file>.svg path. Flourishes are
// otherwise stored verbatim and fetched + v-html'd on the public board
// (CornerFlourish.vue), so this blocks data:-URI and external-URL SVGs that never
// went through the upload-time SVG sanitizer. The directory segment matches a
// category slug; the filename stays permissive ([^/\\]+) so uppercase/space/
// Unicode uploaded names pass. Case-insensitive on the .svg extension.
func ValidFlourishPath(p string) bool {
	if p == "" {
		return true
	}
	return flourishPathRe.MatchString(p)
}

// CreateStyle inserts a new theme from a token map (sanitized) and returns its ID.
func (s *Store) CreateStyle(name string, tokens map[string]string, boardFlourish, numberFlourish string) (int64, error) {
	if !ValidFlourishPath(boardFlourish) || !ValidFlourishPath(numberFlourish) {
		return 0, ErrInvalidFlourish
	}
	tokenJSON, err := marshalTokens(tokens)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(
		"INSERT INTO styles (name, tokens, board_flourish, number_flourish) VALUES (?, ?, ?, ?)",
		name, tokenJSON, boardFlourish, numberFlourish,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateStyle updates a theme's name, tokens (sanitized), and flourishes.
func (s *Store) UpdateStyle(id int64, name string, tokens map[string]string, boardFlourish, numberFlourish string) error {
	if !ValidFlourishPath(boardFlourish) || !ValidFlourishPath(numberFlourish) {
		return ErrInvalidFlourish
	}
	tokenJSON, err := marshalTokens(tokens)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		"UPDATE styles SET name = ?, tokens = ?, board_flourish = ?, number_flourish = ? WHERE id = ?",
		name, tokenJSON, boardFlourish, numberFlourish, id,
	)
	return err
}

// DeleteStyle removes a style by ID. Returns true if a row was deleted.
func (s *Store) DeleteStyle(id int64) (bool, error) {
	res, err := s.db.Exec("DELETE FROM styles WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetActiveStyle returns the currently active style (tokens + generated CSS +
// flourishes), or nil when no active style is set or it no longer exists. Used
// by the active-CSS endpoint and the live style broadcast.
func (s *Store) GetActiveStyle() (*model.Style, error) {
	idStr, err := s.GetSetting("active_style_id")
	if err != nil || idStr == "" {
		return nil, err
	}
	return s.scanStyle(s.db.QueryRow(
		"SELECT id, name, tokens, board_flourish, number_flourish, created_at FROM styles WHERE id = ?", idStr,
	))
}

// GetActiveStyleCSS returns the generated CSS of the currently active style, or
// "" when none is set / it no longer exists.
func (s *Store) GetActiveStyleCSS() (string, error) {
	active, err := s.GetActiveStyle()
	if err != nil || active == nil {
		return "", err
	}
	return active.CSSContent, nil
}
