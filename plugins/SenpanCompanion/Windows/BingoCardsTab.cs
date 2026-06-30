using System;
using System.Collections.Generic;
using System.Linq;
using System.Numerics;
using Dalamud.Bindings.ImGui;
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

        if (ImGui.Button("Refresh##cards"))
            this.cardCache.Refresh();
        ImGui.SameLine();
        if (ImGui.Button("Delete all"))
            Run(async () =>
            {
                await this.api.DeleteAllCardsAsync();
                await this.cardCache.RefreshAsync();
            });

        ImGui.SetNextItemWidth(220);
        ImGui.InputTextWithHint("##playername", "Player name", ref this.newPlayerName, 64);
        ImGui.SameLine();
        DrawNearbyPicker();
        // Create on its own line so it's never pushed off-screen on a narrow window.
        if (ImGui.Button("Create card") && !string.IsNullOrWhiteSpace(this.newPlayerName))
            OnCreateClicked();

        ImGui.TextDisabled($"{cards.Count} card(s)");

        if (cards.Count == 0)
        {
            ImGui.TextDisabled(this.cardCache.LoadFailed ? "Couldn't load cards." : "No cards yet.");
        }
        else if (ImGui.BeginTable("cards", 3, ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY, new Vector2(0, 360)))
        {
            ImGui.TableSetupColumn("Card ID");
            ImGui.TableSetupColumn("Player");
            ImGui.TableSetupColumn("##actions", ImGuiTableColumnFlags.WidthFixed, 130);
            ImGui.TableHeadersRow();

            foreach (var card in cards)
            {
                ImGui.TableNextRow();
                ImGui.TableNextColumn();
                ImGui.TextUnformatted(card.Id);
                ImGui.TableNextColumn();
                ImGui.TextUnformatted(string.IsNullOrEmpty(card.PlayerName) ? "—" : card.PlayerName);
                ImGui.TableNextColumn();
                if (ImGui.SmallButton($"Copy URL##{card.Id}"))
                    ImGui.SetClipboardText(this.config.CardUrl(card.Id));
                ImGui.SameLine();
                if (ImGui.SmallButton($"Delete##{card.Id}"))
                {
                    var id = card.Id;
                    Run(async () =>
                    {
                        await this.api.DeleteCardAsync(id);
                        await this.cardCache.RefreshAsync();
                    });
                }
            }

            ImGui.EndTable();
        }

        DrawReplacePopup();
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
                this.chat.SendTell(name, tellWorld, $"Here's your bingo card: {this.config.CardUrl(created.Card.Id)}");
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
        if (ImGui.Button("Replace"))
        {
            RunCreate(true);
            ImGui.CloseCurrentPopup();
        }
        ImGui.SameLine();
        if (ImGui.Button("Cancel##replace"))
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
