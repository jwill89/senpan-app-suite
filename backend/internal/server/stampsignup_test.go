package server_test

import (
	"fmt"
	"net/http"
	"testing"
)

// -- Public self-service sign-up + link lookup --------------------------------

// createSignupRally creates a rally with the public sign-up opt-in ON and returns
// its id. Mirrors createRally, which leaves the opt-in off (the default).
func (e *testEnv) createSignupRally(t *testing.T, title string) int {
	t.Helper()
	resp := e.postJSON(t, "/api/stamp-rallies", map[string]any{
		"title":         title,
		"public_signup": true,
		"stamps": []map[string]any{
			{"image": "images/stamp_stamps/a.png", "password": "alpha",
				"placement": map[string]any{"x": 10, "y": 10, "width": 15, "height": 15}},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create signup rally status = %d; want 201", resp.StatusCode)
	}
	r := decodeBody(t, resp)["stamp_rally"].(map[string]any)
	if r["public_signup"] != true {
		t.Fatalf("public_signup = %v; want true (the opt-in must round-trip)", r["public_signup"])
	}
	return int(r["id"].(float64))
}

// signUp posts a public sign-up and returns the raw response for status checks.
func (e *testEnv) signUp(t *testing.T, rallyID int, name string) *http.Response {
	t.Helper()
	return e.postJSON(t, fmt.Sprintf("/api/stamp-signup/%d", rallyID), map[string]any{"name": name})
}

// lookup posts a public link lookup and returns its entries.
func (e *testEnv) lookup(t *testing.T, name string) []any {
	t.Helper()
	resp := e.postJSON(t, "/api/stamp-lookup", map[string]any{"name": name})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lookup status = %d; want 200", resp.StatusCode)
	}
	return decodeBody(t, resp)["entries"].([]any)
}

func TestStampSignup_ListsOnlyOptedInRallies(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	optedIn := env.createSignupRally(t, "Festival Rally")
	env.createRally(t, "Staff-issued Rally", false) // opt-in defaults off

	// The listing is public - no session needed.
	env2 := env
	resp := env2.get(t, "/api/stamp-signup")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d; want 200", resp.StatusCode)
	}
	rallies := decodeBody(t, resp)["rallies"].([]any)
	if len(rallies) != 1 {
		t.Fatalf("listed %d rallies; want only the opted-in one", len(rallies))
	}
	if got := int(rallies[0].(map[string]any)["id"].(float64)); got != optedIn {
		t.Errorf("listed rally id = %d; want %d", got, optedIn)
	}
}

func TestStampSignup_IssuesACard(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")

	resp := env.signUp(t, rallyID, "Yao Ming @ Balmung")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("signup status = %d; want 201", resp.StatusCode)
	}
	body := decodeBody(t, resp)
	token, _ := body["card_token"].(string)
	if token == "" {
		t.Fatal("signup returned no card token")
	}
	// A client that still submits one composed string is split server-side, so the
	// record matches what a raffle or a custom card would have stored for the same
	// person - which is the whole point of giving worlds their own column.
	if body["participant_name"] != "Yao Ming" {
		t.Errorf("participant_name = %v; want the name alone", body["participant_name"])
	}
	if body["world"] != "Balmung" {
		t.Errorf("world = %v; want the world split out of the submitted string", body["world"])
	}
	// No linked garapon -> the garapon fields stay absent.
	if _, ok := body["garapon_token"]; ok {
		t.Errorf("garapon_token present for a rally with no linked garapon: %v", body["garapon_token"])
	}

	// The token really addresses a usable public card.
	cardResp := env.get(t, "/api/stamp-card/"+token)
	if cardResp.StatusCode != http.StatusOK {
		t.Fatalf("public card status = %d; want 200", cardResp.StatusCode)
	}
	card := decodeBody(t, cardResp)
	// Stored apart, but the CARD still reads the way it always did - it is what a
	// participant holds up at a stall, and splitting the columns must not change
	// what they show.
	if card["participant_name"] != "Yao Ming @ Balmung" {
		t.Errorf("card participant = %v; want the composed label", card["participant_name"])
	}
}

func TestStampSignup_RejectsDuplicateName(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")

	if resp := env.signUp(t, rallyID, "Yao Ming"); resp.StatusCode != http.StatusCreated {
		t.Fatalf("first signup status = %d; want 201", resp.StatusCode)
	}
	// Same name -> 409, so a second person can't take over an existing card, and a
	// participant who signs up twice is told rather than silently given a new card.
	resp := env.signUp(t, rallyID, "Yao Ming")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate signup status = %d; want 409", resp.StatusCode)
	}
	// Case and surrounding space must not open a loophole: "yao  ming" trimmed and
	// folded is the same participant.
	resp = env.signUp(t, rallyID, "  YAO MING  ")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("case/space-variant signup status = %d; want 409", resp.StatusCode)
	}

	// A DIFFERENT rally is a separate namespace - the same name is fine there.
	other := env.createSignupRally(t, "Second Rally")
	if resp := env.signUp(t, other, "Yao Ming"); resp.StatusCode != http.StatusCreated {
		t.Fatalf("signup on a second rally status = %d; want 201", resp.StatusCode)
	}
}

func TestStampSignup_RejectsClosedAndOptedOutRallies(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// A rally that never opted in must look like it doesn't exist, so this endpoint
	// can't be used to enumerate staff-issued rallies.
	optedOut := env.createRally(t, "Staff-issued Rally", false)
	if resp := env.signUp(t, optedOut, "Yao Ming"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("signup on an opted-out rally = %d; want 404", resp.StatusCode)
	}
	if resp := env.signUp(t, 999999, "Yao Ming"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("signup on a missing rally = %d; want 404", resp.StatusCode)
	}

	// Closing an opted-in rally stops sign-ups.
	rallyID := env.createSignupRally(t, "Festival Rally")
	closeResp := env.postJSON(t, fmt.Sprintf("/api/stamp-rallies/%d/close", rallyID), map[string]any{})
	if closeResp.StatusCode != http.StatusOK {
		t.Fatalf("close status = %d; want 200", closeResp.StatusCode)
	}
	closeResp.Body.Close()
	if resp := env.signUp(t, rallyID, "Yao Ming"); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("signup on a closed rally = %d; want 400", resp.StatusCode)
	}
	// ...and drops it from the public list.
	listResp := env.get(t, "/api/stamp-signup")
	if n := len(decodeBody(t, listResp)["rallies"].([]any)); n != 0 {
		t.Errorf("closed rally still listed (%d entries)", n)
	}
}

func TestStampSignup_RequiresAName(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")

	if resp := env.signUp(t, rallyID, "   "); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("blank-name signup = %d; want 400", resp.StatusCode)
	}
	long := ""
	for range 61 {
		long += "x"
	}
	if resp := env.signUp(t, rallyID, long); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("over-long signup = %d; want 400", resp.StatusCode)
	}
}

func TestStampSignup_IssuesGaraponLinkWhenLinked(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")

	// Link an open garapon to the rally (the link lives on the garapon).
	garaponID := env.createGarapon(t, "Festival Garapon")
	linkResp := env.putJSON(t, fmt.Sprintf("/api/garapons/%d", garaponID), map[string]any{
		"title":          "Festival Garapon",
		"stamp_rally_id": rallyID,
		"prizes": []map[string]any{
			{"name": "Grand", "ball_color": "#e5b53f", "rate": 1, "is_grand": true},
		},
	})
	if linkResp.StatusCode != http.StatusOK {
		t.Fatalf("link garapon status = %d; want 200", linkResp.StatusCode)
	}
	linkResp.Body.Close()

	resp := env.signUp(t, rallyID, "Yao Ming")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("signup status = %d; want 201", resp.StatusCode)
	}
	body := decodeBody(t, resp)
	cardToken, _ := body["card_token"].(string)
	garaponToken, _ := body["garapon_token"].(string)
	if garaponToken == "" {
		t.Fatal("signup issued no garapon token despite a linked open garapon")
	}
	// They must NOT be the same string. While they were, the stamp-card link was
	// itself a spendable drawing token: a screenshotted card, or one pasted in
	// Discord, let anyone burn that participant's draws - which cannot be undone -
	// and any path that recovered a card link handed the draws over with it.
	if garaponToken == cardToken {
		t.Errorf("garapon token and card token are both %q; viewing a card and "+
			"spending its draws must be separate secrets", cardToken)
	}
	if body["garapon_title"] != "Festival Garapon" {
		t.Errorf("garapon_title = %v; want the linked garapon's title", body["garapon_title"])
	}

	// Both tokens address something real.
	if r := env.get(t, "/api/garapon/"+garaponToken); r.StatusCode != http.StatusOK {
		t.Errorf("public garapon status = %d; want 200", r.StatusCode)
	}
	if r := env.get(t, "/api/stamp-card/"+cardToken); r.StatusCode != http.StatusOK {
		t.Errorf("public card status = %d; want 200", r.StatusCode)
	}
	// ...and neither addresses the OTHER thing. This is the whole point of the
	// split: holding a card link must not reach the drawing endpoint.
	if r := env.get(t, "/api/garapon/"+cardToken); r.StatusCode != http.StatusNotFound {
		t.Errorf("garapon by CARD token = %d; want 404 - a card link must not draw",
			r.StatusCode)
	}
	if r := env.get(t, "/api/stamp-card/"+garaponToken); r.StatusCode != http.StatusNotFound {
		t.Errorf("card by GARAPON token = %d; want 404", r.StatusCode)
	}
}

// The lookup is keyed on a character name, which is public and appears on the
// entrant lists the app publishes by design. It therefore must not return anything
// SPENDABLE: it used to hand back the garapon token, so anyone who typed a name
// could burn that participant's draws, irreversibly, before the participant ever
// opened their own link.
//
// What it may return is the card token (opens the stamp card, and since the split
// nothing else) and the number of draws REMAINING, so someone on a borrowed device
// can see their draws are intact without being handed the means to spend them.
func TestStampLookup_NeverReturnsASpendableToken(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")
	garaponID := env.createGarapon(t, "Festival Garapon")
	linkResp := env.putJSON(t, fmt.Sprintf("/api/garapons/%d", garaponID), map[string]any{
		"title":          "Festival Garapon",
		"stamp_rally_id": rallyID,
		"prizes": []map[string]any{
			{"name": "Grand", "ball_color": "#e5b53f", "rate": 1, "is_grand": true},
		},
	})
	linkResp.Body.Close()

	signup := decodeBody(t, env.signUp(t, rallyID, "Aria Ashwood"))
	garaponToken, _ := signup["garapon_token"].(string)
	if garaponToken == "" {
		t.Fatal("signup issued no garapon token")
	}

	body := decodeBody(t, env.postJSON(t, "/api/stamp-lookup", map[string]any{"name": "Aria Ashwood"}))
	entries, _ := body["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("entries = %d; want 1", len(entries))
	}
	entry, _ := entries[0].(map[string]any)

	// The token itself must be absent - not blank, not present under another key.
	for key, value := range entry {
		if str, ok := value.(string); ok && str == garaponToken {
			t.Fatalf("lookup returned the drawing token under %q; a public name must "+
				"never yield a spendable capability", key)
		}
	}
	if _, present := entry["garapon_token"]; present {
		t.Error("lookup response still carries a garapon_token field")
	}

	// It still tells the participant their draws exist and are unspent.
	if entry["garapon_title"] != "Festival Garapon" {
		t.Errorf("garapon_title = %v; want the linked garapon's title", entry["garapon_title"])
	}
	if left, _ := entry["garapon_draws_left"].(float64); left != 1 {
		t.Errorf("garapon_draws_left = %v; want 1", entry["garapon_draws_left"])
	}

	// And the card token it DOES return cannot be used to draw.
	cardToken, _ := entry["card_token"].(string)
	if cardToken == "" {
		t.Fatal("lookup returned no card token")
	}
	resp := env.postJSON(t, "/api/garapon/"+cardToken+"/draw", map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("draw with the looked-up card token = %d; want 404", resp.StatusCode)
	}
}

func TestStampLookup_ReturnsOnlyExactMatches(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")

	resp := env.signUp(t, rallyID, "Yao Ming @ Balmung")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("signup status = %d; want 201", resp.StatusCode)
	}
	token := decodeBody(t, resp)["card_token"].(string)

	// Exact name (case-insensitively) -> the link comes back.
	entries := env.lookup(t, "yao ming @ balmung")
	if len(entries) != 1 {
		t.Fatalf("exact lookup returned %d entries; want 1", len(entries))
	}
	e := entries[0].(map[string]any)
	if e["card_token"] != token {
		t.Errorf("looked-up token = %v; want %v", e["card_token"], token)
	}
	if e["rally_title"] != "Festival Rally" {
		t.Errorf("rally_title = %v; want Festival Rally", e["rally_title"])
	}

	// Surrounding whitespace is trimmed, matching what sign-up stores - someone who
	// pastes their name with a trailing space still finds their own links.
	if got := env.lookup(t, "  Yao Ming @ Balmung  "); len(got) != 1 {
		t.Errorf("padded lookup returned %d entries; want 1", len(got))
	}

	// A PREFIX or fragment of the name must reveal nothing - the whole point of
	// exact matching is that the endpoint can't be walked to harvest other people's
	// links. "Yao Ming" is the interesting one: it is a real prefix of a real
	// participant, and the sort of thing a guesser would try first.
	for _, probe := range []string{"Yao", "Yao Ming", "ming", "Balmung"} {
		if got := env.lookup(t, probe); len(got) != 0 {
			t.Errorf("lookup(%q) returned %d entries; want 0", probe, len(got))
		}
	}
}

func TestStampLookup_MissRevealsNothing(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	env.createSignupRally(t, "Festival Rally")

	// An unknown name is a 200 with an empty list - NOT a 404, which would confirm
	// the difference between "no such participant" and "participant with no cards".
	resp := env.postJSON(t, "/api/stamp-lookup", map[string]any{"name": "Nobody At All"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("miss status = %d; want 200", resp.StatusCode)
	}
	if n := len(decodeBody(t, resp)["entries"].([]any)); n != 0 {
		t.Errorf("miss returned %d entries; want 0", n)
	}

	// A blank name is a client error rather than a match-everything query.
	blank := env.postJSON(t, "/api/stamp-lookup", map[string]any{"name": "  "})
	if blank.StatusCode != http.StatusBadRequest {
		t.Errorf("blank lookup status = %d; want 400", blank.StatusCode)
	}
	blank.Body.Close()
}

func TestStampLookup_SkipsClosedRallies(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")
	if resp := env.signUp(t, rallyID, "Yao Ming"); resp.StatusCode != http.StatusCreated {
		t.Fatalf("signup status = %d; want 201", resp.StatusCode)
	}
	if got := env.lookup(t, "Yao Ming"); len(got) != 1 {
		t.Fatalf("lookup before close returned %d entries; want 1", len(got))
	}

	closeResp := env.postJSON(t, fmt.Sprintf("/api/stamp-rallies/%d/close", rallyID), map[string]any{})
	closeResp.Body.Close()

	// A closed rally is over; its links stop being handed back out.
	if got := env.lookup(t, "Yao Ming"); len(got) != 0 {
		t.Errorf("lookup after close returned %d entries; want 0", len(got))
	}
}

func TestStampSignup_UsesTheGaraponsDefaultDrawAllowance(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createSignupRally(t, "Festival Rally")

	// A garapon configured to hand out three draws per link.
	garaponID := env.createGarapon(t, "Festival Garapon")
	linkResp := env.putJSON(t, fmt.Sprintf("/api/garapons/%d", garaponID), map[string]any{
		"title":          "Festival Garapon",
		"stamp_rally_id": rallyID,
		"default_draws":  3,
		"prizes": []map[string]any{
			{"name": "Grand", "ball_color": "#e5b53f", "rate": 1, "is_grand": true},
		},
	})
	if linkResp.StatusCode != http.StatusOK {
		t.Fatalf("configure garapon status = %d; want 200", linkResp.StatusCode)
	}
	linkResp.Body.Close()

	if resp := env.signUp(t, rallyID, "Yao Ming"); resp.StatusCode != http.StatusCreated {
		t.Fatalf("signup status = %d; want 201", resp.StatusCode)
	}

	// The self-issued link carries the garapon's default, not a hardcoded 1 - nobody
	// is present at a public sign-up to choose a number.
	players := env.garaponPlayers(t, garaponID)
	if len(players) != 1 {
		t.Fatalf("garapon has %d links; want 1", len(players))
	}
	if got := int(players[0]["max_draws"].(float64)); got != 3 {
		t.Errorf("self-issued link max_draws = %d; want the garapon default of 3", got)
	}

	// An admin issuing a link without a number gets the same default.
	_, _ = env.createGaraponPlayer(t, garaponID, "Staff Issued", 0)
	players = env.garaponPlayers(t, garaponID)
	for _, p := range players {
		if p["player_name"] == "Staff Issued" {
			if got := int(p["max_draws"].(float64)); got != 3 {
				t.Errorf("admin-issued link with no number = %d draws; want the default of 3", got)
			}
		}
	}
}

func TestGaraponDefaultDraws_FallsBackToOne(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	// A garapon created without the field (an older client, or just left blank) must
	// not end up issuing links that cannot draw at all.
	garaponID := env.createGarapon(t, "No Default Set")
	resp := env.get(t, fmt.Sprintf("/api/garapons/%d", garaponID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detail status = %d; want 200", resp.StatusCode)
	}
	g := decodeBody(t, resp)["garapon"].(map[string]any)
	if got := int(g["default_draws"].(float64)); got != 1 {
		t.Errorf("default_draws = %d; want 1 when unset", got)
	}
}

// garaponPlayers returns a garapon's drawing links from its detail endpoint.
func (e *testEnv) garaponPlayers(t *testing.T, garaponID int) []map[string]any {
	t.Helper()
	resp := e.get(t, fmt.Sprintf("/api/garapons/%d", garaponID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("garapon detail status = %d; want 200", resp.StatusCode)
	}
	raw, _ := decodeBody(t, resp)["players"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, p := range raw {
		out = append(out, p.(map[string]any))
	}
	return out
}
