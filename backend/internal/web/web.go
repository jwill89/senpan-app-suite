// Package web serves the SPA the binary embeds: frontend/'s Vite build, which
// `npm run build` writes into dist/ here (vite.config.ts's outDir). Go embeds only
// files inside its own module, so the front end builds before any Go build, test or
// lint; without it, compiling this package fails on "pattern all:dist: no matching
// files found".
//
// Before 4.0.0 Apache served the build from the web root (deploy/.htaccess); the
// headers and cache rules it set are here now. Apache still serves the web root's
// upload folders, and this handler never answers for them.
package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:dist
var dist embed.FS

// Dist is the embedded build, rooted at its index.html.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" is a valid path, so fs.Sub cannot fail
	}
	return sub
}

// webRootFolders are the web root's own folders, which Apache serves (images and the
// raffle uploads, the Carrd sites, the plugin repository) or refuses (fonts, streamed
// by the API instead). A request for one that reaches the binary is for a file Apache
// doesn't have.
var webRootFolders = []string{"/images/", "/fonts/", "/carrd/", "/plugin/"}

// contentSecurityPolicy is the policy deploy/.htaccess set before 4.0.0, unchanged:
// Google Fonts and cross-origin images over https:, the themes' inline CSS, data:
// exports, the wss: game socket, Turnstile (script and frame) and Cloudflare's Web
// Analytics beacon.
const contentSecurityPolicy = "default-src 'self'; img-src 'self' data: https:; " +
	"style-src 'self' 'unsafe-inline' https:; font-src 'self' https: data:; " +
	"script-src 'self' https://challenges.cloudflare.com https://static.cloudflareinsights.com; " +
	"frame-src https://challenges.cloudflare.com; connect-src 'self' https: wss:; " +
	"frame-ancestors 'none'; base-uri 'self'; object-src 'none'"

// contentTypes names the build's types Go's own table doesn't (it knows .css, .html,
// .js and .webp), since every response goes out with nosniff.
var contentTypes = map[string]string{
	".mp3":         "audio/mpeg",
	".ttf":         "font/ttf",
	".webmanifest": "application/manifest+json",
	".woff":        "font/woff",
	".woff2":       "font/woff2",
}

const (
	// The page, and the service worker with its registration: never cached, so a
	// deploy reaches every tab at once, and the worker can always update itself.
	noStore = "no-cache, no-store, must-revalidate"
	// Vite's content-hashed bundles, and Workbox's: a new build names them anew.
	immutable = "public, max-age=31536000, immutable"
	// Anything else: kept, but checked against its ETag before each use.
	revalidate = "no-cache"
)

type file struct {
	body        []byte
	etag        string
	contentType string
	cache       string
}

// Handler serves the build: its files, and the app's page for any other path, so the
// SPA's router can take it.
type Handler struct {
	files map[string]file
}

// New reads the build into memory, as the binary holds it anyway, with an ETag for
// each file. It fails when the build has no index.html.
func New(build fs.FS) (*Handler, error) {
	files := map[string]file{}
	err := fs.WalkDir(build, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(build, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		files[name] = file{
			body:        body,
			etag:        `"` + hex.EncodeToString(sum[:8]) + `"`,
			contentType: contentType(name),
			cache:       cachePolicy(name),
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read the embedded front end: %w", err)
	}
	if _, ok := files["index.html"]; !ok {
		return nil, errors.New("the embedded front end has no index.html: build it (task build:web) before the Go build")
	}
	return &Handler{files: files}, nil
}

func contentType(name string) string {
	ext := path.Ext(name)
	if t, ok := contentTypes[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

func cachePolicy(name string) string {
	switch {
	case name == "index.html" || name == "sw.js" || name == "registerSW.js":
		return noStore
	case strings.HasPrefix(name, "assets/") || strings.HasPrefix(name, "workbox-"):
		return immutable
	default:
		return revalidate
	}
}

// ServeHTTP answers GET and HEAD: a file of the build, a 404 for a file it lacks (a
// path with an extension, or one in an API or web-root folder), and the app's page for
// anything else.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	setSecurityHeaders(w.Header())
	clean := path.Clean("/" + r.URL.Path)
	name := strings.TrimPrefix(clean, "/")
	if f, ok := h.files[name]; ok && name != "" {
		serve(w, r, name, f)
		return
	}
	if missingFile(clean) {
		http.NotFound(w, r)
		return
	}
	serve(w, r, "index.html", h.files["index.html"])
}

// missingFile reports whether a path not in the build names a file rather than one of
// the app's own pages: it has an extension, or it is in the API's or a web-root folder.
func missingFile(clean string) bool {
	if clean == "/api" || strings.HasPrefix(clean, "/api/") || path.Ext(clean) != "" {
		return true
	}
	for _, folder := range webRootFolders {
		if strings.HasPrefix(clean+"/", folder) {
			return true
		}
	}
	return false
}

func serve(w http.ResponseWriter, r *http.Request, name string, f file) {
	header := w.Header()
	header.Set("Content-Type", f.contentType)
	header.Set("Cache-Control", f.cache)
	header.Set("ETag", f.etag)
	// The zero time leaves out Last-Modified, which an embedded file doesn't have;
	// the ETag answers a revalidation with 304.
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(f.body))
}

// setSecurityHeaders sets what deploy/.htaccess set on the page and its files.
func setSecurityHeaders(h http.Header) {
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")
	h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
}
