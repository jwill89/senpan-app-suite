package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// standardCategory is the pattern category the migrations seed, first in order.
const standardCategory = 1

func testPattern5x5() [][]bool {
	grid := make([][]bool, 5)
	for i := range grid {
		grid[i] = make([]bool, 5)
	}
	for c := range 5 {
		grid[0][c] = true
	}
	return grid
}

// testPattern5x5Alt returns a different 5x5 pattern (bottom row) to avoid
// duplicate detection when a test needs two distinct patterns.
func testPattern5x5Alt() [][]bool {
	grid := make([][]bool, 5)
	for i := range grid {
		grid[i] = make([]bool, 5)
	}
	for c := range 5 {
		grid[4][c] = true
	}
	return grid
}

// diagonal is a third grid, unlike either of the above.
func diagonal() [][]bool {
	grid := make([][]bool, 5)
	for i := range grid {
		grid[i] = make([]bool, 5)
		grid[i][i] = true
	}
	return grid
}

// createPattern makes a pattern in the seeded category through the API and answers
// its id, failing the test unless it was made.
func createPattern(t *testing.T, env *testEnv, name string, grid [][]bool) int64 {
	t.Helper()
	var p map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/patterns", map[string]any{
		"name": name, "patternData": grid, "categoryId": standardCategory,
	}), http.StatusCreated, &p)
	return int64(p["id"].(float64))
}

// seedPattern makes a pattern straight in the database, in the seeded category.
func (e *testEnv) seedPattern(t *testing.T, name string) int64 {
	t.Helper()
	id, err := e.store.SavePattern(t.Context(), name, diagonal(), 0)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// patternList reads every pattern and category.
func patternList(t *testing.T, env *testEnv) (patterns, categories []map[string]any) {
	t.Helper()
	var list struct {
		Patterns   []map[string]any `json:"patterns"`
		Categories []map[string]any `json:"categories"`
	}
	decodeInto(t, env.get(t, "/api/v1/patterns"), http.StatusOK, &list)
	return list.Patterns, list.Categories
}

// names is each item's name, in order.
func names(items []map[string]any) string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, fmt.Sprint(it["name"]))
	}
	return strings.Join(out, ",")
}

// invalid reads a 422's problem and answers the location of each field it names.
func invalid(t *testing.T, resp *http.Response) []string {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d, want 422: %s", resp.StatusCode, body)
	}
	var p struct {
		Errors []struct {
			Location string `json:"location"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	var at []string
	for _, e := range p.Errors {
		at = append(at, e.Location)
	}
	return at
}

// status reads a response's status and closes it.
func status(resp *http.Response) int {
	resp.Body.Close()
	return resp.StatusCode
}

// The Patterns page's permission guards every operation: nobody signed in gets 401,
// an account without the page 403, and its grantee gets through.
func TestPatternsGuard(t *testing.T) {
	env := newTestEnv(t)
	if got := status(env.get(t, "/api/v1/patterns")); got != http.StatusUnauthorized {
		t.Errorf("signed out: %d, want 401", got)
	}

	addAccount(t, env, "presetsonly", "correct horse battery", []string{"bingo-presets"})
	env.signIn(t, "presetsonly", "correct horse battery")
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/patterns"}, {http.MethodPost, "/api/v1/patterns"},
		{http.MethodPost, "/api/v1/pattern-categories"}, {http.MethodDelete, "/api/v1/pattern-categories/1"},
	} {
		if got := status(env.reqJSON(t, call.method, call.path, map[string]any{})); got != http.StatusForbidden {
			t.Errorf("%s %s without the page: %d, want 403", call.method, call.path, got)
		}
	}

	addAccount(t, env, "patterner", "correct horse battery", []string{"bingo-patterns"})
	env.signIn(t, "patterner", "correct horse battery")
	createPattern(t, env, "Top Row", testPattern5x5())
	if patterns, _ := patternList(t, env); len(patterns) != 1 {
		t.Errorf("the grantee's list: %v", patterns)
	}
}

// A pattern is made last in its category and answered in full; the list has it.
func TestPatternsCreateAndList(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/patterns", map[string]any{
		"name": "  Top Row ", "patternData": testPattern5x5(), "categoryId": standardCategory,
	}), http.StatusCreated, &made)
	if made["name"] != "Top Row" || made["categoryId"] != float64(standardCategory) || made["categoryName"] != "Standard" ||
		made["sortOrder"] != float64(1) || fmt.Sprint(made["patternData"]) != fmt.Sprint(testPattern5x5()) {
		t.Errorf("made: %v", made)
	}
	createPattern(t, env, "Bottom Row", testPattern5x5Alt())

	patterns, categories := patternList(t, env)
	if names(patterns) != "Top Row,Bottom Row" || names(categories) != "Standard" {
		t.Errorf("list: %s / %s", names(patterns), names(categories))
	}
}

// A grid that isn't 5x5, a blank name and a category that isn't there are each a 422
// naming the field; a grid another pattern has is a 409 naming that pattern.
func TestPatternsCreateRefusesBadInput(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	fourWide := [][]bool{{true, true, true, true}, {}, {}, {}, {}}
	for _, c := range []struct {
		body map[string]any
		want string
	}{
		{map[string]any{"name": "Bad", "patternData": [][]bool{{true}}, "categoryId": 1}, "body.patternData"},
		{map[string]any{"name": "Bad", "patternData": fourWide, "categoryId": 1}, "body.patternData[0]"},
		{map[string]any{"name": "   ", "patternData": testPattern5x5(), "categoryId": 1}, "body.name"},
		{map[string]any{"name": "", "patternData": testPattern5x5(), "categoryId": 1}, "body.name"},
		{map[string]any{"name": "Lost", "patternData": testPattern5x5(), "categoryId": 999}, "body.categoryId"},
	} {
		if at := invalid(t, env.postJSON(t, "/api/v1/patterns", c.body)); !slices.Contains(at, c.want) {
			t.Errorf("%v: errors at %v, want %s", c.body, at, c.want)
		}
	}

	createPattern(t, env, "Top Row", testPattern5x5())
	_, got, detail := problem(t, env.postJSON(t, "/api/v1/patterns", map[string]any{
		"name": "Also Top Row", "patternData": testPattern5x5(), "categoryId": standardCategory,
	}))
	if got != http.StatusConflict || !strings.Contains(detail, `"Top Row"`) || !strings.Contains(detail, `"Standard"`) {
		t.Errorf("a duplicate grid: %d %q, want 409 naming the pattern and its category", got, detail)
	}
	if patterns, _ := patternList(t, env); len(patterns) != 1 {
		t.Errorf("after the refusals: %s", names(patterns))
	}
}

// A PATCH renames, or moves a pattern to the end of another category, or both, and
// leaves what it doesn't send.
func TestPatternsUpdate(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := createPattern(t, env, "Old", testPattern5x5())
	var bonus map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/pattern-categories", map[string]any{"name": "Bonus"}), http.StatusCreated, &bonus)
	bonusID := int64(bonus["id"].(float64))
	other := env.seedPattern(t, "Already There")
	path := fmt.Sprintf("/api/v1/patterns/%d", id)

	var p map[string]any
	decodeInto(t, env.patchJSON(t, path, map[string]any{"name": " New "}), http.StatusOK, &p)
	if p["name"] != "New" || p["categoryId"] != float64(standardCategory) {
		t.Errorf("after a rename: %v", p)
	}
	decodeInto(t, env.patchJSON(t, fmt.Sprintf("/api/v1/patterns/%d", other), map[string]any{"categoryId": bonusID}), http.StatusOK, nil)
	decodeInto(t, env.patchJSON(t, path, map[string]any{"categoryId": bonusID}), http.StatusOK, &p)
	if p["name"] != "New" || p["categoryId"] != float64(bonusID) || p["categoryName"] != "Bonus" {
		t.Errorf("after a move: %v", p)
	}
	decodeInto(t, env.patchJSON(t, path, map[string]any{}), http.StatusOK, &p)
	if p["name"] != "New" || p["categoryId"] != float64(bonusID) {
		t.Errorf("after an empty change: %v", p)
	}
	if patterns, _ := patternList(t, env); names(patterns) != "Already There,New" {
		t.Errorf("a moved pattern goes last: %s", names(patterns))
	}

	if at := invalid(t, env.patchJSON(t, path, map[string]any{"name": " "})); !slices.Contains(at, "body.name") {
		t.Errorf("a blank name: errors at %v", at)
	}
	if at := invalid(t, env.patchJSON(t, path, map[string]any{"categoryId": 999})); !slices.Contains(at, "body.categoryId") {
		t.Errorf("an unknown category: errors at %v", at)
	}
	if got := status(env.patchJSON(t, "/api/v1/patterns/999", map[string]any{"name": "X"})); got != http.StatusNotFound {
		t.Errorf("an unknown pattern: %d, want 404", got)
	}
}

// A delete answers 204, then 404 for the same pattern.
func TestPatternsDelete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := createPattern(t, env, "Temp", testPattern5x5())
	path := fmt.Sprintf("/api/v1/patterns/%d", id)

	if got := status(env.del(t, path)); got != http.StatusNoContent {
		t.Fatalf("delete: %d, want 204", got)
	}
	if patterns, _ := patternList(t, env); len(patterns) != 0 {
		t.Errorf("after the delete: %s", names(patterns))
	}
	if got := status(env.del(t, path)); got != http.StatusNotFound {
		t.Errorf("deleting it again: %d, want 404", got)
	}
}

// A move steps a pattern up or down its own category and answers the list in its
// new order; at the end already, nothing moves.
func TestPatternsMove(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	createPattern(t, env, "A", testPattern5x5())
	b := createPattern(t, env, "B", testPattern5x5Alt())
	c := createPattern(t, env, "C", diagonal())

	move := func(id int64, direction string) string {
		t.Helper()
		var list struct {
			Patterns []map[string]any `json:"patterns"`
		}
		decodeInto(t, env.postJSON(t, fmt.Sprintf("/api/v1/patterns/%d/move", id), map[string]any{"direction": direction}), http.StatusOK, &list)
		return names(list.Patterns)
	}
	if got := move(b, "up"); got != "B,A,C" {
		t.Errorf("B up: %s", got)
	}
	if got := move(b, "up"); got != "B,A,C" {
		t.Errorf("B up from the top: %s", got)
	}
	if got := move(c, "down"); got != "B,A,C" {
		t.Errorf("C down from the bottom: %s", got)
	}
	if got := move(b, "down"); got != "A,B,C" {
		t.Errorf("B down: %s", got)
	}

	if at := invalid(t, env.postJSON(t, fmt.Sprintf("/api/v1/patterns/%d/move", b), map[string]any{"direction": "sideways"})); !slices.Contains(at, "body.direction") {
		t.Errorf("a direction that isn't one: errors at %v", at)
	}
	if got := status(env.postJSON(t, "/api/v1/patterns/999/move", map[string]any{"direction": "up"})); got != http.StatusNotFound {
		t.Errorf("an unknown pattern: %d, want 404", got)
	}
}

// A reorder puts a category's patterns in the order given, taking in one from
// another category, and answers the list; an id that isn't there is a 422.
func TestPatternsReorder(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	a := createPattern(t, env, "A", testPattern5x5())
	b := createPattern(t, env, "B", testPattern5x5Alt())
	var bonus map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/pattern-categories", map[string]any{"name": "Bonus"}), http.StatusCreated, &bonus)
	bonusID := bonus["id"].(float64)

	var list struct {
		Patterns []map[string]any `json:"patterns"`
	}
	decodeInto(t, env.postJSON(t, "/api/v1/patterns/reorder", map[string]any{
		"categoryId": standardCategory, "orderedIds": []int64{b, a},
	}), http.StatusOK, &list)
	if names(list.Patterns) != "B,A" {
		t.Errorf("reversed: %s", names(list.Patterns))
	}
	decodeInto(t, env.postJSON(t, "/api/v1/patterns/reorder", map[string]any{
		"categoryId": bonusID, "orderedIds": []int64{a},
	}), http.StatusOK, &list)
	if names(list.Patterns) != "B,A" || list.Patterns[1]["categoryId"] != bonusID {
		t.Errorf("A dragged into Bonus: %v", list.Patterns)
	}

	for _, c := range []struct {
		body map[string]any
		want string
	}{
		{map[string]any{"categoryId": 999, "orderedIds": []int64{a}}, "body.categoryId"},
		{map[string]any{"categoryId": standardCategory, "orderedIds": []int64{b, 999}}, "body.orderedIds[1]"},
		{map[string]any{"categoryId": standardCategory, "orderedIds": []int64{}}, "body.orderedIds"},
		{map[string]any{"categoryId": standardCategory, "orderedIds": []int64{b, b}}, "body.orderedIds"},
	} {
		if at := invalid(t, env.postJSON(t, "/api/v1/patterns/reorder", c.body)); !slices.Contains(at, c.want) {
			t.Errorf("%v: errors at %v, want %s", c.body, at, c.want)
		}
	}
	// The refused reorder left B where it was.
	if patterns, _ := patternList(t, env); patterns[0]["categoryId"] != float64(standardCategory) {
		t.Errorf("after a refused reorder: %v", patterns)
	}
}

// A category is made last, renamed, moved, put in order, and deleted, its patterns
// going to the end of the first remaining category; the last one stays (409).
func TestPatternCategoriesLifecycle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var bonus map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/pattern-categories", map[string]any{"name": " Bonus "}), http.StatusCreated, &bonus)
	if bonus["name"] != "Bonus" || bonus["sortOrder"] != float64(1) {
		t.Errorf("made: %v", bonus)
	}
	bonusID := int64(bonus["id"].(float64))
	if at := invalid(t, env.postJSON(t, "/api/v1/pattern-categories", map[string]any{"name": "  "})); !slices.Contains(at, "body.name") {
		t.Errorf("a blank name: errors at %v", at)
	}

	var renamed map[string]any
	decodeInto(t, env.patchJSON(t, fmt.Sprintf("/api/v1/pattern-categories/%d", bonusID), map[string]any{"name": "Specials"}), http.StatusOK, &renamed)
	if renamed["name"] != "Specials" || renamed["id"] != bonus["id"] {
		t.Errorf("renamed: %v", renamed)
	}
	if at := invalid(t, env.patchJSON(t, fmt.Sprintf("/api/v1/pattern-categories/%d", bonusID), map[string]any{"name": " "})); !slices.Contains(at, "body.name") {
		t.Errorf("a blank rename: errors at %v", at)
	}
	if got := status(env.patchJSON(t, "/api/v1/pattern-categories/999", map[string]any{"name": "X"})); got != http.StatusNotFound {
		t.Errorf("renaming an unknown category: %d, want 404", got)
	}

	var list struct {
		Categories []map[string]any `json:"categories"`
	}
	decodeInto(t, env.postJSON(t, fmt.Sprintf("/api/v1/pattern-categories/%d/move", bonusID), map[string]any{"direction": "up"}), http.StatusOK, &list)
	if names(list.Categories) != "Specials,Standard" {
		t.Errorf("Specials up: %s", names(list.Categories))
	}
	if got := status(env.postJSON(t, "/api/v1/pattern-categories/999/move", map[string]any{"direction": "up"})); got != http.StatusNotFound {
		t.Errorf("moving an unknown category: %d, want 404", got)
	}
	decodeInto(t, env.postJSON(t, "/api/v1/pattern-categories/reorder", map[string]any{
		"orderedIds": []int64{standardCategory, bonusID},
	}), http.StatusOK, &list)
	if names(list.Categories) != "Standard,Specials" {
		t.Errorf("put back in order: %s", names(list.Categories))
	}
	if at := invalid(t, env.postJSON(t, "/api/v1/pattern-categories/reorder", map[string]any{
		"orderedIds": []int64{bonusID, 999},
	})); !slices.Contains(at, "body.orderedIds[1]") {
		t.Errorf("an unknown category in the order: errors at %v", at)
	}

	// Specials' patterns go to the end of Standard, keeping their order.
	createPattern(t, env, "Kept", testPattern5x5())
	for _, name := range []string{"First", "Second"} {
		id := env.seedPattern(t, name)
		decodeInto(t, env.patchJSON(t, fmt.Sprintf("/api/v1/patterns/%d", id), map[string]any{"categoryId": bonusID}), http.StatusOK, nil)
	}
	if got := status(env.del(t, fmt.Sprintf("/api/v1/pattern-categories/%d", bonusID))); got != http.StatusNoContent {
		t.Fatalf("delete: %d, want 204", got)
	}
	patterns, categories := patternList(t, env)
	if names(categories) != "Standard" || names(patterns) != "Kept,First,Second" || patterns[2]["categoryName"] != "Standard" {
		t.Errorf("after the delete: %s / %s", names(categories), names(patterns))
	}
	if got := status(env.del(t, fmt.Sprintf("/api/v1/pattern-categories/%d", bonusID))); got != http.StatusNotFound {
		t.Errorf("deleting it again: %d, want 404", got)
	}
	_, got, detail := problem(t, env.del(t, fmt.Sprintf("/api/v1/pattern-categories/%d", standardCategory)))
	if got != http.StatusConflict || detail == "" {
		t.Errorf("deleting the last category: %d %q, want 409", got, detail)
	}
}

// A pattern write reaches the staff with the Patterns page as patterns_changed, and
// never a player's board nor an account without the page; the patterns read again
// have the change.
func TestPatternWritesReachOnlyStaff(t *testing.T) {
	env := newTestEnv(t)
	gameOnly := makeActiveUser(t, env, "gamestaff", "correct horse battery", []string{"bingo-game"})
	if err := env.store.SaveCard(t.Context(), "PATT01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	player := boardStream(t, env, "PATT01")
	admin := staffStream(t, env, env.client)
	without := staffStream(t, env, gameOnly)

	createPattern(t, env, "Live", testPattern5x5())

	if e := admin.expect("patterns_changed", 3*time.Second); len(e.Payload) != 0 {
		t.Errorf("patterns_changed: %+v", e)
	}
	var list struct {
		Patterns   []map[string]any `json:"patterns"`
		Categories []map[string]any `json:"categories"`
	}
	decodeInto(t, env.get(t, "/api/v1/patterns"), http.StatusOK, &list)
	if names(list.Patterns) != "Live" || list.Patterns[0]["categoryName"] != "Standard" {
		t.Errorf("the patterns read again: %+v", list)
	}
	player.expectNone("patterns_changed", 500*time.Millisecond)
	without.expectNone("patterns_changed", 100*time.Millisecond)
}
