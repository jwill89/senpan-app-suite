package server_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"app-suite/internal/db"
)

// logWins writes a confirmed win for each player named, as a game's end does.
func logWins(t *testing.T, env *testEnv, players ...string) {
	t.Helper()
	q := db.New(env.store.DB())
	for i, p := range players {
		if err := q.AddWinner(t.Context(), db.AddWinnerParams{
			CardID: fmt.Sprintf("CARD%02d", i), PlayerName: p, GameDetails: "Round " + p, WinningPatterns: `["Top Row"]`,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

type winnersPage struct {
	Entries []map[string]any `json:"entries"`
	Total   int              `json:"total"`
	Page    int              `json:"page"`
	PerPage int              `json:"perPage"`
}

// players is each entry's player, in order.
func (p winnersPage) players() string {
	out := make([]string, 0, len(p.Entries))
	for _, e := range p.Entries {
		out = append(out, fmt.Sprint(e["playerName"]))
	}
	return strings.Join(out, ",")
}

// The Winners log page's permission guards every operation.
func TestWinnersLogGuard(t *testing.T) {
	env := newTestEnv(t)
	for _, path := range []string{"/api/v1/winners-log", "/api/v1/winners-log/frequent"} {
		if got := status(env.get(t, path)); got != http.StatusUnauthorized {
			t.Errorf("%s signed out: %d, want 401", path, got)
		}
	}
	addAccount(t, env, "presetsonly", "correct horse battery", []string{"bingo-presets"})
	env.signIn(t, "presetsonly", "correct horse battery")
	if got := status(env.get(t, "/api/v1/winners-log")); got != http.StatusForbidden {
		t.Errorf("without the page: %d, want 403", got)
	}
	if got := status(env.del(t, "/api/v1/winners-log/all")); got != http.StatusForbidden {
		t.Errorf("clearing without the page: %d, want 403", got)
	}
	addAccount(t, env, "logkeeper", "correct horse battery", []string{"bingo-winners-log"})
	env.signIn(t, "logkeeper", "correct horse battery")
	decodeInto(t, env.get(t, "/api/v1/winners-log"), http.StatusOK, nil)
}

// The log pages newest first by default, sorts by any of its fields either way, and
// a page past the end is empty but still tells the log's size.
func TestWinnersLogPages(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var page winnersPage
	decodeInto(t, env.get(t, "/api/v1/winners-log"), http.StatusOK, &page)
	if len(page.Entries) != 0 || page.Total != 0 || page.Page != 1 || page.PerPage != 25 {
		t.Errorf("the empty log: %+v", page)
	}

	logWins(t, env, "Cyra", "Alice", "Bob")
	decodeInto(t, env.get(t, "/api/v1/winners-log"), http.StatusOK, &page)
	if page.players() != "Bob,Alice,Cyra" || page.Total != 3 {
		t.Errorf("newest first: %s of %d", page.players(), page.Total)
	}
	e := page.Entries[0]
	if e["cardId"] != "CARD02" || e["gameDetails"] != "Round Bob" || e["winningPatterns"] != `["Top Row"]` || e["loggedAt"] == nil {
		t.Errorf("an entry: %v", e)
	}
	decodeInto(t, env.get(t, "/api/v1/winners-log?sort=playerName&dir=asc&perPage=2&page=2"), http.StatusOK, &page)
	if page.players() != "Cyra" || page.Total != 3 || page.Page != 2 || page.PerPage != 2 {
		t.Errorf("by name, the second page of 2: %+v", page)
	}
	decodeInto(t, env.get(t, "/api/v1/winners-log?page=9"), http.StatusOK, &page)
	if len(page.Entries) != 0 || page.Total != 3 {
		t.Errorf("a page past the end: %+v", page)
	}

	for query, want := range map[string]string{
		"perPage=500": "query.perPage", "perPage=0": "query.perPage", "page=0": "query.page",
		"sort=logged_at": "query.sort", "dir=up": "query.dir",
	} {
		if at := invalid(t, env.get(t, "/api/v1/winners-log?"+query)); !slices.Contains(at, want) {
			t.Errorf("%s: errors at %v, want %s", query, at, want)
		}
	}
}

// An entry is deleted (204, then 404), and the whole log cleared, saying how many
// entries went.
func TestWinnersLogDeletes(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	logWins(t, env, "Alice", "Bob", "Cyra")
	var page winnersPage
	decodeInto(t, env.get(t, "/api/v1/winners-log"), http.StatusOK, &page)
	path := fmt.Sprintf("/api/v1/winners-log/%d", int64(page.Entries[0]["id"].(float64)))

	if got := status(env.del(t, path)); got != http.StatusNoContent {
		t.Fatalf("delete: %d, want 204", got)
	}
	if got := status(env.del(t, path)); got != http.StatusNotFound {
		t.Errorf("deleting it again: %d, want 404", got)
	}
	var cleared map[string]any
	decodeInto(t, env.del(t, "/api/v1/winners-log/all"), http.StatusOK, &cleared)
	if cleared["deleted"] != float64(2) {
		t.Errorf("clearing the log: %v, want 2 deleted", cleared)
	}
	decodeInto(t, env.del(t, "/api/v1/winners-log/all"), http.StatusOK, &cleared)
	if cleared["deleted"] != float64(0) {
		t.Errorf("clearing an empty log: %v", cleared)
	}
}

// The frequent winners are those with the settings' threshold of wins in the
// window, most first; a lower threshold takes in more.
func TestWinnersLogFrequent(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	var list struct {
		Winners []map[string]any `json:"winners"`
	}
	decodeInto(t, env.get(t, "/api/v1/winners-log/frequent"), http.StatusOK, &list)
	if len(list.Winners) != 0 {
		t.Errorf("with no wins: %v", list.Winners)
	}

	logWins(t, env, "Alice", "Bob", "Alice", "Bob", "Alice", "", "", "")
	decodeInto(t, env.get(t, "/api/v1/winners-log/frequent"), http.StatusOK, &list)
	if len(list.Winners) != 1 || list.Winners[0]["playerName"] != "Alice" || list.Winners[0]["winCount"] != float64(3) {
		t.Errorf("at the default threshold of 3: %v", list.Winners)
	}

	decodeInto(t, saveSettings(t, env, map[string]any{"frequentWinnerThreshold": 2}), http.StatusOK, nil)
	decodeInto(t, env.get(t, "/api/v1/winners-log/frequent"), http.StatusOK, &list)
	if len(list.Winners) != 2 || list.Winners[0]["playerName"] != "Alice" || list.Winners[1]["playerName"] != "Bob" {
		t.Errorf("at a threshold of 2: %v", list.Winners)
	}
}
