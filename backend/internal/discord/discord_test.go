package discord

import "testing"

func TestIsWebhookURL(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://discord.com/api/webhooks/1/abc":        true,
		"https://canary.discord.com/api/webhooks/1/abc": true,
		"https://DiscordApp.com/api/webhooks/1/abc":     true,
		"http://discord.com/api/webhooks/1/abc":         false,
		"https://discord.com/channels/1/2":              false,
		"https://discord.com.evil.com/api/webhooks/1":   false,
		"https://127.0.0.1/api/webhooks/1":              false,
		"":                                              false,
	} {
		if got := IsWebhookURL(raw); got != want {
			t.Errorf("IsWebhookURL(%q) = %v, want %v", raw, got, want)
		}
	}
}
