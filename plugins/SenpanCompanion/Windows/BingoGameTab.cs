using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using System.Numerics;
using System.Threading.Tasks;
using Dalamud.Bindings.ImGui;
using Dalamud.Interface;
using SenpanCompanion.Api;
using SenpanCompanion.Services;

namespace SenpanCompanion.Windows;

/// <summary>
/// Bingo Game tab, mirroring the web admin Game view: a New Game setup (preset +
/// win patterns + game details) and the live Current Game view (draw controls,
/// last-drawn, the B-I-N-G-O called-numbers grid, winners, active patterns). Draws,
/// winners, the half-time prompt and its answer, the draw delay and the details arrive
/// over the live stream from whichever console or website changed them; a chime plays
/// when a new winner appears. Card -> player-name lookups come from the shared cache.
/// </summary>
internal sealed class BingoGameTab : TabBase, IDisposable
{
    private static readonly int[] DrawDelayOptions = { 0, 3, 5, 10, 15, 20, 30, 45, 60 };
    private static readonly int[] AutoIntervalOptions = { 10, 15, 20, 30, 45, 60, 90, 120, 180, 300 };
    private const int DefaultAutoInterval = 30;
    private static readonly string[] BingoLetters = { "B", "I", "N", "G", "O" };

    private static readonly Vector4 GridHeaderColor = new(0.72f, 0.72f, 0.78f, 1f);
    private static readonly Vector4 GridDimColor = new(0.45f, 0.45f, 0.50f, 1f);
    private static readonly Vector4 GridHotColor = new(1f, 1f, 1f, 1f);

    private readonly SenpanApi api;
    private readonly LiveConnection live;
    private readonly CardCache cardCache;

    private List<Pattern> patterns = new();
    private List<GamePreset> presets = new();
    private List<FrequentWinner> frequentWinners = new();
    private readonly HashSet<long> selectedPatterns = new();
    private long selectedPresetId;

    // Bulk expand/collapse of the win-pattern category headers: pendingPatternOpen forces
    // every header open/closed for one frame; patternsCollapsed drives the toggle label.
    private bool? pendingPatternOpen;
    private bool patternsCollapsed;

    private Game? game;
    private List<string> winners = new();
    private string? lastCalled;
    private string gameDetails = string.Empty;

    // Whether the details box has the keyboard: a details change from another console
    // waits rather than replacing what is being typed.
    private bool detailsEditing;

    /// <summary>Seconds a drawn number waits before players get it. One value for every
    /// console, kept by the server: read with the game, followed live, saved on change.</summary>
    private long drawDelay;

    // New Game auto-draw controls (session-local, like the web's new-game form).
    private bool newGameAuto;
    private int newGameAutoInterval = DefaultAutoInterval;

    private bool openHalftimePopup;
    private bool closeHalftimePopup;
    private bool halftimeAutoPaused;

    /// <summary>
    /// The game the pending half-time prompt belongs to, 0 for none. An answer names it,
    /// so one left open across a new game is refused rather than alerting players who never
    /// reached half time.
    /// </summary>
    private long halftimeGameId;

    /// <summary>
    /// When this page last drew a frame. The half-time prompt is a modal on THIS page, so
    /// it is invisible while the window is closed or showing something else - and the
    /// server has paused auto-draw waiting for an answer, so the game just stalls with
    /// nothing said. This is how the prompt knows to speak up.
    /// </summary>
    private DateTime lastDrawnAt = DateTime.MinValue;

    private bool openEndGamePopup;
    private bool closeEndGamePopup;
    private readonly HashSet<string> endGameSelected = new();

    /// <summary>The game the End Game confirm was opened for: ending names it, so a confirm
    /// left open while another console starts a new game can't end that one.</summary>
    private long endGameId;

    private bool openViewCard;
    private Card? viewCard;
    private HashSet<(int, int)> viewMatched = new();

    public BingoGameTab(SenpanApi api, LiveConnection live, CardCache cardCache)
    {
        this.api = api;
        this.live = live;
        this.cardCache = cardCache;

        this.live.Draw += this.OnDraw;
        this.live.GameStarted += this.OnGameStarted;
        this.live.GameEnded += this.OnGameEnded;
        this.live.PatternsChanged += this.MarkStale;
        this.live.Yoever += this.OnYoever;
        this.live.YoeverSwitched += this.OnYoeverSwitched;
        this.live.AutoDraw += this.OnAutoDraw;
        this.live.HalftimePrompt += this.OnHalftimePrompt;
        this.live.HalftimeResolved += this.OnHalftimeResolved;
        this.live.DrawDelay += this.OnDrawDelay;
        this.live.GameDetails += this.OnGameDetails;
    }

    public void Dispose()
    {
        this.live.Draw -= this.OnDraw;
        this.live.GameStarted -= this.OnGameStarted;
        this.live.GameEnded -= this.OnGameEnded;
        this.live.PatternsChanged -= this.MarkStale;
        this.live.Yoever -= this.OnYoever;
        this.live.YoeverSwitched -= this.OnYoeverSwitched;
        this.live.AutoDraw -= this.OnAutoDraw;
        this.live.HalftimePrompt -= this.OnHalftimePrompt;
        this.live.HalftimeResolved -= this.OnHalftimeResolved;
        this.live.DrawDelay -= this.OnDrawDelay;
        this.live.GameDetails -= this.OnGameDetails;
    }

    protected override async Task LoadAsync()
    {
        // Patterns, presets and frequent-winners are gated by their own permissions
        // (bingo-patterns / bingo-presets / bingo-winners-log). Fetch them optionally,
        // so an operator with only bingo-game still gets the game state and can draw -
        // a missing sub-permission just leaves that section empty.
        var patternsRes = await OptionalAsync(() => this.api.ListPatternsAsync());
        var presetsRes = await OptionalAsync(() => this.api.ListPresetsAsync());
        var state = await this.api.GetGameAsync();
        var freqRes = await OptionalAsync(() => this.api.ListFrequentWinnersAsync());
        await Apply(() =>
        {
            this.patterns = patternsRes?.Patterns.ToList() ?? new List<Pattern>();
            this.presets = presetsRes?.Presets.ToList() ?? new List<GamePreset>();
            this.frequentWinners = freqRes?.Winners.ToList() ?? new List<FrequentWinner>();
            this.TakeState(state);
        });
    }

    // Enrichment fetch that yields null on a permission/API error, so a missing
    // sub-permission doesn't blank the whole tab.
    private static async Task<T?> OptionalAsync<T>(Func<Task<T>> fetch) where T : class
    {
        try
        {
            return await fetch();
        }
        catch (ApiException)
        {
            return null;
        }
    }

    /// <summary>
    /// Takes the game as the server answers it. The answer carries what a console opening
    /// mid-way needs: the draw delay, the last number called, and a half-time prompt still
    /// waiting for an answer, which opens here as it would have on the draw.
    /// </summary>
    private void TakeState(GameState state)
    {
        if (state.Game == null || state.Game.Id != this.game?.Id)
            this.ClearHalftimePrompt();
        this.game = state.Game;
        this.winners = state.Winners.ToList();
        this.drawDelay = state.DrawDelay;
        if (!this.detailsEditing)
            this.gameDetails = state.GameDetails;
        this.lastCalled = state.Game?.LastDraw is { } last ? $"{last.Letter}-{last.Number}" : null;
        if (state.Game?.Halftime is { } pending && pending.GameId != this.halftimeGameId)
            this.ShowHalftime(pending.GameId, pending.AutoPaused, alert: false);
    }

    public void Draw()
    {
        // Stamped every frame this page is visible, so a prompt arriving later can
        // tell whether the operator is actually looking at it (see OnHalftimePrompt).
        this.lastDrawnAt = DateTime.UtcNow;
        this.DrawStatusLine();

        if (this.game == null)
            this.DrawNewGame();
        else
            this.DrawCurrentGame(this.game);

        this.DrawHalftimePopup();
        this.DrawEndGamePopup();
        this.DrawViewCardPopup();
    }

    // -- New game -------------------------------------------------------------

    private void DrawNewGame()
    {
        if (this.patterns.Count == 0)
        {
            UiText.WrappedDisabled("No win patterns exist yet. Create some on the website first.");
            return;
        }

        if (this.presets.Count > 0)
        {
            var current = this.presets.FirstOrDefault(p => p.Id == this.selectedPresetId);
            ImGui.SetNextItemWidth(220);
            if (ImGui.BeginCombo("Preset", current?.Name ?? "- None -"))
            {
                if (ImGui.Selectable("- None -", this.selectedPresetId == 0))
                    this.selectedPresetId = 0;
                foreach (var p in this.presets)
                {
                    if (ImGui.Selectable($"{p.Name}##preset{p.Id}", p.Id == this.selectedPresetId))
                        this.selectedPresetId = p.Id;
                }
                ImGui.EndCombo();
            }
            ImGui.SameLine();
            if (Ui.Button("Apply Preset") && current != null)
                this.ApplyPreset(current);
        }

        Ui.Section(FontAwesomeIcon.ThLarge, "Win patterns");
        Ui.Help("Select one or more.");
        ImGui.SameLine();
        if (Ui.SmallButton(this.patternsCollapsed ? "Show all" : "Collapse all"))
        {
            this.patternsCollapsed = !this.patternsCollapsed;
            this.pendingPatternOpen = !this.patternsCollapsed;
        }
        this.DrawPatternPicker();

        Ui.Section(FontAwesomeIcon.AlignLeft, "Game details");
        Ui.Help("Markdown supported.");
        this.DrawDetailsEditor();

        Ui.Section(FontAwesomeIcon.Robot, "Auto-draw");
        var auto = this.newGameAuto;
        if (ImGui.Checkbox("Auto-draw numbers", ref auto))
            this.newGameAuto = auto;
        if (this.newGameAuto)
        {
            ImGui.SameLine();
            ImGui.SetNextItemWidth(140);
            if (ImGui.BeginCombo("Time Between Calls", AutoIntervalLabel(this.newGameAutoInterval)))
            {
                foreach (var s in AutoIntervalOptions)
                {
                    if (ImGui.Selectable(AutoIntervalLabel(s), s == this.newGameAutoInterval))
                        this.newGameAutoInterval = s;
                }
                ImGui.EndCombo();
            }
            Ui.Help("The server draws a number this often (plus the player delay). Auto turns off at half-time and when a winner is found.");
        }

        ImGui.Spacing();
        var canStart = this.selectedPatterns.Count > 0;
        if (!canStart)
            ImGui.BeginDisabled();
        if (Ui.PrimaryButton($"Start Game ({this.selectedPatterns.Count})"))
        {
            var start = new GameStartInput
            {
                PatternIds = this.selectedPatterns.ToList(),
                Auto = this.newGameAuto,
                AutoInterval = this.newGameAutoInterval,
            };
            this.Run(async () =>
            {
                var state = await this.api.StartGameAsync(start);
                await Apply(() =>
                {
                    this.TakeState(state);
                    this.selectedPatterns.Clear();
                });
            });
        }
        if (!canStart)
            ImGui.EndDisabled();
    }

    /// <summary>The details box, saved when it loses focus; every console hears the change.</summary>
    private void DrawDetailsEditor()
    {
        var details = this.gameDetails;
        if (ImGui.InputTextMultiline("##gamedetails", ref details, 4000, new Vector2(-1, 90)))
            this.gameDetails = details;
        this.detailsEditing = ImGui.IsItemActive();
        if (ImGui.IsItemDeactivatedAfterEdit())
            this.SaveDetails(this.gameDetails);
    }

    private void SaveDetails(string details) =>
        this.Run(() => this.api.UpdateGameAsync(new GameChanges { Details = details }));

    private void ApplyPreset(GamePreset preset)
    {
        var valid = this.patterns.Select(p => p.Id).ToHashSet();
        this.selectedPatterns.Clear();
        foreach (var id in preset.PatternIds.Where(valid.Contains))
            this.selectedPatterns.Add(id);
        this.gameDetails = preset.GameDetails;
        // Pre-fill the auto-draw controls from the preset (tweaking here never
        // writes back to the preset).
        this.newGameAuto = preset.Auto;
        this.newGameAutoInterval = preset.AutoInterval > 0 ? (int)preset.AutoInterval : DefaultAutoInterval;
        this.SaveDetails(this.gameDetails);
    }

    private void DrawPatternPicker()
    {
        foreach (var group in this.patterns.GroupBy(p => p.CategoryName))
        {
            var header = string.IsNullOrEmpty(group.Key) ? "Patterns" : group.Key;
            if (this.pendingPatternOpen.HasValue)
                ImGui.SetNextItemOpen(this.pendingPatternOpen.Value);
            if (!ImGui.CollapsingHeader($"{header}###cat{header}", ImGuiTreeNodeFlags.DefaultOpen))
                continue;

            if (ImGui.BeginTable($"pp{header}", 3))
            {
                foreach (var pattern in group)
                {
                    ImGui.TableNextColumn();
                    var id = pattern.Id;
                    DrawPatternMini(pattern.PatternData);
                    ImGui.SameLine();
                    var selected = this.selectedPatterns.Contains(id);
                    if (ImGui.Checkbox($"{pattern.Name}##pp{id}", ref selected))
                    {
                        if (selected)
                            this.selectedPatterns.Add(id);
                        else
                            this.selectedPatterns.Remove(id);
                    }
                }
                ImGui.EndTable();
            }
        }

        // The bulk open/close only applies for the frame the button was clicked; after
        // that, individual header toggles work normally again.
        this.pendingPatternOpen = null;
    }

    // -- Current game ---------------------------------------------------------

    private void DrawCurrentGame(Game state)
    {
        // Snapshot Busy: the click handler below calls Run(), which sets Busy true
        // synchronously, so re-reading the field at the End would pop a disabled
        // scope that was never pushed and corrupt ImGui's stack for the rest of the
        // frame. Same reason the canStart/canCreate/canAdd sites use a local.
        var drawDisabled = this.Busy;
        if (drawDisabled)
            ImGui.BeginDisabled();
        if (Ui.PrimaryButton("Draw Number"))
        {
            var delay = this.drawDelay;
            this.Run(async () =>
            {
                var result = await this.api.DrawNumberAsync(new DrawInput { Delay = delay });
                await Apply(() =>
                {
                    this.lastCalled = $"{result.Drawn.Letter}-{result.Drawn.Number}";
                    this.ApplyWinners(result.Winners);
                    this.game = result.Game;
                });
            });
        }
        if (drawDisabled)
            ImGui.EndDisabled();

        ImGui.SameLine();
        this.DrawDelayCombo();
        ImGui.SameLine();
        if (Ui.DangerButton("End Game"))
        {
            this.endGameId = state.Id;
            if (this.winners.Count > 0)
            {
                this.endGameSelected.Clear();
                foreach (var w in this.winners)
                    this.endGameSelected.Add(w);
                this.openEndGamePopup = true;
            }
            else
            {
                this.EndGame(state.Id, Array.Empty<string>());
            }
        }

        // A prompt closed without an answer still holds the automatic draws; it can be
        // brought back here.
        if (this.halftimeGameId == state.Id && !ImGui.IsPopupOpen("Half-Time###halftime"))
        {
            ImGui.SameLine();
            if (Ui.Button("Half-time..."))
                this.openHalftimePopup = true;
        }

        // "It's Yoever" live controls: switch the reaction on/off for all players
        // (server-side, per game) and watch the running trigger count.
        var yoeverEnabled = state.YoeverEnabled;
        if (ImGui.Checkbox("It's Yoever", ref yoeverEnabled))
        {
            var next = yoeverEnabled;
            state.YoeverEnabled = next; // optimistic; yoever_switched confirms
            this.Run(() => this.api.UpdateGameAsync(new GameChanges { YoeverEnabled = next }));
        }
        if (ImGui.IsItemHovered())
            ImGui.SetTooltip("Let players trigger the \"It's Yoever\" reaction. Switch off to curb spam.");
        ImGui.SameLine();
        ImGui.TextDisabled($"Yoevers: {state.YoeverCount}");

        // Auto-draw live controls: switch the loop on/off and adjust the interval
        // mid-game (never writes back to a preset).
        var autoEnabled = state.AutoEnabled;
        if (ImGui.Checkbox("Auto-Draw", ref autoEnabled))
        {
            var next = autoEnabled;
            state.AutoEnabled = next; // optimistic; auto_draw confirms
            this.Run(() => this.api.UpdateGameAsync(new GameChanges { AutoEnabled = next }));
        }
        if (ImGui.IsItemHovered())
            ImGui.SetTooltip("Draw numbers automatically on a timer. Turns off at half-time and when a winner is found.");
        if (state.AutoEnabled)
        {
            ImGui.SameLine();
            ImGui.SetNextItemWidth(140);
            if (ImGui.BeginCombo("Time Between Calls##live", AutoIntervalLabel(state.AutoInterval)))
            {
                foreach (var s in AutoIntervalOptions)
                {
                    if (ImGui.Selectable(AutoIntervalLabel(s), s == state.AutoInterval))
                    {
                        state.AutoInterval = s; // optimistic; auto_draw confirms
                        this.Run(() => this.api.UpdateGameAsync(new GameChanges { AutoInterval = s }));
                    }
                }
                ImGui.EndCombo();
            }
        }

        ImGui.Separator();

        // Dual column, mirroring the admin view: last-called + called grid on the
        // left; winners, frequent winners, game details, and active patterns on the
        // right.
        ImGui.BeginGroup();
        if (this.lastCalled != null)
        {
            ImGui.TextDisabled("Last Called:");
            ImGui.SameLine();
            ImGui.TextColored(new Vector4(0.4f, 0.8f, 1f, 1f), this.lastCalled);
        }
        DrawCalledNumbersGrid(state);
        ImGui.EndGroup();

        ImGui.SameLine();

        ImGui.BeginGroup();
        this.DrawWinners();
        this.DrawFrequentWinners();
        if (!string.IsNullOrWhiteSpace(this.gameDetails))
        {
            ImGui.TextDisabled("Game Details");
            ImGui.TextWrapped(this.gameDetails);
            ImGui.Spacing();
        }
        DrawActivePatterns(state);
        ImGui.EndGroup();
    }

    /// <summary>The draw delay, shared by every console: picking one saves it for all.</summary>
    private void DrawDelayCombo()
    {
        var d = this.drawDelay;
        ImGui.SetNextItemWidth(120);
        if (ImGui.BeginCombo("##delay", DelayLabel(d)))
        {
            foreach (var s in DrawDelayOptions)
            {
                if (ImGui.Selectable(DelayLabel(s), s == d))
                {
                    this.drawDelay = s;
                    this.Run(() => this.api.UpdateGameAsync(new GameChanges { Delay = s }));
                }
            }
            ImGui.EndCombo();
        }
        Ui.ItemTooltip("How long a drawn number waits before players get it. Shared with every console.");
    }

    private static string DelayLabel(long s) => s == 0 ? "Instant" : $"{s}s Delay";

    private static void DrawCalledNumbersGrid(Game state)
    {
        var called = new HashSet<long>(state.CalledNumbers);
        ImGui.TextUnformatted($"Called Numbers ({called.Count} / 75)");

        ImGui.PushStyleVar(ImGuiStyleVar.CellPadding, new Vector2(3, 2));
        // NoHostExtendX makes the table auto-fit its fixed columns instead of
        // stretching to the host width (which made the O column run to the edge and
        // left no room for the right-hand column).
        if (ImGui.BeginTable("called", 5, ImGuiTableFlags.Borders | ImGuiTableFlags.SizingFixedFit | ImGuiTableFlags.NoHostExtendX))
        {
            for (var c = 0; c < 5; c++)
                ImGui.TableSetupColumn($"##c{c}", ImGuiTableColumnFlags.WidthFixed, 30f);

            ImGui.TableNextRow();
            for (var c = 0; c < 5; c++)
            {
                ImGui.TableNextColumn();
                CenteredCell(BingoLetters[c], GridHeaderColor);
            }

            var highlight = ImGui.GetColorU32(new Vector4(0.20f, 0.55f, 0.30f, 0.85f));
            for (var row = 0; row < 15; row++)
            {
                ImGui.TableNextRow();
                for (var c = 0; c < 5; c++)
                {
                    ImGui.TableNextColumn();
                    var n = (c * 15) + row + 1;
                    var hit = called.Contains(n);
                    if (hit)
                        ImGui.TableSetBgColor(ImGuiTableBgTarget.CellBg, highlight);
                    CenteredCell(n.ToString(CultureInfo.InvariantCulture), hit ? GridHotColor : GridDimColor);
                }
            }

            ImGui.EndTable();
        }
        ImGui.PopStyleVar();
    }

    private void DrawWinners()
    {
        if (this.winners.Count == 0)
        {
            ImGui.TextDisabled("No winners yet.");
            return;
        }

        ImGui.TextColored(new Vector4(0.3f, 0.9f, 0.4f, 1f), $"Winning Cards ({this.winners.Count})");
        foreach (var id in this.winners)
        {
            var name = this.cardCache.NameFor(id);
            ImGui.BulletText(string.IsNullOrEmpty(name) ? id : $"{name}  ({id})");
            ImGui.SameLine();
            if (Ui.SmallButton($"View##win{id}"))
                this.OpenViewCard(id);
        }
        ImGui.Spacing();
    }

    private static void DrawActivePatterns(Game state)
    {
        if (state.Patterns.Count == 0)
            return;
        if (!ImGui.CollapsingHeader($"Active Win Patterns ({state.Patterns.Count})###activepatterns", ImGuiTreeNodeFlags.DefaultOpen))
            return;

        if (ImGui.BeginTable("activepat", 2))
        {
            foreach (var p in state.Patterns)
            {
                ImGui.TableNextColumn();
                DrawPatternMini(p.PatternData);
                ImGui.TextUnformatted(p.Name);
            }
            ImGui.EndTable();
        }
        ImGui.Spacing();
    }

    private void DrawFrequentWinners()
    {
        if (this.frequentWinners.Count == 0)
            return;
        ImGui.TextColored(new Vector4(0.9f, 0.7f, 0.3f, 1f), "Frequent Winners (3+ in 12h)");
        foreach (var fw in this.frequentWinners)
            ImGui.BulletText($"{fw.PlayerName} ({fw.WinCount})");
        ImGui.Spacing();
    }

    private void DrawHalftimePopup()
    {
        // Refuse a prompt that outlived the game it was raised for. A game boundary
        // clears it, but the operator may not have had this page open when that
        // arrived, so check again at the point of use.
        if (this.halftimeGameId != 0 && this.game != null && this.halftimeGameId != this.game.Id)
            this.ClearHalftimePrompt();

        if (this.openHalftimePopup)
        {
            ImGui.OpenPopup("Half-Time###halftime");
            this.openHalftimePopup = false;
            this.closeHalftimePopup = false;
        }

        ImGui.SetNextWindowSize(new Vector2(340, 0), ImGuiCond.Appearing);
        var open = true;
        if (!ImGui.BeginPopupModal("Half-Time###halftime", ref open))
        {
            this.closeHalftimePopup = false;
            return;
        }

        // Answered on another console: nothing is left to decide here.
        if (this.closeHalftimePopup || this.halftimeGameId == 0)
        {
            this.closeHalftimePopup = false;
            ImGui.CloseCurrentPopup();
            ImGui.EndPopup();
            return;
        }

        var threshold = this.game != null ? HalftimeThreshold(this.game.Patterns) : 0;
        ImGui.TextWrapped($"You've drawn {threshold} numbers! Alert players about a half-time mini-game?");
        if (this.halftimeAutoPaused)
        {
            ImGui.Spacing();
            ImGui.TextWrapped("Auto-draw has been paused. Choose No to resume it, or Yes to run a mini-game (auto stays off until you switch it back on).");
        }
        ImGui.Spacing();
        if (Ui.PrimaryButton("Yes"))
        {
            // Yes -> run a mini-game (alert players; auto stays paused).
            this.AnswerHalftime(true);
            ImGui.CloseCurrentPopup();
        }
        ImGui.SameLine();
        if (Ui.Button("No"))
        {
            // No -> decline the mini-game; the server resumes auto if it was paused.
            this.AnswerHalftime(false);
            ImGui.CloseCurrentPopup();
        }
        ImGui.EndPopup();
    }

    /// <summary>Answers the prompt for the game it was raised for. A second answer, from
    /// any console, is refused by the server and its words shown.</summary>
    private void AnswerHalftime(bool minigame)
    {
        var answer = new HalftimeAnswer { GameId = this.halftimeGameId, Minigame = minigame };
        this.ClearHalftimePrompt();
        this.Run(async () =>
        {
            var state = await this.api.AnswerHalftimeAsync(answer);
            await Apply(() => this.TakeState(state));
        });
    }

    private void DrawEndGamePopup()
    {
        if (this.openEndGamePopup)
        {
            ImGui.OpenPopup("End Game###endgame");
            this.openEndGamePopup = false;
            this.closeEndGamePopup = false;
        }

        ImGui.SetNextWindowSize(new Vector2(360, 0), ImGuiCond.Appearing);
        var open = true;
        if (!ImGui.BeginPopupModal("End Game###endgame", ref open))
        {
            this.closeEndGamePopup = false;
            return;
        }

        // The game ended (or another began) on another console: this confirm is moot.
        if (this.closeEndGamePopup)
        {
            this.closeEndGamePopup = false;
            ImGui.CloseCurrentPopup();
            ImGui.EndPopup();
            return;
        }

        ImGui.TextWrapped("Confirm the valid winners to record, then end the game:");
        ImGui.Spacing();
        foreach (var id in this.winners)
        {
            var name = this.cardCache.NameFor(id);
            var on = this.endGameSelected.Contains(id);
            if (ImGui.Checkbox($"{(string.IsNullOrEmpty(name) ? id : $"{name} ({id})")}##eg{id}", ref on))
            {
                if (on)
                    this.endGameSelected.Add(id);
                else
                    this.endGameSelected.Remove(id);
            }
        }

        ImGui.Spacing();
        if (Ui.DangerButton("End Game##confirm"))
        {
            this.EndGame(this.endGameId, this.endGameSelected.ToArray());
            ImGui.CloseCurrentPopup();
        }
        ImGui.SameLine();
        if (Ui.Button("Cancel##endgame"))
            ImGui.CloseCurrentPopup();
        ImGui.EndPopup();
    }

    private void EndGame(long gameId, string[] validWinnerIds)
    {
        var end = new GameEndInput { GameId = gameId, ValidWinnerIds = validWinnerIds };
        this.Run(async () =>
        {
            await this.api.EndGameAsync(end);
            await Apply(() =>
            {
                this.game = null;
                this.winners = new List<string>();
                this.lastCalled = null;
                this.ClearHalftimePrompt();
            });
        });
    }

    // -- View winning card -----------------------------------------------------

    private void OpenViewCard(string cardId)
    {
        this.viewCard = null;
        this.viewMatched = new HashSet<(int, int)>();
        this.openViewCard = true;
        // Dedicated fetch (not gated by the shared busy flag), so View always opens.
        _ = Task.Run(async () =>
        {
            try
            {
                var res = await this.api.GetBoardAsync(cardId, preview: true);
                await Apply(() =>
                {
                    this.viewCard = res.Card;
                    this.viewMatched = this.ComputeMatched(res.Card.BoardData);
                });
            }
            catch (Exception ex)
            {
                this.Status = Core.Problems.Say(ex);
            }
        });
    }

    /// <summary>
    /// Cells that complete a satisfied win pattern, mirroring the web's winner
    /// verification: a pattern is satisfied when every non-FREE required cell has
    /// been called; that pattern's required cells are then all "hits".
    /// </summary>
    private HashSet<(int, int)> ComputeMatched(IReadOnlyList<IReadOnlyList<int>> board)
    {
        var matched = new HashSet<(int, int)>();
        if (this.game == null)
            return matched;
        var called = new HashSet<long>(this.game.CalledNumbers);

        foreach (var pat in this.game.Patterns)
        {
            var pd = pat.PatternData;
            var satisfied = true;
            for (var r = 0; r < 5 && satisfied; r++)
            {
                for (var c = 0; c < 5; c++)
                {
                    if (r < pd.Count && c < pd[r].Count && pd[r][c])
                    {
                        var val = r < board.Count && c < board[r].Count ? board[r][c] : 0;
                        if (val != 0 && !called.Contains(val))
                        {
                            satisfied = false;
                            break;
                        }
                    }
                }
            }
            if (!satisfied)
                continue;
            for (var r = 0; r < 5; r++)
            {
                for (var c = 0; c < 5; c++)
                {
                    if (r < pd.Count && c < pd[r].Count && pd[r][c])
                        matched.Add((r, c));
                }
            }
        }
        return matched;
    }

    private void DrawViewCardPopup()
    {
        if (this.openViewCard)
        {
            ImGui.OpenPopup("View Card###viewcard");
            this.openViewCard = false;
        }

        // Title carries the player's name; the stable ### id keeps OpenPopup and
        // BeginPopupModal matched even as the visible title changes once it loads.
        var who = this.viewCard?.PlayerName;
        var title = string.IsNullOrEmpty(who)
            ? "Viewing Card###viewcard"
            : $"Viewing {who}'s Card###viewcard";

        // AlwaysAutoResize so the window fits the board once it loads (a fixed
        // appearing-size locked it to the tiny "Loading..." content and stayed small).
        var open = true;
        if (!ImGui.BeginPopupModal(title, ref open, ImGuiWindowFlags.AlwaysAutoResize))
            return;

        if (this.viewCard == null)
        {
            ImGui.TextDisabled("Loading...");
            ImGui.EndPopup();
            return;
        }

        ImGui.TextDisabled($"Card {this.viewCard.Id} - gold = winning pattern");
        ImGui.Spacing();

        DrawCardBoard(this.viewCard.BoardData, this.viewMatched);

        ImGui.Spacing();
        if (Ui.Button("Close##viewcard"))
            ImGui.CloseCurrentPopup();
        ImGui.EndPopup();
    }

    private static readonly Vector4 BoardMatchBg = new(0.85f, 0.66f, 0.18f, 0.9f);
    private static readonly Vector4 BoardMatchText = new(0.10f, 0.08f, 0.02f, 1f);
    private static readonly Vector4 BoardFreeText = new(0.55f, 0.55f, 0.60f, 1f);
    private static readonly Vector4 BoardText = new(0.90f, 0.90f, 0.92f, 1f);

    private static void DrawCardBoard(IReadOnlyList<IReadOnlyList<int>> board, HashSet<(int, int)> matched)
    {
        ImGui.PushStyleVar(ImGuiStyleVar.CellPadding, new Vector2(4, 5));
        if (ImGui.BeginTable("cardboard", 5, ImGuiTableFlags.Borders | ImGuiTableFlags.SizingFixedFit | ImGuiTableFlags.NoHostExtendX))
        {
            for (var c = 0; c < 5; c++)
                ImGui.TableSetupColumn($"##cb{c}", ImGuiTableColumnFlags.WidthFixed, 36f);

            ImGui.TableNextRow();
            for (var c = 0; c < 5; c++)
            {
                ImGui.TableNextColumn();
                CenteredCell(BingoLetters[c], GridHeaderColor);
            }

            var matchBg = ImGui.GetColorU32(BoardMatchBg);
            for (var r = 0; r < 5; r++)
            {
                ImGui.TableNextRow();
                for (var c = 0; c < 5; c++)
                {
                    ImGui.TableNextColumn();
                    var val = r < board.Count && c < board[r].Count ? board[r][c] : 0;
                    var hit = matched.Contains((r, c));
                    if (hit)
                        ImGui.TableSetBgColor(ImGuiTableBgTarget.CellBg, matchBg);
                    var text = val == 0 ? "FREE" : val.ToString(CultureInfo.InvariantCulture);
                    var color = hit ? BoardMatchText : val == 0 ? BoardFreeText : BoardText;
                    CenteredCell(text, color);
                }
            }

            ImGui.EndTable();
        }
        ImGui.PopStyleVar();
    }

    // -- Live event handlers (already on the framework thread) -----------------

    private void OnDraw(long? gameId, DrawnNumber drawn, IReadOnlyList<string> winnerIds)
    {
        if (this.game == null || this.game.Id != gameId)
        {
            // A draw for a game this page doesn't hold: it is out of step, so read it again.
            this.MarkStale();
            return;
        }
        this.lastCalled = $"{drawn.Letter}-{drawn.Number}";
        this.ApplyWinners(winnerIds);
        if (!this.game.CalledNumbers.Contains(drawn.Number))
            this.game.CalledNumbers = [.. this.game.CalledNumbers, drawn.Number];
    }

    private void OnGameStarted(long? gameId)
    {
        // A different game makes any pending prompt or End Game confirm meaningless.
        this.ClearHalftimePrompt();
        this.closeEndGamePopup = true;
        this.MarkStale();
    }

    private void OnGameEnded(long? gameId)
    {
        if (this.game == null || gameId == null || this.game.Id == gameId)
        {
            this.game = null;
            this.winners = new List<string>();
            this.lastCalled = null;
            this.ClearHalftimePrompt();
            this.closeEndGamePopup = true;
        }
        // The frequent winners follow the log the end just wrote.
        this.MarkStale();
    }

    private void ClearHalftimePrompt()
    {
        this.openHalftimePopup = false;
        this.halftimeAutoPaused = false;
        this.halftimeGameId = 0;
    }

    // Auto-draw state changed on the server (started, toggled, interval adjusted, or
    // switched off by a winner/half-time): mirror it onto the current game.
    private void OnAutoDraw(bool enabled, long interval)
    {
        if (this.game == null)
            return;
        this.game.AutoEnabled = enabled;
        this.game.AutoInterval = interval;
    }

    /// <summary>
    /// Tells the operator half time was reached while the Companion window was
    /// closed or on another page.
    /// </summary>
    /// <remarks>
    /// Both channels are purely LOCAL. IChatGui.Print writes into this client's own
    /// chat log and sends nothing to the server - it is not ChatSender, which
    /// actually transmits and carries the account risk that comes with automated
    /// chat. The chime is the same one a new winner already plays, and is
    /// documented there as using no game interop at all. So this adds a message
    /// only the operator can see, on a prompt that has genuinely paused their game.
    /// </remarks>
    private static void AlertHalftimeOffPage(bool autoPaused)
    {
        Chime.Winner();
        var paused = autoPaused ? " Auto-draw is paused until you answer." : string.Empty;
        Plugin.ChatGui.Print(
            $"[Senpan] Half time reached - open the Companion window to answer the mini-game prompt.{paused}");
    }

    // The server reached the half-time mark (on any draw, manual or automatic):
    // open the mini-game prompt. autoPaused tells the modal whether declining will
    // resume the auto draws.
    private void OnHalftimePrompt(long? gameId, bool autoPaused)
    {
        if (gameId is { } id)
            this.ShowHalftime(id, autoPaused, alert: true);
    }

    private void ShowHalftime(long gameId, bool autoPaused, bool alert)
    {
        this.halftimeAutoPaused = autoPaused;
        this.halftimeGameId = gameId;
        this.openHalftimePopup = true;

        // If this page was not on screen a moment ago, nobody is going to see the
        // modal. Say so where the operator IS looking - the game itself.
        if (alert && DateTime.UtcNow - this.lastDrawnAt > TimeSpan.FromSeconds(1))
            AlertHalftimeOffPage(autoPaused);
    }

    /// <summary>
    /// Someone answered the prompt. One this console was still showing closes, with a
    /// line in this client's own chat log saying who answered and how (local only, as
    /// <see cref="AlertHalftimeOffPage"/>). This console's own answer cleared its prompt
    /// already, so it says nothing.
    /// </summary>
    private void OnHalftimeResolved(long? gameId, bool minigame, string by)
    {
        if (this.game?.Id == gameId && this.game != null)
            this.game.Halftime = null;
        if (this.halftimeGameId == 0 || this.halftimeGameId != gameId)
            return;
        this.ClearHalftimePrompt();
        this.closeHalftimePopup = true;
        var answer = minigame ? "a mini-game runs" : "no mini-game";
        var who = string.IsNullOrWhiteSpace(by) ? "another console" : by;
        Plugin.ChatGui.Print($"[Senpan] Half time was answered by {who}: {answer}.");
    }

    private void OnDrawDelay(long delay) => this.drawDelay = delay;

    private void OnGameDetails(string details)
    {
        if (!this.detailsEditing)
            this.gameDetails = details;
    }

    // A player fired the reaction: keep the running count in step.
    private void OnYoever(long? gameId, string playerName, long count)
    {
        if (this.game != null && this.game.Id == gameId)
            this.game.YoeverCount = count;
    }

    // An admin (here or on the web) switched the reaction on/off.
    private void OnYoeverSwitched(bool enabled)
    {
        if (this.game != null)
            this.game.YoeverEnabled = enabled;
    }

    /// <summary>
    /// Replaces the winners list, chiming once when it grows (a genuinely new
    /// winner). The plugin's own draw arrives twice (its answer and the live event);
    /// the count guard de-dupes it.
    /// </summary>
    private void ApplyWinners(IReadOnlyList<string> next)
    {
        if (next.Count > this.winners.Count)
            Chime.Winner();
        this.winners = next.ToList();
    }

    // -- Helpers --------------------------------------------------------------

    private static string AutoIntervalLabel(long seconds)
    {
        if (seconds < 60)
            return $"{seconds}s";
        var m = seconds / 60;
        var s = seconds % 60;
        return s == 0 ? $"{m}m" : $"{m}m {s}s";
    }

    /// <summary>
    /// The half-way call count, mirroring the web (lib/halftime.ts): the classic
    /// 35-of-75 ratio scaled to this game's callable pool (active columns x 15).
    /// </summary>
    private static int HalftimeThreshold(IReadOnlyList<GamePattern> patterns)
    {
        var cols = new bool[5];
        foreach (var p in patterns)
        {
            var grid = p.PatternData;
            for (var r = 0; r < grid.Count && r < 5; r++)
            {
                for (var c = 0; c < grid[r].Count && c < 5; c++)
                {
                    if (grid[r][c] && !(r == 2 && c == 2))
                        cols[c] = true;
                }
            }
        }

        var active = cols.Count(x => x);
        if (active == 0)
            active = 5;
        var maxCallable = active * 15;
        return Math.Max(1, (int)Math.Round(35.0 / 75.0 * maxCallable, MidpointRounding.AwayFromZero));
    }

    private static void DrawPatternMini(IReadOnlyList<IReadOnlyList<bool>> data, float cell = 8f)
    {
        var dl = ImGui.GetWindowDrawList();
        var origin = ImGui.GetCursorScreenPos();
        var on = ImGui.GetColorU32(new Vector4(0.40f, 0.70f, 1f, 1f));
        var off = ImGui.GetColorU32(new Vector4(0.24f, 0.24f, 0.28f, 1f));
        for (var r = 0; r < 5; r++)
        {
            for (var c = 0; c < 5; c++)
            {
                var a = new Vector2(origin.X + (c * cell), origin.Y + (r * cell));
                var b = new Vector2(a.X + cell - 1.5f, a.Y + cell - 1.5f);
                var set = r < data.Count && c < data[r].Count && data[r][c];
                dl.AddRectFilled(a, b, set ? on : off);
            }
        }
        ImGui.Dummy(new Vector2(cell * 5, cell * 5));
    }

    private static void CenteredCell(string text, Vector4 color)
    {
        var avail = ImGui.GetContentRegionAvail().X;
        var textWidth = ImGui.CalcTextSize(text).X;
        var off = (avail - textWidth) * 0.5f;
        if (off > 0)
            ImGui.SetCursorPosX(ImGui.GetCursorPosX() + off);
        ImGui.TextColored(color, text);
    }
}
