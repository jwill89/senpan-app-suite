package discord_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"app-suite/internal/discord"
	"app-suite/internal/discord/discordtest"
)

func TestLookupWebhook(t *testing.T) {
	f := discordtest.New()
	srv := httptest.NewServer(f.Handler())
	defer srv.Close()
	f.AddWebhook("111111111111111111", discordtest.Webhook{Token: "hook-secret-value", Name: "Senpan Tea House",
		GuildID: discordtest.GuildID, ChannelID: "200000000000000002"})
	ctx := context.Background()

	w, err := discord.LookupWebhook(ctx, srv.URL, "https://discord.com/api/webhooks/111111111111111111/hook-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	if w.GuildID != discordtest.GuildID || w.ChannelID != "200000000000000002" || w.Name != "Senpan Tea House" {
		t.Fatalf("webhook = %+v", w)
	}
	// The lookup carries no bot token.
	for _, c := range f.Calls() {
		if strings.Contains(c, "hook-secret-value") {
			t.Fatalf("a call log names the webhook token: %s", c)
		}
	}

	for name, url := range map[string]string{
		"deleted webhook":     "https://discord.com/api/webhooks/999999999999999999/tok",
		"reset webhook token": "https://discordapp.com/api/webhooks/111111111111111111/old",
	} {
		if _, err := discord.LookupWebhook(ctx, srv.URL, url); !errors.Is(err, discord.ErrNotFound) {
			t.Errorf("%s = %v, want ErrNotFound", name, err)
		}
	}
	if _, err := discord.LookupWebhook(ctx, srv.URL, "https://example.com/hook"); err == nil {
		t.Error("a non-Discord URL was looked up")
	}

	f.Fail("GET /webhooks/", http.StatusBadGateway, "")
	_, err = discord.LookupWebhook(ctx, srv.URL, "https://discord.com/api/webhooks/111111111111111111/hook-secret-value")
	if err == nil || errors.Is(err, discord.ErrNotFound) {
		t.Fatalf("a server error = %v, want a retryable error", err)
	}
	if strings.Contains(err.Error(), "hook-secret-value") {
		t.Fatalf("error names the token: %v", err)
	}
}

func TestDeleteWebhook(t *testing.T) {
	f, c := fake(t)
	f.AddWebhook("111111111111111111", discordtest.Webhook{Token: "tok", GuildID: discordtest.GuildID, ChannelID: "200000000000000002"})
	f.AddWebhook("222222222222222222", discordtest.Webhook{Token: "tok", GuildID: "123456789012345678", ChannelID: "1"})
	ctx := context.Background()
	if err := c.DeleteWebhook(ctx, "111111111111111111"); err != nil {
		t.Fatal(err)
	}
	if !f.WebhookDeleted("111111111111111111") {
		t.Fatal("not deleted")
	}
	if err := c.DeleteWebhook(ctx, "111111111111111111"); !errors.Is(err, discord.ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
	if err := c.DeleteWebhook(ctx, "222222222222222222"); !errors.Is(err, discord.ErrForbidden) {
		t.Fatalf("another server's webhook = %v, want ErrForbidden", err)
	}
}
