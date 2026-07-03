package server

import (
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"app-suite/internal/model"
)

// ── Font file management (Atelier → Font Upload admin tab) ────────────────────
//
// Uploaded font FILES live flat in <webRoot>/fonts and group into logical FONTS
// by base name (see fontconvert.go). These admin-only endpoints let the admin
// list fonts (grouped, with variants), upload files (multiple at once), rename
// or delete individual files, edit a font's metadata (CSS family name, served
// variant, per-font allowed sites), and delete a whole font. Uploads of a name
// that already exists are rejected — the existing file must be deleted first —
// so an in-use font is never silently overwritten.
//
// Fonts are served publicly ONLY through the tokenized, origin-gated endpoints
// in fontserve.go (the fonts.senpan.cafe vhost reverse-proxies to them).

// allowedFontExts is the set of permitted font file extensions (lowercase).
var allowedFontExts = map[string]bool{
	".ttf":   true,
	".otf":   true,
	".woff":  true,
	".woff2": true,
	".eot":   true,
}

// fontsDir returns the absolute path to the fonts upload directory.
func (s *Server) fontsDir() string {
	return filepath.Join(s.webRoot, "fonts")
}

// fontFileNames returns the sorted base names of the uploaded font files in
// the fonts directory root (font extensions only; the .woff2 conversions
// sub-directory is skipped). Returns nil when the directory is missing or
// unreadable.
func (s *Server) fontFileNames() []string {
	entries, err := os.ReadDir(s.fontsDir())
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !allowedFontExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	return names
}

// safeFontName validates and normalizes an uploaded/target font filename,
// accepting only permitted font extensions. See safeUploadName (uploads.go).
func safeFontName(name string) (string, bool) {
	return safeUploadName(name, func(ext string) bool { return allowedFontExts[ext] })
}

// fontModTime returns a file's RFC3339 mod time ("" when unreadable).
func fontModTime(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return info.ModTime().UTC().Format(time.RFC3339)
}

// handleFontsList returns the fonts, grouped by base name with their variants
// (uploaded files + the converted WOFF2 copy), metadata, and serving tokens.
//
//	Endpoint:  GET /api/fonts
//	Auth:      admin, or a user granted this page's permission
//	Response:  {"fonts": [{base, family, serve, served_type, served_token,
//	            origins, modified, variants: [{name, type, converted, size,
//	            modified, token}]}]}
func (s *Server) handleFontsList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierFonts) {
		return
	}
	if err := os.MkdirAll(s.fontsDir(), 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access fonts directory")
		return
	}

	bucket := fontTokenBucket(time.Now())
	metas := s.fontMetaMap()
	groups := s.fontGroupList()
	fonts := make([]model.Font, 0, len(groups))
	for _, g := range groups {
		m := metas[g.Key]
		variants := make([]model.FontVariant, 0, len(g.Files)+1)
		newest := ""
		for _, name := range g.Files {
			path := filepath.Join(s.fontsDir(), name)
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			modified := info.ModTime().UTC().Format(time.RFC3339)
			if modified > newest {
				newest = modified
			}
			variants = append(variants, model.FontVariant{
				Name:     name,
				Type:     fontTypeLabels[strings.ToLower(filepath.Ext(name))],
				Size:     info.Size(),
				Modified: modified,
				Token:    s.fontFileToken(name, bucket),
			})
		}
		if size, ok := s.fontDerivativeInfo(g.Key); ok {
			variants = append(variants, model.FontVariant{
				Name:      g.Base + ".woff2",
				Type:      "WOFF2",
				Converted: true,
				Size:      size,
				Modified:  fontModTime(s.derivedFontPath(g.Key)),
				Token:     s.fontDerivedToken(g.Key, bucket),
			})
		}
		servedToken, servedType := s.servedFontVariant(g, m, bucket)
		origins := m.Origins
		if origins == nil {
			origins = []string{}
		}
		fonts = append(fonts, model.Font{
			Base:        g.Base,
			Family:      fontFamilyFor(g.Base, m),
			Serve:       m.Serve,
			ServedType:  servedType,
			ServedToken: servedToken,
			Origins:     origins,
			Modified:    newest,
			Variants:    variants,
		})
	}
	writeJSON(w, http.StatusOK, model.FontsResponse{Fonts: fonts})
}

// fontRenameRequest is the JSON body for PATCH /api/fonts/{name}: a file
// rename (metadata edits live on PATCH /api/fonts/families/{base}).
type fontRenameRequest struct {
	NewName string `json:"new_name"`
}

// handleFontDelete removes one uploaded font FILE (a single variant). The
// filename comes from the path (URL decoding is automatic) and is still run
// through safeFontName to guard against path traversal. The owning group's
// converted copy and metadata are reconciled: deleting the group's last file
// drops its metadata, deleting its uploaded WOFF2 re-converts one.
//
//	Endpoint:  DELETE /api/fonts/{name}
//	Auth:      admin, or a user granted this page's permission
//	Response:  204 No Content
func (s *Server) handleFontDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierFonts) {
		return
	}

	name, ok := safeFontName(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid font file name")
		return
	}

	if err := os.Remove(filepath.Join(s.fontsDir(), name)); err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "Font file not found")
			return
		}
		writeInternalError(w, "delete font", err)
		return
	}
	key := fontGroupKey(name)
	if err := s.refreshGroupDerivativeByKey(key); err != nil {
		slog.Warn("refresh font conversion after delete", "font", key, "error", err)
	}
	if _, stillExists := s.fontGroupByBase(key); !stillExists {
		s.deleteFontMetaKey(key)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleFontRename renames one uploaded font FILE. Fails with 404 when the
// source is missing or 409 when the target already exists (an in-use font is
// never clobbered). Group bookkeeping follows: both the old and new groups'
// converted copies are reconciled, and when the rename empties its old group
// the metadata moves to the new one.
//
//	Endpoint:  PATCH /api/fonts/{name}
//	Auth:      admin, or a user granted this page's permission
//	Request:   {"new_name": "..."}
//	Response:  {"ok": true, "name": newName}
func (s *Server) handleFontRename(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierFonts) {
		return
	}

	name, ok := safeFontName(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid font file name")
		return
	}

	req, err := readJSON[fontRenameRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	newName, ok := safeFontName(req.NewName)
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid new file name (allowed: .ttf, .otf, .woff, .woff2, .eot)")
		return
	}

	dir := s.fontsDir()
	if newName == name {
		writeJSON(w, http.StatusOK, model.NamedOKResponse{OK: true, Name: newName})
		return
	}
	// Source must exist…
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		writeError(w, http.StatusNotFound, "Font file not found")
		return
	}
	// …and the target must not (don't clobber an existing font file).
	dst := filepath.Join(dir, newName)
	if _, err := os.Stat(dst); err == nil {
		writeError(w, http.StatusConflict, "A font file named \""+newName+"\" already exists")
		return
	}
	if err := os.Rename(filepath.Join(dir, name), dst); err != nil {
		writeInternalError(w, "rename font", err)
		return
	}

	oldKey, newKey := fontGroupKey(name), fontGroupKey(newName)
	s.renameFontMetaKey(oldKey, newKey)
	for _, key := range []string{oldKey, newKey} {
		if err := s.refreshGroupDerivativeByKey(key); err != nil {
			slog.Warn("refresh font conversion after rename", "font", key, "error", err)
		}
	}
	writeJSON(w, http.StatusOK, model.NamedOKResponse{OK: true, Name: newName})
}

// fontFamilyRequest is the JSON body for PATCH /api/fonts/families/{base}: a
// partial update of one font's metadata. nil fields are left unchanged.
type fontFamilyRequest struct {
	// Family sets the CSS font-family name ("" resets to the base name).
	Family *string `json:"family"`
	// Serve sets the served variant type ("TTF"/"WOFF2"/…; "" = auto).
	Serve *string `json:"serve"`
	// Origins replaces this font's external-site allowlist.
	Origins *[]string `json:"origins"`
}

// handleFontFamilyPatch updates one font's metadata: its CSS family name, the
// variant type it serves publicly, and/or its per-font origin allowlist.
//
//	Endpoint:  PATCH /api/fonts/families/{base}
//	Auth:      admin, or a user granted this page's permission
//	Request:   any of {"family": "..."}, {"serve": "WOFF2"},
//	           {"origins": ["https://mysite.carrd.co", ...]}
//	Response:  {"ok": true}
func (s *Server) handleFontFamilyPatch(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierFonts) {
		return
	}
	g, ok := s.fontGroupByBase(r.PathValue("base"))
	if !ok {
		writeError(w, http.StatusNotFound, "Font not found")
		return
	}
	req, err := readJSON[fontFamilyRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Family == nil && req.Serve == nil && req.Origins == nil {
		writeError(w, http.StatusBadRequest, "Nothing to update")
		return
	}

	// Validate everything before writing, so a combined update can't half-apply.
	family := ""
	if req.Family != nil {
		family = strings.TrimSpace(*req.Family)
		if len(family) > 100 {
			writeError(w, http.StatusBadRequest, "Font name is too long (max 100 characters)")
			return
		}
		if strings.ContainsAny(family, `'"\`) {
			writeError(w, http.StatusBadRequest, "Font name may not contain quotes or backslashes")
			return
		}
		if family != "" {
			// Must stay unique across the other fonts' effective families (the
			// kit would otherwise emit two identical @font-face names).
			metas := s.fontMetaMap()
			for _, other := range s.fontGroupList() {
				if other.Key == g.Key {
					continue
				}
				if strings.EqualFold(fontFamilyFor(other.Base, metas[other.Key]), family) {
					writeError(w, http.StatusConflict, "Another font is already named \""+family+"\"")
					return
				}
			}
		}
	}

	serve := ""
	if req.Serve != nil {
		serve = strings.TrimSpace(*req.Serve)
		if serve != "" {
			if fontTypeExts[serve] == "" {
				writeError(w, http.StatusBadRequest, "Unknown font type: "+serve)
				return
			}
			_, hasConverted := s.fontDerivativeInfo(g.Key)
			if g.fontVariantByType(serve) == "" && !(serve == "WOFF2" && hasConverted) {
				writeError(w, http.StatusBadRequest, "This font has no "+serve+" variant")
				return
			}
		}
	}

	var origins []string
	if req.Origins != nil {
		if len(*req.Origins) > maxFontOrigins {
			writeError(w, http.StatusBadRequest, "Too many origins")
			return
		}
		seen := make(map[string]bool, len(*req.Origins))
		origins = make([]string, 0, len(*req.Origins))
		for _, raw := range *req.Origins {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			norm, ok := normalizeFontOrigin(raw)
			if !ok {
				writeError(w, http.StatusBadRequest,
					"Invalid site origin: "+raw+" (expected e.g. https://mysite.carrd.co — no path)")
				return
			}
			if seen[norm] {
				continue
			}
			seen[norm] = true
			origins = append(origins, norm)
		}
	}

	err = s.updateFontMeta(g.Key, func(m *fontMeta) {
		if req.Family != nil {
			// The base name spelled out explicitly is just the default.
			if strings.EqualFold(family, g.Base) {
				family = ""
			}
			m.Family = family
		}
		if req.Serve != nil {
			m.Serve = serve
		}
		if req.Origins != nil {
			m.Origins = origins
		}
	})
	if err != nil {
		writeInternalError(w, "save font metadata", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// handleFontFamilyDelete deletes a whole font: every uploaded variant file,
// the converted WOFF2 copy, and the metadata entry.
//
//	Endpoint:  DELETE /api/fonts/families/{base}
//	Auth:      admin, or a user granted this page's permission
//	Response:  204 No Content
func (s *Server) handleFontFamilyDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierFonts) {
		return
	}
	g, ok := s.fontGroupByBase(r.PathValue("base"))
	if !ok {
		writeError(w, http.StatusNotFound, "Font not found")
		return
	}
	for _, name := range g.Files {
		if err := os.Remove(filepath.Join(s.fontsDir(), name)); err != nil && !os.IsNotExist(err) {
			writeInternalError(w, "delete font file", err)
			return
		}
	}
	if err := os.Remove(s.derivedFontPath(g.Key)); err != nil && !os.IsNotExist(err) {
		slog.Warn("remove font conversion", "font", g.Key, "error", err)
	}
	s.deleteFontMetaKey(g.Key)
	w.WriteHeader(http.StatusNoContent)
}

// handleFontUpload handles multipart uploads of one or more font files to
// <webRoot>/fonts. A file whose name already exists is rejected (the existing
// one must be deleted first) so an in-use font is never overwritten. Each file
// is processed independently; the response reports which succeeded and which
// were skipped, so a partial batch still uploads the valid files.
//
// After each save the owning group's WOFF2 conversion is reconciled: a group
// with no uploaded WOFF2 gets one converted; uploading a real WOFF2 removes a
// now-redundant converted copy. Conversion failure keeps the upload (an
// uploaded format is served) and is reported in the warnings list.
//
//	Endpoint:  POST /api/fonts/upload
//	Auth:      admin, or a user granted this page's permission
//	Request:   multipart form with one or more "files" fields
//	Response:  {"uploaded": ["a.ttf"], "skipped": [{"name":"b.ttf","reason":"..."}],
//	            "warnings": ["c.ttf: ..."]}
func (s *Server) handleFontUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierFonts) {
		return
	}
	var warnings []string
	uploaded, skipped, ok := s.handleMultipartUploads(w, r,
		func() (string, bool) {
			dir := s.fontsDir()
			if err := os.MkdirAll(dir, 0755); err != nil {
				writeError(w, http.StatusInternalServerError, "Failed to create fonts directory")
				return "", false
			}
			return dir, true
		},
		func(header *multipart.FileHeader, destDir string) (string, string) {
			name, ok := safeFontName(header.Filename)
			if !ok {
				return header.Filename, "Unsupported type (allowed: .ttf, .otf, .woff, .woff2, .eot)"
			}
			dst := filepath.Join(destDir, name)
			// Reject if a file with this name already exists — an in-use font must
			// be deleted first, never silently overwritten.
			if _, err := os.Stat(dst); err == nil {
				return name, "Already exists — delete the existing file first"
			}
			if err := saveMultipartFile(header, dst); err != nil {
				return name, "Failed to save"
			}
			// Reconcile the group's WOFF2 conversion. Failure is a warning, not a
			// rejection — an uploaded format is served for this font instead.
			if err := s.refreshGroupDerivativeByKey(fontGroupKey(name)); err != nil {
				warnings = append(warnings,
					name+": WOFF2 conversion failed — an uploaded format will be served")
			}
			return name, ""
		},
	)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, model.FontUploadResponse{Uploaded: uploaded, Skipped: skipped, Warnings: warnings})
}
