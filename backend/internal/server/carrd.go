package server

import (
	"encoding/json"
	"io/fs"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"app-suite/internal/model"
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

// carrdDir returns the absolute path to the carrd projects root directory.
func (s *Server) carrdDir() string {
	return filepath.Join(s.webRoot, "carrd")
}

// slugifyFolder derives a URL-safe folder name from a project title: lowercase,
// spaces/underscores become hyphens, only letters/digits/hyphens are kept, and
// runs of hyphens are collapsed and trimmed. Used both to default the folder
// from the title and to normalize an admin-supplied folder name.
func slugifyFolder(s string) string { return slugify(s, '-') }

// validCarrdFolder reports whether name is a safe, already-normalized folder
// name (lowercase letters, digits, hyphens only). Guards against path traversal
// for folder names received from the client on delete/list/upload.
func validCarrdFolder(name string) bool { return validSlug(name, '-') }

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
	return safeUploadName(name, isAllowedCarrdFileExt)
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

// carrdProjectStats walks a project folder and returns, across the whole tree,
// the number of nested sub-directories, the number of media files, and their
// combined size in bytes. The project folder itself is not counted as a
// sub-directory, and dotfiles (e.g. the .carrd.json title sidecar) are ignored.
func carrdProjectStats(folderPath string) (subfolders, files int, totalSize int64) {
	_ = filepath.WalkDir(folderPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != folderPath {
				subfolders++
			}
			return nil
		}
		if isAllowedCarrdFileExt(strings.ToLower(filepath.Ext(d.Name()))) {
			files++
			if info, err := d.Info(); err == nil {
				totalSize += info.Size()
			}
		}
		return nil
	})
	return
}

// listCarrdProjects reads the carrd root and returns its projects sorted by
// title (case-insensitive). Returns an empty slice when the root is missing.
func (s *Server) listCarrdProjects() ([]model.CarrdProject, error) {
	root := s.carrdDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []model.CarrdProject{}, nil
		}
		return nil, err
	}

	projects := make([]model.CarrdProject, 0, len(entries))
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
		subfolders, files, totalSize := carrdProjectStats(folderPath)
		projects = append(projects, model.CarrdProject{
			Title:          readCarrdTitle(folderPath, folder),
			Folder:         folder,
			FileCount:      files,
			SubfolderCount: subfolders,
			TotalSize:      totalSize,
			Modified:       modified,
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
//	Auth:      admin, or a user granted this page's permission
//	Response:  {"projects": [{title, folder, file_count, subfolder_count,
//	            total_size, modified}]}
func (s *Server) handleCarrdProjectsList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierCarrd) {
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
	writeJSON(w, http.StatusOK, model.CarrdProjectsResponse{Projects: projects})
}

// carrdProjectCreateRequest is the JSON body for POST /api/carrd/projects.
type carrdProjectCreateRequest struct {
	Title  string `json:"title"`
	Folder string `json:"folder"`
}

// carrdProjectRenameRequest is the JSON body for PATCH /api/carrd/projects/{folder}.
// The existing folder comes from the path; NewFolder is the desired folder
// ("" keeps the current one).
type carrdProjectRenameRequest struct {
	Title     string `json:"title"`
	NewFolder string `json:"new_folder"`
}

// handleCarrdProjectCreate creates a carrd project (folder).
//
//	Endpoint:  POST /api/carrd/projects
//	Auth:      admin, or a user granted this page's permission
//	Request:   {"title": "...", "folder": "..."(optional)}
//	Response:  201 {"ok": true, "project": {...}}
func (s *Server) handleCarrdProjectCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierCarrd) {
		return
	}

	req, err := readJSON[carrdProjectCreateRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	root := s.carrdDir()
	if err := os.MkdirAll(root, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access carrd directory")
		return
	}
	s.createCarrdProject(w, root, req.Title, req.Folder)
}

// handleCarrdProjectRename renames a carrd project. The current folder comes from
// the path; the new title/folder from the body.
//
//	Endpoint:  PATCH /api/carrd/projects/{folder}
//	Auth:      admin, or a user granted this page's permission
//	Request:   {"title": "...", "new_folder": "..."(optional)}
//	Response:  200 {"ok": true, "project": {...}}
func (s *Server) handleCarrdProjectRename(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierCarrd) {
		return
	}

	req, err := readJSON[carrdProjectRenameRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	root := s.carrdDir()
	if err := os.MkdirAll(root, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access carrd directory")
		return
	}
	s.renameCarrdProject(w, root, r.PathValue("folder"), req.Title, req.NewFolder)
}

// handleCarrdProjectDelete deletes a carrd project (folder) and its contents.
//
//	Endpoint:  DELETE /api/carrd/projects/{folder}
//	Auth:      admin, or a user granted this page's permission
//	Response:  204 No Content
func (s *Server) handleCarrdProjectDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierCarrd) {
		return
	}

	root := s.carrdDir()
	if err := os.MkdirAll(root, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access carrd directory")
		return
	}
	folder := strings.TrimSpace(r.PathValue("folder"))
	if !validCarrdFolder(folder) {
		writeError(w, http.StatusBadRequest, "Invalid folder name")
		return
	}
	if err := os.RemoveAll(filepath.Join(root, folder)); err != nil {
		writeInternalError(w, "delete carrd project", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// createCarrdProject validates that the title and folder are non-empty and
// unique, creates the folder, and writes the title sidecar. reqTitle/reqFolder
// are the raw (untrimmed/unnormalized) values from the request.
func (s *Server) createCarrdProject(w http.ResponseWriter, root, reqTitle, reqFolder string) {
	title := strings.TrimSpace(reqTitle)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Project title is required")
		return
	}

	// Folder: use the supplied name (normalized) or derive it from the title.
	folder := slugifyFolder(reqFolder)
	if folder == "" {
		folder = slugifyFolder(title)
	}
	if folder == "" {
		writeError(w, http.StatusBadRequest, "Could not derive a folder name — use letters or numbers in the title or folder")
		return
	}

	// Enforce uniqueness of both the title and the folder across projects. Read
	// the root entries directly and only the title sidecars — listCarrdProjects
	// would additionally walk every project's whole tree counting images, which
	// is wasted work just to reject a duplicate name.
	entries, err := os.ReadDir(root)
	if err != nil {
		writeInternalError(w, "list carrd projects", err)
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == folder {
			writeError(w, http.StatusConflict, "A project folder \""+folder+"\" already exists")
			return
		}
		if strings.EqualFold(readCarrdTitle(filepath.Join(root, e.Name()), e.Name()), title) {
			writeError(w, http.StatusConflict, "A project titled \""+title+"\" already exists")
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
	writeJSON(w, http.StatusCreated, model.CarrdProjectCreateResponse{
		OK: true,
		Project: model.CarrdProject{
			Title:    title,
			Folder:   folder,
			Modified: modified,
		},
	})
}

// renameCarrdProject updates a project's title sidecar and, when new_folder
// differs, renames the project folder on disk. Both the new title and the new
// folder must stay unique across projects. reqFolder is the existing folder
// (from the path); reqTitle/reqNewFolder are the raw request values.
func (s *Server) renameCarrdProject(w http.ResponseWriter, root, reqFolder, reqTitle, reqNewFolder string) {
	folder := strings.TrimSpace(reqFolder)
	if !validCarrdFolder(folder) {
		writeError(w, http.StatusBadRequest, "Invalid folder name")
		return
	}
	title := strings.TrimSpace(reqTitle)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Project title is required")
		return
	}
	// Target folder: normalize the supplied new folder, or keep the current one.
	newFolder := slugifyFolder(reqNewFolder)
	if newFolder == "" {
		newFolder = folder
	}

	srcPath := filepath.Join(root, folder)
	if info, err := os.Stat(srcPath); err != nil || !info.IsDir() {
		writeError(w, http.StatusNotFound, "Project not found")
		return
	}

	// Enforce uniqueness of both the new title and the new folder against the
	// OTHER projects (a project may keep its own title/folder unchanged).
	entries, err := os.ReadDir(root)
	if err != nil {
		writeInternalError(w, "list carrd projects", err)
		return
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == folder {
			continue
		}
		if e.Name() == newFolder {
			writeError(w, http.StatusConflict, "A project folder \""+newFolder+"\" already exists")
			return
		}
		if strings.EqualFold(readCarrdTitle(filepath.Join(root, e.Name()), e.Name()), title) {
			writeError(w, http.StatusConflict, "A project titled \""+title+"\" already exists")
			return
		}
	}

	// Rename the folder on disk when it changed.
	destPath := srcPath
	if newFolder != folder {
		destPath = filepath.Join(root, newFolder)
		if err := os.Rename(srcPath, destPath); err != nil {
			if os.IsExist(err) {
				writeError(w, http.StatusConflict, "A project folder \""+newFolder+"\" already exists")
				return
			}
			writeInternalError(w, "rename carrd project", err)
			return
		}
	}

	// Update the title sidecar in the (possibly moved) folder.
	meta, _ := json.Marshal(carrdMeta{Title: title})
	if err := os.WriteFile(filepath.Join(destPath, carrdMetaFile), meta, 0644); err != nil {
		writeInternalError(w, "write carrd metadata", err)
		return
	}

	subfolders, files, totalSize := carrdProjectStats(destPath)
	modified := ""
	if info, err := os.Stat(destPath); err == nil {
		modified = info.ModTime().UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, model.CarrdProjectCreateResponse{
		OK: true,
		Project: model.CarrdProject{
			Title:          title,
			Folder:         newFolder,
			FileCount:      files,
			SubfolderCount: subfolders,
			TotalSize:      totalSize,
			Modified:       modified,
		},
	})
}

// handleCarrdImagesList returns the sub-directories and images at a path within
// a project (the project root when no path is given).
//
//	Endpoint:  GET /api/carrd/images?folder=<folder>&path=<subpath>
//	Auth:      admin, or a user granted this page's permission
//	Response:  {"folder": "...", "path": "...", "dirs": ["..."],
//	            "images": [{name, size, modified}]}
func (s *Server) handleCarrdImagesList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierCarrd) {
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
	images := make([]model.CarrdImage, 0, len(entries))
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
		images = append(images, model.CarrdImage{
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

	writeJSON(w, http.StatusOK, model.CarrdImagesResponse{
		Folder: folder,
		Path:   cleanPath,
		Dirs:   dirs,
		Images: images,
	})
}

// carrdCreateDirRequest is the JSON body for POST /api/carrd/images/dirs. Path is
// the relative subpath of the parent directory within the project ("" = project
// root); Name is the new sub-directory name.
type carrdCreateDirRequest struct {
	Folder string `json:"folder"`
	Path   string `json:"path"`
	Name   string `json:"name"`
}

// handleCarrdImageDelete deletes an image within a project. The image identity
// (folder + path + name) is supplied as query parameters, since the path may
// contain slashes.
//
//	Endpoint:  DELETE /api/carrd/images?folder=<folder>&path=<subpath>&name=<file>
//	Auth:      admin, or a user granted this page's permission
//	Response:  204 No Content
func (s *Server) handleCarrdImageDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierCarrd) {
		return
	}

	dirPath, _, ok := s.carrdResolve(strings.TrimSpace(r.URL.Query().Get("folder")), r.URL.Query().Get("path"))
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid folder or path")
		return
	}
	name, ok := safeCarrdFileName(r.URL.Query().Get("name"))
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
	w.WriteHeader(http.StatusNoContent)
}

// handleCarrdDirCreate creates a sub-directory within a project. Path is the
// parent directory (project root when "").
//
//	Endpoint:  POST /api/carrd/images/dirs
//	Auth:      admin, or a user granted this page's permission
//	Request:   {"folder":"...","path":"...","name":"..."}
//	Response:  201 {"ok": true, "name": newDir}
func (s *Server) handleCarrdDirCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierCarrd) {
		return
	}

	req, err := readJSON[carrdCreateDirRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	dirPath, _, ok := s.carrdResolve(strings.TrimSpace(req.Folder), req.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid folder or path")
		return
	}

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
	writeJSON(w, http.StatusCreated, model.NamedOKResponse{OK: true, Name: newDir})
}

// handleCarrdDirDelete deletes a sub-directory (and its contents) within a
// project. The directory identity (folder + path) is supplied as query
// parameters, since the path may contain slashes. The project root cannot be
// deleted here — use the projects endpoint instead.
//
//	Endpoint:  DELETE /api/carrd/images/dirs?folder=<folder>&path=<subpath>
//	Auth:      admin, or a user granted this page's permission
//	Response:  204 No Content
func (s *Server) handleCarrdDirDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierCarrd) {
		return
	}

	dirPath, cleanPath, ok := s.carrdResolve(strings.TrimSpace(r.URL.Query().Get("folder")), r.URL.Query().Get("path"))
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid folder or path")
		return
	}
	// Refuse to delete the project root through this endpoint.
	if cleanPath == "" {
		writeError(w, http.StatusBadRequest, "Use the projects endpoint to delete a whole project")
		return
	}
	if err := os.RemoveAll(dirPath); err != nil {
		writeInternalError(w, "delete carrd dir", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCarrdUpload handles multipart uploads of one or more images to a
// directory within a project (the project root, or a sub-directory given by the
// "path" field). An image whose name already exists OVERWRITES the existing
// file. Each file is processed independently; the response reports which
// succeeded and which were skipped, so a partial batch still uploads the valid
// files.
//
//	Endpoint:  POST /api/carrd/upload
//	Auth:      admin, or a user granted this page's permission
//	Request:   multipart form with "folder" + optional "path" fields and one or
//	           more "files" fields
//	Response:  {"uploaded": ["a.png"], "skipped": [{"name":"b.txt","reason":"..."}]}
func (s *Server) handleCarrdUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permAtelierCarrd) {
		return
	}
	uploaded, skipped, ok := s.handleMultipartUploads(w, r,
		func() (string, bool) {
			folderPath, _, resolved := s.carrdResolve(strings.TrimSpace(r.FormValue("folder")), r.FormValue("path"))
			if !resolved {
				writeError(w, http.StatusBadRequest, "Invalid folder or path")
				return "", false
			}
			// The target directory must already exist (the project and any
			// sub-dirs are created via their own endpoints).
			if info, err := os.Stat(folderPath); err != nil || !info.IsDir() {
				writeError(w, http.StatusNotFound, "Folder not found")
				return "", false
			}
			return folderPath, true
		},
		func(header *multipart.FileHeader, destDir string) (string, string) {
			name, ok := safeCarrdFileName(header.Filename)
			if !ok {
				return header.Filename, "Unsupported type (allowed: .jpg, .jpeg, .png, .webp, .gif, .mp3, .mp4)"
			}
			// For image extensions, confirm the bytes are actually an image
			// (defense in depth). mp3/mp4 sniff unreliably, so they stay
			// extension-validated.
			if isAllowedImageExt(strings.ToLower(filepath.Ext(name))) {
				f, err := header.Open()
				if err != nil {
					return name, "Failed to read"
				}
				validImage := isAllowedImageContentType(sniffedImageType(f))
				_ = f.Close()
				if !validImage {
					return name, "Not a valid image"
				}
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
	writeJSON(w, http.StatusOK, model.CarrdUploadResponse{Uploaded: uploaded, Skipped: skipped})
}

// (saveMultipartFile lives in uploads.go; it streams a multipart file part to a
// destination path and is filename-agnostic, so it is reused here for uploads.)
