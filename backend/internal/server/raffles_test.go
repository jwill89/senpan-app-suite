package server_test

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// raffleBody is a full raffle as a save sends it; over replaces fields of it.
func raffleBody(over map[string]any) map[string]any {
	body := map[string]any{
		"title": "Prize Raffle", "description": "", "rules": "", "maxEntries": 5, "signupInstructions": "",
		"entryMode": "single", "costPerEntry": 100, "tierCosts": []int{}, "prizeImage": "", "payImage": "",
	}
	for k, v := range over {
		body[k] = v
	}
	return body
}

// createRaffle makes a raffle (201, the raffle itself) and answers its id.
func (e *testEnv) createRaffle(t *testing.T, over map[string]any) int {
	t.Helper()
	var made map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/raffles", raffleBody(over)), http.StatusCreated, &made)
	return int(made["id"].(float64))
}

func rafflePath(id int, tail string) string {
	return fmt.Sprintf("/api/v1/raffles/%d", id) + tail
}

// addRaffleEntry has staff add tickets for a character and answers the entry.
func (e *testEnv) addRaffleEntry(t *testing.T, raffleID int, name, world string, n int) map[string]any {
	t.Helper()
	var entry map[string]any
	resp := e.postJSON(t, rafflePath(raffleID, "/entries"), map[string]any{"characterName": name, "world": world, "numEntries": n})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("add an entry: %d", resp.StatusCode)
	}
	decodeInto(t, resp, resp.StatusCode, &entry)
	return entry
}

// settle records a payment on an entry (the body is the settlement's fields) and
// answers the entry.
func (e *testEnv) settle(t *testing.T, raffleID int, entry map[string]any, body map[string]any) map[string]any {
	t.Helper()
	if body == nil {
		body = map[string]any{}
	}
	var out map[string]any
	decodeInto(t, e.postJSON(t, rafflePath(raffleID, fmt.Sprintf("/entries/%d/settle", int(entry["id"].(float64)))), body), http.StatusOK, &out)
	return out
}

// raffleEntryByName finds an entry of a raffle by character and world, ignoring
// case, or nil.
func (e *testEnv) raffleEntryByName(t *testing.T, raffleID int, name, world string) map[string]any {
	t.Helper()
	var list struct {
		Entries []map[string]any `json:"entries"`
	}
	decodeInto(t, e.get(t, rafflePath(raffleID, "/entries")), http.StatusOK, &list)
	for _, entry := range list.Entries {
		if strings.EqualFold(entry["characterName"].(string), name) && strings.EqualFold(entry["world"].(string), world) {
			return entry
		}
	}
	return nil
}

// raffleDetail reads one raffle's detail as whoever the client is.
func (e *testEnv) raffleDetail(t *testing.T, id int) map[string]any {
	t.Helper()
	var detail map[string]any
	decodeInto(t, e.get(t, rafflePath(id, "")), http.StatusOK, &detail)
	return detail
}

func enterBody(name, world string, n int) map[string]any {
	return map[string]any{"characterName": name, "world": world, "numEntries": n}
}

// Every staff operation needs the Raffles page: 401 signed out, 403 without it, and
// the page's grantee gets through. The public ones need nothing.
func TestRafflesGuard(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, nil)
	env.signOut(t)

	staff := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/raffles"},
		{http.MethodPut, rafflePath(id, "")},
		{http.MethodDelete, rafflePath(id, "")},
		{http.MethodPost, rafflePath(id, "/close")},
		{http.MethodPost, rafflePath(id, "/reopen")},
		{http.MethodGet, rafflePath(id, "/entries")},
		{http.MethodPost, rafflePath(id, "/entries")},
		{http.MethodPost, rafflePath(id, "/entries/1/settle")},
		{http.MethodPost, rafflePath(id, "/entries/1/unsettle")},
		{http.MethodDelete, rafflePath(id, "/entries/1")},
		{http.MethodPost, rafflePath(id, "/pick-winner")},
		{http.MethodPost, rafflePath(id, "/pick-another")},
		{http.MethodPost, rafflePath(id, "/verify-winner")},
	}
	for _, c := range staff {
		if got := status(env.reqJSON(t, c.method, c.path, map[string]any{})); got != http.StatusUnauthorized {
			t.Errorf("%s %s signed out: %d, want 401", c.method, c.path, got)
		}
	}

	addAccount(t, env, "other", "correct horse battery", []string{"bingo-presets"})
	env.signIn(t, "other", "correct horse battery")
	for _, c := range staff {
		if got := status(env.reqJSON(t, c.method, c.path, map[string]any{})); got != http.StatusForbidden {
			t.Errorf("%s %s without the page: %d, want 403", c.method, c.path, got)
		}
	}

	addAccount(t, env, "raffler", "correct horse battery", []string{"teahouse-raffles"})
	env.signIn(t, "raffler", "correct horse battery")
	decodeInto(t, env.postJSON(t, "/api/v1/raffles", raffleBody(nil)), http.StatusCreated, nil)
}

// A raffle is made open (201, itself, in full), replaced without touching its status,
// closed and reopened, and deleted (204, then 404).
func TestRafflesLifecycle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/raffles", raffleBody(map[string]any{
		"title": " Prize Raffle ", "description": "Win big!", "prizeImage": "images/raffles/p.png",
	})), http.StatusCreated, &made)
	if made["title"] != "Prize Raffle" || made["status"] != "open" || made["entryMode"] != "single" ||
		made["costPerEntry"] != float64(100) || made["maxEntries"] != float64(5) || made["description"] != "Win big!" ||
		made["winnerEntryId"] != nil || made["availableFrom"] != nil || made["festivalMapName"] != "" ||
		made["winnerName"] != nil || made["paidTotal"] != nil || made["createdAt"] == nil {
		t.Errorf("made: %v", made)
	}
	id := int(made["id"].(float64))

	var saved map[string]any
	decodeInto(t, env.putJSON(t, rafflePath(id, ""), raffleBody(map[string]any{"title": "Updated", "maxEntries": 10})),
		http.StatusOK, &saved)
	if saved["title"] != "Updated" || saved["maxEntries"] != float64(10) || saved["status"] != "open" {
		t.Errorf("replaced: %v", saved)
	}
	if got := status(env.putJSON(t, "/api/v1/raffles/999", raffleBody(nil))); got != http.StatusNotFound {
		t.Errorf("replacing an unknown raffle: %d, want 404", got)
	}

	var closed map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(id, "/close"), nil), http.StatusOK, &closed)
	if closed["status"] != "closed" {
		t.Errorf("closed: %v", closed)
	}
	decodeInto(t, env.postJSON(t, rafflePath(id, "/reopen"), nil), http.StatusOK, &closed)
	if closed["status"] != "open" {
		t.Errorf("reopened: %v", closed)
	}
	for _, verb := range []string{"close", "reopen"} {
		if got := status(env.postJSON(t, "/api/v1/raffles/999/"+verb, nil)); got != http.StatusNotFound {
			t.Errorf("%s of an unknown raffle: %d, want 404", verb, got)
		}
	}

	if got := status(env.del(t, rafflePath(id, ""))); got != http.StatusNoContent {
		t.Fatalf("delete: %d, want 204", got)
	}
	if got := status(env.del(t, rafflePath(id, ""))); got != http.StatusNotFound {
		t.Errorf("deleting it again: %d, want 404", got)
	}
}

// Custom mode pins the allowance to the price ladder and drops the flat cost; the
// other modes drop the ladder; a ladder is required and held to non-negative prices.
func TestRafflesEntryModes(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var custom map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/raffles", raffleBody(map[string]any{
		"title": "Tiered", "entryMode": "custom", "maxEntries": 99, "costPerEntry": 777,
		"tierCosts": []int{50000, 100000, 150000},
	})), http.StatusCreated, &custom)
	if custom["entryMode"] != "custom" || custom["maxEntries"] != float64(3) || custom["costPerEntry"] != float64(0) {
		t.Errorf("custom: %v", custom)
	}
	if tiers := custom["tierCosts"].([]any); len(tiers) != 3 || tiers[2] != float64(150000) {
		t.Errorf("tierCosts: %v", custom["tierCosts"])
	}

	// Switching it back to single leaves no ladder behind as a shadow price.
	var single map[string]any
	decodeInto(t, env.putJSON(t, rafflePath(int(custom["id"].(float64)), ""), raffleBody(map[string]any{
		"tierCosts": []int{1, 2}, "costPerEntry": 40,
	})), http.StatusOK, &single)
	if single["costPerEntry"] != float64(40) || len(single["tierCosts"].([]any)) != 0 {
		t.Errorf("single: %v", single)
	}

	for body, want := range map[string]string{
		"no ladder":      "body.tierCosts",
		"negative price": "body.tierCosts[1]",
	} {
		tiers := []int{}
		if body == "negative price" {
			tiers = []int{50000, -1}
		}
		resp := env.postJSON(t, "/api/v1/raffles", raffleBody(map[string]any{"entryMode": "custom", "tierCosts": tiers}))
		if at := invalid(t, resp); !slices.Contains(at, want) {
			t.Errorf("%s: errors at %v, want %s", body, at, want)
		}
	}
}

// An allowance past 100, a ladder past 100, a blank title and a time that isn't a
// time are each a 422 naming the field, on a create and a replace alike; 100 itself
// is allowed.
func TestRafflesValidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, map[string]any{"maxEntries": 100})
	path := rafflePath(id, "")

	ladder := make([]int, 101)
	for _, c := range []struct {
		name string
		over map[string]any
		want string
	}{
		{"blank title", map[string]any{"title": " "}, "body.title"},
		{"allowance past the cap", map[string]any{"maxEntries": 101}, "body.maxEntries"},
		{"ladder past the cap", map[string]any{"entryMode": "custom", "tierCosts": ladder}, "body.tierCosts"},
		{"negative cost", map[string]any{"costPerEntry": -1}, "body.costPerEntry"},
		{"not a time", map[string]any{"availableFrom": "next tuesday"}, "body.availableFrom"},
		{"unknown mode", map[string]any{"entryMode": "free"}, "body.entryMode"},
		{"unknown festival", map[string]any{"festivalMapId": 999}, "body.festivalMapId"},
	} {
		if at := invalid(t, env.postJSON(t, "/api/v1/raffles", raffleBody(c.over))); !slices.Contains(at, c.want) {
			t.Errorf("create, %s: errors at %v, want %s", c.name, at, c.want)
		}
		if at := invalid(t, env.putJSON(t, path, raffleBody(c.over))); !slices.Contains(at, c.want) {
			t.Errorf("replace, %s: errors at %v, want %s", c.name, at, c.want)
		}
	}
}

// The public list holds the raffles open now inside their window, with no staff
// totals; the Raffles page's holder gets every raffle with its winner and the gil
// collected.
func TestRaffleListByViewer(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	open := env.createRaffle(t, map[string]any{"title": "Open", "costPerEntry": 50000})
	env.createRaffle(t, map[string]any{"title": "Not yet", "availableFrom": "2099-01-01T00:00:00Z"})
	closed := env.createRaffle(t, map[string]any{"title": "Closed"})
	decodeInto(t, env.postJSON(t, rafflePath(closed, "/close"), nil), http.StatusOK, nil)

	entry := env.addRaffleEntry(t, open, "Aria", "Gilgamesh", 3)
	env.settle(t, open, entry, map[string]any{"waived": 100000})
	decodeInto(t, env.postJSON(t, rafflePath(open, "/pick-winner"), nil), http.StatusOK, nil)

	type list struct {
		Raffles []map[string]any `json:"raffles"`
	}
	var staff list
	decodeInto(t, env.get(t, "/api/v1/raffles"), http.StatusOK, &staff)
	if len(staff.Raffles) != 3 {
		t.Fatalf("staff see %d raffles, want all 3", len(staff.Raffles))
	}
	for _, r := range staff.Raffles {
		if r["paidTotal"] == nil {
			t.Errorf("staff: %v has no paidTotal", r["title"])
		}
		if r["title"] == "Open" && (r["winnerName"] != "Aria @ Gilgamesh" || r["paidTotal"] != float64(50000)) {
			t.Errorf("staff: the open raffle: %v", r)
		}
	}

	env.signOut(t)
	var public list
	decodeInto(t, env.get(t, "/api/v1/raffles"), http.StatusOK, &public)
	if len(public.Raffles) != 1 || public.Raffles[0]["title"] != "Open" {
		t.Fatalf("the public see %v, want only the open raffle", public.Raffles)
	}
	if public.Raffles[0]["winnerName"] != nil || public.Raffles[0]["paidTotal"] != nil {
		t.Errorf("the public see staff totals: %v", public.Raffles[0])
	}
}

// A raffle not open yet is a 404 to the public, so guessing its id reveals nothing,
// and a closed one shows its winner by name alone.
func TestRaffleDetailByViewer(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	hidden := env.createRaffle(t, map[string]any{"availableFrom": "2099-01-01T00:00:00Z"})
	drawn := env.createRaffle(t, map[string]any{"costPerEntry": 50000})
	entry := env.addRaffleEntry(t, drawn, "Aria", "Gilgamesh", 3)
	env.settle(t, drawn, entry, map[string]any{"waived": 150000})
	decodeInto(t, env.postJSON(t, rafflePath(drawn, "/pick-winner"), nil), http.StatusOK, nil)
	decodeInto(t, env.postJSON(t, rafflePath(drawn, "/verify-winner"), map[string]any{"winnerEntryId": entry["id"]}), http.StatusOK, nil)

	if d := env.raffleDetail(t, hidden); d["totalEntries"] != float64(0) {
		t.Errorf("staff see a hidden raffle's detail: %v", d)
	}
	env.signOut(t)

	if got := status(env.get(t, rafflePath(hidden, ""))); got != http.StatusNotFound {
		t.Errorf("a hidden raffle: %d, want 404", got)
	}
	if got := status(env.get(t, "/api/v1/raffles/99999")); got != http.StatusNotFound {
		t.Errorf("an unknown raffle: %d, want 404", got)
	}
	d := env.raffleDetail(t, drawn)
	winner, _ := d["winnerEntry"].(map[string]any)
	if d["totalEntries"] != float64(3) || winner["characterName"] != "Aria" || winner["world"] != "Gilgamesh" || winner["numEntries"] != float64(3) {
		t.Errorf("a closed raffle's detail: %v", d)
	}
	for _, k := range []string{"amountWaived", "paidEntries", "paid", "id"} {
		if _, leaked := winner[k]; leaked {
			t.Errorf("the winner leaks %q: %v", k, winner)
		}
	}
}

// Staff add tickets: 201 for a new entry, 200 when they join the character's entry
// (matched ignoring case); tickets past the allowance, or on a closed raffle, are a
// 409; a paid add settles them at once.
func TestRaffleStaffAddsEntries(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, map[string]any{"maxEntries": 5})

	var made map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(id, "/entries"), enterBody("Cloud", "Gaia", 2)), http.StatusCreated, &made)
	if made["characterName"] != "Cloud" || made["numEntries"] != float64(2) || made["paid"] != false || made["paymentState"] != "unpaid" {
		t.Errorf("made: %v", made)
	}
	var merged map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(id, "/entries"), enterBody("cloud", "GAIA", 2)), http.StatusOK, &merged)
	if merged["id"] != made["id"] || merged["numEntries"] != float64(4) {
		t.Errorf("merged: %v", merged)
	}
	if got := status(env.postJSON(t, rafflePath(id, "/entries"), enterBody("Cloud", "Gaia", 2))); got != http.StatusConflict {
		t.Errorf("past the allowance: %d, want 409", got)
	}
	if got := status(env.postJSON(t, rafflePath(id, "/entries"), enterBody("Tifa", "Gaia", 6))); got != http.StatusConflict {
		t.Errorf("a first add past the allowance: %d, want 409", got)
	}

	var paid map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(id, "/entries"), map[string]any{
		"characterName": "Tifa", "world": "Gaia", "numEntries": 1, "paid": true, "amountWaived": 25,
	}), http.StatusCreated, &paid)
	if paid["paid"] != true || paid["paidEntries"] != float64(1) || paid["amountWaived"] != float64(25) || paid["paymentState"] != "paid" {
		t.Errorf("a paid add: %v", paid)
	}

	for body, want := range map[string]string{"characterName": "body.characterName", "world": "body.world", "numEntries": "body.numEntries"} {
		bad := enterBody("P", "W", 1)
		bad[body] = map[string]any{"characterName": " ", "world": "", "numEntries": 0}[body]
		if at := invalid(t, env.postJSON(t, rafflePath(id, "/entries"), bad)); !slices.Contains(at, want) {
			t.Errorf("bad %s: errors at %v, want %s", body, at, want)
		}
	}

	decodeInto(t, env.postJSON(t, rafflePath(id, "/close"), nil), http.StatusOK, nil)
	if got := status(env.postJSON(t, rafflePath(id, "/entries"), enterBody("P", "W", 1))); got != http.StatusConflict {
		t.Errorf("a closed raffle: %d, want 409", got)
	}
	if got := status(env.postJSON(t, "/api/v1/raffles/999/entries", enterBody("P", "W", 1))); got != http.StatusNotFound {
		t.Errorf("an unknown raffle: %d, want 404", got)
	}
}

// Staff add ignores the availability window while the raffle is open, where the
// public sign-up does not.
func TestRaffleStaffAddIgnoresTheWindow(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, map[string]any{"availableTo": "2000-01-01T00:00:00Z"})

	if got := status(env.postJSON(t, rafflePath(id, "/enter"), enterBody("P", "W", 1))); got != http.StatusConflict {
		t.Errorf("the public sign-up after the window: %d, want 409", got)
	}
	decodeInto(t, env.postJSON(t, rafflePath(id, "/entries"), enterBody("P", "W", 1)), http.StatusCreated, nil)
}

// Settlements add up: a waiver adds to the running total, a part payment reads as
// partial, settling again changes nothing, unsettling clears everything, and an entry
// of another raffle can't be reached through this one.
func TestRaffleSettlement(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, map[string]any{"maxEntries": 5, "costPerEntry": 50000})
	entry := env.addRaffleEntry(t, id, "Aria", "Gilgamesh", 1)

	first := env.settle(t, id, entry, map[string]any{"waived": 10000})
	if first["amountWaived"] != float64(10000) || first["paidEntries"] != float64(1) || first["paid"] != true {
		t.Fatalf("first settlement: %v", first)
	}
	// The same payment again moves nothing: it is idempotent, not stacked.
	if again := env.settle(t, id, entry, map[string]any{"waived": 10000}); again["amountWaived"] != float64(10000) {
		t.Errorf("settling a settled entry: %v", again)
	}

	// Two more tickets bought, then settled with another waiver: the waivers add.
	entry = env.addRaffleEntry(t, id, "Aria", "Gilgamesh", 2)
	if entry["paymentState"] != "partial" || entry["paid"] != false || entry["paidEntries"] != float64(1) || entry["amountWaived"] != float64(10000) {
		t.Fatalf("after buying more: %v", entry)
	}
	second := env.settle(t, id, entry, map[string]any{"waived": 5000})
	if second["amountWaived"] != float64(15000) || second["paidEntries"] != float64(3) || second["paid"] != true {
		t.Errorf("second settlement: %v", second)
	}

	var cleared map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(id, fmt.Sprintf("/entries/%d/unsettle", int(entry["id"].(float64)))), nil), http.StatusOK, &cleared)
	if cleared["amountWaived"] != float64(0) || cleared["paidEntries"] != float64(0) || cleared["paid"] != false || cleared["paymentState"] != "unpaid" {
		t.Errorf("cleared: %v", cleared)
	}

	// A part payment: two of three tickets now.
	part := env.settle(t, id, entry, map[string]any{"paidEntries": 2, "waived": 10000})
	if part["paidEntries"] != float64(2) || part["paid"] != false || part["paymentState"] != "partial" || part["amountWaived"] != float64(10000) {
		t.Errorf("part payment: %v", part)
	}
	if full := env.settle(t, id, entry, map[string]any{}); full["paidEntries"] != float64(3) || full["paid"] != true {
		t.Errorf("settling the rest: %v", full)
	}

	other := env.createRaffle(t, nil)
	entryPath := fmt.Sprintf("/entries/%d", int(entry["id"].(float64)))
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, rafflePath(other, entryPath+"/settle")},
		{http.MethodPost, rafflePath(other, entryPath+"/unsettle")},
		{http.MethodDelete, rafflePath(other, entryPath)},
		{http.MethodPost, rafflePath(id, "/entries/9999/settle")},
	} {
		if got := status(env.reqJSON(t, c.method, c.path, map[string]any{})); got != http.StatusNotFound {
			t.Errorf("%s %s: %d, want 404", c.method, c.path, got)
		}
	}
	if at := invalid(t, env.postJSON(t, rafflePath(id, entryPath+"/settle"), map[string]any{"waived": -1})); !slices.Contains(at, "body.waived") {
		t.Errorf("a negative waiver: errors at %v", at)
	}
	if at := invalid(t, env.postJSON(t, rafflePath(id, entryPath+"/settle"), map[string]any{"paidEntries": 0})); !slices.Contains(at, "body.paidEntries") {
		t.Errorf("zero tickets: errors at %v", at)
	}
}

// A paying raffle draws on settled tickets, a details-only one on every ticket; with
// none to draw from the pick is a 409.
func TestRafflePickWinner(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	paying := env.createRaffle(t, nil)
	if got := status(env.postJSON(t, rafflePath(paying, "/pick-winner"), nil)); got != http.StatusConflict {
		t.Errorf("no entries: %d, want 409", got)
	}
	env.addRaffleEntry(t, paying, "Unpaid", "Hades", 5)
	if got := status(env.postJSON(t, rafflePath(paying, "/pick-winner"), nil)); got != http.StatusConflict {
		t.Errorf("only unsettled entries: %d, want 409", got)
	}
	winner := env.addRaffleEntry(t, paying, "Winner", "World", 3)
	env.settle(t, paying, winner, map[string]any{"paidEntries": 1})
	var picked map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(paying, "/pick-winner"), nil), http.StatusOK, &picked)
	if picked["characterName"] != "Winner" {
		t.Errorf("picked %v; the only settled entry should win", picked["characterName"])
	}
	if got := status(env.postJSON(t, rafflePath(paying, "/pick-winner"), nil)); got != http.StatusConflict {
		t.Errorf("a second pick with a pending winner: %d, want 409", got)
	}
	if d := env.raffleDetail(t, paying); d["raffle"].(map[string]any)["winnerEntryId"] != picked["id"] {
		t.Errorf("the winner was not recorded: %v", d["raffle"])
	}

	details := env.createRaffle(t, map[string]any{"entryMode": "details", "maxEntries": 3})
	env.addRaffleEntry(t, details, "Aria", "Gilgamesh", 1)
	decodeInto(t, env.postJSON(t, rafflePath(details, "/pick-winner"), nil), http.StatusOK, &picked)
	if picked["characterName"] != "Aria" {
		t.Errorf("a details-only raffle drew %v, want Aria (never settled)", picked["characterName"])
	}
	if got := status(env.postJSON(t, "/api/v1/raffles/999/pick-winner", nil)); got != http.StatusNotFound {
		t.Errorf("an unknown raffle: %d, want 404", got)
	}
}

// Pick another replaces the pending winner with a different entry; it needs the
// pending winner named, and with no one else to draw leaves none pending.
func TestRafflePickAnother(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, nil)
	aria := env.addRaffleEntry(t, id, "Aria", "Gilgamesh", 1)
	env.settle(t, id, aria, nil)
	borin := env.addRaffleEntry(t, id, "Borin", "Hades", 1)
	env.settle(t, id, borin, nil)

	var first map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(id, "/pick-winner"), nil), http.StatusOK, &first)

	if got := status(env.postJSON(t, rafflePath(id, "/pick-another"), map[string]any{"replaces": 99999})); got != http.StatusConflict {
		t.Errorf("another console's pick: %d, want 409", got)
	}
	var second map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(id, "/pick-another"), map[string]any{"replaces": first["id"]}), http.StatusOK, &second)
	if second["id"] == first["id"] {
		t.Errorf("pick another drew the same entry %v", first["id"])
	}
	if d := env.raffleDetail(t, id); d["raffle"].(map[string]any)["winnerEntryId"] != second["id"] {
		t.Errorf("the new winner was not recorded: %v", d["raffle"])
	}

	// Deleting the pending winner clears it; drawing again, then deleting the only
	// other entry, leaves nobody to draw once the winner is replaced.
	if got := status(env.del(t, rafflePath(id, fmt.Sprintf("/entries/%d", int(second["id"].(float64)))))); got != http.StatusNoContent {
		t.Fatalf("delete the winner's entry: %d", got)
	}
	if d := env.raffleDetail(t, id); d["raffle"].(map[string]any)["winnerEntryId"] != nil {
		t.Errorf("deleting the winner's entry left it pending: %v", d["raffle"])
	}
	var last map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(id, "/pick-winner"), nil), http.StatusOK, &last)
	if got := status(env.postJSON(t, rafflePath(id, "/pick-another"), map[string]any{"replaces": last["id"]})); got != http.StatusConflict {
		t.Errorf("nobody else to draw: %d, want 409", got)
	}
	if d := env.raffleDetail(t, id); d["raffle"].(map[string]any)["winnerEntryId"] != nil {
		t.Errorf("the winner stays cleared when nobody else is left: %v", d["raffle"])
	}
}

// Confirming the winner closes the raffle; with no pending winner, or the wrong one,
// it is a 409; close and reopen keep a recorded winner.
func TestRaffleVerifyWinner(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, nil)
	entry := env.addRaffleEntry(t, id, "Aria", "Gilgamesh", 1)
	env.settle(t, id, entry, nil)

	if got := status(env.postJSON(t, rafflePath(id, "/verify-winner"), map[string]any{"winnerEntryId": entry["id"]})); got != http.StatusConflict {
		t.Errorf("no pending winner: %d, want 409", got)
	}
	decodeInto(t, env.postJSON(t, rafflePath(id, "/pick-winner"), nil), http.StatusOK, nil)
	if got := status(env.postJSON(t, rafflePath(id, "/verify-winner"), map[string]any{"winnerEntryId": 99999})); got != http.StatusConflict {
		t.Errorf("another entry than the pending winner: %d, want 409", got)
	}
	var closed map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(id, "/verify-winner"), map[string]any{"winnerEntryId": entry["id"]}), http.StatusOK, &closed)
	if closed["status"] != "closed" || closed["winnerEntryId"] != entry["id"] {
		t.Errorf("confirmed: %v", closed)
	}

	decodeInto(t, env.postJSON(t, rafflePath(id, "/reopen"), nil), http.StatusOK, nil)
	decodeInto(t, env.postJSON(t, rafflePath(id, "/close"), nil), http.StatusOK, &closed)
	if closed["winnerEntryId"] != entry["id"] {
		t.Errorf("closing rewrote the winner: %v", closed)
	}
}

// A details-only raffle can be closed without a winner, which verify can't do.
func TestRaffleCloseWithoutWinner(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, map[string]any{"entryMode": "details"})

	if got := status(env.postJSON(t, rafflePath(id, "/verify-winner"), map[string]any{"winnerEntryId": 1})); got != http.StatusConflict {
		t.Errorf("verify with no winner: %d, want 409", got)
	}
	var closed map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(id, "/close"), nil), http.StatusOK, &closed)
	if closed["status"] != "closed" || closed["winnerEntryId"] != nil {
		t.Errorf("closed: %v", closed)
	}
}

// The public sign-up: 201 then 200 as the tickets merge, the total climbing the
// ladder, the amount due skipping tickets already settled, and every refusal a 409
// or 422.
func TestRaffleEnter(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, map[string]any{
		"entryMode": "custom", "tierCosts": []int{50000, 100000, 150000}, "signupInstructions": "Pay the gil!",
	})
	env.signOut(t)

	var first map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(id, "/enter"), enterBody("Aria", "Gilgamesh", 1)), http.StatusCreated, &first)
	if first["totalEntries"] != float64(1) || first["totalCost"] != float64(50000) || first["amountDue"] != float64(50000) ||
		first["signupInstructions"] != "Pay the gil!" || first["message"] != "Signed up successfully" {
		t.Errorf("first sign-up: %v", first)
	}

	// Staff settle that ticket, waiving half of it.
	env.loginAdmin(t)
	env.settle(t, id, env.raffleEntryByName(t, id, "Aria", "Gilgamesh"), map[string]any{"waived": 25000})
	env.signOut(t)

	// Coming back for two more: the sticker price is the whole ladder, only the two
	// new tickets are owed.
	var more map[string]any
	decodeInto(t, env.postJSON(t, rafflePath(id, "/enter"), enterBody("Aria", "Gilgamesh", 2)), http.StatusOK, &more)
	if more["totalEntries"] != float64(3) || more["totalCost"] != float64(300000) || more["amountDue"] != float64(250000) ||
		more["message"] != "Entries added successfully" {
		t.Errorf("returning sign-up: %v", more)
	}
	// A fourth ticket is past the ladder-derived allowance.
	if got := status(env.postJSON(t, rafflePath(id, "/enter"), enterBody("Aria", "Gilgamesh", 1))); got != http.StatusConflict {
		t.Errorf("past the allowance: %d, want 409", got)
	}
}

// The public sign-up refuses what it can't take: a raffle that is closed, details-only,
// or outside its window is a 409, an invalid body a 422, an unknown raffle a 404.
func TestRaffleEnterRefusals(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	closed := env.createRaffle(t, nil)
	decodeInto(t, env.postJSON(t, rafflePath(closed, "/close"), nil), http.StatusOK, nil)
	details := env.createRaffle(t, map[string]any{"entryMode": "details", "maxEntries": 3})
	notYet := env.createRaffle(t, map[string]any{"availableFrom": "2099-01-01T00:00:00Z"})
	over := env.createRaffle(t, map[string]any{"availableTo": "2000-01-01T00:00:00Z"})
	open := env.createRaffle(t, map[string]any{"maxEntries": 3})
	env.signOut(t)

	for name, id := range map[string]int{"closed": closed, "details-only": details, "not yet open": notYet, "over": over} {
		if got := status(env.postJSON(t, rafflePath(id, "/enter"), enterBody("Aria", "Gilgamesh", 1))); got != http.StatusConflict {
			t.Errorf("%s raffle: %d, want 409", name, got)
		}
	}
	if got := status(env.postJSON(t, "/api/v1/raffles/99999/enter", enterBody("Aria", "Gilgamesh", 1))); got != http.StatusNotFound {
		t.Errorf("unknown raffle: %d, want 404", got)
	}
	if got := status(env.postJSON(t, rafflePath(open, "/enter"), enterBody("Aria", "Gilgamesh", 5))); got != http.StatusConflict {
		t.Errorf("more than the allowance: %d, want 409", got)
	}

	for name, c := range map[string]struct {
		body map[string]any
		want string
	}{
		"blank name":          {enterBody(" ", "Gilgamesh", 1), "body.characterName"},
		"no world":            {enterBody("Aria", "", 1), "body.world"},
		"oversized name":      {enterBody(strings.Repeat("A", 500), "Gilgamesh", 1), "body.characterName"},
		"no tickets":          {enterBody("Aria", "Gilgamesh", 0), "body.numEntries"},
		"too many tickets":    {enterBody("Aria", "Gilgamesh", 101), "body.numEntries"},
		"overflowing tickets": {enterBody("Aria", "Gilgamesh", math.MaxInt64), "body.numEntries"},
	} {
		if at := invalid(t, env.postJSON(t, rafflePath(open, "/enter"), c.body)); !slices.Contains(at, c.want) {
			t.Errorf("%s: errors at %v, want %s", name, at, c.want)
		}
	}

	// The raffle stays readable after all that.
	if got := status(env.get(t, rafflePath(open, ""))); got != http.StatusOK {
		t.Errorf("the raffle after the refusals: %d, want 200", got)
	}
}

// Twenty sign-ups an address may try in ten minutes, made or not; the twenty-first is
// a 429 saying when to try again.
func TestRaffleEnterIsRateLimited(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, map[string]any{"maxEntries": 1})
	env.signOut(t)

	var limited *http.Response
	for range 25 {
		resp := env.postJSON(t, rafflePath(id, "/enter"), enterBody("Aria", "Gilgamesh", 1))
		if resp.StatusCode == http.StatusTooManyRequests {
			limited = resp
			break
		}
		resp.Body.Close()
	}
	if limited == nil {
		t.Fatal("25 sign-ups from one address were never limited")
	}
	defer limited.Body.Close()
	if limited.Header.Get("Retry-After") == "" {
		t.Error("the 429 doesn't say when to try again")
	}
}

// The public search finds part of a name ignoring case, carries no id and no gil, is
// refused for a name too short to be a search, and can't see a raffle that isn't public.
func TestRaffleEntryLookup(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, map[string]any{"maxEntries": 3, "costPerEntry": 50000})
	entry := env.addRaffleEntry(t, id, "Aria Fairwind", "Gilgamesh", 3)
	env.settle(t, id, entry, map[string]any{"waived": 90000})
	env.addRaffleEntry(t, id, "50% Off", "Hades", 1)
	hidden := env.createRaffle(t, map[string]any{"availableFrom": "2099-01-01T00:00:00Z"})
	env.addRaffleEntry(t, hidden, "Aria", "Gilgamesh", 1)
	env.signOut(t)

	var found struct {
		Entries   []map[string]any `json:"entries"`
		Truncated bool             `json:"truncated"`
	}
	decodeInto(t, env.postJSON(t, rafflePath(id, "/lookup"), map[string]any{"name": " fairwind "}), http.StatusOK, &found)
	if len(found.Entries) != 1 || found.Truncated {
		t.Fatalf("lookup: %+v", found)
	}
	hit := found.Entries[0]
	if hit["characterName"] != "Aria Fairwind" || hit["world"] != "Gilgamesh" || hit["numEntries"] != float64(3) ||
		hit["paymentState"] != "paid" || hit["paidEntries"] != float64(3) {
		t.Errorf("hit: %v", hit)
	}
	for _, k := range []string{"amountWaived", "waived", "id", "totalCost", "amountDue"} {
		if _, leaked := hit[k]; leaked {
			t.Errorf("a hit leaks %q: %v", k, hit)
		}
	}

	// A wildcard is a literal character, not everyone.
	decodeInto(t, env.postJSON(t, rafflePath(id, "/lookup"), map[string]any{"name": "0%"}), http.StatusOK, &found)
	if len(found.Entries) != 1 || found.Entries[0]["characterName"] != "50% Off" {
		t.Errorf("a percent sign matched %v", found.Entries)
	}
	decodeInto(t, env.postJSON(t, rafflePath(id, "/lookup"), map[string]any{"name": "nobody here"}), http.StatusOK, &found)
	if len(found.Entries) != 0 {
		t.Errorf("a name nobody has matched %v", found.Entries)
	}

	if at := invalid(t, env.postJSON(t, rafflePath(id, "/lookup"), map[string]any{"name": "a"})); !slices.Contains(at, "body.name") {
		t.Errorf("one character: errors at %v", at)
	}
	if at := invalid(t, env.postJSON(t, rafflePath(id, "/lookup"), map[string]any{"name": " a "})); !slices.Contains(at, "body.name") {
		t.Errorf("one character between spaces: errors at %v", at)
	}
	if got := status(env.postJSON(t, rafflePath(hidden, "/lookup"), map[string]any{"name": "aria"})); got != http.StatusNotFound {
		t.Errorf("a raffle that isn't public yet: %d, want 404", got)
	}
	if got := status(env.postJSON(t, "/api/v1/raffles/99999/lookup", map[string]any{"name": "aria"})); got != http.StatusNotFound {
		t.Errorf("an unknown raffle: %d, want 404", got)
	}
}

// A search reports when it matched more than it lists.
func TestRaffleEntryLookupTruncates(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, map[string]any{"maxEntries": 1})
	for i := range 51 {
		env.addRaffleEntry(t, id, fmt.Sprintf("Aria%02d", i), "Gilgamesh", 1)
	}
	var found struct {
		Entries   []map[string]any `json:"entries"`
		Truncated bool             `json:"truncated"`
	}
	decodeInto(t, env.postJSON(t, rafflePath(id, "/lookup"), map[string]any{"name": "aria"}), http.StatusOK, &found)
	if len(found.Entries) != 50 || !found.Truncated {
		t.Errorf("got %d entries, truncated %v; want 50 and truncated", len(found.Entries), found.Truncated)
	}
	decodeInto(t, env.postJSON(t, rafflePath(id, "/lookup"), map[string]any{"name": "aria50"}), http.StatusOK, &found)
	if len(found.Entries) != 1 || found.Truncated {
		t.Errorf("one match: %d entries, truncated %v", len(found.Entries), found.Truncated)
	}
}

// Searching has a budget of its own, so it can't spend the sign-up's.
func TestRaffleLookupHasItsOwnLimit(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, nil)
	env.signOut(t)

	var limited bool
	for range 40 {
		resp := env.postJSON(t, rafflePath(id, "/lookup"), map[string]any{"name": "aria"})
		if resp.StatusCode == http.StatusTooManyRequests {
			limited = true
			resp.Body.Close()
			break
		}
		resp.Body.Close()
	}
	if !limited {
		t.Fatal("40 searches from one address were never limited")
	}
	// The sign-up's own budget is untouched.
	if got := status(env.postJSON(t, rafflePath(id, "/enter"), enterBody("Aria", "Gilgamesh", 1))); got != http.StatusCreated {
		t.Errorf("a sign-up after the searches: %d, want 201", got)
	}
}

// A staff write tells the Raffles page's streams, and a public search tells nobody.
func TestRaffleWritesTellTheirPage(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRaffle(t, nil)
	s := staffStream(t, env, env.client)

	env.addRaffleEntry(t, id, "Aria", "Gilgamesh", 1)
	if e := s.expect("resource_changed", 3*time.Second); e.str("resource") != "raffles" {
		t.Errorf("a staff add: %+v", e)
	}
	decodeInto(t, env.postJSON(t, rafflePath(id, "/lookup"), map[string]any{"name": "aria"}), http.StatusOK, nil)
	s.expectNone("resource_changed", 300*time.Millisecond)
}
