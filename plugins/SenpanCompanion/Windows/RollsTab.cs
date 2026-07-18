using System;
using System.Collections.Generic;
using System.Linq;
using System.Numerics;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using SenpanCompanion.Services;

namespace SenpanCompanion.Windows;

/// <summary>
/// Rolls tool. A permission-free, server-independent helper that lists the
/// <c>/random</c> · <c>/dice</c> rolls <see cref="RollTracker"/> has seen in chat this
/// session. Laid out as distinct sections — session summary, filters, a Highest / Lowest /
/// Closest-to query, and the results table — so each control cluster reads on its own. A
/// query brings the matching roll(s) to the top and highlights them, with a heads-up when
/// the winner rolled more than once in the window. Nothing here is stored: the log lives
/// only in memory and clears on logout.
/// </summary>
internal sealed class RollsTab
{
    private enum RollQuery
    {
        Off,
        Highest,
        Lowest,
        ClosestTo,
    }

    private static readonly int[] PageSizes = { 15, 30, 45 };

    private static readonly Vector4 WinnerBg = new(0.85f, 0.65f, 0.15f, 0.28f);
    private static readonly Vector4 StarColor = new(1f, 0.84f, 0.2f, 1f);
    private static readonly Vector4 NoteColor = new(0.9f, 0.65f, 0.2f, 1f);

    private readonly RollTracker tracker;

    // Filters + view state.
    private string nameFilter = string.Empty;
    private bool windowEnabled;
    private int windowMinutes = 5;
    private RollQuery query = RollQuery.Off;
    private int closestTarget = 100;
    private int perPage = 15;
    private int page;

    public RollsTab(RollTracker tracker) => this.tracker = tracker;

    public void Draw()
    {
        var all = this.tracker.Snapshot(); // newest-first
        var filtered = ApplyFilters(all);

        // Split into the query's winner(s) and the rest. Winners float to the top so the
        // answer is always on the first page, and each is highlighted in the table.
        var winners = FindWinners(filtered);
        var ordered = this.query == RollQuery.Off
            ? filtered
            : filtered.Where(winners.Contains).Concat(filtered.Where(e => !winners.Contains(e))).ToList();

        Ui.Section(FontAwesomeIcon.Dice, "Captured rolls");
        using (Ui.Body())
            DrawToolbar();

        Ui.Section(FontAwesomeIcon.Filter, "Filter");
        using (Ui.Body())
            DrawFilters();

        Ui.Section(FontAwesomeIcon.Trophy, "Find the winner");
        using (Ui.Body())
        {
            DrawQuery();
            if (this.query != RollQuery.Off)
                DrawResultBanner(filtered, winners);
        }

        Ui.Section(FontAwesomeIcon.ListOl, $"Rolls ({ordered.Count})");
        if (all.Count == 0)
        {
            UiText.WrappedDisabled("No rolls captured yet — they appear here as people use /random or /dice near you.");
            return;
        }
        if (ordered.Count == 0)
        {
            ImGui.TextDisabled("No rolls match the current filters.");
            return;
        }

        DrawPager(ordered.Count);
        DrawTable(ordered, winners);
    }

    // ── Session summary ──────────────────────────────────────────────────────────

    private void DrawToolbar()
    {
        var total = this.tracker.Count;
        ImGui.AlignTextToFramePadding();
        Ui.Help($"{total} roll{(total == 1 ? string.Empty : "s")} this session · kept in memory only");
        ImGui.SameLine();
        Ui.HelpMarker(
            "This tool keeps rolls in memory only — nothing is saved or sent anywhere. The list " +
            "clears when you log out, and is gone entirely once the game closes and the plugin unloads.");
        ImGui.SameLine();
        if (Ui.SmallButton("Clear"))
        {
            this.tracker.Clear();
            this.page = 0;
        }
    }

    // ── Filters ──────────────────────────────────────────────────────────────────

    private void DrawFilters()
    {
        ImGui.SetNextItemWidth(220);
        ImGui.InputTextWithHint("##rollname", "Filter by player name", ref this.nameFilter, 64);
        ImGui.SameLine();
        if (Ui.SmallButton("Clear name"))
            this.nameFilter = string.Empty;

        if (ImGui.Checkbox("Only the last", ref this.windowEnabled))
            this.page = 0;
        ImGui.SameLine();
        ImGui.BeginDisabled(!this.windowEnabled);
        ImGui.SetNextItemWidth(90);
        if (ImGui.InputInt("minutes##rollwindow", ref this.windowMinutes))
        {
            this.windowMinutes = Math.Clamp(this.windowMinutes, 1, 1440);
            this.page = 0;
        }
        ImGui.EndDisabled();
    }

    // ── Query ────────────────────────────────────────────────────────────────────

    private void DrawQuery()
    {
        ImGui.SetNextItemWidth(160);
        DrawQueryCombo();
        if (this.query == RollQuery.ClosestTo)
        {
            ImGui.SameLine();
            ImGui.SetNextItemWidth(110);
            if (ImGui.InputInt("target##rollclosest", ref this.closestTarget))
                this.closestTarget = Math.Max(0, this.closestTarget);
        }
    }

    private void DrawQueryCombo()
    {
        if (!ImGui.BeginCombo("Find##rollquery", QueryLabel(this.query)))
            return;
        foreach (var mode in new[] { RollQuery.Off, RollQuery.Highest, RollQuery.Lowest, RollQuery.ClosestTo })
        {
            if (ImGui.Selectable(QueryLabel(mode), this.query == mode))
            {
                this.query = mode;
                this.page = 0;
            }
        }
        ImGui.EndCombo();
    }

    private static string QueryLabel(RollQuery mode) => mode switch
    {
        RollQuery.Off => "Show all",
        RollQuery.Highest => "Highest roll",
        RollQuery.Lowest => "Lowest roll",
        RollQuery.ClosestTo => "Closest to…",
        _ => "Show all",
    };

    // ── Filtering + query ────────────────────────────────────────────────────────

    private List<RollEntry> ApplyFilters(List<RollEntry> rolls)
    {
        IEnumerable<RollEntry> q = rolls;

        if (this.windowEnabled && this.windowMinutes > 0)
        {
            var cutoff = DateTime.Now.AddMinutes(-this.windowMinutes);
            q = q.Where(e => e.Time >= cutoff);
        }

        var needle = this.nameFilter.Trim();
        if (needle.Length > 0)
            q = q.Where(e => e.PlayerName.Contains(needle, StringComparison.OrdinalIgnoreCase));

        return q.ToList();
    }

    /// <summary>
    /// The roll(s) that satisfy the active query within <paramref name="filtered"/>:
    /// the max value, the min value, or the smallest distance to the target. Ties all
    /// win. Empty when no query is active.
    /// </summary>
    private HashSet<RollEntry> FindWinners(List<RollEntry> filtered)
    {
        if (this.query == RollQuery.Off || filtered.Count == 0)
            return new HashSet<RollEntry>();

        switch (this.query)
        {
            case RollQuery.Highest:
                var hi = filtered.Max(e => e.Value);
                return filtered.Where(e => e.Value == hi).ToHashSet();
            case RollQuery.Lowest:
                var lo = filtered.Min(e => e.Value);
                return filtered.Where(e => e.Value == lo).ToHashSet();
            case RollQuery.ClosestTo:
                var best = filtered.Min(e => Math.Abs(e.Value - this.closestTarget));
                return filtered.Where(e => Math.Abs(e.Value - this.closestTarget) == best).ToHashSet();
            default:
                return new HashSet<RollEntry>();
        }
    }

    private void DrawResultBanner(List<RollEntry> filtered, HashSet<RollEntry> winners)
    {
        // Nothing in range — the "no rolls match" line below the banner covers it.
        if (winners.Count == 0)
            return;

        var headline = this.query switch
        {
            RollQuery.Highest => "Highest roll",
            RollQuery.Lowest => "Lowest roll",
            RollQuery.ClosestTo => $"Closest to {this.closestTarget}",
            _ => "Result",
        };
        var scope = this.windowEnabled ? $" in the last {this.windowMinutes} min" : string.Empty;
        ImGui.Spacing();
        ImGui.TextColored(StarColor, $"★ {headline}{scope}:");
        ImGui.SameLine();
        ImGui.TextUnformatted(winners.Count == 1 ? "1 match" : $"{winners.Count} matches (tie)");

        foreach (var w in winners.OrderByDescending(e => e.Time))
            ImGui.BulletText($"{Who(w)} — {RollText(w)}  ·  {w.Time:HH:mm:ss}");

        // Warn when a winning player also rolled other times in the window — the operator
        // may need to know they didn't roll just once.
        foreach (var key in winners.Select(PlayerKey).Distinct())
        {
            var theirs = filtered.Where(e => PlayerKey(e) == key).OrderByDescending(e => e.Time).ToList();
            if (theirs.Count <= 1)
                continue;
            UiText.WrappedColored(NoteColor, $"Multiple rolls detected in the time frame for {Who(theirs[0])}:");
            ImGui.Indent(18f);
            foreach (var e in theirs)
                ImGui.TextColored(NoteColor, $"• {RollText(e)}  ·  {e.Time:HH:mm:ss}");
            ImGui.Unindent(18f);
        }
    }

    // ── Pagination + table ───────────────────────────────────────────────────────

    private void DrawPager(int total)
    {
        var pages = Math.Max(1, (total + this.perPage - 1) / this.perPage);
        this.page = Math.Clamp(this.page, 0, pages - 1);

        ImGui.TextDisabled("Per page:");
        foreach (var size in PageSizes)
        {
            ImGui.SameLine();
            var active = this.perPage == size;
            if (active)
                ImGui.BeginDisabled();
            if (Ui.SmallButton($"{size}##pp{size}"))
            {
                this.perPage = size;
                this.page = 0;
            }
            if (active)
                ImGui.EndDisabled();
        }

        ImGui.SameLine();
        ImGui.TextDisabled("  |  ");
        ImGui.SameLine();
        ImGui.BeginDisabled(this.page <= 0);
        if (Ui.SmallButton("< Prev"))
            this.page--;
        ImGui.EndDisabled();
        ImGui.SameLine();
        ImGui.TextUnformatted($"Page {this.page + 1} / {pages}");
        ImGui.SameLine();
        ImGui.BeginDisabled(this.page >= pages - 1);
        if (Ui.SmallButton("Next >"))
            this.page++;
        ImGui.EndDisabled();
        ImGui.SameLine();
        ImGui.TextDisabled($"({total} shown)");
    }

    private void DrawTable(List<RollEntry> ordered, HashSet<RollEntry> winners)
    {
        var start = this.page * this.perPage;
        var pageRows = ordered.Skip(start).Take(this.perPage).ToList();

        // Fill the remaining pane; if a tall result banner left almost nothing, fall back
        // to auto-height (0) so the outer content pane scrolls instead of the table getting
        // a degenerate/negative height.
        var height = ImGui.GetContentRegionAvail().Y;
        if (height < 60f)
            height = 0f;
        if (!ImGui.BeginTable("rollslog", 5,
                ImGuiTableFlags.Borders | ImGuiTableFlags.RowBg | ImGuiTableFlags.ScrollY,
                new Vector2(0f, height)))
            return;

        ImGui.TableSetupColumn("##star", ImGuiTableColumnFlags.WidthFixed, 22);
        ImGui.TableSetupColumn("Player");
        ImGui.TableSetupColumn("World", ImGuiTableColumnFlags.WidthFixed, 130);
        ImGui.TableSetupColumn("Roll", ImGuiTableColumnFlags.WidthFixed, 130);
        ImGui.TableSetupColumn("When", ImGuiTableColumnFlags.WidthFixed, 90);
        ImGui.TableHeadersRow();

        foreach (var e in pageRows)
        {
            ImGui.TableNextRow();
            var win = winners.Contains(e);
            if (win)
                ImGui.TableSetBgColor(ImGuiTableBgTarget.RowBg0, ImGui.GetColorU32(WinnerBg), -1);

            ImGui.TableNextColumn();
            if (win)
                ImGui.TextColored(StarColor, "★");
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(e.PlayerName) ? "—" : e.PlayerName);
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(string.IsNullOrEmpty(e.World) ? "—" : e.World);
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(RollText(e));
            ImGui.TableNextColumn();
            ImGui.TextUnformatted(e.Time.ToString("HH:mm:ss"));
        }

        ImGui.EndTable();
    }

    // ── Helpers ──────────────────────────────────────────────────────────────────

    private static string RollText(RollEntry e)
        => e.OutOf.HasValue ? $"{e.Value} (out of {e.OutOf.Value})" : e.Value.ToString();

    private static string Who(RollEntry e)
        => string.IsNullOrEmpty(e.World) ? e.PlayerName : $"{e.PlayerName} ({e.World})";

    private static string PlayerKey(RollEntry e) => $"{e.PlayerName}@{e.World}";
}
