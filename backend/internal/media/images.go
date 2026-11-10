package media

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"app-suite/internal/uploads"
)

// The image library is a set of categories, each a folder under <webRoot>/images
// whose files are served publicly at /images/<dir>/<file>. A category has a display
// name and a folder name; the pairs are kept in the dotfile manifest
// images/.categories.json, so no database table is needed. Deleting a category
// deletes its folder. A file uploaded under a name already there replaces it, so a
// picture can be swapped in place and every reference to it keeps working.

const (
	manifestFile = ".categories.json"
	// manifestVersion marks the manifest that holds the default categories as
	// ordinary ones, which staff may rename or delete.
	manifestVersion = 2

	dirAnnouncementsMain = "announcements_main"
	dirFlourishes        = "flourishes"
	// CoverDir is the book-club covers' folder: not a category, so a page with the
	// library alone can't list, replace or delete them.
	CoverDir = "bookclub"
)

// reservedDirs belong to a feature that writes under images/ without the manifest: a
// category made or renamed onto one would let the library's page list, replace or wipe
// files another page's permission guards.
var reservedDirs = []string{CoverDir, "announcements", "ema_plaques"}

// Reserved reports whether dir is a folder that is not the library's to make a
// category of.
func Reserved(dir string) bool {
	for _, r := range reservedDirs {
		if dir == r {
			return true
		}
	}
	return false
}

// Category is a folder of the image library.
type Category struct {
	Name string `json:"name"`
	Dir  string `json:"dir"`
	// FileCount and TotalSize describe its pictures; they are filled in by listings.
	FileCount int   `json:"-"`
	TotalSize int64 `json:"-"`
}

// Image is a picture in a category.
type Image struct {
	Name     string
	Size     int64
	Modified time.Time
}

type manifest struct {
	Version    int        `json:"version"`
	Categories []Category `json:"categories"`
}

// Images is the image library under a web root.
type Images struct {
	root string
	// mu serializes the manifest's read-modify-write, so two edits landing together
	// can't lose one or orphan a folder.
	mu sync.Mutex
}

// NewImages is the image library under webRoot.
func NewImages(webRoot string) *Images { return &Images{root: webRoot} }

func (m *Images) dir() string { return filepath.Join(m.root, "images") }

// ValidDir reports whether dir has the shape of a category folder: lowercase letters,
// digits and underscores, so it can't hold a path separator or a traversal.
func ValidDir(dir string) bool { return uploads.ValidSlug(dir, '_') }

func defaultCategories() []Category {
	return []Category{
		{Name: "Announcement Main", Dir: dirAnnouncementsMain},
		{Name: "Announcement Thumbnail", Dir: "announcements_thumb"},
		{Name: "Raffle", Dir: "raffles"},
		{Name: "Garapon", Dir: "garapons"},
		{Name: "Flourishes", Dir: dirFlourishes},
		{Name: "Affiliate Logos", Dir: "affiliate_logos"},
		{Name: "Affiliate Images", Dir: "affiliate_images"},
		{Name: "Stamp Cards", Dir: "stamp_cards"},
		{Name: "Stamp Stamps", Dir: "stamp_stamps"},
		{Name: "Stamp Prizes", Dir: "stamp_prizes"},
	}
}

// read is the manifest's categories, none when it is missing or unreadable.
func (m *Images) read() []Category {
	data, err := os.ReadFile(filepath.Join(m.dir(), manifestFile)) //nolint:gosec // the manifest's fixed name under the images root
	if err != nil {
		return []Category{}
	}
	var mf manifest
	if err := json.Unmarshal(data, &mf); err != nil {
		return []Category{}
	}
	out := make([]Category, 0, len(mf.Categories))
	for _, c := range mf.Categories {
		if ValidDir(c.Dir) {
			out = append(out, Category{Name: c.Name, Dir: c.Dir})
		}
	}
	return out
}

// write saves the manifest by writing a temporary file and renaming it over the
// manifest, so a crash mid-write can't leave one that doesn't parse, which would
// disable the whole library.
func (m *Images) write(cats []Category) error {
	if err := os.MkdirAll(m.dir(), 0o755); err != nil { //nolint:gosec // a public folder, served by the web server
		return err
	}
	data, err := json.Marshal(manifest{Version: manifestVersion, Categories: cats})
	if err != nil {
		return err
	}
	dst := filepath.Join(m.dir(), manifestFile)
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil { //nolint:gosec // the manifest is read by this server only
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// MigrateManifest brings the manifest to the current version at startup: an older one
// (or none) gets the default categories ahead of its own. It runs once; a current
// manifest is left alone, so a category staff renamed or deleted stays so. A manifest
// that doesn't parse is left for repair rather than overwritten.
func (m *Images) MigrateManifest() {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.dir(), manifestFile)
	data, err := os.ReadFile(path) //nolint:gosec // the manifest's fixed name under the images root
	switch {
	case err == nil:
		var mf manifest
		if uerr := json.Unmarshal(data, &mf); uerr != nil {
			slog.Error("image category manifest is corrupt; leaving it untouched - image admin disabled until repaired",
				"path", path, "error", uerr)
			return
		}
		if mf.Version >= manifestVersion {
			return
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		slog.Error("image category manifest unreadable; not seeding", "path", path, "error", err)
		return
	}

	merged := defaultCategories()
	seen := make(map[string]bool, len(merged))
	for _, c := range merged {
		seen[c.Dir] = true
	}
	for _, c := range m.read() {
		if !seen[c.Dir] {
			merged = append(merged, c)
			seen[c.Dir] = true
		}
	}
	if werr := m.write(merged); werr != nil {
		slog.Error("failed to write migrated image category manifest", "path", path, "error", werr)
	}
}

// MigrateAnnouncementImages copies the files of the old images/announcements folder
// into the announcements_main category, once each, so earlier announcement pictures
// stay in the picker. Safe to run at every start.
func (m *Images) MigrateAnnouncementImages() {
	srcDir := filepath.Join(m.dir(), "announcements")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return
	}
	dstDir := filepath.Join(m.dir(), dirAnnouncementsMain)
	for _, e := range entries {
		if e.IsDir() || !uploads.IsRasterExt(strings.ToLower(filepath.Ext(e.Name()))) {
			continue
		}
		dst := filepath.Join(dstDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.MkdirAll(dstDir, 0o755); err != nil { //nolint:gosec // a public folder, served by the web server
			return
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name())) //nolint:gosec // a directory entry's own name, in a fixed folder
		if err != nil {
			continue
		}
		_ = os.WriteFile(dst, data, 0o644) //nolint:gosec // a fixed folder and a name from its own listing
	}
}

// SeedFlourishes copies the app's built-in flourish SVGs, shipped in images/, into the
// Flourishes category so the theme pickers offer them. A file already there is left
// as it is, so staff may delete the copies.
func (m *Images) SeedFlourishes() {
	dstDir := filepath.Join(m.dir(), dirFlourishes)
	for _, name := range []string{"corner_flourish.svg", "called_flourish.svg"} {
		dst := filepath.Join(dstDir, name)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.dir(), name)) //nolint:gosec // one of the two fixed names this loop ranges over
		if err != nil {
			continue
		}
		if err := os.MkdirAll(dstDir, 0o755); err != nil { //nolint:gosec // a public folder, served by the web server
			return
		}
		_ = os.WriteFile(dst, data, 0o644) //nolint:gosec // a fixed folder and a fixed name
	}
}

// stats counts a category's pictures and their size. A folder not made yet counts none.
func (m *Images) stats(dir string) (count int, total int64) {
	entries, err := os.ReadDir(filepath.Join(m.dir(), dir))
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() || !uploads.IsLibraryExt(strings.ToLower(filepath.Ext(e.Name()))) {
			continue
		}
		if info, err := e.Info(); err == nil {
			count++
			total += info.Size()
		}
	}
	return count, total
}

// Categories lists the categories with their file counts and sizes.
func (m *Images) Categories() ([]Category, error) {
	if err := os.MkdirAll(m.dir(), 0o755); err != nil { //nolint:gosec // a public folder, served by the web server
		return nil, err
	}
	cats := m.read()
	for i := range cats {
		cats[i].FileCount, cats[i].TotalSize = m.stats(cats[i].Dir)
	}
	return cats, nil
}

// Known reports whether dir is a recorded category, so nothing is listed or written in
// an arbitrary folder of images/.
func (m *Images) Known(dir string) bool {
	if !ValidDir(dir) {
		return false
	}
	for _, c := range m.read() {
		if c.Dir == dir {
			return true
		}
	}
	return false
}

const (
	msgReserved       = "That directory name is reserved for another feature"
	msgNoSuchCategory = "There is no such image category."
)

// CreateCategory adds a category. The folder is dir normalized, or derived from name
// when dir is blank.
func (m *Images) CreateCategory(reqName, reqDir string) (Category, error) {
	name := strings.TrimSpace(reqName)
	if name == "" {
		return Category{}, invalid("name", "Category name is required")
	}
	dir := uploads.Slugify(reqDir, '_')
	if dir == "" {
		dir = uploads.Slugify(name, '_')
	}
	if dir == "" {
		return Category{}, invalid("name", "Could not derive a directory name - use letters or numbers in the name or directory")
	}
	if Reserved(dir) {
		return Category{}, conflict(msgReserved)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	cats := m.read()
	for _, c := range cats {
		if c.Dir == dir {
			return Category{}, conflict("A category using the directory \"" + dir + "\" already exists")
		}
		if strings.EqualFold(c.Name, name) {
			return Category{}, conflict("A category named \"" + name + "\" already exists")
		}
	}
	if err := os.MkdirAll(filepath.Join(m.dir(), dir), 0o755); err != nil { //nolint:gosec // dir is Slugify's output
		return Category{}, err
	}
	cat := Category{Name: name, Dir: dir}
	if err := m.write(append(cats, cat)); err != nil {
		return Category{}, err
	}
	return cat, nil
}

// UpdateCategory renames a category and, when newDir is given and differs, moves its
// folder. A nil or blank name or newDir keeps what it was, so a rename never moves a
// folder that stored references point into.
func (m *Images) UpdateCategory(dir string, reqName, reqNewDir *string) (Category, error) {
	if !ValidDir(dir) {
		return Category{}, notFound(msgNoSuchCategory)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cats := m.read()
	idx := -1
	for i, c := range cats {
		if c.Dir == dir {
			idx = i
		}
	}
	if idx == -1 {
		return Category{}, notFound(msgNoSuchCategory)
	}

	name := cats[idx].Name
	if reqName != nil {
		if name = strings.TrimSpace(*reqName); name == "" {
			return Category{}, invalid("name", "Category name is required")
		}
	}
	newDir := dir
	if reqNewDir != nil && strings.TrimSpace(*reqNewDir) != "" {
		if newDir = uploads.Slugify(*reqNewDir, '_'); newDir == "" {
			return Category{}, invalid("dir", "Could not derive a directory name")
		}
	}
	// Moving onto a reserved folder is refused, but renaming a category already
	// there is not.
	if newDir != dir && Reserved(newDir) {
		return Category{}, conflict(msgReserved)
	}
	for i, c := range cats {
		if i == idx {
			continue
		}
		if c.Dir == newDir {
			return Category{}, conflict("A category using the directory \"" + newDir + "\" already exists")
		}
		if strings.EqualFold(c.Name, name) {
			return Category{}, conflict("A category named \"" + name + "\" already exists")
		}
	}

	if newDir != dir {
		src, dst := filepath.Join(m.dir(), dir), filepath.Join(m.dir(), newDir)
		// The folder may not exist yet when nothing was uploaded; it moves only if there.
		if info, err := os.Stat(src); err == nil && info.IsDir() { //nolint:gosec // dir passed ValidDir and newDir is Slugify's output
			if err := os.Rename(src, dst); err != nil { //nolint:gosec // as above
				if os.IsExist(err) {
					return Category{}, conflict("A directory named \"" + newDir + "\" already exists")
				}
				return Category{}, err
			}
		} else if err := os.MkdirAll(dst, 0o755); err != nil { //nolint:gosec // as above
			return Category{}, err
		}
	}
	cats[idx] = Category{Name: name, Dir: newDir}
	if err := m.write(cats); err != nil {
		return Category{}, err
	}
	cat := cats[idx]
	cat.FileCount, cat.TotalSize = m.stats(cat.Dir)
	return cat, nil
}

// DeleteCategory removes a category and every file in its folder.
func (m *Images) DeleteCategory(dir string) error {
	if !ValidDir(dir) {
		return notFound(msgNoSuchCategory)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cats := m.read()
	idx := -1
	for i, c := range cats {
		if c.Dir == dir {
			idx = i
			break
		}
	}
	if idx == -1 {
		return notFound(msgNoSuchCategory)
	}
	if err := os.RemoveAll(filepath.Join(m.dir(), dir)); err != nil { //nolint:gosec // dir passed ValidDir
		return err
	}
	return m.write(append(cats[:idx], cats[idx+1:]...))
}

// List is a category's pictures, newest first. A category nothing was uploaded to has
// none.
func (m *Images) List(dir string) ([]Image, error) {
	if !m.Known(dir) {
		return nil, notFound(msgNoSuchCategory)
	}
	entries, err := os.ReadDir(filepath.Join(m.dir(), dir)) //nolint:gosec // dir is a recorded category
	if err != nil {
		return []Image{}, nil //nolint:nilerr // a folder not made yet is an empty category
	}
	out := make([]Image, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !uploads.IsLibraryExt(strings.ToLower(filepath.Ext(e.Name()))) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Image{Name: e.Name(), Size: info.Size(), Modified: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	return out, nil
}

// Save stores a picture in a category under name, replacing one of that name. The name
// must already be a safe library name (uploads.SafeName).
func (m *Images) Save(dir, name string, data []byte) error {
	if !m.Known(dir) {
		return notFound(msgNoSuchCategory)
	}
	dest := filepath.Join(m.dir(), dir)
	if err := os.MkdirAll(dest, 0o755); err != nil { //nolint:gosec // dir is a recorded category
		return err
	}
	return os.WriteFile(filepath.Join(dest, name), data, 0o644) //nolint:gosec // dir is a recorded category and name passed SafeName
}

// Delete removes one picture from a category.
func (m *Images) Delete(dir, reqName string) error {
	if !m.Known(dir) {
		return notFound(msgNoSuchCategory)
	}
	name, ok := uploads.SafeName(reqName, uploads.IsLibraryExt)
	if !ok {
		return notFound("There is no such image.")
	}
	if err := os.Remove(filepath.Join(m.dir(), dir, name)); err != nil { //nolint:gosec // dir is a recorded category and name passed SafeName
		if os.IsNotExist(err) {
			return notFound("There is no such image.")
		}
		return err
	}
	return nil
}

// SaveCover stores a book-club cover under name, replacing one of that name.
func (m *Images) SaveCover(name string, data []byte) error {
	dest := filepath.Join(m.dir(), CoverDir)
	if err := os.MkdirAll(dest, 0o755); err != nil { //nolint:gosec // a public folder, served by the web server
		return err
	}
	return os.WriteFile(filepath.Join(dest, name), data, 0o644) //nolint:gosec // name passed uploads.SafeName
}

// RemoveCover deletes an uploaded book-club cover, but only when the address points
// inside this site's covers folder. A cover from AniList, or anything outside the
// folder, is left alone.
func (m *Images) RemoveCover(coverURL string) {
	if coverURL == "" {
		return
	}
	path := coverURL
	if u, err := url.Parse(coverURL); err == nil && u.Path != "" {
		path = u.Path
	}
	path = strings.TrimPrefix(path, "/")
	if !strings.HasPrefix(path, "images/"+CoverDir+"/") {
		return
	}
	uploadDir, err := filepath.Abs(filepath.Join(m.dir(), CoverDir))
	if err != nil {
		return
	}
	target, err := filepath.Abs(filepath.Join(m.root, filepath.Clean(path)))
	if err != nil {
		return
	}
	if target == uploadDir || !strings.HasPrefix(target, uploadDir+string(os.PathSeparator)) {
		return
	}
	_ = os.Remove(target)
}
