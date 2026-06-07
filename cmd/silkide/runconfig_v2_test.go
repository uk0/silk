package main

import (
	"reflect"
	"testing"
)

// TestRunWorkingDirRoundTrip: SetRunWorkingDir persists the directory
// override across preferences instances, mirroring the OpenSession /
// RunArgs round-trip pattern. Empty round-trips as empty (no preference).
func TestRunWorkingDirRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp) // Linux fallback path
	t.Setenv("HOME", tmp)            // macOS Application Support path

	const dir = "/Users/test/projects/widget"

	p := newPreferences()
	p.SetRunWorkingDir(dir)

	got := newPreferences().RunWorkingDir()
	if got != dir {
		t.Errorf("RunWorkingDir() = %q, want %q", got, dir)
	}

	// Clearing the override round-trips to empty (back to projectDir
	// auto-detect).
	p.SetRunWorkingDir("")
	if got := newPreferences().RunWorkingDir(); got != "" {
		t.Errorf("RunWorkingDir() after clear = %q, want empty", got)
	}
}

// TestRunEnvRoundTrip: SetRunEnv persists a "KEY=value" list across
// preferences instances. Order is preserved (downstream consumers may
// treat the list as ordered overrides) and an empty slice round-trips
// as nothing-to-restore.
func TestRunEnvRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)

	env := []string{
		"DEBUG=1",
		"PORT=9090",
		"FOO=hello world",
	}

	p := newPreferences()
	p.SetRunEnv(env)

	got := newPreferences().RunEnv()
	if !reflect.DeepEqual(got, env) {
		t.Errorf("RunEnv() = %#v, want %#v", got, env)
	}

	// Clearing yields an empty / nil read on the next launch.
	p.SetRunEnv(nil)
	if got := newPreferences().RunEnv(); len(got) != 0 {
		t.Errorf("RunEnv() after clear = %#v, want empty", got)
	}
}

// TestEffectiveRunDir: the override rule used by runProjectInTerminal —
// a non-empty prefs value wins over the auto-detected project dir;
// empty means "use the auto-detected dir as-is" (backwards-compat).
func TestEffectiveRunDir(t *testing.T) {
	cases := []struct {
		name     string
		prefsDir string
		autoDir  string
		want     string
	}{
		{"empty prefs falls back to auto", "", "/auto/dir", "/auto/dir"},
		{"non-empty prefs overrides auto", "/override", "/auto/dir", "/override"},
		{"both empty stays empty", "", "", ""},
		{"prefs wins even when auto empty", "/override", "", "/override"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := effectiveRunDir(c.prefsDir, c.autoDir)
			if got != c.want {
				t.Errorf("effectiveRunDir(%q, %q) = %q, want %q",
					c.prefsDir, c.autoDir, got, c.want)
			}
		})
	}
}

// TestConfigureRunPaletteCommandRegistered: the palette entry must keep
// pointing at the new configureRun handler (not the retired
// configureRunArgs). Catches regressions where the rename loses the
// palette wiring.
func TestConfigureRunPaletteCommandRegistered(t *testing.T) {
	saved := paletteCommands
	defer func() { paletteCommands = saved }()
	paletteCommands = nil

	registerPaletteCommands(nil, nil)

	found := false
	for _, c := range paletteCommands {
		if c.Name == "Configure Run..." {
			found = true
			if c.Fn == nil {
				t.Errorf("Configure Run... command has nil Fn")
			}
			break
		}
	}
	if !found {
		t.Errorf("paletteCommands missing %q", "Configure Run...")
	}
}
