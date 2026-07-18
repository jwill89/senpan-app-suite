using System;
using System.Collections.Generic;
using System.Numerics;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using Dalamud.Interface.Utility.Raii;
using SenpanCompanion.Services;

namespace SenpanCompanion.Windows;

/// <summary>
/// Timed Text Macros tool. A permission-free, account-free helper for venue-style timed
/// announcements: define a named message, a channel (Say / Yell / Shout), an interval, and
/// an optional send cap, then start it — the first send goes out immediately and it repeats
/// on the interval (via <see cref="TimedMacroRunner"/>) until stopped or the cap is reached.
/// Long messages are split with the same logic as the auto-tells and sent one part per
/// second. Macros persist across logout but always reload stopped.
///
/// Each macro renders as its own card (<see cref="Ui.Box"/>): an enumerated title with a
/// channel + status badge, a one-line message preview that expands on demand (so a 1500-char
/// macro never floods the panel), and a tiered action row. A stopped macro can be edited in
/// place via a shared <see cref="MacroForm"/> that also backs the create form.
/// </summary>
internal sealed class TimedMacrosTab
{
    private static readonly string[] ChannelLabels = { "Say", "Yell", "Shout" };
    private static readonly string[] ChannelKeys = { "say", "yell", "shout" };

    private static readonly Vector4 RunningColor = new(0.3f, 0.85f, 0.35f, 1f);
    private static readonly Vector4 StoppedColor = new(0.85f, 0.55f, 0.2f, 1f);
    private static readonly Vector4 DoneColor = new(0.55f, 0.75f, 1f, 1f);
    private static readonly Vector4 SplitWarnColor = new(0.9f, 0.65f, 0.2f, 1f);

    private readonly Configuration config;
    private readonly TimedMacroRunner runner;

    // The create form, plus the edit form and which macro (if any) it is editing.
    private readonly MacroForm createForm = new();
    private readonly MacroForm editForm = new();
    private string? editingId;

    // Macro ids whose full message is currently expanded in the list.
    private readonly HashSet<string> expanded = new();

    public TimedMacrosTab(Configuration config, TimedMacroRunner runner)
    {
        this.config = config;
        this.runner = runner;
    }

    public void Draw()
    {
        DrawIntro();

        var macros = this.config.TimedTextMacros;

        // Create form: open by default only when there's nothing yet, so a full list
        // isn't pushed down by an editor the operator rarely needs.
        if (Ui.CollapsingSection(FontAwesomeIcon.Plus, "Create a macro", "createmacro", defaultOpen: macros.Count == 0))
        {
            using (Ui.Body())
            {
                DrawForm(this.createForm, "new");
                using (ImRaii.Disabled(!this.createForm.Valid))
                {
                    if (Ui.PrimaryButton("Add macro"))
                        AddMacro();
                }
            }
        }

        Ui.Section(FontAwesomeIcon.ListUl, $"Macros ({macros.Count})");

        if (macros.Count == 0)
        {
            Ui.Help("No macros yet — create one above.");
            return;
        }

        var loggedIn = Plugin.ClientState.IsLoggedIn;
        if (!loggedIn)
            UiText.WrappedColored(StoppedColor, "You're logged out — log in to start a macro.");

        string? toDelete = null;
        for (var i = 0; i < macros.Count; i++)
        {
            var macro = macros[i];
            var index = i + 1;
            Ui.Box($"macro{macro.Id}", () =>
            {
                if (DrawMacroCard(macro, index, loggedIn))
                    toDelete = macro.Id;
            });
        }

        if (toDelete != null)
            DeleteMacro(toDelete);
    }

    private static void DrawIntro()
    {
        ImGui.AlignTextToFramePadding();
        Ui.Help("Sends real /say · /yell · /shout chat on your behalf, on a timer.");
        ImGui.SameLine();
        Ui.HelpMarker(
            "Timed Text Macros send real chat over /say, /yell, or /shout on your behalf, " +
            "automatically on a timer — the same kind of outgoing chat as the auto-tells (see " +
            "the README's ToS note). A message too long for one line is split and sent one part " +
            "per second. Macros are saved and survive logout, but always reload stopped — you " +
            "must start each one again by hand.");
    }

    // ── Shared create/edit form ──────────────────────────────────────────────────

    private void DrawForm(MacroForm form, string idPrefix)
    {
        ImGui.SetNextItemWidth(300);
        ImGui.InputTextWithHint($"Name##{idPrefix}name", "Macro title", ref form.Name, 128);

        ImGui.TextUnformatted("Channel:");
        for (var i = 0; i < ChannelLabels.Length; i++)
        {
            ImGui.SameLine();
            if (ImGui.RadioButton($"{ChannelLabels[i]}##{idPrefix}ch{i}", form.Channel == i))
                form.Channel = i;
        }

        var wide = Math.Max(160f, ImGui.GetContentRegionAvail().X - 10f);
        ImGui.InputTextMultiline($"##{idPrefix}text", ref form.Text, 8192, new Vector2(wide, 80f));

        ImGui.SetNextItemWidth(160);
        if (ImGui.InputInt($"Minutes between sends##{idPrefix}int", ref form.Interval))
            form.Interval = Math.Clamp(form.Interval, 1, 1440);

        ImGui.Checkbox($"Limit the number of sends##{idPrefix}lim", ref form.Limit);
        if (form.Limit)
        {
            ImGui.SameLine();
            ImGui.SetNextItemWidth(120);
            if (ImGui.InputInt($"times total##{idPrefix}max", ref form.MaxSends))
                form.MaxSends = Math.Clamp(form.MaxSends, 1, 100000);
        }

        DrawSplitEstimate(form.Text);
    }

    private static void DrawSplitEstimate(string text)
    {
        if (string.IsNullOrWhiteSpace(text))
            return;
        var parts = TellComposer.PartCountPlain(text);
        if (parts <= 1)
            Ui.Help("Fits in a single message.");
        else
            UiText.WrappedColored(SplitWarnColor,
                $"Too long for one line — will be sent as {parts} messages, one second apart.");
    }

    private void AddMacro()
    {
        var macro = new TimedTextMacro();
        this.createForm.ApplyTo(macro);
        this.config.TimedTextMacros.Add(macro);
        this.config.Save();

        // Clear the message + name for the next one; keep channel/interval/limit as-is.
        this.createForm.ClearNameAndText();
    }

    // ── One macro card ───────────────────────────────────────────────────────────

    /// <summary>Draws one macro card; returns true if the user asked to delete it.</summary>
    private bool DrawMacroCard(TimedTextMacro macro, int index, bool loggedIn)
    {
        if (this.editingId == macro.Id)
            return DrawMacroEditor(macro);

        var running = this.runner.IsRunning(macro.Id);
        var parts = TellComposer.PartCountPlain(macro.Text);

        // Title row: "1. Name" + channel badge + status badge.
        ImGui.TextUnformatted($"{index}. {macro.Name}");
        ImGui.SameLine();
        Ui.Badge(ChannelLabel(macro.Channel), ChannelColor(macro.Channel));
        ImGui.SameLine();
        DrawStatusBadge(macro, running);

        // Meta line.
        var cap = macro.MaxSends > 0 ? $"  ·  {macro.MaxSends}× max" : string.Empty;
        var partsNote = parts > 1 ? $"  ·  {parts} messages/send" : string.Empty;
        Ui.Help($"every {macro.IntervalMinutes} min{cap}{partsNote}");

        DrawStatusLine(macro, running);
        DrawMessage(macro, parts);

        return DrawActions(macro, running, loggedIn);
    }

    private bool DrawMacroEditor(TimedTextMacro macro)
    {
        Ui.Label($"Editing macro");
        DrawForm(this.editForm, $"edit{macro.Id}");
        ImGui.Spacing();
        using (ImRaii.Disabled(!this.editForm.Valid))
        {
            if (Ui.PrimaryButton("Save"))
                SaveEdit(macro);
        }
        ImGui.SameLine();
        if (Ui.Button("Cancel"))
            this.editingId = null;
        return false;
    }

    private static void DrawStatusBadge(TimedTextMacro macro, bool running)
    {
        if (running)
            Ui.Badge("Running", Ui.SuccessColor);
        else if (macro.IsComplete)
            Ui.Badge("Done", Ui.InfoColor);
        else
            Ui.Badge("Stopped", Ui.WarnColor);
    }

    private void DrawStatusLine(TimedTextMacro macro, bool running)
    {
        if (running)
        {
            var left = this.runner.TimeUntilNext(macro.Id) ?? TimeSpan.Zero;
            var remaining = macro.MaxSends > 0
                ? $"{macro.SendsCompleted}/{macro.MaxSends} sent, {Math.Max(0, macro.MaxSends - macro.SendsCompleted)} left"
                : $"{macro.SendsCompleted} sent";
            UiText.WrappedColored(RunningColor, $"Next send in {FormatCountdown(left)}  ·  {remaining}");
        }
        else if (macro.IsComplete)
        {
            UiText.WrappedColored(DoneColor, $"Sent {macro.SendsCompleted} time{(macro.SendsCompleted == 1 ? string.Empty : "s")} — send cap reached.");
        }
        else
        {
            var progress = macro.SendsCompleted > 0
                ? macro.MaxSends > 0
                    ? $"{macro.SendsCompleted}/{macro.MaxSends} sent so far"
                    : $"{macro.SendsCompleted} sent so far"
                : "Not started yet.";
            UiText.WrappedColored(StoppedColor, progress);
        }
    }

    private void DrawMessage(TimedTextMacro macro, int parts)
    {
        ImGui.Spacing();
        var chars = (macro.Text ?? string.Empty).Length;
        var sizeNote = $"{chars} char{(chars == 1 ? string.Empty : "s")}{(parts > 1 ? $"  ·  {parts} messages" : string.Empty)}";
        var shown = this.expanded.Contains(macro.Id);

        // The message is hidden by default and only rendered when expanded, so a long
        // (multi-message) macro never floods the card.
        if (Ui.SmallButton($"{(shown ? "Hide message" : "Show message")}##msg{macro.Id}"))
        {
            if (shown)
                this.expanded.Remove(macro.Id);
            else
                this.expanded.Add(macro.Id);
        }
        ImGui.SameLine();
        Ui.Help(sizeNote);

        if (shown)
        {
            // Full text, wrapped to the card and '%'-safe (TextUnformatted honours the
            // card's wrap position pushed by Ui.Box).
            ImGui.PushStyleColor(ImGuiCol.Text, ImGui.GetColorU32(ImGuiCol.TextDisabled));
            ImGui.TextUnformatted(macro.Text ?? string.Empty);
            ImGui.PopStyleColor();
        }
    }

    private bool DrawActions(TimedTextMacro macro, bool running, bool loggedIn)
    {
        ImGui.Spacing();
        var delete = false;

        if (running)
        {
            // A running macro can't be edited — stop it first.
            if (Ui.Button("Stop"))
                this.runner.Stop(macro.Id);
        }
        else if (macro.IsComplete)
        {
            if (Ui.Button("Reset"))
            {
                macro.SendsCompleted = 0;
                this.config.Save();
            }
            ImGui.SameLine();
            if (Ui.Button("Edit"))
                BeginEdit(macro);
        }
        else
        {
            // Starting sends immediately, which needs to be logged in (the runner refuses
            // otherwise); disable the button so it's clear rather than a silent no-op.
            using (ImRaii.Disabled(!loggedIn))
            {
                if (Ui.PrimaryButton(macro.SendsCompleted > 0 ? "Resume" : "Send"))
                    this.runner.Start(macro);
            }
            ImGui.SameLine();
            if (Ui.Button("Edit"))
                BeginEdit(macro);
        }

        ImGui.SameLine();
        if (Ui.DangerButton("Delete"))
            delete = true;

        return delete;
    }

    private void BeginEdit(TimedTextMacro macro)
    {
        this.editingId = macro.Id;
        this.editForm.LoadFrom(macro);
    }

    private void SaveEdit(TimedTextMacro macro)
    {
        this.editForm.ApplyTo(macro);
        this.config.Save();
        this.editingId = null;
    }

    private void DeleteMacro(string id)
    {
        this.runner.Remove(id);
        this.config.TimedTextMacros.RemoveAll(m => m.Id == id);
        this.config.Save();
        this.expanded.Remove(id);
        if (this.editingId == id)
            this.editingId = null;
    }

    // ── Helpers ──────────────────────────────────────────────────────────────────

    private static string ChannelLabel(string key) => key?.Trim().ToLowerInvariant() switch
    {
        "say" => "Say",
        "yell" => "Yell",
        "shout" => "Shout",
        _ => key ?? "?",
    };

    private static int ChannelIndex(string key) => key?.Trim().ToLowerInvariant() switch
    {
        "yell" => 1,
        "shout" => 2,
        _ => 0,
    };

    private static Vector4 ChannelColor(string key) => key?.Trim().ToLowerInvariant() switch
    {
        "yell" => new Vector4(0.95f, 0.75f, 0.35f, 1f),  // amber
        "shout" => new Vector4(0.95f, 0.55f, 0.45f, 1f), // coral
        _ => new Vector4(0.55f, 0.75f, 1f, 1f),          // say: blue
    };

    private static string FormatCountdown(TimeSpan t)
    {
        if (t < TimeSpan.Zero)
            t = TimeSpan.Zero;
        return t.TotalHours >= 1
            ? $"{(int)t.TotalHours}:{t.Minutes:00}:{t.Seconds:00}"
            : $"{t.Minutes:00}:{t.Seconds:00}";
    }

    // ── Create/edit form state ───────────────────────────────────────────────────

    /// <summary>
    /// Mutable working copy of a macro's editable fields, shared by the create form and
    /// the in-place editor. Kept as public fields so ImGui widgets can bind them by ref.
    /// </summary>
    private sealed class MacroForm
    {
        public string Name = string.Empty;
        public string Text = string.Empty;
        public int Channel;
        public int Interval = 15;
        public bool Limit;
        public int MaxSends = 8;

        public bool Valid => !string.IsNullOrWhiteSpace(this.Name) && !string.IsNullOrWhiteSpace(this.Text);

        public void LoadFrom(TimedTextMacro m)
        {
            this.Name = m.Name;
            this.Text = m.Text;
            this.Channel = ChannelIndex(m.Channel);
            this.Interval = Math.Clamp(m.IntervalMinutes, 1, 1440);
            this.Limit = m.MaxSends > 0;
            this.MaxSends = m.MaxSends > 0 ? m.MaxSends : 8;
        }

        public void ApplyTo(TimedTextMacro m)
        {
            m.Name = this.Name.Trim();
            m.Text = this.Text;
            m.Channel = ChannelKeys[this.Channel];
            m.IntervalMinutes = Math.Max(1, this.Interval);
            m.MaxSends = this.Limit ? Math.Max(1, this.MaxSends) : 0;
        }

        public void ClearNameAndText()
        {
            this.Name = string.Empty;
            this.Text = string.Empty;
        }
    }
}
