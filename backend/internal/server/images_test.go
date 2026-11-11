package server_test

import (
	"bytes"
	"image/jpeg"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"app-suite/internal/api/apitest"
	"app-suite/internal/live"
	"app-suite/internal/server"
	"app-suite/internal/store"
	"app-suite/internal/store/storetest"
)

// newTestEnvWithDataDir builds a test env backed by a caller-provided dataDir, so
// a test can pre-seed files before the server (and its startup migration) run.
func newTestEnvWithDataDir(t *testing.T, dataDir string) *testEnv {
	t.Helper()
	st, err := store.New(t.Context(), storetest.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	srv := server.New(t.Context(), st, live.New(), dataDir, nil)
	ts := httptest.NewTLSServer(srv)
	t.Cleanup(ts.Close)

	jar, _ := cookiejar.New(nil)
	client := ts.Client()
	client.Jar = jar
	client.Transport = apitest.Transport(t, client.Transport)
	return &testEnv{ts: ts, client: client, store: st, srv: srv}
}

// newTestEnvWithStore is newTestEnvWithDataDir with settings stored before the server
// starts, for the startup migrations that read them.
func newTestEnvWithStore(t *testing.T, dataDir string, seed func(set func(key, value string))) *testEnv {
	t.Helper()
	st, err := store.New(t.Context(), storetest.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	seed(func(key, value string) {
		if err := st.SetSetting(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	})

	srv := server.New(t.Context(), st, live.New(), dataDir, nil)
	ts := httptest.NewTLSServer(srv)
	t.Cleanup(ts.Close)

	jar, _ := cookiejar.New(nil)
	client := ts.Client()
	client.Jar = jar
	client.Transport = apitest.Transport(t, client.Transport)
	return &testEnv{ts: ts, client: client, store: st, srv: srv}
}

type testImage struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

type testImageUpload struct {
	Uploaded []testImage `json:"uploaded"`
	Skipped  []struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"skipped"`
}

type testCategory struct {
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	FileCount int    `json:"fileCount"`
	TotalSize int64  `json:"totalSize"`
}

// categories lists the library's categories by dir.
func (e *testEnv) categories(t *testing.T) map[string]testCategory {
	t.Helper()
	var out struct {
		Categories []testCategory `json:"categories"`
	}
	decodeInto(t, e.get(t, "/api/v1/image-categories"), http.StatusOK, &out)
	byDir := map[string]testCategory{}
	for _, c := range out.Categories {
		byDir[c.Dir] = c
	}
	return byDir
}

// imageNames lists the names of a category's pictures, newest first.
func (e *testEnv) imageNames(t *testing.T, dir string) []string {
	t.Helper()
	var out struct {
		Dir    string      `json:"dir"`
		Images []testImage `json:"images"`
	}
	decodeInto(t, e.get(t, "/api/v1/image-categories/"+dir+"/images"), http.StatusOK, &out)
	names := make([]string, 0, len(out.Images))
	for _, im := range out.Images {
		names = append(names, im.Name)
	}
	return names
}

func (e *testEnv) uploadImages(t *testing.T, dir string, named map[string][]byte) *http.Response {
	t.Helper()
	return e.upload(t, "/api/v1/image-categories/"+dir+"/images", nil, files(named)...)
}

func TestImages_Guards(t *testing.T) {
	env := newTestEnv(t)

	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/v1/image-categories", nil},
		{http.MethodPost, "/api/v1/image-categories", map[string]any{"name": "X"}},
		{http.MethodGet, "/api/v1/image-categories/raffles/images", nil},
		{http.MethodDelete, "/api/v1/image-categories/raffles", nil},
	} {
		resp := env.reqJSON(t, tc.method, tc.path, tc.body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s signed out = %d; want 401", tc.method, tc.path, resp.StatusCode)
		}
	}
	resp := env.upload(t, "/api/v1/image-categories/raffles/images", nil, files(map[string][]byte{"a.png": realPNG(t, 2, 2)})...)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("upload signed out = %d; want 401", resp.StatusCode)
	}

	// Every page with a picture field may read the library, and only Images may change it.
	for _, perm := range []string{"teahouse-raffles", "festival-ema", "festival-map"} {
		user := makeActiveUser(t, env, "reader-"+perm, "password123", []string{perm})
		resp := reqAs(t, user, env, http.MethodGet, "/api/v1/image-categories", nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s reading the library = %d; want 200", perm, resp.StatusCode)
		}
		resp = reqAs(t, user, env, http.MethodPost, "/api/v1/image-categories", map[string]any{"name": "Nope"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s changing the library = %d; want 403", perm, resp.StatusCode)
		}
		resp = uploadAs(t, user, env, "/api/v1/image-categories/raffles/images", nil, files(map[string][]byte{"a.png": realPNG(t, 2, 2)})...)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s uploading = %d; want 403", perm, resp.StatusCode)
		}
	}
	noPictures := makeActiveUser(t, env, "tea", "password123", []string{"bingo-cards"})
	resp = reqAs(t, noPictures, env, http.MethodGet, "/api/v1/image-categories", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a page with no picture field reading the library = %d; want 403", resp.StatusCode)
	}
}

func TestImages_DefaultCategories(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	cats := env.categories(t)
	for _, dir := range []string{
		"announcements_main", "announcements_thumb", "raffles", "garapons", "flourishes",
		"affiliate_logos", "affiliate_images", "stamp_cards", "stamp_stamps", "stamp_prizes",
	} {
		if _, ok := cats[dir]; !ok {
			t.Errorf("default category %q missing", dir)
		}
	}
}

func TestImages_CategoryCRUD(t *testing.T) {
	dataDir := t.TempDir()
	env := newTestEnvWithDataDir(t, dataDir)
	env.loginAdmin(t)

	// The folder is derived from the name, or taken from the dir given.
	var cat testCategory
	decodeInto(t, env.postJSON(t, "/api/v1/image-categories", map[string]any{"name": "Event Banners"}), http.StatusCreated, &cat)
	if cat.Dir != "event_banners" || cat.Name != "Event Banners" {
		t.Errorf("created %+v; want event_banners", cat)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "images", "event_banners")); err != nil {
		t.Errorf("the folder wasn't made: %v", err)
	}
	decodeInto(t, env.postJSON(t, "/api/v1/image-categories", map[string]any{"name": "Misc", "dir": "My Own-Dir"}), http.StatusCreated, &cat)
	if cat.Dir != "my_own_dir" {
		t.Errorf("explicit dir = %q; want it normalized to my_own_dir", cat.Dir)
	}

	for name, body := range map[string]map[string]any{
		"the same dir":             {"name": "Other", "dir": "event_banners"},
		"the same name, any case":  {"name": "event BANNERS"},
		"a seeded dir":             {"name": "Mine", "dir": "raffles"},
		"the book-club covers":     {"name": "Covers", "dir": "bookclub"},
		"the legacy announcements": {"name": "Legacy", "dir": "announcements"},
		"the held plaques":         {"name": "Plaques", "dir": "ema_plaques"},
	} {
		resp := env.postJSON(t, "/api/v1/image-categories", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("creating %s = %d; want 409", name, resp.StatusCode)
		}
	}
	for name, body := range map[string]map[string]any{
		"a blank name":             {"name": "   "},
		"nothing to make a dir of": {"name": "***"},
	} {
		resp := env.postJSON(t, "/api/v1/image-categories", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("creating with %s = %d; want 422", name, resp.StatusCode)
		}
	}

	// Renaming keeps the folder unless a new one is asked for, so every stored
	// reference to a picture in it keeps working.
	env.uploadImages(t, "event_banners", map[string][]byte{"a.png": realPNG(t, 2, 2)}).Body.Close()
	decodeInto(t, env.patchJSON(t, "/api/v1/image-categories/event_banners", map[string]any{"name": "Festival Banners"}), http.StatusOK, &cat)
	if cat.Dir != "event_banners" || cat.Name != "Festival Banners" || cat.FileCount != 1 {
		t.Errorf("renamed %+v; want the folder kept and one file", cat)
	}
	decodeInto(t, env.patchJSON(t, "/api/v1/image-categories/event_banners", map[string]any{"name": "Festival Banners", "dir": ""}), http.StatusOK, &cat)
	if cat.Dir != "event_banners" {
		t.Errorf("a blank dir moved the folder to %q; it should keep it", cat.Dir)
	}
	decodeInto(t, env.patchJSON(t, "/api/v1/image-categories/event_banners", map[string]any{"dir": "banners"}), http.StatusOK, &cat)
	if cat.Dir != "banners" || cat.Name != "Festival Banners" {
		t.Errorf("moved %+v; want banners keeping the name", cat)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "images", "banners", "a.png")); err != nil {
		t.Errorf("the file didn't move with the folder: %v", err)
	}
	if names := env.imageNames(t, "banners"); len(names) != 1 {
		t.Errorf("banners = %v; want the moved file", names)
	}
	for name, tc := range map[string]struct {
		path string
		body map[string]any
		want int
	}{
		"an unknown category": {"/api/v1/image-categories/nope", map[string]any{"name": "X"}, http.StatusNotFound},
		"onto a seeded dir":   {"/api/v1/image-categories/banners", map[string]any{"dir": "raffles"}, http.StatusConflict},
		"onto a reserved dir": {"/api/v1/image-categories/banners", map[string]any{"dir": "bookclub"}, http.StatusConflict},
		"onto a taken name":   {"/api/v1/image-categories/banners", map[string]any{"name": "raffle"}, http.StatusConflict},
		"a blank name":        {"/api/v1/image-categories/banners", map[string]any{"name": "  "}, http.StatusUnprocessableEntity},
		"nothing to change":   {"/api/v1/image-categories/banners", map[string]any{}, http.StatusUnprocessableEntity},
	} {
		resp := env.patchJSON(t, tc.path, tc.body)
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("renaming %s = %d; want %d", name, resp.StatusCode, tc.want)
		}
	}

	// Deleting removes the folder and its files; an unknown category is a 404.
	resp := env.del(t, "/api/v1/image-categories/banners")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d; want 204", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "images", "banners")); !os.IsNotExist(err) {
		t.Errorf("the folder survived its category: %v", err)
	}
	resp = env.del(t, "/api/v1/image-categories/banners")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("deleting an unknown category = %d; want 404", resp.StatusCode)
	}
	resp = env.get(t, "/api/v1/image-categories/banners/images")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("listing an unknown category = %d; want 404", resp.StatusCode)
	}
	// A name that could climb out of the images folder is just not a category.
	resp = env.del(t, "/api/v1/image-categories/..%2Fimages")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("deleting a traversal = %d; want 404", resp.StatusCode)
	}
}

func TestImages_UploadListDelete(t *testing.T) {
	dataDir := t.TempDir()
	env := newTestEnvWithDataDir(t, dataDir)
	env.loginAdmin(t)

	var up testImageUpload
	decodeInto(t, env.uploadImages(t, "raffles", map[string][]byte{
		"first.png":      realPNG(t, 4, 3),
		"my picture.gif": smallGIF(t),
		"notes.txt":      []byte("hello"),
		"fake.png":       []byte("<html><script>alert(1)</script></html>"),
	}), http.StatusCreated, &up)
	if len(up.Uploaded) != 2 || len(up.Skipped) != 2 {
		t.Fatalf("upload = %+v; want 2 saved and 2 skipped", up)
	}
	skipped := map[string]string{}
	for _, s := range up.Skipped {
		skipped[s.Name] = s.Reason
	}
	if !strings.HasPrefix(skipped["notes.txt"], "Unsupported type") || skipped["fake.png"] != "Not a valid image" {
		t.Errorf("skipped = %v", skipped)
	}
	for _, im := range up.Uploaded {
		if im.Name == "my picture.gif" {
			if im.Path != "images/raffles/my picture.gif" || !strings.HasSuffix(im.URL, "/images/raffles/my%20picture.gif") ||
				!strings.HasPrefix(im.URL, "https://") {
				t.Errorf("image = %+v; want an absolute, path-escaped address", im)
			}
		}
	}
	if names := env.imageNames(t, "raffles"); len(names) != 2 {
		t.Errorf("listed = %v; want the two saved", names)
	}
	if env.categories(t)["raffles"].FileCount != 2 {
		t.Errorf("category = %+v; want two files", env.categories(t)["raffles"])
	}

	// A same-named upload replaces the file.
	bigger := realPNG(t, 40, 30)
	env.uploadImages(t, "raffles", map[string][]byte{"first.png": bigger}).Body.Close()
	after, err := os.ReadFile(filepath.Join(dataDir, "images", "raffles", "first.png"))
	if err != nil || len(after) == 0 {
		t.Fatal(err)
	}
	if names := env.imageNames(t, "raffles"); len(names) != 2 {
		t.Errorf("listed after a replace = %v; want still two", names)
	}

	// Deleting: a file, then one that is gone, then names that aren't files there.
	resp := env.del(t, "/api/v1/image-categories/raffles/images/first.png")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d; want 204", resp.StatusCode)
	}
	for _, name := range []string{"first.png", "notes.txt", "..%2F..%2Fsecret.png", ".categories.json"} {
		resp = env.del(t, "/api/v1/image-categories/raffles/images/"+name)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("deleting %q = %d; want 404", name, resp.StatusCode)
		}
	}
}

func TestImages_UploadRefusals(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Nothing saved is a 422 naming each refused file.
	locs := problemLocations(t, env.uploadImages(t, "raffles", map[string][]byte{"a.txt": []byte("x")}))
	if len(locs) != 1 || locs[0] != "body.files[0]" {
		t.Errorf("locations = %v; want body.files[0]", locs)
	}
	locs = problemLocations(t, env.upload(t, "/api/v1/image-categories/raffles/images", nil))
	if len(locs) != 1 || locs[0] != "body.files" {
		t.Errorf("no files: locations = %v; want body.files", locs)
	}
	var many []filePart
	for i := range 51 {
		many = append(many, filePart{field: "files", name: "a" + string(rune('a'+i%26)) + ".png", data: realPNG(t, 1, 1)})
	}
	resp := env.upload(t, "/api/v1/image-categories/raffles/images", nil, many...)
	if locs := problemLocations(t, resp); len(locs) != 1 || locs[0] != "body.files" {
		t.Errorf("51 files: locations = %v; want body.files", locs)
	}

	resp = env.uploadImages(t, "nope", map[string][]byte{"a.png": realPNG(t, 1, 1)})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("upload to an unknown category = %d; want 404", resp.StatusCode)
	}

	// A picture over 40 megapixels is refused from its header.
	huge := realPNG(t, 7000, 7000)
	locs = problemLocations(t, env.uploadImages(t, "raffles", map[string][]byte{"huge.png": huge}))
	if len(locs) != 1 {
		t.Errorf("a 49-megapixel picture: locations = %v; want it refused", locs)
	}

	// A body over the cap is a 413 before it is read.
	big := bytes.Repeat([]byte{0}, 66<<20)
	resp = env.uploadImages(t, "raffles", map[string][]byte{"big.png": big})
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("a 66 MB body = %d; want 413", resp.StatusCode)
	}
}

// TestImages_RasterIsSavedAgain checks the stored bytes: a JPEG or PNG loses its
// metadata (a JPEG is turned upright by its orientation first), a GIF is kept as sent.
func TestImages_RasterIsSavedAgain(t *testing.T) {
	dataDir := t.TempDir()
	env := newTestEnvWithDataDir(t, dataDir)
	env.loginAdmin(t)

	gifData := smallGIF(t)
	env.uploadImages(t, "raffles", map[string][]byte{
		"phone.jpg": exifJPEG(t, 8, 4, 6),
		"kept.gif":  gifData,
	}).Body.Close()

	jpg, err := os.ReadFile(filepath.Join(dataDir, "images", "raffles", "phone.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(jpg, []byte("Exif")) {
		t.Error("the stored JPEG still has its EXIF")
	}
	// Rotated a quarter turn: 8 wide by 4 high becomes 4 by 8.
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpg))
	if err != nil || cfg.Width != 4 || cfg.Height != 8 {
		t.Errorf("stored JPEG is %+v, %v; want 4x8 (turned upright)", cfg, err)
	}
	kept, err := os.ReadFile(filepath.Join(dataDir, "images", "raffles", "kept.gif"))
	if err != nil || !bytes.Equal(kept, gifData) {
		t.Errorf("the GIF should be kept as sent: %v", err)
	}
}

func TestImages_SVGUploadPersistsTheSanitizedBytes(t *testing.T) {
	dataDir := t.TempDir()
	env := newTestEnvWithDataDir(t, dataDir)
	env.loginAdmin(t)

	// A valid SVG carrying the things the sanitizer is there to strip.
	hostile := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1">` +
		`<script>alert(1)</script>` +
		`<a href="javascript:alert(2)"><path d="M0 0h1v1H0z" onload="alert(3)"/></a>` +
		`</svg>`)

	var up testImageUpload
	decodeInto(t, env.uploadImages(t, "flourishes", map[string][]byte{"evil.svg": hostile, "broken.svg": []byte("<svg><")}),
		http.StatusCreated, &up)
	if len(up.Uploaded) != 1 || len(up.Skipped) != 1 || up.Skipped[0].Reason != "Not a valid SVG" {
		t.Fatalf("upload = %+v; want the SVG saved and the broken one refused", up)
	}

	stored, err := os.ReadFile(filepath.Join(dataDir, "images", "flourishes", "evil.svg"))
	if err != nil {
		t.Fatalf("read stored svg: %v", err)
	}
	got := string(stored)
	for _, banned := range []string{"<script", "javascript:", "onload"} {
		if strings.Contains(strings.ToLower(got), banned) {
			t.Errorf("stored SVG still contains %q - the raw upload was persisted, not the sanitized output:\n%s", banned, got)
		}
	}
	if !strings.Contains(got, "<svg") || !strings.Contains(got, "<path") {
		t.Errorf("sanitized SVG lost its drawable content: %s", got)
	}
}

func TestImages_SeedFlourishes(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, dataDir, "images/corner_flourish.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0"/></svg>`))
	writeFile(t, dataDir, "images/called_flourish.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M1 1"/></svg>`))

	env := newTestEnvWithDataDir(t, dataDir)
	env.loginAdmin(t)
	got := map[string]bool{}
	for _, n := range env.imageNames(t, "flourishes") {
		got[n] = true
	}
	if !got["corner_flourish.svg"] || !got["called_flourish.svg"] {
		t.Errorf("seeded flourishes = %v; want both built-ins", got)
	}
}

// TestImages_Migration verifies the one-time copy of the old images/announcements
// files into the announcements_main category on server start.
func TestImages_Migration(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, dataDir, "images/announcements/legacy.png", realPNG(t, 2, 2))

	env := newTestEnvWithDataDir(t, dataDir)
	env.loginAdmin(t)

	if names := env.imageNames(t, "announcements_main"); len(names) != 1 || names[0] != "legacy.png" {
		t.Errorf("announcements_main = %v; want [legacy.png] migrated from the old folder", names)
	}
}

// TestImages_ManifestMigration verifies a manifest from before the defaults were
// ordinary categories gets them folded in, keeping its own entries.
func TestImages_ManifestMigration(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, dataDir, "images/.categories.json", []byte(`{"categories":[{"name":"Event Banners","dir":"event_banners"}]}`))

	env := newTestEnvWithDataDir(t, dataDir)
	env.loginAdmin(t)

	cats := env.categories(t)
	for _, want := range []string{"raffles", "announcements_main", "event_banners"} {
		if _, ok := cats[want]; !ok {
			t.Errorf("category %q missing after the manifest migration (got %v)", want, cats)
		}
	}
}

// TestImages_ManifestMigration_RunsOnce verifies the migration doesn't bring back a
// default staff deleted: a second start over the same data folder leaves the manifest be.
func TestImages_ManifestMigration_RunsOnce(t *testing.T) {
	dataDir := t.TempDir()

	env := newTestEnvWithDataDir(t, dataDir)
	env.loginAdmin(t)
	resp := env.del(t, "/api/v1/image-categories/raffles")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d; want 204", resp.StatusCode)
	}

	env2 := newTestEnvWithDataDir(t, dataDir)
	env2.loginAdmin(t)
	if _, ok := env2.categories(t)["raffles"]; ok {
		t.Error("a deleted default category came back after a restart")
	}
}

// TestImages_ManifestMigration_CorruptNotWiped verifies a manifest that doesn't parse
// is left alone rather than reseeded, which would silently drop any custom categories.
func TestImages_ManifestMigration_CorruptNotWiped(t *testing.T) {
	dataDir := t.TempDir()
	corrupt := []byte(`{"version":2,"categories":[{"name":"Keep",`)
	writeFile(t, dataDir, "images/.categories.json", corrupt)

	_ = newTestEnvWithDataDir(t, dataDir)

	got, err := os.ReadFile(filepath.Join(dataDir, "images", ".categories.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, corrupt) {
		t.Errorf("the corrupt manifest was modified; want it left untouched.\n got: %s", got)
	}
}
