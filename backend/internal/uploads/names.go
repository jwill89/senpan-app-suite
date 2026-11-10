// Package uploads holds the rules for files people upload that need no database or
// network: which names and extensions are accepted, what a file's bytes say it is,
// the SVG sanitizer, and the image rule that checks and re-encodes a picture.
package uploads

import (
	"path/filepath"
	"strings"
)

// IsRasterExt reports whether ext (lowercase, with its dot) is a raster image type.
func IsRasterExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	}
	return false
}

// IsLibraryExt reports whether ext may be stored in the image library: the raster
// types and SVG, which is sanitized first.
func IsLibraryExt(ext string) bool { return ext == ".svg" || IsRasterExt(ext) }

// IsCarrdExt reports whether ext may be stored in a Carrd project: the raster types
// and audio or video for embedding in the external sites.
func IsCarrdExt(ext string) bool {
	return ext == ".mp3" || ext == ".mp4" || IsRasterExt(ext)
}

// SafeName reduces an uploaded file name to a safe base name, accepting it only when
// allow approves its extension (lowercase, with its dot). It strips any path and
// refuses an empty, dot, hidden or separator-bearing name, so the result is safe as
// both the on-disk name and the URL's.
func SafeName(name string, allow func(ext string) bool) (string, bool) {
	name = strings.TrimSpace(name)
	name = filepath.Base(filepath.FromSlash(name))
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return "", false
	}
	if strings.ContainsAny(name, `/\`) {
		return "", false
	}
	if !allow(strings.ToLower(filepath.Ext(name))) {
		return "", false
	}
	return name, true
}

// Slugify lowercases s and keeps letters a-z and digits, turning spaces, underscores
// and hyphens into sep, collapsing repeats and trimming sep from both ends. Carrd
// folders use '-' and image-library folders '_'.
func Slugify(s string, sep byte) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteByte(sep)
		}
	}
	out := b.String()
	double := string([]byte{sep, sep})
	for strings.Contains(out, double) {
		out = strings.ReplaceAll(out, double, string(sep))
	}
	return strings.Trim(out, string(sep))
}

// ValidSlug reports whether name is a non-empty string of a-z, digits and sep: the
// shape Slugify produces. It validates a folder name a client sends.
func ValidSlug(name string, sep byte) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		allowed := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == rune(sep)
		if !allowed {
			return false
		}
	}
	return true
}
