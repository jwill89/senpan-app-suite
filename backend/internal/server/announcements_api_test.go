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
)

const announcementWebhook = "https://discord.com/api/webhooks/1/announcement-token"

// announcementBody is a full announcement save for a type, with the given fields set.
func announcementBody(typeID int64, set map[string]any) map[string]any {
	body := map[string]any{
		"typeId": typeID, "title": "Bingo Night", "details": "Tonight!", "image": "", "thumbnail": "",
		"color": "", "location": "", "startLocal": "", "endLocal": "", "startFormat": "", "endFormat": "",
		"dynamicDates": false, "scheduleKind": "", "timezone": "", "onceLocal": "", "scheduleMinutes": 0,
		"scheduleWeekdays": "", "scheduleWeekOfMonth": 0, "buttons": []any{}, "mention": "",
	}
	for k, v := range set {
		body[k] = v
	}
	return body
}

// makeAnnouncementType saves a type with a Discord webhook and returns its id.
func (e *testEnv) makeAnnouncementType(t *testing.T, name string) int64 {
	t.Helper()
	var made map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/announcement-types", map[string]any{"name": name, "webhookUrl": announcementWebhook}), http.StatusCreated, &made)
	return int64(made["id"].(float64))
}

// makeAnnouncement saves an announcement for a type and returns it.
func (e *testEnv) makeAnnouncement(t *testing.T, typeID int64, set map[string]any) map[string]any {
	t.Helper()
	var made map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/announcements", announcementBody(typeID, set)), http.StatusCreated, &made)
	return made
}

func announcementPath(a map[string]any) string {
	return fmt.Sprintf("/api/v1/announcements/%d", int64(a["id"].(float64)))
}

// The Announcements page's permission guards every operation.
func TestAnnouncementsGuard(t *testing.T) {
	env := newTestEnv(t)
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/announcements"}, {http.MethodPost, "/api/v1/announcements"},
		{http.MethodPost, "/api/v1/announcements/1/send"}, {http.MethodGet, "/api/v1/announcement-types"},
		{http.MethodDelete, "/api/v1/announcement-roles/1"},
	} {
		if got := status(env.reqJSON(t, call.method, call.path, map[string]any{})); got != http.StatusUnauthorized {
			t.Errorf("%s %s signed out: %d, want 401", call.method, call.path, got)
		}
	}

	addAccount(t, env, "presetsonly", "correct horse battery", []string{"bingo-presets"})
	env.signIn(t, "presetsonly", "correct horse battery")
	for _, path := range []string{"/api/v1/announcements", "/api/v1/announcement-types", "/api/v1/announcement-roles"} {
		if got := status(env.get(t, path)); got != http.StatusForbidden {
			t.Errorf("GET %s without the page: %d, want 403", path, got)
		}
	}

	addAccount(t, env, "announcer", "correct horse battery", []string{"teahouse-announcements"})
	env.signIn(t, "announcer", "correct horse battery")
	env.makeAnnouncementType(t, "Mine")
}

// A type is made, listed, replaced and deleted; one announcements use is refused (409)
// and a webhook that isn't Discord's is a 422.
func TestAnnouncementTypes(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/announcement-types", map[string]any{"name": "  Events  ", "webhookUrl": announcementWebhook}), http.StatusCreated, &made)
	if made["name"] != "Events" || made["webhookUrl"] != announcementWebhook || made["createdAt"] == nil {
		t.Errorf("made: %v", made)
	}
	id := int64(made["id"].(float64))
	path := fmt.Sprintf("/api/v1/announcement-types/%d", id)

	var list struct{ Types []map[string]any }
	decodeInto(t, env.get(t, "/api/v1/announcement-types"), http.StatusOK, &list)
	if len(list.Types) != 1 || list.Types[0]["name"] != "Events" {
		t.Errorf("list: %v", list.Types)
	}

	var saved map[string]any
	decodeInto(t, env.putJSON(t, path, map[string]any{"name": "Raids", "webhookUrl": ""}), http.StatusOK, &saved)
	if saved["name"] != "Raids" || saved["webhookUrl"] != "" {
		t.Errorf("saved: %v", saved)
	}
	if at := invalid(t, env.putJSON(t, path, map[string]any{"name": "Raids", "webhookUrl": "https://evil.example/api/webhooks/1"})); !slices.Contains(at, "body.webhookUrl") {
		t.Errorf("a non-Discord webhook: errors at %v", at)
	}
	if at := invalid(t, env.postJSON(t, "/api/v1/announcement-types", map[string]any{"name": "   ", "webhookUrl": ""})); !slices.Contains(at, "body.name") {
		t.Errorf("a blank name: errors at %v", at)
	}
	if got := status(env.putJSON(t, "/api/v1/announcement-types/999999", map[string]any{"name": "X", "webhookUrl": ""})); got != http.StatusNotFound {
		t.Errorf("an unknown type: %d, want 404", got)
	}

	env.makeAnnouncement(t, id, nil)
	title, code, detail := problem(t, env.del(t, path))
	if code != http.StatusConflict || !strings.Contains(detail, "1 announcement") {
		t.Errorf("a type in use: %d %q %q; want 409 naming the count", code, title, detail)
	}

	spare := env.makeAnnouncementType(t, "Spare")
	if got := status(env.del(t, fmt.Sprintf("/api/v1/announcement-types/%d", spare))); got != http.StatusNoContent {
		t.Errorf("delete an unused type: %d, want 204", got)
	}
	if got := status(env.del(t, fmt.Sprintf("/api/v1/announcement-types/%d", spare))); got != http.StatusNotFound {
		t.Errorf("delete it again: %d, want 404", got)
	}
}

// A role is made, listed, replaced and deleted; its Discord id is digits, and one an
// announcement tags can't be deleted.
func TestAnnouncementRoles(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/announcement-roles", map[string]any{"name": " Crew ", "roleId": "123456789012345678"}), http.StatusCreated, &made)
	if made["name"] != "Crew" || made["roleId"] != "123456789012345678" {
		t.Errorf("made: %v", made)
	}
	id := int64(made["id"].(float64))
	path := fmt.Sprintf("/api/v1/announcement-roles/%d", id)

	var list struct{ Roles []map[string]any }
	decodeInto(t, env.get(t, "/api/v1/announcement-roles"), http.StatusOK, &list)
	if len(list.Roles) != 1 {
		t.Errorf("list: %v", list.Roles)
	}
	var saved map[string]any
	decodeInto(t, env.putJSON(t, path, map[string]any{"name": "Staff", "roleId": "42"}), http.StatusOK, &saved)
	if saved["name"] != "Staff" || saved["roleId"] != "42" {
		t.Errorf("saved: %v", saved)
	}
	for _, bad := range []string{"12a", "-5", "1 2"} {
		if at := invalid(t, env.postJSON(t, "/api/v1/announcement-roles", map[string]any{"name": "X", "roleId": bad})); !slices.Contains(at, "body.roleId") {
			t.Errorf("role id %q: errors at %v", bad, at)
		}
	}
	if got := status(env.putJSON(t, "/api/v1/announcement-roles/999999", map[string]any{"name": "X", "roleId": "1"})); got != http.StatusNotFound {
		t.Errorf("an unknown role: %d, want 404", got)
	}

	typeID := env.makeAnnouncementType(t, "T")
	env.makeAnnouncement(t, typeID, map[string]any{"mention": fmt.Sprintf("role:%d", id)})
	if _, code, detail := problem(t, env.del(t, path)); code != http.StatusConflict || !strings.Contains(detail, "1 announcement") {
		t.Errorf("a tagged role: %d %q; want 409 naming the count", code, detail)
	}

	spare := fmt.Sprintf("/api/v1/announcement-roles/%d", int64(decodeBody(t, env.postJSON(t, "/api/v1/announcement-roles", map[string]any{"name": "Spare", "roleId": "7"}))["id"].(float64)))
	if got := status(env.del(t, spare)); got != http.StatusNoContent {
		t.Errorf("delete an unused role: %d, want 204", got)
	}
	if got := status(env.del(t, spare)); got != http.StatusNotFound {
		t.Errorf("delete it again: %d, want 404", got)
	}
}

// An announcement is made (201, in full), listed with its type's name, replaced and
// deleted, and each rule the schema can't hold is a 422 at its own field.
func TestAnnouncementLifecycle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	typeID := env.makeAnnouncementType(t, "Events")

	made := env.makeAnnouncement(t, typeID, map[string]any{
		"title": "  Tea Time  ", "details": " **Come** ", "color": "#1abc9c", "timezone": "America/Chicago",
		"startLocal": "2026-06-13T19:00", "endLocal": "2026-06-13T21:00",
		"buttons": []any{
			map[string]any{"label": "Sign up", "emoji": "", "url": "https://example.com/a"},
			map[string]any{"label": "Bad", "emoji": "", "url": "ftp://nope"},
		},
	})
	if made["title"] != "Tea Time" || made["details"] != "**Come**" || made["typeName"] != "Events" {
		t.Errorf("made: %v", made)
	}
	if made["startAt"] != "2026-06-14T00:00:00Z" || made["endAt"] != "2026-06-14T02:00:00Z" {
		t.Errorf("the window = %v to %v; want it resolved to UTC", made["startAt"], made["endAt"])
	}
	if made["startFormat"] != "F" || made["endFormat"] != "t" {
		t.Errorf("formats = %v / %v; want the defaults", made["startFormat"], made["endFormat"])
	}
	if made["active"] != false || made["nextPostAt"] != nil || made["lastPostedAt"] != nil || made["skipCount"] != float64(0) {
		t.Errorf("a manual announcement: %v", made)
	}
	if buttons, _ := made["buttons"].([]any); len(buttons) != 1 {
		t.Errorf("buttons = %v; want the invalid one dropped", made["buttons"])
	}
	path := announcementPath(made)

	var list struct{ Announcements []map[string]any }
	decodeInto(t, env.get(t, "/api/v1/announcements"), http.StatusOK, &list)
	if len(list.Announcements) != 1 || list.Announcements[0]["typeName"] != "Events" {
		t.Errorf("list: %v", list.Announcements)
	}

	var saved map[string]any
	decodeInto(t, env.putJSON(t, path, announcementBody(typeID, map[string]any{"title": "Renamed"})), http.StatusOK, &saved)
	if saved["title"] != "Renamed" || saved["startAt"] != nil {
		t.Errorf("saved: %v", saved)
	}

	if got := status(env.putJSON(t, "/api/v1/announcements/999999", announcementBody(typeID, nil))); got != http.StatusNotFound {
		t.Errorf("an unknown announcement: %d, want 404", got)
	}

	bad := []struct {
		name string
		set  map[string]any
		at   string
	}{
		{"blank title", map[string]any{"title": "   "}, "body.title"},
		{"blank details", map[string]any{"details": "  "}, "body.details"},
		{"unknown type", map[string]any{"typeId": 999999}, "body.typeId"},
		{"bad mention", map[string]any{"mention": "here"}, "body.mention"},
		{"role gone", map[string]any{"mention": "role:999999"}, "body.mention"},
		{"no timezone", map[string]any{"startLocal": "2026-06-13T19:00"}, "body.timezone"},
		{"bad timezone", map[string]any{"startLocal": "2026-06-13T19:00", "timezone": "Mars/Base"}, "body.timezone"},
		{"bad start", map[string]any{"startLocal": "soon", "timezone": "UTC"}, "body.startLocal"},
		{"unknown schedule", map[string]any{"scheduleKind": "hourly", "timezone": "UTC"}, "body.scheduleKind"},
		{"once without a date", map[string]any{"scheduleKind": "once", "timezone": "UTC"}, "body.onceLocal"},
		{"weekly without a day", map[string]any{"scheduleKind": "weekly", "timezone": "UTC", "scheduleMinutes": 60}, "body.scheduleWeekdays"},
		{"minutes past a day", map[string]any{"scheduleKind": "daily", "timezone": "UTC", "scheduleMinutes": 1440}, "body.scheduleMinutes"},
		{"monthly without a week", map[string]any{"scheduleKind": "monthly", "timezone": "UTC", "scheduleWeekdays": "5"}, "body.scheduleWeekOfMonth"},
	}
	for _, c := range bad {
		if at := invalid(t, env.postJSON(t, "/api/v1/announcements", announcementBody(typeID, c.set))); !slices.Contains(at, c.at) {
			t.Errorf("%s: errors at %v; want %s", c.name, at, c.at)
		}
	}

	if got := status(env.del(t, path)); got != http.StatusNoContent {
		t.Errorf("delete: %d, want 204", got)
	}
	if got := status(env.del(t, path)); got != http.StatusNotFound {
		t.Errorf("delete again: %d, want 404", got)
	}
}

// A recurring announcement is armed on its next occurrence and stays live.
func TestAnnouncementRecurringIsArmed(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	a := env.makeAnnouncement(t, env.makeAnnouncementType(t, "T"), map[string]any{
		"scheduleKind": "weekly", "timezone": "America/Chicago", "scheduleWeekdays": "5", "scheduleMinutes": 1320,
	})
	if a["active"] != true || a["nextPostAt"] == nil || a["scheduleKind"] != "weekly" {
		t.Errorf("a weekly announcement: %v", a)
	}
}

// Reordering answers the list in the new order; an id that isn't one is a 422.
func TestAnnouncementReorder(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	typeID := env.makeAnnouncementType(t, "T")
	var ids []float64
	for _, title := range []string{"alpha", "bravo", "charlie"} {
		ids = append(ids, env.makeAnnouncement(t, typeID, map[string]any{"title": title})["id"].(float64))
	}
	titles := func(list struct{ Announcements []map[string]any }) []string {
		var out []string
		for _, a := range list.Announcements {
			out = append(out, a["title"].(string))
		}
		return out
	}

	var list struct{ Announcements []map[string]any }
	decodeInto(t, env.get(t, "/api/v1/announcements"), http.StatusOK, &list)
	if got := titles(list); fmt.Sprint(got) != "[charlie bravo alpha]" {
		t.Fatalf("the default order = %v; want newest first", got)
	}

	decodeInto(t, env.postJSON(t, "/api/v1/announcements/reorder", map[string]any{"orderedIds": []float64{ids[0], ids[2], ids[1]}}), http.StatusOK, &list)
	if got := titles(list); fmt.Sprint(got) != "[alpha charlie bravo]" {
		t.Errorf("after reordering = %v", got)
	}

	if at := invalid(t, env.postJSON(t, "/api/v1/announcements/reorder", map[string]any{"orderedIds": []float64{ids[0], 999999}})); !slices.Contains(at, "body.orderedIds[1]") {
		t.Errorf("an unknown id: errors at %v", at)
	}
	decodeInto(t, env.get(t, "/api/v1/announcements"), http.StatusOK, &list)
	if got := titles(list); fmt.Sprint(got) != "[alpha charlie bravo]" {
		t.Errorf("a refused reorder changed the order: %v", got)
	}
	if got := status(env.postJSON(t, "/api/v1/announcements/reorder", map[string]any{"orderedIds": []float64{}})); got != http.StatusUnprocessableEntity {
		t.Errorf("an empty order: %d, want 422", got)
	}
}

// recordingDiscord stands in for Discord: every post goes to it whatever the webhook
// says, and it answers with one status and records what it was sent.
type recordingDiscord struct {
	mu    sync.Mutex
	posts []map[string]any
}

func (r *recordingDiscord) use(t *testing.T, env *testEnv, status int, header http.Header) {
	t.Helper()
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		var p map[string]any
		_ = json.Unmarshal(raw, &p)
		r.mu.Lock()
		r.posts = append(r.posts, p)
		r.mu.Unlock()
		for k, v := range header {
			w.Header()[k] = v
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(stub.Close)
	env.srv.SetWebhookPolicy(stub.URL, false)
}

func (r *recordingDiscord) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.posts)
}

// Sending posts the embed to the type's webhook, tagging who the announcement names,
// stamps it as posted, and leaves its schedule alone.
func TestAnnouncementSend(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	var discord recordingDiscord
	discord.use(t, env, http.StatusNoContent, nil)

	typeID := env.makeAnnouncementType(t, "Events")
	var role map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/announcement-roles", map[string]any{"name": "Crew", "roleId": "123456789012345678"}), http.StatusCreated, &role)
	a := env.makeAnnouncement(t, typeID, map[string]any{
		"title": "Tea Time", "mention": fmt.Sprintf("role:%d", int64(role["id"].(float64))),
		"scheduleKind": "weekly", "timezone": "UTC", "scheduleWeekdays": "6", "scheduleMinutes": 600,
		"buttons": []any{map[string]any{"label": "Open", "emoji": "", "url": "https://example.com"}},
	})

	var sent map[string]any
	decodeInto(t, env.postJSON(t, announcementPath(a)+"/send", nil), http.StatusOK, &sent)
	if sent["lastPostedAt"] == nil {
		t.Errorf("sent: %v; want it stamped as posted", sent)
	}
	if sent["nextPostAt"] != a["nextPostAt"] || sent["active"] != true {
		t.Errorf("sending moved the schedule: next %v -> %v", a["nextPostAt"], sent["nextPostAt"])
	}
	if discord.count() != 1 {
		t.Fatalf("Discord got %d posts, want 1", discord.count())
	}
	post := discord.posts[0]
	if post["content"] != "<@&123456789012345678>" {
		t.Errorf("content = %v; want the role tag", post["content"])
	}
	embeds, _ := post["embeds"].([]any)
	if len(embeds) != 1 || embeds[0].(map[string]any)["title"] != "Tea Time" {
		t.Errorf("embeds = %v", post["embeds"])
	}
	if comps, _ := post["components"].([]any); len(comps) != 1 {
		t.Errorf("components = %v; want the button row", post["components"])
	}

	if got := status(env.postJSON(t, "/api/v1/announcements/999999/send", nil)); got != http.StatusNotFound {
		t.Errorf("an unknown announcement: %d, want 404", got)
	}
}

// A type with no webhook is the server's missing configuration: 500, and Discord is
// never called.
func TestAnnouncementSendWithoutAWebhook(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	var discord recordingDiscord
	discord.use(t, env, http.StatusNoContent, nil)

	var typ map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/announcement-types", map[string]any{"name": "Silent", "webhookUrl": ""}), http.StatusCreated, &typ)
	a := env.makeAnnouncement(t, int64(typ["id"].(float64)), nil)

	_, code, detail := problem(t, env.postJSON(t, announcementPath(a)+"/send", nil))
	if code != http.StatusInternalServerError || !strings.Contains(detail, "No Discord webhook is set") {
		t.Errorf("no webhook: %d %q", code, detail)
	}
	if discord.count() != 0 {
		t.Error("Discord was called with no webhook set")
	}
}

// Each way Discord can fail a manual send is answered with the status that says what
// to do, and the announcement is not stamped as posted.
func TestAnnouncementSendDiscordFailures(t *testing.T) {
	cases := []struct {
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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.loginAdmin(t)
			var discord recordingDiscord
			discord.use(t, env, c.status, c.header)
			a := env.makeAnnouncement(t, env.makeAnnouncementType(t, "T"), nil)

			resp := env.postJSON(t, announcementPath(a)+"/send", nil)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != c.want || resp.Header.Get("Retry-After") != c.wantRetry || !strings.Contains(string(body), c.wantDetail) {
				t.Errorf("%d Retry-After %q %s; want %d, %q, %q", resp.StatusCode, resp.Header.Get("Retry-After"), body, c.want, c.wantRetry, c.wantDetail)
			}
			var list struct{ Announcements []map[string]any }
			decodeInto(t, env.get(t, "/api/v1/announcements"), http.StatusOK, &list)
			if list.Announcements[0]["lastPostedAt"] != nil {
				t.Error("a failed send was stamped as posted")
			}
		})
	}
}
