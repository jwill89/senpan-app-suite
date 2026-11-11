package media

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// get serves rel through serve as a GET (or the method given), with the headers given.
func get(t *testing.T, serve func(http.ResponseWriter, *http.Request, string), rel string, method string, header map[string]string) *http.Response {
	t.Helper()
	if method == "" {
		method = http.MethodGet
	}
	r := httptest.NewRequest(method, "/x/"+rel, nil)
	for k, v := range header {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	serve(w, r, rel)
	return w.Result()
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPublicPath(t *testing.T) {
	for rel, want := range map[string]bool{
		"logo.png":                     true,
		"raffles/prize.png":            true,
		"site/spring-art/deep.png":     true,
		"":                             false,
		"/logo.png":                    false,
		"raffles/":                     false,
		"raffles//prize.png":           false,
		"raffles/./prize.png":          false,
		"../fonts/Jasper.ttf":          false,
		"raffles/../../fonts/x.ttf":    false,
		".categories.json":             false,
		"site/.carrd.json":             false,
		"fonts/.woff2/x.woff2":         false,
		`raffles\prize.png`:            false,
		"raffles/prize.png\x00.svg":    false,
		"site/.hidden/visible.png":     false,
		"SenpanCompanionAdmin/a.zip":   true,
		"a/b/c/d/e/f/g/h/i/j/k/l.webp": true,
	} {
		if got := publicPath(rel); got != want {
			t.Errorf("publicPath(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestImagesServeAPicture(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "images", "raffles", "prize.png"), "png bytes")
	m := NewImages(root)

	resp := get(t, m.Serve, "raffles/prize.png", "", nil)
	if resp.StatusCode != http.StatusOK || body(t, resp) != "png bytes" {
		t.Fatalf("status %d", resp.StatusCode)
	}
	for k, want := range map[string]string{
		"Content-Type":                "image/png",
		"Cache-Control":               "public, max-age=86400",
		"X-Content-Type-Options":      "nosniff",
		"X-Frame-Options":             "DENY",
		"Access-Control-Allow-Origin": "",
		"Content-Security-Policy":     "",
	} {
		if got := resp.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	etag := resp.Header.Get("ETag")
	if etag == "" || resp.Header.Get("Last-Modified") == "" {
		t.Fatalf("no validators: ETag %q, Last-Modified %q", etag, resp.Header.Get("Last-Modified"))
	}

	again := get(t, m.Serve, "raffles/prize.png", "", map[string]string{"If-None-Match": etag})
	if again.StatusCode != http.StatusNotModified {
		t.Errorf("a repeat with its ETag answered %d, want 304", again.StatusCode)
	}
	head := get(t, m.Serve, "raffles/prize.png", http.MethodHead, nil)
	if head.StatusCode != http.StatusOK || body(t, head) != "" || head.Header.Get("Content-Length") != "9" {
		t.Errorf("HEAD answered %d, length %q", head.StatusCode, head.Header.Get("Content-Length"))
	}
}

func TestImagesServeAnSVGSandboxed(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "images", "flourishes", "swirl.svg"), "<svg/>")
	resp := get(t, NewImages(root).Serve, "flourishes/swirl.svg", "", nil)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("status %d, type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("Content-Security-Policy") != svgPolicy {
		t.Errorf("CSP = %q", resp.Header.Get("Content-Security-Policy"))
	}
}

func TestImagesRefuseWhatIsNotAPublicPicture(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "images", ".categories.json"), `{"version":2}`)
	write(t, filepath.Join(root, "images", "raffles", "notes.txt"), "text")
	write(t, filepath.Join(root, "images", "raffles", "prize.png"), "png")
	write(t, filepath.Join(root, "fonts", "Jasper.ttf"), "licensed")
	m := NewImages(root)

	for _, rel := range []string{
		".categories.json",        // the manifest
		"raffles/notes.txt",       // not a picture
		"raffles",                 // a folder: never listed
		"raffles/",                // the same
		"raffles/missing.png",     // not there
		"../fonts/Jasper.ttf",     // out of the folder
		"raffles/../prize.png",    // not clean
		`raffles\prize.png`,       // a backslash
		"raffles/prize.png/x.png", // a file taken for a folder
	} {
		resp := get(t, m.Serve, rel, "", nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%q answered %d, want 404", rel, resp.StatusCode)
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%q's 404 has Cache-Control %q, want no-store", rel, got)
		}
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%q's 404 lacks nosniff", rel)
		}
	}
}

func TestImagesRefuseALinkOutOfTheFolder(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "fonts", "Jasper.png"), "licensed")
	if err := os.MkdirAll(filepath.Join(root, "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "fonts"), filepath.Join(root, "images", "out")); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	resp := get(t, NewImages(root).Serve, "out/Jasper.png", "", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("a link out of the folder answered %d, want 404", resp.StatusCode)
	}
}

func TestCarrdServesToAnySite(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "carrd", "site", ".carrd.json"), `{"title":"Site"}`)
	write(t, filepath.Join(root, "carrd", "site", "spring-art", "song.mp3"), "0123456789")
	write(t, filepath.Join(root, "carrd", "site", "spring-art", "a.png"), "png")
	c := NewCarrd(root)

	resp := get(t, c.Serve, "site/spring-art/a.png", "", map[string]string{"Origin": "https://example.carrd.co"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" || resp.Header.Get("Cache-Control") != "public, max-age=86400" {
		t.Errorf("CORS %q, cache %q", resp.Header.Get("Access-Control-Allow-Origin"), resp.Header.Get("Cache-Control"))
	}

	// Audio and video seek with ranges.
	part := get(t, c.Serve, "site/spring-art/song.mp3", "", map[string]string{"Range": "bytes=2-4"})
	if part.StatusCode != http.StatusPartialContent || body(t, part) != "234" || part.Header.Get("Content-Type") != "audio/mpeg" {
		t.Errorf("a range answered %d, type %q", part.StatusCode, part.Header.Get("Content-Type"))
	}

	for _, rel := range []string{"site/.carrd.json", "site", "site/spring-art/", "site/spring-art/a.svg"} {
		if resp := get(t, c.Serve, rel, "", nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%q answered %d, want 404", rel, resp.StatusCode)
		}
	}
}

func TestPluginServesItsTwoFilesUncached(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "plugin", "pluginmaster.json"), `[{"AssemblyVersion":"4.0.0.0"}]`)
	write(t, filepath.Join(root, "plugin", "SenpanCompanionAdmin", "latest.zip"), "zip")
	write(t, filepath.Join(root, "plugin", "SenpanCompanionAdmin", "other.zip"), "zip")
	write(t, filepath.Join(root, "plugin", ".htaccess"), "Options -Indexes")
	p := NewPlugin(root)

	for rel, wantType := range map[string]string{
		"pluginmaster.json":               "application/json",
		"SenpanCompanionAdmin/latest.zip": "application/zip",
	} {
		resp := get(t, p.Serve, rel, "", nil)
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != wantType {
			t.Errorf("%q answered %d, type %q", rel, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		if resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%q has Cache-Control %q, want no-store", rel, resp.Header.Get("Cache-Control"))
		}
	}
	for _, rel := range []string{"SenpanCompanionAdmin/other.zip", ".htaccess", "SenpanCompanionAdmin"} {
		if resp := get(t, p.Serve, rel, "", nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%q answered %d, want 404", rel, resp.StatusCode)
		}
	}
}

func TestPluginMissingIsNotCached(t *testing.T) {
	resp := get(t, NewPlugin(t.TempDir()).Serve, "pluginmaster.json", "", nil)
	if resp.StatusCode != http.StatusNotFound || resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("answered %d with Cache-Control %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
}

func TestSoundsServeTheirMP3s(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "sounds", "this_is_bad.mp3"), "mp3 bytes")
	write(t, filepath.Join(root, "sounds", "notes.txt"), "text")
	s := NewSounds(root)

	resp := get(t, s.Serve, "this_is_bad.mp3", "", nil)
	if resp.StatusCode != http.StatusOK || body(t, resp) != "mp3 bytes" {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "audio/mpeg" || resp.Header.Get("Cache-Control") != "public, max-age=86400" {
		t.Errorf("type %q, cache %q", resp.Header.Get("Content-Type"), resp.Header.Get("Cache-Control"))
	}
	for _, rel := range []string{"notes.txt", "missing.mp3", "../sounds/this_is_bad.mp3", ""} {
		if resp := get(t, s.Serve, rel, "", nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%q answered %d, want 404", rel, resp.StatusCode)
		}
	}
}
