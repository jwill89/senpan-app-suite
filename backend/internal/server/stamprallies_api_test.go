package server_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// placement is a stamp's or prize's box on the card.
func placement(x, y, w, h float64) map[string]any {
	return map[string]any{"x": x, "y": y, "width": w, "height": h, "rotation": 0}
}

// stampBody is a stamp as a save sends it; over replaces fields of it.
func stampBody(password string, over map[string]any) map[string]any {
	body := map[string]any{
		"id": 0, "image": "images/stamp_stamps/a.png", "password": password, "stampType": "food",
		"placement": placement(10, 10, 15, 15), "paused": false,
	}
	for k, v := range over {
		body[k] = v
	}
	return body
}

// rallyBody is a full rally as a save sends it, with one stamp (password "alpha") and one
// prize; over replaces fields of it.
func rallyBody(title string, over map[string]any) map[string]any {
	body := map[string]any{
		"title": title, "cardImage": "", "notStampedImage": "", "redeemImage": "", "details": "",
		"redeemInstructions": "", "publicSignup": false, "completionMode": "all", "requiredFood": 0, "requiredGame": 0,
		"stamps": []map[string]any{stampBody("alpha", nil)},
		"prizes": []map[string]any{
			{"name": "Trophy", "image": "images/stamp_prizes/t.png", "placement": placement(70, 20, 20, 20)},
		},
	}
	for k, v := range over {
		body[k] = v
	}
	return body
}

func rallyPath(id int, tail string) string {
	return fmt.Sprintf("/api/v1/stamp-rallies/%d", id) + tail
}

// createRally makes a rally (201, the rally itself) and answers its id.
func (e *testEnv) createRally(t *testing.T, title string, over map[string]any) int {
	t.Helper()
	var made map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/stamp-rallies", rallyBody(title, over)), http.StatusCreated, &made)
	if made["status"] != "open" {
		t.Errorf("a new rally is %v, want open", made["status"])
	}
	return int(made["id"].(float64))
}

// createSignupRally makes a rally that takes public sign-ups.
func (e *testEnv) createSignupRally(t *testing.T, title string) int {
	t.Helper()
	return e.createRally(t, title, map[string]any{"publicSignup": true})
}

// rallyDetail reads a rally's staff detail: the rally and its cards.
func (e *testEnv) rallyDetail(t *testing.T, id int) (rally map[string]any, cards []any) {
	t.Helper()
	var detail map[string]any
	decodeInto(t, e.get(t, rallyPath(id, "")), http.StatusOK, &detail)
	return detail["stampRally"].(map[string]any), detail["cards"].([]any)
}

// issueCard has staff issue a card (201) and answers its token.
func (e *testEnv) issueCard(t *testing.T, rallyID int, name string) string {
	t.Helper()
	var card map[string]any
	decodeInto(t, e.postJSON(t, rallyPath(rallyID, "/cards"), map[string]any{"participantName": name}), http.StatusCreated, &card)
	return card["token"].(string)
}

// setRallyStatus closes or reopens a rally (status is "closed" or "open").
func (e *testEnv) setRallyStatus(t *testing.T, rallyID int, status string) {
	t.Helper()
	verb := "reopen"
	if status == "closed" {
		verb = "close"
	}
	var out map[string]any
	decodeInto(t, e.postJSON(t, rallyPath(rallyID, "/"+verb), nil), http.StatusOK, &out)
	if out["status"] != status {
		t.Errorf("%s answered a %v rally", verb, out["status"])
	}
}

func cardPath(token, tail string) string { return "/api/v1/stamp-cards/" + token + tail }

// collect enters a password on a card as the anonymous client and answers the response.
func (e *testEnv) collect(t *testing.T, c interface {
	Do(*http.Request) (*http.Response, error)
}, token, password string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.url(cardPath(token, "/stamp")),
		strings.NewReader(fmt.Sprintf(`{"password":%q}`, password)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// signUp posts a public sign-up as the client and answers the response.
func (e *testEnv) signUp(t *testing.T, c *http.Client, rallyID int, name, world string) *http.Response {
	t.Helper()
	return postAs(t, c, e, rallyPath(rallyID, "/sign-up"), map[string]any{"characterName": name, "world": world})
}

// lookup posts a public card lookup as the client and answers its entries.
func (e *testEnv) lookup(t *testing.T, c *http.Client, name, world string) []any {
	t.Helper()
	var out struct {
		Entries []any `json:"entries"`
	}
	decodeInto(t, postAs(t, c, e, "/api/v1/stamp-cards/lookup", map[string]any{"characterName": name, "world": world}), http.StatusOK, &out)
	return out.Entries
}

// Every staff operation needs the Stamp Rally page: 401 signed out, 403 without it, and
// its grantee gets through. The public ones need nothing.
func TestStampRalliesGuard(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "Rally", nil)
	env.issueCard(t, id, "Tataru")
	_, cards := env.rallyDetail(t, id)
	cardID := int(cards[0].(map[string]any)["id"].(float64))
	env.signOut(t)

	staff := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/stamp-rallies"},
		{http.MethodPost, "/api/v1/stamp-rallies"},
		{http.MethodGet, rallyPath(id, "")},
		{http.MethodPut, rallyPath(id, "")},
		{http.MethodDelete, rallyPath(id, "")},
		{http.MethodPost, rallyPath(id, "/close")},
		{http.MethodPost, rallyPath(id, "/reopen")},
		{http.MethodGet, rallyPath(id, "/logs")},
		{http.MethodPatch, rallyPath(id, "/stamps/1")},
		{http.MethodPost, rallyPath(id, "/cards")},
		{http.MethodDelete, rallyPath(id, fmt.Sprintf("/cards/%d", cardID))},
	}
	for _, s := range staff {
		if got := status(reqAs(t, env.client, env, s.method, s.path, map[string]any{})); got != http.StatusUnauthorized {
			t.Errorf("%s %s signed out: %d, want 401", s.method, s.path, got)
		}
	}

	other := makeActiveUser(t, env, "cardsonly", "correct horse battery", []string{"bingo-cards"})
	for _, s := range staff {
		if got := status(reqAs(t, other, env, s.method, s.path, map[string]any{})); got != http.StatusForbidden {
			t.Errorf("%s %s without the page: %d, want 403", s.method, s.path, got)
		}
	}

	grantee := makeActiveUser(t, env, "rallystaff", "correct horse battery", []string{"festival-stamp-rally"})
	if got := status(getAs(t, grantee, env, "/api/v1/stamp-rallies")); got != http.StatusOK {
		t.Errorf("the page's grantee: %d, want 200", got)
	}

	anon := newClient(t, env)
	if got := status(getAs(t, anon, env, "/api/v1/stamp-rallies/public")); got != http.StatusOK {
		t.Errorf("the public list: %d, want 200", got)
	}
}

func TestStampRallyCreateListDetail(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "Summer Rally", nil)

	var list struct {
		StampRallies []map[string]any `json:"stampRallies"`
	}
	decodeInto(t, env.get(t, "/api/v1/stamp-rallies"), http.StatusOK, &list)
	if len(list.StampRallies) != 1 {
		t.Fatalf("%d rallies listed, want 1", len(list.StampRallies))
	}
	row := list.StampRallies[0]
	if len(row["stamps"].([]any)) != 0 || len(row["prizes"].([]any)) != 0 {
		t.Errorf("the list carries stamps or prizes: %v", row)
	}
	if row["stampCount"].(float64) != 1 || row["activeStampCount"].(float64) != 1 {
		t.Errorf("counts: %v stamps, %v active", row["stampCount"], row["activeStampCount"])
	}

	rally, cards := env.rallyDetail(t, id)
	if len(rally["stamps"].([]any)) != 1 || len(rally["prizes"].([]any)) != 1 || len(cards) != 0 {
		t.Errorf("detail: %v / %d cards", rally, len(cards))
	}
	stamp := rally["stamps"].([]any)[0].(map[string]any)
	if stamp["id"].(float64) == 0 || stamp["password"] != "alpha" || stamp["stampType"] != "food" {
		t.Errorf("the stamp: %v", stamp)
	}
}

// A save is refused for what the schema and the rules can't take, each at its field.
func TestStampRallySaveValidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	noStamps := rallyBody("T", nil)
	delete(noStamps, "stamps")

	cases := map[string]struct {
		body map[string]any
		want string
	}{
		"blank title":       {rallyBody("   ", nil), "body.title"},
		"not a time":        {rallyBody("T", map[string]any{"availableFrom": "tomorrow-ish"}), "body.availableFrom"},
		"a stamp's window":  {rallyBody("T", map[string]any{"stamps": []map[string]any{stampBody("a", map[string]any{"activeTo": "later"})}}), "body.stamps[0].activeTo"},
		"a stamp type":      {rallyBody("T", map[string]any{"stamps": []map[string]any{stampBody("a", map[string]any{"stampType": "drink"})}}), "body.stamps[0].stampType"},
		"counts need some":  {rallyBody("T", map[string]any{"completionMode": "counts"}), "body.requiredFood"},
		"no such festival":  {rallyBody("T", map[string]any{"festivalMapId": 999999}), "body.festivalMapId"},
		"stamps are needed": {noStamps, "body"},
	}
	for name, c := range cases {
		if at := invalid(t, env.postJSON(t, "/api/v1/stamp-rallies", c.body)); !slices.Contains(at, c.want) {
			t.Errorf("%s: errors at %v, want %s", name, at, c.want)
		}
	}

	// The same rule holds for an edit: an existing rally can't be turned into one.
	id := env.createRally(t, "Real Rally", nil)
	if at := invalid(t, env.putJSON(t, rallyPath(id, ""), rallyBody("Real Rally", map[string]any{"completionMode": "counts"}))); !slices.Contains(at, "body.requiredFood") {
		t.Errorf("an edit to counts with none required: errors at %v", at)
	}
}

// A requirement can't be more than the stamps of its type the card carries.
func TestStampRallyRequiredCountsAreClamped(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "Impossible", map[string]any{"completionMode": "counts", "requiredFood": 9, "requiredGame": 0})
	rally, _ := env.rallyDetail(t, id)
	if rally["requiredFood"].(float64) != 1 || rally["requiredGame"].(float64) != 0 {
		t.Errorf("required %v/%v, want 1/0 (kept to the stamps that exist)", rally["requiredFood"], rally["requiredGame"])
	}
}

// A save replaces the stamps and prizes it carries: a stamp with its id keeps what was
// collected on it, id 0 adds one, one left out is deleted, and an empty list clears them.
func TestStampRallyUpdateReconcilesStamps(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "Rally", map[string]any{"stamps": []map[string]any{stampBody("alpha", nil), stampBody("bravo", nil)}})
	rally, _ := env.rallyDetail(t, id)
	stamps := rally["stamps"].([]any)
	keepID := int(stamps[0].(map[string]any)["id"].(float64))
	token := env.issueCard(t, id, "Tataru")
	if got := status(env.collect(t, newClient(t, env), token, "alpha")); got != http.StatusOK {
		t.Fatalf("collect: %d", got)
	}

	var saved map[string]any
	decodeInto(t, env.putJSON(t, rallyPath(id, ""), rallyBody("Renamed", map[string]any{"stamps": []map[string]any{
		stampBody("alpha-2", map[string]any{"id": keepID}), stampBody("charlie", nil),
	}})), http.StatusOK, &saved)
	got := saved["stamps"].([]any)
	if len(got) != 2 || int(got[0].(map[string]any)["id"].(float64)) != keepID || got[0].(map[string]any)["password"] != "alpha-2" {
		t.Errorf("after the save: %v", got)
	}
	if saved["title"] != "Renamed" {
		t.Errorf("title %v", saved["title"])
	}
	_, cards := env.rallyDetail(t, id)
	if cards[0].(map[string]any)["collectedCount"].(float64) != 1 {
		t.Error("the stamp that kept its id lost what was collected on it")
	}

	// A stamp id from nowhere is refused, and nothing is saved.
	if at := invalid(t, env.putJSON(t, rallyPath(id, ""), rallyBody("X", map[string]any{"stamps": []map[string]any{stampBody("z", map[string]any{"id": 987654})}}))); !slices.Contains(at, "body.stamps[0].id") {
		t.Errorf("a foreign stamp id: errors at %v", at)
	}
	if after, _ := env.rallyDetail(t, id); after["title"] != "Renamed" {
		t.Error("a refused save changed the rally")
	}

	var emptied map[string]any
	decodeInto(t, env.putJSON(t, rallyPath(id, ""), rallyBody("Renamed", map[string]any{
		"stamps": []map[string]any{}, "prizes": []map[string]any{},
	})), http.StatusOK, &emptied)
	if len(emptied["stamps"].([]any)) != 0 || len(emptied["prizes"].([]any)) != 0 {
		t.Errorf("an empty save left %v", emptied)
	}
}

func TestStampRallyUnknownIDs(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, rallyPath(9999, "")},
		{http.MethodPut, rallyPath(9999, "")},
		{http.MethodDelete, rallyPath(9999, "")},
		{http.MethodPost, rallyPath(9999, "/close")},
		{http.MethodPost, rallyPath(9999, "/reopen")},
		{http.MethodGet, rallyPath(9999, "/logs")},
		{http.MethodPatch, rallyPath(9999, "/stamps/1")},
		{http.MethodPost, rallyPath(9999, "/cards")},
		{http.MethodDelete, rallyPath(9999, "/cards/1")},
	} {
		var body any
		switch c.method {
		case http.MethodPut:
			body = rallyBody("T", nil)
		case http.MethodPatch:
			body = map[string]any{"paused": true}
		case http.MethodPost:
			body = map[string]any{"participantName": "Tataru"}
		}
		if got := status(env.reqJSON(t, c.method, c.path, body)); got != http.StatusNotFound {
			t.Errorf("%s %s: %d, want 404", c.method, c.path, got)
		}
	}
}

// A stamp is paused and resumed on its own, and only through its own rally.
func TestStampRallyPauseAStamp(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "Rally", nil)
	other := env.createRally(t, "Other", nil)
	rally, _ := env.rallyDetail(t, id)
	stampID := int(rally["stamps"].([]any)[0].(map[string]any)["id"].(float64))

	var stamp map[string]any
	decodeInto(t, env.patchJSON(t, rallyPath(id, fmt.Sprintf("/stamps/%d", stampID)), map[string]any{"paused": true}), http.StatusOK, &stamp)
	if stamp["paused"] != true {
		t.Errorf("the stamp: %v", stamp)
	}
	if got := status(env.patchJSON(t, rallyPath(other, fmt.Sprintf("/stamps/%d", stampID)), map[string]any{"paused": false})); got != http.StatusNotFound {
		t.Errorf("another rally's stamp: %d, want 404", got)
	}
}

// -- The public card -----------------------------------------------------------

func TestStampCardPublicFlowAndCompletion(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "Rally", nil)
	token := env.issueCard(t, id, "Tataru")
	anon := newClient(t, env)

	// No password leaks; one stamp is not collected; the prize is hidden.
	var card map[string]any
	decodeInto(t, getAs(t, anon, env, cardPath(token, "")), http.StatusOK, &card)
	stamps := card["stamps"].([]any)
	if len(stamps) != 1 {
		t.Fatalf("%d public stamps, want 1", len(stamps))
	}
	if _, leaked := stamps[0].(map[string]any)["password"]; leaked {
		t.Error("a public stamp leaked its password")
	}
	if card["prizesRevealed"] != false || card["prizes"].([]any)[0].(map[string]any)["name"] != "" {
		t.Errorf("prizes shown before completion: %v", card["prizes"])
	}

	// A password matching nothing is a 422 at the field.
	resp := env.collect(t, anon, token, "nope")
	if at := invalid(t, resp); !slices.Contains(at, "body.password") {
		t.Errorf("a wrong password: errors at %v", at)
	}

	// The only stamp completes the card and reveals the prize.
	var result struct {
		Card             map[string]any `json:"card"`
		CollectedStampID float64        `json:"collectedStampId"`
	}
	decodeInto(t, env.collect(t, anon, token, "alpha"), http.StatusOK, &result)
	if result.Card["completed"] != true || result.Card["prizesRevealed"] != true || result.CollectedStampID == 0 {
		t.Errorf("after the last stamp: %v", result)
	}
	if prize := result.Card["prizes"].([]any)[0].(map[string]any); prize["name"] != "Trophy" {
		t.Errorf("the revealed prize: %v", prize)
	}
	if result.Card["completedAt"] == nil {
		t.Error("a complete card has no completedAt")
	}

	// A stamp already collected is a 409.
	if got := status(env.collect(t, anon, token, "alpha")); got != http.StatusConflict {
		t.Errorf("collect again: %d, want 409", got)
	}

	// The log shows the collection with the participant and the default stall.
	var logs struct {
		Logs []map[string]any `json:"logs"`
	}
	decodeInto(t, env.get(t, rallyPath(id, "/logs")), http.StatusOK, &logs)
	if len(logs.Logs) != 1 || logs.Logs[0]["participantName"] != "Tataru" || logs.Logs[0]["stallName"] != "Senpan Tea House" {
		t.Errorf("logs: %v", logs.Logs)
	}
}

// A counts rally completes on the required number of each type, whatever else is left:
// two food stamps don't finish a card that needs a game stamp too.
func TestStampCardTypedCompletion(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "Festival", map[string]any{
		"completionMode": "counts", "requiredFood": 1, "requiredGame": 1,
		"stamps": []map[string]any{
			stampBody("food1", nil), stampBody("food2", nil),
			stampBody("game1", map[string]any{"stampType": "game"}), stampBody("game2", map[string]any{"stampType": "game"}),
		},
	})
	token := env.issueCard(t, id, "Tataru")
	anon := newClient(t, env)

	var card map[string]any
	decodeInto(t, getAs(t, anon, env, cardPath(token, "")), http.StatusOK, &card)
	rally := card["rally"].(map[string]any)
	if rally["completionMode"] != "counts" || rally["requiredFood"].(float64) != 1 || rally["requiredGame"].(float64) != 1 {
		t.Errorf("the public rule: %v", rally)
	}

	collect := func(password string) map[string]any {
		t.Helper()
		var r struct {
			Card map[string]any `json:"card"`
		}
		decodeInto(t, env.collect(t, anon, token, password), http.StatusOK, &r)
		return r.Card
	}
	if collect("food1")["completed"] != false || collect("food2")["completed"] != false {
		t.Error("the card completed on food stamps alone; the game requirement was ignored")
	}
	if got := collect("game1"); got["completed"] != true {
		t.Errorf("after one of each: %v", got["completed"])
	}
	var logs struct {
		Logs []map[string]any `json:"logs"`
	}
	decodeInto(t, env.get(t, rallyPath(id, "/logs")), http.StatusOK, &logs)
	if len(logs.Logs) != 3 || logs.Logs[2]["stampType"] != "game" {
		t.Errorf("the log types: %v", logs.Logs)
	}
}

// A stamp that is paused, or whose rally is closed, can't be collected.
func TestStampCardClosedStall(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "Paused Rally", map[string]any{"stamps": []map[string]any{stampBody("alpha", map[string]any{"paused": true})}})
	token := env.issueCard(t, id, "Borin")
	anon := newClient(t, env)

	resp := env.collect(t, anon, token, "alpha")
	if _, code, detail := problem(t, resp); code != http.StatusConflict || detail != "This stall is currently closed and cannot be stamped." {
		t.Errorf("a paused stamp: %d %q", code, detail)
	}

	rally, _ := env.rallyDetail(t, id)
	stampID := int(rally["stamps"].([]any)[0].(map[string]any)["id"].(float64))
	env.patchJSON(t, rallyPath(id, fmt.Sprintf("/stamps/%d", stampID)), map[string]any{"paused": false}).Body.Close()
	env.setRallyStatus(t, id, "closed")
	if got := status(env.collect(t, anon, token, "alpha")); got != http.StatusConflict {
		t.Errorf("a closed rally: %d, want 409", got)
	}
}

func TestStampCardUnknownToken(t *testing.T) {
	env := newTestEnv(t)
	anon := newClient(t, env)
	if got := status(getAs(t, anon, env, cardPath("deadbeef", ""))); got != http.StatusNotFound {
		t.Errorf("view: %d, want 404", got)
	}
	if got := status(env.collect(t, anon, "deadbeef", "alpha")); got != http.StatusNotFound {
		t.Errorf("stamp: %d, want 404", got)
	}
}

// A blank password is refused by the schema before any guess is counted.
func TestStampCardBlankPassword(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	token := env.issueCard(t, env.createRally(t, "R", nil), "Tataru")
	if at := invalid(t, env.collect(t, newClient(t, env), token, "   ")); !slices.Contains(at, "body.password") {
		t.Errorf("a blank password: errors at %v", at)
	}
}

// Only a wrong password costs budget: ten of them from one address run it out, then even
// the right one is held back, with a 429 saying when to try again.
func TestStampCardGuessesAreLimited(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	token := env.issueCard(t, env.createRally(t, "R", nil), "Tataru")
	anon := newClient(t, env)

	for i := range 10 {
		if got := status(env.collect(t, anon, token, fmt.Sprintf("wrong-%d", i))); got != http.StatusUnprocessableEntity {
			t.Fatalf("guess %d: %d, want 422", i, got)
		}
	}
	resp := env.collect(t, anon, token, "alpha")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Errorf("after ten wrong guesses: %d, Retry-After %q, want a 429 saying when", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
}

// A right password costs nothing and clears the misses, so a participant collecting
// normally never meets the limit.
func TestStampCardRightPasswordsDoNotCount(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	stamps := []map[string]any{}
	for i := range 12 {
		stamps = append(stamps, stampBody(fmt.Sprintf("word%d", i), nil))
	}
	token := env.issueCard(t, env.createRally(t, "R", map[string]any{"stamps": stamps}), "Tataru")
	anon := newClient(t, env)
	for i := range 12 {
		if got := status(env.collect(t, anon, token, fmt.Sprintf("word%d", i))); got != http.StatusOK {
			t.Fatalf("collect %d: %d, want 200", i, got)
		}
	}
}

// -- Cards and the log ---------------------------------------------------------

// A closed rally is read-only. A card with stamps can only be deleted once it is closed,
// and deleting it keeps the log, which holds its own copy of the names.
func TestStampRallyCloseKeepsLogs(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "Festival", nil)
	token := env.issueCard(t, id, "Tester")
	if got := status(env.collect(t, newClient(t, env), token, "alpha")); got != http.StatusOK {
		t.Fatalf("collect: %d", got)
	}
	_, cards := env.rallyDetail(t, id)
	cardID := int(cards[0].(map[string]any)["id"].(float64))
	cardURL := rallyPath(id, fmt.Sprintf("/cards/%d", cardID))

	if got := status(env.del(t, cardURL)); got != http.StatusConflict {
		t.Errorf("delete a stamped card (open): %d, want 409", got)
	}
	env.setRallyStatus(t, id, "closed")
	if got := status(env.del(t, cardURL)); got != http.StatusNoContent {
		t.Fatalf("delete a stamped card (closed): %d, want 204", got)
	}
	var logs struct {
		Logs []map[string]any `json:"logs"`
	}
	decodeInto(t, env.get(t, rallyPath(id, "/logs")), http.StatusOK, &logs)
	if len(logs.Logs) != 1 || logs.Logs[0]["participantName"] != "Tester" || logs.Logs[0]["cardId"] != nil {
		t.Errorf("the kept log: %v", logs.Logs)
	}
}

func TestStampRallyCardIssueAndDelete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "R", nil)
	other := env.createRally(t, "Other", nil)

	var card map[string]any
	decodeInto(t, env.postJSON(t, rallyPath(id, "/cards"), map[string]any{"participantName": "Aria Ashwood @ Gilgamesh"}), http.StatusCreated, &card)
	if card["participantName"] != "Aria Ashwood" || card["world"] != "Gilgamesh" || card["token"] == "" {
		t.Errorf("the issued card: %v", card)
	}
	cardID := int(card["id"].(float64))

	// The card is not found under another rally, and an unknown card is a 404 too.
	if got := status(env.del(t, rallyPath(other, fmt.Sprintf("/cards/%d", cardID)))); got != http.StatusNotFound {
		t.Errorf("under another rally: %d, want 404", got)
	}
	if got := status(env.del(t, rallyPath(id, "/cards/99999"))); got != http.StatusNotFound {
		t.Errorf("an unknown card: %d, want 404", got)
	}
	if got := status(env.del(t, rallyPath(id, fmt.Sprintf("/cards/%d", cardID)))); got != http.StatusNoContent {
		t.Errorf("an unstamped card: %d, want 204", got)
	}
	if at := invalid(t, env.postJSON(t, rallyPath(id, "/cards"), map[string]any{"participantName": "  "})); !slices.Contains(at, "body.participantName") {
		t.Errorf("a blank name: errors at %v", at)
	}
}

// -- Public sign-up and lookup -------------------------------------------------

func TestStampSignupListsOnlyOptedInRallies(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	optedIn := env.createSignupRally(t, "Festival Rally")
	env.createRally(t, "Staff-issued Rally", nil)

	var out struct {
		Rallies []map[string]any `json:"rallies"`
	}
	decodeInto(t, getAs(t, newClient(t, env), env, "/api/v1/stamp-rallies/public"), http.StatusOK, &out)
	if len(out.Rallies) != 1 || int(out.Rallies[0]["id"].(float64)) != optedIn {
		t.Errorf("listed %v, want only the opted-in rally", out.Rallies)
	}
}

func TestStampSignupIssuesACard(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")
	anon := newClient(t, env)

	var issued map[string]any
	decodeInto(t, env.signUp(t, anon, rallyID, "Yao Ming", "Balmung"), http.StatusCreated, &issued)
	token, _ := issued["cardToken"].(string)
	if token == "" || issued["participantName"] != "Yao Ming" || issued["world"] != "Balmung" || issued["rallyTitle"] != "Festival Rally" {
		t.Fatalf("the sign-up: %v", issued)
	}
	// No linked garapon: no drawing link.
	if issued["garaponToken"] != nil || issued["garaponTitle"] != nil {
		t.Errorf("a drawing link for a rally with no garapon: %v", issued)
	}

	// The card reads as it always did: the label a participant holds up at a stall.
	var card map[string]any
	decodeInto(t, getAs(t, anon, env, cardPath(token, "")), http.StatusOK, &card)
	if card["participantName"] != "Yao Ming @ Balmung" {
		t.Errorf("the card's participant %v, want the composed label", card["participantName"])
	}
}

func TestStampSignupRejectsADuplicate(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")
	anon := newClient(t, env)

	if got := status(env.signUp(t, anon, rallyID, "Yao Ming", "Balmung")); got != http.StatusCreated {
		t.Fatalf("first sign-up: %d", got)
	}
	// Case and space don't open a loophole; the world tells two players apart.
	if got := status(env.signUp(t, anon, rallyID, "  YAO MING  ", "balmung")); got != http.StatusConflict {
		t.Errorf("the same person again: %d, want 409", got)
	}
	if got := status(env.signUp(t, anon, rallyID, "Yao Ming", "Gilgamesh")); got != http.StatusCreated {
		t.Errorf("the same name on another world: %d, want 201", got)
	}
	other := env.createSignupRally(t, "Second Rally")
	if got := status(env.signUp(t, anon, other, "Yao Ming", "Balmung")); got != http.StatusCreated {
		t.Errorf("the same person on another rally: %d, want 201", got)
	}
}

func TestStampSignupRefusals(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	anon := newClient(t, env)

	// A rally that never opted in looks like one that doesn't exist.
	optedOut := env.createRally(t, "Staff-issued", nil)
	if got := status(env.signUp(t, anon, optedOut, "Yao Ming", "Balmung")); got != http.StatusNotFound {
		t.Errorf("an opted-out rally: %d, want 404", got)
	}
	if got := status(env.signUp(t, anon, 999999, "Yao Ming", "Balmung")); got != http.StatusNotFound {
		t.Errorf("a missing rally: %d, want 404", got)
	}

	rallyID := env.createSignupRally(t, "Festival Rally")
	long := strings.Repeat("x", 61)
	for name, c := range map[string]struct{ name, world, at string }{
		"a blank name":  {"   ", "Balmung", "body.characterName"},
		"a long name":   {long, "Balmung", "body.characterName"},
		"a blank world": {"Yao Ming", "   ", "body.world"},
		"a long world":  {"Yao Ming", long, "body.world"},
	} {
		if at := invalid(t, env.signUp(t, anon, rallyID, c.name, c.world)); !slices.Contains(at, c.at) {
			t.Errorf("%s: errors at %v, want %s", name, at, c.at)
		}
	}

	// Closing it stops sign-ups (409) and drops it from the public list.
	env.setRallyStatus(t, rallyID, "closed")
	if got := status(env.signUp(t, anon, rallyID, "Yao Ming", "Balmung")); got != http.StatusConflict {
		t.Errorf("a closed rally: %d, want 409", got)
	}
	var out struct {
		Rallies []any `json:"rallies"`
	}
	decodeInto(t, getAs(t, anon, env, "/api/v1/stamp-rallies/public"), http.StatusOK, &out)
	if len(out.Rallies) != 0 {
		t.Errorf("a closed rally is still listed: %v", out.Rallies)
	}
}

// A rally not yet open, or already over, takes no sign-ups and isn't listed.
func TestStampSignupHonorsTheWindow(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	anon := newClient(t, env)
	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	past := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	notYet := env.createRally(t, "Not yet", map[string]any{"publicSignup": true, "availableFrom": future})
	over := env.createRally(t, "Over", map[string]any{"publicSignup": true, "availableTo": past})

	for _, id := range []int{notYet, over} {
		if got := status(env.signUp(t, anon, id, "Yao Ming", "Balmung")); got != http.StatusConflict {
			t.Errorf("rally %d outside its window: %d, want 409", id, got)
		}
	}
	var out struct {
		Rallies []any `json:"rallies"`
	}
	decodeInto(t, getAs(t, anon, env, "/api/v1/stamp-rallies/public"), http.StatusOK, &out)
	if len(out.Rallies) != 0 {
		t.Errorf("rallies outside their window are listed: %v", out.Rallies)
	}
}

// The bot check, when it is on, refuses a sign-up that brings no passing response.
func TestStampSignupNeedsTheBotCheck(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")
	env.srv.SetTurnstile("secret", "sitekey")

	resp := env.signUp(t, newClient(t, env), rallyID, "Yao Ming", "Balmung")
	if _, code, detail := problem(t, resp); code != http.StatusForbidden || detail != "Bot check failed. Please try again." {
		t.Errorf("without a response: %d %q, want 403", code, detail)
	}
	if _, cards := env.rallyDetail(t, rallyID); len(cards) != 0 {
		t.Error("a refused sign-up issued a card")
	}
}

// Fifteen tries an address may make in ten minutes, sign-ups and lookups together.
func TestStampSignupAndLookupShareALimit(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")
	anon := newClient(t, env)

	var limited *http.Response
	for i := range 20 {
		var resp *http.Response
		if i%2 == 0 {
			resp = env.signUp(t, anon, rallyID, fmt.Sprintf("Player %d", i), "Balmung")
		} else {
			resp = postAs(t, anon, env, "/api/v1/stamp-cards/lookup", map[string]any{"characterName": "Nobody", "world": "Balmung"})
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			limited = resp
			break
		}
		resp.Body.Close()
	}
	if limited == nil {
		t.Fatal("20 tries from one address were never limited")
	}
	defer limited.Body.Close()
	if limited.Header.Get("Retry-After") == "" {
		t.Error("the 429 doesn't say when to try again")
	}
}

// A rally with an open garapon issues a drawing link too, on a token apart from the
// card's, and uses the garapon's own default for the draws.
func TestStampSignupIssuesAGaraponLink(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")
	gid := env.createGarapon(t, "Festival Garapon", map[string]any{"stampRallyId": rallyID, "defaultDraws": 3})
	anon := newClient(t, env)

	var issued map[string]any
	decodeInto(t, env.signUp(t, anon, rallyID, "Yao Ming", "Balmung"), http.StatusCreated, &issued)
	cardToken, _ := issued["cardToken"].(string)
	drawToken, _ := issued["garaponToken"].(string)
	if drawToken == "" || issued["garaponTitle"] != "Festival Garapon" {
		t.Fatalf("the sign-up: %v", issued)
	}
	if drawToken == cardToken {
		t.Errorf("both tokens are %q: viewing a card and spending its draws must be separate secrets", cardToken)
	}
	if got := status(getAs(t, anon, env, "/api/v1/garapon-players/"+drawToken)); got != http.StatusOK {
		t.Errorf("the drawing link: %d, want 200", got)
	}
	if got := status(getAs(t, anon, env, "/api/v1/garapon-players/"+cardToken)); got != http.StatusNotFound {
		t.Errorf("a drawing link by the card token: %d, want 404 (a card link must not draw)", got)
	}
	if got := status(getAs(t, anon, env, cardPath(drawToken, ""))); got != http.StatusNotFound {
		t.Errorf("a card by the drawing token: %d, want 404", got)
	}

	players := env.garaponDetail(t, gid)["players"].([]any)
	if len(players) != 1 || players[0].(map[string]any)["maxDraws"].(float64) != 3 || players[0].(map[string]any)["stampCardToken"] != cardToken {
		t.Errorf("the garapon's links: %v, want one with 3 draws tied to the card", players)
	}
}

// The lookup is keyed on a name, which is public, so it gives nothing that can be spent:
// the card token (which opens the card and nothing else) and the draws left, never the
// drawing token.
func TestStampLookupNeverReturnsASpendableToken(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")
	env.createGarapon(t, "Festival Garapon", map[string]any{"stampRallyId": rallyID})
	anon := newClient(t, env)

	var issued map[string]any
	decodeInto(t, env.signUp(t, anon, rallyID, "Aria Ashwood", "Gilgamesh"), http.StatusCreated, &issued)
	drawToken := issued["garaponToken"].(string)

	entries := env.lookup(t, anon, "Aria Ashwood", "Gilgamesh")
	if len(entries) != 1 {
		t.Fatalf("%d entries, want 1", len(entries))
	}
	entry := entries[0].(map[string]any)
	for key, value := range entry {
		if str, ok := value.(string); ok && str == drawToken {
			t.Fatalf("the lookup returned the drawing token under %q", key)
		}
	}
	if entry["garaponTitle"] != "Festival Garapon" || entry["garaponDrawsLeft"].(float64) != 1 {
		t.Errorf("the entry: %v, want the garapon and 1 draw left", entry)
	}
	if got := status(postAs(t, anon, env, drawPath(entry["cardToken"].(string)), nil)); got != http.StatusNotFound {
		t.Errorf("a draw with the looked-up card token: %d, want 404", got)
	}
}

func TestStampLookupMatchesTheWholeName(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")
	anon := newClient(t, env)
	var issued map[string]any
	decodeInto(t, env.signUp(t, anon, rallyID, "Yao Ming", "Balmung"), http.StatusCreated, &issued)

	entries := env.lookup(t, anon, "  yao ming ", "BALMUNG")
	if len(entries) != 1 {
		t.Fatalf("an exact lookup found %d entries, want 1", len(entries))
	}
	e := entries[0].(map[string]any)
	if e["cardToken"] != issued["cardToken"] || e["rallyTitle"] != "Festival Rally" || e["garaponDrawsLeft"] != nil {
		t.Errorf("the entry: %v", e)
	}
	// A composed label with no world beside it is split.
	if got := env.lookup(t, anon, "Yao Ming @ Balmung", ""); len(got) != 1 {
		t.Errorf("a composed lookup found %d entries, want 1", len(got))
	}
	// A prefix or a fragment reveals nothing, so the lookup can't be walked.
	for _, probe := range [][2]string{{"Yao", "Balmung"}, {"ming", "Balmung"}, {"Yao Ming", "Bal"}, {"Balmung", "Yao Ming"}} {
		if got := env.lookup(t, anon, probe[0], probe[1]); len(got) != 0 {
			t.Errorf("lookup(%q, %q) found %d entries, want none", probe[0], probe[1], len(got))
		}
	}
}

// A miss is a 200 with nothing in it, never a 404 that tells a stranger from a participant
// with no card; a blank name is refused; a closed rally's cards are not handed back.
func TestStampLookupMissAndClosed(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")
	anon := newClient(t, env)

	if got := env.lookup(t, anon, "Nobody At All", "Balmung"); len(got) != 0 {
		t.Errorf("a miss found %v", got)
	}
	if at := invalid(t, postAs(t, anon, env, "/api/v1/stamp-cards/lookup", map[string]any{"characterName": "  ", "world": ""})); !slices.Contains(at, "body.characterName") {
		t.Errorf("a blank name: errors at %v", at)
	}

	env.signUp(t, anon, rallyID, "Yao Ming", "Balmung").Body.Close()
	if got := env.lookup(t, anon, "Yao Ming", "Balmung"); len(got) != 1 {
		t.Fatalf("before the close: %d entries, want 1", len(got))
	}
	env.setRallyStatus(t, rallyID, "closed")
	if got := env.lookup(t, anon, "Yao Ming", "Balmung"); len(got) != 0 {
		t.Errorf("after the close: %d entries, want none", len(got))
	}
}

// -- Events --------------------------------------------------------------------

// Public writes the admin-mutation rule leaves out tell their pages themselves: a
// sign-up the rallies (and the garapons when it issued a drawing link), a stamp the rallies.
func TestStampPublicWritesTellTheirPages(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")
	env.createGarapon(t, "Drum", map[string]any{"stampRallyId": rallyID})
	s := staffStream(t, env, env.client)
	anon := newClient(t, env)

	var issued map[string]any
	decodeInto(t, env.signUp(t, anon, rallyID, "Yao Ming", "Balmung"), http.StatusCreated, &issued)
	seen := map[string]bool{}
	for range 2 {
		seen[s.expect("resource_changed", 3*time.Second).str("resource")] = true
	}
	if !seen["stamp-rallies"] || !seen["garapons"] {
		t.Errorf("a sign-up told %v, want stamp-rallies and garapons", seen)
	}

	decodeInto(t, env.collect(t, anon, issued["cardToken"].(string), "alpha"), http.StatusOK, nil)
	if e := s.expect("resource_changed", 3*time.Second); e.str("resource") != "stamp-rallies" {
		t.Errorf("a stamp told %+v, want stamp-rallies", e)
	}
}
