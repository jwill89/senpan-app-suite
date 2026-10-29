package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"app-suite/internal/discord/discordtest"
	"app-suite/internal/model"
)

const otherGuild = "123456789012345678"

// seedImport sets up the four kinds of outcome the importer has to handle, as the v70
// migration would leave them, and returns the announcement type's id.
func seedImport(t *testing.T, env *testEnv, f *discordtest.Server) int64 {
	t.Helper()
	typeID, err := env.store.CreateAnnouncementType("Events", "")
	if err != nil {
		t.Fatal(err)
	}
	hooks := map[string]discordtest.Webhook{
		"111111111111111111": {Token: "a", GuildID: discordtest.GuildID, ChannelID: "200000000000000002"},
		"222222222222222222": {Token: "b", GuildID: discordtest.GuildID, ChannelID: "200000000000000004"},
		"333333333333333333": {Token: "c", GuildID: otherGuild, ChannelID: "700000000000000001"},
	}
	for id, h := range hooks {
		f.AddWebhook(id, h)
	}
	for _, row := range []struct{ source, label, url, id string }{
		{"announcement_type:" + itoa(typeID), "Events", "https://discord.com/api/webhooks/111111111111111111/a", "111111111111111111"},
		{"bookclub:yaoi", "Book club: yaoi", "https://discord.com/api/webhooks/222222222222222222/b", "222222222222222222"},
		{"tea-rooms", "Tea Rooms", "https://discord.com/api/webhooks/333333333333333333/c", "333333333333333333"},
		{"affiliates", "Affiliates", "https://discord.com/api/webhooks/444444444444444444/gone", "444444444444444444"},
	} {
		if err := env.store.AddLegacyWebhook(row.source, row.label, row.url, row.id); err != nil {
			t.Fatal(err)
		}
	}
	return typeID
}

func TestDiscordImport_ResolvesEachWebhook(t *testing.T) {
	env := newTestEnv(t)
	f := env.withDiscord(t)
	typeID := seedImport(t, env, f)

	env.srv.RunDiscordImport(context.Background())

	typ, err := env.store.GetAnnouncementType(typeID)
	if err != nil || typ.ChannelID != "200000000000000002" {
		t.Fatalf("announcement type channel = %+v, %v", typ, err)
	}
	if d, _ := env.store.GetDiscordDestination("bookclub:yaoi"); d.ChannelID != "200000000000000004" {
		t.Fatalf("book club destination = %+v", d)
	}
	if d, _ := env.store.GetDiscordDestination("tea-rooms"); d.ChannelID != "" {
		t.Fatalf("another server's channel was adopted: %+v", d)
	}

	rows, err := env.store.LegacyWebhooks()
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	for _, r := range rows {
		status[r.Source] = r.Status
		if r.URL != "" {
			t.Errorf("%s still holds its URL after settling as %s", r.Source, r.Status)
		}
	}
	want := map[string]string{
		"announcement_type:" + itoa(typeID): model.ImportImported,
		"bookclub:yaoi":                     model.ImportImported,
		"tea-rooms":                         model.ImportOtherServer,
		"affiliates":                        model.ImportWebhookDeleted,
	}
	for source, s := range want {
		if status[source] != s {
			t.Errorf("%s = %q, want %q", source, status[source], s)
		}
	}

	// A second run has nothing left to do.
	calls := len(f.Calls())
	env.srv.RunDiscordImport(context.Background())
	if len(f.Calls()) != calls {
		t.Fatal("a settled import looked webhooks up again")
	}
}

func TestDiscordImport_KeepsAChannelAlreadyChosen(t *testing.T) {
	env := newTestEnv(t)
	f := env.withDiscord(t)
	seedImport(t, env, f)
	if err := env.store.SaveDiscordDestination(model.DiscordDestination{Key: "bookclub:yaoi",
		ChannelID: "200000000000000005", RepostMode: model.RepostReplace}); err != nil {
		t.Fatal(err)
	}
	env.srv.RunDiscordImport(context.Background())
	if d, _ := env.store.GetDiscordDestination("bookclub:yaoi"); d.ChannelID != "200000000000000005" || d.RepostMode != model.RepostReplace {
		t.Fatalf("the admin's choice was overwritten: %+v", d)
	}
}

func TestDiscordImport_TransientErrorsAreRetried(t *testing.T) {
	env := newTestEnv(t)
	f := env.withDiscord(t)
	typeID := seedImport(t, env, f)
	for range 4 {
		f.Fail("GET /webhooks/", http.StatusBadGateway, "")
	}
	env.srv.RunDiscordImport(context.Background())
	todo, _ := env.store.LegacyWebhooksToImport()
	if len(todo) != 4 {
		t.Fatalf("after a Discord outage, %d left to import; want all 4 kept with their URLs", len(todo))
	}

	env.loginAdmin(t)
	resp := env.postJSON(t, "/api/discord/import/retry", nil)
	body := decodeBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("retry = %d %v", resp.StatusCode, body)
	}
	if typ, _ := env.store.GetAnnouncementType(typeID); typ.ChannelID != "200000000000000002" {
		t.Fatalf("the retry didn't import: %+v", typ)
	}
}

func TestDiscordImport_ReportNamesWhereToLook(t *testing.T) {
	env := newTestEnv(t)
	f := env.withDiscord(t)
	typeID := seedImport(t, env, f)
	env.srv.RunDiscordImport(context.Background())
	// The type was renamed since: the report names it as it is now.
	if err := env.store.UpdateAnnouncementType(typeID, "Club Events", ""); err != nil {
		t.Fatal(err)
	}

	resp := env.get(t, "/api/discord/import")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous report = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
	env.loginAdmin(t)
	body := decodeBody(t, env.get(t, "/api/discord/import"))
	got := map[string]map[string]any{}
	for _, r := range body["rows"].([]any) {
		row := r.(map[string]any)
		got[row["source"].(string)] = row
		if _, leaked := row["url"]; leaked {
			t.Fatal("the report carries a webhook URL")
		}
	}
	if got["bookclub:yaoi"]["label"] != "Yaoi Book Club reading lists" ||
		!strings.Contains(got["bookclub:yaoi"]["where"].(string), "Yaoi Book Club") {
		t.Errorf("book club row = %v", got["bookclub:yaoi"])
	}
	if got["announcement_type:"+itoa(typeID)]["label"] != "Announcement type: Club Events" {
		t.Errorf("type row = %v", got["announcement_type:"+itoa(typeID)])
	}
	if got["tea-rooms"]["status"] != model.ImportOtherServer || got["tea-rooms"]["detail"] == "" {
		t.Errorf("tea rooms row = %v", got["tea-rooms"])
	}
}

func TestDiscordImport_CleanupDeletesOnlyOurServersWebhooks(t *testing.T) {
	env := newTestEnv(t)
	f := env.withDiscord(t)
	seedImport(t, env, f)
	env.srv.RunDiscordImport(context.Background())
	env.loginAdmin(t)
	env.setDiscordToken(t, discordtest.Token)

	resp := env.del(t, "/api/discord/legacy-webhooks")
	body := decodeBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cleanup = %d %v", resp.StatusCode, body)
	}
	if body["deleted"] != float64(2) || body["skipped"] != float64(2) || body["failed"] != float64(0) {
		t.Fatalf("cleanup counts = %v", body)
	}
	if !f.WebhookDeleted("111111111111111111") || !f.WebhookDeleted("222222222222222222") {
		t.Fatal("our server's old webhooks were not deleted")
	}
	if f.WebhookDeleted("333333333333333333") {
		t.Fatal("another server's webhook was touched")
	}
	if rows, _ := env.store.LegacyWebhooks(); len(rows) != 0 {
		t.Fatalf("report not cleared: %v", rows)
	}
}

func TestDiscordImport_CleanupHonorsDryRun(t *testing.T) {
	env := newTestEnv(t)
	f := env.withDiscordPolicy(t, true, "")
	seedImport(t, env, f)
	env.srv.RunDiscordImport(context.Background())
	env.loginAdmin(t)
	env.setDiscordToken(t, discordtest.Token)
	body := decodeBody(t, env.del(t, "/api/discord/legacy-webhooks"))
	if body["deleted"] != float64(0) || f.WebhookDeleted("111111111111111111") {
		t.Fatalf("dry run deleted a webhook: %v", body)
	}
}
