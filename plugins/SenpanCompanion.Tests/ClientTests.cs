using System;
using System.Linq;
using System.Net;
using System.Threading.Tasks;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using SenpanCompanion.Api;
using SenpanCompanion.Core;
using Xunit;

namespace SenpanCompanion.Tests;

/// <summary>The generated client against a pretend server answering as the document declares.</summary>
public class ClientTests
{
    [Theory]
    [InlineData("senpan.test", "https://senpan.test/api/v1/")]
    [InlineData(" https://senpan.test/ ", "https://senpan.test/api/v1/")]
    [InlineData("https://senpan.test/suite", "https://senpan.test/suite/api/v1/")]
    [InlineData("http://localhost:8080", "http://localhost:8080/api/v1/")]
    [InlineData("http://127.0.0.1:8080/", "http://127.0.0.1:8080/api/v1/")]
    public void A_server_address_as_typed_finds_the_api(string typed, string api) =>
        Assert.Equal(new Uri(api), SenpanApi.ApiBase(typed));

    [Theory]
    [InlineData("")]
    [InlineData("   ")]
    [InlineData("http://senpan.test")]
    [InlineData("ftp://senpan.test")]
    public void An_address_the_token_should_not_go_to_is_refused(string typed) =>
        Assert.Null(SenpanApi.ApiBase(typed));

    [Fact]
    public async Task Every_call_goes_to_the_api_with_the_token_it_is_given_then()
    {
        var server = new FakeServer().Answer("me", "me.json");
        var (api, http) = server.Client();
        using (http)
        {
            var me = await api.GetMeAsync(TestContext.Current.CancellationToken);
            server.Token = "pat_changed";
            await api.GetMeAsync(TestContext.Current.CancellationToken);

            Assert.Equal(("tataru", false), (me.Account!.Username, me.Account.IsAdmin));
            Assert.Equal(["bingo-game", "teahouse-raffles"], me.Account.Permissions);
            var sent = server.Requests.ToArray();
            Assert.All(sent, s => Assert.Equal(new Uri("https://senpan.test/api/v1/me"), s.Uri));
            Assert.Equal(["Bearer pat_test", "Bearer pat_changed"], sent.Select(s => s.Authorization));
        }
    }

    [Fact]
    public async Task The_game_reads_as_the_server_sends_it()
    {
        var server = new FakeServer().Answer("game", "game.json");
        var (api, http) = server.Client();
        using (http)
        {
            var state = await api.GetGameAsync(TestContext.Current.CancellationToken);

            var game = state.Game!;
            Assert.Equal((7L, 5L), (game.Id, state.DrawDelay));
            Assert.Equal([12L, 47L], game.CalledNumbers);
            Assert.Equal((7L, true), (game.Halftime!.GameId, game.Halftime.AutoPaused));
            Assert.Equal((LastDrawLetter.G, 47L), (game.LastDraw!.Letter, game.LastDraw.Number));
            Assert.True(game.Patterns[0].PatternData[0][4]);
            Assert.Equal(["ABC234"], state.Winners);

            // A time sent as text arrives as the same text, whatever this PC's own date format.
            Assert.Equal("2026-11-10T20:00:00Z", game.CreatedAt);
        }
    }

    [Fact]
    public async Task No_game_running_reads_as_none()
    {
        var server = new FakeServer().Answer("game", "game-none.json");
        var (api, http) = server.Client();
        using (http)
        {
            var state = await api.GetGameAsync(TestContext.Current.CancellationToken);
            Assert.Null(state.Game);
            Assert.Empty(state.Winners);
        }
    }

    [Fact]
    public async Task A_raffle_reads_its_nulls_and_its_pending_winner()
    {
        var server = new FakeServer().Answer("raffles/9", "raffle.json").Answer("raffles/9/entries", "raffle-entries.json");
        var (api, http) = server.Client();
        using (http)
        {
            var detail = await api.GetRaffleAsync(9, TestContext.Current.CancellationToken);
            var entries = (await api.ListRaffleEntriesAsync(9, TestContext.Current.CancellationToken)).Entries;

            Assert.Null(detail.WinnerEntry);
            Assert.Null(detail.Raffle.AvailableFrom);
            Assert.Null(detail.Raffle.FestivalMapId);
            Assert.Equal((RaffleStatus.Open, RaffleEntryMode.Custom, 11L), (detail.Raffle.Status, detail.Raffle.EntryMode, detail.Raffle.WinnerEntryId));
            Assert.Equal([50000L, 100000L, 150000L], detail.Raffle.TierCosts);
            Assert.Equal([RaffleEntryPaymentState.Partial, RaffleEntryPaymentState.Paid], entries.Select(e => e.PaymentState));
            Assert.Null(entries[1].CreatedAt);
        }
    }

    [Fact]
    public async Task A_value_newer_than_this_build_does_not_refuse_the_answer()
    {
        var later = JObject.Parse(FakeServer.Fixture("raffle-entries.json"));
        later["entries"]![0]!["paymentState"] = "refunded";
        var server = new FakeServer().AnswerWith("raffles/9/entries", later.ToString());
        var (api, http) = server.Client();
        using (http)
        {
            var entries = (await api.ListRaffleEntriesAsync(9, TestContext.Current.CancellationToken)).Entries;
            Assert.False(Enum.IsDefined(entries[0].PaymentState));
            Assert.Equal(RaffleEntryPaymentState.Paid, entries[1].PaymentState);
        }
    }

    [Fact]
    public async Task A_tea_room_without_a_lock_time_reads_as_none()
    {
        var server = new FakeServer().Answer("tea-rooms", "tea-rooms.json");
        var (api, http) = server.Client();
        using (http)
        {
            var rooms = (await api.ListTeaRoomsAsync(TestContext.Current.CancellationToken)).TeaRooms;
            Assert.Equal(("3", "2026-11-10T21:30:00Z"), (rooms[0].RoomNumber, rooms[0].LockedUntil));
            Assert.Null(rooms[1].LockedUntil);
        }
    }

    [Fact]
    public async Task The_winners_log_is_asked_for_in_camel_case()
    {
        var server = new FakeServer().AnswerWith("winners-log?page=1&perPage=200&sort=loggedAt&dir=desc", "{\"entries\":[],\"page\":1,\"perPage\":200,\"total\":0}");
        var (api, http) = server.Client();
        using (http)
        {
            var page = await api.ListWinnersAsync(1, 200, Sort.LoggedAt, Dir.Desc, TestContext.Current.CancellationToken);
            Assert.Equal(0, page.Total);
        }
    }

    [Fact]
    public async Task A_change_sends_only_what_it_sets()
    {
        const string room = "{\"color\":\"\",\"costPerHalfHour\":1,\"createdAt\":null,\"description\":\"\",\"discounted\":false,\"hashtags\":\"\",\"id\":1,\"image\":\"\",\"lockable\":true,\"locked\":true,\"lockedUntil\":null,\"name\":\"R\",\"open\":true,\"roomNumber\":\"1\",\"roomOwner\":\"\",\"seasonal\":false,\"sortOrder\":0,\"subtitle\":\"\"}";
        var server = new FakeServer().AnswerWith("tea-rooms/1", room).AnswerWith("raffles/9/entries/11/settle", JObject.Parse(FakeServer.Fixture("raffle-entries.json"))["entries"]![0]!.ToString());
        var (api, http) = server.Client();
        using (http)
        {
            var ct = TestContext.Current.CancellationToken;
            await api.ChangeTeaRoomAsync(1, new TeaRoomChanges { Locked = true }, ct);
            await api.ChangeTeaRoomAsync(1, new TeaRoomChanges { Locked = true, LockedUntil = new DateTimeOffset(2026, 11, 10, 21, 30, 0, TimeSpan.Zero) }, ct);
            await api.SettleRaffleEntryAsync(9, 11, new RaffleSettlement { Waived = 0 }, ct);
            await api.SettleRaffleEntryAsync(9, 11, new RaffleSettlement { PaidEntries = 2, Waived = 5000 }, ct);

            var bodies = server.Requests.Select(s => JsonConvert.DeserializeObject<JObject>(s.Body!, SenpanApi.Reading)!).ToArray();
            Assert.Equal(new JObject { ["locked"] = true }, bodies[0]);
            Assert.Equal("2026-11-10T21:30:00+00:00", bodies[1].Value<string>("lockedUntil"));
            Assert.Equal(new JObject { ["waived"] = 0 }, bodies[2]);
            Assert.Equal(new JObject { ["paidEntries"] = 2, ["waived"] = 5000 }, bodies[3]);
            Assert.All(server.Requests, s => Assert.Equal(s.Uri.AbsolutePath.StartsWith("/api/v1/tea-rooms", StringComparison.Ordinal) ? "PATCH" : "POST", s.Method));
        }
    }

    [Fact]
    public async Task A_refusal_says_the_servers_own_words()
    {
        var server = new FakeServer()
            .Answer("raffles/9/entries", "problem-validation.json", HttpStatusCode.UnprocessableEntity)
            .Answer("raffles/9/pick-winner", "problem-conflict.json", HttpStatusCode.Conflict);
        var (api, http) = server.Client();
        using (http)
        {
            var ct = TestContext.Current.CancellationToken;
            var invalid = await Assert.ThrowsAsync<ApiException<ErrorModel>>(() =>
                api.AddRaffleEntryAsync(9, new RaffleEntryInput { CharacterName = "A", World = "B", NumEntries = 1000 }, ct));
            var conflict = await Assert.ThrowsAsync<ApiException<ErrorModel>>(() => api.PickRaffleWinnerAsync(9, ct));

            Assert.Equal("Expected number <= 100.", Problems.Say(invalid));
            Assert.Equal("A winner is already picked: confirm it or pick another.", Problems.Say(conflict));
        }
    }

    [Fact]
    public async Task The_live_stream_opens_from_the_last_event_heard()
    {
        var server = new FakeServer().AnswerWith("events", string.Empty);
        var (api, http) = server.Client();
        using (http)
        {
            using var response = await api.OpenEventsAsync("41", TestContext.Current.CancellationToken);

            var sent = Assert.Single(server.Requests);
            Assert.Equal(new Uri("https://senpan.test/api/v1/events"), sent.Uri);
            Assert.Equal(("text/event-stream", "41", "Bearer pat_test"), (sent.Accept, sent.LastEventId, sent.Authorization));
        }
    }

    [Fact]
    public async Task A_refused_stream_says_why_with_its_status()
    {
        var server = new FakeServer().AnswerWith("events", "{\"title\":\"Forbidden\",\"status\":403,\"detail\":\"Your account has no admin page.\"}", HttpStatusCode.Forbidden);
        var (api, http) = server.Client();
        using (http)
        {
            var refused = await Assert.ThrowsAsync<ApiException<ErrorModel>>(() => api.OpenEventsAsync(null, TestContext.Current.CancellationToken));
            Assert.Equal(403, refused.StatusCode);
            Assert.Equal("Your account has no admin page.", Problems.Say(refused));
            Assert.Null(Assert.Single(server.Requests).LastEventId);
        }
    }
}
