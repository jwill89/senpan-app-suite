package server

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"app-suite/internal/db"
)

// handleBookclubUpload handles multipart cover-image uploads for reading list
// items, storing them under <webRoot>/images/bookclub and returning the full
// URL (per the requirement that covers are stored as full URLs).
//
//	Endpoint:  POST /api/bookclub/upload
//	Auth:      admin, or any book-club page permission
//	Request:   multipart form with "image" field
//	Response:  {"url": "https://host/images/bookclub/<original-filename>"}
func (s *Server) handleBookclubUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireAnyBookClub(w, r) {
		return
	}
	s.saveSingleImageUpload(w, r, bookclubCoverRelDir)
}

// siteBaseURL reconstructs this site's scheme://host for building absolute URLs
// to uploaded assets. Honors X-Forwarded-Proto (the app runs behind a reverse
// proxy) and falls back to TLS detection.
func (s *Server) siteBaseURL(r *http.Request) string {
	scheme := "https"
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	} else if r.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// removeUnusedCovers deletes each uploaded cover file that no reading-list item
// references. Covers keep their uploaded filename (the app does not rewrite upload
// names), so two items can share one file; this is called AFTER the owning item or
// list rows are deleted so the reference count reflects only the items that remain.
// External (AniList) urls are ignored by the removal.
func (s *Server) removeUnusedCovers(ctx context.Context, urls ...string) {
	q := db.New(s.store.ReadDB())
	for _, cover := range urls {
		if cover == "" {
			continue
		}
		if n, err := q.CountReadingListItemsByCover(ctx, cover); err != nil || n > 0 {
			continue
		}
		s.removeUploadedBookclubImage(cover)
	}
}

// removeUploadedBookclubImage deletes a previously uploaded cover image, but
// only when the URL points inside this site's images/bookclub directory. API
// covers (e.g. s4.anilist.co) and anything outside the upload area are left
// untouched.
func (s *Server) removeUploadedBookclubImage(coverURL string) {
	if coverURL == "" {
		return
	}
	// Reduce a full URL to its path so we can resolve it against webRoot.
	path := coverURL
	if u, err := url.Parse(coverURL); err == nil && u.Path != "" {
		path = u.Path
	}
	path = strings.TrimPrefix(path, "/")
	if !strings.HasPrefix(path, "images/bookclub/") {
		return
	}
	uploadDir, err := filepath.Abs(filepath.Join(s.webRoot, "images", "bookclub"))
	if err != nil {
		return
	}
	target, err := filepath.Abs(filepath.Join(s.webRoot, filepath.Clean(path)))
	if err != nil {
		return
	}
	if target == uploadDir || !strings.HasPrefix(target, uploadDir+string(os.PathSeparator)) {
		return
	}
	_ = os.Remove(target)
}
