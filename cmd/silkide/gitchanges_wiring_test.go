package main

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
)

// TestCoreLevelToGed pins the core→ged level mapping the
// installCoreLogSink bridge routes every core log line through. The two
// enums are numerically parallel today; this table keeps the bridge
// honest if either side is ever reordered or extended, and pins the
// "unknown level reads as info" fallback.
func TestCoreLevelToGed(t *testing.T) {
	cases := []struct {
		name string
		in   core.LogLevel
		want ged.LogLevel
	}{
		{"debug", core.LevelDebug, ged.LogDebug},
		{"info", core.LevelInfo, ged.LogInfo},
		{"warn", core.LevelWarn, ged.LogWarn},
		{"error", core.LevelError, ged.LogError},
		{"unknown falls back to info", core.LogLevel(42), ged.LogInfo},
		{"negative falls back to info", core.LogLevel(-1), ged.LogInfo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := coreLevelToGed(tc.in); got != tc.want {
				t.Errorf("coreLevelToGed(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestInstallCoreLogSinkNilPanel pins the headless guard: before
// buildPanels constructs globalLog (or in tests that never do), the
// bridge must be a silent no-op rather than registering a sink that
// posts into a panel that will never exist.
func TestInstallCoreLogSinkNilPanel(t *testing.T) {
	if globalLog != nil {
		t.Skip("globalLog unexpectedly constructed in tests")
	}
	installCoreLogSink() // must not panic and must not register
}

// TestRefreshGitChangesNilPanel pins the save-path hook's guard: the
// end of saveActiveEditorToDisk calls refreshGitChanges(globalCanvas)
// unconditionally, so with no GitChangesPanel built (headless tests)
// the call must be a silent no-op.
func TestRefreshGitChangesNilPanel(t *testing.T) {
	if globalGitChanges != nil {
		t.Skip("globalGitChanges unexpectedly constructed in tests")
	}
	refreshGitChanges(nil) // must not panic, must not spawn the git goroutine
}
