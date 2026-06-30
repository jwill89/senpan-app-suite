using System;
using Dalamud.Configuration;
using Dalamud.Plugin;

namespace SenpanCompanion;

/// <summary>
/// Persisted plugin settings. Stored by Dalamud as JSON in the plugin's config
/// directory. The only secret here is the personal access token; it is sent as a
/// Bearer credential to the Senpan server and never leaves this machine otherwise.
/// </summary>
[Serializable]
public class Configuration : IPluginConfiguration
{
    public int Version { get; set; } = 1;

    /// <summary>
    /// Base URL of the Senpan server. Defaults to the production host; there is no
    /// need to enter it, but it stays configurable for dev/self-hosting.
    /// </summary>
    public string ServerUrl { get; set; } = DefaultServerUrl;

    /// <summary>Personal access token ("pat_…") generated on the account page.</summary>
    public string Token { get; set; } = string.Empty;

    /// <summary>Last-used draw delay (seconds) for the bingo game tab.</summary>
    public int DrawDelaySeconds { get; set; }

    /// <summary>Open the live WebSocket automatically once a URL + token are set.</summary>
    public bool AutoConnect { get; set; } = true;

    /// <summary>
    /// When creating a card for a player picked from the nearby list, send them a
    /// /tell with the card's URL. This sends an outgoing chat message on your
    /// behalf — see ChatSender for the ToS caveat — so it is opt-out here.
    /// </summary>
    public bool TellCardUrlOnCreate { get; set; } = true;

    /// <summary>The production server, used as the default and for fresh installs.</summary>
    public const string DefaultServerUrl = "https://apps.senpan.cafe";

    // Not serialized — wired up at load so Save() can round-trip without the caller
    // needing the plugin interface.
    [NonSerialized]
    private IDalamudPluginInterface? pluginInterface;

    public void Initialize(IDalamudPluginInterface pi) => this.pluginInterface = pi;

    public void Save() => this.pluginInterface?.SavePluginConfig(this);

    /// <summary>The public player URL for a card id (…/play/{id}).</summary>
    public string CardUrl(string id) => $"{this.ServerUrl.Trim().TrimEnd('/')}/play/{id}";
}
