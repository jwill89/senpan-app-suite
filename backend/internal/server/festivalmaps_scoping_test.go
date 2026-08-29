package server_test

import (
	"fmt"
	"net/http"
	"testing"
)

// saveMapRoundTrip PUTs a map back exactly as its detail returned it, changing
// only the description - the shape of an ordinary "fix a typo" edit, which is what
// makes the data-loss bugs below so easy to hit.
func (e *testEnv) saveMapRoundTrip(t *testing.T, id int, description string) *http.Response {
	t.Helper()
	d := e.mapDetail(t, id)
	return e.putJSON(t, fmt.Sprintf("/api/festival-maps/%d", id), map[string]any{
		"title":       d["title"],
		"slug":        d["slug"],
		"map_image":   d["map_image"],
		"description": description,
		"times":       d["times"],
		"stalls":      stallsOf(d),
	})
}

// TestFestivalMap_AffiliateDeleteKeepsPitch covers the chain that silently deleted
// festival pitches. An occupant may name an affiliate and nothing else - a blank
// title, identity carried by the link. Deleting that affiliate NULLed the link
// (ON DELETE SET NULL), leaving an occupant with no identity at all; the next save
// of the map, however unrelated, sanitized that occupant away as an "empty row"
// and dropped the whole pitch with it, since a pitch with no occupants is dropped.
// The affiliate's name is now copied down on delete, so the pitch survives and
// still says who was standing in it.
func TestFestivalMap_AffiliateDeleteKeepsPitch(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	affID := env.createAffiliate(t, "Flora's Fancies")
	resp := env.postJSON(t, "/api/festival-maps", map[string]any{
		"title":     "Obon Matsuri",
		"map_image": "images/festival_maps/plan.png",
		"stalls": []map[string]any{
			{
				"shape":     "rect",
				"placement": map[string]any{"x": 10, "y": 10, "width": 12, "height": 8},
				// Identity carried ONLY by the affiliate link.
				"occupants": []map[string]any{{"title": "", "stall_type": "food", "affiliate_id": affID}},
			},
			{
				"shape":     "rect",
				"placement": map[string]any{"x": 60, "y": 10, "width": 12, "height": 8},
				"occupants": []map[string]any{occupant("Other Stall", "game", nil)},
			},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d; want 201", resp.StatusCode)
	}
	id := int(decodeBody(t, resp)["map"].(map[string]any)["id"].(float64))

	if r := env.del(t, fmt.Sprintf("/api/affiliates/%d", affID)); r.StatusCode != http.StatusNoContent {
		t.Fatalf("affiliate delete status = %d; want 204", r.StatusCode)
	} else {
		r.Body.Close()
	}

	// The occupant must have kept an identity rather than being left nameless.
	stalls := stallsOf(env.mapDetail(t, id))
	if len(stalls) != 2 {
		t.Fatalf("pitches after affiliate delete = %d; want 2", len(stalls))
	}
	if got := firstOccupant(stalls[0])["title"]; got != "Flora's Fancies" {
		t.Errorf("occupant title after affiliate delete = %v; want the affiliate's name carried down", got)
	}

	r := env.saveMapRoundTrip(t, id, "Obon is here!")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("round-trip save status = %d; want 200", r.StatusCode)
	}
	r.Body.Close()

	if after := stallsOf(env.mapDetail(t, id)); len(after) != 2 {
		t.Errorf("pitches after unrelated save = %d; want 2 (a pitch was silently deleted)", len(after))
	}
}

// TestFestivalMap_SaveCannotReachAnotherMapsStall covers the cross-map write. The
// stall UPDATE was scoped to map_id but its row count was never checked, so a save
// of map A carrying a stall id from map B matched no row here and then went on to
// reconcile - rewriting and deleting - map B's occupants for that pitch, clearing
// the rally stamps and raffles that named them. The save is now rejected whole.
func TestFestivalMap_SaveCannotReachAnotherMapsStall(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	victimID := env.createMap(t, "Victim Festival")
	attackerID := env.createMap(t, "Attacker Festival")

	victimStalls := stallsOf(env.mapDetail(t, victimID))
	victimStallID := victimStalls[0].(map[string]any)["id"]
	wantOccupant := firstOccupant(victimStalls[0])["title"]

	// Save the attacker's map, but claim one of its pitches IS the victim's stall
	// and hand it an empty occupant list - the shape that wiped the other map.
	attacker := env.mapDetail(t, attackerID)
	resp := env.putJSON(t, fmt.Sprintf("/api/festival-maps/%d", attackerID), map[string]any{
		"title":     attacker["title"],
		"slug":      attacker["slug"],
		"map_image": attacker["map_image"],
		"times":     attacker["times"],
		"stalls": []map[string]any{
			{
				"id":        victimStallID,
				"shape":     "rect",
				"placement": map[string]any{"x": 1, "y": 1, "width": 5, "height": 5},
				"occupants": []map[string]any{occupant("Hijacked", "food", nil)},
			},
		},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("cross-map save status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// The victim map is untouched: same pitch count, same occupant.
	after := stallsOf(env.mapDetail(t, victimID))
	if len(after) != len(victimStalls) {
		t.Fatalf("victim pitches = %d; want %d", len(after), len(victimStalls))
	}
	if got := firstOccupant(after[0])["title"]; got != wantOccupant {
		t.Errorf("victim occupant = %v; want %v (rewritten across maps)", got, wantOccupant)
	}
}

// TestFestivalMap_SaveWithoutStallsKeyKeepsPitches covers the full-replace hazard.
// The admin list omits `stalls` entirely, and an editor opened before the detail
// fetch landed (or after it failed) holds exactly that shape - so saving it sent
// no stalls at all and the reconciliation deleted every pitch on the map, along
// with the rally stamps and raffles pinned to them. An omitted key now means
// "leave them alone"; an explicit empty array still clears them, so deliberately
// emptying a map keeps working.
func TestFestivalMap_SaveWithoutStallsKeyKeepsPitches(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createMap(t, "Obon Matsuri")
	before := len(stallsOf(env.mapDetail(t, id)))
	if before == 0 {
		t.Fatal("fixture should have created pitches")
	}

	// A save with no "stalls" key at all - the list-row shape.
	d := env.mapDetail(t, id)
	resp := env.putJSON(t, fmt.Sprintf("/api/festival-maps/%d", id), map[string]any{
		"title":       d["title"],
		"slug":        d["slug"],
		"map_image":   d["map_image"],
		"description": "Typo fixed",
		"times":       d["times"],
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save without stalls status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	after := env.mapDetail(t, id)
	// Read defensively: if the save wrongly wiped the pitches, the response omits
	// "stalls" entirely and a type assertion would panic instead of reporting.
	kept, _ := after["stalls"].([]any)
	if len(kept) != before {
		t.Errorf("pitches after a stall-less save = %d; want %d untouched", len(kept), before)
	}
	if after["description"] != "Typo fixed" {
		t.Errorf("the edit itself did not save: description = %v", after["description"])
	}

	// An EXPLICIT empty array still means "this map has no pitches".
	resp = env.putJSON(t, fmt.Sprintf("/api/festival-maps/%d", id), map[string]any{
		"title":     d["title"],
		"slug":      d["slug"],
		"map_image": d["map_image"],
		"times":     d["times"],
		"stalls":    []map[string]any{},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save with empty stalls status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	// A map with no pitches omits "stalls" from the response entirely - the very
	// omitempty that makes a list row indistinguishable from "no stalls" and
	// created this bug class in the first place - so read it defensively.
	emptied, _ := env.mapDetail(t, id)["stalls"].([]any)
	if len(emptied) != 0 {
		t.Errorf("pitches after an explicit empty save = %d; want 0", len(emptied))
	}
}
