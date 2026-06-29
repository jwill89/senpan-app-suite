using System;
using System.Numerics;
using Dalamud.Bindings.ImGui;
using SenpanCompanion.Api;
using SenpanCompanion.Services;

namespace SenpanCompanion.Windows;

/// <summary>
/// Bingo Cards tab: create named cards (with a nearby-player picker that can also
/// /tell the player their card URL), copy a card's URL, and delete cards. The card
/// list is the shared <see cref="CardCache"/>, so the Game tab's winner names and
/// this tab stay in sync and refresh once on a WebSocket card change.
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
        ImGui.InputText("##playername", ref this.newPlayerName, 64);
        ImGui.SameLine();
        DrawNearbyPicker();
        ImGui.SameLine();
        if (ImGui.Button("Create card") && !string.IsNullOrWhiteSpace(this.newPlayerName))
            CreateCard();

        ImGui.TextDisabled($"{cards.Count} card(s)");

        if (cards.Count == 0)
        {
            ImGui.TextDisabled(this.cardCache.LoadFailed ? "Couldn't load cards." : "No cards yet.");
            return;
        }

        if (ImGui.BeginTable("cards", 3, ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY, new Vector2(0, 360)))
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
    }

    private void CreateCard()
    {
        var name = this.newPlayerName.Trim();
        var tellName = this.pendingTellName;
        var tellWorld = this.pendingTellWorld;
        var doTell = this.config.TellCardUrlOnCreate
                     && !string.IsNullOrEmpty(tellWorld)
                     && string.Equals(tellName, name, StringComparison.Ordinal);

        Run(async () =>
        {
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
