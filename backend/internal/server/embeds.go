package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// errWebhookAmbiguous marks a transport-level failure (timeout, connection reset)
// where the request may still have reached Discord. The scheduler must NOT retry
// these: if the message did in fact arrive, retrying would double-post it. An
// HTTP error *status* (4xx/5xx, incl. 429) is NOT ambiguous - Discord received
// the request and declined it, so the message was not delivered and is safe to
// retry.
var errWebhookAmbiguous = errors.New("discord webhook delivery ambiguous (transport failure)")

// This file holds the generic Discord embed plumbing shared by every feature
// that posts to a webhook (reading lists, book-club events, announcements):
// the embed schema types, a fluent builder for assembling one in a customizable
// way, the colour helper, and the HTTP post. Feature code builds an embed with
// newEmbed()... and sends it with postDiscordEmbed; new embed shapes only need a
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
	// Content is the plain message text above the embed - used to carry a role
	// mention (@everyone or <@&id>), since mentions inside an embed don't notify.
	Content         string                  `json:"content,omitempty"`
	Embeds          []discordEmbed          `json:"embeds"`
	Components      []discordComponent      `json:"components,omitempty"`
	AllowedMentions *discordAllowedMentions `json:"allowed_mentions,omitempty"`
}

// discordAllowedMentions whitelists which mentions in Content actually ping.
// Without it Discord may silently suppress webhook mentions; we set it so the
// chosen role (or @everyone) reliably notifies. Parse "everyone" enables the
// @everyone/@here ping; Roles lists explicit role IDs allowed to ping.
type discordAllowedMentions struct {
	Parse []string `json:"parse"`
	Roles []string `json:"roles,omitempty"`
}

// Discord message-component type/style constants for the subset we emit: an
// action row (type 1) holding link buttons (type 2, style 5 = URL button).
const (
	componentActionRow = 1
	componentButton    = 2
	buttonStyleLink    = 5
	maxButtonsPerRow   = 5  // Discord's cap on buttons in one action row
	buttonLabelMax     = 80 // Discord's per-button label length cap
)

// discordEmoji is the emoji shown on a button - either a unicode emoji (Name holds
// the character) or a custom guild emoji (Name + numeric ID, Animated for "a:").
type discordEmoji struct {
	Name     string `json:"name,omitempty"`
	ID       string `json:"id,omitempty"`
	Animated bool   `json:"animated,omitempty"`
}

// discordComponent models the message-component subset we send: an action row
// (Components populated) or a link button (Style/Label/URL, optional Emoji).
type discordComponent struct {
	Type       int                `json:"type"`
	Style      int                `json:"style,omitempty"`
	Label      string             `json:"label,omitempty"`
	URL        string             `json:"url,omitempty"`
	Emoji      *discordEmoji      `json:"emoji,omitempty"`
	Components []discordComponent `json:"components,omitempty"`
}

// linkButtonRow builds a single action row of link buttons from (label, emoji, url)
// triples, skipping any with a blank label or non-http(s) URL and capping at
// Discord's five-per-row limit. Returns nil when no valid button remains, so the
// caller can omit Components entirely.
func linkButtonRow(buttons []struct{ Label, Emoji, URL string }) []discordComponent {
	row := make([]discordComponent, 0, maxButtonsPerRow)
	for _, b := range buttons {
		label := strings.TrimSpace(b.Label)
		url := strings.TrimSpace(b.URL)
		if label == "" || !isHTTPURL(url) {
			continue
		}
		btn := discordComponent{
			Type:  componentButton,
			Style: buttonStyleLink,
			Label: truncateRunes(label, buttonLabelMax),
			URL:   url,
		}
		if e := parseEmoji(b.Emoji); e != nil {
			btn.Emoji = e
		}
		row = append(row, btn)
		if len(row) >= maxButtonsPerRow {
			break
		}
	}
	if len(row) == 0 {
		return nil
	}
	return []discordComponent{{Type: componentActionRow, Components: row}}
}

// parseEmoji turns an emoji string into the Discord emoji object: a custom-emoji
// token "<:name:id>" / "<a:name:id>" becomes {name,id,animated}; anything else is
// treated as a unicode emoji ({name}). Returns nil for an empty/blank input.
func parseEmoji(s string) *discordEmoji {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		inner := strings.TrimSuffix(strings.TrimPrefix(s, "<"), ">")
		animated := strings.HasPrefix(inner, "a:")
		inner = strings.TrimPrefix(inner, "a")
		inner = strings.TrimPrefix(inner, ":")
		parts := strings.SplitN(inner, ":", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return &discordEmoji{Name: parts[0], ID: parts[1], Animated: animated}
		}
		return nil
	}
	return &discordEmoji{Name: s}
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

// thumbnail sets the small top-right image (only for absolute http(s) URLs, with
// any unsafe characters - e.g. a space in a filename - percent-encoded so Discord
// accepts the embed; see normalizeEmbedURL).
func (b *embedBuilder) thumbnail(rawURL string) *embedBuilder {
	if u, ok := normalizeEmbedURL(rawURL); ok {
		b.embed.Thumbnail = &discordEmbedThumbnail{URL: u}
	}
	return b
}

// image sets the large bottom image (only for absolute http(s) URLs, encoded as
// in thumbnail).
func (b *embedBuilder) image(rawURL string) *embedBuilder {
	if u, ok := normalizeEmbedURL(rawURL); ok {
		b.embed.Image = &discordEmbedImage{URL: u}
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

// webhookTarget labels a Discord post for the log: the feature that posted (a
// stable key to filter on) and which record went out. Every outbound post funnels
// through postDiscordWebhook, so carrying the label there is what makes a
// SUCCESSFUL post visible - failures were already logged (writeUpstreamError, the
// announcement scheduler), but a post that worked left no trace at all, which made
// "did that actually go out, and when?" unanswerable from the log alone.
type webhookTarget struct {
	// Kind is the posting feature: "announcement", "announcement_scheduled",
	// "bookclub_item", "affiliate" or "tea_room".
	Kind string
	// Name is the record's human identity - an announcement or item title, an
	// affiliate or tea-room name. Empty when the record has no useful name.
	Name string
}

// postDiscordEmbed sends a single embed to the webhook URL. The context bounds the
// request, so a caller shutdown or client disconnect cancels an in-flight POST.
func (s *Server) postDiscordEmbed(ctx context.Context, target webhookTarget, webhookURL string, embed discordEmbed) error {
	return s.postDiscordWebhook(ctx, target, webhookURL, discordWebhookPayload{Embeds: []discordEmbed{embed}})
}

// postDiscordWebhook sends a full webhook payload (embeds + optional components) to
// the webhook URL. When the payload carries components (our link-button rows),
// Discord ignores them unless the execute request opts in with
// `?with_components=true`; without it the message posts but the buttons silently
// vanish. Link buttons are non-interactive, so channel (non-application-owned)
// webhooks are allowed to send them once the flag is set.
func (s *Server) postDiscordWebhook(ctx context.Context, target webhookTarget, webhookURL string, payload discordWebhookPayload) error {
	// Dev safety, applied before anything leaves the process. A development server
	// is routinely pointed at a COPY OF THE LIVE DATABASE, which carries real
	// webhook URLs for every announcement type, book club, affiliate and tea room -
	// so an ordinary local run (or just the announcement scheduler ticking) posts to
	// production channels. Both guards sit at this funnel, the one place every
	// posting feature already goes through, so no feature can forget them and a
	// future one inherits them for free.
	if s.webhookDryRun {
		slog.Warn("discord post SUPPRESSED (dry run)", "kind", target.Kind, "name", target.Name,
			"embeds", len(payload.Embeds))
		return nil
	}
	if s.webhookOverride != "" {
		slog.Warn("discord post REDIRECTED to the override webhook", "kind", target.Kind,
			"name", target.Name)
		webhookURL = s.webhookOverride
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode embed")
	}
	if len(payload.Components) > 0 {
		webhookURL = withComponentsParam(webhookURL)
	}
	slog.Debug("discord webhook post", "kind", target.Kind, "name", target.Name,
		"embeds", len(payload.Embeds), "components", len(payload.Components), "bytes", len(body))
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %s", sanitizeWebhookErr(err))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := bookclubHTTPClient.Do(req)
	if err != nil {
		// Transport failure: the request may or may not have reached Discord, so
		// mark it ambiguous (callers must not blindly retry - see errWebhookAmbiguous).
		// sanitizeWebhookErr strips the webhook URL (its path carries the secret
		// token) that http.Client bakes into the *url.Error it returns.
		return fmt.Errorf("%w: %s", errWebhookAmbiguous, sanitizeWebhookErr(err))
	}
	defer resp.Body.Close()
	// Read (not discard) the response so a non-2xx can report Discord's own reason
	// - e.g. "Unknown Webhook" for a deleted webhook - instead of a bare status
	// code. The read also drains the body so the keep-alive connection is reusable.
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	slog.Debug("discord webhook response", "status", resp.StatusCode)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, discordErrorMessage(respBody))
	}
	// The one record that a post reached Discord. Failures are logged by the caller,
	// which is the layer that knows whether it will retry (scheduler) or surface a
	// 502 (an admin's manual send), so this deliberately covers the success path only
	// rather than double-reporting every error.
	slog.Info("discord post sent", "kind", target.Kind, "name", target.Name,
		"status", resp.StatusCode, "embeds", len(payload.Embeds),
		"duration_ms", time.Since(start).Milliseconds())
	return nil
}

// discordErrorMessage extracts a human-readable reason from a Discord webhook
// error response body. Discord returns a JSON envelope like
// {"message":"Unknown Webhook","code":10015}, so we surface "Unknown Webhook
// (code 10015)" when present - telling the admin the post failed because the
// webhook was deleted, malformed, rate-limited, etc. - and fall back to a trimmed
// raw snippet (or "no response body") otherwise. The body is Discord's response,
// so it never carries our webhook token.
func discordErrorMessage(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return "no response body"
	}
	var e struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		if e.Code != 0 {
			return fmt.Sprintf("%s (code %d)", e.Message, e.Code)
		}
		return e.Message
	}
	return truncateRunes(string(body), 300)
}

// sanitizeWebhookErr renders a transport error without exposing the webhook URL,
// whose path carries the secret webhook token. http.Client.Do wraps failures in a
// *url.Error whose Error() prints the full URL (url.Redacted masks only userinfo,
// not a token in the path), so we surface only the underlying cause - mirroring
// the AniList client's handling (see bookclubs.go). This keeps the token out of
// both the API response body and the server log.
func sanitizeWebhookErr(err error) string {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		if urlErr.Timeout() {
			return fmt.Sprintf("timed out after %s", bookclubHTTPClient.Timeout)
		}
		return fmt.Sprintf("could not connect (%v)", urlErr.Err)
	}
	return err.Error()
}

// withComponentsParam returns the webhook URL with `with_components=true` added to
// its query, so Discord honours the payload's `components` (button rows) instead of
// silently dropping them. A URL that can't be parsed is returned unchanged.
func withComponentsParam(webhookURL string) string {
	u, err := url.Parse(webhookURL)
	if err != nil {
		return webhookURL
	}
	q := u.Query()
	q.Set("with_components", "true")
	u.RawQuery = q.Encode()
	return u.String()
}

// isHTTPURL reports whether u is an http(s) URL (Discord requires absolute URLs
// for embed thumbnails and images).
func isHTTPURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// normalizeEmbedURL trims raw and, when it is an http(s) URL, returns it with any
// characters that aren't URL-safe percent-encoded - a space in an image filename
// becomes %20, "café.png" becomes "caf%C3%A9.png". Discord rejects the ENTIRE
// embed (400, "embeds" invalid) when an image/thumbnail URL isn't well-formed, so
// this repairs URLs stored raw, e.g. an image picked from a file whose name has
// spaces. Parsing then re-stringifying is idempotent - an already-encoded URL is
// returned unchanged - so it is safe to apply to every URL. Returns ("", false)
// when raw isn't a usable absolute http(s) URL, so the caller omits the field.
func normalizeEmbedURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if !isHTTPURL(raw) {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	return u.String(), true
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
	return string(runes[:n-1]) + "..."
}
