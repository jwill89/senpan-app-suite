package server_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"
)

// garaponBody is a full garapon as a save sends it; over replaces fields of it.
func garaponBody(title string, over map[string]any) map[string]any {
	body := map[string]any{
		"title": title, "details": "round and round", "grandPrizeImage": "", "defaultDraws": 1,
		"prizes": []map[string]any{
			{"name": "Grand", "ballColor": "#e5b53f", "rate": 1, "isGrand": true},
			{"name": "Consolation", "ballColor": "#cccccc", "rate": 9, "isGrand": false},
		},
	}
	for k, v := range over {
		body[k] = v
	}
	return body
}

func garaponPath(id int, tail string) string {
	return fmt.Sprintf("/api/v1/garapons/%d", id) + tail
}

// createGarapon makes a garapon (201, the garapon itself) and answers its id.
func (e *testEnv) createGarapon(t *testing.T, title string, over map[string]any) int {
	t.Helper()
	var made map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/garapons", garaponBody(title, over)), http.StatusCreated, &made)
	if made["status"] != "open" {
		t.Errorf("a new garapon is %v, want open", made["status"])
	}
	return int(made["id"].(float64))
}

// issueLink issues a drawing link (201) and answers it.
func (e *testEnv) issueLink(t *testing.T, garaponID int, name string, maxDraws int) map[string]any {
	t.Helper()
	var player map[string]any
	decodeInto(t, e.postJSON(t, garaponPath(garaponID, "/players"),
		map[string]any{"playerName": name, "maxDraws": maxDraws}), http.StatusCreated, &player)
	return player
}

// garaponDetail reads a garapon's staff detail.
func (e *testEnv) garaponDetail(t *testing.T, id int) map[string]any {
	t.Helper()
	var detail map[string]any
	decodeInto(t, e.get(t, garaponPath(id, "")), http.StatusOK, &detail)
	return detail
}

func drawPath(token string) string { return "/api/v1/garapon-players/" + token + "/draw" }

// Every staff operation needs the Garapon page: 401 signed out, 403 without it, and its
// grantee gets through. The two token operations need nothing.
func TestGaraponsGuard(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createGarapon(t, "Drum", nil)
	playerID := int(env.issueLink(t, id, "Hero", 1)["id"].(float64))
	env.signOut(t)

	staff := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/garapons"},
		{http.MethodPost, "/api/v1/garapons"},
		{http.MethodGet, garaponPath(id, "")},
		{http.MethodPut, garaponPath(id, "")},
		{http.MethodDelete, garaponPath(id, "")},
		{http.MethodPost, garaponPath(id, "/close")},
		{http.MethodPost, garaponPath(id, "/reopen")},
		{http.MethodPost, garaponPath(id, "/players")},
		{http.MethodDelete, garaponPath(id, fmt.Sprintf("/players/%d", playerID))},
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

	grantee := makeActiveUser(t, env, "drumstaff", "correct horse battery", []string{"festival-garapon"})
	if got := status(getAs(t, grantee, env, "/api/v1/garapons")); got != http.StatusOK {
		t.Errorf("the page's grantee: %d, want 200", got)
	}
}

// A garapon is made with its prizes, listed without them (but with its counts) and read
// whole with its players and draw log.
func TestGaraponCreateListDetail(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createGarapon(t, "Festival Drum", nil)
	env.issueLink(t, id, "Hero", 2)

	var list struct {
		Garapons []map[string]any `json:"garapons"`
	}
	decodeInto(t, env.get(t, "/api/v1/garapons"), http.StatusOK, &list)
	if len(list.Garapons) != 1 {
		t.Fatalf("%d garapons listed, want 1", len(list.Garapons))
	}
	row := list.Garapons[0]
	if n := len(row["prizes"].([]any)); n != 0 {
		t.Errorf("the list carries %d prizes, want none", n)
	}
	if row["playerCount"].(float64) != 1 || row["drawCount"].(float64) != 0 {
		t.Errorf("counts: %v players, %v draws, want 1 and 0", row["playerCount"], row["drawCount"])
	}

	detail := env.garaponDetail(t, id)
	g := detail["garapon"].(map[string]any)
	if g["title"] != "Festival Drum" || len(g["prizes"].([]any)) != 2 {
		t.Errorf("detail: %v, want the title and 2 prizes", g)
	}
	if len(detail["players"].([]any)) != 1 || len(detail["draws"].([]any)) != 0 {
		t.Errorf("detail players/draws: %v / %v", detail["players"], detail["draws"])
	}
}

// A save is refused for what the schema and the prize rules can't take, each at its field.
func TestGaraponSaveValidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	cases := map[string]struct {
		body map[string]any
		want string
	}{
		"blank title":      {garaponBody("   ", nil), "body.title"},
		"no prizes":        {garaponBody("T", map[string]any{"prizes": []map[string]any{}}), "body.prizes"},
		"only blank prize": {garaponBody("T", map[string]any{"prizes": []map[string]any{{"name": " ", "rate": 1}}}), "body.prizes"},
		"two grand prizes": {garaponBody("T", map[string]any{"prizes": []map[string]any{
			{"name": "A", "rate": 1, "isGrand": true}, {"name": "B", "rate": 1, "isGrand": true}}}), "body.prizes"},
		"a negative rate": {garaponBody("T", map[string]any{"prizes": []map[string]any{
			{"name": "A", "rate": -1, "isGrand": true}}}), "body.prizes[0].rate"},
		"no such rally": {garaponBody("T", map[string]any{"stampRallyId": 999999}), "body.stampRallyId"},
	}
	for name, c := range cases {
		if at := invalid(t, env.postJSON(t, "/api/v1/garapons", c.body)); !slices.Contains(at, c.want) {
			t.Errorf("%s: errors at %v, want %s", name, at, c.want)
		}
	}
}

// With no prize marked grand the first is; blank prizes are dropped and a blank color
// takes the default.
func TestGaraponPrizesAreSanitized(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createGarapon(t, "T", map[string]any{"prizes": []map[string]any{
		{"name": "  ", "rate": 5},
		{"name": "A", "ballColor": "", "rate": 1},
		{"name": "B", "ballColor": "#111111", "rate": 1},
	}})

	prizes := env.garaponDetail(t, id)["garapon"].(map[string]any)["prizes"].([]any)
	if len(prizes) != 2 {
		t.Fatalf("%d prizes kept, want 2", len(prizes))
	}
	first := prizes[0].(map[string]any)
	if first["isGrand"] != true || first["ballColor"] != "#cccccc" {
		t.Errorf("the first prize: %v, want grand with the default color", first)
	}
}

func TestGaraponUpdateReplacesPrizes(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createGarapon(t, "Old Title", nil)

	var saved map[string]any
	decodeInto(t, env.putJSON(t, garaponPath(id, ""), garaponBody("New Title", map[string]any{
		"prizes": []map[string]any{{"name": "Only", "rate": 1, "isGrand": true}},
	})), http.StatusOK, &saved)
	if saved["title"] != "New Title" || len(saved["prizes"].([]any)) != 1 {
		t.Errorf("the saved garapon: %v", saved)
	}
	if env.garaponDetail(t, id)["garapon"].(map[string]any)["status"] != "open" {
		t.Error("a save changed the status")
	}
}

func TestGaraponCloseAndReopen(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createGarapon(t, "G", nil)

	var closed, reopened map[string]any
	decodeInto(t, env.postJSON(t, garaponPath(id, "/close"), nil), http.StatusOK, &closed)
	if closed["status"] != "closed" {
		t.Errorf("close answered %v", closed["status"])
	}
	decodeInto(t, env.postJSON(t, garaponPath(id, "/reopen"), nil), http.StatusOK, &reopened)
	if reopened["status"] != "open" {
		t.Errorf("reopen answered %v", reopened["status"])
	}
}

func TestGaraponDelete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createGarapon(t, "G", nil)

	if got := status(env.del(t, garaponPath(id, ""))); got != http.StatusNoContent {
		t.Fatalf("delete: %d, want 204", got)
	}
	if got := status(env.get(t, garaponPath(id, ""))); got != http.StatusNotFound {
		t.Errorf("read after delete: %d, want 404", got)
	}
}

// An id nothing has is a 404 on every operation that names one, the writes included.
func TestGaraponUnknownIDs(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, garaponPath(9999, "")},
		{http.MethodPut, garaponPath(9999, "")},
		{http.MethodDelete, garaponPath(9999, "")},
		{http.MethodPost, garaponPath(9999, "/close")},
		{http.MethodPost, garaponPath(9999, "/reopen")},
		{http.MethodPost, garaponPath(9999, "/players")},
		{http.MethodDelete, garaponPath(9999, "/players/1")},
	} {
		var body any
		switch c.method {
		case http.MethodPut:
			body = garaponBody("T", nil)
		case http.MethodPost:
			body = map[string]any{"playerName": "Hero"}
		}
		if got := status(env.reqJSON(t, c.method, c.path, body)); got != http.StatusNotFound {
			t.Errorf("%s %s: %d, want 404", c.method, c.path, got)
		}
	}
}

// -- Drawing links -------------------------------------------------------------

func TestGaraponPlayerCreateAndDelete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G", nil)

	p := env.issueLink(t, gid, "Hero", 2)
	if p["token"] == "" || p["maxDraws"].(float64) != 2 || p["drawsUsed"].(float64) != 0 || p["stampCardToken"] != "" {
		t.Errorf("the issued link: %v", p)
	}
	if n := len(env.garaponDetail(t, gid)["players"].([]any)); n != 1 {
		t.Fatalf("%d links listed, want 1", n)
	}
	if got := status(env.del(t, garaponPath(gid, fmt.Sprintf("/players/%d", int(p["id"].(float64)))))); got != http.StatusNoContent {
		t.Errorf("delete an undrawn link: %d, want 204", got)
	}
}

// A composed Name @ World is split when no world comes beside it; a link made without a
// number of draws takes the garapon's default.
func TestGaraponPlayerNameAndDefaultDraws(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G", map[string]any{"defaultDraws": 3})

	var p map[string]any
	decodeInto(t, env.postJSON(t, garaponPath(gid, "/players"), map[string]any{"playerName": "Aria Ashwood @ Gilgamesh"}),
		http.StatusCreated, &p)
	if p["playerName"] != "Aria Ashwood" || p["world"] != "Gilgamesh" {
		t.Errorf("name %v / world %v, want the label split", p["playerName"], p["world"])
	}
	if p["maxDraws"].(float64) != 3 {
		t.Errorf("maxDraws %v, want the garapon's default of 3", p["maxDraws"])
	}

	if at := invalid(t, env.postJSON(t, garaponPath(gid, "/players"), map[string]any{"playerName": "  "})); !slices.Contains(at, "body.playerName") {
		t.Errorf("a blank name: errors at %v", at)
	}
}

// A garapon left without a default draw count carries 1, so its links can draw.
func TestGaraponDefaultDrawsFallsBackToOne(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	body := garaponBody("No Default", nil)
	delete(body, "defaultDraws")
	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/garapons", body), http.StatusCreated, &made)
	if made["defaultDraws"].(float64) != 1 {
		t.Errorf("defaultDraws %v, want 1", made["defaultDraws"])
	}
}

// A link that has drawn can't be deleted while the garapon is open; once it is closed
// any can, and the draws stay in the log.
func TestGaraponDeleteDrawnLink(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G", nil)
	p := env.issueLink(t, gid, "Hero", 2)
	pid := int(p["id"].(float64))
	anon := newClient(t, env)
	if got := status(postAs(t, anon, env, drawPath(p["token"].(string)), nil)); got != http.StatusOK {
		t.Fatalf("draw: %d, want 200", got)
	}

	if got := status(env.del(t, garaponPath(gid, fmt.Sprintf("/players/%d", pid)))); got != http.StatusConflict {
		t.Errorf("delete a drawn link (open): %d, want 409", got)
	}
	decodeInto(t, env.postJSON(t, garaponPath(gid, "/close"), nil), http.StatusOK, nil)
	if got := status(env.del(t, garaponPath(gid, fmt.Sprintf("/players/%d", pid)))); got != http.StatusNoContent {
		t.Errorf("delete a drawn link (closed): %d, want 204", got)
	}
	if n := len(env.garaponDetail(t, gid)["draws"].([]any)); n != 1 {
		t.Errorf("%d draws kept after the link went, want 1", n)
	}
}

// Another garapon's link is not found under this one, so the open check can't be read off
// the wrong drum.
func TestGaraponDeleteLinkOfAnotherGarapon(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	a, b := env.createGarapon(t, "A", nil), env.createGarapon(t, "B", nil)
	p := env.issueLink(t, b, "Hero", 1)
	if got := status(env.del(t, garaponPath(a, fmt.Sprintf("/players/%d", int(p["id"].(float64)))))); got != http.StatusNotFound {
		t.Errorf("delete under the wrong garapon: %d, want 404", got)
	}
}

// -- A garapon that serves a stamp rally ---------------------------------------

// A link for a garapon that serves an open rally issues a card too, on a token of its
// own, and the two tokens open only their own things. Deleting the rally unlinks the
// garapon; linking a closed one is refused.
func TestGaraponLinkedStampRally(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createRally(t, "Linked Rally", nil)
	gid := env.createGarapon(t, "Drum", map[string]any{"stampRallyId": rallyID})

	g := env.garaponDetail(t, gid)["garapon"].(map[string]any)
	if g["stampRallyTitle"] != "Linked Rally" {
		t.Errorf("stampRallyTitle %v", g["stampRallyTitle"])
	}

	player := env.issueLink(t, gid, "Tester", 1)
	token, cardToken := player["token"].(string), player["stampCardToken"].(string)
	if cardToken == "" || cardToken == token {
		t.Fatalf("card token %q beside drawing token %q: want two secrets", cardToken, token)
	}
	anon := newClient(t, env)
	if got := status(getAs(t, anon, env, "/api/v1/stamp-cards/"+cardToken)); got != http.StatusOK {
		t.Errorf("the card by its token: %d, want 200", got)
	}
	if got := status(getAs(t, anon, env, "/api/v1/garapon-players/"+cardToken)); got != http.StatusNotFound {
		t.Errorf("a drawing link by the card token: %d, want 404", got)
	}
	if got := status(getAs(t, anon, env, "/api/v1/stamp-cards/"+token)); got != http.StatusNotFound {
		t.Errorf("a card by the drawing token: %d, want 404", got)
	}

	env.setRallyStatus(t, rallyID, "closed")
	if got := status(env.postJSON(t, "/api/v1/garapons", garaponBody("Drum2", map[string]any{"stampRallyId": rallyID}))); got != http.StatusConflict {
		t.Errorf("link a closed rally: %d, want 409", got)
	}

	if got := status(env.del(t, "/api/v1/stamp-rallies/"+fmt.Sprint(rallyID))); got != http.StatusNoContent {
		t.Fatalf("delete the rally: %d", got)
	}
	if env.garaponDetail(t, gid)["garapon"].(map[string]any)["stampRallyId"] != nil {
		t.Error("the garapon still names the deleted rally")
	}
}

// Deleting a link takes the card issued with it, unless that card has collected stamps in
// a rally still open.
func TestGaraponDeleteLinkSweepsItsCard(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createRally(t, "Rally", nil)
	gid := env.createGarapon(t, "Drum", map[string]any{"stampRallyId": rallyID})
	anon := newClient(t, env)

	idle := env.issueLink(t, gid, "Idle", 1)
	stamped := env.issueLink(t, gid, "Stamped", 1)
	if got := status(postAs(t, anon, env, "/api/v1/stamp-cards/"+stamped["stampCardToken"].(string)+"/stamp",
		map[string]any{"password": "alpha"})); got != http.StatusOK {
		t.Fatalf("collect: %d", got)
	}

	for _, p := range []map[string]any{idle, stamped} {
		if got := status(env.del(t, garaponPath(gid, fmt.Sprintf("/players/%d", int(p["id"].(float64)))))); got != http.StatusNoContent {
			t.Fatalf("delete a link: %d", got)
		}
	}
	if got := status(getAs(t, anon, env, "/api/v1/stamp-cards/"+idle["stampCardToken"].(string))); got != http.StatusNotFound {
		t.Errorf("the idle card after its link went: %d, want 404", got)
	}
	if got := status(getAs(t, anon, env, "/api/v1/stamp-cards/"+stamped["stampCardToken"].(string))); got != http.StatusOK {
		t.Errorf("the stamped card after its link went: %d, want 200 (kept while the rally is open)", got)
	}
}

// -- The public drawing link ---------------------------------------------------

func TestGaraponPublicView(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "Festival", nil)
	p := env.issueLink(t, gid, "Hero", 3)

	var view struct {
		Garapon struct {
			Title  string           `json:"title"`
			Prizes []map[string]any `json:"prizes"`
		} `json:"garapon"`
		Player map[string]any `json:"player"`
	}
	decodeInto(t, getAs(t, newClient(t, env), env, "/api/v1/garapon-players/"+p["token"].(string)), http.StatusOK, &view)
	if view.Garapon.Title != "Festival" {
		t.Errorf("title %q", view.Garapon.Title)
	}
	for _, pr := range view.Garapon.Prizes {
		if _, leaked := pr["rate"]; leaked {
			t.Errorf("a public prize carries its rate: %v", pr)
		}
	}
	if view.Player["maxDraws"].(float64) != 3 {
		t.Errorf("maxDraws %v", view.Player["maxDraws"])
	}
	if _, leaked := view.Player["token"]; leaked {
		t.Error("the public player carries its token")
	}
}

func TestGaraponPublicUnknownToken(t *testing.T) {
	env := newTestEnv(t)
	anon := newClient(t, env)
	if got := status(getAs(t, anon, env, "/api/v1/garapon-players/deadbeef")); got != http.StatusNotFound {
		t.Errorf("view: %d, want 404", got)
	}
	if got := status(postAs(t, anon, env, drawPath("deadbeef"), nil)); got != http.StatusNotFound {
		t.Errorf("draw: %d, want 404", got)
	}
}

func TestGaraponDrawSucceedsAndCounts(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G", nil)
	p := env.issueLink(t, gid, "Hero", 2)

	var res struct {
		Draw      map[string]any `json:"draw"`
		DrawsUsed int            `json:"drawsUsed"`
		MaxDraws  int            `json:"maxDraws"`
	}
	decodeInto(t, postAs(t, newClient(t, env), env, drawPath(p["token"].(string)), nil), http.StatusOK, &res)
	if res.DrawsUsed != 1 || res.MaxDraws != 2 || res.Draw["prizeName"] == nil {
		t.Errorf("the draw: %+v", res)
	}
}

// Only the allowance is drawn and a prize with no rate never wins: 15 draws land on the
// one prize with weight, and the 16th is refused.
func TestGaraponDrawCapAndWeighting(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "Fest", map[string]any{"prizes": []map[string]any{
		{"name": "Nothing", "ballColor": "#111111", "rate": 0},
		{"name": "Jackpot", "ballColor": "#ffcc00", "rate": 100, "isGrand": true},
	}})
	p := env.issueLink(t, gid, "Aria", 15)
	anon := newClient(t, env)

	for i := range 15 {
		var res struct {
			Draw map[string]any `json:"draw"`
		}
		decodeInto(t, postAs(t, anon, env, drawPath(p["token"].(string)), nil), http.StatusOK, &res)
		if res.Draw["prizeName"] != "Jackpot" || res.Draw["isGrand"] != true {
			t.Fatalf("draw %d landed on %v; a 0-rate prize must never win", i, res.Draw)
		}
	}
	if got := status(postAs(t, anon, env, drawPath(p["token"].(string)), nil)); got != http.StatusConflict {
		t.Errorf("a draw past the allowance: %d, want 409", got)
	}
	if n := len(env.garaponDetail(t, gid)["draws"].([]any)); n != 15 {
		t.Errorf("%d draws logged, want 15", n)
	}
}

func TestGaraponDrawRefusals(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G", nil)
	p := env.issueLink(t, gid, "Hero", 1)
	token := p["token"].(string)
	anon := newClient(t, env)

	if got := status(postAs(t, anon, env, drawPath(token), nil)); got != http.StatusOK {
		t.Fatalf("the only draw: %d", got)
	}
	if _, _, detail := problem(t, postAs(t, anon, env, drawPath(token), nil)); detail != "No draws remaining." {
		t.Errorf("no draws left: %q", detail)
	}

	q := env.issueLink(t, gid, "Other", 2)
	decodeInto(t, env.postJSON(t, garaponPath(gid, "/close"), nil), http.StatusOK, nil)
	resp := postAs(t, anon, env, drawPath(q["token"].(string)), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("a closed garapon: %d, want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

// The log keeps the whole identity, the name and the world, so two players who share a
// character name stay apart once their link is gone.
func TestGaraponDrawLogKeepsNameAndWorld(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G", nil)
	var p map[string]any
	decodeInto(t, env.postJSON(t, garaponPath(gid, "/players"),
		map[string]any{"playerName": "Aria Ashwood", "world": "Gilgamesh", "maxDraws": 1}), http.StatusCreated, &p)
	decodeInto(t, postAs(t, newClient(t, env), env, drawPath(p["token"].(string)), nil), http.StatusOK, nil)

	draws := env.garaponDetail(t, gid)["draws"].([]any)
	if got := draws[0].(map[string]any)["playerName"]; got != "Aria Ashwood @ Gilgamesh" {
		t.Errorf("the logged player %v, want the composed identity", got)
	}
}

// The draw can't be undone, so the budget is spent before the token is looked up: tries at
// wrong tokens run it out, and then a real one is held back as well.
func TestGaraponDrawIsRateLimited(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G", nil)
	p := env.issueLink(t, gid, "Hero", 1)
	anon := newClient(t, env)

	var limited *http.Response
	for range 25 {
		resp := postAs(t, anon, env, drawPath("deadbeef00"), nil)
		if resp.StatusCode == http.StatusTooManyRequests {
			limited = resp
			break
		}
		resp.Body.Close()
	}
	if limited == nil {
		t.Fatal("25 draws at an unknown token were never limited")
	}
	if limited.Header.Get("Retry-After") == "" {
		t.Error("the 429 doesn't say when to try again")
	}
	limited.Body.Close()

	if got := status(postAs(t, anon, env, drawPath(p["token"].(string)), nil)); got != http.StatusTooManyRequests {
		t.Errorf("a real token once the budget is spent: %d, want 429", got)
	}
}

// A public draw tells the Garapon page; so does issuing a link that came with a card, and
// deleting a stamp rally tells the garapons it unlinks.
func TestGaraponChangesTellTheirPages(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createRally(t, "Rally", nil)
	gid := env.createGarapon(t, "G", map[string]any{"stampRallyId": rallyID})
	p := env.issueLink(t, gid, "Hero", 2)
	s := staffStream(t, env, env.client)

	decodeInto(t, postAs(t, newClient(t, env), env, drawPath(p["token"].(string)), nil), http.StatusOK, nil)
	if e := s.expect("resource_changed", 3*time.Second); e.str("resource") != "garapons" {
		t.Errorf("a draw told %+v, want garapons", e)
	}

	env.issueLink(t, gid, "Another", 1)
	seen := map[string]bool{}
	for range 2 {
		seen[s.expect("resource_changed", 3*time.Second).str("resource")] = true
	}
	if !seen["garapons"] || !seen["stamp-rallies"] {
		t.Errorf("issuing a link with a card told %v, want garapons and stamp-rallies", seen)
	}

	if got := status(env.del(t, "/api/v1/stamp-rallies/"+fmt.Sprint(rallyID))); got != http.StatusNoContent {
		t.Fatalf("delete the rally: %d", got)
	}
	seen = map[string]bool{}
	for range 2 {
		seen[s.expect("resource_changed", 3*time.Second).str("resource")] = true
	}
	if !seen["garapons"] || !seen["stamp-rallies"] {
		t.Errorf("deleting the rally told %v, want stamp-rallies and garapons", seen)
	}
}
