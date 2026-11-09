// Command schema-gen writes backend/schema.sql: the schema the migrations build,
// committed so every schema change shows in review. Run it from backend/ after
// adding a migration; TestSchemaIsCurrent fails until the result is committed.
package main

import (
	"context"
	"fmt"
	"os"

	"app-suite/internal/store"
)

func main() {
	schema, err := store.Schema(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "schema-gen:", err)
		os.Exit(1)
	}
	if err := os.WriteFile("schema.sql", []byte(schema), 0o644); err != nil { //nolint:gosec // a committed source file
		fmt.Fprintln(os.Stderr, "schema-gen:", err)
		os.Exit(1)
	}
}
