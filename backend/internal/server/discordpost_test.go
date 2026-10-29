package server_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"app-suite/internal/discord/discordtest"
)

// The fake server's channels (see discordtest.New).
const (
	chAnnouncements = "200000000000000002"
	chTeaRooms      = "200000000000000003"
	chBookClub      = "200000000000000004"
	chTest          = "200000000000000005"
	roleStaff       = "300000000000000001"
)

// discordReady logs in as admin and connects the bot to a fresh fake Discord.
func (e *testEnv) discordReady(t *testing.T) *discordtest.Server {
	t.Helper()
	e.loginAdmin(t)
	f := e.withDiscord(t)
	e.setDiscordToken(t, discordtest.Token)
	return f
}

// setDestination saves where a system posts.
func (e *testEnv) setDestination(t *testing.T, key, channel, mention, mode string) {
	t.Helper()
	resp := e.putJSON(t, "/api/discord/destinations/"+key,
		map[string]string{"channel_id": channel, "mention": mention, "repost_mode": mode})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("set destination %s = %d: %s", key, resp.StatusCode, b)
	}
	resp.Body.Close()
}

// postOK posts (or takes down) and returns the DiscordPostResult body.
func (e *testEnv) discordAction(t *testing.T, method, path string) map[string]any {
	t.Helper()
	var resp *http.Response
	if method == http.MethodDelete {
		resp = e.del(t, path)
	} else {
		resp = e.postJSON(t, path, map[string]any{})
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("%s %s = %d: %s", method, path, resp.StatusCode, b)
	}
	return decodeBody(t, resp)
}

// roomPost reads a tea room's discord_post from the list (nil when it has none).
func (e *testEnv) roomPost(t *testing.T, id int64) map[string]any {
	t.Helper()
	rooms, _ := decodeBody(t, e.get(t, "/api/tea-rooms"))["tea_rooms"].([]any)
	for _, r := range rooms {
		room := r.(map[string]any)
		if int64(room["id"].(float64)) == id {
			p, _ := room["discord_post"].(map[string]any)
			return p
		}
	}
	t.Fatalf("tea room %d not in the list", id)
	return nil
}

func TestDiscordDestinations_ValidationAndGuards(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.get(t, "/api/discord/destinations/tea-rooms")
	d, _ := decodeBody(t, resp)["destination"].(map[string]any)
	if d["channel_id"] != "" || d["repost_mode"] != "edit" {
		t.Errorf("an unsaved destination = %v; want no channel, tea rooms' default mode (edit)", d)
	}

	for _, c := range []struct {
		key, channel, mention, mode string
		want                        int
	}{
		{"tea-rooms", chTeaRooms, "", "edit", http.StatusOK},
		{"affiliates", chTeaRooms, "everyone", "replace", http.StatusOK},
		{"bookclub:yaoi", chBookClub, "role:" + roleStaff, "replace", http.StatusOK},
		{"bookclub:yaoi", chBookClub, "", "edit", http.StatusBadRequest}, // a list can't be edited in place
		{"tea-rooms", "not-a-channel", "", "new", http.StatusBadRequest},
		{"tea-rooms", chTeaRooms, "role:Staff", "new", http.StatusBadRequest},
		{"tea-rooms", chTeaRooms, "", "sometimes", http.StatusBadRequest},
		{"bookclub:nope", chBookClub, "", "new", http.StatusNotFound},
		{"raffles", chBookClub, "", "new", http.StatusNotFound},
	} {
		resp := env.putJSON(t, "/api/discord/destinations/"+c.key,
			map[string]string{"channel_id": c.channel, "mention": c.mention, "repost_mode": c.mode})
		resp.Body.Close()
		if resp.StatusCode != c.want {
			t.Errorf("PUT %s %v = %d; want %d", c.key, c, resp.StatusCode, c.want)
		}
	}

	// Each destination belongs to its own page.
	staff := newTestEnv(t)
	staff.loginStaff(t, "teahouse-tea-rooms")
	resp = staff.putJSON(t, "/api/discord/destinations/tea-rooms",
		map[string]string{"channel_id": chTeaRooms, "mention": "", "repost_mode": "edit"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("a tea rooms grantee setting its destination = %d; want 200", resp.StatusCode)
	}
	resp = staff.get(t, "/api/discord/destinations/affiliates")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a tea rooms grantee reading the affiliates destination = %d; want 403", resp.StatusCode)
	}
}

func TestTeaRoomPost_EditsInPlace(t *testing.T) {
	env := newTestEnv(t)
	f := env.discordReady(t)
	id := env.createTeaRoom(t, "Sakura Room", "7")
	path := fmt.Sprintf("/api/tea-rooms/%d/post", id)

	resp := env.postJSON(t, path, map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("post with no channel = %d; want 400", resp.StatusCode)
	}

	env.setDestination(t, "tea-rooms", chTeaRooms, "", "edit")
	res := env.discordAction(t, http.MethodPost, path)
	post, _ := res["post"].(map[string]any)
	msgs := f.Messages()
	if len(msgs) != 1 || post["state"] != "live" || !strings.Contains(post["url"].(string), "/"+chTeaRooms+"/"+msgs[0].ID) {
		t.Fatalf("first post: messages %+v, post %v", msgs, post)
	}
	if msgs[0].Embeds[0].Title != "Sakura Room" {
		t.Errorf("embed title = %q", msgs[0].Embeds[0].Title)
	}

	// A change is edited into the same message.
	resp = env.putJSON(t, fmt.Sprintf("/api/tea-rooms/%d", id), map[string]any{
		"tea_room": map[string]any{"name": "Sakura Suite", "room_number": "7"}})
	resp.Body.Close()
	env.discordAction(t, http.MethodPost, path)
	if got, _ := f.Message(msgs[0].ID); got.Edits != 1 || got.Embeds[0].Title != "Sakura Suite" || len(f.Messages()) != 1 {
		t.Errorf("after an update: %+v (%d messages)", got, len(f.Messages()))
	}

	// Deleted in Discord: the next post notices - even with nothing changed - and makes a
	// fresh one.
	f.DeleteInDiscord(msgs[0].ID)
	env.discordAction(t, http.MethodPost, path)
	now := f.Messages()
	if len(now) != 1 || now[0].ID == msgs[0].ID {
		t.Fatalf("after a delete in Discord: %+v; want one new message", now)
	}
	if p := env.roomPost(t, id); p["state"] != "live" || p["channel_id"] != chTeaRooms {
		t.Errorf("list post state = %v", p)
	}
}

func TestTeaRoomPost_NewAndReplace(t *testing.T) {
	env := newTestEnv(t)
	f := env.discordReady(t)
	id := env.createTeaRoom(t, "Koi Pond", "8")
	path := fmt.Sprintf("/api/tea-rooms/%d/post", id)

	env.setDestination(t, "tea-rooms", chTeaRooms, "", "new")
	env.discordAction(t, http.MethodPost, path)
	env.discordAction(t, http.MethodPost, path)
	if n := len(f.Messages()); n != 2 {
		t.Fatalf("new mode: %d messages; want 2", n)
	}

	env.setDestination(t, "tea-rooms", chTeaRooms, "", "replace")
	env.discordAction(t, http.MethodPost, path)
	if n := len(f.Messages()); n != 1 {
		t.Errorf("replace mode left %d messages; want just the new one", n)
	}
}

// TestTeaRoomPost_ConcurrentPostsMakeOne: two posts of one room at once (a double-click,
// two staff) are serialized, so in edit mode the second edits what the first posted.
func TestTeaRoomPost_ConcurrentPostsMakeOne(t *testing.T) {
	env := newTestEnv(t)
	f := env.discordReady(t)
	env.setDestination(t, "tea-rooms", chTeaRooms, "", "edit")
	id := env.createTeaRoom(t, "Twice Room", "12")
	path := fmt.Sprintf("/api/tea-rooms/%d/post", id)

	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			resp := env.postJSON(t, path, map[string]any{})
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("post = %d", resp.StatusCode)
			}
		})
	}
	wg.Wait()
	msgs := f.Messages()
	if len(msgs) != 1 {
		t.Fatalf("two concurrent posts left %d messages; want 1", len(msgs))
	}
	if p := env.roomPost(t, id); p["state"] != "live" || !strings.HasSuffix(p["url"].(string), "/"+msgs[0].ID) {
		t.Errorf("post state = %v; want the one message, live", p)
	}
}

func TestTeaRoomTakeDownAndDelete(t *testing.T) {
	env := newTestEnv(t)
	f := env.discordReady(t)
	env.setDestination(t, "tea-rooms", chTeaRooms, "", "edit")
	id := env.createTeaRoom(t, "Moon Room", "9")
	path := fmt.Sprintf("/api/tea-rooms/%d/post", id)

	env.discordAction(t, http.MethodPost, path)
	res := env.discordAction(t, http.MethodDelete, path)
	if len(f.Messages()) != 0 || res["post"] != nil {
		t.Fatalf("take down: %d messages left, post %v", len(f.Messages()), res["post"])
	}
	if p := env.roomPost(t, id); p != nil {
		t.Errorf("a taken-down room still lists a post: %v", p)
	}

	// Deleting the room takes its post down too.
	env.discordAction(t, http.MethodPost, path)
	resp := env.del(t, fmt.Sprintf("/api/tea-rooms/%d", id))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || len(f.Messages()) != 0 {
		t.Errorf("delete = %d, %d messages left; want the post gone with the room", resp.StatusCode, len(f.Messages()))
	}
}

func TestDiscordPost_TestModeGoesToTheTestChannelWithoutPings(t *testing.T) {
	env := newTestEnv(t)
	f := env.discordReady(t)
	resp := env.putJSON(t, "/api/discord/config", map[string]any{
		"guild_id": discordtest.GuildID, "application_id": discordtest.AppID,
		"public_base_url": "https://apps.senpan.cafe", "test_mode": true, "test_channel_id": chTest,
	})
	resp.Body.Close()
	env.setDestination(t, "tea-rooms", chTeaRooms, "everyone", "edit")
	id := env.createTeaRoom(t, "Test Room", "10")

	res := env.discordAction(t, http.MethodPost, fmt.Sprintf("/api/tea-rooms/%d/post", id))
	msgs := f.Messages()
	if len(msgs) != 1 || msgs[0].ChannelID != chTest {
		t.Fatalf("messages = %+v; want one in the test channel", msgs)
	}
	if msgs[0].Content != "@everyone" || len(msgs[0].AllowedMentions.Parse) != 0 {
		t.Errorf("content %q, mentions %+v; want the text without a ping", msgs[0].Content, msgs[0].AllowedMentions)
	}
	if !strings.Contains(res["note"].(string), "test channel") {
		t.Errorf("note = %q; want it to say where the post went", res["note"])
	}
}

func TestDiscordPost_DryRunSendsNothing(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.withDiscordPolicy(t, true, "")
	env.setDiscordToken(t, discordtest.Token)
	env.setDestination(t, "tea-rooms", chTeaRooms, "", "new")
	id := env.createTeaRoom(t, "Dry Room", "11")

	res := env.discordAction(t, http.MethodPost, fmt.Sprintf("/api/tea-rooms/%d/post", id))
	if len(f.Messages()) != 0 || !strings.Contains(res["note"].(string), "dry-run") {
		t.Errorf("dry run: %d messages, note %q", len(f.Messages()), res["note"])
	}
}

func TestAffiliatePost_UsesThePublicSiteURL(t *testing.T) {
	env := newTestEnv(t)
	f := env.discordReady(t)
	env.setDestination(t, "affiliates", chTeaRooms, "", "edit")
	id := env.createAffiliate(t, "Moogle Mart")
	resp := env.putJSON(t, fmt.Sprintf("/api/affiliates/%d", id), map[string]any{
		"name": "Moogle Mart", "logo": "images/affiliate_logos/moogle mart.png"})
	resp.Body.Close()

	env.discordAction(t, http.MethodPost, fmt.Sprintf("/api/affiliates/%d/post", id))
	msgs := f.Messages()
	if len(msgs) != 1 || msgs[0].Embeds[0].Thumbnail == nil {
		t.Fatalf("messages = %+v", msgs)
	}
	if got := msgs[0].Embeds[0].Thumbnail.URL; got != "https://apps.senpan.cafe/images/affiliate_logos/moogle%20mart.png" {
		t.Errorf("logo URL = %q; want it built from the public site URL", got)
	}
	list := decodeBody(t, env.get(t, "/api/affiliates"))
	if a := list["affiliates"].([]any)[0].(map[string]any); a["discord_post"] == nil {
		t.Errorf("the affiliates page doesn't show the post: %v", a)
	}

	resp = env.del(t, fmt.Sprintf("/api/affiliates/%d", id))
	resp.Body.Close()
	if len(f.Messages()) != 0 {
		t.Error("deleting the affiliate left its post up")
	}
}

// addItems adds titled items to a reading list.
func addItems(t *testing.T, e *testEnv, club string, listID int64, titles ...string) {
	t.Helper()
	for _, title := range titles {
		resp := e.postJSON(t, "/api/book-clubs/"+club+"/reading-lists/"+itoa(listID)+"/items", map[string]any{
			"item": map[string]any{"title": title, "format": "Manga", "summary": title + " summary", "comments": "Loved it."},
		})
		resp.Body.Close()
	}
}

func publish(t *testing.T, e *testEnv, club string, listID int64) *http.Response {
	t.Helper()
	return e.postJSON(t, "/api/book-clubs/"+club+"/reading-lists/"+itoa(listID)+"/publish", map[string]any{})
}

func TestPublishReadingList_OneMessagePerItemPingingOnce(t *testing.T) {
	env := newTestEnv(t)
	f := env.discordReady(t)
	listID := createReadingListIn(t, env, "yuri", "Yuri Faves")
	addItems(t, env, "yuri", listID, "Alpha", "Beta", "Gamma")

	resp := publish(t, env, "yuri", listID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("publish with no channel = %d; want 400", resp.StatusCode)
	}

	env.setDestination(t, "bookclub:yuri", chBookClub, "role:"+roleStaff, "new")
	body := decodeBody(t, publish(t, env, "yuri", listID))
	if body["published"] != float64(3) {
		t.Fatalf("published = %v; want 3", body["published"])
	}
	msgs := f.Messages()
	if len(msgs) != 3 {
		t.Fatalf("messages = %d; want 3", len(msgs))
	}
	for i, m := range msgs {
		if len(m.Embeds) != 1 || m.ChannelID != chBookClub {
			t.Errorf("message %d: %+v", i, m)
		}
		pinged := len(m.AllowedMentions.Roles) == 1
		if pinged != (i == 0) {
			t.Errorf("message %d pings = %+v; want only the first to ping", i, m.AllowedMentions)
		}
	}
	// The comments field is labeled for the club.
	var labeled bool
	for _, field := range msgs[0].Embeds[0].Fields {
		labeled = labeled || field.Name == "Drani's Comments"
	}
	if !labeled {
		t.Errorf("fields = %+v; want the Yuri club's comments label", msgs[0].Embeds[0].Fields)
	}
}

func TestPublishReadingList_ReplacesThePreviousList(t *testing.T) {
	env := newTestEnv(t)
	f := env.discordReady(t)
	env.setDestination(t, "bookclub:yaoi", chBookClub, "", "replace")
	first := createReadingList(t, env, "Spring")
	addItems(t, env, "yaoi", first, "One", "Two")
	second := createReadingList(t, env, "Summer")
	addItems(t, env, "yaoi", second, "Three")

	decodeBody(t, publish(t, env, "yaoi", first))
	body := decodeBody(t, publish(t, env, "yaoi", second))
	msgs := f.Messages()
	if body["replaced"] != float64(2) || len(msgs) != 1 || msgs[0].Embeds[0].Title != "Three" {
		t.Errorf("replaced = %v, messages %+v; want only the new list left", body["replaced"], msgs)
	}
}

func TestPublishReadingList_FailurePartwayLeavesTheChannelAsItWas(t *testing.T) {
	env := newTestEnv(t)
	f := env.discordReady(t)
	env.setDestination(t, "bookclub:yaoi", chBookClub, "", "replace")
	old := createReadingList(t, env, "Old")
	addItems(t, env, "yaoi", old, "Kept")
	decodeBody(t, publish(t, env, "yaoi", old))

	next := createReadingList(t, env, "New")
	addItems(t, env, "yaoi", next, "A", "B", "C")
	f.FailAfter("POST /channels/", 1, http.StatusInternalServerError, `{"message":"boom","code":0}`)
	resp := publish(t, env, "yaoi", next)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway || !strings.Contains(string(b), "removed") {
		t.Fatalf("publish = %d %s; want a 502 saying what was rolled back", resp.StatusCode, b)
	}
	msgs := f.Messages()
	if len(msgs) != 1 || msgs[0].Embeds[0].Title != "Kept" {
		t.Errorf("messages = %+v; want just the old list", msgs)
	}
}

func TestAnnouncementSend_PostsWithItsPingAndButtons(t *testing.T) {
	env := newTestEnv(t)
	f := env.discordReady(t)

	tr := env.postJSON(t, "/api/announcement-types", map[string]any{"name": "Events", "channel_id": "nope"})
	tr.Body.Close()
	if tr.StatusCode != http.StatusBadRequest {
		t.Fatalf("a type with a bad channel = %d; want 400", tr.StatusCode)
	}
	tr = env.postJSON(t, "/api/announcement-types", map[string]any{"name": "Events", "channel_id": chAnnouncements})
	typeID := int(decodeBody(t, tr)["type"].(map[string]any)["id"].(float64))
	cr := env.postJSON(t, "/api/announcements", map[string]any{"announcement": map[string]any{
		"type_id": typeID, "title": "Tea Party", "details": "Tonight!", "active": true,
		"mention": "role:" + roleStaff, "repost_mode": "replace",
		"buttons": []map[string]string{{"label": "Join", "url": "https://example.com"}},
	}})
	if cr.StatusCode != http.StatusCreated {
		t.Fatalf("create announcement = %d", cr.StatusCode)
	}
	id := int(decodeBody(t, cr)["announcement"].(map[string]any)["id"].(float64))

	send := func() map[string]any {
		t.Helper()
		resp := env.postJSON(t, fmt.Sprintf("/api/announcements/%d/send", id), map[string]any{})
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("send = %d: %s", resp.StatusCode, b)
		}
		return decodeBody(t, resp)["announcement"].(map[string]any)
	}
	a := send()
	msgs := f.Messages()
	if len(msgs) != 1 || msgs[0].Content != "<@&"+roleStaff+">" || len(msgs[0].AllowedMentions.Roles) != 1 {
		t.Fatalf("message = %+v; want the role ping", msgs)
	}
	if len(msgs[0].Components) != 1 || msgs[0].Components[0].Components[0].URL != "https://example.com" {
		t.Errorf("components = %+v; want the link button", msgs[0].Components)
	}
	if url, _ := a["discord_url"].(string); !strings.HasSuffix(url, "/"+chAnnouncements+"/"+msgs[0].ID) || a["last_posted_at"] == "" {
		t.Errorf("announcement = %v; want a link to the post and a last-posted time", a)
	}

	// Replace: the second send leaves only itself.
	send()
	if now := f.Messages(); len(now) != 1 || now[0].ID == msgs[0].ID {
		t.Errorf("after a second send: %+v; want only the new post", now)
	}
}

func TestAnnouncementSend_TypeWithoutAChannel(t *testing.T) {
	env := newTestEnv(t)
	env.discordReady(t)
	tr := env.postJSON(t, "/api/announcement-types", map[string]any{"name": "Nowhere", "channel_id": ""})
	typeID := int(decodeBody(t, tr)["type"].(map[string]any)["id"].(float64))
	cr := env.postJSON(t, "/api/announcements", map[string]any{"announcement": map[string]any{
		"type_id": typeID, "title": "Lost", "details": "x", "active": true}})
	id := int(decodeBody(t, cr)["announcement"].(map[string]any)["id"].(float64))
	resp := env.postJSON(t, fmt.Sprintf("/api/announcements/%d/send", id), map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("send with no channel = %d; want 400", resp.StatusCode)
	}
}
