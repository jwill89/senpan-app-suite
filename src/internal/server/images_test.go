package server_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"app-suite/internal/server"
	"app-suite/internal/store"
	"app-suite/internal/ws"
)

// pngBytes is a minimal payload whose leading bytes sniff as image/png, so it
// passes the upload content-type check (mirrors the carrd upload tests).
var pngBytes = []byte("\x89PNG\r\n\x1a\nimg")

// postImagesUpload posts a multipart upload of name→content files to a category
// dir via /api/images/upload.
func (e *testEnv) postImagesUpload(t *testing.T, dir string, files map[string][]byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("dir", dir); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		fw, err := mw.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	resp, err := e.client.Post(e.url("/api/images/upload"), mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// imageNames returns the names of the images listed for a category.
func (e *testEnv) imageNames(t *testing.T, dir string) []string {
	t.Helper()
	resp := e.get(t, "/api/images?dir="+dir)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/images?dir=%s status = %d; want 200", dir, resp.StatusCode)
	}
	data := decodeBody(t, resp)
	raw, _ := data["images"].([]any)
	names := make([]string, 0, len(raw))
	for _, it := range raw {
		if m, ok := it.(map[string]any); ok {
			names = append(names, m["name"].(string))
		}
	}
	return names
}

func TestImages_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/image-categories")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /api/image-categories status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()

	resp = env.postJSON(t, "/api/image-categories", map[string]any{"action": "create", "name": "X"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST /api/image-categories status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestImages_ListCategories_Permanent(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.get(t, "/api/image-categories")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	data := decodeBody(t, resp)
	cats, _ := data["categories"].([]any)
	want := map[string]bool{
		"announcements_main": false, "announcements_thumb": false,
		"raffles": false, "flourishes": false,
		"affiliate_logos": false, "affiliate_images": false,
	}
	for _, c := range cats {
		m := c.(map[string]any)
		dir := m["dir"].(string)
		if _, ok := want[dir]; ok {
			if m["permanent"] != true {
				t.Errorf("category %s permanent = %v; want true", dir, m["permanent"])
			}
			want[dir] = true
		}
	}
	for dir, found := range want {
		if !found {
			t.Errorf("permanent category %q missing from listing", dir)
		}
	}
}

func TestImages_CreateCategory_DefaultDir(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/image-categories", map[string]any{
		"action": "create", "name": "Event Banners!",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d; want 201", resp.StatusCode)
	}
	data := decodeBody(t, resp)
	cat := data["category"].(map[string]any)
	if cat["dir"] != "event_banners" { // spaces → underscores, punctuation dropped
		t.Errorf("dir = %v; want event_banners", cat["dir"])
	}
}

func TestImages_CreateCategory_ExplicitDir(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/image-categories", map[string]any{
		"action": "create", "name": "Promo", "dir": "My Promo 2026",
	})
	data := decodeBody(t, resp)
	cat := data["category"].(map[string]any)
	if cat["dir"] != "my_promo_2026" {
		t.Errorf("dir = %v; want my_promo_2026", cat["dir"])
	}
}

func TestImages_CreateCategory_ReservedDir(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/image-categories", map[string]any{
		"action": "create", "name": "Sneaky", "dir": "raffles",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d; want 409 (reserved permanent dir)", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestImages_CreateCategory_DuplicateDir(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/image-categories", map[string]any{
		"action": "create", "name": "First", "dir": "shared",
	}).Body.Close()

	resp := env.postJSON(t, "/api/image-categories", map[string]any{
		"action": "create", "name": "Second", "dir": "shared",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestImages_DeletePermanent_Forbidden(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/image-categories", map[string]any{
		"action": "delete", "dir": "raffles",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d; want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestImages_RenameCustomCategory(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/image-categories", map[string]any{
		"action": "create", "name": "Old", "dir": "old_dir",
	}).Body.Close()
	// Put an image in it so the rename must move a real folder.
	env.postImagesUpload(t, "old_dir", map[string][]byte{"a.png": pngBytes}).Body.Close()

	resp := env.postJSON(t, "/api/image-categories", map[string]any{
		"action": "rename", "dir": "old_dir", "name": "New", "new_dir": "new_dir",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// The image followed the rename into the new directory.
	if names := env.imageNames(t, "new_dir"); len(names) != 1 || names[0] != "a.png" {
		t.Errorf("new_dir images = %v; want [a.png]", names)
	}
	// The old directory is no longer a known category.
	resp = env.get(t, "/api/images?dir=old_dir")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("GET old_dir status = %d; want 400 (unknown)", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestImages_UploadListDelete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postImagesUpload(t, "announcements_main", map[string][]byte{
		"hero.png": pngBytes,
		"note.txt": []byte("not an image"),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d; want 200", resp.StatusCode)
	}
	data := decodeBody(t, resp)
	if up, _ := data["uploaded"].([]any); len(up) != 1 {
		t.Errorf("uploaded = %v; want 1 (only the png)", data["uploaded"])
	}
	if sk, _ := data["skipped"].([]any); len(sk) != 1 {
		t.Errorf("skipped = %v; want 1 (the txt)", data["skipped"])
	}

	if names := env.imageNames(t, "announcements_main"); len(names) != 1 || names[0] != "hero.png" {
		t.Fatalf("listed images = %v; want [hero.png]", names)
	}

	resp = env.postJSON(t, "/api/images", map[string]any{
		"action": "delete", "dir": "announcements_main", "name": "hero.png",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	if names := env.imageNames(t, "announcements_main"); len(names) != 0 {
		t.Errorf("after delete images = %v; want empty", names)
	}
}

func TestImages_List_UnknownDir(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.get(t, "/api/images?dir=does_not_exist")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestImages_DeleteCustomCategory_RemovesFiles(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/image-categories", map[string]any{
		"action": "create", "name": "Temp", "dir": "temp_cat",
	}).Body.Close()
	env.postImagesUpload(t, "temp_cat", map[string][]byte{"x.png": pngBytes}).Body.Close()

	resp := env.postJSON(t, "/api/image-categories", map[string]any{
		"action": "delete", "dir": "temp_cat",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// The category (and its directory) is gone.
	resp = env.get(t, "/api/images?dir=temp_cat")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("GET deleted category status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// svgBytes is a minimal valid SVG (sniffs as XML/text, contains "<svg").
var svgBytes = []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path d="M0 0h1v1H0z"/></svg>`)

func TestImages_SVGUploadToFlourishes(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postImagesUpload(t, "flourishes", map[string][]byte{
		"swirl.svg": svgBytes,
		"bad.svg":   []byte("not really svg"),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d; want 200", resp.StatusCode)
	}
	data := decodeBody(t, resp)
	if up, _ := data["uploaded"].([]any); len(up) != 1 {
		t.Errorf("uploaded = %v; want 1 (only the valid svg)", data["uploaded"])
	}
	if sk, _ := data["skipped"].([]any); len(sk) != 1 {
		t.Errorf("skipped = %v; want 1 (the non-svg)", data["skipped"])
	}
	if names := env.imageNames(t, "flourishes"); len(names) != 1 || names[0] != "swirl.svg" {
		t.Errorf("flourishes images = %v; want [swirl.svg]", names)
	}
}

// TestImages_SeedFlourishes verifies the built-in flourish SVGs shipped to
// <webRoot>/images are copied into the Flourishes category on startup.
func TestImages_SeedFlourishes(t *testing.T) {
	webRoot := t.TempDir()
	imagesDir := filepath.Join(webRoot, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"corner_flourish.svg", "called_flourish.svg"} {
		if err := os.WriteFile(filepath.Join(imagesDir, name), svgBytes, 0644); err != nil {
			t.Fatal(err)
		}
	}

	env := newTestEnvWithWebRoot(t, webRoot)
	env.loginAdmin(t)

	names := env.imageNames(t, "flourishes")
	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	if !got["corner_flourish.svg"] || !got["called_flourish.svg"] {
		t.Errorf("seeded flourishes = %v; want both built-ins", names)
	}
}

// TestImages_Migration verifies the one-time copy of legacy images/announcements
// files into the announcements_main category on server start.
func TestImages_Migration(t *testing.T) {
	webRoot := t.TempDir()
	legacyDir := filepath.Join(webRoot, "images", "announcements")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "legacy.png"), pngBytes, 0644); err != nil {
		t.Fatal(err)
	}

	env := newTestEnvWithWebRoot(t, webRoot)
	env.loginAdmin(t)

	if names := env.imageNames(t, "announcements_main"); len(names) != 1 || names[0] != "legacy.png" {
		t.Errorf("announcements_main = %v; want [legacy.png] migrated from legacy dir", names)
	}
}

// newTestEnvWithWebRoot builds a test env backed by a caller-provided webRoot, so
// a test can pre-seed files before the server (and its startup migration) run.
func newTestEnvWithWebRoot(t *testing.T, webRoot string) *testEnv {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	srv := server.New(st, ws.NewHub(), testSecret, webRoot, nil)
	ts := httptest.NewTLSServer(srv)
	t.Cleanup(ts.Close)

	jar, _ := cookiejar.New(nil)
	client := ts.Client()
	client.Jar = jar
	return &testEnv{ts: ts, client: client, store: st, srv: srv}
}
