package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"app-suite/internal/bingo"
	"app-suite/internal/db"
	"app-suite/internal/live"
	"app-suite/internal/permissions"
	"app-suite/internal/store"
	"app-suite/internal/store/storetest"
)

// recorder is a Hub that keeps every event, with who it went to.
type recorder struct {
	mu   sync.Mutex
	sent []sent
}

type sent struct {
	name    string
	to      live.Audience
	gameID  *int64
	payload map[string]any
	msg     live.Message
}

func (r *recorder) Publish(msg live.Message, to live.Audience) {
	b, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	var m struct {
		GameID  *int64         `json:"gameId"`
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		panic(err)
	}
	if m.Payload == nil {
		panic(msg.Name() + " has a null payload")
	}
	r.mu.Lock()
	r.sent = append(r.sent, sent{msg.Name(), to, m.GameID, m.Payload, msg})
	r.mu.Unlock()
}

// of is every event of type name, each with who it went to.
func (r *recorder) of(name string) []sent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []sent
	for _, s := range r.sent {
		if s.name == name {
			out = append(out, s)
		}
	}
	return out
}

// game is the game an event names, or 0 for none.
func (s sent) game() int64 {
	if s.gameID == nil {
		return 0
	}
	return *s.gameID
}

// toGameStaff is whether the event goes to staff with the Game page alone.
func (s sent) toGameStaff() bool {
	return s.to.Staff && !s.to.Players && slices.Equal(s.to.Perms, []string{permissions.BingoGame})
}

// toAll is whether the event goes to the boards and to staff with the Game page.
func (s sent) toAll() bool {
	return s.to.Staff && s.to.Players && slices.Equal(s.to.Perms, []string{permissions.BingoGame})
}

// toBoards is whether the event goes to the boards alone.
func (s sent) toBoards() bool {
	return s.to.Players && !s.to.Staff
}

// testGame is a game on a fresh database, with what it sends recorded.
func testGame(t *testing.T) (*Service, *store.Store, *recorder) {
	t.Helper()
	st, err := store.New(t.Context(), storetest.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	rec := &recorder{}
	return New(st.ReadDB(), st.DB(), rec), st, rec
}

func grid(cells ...[2]int) [][]bool {
	g := make([][]bool, 5)
	for i := range g {
		g[i] = make([]bool, 5)
	}
	for _, rc := range cells {
		g[rc[0]][rc[1]] = true
	}
	return g
}

func topRow() [][]bool {
	return grid([2]int{0, 0}, [2]int{0, 1}, [2]int{0, 2}, [2]int{0, 3}, [2]int{0, 4})
}

// bColumn uses the B column alone: 15 numbers, half-time at 7.
func bColumn() [][]bool {
	return grid([2]int{0, 0}, [2]int{1, 0}, [2]int{2, 0}, [2]int{3, 0}, [2]int{4, 0})
}

func postageStamps() [][]bool {
	return grid(
		[2]int{0, 0}, [2]int{0, 1}, [2]int{1, 0}, [2]int{1, 1},
		[2]int{0, 3}, [2]int{0, 4}, [2]int{1, 3}, [2]int{1, 4},
		[2]int{3, 0}, [2]int{3, 1}, [2]int{4, 0}, [2]int{4, 1},
		[2]int{3, 3}, [2]int{3, 4}, [2]int{4, 3}, [2]int{4, 4},
	)
}

// winningBoard's top row is B1, I16, N31, G46, O61.
func winningBoard() [][]int {
	return [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
}

func pattern(t *testing.T, st *store.Store, name string, g [][]bool) int64 {
	t.Helper()
	id, err := st.SavePattern(t.Context(), name, g, 1)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// start starts a game for one pattern and answers it.
func start(t *testing.T, g *Service, st *store.Store, grid [][]bool) *Game {
	t.Helper()
	state, err := g.Start(t.Context(), Start{PatternIDs: []int64{pattern(t, st, "P", grid)}, Replace: true})
	if err != nil {
		t.Fatal(err)
	}
	return state.Game
}

func draw(t *testing.T, g *Service, delay int) *DrawResult {
	t.Helper()
	res, err := g.Draw(t.Context(), delay)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestStartAndState(t *testing.T) {
	g, st, rec := testGame(t)

	state, err := g.State(t.Context(), true)
	if err != nil || state.Game != nil || len(state.Winners) != 0 {
		t.Fatalf("no game: %+v, %v", state, err)
	}

	started := start(t, g, st, topRow())
	if started.TotalCalled != 0 || len(started.Patterns) != 1 || started.Patterns[0].Name != "P" ||
		started.CreatedAt == nil || !started.YoeverEnabled || started.AutoEnabled || started.LastDraw != nil {
		t.Errorf("started: %+v", started)
	}
	state, err = g.State(t.Context(), false)
	if err != nil || state.Game == nil || state.Game.ID != started.ID {
		t.Fatalf("the state: %+v, %v", state, err)
	}
	if ups := rec.of("game_started"); len(ups) != 1 || !ups[0].toAll() || ups[0].game() != started.ID || len(ups[0].payload) != 0 {
		t.Errorf("game_started: %+v; want one, naming the game, to the boards and the staff", ups)
	}
}

// A start while a game runs is refused unless it says to replace it; replacing ends
// the old game (one is active) without logging its winners.
func TestStartWhileLive(t *testing.T) {
	g, st, _ := testGame(t)
	if err := st.SaveCard(t.Context(), "WIN001", winningBoard()); err != nil {
		t.Fatal(err)
	}
	id := pattern(t, st, "Top Row", topRow())
	first, err := g.Start(t.Context(), Start{PatternIDs: []int64{id}})
	if err != nil {
		t.Fatal(err)
	}
	for range 75 {
		if _, err := g.Draw(t.Context(), 0); err != nil {
			break
		}
	}
	if _, err := g.Start(t.Context(), Start{PatternIDs: []int64{id}}); !errors.Is(err, ErrGameLive) {
		t.Fatalf("a second start: %v, want ErrGameLive", err)
	}
	if running, _ := g.Running(t.Context()); running != first.Game.ID {
		t.Fatalf("the refused start changed the game: %d", running)
	}
	second, err := g.Start(t.Context(), Start{PatternIDs: []int64{id}, Replace: true})
	if err != nil {
		t.Fatal(err)
	}
	if second.Game.ID == first.Game.ID || second.Game.TotalCalled != 0 {
		t.Errorf("the replacement: %+v", second.Game)
	}
	if running, _ := g.Running(t.Context()); running != second.Game.ID {
		t.Errorf("running %d; want the replacement %d", running, second.Game.ID)
	}
	if n, _ := db.New(st.ReadDB()).CountWinners(t.Context()); n != 0 {
		t.Errorf("replacing logged %d winners; nobody confirmed them", n)
	}
}

func TestStartWithDeletedPatterns(t *testing.T) {
	g, _, _ := testGame(t)
	if _, err := g.Start(t.Context(), Start{PatternIDs: []int64{999}}); !errors.Is(err, ErrNoPatterns) {
		t.Errorf("start: %v, want ErrNoPatterns", err)
	}
	if running, _ := g.Running(t.Context()); running != 0 {
		t.Error("a game started with no patterns")
	}
}

func TestDraw(t *testing.T) {
	g, st, _ := testGame(t)
	if _, err := g.Draw(t.Context(), 0); !errors.Is(err, ErrNoGame) {
		t.Fatalf("no game: %v, want ErrNoGame", err)
	}
	start(t, g, st, topRow())
	seen := map[int]bool{}
	for i := 1; i <= 75; i++ {
		res := draw(t, g, 0)
		n := res.Drawn.Number
		if n < 1 || n > 75 || seen[n] || res.Drawn.CallOrder != i || res.Drawn.Letter != bingo.LetterForNumber(n) ||
			res.Game.TotalCalled != i || res.Game.LastDraw.Number != n {
			t.Fatalf("draw %d: %+v", i, res.Drawn)
		}
		seen[n] = true
	}
	if _, err := g.Draw(t.Context(), 0); !errors.Is(err, ErrAllDrawn) {
		t.Errorf("a 76th draw: %v, want ErrAllDrawn", err)
	}
}

// A postage-stamp game (no N cells) never draws an N, and runs out after B, I, G, O.
func TestDrawSkipsColumnsNoPatternUses(t *testing.T) {
	g, st, _ := testGame(t)
	start(t, g, st, postageStamps())
	count := 0
	for {
		res, err := g.Draw(t.Context(), 0)
		if errors.Is(err, ErrAllDrawn) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
		if res.Drawn.Number >= 31 && res.Drawn.Number <= 45 {
			t.Errorf("drew N%d, which no pattern uses", res.Drawn.Number)
		}
	}
	if count != 60 {
		t.Errorf("%d drawn; want 60", count)
	}
}

// Many draws at once never call a number twice.
func TestConcurrentDraws(t *testing.T) {
	g, st, _ := testGame(t)
	start(t, g, st, topRow())
	var wg sync.WaitGroup
	results := make(chan int, 120)
	for range 120 {
		wg.Go(func() {
			if res, err := g.Draw(t.Context(), 0); err == nil {
				results <- res.Drawn.Number
			} else if !errors.Is(err, ErrAllDrawn) {
				t.Errorf("draw: %v", err)
			}
		})
	}
	wg.Wait()
	close(results)
	seen := map[int]bool{}
	for n := range results {
		if seen[n] {
			t.Errorf("%d drawn twice", n)
		}
		seen[n] = true
	}
	if len(seen) != 75 {
		t.Errorf("%d drawn; want 75", len(seen))
	}
}

// A card completing a pattern wins, once; a personal card waiting for approval never
// does; a card made after the game read them plays once the cards change.
func TestWinners(t *testing.T) {
	g, st, rec := testGame(t)
	if err := st.SaveCard(t.Context(), "WIN001", winningBoard()); err != nil {
		t.Fatal(err)
	}
	pending, _ := json.Marshal(winningBoard())
	if err := db.New(st.DB()).CreateCardRequest(t.Context(), db.CreateCardRequestParams{
		ID: "PEND01", BoardData: string(pending), PlayerName: "Aria", World: "Gilgamesh",
	}); err != nil {
		t.Fatal(err)
	}
	start(t, g, st, topRow())
	// The first draw reads the cards; one made after it isn't seen until they change.
	if _, err := g.Draw(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCard(t.Context(), "LATE01", winningBoard()); err != nil {
		t.Fatal(err)
	}
	g.CardsChanged()

	newWinners := 0
	var last *DrawResult
	for {
		res, newWinner, err := g.drawLocked(t.Context(), 0)
		if errors.Is(err, ErrAllDrawn) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if newWinner {
			newWinners++
		}
		last = res
	}
	if !slices.Equal(last.Winners, []string{"LATE01", "WIN001"}) {
		t.Errorf("winners = %v; want LATE01 and WIN001, not the pending PEND01", last.Winners)
	}
	if newWinners != 1 {
		t.Errorf("%d draws made new winners; want the one that completed the row", newWinners)
	}
	if state, _ := g.State(t.Context(), true); !slices.Equal(state.Winners, last.Winners) {
		t.Errorf("the staff's state: winners %v", state.Winners)
	}
	if state, _ := g.State(t.Context(), false); len(state.Winners) != 0 {
		t.Errorf("anyone else's state: winners %v; want none", state.Winners)
	}
	// The first draw (Draw) told staff and players; drawLocked alone tells nobody.
	if n := len(rec.of("draw")); n != 2 {
		t.Errorf("%d draw events; want the first draw's two", n)
	}
}

// A draw tells staff at once with the number and the winners, and the boards the
// number alone.
func TestDrawEvents(t *testing.T) {
	g, st, rec := testGame(t)
	if err := st.SaveCard(t.Context(), "WIN001", winningBoard()); err != nil {
		t.Fatal(err)
	}
	id := start(t, g, st, topRow()).ID
	res := draw(t, g, 0)
	draws := rec.of("draw")
	if len(draws) != 2 {
		t.Fatalf("draw events: %+v", draws)
	}
	staff, ok := draws[0].msg.(*DrawEvent)
	if !ok || !draws[0].toGameStaff() || draws[0].game() != id || staff.Payload.Drawn != res.Drawn || staff.Payload.Winners == nil {
		t.Errorf("staff's draw: %+v to %+v", draws[0].msg, draws[0].to)
	}
	players, ok := draws[1].msg.(*PlayerDrawEvent)
	if !ok || !draws[1].toBoards() || draws[1].game() != id || players.Payload.Drawn != res.Drawn {
		t.Errorf("the boards' draw: %+v to %+v", draws[1].msg, draws[1].to)
	}
	if _, carries := draws[1].payload["winners"]; carries {
		t.Error("the boards' draw carries the winners")
	}
}

func TestParseWinners(t *testing.T) {
	for raw, want := range map[string]int{"": 0, `["ABC123","DEF456"]`: 2, "{bad json": 0, "null": 0} {
		if got := parseWinners(raw); got == nil || len(got) != want {
			t.Errorf("parseWinners(%q) = %v; want %d", raw, got, want)
		}
	}
}

// A number inside its draw delay is staff's alone: anyone else sees it once the delay
// has passed. The last draw says when players get it.
func TestDrawDelayHidesTheNumber(t *testing.T) {
	g, st, rec := testGame(t)
	start(t, g, st, topRow())
	first := draw(t, g, 0)
	now := time.Now()
	g.now = func() time.Time { return now }
	second := draw(t, g, 30)

	staff, _ := g.State(t.Context(), true)
	public, _ := g.State(t.Context(), false)
	if !slices.Equal(staff.Game.CalledNumbers, []int{first.Drawn.Number, second.Drawn.Number}) || staff.Game.TotalCalled != 2 {
		t.Errorf("staff see %v", staff.Game.CalledNumbers)
	}
	if !slices.Equal(public.Game.CalledNumbers, []int{first.Drawn.Number}) || public.Game.TotalCalled != 1 {
		t.Errorf("anyone else sees %v; want the first number alone", public.Game.CalledNumbers)
	}
	if l := staff.Game.LastDraw; l == nil || l.Number != second.Drawn.Number || l.CallOrder != 2 || l.PlayersAt == nil ||
		*l.PlayersAt != *stamp(now.Add(30 * time.Second)) {
		t.Errorf("staff's last draw = %+v", l)
	}
	if l := public.Game.LastDraw; l == nil || l.Number != first.Drawn.Number || l.CallOrder != 1 {
		t.Errorf("anyone else's last draw = %+v; want the first", l)
	}
	// Staff hear it at once; players after the delay.
	if draws := rec.of("draw"); len(draws) != 3 || !draws[2].toGameStaff() {
		t.Errorf("draw: %+v; want the first to both, the second to staff alone so far", draws)
	}

	g.now = func() time.Time { return now.Add(30 * time.Second) }
	if public, _ = g.State(t.Context(), false); public.Game.TotalCalled != 2 || public.Game.LastDraw.Number != second.Drawn.Number {
		t.Errorf("once the delay has passed: %+v", public.Game)
	}
}

// halftimeGame draws a B-column game to its half-time mark (7), the last draw with
// delay, and answers the game's id.
func halftimeGame(t *testing.T, g *Service, st *store.Store, delay int) int64 {
	t.Helper()
	game := start(t, g, st, bColumn())
	for range 6 {
		draw(t, g, 0)
	}
	draw(t, g, delay)
	return game.ID
}

// The half-time prompt is game state: it waits for one answer, naming its game;
// a second answer, or one for another game, is refused, and every console hears it
// was answered and by whom.
func TestHalftime(t *testing.T) {
	g, st, rec := testGame(t)
	id := start(t, g, st, bColumn()).ID
	if err := g.AnswerHalftime(t.Context(), id, false, "ann"); !errors.Is(err, ErrNoHalftime) {
		t.Errorf("before half-time: %v, want ErrNoHalftime", err)
	}
	for range 6 {
		draw(t, g, 0)
	}
	if len(rec.of("halftime_prompt")) != 0 {
		t.Fatal("prompted before the mark")
	}
	draw(t, g, 0)
	prompts := rec.of("halftime_prompt")
	if len(prompts) != 1 || !prompts[0].toGameStaff() || prompts[0].game() != id || prompts[0].payload["autoPaused"] != false {
		t.Fatalf("prompts: %+v", prompts)
	}
	staff, _ := g.State(t.Context(), true)
	if h := staff.Game.Halftime; h == nil || h.GameID != id || h.AutoPaused {
		t.Errorf("staff's halftime = %+v", h)
	}
	if public, _ := g.State(t.Context(), false); public.Game.Halftime != nil {
		t.Error("anyone else sees the prompt")
	}

	if err := g.AnswerHalftime(t.Context(), id+1, false, "ann"); !errors.Is(err, ErrHalftimeStale) {
		t.Errorf("another game's answer: %v, want ErrHalftimeStale", err)
	}
	if err := g.AnswerHalftime(t.Context(), id, false, "ann"); err != nil {
		t.Fatal(err)
	}
	if err := g.AnswerHalftime(t.Context(), id, true, "bob"); !errors.Is(err, ErrHalftimeAnswered) {
		t.Errorf("a second answer: %v, want ErrHalftimeAnswered", err)
	}
	resolved := rec.of("halftime_resolved")
	if len(resolved) != 1 || !resolved[0].toGameStaff() || resolved[0].game() != id ||
		resolved[0].payload["minigame"] != false || resolved[0].payload["by"] != "ann" {
		t.Errorf("halftime_resolved: %+v", resolved)
	}
	if staff, _ = g.State(t.Context(), true); staff.Game.Halftime != nil || staff.Game.MinigameAlerted {
		t.Errorf("after the answer: %+v", staff.Game)
	}
	if len(rec.of("minigame")) != 0 {
		t.Error("declining told the players about a mini-game")
	}

	// A new game starts afresh, and the old game's answer is stale.
	if err := g.AnswerHalftime(t.Context(), id, true, "ann"); !errors.Is(err, ErrHalftimeAnswered) {
		t.Errorf("still the same game: %v", err)
	}
	start(t, g, st, bColumn())
	if err := g.AnswerHalftime(t.Context(), id, true, "ann"); !errors.Is(err, ErrHalftimeStale) {
		t.Errorf("after a new game: %v, want ErrHalftimeStale", err)
	}
}

// A mini-game waits for the number that reached half-time to get to the players,
// then tells them; the state says so, for a board opening after.
func TestHalftimeMinigameWaitsForTheNumber(t *testing.T) {
	g, st, rec := testGame(t)
	id := halftimeGame(t, g, st, 1)
	if err := g.AnswerHalftime(t.Context(), id, true, "ann"); err != nil {
		t.Fatal(err)
	}
	if len(rec.of("minigame")) != 0 {
		t.Fatal("the players heard of the mini-game before the number")
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(rec.of("minigame")) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the players never heard of the mini-game")
		}
		time.Sleep(20 * time.Millisecond)
	}
	alerts := rec.of("minigame")
	if !alerts[0].toBoards() || alerts[0].game() != id {
		t.Errorf("minigame: %+v", alerts)
	}
	if public, _ := g.State(t.Context(), false); !public.Game.MinigameAlerted {
		t.Error("the state doesn't say the players were told")
	}
	if _, err := g.End(t.Context(), id, nil, "ann"); err != nil {
		t.Fatal(err)
	}
	if state, _ := g.State(t.Context(), false); state.Game != nil {
		t.Errorf("after the end: %+v", state.Game)
	}
}

// Ending names its game: a repeat says it had ended; a confirm for a game that ended
// while another started is refused and leaves the new one running. The confirmed
// winners are logged, with their players, the details and the patterns.
func TestEnd(t *testing.T) {
	g, st, rec := testGame(t)
	if out, err := g.End(t.Context(), 1, nil, "ann"); err != nil || out.Ended {
		t.Fatalf("nothing to end: %+v, %v", out, err)
	}
	if err := st.SaveCard(t.Context(), "WIN001", winningBoard()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.New(st.DB()).UpdateCard(t.Context(), db.UpdateCardParams{
		PlayerName: sql.NullString{String: "Cloud", Valid: true}, ID: "WIN001",
	}); err != nil {
		t.Fatal(err)
	}
	if err := g.Change(t.Context(), Changes{Details: new("Round 3")}); err != nil {
		t.Fatal(err)
	}
	first := start(t, g, st, topRow())
	draw(t, g, 0)

	out, err := g.End(t.Context(), first.ID, []string{"WIN001"}, "ann")
	if err != nil || !out.Ended || out.Logged != 1 {
		t.Fatalf("end: %+v, %v", out, err)
	}
	rows, err := db.New(st.ReadDB()).ListWinners(t.Context(), db.ListWinnersParams{Sort: "loggedAt", Dir: "desc", PageSize: 10})
	if err != nil || len(rows) != 1 || rows[0].CardID != "WIN001" || rows[0].PlayerName != "Cloud" ||
		rows[0].GameDetails != "Round 3" || rows[0].WinningPatterns != `["P"]` {
		t.Errorf("the log: %+v, %v", rows, err)
	}
	if ended := rec.of("game_ended"); len(ended) != 1 || !ended[0].toAll() || ended[0].game() != first.ID {
		t.Errorf("game_ended: %+v", ended)
	}
	if out, err = g.End(t.Context(), first.ID, []string{"WIN001"}, "bob"); err != nil || out.Ended || out.Logged != 0 {
		t.Errorf("a repeat: %+v, %v; want ended false, nothing logged", out, err)
	}
	if n := len(rec.of("game_ended")); n != 1 {
		t.Errorf("%d game_ended; a repeat that ended nothing said so", n)
	}

	second := start(t, g, st, topRow())
	if _, err := g.End(t.Context(), first.ID, []string{"WIN001"}, "bob"); !errors.Is(err, ErrEndStale) {
		t.Errorf("a confirm across games: %v, want ErrEndStale", err)
	}
	if running, _ := g.Running(t.Context()); running != second.ID {
		t.Errorf("the stale confirm ended the new game")
	}
	if n, _ := db.New(st.ReadDB()).CountWinners(t.Context()); n != 1 {
		t.Errorf("the log holds %d; the stale confirm logged", n)
	}
}

// -- The automatic draws -----------------------------------------------------

func TestAutoStateFollowsTheGame(t *testing.T) {
	g, st, _ := testGame(t)
	id := pattern(t, st, "Top Row", topRow())
	state, err := g.Start(t.Context(), Start{PatternIDs: []int64{id}, Auto: true, AutoInterval: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !state.Game.AutoEnabled || state.Game.AutoInterval != 20 {
		t.Errorf("started with them: %+v", state.Game)
	}
	if !g.autoDrawNow.Load() {
		t.Error("a game started with automatic draws doesn't draw at once")
	}
	state, err = g.Start(t.Context(), Start{PatternIDs: []int64{id}, AutoInterval: 3, Replace: true})
	if err != nil {
		t.Fatal(err)
	}
	if state.Game.AutoEnabled || state.Game.AutoInterval != bingo.MinAutoInterval {
		t.Errorf("a manual game: %+v", state.Game)
	}

	if err := g.Change(t.Context(), Changes{AutoEnabled: new(true), AutoInterval: new(15)}); err != nil {
		t.Fatal(err)
	}
	if on, interval := g.AutoState(); !on || interval != 15 {
		t.Errorf("after the change: %v, %d", on, interval)
	}
	if err := g.Change(t.Context(), Changes{AutoInterval: new(1)}); err != nil {
		t.Fatal(err)
	}
	if _, interval := g.AutoState(); interval != bingo.MinAutoInterval {
		t.Errorf("interval %d; want it held to %d", interval, bingo.MinAutoInterval)
	}
	if !g.DisableAuto() || g.DisableAuto() {
		t.Error("DisableAuto should report it was on, then off")
	}
}

// Half-time pauses running automatic draws; declining resumes them, choosing the
// mini-game or switching them by hand leaves them to staff.
func TestHalftimePausesAutoDraws(t *testing.T) {
	g, _, _ := testGame(t)
	if g.pauseForHalftime() {
		t.Error("paused draws that weren't running")
	}
	g.setAuto(true)
	if !g.pauseForHalftime() {
		t.Fatal("didn't pause running draws")
	}
	if on, _ := g.AutoState(); on {
		t.Error("still on after the pause")
	}
	if !g.resume() || g.resume() {
		t.Error("resume should switch them on once")
	}
	g.pauseForHalftime()
	g.clearResume()
	if g.resume() {
		t.Error("resumed after the mini-game was chosen")
	}
	g.setAuto(true)
	g.pauseForHalftime()
	g.setAuto(false)
	if g.resume() {
		t.Error("resumed after staff switched them by hand")
	}
}

// A draw at the half-time mark pauses the automatic draws, the state says so, and
// declining starts them again.
func TestHalftimeAnswerResumesAutoDraws(t *testing.T) {
	g, st, rec := testGame(t)
	id := start(t, g, st, bColumn()).ID
	for range 6 {
		draw(t, g, 0)
	}
	g.setAuto(true)
	res, newWinner, err := g.drawAuto(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	g.afterDraw(t.Context(), res, newWinner, 0)
	if on, _ := g.AutoState(); on {
		t.Fatal("half-time didn't pause them")
	}
	if p := rec.of("halftime_prompt"); len(p) != 1 || p[0].payload["autoPaused"] != true {
		t.Errorf("the prompt: %+v", p)
	}
	if a := rec.of("auto_draw"); len(a) == 0 || a[len(a)-1].payload["enabled"] != false || !a[len(a)-1].toGameStaff() {
		t.Errorf("auto_draw: %+v; want the pause, to the staff", a)
	}
	if state, _ := g.State(t.Context(), true); state.Game.Halftime == nil || !state.Game.Halftime.AutoPaused {
		t.Errorf("the state's prompt: %+v", state.Game.Halftime)
	}
	if err := g.AnswerHalftime(t.Context(), id, false, "ann"); err != nil {
		t.Fatal(err)
	}
	if on, _ := g.AutoState(); !on {
		t.Error("declining didn't resume them")
	}
}

func TestDrawAutoOnlyWhileOn(t *testing.T) {
	g, st, _ := testGame(t)
	start(t, g, st, topRow())
	if res, _, err := g.drawAuto(t.Context(), 0); err != nil || res != nil {
		t.Fatalf("off: %+v, %v", res, err)
	}
	g.setAuto(true)
	if res, _, err := g.drawAuto(t.Context(), 0); err != nil || res == nil {
		t.Fatalf("on: %+v, %v", res, err)
	}
	// A manual draw takes over: they switch off, and say so.
	draw(t, g, 0)
	if on, _ := g.AutoState(); on {
		t.Error("a manual draw left them on")
	}
	if res, _, _ := g.drawAuto(t.Context(), 0); res != nil {
		t.Error("drew after they were switched off")
	}
}

// The loop draws at once when switched on, then waits its interval; off, it parks.
func TestRunDrawsAtOnce(t *testing.T) {
	g, st, _ := testGame(t)
	start(t, g, st, topRow())
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		g.Run(ctx)
	}()
	defer func() { cancel(); <-done }()

	if err := g.Change(t.Context(), Changes{AutoEnabled: new(true), AutoInterval: new(600)}); err != nil {
		t.Fatal(err)
	}
	called := func() int {
		state, _ := g.State(t.Context(), true)
		return state.Game.TotalCalled
	}
	deadline := time.Now().Add(2 * time.Second)
	for called() < 1 {
		if time.Now().After(deadline) {
			t.Fatal("no draw at once")
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	if n := called(); n != 1 {
		t.Errorf("%d drawn; want the first alone", n)
	}
}

// -- The It's Yoever reaction --------------------------------------------------

func TestYoever(t *testing.T) {
	g, st, rec := testGame(t)
	if err := st.SaveCard(t.Context(), "AAA111", winningBoard()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.Yoever(t.Context(), "NOPE99"); !errors.Is(err, ErrNoCard) {
		t.Errorf("an unknown card: %v", err)
	}
	if _, _, err := g.Yoever(t.Context(), "AAA111"); !errors.Is(err, ErrNoGame) {
		t.Errorf("no game: %v", err)
	}
	start(t, g, st, topRow())
	out, wait, err := g.Yoever(t.Context(), "AAA111")
	if err != nil || wait != 0 || out.Count != 1 || out.CooldownSeconds != 180 {
		t.Fatalf("the first: %+v, %v, %v", out, wait, err)
	}
	if y := rec.of("yoever"); len(y) != 1 || !y[0].toAll() || y[0].payload["count"] != float64(1) || y[0].game() == 0 {
		t.Errorf("yoever: %+v", y)
	}
	if _, wait, _ = g.Yoever(t.Context(), "AAA111"); wait <= 0 {
		t.Error("a second within the cooldown wasn't held back")
	}
	if state, _ := g.State(t.Context(), false); state.Game.YoeverCount != 1 {
		t.Errorf("count %d; want 1", state.Game.YoeverCount)
	}
	if err := g.Change(t.Context(), Changes{YoeverEnabled: new(false)}); err != nil {
		t.Fatal(err)
	}
	if s := rec.of("yoever_switched"); len(s) != 1 || !s[0].toAll() || s[0].payload["enabled"] != false {
		t.Errorf("yoever_switched: %+v", s)
	}
	if _, _, err := g.Yoever(t.Context(), "AAA111"); !errors.Is(err, ErrYoeverOff) {
		t.Errorf("switched off: %v", err)
	}
	// A new game: on again, counted afresh, no card waiting.
	start(t, g, st, topRow())
	if out, wait, err = g.Yoever(t.Context(), "AAA111"); err != nil || wait != 0 || out.Count != 1 {
		t.Errorf("a new game: %+v, %v, %v", out, wait, err)
	}
}

func TestYoeverCooldown(t *testing.T) {
	g, _, _ := testGame(t)
	base := time.Unix(1_700_000_000, 0)
	cd := 3 * time.Minute
	if count, wait, err := g.countYoever("AAA111", base, cd); err != nil || wait != 0 || count != 1 {
		t.Fatalf("first: %d, %v, %v", count, wait, err)
	}
	if count, wait, _ := g.countYoever("AAA111", base.Add(30*time.Second), cd); count != 0 || wait != cd-30*time.Second {
		t.Errorf("within the cooldown: %d, %v", count, wait)
	}
	if count, _, _ := g.countYoever("BBB222", base.Add(31*time.Second), cd); count != 2 {
		t.Errorf("another card: %d; want 2", count)
	}
	if count, wait, _ := g.countYoever("AAA111", base.Add(cd+time.Second), cd); wait != 0 || count != 3 {
		t.Errorf("after the cooldown: %d, %v", count, wait)
	}
	if count, wait, _ := g.countYoever("AAA111", base.Add(cd+time.Second), 0); wait != 0 || count != 4 {
		t.Errorf("no cooldown: %d, %v", count, wait)
	}
}

// The delay and the details are saved, and every console told.
func TestChangeSavesAndTells(t *testing.T) {
	g, st, rec := testGame(t)
	if err := g.Change(t.Context(), Changes{Delay: new(15), Details: new("**Round 2**")}); err != nil {
		t.Fatal(err)
	}
	if d := g.DrawDelay(t.Context()); d != 15 {
		t.Errorf("draw delay %d; want 15", d)
	}
	if v, _ := db.New(st.ReadDB()).GetSetting(t.Context(), "game_details"); v != "**Round 2**" {
		t.Errorf("details saved as %q", v)
	}
	state, _ := g.State(t.Context(), false)
	if state.DrawDelay != 15 || state.GameDetails != "**Round 2**" {
		t.Errorf("the state: %+v", state)
	}
	if d := rec.of("draw_delay"); len(d) != 1 || d[0].payload["delay"] != float64(15) || !d[0].to.Staff || d[0].to.Players ||
		!slices.Equal(d[0].to.Perms, []string{permissions.BingoGame, permissions.SystemSettings}) {
		t.Errorf("draw_delay: %+v; want the delay, to the Game and Settings pages", d)
	}
	if d := rec.of("game_details"); len(d) != 1 || !d[0].toAll() || d[0].payload["details"] != "**Round 2**" {
		t.Errorf("game_details: %+v", d)
	}
}
