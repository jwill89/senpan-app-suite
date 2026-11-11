package server_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// The pages' events end to end: a write happens, public or by staff, and the staff
// whose pages show it hear that it changed, then read it again over REST.

// listedCard reads the cards and answers the one with id, as a page reading them
// again after cards_changed would.
func listedCard(t *testing.T, env *testEnv, id string) cardSummary {
	t.Helper()
	var list struct{ Cards []cardSummary }
	decodeInto(t, env.get(t, "/api/v1/cards"), http.StatusOK, &list)
	for _, c := range list.Cards {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("the cards don't list %s", id)
	return cardSummary{}
}

// The card list names who holds which card, so its event goes to the staff alone: a
// board hears nothing of it.
func TestCardsChangedNotSentToPlayers(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	if err := env.store.SaveCard(t.Context(), "PLAYR1", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	player := boardStream(t, env, "PLAYR1")
	admin := staffStream(t, env, env.client)

	decodeInto(t, env.postJSON(t, "/api/v1/cards/PLAYR1/protect", map[string]any{"protected": true}), http.StatusOK, nil)

	admin.expect("cards_changed", 3*time.Second)
	player.expectNone("cards_changed", 750*time.Millisecond)
}

// Protecting a card tells the staff, and the list they read again has it protected.
func TestCardProtectTellsStaff(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	if err := env.store.SaveCard(t.Context(), "PROT01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	s := staffStream(t, env, env.client)

	decodeInto(t, env.postJSON(t, "/api/v1/cards/PROT01/protect", map[string]any{"protected": true}), http.StatusOK, nil)

	s.expect("cards_changed", 3*time.Second)
	if !listedCard(t, env, "PROT01").Protected {
		t.Error("the list read again doesn't have the card protected")
	}
}

// Approving a personal card tells the staff, and the list they read again has it
// approved and protected.
func TestCardApproveTellsStaff(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	env.seedCardRequest(t, "CUST01", srvValidBoard())
	s := staffStream(t, env, env.client)

	decodeInto(t, env.postJSON(t, "/api/v1/cards/CUST01/approve", nil), http.StatusOK, nil)

	s.expect("cards_changed", 3*time.Second)
	c := listedCard(t, env, "CUST01")
	if c.CustomStatus != "approved" || !c.Protected {
		t.Errorf("the list read again: %+v; want approved and protected", c)
	}
}

// A public raffle sign-up (".../enter"), which the admin-mutation rule leaves out,
// still tells the raffles page.
func TestRaffleEnterTellsItsPage(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rid := env.createRaffle(t, map[string]any{"maxEntries": 10})
	s := staffStream(t, env, env.client)

	decodeInto(t, env.postJSON(t, fmt.Sprintf("/api/v1/raffles/%d/enter", rid), enterBody("Cloud", "Gaia", 1)), http.StatusCreated, nil)

	if e := s.expect("resource_changed", 3*time.Second); e.str("resource") != "raffles" {
		t.Errorf("resource_changed: %+v; want raffles", e)
	}
}

// A book club's change tells that club's page alone.
func TestBookClubChangeTellsItsClub(t *testing.T) {
	env := newTestEnv(t)
	yaoi := makeActiveUser(t, env, "yaoistaff", "correct horse battery", []string{"bookclub-yaoi"})
	yuri := makeActiveUser(t, env, "yuristaff", "correct horse battery", []string{"bookclub-yuri"})
	club := staffStream(t, env, yaoi)
	other := staffStream(t, env, yuri)

	if resp := env.postJSON(t, "/api/v1/book-clubs/yaoi/reading-lists", map[string]any{"title": "Spring"}); status(resp) != http.StatusCreated {
		t.Fatalf("create a reading list: %d", resp.StatusCode)
	}

	if e := club.expect("resource_changed", 3*time.Second); e.str("resource") != "bookclub-yaoi" {
		t.Errorf("resource_changed: %+v; want bookclub-yaoi", e)
	}
	other.expectNone("resource_changed", 300*time.Millisecond)
}

// Deleting a card tells its board, every time, and ends its stream: the board goes
// home rather than holding a board for a deleted card.
func TestCardDeleteReachesThePlayer(t *testing.T) {
	for i := range 12 {
		env := newTestEnv(t)
		env.loginAdmin(t)
		cardID := fmt.Sprintf("DEL%03d", i)
		if err := env.store.SaveCard(t.Context(), cardID, srvValidBoard()); err != nil {
			t.Fatal(err)
		}
		player := boardStream(t, env, cardID)

		if resp := env.del(t, "/api/v1/cards/"+cardID); status(resp) != http.StatusNoContent {
			t.Fatalf("delete card status = %d; want 204", resp.StatusCode)
		}

		player.expect("card_deleted", 3*time.Second)
		player.expectEnd(time.Second)
	}
}

// Delete all tells the boards of the cards it took, and only them: a board on a
// protected card keeps its stream.
func TestCardDeleteAllReachesOnlyTheirPlayers(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	for _, id := range []string{"GONE01", "KEEP01"} {
		if err := env.store.SaveCard(t.Context(), id, srvValidBoard()); err != nil {
			t.Fatal(err)
		}
	}
	decodeInto(t, env.postJSON(t, "/api/v1/cards/KEEP01/protect", map[string]any{"protected": true}), http.StatusOK, nil)
	gone := boardStream(t, env, "GONE01")
	kept := boardStream(t, env, "KEEP01")

	decodeInto(t, env.del(t, "/api/v1/cards/all"), http.StatusOK, nil)

	gone.expect("card_deleted", 3*time.Second)
	kept.expectNone("card_deleted", 500*time.Millisecond)
}

// The media pages: a font write tells the Fonts page and, because the public settings
// carry the uploaded fonts, everyone's settings; a change to the image library tells
// every page that picks a picture; a cover upload changes no reading list.
func TestMediaWritesTellTheirPages(t *testing.T) {
	env := newTestEnv(t)
	fontStaff := makeActiveUser(t, env, "fontstaff", "correct horse battery", []string{"atelier-fonts"})
	emaStaff := makeActiveUser(t, env, "emastaff", "correct horse battery", []string{"festival-ema"})
	clubStaff := makeActiveUser(t, env, "clubstaff", "correct horse battery", []string{"bookclub-yaoi"})
	fonts := staffStream(t, env, fontStaff)
	ema := staffStream(t, env, emaStaff)
	club := staffStream(t, env, clubStaff)

	env.loginAdmin(t)
	decodeInto(t, env.upload(t, "/api/v1/fonts", nil, filePart{field: "files", name: "Jasper.woff2", data: []byte("woff2")}),
		http.StatusCreated, nil)
	fonts.expect("settings_changed", 3*time.Second)
	if e := fonts.expect("resource_changed", 3*time.Second); e.str("resource") != "fonts" {
		t.Errorf("resource_changed: %+v; want fonts", e)
	}
	ema.expect("settings_changed", 3*time.Second)

	decodeInto(t, env.postJSON(t, "/api/v1/image-categories", map[string]any{"name": "Plaque Art"}), http.StatusCreated, nil)
	if e := ema.expect("resource_changed", 3*time.Second); e.str("resource") != "images" {
		t.Errorf("resource_changed: %+v; want images", e)
	}
	fonts.expectNone("resource_changed", 300*time.Millisecond)

	decodeInto(t, uploadAs(t, clubStaff, env, "/api/v1/book-clubs/yaoi/covers", nil,
		filePart{field: "image", name: "cover.png", data: realPNG(t, 2, 2), kind: "image/png"}), http.StatusCreated, nil)
	club.expectNone("resource_changed", 500*time.Millisecond)
}
