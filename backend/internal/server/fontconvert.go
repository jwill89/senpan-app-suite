package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tdewolff/font"
)

// ── Font groups, WOFF2 conversion, and per-font metadata ─────────────────────
//
// A logical "font" is a GROUP of uploaded files sharing a base name (the
// filename minus its extension, case-insensitive): "Jasper.otf" and
// "Jasper.woff2" are two format VARIANTS of the font "Jasper". A group whose
// uploads include no WOFF2 gets one auto-converted into the hidden
// <webRoot>/fonts/.woff2/ directory (named "<groupkey>.woff2"); a group that
// already has an uploaded WOFF2 is never converted, and a stale converted copy
// is swept away.
//
// Each group carries admin-editable metadata (JSON map in the settings table,
// keyed by the lowercased base name):
//   - Family:  the CSS font-family name used by the kit/app/picker ("" = the
//     base name).
//   - Serve:   which variant TYPE is served publicly ("" = auto: WOFF2 when
//     available, else the best remaining format).
//   - Origins: the external site origins allowed to load THIS font (per-font
//     allowlist; the app's own origin is always allowed).

// fontDerivedDirName is the hidden sub-directory holding converted WOFF2 copies.
const fontDerivedDirName = ".woff2"

// settingFontMeta is the settings key holding the per-font metadata map
// (JSON: group key → fontMeta). Deliberately not in settingsKeys (managed via
// the fonts API, never the settings API).
const settingFontMeta = "font_meta"

// settingFontOriginsLegacy is the pre-group GLOBAL origin allowlist key; its
// value is migrated into every font's per-font Origins on startup, then cleared.
const settingFontOriginsLegacy = "font_allowed_origins"

// maxFontOrigins caps a single font's allowlist (sanity bound).
const maxFontOrigins = 100

// fontTypeLabels maps a variant file extension to its display/API type label.
var fontTypeLabels = map[string]string{
	".ttf": "TTF", ".otf": "OTF", ".woff": "WOFF", ".woff2": "WOFF2", ".eot": "EOT",
}

// fontTypeExts is the reverse of fontTypeLabels (label → extension).
var fontTypeExts = func() map[string]string {
	m := make(map[string]string, len(fontTypeLabels))
	for ext, label := range fontTypeLabels {
		m[label] = ext
	}
	return m
}()

// fontConvertPreference orders the source formats tried when converting a
// group to WOFF2 (all are lossless containers; this is just a stable choice).
var fontConvertPreference = []string{".ttf", ".otf", ".woff", ".eot"}

// fontServePreference orders the variant types served when the admin hasn't
// picked one (Serve == ""): the compressed WOFF2 first, then smaller-first
// web formats.
var fontServePreference = []string{".woff2", ".woff", ".ttf", ".otf", ".eot"}

// fontBase returns a filename's base (extension stripped) — the group's
// display name and the default CSS family.
func fontBase(name string) string {
	return strings.TrimSpace(strings.TrimSuffix(name, filepath.Ext(name)))
}

// fontGroupKey returns the group identity for a filename (lowercased base).
func fontGroupKey(name string) string {
	return strings.ToLower(fontBase(name))
}

// fontGroup is one logical font: its identity, display base, and member files.
type fontGroup struct {
	Key   string   // lowercased base (meta map key, derivative filename)
	Base  string   // display base (from the first member, sort order)
	Files []string // member filenames, sorted (fontFileNames order)
}

// fontGroupList builds the ordered group list from the fonts directory.
func (s *Server) fontGroupList() []fontGroup {
	var groups []fontGroup
	index := map[string]int{}
	for _, name := range s.fontFileNames() {
		key := fontGroupKey(name)
		if i, ok := index[key]; ok {
			groups[i].Files = append(groups[i].Files, name)
			continue
		}
		index[key] = len(groups)
		groups = append(groups, fontGroup{Key: key, Base: fontBase(name), Files: []string{name}})
	}
	return groups
}

// fontGroupByBase finds a group by its base name (case-insensitive).
func (s *Server) fontGroupByBase(base string) (fontGroup, bool) {
	key := strings.ToLower(strings.TrimSpace(base))
	for _, g := range s.fontGroupList() {
		if g.Key == key {
			return g, true
		}
	}
	return fontGroup{}, false
}

// hasUploadedWOFF2 reports whether the group includes an uploaded .woff2 file.
func (g fontGroup) hasUploadedWOFF2() bool {
	return slices.ContainsFunc(g.Files, func(n string) bool {
		return strings.EqualFold(filepath.Ext(n), ".woff2")
	})
}

// fontDerivedDir returns the absolute path of the converted-copies directory.
func (s *Server) fontDerivedDir() string {
	return filepath.Join(s.fontsDir(), fontDerivedDirName)
}

// derivedFontPath returns the absolute path of a group's converted WOFF2 copy.
func (s *Server) derivedFontPath(groupKey string) string {
	return filepath.Join(s.fontDerivedDir(), groupKey+".woff2")
}

// fontDerivativeInfo returns a group's converted-copy size (ok=false when the
// group has none on disk).
func (s *Server) fontDerivativeInfo(groupKey string) (size int64, ok bool) {
	info, err := os.Stat(s.derivedFontPath(groupKey))
	if err != nil || info.IsDir() {
		return 0, false
	}
	return info.Size(), true
}

// convertFontToWOFF2 converts font bytes in any supported format (TTF, OTF,
// TTC, WOFF, WOFF2, EOT) to WOFF2. The underlying parser handles hostile
// input, but it is an untagged third-party library fed admin uploads — a panic
// is recovered into an error so a bad file can never take the server down.
func convertFontToWOFF2(data []byte) (out []byte, err error) {
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

// refreshGroupDerivative reconciles one group's converted WOFF2 copy with its
// current members: a group with an uploaded WOFF2 (or no members) must have no
// converted copy; any other group gets one converted from its best source.
// Returns an error only for a failed conversion (the group then serves an
// uploaded format instead).
func (s *Server) refreshGroupDerivative(g fontGroup) error {
	dst := s.derivedFontPath(g.Key)
	if len(g.Files) == 0 || g.hasUploadedWOFF2() {
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			slog.Warn("remove stale font conversion", "font", g.Key, "error", err)
		}
		return nil
	}
	if _, err := os.Stat(dst); err == nil {
		return nil // already converted
	}
	// Pick the conversion source by format preference (first match wins).
	source := ""
	for _, ext := range fontConvertPreference {
		for _, name := range g.Files {
			if strings.EqualFold(filepath.Ext(name), ext) {
				source = name
				break
			}
		}
		if source != "" {
			break
		}
	}
	if source == "" {
		return fmt.Errorf("no convertible source in group")
	}
	data, err := os.ReadFile(filepath.Join(s.fontsDir(), source))
	if err != nil {
		return err
	}
	woff2, err := convertFontToWOFF2(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.fontDerivedDir(), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, woff2, 0644)
}

// refreshGroupDerivativeByKey is refreshGroupDerivative for a (possibly now
// empty) group identified by key alone.
func (s *Server) refreshGroupDerivativeByKey(key string) error {
	for _, g := range s.fontGroupList() {
		if g.Key == key {
			return s.refreshGroupDerivative(g)
		}
	}
	return s.refreshGroupDerivative(fontGroup{Key: key}) // empty → removes the copy
}

// sweepFontDerivatives removes files in the converted-copies directory that no
// current group expects: leftovers from the pre-group per-file naming, copies
// made redundant by an uploaded WOFF2, and orphans of renamed/deleted fonts.
func (s *Server) sweepFontDerivatives() {
	entries, err := os.ReadDir(s.fontDerivedDir())
	if err != nil {
		return
	}
	expected := map[string]bool{}
	for _, g := range s.fontGroupList() {
		if !g.hasUploadedWOFF2() {
			expected[g.Key+".woff2"] = true
		}
	}
	for _, e := range entries {
		if e.IsDir() || expected[e.Name()] {
			continue
		}
		if err := os.Remove(filepath.Join(s.fontDerivedDir(), e.Name())); err != nil {
			slog.Warn("sweep font conversion", "file", e.Name(), "error", err)
		}
	}
}

// migrateFontDerivatives reconciles every group's converted copy at startup:
// backfills conversions for groups that need one, and sweeps stale files
// (including the pre-group "<file>.woff2" naming). Idempotent, never fatal.
func (s *Server) migrateFontDerivatives() {
	converted, failed := 0, 0
	for _, g := range s.fontGroupList() {
		had := false
		if _, ok := s.fontDerivativeInfo(g.Key); ok {
			had = true
		}
		if err := s.refreshGroupDerivative(g); err != nil {
			failed++
			slog.Warn("font WOFF2 backfill failed; an uploaded format will be served", "font", g.Key, "error", err)
			continue
		}
		if _, ok := s.fontDerivativeInfo(g.Key); ok && !had {
			converted++
		}
	}
	s.sweepFontDerivatives()
	if converted > 0 || failed > 0 {
		slog.Info("font WOFF2 backfill", "converted", converted, "failed", failed)
	}
}

// ── Per-font metadata ─────────────────────────────────────────────────────────

// fontMeta is the admin-editable metadata for one font group.
type fontMeta struct {
	// Family is the custom CSS font-family name ("" = the group's base name).
	Family string `json:"family,omitempty"`
	// Serve is the variant type label served publicly ("TTF"/"WOFF2"/…;
	// "" = auto: WOFF2 when available, else the best remaining format).
	Serve string `json:"serve,omitempty"`
	// Origins is this font's external-site allowlist (normalized bare origins).
	Origins []string `json:"origins,omitempty"`

	// LegacyOriginal is the pre-group "serve the original file" flag, read only
	// so migrateFontMetaV2 can translate it into Serve. Never written back.
	LegacyOriginal bool `json:"original,omitempty"`
}

// isZero reports whether the entry carries no information worth persisting.
func (m fontMeta) isZero() bool {
	return m.Family == "" && m.Serve == "" && len(m.Origins) == 0
}

// fontMetaMap reads the per-font metadata map. Returns an empty map when unset
// or unreadable.
func (s *Server) fontMetaMap() map[string]fontMeta {
	raw, err := s.store.GetSetting(settingFontMeta)
	if err != nil || raw == "" {
		return map[string]fontMeta{}
	}
	metas := map[string]fontMeta{}
	if err := json.Unmarshal([]byte(raw), &metas); err != nil {
		return map[string]fontMeta{}
	}
	return metas
}

// saveFontMetaMap persists the metadata map, dropping empty entries (and the
// migration-only legacy flag) so deleted customizations don't accumulate.
func (s *Server) saveFontMetaMap(metas map[string]fontMeta) error {
	for key, m := range metas {
		m.LegacyOriginal = false
		if m.isZero() {
			delete(metas, key)
			continue
		}
		metas[key] = m
	}
	data, err := json.Marshal(metas)
	if err != nil {
		return err
	}
	return s.store.SetSetting(settingFontMeta, string(data))
}

// updateFontMeta applies mutate to a group's metadata entry and persists it.
func (s *Server) updateFontMeta(groupKey string, mutate func(*fontMeta)) error {
	metas := s.fontMetaMap()
	m := metas[groupKey]
	mutate(&m)
	metas[groupKey] = m
	return s.saveFontMetaMap(metas)
}

// fontFamilyFor returns a group's effective CSS family name.
func fontFamilyFor(base string, m fontMeta) string {
	if m.Family != "" {
		return m.Family
	}
	return base
}

// migrateFontMetaV2 upgrades pre-group metadata at startup, idempotently:
//   - v1 entries were keyed by FILENAME with an "original" bool — re-key them
//     by group and translate original→Serve (that file's type).
//   - the global origin allowlist becomes every font's per-font Origins, and
//     the legacy settings key is cleared.
func (s *Server) migrateFontMetaV2() {
	metas := s.fontMetaMap()
	changed := false
	for key, m := range metas {
		ext := strings.ToLower(filepath.Ext(key))
		if fontTypeLabels[ext] == "" {
			continue // already a group key
		}
		changed = true
		delete(metas, key)
		target := metas[fontGroupKey(key)]
		if target.Family == "" {
			target.Family = m.Family
		}
		if m.LegacyOriginal && target.Serve == "" {
			target.Serve = fontTypeLabels[ext]
		}
		metas[fontGroupKey(key)] = target
	}

	if legacy, err := s.store.GetSetting(settingFontOriginsLegacy); err == nil && legacy != "" {
		var origins []string
		if err := json.Unmarshal([]byte(legacy), &origins); err == nil && len(origins) > 0 {
			for _, g := range s.fontGroupList() {
				m := metas[g.Key]
				if len(m.Origins) == 0 {
					m.Origins = slices.Clone(origins)
					metas[g.Key] = m
					changed = true
				}
			}
		}
		if err := s.store.SetSetting(settingFontOriginsLegacy, ""); err == nil {
			slog.Info("migrated global font origins to per-font allowlists", "origins", len(origins))
		}
	}

	if changed {
		if err := s.saveFontMetaMap(metas); err != nil {
			slog.Warn("migrate font metadata", "error", err)
		}
	}
}

// renameFontMetaKey moves a group's metadata when a rename empties its group
// into a new one (oldKey no longer has files). The metadata only follows when
// the destination group has none of its own; it is dropped when oldKey still
// has members (the renamed file simply joins/creates the other group).
func (s *Server) renameFontMetaKey(oldKey, newKey string) {
	if oldKey == newKey {
		return
	}
	if _, stillExists := s.fontGroupByBase(oldKey); stillExists {
		return // other members keep the metadata
	}
	metas := s.fontMetaMap()
	m, ok := metas[oldKey]
	if !ok {
		return
	}
	delete(metas, oldKey)
	if _, taken := metas[newKey]; !taken {
		metas[newKey] = m
	}
	if err := s.saveFontMetaMap(metas); err != nil {
		slog.Warn("move font metadata", "font", oldKey, "error", err)
	}
}

// deleteFontMetaKey removes a group's metadata entry (used when its last file
// is deleted).
func (s *Server) deleteFontMetaKey(key string) {
	metas := s.fontMetaMap()
	if _, ok := metas[key]; !ok {
		return
	}
	delete(metas, key)
	if err := s.saveFontMetaMap(metas); err != nil {
		slog.Warn("delete font metadata", "font", key, "error", err)
	}
}
