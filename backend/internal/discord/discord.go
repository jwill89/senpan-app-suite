// Package discord holds what Senpan knows about Discord apart from sending to it.
package discord

import (
	"net/url"
	"strings"
)

// IsWebhookURL reports whether raw is a Discord webhook URL: https, on an official
// Discord host, under /api/webhooks/. Every webhook an admin enters (announcement
// types, affiliates, tea rooms, ema windows, the book clubs) is held to it, so a
// saved value can't point the server's outbound posts at another host, such as an
// internal one.
func IsWebhookURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "discord.com", "discordapp.com", "ptb.discord.com", "canary.discord.com":
		return strings.HasPrefix(u.Path, "/api/webhooks/")
	}
	return false
}
