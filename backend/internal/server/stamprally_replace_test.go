package server_test

import (
	"fmt"
	"net/http"
	"testing"
)

// rallyDetail fetches a rally's admin detail body.
func (e *testEnv) rallyDetail(t *testing.T, id int) map[string]any {
	t.Helper()
	resp := e.get(t, fmt.Sprintf("/api/stamp-rallies/%d", id))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rally detail status = %d; want 200", resp.StatusCode)
	}
	return decodeBody(t, resp)["stamp_rally"].(map[string]any)
}

// countOf returns the length of a detail body's child collection, treating an
// omitted key as zero - the admin LIST omits stamps/prizes entirely, which is the
// whole reason a save has to distinguish "absent" from "empty".
func countOf(m map[string]any, key string) int {
	v, _ := m[key].([]any)
	return len(v)
}

// TestStampRally_SaveWithoutChildKeysKeepsThem covers the full-replace hazard on
// the rally side. A rally save replaces its stamps and prizes wholesale, so an
// editor opened from a list row - which carries neither - deleted every stamp on
// the card and, with them, every participant's collected rows. An omitted key now
// means "leave them alone" while an explicit empty array still clears them.
func TestStampRally_SaveWithoutChildKeysKeepsThem(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createRally(t, "Summer Rally", false)
	before := env.rallyDetail(t, id)
	wantStamps, wantPrizes := countOf(before, "stamps"), countOf(before, "prizes")
	if wantStamps == 0 || wantPrizes == 0 {
		t.Fatalf("fixture should have stamps and prizes; got %d/%d", wantStamps, wantPrizes)
	}

	// The list-row shape: a save carrying neither collection.
	resp := env.putJSON(t, fmt.Sprintf("/api/stamp-rallies/%d", id), map[string]any{
		"title":   "Summer Rally (renamed)",
		"details": "Now with a typo fixed",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save without child keys status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	after := env.rallyDetail(t, id)
	if got := countOf(after, "stamps"); got != wantStamps {
		t.Errorf("stamps after a childless save = %d; want %d untouched", got, wantStamps)
	}
	if got := countOf(after, "prizes"); got != wantPrizes {
		t.Errorf("prizes after a childless save = %d; want %d untouched", got, wantPrizes)
	}
	if after["title"] != "Summer Rally (renamed)" {
		t.Errorf("the edit itself did not save: title = %v", after["title"])
	}

	// Explicit empty arrays still mean "this rally has none".
	resp = env.putJSON(t, fmt.Sprintf("/api/stamp-rallies/%d", id), map[string]any{
		"title":  "Summer Rally (renamed)",
		"stamps": []map[string]any{},
		"prizes": []map[string]any{},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save with empty children status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	emptied := env.rallyDetail(t, id)
	if got := countOf(emptied, "stamps"); got != 0 {
		t.Errorf("stamps after an explicit empty save = %d; want 0", got)
	}
	if got := countOf(emptied, "prizes"); got != 0 {
		t.Errorf("prizes after an explicit empty save = %d; want 0", got)
	}
}
