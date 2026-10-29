package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Webhook is what Discord says about an (old-style, channel) webhook: where it posts.
type Webhook struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
}

// webhookURL matches a Discord webhook URL and captures its id and token.
var webhookURL = regexp.MustCompile(`^https://(?:ptb\.|canary\.)?discord(?:app)?\.com/api(?:/v\d+)?/webhooks/(\d+)/([\w-]+)`)

// lookupClient is the plain HTTP client for webhook lookups: no bot token involved.
var lookupClient = &http.Client{Timeout: 15 * time.Second}

// LookupWebhook asks Discord which server and channel an old webhook URL posts to. It needs
// no bot token - the URL's own token is the credential - which is what lets the move off
// webhooks resolve every destination before a bot token is ever entered.
//
// base is the REST base (DefaultAPI when ""); the URL's host is ignored beyond checking it
// is Discord's. ErrNotFound means the webhook is gone (deleted, or its token reset).
// Errors never include the URL.
func LookupWebhook(ctx context.Context, base, rawURL string) (Webhook, error) {
	m := webhookURL.FindStringSubmatch(strings.TrimSpace(rawURL))
	if m == nil {
		return Webhook{}, errors.New("discord: not a webhook URL")
	}
	if base == "" {
		base = DefaultAPI
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(base, "/")+"/webhooks/"+m[1]+"/"+m[2], nil)
	if err != nil {
		return Webhook{}, errors.New("discord: bad webhook URL")
	}
	resp, err := lookupClient.Do(req)
	if err != nil {
		if ue, ok := errors.AsType[*url.Error](err); ok {
			err = ue.Err
		}
		return Webhook{}, fmt.Errorf("discord webhook lookup: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
	if err != nil {
		return Webhook{}, fmt.Errorf("discord webhook lookup: read the answer: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusUnauthorized:
		// Unknown Webhook, or Invalid Webhook Token: either way it no longer works.
		return Webhook{}, ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return Webhook{}, ErrRateLimited
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return Webhook{}, fmt.Errorf("discord webhook lookup: status %d", resp.StatusCode)
	}
	var w Webhook
	if err := json.Unmarshal(data, &w); err != nil {
		return Webhook{}, fmt.Errorf("discord webhook lookup: decode: %w", err)
	}
	return w, nil
}

// DeleteWebhook deletes a webhook by id, as the bot (Manage Webhooks). ErrNotFound means it
// was already gone.
func (c *Client) DeleteWebhook(ctx context.Context, id string) error {
	return c.doPatient(ctx, http.MethodDelete, "/webhooks/"+id, nil, nil, true)
}
