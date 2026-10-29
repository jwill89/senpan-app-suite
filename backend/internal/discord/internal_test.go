package discord

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRouteKeyFoldsMessageIDs(t *testing.T) {
	for _, c := range []struct{ method, path, want string }{
		{"POST", "/channels/200000000000000003/messages", "POST /channels/200000000000000003/messages"},
		{"PATCH", "/channels/200000000000000003/messages/400000000000000001", "PATCH /channels/200000000000000003/messages/{id}"},
		// Every message in a channel shares one bucket.
		{"PATCH", "/channels/200000000000000003/messages/400000000000000002", "PATCH /channels/200000000000000003/messages/{id}"},
		{"GET", "/guilds/999011226568888410/channels", "GET /guilds/999011226568888410/channels"},
		{"GET", "/users/@me?with_counts=true", "GET /users/@me"},
	} {
		if got := routeKey(c.method, c.path); got != c.want {
			t.Errorf("routeKey(%s %s) = %q, want %q", c.method, c.path, got, c.want)
		}
	}
}

func TestRefusalFlattensDiscordsErrors(t *testing.T) {
	body := `{"code":50035,"message":"Invalid Form Body","errors":{
		"content":{"_errors":[{"code":"X","message":"Must be 2000 or fewer in length."}]},
		"embeds":{"0":{"fields":{"3":{"value":{"_errors":[{"code":"Y","message":"Must be 1024 or fewer in length."}]}}}}}}}`
	want := "content: Must be 2000 or fewer in length. embeds.0.fields.3.value: Must be 1024 or fewer in length."
	if got := refusal([]byte(body)); got != want {
		t.Fatalf("refusal = %q\nwant      %q", got, want)
	}
	if got := refusal([]byte(`{"message":"Cannot send an empty message","code":50006}`)); got != "Cannot send an empty message" {
		t.Fatalf("message-only refusal = %q", got)
	}
	if got := refusal([]byte("not json")); got != "" {
		t.Fatalf("unparseable refusal = %q", got)
	}
}

func TestPresencePayload(t *testing.T) {
	for _, c := range []struct {
		name string
		in   Presence
		want string
	}{
		{"nothing set", Presence{}, `{"activities":[],"afk":false,"since":null,"status":"online"}`},
		{"a status alone", Presence{Status: "dnd"}, `{"activities":[],"afk":false,"since":null,"status":"dnd"}`},
		{"custom words", Presence{Activity: "custom", Text: " Doors at nine "}, `{"activities":[{"name":"Custom Status","state":"Doors at nine","type":4}],"afk":false,"since":null,"status":"online"}`},
		{"one of Discord's verbs", Presence{Activity: "listening", Text: "the kettle"}, `{"activities":[{"name":"the kettle","type":2}],"afk":false,"since":null,"status":"online"}`},
		{"an activity with no words", Presence{Activity: "playing", Text: "  "}, `{"activities":[],"afk":false,"since":null,"status":"online"}`},
		{"words with no activity", Presence{Activity: "none", Text: "left over"}, `{"activities":[],"afk":false,"since":null,"status":"online"}`},
	} {
		got, err := json.Marshal(c.in.payload())
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != c.want {
			t.Errorf("%s: %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestChannelAndRoleEventsDropTheDirectoryCache(t *testing.T) {
	s := NewService("", "")
	for _, event := range []string{"CHANNEL_CREATE", "CHANNEL_UPDATE", "CHANNEL_DELETE",
		"GUILD_ROLE_CREATE", "GUILD_ROLE_UPDATE", "GUILD_ROLE_DELETE", "GUILD_UPDATE"} {
		s.cache, s.cachedAt = &Directory{InGuild: true}, time.Now()
		s.handle(event, nil)
		if s.cache != nil {
			t.Errorf("%s left the cache in place", event)
		}
	}
	s.cache, s.cachedAt = &Directory{InGuild: true}, time.Now()
	s.handle("MESSAGE_CREATE", nil)
	if s.cache == nil {
		t.Error("an unrelated event dropped the cache")
	}
}

func TestClipCountsRunes(t *testing.T) {
	if got := clip("tea ☕ time", 5); got != "tea ☕" {
		t.Fatalf("clip = %q", got)
	}
	if got := clip("short", 10); got != "short" {
		t.Fatalf("clip = %q", got)
	}
}
