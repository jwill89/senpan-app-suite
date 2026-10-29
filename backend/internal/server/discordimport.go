package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/discord"
	"app-suite/internal/model"
	"app-suite/internal/store"
)

// -- The one-time move off webhooks --------------------------------------------
//
// Schema v70 moved every old webhook URL into discord_legacy_webhooks. At startup the
// importer looks each one up - Discord answers "which server and channel does this
// webhook post to?" for the webhook's own URL, no bot token needed - and gives the
// system it belonged to that channel, unless an admin has already picked one. A webhook
// that posts into another server can't be followed (the bot works in one server), so it
// is reported for someone to pick a channel by hand.
//
// Every outcome but a transient error blanks the URL, so the credentials leave the
// database as soon as they have been read. The rows stay as the import report on the
// Discord page until an admin clears it.

// RunDiscordImport resolves any old webhooks still waiting. Called once at startup; it
// returns when done (or when ctx ends).
func (s *Server) RunDiscordImport(ctx context.Context) {
	s.importLegacyWebhooks(ctx)
}

// importLegacyWebhooks looks up every pending webhook and settles it.
func (s *Server) importLegacyWebhooks(ctx context.Context) {
	todo, err := s.store.LegacyWebhooksToImport()
	if err != nil {
		slog.Error("discord webhook import: read pending webhooks", "error", err)
		return
	}
	if len(todo) == 0 {
		return
	}
	guildID := s.discordSetting(settingDiscordGuild)
	counts := map[string]int{}
	for _, w := range todo {
		if ctx.Err() != nil {
			return
		}
		status, detail, guild, channel := s.importOne(ctx, w, guildID)
		if err := s.store.SettleLegacyWebhook(w.Source, status, detail, guild, channel); err != nil {
			slog.Error("discord webhook import: record result", "source", w.Source, "error", err)
			continue
		}
		counts[status]++
	}
	slog.Info("discord webhook import", "imported", counts[model.ImportImported],
		"other_server", counts[model.ImportOtherServer], "webhook_deleted", counts[model.ImportWebhookDeleted],
		"errors", counts[model.ImportError])
}

// importOne looks one webhook up and, when it posts into the bot's server, gives its
// system that channel.
func (s *Server) importOne(ctx context.Context, w store.LegacyWebhook, guildID string) (status, detail, guild, channel string) {
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	hook, err := discord.LookupWebhook(lookupCtx, s.discord.Base(), w.URL)
	switch {
	case errors.Is(err, discord.ErrNotFound):
		return model.ImportWebhookDeleted, "The webhook no longer exists in Discord.", "", ""
	case err != nil:
		return model.ImportError, "Couldn't reach Discord to look it up; it will be tried again.", "", ""
	case hook.GuildID != guildID:
		return model.ImportOtherServer, "It posted into another Discord server. Pick a channel in this one.", hook.GuildID, hook.ChannelID
	}
	set, err := s.assignImportedChannel(w.Source, hook.ChannelID)
	switch {
	case err != nil:
		slog.Error("discord webhook import: assign channel", "source", w.Source, "error", err)
		return model.ImportError, "Couldn't save the channel; it will be tried again.", "", ""
	case !set:
		return model.ImportImported, "A channel was already chosen, so it was kept.", hook.GuildID, hook.ChannelID
	}
	return model.ImportImported, "", hook.GuildID, hook.ChannelID
}

// assignImportedChannel gives the webhook's system its channel, unless one is set.
// Returns false when it kept an existing choice (or the announcement type is gone).
func (s *Server) assignImportedChannel(source, channelID string) (bool, error) {
	if id, ok := strings.CutPrefix(source, "announcement_type:"); ok {
		typeID, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return false, nil
		}
		return s.store.SetAnnouncementTypeChannelIfEmpty(typeID, channelID)
	}
	return s.store.SetDiscordDestinationChannelIfEmpty(source, channelID)
}

// importWhere says where a source's channel is set now, for the report.
func importWhere(source string) (label, where string) {
	switch {
	case source == "tea-rooms":
		return "Tea Rooms", "Senpan Tea House -> Tea Rooms -> Discord"
	case source == "affiliates":
		return "Affiliates", "Senpan Tea House -> Affiliates -> Discord"
	case strings.HasPrefix(source, "bookclub:"):
		slug := strings.TrimPrefix(source, "bookclub:")
		for _, c := range bookClubs {
			if c.Slug == slug {
				return c.Name + " reading lists", "Senpan Tea House -> " + c.Name + " -> Discord"
			}
		}
		return "Book club " + slug + " reading lists", "Senpan Tea House -> book club -> Discord"
	}
	return "", "Senpan Tea House -> Announcements -> Manage Types"
}

// importReport builds the report rows, naming announcement types by their current name.
func (s *Server) importReport() ([]model.DiscordImportRow, error) {
	rows, err := s.store.LegacyWebhooks()
	if err != nil {
		return nil, err
	}
	out := make([]model.DiscordImportRow, 0, len(rows))
	for _, w := range rows {
		label, where := importWhere(w.Source)
		if label == "" {
			label = "Announcement type: " + w.Label
			if id, err := strconv.ParseInt(strings.TrimPrefix(w.Source, "announcement_type:"), 10, 64); err == nil {
				if t, err := s.store.GetAnnouncementType(id); err == nil && t != nil {
					label = "Announcement type: " + t.Name
				}
			}
		}
		out = append(out, model.DiscordImportRow{Source: w.Source, Label: label, Where: where, Status: w.Status,
			Detail: w.Detail, ChannelID: w.ChannelID, SettledAt: w.SettledAt})
	}
	return out, nil
}

func (s *Server) writeImportReport(w http.ResponseWriter) {
	rows, err := s.importReport()
	if err != nil {
		writeInternalError(w, "read discord import report", err)
		return
	}
	writeJSON(w, http.StatusOK, model.DiscordImportResponse{Rows: rows})
}

// handleDiscordImport serves the webhook import report.
//
//	Endpoint:  GET /api/discord/import
//	Auth:      admin
//	Response:  model.DiscordImportResponse
func (s *Server) handleDiscordImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	s.writeImportReport(w)
}

// handleDiscordImportRetry looks up any webhook that couldn't be reached before.
//
//	Endpoint:  POST /api/discord/import/retry
//	Auth:      admin
//	Response:  model.DiscordImportResponse
func (s *Server) handleDiscordImportRetry(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	s.importLegacyWebhooks(ctx)
	slog.Info("discord webhook import retried", "by", s.actorName(r))
	s.writeImportReport(w)
}

// handleDiscordLegacyCleanup deletes the old webhooks from Discord - those imported from
// the bot's own server - and clears the report. The rest (another server's, or already
// gone) are left for someone with access to delete by hand. Irreversible: the UI confirms
// first. The dry-run valve suppresses the deletes but still clears the report.
//
//	Endpoint:  DELETE /api/discord/legacy-webhooks
//	Auth:      admin
//	Response:  model.DiscordLegacyCleanupResponse
func (s *Server) handleDiscordLegacyCleanup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	rows, err := s.store.LegacyWebhooks()
	if err != nil {
		writeInternalError(w, "read discord import report", err)
		return
	}
	c := s.discord.Client()
	guildID := s.discordSetting(settingDiscordGuild)
	var out model.DiscordLegacyCleanupResponse
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	for _, row := range rows {
		if row.Status != model.ImportImported || row.GuildID != guildID || row.WebhookID == "" || c == nil {
			out.Skipped++
			continue
		}
		if s.discordDryRun {
			slog.Warn("discord webhook delete SUPPRESSED (dry run)", "source", row.Source)
			out.Skipped++
			continue
		}
		switch err := c.DeleteWebhook(ctx, row.WebhookID); {
		case err == nil, errors.Is(err, discord.ErrNotFound):
			out.Deleted++
		default:
			slog.Warn("discord webhook delete failed", "source", row.Source, "error", err)
			out.Failed++
		}
	}
	if err := s.store.ClearLegacyWebhooks(); err != nil {
		writeInternalError(w, "clear discord import report", err)
		return
	}
	slog.Info("discord old webhooks removed", "deleted", out.Deleted, "skipped", out.Skipped,
		"failed", out.Failed, "by", s.actorName(r))
	writeJSON(w, http.StatusOK, out)
}
