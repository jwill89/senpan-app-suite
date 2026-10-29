package store_test

import (
	"testing"

	"app-suite/internal/model"
)

func TestDiscordDestinations(t *testing.T) {
	s := newTestStore(t)
	d, err := s.GetDiscordDestination("tea-rooms")
	if err != nil {
		t.Fatal(err)
	}
	// Tea rooms keep one post up to date unless an admin says otherwise; a book club adds
	// each list, as posting always did.
	if d.Key != "tea-rooms" || d.ChannelID != "" || d.RepostMode != model.RepostEdit {
		t.Fatalf("unsaved destination = %+v", d)
	}
	if club, _ := s.GetDiscordDestination("bookclub:yaoi"); club.RepostMode != model.RepostNew {
		t.Fatalf("unsaved book club destination = %+v", club)
	}

	// The importer fills an empty channel...
	set, err := s.SetDiscordDestinationChannelIfEmpty("tea-rooms", "200000000000000003")
	if err != nil || !set {
		t.Fatalf("first import = %v, %v", set, err)
	}
	if d, _ := s.GetDiscordDestination("tea-rooms"); d.RepostMode != model.RepostEdit {
		t.Fatalf("imported destination mode = %q; want the tea rooms default", d.RepostMode)
	}
	// ...but never overwrites one.
	set, err = s.SetDiscordDestinationChannelIfEmpty("tea-rooms", "209999999999999999")
	if err != nil || set {
		t.Fatalf("second import = %v, %v; want false", set, err)
	}

	if err := s.SaveDiscordDestination(model.DiscordDestination{
		Key: "tea-rooms", ChannelID: "200000000000000004", Mention: "everyone", RepostMode: model.RepostEdit,
	}); err != nil {
		t.Fatal(err)
	}
	d, _ = s.GetDiscordDestination("tea-rooms")
	if d.ChannelID != "200000000000000004" || d.Mention != "everyone" || d.RepostMode != model.RepostEdit || d.UpdatedAt == "" {
		t.Fatalf("saved destination = %+v", d)
	}
}

func TestAnnouncementTypeChannelIfEmpty(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateAnnouncementType("Events", "")
	if err != nil {
		t.Fatal(err)
	}
	if set, _ := s.SetAnnouncementTypeChannelIfEmpty(id, "200000000000000002"); !set {
		t.Fatal("an empty type channel was not filled")
	}
	if set, _ := s.SetAnnouncementTypeChannelIfEmpty(id, "209999999999999999"); set {
		t.Fatal("a set type channel was overwritten")
	}
	if set, _ := s.SetAnnouncementTypeChannelIfEmpty(9999, "200000000000000002"); set {
		t.Fatal("a missing type reported a change")
	}
}

func TestDiscordPostsTrackLiveMessages(t *testing.T) {
	s := newTestStore(t)
	first, err := s.RecordDiscordPost(model.DiscordPost{Kind: "reading_list", RefID: 7, Scope: "yaoi", Batch: "b1", Seq: 0,
		ChannelID: "200000000000000004", MessageID: "400000000000000001"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordDiscordPost(model.DiscordPost{Kind: "reading_list", RefID: 7, Scope: "yaoi", Batch: "b1", Seq: 1,
		ChannelID: "200000000000000004", MessageID: "400000000000000002"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordDiscordPost(model.DiscordPost{Kind: "reading_list", RefID: 8, Scope: "yuri",
		ChannelID: "200000000000000004", MessageID: "400000000000000003"}); err != nil {
		t.Fatal(err)
	}

	live, err := s.LiveDiscordPostsInScope("reading_list", "yaoi")
	if err != nil || len(live) != 2 || live[0].Seq != 0 || live[0].PostedAt == "" {
		t.Fatalf("yaoi live = %+v, %v", live, err)
	}

	// Marking gone needs the matching message id, so a stale caller can't hit a newer post.
	if err := s.MarkDiscordPostGone(first, "999"); err != nil {
		t.Fatal(err)
	}
	if live, _ := s.LiveDiscordPosts("reading_list", 7); len(live) != 2 {
		t.Fatal("a mismatched message id marked a post gone")
	}
	if err := s.MarkDiscordPostGone(first, "400000000000000001"); err != nil {
		t.Fatal(err)
	}
	live, _ = s.LiveDiscordPosts("reading_list", 7)
	if len(live) != 1 || live[0].MessageID != "400000000000000002" {
		t.Fatalf("after gone = %+v", live)
	}

	if err := s.MarkDiscordPostEdited(live[0].ID, "fp2"); err != nil {
		t.Fatal(err)
	}
	live, _ = s.LiveDiscordPosts("reading_list", 7)
	if live[0].Fingerprint != "fp2" || live[0].EditedAt == "" {
		t.Fatalf("after edit = %+v", live[0])
	}
}

func TestLegacyWebhookSettling(t *testing.T) {
	s := newTestStore(t)
	// A real install fills the table in the v70 migration; a test seeds it directly.
	if _, err := s.CreateAnnouncementType("Events", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.AddLegacyWebhook("tea-rooms", "Tea Rooms", "https://discord.com/api/webhooks/1/tok", "1"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddLegacyWebhook("affiliates", "Affiliates", "https://discord.com/api/webhooks/2/tok", "2"); err != nil {
		t.Fatal(err)
	}
	todo, err := s.LegacyWebhooksToImport()
	if err != nil || len(todo) != 2 {
		t.Fatalf("to import = %+v, %v", todo, err)
	}

	// An error keeps the URL for the next try; anything else blanks it.
	if err := s.SettleLegacyWebhook("tea-rooms", model.ImportError, "timeout", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SettleLegacyWebhook("affiliates", model.ImportImported, "", "999", "200000000000000003"); err != nil {
		t.Fatal(err)
	}
	todo, _ = s.LegacyWebhooksToImport()
	if len(todo) != 1 || todo[0].Source != "tea-rooms" || todo[0].URL == "" {
		t.Fatalf("after settling = %+v", todo)
	}
	all, _ := s.LegacyWebhooks()
	for _, w := range all {
		if w.Source == "affiliates" && (w.URL != "" || w.Status != model.ImportImported || w.ChannelID != "200000000000000003" || w.SettledAt == "") {
			t.Fatalf("imported row = %+v", w)
		}
	}

	if err := s.ClearLegacyWebhooks(); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.LegacyWebhooks(); len(all) != 0 {
		t.Fatalf("after clearing = %+v", all)
	}
}
