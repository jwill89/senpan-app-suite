package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

// The /api layer every API request passes through, the huma operations' and the
// hand-registered routes' alike:
//
//   - a path the mux doesn't know answers problem details, 404, or 405 keeping Allow,
//     rather than the mux's plain text;
//   - a trailing slash answers as the path without it;
//   - an answer is private, no-store unless its operation says otherwise; one that
//     says no-cache (a public read) gets an ETag, and a request that already holds it
//     gets 304 with no body.

// writeProblem writes an RFC 9457 problem details body: huma's shape, which every
// operation's errors take too.
func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Title  string `json:"title"`
		Status int    `json:"status"`
		Detail string `json:"detail,omitempty"`
	}{http.StatusText(status), status, detail})
}

// routeAPI dispatches an /api request on the mux, after the trailing-slash rule, and
// answers problem details for a path or method the mux doesn't have.
func (s *Server) routeAPI(w http.ResponseWriter, r *http.Request) {
	if _, pattern := s.mux.Handler(r); pattern == "" {
		if trimmed := strings.TrimSuffix(r.URL.Path, "/"); trimmed != r.URL.Path && strings.HasPrefix(trimmed, "/api/") {
			again := r.Clone(r.Context())
			again.URL.Path, again.URL.RawPath = trimmed, ""
			if _, pattern := s.mux.Handler(again); pattern != "" {
				s.mux.ServeHTTP(w, again)
				return
			}
		}
		// The mux's own answer says which: 404, or 405 with the methods it has.
		probe := &probeWriter{header: http.Header{}, status: http.StatusOK}
		s.mux.ServeHTTP(probe, r)
		if allow := probe.header.Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		switch probe.status {
		case http.StatusMethodNotAllowed:
			writeProblem(w, probe.status, r.Method+" isn't allowed here.")
		default:
			writeProblem(w, http.StatusNotFound, "There is no such endpoint.")
		}
		return
	}
	s.mux.ServeHTTP(w, r)
}

// probeWriter takes the mux's not-found or method-not-allowed answer, to read its
// status and Allow header, and drops its body.
type probeWriter struct {
	header http.Header
	status int
}

func (p *probeWriter) Header() http.Header         { return p.header }
func (p *probeWriter) Write(b []byte) (int, error) { return len(b), nil }
func (p *probeWriter) WriteHeader(status int)      { p.status = status }

// withCaching applies the caching baseline: every answer starts private, no-store,
// and a GET answered 200 with no-cache is held to add its ETag and to answer a
// matching If-None-Match with 304.
func withCaching(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		ew := &etagWriter{ResponseWriter: w}
		next.ServeHTTP(ew, r)
		ew.finish(r)
	})
}

// etagWriter holds a revalidated answer's body until it is complete, since its ETag
// is a hash of it; any other answer passes straight through. An event stream never
// completes, so it always passes through, and Unwrap and Flush let it flush.
type etagWriter struct {
	http.ResponseWriter
	status  int
	holding bool
	body    bytes.Buffer
}

func (e *etagWriter) WriteHeader(status int) {
	if e.status != 0 {
		return
	}
	e.status = status
	if status == http.StatusOK && e.Header().Get("Cache-Control") == "no-cache" &&
		!strings.HasPrefix(e.Header().Get("Content-Type"), "text/event-stream") {
		e.holding = true
		return
	}
	e.ResponseWriter.WriteHeader(status)
}

func (e *etagWriter) Unwrap() http.ResponseWriter { return e.ResponseWriter }

// Flush sends what has been written so far, unless the answer is being held for its
// ETag.
func (e *etagWriter) Flush() {
	if e.status == 0 {
		e.WriteHeader(http.StatusOK)
	}
	if !e.holding {
		_ = http.NewResponseController(e.ResponseWriter).Flush()
	}
}

func (e *etagWriter) Write(b []byte) (int, error) {
	if e.status == 0 {
		e.WriteHeader(http.StatusOK)
	}
	if e.holding {
		return e.body.Write(b)
	}
	return e.ResponseWriter.Write(b)
}

func (e *etagWriter) finish(r *http.Request) {
	if !e.holding {
		return
	}
	sum := sha256.Sum256(e.body.Bytes())
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	e.Header().Set("ETag", etag)
	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		e.Header().Del("Content-Length")
		e.Header().Del("Content-Type")
		e.ResponseWriter.WriteHeader(http.StatusNotModified)
		return
	}
	e.ResponseWriter.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = e.ResponseWriter.Write(e.body.Bytes())
	}
}

// matchesETag reports whether an If-None-Match header names etag, weakly compared
// as RFC 9110 has it for If-None-Match, or is "*".
func matchesETag(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}
