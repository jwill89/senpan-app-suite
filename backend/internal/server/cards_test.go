package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"app-suite/internal/bingo"
	"app-suite/internal/db"
)

// srvValidBoard returns a fresh structurally valid 5x5 bingo board (centre = FREE).
func srvValidBoard() [][]int {
	return [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
}

// cardSummary is a card as the list and cards_update give it.
type cardSummary struct {
	ID           string  `json:"id"`
	PlayerName   string  `json:"playerName"`
	Details      string  `json:"details"`
	CreatedAt    *string `json:"createdAt"`
	Protected    bool    `json:"protected"`
	CustomStatus string  `json:"customStatus"`
	World        string  `json:"world"`
}

// card is a card with its numbers, as get-card and the writes answer it.
type card struct {
	cardSummary
	BoardData [][]int `json:"boardData"`
}

// seedCardRequest stores a pending personal card straight in the database.
func (e *testEnv) seedCardRequest(t *testing.T, id string, board [][]int) {
	t.Helper()
	data, _ := json.Marshal(board)
	if err := db.New(e.store.DB()).CreateCardRequest(t.Context(), db.CreateCardRequestParams{
		ID: id, BoardData: string(data), PlayerName: "Aria", World: "Gilgamesh",
	}); err != nil {
		t.Fatal(err)
	}
}

// cardList reads every card.
func cardList(t *testing.T, env *testEnv) []cardSummary {
	t.Helper()
	var list struct {
		Cards []cardSummary `json:"cards"`
	}
	decodeInto(t, env.get(t, "/api/v1/cards"), http.StatusOK, &list)
	return list.Cards
}

// makeCard makes a card through the API, issued to player, and answers it.
func makeCard(t *testing.T, env *testEnv, player string) card {
	t.Helper()
	var c card
	decodeInto(t, env.postJSON(t, "/api/v1/cards", map[string]any{"playerName": player}), http.StatusCreated, &c)
	return c
}

// requestAddress counts the addresses requestFrom sends from, so each try comes from
// one the request limit hasn't seen.
var requestAddress atomic.Int64

// requestFrom asks for a personal card from the client address ip, as the proxy in
// front of the server names it.
func requestFrom(t *testing.T, env *testEnv, ip string, body map[string]any) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, env.url("/api/v1/cards/request"), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("CF-Connecting-IP", ip)
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// requestCard asks for a personal card from an address of its own.
func requestCard(t *testing.T, env *testEnv, body map[string]any) *http.Response {
	t.Helper()
	return requestFrom(t, env, "10.9.0."+strconv.FormatInt(requestAddress.Add(1)%250+1, 10), body)
}

func cardRequest(id string, board [][]int) map[string]any {
	return map[string]any{"characterName": "Aria Nightsong", "world": "Gilgamesh", "cardId": id, "boardData": board}
}

// The Manage Cards permission guards every operation but the public request: nobody
// signed in gets 401, an account without the page 403, and its grantee gets through.
func TestCardsGuard(t *testing.T) {
	env := newTestEnv(t)
	if err := env.store.SaveCard(t.Context(), "GUARD1", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	if got := status(env.get(t, "/api/v1/cards")); got != http.StatusUnauthorized {
		t.Errorf("signed out: %d, want 401", got)
	}
	if got := status(env.postJSON(t, "/api/v1/cards/generate", map[string]any{"count": 1})); got != http.StatusUnauthorized {
		t.Errorf("signed out, generating: %d, want 401", got)
	}

	addAccount(t, env, "presetsonly", "correct horse battery", []string{"bingo-presets"})
	env.signIn(t, "presetsonly", "correct horse battery")
	for _, call := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/v1/cards", nil},
		{http.MethodGet, "/api/v1/cards/GUARD1", nil},
		{http.MethodPost, "/api/v1/cards", map[string]any{"playerName": "X"}},
		{http.MethodPost, "/api/v1/cards/generate", map[string]any{"count": 1}},
		{http.MethodPatch, "/api/v1/cards/GUARD1", map[string]any{"details": "X"}},
		{http.MethodPost, "/api/v1/cards/GUARD1/approve", nil},
		{http.MethodPost, "/api/v1/cards/GUARD1/protect", map[string]any{"protected": true}},
		{http.MethodDelete, "/api/v1/cards/GUARD1", nil},
		{http.MethodDelete, "/api/v1/cards/all", nil},
	} {
		if got := status(env.reqJSON(t, call.method, call.path, call.body)); got != http.StatusForbidden {
			t.Errorf("%s %s without the page: %d, want 403", call.method, call.path, got)
		}
	}

	addAccount(t, env, "cardstaff", "correct horse battery", []string{"bingo-cards"})
	env.signIn(t, "cardstaff", "correct horse battery")
	if cards := cardList(t, env); len(cards) != 1 || cards[0].ID != "GUARD1" {
		t.Errorf("after the refusals: %+v, want GUARD1 alone", cards)
	}
	makeCard(t, env, "Guest")
	if cards := cardList(t, env); len(cards) != 2 {
		t.Errorf("the grantee's list: %v", cards)
	}
}

// A card is made with random numbers and a new id, issued to the player named (or
// none), and answered whole.
func TestCardsCreate(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	c := makeCard(t, env, "  Aerith ")
	if c.PlayerName != "Aerith" || len(c.ID) != 6 || c.CreatedAt == nil || c.Protected || c.CustomStatus != "" || c.Details != "" {
		t.Errorf("made: %+v", c)
	}
	if p := bingo.ValidateBoard(c.BoardData); p != nil {
		t.Errorf("its numbers aren't a card: %v", p)
	}
	var unnamed card
	decodeInto(t, env.postJSON(t, "/api/v1/cards", map[string]any{}), http.StatusCreated, &unnamed)
	if unnamed.PlayerName != "" || unnamed.ID == c.ID {
		t.Errorf("an unnamed card: %+v", unnamed)
	}
	long := strings.Repeat("x", 101)
	if at := invalid(t, env.postJSON(t, "/api/v1/cards", map[string]any{"playerName": long})); !slices.Contains(at, "body.playerName") {
		t.Errorf("a name too long: errors at %v", at)
	}
}

// A bulk generate makes as many unassigned cards as asked, each with an id of its
// own; a count out of range is a 422.
func TestCardsGenerate(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var batch struct {
		Cards []card `json:"cards"`
		Count int    `json:"count"`
	}
	decodeInto(t, env.postJSON(t, "/api/v1/cards/generate", map[string]any{"count": 3}), http.StatusCreated, &batch)
	if batch.Count != 3 || len(batch.Cards) != 3 {
		t.Fatalf("generated: %+v", batch)
	}
	for _, c := range batch.Cards {
		if len(c.ID) != 6 || c.PlayerName != "" || bingo.ValidateBoard(c.BoardData) != nil {
			t.Errorf("a generated card: %+v", c)
		}
	}
	for _, count := range []int{0, 501} {
		if at := invalid(t, env.postJSON(t, "/api/v1/cards/generate", map[string]any{"count": count})); !slices.Contains(at, "body.count") {
			t.Errorf("count %d: errors at %v", count, at)
		}
	}
}

// TestGeneratedCardIDsAreUniqueWithinTheBatch pins batch-local uniqueness. The id
// generator checked the DB, but nothing in the batch was saved until the end - so
// two cards in one run could take the same id, and the insert then kept one of
// them, silently handing out fewer cards than were asked for.
func TestGeneratedCardIDsAreUniqueWithinTheBatch(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	const want = 200
	var batch struct {
		Cards []card `json:"cards"`
	}
	decodeInto(t, env.postJSON(t, "/api/v1/cards/generate", map[string]any{"count": want}), http.StatusCreated, &batch)
	seen := map[string]bool{}
	for _, c := range batch.Cards {
		if seen[c.ID] {
			t.Fatalf("duplicate card id %q within one generated batch", c.ID)
		}
		seen[c.ID] = true
	}
	if len(seen) != want {
		t.Errorf("generated %d distinct ids; want %d", len(seen), want)
	}
	if stored := len(cardList(t, env)); stored != want {
		t.Errorf("stored %d cards; want %d - a collision was silently dropped by the insert", stored, want)
	}
}

// The list has every card by id, without numbers; get-card has one with them, by its
// id in any case. An unknown id is a 404, and one that isn't six letters or digits a
// 422 before any lookup.
func TestCardsListAndGet(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	for _, id := range []string{"BBB222", "AAA111"} {
		if err := env.store.SaveCard(t.Context(), id, srvValidBoard()); err != nil {
			t.Fatal(err)
		}
	}

	var raw struct {
		Cards []map[string]any `json:"cards"`
	}
	decodeInto(t, env.get(t, "/api/v1/cards"), http.StatusOK, &raw)
	if len(raw.Cards) != 2 || raw.Cards[0]["id"] != "AAA111" || raw.Cards[1]["id"] != "BBB222" {
		t.Fatalf("list: %v", raw.Cards)
	}
	if _, has := raw.Cards[0]["boardData"]; has {
		t.Error("the list carries numbers")
	}

	var c card
	decodeInto(t, env.get(t, "/api/v1/cards/aaa111"), http.StatusOK, &c)
	if c.ID != "AAA111" || fmt.Sprint(c.BoardData) != fmt.Sprint(srvValidBoard()) {
		t.Errorf("get-card: %+v", c)
	}
	if got := status(env.get(t, "/api/v1/cards/ZZZZZZ")); got != http.StatusNotFound {
		t.Errorf("an unknown card: %d, want 404", got)
	}
	for _, bad := range []string{"ABC", "ABC-12", "ABCDEFG"} {
		if at := invalid(t, env.get(t, "/api/v1/cards/"+bad)); !slices.Contains(at, "path.id") {
			t.Errorf("id %q: errors at %v", bad, at)
		}
	}
}

// A PATCH changes only what it sends and answers the card; an unknown card is a 404,
// and a bad id or a note too long a 422.
func TestCardsUpdate(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := makeCard(t, env, "Cloud").ID
	path := "/api/v1/cards/" + id

	var c card
	decodeInto(t, env.patchJSON(t, path, map[string]any{"details": " VIP "}), http.StatusOK, &c)
	if c.PlayerName != "Cloud" || c.Details != "VIP" || len(c.BoardData) != 5 {
		t.Errorf("after the note: %+v", c)
	}
	decodeInto(t, env.patchJSON(t, strings.ToLower(path), map[string]any{"playerName": "Tifa"}), http.StatusOK, &c)
	if c.PlayerName != "Tifa" || c.Details != "VIP" {
		t.Errorf("after the rename: %+v", c)
	}
	decodeInto(t, env.patchJSON(t, path, map[string]any{}), http.StatusOK, &c)
	if c.PlayerName != "Tifa" || c.Details != "VIP" {
		t.Errorf("after an empty change: %+v", c)
	}
	decodeInto(t, env.patchJSON(t, path, map[string]any{"playerName": ""}), http.StatusOK, &c)
	if c.PlayerName != "" || c.Details != "VIP" {
		t.Errorf("after unassigning: %+v", c)
	}
	if list := cardList(t, env); list[0].PlayerName != "" || list[0].Details != "VIP" {
		t.Errorf("the list: %+v", list)
	}

	if got := status(env.patchJSON(t, "/api/v1/cards/NOPE99", map[string]any{"details": "X"})); got != http.StatusNotFound {
		t.Errorf("an unknown card: %d, want 404", got)
	}
	if at := invalid(t, env.patchJSON(t, "/api/v1/cards/NOPE", map[string]any{"details": "X"})); !slices.Contains(at, "path.id") {
		t.Errorf("a bad id: errors at %v", at)
	}
	if at := invalid(t, env.patchJSON(t, path, map[string]any{"details": strings.Repeat("x", 1001)})); !slices.Contains(at, "body.details") {
		t.Errorf("a note too long: errors at %v", at)
	}
}

// A delete answers 204, then 404 for the same card, and the card no longer plays.
func TestCardsDelete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := makeCard(t, env, "").ID

	if got := status(env.del(t, "/api/v1/cards/"+id)); got != http.StatusNoContent {
		t.Fatalf("delete: %d, want 204", got)
	}
	if got := status(env.del(t, "/api/v1/cards/"+id)); got != http.StatusNotFound {
		t.Errorf("deleting it again: %d, want 404", got)
	}
	if got := status(env.get(t, "/api/v1/board/"+id)); got != http.StatusNotFound {
		t.Errorf("the deleted card's board: %d, want 404", got)
	}
	if at := invalid(t, env.del(t, "/api/v1/cards/A")); !slices.Contains(at, "path.id") {
		t.Errorf("a bad id: errors at %v", at)
	}
}

// Delete all takes every unprotected card, answers how many, and keeps the
// protected ones.
func TestCardsDeleteAllSparesProtected(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	for _, id := range []string{"AAA111", "BBB222", "KEEP01"} {
		if err := env.store.SaveCard(t.Context(), id, srvValidBoard()); err != nil {
			t.Fatal(err)
		}
	}
	decodeInto(t, env.postJSON(t, "/api/v1/cards/KEEP01/protect", map[string]any{"protected": true}), http.StatusOK, nil)

	var got struct {
		Deleted int `json:"deleted"`
	}
	decodeInto(t, env.del(t, "/api/v1/cards/all"), http.StatusOK, &got)
	if got.Deleted != 2 {
		t.Errorf("deleted = %d; want 2", got.Deleted)
	}
	if list := cardList(t, env); len(list) != 1 || list[0].ID != "KEEP01" || !list[0].Protected {
		t.Errorf("after delete all: %+v", list)
	}
	decodeInto(t, env.del(t, "/api/v1/cards/all"), http.StatusOK, &got)
	if got.Deleted != 0 {
		t.Errorf("deleted again = %d; want 0", got.Deleted)
	}
}

// Anyone may ask for a personal card: it is kept, pending, under the id they chose
// (upper-cased), and doesn't play until approved. A taken id or numbers another card
// has are a 409; a board that isn't a card, a bad id and a blank name each a 422
// naming the field (the cell, for a board).
func TestCardRequestFlow(t *testing.T) {
	env := newTestEnv(t)

	var receipt map[string]any
	decodeInto(t, requestCard(t, env, cardRequest("tst001", srvValidBoard())), http.StatusAccepted, &receipt)
	if receipt["id"] != "TST001" || receipt["status"] != "pending" {
		t.Fatalf("receipt: %v", receipt)
	}
	if got := status(env.get(t, "/api/v1/board/TST001")); got != http.StatusForbidden {
		t.Errorf("a pending card's board = %d; want 403", got)
	}

	otherBoard := srvValidBoard()
	otherBoard[0][0], otherBoard[1][0] = otherBoard[1][0], otherBoard[0][0]
	if _, got, detail := problem(t, requestCard(t, env, cardRequest("TST001", otherBoard))); got != http.StatusConflict || !strings.Contains(detail, "taken") {
		t.Errorf("a taken id: %d %q, want 409", got, detail)
	}
	if _, got, detail := problem(t, requestCard(t, env, cardRequest("NEW999", srvValidBoard()))); got != http.StatusConflict || strings.Contains(detail, "TST001") {
		t.Errorf("the same numbers: %d %q, want 409 that doesn't name the other card", got, detail)
	}
	decodeInto(t, requestCard(t, env, cardRequest("NEW999", otherBoard)), http.StatusAccepted, nil)

	board := func(change func([][]int)) [][]int {
		b := srvValidBoard()
		change(b)
		return b
	}
	for _, c := range []struct {
		body map[string]any
		want string
	}{
		{cardRequest("BAD001", board(func(b [][]int) { b[0][0] = 99 })), "body.boardData[0][0]"},
		{cardRequest("BAD002", board(func(b [][]int) { b[3][0] = 20 })), "body.boardData[3][0]"},
		{cardRequest("BAD003", board(func(b [][]int) { b[2][2] = 33 })), "body.boardData[2][2]"},
		{cardRequest("BAD004", board(func(b [][]int) { b[1][4] = 61 })), "body.boardData[1][4]"},
		{cardRequest("BAD005", srvValidBoard()[:4]), "body.boardData"},
		{cardRequest("TOOLONG9", srvValidBoard()), "body.cardId"},
		{cardRequest("AB-123", srvValidBoard()), "body.cardId"},
		{map[string]any{"characterName": "   ", "world": "Gilgamesh", "cardId": "OKID12", "boardData": srvValidBoard()}, "body.characterName"},
		{map[string]any{"characterName": "Aria", "world": "", "cardId": "OKID12", "boardData": srvValidBoard()}, "body.world"},
	} {
		if at := invalid(t, requestCard(t, env, c.body)); !slices.Contains(at, c.want) {
			t.Errorf("%v: errors at %v, want %s", c.body["cardId"], at, c.want)
		}
	}

	env.loginAdmin(t)
	list := cardList(t, env)
	if len(list) != 2 || list[0].CustomStatus != "pending" || list[0].PlayerName != "Aria Nightsong" || list[0].World != "Gilgamesh" {
		t.Errorf("the cards after the requests: %+v", list)
	}
}

// Ten requests from one address in ten minutes, taken in or not; the next is a 429
// saying when to try again. Another address is still let through.
func TestCardRequestRateLimit(t *testing.T) {
	env := newTestEnv(t)
	for i := range 10 {
		resp := requestFrom(t, env, "10.8.0.1", cardRequest("RATE01", srvValidBoard()))
		resp.Body.Close()
		if want := map[bool]int{true: http.StatusAccepted, false: http.StatusConflict}[i == 0]; resp.StatusCode != want {
			t.Fatalf("try %d: %d, want %d", i+1, resp.StatusCode, want)
		}
	}
	resp := requestFrom(t, env, "10.8.0.1", cardRequest("RATE02", srvValidBoard()))
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("the eleventh try: %d, want 429", resp.StatusCode)
	}
	if after, err := strconv.Atoi(resp.Header.Get("Retry-After")); err != nil || after < 1 {
		t.Errorf("Retry-After = %q, want a number of seconds", resp.Header.Get("Retry-After"))
	}
	other := srvValidBoard()
	other[0][0], other[1][0] = other[1][0], other[0][0]
	decodeInto(t, requestFrom(t, env, "10.8.0.2", cardRequest("RATE02", other)), http.StatusAccepted, nil)
}

// Approving a pending personal card makes it playable and protected, and answers it;
// a card that isn't pending is a 409 and an unknown one a 404. Protecting sets and
// clears the flag.
func TestCardApproveAndProtect(t *testing.T) {
	env := newTestEnv(t)
	decodeInto(t, requestCard(t, env, cardRequest("CUST01", srvValidBoard())), http.StatusAccepted, nil)
	if got := status(env.postJSON(t, "/api/v1/cards/CUST01/approve", nil)); got != http.StatusUnauthorized {
		t.Errorf("approving signed out: %d, want 401", got)
	}
	env.loginAdmin(t)

	var c card
	decodeInto(t, env.postJSON(t, "/api/v1/cards/cust01/approve", nil), http.StatusOK, &c)
	if c.ID != "CUST01" || c.CustomStatus != "approved" || !c.Protected || len(c.BoardData) != 5 {
		t.Errorf("approved: %+v", c)
	}
	if got := status(env.get(t, "/api/v1/board/CUST01")); got != http.StatusOK {
		t.Errorf("an approved card's board = %d; want 200", got)
	}
	if _, got, _ := problem(t, env.postJSON(t, "/api/v1/cards/CUST01/approve", nil)); got != http.StatusConflict {
		t.Errorf("approving it again: %d, want 409", got)
	}
	plain := makeCard(t, env, "").ID
	if _, got, _ := problem(t, env.postJSON(t, "/api/v1/cards/"+plain+"/approve", nil)); got != http.StatusConflict {
		t.Errorf("approving a generated card: %d, want 409", got)
	}
	if got := status(env.postJSON(t, "/api/v1/cards/NOPE99/approve", nil)); got != http.StatusNotFound {
		t.Errorf("approving an unknown card: %d, want 404", got)
	}

	decodeInto(t, env.postJSON(t, "/api/v1/cards/"+plain+"/protect", map[string]any{"protected": true}), http.StatusOK, &c)
	if c.ID != plain || !c.Protected {
		t.Errorf("protected: %+v", c)
	}
	decodeInto(t, env.postJSON(t, "/api/v1/cards/"+plain+"/protect", map[string]any{"protected": false}), http.StatusOK, &c)
	if c.Protected {
		t.Errorf("unprotected: %+v", c)
	}
	if got := status(env.postJSON(t, "/api/v1/cards/NOPE99/protect", map[string]any{"protected": true})); got != http.StatusNotFound {
		t.Errorf("protecting an unknown card: %d, want 404", got)
	}
	// A missing property is huma's to name: at the body that lacks it.
	if at := invalid(t, env.postJSON(t, "/api/v1/cards/"+plain+"/protect", map[string]any{})); !slices.Contains(at, "body") {
		t.Errorf("protect without the flag: errors at %v", at)
	}
}
