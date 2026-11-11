using System;
using System.Collections.Generic;
using System.Globalization;
using System.Threading;
using System.Threading.Tasks;
using Dalamud.Plugin.Services;
using SenpanCompanion.Api;
using SenpanCompanion.Core;

namespace SenpanCompanion.Services;

/// <summary>
/// Holds the staff's live event stream (GET /api/v1/events, with the token as a Bearer
/// header): number draws with their winners, the game starting and ending, half-time, the
/// automatic draws, the draw delay, the details, card and page changes, and expiring
/// tea-room locks, so every page follows what any console or the website does.
///
/// A dropped stream is opened again after a back-off of 1 to 16 seconds, sending the last
/// event's id so the server first replays what was missed; 70 seconds without even a ping
/// counts as a drop. A 401 or 403 stops it until the token changes. Connecting is
/// user-initiated (the auto-connect setting), which keeps it on the right side of
/// Dalamud's "no automatic polling" rule. Every event is raised on the game's framework
/// thread, so subscribers can touch UI state directly.
/// </summary>
public sealed class LiveConnection : IDisposable
{
    private readonly SenpanApi api;
    private readonly Configuration config;
    private readonly IPluginLog log;
    private readonly IFramework framework;

    private CancellationTokenSource? cts;
    private volatile bool connected;
    private volatile bool refused;

    public LiveConnection(SenpanApi api, Configuration config, IPluginLog log, IFramework framework)
    {
        this.api = api;
        this.config = config;
        this.log = log;
        this.framework = framework;
    }

    /// <summary>Whether the stream is open (polled by the status badge).</summary>
    public bool Connected => this.connected;

    /// <summary>Whether the server refused the token (401) or the account (403); it stays
    /// stopped until <see cref="Start"/> runs again with new settings.</summary>
    public bool Refused => this.refused;

    /// <summary>
    /// The stream opened, the first time or again after a drop. Whatever is shown is only as
    /// complete as the events it was built from, so the game is read again on every open.
    /// </summary>
    public event Action? Opened;

    /// <summary>The server doesn't hold the events since the last one heard: read everything again.</summary>
    public event Action? Resync;

    /// <summary>A number was drawn: the game, the number and every winning card so far.</summary>
    public event Action<long?, DrawnNumber, IReadOnlyList<string>>? Draw;

    /// <summary>A game started; read it again.</summary>
    public event Action<long?>? GameStarted;

    /// <summary>A game ended; read it again.</summary>
    public event Action<long?>? GameEnded;

    /// <summary>The card list changed.</summary>
    public event Action? CardsChanged;

    /// <summary>The win patterns changed.</summary>
    public event Action? PatternsChanged;

    /// <summary>A player sent the It's Yoever reaction: the game, their name and the running count.</summary>
    public event Action<long?, string, long>? Yoever;

    /// <summary>The It's Yoever reaction was switched on or off.</summary>
    public event Action<bool>? YoeverSwitched;

    /// <summary>The automatic draws changed: whether they run and the seconds between them.</summary>
    public event Action<bool, long>? AutoDraw;

    /// <summary>The game reached half time: the game and whether the automatic draws paused for it.</summary>
    public event Action<long?, bool>? HalftimePrompt;

    /// <summary>Someone answered the half-time prompt: the game, whether a mini-game runs, and who.</summary>
    public event Action<long?, bool, string>? HalftimeResolved;

    /// <summary>The seconds a drawn number waits before players get it changed.</summary>
    public event Action<long>? DrawDelay;

    /// <summary>The game's details changed.</summary>
    public event Action<string>? GameDetails;

    /// <summary>A page's records changed, named as the server names it (raffles, garapons, ...).</summary>
    public event Action<string>? ResourceChanged;

    /// <summary>
    /// A tea room's lock ran out and the server lifted it: the room's id, name and number.
    /// The name rides along because this event is announced rather than rendered, wherever
    /// the operator is in the plugin.
    /// </summary>
    public event Action<long, string, string>? TeaRoomUnlocked;

    /// <summary>Starts (or restarts) the stream, unless there is no server address or token.</summary>
    public void Start()
    {
        this.Stop();
        this.refused = false;
        if (SenpanApi.ApiBase(this.config.ServerUrl) == null || this.config.GetToken().Length == 0)
            return;
        this.cts = new CancellationTokenSource();
        var ct = this.cts.Token;
        _ = Task.Run(() => this.RunAsync(ct));
    }

    /// <summary>Closes the stream and stops reopening it.</summary>
    public void Stop()
    {
        try
        {
            this.cts?.Cancel();
        }
        catch (ObjectDisposedException)
        {
            // Already torn down.
        }
        this.cts?.Dispose();
        this.cts = null;
        this.connected = false;
    }

    public void Dispose() => this.Stop();

    private async Task RunAsync(CancellationToken ct)
    {
        var failures = 0;
        string? lastId = null;
        while (!ct.IsCancellationRequested)
        {
            try
            {
                using var response = await this.api.OpenEventsAsync(lastId, ct).ConfigureAwait(false);
                await using var stream = await response.Content.ReadAsStreamAsync(ct).ConfigureAwait(false);
                this.connected = true;
                this.RunOnUi(() => this.Opened?.Invoke());
                await foreach (var live in Feed.ReadAsync(stream, Feed.Silence, () => failures = 0, ct).ConfigureAwait(false))
                {
                    if (live.Id > 0)
                        lastId = live.Id.ToString(CultureInfo.InvariantCulture);
                    this.Dispatch(live);
                }
            }
            catch (ApiException e) when (e.StatusCode is 401 or 403)
            {
                this.connected = false;
                this.refused = true;
                this.log.Warning($"Senpan live stream refused ({e.StatusCode}): {Problems.Say(e)}");
                return;
            }
            catch (OperationCanceledException) when (ct.IsCancellationRequested)
            {
                break;
            }
            catch (Exception e)
            {
                // The type alone: an answer can carry a player's name.
                this.log.Debug($"Senpan live stream dropped: {e.GetType().Name}");
            }

            this.connected = false;
            failures++;
            try
            {
                await Task.Delay(Backoff.After(failures, Random.Shared.NextDouble()), ct).ConfigureAwait(false);
            }
            catch (OperationCanceledException)
            {
                break;
            }
        }

        this.connected = false;
    }

    private void Dispatch(LiveEvent live)
    {
        try
        {
            switch (live.Type)
            {
                case "draw":
                {
                    var draw = live.Read<DrawPayload>();
                    this.RunOnUi(() => this.Draw?.Invoke(live.GameId, draw.Drawn, draw.Winners));
                    break;
                }
                case "game_started":
                    this.RunOnUi(() => this.GameStarted?.Invoke(live.GameId));
                    break;
                case "game_ended":
                    this.RunOnUi(() => this.GameEnded?.Invoke(live.GameId));
                    break;
                case "cards_changed":
                    this.RunOnUi(() => this.CardsChanged?.Invoke());
                    break;
                case "patterns_changed":
                    this.RunOnUi(() => this.PatternsChanged?.Invoke());
                    break;
                case "yoever":
                {
                    var yoever = live.Read<YoeverPayload>();
                    this.RunOnUi(() => this.Yoever?.Invoke(live.GameId, yoever.PlayerName, yoever.Count));
                    break;
                }
                case "yoever_switched":
                {
                    var switched = live.Read<YoeverSwitchedPayload>();
                    this.RunOnUi(() => this.YoeverSwitched?.Invoke(switched.Enabled));
                    break;
                }
                case "auto_draw":
                {
                    var auto = live.Read<AutoDrawPayload>();
                    this.RunOnUi(() => this.AutoDraw?.Invoke(auto.Enabled, auto.Interval));
                    break;
                }
                case "halftime_prompt":
                {
                    var prompt = live.Read<HalftimePromptPayload>();
                    this.RunOnUi(() => this.HalftimePrompt?.Invoke(live.GameId, prompt.AutoPaused));
                    break;
                }
                case "halftime_resolved":
                {
                    var answer = live.Read<HalftimeResolvedPayload>();
                    this.RunOnUi(() => this.HalftimeResolved?.Invoke(live.GameId, answer.Minigame, answer.By));
                    break;
                }
                case "draw_delay":
                {
                    var delay = live.Read<DrawDelayPayload>();
                    this.RunOnUi(() => this.DrawDelay?.Invoke(delay.Delay));
                    break;
                }
                case "game_details":
                {
                    var details = live.Read<GameDetailsPayload>();
                    this.RunOnUi(() => this.GameDetails?.Invoke(details.Details));
                    break;
                }
                case "resource_changed":
                {
                    var changed = live.Read<ResourceChangedPayload>();
                    this.RunOnUi(() => this.ResourceChanged?.Invoke(changed.Resource));
                    break;
                }
                case "tea_room_unlocked":
                {
                    var room = live.Read<TeaRoomUnlockedPayload>();
                    this.RunOnUi(() => this.TeaRoomUnlocked?.Invoke(room.Id, room.Name, room.RoomNumber));
                    break;
                }
                case "resync":
                    this.RunOnUi(() => this.Resync?.Invoke());
                    break;
            }
        }
        catch (Newtonsoft.Json.JsonException e)
        {
            // One event this build can't read is passed over; the stream carries on.
            this.log.Debug($"Senpan live event {live.Type} unreadable: {e.GetType().Name}");
        }
    }

    private void RunOnUi(Action action)
    {
        // Marshal to the framework thread so subscribers can mutate UI state safely, and log
        // a throwing subscriber rather than lose it in the fire-and-forget task.
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
}
