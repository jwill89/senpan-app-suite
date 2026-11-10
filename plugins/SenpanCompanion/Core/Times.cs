using System;
using System.Globalization;

namespace SenpanCompanion.Core;

/// <summary>The server's times (RFC 3339 in UTC) as this PC's local time.</summary>
public static class Times
{
    /// <summary>The time in this PC's zone, "-" for none, or the text itself when it isn't a time.</summary>
    public static string Local(string? stamp, string format = "yyyy-MM-dd HH:mm")
    {
        if (string.IsNullOrWhiteSpace(stamp))
            return "-";
        return DateTimeOffset.TryParse(stamp, CultureInfo.InvariantCulture, DateTimeStyles.AssumeUniversal, out var at)
            ? at.ToLocalTime().ToString(format, CultureInfo.InvariantCulture)
            : stamp;
    }
}
