package store

import (
	"path/filepath"
	"testing"
)

// TestMigrateDiscordBotMovesEveryWebhook runs v70 over a database holding webhooks the
// way v69 did - on announcement types and in four settings - and checks every URL ends up
// in the import table, and nowhere else.
func TestMigrateDiscordBotMovesEveryWebhook(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "v69.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	db := s.db

	const (
		events   = "https://discord.com/api/webhooks/111111111111111111/eventsToken"
		openings = "https://discord.com/api/webhooks/222222222222222222/openingsToken"
		yaoi     = "https://discord.com/api/webhooks/333333333333333333/yaoiToken"
		rooms    = "https://discord.com/api/webhooks/444444444444444444/roomsToken"
		affil    = "https://discord.com/api/webhooks/555555555555555555/affilToken"
	)
	// Raw SQL: the store no longer writes webhook URLs.
	var eventsID int64
	for _, typ := range []struct{ name, url string }{{"Events", events}, {"Openings", openings}, {"No Webhook", ""}} {
		res, err := db.Exec(`INSERT INTO announcement_types (name, webhook_url) VALUES (?, ?)`, typ.name, typ.url)
		if err != nil {
			t.Fatal(err)
		}
		if eventsID == 0 {
			eventsID, _ = res.LastInsertId()
		}
	}
	for key, value := range map[string]string{
		"discord_webhook_url_yaoi": yaoi,
		"discord_webhook_url_yuri": "", // set but empty: nothing to move
		"tearoom_webhook_url":      rooms,
		"affiliate_webhook_url":    affil,
		"app_title":                "Senpan", // unrelated, untouched
	} {
		if err := s.SetSetting(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("PRAGMA user_version = 69"); err != nil {
		t.Fatal(err)
	}
	if err := ensureSchema(db); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}

	rows, err := s.LegacyWebhooks()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]LegacyWebhook{}
	for _, r := range rows {
		got[r.Source] = r
	}
	want := map[string]struct{ label, url, id string }{
		"announcement_type:1": {"Events", events, "111111111111111111"},
		"announcement_type:2": {"Openings", openings, "222222222222222222"},
		"bookclub:yaoi":       {"Book club: yaoi", yaoi, "333333333333333333"},
		"tea-rooms":           {"Tea Rooms", rooms, "444444444444444444"},
		"affiliates":          {"Affiliates", affil, "555555555555555555"},
	}
	if eventsID != 1 {
		t.Fatalf("test assumes the first type is id 1, got %d", eventsID)
	}
	if len(got) != len(want) {
		t.Fatalf("import rows = %v, want %d", rows, len(want))
	}
	for source, w := range want {
		r, ok := got[source]
		if !ok {
			t.Errorf("missing import row %s", source)
			continue
		}
		if r.Label != w.label || r.URL != w.url || r.WebhookID != w.id || r.Status != "pending" {
			t.Errorf("%s = %+v, want label %q url %q id %q pending", source, r, w.label, w.url, w.id)
		}
	}

	// The originals are gone: the types' URLs blanked, the webhook settings deleted.
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM announcement_types WHERE webhook_url <> ''`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d announcement types still hold a webhook URL", left)
	}
	for _, key := range []string{"discord_webhook_url_yaoi", "discord_webhook_url_yuri", "tearoom_webhook_url", "affiliate_webhook_url"} {
		if v, _ := s.GetSetting(key); v != "" {
			t.Errorf("setting %s still holds %q", key, v)
		}
	}
	if v, _ := s.GetSetting("app_title"); v != "Senpan" {
		t.Errorf("an unrelated setting was touched: %q", v)
	}

	// New columns exist with their defaults.
	if !hasColumn(db, "announcement_types", "channel_id") || !hasColumn(db, "announcements", "repost_mode") {
		t.Fatal("v70 columns missing")
	}

	// Running it again moves nothing more and changes nothing.
	if err := migrateDiscordBot(db); err != nil {
		t.Fatalf("second run: %v", err)
	}
	again, _ := s.LegacyWebhooks()
	if len(again) != len(want) {
		t.Fatalf("second run changed the import rows: %d", len(again))
	}
}

// TestMigrateDiscordBotRewritesRoleMentions checks that pings naming an announcement_roles
// row become pings naming the Discord role, and that a second run leaves them alone.
func TestMigrateDiscordBotRewritesRoleMentions(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "v69.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	db := s.db

	for _, stmt := range []string{
		`INSERT INTO announcement_types (id, name) VALUES (1, 'Events')`,
		`INSERT INTO announcement_roles (id, name, role_id) VALUES (3, 'Staff', '1100000000000000003')`,
		`INSERT INTO announcement_roles (id, name, role_id) VALUES (4, 'No snowflake', '')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	mentions := map[string]string{
		"staff":    "role:3",
		"blank":    "role:4",
		"deleted":  "role:9", // its role row is gone
		"everyone": "everyone",
		"none":     "",
	}
	for title, mention := range mentions {
		if _, err := db.Exec(`INSERT INTO announcements (type_id, title, mention) VALUES (1, ?, ?)`, title, mention); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("PRAGMA user_version = 69"); err != nil {
		t.Fatal(err)
	}
	if err := ensureSchema(db); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}
	want := map[string]string{
		"staff":    "role:1100000000000000003",
		"blank":    "",
		"deleted":  "",
		"everyone": "everyone",
		"none":     "",
	}
	check := func(when string) {
		t.Helper()
		for title, w := range want {
			var got string
			if err := db.QueryRow(`SELECT mention FROM announcements WHERE title = ?`, title).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != w {
				t.Errorf("%s: %s mention = %q, want %q", when, title, got, w)
			}
		}
	}
	check("after v70")
	if err := migrateDiscordBot(db); err != nil {
		t.Fatalf("second run: %v", err)
	}
	check("after a second run")
}
