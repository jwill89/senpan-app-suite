package server_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// -- Festival dashboard -------------------------------------------------------
//
// POST /api/v1/festivals/{idOrSlug}/dashboard answers "where do I stand at this festival"
// from whatever the participant's device holds: the card and drawing tokens
// issued at sign-up, and the name and world they entered as.

// festival is a published map with one rally stamp on its first occupant, an
// open garapon linked to that rally, and a paid raffle filed under the map.
type festival struct {
	mapID, rallyID, garaponID, raffleID, occupantID int
}

func (e *testEnv) createFestival(t *testing.T) festival {
	t.Helper()
	f := festival{mapID: e.createMap(t, "Obon Matsuri 2026")}
	e.publishMap(t, f.mapID)
	f.occupantID = int(firstOccupant(stallsOf(e.mapDetail(t, f.mapID))[0])["id"].(float64))

	f.rallyID = e.createRally(t, "Obon Stamp Rally", map[string]any{
		"festivalMapId": f.mapID, "publicSignup": true,
		"stamps": []map[string]any{stampBody("alpha", map[string]any{"occupantId": f.occupantID, "stampType": "game"})},
	})
	f.garaponID = e.createGarapon(t, "Obon Garapon", map[string]any{
		"stampRallyId": f.rallyID, "defaultDraws": 2,
		"prizes": []map[string]any{{"name": "Grand", "ballColor": "#e5b53f", "rate": 1, "isGrand": true}},
	})

	f.raffleID = e.assignRaffle(t, "Gaelicat Raffle", f.mapID, f.occupantID,
		map[string]any{"entryMode": "single", "costPerEntry": 50000, "maxEntries": 3})
	return f
}

// dashboard posts a dashboard read and returns its body, failing on any non-200.
func (e *testEnv) dashboard(t *testing.T, festival any, body map[string]any) map[string]any {
	t.Helper()
	resp := e.postJSON(t, fmt.Sprintf("/api/v1/festivals/%v/dashboard", festival), body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard status = %d; want 200", resp.StatusCode)
	}
	return decodeBody(t, resp)
}

// signUpForFestival signs a participant up for the festival's rally and returns
// the card and drawing tokens the sign-up issued.
func (e *testEnv) signUpForFestival(t *testing.T, f festival, name string) (card, draw string) {
	t.Helper()
	who, world, _ := strings.Cut(name, " @ ")
	var body map[string]any
	decodeInto(t, e.signUp(t, newClient(t, e), f.rallyID, who, world), http.StatusCreated, &body)
	return body["cardToken"].(string), body["garaponToken"].(string)
}

func (e *testEnv) enterRaffle(t *testing.T, raffleID int, name, world string, n int) {
	t.Helper()
	resp := e.postJSON(t, fmt.Sprintf("/api/v1/raffles/%d/enter", raffleID), enterBody(name, world, n))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("raffle enter status = %d; want 201/200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestFestivalDashboard_UnpublishedIsNotFound(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	mapID := env.createMap(t, "Draft Festival")

	resp := env.postJSON(t, fmt.Sprintf("/api/v1/festivals/%d/dashboard", mapID), map[string]any{})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unpublished dashboard status = %d; want 404 (a draft is not confirmed)", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestFestivalDashboard_AnonymousSeesTheFestivalOnly(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.createFestival(t)

	d := env.dashboard(t, f.mapID, map[string]any{})
	if d["card"] != nil {
		t.Errorf("card = %v; want none without a token", d["card"])
	}
	rally := d["rally"].(map[string]any)
	if int(rally["id"].(float64)) != f.rallyID || rally["signupOpen"] != true {
		t.Errorf("rally = %v; want the festival's rally, open for sign-up", rally)
	}
	g := d["garapon"].(map[string]any)
	if g["player"] != nil {
		t.Errorf("garapon player = %v; want none without a drawing token", g["player"])
	}
	if g["defaultDraws"].(float64) != 2 {
		t.Errorf("defaultDraws = %v; want 2 (what a sign-up would issue)", g["defaultDraws"])
	}
	// Odds stay admin-only, here as on the drawing page.
	prize := g["garapon"].(map[string]any)["prizes"].([]any)[0].(map[string]any)
	if _, has := prize["rate"]; has {
		t.Errorf("prize carries a rate (%v); the odds are for staff", prize["rate"])
	}
	raffles := d["raffles"].([]any)
	if len(raffles) != 1 {
		t.Fatalf("raffles = %d; want the one filed under the festival", len(raffles))
	}
	r := raffles[0].(map[string]any)
	if r["running"] != true || r["entry"] != nil {
		t.Errorf("raffle = %v; want running with no entry for nobody in particular", r)
	}
}

func TestFestivalDashboard_ParticipantSeesTheirOwnStanding(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.createFestival(t)
	cardToken, drawToken := env.signUpForFestival(t, f, "Yao Ming @ Balmung")
	env.enterRaffle(t, f.raffleID, "Yao Ming", "Balmung", 2)

	// Collect the rally's only stamp, so the card has something to show.
	decodeInto(t, env.collect(t, newClient(t, env), cardToken, "alpha"), http.StatusOK, nil)

	d := env.dashboard(t, f.mapID, map[string]any{"cardToken": cardToken, "garaponToken": drawToken})
	if d["participantName"] != "Yao Ming" || d["world"] != "Balmung" {
		t.Errorf("participant = %v @ %v; want the card's holder", d["participantName"], d["world"])
	}
	card := d["card"].(map[string]any)
	stamps := card["stamps"].([]any)
	if len(stamps) != 1 {
		t.Fatalf("stamps = %d; want 1", len(stamps))
	}
	s0 := stamps[0].(map[string]any)
	if s0["collected"] != true {
		t.Error("the collected stamp doesn't read as collected")
	}
	// The map marks collected stalls through this.
	if s0["occupantId"] == nil || int(s0["occupantId"].(float64)) != f.occupantID {
		t.Errorf("occupantId = %v; want %d", s0["occupantId"], f.occupantID)
	}

	player := d["garapon"].(map[string]any)["player"].(map[string]any)
	if player["maxDraws"].(float64) != 2 || player["drawsUsed"].(float64) != 0 {
		t.Errorf("player = %v; want 2 draws, none used", player)
	}

	entry := d["raffles"].([]any)[0].(map[string]any)["entry"].(map[string]any)
	if entry["numEntries"].(float64) != 2 || entry["paymentState"] != "unpaid" {
		t.Errorf("entry = %v; want 2 unpaid tickets", entry)
	}
	// What a waiver is stays between the entrant and the staff.
	for _, key := range []string{"amountWaived", "amountDue", "totalCost"} {
		if _, has := entry[key]; has {
			t.Errorf("entry carries %q; no gil figure belongs on a public payload", key)
		}
	}
}

func TestFestivalDashboard_DrawingLinkBringsItsCard(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.createFestival(t)
	cardToken, drawToken := env.signUpForFestival(t, f, "Yao Ming @ Balmung")

	// A device that only kept the drawing link - a staff-issued one, say.
	d := env.dashboard(t, f.mapID, map[string]any{"garaponToken": drawToken})
	if d["card"] == nil {
		t.Fatal("the drawing link's paired card wasn't loaded")
	}
	if d["cardToken"] != cardToken {
		t.Errorf("cardToken = %v; want the paired card's, so the device can keep it", d["cardToken"])
	}
}

func TestFestivalDashboard_CardAloneReportsDrawsButNotTheirToken(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.createFestival(t)
	cardToken, drawToken := env.signUpForFestival(t, f, "Yao Ming @ Balmung")

	d := env.dashboard(t, f.mapID, map[string]any{"cardToken": cardToken})
	g := d["garapon"].(map[string]any)
	if g["player"] != nil {
		t.Error("a card alone must not open the drawing link")
	}
	if g["hasPairedLink"] != true || g["pairedDrawsLeft"].(float64) != 2 {
		t.Errorf("paired = %v / %v; want a link with 2 draws left", g["hasPairedLink"], g["pairedDrawsLeft"])
	}
	if containsString(d, drawToken) {
		t.Error("the response carries the drawing token; a card must never reveal it")
	}
}

func TestFestivalDashboard_DropsAnotherPersonsDrawingLink(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.createFestival(t)
	cardA, _ := env.signUpForFestival(t, f, "Yao Ming @ Balmung")
	_, drawB := env.signUpForFestival(t, f, "Aria Ashwood @ Gilgamesh")

	d := env.dashboard(t, f.mapID, map[string]any{"cardToken": cardA, "garaponToken": drawB})
	if d["participantName"] != "Yao Ming" {
		t.Errorf("participant = %v; want the card's holder", d["participantName"])
	}
	g := d["garapon"].(map[string]any)
	if g["player"] != nil {
		t.Error("another participant's drawing link was shown beside this card")
	}
	if g["hasPairedLink"] != true {
		t.Error("the card's own paired link should still be reported")
	}
}

func TestFestivalDashboard_IgnoresAnotherFestivalsTokens(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.createFestival(t)
	cardToken, drawToken := env.signUpForFestival(t, f, "Yao Ming @ Balmung")

	other := env.createMap(t, "Another Festival")
	env.publishMap(t, other)

	d := env.dashboard(t, other, map[string]any{"cardToken": cardToken, "garaponToken": drawToken})
	if d["card"] != nil || d["garapon"] != nil || d["rally"] != nil {
		t.Errorf("another festival's links leaked in: card=%v garapon=%v rally=%v", d["card"], d["garapon"], d["rally"])
	}
	if d["participantName"] != "" {
		t.Errorf("participantName = %v; want nothing known here", d["participantName"])
	}
}

func TestFestivalDashboard_NameMatchesWholeEntriesOnly(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.createFestival(t)
	env.enterRaffle(t, f.raffleID, "Yao Ming", "Balmung", 1)

	d := env.dashboard(t, f.mapID, map[string]any{"characterName": "yao ming", "world": "balmung"})
	if d["raffles"].([]any)[0].(map[string]any)["entry"] == nil {
		t.Error("the entry wasn't matched case-insensitively on the whole name")
	}
	for _, probe := range []map[string]any{
		{"characterName": "Yao", "world": "Balmung"},        // a prefix is not a name
		{"characterName": "Yao Ming", "world": "Gilgamesh"}, // nor is the same name elsewhere
	} {
		d := env.dashboard(t, f.mapID, probe)
		if e := d["raffles"].([]any)[0].(map[string]any)["entry"]; e != nil {
			t.Errorf("probe %v matched %v; want only a whole name and world", probe, e)
		}
	}
}

func TestFestivalDashboard_ClosedRaffleNamesItsWinner(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.createFestival(t)
	env.enterRaffle(t, f.raffleID, "Yao Ming", "Balmung", 1)

	entry := env.raffleEntryByName(t, f.raffleID, "Yao Ming", "Balmung")
	env.settle(t, f.raffleID, entry, nil)
	decodeInto(t, env.postJSON(t, fmt.Sprintf("/api/v1/raffles/%d/pick-winner", f.raffleID), nil), http.StatusOK, nil)
	decodeInto(t, env.postJSON(t, fmt.Sprintf("/api/v1/raffles/%d/verify-winner", f.raffleID),
		map[string]any{"winnerEntryId": entry["id"]}), http.StatusOK, nil)

	d := env.dashboard(t, f.mapID, map[string]any{"characterName": "Yao Ming", "world": "Balmung"})
	r := d["raffles"].([]any)[0].(map[string]any)
	if r["running"] != false {
		t.Error("a closed raffle reads as running")
	}
	winner, _ := r["winner"].(map[string]any)
	if winner["characterName"] != "Yao Ming" || winner["world"] != "Balmung" || r["isWinner"] != true {
		t.Errorf("winner = %v (isWinner %v); want the participant", winner, r["isWinner"])
	}
	if winner["numEntries"] != float64(1) {
		t.Errorf("winner holds %v tickets; want 1", winner["numEntries"])
	}
}

func TestFestivalDashboard_CardAndDrawingPagesLinkTheFestival(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.createFestival(t)
	cardToken, drawToken := env.signUpForFestival(t, f, "Yao Ming @ Balmung")
	want := fmt.Sprint(f.mapID) // no shortcode set, so the link is the id

	anon := newClient(t, env)
	card := decodeBody(t, getAs(t, anon, env, cardPath(cardToken, "")))
	if got := card["rally"].(map[string]any)["festivalLink"]; got != want {
		t.Errorf("card festivalLink = %v; want %q", got, want)
	}
	draw := decodeBody(t, getAs(t, anon, env, "/api/v1/garapon-players/"+drawToken))
	if got := draw["festivalLink"]; got != want {
		t.Errorf("garapon festivalLink = %v; want %q", got, want)
	}
}

// containsString reports whether s appears anywhere in a decoded JSON value.
func containsString(v any, s string) bool {
	switch x := v.(type) {
	case string:
		return x == s
	case map[string]any:
		for _, child := range x {
			if containsString(child, s) {
				return true
			}
		}
	case []any:
		for _, child := range x {
			if containsString(child, s) {
				return true
			}
		}
	}
	return false
}

func TestFestivalDashboard_RefusesLongNamesAndBudgetsReads(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createMap(t, "Obon Matsuri 2026")
	env.publishMap(t, id)
	path := fmt.Sprintf("/api/v1/festivals/%d/dashboard", id)

	resp := env.postJSON(t, path, map[string]any{"characterName": strings.Repeat("a", 61), "world": "Balmung"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("a name past 60 characters = %d; want 422", resp.StatusCode)
	}
	resp.Body.Close()

	// The shortcode works too, and an unknown festival is a 404.
	if r := env.postJSON(t, "/api/v1/festivals/nowhere/dashboard", map[string]any{}); r.StatusCode != http.StatusNotFound {
		t.Errorf("an unknown festival = %d; want 404", r.StatusCode)
	} else {
		r.Body.Close()
	}

	// 120 reads from one address in ten minutes, the refused ones included.
	var last *http.Response
	for range 125 {
		if last != nil {
			last.Body.Close()
		}
		last = env.postJSON(t, path, map[string]any{})
		if last.StatusCode == http.StatusTooManyRequests {
			break
		}
	}
	if last.StatusCode != http.StatusTooManyRequests || last.Header.Get("Retry-After") == "" {
		t.Errorf("the budget never ran out: %d, Retry-After %q", last.StatusCode, last.Header.Get("Retry-After"))
	}
	last.Body.Close()
}
