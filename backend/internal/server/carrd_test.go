package server_test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testCarrdProject struct {
	Title          string `json:"title"`
	Folder         string `json:"folder"`
	FileCount      int    `json:"fileCount"`
	SubfolderCount int    `json:"subfolderCount"`
	TotalSize      int64  `json:"totalSize"`
}

type testCarrdFolder struct {
	Folder  string   `json:"folder"`
	Path    string   `json:"path"`
	Folders []string `json:"folders"`
	Files   []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	} `json:"files"`
}

type testCarrdUpload struct {
	Uploaded []string `json:"uploaded"`
	Skipped  []struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"skipped"`
}

func (e *testEnv) carrdProject(t *testing.T, title, folder string) testCarrdProject {
	t.Helper()
	body := map[string]any{"title": title}
	if folder != "" {
		body["folder"] = folder
	}
	var p testCarrdProject
	decodeInto(t, e.postJSON(t, "/api/v1/carrd-projects", body), http.StatusCreated, &p)
	return p
}

func (e *testEnv) carrdFolder(t *testing.T, folder, path string) testCarrdFolder {
	t.Helper()
	var f testCarrdFolder
	decodeInto(t, e.get(t, "/api/v1/carrd-projects/"+folder+"/files?path="+url.QueryEscape(path)), http.StatusOK, &f)
	return f
}

func (e *testEnv) uploadCarrd(t *testing.T, folder, path string, named map[string][]byte) *http.Response {
	t.Helper()
	return e.upload(t, "/api/v1/carrd-projects/"+folder+"/files", map[string]string{"path": path}, files(named)...)
}

func TestCarrd_Guards(t *testing.T) {
	env := newTestEnv(t)
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/v1/carrd-projects", nil},
		{http.MethodPost, "/api/v1/carrd-projects", map[string]any{"title": "X"}},
		{http.MethodDelete, "/api/v1/carrd-projects/x", nil},
		{http.MethodGet, "/api/v1/carrd-projects/x/files", nil},
		{http.MethodPost, "/api/v1/carrd-projects/x/folders", map[string]any{"name": "y"}},
	} {
		resp := env.reqJSON(t, tc.method, tc.path, tc.body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s signed out = %d; want 401", tc.method, tc.path, resp.StatusCode)
		}
	}
	user := makeActiveUser(t, env, "images", "password123", []string{"system-images"})
	resp := reqAs(t, user, env, http.MethodGet, "/api/v1/carrd-projects", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("an account without the Carrd page = %d; want 403", resp.StatusCode)
	}
	granted := makeActiveUser(t, env, "carrd", "password123", []string{"atelier-carrd"})
	resp = reqAs(t, granted, env, http.MethodGet, "/api/v1/carrd-projects", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("an account with the Carrd page = %d; want 200", resp.StatusCode)
	}
}

func TestCarrd_Projects(t *testing.T) {
	webRoot := t.TempDir()
	env := newTestEnvWithWebRoot(t, webRoot)
	env.loginAdmin(t)

	p := env.carrdProject(t, "My Cool Project!", "")
	if p.Folder != "my-cool-project" || p.Title != "My Cool Project!" {
		t.Errorf("created %+v; want the folder derived from the title", p)
	}
	if p := env.carrdProject(t, "Spring Sale", "Spring 2026 Promo"); p.Folder != "spring-2026-promo" {
		t.Errorf("folder = %q; want it normalized", p.Folder)
	}
	if _, err := os.Stat(filepath.Join(webRoot, "carrd", "my-cool-project", ".carrd.json")); err != nil {
		t.Errorf("no title sidecar: %v", err)
	}

	for name, body := range map[string]map[string]any{
		"the same title in another case": {"title": "my cool PROJECT!", "folder": "other"},
		"the same folder":                {"title": "Another", "folder": "my-cool-project"},
	} {
		resp := env.postJSON(t, "/api/v1/carrd-projects", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("creating with %s = %d; want 409", name, resp.StatusCode)
		}
	}
	for name, body := range map[string]map[string]any{
		"a blank title":            {"title": "   "},
		"nothing to make a folder": {"title": "***"},
	} {
		resp := env.postJSON(t, "/api/v1/carrd-projects", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("creating with %s = %d; want 422", name, resp.StatusCode)
		}
	}

	var list struct {
		Projects []testCarrdProject `json:"projects"`
	}
	decodeInto(t, env.get(t, "/api/v1/carrd-projects"), http.StatusOK, &list)
	if len(list.Projects) != 2 || list.Projects[0].Title != "My Cool Project!" {
		t.Errorf("projects = %+v; want two, by title", list.Projects)
	}

	// Retitling keeps the folder; a new folder renames it; blank keeps either.
	var got testCarrdProject
	decodeInto(t, env.patchJSON(t, "/api/v1/carrd-projects/my-cool-project", map[string]any{"title": "Renamed"}), http.StatusOK, &got)
	if got.Title != "Renamed" || got.Folder != "my-cool-project" {
		t.Errorf("retitled %+v", got)
	}
	decodeInto(t, env.patchJSON(t, "/api/v1/carrd-projects/my-cool-project", map[string]any{"title": "", "folder": "New Name"}), http.StatusOK, &got)
	if got.Title != "Renamed" || got.Folder != "new-name" {
		t.Errorf("moved %+v; want the title kept and the folder renamed", got)
	}
	if _, err := os.Stat(filepath.Join(webRoot, "carrd", "new-name")); err != nil {
		t.Errorf("the folder wasn't renamed: %v", err)
	}
	for name, tc := range map[string]struct {
		path string
		body map[string]any
		want int
	}{
		"an unknown project":      {"/api/v1/carrd-projects/nope", map[string]any{"title": "X"}, http.StatusNotFound},
		"a taken title":           {"/api/v1/carrd-projects/new-name", map[string]any{"title": "spring sale"}, http.StatusConflict},
		"a taken folder":          {"/api/v1/carrd-projects/new-name", map[string]any{"folder": "spring-2026-promo"}, http.StatusConflict},
		"nothing to change":       {"/api/v1/carrd-projects/new-name", map[string]any{}, http.StatusUnprocessableEntity},
		"a folder that is a path": {"/api/v1/carrd-projects/..%2Fx", map[string]any{"title": "X"}, http.StatusNotFound},
	} {
		resp := env.patchJSON(t, tc.path, tc.body)
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("updating %s = %d; want %d", name, resp.StatusCode, tc.want)
		}
	}

	resp := env.del(t, "/api/v1/carrd-projects/new-name")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d; want 204", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(webRoot, "carrd", "new-name")); !os.IsNotExist(err) {
		t.Errorf("the folder survived its project: %v", err)
	}
	for _, folder := range []string{"new-name", "..%2F..%2Fx", "Bad_Folder"} {
		resp = env.del(t, "/api/v1/carrd-projects/"+folder)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("deleting %q = %d; want 404", folder, resp.StatusCode)
		}
	}
}

func TestCarrd_FilesFoldersAndUploads(t *testing.T) {
	webRoot := t.TempDir()
	env := newTestEnvWithWebRoot(t, webRoot)
	env.loginAdmin(t)
	env.carrdProject(t, "Site", "site")

	mp3 := append([]byte("ID3\x04\x00\x00"), make([]byte, 32)...)
	mp4 := append([]byte("\x00\x00\x00\x18ftypmp42"), make([]byte, 32)...)
	var up testCarrdUpload
	decodeInto(t, env.uploadCarrd(t, "site", "", map[string][]byte{
		"hero.png":    realPNG(t, 4, 4),
		"song.mp3":    mp3,
		"clip.mp4":    mp4,
		"logo.svg":    []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
		"fake.png":    []byte("<html></html>"),
		"fake.mp3":    []byte("<html></html>"),
		"fake.mp4":    []byte("<html></html>"),
		"readme.txt":  []byte("hi"),
		".carrd.json": []byte(`{"title":"hijacked"}`),
	}), http.StatusCreated, &up)
	if len(up.Uploaded) != 3 || len(up.Skipped) != 6 {
		t.Fatalf("upload = %+v; want hero, song and clip saved and six refused", up)
	}
	reasons := map[string]string{}
	for _, s := range up.Skipped {
		reasons[s.Name] = s.Reason
	}
	if reasons["fake.png"] != "Not a valid image" || reasons["fake.mp3"] != "Not a valid audio file" ||
		reasons["fake.mp4"] != "Not a valid video file" || !strings.HasPrefix(reasons["logo.svg"], "Unsupported type") {
		t.Errorf("reasons = %v", reasons)
	}
	// The sidecar can't be replaced through an upload.
	sidecar, _ := os.ReadFile(filepath.Join(webRoot, "carrd", "site", ".carrd.json"))
	if !strings.Contains(string(sidecar), "Site") {
		t.Errorf("sidecar = %s; want the project's title", sidecar)
	}

	root := env.carrdFolder(t, "site", "")
	if len(root.Files) != 3 || root.Files[0].Name != "clip.mp4" || len(root.Folders) != 0 || root.Path != "" {
		t.Errorf("root = %+v; want three files by name", root)
	}

	// Sub-folders: made, uploaded into, listed, counted, deleted.
	var made struct {
		Name string `json:"name"`
	}
	decodeInto(t, env.postJSON(t, "/api/v1/carrd-projects/site/folders", map[string]any{"path": "", "name": "Spring Art"}), http.StatusCreated, &made)
	if made.Name != "spring-art" {
		t.Errorf("made %q; want it normalized", made.Name)
	}
	decodeInto(t, env.postJSON(t, "/api/v1/carrd-projects/site/folders", map[string]any{"path": "spring-art", "name": "nested"}), http.StatusCreated, &made)
	env.uploadCarrd(t, "site", "spring-art/nested", map[string][]byte{"deep.png": realPNG(t, 2, 2)}).Body.Close()
	if f := env.carrdFolder(t, "site", "spring-art"); len(f.Folders) != 1 || f.Folders[0] != "nested" || f.Path != "spring-art" {
		t.Errorf("spring-art = %+v", f)
	}
	var list struct {
		Projects []testCarrdProject `json:"projects"`
	}
	decodeInto(t, env.get(t, "/api/v1/carrd-projects"), http.StatusOK, &list)
	if p := list.Projects[0]; p.FileCount != 4 || p.SubfolderCount != 2 {
		t.Errorf("project = %+v; want counts across the tree (4 files, 2 folders)", p)
	}
	for name, tc := range map[string]struct {
		body map[string]any
		want int
	}{
		"a folder that exists":   {map[string]any{"path": "", "name": "spring-art"}, http.StatusConflict},
		"a missing parent":       {map[string]any{"path": "nope", "name": "x"}, http.StatusNotFound},
		"a traversing parent":    {map[string]any{"path": "../..", "name": "x"}, http.StatusUnprocessableEntity},
		"a name with no letters": {map[string]any{"path": "", "name": "***"}, http.StatusUnprocessableEntity},
	} {
		resp := env.postJSON(t, "/api/v1/carrd-projects/site/folders", tc.body)
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("making a folder with %s = %d; want %d", name, resp.StatusCode, tc.want)
		}
	}

	// A same-named upload replaces the file.
	env.uploadCarrd(t, "site", "", map[string][]byte{"hero.png": realPNG(t, 40, 40)}).Body.Close()
	if f := env.carrdFolder(t, "site", ""); len(f.Files) != 3 {
		t.Errorf("after a replace the root has %d files; want still 3", len(f.Files))
	}

	// Uploading to a folder that isn't there, and failing the whole batch.
	resp := env.uploadCarrd(t, "site", "missing", map[string][]byte{"a.png": realPNG(t, 1, 1)})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("upload to a missing folder = %d; want 404", resp.StatusCode)
	}
	resp = env.uploadCarrd(t, "nope", "", map[string][]byte{"a.png": realPNG(t, 1, 1)})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("upload to a missing project = %d; want 404", resp.StatusCode)
	}
	resp = env.uploadCarrd(t, "site", "../..", map[string][]byte{"a.png": realPNG(t, 1, 1)})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("upload to a traversing path = %d; want 422", resp.StatusCode)
	}
	if locs := problemLocations(t, env.uploadCarrd(t, "site", "", map[string][]byte{"x.txt": []byte("x")})); len(locs) != 1 || locs[0] != "body.files[0]" {
		t.Errorf("locations = %v; want body.files[0]", locs)
	}

	// Files deleted by name within a folder.
	del := func(folder, path, name string) int {
		resp := env.del(t, "/api/v1/carrd-projects/"+folder+"/files?path="+url.QueryEscape(path)+"&name="+url.QueryEscape(name))
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := del("site", "", "hero.png"); code != http.StatusNoContent {
		t.Errorf("delete file = %d; want 204", code)
	}
	for name, tc := range map[string]struct {
		folder, path, file string
		want               int
	}{
		"a file already gone": {"site", "", "hero.png", http.StatusNotFound},
		"a traversing name":   {"site", "", "../../secret.png", http.StatusNotFound},
		"the sidecar":         {"site", "", ".carrd.json", http.StatusNotFound},
		"a traversing path":   {"site", "../..", "a.png", http.StatusUnprocessableEntity},
		"a missing project":   {"nope", "", "a.png", http.StatusNotFound},
	} {
		if code := del(tc.folder, tc.path, tc.file); code != tc.want {
			t.Errorf("deleting %s = %d; want %d", name, code, tc.want)
		}
	}

	// Sub-folders deleted with their contents; the root and unknown ones are refused.
	delDir := func(path string) int {
		resp := env.del(t, "/api/v1/carrd-projects/site/folders?path="+url.QueryEscape(path))
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := delDir(""); code != http.StatusUnprocessableEntity {
		t.Errorf("deleting the root = %d; want 422", code)
	}
	if code := delDir("../.."); code != http.StatusUnprocessableEntity {
		t.Errorf("deleting a traversing path = %d; want 422", code)
	}
	if code := delDir("nope"); code != http.StatusNotFound {
		t.Errorf("deleting an unknown folder = %d; want 404", code)
	}
	if code := delDir("spring-art"); code != http.StatusNoContent {
		t.Errorf("deleting a folder = %d; want 204", code)
	}
	if _, err := os.Stat(filepath.Join(webRoot, "carrd", "site", "spring-art", "nested", "deep.png")); !os.IsNotExist(err) {
		t.Errorf("a deleted folder's files survived: %v", err)
	}
	resp = env.get(t, "/api/v1/carrd-projects/site/files?path=gone")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("listing a missing folder = %d; want 404", resp.StatusCode)
	}
	resp = env.get(t, "/api/v1/carrd-projects/site/files?path=..%2F..")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("listing a traversing path = %d; want 422", resp.StatusCode)
	}
}
