using System;
using System.Collections.Concurrent;
using System.IO;
using System.Net;
using System.Net.Http;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using Newtonsoft.Json;
using SenpanCompanion.Api;

namespace SenpanCompanion.Tests;

/// <summary>A pretend Senpan server: canned answers by path and query, from Fixtures.</summary>
internal sealed class FakeServer : HttpMessageHandler
{
    public const string Address = "https://senpan.test";

    private readonly ConcurrentDictionary<string, (HttpStatusCode Status, string Body)> answers = new();

    /// <summary>Every request as it was sent, read here since the client lets go of a body
    /// once it has its answer.</summary>
    public ConcurrentQueue<Sent> Requests { get; } = new();

    /// <summary>The token the client is asked for on each request.</summary>
    public string Token { get; set; } = "pat_test";

    public static string Fixture(string name) => File.ReadAllText(Path.Combine(AppContext.BaseDirectory, "Fixtures", name));

    public static T Fixture<T>(string name) => JsonConvert.DeserializeObject<T>(Fixture(name), SenpanApi.Reading)!;

    /// <summary>Answers <paramref name="path"/> (below /api/v1/, with its query) with a fixture.</summary>
    public FakeServer Answer(string path, string fixture, HttpStatusCode status = HttpStatusCode.OK) =>
        this.AnswerWith(path, Fixture(fixture), status);

    public FakeServer AnswerWith(string path, string body, HttpStatusCode status = HttpStatusCode.OK)
    {
        this.answers["/api/v1/" + path] = (status, body);
        return this;
    }

    /// <summary>A client of this server, with the HttpClient it owns.</summary>
    public (SenpanApi Api, HttpClient Http) Client()
    {
        var http = new HttpClient(this);
        return (new SenpanApi(http, () => SenpanApi.ApiBase(Address), () => this.Token), http);
    }

    protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
    {
        var body = request.Content == null ? null : await request.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
        request.Headers.TryGetValues("Last-Event-ID", out var lastEventId);
        this.Requests.Enqueue(new Sent(
            request.Method.Method,
            request.RequestUri!,
            request.Headers.Authorization?.ToString(),
            request.Headers.Accept.ToString(),
            lastEventId == null ? null : string.Join(",", lastEventId),
            body));

        var key = request.RequestUri!.PathAndQuery;
        if (!this.answers.TryGetValue(key, out var answer))
            answer = (HttpStatusCode.NotFound, "{\"title\":\"Not Found\",\"status\":404,\"detail\":\"no such path\"}");
        var type = (int)answer.Status < 300 ? "application/json" : "application/problem+json";
        return new HttpResponseMessage(answer.Status)
        {
            Content = answer.Body.Length == 0 ? new ByteArrayContent([]) : new StringContent(answer.Body, Encoding.UTF8, type),
        };
    }

    /// <summary>One request: what it asked for and how.</summary>
    internal sealed record Sent(string Method, Uri Uri, string? Authorization, string Accept, string? LastEventId, string? Body);
}
