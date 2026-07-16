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
/// Stamp Rally operator panel. One object backs two sidebar pages that share the
/// picked rally + its detail (GET /api/stamp-rallies/{id} returns the stalls and
/// issued cards; the collected log is a separate GET .../logs):
/// <list type="bullet">
/// <item><see cref="DrawManage"/> — issue a participant card (nearby-player
/// quick-fill, optional /tell, copy link) and pause/resume individual stalls.</item>
/// <item><see cref="DrawLog"/> — the event-wide collected-stamp log.</item>
/// </list>
/// Rallies aren't pushed over the WebSocket, so a Refresh button re-pulls the list,
/// the detail, and the log.
/// </summary>
internal sealed class StampRallyTab : TabBase
{
    private readonly ApiClient api;
    private readonly NearbyPlayers nearby;
    private readonly Configuration config;
    private readonly ChatSender chat;

    private List<StampRally> rallies = new();
    private long selectedRallyId;
    private StampRallyDetailResponse? detail;
    private List<StampRallyLogEntry> logs = new();

    private string newParticipantName = string.Empty;
    private string pendingTellName = string.Empty;
    private string pendingTellWorld = string.Empty;

    public StampRallyTab(ApiClient api, NearbyPlayers nearby, Configuration config, ChatSender chat)
    {
        this.api = api;
        this.nearby = nearby;
        this.config = config;
        this.chat = chat;
    }

    /// <summary>Reloads the rally list and, if one is selected, its detail + log.</summary>
    protected override async Task LoadAsync()
    {
        var selected = this.selectedRallyId;
        var listRes = await this.api.ListStampRalliesAsync();
        var detailRes = selected != 0 ? await this.api.GetStampRallyAsync(selected) : null;
        var logsRes = selected != 0 ? await this.api.StampRallyLogsAsync(selected) : null;
        await Apply(() =>
        {
            this.rallies = listRes.StampRallies;
            // Install only if the selection hasn't moved on (guards a stale fetch from
            // overwriting a newer selection's detail/log).
            if (this.selectedRallyId == selected)
            {
                if (detailRes != null)
                    this.detail = detailRes;
                if (logsRes != null)
                    this.logs = logsRes.Logs;
            }
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
            ImGui.TextDisabled(this.selectedRallyId == 0 ? "Select a stamp rally." : "Loading…");
            return;
        }

        ImGui.Separator();
        DrawHeader(d);
        ImGui.Spacing();

        var open = string.Equals(d.StampRally.Status, "open", StringComparison.OrdinalIgnoreCase);
        if (open)
        {
            DrawIssueCardForm();
        }
        else
        {
            ImGui.TextDisabled("This rally is closed — no new cards can be issued.");
        }
        ImGui.Spacing();

        DrawCards(d.Cards);
        ImGui.Spacing();
        DrawStalls(d.StampRally.Stamps);
    }

    private void DrawIssueCardForm()
    {
        ImGui.SetNextItemWidth(220);
        ImGui.InputTextWithHint("##rallyname", "Participant name", ref this.newParticipantName, 64);
        ImGui.SameLine();
        DrawNearbyPicker();

        var canCreate = !string.IsNullOrWhiteSpace(this.newParticipantName);
        if (!canCreate)
            ImGui.BeginDisabled();
        if (ImGui.Button("Issue card"))
            RunCreateCard();
        if (!canCreate)
            ImGui.EndDisabled();
    }

    private void RunCreateCard()
    {
        var id = this.selectedRallyId;
        var name = this.newParticipantName.Trim();
        if (id == 0 || name.Length == 0)
            return;

        // Only /tell when the name came from the nearby picker (so we have a world)
        // and it still matches. Opt-in via settings.
        var doTell = this.config.TellStampCardUrlOnCreate
                     && !string.IsNullOrEmpty(this.pendingTellWorld)
                     && string.Equals(this.pendingTellName, name, StringComparison.Ordinal);
        var tellWorld = this.pendingTellWorld;

        Run(async () =>
        {
            var created = (await this.api.CreateStampRallyCardAsync(id, name)).Card;
            var d = await this.api.GetStampRallyAsync(id);
            await Apply(() =>
            {
                if (this.selectedRallyId == id)
                    this.detail = d;
                this.newParticipantName = string.Empty;
                this.pendingTellName = string.Empty;
                this.pendingTellWorld = string.Empty;
            });

            if (doTell && !string.IsNullOrEmpty(created.Token))
            {
                var parts = TellComposer.Compose(this.config.StampCardTellTemplate, new Dictionary<string, string>
                {
                    [TellComposer.TargetToken] = name,
                    [TellComposer.StampCardLinkToken] = this.config.StampCardUrl(created.Token),
                });
                this.chat.SendTell(name, tellWorld, parts);
            }
        });
    }

    private void DrawCards(List<StampRallyCard> cards)
    {
        ImGui.TextDisabled($"{cards.Count} card(s)");
        if (cards.Count == 0)
        {
            ImGui.TextDisabled("No cards issued yet.");
            return;
        }

        if (!ImGui.BeginTable("rallycards", 4,
                ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY,
                new Vector2(0, 200)))
            return;

        ImGui.TableSetupColumn("Participant");
        ImGui.TableSetupColumn("Stamps", ImGuiTableColumnFlags.WidthFixed, 70);
        ImGui.TableSetupColumn("Done", ImGuiTableColumnFlags.WidthFixed, 50);
        ImGui.TableSetupColumn("##actions", ImGuiTableColumnFlags.WidthFixed, 100);
        ImGui.TableHeadersRow();

        foreach (var c in cards)
        {
            ImGui.TableNextRow();
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(c.ParticipantName) ? "—" : c.ParticipantName);
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(c.CollectedCount.ToString());
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(c.Completed ? "✓" : "—");
            ImGui.TableNextColumn();
            if (ImGui.SmallButton($"Copy link##rc{c.Id}"))
                ImGui.SetClipboardText(this.config.StampCardUrl(c.Token));
        }

        ImGui.EndTable();
    }

    private void DrawStalls(List<StampRallyStamp> stamps)
    {
        var active = stamps.Count(s => !s.Paused);
        ImGui.TextDisabled($"{active}/{stamps.Count} stall(s) active");
        if (stamps.Count == 0)
        {
            ImGui.TextDisabled("This rally has no stalls.");
            return;
        }

        if (!ImGui.BeginTable("rallystalls", 3,
                ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY,
                new Vector2(0, 200)))
            return;

        ImGui.TableSetupColumn("Stall");
        ImGui.TableSetupColumn("Status", ImGuiTableColumnFlags.WidthFixed, 80);
        ImGui.TableSetupColumn("##actions", ImGuiTableColumnFlags.WidthFixed, 90);
        ImGui.TableHeadersRow();

        foreach (var s in stamps.OrderBy(s => s.SortOrder))
        {
            ImGui.TableNextRow();
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(StallName(s.AffiliateName));
            ImGui.TableNextColumn();
            if (s.Paused)
                ImGui.TextColored(new Vector4(0.85f, 0.55f, 0.2f, 1f), "Paused");
            else
                ImGui.TextColored(new Vector4(0.3f, 0.85f, 0.35f, 1f), "Active");
            ImGui.TableNextColumn();
            if (ImGui.SmallButton($"{(s.Paused ? "Resume" : "Pause")}##stall{s.Id}"))
                SetStallPaused(s.Id, !s.Paused);
        }

        ImGui.EndTable();
    }

    private void SetStallPaused(long stampId, bool paused)
    {
        var id = this.selectedRallyId;
        if (id == 0)
            return;
        Run(async () =>
        {
            await this.api.SetStampPausedAsync(id, stampId, paused);
            var d = await this.api.GetStampRallyAsync(id);
            await Apply(() =>
            {
                if (this.selectedRallyId == id)
                    this.detail = d;
            });
        });
    }

    // ── Log page ───────────────────────────────────────────────────────────────

    public void DrawLog()
    {
        DrawStatusLine();
        DrawPickerRow();

        if (this.detail == null)
        {
            ImGui.TextDisabled(this.selectedRallyId == 0 ? "Select a stamp rally." : "Loading…");
            return;
        }

        ImGui.Separator();
        DrawHeader(this.detail);
        ImGui.Spacing();

        ImGui.TextDisabled($"{this.logs.Count} collected stamp(s)");
        if (this.logs.Count == 0)
        {
            ImGui.TextDisabled("No stamps collected yet.");
            return;
        }

        if (!ImGui.BeginTable("rallylog", 3,
                ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY | ImGuiTableFlags.Resizable))
            return;

        ImGui.TableSetupColumn("Participant");
        ImGui.TableSetupColumn("Stall");
        ImGui.TableSetupColumn("When", ImGuiTableColumnFlags.WidthFixed, 150);
        ImGui.TableHeadersRow();

        // Rows arrive grouped by participant (then time) from the server.
        foreach (var e in this.logs)
        {
            ImGui.TableNextRow();
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(e.ParticipantName) ? "—" : e.ParticipantName);
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(StallName(e.StallName));
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(FormatTime(e.StampedAt));
        }

        ImGui.EndTable();
    }

    // ── Shared ───────────────────────────────────────────────────────────────

    private void DrawPickerRow()
    {
        if (ImGui.Button("Refresh##rally"))
            Run(LoadAsync);
        ImGui.SameLine();
        DrawRallyPicker();
    }

    private void DrawRallyPicker()
    {
        var current = this.rallies.FirstOrDefault(r => r.Id == this.selectedRallyId);
        var preview = current != null ? $"{current.Title} ({current.Status})" : "Select stamp rally…";

        // Lock selection while a load/action is in flight so the picked rally and the
        // loaded detail/log can't diverge (see the same guard on the Garapon picker).
        if (this.Busy)
            ImGui.BeginDisabled();
        ImGui.SetNextItemWidth(280);
        if (ImGui.BeginCombo("##rallypick", preview))
        {
            foreach (var r in this.rallies)
            {
                if (ImGui.Selectable($"{r.Title} ({r.Status})##sr{r.Id}", r.Id == this.selectedRallyId))
                    LoadRally(r.Id);
            }
            ImGui.EndCombo();
        }
        if (this.Busy)
            ImGui.EndDisabled();
    }

    private void LoadRally(long id)
    {
        this.selectedRallyId = id;
        this.detail = null;         // clear stale detail/log; the body shows "Loading…"
        this.logs = new();          // until the new rally's data arrives
        Run(async () =>
        {
            var d = await this.api.GetStampRallyAsync(id);
            var l = await this.api.StampRallyLogsAsync(id);
            await Apply(() =>
            {
                if (this.selectedRallyId == id)
                {
                    this.detail = d;
                    this.logs = l.Logs;
                }
            });
        });
    }

    private static void DrawHeader(StampRallyDetailResponse d)
    {
        var completed = d.Cards.Count(c => c.Completed);
        var active = d.StampRally.Stamps.Count(s => !s.Paused);
        ImGui.Text(d.StampRally.Title);
        ImGui.SameLine();
        ImGui.TextDisabled($"— {d.StampRally.Status}");
        ImGui.TextDisabled($"{d.Cards.Count} card(s), {completed} completed  •  {active}/{d.StampRally.Stamps.Count} stall(s) active");
    }

    private void DrawNearbyPicker()
    {
        if (!ImGui.BeginCombo("##rallynearby", "Nearby…", ImGuiComboFlags.NoArrowButton))
            return;
        foreach (var np in this.nearby.Snapshot())
        {
            if (ImGui.Selectable($"{np.Name} ({np.World})"))
            {
                this.newParticipantName = np.Name;
                this.pendingTellName = np.Name;
                this.pendingTellWorld = np.World;
            }
        }
        ImGui.EndCombo();
    }

    /// <summary>A stall with no affiliate is the Senpan Tea House default.</summary>
    private static string StallName(string affiliateName)
        => string.IsNullOrWhiteSpace(affiliateName) ? "Senpan Tea House" : affiliateName;

    private static string FormatTime(string ts)
    {
        if (string.IsNullOrWhiteSpace(ts))
            return "—";
        var normalized = ts.Contains('T') ? ts : ts.Replace(' ', 'T') + "Z";
        return DateTimeOffset.TryParse(normalized, out var dto)
            ? dto.ToLocalTime().ToString("yyyy-MM-dd HH:mm")
            : ts;
    }
}
