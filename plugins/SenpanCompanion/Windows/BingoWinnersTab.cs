using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using Newtonsoft.Json;
using SenpanCompanion.Api;
using SenpanCompanion.Core;

namespace SenpanCompanion.Windows;

/// <summary>
/// Bingo Winners tab: the winners log (most recent first) with player, card, time,
/// and winning patterns, plus per-entry delete. Read-through to GET /api/v1/winners-log;
/// gated server-side by the bingo-winners-log permission, and read again when a game
/// ends or the log changes anywhere. There is intentionally no "clear all", so the log
/// can't be wiped from in-game.
/// </summary>
internal sealed class BingoWinnersTab : TabBase
{
    private const int PageSize = 200;

    private readonly SenpanApi api;

    private List<WinnersLogEntry> entries = new();
    private long total;

    public BingoWinnersTab(SenpanApi api) => this.api = api;

    protected override async Task LoadAsync()
    {
        var res = await this.api.ListWinnersAsync(1, PageSize, Sort.LoggedAt, Dir.Desc);
        await Apply(() =>
        {
            this.entries = res.Entries.ToList();
            this.total = res.Total;
        });
    }

    public void Draw()
    {
        this.DrawStatusLine();

        Ui.Section(FontAwesomeIcon.Trophy, "Winners log");
        if (Ui.Button("Refresh##winlog"))
            this.Run(this.LoadAsync);
        ImGui.SameLine();
        ImGui.TextDisabled(this.total > this.entries.Count
            ? $"showing {this.entries.Count} of {this.total}"
            : $"{this.total} entr{(this.total == 1 ? "y" : "ies")}");

        if (this.entries.Count == 0)
        {
            ImGui.TextDisabled("No winners logged yet.");
            return;
        }

        if (!ImGui.BeginTable("winlog", 5,
                ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY | ImGuiTableFlags.Resizable))
        {
            return;
        }

        ImGui.TableSetupColumn("When", ImGuiTableColumnFlags.WidthFixed, 150);
        ImGui.TableSetupColumn("Player");
        ImGui.TableSetupColumn("Card", ImGuiTableColumnFlags.WidthFixed, 70);
        ImGui.TableSetupColumn("Patterns");
        ImGui.TableSetupColumn("##actions", ImGuiTableColumnFlags.WidthFixed, 60);
        ImGui.TableHeadersRow();

        foreach (var e in this.entries)
        {
            ImGui.TableNextRow();
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(Times.Local(e.LoggedAt));
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(e.PlayerName) ? "-" : e.PlayerName);
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(e.CardId);
            ImGui.TableNextColumn();
            ImGui.TextWrapped(FormatPatterns(e.WinningPatterns));
            ImGui.TableNextColumn();
            if (Ui.DangerIconButton($"del{e.Id}", FontAwesomeIcon.Trash, "Delete entry"))
            {
                var id = e.Id;
                this.Run(async () =>
                {
                    await this.api.DeleteWinnerAsync(id);
                    await this.LoadAsync();
                });
            }
        }

        ImGui.EndTable();
    }

    // The winning patterns are kept as a JSON array of their names.
    private static string FormatPatterns(string json)
    {
        if (string.IsNullOrWhiteSpace(json))
            return string.Empty;
        try
        {
            var names = JsonConvert.DeserializeObject<string[]>(json);
            return names != null ? string.Join(", ", names) : json;
        }
        catch (JsonException)
        {
            return json;
        }
    }
}
