//go:build darwin && cgo

package gui

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Compile the actual Objective-C routing predicate with responder doubles.
// This isolates AppKit ownership rules; real key delivery is separate UI QA.
func TestDarwinNativeTabNavigationPolicy(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "tab-policy")
	out, err := exec.Command("xcrun", "clang", "-fblocks", "-framework", "Cocoa", "testdata/native_tab_navigation.m", "-o", exe).CombinedOutput()
	if err != nil {
		t.Fatalf("compile native tab policy: %v\n%s", err, out)
	}
	out, err = exec.Command(exe).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "NATIVE_TAB_POLICY_OK") {
		t.Fatalf("native tab policy: %v\n%s", err, out)
	}
}
