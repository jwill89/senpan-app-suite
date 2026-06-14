package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// This file holds the generic Discord embed plumbing shared by every feature
// that posts to a webhook (reading lists, book-club events, announcements):
// the embed schema types, a fluent builder for assembling one in a customizable
// way, the colour helper, and the HTTP post. Feature code builds an embed with
// newEmbed()… and sends it with postDiscordEmbed; new embed shapes only need a
// new builder chain, not new transport code.

// accentColor is the brand accent (pink) used as the default embed colour.
const accentColor = 0xE53170

// Discord's per-field length caps; truncateRunes enforces them so callers can
// pass raw content without pre-trimming.
const (
	embedTitleMax       = 256
	embedDescriptionMax = 4096
	embedFieldValueMax  = 1024
	maxEmbedFields      = 25 // Discord's hard cap on fields per embed
)

// embedNoHeading is a zero-width space (U+200B) used as a field name when a field
// should render with no visible heading (Discord requires a non-empty field name).
var embedNoHeading = string(rune(0x200B))

// discordEmbedField / discordEmbed / discordWebhookPayload model the subset of
// the Discord webhook embed schema we send.
type discordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type discordEmbedThumbnail struct {
	URL string `json:"url"`
}

// discordEmbedImage is the large, full-width image rendered at the bottom of an
// embed (distinct from the small top-right thumbnail).
type discordEmbedImage struct {
	URL string `json:"url"`
}

// discordEmbedFooter is the small footer line shown at the bottom of an embed.
type discordEmbedFooter struct {
	Text string `json:"text"`
}

type discordEmbed struct {
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	Color       int                    `json:"color,omitempty"`
	Fields      []discordEmbedField    `json:"fields,omitempty"`
	Thumbnail   *discordEmbedThumbnail `json:"thumbnail,omitempty"`
	Image       *discordEmbedImage     `json:"image,omitempty"`
	Footer      *discordEmbedFooter    `json:"footer,omitempty"`
}

type discordWebhookPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

// embedBuilder assembles a discordEmbed fluently. Empty values are skipped and
// text is truncated to Discord's limits, so callers can pass raw content without
// pre-checking. New embed shapes extend this with more chainable setters.
type embedBuilder struct {
	embed discordEmbed
}

// newEmbed starts a builder pre-seeded with the brand accent colour.
func newEmbed() *embedBuilder {
	return &embedBuilder{embed: discordEmbed{Color: accentColor}}
}

func (b *embedBuilder) title(s string) *embedBuilder {
	b.embed.Title = truncateRunes(strings.TrimSpace(s), embedTitleMax)
	return b
}

func (b *embedBuilder) description(s string) *embedBuilder {
	b.embed.Description = truncateRunes(strings.TrimSpace(s), embedDescriptionMax)
	return b
}

func (b *embedBuilder) color(c int) *embedBuilder {
	b.embed.Color = c
	return b
}

// colorHex sets the colour from a "#rrggbb" (or "rrggbb") string, falling back
// to the brand accent when empty or invalid.
func (b *embedBuilder) colorHex(hex string) *embedBuilder {
	b.embed.Color = colorFromHex(hex, accentColor)
	return b
}

// field appends a field. Fields render in call order, each with its own inline
// flag (Discord lays inline fields side by side, up to three per row, and breaks
// to a new row at the next non-inline field). Blank values are skipped, and once
// the embed reaches Discord's 25-field cap further fields are ignored, so callers
// can append freely without bookkeeping.
func (b *embedBuilder) field(name, value string, inline bool) *embedBuilder {
	value = strings.TrimSpace(value)
	if value == "" || len(b.embed.Fields) >= maxEmbedFields {
		return b
	}
	b.embed.Fields = append(b.embed.Fields, discordEmbedField{
		Name: name, Value: truncateRunes(value, embedFieldValueMax), Inline: inline,
	})
	return b
}

// thumbnail sets the small top-right image (only for absolute http(s) URLs).
func (b *embedBuilder) thumbnail(url string) *embedBuilder {
	if isHTTPURL(url) {
		b.embed.Thumbnail = &discordEmbedThumbnail{URL: url}
	}
	return b
}

// image sets the large bottom image (only for absolute http(s) URLs).
func (b *embedBuilder) image(url string) *embedBuilder {
	if isHTTPURL(url) {
		b.embed.Image = &discordEmbedImage{URL: url}
	}
	return b
}

// footer sets the footer line, skipping it when blank.
func (b *embedBuilder) footer(text string) *embedBuilder {
	if strings.TrimSpace(text) != "" {
		b.embed.Footer = &discordEmbedFooter{Text: text}
	}
	return b
}

// build returns the assembled embed.
func (b *embedBuilder) build() discordEmbed {
	return b.embed
}

// colorFromHex parses a "#rrggbb" (or "rrggbb") colour into the Discord integer
// colour value, returning def when the input is empty or not a valid 24-bit hex.
func colorFromHex(hex string, def int) int {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if hex == "" {
		return def
	}
	v, err := strconv.ParseInt(hex, 16, 32)
	if err != nil || v < 0 || v > 0xFFFFFF {
		return def
	}
	return int(v)
}

// postDiscordEmbed sends a single embed to the webhook URL.
func postDiscordEmbed(webhookURL string, embed discordEmbed) error {
	payload, err := json.Marshal(discordWebhookPayload{Embeds: []discordEmbed{embed}})
	if err != nil {
		return fmt.Errorf("encode embed")
	}
	resp, err := bookclubHTTPClient.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("request failed")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// isHTTPURL reports whether u is an http(s) URL (Discord requires absolute URLs
// for embed thumbnails and images).
func isHTTPURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// truncateRunes caps a string at n runes, appending an ellipsis when trimmed,
// so embeds never exceed Discord's per-field limits.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return string(runes[:n])
	}
	return string(runes[:n-1]) + "…"
}
