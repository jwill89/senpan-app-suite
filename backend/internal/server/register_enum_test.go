package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"app-suite/internal/store"
)

// TestRegisterNoEnumeration verifies POST /api/register returns an IDENTICAL
// generic response (status + body) whether the username is brand new, already
// taken, or the reserved bootstrap "admin" — so the endpoint can't be used to
// enumerate which accounts exist. The eligible account is still created.
func TestRegisterNoEnumeration(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	s := &Server{store: st, regLimiter: newRateLimiter(100, time.Hour)} // no turnstile

	post := func(username string) (int, string) {
		b, _ := json.Marshal(map[string]string{"username": username, "password": "password123"})
		req := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(b))
		rec := httptest.NewRecorder()
		s.handleRegister(rec, req)
		return rec.Code, strings.TrimSpace(rec.Body.String())
	}

	newCode, newBody := post("brandnew")       // freshly created
	dupCode, dupBody := post("brandnew")       // now already taken
	resCode, resBody := post(reservedUsername) // reserved bootstrap account

	if newCode != http.StatusOK || dupCode != http.StatusOK || resCode != http.StatusOK {
		t.Fatalf("all attempts must return 200; got new=%d dup=%d reserved=%d", newCode, dupCode, resCode)
	}
	if newBody != dupBody || newBody != resBody {
		t.Errorf("responses must be identical (no enumeration):\n new=%q\n dup=%q\n reserved=%q", newBody, dupBody, resBody)
	}

	// The brand-new account must actually have been created (inactive).
	u, _, err := st.GetUserByUsername("brandnew")
	if err != nil {
		t.Fatal(err)
	}
	if u == nil {
		t.Error("eligible registration must still create the account")
	}
}
