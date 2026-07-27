package workspace

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestSaveLoadRoundTrip: a fully populated session survives Save -> Load
// unchanged — per-tab cursor / scroll / folds, the pane split, dock state
// and the unsaved buffers. Save also has to create the parent directory,
// so the path points into one that does not exist yet.
func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "session.json")
	want := &Session{
		Project: "demo.silkui",
		Tabs: []Tab{
			{Path: "b.go", Cursor: Pos{Line: 12, Col: 4}, Scroll: 8, Folds: []int{3, 40}},
			{Path: "a.go"},
		},
		ActiveTab: 1,
		Panes:     []Pane{{Tabs: []int{0}, Active: 0}, {Tabs: []int{1}, Active: 0}},
		Split:     Split{Orientation: "h", Ratio: 0.5},
		DockState: map[string]bool{"ged.BookmarksPanel": true, "ged.ReferencesPanel": false},
		Unsaved:   map[string]string{"a.go": "package main\n"},
	}
	if err := want.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Save stamps the version; the literal above deliberately did not.
	if want.Version != 0 {
		t.Errorf("Save mutated the receiver's Version: %d, want 0", want.Version)
	}
	want.Version = Version
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

// TestLoadMigratesV0PathList: the pre-versioning format — the whole file
// being a JSON array of paths — loads as ordered tabs instead of failing.
// Empty and repeated entries drop out, matching what silkide's
// existingPaths filter did on the way back in.
func TestLoadMigratesV0PathList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := []byte(`["design.silkui", "main.go", "main.go", ""]`)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load of a v0 path list: %v", err)
	}
	if s.Version != Version {
		t.Errorf("Version = %d, want %d (not migrated)", s.Version, Version)
	}
	want := []string{"design.silkui", "main.go"}
	if got := s.Paths(); !reflect.DeepEqual(got, want) {
		t.Errorf("Paths() = %v, want %v", got, want)
	}
	if s.ActiveTab != 0 {
		t.Errorf("ActiveTab = %d, want 0", s.ActiveTab)
	}
}

// TestLoadMigratesV0Object: the flat object ged.SaveSession wrote carries
// no version key, so open_files becomes the ordered tabs, active_file
// becomes ActiveTab and last_project becomes Project. Keys v1 dropped
// (last_mode, the window geometry) are ignored rather than fatal.
func TestLoadMigratesV0Object(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := []byte(`{"last_mode":1,"open_files":["a.go","b.go","c.go"],` +
		`"active_file":"b.go","last_project":"p.silkui","window_width":1200}`)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load of a v0 object: %v", err)
	}
	if s.Version != Version {
		t.Errorf("Version = %d, want %d (not migrated)", s.Version, Version)
	}
	if s.Project != "p.silkui" {
		t.Errorf("Project = %q, want %q", s.Project, "p.silkui")
	}
	want := []string{"a.go", "b.go", "c.go"}
	if got := s.Paths(); !reflect.DeepEqual(got, want) {
		t.Errorf("Paths() = %v, want %v", got, want)
	}
	if s.ActiveTab != 1 {
		t.Errorf("ActiveTab = %d, want 1 (active_file b.go)", s.ActiveTab)
	}
}

// TestLoadPreservesTabOrder: tabs come back in the order they were saved,
// not sorted and not shuffled. This is the regression the package exists
// for — currentSessionPaths built its list by ranging a map, so the
// restored tab bar was in a different order every launch.
func TestLoadPreservesTabOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	order := []string{"zeta.go", "alpha.go", "main.go", "beta.go", "core.go", "util.go"}
	s := FromPaths(order)
	s.ActiveTab = 3
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if paths := got.Paths(); !reflect.DeepEqual(paths, order) {
		t.Fatalf("Paths() = %v, want %v", paths, order)
	}
	if got.ActiveTab != 3 {
		t.Errorf("ActiveTab = %d, want 3", got.ActiveTab)
	}
}

// TestSaveDeterministicBytes: the file is a function of the session only.
// Two sessions that differ solely in the order their fold lists and maps
// were built serialize to identical bytes, and saving one twice produces
// the same bytes both times.
func TestSaveDeterministicBytes(t *testing.T) {
	dir := t.TempDir()
	tabs := func(folds []int) []Tab {
		return []Tab{{Path: "z.go", Folds: folds}, {Path: "a.go", Scroll: 5}}
	}
	// Folds as the editor's map hands them over: unsorted, with a repeat.
	a := &Session{
		Tabs:      tabs([]int{9, 2, 2}),
		ActiveTab: 1,
		DockState: map[string]bool{"b": true, "a": false, "c": true},
		Unsaved:   map[string]string{"z.go": "x", "a.go": "y"},
	}
	// The same session with the fold list already tidy and both maps
	// written in a different order.
	b := &Session{
		Tabs:      tabs([]int{2, 9}),
		ActiveTab: 1,
		DockState: map[string]bool{"c": true, "a": false, "b": true},
		Unsaved:   map[string]string{"a.go": "y", "z.go": "x"},
	}

	save := func(s *Session, name string) []byte {
		p := filepath.Join(dir, name)
		if err := s.Save(p); err != nil {
			t.Fatalf("Save(%s): %v", name, err)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		return data
	}

	first := save(a, "a1.json")
	second := save(a, "a2.json")
	if !bytes.Equal(first, second) {
		t.Errorf("saving the same session twice differed:\n%s\n---\n%s", first, second)
	}
	if other := save(b, "b.json"); !bytes.Equal(first, other) {
		t.Errorf("map/fold build order leaked into the file:\n%s\n---\n%s", first, other)
	}

	// Tab order is user state, so it is written verbatim rather than
	// sorted: z.go was opened first and stays first.
	if bytes.Index(first, []byte("z.go")) > bytes.Index(first, []byte("a.go")) {
		t.Errorf("tabs were reordered on save:\n%s", first)
	}
	// Normalizing happens on a copy: a's fold list is still as built.
	if want := []int{9, 2, 2}; !reflect.DeepEqual(a.Tabs[0].Folds, want) {
		t.Errorf("Save mutated the receiver's Folds: %v, want %v", a.Tabs[0].Folds, want)
	}
}

// TestSaveNormalizesFolds: the fold lines land in the file sorted and
// de-duped, since a host reads them out of gui.CodeEditor's map.
func TestSaveNormalizesFolds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := &Session{Tabs: []Tab{{Path: "a.go", Folds: []int{40, 3, 40, 12}}}}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []int{3, 12, 40}
	if !reflect.DeepEqual(got.Tabs[0].Folds, want) {
		t.Errorf("Folds = %v, want %v", got.Tabs[0].Folds, want)
	}
}

// TestActiveTabClamped: an active index that no longer addresses a tab is
// normalized on save and on load, so restoring never indexes past the tab
// bar. New() starts with nothing focused.
func TestActiveTabClamped(t *testing.T) {
	if s := New(); s.ActiveTab != -1 || s.Version != Version {
		t.Errorf("New() = {Version:%d ActiveTab:%d}, want {%d -1}", s.Version, s.ActiveTab, Version)
	}
	dir := t.TempDir()
	cases := []struct {
		name string
		in   *Session
		want int
	}{
		{"past the end", &Session{Tabs: []Tab{{Path: "a.go"}, {Path: "b.go"}}, ActiveTab: 7}, 0},
		{"negative", &Session{Tabs: []Tab{{Path: "a.go"}}, ActiveTab: -3}, 0},
		{"no tabs", &Session{ActiveTab: 3}, -1},
	}
	for _, c := range cases {
		path := filepath.Join(dir, c.name+".json")
		if err := c.in.Save(path); err != nil {
			t.Fatalf("%s: Save: %v", c.name, err)
		}
		got, err := Load(path)
		if err != nil {
			t.Fatalf("%s: Load: %v", c.name, err)
		}
		if got.ActiveTab != c.want {
			t.Errorf("%s: ActiveTab = %d, want %d", c.name, got.ActiveTab, c.want)
		}
	}
}

// TestLoadKeepsNewerVersion: a file from a future build loads best-effort
// with the fields this build knows, and its version is left alone so the
// newer build is not downgraded behind its back.
func TestLoadKeepsNewerVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := []byte(`{"version":99,"tabs":[{"path":"a.go"}],"active_tab":0,"future":{"x":1}}`)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load of a newer session: %v", err)
	}
	if s.Version != 99 {
		t.Errorf("Version = %d, want 99 (left as written)", s.Version)
	}
	if got := s.Paths(); !reflect.DeepEqual(got, []string{"a.go"}) {
		t.Errorf("Paths() = %v, want [a.go]", got)
	}
}

// TestLoadEdgeFiles: a missing file is reported as such so a first run is
// distinguishable, an empty file is an empty session, and genuinely
// corrupt JSON errors instead of pretending to be v0.
func TestLoadEdgeFiles(t *testing.T) {
	dir := t.TempDir()

	if _, err := Load(filepath.Join(dir, "absent.json")); !os.IsNotExist(err) {
		t.Errorf("Load of a missing file: err = %v, want IsNotExist", err)
	}

	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte("  \n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := Load(empty)
	if err != nil {
		t.Fatalf("Load of an empty file: %v", err)
	}
	if len(s.Tabs) != 0 || s.ActiveTab != -1 || s.Version != Version {
		t.Errorf("Load of an empty file = %+v, want an empty session", s)
	}

	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte(`{"tabs":`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Load(broken); err == nil {
		t.Error("Load of truncated JSON returned no error")
	}
}
