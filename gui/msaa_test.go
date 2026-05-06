//go:build !windows

package gui

import (
	"os"
	"testing"
)

// TestMSAASampleCountDefault: with the env var unset, the default sample
// count is 4. This is the value passed straight to glfw.Samples and
// chosen because every desktop GPU since ~2008 supports it without a
// driver fallback. Changing the default would alter visual output of
// every existing app — pin it.
func TestMSAASampleCountDefault(t *testing.T) {
	t.Setenv("SILK_GLUI_MSAA", "")
	if got := msaaSampleCount(); got != 4 {
		t.Fatalf("default MSAA sample count = %d, want 4", got)
	}
}

// TestMSAASampleCountAcceptedTiers: every documented value snaps to its
// own tier so the GLFW hint never receives a value the driver might
// reject silently. 0 is the explicit-disable value and must round-trip.
func TestMSAASampleCountAcceptedTiers(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"0", 0},
		{"1", 2},  // sub-2 snaps up to the lowest tier
		{"2", 2},
		{"3", 4},  // values between tiers round up to the next supported one
		{"4", 4},
		{"5", 8},
		{"8", 8},
		{"9", 16},
		{"16", 16},
		{"32", 16}, // beyond cap — clamped
	}
	for _, tc := range cases {
		t.Setenv("SILK_GLUI_MSAA", tc.in)
		if got := msaaSampleCount(); got != tc.want {
			t.Errorf("SILK_GLUI_MSAA=%q: got %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestMSAASampleCountRejectsGarbage: non-numeric or negative input falls
// back to the default rather than crashing or being passed through to
// the driver. Defensive against malformed env values from CI or shells.
func TestMSAASampleCountRejectsGarbage(t *testing.T) {
	cases := []string{"abc", "-1", "4x", "  ", "1.5"}
	for _, in := range cases {
		t.Setenv("SILK_GLUI_MSAA", in)
		if got := msaaSampleCount(); got != 4 {
			t.Errorf("SILK_GLUI_MSAA=%q produced %d, want default 4", in, got)
		}
	}
}

// TestMSAAEnvUnsetMatchesEmptyString: explicitly clearing the env var
// (Unsetenv) must produce the same default as never setting it. Older
// Go test rigs that bypass Setenv could otherwise see leak-through
// state from a prior test.
func TestMSAAEnvUnsetMatchesEmptyString(t *testing.T) {
	os.Unsetenv("SILK_GLUI_MSAA")
	if got := msaaSampleCount(); got != 4 {
		t.Fatalf("with unset env, got %d, want 4", got)
	}
}
