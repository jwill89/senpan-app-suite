package server_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"
)

// ── Carrd image hosting (admin-only) ─────────────────────────────────────────

func TestCarrd_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/carrd/projects")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /api/carrd/projects status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()

	resp = env.postJSON(t, "/api/carrd/projects", map[string]any{"title": "X"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST /api/carrd/projects status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCarrd_CreateProject_DefaultFolder(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "My Cool Project!",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d; want 201", resp.StatusCode)
	}
	data := decodeBody(t, resp)
	project, _ := data["project"].(map[string]any)
	if project == nil {
		t.Fatal("expected project in response")
	}
	// "My Cool Project!" → letters/numbers only, spaces → hyphens.
	if project["folder"] != "my-cool-project" {
		t.Errorf("folder = %v; want my-cool-project", project["folder"])
	}
	if project["title"] != "My Cool Project!" {
		t.Errorf("title = %v; want My Cool Project!", project["title"])
	}
}

func TestCarrd_CreateProject_ExplicitFolder(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Spring Sale", "folder": "Spring 2026 Promo",
	})
	data := decodeBody(t, resp)
	project, _ := data["project"].(map[string]any)
	if project == nil {
		t.Fatal("expected project in response")
	}
	if project["folder"] != "spring-2026-promo" {
		t.Errorf("folder = %v; want spring-2026-promo", project["folder"])
	}
}

func TestCarrd_CreateProject_EmptyTitle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "   ",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCarrd_CreateProject_DuplicateTitle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Gallery", "folder": "gallery-a",
	}).Body.Close()

	// Same title (case-insensitive), different folder → conflict.
	resp := env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "gallery", "folder": "gallery-b",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCarrd_CreateProject_DuplicateFolder(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "First", "folder": "shared",
	}).Body.Close()

	resp := env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Second", "folder": "shared",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCarrd_ListProjects(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Beta", "folder": "beta",
	}).Body.Close()
	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Alpha", "folder": "alpha",
	}).Body.Close()

	resp := env.get(t, "/api/carrd/projects")
	data := decodeBody(t, resp)
	projects, _ := data["projects"].([]any)
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	// Sorted by title (case-insensitive): Alpha before Beta.
	first, _ := projects[0].(map[string]any)
	if first["title"] != "Alpha" {
		t.Errorf("first project = %v; want Alpha", first["title"])
	}
}

func TestCarrd_DeleteProject(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Temp", "folder": "temp",
	}).Body.Close()

	resp := env.del(t, "/api/carrd/projects/temp")
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// Now gone from the listing.
	resp = env.get(t, "/api/carrd/projects")
	data := decodeBody(t, resp)
	projects, _ := data["projects"].([]any)
	if len(projects) != 0 {
		t.Errorf("expected 0 projects after delete, got %d", len(projects))
	}
}

func TestCarrd_DeleteProject_InvalidFolder(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Path-traversal attempt must be rejected by folder validation. Encode the
	// slash so it stays a single {folder} path segment rather than routing away.
	resp := env.del(t, "/api/carrd/projects/..%2Fsecret")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCarrd_RenameProject(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Old Title", "folder": "old-folder",
	}).Body.Close()

	// Rename the title and the folder together.
	resp := env.patchJSON(t, "/api/carrd/projects/old-folder", map[string]any{
		"title": "New Title", "new_folder": "Brand New Folder",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename status = %d; want 200", resp.StatusCode)
	}
	project := decodeBody(t, resp)["project"].(map[string]any)
	if project["title"] != "New Title" {
		t.Errorf("title = %v; want New Title", project["title"])
	}
	if project["folder"] != "brand-new-folder" {
		t.Errorf("folder = %v; want brand-new-folder", project["folder"])
	}

	// The listing reflects the new title/folder and the old folder is gone.
	resp = env.get(t, "/api/carrd/projects")
	projects, _ := decodeBody(t, resp)["projects"].([]any)
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	got := projects[0].(map[string]any)
	if got["folder"] != "brand-new-folder" || got["title"] != "New Title" {
		t.Errorf("listed project = %v; want New Title / brand-new-folder", got)
	}
}

func TestCarrd_RenameProject_TitleOnly(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Keep Folder", "folder": "keep",
	}).Body.Close()

	// No new_folder → the folder stays, only the title changes.
	resp := env.patchJSON(t, "/api/carrd/projects/keep", map[string]any{
		"title": "Renamed",
	})
	project := decodeBody(t, resp)["project"].(map[string]any)
	if project["folder"] != "keep" {
		t.Errorf("folder = %v; want keep (unchanged)", project["folder"])
	}
	if project["title"] != "Renamed" {
		t.Errorf("title = %v; want Renamed", project["title"])
	}
}

func TestCarrd_RenameProject_DuplicateTitle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Alpha", "folder": "alpha",
	}).Body.Close()
	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Beta", "folder": "beta",
	}).Body.Close()

	// Renaming Beta to Alpha's title (case-insensitive) → conflict.
	resp := env.patchJSON(t, "/api/carrd/projects/beta", map[string]any{
		"title": "alpha",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCarrd_UploadAndListAndDeleteImage(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Pics", "folder": "pics",
	}).Body.Close()

	// Upload a valid .png and an invalid .txt in the same batch.
	resp := env.postCarrdUpload(t, "pics", "", map[string][]byte{
		"hero.png":  []byte("\x89PNG\r\n\x1a\nfake"),
		"notes.txt": []byte("nope"),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d; want 200", resp.StatusCode)
	}
	data := decodeBody(t, resp)
	uploaded, _ := data["uploaded"].([]any)
	skipped, _ := data["skipped"].([]any)
	if len(uploaded) != 1 || uploaded[0] != "hero.png" {
		t.Errorf("uploaded = %v; want [hero.png]", uploaded)
	}
	if len(skipped) != 1 {
		t.Errorf("skipped = %v; want 1 (notes.txt)", skipped)
	}

	// List images.
	resp = env.get(t, "/api/carrd/images?folder=pics")
	data = decodeBody(t, resp)
	images, _ := data["images"].([]any)
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	img := images[0].(map[string]any)
	if img["name"] != "hero.png" {
		t.Errorf("image name = %v; want hero.png", img["name"])
	}

	// Delete it (query params, 204).
	resp = env.del(t, "/api/carrd/images?folder=pics&name=hero.png")
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete image status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	resp = env.get(t, "/api/carrd/images?folder=pics")
	data = decodeBody(t, resp)
	images, _ = data["images"].([]any)
	if len(images) != 0 {
		t.Errorf("expected 0 images after delete, got %d", len(images))
	}
}

func TestCarrd_UploadOverwrites(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Over", "folder": "over",
	}).Body.Close()

	env.postCarrdUpload(t, "over", "", map[string][]byte{"a.png": []byte("\x89PNG\r\n\x1a\nfirst")}).Body.Close()
	// Same name again — should overwrite, not error or duplicate.
	resp := env.postCarrdUpload(t, "over", "", map[string][]byte{"a.png": []byte("\x89PNG\r\n\x1a\nsecond-longer")})
	data := decodeBody(t, resp)
	uploaded, _ := data["uploaded"].([]any)
	if len(uploaded) != 1 {
		t.Fatalf("uploaded = %v; want 1 (overwrite)", uploaded)
	}

	resp = env.get(t, "/api/carrd/images?folder=over")
	data = decodeBody(t, resp)
	images, _ := data["images"].([]any)
	if len(images) != 1 {
		t.Errorf("expected 1 image after overwrite, got %d", len(images))
	}
	if images[0].(map[string]any)["size"] != float64(len("\x89PNG\r\n\x1a\nsecond-longer")) {
		t.Errorf("size = %v; want overwritten content size", images[0].(map[string]any)["size"])
	}
}

func TestCarrd_UploadAudioAndVideo(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Media", "folder": "media",
	}).Body.Close()

	// .mp3 and .mp4 are accepted; .txt is rejected.
	resp := env.postCarrdUpload(t, "media", "", map[string][]byte{
		"theme.mp3":  []byte("ID3audio"),
		"clip.mp4":   []byte("ftypmp4"),
		"readme.txt": []byte("nope"),
	})
	data := decodeBody(t, resp)
	uploaded, _ := data["uploaded"].([]any)
	skipped, _ := data["skipped"].([]any)
	if len(uploaded) != 2 {
		t.Errorf("uploaded = %v; want 2 (mp3 + mp4)", uploaded)
	}
	if len(skipped) != 1 {
		t.Errorf("skipped = %v; want 1 (readme.txt)", skipped)
	}

	// Both media files are listed.
	resp = env.get(t, "/api/carrd/images?folder=media")
	data = decodeBody(t, resp)
	images, _ := data["images"].([]any)
	if len(images) != 2 {
		t.Errorf("listed = %d; want 2 media files", len(images))
	}
}

func TestCarrd_UploadMissingProject(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postCarrdUpload(t, "ghost", "", map[string][]byte{"a.png": []byte("x")})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCarrd_ListImages_InvalidFolder(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.get(t, "/api/carrd/images?folder=..%2Fetc")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCarrd_DeleteImage_InvalidName(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Names", "folder": "names",
	}).Body.Close()

	// A disallowed extension must be rejected by safeCarrdFileName (400).
	resp := env.del(t, "/api/carrd/images?folder=names&name=evil.txt")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// A valid name that doesn't exist → 404.
	resp = env.del(t, "/api/carrd/images?folder=names&name=missing.png")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCarrd_SubDir_CreateUploadListDelete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Client", "folder": "client",
	}).Body.Close()

	// Create a nested sub-directory: client/spring/banners (201).
	resp := env.postJSON(t, "/api/carrd/images/dirs", map[string]any{
		"folder": "client", "path": "", "name": "Spring Sale",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create_dir status = %d; want 201", resp.StatusCode)
	}
	resp.Body.Close()
	resp = env.postJSON(t, "/api/carrd/images/dirs", map[string]any{
		"folder": "client", "path": "spring-sale", "name": "banners",
	})
	data := decodeBody(t, resp)
	if data["name"] != "banners" {
		t.Errorf("create_dir name = %v; want banners", data["name"])
	}

	// Root listing shows the sub-directory and no images.
	resp = env.get(t, "/api/carrd/images?folder=client")
	data = decodeBody(t, resp)
	dirs, _ := data["dirs"].([]any)
	if len(dirs) != 1 || dirs[0] != "spring-sale" {
		t.Fatalf("root dirs = %v; want [spring-sale]", dirs)
	}

	// Upload into the nested dir.
	resp = env.postCarrdUpload(t, "client", "spring-sale/banners", map[string][]byte{
		"top.png": []byte("\x89PNG\r\n\x1a\nimg"),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("nested upload status = %d; want 200", resp.StatusCode)
	}

	// The image is listed at the nested path…
	resp = env.get(t, "/api/carrd/images?folder=client&path=spring-sale/banners")
	data = decodeBody(t, resp)
	images, _ := data["images"].([]any)
	if len(images) != 1 || images[0].(map[string]any)["name"] != "top.png" {
		t.Fatalf("nested images = %v; want [top.png]", images)
	}

	// …and the project's recursive stats reflect it: 1 file and the two nested
	// sub-folders (spring-sale and its banners child).
	resp = env.get(t, "/api/carrd/projects")
	data = decodeBody(t, resp)
	projects, _ := data["projects"].([]any)
	proj := projects[0].(map[string]any)
	if proj["file_count"] != float64(1) {
		t.Errorf("file_count = %v; want 1 (recursive)", proj["file_count"])
	}
	if proj["subfolder_count"] != float64(2) {
		t.Errorf("subfolder_count = %v; want 2 (spring-sale + banners)", proj["subfolder_count"])
	}
	if proj["total_size"].(float64) <= 0 {
		t.Errorf("total_size = %v; want > 0", proj["total_size"])
	}

	// Delete the nested image with its path (query params, 204).
	resp = env.del(t, "/api/carrd/images?folder=client&path="+url.QueryEscape("spring-sale/banners")+"&name=top.png")
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete nested image status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete the whole sub-tree (spring-sale and its banners child), 204.
	resp = env.del(t, "/api/carrd/images/dirs?folder=client&path=spring-sale")
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete_dir status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()
	resp = env.get(t, "/api/carrd/images?folder=client")
	data = decodeBody(t, resp)
	dirs, _ = data["dirs"].([]any)
	if len(dirs) != 0 {
		t.Errorf("dirs after delete_dir = %v; want empty", dirs)
	}
}

func TestCarrd_CreateDir_Duplicate(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Dup", "folder": "dup",
	}).Body.Close()
	env.postJSON(t, "/api/carrd/images/dirs", map[string]any{
		"folder": "dup", "name": "sub",
	}).Body.Close()

	resp := env.postJSON(t, "/api/carrd/images/dirs", map[string]any{
		"folder": "dup", "name": "sub",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCarrd_DeleteDir_RejectsRoot(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Root", "folder": "rootp",
	}).Body.Close()

	// Empty path = project root: must be refused (use the projects endpoint).
	resp := env.del(t, "/api/carrd/images/dirs?folder=rootp&path=")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCarrd_Path_TraversalRejected(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/carrd/projects", map[string]any{
		"title": "Trav", "folder": "trav",
	}).Body.Close()

	resp := env.get(t, "/api/carrd/images?folder=trav&path=..%2F..%2Fetc")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("list status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	resp = env.postCarrdUpload(t, "trav", "../escape", map[string][]byte{"a.png": []byte("x")})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("upload status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// A traversal path on the delete-image endpoint is also rejected (400).
	resp = env.del(t, "/api/carrd/images?folder=trav&path=..%2F..%2Fetc&name=a.png")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("delete status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// postCarrdUpload posts a multipart upload of name→content files to a path
// within a project folder via /api/carrd/upload (path "" = project root).
func (e *testEnv) postCarrdUpload(t *testing.T, folder, path string, files map[string][]byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("folder", folder); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("path", path); err != nil {
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
	resp, err := e.client.Post(e.url("/api/carrd/upload"), mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
