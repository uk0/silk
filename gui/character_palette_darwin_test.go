//go:build darwin && cgo

package gui

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinNativeCharacterPalettePolicy(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "palette-policy")
	out, err := exec.Command("xcrun", "clang", "-fblocks", "-framework", "Cocoa", "testdata/character_palette.m", "-o", exe).CombinedOutput()
	if err != nil {
		t.Fatalf("compile native palette policy: %v\n%s", err, out)
	}
	out, err = exec.Command(exe).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "NATIVE_CHARACTER_PALETTE_POLICY_OK") {
		t.Fatalf("native palette policy: %v\n%s", err, out)
	}
}
