package server

import (
	"encoding/json"
	"io"
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
	imageDirFlourishes         = "flourishes"
)

// imageCategory is the JSON shape for one category in the listing. FileCount and
// TotalSize are populated for listings and omitted (zero) for action responses.
type imageCategory struct {
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	Permanent bool   `json:"permanent"`
	FileCount int    `json:"file_count"`
	TotalSize int64  `json:"total_size"`
}

// imageEntry is the JSON shape for one image in a category listing. Url is the
// absolute public URL (Discord embeds require absolute URLs); Path is the
// root-relative web path (raffles store the relative path in prize_image).
type imageEntry struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"` // RFC3339
}

// permanentImageCategories returns the fixed categories, in a stable order.
func permanentImageCategories() []imageCategory {
	return []imageCategory{
		{Name: "Announcement Main", Dir: imageDirAnnouncementsMain, Permanent: true},
		{Name: "Announcement Thumbnail", Dir: imageDirAnnouncementsThumb, Permanent: true},
		{Name: "Raffle", Dir: imageDirRaffles, Permanent: true},
		{Name: "Flourishes", Dir: imageDirFlourishes, Permanent: true},
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
func slugifyImageDir(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteByte('_')
		}
	}
	out := b.String()
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return strings.Trim(out, "_")
}

// validImageDir reports whether dir is a safe, already-normalized directory name
// (lowercase letters, digits, underscores only — no path separators). Guards
// against traversal for dir names received from the client on list/upload/delete.
func validImageDir(dir string) bool {
	if dir == "" {
		return false
	}
	for _, r := range dir {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// imageManifest is the JSON shape of the custom-categories manifest dotfile.
type imageManifest struct {
	Categories []imageCategory `json:"categories"`
}

// readImageCategories reads the custom-category manifest. Returns an empty slice
// when the manifest is missing or unreadable (it isn't created until the first
// custom category is added).
func (s *Server) readImageCategories() []imageCategory {
	data, err := os.ReadFile(filepath.Join(s.imagesRootDir(), imageCategoriesManifest))
	if err != nil {
		return []imageCategory{}
	}
	var m imageManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return []imageCategory{}
	}
	// Defensive: drop any entry that collides with a permanent dir or is invalid.
	out := make([]imageCategory, 0, len(m.Categories))
	for _, c := range m.Categories {
		if validImageDir(c.Dir) && !isPermanentImageDir(c.Dir) {
			c.Permanent = false
			out = append(out, c)
		}
	}
	return out
}

// writeImageCategories persists the custom categories to the manifest dotfile.
func (s *Server) writeImageCategories(cats []imageCategory) error {
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
func (s *Server) allImageCategories() []imageCategory {
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
	case imageDirFlourishes:
		return userHasPermission(u, permSystemThemes)
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
	writeJSON(w, http.StatusOK, map[string]any{"categories": s.allImageCategories()})
}

// imageCategoriesActionRequest is the JSON body for POST /api/image-categories.
// Action: "create", "rename", or "delete". For "rename", Dir is the existing
// directory and NewDir is the desired one ("" keeps the current directory).
type imageCategoriesActionRequest struct {
	Action string `json:"action"`
	Name   string `json:"name"`
	Dir    string `json:"dir"`
	NewDir string `json:"new_dir"`
}

// handleImageCategoriesAction creates, renames, or deletes a custom category.
//
//	Endpoint:  POST /api/image-categories
//	Auth:      admin, or a user granted system-images
//	Request:   {"action":"create","name":"...","dir":"..."(optional)}
//	           {"action":"rename","dir":"...","name":"...","new_dir":"..."(optional)}
//	           {"action":"delete","dir":"..."}
//	Response:  create/rename → {"ok": true, "category": {...}}
//	           delete        → {"ok": true}
func (s *Server) handleImageCategoriesAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemImages) {
		return
	}
	req, err := readJSON[imageCategoriesActionRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if err := os.MkdirAll(s.imagesRootDir(), 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access images directory")
		return
	}

	switch req.Action {
	case "create":
		s.createImageCategory(w, req)
	case "rename":
		s.renameImageCategory(w, req)
	case "delete":
		s.deleteImageCategory(w, req)
	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, rename, delete")
	}
}

// createImageCategory validates the name + directory, ensures the directory does
// not collide with a permanent or existing custom one, creates the folder, and
// records it in the manifest.
func (s *Server) createImageCategory(w http.ResponseWriter, req imageCategoriesActionRequest) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Category name is required")
		return
	}
	// Directory: use the supplied name (normalized) or derive it from the name.
	dir := slugifyImageDir(req.Dir)
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
	cat := imageCategory{Name: name, Dir: dir, Permanent: false}
	if err := s.writeImageCategories(append(cats, cat)); err != nil {
		writeInternalError(w, "write image categories", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "category": cat})
}

// renameImageCategory updates a custom category's display name and, when the
// directory changes, moves the folder on disk. Permanent categories are rejected.
func (s *Server) renameImageCategory(w http.ResponseWriter, req imageCategoriesActionRequest) {
	dir := strings.TrimSpace(req.Dir)
	if !validImageDir(dir) {
		writeError(w, http.StatusBadRequest, "Invalid directory name")
		return
	}
	if isPermanentImageDir(dir) {
		writeError(w, http.StatusForbidden, "Permanent categories cannot be modified")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Category name is required")
		return
	}
	newDir := slugifyImageDir(req.NewDir)
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

	cats[idx] = imageCategory{Name: name, Dir: newDir, Permanent: false}
	if err := s.writeImageCategories(cats); err != nil {
		writeInternalError(w, "write image categories", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "category": cats[idx]})
}

// deleteImageCategory removes a custom category's folder (and all its files) and
// its manifest entry. Permanent categories cannot be deleted.
func (s *Server) deleteImageCategory(w http.ResponseWriter, req imageCategoriesActionRequest) {
	dir := strings.TrimSpace(req.Dir)
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
		writeJSON(w, http.StatusOK, map[string]any{"dir": dir, "images": []imageEntry{}})
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
	images := make([]imageEntry, 0, len(infos))
	for _, info := range infos {
		rel := "images/" + dir + "/" + info.name
		images = append(images, imageEntry{
			Name:     info.name,
			URL:      base + "/" + rel,
			Path:     rel,
			Size:     info.size,
			Modified: info.mod.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"dir": dir, "images": images})
}

// imagesActionRequest is the JSON body for POST /api/images (image delete).
type imagesActionRequest struct {
	Action string `json:"action"`
	Dir    string `json:"dir"`
	Name   string `json:"name"`
}

// handleImagesAction deletes a single image within a category.
//
//	Endpoint:  POST /api/images
//	Auth:      admin, or a user granted system-images
//	Request:   {"action":"delete","dir":"...","name":"..."}
//	Response:  {"ok": true}
func (s *Server) handleImagesAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemImages) {
		return
	}
	req, err := readJSON[imagesActionRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Action != "delete" {
		writeError(w, http.StatusBadRequest, "Invalid action. Use: delete")
		return
	}
	dir := strings.TrimSpace(req.Dir)
	if !validImageDir(dir) || !s.imageDirIsKnown(dir) {
		writeError(w, http.StatusBadRequest, "Unknown image category")
		return
	}
	name, ok := safeImageFileName(req.Name)
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// safeImageFileName validates and normalizes an uploaded/target image filename:
// strips any path, rejects empty/dotfile names and disallowed extensions, and
// guards against traversal. Returns the clean base name and true when valid.
func safeImageFileName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	name = filepath.Base(filepath.FromSlash(name))
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return "", false
	}
	if strings.ContainsAny(name, `/\`) {
		return "", false
	}
	if !isAllowedImagesExt(strings.ToLower(filepath.Ext(name))) {
		return "", false
	}
	return name, true
}

// looksLikeSVG reports whether r begins with SVG markup (an "<svg" root within the
// first bytes). SVG is XML/text, so the raster content-sniff used for other images
// would reject it; this is the SVG counterpart (defense in depth alongside the
// .svg extension check). Rewinds r so it can still be read in full afterwards.
func looksLikeSVG(r io.ReadSeeker) bool {
	head := make([]byte, 1024)
	n, _ := io.ReadFull(r, head)
	_, _ = r.Seek(0, io.SeekStart)
	return strings.Contains(strings.ToLower(string(head[:n])), "<svg")
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

	// Cap the whole request at 64 MB (several images at once).
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "Upload failed (max 64MB total)")
		return
	}

	dir := strings.TrimSpace(r.FormValue("dir"))
	if !validImageDir(dir) || !s.imageDirIsKnown(dir) {
		writeError(w, http.StatusBadRequest, "Unknown image category")
		return
	}
	destDir := filepath.Join(s.imagesRootDir(), dir)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create category directory")
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "No files provided")
		return
	}

	type skipEntry struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	}
	uploaded := make([]string, 0, len(files))
	skipped := make([]skipEntry, 0)

	for _, header := range files {
		name, ok := safeImageFileName(header.Filename)
		if !ok {
			skipped = append(skipped, skipEntry{
				Name:   header.Filename,
				Reason: "Unsupported type (allowed: .jpg, .jpeg, .png, .webp, .gif, .svg)",
			})
			continue
		}
		// Defense in depth: confirm the bytes match the extension. SVG is XML/text
		// (the raster content-sniff would reject it), so it's validated separately.
		f, err := header.Open()
		if err != nil {
			skipped = append(skipped, skipEntry{Name: name, Reason: "Failed to read"})
			continue
		}
		isSVG := strings.EqualFold(filepath.Ext(name), ".svg")
		var valid bool
		if isSVG {
			valid = looksLikeSVG(f)
		} else {
			valid = isAllowedImageContentType(sniffedImageType(f))
		}
		_ = f.Close()
		if !valid {
			reason := "Not a valid image"
			if isSVG {
				reason = "Not a valid SVG"
			}
			skipped = append(skipped, skipEntry{Name: name, Reason: reason})
			continue
		}
		// Same name overwrites the existing file on purpose.
		if err := saveMultipartFile(header, filepath.Join(destDir, name)); err != nil {
			skipped = append(skipped, skipEntry{Name: name, Reason: "Failed to save"})
			continue
		}
		uploaded = append(uploaded, name)
	}

	writeJSON(w, http.StatusOK, map[string]any{"uploaded": uploaded, "skipped": skipped})
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
