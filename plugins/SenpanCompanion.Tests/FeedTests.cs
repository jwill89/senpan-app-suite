using System;
using System.Collections.Generic;
using System.IO;
using System.IO.Pipelines;
using System.Linq;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using SenpanCompanion.Api;
using SenpanCompanion.Core;
using Xunit;

namespace SenpanCompanion.Tests;

public class FeedTests
{
    private const string Wire =
        "id: 41\nevent: draw\ndata: {\"id\":41,\"type\":\"draw\",\"gameId\":7,\"at\":\"2026-11-10T20:05:00Z\",\"payload\":{\"drawn\":{\"callOrder\":3,\"letter\":\"B\",\"number\":12},\"winners\":[\"ABC234\"]}}\n\n" +
        "event: ping\ndata: {\"id\":0,\"type\":\"ping\",\"gameId\":null,\"at\":\"2026-11-10T20:05:25Z\",\"payload\":{}}\n\n" +
        "id: 42\nevent: resource_changed\ndata: {\"id\":42,\"type\":\"resource_changed\",\"gameId\":null,\"at\":\"2026-11-10T20:05:30Z\",\"payload\":{\"resource\":\"raffles\"}}\n\n" +
        "id: 43\nevent: halftime_resolved\ndata: {\"id\":43,\"type\":\"halftime_resolved\",\"gameId\":7,\"at\":\"2026-11-10T20:06:00Z\",\"payload\":{\"minigame\":true,\"by\":\"tataru\"}}\n\n";

    private static async Task<List<LiveEvent>> ReadAll(string wire, Action? heard = null)
    {
        var seen = new List<LiveEvent>();
        await foreach (var live in Feed.ReadAsync(new MemoryStream(Encoding.UTF8.GetBytes(wire)), TimeSpan.FromSeconds(5), heard, TestContext.Current.CancellationToken))
            seen.Add(live);
        return seen;
    }

    [Fact]
    public async Task Events_are_read_and_pings_are_not_passed_on()
    {
        var heard = 0;
        var seen = await ReadAll(Wire, () => heard++);

        // A ping is not passed on, and is still proof the line is alive.
        Assert.Equal(4, heard);
        Assert.Equal(["draw", "resource_changed", "halftime_resolved"], seen.Select(e => e.Type));
        Assert.Equal([41L, 42L, 43L], seen.Select(e => e.Id));
        Assert.Equal([7L, null, 7L], seen.Select(e => e.GameId));
    }

    [Fact]
    public async Task A_payload_reads_as_the_document_types_it()
    {
        var seen = await ReadAll(Wire);

        var draw = seen[0].Read<DrawPayload>();
        Assert.Equal((DrawnNumberLetter.B, 12L, 3L), (draw.Drawn.Letter, draw.Drawn.Number, draw.Drawn.CallOrder));
        Assert.Equal(["ABC234"], draw.Winners);
        Assert.Equal("raffles", seen[1].Read<ResourceChangedPayload>().Resource);
        var answer = seen[2].Read<HalftimeResolvedPayload>();
        Assert.Equal((true, "tataru"), (answer.Minigame, answer.By));
    }

    [Fact]
    public async Task An_event_this_build_cannot_read_is_passed_over()
    {
        const string wire =
            "id: 50\nevent: draw\ndata: {not json\n\n" +
            "id: 51\nevent: game_started\ndata: {\"id\":51,\"type\":\"game_started\",\"gameId\":8,\"at\":\"2026-11-10T21:00:00Z\",\"payload\":{},\"new\":\"field\"}\n\n";
        var only = Assert.Single(await ReadAll(wire));
        Assert.Equal(("game_started", 51L, 8L), (only.Type, only.Id, only.GameId));
    }

    [Fact]
    public async Task A_line_that_goes_quiet_is_a_lost_connection()
    {
        var silent = new Pipe().Reader.AsStream();
        await Assert.ThrowsAsync<IOException>(async () =>
        {
            await foreach (var _ in Feed.ReadAsync(silent, TimeSpan.FromMilliseconds(50), null, TestContext.Current.CancellationToken))
            {
            }
        });
    }

    [Fact]
    public async Task Stopping_is_not_a_lost_connection()
    {
        var silent = new Pipe().Reader.AsStream();
        using var stop = new CancellationTokenSource(50);
        await Assert.ThrowsAnyAsync<OperationCanceledException>(async () =>
        {
            await foreach (var _ in Feed.ReadAsync(silent, TimeSpan.FromSeconds(30), null, stop.Token))
            {
            }
        });
    }
}
