package main

import (
	"errors"
	"github.com/uk0/silk/ged"
	"strings"
	"testing"
	"unicode/utf8"
)

func searchWorkbench(t *testing.T) *workbench {
	t.Helper()
	w := renameWorkbench(t)
	old := workspaceSearchReserved
	workspaceSearchReserved = nil
	t.Cleanup(func() { workspaceSearchReserved = old })
	return w
}
func TestWorkspaceSearchNavigationUnicodeAndStale(t *testing.T) {
	w := searchWorkbench(t)
	line := "中🙂 foo target"
	path := writeFile(t, t.TempDir(), "a.txt", line+"\n")
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open")
	}
	ed := openEditors[path]
	m := ged.SearchMatch{FilePath: path, Line: 1, LineText: line, Column: strings.Index(line, "target"), MatchLen: 6}
	if !navigateSearchMatch(ed, m) || ed.CursorCol() != utf8.RuneCountInString(line[:m.Column]) {
		t.Fatal(ed.NavigationPosition())
	}
	ed.ReplaceAllText("newer unsaved buffer")
	before := ed.NavigationPosition()
	if navigateSearchMatch(ed, m) || ed.Text() != "newer unsaved buffer" || ed.NavigationPosition() != before {
		t.Fatal("stale result changed buffer/cursor")
	}
	m.LineText = ed.Text()
	m.Column = 999
	if navigateSearchMatch(ed, m) {
		t.Fatal("bad column accepted")
	}
}
func TestWorkspaceSearchReservationReloadAndUndo(t *testing.T) {
	w := searchWorkbench(t)
	path := writeFile(t, t.TempDir(), "a.txt", "foo\n")
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open")
	}
	ed := openEditors[path]
	reps := []ged.FileReplacement{{Path: path, Before: "foo\n", After: "bar\n", Count: 1}}
	release, err := prepareWorkspaceReplacement(w.tabs, w.canvas, reps)
	if err != nil {
		t.Fatal(err)
	}
	if ed.IsEnabled() || !workspaceSearchAffects(path) || !workspaceSearchBusy() {
		t.Fatal("no reservation")
	}
	if saveEditorToDisk(ed, path) || openFileInEditor(w.tabs, path) || closeEditorTab(w.tabs, editorTabIndex(w.tabs, ed)) || canQuit(w.canvas) {
		t.Fatal("reserved operation accepted")
	}
	if _, err := prepareEditorRename(w.tabs, w.canvas, path, path+"2"); err == nil {
		t.Fatal("rename during apply")
	}
	if _, err := prepareWorkspaceReplacement(w.tabs, w.canvas, reps); err == nil {
		t.Fatal("second write accepted")
	}
	// Use the same SDK transaction as the background UI writer, then its actual
	// host release callback. No test-only replacement/reload implementation.
	if err := ged.NewGlobalSearchPanel().ApplyReplacements(reps); err != nil {
		t.Fatal(err)
	}
	release(nil)
	release(nil)
	if workspaceSearchBusy() || !ed.IsEnabled() || ed.Modified() || ed.Text() != "bar\n" || diskText(t, path) != "bar\n" {
		t.Fatal("reload", ed.Text(), ed.Modified())
	}
	ed.Undo()
	if ed.Text() != "foo\n" || !ed.Modified() {
		t.Fatal("replacement not one undo step", ed.Text())
	}
}
func TestWorkspaceSearchGuardDirtyFailureAndLateEdits(t *testing.T) {
	w := searchWorkbench(t)
	path := writeFile(t, t.TempDir(), "a.txt", "foo")
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open")
	}
	ed := openEditors[path]
	reps := []ged.FileReplacement{{Path: path, Before: "foo", After: "bar"}}
	ed.ReplaceAllText("dirty")
	if _, err := prepareWorkspaceReplacement(w.tabs, w.canvas, reps); err == nil || workspaceSearchBusy() {
		t.Fatal("dirty accepted", err)
	}
	ed.ReplaceAllText("foo")
	ed.MarkSaved()
	release, err := prepareWorkspaceReplacement(w.tabs, w.canvas, reps)
	if err != nil {
		t.Fatal(err)
	}
	release(errors.New("disk conflict"))
	if workspaceSearchBusy() || !ed.IsEnabled() || ed.Text() != "foo" || ed.Modified() {
		t.Fatal("failed write changed buffer")
	}
	release, err = prepareWorkspaceReplacement(w.tabs, w.canvas, reps)
	if err != nil {
		t.Fatal(err)
	}
	ed.ReplaceAllText("programmatic change while writing")
	release(nil)
	if workspaceSearchBusy() || !ed.IsEnabled() || ed.Text() != "programmatic change while writing" || !ed.Modified() {
		t.Fatal("newer buffer lost")
	}
}
func TestWorkspaceSearchGuardActivity(t *testing.T) {
	w := searchWorkbench(t)
	path := writeFile(t, t.TempDir(), "a.txt", "foo")
	reps := []ged.FileReplacement{{Path: path, Before: "foo", After: "bar"}}
	macroBusy = true
	if _, err := prepareWorkspaceReplacement(w.tabs, w.canvas, reps); err == nil {
		t.Fatal("macro accepted")
	}
	macroBusy = false
	activeFileRename = &editorRename{old: path, next: path + "2"}
	if _, err := prepareWorkspaceReplacement(w.tabs, w.canvas, reps); err == nil {
		t.Fatal("rename accepted")
	}
	activeFileRename = nil
	if _, err := prepareWorkspaceReplacement(w.tabs, w.canvas, append(reps, reps...)); err == nil {
		t.Fatal("duplicate accepted")
	}
	if workspaceSearchBusy() {
		t.Fatal("failed preparation leaked reservation")
	}
}
