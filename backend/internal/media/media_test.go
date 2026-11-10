package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func write(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func kind(err error) Kind {
	if p := AsProblem(err); p != nil {
		return p.Kind
	}
	return 0
}

type memSettings struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSettings) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[key], nil
}

func (s *memSettings) Set(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[key] = value
	return nil
}

func TestCleanPath(t *testing.T) {
	for in, want := range map[string]string{
		"":          "",
		"  ":        "",
		"/":         "",
		"a":         "a",
		"a/b":       "a/b",
		"/a/b/":     "a/b",
		`a\b`:       "a/b",
		"spring-26": "spring-26",
	} {
		if got, ok := CleanPath(in); !ok || got != want {
			t.Errorf("CleanPath(%q) = (%q, %v); want (%q, true)", in, got, ok, want)
		}
	}
	for _, in := range []string{"..", "../..", "a/../b", "a//b", ".hidden", "a/.carrd.json", "UPPER", "a b", "a_b", "%2e%2e", "~"} {
		if got, ok := CleanPath(in); ok {
			t.Errorf("CleanPath(%q) = (%q, true); want refused", in, got)
		}
	}
}

func TestCarrdPathsStayInTheProject(t *testing.T) {
	root := t.TempDir()
	c := NewCarrd(root)
	if _, err := c.CreateProject("Site", "site"); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "secret.txt"), "outside")
	write(t, filepath.Join(root, "carrd", "other", "keep.png"), "x")

	for _, folder := range []string{"..", "../site", "site/../other", "", "Site", "a/b"} {
		if _, err := c.Folder(folder, ""); kind(err) != NotFound {
			t.Errorf("Folder(%q) = %v; want not found", folder, err)
		}
		if err := c.DeleteProject(folder); kind(err) != NotFound {
			t.Errorf("DeleteProject(%q) = %v; want not found", folder, err)
		}
	}
	for _, rel := range []string{"..", "../other", "../../", "a/../..", ".."} {
		if _, err := c.Folder("site", rel); kind(err) != Invalid {
			t.Errorf("Folder(site, %q) = %v; want invalid", rel, err)
		}
		if _, err := c.Dest("site", rel); kind(err) != Invalid {
			t.Errorf("Dest(site, %q) = %v; want invalid", rel, err)
		}
		if err := c.DeleteFolder("site", rel); kind(err) != Invalid {
			t.Errorf("DeleteFolder(site, %q) = %v; want invalid", rel, err)
		}
		if err := c.DeleteFile("site", rel, "keep.png"); kind(err) != Invalid {
			t.Errorf("DeleteFile(site, %q) = %v; want invalid", rel, err)
		}
	}
	for _, name := range []string{"../secret.txt", "../../secret.txt", `..\secret.txt`, ".carrd.json", "notes.txt"} {
		if err := c.DeleteFile("site", "", name); kind(err) != NotFound {
			t.Errorf("DeleteFile(%q) = %v; want not found", name, err)
		}
	}
	if !exists(filepath.Join(root, "secret.txt")) || !exists(filepath.Join(root, "carrd", "other", "keep.png")) ||
		!exists(filepath.Join(root, "carrd", "site", ".carrd.json")) {
		t.Error("a refused path still deleted something")
	}
	if err := c.DeleteFolder("site", ""); kind(err) != Invalid {
		t.Errorf("deleting the project's root as a folder = %v; want invalid", err)
	}
}

func TestImagesRemoveCoverStaysInTheCoverFolder(t *testing.T) {
	root := t.TempDir()
	m := NewImages(root)
	write(t, filepath.Join(root, "images", "bookclub", "a.jpg"), "a")
	write(t, filepath.Join(root, "images", "bookclub", "b.jpg"), "b")
	write(t, filepath.Join(root, "images", "raffles", "prize.png"), "p")
	write(t, filepath.Join(root, "secret.txt"), "s")

	for _, u := range []string{
		"", "https://s4.anilist.co/cover.jpg",
		"https://h/images/raffles/prize.png",
		"https://h/images/bookclub/../../secret.txt",
		"images/bookclub/../raffles/prize.png",
		"https://h/images/bookclub/",
		"https://h/images/bookclub",
	} {
		m.RemoveCover(u)
	}
	if !exists(filepath.Join(root, "secret.txt")) || !exists(filepath.Join(root, "images", "raffles", "prize.png")) ||
		!exists(filepath.Join(root, "images", "bookclub", "b.jpg")) {
		t.Error("RemoveCover reached outside the covers folder")
	}
	m.RemoveCover("https://h/images/bookclub/a.jpg")
	m.RemoveCover("https://h/images/bookclub/missing.jpg")
	if exists(filepath.Join(root, "images", "bookclub", "a.jpg")) {
		t.Error("a cover inside the folder wasn't removed")
	}
}

func TestImagesSaveNeedsAKnownCategory(t *testing.T) {
	root := t.TempDir()
	m := NewImages(root)
	m.MigrateManifest()
	if err := m.Save("raffles", "a.png", []byte("x")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"nope", "..", "../x", "bookclub", "ema_plaques", ""} {
		if err := m.Save(dir, "a.png", []byte("x")); kind(err) != NotFound {
			t.Errorf("Save(%q) = %v; want not found", dir, err)
		}
	}
	if exists(filepath.Join(root, "images", "bookclub")) || exists(filepath.Join(root, "images", "ema_plaques")) {
		t.Error("a reserved folder was made through the library")
	}
	if !Reserved("ema_plaques") || !Reserved("bookclub") || !Reserved("announcements") || Reserved("raffles") {
		t.Error("the reserved folders are wrong")
	}
}

func TestFontsMetaFollowsRenameAndDelete(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	set := &memSettings{}
	f := NewFonts(root, set)
	write(t, filepath.Join(root, "fonts", "Old.ttf"), "not a font")
	write(t, filepath.Join(root, "fonts", "Other.ttf"), "not a font")

	family, serve := "Kept", "TTF"
	origins := []string{"https://mysite.carrd.co"}
	if err := f.Update(ctx, "Old", Changes{Family: &family, Serve: &serve, Origins: &origins}); err != nil {
		t.Fatal(err)
	}
	if err := f.RenameFile(ctx, "Old.ttf", "New.ttf"); err != nil {
		t.Fatal(err)
	}
	got, err := f.Get(ctx, "New", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Family != "Kept" || got.Serve != "TTF" || len(got.Origins) != 1 {
		t.Errorf("metadata did not follow the rename: %+v", got)
	}
	if err := f.RenameFile(ctx, "New.ttf", "Other.ttf"); kind(err) != Conflict {
		t.Errorf("renaming onto a file = %v; want a conflict", err)
	}
	if err := f.DeleteFont(ctx, "New"); err != nil {
		t.Fatal(err)
	}
	if m := f.MetaMap(ctx); len(m) != 0 {
		t.Errorf("metadata = %+v; want it gone with the font", m)
	}
	if err := f.DeleteFont(ctx, "New"); kind(err) != NotFound {
		t.Errorf("deleting twice = %v; want not found", err)
	}
}

func TestFontsSecretIsKept(t *testing.T) {
	ctx := context.Background()
	set := &memSettings{}
	a, err := NewFonts(t.TempDir(), set).Secret(ctx)
	if err != nil || len(a) != 32 {
		t.Fatalf("secret = %d bytes, %v", len(a), err)
	}
	// A new process reads the same key back, so published links keep working.
	b, err := NewFonts(t.TempDir(), set).Secret(ctx)
	if err != nil || string(a) != string(b) {
		t.Errorf("the secret changed between starts: %v", err)
	}
}

func TestFontsAddNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	f := NewFonts(root, &memSettings{})
	if _, err := f.Add("A.ttf", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Add("A.ttf", []byte("second")); !errors.Is(err, ErrExists) {
		t.Errorf("adding again = %v; want ErrExists", err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "fonts", "A.ttf")); string(b) != "first" {
		t.Errorf("the file was overwritten: %q", b)
	}
}
