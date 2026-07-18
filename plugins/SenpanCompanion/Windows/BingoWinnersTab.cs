using System;
using System.Collections.Generic;
using System.Numerics;
using System.Text.Json;
using System.Threading.Tasks;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using SenpanCompanion.Api;

namespace SenpanCompanion.Windows;

/// <summary>
/// Bingo Winners tab: the winners log (most recent first) with player, card, time,
/// and winning patterns, plus per-entry delete. Read-through to GET /api/winners-log;
/// gated server-side by the bingo-winners-log permission. There is intentionally no
/// "clear all", so the log can't be wiped from in-game.
/// </summary>
internal sealed class BingoWinnersTab : TabBase
{
    private const int PageSize = 200;

    private readonly ApiClient api;

    private List<WinnersLogEntry> entries = new();
    private int total;

    public BingoWinnersTab(ApiClient api) => this.api = api;

    protected override async Task LoadAsync()
    {
        var res = await this.api.WinnersLogAsync(1, PageSize);
        await Apply(() =>
        {
            this.entries = res.Entries;
            this.total = res.Total;
        });
    }

    public void Draw()
    {
        DrawStatusLine();

        Ui.Section(FontAwesomeIcon.Trophy, "Winners log");
        if (Ui.Button("Refresh##winlog"))
            Run(LoadAsync);
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
            return;

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
            ImGui.TextUnformatted(FormatTime(e.LoggedAt));
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(e.PlayerName) ? "—" : e.PlayerName);
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(e.CardId);
            ImGui.TableNextColumn();
            ImGui.TextWrapped(FormatPatterns(e.WinningPatterns));
            ImGui.TableNextColumn();
            if (Ui.DangerIconButton($"del{e.Id}", FontAwesomeIcon.Trash, "Delete entry"))
            {
                var id = e.Id;
                Run(async () =>
                {
                    await this.api.DeleteWinnersLogEntryAsync(id);
                    await LoadAsync();
                });
            }
        }

        ImGui.EndTable();
    }

    private static string FormatTime(string ts)
    {
        if (string.IsNullOrWhiteSpace(ts))
            return "—";
        var normalized = ts.Contains('T') ? ts : ts.Replace(' ', 'T') + "Z";
        return DateTimeOffset.TryParse(normalized, out var dto)
            ? dto.ToLocalTime().ToString("yyyy-MM-dd HH:mm")
            : ts;
    }

    private static string FormatPatterns(string json)
    {
        if (string.IsNullOrWhiteSpace(json))
            return string.Empty;
        try
        {
            var names = JsonSerializer.Deserialize<string[]>(json);
            return names != null ? string.Join(", ", names) : json;
        }
        catch (JsonException)
        {
            return json;
        }
    }
}
