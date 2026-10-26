package main

import (
	"path/filepath"
	"slices"
	"testing"
)

func recentOpenWorkbench(t *testing.T) (*workbench, *preferences) {
	t.Helper()
	root := t.TempDir()
	for _, key := range []string{"HOME", "APPDATA", "XDG_CONFIG_HOME"} {
		t.Setenv(key, root)
	}
	p := testPreferences(t)
	old := globalPrefs
	globalPrefs = p
	t.Cleanup(func() { globalPrefs = old })
	return testWorkbench(t), p
}

func TestSuccessfulOpenRoutesShareRecentFiles(t *testing.T) {
	w, p := recentOpenWorkbench(t)
	root := t.TempDir()
	a := writeFile(t, root, "alpha.txt", "alpha\nbeta\n")
	b := writeFile(t, root, "beta.txt", "beta\n")
	if !openFileFinderSelection(w.tabs, a, "") {
		t.Fatal("Quick Open failed")
	}
	if got := p.RecentFiles(); !slices.Equal(got, []string{a}) {
		t.Fatalf("Quick Open MRU=%v", got)
	}
	openFromTree(b, w.tabs, w.canvas, w.center)
	if got := p.RecentFiles(); !slices.Equal(got, []string{b, a}) {
		t.Fatalf("tree MRU=%v", got)
	}
	ed := openEditors[a]
	ed.SetText("unsaved\ncontent\n")
	if !openFileFinderSelection(w.tabs, a, "2:2") {
		t.Fatal("location open failed")
	}
	if openEditors[a] != ed || ed.Text() != "unsaved\ncontent\n" || !ed.Modified() || w.tabs.Count() != 2 {
		t.Fatal("reopen replaced dirty editor")
	}
	if got := p.RecentFiles(); !slices.Equal(got, []string{a, b}) {
		t.Fatalf("reopen MRU=%v", got)
	}
	for _, path := range []string{"", filepath.Join(root, "missing.txt"), root} {
		if openFileInEditor(w.tabs, path) {
			t.Fatalf("invalid file opened: %s", path)
		}
	}
	if openFileInEditor(nil, b) {
		t.Fatal("nil tabs opened file")
	}
	if got := p.RecentFiles(); !slices.Equal(got, []string{a, b}) {
		t.Fatalf("failed open changed MRU: %v", got)
	}
	flushTestPreferences(t, p)
	if got := testPreferences(t).RecentFiles(); !slices.Equal(got, []string{a, b}) {
		t.Fatalf("persisted MRU=%v", got)
	}
}

func TestSessionRestoreDoesNotReorderRecentFiles(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "workspace", true: "legacy"}[legacy], func(t *testing.T) {
			w, p := recentOpenWorkbench(t)
			root := t.TempDir()
			a := writeFile(t, root, "a.txt", "first\nsecond\n")
			b := writeFile(t, root, "b.txt", "other\n")
			p.AddRecentFile(b)
			p.AddRecentFile(a)
			state := workspaceSession{Version: workspaceSessionVersion, Editors: []editorSession{{Path: a, Line: 1, Column: 2}, {Path: b}}, ActiveFile: a, Mode: modeEdit}
			if legacy {
				p.SetOpenSession([]string{a, b})
				restoreSession(w.tabs, w.canvas)
			} else {
				restoreWorkspaceSession(state, w.tabs, w.canvas)
			}
			if w.tabs.Count() != 2 {
				t.Fatal("restore failed")
			}
			if got := p.RecentFiles(); !slices.Equal(got, []string{a, b}) {
				t.Fatalf("restore reordered MRU=%v", got)
			}
			if !legacy && (activeEditor(w.tabs) != openEditors[a] || activeEditor(w.tabs).CursorLine() != 1 || activeEditor(w.tabs).CursorCol() != 2) {
				t.Fatal("restore lost active caret")
			}
		})
	}
}
