package server_test

import (
	"net/http"
	"slices"
	"testing"
	"time"
)

// gameView is the game as get-game, start-game and the board answer it.
type gameView struct {
	ID              int64
	CalledNumbers   []int
	Patterns        []struct{ ID int64 }
	TotalCalled     int
	YoeverEnabled   bool
	YoeverCount     int
	AutoEnabled     bool
	AutoInterval    int
	MinigameAlerted bool
	Halftime        *struct {
		GameID     int64
		AutoPaused bool
	}
	LastDraw *struct {
		Number, CallOrder int
		Letter            string
		PlayersAt         *string
	}
}

// gameState is get-game's answer.
type gameState struct {
	Game        *gameView
	Winners     []string
	GameDetails string
	DrawDelay   int
}

// drawn is draw-number's answer.
type drawn struct {
	Drawn struct {
		Number, CallOrder int
		Letter            string
	}
	Game    gameView
	Winners []string
}

// readGame reads the game as c's account sees it.
func readGame(t *testing.T, c *http.Client, env *testEnv) gameState {
	t.Helper()
	var s gameState
	decodeInto(t, getAs(t, c, env, "/api/v1/game"), http.StatusOK, &s)
	return s
}

// startGame starts a game for the patterns as env.client's account, replacing any
// game running, and answers it.
func startGame(t *testing.T, env *testEnv, patterns ...int64) gameView {
	t.Helper()
	var s gameState
	decodeInto(t, env.postJSON(t, "/api/v1/game/start", map[string]any{"patternIds": patterns, "replace": true}), http.StatusOK, &s)
	return *s.Game
}

// drawNumber draws as env.client's account, with players getting it after delay seconds.
func drawNumber(t *testing.T, env *testEnv, delay int) drawn {
	t.Helper()
	var d drawn
	decodeInto(t, env.postJSON(t, "/api/v1/game/draw", map[string]any{"delay": delay}), http.StatusOK, &d)
	return d
}

// bColumnPattern makes a pattern on the B column alone: 15 numbers, half-time at 7.
func bColumnPattern(t *testing.T, env *testEnv) int64 {
	t.Helper()
	grid := make([][]bool, 5)
	for r := range grid {
		grid[r] = make([]bool, 5)
		grid[r][0] = true
	}
	return createPattern(t, env, "B Line", grid)
}

// The Game page's permission guards the commands: nobody signed in gets 401, an
// account without the page 403. The game, the board and the reaction are public.
func TestGameGuard(t *testing.T) {
	env := newTestEnv(t)
	calls := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/v1/game/start", map[string]any{"patternIds": []int{1}}},
		{http.MethodPost, "/api/v1/game/draw", map[string]any{}},
		{http.MethodPost, "/api/v1/game/end", map[string]any{"gameId": 1}},
		{http.MethodPost, "/api/v1/game/halftime", map[string]any{"gameId": 1, "minigame": true}},
		{http.MethodPatch, "/api/v1/game", map[string]any{"delay": 5}},
	}
	for _, call := range calls {
		if got := status(env.reqJSON(t, call.method, call.path, call.body)); got != http.StatusUnauthorized {
			t.Errorf("%s %s signed out: %d, want 401", call.method, call.path, got)
		}
	}
	cards := makeActiveUser(t, env, "cardstaff", "correct horse battery", []string{"bingo-cards"})
	for _, call := range calls {
		if got := status(reqAs(t, cards, env, call.method, call.path, call.body)); got != http.StatusForbidden {
			t.Errorf("%s %s without the page: %d, want 403", call.method, call.path, got)
		}
	}
	if s := readGame(t, newClient(t, env), env); s.Game != nil {
		t.Errorf("the game, signed out: %+v", s)
	}
	game := makeActiveUser(t, env, "caller", "correct horse battery", []string{"bingo-game"})
	pattern := createPattern(t, env, "Top Row", testPattern5x5())
	decodeInto(t, postAs(t, game, env, "/api/v1/game/start", map[string]any{"patternIds": []int64{pattern}}), http.StatusOK, nil)
}

// A game starts, draws, and ends once: the end names it, and a repeat says it had
// already ended.
func TestGameLifecycle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	if s := readGame(t, env.client, env); s.Game != nil || len(s.Winners) != 0 {
		t.Fatalf("no game: %+v", s)
	}
	pattern := createPattern(t, env, "Test", testPattern5x5())

	var started gameState
	decodeInto(t, env.postJSON(t, "/api/v1/game/start", map[string]any{"patternIds": []int64{pattern}}), http.StatusOK, &started)
	g := started.Game
	if g == nil || g.ID == 0 || len(g.Patterns) != 1 || g.TotalCalled != 0 || !g.YoeverEnabled || g.AutoEnabled || g.LastDraw != nil {
		t.Fatalf("started: %+v", g)
	}
	if s := readGame(t, env.client, env); s.Game == nil || s.Game.ID != g.ID {
		t.Errorf("get-game: %+v", s)
	}

	d := drawNumber(t, env, 0)
	if d.Drawn.Number < 1 || d.Drawn.Number > 75 || d.Drawn.Letter == "" || d.Drawn.CallOrder != 1 ||
		d.Game.TotalCalled != 1 || d.Game.LastDraw == nil || d.Game.LastDraw.Number != d.Drawn.Number {
		t.Errorf("drawn: %+v", d)
	}

	var end struct {
		Ended  bool
		Logged int
	}
	decodeInto(t, env.postJSON(t, "/api/v1/game/end", map[string]any{"gameId": g.ID}), http.StatusOK, &end)
	if !end.Ended || end.Logged != 0 {
		t.Errorf("end: %+v", end)
	}
	decodeInto(t, env.postJSON(t, "/api/v1/game/end", map[string]any{"gameId": g.ID}), http.StatusOK, &end)
	if end.Ended {
		t.Error("a second end says it ended the game")
	}
	if s := readGame(t, env.client, env); s.Game != nil {
		t.Errorf("after the end: %+v", s.Game)
	}
}

// A start without patterns, or with only deleted ones, is a 422; one while a game is
// running is a 409 unless it replaces it, and the replaced game's winners aren't
// logged (nobody confirmed them).
func TestGameStartRefusals(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	if at := invalid(t, env.postJSON(t, "/api/v1/game/start", map[string]any{"patternIds": []int{}})); !slices.Contains(at, "body.patternIds") {
		t.Errorf("no patterns: errors at %v", at)
	}
	// Only deleted patterns: TestGameStartRejectsDeletedPatterns.

	if err := env.store.SaveCard(t.Context(), "WIN001", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	pattern := createPattern(t, env, "Top Row", testPattern5x5())
	var first gameState
	decodeInto(t, env.postJSON(t, "/api/v1/game/start", map[string]any{"patternIds": []int64{pattern}}), http.StatusOK, &first)
	var d drawn
	for d.Game.TotalCalled < 75 {
		d = drawNumber(t, env, 0)
	}
	if !slices.Contains(d.Winners, "WIN001") {
		t.Fatalf("winners %v; want WIN001", d.Winners)
	}

	if _, got, _ := problem(t, env.postJSON(t, "/api/v1/game/start", map[string]any{"patternIds": []int64{pattern}})); got != http.StatusConflict {
		t.Errorf("a start while one runs: %d, want 409", got)
	}
	if s := readGame(t, env.client, env); s.Game.ID != first.Game.ID {
		t.Fatal("the refused start replaced the game")
	}
	second := startGame(t, env, pattern)
	if second.ID == first.Game.ID || second.TotalCalled != 0 {
		t.Errorf("the replacement: %+v", second)
	}
	var log winnersPage
	decodeInto(t, env.get(t, "/api/v1/winners-log"), http.StatusOK, &log)
	if log.Total != 0 {
		t.Errorf("replacing logged %d winners", log.Total)
	}
}

// A draw with no game, or a delay out of range, is refused.
func TestGameDrawRefusals(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	if _, got, _ := problem(t, env.postJSON(t, "/api/v1/game/draw", map[string]any{})); got != http.StatusConflict {
		t.Errorf("no game: %d, want 409", got)
	}
	startGame(t, env, createPattern(t, env, "Test", testPattern5x5()))
	if at := invalid(t, env.postJSON(t, "/api/v1/game/draw", map[string]any{"delay": 61})); !slices.Contains(at, "body.delay") {
		t.Errorf("a delay of 61: errors at %v", at)
	}
}

// The end names its game: a confirm left open while another game started is a 409
// and leaves the new game running; the confirmed winners are logged, and every
// console hears that the winners log changed.
func TestGameEndNamesItsGame(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	if err := env.store.SaveCard(t.Context(), "WIN001", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	pattern := createPattern(t, env, "Top Row", testPattern5x5())
	old := startGame(t, env, pattern)
	current := startGame(t, env, pattern)

	if _, got, _ := problem(t, env.postJSON(t, "/api/v1/game/end", map[string]any{"gameId": old.ID, "validWinnerIds": []string{"WIN001"}})); got != http.StatusConflict {
		t.Errorf("a stale confirm: %d, want 409", got)
	}
	if s := readGame(t, env.client, env); s.Game == nil || s.Game.ID != current.ID {
		t.Fatal("the stale confirm ended the new game")
	}

	other := newClient(t, env)
	signInAs(t, other, env, seedAdminUser, seedAdminPass)
	s := staffStream(t, env, other)

	var end struct {
		Ended  bool
		Logged int
	}
	decodeInto(t, env.postJSON(t, "/api/v1/game/end", map[string]any{"gameId": current.ID, "validWinnerIds": []string{"WIN001"}}), http.StatusOK, &end)
	if !end.Ended || end.Logged != 1 {
		t.Errorf("end: %+v", end)
	}
	if e := s.expect("game_ended", 3*time.Second); e.game() != current.ID {
		t.Errorf("game_ended: %+v; want game %d", e, current.ID)
	}
	if e := s.expect("resource_changed", 3*time.Second); e.str("resource") != "winners-log" {
		t.Errorf("resource_changed: %+v; want winners-log", e)
	}
	var log winnersPage
	decodeInto(t, env.get(t, "/api/v1/winners-log"), http.StatusOK, &log)
	if log.Total != 1 || log.Entries[0]["cardId"] != "WIN001" || log.Entries[0]["winningPatterns"] != `["Top Row"]` {
		t.Errorf("the log: %+v", log)
	}
	decodeInto(t, env.postJSON(t, "/api/v1/game/end", map[string]any{"gameId": current.ID, "validWinnerIds": []string{"WIN001"}}), http.StatusOK, &end)
	if end.Ended || end.Logged != 0 {
		t.Errorf("a repeat: %+v; want ended false, nothing logged", end)
	}
}

// A number inside its draw delay is the staff's alone: the public game and the board
// show it once the delay has passed, as the boards' stream does, while staff with the
// Game page see it at once. An account without the page sees what players do. The
// read differs by who asks, so it is never cached.
func TestGameDrawDelayHidesTheNumber(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	if err := env.store.SaveCard(t.Context(), "PLAY01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	startGame(t, env, createPattern(t, env, "Test", testPattern5x5()))
	first := drawNumber(t, env, 0)
	second := drawNumber(t, env, 30)

	anon := newClient(t, env)
	cards := makeActiveUser(t, env, "cardstaff", "correct horse battery", []string{"bingo-cards"})
	for who, c := range map[string]*http.Client{"signed out": anon, "without the Game page": cards} {
		s := readGame(t, c, env)
		if !slices.Equal(s.Game.CalledNumbers, []int{first.Drawn.Number}) || s.Game.TotalCalled != 1 || len(s.Winners) != 0 {
			t.Errorf("%s, the game: %+v", who, s)
		}
		if l := s.Game.LastDraw; l == nil || l.Number != first.Drawn.Number || l.PlayersAt == nil {
			t.Errorf("%s, the last draw: %+v", who, l)
		}
		var b struct{ Game *gameView }
		decodeInto(t, getAs(t, c, env, "/api/v1/board/PLAY01"), http.StatusOK, &b)
		if !slices.Equal(b.Game.CalledNumbers, []int{first.Drawn.Number}) {
			t.Errorf("%s, the board: %v", who, b.Game.CalledNumbers)
		}
	}
	staff := readGame(t, env.client, env)
	if !slices.Equal(staff.Game.CalledNumbers, []int{first.Drawn.Number, second.Drawn.Number}) {
		t.Errorf("staff see %v", staff.Game.CalledNumbers)
	}
	l := staff.Game.LastDraw
	if l == nil || l.Number != second.Drawn.Number || l.CallOrder != 2 || l.PlayersAt == nil {
		t.Fatalf("staff's last draw: %+v", l)
	}
	if at, err := time.Parse(time.RFC3339, *l.PlayersAt); err != nil || time.Until(at) < 25*time.Second {
		t.Errorf("players get it at %s; want about 30 seconds from now", *l.PlayersAt)
	}
	var b struct{ Game *gameView }
	decodeInto(t, env.get(t, "/api/v1/board/PLAY01"), http.StatusOK, &b)
	if b.Game.TotalCalled != 2 {
		t.Errorf("staff's board: %v", b.Game.CalledNumbers)
	}

	for _, path := range []string{"/api/v1/game", "/api/v1/board/PLAY01"} {
		resp := getAs(t, anon, env, path)
		resp.Body.Close()
		if cc := resp.Header.Get("Cache-Control"); cc != "private, no-store" || resp.Header.Get("ETag") != "" {
			t.Errorf("%s: Cache-Control %q, ETag %q; want private, no-store and none", path, cc, resp.Header.Get("ETag"))
		}
	}
}

// The half-time prompt is game state for a console opening mid-way; the answer names
// its game, is taken once, and every staff connection hears who answered it.
func TestGameHalftimePrompt(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	g := startGame(t, env, bColumnPattern(t, env))
	if _, got, _ := problem(t, env.postJSON(t, "/api/v1/game/halftime", map[string]any{"gameId": g.ID, "minigame": false})); got != http.StatusConflict {
		t.Errorf("before half-time: %d, want 409", got)
	}

	first := staffStream(t, env, env.client)
	other := newClient(t, env)
	signInAs(t, other, env, seedAdminUser, seedAdminPass)
	second := staffStream(t, env, other)

	for range 7 {
		drawNumber(t, env, 0)
	}
	for _, s := range []*stream{first, second} {
		if e := s.expect("halftime_prompt", 3*time.Second); e.game() != g.ID || e.Payload["autoPaused"] != false {
			t.Errorf("halftime_prompt: %+v", e)
		}
	}
	// A console opening now sees the prompt waiting; the public doesn't.
	if h := readGame(t, other, env).Game.Halftime; h == nil || h.GameID != g.ID || h.AutoPaused {
		t.Errorf("the staff's state: %+v", h)
	}
	if h := readGame(t, newClient(t, env), env).Game.Halftime; h != nil {
		t.Errorf("the public state: %+v", h)
	}

	if _, got, _ := problem(t, env.postJSON(t, "/api/v1/game/halftime", map[string]any{"gameId": g.ID + 1, "minigame": false})); got != http.StatusConflict {
		t.Errorf("another game's answer: %d, want 409", got)
	}
	var answered gameState
	decodeInto(t, env.postJSON(t, "/api/v1/game/halftime", map[string]any{"gameId": g.ID, "minigame": true}), http.StatusOK, &answered)
	if answered.Game.Halftime != nil {
		t.Errorf("the answer's game still has the prompt: %+v", answered.Game.Halftime)
	}
	for _, s := range []*stream{first, second} {
		e := s.expect("halftime_resolved", 3*time.Second)
		if e.game() != g.ID || e.Payload["minigame"] != true || e.str("by") != seedAdminUser {
			t.Errorf("halftime_resolved: %+v", e)
		}
	}
	if _, got, _ := problem(t, postAs(t, other, env, "/api/v1/game/halftime", map[string]any{"gameId": g.ID, "minigame": false})); got != http.StatusConflict {
		t.Errorf("a second answer, from the other console: %d, want 409", got)
	}
	if !readGame(t, env.client, env).Game.MinigameAlerted {
		t.Error("the state doesn't say the players were told about the mini-game")
	}
}

// The controls change only what is sent, each answered with the game; the delay is
// the settings' and the state's, and one out of range is a 422.
func TestGameControls(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	var s gameState
	decodeInto(t, env.patchJSON(t, "/api/v1/game", map[string]any{"delay": 15, "details": "**Round 2**"}), http.StatusOK, &s)
	if s.DrawDelay != 15 || s.GameDetails != "**Round 2**" || s.Game != nil {
		t.Errorf("the answer: %+v", s)
	}
	if got := siteSettings(t, env)["defaultDrawDelay"]; got != float64(15) {
		t.Errorf("defaultDrawDelay = %v; want 15", got)
	}
	if s = readGame(t, newClient(t, env), env); s.DrawDelay != 15 || s.GameDetails != "**Round 2**" {
		t.Errorf("the public state: %+v", s)
	}
	if at := invalid(t, env.patchJSON(t, "/api/v1/game", map[string]any{"delay": 999})); !slices.Contains(at, "body.delay") {
		t.Errorf("a delay of 999: errors at %v", at)
	}
}

// The board is the card and the game as players see it; a preview is the card alone.
// An id that isn't six letters or digits is a 422, an unknown card a 404, and any
// case finds the card.
func TestBoard(t *testing.T) {
	env := newTestEnv(t)
	if err := env.store.SaveCard(t.Context(), "TEST01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	var b struct {
		Card        map[string]any
		Game        *gameView
		GameDetails *string
	}
	decodeInto(t, env.get(t, "/api/v1/board/test01"), http.StatusOK, &b)
	if b.Card["id"] != "TEST01" || b.Card["boardData"] == nil || b.Game != nil || b.GameDetails == nil {
		t.Errorf("no game: %+v", b)
	}
	if got := status(env.get(t, "/api/v1/board/ZZZZZZ")); got != http.StatusNotFound {
		t.Errorf("an unknown card: %d, want 404", got)
	}
	if at := invalid(t, env.get(t, "/api/v1/board/ZZZ")); !slices.Contains(at, "path.id") {
		t.Errorf("a bad id: errors at %v", at)
	}

	env.loginAdmin(t)
	startGame(t, env, createPattern(t, env, "Test", testPattern5x5()))
	drawNumber(t, env, 0)
	anon := newClient(t, env)
	decodeInto(t, getAs(t, anon, env, "/api/v1/board/TEST01"), http.StatusOK, &b)
	if b.Game == nil || b.Game.TotalCalled != 1 || b.Game.MinigameAlerted {
		t.Errorf("with a game: %+v", b.Game)
	}
	b.Game = nil
	decodeInto(t, getAs(t, anon, env, "/api/v1/board/TEST01?preview=1"), http.StatusOK, &b)
	if b.Card["id"] != "TEST01" || b.Game != nil {
		t.Errorf("a preview: %+v", b)
	}
}
