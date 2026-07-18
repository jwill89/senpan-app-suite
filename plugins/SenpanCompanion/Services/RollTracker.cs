using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using System.Text.RegularExpressions;
using Dalamud.Game.Chat;
using Dalamud.Game.Text;
using Dalamud.Game.Text.SeStringHandling;
using Dalamud.Game.Text.SeStringHandling.Payloads;
using Dalamud.Plugin.Services;

namespace SenpanCompanion.Services;

/// <summary>A single /random (or /dice) roll seen in chat: who rolled, their home
/// world, the number and (for <c>/random N</c>) the ceiling it was rolled against,
/// and when we captured it. <see cref="OutOf"/> is null for a plain <c>/random</c>.</summary>
public readonly record struct RollEntry(string PlayerName, string World, int Value, int? OutOf, DateTime Time);

/// <summary>
/// Watches chat for <c>/random</c> · <c>/dice</c> rolls and keeps them in memory only,
/// for the Rolls tool. It is deliberately independent of the Senpan server: nothing is
/// sent anywhere and nothing is persisted. The log is wiped on logout (and, because it
/// lives only in this object, is gone the moment the plugin unloads / the game closes).
///
/// Rolls arrive on the <see cref="XivChatType.RandomNumber"/> chat channel. Parsing is
/// deliberately language-agnostic (the game text is localized per client): that channel
/// only carries roll lines and FFXIV names never contain digits, so the numbers in the
/// line are the roll (and, for <c>/random N</c>, its ceiling). The game renders another
/// player's name as a clickable link, so the home world comes from that
/// <see cref="PlayerPayload"/>; your own rolls have no link, so an absent link means the
/// local character. Both the capture (a framework-thread chat event) and the UI read run
/// on the framework thread, but a lock keeps the list self-consistent regardless.
/// </summary>
public sealed partial class RollTracker : IDisposable
{
    // A long venue session can rack up a lot of rolls; cap the buffer so it can't grow
    // without bound. The oldest entries fall off first — the tool is about the recent
    // window, never the whole history.
    private const int MaxEntries = 5000;

    // Every number in a roll line (any client language): a plain /random has one (the
    // roll); /random N has two (the roll and its ceiling). ASCII digits with an optional
    // thousands separator — a ceiling of 1,000 renders "1,000". Source-generated.
    [GeneratedRegex(@"[0-9][0-9,]*")]
    private static partial Regex NumberPattern();

    private readonly IChatGui chat;
    private readonly IClientState clientState;
    private readonly IObjectTable objectTable;
    private readonly IPluginLog log;

    private readonly object gate = new();
    private readonly List<RollEntry> rolls = new();

    public RollTracker(IChatGui chat, IClientState clientState, IObjectTable objectTable, IPluginLog log)
    {
        this.chat = chat;
        this.clientState = clientState;
        this.objectTable = objectTable;
        this.log = log;

        this.chat.ChatMessage += OnChatMessage;
        this.clientState.Logout += OnLogout;
    }

    public void Dispose()
    {
        this.chat.ChatMessage -= OnChatMessage;
        this.clientState.Logout -= OnLogout;
        Clear();
    }

    /// <summary>Number of rolls currently held.</summary>
    public int Count
    {
        get
        {
            lock (this.gate)
                return this.rolls.Count;
        }
    }

    /// <summary>A newest-first copy of the captured rolls, safe to read from the UI.</summary>
    public List<RollEntry> Snapshot()
    {
        lock (this.gate)
        {
            var copy = new List<RollEntry>(this.rolls);
            copy.Reverse(); // stored oldest-first (append), shown newest-first
            return copy;
        }
    }

    /// <summary>Drops every captured roll (the Rolls tab's "Clear" button; also logout).</summary>
    public void Clear()
    {
        lock (this.gate)
            this.rolls.Clear();
    }

    // Logout wipes the log so a character's rolls don't linger into the next session.
    private void OnLogout(int type, int code) => Clear();

    private void OnChatMessage(IHandleableChatMessage message)
    {
        if (message.LogKind != XivChatType.RandomNumber)
            return;

        try
        {
            var line = message.Message;
            var text = line.TextValue ?? string.Empty;

            var numbers = new List<int>();
            foreach (Match m in NumberPattern().Matches(text))
                if (TryParseNumber(m.Value, out var n))
                    numbers.Add(n);
            if (numbers.Count == 0)
                return;

            // One number → a plain /random (no ceiling). Two → the roll and its ceiling in
            // whatever order the client's language writes them; a roll is always ≤ its
            // ceiling, so the smaller is the roll and the larger the ceiling.
            var value = numbers.Min();
            int? outOf = numbers.Count > 1 ? numbers.Max() : null;

            var (name, world) = ResolveRoller(line);

            var entry = new RollEntry(name, world, value, outOf, DateTime.Now);
            lock (this.gate)
            {
                this.rolls.Add(entry);
                if (this.rolls.Count > MaxEntries)
                    this.rolls.RemoveRange(0, this.rolls.Count - MaxEntries);
            }
        }
        catch (Exception ex)
        {
            // A malformed roll line must never take down the chat handler.
            this.log.Warning($"Failed to record a roll: {ex.Message}");
        }
    }

    /// <summary>
    /// Works out who rolled and their home world, language-agnostically. Another player's
    /// roll carries a <see cref="PlayerPayload"/> (the clickable name link) with the world;
    /// your own roll has no link, so an absent link means the local character.
    /// </summary>
    private (string Name, string World) ResolveRoller(SeString message)
    {
        var link = message.Payloads.OfType<PlayerPayload>().FirstOrDefault();
        if (link != null && !string.IsNullOrWhiteSpace(link.PlayerName))
            return (link.PlayerName, link.World.ValueNullable?.Name.ExtractText() ?? string.Empty);

        var me = this.objectTable.LocalPlayer;
        if (me != null)
            return (me.Name.TextValue, me.HomeWorld.ValueNullable?.Name.ExtractText() ?? string.Empty);

        // Own roll but the local character isn't readable (e.g. mid-transition).
        return ("You", string.Empty);
    }

    private static bool TryParseNumber(string raw, out int value)
        => int.TryParse(raw, NumberStyles.Integer | NumberStyles.AllowThousands, CultureInfo.InvariantCulture, out value);
}
