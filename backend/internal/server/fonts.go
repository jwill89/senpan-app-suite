package server

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"app-suite/internal/model"
)

// ── Font file management (System → Font Upload admin tab) ────────────────────
//
// Fonts live in <webRoot>/fonts and are served publicly from
// https://fonts.senpan.cafe/<file>. These admin-only endpoints let the admin
// list, upload (multiple at once), rename, and delete font files. Uploads of a
// name that already exists are rejected — the existing file must be deleted
// first — so an in-use font is never silently overwritten.

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

// fontFileNames returns the sorted base names of the font files in the fonts
// directory (font extensions only). Used by the public settings endpoint so the
// frontend can register @font-face rules for uploaded fonts. Returns nil when
// the directory is missing or unreadable.
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

// safeFontName validates and normalizes an uploaded/target font filename. It
// strips any directory components, rejects empty/dotfile names and disallowed
// extensions, and guards against path traversal. Returns the clean base name
// and true when valid.
func safeFontName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	// Strip any path the client may have included; we only keep the base name.
	name = filepath.Base(filepath.FromSlash(name))
	if name == "" || name == "." || name == ".." {
		return "", false
	}
	if strings.ContainsAny(name, `/\`) {
		return "", false
	}
	if !allowedFontExts[strings.ToLower(filepath.Ext(name))] {
		return "", false
	}
	return name, true
}

// handleFontsList returns the font files in <webRoot>/fonts.
//
//	Endpoint:  GET /api/fonts
//	Auth:      admin, or a user granted this page's permission
//	Response:  {"fonts": [{name, size, modified}]}
func (s *Server) handleFontsList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierFonts) {
		return
	}

	dir := s.fontsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access fonts directory")
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		writeInternalError(w, "read fonts dir", err)
		return
	}

	fonts := make([]model.FontFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !allowedFontExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fonts = append(fonts, model.FontFile{
			Name:     e.Name(),
			Size:     info.Size(),
			Modified: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	// Stable alphabetical order (case-insensitive) for a predictable table.
	sort.Slice(fonts, func(i, j int) bool {
		return strings.ToLower(fonts[i].Name) < strings.ToLower(fonts[j].Name)
	})

	writeJSON(w, http.StatusOK, model.FontsResponse{Fonts: fonts})
}

// fontRenameRequest is the JSON body for PATCH /api/fonts/{name}.
type fontRenameRequest struct {
	NewName string `json:"new_name"`
}

// handleFontDelete removes a font file. The filename comes from the path (URL
// decoding is automatic) and is still run through safeFontName to guard against
// path traversal and reject disallowed names.
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

	src := filepath.Join(s.fontsDir(), name)
	if err := os.Remove(src); err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "Font file not found")
			return
		}
		writeInternalError(w, "delete font", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleFontRename renames a font file. The current name comes from the path and
// the target from the body. Fails with 404 when the source is missing or 409
// when the target already exists (an in-use font is never clobbered).
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
	src := filepath.Join(dir, name)
	if newName == name {
		writeJSON(w, http.StatusOK, model.NamedOKResponse{OK: true, Name: newName})
		return
	}
	// Source must exist…
	if _, err := os.Stat(src); err != nil {
		writeError(w, http.StatusNotFound, "Font file not found")
		return
	}
	dst := filepath.Join(dir, newName)
	// …and the target must not (don't clobber an existing font).
	if _, err := os.Stat(dst); err == nil {
		writeError(w, http.StatusConflict, "A font named \""+newName+"\" already exists")
		return
	}
	if err := os.Rename(src, dst); err != nil {
		writeInternalError(w, "rename font", err)
		return
	}
	writeJSON(w, http.StatusOK, model.NamedOKResponse{OK: true, Name: newName})
}

// handleFontUpload handles multipart uploads of one or more font files to
// <webRoot>/fonts. A file whose name already exists is rejected (the existing
// one must be deleted first) so an in-use font is never overwritten. Each file
// is processed independently; the response reports which succeeded and which
// were skipped, so a partial batch still uploads the valid files.
//
//	Endpoint:  POST /api/fonts/upload
//	Auth:      admin, or a user granted this page's permission
//	Request:   multipart form with one or more "files" fields
//	Response:  {"uploaded": ["a.ttf"], "skipped": [{"name":"b.ttf","reason":"..."}]}
func (s *Server) handleFontUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierFonts) {
		return
	}

	// Cap the whole request at 64 MB (several font files at once).
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "Upload failed (max 64MB total)")
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "No files provided")
		return
	}

	dir := s.fontsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create fonts directory")
		return
	}

	uploaded := make([]string, 0, len(files))
	skipped := make([]model.SkippedUpload, 0)

	for _, header := range files {
		name, ok := safeFontName(header.Filename)
		if !ok {
			skipped = append(skipped, model.SkippedUpload{
				Name:   header.Filename,
				Reason: "Unsupported type (allowed: .ttf, .otf, .woff, .woff2, .eot)",
			})
			continue
		}

		dst := filepath.Join(dir, name)
		// Reject if a file with this name already exists.
		if _, err := os.Stat(dst); err == nil {
			skipped = append(skipped, model.SkippedUpload{
				Name:   name,
				Reason: "Already exists — delete the existing file first",
			})
			continue
		}

		if err := saveMultipartFile(header, dst); err != nil {
			skipped = append(skipped, model.SkippedUpload{Name: name, Reason: "Failed to save"})
			continue
		}
		uploaded = append(uploaded, name)
	}

	writeJSON(w, http.StatusOK, model.FontUploadResponse{
		Uploaded: uploaded,
		Skipped:  skipped,
	})
}
