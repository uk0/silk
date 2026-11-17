package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
)

func processAssetTestHooks(t *testing.T) {
	t.Helper()
	load, show, opening := startProcessAssetLoad, showProcessAssetEditor, processAssetOpening
	processAssetOpening = false
	startProcessAssetLoad = func(source string, done func(indexedProcessAsset, error)) error {
		p, i, err := hmi.ParseIndexedSource(source)
		done(indexedProcessAsset{p, i}, err)
		return nil
	}
	t.Cleanup(func() { startProcessAssetLoad, showProcessAssetEditor, processAssetOpening = load, show, opening })
}

func TestProcessAssetEditorMenuPaletteSingleUndoAndDiskRoundtrip(t *testing.T) {
	w := testWorkbench(t)
	processAssetTestHooks(t)
	source, _ := hmi.Template().JSON()
	path := writeFile(t, t.TempDir(), "plant.silkhmi", string(source))
	openTestEditorReady(t, w.tabs, path)
	w.activate(modeEdit)
	ed := activeTestEditorReady(t, w.tabs)
	ed.SetFocus()
	if ed.Language() != "json" {
		t.Fatal("not a JSON document")
	}
	called := 0
	showProcessAssetEditor = func(parent gui.IWidget, p hmi.Project, commit func(hmi.Project) error, _ gui.ProcessDocumentNavigation) (hmi.SourceTarget, error) {
		called++
		if parent != ed || p.Nodes[0].ID != "reactor" {
			t.Fatal("wrong source")
		}
		e := gui.NewProcessEditor()
		defer e.Close()
		e.SetProjectJSON(string(source))
		e.Diagram.SelectNodes([]string{"reactor", "exchanger"})
		if err := e.Diagram.NudgeSelection(20, 10); err != nil {
			return hmi.SourceTarget{}, err
		}
		if err := e.Diagram.DuplicateSelected(); err != nil {
			return hmi.SourceTarget{}, err
		}
		return hmi.SourceTarget{}, commit(e.Diagram.Project())
	}
	menu := buildWorkbenchSubMenu("Tools", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	b := menuCommandButton(menu, "tools.processEditor")
	if b == nil || b.Icon() == nil || !b.IsEnabled() {
		t.Fatal("missing/disabled menu or icon")
	}
	b.Action().Trigger(nil)
	if called != 1 || !ed.Modified() || !ed.CanUndo() {
		t.Fatal("not a buffered transaction")
	}
	disk, _ := os.ReadFile(path)
	if string(disk) != string(source) {
		t.Fatal("hidden disk write")
	}
	edited := ed.Text()
	ed.Undo()
	if ed.Text() != string(source) || ed.CanUndo() {
		t.Fatal("not one host undo step")
	}
	ed.Redo()
	if ed.Text() != edited || !saveEditorToDisk(ed, path) {
		t.Fatal("redo/save failed")
	}
	p, err := hmi.Load(path)
	if err != nil || len(p.Nodes) != 6 || len(p.Links) != 3 || p.Nodes[0].X != 110 || p.Nodes[4].X != 130 {
		t.Fatal("persisted edits incorrect", err, p)
	}
	ed.SetFocus()
	registerPaletteCommands(w.tabs, w.canvas)
	found := false
	for _, cmd := range paletteCommands {
		if cmd.Name == i18n.T("Edit 2D Process...") {
			found = true
			if cmd.Available == nil || !cmd.Available() || cmd.Bind == nil {
				t.Fatal("palette availability missing")
			}
			cmd.Bind().Fn()
		}
	}
	if !found || called != 2 {
		t.Fatal("palette not wired")
	}
}

func TestProcessAssetEditorRejectsLateLoadsStaleWritesAndReadOnly(t *testing.T) {
	w := testWorkbench(t)
	processAssetTestHooks(t)
	source, _ := hmi.Template().JSON()
	path := writeFile(t, t.TempDir(), "plant.SILKHMI", string(source))
	openTestEditorReady(t, w.tabs, path)
	w.activate(modeEdit)
	ed := activeTestEditorReady(t, w.tabs)
	ed.SetFocus()
	var complete func(indexedProcessAsset, error)
	startProcessAssetLoad = func(_ string, done func(indexedProcessAsset, error)) error { complete = done; return nil }
	opened := 0
	showProcessAssetEditor = func(gui.IWidget, hmi.Project, func(hmi.Project) error, gui.ProcessDocumentNavigation) (hmi.SourceTarget, error) {
		opened++
		return hmi.SourceTarget{}, nil
	}
	editProcessAsset(w.tabs, w.canvas, ed)
	if !processAssetOpening {
		t.Fatal("load not reserved")
	}
	ed.ReplaceAllText(string(source) + " ")
	complete(indexedProcessAsset{project: hmi.Template()}, nil)
	if opened != 0 || processAssetOpening {
		t.Fatal("late decode opened")
	}
	ed.SetReadOnly(true)
	editProcessAsset(w.tabs, w.canvas, ed)
	if processAssetOpening {
		t.Fatal("readonly started")
	}
	ed.SetReadOnly(false)
	editProcessAsset(w.tabs, w.canvas, ed)
	openTestEditorReady(t, w.tabs, writeFile(t, t.TempDir(), "other.txt", "untouched"))
	complete(indexedProcessAsset{project: hmi.Template()}, nil)
	if opened != 0 || activeTestEditorReady(t, w.tabs).Text() != "untouched" {
		t.Fatal("late decode followed a different tab")
	}
	focusEditorTab(w.tabs, ed)
	startProcessAssetLoad = func(s string, done func(indexedProcessAsset, error)) error {
		p, i, e := hmi.ParseIndexedSource(s)
		done(indexedProcessAsset{p, i}, e)
		return nil
	}
	showProcessAssetEditor = func(_ gui.IWidget, p hmi.Project, commit func(hmi.Project) error, _ gui.ProcessDocumentNavigation) (hmi.SourceTarget, error) {
		ed.ReplaceAllText(string(source) + "\n\n")
		before := ed.Text()
		p.Nodes[0].X = 10
		if err := commit(p); err == nil || ed.Text() != before {
			t.Fatal("stale commit overwrote source")
		}
		return hmi.SourceTarget{}, nil
	}
	editProcessAsset(w.tabs, w.canvas, ed)
	startProcessAssetLoad = func(string, func(indexedProcessAsset, error)) error { return errors.New("queue closed") }
	editProcessAsset(w.tabs, w.canvas, ed)
	if processAssetOpening {
		t.Fatal("failed submit stuck busy")
	}
}

func TestProcessAssetEditorCancelInvalidAndCRLF(t *testing.T) {
	w := testWorkbench(t)
	processAssetTestHooks(t)
	source, _ := hmi.Template().JSON()
	path := writeFile(t, t.TempDir(), "plant.silkhmi", strings.ReplaceAll(string(source), "\n", "\r\n")+"\r\n")
	openTestEditorReady(t, w.tabs, path)
	w.activate(modeEdit)
	ed := activeTestEditorReady(t, w.tabs)
	ed.SetFocus()
	showProcessAssetEditor = func(gui.IWidget, hmi.Project, func(hmi.Project) error, gui.ProcessDocumentNavigation) (hmi.SourceTarget, error) {
		return hmi.SourceTarget{}, nil
	}
	editProcessAsset(w.tabs, w.canvas, ed)
	if ed.Modified() || ed.CanUndo() {
		t.Fatal("cancel dirtied buffer")
	}
	showProcessAssetEditor = func(_ gui.IWidget, p hmi.Project, commit func(hmi.Project) error, _ gui.ProcessDocumentNavigation) (hmi.SourceTarget, error) {
		p.Nodes[0].Label = "设备 1"
		return hmi.SourceTarget{}, commit(p)
	}
	editProcessAsset(w.tabs, w.canvas, ed)
	if !strings.Contains(ed.Text(), "\r\n") || strings.Contains(strings.ReplaceAll(ed.Text(), "\r\n", ""), "\n") {
		t.Fatal("line endings lost")
	}
	ed.ReplaceAllText(`{"version":99}`)
	before := ed.Text()
	showProcessAssetEditor = func(gui.IWidget, hmi.Project, func(hmi.Project) error, gui.ProcessDocumentNavigation) (hmi.SourceTarget, error) {
		t.Fatal("opened invalid source")
		return hmi.SourceTarget{}, nil
	}
	editProcessAsset(w.tabs, w.canvas, ed)
	if ed.Text() != before || processAssetOpening {
		t.Fatal("invalid source changed")
	}
}

func TestProcessAssetEditorRoundtripTargetsCurrentCRLFBuffer(t *testing.T) {
	w := testWorkbench(t)
	processAssetTestHooks(t)
	data, _ := hmi.Template().JSON()
	path := writeFile(t, t.TempDir(), "plant.silkhmi", string(data))
	openTestEditorReady(t, w.tabs, path)
	w.activate(modeEdit)
	ed := activeTestEditorReady(t, w.tabs)
	buffer := strings.Replace(string(data), `"id": "feed",`, `"tag": "", "id": "removed", "ID": "feed",`, 1)
	// Preserve the same visible path while moving source offsets away from disk.
	buffer = strings.Replace(buffer, `"id": "reactor",`, `"label": "🏭 字", "id": "reactor",`, 1)
	buffer = strings.ReplaceAll("\n\n"+buffer, "\n", "\r\n")
	ed.ReplaceAllText(buffer)
	rowOf := func(marker string) int { return strings.Count(buffer[:strings.Index(buffer, marker)], "\n") }
	nodeRow, pipeRow := rowOf(`"label": "🏭 字"`), rowOf(`"ID": "feed"`)
	pipeSource, _ := ed.LineText(pipeRow)
	wantCol := strings.Index(pipeSource, `"ID": "feed"`) + len(`"ID": "`)
	ed.NavigateToLineCol(nodeRow, 14)
	origin := ed.NavigationPosition()
	rev := ed.TextRevision()
	node := hmi.SourceTarget{Kind: hmi.SourceNode, ID: "reactor"}
	link := hmi.SourceTarget{Kind: hmi.SourceLink, ID: "feed"}
	showProcessAssetEditor = func(_ gui.IWidget, p hmi.Project, _ func(hmi.Project) error, n gui.ProcessDocumentNavigation) (hmi.SourceTarget, error) {
		if n.Initial != node || p.Nodes[0].ID != "reactor" || !n.HasTarget(link) {
			t.Fatal("navigation used wrong source snapshot", n.Initial)
		}
		return link, nil
	}
	editProcessAsset(w.tabs, w.canvas, ed)
	if ed.CursorLine() != pipeRow || ed.CursorCol() != wantCol || ed.TextRevision() != rev || !ed.Modified() || ed.Text() != buffer || !w.navigation.CanGoBack() {
		t.Fatal("incorrect jump or mutation", ed.NavigationPosition(), wantCol)
	}
	currentDisk, err := os.ReadFile(path)
	if err != nil || string(currentDisk) != string(data) {
		t.Fatal("jump saved source", err)
	}
	ed.NavGoBack()
	pumpTestEditorOpens(t, w.tabs)
	if ed.CursorLine() != origin.Line || ed.CursorCol() != origin.Column {
		t.Fatal("history lost old caret", ed.NavigationPosition(), origin)
	}
	ed.NavGoForward()
	pumpTestEditorOpens(t, w.tabs)
	if ed.CursorLine() != pipeRow || ed.CursorCol() != wantCol {
		t.Fatal("history lost destination")
	}
	ed.Undo()
	if ed.Text() != string(data) || ed.CanUndo() {
		t.Fatal("navigation changed undo stack")
	}
}

func TestProcessAssetEditorRejectsReturnedTargetAfterRevisionOrTabChange(t *testing.T) {
	for _, change := range []string{"revision", "tab"} {
		t.Run(change, func(t *testing.T) {
			w := testWorkbench(t)
			processAssetTestHooks(t)
			p, _ := hmi.Template().JSON()
			dir := t.TempDir()
			a := writeFile(t, dir, "a.silkhmi", string(p))
			b := writeFile(t, dir, "b.silkhmi", string(p))
			openTestEditorReady(t, w.tabs, b)
			other := activeTestEditorReady(t, w.tabs)
			openTestEditorReady(t, w.tabs, a)
			w.activate(modeEdit)
			ed := activeTestEditorReady(t, w.tabs)
			showProcessAssetEditor = func(_ gui.IWidget, _ hmi.Project, _ func(hmi.Project) error, n gui.ProcessDocumentNavigation) (hmi.SourceTarget, error) {
				switch change {
				case "revision":
					ed.ReplaceAllText("\n" + ed.Text())
				case "tab":
					focusEditorTab(w.tabs, other)
				}
				return hmi.SourceTarget{Kind: hmi.SourceNode, ID: "reactor"}, nil
			}
			editProcessAsset(w.tabs, w.canvas, ed)
			if ed.CursorLine() != 0 || other.CursorLine() != 0 || w.navigation.CanGoBack() {
				t.Fatal("stale jump changed caret/history", ed.NavigationPosition(), other.NavigationPosition())
			}
		})
	}
}
