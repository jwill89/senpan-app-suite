using System;
using System.Collections.Generic;
using System.Linq;
using System.Numerics;
using System.Threading.Tasks;
using Dalamud.Bindings.ImGui;
using SenpanCompanion.Api;
using SenpanCompanion.Services;

namespace SenpanCompanion.Windows;

/// <summary>
/// Raffle operator panel: pick an open raffle, add entrants (with nearby-player
/// quick-fill), toggle paid status, and draw a winner (pick → confirm, or pick
/// again). Matches the website's admin raffle entry flow; raffle creation stays on
/// the website by design. Raffles aren't broadcast over the WebSocket, so Refresh
/// re-pulls both the list and the open raffle's detail.
/// </summary>
internal sealed class RaffleTab : TabBase
{
    private readonly ApiClient api;
    private readonly NearbyPlayers nearby;

    private List<Raffle> raffles = new();
    private long selectedRaffleId;
    private RaffleDetailResponse? detail;
    private RaffleEntry? pendingWinner;

    private string charName = string.Empty;
    private string world = string.Empty;
    private int numEntries = 1;
    private bool markPaidOnAdd = true;

    public RaffleTab(ApiClient api, NearbyPlayers nearby)
    {
        this.api = api;
        this.nearby = nearby;
    }

    /// <summary>Reloads the raffle list and, if one is open, its detail.</summary>
    protected override async Task LoadAsync()
    {
        var selected = this.selectedRaffleId;
        var rafflesRes = await this.api.ListRafflesAsync();
        var detailRes = selected != 0 ? await this.api.GetRaffleAsync(selected) : null;
        await Apply(() =>
        {
            this.raffles = rafflesRes.Raffles;
            if (detailRes != null)
                this.detail = detailRes;
        });
    }

    public void Draw()
    {
        DrawStatusLine();

        if (ImGui.Button("Refresh##raffles"))
            Run(LoadAsync);
        ImGui.SameLine();
        DrawRafflePicker();

        if (this.detail == null)
        {
            ImGui.TextDisabled("Select a raffle.");
            return;
        }

        ImGui.Separator();
        DrawRaffleHeader(this.detail);
        ImGui.Spacing();
        DrawAddEntry();
        ImGui.Spacing();
        DrawEntries(this.detail);
        ImGui.Spacing();
        DrawWinnerControls();
    }

    private void DrawRafflePicker()
    {
        var current = this.raffles.FirstOrDefault(r => r.Id == this.selectedRaffleId);
        var preview = current != null ? $"{current.Title} ({current.Status})" : "Select raffle…";

        ImGui.SetNextItemWidth(280);
        if (!ImGui.BeginCombo("##rafflepick", preview))
            return;
        foreach (var raffle in this.raffles)
        {
            var selected = raffle.Id == this.selectedRaffleId;
            if (ImGui.Selectable($"{raffle.Title} ({raffle.Status})##r{raffle.Id}", selected))
                LoadRaffle(raffle.Id);
        }
        ImGui.EndCombo();
    }

    private void LoadRaffle(long id)
    {
        this.selectedRaffleId = id;
        this.pendingWinner = null;
        Run(async () =>
        {
            var d = await this.api.GetRaffleAsync(id);
            await Apply(() => this.detail = d);
        });
    }

    private void DrawRaffleHeader(RaffleDetailResponse d)
    {
        ImGui.Text($"{d.Raffle.Title}");
        ImGui.SameLine();
        ImGui.TextDisabled($"— {d.Raffle.Status}, {d.TotalEntries} entr{(d.TotalEntries == 1 ? "y" : "ies")}");
        if (d.Raffle.CostPerEntry > 0)
            ImGui.TextDisabled($"Cost per entry: {d.Raffle.CostPerEntry:0.##}  •  Max per person: {d.Raffle.MaxEntries}");
    }

    private void DrawAddEntry()
    {
        var open = string.Equals(this.detail?.Raffle.Status, "open", StringComparison.OrdinalIgnoreCase);
        if (!open)
        {
            ImGui.TextDisabled("This raffle is closed — entries can't be added.");
            return;
        }

        ImGui.SetNextItemWidth(160);
        ImGui.InputText("Name##entry", ref this.charName, 64);
        ImGui.SameLine();
        ImGui.SetNextItemWidth(140);
        ImGui.InputText("World##entry", ref this.world, 32);
        ImGui.SameLine();
        DrawNearbyPicker();

        ImGui.SetNextItemWidth(120);
        if (ImGui.InputInt("Tickets", ref this.numEntries))
            this.numEntries = Math.Max(1, this.numEntries);
        ImGui.SameLine();
        ImGui.Checkbox("Paid", ref this.markPaidOnAdd);
        ImGui.SameLine();

        var canAdd = !string.IsNullOrWhiteSpace(this.charName) && !string.IsNullOrWhiteSpace(this.world);
        if (!canAdd)
            ImGui.BeginDisabled();
        if (ImGui.Button("Add entrant"))
        {
            var id = this.selectedRaffleId;
            var name = this.charName.Trim();
            var w = this.world.Trim();
            var n = Math.Max(1, this.numEntries);
            var paid = this.markPaidOnAdd;
            Run(async () =>
            {
                await this.api.AddRaffleEntryAsync(id, name, w, n, paid);
                var d = await this.api.GetRaffleAsync(id);
                await Apply(() =>
                {
                    this.detail = d;
                    this.charName = string.Empty;
                    this.world = string.Empty;
                    this.numEntries = 1;
                });
            });
        }
        if (!canAdd)
            ImGui.EndDisabled();
    }

    private void DrawEntries(RaffleDetailResponse d)
    {
        if (d.Entries.Count == 0)
        {
            ImGui.TextDisabled("No entries yet.");
            return;
        }

        if (!ImGui.BeginTable("entries", 5,
                ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY,
                new Vector2(0, 200)))
            return;

        ImGui.TableSetupColumn("Name");
        ImGui.TableSetupColumn("World");
        ImGui.TableSetupColumn("Tickets", ImGuiTableColumnFlags.WidthFixed, 60);
        ImGui.TableSetupColumn("Paid", ImGuiTableColumnFlags.WidthFixed, 50);
        ImGui.TableSetupColumn("##actions", ImGuiTableColumnFlags.WidthFixed, 70);
        ImGui.TableHeadersRow();

        foreach (var entry in d.Entries)
        {
            ImGui.TableNextRow();
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(entry.CharacterName);
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(entry.World);
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(entry.NumEntries.ToString());

            ImGui.TableNextColumn();
            var paid = entry.Paid;
            if (ImGui.Checkbox($"##paid{entry.Id}", ref paid))
            {
                var id = this.selectedRaffleId;
                var entryId = entry.Id;
                var value = paid;
                Run(async () =>
                {
                    await this.api.MarkRaffleEntryPaidAsync(id, entryId, value);
                    var d2 = await this.api.GetRaffleAsync(id);
                    await Apply(() => this.detail = d2);
                });
            }

            ImGui.TableNextColumn();
            if (ImGui.SmallButton($"Delete##e{entry.Id}"))
            {
                var id = this.selectedRaffleId;
                var entryId = entry.Id;
                Run(async () =>
                {
                    await this.api.DeleteRaffleEntryAsync(id, entryId);
                    var d2 = await this.api.GetRaffleAsync(id);
                    await Apply(() => this.detail = d2);
                });
            }
        }

        ImGui.EndTable();
    }

    private void DrawWinnerControls()
    {
        ImGui.Separator();
        if (this.pendingWinner != null)
        {
            ImGui.TextColored(new Vector4(0.3f, 0.9f, 0.4f, 1f),
                $"Drawn winner: {this.pendingWinner.CharacterName} @ {this.pendingWinner.World}");

            if (ImGui.Button("Confirm winner"))
            {
                var id = this.selectedRaffleId;
                Run(async () =>
                {
                    await this.api.VerifyRaffleWinnerAsync(id);
                    var d = await this.api.GetRaffleAsync(id);
                    await Apply(() =>
                    {
                        this.detail = d;
                        this.pendingWinner = null;
                    });
                });
            }
            ImGui.SameLine();
            if (ImGui.Button("Draw another"))
            {
                var id = this.selectedRaffleId;
                Run(async () =>
                {
                    var w = (await this.api.PickAnotherRaffleWinnerAsync(id)).Winner;
                    await Apply(() => this.pendingWinner = w);
                });
            }
            return;
        }

        if (ImGui.Button("Pick a winner"))
        {
            var id = this.selectedRaffleId;
            Run(async () =>
            {
                var w = (await this.api.PickRaffleWinnerAsync(id)).Winner;
                await Apply(() => this.pendingWinner = w);
            });
        }
        ImGui.SameLine();
        ImGui.TextDisabled("Draws from paid entries only.");
    }

    private void DrawNearbyPicker()
    {
        if (!ImGui.BeginCombo("##rafflenearby", "Nearby…", ImGuiComboFlags.NoArrowButton))
            return;
        foreach (var np in this.nearby.Snapshot())
        {
            if (ImGui.Selectable($"{np.Name} ({np.World})"))
            {
                this.charName = np.Name;
                this.world = np.World;
            }
        }
        ImGui.EndCombo();
    }
}
