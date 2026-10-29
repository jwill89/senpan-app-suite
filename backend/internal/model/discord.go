package model

// DiscordStatus is what the System -> Discord page shows: whether a bot token can be stored
// and is set, whether the bot is connected and online, who it is, the server it works in,
// and the posting safety switches. It never carries the token itself.
type DiscordStatus struct {
	// KeyAvailable is false when the server has no configured session secret, so a token
	// could not be stored (it would be unreadable after the next restart).
	KeyAvailable bool `json:"key_available"`
	// TokenSet is true when a token is saved; TokenReadable when this server can decrypt it.
	TokenSet      bool `json:"token_set"`
	TokenReadable bool `json:"token_readable"`
	// Connected is true when Discord answers the bot's REST calls; Online when its gateway
	// session is up (the bot shows as online in the member list).
	Connected   bool   `json:"connected"`
	Online      bool   `json:"online"`
	ConnectedAt string `json:"connected_at"`
	LastError   string `json:"last_error"`

	BotID        string `json:"bot_id"`
	BotName      string `json:"bot_name"`
	BotAvatarURL string `json:"bot_avatar_url"`
	// ApplicationID is the configured application; TokenApplicationID the one the token
	// belongs to. ApplicationMismatch flags a token from a different bot.
	ApplicationID       string `json:"application_id"`
	TokenApplicationID  string `json:"token_application_id"`
	ApplicationMismatch bool   `json:"application_mismatch"`

	GuildID      string `json:"guild_id"`
	GuildName    string `json:"guild_name"`
	GuildIconURL string `json:"guild_icon_url"`
	InGuild      bool   `json:"in_guild"`
	BotIsAdmin   bool   `json:"bot_is_admin"`
	// InviteURL adds the bot to the configured server with Administrator.
	InviteURL string `json:"invite_url"`

	StatusText    string `json:"status_text"`
	PublicBaseURL string `json:"public_base_url"`
	// TestMode sends every new post to TestChannelID and holds scheduled announcements.
	TestMode      bool   `json:"test_mode"`
	TestChannelID string `json:"test_channel_id"`
	// DryRun and RedirectChannelID are the server's startup safety valves (flags/env).
	DryRun            bool   `json:"dry_run"`
	RedirectChannelID string `json:"redirect_channel_id"`

	// Problem is the one thing to fix next, in plain words, or "" when all is well.
	Problem string `json:"problem"`
}

// DiscordStatusResponse wraps DiscordStatus.
type DiscordStatusResponse struct {
	Status DiscordStatus `json:"status"`
}

// DiscordChannelOption is a channel the bot can post to, for the channel pickers.
type DiscordChannelOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Type is "text" or "announcement".
	Type string `json:"type"`
	// Category is the name of the category the channel sits under, or "".
	Category string `json:"category"`
}

// DiscordChannelsResponse lists the server's postable channels. Available is false (with a
// Problem) when the bot isn't connected or isn't in the server; the pickers then fall back
// to typing an id.
type DiscordChannelsResponse struct {
	Channels  []DiscordChannelOption `json:"channels"`
	Available bool                   `json:"available"`
	Problem   string                 `json:"problem"`
}

// DiscordRoleOption is a role that can be pinged, for the role pickers. @everyone is not
// listed; the pickers offer it separately.
type DiscordRoleOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Color is "#rrggbb", or "" for a role with no color.
	Color string `json:"color"`
	// Managed roles belong to a bot or an integration.
	Managed bool `json:"managed"`
}

// DiscordRolesResponse lists the server's roles, highest first.
type DiscordRolesResponse struct {
	Roles     []DiscordRoleOption `json:"roles"`
	Available bool                `json:"available"`
	Problem   string              `json:"problem"`
}

// Repost modes: what a system does with its previous post when it posts the same thing
// again.
const (
	// RepostNew posts a new message and leaves earlier ones alone.
	RepostNew = "new"
	// RepostReplace posts the new message first, then deletes the previous one.
	RepostReplace = "replace"
	// RepostEdit keeps one live message per item and edits it in place.
	RepostEdit = "edit"
)

// DefaultRepostMode is a destination's mode until an admin picks one: tea rooms and
// affiliates keep one post up to date ("edit"); a book club adds each reading list ("new"),
// as posting always did.
func DefaultRepostMode(key string) string {
	if key == "tea-rooms" || key == "affiliates" {
		return RepostEdit
	}
	return RepostNew
}

// DiscordDestination is where one posting system sends its posts: a channel, who to
// ping, and what to do with the previous post. Keys: "bookclub:<slug>", "tea-rooms",
// "affiliates". (Announcement types carry their own channel.)
type DiscordDestination struct {
	Key       string `json:"key"`
	ChannelID string `json:"channel_id"`
	// Mention is "", "everyone" or "role:<role id>".
	Mention    string `json:"mention"`
	RepostMode string `json:"repost_mode"`
	UpdatedAt  string `json:"updated_at"`
}

// DiscordPost is one message the bot posted, kept so it can be edited or deleted later.
type DiscordPost struct {
	ID int64 `json:"id"`
	// Kind is the posting system: "announcement", "reading_list", "tea_room",
	// "affiliate" or "test".
	Kind  string `json:"kind"`
	RefID int64  `json:"ref_id"`
	// Scope narrows Kind (the club slug for reading lists); Batch groups the messages of
	// one publish, Seq orders them.
	Scope       string `json:"scope"`
	Batch       string `json:"batch"`
	Seq         int    `json:"seq"`
	ChannelID   string `json:"channel_id"`
	MessageID   string `json:"message_id"`
	Fingerprint string `json:"fingerprint"`
	PostedAt    string `json:"posted_at"`
	EditedAt    string `json:"edited_at"`
	// GoneAt is set when Discord reported the message missing - someone deleted it
	// there. A message the app deletes itself is forgotten (its row removed) instead,
	// so a gone row always means "deleted in Discord".
	GoneAt string `json:"gone_at"`
}

// Post states (DiscordPostState.State).
const (
	// PostLive: the message is up in Discord.
	PostLive = "live"
	// PostGone: the message was deleted in Discord, and nothing has replaced it.
	PostGone = "gone"
)

// DiscordPostState is where an item stands in Discord: its newest message still up,
// or the fact that the last one was deleted there. Absent (nil) means never posted,
// or taken down by the app.
type DiscordPostState struct {
	State     string `json:"state"`
	ChannelID string `json:"channel_id"`
	// URL opens the message in Discord ("" when gone).
	URL      string `json:"url"`
	PostedAt string `json:"posted_at"`
	EditedAt string `json:"edited_at"`
}

// DiscordDestinationResponse is the body of GET/PUT /api/discord/destinations/{key}.
type DiscordDestinationResponse struct {
	Destination DiscordDestination `json:"destination"`
}

// DiscordPostResult is the body of a "post to Discord" or "take down" command: where
// the item now stands, plus a note when something needs saying (a dry run sent
// nothing, an older post couldn't be removed).
type DiscordPostResult struct {
	Post *DiscordPostState `json:"post"`
	Note string            `json:"note"`
}

// Webhook import statuses (DiscordImportRow.Status).
const (
	ImportPending        = "pending"
	ImportImported       = "imported"
	ImportOtherServer    = "other_server"
	ImportWebhookDeleted = "webhook_deleted"
	ImportError          = "error"
)

// DiscordImportRow reports what became of one old webhook when the app moved to the bot.
// It never carries the webhook URL (a credential).
type DiscordImportRow struct {
	// Source is what the webhook belonged to: "announcement_type:<id>",
	// "bookclub:<slug>", "tea-rooms" or "affiliates".
	Source string `json:"source"`
	// Label names it for people; Where says where its channel is set now.
	Label string `json:"label"`
	Where string `json:"where"`
	// Status is pending, imported, other_server, webhook_deleted or error.
	Status    string `json:"status"`
	Detail    string `json:"detail"`
	ChannelID string `json:"channel_id"`
	SettledAt string `json:"settled_at"`
}

// DiscordImportResponse lists the import report.
type DiscordImportResponse struct {
	Rows []DiscordImportRow `json:"rows"`
}

// DiscordLegacyCleanupResponse reports removing the old webhooks from Discord.
type DiscordLegacyCleanupResponse struct {
	// Deleted webhooks were removed from Discord; Skipped ones could not be (another
	// server, or never imported); Failed ones Discord refused. The report is cleared
	// either way.
	Deleted int `json:"deleted"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// DiscordTestResponse reports a test message.
type DiscordTestResponse struct {
	Sent bool `json:"sent"`
	// Suppressed is true when the server's dry-run valve stopped the message.
	Suppressed bool   `json:"suppressed"`
	ChannelID  string `json:"channel_id"`
	MessageID  string `json:"message_id"`
	Note       string `json:"note"`
}
