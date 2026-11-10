package server_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"app-suite/internal/model"
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

// A public garapon draw goes through the token path, which the admin-mutation rule
// leaves out, and still tells the garapon page.
func TestGaraponDrawTellsItsPage(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid, err := env.store.CreateGarapon(t.Context(), &model.Garapon{
		Title:  "Festival Drum",
		Prizes: []model.GaraponPrize{{Name: "Grand", BallColor: "#fff", Rate: 1, IsGrand: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	player, err := env.store.CreateGaraponPlayer(t.Context(), gid, "Hero", "Gilgamesh", 3)
	if err != nil {
		t.Fatal(err)
	}
	s := staffStream(t, env, env.client)

	decodeInto(t, env.postJSON(t, "/api/garapon/"+player.Token+"/draw", map[string]any{}), http.StatusOK, nil)

	if e := s.expect("resource_changed", 3*time.Second); e.str("resource") != "garapons" {
		t.Errorf("resource_changed: %+v; want garapons", e)
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
// home rather than holding a board for a card that no longer exists.
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
