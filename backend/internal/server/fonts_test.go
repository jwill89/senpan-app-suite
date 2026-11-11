package server_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"app-suite/internal/fonts"
)

type testFontVariant struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Converted bool   `json:"converted"`
	Size      int64  `json:"size"`
	Token     string `json:"token"`
}

type testFont struct {
	Base        string            `json:"base"`
	Family      string            `json:"family"`
	Serve       string            `json:"serve"`
	ServedType  string            `json:"servedType"`
	ServedToken string            `json:"servedToken"`
	Origins     []string          `json:"origins"`
	Variants    []testFontVariant `json:"variants"`
}

type testFontUpload struct {
	Uploaded []string `json:"uploaded"`
	Skipped  []struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"skipped"`
	Warnings []string `json:"warnings"`
}

// fontEnv builds a test env whose data folder's fonts folder holds the given files and
// signs the admin in (managing fonts needs it; the public operations need nothing).
func fontEnv(t *testing.T, files map[string][]byte) *testEnv {
	t.Helper()
	dataDir := t.TempDir()
	for name, data := range files {
		writeFile(t, dataDir, "fonts/"+name, data)
	}
	env := newTestEnvWithDataDir(t, dataDir)
	env.loginAdmin(t)
	return env
}

func (e *testEnv) getWithHeaders(t *testing.T, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.url(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (e *testEnv) uploadFonts(t *testing.T, named map[string][]byte) testFontUpload {
	t.Helper()
	var out testFontUpload
	decodeInto(t, e.upload(t, "/api/v1/fonts", nil, files(named)...), http.StatusCreated, &out)
	return out
}

func (e *testEnv) listFonts(t *testing.T) []testFont {
	t.Helper()
	var out struct {
		Fonts []testFont `json:"fonts"`
	}
	decodeInto(t, e.get(t, "/api/v1/fonts"), http.StatusOK, &out)
	return out.Fonts
}

func (e *testEnv) fontByBase(t *testing.T, base string) testFont {
	t.Helper()
	for _, f := range e.listFonts(t) {
		if f.Base == base {
			return f
		}
	}
	t.Fatalf("font %q not in the list", base)
	return testFont{}
}

func variantByType(f testFont, typ string, converted bool) *testFontVariant {
	for i := range f.Variants {
		if f.Variants[i].Type == typ && f.Variants[i].Converted == converted {
			return &f.Variants[i]
		}
	}
	return nil
}

func (e *testEnv) patchFont(t *testing.T, base string, body map[string]any) *http.Response {
	t.Helper()
	return e.patchJSON(t, "/api/v1/fonts/"+base, body)
}

func (e *testEnv) setFontOrigins(t *testing.T, base string, origins ...string) {
	t.Helper()
	resp := e.patchFont(t, base, map[string]any{"origins": origins})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("PATCH /fonts/%s = %d: %s", base, resp.StatusCode, body)
	}
}

func (e *testEnv) kitResponse(t *testing.T, headers map[string]string) (*http.Response, string) {
	t.Helper()
	resp := e.getWithHeaders(t, "/api/v1/fonts/pub/kit.css", headers)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func (e *testEnv) kitCSS(t *testing.T, headers map[string]string) string {
	t.Helper()
	resp, body := e.kitResponse(t, headers)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET kit.css = %d", resp.StatusCode)
	}
	return body
}

// kitToken is the serving token of the (single) font in kit.css.
func (e *testEnv) kitToken(t *testing.T) string {
	t.Helper()
	m := regexp.MustCompile(`url\('f/([^']+)'\)`).FindStringSubmatch(e.kitCSS(t, nil))
	if m == nil {
		t.Fatal("no tokenized src in kit.css")
	}
	return m[1]
}

func TestFonts_Guards(t *testing.T) {
	env := newTestEnv(t)
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/v1/fonts", nil},
		{http.MethodPatch, "/api/v1/fonts/F", map[string]any{"family": "x"}},
		{http.MethodDelete, "/api/v1/fonts/F", nil},
		{http.MethodPatch, "/api/v1/font-files/F.ttf", map[string]any{"name": "G.ttf"}},
		{http.MethodDelete, "/api/v1/font-files/F.ttf", nil},
	} {
		resp := env.reqJSON(t, tc.method, tc.path, tc.body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s signed out = %d; want 401", tc.method, tc.path, resp.StatusCode)
		}
	}
	resp := env.upload(t, "/api/v1/fonts", nil, files(map[string][]byte{"F.ttf": []byte("x")})...)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("upload signed out = %d; want 401", resp.StatusCode)
	}
	user := makeActiveUser(t, env, "images", "password123", []string{"system-images"})
	resp = reqAs(t, user, env, http.MethodGet, "/api/v1/fonts", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("an account without the Fonts page = %d; want 403", resp.StatusCode)
	}
}

func TestFonts_UploadCreatesWOFF2(t *testing.T) {
	env := fontEnv(t, nil)
	res := env.uploadFonts(t, map[string][]byte{"Go Regular.ttf": goregular.TTF})
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
		t.Fatalf("variants = %+v; want the TTF upload and a converted WOFF2", f.Variants)
	}
	if conv.Size <= 0 || conv.Size >= ttf.Size {
		t.Errorf("converted size = %d (ttf %d); want smaller than the upload", conv.Size, ttf.Size)
	}
	if f.ServedType != "WOFF2" || f.ServedToken != conv.Token {
		t.Errorf("served = %s/%s; want the converted WOFF2 by default", f.ServedType, f.ServedToken)
	}

	// The served variant streams real WOFF2 bytes with the right type.
	resp := env.getWithHeaders(t, "/api/v1/fonts/pub/f/"+f.ServedToken, map[string]string{"Sec-Fetch-Site": "same-origin"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fetch woff2 = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "font/woff2" {
		t.Errorf("Content-Type = %q; want font/woff2", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.HasPrefix(body, []byte("wOF2")) {
		t.Errorf("body does not start with the WOFF2 magic (got %q...)", body[:min(8, len(body))])
	}
	if tok := env.kitToken(t); tok != f.ServedToken {
		t.Errorf("kit token = %q; want the served token %q", tok, f.ServedToken)
	}
}

func TestFonts_UploadRules(t *testing.T) {
	env := fontEnv(t, nil)

	env.uploadFonts(t, map[string][]byte{"Go.ttf": goregular.TTF})
	var res testFontUpload
	decodeInto(t, env.upload(t, "/api/v1/fonts", nil, files(map[string][]byte{
		"Go.ttf": goregular.TTF, "x'}.ttf": []byte("x"), "evil.exe": []byte("x"), "Fine.otf": []byte("not a font"),
	})...), http.StatusCreated, &res)
	reasons := map[string]string{}
	for _, s := range res.Skipped {
		reasons[s.Name] = s.Reason
	}
	if !strings.HasPrefix(reasons["Go.ttf"], "Already exists") {
		t.Errorf("a name already there should be skipped, not replaced: %v", reasons)
	}
	if !strings.HasPrefix(reasons["x'}.ttf"], "Unsupported type") || !strings.HasPrefix(reasons["evil.exe"], "Unsupported type") {
		t.Errorf("reasons = %v; want a CSS-breaking name and an exe refused", reasons)
	}
	// A file that can't be converted still uploads, with a warning.
	if len(res.Uploaded) != 1 || res.Uploaded[0] != "Fine.otf" || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "Fine.otf") {
		t.Errorf("upload = %+v; want Fine.otf saved with a conversion warning", res)
	}
	f := env.fontByBase(t, "Fine")
	if len(f.Variants) != 1 || f.ServedType != "OTF" {
		t.Errorf("an unconvertible font should serve its own format: %+v", f)
	}

	if locs := problemLocations(t, env.upload(t, "/api/v1/fonts", nil, files(map[string][]byte{"Go.ttf": goregular.TTF})...)); len(locs) != 1 || locs[0] != "body.files[0]" {
		t.Errorf("only a skipped file: locations = %v; want body.files[0]", locs)
	}
	if locs := problemLocations(t, env.upload(t, "/api/v1/fonts", nil)); len(locs) != 1 || locs[0] != "body.files" {
		t.Errorf("no file: locations = %v; want body.files", locs)
	}
}

func TestFonts_UploadedWOFF2SuppressesConversion(t *testing.T) {
	env := fontEnv(t, nil)
	env.uploadFonts(t, map[string][]byte{"Go.ttf": goregular.TTF, "Go.woff2": []byte("uploaded-woff2")})

	f := env.fontByBase(t, "Go")
	if len(f.Variants) != 2 || variantByType(f, "WOFF2", true) != nil {
		t.Fatalf("variants = %+v; want the TTF and the uploaded WOFF2, no converted copy", f.Variants)
	}
	up := variantByType(f, "WOFF2", false)
	if up == nil || f.ServedToken != up.Token {
		t.Errorf("served = %q; want the uploaded WOFF2's token", f.ServedToken)
	}

	// Deleting the uploaded WOFF2 converts one from the TTF.
	resp := env.del(t, "/api/v1/font-files/Go.woff2")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete file = %d", resp.StatusCode)
	}
	if f = env.fontByBase(t, "Go"); variantByType(f, "WOFF2", true) == nil {
		t.Errorf("variants = %+v; want a converted WOFF2 after the upload was deleted", f.Variants)
	}
	for _, name := range []string{"Go.woff2", "..%2Fx.ttf", "readme.txt"} {
		resp = env.del(t, "/api/v1/font-files/"+name)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("deleting %q = %d; want 404", name, resp.StatusCode)
		}
	}
}

func TestFonts_ServeSelection(t *testing.T) {
	env := fontEnv(t, nil)
	env.uploadFonts(t, map[string][]byte{"Go.ttf": goregular.TTF})
	ttfToken := variantByType(env.fontByBase(t, "Go"), "TTF", false).Token

	var f testFont
	decodeInto(t, env.patchFont(t, "Go", map[string]any{"serve": "TTF"}), http.StatusOK, &f)
	if f.Serve != "TTF" || f.ServedType != "TTF" || f.ServedToken != ttfToken {
		t.Errorf("served = %s/%s; want the TTF after the choice", f.ServedType, f.ServedToken)
	}
	if tok := env.kitToken(t); tok != ttfToken {
		t.Errorf("kit token = %q; want the TTF token", tok)
	}
	for name, serve := range map[string]string{"a type the font hasn't": "EOT", "no such type": "FLAC"} {
		resp := env.patchFont(t, "Go", map[string]any{"serve": serve})
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("serve %s = %d; want 422", name, resp.StatusCode)
		}
	}
	decodeInto(t, env.patchFont(t, "Go", map[string]any{"serve": ""}), http.StatusOK, &f)
	if f.ServedType != "WOFF2" {
		t.Errorf("served type = %s; want WOFF2 on automatic", f.ServedType)
	}
}

func TestFonts_CustomFamily(t *testing.T) {
	env := fontEnv(t, nil)
	env.uploadFonts(t, map[string][]byte{"Go.ttf": goregular.TTF, "Go Mono.ttf": goregular.TTF})

	var f testFont
	decodeInto(t, env.patchFont(t, "Go", map[string]any{"family": "Fancy Heading"}), http.StatusOK, &f)
	if f.Family != "Fancy Heading" {
		t.Errorf("family = %q; want the custom name", f.Family)
	}
	if css := env.kitCSS(t, nil); !strings.Contains(css, "font-family:'Fancy Heading'") {
		t.Errorf("kit.css missing the custom family:\n%s", css)
	}
	var settings struct {
		UploadedFonts []struct{ Name, Family, Token string } `json:"uploadedFonts"`
	}
	decodeInto(t, env.get(t, "/api/v1/settings"), http.StatusOK, &settings)
	found := false
	for _, uf := range settings.UploadedFonts {
		if uf.Name == "Go" && uf.Family == "Fancy Heading" {
			found = true
		}
	}
	if !found {
		t.Errorf("settings uploadedFonts missing the custom family: %+v", settings.UploadedFonts)
	}

	for name, tc := range map[string]struct {
		family string
		want   int
	}{
		"another font's family": {"fancy heading", http.StatusConflict},
		"a quote":               {"Bad'Name", http.StatusUnprocessableEntity},
		"a brace":               {"a{b", http.StatusUnprocessableEntity},
		"too long":              {strings.Repeat("a", 101), http.StatusUnprocessableEntity},
	} {
		resp := env.patchFont(t, "Go Mono", map[string]any{"family": tc.family})
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("family %s = %d; want %d", name, resp.StatusCode, tc.want)
		}
	}
	decodeInto(t, env.patchFont(t, "Go", map[string]any{"family": ""}), http.StatusOK, &f)
	if f.Family != "Go" {
		t.Errorf("family after a reset = %q; want the default \"Go\"", f.Family)
	}
	resp := env.patchFont(t, "Go", map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("nothing to change = %d; want 422", resp.StatusCode)
	}
	resp = env.patchFont(t, "Nope", map[string]any{"family": "X"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("an unknown font = %d; want 404", resp.StatusCode)
	}
}

func TestFonts_RenameCarriesMetaAndConversion(t *testing.T) {
	env := fontEnv(t, nil)
	env.uploadFonts(t, map[string][]byte{"Old.ttf": goregular.TTF, "Other.ttf": goregular.TTF})
	resp := env.patchFont(t, "Old", map[string]any{"family": "Kept Name", "serve": "TTF", "origins": []string{"https://mysite.carrd.co"}})
	resp.Body.Close()

	var renamed struct {
		Name string `json:"name"`
	}
	decodeInto(t, env.patchJSON(t, "/api/v1/font-files/Old.ttf", map[string]any{"name": "New.ttf"}), http.StatusOK, &renamed)
	if renamed.Name != "New.ttf" {
		t.Errorf("renamed to %q", renamed.Name)
	}
	f := env.fontByBase(t, "New")
	if variantByType(f, "WOFF2", true) == nil {
		t.Error("the conversion did not follow the rename")
	}
	if f.Family != "Kept Name" || f.Serve != "TTF" || len(f.Origins) != 1 || f.Origins[0] != "https://mysite.carrd.co" {
		t.Errorf("metadata did not follow the rename: %+v", f)
	}

	for name, tc := range map[string]struct {
		path string
		body map[string]any
		want int
	}{
		"a name taken":              {"New.ttf", map[string]any{"name": "Other.ttf"}, http.StatusConflict},
		"a family another font has": {"Other.ttf", map[string]any{"name": "Kept Name.ttf"}, http.StatusConflict},
		"a bad extension":           {"Other.ttf", map[string]any{"name": "Other.exe"}, http.StatusUnprocessableEntity},
		"a css-breaking name":       {"Other.ttf", map[string]any{"name": "a{b.ttf"}, http.StatusUnprocessableEntity},
		"an unknown file":           {"Missing.ttf", map[string]any{"name": "X.ttf"}, http.StatusNotFound},
		"an unsafe source name":     {"..%2Fx.ttf", map[string]any{"name": "X.ttf"}, http.StatusNotFound},
	} {
		resp := env.patchJSON(t, "/api/v1/font-files/"+tc.path, tc.body)
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("renaming %s = %d; want %d", name, resp.StatusCode, tc.want)
		}
	}
}

func TestFonts_DeleteRemovesEverything(t *testing.T) {
	env := fontEnv(t, nil)
	env.uploadFonts(t, map[string][]byte{"Gone.ttf": goregular.TTF, "Gone.woff": goregular.TTF})
	env.setFontOrigins(t, "Gone", "https://mysite.carrd.co")

	resp := env.del(t, "/api/v1/fonts/Gone")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete font = %d", resp.StatusCode)
	}
	if list := env.listFonts(t); len(list) != 0 {
		t.Errorf("fonts = %+v; want none", list)
	}
	for _, rel := range []string{"fonts/Gone.ttf", "fonts/Gone.woff", "fonts/.woff2/gone.woff2"} {
		if _, err := os.Stat(filepath.Join(env.srv.DataDirForTest(), rel)); !os.IsNotExist(err) {
			t.Errorf("%s survived its font: %v", rel, err)
		}
	}
	// Its metadata went too: a font of the same name starts fresh.
	env.uploadFonts(t, map[string][]byte{"Gone.ttf": goregular.TTF})
	if f := env.fontByBase(t, "Gone"); len(f.Origins) != 0 {
		t.Errorf("origins = %v; want the deleted font's list gone", f.Origins)
	}
	resp = env.del(t, "/api/v1/fonts/Nope")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("deleting an unknown font = %d; want 404", resp.StatusCode)
	}
}

func TestFonts_StartupBackfillAndMetaMigration(t *testing.T) {
	// Disk and settings in the form the startup migrations upgrade are seeded before
	// the server starts: a real font, a stale conversion under a superseded name,
	// metadata keyed by file with an "original" flag, and one global origin list. The
	// startup migrations must reconcile all of it.
	dataDir := t.TempDir()
	writeFile(t, dataDir, "fonts/Legacy.ttf", goregular.TTF)
	writeFile(t, dataDir, "fonts/.woff2/Legacy.ttf.woff2", []byte("stale"))

	env := newTestEnvWithStore(t, dataDir, func(set func(key, value string)) {
		set("font_meta", `{"Legacy.ttf":{"family":"Old Style","original":true}}`)
		set("font_allowed_origins", `["https://mysite.carrd.co"]`)
	})
	env.loginAdmin(t)

	f := env.fontByBase(t, "Legacy")
	if variantByType(f, "WOFF2", true) == nil {
		t.Errorf("the startup backfill did not create a conversion: %+v", f.Variants)
	}
	if f.Family != "Old Style" {
		t.Errorf("family = %q; want the migrated custom name", f.Family)
	}
	if f.Serve != "TTF" || f.ServedType != "TTF" {
		t.Errorf("serve = %q/%q; want TTF migrated from the old original flag", f.Serve, f.ServedType)
	}
	if len(f.Origins) != 1 || f.Origins[0] != "https://mysite.carrd.co" {
		t.Errorf("origins = %v; want the migrated list", f.Origins)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "fonts", ".woff2", "Legacy.ttf.woff2")); !os.IsNotExist(err) {
		t.Error("the stale conversion file was not swept")
	}
}

// -- The public operations ----------------------------------------------------------

func TestFontKit(t *testing.T) {
	// The bytes aren't a font, so conversion fails and the TTF itself is served.
	env := fontEnv(t, map[string][]byte{"My Font.ttf": []byte("ttf-bytes")})

	resp, css := env.kitResponse(t, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("kit.css = %d", resp.StatusCode)
	}
	if !strings.Contains(css, "font-family:'My Font'") || !strings.Contains(css, "format('truetype')") {
		t.Errorf("kit.css missing the rule:\n%s", css)
	}
	if strings.Contains(css, "My Font.ttf") || strings.Contains(css, "My%20Font") {
		t.Errorf("kit.css leaks the real file name:\n%s", css)
	}
	for header, want := range map[string]string{
		"Content-Type":           "text/css; charset=utf-8",
		"Cache-Control":          "private, max-age=300",
		"Vary":                   "Referer",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("%s = %q; want %q", header, got, want)
		}
	}
	if resp.Header.Get("ETag") != "" {
		t.Error("the kit should not be revalidated by an ETag the /api layer made up")
	}

	// It needs no sign-in, Origin or credentials.
	anon := newTestEnvWithDataDir(t, env.srv.DataDirForTest())
	if got := anon.kitCSS(t, nil); !strings.Contains(got, "font-family:'My Font'") {
		t.Errorf("kit.css for nobody:\n%s", got)
	}
}

func TestFontKit_FilteredByReferer(t *testing.T) {
	env := fontEnv(t, map[string][]byte{"A.woff2": []byte("woff2"), "B.woff2": []byte("woff2")})
	env.setFontOrigins(t, "A", "https://mysite.carrd.co")

	allowed := env.kitCSS(t, map[string]string{"Referer": "https://mysite.carrd.co/about"})
	if !strings.Contains(allowed, "font-family:'A'") || strings.Contains(allowed, "font-family:'B'") {
		t.Errorf("the allowed site's kit should hold only font A:\n%s", allowed)
	}
	foreign := env.kitCSS(t, map[string]string{"Referer": "https://evil.example.com/page"})
	if strings.Contains(foreign, "@font-face") {
		t.Errorf("a foreign site's kit should hold no fonts:\n%s", foreign)
	}
	local := env.kitCSS(t, map[string]string{"Referer": env.url("/some/page")})
	if !strings.Contains(local, "font-family:'A'") || !strings.Contains(local, "font-family:'B'") {
		t.Errorf("the app's own kit should hold every font:\n%s", local)
	}
}

func TestFontFile_OriginGate(t *testing.T) {
	env := fontEnv(t, map[string][]byte{"Body.woff2": []byte("woff2-bytes")})
	env.setFontOrigins(t, "Body", "https://mysite.carrd.co")
	path := "/api/v1/fonts/pub/f/" + env.kitToken(t)

	t.Run("no site named at all is refused", func(t *testing.T) {
		resp := env.getWithHeaders(t, path, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("bare request = %d; want 403", resp.StatusCode)
		}
		if vary := strings.Join(resp.Header.Values("Vary"), ","); !strings.Contains(vary, "Origin") {
			t.Errorf("a refusal's Vary = %q; want Origin", vary)
		}
	})

	t.Run("an allowed origin is served with the CORS echo", func(t *testing.T) {
		resp := env.getWithHeaders(t, path, map[string]string{"Origin": "https://mysite.carrd.co"})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("allowed origin = %d; want 200", resp.StatusCode)
		}
		for header, want := range map[string]string{
			"Access-Control-Allow-Origin": "https://mysite.carrd.co",
			"Vary":                        "Origin",
			"Content-Type":                "font/woff2",
			"Cache-Control":               "private, max-age=604800",
			"X-Content-Type-Options":      "nosniff",
		} {
			if got := resp.Header.Get(header); got != want {
				t.Errorf("%s = %q; want %q", header, got, want)
			}
		}
		if resp.Header.Get("Access-Control-Allow-Credentials") != "" {
			t.Error("a font is never served with credentials")
		}
		if resp.Header.Get("Accept-Ranges") != "bytes" {
			t.Errorf("Accept-Ranges = %q; want bytes", resp.Header.Get("Accept-Ranges"))
		}
		if body, _ := io.ReadAll(resp.Body); string(body) != "woff2-bytes" {
			t.Errorf("body = %q; want the font bytes", body)
		}
	})

	t.Run("a foreign origin is refused", func(t *testing.T) {
		resp := env.getWithHeaders(t, path, map[string]string{"Origin": "https://other.carrd.co"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("foreign origin = %d; want 403", resp.StatusCode)
		}
	})

	// Safari loads @font-face without CORS: no Origin, and a Referer naming the page.
	safari := func(referer string) map[string]string {
		return map[string]string{"Referer": referer, "Sec-Fetch-Site": "cross-site", "Sec-Fetch-Dest": "font"}
	}
	t.Run("safari: an allowed page in the Referer is served", func(t *testing.T) {
		for _, ref := range []string{"https://mysite.carrd.co/", "https://MySite.Carrd.co/about"} {
			resp := env.getWithHeaders(t, path, safari(ref))
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Referer %q = %d; want 200", ref, resp.StatusCode)
			}
			if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao != "" {
				t.Errorf("Referer %q: ACAO = %q; want none (a no-cors answer is never CORS-checked)", ref, acao)
			}
		}
	})
	t.Run("safari: a foreign page in the Referer is refused", func(t *testing.T) {
		resp := env.getWithHeaders(t, path, safari("https://other.carrd.co/"))
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("foreign Referer = %d; want 403", resp.StatusCode)
		}
	})
	t.Run("a refused Origin is not rescued by an allowed Referer", func(t *testing.T) {
		resp := env.getWithHeaders(t, path, map[string]string{"Origin": "https://other.carrd.co", "Referer": "https://mysite.carrd.co/"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("foreign Origin with an allowed Referer = %d; want 403", resp.StatusCode)
		}
	})

	t.Run("allowlists are per font", func(t *testing.T) {
		writeFile(t, env.srv.DataDirForTest(), "fonts/Locked.woff2", []byte("locked"))
		locked := env.fontByBase(t, "Locked")
		resp := env.getWithHeaders(t, "/api/v1/fonts/pub/f/"+locked.ServedToken, map[string]string{"Origin": "https://mysite.carrd.co"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("another font with the same origin = %d; want 403", resp.StatusCode)
		}
	})

	t.Run("the app's own pages always pass", func(t *testing.T) {
		hostOrigin := strings.TrimSuffix(env.url(""), "/")
		for name, headers := range map[string]map[string]string{
			"origin":         {"Origin": hostOrigin},
			"sec-fetch-site": {"Sec-Fetch-Site": "same-origin"},
			"referer":        {"Referer": env.url("/admin")},
		} {
			resp := env.getWithHeaders(t, path, headers)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("same-origin via %s = %d; want 200", name, resp.StatusCode)
			}
		}
	})

	t.Run("an unknown token is a 404", func(t *testing.T) {
		resp := env.getWithHeaders(t, "/api/v1/fonts/pub/f/AAAAAAAAAAAAAAAAAAAAAA.woff2", map[string]string{"Sec-Fetch-Site": "same-origin"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("bogus token = %d; want 404", resp.StatusCode)
		}
	})

	t.Run("the old path is gone", func(t *testing.T) {
		resp := env.getWithHeaders(t, "/api/fonts/pub/f/"+env.kitToken(t), map[string]string{"Sec-Fetch-Site": "same-origin"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("the old font path = %d; want 404", resp.StatusCode)
		}
	})
}

func TestFontFile_RangeConditionalAndHead(t *testing.T) {
	env := fontEnv(t, map[string][]byte{"Body.woff2": []byte("0123456789")})
	path := "/api/v1/fonts/pub/f/" + env.kitToken(t)
	same := map[string]string{"Sec-Fetch-Site": "same-origin"}

	resp := env.getWithHeaders(t, path, map[string]string{"Sec-Fetch-Site": "same-origin", "Range": "bytes=2-5"})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || string(body) != "2345" {
		t.Fatalf("range = %d %q; want 206 2345", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 2-5/10" {
		t.Errorf("Content-Range = %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "private, max-age=604800" {
		t.Errorf("a range answer's Cache-Control = %q; the /api layer must leave the font's own alone", got)
	}

	// A conditional request is answered 304 with no body.
	resp = env.getWithHeaders(t, path, same)
	modified := resp.Header.Get("Last-Modified")
	resp.Body.Close()
	if modified == "" {
		t.Fatal("no Last-Modified to revalidate with")
	}
	resp = env.getWithHeaders(t, path, map[string]string{"Sec-Fetch-Site": "same-origin", "If-Modified-Since": modified})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified || len(body) != 0 {
		t.Errorf("conditional = %d with %d bytes; want 304 and none", resp.StatusCode, len(body))
	}

	req, _ := http.NewRequest(http.MethodHead, env.url(path), nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(body) != 0 || resp.Header.Get("Content-Length") != "10" {
		t.Errorf("HEAD = %d with %d bytes and length %q; want 200, none, 10", resp.StatusCode, len(body), resp.Header.Get("Content-Length"))
	}
}

func TestFontFile_TokenExpiry(t *testing.T) {
	env := fontEnv(t, map[string][]byte{"Old.ttf": []byte("ttf")})
	same := map[string]string{"Sec-Fetch-Site": "same-origin"}
	secret, err := env.srv.FontSecretForTest(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	bucket := fonts.TokenBucket(time.Now())

	// The previous bucket's token still serves (the grace window)...
	resp := env.getWithHeaders(t, "/api/v1/fonts/pub/f/"+fonts.FileToken(secret, "Old.ttf", bucket-1), same)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("previous-bucket token = %d; want 200", resp.StatusCode)
	}
	// ...older ones have expired.
	resp = env.getWithHeaders(t, "/api/v1/fonts/pub/f/"+fonts.FileToken(secret, "Old.ttf", bucket-2), same)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("two-buckets-old token = %d; want 404", resp.StatusCode)
	}
}

func TestFonts_Origins(t *testing.T) {
	env := fontEnv(t, map[string][]byte{"F.otf": []byte("otf")})

	env.setFontOrigins(t, "F", "https://MySite.Carrd.co/", "https://mysite.carrd.co", " https://other.example.com ")
	got := env.listFonts(t)[0].Origins
	if len(got) != 2 || got[0] != "https://mysite.carrd.co" || got[1] != "https://other.example.com" {
		t.Errorf("saved origins = %v; want them normalized and deduplicated", got)
	}
	for _, bad := range []string{"mysite.carrd.co", "https://site.com/path", "ftp://site.com", "https://"} {
		resp := env.patchFont(t, "F", map[string]any{"origins": []string{bad}})
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("origin %q = %d; want 422", bad, resp.StatusCode)
		}
	}
	tooMany := make([]string, 101)
	for i := range tooMany {
		tooMany[i] = "https://a.example.com"
	}
	resp := env.patchFont(t, "F", map[string]any{"origins": tooMany})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("101 origins = %d; want 422", resp.StatusCode)
	}
}

func TestFonts_SettingsCarryTokens(t *testing.T) {
	env := fontEnv(t, map[string][]byte{"Header.woff": []byte("woff")})
	var out struct {
		UploadedFonts []struct{ Name, Family, Token string } `json:"uploadedFonts"`
	}
	decodeInto(t, env.get(t, "/api/v1/settings"), http.StatusOK, &out)
	if len(out.UploadedFonts) != 1 {
		t.Fatalf("uploadedFonts = %+v; want one", out.UploadedFonts)
	}
	f := out.UploadedFonts[0]
	// The bytes can't convert, so the uploaded WOFF itself is served.
	if f.Name != "Header" || f.Family != "Header" || !strings.HasSuffix(f.Token, ".woff") {
		t.Errorf("font = %+v; want the base name and a tokenized .woff link", f)
	}
	resp := env.getWithHeaders(t, "/api/v1/fonts/pub/f/"+f.Token, map[string]string{"Sec-Fetch-Site": "same-origin"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("the settings token fetch = %d; want 200", resp.StatusCode)
	}
}
