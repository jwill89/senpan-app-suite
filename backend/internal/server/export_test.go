package server

import (
	"context"
	"net/http"
	"net/url"
)

// RouteAnilistForTest points the server's AniList client at targetURL (a test
// server) by rewriting each outbound request's scheme and host, so a test can keep a
// valid, allowlisted anilist_api_url setting - exercising the allowlist end to end -
// while still reaching its local mock.
func (s *Server) RouteAnilistForTest(targetURL string) error {
	target, err := url.Parse(targetURL)
	if err != nil {
		return err
	}
	s.anilist.HTTP.Transport = rewriteHostRoundTripper{scheme: target.Scheme, host: target.Host}
	return nil
}

// rewriteHostRoundTripper redirects every request to a fixed scheme+host,
// leaving the path/query intact (test-only; see RouteAnilistForTest).
type rewriteHostRoundTripper struct{ scheme, host string }

func (rt rewriteHostRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.scheme
	req.URL.Host = rt.host
	return http.DefaultTransport.RoundTrip(req)
}

// The following expose pure, unexported helpers to the external server_test
// package so their logic can be unit-tested without spinning up the HTTP server.
var (
	// AdminMutationResourceForTest maps a write's path to the resource_changed
	// resource it tells, including the deliberate exclusion of the public
	// ".../enter" and ".../draw" paths.
	AdminMutationResourceForTest = adminMutationResource
	// AccountMutationForTest reports whether a write's path has the staff streams
	// check their accounts again.
	AccountMutationForTest = accountMutation
	// LogClientIPForTest exposes the CF-Connecting-IP-first client-IP extraction
	// used for the request log's ip field.
	LogClientIPForTest = logClientIP
	// ExpireDueTeaRoomLocksForTest runs ONE pass of the lock-expiry sweep, so what
	// the scheduler decides - unlock and announce, or leave the lock alone - can be
	// exercised without waiting on its ticker.
	ExpireDueTeaRoomLocksForTest = (*Server).expireDueTeaRoomLocks
)

// DataDirForTest exposes the server's dataDir so external tests can seed
// upload files mid-test (e.g. adding a font after the env is built).
func (s *Server) DataDirForTest() string { return s.dataDir }

// FontSecretForTest is the key the font link tokens are made with, so a test can make
// a token of an earlier time bucket.
func (s *Server) FontSecretForTest(ctx context.Context) ([]byte, error) { return s.fonts.Secret(ctx) }
