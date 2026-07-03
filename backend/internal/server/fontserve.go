package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"app-suite/internal/model"
)

// ── Public font serving (tokenized, origin-gated) ─────────────────────────────
//
// Uploaded fonts are licensed assets, so they are never served as static files.
// Instead the Go server serves them itself through obfuscated, rotating token
// URLs, gated per font by the requesting site's Origin:
//
//	GET /api/fonts/pub/kit.css     @font-face stylesheet for external sites
//	GET /api/fonts/pub/f/{token}   the font bytes behind an opaque token
//
// The fonts.senpan.cafe vhost reverse-proxies everything to these endpoints
// (ProxyPass / → http://localhost:8080/api/fonts/pub/), so an external Carrd
// site embeds  <link rel="stylesheet" href="https://fonts.senpan.cafe/kit.css">
// and the kit's relative url('f/<token>') sources resolve on the same host. The
// SPA does not use the kit: it builds its own @font-face rules (with metric
// clamping) from the tokens in the settings payload, loading fonts SAME-ORIGIN
// via /api/fonts/pub/f/{token} — so the app itself always has font access, no
// matter what any font's allowlist says.
//
// Access model per font request:
//   - Same-origin requests (the SPA, dev via the Vite proxy) — always allowed.
//   - Cross-origin requests — allowed only when the Origin header is on THAT
//     FONT's allowlist (Atelier → Font Upload → Edit); the response echoes the
//     origin in Access-Control-Allow-Origin, which browsers REQUIRE for
//     cross-origin @font-face, so a non-listed site cannot render the font.
//   - No usable Origin (address bar, wget, casual scrapers) — 403.
//
// The kit stylesheet is additionally filtered by the requesting site: a
// foreign Referer only sees @font-face rules for fonts that allow its origin.
//
// Tokens are deterministic HMACs over a coarse time bucket, so they need no
// storage, survive restarts, stay stable long enough to cache well, and still
// expire: a copied font URL goes stale after one to two bucket widths. This
// raises the bar considerably, but it is not DRM — a determined client that
// forges an Origin header can still fetch the bytes (true of every web font
// host, including the commercial ones).

// settingFontSecret is the settings key holding the HMAC key for font tokens.
// Deliberately not in settingsKeys (it must never leave the server).
const settingFontSecret = "font_url_secret"

// fontTokenBucketSeconds is the width of the token time bucket (one week).
// Tokens from the current and previous bucket are accepted, so any served URL
// stays valid for 7–14 days and font responses may be cached up to one bucket.
const fontTokenBucketSeconds = 7 * 24 * 60 * 60

// fontContentTypes maps served font extensions to their MIME types (Go's
// platform mime table is unreliable for fonts, so they are set explicitly).
var fontContentTypes = map[string]string{
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".eot":   "application/vnd.ms-fontobject",
}

// fontFormatHints maps font extensions to the CSS @font-face format() hint used
// in the generated kit stylesheet (mirrors the SPA's FONT_FORMAT_HINTS).
var fontFormatHints = map[string]string{
	".ttf":   "truetype",
	".otf":   "opentype",
	".woff":  "woff",
	".woff2": "woff2",
	".eot":   "embedded-opentype",
}

// fontSecret returns the HMAC key for font tokens, generating and persisting a
// 32-byte secret on first use. If persisting fails the in-memory secret is still
// used — tokens then rotate on restart, which only forces clients to re-fetch.
func (s *Server) fontSecret() []byte {
	s.fontSecretMu.Lock()
	defer s.fontSecretMu.Unlock()
	if s.fontSecretVal != nil {
		return s.fontSecretVal
	}
	if stored, err := s.store.GetSetting(settingFontSecret); err == nil && stored != "" {
		if b, err := hex.DecodeString(stored); err == nil && len(b) == 32 {
			s.fontSecretVal = b
			return b
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing means the platform RNG is broken; nothing sane to
		// do but log — the zeroed key still only guards font obfuscation.
		slog.Error("generate font token secret", "error", err)
	}
	if err := s.store.SetSetting(settingFontSecret, hex.EncodeToString(b)); err != nil {
		slog.Warn("font token secret not persisted; font tokens will rotate on restart", "error", err)
	}
	s.fontSecretVal = b
	return b
}

// fontTokenBucket returns the token time bucket for t.
func fontTokenBucket(t time.Time) int64 {
	return t.Unix() / fontTokenBucketSeconds
}

// fontFileToken derives the opaque serving token for an UPLOADED font file in
// a given time bucket: a truncated HMAC of the filename, plus the real
// (lowercased) extension so clients and the serving handler know the format
// without exposing the name.
func (s *Server) fontFileToken(name string, bucket int64) string {
	mac := hmac.New(sha256.New, s.fontSecret())
	fmt.Fprintf(mac, "font:%d:%s", bucket, name)
	sum := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sum[:16]) + strings.ToLower(filepath.Ext(name))
}

// fontDerivedToken derives the serving token for a group's CONVERTED WOFF2
// copy (distinct HMAC input, so it can never collide with an upload's token).
func (s *Server) fontDerivedToken(groupKey string, bucket int64) string {
	mac := hmac.New(sha256.New, s.fontSecret())
	fmt.Fprintf(mac, "font:%d:woff2:%s", bucket, groupKey)
	sum := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sum[:16]) + ".woff2"
}

// fontVariantByType returns the group member matching a variant type label
// ("" when the label names the converted copy or no member matches).
func (g fontGroup) fontVariantByType(label string) string {
	ext := fontTypeExts[label]
	for _, name := range g.Files {
		if strings.EqualFold(filepath.Ext(name), ext) {
			return name
		}
	}
	return ""
}

// servedFontVariant resolves which variant a group serves publicly, honoring
// the admin's Serve selection and falling back to the auto preference order
// (WOFF2 first — including the converted copy — then the remaining formats).
// Returns the serving token and the variant's type label.
func (s *Server) servedFontVariant(g fontGroup, m fontMeta, bucket int64) (token, typeLabel string) {
	_, hasConverted := s.fontDerivativeInfo(g.Key)

	// Explicit selection first (ignored when that variant no longer exists).
	if m.Serve != "" {
		if name := g.fontVariantByType(m.Serve); name != "" {
			return s.fontFileToken(name, bucket), m.Serve
		}
		if m.Serve == "WOFF2" && hasConverted {
			return s.fontDerivedToken(g.Key, bucket), "WOFF2"
		}
	}
	// Auto: preference order, with the converted copy standing in for WOFF2.
	for _, ext := range fontServePreference {
		label := fontTypeLabels[ext]
		if name := g.fontVariantByType(label); name != "" {
			return s.fontFileToken(name, bucket), label
		}
		if ext == ".woff2" && hasConverted {
			return s.fontDerivedToken(g.Key, bucket), "WOFF2"
		}
	}
	return "", "" // unreachable for a non-empty group
}

// fontFileForToken resolves a serving token to the absolute path of the file
// it serves (an upload in the fonts root, or a group's converted WOFF2 copy)
// plus the owning group's key, accepting the current and previous time
// buckets. The fonts dir is small, so recomputing per file is cheap;
// comparisons are constant-time.
func (s *Server) fontFileForToken(token string) (path, groupKey string, ok bool) {
	if token == "" || len(token) > 100 {
		return "", "", false
	}
	tok := []byte(token)
	current := fontTokenBucket(time.Now())
	for _, g := range s.fontGroupList() {
		for _, bucket := range []int64{current, current - 1} {
			for _, name := range g.Files {
				if subtle.ConstantTimeCompare([]byte(s.fontFileToken(name, bucket)), tok) == 1 {
					return filepath.Join(s.fontsDir(), name), g.Key, true
				}
			}
			if subtle.ConstantTimeCompare([]byte(s.fontDerivedToken(g.Key, bucket)), tok) == 1 {
				if _, exists := s.fontDerivativeInfo(g.Key); exists {
					return s.derivedFontPath(g.Key), g.Key, true
				}
			}
		}
	}
	return "", "", false
}

// uploadedFontRefs returns the fonts (one per group) with their effective CSS
// family names and current serving tokens — the shape the public settings
// payload exposes for the SPA's @font-face registration and header-font picker.
func (s *Server) uploadedFontRefs() []model.UploadedFont {
	groups := s.fontGroupList()
	if len(groups) == 0 {
		return nil
	}
	bucket := fontTokenBucket(time.Now())
	metas := s.fontMetaMap()
	refs := make([]model.UploadedFont, 0, len(groups))
	for _, g := range groups {
		token, _ := s.servedFontVariant(g, metas[g.Key], bucket)
		if token == "" {
			continue
		}
		refs = append(refs, model.UploadedFont{
			Name:   g.Base,
			Family: fontFamilyFor(g.Base, metas[g.Key]),
			Token:  token,
		})
	}
	return refs
}

// ── Origin allowlisting ───────────────────────────────────────────────────────

// normalizeFontOrigin validates a site origin ("https://host[:port]") and
// returns it normalized (lowercased, no path/query/fragment/credentials). A
// single trailing slash is tolerated since people paste URLs.
func normalizeFontOrigin(raw string) (string, bool) {
	raw = strings.TrimSuffix(strings.TrimSpace(raw), "/")
	if raw == "" || len(raw) > 300 {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "http" && scheme != "https") || u.Host == "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	return scheme + "://" + strings.ToLower(u.Host), true
}

// fontOriginAllowed reports whether an Origin header value is on a font's
// per-font allowlist (compared in normalized form).
func fontOriginAllowed(origin string, m fontMeta) bool {
	norm, ok := normalizeFontOrigin(origin)
	if !ok {
		return false
	}
	return slices.Contains(m.Origins, norm)
}

// fontRequestAllowed decides whether a font-file request may be served given
// the font's metadata, and returns the Origin value to echo in
// Access-Control-Allow-Origin ("" when the request carried none). Same-origin
// requests (the SPA) are always allowed; cross-origin requests need an Origin
// on the font's allowlist; requests with no usable Origin at all (address bar,
// plain fetch tools) are refused.
func (s *Server) fontRequestAllowed(r *http.Request, m fontMeta) (echoOrigin string, allowed bool) {
	if origin := r.Header.Get("Origin"); origin != "" {
		if u, err := url.Parse(origin); err == nil && sameHost(u.Hostname(), r.Host) {
			return origin, true // the app itself (or dev) — always allowed
		}
		if fontOriginAllowed(origin, m) {
			return origin, true
		}
		return "", false
	}
	// No Origin header. Browsers send one on every cross-origin font fetch, so
	// only same-origin loads may land here (some browsers omit Origin on those).
	if r.Header.Get("Sec-Fetch-Site") == "same-origin" {
		return "", true
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && sameHost(u.Hostname(), r.Host) {
			return "", true
		}
	}
	return "", false
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// cssFamilyEscape escapes a font family name for use in a single-quoted CSS
// string (filenames may contain quotes on some filesystems; better safe).
var cssFamilyEscape = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

// handleFontKitCSS serves the generated @font-face stylesheet external sites
// embed (via the fonts vhost as https://fonts.senpan.cafe/kit.css). Sources are
// RELATIVE ("f/<token>") so they resolve on whichever host served the CSS.
//
// The stylesheet is filtered per requesting site: a foreign Referer only sees
// the fonts whose allowlist includes its origin (a site with no allowed fonts
// gets an empty stylesheet). Same-host or Referer-less requests see every font
// — the tokens aren't secrets; each font FILE request re-checks its own
// allowlist, which is the real gate. Short cache so changes propagate quickly.
//
//	Endpoint:  GET /api/fonts/pub/kit.css
//	Auth:      public (content filtered by Referer origin)
//	Response:  text/css
func (s *Server) handleFontKitCSS(w http.ResponseWriter, r *http.Request) {
	// The requesting site's origin: "" means unfiltered (same-host / unknown).
	refOrigin := ""
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && !sameHost(u.Hostname(), r.Host) {
			refOrigin = strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
		}
	}

	bucket := fontTokenBucket(time.Now())
	metas := s.fontMetaMap()
	var b strings.Builder
	b.WriteString("/* Senpan font kit — generated. Font URLs are tokenized and rotate;\n")
	b.WriteString("   always reference fonts through this stylesheet, never by copied URL. */\n")
	for _, g := range s.fontGroupList() {
		m := metas[g.Key]
		if refOrigin != "" && !slices.Contains(m.Origins, refOrigin) {
			continue // this site isn't allowed to use this font
		}
		family := fontFamilyFor(g.Base, m)
		if family == "" {
			continue
		}
		token, _ := s.servedFontVariant(g, m, bucket)
		if token == "" {
			continue
		}
		format := ""
		if hint := fontFormatHints[strings.ToLower(filepath.Ext(token))]; hint != "" {
			format = fmt.Sprintf(" format('%s')", hint)
		}
		fmt.Fprintf(&b, "@font-face{font-family:'%s';src:url('f/%s')%s;font-display:swap;}\n",
			cssFamilyEscape.Replace(family), url.PathEscape(token), format)
	}

	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// PRIVATE: shared caches (Cloudflare fronts the site and ignores Vary on
	// non-image responses) must never store this — the content is filtered per
	// requesting site, and an edge-cached copy would leak one site's kit (and
	// tokens) to every other. Browsers may still cache it briefly.
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Vary", "Referer")
	_, _ = w.Write([]byte(b.String()))
}

// handleFontPublicFile streams a font file identified by its opaque token,
// applying the owning font's origin gate. The echoed
// Access-Control-Allow-Origin is what lets an allowed site's browser actually
// use the font — cross-origin @font-face loads hard-require CORS, so this
// check is enforcement, not advice.
//
//	Endpoint:  GET /api/fonts/pub/f/{token}
//	Auth:      public (per-font origin gate)
//	Response:  the font bytes (font/* content type)
func (s *Server) handleFontPublicFile(w http.ResponseWriter, r *http.Request) {
	path, groupKey, ok := s.fontFileForToken(r.PathValue("token"))
	if !ok {
		writeError(w, http.StatusNotFound, "Unknown or expired font link")
		return
	}
	origin, allowed := s.fontRequestAllowed(r, s.fontMetaMap()[groupKey])
	// The response varies by Origin (both the ACAO header and the 403), so
	// caches must key on it — set even on the refusal path.
	w.Header().Set("Vary", "Origin")
	if !allowed {
		writeError(w, http.StatusForbidden, "This font may only be used by approved sites")
		return
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	if ct := fontContentTypes[strings.ToLower(filepath.Ext(path))]; ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// PRIVATE, one bucket width (a browser-cached copy never outlives its
	// token's validity). It must not be "public": Cloudflare fronts the site,
	// caches *.woff2 by default, and ignores Vary: Origin — a shared-cache copy
	// primed by an ALLOWED request would then be served to ungated bare
	// requests, silently bypassing this handler's origin gate.
	w.Header().Set("Cache-Control", "private, max-age=604800")
	http.ServeFile(w, r, path)
}
