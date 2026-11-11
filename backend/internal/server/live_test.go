package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"app-suite/internal/api"
	"app-suite/internal/api/apitest"
)

// -- Following a stream --------------------------------------------------------

// stream is an event stream a test follows, read line by line as it comes.
type stream struct {
	t      *testing.T
	resp   *http.Response
	events <-chan apitest.Event
}

// event is one event of a stream, with its envelope read.
type event struct {
	sseID   string
	name    string
	ID      int64          `json:"id"`
	Type    string         `json:"type"`
	GameID  *int64         `json:"gameId"`
	At      string         `json:"at"`
	Payload map[string]any `json:"payload"`
}

// openStream opens path's event stream as c, sending lastID as Last-Event-ID when it
// isn't empty. The stream is open, and hears what is published, once this returns.
func openStream(t *testing.T, env *testEnv, c *http.Client, path, lastID string) *stream {
	t.Helper()
	resp := streamAnswer(t, env, c, path, lastID)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("open %s: %d %s", path, resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("open %s: Content-Type %q", path, ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "private, no-store" || resp.Header.Get("ETag") != "" {
		t.Errorf("open %s: Cache-Control %q, ETag %q; want private, no-store and none", path, cc, resp.Header.Get("ETag"))
	}
	s := &stream{t: t, resp: resp, events: apitest.Events(resp.Body)}
	t.Cleanup(func() {
		resp.Body.Close()
		// The reader stops before the test ends, so it never reports on a test that has.
		for range s.events {
		}
	})
	return s
}

// streamAnswer requests path's event stream as c and answers the response as it is.
func streamAnswer(t *testing.T, env *testEnv, c *http.Client, path, lastID string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, env.url(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// staffStream opens the staff's stream as c.
func staffStream(t *testing.T, env *testEnv, c *http.Client) *stream {
	t.Helper()
	return openStream(t, env, c, "/api/v1/events", "")
}

// boardStream opens a board's stream, signed in as nobody.
func boardStream(t *testing.T, env *testEnv, cardID string) *stream {
	t.Helper()
	return openStream(t, env, newClient(t, env), "/api/v1/board/"+cardID+"/events", "")
}

// next is the stream's next event within d; ok is false when none came, and ended
// true when the stream ended.
func (s *stream) next(d time.Duration) (e event, ok, ended bool) {
	select {
	case raw, open := <-s.events:
		if !open {
			return event{}, false, true
		}
		if err := json.Unmarshal(raw.Data, &e); err != nil {
			s.t.Fatalf("event %s: %v: %s", raw.Name, err, raw.Data)
		}
		e.sseID, e.name = raw.ID, raw.Name
		if e.Type != e.name {
			s.t.Errorf("event %s says its type is %s", e.name, e.Type)
		}
		if e.Payload == nil {
			s.t.Errorf("event %s has no payload object: %s", e.name, raw.Data)
		}
		return e, true, false
	case <-time.After(d):
		return event{}, false, false
	}
}

// expect reads until an event named name, failing if none comes within d.
func (s *stream) expect(name string, d time.Duration) event {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for {
		e, ok, ended := s.next(time.Until(deadline))
		switch {
		case ended:
			s.t.Fatalf("the stream ended waiting for %s", name)
		case !ok:
			s.t.Fatalf("no %s within %s", name, d)
		case e.name == name:
			return e
		}
	}
}

// expectNone reads for d and fails if an event named name comes.
func (s *stream) expectNone(name string, d time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for {
		e, ok, ended := s.next(time.Until(deadline))
		if !ok || ended {
			return
		}
		if e.name == name {
			s.t.Fatalf("heard %s: %+v", name, e)
		}
	}
}

// expectEnd reads until the stream ends, failing if it hasn't within d.
func (s *stream) expectEnd(d time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for {
		_, ok, ended := s.next(time.Until(deadline))
		if ended {
			return
		}
		if !ok {
			s.t.Fatalf("the stream is still open after %s", d)
		}
	}
}

// payload is a payload's field as a string, for comparing.
func (e event) str(key string) string {
	v, _ := e.Payload[key].(string)
	return v
}

func (e event) game() int64 {
	if e.GameID == nil {
		return 0
	}
	return *e.GameID
}

// -- The streams answer --------------------------------------------------------

// The staff's stream takes a signed-in account with something to follow; a board's
// takes a card id that is a card, and a playable one.
func TestStreamGuards(t *testing.T) {
	env := newTestEnv(t)
	anon := newClient(t, env)
	if _, got, _ := problem(t, streamAnswer(t, env, anon, "/api/v1/events", "")); got != http.StatusUnauthorized {
		t.Errorf("staff stream signed out: %d, want 401", got)
	}
	nothing := makeActiveUser(t, env, "nobody", "correct horse battery", []string{})
	if _, got, _ := problem(t, streamAnswer(t, env, nothing, "/api/v1/events", "")); got != http.StatusForbidden {
		t.Errorf("staff stream with no permission: %d, want 403", got)
	}

	if err := env.store.SaveCard(t.Context(), "PLAY01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	env.seedCardRequest(t, "PEND01", srvValidBoard())
	for path, want := range map[string]int{
		"/api/v1/board/NOPE99/events": http.StatusNotFound,
		"/api/v1/board/PEND01/events": http.StatusForbidden,
		"/api/v1/board/bad!/events":   http.StatusUnprocessableEntity,
	} {
		if _, got, _ := problem(t, streamAnswer(t, env, anon, path, "")); got != want {
			t.Errorf("%s: %d, want %d", path, got, want)
		}
	}
	if players, _ := env.hub.Streams(); players != 0 {
		t.Errorf("%d player streams open after refusals; want none", players)
	}
	// Any case opens the card's stream.
	boardStream(t, env, "play01")
	if players, _ := env.hub.Streams(); players != 1 {
		t.Errorf("%d player streams open; want 1", players)
	}
}

// The players' streams are bounded: past the limit, one more is refused with 503.
func TestBoardStreamLimit(t *testing.T) {
	env := newTestEnv(t)
	env.hub.SetPlayerLimit(1)
	if err := env.store.SaveCard(t.Context(), "PLAY01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	boardStream(t, env, "PLAY01")
	resp := streamAnswer(t, env, newClient(t, env), "/api/v1/board/PLAY01/events", "")
	if resp.Header.Get("Retry-After") == "" {
		t.Error("no Retry-After on the refusal")
	}
	if _, got, _ := problem(t, resp); got != http.StatusServiceUnavailable {
		t.Errorf("past the limit: %d, want 503", got)
	}
}

// The heartbeat is a named ping event, an envelope with no place in the feed.
func TestStreamHeartbeat(t *testing.T) {
	env := newTestEnv(t)
	env.hub.SetHeartbeat(50 * time.Millisecond)
	env.loginAdmin(t)
	if err := env.store.SaveCard(t.Context(), "PLAY01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]*stream{"staff": staffStream(t, env, env.client), "board": boardStream(t, env, "PLAY01")} {
		ping := s.expect("ping", 2*time.Second)
		if ping.sseID != "" || ping.ID != 0 || ping.GameID != nil || len(ping.Payload) != 0 || ping.At == "" {
			t.Errorf("%s's ping: %+v", name, ping)
		}
	}
}

// A draw reaches the staff at once, with its number and the winners, and a board
// only once its draw delay has passed, with the number alone.
func TestDrawReachesStaffAtOnceAndPlayersAfterTheDelay(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	if err := env.store.SaveCard(t.Context(), "PLAY01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	g := startGame(t, env, createPattern(t, env, "Test", testPattern5x5()))
	staff := staffStream(t, env, env.client)
	board := boardStream(t, env, "PLAY01")

	d := drawNumber(t, env, 1)
	got := staff.expect("draw", time.Second)
	drawnTo := func(e event) map[string]any { m, _ := e.Payload["drawn"].(map[string]any); return m }
	if got.game() != g.ID || drawnTo(got)["number"] != float64(d.Drawn.Number) || drawnTo(got)["callOrder"] != float64(1) {
		t.Errorf("staff's draw: %+v; want %d", got, d.Drawn.Number)
	}
	if _, has := got.Payload["winners"]; !has || got.sseID != strconv.FormatInt(got.ID, 10) {
		t.Errorf("staff's draw: winners present %v, SSE id %q for %d", has, got.sseID, got.ID)
	}
	board.expectNone("draw", 600*time.Millisecond)
	late := board.expect("draw", 3*time.Second)
	if late.game() != g.ID || drawnTo(late)["number"] != float64(d.Drawn.Number) || drawnTo(late)["letter"] != d.Drawn.Letter {
		t.Errorf("the board's draw: %+v", late)
	}
	if _, has := late.Payload["winners"]; has {
		t.Error("the board's draw carries the winners")
	}
}

// The game's events reach the staff with the Game page and the boards; an account
// without the page hears none of them.
func TestGameEventsFollowTheGamePage(t *testing.T) {
	env := newTestEnv(t)
	cards := makeActiveUser(t, env, "cardstaff", "correct horse battery", []string{"bingo-cards"})
	if err := env.store.SaveCard(t.Context(), "PLAY01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	pattern := createPattern(t, env, "Test", testPattern5x5())
	admin := staffStream(t, env, env.client)
	cardStaff := staffStream(t, env, cards)
	board := boardStream(t, env, "PLAY01")

	g := startGame(t, env, pattern)
	for name, s := range map[string]*stream{"admin": admin, "board": board} {
		if e := s.expect("game_started", time.Second); e.game() != g.ID || len(e.Payload) != 0 {
			t.Errorf("%s's game_started: %+v", name, e)
		}
	}
	decodeInto(t, env.patchJSON(t, "/api/v1/game", map[string]any{"details": "Round 2", "yoeverEnabled": false}), http.StatusOK, nil)
	for name, s := range map[string]*stream{"admin": admin, "board": board} {
		if e := s.expect("game_details", time.Second); e.str("details") != "Round 2" {
			t.Errorf("%s's game_details: %+v", name, e)
		}
		if e := s.expect("yoever_switched", time.Second); e.Payload["enabled"] != false {
			t.Errorf("%s's yoever_switched: %+v", name, e)
		}
	}
	decodeInto(t, env.postJSON(t, "/api/v1/game/end", map[string]any{"gameId": g.ID}), http.StatusOK, nil)
	for name, s := range map[string]*stream{"admin": admin, "board": board} {
		if e := s.expect("game_ended", time.Second); e.game() != g.ID {
			t.Errorf("%s's game_ended: %+v", name, e)
		}
	}
	cardStaff.expectNone("game_started", 300*time.Millisecond)
	cardStaff.expectNone("game_ended", 100*time.Millisecond)
}

// The card events reach the accounts with the Manage Cards page and nobody else: not
// an account with another page, not a board.
func TestCardEventsFollowTheCardsPage(t *testing.T) {
	env := newTestEnv(t)
	gameOnly := makeActiveUser(t, env, "gamestaff", "correct horse battery", []string{"bingo-game"})
	cardsToo := makeActiveUser(t, env, "cardstaff", "correct horse battery", []string{"bingo-cards"})
	if err := env.store.SaveCard(t.Context(), "PLAY01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	withPage := staffStream(t, env, cardsToo)
	without := staffStream(t, env, gameOnly)
	board := boardStream(t, env, "PLAY01")

	makeCard(t, env, "Guest")
	if e := withPage.expect("cards_changed", time.Second); len(e.Payload) != 0 || e.GameID != nil {
		t.Errorf("cards_changed: %+v", e)
	}
	without.expectNone("cards_changed", 300*time.Millisecond)
	board.expectNone("cards_changed", 100*time.Millisecond)
}

// The log reaches an admin's stream alone: not an account with every page but not an
// admin.
func TestLogEventsReachAdminsAlone(t *testing.T) {
	env := newTestEnv(t)
	staffAll := makeActiveUser(t, env, "staff", "correct horse battery", []string{"bingo-game", "bingo-cards", "system-settings"})
	admin := staffStream(t, env, env.client)
	other := staffStream(t, env, staffAll)

	tail := api.LogTail(env.hub)
	tail([]byte(`{"time":"2026-11-09T20:00:00Z","level":"INFO","msg":"http request","path":"/api/v1/game","status":200}`))
	e := admin.expect("log", time.Second)
	entry, _ := e.Payload["entry"].(map[string]any)
	if entry["message"] != "http request" || entry["level"] != "INFO" || e.ID != 0 || e.sseID != "" {
		t.Errorf("the admin's log event: %+v", e)
	}
	if fields, _ := entry["fields"].(map[string]any); fields["path"] != "/api/v1/game" {
		t.Errorf("its fields: %v", entry["fields"])
	}
	other.expectNone("log", 300*time.Millisecond)
}

// A resource change reaches the accounts whose pages show it alone.
func TestResourceChangedFollowsItsPage(t *testing.T) {
	env := newTestEnv(t)
	raffles := makeActiveUser(t, env, "raffler", "correct horse battery", []string{"teahouse-raffles"})
	garapons := makeActiveUser(t, env, "drummer", "correct horse battery", []string{"festival-garapon"})
	withPage := staffStream(t, env, raffles)
	without := staffStream(t, env, garapons)
	admin := staffStream(t, env, env.client)

	decodeInto(t, env.postJSON(t, "/api/v1/raffles", raffleBody(map[string]any{"maxEntries": 10})), http.StatusCreated, nil)
	for name, s := range map[string]*stream{"raffles": withPage, "admin": admin} {
		if e := s.expect("resource_changed", time.Second); e.str("resource") != "raffles" {
			t.Errorf("%s: %+v", name, e)
		}
	}
	without.expectNone("resource_changed", 300*time.Millisecond)

	// A new sign-up is the admins' to see on the Users page.
	decodeInto(t, postAs(t, newClient(t, env), env, "/api/v1/auth/register",
		map[string]string{"username": "newcomer", "password": "correct horse battery"}), http.StatusAccepted, nil)
	if e := admin.expect("resource_changed", time.Second); e.str("resource") != "users" {
		t.Errorf("the sign-up: %+v", e)
	}
	withPage.expectNone("resource_changed", 300*time.Millisecond)
}

// A theme written, whichever, and the settings saved reach every stream; saving the
// settings' draw delay reaches the Game page as a draw_delay.
func TestThemeAndSettingsEventsReachEveryone(t *testing.T) {
	env := newTestEnv(t)
	gameStaff := makeActiveUser(t, env, "gamestaff", "correct horse battery", []string{"bingo-game"})
	if err := env.store.SaveCard(t.Context(), "PLAY01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	staff := staffStream(t, env, gameStaff)
	board := boardStream(t, env, "PLAY01")

	var style struct{ ID int64 }
	decodeInto(t, env.postJSON(t, "/api/v1/styles", map[string]any{
		"name": "Night", "tokens": map[string]any{}, "boardFlourish": "", "numberFlourish": "", "isPublic": false,
	}), http.StatusCreated, &style)
	for name, s := range map[string]*stream{"staff": staff, "board": board} {
		if e := s.expect("theme_changed", time.Second); e.Payload["styleId"] != float64(style.ID) {
			t.Errorf("%s's theme_changed: %+v; want theme %d", name, e, style.ID)
		}
	}
	decodeInto(t, env.postJSON(t, "/api/v1/styles/deactivate", nil), http.StatusOK, nil)
	if e := board.expect("theme_changed", time.Second); e.Payload["styleId"] != nil {
		t.Errorf("going back to the default look: %+v", e)
	}

	var settings map[string]any
	decodeInto(t, env.get(t, "/api/v1/settings"), http.StatusOK, &settings)
	delete(settings, "uploadedFonts")
	settings["defaultDrawDelay"] = 12
	decodeInto(t, env.putJSON(t, "/api/v1/settings", settings), http.StatusOK, nil)
	for name, s := range map[string]*stream{"staff": staff, "board": board} {
		s.expect("settings_changed", time.Second)
		if name == "staff" {
			if e := s.expect("draw_delay", time.Second); e.Payload["delay"] != float64(12) {
				t.Errorf("draw_delay: %+v", e)
			}
		}
	}
	board.expectNone("draw_delay", 300*time.Millisecond)
}

// Deleting a card tells its board, whose stream then ends; a board on another card
// keeps its stream.
func TestCardDeletedEndsItsBoardsStream(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	for _, id := range []string{"GONE01", "KEEP01"} {
		if err := env.store.SaveCard(t.Context(), id, srvValidBoard()); err != nil {
			t.Fatal(err)
		}
	}
	gone := boardStream(t, env, "GONE01")
	kept := boardStream(t, env, "KEEP01")
	if resp := env.del(t, "/api/v1/cards/GONE01"); status(resp) != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	gone.expect("card_deleted", time.Second)
	gone.expectEnd(time.Second)
	kept.expectNone("card_deleted", 300*time.Millisecond)
	if players, _ := env.hub.Streams(); players != 1 {
		t.Errorf("%d boards open; want the kept one", players)
	}
}

// Deactivating an account ends its stream at once; so does a password change, which
// signs out its other sessions, and a sign-out.
func TestRevokedAccountsLoseTheirStream(t *testing.T) {
	env := newTestEnv(t)
	staff := makeActiveUser(t, env, "gamestaff", "correct horse battery", []string{"bingo-game"})
	s := staffStream(t, env, staff)
	id := findUserID(t, env, "gamestaff")
	decodeInto(t, env.patchJSON(t, "/api/v1/users/"+itoa(id), map[string]any{"isActive": false}), http.StatusOK, nil)
	s.expectEnd(2 * time.Second)

	decodeInto(t, env.patchJSON(t, "/api/v1/users/"+itoa(id), map[string]any{"isActive": true}), http.StatusOK, nil)
	first := newClient(t, env)
	signInAs(t, first, env, "gamestaff", "correct horse battery")
	second := newClient(t, env)
	signInAs(t, second, env, "gamestaff", "correct horse battery")
	s = staffStream(t, env, first)
	resp := reqAs(t, second, env, http.MethodPut, "/api/v1/me/password",
		map[string]string{"currentPassword": "correct horse battery", "newPassword": "a better horse battery"})
	if status(resp) != http.StatusNoContent {
		t.Fatalf("change password: %d", resp.StatusCode)
	}
	s.expectEnd(2 * time.Second)

	third := newClient(t, env)
	signInAs(t, third, env, "gamestaff", "a better horse battery")
	s = staffStream(t, env, third)
	if resp := reqAs(t, third, env, http.MethodPost, "/api/v1/auth/logout", nil); status(resp) != http.StatusNoContent {
		t.Fatalf("sign out: %d", resp.StatusCode)
	}
	s.expectEnd(2 * time.Second)
}

// An account that loses a permission stops hearing what it covered and keeps its
// stream; one checked on the heartbeat finds its account deleted and ends.
func TestLostPermissionStopsItsEvents(t *testing.T) {
	env := newTestEnv(t)
	staff := makeActiveUser(t, env, "staff", "correct horse battery", []string{"bingo-cards", "bingo-game"})
	s := staffStream(t, env, staff)
	id := findUserID(t, env, "staff")
	decodeInto(t, env.patchJSON(t, "/api/v1/users/"+itoa(id), map[string]any{"permissions": []string{"bingo-game"}}), http.StatusOK, nil)
	// The account change has every staff stream check its account; give it a moment.
	time.Sleep(200 * time.Millisecond)
	makeCard(t, env, "Guest")
	s.expectNone("cards_changed", 300*time.Millisecond)
	startGame(t, env, createPattern(t, env, "Test", testPattern5x5()))
	s.expect("game_started", time.Second)
}

// An account deleted behind the server's back, with no request to say so, is found
// gone at the next heartbeat's check, and its stream ends.
func TestHeartbeatChecksTheAccount(t *testing.T) {
	env := newTestEnv(t)
	staff := makeActiveUser(t, env, "staff", "correct horse battery", []string{"bingo-game"})
	id := findUserID(t, env, "staff")
	env.hub.SetHeartbeat(50 * time.Millisecond)
	s := staffStream(t, env, staff)
	s.expect("ping", time.Second)
	if _, err := env.store.DB().ExecContext(t.Context(), "DELETE FROM users WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}
	s.expectEnd(2 * time.Second)
}

// A stream reopened with Last-Event-ID gets what it missed, of what it may hear, in
// order, then carries on; one whose id has fallen out of the server's replay window
// is told to read everything again.
func TestStreamResumesFromLastEventID(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	if err := env.store.SaveCard(t.Context(), "PLAY01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	pattern := createPattern(t, env, "Test", testPattern5x5())
	first := boardStream(t, env, "PLAY01")
	g := startGame(t, env, pattern)
	started := first.expect("game_started", time.Second)
	if started.sseID == "" {
		t.Fatal("game_started carries no SSE id")
	}
	first.resp.Body.Close()

	d := drawNumber(t, env, 0)
	makeCard(t, env, "Guest")
	decodeInto(t, env.patchJSON(t, "/api/v1/game", map[string]any{"details": "Round 3"}), http.StatusOK, nil)

	again := openStream(t, env, newClient(t, env), "/api/v1/board/PLAY01/events", started.sseID)
	if e := again.expect("draw", time.Second); e.game() != g.ID {
		t.Errorf("the missed draw: %+v", e)
	} else if drawn, _ := e.Payload["drawn"].(map[string]any); drawn["number"] != float64(d.Drawn.Number) {
		t.Errorf("the missed draw's number: %v", drawn)
	}
	if e, ok, _ := again.next(time.Second); !ok || e.name != "game_details" {
		t.Errorf("after the draw: %+v; want game_details, and nothing the board may not hear", e)
	}
	decodeInto(t, env.patchJSON(t, "/api/v1/game", map[string]any{"yoeverEnabled": false}), http.StatusOK, nil)
	again.expect("yoever_switched", time.Second)

	stale := openStream(t, env, newClient(t, env), "/api/v1/board/PLAY01/events", "1")
	if e := stale.expect("resync", time.Second); e.sseID == "" || e.ID == 0 || len(e.Payload) != 0 {
		t.Errorf("resync: %+v", e)
	}
	stale.expectNone("draw", 200*time.Millisecond)
}

// A plugin's token opens the staff's stream as its account, as the header carries it.
func TestStaffStreamTakesAToken(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	var tok struct{ Token string }
	decodeInto(t, env.postJSON(t, "/api/v1/me/token", nil), http.StatusCreated, &tok)
	plugin := &http.Client{Transport: bearer{tok.Token, env.client.Transport}}
	s := staffStream(t, env, plugin)
	makeCard(t, env, "Guest")
	s.expect("cards_changed", time.Second)

	resp := streamAnswer(t, env, newClient(t, env), "/api/v1/events?token="+tok.Token, "")
	if _, got, _ := problem(t, resp); got != http.StatusUnauthorized {
		t.Errorf("a token in the URL: %d, want 401", got)
	}
}

// bearer sends a personal access token with every request.
type bearer struct {
	token string
	next  http.RoundTripper
}

func (b bearer) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(req)
}

// A hub that closes, as on shutdown, ends every open stream.
func TestHubCloseEndsStreams(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	s := staffStream(t, env, env.client)
	env.hub.Close()
	s.expectEnd(time.Second)
	if players, staffStreams := env.hub.Streams(); players != 0 || staffStreams != 0 {
		t.Errorf("open after the close: %d, %d", players, staffStreams)
	}
}

// A stream is written in the access log once, when it closes.
func TestStreamLoggedOnceWhenItCloses(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	lines := captureLog(t)
	s := staffStream(t, env, env.client)
	time.Sleep(100 * time.Millisecond)
	if n := countRequests(lines(), "/api/v1/events"); n != 0 {
		t.Errorf("%d access-log lines while the stream is open; want none", n)
	}
	env.hub.Close()
	s.expectEnd(time.Second)
	deadline := time.Now().Add(time.Second)
	for countRequests(lines(), "/api/v1/events") == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := countRequests(lines(), "/api/v1/events"); n != 1 {
		t.Errorf("%d access-log lines for the stream; want 1", n)
	}
}

// countRequests is how many access-log lines name path.
func countRequests(lines []map[string]any, path string) int {
	n := 0
	for _, l := range lines {
		if l["msg"] == "http request" && l["path"] == path {
			n++
		}
	}
	return n
}

// captureLog routes the default logger into memory for the test, and answers the
// lines written so far. The server writes from its own goroutines, so the memory is
// locked.
func captureLog(t *testing.T) func() []map[string]any {
	t.Helper()
	w := &lockedLog{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return w.lines
}

type lockedLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedLog) lines() []map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []map[string]any
	for line := range strings.SplitSeq(l.buf.String(), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}
