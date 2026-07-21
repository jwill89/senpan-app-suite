using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Threading;
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
/// Dalamud repo discourages. It's included here for a private/custom-repo build and is
/// opt-out in settings. Most sends are explicit, operator-initiated conveniences — the
/// /tell(s) for one card you personally hand out from the nearby list. The Timed Text
/// Macros are the exception: those DO fire unattended on a repeating timer (see
/// <see cref="TimedMacroRunner"/>) until you stop them or their send cap is reached.
/// A single message may deliver as two or three sequential parts if it's too long for
/// one in-game chat message.
///
/// Every send — regardless of which feature enqueued it — is funnelled through one
/// global, serialized send path (<see cref="SendSpaced"/>) that spaces consecutive
/// messages <see cref="MessageSpacingMs"/> apart, so concurrent features can't gang up
/// and defeat the game's outgoing-chat throttle.
/// </summary>
public sealed class ChatSender
{
    // Minimum gap between ANY two consecutive outgoing messages — one second — so the
    // game's chat throttle doesn't drop a rapid follow-up. Enforced globally across every
    // send path (the auto-tells and the Timed Text Macros' say/yell/shout), not just
    // within a single call, so concurrent features can't each start at t=0 and burst.
    private const int MessageSpacingMs = 1000;

    // Serializes every outgoing message across all callers. Held while a single message
    // is delayed + executed, so only one send is ever in flight and the spacing below is
    // measured from the previous message no matter which feature enqueued it.
    private readonly SemaphoreSlim sendGate = new(1, 1);

    // Earliest UtcNow.Ticks at which the next message may be sent. Read/written only while
    // holding sendGate, so it needs no further synchronization.
    private long nextAllowedTicks;

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
    /// <see cref="MessageSpacingMs"/> so the chat throttle doesn't drop the follow-up.
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
        SendSpaced(commands);
    }

    /// <summary>
    /// Sends a plain message over a public channel (<c>say</c> / <c>yell</c> / <c>shout</c>)
    /// — the Timed Text Macros. Like <see cref="SendTell"/>, the message is pre-split into
    /// <paramref name="parts"/> (see <see cref="TellComposer.SplitPlain"/>) and the parts are
    /// delivered one second apart. An unknown channel is ignored.
    /// </summary>
    public void SendChannelMessage(string channel, IReadOnlyList<string> parts)
    {
        var command = ChannelCommand(channel);
        if (command == null || parts == null)
            return;

        var commands = new List<string>(parts.Count);
        foreach (var part in parts)
        {
            var msg = Sanitize(part);
            if (msg.Length > 0)
                commands.Add($"{command} {msg}");
        }
        SendSpaced(commands);
    }

    /// <summary>Maps a channel key to its chat command, or null if unrecognized.</summary>
    private static string? ChannelCommand(string channel) => channel?.Trim().ToLowerInvariant() switch
    {
        "say" => "/say",
        "yell" => "/yell",
        "shout" => "/shout",
        _ => null,
    };

    // Runs each command on the framework thread, best-effort, spaced globally. Fire-and-
    // forget — failures are logged, never thrown (the caller is often a UI click or a timer).
    // The per-command work runs under sendGate so all sends across the whole plugin form one
    // serialized stream: each waits until nextAllowedTicks, executes, then pushes the next
    // slot MessageSpacingMs into the future. This holds regardless of how many features
    // enqueue at once, so the throttle sees a steady one-per-second cadence.
    private void SendSpaced(List<string> commands)
    {
        if (commands.Count == 0)
            return;

        _ = Task.Run(async () =>
        {
            try
            {
                foreach (var command in commands)
                    await SendOneSpaced(command).ConfigureAwait(false);
            }
            catch (Exception ex)
            {
                // Best-effort (e.g. the framework tore down between parts) — never throw
                // from this fire-and-forget task.
                this.log.Warning($"Failed to send chat message: {ex.Message}");
            }
        });
    }

    // Sends a single command through the global gate: acquire, wait out any remaining
    // spacing from the previous message, execute on the framework thread, then reserve the
    // next slot. Releasing only after the delay keeps the whole plugin to one send per slot.
    private async Task SendOneSpaced(string command)
    {
        await this.sendGate.WaitAsync().ConfigureAwait(false);
        try
        {
            var waitTicks = this.nextAllowedTicks - DateTime.UtcNow.Ticks;
            if (waitTicks > 0)
            {
                var waitMs = (int)Math.Min(int.MaxValue, waitTicks / TimeSpan.TicksPerMillisecond);
                if (waitMs > 0)
                    await Task.Delay(waitMs).ConfigureAwait(false);
            }

            await this.framework.RunOnFrameworkThread(() => Execute(command)).ConfigureAwait(false);
            this.nextAllowedTicks = DateTime.UtcNow.AddMilliseconds(MessageSpacingMs).Ticks;
        }
        finally
        {
            this.sendGate.Release();
        }
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
        // byte-aware split (TellComposer.MaxBytes), so a valid part never hits this. Clamp
        // by UTF-8 byte length (the game's actual limit), not UTF-16 char count, so a
        // multibyte message isn't wrongly truncated (or, worse, let through over budget).
        return ClampToByteBudget(cleaned);
    }

    // Truncates to at most TellComposer.MaxBytes UTF-8 bytes at a code-point boundary, so a
    // surrogate pair is never split. Returns the input unchanged when it already fits.
    private static string ClampToByteBudget(string s)
    {
        if (Encoding.UTF8.GetByteCount(s) <= TellComposer.MaxBytes)
            return s;

        var bytes = 0;
        var idx = 0;
        foreach (var rune in s.EnumerateRunes())
        {
            if (bytes + rune.Utf8SequenceLength > TellComposer.MaxBytes)
                break;
            bytes += rune.Utf8SequenceLength;
            idx += rune.Utf16SequenceLength;
        }
        return s[..idx];
    }
}
