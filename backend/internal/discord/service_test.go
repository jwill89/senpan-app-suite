package discord_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"app-suite/internal/discord"
	"app-suite/internal/discord/discordtest"
)

func service(t *testing.T) (*discordtest.Server, *discord.Service) {
	t.Helper()
	f := discordtest.New()
	srv := httptest.NewServer(f.Handler())
	t.Cleanup(srv.Close)
	return f, discord.NewService(srv.URL, "")
}

func countCalls(f *discordtest.Server, prefix string) int {
	n := 0
	for _, c := range f.Calls() {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestServiceIsOffWithoutAToken(t *testing.T) {
	_, s := service(t)
	if s.Enabled() || s.Client() != nil {
		t.Fatal("no token, but the service is on")
	}
	if _, err := s.Directory(context.Background()); !errors.Is(err, discord.ErrOff) {
		t.Fatalf("Directory = %v, want ErrOff", err)
	}
	if s.GatewayStatus().Online {
		t.Fatal("offline service reports online")
	}
}

func TestServiceDirectoryIsCachedAndInvalidated(t *testing.T) {
	f, s := service(t)
	s.Configure(discord.Config{Token: discordtest.Token, GuildID: discordtest.GuildID})
	ctx := context.Background()
	d, err := s.Directory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !d.InGuild || d.Guild.Name != "Senpan Tea House" || len(d.Channels) == 0 || len(d.Roles) == 0 {
		t.Fatalf("directory = %+v", d)
	}
	if !d.IsAdmin() || d.Bot.ID != discordtest.BotUserID || d.Application.ID != discordtest.AppID {
		t.Fatalf("identity/admin wrong: %+v", d)
	}
	reads := countCalls(f, "GET /guilds/")
	if _, err := s.Directory(ctx); err != nil {
		t.Fatal(err)
	}
	if got := countCalls(f, "GET /guilds/"); got != reads {
		t.Fatalf("a cached read asked Discord again (%d -> %d calls)", reads, got)
	}
	s.Invalidate()
	if _, err := s.Directory(ctx); err != nil {
		t.Fatal(err)
	}
	if got := countCalls(f, "GET /guilds/"); got == reads {
		t.Fatal("an invalidated cache was not refetched")
	}
}

func TestServiceDirectoryWhenTheBotIsNotInTheServer(t *testing.T) {
	_, s := service(t)
	s.Configure(discord.Config{Token: discordtest.Token, GuildID: "123456789012345678"})
	d, err := s.Directory(context.Background())
	if err != nil {
		t.Fatalf("not being in the server is an answer, not an error: %v", err)
	}
	if d.InGuild || d.IsAdmin() || len(d.Channels) != 0 || d.Bot.ID == "" {
		t.Fatalf("directory = %+v", d)
	}
}

func TestServiceDirectoryWithABadToken(t *testing.T) {
	_, s := service(t)
	s.Configure(discord.Config{Token: "wrong", GuildID: discordtest.GuildID})
	if _, err := s.Directory(context.Background()); !errors.Is(err, discord.ErrUnauthorized) {
		t.Fatalf("Directory = %v, want ErrUnauthorized", err)
	}
}

func TestServiceRestartsTheGatewayWhenTheTokenChanges(t *testing.T) {
	f, s := service(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	// No token yet: nothing connects.
	time.Sleep(50 * time.Millisecond)
	if identifies, _, _ := f.Gateway(); identifies != 0 {
		t.Fatalf("connected without a token (%d identifies)", identifies)
	}

	s.Configure(discord.Config{Token: discordtest.Token, GuildID: discordtest.GuildID})
	eventually(t, "online", func() bool { return s.GatewayStatus().Online })

	// A new token: the old session is dropped and a fresh one identifies with it.
	f.Lock()
	f.Token = "second-token"
	f.Unlock()
	s.Configure(discord.Config{Token: "second-token", GuildID: discordtest.GuildID})
	eventually(t, "a second identify", func() bool {
		identifies, _, _ := f.Gateway()
		return identifies == 2 && s.GatewayStatus().Online
	})

	// Token removed: offline, and the service is off.
	s.Configure(discord.Config{GuildID: discordtest.GuildID})
	if s.GatewayStatus().Online || s.Enabled() {
		t.Fatal("still on after the token was removed")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestServiceTellsAPresenceChangeToTheOpenSession(t *testing.T) {
	f, s := service(t)
	s.Configure(discord.Config{Token: discordtest.Token, GuildID: discordtest.GuildID,
		Presence: discord.Presence{Activity: "custom", Text: "Pouring tea"}})
	go s.Run(t.Context())
	eventually(t, "online", func() bool { return s.GatewayStatus().Online })

	s.Configure(discord.Config{Token: discordtest.Token, GuildID: discordtest.GuildID,
		Presence: discord.Presence{Activity: "custom", Text: "Closed for the night"}})
	eventually(t, "the presence update", func() bool {
		_, _, presences := f.Gateway()
		return len(presences) >= 2 && strings.Contains(presences[len(presences)-1], "Closed for the night")
	})
	if identifies, _, _ := f.Gateway(); identifies != 1 {
		t.Fatalf("a presence change reconnected the session (%d identifies)", identifies)
	}
}
