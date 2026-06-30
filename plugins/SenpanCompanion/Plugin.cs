using System;
using Dalamud.Game.Command;
using Dalamud.IoC;
using Dalamud.Interface.Windowing;
using Dalamud.Plugin;
using Dalamud.Plugin.Services;
using SenpanCompanion.Api;
using SenpanCompanion.Services;
using SenpanCompanion.Windows;

namespace SenpanCompanion;

/// <summary>
/// Senpan Companion: a second UI over the Senpan App Suite server. It is purely an
/// API client — the server stays the single source of truth, and every action here
/// still broadcasts to the website via the server's WebSocket.
/// </summary>
public sealed class Plugin : IDalamudPlugin
{
    [PluginService] internal static IDalamudPluginInterface PluginInterface { get; private set; } = null!;
    [PluginService] internal static ICommandManager CommandManager { get; private set; } = null!;
    [PluginService] internal static IClientState ClientState { get; private set; } = null!;
    [PluginService] internal static IObjectTable ObjectTable { get; private set; } = null!;
    [PluginService] internal static IFramework Framework { get; private set; } = null!;
    [PluginService] internal static ITextureProvider TextureProvider { get; private set; } = null!;
    [PluginService] internal static IPluginLog Log { get; private set; } = null!;

    /// <summary>Path to the bundled Senpan Tea House logo (copied next to the DLL).</summary>
    internal static string LogoPath => System.IO.Path.Combine(
        PluginInterface.AssemblyLocation.Directory!.FullName, "Data", "senpan-logo.png");

    private const string CommandName = "/senpan";

    public readonly WindowSystem WindowSystem = new("SenpanCompanion");

    private readonly Configuration config;
    private readonly ApiClient api;
    private readonly LiveConnection live;
    private readonly MainWindow mainWindow;

    public Plugin()
    {
        this.config = PluginInterface.GetPluginConfig() as Configuration ?? new Configuration();
        this.config.Initialize(PluginInterface);
        if (string.IsNullOrWhiteSpace(this.config.ServerUrl))
        {
            this.config.ServerUrl = Configuration.DefaultServerUrl;
            this.config.Save();
        }

        this.api = new ApiClient(this.config, Log);
        this.live = new LiveConnection(this.config, Log, Framework);
        var nearby = new NearbyPlayers(ObjectTable);
        var chat = new ChatSender(Framework, Log);

        this.mainWindow = new MainWindow(this, this.config, this.api, this.live, nearby, chat);
        this.WindowSystem.AddWindow(this.mainWindow);

        CommandManager.AddHandler(CommandName, new CommandInfo(OnCommand)
        {
            HelpMessage = "Open the Senpan Companion window (settings are a tab inside).",
        });

        PluginInterface.UiBuilder.Draw += DrawUi;
        PluginInterface.UiBuilder.OpenConfigUi += OpenMain;
        PluginInterface.UiBuilder.OpenMainUi += ToggleMain;

        if (this.config.AutoConnect)
            this.live.Start();
    }

    public void Dispose()
    {
        PluginInterface.UiBuilder.Draw -= DrawUi;
        PluginInterface.UiBuilder.OpenConfigUi -= OpenMain;
        PluginInterface.UiBuilder.OpenMainUi -= ToggleMain;
        CommandManager.RemoveHandler(CommandName);

        this.WindowSystem.RemoveAllWindows();
        this.mainWindow.Dispose();
        this.live.Dispose();
        this.api.Dispose();
    }

    /// <summary>Reconnect (or stop) the live WebSocket after the server URL/token changes.</summary>
    public void OnConnectionSettingsChanged()
    {
        if (this.config.AutoConnect)
            this.live.Start();
        else
            this.live.Stop();
    }

    private void OnCommand(string command, string args)
    {
        // Both open the main window; settings are a tab inside it now.
        if (args.Trim().Equals("config", StringComparison.OrdinalIgnoreCase))
            this.mainWindow.IsOpen = true;
        else
            this.mainWindow.Toggle();
    }

    private void DrawUi() => this.WindowSystem.Draw();

    private void OpenMain() => this.mainWindow.IsOpen = true;

    private void ToggleMain() => this.mainWindow.Toggle();
}
