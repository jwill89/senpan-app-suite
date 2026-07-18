using System;
using System.Collections.Generic;
using System.Numerics;
using System.Threading.Tasks;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using Dalamud.Interface.Windowing;
using Dalamud.Utility;
using SenpanCompanion.Api;
using SenpanCompanion.Services;

namespace SenpanCompanion.Windows;

/// <summary>
/// Main window. Before a token is set it shows the setup panel inline; afterwards a
/// live-status badge above a collapsible left sidebar (Bingo · Festival sections,
/// plus Settings/About) whose selection drives the right-hand content pane. Sections
/// and page links are hidden for permissions the account lacks, mirroring the web
/// sidebar. Each page lazily loads the first time it's viewed (no manual refresh),
/// and the Bingo pages share one live card cache. Settings live on their own page —
/// there is no separate window.
/// </summary>
public sealed class MainWindow : Window, IDisposable
{
    private readonly Plugin plugin;
    private readonly Configuration config;
    private readonly ApiClient api;
    private readonly LiveConnection live;
    private readonly Session session;
    private readonly CardCache cardCache;
    private readonly BingoGameTab game;
    private readonly BingoCardsTab cards;
    private readonly BingoWinnersTab winnersLog;
    private readonly RaffleTab raffle;
    private readonly GaraponTab garapon;
    private readonly StampRallyTab stampRally;
    private readonly RollsTab rolls;
    private readonly TimedMacrosTab timedMacros;

    private const float NavWidth = 175f;

    /// <summary>Which sidebar page is showing. navInitialized guards a one-time "jump
    /// to the first page you can access" once the session's permissions have loaded.</summary>
    private Page currentPage = Page.BingoGame;
    private bool navInitialized;

    // Armed while no token is set (see Draw); on the first frame after a token connects,
    // it lands the view on the first accessible suite page instead of leaving it on the
    // setup screen.
    private bool landOnConnect;

    // Settings panel (shared between first-run setup and the Settings tab). The
    // URL/token are staged copies; auto-connect/tell write straight to config.
    private string settingsUrl = string.Empty;
    private string settingsToken = string.Empty;
    private string settingsStatus = string.Empty;
    private bool settingsTesting;
    private bool settingsPrimed;

    public MainWindow(Plugin plugin, Configuration config, ApiClient api, LiveConnection live, NearbyPlayers nearby, ChatSender chat, RollTracker rollTracker, TimedMacroRunner timedMacroRunner)
        : base("Senpan Admin Companion###SenpanMain")
    {
        this.plugin = plugin;
        this.config = config;
        this.api = api;
        this.live = live;
        this.session = new Session(api);
        this.cardCache = new CardCache(api, live);
        this.game = new BingoGameTab(api, config, live, this.cardCache);
        this.cards = new BingoCardsTab(api, nearby, config, chat, this.cardCache);
        this.winnersLog = new BingoWinnersTab(api);
        this.raffle = new RaffleTab(api, nearby);
        this.garapon = new GaraponTab(api, nearby, config, chat);
        this.stampRally = new StampRallyTab(api, nearby, config, chat);
        this.rolls = new RollsTab(rollTracker);
        this.timedMacros = new TimedMacrosTab(config, timedMacroRunner);
        this.SizeConstraints = new WindowSizeConstraints
        {
            // The sidebar + content split needs more horizontal room than the old
            // tab bar did.
            MinimumSize = new Vector2(760, 460),
            MaximumSize = new Vector2(1500, 1100),
        };
    }

    public void Dispose()
    {
        this.game.Dispose();
        this.cardCache.Dispose();
    }

    public override void OnOpen() => SyncSettingsFields();

    public override void Draw()
    {
        // The Rolls tool (plus Settings/About) are account-free, so the window always
        // renders the sidebar + content layout — even before a token is set, where the
        // suite sections stay hidden and setup lands on the Settings page.
        if (!string.IsNullOrWhiteSpace(this.config.Token))
        {
            this.session.EnsureLoaded();
            // Once permissions are known, land on the first page the account can reach
            // (the default is Bingo Game; an account without it shouldn't stare at a
            // blank panel). Runs once per session load — token changes re-arm it.
            if (this.session.Loaded && !this.navInitialized)
            {
                this.navInitialized = true;
                if (!IsAccessible(this.currentPage))
                    this.currentPage = FirstAccessiblePage();
            }
            // A just-connected first-run account was parked on Settings by the no-token
            // branch below; move it onto the first usable suite page (its accessible
            // check above would otherwise keep it on the always-accessible Settings page).
            if (this.session.Loaded && this.landOnConnect)
            {
                this.landOnConnect = false;
                this.currentPage = FirstAccessiblePage();
            }

            DrawTopBar();
            DrawSessionNotice();
        }
        else
        {
            // No account yet: park suite pages on Settings so a fresh install sees the
            // setup panel, but leave the account-free tools (Rolls/About) reachable. Arm
            // the one-time land-on-connect jump for when a token is finally entered.
            this.landOnConnect = true;
            if (this.currentPage is not (Page.Rolls or Page.TimedMacros or Page.Settings or Page.About))
                this.currentPage = Page.Settings;
            ImGui.TextColored(new Vector4(0.85f, 0.55f, 0.2f, 1f), "○ Not connected");
            UiText.WrappedDisabled("Connect your account on the Settings page. The Rolls and Timed Text Macros tools work without an account.");
            ImGui.Separator();
        }

        // The old tab bar re-synced the staged settings fields whenever the Settings
        // tab lost focus; with a sidebar there is no such event, so re-arm the prime
        // whenever we're off the Settings page.
        if (this.currentPage != Page.Settings)
            this.settingsPrimed = false;

        // Two-pane layout: a fixed-width nav child on the left, the active page on the
        // right. Child regions (not table cells) host the pages so their full-height
        // scrolling tables get a properly bounded region and behave exactly as they did
        // as top-level tabs. EndChild is always called regardless of BeginChild's
        // return (ImGui requires it). Borderless — the ImGuiChildFlags border member's
        // name varies across ImGui versions, and the layout reads clearly without it.
        ImGui.BeginChild("##senpanNav", new Vector2(NavWidth, 0f));
        DrawSidebar();
        ImGui.EndChild();
        ImGui.SameLine();
        ImGui.BeginChild("##senpanContent", new Vector2(0f, 0f));
        DrawContent();
        ImGui.EndChild();
    }

    // ── Sidebar navigation ─────────────────────────────────────────────────────

    private void DrawSidebar()
    {
        var s = this.session;

        // Section headers are pure accordion toggles (they never navigate); a whole
        // section is hidden when the account can reach none of its pages, mirroring
        // the web sidebar.
        var showBingo = s.Has(Perms.BingoGame) || s.Has(Perms.BingoCards) || s.Has(Perms.BingoWinnersLog);
        if (showBingo && ImGui.CollapsingHeader("Bingo###secBingo", ImGuiTreeNodeFlags.DefaultOpen))
        {
            NavItem("Game", Page.BingoGame, s.Has(Perms.BingoGame));
            NavItem("Cards", Page.BingoCards, s.Has(Perms.BingoCards));
            NavItem("Winners", Page.BingoWinners, s.Has(Perms.BingoWinnersLog));
        }

        var showFestival = s.Has(Perms.TeahouseRaffles) || s.Has(Perms.FestivalGarapon) || s.Has(Perms.FestivalStampRally);
        if (showFestival && ImGui.CollapsingHeader("Festival###secFestival", ImGuiTreeNodeFlags.DefaultOpen))
        {
            NavItem("Raffles", Page.Raffles, s.Has(Perms.TeahouseRaffles));
            NavItem("Garapon", Page.Garapon, s.Has(Perms.FestivalGarapon));
            NavItem("Garapon Draw Log", Page.GaraponLog, s.Has(Perms.FestivalGarapon));
            NavItem("Stamp Rally", Page.StampRally, s.Has(Perms.FestivalStampRally));
            NavItem("Stamp Rally Log", Page.StampRallyLog, s.Has(Perms.FestivalStampRally));
        }

        // Account-free tools live under their own always-visible group, mirroring the
        // Bingo/Festival accordions (a token-less install sees only this one).
        ImGui.Spacing();
        if (ImGui.CollapsingHeader("General###secGeneral", ImGuiTreeNodeFlags.DefaultOpen))
        {
            NavItem("Rolls", Page.Rolls, true);
            NavItem("Timed Text Macros", Page.TimedMacros, true);
            NavItem("Settings", Page.Settings, true);
            NavItem("About", Page.About, true);
        }
    }

    private void NavItem(string label, Page page, bool visible, bool indent = true)
    {
        if (!visible)
            return;
        if (indent)
            ImGui.Indent(12f);
        if (ImGui.Selectable($"{label}##nav{page}", this.currentPage == page))
            this.currentPage = page;
        if (indent)
            ImGui.Unindent(12f);
    }

    private void DrawContent()
    {
        if (!IsAccessible(this.currentPage))
        {
            ImGui.TextDisabled("Select a page from the menu.");
            return;
        }

        switch (this.currentPage)
        {
            case Page.BingoGame:
                if (this.session.Has(Perms.BingoCards))
                    this.cardCache.EnsureLoaded(); // winner-name lookups
                this.game.EnsureLoaded();
                this.game.Draw();
                break;
            case Page.BingoCards:
                this.cardCache.EnsureLoaded();
                this.cards.Draw();
                break;
            case Page.BingoWinners:
                this.winnersLog.EnsureLoaded();
                this.winnersLog.Draw();
                break;
            case Page.Raffles:
                this.raffle.EnsureLoaded();
                this.raffle.Draw();
                break;
            case Page.Garapon:
                this.garapon.EnsureLoaded();
                this.garapon.DrawManage();
                break;
            case Page.GaraponLog:
                this.garapon.EnsureLoaded();
                this.garapon.DrawLog();
                break;
            case Page.StampRally:
                this.stampRally.EnsureLoaded();
                this.stampRally.DrawManage();
                break;
            case Page.StampRallyLog:
                this.stampRally.EnsureLoaded();
                this.stampRally.DrawLog();
                break;
            case Page.Rolls:
                this.rolls.Draw();
                break;
            case Page.TimedMacros:
                this.timedMacros.Draw();
                break;
            case Page.Settings:
                // Re-sync the staged fields from config the first frame Settings shows,
                // so a token set on first-run (or elsewhere) is reflected, not clobbered.
                if (!this.settingsPrimed)
                {
                    SyncSettingsFields();
                    this.settingsPrimed = true;
                }
                DrawSettingsPanel();
                break;
            case Page.About:
                DrawAboutTab();
                break;
        }
    }

    private bool IsAccessible(Page page) => page switch
    {
        Page.BingoGame => this.session.Has(Perms.BingoGame),
        Page.BingoCards => this.session.Has(Perms.BingoCards),
        Page.BingoWinners => this.session.Has(Perms.BingoWinnersLog),
        Page.Raffles => this.session.Has(Perms.TeahouseRaffles),
        Page.Garapon or Page.GaraponLog => this.session.Has(Perms.FestivalGarapon),
        Page.StampRally or Page.StampRallyLog => this.session.Has(Perms.FestivalStampRally),
        Page.Rolls or Page.TimedMacros or Page.Settings or Page.About => true,
        _ => false,
    };

    private Page FirstAccessiblePage()
    {
        Page[] order =
        {
            Page.BingoGame, Page.BingoCards, Page.BingoWinners, Page.Raffles,
            Page.Garapon, Page.GaraponLog, Page.StampRally, Page.StampRallyLog, Page.Rolls, Page.TimedMacros, Page.Settings,
        };
        foreach (var p in order)
            if (IsAccessible(p))
                return p;
        return Page.Settings; // always reachable
    }

    private enum Page
    {
        BingoGame,
        BingoCards,
        BingoWinners,
        Raffles,
        Garapon,
        GaraponLog,
        StampRally,
        StampRallyLog,
        Rolls,
        TimedMacros,
        Settings,
        About,
    }

    private void DrawTopBar()
    {
        var connected = this.live.Connected;
        var color = connected
            ? new Vector4(0.3f, 0.85f, 0.35f, 1f)
            : new Vector4(0.85f, 0.55f, 0.2f, 1f);
        ImGui.TextColored(color, connected ? "● Live" : "○ Offline");
        ImGui.SameLine();
        ImGui.TextDisabled(connected ? "receiving live updates" : "connecting…");
        ImGui.Separator();
    }

    private void DrawSessionNotice()
    {
        if (!this.session.Loaded)
        {
            if (this.session.LoadFailed)
                UiText.WrappedColored(new Vector4(0.9f, 0.5f, 0.4f, 1f),
                    "Couldn't verify your account — check your token on the Settings tab.");
            else
                ImGui.TextDisabled("Verifying your account…");
            return;
        }

        var anyPanel = this.session.Has(Perms.BingoGame) || this.session.Has(Perms.BingoCards)
            || this.session.Has(Perms.BingoWinnersLog) || this.session.Has(Perms.TeahouseRaffles)
            || this.session.Has(Perms.FestivalGarapon) || this.session.Has(Perms.FestivalStampRally);
        if (!anyPanel)
            UiText.WrappedDisabled("Your account has no Senpan panel permissions — ask an admin to grant access.");
    }

    // ── About ─────────────────────────────────────────────────────────────────

    private static readonly Vector4 LinkColor = new(0.40f, 0.70f, 1f, 1f);

    private void DrawAboutTab()
    {
        var logo = Plugin.TextureProvider.GetFromFile(Plugin.LogoPath).GetWrapOrEmpty();
        if (logo.Handle != IntPtr.Zero && logo.Width > 0)
        {
            const float maxWidth = 340f;
            var avail = ImGui.GetContentRegionAvail().X;
            var target = Math.Min(maxWidth, avail); // shrink to fit a narrow window
            var scale = Math.Min(1f, target / logo.Width);
            var size = new Vector2(logo.Width * scale, logo.Height * scale);
            if (avail > size.X)
                ImGui.SetCursorPosX(ImGui.GetCursorPosX() + ((avail - size.X) * 0.5f));
            ImGui.Image(logo.Handle, size);
        }

        ImGui.Spacing();
        ImGui.Separator();
        ImGui.Spacing();

        ImGui.TextUnformatted("Made by");
        ImGui.SameLine();
        Hyperlink("MathDad", "https://mathdad.me");

        ImGui.TextUnformatted("Made for");
        ImGui.SameLine();
        Hyperlink("Senpan Tea House", "https://senpan.cafe");

        ImGui.Spacing();
        var version = typeof(Plugin).Assembly.GetName().Version;
        ImGui.TextDisabled(version != null ? $"Senpan Admin Companion v{version.ToString(3)}" : "Senpan Admin Companion");
    }

    private static void Hyperlink(string text, string url)
    {
        ImGui.TextColored(LinkColor, text);
        if (ImGui.IsItemHovered())
        {
            ImGui.SetMouseCursor(ImGuiMouseCursor.Hand);
            var min = ImGui.GetItemRectMin();
            var max = ImGui.GetItemRectMax();
            ImGui.GetWindowDrawList().AddLine(new Vector2(min.X, max.Y), new Vector2(max.X, max.Y), ImGui.GetColorU32(LinkColor));
        }
        if (ImGui.IsItemClicked())
            Util.OpenLink(url);
    }

    // ── Settings ──────────────────────────────────────────────────────────────

    private void SyncSettingsFields()
    {
        this.settingsUrl = this.config.ServerUrl;
        this.settingsToken = this.config.Token;
    }

    private static readonly Vector4 WarnColor = new(0.9f, 0.65f, 0.2f, 1f);

    // A representative recipient name for the settings-time split estimate — the real
    // name is only known when a card is issued. Uses the maximum FFXIV name width
    // (forename 15 + space + surname 15 = 31) so the estimate is a conservative upper
    // bound: it errs toward showing the split warning rather than silently splitting a
    // <t>-containing message at send time. (Link widths in the estimate are already
    // exact — real server URL + real 32/6-char token widths.)
    private static readonly string ReprName = new('W', 31);

    // Auto-tell checkbox + (when enabled) a template editor, a placeholder hint, and a
    // live "this will split into N tells" warning estimated from the real server URL +
    // representative link/name widths. Config-backed via get/set delegates; saved on
    // each edit (the config file is tiny) so an edit can't be lost by navigating away.
    private void DrawTellSetting(
        string checkboxLabel,
        Func<bool> getEnabled, Action<bool> setEnabled,
        Func<string> getTemplate, Action<string> setTemplate,
        string editorId, string placeholderHint,
        Func<string, int> estimateParts)
    {
        var enabled = getEnabled();
        if (ImGui.Checkbox(checkboxLabel, ref enabled))
        {
            setEnabled(enabled);
            this.config.Save();
        }
        if (!enabled)
            return;

        ImGui.Indent(22f);
        var tmpl = getTemplate();
        if (ImGui.InputTextMultiline(editorId, ref tmpl, 2048, new Vector2(-1f, 54f)))
        {
            setTemplate(tmpl);
            this.config.Save();
        }
        UiText.WrappedDisabled(placeholderHint);

        var parts = estimateParts(getTemplate());
        if (parts >= 2)
            UiText.WrappedColored(WarnColor, parts == 2
                ? "This message is too long and will be split into two separate tells."
                : $"This message is too long and will be split into {parts} separate tells.");
        ImGui.Unindent(22f);
    }

    // Representative placeholder expansions for the settings-time split estimate: the
    // real server URL + a token of the real width (garapon/stamp = 32 hex chars, bingo
    // card id = 6) so the link is measured at the full width it will render as.
    private Dictionary<string, string> BingoReprValues() => new()
    {
        [TellComposer.TargetToken] = ReprName,
        [TellComposer.BingoCardLinkToken] = this.config.CardUrl(new string('x', 6)),
    };

    private Dictionary<string, string> GaraponReprValues() => new()
    {
        [TellComposer.TargetToken] = ReprName,
        [TellComposer.GaraponLinkToken] = this.config.GaraponUrl(new string('x', 32)),
    };

    private Dictionary<string, string> StampReprValues() => new()
    {
        [TellComposer.TargetToken] = ReprName,
        [TellComposer.StampCardLinkToken] = this.config.StampCardUrl(new string('x', 32)),
    };

    private void DrawSettingsPanel()
    {
        Ui.Section(FontAwesomeIcon.Plug, "Connection");
        ImGui.TextWrapped(
            "Generate a personal access token on the website (User Options → Access " +
            "Token) and paste it here. The token signs in as your account, so the " +
            "plugin can only do what your account is allowed to.");
        ImGui.Spacing();

        ImGui.SetNextItemWidth(360);
        ImGui.InputText("Server URL", ref this.settingsUrl, 512);
        ImGui.SetNextItemWidth(360);
        ImGui.InputText("Access token", ref this.settingsToken, 256, ImGuiInputTextFlags.Password);

        var auto = this.config.AutoConnect;
        if (ImGui.Checkbox("Auto-connect live updates", ref auto))
        {
            this.config.AutoConnect = auto;
            this.config.Save();
            this.plugin.OnConnectionSettingsChanged();
        }

        Ui.Section(FontAwesomeIcon.Bullhorn, "Automatic tells");
        DrawTellSetting(
            "/tell the bingo card link when creating from the nearby list",
            () => this.config.TellCardUrlOnCreate, v => this.config.TellCardUrlOnCreate = v,
            () => this.config.BingoCardTellTemplate, v => this.config.BingoCardTellTemplate = v,
            "##bingotelltmpl",
            "Placeholders: <t> = player name  ·  <bingocard-link> = the card link.",
            t => TellComposer.PartCount(t, BingoReprValues()));

        DrawTellSetting(
            "/tell the Garapon drawing link when issuing from the nearby list",
            () => this.config.TellGaraponUrlOnCreate, v => this.config.TellGaraponUrlOnCreate = v,
            () => this.config.GaraponTellTemplate, v => this.config.GaraponTellTemplate = v,
            "##garapontelltmpl",
            "Placeholders: <t> = player name  ·  <garapon-link> = the drawing link.",
            t => TellComposer.PartCount(t, GaraponReprValues()));

        DrawTellSetting(
            "/tell the Stamp Rally card link when issuing from the nearby list",
            () => this.config.TellStampCardUrlOnCreate, v => this.config.TellStampCardUrlOnCreate = v,
            () => this.config.StampCardTellTemplate, v => this.config.StampCardTellTemplate = v,
            "##stamptelltmpl",
            "Placeholders: <t> = player name  ·  <stamprally-link> = the card link.",
            t => TellComposer.PartCount(t, StampReprValues()));

        UiText.WrappedDisabled("Each sends an outgoing chat message on your behalf — a long message splits into multiple tells. See the README's ToS note.");

        ImGui.Spacing();
        if (Ui.PrimaryButton("Save"))
            SaveSettings();
        ImGui.SameLine();
        if (this.settingsTesting)
            ImGui.BeginDisabled();
        if (Ui.Button("Save & Test Connection"))
        {
            SaveSettings();
            TestSettings();
            // The user explicitly asked to see the connection result — don't let the
            // first-run land-on-connect jump yank them off Settings before they read it.
            this.landOnConnect = false;
        }
        if (this.settingsTesting)
            ImGui.EndDisabled();

        if (!string.IsNullOrEmpty(this.settingsStatus))
        {
            ImGui.Spacing();
            ImGui.Separator();
            ImGui.TextWrapped(this.settingsStatus);
        }
    }

    private void SaveSettings()
    {
        this.config.ServerUrl = this.settingsUrl.Trim();
        this.config.Token = this.settingsToken.Trim();
        this.config.Save();
        this.plugin.OnConnectionSettingsChanged();
        this.session.MarkStale();
        this.cardCache.MarkStale();
        this.game.MarkStale();
        this.winnersLog.MarkStale();
        this.raffle.MarkStale();
        this.garapon.MarkStale();
        this.stampRally.MarkStale();
        // Re-arm the "jump to first accessible page" once permissions reload — a new
        // token may grant or revoke access to the current page.
        this.navInitialized = false;
        this.settingsStatus = "Saved.";
    }

    private void TestSettings()
    {
        this.settingsTesting = true;
        this.settingsStatus = "Testing…";
        _ = Task.Run(async () =>
        {
            try
            {
                var res = await this.api.CheckAuthAsync();
                if (res is { Authenticated: true, User: not null })
                {
                    var who = res.User.IsAdmin
                        ? $"{res.User.Username} (admin)"
                        : $"{res.User.Username} — permissions: {string.Join(", ", res.User.Permissions)}";
                    this.settingsStatus = $"Connected as {who}.";
                }
                else
                {
                    this.settingsStatus = "Reached the server, but the token was not accepted. Generate a fresh one and try again.";
                }
            }
            catch (Exception ex)
            {
                this.settingsStatus = $"Connection failed: {ex.Message}";
            }
            finally
            {
                this.settingsTesting = false;
            }
        });
    }
}
