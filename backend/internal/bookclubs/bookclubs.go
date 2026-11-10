// Package bookclubs holds the book clubs' registry and the rules that need no
// database or network: each club's settings key, which AniList addresses the
// server may look titles up at, how a reading-list item is cleaned, and how one is
// rendered as a Discord embed.
package bookclubs

import (
	"net/url"
	"slices"
	"strings"

	"app-suite/internal/discord"
)

// Club is one book club. Adding one here gives it its reading lists and its own
// Discord webhook.
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

// Exists reports whether slug is a registered club.
func Exists(slug string) bool {
	return slices.Contains(Slugs(), slug)
}

// Source is a named external link on a reading-list item.
type Source struct {
	Title string
	URL   string
}

// Item is what an embed shows of a reading-list item.
type Item struct {
	CoverImage string
	Title      string
	Summary    string
	Format     string
	Genres     string
	Tropes     string
	Chapters   string
	Comments   string
	Sources    []Source
}

// SanitizeSources trims each source and drops those with no URL, so blank rows from
// a form aren't kept. A source needs at least a URL.
func SanitizeSources(in []Source) []Source {
	out := make([]Source, 0, len(in))
	for _, s := range in {
		if u := strings.TrimSpace(s.URL); u != "" {
			out = append(out, Source{Title: strings.TrimSpace(s.Title), URL: u})
		}
	}
	return out
}

// ItemEmbed renders an item as a Discord embed for the club. The cover is the large
// full-width image rather than the small thumbnail, and the curator's comments are
// labeled the club's way.
func ItemEmbed(it Item, slug string) discord.Embed {
	b := discord.NewEmbed().
		Title(it.Title).
		Description(it.Summary).
		Image(it.CoverImage).
		Field("Format", it.Format, true).
		Field("Chapters", it.Chapters, true).
		Field("Genres", it.Genres, false).
		Field("Tropes", it.Tropes, false).
		Field(CommentsLabel(slug), it.Comments, false)

	if len(it.Sources) > 0 {
		links := make([]string, 0, len(it.Sources))
		for _, src := range it.Sources {
			if strings.TrimSpace(src.URL) == "" {
				continue
			}
			label := strings.TrimSpace(src.Title)
			if label == "" {
				label = src.URL
			}
			links = append(links, "["+label+"]("+src.URL+")")
		}
		b.Field("Sources", strings.Join(links, "\n"), false)
	}
	return b.Build()
}
