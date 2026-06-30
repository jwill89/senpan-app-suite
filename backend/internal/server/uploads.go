package server

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// saveSingleImageUpload handles the common single-image upload flow shared by the
// announcement and book-club cover endpoints: it reads the "image"
// multipart field (max 5 MB), validates it as an allowed image type, writes it
// under <webRoot>/<relDir> as "<prefix>_<nanos><ext>", and writes the JSON
// {"url": <fullURL>} success response. On any failure it writes the error
// response itself. relDir is a forward-slash path that doubles as the URL path.
func (s *Server) saveSingleImageUpload(w http.ResponseWriter, r *http.Request, relDir, prefix string) {
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20) // 5 MB

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Image upload failed (max 5MB)")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !isAllowedImageExt(ext) {
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
	filename := fmt.Sprintf("%s_%d%s", prefix, time.Now().UnixNano(), ext)

	dst, err := os.Create(filepath.Join(destDir, filename))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save image")
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save image")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"url": s.siteBaseURL(r) + "/" + relDir + "/" + filename})
}
