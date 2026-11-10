package server_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// affiliateBody is a full affiliate save with the given name; extra overrides or
// adds fields.
func affiliateBody(name string, extra map[string]any) map[string]any {
	body := map[string]any{
		"name":        name,
		"subtitle":    "",
		"owners":      []string{"Tataru", "  ", "Hildibrand"}, // the blank one is dropped
		"location":    "Ul'dah",
		"timezone":    "America/New_York",
		"hours":       []map[string]any{{"label": "Mon-Fri", "start": "18:00", "end": "23:00"}, {"label": "Closed row", "start": "  ", "end": ""}},
		"details":     "Cozy.",
		"logo":        "images/affiliate_logos/x.png",
		"screenshot":  "images/affiliate_images/x.png",
		"embedColor":  "",
		"discordLink": "",
		"carrdLink":   "",
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

// createAffiliate makes an affiliate and returns its id.
func (e *testEnv) createAffiliate(t *testing.T, name string) int {
	t.Helper()
	var made map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/affiliates", affiliateBody(name, nil)), http.StatusCreated, &made)
	return int(made["id"].(float64))
}

// affiliateIDs is the ids of the listed affiliates, in order.
func affiliateIDs(t *testing.T, e *testEnv) []int {
	t.Helper()
	var list struct {
		Affiliates []map[string]any `json:"affiliates"`
	}
	decodeInto(t, e.get(t, "/api/v1/affiliates"), http.StatusOK, &list)
	ids := []int{}
	for _, a := range list.Affiliates {
		ids = append(ids, int(a["id"].(float64)))
	}
	return ids
}

// The list needs any one of the three pages that name an operator; nobody gets 401,
// anyone else 403; every other operation needs the Affiliates page.
func TestAffiliatesGuard(t *testing.T) {
	env := newTestEnv(t)
	if got := status(env.get(t, "/api/v1/affiliates")); got != http.StatusUnauthorized {
		t.Errorf("signed out: %d, want 401", got)
	}
	if got := status(env.postJSON(t, "/api/v1/affiliates", affiliateBody("X", nil))); got != http.StatusUnauthorized {
		t.Errorf("a create signed out: %d, want 401", got)
	}

	addAccount(t, env, "other", "correct horse battery", []string{"bingo-cards"})
	env.signIn(t, "other", "correct horse battery")
	if got := status(env.get(t, "/api/v1/affiliates")); got != http.StatusForbidden {
		t.Errorf("without a page: %d, want 403", got)
	}

	for _, page := range []string{"teahouse-affiliates", "festival-map", "festival-stamp-rally"} {
		name := "reader-" + page
		addAccount(t, env, name, "correct horse battery", []string{page})
		env.signIn(t, name, "correct horse battery")
		if got := status(env.get(t, "/api/v1/affiliates")); got != http.StatusOK {
			t.Errorf("list with %s: %d, want 200", page, got)
		}
		wantWrite := http.StatusForbidden
		if page == "teahouse-affiliates" {
			wantWrite = http.StatusCreated
		}
		if got := status(env.postJSON(t, "/api/v1/affiliates", affiliateBody("By "+page, nil))); got != wantWrite {
			t.Errorf("create with %s: %d, want %d", page, got, wantWrite)
		}
	}
}

// An affiliate is made (201, in full, with blank owners and start-less hours
// dropped), listed, replaced, and deleted (204, then 404).
func TestAffiliatesLifecycle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/affiliates", affiliateBody(" The Tipsy Moogle ", nil)), http.StatusCreated, &made)
	if made["name"] != "The Tipsy Moogle" || len(made["owners"].([]any)) != 2 || len(made["hours"].([]any)) != 1 ||
		made["createdAt"] == nil || made["sortOrder"] != float64(0) {
		t.Errorf("made: %v", made)
	}
	id := int(made["id"].(float64))
	path := fmt.Sprintf("/api/v1/affiliates/%d", id)

	var saved map[string]any
	decodeInto(t, env.putJSON(t, path, affiliateBody("Renamed", map[string]any{"owners": []string{"Solo"}})), http.StatusOK, &saved)
	if saved["name"] != "Renamed" || len(saved["owners"].([]any)) != 1 || saved["id"] != made["id"] {
		t.Errorf("replaced: %v", saved)
	}
	if got := status(env.putJSON(t, "/api/v1/affiliates/999999", affiliateBody("Lost", nil))); got != http.StatusNotFound {
		t.Errorf("replacing an unknown affiliate: %d, want 404", got)
	}

	if got := status(env.del(t, path)); got != http.StatusNoContent {
		t.Fatalf("delete: %d, want 204", got)
	}
	if got := status(env.del(t, path)); got != http.StatusNotFound {
		t.Errorf("deleting it again: %d, want 404", got)
	}
	if ids := affiliateIDs(t, env); len(ids) != 0 {
		t.Errorf("affiliates after the delete: %v", ids)
	}
}

// A blank name is a 422 on a create and a replace alike.
func TestAffiliatesValidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createAffiliate(t, "Original")

	for _, name := range []string{"", "   "} {
		if got := status(env.postJSON(t, "/api/v1/affiliates", affiliateBody(name, nil))); got != http.StatusUnprocessableEntity {
			t.Errorf("create named %q: %d, want 422", name, got)
		}
		if got := status(env.putJSON(t, fmt.Sprintf("/api/v1/affiliates/%d", id), affiliateBody(name, nil))); got != http.StatusUnprocessableEntity {
			t.Errorf("replace named %q: %d, want 422", name, got)
		}
	}
}

// A reorder answers the list as it is now; an empty list, a repeat and an unknown id
// are each a 422.
func TestAffiliatesReorder(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	a := env.createAffiliate(t, "Alpha")
	b := env.createAffiliate(t, "Bravo")
	c := env.createAffiliate(t, "Charlie")

	var list struct {
		Affiliates []map[string]any `json:"affiliates"`
	}
	decodeInto(t, env.postJSON(t, "/api/v1/affiliates/reorder", map[string]any{"orderedIds": []int{c, a, b}}), http.StatusOK, &list)
	got := []int{}
	for _, af := range list.Affiliates {
		got = append(got, int(af["id"].(float64)))
	}
	if want := []int{c, a, b}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("order after reorder = %v; want %v", got, want)
	}
	if ids := affiliateIDs(t, env); fmt.Sprint(ids) != fmt.Sprint([]int{c, a, b}) {
		t.Errorf("listed order = %v", ids)
	}

	for name, body := range map[string]any{
		"empty":   map[string]any{"orderedIds": []int{}},
		"repeat":  map[string]any{"orderedIds": []int{a, a}},
		"unknown": map[string]any{"orderedIds": []int{a, 999999}},
	} {
		if got := status(env.postJSON(t, "/api/v1/affiliates/reorder", body)); got != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d, want 422", name, got)
		}
	}
	if ids := affiliateIDs(t, env); fmt.Sprint(ids) != fmt.Sprint([]int{c, a, b}) {
		t.Errorf("a refused reorder changed the order: %v", ids)
	}
}

// The webhook is saved (trimmed), read back, cleared, and refused when it isn't a
// Discord webhook.
func TestAffiliatesWebhook(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var hook map[string]any
	decodeInto(t, env.get(t, "/api/v1/affiliates/webhook"), http.StatusOK, &hook)
	if hook["webhookUrl"] != "" {
		t.Errorf("unset: %v", hook)
	}

	const url = "https://discord.com/api/webhooks/123/abc"
	decodeInto(t, env.putJSON(t, "/api/v1/affiliates/webhook", map[string]any{"webhookUrl": " " + url + " "}), http.StatusOK, &hook)
	if hook["webhookUrl"] != url {
		t.Errorf("saved: %v", hook)
	}
	decodeInto(t, env.get(t, "/api/v1/affiliates/webhook"), http.StatusOK, &hook)
	if hook["webhookUrl"] != url {
		t.Errorf("read back: %v", hook)
	}

	resp := env.putJSON(t, "/api/v1/affiliates/webhook", map[string]any{"webhookUrl": "https://evil.example.com/x"})
	var problem struct {
		Errors []struct {
			Location string `json:"location"`
		} `json:"errors"`
	}
	decodeInto(t, resp, http.StatusUnprocessableEntity, &problem)
	if len(problem.Errors) != 1 || problem.Errors[0].Location != "body.webhookUrl" {
		t.Errorf("problem: %+v", problem)
	}
	decodeInto(t, env.get(t, "/api/v1/affiliates/webhook"), http.StatusOK, &hook)
	if hook["webhookUrl"] != url {
		t.Errorf("a refused save changed the webhook: %v", hook)
	}

	decodeInto(t, env.putJSON(t, "/api/v1/affiliates/webhook", map[string]any{"webhookUrl": ""}), http.StatusOK, &hook)
	if hook["webhookUrl"] != "" {
		t.Errorf("cleared: %v", hook)
	}
}

// useAffiliateWebhook points the affiliates' webhook at a test server that answers
// with answer, and counts the posts it gets.
func useAffiliateWebhook(t *testing.T, env *testEnv, answer http.HandlerFunc) *atomic.Int32 {
	t.Helper()
	var posts atomic.Int32
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		answer(w, r)
	}))
	t.Cleanup(hook.Close)
	if err := env.store.SetSetting(t.Context(), "affiliate_webhook_url", hook.URL); err != nil {
		t.Fatal(err)
	}
	return &posts
}

// Posting an affiliate answers it; a missing one is a 404 and no webhook a 500 that
// never calls Discord.
func TestAffiliatesPost(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createAffiliate(t, "Poster")
	path := fmt.Sprintf("/api/v1/affiliates/%d/post", id)

	var problem struct {
		Detail string `json:"detail"`
	}
	decodeInto(t, env.postJSON(t, path, nil), http.StatusInternalServerError, &problem)
	if !strings.Contains(problem.Detail, "No Discord webhook is set for the affiliates") {
		t.Errorf("no webhook: %q", problem.Detail)
	}

	var body string
	posts := useAffiliateWebhook(t, env, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusNoContent)
	})
	var posted map[string]any
	decodeInto(t, env.postJSON(t, path, nil), http.StatusOK, &posted)
	if posted["name"] != "Poster" || posts.Load() != 1 || !strings.Contains(body, "Poster") {
		t.Errorf("posted: %v after %d posts, body %q", posted, posts.Load(), body)
	}

	if got := status(env.postJSON(t, "/api/v1/affiliates/999999/post", nil)); got != http.StatusNotFound {
		t.Errorf("an unknown affiliate: %d, want 404", got)
	}
	if posts.Load() != 1 {
		t.Errorf("an unknown affiliate posted anyway: %d posts", posts.Load())
	}
}

// A post Discord refuses, limits or can't answer is a 500, 429 (with Retry-After) or
// 503, never a 502.
func TestAffiliatesPostDiscordFailures(t *testing.T) {
	cases := []struct {
		name      string
		answer    http.HandlerFunc
		want      int
		wantRetry string
	}{
		{"refused", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }, http.StatusInternalServerError, ""},
		{"rate limited", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
		}, http.StatusTooManyRequests, "3"},
		{"Discord down", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }, http.StatusServiceUnavailable, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.loginAdmin(t)
			id := env.createAffiliate(t, "Poster")
			useAffiliateWebhook(t, env, tc.answer)

			resp := env.postJSON(t, fmt.Sprintf("/api/v1/affiliates/%d/post", id), nil)
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d; want %d", resp.StatusCode, tc.want)
			}
			if got := resp.Header.Get("Retry-After"); got != tc.wantRetry {
				t.Errorf("Retry-After = %q; want %q", got, tc.wantRetry)
			}
		})
	}
}
