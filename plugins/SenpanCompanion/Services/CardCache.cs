using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using SenpanCompanion.Api;

namespace SenpanCompanion.Services;

/// <summary>
/// Shared, live card list. Both the Bingo Cards tab (which manages cards) and the
/// Bingo Game tab (which needs card → player-name lookups for winners) read from
/// this single source, so they stay consistent and the WebSocket card-change event
/// refreshes them once rather than per tab. Concurrent refreshes coalesce, and the
/// list + name index are swapped together on the framework thread.
/// </summary>
public sealed class CardCache : IDisposable
{
    private readonly ApiClient api;
    private readonly LiveConnection live;
    private readonly object gate = new();

    private bool loadedOnce;
    private Task? inFlight;
    private Dictionary<string, string> namesById = new();

    /// <summary>The current cards (snapshot; replaced wholesale on refresh).</summary>
    public List<CardListEntry> Cards { get; private set; } = new();

    /// <summary>True if the most recent refresh failed (so the UI can say so).</summary>
    public bool LoadFailed { get; private set; }

    public CardCache(ApiClient api, LiveConnection live)
    {
        this.api = api;
        this.live = live;
        this.live.CardsChanged += OnCardsChanged;
    }

    public void Dispose() => this.live.CardsChanged -= OnCardsChanged;

    /// <summary>Loads once (called each frame; self-guards).</summary>
    public void EnsureLoaded()
    {
        if (this.loadedOnce)
            return;
        this.loadedOnce = true;
        Refresh();
    }

    /// <summary>Marks stale so the next EnsureLoaded reloads (e.g. token change).</summary>
    public void MarkStale() => this.loadedOnce = false;

    /// <summary>Kicks a background refresh of the card list.</summary>
    public void Refresh() => _ = RefreshAsync();

    /// <summary>
    /// Refreshes the card list. Concurrent callers share one in-flight request, so a
    /// create/delete (which refreshes) plus the WebSocket card-change event don't
    /// double-fetch.
    /// </summary>
    public Task RefreshAsync()
    {
        lock (this.gate)
            return this.inFlight ??= RefreshCore();
    }

    private async Task RefreshCore()
    {
        try
        {
            // Newest-first by created_at (the server returns them ordered by id).
            // created_at is an ISO/SQLite timestamp string, which sorts chronologically;
            // blank (pre-tracking) values sort last under descending order.
            var cards = (await this.api.ListCardsAsync().ConfigureAwait(false)).Cards
                .OrderByDescending(c => c.CreatedAt, StringComparer.Ordinal)
                .ToList();
            await Plugin.Framework.RunOnFrameworkThread(() =>
            {
                this.Cards = cards;
                this.namesById = cards
                    .GroupBy(c => c.Id)
                    .ToDictionary(g => g.Key, g => g.First().PlayerName);
                this.LoadFailed = false;
            }).ConfigureAwait(false);
        }
        catch
        {
            // Best-effort; keep the previous contents and flag the failure.
            this.LoadFailed = true;
        }
        finally
        {
            lock (this.gate)
                this.inFlight = null;
        }
    }

    /// <summary>Player name for a card id, or null if unknown. O(1).</summary>
    public string? NameFor(string id)
        => this.namesById.TryGetValue(id, out var name) ? name : null;

    private void OnCardsChanged() => Refresh();
}
