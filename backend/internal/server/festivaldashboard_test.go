package server_test

import (
	"fmt"
	"net/http"
	"testing"
)

// -- Festival dashboard -------------------------------------------------------
//
// POST /api/festival-dashboard/{id} answers "where do I stand at this festival"
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

	resp := e.postJSON(t, "/api/stamp-rallies", map[string]any{
		"title": "Obon Stamp Rally", "festival_map_id": f.mapID, "public_signup": true,
		"stamps": []map[string]any{
			{"occupant_id": f.occupantID, "password": "alpha", "stamp_type": "game",
				"image":     "images/stamp_stamps/a.png",
				"placement": map[string]any{"x": 10, "y": 10, "width": 15, "height": 15}},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create rally status = %d; want 201", resp.StatusCode)
	}
	f.rallyID = int(decodeBody(t, resp)["stamp_rally"].(map[string]any)["id"].(float64))

	f.garaponID = e.createGarapon(t, "Obon Garapon")
	link := e.putJSON(t, fmt.Sprintf("/api/garapons/%d", f.garaponID), map[string]any{
		"title": "Obon Garapon", "stamp_rally_id": f.rallyID, "default_draws": 2,
		"prizes": []map[string]any{{"name": "Grand", "ball_color": "#e5b53f", "rate": 1, "is_grand": true}},
	})
	if link.StatusCode != http.StatusOK {
		t.Fatalf("link garapon status = %d; want 200", link.StatusCode)
	}
	link.Body.Close()

	f.raffleID = e.assignRaffle(t, "Gaelicat Raffle", f.mapID, f.occupantID,
		map[string]any{"entry_mode": "single", "cost_per_entry": 50000, "max_entries": 3})
	return f
}

// dashboard posts a dashboard read and returns its body, failing on any non-200.
func (e *testEnv) dashboard(t *testing.T, festival any, body map[string]any) map[string]any {
	t.Helper()
	resp := e.postJSON(t, fmt.Sprintf("/api/festival-dashboard/%v", festival), body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard status = %d; want 200", resp.StatusCode)
	}
	return decodeBody(t, resp)
}

// signUpForFestival signs a participant up for the festival's rally and returns
// the card and drawing tokens the sign-up issued.
func (e *testEnv) signUpForFestival(t *testing.T, f festival, name string) (card, draw string) {
	t.Helper()
	resp := e.signUp(t, f.rallyID, name)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("signup status = %d; want 201", resp.StatusCode)
	}
	body := decodeBody(t, resp)
	return body["card_token"].(string), body["garapon_token"].(string)
}

func (e *testEnv) enterRaffle(t *testing.T, raffleID int, name, world string, n int) {
	t.Helper()
	resp := e.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", raffleID), map[string]any{
		"character_name": name, "world": world, "num_entries": n,
	})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("raffle enter status = %d; want 201/200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestFestivalDashboard_UnpublishedIsNotFound(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	mapID := env.createMap(t, "Draft Festival")

	resp := env.postJSON(t, fmt.Sprintf("/api/festival-dashboard/%d", mapID), map[string]any{})
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
	if int(rally["id"].(float64)) != f.rallyID || rally["signup_open"] != true {
		t.Errorf("rally = %v; want the festival's rally, open for sign-up", rally)
	}
	g := d["garapon"].(map[string]any)
	if g["player"] != nil {
		t.Errorf("garapon player = %v; want none without a drawing token", g["player"])
	}
	if g["default_draws"].(float64) != 2 {
		t.Errorf("default_draws = %v; want 2 (what a sign-up would issue)", g["default_draws"])
	}
	// Odds stay admin-only, here as on the drawing page.
	prize := g["garapon"].(map[string]any)["prizes"].([]any)[0].(map[string]any)
	if prize["rate"].(float64) != 0 {
		t.Errorf("prize rate = %v; want 0 on a public payload", prize["rate"])
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
	stamp := env.postJSON(t, "/api/stamp-card/"+cardToken+"/stamp", map[string]any{"password": "alpha"})
	if stamp.StatusCode != http.StatusOK {
		t.Fatalf("stamp status = %d; want 200", stamp.StatusCode)
	}
	stamp.Body.Close()

	d := env.dashboard(t, f.mapID, map[string]any{"card_token": cardToken, "garapon_token": drawToken})
	if d["participant_name"] != "Yao Ming" || d["world"] != "Balmung" {
		t.Errorf("participant = %v @ %v; want the card's holder", d["participant_name"], d["world"])
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
	if s0["occupant_id"] == nil || int(s0["occupant_id"].(float64)) != f.occupantID {
		t.Errorf("occupant_id = %v; want %d", s0["occupant_id"], f.occupantID)
	}

	player := d["garapon"].(map[string]any)["player"].(map[string]any)
	if player["max_draws"].(float64) != 2 || player["draws_used"].(float64) != 0 {
		t.Errorf("player = %v; want 2 draws, none used", player)
	}

	entry := d["raffles"].([]any)[0].(map[string]any)["entry"].(map[string]any)
	if entry["num_entries"].(float64) != 2 || entry["payment_state"] != "unpaid" {
		t.Errorf("entry = %v; want 2 unpaid tickets", entry)
	}
	// What a waiver is stays between the entrant and the staff.
	for _, key := range []string{"amount_waived", "amount_due", "total_cost"} {
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
	d := env.dashboard(t, f.mapID, map[string]any{"garapon_token": drawToken})
	if d["card"] == nil {
		t.Fatal("the drawing link's paired card wasn't loaded")
	}
	if d["card_token"] != cardToken {
		t.Errorf("card_token = %v; want the paired card's, so the device can keep it", d["card_token"])
	}
}

func TestFestivalDashboard_CardAloneReportsDrawsButNotTheirToken(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.createFestival(t)
	cardToken, drawToken := env.signUpForFestival(t, f, "Yao Ming @ Balmung")

	d := env.dashboard(t, f.mapID, map[string]any{"card_token": cardToken})
	g := d["garapon"].(map[string]any)
	if g["player"] != nil {
		t.Error("a card alone must not open the drawing link")
	}
	if g["has_paired_link"] != true || g["paired_draws_left"].(float64) != 2 {
		t.Errorf("paired = %v / %v; want a link with 2 draws left", g["has_paired_link"], g["paired_draws_left"])
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

	d := env.dashboard(t, f.mapID, map[string]any{"card_token": cardA, "garapon_token": drawB})
	if d["participant_name"] != "Yao Ming" {
		t.Errorf("participant = %v; want the card's holder", d["participant_name"])
	}
	g := d["garapon"].(map[string]any)
	if g["player"] != nil {
		t.Error("another participant's drawing link was shown beside this card")
	}
	if g["has_paired_link"] != true {
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

	d := env.dashboard(t, other, map[string]any{"card_token": cardToken, "garapon_token": drawToken})
	if d["card"] != nil || d["garapon"] != nil || d["rally"] != nil {
		t.Errorf("another festival's links leaked in: card=%v garapon=%v rally=%v", d["card"], d["garapon"], d["rally"])
	}
	if d["participant_name"] != "" {
		t.Errorf("participant_name = %v; want nothing known here", d["participant_name"])
	}
}

func TestFestivalDashboard_NameMatchesWholeEntriesOnly(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.createFestival(t)
	env.enterRaffle(t, f.raffleID, "Yao Ming", "Balmung", 1)

	d := env.dashboard(t, f.mapID, map[string]any{"name": "yao ming", "world": "balmung"})
	if d["raffles"].([]any)[0].(map[string]any)["entry"] == nil {
		t.Error("the entry wasn't matched case-insensitively on the whole name")
	}
	for _, probe := range []map[string]any{
		{"name": "Yao", "world": "Balmung"},        // a prefix is not a name
		{"name": "Yao Ming", "world": "Gilgamesh"}, // nor is the same name elsewhere
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

	entry := getRaffleEntryByName(t, env.store, int64(f.raffleID), "Yao Ming", "Balmung")
	if err := env.store.SetRaffleWinner(t.Context(), int64(f.raffleID), &entry.ID); err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetRaffleStatus(t.Context(), int64(f.raffleID), "closed"); err != nil {
		t.Fatal(err)
	}

	d := env.dashboard(t, f.mapID, map[string]any{"name": "Yao Ming", "world": "Balmung"})
	r := d["raffles"].([]any)[0].(map[string]any)
	if r["running"] != false {
		t.Error("a closed raffle reads as running")
	}
	if r["winner_name"] != "Yao Ming @ Balmung" || r["is_winner"] != true {
		t.Errorf("winner = %v (is_winner %v); want the participant", r["winner_name"], r["is_winner"])
	}
}

func TestFestivalDashboard_CardAndDrawingPagesLinkTheFestival(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	f := env.createFestival(t)
	cardToken, drawToken := env.signUpForFestival(t, f, "Yao Ming @ Balmung")
	want := fmt.Sprint(f.mapID) // no shortcode set, so the link is the id

	card := decodeBody(t, env.get(t, "/api/stamp-card/"+cardToken))
	if got := card["rally"].(map[string]any)["festival_link"]; got != want {
		t.Errorf("card festival_link = %v; want %q", got, want)
	}
	draw := decodeBody(t, env.get(t, "/api/garapon/"+drawToken))
	if got := draw["festival_link"]; got != want {
		t.Errorf("garapon festival_link = %v; want %q", got, want)
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
