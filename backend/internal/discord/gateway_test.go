package discord_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"app-suite/internal/discord"
	"app-suite/internal/discord/discordtest"
)

// gateway starts a fake Discord and a gateway for it, connecting to the fake's gateway the
// way production does (asking GET /gateway/bot where it is).
func gateway(t *testing.T, token string) (*discordtest.Server, *discord.Gateway) {
	t.Helper()
	f := discordtest.New()
	srv := httptest.NewServer(f.Handler())
	t.Cleanup(srv.Close)
	g := discord.NewGateway(discord.New(token, srv.URL))
	g.Intents = discord.IntentGuilds
	return f, g
}

// run starts the gateway and returns a stop function that waits for it to end.
func run(t *testing.T, g *discord.Gateway) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		g.Run(ctx)
		close(done)
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("the gateway did not stop")
		}
	}
}

func waitFor(t *testing.T, events <-chan string, want string) {
	t.Helper()
	select {
	case got := <-events:
		if got != want {
			t.Fatalf("event %s, want %s", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("no %s", want)
	}
}

func TestGatewayIdentifiesThenResumes(t *testing.T) {
	f, g := gateway(t, discordtest.Token)
	f.ReconnectAfter(2)
	events := make(chan string, 8)
	g.Handle = func(event string, _ json.RawMessage) { events <- event }
	stop := run(t, g)

	waitFor(t, events, "READY")
	if st := g.Status(); !st.Online || st.ConnectedAt == "" {
		t.Fatalf("not online after READY: %+v", st)
	}
	if user, app := g.Self(); user != discordtest.BotUserID || app != discordtest.AppID {
		t.Fatalf("Self = %s, %s", user, app)
	}
	// After two heartbeats the fake asks for a reconnect; the session resumes, not re-identifies.
	waitFor(t, events, "RESUMED")
	identifies, resumes, _ := f.Gateway()
	if identifies != 1 || resumes != 1 {
		t.Fatalf("identifies %d, resumes %d; want 1 and 1", identifies, resumes)
	}

	stop()
	if g.Status().Online {
		t.Fatal("still online after stopping")
	}
}

func TestGatewayPresenceIsToldOnceReady(t *testing.T) {
	f, g := gateway(t, discordtest.Token)
	var mu sync.Mutex
	showing := discord.Presence{Activity: "custom", Text: "Pouring tea"}
	g.Presence = func() discord.Presence {
		mu.Lock()
		defer mu.Unlock()
		return showing
	}
	ready := make(chan string, 4)
	g.Handle = func(event string, _ json.RawMessage) { ready <- event }
	// Asked for before any session exists: told once one is up.
	g.RefreshPresence()
	stop := run(t, g)
	defer stop()
	waitFor(t, ready, "READY")

	said := func(n int) []string {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			_, _, got := f.Gateway()
			if len(got) >= n {
				return got
			}
			if time.Now().After(deadline) {
				t.Fatalf("told %d presences, want %d", len(got), n)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	first := `{"activities":[{"name":"Custom Status","state":"Pouring tea","type":4}],"afk":false,"since":null,"status":"online"}`
	if got := said(2); got[0] != first || got[1] != first {
		t.Fatalf("identify and the waiting update: %v", got)
	}

	mu.Lock()
	showing = discord.Presence{Activity: "watching", Text: "the tea house"}
	mu.Unlock()
	g.RefreshPresence()
	if got := said(3); got[2] != `{"activities":[{"name":"the tea house","type":3}],"afk":false,"since":null,"status":"online"}` {
		t.Fatalf("the update: %s", got[2])
	}
}

func TestGatewayWithABadTokenReportsTheError(t *testing.T) {
	_, g := gateway(t, "wrong-token")
	stop := run(t, g)
	defer stop()
	deadline := time.Now().Add(3 * time.Second)
	for g.Status().LastError == "" {
		if time.Now().After(deadline) {
			t.Fatal("no error reported for a refused token")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// GET /gateway/bot is refused first, with the same bad token.
	if st := g.Status(); st.Online || !strings.Contains(st.LastError, "unauthorized") {
		t.Fatalf("status = %+v", st)
	}
}

func TestGatewayWithoutClientDoesNothing(t *testing.T) {
	g := discord.NewGateway(nil)
	if g.Status().Online {
		t.Fatal("a gateway without a client is offline")
	}
	g.Run(context.Background()) // returns at once
	g.RefreshPresence()         // and so does this
	var none *discord.Gateway
	none.RefreshPresence()
	if user, app := none.Self(); user != "" || app != "" {
		t.Fatal("a nil gateway knows no one")
	}
}
