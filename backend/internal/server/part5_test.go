package server_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
)

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
		resp := env.patchJSON(t, fmt.Sprintf("/api/v1/festival-maps/%d", id), body)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("PATCH %v status = %d; want 422", body, resp.StatusCode)
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

// TestImagePickerReachableByMapTeaRoomAndEmaGrantees pins the permission list behind
// the shared image picker. Both the Festival Map and Tea Room editors embed it, but
// neither permission was listed - so a grantee of either page got a 403 from the
// picker their own form depends on and could not finish the task the permission
// exists for.
func TestImagePickerReachableByMapTeaRoomAndEmaGrantees(t *testing.T) {
	for _, perm := range []string{"festival-map", "teahouse-tea-rooms", "festival-ema"} {
		t.Run(perm, func(t *testing.T) {
			env := newTestEnv(t)
			c := makeActiveUser(t, env, "staff-"+perm, "correct horse battery", []string{perm})
			// The permission gate is what is under test, so assert it is not the
			// thing refusing: an unknown-category 404 still means the guard passed.
			resp := getAs(t, c, env, "/api/v1/image-categories/announcements/images")
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
				t.Errorf("image list for a %s grantee = %d; the picker their own editor embeds is refusing them",
					perm, resp.StatusCode)
			}
		})
	}
}

// TestAffiliateListReadableByMapGranteeWithoutWebhook covers both halves of the list
// guard: the festival map editor needs the affiliate names for its operator select,
// but the shared Discord webhook belongs to the affiliates page. A read granted for
// names must not confer a secret.
func TestAffiliateListReadableByMapGranteeWithoutWebhook(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	decodeInto(t, env.putJSON(t, "/api/v1/affiliates/webhook", map[string]any{
		"webhookUrl": "https://discord.com/api/webhooks/123/abc",
	}), http.StatusOK, nil)

	// Read it back as admin first: the guard narrows who sees the secret, it must
	// not blank it for the page that owns it.
	var hook map[string]any
	decodeInto(t, env.get(t, "/api/v1/affiliates/webhook"), http.StatusOK, &hook)
	if hook["webhookUrl"] == "" {
		t.Fatal("an admin lost access to the affiliate webhook")
	}

	c := makeActiveUser(t, env, "mapstaff", "correct horse battery", []string{"festival-map"})
	resp := getAs(t, c, env, "/api/v1/affiliates")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("affiliate list for a festival-map grantee = %d; want 200 (the operator select needs it)",
			resp.StatusCode)
	}
	if _, ok := decodeBody(t, resp)["webhookUrl"]; ok {
		t.Error("the list carries the webhook")
	}
	if got := status(getAs(t, c, env, "/api/v1/affiliates/webhook")); got != http.StatusForbidden {
		t.Errorf("the webhook for a caller who cannot edit affiliates = %d; want 403", got)
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

	decodeInto(t, env.postJSON(t, "/api/v1/affiliates", affiliateBody("Hostile", map[string]any{
		"carrdLink":   "javascript:alert(1)",
		"discordLink": "data:text/html,<script>alert(1)</script>",
	})), http.StatusCreated, nil)

	list := decodeBody(t, env.get(t, "/api/v1/affiliates"))["affiliates"].([]any)
	if len(list) != 1 {
		t.Fatalf("affiliates = %d; want 1", len(list))
	}
	a := list[0].(map[string]any)
	for _, field := range []string{"carrdLink", "discordLink"} {
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
	decodeInto(t, env.postJSON(t, "/api/v1/affiliates", affiliateBody("Friendly", map[string]any{
		"carrdLink": "https://example.test/shop",
	})), http.StatusCreated, nil)
	for _, row := range decodeBody(t, env.get(t, "/api/v1/affiliates"))["affiliates"].([]any) {
		m := row.(map[string]any)
		if m["name"] == "Friendly" && m["carrdLink"] != "https://example.test/shop" {
			t.Errorf("an ordinary https link was rewritten: %v", m["carrdLink"])
		}
	}
}
