package media

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"app-suite/internal/uploads"
)

// A Carrd project is a folder under <webRoot>/carrd holding media files (pictures,
// .mp3 and .mp4) directly or in sub-folders of any depth, served publicly from the
// carrd host for embedding in outside sites. The folder tree is the only record of
// what is inside; only the project itself carries data, a title kept in the dotfile
// sidecar .carrd.json so it survives uploads. Both a project's folder and its title
// are unique. A file uploaded under a name already there replaces it, so a site
// referencing its address picks up the new one.

const carrdMetaFile = ".carrd.json"

type carrdMeta struct {
	Title string `json:"title"`
}

// Project is a Carrd project with counts across its whole tree.
type Project struct {
	Title          string
	Folder         string
	FileCount      int
	SubfolderCount int
	TotalSize      int64
	Modified       time.Time
}

// File is a media file in a project folder.
type File struct {
	Name     string
	Size     int64
	Modified time.Time
}

// Listing is what one folder of a project holds.
type Listing struct {
	// Folder is the project's folder and Path the folder within it ("" for its root).
	Folder, Path string
	// Folders are the sub-folder names, files the media files, each sorted by name.
	Folders []string
	Files   []File
}

// Carrd is the Carrd projects under a web root.
type Carrd struct{ root string }

// NewCarrd is the Carrd projects under webRoot.
func NewCarrd(webRoot string) *Carrd { return &Carrd{root: webRoot} }

func (c *Carrd) dir() string { return filepath.Join(c.root, "carrd") }

const (
	msgNoSuchProject = "There is no such project."
	msgNoSuchFolder  = "There is no such folder."
)

// ValidFolder reports whether name is a normalized project or sub-folder name:
// lowercase letters, digits and hyphens, which can't hold a separator or a traversal.
func ValidFolder(name string) bool { return uploads.ValidSlug(name, '-') }

// CleanPath validates a slash-separated path within a project and returns it
// normalized, "" for the project's root. Each segment must be a valid folder name,
// which refuses empty segments, dotfiles and any "." or "..".
func CleanPath(rel string) (string, bool) {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	if rel == "" {
		return "", true
	}
	segs := strings.Split(rel, "/")
	for _, seg := range segs {
		if !ValidFolder(seg) {
			return "", false
		}
	}
	return strings.Join(segs, "/"), true
}

// resolve maps a project and a path within it to a folder on disk. Both are validated,
// so the result is always inside the project. A folder name that can't be a project is
// not found; a bad path is invalid.
func (c *Carrd) resolve(folder, rel string) (abs, cleanRel string, err error) {
	if !ValidFolder(folder) {
		return "", "", notFound(msgNoSuchProject)
	}
	cleanRel, ok := CleanPath(rel)
	if !ok {
		return "", "", invalid("path", "That isn't a valid folder path.")
	}
	abs = filepath.Join(c.dir(), folder)
	if cleanRel != "" {
		abs = filepath.Join(abs, filepath.FromSlash(cleanRel))
	}
	return abs, cleanRel, nil
}

func readTitle(folderPath, folder string) string {
	data, err := os.ReadFile(filepath.Join(folderPath, carrdMetaFile)) //nolint:gosec // a project folder under the carrd root, and a fixed file name
	if err != nil {
		return folder
	}
	var meta carrdMeta
	if err := json.Unmarshal(data, &meta); err != nil || strings.TrimSpace(meta.Title) == "" {
		return folder
	}
	return meta.Title
}

// stats walks a project and counts, across its whole tree, the sub-folders, the media
// files and their size. Dotfiles are ignored.
func stats(folderPath string) (subfolders, files int, total int64) {
	_ = filepath.WalkDir(folderPath, func(path string, d fs.DirEntry, err error) error { //nolint:gosec // a project folder under the carrd root: a listed entry or a checked name
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is skipped: the stats are a best-effort summary
		}
		if d.IsDir() {
			if path != folderPath {
				subfolders++
			}
			return nil
		}
		if uploads.IsCarrdExt(strings.ToLower(filepath.Ext(d.Name()))) {
			files++
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return
}

func (c *Carrd) project(folder string) Project {
	path := filepath.Join(c.dir(), folder)
	p := Project{Title: readTitle(path, folder), Folder: folder}
	p.SubfolderCount, p.FileCount, p.TotalSize = stats(path)
	if info, err := os.Stat(path); err == nil { //nolint:gosec // a folder under the carrd root named by a checked name
		p.Modified = info.ModTime()
	}
	return p
}

// Projects lists the projects by title, ignoring case.
func (c *Carrd) Projects() ([]Project, error) {
	if err := os.MkdirAll(c.dir(), 0o755); err != nil { //nolint:gosec // a public folder, served by the web server
		return nil, err
	}
	entries, err := os.ReadDir(c.dir())
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, c.project(e.Name()))
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title) })
	return out, nil
}

// taken checks a title and a folder against the other projects (the one named except
// is skipped).
func (c *Carrd) taken(title, folder, except string) error {
	entries, err := os.ReadDir(c.dir())
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == except {
			continue
		}
		if e.Name() == folder {
			return conflict("A project folder \"" + folder + "\" already exists")
		}
		if strings.EqualFold(readTitle(filepath.Join(c.dir(), e.Name()), e.Name()), title) {
			return conflict("A project titled \"" + title + "\" already exists")
		}
	}
	return nil
}

func writeMeta(folderPath, title string) error {
	meta, _ := json.Marshal(carrdMeta{Title: title})
	return os.WriteFile(filepath.Join(folderPath, carrdMetaFile), meta, 0o644) //nolint:gosec // a checked project folder and a fixed file name
}

// CreateProject makes a project. The folder is the one given normalized, or derived
// from the title.
func (c *Carrd) CreateProject(reqTitle, reqFolder string) (Project, error) {
	title := strings.TrimSpace(reqTitle)
	if title == "" {
		return Project{}, invalid("title", "Project title is required")
	}
	folder := uploads.Slugify(reqFolder, '-')
	if folder == "" {
		folder = uploads.Slugify(title, '-')
	}
	if folder == "" {
		return Project{}, invalid("title", "Could not derive a folder name - use letters or numbers in the title or folder")
	}
	if err := os.MkdirAll(c.dir(), 0o755); err != nil { //nolint:gosec // a public folder, served by the web server
		return Project{}, err
	}
	if err := c.taken(title, folder, ""); err != nil {
		return Project{}, err
	}
	path := filepath.Join(c.dir(), folder)
	if err := os.Mkdir(path, 0o755); err != nil { //nolint:gosec // folder is Slugify's output
		if os.IsExist(err) {
			return Project{}, conflict("A project folder \"" + folder + "\" already exists")
		}
		return Project{}, err
	}
	if err := writeMeta(path, title); err != nil {
		return Project{}, err
	}
	return c.project(folder), nil
}

// UpdateProject changes a project's title and/or folder. A nil or blank title or
// newFolder keeps what it was.
func (c *Carrd) UpdateProject(folder string, reqTitle, reqNewFolder *string) (Project, error) {
	if !ValidFolder(folder) {
		return Project{}, notFound(msgNoSuchProject)
	}
	src := filepath.Join(c.dir(), folder)
	if info, err := os.Stat(src); err != nil || !info.IsDir() { //nolint:gosec // folder passed ValidFolder
		return Project{}, notFound(msgNoSuchProject)
	}
	title := readTitle(src, folder)
	if reqTitle != nil && strings.TrimSpace(*reqTitle) != "" {
		title = strings.TrimSpace(*reqTitle)
	}
	newFolder := folder
	if reqNewFolder != nil && strings.TrimSpace(*reqNewFolder) != "" {
		if newFolder = uploads.Slugify(*reqNewFolder, '-'); newFolder == "" {
			return Project{}, invalid("folder", "Could not derive a folder name")
		}
	}
	if err := c.taken(title, newFolder, folder); err != nil {
		return Project{}, err
	}
	dest := src
	if newFolder != folder {
		dest = filepath.Join(c.dir(), newFolder)
		if err := os.Rename(src, dest); err != nil { //nolint:gosec // folder passed ValidFolder and newFolder is Slugify's output
			if os.IsExist(err) {
				return Project{}, conflict("A project folder \"" + newFolder + "\" already exists")
			}
			return Project{}, err
		}
	}
	if err := writeMeta(dest, title); err != nil {
		return Project{}, err
	}
	return c.project(newFolder), nil
}

// DeleteProject removes a project and everything in it.
func (c *Carrd) DeleteProject(folder string) error {
	if !ValidFolder(folder) {
		return notFound(msgNoSuchProject)
	}
	path := filepath.Join(c.dir(), folder)
	if info, err := os.Stat(path); err != nil || !info.IsDir() { //nolint:gosec // folder passed ValidFolder
		return notFound(msgNoSuchProject)
	}
	return os.RemoveAll(path) //nolint:gosec // folder passed ValidFolder
}

// Folder lists a folder of a project, its root when rel is "".
func (c *Carrd) Folder(folder, rel string) (Listing, error) {
	dir, clean, err := c.resolve(folder, rel)
	if err != nil {
		return Listing{}, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Listing{}, notFound(msgNoSuchFolder)
		}
		return Listing{}, err
	}
	out := Listing{Folder: folder, Path: clean, Folders: []string{}, Files: []File{}}
	for _, e := range entries {
		// Dotfiles, the title sidecar among them, stay hidden.
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			out.Folders = append(out.Folders, e.Name())
			continue
		}
		if !uploads.IsCarrdExt(strings.ToLower(filepath.Ext(e.Name()))) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out.Files = append(out.Files, File{Name: e.Name(), Size: info.Size(), Modified: info.ModTime()})
	}
	sort.Slice(out.Folders, func(i, j int) bool { return strings.ToLower(out.Folders[i]) < strings.ToLower(out.Folders[j]) })
	sort.Slice(out.Files, func(i, j int) bool { return strings.ToLower(out.Files[i].Name) < strings.ToLower(out.Files[j].Name) })
	return out, nil
}

// Dest is the folder of a project that files are saved into: it must already exist,
// since projects and sub-folders are made by their own operations.
func (c *Carrd) Dest(folder, rel string) (string, error) {
	dir, _, err := c.resolve(folder, rel)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() { //nolint:gosec // dir comes from resolve
		return "", notFound(msgNoSuchFolder)
	}
	return dir, nil
}

// Save stores a file in dest (from Dest) under name, replacing one of that name. The
// name must already be a safe Carrd name (uploads.SafeName).
func (c *Carrd) Save(dest, name string, data []byte) error {
	return os.WriteFile(filepath.Join(dest, name), data, 0o644) //nolint:gosec // dest comes from Dest and name passed SafeName
}

// DeleteFile removes one file from a folder of a project.
func (c *Carrd) DeleteFile(folder, rel, reqName string) error {
	dir, _, err := c.resolve(folder, rel)
	if err != nil {
		return err
	}
	name, ok := uploads.SafeName(reqName, uploads.IsCarrdExt)
	if !ok {
		return notFound("There is no such file.")
	}
	if err := os.Remove(filepath.Join(dir, name)); err != nil { //nolint:gosec // dir is from resolve and name from SafeName
		if os.IsNotExist(err) {
			return notFound("There is no such file.")
		}
		return err
	}
	return nil
}

// CreateFolder makes a sub-folder of name (normalized) inside a folder of a project and
// answers the name it got.
func (c *Carrd) CreateFolder(folder, rel, reqName string) (string, error) {
	dir, _, err := c.resolve(folder, rel)
	if err != nil {
		return "", err
	}
	name := uploads.Slugify(reqName, '-')
	if name == "" {
		return "", invalid("name", "Folder name must contain letters or numbers")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() { //nolint:gosec // dir comes from resolve
		return "", notFound("There is no such parent folder.")
	}
	if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil { //nolint:gosec // dir is from resolve and name is Slugify's output
		if os.IsExist(err) {
			return "", conflict("A folder named \"" + name + "\" already exists here")
		}
		return "", err
	}
	return name, nil
}

// DeleteFolder removes a sub-folder of a project and its contents. The project's root
// is not deleted this way.
func (c *Carrd) DeleteFolder(folder, rel string) error {
	dir, clean, err := c.resolve(folder, rel)
	if err != nil {
		return err
	}
	if clean == "" {
		return invalid("path", "Use the projects endpoint to delete a whole project")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() { //nolint:gosec // dir comes from resolve
		return notFound(msgNoSuchFolder)
	}
	return os.RemoveAll(dir) //nolint:gosec // dir comes from resolve
}
