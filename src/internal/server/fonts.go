package server

import (
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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

// fontFile is the JSON shape for a single font in the directory listing.
type fontFile struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"` // RFC3339
}

// fontsDir returns the absolute path to the fonts upload directory.
func (s *Server) fontsDir() string {
	return filepath.Join(s.webRoot, "fonts")
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
//	Auth:      admin
//	Response:  {"fonts": [{name, size, modified}]}
func (s *Server) handleFontsList(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
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

	fonts := make([]fontFile, 0, len(entries))
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
		fonts = append(fonts, fontFile{
			Name:     e.Name(),
			Size:     info.Size(),
			Modified: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	// Stable alphabetical order (case-insensitive) for a predictable table.
	sort.Slice(fonts, func(i, j int) bool {
		return strings.ToLower(fonts[i].Name) < strings.ToLower(fonts[j].Name)
	})

	writeJSON(w, http.StatusOK, map[string]any{"fonts": fonts})
}

// fontsActionRequest is the JSON body for POST /api/fonts.
// Action: "delete" or "rename".
type fontsActionRequest struct {
	Action  string `json:"action"`
	Name    string `json:"name"`
	NewName string `json:"new_name"`
}

// handleFontsAction processes admin rename/delete on font files.
//
//	Endpoint:  POST /api/fonts
//	Auth:      admin
//	Request:   {"action": "delete"|"rename", "name": "...", "new_name": "..."}
//	Response:  {"ok": true} (rename also returns {"name": newName})
func (s *Server) handleFontsAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	req, err := readJSON[fontsActionRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	name, ok := safeFontName(req.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid font file name")
		return
	}

	dir := s.fontsDir()
	src := filepath.Join(dir, name)

	switch req.Action {
	case "delete":
		if err := os.Remove(src); err != nil {
			if os.IsNotExist(err) {
				writeError(w, http.StatusNotFound, "Font file not found")
				return
			}
			writeInternalError(w, "delete font", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "rename":
		newName, ok := safeFontName(req.NewName)
		if !ok {
			writeError(w, http.StatusBadRequest, "Invalid new file name (allowed: .ttf, .otf, .woff, .woff2, .eot)")
			return
		}
		if newName == name {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": newName})
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
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": newName})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: delete, rename")
	}
}

// handleFontUpload handles multipart uploads of one or more font files to
// <webRoot>/fonts. A file whose name already exists is rejected (the existing
// one must be deleted first) so an in-use font is never overwritten. Each file
// is processed independently; the response reports which succeeded and which
// were skipped, so a partial batch still uploads the valid files.
//
//	Endpoint:  POST /api/fonts/upload
//	Auth:      admin
//	Request:   multipart form with one or more "files" fields
//	Response:  {"uploaded": ["a.ttf"], "skipped": [{"name":"b.ttf","reason":"..."}]}
func (s *Server) handleFontUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
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

	type skipEntry struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	}
	uploaded := make([]string, 0, len(files))
	skipped := make([]skipEntry, 0)

	for _, header := range files {
		name, ok := safeFontName(header.Filename)
		if !ok {
			skipped = append(skipped, skipEntry{
				Name:   header.Filename,
				Reason: "Unsupported type (allowed: .ttf, .otf, .woff, .woff2, .eot)",
			})
			continue
		}

		dst := filepath.Join(dir, name)
		// Reject if a file with this name already exists.
		if _, err := os.Stat(dst); err == nil {
			skipped = append(skipped, skipEntry{
				Name:   name,
				Reason: "Already exists — delete the existing file first",
			})
			continue
		}

		if err := saveUploadedFont(header, dst); err != nil {
			skipped = append(skipped, skipEntry{Name: name, Reason: "Failed to save"})
			continue
		}
		uploaded = append(uploaded, name)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"uploaded": uploaded,
		"skipped":  skipped,
	})
}

// saveUploadedFont streams a single multipart file part to dst.
func saveUploadedFont(header *multipart.FileHeader, dst string) error {
	src, err := header.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, src); err != nil {
		return err
	}
	return nil
}
