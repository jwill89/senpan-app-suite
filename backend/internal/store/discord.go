package store

import (
	"database/sql"
	"errors"
	"time"

	"app-suite/internal/model"
)

// -- Discord bot: destinations, posted messages, and the webhook import ----------

// now is the timestamp format the Discord tables store (UTC RFC-3339).
func discordNow() string { return time.Now().UTC().Format(time.RFC3339) }

// GetDiscordDestination returns a system's destination. One that was never saved comes
// back empty (no channel) with the default repost mode, so callers need no special case.
func (s *Store) GetDiscordDestination(key string) (model.DiscordDestination, error) {
	d := model.DiscordDestination{Key: key, RepostMode: model.DefaultRepostMode(key)}
	err := s.db.QueryRow(`SELECT channel_id, mention, repost_mode, updated_at
		FROM discord_destinations WHERE key = ?`, key).
		Scan(&d.ChannelID, &d.Mention, &d.RepostMode, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return d, nil
	}
	return d, err
}

// SaveDiscordDestination stores a system's destination, replacing what was there.
func (s *Store) SaveDiscordDestination(d model.DiscordDestination) error {
	_, err := s.db.Exec(`INSERT INTO discord_destinations (key, channel_id, mention, repost_mode, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET channel_id = excluded.channel_id, mention = excluded.mention,
			repost_mode = excluded.repost_mode, updated_at = excluded.updated_at`,
		d.Key, d.ChannelID, d.Mention, d.RepostMode, discordNow())
	return err
}

// SetDiscordDestinationChannelIfEmpty gives a system a channel unless an admin already
// chose one, reporting whether it did. The webhook importer uses it, so an import can
// never overwrite a choice made on the new pickers.
func (s *Store) SetDiscordDestinationChannelIfEmpty(key, channelID string) (bool, error) {
	res, err := s.db.Exec(`INSERT INTO discord_destinations (key, channel_id, repost_mode, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET channel_id = excluded.channel_id, updated_at = excluded.updated_at
		WHERE discord_destinations.channel_id = ''`, key, channelID, model.DefaultRepostMode(key), discordNow())
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// SetAnnouncementTypeChannelIfEmpty gives an announcement type a channel unless one is
// already set, reporting whether it did (false too when the type no longer exists).
func (s *Store) SetAnnouncementTypeChannelIfEmpty(id int64, channelID string) (bool, error) {
	res, err := s.db.Exec(`UPDATE announcement_types SET channel_id = ? WHERE id = ? AND channel_id = ''`,
		channelID, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// -- Posted messages -------------------------------------------------------------

const discordPostColumns = `id, kind, ref_id, scope, batch, seq, channel_id, message_id,
	fingerprint, posted_at, edited_at, gone_at`

func scanDiscordPosts(rows *sql.Rows) ([]model.DiscordPost, error) {
	defer rows.Close()
	out := []model.DiscordPost{}
	for rows.Next() {
		var p model.DiscordPost
		if err := rows.Scan(&p.ID, &p.Kind, &p.RefID, &p.Scope, &p.Batch, &p.Seq, &p.ChannelID,
			&p.MessageID, &p.Fingerprint, &p.PostedAt, &p.EditedAt, &p.GoneAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RecordDiscordPost stores a message the bot just posted and returns its row id.
func (s *Store) RecordDiscordPost(p model.DiscordPost) (int64, error) {
	if p.PostedAt == "" {
		p.PostedAt = discordNow()
	}
	res, err := s.db.Exec(`INSERT INTO discord_posts (kind, ref_id, scope, batch, seq, channel_id,
		message_id, fingerprint, posted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Kind, p.RefID, p.Scope, p.Batch, p.Seq, p.ChannelID, p.MessageID, p.Fingerprint, p.PostedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// LiveDiscordPosts lists an item's messages that are still up, oldest first.
func (s *Store) LiveDiscordPosts(kind string, refID int64) ([]model.DiscordPost, error) {
	rows, err := s.db.Query(`SELECT `+discordPostColumns+` FROM discord_posts
		WHERE kind = ? AND ref_id = ? AND gone_at = '' ORDER BY id`, kind, refID)
	if err != nil {
		return nil, err
	}
	return scanDiscordPosts(rows)
}

// LiveDiscordPostsInScope lists a scope's messages that are still up (every live
// reading-list message of one club, say), oldest first.
func (s *Store) LiveDiscordPostsInScope(kind, scope string) ([]model.DiscordPost, error) {
	rows, err := s.db.Query(`SELECT `+discordPostColumns+` FROM discord_posts
		WHERE kind = ? AND scope = ? AND gone_at = '' ORDER BY id`, kind, scope)
	if err != nil {
		return nil, err
	}
	return scanDiscordPosts(rows)
}

// DiscordPostStates returns, for every item of a kind that has posted, the post that
// stands for it: its newest message still up, or - when none is - its newest row, which
// is then one Discord reported deleted. Items never posted (or taken down) are absent.
func (s *Store) DiscordPostStates(kind string) (map[int64]model.DiscordPost, error) {
	rows, err := s.db.Query(`SELECT `+discordPostColumns+` FROM discord_posts WHERE kind = ? ORDER BY id`, kind)
	if err != nil {
		return nil, err
	}
	all, err := scanDiscordPosts(rows)
	if err != nil {
		return nil, err
	}
	out := map[int64]model.DiscordPost{}
	for _, p := range all {
		cur, seen := out[p.RefID]
		// Rows come oldest first: a later row wins unless it would replace a live post
		// with a gone one.
		if !seen || p.GoneAt == "" || cur.GoneAt != "" {
			out[p.RefID] = p
		}
	}
	return out, nil
}

// ForgetDiscordPost removes the record of a message the app deleted itself.
func (s *Store) ForgetDiscordPost(id int64) error {
	_, err := s.db.Exec(`DELETE FROM discord_posts WHERE id = ?`, id)
	return err
}

// ForgetDiscordPosts removes every record for an item - once the item itself is deleted
// and its messages have been taken down.
func (s *Store) ForgetDiscordPosts(kind string, refID int64) error {
	_, err := s.db.Exec(`DELETE FROM discord_posts WHERE kind = ? AND ref_id = ?`, kind, refID)
	return err
}

// ForgetGoneDiscordPosts removes an item's "deleted in Discord" records - once a new post
// has replaced them, or the item was taken down on purpose, they no longer describe it.
func (s *Store) ForgetGoneDiscordPosts(kind string, refID int64) error {
	_, err := s.db.Exec(`DELETE FROM discord_posts WHERE kind = ? AND ref_id = ? AND gone_at <> ''`, kind, refID)
	return err
}

// MarkDiscordPostGone records that Discord no longer has a message (someone deleted it
// there). It only touches the row still holding that message id, so a stale caller can't
// mark a newer post gone.
func (s *Store) MarkDiscordPostGone(id int64, messageID string) error {
	_, err := s.db.Exec(`UPDATE discord_posts SET gone_at = ? WHERE id = ? AND message_id = ? AND gone_at = ''`,
		discordNow(), id, messageID)
	return err
}

// MarkDiscordPostEdited records an edit and the fingerprint of what the message now says.
func (s *Store) MarkDiscordPostEdited(id int64, fingerprint string) error {
	_, err := s.db.Exec(`UPDATE discord_posts SET edited_at = ?, fingerprint = ? WHERE id = ?`,
		discordNow(), fingerprint, id)
	return err
}

// -- The webhook import ---------------------------------------------------------

// LegacyWebhook is one old webhook awaiting (or done with) import. URL is a credential:
// it is never put on an API model, and is blanked once the row settles.
type LegacyWebhook struct {
	Source    string
	Label     string
	URL       string
	WebhookID string
	GuildID   string
	ChannelID string
	Status    string
	Detail    string
	SettledAt string
}

func (s *Store) legacyWebhooks(where string, args ...any) ([]LegacyWebhook, error) {
	rows, err := s.db.Query(`SELECT source, label, url, webhook_id, guild_id, channel_id, status, detail, settled_at
		FROM discord_legacy_webhooks `+where+` ORDER BY source`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LegacyWebhook{}
	for rows.Next() {
		var w LegacyWebhook
		if err := rows.Scan(&w.Source, &w.Label, &w.URL, &w.WebhookID, &w.GuildID, &w.ChannelID,
			&w.Status, &w.Detail, &w.SettledAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// LegacyWebhooksToImport lists the webhooks still to look up: never tried, or tried and
// failed for a reason worth trying again.
func (s *Store) LegacyWebhooksToImport() ([]LegacyWebhook, error) {
	return s.legacyWebhooks(`WHERE status IN ('pending', 'error') AND url <> ''`)
}

// LegacyWebhooks lists every row of the import report.
func (s *Store) LegacyWebhooks() ([]LegacyWebhook, error) {
	return s.legacyWebhooks(``)
}

// SettleLegacyWebhook records what the importer found. Every outcome except "error"
// is final, so the URL is blanked; an error keeps it for the next try.
func (s *Store) SettleLegacyWebhook(source, status, detail, guildID, channelID string) error {
	_, err := s.db.Exec(`UPDATE discord_legacy_webhooks
		SET status = ?, detail = ?, guild_id = ?, channel_id = ?, settled_at = ?,
			url = CASE WHEN ? = 'error' THEN url ELSE '' END
		WHERE source = ?`, status, detail, guildID, channelID, discordNow(), status, source)
	return err
}

// AddLegacyWebhook queues one old webhook for import. The v70 migration fills the table
// itself; this exists so tests (and any later one-off) can add a row the same way.
func (s *Store) AddLegacyWebhook(source, label, url, webhookID string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO discord_legacy_webhooks (source, label, url, webhook_id)
		VALUES (?, ?, ?, ?)`, source, label, url, webhookID)
	return err
}

// ClearLegacyWebhooks deletes the import report (and any URL still waiting in it).
func (s *Store) ClearLegacyWebhooks() error {
	_, err := s.db.Exec(`DELETE FROM discord_legacy_webhooks`)
	return err
}
