package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"app-suite/internal/model"
	"app-suite/internal/server"
	"app-suite/internal/store"
	"app-suite/internal/ws"
)

// postFontsUpload posts a multipart upload of name→content font files.
func (e *testEnv) postFontsUpload(t *testing.T, files map[string][]byte) model.FontUploadResponse {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
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
	resp, err := e.client.Post(e.url("/api/fonts/upload"), mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /api/fonts/upload = %d: %s", resp.StatusCode, body)
	}
	var out model.FontUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// listFonts fetches GET /api/fonts and returns the decoded response.
func (e *testEnv) listFonts(t *testing.T) model.FontsResponse {
	t.Helper()
	resp := e.get(t, "/api/fonts")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/fonts = %d", resp.StatusCode)
	}
	var out model.FontsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// fontByBase finds a listed font by base name, failing the test when absent.
func (e *testEnv) fontByBase(t *testing.T, base string) model.Font {
	t.Helper()
	for _, f := range e.listFonts(t).Fonts {
		if f.Base == base {
			return f
		}
	}
	t.Fatalf("font %q not in listing", base)
	return model.Font{}
}

// variantByType finds a font's variant by type label (nil when absent).
func variantByType(f model.Font, typ string, converted bool) *model.FontVariant {
	for i := range f.Variants {
		if f.Variants[i].Type == typ && f.Variants[i].Converted == converted {
			return &f.Variants[i]
		}
	}
	return nil
}

func TestFontConvert_UploadCreatesWOFF2(t *testing.T) {
	env := fontEnv(t, nil)
	res := env.postFontsUpload(t, map[string][]byte{"Go Regular.ttf": goregular.TTF})
	if len(res.Uploaded) != 1 || len(res.Skipped) != 0 || len(res.Warnings) != 0 {
		t.Fatalf("upload = %+v; want one clean upload", res)
	}

	f := env.fontByBase(t, "Go Regular")
	if f.Family != "Go Regular" {
		t.Errorf("family = %q; want the base-name default", f.Family)
	}
	ttf := variantByType(f, "TTF", false)
	conv := variantByType(f, "WOFF2", true)
	if ttf == nil || conv == nil || len(f.Variants) != 2 {
		t.Fatalf("variants = %+v; want the TTF upload + a converted WOFF2", f.Variants)
	}
	if conv.Size <= 0 || conv.Size >= ttf.Size {
		t.Errorf("converted size = %d (ttf %d); want smaller than the upload", conv.Size, ttf.Size)
	}
	if f.ServedType != "WOFF2" || f.ServedToken != conv.Token {
		t.Errorf("served = %s/%s; want the converted WOFF2 by default", f.ServedType, f.ServedToken)
	}

	// The served variant streams real WOFF2 bytes with the right type.
	resp := env.getWithHeaders(t, "/api/fonts/pub/f/"+f.ServedToken,
		map[string]string{"Sec-Fetch-Site": "same-origin"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fetch woff2 = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "font/woff2" {
		t.Errorf("Content-Type = %q; want font/woff2", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.HasPrefix(body, []byte("wOF2")) {
		t.Errorf("body does not start with the WOFF2 magic (got %q…)", body[:min(8, len(body))])
	}

	// The kit serves the same variant.
	if tok := env.kitToken(t); tok != f.ServedToken {
		t.Errorf("kit token = %q; want the served token %q", tok, f.ServedToken)
	}
}

func TestFontConvert_UploadedWOFF2SuppressesConversion(t *testing.T) {
	env := fontEnv(t, nil)
	env.postFontsUpload(t, map[string][]byte{"Go.ttf": goregular.TTF, "Go.woff2": []byte("uploaded-woff2")})

	f := env.fontByBase(t, "Go")
	if len(f.Variants) != 2 || variantByType(f, "WOFF2", true) != nil {
		t.Fatalf("variants = %+v; want TTF + uploaded WOFF2, no converted copy", f.Variants)
	}
	up := variantByType(f, "WOFF2", false)
	if up == nil || f.ServedToken != up.Token {
		t.Errorf("served = %q; want the uploaded WOFF2's token", f.ServedToken)
	}

	// Deleting the uploaded WOFF2 file re-converts one from the TTF.
	resp := env.del(t, "/api/fonts/Go.woff2")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete file = %d", resp.StatusCode)
	}
	f = env.fontByBase(t, "Go")
	if variantByType(f, "WOFF2", true) == nil {
		t.Errorf("variants = %+v; want a converted WOFF2 after the upload was deleted", f.Variants)
	}
}

func TestFontConvert_InvalidFontUploadsWithWarning(t *testing.T) {
	env := fontEnv(t, nil)
	res := env.postFontsUpload(t, map[string][]byte{"Broken.ttf": []byte("not really a font")})
	if len(res.Uploaded) != 1 {
		t.Fatalf("upload = %+v; want the file still uploaded", res)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "Broken.ttf") {
		t.Fatalf("warnings = %v; want a conversion warning for Broken.ttf", res.Warnings)
	}
	f := env.fontByBase(t, "Broken")
	if len(f.Variants) != 1 || f.ServedType != "TTF" {
		t.Errorf("unconvertible font should serve its TTF: %+v", f)
	}
	if tok := env.kitToken(t); tok != f.ServedToken {
		t.Errorf("kit token = %q; want the TTF token %q", tok, f.ServedToken)
	}
}

func TestFontConvert_ServeSelection(t *testing.T) {
	env := fontEnv(t, nil)
	env.postFontsUpload(t, map[string][]byte{"Go.ttf": goregular.TTF})
	f := env.fontByBase(t, "Go")
	ttfToken := variantByType(f, "TTF", false).Token

	// Explicitly serve the TTF.
	resp := env.patchJSON(t, "/api/fonts/families/Go", map[string]any{"serve": "TTF"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH serve = %d", resp.StatusCode)
	}
	f = env.fontByBase(t, "Go")
	if f.Serve != "TTF" || f.ServedType != "TTF" || f.ServedToken != ttfToken {
		t.Errorf("served = %s/%s; want the TTF after selection", f.ServedType, f.ServedToken)
	}
	if tok := env.kitToken(t); tok != ttfToken {
		t.Errorf("kit token = %q; want the TTF token", tok)
	}

	// A type the font doesn't have is refused.
	resp = env.patchJSON(t, "/api/fonts/families/Go", map[string]any{"serve": "EOT"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("serve EOT = %d; want 400", resp.StatusCode)
	}

	// Back to auto: the converted WOFF2 again.
	resp = env.patchJSON(t, "/api/fonts/families/Go", map[string]any{"serve": ""})
	resp.Body.Close()
	if f = env.fontByBase(t, "Go"); f.ServedType != "WOFF2" {
		t.Errorf("served type = %s; want WOFF2 on auto", f.ServedType)
	}
}

func TestFontConvert_CustomFamily(t *testing.T) {
	env := fontEnv(t, nil)
	env.postFontsUpload(t, map[string][]byte{"Go.ttf": goregular.TTF, "Go Mono.ttf": goregular.TTF})

	resp := env.patchJSON(t, "/api/fonts/families/Go", map[string]any{"family": "Fancy Heading"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH family = %d", resp.StatusCode)
	}
	if f := env.fontByBase(t, "Go"); f.Family != "Fancy Heading" {
		t.Errorf("family = %q; want the custom name", f.Family)
	}

	// The kit and the settings payload use the custom family.
	if css := env.kitCSS(t, nil); !strings.Contains(css, "font-family:'Fancy Heading'") {
		t.Errorf("kit.css missing the custom family:\n%s", css)
	}
	sresp := env.get(t, "/api/settings")
	var settings model.SettingsResponse
	_ = json.NewDecoder(sresp.Body).Decode(&settings)
	sresp.Body.Close()
	found := false
	for _, uf := range settings.UploadedFonts {
		if uf.Name == "Go" && uf.Family == "Fancy Heading" {
			found = true
		}
	}
	if !found {
		t.Errorf("settings uploaded_fonts missing the custom family: %+v", settings.UploadedFonts)
	}

	// A duplicate of another font's effective family is refused; so are quotes.
	resp = env.patchJSON(t, "/api/fonts/families/Go Mono", map[string]any{"family": "fancy heading"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate family = %d; want 409", resp.StatusCode)
	}
	resp = env.patchJSON(t, "/api/fonts/families/Go Mono", map[string]any{"family": "Bad'Name"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("quoted family = %d; want 400", resp.StatusCode)
	}
	// "" resets to the default.
	resp = env.patchJSON(t, "/api/fonts/families/Go", map[string]any{"family": ""})
	resp.Body.Close()
	if f := env.fontByBase(t, "Go"); f.Family != "Go" {
		t.Errorf("family after reset = %q; want the default \"Go\"", f.Family)
	}
}

func TestFontConvert_RenameCarriesMetaAndConversion(t *testing.T) {
	env := fontEnv(t, nil)
	env.postFontsUpload(t, map[string][]byte{"Old.ttf": goregular.TTF})
	resp := env.patchJSON(t, "/api/fonts/families/Old", map[string]any{
		"family": "Kept Name", "serve": "TTF", "origins": []string{"https://mysite.carrd.co"},
	})
	resp.Body.Close()

	resp = env.patchJSON(t, "/api/fonts/Old.ttf", map[string]any{"new_name": "New.ttf"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename = %d", resp.StatusCode)
	}

	fonts := env.listFonts(t).Fonts
	if len(fonts) != 1 || fonts[0].Base != "New" {
		t.Fatalf("fonts = %+v; want just New", fonts)
	}
	f := fonts[0]
	if variantByType(f, "WOFF2", true) == nil {
		t.Error("conversion did not follow the rename")
	}
	if f.Family != "Kept Name" || f.Serve != "TTF" ||
		len(f.Origins) != 1 || f.Origins[0] != "https://mysite.carrd.co" {
		t.Errorf("metadata did not follow the rename: %+v", f)
	}
}

func TestFontConvert_FamilyDeleteRemovesEverything(t *testing.T) {
	env := fontEnv(t, nil)
	env.postFontsUpload(t, map[string][]byte{"Gone.ttf": goregular.TTF, "Gone.woff": goregular.TTF})

	resp := env.reqJSON(t, http.MethodDelete, "/api/fonts/families/Gone", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete family = %d", resp.StatusCode)
	}
	if fonts := env.listFonts(t).Fonts; len(fonts) != 0 {
		t.Errorf("fonts = %+v; want empty", fonts)
	}
}

func TestFontConvert_StartupBackfillAndMetaMigration(t *testing.T) {
	// Seed disk + legacy settings BEFORE the server starts: a real font, a
	// stale pre-group conversion, v1 file-keyed metadata, and the old global
	// origin allowlist — the startup migrations must reconcile all of it.
	webRoot := t.TempDir()
	fontsDir := filepath.Join(webRoot, "fonts")
	if err := os.MkdirAll(filepath.Join(fontsDir, ".woff2"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fontsDir, "Legacy.ttf"), goregular.TTF, 0644); err != nil {
		t.Fatal(err)
	}
	// Old per-file conversion naming — must be swept.
	if err := os.WriteFile(filepath.Join(fontsDir, ".woff2", "Legacy.ttf.woff2"), []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	// v1 metadata (keyed by FILENAME, with the old "original" flag) + the old
	// global origins list.
	if err := st.SetSetting("font_meta", `{"Legacy.ttf":{"family":"Old Style","original":true}}`); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting("font_allowed_origins", `["https://mysite.carrd.co"]`); err != nil {
		t.Fatal(err)
	}

	srv := server.New(st, ws.NewHub(), testSecret, webRoot, nil)
	ts := httptest.NewTLSServer(srv)
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	client := ts.Client()
	client.Jar = jar
	env := &testEnv{ts: ts, client: client, store: st, srv: srv}
	env.loginAdmin(t)

	f := env.fontByBase(t, "Legacy")
	if variantByType(f, "WOFF2", true) == nil {
		t.Errorf("startup backfill did not create a conversion: %+v", f.Variants)
	}
	if f.Family != "Old Style" {
		t.Errorf("family = %q; want the migrated v1 custom name", f.Family)
	}
	if f.Serve != "TTF" || f.ServedType != "TTF" {
		t.Errorf("serve = %q/%q; want TTF migrated from the v1 original flag", f.Serve, f.ServedType)
	}
	if len(f.Origins) != 1 || f.Origins[0] != "https://mysite.carrd.co" {
		t.Errorf("origins = %v; want the migrated global allowlist", f.Origins)
	}
	// The stale pre-group conversion file was swept.
	if _, err := os.Stat(filepath.Join(fontsDir, ".woff2", "Legacy.ttf.woff2")); !os.IsNotExist(err) {
		t.Error("stale pre-group conversion file was not swept")
	}
}
