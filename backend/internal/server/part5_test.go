package server_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// TestRaffleEnterRejectsOversizedFields pins the length cap on the PUBLIC sign-up.
// The endpoint took character_name and world unbounded, so an unauthenticated
// caller could store a value that then had to render on the staff entry list and
// inside a Discord embed; only the 1MB JSON body limit stood in the way.
func TestRaffleEnterRejectsOversizedFields(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Length Test", "max_entries": 5,
	})
	id := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": strings.Repeat("A", 500), "world": "Gilgamesh", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("oversized character_name status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// A realistic name still goes through.
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": "Aria Nightingale", "world": "Gilgamesh", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("normal entry status = %d; want 201", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestStampPasswordGuessingIsRateLimited pins the throttle on the public stamp
// collect endpoint. Stamp passwords are short words staff read out at a stall, so
// an unthrottled endpoint let a script collect a whole card without visiting
// anything. Only a MISS costs budget.
func TestStampPasswordGuessingIsRateLimited(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	rallyID := env.createRally(t, "Guess Rally", false)
	token := env.issueCard(t, rallyID, "Tataru")

	limited := false
	for i := range 25 {
		resp := env.postJSON(t, "/api/stamp-card/"+token+"/stamp",
			map[string]any{"password": fmt.Sprintf("guess%d", i)})
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("stamp password guessing was never throttled; the endpoint is a password oracle")
	}
}

// TestFestivalMapPatchRejectsUnknownStatus pins the validation that replaced a
// silent coercion. NormalizeMapStatus falls back to in_progress for anything it
// does not recognize, which is right when reading a stored row and wrong on a
// PATCH: a typo, or a body with no status at all, silently unpublished a live map.
func TestFestivalMapPatchRejectsUnknownStatus(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createMap(t, "Status Test")
	env.publishMap(t, id)

	for _, body := range []map[string]any{
		{"status": "publised"}, // typo
		{},                     // no status at all
		{"status": ""},
	} {
		resp := env.patchJSON(t, fmt.Sprintf("/api/festival-maps/%d", id), body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("PATCH %v status = %d; want 400", body, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Still published - none of the above was allowed to change it.
	if got := env.mapDetail(t, id)["status"]; got != "published" {
		t.Errorf("map status = %v; want it left published", got)
	}
}

// TestGameStartRejectsDeletedPatterns pins the guard against a permanently
// unwinnable game. The handler checks the request carries pattern ids at all, but
// the store returns only the ids that still exist - so a selection whose patterns
// were all deleted between opening the form and pressing Start resolved to an
// empty win set and started a game nobody could ever win, silently.
func TestGameStartRejectsDeletedPatterns(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	pid := env.seedPattern(t, "Top Row")
	resp := env.del(t, fmt.Sprintf("/api/v1/patterns/%d", pid))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete pattern status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	if at := invalid(t, env.postJSON(t, "/api/v1/game/start", map[string]any{"patternIds": []int64{pid}})); !slices.Contains(at, "body.patternIds") {
		t.Errorf("start with only deleted patterns: errors at %v; want body.patternIds", at)
	}

	// No game should have been started.
	if s := readGame(t, env.client, env); s.Game != nil {
		t.Error("a game was started with no resolvable patterns")
	}
}

// TestBoardStreamRequiresARealCard pins the id check on the public stream: a stream
// opened on an id that was never a card, or on a card since deleted, is refused, so
// no stream is held open for it.
func TestBoardStreamRequiresARealCard(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	if _, got, _ := problem(t, env.get(t, "/api/v1/board/NOPE99/events")); got != http.StatusNotFound {
		t.Errorf("a stream for a non-existent card = %d; want 404", got)
	}
	if players, _ := env.hub.Streams(); players != 0 {
		t.Errorf("%d player streams open for a bogus card id; want 0", players)
	}
}

// TestImagePickerReachableByMapAndTeaRoomGrantees pins the permission list behind
// the shared image picker. Both the Festival Map and Tea Room editors embed it, but
// neither permission was listed - so a grantee of either page got a 403 from the
// picker their own form depends on and could not finish the task the permission
// exists for.
func TestImagePickerReachableByMapAndTeaRoomGrantees(t *testing.T) {
	for _, perm := range []string{"festival-map", "teahouse-tea-rooms"} {
		t.Run(perm, func(t *testing.T) {
			env := newTestEnv(t)
			c := makeActiveUser(t, env, "staff-"+perm, "correct horse battery", []string{perm})
			// The permission gate is what is under test, so assert it is not the
			// thing refusing: an unknown-category 400 still means the guard passed.
			resp := getAs(t, c, env, "/api/images?dir=announcements")
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
				t.Errorf("image list for a %s grantee = %d; the picker their own editor embeds is refusing them",
					perm, resp.StatusCode)
			}
		})
	}
}

// TestAffiliateListReadableByMapGranteeWithoutWebhook covers both halves of that
// permission change: the festival map editor needs the affiliate names for its
// operator select, but the same response carries the shared Discord webhook, which
// belongs to the affiliates page. A read granted for names must not confer a secret.
func TestAffiliateListReadableByMapGranteeWithoutWebhook(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	resp := env.putJSON(t, "/api/affiliates/webhook", map[string]any{
		"webhook_url": "https://discord.com/api/webhooks/123/abc",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("seed affiliate webhook = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Read it back as admin FIRST: the guard narrows who sees the secret, it must
	// not blank it for the page that owns it.
	if got := decodeBody(t, env.get(t, "/api/affiliates"))["webhook_url"]; got == "" {
		t.Fatal("an admin lost access to the affiliate webhook")
	}

	c := makeActiveUser(t, env, "mapstaff", "correct horse battery", []string{"festival-map"})
	resp = getAs(t, c, env, "/api/affiliates")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("affiliate list for a festival-map grantee = %d; want 200 (the operator select needs it)",
			resp.StatusCode)
	}
	if got, ok := decodeBody(t, resp)["webhook_url"]; ok && got != "" {
		t.Errorf("webhook_url = %v; want it withheld from a caller who cannot edit affiliates", got)
	}
}

// TestPendingCardPreviewAllowedForCardsGrantee pins the review gate. Approving a
// pending custom card IS the Manage Cards page's job, so requiring full admin to
// preview one locked the grantee out of the task the permission was granted for.
// Anonymous callers must still be refused: a pending card is not yet playable.
func TestPendingCardPreviewAllowedForCardsGrantee(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	env.seedCardRequest(t, "PEND01", srvValidBoard())

	c := makeActiveUser(t, env, "cardstaff", "correct horse battery", []string{"bingo-cards"})
	resp := getAs(t, c, env, "/api/v1/board/PEND01?preview=1")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("pending-card preview for a bingo-cards grantee = %d; want 200", resp.StatusCode)
	}
	// The board itself too, with the game.
	if got := status(getAs(t, c, env, "/api/v1/board/PEND01")); got != http.StatusOK {
		t.Errorf("pending card's board for a bingo-cards grantee = %d; want 200", got)
	}

	// No session at all: still refused.
	anon := newClient(t, env)
	resp = getAs(t, anon, env, "/api/v1/board/PEND01?preview=1")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("anonymous pending-card preview = %d; want 403", resp.StatusCode)
	}
}

// TestAffiliateLinksAreNormalizedOnWrite pins the scheme hardening. carrd_link and
// discord_link are rendered as href on the PUBLIC festival map, and nothing on the
// write path checked their scheme - so "javascript:..." was stored verbatim and
// reached an anonymous page, with the production CSP the only thing standing in
// the way. normalizeExternalURL turns anything that is not already http(s) into an
// inert "https://<typed>" link.
func TestAffiliateLinksAreNormalizedOnWrite(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/affiliates", map[string]any{
		"name":         "Hostile",
		"carrd_link":   "javascript:alert(1)",
		"discord_link": "data:text/html,<script>alert(1)</script>",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create affiliate status = %d; want 201", resp.StatusCode)
	}
	resp.Body.Close()

	list := decodeBody(t, env.get(t, "/api/affiliates"))["affiliates"].([]any)
	if len(list) != 1 {
		t.Fatalf("affiliates = %d; want 1", len(list))
	}
	a := list[0].(map[string]any)
	for _, field := range []string{"carrd_link", "discord_link"} {
		got, _ := a[field].(string)
		if strings.HasPrefix(strings.ToLower(got), "javascript:") ||
			strings.HasPrefix(strings.ToLower(got), "data:") {
			t.Errorf("%s stored as %q - a non-http scheme reached the public page", field, got)
		}
		if !strings.HasPrefix(got, "https://") {
			t.Errorf("%s = %q; want it normalized to an https URL", field, got)
		}
	}

	// A real link is left exactly as typed.
	resp = env.postJSON(t, "/api/affiliates", map[string]any{
		"name": "Friendly", "carrd_link": "https://example.test/shop",
	})
	resp.Body.Close()
	for _, row := range decodeBody(t, env.get(t, "/api/affiliates"))["affiliates"].([]any) {
		m := row.(map[string]any)
		if m["name"] == "Friendly" && m["carrd_link"] != "https://example.test/shop" {
			t.Errorf("an ordinary https link was rewritten: %v", m["carrd_link"])
		}
	}
}
