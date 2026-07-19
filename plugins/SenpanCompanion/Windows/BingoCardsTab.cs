using System;
using System.Collections.Generic;
using System.Linq;
using System.Numerics;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using SenpanCompanion.Api;
using SenpanCompanion.Services;

namespace SenpanCompanion.Windows;

/// <summary>
/// Bingo Cards tab: create named cards (with a nearby-player picker that can also
/// /tell the player their card URL), copy a card's URL, and delete cards. The card
/// list is the shared <see cref="CardCache"/> (newest-first), so the Game tab's
/// winner names and this tab stay in sync and refresh once on a WebSocket card
/// change. Creating a card for a character who already has one prompts to replace it.
/// </summary>
internal sealed class BingoCardsTab : TabBase
{
    // Muted grey for the "pending approval" star (vs. gold for an approved custom card).
    private static readonly Vector4 PendingColor = new(0.62f, 0.62f, 0.62f, 1f);

    private readonly ApiClient api;
    private readonly NearbyPlayers nearby;
    private readonly Configuration config;
    private readonly ChatSender chat;
    private readonly CardCache cardCache;

    private string newPlayerName = string.Empty;
    private string pendingTellName = string.Empty;
    private string pendingTellWorld = string.Empty;

    // Staged create request (set on button click; run directly, or after a
    // replace-confirm when the character already has a card).
    private bool openReplacePopup;
    private string createName = string.Empty;
    private string createTellWorld = string.Empty;
    private bool createDoTell;
    private List<string> replaceIds = new();

    public BingoCardsTab(ApiClient api, NearbyPlayers nearby, Configuration config, ChatSender chat, CardCache cardCache)
    {
        this.api = api;
        this.nearby = nearby;
        this.config = config;
        this.chat = chat;
        this.cardCache = cardCache;
    }

    public void Draw()
    {
        DrawStatusLine();

        var cards = this.cardCache.Cards;

        if (Ui.Button("Refresh##cards"))
            this.cardCache.Refresh();
        ImGui.SameLine();
        if (Ui.DangerButton("Delete all"))
            Run(async () =>
            {
                await this.api.DeleteAllCardsAsync();
                await this.cardCache.RefreshAsync();
            });

        Ui.Section(FontAwesomeIcon.Plus, "Issue a card");
        ImGui.SetNextItemWidth(220);
        ImGui.InputTextWithHint("##playername", "Player name", ref this.newPlayerName, 64);
        ImGui.SameLine();
        DrawNearbyPicker();
        // Create on its own line so it's never pushed off-screen on a narrow window.
        if (Ui.PrimaryButton("Create card") && !string.IsNullOrWhiteSpace(this.newPlayerName))
            OnCreateClicked();

        Ui.Section(FontAwesomeIcon.ThLarge, $"Cards ({cards.Count})");

        if (cards.Count == 0)
        {
            ImGui.TextDisabled(this.cardCache.LoadFailed ? "Couldn't load cards." : "No cards yet.");
        }
        else if (ImGui.BeginTable("cards", 4, ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY, new Vector2(0, 360)))
        {
            ImGui.TableSetupColumn("Card ID");
            ImGui.TableSetupColumn("Player");
            ImGui.TableSetupColumn("Status", ImGuiTableColumnFlags.WidthFixed, 70);
            ImGui.TableSetupColumn("##actions", ImGuiTableColumnFlags.WidthFixed, 150);
            ImGui.TableHeadersRow();

            foreach (var card in cards)
            {
                ImGui.TableNextRow();
                ImGui.TableNextColumn();
                ImGui.TextUnformatted(card.Id);
                ImGui.TableNextColumn();
                ImGui.TextUnformatted(string.IsNullOrEmpty(card.PlayerName) ? "—" : card.PlayerName);
                ImGui.TableNextColumn();
                DrawStatusIcons(card);
                ImGui.TableNextColumn();
                DrawRowActions(card);
            }

            ImGui.EndTable();
        }

        DrawReplacePopup();
    }

    /// <summary>Status glyphs for a card: a pending/approved custom star and/or a Protected lock.</summary>
    private static void DrawStatusIcons(CardListEntry card)
    {
        var drewAny = false;

        if (card.CustomStatus == "pending")
        {
            StatusIcon(FontAwesomeIcon.Star, PendingColor, CustomTooltip(card, "Pending approval"));
            drewAny = true;
        }
        else if (card.CustomStatus == "approved")
        {
            StatusIcon(FontAwesomeIcon.Star, Ui.WarnColor, CustomTooltip(card, "Approved custom card"));
            drewAny = true;
        }

        if (card.Protected)
        {
            if (drewAny)
                ImGui.SameLine();
            StatusIcon(FontAwesomeIcon.Lock, Ui.AccentColor, "Protected — kept when deleting all cards");
            drewAny = true;
        }

        if (!drewAny)
            ImGui.TextDisabled("—");
    }

    /// <summary>Render a status glyph with a hover tooltip (no click).</summary>
    private static void StatusIcon(FontAwesomeIcon icon, Vector4 color, string tooltip)
    {
        Ui.Icon(icon, color);
        if (ImGui.IsItemHovered())
            ImGui.SetTooltip(tooltip);
    }

    /// <summary>Appends the requester's character @ world to a custom-card status tooltip.</summary>
    private static string CustomTooltip(CardListEntry card, string label)
    {
        var who = card.PlayerName;
        if (!string.IsNullOrEmpty(card.World))
            who = string.IsNullOrEmpty(who) ? card.World : $"{who} @ {card.World}";
        return string.IsNullOrEmpty(who) ? label : $"{label} — {who}";
    }

    /// <summary>Row action buttons: approve (pending only), protect toggle, copy URL, delete.</summary>
    private void DrawRowActions(CardListEntry card)
    {
        var id = card.Id;

        // Approve — only shown for a pending custom-card request.
        if (card.CustomStatus == "pending")
        {
            if (Ui.IconButton($"approve{id}", FontAwesomeIcon.Check, "Approve custom card"))
                Run(async () =>
                {
                    await this.api.ApproveCardAsync(id);
                    await this.cardCache.RefreshAsync();
                });
            ImGui.SameLine();
        }

        // Protect / unprotect toggle.
        var isProtected = card.Protected;
        if (Ui.IconButton($"prot{id}", isProtected ? FontAwesomeIcon.LockOpen : FontAwesomeIcon.Lock,
                isProtected ? "Unprotect (allow Delete all)" : "Protect from Delete all"))
            Run(async () =>
            {
                await this.api.SetCardProtectedAsync(id, !isProtected);
                await this.cardCache.RefreshAsync();
            });
        ImGui.SameLine();

        if (Ui.IconButton($"copy{id}", FontAwesomeIcon.Copy, "Copy card URL"))
            ImGui.SetClipboardText(this.config.CardUrl(id));
        ImGui.SameLine();

        if (Ui.DangerIconButton($"del{id}", FontAwesomeIcon.Trash, "Delete card"))
            Run(async () =>
            {
                await this.api.DeleteCardAsync(id);
                await this.cardCache.RefreshAsync();
            });
    }

    private void OnCreateClicked()
    {
        var name = this.newPlayerName.Trim();
        if (name.Length == 0)
            return;

        this.createName = name;
        this.createTellWorld = this.pendingTellWorld;
        this.createDoTell = this.config.TellCardUrlOnCreate
                            && !string.IsNullOrEmpty(this.pendingTellWorld)
                            && string.Equals(this.pendingTellName, name, StringComparison.Ordinal);

        // Does this character already have a card? If so, confirm a replace rather
        // than silently creating a duplicate.
        this.replaceIds = this.cardCache.Cards
            .Where(c => string.Equals(c.PlayerName.Trim(), name, StringComparison.OrdinalIgnoreCase))
            .Select(c => c.Id)
            .ToList();

        if (this.replaceIds.Count > 0)
            this.openReplacePopup = true;
        else
            RunCreate(false);
    }

    private void RunCreate(bool replaceExisting)
    {
        var name = this.createName;
        var doTell = this.createDoTell;
        var tellWorld = this.createTellWorld;
        var toDelete = replaceExisting ? this.replaceIds.ToList() : new List<string>();

        Run(async () =>
        {
            foreach (var id in toDelete)
                await this.api.DeleteCardAsync(id);
            var created = await this.api.CreateNamedCardAsync(name);
            await this.cardCache.RefreshAsync();
            await Apply(() =>
            {
                this.newPlayerName = string.Empty;
                this.pendingTellName = string.Empty;
                this.pendingTellWorld = string.Empty;
            });

            if (doTell && !string.IsNullOrEmpty(created.Card.Id))
            {
                var parts = TellComposer.Compose(this.config.BingoCardTellTemplate, new Dictionary<string, string>
                {
                    [TellComposer.TargetToken] = name,
                    [TellComposer.BingoCardLinkToken] = this.config.CardUrl(created.Card.Id),
                });
                this.chat.SendTell(name, tellWorld, parts);
            }
        });
    }

    private void DrawReplacePopup()
    {
        if (this.openReplacePopup)
        {
            ImGui.OpenPopup("Replace card###replacecard");
            this.openReplacePopup = false;
        }

        ImGui.SetNextWindowSize(new Vector2(360, 0), ImGuiCond.Appearing);
        var open = true;
        if (!ImGui.BeginPopupModal("Replace card###replacecard", ref open))
            return;

        var plural = this.replaceIds.Count != 1;
        ImGui.TextWrapped(
            $"{this.createName} already has {(plural ? $"{this.replaceIds.Count} cards" : "a card")} " +
            $"({string.Join(", ", this.replaceIds)}). This will delete {(plural ? "them" : "it")} " +
            "and create a new one. Proceed?");
        ImGui.Spacing();
        if (Ui.DangerButton("Replace"))
        {
            RunCreate(true);
            ImGui.CloseCurrentPopup();
        }
        ImGui.SameLine();
        if (Ui.Button("Cancel##replace"))
            ImGui.CloseCurrentPopup();
        ImGui.EndPopup();
    }

    private void DrawNearbyPicker()
    {
        if (!ImGui.BeginCombo("##cardnearby", "Nearby…", ImGuiComboFlags.NoArrowButton))
            return;
        foreach (var np in this.nearby.Snapshot())
        {
            if (ImGui.Selectable($"{np.Name} ({np.World})"))
            {
                this.newPlayerName = np.Name;
                this.pendingTellName = np.Name;
                this.pendingTellWorld = np.World;
            }
        }
        ImGui.EndCombo();
    }
}
