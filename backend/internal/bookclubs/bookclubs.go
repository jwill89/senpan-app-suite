// Package bookclubs holds the book clubs' registry and the rules that need no
// database or network: each club's settings key, and which AniList addresses the
// server may look titles up at.
package bookclubs

import (
	"net/url"
	"strings"
)

// Club is one book club. Adding one here gives it its reading lists and its own
// Discord webhook (Settings -> Integrations).
type Club struct {
	Slug string
	Name string
	// CommentsLabel names the curator's comments on each title in the published
	// Discord embed (e.g. "Yao's Comments").
	CommentsLabel string
}

// Clubs is the registry, in the order the admin pages show them. The SPA keeps the
// same list (frontend/src/lib/constants.ts, BOOK_CLUBS).
var Clubs = []Club{
	{Slug: "yaoi", Name: "Yaoi Book Club", CommentsLabel: "Yao's Comments"},
	{Slug: "yuri", Name: "Yuri Book Club", CommentsLabel: "Drani's Comments"},
}

// Slugs is every club's slug, in registry order.
func Slugs() []string {
	slugs := make([]string, len(Clubs))
	for i, c := range Clubs {
		slugs[i] = c.Slug
	}
	return slugs
}

// CommentsLabel is a club's label for its curator's comments, or "Comments" for a
// slug the registry doesn't have.
func CommentsLabel(slug string) string {
	for _, c := range Clubs {
		if c.Slug == slug {
			return c.CommentsLabel
		}
	}
	return "Comments"
}

// WebhookSettingKey is the settings key holding a club's reading-list Discord
// webhook (e.g. "discord_webhook_url_yaoi").
func WebhookSettingKey(slug string) string {
	return "discord_webhook_url_" + slug
}

// DefaultAniListURL is the AniList GraphQL endpoint used when the setting is empty.
// AniList needs no key for public reads.
const DefaultAniListURL = "https://graphql.anilist.co"

// IsAllowedAniListURL reports whether raw may be the AniList endpoint: https, on
// anilist.co or a subdomain. The setting drives the server's outbound lookup, so it
// is held to this, as is every redirect the lookup follows, to keep it from being
// aimed at an internal service or a cloud metadata address.
func IsAllowedAniListURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "anilist.co" || strings.HasSuffix(host, ".anilist.co")
}
