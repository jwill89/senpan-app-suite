using System;
using System.Collections.Generic;
using System.Text;

namespace SenpanCompanion.Services;

/// <summary>
/// Turns a user-editable auto-tell template into the actual /tell message(s).
///
/// Placeholders are expanded to their literal text FIRST, then the length is checked —
/// so a link placeholder (which the plugin, not the game, expands) is measured at the
/// full width of the URL it becomes. <c>&lt;t&gt;</c> is likewise expanded by the plugin to
/// the recipient's character name: a raw <c>&lt;t&gt;</c> sent through the chat command would
/// be delivered as the literal text, not a name, so the plugin must substitute it.
///
/// If the expanded message is longer than one in-game chat message allows, it is split
/// into multiple tells at the best break point — the latest sentence end within budget,
/// else the latest word boundary, else a hard cut — so nothing is dropped.
///
/// Pure (System-only, no Dalamud) so the splitting logic is easy to reason about and
/// exercise in isolation.
/// </summary>
public static class TellComposer
{
    /// <summary>Recipient's character name (expanded by the plugin).</summary>
    public const string TargetToken = "<t>";
    public const string BingoCardLinkToken = "<bingocard-link>";
    public const string GaraponLinkToken = "<garapon-link>";
    public const string StampCardLinkToken = "<stamprally-link>";

    /// <summary>
    /// Maximum UTF-8 bytes in one tell message. FFXIV caps a chat message at 500 bytes;
    /// this budgets the message the recipient sees (the "/tell name@world " routing
    /// prefix is added by <see cref="ChatSender"/> and is not part of this count).
    /// </summary>
    public const int MaxBytes = 500;

    // Every recognized placeholder. Any not supplied for the current context is blanked,
    // so a mis-used token can't leak to the recipient as literal "<garapon-link>" text.
    private static readonly string[] AllTokens =
    {
        TargetToken, BingoCardLinkToken, GaraponLinkToken, StampCardLinkToken,
    };

    /// <summary>
    /// Expands <paramref name="template"/> with <paramref name="values"/> (token → text),
    /// blanks any unsupplied recognized token, flattens to a single line, and splits the
    /// result into tell-sized parts. Always returns at least one entry (an empty string
    /// for an empty template — callers skip empty sends).
    /// </summary>
    public static List<string> Compose(string template, IReadOnlyDictionary<string, string> values)
        => Split(Expand(template, values));

    /// <summary>How many tells the template would produce with the given expansions.</summary>
    public static int PartCount(string template, IReadOnlyDictionary<string, string> values)
        => Compose(template, values).Count;

    private static string Expand(string template, IReadOnlyDictionary<string, string> values)
    {
        var sb = new StringBuilder(template ?? string.Empty);
        foreach (var token in AllTokens)
            sb.Replace(token, values.TryGetValue(token, out var v) ? v : string.Empty);
        return CollapseToLine(sb.ToString());
    }

    // A tell is a single line: turn newlines/tabs into spaces, drop other control chars,
    // and collapse the whitespace runs that produces.
    private static string CollapseToLine(string s)
    {
        var sb = new StringBuilder(s.Length);
        var lastWasSpace = false;
        foreach (var ch in s)
        {
            var c = ch is '\r' or '\n' or '\t' ? ' ' : ch;
            if (c != ' ' && char.IsControl(c))
                continue;
            if (c == ' ')
            {
                if (lastWasSpace)
                    continue;
                lastWasSpace = true;
            }
            else
            {
                lastWasSpace = false;
            }
            sb.Append(c);
        }
        return sb.ToString().Trim();
    }

    private static List<string> Split(string message)
    {
        var parts = new List<string>();
        var remaining = message;
        // Guard against a pathological non-advancing cut (shouldn't happen — FindCut
        // returns >= 1 — but keep the loop provably terminating).
        while (Utf8Len(remaining) > MaxBytes)
        {
            var cut = FindCut(remaining);
            var head = remaining[..cut].Trim();
            if (head.Length > 0)
                parts.Add(head);
            remaining = remaining[cut..].TrimStart();
        }
        var tail = remaining.Trim();
        if (tail.Length > 0)
            parts.Add(tail);
        if (parts.Count == 0)
            parts.Add(string.Empty);
        return parts;
    }

    // Char index to cut at: the most text that stays within MaxBytes, backed off to the
    // latest sentence end (preferred) or word boundary in the later half of that window,
    // else a hard cut at the byte boundary.
    private static int FindCut(string s)
    {
        var hard = MaxCharsWithinBudget(s); // 1..s.Length, s[..hard] fits in MaxBytes
        var floor = Math.Max(1, hard / 2);  // don't break absurdly early

        for (var i = hard; i >= floor; i--)
        {
            var c = s[i - 1];
            // A sentence end: . ! ? at (or just before) a space or the boundary.
            if ((c == '.' || c == '!' || c == '?') && (i == s.Length || s[i] == ' '))
                return i;
        }
        for (var i = hard; i >= floor; i--)
        {
            if (s[i - 1] == ' ')
                return i;
        }
        return hard;
    }

    // Largest count of leading chars whose UTF-8 length is <= MaxBytes, at a code-point
    // boundary (so a surrogate pair is never split). At least 1 so Split always advances.
    private static int MaxCharsWithinBudget(string s)
    {
        var bytes = 0;
        var idx = 0;
        foreach (var rune in s.EnumerateRunes())
        {
            var runeBytes = rune.Utf8SequenceLength;
            if (bytes + runeBytes > MaxBytes)
                return Math.Max(1, idx);
            bytes += runeBytes;
            idx += rune.Utf16SequenceLength;
        }
        return s.Length;
    }

    private static int Utf8Len(string s) => Encoding.UTF8.GetByteCount(s);
}
