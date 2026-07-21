using System;
using System.Collections.Generic;
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
/// winners, and the halftime prompt arrive live over the WebSocket; a chime plays
/// when a new winner appears. Card → player-name lookups come from the shared cache.
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

    private readonly ApiClient api;
    private readonly Configuration config;
    private readonly LiveConnection live;
    private readonly CardCache cardCache;

    private List<Pattern> patterns = new();
    private List<GamePreset> presets = new();
    private List<FrequentWinner> frequentWinners = new();
    private readonly HashSet<int> selectedPatterns = new();
    private long selectedPresetId;

    // Bulk expand/collapse of the win-pattern category headers: pendingPatternOpen forces
    // every header open/closed for one frame; patternsCollapsed drives the toggle label.
    private bool? pendingPatternOpen;
    private bool patternsCollapsed;

    private GameState? game;
    private List<string> winners = new();
    private DrawnNumber? lastDrawn;
    private string gameDetails = string.Empty;

    // New Game auto-draw controls (session-local, like the web's new-game form).
    private bool newGameAuto;
    private int newGameAutoInterval = DefaultAutoInterval;

    private bool openHalftimePopup;
    private bool halftimeAutoPaused;

    private bool openEndGamePopup;
    private readonly HashSet<string> endGameSelected = new();

    private bool openViewCard;
    private Card? viewCard;
    private HashSet<(int, int)> viewMatched = new();

    public BingoGameTab(ApiClient api, Configuration config, LiveConnection live, CardCache cardCache)
    {
        this.api = api;
        this.config = config;
        this.live = live;
        this.cardCache = cardCache;

        this.live.GameDraw += OnGameDraw;
        this.live.GameUpdate += OnGameUpdate;
        this.live.Yoever += OnYoever;
        this.live.YoeverConfig += OnYoeverConfig;
        this.live.AutoConfig += OnAutoConfig;
        this.live.HalftimePrompt += OnHalftimePrompt;
    }

    public void Dispose()
    {
        this.live.GameDraw -= OnGameDraw;
        this.live.GameUpdate -= OnGameUpdate;
        this.live.Yoever -= OnYoever;
        this.live.YoeverConfig -= OnYoeverConfig;
        this.live.AutoConfig -= OnAutoConfig;
        this.live.HalftimePrompt -= OnHalftimePrompt;
    }

    protected override async Task LoadAsync()
    {
        // Patterns, presets and frequent-winners are gated by their own permissions
        // (bingo-patterns / bingo-presets / bingo-winners-log). Fetch them optionally,
        // so an operator with only bingo-game still gets the (public) game state and
        // can draw — a missing sub-permission just leaves that section empty.
        var patternsRes = await OptionalAsync(() => this.api.ListPatternsAsync());
        var presetsRes = await OptionalAsync(() => this.api.ListPresetsAsync());
        var gameRes = await this.api.GetGameAsync();
        var freqRes = await OptionalAsync(() => this.api.FrequentWinnersAsync());
        await Apply(() =>
        {
            this.patterns = patternsRes?.Patterns ?? new List<Pattern>();
            this.presets = presetsRes?.Presets ?? new List<GamePreset>();
            this.game = gameRes.Game;
            this.winners = gameRes.Winners;
            this.gameDetails = gameRes.GameDetails;
            this.frequentWinners = freqRes?.Winners ?? new List<FrequentWinner>();
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

    public void Draw()
    {
        DrawStatusLine();

        if (this.game == null)
            DrawNewGame();
        else
            DrawCurrentGame(this.game);

        DrawHalftimePopup();
        DrawEndGamePopup();
        DrawViewCardPopup();
    }

    // ── New game ─────────────────────────────────────────────────────────────

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
            if (ImGui.BeginCombo("Preset", current?.Name ?? "— None —"))
            {
                if (ImGui.Selectable("— None —", this.selectedPresetId == 0))
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
                ApplyPreset(current);
        }

        Ui.Section(FontAwesomeIcon.ThLarge, "Win patterns");
        Ui.Help("Select one or more.");
        ImGui.SameLine();
        if (Ui.SmallButton(this.patternsCollapsed ? "Show all" : "Collapse all"))
        {
            this.patternsCollapsed = !this.patternsCollapsed;
            this.pendingPatternOpen = !this.patternsCollapsed;
        }
        DrawPatternPicker();

        Ui.Section(FontAwesomeIcon.AlignLeft, "Game details");
        Ui.Help("Markdown supported.");
        var details = this.gameDetails;
        if (ImGui.InputTextMultiline("##gamedetails", ref details, 4000, new Vector2(-1, 90)))
            this.gameDetails = details;
        if (ImGui.IsItemDeactivatedAfterEdit())
        {
            var toSave = this.gameDetails;
            Run(() => this.api.UpdateGameDetailsAsync(toSave));
        }

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
            var ids = this.selectedPatterns.ToArray();
            var startAuto = this.newGameAuto;
            var startInterval = this.newGameAutoInterval;
            Run(async () =>
            {
                var g = await this.api.StartGameAsync(ids, startAuto, startInterval);
                await Apply(() =>
                {
                    this.game = g.Game;
                    this.winners = g.Winners;
                    this.gameDetails = g.GameDetails;
                    this.lastDrawn = null;
                    this.selectedPatterns.Clear();
                });
            });
        }
        if (!canStart)
            ImGui.EndDisabled();
    }

    private void ApplyPreset(GamePreset preset)
    {
        var valid = this.patterns.Select(p => (int)p.Id).ToHashSet();
        this.selectedPatterns.Clear();
        foreach (var id in preset.PatternIds.Where(valid.Contains))
            this.selectedPatterns.Add(id);
        this.gameDetails = preset.GameDetails;
        // Pre-fill the auto-draw controls from the preset (tweaking here never
        // writes back to the preset).
        this.newGameAuto = preset.Auto;
        this.newGameAutoInterval = preset.AutoInterval > 0 ? preset.AutoInterval : DefaultAutoInterval;
        var toSave = this.gameDetails;
        Run(() => this.api.UpdateGameDetailsAsync(toSave));
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
                    var id = (int)pattern.Id;
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

    // ── Current game ─────────────────────────────────────────────────────────

    private void DrawCurrentGame(GameState state)
    {
        if (this.Busy)
            ImGui.BeginDisabled();
        if (Ui.PrimaryButton("Draw Number"))
        {
            var delay = this.config.DrawDelaySeconds;
            Run(async () =>
            {
                var result = await this.api.DrawAsync(delay);
                await Apply(() =>
                {
                    this.lastDrawn = result.Drawn;
                    ApplyWinners(result.Winners);
                    if (this.game != null && !this.game.CalledNumbers.Contains(result.Drawn.Number))
                        this.game.CalledNumbers.Add(result.Drawn.Number);
                    // The half-time prompt is server-driven now (halftime_prompt),
                    // so it fires for manual and automatic draws alike.
                });
            });
        }
        if (this.Busy)
            ImGui.EndDisabled();

        ImGui.SameLine();
        DrawDelayCombo();
        ImGui.SameLine();
        if (Ui.DangerButton("End Game"))
        {
            if (this.winners.Count > 0)
            {
                this.endGameSelected.Clear();
                foreach (var w in this.winners)
                    this.endGameSelected.Add(w);
                this.openEndGamePopup = true;
            }
            else
            {
                EndGame(Array.Empty<string>());
            }
        }

        // "It's Yoever" live controls: switch the reaction on/off for all players
        // (server-side, per game) and watch the running trigger count.
        var yoeverEnabled = state.YoeverEnabled;
        if (ImGui.Checkbox("It's Yoever", ref yoeverEnabled))
        {
            var next = yoeverEnabled;
            state.YoeverEnabled = next; // optimistic; the yoever_config broadcast confirms
            Run(() => this.api.SetYoeverEnabledAsync(next));
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
            state.AutoEnabled = next; // optimistic; the auto_config broadcast confirms
            Run(() => this.api.SetAutoEnabledAsync(next));
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
                        state.AutoInterval = s; // optimistic; the auto_config broadcast confirms
                        Run(() => this.api.SetAutoIntervalAsync(s));
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
        if (this.lastDrawn != null)
        {
            ImGui.TextDisabled("Last Called:");
            ImGui.SameLine();
            ImGui.TextColored(new Vector4(0.4f, 0.8f, 1f, 1f), $"{this.lastDrawn.Letter}-{this.lastDrawn.Number}");
        }
        DrawCalledNumbersGrid(state);
        ImGui.EndGroup();

        ImGui.SameLine();

        ImGui.BeginGroup();
        DrawWinners();
        DrawFrequentWinners();
        if (!string.IsNullOrWhiteSpace(this.gameDetails))
        {
            ImGui.TextDisabled("Game Details");
            ImGui.TextWrapped(this.gameDetails);
            ImGui.Spacing();
        }
        DrawActivePatterns(state);
        ImGui.EndGroup();
    }

    private void DrawDelayCombo()
    {
        var d = this.config.DrawDelaySeconds;
        ImGui.SetNextItemWidth(120);
        if (ImGui.BeginCombo("##delay", DelayLabel(d)))
        {
            foreach (var s in DrawDelayOptions)
            {
                if (ImGui.Selectable(DelayLabel(s), s == d))
                {
                    this.config.DrawDelaySeconds = s;
                    this.config.Save();
                }
            }
            ImGui.EndCombo();
        }
    }

    private static string DelayLabel(int s) => s == 0 ? "Instant" : $"{s}s Delay";

    private static void DrawCalledNumbersGrid(GameState state)
    {
        var called = new HashSet<int>(state.CalledNumbers);
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
                    var n = c * 15 + row + 1;
                    var hit = called.Contains(n);
                    if (hit)
                        ImGui.TableSetBgColor(ImGuiTableBgTarget.CellBg, highlight);
                    CenteredCell(n.ToString(), hit ? GridHotColor : GridDimColor);
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
                OpenViewCard(id);
        }
        ImGui.Spacing();
    }

    private static void DrawActivePatterns(GameState state)
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
        if (this.openHalftimePopup)
        {
            ImGui.OpenPopup("Half-Time###halftime");
            this.openHalftimePopup = false;
        }

        ImGui.SetNextWindowSize(new Vector2(340, 0), ImGuiCond.Appearing);
        var open = true;
        if (!ImGui.BeginPopupModal("Half-Time###halftime", ref open))
            return;

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
            // Yes → run a mini-game (alert players; auto stays paused).
            Run(() => this.api.TriggerHalftimeAsync(true));
            this.halftimeAutoPaused = false;
            ImGui.CloseCurrentPopup();
        }
        ImGui.SameLine();
        if (Ui.Button("No"))
        {
            // No → decline the mini-game; the server resumes auto if it was paused.
            Run(() => this.api.TriggerHalftimeAsync(false));
            this.halftimeAutoPaused = false;
            ImGui.CloseCurrentPopup();
        }
        ImGui.EndPopup();
    }

    private void DrawEndGamePopup()
    {
        if (this.openEndGamePopup)
        {
            ImGui.OpenPopup("End Game###endgame");
            this.openEndGamePopup = false;
        }

        ImGui.SetNextWindowSize(new Vector2(360, 0), ImGuiCond.Appearing);
        var open = true;
        if (!ImGui.BeginPopupModal("End Game###endgame", ref open))
            return;

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
            EndGame(this.endGameSelected.ToArray());
            ImGui.CloseCurrentPopup();
        }
        ImGui.SameLine();
        if (Ui.Button("Cancel##endgame"))
            ImGui.CloseCurrentPopup();
        ImGui.EndPopup();
    }

    private void EndGame(string[] validWinnerIds)
    {
        Run(async () =>
        {
            await this.api.EndGameAsync(validWinnerIds);
            await Apply(() =>
            {
                this.game = null;
                this.winners = new List<string>();
                this.lastDrawn = null;
            });
        });
    }

    // ── View winning card ─────────────────────────────────────────────────────

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
                var res = await this.api.GetCardBoardAsync(cardId);
                await Apply(() =>
                {
                    this.viewCard = res.Card;
                    this.viewMatched = ComputeMatched(res.Card.BoardData);
                });
            }
            catch (Exception ex)
            {
                this.Status = ex.Message;
            }
        });
    }

    /// <summary>
    /// Cells that complete a satisfied win pattern, mirroring the web's winner
    /// verification: a pattern is satisfied when every non-FREE required cell has
    /// been called; that pattern's required cells are then all "hits".
    /// </summary>
    private HashSet<(int, int)> ComputeMatched(int[][] board)
    {
        var matched = new HashSet<(int, int)>();
        if (this.game == null)
            return matched;
        var called = new HashSet<int>(this.game.CalledNumbers);

        foreach (var pat in this.game.Patterns)
        {
            var pd = pat.PatternData;
            var satisfied = true;
            for (var r = 0; r < 5 && satisfied; r++)
            {
                for (var c = 0; c < 5; c++)
                {
                    if (r < pd.Length && c < pd[r].Length && pd[r][c])
                    {
                        var val = r < board.Length && c < board[r].Length ? board[r][c] : 0;
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
                for (var c = 0; c < 5; c++)
                    if (r < pd.Length && c < pd[r].Length && pd[r][c])
                        matched.Add((r, c));
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
        // appearing-size locked it to the tiny "Loading…" content and stayed small).
        var open = true;
        if (!ImGui.BeginPopupModal(title, ref open, ImGuiWindowFlags.AlwaysAutoResize))
            return;

        if (this.viewCard == null)
        {
            ImGui.TextDisabled("Loading…");
            ImGui.EndPopup();
            return;
        }

        ImGui.TextDisabled($"Card {this.viewCard.Id} · gold = winning pattern");
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

    private static void DrawCardBoard(int[][] board, HashSet<(int, int)> matched)
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
                    var val = r < board.Length && c < board[r].Length ? board[r][c] : 0;
                    var hit = matched.Contains((r, c));
                    if (hit)
                        ImGui.TableSetBgColor(ImGuiTableBgTarget.CellBg, matchBg);
                    var text = val == 0 ? "FREE" : val.ToString();
                    var color = hit ? BoardMatchText : val == 0 ? BoardFreeText : BoardText;
                    CenteredCell(text, color);
                }
            }

            ImGui.EndTable();
        }
        ImGui.PopStyleVar();
    }

    // ── Live event handlers (already on the framework thread) ─────────────────

    private void OnGameDraw(DrawnNumber drawn, string[] winnerIds)
    {
        this.lastDrawn = drawn;
        ApplyWinners(winnerIds.ToList());
        if (this.game != null && !this.game.CalledNumbers.Contains(drawn.Number))
            this.game.CalledNumbers.Add(drawn.Number);
    }

    private void OnGameUpdate(GameState? state)
    {
        this.game = state;
        if (state == null)
        {
            this.winners = new List<string>();
            this.lastDrawn = null;
        }
    }

    // Auto-draw state changed on the server (started, toggled, interval adjusted, or
    // switched off by a winner/half-time): mirror it onto the current game.
    private void OnAutoConfig(bool enabled, int interval)
    {
        if (this.game == null)
            return;
        this.game.AutoEnabled = enabled;
        this.game.AutoInterval = interval;
    }

    // The server reached the half-time mark (on any draw, manual or automatic):
    // open the mini-game prompt. autoPaused tells the modal whether declining will
    // resume the auto draws.
    private void OnHalftimePrompt(bool autoPaused)
    {
        this.halftimeAutoPaused = autoPaused;
        this.openHalftimePopup = true;
    }

    // A player fired the reaction: keep the running count in step. The dedicated
    // "yoever" broadcast is the only live count update between draws (game_update
    // only arrives on start/draw/end), so the tracker relies on it.
    private void OnYoever(string playerName, int count)
    {
        if (this.game != null)
            this.game.YoeverCount = count;
    }

    // An admin (here or on the web) switched the reaction on/off.
    private void OnYoeverConfig(bool enabled)
    {
        if (this.game != null)
            this.game.YoeverEnabled = enabled;
    }

    /// <summary>
    /// Replaces the winners list, chiming once when it grows (a genuinely new
    /// winner). The plugin's own draw arrives twice (REST result + WS echo); the
    /// count guard de-dupes it.
    /// </summary>
    private void ApplyWinners(List<string> next)
    {
        if (next.Count > this.winners.Count)
            WinnerChime.Play();
        this.winners = next;
    }

    // ── Helpers ──────────────────────────────────────────────────────────────

    private static string AutoIntervalLabel(int seconds)
    {
        if (seconds < 60)
            return $"{seconds}s";
        var m = seconds / 60;
        var s = seconds % 60;
        return s == 0 ? $"{m}m" : $"{m}m {s}s";
    }

    /// <summary>
    /// The half-way call count, mirroring the web (lib/halftime.ts): the classic
    /// 35-of-75 ratio scaled to this game's callable pool (active columns × 15).
    /// </summary>
    private static int HalftimeThreshold(List<GamePattern> patterns)
    {
        var cols = new bool[5];
        foreach (var p in patterns)
        {
            var grid = p.PatternData;
            for (var r = 0; r < grid.Length && r < 5; r++)
            {
                for (var c = 0; c < grid[r].Length && c < 5; c++)
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

    private static void DrawPatternMini(bool[][] data, float cell = 8f)
    {
        var dl = ImGui.GetWindowDrawList();
        var origin = ImGui.GetCursorScreenPos();
        var on = ImGui.GetColorU32(new Vector4(0.40f, 0.70f, 1f, 1f));
        var off = ImGui.GetColorU32(new Vector4(0.24f, 0.24f, 0.28f, 1f));
        for (var r = 0; r < 5; r++)
        {
            for (var c = 0; c < 5; c++)
            {
                var a = new Vector2(origin.X + c * cell, origin.Y + r * cell);
                var b = new Vector2(a.X + cell - 1.5f, a.Y + cell - 1.5f);
                var set = r < data.Length && c < data[r].Length && data[r][c];
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
