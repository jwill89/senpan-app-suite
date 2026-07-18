using System;
using System.Collections.Generic;
using Dalamud.Plugin.Services;

namespace SenpanCompanion.Services;

/// <summary>
/// Drives the Timed Text Macros. Each running macro fires an initial send when started and
/// then repeats every <see cref="TimedTextMacro.IntervalMinutes"/> minutes until an optional
/// send cap is reached. Timing is checked on <see cref="IFramework.Update"/> (the game frame),
/// so it keeps ticking whether or not the window is open, and the actual chat is sent through
/// <see cref="ChatSender"/> (split to fit, one part per second).
///
/// Run state (running / next-due) is intentionally in-memory only — macros persist, but a
/// macro always reloads stopped. <see cref="TimedTextMacro.SendsCompleted"/> is saved after
/// every fire, so a crash mid-run leaves the remaining count correct. Logout stops every
/// macro (you can't chat while logged out, and the spec is "won't resume until restarted").
///
/// The Update handler, Start/Stop/Remove (called from the framework-thread Draw loop) and
/// logout all run on the framework thread, so state is single-threaded; a lock guards it
/// anyway since <see cref="TimeUntilNext"/>/<see cref="IsRunning"/> are cheap and defensive.
/// </summary>
public sealed class TimedMacroRunner : IDisposable
{
    private sealed class RunState
    {
        public bool Running;
        public DateTime NextSendUtc;
    }

    private readonly Configuration config;
    private readonly ChatSender chat;
    private readonly IFramework framework;
    private readonly IClientState clientState;
    private readonly IPluginLog log;

    private readonly object gate = new();
    private readonly Dictionary<string, RunState> states = new();

    public TimedMacroRunner(Configuration config, ChatSender chat, IFramework framework, IClientState clientState, IPluginLog log)
    {
        this.config = config;
        this.chat = chat;
        this.framework = framework;
        this.clientState = clientState;
        this.log = log;

        this.framework.Update += OnUpdate;
        this.clientState.Logout += OnLogout;
    }

    public void Dispose()
    {
        this.framework.Update -= OnUpdate;
        this.clientState.Logout -= OnLogout;
    }

    /// <summary>True if the macro is currently ticking.</summary>
    public bool IsRunning(string id)
    {
        lock (this.gate)
            return this.states.TryGetValue(id, out var s) && s.Running;
    }

    /// <summary>Time until the next send for a running macro, else null (clamped at zero).</summary>
    public TimeSpan? TimeUntilNext(string id)
    {
        lock (this.gate)
        {
            if (!this.states.TryGetValue(id, out var s) || !s.Running)
                return null;
            var left = s.NextSendUtc - DateTime.UtcNow;
            return left > TimeSpan.Zero ? left : TimeSpan.Zero;
        }
    }

    /// <summary>
    /// Starts (or resumes) a macro: fires one send now, then schedules the next. Refused
    /// while logged out — otherwise a click at the title/character screen would bank a
    /// phantom send and leave the macro armed to auto-resume on the next login (there is
    /// no logout event to clear it, since we're already logged out).
    /// </summary>
    public bool Start(TimedTextMacro macro)
    {
        if (!this.clientState.IsLoggedIn)
            return false;
        lock (this.gate)
            this.states[macro.Id] = new RunState { Running = true };
        Fire(macro);
        return true;
    }

    /// <summary>Halts a macro without touching its progress.</summary>
    public void Stop(string id)
    {
        lock (this.gate)
        {
            if (this.states.TryGetValue(id, out var s))
                s.Running = false;
        }
    }

    /// <summary>Stops and forgets a macro's run state (on delete).</summary>
    public void Remove(string id)
    {
        lock (this.gate)
            this.states.Remove(id);
    }

    private void OnLogout(int type, int code)
    {
        // Can't chat while logged out; stop everything so nothing fires into the void and
        // nothing auto-resumes on the next login.
        lock (this.gate)
        {
            foreach (var s in this.states.Values)
                s.Running = false;
        }
    }

    private void OnUpdate(IFramework fw)
    {
        if (!this.clientState.IsLoggedIn)
            return;

        var now = DateTime.UtcNow;
        List<TimedTextMacro>? due = null;
        lock (this.gate)
        {
            foreach (var macro in this.config.TimedTextMacros)
            {
                if (this.states.TryGetValue(macro.Id, out var s) && s.Running && now >= s.NextSendUtc)
                    (due ??= new List<TimedTextMacro>()).Add(macro);
            }
        }

        if (due == null)
            return;
        foreach (var macro in due)
            Fire(macro);
    }

    // Sends the macro's text now, records progress, and either schedules the next send or
    // marks the run complete when the cap is reached.
    private void Fire(TimedTextMacro macro)
    {
        try
        {
            var parts = TellComposer.SplitPlain(macro.Text);
            this.chat.SendChannelMessage(macro.Channel, parts);

            macro.SendsCompleted++;
            this.config.Save();
        }
        catch (Exception ex)
        {
            this.log.Warning($"Failed to fire timed macro '{macro.Name}': {ex.Message}");
        }

        lock (this.gate)
        {
            if (!this.states.TryGetValue(macro.Id, out var s))
                return;
            if (macro.IsComplete)
                s.Running = false;
            else
                s.NextSendUtc = DateTime.UtcNow.AddMinutes(Math.Max(1, macro.IntervalMinutes));
        }
    }
}
