using System;
using System.Collections.Generic;
using System.IO;
using System.Net.ServerSentEvents;
using System.Runtime.CompilerServices;
using System.Threading;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using SenpanCompanion.Api;

namespace SenpanCompanion.Core;

/// <summary>One event off the live stream: what changed, the game it belongs to, and what it carries.</summary>
/// <param name="Id">The event's number, sent back as Last-Event-ID on reopening; 0 for one
/// the server never replays.</param>
/// <param name="Type">The event's name, such as draw or resource_changed.</param>
/// <param name="GameId">The game it belongs to, or null.</param>
/// <param name="Payload">The event's own fields.</param>
public sealed record LiveEvent(long Id, string Type, long? GameId, JObject Payload)
{
    /// <summary>The payload as the document types it, such as <see cref="DrawPayload"/>.</summary>
    public T Read<T>() => this.Payload.ToObject<T>(JsonSerializer.Create(SenpanApi.Reading))
        ?? throw new JsonSerializationException($"The {this.Type} event carried nothing.");
}

/// <summary>Reads the staff's live stream: server-sent events, each a JSON envelope.</summary>
public static class Feed
{
    /// <summary>The server sends a ping every 25 seconds, so a stream this quiet has died
    /// without saying so.</summary>
    public static readonly TimeSpan Silence = TimeSpan.FromSeconds(70);

    /// <summary>Yields each event until the server closes the stream. Throws
    /// <see cref="IOException"/> when nothing, not even a ping, arrives within the silence.</summary>
    /// <param name="heard">Called for everything that arrives, pings included: proof the line is alive.</param>
    public static async IAsyncEnumerable<LiveEvent> ReadAsync(Stream stream, TimeSpan silence, Action? heard, [EnumeratorCancellation] CancellationToken ct)
    {
        using var quiet = CancellationTokenSource.CreateLinkedTokenSource(ct);
        quiet.CancelAfter(silence);
        await using var items = SseParser.Create(stream).EnumerateAsync(quiet.Token).GetAsyncEnumerator(quiet.Token);
        while (true)
        {
            bool more;
            try
            {
                more = await items.MoveNextAsync().ConfigureAwait(false);
            }
            catch (OperationCanceledException) when (!ct.IsCancellationRequested)
            {
                throw new IOException("The live stream went quiet.");
            }

            if (!more)
                yield break;
            quiet.CancelAfter(silence);
            heard?.Invoke();
            if (items.Current.EventType == "ping")
                continue;
            if (Parse(items.Current.EventType, items.Current.Data) is { } live)
                yield return live;
        }
    }

    // An event this build can't read is passed over rather than ending the stream: the
    // reconnect would only bring it back.
    private static LiveEvent? Parse(string eventType, string data)
    {
        JObject? envelope;
        try
        {
            envelope = JsonConvert.DeserializeObject<JObject>(data, SenpanApi.Reading);
        }
        catch (JsonException)
        {
            return null;
        }

        if (envelope == null)
            return null;
        var type = envelope.Value<string>("type") ?? eventType;
        var id = envelope["id"]?.Type == JTokenType.Integer ? envelope.Value<long>("id") : 0;
        var gameId = envelope["gameId"]?.Type == JTokenType.Integer ? envelope.Value<long>("gameId") : (long?)null;
        return new LiveEvent(id, type, gameId, envelope["payload"] as JObject ?? new JObject());
    }
}
