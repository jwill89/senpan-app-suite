package discord

import (
	"context"
	"net/http"
)

// Application is the bot's application record: its id (which the invite link needs) and
// name.
type Application struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
	Bot  *User  `json:"bot"`
}

// Application reads the bot's own application. With a bad token this is where it shows:
// ErrUnauthorized.
func (c *Client) Application(ctx context.Context) (Application, error) {
	var out Application
	err := c.do(ctx, http.MethodGet, "/applications/@me", nil, &out)
	return out, err
}

// User is a Discord account, as the API gives one.
type User struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
	Avatar     string `json:"avatar"`
	Bot        bool   `json:"bot"`
}

// Named is what the account is called: its display name, or failing that its username.
func (u User) Named() string {
	if u.GlobalName != "" {
		return u.GlobalName
	}
	return u.Username
}

// AvatarURL is the account's avatar on Discord's CDN, or "" when it has none.
func (u User) AvatarURL() string {
	if u.ID == "" || u.Avatar == "" {
		return ""
	}
	return "https://cdn.discordapp.com/avatars/" + u.ID + "/" + u.Avatar + ".png?size=128"
}

// Me reads the bot's own account. It doubles as the check that a token works.
func (c *Client) Me(ctx context.Context) (User, error) {
	var out User
	err := c.do(ctx, http.MethodGet, "/users/@me", nil, &out)
	return out, err
}

// Guild is the server itself: its name and icon.
type Guild struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
	OwnerID string `json:"owner_id"`
}

// IconURL is the server's icon on Discord's CDN, or "" when it has none.
func (g Guild) IconURL() string {
	if g.ID == "" || g.Icon == "" {
		return ""
	}
	return "https://cdn.discordapp.com/icons/" + g.ID + "/" + g.Icon + ".png?size=128"
}

// Guild reads a server. ErrNotFound or ErrForbidden here means the bot is not in it.
func (c *Client) Guild(ctx context.Context, guildID string) (Guild, error) {
	var out Guild
	err := c.do(ctx, http.MethodGet, "/guilds/"+guildID, nil, &out)
	return out, err
}

// Channel is a server channel, with the fields the pickers need.
type Channel struct {
	ID       string `json:"id"`
	Type     int    `json:"type"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Position int    `json:"position"`
}

// Discord's numbers for the kinds of channel the app tells apart. Only text and
// announcement channels can be posted to here; a category only groups them.
const (
	ChannelText         = 0
	ChannelCategory     = 4
	ChannelAnnouncement = 5
)

// GuildChannels lists the server's channels. Threads are not among them.
func (c *Client) GuildChannels(ctx context.Context, guildID string) ([]Channel, error) {
	var out []Channel
	err := c.do(ctx, http.MethodGet, "/guilds/"+guildID+"/channels", nil, &out)
	return out, err
}

// Role is a server role. The one every member holds (@everyone) has the server's own id.
type Role struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       int    `json:"color"`
	Position    int    `json:"position"`
	Permissions string `json:"permissions"`
	Managed     bool   `json:"managed"`
	Mentionable bool   `json:"mentionable"`
}

// GuildRoles lists the server's roles.
func (c *Client) GuildRoles(ctx context.Context, guildID string) ([]Role, error) {
	var out []Role
	err := c.do(ctx, http.MethodGet, "/guilds/"+guildID+"/roles", nil, &out)
	return out, err
}

// Member is one account's membership of a server: the roles it holds there.
type Member struct {
	Roles []string `json:"roles"`
}

// Member reads an account's membership. ErrNotFound means it is not in the server.
func (c *Client) Member(ctx context.Context, guildID, userID string) (Member, error) {
	var out Member
	err := c.do(ctx, http.MethodGet, "/guilds/"+guildID+"/members/"+userID, nil, &out)
	return out, err
}

// GatewayURL asks Discord where the gateway is.
func (c *Client) GatewayURL(ctx context.Context) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, http.MethodGet, "/gateway/bot", nil, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}
