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
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// -- Ema plaques --------------------------------------------------------------

const testWebhook = "https://discord.com/api/webhooks/1/test-token"

// emaFestival is a published festival with an open plaque board whose one day is
// taking plaques now.
type emaFestival struct {
	mapID, boardID, windowID int
	webRoot                  string
	env                      *testEnv
}

// emaBoardBody is a full board as a save sends it; over replaces fields of it.
func emaBoardBody(mapID int, windows []map[string]any, over map[string]any) map[string]any {
	b := map[string]any{
		"festivalMapId": mapID, "title": "Wishing Wall", "templateImage": "images/ema/template.png",
		"instructions": "Edit and post it.", "discordLink": "", "allowSelf": true, "allowSite": true,
		"allowArtist": true, "artistName": "Tara", "windows": windows,
	}
	for k, v := range over {
		b[k] = v
	}
	return b
}

// day is one day as a save sends it, taking plaques from an hour ago to an hour from now
// unless over says otherwise.
func day(over map[string]any) map[string]any {
	now := time.Now().UTC()
	d := map[string]any{
		"label": "Day 1", "startAt": now.Add(-time.Hour).Format(time.RFC3339),
		"endAt": now.Add(time.Hour).Format(time.RFC3339), "webhookUrl": testWebhook, "artistLimit": 0,
	}
	for k, v := range over {
		d[k] = v
	}
	return d
}

func newEmaFestival(t *testing.T) emaFestival {
	t.Helper()
	webRoot := t.TempDir()
	env := newTestEnvWithWebRoot(t, webRoot)
	env.loginAdmin(t)
	f := emaFestival{mapID: env.createMap(t, "Obon Matsuri 2026"), webRoot: webRoot, env: env}
	env.publishMap(t, f.mapID)

	var board map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/ema-boards", emaBoardBody(f.mapID, []map[string]any{day(nil)}, nil)),
		http.StatusCreated, &board)
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

// form builds a multipart body with the fields and one file part.
func form(fields map[string]string, field, filename string, data []byte) (*bytes.Buffer, string) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if data != nil {
		// As a browser sends a picked or exported picture: with the type it believes it is.
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, filename))
		header.Set("Content-Type", "image/png")
		part, _ := mw.CreatePart(header)
		_, _ = part.Write(data)
	}
	_ = mw.Close()
	return &body, mw.FormDataContentType()
}

// submitPlaque posts a site plaque as `name @ Gilgamesh` and returns the response.
func (f emaFestival) submitPlaque(t *testing.T, name string, img []byte) *http.Response {
	t.Helper()
	body, kind := form(map[string]string{"characterName": name, "world": "Gilgamesh"}, "image", "ema.png", img)
	resp, err := f.env.client.Post(f.env.url(fmt.Sprintf("/api/v1/festivals/%d/ema-plaques", f.mapID)), kind, body)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (f emaFestival) requestPlaque(t *testing.T, name, desc string) *http.Response {
	t.Helper()
	return f.env.postJSON(t, fmt.Sprintf("/api/v1/festivals/%d/ema-requests", f.mapID), map[string]any{
		"characterName": name, "world": "Gilgamesh", "description": desc,
	})
}

// plaqueFile is where a stored plaque image lives on disk.
func (f emaFestival) plaqueFile(rel string) string {
	return filepath.Join(f.webRoot, filepath.FromSlash(rel))
}

// submitted sends a plaque and answers it as the visitor sees it.
func (f emaFestival) submitted(t *testing.T, name string) map[string]any {
	t.Helper()
	var p map[string]any
	decodeInto(t, f.submitPlaque(t, name, plaquePNG(t)), http.StatusCreated, &p)
	return p
}

func boardPath(id int, tail string) string { return fmt.Sprintf("/api/v1/ema-boards/%d", id) + tail }

func TestEma_Guards(t *testing.T) {
	f := newEmaFestival(t)
	plaque := f.submitted(t, "Aria Ashwood")
	id := int(plaque["id"].(float64))
	req := f.requestPlaque(t, "Bea Bright", "A koi")
	reqID := int(decodeBody(t, req)["id"].(float64))

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/ema-boards"},
		{http.MethodPost, "/api/v1/ema-boards"},
		{http.MethodGet, boardPath(f.boardID, "")},
		{http.MethodPut, boardPath(f.boardID, "")},
		{http.MethodPost, boardPath(f.boardID, "/close")},
		{http.MethodPost, boardPath(f.boardID, "/reopen")},
		{http.MethodDelete, boardPath(f.boardID, "")},
		{http.MethodGet, boardPath(f.boardID, "/plaques")},
		{http.MethodPost, boardPath(f.boardID, fmt.Sprintf("/plaques/%d/approve", id))},
		{http.MethodPost, boardPath(f.boardID, fmt.Sprintf("/plaques/%d/reject", id))},
		{http.MethodGet, "/api/v1/ema-requests"},
		{http.MethodPost, fmt.Sprintf("/api/v1/ema-requests/%d/complete", reqID)},
		{http.MethodPost, fmt.Sprintf("/api/v1/ema-requests/%d/reopen", reqID)},
	}
	anon := newClient(t, f.env)
	boards := makeActiveUser(t, f.env, "boards", "correct horse battery", []string{"festival-ema"})
	queue := makeActiveUser(t, f.env, "queue", "correct horse battery", []string{"festival-ema-requests"})
	for _, r := range routes {
		isQueue := strings.Contains(r.path, "/ema-requests")
		resp := reqAs(t, anon, f.env, r.method, r.path, map[string]any{})
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s signed out = %d; want 401", r.method, r.path, resp.StatusCode)
		}
		wrong, right := queue, boards
		if isQueue {
			wrong, right = boards, queue
		}
		resp = reqAs(t, wrong, f.env, r.method, r.path, map[string]any{})
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s with the other page's access = %d; want 403", r.method, r.path, resp.StatusCode)
		}
		if r.method == http.MethodGet {
			resp = getAs(t, right, f.env, r.path)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET %s with its own page = %d; want 200", r.path, resp.StatusCode)
			}
		}
	}
}

func TestEma_OneBoardPerFestivalAndTheFestivalMustExist(t *testing.T) {
	f := newEmaFestival(t)

	resp := f.env.postJSON(t, "/api/v1/ema-boards", emaBoardBody(f.mapID, []map[string]any{day(nil)}, nil))
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("a second board for one festival = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()

	resp = f.env.postJSON(t, "/api/v1/ema-boards", emaBoardBody(9999, []map[string]any{day(nil)}, nil))
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("a board for a festival that doesn't exist = %d; want 422", resp.StatusCode)
	}
	if errs := problemOf(t, resp)["errors"].([]any); errs[0].(map[string]any)["location"] != "body.festivalMapId" {
		t.Errorf("errors = %v; want body.festivalMapId", errs)
	}

	// Moving this board onto a festival that has another's is a conflict too.
	other := f.env.createMap(t, "Other")
	var made map[string]any
	decodeInto(t, f.env.postJSON(t, "/api/v1/ema-boards", emaBoardBody(other, []map[string]any{day(nil)}, nil)),
		http.StatusCreated, &made)
	resp = f.env.putJSON(t, boardPath(f.boardID, ""), emaBoardBody(other, []map[string]any{day(nil)}, nil))
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("moving onto a festival with a board = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestEma_BoardSaveValidates(t *testing.T) {
	f := newEmaFestival(t)
	start, end := "2026-08-01T00:00:00Z", "2026-08-02T00:00:00Z"
	cases := map[string]struct {
		body  map[string]any
		field string
	}{
		"overlapping days": {emaBoardBody(f.mapID, []map[string]any{
			day(map[string]any{"label": "Day 1", "startAt": start, "endAt": "2026-08-02T06:00:00Z"}),
			day(map[string]any{"label": "Day 2", "startAt": end, "endAt": "2026-08-03T00:00:00Z"}),
		}, nil), "body.windows"},
		"ends before it starts": {emaBoardBody(f.mapID, []map[string]any{
			day(map[string]any{"startAt": end, "endAt": start})}, nil), "body.windows[0].endAt"},
		"not a discord webhook": {emaBoardBody(f.mapID, []map[string]any{
			day(map[string]any{"startAt": start, "endAt": end, "webhookUrl": "https://example.com/hook"})}, nil), "body.windows[0].webhookUrl"},
		"no days": {emaBoardBody(f.mapID, []map[string]any{}, nil), ""},
		"negative artist limit": {emaBoardBody(f.mapID, []map[string]any{
			day(map[string]any{"startAt": start, "endAt": end, "artistLimit": -1})}, nil), ""},
		"artist limit past the cap": {emaBoardBody(f.mapID, []map[string]any{
			day(map[string]any{"startAt": start, "endAt": end, "artistLimit": 1001})}, nil), ""},
		"no way to make a plaque": {emaBoardBody(f.mapID, []map[string]any{day(nil)},
			map[string]any{"allowSelf": false, "allowSite": false, "allowArtist": false}), "body.allowSelf"},
		"no template": {emaBoardBody(f.mapID, []map[string]any{day(nil)}, map[string]any{"templateImage": ""}), "body.templateImage"},
		"discord link not a web address": {emaBoardBody(f.mapID, []map[string]any{day(nil)},
			map[string]any{"discordLink": "javascript:alert(1)"}), "body.discordLink"},
	}
	for name, c := range cases {
		resp := f.env.putJSON(t, boardPath(f.boardID, ""), c.body)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d; want 422", name, resp.StatusCode)
			resp.Body.Close()
			continue
		}
		errs, _ := problemOf(t, resp)["errors"].([]any)
		if c.field != "" && (len(errs) == 0 || errs[0].(map[string]any)["location"] != c.field) {
			t.Errorf("%s: errors = %v; want one at %s", name, errs, c.field)
		}
	}

	// A day that isn't on this board, and a board that isn't there.
	resp := f.env.putJSON(t, boardPath(f.boardID, ""), emaBoardBody(f.mapID, []map[string]any{day(map[string]any{"id": 9999})}, nil))
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("a day from elsewhere = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
	resp = f.env.putJSON(t, boardPath(9999, ""), emaBoardBody(f.mapID, []map[string]any{day(nil)}, nil))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("saving a board that isn't there = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestEma_BoardStatusAndUnknownIDs(t *testing.T) {
	f := newEmaFestival(t)
	var closed map[string]any
	decodeInto(t, f.env.postJSON(t, boardPath(f.boardID, "/close"), nil), http.StatusOK, &closed)
	if closed["status"] != "closed" {
		t.Errorf("status after close = %v; want closed", closed["status"])
	}
	var reopened map[string]any
	decodeInto(t, f.env.postJSON(t, boardPath(f.boardID, "/reopen"), nil), http.StatusOK, &reopened)
	if reopened["status"] != "open" {
		t.Errorf("status after reopen = %v; want open", reopened["status"])
	}
	for _, r := range []struct{ method, tail string }{
		{http.MethodGet, ""}, {http.MethodPost, "/close"}, {http.MethodPost, "/reopen"},
		{http.MethodDelete, ""}, {http.MethodGet, "/plaques"},
	} {
		resp := f.env.reqJSON(t, r.method, boardPath(9999, r.tail), nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s of an unknown board%s = %d; want 404", r.method, r.tail, resp.StatusCode)
		}
	}
}

func TestEma_OnePlaquePerPersonPerDayAcrossOptions(t *testing.T) {
	f := newEmaFestival(t)

	plaque := f.submitted(t, "Aria Ashwood")
	if plaque["status"] != "pending" || plaque["windowLabel"] != "Day 1" || plaque["kind"] != "site" {
		t.Errorf("plaque = %v; want a pending site plaque on Day 1", plaque)
	}
	if _, err := os.Stat(f.plaqueFile(plaque["image"].(string))); err != nil {
		t.Errorf("the plaque image wasn't kept for review: %v", err)
	}

	// Same person, same day: neither another design nor a request to the artist.
	if r := f.submitPlaque(t, "aria ashwood", plaquePNG(t)); r.StatusCode != http.StatusConflict {
		t.Errorf("second design = %d; want 409", r.StatusCode)
	}
	files, _ := os.ReadDir(filepath.Join(f.webRoot, "images", "ema_plaques"))
	if len(files) != 1 {
		t.Errorf("%d plaque files kept; want the refused one removed again", len(files))
	}
	resp := f.requestPlaque(t, "Aria Ashwood", "A koi")
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("request after a design = %d; want 409 (one per day, any option)", resp.StatusCode)
	}
	if msg := problemOf(t, resp)["detail"]; msg != "You already have a plaque for Day 1 - one per day." {
		t.Errorf("detail = %v", msg)
	}
	// Someone else is unaffected.
	if r := f.requestPlaque(t, "Bea Bright", "A koi"); r.StatusCode != http.StatusCreated {
		t.Errorf("another person's request = %d; want 201", r.StatusCode)
	}
}

func TestEma_SubmitRefusesWhatIsNotAPlaque(t *testing.T) {
	f := newEmaFestival(t)
	refused := func(name string, resp *http.Response, want int) {
		t.Helper()
		if resp.StatusCode != want {
			t.Errorf("%s = %d; want %d", name, resp.StatusCode, want)
		}
		resp.Body.Close()
	}
	refused("non-PNG", f.submitPlaque(t, "Aria Ashwood", []byte("not a png")), http.StatusUnprocessableEntity)
	refused("an unnamed visitor", f.submitPlaque(t, "  ", plaquePNG(t)), http.StatusUnprocessableEntity)
	body, kind := form(map[string]string{"characterName": "Aria", "world": "Gilgamesh"}, "", "", nil)
	resp, _ := f.env.client.Post(f.env.url(fmt.Sprintf("/api/v1/festivals/%d/ema-plaques", f.mapID)), kind, body)
	refused("no file", resp, http.StatusUnprocessableEntity)
	refused("a request with no description", f.requestPlaque(t, "Aria Ashwood", "   "), http.StatusUnprocessableEntity)
	refused("a request with no world", f.env.postJSON(t, fmt.Sprintf("/api/v1/festivals/%d/ema-requests", f.mapID),
		map[string]any{"characterName": "Aria", "world": "", "description": "A koi"}), http.StatusUnprocessableEntity)
	refused("a festival that isn't there", f.env.postJSON(t, "/api/v1/festivals/9999/ema-requests",
		map[string]any{"characterName": "Aria", "world": "Gilgamesh", "description": "A koi"}), http.StatusNotFound)

	// A plaque too big in pixels is refused before it is kept.
	var big bytes.Buffer
	_ = png.Encode(&big, image.NewRGBA(image.Rect(0, 0, 4100, 4)))
	refused("an oversized plaque", f.submitPlaque(t, "Aria Ashwood", big.Bytes()), http.StatusUnprocessableEntity)
	// A body past the cap never reaches the handler.
	refused("a body past the cap", f.submitPlaque(t, "Aria Ashwood", make([]byte, 14<<20)), http.StatusRequestEntityTooLarge)

	// A day not open now: no plaques, whatever the board offers.
	f.env.putJSON(t, boardPath(f.boardID, ""), emaBoardBody(f.mapID, []map[string]any{day(map[string]any{
		"startAt": time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339),
		"endAt":   time.Now().UTC().Add(72 * time.Hour).Format(time.RFC3339),
	})}, nil)).Body.Close()
	refused("a plaque between days", f.submitPlaque(t, "Aria Ashwood", plaquePNG(t)), http.StatusConflict)
	refused("a request between days", f.requestPlaque(t, "Aria Ashwood", "A koi"), http.StatusConflict)

}

// A closed board takes nothing, and neither does one that doesn't offer the way.
func TestEma_ClosedBoardOrMissingWayTakesNothing(t *testing.T) {
	f := newEmaFestival(t)
	refused := func(name string, resp *http.Response, want int) {
		t.Helper()
		if resp.StatusCode != want {
			t.Errorf("%s = %d; want %d", name, resp.StatusCode, want)
		}
		resp.Body.Close()
	}
	f.env.postJSON(t, boardPath(f.boardID, "/close"), nil).Body.Close()
	refused("a closed board", f.requestPlaque(t, "Aria Ashwood", "A koi"), http.StatusNotFound)
	f.env.postJSON(t, boardPath(f.boardID, "/reopen"), nil).Body.Close()
	f.env.putJSON(t, boardPath(f.boardID, ""), emaBoardBody(f.mapID, []map[string]any{day(nil)},
		map[string]any{"allowArtist": false})).Body.Close()
	refused("a way the board doesn't offer", f.requestPlaque(t, "Aria Ashwood", "A koi"), http.StatusNotFound)
}

func TestEma_PublicSubmissionsAreBudgeted(t *testing.T) {
	f := newEmaFestival(t)
	var last *http.Response
	for i := range 11 {
		last = f.requestPlaque(t, fmt.Sprintf("Visitor %d", i), "A koi")
		if i < 10 {
			last.Body.Close()
		}
	}
	if last.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("the eleventh try = %d; want 429", last.StatusCode)
	}
	if last.Header.Get("Retry-After") == "" {
		t.Error("the 429 doesn't say when to try again")
	}
	last.Body.Close()
}

// approvalHook stands in for Discord, recording what each post carried.
type approvalHook struct {
	mu    sync.Mutex
	posts []string // the payload each post carried
	files [][]byte
}

func (h *approvalHook) serve(t *testing.T, status int, header http.Header) *httptest.Server {
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
		for k, v := range header {
			w.Header()[k] = v
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestEma_ApprovePostsTheImageThenDeletesIt(t *testing.T) {
	f := newEmaFestival(t)
	img := plaquePNG(t)
	var plaque map[string]any
	decodeInto(t, f.submitPlaque(t, "Aria Ashwood", img), http.StatusCreated, &plaque)
	stored := f.plaqueFile(plaque["image"].(string))

	hook := &approvalHook{}
	f.env.srv.SetWebhookPolicy(hook.serve(t, http.StatusOK, nil).URL, false)

	path := boardPath(f.boardID, fmt.Sprintf("/plaques/%d/approve", int(plaque["id"].(float64))))
	var after map[string]any
	decodeInto(t, f.env.postJSON(t, path, nil), http.StatusOK, &after)
	if after["status"] != "posted" || after["image"] != "" || after["reviewedBy"] != "admin" || after["reviewedAt"] == nil {
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

	// A plaque of another board, and an unknown one, are not found.
	for _, p := range []string{boardPath(9999, fmt.Sprintf("/plaques/%d/approve", int(plaque["id"].(float64)))), boardPath(f.boardID, "/plaques/9999/approve")} {
		resp := f.env.postJSON(t, p, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("POST %s = %d; want 404", p, resp.StatusCode)
		}
	}
}

func TestEma_FailedPostKeepsThePlaquePendingAndAnswersTheFailure(t *testing.T) {
	cases := []struct {
		name   string
		status int
		header http.Header
		want   int
		retry  string
	}{
		{"refused", http.StatusBadRequest, nil, http.StatusInternalServerError, ""},
		{"rate limited", http.StatusTooManyRequests, http.Header{"Retry-After": {"7"}}, http.StatusTooManyRequests, "7"},
		{"Discord down", http.StatusBadGateway, nil, http.StatusServiceUnavailable, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newEmaFestival(t)
			var plaque map[string]any
			decodeInto(t, f.submitPlaque(t, "Aria Ashwood", plaquePNG(t)), http.StatusCreated, &plaque)
			hook := &approvalHook{}
			f.env.srv.SetWebhookPolicy(hook.serve(t, c.status, c.header).URL, false)

			resp := f.env.postJSON(t, boardPath(f.boardID, fmt.Sprintf("/plaques/%d/approve", int(plaque["id"].(float64)))), nil)
			if resp.StatusCode != c.want || resp.Header.Get("Retry-After") != c.retry {
				t.Errorf("approve with Discord %s = %d (Retry-After %q); want %d (%q)", c.name, resp.StatusCode,
					resp.Header.Get("Retry-After"), c.want, c.retry)
			}
			resp.Body.Close()
			var list map[string]any
			decodeInto(t, f.env.get(t, boardPath(f.boardID, "/plaques")), http.StatusOK, &list)
			got := list["plaques"].([]any)[0].(map[string]any)
			if got["status"] != "pending" || got["image"] == "" {
				t.Errorf("after a failed post = %v; want it back in the queue with its image", got)
			}
		})
	}
}

func TestEma_ApproveNeedsTheDaysWebhook(t *testing.T) {
	f := newEmaFestival(t)
	plaque := f.submitted(t, "Aria Ashwood")
	f.env.putJSON(t, boardPath(f.boardID, ""), emaBoardBody(f.mapID, []map[string]any{
		day(map[string]any{"id": f.windowID, "webhookUrl": ""})}, nil)).Body.Close()

	resp := f.env.postJSON(t, boardPath(f.boardID, fmt.Sprintf("/plaques/%d/approve", int(plaque["id"].(float64)))), nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("approving with no webhook = %d; want 500 (the server's configuration)", resp.StatusCode)
	}
	if msg := problemOf(t, resp)["detail"]; msg != "Set a webhook for Day 1 before approving its plaques." {
		t.Errorf("detail = %v", msg)
	}
}

func TestEma_RejectDeletesTheImageAndFreesTheDay(t *testing.T) {
	f := newEmaFestival(t)
	plaque := f.submitted(t, "Aria Ashwood")
	stored := f.plaqueFile(plaque["image"].(string))
	path := boardPath(f.boardID, fmt.Sprintf("/plaques/%d/reject", int(plaque["id"].(float64))))

	var after map[string]any
	decodeInto(t, f.env.postJSON(t, path, nil), http.StatusOK, &after)
	if after["status"] != "rejected" {
		t.Errorf("status = %v; want rejected", after["status"])
	}
	if _, err := os.Stat(stored); !os.IsNotExist(err) {
		t.Errorf("a rejected plaque's image is still stored (err = %v)", err)
	}
	if r := f.env.postJSON(t, path, nil); r.StatusCode != http.StatusConflict {
		t.Errorf("rejecting twice = %d; want 409", r.StatusCode)
	}
	if r := f.submitPlaque(t, "Aria Ashwood", plaquePNG(t)); r.StatusCode != http.StatusCreated {
		t.Errorf("resubmitting after a rejection = %d; want 201", r.StatusCode)
	}
}

func TestEma_ArtistQueueCompleteAndReopen(t *testing.T) {
	f := newEmaFestival(t)
	req := decodeBody(t, f.requestPlaque(t, "Aria Ashwood", "A koi under the moon"))
	if req["status"] != "requested" || req["kind"] != "artist" {
		t.Fatalf("request = %v; want a requested artist plaque", req)
	}
	id := int(req["id"].(float64))
	site := f.submitted(t, "Bea Bright")

	var queue map[string]any
	decodeInto(t, f.env.get(t, "/api/v1/ema-requests"), http.StatusOK, &queue)
	plaques := queue["plaques"].([]any)
	if len(plaques) != 1 || plaques[0].(map[string]any)["festivalMapName"] != "Obon Matsuri 2026" {
		t.Fatalf("queue = %v; want the one request, with its festival", plaques)
	}

	reopen := fmt.Sprintf("/api/v1/ema-requests/%d/reopen", id)
	complete := fmt.Sprintf("/api/v1/ema-requests/%d/complete", id)
	if r := f.env.postJSON(t, reopen, nil); r.StatusCode != http.StatusConflict {
		t.Errorf("reopening an open request = %d; want 409", r.StatusCode)
	}
	var done map[string]any
	decodeInto(t, f.env.postJSON(t, complete, nil), http.StatusOK, &done)
	if done["status"] != "complete" || done["reviewedBy"] != "admin" || done["reviewedAt"] == nil {
		t.Errorf("completed = %v; want complete, with who and when", done)
	}
	if r := f.env.postJSON(t, complete, nil); r.StatusCode != http.StatusConflict {
		t.Errorf("completing twice = %d; want 409", r.StatusCode)
	}
	var back map[string]any
	decodeInto(t, f.env.postJSON(t, reopen, nil), http.StatusOK, &back)
	if back["status"] != "requested" || back["reviewedAt"] != nil || back["reviewedBy"] != "" {
		t.Errorf("reopened = %v; want requested again, with no reviewer", back)
	}

	// A site plaque is not an artist request, and an unknown id is not found.
	for _, p := range []string{
		fmt.Sprintf("/api/v1/ema-requests/%d/complete", int(site["id"].(float64))), "/api/v1/ema-requests/9999/complete",
	} {
		resp := f.env.postJSON(t, p, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("POST %s = %d; want 404", p, resp.StatusCode)
		}
	}
}

// setArtistLimit re-saves the board with Day 1 (kept by id) taking at most `limit`
// artist requests.
func (f emaFestival) setArtistLimit(t *testing.T, limit int) {
	t.Helper()
	decodeInto(t, f.env.putJSON(t, boardPath(f.boardID, ""), emaBoardBody(f.mapID, []map[string]any{
		day(map[string]any{"id": f.windowID, "artistLimit": limit})}, nil)), http.StatusOK, nil)
}

func TestEma_ArtistRequestsStopAtTheDaysLimit(t *testing.T) {
	f := newEmaFestival(t)
	f.setArtistLimit(t, 2)

	for _, name := range []string{"Aria Ashwood", "Bea Bright"} {
		if r := f.requestPlaque(t, name, "A koi"); r.StatusCode != http.StatusCreated {
			t.Fatalf("%s's request = %d; want 201", name, r.StatusCode)
		}
	}
	r := f.requestPlaque(t, "Cass Cole", "A koi")
	if r.StatusCode != http.StatusConflict {
		t.Fatalf("request past the limit = %d; want 409", r.StatusCode)
	}
	if msg := problemOf(t, r)["detail"]; msg != "Tara's requests for Day 1 are full." {
		t.Errorf("detail = %q; want the full message", msg)
	}
	// The limit is the artist's: designing on the site is still open.
	if r := f.submitPlaque(t, "Cass Cole", plaquePNG(t)); r.StatusCode != http.StatusCreated {
		t.Errorf("site plaque on a full artist day = %d; want 201", r.StatusCode)
	}

	var board map[string]any
	decodeInto(t, f.env.get(t, boardPath(f.boardID, "")), http.StatusOK, &board)
	d := board["windows"].([]any)[0].(map[string]any)
	if d["artistLimit"] != float64(2) || d["artistCount"] != float64(2) {
		t.Errorf("staff see the day as %v; want limit 2, count 2", d)
	}
	ema := f.env.dashboard(t, f.mapID, map[string]any{"characterName": "Dee Dunn", "world": "Gilgamesh"})["ema"].(map[string]any)
	pub := ema["windows"].([]any)[0].(map[string]any)
	if pub["artistLimit"] != float64(2) || pub["artistLeft"] != float64(0) {
		t.Errorf("visitors see the day as %v; want limit 2, none left", pub)
	}

	// Raising the limit reopens the day.
	f.setArtistLimit(t, 3)
	if r := f.requestPlaque(t, "Dee Dunn", "A koi"); r.StatusCode != http.StatusCreated {
		t.Errorf("request after raising the limit = %d; want 201", r.StatusCode)
	}
}

// postFinished uploads the artist's finished plaque for request `id` through
// `client` (the logged-in admin's, or another).
func (f emaFestival) postFinished(t *testing.T, client *http.Client, id int, img []byte) *http.Response {
	t.Helper()
	body, kind := form(nil, "image", "finished.png", img)
	resp, err := client.Post(f.env.url(fmt.Sprintf("/api/v1/ema-requests/%d/post", id)), kind, body)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestEma_ArtistPostsTheFinishedPlaqueWithoutKeepingIt(t *testing.T) {
	f := newEmaFestival(t)
	id := int(decodeBody(t, f.requestPlaque(t, "Aria Ashwood", "A koi"))["id"].(float64))
	hook := &approvalHook{}
	f.env.srv.SetWebhookPolicy(hook.serve(t, http.StatusOK, nil).URL, false)
	img := plaquePNG(t)

	anon := newClient(t, f.env)
	if r := f.postFinished(t, anon, id, img); r.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous post = %d; want 401", r.StatusCode)
	}
	if r := f.postFinished(t, f.env.client, id, []byte("not an image")); r.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("non-image post = %d; want 422", r.StatusCode)
	}
	if r := f.postFinished(t, f.env.client, 9999, img); r.StatusCode != http.StatusNotFound {
		t.Errorf("post for an unknown request = %d; want 404", r.StatusCode)
	}
	if r := f.postFinished(t, f.env.client, id, make([]byte, 11<<20)); r.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("post past Discord's limit = %d; want 413", r.StatusCode)
	}
	if len(hook.posts) != 0 {
		t.Fatalf("discord got %d posts before a valid one; want none", len(hook.posts))
	}

	var after map[string]any
	decodeInto(t, f.postFinished(t, f.env.client, id, img), http.StatusOK, &after)
	if after["status"] != "complete" || after["reviewedBy"] != "admin" {
		t.Errorf("after posting = %v; want complete, with who posted it", after)
	}
	if len(hook.files) != 1 || !bytes.Equal(hook.files[0], img) {
		t.Fatalf("discord got %d file(s); want the finished plaque once", len(hook.files))
	}
	if want := "Requested by **Aria Ashwood @ Gilgamesh**, made by **Tara** - Day 1"; !strings.Contains(hook.posts[0], want) {
		t.Errorf("payload = %s; want %q", hook.posts[0], want)
	}
	// Posted and dropped: no copy of it anywhere under the web root.
	_ = filepath.WalkDir(f.webRoot, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if data, _ := os.ReadFile(p); bytes.Equal(data, img) { //nolint:gosec // a test walking its own temp dir
				t.Errorf("the finished plaque was kept on the server at %s", p)
			}
		}
		return nil
	})

	// Finished: a second post is refused rather than posted again.
	if r := f.postFinished(t, f.env.client, id, img); r.StatusCode != http.StatusConflict {
		t.Errorf("second post = %d; want 409", r.StatusCode)
	}
	if len(hook.files) != 1 {
		t.Errorf("discord got %d posts; want exactly one", len(hook.files))
	}
}

func TestEma_FailedArtistPostReopensTheRequest(t *testing.T) {
	f := newEmaFestival(t)
	id := int(decodeBody(t, f.requestPlaque(t, "Aria Ashwood", "A koi"))["id"].(float64))
	hook := &approvalHook{}
	f.env.srv.SetWebhookPolicy(hook.serve(t, http.StatusBadRequest, nil).URL, false)

	resp := f.postFinished(t, f.env.client, id, plaquePNG(t))
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("post with Discord refusing = %d; want 500", resp.StatusCode)
	}
	resp.Body.Close()
	var queue map[string]any
	decodeInto(t, f.env.get(t, "/api/v1/ema-requests"), http.StatusOK, &queue)
	if got := queue["plaques"].([]any)[0].(map[string]any)["status"]; got != "requested" {
		t.Errorf("after a failed post the request is %v; want requested, to try again", got)
	}
}

func TestEma_DashboardShowsTheBoardWithoutWebhooks(t *testing.T) {
	f := newEmaFestival(t)
	f.requestPlaque(t, "Aria Ashwood", "A koi").Body.Close()

	d := f.env.dashboard(t, f.mapID, map[string]any{"characterName": "Aria Ashwood", "world": "Gilgamesh"})
	ema := d["ema"].(map[string]any)
	if ema["artistName"] != "Tara" || ema["hasPlaqueThisWindow"] != true {
		t.Errorf("ema = %v; want the board, and today's plaque already taken", ema)
	}
	if int(ema["currentWindowId"].(float64)) != f.windowID {
		t.Errorf("currentWindowId = %v; want %d", ema["currentWindowId"], f.windowID)
	}
	if containsString(d, testWebhook) {
		t.Error("the dashboard response carries a day's webhook URL")
	}
	if len(ema["plaques"].([]any)) != 1 {
		t.Errorf("plaques = %v; want the visitor's one request", ema["plaques"])
	}
	// A visitor who sent no name sees the board and none of its plaques.
	anon := f.env.dashboard(t, f.mapID, map[string]any{})["ema"].(map[string]any)
	if len(anon["plaques"].([]any)) != 0 {
		t.Errorf("anonymous plaques = %v; want none", anon["plaques"])
	}
	// A closed board leaves the dashboard.
	f.env.postJSON(t, boardPath(f.boardID, "/close"), nil).Body.Close()
	if closed := f.env.dashboard(t, f.mapID, map[string]any{})["ema"]; closed != nil {
		t.Errorf("a closed board = %v; want it off the dashboard", closed)
	}
}

func TestEma_DeletingTheBoardOrTheFestivalRemovesWaitingImages(t *testing.T) {
	f := newEmaFestival(t)
	first := f.submitted(t, "Aria Ashwood")
	stored := f.plaqueFile(first["image"].(string))
	resp := f.env.del(t, boardPath(f.boardID, ""))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete board status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()
	if _, err := os.Stat(stored); !os.IsNotExist(err) {
		t.Errorf("a waiting plaque's image outlived its board (err = %v)", err)
	}

	g := newEmaFestival(t)
	second := g.submitted(t, "Aria Ashwood")
	stored = g.plaqueFile(second["image"].(string))
	resp = g.env.del(t, fmt.Sprintf("/api/v1/festival-maps/%d", g.mapID))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete festival status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()
	if _, err := os.Stat(stored); !os.IsNotExist(err) {
		t.Errorf("a waiting plaque's image outlived its festival (err = %v)", err)
	}
	if r := g.env.get(t, boardPath(g.boardID, "")); r.StatusCode != http.StatusNotFound {
		t.Errorf("the board after its festival went = %d; want 404", r.StatusCode)
	}
}

func TestEma_SubmissionsTellTheirQueues(t *testing.T) {
	f := newEmaFestival(t)
	s := staffStream(t, f.env, f.env.client)

	f.submitted(t, "Aria Ashwood")
	if e := s.expect("resource_changed", 3*time.Second); e.str("resource") != "ema-boards" {
		t.Errorf("a site plaque told %+v; want ema-boards", e)
	}
	f.requestPlaque(t, "Bea Bright", "A koi").Body.Close()
	if e := s.expect("resource_changed", 3*time.Second); e.str("resource") != "ema-requests" {
		t.Errorf("an artist request told %+v; want ema-requests", e)
	}
}

func TestGoogleFonts_EmptyWithoutAKey(t *testing.T) {
	env := newTestEnv(t)
	var body map[string]any
	decodeInto(t, env.get(t, "/api/v1/google-fonts"), http.StatusOK, &body)
	if fams, ok := body["families"].([]any); !ok || len(fams) != 0 {
		t.Errorf("families = %v; want an empty list when no key is configured", body["families"])
	}
}

// The artist's picture is saved again before it goes to Discord, so a phone photo
// loses its location and camera; a plaque designed on the site must be a PNG.
func TestEma_PicturesAreCheckedAndSavedAgain(t *testing.T) {
	f := newEmaFestival(t)
	id := int(decodeBody(t, f.requestPlaque(t, "Aria Ashwood", "A koi"))["id"].(float64))
	hook := &approvalHook{}
	f.env.srv.SetWebhookPolicy(hook.serve(t, http.StatusOK, nil).URL, false)

	photo := exifJPEG(t, 8, 4, 6)
	decodeInto(t, f.postFinished(t, f.env.client, id, photo), http.StatusOK, nil)
	if len(hook.files) != 1 || bytes.Contains(hook.files[0], []byte("Exif")) {
		t.Fatalf("discord got %d file(s); want one with its EXIF dropped", len(hook.files))
	}
	if !strings.Contains(hook.posts[0], "attachment://ema.jpg") {
		t.Errorf("payload = %s; want the JPEG attached as ema.jpg", hook.posts[0])
	}

	// A JPEG isn't a site plaque, and neither is a PNG with a side over 4096.
	resp := f.submitPlaque(t, "Bria Brook", photo)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("a JPEG plaque = %d; want 422", resp.StatusCode)
	}
}
