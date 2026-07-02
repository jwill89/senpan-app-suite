package server

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"app-suite/internal/model"
)

// ── Central image hosting (System → Images admin page) ───────────────────────
//
// Image "categories" are curated subdirectories of <webRoot>/images. Each
// category maps to exactly one subfolder; uploaded images for that category live
// directly in it and are served publicly from /images/<dir>/<file>.
//
// Three categories are PERMANENT (hardcoded, not renamable/deletable) and back
// the announcement + raffle editors:
//
//	"Announcement Main"      → images/announcements_main
//	"Announcement Thumbnail" → images/announcements_thumb
//	"Raffle"                 → images/raffles
//
// Admins may add custom categories (a display name + a directory name). Custom
// categories are tracked in a dotfile manifest at <webRoot>/images/.categories.json
// — the directory tree holds the files, the manifest only records the
// human-readable name ↔ directory mapping (mirrors the carrd title sidecar
// approach, so no database/model change is needed). Deleting a custom category
// removes its folder and all files within it.
//
// Like carrd (and unlike fonts), an uploaded image whose name already exists
// OVERWRITES the existing file.

// imageCategoriesManifest is the dotfile under <webRoot>/images that records the
// custom (non-permanent) categories. A dotfile so it is never served/listed.
const imageCategoriesManifest = ".categories.json"

// Permanent category directories (also referenced by canAccessImageDir and the
// announcement/raffle editors). Kept as constants so callers can name them.
const (
	imageDirAnnouncementsMain  = "announcements_main"
	imageDirAnnouncementsThumb = "announcements_thumb"
	imageDirRaffles            = "raffles"
	imageDirGarapons           = "garapons"
	imageDirFlourishes         = "flourishes"
	imageDirAffiliateLogos     = "affiliate_logos"
	imageDirAffiliateImages    = "affiliate_images"
	imageDirStampCards         = "stamp_cards"
	imageDirStampStamps        = "stamp_stamps"
	imageDirStampPrizes        = "stamp_prizes"
)

// permanentImageCategories returns the fixed categories, in a stable order.
func permanentImageCategories() []model.ImageCategory {
	return []model.ImageCategory{
		{Name: "Announcement Main", Dir: imageDirAnnouncementsMain, Permanent: true},
		{Name: "Announcement Thumbnail", Dir: imageDirAnnouncementsThumb, Permanent: true},
		{Name: "Raffle", Dir: imageDirRaffles, Permanent: true},
		{Name: "Garapon", Dir: imageDirGarapons, Permanent: true},
		{Name: "Flourishes", Dir: imageDirFlourishes, Permanent: true},
		{Name: "Affiliate Logos", Dir: imageDirAffiliateLogos, Permanent: true},
		{Name: "Affiliate Images", Dir: imageDirAffiliateImages, Permanent: true},
		{Name: "Stamp Cards", Dir: imageDirStampCards, Permanent: true},
		{Name: "Stamp Stamps", Dir: imageDirStampStamps, Permanent: true},
		{Name: "Stamp Prizes", Dir: imageDirStampPrizes, Permanent: true},
	}
}

// isAllowedImagesExt reports whether ext (lowercase, with dot) may be uploaded in
// the central Images section. It extends the raster image types with SVG (used by
// the theme flourishes); SVG stays scoped to this section (carrd has its own
// allow-list), and validation of the SVG bytes happens in handleImagesUpload.
func isAllowedImagesExt(ext string) bool {
	return ext == ".svg" || isAllowedImageExt(ext)
}

// isPermanentImageDir reports whether dir is one of the permanent category dirs.
func isPermanentImageDir(dir string) bool {
	for _, c := range permanentImageCategories() {
		if c.Dir == dir {
			return true
		}
	}
	return false
}

// imagesRootDir returns the absolute path to <webRoot>/images.
func (s *Server) imagesRootDir() string {
	return filepath.Join(s.webRoot, "images")
}

// slugifyImageDir derives a safe directory name from a category name (or a
// supplied directory): lowercase, spaces/hyphens become underscores, only
// letters/digits/underscores are kept, and runs of underscores are collapsed and
// trimmed. The underscore variant of carrd's slugifyFolder (per the spec, the
// directory name converts spaces to underscores).
func slugifyImageDir(s string) string { return slugify(s, '_') }

// validImageDir reports whether dir is a safe, already-normalized directory name
// (lowercase letters, digits, underscores only — no path separators). Guards
// against traversal for dir names received from the client on list/upload/delete.
func validImageDir(dir string) bool { return validSlug(dir, '_') }

// imageManifest is the JSON shape of the custom-categories manifest dotfile.
type imageManifest struct {
	Categories []model.ImageCategory `json:"categories"`
}

// readImageCategories reads the custom-category manifest. Returns an empty slice
// when the manifest is missing or unreadable (it isn't created until the first
// custom category is added).
func (s *Server) readImageCategories() []model.ImageCategory {
	data, err := os.ReadFile(filepath.Join(s.imagesRootDir(), imageCategoriesManifest))
	if err != nil {
		return []model.ImageCategory{}
	}
	var m imageManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return []model.ImageCategory{}
	}
	// Defensive: drop any entry that collides with a permanent dir or is invalid.
	out := make([]model.ImageCategory, 0, len(m.Categories))
	for _, c := range m.Categories {
		if validImageDir(c.Dir) && !isPermanentImageDir(c.Dir) {
			c.Permanent = false
			out = append(out, c)
		}
	}
	return out
}

// writeImageCategories persists the custom categories to the manifest dotfile.
func (s *Server) writeImageCategories(cats []model.ImageCategory) error {
	if err := os.MkdirAll(s.imagesRootDir(), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(imageManifest{Categories: cats})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.imagesRootDir(), imageCategoriesManifest), data, 0644)
}

// imageCategoryDirStats returns the number of image files in a category dir and
// their combined size. Missing dirs report zero (created on first upload).
func (s *Server) imageCategoryDirStats(dir string) (count int, totalSize int64) {
	entries, err := os.ReadDir(filepath.Join(s.imagesRootDir(), dir))
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() || !isAllowedImagesExt(strings.ToLower(filepath.Ext(e.Name()))) {
			continue
		}
		if info, err := e.Info(); err == nil {
			count++
			totalSize += info.Size()
		}
	}
	return count, totalSize
}

// allImageCategories returns the permanent categories followed by the custom
// ones, each with file counts/size populated.
func (s *Server) allImageCategories() []model.ImageCategory {
	cats := append(permanentImageCategories(), s.readImageCategories()...)
	for i := range cats {
		cats[i].FileCount, cats[i].TotalSize = s.imageCategoryDirStats(cats[i].Dir)
	}
	return cats
}

// canAccessImageDir reports whether the user may LIST a category's images. The
// management endpoints (upload/delete/category CRUD) require system-images, but
// the announcement and raffle editors need to read their own categories without
// it, so the permanent dirs map to those editors' page permissions.
func canAccessImageDir(u *model.User, dir string) bool {
	if u == nil {
		return false
	}
	if u.IsAdmin || userHasPermission(u, permSystemImages) {
		return true
	}
	switch dir {
	case imageDirAnnouncementsMain, imageDirAnnouncementsThumb:
		return userHasPermission(u, permTeahouseAnnounce)
	case imageDirRaffles:
		return userHasPermission(u, permTeahouseRaffles)
	case imageDirGarapons:
		return userHasPermission(u, permFestivalGarapon)
	case imageDirFlourishes:
		return userHasPermission(u, permSystemThemes)
	case imageDirAffiliateLogos, imageDirAffiliateImages:
		return userHasPermission(u, permTeahouseAffiliates)
	case imageDirStampCards, imageDirStampStamps, imageDirStampPrizes:
		return userHasPermission(u, permFestivalStampRally)
	}
	return false
}

// handleImageCategoriesList returns all image categories (permanent + custom).
//
//	Endpoint:  GET /api/image-categories
//	Auth:      admin, or a user granted system-images
//	Response:  {"categories": [{name, dir, permanent, file_count, total_size}]}
func (s *Server) handleImageCategoriesList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemImages) {
		return
	}
	if err := os.MkdirAll(s.imagesRootDir(), 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access images directory")
		return
	}
	writeJSON(w, http.StatusOK, model.ImageCategoriesResponse{Categories: s.allImageCategories()})
}

// imageCategoryCreateRequest is the JSON body for POST /api/image-categories.
type imageCategoryCreateRequest struct {
	Name string `json:"name"`
	Dir  string `json:"dir"`
}

// imageCategoryRenameRequest is the JSON body for PATCH /api/image-categories/{dir}.
// The existing directory comes from the path; NewDir is the desired one
// ("" keeps the current directory).
type imageCategoryRenameRequest struct {
	Name   string `json:"name"`
	NewDir string `json:"new_dir"`
}

// handleImageCategoryCreate creates a custom category.
//
//	Endpoint:  POST /api/image-categories
//	Auth:      admin, or a user granted system-images
//	Request:   {"name":"...","dir":"..."(optional)}
//	Response:  201 {"ok": true, "category": {...}}
func (s *Server) handleImageCategoryCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemImages) {
		return
	}
	req, err := readJSON[imageCategoryCreateRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if err := os.MkdirAll(s.imagesRootDir(), 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access images directory")
		return
	}
	s.createImageCategory(w, req.Name, req.Dir)
}

// handleImageCategoryRename renames a custom category. The current directory
// comes from the path; the new name/dir from the body.
//
//	Endpoint:  PATCH /api/image-categories/{dir}
//	Auth:      admin, or a user granted system-images
//	Request:   {"name":"...","new_dir":"..."(optional)}
//	Response:  200 {"ok": true, "category": {...}}
func (s *Server) handleImageCategoryRename(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemImages) {
		return
	}
	req, err := readJSON[imageCategoryRenameRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if err := os.MkdirAll(s.imagesRootDir(), 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access images directory")
		return
	}
	s.renameImageCategory(w, r.PathValue("dir"), req.Name, req.NewDir)
}

// handleImageCategoryDelete deletes a custom category (and its files).
//
//	Endpoint:  DELETE /api/image-categories/{dir}
//	Auth:      admin, or a user granted system-images
//	Response:  204 No Content
func (s *Server) handleImageCategoryDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemImages) {
		return
	}
	if err := os.MkdirAll(s.imagesRootDir(), 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access images directory")
		return
	}
	s.deleteImageCategory(w, r.PathValue("dir"))
}

// createImageCategory validates the name + directory, ensures the directory does
// not collide with a permanent or existing custom one, creates the folder, and
// records it in the manifest. reqName/reqDir are the raw request values.
func (s *Server) createImageCategory(w http.ResponseWriter, reqName, reqDir string) {
	name := strings.TrimSpace(reqName)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Category name is required")
		return
	}
	// Directory: use the supplied name (normalized) or derive it from the name.
	dir := slugifyImageDir(reqDir)
	if dir == "" {
		dir = slugifyImageDir(name)
	}
	if dir == "" {
		writeError(w, http.StatusBadRequest, "Could not derive a directory name — use letters or numbers in the name or directory")
		return
	}
	if isPermanentImageDir(dir) {
		writeError(w, http.StatusConflict, "That directory is reserved by a permanent category")
		return
	}

	cats := s.readImageCategories()
	for _, c := range cats {
		if c.Dir == dir {
			writeError(w, http.StatusConflict, "A category using the directory \""+dir+"\" already exists")
			return
		}
		if strings.EqualFold(c.Name, name) {
			writeError(w, http.StatusConflict, "A category named \""+name+"\" already exists")
			return
		}
	}

	if err := os.MkdirAll(filepath.Join(s.imagesRootDir(), dir), 0755); err != nil {
		writeInternalError(w, "create image category", err)
		return
	}
	cat := model.ImageCategory{Name: name, Dir: dir, Permanent: false}
	if err := s.writeImageCategories(append(cats, cat)); err != nil {
		writeInternalError(w, "write image categories", err)
		return
	}
	writeJSON(w, http.StatusCreated, model.ImageCategoryActionResponse{OK: true, Category: cat})
}

// renameImageCategory updates a custom category's display name and, when the
// directory changes, moves the folder on disk. Permanent categories are rejected.
// reqDir is the existing directory (from the path); reqName/reqNewDir are the
// raw request values.
func (s *Server) renameImageCategory(w http.ResponseWriter, reqDir, reqName, reqNewDir string) {
	dir := strings.TrimSpace(reqDir)
	if !validImageDir(dir) {
		writeError(w, http.StatusBadRequest, "Invalid directory name")
		return
	}
	if isPermanentImageDir(dir) {
		writeError(w, http.StatusForbidden, "Permanent categories cannot be modified")
		return
	}
	name := strings.TrimSpace(reqName)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Category name is required")
		return
	}
	newDir := slugifyImageDir(reqNewDir)
	if newDir == "" {
		newDir = slugifyImageDir(name)
	}
	if newDir == "" {
		writeError(w, http.StatusBadRequest, "Could not derive a directory name")
		return
	}
	if isPermanentImageDir(newDir) {
		writeError(w, http.StatusConflict, "That directory is reserved by a permanent category")
		return
	}

	cats := s.readImageCategories()
	idx := -1
	for i, c := range cats {
		if c.Dir == dir {
			idx = i
			continue
		}
		// Uniqueness against the OTHER categories.
		if c.Dir == newDir {
			writeError(w, http.StatusConflict, "A category using the directory \""+newDir+"\" already exists")
			return
		}
		if strings.EqualFold(c.Name, name) {
			writeError(w, http.StatusConflict, "A category named \""+name+"\" already exists")
			return
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "Category not found")
		return
	}

	if newDir != dir {
		src := filepath.Join(s.imagesRootDir(), dir)
		dst := filepath.Join(s.imagesRootDir(), newDir)
		// The source may not exist yet if no images were uploaded; only move when present.
		if info, err := os.Stat(src); err == nil && info.IsDir() {
			if err := os.Rename(src, dst); err != nil {
				if os.IsExist(err) {
					writeError(w, http.StatusConflict, "A directory named \""+newDir+"\" already exists")
					return
				}
				writeInternalError(w, "rename image category dir", err)
				return
			}
		} else if err := os.MkdirAll(dst, 0755); err != nil {
			writeInternalError(w, "create image category dir", err)
			return
		}
	}

	cats[idx] = model.ImageCategory{Name: name, Dir: newDir, Permanent: false}
	if err := s.writeImageCategories(cats); err != nil {
		writeInternalError(w, "write image categories", err)
		return
	}
	writeJSON(w, http.StatusOK, model.ImageCategoryActionResponse{OK: true, Category: cats[idx]})
}

// deleteImageCategory removes a custom category's folder (and all its files) and
// its manifest entry. Permanent categories cannot be deleted. reqDir is the
// directory (from the path).
func (s *Server) deleteImageCategory(w http.ResponseWriter, reqDir string) {
	dir := strings.TrimSpace(reqDir)
	if !validImageDir(dir) {
		writeError(w, http.StatusBadRequest, "Invalid directory name")
		return
	}
	if isPermanentImageDir(dir) {
		writeError(w, http.StatusForbidden, "Permanent categories cannot be deleted")
		return
	}

	cats := s.readImageCategories()
	idx := -1
	for i, c := range cats {
		if c.Dir == dir {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "Category not found")
		return
	}

	if err := os.RemoveAll(filepath.Join(s.imagesRootDir(), dir)); err != nil {
		writeInternalError(w, "delete image category", err)
		return
	}
	if err := s.writeImageCategories(append(cats[:idx], cats[idx+1:]...)); err != nil {
		writeInternalError(w, "write image categories", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// imageDirIsKnown reports whether dir is a permanent or a recorded custom
// category (so we never list/upload into arbitrary subfolders of images/).
func (s *Server) imageDirIsKnown(dir string) bool {
	if isPermanentImageDir(dir) {
		return true
	}
	for _, c := range s.readImageCategories() {
		if c.Dir == dir {
			return true
		}
	}
	return false
}

// handleImagesList returns the images in a category, newest first.
//
//	Endpoint:  GET /api/images?dir=<dir>
//	Auth:      admin/system-images, or the editor permission that owns the dir
//	           (teahouse-announcements for the announcement dirs, teahouse-raffles
//	           for the raffle dir).
//	Response:  {"dir": "...", "images": [{name, url, path, size, modified}]}
func (s *Server) handleImagesList(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized – login required")
		return
	}
	dir := strings.TrimSpace(r.URL.Query().Get("dir"))
	if !validImageDir(dir) || !s.imageDirIsKnown(dir) {
		writeError(w, http.StatusBadRequest, "Unknown image category")
		return
	}
	if !canAccessImageDir(u, dir) {
		writeError(w, http.StatusForbidden, "Forbidden – you do not have access to this category")
		return
	}

	entries, err := os.ReadDir(filepath.Join(s.imagesRootDir(), dir))
	if err != nil {
		// Missing dir (no uploads yet) → empty list, not an error.
		writeJSON(w, http.StatusOK, model.ImagesResponse{Dir: dir, Images: []model.ImageEntry{}})
		return
	}

	type infoPair struct {
		name string
		size int64
		mod  time.Time
	}
	infos := make([]infoPair, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !isAllowedImagesExt(strings.ToLower(filepath.Ext(e.Name()))) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		infos = append(infos, infoPair{name: e.Name(), size: fi.Size(), mod: fi.ModTime()})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].mod.After(infos[j].mod) })

	base := s.siteBaseURL(r)
	images := make([]model.ImageEntry, 0, len(infos))
	for _, info := range infos {
		rel := "images/" + dir + "/" + info.name
		images = append(images, model.ImageEntry{
			Name:     info.name,
			URL:      base + "/" + rel,
			Path:     rel,
			Size:     info.size,
			Modified: info.mod.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, model.ImagesResponse{Dir: dir, Images: images})
}

// handleImageDelete deletes a single image within a category. The image identity
// (dir + name) is supplied as query parameters.
//
//	Endpoint:  DELETE /api/images?dir=<dir>&name=<file>
//	Auth:      admin, or a user granted system-images
//	Response:  204 No Content
func (s *Server) handleImageDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemImages) {
		return
	}
	dir := strings.TrimSpace(r.URL.Query().Get("dir"))
	if !validImageDir(dir) || !s.imageDirIsKnown(dir) {
		writeError(w, http.StatusBadRequest, "Unknown image category")
		return
	}
	name, ok := safeImageFileName(r.URL.Query().Get("name"))
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid file name")
		return
	}
	if err := os.Remove(filepath.Join(s.imagesRootDir(), dir, name)); err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "File not found")
			return
		}
		writeInternalError(w, "delete image", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// safeImageFileName validates and normalizes an uploaded/target image filename:
// strips any path, rejects empty/dotfile names and disallowed extensions, and
// guards against traversal. Returns the clean base name and true when valid.
func safeImageFileName(name string) (string, bool) {
	return safeUploadName(name, isAllowedImagesExt)
}

// handleImagesUpload handles multipart uploads of one or more images to a
// category directory. An image whose name already exists OVERWRITES it. Each
// file is processed independently; the response reports which succeeded and which
// were skipped, so a partial batch still uploads the valid files.
//
//	Endpoint:  POST /api/images/upload
//	Auth:      admin, or a user granted system-images
//	Request:   multipart form with a "dir" field and one or more "files" fields
//	Response:  {"uploaded": ["a.png"], "skipped": [{"name":"b.txt","reason":"..."}]}
func (s *Server) handleImagesUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemImages) {
		return
	}
	uploaded, skipped, ok := s.handleMultipartUploads(w, r,
		func() (string, bool) {
			dir := strings.TrimSpace(r.FormValue("dir"))
			if !validImageDir(dir) || !s.imageDirIsKnown(dir) {
				writeError(w, http.StatusBadRequest, "Unknown image category")
				return "", false
			}
			destDir := filepath.Join(s.imagesRootDir(), dir)
			if err := os.MkdirAll(destDir, 0755); err != nil {
				writeError(w, http.StatusInternalServerError, "Failed to create category directory")
				return "", false
			}
			return destDir, true
		},
		func(header *multipart.FileHeader, destDir string) (string, string) {
			name, ok := safeImageFileName(header.Filename)
			if !ok {
				return header.Filename, "Unsupported type (allowed: .jpg, .jpeg, .png, .webp, .gif, .svg)"
			}
			// Defense in depth: confirm the bytes match the extension. SVG is
			// XML/text (the raster content-sniff would reject it), so it's handled
			// separately.
			f, err := header.Open()
			if err != nil {
				return name, "Failed to read"
			}
			if strings.EqualFold(filepath.Ext(name), ".svg") {
				// SVG is script-capable and served from our origin (and inlined via
				// v-html on the player board), so sanitize it before persisting
				// rather than storing the raw upload. sanitizeSVG also rejects
				// markup that isn't a parseable <svg>.
				raw, readErr := io.ReadAll(io.LimitReader(f, 2<<20)) // 2 MB cap for SVG text
				_ = f.Close()
				if readErr != nil {
					return name, "Failed to read"
				}
				clean, valid := sanitizeSVG(raw)
				if !valid {
					return name, "Not a valid SVG"
				}
				// Same name overwrites the existing file on purpose.
				if err := os.WriteFile(filepath.Join(destDir, name), clean, 0644); err != nil {
					return name, "Failed to save"
				}
				return name, ""
			}
			valid := isAllowedImageContentType(sniffedImageType(f))
			_ = f.Close()
			if !valid {
				return name, "Not a valid image"
			}
			// Same name overwrites the existing file on purpose.
			if err := saveMultipartFile(header, filepath.Join(destDir, name)); err != nil {
				return name, "Failed to save"
			}
			return name, ""
		},
	)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, model.ImagesUploadResponse{Uploaded: uploaded, Skipped: skipped})
}

// migrateAnnouncementImages performs a one-time, idempotent copy of any files in
// the legacy <webRoot>/images/announcements directory into the new
// announcements_main category directory, so previously uploaded announcement
// images remain reusable in the new Main picker. Files already present in the
// destination are skipped, so it is safe to run on every startup. Existing
// announcements keep rendering from their stored URLs regardless.
func (s *Server) migrateAnnouncementImages() {
	srcDir := filepath.Join(s.imagesRootDir(), "announcements")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return // legacy dir absent → nothing to migrate
	}
	dstDir := filepath.Join(s.imagesRootDir(), imageDirAnnouncementsMain)
	for _, e := range entries {
		if e.IsDir() || !isAllowedImageExt(strings.ToLower(filepath.Ext(e.Name()))) {
			continue
		}
		dst := filepath.Join(dstDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue // already migrated
		}
		if err := os.MkdirAll(dstDir, 0755); err != nil {
			return
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			continue
		}
		_ = os.WriteFile(dst, data, 0644)
	}
}

// seedFlourishes copies the app's built-in flourish SVGs (shipped to
// <webRoot>/images) into the Flourishes category so admins can pick them in the
// theme flourish selectors. Idempotent: a file already present in the category is
// left as-is, so it's safe to run on every startup and admins may delete the
// seeded copies. An unset theme flourish still falls back to the built-in art.
func (s *Server) seedFlourishes() {
	dstDir := filepath.Join(s.imagesRootDir(), imageDirFlourishes)
	for _, name := range []string{"corner_flourish.svg", "called_flourish.svg"} {
		dst := filepath.Join(dstDir, name)
		if _, err := os.Stat(dst); err == nil {
			continue // already seeded
		}
		data, err := os.ReadFile(filepath.Join(s.imagesRootDir(), name))
		if err != nil {
			continue // source not deployed → nothing to seed
		}
		if err := os.MkdirAll(dstDir, 0755); err != nil {
			return
		}
		_ = os.WriteFile(dst, data, 0644)
	}
}
