using System;
using System.Numerics;
using System.Threading.Tasks;
using Dalamud.Bindings.ImGui;

namespace SenpanCompanion.Windows;

/// <summary>
/// Shared plumbing for the tab panels:
/// <list type="bullet">
/// <item>a busy-gated background <see cref="Run"/>ner whose result-writes are applied
/// on the framework thread (via <see cref="Apply"/>), so UI state never races the
/// render loop or the WebSocket handlers;</item>
/// <item>a once-with-cooldown-retry data load (<see cref="EnsureLoaded"/> +
/// <see cref="LoadAsync"/>); and</item>
/// <item>a persistent error line (<see cref="DrawStatusLine"/>).</item>
/// </list>
/// </summary>
internal abstract class TabBase
{
    private static readonly Vector4 ErrorColor = new(0.9f, 0.5f, 0.4f, 1f);
    private const long RetryCooldownMs = 3000;

    // Written from the background Run op (see below) and read by Draw on the framework
    // thread, so both are volatile — the render loop must always see the latest value the
    // worker published, not a stale cached copy.
    protected volatile string Status = string.Empty;
    protected volatile bool Busy;

    private bool loaded;
    private long lastLoadAttempt; // 0 = no attempt yet (load immediately, no cooldown)

    /// <summary>Override to fetch this tab's data. Apply writes via <see cref="Apply"/>.</summary>
    protected virtual Task LoadAsync() => Task.CompletedTask;

    /// <summary>
    /// Loads on first view. After a *failed* load it retries, but no more than once
    /// per cooldown so a persistent failure doesn't storm the server. A successful
    /// load sets <c>loaded</c>, so the cooldown never gates normal use.
    /// </summary>
    public void EnsureLoaded()
    {
        if (this.loaded || this.Busy)
            return;
        var now = Environment.TickCount64;
        if (this.lastLoadAttempt != 0 && now - this.lastLoadAttempt < RetryCooldownMs)
            return;
        this.lastLoadAttempt = now;
        Run(async () =>
        {
            await LoadAsync();
            this.loaded = true;
        });
    }

    /// <summary>Marks data stale so the next <see cref="EnsureLoaded"/> reloads at once.</summary>
    public void MarkStale()
    {
        this.loaded = false;
        this.lastLoadAttempt = 0;
    }

    /// <summary>
    /// Runs <paramref name="op"/> on a background thread (network), busy-gated;
    /// exceptions land in <see cref="Status"/>. UI-state writes inside the op must go
    /// through <see cref="Apply"/> so they happen on the framework thread.
    /// </summary>
    protected void Run(Func<Task> op)
    {
        if (this.Busy)
            return;
        this.Busy = true;
        this.Status = string.Empty;
        _ = Task.Run(async () =>
        {
            try
            {
                await op();
            }
            catch (Exception ex)
            {
                this.Status = ex.Message;
            }
            finally
            {
                this.Busy = false;
            }
        });
    }

    /// <summary>Applies UI-state writes on the framework thread. Await inside a Run op.</summary>
    protected static Task Apply(Action apply) => Plugin.Framework.RunOnFrameworkThread(apply);

    /// <summary>Draws the persistent error line, if any.</summary>
    protected void DrawStatusLine()
    {
        if (!string.IsNullOrEmpty(this.Status))
            ImGui.TextColored(ErrorColor, this.Status);
    }
}
