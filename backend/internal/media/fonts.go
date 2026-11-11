package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tdewolff/font"

	"app-suite/internal/fonts"
)

// Uploaded fonts live flat in <dataDir>/fonts and group into fonts by base name
// (internal/fonts). A font with no WOFF2 among its files gets one converted into the
// hidden fonts/.woff2 folder, named for the font; a font that has one uploaded never
// is, and a stale copy is swept away. The metadata of every font (family, served
// format, allowed sites) is one JSON map in the settings table.
//
// Fonts are never served as static files. The public operations serve them through
// tokens that rotate, gated per font by the requesting site.

const (
	// settingMeta is the settings key of the metadata map, keyed by font group.
	settingMeta = "font_meta"
	// settingSecret is the settings key of the HMAC key the link tokens are made with.
	// It never leaves the server.
	settingSecret = "font_url_secret" //nolint:gosec // the setting's key, not its value
	// settingLegacyOrigins is a site allowlist shared by every font, read only so
	// MigrateMeta can give each font its own and then empty it.
	settingLegacyOrigins = "font_allowed_origins"
)

// Settings is the key-value store the font metadata and secret are kept in. Get answers
// "" and no error for a key that isn't set.
type Settings interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// Fonts is the uploaded fonts under a data folder.
type Fonts struct {
	root     string
	settings Settings

	secretMu sync.Mutex
	secret   []byte
	// metaMu serializes the metadata's read-modify-write: every font shares one JSON
	// map, so two edits landing together would each write back a copy missing the
	// other's change.
	metaMu sync.Mutex
}

// NewFonts is the uploaded fonts under dataDir, with their metadata in settings.
func NewFonts(dataDir string, settings Settings) *Fonts {
	return &Fonts{root: dataDir, settings: settings}
}

// Dir is the folder the font files are in.
func (f *Fonts) Dir() string { return filepath.Join(f.root, "fonts") }

func (f *Fonts) derivedDir() string { return filepath.Join(f.Dir(), fonts.DerivedDir) }

// DerivedPath is the path of a font's converted WOFF2 copy.
func (f *Fonts) DerivedPath(groupKey string) string {
	return filepath.Join(f.derivedDir(), groupKey+".woff2")
}

// derivedSize is the size of a font's converted copy, ok false when it has none.
func (f *Fonts) derivedSize(groupKey string) (int64, bool) {
	info, err := os.Stat(f.DerivedPath(groupKey))
	if err != nil || info.IsDir() {
		return 0, false
	}
	return info.Size(), true
}

// fileNames are the font files in the fonts folder, sorted ignoring case. The converted
// copies' sub-folder is skipped.
func (f *Fonts) fileNames() []string {
	entries, err := os.ReadDir(f.Dir())
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !fonts.AllowedExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return names
}

// Groups are the fonts on disk.
func (f *Fonts) Groups() []fonts.Group { return fonts.Groups(f.fileNames()) }

func (f *Fonts) groupByBase(base string) (fonts.Group, bool) {
	key := strings.ToLower(strings.TrimSpace(base))
	for _, g := range f.Groups() {
		if g.Key == key {
			return g, true
		}
	}
	return fonts.Group{}, false
}

// -- Metadata and the token secret ---------------------------------------------

// MetaMap reads the metadata of every font, empty when unset or unreadable.
func (f *Fonts) MetaMap(ctx context.Context) map[string]fonts.Meta {
	raw, err := f.settings.Get(ctx, settingMeta)
	if err != nil || raw == "" {
		return map[string]fonts.Meta{}
	}
	metas := map[string]fonts.Meta{}
	if err := json.Unmarshal([]byte(raw), &metas); err != nil {
		return map[string]fonts.Meta{}
	}
	return metas
}

// saveMetaMap saves the map, dropping entries with nothing in them and the
// read-only LegacyOriginal flag.
func (f *Fonts) saveMetaMap(ctx context.Context, metas map[string]fonts.Meta) error {
	for key, m := range metas {
		m.LegacyOriginal = false
		if m.IsZero() {
			delete(metas, key)
			continue
		}
		metas[key] = m
	}
	data, err := json.Marshal(metas)
	if err != nil {
		return err
	}
	return f.settings.Set(ctx, settingMeta, string(data))
}

func (f *Fonts) updateMeta(ctx context.Context, groupKey string, mutate func(*fonts.Meta)) error {
	f.metaMu.Lock()
	defer f.metaMu.Unlock()
	metas := f.MetaMap(ctx)
	m := metas[groupKey]
	mutate(&m)
	metas[groupKey] = m
	return f.saveMetaMap(ctx, metas)
}

// renameMetaKey moves a font's metadata to its new key when a rename emptied the old
// font. Metadata moves only if the new font has none of its own; when the old font
// still has files, the renamed file just joins or makes the other font.
func (f *Fonts) renameMetaKey(ctx context.Context, oldKey, newKey string) {
	if oldKey == newKey {
		return
	}
	if _, stillExists := f.groupByBase(oldKey); stillExists {
		return
	}
	f.metaMu.Lock()
	defer f.metaMu.Unlock()
	metas := f.MetaMap(ctx)
	m, ok := metas[oldKey]
	if !ok {
		return
	}
	delete(metas, oldKey)
	if _, taken := metas[newKey]; !taken {
		metas[newKey] = m
	}
	if err := f.saveMetaMap(ctx, metas); err != nil {
		slog.Warn("move font metadata", "font", oldKey, "error", err)
	}
}

func (f *Fonts) deleteMetaKey(ctx context.Context, key string) {
	f.metaMu.Lock()
	defer f.metaMu.Unlock()
	metas := f.MetaMap(ctx)
	if _, ok := metas[key]; !ok {
		return
	}
	delete(metas, key)
	if err := f.saveMetaMap(ctx, metas); err != nil {
		slog.Warn("delete font metadata", "font", key, "error", err)
	}
}

// Secret is the HMAC key the link tokens are made with, made and saved on first use.
// If saving fails the key is kept in memory, so tokens then only change at a restart.
func (f *Fonts) Secret(ctx context.Context) ([]byte, error) {
	f.secretMu.Lock()
	defer f.secretMu.Unlock()
	if f.secret != nil {
		return f.secret, nil
	}
	// Detached: a read cut short by a departing client must never pass for "no key
	// stored yet", which would mint a new key and break every published font link.
	ctx = context.WithoutCancel(ctx)
	stored, err := f.settings.Get(ctx, settingSecret)
	if err != nil {
		return nil, fmt.Errorf("read font token secret: %w", err)
	}
	if stored != "" {
		if b, err := hex.DecodeString(stored); err == nil && len(b) == 32 {
			f.secret = b
			return b, nil
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// A broken random source voids the whole scheme; never fall back to a known key.
		return nil, fmt.Errorf("font token secret: crypto/rand unavailable: %w", err)
	}
	if err := f.settings.Set(ctx, settingSecret, hex.EncodeToString(b)); err != nil {
		slog.Warn("font token secret not persisted; font tokens will change at a restart", "error", err)
	}
	f.secret = b
	return b, nil
}

// -- Listing ---------------------------------------------------------------------

// Variant is one format of a font: an uploaded file or the converted WOFF2 copy.
type Variant struct {
	Name      string
	Type      string
	Converted bool
	Size      int64
	Modified  time.Time
	Token     string
}

// Font is one font with its variants, its metadata and the format it serves.
type Font struct {
	Base        string
	Family      string
	Serve       string
	ServedType  string
	ServedToken string
	Origins     []string
	Modified    time.Time
	Variants    []Variant
}

// List is the fonts with their variants and current tokens.
func (f *Fonts) List(ctx context.Context, now time.Time) ([]Font, error) {
	if err := os.MkdirAll(f.Dir(), 0o755); err != nil { //nolint:gosec // an unserved folder; the server streams its files
		return nil, err
	}
	secret, err := f.Secret(ctx)
	if err != nil {
		return nil, err
	}
	bucket := fonts.TokenBucket(now)
	metas := f.MetaMap(ctx)
	groups := f.Groups()
	out := make([]Font, 0, len(groups))
	for _, g := range groups {
		m := metas[g.Key]
		font := Font{Base: g.Base, Family: fonts.FamilyFor(g.Base, m), Serve: m.Serve, Origins: m.Origins, Variants: []Variant{}}
		if font.Origins == nil {
			font.Origins = []string{}
		}
		for _, name := range g.Files {
			info, err := os.Stat(filepath.Join(f.Dir(), name))
			if err != nil {
				continue
			}
			if info.ModTime().After(font.Modified) {
				font.Modified = info.ModTime()
			}
			font.Variants = append(font.Variants, Variant{
				Name: name, Type: fonts.TypeLabels[strings.ToLower(filepath.Ext(name))], Size: info.Size(),
				Modified: info.ModTime(), Token: fonts.FileToken(secret, name, bucket),
			})
		}
		size, hasConverted := f.derivedSize(g.Key)
		if hasConverted {
			v := Variant{
				Name: g.Base + ".woff2", Type: "WOFF2", Converted: true, Size: size,
				Token: fonts.DerivedToken(secret, g.Key, bucket),
			}
			if info, err := os.Stat(f.DerivedPath(g.Key)); err == nil {
				v.Modified = info.ModTime()
			}
			font.Variants = append(font.Variants, v)
		}
		font.ServedToken, font.ServedType = fonts.ServedVariant(secret, g, m, hasConverted, bucket)
		out = append(out, font)
	}
	return out, nil
}

// Ref is an uploaded font as the public settings list it.
type Ref struct {
	Name, Family, Token string
}

// Refs lists the fonts that can be served, one per font, with their current tokens.
func (f *Fonts) Refs(ctx context.Context) ([]Ref, error) {
	groups := f.Groups()
	if len(groups) == 0 {
		return nil, nil
	}
	secret, err := f.Secret(ctx)
	if err != nil {
		return nil, err
	}
	bucket := fonts.TokenBucket(time.Now())
	metas := f.MetaMap(ctx)
	refs := make([]Ref, 0, len(groups))
	for _, g := range groups {
		_, has := f.derivedSize(g.Key)
		token, _ := fonts.ServedVariant(secret, g, metas[g.Key], has, bucket)
		if token == "" {
			continue
		}
		refs = append(refs, Ref{Name: g.Base, Family: fonts.FamilyFor(g.Base, metas[g.Key]), Token: token})
	}
	return refs, nil
}

// Kit is the fonts a site may load, to write into the kit stylesheet: every font when
// site is "", otherwise those whose allowlist names it.
func (f *Fonts) Kit(ctx context.Context, site string) ([]fonts.KitFont, error) {
	secret, err := f.Secret(ctx)
	if err != nil {
		return nil, err
	}
	bucket := fonts.TokenBucket(time.Now())
	metas := f.MetaMap(ctx)
	var out []fonts.KitFont
	for _, g := range f.Groups() {
		m := metas[g.Key]
		if site != "" && !slices.Contains(m.Origins, site) {
			continue
		}
		_, has := f.derivedSize(g.Key)
		token, _ := fonts.ServedVariant(secret, g, m, has, bucket)
		out = append(out, fonts.KitFont{Family: fonts.FamilyFor(g.Base, m), Token: token})
	}
	return out, nil
}

// Served is a font file a token names, for serving.
type Served struct {
	Path     string
	GroupKey string
	Meta     fonts.Meta
}

// FileForToken finds the file a token names (this week's or last week's) and the
// metadata of its font. ok is false for a token that names nothing.
func (f *Fonts) FileForToken(ctx context.Context, token string) (file Served, ok bool, err error) {
	secret, err := f.Secret(ctx)
	if err != nil {
		return Served{}, false, err
	}
	res, ok := fonts.Resolve(secret, f.Groups(), func(key string) bool {
		_, has := f.derivedSize(key)
		return has
	}, token, time.Now())
	if !ok {
		return Served{}, false, nil
	}
	path := f.DerivedPath(res.GroupKey)
	if !res.Derived {
		path = filepath.Join(f.Dir(), res.File)
	}
	return Served{Path: path, GroupKey: res.GroupKey, Meta: f.MetaMap(ctx)[res.GroupKey]}, true, nil
}

// -- Conversion --------------------------------------------------------------------

// ConvertToWOFF2 converts font bytes in any supported format to WOFF2. The parser is an
// untagged third-party library fed uploads, so a panic in it becomes an error and a
// bad file can't take the server down.
func ConvertToWOFF2(data []byte) (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("font parser panicked: %v", r)
		}
	}()
	sfntData, err := font.ToSFNT(data)
	if err != nil {
		return nil, fmt.Errorf("parse font: %w", err)
	}
	sfnt, err := font.ParseSFNT(sfntData, 0)
	if err != nil {
		return nil, fmt.Errorf("parse sfnt: %w", err)
	}
	woff2, err := sfnt.WriteWOFF2()
	if err != nil {
		return nil, fmt.Errorf("encode woff2: %w", err)
	}
	return woff2, nil
}

// refreshDerivative reconciles one font's converted copy with its files: a font with an
// uploaded WOFF2, or with no files, has none; any other gets one made from its best
// source. An error means only that the conversion failed, and the font then serves an
// uploaded format.
func (f *Fonts) refreshDerivative(g fonts.Group) error {
	dst := f.DerivedPath(g.Key)
	if len(g.Files) == 0 || g.HasUploadedWOFF2() {
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			slog.Warn("remove stale font conversion", "font", g.Key, "error", err)
		}
		return nil
	}
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	source := g.ConvertSource()
	if source == "" {
		return errors.New("no convertible source in group")
	}
	data, err := os.ReadFile(filepath.Join(f.Dir(), source)) //nolint:gosec // source is a file name from the fonts folder's own listing
	if err != nil {
		return err
	}
	woff2, err := ConvertToWOFF2(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(f.derivedDir(), 0o755); err != nil { //nolint:gosec // an unserved folder
		return err
	}
	return os.WriteFile(dst, woff2, 0o644) //nolint:gosec // named for a font already in the fonts folder
}

func (f *Fonts) refreshDerivativeByKey(key string) error {
	for _, g := range f.Groups() {
		if g.Key == key {
			return f.refreshDerivative(g)
		}
	}
	return f.refreshDerivative(fonts.Group{Key: key}) // no files left: removes the copy
}

// sweepDerivatives removes converted copies no font expects: copies under a name no
// font maps to, copies made redundant by an uploaded WOFF2, and orphans of fonts
// renamed or deleted.
func (f *Fonts) sweepDerivatives() {
	entries, err := os.ReadDir(f.derivedDir())
	if err != nil {
		return
	}
	expected := map[string]bool{}
	for _, g := range f.Groups() {
		if !g.HasUploadedWOFF2() {
			expected[g.Key+".woff2"] = true
		}
	}
	for _, e := range entries {
		if e.IsDir() || expected[e.Name()] {
			continue
		}
		if err := os.Remove(filepath.Join(f.derivedDir(), e.Name())); err != nil {
			slog.Warn("sweep font conversion", "file", e.Name(), "error", err)
		}
	}
}

// MigrateDerivatives reconciles every font's converted copy at startup, making the
// ones that are missing and sweeping the stale. It is idempotent and never fatal.
func (f *Fonts) MigrateDerivatives() {
	converted, failed := 0, 0
	for _, g := range f.Groups() {
		_, had := f.derivedSize(g.Key)
		if err := f.refreshDerivative(g); err != nil {
			failed++
			slog.Warn("font WOFF2 backfill failed; an uploaded format will be served", "font", g.Key, "error", err)
			continue
		}
		if _, has := f.derivedSize(g.Key); has && !had {
			converted++
		}
	}
	f.sweepDerivatives()
	if converted > 0 || failed > 0 {
		slog.Info("font WOFF2 backfill", "converted", converted, "failed", failed)
	}
}

// MigrateMeta converts metadata kept per file, once and idempotently: entries keyed by
// file name move to their font, with the "original" flag becoming the served format,
// and the shared allowlist becomes each font's own.
func (f *Fonts) MigrateMeta(ctx context.Context) {
	metas := f.MetaMap(ctx)
	changed := false
	for key, m := range metas {
		ext := strings.ToLower(filepath.Ext(key))
		if fonts.TypeLabels[ext] == "" {
			continue // already a font key
		}
		changed = true
		delete(metas, key)
		target := metas[fonts.GroupKey(key)]
		if target.Family == "" {
			target.Family = m.Family
		}
		if m.LegacyOriginal && target.Serve == "" {
			target.Serve = fonts.TypeLabels[ext]
		}
		metas[fonts.GroupKey(key)] = target
	}

	if legacy, err := f.settings.Get(ctx, settingLegacyOrigins); err == nil && legacy != "" {
		var origins []string
		if err := json.Unmarshal([]byte(legacy), &origins); err == nil && len(origins) > 0 {
			for _, g := range f.Groups() {
				m := metas[g.Key]
				if len(m.Origins) == 0 {
					m.Origins = slices.Clone(origins)
					metas[g.Key] = m
					changed = true
				}
			}
		}
		if err := f.settings.Set(ctx, settingLegacyOrigins, ""); err == nil {
			slog.Info("migrated global font origins to per-font allowlists", "origins", len(origins))
		}
	}

	if changed {
		if err := f.saveMetaMap(ctx, metas); err != nil {
			slog.Warn("migrate font metadata", "error", err)
		}
	}
}

// -- Changes -----------------------------------------------------------------------

// familyTakenBy reports whether another font already resolves to a CSS family name,
// ignoring case. Two fonts sharing one would write two @font-face rules of the same
// name into the kit, and whichever came second would silently win.
func (f *Fonts) familyTakenBy(ctx context.Context, family, exceptKey string) bool {
	metas := f.MetaMap(ctx)
	for _, other := range f.Groups() {
		if other.Key != exceptKey && strings.EqualFold(fonts.FamilyFor(other.Base, metas[other.Key]), family) {
			return true
		}
	}
	return false
}

// Changes are the edits to a font's metadata; a nil field is left as it is.
type Changes struct {
	// Family sets the CSS family name; blank resets it to the base name.
	Family *string
	// Serve sets the format served; blank means automatic.
	Serve *string
	// Origins replaces the allowlist.
	Origins *[]string
}

// Update validates and applies changes to the font whose base name is base, all or
// nothing, and answers the font as it is then.
func (f *Fonts) Update(ctx context.Context, base string, ch Changes) error {
	g, ok := f.groupByBase(base)
	if !ok {
		return notFound("There is no such font.")
	}

	family := ""
	if ch.Family != nil {
		family = strings.TrimSpace(*ch.Family)
		if len(family) > fonts.MaxFamilyLen {
			return invalid("family", "Font name is too long (max 100 characters)")
		}
		if fonts.NameUnsafe(family) {
			return invalid("family", "Font name may not contain quotes, backslashes, control characters, or any of { } ; < >")
		}
		if family != "" && f.familyTakenBy(ctx, family, g.Key) {
			return conflict("Another font is already named \"" + family + "\"")
		}
	}

	serve := ""
	if ch.Serve != nil {
		serve = strings.TrimSpace(*ch.Serve)
		if serve != "" {
			if fonts.TypeExts[serve] == "" {
				return invalid("serve", "Unknown font type: "+serve)
			}
			_, hasConverted := f.derivedSize(g.Key)
			if g.VariantByType(serve) == "" && (serve != "WOFF2" || !hasConverted) {
				return invalid("serve", "This font has no "+serve+" variant")
			}
		}
	}

	var origins []string
	if ch.Origins != nil {
		if len(*ch.Origins) > fonts.MaxOrigins {
			return invalid("origins", "Too many origins")
		}
		var bad string
		var ok bool
		if origins, bad, ok = fonts.NormalizeOrigins(*ch.Origins); !ok {
			return invalid("origins", "Invalid site origin: "+bad+" (expected e.g. https://mysite.carrd.co - no path)")
		}
	}

	return f.updateMeta(ctx, g.Key, func(m *fonts.Meta) {
		if ch.Family != nil {
			// The base name spelled out is just the default.
			if strings.EqualFold(family, g.Base) {
				family = ""
			}
			m.Family = family
		}
		if ch.Serve != nil {
			m.Serve = serve
		}
		if ch.Origins != nil {
			m.Origins = origins
		}
	})
}

// Get is the font whose base name is base, as List shows it.
func (f *Fonts) Get(ctx context.Context, base string, now time.Time) (Font, error) {
	list, err := f.List(ctx, now)
	if err != nil {
		return Font{}, err
	}
	key := strings.ToLower(strings.TrimSpace(base))
	for _, font := range list {
		if strings.ToLower(font.Base) == key {
			return font, nil
		}
	}
	return Font{}, notFound("There is no such font.")
}

// Add stores an uploaded font file. ErrExists means a file of that name is there: it
// must be deleted first, so a font in use is never overwritten. A conversion that
// fails doesn't fail the upload, whose own format is then served, and is reported as a
// warning.
func (f *Fonts) Add(name string, data []byte) (warning string, err error) {
	if err := os.MkdirAll(f.Dir(), 0o755); err != nil { //nolint:gosec // an unserved folder; the server streams its files
		return "", err
	}
	dst := filepath.Join(f.Dir(), name)
	if _, err := os.Stat(dst); err == nil { //nolint:gosec // name passed fonts.SafeName
		return "", ErrExists
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil { //nolint:gosec // name passed fonts.SafeName
		return "", err
	}
	if err := f.refreshDerivativeByKey(fonts.GroupKey(name)); err != nil {
		return name + ": WOFF2 conversion failed - an uploaded format will be served", nil //nolint:nilerr // a failed conversion is a warning, not a failed upload
	}
	return "", nil
}

// DeleteFile removes one font file, reconciling its font's converted copy and, when it
// was the last file, dropping the font's metadata. The name must be a safe font name.
func (f *Fonts) DeleteFile(ctx context.Context, name string) error {
	if err := os.Remove(filepath.Join(f.Dir(), name)); err != nil { //nolint:gosec // name passed fonts.SafeName
		if os.IsNotExist(err) {
			return notFound("There is no such font file.")
		}
		return err
	}
	key := fonts.GroupKey(name)
	if err := f.refreshDerivativeByKey(key); err != nil {
		slog.Warn("refresh font conversion after delete", "font", key, "error", err)
	}
	// The file is gone, so its metadata goes with it even if the client has.
	if _, stillExists := f.groupByBase(key); !stillExists {
		f.deleteMetaKey(context.WithoutCancel(ctx), key)
	}
	return nil
}

// RenameFile renames one font file. 404 when it isn't there, 409 when the new name is
// taken or would give the font a CSS family another font has. Both the old and new
// fonts' converted copies are reconciled, and the metadata follows when the rename
// empties the old font.
func (f *Fonts) RenameFile(ctx context.Context, name, newName string) error {
	if newName == name {
		return nil
	}
	if _, err := os.Stat(filepath.Join(f.Dir(), name)); err != nil { //nolint:gosec // name passed fonts.SafeName
		return notFound("There is no such font file.")
	}
	dst := filepath.Join(f.Dir(), newName)
	if _, err := os.Stat(dst); err == nil { //nolint:gosec // newName passed fonts.SafeName
		return conflict("A font file named \"" + newName + "\" already exists")
	}
	// The reads below answer "nothing stored" on any error, and the rename can't be
	// stopped once started, so they and the metadata move run detached.
	ctx = context.WithoutCancel(ctx)
	newKey := fonts.GroupKey(newName)
	// A rename changes the font's CSS family when it has no explicit one, so the same
	// uniqueness rule as a family edit applies.
	if newFamily := fonts.FamilyFor(fonts.Base(newName), f.MetaMap(ctx)[newKey]); newFamily != "" &&
		f.familyTakenBy(ctx, newFamily, newKey) {
		return conflict("Another font is already named \"" + newFamily + "\"")
	}
	if err := os.Rename(filepath.Join(f.Dir(), name), dst); err != nil { //nolint:gosec // both names passed fonts.SafeName
		return err
	}
	oldKey := fonts.GroupKey(name)
	// The file has moved; its metadata follows it, or the font would come back under
	// its new name without its family or its allowed sites.
	f.renameMetaKey(ctx, oldKey, newKey)
	for _, key := range []string{oldKey, newKey} {
		if err := f.refreshDerivativeByKey(key); err != nil {
			slog.Warn("refresh font conversion after rename", "font", key, "error", err)
		}
	}
	return nil
}

// DeleteFont removes a whole font: every file, the converted copy and the metadata.
func (f *Fonts) DeleteFont(ctx context.Context, base string) error {
	g, ok := f.groupByBase(base)
	if !ok {
		return notFound("There is no such font.")
	}
	for _, name := range g.Files {
		if err := os.Remove(filepath.Join(f.Dir(), name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Remove(f.DerivedPath(g.Key)); err != nil && !os.IsNotExist(err) {
		slog.Warn("remove font conversion", "font", g.Key, "error", err)
	}
	f.deleteMetaKey(context.WithoutCancel(ctx), g.Key)
	return nil
}
