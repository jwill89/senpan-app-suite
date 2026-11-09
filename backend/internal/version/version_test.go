package version

import (
	"encoding/json"
	"os"
	"testing"
)

// TestVersionIsTheApps holds Version to the app's one version, frontend/package.json's,
// which CI tags and the deploy ships.
func TestVersionIsTheApps(t *testing.T) {
	raw, err := os.ReadFile("../../../frontend/package.json")
	if err != nil {
		t.Fatalf("read frontend/package.json: %v", err)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatalf("parse frontend/package.json: %v", err)
	}
	if pkg.Version != Version {
		t.Fatalf("version.Version is %q, but frontend/package.json says %q; a release sets both", Version, pkg.Version)
	}
}
