package discord_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"app-suite/internal/discord"
	"app-suite/internal/discord/discordtest"
)

// fake starts a fake Discord and a client for it.
func fake(t *testing.T) (*discordtest.Server, *discord.Client) {
	t.Helper()
	f := discordtest.New()
	srv := httptest.NewServer(f.Handler())
	t.Cleanup(srv.Close)
	return f, discord.New(discordtest.Token, srv.URL)
}

const (
	announcements = "200000000000000002"
	teaRooms      = "200000000000000003"
)

func TestNewWithoutTokenIsOff(t *testing.T) {
	if discord.New("  ", "") != nil {
		t.Fatal("a blank token must give a nil client")
	}
}

func TestSendEditDeleteMessage(t *testing.T) {
	f, c := fake(t)
	ctx := context.Background()
	posted, err := c.SendMessage(ctx, teaRooms, discord.Message{
		Content: "<@&300000000000000002>",
		Embeds:  []discord.Embed{{Title: "Jasmine Room", Color: 0xE53170}},
		Components: []discord.Component{{Type: discord.ComponentActionRow, Components: []discord.Component{
			{Type: discord.ComponentButton, Style: discord.ButtonStyleLink, Label: "Book", URL: "https://example.com"},
		}}},
		AllowedMentions: discord.AllowedMentions{Parse: []string{}, Roles: []string{"300000000000000002"}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if posted.ID == "" || posted.ChannelID != teaRooms {
		t.Fatalf("posted = %+v", posted)
	}
	m, _ := f.Message(posted.ID)
	if m.Embeds[0].Title != "Jasmine Room" || len(m.Components) != 1 || m.AllowedMentions.Roles[0] != "300000000000000002" {
		t.Fatalf("stored = %s %+v", m, m.AllowedMentions)
	}

	// An edit replaces the whole message: dropping the buttons takes them off.
	if err := c.EditMessage(ctx, teaRooms, posted.ID, discord.Message{
		Embeds: []discord.Embed{{Title: "Jasmine Room (closed)"}},
	}); err != nil {
		t.Fatal(err)
	}
	m, _ = f.Message(posted.ID)
	if m.Embeds[0].Title != "Jasmine Room (closed)" || len(m.Components) != 0 || m.Content != "" || m.Edits != 1 {
		t.Fatalf("after edit = %s, components %d", m, len(m.Components))
	}

	if err := c.DeleteMessage(ctx, teaRooms, posted.ID, false); err != nil {
		t.Fatal(err)
	}
	if len(f.Messages()) != 0 {
		t.Fatalf("still up: %v", f.Messages())
	}
	if err := c.DeleteMessage(ctx, teaRooms, posted.ID, false); !errors.Is(err, discord.ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
	if err := c.EditMessage(ctx, teaRooms, posted.ID, discord.Message{Content: "x"}); !errors.Is(err, discord.ErrNotFound) {
		t.Fatalf("edit of a deleted message = %v, want ErrNotFound", err)
	}
}

func TestEditOfMessageDeletedInDiscord(t *testing.T) {
	f, c := fake(t)
	ctx := context.Background()
	posted, err := c.SendMessage(ctx, teaRooms, discord.Message{Content: "hi"}, false)
	if err != nil {
		t.Fatal(err)
	}
	f.DeleteInDiscord(posted.ID)
	if err := c.EditMessage(ctx, teaRooms, posted.ID, discord.Message{Content: "again"}); !errors.Is(err, discord.ErrNotFound) {
		t.Fatalf("edit = %v, want ErrNotFound", err)
	}
}

func TestNonceMakesARetrySafe(t *testing.T) {
	f, c := fake(t)
	ctx := context.Background()
	msg := discord.Message{Content: "Tea House is open!", Nonce: "a12-1792195200"}
	first, err := c.SendMessage(ctx, announcements, msg, false)
	if err != nil {
		t.Fatal(err)
	}
	again, err := c.SendMessage(ctx, announcements, msg, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != again.ID || len(f.Messages()) != 1 {
		t.Fatalf("a repeated nonce posted twice: %s vs %s, %d messages", first.ID, again.ID, len(f.Messages()))
	}
	if m, _ := f.Message(first.ID); m.Nonce != "a12-1792195200" {
		t.Fatalf("nonce not sent: %q", m.Nonce)
	}
}

func TestNonceTooLongIsRefusedLocally(t *testing.T) {
	f, c := fake(t)
	_, err := c.SendMessage(context.Background(), teaRooms, discord.Message{Content: "x", Nonce: strings.Repeat("n", 26)}, false)
	if err == nil {
		t.Fatal("a 26-character nonce was accepted")
	}
	if len(f.Calls()) != 0 {
		t.Fatalf("a request went out anyway: %v", f.Calls())
	}
}

func TestWrongTokenIsUnauthorized(t *testing.T) {
	f := discordtest.New()
	srv := httptest.NewServer(f.Handler())
	defer srv.Close()
	c := discord.New("not-the-token", srv.URL)
	if _, err := c.Me(context.Background()); !errors.Is(err, discord.ErrUnauthorized) {
		t.Fatalf("Me = %v, want ErrUnauthorized", err)
	}
}

func TestShortRateLimitIsWaitedOutOnce(t *testing.T) {
	f, c := fake(t)
	f.RateLimit("POST /channels/", 0.05)
	if _, err := c.SendMessage(context.Background(), teaRooms, discord.Message{Content: "x"}, false); err != nil {
		t.Fatalf("send after one short limit: %v", err)
	}
	posts := 0
	for _, call := range f.Calls() {
		if strings.HasPrefix(call, "POST ") {
			posts++
		}
	}
	if posts != 2 {
		t.Fatalf("POSTs = %d, want 2 (the limited one and the retry)", posts)
	}

	// A second limit in a row is more than an ordinary call waits for.
	f.RateLimit("POST /channels/", 0.05)
	f.RateLimit("POST /channels/", 0.05)
	if _, err := c.SendMessage(context.Background(), teaRooms, discord.Message{Content: "y"}, false); !errors.Is(err, discord.ErrRateLimited) {
		t.Fatalf("send after two limits = %v, want ErrRateLimited", err)
	}
}

func TestPatientCallWaitsOutSeveralLimits(t *testing.T) {
	f, c := fake(t)
	for range 3 {
		f.RateLimit("POST /channels/", 0.02)
	}
	if _, err := c.SendMessage(context.Background(), teaRooms, discord.Message{Content: "x"}, true); err != nil {
		t.Fatalf("patient send: %v", err)
	}
}

func TestLongRateLimitIsNotWaited(t *testing.T) {
	f, c := fake(t)
	f.RateLimit("POST /channels/", 30)
	start := time.Now()
	_, err := c.SendMessage(context.Background(), teaRooms, discord.Message{Content: "x"}, false)
	if !errors.Is(err, discord.ErrRateLimited) {
		t.Fatalf("send = %v, want ErrRateLimited", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("waited for a limit longer than the call accepts")
	}
}

func TestExhaustedBucketPacesTheNextCallOnThatChannel(t *testing.T) {
	f, c := fake(t)
	ctx := context.Background()
	f.ExhaustNext(200 * time.Millisecond)
	if _, err := c.SendMessage(ctx, teaRooms, discord.Message{Content: "1"}, false); err != nil {
		t.Fatal(err)
	}
	// Another channel has its own bucket and goes at once.
	start := time.Now()
	if _, err := c.SendMessage(ctx, announcements, discord.Message{Content: "2"}, false); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 150*time.Millisecond {
		t.Fatal("a different channel waited for another channel's bucket")
	}
	start = time.Now()
	if _, err := c.SendMessage(ctx, teaRooms, discord.Message{Content: "3"}, false); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 100*time.Millisecond {
		t.Fatalf("waited %s on an exhausted bucket, want about 200ms", waited)
	}
}

func TestRefusalCarriesDiscordsReason(t *testing.T) {
	f, c := fake(t)
	f.Fail("POST /channels/", http.StatusBadRequest,
		`{"code":50035,"message":"Invalid Form Body","errors":{"embeds":{"0":{"description":{"_errors":[{"code":"BASE_TYPE_MAX_LENGTH","message":"Must be 4096 or fewer in length."}]}}}}}`)
	_, err := c.SendMessage(context.Background(), teaRooms, discord.Message{Content: "x"}, false)
	if !errors.Is(err, discord.ErrRejected) {
		t.Fatalf("err = %v, want ErrRejected", err)
	}
	if got := discord.Said(err); got != "embeds.0.description: Must be 4096 or fewer in length." {
		t.Fatalf("Said = %q", got)
	}
}

func TestForbiddenAndUnknownChannel(t *testing.T) {
	f, c := fake(t)
	ctx := context.Background()
	f.Fail("POST /channels/", http.StatusForbidden, `{"message":"Missing Permissions","code":50013}`)
	if _, err := c.SendMessage(ctx, teaRooms, discord.Message{Content: "x"}, false); !errors.Is(err, discord.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	if _, err := c.SendMessage(ctx, "209999999999999999", discord.Message{Content: "x"}, false); !errors.Is(err, discord.ErrNotFound) {
		t.Fatalf("unknown channel = %v, want ErrNotFound", err)
	}
}

func TestTransportErrorDoesNotNameThePath(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // nothing listens there now
	c := discord.New("tok", srv.URL)
	_, err := c.SendMessage(context.Background(), teaRooms, discord.Message{Content: "x"}, false)
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if strings.Contains(err.Error(), teaRooms) || strings.Contains(err.Error(), "/channels/") {
		t.Fatalf("error names the path: %v", err)
	}
}

func TestServerReads(t *testing.T) {
	f, c := fake(t)
	ctx := context.Background()
	app, err := c.Application(ctx)
	if err != nil || app.ID != discordtest.AppID {
		t.Fatalf("Application = %+v, %v", app, err)
	}
	me, err := c.Me(ctx)
	if err != nil || me.ID != discordtest.BotUserID || !me.Bot {
		t.Fatalf("Me = %+v, %v", me, err)
	}
	g, err := c.Guild(ctx, discordtest.GuildID)
	if err != nil || g.Name != "Senpan Tea House" {
		t.Fatalf("Guild = %+v, %v", g, err)
	}
	if _, err := c.Guild(ctx, "123456789012345678"); !errors.Is(err, discord.ErrNotFound) {
		t.Fatalf("a server the bot is not in = %v, want ErrNotFound", err)
	}
	channels, err := c.GuildChannels(ctx, discordtest.GuildID)
	if err != nil || len(channels) != len(f.Channels) {
		t.Fatalf("GuildChannels = %d, %v", len(channels), err)
	}
	roles, err := c.GuildRoles(ctx, discordtest.GuildID)
	if err != nil || len(roles) != len(f.Roles) {
		t.Fatalf("GuildRoles = %d, %v", len(roles), err)
	}
	member, err := c.Member(ctx, discordtest.GuildID, discordtest.BotUserID)
	if err != nil {
		t.Fatal(err)
	}
	if !discord.HasAdministrator(discordtest.GuildID, member.Roles, roles) {
		t.Fatal("the bot's role grants Administrator")
	}
	if discord.HasAdministrator(discordtest.GuildID, nil, roles) {
		t.Fatal("without the bot's role nothing grants Administrator")
	}
}

func TestAvatarAndIconURLs(t *testing.T) {
	if got := (discord.User{ID: "1", Avatar: "abc"}).AvatarURL(); got != "https://cdn.discordapp.com/avatars/1/abc.png?size=128" {
		t.Fatalf("AvatarURL = %q", got)
	}
	if got := (discord.User{ID: "1"}).AvatarURL(); got != "" {
		t.Fatalf("no avatar = %q", got)
	}
	if got := (discord.Guild{ID: "9", Icon: "def"}).IconURL(); got != "https://cdn.discordapp.com/icons/9/def.png?size=128" {
		t.Fatalf("IconURL = %q", got)
	}
	if got := (discord.User{Username: "senpan", GlobalName: "Senpan"}).Named(); got != "Senpan" {
		t.Fatalf("Named = %q", got)
	}
}
