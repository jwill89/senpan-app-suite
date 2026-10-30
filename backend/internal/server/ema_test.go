package server_test

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// -- Ema Plaques --------------------------------------------------------------

const testWebhook = "https://discord.com/api/webhooks/1/test-token"

// emaFestival is a published festival with an open plaque board whose one window
// is taking plaques now.
type emaFestival struct {
	mapID, boardID, windowID int
	webRoot                  string
	env                      *testEnv
}

func newEmaFestival(t *testing.T) emaFestival {
	t.Helper()
	webRoot := t.TempDir()
	env := newTestEnvWithWebRoot(t, webRoot)
	env.loginAdmin(t)
	f := emaFestival{mapID: env.createMap(t, "Obon Matsuri 2026"), webRoot: webRoot, env: env}
	env.publishMap(t, f.mapID)

	now := time.Now().UTC()
	resp := env.postJSON(t, "/api/ema-boards", map[string]any{
		"festival_map_id": f.mapID, "title": "Wishing Wall",
		"template_image": "images/ema/template.png", "instructions": "Edit and post it.",
		"allow_self": true, "allow_site": true, "allow_artist": true, "artist_name": "Tara",
		"windows": []map[string]any{{
			"label":    "Day 1",
			"start_at": now.Add(-time.Hour).Format(time.RFC3339),
			"end_at":   now.Add(time.Hour).Format(time.RFC3339), "webhook_url": testWebhook,
		}},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create board status = %d; want 201", resp.StatusCode)
	}
	board := decodeBody(t, resp)["board"].(map[string]any)
	f.boardID = int(board["id"].(float64))
	f.windowID = int(board["windows"].([]any)[0].(map[string]any)["id"].(float64))
	return f
}

// plaquePNG is a tiny valid PNG.
func plaquePNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 6))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// submitPlaque posts a site plaque as `name @ Gilgamesh` and returns the response.
func (f emaFestival) submitPlaque(t *testing.T, name string, img []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("name", name)
	_ = mw.WriteField("world", "Gilgamesh")
	part, _ := mw.CreateFormFile("image", "ema.png")
	_, _ = part.Write(img)
	_ = mw.Close()
	resp, err := f.env.client.Post(f.env.url(fmt.Sprintf("/api/ema/%d/plaques", f.boardID)), mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (f emaFestival) requestPlaque(t *testing.T, name, desc string) *http.Response {
	t.Helper()
	return f.env.postJSON(t, fmt.Sprintf("/api/ema/%d/requests", f.boardID), map[string]any{
		"name": name, "world": "Gilgamesh", "description": desc,
	})
}

// plaqueFile is where a stored plaque image lives on disk.
func (f emaFestival) plaqueFile(rel string) string {
	return filepath.Join(f.webRoot, filepath.FromSlash(rel))
}

func TestEma_BoardRequiresPermissionAndOnePerFestival(t *testing.T) {
	f := newEmaFestival(t)

	anon := &http.Client{Transport: f.env.ts.Client().Transport}
	if r, err := anon.Get(f.env.url("/api/ema-boards")); err == nil {
		if r.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous list status = %d; want 401", r.StatusCode)
		}
		r.Body.Close()
	}

	resp := f.env.postJSON(t, "/api/ema-boards", map[string]any{
		"festival_map_id": f.mapID, "template_image": "images/x.png", "allow_artist": true,
		"windows": []map[string]any{{"start_at": "2026-08-01T00:00:00Z", "end_at": "2026-08-02T00:00:00Z"}},
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("second board for one festival = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestEma_BoardSaveValidates(t *testing.T) {
	f := newEmaFestival(t)
	base := func(windows []map[string]any) map[string]any {
		return map[string]any{
			"festival_map_id": f.mapID, "template_image": "images/x.png", "allow_site": true,
			"windows": windows,
		}
	}
	cases := map[string]map[string]any{
		"overlapping days": base([]map[string]any{
			{"label": "Day 1", "start_at": "2026-08-01T00:00:00Z", "end_at": "2026-08-02T06:00:00Z"},
			{"label": "Day 2", "start_at": "2026-08-02T00:00:00Z", "end_at": "2026-08-03T00:00:00Z"},
		}),
		"ends before it starts": base([]map[string]any{
			{"start_at": "2026-08-02T00:00:00Z", "end_at": "2026-08-01T00:00:00Z"},
		}),
		"not a discord webhook": base([]map[string]any{
			{"start_at": "2026-08-01T00:00:00Z", "end_at": "2026-08-02T00:00:00Z", "webhook_url": "https://example.com/hook"},
		}),
		"no days": base(nil),
	}
	for name, body := range cases {
		resp := f.env.reqJSON(t, http.MethodPut, fmt.Sprintf("/api/ema-boards/%d", f.boardID), body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d; want 400", name, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestEma_OnePlaquePerPersonPerDayAcrossOptions(t *testing.T) {
	f := newEmaFestival(t)

	resp := f.submitPlaque(t, "Aria Ashwood", plaquePNG(t))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("submit status = %d; want 201", resp.StatusCode)
	}
	plaque := decodeBody(t, resp)["plaque"].(map[string]any)
	if plaque["status"] != "pending" || plaque["window_label"] != "Day 1" {
		t.Errorf("plaque = %v; want pending on Day 1", plaque)
	}
	if _, err := os.Stat(f.plaqueFile(plaque["image"].(string))); err != nil {
		t.Errorf("the plaque image wasn't kept for review: %v", err)
	}

	// Same person, same day: neither another design nor a request to the artist.
	if r := f.submitPlaque(t, "aria ashwood", plaquePNG(t)); r.StatusCode != http.StatusConflict {
		t.Errorf("second design = %d; want 409", r.StatusCode)
	}
	if r := f.requestPlaque(t, "Aria Ashwood", "A koi"); r.StatusCode != http.StatusConflict {
		t.Errorf("request after a design = %d; want 409 (one per day, any option)", r.StatusCode)
	}
	// Someone else is unaffected.
	if r := f.requestPlaque(t, "Bea Bright", "A koi"); r.StatusCode != http.StatusCreated {
		t.Errorf("another person's request = %d; want 201", r.StatusCode)
	}
}

func TestEma_SubmitRefusesWhatIsNotAPlaque(t *testing.T) {
	f := newEmaFestival(t)
	if r := f.submitPlaque(t, "Aria Ashwood", []byte("not a png")); r.StatusCode != http.StatusBadRequest {
		t.Errorf("non-PNG = %d; want 400", r.StatusCode)
	}
	if r := f.requestPlaque(t, "Aria Ashwood", "   "); r.StatusCode != http.StatusBadRequest {
		t.Errorf("empty description = %d; want 400", r.StatusCode)
	}
	// A closed board takes nothing.
	resp := f.env.reqJSON(t, http.MethodPatch, fmt.Sprintf("/api/ema-boards/%d", f.boardID), map[string]any{"status": "closed"})
	resp.Body.Close()
	if r := f.requestPlaque(t, "Aria Ashwood", "A koi"); r.StatusCode != http.StatusNotFound {
		t.Errorf("closed board = %d; want 404", r.StatusCode)
	}
}

// approvalHook stands in for Discord, recording what each post carried.
type approvalHook struct {
	mu    sync.Mutex
	posts []string // the multipart form names each post carried
	files [][]byte
}

func (h *approvalHook) serve(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("post wasn't multipart: %v", err)
		}
		h.mu.Lock()
		h.posts = append(h.posts, r.FormValue("payload_json"))
		if f, _, err := r.FormFile("files[0]"); err == nil {
			data, _ := io.ReadAll(f)
			h.files = append(h.files, data)
		}
		h.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestEma_ApprovePostsTheImageThenDeletesIt(t *testing.T) {
	f := newEmaFestival(t)
	img := plaquePNG(t)
	plaque := decodeBody(t, f.submitPlaque(t, "Aria Ashwood", img))["plaque"].(map[string]any)
	stored := f.plaqueFile(plaque["image"].(string))

	hook := &approvalHook{}
	f.env.srv.SetWebhookPolicy(hook.serve(t, http.StatusOK).URL, false)

	path := fmt.Sprintf("/api/ema-boards/%d/plaques/%d/approve", f.boardID, int(plaque["id"].(float64)))
	resp := f.env.postJSON(t, path, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve status = %d; want 200", resp.StatusCode)
	}
	after := decodeBody(t, resp)["plaque"].(map[string]any)
	if after["status"] != "posted" || after["image"] != "" || after["reviewed_by"] != "admin" {
		t.Errorf("after approval = %v; want posted, image cleared, reviewer recorded", after)
	}
	if len(hook.files) != 1 || !bytes.Equal(hook.files[0], img) {
		t.Fatalf("discord got %d file(s); want the plaque's own PNG once", len(hook.files))
	}
	if !strings.Contains(hook.posts[0], "Aria Ashwood @ Gilgamesh") || !strings.Contains(hook.posts[0], "attachment://ema.png") {
		t.Errorf("payload = %s; want the character named and the image attached", hook.posts[0])
	}
	if _, err := os.Stat(stored); !os.IsNotExist(err) {
		t.Errorf("the image is still stored after posting (err = %v); it should be deleted", err)
	}

	// Already posted: a second approval is refused rather than posted again.
	if r := f.env.postJSON(t, path, nil); r.StatusCode != http.StatusConflict {
		t.Errorf("second approval = %d; want 409", r.StatusCode)
	}
	if len(hook.files) != 1 {
		t.Errorf("discord got %d posts; want exactly one", len(hook.files))
	}
}

func TestEma_FailedPostKeepsThePlaquePending(t *testing.T) {
	f := newEmaFestival(t)
	plaque := decodeBody(t, f.submitPlaque(t, "Aria Ashwood", plaquePNG(t)))["plaque"].(map[string]any)
	hook := &approvalHook{}
	f.env.srv.SetWebhookPolicy(hook.serve(t, http.StatusBadRequest).URL, false)

	resp := f.env.postJSON(t, fmt.Sprintf("/api/ema-boards/%d/plaques/%d/approve", f.boardID, int(plaque["id"].(float64))), nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("approve with Discord refusing = %d; want 502", resp.StatusCode)
	}
	resp.Body.Close()
	list := decodeBody(t, f.env.get(t, fmt.Sprintf("/api/ema-boards/%d/plaques", f.boardID)))["plaques"].([]any)
	got := list[0].(map[string]any)
	if got["status"] != "pending" || got["image"] == "" {
		t.Errorf("after a failed post = %v; want it back in the queue with its image", got)
	}
}

func TestEma_RejectDeletesTheImageAndFreesTheDay(t *testing.T) {
	f := newEmaFestival(t)
	plaque := decodeBody(t, f.submitPlaque(t, "Aria Ashwood", plaquePNG(t)))["plaque"].(map[string]any)
	stored := f.plaqueFile(plaque["image"].(string))

	resp := f.env.postJSON(t, fmt.Sprintf("/api/ema-boards/%d/plaques/%d/reject", f.boardID, int(plaque["id"].(float64))), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reject status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	if _, err := os.Stat(stored); !os.IsNotExist(err) {
		t.Errorf("a rejected plaque's image is still stored (err = %v)", err)
	}
	if r := f.submitPlaque(t, "Aria Ashwood", plaquePNG(t)); r.StatusCode != http.StatusCreated {
		t.Errorf("resubmitting after a rejection = %d; want 201", r.StatusCode)
	}
}

func TestEma_ArtistQueueHasItsOwnPermission(t *testing.T) {
	f := newEmaFestival(t)
	req := decodeBody(t, f.requestPlaque(t, "Aria Ashwood", "A koi under the moon"))["plaque"].(map[string]any)
	id := int(req["id"].(float64))

	queue := decodeBody(t, f.env.get(t, "/api/ema-requests"))["plaques"].([]any)
	if len(queue) != 1 || queue[0].(map[string]any)["festival_map_name"] != "Obon Matsuri 2026" {
		t.Fatalf("queue = %v; want the one request, with its festival", queue)
	}
	resp := f.env.reqJSON(t, http.MethodPatch, fmt.Sprintf("/api/ema-requests/%d", id), map[string]any{"complete": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("complete status = %d; want 200", resp.StatusCode)
	}
	if got := decodeBody(t, resp)["plaque"].(map[string]any)["status"]; got != "complete" {
		t.Errorf("status = %v; want complete", got)
	}
}

func TestEma_DashboardShowsTheBoardWithoutWebhooks(t *testing.T) {
	f := newEmaFestival(t)
	f.requestPlaque(t, "Aria Ashwood", "A koi").Body.Close()

	d := f.env.dashboard(t, f.mapID, map[string]any{"name": "Aria Ashwood", "world": "Gilgamesh"})
	ema := d["ema"].(map[string]any)
	if ema["artist_name"] != "Tara" || ema["has_plaque_this_window"] != true {
		t.Errorf("ema = %v; want the board, and today's plaque already taken", ema)
	}
	if int(ema["current_window_id"].(float64)) != f.windowID {
		t.Errorf("current_window_id = %v; want %d", ema["current_window_id"], f.windowID)
	}
	if containsString(d, testWebhook) {
		t.Error("the dashboard response carries a window's webhook URL")
	}
	if len(ema["plaques"].([]any)) != 1 {
		t.Errorf("plaques = %v; want the visitor's one request", ema["plaques"])
	}
}

func TestEma_DeletingTheFestivalRemovesWaitingImages(t *testing.T) {
	f := newEmaFestival(t)
	plaque := decodeBody(t, f.submitPlaque(t, "Aria Ashwood", plaquePNG(t)))["plaque"].(map[string]any)
	stored := f.plaqueFile(plaque["image"].(string))

	resp := f.env.del(t, fmt.Sprintf("/api/festival-maps/%d", f.mapID))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete festival status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()
	if _, err := os.Stat(stored); !os.IsNotExist(err) {
		t.Errorf("a waiting plaque's image outlived its festival (err = %v)", err)
	}
}

func TestGoogleFonts_EmptyWithoutAKey(t *testing.T) {
	env := newTestEnv(t)
	body := decodeBody(t, env.get(t, "/api/google-fonts"))
	if fams, ok := body["families"].([]any); !ok || len(fams) != 0 {
		t.Errorf("families = %v; want an empty list when no key is configured", body["families"])
	}
}
