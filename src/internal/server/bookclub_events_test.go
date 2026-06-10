package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// expectedUnix resolves a wall-clock string in an IANA zone to UTC seconds,
// mirroring the server so tests don't hard-code offsets (DST-safe).
func expectedUnix(t *testing.T, local, tz string) int64 {
	t.Helper()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		t.Fatalf("load location %q: %v", tz, err)
	}
	tm, err := time.ParseInLocation("2006-01-02T15:04", local, loc)
	if err != nil {
		t.Fatalf("parse %q: %v", local, err)
	}
	return tm.Unix()
}

func createEvent(t *testing.T, e *testEnv, ev map[string]any) map[string]any {
	t.Helper()
	resp := e.postJSON(t, "/api/bookclub/events", map[string]any{
		"action": "create", "club_slug": "yaoi", "event": ev,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create event status = %d; want 201", resp.StatusCode)
	}
	body := decodeBody(t, resp)
	out, _ := body["event"].(map[string]any)
	if out == nil {
		t.Fatalf("create event returned no event: %+v", body)
	}
	return out
}

func TestBookClubEventResolvesTimesAndCRUD(t *testing.T) {
	e := newTestEnv(t)

	// Anonymous is rejected.
	resp := e.get(t, "/api/bookclub/events?club=yaoi")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon events status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()

	e.loginAdmin(t)

	const tz = "America/New_York"
	start := "2026-07-15T19:00"
	postAt := "2026-07-10T09:00"
	out := createEvent(t, e, map[string]any{
		"title":         "July 2026 Meeting",
		"start_local":   start,
		"timezone":      tz,
		"length_hours":  2,
		"location":      "Voice Channel #1",
		"image":         "https://example.com/cover.png",
		"post_at_local": postAt,
	})

	// Server resolved the wall-clock + tz into the correct absolute instants.
	if got := int64(out["start_at_unix"].(float64)); got != expectedUnix(t, start, tz) {
		t.Errorf("start_at_unix = %d; want %d", got, expectedUnix(t, start, tz))
	}
	if got := int64(out["post_at_unix"].(float64)); got != expectedUnix(t, postAt, tz) {
		t.Errorf("post_at_unix = %d; want %d", got, expectedUnix(t, postAt, tz))
	}
	if out["posted"].(bool) {
		t.Errorf("new event should not be posted")
	}
	id := int64(out["id"].(float64))

	// List returns it.
	resp = e.get(t, "/api/bookclub/events?club=yaoi")
	body := decodeBody(t, resp)
	if events, _ := body["events"].([]any); len(events) != 1 {
		t.Fatalf("events = %d; want 1", len(events))
	}

	// Update the length.
	resp = e.postJSON(t, "/api/bookclub/events", map[string]any{
		"action": "update", "id": id,
		"event": map[string]any{
			"title": "July 2026 Meeting", "start_local": start, "timezone": tz,
			"length_hours": 4, "post_at_local": postAt,
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d; want 200", resp.StatusCode)
	}
	ub := decodeBody(t, resp)
	uev, _ := ub["event"].(map[string]any)
	if int(uev["length_hours"].(float64)) != 4 {
		t.Errorf("length_hours = %v; want 4", uev["length_hours"])
	}

	// Delete it.
	resp = e.postJSON(t, "/api/bookclub/events", map[string]any{"action": "delete", "id": id})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	resp = e.get(t, "/api/bookclub/events?club=yaoi")
	body = decodeBody(t, resp)
	if events, _ := body["events"].([]any); len(events) != 0 {
		t.Fatalf("events after delete = %d; want 0", len(events))
	}
}

func TestBookClubEventValidation(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	cases := []struct {
		name  string
		event map[string]any
	}{
		{"missing title", map[string]any{"title": "", "timezone": "UTC", "length_hours": 1, "start_local": "2026-07-15T19:00", "post_at_local": "2026-07-10T09:00"}},
		{"bad length", map[string]any{"title": "X", "timezone": "UTC", "length_hours": 9, "start_local": "2026-07-15T19:00", "post_at_local": "2026-07-10T09:00"}},
		{"bad timezone", map[string]any{"title": "X", "timezone": "Not/AZone", "length_hours": 1, "start_local": "2026-07-15T19:00", "post_at_local": "2026-07-10T09:00"}},
		{"bad start", map[string]any{"title": "X", "timezone": "UTC", "length_hours": 1, "start_local": "nonsense", "post_at_local": "2026-07-10T09:00"}},
	}
	for _, c := range cases {
		resp := e.postJSON(t, "/api/bookclub/events", map[string]any{
			"action": "create", "club_slug": "yaoi", "event": c.event,
		})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d; want 400", c.name, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestBookClubEventSchedulerPostsDueEvent(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	var mu sync.Mutex
	var received []map[string]any
	discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(data, &payload)
		mu.Lock()
		received = append(received, payload)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer discord.Close()
	if err := e.store.SetSetting("discord_events_webhook_url_yaoi", discord.URL); err != nil {
		t.Fatal(err)
	}

	// An event whose post time is already in the past → due now.
	out := createEvent(t, e, map[string]any{
		"title": "Past Watch Party", "start_local": "2026-07-15T19:00", "timezone": "America/New_York",
		"length_hours": 2, "image": "https://example.com/party.png", "post_at_local": "2020-01-01T00:00",
	})
	id := int64(out["id"].(float64))

	e.srv.PostDueEventsForTest()

	mu.Lock()
	if len(received) != 1 {
		mu.Unlock()
		t.Fatalf("webhook received %d posts; want 1", len(received))
	}
	embeds, _ := received[0]["embeds"].([]any)
	mu.Unlock()
	if len(embeds) != 1 {
		t.Fatalf("embeds = %d; want 1", len(embeds))
	}
	embed, _ := embeds[0].(map[string]any)
	// Full-width image present.
	img, _ := embed["image"].(map[string]any)
	if img == nil || img["url"] != "https://example.com/party.png" {
		t.Errorf("embed image = %v; want full-width party.png", embed["image"])
	}
	// Discord timestamps used for the date/time; the main date uses long format
	// (:F) so the weekday shows.
	if desc, _ := embed["description"].(string); !strings.Contains(desc, "<t:") || !strings.Contains(desc, ":F>") {
		t.Errorf("embed description missing long-format Discord timestamp: %q", desc)
	}
	// Footer clarifies the local-timezone rendering.
	footer, _ := embed["footer"].(map[string]any)
	if footer == nil || footer["text"] != "These dates are in your local time zone." {
		t.Errorf("embed footer = %v; want local-timezone note", embed["footer"])
	}

	// Marked posted; a second sweep must not re-post.
	ev, err := e.store.GetBookClubEvent(id)
	if err != nil || ev == nil || !ev.Posted {
		t.Fatalf("event not marked posted: %+v (err %v)", ev, err)
	}
	e.srv.PostDueEventsForTest()
	mu.Lock()
	n := len(received)
	mu.Unlock()
	if n != 1 {
		t.Errorf("re-posted a posted event: total posts = %d; want 1", n)
	}
}

func TestBookClubEventPostNow(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	var got map[string]any
	discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer discord.Close()

	out := createEvent(t, e, map[string]any{
		"title": "Manual Post", "start_local": "2026-08-01T18:00", "timezone": "UTC",
		"length_hours": 1, "post_at_local": "2026-07-31T18:00",
	})
	id := int64(out["id"].(float64))

	// Without a webhook, post_now should fail with 400.
	resp := e.postJSON(t, "/api/bookclub/events", map[string]any{"action": "post_now", "id": id})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("post_now without webhook = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	if err := e.store.SetSetting("discord_events_webhook_url_yaoi", discord.URL); err != nil {
		t.Fatal(err)
	}
	resp = e.postJSON(t, "/api/bookclub/events", map[string]any{"action": "post_now", "id": id})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post_now = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	if embeds, _ := got["embeds"].([]any); len(embeds) != 1 {
		t.Fatalf("post_now embeds = %d; want 1", len(embeds))
	}
}

func TestEventsWebhookRedactedForPublic(t *testing.T) {
	e := newTestEnv(t)
	if err := e.store.SetSetting("discord_events_webhook_url_yaoi", "https://discord.test/events"); err != nil {
		t.Fatal(err)
	}

	resp := e.get(t, "/api/settings")
	body := decodeBody(t, resp)
	settings, _ := body["settings"].(map[string]any)
	if v, _ := settings["discord_events_webhook_url_yaoi"].(string); v != "" {
		t.Fatalf("public events webhook = %q; want blank", v)
	}

	e.loginAdmin(t)
	resp = e.get(t, "/api/settings")
	body = decodeBody(t, resp)
	settings, _ = body["settings"].(map[string]any)
	if v, _ := settings["discord_events_webhook_url_yaoi"].(string); v != "https://discord.test/events" {
		t.Fatalf("admin events webhook = %q; want the real value", v)
	}
}

func TestBookClubEventImagesEmpty(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	resp := e.get(t, "/api/bookclub/events/images")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("images status = %d; want 200", resp.StatusCode)
	}
	body := decodeBody(t, resp)
	imgs, ok := body["images"].([]any)
	if !ok || len(imgs) != 0 {
		t.Fatalf("images = %v; want empty array", body["images"])
	}
}
