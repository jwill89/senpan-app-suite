using System;
using System.Numerics;
using System.Threading.Tasks;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface.Windowing;
using Dalamud.Utility;
using SenpanCompanion.Api;
using SenpanCompanion.Services;

namespace SenpanCompanion.Windows;

/// <summary>
/// Main window. Before a token is set it shows the setup panel inline; afterwards a
/// live-status badge over the Bingo Game / Bingo Cards / Bingo Winners / Raffles /
/// Settings tabs. Each data tab lazily loads the first time it's viewed (no manual
/// refresh), and the Bingo tabs share one live card cache. Settings live in the
/// Settings tab — there is no separate window.
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

    // Settings panel (shared between first-run setup and the Settings tab). The
    // URL/token are staged copies; auto-connect/tell write straight to config.
    private string settingsUrl = string.Empty;
    private string settingsToken = string.Empty;
    private string settingsStatus = string.Empty;
    private bool settingsTesting;
    private bool settingsPrimed;

    public MainWindow(Plugin plugin, Configuration config, ApiClient api, LiveConnection live, NearbyPlayers nearby, ChatSender chat)
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
        this.SizeConstraints = new WindowSizeConstraints
        {
            MinimumSize = new Vector2(560, 440),
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
        if (string.IsNullOrWhiteSpace(this.config.Token))
        {
            ImGui.TextWrapped("Welcome to Senpan Companion. Connect to your account to get started:");
            ImGui.Spacing();
            DrawSettingsPanel();
            return;
        }

        this.session.EnsureLoaded();
        DrawTopBar();
        DrawSessionNotice();

        if (!ImGui.BeginTabBar("SenpanTabs"))
            return;

        // Tabs appear only for permissions the account holds (admins hold all),
        // mirroring the web, which hides nav sections you can't access.
        if (this.session.Has(Perms.BingoGame) && ImGui.BeginTabItem("Bingo Game"))
        {
            if (this.session.Has(Perms.BingoCards))
                this.cardCache.EnsureLoaded(); // winner-name lookups
            this.game.EnsureLoaded();
            this.game.Draw();
            ImGui.EndTabItem();
        }

        if (this.session.Has(Perms.BingoCards) && ImGui.BeginTabItem("Bingo Cards"))
        {
            this.cardCache.EnsureLoaded();
            this.cards.Draw();
            ImGui.EndTabItem();
        }

        if (this.session.Has(Perms.BingoWinnersLog) && ImGui.BeginTabItem("Bingo Winners"))
        {
            this.winnersLog.EnsureLoaded();
            this.winnersLog.Draw();
            ImGui.EndTabItem();
        }

        if (this.session.Has(Perms.TeahouseRaffles) && ImGui.BeginTabItem("Raffles"))
        {
            this.raffle.EnsureLoaded();
            this.raffle.Draw();
            ImGui.EndTabItem();
        }

        if (ImGui.BeginTabItem("Settings"))
        {
            // Re-sync the staged fields from config each time the tab is opened, so
            // a token set on first-run (or elsewhere) is reflected, not clobbered.
            if (!this.settingsPrimed)
            {
                SyncSettingsFields();
                this.settingsPrimed = true;
            }
            DrawSettingsPanel();
            ImGui.EndTabItem();
        }
        else
        {
            this.settingsPrimed = false;
        }

        if (ImGui.BeginTabItem("About"))
        {
            DrawAboutTab();
            ImGui.EndTabItem();
        }

        ImGui.EndTabBar();
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
                ImGui.TextColored(new Vector4(0.9f, 0.5f, 0.4f, 1f),
                    "Couldn't verify your account — check your token on the Settings tab.");
            else
                ImGui.TextDisabled("Verifying your account…");
            return;
        }

        var anyPanel = this.session.Has(Perms.BingoGame) || this.session.Has(Perms.BingoCards)
            || this.session.Has(Perms.BingoWinnersLog) || this.session.Has(Perms.TeahouseRaffles);
        if (!anyPanel)
            ImGui.TextDisabled("Your account has no Senpan panel permissions — ask an admin to grant access.");
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

    private void DrawSettingsPanel()
    {
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

        var tell = this.config.TellCardUrlOnCreate;
        if (ImGui.Checkbox("/tell the card URL when creating from the nearby list", ref tell))
        {
            this.config.TellCardUrlOnCreate = tell;
            this.config.Save();
        }
        ImGui.TextDisabled("    Sends an outgoing chat message on your behalf — see the README's ToS note.");

        ImGui.Spacing();
        if (ImGui.Button("Save"))
            SaveSettings();
        ImGui.SameLine();
        if (this.settingsTesting)
            ImGui.BeginDisabled();
        if (ImGui.Button("Save & Test Connection"))
        {
            SaveSettings();
            TestSettings();
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
