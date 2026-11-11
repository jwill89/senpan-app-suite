package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"app-suite/internal/db"
	"app-suite/internal/server"
)

const teaRoomWebhook = "https://discord.com/api/webhooks/1/tea-room-token"

// teaRoomBody is a full tea room save with the given name and number.
func teaRoomBody(name, number string) map[string]any {
	return map[string]any{
		"name": name, "subtitle": "", "roomOwner": "", "roomNumber": number, "costPerHalfHour": 125000,
		"hashtags": "cozy, private", "description": "A quiet room.", "seasonal": false, "open": true,
		"lockable": true, "discounted": false, "locked": false, "lockedUntil": nil, "image": "", "color": "",
	}
}

// makeTeaRoom saves a room through the API and returns it.
func makeTeaRoom(t *testing.T, env *testEnv, name, number string) map[string]any {
	t.Helper()
	var room map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/tea-rooms", teaRoomBody(name, number)), http.StatusCreated, &room)
	return room
}

func teaRoomPath(room map[string]any) string {
	return fmt.Sprintf("/api/v1/tea-rooms/%d", int64(room["id"].(float64)))
}

// patchTeaRoom changes a room and returns the saved room.
func patchTeaRoom(t *testing.T, env *testEnv, room map[string]any, fields map[string]any) map[string]any {
	t.Helper()
	var saved map[string]any
	decodeInto(t, env.patchJSON(t, teaRoomPath(room), fields), http.StatusOK, &saved)
	return saved
}

// The Tea Rooms page's permission guards every admin operation; the public reads
// need nothing.
func TestTeaRoomsGuard(t *testing.T) {
	env := newTestEnv(t)
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/tea-rooms"}, {http.MethodPost, "/api/v1/tea-rooms"},
		{http.MethodPut, "/api/v1/tea-rooms/webhook"}, {http.MethodPost, "/api/v1/tea-rooms/1/post"},
	} {
		if got := status(env.reqJSON(t, call.method, call.path, map[string]any{})); got != http.StatusUnauthorized {
			t.Errorf("%s %s signed out: %d, want 401", call.method, call.path, got)
		}
	}
	if got := status(env.get(t, "/api/v1/tea-rooms/public")); got != http.StatusOK {
		t.Errorf("public list signed out: %d, want 200", got)
	}

	addAccount(t, env, "presetsonly", "correct horse battery", []string{"bingo-presets"})
	env.signIn(t, "presetsonly", "correct horse battery")
	if got := status(env.get(t, "/api/v1/tea-rooms")); got != http.StatusForbidden {
		t.Errorf("without the page: %d, want 403", got)
	}
	if got := status(env.postJSON(t, "/api/v1/tea-rooms", teaRoomBody("X", "1"))); got != http.StatusForbidden {
		t.Errorf("a save without the page: %d, want 403", got)
	}

	addAccount(t, env, "hostess", "correct horse battery", []string{"teahouse-tea-rooms"})
	env.signIn(t, "hostess", "correct horse battery")
	makeTeaRoom(t, env, "Mine", "1")
}

// A room is made (201, in full, trimmed, hashtags normalized), listed with no
// webhook, replaced and deleted (204, then 404). A room number another room has is
// a 409 on a create and a replace, and a blank name or number a 422.
func TestTeaRoomsLifecycle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	body := teaRoomBody("  Jasmine Room ", " 12 ")
	body["hashtags"] = "cozy, #Private cozy"
	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/tea-rooms", body), http.StatusCreated, &made)
	if made["name"] != "Jasmine Room" || made["roomNumber"] != "12" || made["hashtags"] != "#cozy #Private" ||
		made["costPerHalfHour"] != float64(125000) || made["open"] != true || made["lockedUntil"] != nil ||
		made["createdAt"] == nil || made["sortOrder"] != float64(0) {
		t.Errorf("made: %v", made)
	}
	path := teaRoomPath(made)

	if got := status(env.postJSON(t, "/api/v1/tea-rooms", teaRoomBody("Copy", "12"))); got != http.StatusConflict {
		t.Errorf("a repeated room number on create: %d, want 409", got)
	}
	other := makeTeaRoom(t, env, "Peony Room", "13")
	if got := status(env.putJSON(t, teaRoomPath(other), teaRoomBody("Peony Room", "12"))); got != http.StatusConflict {
		t.Errorf("taking another room's number on replace: %d, want 409", got)
	}

	var list struct {
		TeaRooms []map[string]any `json:"teaRooms"`
	}
	resp := env.get(t, "/api/v1/tea-rooms")
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(raw), "webhook") {
		t.Errorf("the list carries the webhook: %s", raw)
	}
	if err := json.Unmarshal(raw, &list); err != nil || len(list.TeaRooms) != 2 {
		t.Fatalf("list: %v %s", err, raw)
	}

	replace := teaRoomBody("Renamed", "12")
	replace["discounted"] = true
	var saved map[string]any
	decodeInto(t, env.putJSON(t, path, replace), http.StatusOK, &saved)
	if saved["name"] != "Renamed" || saved["discounted"] != true || saved["id"] != made["id"] {
		t.Errorf("replaced: %v", saved)
	}

	if got := status(env.putJSON(t, "/api/v1/tea-rooms/999", teaRoomBody("Lost", "99"))); got != http.StatusNotFound {
		t.Errorf("replacing an unknown room: %d, want 404", got)
	}
	if got := status(env.patchJSON(t, "/api/v1/tea-rooms/999", map[string]any{"open": false})); got != http.StatusNotFound {
		t.Errorf("changing an unknown room: %d, want 404", got)
	}
	if got := status(env.postJSON(t, "/api/v1/tea-rooms/999/post", nil)); got != http.StatusNotFound {
		t.Errorf("posting an unknown room: %d, want 404", got)
	}

	for _, c := range []struct {
		body map[string]any
		want string
	}{
		{teaRoomBody(" ", "1"), "body.name"},
		{teaRoomBody("Fine", " "), "body.roomNumber"},
	} {
		if at := invalid(t, env.postJSON(t, "/api/v1/tea-rooms", c.body)); !slices.Contains(at, c.want) {
			t.Errorf("create %v: errors at %v, want %s", c.body, at, c.want)
		}
	}
	negative := teaRoomBody("Fine", "5")
	negative["costPerHalfHour"] = -1
	if at := invalid(t, env.postJSON(t, "/api/v1/tea-rooms", negative)); !slices.Contains(at, "body.costPerHalfHour") {
		t.Errorf("a negative cost: errors at %v", at)
	}

	if got := status(env.del(t, path)); got != http.StatusNoContent {
		t.Fatalf("delete: %d, want 204", got)
	}
	if got := status(env.del(t, path)); got != http.StatusNotFound {
		t.Errorf("deleting it again: %d, want 404", got)
	}
}

// assertLock checks a room's lock state.
func assertLock(t *testing.T, room map[string]any, wantLocked bool, wantUntil any) {
	t.Helper()
	if room["locked"] != wantLocked || room["lockedUntil"] != wantUntil {
		t.Errorf("lock = %v / %v; want %v / %v", room["locked"], room["lockedUntil"], wantLocked, wantUntil)
	}
}

// The lock half of PATCH: the two fields move independently, an expiry is stored
// canonically whatever RFC 3339 spelling arrives, and unlocking takes the expiry with
// it. An unreadable time is a 422 that applies nothing, flags sent beside it included.
func TestTeaRoomLockPatch(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	room := makeTeaRoom(t, env, "Jasmine Room", "12")

	// The browser sends the millisecond form (Date.toISOString()); it is stored as
	// plain RFC 3339 so the sweeper's compare-and-set has one spelling to match.
	saved := patchTeaRoom(t, env, room, map[string]any{"locked": true, "lockedUntil": "2030-01-02T15:04:05.000Z"})
	assertLock(t, saved, true, "2030-01-02T15:04:05Z")

	// lockedUntil alone re-times a lock already in place.
	saved = patchTeaRoom(t, env, room, map[string]any{"lockedUntil": "2030-02-03T04:05:06Z"})
	assertLock(t, saved, true, "2030-02-03T04:05:06Z")

	// A discount toggle in between leaves the lock exactly as it was.
	saved = patchTeaRoom(t, env, room, map[string]any{"discounted": true})
	assertLock(t, saved, true, "2030-02-03T04:05:06Z")
	if saved["discounted"] != true {
		t.Errorf("the toggle was not applied: %v", saved)
	}

	// Locking again with no time sets the lock afresh: it waits for a person.
	saved = patchTeaRoom(t, env, room, map[string]any{"locked": true})
	assertLock(t, saved, true, nil)
	saved = patchTeaRoom(t, env, room, map[string]any{"lockedUntil": "2030-02-03T04:05:06Z"})
	assertLock(t, saved, true, "2030-02-03T04:05:06Z")

	// Unlocking clears the expiry, so a stale time can't outlive its lock.
	saved = patchTeaRoom(t, env, room, map[string]any{"locked": false})
	assertLock(t, saved, false, nil)

	// Locking with no expiry is the lock that waits for a person.
	saved = patchTeaRoom(t, env, room, map[string]any{"locked": true})
	assertLock(t, saved, true, nil)

	// An unreadable time is refused rather than quietly dropped, and all-or-nothing.
	at := invalid(t, env.patchJSON(t, teaRoomPath(room), map[string]any{
		"discounted": false, "locked": true, "lockedUntil": "tomorrow evening",
	}))
	if !slices.Contains(at, "body.lockedUntil") {
		t.Errorf("an unreadable lockedUntil: errors at %v", at)
	}
	got, err := db.New(env.store.ReadDB()).GetTeaRoom(t.Context(), int64(room["id"].(float64)))
	if err != nil {
		t.Fatal(err)
	}
	if got.LockedUntil != nil {
		t.Errorf("rejected time was still stored: %q", *got.LockedUntil)
	}
	if got.Discounted != 1 {
		t.Error("the discount toggle sent with the rejected time was applied anyway")
	}

	// A save carries a lock too.
	body := teaRoomBody("Jasmine Room", "12")
	body["locked"], body["lockedUntil"] = true, "2031-05-06T07:08:09Z"
	decodeInto(t, env.putJSON(t, teaRoomPath(room), body), http.StatusOK, &saved)
	assertLock(t, saved, true, "2031-05-06T07:08:09Z")
	body["locked"] = false
	decodeInto(t, env.putJSON(t, teaRoomPath(room), body), http.StatusOK, &saved)
	assertLock(t, saved, false, nil)
}

func TestTeaRoomsReorder(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	a := makeTeaRoom(t, env, "A", "1")
	b := makeTeaRoom(t, env, "B", "2")
	c := makeTeaRoom(t, env, "C", "3")
	id := func(r map[string]any) int64 { return int64(r["id"].(float64)) }

	var list struct {
		TeaRooms []map[string]any `json:"teaRooms"`
	}
	decodeInto(t, env.postJSON(t, "/api/v1/tea-rooms/reorder", map[string]any{"orderedIds": []int64{id(c), id(a), id(b)}}), http.StatusOK, &list)
	var names []string
	for _, r := range list.TeaRooms {
		names = append(names, r["name"].(string))
	}
	if fmt.Sprint(names) != "[C A B]" {
		t.Errorf("order: %v", names)
	}
	decodeInto(t, env.get(t, "/api/v1/tea-rooms"), http.StatusOK, &list)
	if list.TeaRooms[0]["name"] != "C" {
		t.Errorf("the order was not kept: %v", list.TeaRooms)
	}
	if at := invalid(t, env.postJSON(t, "/api/v1/tea-rooms/reorder", map[string]any{"orderedIds": []int64{}})); !slices.Contains(at, "body.orderedIds") {
		t.Errorf("an empty order: errors at %v", at)
	}
	// An id that isn't a room is refused at its place, and nothing moves.
	if at := invalid(t, env.postJSON(t, "/api/v1/tea-rooms/reorder", map[string]any{"orderedIds": []int64{id(a), 999, id(b)}})); !slices.Contains(at, "body.orderedIds[1]") {
		t.Errorf("an unknown room: errors at %v", at)
	}
	decodeInto(t, env.get(t, "/api/v1/tea-rooms"), http.StatusOK, &list)
	if list.TeaRooms[0]["name"] != "C" {
		t.Errorf("a refused reorder moved rooms: %v", list.TeaRooms)
	}
}

func TestTeaRoomWebhook(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var hook map[string]any
	decodeInto(t, env.get(t, "/api/v1/tea-rooms/webhook"), http.StatusOK, &hook)
	if hook["webhookUrl"] != "" {
		t.Errorf("a fresh webhook: %v", hook)
	}
	decodeInto(t, env.putJSON(t, "/api/v1/tea-rooms/webhook", map[string]any{"webhookUrl": " " + teaRoomWebhook + " "}), http.StatusOK, &hook)
	if hook["webhookUrl"] != teaRoomWebhook {
		t.Errorf("saved: %v", hook)
	}
	decodeInto(t, env.get(t, "/api/v1/tea-rooms/webhook"), http.StatusOK, &hook)
	if hook["webhookUrl"] != teaRoomWebhook {
		t.Errorf("read back: %v", hook)
	}
	if at := invalid(t, env.putJSON(t, "/api/v1/tea-rooms/webhook", map[string]any{"webhookUrl": "https://evil.example/hook"})); !slices.Contains(at, "body.webhookUrl") {
		t.Errorf("a non-Discord webhook: errors at %v", at)
	}
	decodeInto(t, env.putJSON(t, "/api/v1/tea-rooms/webhook", map[string]any{"webhookUrl": ""}), http.StatusOK, &hook)
	if hook["webhookUrl"] != "" {
		t.Errorf("cleared: %v", hook)
	}
}

// discordStub stands in for Discord, answering every post with one status and
// recording the embed titles it was sent.
type discordStub struct {
	mu     sync.Mutex
	titles []string
}

func (d *discordStub) serve(t *testing.T, status int, header http.Header) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Embeds []struct {
				Title string `json:"title"`
			} `json:"embeds"`
		}
		_ = json.NewDecoder(r.Body).Decode(&p)
		d.mu.Lock()
		for _, e := range p.Embeds {
			d.titles = append(d.titles, e.Title)
		}
		d.mu.Unlock()
		for k, v := range header {
			w.Header()[k] = v
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Posting a room sends its embed to the webhook and answers the room; each way
// Discord can fail is answered with the status that says what to do.
func TestTeaRoomPost(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	room := makeTeaRoom(t, env, "Jasmine Room", "12")
	path := teaRoomPath(room) + "/post"

	// No webhook is the server's missing configuration: 500, and Discord is not called.
	stub := &discordStub{}
	env.srv.SetWebhookPolicy(stub.serve(t, http.StatusOK, nil).URL, false)
	if got := status(env.postJSON(t, path, nil)); got != http.StatusInternalServerError {
		t.Errorf("no webhook: %d, want 500", got)
	}
	if len(stub.titles) != 0 {
		t.Errorf("Discord was called with no webhook set: %v", stub.titles)
	}

	decodeInto(t, env.putJSON(t, "/api/v1/tea-rooms/webhook", map[string]any{"webhookUrl": teaRoomWebhook}), http.StatusOK, nil)
	var posted map[string]any
	decodeInto(t, env.postJSON(t, path, nil), http.StatusOK, &posted)
	if posted["id"] != room["id"] || posted["name"] != "Jasmine Room" {
		t.Errorf("posted: %v", posted)
	}
	if fmt.Sprint(stub.titles) != "[Jasmine Room]" {
		t.Errorf("Discord was sent %v", stub.titles)
	}

	for _, c := range []struct {
		name       string
		status     int
		header     http.Header
		want       int
		wantRetry  string
		wantDetail string
	}{
		{"refused", http.StatusNotFound, nil, http.StatusInternalServerError, "", "Check the webhook"},
		{"rate limited", http.StatusTooManyRequests, http.Header{"Retry-After": {"4"}}, http.StatusTooManyRequests, "4", "4 seconds"},
		{"down", http.StatusBadGateway, nil, http.StatusServiceUnavailable, "", "can't be reached"},
	} {
		env.srv.SetWebhookPolicy((&discordStub{}).serve(t, c.status, c.header).URL, false)
		resp := env.postJSON(t, path, nil)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != c.want || resp.Header.Get("Retry-After") != c.wantRetry || !strings.Contains(string(body), c.wantDetail) {
			t.Errorf("%s: %d Retry-After %q %s; want %d, %q, %q", c.name, resp.StatusCode, resp.Header.Get("Retry-After"), body, c.want, c.wantRetry, c.wantDetail)
		}
	}
}

// getFrom is a GET carrying an Origin and, optionally, an If-None-Match.
func getFrom(t *testing.T, env *testEnv, path, origin, etag string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, env.url(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// The public reads need no sign-in, carry every field, and answer one room bare.
func TestPublicTeaRooms(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	room := makeTeaRoom(t, env, "Jasmine Room", "West 3")
	patchTeaRoom(t, env, room, map[string]any{"locked": true, "lockedUntil": "2030-01-02T15:04:05Z"})
	makeTeaRoom(t, env, "Peony Room", "13")
	env.signOut(t)

	var list struct {
		TeaRooms []map[string]any `json:"teaRooms"`
	}
	decodeInto(t, env.get(t, "/api/v1/tea-rooms/public"), http.StatusOK, &list)
	if len(list.TeaRooms) != 2 {
		t.Fatalf("list: %v", list.TeaRooms)
	}

	var one map[string]any
	decodeInto(t, env.get(t, "/api/v1/tea-rooms/public/West%203"), http.StatusOK, &one)
	for _, field := range []string{
		"id", "name", "subtitle", "roomOwner", "roomNumber", "costPerHalfHour", "hashtags", "description", "seasonal",
		"open", "lockable", "discounted", "locked", "lockedUntil", "image", "color", "sortOrder", "createdAt",
	} {
		if _, ok := one[field]; !ok {
			t.Errorf("the public room has no %s: %v", field, one)
		}
	}
	if one["name"] != "Jasmine Room" || one["locked"] != true || one["lockedUntil"] != "2030-01-02T15:04:05Z" {
		t.Errorf("one: %v", one)
	}
	if _, wrapped := one["teaRoom"]; wrapped {
		t.Error("the room is wrapped; it should be bare")
	}

	if got := status(env.get(t, "/api/v1/tea-rooms/public/nope")); got != http.StatusNotFound {
		t.Errorf("an unknown room: %d, want 404", got)
	}
}

// An origin on the saved allowlist (the seeded one, an exact entry and a subdomain
// of the wildcard) is echoed; any other gets nothing; "*" and credentials never; and
// a 404 carries the same headers as an answer.
func TestPublicTeaRoomsCORS(t *testing.T) {
	env := newTestEnv(t)
	for _, path := range []string{"/api/v1/tea-rooms/public", "/api/v1/tea-rooms/public/nope"} {
		for _, c := range []struct {
			origin string
			echoed bool
		}{
			{"https://senpan.cafe", true},
			{"https://tea.senpan.cafe", true},
			{"https://a.b.senpan.cafe", true},
			{"https://evil.example", false},
			{"http://senpan.cafe", false},
			{"https://senpan.cafe.evil.example", false},
		} {
			resp := getFrom(t, env, path, c.origin, "")
			resp.Body.Close()
			got := resp.Header.Get("Access-Control-Allow-Origin")
			switch {
			case c.echoed && got != c.origin:
				t.Errorf("%s from %s: Allow-Origin %q, want it echoed", path, c.origin, got)
			case !c.echoed && got != "":
				t.Errorf("%s from %s: Allow-Origin %q, want none", path, c.origin, got)
			}
			if got == "*" || resp.Header.Get("Access-Control-Allow-Credentials") != "" {
				t.Errorf("%s from %s: wildcard or credentials: %v", path, c.origin, resp.Header)
			}
			if vary := strings.Join(resp.Header.Values("Vary"), ","); !strings.Contains(vary, "Origin") {
				t.Errorf("%s from %s: Vary = %q, want Origin", path, c.origin, vary)
			}
		}
		resp := getFrom(t, env, path, "", "")
		resp.Body.Close()
		// No origin to allow, but the answer still says it varies by one, so a cache
		// never hands this copy to a site's request.
		if resp.Header.Get("Access-Control-Allow-Origin") != "" || !strings.Contains(strings.Join(resp.Header.Values("Vary"), ","), "Origin") {
			t.Errorf("%s with no Origin: want no Access-Control-Allow-Origin and Vary: Origin, got %v", path, resp.Header)
		}
	}

	// The allowlist is the saved setting: a replaced one allows only what it names.
	env.loginAdmin(t)
	decodeInto(t, saveSettings(t, env, map[string]any{"teaRoomOrigins": []string{"https://only.example"}}), http.StatusOK, nil)
	env.signOut(t)
	for origin, want := range map[string]string{"https://only.example": "https://only.example", "https://senpan.cafe": ""} {
		resp := getFrom(t, env, "/api/v1/tea-rooms/public", origin, "")
		resp.Body.Close()
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != want {
			t.Errorf("after saving the allowlist, %s: Allow-Origin %q, want %q", origin, got, want)
		}
	}
}

func TestPublicTeaRoomsPreflight(t *testing.T) {
	env := newTestEnv(t)
	preflight := func(path, origin string) *http.Response {
		req, _ := http.NewRequest(http.MethodOptions, env.url(path), nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Set("Access-Control-Request-Headers", "if-none-match")
		resp, err := env.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}

	for _, path := range []string{"/api/v1/tea-rooms/public", "/api/v1/tea-rooms/public/12"} {
		resp := preflight(path, "https://tea.senpan.cafe")
		want := map[string]string{
			"Access-Control-Allow-Origin":  "https://tea.senpan.cafe",
			"Access-Control-Allow-Methods": "GET",
			"Access-Control-Allow-Headers": "If-None-Match",
			"Access-Control-Max-Age":       "600",
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("%s: preflight %d, want 204", path, resp.StatusCode)
		}
		for k, v := range want {
			if got := resp.Header.Get(k); got != v {
				t.Errorf("%s: %s = %q, want %q", path, k, got, v)
			}
		}
		if vary := strings.Join(resp.Header.Values("Vary"), ","); !strings.Contains(vary, "Origin") {
			t.Errorf("%s: Vary = %q", path, vary)
		}
	}

	resp := preflight("/api/v1/tea-rooms/public", "https://evil.example")
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "" ||
		resp.Header.Get("Access-Control-Allow-Methods") != "" {
		t.Errorf("a preflight from an unlisted origin: %d %v", resp.StatusCode, resp.Header)
	}
	resp = preflight("/api/v1/tea-rooms", "https://senpan.cafe")
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("a preflight for the staff list: %d %v", resp.StatusCode, resp.Header)
	}
}

// A repeated public read with the ETag it was given answers 304 with no body, and a
// change to a room gives a new tag.
func TestPublicTeaRoomsRevalidate(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	room := makeTeaRoom(t, env, "Jasmine Room", "12")

	for _, path := range []string{"/api/v1/tea-rooms/public", "/api/v1/tea-rooms/public/12"} {
		first := getFrom(t, env, path, "https://senpan.cafe", "")
		first.Body.Close()
		etag := first.Header.Get("ETag")
		if first.StatusCode != http.StatusOK || etag == "" || first.Header.Get("Cache-Control") != "no-cache" {
			t.Fatalf("%s: %d ETag %q Cache-Control %q", path, first.StatusCode, etag, first.Header.Get("Cache-Control"))
		}
		again := getFrom(t, env, path, "https://senpan.cafe", etag)
		body, _ := io.ReadAll(again.Body)
		again.Body.Close()
		if again.StatusCode != http.StatusNotModified || len(body) != 0 {
			t.Errorf("%s: repeat %d with %d body bytes, want 304", path, again.StatusCode, len(body))
		}
		if again.Header.Get("Access-Control-Allow-Origin") != "https://senpan.cafe" {
			t.Errorf("%s: the 304 lost its CORS header: %v", path, again.Header)
		}
	}

	etagOf := func() string {
		resp := getFrom(t, env, "/api/v1/tea-rooms/public/12", "", "")
		resp.Body.Close()
		return resp.Header.Get("ETag")
	}
	before := etagOf()
	patchTeaRoom(t, env, room, map[string]any{"open": false})
	if after := etagOf(); after == before {
		t.Error("the ETag did not change with the room")
	}
}

// A lock whose time has passed lifts on its own and says so by name, to the staff
// with the Tea Rooms page alone, while a lock with no expiry and one still in the
// future are left for the people who set them.
func TestTeaRoomLockExpirySweep(t *testing.T) {
	env := newTestEnv(t)
	other := makeActiveUser(t, env, "gamestaff", "correct horse battery", []string{"bingo-game"})
	env.loginAdmin(t)

	q := db.New(env.store.DB())
	now := time.Now().UTC()
	lockedRoom := func(name, number string, until *string) int64 {
		t.Helper()
		r, err := q.CreateTeaRoom(t.Context(), db.CreateTeaRoomParams{Name: name, RoomNumber: number, Locked: 1, LockedUntil: until})
		if err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	past, soon := now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339)
	due := lockedRoom("Jasmine Room", "12", &past)
	indefinite := lockedRoom("Peony Room", "13", nil)
	future := lockedRoom("Lotus Room", "14", &soon)

	admin := staffStream(t, env, env.client)
	without := staffStream(t, env, other)

	server.ExpireDueTeaRoomLocksForTest(env.srv, t.Context())

	read := db.New(env.store.ReadDB())
	if got, _ := read.GetTeaRoom(t.Context(), due); got.Locked != 0 || got.LockedUntil != nil {
		t.Errorf("expired room: locked=%v until=%v; want unlocked with no expiry", got.Locked, got.LockedUntil)
	}
	if got, _ := read.GetTeaRoom(t.Context(), indefinite); got.Locked != 1 {
		t.Error("a lock with no expiry was swept; it waits on a person, not the clock")
	}
	if got, _ := read.GetTeaRoom(t.Context(), future); got.Locked != 1 {
		t.Error("a lock still in the future was swept")
	}

	e := admin.expect("tea_room_unlocked", 3*time.Second)
	if e.str("name") != "Jasmine Room" || e.str("roomNumber") != "12" || e.Payload["id"] != float64(due) {
		t.Errorf("tea_room_unlocked: %+v", e)
	}
	if e := admin.expect("resource_changed", time.Second); e.str("resource") != "tea-rooms" {
		t.Errorf("resource_changed: %+v; want tea-rooms", e)
	}
	without.expectNone("tea_room_unlocked", 300*time.Millisecond)

	// A second pass has nothing left to do, so nothing is announced twice.
	server.ExpireDueTeaRoomLocksForTest(env.srv, t.Context())
	admin.expectNone("tea_room_unlocked", 500*time.Millisecond)
}
