package media

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"app-suite/internal/uploads"
)

// The data folder's public files: the image library at /images/, the Carrd projects
// at /carrd/ (the Carrd host proxies its whole site there), the plugin repository at
// /plugin/ and the game's sound effects at /sounds/. The fonts are never among them:
// the API streams those through its tokenized, origin-gated links, and nothing here
// opens the fonts folder.
//
// A file is served only when every segment of its path is a plain name (no dotfile, so
// the manifests and sidecars stay hidden, and no "." or ".."), its extension is one the
// folder holds, and it is a regular file inside the folder: a folder is never listed,
// and a link out of it is refused.

const (
	// dayCache keeps an unhashed file a day, at the edge and in the browser. A picture
	// replaced under its name can show the old one until then, or until it is purged.
	dayCache = "public, max-age=86400"
	// noStore is for the plugin repository, which a client must never get stale, and
	// for every miss, so a 404 never outlives the upload that would answer it.
	noStore = "no-store"
	// svgPolicy keeps a script in an SVG opened on its own from running.
	svgPolicy = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox"
)

// contentTypes are the types the public files go out with, by extension.
var contentTypes = map[string]string{
	".gif":  "image/gif",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".json": "application/json",
	".mp3":  "audio/mpeg",
	".mp4":  "video/mp4",
	".png":  "image/png",
	".svg":  "image/svg+xml",
	".webp": "image/webp",
	".zip":  "application/zip",
}

// publicFiles is how one folder's files are served.
type publicFiles struct {
	// allow approves an extension (lowercase, with its dot).
	allow func(ext string) bool
	// cache is the Cache-Control of a file served.
	cache string
	// anySite lets a page on any site read the files, as an outside site embedding
	// them needs.
	anySite bool
}

var (
	libraryFiles = publicFiles{allow: uploads.IsLibraryExt, cache: dayCache}
	carrdFiles   = publicFiles{allow: uploads.IsCarrdExt, cache: dayCache, anySite: true}
)

// Serve answers a GET or HEAD for /images/<rel>: a picture of the library, a reading
// list's cover or a plaque waiting for review.
func (m *Images) Serve(w http.ResponseWriter, r *http.Request, rel string) {
	libraryFiles.serve(w, r, m.dir(), rel)
}

// Serve answers a GET or HEAD for /carrd/<rel>: a file of a Carrd project, readable
// from any site.
func (c *Carrd) Serve(w http.ResponseWriter, r *http.Request, rel string) {
	carrdFiles.serve(w, r, c.dir(), rel)
}

// Plugin is the Dalamud plugin repository under a data folder: the index Dalamud reads
// and the package it downloads, both put there whole by a publish.
type Plugin struct{ root string }

// NewPlugin is the plugin repository under dataDir.
func NewPlugin(dataDir string) *Plugin { return &Plugin{root: dataDir} }

func (p *Plugin) dir() string { return filepath.Join(p.root, "plugin") }

// pluginFiles are the files a publish puts in the repository, by their path in it.
var pluginFiles = map[string]bool{
	"pluginmaster.json":               true,
	"SenpanCompanionAdmin/latest.zip": true,
}

// Serve answers a GET or HEAD for /plugin/<rel>, kept out of every cache: Dalamud
// refuses a package whose version is not the one the index promised.
func (p *Plugin) Serve(w http.ResponseWriter, r *http.Request, rel string) {
	files := publicFiles{allow: func(string) bool { return pluginFiles[rel] }, cache: noStore}
	files.serve(w, r, p.dir(), rel)
}

// Sounds is the game's sound effects under a data folder: MP3s the player's "game"
// sounds and the Yoever reaction play. They are kept out of the build and the
// repository, and put in the folder by hand.
type Sounds struct{ root string }

// NewSounds is the sound effects under dataDir.
func NewSounds(dataDir string) *Sounds { return &Sounds{root: dataDir} }

func (s *Sounds) dir() string { return filepath.Join(s.root, "sounds") }

var soundFiles = publicFiles{allow: func(ext string) bool { return ext == ".mp3" }, cache: dayCache}

// Serve answers a GET or HEAD for /sounds/<rel>: one of the sound effects.
func (s *Sounds) Serve(w http.ResponseWriter, r *http.Request, rel string) {
	soundFiles.serve(w, r, s.dir(), rel)
}

// publicPath reports whether rel, the part of a request's path below a public
// folder, may name a file there: slash-separated and already clean, with no empty,
// "." or ".." segment, no dotfile, and no backslash or NUL, so it reads the same on
// every OS and can't leave the folder or reach a hidden file.
func publicPath(rel string) bool {
	if rel == "" || strings.ContainsAny(rel, "\\\x00") || path.Clean("/"+rel) != "/"+rel {
		return false
	}
	for seg := range strings.SplitSeq(rel, "/") {
		if seg == "" || strings.HasPrefix(seg, ".") {
			return false
		}
	}
	return true
}

func (p publicFiles) serve(w http.ResponseWriter, r *http.Request, dir, rel string) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

	ext := strings.ToLower(path.Ext(rel))
	contentType, known := contentTypes[ext]
	if !publicPath(rel) || !known || !p.allow(ext) {
		answerMissing(w)
		return
	}
	// The root keeps the open inside dir: a symbolic link that leads out of it fails.
	f, err := os.OpenInRoot(dir, filepath.FromSlash(rel))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("a public file could not be opened", "folder", dir, "error", openReason(err))
		}
		answerMissing(w)
		return
	}
	defer func() { _ = f.Close() }() // read only: a close error changes nothing sent
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		answerMissing(w)
		return
	}

	h.Set("Content-Type", contentType)
	h.Set("Cache-Control", p.cache)
	h.Set("ETag", fmt.Sprintf(`"%x-%x"`, info.Size(), info.ModTime().UnixMicro()))
	if p.anySite {
		h.Set("Access-Control-Allow-Origin", "*")
	}
	if ext == ".svg" {
		h.Set("Content-Security-Policy", svgPolicy)
	}
	// ServeContent answers HEAD, ranges (audio and video seek with them) and the
	// conditional requests, and drops the caching headers from an error it sends.
	http.ServeContent(w, r, "", info.ModTime(), f)
}

func answerMissing(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", noStore)
	http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
}

// openReason is what went wrong opening a file, without the file's path, which may be a
// plaque's unguessable name.
func openReason(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}
