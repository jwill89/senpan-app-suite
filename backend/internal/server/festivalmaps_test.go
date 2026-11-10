package server_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// -- Festival maps -------------------------------------------------------------
//
// A stall on a map is a PITCH (shape, color, placement) holding one or more
// OCCUPANTS (affiliate, title, offering, hours) - so a booth that changes hands
// between days is one shape with two names rather than two stalls stacked on the
// same coordinates. Most of what follows exercises that split.

func mapPathOf(id int) string { return fmt.Sprintf("/api/v1/festival-maps/%d", id) }

// occupant builds one occupant as a save sends it; over replaces fields of it.
func occupant(title, stallType string, over map[string]any) map[string]any {
	o := map[string]any{
		"title": title, "description": "", "eventCarrd": "", "stallType": stallType, "typeLabel": "",
		"times": []map[string]any{},
	}
	for k, v := range over {
		o[k] = v
	}
	return o
}

// pitch builds one pitch as a save sends it, holding the occupants.
func pitch(shape string, x, y, w, h float64, occupants ...map[string]any) map[string]any {
	if occupants == nil {
		occupants = []map[string]any{}
	}
	return map[string]any{
		"shape": shape, "color": "", "textColor": "", "selectionColor": "",
		"placement": placement(x, y, w, h), "occupants": occupants,
	}
}

// mapBody builds a full map as a save sends it; over replaces fields of it.
func mapBody(title string, over map[string]any) map[string]any {
	b := map[string]any{
		"title": title, "slug": "", "description": "", "times": []map[string]any{},
		"mapImage": "images/festival_maps/plan.png", "stalls": []map[string]any{},
	}
	for k, v := range over {
		b[k] = v
	}
	return b
}

// createMap posts a create with two single-occupant pitches (one game circle, one
// food rectangle) and returns the new map's id.
func (e *testEnv) createMap(t *testing.T, title string) int {
	t.Helper()
	first := pitch("circle", 10, 10, 12, 12, occupant("The Green Gaelicat", "game", nil))
	first["color"] = "#AABBCC"
	var made map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/festival-maps", mapBody(title, map[string]any{
		"description": "**Obon** is here.",
		"times":       []map[string]any{{"label": "Day 1", "start": "2026-08-01T18:00:00Z", "end": "2026-08-01T22:00:00Z"}},
		"stalls":      []map[string]any{first, pitch("rect", 40, 30, 18, 10, occupant("Flora Teahouse", "food", nil))},
	})), http.StatusCreated, &made)
	return int(made["id"].(float64))
}

// mapDetail fetches a map's staff detail.
func (e *testEnv) mapDetail(t *testing.T, id int) map[string]any {
	t.Helper()
	var m map[string]any
	decodeInto(t, e.get(t, mapPathOf(id)), http.StatusOK, &m)
	return m
}

// stallsOf returns a map detail's pitches.
func stallsOf(m map[string]any) []any { return m["stalls"].([]any) }

// occupantsOf returns one pitch's occupants.
func occupantsOf(stall any) []any { return stall.(map[string]any)["occupants"].([]any) }

// firstOccupant returns a pitch's first occupant.
func firstOccupant(stall any) map[string]any { return occupantsOf(stall)[0].(map[string]any) }

// publishMap publishes a map, failing the test when the server refuses.
func (e *testEnv) publishMap(t *testing.T, id int) {
	t.Helper()
	decodeInto(t, e.patchJSON(t, mapPathOf(id), map[string]any{"status": "published"}), http.StatusOK, nil)
}

// saveMap replaces a map, answering the response.
func (e *testEnv) saveMap(t *testing.T, id int, body map[string]any) *http.Response {
	t.Helper()
	return e.putJSON(t, mapPathOf(id), body)
}

// roundTrip replaces a map with what its detail returned, changing only the
// description: the shape of an ordinary "fix a typo" edit.
func (e *testEnv) roundTrip(t *testing.T, id int, description string) *http.Response {
	t.Helper()
	d := e.mapDetail(t, id)
	return e.saveMap(t, id, mapBody(d["title"].(string), map[string]any{
		"slug": d["slug"], "mapImage": d["mapImage"], "description": description, "times": d["times"],
		"stalls": inputStalls(d),
	}))
}

// inputStalls turns a detail's pitches into what a save takes back: the same, less
// the fields only an answer carries.
func inputStalls(d map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, raw := range stallsOf(d) {
		st := raw.(map[string]any)
		occupants := []map[string]any{}
		for _, o := range occupantsOf(st) {
			occ := o.(map[string]any)
			occupants = append(occupants, map[string]any{
				"id": occ["id"], "affiliateId": occ["affiliateId"], "title": occ["title"], "description": occ["description"],
				"eventCarrd": occ["eventCarrd"], "stallType": occ["stallType"], "typeLabel": occ["typeLabel"], "times": occ["times"],
			})
		}
		out = append(out, map[string]any{
			"id": st["id"], "shape": st["shape"], "color": st["color"], "textColor": st["textColor"],
			"selectionColor": st["selectionColor"], "placement": st["placement"], "occupants": occupants,
		})
	}
	return out
}

// problemOf decodes a problem details body.
func problemOf(t *testing.T, resp *http.Response) map[string]any { return decodeBody(t, resp) }

func TestFestivalMap_Guards(t *testing.T) {
	env := newTestEnv(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/festival-maps"},
		{http.MethodPost, "/api/v1/festival-maps"},
		{http.MethodGet, "/api/v1/festival-maps/1"},
		{http.MethodPut, "/api/v1/festival-maps/1"},
		{http.MethodPatch, "/api/v1/festival-maps/1"},
		{http.MethodDelete, "/api/v1/festival-maps/1"},
	} {
		resp := reqAs(t, env.client, env, route.method, route.path, map[string]any{})
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s signed out = %d; want 401", route.method, route.path, resp.StatusCode)
		}
	}

	env.loginAdmin(t)
	id := env.createMap(t, "Obon")
	mapsOnly := makeActiveUser(t, env, "mapper", "correct horse battery", []string{"festival-map"})
	emaOnly := makeActiveUser(t, env, "emaonly", "correct horse battery", []string{"festival-ema"})
	other := makeActiveUser(t, env, "other", "correct horse battery", []string{"bingo-presets"})

	// The list is open to the Ema page, whose board editor picks a festival from it;
	// everything else about a map is the map page's.
	for who, c := range map[string]*http.Client{"festival-map": mapsOnly, "festival-ema": emaOnly} {
		resp := getAs(t, c, env, "/api/v1/festival-maps")
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("list with %s = %d; want 200", who, resp.StatusCode)
		}
	}
	for _, path := range []string{"/api/v1/festival-maps", mapPathOf(id)} {
		resp := getAs(t, other, env, path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s without access = %d; want 403", path, resp.StatusCode)
		}
	}
	resp := getAs(t, emaOnly, env, mapPathOf(id))
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a map's detail for the Ema page = %d; want 403", resp.StatusCode)
	}
	resp = postAs(t, emaOnly, env, "/api/v1/festival-maps", mapBody("Nope", nil))
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("creating a map for the Ema page = %d; want 403", resp.StatusCode)
	}
}

func TestFestivalMap_CreateStartsInProgressAndNormalizes(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createMap(t, "Obon Matsuri 2026")
	m := env.mapDetail(t, id)

	// A new map is never live until someone publishes it deliberately.
	if m["status"] != "in_progress" {
		t.Errorf("status = %v; want in_progress", m["status"])
	}
	if m["publicPath"] != fmt.Sprint(id) {
		t.Errorf("publicPath = %v; want the id while there is no shortcode", m["publicPath"])
	}
	stalls := stallsOf(m)
	if len(stalls) != 2 {
		t.Fatalf("stalls = %d; want 2", len(stalls))
	}
	first := stalls[0].(map[string]any)
	if first["shape"] != "circle" {
		t.Errorf("first pitch shape = %v; want circle", first["shape"])
	}
	// Colors are stored lowercase so the value is a literal the stylesheet can use.
	if first["color"] != "#aabbcc" {
		t.Errorf("color = %v; want #aabbcc", first["color"])
	}
	if got := firstOccupant(stalls[0])["stallType"]; got != "game" {
		t.Errorf("first occupant type = %v; want game", got)
	}
	times := m["times"].([]any)
	if len(times) != 1 || times[0].(map[string]any)["label"] != "Day 1" {
		t.Errorf("times = %v; want one Day 1 range", times)
	}
	if end := times[0].(map[string]any)["end"]; end != "2026-08-01T22:00:00Z" {
		t.Errorf("end = %v; want it kept in the stored form", end)
	}
}

func TestFestivalMap_SaveKeepsWhatItCanAndRefusesWhatItCannot(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// A shape or a kind that doesn't exist is refused by the schema.
	for name, body := range map[string]map[string]any{
		"shape": mapBody("Odd", map[string]any{"stalls": []map[string]any{
			pitch("triangle", 10, 10, 10, 10, occupant("Weird", "food", nil))}}),
		"kind": mapBody("Odd", map[string]any{"stalls": []map[string]any{
			pitch("rect", 10, 10, 10, 10, occupant("Weird", "hovercraft", nil))}}),
		"no pitches key": {"title": "Odd", "slug": "", "description": "", "times": []any{}, "mapImage": ""},
	} {
		resp := env.postJSON(t, "/api/v1/festival-maps", body)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("a map with a bad %s = %d; want 422", name, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// A color that isn't a hex is dropped, a placement is clamped into the box, and a
	// range with no start says nothing and goes.
	bad := pitch("rect", -20, 130, 0, 0, occupant("Weird", "other", map[string]any{
		"times": []map[string]any{{"label": "Ghost", "start": ""}},
	}))
	bad["color"] = "red; background: url(x)"
	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/festival-maps", mapBody("Odd", map[string]any{
		"stalls": []map[string]any{bad},
	})), http.StatusCreated, &made)
	stall := stallsOf(made)[0].(map[string]any)
	if stall["color"] != "" {
		t.Errorf("color = %q; want it dropped", stall["color"])
	}
	p := stall["placement"].(map[string]any)
	if p["x"].(float64) != 0 || p["y"].(float64) != 100 || p["width"].(float64) <= 0 {
		t.Errorf("placement = %v; want clamped into the map box with a non-zero size", p)
	}
	if times := firstOccupant(stall)["times"].([]any); len(times) != 0 {
		t.Errorf("times = %v; want the startless range dropped", times)
	}
}

func TestFestivalMap_DropsEmptyOccupantsAndPitches(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/festival-maps", mapBody("Sparse", map[string]any{
		"stalls": []map[string]any{
			pitch("rect", 10, 10, 12, 8,
				occupant("Real Stall", "food", nil),
				// Neither a title nor an affiliate: the row the form leaves behind when
				// someone adds an occupant and thinks better of it.
				occupant("   ", "other", nil)),
			// A pitch nobody occupies is a colored box with nothing to say.
			pitch("rect", 60, 10, 12, 8),
		},
	})), http.StatusCreated, &made)

	stalls := stallsOf(made)
	if len(stalls) != 1 {
		t.Fatalf("pitches = %d; want the occupant-less one dropped", len(stalls))
	}
	if occ := occupantsOf(stalls[0]); len(occ) != 1 {
		t.Errorf("occupants = %d; want the empty one dropped", len(occ))
	}
}

func TestFestivalMap_PublishNeedsAMapImage(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/festival-maps", mapBody("No art", map[string]any{"mapImage": ""})),
		http.StatusCreated, &made)
	id := int(made["id"].(float64))

	// Stalls are positioned as a share of the base image's box, so publishing without
	// one would show the public a plan of floating shapes.
	resp := env.patchJSON(t, mapPathOf(id), map[string]any{"status": "published"})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("publish status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
	var list map[string]any
	decodeInto(t, env.get(t, "/api/v1/festival-maps/public"), http.StatusOK, &list)
	if maps := list["maps"].([]any); len(maps) != 0 {
		t.Errorf("public maps = %v; want none", maps)
	}

	// A status that isn't one is refused, and an unknown map is a 404.
	resp = env.patchJSON(t, mapPathOf(id), map[string]any{"status": "live"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("unknown status = %d; want 422", resp.StatusCode)
	}
	resp.Body.Close()
	resp = env.patchJSON(t, mapPathOf(999), map[string]any{"status": "closed"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("patching an unknown map = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestFestivalMap_UnknownIDsAre404(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	for _, c := range []struct{ method, tail string }{
		{http.MethodGet, ""}, {http.MethodPut, ""}, {http.MethodDelete, ""},
	} {
		var body any
		if c.method == http.MethodPut {
			body = mapBody("Ghost", nil)
		}
		resp := env.reqJSON(t, c.method, mapPathOf(999), body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s of an unknown map = %d; want 404", c.method, resp.StatusCode)
		}
	}
}

func TestFestivalMap_UpdateKeepsOccupantIDsAndDropsRemoved(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createMap(t, "Obon Matsuri 2026")
	stalls := stallsOf(env.mapDetail(t, id))
	keptStall := int(stalls[0].(map[string]any)["id"].(float64))
	keptOccupant := int(firstOccupant(stalls[0])["id"].(float64))

	// Re-save with the first pitch (by id) renamed and the second dropped.
	kept := pitch("circle", 10, 10, 12, 12, occupant("The Green Gaelicat (Day 2)", "game", map[string]any{"id": keptOccupant}))
	kept["id"] = keptStall
	var after map[string]any
	decodeInto(t, env.saveMap(t, id, mapBody("Obon Matsuri 2026", map[string]any{"stalls": []map[string]any{kept}})),
		http.StatusOK, &after)

	pitches := stallsOf(after)
	if len(pitches) != 1 {
		t.Fatalf("pitches after update = %d; want 1", len(pitches))
	}
	occ := firstOccupant(pitches[0])
	if int(occ["id"].(float64)) != keptOccupant {
		t.Errorf("occupant id = %v; want %d preserved so a rally keeps naming it", occ["id"], keptOccupant)
	}
	if occ["title"] != "The Green Gaelicat (Day 2)" {
		t.Errorf("title = %v; want the renamed occupant", occ["title"])
	}
}

// -- Multi-day pitches --------------------------------------------------------

// createDayShare builds a map with one pitch occupied by two businesses on
// different days, returning the map id.
func (e *testEnv) createDayShare(t *testing.T) int {
	t.Helper()
	day := func(label, d string) map[string]any {
		return map[string]any{"label": label, "start": "2026-08-0" + d + "T00:00:00Z", "end": "2026-08-0" + d + "T23:59:00Z"}
	}
	var made map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/festival-maps", mapBody("Obon Matsuri 2026", map[string]any{
		"times": []map[string]any{day("Day 1", "1"), day("Day 2", "2")},
		"stalls": []map[string]any{pitch("rect", 30, 40, 14, 8,
			occupant("Flora Teahouse", "game", map[string]any{"times": []map[string]any{day("Day 1", "1")}}),
			occupant("The Great Below", "both", map[string]any{"times": []map[string]any{day("Day 2", "2")}}),
		)},
	})), http.StatusCreated, &made)
	return int(made["id"].(float64))
}

// -- Saves that must not reach past their map ------------------------------------

func TestFestivalMap_AffiliateDeleteKeepsPitch(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// An occupant may name an affiliate and nothing else: a blank title, its identity
	// carried by the link. Deleting the affiliate must not leave it nameless, or the
	// next unrelated save would drop it as an empty row, and its pitch with it.
	affID := env.createAffiliate(t, "Flora's Fancies")
	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/festival-maps", mapBody("Obon Matsuri", map[string]any{
		"stalls": []map[string]any{
			pitch("rect", 10, 10, 12, 8, occupant("", "food", map[string]any{"affiliateId": affID})),
			pitch("rect", 60, 10, 12, 8, occupant("Other Stall", "game", nil)),
		},
	})), http.StatusCreated, &made)
	id := int(made["id"].(float64))

	if r := env.del(t, fmt.Sprintf("/api/v1/affiliates/%d", affID)); r.StatusCode != http.StatusNoContent {
		t.Fatalf("affiliate delete status = %d; want 204", r.StatusCode)
	} else {
		r.Body.Close()
	}
	stalls := stallsOf(env.mapDetail(t, id))
	if len(stalls) != 2 {
		t.Fatalf("pitches after affiliate delete = %d; want 2", len(stalls))
	}
	if got := firstOccupant(stalls[0])["title"]; got != "Flora's Fancies" {
		t.Errorf("occupant title after affiliate delete = %v; want the affiliate's name carried down", got)
	}
	decodeInto(t, env.roundTrip(t, id, "Obon is here!"), http.StatusOK, nil)
	if after := stallsOf(env.mapDetail(t, id)); len(after) != 2 {
		t.Errorf("pitches after an unrelated save = %d; want 2 (a pitch was silently deleted)", len(after))
	}
}

func TestFestivalMap_SaveCannotReachAnotherMapsStall(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	victimID := env.createMap(t, "Victim Festival")
	attackerID := env.createMap(t, "Attacker Festival")
	victim := stallsOf(env.mapDetail(t, victimID))
	wantOccupant := firstOccupant(victim[0])["title"]

	// Save the attacker's map claiming one of its pitches IS the victim's: the editor
	// is stale or the id spoofed, and nothing is saved.
	hijack := pitch("rect", 1, 1, 5, 5, occupant("Hijacked", "food", nil))
	hijack["id"] = victim[0].(map[string]any)["id"]
	resp := env.saveMap(t, attackerID, mapBody("Attacker Festival", map[string]any{"stalls": []map[string]any{hijack}}))
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("cross-map save status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()

	after := stallsOf(env.mapDetail(t, victimID))
	if len(after) != len(victim) || firstOccupant(after[0])["title"] != wantOccupant {
		t.Errorf("the victim map changed: %d pitches, first occupant %v", len(after), firstOccupant(after[0])["title"])
	}
	// The attacker's own pitches stand as they were: the rejected save applied nothing.
	if got := len(stallsOf(env.mapDetail(t, attackerID))); got != 2 {
		t.Errorf("attacker pitches = %d; want its 2 untouched", got)
	}

	// An occupant id that isn't on the pitch it is saved under is stale in the same way.
	own := stallsOf(env.mapDetail(t, attackerID))
	foreign := firstOccupant(victim[0])["id"]
	stale := pitch("rect", 1, 1, 5, 5, occupant("Mixed up", "food", map[string]any{"id": foreign}))
	stale["id"] = own[0].(map[string]any)["id"]
	resp = env.saveMap(t, attackerID, mapBody("Attacker Festival", map[string]any{"stalls": []map[string]any{stale}}))
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("save with another pitch's occupant = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestFestivalMap_AStallsKeyIsRequiredAndEmptyClears(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createMap(t, "Obon Matsuri")
	d := env.mapDetail(t, id)

	// A save with no "stalls" at all - the shape of a list row - is refused rather than
	// read as "delete them all".
	resp := env.saveMap(t, id, map[string]any{
		"title": d["title"], "slug": d["slug"], "mapImage": d["mapImage"], "description": "Typo fixed", "times": d["times"],
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("save without stalls status = %d; want 422", resp.StatusCode)
	}
	resp.Body.Close()
	if got := len(stallsOf(env.mapDetail(t, id))); got != 2 {
		t.Errorf("pitches after the refused save = %d; want 2 untouched", got)
	}

	// An explicit empty list still means "this map has no pitches".
	var emptied map[string]any
	decodeInto(t, env.saveMap(t, id, mapBody("Obon Matsuri", nil)), http.StatusOK, &emptied)
	if got := len(stallsOf(emptied)); got != 0 {
		t.Errorf("pitches after an explicit empty save = %d; want 0", got)
	}
}

func TestFestivalMap_OnePitchHoldsSeveralOccupants(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createDayShare(t)
	stalls := stallsOf(env.mapDetail(t, id))

	// One shape on the plan, not two stacked on the same coordinates.
	if len(stalls) != 1 {
		t.Fatalf("pitches = %d; want 1", len(stalls))
	}
	occupants := occupantsOf(stalls[0])
	if len(occupants) != 2 {
		t.Fatalf("occupants = %d; want 2", len(occupants))
	}
	// Display order is the order they were sent in.
	if got := occupants[0].(map[string]any)["title"]; got != "Flora Teahouse" {
		t.Errorf("first occupant = %v; want Flora Teahouse", got)
	}
	if got := occupants[1].(map[string]any)["title"]; got != "The Great Below" {
		t.Errorf("second occupant = %v; want The Great Below", got)
	}
	// Each keeps its own days - that is what decides who the map leads with.
	if times := occupants[1].(map[string]any)["times"].([]any); len(times) != 1 ||
		times[0].(map[string]any)["label"] != "Day 2" {
		t.Errorf("second occupant times = %v; want its own Day 2 range", times)
	}
}

func TestFestivalMap_PublicPitchCarriesEveryOccupant(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createDayShare(t)
	env.publishMap(t, id)

	detail := decodeBody(t, env.get(t, fmt.Sprintf("/api/v1/festival-maps/public/%d", id)))
	stalls := detail["stalls"].([]any)
	if len(stalls) != 1 {
		t.Fatalf("public pitches = %d; want 1", len(stalls))
	}
	occupants := occupantsOf(stalls[0])
	if len(occupants) != 2 {
		t.Fatalf("public occupants = %d; want 2", len(occupants))
	}
	// The festival ran in 2026-08; neither day is open now, so both read closed
	// rather than one being silently promoted.
	for _, o := range occupants {
		if o.(map[string]any)["isOpen"].(bool) {
			t.Errorf("occupant %v is open outside the festival", o)
		}
	}
}

// -- Public map ---------------------------------------------------------------

func TestFestivalMap_PublicShowsPublishedOnly(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	draft := env.createMap(t, "Draft Festival")
	live := env.createMap(t, "Obon Matsuri 2026")
	env.publishMap(t, live)

	body := decodeBody(t, env.get(t, "/api/v1/festival-maps/public"))
	maps := body["maps"].([]any)
	if len(maps) != 1 || maps[0].(map[string]any)["title"] != "Obon Matsuri 2026" {
		t.Fatalf("public list = %v; want only the published map", maps)
	}
	if count := maps[0].(map[string]any)["stallCount"].(float64); count != 2 {
		t.Errorf("stallCount = %v; want 2", count)
	}

	// A draft's existence isn't something the public endpoint should confirm.
	resp := env.get(t, fmt.Sprintf("/api/v1/festival-maps/public/%d", draft))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("draft public detail status = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()

	detail := decodeBody(t, env.get(t, fmt.Sprintf("/api/v1/festival-maps/public/%d", live)))
	if len(detail["stalls"].([]any)) != 2 {
		t.Errorf("public pitches = %v; want 2", detail["stalls"])
	}
	// No rally is linked, so no occupant carries a badge.
	for _, s := range detail["stalls"].([]any) {
		if firstOccupant(s)["inStampRally"].(bool) {
			t.Errorf("occupant %v is badged with no rally linked", s)
		}
	}
}

func TestFestivalMap_PublicRequiresNoAuth(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createMap(t, "Obon Matsuri 2026")
	env.publishMap(t, id)

	// The same server reached by a client with no cookie jar - so no session
	// travels with the request and the endpoint has to stand on its own.
	anon := &http.Client{Transport: env.client.Transport}
	resp, err := anon.Get(env.url("/api/v1/festival-maps/public"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("anonymous public list status = %d; want 200", resp.StatusCode)
	}
}

// -- Shortcodes ---------------------------------------------------------------

// createMapWithSlug posts a create carrying a shortcode and returns the response.
func (e *testEnv) createMapWithSlug(t *testing.T, title, slug string) *http.Response {
	t.Helper()
	return e.postJSON(t, "/api/v1/festival-maps", mapBody(title, map[string]any{"slug": slug}))
}

func TestFestivalMap_SlugIsNormalizedOnSave(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Whatever the admin types is settled into the stored form rather than bounced
	// back as invalid: trimmed, lowercased, spaces and underscores folded to dashes.
	resp := env.createMapWithSlug(t, "Obon Matsuri 2026", "  Obon _ Matsuri  2026 ")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d; want 201", resp.StatusCode)
	}
	m := decodeBody(t, resp)
	if m["slug"] != "obon-matsuri-2026" {
		t.Errorf("slug = %v; want obon-matsuri-2026", m["slug"])
	}
}

func TestFestivalMap_SlugRejectsBadShortcodes(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// All-numeric would be indistinguishable from a map id in the public path.
	bad := map[string]string{"numeric": "2026", "too short": "a", "punctuation": "obon!2026"}
	for name, slug := range bad {
		resp := env.createMapWithSlug(t, "Map "+name, slug)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("create with %s shortcode %q = %d; want 422", name, slug, resp.StatusCode)
		}
		problem := problemOf(t, resp)
		if errs, _ := problem["errors"].([]any); len(errs) == 0 || errs[0].(map[string]any)["location"] != "body.slug" {
			t.Errorf("problem = %v; want the error at body.slug", problem)
		}
	}
}

func TestFestivalMap_SlugMustBeUnique(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	first := env.createMapWithSlug(t, "Obon 2026", "obon-2026")
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first create status = %d; want 201", first.StatusCode)
	}
	id := int(decodeBody(t, first)["id"].(float64))

	second := env.createMapWithSlug(t, "Obon 2027", "obon-2026")
	if second.StatusCode != http.StatusConflict {
		t.Errorf("duplicate shortcode = %d; want 409", second.StatusCode)
	}
	second.Body.Close()

	// A map may of course keep its own shortcode across an edit.
	resp := env.saveMap(t, id, mapBody("Obon 2026", map[string]any{"slug": "obon-2026"}))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("re-saving its own shortcode = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Several maps with NO shortcode must not collide - '' means "not set", which
	// is why the unique index is partial.
	for i := range 2 {
		resp := env.createMapWithSlug(t, fmt.Sprintf("Unslugged %d", i), "")
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("create without a shortcode = %d; want 201", resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestFestivalMap_PublicResolvesShortcodeAndID(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createMap(t, "Obon Matsuri 2026")
	d := env.mapDetail(t, id)
	decodeInto(t, env.saveMap(t, id, mapBody("Obon Matsuri 2026", map[string]any{"slug": "obon-2026", "stalls": inputStalls(d)})),
		http.StatusOK, nil)
	env.publishMap(t, id)

	// Both forms resolve to the same map, so a link posted before the shortcode
	// existed keeps working.
	for _, key := range []string{"obon-2026", fmt.Sprint(id)} {
		body := decodeBody(t, env.get(t, "/api/v1/festival-maps/public/"+key))
		if int(body["id"].(float64)) != id {
			t.Errorf("public/%s resolved to id %v; want %d", key, body["id"], id)
		}
		if body["slug"] != "obon-2026" {
			t.Errorf("public/%s slug = %v; want obon-2026", key, body["slug"])
		}
	}

	// An unknown shortcode is a 404, not a 500 or an arbitrary map.
	miss := env.get(t, "/api/v1/festival-maps/public/hanami-2027")
	if miss.StatusCode != http.StatusNotFound {
		t.Errorf("unknown shortcode = %d; want 404", miss.StatusCode)
	}
	miss.Body.Close()
}

func TestFestivalMap_PublicShortcodeStillRespectsPublishing(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.createMapWithSlug(t, "Draft Festival", "draft-2026")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d; want 201", resp.StatusCode)
	}
	resp.Body.Close()

	// A shortcode is not a capability - an unpublished map stays 404 either way.
	miss := env.get(t, "/api/v1/festival-maps/public/draft-2026")
	if miss.StatusCode != http.StatusNotFound {
		t.Errorf("unpublished map by shortcode = %d; want 404", miss.StatusCode)
	}
	miss.Body.Close()
}

// -- Stamp Rally linkage ------------------------------------------------------

func TestFestivalMap_RallyOccupantLinkBadgesThePublicMap(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createDayShare(t)
	env.publishMap(t, mapID)
	occupants := occupantsOf(stallsOf(env.mapDetail(t, mapID))[0])
	day1 := int(occupants[0].(map[string]any)["id"].(float64))

	// The rally names Flora (Day 1) - not the pitch, which The Great Below also
	// occupies on Day 2 with a different activity.
	rallyID := env.createRally(t, "Obon Stamp Rally", map[string]any{
		"festivalMapId": mapID,
		"stamps":        []map[string]any{stampBody("alpha", map[string]any{"occupantId": day1, "stampType": "game"})},
	})

	detail := decodeBody(t, env.get(t, fmt.Sprintf("/api/v1/festival-maps/public/%d", mapID)))
	if detail["stampRallyTitle"] != "Obon Stamp Rally" {
		t.Errorf("stampRallyTitle = %v; want the linked rally", detail["stampRallyTitle"])
	}
	pitchOccupants := occupantsOf(detail["stalls"].([]any)[0])
	flora := pitchOccupants[0].(map[string]any)
	below := pitchOccupants[1].(map[string]any)
	if !flora["inStampRally"].(bool) {
		t.Error("Flora carries the rally stamp but isn't badged")
	}
	if flora["stampImage"] != "images/stamp_stamps/a.png" {
		t.Errorf("stampImage = %v; want the stamp art", flora["stampImage"])
	}
	// The badge follows the OCCUPANT, not the pitch they share.
	if below["inStampRally"].(bool) {
		t.Error("The Great Below is badged for a stamp that belongs to Flora")
	}

	// The rally's own stall name now comes from the occupant.
	rally, _ := env.rallyDetail(t, rallyID)
	stamp := rally["stamps"].([]any)[0].(map[string]any)
	if stamp["stallName"] != "Flora Teahouse" {
		t.Errorf("stallName = %v; want the occupant's title", stamp["stallName"])
	}
}

func TestFestivalMap_UnlinkedRallyClearsOccupantIDs(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createMap(t, "Obon Matsuri 2026")
	occID := int(firstOccupant(stallsOf(env.mapDetail(t, mapID))[0])["id"].(float64))

	// An occupant id on a rally that names no map would claim a link it doesn't have.
	rallyID := env.createRally(t, "Unlinked Rally", map[string]any{
		"stamps": []map[string]any{stampBody("alpha", map[string]any{"occupantId": occID})},
	})

	rally, _ := env.rallyDetail(t, rallyID)
	stamp := rally["stamps"].([]any)[0].(map[string]any)
	if stamp["occupantId"] != nil {
		t.Errorf("occupantId = %v; want it cleared on an unlinked rally", stamp["occupantId"])
	}
	if stamp["stallName"] != "" {
		t.Errorf("stallName = %v; want empty", stamp["stallName"])
	}
}

func TestFestivalMap_DeleteUnlinksTheRally(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createMap(t, "Obon Matsuri 2026")
	occID := int(firstOccupant(stallsOf(env.mapDetail(t, mapID))[0])["id"].(float64))
	rallyID := env.createRally(t, "Obon Stamp Rally", map[string]any{
		"festivalMapId": mapID,
		"stamps":        []map[string]any{stampBody("alpha", map[string]any{"occupantId": occID})},
	})

	resp := env.del(t, fmt.Sprintf("/api/v1/festival-maps/%d", mapID))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete map status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// The rally survives the map it was authored against.
	rally, _ := env.rallyDetail(t, rallyID)
	if rally["festivalMapId"] != nil {
		t.Errorf("festivalMapId = %v; want it cleared", rally["festivalMapId"])
	}
	stamp := rally["stamps"].([]any)[0].(map[string]any)
	if stamp["occupantId"] != nil {
		t.Errorf("occupantId = %v; want it cleared", stamp["occupantId"])
	}
	// The stamp still works - it just falls back to the venue's own name.
	if stamp["password"] != "alpha" {
		t.Errorf("password = %v; want the stamp intact", stamp["password"])
	}
}

func TestFestivalMap_RemovingAnOccupantClearsItsRallyStamp(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createDayShare(t)
	stall := stallsOf(env.mapDetail(t, mapID))[0].(map[string]any)
	stallID := int(stall["id"].(float64))
	occupants := occupantsOf(stall)
	keptID := int(occupants[0].(map[string]any)["id"].(float64))
	droppedID := int(occupants[1].(map[string]any)["id"].(float64))

	rallyID := env.createRally(t, "Obon Stamp Rally", map[string]any{
		"festivalMapId": mapID,
		"stamps": []map[string]any{
			stampBody("alpha", map[string]any{"occupantId": keptID}),
			stampBody("beta", map[string]any{"occupantId": droppedID, "placement": placement(40, 10, 15, 15)}),
		},
	})

	// Re-save the map with only the Day 1 occupant.
	only := pitch("rect", 30, 40, 14, 8, occupant("Flora Teahouse", "game", map[string]any{"id": keptID}))
	only["id"] = stallID
	decodeInto(t, env.saveMap(t, mapID, mapBody("Obon Matsuri 2026", map[string]any{"stalls": []map[string]any{only}})),
		http.StatusOK, nil)

	after, _ := env.rallyDetail(t, rallyID)
	stamps := after["stamps"].([]any)
	if len(stamps) != 2 {
		t.Fatalf("stamps = %d; want both kept - a stamp outlives the occupant it named", len(stamps))
	}
	var kept, orphaned map[string]any
	for _, raw := range stamps {
		st := raw.(map[string]any)
		if st["password"] == "alpha" {
			kept = st
		} else {
			orphaned = st
		}
	}
	if kept["occupantId"] == nil {
		t.Error("the surviving occupant's stamp lost its link")
	}
	// The dropped occupant's stamp falls back to its affiliate rather than
	// pointing at a row that is gone.
	if orphaned["occupantId"] != nil {
		t.Errorf("occupantId = %v; want it cleared with the occupant", orphaned["occupantId"])
	}
	if orphaned["stallName"] != "" {
		t.Errorf("stallName = %v; want empty once the occupant is gone", orphaned["stallName"])
	}
}

// -- Raffles pinned to a stall ------------------------------------------------

// assignRaffle creates a raffle filed under a map and pinned to one of its
// occupants, returning the new raffle's id. `over` merges extra raffle fields.
func (e *testEnv) assignRaffle(t *testing.T, title string, mapID, occupantID int, over map[string]any) int {
	t.Helper()
	body := map[string]any{
		"title": title, "maxEntries": 1, "entryMode": "details",
		"festivalMapId": mapID, "occupantId": occupantID,
	}
	for k, v := range over {
		body[k] = v
	}
	return e.createRaffle(t, body)
}

// stallRaffle returns the `raffle` block on a published map's first occupant of
// the pitch at `index`, or nil when that occupant carries none.
func (e *testEnv) stallRaffle(t *testing.T, mapID, index int) map[string]any {
	t.Helper()
	detail := decodeBody(t, e.get(t, fmt.Sprintf("/api/v1/festival-maps/public/%d", mapID)))
	occ := firstOccupant(detail["stalls"].([]any)[index])
	if raw, ok := occ["raffle"]; ok && raw != nil {
		return raw.(map[string]any)
	}
	return nil
}

func TestFestivalMap_RunningRaffleLinksFromItsStall(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createMap(t, "Obon Matsuri 2026")
	env.publishMap(t, mapID)
	stalls := stallsOf(env.mapDetail(t, mapID))
	occupantID := int(firstOccupant(stalls[0])["id"].(float64))

	raffleID := env.assignRaffle(t, "Gaelicat Raffle", mapID, occupantID,
		map[string]any{"prizeImage": "images/raffles/prize.png"})

	raffle := env.stallRaffle(t, mapID, 0)
	if raffle == nil {
		t.Fatal("the stall carries a running raffle but the map doesn't link it")
	}
	if int(raffle["id"].(float64)) != raffleID {
		t.Errorf("raffle id = %v; want %d", raffle["id"], raffleID)
	}
	if raffle["title"] != "Gaelicat Raffle" || raffle["prizeImage"] != "images/raffles/prize.png" {
		t.Errorf("raffle block = %v; want the title + prize art", raffle)
	}
	// The badge follows the OCCUPANT: the other pitch carries nothing.
	if other := env.stallRaffle(t, mapID, 1); other != nil {
		t.Errorf("second pitch linked raffle %v; want none", other)
	}
}

func TestFestivalMap_ClosedOrOutOfWindowRaffleIsNotLinked(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createMap(t, "Obon Matsuri 2026")
	env.publishMap(t, mapID)
	occupantID := int(firstOccupant(stallsOf(env.mapDetail(t, mapID))[0])["id"].(float64))

	// A raffle whose window opens next year isn't something a visitor can enter.
	env.assignRaffle(t, "Future Raffle", mapID, occupantID,
		map[string]any{"availableFrom": "2099-01-01T00:00:00Z"})
	if raffle := env.stallRaffle(t, mapID, 0); raffle != nil {
		t.Errorf("linked %v; want a raffle outside its window left off", raffle)
	}
}

func TestFestivalMap_ClosingARaffleDropsItsStallLink(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createMap(t, "Obon Matsuri 2026")
	env.publishMap(t, mapID)
	occupantID := int(firstOccupant(stallsOf(env.mapDetail(t, mapID))[0])["id"].(float64))
	raffleID := env.assignRaffle(t, "Gaelicat Raffle", mapID, occupantID, nil)

	if env.stallRaffle(t, mapID, 0) == nil {
		t.Fatal("the open raffle should be linked before it is closed")
	}

	resp := env.postJSON(t, fmt.Sprintf("/api/v1/raffles/%d/close", raffleID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("close raffle status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Closed means the entry form is gone, so the map stops pointing at it.
	if raffle := env.stallRaffle(t, mapID, 0); raffle != nil {
		t.Errorf("linked %v; want a closed raffle left off", raffle)
	}
}

func TestFestivalMap_UnlinkedRaffleClearsItsStall(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createMap(t, "Obon Matsuri 2026")
	occupantID := int(firstOccupant(stallsOf(env.mapDetail(t, mapID))[0])["id"].(float64))

	// A stall on a raffle that names no map claims a festival link it doesn't have.
	var raffle map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/raffles", raffleBody(map[string]any{
		"title": "Standalone Raffle", "maxEntries": 1, "entryMode": "details", "occupantId": occupantID,
	})), http.StatusCreated, &raffle)
	if raffle["occupantId"] != nil {
		t.Errorf("occupantId = %v; want it cleared on a raffle with no map", raffle["occupantId"])
	}
}

func TestFestivalMap_RaffleKeepsItsMapWhenTheStallGoesAway(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createMap(t, "Obon Matsuri 2026")
	stalls := stallsOf(env.mapDetail(t, mapID))
	keptStall := int(stalls[0].(map[string]any)["id"].(float64))
	keptOccupant := int(firstOccupant(stalls[0])["id"].(float64))
	doomedOccupant := int(firstOccupant(stalls[1])["id"].(float64))
	raffleID := env.assignRaffle(t, "Homeless Raffle", mapID, doomedOccupant, nil)

	// Re-save the map without the second pitch.
	only := pitch("circle", 10, 10, 12, 12, occupant("The Green Gaelicat", "game", map[string]any{"id": keptOccupant}))
	only["id"] = keptStall
	decodeInto(t, env.saveMap(t, mapID, mapBody("Obon Matsuri 2026", map[string]any{"stalls": []map[string]any{only}})),
		http.StatusOK, nil)

	after := env.raffleDetail(t, raffleID)["raffle"].(map[string]any)
	// It is still part of the festival - it just isn't pinned to a pitch any more.
	if after["festivalMapId"] == nil {
		t.Error("the raffle lost its festival map along with the stall")
	}
	if after["occupantId"] != nil {
		t.Errorf("occupantId = %v; want it cleared with the stall", after["occupantId"])
	}
}

func TestFestivalMap_DeleteReleasesItsRaffles(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createMap(t, "Obon Matsuri 2026")
	occupantID := int(firstOccupant(stallsOf(env.mapDetail(t, mapID))[0])["id"].(float64))
	raffleID := env.assignRaffle(t, "Gaelicat Raffle", mapID, occupantID, nil)

	resp := env.del(t, fmt.Sprintf("/api/v1/festival-maps/%d", mapID))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete map status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// The raffle survives the festival it was filed under.
	after := env.raffleDetail(t, raffleID)["raffle"].(map[string]any)
	if after["festivalMapId"] != nil || after["occupantId"] != nil {
		t.Errorf("raffle links = %v/%v; want both cleared", after["festivalMapId"], after["occupantId"])
	}
	if after["title"] != "Gaelicat Raffle" {
		t.Errorf("title = %v; want the raffle intact", after["title"])
	}
}

func TestFestivalMap_PublicReadsAreCachedByRevalidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createMap(t, "Obon Matsuri 2026")
	env.publishMap(t, id)

	for _, path := range []string{"/api/v1/festival-maps/public", fmt.Sprintf("/api/v1/festival-maps/public/%d", id)} {
		resp := env.get(t, path)
		resp.Body.Close()
		tag := resp.Header.Get("ETag")
		if tag == "" || resp.Header.Get("Cache-Control") != "no-cache" {
			t.Fatalf("GET %s: ETag %q, Cache-Control %q; want an ETag and no-cache", path, tag, resp.Header.Get("Cache-Control"))
		}
		req, _ := http.NewRequest(http.MethodGet, env.url(path), nil)
		req.Header.Set("If-None-Match", tag)
		again, err := env.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		again.Body.Close()
		if again.StatusCode != http.StatusNotModified {
			t.Errorf("GET %s with the ETag = %d; want 304", path, again.StatusCode)
		}
	}
	// A staff answer is private.
	if got := env.get(t, mapPathOf(id)); !strings.Contains(got.Header.Get("Cache-Control"), "no-store") {
		t.Errorf("staff detail Cache-Control = %q; want no-store", got.Header.Get("Cache-Control"))
	}
}

// told reads the next n resource_changed events from a staff stream and answers the
// resources they name.
func told(t *testing.T, s *stream, n int) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	for range n {
		got[s.expect("resource_changed", 3*time.Second).str("resource")] = true
	}
	return got
}

func TestFestivalMap_WritesTellTheMapsAndWhatTheyLink(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	s := staffStream(t, env, env.client)

	id := env.createMap(t, "Obon Matsuri 2026")
	if got := told(t, s, 1); !got["festival-maps"] {
		t.Errorf("a create told %v; want festival-maps", got)
	}

	d := env.mapDetail(t, id)
	decodeInto(t, env.saveMap(t, id, mapBody("Obon Matsuri 2026", map[string]any{"stalls": inputStalls(d)})), http.StatusOK, nil)
	// The stall names show on the stamp cards and the raffles.
	if got := told(t, s, 3); !got["festival-maps"] || !got["stamp-rallies"] || !got["raffles"] {
		t.Errorf("an update told %v; want festival-maps, stamp-rallies and raffles", got)
	}

	resp := env.del(t, mapPathOf(id))
	resp.Body.Close()
	if got := told(t, s, 5); !got["festival-maps"] || !got["stamp-rallies"] || !got["raffles"] || !got["ema-boards"] || !got["ema-requests"] {
		t.Errorf("a delete told %v; want the map and everything it unlinked", got)
	}
}
