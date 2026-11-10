package fonts

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"app-suite/internal/uploads"
)

// A font is a group of uploaded files sharing a base name (the file name without its
// extension, compared ignoring case): Jasper.otf and Jasper.woff2 are two formats of
// the font Jasper. Each font has a CSS family name, a served format and the sites that
// may load it, kept as Meta.

// Limits on what a font's metadata may hold.
const (
	// MaxOrigins is the most sites one font's allowlist may name.
	MaxOrigins = 100
	// MaxFamilyLen is the longest CSS family name.
	MaxFamilyLen = 100
	// BucketSeconds is the width of the token time bucket: one week. A token from the
	// current or the previous bucket is accepted, so a served link works for one to
	// two weeks.
	BucketSeconds = 7 * 24 * 60 * 60
	// MaxTokenLen bounds the length of a token a request may present.
	MaxTokenLen = 100
)

// DerivedDir is the hidden folder inside the fonts folder that holds the converted
// WOFF2 copies.
const DerivedDir = ".woff2"

// AllowedExts are the font file extensions that may be uploaded.
var AllowedExts = map[string]bool{".ttf": true, ".otf": true, ".woff": true, ".woff2": true, ".eot": true}

// TypeLabels maps a font file extension to the label the API uses for its format.
var TypeLabels = map[string]string{
	".ttf": "TTF", ".otf": "OTF", ".woff": "WOFF", ".woff2": "WOFF2", ".eot": "EOT",
}

// TypeExts is TypeLabels the other way round.
var TypeExts = func() map[string]string {
	m := make(map[string]string, len(TypeLabels))
	for ext, label := range TypeLabels {
		m[label] = ext
	}
	return m
}()

// ContentTypes maps a served font extension to its content type, set explicitly since
// the platform's table is unreliable for fonts.
var ContentTypes = map[string]string{
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".eot":   "application/vnd.ms-fontobject",
}

// FormatHints maps a font extension to the format() hint of an @font-face rule.
var FormatHints = map[string]string{
	".ttf":   "truetype",
	".otf":   "opentype",
	".woff":  "woff",
	".woff2": "woff2",
	".eot":   "embedded-opentype",
}

// ConvertPreference orders the formats tried as the source of a WOFF2 conversion.
var ConvertPreference = []string{".ttf", ".otf", ".woff", ".eot"}

// ServePreference orders the formats served when none is picked: the compressed WOFF2
// first, then the smaller web formats.
var ServePreference = []string{".woff2", ".woff", ".ttf", ".otf", ".eot"}

// Base is a file name without its extension: a font's display name and its default
// CSS family.
func Base(name string) string {
	return strings.TrimSpace(strings.TrimSuffix(name, filepath.Ext(name)))
}

// GroupKey is a font's identity: the lowercased base of any of its files.
func GroupKey(name string) string { return strings.ToLower(Base(name)) }

// SafeName reduces an uploaded or targeted font file name to a safe base name with a
// font extension. The base also becomes the default CSS family, so a name carrying a
// CSS-breaking character is refused too, which closes the upload path the family
// edit's own check never sees.
func SafeName(name string) (string, bool) {
	n, ok := uploads.SafeName(name, func(ext string) bool { return AllowedExts[ext] })
	if !ok || NameUnsafe(Base(n)) {
		return "", false
	}
	return n, true
}

// Group is one font: its identity, its display base and its files.
type Group struct {
	// Key is the lowercased base; the metadata map's key and the converted copy's name.
	Key string
	// Base is the display base, taken from the first file.
	Base string
	// Files are the file names, in the order they were listed.
	Files []string
}

// Groups collects font file names (sorted, extensions already checked) into fonts, in
// the order each first appears.
func Groups(names []string) []Group {
	var groups []Group
	index := map[string]int{}
	for _, name := range names {
		key := GroupKey(name)
		if i, ok := index[key]; ok {
			groups[i].Files = append(groups[i].Files, name)
			continue
		}
		index[key] = len(groups)
		groups = append(groups, Group{Key: key, Base: Base(name), Files: []string{name}})
	}
	return groups
}

// HasUploadedWOFF2 reports whether one of the font's files is a WOFF2.
func (g Group) HasUploadedWOFF2() bool {
	return slices.ContainsFunc(g.Files, func(n string) bool {
		return strings.EqualFold(filepath.Ext(n), ".woff2")
	})
}

// VariantByType is the font's file in the format label names ("TTF", "WOFF2"), or ""
// when it has none.
func (g Group) VariantByType(label string) string {
	ext := TypeExts[label]
	for _, name := range g.Files {
		if strings.EqualFold(filepath.Ext(name), ext) {
			return name
		}
	}
	return ""
}

// ConvertSource is the file a WOFF2 copy is made from, by ConvertPreference, or "".
func (g Group) ConvertSource() string {
	for _, ext := range ConvertPreference {
		for _, name := range g.Files {
			if strings.EqualFold(filepath.Ext(name), ext) {
				return name
			}
		}
	}
	return ""
}

// Meta is what staff set for one font.
type Meta struct {
	// Family is the custom CSS family name; "" means the font's base name.
	Family string `json:"family,omitempty"`
	// Serve is the format served ("TTF", "WOFF2" and so on); "" means automatic.
	Serve string `json:"serve,omitempty"`
	// Origins are the sites that may load the font, as bare origins. The app's own
	// origin is always allowed and never listed.
	Origins []string `json:"origins,omitempty"`

	// LegacyOriginal is the flag from before fonts were grouped, read only so it can be
	// turned into Serve. It is never written.
	LegacyOriginal bool `json:"original,omitempty"`
}

// IsZero reports whether the entry holds nothing worth keeping.
func (m Meta) IsZero() bool {
	return m.Family == "" && m.Serve == "" && len(m.Origins) == 0
}

// FamilyFor is a font's CSS family name: the custom one, else its base.
func FamilyFor(base string, m Meta) string {
	if m.Family != "" {
		return m.Family
	}
	return base
}

// -- Tokens ----------------------------------------------------------------------

// TokenBucket is the token time bucket for t.
func TokenBucket(t time.Time) int64 { return t.Unix() / BucketSeconds }

// FileToken is the opaque link token of an uploaded file in a time bucket: a
// truncated HMAC of its name, with the file's real extension (lowercased) so the
// format is known without exposing the name.
func FileToken(secret []byte, name string, bucket int64) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = fmt.Fprintf(mac, "font:%d:%s", bucket, name) // a hash's Write never fails
	sum := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sum[:16]) + strings.ToLower(filepath.Ext(name))
}

// DerivedToken is the link token of a font's converted WOFF2 copy. Its HMAC input
// differs from an upload's, so the two can never collide.
func DerivedToken(secret []byte, groupKey string, bucket int64) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = fmt.Fprintf(mac, "font:%d:woff2:%s", bucket, groupKey) // a hash's Write never fails
	sum := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sum[:16]) + ".woff2"
}

// ServedVariant picks the format a font serves publicly, honoring the saved choice and
// falling back to ServePreference (WOFF2 first, the converted copy standing in for it
// when there is one). It answers the link token and the format's label, "" for a font
// with nothing to serve.
func ServedVariant(secret []byte, g Group, m Meta, hasConverted bool, bucket int64) (token, label string) {
	if m.Serve != "" {
		if name := g.VariantByType(m.Serve); name != "" {
			return FileToken(secret, name, bucket), m.Serve
		}
		if m.Serve == "WOFF2" && hasConverted {
			return DerivedToken(secret, g.Key, bucket), "WOFF2"
		}
	}
	for _, ext := range ServePreference {
		l := TypeLabels[ext]
		if name := g.VariantByType(l); name != "" {
			return FileToken(secret, name, bucket), l
		}
		if ext == ".woff2" && hasConverted {
			return DerivedToken(secret, g.Key, bucket), "WOFF2"
		}
	}
	return "", ""
}

// Resolved is the file a link token names.
type Resolved struct {
	// GroupKey is the font it belongs to.
	GroupKey string
	// File is the uploaded file's name, "" for a converted copy.
	File string
	// Derived says the token names the font's converted WOFF2 copy.
	Derived bool
}

// Resolve finds the file a token names, accepting the current and the previous time
// bucket. hasConverted says whether a font has a converted copy on disk. Every
// comparison is constant-time.
func Resolve(secret []byte, groups []Group, hasConverted func(groupKey string) bool, token string, now time.Time) (Resolved, bool) {
	if token == "" || len(token) > MaxTokenLen {
		return Resolved{}, false
	}
	tok := []byte(token)
	current := TokenBucket(now)
	for _, g := range groups {
		for _, bucket := range []int64{current, current - 1} {
			for _, name := range g.Files {
				if subtle.ConstantTimeCompare([]byte(FileToken(secret, name, bucket)), tok) == 1 {
					return Resolved{GroupKey: g.Key, File: name}, true
				}
			}
			if subtle.ConstantTimeCompare([]byte(DerivedToken(secret, g.Key, bucket)), tok) == 1 && hasConverted(g.Key) {
				return Resolved{GroupKey: g.Key, Derived: true}, true
			}
		}
	}
	return Resolved{}, false
}

// -- Origins -----------------------------------------------------------------------

// NormalizeOrigin validates a site origin ("https://host[:port]") and returns it
// lowercased with no path, query, fragment or credentials. One trailing slash is
// tolerated, since people paste addresses.
func NormalizeOrigin(raw string) (string, bool) {
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

// NormalizeOrigins cleans a list of origins: blanks dropped, each normalized,
// duplicates removed. bad is the first entry that isn't an origin.
func NormalizeOrigins(raw []string) (origins []string, bad string, ok bool) {
	seen := make(map[string]bool, len(raw))
	origins = make([]string, 0, len(raw))
	for _, r := range raw {
		if strings.TrimSpace(r) == "" {
			continue
		}
		norm, valid := NormalizeOrigin(r)
		if !valid {
			return nil, r, false
		}
		if seen[norm] {
			continue
		}
		seen[norm] = true
		origins = append(origins, norm)
	}
	return origins, "", true
}

// OriginAllowed reports whether an Origin header value is on the font's allowlist.
func OriginAllowed(origin string, m Meta) bool {
	norm, ok := NormalizeOrigin(origin)
	return ok && slices.Contains(m.Origins, norm)
}

// RefererSite is the "scheme://host" of a parsed Referer, lowercased the way the
// allowlist is stored.
func RefererSite(u *url.URL) string {
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}

// SameHost reports whether an origin's hostname matches the request's host, port
// ignored, so the dev setup's :5173 origin against a :8080 host still counts.
func SameHost(originHost, requestHost string) bool {
	if h, _, err := net.SplitHostPort(requestHost); err == nil {
		requestHost = h
	}
	return strings.EqualFold(originHost, requestHost)
}

// Request is the parts of a font file request the origin gate reads.
type Request struct {
	// Host is the request's Host.
	Host string
	// Origin, Referer and SecFetchSite are the headers of those names.
	Origin, Referer, SecFetchSite string
}

// RequestAllowed decides whether a font file request may be served, and answers the
// origin to echo in Access-Control-Allow-Origin ("" when the request has none).
//
// The app's own pages are always allowed. A cross-origin request needs its site on the
// font's allowlist: named by Origin, or, when there is no Origin, by the Referer. That
// is Safari, which fetches @font-face without CORS and so sends no Origin; the Referer
// names the embedding page. A refused Origin is final: the Referer isn't consulted
// after the browser's own answer said no. A request naming no site at all (an address
// bar, a script) is refused.
func RequestAllowed(r Request, m Meta) (echoOrigin string, allowed bool) {
	if r.Origin != "" {
		if u, err := url.Parse(r.Origin); err == nil && SameHost(u.Hostname(), r.Host) {
			return r.Origin, true
		}
		if OriginAllowed(r.Origin, m) {
			return r.Origin, true
		}
		return "", false
	}
	if r.SecFetchSite == "same-origin" {
		return "", true
	}
	if r.Referer != "" {
		if u, err := url.Parse(r.Referer); err == nil {
			if SameHost(u.Hostname(), r.Host) {
				return "", true
			}
			if slices.Contains(m.Origins, RefererSite(u)) {
				return "", true
			}
		}
	}
	return "", false
}

// KitSite is the site a kit.css request comes from, to filter the kit by: the
// Referer's origin when it is another host, "" (everything) for the same host or no
// Referer.
func KitSite(referer, host string) string {
	if referer == "" {
		return ""
	}
	u, err := url.Parse(referer)
	if err != nil || SameHost(u.Hostname(), host) {
		return ""
	}
	return RefererSite(u)
}

// -- The kit stylesheet ----------------------------------------------------------

// KitFont is one font of the kit stylesheet.
type KitFont struct {
	// Family is its CSS family name.
	Family string
	// Token is the link token of the served format.
	Token string
}

var familyEscape = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

// KitCSS renders the stylesheet outside sites embed. Sources are relative
// ("f/<token>") so they resolve on whichever host served it. A font whose family could
// break out of the rule is left out rather than written.
func KitCSS(list []KitFont) string {
	var b strings.Builder
	b.WriteString("/* Senpan font kit - generated. Font URLs are tokenized and rotate;\n")
	b.WriteString("   always reference fonts through this stylesheet, never by copied URL. */\n")
	for _, f := range list {
		if f.Family == "" || NameUnsafe(f.Family) || f.Token == "" {
			continue
		}
		format := ""
		if hint := FormatHints[strings.ToLower(filepath.Ext(f.Token))]; hint != "" {
			format = fmt.Sprintf(" format('%s')", hint)
		}
		fmt.Fprintf(&b, "@font-face{font-family:'%s';src:url('f/%s')%s;font-display:swap;}\n",
			familyEscape.Replace(f.Family), url.PathEscape(f.Token), format)
	}
	return b.String()
}
