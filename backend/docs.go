package main

import _ "embed"

// openAPISpec is the committed OpenAPI document, embedded so the server binary
// is self-contained (served at GET /api/openapi.yaml, rendered by GET /api/docs).
// Regenerate with `go run ./cmd/openapi-gen`; CI checks it stays current.
//
//go:embed openapi.yaml
var openAPISpec []byte
