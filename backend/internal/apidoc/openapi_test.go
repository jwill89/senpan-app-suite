package apidoc

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestSpecIsCurrent fails if the committed backend/openapi.yaml differs from a
// fresh generation — the OpenAPI equivalent of the gen:types check. Because the
// schemas are reflected from the model structs, this guarantees the documented
// shapes never drift from the code: change a struct and this test goes red until
// you run `go run ./cmd/openapi-gen`.
func TestSpecIsCurrent(t *testing.T) {
	want, err := YAML()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	got, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read committed openapi.yaml: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("backend/openapi.yaml is out of date — run `go run ./cmd/openapi-gen` and commit the result")
	}
}

// handleFuncPattern captures the routing pattern from each s.mux.HandleFunc call
// in server.go, e.g. "GET /api/version" or "/api/ws".
var handleFuncPattern = regexp.MustCompile(`HandleFunc\("([^"]+)"`)

// TestEveryRouteIsDocumented cross-checks the spec against the actual route
// registrations in server.go (the source of truth), both directions: every
// registered route (except the WebSocket upgrade, which OpenAPI can't model and
// is documented in the spec description) must appear in the spec, and every spec
// operation must correspond to a real route. This catches an endpoint added or
// changed without updating the paths table.
func TestEveryRouteIsDocumented(t *testing.T) {
	src, err := os.ReadFile("../server/server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	routes := map[string]bool{}
	for _, m := range handleFuncPattern.FindAllStringSubmatch(string(src), -1) {
		pat := m[1]
		if pat == "/api/ws" {
			continue // WebSocket upgrade — documented in the spec description, not as an operation
		}
		method, path, ok := strings.Cut(pat, " ")
		if !ok {
			t.Fatalf("unexpected route pattern %q (want \"METHOD /path\")", pat)
		}
		routes[method+" "+path] = true
	}
	if len(routes) == 0 {
		t.Fatal("no routes parsed from server.go — did the HandleFunc format change?")
	}

	doc, err := Build()
	if err != nil {
		t.Fatalf("build spec: %v", err)
	}
	spec := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			spec[method+" "+path] = true
		}
	}

	for route := range routes {
		if !spec[route] {
			t.Errorf("route %s is registered but NOT documented in the OpenAPI spec", route)
		}
	}
	for op := range spec {
		if !routes[op] {
			t.Errorf("spec documents %s but no such route is registered in server.go", op)
		}
	}
	if t.Failed() {
		t.Log("routes:", sortedKeys(routes))
		t.Log("spec:  ", sortedKeys(spec))
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
