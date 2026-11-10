using System;
using SenpanCompanion.Api;

namespace SenpanCompanion.Core;

/// <summary>A raffle's pricing, as the server prices it (raffles.Pricing.EntryCost), so the
/// plugin never quotes a different total.</summary>
public static class Raffles
{
    /// <summary>False for a details-only raffle, which is entered outside the app.</summary>
    public static bool AcceptsSignups(this Raffle raffle) => raffle.EntryMode != RaffleEntryMode.Details;

    /// <summary>Whether the raffle charges anything for a ticket.</summary>
    public static bool Charges(this Raffle raffle) =>
        raffle.AcceptsSignups() && (raffle.CostPerEntry > 0 || raffle.TierCosts.Count > 0);

    /// <summary>
    /// What holding <paramref name="tickets"/> tickets costs: a flat multiple in single mode,
    /// the first N rungs of the ladder in custom mode, and nothing for a details-only raffle.
    /// A mode this build doesn't know is priced as single, as the server reads one.
    /// </summary>
    public static long EntryCost(this Raffle raffle, long tickets)
    {
        if (tickets < 1 || raffle.EntryMode == RaffleEntryMode.Details)
            return 0;
        if (raffle.EntryMode != RaffleEntryMode.Custom)
            return tickets * raffle.CostPerEntry;

        // The ladder, not the ticket count: only rungs that exist have a price.
        var rungs = (int)Math.Min(tickets, raffle.TierCosts.Count);
        var total = 0L;
        for (var i = 0; i < rungs; i++)
            total += raffle.TierCosts[i];
        return total;
    }
}
