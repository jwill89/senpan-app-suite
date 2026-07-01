package server

import (
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"app-suite/internal/model"
)

// This file holds the upload plumbing shared by the image-hosting features
// (raffles, announcements, book-club covers). Each feature stores its files
// under a fixed sub-path of <webRoot>; the relative path doubles as the public
// URL path, so it is the single source of truth for both the on-disk location
// and the returned URL.

// Relative (forward-slash) upload directories under <webRoot>. Used both to
// resolve the on-disk path (filepath.FromSlash) and to build the public URL.
const (
	bookclubCoverRelDir = "images/bookclub"
)

// isAllowedImageExt reports whether ext (lowercase, with dot) is a permitted
// uploaded-image extension.
func isAllowedImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	}
	return false
}

// safeImageUploadName reduces an uploaded filename to a safe basename and accepts
// it only when it carries an allowed raster-image extension. It strips any path,
// and rejects empty/dot/hidden names and embedded separators, so the result is safe
// to use directly as the on-disk (and URL) filename. Mirrors safeImageFileName
// (images.go) but for the raster-only cover set (no SVG).
func safeImageUploadName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	name = filepath.Base(filepath.FromSlash(name))
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return "", false
	}
	if strings.ContainsAny(name, `/\`) {
		return "", false
	}
	if !isAllowedImageExt(strings.ToLower(filepath.Ext(name))) {
		return "", false
	}
	return name, true
}

// saveMultipartFile streams a single multipart file part to dst. It is
// filename-agnostic (fonts, images, audio/video), so callers must validate the
// name/extension before calling it.
func saveMultipartFile(header *multipart.FileHeader, dst string) error {
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

// sniffedImageType returns the content type detected from the first bytes of r,
// then rewinds r to the start so it can be read again in full. Pairs with the
// extension check as defense in depth: it confirms the upload's *bytes* really
// are an image, so a non-image (e.g. an HTML or script file) renamed to .png is
// rejected rather than written to a publicly served directory.
func sniffedImageType(r io.ReadSeeker) string {
	head := make([]byte, 512)
	n, _ := io.ReadFull(r, head) // short reads are fine; n is what was read
	_, _ = r.Seek(0, io.SeekStart)
	return http.DetectContentType(head[:n])
}

// isAllowedImageContentType reports whether a sniffed content type is one of the
// raster image formats we accept (matches isAllowedImageExt).
func isAllowedImageContentType(ct string) bool {
	switch ct {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	}
	return false
}

// saveSingleImageUpload handles the single-image upload flow used by the book-club
// cover endpoint: it reads the "image" multipart field (max 5 MB), validates it as
// an allowed raster image, writes it under <webRoot>/<relDir> keeping the uploaded
// filename, and writes the JSON {"url": <fullURL>} success response. On any failure
// it writes the error response itself. relDir is a forward-slash path that doubles
// as the URL path. A same-named upload overwrites the existing file — matching the
// central image-hosting and Carrd uploads; the app no longer rewrites uploaded
// names (callers that auto-clean covers must therefore guard against shared files).
func (s *Server) saveSingleImageUpload(w http.ResponseWriter, r *http.Request, relDir string) {
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20) // 5 MB

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Image upload failed (max 5MB)")
		return
	}
	defer file.Close()

	name, ok := safeImageUploadName(header.Filename)
	if !ok {
		writeError(w, http.StatusBadRequest, "Only jpg, png, webp, and gif images are allowed")
		return
	}
	// Defense in depth: confirm the bytes are actually an image, not just the
	// extension. file is an io.ReadSeeker, rewound by sniffedImageType.
	if !isAllowedImageContentType(sniffedImageType(file)) {
		writeError(w, http.StatusBadRequest, "That file does not appear to be a valid image")
		return
	}

	destDir := filepath.Join(s.webRoot, filepath.FromSlash(relDir))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create upload directory")
		return
	}
	if err := saveMultipartFile(header, filepath.Join(destDir, name)); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save image")
		return
	}

	writeJSON(w, http.StatusOK, model.BookclubUploadResponse{URL: s.siteBaseURL(r) + "/" + relDir + "/" + name})
}
