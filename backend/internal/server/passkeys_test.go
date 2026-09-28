package server_test

import (
	"net/http"
	"testing"
)

// TestPasskeyLoginBeginIsRateLimited pins a guard that could never fire. The
// handler checked the shared login limiter but nothing on this path ever
// incremented it, so an unauthenticated client could call begin without limit -
// and every call stores a WebAuthn challenge in the in-memory session store,
// which only goes away on expiry. It now has its own counting budget, kept
// separate from the login limiter so opening the passkey prompt repeatedly cannot
// lock the account out of password login.
func TestPasskeyLoginBeginIsRateLimited(t *testing.T) {
	env := newTestEnv(t)

	limited := false
	// The budget is 30 per 10 minutes; go well past it.
	for i := 0; i < 40; i++ {
		resp := env.postJSON(t, "/api/auth/passkey/begin", map[string]any{})
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("passkey begin never rate limited; unauthenticated callers can mint session challenges without bound")
	}

	// The shared login limiter must be untouched: password login still works.
	env.loginAdmin(t)
}
