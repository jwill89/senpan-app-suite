using System;
using System.Linq;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Reflection;
using System.Threading;
using System.Threading.Tasks;
using Newtonsoft.Json;
using Newtonsoft.Json.Converters;
using Newtonsoft.Json.Serialization;

namespace SenpanCompanion.Api;

public partial class SenpanApi
{
    private readonly Func<Uri?> apiBase = () => null;
    private readonly Func<string> token = () => string.Empty;

    /// <param name="http">Carries every request. It needs no base address: each request is
    /// sent to wherever <paramref name="apiBase"/> says when it goes.</param>
    /// <param name="apiBase">Where the API is, asked again for every request so a server
    /// address changed in Settings applies at once; null while there is none.</param>
    /// <param name="token">The personal access token, asked again for every request; empty
    /// for none.</param>
    public SenpanApi(HttpClient http, Func<Uri?> apiBase, Func<string> token)
        : this(http)
    {
        this.apiBase = apiBase;
        this.token = token;
    }

    /// <summary>How every answer of the server's is read, the live events included.</summary>
    internal static JsonSerializerSettings Reading => _settings.Value;

    /// <summary>
    /// Where the API lives for a server address as a person types it, or null when it isn't
    /// one. An address without a scheme is taken as https. Plain http is only for a server on
    /// this machine: anywhere else it would send the token in the clear.
    /// </summary>
    public static Uri? ApiBase(string server)
    {
        server = server.Trim();
        if (server.Length == 0)
            return null;
        if (!server.Contains("://", StringComparison.Ordinal))
            server = "https://" + server;
        if (!Uri.TryCreate(server.TrimEnd('/') + "/", UriKind.Absolute, out var root))
            return null;
        var secure = root.Scheme == Uri.UriSchemeHttps;
        if (!secure && !(root.Scheme == Uri.UriSchemeHttp && root.IsLoopback))
            return null;
        return new Uri(root, "api/v1/");
    }

    /// <summary>Opens the staff's live event stream. The caller disposes the answer to close it.</summary>
    /// <param name="lastEventId">The id of the last event heard, so the server first sends
    /// what was missed; null on a first open.</param>
    public async Task<HttpResponseMessage> OpenEventsAsync(string? lastEventId, CancellationToken ct)
    {
        using var request = new HttpRequestMessage(HttpMethod.Get, "events");
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("text/event-stream"));
        if (lastEventId != null)
            request.Headers.TryAddWithoutValidation("Last-Event-ID", lastEventId);
        this.Prepare(request, "events");

        var response = await this._httpClient.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, ct).ConfigureAwait(false);
        if (response.IsSuccessStatusCode)
            return response;

        using (response)
        {
            var status = (int)response.StatusCode;
            var text = await response.Content.ReadAsStringAsync(ct).ConfigureAwait(false);
            var headers = response.Headers.ToDictionary(h => h.Key, h => h.Value);
            if (ReadProblem(text) is { } problem)
                throw new ApiException<ErrorModel>("The live stream did not open.", status, text, headers, problem, null);
            throw new ApiException("The live stream did not open.", status, text, headers, null);
        }
    }

    /// <summary>A problem details body, or null when the text is not one.</summary>
    internal static ErrorModel? ReadProblem(string? text)
    {
        if (string.IsNullOrWhiteSpace(text))
            return null;
        try
        {
            return JsonConvert.DeserializeObject<ErrorModel>(text, Reading);
        }
        catch (JsonException)
        {
            return null;
        }
    }

    partial void PrepareRequest(HttpClient client, HttpRequestMessage request, string url) => this.Prepare(request, url);

    private void Prepare(HttpRequestMessage request, string path)
    {
        var root = this.apiBase() ?? throw new InvalidOperationException("Set an https:// server address on the Settings page.");
        request.RequestUri = new Uri(root, path);
        var bearer = this.token();
        if (bearer.Length > 0)
            request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", bearer);
    }

    static partial void UpdateJsonSerializerSettings(JsonSerializerSettings settings)
    {
        // Times the server sends as text stay text: left on, the reader would turn them into
        // dates and back into strings in this PC's own format.
        settings.DateParseHandling = DateParseHandling.None;
        settings.ContractResolver = new NamedOrNotResolver();
    }

    private sealed class NamedOrNotResolver : DefaultContractResolver
    {
        private static readonly JsonConverter Named = new NamedOrNot();

        protected override JsonProperty CreateProperty(MemberInfo member, MemberSerialization memberSerialization)
        {
            var property = base.CreateProperty(member, memberSerialization);
            if (property.Converter is StringEnumConverter)
                property.Converter = Named;
            if (property.ItemConverter is StringEnumConverter)
                property.ItemConverter = Named;
            return property;
        }
    }

    // A value newer than this build, such as a status the server adds later, reads as one
    // that is none of those it has a name for (Enum.IsDefined says so), where the generated
    // reader would refuse the whole answer.
    private sealed class NamedOrNot : StringEnumConverter
    {
        public override object? ReadJson(JsonReader reader, Type objectType, object? existingValue, JsonSerializer serializer)
        {
            try
            {
                return base.ReadJson(reader, objectType, existingValue, serializer);
            }
            catch (JsonSerializationException) when (reader.TokenType == JsonToken.String)
            {
                return Enum.ToObject(Nullable.GetUnderlyingType(objectType) ?? objectType, -1);
            }
        }
    }
}
