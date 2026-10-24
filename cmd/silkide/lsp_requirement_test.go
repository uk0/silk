package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequiredGoplsNeverSkips(t *testing.T) {
	if os.Getenv("SILK_QA_GOPLS_REQUIREMENT_CHILD") == "1" {
		requireRealGopls(t)
		t.Fatal("invalid explicit tool unexpectedly resolved")
	}
	for _, flag := range []string{"SILK_TEST_REQUIRE_GOPLS", "SILK_TEST_REQUIRE_BUNDLED_GOPLS"} {
		for _, short := range []bool{false, true} {
			name := flag + "/normal"
			if short {
				name = flag + "/short"
			}
			t.Run(name, func(t *testing.T) {
				exe, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				args := []string{"-test.run=^TestRequiredGoplsNeverSkips$", "-test.v"}
				if short {
					args = append(args, "-test.short")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, exe, args...)
				cmd.WaitDelay = time.Second
				for _, e := range os.Environ() {
					k, _, _ := strings.Cut(e, "=")
					switch k {
					case "SILK_TEST_REQUIRE_GOPLS", "SILK_TEST_REQUIRE_BUNDLED_GOPLS", "SILKIDE_GOPLS", "SILK_QA_GOPLS_REQUIREMENT_CHILD":
						continue
					}
					cmd.Env = append(cmd.Env, e)
				}
				cmd.Env = append(cmd.Env, "SILK_QA_GOPLS_REQUIREMENT_CHILD=1", flag+"=1", "SILKIDE_GOPLS="+filepath.Join(t.TempDir(), "missing-gopls"))
				output, err := cmd.CombinedOutput()
				if err == nil || strings.Contains(string(output), "--- SKIP:") || !strings.Contains(string(output), "--- FAIL: TestRequiredGoplsNeverSkips") {
					t.Fatalf("required integration skipped or did not run: %v\n%s", err, output)
				}
			})
		}
	}
}
