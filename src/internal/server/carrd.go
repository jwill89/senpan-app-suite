package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ── Carrd image hosting (System → Carrd Upload admin tab) ────────────────────
//
// Carrd "projects" are folders under <webRoot>/carrd. A project can contain
// media files (images plus audio/.mp3 and video/.mp4) directly and/or in
// arbitrarily nested sub-directories. Everything is served publicly from
// https://carrd.senpan.cafe/<folder>/<sub>/.../<file> for embedding in external
// Carrd sites. These admin-only endpoints let the admin create/delete projects,
// create/delete sub-directories, and upload/list/delete files at any path
// within a project.
//
// The directory tree is the source of truth — sub-directories are not tracked
// anywhere but on disk. Only the top-level project carries metadata: a
// human-readable Title separate from its URL folder name, persisted in a
// per-project sidecar file (carrdMetaFile) so the title survives across uploads.
// Both the project title and the project folder name must be unique.
//
// Unlike fonts, an uploaded image whose name already exists OVERWRITES the
// existing file (so a Carrd site referencing that URL picks up the new image).

// carrdMetaFile is the per-project sidecar that stores the project Title. It is
// a dotfile so the carrd vhost's .htaccess can hide it from public access.
const carrdMetaFile = ".carrd.json"

// carrdMeta is the JSON shape of the per-project metadata sidecar.
type carrdMeta struct {
	Title string `json:"title"`
}

// carrdProject is the JSON shape for one project in the listing.
type carrdProject struct {
	Title      string `json:"title"`
	Folder     string `json:"folder"`
	ImageCount int    `json:"image_count"`
	Modified   string `json:"modified"` // RFC3339 (folder mod time)
}

// carrdImage is the JSON shape for one image in a project listing.
type carrdImage struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"` // RFC3339
}

// carrdDir returns the absolute path to the carrd projects root directory.
func (s *Server) carrdDir() string {
	return filepath.Join(s.webRoot, "carrd")
}

// slugifyFolder derives a URL-safe folder name from a project title: lowercase,
// spaces/underscores become hyphens, only letters/digits/hyphens are kept, and
// runs of hyphens are collapsed and trimmed. Used both to default the folder
// from the title and to normalize an admin-supplied folder name.
func slugifyFolder(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteByte('-')
		}
	}
	// Collapse repeated hyphens and trim leading/trailing ones.
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return strings.Trim(out, "-")
}

// validCarrdFolder reports whether name is a safe, already-normalized folder
// name (lowercase letters, digits, hyphens only). Guards against path traversal
// for folder names received from the client on delete/list/upload.
func validCarrdFolder(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// cleanCarrdRelPath validates a forward-slash relative subpath within a project
// and returns it in normalized form ("" for the project root). Every segment
// must be a valid folder name, which rejects empty segments, dotfiles, and any
// "." / ".." traversal. The returned value is safe to join under a project dir.
func cleanCarrdRelPath(rel string) (string, bool) {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	if rel == "" {
		return "", true
	}
	segs := strings.Split(rel, "/")
	for _, seg := range segs {
		if !validCarrdFolder(seg) {
			return "", false
		}
	}
	return strings.Join(segs, "/"), true
}

// carrdResolve validates a project folder + relative subpath and returns the
// absolute filesystem directory they map to. The folder and every path segment
// are validated, so the result is always contained within the project.
func (s *Server) carrdResolve(folder, rel string) (abs, cleanRel string, ok bool) {
	if !validCarrdFolder(folder) {
		return "", "", false
	}
	cleanRel, ok = cleanCarrdRelPath(rel)
	if !ok {
		return "", "", false
	}
	abs = filepath.Join(s.carrdDir(), folder)
	if cleanRel != "" {
		abs = filepath.Join(abs, filepath.FromSlash(cleanRel))
	}
	return abs, cleanRel, true
}

// isAllowedCarrdFileExt reports whether ext (lowercase, with dot) is a permitted
// carrd upload type. Carrd hosts images plus audio (.mp3) and video (.mp4) for
// embedding in external Carrd sites, so it accepts a wider set than the
// image-only bookclub uploads.
func isAllowedCarrdFileExt(ext string) bool {
	switch ext {
	case ".mp3", ".mp4":
		return true
	}
	return isAllowedImageExt(ext)
}

// safeCarrdFileName validates and normalizes an uploaded/target carrd filename:
// strips any path, rejects empty/dotfile names and disallowed extensions, and
// guards against traversal. Returns the clean base name and true when valid.
func safeCarrdFileName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	name = filepath.Base(filepath.FromSlash(name))
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return "", false
	}
	if strings.ContainsAny(name, `/\`) {
		return "", false
	}
	if !isAllowedCarrdFileExt(strings.ToLower(filepath.Ext(name))) {
		return "", false
	}
	return name, true
}

// readCarrdTitle reads the project Title from a folder's sidecar metadata,
// falling back to the folder name when the sidecar is missing or unreadable.
func readCarrdTitle(folderPath, folder string) string {
	data, err := os.ReadFile(filepath.Join(folderPath, carrdMetaFile))
	if err != nil {
		return folder
	}
	var meta carrdMeta
	if err := json.Unmarshal(data, &meta); err != nil || strings.TrimSpace(meta.Title) == "" {
		return folder
	}
	return meta.Title
}

// countCarrdImages returns the total number of media files in a project folder,
// counting files in every nested sub-directory too.
func countCarrdImages(folderPath string) int {
	n := 0
	_ = filepath.WalkDir(folderPath, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && isAllowedCarrdFileExt(strings.ToLower(filepath.Ext(d.Name()))) {
			n++
		}
		return nil
	})
	return n
}

// listCarrdProjects reads the carrd root and returns its projects sorted by
// title (case-insensitive). Returns an empty slice when the root is missing.
func (s *Server) listCarrdProjects() ([]carrdProject, error) {
	root := s.carrdDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []carrdProject{}, nil
		}
		return nil, err
	}

	projects := make([]carrdProject, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		folder := e.Name()
		folderPath := filepath.Join(root, folder)
		modified := ""
		if info, err := e.Info(); err == nil {
			modified = info.ModTime().UTC().Format(time.RFC3339)
		}
		projects = append(projects, carrdProject{
			Title:      readCarrdTitle(folderPath, folder),
			Folder:     folder,
			ImageCount: countCarrdImages(folderPath),
			Modified:   modified,
		})
	}
	sort.Slice(projects, func(i, j int) bool {
		return strings.ToLower(projects[i].Title) < strings.ToLower(projects[j].Title)
	})
	return projects, nil
}

// handleCarrdProjectsList returns the carrd projects (folders under
// <webRoot>/carrd).
//
//	Endpoint:  GET /api/carrd/projects
//	Auth:      admin
//	Response:  {"projects": [{title, folder, image_count, modified}]}
func (s *Server) handleCarrdProjectsList(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	if err := os.MkdirAll(s.carrdDir(), 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access carrd directory")
		return
	}

	projects, err := s.listCarrdProjects()
	if err != nil {
		writeInternalError(w, "list carrd projects", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

// carrdProjectsActionRequest is the JSON body for POST /api/carrd/projects.
// Action: "create" or "delete".
type carrdProjectsActionRequest struct {
	Action string `json:"action"`
	Title  string `json:"title"`
	Folder string `json:"folder"`
}

// handleCarrdProjectsAction creates or deletes a carrd project (folder).
//
//	Endpoint:  POST /api/carrd/projects
//	Auth:      admin
//	Request:   {"action": "create", "title": "...", "folder": "..."(optional)}
//	           {"action": "delete", "folder": "..."}
//	Response:  create → {"ok": true, "project": {...}}
//	           delete → {"ok": true}
func (s *Server) handleCarrdProjectsAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	req, err := readJSON[carrdProjectsActionRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	root := s.carrdDir()
	if err := os.MkdirAll(root, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access carrd directory")
		return
	}

	switch req.Action {
	case "create":
		s.createCarrdProject(w, root, req)

	case "delete":
		folder := strings.TrimSpace(req.Folder)
		if !validCarrdFolder(folder) {
			writeError(w, http.StatusBadRequest, "Invalid folder name")
			return
		}
		if err := os.RemoveAll(filepath.Join(root, folder)); err != nil {
			writeInternalError(w, "delete carrd project", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, delete")
	}
}

// createCarrdProject handles the "create" action: it validates that the title
// and folder are non-empty and unique, creates the folder, and writes the
// title sidecar.
func (s *Server) createCarrdProject(w http.ResponseWriter, root string, req carrdProjectsActionRequest) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Project title is required")
		return
	}

	// Folder: use the supplied name (normalized) or derive it from the title.
	folder := slugifyFolder(req.Folder)
	if folder == "" {
		folder = slugifyFolder(title)
	}
	if folder == "" {
		writeError(w, http.StatusBadRequest, "Could not derive a folder name — use letters or numbers in the title or folder")
		return
	}

	// Enforce uniqueness of both the title and the folder across projects.
	existing, err := s.listCarrdProjects()
	if err != nil {
		writeInternalError(w, "list carrd projects", err)
		return
	}
	for _, p := range existing {
		if strings.EqualFold(p.Title, title) {
			writeError(w, http.StatusConflict, "A project titled \""+title+"\" already exists")
			return
		}
		if p.Folder == folder {
			writeError(w, http.StatusConflict, "A project folder \""+folder+"\" already exists")
			return
		}
	}

	folderPath := filepath.Join(root, folder)
	if err := os.Mkdir(folderPath, 0755); err != nil {
		if os.IsExist(err) {
			writeError(w, http.StatusConflict, "A project folder \""+folder+"\" already exists")
			return
		}
		writeInternalError(w, "create carrd project", err)
		return
	}

	meta, _ := json.Marshal(carrdMeta{Title: title})
	if err := os.WriteFile(filepath.Join(folderPath, carrdMetaFile), meta, 0644); err != nil {
		writeInternalError(w, "write carrd metadata", err)
		return
	}

	modified := ""
	if info, err := os.Stat(folderPath); err == nil {
		modified = info.ModTime().UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok": true,
		"project": carrdProject{
			Title:      title,
			Folder:     folder,
			ImageCount: 0,
			Modified:   modified,
		},
	})
}

// handleCarrdImagesList returns the sub-directories and images at a path within
// a project (the project root when no path is given).
//
//	Endpoint:  GET /api/carrd/images?folder=<folder>&path=<subpath>
//	Auth:      admin
//	Response:  {"folder": "...", "path": "...", "dirs": ["..."],
//	            "images": [{name, size, modified}]}
func (s *Server) handleCarrdImagesList(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	folder := strings.TrimSpace(r.URL.Query().Get("folder"))
	dirPath, cleanPath, ok := s.carrdResolve(folder, r.URL.Query().Get("path"))
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid folder or path")
		return
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "Folder not found")
			return
		}
		writeInternalError(w, "read carrd folder", err)
		return
	}

	dirs := make([]string, 0)
	images := make([]carrdImage, 0, len(entries))
	for _, e := range entries {
		// Hide dotfiles/dirs (e.g. the .carrd.json title sidecar).
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, e.Name())
			continue
		}
		if !isAllowedCarrdFileExt(strings.ToLower(filepath.Ext(e.Name()))) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		images = append(images, carrdImage{
			Name:     e.Name(),
			Size:     info.Size(),
			Modified: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j])
	})
	sort.Slice(images, func(i, j int) bool {
		return strings.ToLower(images[i].Name) < strings.ToLower(images[j].Name)
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"folder": folder,
		"path":   cleanPath,
		"dirs":   dirs,
		"images": images,
	})
}

// carrdImagesActionRequest is the JSON body for POST /api/carrd/images.
// Action: "delete" (image), "create_dir", or "delete_dir".
//
//	Path is the relative subpath within the project ("" = project root). For
//	"delete" and "create_dir" it is the parent directory; for "delete_dir" it is
//	the directory to remove (and must be non-empty — delete the whole project via
//	/api/carrd/projects instead). Name is the image filename ("delete") or the
//	new sub-directory name ("create_dir").
type carrdImagesActionRequest struct {
	Action string `json:"action"`
	Folder string `json:"folder"`
	Path   string `json:"path"`
	Name   string `json:"name"`
}

// handleCarrdImagesAction deletes an image, creates a sub-directory, or deletes
// a sub-directory (and its contents) within a project.
//
//	Endpoint:  POST /api/carrd/images
//	Auth:      admin
//	Request:   {"action":"delete","folder":"...","path":"...","name":"..."}
//	           {"action":"create_dir","folder":"...","path":"...","name":"..."}
//	           {"action":"delete_dir","folder":"...","path":"..."}
//	Response:  {"ok": true} (create_dir also returns {"name": newDir})
func (s *Server) handleCarrdImagesAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	req, err := readJSON[carrdImagesActionRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	dirPath, cleanPath, ok := s.carrdResolve(strings.TrimSpace(req.Folder), req.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid folder or path")
		return
	}

	switch req.Action {
	case "delete":
		name, ok := safeCarrdFileName(req.Name)
		if !ok {
			writeError(w, http.StatusBadRequest, "Invalid file name")
			return
		}
		if err := os.Remove(filepath.Join(dirPath, name)); err != nil {
			if os.IsNotExist(err) {
				writeError(w, http.StatusNotFound, "File not found")
				return
			}
			writeInternalError(w, "delete carrd file", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "create_dir":
		newDir := slugifyFolder(req.Name)
		if newDir == "" {
			writeError(w, http.StatusBadRequest, "Folder name must contain letters or numbers")
			return
		}
		// The parent (project root or sub-directory) must already exist.
		if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
			writeError(w, http.StatusNotFound, "Parent folder not found")
			return
		}
		if err := os.Mkdir(filepath.Join(dirPath, newDir), 0755); err != nil {
			if os.IsExist(err) {
				writeError(w, http.StatusConflict, "A folder named \""+newDir+"\" already exists here")
				return
			}
			writeInternalError(w, "create carrd dir", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": newDir})

	case "delete_dir":
		// Refuse to delete the project root through this endpoint.
		if cleanPath == "" {
			writeError(w, http.StatusBadRequest, "Use the projects endpoint to delete a whole project")
			return
		}
		if err := os.RemoveAll(dirPath); err != nil {
			writeInternalError(w, "delete carrd dir", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: delete, create_dir, delete_dir")
	}
}

// handleCarrdUpload handles multipart uploads of one or more images to a
// directory within a project (the project root, or a sub-directory given by the
// "path" field). An image whose name already exists OVERWRITES the existing
// file. Each file is processed independently; the response reports which
// succeeded and which were skipped, so a partial batch still uploads the valid
// files.
//
//	Endpoint:  POST /api/carrd/upload
//	Auth:      admin
//	Request:   multipart form with "folder" + optional "path" fields and one or
//	           more "files" fields
//	Response:  {"uploaded": ["a.png"], "skipped": [{"name":"b.txt","reason":"..."}]}
func (s *Server) handleCarrdUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	// Cap the whole request at 64 MB (several images at once).
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "Upload failed (max 64MB total)")
		return
	}

	folderPath, _, ok := s.carrdResolve(strings.TrimSpace(r.FormValue("folder")), r.FormValue("path"))
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid folder or path")
		return
	}
	// The target directory must already exist (the project and any sub-dirs are
	// created via their own endpoints).
	if info, err := os.Stat(folderPath); err != nil || !info.IsDir() {
		writeError(w, http.StatusNotFound, "Folder not found")
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
		name, ok := safeCarrdFileName(header.Filename)
		if !ok {
			skipped = append(skipped, skipEntry{
				Name:   header.Filename,
				Reason: "Unsupported type (allowed: .jpg, .jpeg, .png, .webp, .gif, .mp3, .mp4)",
			})
			continue
		}
		// Same name overwrites the existing file on purpose.
		if err := saveUploadedFont(header, filepath.Join(folderPath, name)); err != nil {
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

// (saveUploadedFont in fonts.go streams a multipart file part to a destination
// path; it is filename-agnostic, so we reuse it here for image uploads.)
