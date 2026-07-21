using System;
using System.Collections.Generic;
using System.Net;
using System.Threading.Tasks;
using SenpanCompanion.Api;

namespace SenpanCompanion.Services;

/// <summary>Server page-permission keys (mirror internal/server/permissions.go).</summary>
internal static class Perms
{
    public const string BingoGame = "bingo-game";
    public const string BingoCards = "bingo-cards";
    public const string BingoWinnersLog = "bingo-winners-log";
    public const string BingoPatterns = "bingo-patterns";
    public const string BingoPresets = "bingo-presets";
    public const string TeahouseRaffles = "teahouse-raffles";
    public const string FestivalGarapon = "festival-garapon";
    public const string FestivalStampRally = "festival-stamp-rally";
}

/// <summary>
/// Caches the authenticated account's identity + permissions (from GET /api/auth)
/// so the UI can hide panels the user can't access — mirroring the web, which hides
/// nav sections you lack permission for. Admins implicitly hold every permission.
/// </summary>
public sealed class Session
{
    private readonly ApiClient api;
    private readonly object gate = new();

    private long lastAttempt;
    private Task? inFlight;
    private HashSet<string> permissions = new();

    public bool Loaded { get; private set; }
    public bool LoadFailed { get; private set; }

    /// <summary>
    /// True once the token was rejected outright (401/403 or a reachable server that
    /// declined it). This is terminal: retrying won't help until the token changes, so
    /// <see cref="EnsureLoaded"/> stops polling until <see cref="MarkStale"/> is called
    /// (the settings tab does this on save), rather than hammering every 3s forever.
    /// </summary>
    public bool AuthRejected { get; private set; }

    public bool IsAdmin { get; private set; }
    public string Username { get; private set; } = string.Empty;

    public Session(ApiClient api) => this.api = api;

    /// <summary>True if the account holds the permission (admins hold everything).</summary>
    public bool Has(string permission) => this.IsAdmin || this.permissions.Contains(permission);

    /// <summary>
    /// Loads once; retries after a cooldown if the last attempt failed transiently.
    /// Once the token is rejected (<see cref="AuthRejected"/>) it stops retrying until
    /// <see cref="MarkStale"/> clears it, so a bad token isn't polled every 3s forever.
    /// </summary>
    public void EnsureLoaded()
    {
        if (this.Loaded || this.AuthRejected)
            return;
        var now = Environment.TickCount64;
        if (this.lastAttempt != 0 && now - this.lastAttempt < 3000)
            return;
        this.lastAttempt = now;
        _ = RefreshAsync();
    }

    /// <summary>Forces a reload on the next EnsureLoaded (e.g. after a token change).</summary>
    public void MarkStale()
    {
        this.Loaded = false;
        this.AuthRejected = false;
        this.lastAttempt = 0;
    }

    public Task RefreshAsync()
    {
        lock (this.gate)
            return this.inFlight ??= RefreshCore();
    }

    private async Task RefreshCore()
    {
        try
        {
            var res = await this.api.CheckAuthAsync().ConfigureAwait(false);
            await Plugin.Framework.RunOnFrameworkThread(() =>
            {
                if (res is { Authenticated: true, User: not null })
                {
                    this.IsAdmin = res.User.IsAdmin;
                    this.Username = res.User.Username;
                    this.permissions = new HashSet<string>(res.User.Permissions);
                    this.Loaded = true;
                    this.LoadFailed = false;
                }
                else
                {
                    // Reachable server, token not accepted → terminal: stop polling
                    // (until MarkStale) instead of retrying a token that won't work.
                    this.IsAdmin = false;
                    this.Username = string.Empty;
                    this.permissions = new HashSet<string>();
                    this.Loaded = false;
                    this.LoadFailed = true;
                    this.AuthRejected = true;
                }
            }).ConfigureAwait(false);
        }
        catch (ApiException ex) when (ex.Status is HttpStatusCode.Unauthorized or HttpStatusCode.Forbidden)
        {
            // Token rejected outright → terminal; back off until the token changes.
            // Marshal the state write to the framework thread (Dalamud has no SyncContext).
            await Plugin.Framework.RunOnFrameworkThread(() =>
            {
                this.LoadFailed = true;
                this.AuthRejected = true;
            }).ConfigureAwait(false);
        }
        catch
        {
            // Transient failure (network / 5xx / timeout) → keep the 3s retry cadence.
            // Marshal the status write to the framework thread per the plugin's rule.
            await Plugin.Framework.RunOnFrameworkThread(() => this.LoadFailed = true).ConfigureAwait(false);
        }
        finally
        {
            lock (this.gate)
                this.inFlight = null;
        }
    }
}
