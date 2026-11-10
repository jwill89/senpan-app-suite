using System;
using System.Linq;
using System.Net.Http;
using Newtonsoft.Json;
using SenpanCompanion.Api;

namespace SenpanCompanion.Core;

/// <summary>What went wrong with a call, in words to act on.</summary>
public static class Problems
{
    /// <summary>
    /// The server's own words where it refused something (each field a validation named, or
    /// the problem's detail), what to do where it could not be asked, and a plain message for
    /// anything else.
    /// </summary>
    public static string Say(Exception e)
    {
        if (e is ApiException { StatusCode: 401 })
            return "The server did not accept this token. Make a new one on the website and paste it on the Settings page.";

        var problem = e switch
        {
            ApiException<ErrorModel> { Result: { } result } => result,
            ApiException api => SenpanApi.ReadProblem(api.Response),
            _ => null,
        };
        if (problem != null)
        {
            var named = problem.Errors is { Count: > 0 } errors
                ? string.Join("; ", errors.Select(x => x.Message).Where(m => !string.IsNullOrWhiteSpace(m)).Distinct())
                : null;
            var said = string.IsNullOrWhiteSpace(named) ? problem.Detail ?? problem.Title : named;
            if (!string.IsNullOrWhiteSpace(said))
                return Sentence(said);
        }

        return e switch
        {
            ApiException { StatusCode: 403 } => "Your account doesn't have access to that.",
            ApiException { StatusCode: 404 } => "The server doesn't have that. It may have been deleted.",
            JsonException or ApiException { InnerException: JsonException } => "The server sent something this version can't read. Update the plugin.",
            ApiException api => $"The server answered {api.StatusCode}. Try again.",
            HttpRequestException or TimeoutException or OperationCanceledException => "Can't reach the server. Try again.",
            _ => e.Message,
        };
    }

    // A detail may come without a capital or a full stop; it is shown as a sentence.
    private static string Sentence(string said)
    {
        said = said.Trim();
        said = char.ToUpperInvariant(said[0]) + said[1..];
        return said.EndsWith('.') || said.EndsWith('!') || said.EndsWith('?') ? said : said + ".";
    }
}
