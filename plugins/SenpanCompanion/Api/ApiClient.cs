using System;
using System.Net;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading;
using System.Threading.Tasks;
using Dalamud.Networking.Http;
using Dalamud.Plugin.Services;

namespace SenpanCompanion.Api;

/// <summary>Thrown for any non-2xx response; Message carries the server's error text.</summary>
public sealed class ApiException : Exception
{
    public HttpStatusCode Status { get; }
    public ApiException(string message, HttpStatusCode status) : base(message) => this.Status = status;
}

/// <summary>
/// Typed REST client for the Senpan server. Every request carries the configured
/// personal access token as a Bearer credential, so the server applies the same
/// per-page permission guards the web UI gets. All methods are async and must be
/// awaited off the game's framework thread (the UI fires them and reads results
/// back on the next frame).
/// </summary>
public sealed class ApiClient : IDisposable
{
    public static readonly JsonSerializerOptions Json = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    private readonly HttpClient http;
    private readonly Configuration config;
    private readonly IPluginLog log;
    private readonly HappyEyeballsCallback happyEyeballs = new();
    private readonly CancellationTokenSource lifetime = new();

    public ApiClient(Configuration config, IPluginLog log)
    {
        this.config = config;
        this.log = log;
        // HappyEyeballs improves IPv6/dual-stack connect behaviour (Dalamud guidance).
        var handler = new SocketsHttpHandler { ConnectCallback = this.happyEyeballs.ConnectCallback };
        this.http = new HttpClient(handler) { Timeout = TimeSpan.FromSeconds(15) };
    }

    public void Dispose()
    {
        this.lifetime.Cancel();
        this.http.Dispose();
        this.happyEyeballs.Dispose();
        this.lifetime.Dispose();
    }

    // ── Auth ─────────────────────────────────────────────────────────────────

    public Task<AuthCheckResponse> CheckAuthAsync(CancellationToken ct = default)
        => SendAsync<AuthCheckResponse>(HttpMethod.Get, "api/auth", null, ct);

    // ── Bingo: cards ───────────────────────────────────────────────────────────

    public Task<CardsResponse> ListCardsAsync(CancellationToken ct = default)
        => SendAsync<CardsResponse>(HttpMethod.Get, "api/cards", null, ct);

    public Task<BoardResponse> GetCardBoardAsync(string id, CancellationToken ct = default)
        => SendAsync<BoardResponse>(HttpMethod.Get, $"api/board?id={Uri.EscapeDataString(id)}&preview=1", null, ct);

    public Task<GenerateSingleResponse> CreateNamedCardAsync(string playerName, CancellationToken ct = default)
        => SendAsync<GenerateSingleResponse>(HttpMethod.Post, "api/cards",
            new { player_name = playerName }, ct);

    public Task DeleteCardAsync(string id, CancellationToken ct = default)
        => SendNoContentAsync(HttpMethod.Delete, $"api/cards/{Uri.EscapeDataString(id)}", null, ct);

    public Task<DeletedCountResponse> DeleteAllCardsAsync(CancellationToken ct = default)
        => SendAsync<DeletedCountResponse>(HttpMethod.Delete, "api/cards/all", null, ct);

    // ── Bingo: patterns + game ───────────────────────────────────────────────

    public Task<PatternsResponse> ListPatternsAsync(CancellationToken ct = default)
        => SendAsync<PatternsResponse>(HttpMethod.Get, "api/patterns", null, ct);

    public Task<PresetsResponse> ListPresetsAsync(CancellationToken ct = default)
        => SendAsync<PresetsResponse>(HttpMethod.Get, "api/presets", null, ct);

    public Task<FrequentWinnersResponse> FrequentWinnersAsync(CancellationToken ct = default)
        => SendAsync<FrequentWinnersResponse>(HttpMethod.Get, "api/winners-log/frequent", null, ct);

    public Task<WinnersLogResponse> WinnersLogAsync(int page, int perPage, string sort = "logged_at", string dir = "desc", CancellationToken ct = default)
        => SendAsync<WinnersLogResponse>(HttpMethod.Get,
            $"api/winners-log?page={page}&per_page={perPage}&sort={Uri.EscapeDataString(sort)}&dir={Uri.EscapeDataString(dir)}", null, ct);

    // Per-entry delete only — the plugin deliberately exposes no "clear all" for
    // the winners log, so it can't be wiped from in-game. Returns no body (204).
    public Task DeleteWinnersLogEntryAsync(long id, CancellationToken ct = default)
        => SendNoContentAsync(HttpMethod.Delete, $"api/winners-log/{id}", null, ct);

    public Task<GameStateResponse> GetGameAsync(CancellationToken ct = default)
        => SendAsync<GameStateResponse>(HttpMethod.Get, "api/game", null, ct);

    public Task<GameStateResponse> StartGameAsync(int[] patternIds, CancellationToken ct = default)
        => SendAsync<GameStateResponse>(HttpMethod.Post, "api/game/start",
            new { pattern_ids = patternIds }, ct);

    public Task<DrawResult> DrawAsync(int delaySeconds, CancellationToken ct = default)
        => SendAsync<DrawResult>(HttpMethod.Post, "api/game/draw",
            new { delay = delaySeconds }, ct);

    public Task<OkResponse> EndGameAsync(string[] validWinnerIds, CancellationToken ct = default)
        => SendAsync<OkResponse>(HttpMethod.Post, "api/game/end",
            new { valid_winner_ids = validWinnerIds }, ct);

    public Task<OkResponse> TriggerHalftimeAsync(CancellationToken ct = default)
        => SendAsync<OkResponse>(HttpMethod.Post, "api/game/halftime", null, ct);

    public Task<OkResponse> UpdateGameDetailsAsync(string details, CancellationToken ct = default)
        => SendAsync<OkResponse>(HttpMethod.Patch, "api/game", new { details }, ct);

    // Switches the "It's Yoever" reaction on/off for the current game. The server
    // broadcasts a yoever_config message so every client (and admin) stays in sync.
    public Task<OkResponse> SetYoeverEnabledAsync(bool enabled, CancellationToken ct = default)
        => SendAsync<OkResponse>(HttpMethod.Patch, "api/game", new { yoever_enabled = enabled }, ct);

    // ── Raffles ──────────────────────────────────────────────────────────────

    public Task<RafflesResponse> ListRafflesAsync(CancellationToken ct = default)
        => SendAsync<RafflesResponse>(HttpMethod.Get, "api/raffles", null, ct);

    public Task<RaffleDetailResponse> GetRaffleAsync(long id, CancellationToken ct = default)
        => SendAsync<RaffleDetailResponse>(HttpMethod.Get, $"api/raffles/{id}", null, ct);

    public Task<RaffleEntryResponse> AddRaffleEntryAsync(long raffleId, string characterName, string world, int numEntries, bool paid, CancellationToken ct = default)
        => SendAsync<RaffleEntryResponse>(HttpMethod.Post, $"api/raffles/{raffleId}/entries",
            new { character_name = characterName, world, num_entries = numEntries, paid }, ct);

    public Task<RaffleEntryResponse> MarkRaffleEntryPaidAsync(long raffleId, long entryId, bool paid, CancellationToken ct = default)
        => SendAsync<RaffleEntryResponse>(HttpMethod.Patch, $"api/raffles/{raffleId}/entries/{entryId}",
            new { paid }, ct);

    public Task DeleteRaffleEntryAsync(long raffleId, long entryId, CancellationToken ct = default)
        => SendNoContentAsync(HttpMethod.Delete, $"api/raffles/{raffleId}/entries/{entryId}", null, ct);

    public Task<RaffleWinnerResponse> PickRaffleWinnerAsync(long raffleId, CancellationToken ct = default)
        => SendAsync<RaffleWinnerResponse>(HttpMethod.Post, $"api/raffles/{raffleId}/pick-winner", null, ct);

    public Task<RaffleWinnerResponse> PickAnotherRaffleWinnerAsync(long raffleId, CancellationToken ct = default)
        => SendAsync<RaffleWinnerResponse>(HttpMethod.Post, $"api/raffles/{raffleId}/pick-another", null, ct);

    public Task<OkResponse> VerifyRaffleWinnerAsync(long raffleId, CancellationToken ct = default)
        => SendAsync<OkResponse>(HttpMethod.Post, $"api/raffles/{raffleId}/verify-winner", null, ct);

    // ── Transport ────────────────────────────────────────────────────────────

    private async Task<T> SendAsync<T>(HttpMethod method, string path, object? body, CancellationToken ct)
    {
        // Link the caller's token with the client lifetime so in-flight requests
        // cancel when the plugin unloads.
        using var linked = CancellationTokenSource.CreateLinkedTokenSource(this.lifetime.Token, ct);
        using var req = new HttpRequestMessage(method, BuildUrl(path));
        var bearer = this.config.Token.Trim();
        if (bearer.Length > 0)
            req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", bearer);
        if (body != null)
            req.Content = new StringContent(JsonSerializer.Serialize(body, Json), Encoding.UTF8, "application/json");

        using var resp = await this.http.SendAsync(req, linked.Token).ConfigureAwait(false);
        var text = await resp.Content.ReadAsStringAsync(linked.Token).ConfigureAwait(false);

        if (!resp.IsSuccessStatusCode)
        {
            var msg = TryExtractError(text) ?? $"Request failed ({(int)resp.StatusCode})";
            this.log.Warning($"Senpan API {method} {path} -> {(int)resp.StatusCode}: {msg}");
            throw new ApiException(msg, resp.StatusCode);
        }

        var result = JsonSerializer.Deserialize<T>(text, Json);
        if (result == null)
            throw new ApiException("Empty or unreadable response", resp.StatusCode);
        return result;
    }

    // SendNoContentAsync issues a request that returns no body (e.g. a 204 from a
    // DELETE). It shares the auth + error handling of SendAsync but skips
    // deserialization, so an empty success body isn't treated as an error.
    private async Task SendNoContentAsync(HttpMethod method, string path, object? body, CancellationToken ct)
    {
        using var linked = CancellationTokenSource.CreateLinkedTokenSource(this.lifetime.Token, ct);
        using var req = new HttpRequestMessage(method, BuildUrl(path));
        var bearer = this.config.Token.Trim();
        if (bearer.Length > 0)
            req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", bearer);
        if (body != null)
            req.Content = new StringContent(JsonSerializer.Serialize(body, Json), Encoding.UTF8, "application/json");

        using var resp = await this.http.SendAsync(req, linked.Token).ConfigureAwait(false);
        if (!resp.IsSuccessStatusCode)
        {
            var text = await resp.Content.ReadAsStringAsync(linked.Token).ConfigureAwait(false);
            var msg = TryExtractError(text) ?? $"Request failed ({(int)resp.StatusCode})";
            this.log.Warning($"Senpan API {method} {path} -> {(int)resp.StatusCode}: {msg}");
            throw new ApiException(msg, resp.StatusCode);
        }
    }

    private Uri BuildUrl(string path)
    {
        var baseUrl = this.config.ServerUrl.Trim().TrimEnd('/');
        return new Uri($"{baseUrl}/{path.TrimStart('/')}");
    }

    private static string? TryExtractError(string body)
    {
        if (string.IsNullOrWhiteSpace(body))
            return null;
        try
        {
            using var doc = JsonDocument.Parse(body);
            if (doc.RootElement.ValueKind == JsonValueKind.Object &&
                doc.RootElement.TryGetProperty("error", out var err) &&
                err.ValueKind == JsonValueKind.String)
                return err.GetString();
        }
        catch (JsonException)
        {
            // Non-JSON error body — fall through.
        }
        return null;
    }
}
