package discord

import (
	"context"
	"fmt"
	"net/http"
)

// Message is a post as the bot sends it. Its pings are explicit: nothing in Content notifies
// anyone beyond what AllowedMentions lists, and a mention inside an embed never notifies at
// all, which is why a role ping travels in Content.
//
// An edit replaces the whole post, so an embed or a button left out of an edit is taken off
// the message (see EditMessage).
type Message struct {
	Content         string          `json:"content"`
	Embeds          []Embed         `json:"embeds"`
	AllowedMentions AllowedMentions `json:"allowed_mentions"`
	Components      []Component     `json:"components"`
	// Nonce makes a create safe to retry: with EnforceNonce, Discord answers a repeat of the
	// same nonce within a few minutes with the message it already made, instead of posting
	// twice. Only meaningful on a create; at most 25 characters.
	Nonce        string `json:"nonce,omitempty"`
	EnforceNonce bool   `json:"enforce_nonce,omitempty"`
}

// MaxNonce is the longest nonce Discord accepts.
const MaxNonce = 25

// AllowedMentions is who a message may notify: "everyone" in Parse for @everyone and @here,
// and roles by id.
type AllowedMentions struct {
	Parse []string `json:"parse"`
	Roles []string `json:"roles"`
}

// Nobody is a message that pings no one, whatever its text names.
func Nobody() AllowedMentions {
	return AllowedMentions{Parse: []string{}, Roles: []string{}}
}

// Embed is the boxed part of a message.
type Embed struct {
	Title       string       `json:"title,omitempty"`
	URL         string       `json:"url,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Thumbnail   *EmbedImage  `json:"thumbnail,omitempty"`
	Image       *EmbedImage  `json:"image,omitempty"`
	Footer      *EmbedFooter `json:"footer,omitempty"`
}

// EmbedField is one name/value pair inside an embed.
type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// EmbedImage is an embed's thumbnail or large image.
type EmbedImage struct {
	URL string `json:"url"`
}

// EmbedFooter is the small line at the bottom of an embed.
type EmbedFooter struct {
	Text string `json:"text"`
}

// Component is an action row or a link button - the only components the app sends.
// Interactive components (buttons that call back, menus, forms) are not used yet.
type Component struct {
	Type       int         `json:"type"`
	Style      int         `json:"style,omitempty"`
	Label      string      `json:"label,omitempty"`
	URL        string      `json:"url,omitempty"`
	Emoji      *Emoji      `json:"emoji,omitempty"`
	Components []Component `json:"components,omitempty"`
}

// Emoji is a unicode emoji (Name holds the character) or a custom server emoji (Name + ID).
type Emoji struct {
	Name     string `json:"name,omitempty"`
	ID       string `json:"id,omitempty"`
	Animated bool   `json:"animated,omitempty"`
}

// Discord's numbers for the components the app sends.
const (
	ComponentActionRow = 1
	ComponentButton    = 2
	ButtonStyleLink    = 5
)

// Posted identifies a message the bot has just made.
type Posted struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
}

// normalize fills the lists Discord would otherwise read as "unchanged" on an edit.
func (m *Message) normalize() {
	if m.Embeds == nil {
		m.Embeds = []Embed{}
	}
	if m.Components == nil {
		m.Components = []Component{}
	}
	if m.AllowedMentions.Parse == nil {
		m.AllowedMentions.Parse = []string{}
	}
	if m.AllowedMentions.Roles == nil {
		m.AllowedMentions.Roles = []string{}
	}
}

// SendMessage posts to a channel. A patient send waits out longer rate limits, as a batch
// of posts has to. A nonce over MaxNonce characters is refused before anything is sent.
func (c *Client) SendMessage(ctx context.Context, channelID string, m Message, patient bool) (Posted, error) {
	if len(m.Nonce) > MaxNonce {
		return Posted{}, fmt.Errorf("discord: nonce longer than %d characters", MaxNonce)
	}
	m.EnforceNonce = m.Nonce != ""
	m.normalize()
	var out Posted
	err := c.doPatient(ctx, http.MethodPost, "/channels/"+channelID+"/messages", m, &out, patient)
	return out, err
}

// EditMessage replaces one of the bot's posts with m, whole: content, embeds, buttons and
// mentions. ErrNotFound means the message was deleted in Discord.
func (c *Client) EditMessage(ctx context.Context, channelID, messageID string, m Message) error {
	m.Nonce, m.EnforceNonce = "", false
	m.normalize()
	return c.do(ctx, http.MethodPatch, "/channels/"+channelID+"/messages/"+messageID, m, nil)
}

// DeleteMessage deletes a message. ErrNotFound means it was already gone, which callers
// treat as done. A patient delete waits out longer rate limits, as one of many in a row has
// to.
func (c *Client) DeleteMessage(ctx context.Context, channelID, messageID string, patient bool) error {
	return c.doPatient(ctx, http.MethodDelete, "/channels/"+channelID+"/messages/"+messageID, nil, nil, patient)
}
