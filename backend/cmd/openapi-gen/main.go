// Command openapi-gen writes backend/openapi.yaml from the API description built
// in internal/apidoc (schemas reflected from model, paths hand-maintained).
//
// Run from backend/:  go run ./cmd/openapi-gen
package main

import (
	"fmt"
	"os"

	"app-suite/internal/apidoc"
)

func main() {
	data, err := apidoc.YAML()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile("openapi.yaml", data, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote openapi.yaml")
}
