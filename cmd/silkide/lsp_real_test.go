package main

import (
	"os"
	"testing"

	"github.com/uk0/silk/internal/idetools"
)

// Release/Windows QA sets REQUIRED so absence or accidental -short is a hard
// failure. Source-only quick tests can still opt out without downloading tools.
func requireRealGopls(t *testing.T) string {
	t.Helper()
	required := os.Getenv("SILK_TEST_REQUIRE_GOPLS") == "1" || os.Getenv("SILK_TEST_REQUIRE_BUNDLED_GOPLS") == "1"
	if testing.Short() {
		if required {
			t.Fatal("required gopls integration cannot run with -short")
		}
		t.Skip("real gopls integration")
	}
	tool, err := idetools.Resolve()
	if err != nil {
		if required {
			t.Fatal(err)
		}
		t.Skipf("real gopls unavailable: %v", err)
	}
	if os.Getenv("SILK_TEST_REQUIRE_BUNDLED_GOPLS") == "1" && tool.Source != "bundled" {
		t.Fatalf("required bundled gopls, got %s at %s", tool.Source, tool.Path)
	}
	return tool.Path
}
