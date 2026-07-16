using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using Dalamud.Plugin.Services;
using FFXIVClientStructs.FFXIV.Client.System.String;
using FFXIVClientStructs.FFXIV.Client.UI;
using FFXIVClientStructs.FFXIV.Client.UI.Shell;

namespace SenpanCompanion.Services;

/// <summary>
/// Sends an outgoing chat command (e.g. a /tell) by feeding it to the game's
/// chat-box processor on the framework thread. This is the only part of the
/// plugin that drives the game client directly.
///
/// ToS note: sending chat programmatically is the kind of automation the official
/// Dalamud repo discourages. It's included here as an explicit, operator-initiated
/// convenience — the /tell(s) for one card you personally hand out from the nearby
/// list — for a private/custom-repo build, and it's opt-out in settings. A single
/// hand-out may deliver as two or three sequential tells if the message is too long
/// for one in-game chat message; that is still one operator-initiated action, spaced
/// out to respect the chat throttle. It never loops or fires unattended.
/// </summary>
public sealed class ChatSender
{
    // Gap between the parts of a split message so the game's chat throttle doesn't
    // drop a rapid follow-up tell.
    private const int TellSpacingMs = 1000;

    private readonly IFramework framework;
    private readonly IPluginLog log;

    public ChatSender(IFramework framework, IPluginLog log)
    {
        this.framework = framework;
        this.log = log;
    }

    /// <summary>
    /// Sends one or more /tell messages to a character (the parts of a single message
    /// that was split to fit the in-game chat limit — see <see cref="TellComposer"/>).
    /// Each part is marshalled to the framework thread, best-effort (failures are
    /// logged, never thrown), and multiple parts are spaced out by
    /// <see cref="TellSpacingMs"/> so the chat throttle doesn't drop the follow-up.
    /// </summary>
    public void SendTell(string characterName, string world, IReadOnlyList<string> parts)
    {
        var name = Sanitize(characterName);
        var w = Sanitize(world);
        if (name.Length == 0 || w.Length == 0 || parts == null)
            return;

        var commands = new List<string>(parts.Count);
        foreach (var part in parts)
        {
            var msg = Sanitize(part);
            if (msg.Length > 0)
                commands.Add($"/tell {name}@{w} {msg}");
        }
        if (commands.Count == 0)
            return;

        _ = Task.Run(async () =>
        {
            try
            {
                for (var i = 0; i < commands.Count; i++)
                {
                    if (i > 0)
                        await Task.Delay(TellSpacingMs).ConfigureAwait(false);
                    var command = commands[i];
                    await this.framework.RunOnFrameworkThread(() => Execute(command)).ConfigureAwait(false);
                }
            }
            catch (Exception ex)
            {
                // Best-effort (e.g. the framework tore down between parts) — never throw
                // from this fire-and-forget task.
                this.log.Warning($"Failed to send tell: {ex.Message}");
            }
        });
    }

    private unsafe void Execute(string command)
    {
        try
        {
            var shell = RaptureShellModule.Instance();
            var ui = UIModule.Instance();
            if (shell == null || ui == null)
                return;

            var str = Utf8String.FromString(command);
            try
            {
                shell->ExecuteCommandInner(str, ui);
            }
            finally
            {
                str->Dtor(true);
            }
        }
        catch (Exception ex)
        {
            this.log.Warning($"Failed to send tell: {ex.Message}");
        }
    }

    private static string Sanitize(string s)
    {
        if (string.IsNullOrWhiteSpace(s))
            return string.Empty;
        var cleaned = new string(s.Where(ch => !char.IsControl(ch)).ToArray()).Trim();
        // Last-resort clamp; the real per-message budget is enforced by TellComposer's
        // byte-aware split (TellComposer.MaxBytes), so a valid part never hits this.
        return cleaned.Length > TellComposer.MaxBytes ? cleaned[..TellComposer.MaxBytes] : cleaned;
    }
}
