using System;

namespace SenpanCompanion.Core;

/// <summary>How long to wait before opening the live stream again: doubling from a second
/// to sixteen, spread out a little so every console doesn't come back at once.</summary>
public static class Backoff
{
    private static readonly TimeSpan Longest = TimeSpan.FromSeconds(16);

    /// <param name="failures">How many tries in a row have failed, counting this one.</param>
    /// <param name="spread">A random number from 0 to 1.</param>
    public static TimeSpan After(int failures, double spread)
    {
        var seconds = Math.Min(Longest.TotalSeconds, Math.Pow(2, Math.Clamp(failures - 1, 0, 10)));
        return TimeSpan.FromSeconds(Math.Min(Longest.TotalSeconds, seconds * (0.8 + (0.4 * spread))));
    }
}
