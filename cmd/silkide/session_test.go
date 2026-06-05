package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExistingPathsFiltersAndDedups: existingPaths keeps only paths
// that exist on disk, preserves order, drops blanks, and de-dups.
// This is the pure filter session restore runs over OpenSession()
// before reopening, so a deleted/renamed file from the last run is
// silently skipped instead of erroring on launch.
func TestExistingPaths(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.silkui")
	b := filepath.Join(dir, "b.go")
	missing := filepath.Join(dir, "gone.go")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	in := []string{a, "", b, missing, a} // blank + missing + dup of a
	got := existingPaths(in)

	want := []string{a, b}
	if len(got) != len(want) {
		t.Fatalf("existingPaths(%v) = %v, want %v", in, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("existingPaths[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// Empty / nil input → empty (non-nil) slice, never panics.
	if out := existingPaths(nil); len(out) != 0 {
		t.Errorf("existingPaths(nil) = %v, want empty", out)
	}
}

// TestOpenSessionRoundTrip: SetOpenSession persists a path list that
// OpenSession reads back verbatim across a fresh preferences instance,
// using the same on-disk settings store the window-geometry prefs use.
func TestOpenSessionRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp) // Linux fallback path
	t.Setenv("HOME", tmp)            // macOS Application Support path

	paths := []string{
		filepath.Join(tmp, "Form.silkui"),
		filepath.Join(tmp, "main.go"),
	}

	p := newPreferences()
	p.SetOpenSession(paths)

	got := newPreferences().OpenSession()
	if len(got) != len(paths) {
		t.Fatalf("OpenSession() = %v, want %v", got, paths)
	}
	for i := range paths {
		if got[i] != paths[i] {
			t.Errorf("OpenSession()[%d] = %q, want %q", i, got[i], paths[i])
		}
	}

	// Round-tripping an empty session yields nothing to restore.
	p.SetOpenSession(nil)
	if got := newPreferences().OpenSession(); len(got) != 0 {
		t.Errorf("OpenSession() after clear = %v, want empty", got)
	}
}

// TestOpenRecentPaletteCommandRegistered: registerPaletteCommands must
// include the "Open Recent..." entry so the palette exposes the MRU
// fast-path alongside the hamburger submenu.
func TestOpenRecentPaletteCommandRegistered(t *testing.T) {
	saved := paletteCommands
	defer func() { paletteCommands = saved }()
	paletteCommands = nil

	registerPaletteCommands(nil, nil)

	found := false
	for _, c := range paletteCommands {
		if c.Name == "Open Recent..." {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("paletteCommands missing %q", "Open Recent...")
	}
}
