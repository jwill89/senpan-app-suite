using System;
using System.IO;
using System.Net;
using System.Net.WebSockets;
using System.Text;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;
using Dalamud.Plugin.Services;

namespace SenpanCompanion.Api;

/// <summary>
/// Maintains the admin WebSocket to the Senpan server (/api/ws), authenticated via
/// the personal access token on the query string. It surfaces the same live events
/// the web admin receives — number draws (with winners), game start/end, and card
/// list changes — so the bingo tab updates in real time, including when another
/// operator (or the website) drives the game.
///
/// Connecting is user-initiated (it starts when the window is open and a token is
/// configured), which keeps it on the right side of Dalamud's "no automatic
/// polling" rule. All handler callbacks are marshalled onto the game's framework
/// thread, so subscribers can touch UI state directly.
/// </summary>
public sealed class LiveConnection : IDisposable
{
    private readonly Configuration config;
    private readonly IPluginLog log;
    private readonly IFramework framework;

    private CancellationTokenSource? cts;

    // Reconnect backoff bounds. Transient drops grow the delay exponentially up to the
    // max; a clean disconnect resets to the initial delay. A token the server rejects
    // outright is terminal for the current credentials, so it backs off hard (a token
    // change restarts this loop via Start(), which resets the backoff) rather than
    // reconnecting every few seconds forever.
    private static readonly TimeSpan InitialBackoff = TimeSpan.FromSeconds(3);
    private static readonly TimeSpan MaxBackoff = TimeSpan.FromSeconds(60);
    private static readonly TimeSpan AuthRejectedBackoff = TimeSpan.FromMinutes(5);

    // Guard against a hostile/broken server streaming an unbounded message and OOMing
    // us: abort the connection once an in-progress message exceeds this many bytes.
    private const int MaxMessageBytes = 1 * 1024 * 1024;

    /// <summary>Whether the socket is currently up (polled by the status badge).</summary>
    public bool Connected { get; private set; }

    /// <summary>A number was drawn: the drawn number and the current winner card IDs.</summary>
    public event Action<DrawnNumber, string[]>? GameDraw;

    /// <summary>The game started (state != null) or ended (state == null).</summary>
    public event Action<GameState?>? GameUpdate;

    /// <summary>The card list changed (generated / deleted / renamed).</summary>
    public event Action? CardsChanged;

    /// <summary>An "It's Yoever" reaction fired: the triggering player's name and the running count.</summary>
    public event Action<string, int>? Yoever;

    /// <summary>An admin switched the "It's Yoever" reaction on/off for the current game.</summary>
    public event Action<bool>? YoeverConfig;

    /// <summary>Auto-draw state changed: the new enabled flag and interval (seconds).</summary>
    public event Action<bool, int>? AutoConfig;

    /// <summary>The server reached the half-time mark; the flag is whether auto was paused for it.</summary>
    public event Action<bool>? HalftimePrompt;

    public LiveConnection(Configuration config, IPluginLog log, IFramework framework)
    {
        this.config = config;
        this.log = log;
        this.framework = framework;
    }

    /// <summary>Starts (or restarts) the background connect/receive/reconnect loop.</summary>
    public void Start()
    {
        Stop();
        this.cts = new CancellationTokenSource();
        _ = Task.Run(() => RunAsync(this.cts.Token));
    }

    /// <summary>Stops the connection and the reconnect loop.</summary>
    public void Stop()
    {
        try
        {
            this.cts?.Cancel();
        }
        catch (ObjectDisposedException)
        {
            // already torn down
        }
        this.cts?.Dispose();
        this.cts = null;
        this.Connected = false;
    }

    public void Dispose() => Stop();

    private async Task RunAsync(CancellationToken ct)
    {
        var backoff = InitialBackoff;
        while (!ct.IsCancellationRequested)
        {
            var authRejected = false;
            try
            {
                await ConnectAndReceiveAsync(ct).ConfigureAwait(false);
                backoff = InitialBackoff; // clean disconnect → reset the backoff
            }
            catch (OperationCanceledException)
            {
                break;
            }
            catch (LiveAuthException ex)
            {
                authRejected = true;
                this.log.Warning($"Senpan live connection rejected — check your token: {ex.Message}");
            }
            catch (Exception ex)
            {
                this.log.Debug($"Senpan live connection dropped: {ex.Message}");
            }

            this.Connected = false;
            if (ct.IsCancellationRequested)
                break;

            // Back off before reconnecting so a downed (or rejecting) server isn't
            // hammered. Transient drops use exponential backoff; a rejected token backs
            // off hard until the token changes (which restarts this loop).
            TimeSpan delay;
            if (authRejected)
            {
                delay = AuthRejectedBackoff;
            }
            else
            {
                delay = backoff;
                backoff = TimeSpan.FromTicks(Math.Min(backoff.Ticks * 2, MaxBackoff.Ticks));
            }

            try
            {
                await Task.Delay(delay, ct).ConfigureAwait(false);
            }
            catch (OperationCanceledException)
            {
                break;
            }
        }
    }

    private async Task ConnectAndReceiveAsync(CancellationToken ct)
    {
        var uri = BuildWebSocketUri();
        if (uri == null)
            return;

        using var ws = new ClientWebSocket();
        // Collect the HTTP response details so a failed handshake (e.g. 401) exposes its
        // status code, letting us distinguish a rejected token from a transient error.
        ws.Options.CollectHttpResponseDetails = true;
        // Send the personal access token as an Authorization header rather than a
        // URL query parameter, so it isn't captured in server/proxy access logs.
        // The server accepts either, but the header keeps the secret out of logs.
        ws.Options.SetRequestHeader("Authorization", $"Bearer {this.config.GetToken()}");
        try
        {
            await ws.ConnectAsync(uri, ct).ConfigureAwait(false);
        }
        catch (WebSocketException) when (ws.HttpStatusCode is HttpStatusCode.Unauthorized or HttpStatusCode.Forbidden)
        {
            throw new LiveAuthException($"server returned {(int)ws.HttpStatusCode}");
        }
        this.Connected = true;

        var buffer = new byte[16 * 1024];
        using var ms = new MemoryStream();
        while (!ct.IsCancellationRequested && ws.State == WebSocketState.Open)
        {
            ms.SetLength(0);
            WebSocketReceiveResult result;
            do
            {
                result = await ws.ReceiveAsync(new ArraySegment<byte>(buffer), ct).ConfigureAwait(false);
                if (result.MessageType == WebSocketMessageType.Close)
                {
                    await ws.CloseAsync(WebSocketCloseStatus.NormalClosure, null, CancellationToken.None).ConfigureAwait(false);
                    return;
                }
                if (ms.Length + result.Count > MaxMessageBytes)
                {
                    // Refuse an over-sized message rather than growing the buffer without
                    // bound. Abort and throw so the reconnect loop backs off.
                    this.log.Warning($"Senpan live message exceeded the {MaxMessageBytes:N0}-byte cap; dropping connection.");
                    ws.Abort();
                    throw new InvalidOperationException("live message exceeded size cap");
                }
                ms.Write(buffer, 0, result.Count);
            }
            while (!result.EndOfMessage);

            // Decode once, after the full message, so a multi-byte char split across
            // 16 KB frames isn't corrupted.
            Dispatch(Encoding.UTF8.GetString(ms.GetBuffer(), 0, (int)ms.Length));
        }
    }

    private void Dispatch(string message)
    {
        WsMessage? msg;
        try
        {
            msg = JsonSerializer.Deserialize<WsMessage>(message, ApiClient.Json);
        }
        catch (JsonException)
        {
            return;
        }

        switch (msg?.Type)
        {
            case "game_draw" when msg.Drawn != null:
            {
                var drawn = msg.Drawn;
                var winners = msg.Winners ?? Array.Empty<string>();
                RunOnUi(() => GameDraw?.Invoke(drawn, winners));
                break;
            }

            case "game_update":
            {
                var state = msg.Game;
                RunOnUi(() => GameUpdate?.Invoke(state));
                break;
            }

            case "cards_update":
                RunOnUi(() => CardsChanged?.Invoke());
                break;

            case "yoever":
            {
                var name = msg.PlayerName ?? string.Empty;
                var count = msg.Count;
                RunOnUi(() => Yoever?.Invoke(name, count));
                break;
            }

            case "yoever_config":
            {
                var enabled = msg.Enabled;
                RunOnUi(() => YoeverConfig?.Invoke(enabled));
                break;
            }

            case "auto_config":
            {
                var enabled = msg.Enabled;
                var interval = msg.Interval;
                RunOnUi(() => AutoConfig?.Invoke(enabled, interval));
                break;
            }

            case "halftime_prompt":
            {
                var autoPaused = msg.AutoPaused;
                RunOnUi(() => HalftimePrompt?.Invoke(autoPaused));
                break;
            }
        }
    }

    private void RunOnUi(Action action)
    {
        // Marshal to the framework thread so subscribers can mutate UI state safely.
        // Guard the dispatch so a throwing subscriber is logged rather than silently
        // swallowed by the fire-and-forget framework task.
        _ = this.framework.RunOnFrameworkThread(() =>
        {
            try
            {
                action();
            }
            catch (Exception ex)
            {
                this.log.Error(ex, "Senpan live event subscriber threw during dispatch.");
            }
        });
    }

    private Uri? BuildWebSocketUri()
    {
        var baseUrl = this.config.ServerUrl.Trim().TrimEnd('/');
        var token = this.config.GetToken();
        if (baseUrl.Length == 0 || token.Length == 0)
            return null;

        if (!Uri.TryCreate(baseUrl, UriKind.Absolute, out var parsed))
            return null;

        // The token is sent via the Authorization header (see ConnectAndReceiveAsync),
        // not the query string, so it stays out of access logs.
        var scheme = parsed.Scheme == "https" ? "wss" : "ws";
        return new Uri($"{scheme}://{parsed.Authority}{parsed.AbsolutePath.TrimEnd('/')}/api/ws");
    }

    // Combined admin-channel message envelope — deserialized once; only the fields
    // relevant to each `type` are populated.
    private sealed class WsMessage
    {
        public string? Type { get; set; }
        public DrawnNumber? Drawn { get; set; }
        public string[]? Winners { get; set; }
        public GameState? Game { get; set; }

        // "yoever" carries the triggering player's name + running count;
        // "yoever_config" carries the new enabled flag.
        public string? PlayerName { get; set; }
        public int Count { get; set; }
        public bool Enabled { get; set; }

        // "auto_config" carries the interval alongside Enabled; "halftime_prompt"
        // carries whether auto was paused for the mini-game decision.
        public int Interval { get; set; }
        public bool AutoPaused { get; set; }
    }

    // Raised when the WebSocket handshake is rejected because the token isn't accepted
    // (401/403). Terminal for the current credentials — the reconnect loop backs off
    // hard rather than retrying every few seconds.
    private sealed class LiveAuthException : Exception
    {
        public LiveAuthException(string message) : base(message) { }
    }
}
