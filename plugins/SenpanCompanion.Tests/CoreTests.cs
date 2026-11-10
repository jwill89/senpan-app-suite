using System;
using System.Collections.Generic;
using System.Net.Http;
using SenpanCompanion.Api;
using SenpanCompanion.Core;
using Xunit;

namespace SenpanCompanion.Tests;

public class BackoffTests
{
    [Fact]
    public void Waits_double_from_a_second_to_sixteen()
    {
        Assert.Equal(TimeSpan.FromSeconds(1), Backoff.After(1, 0.5));
        Assert.Equal(TimeSpan.FromSeconds(2), Backoff.After(2, 0.5));
        Assert.Equal(TimeSpan.FromSeconds(8), Backoff.After(4, 0.5));
        Assert.Equal(TimeSpan.FromSeconds(16), Backoff.After(5, 0.5));
        Assert.Equal(TimeSpan.FromSeconds(16), Backoff.After(40, 0.5));
    }

    [Fact]
    public void Waits_are_spread_a_fifth_either_way_and_never_past_sixteen()
    {
        Assert.Equal(TimeSpan.FromSeconds(0.8), Backoff.After(1, 0));
        Assert.Equal(TimeSpan.FromSeconds(1.2), Backoff.After(1, 1));
        Assert.Equal(TimeSpan.FromSeconds(12.8), Backoff.After(5, 0));
        Assert.Equal(TimeSpan.FromSeconds(16), Backoff.After(5, 1));
    }
}

public class ProblemsTests
{
    private static readonly Dictionary<string, IEnumerable<string>> NoHeaders = new();

    [Fact]
    public void A_token_the_server_refuses_says_to_make_a_new_one() =>
        Assert.StartsWith("The server did not accept this token.", Problems.Say(new ApiException("Unauthorized", 401, null, NoHeaders, null)));

    [Fact]
    public void A_problem_without_a_typed_answer_is_still_read()
    {
        var conflict = new ApiException("Conflict", 409, "{\"title\":\"Conflict\",\"status\":409,\"detail\":\"That game has ended.\"}", NoHeaders, null);
        Assert.Equal("That game has ended.", Problems.Say(conflict));
    }

    [Fact]
    public void Each_field_a_validation_names_is_said()
    {
        var problem = new ErrorModel
        {
            Detail = "validation failed",
            Errors = [new ErrorDetail { Message = "expected length >= 1" }, new ErrorDetail { Message = "expected number >= 1" }],
        };
        var invalid = new ApiException<ErrorModel>("Unprocessable Entity", 422, null, NoHeaders, problem, null);
        Assert.Equal("Expected length >= 1; expected number >= 1.", Problems.Say(invalid));
    }

    [Fact]
    public void Without_the_servers_words_it_says_what_to_do()
    {
        Assert.Equal("Your account doesn't have access to that.", Problems.Say(new ApiException("Forbidden", 403, string.Empty, NoHeaders, null)));
        Assert.Equal("Can't reach the server. Try again.", Problems.Say(new HttpRequestException("refused")));
        Assert.Equal("The server answered 502. Try again.", Problems.Say(new ApiException("Bad Gateway", 502, "<html>", NoHeaders, null)));
    }
}

public class RafflesTests
{
    private static Raffle Raffle(RaffleEntryMode mode, long cost = 0, params long[] tiers) =>
        new() { EntryMode = mode, CostPerEntry = cost, TierCosts = tiers };

    [Fact]
    public void A_single_cost_raffle_prices_every_ticket_the_same()
    {
        Assert.Equal(30000, Raffle(RaffleEntryMode.Single, 10000).EntryCost(3));
        Assert.Equal(0, Raffle(RaffleEntryMode.Single, 10000).EntryCost(0));
    }

    [Fact]
    public void A_custom_raffle_walks_its_ladder_and_prices_no_rung_it_lacks()
    {
        var raffle = Raffle(RaffleEntryMode.Custom, 0, 50000, 100000, 150000);
        Assert.Equal(300000, raffle.EntryCost(3));
        Assert.Equal(300000, raffle.EntryCost(100));
        Assert.True(raffle.Charges());
    }

    [Fact]
    public void A_details_only_raffle_charges_nothing_and_takes_no_signups()
    {
        var raffle = Raffle(RaffleEntryMode.Details, 10000);
        Assert.Equal(0, raffle.EntryCost(5));
        Assert.False(raffle.AcceptsSignups());
        Assert.False(raffle.Charges());
    }

    [Fact]
    public void A_mode_newer_than_this_build_is_priced_as_single() =>
        Assert.Equal(20000, Raffle((RaffleEntryMode)(-1), 10000).EntryCost(2));
}
