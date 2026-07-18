using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using System.Numerics;
using System.Threading.Tasks;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using SenpanCompanion.Api;
using SenpanCompanion.Services;

namespace SenpanCompanion.Windows;

/// <summary>
/// Garapon operator panel. One object backs two sidebar pages that share the
/// picked garapon + its detail (a single GET /api/garapons/{id} returns the drawing
/// links and the full draw log):
/// <list type="bullet">
/// <item><see cref="DrawManage"/> — issue a per-player drawing link (nearby-player
/// quick-fill, optional /tell, copy link + the paired stamp-card link). Deliberately
/// create-only: no edit/delete, mirroring "limited to creating new entries".</item>
/// <item><see cref="DrawLog"/> — the read-only draw log for the picked garapon.</item>
/// </list>
/// When the garapon is linked to an open Stamp Rally, the server auto-issues the
/// paired stamp card (same token) on create — the plugin makes one call and reads
/// back player.stamp_card_token. Garapons aren't pushed over the WebSocket, so a
/// Refresh button re-pulls the list + detail.
/// </summary>
internal sealed class GaraponTab : TabBase
{
    private readonly ApiClient api;
    private readonly NearbyPlayers nearby;
    private readonly Configuration config;
    private readonly ChatSender chat;

    private List<Garapon> garapons = new();
    private long selectedGaraponId;
    private GaraponDetailResponse? detail;

    private string newPlayerName = string.Empty;
    private int newMaxDraws = 1;
    private string pendingTellName = string.Empty;
    private string pendingTellWorld = string.Empty;

    public GaraponTab(ApiClient api, NearbyPlayers nearby, Configuration config, ChatSender chat)
    {
        this.api = api;
        this.nearby = nearby;
        this.config = config;
        this.chat = chat;
    }

    /// <summary>Reloads the garapon list and, if one is selected, its detail.</summary>
    protected override async Task LoadAsync()
    {
        var selected = this.selectedGaraponId;
        var listRes = await this.api.ListGaraponsAsync();
        var detailRes = selected != 0 ? await this.api.GetGaraponAsync(selected) : null;
        await Apply(() =>
        {
            this.garapons = listRes.Garapons;
            // Install only if the selection hasn't moved on (guards against a stale
            // fetch overwriting a newer selection's detail).
            if (detailRes != null && this.selectedGaraponId == selected)
                this.detail = detailRes;
        });
    }

    // ── Manage page ────────────────────────────────────────────────────────────

    public void DrawManage()
    {
        DrawStatusLine();
        DrawPickerRow();

        var d = this.detail;
        if (d == null)
        {
            ImGui.TextDisabled(this.selectedGaraponId == 0 ? "Select a garapon." : "Loading…");
            return;
        }

        ImGui.Separator();
        DrawHeader(d.Garapon);

        var open = string.Equals(d.Garapon.Status, "open", StringComparison.OrdinalIgnoreCase);
        Ui.Section(FontAwesomeIcon.Plus, "Issue a drawing link");
        if (open)
            DrawCreateForm();
        else
            UiText.WrappedDisabled("This garapon is closed — no new drawing links can be issued.");

        Ui.Section(FontAwesomeIcon.Link, $"Drawing links ({d.Players.Count})");
        DrawPlayers(d.Players);
    }

    private void DrawCreateForm()
    {
        ImGui.SetNextItemWidth(220);
        ImGui.InputTextWithHint("##garaponname", "Player name", ref this.newPlayerName, 64);
        ImGui.SameLine();
        DrawNearbyPicker();

        ImGui.SetNextItemWidth(120);
        if (ImGui.InputInt("Draws", ref this.newMaxDraws))
            this.newMaxDraws = Math.Max(1, this.newMaxDraws);

        // On its own line so it's never pushed off-screen on a narrow window.
        var canCreate = !string.IsNullOrWhiteSpace(this.newPlayerName);
        if (!canCreate)
            ImGui.BeginDisabled();
        if (Ui.PrimaryButton("Issue drawing link"))
            RunCreatePlayer();
        if (!canCreate)
            ImGui.EndDisabled();
    }

    private void RunCreatePlayer()
    {
        var id = this.selectedGaraponId;
        var name = this.newPlayerName.Trim();
        if (id == 0 || name.Length == 0)
            return;
        var maxDraws = Math.Max(1, this.newMaxDraws);

        // Only /tell when the name came from the nearby picker (so we have a world)
        // and it still matches — never guess a target. Opt-in via settings.
        var doTell = this.config.TellGaraponUrlOnCreate
                     && !string.IsNullOrEmpty(this.pendingTellWorld)
                     && string.Equals(this.pendingTellName, name, StringComparison.Ordinal);
        var tellWorld = this.pendingTellWorld;

        Run(async () =>
        {
            var created = (await this.api.CreateGaraponPlayerAsync(id, name, maxDraws)).Player;
            var d = await this.api.GetGaraponAsync(id);
            await Apply(() =>
            {
                if (this.selectedGaraponId == id)
                    this.detail = d;
                this.newPlayerName = string.Empty;
                this.newMaxDraws = 1;
                this.pendingTellName = string.Empty;
                this.pendingTellWorld = string.Empty;
            });

            if (doTell && !string.IsNullOrEmpty(created.Token))
            {
                var parts = TellComposer.Compose(this.config.GaraponTellTemplate, new Dictionary<string, string>
                {
                    [TellComposer.TargetToken] = name,
                    [TellComposer.GaraponLinkToken] = this.config.GaraponUrl(created.Token),
                });
                this.chat.SendTell(name, tellWorld, parts);
            }
        });
    }

    private void DrawPlayers(List<GaraponPlayer> players)
    {
        if (players.Count == 0)
        {
            ImGui.TextDisabled("No drawing links issued yet.");
            return;
        }

        if (!ImGui.BeginTable("garaponplayers", 3,
                ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY,
                new Vector2(0, 320)))
            return;

        ImGui.TableSetupColumn("Player");
        ImGui.TableSetupColumn("Draws", ImGuiTableColumnFlags.WidthFixed, 70);
        ImGui.TableSetupColumn("##actions", ImGuiTableColumnFlags.WidthFixed, 90);
        ImGui.TableHeadersRow();

        foreach (var p in players)
        {
            ImGui.TableNextRow();
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(p.PlayerName) ? "—" : p.PlayerName);
            ImGui.TableNextColumn();
            ImGui.TextUnformatted($"{p.DrawsUsed}/{p.MaxDraws}");
            ImGui.TableNextColumn();
            if (Ui.IconButton($"g{p.Id}", FontAwesomeIcon.Copy, "Copy drawing link"))
                ImGui.SetClipboardText(this.config.GaraponUrl(p.Token));
            if (!string.IsNullOrEmpty(p.StampCardToken))
            {
                ImGui.SameLine();
                if (Ui.IconButton($"sc{p.Id}", FontAwesomeIcon.Stamp, "Copy stamp card link"))
                    ImGui.SetClipboardText(this.config.StampCardUrl(p.StampCardToken));
            }
        }

        ImGui.EndTable();
    }

    // ── Draw-log page ──────────────────────────────────────────────────────────

    public void DrawLog()
    {
        DrawStatusLine();
        DrawPickerRow();

        var d = this.detail;
        if (d == null)
        {
            ImGui.TextDisabled(this.selectedGaraponId == 0 ? "Select a garapon." : "Loading…");
            return;
        }

        ImGui.Separator();
        DrawHeader(d.Garapon);

        Ui.Section(FontAwesomeIcon.ListOl, $"Draw log ({d.Draws.Count})");
        if (d.Draws.Count == 0)
        {
            ImGui.TextDisabled("No draws recorded yet.");
            return;
        }

        if (!ImGui.BeginTable("garaponlog", 3,
                ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY | ImGuiTableFlags.Resizable))
            return;

        ImGui.TableSetupColumn("When", ImGuiTableColumnFlags.WidthFixed, 150);
        ImGui.TableSetupColumn("Player");
        ImGui.TableSetupColumn("Prize");
        ImGui.TableHeadersRow();

        foreach (var draw in d.Draws)
        {
            ImGui.TableNextRow();
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(FormatTime(draw.DrawnAt));
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(draw.PlayerName) ? "—" : draw.PlayerName);
            ImGui.TableNextColumn();
            ImGui.TextColored(ParseColor(draw.BallColor), "●"); // ● ball swatch
            ImGui.SameLine();
            ImGui.TextUnformatted(draw.PrizeName);
        }

        ImGui.EndTable();
    }

    // ── Shared ───────────────────────────────────────────────────────────────

    private void DrawPickerRow()
    {
        if (Ui.Button("Refresh##garapon"))
            Run(LoadAsync);
        ImGui.SameLine();
        DrawGaraponPicker();
    }

    private void DrawGaraponPicker()
    {
        var current = this.garapons.FirstOrDefault(g => g.Id == this.selectedGaraponId);
        var preview = current != null ? $"{current.Title} ({current.Status})" : "Select garapon…";

        // Lock selection while a load/action is in flight so the picked garapon and the
        // loaded detail can't diverge: TabBase.Run is busy-gated, so a selection made
        // mid-load would drop its fetch and leave the body (and the create target) on a
        // different garapon than the picker shows.
        if (this.Busy)
            ImGui.BeginDisabled();
        ImGui.SetNextItemWidth(280);
        if (ImGui.BeginCombo("##garaponpick", preview))
        {
            foreach (var g in this.garapons)
            {
                if (ImGui.Selectable($"{g.Title} ({g.Status})##g{g.Id}", g.Id == this.selectedGaraponId))
                    LoadGarapon(g.Id);
            }
            ImGui.EndCombo();
        }
        if (this.Busy)
            ImGui.EndDisabled();
    }

    private void LoadGarapon(long id)
    {
        this.selectedGaraponId = id;
        this.detail = null; // clear stale detail; the body shows "Loading…" until it arrives
        Run(async () =>
        {
            var d = await this.api.GetGaraponAsync(id);
            await Apply(() =>
            {
                if (this.selectedGaraponId == id)
                    this.detail = d;
            });
        });
    }

    private static void DrawHeader(Garapon g)
    {
        ImGui.Text(g.Title);
        ImGui.SameLine();
        ImGui.TextDisabled($"— {g.Status}");
        if (!string.IsNullOrEmpty(g.StampRallyTitle))
            UiText.WrappedDisabled($"Linked to Stamp Rally \"{g.StampRallyTitle}\" — a stamp card is issued with each drawing link.");
    }

    private void DrawNearbyPicker()
    {
        if (!ImGui.BeginCombo("##garaponnearby", "Nearby…", ImGuiComboFlags.NoArrowButton))
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

    private static string FormatTime(string ts)
    {
        if (string.IsNullOrWhiteSpace(ts))
            return "—";
        var normalized = ts.Contains('T') ? ts : ts.Replace(' ', 'T') + "Z";
        return DateTimeOffset.TryParse(normalized, out var dto)
            ? dto.ToLocalTime().ToString("yyyy-MM-dd HH:mm")
            : ts;
    }

    /// <summary>Parses a "#rrggbb" ball colour to a Vector4; white on any failure.</summary>
    private static Vector4 ParseColor(string hex)
    {
        var s = hex.Trim().TrimStart('#');
        if (s.Length == 6 &&
            int.TryParse(s.AsSpan(0, 2), NumberStyles.HexNumber, CultureInfo.InvariantCulture, out var r) &&
            int.TryParse(s.AsSpan(2, 2), NumberStyles.HexNumber, CultureInfo.InvariantCulture, out var g) &&
            int.TryParse(s.AsSpan(4, 2), NumberStyles.HexNumber, CultureInfo.InvariantCulture, out var b))
            return new Vector4(r / 255f, g / 255f, b / 255f, 1f);
        return new Vector4(1f, 1f, 1f, 1f);
    }
}
