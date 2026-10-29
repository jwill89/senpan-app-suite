package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"app-suite/internal/discord"
	"app-suite/internal/model"
	"app-suite/internal/secretbox"
)

// -- Discord bot: configuration, status, and the server directory --------------
//
// The bot's configuration lives in the settings table under the keys below, none of which
// are in settingsKeys: they are read and written only through the admin-only /api/discord
// endpoints, never through /api/settings (whose GET is public and whose POST any
// system-settings holder may call).
//
// The token is stored sealed (secretbox) under a key derived from the session secret, so a
// copy of the database - the live copy deploy.ps1 pulls to a dev machine - carries only
// ciphertext. Without the production secret, Discord is simply off there.

const (
	settingDiscordToken       = "discord_bot_token"
	settingDiscordGuild       = "discord_guild_id"
	settingDiscordApplication = "discord_application_id"
	settingDiscordStatusText  = "discord_status_text"
	settingDiscordTestMode    = "discord_test_mode"
	settingDiscordTestChannel = "discord_test_channel_id"
	settingPublicBaseURL      = "public_base_url"

	// DiscordTokenPurpose is the HKDF purpose the token key is derived for. main uses it
	// with the session secret; changing it makes every stored token unreadable.
	DiscordTokenPurpose = "senpan discord bot token v1"

	// discordStatusMax is Discord's limit on a custom status.
	discordStatusMax = 128
)

// discordDefaults are the values used until an admin saves something else: the Senpan Tea
// House server and the app's own Discord application.
var discordDefaults = map[string]string{
	settingDiscordGuild:       "999011226568888410",
	settingDiscordApplication: "1563399760994635906",
	settingPublicBaseURL:      "https://apps.senpan.cafe",
}

// SetDiscord installs the bot service, the box tokens are sealed with (nil when the server
// has no configured session secret - a token then can't be stored), and the startup safety
// valves, then applies the saved configuration. Call before serving.
//
//   - dryRun suppresses every Discord write (post, edit, delete) and logs it instead.
//   - redirect sends every new post to that one channel id, whatever the feature chose.
func (s *Server) SetDiscord(svc *discord.Service, box *secretbox.Box, dryRun bool, redirect string) {
	s.discord = svc
	s.secrets = box
	s.discordDryRun = dryRun
	s.discordRedirect = strings.TrimSpace(redirect)
	s.applyDiscordConfig()
}

// discordSetting reads one of the bot's settings, falling back to its default.
func (s *Server) discordSetting(key string) string {
	v, err := s.store.GetSetting(key)
	if err != nil {
		slog.Error("read discord setting", "key", key, "error", err)
	}
	if v == "" {
		return discordDefaults[key]
	}
	return v
}

// applyDiscordConfig reads the saved configuration, opens the token, and reconfigures the
// service. Called at startup and after every change on the Discord page.
func (s *Server) applyDiscordConfig() {
	sealed := s.discordSetting(settingDiscordToken)
	token, readable := "", false
	if sealed != "" && s.secrets != nil {
		if t, err := s.secrets.Open(sealed); err == nil {
			token, readable = t, true
		}
	}
	if sealed != "" && !readable {
		slog.Warn("the saved Discord bot token can't be read on this server; Discord is off until it is entered again")
	}
	s.discordMu.Lock()
	s.discordTokenSet, s.discordTokenReadable = sealed != "", readable
	s.discordMu.Unlock()

	presence := discord.Presence{}
	if text := s.discordSetting(settingDiscordStatusText); text != "" {
		presence = discord.Presence{Activity: "custom", Text: text}
	}
	s.discord.Configure(discord.Config{
		Token:    token,
		GuildID:  s.discordSetting(settingDiscordGuild),
		Presence: presence,
	})
}

// discordInviteURL adds the bot to the server with Administrator (permissions=8), the
// server preselected.
func discordInviteURL(appID, guildID string) string {
	if appID == "" {
		return ""
	}
	q := url.Values{"client_id": {appID}, "scope": {"bot"}, "permissions": {"8"}}
	if guildID != "" {
		q.Set("guild_id", guildID)
		q.Set("disable_guild_select", "true")
	}
	return "https://discord.com/oauth2/authorize?" + q.Encode()
}

// discordErrorText turns a Discord failure into words an admin can act on. It never
// contains the token or a request path.
func discordErrorText(err error) string {
	switch {
	case errors.Is(err, discord.ErrOff):
		return "Discord isn't connected. Set the bot token on the Discord page."
	case errors.Is(err, discord.ErrUnauthorized):
		return "Discord refused the bot token. Paste it again on the Discord page."
	case errors.Is(err, discord.ErrForbidden):
		return "The bot isn't allowed to do that in this channel."
	case errors.Is(err, discord.ErrNotFound):
		return "Discord couldn't find that channel or message."
	case errors.Is(err, discord.ErrRateLimited):
		return "Discord is rate limiting the bot. Try again in a minute."
	case errors.Is(err, discord.ErrRejected):
		if said := discord.Said(err); said != "" {
			return "Discord refused the message: " + said
		}
		return "Discord refused the message."
	case errors.Is(err, context.DeadlineExceeded):
		return "Discord didn't answer in time."
	}
	return "Couldn't reach Discord (" + err.Error() + ")."
}

// discordStatus assembles the Discord page's view, asking Discord (through the directory
// cache) who the bot is and whether it is in the server.
func (s *Server) discordStatus(ctx context.Context) model.DiscordStatus {
	s.discordMu.Lock()
	tokenSet, readable := s.discordTokenSet, s.discordTokenReadable
	s.discordMu.Unlock()
	appID := s.discordSetting(settingDiscordApplication)
	guildID := s.discordSetting(settingDiscordGuild)
	gw := s.discord.GatewayStatus()
	st := model.DiscordStatus{
		KeyAvailable:      s.secrets != nil,
		TokenSet:          tokenSet,
		TokenReadable:     readable,
		Online:            gw.Online,
		ConnectedAt:       gw.ConnectedAt,
		LastError:         gw.LastError,
		ApplicationID:     appID,
		GuildID:           guildID,
		InviteURL:         discordInviteURL(appID, guildID),
		StatusText:        s.discordSetting(settingDiscordStatusText),
		PublicBaseURL:     s.discordSetting(settingPublicBaseURL),
		TestMode:          s.discordSetting(settingDiscordTestMode) == "1",
		TestChannelID:     s.discordSetting(settingDiscordTestChannel),
		DryRun:            s.discordDryRun,
		RedirectChannelID: s.discordRedirect,
	}
	var dirErr error
	if s.discord.Enabled() {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		d, err := s.discord.Directory(ctx)
		if err != nil {
			dirErr = err
		} else {
			st.Connected = true
			st.BotID, st.BotName, st.BotAvatarURL = d.Bot.ID, d.Bot.Named(), d.Bot.AvatarURL()
			st.TokenApplicationID = d.Application.ID
			st.ApplicationMismatch = appID != "" && d.Application.ID != appID
			st.InGuild, st.BotIsAdmin = d.InGuild, d.IsAdmin()
			st.GuildName, st.GuildIconURL = d.Guild.Name, d.Guild.IconURL()
		}
	}
	switch {
	case !st.KeyAvailable:
		st.Problem = "This server has no session secret configured (APPSUITE_SESSION_SECRET), so a bot token can't be stored."
	case st.TokenSet && !st.TokenReadable:
		st.Problem = "The saved bot token can't be read on this server - the session secret changed, or this is a copy of another server's data. Paste the token again."
	case !st.TokenSet:
		st.Problem = "Paste the bot token to connect the bot."
	case dirErr != nil:
		st.Problem = discordErrorText(dirErr)
	case st.ApplicationMismatch:
		st.Problem = fmt.Sprintf("This token belongs to application %s, not the configured application %s.", st.TokenApplicationID, appID)
	case !st.InGuild:
		st.Problem = "The bot isn't in this server yet. Use Invite to add it."
	case !st.BotIsAdmin:
		st.Problem = "The bot doesn't hold Administrator in this server. Give its role Administrator, or invite it again."
	case st.TestMode && st.TestChannelID == "":
		st.Problem = "Test mode is on but has no test channel."
	}
	return st
}

// writeDiscordStatus answers with the current status.
func (s *Server) writeDiscordStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.DiscordStatusResponse{Status: s.discordStatus(r.Context())})
}

// handleDiscordStatus serves the Discord page's status.
//
//	Endpoint:  GET /api/discord/status
//	Auth:      admin
//	Response:  model.DiscordStatusResponse
func (s *Server) handleDiscordStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	s.writeDiscordStatus(w, r)
}

type discordTokenRequest struct {
	Token string `json:"token"`
}

// handleDiscordSetToken checks a bot token with Discord, seals it, and starts the bot with
// it. A token from a different application than the configured one is refused, since it
// would post as the wrong bot.
//
//	Endpoint:  PUT /api/discord/token
//	Auth:      admin
//	Request:   {"token": "..."}
//	Response:  model.DiscordStatusResponse
func (s *Server) handleDiscordSetToken(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	req, err := readJSON[discordTokenRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	// People paste "Bot <token>" from examples; the stored value is the bare token.
	token := strings.TrimSpace(req.Token)
	token = strings.TrimSpace(strings.TrimPrefix(token, "Bot "))
	if token == "" || len(token) > 200 || strings.ContainsAny(token, " \t\r\n") {
		writeError(w, http.StatusBadRequest, "Paste the bot token from the Developer Portal (Bot -> Reset Token)")
		return
	}
	if s.secrets == nil {
		writeError(w, http.StatusConflict, "This server has no session secret configured (APPSUITE_SESSION_SECRET), so a bot token can't be stored")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	me, app, err := s.discord.Probe(ctx, token)
	if err != nil {
		if errors.Is(err, discord.ErrUnauthorized) {
			writeError(w, http.StatusBadRequest, "Discord refused this token. Copy it again from the Developer Portal.")
			return
		}
		writeError(w, http.StatusBadGateway, discordErrorText(err))
		return
	}
	if want := s.discordSetting(settingDiscordApplication); want != "" && app.ID != want {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"This token belongs to application %s (%s), not the configured application %s. Change the application ID first if this is the bot you mean.",
			app.ID, app.Name, want))
		return
	}
	sealed, err := s.secrets.Seal(token)
	if err != nil {
		writeInternalError(w, "seal discord token", err)
		return
	}
	if err := s.store.SetSetting(settingDiscordToken, sealed); err != nil {
		writeInternalError(w, "save discord token", err)
		return
	}
	s.applyDiscordConfig()
	slog.Info("discord bot token saved", "bot", me.Named(), "by", s.actorName(r))
	s.writeDiscordStatus(w, r)
}

// handleDiscordClearToken forgets the token and takes the bot offline.
//
//	Endpoint:  DELETE /api/discord/token
//	Auth:      admin
//	Response:  model.DiscordStatusResponse
func (s *Server) handleDiscordClearToken(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if err := s.store.SetSetting(settingDiscordToken, ""); err != nil {
		writeInternalError(w, "clear discord token", err)
		return
	}
	s.applyDiscordConfig()
	slog.Info("discord bot token removed", "by", s.actorName(r))
	s.writeDiscordStatus(w, r)
}

type discordConfigRequest struct {
	GuildID       string `json:"guild_id"`
	ApplicationID string `json:"application_id"`
	StatusText    string `json:"status_text"`
	PublicBaseURL string `json:"public_base_url"`
	TestMode      bool   `json:"test_mode"`
	TestChannelID string `json:"test_channel_id"`
}

// normalizeBaseURL validates an absolute http(s) URL and drops a trailing slash.
func normalizeBaseURL(raw string) (string, bool) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	return raw, true
}

// handleDiscordConfig saves the server, application, presence, public URL and test mode.
//
//	Endpoint:  PUT /api/discord/config
//	Auth:      admin
//	Request:   discordConfigRequest
//	Response:  model.DiscordStatusResponse
func (s *Server) handleDiscordConfig(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	req, err := readJSON[discordConfigRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	guild := strings.TrimSpace(req.GuildID)
	app := strings.TrimSpace(req.ApplicationID)
	text := strings.TrimSpace(req.StatusText)
	testChannel := strings.TrimSpace(req.TestChannelID)
	base, ok := normalizeBaseURL(req.PublicBaseURL)
	switch {
	case !isDiscordSnowflake(guild):
		writeError(w, http.StatusBadRequest, "Server ID must be digits (right-click the server -> Copy Server ID)")
		return
	case !isDiscordSnowflake(app):
		writeError(w, http.StatusBadRequest, "Application ID must be digits (Developer Portal -> General Information)")
		return
	case utf8.RuneCountInString(text) > discordStatusMax:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Status text can be at most %d characters", discordStatusMax))
		return
	case !ok:
		writeError(w, http.StatusBadRequest, "Public site URL must be an absolute http(s) address, like https://apps.senpan.cafe")
		return
	case testChannel != "" && !isDiscordSnowflake(testChannel):
		writeError(w, http.StatusBadRequest, "Test channel must be a channel ID")
		return
	case req.TestMode && testChannel == "":
		writeError(w, http.StatusBadRequest, "Pick a test channel before turning test mode on")
		return
	}
	testMode := "0"
	if req.TestMode {
		testMode = "1"
	}
	for key, value := range map[string]string{
		settingDiscordGuild:       guild,
		settingDiscordApplication: app,
		settingDiscordStatusText:  text,
		settingPublicBaseURL:      base,
		settingDiscordTestMode:    testMode,
		settingDiscordTestChannel: testChannel,
	} {
		if err := s.store.SetSetting(key, value); err != nil {
			writeInternalError(w, "save discord config", err)
			return
		}
	}
	s.applyDiscordConfig()
	slog.Info("discord configuration saved", "test_mode", req.TestMode, "by", s.actorName(r))
	s.writeDiscordStatus(w, r)
}

type discordTestRequest struct {
	ChannelID string `json:"channel_id"`
}

// handleDiscordTest posts a short test message to a channel. The dry-run and redirect
// valves apply; test mode doesn't (the channel is chosen explicitly).
//
//	Endpoint:  POST /api/discord/test
//	Auth:      admin
//	Request:   {"channel_id": "..."}
//	Response:  model.DiscordTestResponse
func (s *Server) handleDiscordTest(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	req, err := readJSON[discordTestRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	channel := strings.TrimSpace(req.ChannelID)
	if !isDiscordSnowflake(channel) {
		writeError(w, http.StatusBadRequest, "Pick a channel")
		return
	}
	c := s.discord.Client()
	if c == nil {
		writeError(w, http.StatusConflict, discordErrorText(discord.ErrOff))
		return
	}
	by := s.actorName(r)
	if s.discordDryRun {
		slog.Warn("discord post SUPPRESSED (dry run)", "kind", "test", "by", by)
		writeJSON(w, http.StatusOK, model.DiscordTestResponse{Suppressed: true, ChannelID: channel,
			Note: "This server is in dry-run mode, so nothing was sent."})
		return
	}
	note := ""
	if s.discordRedirect != "" {
		channel = s.discordRedirect
		note = "Redirected to this server's redirect channel."
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	posted, err := c.SendMessage(ctx, channel, discord.Message{
		Content:         "✅ Test message from the Senpan App Suite, sent by " + by + ". Posting works.",
		AllowedMentions: discord.Nobody(),
	}, false)
	if err != nil {
		slog.Error("discord test message failed", "error", err, "by", by)
		writeError(w, http.StatusBadGateway, discordErrorText(err))
		return
	}
	slog.Info("discord post sent", "kind", "test", "by", by)
	writeJSON(w, http.StatusOK, model.DiscordTestResponse{Sent: true, ChannelID: posted.ChannelID,
		MessageID: posted.ID, Note: note})
}

// requireAnyDiscordPoster guards the channel and role lists: admins, and anyone who can
// post from a system (announcements, tea rooms, affiliates, any book club).
func (s *Server) requireAnyDiscordPoster(w http.ResponseWriter, r *http.Request) bool {
	u := s.currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized - login required")
		return false
	}
	if anyBookClubPerm(u) || userHasPermission(u, permTeahouseAnnounce) ||
		userHasPermission(u, permTeahouseTeaRooms) || userHasPermission(u, permTeahouseAffiliates) {
		return true
	}
	writeError(w, http.StatusForbidden, "Forbidden - you do not have access to this feature")
	return false
}

// discordDirectory reads the directory for a picker endpoint, or explains why it can't.
func (s *Server) discordDirectory(r *http.Request) (discord.Directory, string) {
	if !s.discord.Enabled() {
		return discord.Directory{}, discordErrorText(discord.ErrOff)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	d, err := s.discord.Directory(ctx)
	if err != nil {
		return discord.Directory{}, discordErrorText(err)
	}
	if !d.InGuild {
		return discord.Directory{}, "The bot isn't in the server yet."
	}
	return d, ""
}

// handleDiscordChannels lists the channels the bot can post to: text and announcement
// channels, grouped the way Discord shows them (uncategorized first, then each category in
// order, each channel in position order).
//
//	Endpoint:  GET /api/discord/channels
//	Auth:      admin or any posting permission
//	Response:  model.DiscordChannelsResponse
func (s *Server) handleDiscordChannels(w http.ResponseWriter, r *http.Request) {
	if !s.requireAnyDiscordPoster(w, r) {
		return
	}
	d, problem := s.discordDirectory(r)
	if problem != "" {
		writeJSON(w, http.StatusOK, model.DiscordChannelsResponse{Channels: []model.DiscordChannelOption{}, Problem: problem})
		return
	}
	writeJSON(w, http.StatusOK, model.DiscordChannelsResponse{Channels: channelOptions(d.Channels), Available: true})
}

// channelOptions orders and labels the postable channels.
func channelOptions(all []discord.Channel) []model.DiscordChannelOption {
	categories := map[string]discord.Channel{}
	for _, c := range all {
		if c.Type == discord.ChannelCategory {
			categories[c.ID] = c
		}
	}
	postable := slices.DeleteFunc(slices.Clone(all), func(c discord.Channel) bool {
		return c.Type != discord.ChannelText && c.Type != discord.ChannelAnnouncement
	})
	// A channel's category position decides its group (-1 = uncategorized, first).
	groupOf := func(c discord.Channel) int {
		if cat, ok := categories[c.ParentID]; ok {
			return cat.Position
		}
		return -1
	}
	slices.SortStableFunc(postable, func(a, b discord.Channel) int {
		return cmp.Or(
			cmp.Compare(groupOf(a), groupOf(b)),
			cmp.Compare(a.ParentID, b.ParentID),
			cmp.Compare(a.Position, b.Position),
			cmp.Compare(a.ID, b.ID),
		)
	})
	out := make([]model.DiscordChannelOption, 0, len(postable))
	for _, c := range postable {
		kind := "text"
		if c.Type == discord.ChannelAnnouncement {
			kind = "announcement"
		}
		out = append(out, model.DiscordChannelOption{ID: c.ID, Name: c.Name, Type: kind, Category: categories[c.ParentID].Name})
	}
	return out
}

// handleDiscordRoles lists the server's roles for the ping pickers, highest first, without
// @everyone (the pickers offer it as its own option).
//
//	Endpoint:  GET /api/discord/roles
//	Auth:      admin or any posting permission
//	Response:  model.DiscordRolesResponse
func (s *Server) handleDiscordRoles(w http.ResponseWriter, r *http.Request) {
	if !s.requireAnyDiscordPoster(w, r) {
		return
	}
	d, problem := s.discordDirectory(r)
	if problem != "" {
		writeJSON(w, http.StatusOK, model.DiscordRolesResponse{Roles: []model.DiscordRoleOption{}, Problem: problem})
		return
	}
	roles := slices.DeleteFunc(slices.Clone(d.Roles), func(role discord.Role) bool { return role.ID == d.Guild.ID })
	slices.SortStableFunc(roles, func(a, b discord.Role) int {
		return cmp.Or(cmp.Compare(b.Position, a.Position), cmp.Compare(a.ID, b.ID))
	})
	out := make([]model.DiscordRoleOption, 0, len(roles))
	for _, role := range roles {
		color := ""
		if role.Color != 0 {
			color = fmt.Sprintf("#%06x", role.Color)
		}
		out = append(out, model.DiscordRoleOption{ID: role.ID, Name: role.Name, Color: color, Managed: role.Managed})
	}
	writeJSON(w, http.StatusOK, model.DiscordRolesResponse{Roles: out, Available: true})
}
