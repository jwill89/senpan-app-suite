package server_test

import (
	"fmt"
	"net/http"
	"testing"
)

// -- Festival Map admin CRUD --------------------------------------------------
//
// A stall on a map is a PITCH (shape, color, placement) holding one or more
// OCCUPANTS (affiliate, title, offering, hours) - so a booth that changes hands
// between days is one shape with two names rather than two stalls stacked on the
// same coordinates. Most of what follows exercises that split.

func TestFestivalMap_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)
	if resp := env.get(t, "/api/festival-maps"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("list status = %d; want 401", resp.StatusCode)
		resp.Body.Close()
	}
	resp := env.postJSON(t, "/api/festival-maps", map[string]any{"title": "X"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("create status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

// occupant builds one occupant row for a create/update body.
func occupant(title, stallType string, over map[string]any) map[string]any {
	o := map[string]any{"title": title, "stall_type": stallType}
	for k, v := range over {
		o[k] = v
	}
	return o
}

// createMap posts a create with two single-occupant pitches (one game circle, one
// food rectangle) and returns the new map's id.
func (e *testEnv) createMap(t *testing.T, title string) int {
	t.Helper()
	resp := e.postJSON(t, "/api/festival-maps", map[string]any{
		"title":       title,
		"description": "**Obon** is here.",
		"map_image":   "images/festival_maps/plan.png",
		"times": []map[string]any{
			{"label": "Day 1", "start": "2026-08-01T18:00:00Z", "end": "2026-08-01T22:00:00Z"},
		},
		"stalls": []map[string]any{
			{
				"shape": "circle", "color": "#AABBCC",
				"placement": map[string]any{"x": 10, "y": 10, "width": 12, "height": 12},
				"occupants": []map[string]any{occupant("The Green Gaelicat", "game", nil)},
			},
			{
				"shape":     "rect",
				"placement": map[string]any{"x": 40, "y": 30, "width": 18, "height": 10},
				"occupants": []map[string]any{occupant("Flora Teahouse", "food", nil)},
			},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create map status = %d; want 201", resp.StatusCode)
	}
	return int(decodeBody(t, resp)["map"].(map[string]any)["id"].(float64))
}

// mapDetail fetches a map's admin detail body.
func (e *testEnv) mapDetail(t *testing.T, id int) map[string]any {
	t.Helper()
	resp := e.get(t, fmt.Sprintf("/api/festival-maps/%d", id))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("map detail status = %d; want 200", resp.StatusCode)
	}
	return decodeBody(t, resp)["map"].(map[string]any)
}

// stallsOf returns a map detail's pitches.
func stallsOf(m map[string]any) []any { return m["stalls"].([]any) }

// occupantsOf returns one pitch's occupants.
func occupantsOf(stall any) []any { return stall.(map[string]any)["occupants"].([]any) }

// firstOccupant returns a pitch's first occupant.
func firstOccupant(stall any) map[string]any { return occupantsOf(stall)[0].(map[string]any) }

// publish flips a map to published, failing the test when the server refuses.
func (e *testEnv) publishMap(t *testing.T, id int) {
	t.Helper()
	resp := e.patchJSON(t, fmt.Sprintf("/api/festival-maps/%d", id), map[string]any{"status": "published"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
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
	if got := firstOccupant(stalls[0])["stall_type"]; got != "game" {
		t.Errorf("first occupant type = %v; want game", got)
	}
	times := m["times"].([]any)
	if len(times) != 1 || times[0].(map[string]any)["label"] != "Day 1" {
		t.Errorf("times = %v; want one Day 1 range", times)
	}
}

func TestFestivalMap_RejectsBadShapeTypeAndColor(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/festival-maps", map[string]any{
		"title":     "Odd",
		"map_image": "images/festival_maps/plan.png",
		"stalls": []map[string]any{
			{
				"shape": "triangle", "color": "red; background: url(x)",
				"placement": map[string]any{"x": -20, "y": 130, "width": 0, "height": 0},
				"occupants": []map[string]any{
					// A range with no start says nothing and is dropped.
					occupant("Weird", "hovercraft", map[string]any{
						"times": []map[string]any{{"label": "Ghost", "start": ""}},
					}),
				},
			},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d; want 201", resp.StatusCode)
	}
	id := int(decodeBody(t, resp)["map"].(map[string]any)["id"].(float64))

	stall := stallsOf(env.mapDetail(t, id))[0].(map[string]any)
	if stall["shape"] != "rect" {
		t.Errorf("shape = %v; want rect", stall["shape"])
	}
	if stall["color"] != "" {
		t.Errorf("color = %q; want it dropped", stall["color"])
	}
	p := stall["placement"].(map[string]any)
	if p["x"].(float64) != 0 || p["y"].(float64) != 100 || p["width"].(float64) <= 0 {
		t.Errorf("placement = %v; want clamped into the map box with a non-zero size", p)
	}
	occ := firstOccupant(stall)
	if occ["stall_type"] != "other" {
		t.Errorf("stall_type = %v; want other", occ["stall_type"])
	}
	if times := occ["times"].([]any); len(times) != 0 {
		t.Errorf("times = %v; want the startless range dropped", times)
	}
}

func TestFestivalMap_DropsEmptyOccupantsAndPitches(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/festival-maps", map[string]any{
		"title":     "Sparse",
		"map_image": "images/festival_maps/plan.png",
		"stalls": []map[string]any{
			{
				"shape":     "rect",
				"placement": map[string]any{"x": 10, "y": 10, "width": 12, "height": 8},
				"occupants": []map[string]any{
					occupant("Real Stall", "food", nil),
					// Neither a title nor an affiliate: the row the form leaves behind
					// when someone adds an occupant and thinks better of it.
					occupant("   ", "other", nil),
				},
			},
			{
				// A pitch nobody occupies is a coloured box with nothing to say.
				"shape":     "rect",
				"placement": map[string]any{"x": 60, "y": 10, "width": 12, "height": 8},
				"occupants": []map[string]any{},
			},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d; want 201", resp.StatusCode)
	}
	id := int(decodeBody(t, resp)["map"].(map[string]any)["id"].(float64))

	stalls := stallsOf(env.mapDetail(t, id))
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

	resp := env.postJSON(t, "/api/festival-maps", map[string]any{"title": "No art"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d; want 201", resp.StatusCode)
	}
	id := int(decodeBody(t, resp)["map"].(map[string]any)["id"].(float64))

	// Stalls are positioned as a share of the base image's box, so publishing
	// without one would show the public a plan of floating shapes.
	resp = env.patchJSON(t, fmt.Sprintf("/api/festival-maps/%d", id), map[string]any{"status": "published"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("publish status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	if maps := decodeBody(t, env.get(t, "/api/festival-maps/public"))["maps"].([]any); len(maps) != 0 {
		t.Errorf("public maps = %v; want none", maps)
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
	resp := env.putJSON(t, fmt.Sprintf("/api/festival-maps/%d", id), map[string]any{
		"title":     "Obon Matsuri 2026",
		"map_image": "images/festival_maps/plan.png",
		"stalls": []map[string]any{
			{
				"id": keptStall, "shape": "circle",
				"placement": map[string]any{"x": 10, "y": 10, "width": 12, "height": 12},
				"occupants": []map[string]any{
					{"id": keptOccupant, "title": "The Green Gaelicat (Day 2)", "stall_type": "game"},
				},
			},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	after := stallsOf(env.mapDetail(t, id))
	if len(after) != 1 {
		t.Fatalf("pitches after update = %d; want 1", len(after))
	}
	occ := firstOccupant(after[0])
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
	resp := e.postJSON(t, "/api/festival-maps", map[string]any{
		"title":     "Obon Matsuri 2026",
		"map_image": "images/festival_maps/plan.png",
		"times": []map[string]any{
			{"label": "Day 1", "start": "2026-08-01T00:00:00Z", "end": "2026-08-01T23:59:00Z"},
			{"label": "Day 2", "start": "2026-08-02T00:00:00Z", "end": "2026-08-02T23:59:00Z"},
		},
		"stalls": []map[string]any{
			{
				"shape":     "rect",
				"placement": map[string]any{"x": 30, "y": 40, "width": 14, "height": 8},
				"occupants": []map[string]any{
					occupant("Flora Teahouse", "game", map[string]any{
						"times": []map[string]any{
							{"label": "Day 1", "start": "2026-08-01T00:00:00Z", "end": "2026-08-01T23:59:00Z"},
						},
					}),
					occupant("The Great Below", "both", map[string]any{
						"times": []map[string]any{
							{"label": "Day 2", "start": "2026-08-02T00:00:00Z", "end": "2026-08-02T23:59:00Z"},
						},
					}),
				},
			},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create map status = %d; want 201", resp.StatusCode)
	}
	return int(decodeBody(t, resp)["map"].(map[string]any)["id"].(float64))
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

	detail := decodeBody(t, env.get(t, fmt.Sprintf("/api/festival-maps/public/%d", id)))
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
		if o.(map[string]any)["is_open"].(bool) {
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

	body := decodeBody(t, env.get(t, "/api/festival-maps/public"))
	maps := body["maps"].([]any)
	if len(maps) != 1 || maps[0].(map[string]any)["title"] != "Obon Matsuri 2026" {
		t.Fatalf("public list = %v; want only the published map", maps)
	}
	if count := maps[0].(map[string]any)["stall_count"].(float64); count != 2 {
		t.Errorf("stall_count = %v; want 2", count)
	}

	// A draft's existence isn't something the public endpoint should confirm.
	resp := env.get(t, fmt.Sprintf("/api/festival-maps/public/%d", draft))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("draft public detail status = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()

	detail := decodeBody(t, env.get(t, fmt.Sprintf("/api/festival-maps/public/%d", live)))
	if len(detail["stalls"].([]any)) != 2 {
		t.Errorf("public pitches = %v; want 2", detail["stalls"])
	}
	// No rally is linked, so no occupant carries a badge.
	for _, s := range detail["stalls"].([]any) {
		if firstOccupant(s)["in_stamp_rally"].(bool) {
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
	resp, err := anon.Get(env.url("/api/festival-maps/public"))
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
	return e.postJSON(t, "/api/festival-maps", map[string]any{
		"title": title, "slug": slug, "map_image": "images/festival_maps/plan.png",
	})
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
	m := decodeBody(t, resp)["map"].(map[string]any)
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
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("create with %s shortcode %q = %d; want 400", name, slug, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestFestivalMap_SlugMustBeUnique(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	first := env.createMapWithSlug(t, "Obon 2026", "obon-2026")
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first create status = %d; want 201", first.StatusCode)
	}
	id := int(decodeBody(t, first)["map"].(map[string]any)["id"].(float64))

	second := env.createMapWithSlug(t, "Obon 2027", "obon-2026")
	if second.StatusCode != http.StatusConflict {
		t.Errorf("duplicate shortcode = %d; want 409", second.StatusCode)
	}
	second.Body.Close()

	// A map may of course keep its own shortcode across an edit.
	resp := env.putJSON(t, fmt.Sprintf("/api/festival-maps/%d", id), map[string]any{
		"title": "Obon 2026", "slug": "obon-2026", "map_image": "images/festival_maps/plan.png",
	})
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
	resp := env.putJSON(t, fmt.Sprintf("/api/festival-maps/%d", id), map[string]any{
		"title": "Obon Matsuri 2026", "slug": "obon-2026",
		"map_image": "images/festival_maps/plan.png",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	env.publishMap(t, id)

	// Both forms resolve to the same map, so a link posted before the shortcode
	// existed keeps working.
	for _, key := range []string{"obon-2026", fmt.Sprint(id)} {
		body := decodeBody(t, env.get(t, "/api/festival-maps/public/"+key))
		if int(body["id"].(float64)) != id {
			t.Errorf("public/%s resolved to id %v; want %d", key, body["id"], id)
		}
		if body["slug"] != "obon-2026" {
			t.Errorf("public/%s slug = %v; want obon-2026", key, body["slug"])
		}
	}

	// An unknown shortcode is a 404, not a 500 or an arbitrary map.
	miss := env.get(t, "/api/festival-maps/public/hanami-2027")
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
	miss := env.get(t, "/api/festival-maps/public/draft-2026")
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
	resp := env.postJSON(t, "/api/stamp-rallies", map[string]any{
		"title":           "Obon Stamp Rally",
		"festival_map_id": mapID,
		"stamps": []map[string]any{
			{"occupant_id": day1, "password": "alpha", "stamp_type": "game",
				"image":     "images/stamp_stamps/a.png",
				"placement": map[string]any{"x": 10, "y": 10, "width": 15, "height": 15}},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create rally status = %d; want 201", resp.StatusCode)
	}
	rallyID := int(decodeBody(t, resp)["stamp_rally"].(map[string]any)["id"].(float64))

	detail := decodeBody(t, env.get(t, fmt.Sprintf("/api/festival-maps/public/%d", mapID)))
	if detail["stamp_rally_title"] != "Obon Stamp Rally" {
		t.Errorf("stamp_rally_title = %v; want the linked rally", detail["stamp_rally_title"])
	}
	pitchOccupants := occupantsOf(detail["stalls"].([]any)[0])
	flora := pitchOccupants[0].(map[string]any)
	below := pitchOccupants[1].(map[string]any)
	if !flora["in_stamp_rally"].(bool) {
		t.Error("Flora carries the rally stamp but isn't badged")
	}
	if flora["stamp_image"] != "images/stamp_stamps/a.png" {
		t.Errorf("stamp_image = %v; want the stamp art", flora["stamp_image"])
	}
	// The badge follows the OCCUPANT, not the pitch they share.
	if below["in_stamp_rally"].(bool) {
		t.Error("The Great Below is badged for a stamp that belongs to Flora")
	}

	// The rally's own stall name now comes from the occupant.
	rally := decodeBody(t, env.get(t, fmt.Sprintf("/api/stamp-rallies/%d", rallyID)))["stamp_rally"].(map[string]any)
	stamp := rally["stamps"].([]any)[0].(map[string]any)
	if stamp["stall_name"] != "Flora Teahouse" {
		t.Errorf("stall_name = %v; want the occupant's title", stamp["stall_name"])
	}
}

func TestFestivalMap_UnlinkedRallyClearsOccupantIDs(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createMap(t, "Obon Matsuri 2026")
	occID := int(firstOccupant(stallsOf(env.mapDetail(t, mapID))[0])["id"].(float64))

	// An occupant id on a rally that names no map would claim a link it doesn't have.
	resp := env.postJSON(t, "/api/stamp-rallies", map[string]any{
		"title": "Unlinked Rally",
		"stamps": []map[string]any{
			{"occupant_id": occID, "password": "alpha",
				"placement": map[string]any{"x": 10, "y": 10, "width": 15, "height": 15}},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create rally status = %d; want 201", resp.StatusCode)
	}
	rallyID := int(decodeBody(t, resp)["stamp_rally"].(map[string]any)["id"].(float64))

	stamp := decodeBody(t, env.get(t, fmt.Sprintf("/api/stamp-rallies/%d", rallyID)))["stamp_rally"].(map[string]any)["stamps"].([]any)[0].(map[string]any)
	if stamp["occupant_id"] != nil {
		t.Errorf("occupant_id = %v; want it cleared on an unlinked rally", stamp["occupant_id"])
	}
	if stamp["stall_name"] != "" {
		t.Errorf("stall_name = %v; want empty", stamp["stall_name"])
	}
}

func TestFestivalMap_DeleteUnlinksTheRally(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createMap(t, "Obon Matsuri 2026")
	occID := int(firstOccupant(stallsOf(env.mapDetail(t, mapID))[0])["id"].(float64))
	resp := env.postJSON(t, "/api/stamp-rallies", map[string]any{
		"title": "Obon Stamp Rally", "festival_map_id": mapID,
		"stamps": []map[string]any{
			{"occupant_id": occID, "password": "alpha",
				"placement": map[string]any{"x": 10, "y": 10, "width": 15, "height": 15}},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create rally status = %d; want 201", resp.StatusCode)
	}
	rallyID := int(decodeBody(t, resp)["stamp_rally"].(map[string]any)["id"].(float64))

	resp = env.del(t, fmt.Sprintf("/api/festival-maps/%d", mapID))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete map status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// The rally survives the map it was authored against.
	rally := decodeBody(t, env.get(t, fmt.Sprintf("/api/stamp-rallies/%d", rallyID)))["stamp_rally"].(map[string]any)
	if rally["festival_map_id"] != nil {
		t.Errorf("festival_map_id = %v; want it cleared", rally["festival_map_id"])
	}
	stamp := rally["stamps"].([]any)[0].(map[string]any)
	if stamp["occupant_id"] != nil {
		t.Errorf("occupant_id = %v; want it cleared", stamp["occupant_id"])
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

	resp := env.postJSON(t, "/api/stamp-rallies", map[string]any{
		"title": "Obon Stamp Rally", "festival_map_id": mapID,
		"stamps": []map[string]any{
			{"occupant_id": keptID, "password": "alpha",
				"placement": map[string]any{"x": 10, "y": 10, "width": 15, "height": 15}},
			{"occupant_id": droppedID, "password": "beta",
				"placement": map[string]any{"x": 40, "y": 10, "width": 15, "height": 15}},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create rally status = %d; want 201", resp.StatusCode)
	}
	rallyID := int(decodeBody(t, resp)["stamp_rally"].(map[string]any)["id"].(float64))

	// Re-save the map with only the Day 1 occupant.
	resp = env.putJSON(t, fmt.Sprintf("/api/festival-maps/%d", mapID), map[string]any{
		"title": "Obon Matsuri 2026", "map_image": "images/festival_maps/plan.png",
		"stalls": []map[string]any{
			{
				"id": stallID, "shape": "rect",
				"placement": map[string]any{"x": 30, "y": 40, "width": 14, "height": 8},
				"occupants": []map[string]any{
					{"id": keptID, "title": "Flora Teahouse", "stall_type": "game"},
				},
			},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update map status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	stamps := decodeBody(t, env.get(t, fmt.Sprintf("/api/stamp-rallies/%d", rallyID)))["stamp_rally"].(map[string]any)["stamps"].([]any)
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
	if kept["occupant_id"] == nil {
		t.Error("the surviving occupant's stamp lost its link")
	}
	// The dropped occupant's stamp falls back to its affiliate rather than
	// pointing at a row that is gone.
	if orphaned["occupant_id"] != nil {
		t.Errorf("occupant_id = %v; want it cleared with the occupant", orphaned["occupant_id"])
	}
	if orphaned["stall_name"] != "" {
		t.Errorf("stall_name = %v; want empty once the occupant is gone", orphaned["stall_name"])
	}
}

// -- Raffles pinned to a stall ------------------------------------------------

// assignRaffle creates a raffle filed under a map and pinned to one of its
// occupants, returning the new raffle's id. `over` merges extra raffle fields.
func (e *testEnv) assignRaffle(t *testing.T, title string, mapID, occupantID int, over map[string]any) int {
	t.Helper()
	body := map[string]any{
		"title": title, "max_entries": 1, "entry_mode": "details",
		"festival_map_id": mapID, "occupant_id": occupantID,
	}
	for k, v := range over {
		body[k] = v
	}
	resp := e.postJSON(t, "/api/raffles", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create raffle status = %d; want 201", resp.StatusCode)
	}
	return int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))
}

// stallRaffle returns the `raffle` block on a published map's first occupant of
// the pitch at `index`, or nil when that occupant carries none.
func (e *testEnv) stallRaffle(t *testing.T, mapID, index int) map[string]any {
	t.Helper()
	detail := decodeBody(t, e.get(t, fmt.Sprintf("/api/festival-maps/public/%d", mapID)))
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
		map[string]any{"prize_image": "images/raffles/prize.png"})

	raffle := env.stallRaffle(t, mapID, 0)
	if raffle == nil {
		t.Fatal("the stall carries a running raffle but the map doesn't link it")
	}
	if int(raffle["id"].(float64)) != raffleID {
		t.Errorf("raffle id = %v; want %d", raffle["id"], raffleID)
	}
	if raffle["title"] != "Gaelicat Raffle" || raffle["prize_image"] != "images/raffles/prize.png" {
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
		map[string]any{"available_from": "2099-01-01T00:00:00Z"})
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

	resp := env.postJSON(t, fmt.Sprintf("/api/raffles/%d/close", raffleID), nil)
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
	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"title": "Standalone Raffle", "max_entries": 1, "entry_mode": "details",
		"occupant_id": occupantID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create raffle status = %d; want 201", resp.StatusCode)
	}
	raffle := decodeBody(t, resp)["raffle"].(map[string]any)
	if raffle["occupant_id"] != nil {
		t.Errorf("occupant_id = %v; want it cleared on a raffle with no map", raffle["occupant_id"])
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
	resp := env.putJSON(t, fmt.Sprintf("/api/festival-maps/%d", mapID), map[string]any{
		"title": "Obon Matsuri 2026", "map_image": "images/festival_maps/plan.png",
		"stalls": []map[string]any{
			{
				"id": keptStall, "shape": "circle",
				"placement": map[string]any{"x": 10, "y": 10, "width": 12, "height": 12},
				"occupants": []map[string]any{
					{"id": keptOccupant, "title": "The Green Gaelicat", "stall_type": "game"},
				},
			},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update map status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	after := decodeBody(t, env.get(t, fmt.Sprintf("/api/raffles/%d", raffleID)))["raffle"].(map[string]any)
	// It is still part of the festival - it just isn't pinned to a pitch any more.
	if after["festival_map_id"] == nil {
		t.Error("the raffle lost its festival map along with the stall")
	}
	if after["occupant_id"] != nil {
		t.Errorf("occupant_id = %v; want it cleared with the stall", after["occupant_id"])
	}
}

func TestFestivalMap_DeleteReleasesItsRaffles(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	mapID := env.createMap(t, "Obon Matsuri 2026")
	occupantID := int(firstOccupant(stallsOf(env.mapDetail(t, mapID))[0])["id"].(float64))
	raffleID := env.assignRaffle(t, "Gaelicat Raffle", mapID, occupantID, nil)

	resp := env.del(t, fmt.Sprintf("/api/festival-maps/%d", mapID))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete map status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// The raffle survives the festival it was filed under.
	after := decodeBody(t, env.get(t, fmt.Sprintf("/api/raffles/%d", raffleID)))["raffle"].(map[string]any)
	if after["festival_map_id"] != nil || after["occupant_id"] != nil {
		t.Errorf("raffle links = %v/%v; want both cleared", after["festival_map_id"], after["occupant_id"])
	}
	if after["title"] != "Gaelicat Raffle" {
		t.Errorf("title = %v; want the raffle intact", after["title"])
	}
}
