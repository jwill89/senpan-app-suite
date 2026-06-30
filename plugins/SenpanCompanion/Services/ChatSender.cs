using System;
using System.Linq;
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
/// convenience — one /tell per card you personally hand out from the nearby list —
/// for a private/custom-repo build, and it's opt-out in settings. It never loops
/// or fires unattended.
/// </summary>
public sealed class ChatSender
{
    private readonly IFramework framework;
    private readonly IPluginLog log;

    public ChatSender(IFramework framework, IPluginLog log)
    {
        this.framework = framework;
        this.log = log;
    }

    /// <summary>
    /// Sends a /tell to a character. Marshalled to the framework thread and
    /// best-effort (failures are logged, never thrown). The text is reduced to a
    /// single line and clamped so it can't carry control characters or overflow.
    /// </summary>
    public void SendTell(string characterName, string world, string message)
    {
        var name = Sanitize(characterName);
        var w = Sanitize(world);
        var msg = Sanitize(message);
        if (name.Length == 0 || w.Length == 0 || msg.Length == 0)
            return;

        var command = $"/tell {name}@{w} {msg}";
        _ = this.framework.RunOnFrameworkThread(() => Execute(command));
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
        return cleaned.Length > 400 ? cleaned[..400] : cleaned;
    }
}
