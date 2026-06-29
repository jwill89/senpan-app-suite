using System;
using System.Collections.Generic;
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
    public bool IsAdmin { get; private set; }
    public string Username { get; private set; } = string.Empty;

    public Session(ApiClient api) => this.api = api;

    /// <summary>True if the account holds the permission (admins hold everything).</summary>
    public bool Has(string permission) => this.IsAdmin || this.permissions.Contains(permission);

    /// <summary>Loads once; retries after a cooldown if the last attempt failed.</summary>
    public void EnsureLoaded()
    {
        if (this.Loaded)
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
                    // Reachable server, token not accepted → no access.
                    this.IsAdmin = false;
                    this.Username = string.Empty;
                    this.permissions = new HashSet<string>();
                    this.Loaded = false;
                    this.LoadFailed = true;
                }
            }).ConfigureAwait(false);
        }
        catch
        {
            this.LoadFailed = true;
        }
        finally
        {
            lock (this.gate)
                this.inFlight = null;
        }
    }
}
