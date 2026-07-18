using System;

namespace SenpanCompanion.Services;

/// <summary>
/// A user-defined timed announcement: some text repeatedly sent over a public chat
/// channel on a fixed interval. Persisted in <see cref="Configuration"/> (so it survives
/// logout / restart); the run state — whether it's currently ticking and when the next
/// send is due — is NOT persisted and lives in <see cref="TimedMacroRunner"/>, so a macro
/// always comes back stopped and must be started by hand.
///
/// <see cref="SendsCompleted"/> is saved after every send, so a crash mid-run leaves the
/// remaining count accurate when the plugin reloads.
/// </summary>
[Serializable]
public class TimedTextMacro
{
    /// <summary>Stable identity, used to key the run state and the UI widgets.</summary>
    public string Id { get; set; } = Guid.NewGuid().ToString("N");

    /// <summary>Display name / title (required).</summary>
    public string Name { get; set; } = string.Empty;

    /// <summary>The message text; sent verbatim, split across messages if too long.</summary>
    public string Text { get; set; } = string.Empty;

    /// <summary>Public channel to send on: <c>say</c>, <c>yell</c>, or <c>shout</c>.</summary>
    public string Channel { get; set; } = "say";

    /// <summary>Minutes between sends (minimum 1).</summary>
    public int IntervalMinutes { get; set; } = 15;

    /// <summary>Total number of sends before it stops; <c>0</c> means unlimited.</summary>
    public int MaxSends { get; set; }

    /// <summary>How many times this macro has fired so far (persisted after each send).</summary>
    public int SendsCompleted { get; set; }

    /// <summary>True when a send cap is set and it has been reached.</summary>
    public bool IsComplete => this.MaxSends > 0 && this.SendsCompleted >= this.MaxSends;
}
