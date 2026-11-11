package server_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func writeDataFile(t *testing.T, dataDir, rel, content string) {
	t.Helper()
	p := filepath.Join(dataDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The data folder's public files are the server's to serve, and the fonts never are:
// they are reachable only through their tokenized links.
func TestDataFolderServesItsPublicFilesAndNeverTheFonts(t *testing.T) {
	dataDir := t.TempDir()
	env := newTestEnvWithDataDir(t, dataDir)
	writeDataFile(t, dataDir, "images/logo.png", "logo")
	writeDataFile(t, dataDir, "carrd/site/a.png", "carrd")
	writeDataFile(t, dataDir, "plugin/pluginmaster.json", `[]`)
	writeDataFile(t, dataDir, "sounds/level_up.mp3", "mp3")
	writeDataFile(t, dataDir, "fonts/Jasper.ttf", "licensed")
	writeDataFile(t, dataDir, "fonts/.woff2/Jasper.woff2", "licensed")

	for path, want := range map[string]string{
		"/images/logo.png":          "logo",
		"/carrd/site/a.png":         "carrd",
		"/plugin/pluginmaster.json": "[]",
		"/sounds/level_up.mp3":      "mp3",
	} {
		resp := env.get(t, path)
		got, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK || string(got) != want {
			t.Errorf("%s answered %d %q, want 200 %q", path, resp.StatusCode, got, want)
		}
	}

	for _, path := range []string{
		"/fonts/Jasper.ttf",
		"/fonts/.woff2/Jasper.woff2",
		"/images/../fonts/Jasper.ttf",
		"/carrd/../fonts/Jasper.ttf",
		"/images/.categories.json",
		"/images/",
	} {
		resp := env.get(t, path)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("%s answered 200; it must not be served", path)
		}
	}
}

// The Carrd host proxies its whole site to /carrd/, and a page on any site may embed
// its files.
func TestCarrdFilesAnswerAnyOrigin(t *testing.T) {
	dataDir := t.TempDir()
	env := newTestEnvWithDataDir(t, dataDir)
	writeDataFile(t, dataDir, "carrd/site/clip.mp4", "mp4")
	resp := env.getWithHeaders(t, "/carrd/site/clip.mp4", map[string]string{"Origin": "https://someone.carrd.co"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("answered %d with Access-Control-Allow-Origin %q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
}
