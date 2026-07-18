using System;
using System.Collections.Generic;
using Dalamud.Configuration;
using Dalamud.Plugin;
using SenpanCompanion.Services;

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

    /// <summary>
    /// When issuing a Garapon drawing link for a player picked from the nearby list,
    /// /tell them the link. Like <see cref="TellCardUrlOnCreate"/> this drives an
    /// outgoing chat message (ToS caveat in ChatSender), so it defaults OFF (opt-in).
    /// </summary>
    public bool TellGaraponUrlOnCreate { get; set; }

    /// <summary>
    /// When issuing a Stamp Rally card for a player picked from the nearby list,
    /// /tell them the card link. Outgoing chat, so it defaults OFF (opt-in).
    /// </summary>
    public bool TellStampCardUrlOnCreate { get; set; }

    // Editable auto-tell message templates. Placeholders (expanded by the plugin, see
    // TellComposer): <t> = recipient name; <bingocard-link>/<garapon-link>/<stamprally-link>
    // = the relevant link. Defaults reproduce the messages the plugin sent before the
    // templates existed, so enabling a tell behaves the same until it's customized.

    /// <summary>Template for the bingo-card auto-tell (uses <c>&lt;bingocard-link&gt;</c>).</summary>
    public string BingoCardTellTemplate { get; set; } = "Here's your bingo card: <bingocard-link>";

    /// <summary>Template for the Garapon drawing-link auto-tell (uses <c>&lt;garapon-link&gt;</c>).</summary>
    public string GaraponTellTemplate { get; set; } = "Here's your Garapon drawing link: <garapon-link>";

    /// <summary>Template for the Stamp Rally card auto-tell (uses <c>&lt;stamprally-link&gt;</c>).</summary>
    public string StampCardTellTemplate { get; set; } = "Here's your Stamp Rally card: <stamprally-link>";

    /// <summary>
    /// User-defined Timed Text Macros (the account-free Timed Text Macros tool). Persisted
    /// so they survive logout/restart; each macro's per-send progress is saved as it runs,
    /// but macros always reload stopped (see <see cref="Services.TimedMacroRunner"/>).
    /// </summary>
    public List<TimedTextMacro> TimedTextMacros { get; set; } = new();

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

    /// <summary>The public Garapon drawing-link URL for a player token (…/garapon/{token}).</summary>
    public string GaraponUrl(string token) => $"{this.ServerUrl.Trim().TrimEnd('/')}/garapon/{token}";

    /// <summary>The public Stamp Rally card URL for a token (…/stamp-card/{token}).</summary>
    public string StampCardUrl(string token) => $"{this.ServerUrl.Trim().TrimEnd('/')}/stamp-card/{token}";
}
