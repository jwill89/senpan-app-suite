package server

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"app-suite/internal/discord"
	"app-suite/internal/discord/discordtest"
	"app-suite/internal/model"
	"app-suite/internal/store"
	"app-suite/internal/ws"
)

// newEditEnv builds a Server with a fake Discord, a tea room posted to it in edit mode,
// and returns the server, the fake, the room id and its message id.
func newEditEnv(t *testing.T) (*Server, *discordtest.Server, int64, string) {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	fake := discordtest.New()
	ts := httptest.NewServer(fake.Handler())
	t.Cleanup(ts.Close)
	svc := discord.NewService(ts.URL, "")
	svc.Configure(discord.Config{Token: discordtest.Token, GuildID: discordtest.GuildID})
	s := &Server{store: st, discord: svc, hub: ws.NewHub()}

	if err := st.SaveDiscordDestination(model.DiscordDestination{Key: destTeaRooms,
		ChannelID: "200000000000000003", RepostMode: model.RepostEdit}); err != nil {
		t.Fatal(err)
	}
	room := &model.TeaRoom{Name: "Sakura Room", RoomNumber: "7"}
	id, err := st.CreateTeaRoom(room)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.postDiscordItem(context.Background(), teaRoomItems, id); err != nil {
		t.Fatal(err)
	}
	msgs := fake.Messages()
	if len(msgs) != 1 {
		t.Fatalf("setup posted %d messages", len(msgs))
	}
	return s, fake, id, msgs[0].ID
}

func TestEditQueueCoalescesABurst(t *testing.T) {
	var s Server
	for range 3 {
		s.queueDiscordEdit(postKindTeaRoom, 1)
	}
	s.queueDiscordEdit(postKindAffiliate, 1)
	if due := s.editQueue.take(time.Now(), false); len(due) != 0 {
		t.Errorf("edits ran before their delay: %v", due)
	}
	due := s.editQueue.take(time.Now().Add(discordEditDelay+time.Second), false)
	if len(due) != 2 {
		t.Errorf("due = %v; want one edit per item", due)
	}
	if _, pending := s.editQueue.next(); pending {
		t.Error("taking the edits left some pending")
	}
}

func TestApplyDiscordEditKeepsThePostCurrent(t *testing.T) {
	s, fake, id, msgID := newEditEnv(t)
	ctx := context.Background()

	if err := s.store.SetTeaRoomOpen(id, true); err != nil {
		t.Fatal(err)
	}
	s.applyDiscordEdit(ctx, postKindTeaRoom, id)
	m, _ := fake.Message(msgID)
	if m.Edits != 1 || fieldValue(m.Embeds[0], "🚪 Status") != "Open" {
		t.Fatalf("after opening the room: %d edits, status %q", m.Edits, fieldValue(m.Embeds[0], "🚪 Status"))
	}

	// Nothing changed since: nothing is sent.
	s.applyDiscordEdit(ctx, postKindTeaRoom, id)
	if m, _ := fake.Message(msgID); m.Edits != 1 {
		t.Errorf("an unchanged room was edited again (%d edits)", m.Edits)
	}

	// Not in edit mode: the post is left alone.
	if err := s.store.SaveDiscordDestination(model.DiscordDestination{Key: destTeaRooms,
		ChannelID: "200000000000000003", RepostMode: model.RepostNew}); err != nil {
		t.Fatal(err)
	}
	if err := s.store.SetTeaRoomOpen(id, false); err != nil {
		t.Fatal(err)
	}
	s.applyDiscordEdit(ctx, postKindTeaRoom, id)
	if m, _ := fake.Message(msgID); m.Edits != 1 {
		t.Errorf("a destination in new mode still edited the post (%d edits)", m.Edits)
	}
}

func TestApplyDiscordEditMarksADeletedPostGone(t *testing.T) {
	s, fake, id, msgID := newEditEnv(t)
	fake.DeleteInDiscord(msgID)
	if err := s.store.SetTeaRoomOpen(id, true); err != nil {
		t.Fatal(err)
	}
	s.applyDiscordEdit(context.Background(), postKindTeaRoom, id)

	if st := s.discordItemState(postKindTeaRoom, id); st == nil || st.State != model.PostGone {
		t.Fatalf("state = %+v; want gone", st)
	}
	if n := len(fake.Messages()); n != 0 {
		t.Errorf("an automatic update reposted (%d messages); it must leave that to an admin", n)
	}
}

func TestApplyDiscordEditLeavesRealPostsAloneInTestMode(t *testing.T) {
	s, fake, id, msgID := newEditEnv(t)
	for k, v := range map[string]string{settingDiscordTestMode: "1", settingDiscordTestChannel: "200000000000000005"} {
		if err := s.store.SetSetting(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.store.SetTeaRoomOpen(id, true); err != nil {
		t.Fatal(err)
	}
	s.applyDiscordEdit(context.Background(), postKindTeaRoom, id)
	if m, _ := fake.Message(msgID); m.Edits != 0 {
		t.Errorf("test mode edited a post in the real channel (%d edits)", m.Edits)
	}
}

func TestRunDiscordWorkerSendsPendingEditsOnShutdown(t *testing.T) {
	s, fake, id, msgID := newEditEnv(t)
	if err := s.store.SetTeaRoomOpen(id, true); err != nil {
		t.Fatal(err)
	}
	s.queueDiscordEdit(postKindTeaRoom, id)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.RunDiscordWorker(ctx)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker didn't stop")
	}
	if m, _ := fake.Message(msgID); m.Edits != 1 {
		t.Errorf("a change queued before shutdown never reached Discord (%d edits)", m.Edits)
	}
}

// fieldValue returns an embed field's value by name ("" when absent).
func fieldValue(e discord.Embed, name string) string {
	for _, f := range e.Fields {
		if f.Name == name {
			return f.Value
		}
	}
	return ""
}
