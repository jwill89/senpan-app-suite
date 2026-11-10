package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"app-suite/internal/auth"
	"app-suite/internal/db"
	"app-suite/internal/live"
	"app-suite/internal/store"
	"app-suite/internal/store/storetest"
)

// newInternalServer is a whole server on a fresh database, for a test inside the
// package that sets what New doesn't take (such as the Turnstile secret).
func newInternalServer(t *testing.T) (*Server, *store.Store, *httptest.Server) {
	t.Helper()
	st, err := store.New(t.Context(), storetest.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := New(t.Context(), st, live.New(), t.TempDir(), nil)
	ts := httptest.NewTLSServer(s)
	t.Cleanup(ts.Close)
	return s, st, ts
}

// postFrom sends body as JSON to path and answers the status and the body read.
func postFrom(t *testing.T, ts *httptest.Server, path string, body any) (int, string) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := ts.Client().Post(ts.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(got)
}

// TestRegisterNoEnumeration: a sign-up answers identically - status and body -
// whether the name is new, already taken, or the reserved bootstrap "admin", so it
// can't be used to find out which accounts exist. The eligible account is still made.
func TestRegisterNoEnumeration(t *testing.T) {
	_, st, ts := newInternalServer(t)
	post := func(username string) (int, string) {
		return postFrom(t, ts, "/api/v1/auth/register", map[string]string{"username": username, "password": "password123"})
	}

	newCode, newBody := post("brandnew")            // made
	dupCode, dupBody := post("brandnew")            // now taken
	resCode, resBody := post(auth.ReservedUsername) // the bootstrap account

	if newCode != http.StatusAccepted || dupCode != http.StatusAccepted || resCode != http.StatusAccepted {
		t.Fatalf("every sign-up must answer 202; got new=%d taken=%d reserved=%d", newCode, dupCode, resCode)
	}
	if newBody != dupBody || newBody != resBody {
		t.Errorf("the answers must be identical:\n new=%q\n taken=%q\n reserved=%q", newBody, dupBody, resBody)
	}
	row, err := db.New(st.ReadDB()).GetUserByUsername(t.Context(), "brandnew")
	if err != nil {
		t.Fatalf("the eligible sign-up must still make the account: %v", err)
	}
	if row.IsActive != 0 {
		t.Error("a new account must wait for an admin to activate it")
	}
}
